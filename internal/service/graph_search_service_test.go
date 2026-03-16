package service

import (
	"context"
	"pai-smart-go/pkg/llm"
	"strings"
	"testing"
)

// MockLLMClient is a mock of llm.Client
type MockLLMClient struct {
	GenerateOneShotFunc func(ctx context.Context, messages []llm.Message) (string, error)
	StreamChatMessagesFunc func(ctx context.Context, messages []llm.Message, gen *llm.GenerationParams, writer llm.MessageWriter) error
}

func (m *MockLLMClient) StreamChatMessages(ctx context.Context, messages []llm.Message, gen *llm.GenerationParams, writer llm.MessageWriter) error {
	if m.StreamChatMessagesFunc != nil {
		return m.StreamChatMessagesFunc(ctx, messages, gen, writer)
	}
	return nil
}

func (m *MockLLMClient) StreamChat(ctx context.Context, prompt string, writer llm.MessageWriter) error {
	return m.StreamChatMessages(ctx, []llm.Message{{Role: "user", Content: prompt}}, nil, writer)
}

func (m *MockLLMClient) GenerateOneShot(ctx context.Context, messages []llm.Message) (string, error) {
	if m.GenerateOneShotFunc != nil {
		return m.GenerateOneShotFunc(ctx, messages)
	}
	return "", nil
}

func TestExtractEntities(t *testing.T) {
	mockLLM := &MockLLMClient{
		GenerateOneShotFunc: func(ctx context.Context, messages []llm.Message) (string, error) {
			return "实体1, 实体2, 产品A", nil
		},
	}

	s := &graphSearchService{llmClient: mockLLM}
	entities, err := s.extractEntities(context.Background(), "帮我查查产品A的信息")
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	expected := []string{"实体1", "实体2", "产品A"}
	if len(entities) != len(expected) {
		t.Fatalf("Expected %d entities, got %d", len(expected), len(entities))
	}
	for i, v := range entities {
		if v != expected[i] {
			t.Errorf("Expected %s, got %s at index %d", expected[i], v, i)
		}
	}
}

func TestFormatGraphKnowledge(t *testing.T) {
	s := &graphSearchService{}
	results := []GraphResult{
		{Subject: "A", Predicate: "Knows", Object: "B"},
		{Subject: "B", Predicate: "Works", Object: "C"},
	}

	formatted := s.formatGraphKnowledge(results)
	if !strings.Contains(formatted, "A --(Knows)--> B") || !strings.Contains(formatted, "B --(Works)--> C") {
		t.Errorf("Formatted string incorrect: %s", formatted)
	}
}
