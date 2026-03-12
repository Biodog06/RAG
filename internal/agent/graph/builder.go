package graph

import (
	"context"
	"fmt"
	"strings"

	"pai-smart-go/internal/agent/state"
	"pai-smart-go/pkg/llm"

	einomodel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

// AgentConfig 包含了初始化智能体图层所需的依赖配置
type AgentConfig struct {
	LLMClient llm.Client
	Tools     []tool.InvokableTool
}

// InterceptorHandler 定义了外部传入处理消息流块的回调
type InterceptorHandler func(chunk string)

// CompileAgentGraph 构建并编译 Eino Agent 的连通运行图 (Workflow)
func CompileAgentGraph(ctx context.Context, config *AgentConfig, streamHandler InterceptorHandler) (compose.Runnable[*state.RagState, *state.RagState], error) {
	// 1. 构造 ToolsNode
	baseTools := make([]tool.BaseTool, 0, len(config.Tools))
	toolInfos := make([]*schema.ToolInfo, 0, len(config.Tools))

	for _, t := range config.Tools {
		baseTools = append(baseTools, t)
		info, err := t.Info(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to get tool info: %v", err)
		}
		toolInfos = append(toolInfos, info)
	}

	toolsNode, err := compose.NewToolNode(ctx, &compose.ToolsNodeConfig{
		Tools: baseTools,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create tools node: %v", err)
	}

	// 2. 初始化 Eino Graph
	graph := compose.NewGraph[*state.RagState, *state.RagState]()

	// Chat 核心推理节点
	graph.AddLambdaNode("chat_node", compose.InvokableLambda(func(ctx context.Context, ragState *state.RagState) (*state.RagState, error) {
		einoModel := config.LLMClient.AsEinoChatModel()

		streamReader, err := einoModel.Stream(ctx, ragState.Messages, einomodel.WithTools(toolInfos))
		if err != nil {
			return nil, fmt.Errorf("llm stream failed: %w", err)
		}
		defer streamReader.Close()

		var isToolCall bool
		var fullMessage *schema.Message

		for {
			chunk, err := streamReader.Recv()
			if err != nil {
				if err.Error() == "EOF" || strings.Contains(err.Error(), "EOF") {
					break
				}
				break
			}

			if len(chunk.ToolCalls) > 0 {
				isToolCall = true
			}

			// Streaming chunk content to the user through external handler if it's not a tool call
			if !isToolCall && chunk.Content != "" && streamHandler != nil {
				streamHandler(chunk.Content)
			}

			if fullMessage == nil {
				fullMessage = chunk
			} else {
				fullMessage, _ = schema.ConcatMessages([]*schema.Message{fullMessage, chunk})
			}
		}

		if fullMessage != nil {
			ragState.Messages = append(ragState.Messages, fullMessage)
		}
		return ragState, nil
	}))

	// 工具执行节点
	graph.AddLambdaNode("tools_node", compose.InvokableLambda(func(ctx context.Context, ragState *state.RagState) (*state.RagState, error) {
		lastMsg := ragState.Messages[len(ragState.Messages)-1]
		toolMessages, err := toolsNode.Invoke(ctx, lastMsg)
		if err != nil {
			return nil, fmt.Errorf("tools execution failed: %w", err)
		}
		ragState.Messages = append(ragState.Messages, toolMessages...)
		return ragState, nil
	}))

	graph.AddLambdaNode("final_node", compose.InvokableLambda(func(ctx context.Context, ragState *state.RagState) (*state.RagState, error) {
		return ragState, nil
	}))

	// 分支路由：判断是否有工具调用
	graph.AddBranch("chat_node", compose.NewGraphBranch(func(ctx context.Context, ragState *state.RagState) (string, error) {
		lastMsg := ragState.Messages[len(ragState.Messages)-1]
		if len(lastMsg.ToolCalls) > 0 {
			return "tools_node", nil
		}
		return "final_node", nil
	}, map[string]bool{"tools_node": true, "final_node": true}))

	// 闭环连接：工具执行完回到模型推理
	graph.AddEdge("tools_node", "chat_node")
	graph.AddEdge("final_node", compose.END)
	graph.AddEdge(compose.START, "chat_node")

	// 编译 Workflow
	workflow, err := graph.Compile(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to compile agent graph: %v", err)
	}

	return workflow, nil
}
