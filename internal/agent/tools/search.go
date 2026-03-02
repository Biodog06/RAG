package tools

import (
	"context"
	"fmt"
	"strings"

	"pai-smart-go/pkg/log"

	"github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
)

type SearchToolInput struct {
	Query string `json:"query" jsonschema:"description=提取的核心搜索关键词汇,说明:需要检索什么样的内容"`
}

// NewSearchTool creates a new InvokableTool for searching the internal knowledge base.
func NewSearchTool(retrieverTool retriever.Retriever) (tool.InvokableTool, error) {
	return utils.InferTool("search_knowledge_base", "当遇到需要查询内部政策、指南、请假规范等非通用知识库内容时，优先调用此工具检索对应文档，提供关键词获取文档背景知识", func(ctx context.Context, input *SearchToolInput) (string, error) {
		log.Infof("[ChatService-Agent] Triggered tool search_knowledge_base, query: '%s'", input.Query)
		docs, err := retrieverTool.Retrieve(ctx, input.Query)
		if err != nil {
			return "", err
		}
		if len(docs) == 0 {
			return "没有相关的知识库文档可以参考。", nil
		}
		var res strings.Builder
		for i, d := range docs {
			res.WriteString(fmt.Sprintf("[%d] %s\n", i+1, d.Content))
		}
		return res.String(), nil
	})
}
