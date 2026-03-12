package generated

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"pai-smart-go/pkg/log"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/traefik/yaegi/interp"
	"github.com/traefik/yaegi/stdlib"
)

// GetDynamicTools uses Yaegi to load and interpret Go files in the generated directory.
// This allows new tools to be added and used without restarting the server.
func GetDynamicTools() []tool.InvokableTool {
	tools := make([]tool.InvokableTool, 0)
	dir := "internal/agent/tools/generated"

	files, err := os.ReadDir(dir)
	if err != nil {
		log.Errorf("[DynamicLoader] Failed to read directory: %v", err)
		return tools
	}

	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".go") {
			continue
		}
		// Skip registry and loader files
		if file.Name() == "registry.go" || file.Name() == "loader.go" {
			continue
		}

		path := filepath.Join(dir, file.Name())
		t, err := loadTool(path)
		if err != nil {
			log.Errorf("[DynamicLoader] Failed to load tool from %s: %v", path, err)
			continue
		}
		if t != nil {
			tools = append(tools, t)
		}
	}

	return tools
}

func loadTool(path string) (tool.InvokableTool, error) {
	i := interp.New(interp.Options{})
	i.Use(stdlib.Symbols)

	// Minimal Eino symbols for Yaegi
	i.Use(interp.Exports{
		"github.com/cloudwego/eino/components/tool/tool": {
			"BaseTool": reflect.ValueOf((*tool.BaseTool)(nil)),
		},
		"github.com/cloudwego/eino/components/tool/utils/utils": {
			"InferTool": reflect.ValueOf(wrapInferTool),
		},
	})

	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	_, err = i.Eval(string(content))
	if err != nil {
		return nil, fmt.Errorf("eval failed: %w", err)
	}

	// Find the New...Tool function using naming convention
	// filename: get_weather.go -> NewGetWeatherTool
	base := filepath.Base(path)
	rawName := strings.TrimSuffix(base, ".go")
	parts := strings.Split(rawName, "_")
	for i := range parts {
		parts[i] = strings.ToUpper(parts[i][:1]) + parts[i][1:]
	}
	funcName := "New" + strings.Join(parts, "") + "Tool"

	v, err := i.Eval("generated." + funcName)
	if err != nil {
		return nil, fmt.Errorf("constructor %s not found: %w", funcName, err)
	}

	// Call the constructor
	args := []reflect.Value{}
	results := v.Call(args)

	if len(results) != 2 {
		return nil, fmt.Errorf("constructor expected 2 returns, got %d", len(results))
	}

	if !results[1].IsNil() {
		return nil, fmt.Errorf("constructor returned error: %v", results[1].Interface())
	}

	t, ok := results[0].Interface().(tool.InvokableTool)
	if !ok {
		return nil, fmt.Errorf("result is not an InvokableTool")
	}

	return t, nil
}

// wrapInferTool is a non-generic wrapper for utils.InferTool to make it compatible with Yaegi.
func wrapInferTool(name, description string, f func(context.Context, any) (string, error)) (tool.InvokableTool, error) {
	return utils.InferTool(name, description, f)
}
