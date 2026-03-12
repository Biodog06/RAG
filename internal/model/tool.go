package model

// ToolDTO 描述一个工具的信息。
type ToolDTO struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	IsGenerated bool   `json:"isGenerated"`
}
