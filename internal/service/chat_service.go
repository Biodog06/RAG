// Package service 包含了应用的业务逻辑层。
package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"pai-smart-go/internal/config"
	"pai-smart-go/internal/model"
	"pai-smart-go/internal/repository"
	"pai-smart-go/pkg/llm"
	"pai-smart-go/pkg/log"
	"strings"
	"time"

	einomodel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"github.com/gorilla/websocket"
)

// ChatService 定义了聊天操作的接口。
type ChatService interface {
	StreamResponse(ctx context.Context, query string, user *model.User, ws *websocket.Conn, shouldStop func() bool) error
}

type chatService struct {
	searchService    SearchService
	llmClient        llm.Client
	conversationRepo repository.ConversationRepository
	cacheService     ContentCacheService // Added
}

// NewChatService 创建一个新的 ChatService 实例。
func NewChatService(searchService SearchService, llmClient llm.Client, conversationRepo repository.ConversationRepository, cacheService ContentCacheService) ChatService {
	return &chatService{
		searchService:    searchService,
		llmClient:        llmClient,
		conversationRepo: conversationRepo,
		cacheService:     cacheService,
	}
}

// StreamResponse 协调 RAG 流程并流式传输 LLM 响应。
func (s *chatService) StreamResponse(ctx context.Context, query string, user *model.User, ws *websocket.Conn, shouldStop func() bool) error {
	// 0. 加载历史记录 (用于改写和后续对话)
	history, err := s.loadHistory(ctx, user.ID)
	if err != nil {
		log.Errorf("Failed to load conversation history: %v", err)
		history = []model.ChatMessage{}
	}

	// 拦截 websocket writer 以捕获完整答案，并包装为 JSON 分块
	answerBuilder := &strings.Builder{}
	interceptor := &wsWriterInterceptor{conn: ws, writer: answerBuilder, shouldStop: shouldStop}

	// Cache Check 提前拦截（相当于短路）
	if cachedAnswer, hit := s.cacheService.Get(ctx, query, history); hit {
		interceptor.WriteMessage(websocket.TextMessage, []byte(cachedAnswer))
		sendCompletion(ws)
		s.addMessageToConversation(context.Background(), user.ID, query, cachedAnswer)
		return nil
	}

	// ============== EINO GRAPH ORCHESTRATION ==============
	// 定义在各节点之间流转的全局状态
	type RagState struct {
		Query    string
		User     *model.User
		History  []model.ChatMessage
		Messages []*schema.Message // 使用 Eino 原生 Message 数组
	}

	// 提前将历史记录组装为初始 messages
	systemMsgText := s.buildSystemMessage("")
	initialMsgs := s.composeMessages(systemMsgText, history, query)
	var einoMsgs []*schema.Message
	for _, m := range initialMsgs {
		if m.Role == "user" {
			einoMsgs = append(einoMsgs, schema.UserMessage(m.Content))
		} else if m.Role == "assistant" {
			einoMsgs = append(einoMsgs, schema.AssistantMessage(m.Content, nil))
		} else {
			einoMsgs = append(einoMsgs, schema.SystemMessage(m.Content))
		}
	}

	initState := &RagState{
		Query:    query,
		User:     user,
		History:  history,
		Messages: einoMsgs,
	}

	// 将 Context 注入用户信息给 Retriever 使用
	ctx = WithUserContext(ctx, user)

	// 1. Convert SearchService to Eino Retriever
	retrieverTool := s.searchService.AsEinoRetriever()

	// 2. Wrap tools into Eino InvokableTool
	type SearchToolInput struct {
		Query string `json:"query" jsonschema:"description=提取的核心搜索关键词汇,说明:需要检索什么样的内容"`
	}
	searchTool, err := utils.InferTool("search_knowledge_base", "当遇到需要查询内部政策、指南、请假规范等非通用知识库内容时，优先调用此工具检索对应文档，提供关键词获取文档背景知识", func(ctx context.Context, input *SearchToolInput) (string, error) {
		log.Infof("[ChatService-Agent] Triggered tool search_knowledge_base, query: '%s'", input.Query)
		docs, err := retrieverTool.Retrieve(ctx, input.Query)
		if err != nil {
			return "", err
		}
		if len(docs) == 0 {
			return "没有相关的知识库文档可以参考。", nil
		}
		var res strings.Builder
		for i, d := range docs {
			res.WriteString(fmt.Sprintf("[%d] %s\n", i+1, d.Content))
		}
		return res.String(), nil
	})
	if err != nil {
		return fmt.Errorf("failed to create search tool: %v", err)
	}

	type WeatherToolInput struct {
		Location string `json:"location" jsonschema:"description=需要查询天气的城市名称或拼音,例如 北京, Beijing, Shanghai"`
	}
	weatherTool, err := utils.InferTool("get_weather", "当用户询问特定地点的当前天气时调用此工具,需提供地名", func(ctx context.Context, input *WeatherToolInput) (string, error) {
		log.Infof("[ChatService-Agent] Triggered tool get_weather, location: '%s'", input.Location)
		url := fmt.Sprintf("https://wttr.in/%s?format=3", input.Location)
		resp, err := http.Get(url)
		if err != nil {
			return "", fmt.Errorf("failed to fetch weather: %v", err)
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return "", fmt.Errorf("failed to read weather response: %v", err)
		}
		return string(body), nil
	})
	if err != nil {
		return fmt.Errorf("failed to create weather tool: %v", err)
	}

	// 3. 构造 ToolsNode
	toolsNode, err := compose.NewToolNode(ctx, &compose.ToolsNodeConfig{
		Tools: []tool.BaseTool{searchTool, weatherTool},
	})
	if err != nil {
		return fmt.Errorf("failed to create tools node: %v", err)
	}

	// 4. 初始化 Eino Graph
	graph := compose.NewGraph[*RagState, *RagState]()

	// Chat 核心推理节点
	graph.AddLambdaNode("chat_node", compose.InvokableLambda(func(ctx context.Context, state *RagState) (*RagState, error) {
		einoModel := s.llmClient.AsEinoChatModel()
		searchToolInfo, _ := searchTool.Info(ctx)
		weatherToolInfo, _ := weatherTool.Info(ctx)

		streamReader, err := einoModel.Stream(ctx, state.Messages, einomodel.WithTools([]*schema.ToolInfo{searchToolInfo, weatherToolInfo}))
		if err != nil {
			return nil, fmt.Errorf("llm stream failed: %w", err)
		}
		defer streamReader.Close()

		var isToolCall bool
		var fullMessage *schema.Message

		for {
			chunk, err := streamReader.Recv()
			if err != nil {
				if err.Error() == "EOF" || strings.Contains(err.Error(), "EOF") {
					break
				}
				// Allow standard eof or stream close
				break
			}

			if len(chunk.ToolCalls) > 0 {
				isToolCall = true
			}

			// Streaming chunk content to the user only if it's not a tool call
			if !isToolCall && chunk.Content != "" {
				interceptor.WriteMessage(websocket.TextMessage, []byte(chunk.Content))
			}

			if fullMessage == nil {
				fullMessage = chunk
			} else {
				fullMessage, _ = schema.ConcatMessages([]*schema.Message{fullMessage, chunk})
			}
		}

		if fullMessage != nil {
			state.Messages = append(state.Messages, fullMessage)
		}
		return state, nil
	}))

	// 工具执行节点
	graph.AddLambdaNode("tools_node", compose.InvokableLambda(func(ctx context.Context, state *RagState) (*RagState, error) {
		lastMsg := state.Messages[len(state.Messages)-1]
		toolMessages, err := toolsNode.Invoke(ctx, lastMsg)
		if err != nil {
			return nil, fmt.Errorf("tools execution failed: %w", err)
		}
		state.Messages = append(state.Messages, toolMessages...)
		return state, nil
	}))

	// 分支路由：判断是否有工具调用
	graph.AddBranch("chat_node", compose.NewGraphBranch(func(ctx context.Context, state *RagState) (string, error) {
		lastMsg := state.Messages[len(state.Messages)-1]
		if len(lastMsg.ToolCalls) > 0 {
			return "tools_node", nil
		}
		return "END", nil
	}, map[string]bool{"tools_node": true, "END": true}))

	// 闭环连接：工具执行完回到模型推理
	graph.AddEdge("tools_node", "chat_node")
	graph.AddEdge(compose.START, "chat_node")

	// 编译并执行 Agent Workflow
	workflow, err := graph.Compile(ctx)
	if err != nil {
		return fmt.Errorf("failed to compile agent graph: %v", err)
	}

	_, err = workflow.Invoke(ctx, initState)
	if err != nil {
		return fmt.Errorf("agent workflow execution failed: %v", err)
	}
	// ======================================================

	// 5. 发送完成通知，并将对话保存到 Redis
	sendCompletion(ws)
	fullAnswer := answerBuilder.String()
	if len(fullAnswer) > 0 {
		bgCtx := context.Background()
		err = s.addMessageToConversation(bgCtx, user.ID, query, fullAnswer)
		if err != nil {
			log.Errorf("Failed to save conversation history: %v", err)
		}
		s.cacheService.Set(bgCtx, query, history, fullAnswer)
	}

	return nil
}

// buildPrompt 根据用户输入和搜索结果构建prompt
func (s *chatService) buildContextText(searchResults []model.SearchResponseDTO) string {
	if len(searchResults) == 0 {
		return ""
	}
	// 与 Processor 的 chunkSize 对齐，尽量不截断分块内容
	const maxSnippetLen = 1000
	var contextBuilder strings.Builder
	for i, r := range searchResults {
		snippet := r.TextContent
		if len(snippet) > maxSnippetLen {
			snippet = snippet[:maxSnippetLen] + "…"
		}
		fileLabel := r.FileName
		if fileLabel == "" {
			fileLabel = "unknown"
		}
		contextBuilder.WriteString(fmt.Sprintf("[%d] (%s) %s\n", i+1, fileLabel, snippet))
	}
	return contextBuilder.String()
}

func (s *chatService) buildSystemMessage(contextText string) string {
	// 从配置读取规则与包裹符
	// 优先使用 ai.prompt；若缺失则回退 llm.prompt
	rules := config.Conf.AI.Prompt.Rules
	if rules == "" {
		rules = config.Conf.LLM.Prompt.Rules
	}
	refStart := config.Conf.AI.Prompt.RefStart
	if refStart == "" {
		refStart = config.Conf.LLM.Prompt.RefStart
	}
	if refStart == "" {
		refStart = "<<REF>>"
	}
	refEnd := config.Conf.AI.Prompt.RefEnd
	if refEnd == "" {
		refEnd = config.Conf.LLM.Prompt.RefEnd
	}
	if refEnd == "" {
		refEnd = "<<END>>"
	}
	var sys strings.Builder
	if rules != "" {
		sys.WriteString(rules)
		sys.WriteString("\n\n")
	}
	sys.WriteString(refStart)
	sys.WriteString("\n")
	if contextText != "" {
		sys.WriteString(contextText)
	} else {
		noRes := config.Conf.AI.Prompt.NoResultText
		if noRes == "" {
			noRes = config.Conf.LLM.Prompt.NoResultText
		}
		if noRes == "" {
			noRes = "（本轮无检索结果）"
		}
		sys.WriteString(noRes)
		sys.WriteString("\n")
	}
	sys.WriteString(refEnd)
	return sys.String()
}

func (s *chatService) loadHistory(ctx context.Context, userID uint) ([]model.ChatMessage, error) {
	convID, err := s.conversationRepo.GetOrCreateConversationID(ctx, userID)
	if err != nil {
		return nil, err
	}
	return s.conversationRepo.GetConversationHistory(ctx, convID)
}

func (s *chatService) composeMessages(systemMsg string, history []model.ChatMessage, userInput string) []model.ChatMessage {
	msgs := make([]model.ChatMessage, 0, len(history)+2)
	msgs = append(msgs, model.ChatMessage{Role: "system", Content: systemMsg})
	msgs = append(msgs, history...)
	msgs = append(msgs, model.ChatMessage{Role: "user", Content: userInput})
	return msgs
}

// addMessageToConversation 是一个用于管理 Redis 中对话历史的辅助函数。
func (s *chatService) addMessageToConversation(ctx context.Context, userID uint, question, answer string) error {
	conversationID, err := s.conversationRepo.GetOrCreateConversationID(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to get or create conversation ID: %w", err)
	}

	history, err := s.conversationRepo.GetConversationHistory(ctx, conversationID)
	if err != nil {
		return fmt.Errorf("failed to get conversation history: %w", err)
	}

	// 添加用户消息
	history = append(history, model.ChatMessage{
		Role:      "user",
		Content:   question,
		Timestamp: time.Now(),
	})

	// 添加助手消息
	history = append(history, model.ChatMessage{
		Role:      "assistant",
		Content:   answer,
		Timestamp: time.Now(),
	})

	return s.conversationRepo.UpdateConversationHistory(ctx, conversationID, history)
}

// wsWriterInterceptor 是对 websocket.Conn 的封装，用于捕获写入的消息。
type wsWriterInterceptor struct {
	conn       *websocket.Conn
	writer     *strings.Builder
	shouldStop func() bool
}

// WriteMessage 满足 llm.MessageWriter 接口。
func (w *wsWriterInterceptor) WriteMessage(messageType int, data []byte) error {
	if w.shouldStop != nil && w.shouldStop() {
		// 停止标志生效：跳过下发
		return nil
	}
	w.writer.Write(data)
	// 将原始分块包装成 {"chunk":"..."}
	payload := map[string]string{"chunk": string(data)}
	b, _ := json.Marshal(payload)
	return w.conn.WriteMessage(messageType, b)
}

// sendCompletion 发送完成通知 JSON
func sendCompletion(ws *websocket.Conn) {
	notif := map[string]interface{}{
		"type":      "completion",
		"status":    "finished",
		"message":   "响应已完成",
		"timestamp": time.Now().UnixMilli(),
		"date":      time.Now().Format("2006-01-02T15:04:05"),
	}
	b, _ := json.Marshal(notif)
	_ = ws.WriteMessage(websocket.TextMessage, b)
}

func (s *chatService) buildGenerationParams() *llm.GenerationParams {
	var gp llm.GenerationParams
	if config.Conf.LLM.Generation.Temperature != 0 {
		t := config.Conf.LLM.Generation.Temperature
		gp.Temperature = &t
	}
	if config.Conf.LLM.Generation.TopP != 0 {
		p := config.Conf.LLM.Generation.TopP
		gp.TopP = &p
	}
	if config.Conf.LLM.Generation.MaxTokens != 0 {
		m := config.Conf.LLM.Generation.MaxTokens
		gp.MaxTokens = &m
	}
	if gp.Temperature == nil && gp.TopP == nil && gp.MaxTokens == nil {
		return nil
	}
	return &gp
}

// rewriteQuery 使用 LLM 将用户的后续问题改写为独立问题。
func (s *chatService) rewriteQuery(ctx context.Context, originalQuery string, history []model.ChatMessage) (string, error) {
	// 构建 Rewrite Prompt
	// 只取最近几轮对话，避免 Prompt 过长
	recentHistory := history
	if len(history) > 6 {
		recentHistory = history[len(history)-6:]
	}

	var historyText strings.Builder
	for _, msg := range recentHistory {
		role := "User"
		if msg.Role == "assistant" {
			role = "Assistant"
		}
		historyText.WriteString(fmt.Sprintf("%s: %s\n", role, msg.Content))
	}

	prompt := fmt.Sprintf(`Given the following conversation and a follow up question, rephrase the follow up question to be a standalone question.
Chat History:
%s
Follow Up Input: %s
Standalone Question:`, historyText.String(), originalQuery)

	messages := []llm.Message{
		{Role: "user", Content: prompt},
	}

	// 调用 LLM 生成 (One-shot)
	rewritten, err := s.llmClient.GenerateOneShot(ctx, messages)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(rewritten), nil
}
