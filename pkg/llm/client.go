// Package llm provides a client for interacting with Large Language Models.
package llm

import (
	"context"
	"fmt"
	"io"
	"pai-smart-go/internal/config"

	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"github.com/gorilla/websocket"
)

// MessageWriter defines an interface for writing WebSocket messages.
// This allows both a standard websocket.Conn and our interceptor to be used.
type MessageWriter interface {
	WriteMessage(messageType int, data []byte) error
}

// Client defines the interface for an LLM client.
type Client interface {
	// StreamChatMessages 以 role-based 消息与可选生成参数调用聊天接口，并将流式分块写入 writer。
	StreamChatMessages(ctx context.Context, messages []Message, gen *GenerationParams, writer MessageWriter) error
	// GenerateOneShot 执行非流式调用，直接返回完整生成的文本。
	GenerateOneShot(ctx context.Context, messages []Message) (string, error)
	// 为兼容旧调用，保留 StreamChat：由内部包装为 messages 调用。
	StreamChat(ctx context.Context, prompt string, writer MessageWriter) error
	// AsEinoChatModel 返回底层的 Eino ChatModel，以便接入 Eino Graph 编排。
	AsEinoChatModel() model.ChatModel
}

type einoClient struct {
	cfg       config.LLMConfig
	chatModel model.ChatModel
}

func (c *einoClient) AsEinoChatModel() model.ChatModel {
	return c.chatModel
}

// NewClient creates a new LLM client based on the provider in the config.
func NewClient(cfg config.LLMConfig) Client {
	chatModel, err := openai.NewChatModel(context.Background(), &openai.ChatModelConfig{
		BaseURL: cfg.BaseURL,
		APIKey:  cfg.APIKey,
		Model:   cfg.Model,
	})
	if err != nil {
		panic(fmt.Sprintf("failed to initialize eino chat model: %v", err))
	}

	return &einoClient{
		cfg:       cfg,
		chatModel: chatModel,
	}
}

// Message 表示一条角色消息
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// GenerationParams 控制生成行为
type GenerationParams struct {
	Temperature *float64
	TopP        *float64
	MaxTokens   *int
}

// GenerateOneShot calls the API for a non-streaming completion.
func (c *einoClient) GenerateOneShot(ctx context.Context, messages []Message) (string, error) {
	einoMsgs, err := convertMessages(messages)
	if err != nil {
		return "", err
	}

	opts := c.buildOptions(nil)
	resp, err := c.chatModel.Generate(ctx, einoMsgs, opts...)
	if err != nil {
		return "", fmt.Errorf("eino GenerateOneShot failed: %w", err)
	}
	if resp == nil {
		return "", fmt.Errorf("empty choice in response")
	}

	return resp.Content, nil
}

// StreamChat calls the API for chat completions and streams the response.
func (c *einoClient) StreamChat(ctx context.Context, prompt string, writer MessageWriter) error {
	return c.StreamChatMessages(ctx, []Message{{Role: "user", Content: prompt}}, nil, writer)
}

func (c *einoClient) StreamChatMessages(ctx context.Context, messages []Message, gen *GenerationParams, writer MessageWriter) error {
	einoMsgs, err := convertMessages(messages)
	if err != nil {
		return err
	}

	opts := c.buildOptions(gen)

	streamReader, err := c.chatModel.Stream(ctx, einoMsgs, opts...)
	if err != nil {
		return fmt.Errorf("eino StreamChat failed: %w", err)
	}
	defer streamReader.Close()

	for {
		chunk, err := streamReader.Recv()
		if err != nil {
			if err == io.EOF {
				break
			}
			return fmt.Errorf("failed to read from eino stream: %w", err)
		}
		if chunk != nil && chunk.Content != "" {
			if err := writer.WriteMessage(websocket.TextMessage, []byte(chunk.Content)); err != nil {
				return fmt.Errorf("failed to write message to websocket: %w", err)
			}
		}
	}
	return nil
}

func convertMessages(msgs []Message) ([]*schema.Message, error) {
	var einoMsgs []*schema.Message
	for _, m := range msgs {
		role := schema.User
		switch m.Role {
		case "user":
			role = schema.User
		case "assistant":
			role = schema.Assistant
		case "system":
			role = schema.System
		}
		einoMsgs = append(einoMsgs, &schema.Message{
			Role:    role,
			Content: m.Content,
		})
	}
	return einoMsgs, nil
}

func (c *einoClient) buildOptions(gen *GenerationParams) []model.Option {
	var opts []model.Option
	if gen != nil {
		if gen.Temperature != nil {
			opts = append(opts, model.WithTemperature(float32(*gen.Temperature)))
		}
		if gen.TopP != nil {
			opts = append(opts, model.WithTopP(float32(*gen.TopP)))
		}
		if gen.MaxTokens != nil {
			opts = append(opts, model.WithMaxTokens(*gen.MaxTokens))
		}
	} else {
		// 从全局配置注入（若非零值）
		if c.cfg.Generation.Temperature != 0 {
			opts = append(opts, model.WithTemperature(float32(c.cfg.Generation.Temperature)))
		}
		if c.cfg.Generation.TopP != 0 {
			opts = append(opts, model.WithTopP(float32(c.cfg.Generation.TopP)))
		}
		if c.cfg.Generation.MaxTokens != 0 {
			opts = append(opts, model.WithMaxTokens(c.cfg.Generation.MaxTokens))
		}
	}
	return opts
}
