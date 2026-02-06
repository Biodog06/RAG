// Package service 包含了应用的业务逻辑层。
package service

import (
	"context"
	"encoding/json"
	"fmt"
	"pai-smart-go/internal/config"
	"pai-smart-go/internal/model"
	"pai-smart-go/internal/repository"
	"pai-smart-go/pkg/llm"
	"pai-smart-go/pkg/log"
	"strings"
	"time"

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

	// Cache Check
	if cachedAnswer, hit := s.cacheService.Get(ctx, query, history); hit {
		// 如果命中缓存，模拟流式输出（或者直接一次性输出）
		// 为了前端体验一致性，我们还是稍微 delay 一下或者切片发送，这里简化为直接发送
		interceptor.WriteMessage(websocket.TextMessage, []byte(cachedAnswer))
		// 发送完成信号
		sendCompletion(ws)
		// 也要保存到历史记录
		s.addMessageToConversation(context.Background(), user.ID, query, cachedAnswer)
		return nil
	}

	searchQuery := query
	// 1. 如果有历史记录，进行查询重写 (Query Rewriting)
	// 只有当查询较短或看起来依赖上下文时才重写，但为简单起见，这里总是尝试重写
	if len(history) > 0 {
		rewritten, err := s.rewriteQuery(ctx, query, history)
		if err != nil {
			log.Warnf("[ChatService] Query rewrite failed, using original query: %v", err)
		} else {
			log.Infof("[ChatService] Query rewritten: '%s' -> '%s'", query, rewritten)
			searchQuery = rewritten
		}
	}

	// 2. 使用 SearchService 检索上下文（使用改写后的查询）
	results, err := s.searchService.HybridSearch(ctx, searchQuery, 10, user)
	if err != nil {
		return fmt.Errorf("failed to retrieve context: %w", err)
	}

	// 3. 构建上下文与 system 消息
	contextText := s.buildContextText(results)
	systemMsg := s.buildSystemMessage(contextText)
	messages := s.composeMessages(systemMsg, history, query) // 对话显示仍用原始 query，保持用户体验一致

	// 3. 调用 LLM 客户端以流式传输响应（带生成参数）
	gen := s.buildGenerationParams()
	var llmMsgs []llm.Message
	for _, m := range messages {
		llmMsgs = append(llmMsgs, llm.Message{Role: m.Role, Content: m.Content})
	}
	err = s.llmClient.StreamChatMessages(ctx, llmMsgs, gen, interceptor)
	if err != nil {
		return err
	}

	// 4. 发送完成通知，并将对话保存到 Redis
	sendCompletion(ws)
	fullAnswer := answerBuilder.String()
	if len(fullAnswer) > 0 {
		// 使用后台上下文，因为即使原始请求被取消，我们也希望保存成功生成的答案
		bgCtx := context.Background()
		err = s.addMessageToConversation(bgCtx, user.ID, query, fullAnswer)
		if err != nil {
			// 只记录错误，不返回给客户端，因为流式响应已经成功
			log.Errorf("Failed to save conversation history: %v", err)
		}
		// 保存到缓存
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
