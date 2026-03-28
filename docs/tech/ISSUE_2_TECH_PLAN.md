# 技术方案设计: Issue #2 - Implement Independent Graph RAG Endpoint with Neo4j

## 1. Context (任务背景)
当前 RAG 系统依赖向量检索和关键词检索。为了增强处理复杂实体关系的能力，引入 Neo4j 图数据库，并提供专门的图增强检索接口。

## 2. Solution (详细设计)

### 2.1 基础设施与配置
- **配置**: 在 `internal/config/config.go` 中增加 `Neo4jConfig` 结构，并在 `configs/config.yaml` 中添加对应配置项（URI, Username, Password）。
- **工具包**: 在 `pkg/neo4j` 中封装 Neo4j 驱动初始化、连接管理及通用的 Cypher 执行逻辑。

### 2.2 数据模型
- 定义 `model.GraphTriplet` 用于表示提取出的三元组 `(Subject, Predicate, Object)`。

### 2.3 索引管线 (Indexing Pipeline)
- **提取逻辑**: 在 `internal/pipeline/processor.go` 的 `Process` 流程中，在文本切块后，新增一个 `extractAndIndexGraph` 步骤。
- **LLM 调用**: 构造专门的 Prompt 提示 LLM 从 Chunk 中提取三元组。
- **持久化**: 使用 Cypher 的 `MERGE` 语句将实体和关系存入 Neo4j，确保操作的幂等性。建立节点与 `file_md5` 的关联。

### 2.4 检索服务 (Search Service)
- **GraphSearchService**: 新建服务，实现基于关键词的图路径检索。
- **检索逻辑**:
  1. 从意图中识别核心实体（关键词）。
  2. 在 Neo4j 中执行 1-hop 或 2-hop 检索，获取关联知识。
  3. 将结果格式化为文本摘要。

### 2.5 接口与路由
- **API 接口**: 新增 `/api/v1/chat/graph` (WebSocket)。
- **Handler**: 实现 `GraphChatHandler`，其逻辑为：向量检索 + 图检索 -> 知识融合 -> LLM 生成。
- **独立性**: 该接口逻辑与标准 RAG 接口隔离。

## 3. Impact (受影响模块)
- `internal/config/`
- `internal/pipeline/processor.go`
- `internal/service/`
- `internal/handler/`
- `cmd/server/main.go`

## 4. Test Plan (单元测试策略)
- **Neo4j 封装测试**: 测试连接初始化。
- **提取逻辑测试**: 使用 Mock LLM 验证三元组解析正确性。
- **检索融合测试**: 模拟图检索结果，验证与向量结果的融合逻辑。

## 5. DoD (完成定义)
- [x] 配置项已添加。
- [x] 三元组提取入库功能上线。
- [x] `/api/v1/chat/graph` 接口可用且能返回图增强结果。
- [x] 所有新增代码通过全量编译校验 (`go build ./...`)。
- [x] 修复了 `rag-admin.go` 等工具类的兼容性报错。
- [x] 编写并验证了 `GraphSearchService` 的核心单元测试。
