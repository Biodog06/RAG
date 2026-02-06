# MCP Server HTTP 版本使用指南

## 快速开始

### 1. 启动服务

```bash
# 从项目根目录执行
.\mcp-server-http.exe
```

服务将启动在 `http://localhost:8082`

**Endpoints:**
- SSE Endpoint: `http://localhost:8082/mcp/sse`
- Message Endpoint: `http://localhost:8082/mcp/message`

### 2. 测试工具调用

#### 使用 curl 测试搜索功能

```bash
# 搜索知识库
curl -X POST http://localhost:8082/mcp/message \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "id": 1,
    "method": "tools/call",
    "params": {
      "name": "search_knowledge_base",
      "arguments": {
        "query": "部署流程",
        "top_k": 5
      }
    }
  }'
```

#### 列出文档

```bash
curl -X POST http://localhost:8082/mcp/message \
  -H "Content-Type": application/json" \
  -d '{
    "jsonrpc": "2.0",
    "id": 2,
    "method": "tools/call",
    "params": {
      "name": "list_documents",
      "arguments": {
        "page": 1,
        "page_size": 10
      }
    }
  }'
```

#### 列出可用工具

```bash
curl -X POST http://localhost:8082/mcp/message \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "id": 3,
    "method": "tools/list"
  }'
```

### 3. 使用 Postman 测试

1. 创建新的 POST 请求
2. URL: `http://localhost:8082/mcp/message`
3. Headers: `Content-Type: application/json`
4. Body (raw JSON):

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "tools/call",
  "params": {
    "name": "search_knowledge_base",
    "arguments": {
      "query": "你的搜索问题",
      "top_k": 10
    }
  }
}
```

## 与 Stdio 版本的区别

| 特性 | Stdio 版本 | HTTP 版本 |
|------|-----------|-----------|
| 通信方式 | 标准输入输出 | HTTP/SSE |
| 使用场景 | Claude Desktop, Cursor 集成 | API调用, Web集成, 独立测试 |
| 测试难度 | 需要 MCP Inspector | 直接使用 curl/Postman |
| 端口 | 无 | 8082 |

## MCP JSON-RPC 协议说明

HTTP 版本使用标准的 JSON-RPC 2.0 协议。

### 请求格式

```json
{
  "jsonrpc": "2.0",
  "id": <唯一请求ID>,
  "method": "<方法名>",
  "params": {
    // 方法参数
  }
}
```

### 响应格式

**成功:**
```json
{
  "jsonrpc": "2.0",
  "id": <对应的请求ID>,
  "result": {
    // 返回结果
  }
}
```

**错误:**
```json
{
  "jsonrpc": "2.0",
  "id": <对应的请求ID>,
  "error": {
    "code": <错误代码>,
    "message": "<错误信息>"
  }
}
```

## 常见问题

**Q: 如何停止服务？**
A: Ctrl+C 终止进程

**Q: 可以修改端口吗？**
A: 可以，编辑 `cmd/mcp-server-http/main.go` 中的 `port := ":8082"` 行，然后重新编译。

**Q: 为什么没有认证？**
A: 这是测试版本，专为本地开发设计。生产环境请使用 API Gateway 添加认证层。

**Q: 如何集成到前端应用？**
A: 使用 fetch 或 axios 发送 POST 请求到 `/mcp/message` endpoint。

## 示例：JavaScript 调用

```javascript
async function searchKnowledgeBase(query, topK = 10) {
  const response = await fetch('http://localhost:8082/mcp/message', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    body: JSON.stringify({
      jsonrpc: '2.0',
      id: Date.now(),
      method: 'tools/call',
      params: {
        name: 'search_knowledge_base',
        arguments: { query, top_k: topK }
      }
    })
  });
  
  const data = await response.json();
  return data.result;
}

// 使用
const results = await searchKnowledgeBase('RAG 是什么');
console.log(results);
```

## 日志

日志文件位于 `./logs` 目录，可实时查看请求和响应。
