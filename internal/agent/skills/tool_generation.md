# Tool Generation Skill

## 1. 核心目标
智能体可以通过编写 Go 代码，自主创建并注册新的工具到系统中。代码将被动态编译并立即生效。

## 2. 输出格式 (严格 JSON)
输出必须是严格的 JSON 对象，不得包含 Markdown 代码块或额外解释：
```json
{
  "tool_name": "snake_case_id",
  "file_name": "filename.go",
  "summary": "工具功能的简短中文描述",
  "code": "完整的 Go 源代码字符串"
}
```

## 3. 代码规范 (强制要求)

### 3.1 基础定义
- **Package**: 必须始终为 `package generated`。
- **依赖库 (强制要求)**: 
  - 必须包含 `"github.com/cloudwego/eino/components/tool"`
  - 必须包含 `"github.com/cloudwego/eino/components/tool/utils"`
  - 禁止使用任何其他三方 Eino 路径。

### 3.2 构造函数
- **命名**: 必须遵循 `New<ToolName>Tool` 格式 (CamelCase)。
- **返回值**: 必须返回 `(tool.InvokableTool, error)`。
- **工厂实现**: 强烈建议使用 `utils.InferTool` 快速创建。

### 3.3 输入参数 (Struct)
- 所有输入参数定义在 Struct 中。
- **Tag**: 每个字段必须包含 `json:"..."` 和 `jsonschema:"description=..."`。

## 4. 示例模板
```go
package generated

import (
	"context"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
)

type MyToolInput struct {
	Param string `json:"param" jsonschema:"description=参数描述"`
}

func NewMyToolTool() (tool.InvokableTool, error) {
	return utils.InferTool(
		"my_tool_id",
		"工具描述",
		func(ctx context.Context, input *MyToolInput) (string, error) {
			return "结果", nil
		},
	)
}
```

## 5. 禁止事项
- 禁止使用 `init()` 函数。
- 禁止从外部文件读取不安全数据。
- 逻辑务必简洁，复杂逻辑可提供 mock 实现作为回退，降低生成阶段截断风险。
