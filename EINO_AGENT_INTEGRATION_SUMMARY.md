# 大模型框架 Eino 与 Agent 化演进全景记录

本文档综合记录了我们将项目逐步适配 [Eino (github.com/cloudwego/eino)](https://github.com/cloudwego/eino) 框架，并从单一线性 RAG 流程升级为真正的大模型智能体（Agent）工作流的全过程。

---

## 📅 阶段一：基础模型层适配与零侵入替换

**目标：将底层原生 HTTP 调用的长连接彻底替换为 Eino 组件，使大模型调用更加稳健、规范且易于管理。**

1. **依赖库变更**
   引入 Eino 核心逻辑库及 Eino-ext 的 OpenAI 扩展组件：
   ```shell
   go get github.com/cloudwego/eino
   go get github.com/cloudwego/eino-ext/components/model/openai
   ```

2. **代码重构 (`pkg/llm/client.go`)**
   - **移除手写 HTTP 和 SSE 流解析逻辑**：移除了原本使用底层 `http.NewRequest` 手工拼接以及基于 `bufio.NewReader` 和文本分割硬编码解析数据的脆弱代码。
   - **引入标准模型实例**：通过 `openai.NewChatModel` 来建立模型连接实例。它天然兼容所有使用 OpenAI 通用接口的服务。
   - **保留现有业务接口边界**：在不改动外部接口契约和业务逻辑调用的前提下，将原有的 `llm.Client` 和 `llm.Message` 进行向 Eino 的透明适配与结构体转换，实现了对旧业务逻辑的“零侵入”基础替代。
   - **动态配置读取**：引入并使用了 `einomodel.WithTemperature`, `einomodel.WithTopP` 以及 `einomodel.WithMaxTokens` 等标准的 `model.Option` 参数注入。

---

## 📅 阶段二：Eino 核心接口规范化扩展

**目标：使业务项目内的核心功能模块规范化为 Eino 定义的组件接口，为后续的组合流（Compose）和智能体化做好底层生态铺垫。**

1. **Embedding （向量嵌入对接）**
   - 修改 `pkg/embedding/client.go` 引入 `github.com/cloudwego/eino-ext/components/embedding/openai`。
   - 彻底废弃手动发送请求的做法，现阶段直接返回由 Eino 提供原生的 `embedding.Embedder` 驱动模块来处理所有的文本字符串词向量映射。

2. **Retriever（检索器规范化绑定）**
   - 在 `internal/service/search_service.go` 中，将原本的 `HybridSearch` (混合搜索 ES 业务引擎) 包装实现为原生的 Eino `retriever.Retriever` 接口对象。
   - 原业务输出为 `model.SearchResponseDTO` 数组，封装层主动将其清洗映射成了标准的 Eino 返回构造型 `schema.Document` 集合。
   - 在向内部传递调用图谱的过程中使用 `context.WithValue` 存取用户身份的 Context 保证了基于安全控制的隔离。

---

## 📅 阶段三：RAG 智能体工作流（Agent Graph）全量重构

**目标：抛弃长代码且职责缠绕的单体面条流水线代码，将 RAG 赋予 AI 主动调用的自治权限，升级为一个具有工具调用、并发路由与分支跳转能力的智能体工作流。**

在 `internal/service/chat_service.go` 的 `StreamResponse()` 函数里，经历了彻底的改头换面，具体架构重塑如下：

1. **基于状态总图的数据流转 (State)**
   - 我们构建了一套清晰的 `RagState` 流转机制：
     ```go
     type RagState struct {
         Query         string
         User          *model.User
         History       []model.ChatMessage
         Messages      []*schema.Message // 使用 Eino 原生 Message 体系
     }
     ```
   - 数据传递变为了全量围绕该节点的状态推进。

2. **ToolsNode 的组装与智能分发路由 (Graph / Branching)**
   这正是大模型从静态 Pipeline 进入“智能体时代”的核心特征：
   - 使用 `utils.InferTool` 我们将前述准备好的 Retriever 注册为了一个可以被智能体主动呼唤的 `search_knowledge_base`（文档内部搜索）工具。并附加上自然语言的功能介绍，大模型引擎可以自动进行识别判定。
   - 使用 `utils.InferTool` 我们同时引入了公网的 HTTP 天气接口调用的工具 `get_weather`。
   - 通过将它们全部挂载入 `compose.NewToolNode()`，构建出了 Eino 工具节点池。

3. **Graph Workflow 构建**
   我们在 Eino 内真正拼装出了循环分支控制逻辑：
   - 节点 `chat_node` 将附带可选大模型工具参数进行推流与解答计算。它被流式捕获拦截器所监控（若生成常规聊解，实时 WebSocket 回推内容）。
   - 若模型决定检索知识库或询问天气，就会生成一个 ToolCall。
   - `Graph` 会由我们自定义的 `compose.NewGraphBranch` 精确截断并分配至工具集处理节点 `tools_node`，最终把获取到的事实背景和工具执行结果挂载在 `RagState.Messages` 尾部并以编排图 `AddEdge` 的形式回路导向 `chat_node` 让模型进行最终分析回答！

---

## ✅ 效果验证

通过 `go run ./cmd/server/main.go` 启动服务器后，项目支持了全自治工具工作流：
1. **基础打招呼**: “你好” -> 模型分析后直接在流中回答响应（无需检索，没有多余延迟）。
2. **私域检索工具唤醒**: “这边的请假规范流程是什么？” -> 模型产生提取文档的检索计划，Graph 流程分管去 `search_knowledge_base` 查询 Elasticsearch 的政策规定，挂载 Document 状态后再综合响应！
3. **外部 API 工具唤醒**: “北京今天天气怎么样？” -> 模型通过 Graph 执行天气工具调用 `wttr.in` 获取外部网络最新的环境实况再回复呈现。

一切运转自如，通过此系列重组，我们把底层架构打实，极大地赋予了大模型系统的可玩性和强大的应用层工程伸缩能力！
