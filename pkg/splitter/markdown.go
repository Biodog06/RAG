package splitter

import (
	"fmt"
	"regexp"
	"strings"
)

// MarkdownSplitter implements a header-aware markdown splitter.
// It tries to keep sections together and preserves header context.
type MarkdownSplitter struct {
	baseSplitter TextSplitter
}

func NewMarkdownSplitter() *MarkdownSplitter {
	return &MarkdownSplitter{
		baseSplitter: NewSimpleTextSplitter(),
	}
}

// Split splits markdown text, respecting headers and hierarchical structure.
func (s *MarkdownSplitter) Split(text string, chunkSize int, chunkOverlap int) []string {
	// 1. Split by headers first to get logical sections
	sections := s.splitByHeaders(text)

	var finalChunks []string

	for _, section := range sections {
		// If section is small enough, keep it as is
		if len([]rune(section.Content)) <= chunkSize {
			// Add partial context if needed, but for now just the content
			// which already includes the header line itself
			finalChunks = append(finalChunks, s.formatChunk(section))
			continue
		}

		// If section is too large, use base splitter but prepend context to each chunk
		subChunks := s.baseSplitter.Split(section.Body, chunkSize, chunkOverlap)
		for _, subChunk := range subChunks {
			// Combine Context + Header + SubChunk
			// We format it like: "Title > SubTitle\n\n Content..."
			contextStr := s.buildContextString(section.Headers)
			combined := fmt.Sprintf("%s\n\n%s", contextStr, subChunk)
			finalChunks = append(finalChunks, combined)
		}
	}

	return finalChunks
}

type mdSection struct {
	Headers []string // Hierarchy of headers: ["# Title", "## SubTitle"]
	Content string   // Full content of section including its own header
	Body    string   // Content without the header line
}

// splitByHeaders parses the markdown and divides it into sections based on headers.
// This is a simplified implementation.
func (s *MarkdownSplitter) splitByHeaders(text string) []mdSection {
	lines := strings.Split(text, "\n")
	var sections []mdSection

	headerStack := make([]string, 0) // Stores current header hierarchy
	var currentSectionBuf strings.Builder
	var currentBodyBuf strings.Builder

	headerRegex := regexp.MustCompile(`^(#{1,6})\s+(.*)`)

	flushSection := func() {
		if currentSectionBuf.Len() > 0 {
			// Save the previous section
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
			// Found a header
			flushSection()

			// Update stack
			level := len(matches[1])
			// headerText := matches[2]
			fullHeader := matches[0]

			// Adjust stack: remove deeper or equal levels
			// (Simple logic: if we see ##, remove any previous ##, ###, etc. keep #)
			// Wait, if we use just level, we might simplify.
			// Stack: ["# H1", "## H2"] -> incoming "## New H2" -> pop "## H2" -> push "## New H2"
			// Stack: ["# H1", "## H2"] -> incoming "# New H1" -> pop all -> push "# New H1"

			// We rebuild stack based on level
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
			// Body doesn't include the header line for the *current* section's new header
			// BUT, for semantic understanding, the header is part of the content.
			// However, when we sub-split, we want to prepend context manually.
		} else {
			currentSectionBuf.WriteString(line + "\n")
			currentBodyBuf.WriteString(line + "\n")
		}
	}
	flushSection() // Flush last section

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
	// Clean headers (remove #) and join with " > "
	var clean []string
	re := regexp.MustCompile(`^#{1,6}\s+`)
	for _, h := range headers {
		clean = append(clean, re.ReplaceAllString(h, ""))
	}
	return strings.Join(clean, " > ")
}

func (s *MarkdownSplitter) formatChunk(section mdSection) string {
	// For small sections, we might still want to prepend the full context if it's missing
	// form the section content itself (e.g. if section only has the last header)
	// But section.Content usually contains the last header.
	// So we might prepend only the *parent* headers.

	if len(section.Headers) <= 1 {
		return section.Content
	}

	// Parents are all except the last one
	parents := section.Headers[:len(section.Headers)-1]
	context := s.buildContextString(parents)

	if context == "" {
		return section.Content
	}

	return fmt.Sprintf("[%s]\n%s", context, section.Content)
}
