# PaiSmart 系统优化与演进计划

本计划旨在通过引入前沿的检索增强技术 (RAG) 和标准协议 (MCP) 支持，将系统从“可用”提升至“生产级高性能”。

## 1. 核心检索与 RAG 优化

### 1.1 检索策略调优 (Relaxed Retrieval) [高优先级]
当前 ES 查询策略过于严格，`must` 下的 `match` 条件可能导致向量检索召回的相关文档被强制过滤（漏召回）。
- **痛点**: `must` 强制要求包含关键词。若用户输入生僻词或拼写错误，即使语义完全匹配（向量相似度高），文档也会被踢出。
- **方案**:
    - **策略松绑**: 将 `match` 查询从 `must` 移至 `should`，或设置 `minimum_should_match: "70%"`。
- **预期收益**: 显著提升**召回率 (Recall)**，不再因为少一个词而漏掉关键文档。

### 1.2 引入重排序 (Rerank) [高优先级]
在放宽召回条件后，候选集会变大且含噪，需引入精排。
- **方案**:
    - 在 `SearchService` 中增加 Post-Retrieval 阶段。
    - 集成 **Cross-Encoder Rerank** 模型（推荐接入 BGE-Reranker-v2 或 Jina Rerank API）。
    - 对 Top-50 初筛结果（包含向量召回的模糊结果）进行语义打分，只取 Top-10 给 LLM。
- **预期收益**: 配合策略松绑，实现“**广撒网（高召回）+ 精挑选（高准确）**”，MRR 提升 20%+。

### 1.3 融合算法升级 (RRF)
- **现状**: 使用硬编码权重 (`0.2` vs `1.0`) 融合向量和关键词分数。
- **优化**: 实现 **Reciprocal Rank Fusion (RRF)** 算法。
    - 公式: `Score = 1 / (k + rank_vector) + 1 / (k + rank_keyword)`
- **收益**: 消除手动调参的痛苦，对不同分布的查询更鲁棒。

### 1.4 Elasticsearch 深度优化
- **向量量化**: 对于 2048 维向量（豆包模型），建议开启 `int8` 量化以节省 75% 内存。
- **HNSW 参数**: 显式设置 `ef_construction=128` 和 `m=24` 以平衡写入速度与检索召回率。
- **混合精度**: 引入 `knn` 的 `filter` 预过滤（Pre-filtering）提升特定 Organization 下的检索速度。

### 1.5 动态匹配阈值调优 (Dynamic Thresholding)
- **现状**: 当前 `minimum_should_match` 硬编码为 `"70%"`，这是一个经验值，可能不适用于所有长度的查询。
- **优化**:
    - 引入 **Auto-Tuning**: 针对不同长度的 Query 设置分段阈值（如 <5个词需100%，>10个词只需50%）。
    - **A/B 测试**: 通过线上流量对比不同匹配占比（60% vs 70% vs 80%）下的用户采纳率，确定最佳参数。
- **收益**: 在保证不漏召回的前提下，最大程度减少噪声文档进入 Rerank 阶段。

---

## 2. MCP (Model Context Protocol) 集成 [新功能]

将 PaiSmart 打造为一个 **MCP Server**，使其能被 Claude Desktop、Cursor 或其他 Agent 平台直接作为“大脑”调用。

### 2.1 架构设计
- **新增入口**: `cmd/mcp-server/main.go`
- **依赖库**: `github.com/mark3labs/mcp-go` (Go 语言社区标准实现)

### 2.2 暴露工具 (Tools)
1.  **`search_knowledge_base`**:
    - 参数: `query` (string), `org_tag` (string, optional)
    - 描述: "在企业知识库中语义检索相关文档片段"
2.  **`list_documents`**:
    - 参数: `page`, `page_size`
    - 描述: "查看当前知识库中已有的文件列表"
3.  **`upload_document`** (可选):
    - 参数: `file_url`, `tags`
    - 描述: "通过 URL 上传新知识"

---

## 3. 系统级功能增强

### 3.1 语义缓存 (Semantic Cache)
- **场景**: 避免重复回答相似问题（如“报销流程”和“怎么报销”），降低 LLM Token 成本。
- **实现**:
    - 使用 Redis 存储 `Query_Vector -> LLM_Response` 映射。
    - 每次提问先查 Redis 向量相似度 (Threshold > 0.95)。
    - 命中则直接返回，延迟 < 50ms。

### 3.2 智能查询重写 (Query Rewriting)
- **痛点**: 用户提问往往含糊或缺失主语。
- **方案**:
    - 在检索前引入轻量级 LLM 调用。
    - 将 "它怎么用？" + [History] -> 重写为 "PaiSmart 系统的 RAG 功能怎么配置？"。
    - 生成多路查询 (Multi-Query) 并行检索。

### 3.3 文档解析升级
- **现状**: Tika 对 PDF 表格和复杂布局支持有限。
- **优化**:
    - 引入专门的 PDF 解析器 (如 PyMuPDF 或 LlamaParse)。
    - 增加 **Markdown 转换器**，将文档转为 Markdown 存入 ES，保留标题层级结构。

---

## 4. 实施路线图 (Phase Plan)

| 阶段 | 重点任务 | 预计工时 |
| :--- | :--- | :--- |
| **Phase 1** | **Rerank & ES Optimization** <br> - 接入 Rerank API <br> - 优化 ES Mapping | 2 Days |
| **Phase 2** | **MCP Server** <br> - 搭建 MCP 服务框架 <br> - 实现 search 工具接口 | 1 Day |
| **Phase 3** | **Architecture** <br> - 实现 Redis 语义缓存 <br> - 增加 Query Rewrite | 2 Days |

建议优先执行 **Phase 1** 以立竿见影地提升现有 RAG 效果，随后执行 **Phase 2** 扩展生态能力。
