package service

import (
	"context"
	"pai-smart-go/internal/model"
	"pai-smart-go/internal/repository"
	"pai-smart-go/pkg/embedding"
	"pai-smart-go/pkg/llm"
	"pai-smart-go/pkg/log"
	"strings"

	"github.com/gorilla/websocket"
)

type GraphChatService interface {
	StreamResponse(ctx context.Context, query string, user *model.User, ws *websocket.Conn, shouldStop func() bool) error
}

type graphChatService struct {
	*chatService
	graphService GraphSearchService
}

func NewGraphChatService(
	searchService SearchService,
	graphService GraphSearchService,
	llmClient llm.Client,
	conversationRepo repository.ConversationRepository,
	cacheService ContentCacheService,
	embeddingClient ...embedding.Client,
) GraphChatService {
	// 复用现有 ChatService 的实现逻辑
	base := NewChatService(searchService, llmClient, conversationRepo, cacheService, embeddingClient...).(*chatService)
	return &graphChatService{
		chatService:  base,
		graphService: graphService,
	}
}

func (s *graphChatService) StreamResponse(ctx context.Context, query string, user *model.User, ws *websocket.Conn, shouldStop func() bool) error {
	log.Infof("[GraphChatService] 开始图增强对话流程, query: %s", query)

	// 1. 加载对话历史
	history, err := s.loadHistory(ctx, user.ID)
	if err != nil {
		log.Errorf("Failed to load conversation history: %v", err)
		history = []model.ChatMessage{}
	}

	// 2. 多路检索：混合检索(ES) + 图检索(Neo4j)
	results, err := s.searchService.HybridSearch(ctx, query, 5, user, history) // 减少向量检索数量，为图谱腾出空间
	if err != nil {
		log.Warnf("Hybrid search failed: %v", err)
	}

	graphKnowledge, err := s.graphService.Search(ctx, query)
	if err != nil {
		log.Warnf("Graph search failed: %v", err)
	}

	// 3. 知识融合 (Knowledge Fusion)
	vectorContext := s.buildContextText(results)
	var finalContext strings.Builder
	if graphKnowledge != "" {
		finalContext.WriteString("### 来自知识图谱的相关关联：\n")
		finalContext.WriteString(graphKnowledge)
		finalContext.WriteString("\n\n")
	}
	if vectorContext != "" {
		finalContext.WriteString("### 来自文档切片的详细信息：\n")
		finalContext.WriteString(vectorContext)
	}

	// 4. 构建并发送
	systemMsg := s.buildSystemMessage(finalContext.String())
	messages := s.composeMessages(systemMsg, history, query)

	answerBuilder := &strings.Builder{}
	interceptor := &wsWriterInterceptor{conn: ws, writer: answerBuilder, shouldStop: shouldStop}

	gen := s.buildGenerationParams()
	var llmMsgs []llm.Message
	for _, m := range messages {
		llmMsgs = append(llmMsgs, llm.Message{Role: m.Role, Content: m.Content})
	}

	err = s.llmClient.StreamChatMessages(ctx, llmMsgs, gen, interceptor)
	if err != nil {
		return err
	}

	sendCompletion(ws)
	fullAnswer := answerBuilder.String()
	if len(fullAnswer) > 0 {
		_ = s.addMessageToConversation(context.Background(), user.ID, query, fullAnswer)
	}

	return nil
}
