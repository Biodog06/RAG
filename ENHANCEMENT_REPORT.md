# 系统功能增强实现报告

本文档总结了本次针对系统功能增强所做的代码修改和架构升级。

## 1. 新增功能

### A. Markdown 智能切分 (Smart Markdown Splitter)
- **目的**: 解决传统按字符数硬切分导致 Markdown 标题结构丢失、语义断裂的问题。
- **实现**:
    - 新增 `pkg/splitter/markdown.go`: 实现了基于 Header 的语义切分器。
    - **逻辑**:
        - 自动识别 `#`, `##` 等标题结构。
        - **上下文保留**: 切分后的每个 Chunk 会自动补全父级标题路径（如 `[产品介绍 > 价格]`）。
        - 智能降级: 只有当一个章节内容超过 `chunkSize` 时才会在内部进行字符切分。
- **影响**: 提升了 RAG 检索的准确性，尤其是对于结构化文档（如手册、FAQ）。

### B. 智能查询重写 (Query Rewriting)
- **目的**: 解决多轮对话中用户提问模糊（如“它多少钱”）导致检索失败的问题。
- **实现**:
    - 修改 `pkg/llm/client.go`: 新增 `GenerateOneShot` 接口，支持非流式快速调用。
    - 修改 `internal/service/chat_service.go`: 在检索前增加 Rewrite 步骤。
    - **逻辑**:
        - 将 `History` + `Current Query` 发给 LLM。
        - LLM 将其改写为独立完整的句子（如“DeepSeek v3 的价格是多少”）。
        - 使用改写后的句子进行 `HybridSearch`，但保留原始对话展示给用户。

### C. 语义缓存 (Semantic Cache - Plan B)
- **目的**: 减少 LLM 重复调用，降低成本并提升高频问题的响应速度。
- **实现**:
    - 新增 `internal/service/cache_service.go`: 基于 Redis 的缓存服务。
    - **逻辑**:
        - **Key**: `md5(query + last_history_hash)`，确保同一个上下文下的相同问题才能命中。
        - **Value**: LLM 生成的完整答案。
        - **Flow**: `ChatService` 收到请求 -> 查 Redis -> 命中则直接返回 -> 未命中则走 RAG 并在结束后写入 Redis。
    - **策略**: 目前采用**精确匹配**策略（Plan B），兼容无 Vector 模块的 Redis 环境。

---

## 2. 代码变更清单

| 模块 | 文件路径 | 修改内容 |
| :--- | :--- | :--- |
| **Pipeline** | `internal/pipeline/processor.go` | 引入 `splitter` 包，根据文件类型 (`.md`) 自动选择分块策略。 |
| **Splitter** | `pkg/splitter/markdown.go` (New) | 新增 Markdown 语义切分逻辑。 |
| **Splitter** | `pkg/splitter/text.go` (New) | 将原有的简单文本切分逻辑抽取为独立服务。 |
| **Service** | `internal/service/chat_service.go` | 集成 `rewriteQuery` 和 `cacheService`；优化 `StreamResponse` 流程。 |
| **Service** | `internal/service/cache_service.go` (New) | 新增 Redis 缓存服务实现。 |
| **LLM** | `pkg/llm/client.go` | 新增 `GenerateOneShot` 方法用于非流式生成。 |
| **Main** | `cmd/server/main.go` | 初始化 `CacheService` 并注入到 `ChatService`。 |

## 3. 验证方法

1.  **验证切分**: 上传一个包含多级标题的 Markdown 文件，查看数据库 `document_vectors` 表，内容应包含 `[标题 > 子标题]` 前缀。
2.  **验证重写**: 发起多轮对话（"它是谁?"），查看日志 `[ChatService] Query rewritten:`，应看到改写后的完整问题。
3.  **验证缓存**: 连续问两次完全相同的问题（在相同上下文下），第二次应秒级返回，且日志显示 `[CacheService] Cache HIT`。
