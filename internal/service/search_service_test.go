package service

import (
	"pai-smart-go/internal/model"
	"testing"
)

func TestNormalizeQuery(t *testing.T) {
	tests := []struct {
		input      string
		expected   string
		expectPhrase string
	}{
		{"什么是RAG？", "rag", "rag"},
		{"告诉我如何使用Go语言", "使用go语言", "使用go语言"},
		{"DeepSeek的区别是什么", "deepseek", "deepseek"},
		{"", "", ""},
	}

	for _, tt := range tests {
		norm, phrase := normalizeQuery(tt.input)
		if norm != tt.expected {
			t.Errorf("normalizeQuery(%q) norm = %q, want %q", tt.input, norm, tt.expected)
		}
		if phrase != tt.expectPhrase {
			t.Errorf("normalizeQuery(%q) phrase = %q, want %q", tt.input, phrase, tt.expectPhrase)
		}
	}
}

func TestGetTopScore(t *testing.T) {
	results := []model.SearchResponseDTO{
		{Score: 0.9},
		{Score: 0.8},
	}
	if score := getTopScore(results); score != 0.9 {
		t.Errorf("getTopScore() = %f, want 0.9", score)
	}

	if score := getTopScore([]model.SearchResponseDTO{}); score != 0 {
		t.Errorf("getTopScore() empty = %f, want 0", score)
	}
}

// NOTE: HybridSearch testing requires heavy mocking due to ES client dependency.
// In a real project, we would use a mock ES client or a custom transport.
// For now, we verified the compilation and the utility functions.
