package tools

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"pai-smart-go/pkg/log"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
)

type WebSearchInput struct {
	Query string `json:"query" jsonschema:"description=需要提取给搜索引擎的搜索关键词汇，如特朗普的最新新闻"`
}

// NewWebSearchTool creates a new InvokableTool that searches DuckDuckGo.
func NewWebSearchTool() (tool.InvokableTool, error) {
	return utils.InferTool("get_web_search", "当且仅当遇到外部世界通用知识、近期新闻、实时股票等突发事件等开放性问题，请务必独立调用此工具联网获取最新信息", func(ctx context.Context, input *WebSearchInput) (string, error) {
		log.Infof("[AgentTool-WebSearch] Triggered DuckDuckGo search, query: '%s'", input.Query)

		// 构造并编码查询目标
		targetURL := fmt.Sprintf("https://html.duckduckgo.com/html/?q=%s", url.QueryEscape(input.Query))
		req, err := http.NewRequestWithContext(ctx, "GET", targetURL, nil)
		if err != nil {
			return "", fmt.Errorf("failed to create request: %v", err)
		}

		// DuckDuckGo 对裸 http 客户端可能会有限制，必须伪装一下 User-Agent
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36")

		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			return "", fmt.Errorf("failed to fetch duckduckgo: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != 200 {
			return "", fmt.Errorf("duckduckgo returned non-200 status: %d", resp.StatusCode)
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return "", fmt.Errorf("failed to read response: %v", err)
		}

		// 简单的正则提取搜索结果摘要片段 (针对 html.duckduckgo.com 特定类名)
		htmlContent := string(body)
		re := regexp.MustCompile(`class="result__snippet[^>]*>(.*?)</a>`)
		matches := re.FindAllStringSubmatch(htmlContent, 5) // 获取前5条摘要

		if len(matches) == 0 {
			return "搜索完成，但在互联网上没有找到相关的总结摘要信息。", nil
		}

		var b strings.Builder
		for i, match := range matches {
			if len(match) > 1 {
				// 简单的清洗一下内部残余的标签元素
				cleanText := regexp.MustCompile(`<[^>]*>`).ReplaceAllString(match[1], "")
				b.WriteString(fmt.Sprintf("[%d] %s\n", i+1, strings.TrimSpace(cleanText)))
			}
		}

		return b.String(), nil
	})
}
