package splitter

// TextSplitter 定义了文本切分的通用接口。
type TextSplitter interface {
	Split(text string, chunkSize int, chunkOverlap int) []string
}

// SimpleTextSplitter 实现了基础的字符级切分逻辑。
type SimpleTextSplitter struct{}

func NewSimpleTextSplitter() *SimpleTextSplitter {
	return &SimpleTextSplitter{}
}

// Split 将长文本按指定大小和重叠进行切分。
func (s *SimpleTextSplitter) Split(text string, chunkSize int, chunkOverlap int) []string {
	if chunkSize <= 0 {
		return nil
	}
	if chunkSize <= chunkOverlap {
		chunkOverlap = 0 // 避免死循环或无效切分
	}

	var chunks []string
	runes := []rune(text)
	if len(runes) == 0 {
		return nil
	}

	step := chunkSize - chunkOverlap
	for i := 0; i < len(runes); i += step {
		end := i + chunkSize
		if end > len(runes) {
			end = len(runes)
		}
		chunks = append(chunks, string(runes[i:end]))
		if end == len(runes) {
			break
		}
	}
	return chunks
}
