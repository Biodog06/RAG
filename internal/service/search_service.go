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
	"pai-smart-go/pkg/llm" // 新增引入
	"pai-smart-go/pkg/log"
	"pai-smart-go/pkg/rerank" // 新增引入
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/elastic/go-elasticsearch/v8"
	"github.com/go-redis/redis/v8"
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
	rerankClient    rerank.Client   // 新增
	llmClient       llm.Client      // 新增：用于关键词提取
	segmenter       *QuerySegmenter // 新增：分词器
	redisClient     *redis.Client   // 新增：缓存客户端
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

// HybridSearch 执行与 Java 项目逻辑一致的两阶段混合搜索。
func (s *searchService) HybridSearch(ctx context.Context, query string, topK int, user *model.User) ([]model.SearchResponseDTO, error) {
	log.Infof("[SearchService] 开始执行混合搜索, query: '%s', topK: %d, user: %s", query, topK, user.Username)

	// 1. 获取用户有效的组织标签（包含层级关系）
	log.Info("[SearchService] 步骤1: 获取用户有效组织标签")
	userEffectiveTags, err := s.userService.GetUserEffectiveOrgTags(user)
	if err != nil {
		log.Errorf("[SearchService] 获取用户有效组织标签失败: %v", err)
		// 即使失败也继续，只是组织标签过滤会失效
		userEffectiveTags = []string{}
	}
	log.Infof("[SearchService] 获取到 %d 个有效组织标签: %v", len(userEffectiveTags), userEffectiveTags)

	// 2. 使用 LLM 提取关键词（核心词 + 可选词）
	log.Info("[SearchService] 步骤2: 开始提取查询关键词")
	keywords, err := s.extractKeywords(ctx, query)
	if err != nil {
		log.Warnf("[SearchService] 关键词提取失败，使用原始查询: %v", err)
		// 降级：使用原始归一化逻辑
		normalized, phrase := normalizeQuery(query)
		keywords = &KeywordExtractionResult{
			CoreKeywords:     []string{normalized},
			OptionalKeywords: []string{},
		}
		if phrase != "" && phrase != normalized {
			keywords.OptionalKeywords = []string{phrase}
		}
	}
	log.Infof("[SearchService] 关键词提取完成 - 核心词: %v, 可选词: %v",
		keywords.CoreKeywords, keywords.OptionalKeywords)

	// 2.5. 检查关键词缓存 (Result Cache)
	if s.redisClient != nil {
		cacheKey := generateCacheKey(keywords)
		if cacheKey != "" {
			log.Infof("[SearchService] 尝试查询缓存, key: %s", cacheKey)
			cachedDocIDs, err := s.redisClient.Get(ctx, cacheKey).Result()
			if err == nil && cachedDocIDs != "" {
				// 缓存命中！解析 DocIDs 并批量查询
				log.Infof("[SearchService] 缓存命中! 跳过 ES 检索与 Rerank")
				docIDs := strings.Split(cachedDocIDs, ",")
				if results, err := s.batchGetDocsByIDs(ctx, docIDs, user, userEffectiveTags); err == nil {
					log.Infof("[SearchService] 从缓存快速路径返回 %d 条结果", len(results))
					return results, nil
				} else {
					log.Warnf("[SearchService] 批量查询文档失败，降级到完整检索: %v", err)
				}
			}
		}
	}

	// 3. 查询重写（Multi-Query）：生成多个变体查询以提升召回率
	log.Info("[SearchService] 步骤3: 查询重写 (Multi-Query)")
	allQueries := []string{query} // 始终包含原始查询
	rewrittenQueries, err := s.rewriteQueries(ctx, query)
	if err != nil {
		log.Warnf("[SearchService] 查询重写失败，仅使用原始查询: %v", err)
	} else if len(rewrittenQueries) > 0 {
		allQueries = append(allQueries, rewrittenQueries...)
		log.Infof("[SearchService] 查询重写完成，共 %d 个查询变体: %v", len(rewrittenQueries), rewrittenQueries)
	}

	// 4. 向量化所有查询（并行）
	log.Info("[SearchService] 步骤4: 开始向量化所有查询")
	type queryVectorResult struct {
		Query  string
		Vector []float32
		Err    error
	}
	vectorResults := make([]queryVectorResult, len(allQueries))
	var vecWg sync.WaitGroup
	for i, q := range allQueries {
		vecWg.Add(1)
		go func(idx int, qText string) {
			defer vecWg.Done()
			vec, err := s.embeddingClient.CreateEmbedding(ctx, qText)
			vectorResults[idx] = queryVectorResult{Query: qText, Vector: vec, Err: err}
		}(i, q)
	}
	vecWg.Wait()

	// 收集成功的向量
	var queryVectors [][]float32
	for _, vr := range vectorResults {
		if vr.Err != nil {
			log.Warnf("[SearchService] 查询 '%s' 向量化失败: %v", vr.Query, vr.Err)
			continue
		}
		queryVectors = append(queryVectors, vr.Vector)
	}
	if len(queryVectors) == 0 {
		log.Errorf("[SearchService] 所有查询向量化均失败")
		return nil, fmt.Errorf("failed to create query embeddings for all queries")
	}
	log.Infof("[SearchService] 步骤4: 向量化完成, 成功 %d/%d", len(queryVectors), len(allQueries))

	// 5. 并行执行多路召回 (Multi-Vector Search + Keyword Search)
	log.Info("[SearchService] 步骤5: 并行执行多路召回")

	const (
		rrfK       = 60.0
		recallSize = 60 // 每路召回数量
	)

	// 构造过滤器 (两路通用)
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

	var wg sync.WaitGroup
	var denseDocs, sparseDocs []model.EsDocument
	var sparseErr error

	// 5.1 多路向量检索 (Dense Retrieval) - 每个查询向量独立检索
	var allDenseDocs []model.EsDocument
	var denseErrors []error
	var denseMu sync.Mutex

	for _, qv := range queryVectors {
		wg.Add(1)
		go func(vector []float32) {
			defer wg.Done()
			queryBody := map[string]interface{}{
				"knn": map[string]interface{}{
					"field":          "vector",
					"query_vector":   vector,
					"k":              recallSize,
					"num_candidates": 100,
					"filter":         boolFilter,
				},
				"_source": []string{"file_md5", "chunk_id", "text_content", "user_id", "org_tag", "is_public", "vector_id"},
				"size":    recallSize,
			}
			docs, err := s.executeEsQuery(ctx, queryBody)
			denseMu.Lock()
			defer denseMu.Unlock()
			if err != nil {
				log.Errorf("[SearchService] 向量检索失败: %v", err)
				denseErrors = append(denseErrors, err)
			} else {
				log.Infof("[SearchService] 向量检索召回 %d 条文档", len(docs))
				allDenseDocs = append(allDenseDocs, docs...)
			}
		}(qv)
	}

	// 5.2 关键词检索 (Sparse Retrieval)
	wg.Add(1)
	go func() {
		defer wg.Done()
		log.Infof("[SearchService] 开始关键词检索 (Sparse Retrieval)")

		// 构建 bool query
		boolQuery := map[string]interface{}{
			"filter": boolFilter,
		}

		// 构建 must/should 子句
		var mustClauses []map[string]interface{}
		for _, kw := range keywords.CoreKeywords {
			mustClauses = append(mustClauses, map[string]interface{}{
				"match": map[string]interface{}{
					"text_content": kw,
				},
			})
		}
		var shouldClauses []map[string]interface{}
		for _, kw := range keywords.OptionalKeywords {
			shouldClauses = append(shouldClauses, map[string]interface{}{
				"match": map[string]interface{}{
					"text_content": kw,
				},
			})
		}

		// 如果没有任何关键词，使用 normalized query 作为兜底
		if len(keywords.CoreKeywords) == 0 && len(keywords.OptionalKeywords) == 0 {
			normalized, _ := normalizeQuery(query)
			mustClauses = append(mustClauses, map[string]interface{}{
				"match": map[string]interface{}{
					"text_content": map[string]interface{}{
						"query":                normalized,
						"minimum_should_match": "70%",
					},
				},
			})
		}

		if len(mustClauses) > 0 {
			boolQuery["must"] = mustClauses
		}
		if len(shouldClauses) > 0 {
			boolQuery["should"] = shouldClauses
		}

		queryBody := map[string]interface{}{
			"query": map[string]interface{}{
				"bool": boolQuery,
			},
			"_source": []string{"file_md5", "chunk_id", "text_content", "user_id", "org_tag", "is_public", "vector_id"},
			"size":    recallSize,
		}
		sparseDocs, sparseErr = s.executeEsQuery(ctx, queryBody)
		if sparseErr != nil {
			log.Errorf("[SearchService] 关键词检索失败: %v", sparseErr)
		} else {
			log.Infof("[SearchService] 关键词检索召回 %d 条文档", len(sparseDocs))
		}
	}()

	wg.Wait()

	// 合并多路向量检索结果并去重
	denseDocs = deduplicateDocs(allDenseDocs)
	log.Infof("[SearchService] 多路向量检索去重后共 %d 条文档", len(denseDocs))

	// 6. RRF 融合 (Reciprocal Rank Fusion)
	log.Info("[SearchService] 步骤6: 执行 RRF 融合")
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
		log.Warnf("[SearchService] 所有召回路径均无结果")
		return []model.SearchResponseDTO{}, nil
	}

	fusionResults := ReciprocalRankFusion(resultLists, func(doc interface{}) string {
		return doc.(model.EsDocument).VectorID
	}, rrfK)

	log.Infof("[SearchService] RRF 融合完成，共 %d 个唯一文档", len(fusionResults))

	// 6. Rerank 重排序
	log.Info("[SearchService] 步骤6: Rerank 重排序")

	// 取 Top 50 给 Rerank
	rerankSize := 50
	if len(fusionResults) > rerankSize {
		fusionResults = fusionResults[:rerankSize]
	}

	// 准备 Rerank 数据
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

	// 执行 Rerank
	var finalDocs []ScoredDoc

	// 如果候选集为空，直接返回空
	if len(candidateTexts) == 0 {
		return []model.SearchResponseDTO{}, nil
	}

	rerankResults, err := s.rerankClient.Rerank(ctx, query, candidateTexts)
	if err != nil {
		log.Warnf("[SearchService] Rerank 失败，使用 RRF 原序: %v", err)
		finalDocs = rerankCandidates
	} else {
		// 根据 Rerank 结果重新排序
		// 注意：rerankResults 的 Index 对应 candidateTexts 的索引
		for _, res := range rerankResults {
			if res.Index < len(rerankCandidates) {
				original := rerankCandidates[res.Index]
				// 使用 Rerank 分数覆盖 RRF 分数
				finalDocs = append(finalDocs, ScoredDoc{original.Doc, res.RelevanceScore})
			}
		}
		log.Infof("[SearchService] Rerank 完成，重新排序了 %d 个文档", len(finalDocs))
	}

	// 7. 截取 Top K 并组装结果
	log.Info("[SearchService] 步骤7: 获取文件名并组装最终结果")

	if len(finalDocs) > topK {
		finalDocs = finalDocs[:topK]
	}

	// 批量查询文件名
	fileMD5s := make([]string, 0, len(finalDocs))
	for _, doc := range finalDocs {
		fileMD5s = append(fileMD5s, doc.Doc.FileMD5)
	}
	// 去重
	uniqueMD5s := make(map[string]struct{})
	for _, md5 := range fileMD5s {
		uniqueMD5s[md5] = struct{}{}
	}
	md5List := make([]string, 0, len(uniqueMD5s))
	for md5 := range uniqueMD5s {
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
		fileName := fileNameMap[doc.Doc.FileMD5]
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
	if s.redisClient != nil {
		cacheKey := generateCacheKey(keywords)
		if cacheKey != "" && len(finalDocs) > 0 {
			// 提取 VectorIDs
			var docIDs []string
			for _, doc := range finalDocs {
				docIDs = append(docIDs, doc.Doc.VectorID)
			}
			cachedValue := strings.Join(docIDs, ",")

			// 异步写入
			go func(key, value string) {
				// 使用 Background context 避免父 context 取消
				cacheCtx := context.Background()
				if err := s.redisClient.Set(cacheCtx, key, value, 24*3600*1000000000).Err(); err != nil { // 24小时 (纳秒)
					log.Warnf("[SearchService] 缓存写入失败: %v", err)
				} else {
					log.Infof("[SearchService] 成功写入缓存, key: %s, docCount: %d", key, len(docIDs))
				}
			}(cacheKey, cachedValue)
		}
	}

	log.Infof("[SearchService] 混合搜索执行完毕, query: '%s', 返回 %d 条结果", query, len(results))
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
	stopPhrases := []string{"是谁", "是什么", "是啥", "请问", "怎么", "如何", "告诉我", "严格", "按照", "不要补充", "的区别", "区别", "吗", "呢", "？", "?"}
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

	res, err := s.esClient.Search(
		s.esClient.Search.WithContext(ctx),
		s.esClient.Search.WithIndex("knowledge_base"),
		s.esClient.Search.WithBody(&buf),
		s.esClient.Search.WithTrackTotalHits(true),
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
		"_source": []string{"file_md5", "chunk_id", "text_content", "user_id", "org_tag", "is_public", "vector_id"},
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
		fileName := fileNameMap[doc.FileMD5]
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

// rewriteQueries 使用 LLM 将用户查询重写为多个变体，以提升向量检索的召回率。
func (s *searchService) rewriteQueries(ctx context.Context, query string) ([]string, error) {
	if s.llmClient == nil {
		return nil, nil
	}

	prompt := fmt.Sprintf(`你是一个查询重写专家。请将用户的问题改写为 2-3 个不同表述但语义相同的查询，用于提升搜索召回率。

要求：
1. 每个改写应使用不同的表达方式（同义词替换、角度转换、抽象/具体化）
2. 保持与原问题相同的核心语义
3. 不要添加原问题中没有的信息

用户问题："%s"

请严格按照以下 JSON 格式返回，不要包含任何其他文字：
{"queries": ["改写1", "改写2", "改写3"]}`, query)

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

	// 过滤空查询和与原查询完全相同的结果
	var rewritten []string
	for _, q := range result.Queries {
		q = strings.TrimSpace(q)
		if q != "" && q != query {
			rewritten = append(rewritten, q)
		}
	}

	return rewritten, nil
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
