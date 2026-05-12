// Package rerank 提供重排序模型的客户端实现。
package rerank

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"pai-smart-go/internal/config"
	"pai-smart-go/pkg/log"
	"sort"
)

// Result 代表重排序后的单个文档结果。
type Result struct {
	Index          int     `json:"index"`           // 原始索引
	RelevanceScore float64 `json:"relevance_score"` // 相关性分数
	Text           string  `json:"-"`               // 方便后续处理，暂存文本内容 (可选)
}

// Client 定义了重排序客户端的接口。
type Client interface {
	// Rerank 对给定的文档列表进行重排序。
	// query: 用户查询
	// documents: 候选文档列表
	// 返回: 排序后的结果列表（包含分数和原始索引）
	Rerank(ctx context.Context, query string, documents []string) ([]Result, error)
}

type httpClient struct {
	cfg    config.RerankConfig
	client *http.Client
}

// NewClient 创建一个新的 Re-rank 客户端。
func NewClient(cfg config.RerankConfig) Client {
	return &httpClient{
		cfg:    cfg,
		client: &http.Client{},
	}
}

// 请求体结构 (适配阿里 DashScope)
type rerankRequest struct {
	Model string `json:"model"`
	Input struct {
		Query     string   `json:"query"`
		Documents []string `json:"documents"`
	} `json:"input"`
	TopN int `json:"top_n,omitempty"`
}

// 响应体结构
type rerankResponse struct {
	Output struct {
		Results []Result `json:"results"`
	} `json:"output"`
}

func (c *httpClient) Rerank(ctx context.Context, query string, documents []string) ([]Result, error) {
	if !c.cfg.Enable {
		// 如果未启用，返回原始顺序的伪结果
		results := make([]Result, len(documents))
		for i := range documents {
			results[i] = Result{Index: i, RelevanceScore: 1.0} // 默认满分
		}
		return results, nil
	}

	if len(documents) == 0 {
		return []Result{}, nil
	}

	log.Infof("[RerankClient] 开始调用 Rerank API, model: %s, doc_count: %d", c.cfg.Model, len(documents))

	reqBody := rerankRequest{
		Model: c.cfg.Model,
		TopN:  len(documents),
	}
	reqBody.Input.Query = query
	reqBody.Input.Documents = documents

	reqBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal rerank request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.cfg.BaseURL, bytes.NewReader(reqBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create rerank request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)

	resp, err := c.client.Do(req)
	if err != nil {
		log.Errorf("[RerankClient] 调用 Rerank API 失败, error: %v", err)
		return nil, fmt.Errorf("failed to call rerank api: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.Errorf("[RerankClient] Rerank API 返回错误 [%d]: %s", resp.StatusCode, string(body))
		return nil, fmt.Errorf("rerank api returned non-200 status: %s", resp.Status)
	}

	var rerankResp rerankResponse
	if err := json.NewDecoder(resp.Body).Decode(&rerankResp); err != nil {
		log.Errorf("[RerankClient] 解析 Rerank 响应失败: %v", err)
		return nil, fmt.Errorf("failed to decode rerank response: %w", err)
	}

	results := rerankResp.Output.Results
	// 确保结果按分数降序排列
	sort.Slice(results, func(i, j int) bool {
		return results[i].RelevanceScore > results[j].RelevanceScore
	})

	log.Infof("[RerankClient] Rerank 完成, 返回 %d 个结果", len(results))
	return results, nil
}
