package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"pai-smart-go/pkg/log"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
)

type CreateToolInput struct {
	ToolName    string `json:"tool_name" jsonschema:"description=工具的唯一名称(小写蛇形,如: get_exchange_rate)"`
	Description string `json:"description" jsonschema:"description=工具的功能描述,让大模型知道何时该调用它"`
	GoCode      string `json:"go_code" jsonschema:"description=Go 语言实现的工具代码,必须包含 New<Name>Tool 构造函数, package 为 generated"`
}

// NewCreateToolTool 创建一个允许智能体自主创建新工具的元工具
func NewCreateToolTool() (tool.InvokableTool, error) {
	// 尝试读取 skill 文件以获取最新的指令
	skillPath := "internal/agent/skills/tool_generation.md"
	skillContent, err := os.ReadFile(skillPath)
	desc := "当现有的工具无法满足用户需求时，智能体可以使用此工具自主编写并注册一个新的 Go 工具。"
	if err == nil {
		desc += "\n\n请严格遵守以下【工具生成规范】进行代码编写：\n" + string(skillContent)
	}

	return utils.InferTool("create_custom_tool", desc, func(ctx context.Context, input *CreateToolInput) (string, error) {
		if input.ToolName == "" || input.GoCode == "" {
			return "", fmt.Errorf("tool_name and go_code are required")
		}

		// 安全校验：只允许写入 generated 目录
		dir := "internal/agent/tools/generated"
		fileName := fmt.Sprintf("%s.go", input.ToolName)
		path := filepath.Join(dir, fileName)

		log.Infof("[Meta-Tool] Creating new dynamic tool: %s at %s", input.ToolName, path)

		// 确保目录存在
		if err := os.MkdirAll(dir, 0755); err != nil {
			return "", fmt.Errorf("failed to create directory: %v", err)
		}

		// 检查代码是否包含 package generated
		if !strings.Contains(input.GoCode, "package generated") {
			return "", fmt.Errorf("the Go code must belong to 'package generated'")
		}

		// 简单校验构造函数是否存在
		funcName := "New"
		parts := strings.Split(input.ToolName, "_")
		for _, p := range parts {
			if len(p) > 0 {
				funcName += strings.ToUpper(p[:1]) + p[1:]
			}
		}
		funcName += "Tool"
		if !strings.Contains(input.GoCode, funcName) {
			return "", fmt.Errorf("the Go code must contain the constructor function: %s", funcName)
		}

		// 写入文件
		err := os.WriteFile(path, []byte(input.GoCode), 0644)
		if err != nil {
			return "", fmt.Errorf("failed to write tool file: %v", err)
		}

		return fmt.Sprintf("成功创建并注册工具: %s。您现在可以在后续对话中直接调用它了（无需重启）。", input.ToolName), nil
	})
}
