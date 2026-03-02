# 设计方案：基于关键词指纹的检索结果缓存 (Keyword-Fingerprint Retrieval Cache)

## 1. 背景与痛点
当前的检索链路引入了 **RRF (Reciprocal Rank Fusion)** 和 **Cross-Encoder Rerank**，虽然显著提升了准确率，但也带来了不可忽视的延迟（尤其是在没有 GPU 加速的 Rerank 场景下，单次请求可能需 500ms+）。

同时，单纯的 **Exact Match (MD5)** 缓存覆盖率太低，而 **Semantic Cache (Vector)** 存在“语义相似但逻辑不同”的误导风险（如“差旅报销”vs“医疗报销”）。

## 2. 核心思路
利用现有的 **智能关键词提取（Tier 1/2）** 模块，构建一个**基于核心关键词指纹**的中间层缓存。

**策略**: 缓存 **Rerank 后的 Top-N 文档 ID**，而非最终的 LLM 答案。

## 3. 详细设计

### 3.1 缓存键生成 (Fingerprint Generation)
不再使用原始 Query 字符串，而是使用**排序后的核心关键词序列**。

- **Query A**: "请问**Docker**镜像怎么**安装**？"
    - Core Keywords: `["Docker", "安装"]`
    - Cache Key: `fingerprint:docker|安装`
- **Query B**: "如何**安装**一个**Docker**容器？"
    - Core Keywords: `["安装", "Docker"]` (提取后) -> Sort -> `["Docker", "安装"]`
    - Cache Key: `fingerprint:docker|安装`

**判定**: Query A 和 Query B 生成相同的 Key，视为即时命中。

### 3.2 安全性保障
此策略完美规避了语义缓存的风险：
- **场景**: "申请**差旅**报销" vs "申请**医疗**报销"
- **关键词**: `["申请", "差旅", "报销"]` vs `["申请", "医疗", "报销"]`
- **结果**: Key 不同 -> **缓存未命中** -> 强制走完整检索流程。
- **结论**: 只要核心实体不同，绝不会误用缓存。

### 3.3 缓存内容 (Value)
存储 **Document ID List** (e.g., `["file_123_chunk_1", "file_456_chunk_2", ...]`)。
- **不存全文**: 节省 Redis 内存，取出来后通过 ID 去 DB/ES 批量拉取内容（毫秒级）。
- **不存 LLM 答案**: 保证 LLM 每次都能基于最新的 Prompt 和上下文生成，保持回答的灵活性。

### 3.4 流程图

```mermaid
graph TD
    A[用户查询] --> B[关键词提取 (LLM/Segmenter)]
    B --> C{生成关键词指纹 Key}
    C -->|Redis Get| D{是否命中?}
    
    D -->|YES (Hit)| E[获取 Cached DocIDs]
    E --> F[DB: 批量拉取文档内容]
    F --> G[LLM 生成回答]
    
    D -->|NO (Miss)| H[ES 并行检索 (Vector + Keyword)]
    H --> I[RRF 融合]
    I --> J[Cross-Encoder Rerank (耗时!)]
    J --> K[提取 Top-N DocIDs]
    K --> L[写入 Redis 缓存]
    K --> G
```

## 4. 代码实现伪代码 (Go)

```go
func (s *searchService) SmartSearch(ctx context.Context, query string) ([]Hit, error) {
    // 1. 提取关键词 (已实现)
    keywords, _ := s.extractKeywords(ctx, query)
    
    // 2. 生成指纹 Key
    // 过滤掉无关词，规范化，排序
    coreKeys := normalizeAndSort(keywords.CoreKeywords)
    cacheKey := fmt.Sprintf("search_cache:%s", strings.Join(coreKeys, "|"))
    
    // 3. 查缓存
    if docIDs, err := s.redis.Get(cacheKey); err == nil {
        log.Info("Hit Keyword Cache! Skipping ES & Rerank")
        // 极速路径：只查 DB
        return s.repo.GetDocsByIDs(docIDs)
    }
    
    // 4. 慢速路径 (Search + RRF + Rerank)
    docs, _ := s.HybridSearch(ctx, query, ...)
    
    // 5. 异步回写缓存
    go func() {
        ids := extractIDs(docs)
        s.redis.Set(cacheKey, ids, 24*time.Hour)
    }()
    
    return docs
}
```

## 5. 预期收益
- **延迟降低**: 命中时，P99 延迟预计从 **800ms+** 降低至 **50ms** 以内。
- **准确性**: 100% 实体级安全，无语义漂移风险。
- **资源节省**: 大幅减少 Rerank 模型（GPU/CPU）的调用次数。
