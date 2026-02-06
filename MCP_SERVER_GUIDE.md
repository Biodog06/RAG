# MCP Server 使用说明

## 什么是 MCP Server?

Model Context Protocol (MCP) 是一个开放标准，允许 AI 应用（如 Claude Desktop、Cursor）通过标准化接口调用外部工具。

PaiSmart MCP Server 将知识库检索能力暴露为 MCP 工具，使得您可以在 Claude Desktop 对话中直接查询企业知识库。

## 编译

```bash
# 从项目根目录执行
go build -o mcp-server.exe ./cmd/mcp-server
```

## 配置集成

### Claude Desktop (Windows)

1. 找到配置文件：`%APPDATA%\Claude\claude_desktop_config.json`

2. 添加以下配置：
```json
{
  "mcpServers": {
    "paismart": {
      "command": "C:\\path\\to\\your\\rag\\mcp-server.exe",
      "args": [],
      "env": {}
    }
  }
}
```

3. 重启 Claude Desktop

### Claude Desktop (macOS/Linux)

1. 找到配置文件：`~/Library/Application Support/Claude/claude_desktop_config.json`

2. 添加以下配置：
```json
{
  "mcpServers": {
    "paismart": {
      "command": "/path/to/your/rag/mcp-server",
      "args": [],
      "env": {}
    }
  }
}
```

3. 重启 Claude Desktop

### Cursor

在 Cursor 的 Settings → MCP 中添加本地服务器路径。

## 可用工具

### 1. search_knowledge_base

在企业知识库中语义检索相关文档片段。

**参数：**
- `query` (string, 必需): 要搜索的问题或关键词
- `top_k` (number, 可选): 返回结果数量（默认10）

**示例：**
```
在 Claude Desktop 中输入:
"请帮我搜索关于部署流程的文档"

Claude 会自动调用 search_knowledge_base 工具。
```

### 2. list_documents

查看知识库中已有的文件列表。

**参数：**
- `page` (number, 可选): 页码（从1开始，默认1）
- `page_size` (number, 可选): 每页数量（默认20）

**示例：**
```
在 Claude Desktop 中输入:
"有哪些文档可用？"

Claude 会自动调用 list_documents 工具。
```

## 测试

### 手动测试

```bash
# 直接运行（会等待 stdio 输入）
./mcp-server.exe
```

### 使用 MCP Inspector

```bash
# 安装
npm install -g @modelcontextprotocol/inspector

# 运行
npx @modelcontextprotocol/inspector mcp-server.exe
```

这将启动一个Web界面，可以手动测试工具调用。

## 注意事项

1. **默认用户**: MCP Server 使用 `admin` 用户执行所有查询。确保数据库中存在此用户。

2. **权限**: 查询结果受用户权限控制（基于 `org_tag` 和 `is_public`）。

3. **日志**: 日志输出到 `./logs` 目录，可用于调试。

4. **性能**: 每次查询都会调用完整的 RAG pipeline（包括 Rerank），请确保相关服务（ES、LLM API）可用。

## 常见问题

**Q: Claude Desktop 看不到工具？**
A: 检查配置文件路径是否正确，确保 mcp-server 可执行文件存在，重启 Claude Desktop。

**Q: 工具调用失败？**
A: 查看日志文件 `./logs` 中的错误信息，确保数据库、ES、Embedding API 等服务正常运行。

**Q: 如何更新工具？**
A: 重新编译 mcp-server.exe 后，重启 Claude Desktop 即可。
