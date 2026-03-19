// Package service 提供了搜索相关的业务逻辑。
package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"pai-smart-go/internal/config"
	"pai-smart-go/internal/model"
	"pai-smart-go/internal/repository"
	"pai-smart-go/pkg/embedding"
	"pai-smart-go/pkg/llm"
	"pai-smart-go/pkg/log"
	"pai-smart-go/pkg/rerank"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/elastic/go-elasticsearch/v8"
	"github.com/go-redis/redis/v8"
)

// SearchService 接口定义了搜索操作。
type SearchService interface {
	HybridSearch(ctx context.Context, query string, topK int, user *model.User, history []model.ChatMessage) ([]model.SearchResponseDTO, error)
}

type searchService struct {
	embeddingClient embedding.Client
	esClient        *elasticsearch.Client
	userService     UserService
	uploadRepo      repository.UploadRepository
	rerankClient    rerank.Client
	llmClient       llm.Client
	segmenter       *QuerySegmenter
	redisClient     *redis.Client
}

// NewSearchService 创建一个新的 SearchService 实例。
func NewSearchService(
	embeddingClient embedding.Client,
	esClient *elasticsearch.Client,
	userService UserService,
	uploadRepo repository.UploadRepository,
	rerankClient rerank.Client, // 新增参数
	llmClient llm.Client, // 新增参数
	segmenterConfig config.SegmenterConfig, // 新增参数
	redisClient *redis.Client, // 新增参数：缓存客户端
) SearchService {
	// 初始化分词器
	segmenter := GetSegmenter(SegmenterConfig{
		Enabled: segmenterConfig.Enabled,
		Dict:    segmenterConfig.Dict,
	})



	return &searchService{
		embeddingClient: embeddingClient,
		esClient:        esClient,
		userService:     userService,
		uploadRepo:      uploadRepo,
		rerankClient:    rerankClient, // 初始化
		llmClient:       llmClient,    // 初始化
		segmenter:       segmenter,    // 初始化
		redisClient:     redisClient,  // 初始化
	}
}

// HybridSearch 执行并行检索与投机返回逻辑（针对 Race Mode 优化）。
func (s *searchService) HybridSearch(ctx context.Context, query string, topK int, user *model.User, history []model.ChatMessage) ([]model.SearchResponseDTO, error) {
	searchStart := time.Now()
	log.Infof("[SearchService] 开始执行并行检索, query: '%s', topK: %d, user: %s", query, topK, user.Username)

	// 慢查询日志：超过 2 秒的查询将被记录为警告
	defer func() {
		duration := time.Since(searchStart)
		if duration > 2*time.Second {
			log.Warnf("[SlowQuery] 查询耗时 %.2fs, Query: '%s', TopK: %d, User: %s",
				duration.Seconds(), query, topK, user.Username)
		}
		log.Infof("[SearchService] 搜索总耗时: %.2fs", duration.Seconds())
	}()

	// 1. 获取用户有效的组织标签
	userEffectiveTags, err := s.userService.GetUserEffectiveOrgTags(user)
	if err != nil {
		log.Errorf("[SearchService] 获取组织标签失败: %v", err)
		userEffectiveTags = []string{}
	}

	// 获取并行控制配置
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

	// Launch Fast Path (Original Query, Fast Search)
	go func() {
		res, err := s.searchInternal(ctx, query, topK, user, userEffectiveTags, history, true)
		fastChan <- searchResult{results: res, err: err}
	}()

	// Launch Slow Path (LLM Rewritten Query, Full Optimized Search)
	go func() {
		rewritten, err := s.rewriteQuery(slowCtx, query)
		if err != nil {
			log.Warnf("[SearchService] 慢路径查询改写失败: %v", err)
			rewritten = query
		}
		res, err := s.searchInternal(slowCtx, rewritten, topK, user, userEffectiveTags, history, false)
		slowChan <- searchResult{results: res, err: err}
	}()

	var fastRes *searchResult
	speculativeTimer := time.NewTimer(timeout)
	defer speculativeTimer.Stop()

	for {
		select {
		case res := <-fastChan:
			if res.err != nil {
				log.Errorf("[SearchService] 快路径失败: %v", res.err)
				continue
			}
			fastRes = &res
			// 满足投机返回条件
			if len(res.results) > 0 && res.results[0].Score >= threshold && time.Since(startTime) <= timeout {
				log.Infof("[SearchService] 触发投机返回 (Fast Path), score: %.4f", res.results[0].Score)
				cancelSlow()
				return res.results, nil
			}
			log.Infof("[SearchService] 快路径完成，但不满足投机条件，等待慢路径中...")

		case res := <-slowChan:
			if res.err != nil {
				log.Errorf("[SearchService] 慢路径失败: %v", res.err)
				if fastRes != nil {
					return fastRes.results, nil
				}
				return nil, res.err
			}
			log.Infof("[SearchService] 返回慢路径检索结果")
			return res.results, nil

		case <-speculativeTimer.C:
			log.Infof("[SearchService] 投机返回超时 (duration: %v)", timeout)

		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

// rewriteQuery 使用 LLM 改写用户查询以提高检索质量。
func (s *searchService) rewriteQuery(ctx context.Context, query string) (string, error) {
	prompt := fmt.Sprintf("你是一个 RAG 系统中的查询改写助手。请将以下用户问句改写为更适合文档检索的关键词短语或更清晰的问题。如果是简单问句，可以丰富背景；如果是长句，请提取核心检索意图。返回改写后的文本，不要有任何解释。\n\n用户问句: %s\n\n改写后的查询:", query)
	messages := []llm.Message{{Role: "user", Content: prompt}}

	rewriteCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	rewritten, err := s.llmClient.Chat(rewriteCtx, messages, nil)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(rewritten), nil
}

// searchInternal 封装了多路召回、RRF 融合与重排序的完整逻辑（适配并行/非并行）。
func (s *searchService) searchInternal(ctx context.Context, query string, topK int, user *model.User, userEffectiveTags []string, history []model.ChatMessage, isFastPath bool) ([]model.SearchResponseDTO, error) {
	// 1. 关键词提取与缓存查询 (Result Cache)
	var keywords *KeywordExtractionResult
	var err error
	if isFastPath {
		keywords = s.fallbackKeywordExtraction(query)
	} else {
		keywords, err = s.extractKeywords(ctx, query)
		if err != nil {
			keywords = s.fallbackKeywordExtraction(query)
		}
	}

	// 1.5 检查关键词缓存
	if s.redisClient != nil {
		cacheKey := generateCacheKey(keywords)
		if cacheKey != "" {
			cachedDocIDs, err := s.redisClient.Get(ctx, cacheKey).Result()
			if err == nil && cachedDocIDs != "" {
				docIDs := strings.Split(cachedDocIDs, ",")
				if results, err := s.batchGetDocsByIDs(ctx, docIDs, user, userEffectiveTags); err == nil {
					log.Infof("[SearchInternal] 缓存命中, fast=%v", isFastPath)
					return results, nil
				}
			}
		}
	}

	// 2. 查询转换（Slow Path 专用）
	allQueries := []string{query}
	if !isFastPath {
		rewritten, err := s.rewriteQueries(ctx, query, history)
		if err == nil && len(rewritten) > 0 {
			allQueries = append(allQueries, rewritten...)
		}
	}

	// 3. 并行向量化所有查询变体
	type qvResult struct {
		vec []float32
		err error
	}
	vecChan := make(chan qvResult, len(allQueries))
	var wg sync.WaitGroup
	for _, qText := range allQueries {
		wg.Add(1)
		go func(q string) {
			defer wg.Done()
			vec, err := s.embeddingClient.CreateEmbedding(ctx, q)
			vecChan <- qvResult{vec, err}
		}(qText)
	}
	wg.Wait()
	close(vecChan)

	var queryVectors [][]float32
	for res := range vecChan {
		if res.err == nil {
			queryVectors = append(queryVectors, res.vec)
		}
	}
	if len(queryVectors) == 0 {
		return nil, fmt.Errorf("all vectorization failed")
	}

	// 4. 并行执行多路召回 (Dense + Sparse)
	const (
		rrfK       = 60.0
		recallSize = 60
	)
	filterConditions := []map[string]interface{}{
		{"term": map[string]interface{}{"user_id": user.ID}},
		{"term": map[string]interface{}{"is_public": true}},
		{"terms": map[string]interface{}{"org_tag": userEffectiveTags}},
	}
	boolFilter := map[string]interface{}{
		"bool": map[string]interface{}{
			"should":               filterConditions,
			"minimum_should_match": 1,
		},
	}

	var recallWg sync.WaitGroup
	var denseMu sync.Mutex
	var allDenseDocs []model.EsDocument
	var sparseDocs []model.EsDocument

	// 4.1 Dense (Vector) Retrieval
	for _, qv := range queryVectors {
		recallWg.Add(1)
		go func(vector []float32) {
			defer recallWg.Done()
			queryBody := map[string]interface{}{
				"knn": map[string]interface{}{
					"field": "vector", "query_vector": vector, "k": recallSize, "num_candidates": 100, "filter": boolFilter,
				},
				"size": recallSize,
			}
			docs, _ := s.executeEsQuery(ctx, queryBody)
			denseMu.Lock()
			allDenseDocs = append(allDenseDocs, docs...)
			denseMu.Unlock()
		}(qv)
	}

	// 4.2 Sparse (Keyword) Retrieval
	recallWg.Add(1)
	go func() {
		defer recallWg.Done()
		boolQuery := map[string]interface{}{"filter": boolFilter}
		var shouldClauses []map[string]interface{}
		for _, kw := range keywords.CoreKeywords {
			shouldClauses = append(shouldClauses, map[string]interface{}{"match": map[string]interface{}{"text_content": map[string]interface{}{"query": kw, "boost": 2.0}}})
		}
		for _, kw := range keywords.OptionalKeywords {
			shouldClauses = append(shouldClauses, map[string]interface{}{"match": map[string]interface{}{"text_content": map[string]interface{}{"query": kw, "boost": 1.0}}})
		}
		if len(shouldClauses) == 0 {
			normalized, _ := normalizeQuery(query)
			shouldClauses = append(shouldClauses, map[string]interface{}{"match": map[string]interface{}{"text_content": normalized}})
		}
		boolQuery["should"] = shouldClauses

		if len(keywords.CoreKeywords) > 0 {
			boolQuery["minimum_should_match"] = "100%"
		} else {
			boolQuery["minimum_should_match"] = 1
		}

		queryBody := map[string]interface{}{"query": map[string]interface{}{"bool": boolQuery}, "size": recallSize}
		sparseDocs, _ = s.executeEsQuery(ctx, queryBody)
	}()
	recallWg.Wait()

	// 5. RRF Fusion
	denseDocs := deduplicateDocs(allDenseDocs)
	var resultLists [][]interface{}
	if len(denseDocs) > 0 {
		list := make([]interface{}, len(denseDocs))
		for i, v := range denseDocs {
			list[i] = v
		}
		resultLists = append(resultLists, list)
	}
	if len(sparseDocs) > 0 {
		list := make([]interface{}, len(sparseDocs))
		for i, v := range sparseDocs {
			list[i] = v
		}
		resultLists = append(resultLists, list)
	}
	if len(resultLists) == 0 {
		return []model.SearchResponseDTO{}, nil
	}

	fusionResults := ReciprocalRankFusion(resultLists, func(doc interface{}) string { return doc.(model.EsDocument).VectorID }, rrfK)

	// 6. Rerank (仅 Slow Path 执行)
	rerankSize := 50
	if len(fusionResults) > rerankSize {
		fusionResults = fusionResults[:rerankSize]
	}

	type ScoredDoc struct {
		Doc   model.EsDocument
		Score float64
	}
	var rerankCandidates []ScoredDoc
	var candidateTexts []string
	for _, res := range fusionResults {
		doc := res.Doc.(model.EsDocument)
		rerankCandidates = append(rerankCandidates, ScoredDoc{doc, res.Score})
		candidateTexts = append(candidateTexts, doc.TextContent)
	}

	var finalDocs []ScoredDoc
	if !isFastPath && len(candidateTexts) > 0 && s.rerankClient != nil {
		rerankResults, err := s.rerankClient.Rerank(ctx, query, candidateTexts)
		if err == nil {
			for _, res := range rerankResults {
				if res.Index < len(rerankCandidates) {
					original := rerankCandidates[res.Index]
					finalDocs = append(finalDocs, ScoredDoc{original.Doc, res.RelevanceScore})
				}
			}
		} else {
			finalDocs = rerankCandidates
		}
	} else {
		finalDocs = rerankCandidates
	}

	// 7. 组装结果与文件名回填
	if len(finalDocs) > topK {
		finalDocs = finalDocs[:topK]
	}

	md5Set := make(map[string]struct{})
	for _, doc := range finalDocs {
		md5Set[doc.Doc.FileMD5] = struct{}{}
	}
	var md5List []string
	for md5 := range md5Set {
		md5List = append(md5List, md5)
	}

	fileInfos, err := s.uploadRepo.FindBatchByMD5s(md5List)
	fileNameMap := make(map[string]string)
	if err != nil {
		log.Errorf("[SearchService] 批量查询文件信息失败: %v", err)
	} else {
		for _, info := range fileInfos {
			fileNameMap[info.FileMD5] = info.FileName
		}
	}

	var results []model.SearchResponseDTO
	for _, doc := range finalDocs {
		fileName := doc.Doc.FileName
		// 如果 ES 中没存（旧数据），则尝试从刚才查询的 Map 中获取
		if fileName == "" {
			fileName = fileNameMap[doc.Doc.FileMD5]
		}
		if fileName == "" {
			fileName = "未知文件"
		}
		dto := model.SearchResponseDTO{
			FileMD5:     doc.Doc.FileMD5,
			FileName:    fileName,
			ChunkID:     doc.Doc.ChunkID,
			TextContent: doc.Doc.TextContent,
			Score:       doc.Score,
			UserID:      strconv.FormatUint(uint64(doc.Doc.UserID), 10),
			OrgTag:      doc.Doc.OrgTag,
			IsPublic:    doc.Doc.IsPublic,
		}
		results = append(results, dto)
	}

	// 8. 异步写入缓存 (仅在未命中时)
	if !isFastPath && s.redisClient != nil && len(finalDocs) > 0 {
		cacheKey := generateCacheKey(keywords)
		if cacheKey != "" {
			// 提取 VectorIDs
			var docIDs []string
			for _, doc := range finalDocs {
				docIDs = append(docIDs, doc.Doc.VectorID)
			}
			cachedValue := strings.Join(docIDs, ",")

			// 异步写入（使用动态 TTL）
			cacheTTL := s.getDynamicCacheTTL(query)
			go func(key, value string, ttl time.Duration) {
				// 使用 Background context 避免父 context 取消
				cacheCtx := context.Background()
				if err := s.redisClient.Set(cacheCtx, key, value, ttl).Err(); err != nil {
					log.Warnf("[SearchService] 缓存写入失败: %v", err)
				} else {
					log.Infof("[SearchService] 成功写入缓存, key: %s, docCount: %d, ttl: %v", key, len(docIDs), ttl)
				}
			}(cacheKey, cachedValue, cacheTTL)
		}
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

// KeywordExtractionResult 关键词提取结果
type KeywordExtractionResult struct {
	CoreKeywords     []string // 核心关键词（必须匹配）
	OptionalKeywords []string // 可选关键词（提升相关性）
}

// extractKeywords 使用 LLM 提取查询中的核心关键词和可选关键词
func (s *searchService) extractKeywords(ctx context.Context, query string) (*KeywordExtractionResult, error) {
	// 如果 LLM 客户端未配置，降级为简单分词
	if s.llmClient == nil {
		log.Warnf("[SearchService] LLM 客户端未配置，使用简单分词策略")
		return s.fallbackKeywordExtraction(query), nil
	}

	prompt := fmt.Sprintf(`你是一个关键词提取专家。请从用户的问题中提取关键词，并分为两类：

1. **核心关键词**：问题的主体，必须出现在搜索结果中（如产品名、技术名词、专有名词）
2. **可选关键词**：辅助词汇，有助于提升相关性但不是必须的（如动词、形容词）

用户问题："%s"

请严格按照以下 JSON 格式返回，不要包含任何其他文字：
{
  "core": ["关键词1", "关键词2"],
  "optional": ["关键词3", "关键词4"]
}`, query)

	messages := []llm.Message{
		{Role: "user", Content: prompt},
	}

	response, err := s.llmClient.GenerateOneShot(ctx, messages)
	if err != nil {
		log.Warnf("[SearchService] LLM 关键词提取失败，降级为简单分词: %v", err)
		return s.fallbackKeywordExtraction(query), nil
	}

	// 解析 JSON 响应
	var result struct {
		Core     []string `json:"core"`
		Optional []string `json:"optional"`
	}

	// 尝试提取 JSON（LLM 可能返回带有额外文本的响应）
	jsonStart := strings.Index(response, "{")
	jsonEnd := strings.LastIndex(response, "}")
	if jsonStart == -1 || jsonEnd == -1 {
		log.Warnf("[SearchService] LLM 返回格式不正确，降级为简单分词")
		return s.fallbackKeywordExtraction(query), nil
	}

	jsonStr := response[jsonStart : jsonEnd+1]
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		log.Warnf("[SearchService] 解析 LLM 返回的 JSON 失败，降级为简单分词: %v", err)
		return s.fallbackKeywordExtraction(query), nil
	}

	log.Infof("[SearchService] 关键词提取成功 - 核心: %v, 可选: %v", result.Core, result.Optional)
	return &KeywordExtractionResult{
		CoreKeywords:     result.Core,
		OptionalKeywords: result.Optional,
	}, nil
}

// fallbackKeywordExtraction 三层降级策略
// 第二层：尝试使用分词器
// 第三层：简单分词
func (s *searchService) fallbackKeywordExtraction(query string) *KeywordExtractionResult {
	// 第二层：尝试使用分词器（如果启用）
	if s.segmenter != nil && s.segmenter.enabled {
		core, optional := s.segmenter.SegmentWithPOSAdvanced(query)
		if len(core) > 0 {
			log.Infof("[SearchService] 使用分词器提取关键词 - 核心: %v, 可选: %v", core, optional)
			return &KeywordExtractionResult{
				CoreKeywords:     core,
				OptionalKeywords: optional,
			}
		}
		log.Warnf("[SearchService] 分词器未提取到关键词，降级到简单分词")
	}

	// 第三层：简单分词策略（兜底）
	log.Infof("[SearchService] 使用简单分词策略（第三层兜底）")
	words := strings.Fields(query)
	if len(words) == 0 {
		return &KeywordExtractionResult{
			CoreKeywords:     []string{query},
			OptionalKeywords: []string{},
		}
	}

	// 简单规则：如果词数 <= 2，全部作为核心词；否则前 60% 作为核心词
	coreCount := len(words)
	if len(words) > 2 {
		coreCount = (len(words) * 6) / 10
		if coreCount == 0 {
			coreCount = 1
		}
	}

	return &KeywordExtractionResult{
		CoreKeywords:     words[:coreCount],
		OptionalKeywords: words[coreCount:],
	}
}

// executeEsQuery 执行 ES 查询并解析结果
func (s *searchService) executeEsQuery(ctx context.Context, queryBody map[string]interface{}) ([]model.EsDocument, error) {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(queryBody); err != nil {
		return nil, fmt.Errorf("failed to encode query: %w", err)
	}

	// 优化：禁用 track_total_hits 以减少查询开销（我们不需要总数）
	res, err := s.esClient.Search(
		s.esClient.Search.WithContext(ctx),
		s.esClient.Search.WithIndex("knowledge_base"),
		s.esClient.Search.WithBody(&buf),
		s.esClient.Search.WithTrackTotalHits(false),
	)
	if err != nil {
		return nil, fmt.Errorf("es search failed: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		bodyBytes, _ := io.ReadAll(res.Body)
		return nil, fmt.Errorf("es returned error: %s, body: %s", res.Status(), string(bodyBytes))
	}

	var esResponse struct {
		Hits struct {
			Hits []struct {
				Source model.EsDocument `json:"_source"`
				Score  float64          `json:"_score"`
			} `json:"hits"`
		} `json:"hits"`
	}

	if err := json.NewDecoder(res.Body).Decode(&esResponse); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	docs := make([]model.EsDocument, 0, len(esResponse.Hits.Hits))
	for _, hit := range esResponse.Hits.Hits {
		// 保留原始分数以便调试（可选）
		docs = append(docs, hit.Source)
	}

	return docs, nil
}

// batchGetDocsByIDs 根据文档 ID 列表批量查询文档（用于缓存命中场景）
func (s *searchService) batchGetDocsByIDs(ctx context.Context, docIDs []string, user *model.User, userEffectiveTags []string) ([]model.SearchResponseDTO, error) {
	if len(docIDs) == 0 {
		return []model.SearchResponseDTO{}, nil
	}

	// 构建 ES 查询：根据 vector_id 批量获取
	idsQuery := map[string]interface{}{
		"query": map[string]interface{}{
			"bool": map[string]interface{}{
				"must": map[string]interface{}{
					"ids": map[string]interface{}{
						"values": docIDs,
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
			},
		},
		"_source": []string{"file_md5", "file_name", "chunk_id", "text_content", "user_id", "org_tag", "is_public", "vector_id"},
		"size":    len(docIDs),
	}

	docs, err := s.executeEsQuery(ctx, idsQuery)
	if err != nil {
		return nil, fmt.Errorf("batch query by IDs failed: %w", err)
	}

	// 组装结果（需要查文件名）
	fileMD5s := make([]string, 0, len(docs))
	for _, doc := range docs {
		fileMD5s = append(fileMD5s, doc.FileMD5)
	}
	uniqueMD5s := make(map[string]struct{})
	for _, md5 := range fileMD5s {
		uniqueMD5s[md5] = struct{}{}
	}
	md5List := make([]string, 0, len(uniqueMD5s))
	for md5 := range uniqueMD5s {
		md5List = append(md5List, md5)
	}

	fileInfos, _ := s.uploadRepo.FindBatchByMD5s(md5List)
	fileNameMap := make(map[string]string)
	for _, info := range fileInfos {
		fileNameMap[info.FileMD5] = info.FileName
	}

	// 转换为 DTO
	var results []model.SearchResponseDTO
	for _, doc := range docs {
		fileName := doc.FileName
		if fileName == "" {
			fileName = fileNameMap[doc.FileMD5]
		}
		if fileName == "" {
			fileName = "未知文件"
		}
		dto := model.SearchResponseDTO{
			FileMD5:     doc.FileMD5,
			FileName:    fileName,
			ChunkID:     doc.ChunkID,
			TextContent: doc.TextContent,
			Score:       1.0, // 缓存命中时用固定分数
			UserID:      strconv.FormatUint(uint64(doc.UserID), 10),
			OrgTag:      doc.OrgTag,
			IsPublic:    doc.IsPublic,
		}
		results = append(results, dto)
	}

	return results, nil
}

// normalizeAndSort 规范化并排序关键词，用于生成缓存指纹
func normalizeAndSort(keywords []string) []string {
	if len(keywords) == 0 {
		return []string{}
	}

	// 去重 + 转小写
	uniqueKeys := make(map[string]bool)
	for _, kw := range keywords {
		normalized := strings.ToLower(strings.TrimSpace(kw))
		if normalized != "" {
			uniqueKeys[normalized] = true
		}
	}

	// 转为 slice 并排序
	result := make([]string, 0, len(uniqueKeys))
	for kw := range uniqueKeys {
		result = append(result, kw)
	}
	sort.Strings(result)

	return result
}

// generateCacheKey 基于核心关键词生成缓存键
func generateCacheKey(keywords *KeywordExtractionResult) string {
	if keywords == nil || len(keywords.CoreKeywords) == 0 {
		return ""
	}

	// 规范化并排序核心关键词
	normalizedKeys := normalizeAndSort(keywords.CoreKeywords)
	if len(normalizedKeys) == 0 {
		return ""
	}

	// 生成指纹：前缀 + 关键词序列（用 | 分隔）
	return "search_cache:" + strings.Join(normalizedKeys, "|")
}

// rewriteQueries 使用 LLM 处理查询转换：重写（针对口语/模糊/指代消解）+ 扩写（生成多路变体）。
func (s *searchService) rewriteQueries(ctx context.Context, query string, history []model.ChatMessage) ([]string, error) {
	if s.llmClient == nil {
		return nil, nil
	}

	// 准备对话历史上下文
	historyContext := "无"
	if len(history) > 0 {
		var sb strings.Builder
		// 仅取最近 3 轮
		start := 0
		if len(history) > 6 {
			start = len(history) - 6
		}
		for i := start; i < len(history); i++ {
			sb.WriteString(fmt.Sprintf("%s: %s\n", history[i].Role, history[i].Content))
		}
		historyContext = sb.String()
	}

	prompt := fmt.Sprintf(`你是一个搜索优化专家。请根据以下用户问题和对话历史，生成 3 个最适合搜索引擎检索的改写变体。

任务要求：
1. **指代消解**：如果问题中包含“他”、“它”、“那个”等代词，请根据历史上下文将其替换为具体的名词。
2. **Query 重写**：将口语化、模糊化的输入（如“理赔咋整”）改写成专业、书面的形式（如“理赔流程和申请规则”）。
3. **Query 扩写**：生成 3 个语义相同但用词不同的检索变体（例如包含同义词、行业术语）。

---
【对话历史】
%s

【用户原始问题】
"%s"
---

请严格按照以下 JSON 格式返回，不要包含任何其他说明文字：
{"queries": ["改写变体1", "改写变体2", "改写变体3"]}`, historyContext, query)

	messages := []llm.Message{
		{Role: "user", Content: prompt},
	}

	response, err := s.llmClient.GenerateOneShot(ctx, messages)
	if err != nil {
		return nil, fmt.Errorf("LLM query rewrite failed: %w", err)
	}

	// 解析 JSON 响应
	var result struct {
		Queries []string `json:"queries"`
	}

	jsonStart := strings.Index(response, "{")
	jsonEnd := strings.LastIndex(response, "}")
	if jsonStart == -1 || jsonEnd == -1 {
		return nil, fmt.Errorf("invalid LLM response format for query rewrite")
	}

	jsonStr := response[jsonStart : jsonEnd+1]
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		return nil, fmt.Errorf("failed to parse query rewrite JSON: %w", err)
	}

	// 过滤空查询
	var rewritten []string
	for _, q := range result.Queries {
		q = strings.TrimSpace(q)
		if q != "" {
			rewritten = append(rewritten, q)
		}
	}

	return rewritten, nil
}

// getDynamicCacheTTL 根据查询频率动态调整缓存 TTL。
// 优化：高频查询缓存时间更长，低频查询及时释放，平衡内存与检索效率。
func (s *searchService) getDynamicCacheTTL(query string) time.Duration {
	freq := s.getQueryFrequency(query)
	switch {
	case freq > 100: // 高频查询
		return 7 * 24 * time.Hour
	case freq > 10: // 中频查询
		return 24 * time.Hour
	default: // 低频查询
		return 1 * time.Hour
	}
}

// getQueryFrequency 从 Redis 获取查询频率计数。
func (s *searchService) getQueryFrequency(query string) int64 {
	if s.redisClient == nil {
		return 0
	}
	// 记录并返回频率
	key := "query_freq:" + query
	freq, _ := s.redisClient.Incr(context.Background(), key).Result()
	// 设置 7 天过期，滑动窗口统计
	_ = s.redisClient.Expire(context.Background(), key, 7*24*time.Hour).Err()
	return freq
}

// deduplicateDocs 根据 VectorID 对文档进行去重。
func deduplicateDocs(docs []model.EsDocument) []model.EsDocument {
	seen := make(map[string]bool)
	var unique []model.EsDocument
	for _, doc := range docs {
		if !seen[doc.VectorID] {
			seen[doc.VectorID] = true
			unique = append(unique, doc)
		}
	}
	return unique
}

func getTopScore(results []model.SearchResponseDTO) float64 {
	if len(results) > 0 {
		return results[0].Score
	}
	return 0
}
