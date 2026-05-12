// Package embedding provides a client for interacting with embedding models.
package embedding

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"pai-smart-go/internal/config"
	"pai-smart-go/pkg/log"
	"time"
)

// Client defines the interface for an embedding client.
type Client interface {
	CreateEmbedding(ctx context.Context, text string) ([]float32, error)
	// CreateEmbeddingBatch 批量向量化，减少 API 调用次数，提升处理效率。
	CreateEmbeddingBatch(ctx context.Context, texts []string) ([][]float32, error)
}

type openAICompatibleClient struct {
	cfg    config.EmbeddingConfig
	client *http.Client
}

// NewClient creates a new embedding client based on the provider in the config.
// 优化：使用连接池化的 HTTP 客户端，复用 TCP 连接。
func NewClient(cfg config.EmbeddingConfig) Client {
	return &openAICompatibleClient{
		cfg: cfg,
		client: &http.Client{
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 10,
				IdleConnTimeout:     90 * time.Second,
			},
			Timeout: 60 * time.Second,
		},
	}
}

type embeddingRequest struct {
	Model      string   `json:"model"`
	Input      []string `json:"input"`
	Dimensions int      `json:"dimensions,omitempty"`
}

type embeddingResponse struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
	} `json:"data"`
}

// CreateEmbedding calls the OpenAI-compatible API to get the vector for a given text.
func (c *openAICompatibleClient) CreateEmbedding(ctx context.Context, text string) ([]float32, error) {
	log.Infof("[EmbeddingClient] 开始调用 Embedding API, model: %s, input_len: %d", c.cfg.Model, len(text))
	vectors, err := c.CreateEmbeddingBatch(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	if len(vectors) == 0 {
		return nil, fmt.Errorf("received empty embedding from api")
	}
	return vectors[0], nil
}

// CreateEmbeddingBatch 批量调用 Embedding API，一次请求处理多个文本。
// OpenAI 兼容协议原生支持 input 为数组，因此无需多次调用。
func (c *openAICompatibleClient) CreateEmbeddingBatch(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	log.Infof("[EmbeddingClient] 开始批量调用 Embedding API, model: %s, batch_size: %d", c.cfg.Model, len(texts))

	reqBody := embeddingRequest{
		Model:      c.cfg.Model,
		Input:      texts,
		Dimensions: c.cfg.Dimensions,
	}

	reqBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal embedding request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.cfg.BaseURL+"/embeddings", bytes.NewReader(reqBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create embedding request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)

	resp, err := c.client.Do(req)
	if err != nil {
		log.Errorf("[EmbeddingClient] 批量调用 Embedding API 失败, error: %v", err)
		return nil, fmt.Errorf("failed to call embedding api: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Errorf("[EmbeddingClient] Embedding API 返回非 200 状态码: %s", resp.Status)
		return nil, fmt.Errorf("embedding api returned non-200 status: %s", resp.Status)
	}

	var embeddingResp embeddingResponse
	if err := json.NewDecoder(resp.Body).Decode(&embeddingResp); err != nil {
		log.Errorf("[EmbeddingClient] 解析 Embedding API 响应失败, error: %v", err)
		return nil, fmt.Errorf("failed to decode embedding response: %w", err)
	}

	if len(embeddingResp.Data) == 0 {
		log.Warnf("[EmbeddingClient] Embedding API 返回了空的向量数据")
		return nil, fmt.Errorf("received empty embedding from api")
	}

	vectors := make([][]float32, len(embeddingResp.Data))
	for i, d := range embeddingResp.Data {
		vectors[i] = d.Embedding
	}

	log.Infof("[EmbeddingClient] 批量 Embedding 成功, 返回 %d 个向量, 维度: %d", len(vectors), len(vectors[0]))
	return vectors, nil
}
