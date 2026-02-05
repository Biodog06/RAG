// Package tika 提供了一个与 Apache Tika 服务器交互的客户端。
package tika

import (
	"context"
	"fmt"
	"io"
	"pai-smart-go/internal/config"

	"github.com/google/go-tika/tika"
)

// Client 是 Tika 服务器的客户端。
type Client struct {
	client *tika.Client
}

// NewClient 创建一个新的 Tika 客户端实例。
func NewClient(cfg config.TikaConfig) *Client {
	return &Client{
		client: tika.NewClient(nil, cfg.ServerURL),
	}
}

// ExtractText 调用 Tika 提取文本。
func (c *Client) ExtractText(fileReader io.Reader, fileName string) (string, error) {
	// 使用 context.Background()，实际项目中建议传入 context
	ctx := context.Background()

	// go-tika 的 Parse 接受一个 io.Reader 并返回内容
	// 我们不需要手动猜测 Content-Type，Tika Server 会自动检测
	body, err := c.client.Parse(ctx, fileReader)
	if err != nil {
		return "", fmt.Errorf("使用 go-tika 提取文本失败: %w", err)
	}

	return body, nil
}
