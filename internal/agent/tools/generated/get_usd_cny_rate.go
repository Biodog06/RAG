package generated

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
)

type GetUsdCnyRateInput struct {
	Source string `json:"source" jsonschema:"description=数据源，可选值：'api'（默认）或 'mock'，mock 返回模拟数据"`
}

func NewGetUsdCnyRateTool() (tool.InvokableTool, error) {
	return utils.InferTool("get_usd_cny_rate", "获取当前美元兑人民币汇率并返回简短说明", func(ctx context.Context, input *GetUsdCnyRateInput) (string, error) {
		source := input.Source
		if source == "" {
			source = "api"
		}

		var rate float64
		if source == "api" {
			// 使用公开 API 获取汇率
			resp, err := http.Get("https://api.exchangerate-api.com/v4/latest/USD")
			if err == nil {
				defer resp.Body.Close()
				body, _ := io.ReadAll(resp.Body)
				var data struct {
					Rates map[string]float64 `json:"rates"`
				}
				if json.Unmarshal(body, &data) == nil {
					if r, ok := data.Rates["CNY"]; ok {
						rate = r
					}
				}
			}
		}

		if rate == 0 {
			rate = 7.23 // Fallback mock
			source = "mock"
		}

		result := fmt.Sprintf("当前美元兑人民币汇率约为 1 美元 = %.2f 人民币。数据来源: %s。", rate, source)
		if source == "mock" {
			result += "（此为联机获取失败后的模拟数据或指定模拟数据）"
		}

		return result, nil
	})
}
