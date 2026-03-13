package generated

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
)

type RmbToJpyConverterInput struct {
	Amount float64 `json:"amount" jsonschema:"description=人民币金额"`
}

func NewRmbToJpyConverterTool() (tool.InvokableTool, error) {
	return utils.InferTool(
		"rmb_to_jpy_converter",
		"将人民币金额转换为日元汇率",
		func(ctx context.Context, input *RmbToJpyConverterInput) (string, error) {
			if input.Amount <= 0 {
				return "", fmt.Errorf("金额必须大于0")
			}

			// 尝试从公开API获取实时汇率
			rate, err := getExchangeRate()
			if err != nil {
				// 如果API调用失败，使用一个合理的默认汇率（约1 CNY = 21 JPY）
				rate = 21.0
			}

			jpyAmount := input.Amount * rate
			
			result := fmt.Sprintf("%.2f 人民币 ≈ %.2f 日元 (汇率: 1 CNY = %.4f JPY)", 
				input.Amount, jpyAmount, rate)
			
			return result, nil
		},
	)
}

// 从公开API获取人民币对日元汇率
type ExchangeRateResponse struct {
	Rates map[string]float64 `json:"rates"`
	Base  string             `json:"base"`
}

func getExchangeRate() (float64, error) {
	url := "https://api.exchangerate-api.com/v4/latest/CNY"

	client := &http.Client{Timeout: 5 * time.Second}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return 0, fmt.Errorf("创建请求失败: %v", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("API请求失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("API返回错误状态码: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, fmt.Errorf("读取响应失败: %v", err)
	}

	var rateResp ExchangeRateResponse
	if err := json.Unmarshal(body, &rateResp); err != nil {
		return 0, fmt.Errorf("解析JSON失败: %v", err)
	}

	jpyRate, ok := rateResp.Rates["JPY"]
	if !ok {
		return 0, fmt.Errorf("未找到日元汇率")
	}

	return jpyRate, nil
}
