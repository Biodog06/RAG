package service

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"pai-smart-go/internal/model"
	"pai-smart-go/pkg/llm"
	"pai-smart-go/pkg/log"

	"github.com/gorilla/websocket"
)

const (
	generatedToolDir       = "internal/agent/tools/generated"
	generatedToolIndexFile = "internal/agent/tools/generated/TOOLS_INDEX.md"
)

type generatedToolPayload struct {
	ToolName     string `json:"tool_name"`
	FileName     string `json:"file_name"`
	Summary      string `json:"summary"`
	Code         string `json:"code"`
	RegisterHint string `json:"register_hint"`
}

type generatedToolResult struct {
	ToolName     string
	FilePath     string
	Summary      string
	RegisterHint string
}

type ToolGenerationResult struct {
	ToolName     string
	FilePath     string
	Summary      string
	RegisterHint string
}

func (s *chatService) tryHandleToolGeneration(ctx context.Context, query string, user *model.User, _ []model.ChatMessage, writer *wsWriterInterceptor) (bool, error) {
	if !isToolGenerationIntent(query) {
		return false, nil
	}

	result, err := s.generateToolFileFromRequest(ctx, query)
	if err != nil {
		msg := fmt.Sprintf("tool 代码生成失败: %v", err)
		_ = writer.WriteMessage(websocket.TextMessage, []byte(msg))
		sendCompletion(writer.conn)
		_ = s.addMessageToConversation(context.Background(), user.ID, query, msg)
		return true, nil
	}

	answer := fmt.Sprintf(
		"已创建 tool 代码。\n- tool: %s\n- 文件: %s\n- 说明: %s\n- 注册提示: %s",
		result.ToolName,
		result.FilePath,
		result.Summary,
		result.RegisterHint,
	)

	_ = writer.WriteMessage(websocket.TextMessage, []byte(answer))
	sendCompletion(writer.conn)
	_ = s.addMessageToConversation(context.Background(), user.ID, query, answer)
	return true, nil
}

func (s *chatService) generateToolFileFromRequest(ctx context.Context, requirement string) (*generatedToolResult, error) {
	return generateToolFileFromRequest(ctx, s.llmClient, requirement)
}

func GenerateToolCodeFromUserInput(ctx context.Context, llmClient llm.Client, requirement string) (*ToolGenerationResult, error) {
	res, err := generateToolFileFromRequest(ctx, llmClient, requirement)
	if err != nil {
		return nil, err
	}
	return &ToolGenerationResult{
		ToolName:     res.ToolName,
		FilePath:     res.FilePath,
		Summary:      res.Summary,
		RegisterHint: res.RegisterHint,
	}, nil
}

func generateToolFileFromRequest(ctx context.Context, llmClient llm.Client, requirement string) (*generatedToolResult, error) {
	systemPrompt := `你是一个Go工程代码生成器。请根据用户需求生成一个可编译的 Eino Tool Go 文件。
输出必须是严格 JSON（不要 markdown 代码块，不要额外解释），字段如下：
{
  "tool_name": "snake_case",
  "file_name": "snake_case.go",
  "summary": "一句中文功能说明",
  "register_hint": "已自动注册",
  "code": "完整 Go 源码字符串"
}
要求：
1) code 必须可编译。尽量包含必要的 import 依赖。
2) package 名称固定为 generated。
3) tool 构造函数命名为 NewXxxTool，返回 (tool.InvokableTool, error)。
4) 强烈建议使用 utils.InferTool 来简化实现，例如：utils.InferTool("name", "desc", func(ctx context.Context, input *InputStruct) (string, error) { ... })。
5) 优先给出可运行实现；如果涉及外部 API，尽量使用 http.Get/Post 实现简单的逻辑；可以提供 mock 逻辑作为回退。`

	raw, err := llmClient.GenerateOneShot(ctx, []llm.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: requirement},
	})
	if err != nil {
		return nil, fmt.Errorf("llm generate failed: %w", err)
	}

	jsonText, err := extractFirstJSONObject(raw)
	if err != nil {
		return nil, fmt.Errorf("invalid llm json output: %w", err)
	}

	var payload generatedToolPayload
	if err := json.Unmarshal([]byte(jsonText), &payload); err != nil {
		return nil, fmt.Errorf("unmarshal generated payload failed: %w", err)
	}

	fileName := sanitizeFileName(payload.FileName)
	if fileName == "" {
		fileName = sanitizeFileName(payload.ToolName + ".go")
	}
	if fileName == "" {
		fileName = fmt.Sprintf("generated_tool_%d.go", time.Now().Unix())
	}

	toolName := sanitizeToolName(payload.ToolName)
	if toolName == "" {
		toolName = strings.TrimSuffix(fileName, ".go")
	}

	code := strings.TrimSpace(payload.Code)
	if code == "" {
		code = fallbackGeneratedToolCode(toolName, requirement)
	}
	if !strings.HasSuffix(code, "\n") {
		code += "\n"
	}

	if err := os.MkdirAll(generatedToolDir, 0o755); err != nil {
		return nil, fmt.Errorf("create generated dir failed: %w", err)
	}

	filePath := filepath.Join(generatedToolDir, fileName)
	if err := os.WriteFile(filePath, []byte(code), 0o644); err != nil {
		return nil, fmt.Errorf("write generated tool file failed: %w", err)
	}

	summary := strings.TrimSpace(payload.Summary)
	if summary == "" {
		summary = "自动生成 tool 代码"
	}

	registerHint := strings.TrimSpace(payload.RegisterHint)
	if registerHint == "" {
		registerHint = "已自动动态加载，无需重启，下一波对话即可直接使用！"
	}

	if err := registerGeneratedToolToRegistry(toolName); err != nil {
		log.Warnf("register generated tool to registry failed: %v", err)
	}

	if err := appendGeneratedToolIndex(toolName, filePath, requirement); err != nil {
		log.Warnf("append generated tool index failed: %v", err)
	}

	return &generatedToolResult{
		ToolName:     toolName,
		FilePath:     filePath,
		Summary:      summary,
		RegisterHint: registerHint,
	}, nil
}

func registerGeneratedToolToRegistry(toolName string) error {
	registryFile := "internal/agent/tools/generated/registry.go"
	content, err := os.ReadFile(registryFile)
	if err != nil {
		return err
	}

	lines := strings.Split(string(content), "\n")
	newLines := make([]string, 0, len(lines)+5)

	funcName := toCamelCase(toolName)
	registrationLine := fmt.Sprintf("\tif t, err := New%sTool(); err == nil {", funcName)
	
	// Check if already registered
	for _, line := range lines {
		if strings.Contains(line, registrationLine) {
			return nil // Already registered
		}
	}

	for _, line := range lines {
		if strings.TrimSpace(line) == "return tools" {
			newLines = append(newLines, registrationLine)
			newLines = append(newLines, "\t\ttools = append(tools, t)")
			newLines = append(newLines, "\t}")
			newLines = append(newLines, "")
		}
		newLines = append(newLines, line)
	}

	return os.WriteFile(registryFile, []byte(strings.Join(newLines, "\n")), 0o644)
}

func isToolGenerationIntent(query string) bool {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return false
	}

	cnDirect := []string{"新增功能", "添加功能", "新增tool", "新建tool", "创建tool", "生成tool", "写一个tool", "加一个tool", "开发一个tool", "给我写个功能"}
	for _, k := range cnDirect {
		if strings.Contains(q, k) {
			return true
		}
	}

	enActions := []string{"create", "add", "new", "generate", "build", "implement"}
	hasToolWord := strings.Contains(q, "tool")
	if hasToolWord {
		for _, a := range enActions {
			if strings.Contains(q, a) {
				return true
			}
		}
	}

	return false
}

func extractFirstJSONObject(raw string) (string, error) {
	start := strings.Index(raw, "{")
	if start < 0 {
		return "", fmt.Errorf("no json object start found")
	}

	depth := 0
	inString := false
	escaped := false
	for i := start; i < len(raw); i++ {
		c := raw[i]
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if c == '\\' {
				escaped = true
				continue
			}
			if c == '"' {
				inString = false
			}
			continue
		}

		if c == '"' {
			inString = true
			continue
		}
		if c == '{' {
			depth++
			continue
		}
		if c == '}' {
			depth--
			if depth == 0 {
				return raw[start : i+1], nil
			}
		}
	}

	return "", fmt.Errorf("json object is not closed")
}

func sanitizeFileName(name string) string {
	base := strings.ToLower(strings.TrimSpace(strings.TrimSuffix(name, ".go")))
	re := regexp.MustCompile(`[^a-z0-9_]+`)
	base = re.ReplaceAllString(base, "_")
	base = strings.Trim(base, "_")
	if base == "" {
		return ""
	}
	return base + ".go"
}

func sanitizeToolName(name string) string {
	base := strings.ToLower(strings.TrimSpace(name))
	re := regexp.MustCompile(`[^a-z0-9_]+`)
	base = re.ReplaceAllString(base, "_")
	base = strings.Trim(base, "_")
	return base
}

func toCamelCase(s string) string {
	parts := strings.Split(s, "_")
	for i := range parts {
		if parts[i] == "" {
			continue
		}
		parts[i] = strings.ToUpper(parts[i][:1]) + parts[i][1:]
	}
	return strings.Join(parts, "")
}

func fallbackGeneratedToolCode(toolName string, requirement string) string {
	funcName := toCamelCase(toolName)
	if funcName == "" {
		funcName = "Generated"
	}

	desc := strings.ReplaceAll(strings.TrimSpace(requirement), "\n", " ")
	if desc == "" {
		desc = "auto generated tool"
	}

	return fmt.Sprintf(`package generated

import (
	"context"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
)

type %sToolInput struct {
	Query string %cjson:"query" jsonschema:"description=tool input query"%c
}

func New%sTool() (tool.InvokableTool, error) {
	return utils.InferTool(%q, %q, func(ctx context.Context, input *%sToolInput) (string, error) {
		if input == nil || strings.TrimSpace(input.Query) == "" {
			return "", fmt.Errorf("query is required")
		}
		return fmt.Sprintf("TODO: implement %s, query=%%s", input.Query), nil
	})
}
`, funcName, '`', '`', funcName, toolName, desc, funcName, desc)
}

func appendGeneratedToolIndex(toolName string, filePath string, requirement string) error {
	if err := os.MkdirAll(filepath.Dir(generatedToolIndexFile), 0o755); err != nil {
		return err
	}

	if _, err := os.Stat(generatedToolIndexFile); os.IsNotExist(err) {
		header := "# Generated Tools\n\nThis file is auto-maintained by chat tool codegen flow.\n\n"
		if err := os.WriteFile(generatedToolIndexFile, []byte(header), 0o644); err != nil {
			return err
		}
	}

	req := strings.ReplaceAll(strings.TrimSpace(requirement), "\n", " ")
	if len(req) > 120 {
		req = req[:120] + "..."
	}

	line := fmt.Sprintf("- %s | `%s` | `%s` | %s\n",
		time.Now().Format("2006-01-02 15:04:05"),
		toolName,
		filepath.ToSlash(filePath),
		req,
	)

	f, err := os.OpenFile(generatedToolIndexFile, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = f.WriteString(line)
	return err
}
