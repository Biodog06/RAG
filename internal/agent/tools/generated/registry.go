package generated

import (
	"github.com/cloudwego/eino/components/tool"
)

// GetGeneratedTools returns a list of all dynamically generated tools.
// This file is automatically updated by the tool codegen service.
func GetGeneratedTools() []tool.InvokableTool {
	tools := make([]tool.InvokableTool, 0)

	// 所有 generated 目录下的工具现在都通过动态加载器即时引入
	// 这样可以确保：“生成即使用”，无需重启。
	tools = append(tools, GetDynamicTools()...)

	return tools
}
