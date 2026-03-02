// Package embedding provides a client for interacting with embedding models.
package embedding

import (
	"context"
	"fmt"
	"pai-smart-go/internal/config"
	"pai-smart-go/pkg/log"

	"github.com/cloudwego/eino-ext/components/embedding/openai"
	"github.com/cloudwego/eino/components/embedding"
)

// Client defines the interface for an embedding client.
type Client interface {
	CreateEmbedding(ctx context.Context, text string) ([]float32, error)
}

type einoEmbeddingClient struct {
	cfg      config.EmbeddingConfig
	embedder embedding.Embedder
}

// NewClient creates a new embedding client based on the provider in the config.
func NewClient(cfg config.EmbeddingConfig) Client {
	embedder, err := openai.NewEmbedder(context.Background(), &openai.EmbeddingConfig{
		BaseURL:    cfg.BaseURL,
		APIKey:     cfg.APIKey,
		Model:      cfg.Model,
		Dimensions: &cfg.Dimensions,
	})
	if err != nil {
		panic(fmt.Sprintf("failed to initialize eino embedder: %v", err))
	}

	return &einoEmbeddingClient{
		cfg:      cfg,
		embedder: embedder,
	}
}

// CreateEmbedding calls the Eino API to get the vector for a given text.
func (c *einoEmbeddingClient) CreateEmbedding(ctx context.Context, text string) ([]float32, error) {
	log.Infof("[EmbeddingClient] 开始调用 Embedding API, model: %s, input_len: %d", c.cfg.Model, len(text))

	// Eino 的 Embedder 接受字符串数组并返回浮点数二维数组
	embeddings, err := c.embedder.EmbedStrings(ctx, []string{text})
	if err != nil {
		log.Errorf("[EmbeddingClient] 调用 Embedding API 失败, error: %v", err)
		return nil, fmt.Errorf("failed to call eino embedder: %w", err)
	}

	if len(embeddings) == 0 || len(embeddings[0]) == 0 {
		log.Warnf("[EmbeddingClient] Embedding API 返回了空的向量数据")
		return nil, fmt.Errorf("received empty embedding from api")
	}

	// 将返回的 []float64 转换为 []float32
	// 针对不同 Eino 版本的兼容，使用浮点降级
	result := make([]float32, len(embeddings[0]))
	for i, v := range embeddings[0] {
		result[i] = float32(v)
	}

	log.Infof("[EmbeddingClient] 成功从 Embedding API 获取向量, 维度: %d", len(result))
	return result, nil
}
