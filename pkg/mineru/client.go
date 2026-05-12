package mineru

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Client 是针对 MinerU gRPC 服务的客户端封装
type Client struct {
	conn   *grpc.ClientConn
	client DocumentIntelligenceClient // 这是一个预期由 protoc 生成的接口
}

// Config 存储 MinerU 客户端配置
type Config struct {
	Endpoint string
	Timeout  time.Duration
}

// NewClient 创建一个新的 MinerU 客户端
func NewClient(cfg Config) (*Client, error) {
	if cfg.Timeout == 0 {
		cfg.Timeout = 60 * time.Second // 默认 60s 超时
	}

	// 建立不安全的 gRPC 连接（内网环境建议如此，外网需配证书）
	conn, err := grpc.Dial(cfg.Endpoint, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("无法连接到 MinerU 服务: %w", err)
	}

	return &Client{
		conn:   conn,
		client: NewDocumentIntelligenceClient(conn), // 预期生成的构造函数
	}, nil
}

// Parse 调用远程 MinerU 服务解析文档
func (c *Client) Parse(ctx context.Context, fileName string, content []byte) (string, error) {
	req := &ParseRequest{
		FileName:    fileName,
		FileContent: content,
		ForceOcr:    false,
	}

	// 执行调用
	resp, err := c.client.ParseDocument(ctx, req)
	if err != nil {
		return "", fmt.Errorf("调用 MinerU gRPC 失败: %w", err)
	}

	if resp.ErrorMsg != "" {
		return "", fmt.Errorf("MinerU 解析服务逻辑错误: %s", resp.ErrorMsg)
	}

	return resp.Markdown, nil
}

// Close 关闭底层连接
func (c *Client) Close() error {
	return c.conn.Close()
}

// 注意：以上代码中 DocumentIntelligenceClient, ParseRequest, NewDocumentIntelligenceClient 等
// 均依赖于执行如下命令生成的第三方代码：
// protoc --go_out=. --go-grpc_out=. api/proto/mineru/mineru.proto
