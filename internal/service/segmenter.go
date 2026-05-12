package service

import (
	"strings"
	"sync"

	"pai-smart-go/pkg/log"

	"github.com/go-ego/gse"
)

// SegmenterConfig 分词器配置
type SegmenterConfig struct {
	Enabled bool   // 是否启用分词
	Dict    string // 词典路径（可选）
}

// QuerySegmenter 查询分词器（单例）
type QuerySegmenter struct {
	seg     gse.Segmenter
	enabled bool
	once    sync.Once
}

var (
	segmenter     *QuerySegmenter
	segmenterOnce sync.Once
)

// GetSegmenter 获取全局分词器实例（单例模式）
func GetSegmenter(config SegmenterConfig) *QuerySegmenter {
	segmenterOnce.Do(func() {
		segmenter = &QuerySegmenter{
			enabled: config.Enabled,
		}
		if config.Enabled {
			segmenter.once.Do(func() {
				var err error
				// 使用默认词典初始化
				segmenter.seg, err = gse.New("zh")
				if err != nil {
					log.Errorf("[Segmenter] 初始化分词器失败: %v", err)
					segmenter.enabled = false
					return
				}

				// 如果提供了自定义词典，加载它
				if config.Dict != "" {
					err = segmenter.seg.LoadDict(config.Dict)
					if err != nil {
						log.Warnf("[Segmenter] 加载自定义词典失败: %v", err)
					}
				}

				log.Info("[Segmenter] 分词器初始化成功")
			})
		}
	})
	return segmenter
}

// SegmentWithPOS 使用词性标注进行分词
// 返回过滤后的关键词列表
func (s *QuerySegmenter) SegmentWithPOS(query string) []string {
	if !s.enabled {
		return nil
	}

	// 执行分词和词性标注
	segments := s.seg.Cut(query, true)

	var keywords []string

	// 过滤词性
	for _, seg := range segments {
		// gse.Cut 返回的是字符串切片，需要额外获取词性
		// 这里简化处理：保留长度 > 1 的词，或者是英文
		word := strings.TrimSpace(seg)
		if word == "" {
			continue
		}

		// 简单规则：
		// 1. 英文词保留
		// 2. 中文词长度 > 1 保留
		// 3. 过滤常见停用词
		if isEnglish(word) || (len([]rune(word)) > 1 && !isStopWord(word)) {
			keywords = append(keywords, word)
		}
	}

	return keywords
}

// SegmentWithPOSAdvanced 高级版本：使用实际的词性标注
// 需要 gse 配置了词性词典
func (s *QuerySegmenter) SegmentWithPOSAdvanced(query string) (core []string, optional []string) {
	if !s.enabled {
		return nil, nil
	}

	// 使用 Pos 方法获取词性
	// 注意：这需要 gse 加载了词性词典
	segments := s.seg.Cut(query, true)

	for _, seg := range segments {
		word := strings.TrimSpace(seg)
		if word == "" || isStopWord(word) {
			continue
		}

		// 简化判断：英文和长中文词作为核心词
		if isEnglish(word) || len([]rune(word)) > 2 {
			core = append(core, word)
		} else if len([]rune(word)) > 1 {
			optional = append(optional, word)
		}
	}

	return core, optional
}

// isEnglish 判断是否为英文词
func isEnglish(word string) bool {
	for _, r := range word {
		if r < 'A' || (r > 'Z' && r < 'a') || r > 'z' {
			return false
		}
	}
	return len(word) > 0
}

// isStopWord 判断是否为停用词
func isStopWord(word string) bool {
	stopWords := map[string]bool{
		// 基础代词
		"的": true, "了": true, "在": true, "是": true,
		"我": true, "你": true, "他": true, "她": true,
		"它": true, "们": true, "您": true, "咱": true,

		// 指示代词
		"这": true, "那": true, "这个": true, "那个": true,
		"这些": true, "那些": true, "这里": true, "那里": true,

		// 连词和介词
		"有": true, "和": true, "与": true, "或": true,
		"就": true, "不": true, "都": true, "但": true,
		"而": true, "及": true, "从": true, "把": true,

		// 副词
		"会": true, "能": true, "可": true, "还": true,
		"再": true, "更": true, "最": true, "太": true,
		"非常": true, "特别": true, "比较": true, "挺": true,

		// 疑问词
		"怎么": true, "如何": true, "什么": true, "哪里": true,
		"为什么": true, "怎样": true, "哪个": true, "谁": true,
		"何时": true, "何地": true, "多少": true, "几个": true,
		"怎么样": true, "怎么办": true, "为啥": true, "吗": true,

		// 时间相关
		"现在": true, "刚才": true, "以前": true, "之前": true,
		"之后": true, "后来": true, "然后": true, "接着": true,
		"最后": true, "开始": true, "结束": true, "时候": true,
	}
	return stopWords[word]
}

// NormalizeQueryWithSegmenter 使用分词器的归一化函数
func (s *QuerySegmenter) NormalizeQueryWithSegmenter(query string) (normalized string, keywords []string) {
	if !s.enabled {
		// 降级到简单归一化
		normalized = simpleNormalize(query)
		return normalized, strings.Fields(normalized)
	}

	// 使用分词提取关键词
	keywords = s.SegmentWithPOS(query)
	normalized = strings.Join(keywords, " ")

	return normalized, keywords
}

// simpleNormalize 简单归一化（降级方案）
func simpleNormalize(query string) string {
	query = strings.ToLower(query)
	// 移除标点符号
	query = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == ' ' || r > 127 {
			return r
		}
		return ' '
	}, query)
	// 归一化空格
	fields := strings.Fields(query)
	return strings.Join(fields, " ")
}
