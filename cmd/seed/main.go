// Package main 是测试数据导入工具。
//
// 用途：将指定目录下的文档直接向量化并索引到 Elasticsearch 和 MySQL，
// 供 cmd/eval 评估程序使用，无需启动完整的 HTTP 服务或 Kafka。
//
// 用法：
//
//	go run ./cmd/seed/main.go -dir test_docs_crypto
package main

import (
	"context"
	"crypto/md5"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"pai-smart-go/internal/config"
	"pai-smart-go/internal/model"
	"pai-smart-go/internal/repository"
	"pai-smart-go/pkg/database"
	"pai-smart-go/pkg/embedding"
	"pai-smart-go/pkg/es"
	"pai-smart-go/pkg/log"
)

func main() {
	dir := flag.String("dir", "test_docs_crypto", "要导入的文档目录")
	flag.Parse()

	// ── 1. 初始化 ──
	config.Init("./configs/config.yaml")
	cfg := config.Conf
	log.Init("info", "text", "")

	if err := es.InitES(cfg.Elasticsearch); err != nil {
		panic(fmt.Sprintf("ES 初始化失败: %v", err))
	}
	database.InitMySQL(cfg.Database.MySQL.DSN)
	database.InitRedis(cfg.Database.Redis.Addr, cfg.Database.Redis.Password, cfg.Database.Redis.DB)

	embeddingClient := embedding.NewClient(cfg.Embedding)
	docVectorRepo := repository.NewDocumentVectorRepository(database.DB)

	// ── 2. 扫描目录 ──
	entries, err := os.ReadDir(*dir)
	if err != nil {
		log.Fatalf("读取目录失败: %v", err)
	}

	total := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		path := filepath.Join(*dir, name)

		data, err := os.ReadFile(path)
		if err != nil {
			fmt.Printf("[WARN] 读取文件失败: %s, err=%v\n", name, err)
			continue
		}

		// 计算 MD5
		sum := md5.Sum(data)
		fileMD5 := fmt.Sprintf("%x", sum)

		content := string(data)
		if utf8.RuneCountInString(content) == 0 {
			fmt.Printf("[SKIP] 空文件: %s\n", name)
			continue
		}

		// ── 3. 切块 (1000 字符，100 重叠) ──
		chunks := splitText(content, 1000, 100)
		fmt.Printf("[INFO] 文件: %-40s  MD5: %s  分块数: %d\n", name, fileMD5[:8]+"...", len(chunks))

		// ── 4. 清理旧数据（幂等） ──
		if err := docVectorRepo.DeleteByFileMD5(fileMD5); err != nil {
			fmt.Printf("[WARN] 清理 DB 旧记录失败: %v\n", err)
		}
		if err := es.DeleteByFileMD5(context.Background(), cfg.Elasticsearch.IndexName, fileMD5); err != nil {
			fmt.Printf("[WARN] 清理 ES 旧记录失败: %v\n", err)
		}

		// ── 5. 写入 MySQL document_vectors ──
		var dbVectors []*model.DocumentVector
		for i, chunk := range chunks {
			dbVectors = append(dbVectors, &model.DocumentVector{
				FileMD5:     fileMD5,
				FileName:    name,
				ChunkID:     i,
				TextContent: chunk,
				UserID:      1,
				OrgTag:      "",
				IsPublic:    true,
			})
		}
		if err := docVectorRepo.BatchCreate(dbVectors); err != nil {
			fmt.Printf("[ERROR] 写入 DB 失败: %s, err=%v\n", name, err)
			continue
		}

		// ── 6. 批量 Embedding + 写入 ES ──
		const batchSize = 5
		for i := 0; i < len(chunks); i += batchSize {
			end := i + batchSize
			if end > len(chunks) {
				end = len(chunks)
			}
			batch := chunks[i:end]

			vectors, err := embeddingClient.CreateEmbeddingBatch(context.Background(), batch)
			if err != nil {
				fmt.Printf("[ERROR] Embedding 失败: %s batch=%d, err=%v\n", name, i/batchSize, err)
				continue
			}

			for j, vec := range vectors {
				chunkIdx := i + j
				doc := model.EsDocument{
					VectorID:     fmt.Sprintf("%s_%d", fileMD5, chunkIdx),
					FileMD5:      fileMD5,
					FileName:     name,
					ChunkID:      chunkIdx,
					TextContent:  batch[j],
					Vector:       vec,
					ModelVersion: cfg.Embedding.Model,
					UserID:       1,
					OrgTag:       "",
					IsPublic:     true,
				}
				if err := es.IndexDocument(context.Background(), cfg.Elasticsearch.IndexName, doc); err != nil {
					fmt.Printf("[WARN] ES 索引失败: %s chunk=%d, err=%v\n", name, chunkIdx, err)
				}
			}
			// 避免 API 限速
			time.Sleep(300 * time.Millisecond)
		}

		total++
		fmt.Printf("[DONE] %-40s (%d 块已索引)\n", name, len(chunks))
	}

	fmt.Printf("\n✅ 导入完成，共处理 %d 个文件。现在可以运行 'go run ./cmd/eval/main.go' 进行评估。\n", total)
}

// splitText 将长文本按 chunkSize/overlap 切块（rune 安全）。
func splitText(text string, chunkSize, chunkOverlap int) []string {
	runes := []rune(text)
	if len(runes) == 0 {
		return nil
	}
	step := chunkSize - chunkOverlap
	if step <= 0 {
		step = chunkSize
	}
	var chunks []string
	for i := 0; i < len(runes); i += step {
		end := i + chunkSize
		if end > len(runes) {
			end = len(runes)
		}
		chunk := strings.TrimSpace(string(runes[i:end]))
		if chunk != "" {
			chunks = append(chunks, chunk)
		}
		if end == len(runes) {
			break
		}
	}
	return chunks
}
