# Segmenter 调用流程 - 快速参考

## 一图看懂调用流程

```
┌─────────────────────────────────────────────────────────────┐
│                    系统启动 (main.go)                        │
│  1. 加载 config.yaml                                         │
│  2. 初始化 SearchService(cfg.Segmenter)                     │
│     └─▶ GetSegmenter() 创建单例分词器                       │
│         └─▶ gse.New("zh") 加载中文词典                      │
└─────────────────────────────────────────────────────────────┘
                            ↓
┌─────────────────────────────────────────────────────────────┐
│              用户发起查询 (chat_service.go)                  │
│  StreamResponse("请问怎么在 Ubuntu 上安装 Docker？")         │
│     └─▶ searchService.HybridSearch()                        │
└─────────────────────────────────────────────────────────────┘
                            ↓
┌─────────────────────────────────────────────────────────────┐
│          关键词提取 (search_service.go)                      │
│  HybridSearch()                                              │
│     ├─▶ 第一层: extractKeywords() [LLM]                     │
│     │   └─▶ 成功 → 返回关键词                               │
│     │   └─▶ 失败 ↓                                          │
│     │                                                         │
│     ├─▶ 第二层: fallbackKeywordExtraction()                 │
│     │   └─▶ segmenter.SegmentWithPOSAdvanced() ← 这里！     │
│     │       └─▶ 成功 → 返回关键词                           │
│     │       └─▶ 失败 ↓                                      │
│     │                                                         │
│     └─▶ 第三层: 简单分词 strings.Fields()                   │
│         └─▶ 保证返回                                        │
└─────────────────────────────────────────────────────────────┘
                            ↓
┌─────────────────────────────────────────────────────────────┐
│          分词处理 (segmenter.go)                             │
│  SegmentWithPOSAdvanced("请问怎么在 Ubuntu 上安装 Docker？")│
│                                                              │
│  Step 1: gse.Cut() 分词                                     │
│     输入: "请问怎么在 Ubuntu 上安装 Docker？"               │
│     输出: [请问, 怎么, 在, Ubuntu, 上, 安装, Docker, ？]   │
│                                                              │
│  Step 2: 遍历 + 过滤停用词                                  │
│     请问   → isStopWord() = true  → ❌ 跳过                 │
│     怎么   → isStopWord() = true  → ❌ 跳过                 │
│     在     → isStopWord() = true  → ❌ 跳过                 │
│     Ubuntu → isEnglish() = true   → ✅ core                 │
│     上     → isStopWord() = true  → ❌ 跳过                 │
│     安装   → len > 1              → ✅ optional             │
│     Docker → isEnglish() = true   → ✅ core                 │
│     ？     → trim后为空           → ❌ 跳过                 │
│                                                              │
│  Step 3: 返回结果                                           │
│     core: [Ubuntu, Docker]                                  │
│     optional: [安装]                                        │
└─────────────────────────────────────────────────────────────┘
                            ↓
┌─────────────────────────────────────────────────────────────┐
│         构建 ES 查询 (search_service.go)                     │
│  {                                                           │
│    "must": [                                                 │
│      {"match": {"text_content": "Ubuntu"}},                 │
│      {"match": {"text_content": "Docker"}}                  │
│    ],                                                        │
│    "should": [                                               │
│      {"match": {"text_content": "安装"}}                    │
│    ]                                                         │
│  }                                                           │
└─────────────────────────────────────────────────────────────┘
                            ↓
                      返回搜索结果
```

## 核心文件和函数

| 文件 | 函数 | 作用 |
|------|------|------|
| `cmd/server/main.go` | `main()` | 初始化分词器 |
| `internal/service/search_service.go` | `NewSearchService()` | 创建分词器实例 |
| `internal/service/search_service.go` | `HybridSearch()` | 触发关键词提取 |
| `internal/service/search_service.go` | `fallbackKeywordExtraction()` | 调用分词器 |
| `internal/service/segmenter.go` | `GetSegmenter()` | 单例初始化 |
| `internal/service/segmenter.go` | `SegmentWithPOSAdvanced()` | **核心分词逻辑** |
| `internal/service/segmenter.go` | `isStopWord()` | 停用词判断 |
| `internal/service/segmenter.go` | `isEnglish()` | 英文判断 |

## 关键代码片段

### 1. 初始化（只执行一次）
```go
// cmd/server/main.go
searchService := service.NewSearchService(
    embeddingClient, es.ESClient, userService, 
    uploadRepo, rerankClient, llmClient,
    cfg.Segmenter,  // ← 传入配置
)
```

### 2. 调用分词器
```go
// internal/service/search_service.go
func (s *searchService) fallbackKeywordExtraction(query string) {
    if s.segmenter != nil && s.segmenter.enabled {
        core, optional := s.segmenter.SegmentWithPOSAdvanced(query)
        // ↑ 这里调用分词器
    }
}
```

### 3. 分词处理
```go
// internal/service/segmenter.go
func (s *QuerySegmenter) SegmentWithPOSAdvanced(query string) (core, optional []string) {
    segments := s.seg.Cut(query, true)  // gse 分词
    
    for _, seg := range segments {
        if isStopWord(seg) { continue }  // 过滤停用词
        
        if isEnglish(seg) || len([]rune(seg)) > 2 {
            core = append(core, seg)      // 核心词
        } else if len([]rune(seg)) > 1 {
            optional = append(optional, seg)  // 可选词
        }
    }
    return
}
```

## 配置开关

### 启用分词器（推荐）
```yaml
# config.yaml
segmenter:
  enabled: true
```

**效果**: LLM 失败时自动使用分词器

### 禁用分词器
```yaml
# config.yaml
segmenter:
  enabled: false
```

**效果**: 跳过分词器，直接使用简单分词

## 日志示例

### 正常流程（LLM 成功）
```
[SearchService] 步骤2: 开始提取查询关键词
[SearchService] 关键词提取成功 - 核心: [Ubuntu Docker], 可选: [安装]
```

### 降级流程（使用分词器）
```
[SearchService] 步骤2: 开始提取查询关键词
[SearchService] LLM 关键词提取失败，降级为简单分词: timeout
[SearchService] 使用分词器提取关键词 - 核心: [Ubuntu Docker], 可选: [安装]
```

### 兜底流程（简单分词）
```
[SearchService] 步骤2: 开始提取查询关键词
[SearchService] LLM 关键词提取失败，降级为简单分词: timeout
[SearchService] 分词器未提取到关键词，降级到简单分词
[SearchService] 使用简单分词策略（第三层兜底）
```

## 性能指标

| 操作 | 延迟 | 说明 |
|------|------|------|
| 初始化分词器 | ~50ms | 只在启动时执行一次 |
| gse.Cut() | ~1ms | 每次查询 |
| isStopWord() | ~0.001ms | O(1) map 查找 |
| isEnglish() | ~0.001ms | 简单字符判断 |
| **总计** | **~1-2ms** | 可接受的降级延迟 |

## 常见问题

### Q: 分词器什么时候被调用？
**A**: 当 LLM 关键词提取失败时（超时、错误、未配置等）

### Q: 可以跳过 LLM 直接用分词器吗？
**A**: 可以，将 `llmClient` 设为 `nil`，系统会直接降级到分词器

### Q: 停用词可以自定义吗？
**A**: 可以，修改 `segmenter.go` 中的 `isStopWord()` 函数

### Q: 分词器会影响性能吗？
**A**: 影响很小，只增加 1-2ms 延迟，且只在 LLM 失败时才使用

## 调试技巧

### 1. 查看分词结果
在 `SegmentWithPOSAdvanced` 中添加日志：
```go
segments := s.seg.Cut(query, true)
log.Debugf("分词结果: %v", segments)
```

### 2. 查看停用词过滤
```go
if isStopWord(word) {
    log.Debugf("过滤停用词: %s", word)
    continue
}
```

### 3. 强制使用分词器
临时禁用 LLM：
```go
// 在 extractKeywords 开头添加
return nil, errors.New("force fallback")
```

## 总结

**调用链路**: 
```
用户查询 → ChatService → SearchService → 
extractKeywords(LLM) → fallbackKeywordExtraction → 
SegmentWithPOSAdvanced → 返回关键词
```

**核心优势**:
- ✅ 自动降级，无需手动干预
- ✅ 单例模式，高效复用
- ✅ 停用词过滤，提升准确率
- ✅ 详细日志，便于追踪

**关键文件**: `segmenter.go` 的 `SegmentWithPOSAdvanced()` 函数
