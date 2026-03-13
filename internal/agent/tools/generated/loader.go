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
	"github.com/cloudwego/eino/schema"
	"github.com/traefik/yaegi/interp"
	"github.com/traefik/yaegi/stdlib"
)

func GetDynamicTools() []tool.InvokableTool {
	tools := make([]tool.InvokableTool, 0)
	dir := "internal/agent/tools/generated"
	absDir, _ := filepath.Abs(dir)
	log.Infof("[DynamicLoader] Searching for tools in: %s", absDir)
	files, err := os.ReadDir(dir)
	if err != nil {
		log.Errorf("[DynamicLoader] Failed to read directory [%s]: %v", absDir, err)
		return tools
	}

	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".go") {
			continue
		}
		if file.Name() == "registry.go" || file.Name() == "loader.go" {
			continue
		}

		path := filepath.Join(dir, file.Name())
		t := func() tool.InvokableTool {
			defer func() {
				if r := recover(); r != nil {
					log.Errorf("[DynamicLoader] 💥 Critical error loading tool [%s]: %v", file.Name(), r)
				}
			}()
			
			it, err := loadTool(path)
			if err != nil {
				log.Errorf("[DynamicLoader] ❌ Error loading [%s]: %v", file.Name(), err)
				return nil
			}
			return it
		}()

		if t != nil {
			info, _ := t.Info(context.Background())
			log.Infof("[DynamicLoader] ✅ Loaded tool meta: %s", info.Name)
			tools = append(tools, t)
		}
	}

	return tools
}

func loadTool(path string) (tool.InvokableTool, error) {
	i := interp.New(interp.Options{})
	i.Use(stdlib.Symbols)

	// Declare dummy infrastructure
	_, err := i.Eval(`
package einotool
import "context"
type ToolInfo struct { Name, Desc string }
type InvokableTool interface {
	Info(ctx context.Context) (*ToolInfo, error)
	InvokableRun(ctx context.Context, input string, opts ...any) (string, error)
}
`)
	if err != nil {
		return nil, err
	}

	_, err = i.Eval(`
package einoutils
import "einotool"
import "context"
import "fmt"
import "encoding/json"
import "reflect"

func InferTool(name, desc string, f any) (einotool.InvokableTool, error) {
	return &internalWrapper{name, desc, f}, nil
}

type internalWrapper struct { Name, Desc string; F any }
func (w *internalWrapper) Info(ctx context.Context) (*einotool.ToolInfo, error) {
	return &einotool.ToolInfo{Name: w.Name, Desc: w.Desc}, nil
}
func (w *internalWrapper) InvokableRun(ctx context.Context, in string, opts ...any) (string, error) {
	return "", nil 
}

// Global registry for constructed tools to keep them in interpreter context
var activeTools = make(map[string]einotool.InvokableTool)

func RegisterTool(id string, t einotool.InvokableTool) {
	activeTools[id] = t
}

func GetInfo(id string) (string, string) {
	if t, ok := activeTools[id]; ok {
		info, _ := t.Info(context.Background())
		if info != nil { return info.Name, info.Desc }
	}
	return "Unknown", ""
}

func RunTool(id string, input string) (string, error) {
	t, ok := activeTools[id]
	if !ok { return "", fmt.Errorf("tool not found") }
	
	val := reflect.ValueOf(t)
	if val.Kind() == reflect.Ptr { val = val.Elem() }
	fVal := val.FieldByName("F")
	
	fType := fVal.Type()
	inType := fType.In(1)
	var inVal reflect.Value
	if inType.Kind() == reflect.Ptr {
		inVal = reflect.New(inType.Elem())
	} else {
		inVal = reflect.New(inType).Elem()
	}

	if err := json.Unmarshal([]byte(input), inVal.Interface()); err != nil {
		return "", fmt.Errorf("unmarshal fail: %v", err)
	}

	res := fVal.Call([]reflect.Value{reflect.ValueOf(context.Background()), inVal})
	if !res[1].IsNil() {
		return "", res[1].Interface().(error)
	}
	return res[0].String(), nil
}
`)
	if err != nil {
		return nil, err
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	code := string(content)
	code = strings.ReplaceAll(code, "\"github.com/cloudwego/eino/components/tool\"", "\"einotool\"")
	code = strings.ReplaceAll(code, "\"github.com/cloudwego/eino/components/tool/utils\"", "\"einoutils\"")
	code = strings.ReplaceAll(code, "\"github.com/cloudwego/eino/schema\"", "\"einotool\"")
	code = strings.ReplaceAll(code, "tool.InvokableTool", "einotool.InvokableTool")
	code = strings.ReplaceAll(code, "utils.InferTool", "einoutils.InferTool")
	code = strings.ReplaceAll(code, "schema.ToolInfo", "einotool.ToolInfo")

	_, err = i.Eval(code)
	if err != nil {
		return nil, fmt.Errorf("eval error: %v", err)
	}

	base := filepath.Base(path)
	rawName := strings.TrimSuffix(base, ".go")
	parts := strings.Split(rawName, "_")
	for j := range parts {
		if len(parts[j]) > 0 {
			parts[j] = strings.ToUpper(parts[j][:1]) + parts[j][1:]
		}
	}
	funcName := "New" + strings.Join(parts, "") + "Tool"

	// Construct and register inside interpreter
	toolID := "tool_" + rawName
	_, err = i.Eval(fmt.Sprintf(`
		if t, err := generated.%s(); err == nil {
			einoutils.RegisterTool("%s", t)
		} else {
			panic(err)
		}
	`, funcName, toolID))

	if err != nil {
		return nil, fmt.Errorf("factory call failed: %v", err)
	}

	// Get Metadata
	nameVal, _ := i.Eval(fmt.Sprintf(`n, _ := einoutils.GetInfo("%s"); n`, toolID))
	descVal, _ := i.Eval(fmt.Sprintf(`_, d := einoutils.GetInfo("%s"); d`, toolID))

	return &hostProxy{
		i:      i,
		toolID: toolID,
		name:   nameVal.String(),
		desc:   descVal.String(),
	}, nil
}

type hostProxy struct {
	i      *interp.Interpreter
	toolID string
	name   string
	desc   string
}

func (h *hostProxy) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{Name: h.name, Desc: h.desc}, nil
}

func (h *hostProxy) InvokableRun(ctx context.Context, input string, opts ...tool.Option) (string, error) {
	// Call RunTool helper in interpreter
	runFunc, err := h.i.Eval("einoutils.RunTool")
	if err != nil {
		return "", err
	}

	results := runFunc.Call([]reflect.Value{
		reflect.ValueOf(h.toolID),
		reflect.ValueOf(input),
	})

	if !results[1].IsNil() {
		return "", results[1].Interface().(error)
	}

	return results[0].String(), nil
}
