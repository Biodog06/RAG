@echo off
REM 测试 MCP Server HTTP 版本

echo ========================================
echo MCP Server HTTP 测试脚本
echo ========================================
echo.

REM 测试 1: 列出可用工具
echo [测试 1] 列出可用工具...
curl -X POST http://localhost:8082/mcp/message ^
  -H "Content-Type: application/json" ^
  -d "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"tools/list\"}"
echo.
echo.

REM 测试 2: 搜索知识库
echo [测试 2] 搜索知识库...
curl -X POST http://localhost:8082/mcp/message ^
  -H "Content-Type: application/json" ^
  -d "{\"jsonrpc\":\"2.0\",\"id\":2,\"method\":\"tools/call\",\"params\":{\"name\":\"search_knowledge_base\",\"arguments\":{\"query\":\"deployment\",\"top_k\":3}}}"
echo.
echo.

REM 测试 3: 列出文档
echo [测试 3] 列出文档...
curl -X POST http://localhost:8082/mcp/message ^
  -H "Content-Type: application/json" ^
  -d "{\"jsonrpc\":\"2.0\",\"id\":3,\"method\":\"tools/call\",\"params\":{\"name\":\"list_documents\",\"arguments\":{\"page\":1,\"page_size\":5}}}"
echo.
echo.

echo ========================================
echo 测试完成！
echo ========================================
pause
