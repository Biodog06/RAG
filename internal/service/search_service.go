// Package service 提供了搜索相关的业务逻辑。
package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"pai-smart-go/internal/model"
	"pai-smart-go/internal/repository"
	"pai-smart-go/pkg/embedding"
	"pai-smart-go/pkg/llm"
	"pai-smart-go/pkg/log"
	"pai-smart-go/internal/config"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/elastic/go-elasticsearch/v8"
)

// SearchService 接口定义了搜索操作。
type SearchService interface {
	HybridSearch(ctx context.Context, query string, topK int, user *model.User) ([]model.SearchResponseDTO, error)
}

type searchService struct {
	embeddingClient embedding.Client
	esClient        *elasticsearch.Client
	userService     UserService
	uploadRepo      repository.UploadRepository
	llmClient       llm.Client
}

// NewSearchService 创建一个新的 SearchService 实例。
func NewSearchService(embeddingClient embedding.Client, esClient *elasticsearch.Client, userService UserService, uploadRepo repository.UploadRepository, llmClient llm.Client) SearchService {
	return &searchService{
		embeddingClient: embeddingClient,
		esClient:        esClient,
		userService:     userService,
		uploadRepo:      uploadRepo,
		llmClient:       llmClient,
	}
}

// HybridSearch 执行与 Java 项目逻辑一致的两阶段混合搜索。
func (s *searchService) HybridSearch(ctx context.Context, query string, topK int, user *model.User) ([]model.SearchResponseDTO, error) {
	log.Infof("[SearchService] 开始执行混合搜索, query: '%s', topK: %d, user: %s", query, topK, user.Username)

	// 1. 获取用户有效的组织标签（包含层级关系）
	userEffectiveTags, err := s.userService.GetUserEffectiveOrgTags(user)
	if err != nil {
		log.Errorf("[SearchService] 获取用户有效组织标签失败: %v", err)
		userEffectiveTags = []string{}
	}

	// 获取配置参数
	threshold := config.Conf.Search.ConfidenceThreshold
	if threshold == 0 {
		threshold = 0.95
	}
	timeoutMs := config.Conf.Search.SpeculativeTimeoutMS
	if timeoutMs == 0 {
		timeoutMs = 200
	}
	timeout := time.Duration(timeoutMs) * time.Millisecond

	type searchResult struct {
		results []model.SearchResponseDTO
		err     error
	}

	fastChan := make(chan searchResult, 1)
	slowChan := make(chan searchResult, 1)

	slowCtx, cancelSlow := context.WithCancel(ctx)
	defer cancelSlow()

	startTime := time.Now()

	// Launch Fast Path (Original Query)
	go func() {
		res, err := s.searchInternal(ctx, query, topK, user, userEffectiveTags)
		fastChan <- searchResult{results: res, err: err}
	}()

	// Launch Slow Path (LLM Rewritten Query)
	go func() {
		rewritten, err := s.rewriteQuery(slowCtx, query)
		if err != nil {
			log.Warnf("[SearchService] 查询改写失败: %v, 将使用原查询执行慢路径", err)
			rewritten = query
		} else {
			log.Infof("[SearchService] 查询改写成功: '%s' -> '%s'", query, rewritten)
		}
		res, err := s.searchInternal(slowCtx, rewritten, topK, user, userEffectiveTags)
		slowChan <- searchResult{results: res, err: err}
	}()

	var fastRes *searchResult

	// speculative return logic
	speculativeTimer := time.NewTimer(timeout)
	defer speculativeTimer.Stop()

	for {
		select {
		case res := <-fastChan:
			if res.err != nil {
				log.Errorf("[SearchService] 快路径执行失败: %v", res.err)
				// 继续等待慢路径
				continue
			}
			fastRes = &res
			// 检查是否满足快速返回条件：高置信度且在超时时间内
			if len(res.results) > 0 && res.results[0].Score >= threshold && time.Since(startTime) <= timeout {
				log.Infof("[SearchService] 触发投机返回 (Fast Path), score: %.4f", res.results[0].Score)
				cancelSlow()
				return res.results, nil
			}
			log.Infof("[SearchService] 快路径完成，但不满足投机返回条件 (score: %.4f), 继续等待慢路径", getTopScore(res.results))

		case res := <-slowChan:
			if res.err != nil {
				log.Errorf("[SearchService] 慢路径执行失败: %v", res.err)
				if fastRes != nil {
					return fastRes.results, nil
				}
				return nil, res.err
			}
			log.Infof("[SearchService] 返回慢路径结果")
			return res.results, nil

		case <-speculativeTimer.C:
			log.Infof("[SearchService] 投机返回超时 (duration: %v)", timeout)
			// 超时后，如果快路径已经完成且结果还行（或者慢路径还没出来），我们也可以考虑返回
			// 但根据要求，如果快路径结果不足（即不满足threshold），我们应等待慢路径

		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

func getTopScore(results []model.SearchResponseDTO) float64 {
	if len(results) > 0 {
		return results[0].Score
	}
	return 0
}

// rewriteQuery 使用 LLM 改写用户查询以提高检索质量。
func (s *searchService) rewriteQuery(ctx context.Context, query string) (string, error) {
	prompt := fmt.Sprintf("你是一个 RAG 系统中的查询改写助手。请将以下用户问句改写为更适合文档检索的关键词短语或更清晰的问题。如果是简单问句，可以丰富背景；如果是长句，请提取核心检索意图。返回改写后的文本，不要有任何解释。\n\n用户问句: %s\n\n改写后的查询:", query)
	
	messages := []llm.Message{
		{Role: "user", Content: prompt},
	}
	
	// 设置较短的超时时间，避免慢路径太慢
	rewriteCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	
	rewritten, err := s.llmClient.Chat(rewriteCtx, messages, nil)
	if err != nil {
		return "", err
	}
	
	return strings.TrimSpace(rewritten), nil
}

// searchInternal 执行核心搜索逻辑。
func (s *searchService) searchInternal(ctx context.Context, query string, topK int, user *model.User, userEffectiveTags []string) ([]model.SearchResponseDTO, error) {
	// 2. 轻量归一化（去噪）以获取核心短语
	normalized, phrase := normalizeQuery(query)
	if normalized != query {
		log.Infof("[SearchService] 规范化查询: '%s' -> '%s' (phrase='%s')", query, normalized, phrase)
	}

	// 3. 向量化查询（用原始用户问句，保持语义检索能力）
	queryVector, err := s.embeddingClient.CreateEmbedding(ctx, query)
	if err != nil {
		log.Errorf("[SearchService] 向量化查询失败: %v", err)
		return nil, fmt.Errorf("failed to create query embedding: %w", err)
	}

	// 4. 构建 Elasticsearch 的复杂混合搜索查询
	var buf bytes.Buffer
	esQuery := map[string]interface{}{
		"knn": map[string]interface{}{
			"field":          "vector",
			"query_vector":   queryVector,
			"k":              topK * 30,
			"num_candidates": topK * 30,
		},
		"query": map[string]interface{}{
			"bool": map[string]interface{}{
				"must": map[string]interface{}{
					"match": map[string]interface{}{
						"text_content": normalized,
					},
				},
				"filter": map[string]interface{}{
					"bool": map[string]interface{}{
						"should": []map[string]interface{}{
							{"term": map[string]interface{}{"user_id": user.ID}},
							{"term": map[string]interface{}{"is_public": true}},
							{"terms": map[string]interface{}{"org_tag": userEffectiveTags}},
						},
						"minimum_should_match": 1,
					},
				},
				"should": buildPhraseShould(phrase),
			},
		},
		"rescore": map[string]interface{}{
			"window_size": topK * 30,
			"query": map[string]interface{}{
				"rescore_query": map[string]interface{}{
					"match": map[string]interface{}{
						"text_content": map[string]interface{}{
							"query":    normalized,
							"operator": "and",
						},
					},
				},
				"query_weight":         0.2,
				"rescore_query_weight": 1.0,
			},
		},
		"size": topK,
	}

	if err := json.NewEncoder(&buf).Encode(esQuery); err != nil {
		return nil, fmt.Errorf("failed to encode es query: %w", err)
	}

	// 5. 执行搜索
	res, err := s.esClient.Search(
		s.esClient.Search.WithContext(ctx),
		s.esClient.Search.WithIndex("knowledge_base"),
		s.esClient.Search.WithBody(&buf),
		s.esClient.Search.WithTrackTotalHits(true),
	)
	if err != nil {
		return nil, fmt.Errorf("elasticsearch search failed: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		bodyBytes, _ := io.ReadAll(res.Body)
		return nil, fmt.Errorf("elasticsearch returned an error: %s, body: %s", res.Status(), string(bodyBytes))
	}

	// 6. 解析结果
	var esResponse struct {
		Hits struct {
			Hits []struct {
				Source model.EsDocument `json:"_source"`
				Score  float64          `json:"_score"`
			} `json:"hits"`
		} `json:"hits"`
	}

	if err := json.NewDecoder(res.Body).Decode(&esResponse); err != nil {
		return nil, fmt.Errorf("failed to decode es response: %w", err)
	}

	if len(esResponse.Hits.Hits) == 0 {
		if phrase != "" && phrase != query {
			// 重试逻辑...
			var retryBuf bytes.Buffer
			retryQuery := esQuery
			((retryQuery["query"].(map[string]interface{}))["bool"].(map[string]interface{}))["must"] = map[string]interface{}{
				"match": map[string]interface{}{"text_content": phrase},
			}
			((retryQuery["rescore"].(map[string]interface{}))["query"].(map[string]interface{}))["rescore_query"] = map[string]interface{}{
				"match": map[string]interface{}{"text_content": map[string]interface{}{"query": phrase, "operator": "and"}},
			}
			if err := json.NewEncoder(&retryBuf).Encode(retryQuery); err == nil {
				res2, err2 := s.esClient.Search(
					s.esClient.Search.WithContext(ctx),
					s.esClient.Search.WithIndex("knowledge_base"),
					s.esClient.Search.WithBody(&retryBuf),
				)
				if err2 == nil && !res2.IsError() {
					defer res2.Body.Close()
					json.NewDecoder(res2.Body).Decode(&esResponse)
				}
			}
		}
		if len(esResponse.Hits.Hits) == 0 {
			return []model.SearchResponseDTO{}, nil
		}
	}

	// 7. 批量获取文件名
	fileMD5s := make([]string, 0, len(esResponse.Hits.Hits))
	for _, hit := range esResponse.Hits.Hits {
		fileMD5s = append(fileMD5s, hit.Source.FileMD5)
	}
	uniqueMD5s := make(map[string]struct{})
	for _, md5 := range fileMD5s {
		uniqueMD5s[md5] = struct{}{}
	}
	md5List := make([]string, 0, len(uniqueMD5s))
	for md5 := range uniqueMD5s {
		md5List = append(md5List, md5)
	}

	fileInfos, err := s.uploadRepo.FindBatchByMD5s(md5List)
	if err != nil {
		return nil, fmt.Errorf("批量查询文件信息失败: %w", err)
	}

	fileNameMap := make(map[string]string)
	for _, info := range fileInfos {
		fileNameMap[info.FileMD5] = info.FileName
	}

	// 8. 组装最终结果
	var results []model.SearchResponseDTO
	for _, hit := range esResponse.Hits.Hits {
		fileName := fileNameMap[hit.Source.FileMD5]
		if fileName == "" {
			fileName = "未知文件"
		}
		dto := model.SearchResponseDTO{
			FileMD5:     hit.Source.FileMD5,
			FileName:    fileName,
			ChunkID:     hit.Source.ChunkID,
			TextContent: hit.Source.TextContent,
			Score:       hit.Score,
			UserID:      strconv.FormatUint(uint64(hit.Source.UserID), 10),
			OrgTag:      hit.Source.OrgTag,
			IsPublic:    hit.Source.IsPublic,
		}
		results = append(results, dto)
	}

	return results, nil
}

// normalizeQuery 对用户查询进行轻量去噪与短语提取。
// 返回值：规范化后的查询（用于 BM25/rescore）与核心短语（用于 match_phrase 兜底）。
func normalizeQuery(q string) (string, string) {
	if q == "" {
		return q, ""
	}
	lower := strings.ToLower(q)
	// 去除常见口语/功能词
	stopPhrases := []string{"什么是", "是谁", "是什么", "是啥", "请问", "怎么", "如何", "告诉我", "严格", "按照", "不要补充", "的区别", "区别", "吗", "呢", "？", "?"}
	for _, sp := range stopPhrases {
		lower = strings.ReplaceAll(lower, sp, " ")
	}
	// 仅保留中文、英文、数字与空白
	reKeep := regexp.MustCompile(`[^\p{Han}a-z0-9\s]+`)
	kept := reKeep.ReplaceAllString(lower, " ")
	// 归一空白
	reSpace := regexp.MustCompile(`\s+`)
	kept = strings.TrimSpace(reSpace.ReplaceAllString(kept, " "))
	if kept == "" {
		return q, ""
	}
	return kept, kept
}

// buildPhraseShould 构建 match_phrase should 子句（带 boost），为空则返回 nil
func buildPhraseShould(phrase string) interface{} {
	if phrase == "" {
		return nil
	}
	return []map[string]interface{}{
		{
			"match_phrase": map[string]interface{}{
				"text_content": map[string]interface{}{
					"query": phrase,
					"boost": 3.0,
				},
			},
		},
	}
}
