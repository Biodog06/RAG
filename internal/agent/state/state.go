package state

import (
	"pai-smart-go/internal/model"

	"github.com/cloudwego/eino/schema"
)

// RagState 定义在 Eino Graph 各节点之间流转的全局状态
type RagState struct {
	Query    string
	User     *model.User
	History  []model.ChatMessage
	Messages []*schema.Message // 使用 Eino 原生 Message 数组
}
