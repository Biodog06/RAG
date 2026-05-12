// Package main 是 RAG 检索性能评估程序的入口。
//
// 用法：
//
//	go run ./cmd/eval/main.go
//
// 输出：
//   - 控制台打印汇总表格与每条 Query 明细
//   - 将结果写入 eval_report.json 文件
//
// 注意：本版本已注释掉 Neo4j / Graph RAG 相关逻辑。
// 如需启用，请先在 config.yaml 中恢复 neo4j 配置，并取消本文件中对应的注释。
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"pai-smart-go/internal/config"
	"pai-smart-go/internal/model"
	"pai-smart-go/internal/repository"
	"pai-smart-go/internal/service"
	"pai-smart-go/pkg/database"
	"pai-smart-go/pkg/embedding"
	"pai-smart-go/pkg/es"
	"pai-smart-go/pkg/evaluation"
	"pai-smart-go/pkg/llm"
	"pai-smart-go/pkg/log"
	"pai-smart-go/pkg/rerank"
	"strings"
	"time"

	"github.com/go-redis/redis/v8"
)

// ─────────────────────────────────────────────
// 数据结构定义
// ─────────────────────────────────────────────

// TestCaseData 从 JSON 文件中加载的单条测试用例。
type TestCaseData struct {
	Query        string   `json:"query"`
	RelevantDocs []string `json:"relevant_docs"`
}

// QueryResult 单条 Query 的评估结果，用于明细输出和 JSON 导出。
type QueryResult struct {
	Query    string  `json:"query"`
	MRR      float64 `json:"mrr"`
	P3       float64 `json:"precision_at_3"`
	NDCG3    float64 `json:"ndcg_at_3"`
	Recall5  float64 `json:"recall_at_5"`
	Recall10 float64 `json:"recall_at_10"`
	Hit      bool    `json:"hit"`
}

// ExperimentReport 单个实验方案的完整报告。
type ExperimentReport struct {
	Name        string        `json:"name"`
	RunAt       string        `json:"run_at"`
	AvgMRR      float64       `json:"avg_mrr"`
	AvgP3       float64       `json:"avg_precision_at_3"`
	AvgNDCG3    float64       `json:"avg_ndcg_at_3"`
	AvgRecall5  float64       `json:"avg_recall_at_5"`
	AvgRecall10 float64       `json:"avg_recall_at_10"`
	HitRate     float64       `json:"hit_rate"`
	TotalCases  int           `json:"total_cases"`
	Details     []QueryResult `json:"details"`
}

// ─────────────────────────────────────────────
// 主函数
// ─────────────────────────────────────────────

func main() {
	// 1. 初始化配置与基础设施
	config.Init("./configs/config.yaml")
	cfg := config.Conf
	log.Init("info", "text", "")

	if err := es.InitES(cfg.Elasticsearch); err != nil {
		panic(fmt.Sprintf("Elasticsearch 初始化失败: %v", err))
	}
	database.InitMySQL(cfg.Database.MySQL.DSN) // 内部遇错会直接 Fatal
	database.InitRedis(cfg.Database.Redis.Addr, cfg.Database.Redis.Password, cfg.Database.Redis.DB)

	// --- Neo4j 初始化（已注释）---
	// 如需启用 Graph RAG 评估，请先在 config.yaml 中恢复 neo4j 配置，再取消以下注释。
	// if err := database.InitNeo4j(cfg.Database.Neo4j); err != nil {
	//     log.Warnf("Neo4j not available, skipping graph parts: %v", err)
	// }

	// 2. 初始化服务依赖
	userRepo := repository.NewUserRepository(database.DB)
	orgTagRepo := repository.NewOrgTagRepository(database.DB)
	uploadRepo := repository.NewUploadRepository(database.DB, database.RDB)
	_ = repository.NewDocumentVectorRepository(database.DB) // 仅用于触发 AutoMigrate，查询直接用 database.DB

	redisClient := redis.NewClient(&redis.Options{
		Addr:     cfg.Database.Redis.Addr,
		Password: cfg.Database.Redis.Password,
		DB:       cfg.Database.Redis.DB,
	})
	embeddingClient := embedding.NewClient(cfg.Embedding)
	rerankClient := rerank.NewClient(cfg.Rerank)
	llmClient := llm.NewClient(cfg.LLM)
	userService := service.NewUserService(userRepo, orgTagRepo, nil)

	searchService := service.NewSearchService(
		embeddingClient, es.ESClient, userService,
		uploadRepo, rerankClient, llmClient,
		cfg.Segmenter, redisClient,
	)

	// --- 图增强检索服务（已注释）---
	// 如需启用 Graph RAG 评估，请先启用 Neo4j，再取消以下注释。
	// graphSearchService := service.NewGraphSearchService(llmClient)

	// 3. 加载测试用例
	data, err := os.ReadFile("test_cases_crypto.json")
	if err != nil {
		log.Fatalf("读取测试用例失败: %v", err)
	}
	var rawCases []TestCaseData
	if err := json.Unmarshal(data, &rawCases); err != nil {
		log.Fatalf("解析测试用例 JSON 失败: %v", err)
	}

	// 4. 构建文件名 → MD5 映射（以文件级 MD5 作为评估颗粒度）
	nameToMD5 := make(map[string]string)
	var allDocs []model.DocumentVector
	database.DB.Select("file_name, file_md5").Distinct("file_name, file_md5").Find(&allDocs)
	for _, d := range allDocs {
		nameToMD5[d.FileName] = d.FileMD5
	}
	log.Infof("共加载 %d 条文件名→MD5 映射，%d 条测试用例", len(nameToMD5), len(rawCases))

	// 5. 执行评估实验
	sep := strings.Repeat("═", 100)
	fmt.Println("\n" + sep)
	fmt.Println("   RAG 检索性能评估报告 (A/B 测试)")
	fmt.Printf("   运行时间: %s\n", time.Now().Format("2006-01-02 15:04:05"))
	fmt.Println(sep)

	var reports []ExperimentReport

	// ── 方案 A：纯混合检索（向量 + BM25 + Rerank）──
	reportA := runExperiment(
		"Hybrid Search (Vector+BM25+Rerank)",
		rawCases, nameToMD5,
		func(query string) []evaluation.SearchResult {
			res, _ := searchService.HybridSearch(
				context.Background(), query, 10,
				&model.User{ID: 1}, nil,
			)
			return toEvalResults(res)
		},
	)
	reports = append(reports, reportA)
	printReport(reportA)

	// ── 方案 B：Graph RAG（已注释，等待 Neo4j 就绪）──
	// 取消注释后可对比图增强的效果差异。
	//
	// reportB := runExperiment(
	//     "Graph-Augmented RAG",
	//     rawCases, nameToMD5,
	//     func(query string) []evaluation.SearchResult {
	//         ctx := context.Background()
	//         // 若需要图知识融合，可在此调用 graphSearchService.Search(ctx, query)
	//         // 并对结果做权重重排。
	//         res, _ := searchService.HybridSearch(ctx, query, 10, &model.User{ID: 1}, nil)
	//         return toEvalResults(res)
	//     },
	// )
	// reports = append(reports, reportB)
	// printReport(reportB)

	fmt.Println(sep)

	// 6. 将结果写入 JSON 文件
	outputPath := "eval_report.json"
	if err := writeJSON(outputPath, reports); err != nil {
		log.Warnf("写入评估报告 JSON 失败: %v", err)
	} else {
		fmt.Printf("\n✅ 评估报告已写入：%s\n", outputPath)
	}
}

// ─────────────────────────────────────────────
// 评估核心逻辑
// ─────────────────────────────────────────────

// runExperiment 遍历所有测试用例，计算各项指标，返回完整实验报告。
func runExperiment(
	name string,
	cases []TestCaseData,
	nameToMD5 map[string]string,
	searchFunc func(query string) []evaluation.SearchResult,
) ExperimentReport {
	var (
		totalMRR, totalP3, totalNDCG3  float64
		totalRecall5, totalRecall10    float64
		hits                           int
		details                        []QueryResult
	)

	for _, c := range cases {
		results := deduplicateByDocID(searchFunc(c.Query))

		// 构建此 Query 的标准答案集（相关度设为最高 4 档）
		relevantMD5s := make(map[string]int)
		for _, docName := range c.RelevantDocs {
			if md5, ok := nameToMD5[docName]; ok {
				relevantMD5s[md5] = 4
			}
		}
		gt := evaluation.GroundTruth{RelevantDocIDs: relevantMD5s}

		mrr := evaluation.CalculateMRR(results, gt)
		p3 := evaluation.CalculatePrecisionAtK(results, gt, 3)
		ndcg3 := evaluation.CalculateNDCG(results, gt, 3)
		recall5 := calculateRecallAtK(results, gt, 5)
		recall10 := calculateRecallAtK(results, gt, 10)

		totalMRR += mrr
		totalP3 += p3
		totalNDCG3 += ndcg3
		totalRecall5 += recall5
		totalRecall10 += recall10
		hit := mrr > 0
		if hit {
			hits++
		}

		details = append(details, QueryResult{
			Query:    c.Query,
			MRR:      mrr,
			P3:       p3,
			NDCG3:    ndcg3,
			Recall5:  recall5,
			Recall10: recall10,
			Hit:      hit,
		})
	}

	total := float64(len(cases))
	return ExperimentReport{
		Name:        name,
		RunAt:       time.Now().Format(time.RFC3339),
		AvgMRR:      safeDivide(totalMRR, total),
		AvgP3:       safeDivide(totalP3, total),
		AvgNDCG3:    safeDivide(totalNDCG3, total),
		AvgRecall5:  safeDivide(totalRecall5, total),
		AvgRecall10: safeDivide(totalRecall10, total),
		HitRate:     safeDivide(float64(hits), total),
		TotalCases:  len(cases),
		Details:     details,
	}
}

// ─────────────────────────────────────────────
// 报告打印
// ─────────────────────────────────────────────

// printReport 将实验报告以美观格式打印到控制台。
func printReport(r ExperimentReport) {
	fmt.Printf("\n▶ 方案：%s\n", r.Name)
	fmt.Println(strings.Repeat("─", 100))
	fmt.Printf("  📊 汇总指标（共 %d 条测试用例）\n", r.TotalCases)
	fmt.Printf("     %-12s %-12s %-12s %-14s %-14s %-10s\n",
		"MRR", "P@3", "NDCG@3", "Recall@5", "Recall@10", "命中率")
	fmt.Printf("     %-12.4f %-12.4f %-12.4f %-14.4f %-14.4f %-10.4f\n",
		r.AvgMRR, r.AvgP3, r.AvgNDCG3, r.AvgRecall5, r.AvgRecall10, r.HitRate)
	fmt.Println()
	fmt.Printf("  📋 逐条 Query 明细\n")
	fmt.Printf("     %-4s  %-40s  %-6s  %-6s  %-8s  %-8s  %-10s  %s\n",
		"#", "Query", "MRR", "P@3", "NDCG@3", "R@5", "R@10", "命中")
	fmt.Println("     " + strings.Repeat("-", 95))
	for i, d := range r.Details {
		query := d.Query
		if len([]rune(query)) > 20 {
			query = string([]rune(query)[:20]) + "…"
		}
		hitStr := "❌"
		if d.Hit {
			hitStr = "✅"
		}
		fmt.Printf("     %-4d  %-40s  %-6.3f  %-6.3f  %-8.3f  %-8.3f  %-10.3f  %s\n",
			i+1, query, d.MRR, d.P3, d.NDCG3, d.Recall5, d.Recall10, hitStr)
	}
	fmt.Println(strings.Repeat("─", 100))

	// 质量评级
	rating, advice := qualityRating(r.AvgMRR, r.HitRate)
	fmt.Printf("  🏷  质量评级：%s\n", rating)
	fmt.Printf("  💡 优化建议：%s\n\n", advice)
}

// qualityRating 根据 MRR 和命中率给出综合评级及建议。
func qualityRating(mrr, hitRate float64) (rating, advice string) {
	switch {
	case mrr >= 0.8 && hitRate >= 0.9:
		return "🌟 优秀 (Excellent)", "检索质量卓越，可继续关注长尾 Query 的表现。"
	case mrr >= 0.6 && hitRate >= 0.7:
		return "🟢 良好 (Good)", "整体检索效果不错，可尝试扩充语料或调整 Rerank 阈值进一步提升。"
	case mrr >= 0.4 && hitRate >= 0.5:
		return "🟡 一般 (Fair)", "部分 Query 命中率偏低，建议检查分块策略和 Embedding 质量。"
	default:
		return "🔴 待改进 (Poor)", "命中率较低，建议检查知识库文档完整性、调整分块大小或更换 Embedding 模型。"
	}
}

// ─────────────────────────────────────────────
// 工具函数
// ─────────────────────────────────────────────

// toEvalResults 将服务层检索结果转换为 evaluation 包使用的格式。
func toEvalResults(res []model.SearchResponseDTO) []evaluation.SearchResult {
	converted := make([]evaluation.SearchResult, 0, len(res))
	for _, r := range res {
		converted = append(converted, evaluation.SearchResult{
			DocID: r.FileMD5, // 以文件 MD5 为评估颗粒度
			Score: r.Score,
		})
	}
	return converted
}

// deduplicateByDocID 按文件级 DocID 去重，保留每个 DocID 首次出现（默认即高分优先）。
func deduplicateByDocID(results []evaluation.SearchResult) []evaluation.SearchResult {
	seen := make(map[string]struct{}, len(results))
	unique := make([]evaluation.SearchResult, 0, len(results))
	for _, r := range results {
		if _, ok := seen[r.DocID]; ok {
			continue
		}
		seen[r.DocID] = struct{}{}
		unique = append(unique, r)
	}
	return unique
}

// calculateRecallAtK 计算 Recall@K：Top-K 中召回的相关文档数 / 总相关文档数。
func calculateRecallAtK(results []evaluation.SearchResult, gt evaluation.GroundTruth, k int) float64 {
	totalRelevant := 0
	for _, grade := range gt.RelevantDocIDs {
		if grade > 0 {
			totalRelevant++
		}
	}
	if totalRelevant == 0 {
		return 0
	}
	n := k
	if len(results) < n {
		n = len(results)
	}
	retrieved := 0
	for i := 0; i < n; i++ {
		if grade, ok := gt.RelevantDocIDs[results[i].DocID]; ok && grade > 0 {
			retrieved++
		}
	}
	return float64(retrieved) / float64(totalRelevant)
}

// safeDivide 安全除法，除数为 0 时返回 0。
func safeDivide(a, b float64) float64 {
	if b == 0 {
		return 0
	}
	return a / b
}

// writeJSON 将报告以 JSON 格式写入指定文件。
func writeJSON(path string, data interface{}) error {
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0644)
}
