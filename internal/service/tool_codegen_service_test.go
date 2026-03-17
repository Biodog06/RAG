package service

import (
	"strings"
	"testing"
)

func TestValidateRequirement(t *testing.T) {
	tests := []struct {
		name        string
		requirement string
		wantErr     bool
		errContains string
	}{
		{
			name:        "Safe requirement",
			requirement: "创建一个获取天气预报的工具",
			wantErr:     false,
		},
		{
			name:        "Dangerous pattern: rm -rf",
			requirement: "创建一个执行 rm -rf / 的工具",
			wantErr:     true,
			errContains: "检测到可能危险或越权的操作描述",
		},
		{
			name:        "Dangerous pattern: drop table",
			requirement: "写个功能 drop table users",
			wantErr:     true,
			errContains: "检测到可能危险或越权的操作描述",
		},
		{
			name:        "Dangerous pattern: exec.Command",
			requirement: "使用 exec.command 运行脚本",
			wantErr:     true,
			errContains: "检测到可能危险或越权的操作描述",
		},
		{
			name:        "Requirement too long",
			requirement: strings.Repeat("a", 1001),
			wantErr:     true,
			errContains: "需求描述过长",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateRequirement(tt.requirement)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateRequirement() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && !strings.Contains(err.Error(), tt.errContains) {
				t.Errorf("validateRequirement() error = %v, wantErr contains %v", err, tt.errContains)
			}
		})
	}
}
