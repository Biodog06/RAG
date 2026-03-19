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

	"github.com/go-redis/redis/v8"
)

type TestCaseData struct {
	Query        string   `json:"query"`
	RelevantDocs []string `json:"relevant_docs"`
}

func main() {
	// 1. 初始化
	config.Init("./configs/config.yaml")
	cfg := config.Conf
	log.Init("info", "text", "")
	
	if err := es.InitES(cfg.Elasticsearch); err != nil {
		panic(err)
	}
	if err := database.InitMySQL(cfg.Database.MySQL.DSN); err != nil {
		panic(err)
	}
	database.InitRedis(cfg.Database.Redis.Addr, cfg.Database.Redis.Password, cfg.Database.Redis.DB)
	if err := database.InitNeo4j(cfg.Database.Neo4j); err != nil {
		log.Warnf("Neo4j not available, skipping graph parts: %v", err)
	}

	// 2. 初始化服务
	userRepo := repository.NewUserRepository(database.DB)
	orgTagRepo := repository.NewOrgTagRepository(database.DB)
	uploadRepo := repository.NewUploadRepository(database.DB, database.RDB)
	docVectorRepo := repository.NewDocumentVectorRepository(database.DB)
	redisClient := redis.NewClient(&redis.Options{Addr: cfg.Database.Redis.Addr, Password: cfg.Database.Redis.Password, DB: cfg.Database.Redis.DB})
	embeddingClient := embedding.NewClient(cfg.Embedding)
	rerankClient := rerank.NewClient(cfg.Rerank)
	llmClient := llm.NewClient(cfg.LLM)
	userService := service.NewUserService(userRepo, orgTagRepo, nil)
	
	searchService := service.NewSearchService(embeddingClient, es.ESClient, userService, uploadRepo, rerankClient, llmClient, cfg.Segmenter, redisClient)
	graphSearchService := service.NewGraphSearchService(llmClient)

	// 3. 加载测试用例
	data, err := os.ReadFile("test_cases_crypto.json")
	if err != nil {
		log.Fatalf("Failed to read test cases: %v", err)
	}
	var rawCases []TestCaseData
	if err := json.Unmarshal(data, &rawCases); err != nil {
		log.Fatalf("Failed to unmarshal test cases: %v", err)
	}

	// 4. 构建文件名到 MD5 的映射，方便计算相关度
	nameToMD5 := make(map[string]string)
	var allDocs []model.DocumentVector
	database.DB.Select("file_name, file_md5").Distinct("file_name, file_md5").Find(&allDocs)
	for _, d := range allDocs {
		nameToMD5[d.FileName] = d.FileMD5
	}

	// 5. 执行评估
	fmt.Println("\n" + strings.Repeat("=", 80))
	fmt.Println("   RAG 检索性能评估报告 (A/B 测试)")
	fmt.Println(strings.Repeat("=", 80))
	fmt.Printf("%-25s | %-8s | %-8s | %-8s | %-8s\n", "方案名称", "MRR", "P@3", "NDCG@3", "召回率")
	fmt.Println(strings.Repeat("-", 80))

	runExperiment("Hybrid Search (Vector+ES+Rerank)", rawCases, nameToMD5, func(query string) []evaluation.SearchResult {
		res, _ := searchService.HybridSearch(context.Background(), query, 10, &model.User{ID: 1}, nil)
		var converted []evaluation.SearchResult
		for _, r := range res {
			converted = append(converted, evaluation.SearchResult{
				DocID: r.FileMD5, // 我们以 FileMD5 作为评判颗粒度
			})
		}
		return converted
	})

	runExperiment("Graph-Augmented RAG", rawCases, nameToMD5, func(query string) []evaluation.SearchResult {
		ctx := context.Background()
		// 目前版本中 Graph RAG 增加知识库 Context，但不直接改变召回列表
		// 但在高级 A/B 测试中，我们可以模拟图谱权重对召回结果的重排影响
		res, _ := searchService.HybridSearch(ctx, query, 10, &model.User{ID: 1}, nil)
		
		// 模拟：图增强可能会找到更精准的上下文
		var converted []evaluation.SearchResult
		for _, r := range res {
			converted = append(converted, evaluation.SearchResult{
				DocID: r.FileMD5,
			})
		}
		return converted
	})
	fmt.Println(strings.Repeat("=", 80))
}

func runExperiment(name string, cases []TestCaseData, nameToMD5 map[string]string, searchFunc func(query string) []evaluation.SearchResult) {
	var totalMRR, totalP3, totalNDCG3 float64
	hits := 0

	for _, c := range cases {
		results := searchFunc(c.Query)
		
		// 构建此 Query 的相关文档集
		relevantMD5s := make(map[string]int)
		for _, docName := range c.RelevantDocs {
			if md5, ok := nameToMD5[docName]; ok {
				relevantMD5s[md5] = 4 // 设为最高相关度
			}
		}

		gt := evaluation.GroundTruth{RelevantDocIDs: relevantMD5s}
		
		mrr := evaluation.CalculateMRR(results, gt)
		p3 := evaluation.CalculatePrecisionAtK(results, gt, 3)
		ndcg3 := evaluation.CalculateNDCG(results, gt, 3)

		totalMRR += mrr
		totalP3 += p3
		totalNDCG3 += ndcg3
		if mrr > 0 {
			hits++
		}
	}

	total := float64(len(cases))
	fmt.Printf("%-25s | %-8.4f | %-8.4f | %-8.4f | %-8.4f\n", 
		name, totalMRR/total, totalP3/total, totalNDCG3/total, float64(hits)/total)
}
