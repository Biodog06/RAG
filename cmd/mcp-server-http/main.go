// Package main 是 MCP Server HTTP 版本的入口点
package main

import (
	"context"
	"fmt"
	"net/http"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"pai-smart-go/internal/config"
	"pai-smart-go/internal/repository"
	"pai-smart-go/internal/service"
	"pai-smart-go/pkg/database"
	"pai-smart-go/pkg/embedding"
	"pai-smart-go/pkg/es"
	"pai-smart-go/pkg/log"
	"pai-smart-go/pkg/rerank"
)

func main() {
	// 1. 初始化配置
	config.Init("./configs/config.yaml")
	cfg := config.Conf

	// 2. 初始化日志
	log.Init(cfg.Log.Level, cfg.Log.Format, cfg.Log.OutputPath)
	defer log.Sync()
	log.Info("MCP Server HTTP 版本启动中...")

	// 3. 初始化数据库
	database.InitMySQL(cfg.Database.MySQL.DSN)
	database.InitRedis(cfg.Database.Redis.Addr, cfg.Database.Redis.Password, cfg.Database.Redis.DB)
	err := es.InitES(cfg.Elasticsearch)
	if err != nil {
		log.Fatalf("Elasticsearch 初始化失败: %v", err)
	}

	// 4. 初始化 Repository 和 Service
	userRepository := repository.NewUserRepository(database.DB)
	uploadRepo := repository.NewUploadRepository(database.DB, database.RDB)
	embeddingClient := embedding.NewClient(cfg.Embedding)
	rerankClient := rerank.NewClient(cfg.Rerank)

	userService := service.NewUserService(userRepository, repository.NewOrgTagRepository(database.DB), nil)
	searchService := service.NewSearchService(embeddingClient, es.ESClient, userService, uploadRepo, rerankClient)

	// 5. 获取默认用户（admin）用于MCP调用
	defaultUser, err := userRepository.FindByUsername("admin")
	if err != nil {
		log.Fatalf("无法找到默认用户 'admin': %v", err)
	}

	// 6. 创建 MCP Server
	mcpServer := server.NewMCPServer("PaiSmart Knowledge Base", "1.0.0")

	// 7. 注册工具 - search_knowledge_base
	searchTool := mcp.NewTool("search_knowledge_base",
		mcp.WithDescription("在企业知识库中语义检索相关文档片段"),
		mcp.WithString("query",
			mcp.Required(),
			mcp.Description("要搜索的问题或关键词"),
		),
		mcp.WithNumber("top_k",
			mcp.Description("返回结果数量（默认10）"),
		),
	)

	mcpServer.AddTool(searchTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, ok := request.Params.Arguments.(map[string]interface{})
		if !ok {
			return mcp.NewToolResultError("无法解析参数"), nil
		}

		query, ok := args["query"].(string)
		if !ok || query == "" {
			return mcp.NewToolResultError("参数 'query' 是必需的"), nil
		}

		topK := 10
		if k, ok := args["top_k"].(float64); ok {
			topK = int(k)
		}

		log.Infof("[MCP HTTP] 收到搜索请求: query='%s', top_k=%d", query, topK)

		results, err := searchService.HybridSearch(ctx, query, topK, defaultUser)
		if err != nil {
			log.Errorf("[MCP HTTP] 搜索失败: %v", err)
			return mcp.NewToolResultError(fmt.Sprintf("搜索失败: %v", err)), nil
		}

		var resultText string
		if len(results) == 0 {
			resultText = "未找到相关文档"
		} else {
			resultText = fmt.Sprintf("找到 %d 条相关结果:\n\n", len(results))
			for i, r := range results {
				resultText += fmt.Sprintf("[%d] 文件: %s (分数: %.3f)\n内容: %s\n\n",
					i+1, r.FileName, r.Score, r.TextContent)
			}
		}

		log.Infof("[MCP HTTP] 搜索成功，返回 %d 条结果", len(results))
		return mcp.NewToolResultText(resultText), nil
	})

	// 8. 注册工具 - list_documents
	listTool := mcp.NewTool("list_documents",
		mcp.WithDescription("查看知识库中已有的文件列表"),
		mcp.WithNumber("page",
			mcp.Description("页码（从1开始，默认1）"),
		),
		mcp.WithNumber("page_size",
			mcp.Description("每页数量（默认20）"),
		),
	)

	mcpServer.AddTool(listTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, ok := request.Params.Arguments.(map[string]interface{})
		if !ok {
			args = make(map[string]interface{})
		}

		page := 1
		if p, ok := args["page"].(float64); ok && p > 0 {
			page = int(p)
		}

		pageSize := 20
		if ps, ok := args["page_size"].(float64); ok && ps > 0 {
			pageSize = int(ps)
		}

		log.Infof("[MCP HTTP] 收到列表请求: page=%d, page_size=%d", page, pageSize)

		files, total, err := uploadRepo.FindAllWithPagination(page, pageSize)
		if err != nil {
			log.Errorf("[MCP HTTP] 查询文件列表失败: %v", err)
			return mcp.NewToolResultError(fmt.Sprintf("查询失败: %v", err)), nil
		}

		resultText := fmt.Sprintf("共有 %d 个文档，当前显示第 %d 页（每页 %d 条）:\n\n", total, page, pageSize)
		for i, f := range files {
			resultText += fmt.Sprintf("%d. %s (MD5: %s)\n",
				(page-1)*pageSize+i+1, f.FileName, f.FileMD5)
		}

		log.Infof("[MCP HTTP] 返回 %d 个文件", len(files))
		return mcp.NewToolResultText(resultText), nil
	})

	// 9. 创建 SSE Server（用于HTTP通信）
	sseServer := server.NewSSEServer(
		mcpServer,
		server.WithBasePath("/mcp"),
	)

	// 10. 启动 HTTP Server
	port := ":8082"
	log.Infof("MCP Server HTTP 版本启动在 http://localhost%s", port)
	log.Infof("SSE Endpoint: http://localhost%s/mcp/sse", port)
	log.Infof("Message Endpoint: http://localhost%s/mcp/message", port)

	if err := http.ListenAndServe(port, sseServer); err != nil {
		log.Fatalf("HTTP Server 启动失败: %v", err)
	}
}
