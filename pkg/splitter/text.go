package splitter

// TextSplitter defines the interface for text splitting strategies.
type TextSplitter interface {
	Split(text string, chunkSize int, chunkOverlap int) []string
}

// SimpleTextSplitter implements a basic character-based sliding window splitter.
type SimpleTextSplitter struct{}

func NewSimpleTextSplitter() *SimpleTextSplitter {
	return &SimpleTextSplitter{}
}

// Split splits text into chunks of specified size with overlap.
func (s *SimpleTextSplitter) Split(text string, chunkSize int, chunkOverlap int) []string {
	if chunkSize <= chunkOverlap {
		return s.simpleSplit(text, chunkSize)
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

func (s *SimpleTextSplitter) simpleSplit(text string, chunkSize int) []string {
	var chunks []string
	runes := []rune(text)
	if len(runes) == 0 {
		return nil
	}
	for i := 0; i < len(runes); i += chunkSize {
		end := i + chunkSize
		if end > len(runes) {
			end = len(runes)
		}
		chunks = append(chunks, string(runes[i:end]))
	}
	return chunks
}
