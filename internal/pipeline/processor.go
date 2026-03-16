// Package pipeline 定义了文件处理的核心流程。
package pipeline

import (
	"bytes" // 引入 bytes 包
	"context"
	"errors"
	"fmt"
	"pai-smart-go/internal/config"
	"pai-smart-go/internal/model"
	"pai-smart-go/internal/repository"
	"pai-smart-go/pkg/embedding"
	"pai-smart-go/pkg/es"
	"pai-smart-go/pkg/log"
	"pai-smart-go/pkg/splitter"
	"pai-smart-go/pkg/storage"
	"pai-smart-go/pkg/tasks"
	"pai-smart-go/pkg/mineru"
	"pai-smart-go/internal/service"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/minio/minio-go/v7"
)

const (
	// embeddingBatchSize 每批向量化的分块数量，减少 API 调用次数
	embeddingBatchSize = 10
	// maxConcurrentWorkers 并发向量化的最大工作协程数
	maxConcurrentWorkers = 5
)

// Processor 封装了文件处理的所有依赖和逻辑。
type Processor struct {
	mineruClient    *mineru.Client
	excelSvc        service.ExcelService
	embeddingClient embedding.Client
	esCfg           config.ElasticsearchConfig
	minioCfg        config.MinIOConfig
	embeddingCfg    config.EmbeddingConfig
	uploadRepo      repository.UploadRepository
	docVectorRepo   repository.DocumentVectorRepository
}

// NewProcessor 创建一个新的 Processor 实例。
func NewProcessor(
	mineruClient *mineru.Client,
	excelSvc service.ExcelService,
	embeddingClient embedding.Client,
	esCfg config.ElasticsearchConfig,
	minioCfg config.MinIOConfig,
	embeddingCfg config.EmbeddingConfig,
	uploadRepo repository.UploadRepository,
	docVectorRepo repository.DocumentVectorRepository,
) *Processor {
	return &Processor{
		mineruClient:    mineruClient,
		excelSvc:        excelSvc,
		embeddingClient: embeddingClient,
		esCfg:           esCfg,
		minioCfg:        minioCfg,
		embeddingCfg:    embeddingCfg,
		uploadRepo:      uploadRepo,
		docVectorRepo:   docVectorRepo,
	}
}

// Process 是文件处理的主函数。
func (p *Processor) Process(ctx context.Context, task tasks.FileProcessingTask) error {
	processStart := time.Now()
	log.Infof("[Processor] 开始处理文件, FileMD5: %s, FileName: %s, UserID: %d", task.FileMD5, task.FileName, task.UserID)

	// 1. 从 MinIO 下载文件
	objectName := fmt.Sprintf("merged/%s", task.FileName)
	log.Infof("[Processor] 步骤1: 从MinIO下载文件, Bucket: %s, Object: %s", p.minioCfg.BucketName, objectName)
	object, err := storage.MinioClient.GetObject(ctx, p.minioCfg.BucketName, objectName, minio.GetObjectOptions{})
	if err != nil {
		log.Errorf("[Processor] 从MinIO下载文件失败, Object: %s, Error: %v", objectName, err)
		return fmt.Errorf("从 MinIO 下载文件失败: %w", err)
	}
	defer object.Close()

	// 增加调试步骤：将文件内容读入内存缓冲区以检查大小
	buf := new(bytes.Buffer)
	size, err := buf.ReadFrom(object)
	if err != nil {
		log.Errorf("[Processor] 从MinIO对象流中读取内容到缓冲区失败, Error: %v", err)
		return fmt.Errorf("读取MinIO对象流失败: %w", err)
	}
	log.Infof("[Processor] 步骤1: 文件下载成功, 从MinIO流中读取到的文件大小为: %d字节", size)
	if size == 0 {
		log.Warnf("[Processor] 文件 '%s' 内容为空, 处理中止", task.FileName)
		return errors.New("文件内容为空")
	}

	// 2. 根据文件类型选择解析引擎
	ext := strings.ToLower(filepath.Ext(task.FileName))
	var textContent string

	if isExcelFile(ext) {
		log.Infof("[Processor] 步骤2: 检测到 Excel 文件, 进入数据工程管线: %s", task.FileName)
		if err := p.excelSvc.ProcessExcel(bytes.NewReader(buf.Bytes()), task.FileMD5); err != nil {
			log.Errorf("[Processor] Excel 数据工程处理失败, FileName: %s, Error: %v", task.FileName, err)
			return fmt.Errorf("Excel 数据工程处理失败: %w", err)
		}
		log.Infof("[Processor] 步骤2: Excel 数据导入完成, FileName: %s. 注意: 结构化数据不进入向量检索流程。", task.FileName)
		return nil // Excel 处理完直接返回，不走向量化切块流程
	}

	if isSmartDocFile(ext) {
		log.Infof("[Processor] 步骤2: 使用 MinerU (gRPC) 提取 PDF/Doc/PPT 文本内容")
		content, err := p.mineruClient.Parse(ctx, task.FileName, buf.Bytes())
		if err != nil {
			log.Errorf("[Processor] 使用 MinerU 提取文本失败, FileName: %s, Error: %v", task.FileName, err)
			return fmt.Errorf("使用 MinerU 提取文本失败: %w", err)
		}
		textContent = content
	} else {
		log.Infof("[Processor] 步骤2: 使用原生方式读取 TXT/MD/Code 内容")
		textContent = string(buf.Bytes())
	}

	if textContent == "" {
		log.Warnf("[Processor] 提取的文本内容为空, 处理中止, FileName: %s", task.FileName)
		return errors.New("提取的文本内容为空")
	}
	log.Infof("[Processor] 步骤2: 文本解析成功, 内容长度: %d 字符", utf8.RuneCountInString(textContent))

	// 3. 自适应文本切块 —— 根据文件类型选择最优分块策略
	chunkSize, chunkOverlap := p.selectChunkParams(task.FileName)
	log.Infof("[Processor] 步骤3: 进行文本分块, chunkSize: %d, chunkOverlap: %d, strategy: %s",
		chunkSize, chunkOverlap, p.describeChunkStrategy(task.FileName))
	chunks := p.splitText(textContent, chunkSize, chunkOverlap, task.FileName)
	log.Infof("[Processor] 步骤3: 文本分块完成, 共生成 %d 个分块", len(chunks))
	if len(chunks) == 0 {
		log.Warnf("[Processor] 未生成任何文本分块, 处理中止, FileName: %s", task.FileName)
		return errors.New("未生成任何文本分块")
	}

	// 阶段一：将分块文本和元数据存入数据库
	log.Info("[Processor] 阶段一: 开始将分块文本存入数据库")
	// 为避免重复写入导致的累计膨胀，处理前先清理该文件既有的分块记录（幂等）
	if err := p.docVectorRepo.DeleteByFileMD5(task.FileMD5); err != nil {
		log.Warnf("[Processor] 清理 document_vectors 旧记录失败 (file_md5=%s): %v", task.FileMD5, err)
	}
	// 新增：同步清理 ES 中的旧分块
	if err := es.DeleteByFileMD5(ctx, p.esCfg.IndexName, task.FileMD5); err != nil {
		log.Warnf("[Processor] 清理 Elasticsearch 旧记录失败 (file_md5=%s): %v", task.FileMD5, err)
	}
	dbVectors := make([]*model.DocumentVector, 0, len(chunks))
	for i, chunk := range chunks {
		dbVectors = append(dbVectors, &model.DocumentVector{
			FileMD5:     task.FileMD5,
			ChunkID:     i,
			TextContent: chunk,
			FileName:    task.FileName, // 新增：保存文件名到 DB
			UserID:      task.UserID,
			OrgTag:      task.OrgTag,
			IsPublic:    task.IsPublic,
		})
	}
	if err := p.docVectorRepo.BatchCreate(dbVectors); err != nil {
		log.Errorf("[Processor] 阶段一: 批量保存文本分块到数据库失败, Error: %v", err)
		return fmt.Errorf("批量保存文本分块失败: %w", err)
	}
	log.Infof("[Processor] 阶段一: 成功将 %d 个分块存入数据库", len(dbVectors))

	// 阶段二：从数据库读取，进行批量向量化，然后索引到ES
	log.Info("[Processor] 阶段二: 开始从数据库读取分块并进行批量向量化")
	savedVectors, err := p.docVectorRepo.FindByFileMD5(task.FileMD5)
	if err != nil {
		log.Errorf("[Processor] 阶段二: 从数据库读取分块失败, FileMD5: %s, Error: %v", task.FileMD5, err)
		return fmt.Errorf("从数据库读取分块失败: %w", err)
	}
	log.Infof("[Processor] 阶段二: 成功从数据库读取 %d 个分块", len(savedVectors))

	// 4. 批量向量化并索引到 ES（带并发控制和部分失败容错）
	log.Info("[Processor] 步骤4: 开始批量向量化与索引")
	failedChunks := p.batchEmbedAndIndex(ctx, savedVectors)

	successCount := len(savedVectors) - len(failedChunks)
	if len(failedChunks) > 0 {
		log.Warnf("[Processor] 文件 %s 有 %d/%d 个分块处理失败: %v",
			task.FileMD5, len(failedChunks), len(savedVectors), failedChunks)
	}

	if successCount == 0 && len(savedVectors) > 0 {
		return fmt.Errorf("所有 %d 个分块均处理失败", len(savedVectors))
	}

	elapsed := time.Since(processStart)
	log.Infof("[Processor] 文件处理完成, FileMD5: %s, 成功: %d/%d, 耗时: %.2fs",
		task.FileMD5, successCount, len(savedVectors), elapsed.Seconds())
	return nil
}

// batchEmbedAndIndex 批量向量化并索引到 ES。
// 使用信号量控制并发度，支持部分失败容错。
// 返回失败的 ChunkID 列表。
func (p *Processor) batchEmbedAndIndex(ctx context.Context, savedVectors []*model.DocumentVector) []int {
	var failedChunks []int
	var mu sync.Mutex
	sem := make(chan struct{}, maxConcurrentWorkers)
	var wg sync.WaitGroup

	// 按 batchSize 分批处理
	for i := 0; i < len(savedVectors); i += embeddingBatchSize {
		end := i + embeddingBatchSize
		if end > len(savedVectors) {
			end = len(savedVectors)
		}
		batch := savedVectors[i:end]
		batchIdx := i / embeddingBatchSize

		wg.Add(1)
		go func(batch []*model.DocumentVector, batchIdx int) {
			defer wg.Done()
			sem <- struct{}{}        // 获取信号量
			defer func() { <-sem }() // 释放信号量

			log.Infof("[Processor] 批次 %d: 开始处理 %d 个分块", batchIdx, len(batch))

			// 收集文本
			texts := make([]string, len(batch))
			for j, dv := range batch {
				texts[j] = dv.TextContent
			}

			// 批量向量化
			vectors, err := p.embeddingClient.CreateEmbeddingBatch(ctx, texts)
			if err != nil {
				log.Errorf("[Processor] 批次 %d 批量向量化失败, Error: %v", batchIdx, err)
				mu.Lock()
				for _, dv := range batch {
					failedChunks = append(failedChunks, dv.ChunkID)
				}
				mu.Unlock()
				return
			}

			if len(vectors) != len(batch) {
				log.Errorf("[Processor] 批次 %d 向量化结果数量不匹配: 期望 %d, 实际 %d",
					batchIdx, len(batch), len(vectors))
				mu.Lock()
				for _, dv := range batch {
					failedChunks = append(failedChunks, dv.ChunkID)
				}
				mu.Unlock()
				return
			}

			// 逐个索引到 ES（单个索引失败不影响同批次其他分块）
			for j, dv := range batch {
				esDoc := model.EsDocument{
					VectorID:     fmt.Sprintf("%s_%d", dv.FileMD5, dv.ChunkID),
					FileMD5:      dv.FileMD5,
					ChunkID:      dv.ChunkID,
					FileName:     dv.FileName, // 新增：索引时包含文件名
					TextContent:  dv.TextContent,
					Vector:       vectors[j],
					ModelVersion: p.embeddingCfg.Model,
					UserID:       dv.UserID,
					OrgTag:       dv.OrgTag,
					IsPublic:     dv.IsPublic,
				}

				if err := es.IndexDocument(ctx, p.esCfg.IndexName, esDoc); err != nil {
					log.Warnf("[Processor] 分块 %d 索引到ES失败, 跳过: %v", dv.ChunkID, err)
					mu.Lock()
					failedChunks = append(failedChunks, dv.ChunkID)
					mu.Unlock()
					continue
				}
			}
			log.Infof("[Processor] 批次 %d 处理完成", batchIdx)
		}(batch, batchIdx)
	}

	wg.Wait()
	return failedChunks
}

// selectChunkParams 根据文件类型返回最优的分块参数。
func (p *Processor) selectChunkParams(fileName string) (chunkSize int, chunkOverlap int) {
	ext := strings.ToLower(filepath.Ext(fileName))
	switch ext {
	case ".md", ".markdown":
		return 1500, 150
	case ".go", ".java", ".py", ".js", ".ts", ".c", ".cpp":
		return 1200, 100
	default:
		return 1000, 100
	}
}

// describeChunkStrategy 返回当前文件使用的分块策略描述
func (p *Processor) describeChunkStrategy(fileName string) string {
	ext := strings.ToLower(filepath.Ext(fileName))
	switch ext {
	case ".md", ".markdown":
		return "MarkdownSplitter"
	case ".go", ".java", ".py", ".js", ".ts", ".c", ".cpp":
		return "CodeAware"
	default:
		return "Default"
	}
}

// splitText 将长文本按指定大小和重叠进行切分。
func (p *Processor) splitText(text string, chunkSize int, chunkOverlap int, fileName string) []string {
	// 1. 判断是否为 Markdown 文件
	if isMarkdownFile(fileName) {
		log.Infof("[Processor] 检测到 Markdown 文件, 使用 MarkdownSplitter: %s", fileName)
		mdSplitter := splitter.NewMarkdownSplitter()
		chunks := mdSplitter.Split(text, chunkSize, chunkOverlap)
		if len(chunks) > 0 {
			return chunks
		}
		log.Warnf("[Processor] MarkdownSplitter 降级处理")
	}

	// 2. 判断是否为代码文件
	if isCodeFile(fileName) {
		log.Infof("[Processor] 检测到代码文件, 使用 CodeSplitter: %s", fileName)
		codeSplitter := splitter.NewCodeSplitter(filepath.Ext(fileName))
		chunks := codeSplitter.Split(text, chunkSize, chunkOverlap)
		if len(chunks) > 0 {
			return chunks
		}
		log.Warnf("[Processor] CodeSplitter 降级处理")
	}

	// 3. 默认：简单字符切分
	if chunkSize <= chunkOverlap {
		return p.simpleSplit(text, chunkSize)
	}

	var chunks []string
	runes := []rune(text)
	if len(runes) == 0 {
		return nil
	}

	step := chunkSize - chunkOverlap
	for i := 0; i < len(runes); i += step {
		end := i + chunkSize
		if end > len(runes) {
			end = len(runes)
		}
		chunks = append(chunks, string(runes[i:end]))
		if end == len(runes) {
			break
		}
	}
	return chunks
}

func (p *Processor) simpleSplit(text string, chunkSize int) []string {
	var chunks []string
	runes := []rune(text)
	if len(runes) == 0 {
		return nil
	}
	for i := 0; i < len(runes); i += chunkSize {
		end := i + chunkSize
		if end > len(runes) {
			end = len(runes)
		}
		chunks = append(chunks, string(runes[i:end]))
	}
	return chunks
}

// isMarkdownFile 判断文件是否为 Markdown 格式
func isMarkdownFile(fileName string) bool {
	lower := strings.ToLower(fileName)
	return strings.HasSuffix(lower, ".md") || strings.HasSuffix(lower, ".markdown")
}

// isCodeFile 判断文件是否为常见的代码文件格式
func isCodeFile(fileName string) bool {
	ext := strings.ToLower(filepath.Ext(fileName))
	codeExts := map[string]bool{
		".go": true, ".java": true, ".py": true, ".js": true, ".ts": true,
		".c": true, ".cpp": true, ".h": true, ".hpp": true, ".cs": true,
		".php": true, ".rb": true, ".rs": true, ".sh": true, ".sql": true,
	}
	return codeExts[ext]
}
// isSmartDocFile 判断是否为 MinerU 擅长处理的“智能文档”类型
func isSmartDocFile(ext string) bool {
	exts := map[string]bool{
		".pdf": true, ".docx": true, ".doc": true, ".pptx": true, ".ppt": true,
	}
	return exts[ext]
}

// isExcelFile 判断是否为表格数据类型
func isExcelFile(ext string) bool {
	exts := map[string]bool{
		".xlsx": true, ".xls": true, ".csv": true,
	}
	return exts[ext]
}
