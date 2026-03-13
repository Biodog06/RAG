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

type GithubUserInfoInput struct {
	Username string `json:"username" jsonschema:"description=GitHub用户名,例如: octocat"`
}

func NewGithubUserInfoTool() (tool.InvokableTool, error) {
	return utils.InferTool(
		"github_user_info",
		"查询GitHub用户信息",
		func(ctx context.Context, input *GithubUserInfoInput) (string, error) {
			if input.Username == "" {
				return "", fmt.Errorf("用户名不能为空")
			}

			// 创建HTTP客户端
			client := &http.Client{
				Timeout: 10 * time.Second,
			}

			// 构建GitHub API URL
			url := fmt.Sprintf("https://api.github.com/users/%s", input.Username)

			// 发送请求
			req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
			if err != nil {
				return "", fmt.Errorf("创建请求失败: %v", err)
			}

			// 设置请求头
			req.Header.Set("Accept", "application/vnd.github.v3+json")
			req.Header.Set("User-Agent", "Eino-Agent")

			resp, err := client.Do(req)
			if err != nil {
				return "", fmt.Errorf("请求失败: %v", err)
			}
			defer resp.Body.Close()

			// 读取响应
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				return "", fmt.Errorf("读取响应失败: %v", err)
			}

			// 检查HTTP状态码
			if resp.StatusCode != 200 {
				if resp.StatusCode == 404 {
					return "", fmt.Errorf("用户 '%s' 不存在", input.Username)
				}
				return "", fmt.Errorf("GitHub API返回错误: %s", resp.Status)
			}

			// 解析JSON响应
			var userInfo map[string]interface{}
			if err := json.Unmarshal(body, &userInfo); err != nil {
				return "", fmt.Errorf("解析JSON失败: %v", err)
			}

			// 提取关键信息
			result := "GitHub用户信息:\n"
			result += fmt.Sprintf("用户名: %v\n", userInfo["login"])
			result += fmt.Sprintf("昵称: %v\n", userInfo["name"])
			result += fmt.Sprintf("公司: %v\n", userInfo["company"])
			result += fmt.Sprintf("位置: %v\n", userInfo["location"])
			result += fmt.Sprintf("邮箱: %v\n", userInfo["email"])
			result += fmt.Sprintf("个人简介: %v\n", userInfo["bio"])
			result += fmt.Sprintf("公开仓库数: %v\n", userInfo["public_repos"])
			result += fmt.Sprintf("粉丝数: %v\n", userInfo["followers"])
			result += fmt.Sprintf("关注数: %v\n", userInfo["following"])
			result += fmt.Sprintf("创建时间: %v\n", userInfo["created_at"])
			result += fmt.Sprintf("主页: %v\n", userInfo["html_url"])

			return result, nil
		},
	)
}