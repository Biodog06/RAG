package tools

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"pai-smart-go/pkg/log"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
)

type WeatherToolInput struct {
	Location string `json:"location" jsonschema:"description=需要查询天气的城市名称或拼音,例如 北京, Beijing, Shanghai"`
}

// NewWeatherTool creates a new InvokableTool for querying the weather.
func NewWeatherTool() (tool.InvokableTool, error) {
	return utils.InferTool("get_weather", "当用户询问特定地点的当前天气时调用此工具,需提供地名", func(ctx context.Context, input *WeatherToolInput) (string, error) {
		log.Infof("[ChatService-Agent] Triggered tool get_weather, location: '%s'", input.Location)
		url := fmt.Sprintf("https://wttr.in/%s?format=3", input.Location)
		resp, err := http.Get(url)
		if err != nil {
			return "", fmt.Errorf("failed to fetch weather: %v", err)
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return "", fmt.Errorf("failed to read weather response: %v", err)
		}
		return string(body), nil
	})
}
