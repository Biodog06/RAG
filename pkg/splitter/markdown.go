package splitter

import (
	"fmt"
	"regexp"
	"strings"
)

// MarkdownSplitter 实现了具有标题感知能力的 Markdown 切分器。
// 它尝试保持章节完整性并保留标题上下文。
type MarkdownSplitter struct {
	baseSplitter TextSplitter
}

func NewMarkdownSplitter() *MarkdownSplitter {
	return &MarkdownSplitter{
		baseSplitter: NewSimpleTextSplitter(),
	}
}

// Split 切分 Markdown 文本，尊崇标题和层级结构。
func (s *MarkdownSplitter) Split(text string, chunkSize int, chunkOverlap int) []string {
	// 1. 首先按标题切分以获取逻辑章节
	sections := s.splitByHeaders(text)

	var finalChunks []string

	for _, section := range sections {
		// 如果章节足够小，直接作为一个块
		if len([]rune(section.Content)) <= chunkSize {
			// 添加部分上下文，但目前主要包含内容本身
			// 内容本身已经包含了标题行
			finalChunks = append(finalChunks, s.formatChunk(section))
			continue
		}

		// 如果章节过大，使用基础切分器，但为每个分块预置上下文
		subChunks := s.baseSplitter.Split(section.Body, chunkSize, chunkOverlap)
		for _, subChunk := range subChunks {
			// 组合方式: 上下文 + 分隔 + 子块
			// 格式示例: "标题 > 子标题\n\n 内容..."
			contextStr := s.buildContextString(section.Headers)
			combined := fmt.Sprintf("%s\n\n%s", contextStr, subChunk)
			finalChunks = append(finalChunks, combined)
		}
	}

	return finalChunks
}

type mdSection struct {
	Headers []string // 标题层级栈: ["# 标题", "## 子标题"]
	Content string   // 章节完整内容（包含自身标题）
	Body    string   // 不包含自身标题行的内容体
}

// splitByHeaders 解析 Markdown 并根据标题将其划分为章节。
// 这是一个简化实现。
func (s *MarkdownSplitter) splitByHeaders(text string) []mdSection {
	lines := strings.Split(text, "\n")
	var sections []mdSection

	headerStack := make([]string, 0) // 存储当前的标题层级
	var currentSectionBuf strings.Builder
	var currentBodyBuf strings.Builder

	headerRegex := regexp.MustCompile(`^(#{1,6})\s+(.*)`)

	flushSection := func() {
		if currentSectionBuf.Len() > 0 {
			// 保存前一个章节
			sections = append(sections, mdSection{
				Headers: copyStack(headerStack),
				Content: currentSectionBuf.String(),
				Body:    currentBodyBuf.String(),
			})
			currentSectionBuf.Reset()
			currentBodyBuf.Reset()
		}
	}

	for _, line := range lines {
		matches := headerRegex.FindStringSubmatch(line)
		if len(matches) > 0 {
			// 发现一个标题
			flushSection()

			// 更新栈
			level := len(matches[1])
			// headerText := matches[2]
			fullHeader := matches[0]

			// 调整栈：移除更深或同级的标题
			// (简单逻辑：如果遇到 ##，移除之前所有的 ##, ### 等，保留 #)
			// 栈操作逻辑：["# H1", "## H2"] -> 遇到 "## New H2" -> 弹出 "## H2" -> 压入 "## New H2"

			// 根据级别重建栈
			newStack := make([]string, 0)
			for _, h := range headerStack {
				currentLevel := getHeaderLevel(h)
				if currentLevel < level {
					newStack = append(newStack, h)
				} else {
					break
				}
			}
			newStack = append(newStack, fullHeader)
			headerStack = newStack

			currentSectionBuf.WriteString(line + "\n")
			// Body 不包含当前章节的新标题行
			// 但为了语义理解，标题其实是内容的一部分。
			// 不过当我们进行二次切分时，会手动前置上下文。
		} else {
			currentSectionBuf.WriteString(line + "\n")
			currentBodyBuf.WriteString(line + "\n")
		}
	}
	flushSection() // 刷新最后一个章节

	return sections
}

func getHeaderLevel(headerLine string) int {
	for i, r := range headerLine {
		if r != '#' {
			return i
		}
	}
	return 0
}

func copyStack(s []string) []string {
	c := make([]string, len(s))
	copy(c, s)
	return c
}

func (s *MarkdownSplitter) buildContextString(headers []string) string {
	// 清理标题（移除 #）并用 " > " 连接
	var clean []string
	re := regexp.MustCompile(`^#{1,6}\s+`)
	for _, h := range headers {
		clean = append(clean, re.ReplaceAllString(h, ""))
	}
	return strings.Join(clean, " > ")
}

func (s *MarkdownSplitter) formatChunk(section mdSection) string {
	// 对于小章节，如果其内容本身缺失上下文（例如只包含末级标题），
	// 我们可能仍希望补全完整上下文。
	// 通常 section.Content 包含了末级标题。
	// 所以我们只补全 *父级* 标题。

	if len(section.Headers) <= 1 {
		return section.Content
	}

	// 父级是除了最后一个之外的所有标题
	parents := section.Headers[:len(section.Headers)-1]
	context := s.buildContextString(parents)

	if context == "" {
		return section.Content
	}

	return fmt.Sprintf("[%s]\n%s", context, section.Content)
}
