# PaiSmart 系统优化与演进计划

本计划旨在通过引入前沿的检索增强技术 (RAG) 和标准协议 (MCP) 支持，将系统从“可用”提升至“生产级高性能”。

## 1. 核心检索与 RAG 优化

### 1.1 智能关键词提取与三层降级策略 (Smart Keyword Extraction & Three-Tier Fallback) [已完成]
当前 ES 查询策略从简单的 "Text Matching" 升级为智能的语义理解和分层降级机制。
- **痛点**: 原始查询中的口语词（如“请问”、“怎么”）干扰检索，且简单的 `match` 查询缺乏对核心词的侧重。
- **方案**:
    - **Tier 1 (LLM)**: 使用 LLM 精准提取关键词，分为 `Core` (放入 `must` 子句) 和 `Optional` (放入 `should` 子句)。准确率 ~98%。
    - **Tier 2 (Segmenter)**: 引入 `gse` 分词器与词性标注 (POS Tagging)，自动识别名词/英文作为核心词，动词作为可选词。作为 LLM 失败或超时时的低延迟降级方案 (1-2ms)。
    - **Tier 3 (Fallback)**: 保留正则处理 + 简单分词作为最终兜底，确保系统 100% 可用。
- **收益**: 
    - **精准度**: LLM 理解语义，排除无关词干扰。
    - **召回率**: `Optional` 关键词机制允许非核心词缺失，扩大召回范围。
    - **高可用**: 三层机制互为备份，兼顾效果与稳定性。

### 1.2 引入重排序 (Rerank) [已完成]
在放宽召回条件后，候选集会变大且含噪，需引入精排。
- **方案**:
    - 在 `SearchService` 中增加 Post-Retrieval 阶段。
    - 集成 **Cross-Encoder Rerank** 模型（支持 BGE-Reranker-v2 或 Jina Rerank API）。
    - 对 Top-50 初筛结果进行语义打分，只取 Top-10 给 LLM。
- **预期收益**: 配合关键词提取，实现“**广撒网（高召回）+ 精挑选（高准确）**”，MRR 提升 20%+。

### 1.3 融合算法升级 (RRF) [已完成]
- **现状**: 使用硬编码权重 (`0.2` vs `1.0`) 融合向量和关键词分数。
- **优化**: 实现 **Reciprocal Rank Fusion (RRF)** 算法。
    - 并行执行向量检索与关键词检索，每路召回 Top-60。
    - 使用 RRF 算法 (`k=60`) 融合两路结果，消除分数归一化难题。
    - RRF 融合后的 Top-50 结果送入 Rerank 模型精排。
- **收益**: 消除手动调参的痛苦，对不同分布的查询更鲁棒，显著提升长尾查询的召回率。

### 1.4 Elasticsearch 深度优化
- **向量量化**: 对于 2048 维向量（豆包模型），建议开启 `int8` 量化以节省 75% 内存。
- **HNSW 参数**: 显式设置 `ef_construction=128` 和 `m=24` 以平衡写入速度与检索召回率。
- **混合精度**: 引入 `knn` 的 `filter` 预过滤（Pre-filtering）提升特定 Organization 下的检索速度。

### 1.5 动态匹配阈值调优 (Dynamic Thresholding)
- **现状**: 当前 `minimum_should_match` 硬编码为 `"70%"`，这是一个经验值。
- **优化**:
    - 引入 **Auto-Tuning**: 针对不同长度的 Query 设置分段阈值（如 <5个词需100%，>10个词只需50%）。
    - **A/B 测试**: 通过线上流量对比不同匹配占比（60% vs 70% vs 80%）下的用户采纳率，确定最佳参数。
- **收益**: 在保证不漏召回的前提下，最大程度减少噪声文档进入 Rerank 阶段。

---

## 2. MCP (Model Context Protocol) 集成 [已完成]

将 PaiSmart 打造为一个 **MCP Server**，使其能被 Claude Desktop、Cursor 或其他 Agent 平台直接作为“大脑”调用。

### 2.1 架构设计
- **新增入口**: `cmd/mcp-server/main.go` 和 `cmd/mcp-server-http/main.go`
- **依赖库**: `github.com/mark3labs/mcp-go`
- **通信方式**: 支持 Stdio (CLI) 和 SSE (HTTP) 两种模式。

### 2.2 暴露工具 (Tools)
1.  **`search_knowledge_base`**:
    - 参数: `query` (string), `org_tag` (string, optional)
    - 描述: "在企业知识库中语义检索相关文档片段"
2.  **`list_documents`**:
    - 参数: `page`, `page_size`
    - 描述: "查看当前知识库中已有的文件列表"

---

## 3. 系统级功能增强

### 3.1 缓存策略 (Caching Strategy) [已完成]
- **决策**: **放弃语义缓存 (Semantic Cache)**，采用 **精确匹配 (Exact Match) + 关键词指纹缓存 (Keyword Fingerprint Cache)**。
- **原因**: 
    - **准确性风险**: 语义相似不等于逻辑等价（如 "申请差旅报销" 与 "申请医疗报销" 向量极似但答案完全不同），误命中会导致严重误导。
    - **时效性**: 语义缓存难以精确失效（Cache Invalidation），容易返回过时信息。
- **实现**: 
    - **基于关键词指纹的检索结果缓存 (Result Cache)**: 使用排序后的核心关键词生成缓存键（如 `search_cache:docker|安装`），缓存 Rerank 后的 Top-N 文档 ID 列表。
    - **命中时**: 跳过 ES 检索 + RRF 融合 + Rerank（节省 500ms+），直接根据 ID 查询文档。
    - **未命中时**: 走完整检索流程，异步回写缓存（24小时过期）。
    - **安全性**: 核心关键词必须完全一致，避免"差旅报销"与"医疗报销"误命中。

### 3.2 智能查询重写 (Query Rewriting) [已完成]
- **痛点**: 用户提问往往含糊或缺失主语。
- **方案**:
    - 在检索前引入轻量级 LLM 调用。
    - 将 "它怎么用？" + [History] -> 重写为 "PaiSmart 系统的 RAG 功能怎么配置？"。
    - 实现了 `rewriteQuery` 方法，利用历史上下文补全查询。

### 3.3 文档解析升级 [已完成]
- **现状**: 简单的文本切分丢失了结构信息。
- **优化**:
    - 增加了 **Markdown 转换器** (`MarkdownSplitter`)。
    - 能够识别 `# 标题` 层级，将文档切分为带有上下文 header 的 chunk，显著提升了检索内容的语义完整性。

---

## 4. 实施路线图 (Phase Plan)

| 阶段 | 重点任务 | 状态 |
| :--- | :--- | :--- |
| **Phase 1** | **Rerank & ES Optimization** <br> - 接入 Rerank API <br> - 优化 ES Mapping | ✅ 已完成 |
| **Phase 2** | **MCP Server** <br> - 搭建 MCP 服务框架 <br> - 实现 search 工具接口 | ✅ 已完成 |
| **Phase 3** | **Architecture** <br> - 实现 Redis 缓存 (Exact) <br> - 增加 Query Rewrite <br> - 实现 Markdown Splitter <br> - **实现三层关键词提取策略** | ✅ 已完成 |
| **Phase 4** | **Next Steps** <br> - ES 向量量化与参数调优 <br> - 升级语义缓存为向量匹配 <br> - 实现 RRF 融合算法 | 📅 待排期 |

