package generated

import (
	"context"
	"fmt"
	"time"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
)

type CheckCurrentYearInput struct {
	// 无输入参数
}

func NewCheckCurrentYearTool() (tool.InvokableTool, error) {
	return utils.InferTool(
		"check_current_year",
		"查询当前年份",
		func(ctx context.Context, input *CheckCurrentYearInput) (string, error) {
			currentYear := time.Now().Year()
			result := fmt.Sprintf("当前是 %d 年", currentYear)
			return result, nil
		},
	)
}
