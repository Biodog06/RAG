package splitter

import (
	"regexp"
	"strings"
)

// CodeSplitter 针对代码文件，尝试按函数/类定义进行语义化切分。
type CodeSplitter struct {
	extension string
}

func NewCodeSplitter(extension string) *CodeSplitter {
	return &CodeSplitter{extension: strings.ToLower(extension)}
}

// Split 尝试在保持函数完整性的前提下进行切分。
func (s *CodeSplitter) Split(text string, chunkSize int, chunkOverlap int) []string {
	if text == "" {
		return nil
	}

	// 1. 获取对应语言的函数/定义标识符正则
	pattern := s.getFunctionPattern()
	re := regexp.MustCompile(pattern)

	// 2. 按行切分，寻找边界
	lines := strings.Split(text, "\n")
	var chunks []string
	var currentChunk []string
	currentLen := 0

	for _, line := range lines {
		lineLen := len([]rune(line)) + 1 // +1 for newline character

		// 如果当前行像是新函数开始，且当前分块已经有内容，则考虑开启新分块
		if currentLen > (chunkSize/2) && re.MatchString(line) {
			chunks = append(chunks, strings.Join(currentChunk, "\n"))
			// 重叠处理：简单方案是保留前一个分块的最后几行
			// 这里我们为了保证函数完整，优先从新定义开始
			currentChunk = []string{line}
			currentLen = lineLen
			continue
		}

		// 如果超过最大长度，被迫切分
		if currentLen+lineLen > chunkSize && currentLen > 0 {
			chunks = append(chunks, strings.Join(currentChunk, "\n"))
			
			// 简单的重叠逻辑：保留最后 2 行
			overlapLines := 0
			if len(currentChunk) > 2 {
				overlapLines = 2
			} else if len(currentChunk) > 0 {
				overlapLines = 1
			}
			
			overlapContent := currentChunk[len(currentChunk)-overlapLines:]
			currentChunk = append([]string{}, overlapContent...)
			currentChunk = append(currentChunk, line)
			
			// 重算长度
			currentLen = 0
			for _, l := range currentChunk {
				currentLen += len([]rune(l)) + 1
			}
			continue
		}

		currentChunk = append(currentChunk, line)
		currentLen += lineLen
	}

	// 最后一个分块
	if len(currentChunk) > 0 {
		chunks = append(chunks, strings.Join(currentChunk, "\n"))
	}

	// 如果正则表达式没起作用导致分块过大，交给底层物理切分器兜底
	if len(chunks) == 1 && len([]rune(text)) > chunkSize {
		simple := NewSimpleTextSplitter()
		return simple.Split(text, chunkSize, chunkOverlap)
	}

	return chunks
}

func (s *CodeSplitter) getFunctionPattern() string {
	switch s.extension {
	case ".go":
		return `^(func|type|var|const)\s+`
	case ".py":
		return `^(def|class)\s+`
	case ".java", ".js", ".ts", ".cpp", ".c", ".cs":
		// C 系列语言：通常寻找行首的关键字或不缩进的函数定义
		return `^(\s*(public|private|protected|static|function|class|struct|enum|export)\s+|[a-zA-Z_][a-zA-Z0-9_*]*\s+[a-zA-Z_][a-zA-Z0-9_]*\s*\()`
	default:
		// 通用规则：尝试匹配行首的非空白字符，可能是定义开始
		return `^[a-zA-Z_]+`
	}
}
