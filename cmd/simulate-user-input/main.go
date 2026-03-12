package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"pai-smart-go/internal/config"
	"pai-smart-go/internal/service"
	"pai-smart-go/pkg/llm"
)

func main() {
	input := flag.String("input", "", "simulate user input text")
	flag.Parse()

	if *input == "" {
		fmt.Fprintln(os.Stderr, "missing -input, example: -input \"新增功能：查询汇率并返回结果\"")
		os.Exit(1)
	}

	config.Init("./configs/config.yaml")
	client := llm.NewClient(config.Conf.LLM)

	result, err := service.GenerateToolCodeFromUserInput(context.Background(), client, *input)
	if err != nil {
		fmt.Fprintf(os.Stderr, "simulate failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("tool: %s\n", result.ToolName)
	fmt.Printf("file: %s\n", result.FilePath)
	fmt.Printf("summary: %s\n", result.Summary)
	fmt.Printf("register_hint: %s\n", result.RegisterHint)
}

