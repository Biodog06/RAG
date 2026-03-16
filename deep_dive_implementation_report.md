# 动态工具链重构：技术深度解析与代码级回顾

本报告深入拆解了 `pai-smart-go` 项目动态工具链的底层升级过程，展示了如何通过架构调整解决复杂的反射 Panic 和元数据同步问题。

---

## 1. 核心挑战：Yaegi 解释器的“边界感”

在重构前，系统频繁在 [loader.go](file:///c:/Users/songyipeng/Desktop/claude-project/rag/rag/internal/agent/tools/generated/loader.go) 中发生 `Panic: reflect.Value.Elem on non-pointer value`。
**根本原因**：Yaegi 解释器在运行脚本时，它生成的对象并不是真实的宿主（Host）类型，而是 Yaegi 内部的一种包装类型。当我们尝试在宿主环境用 `reflect` 直接拆解这些包装对象时，Go 的反射库会因为识别不到底层类型而崩溃。

---

## 2. 代码级修改对比

### 2.1 动态加载器：从“直连模式”到“全代理模式” ([loader.go](file:///c:/Users/songyipeng/Desktop/claude-project/rag/rag/internal/agent/tools/generated/loader.go))

#### ❌ 修改前 (危险的反射访问)
```go
// 这种写法在处理解释器内部对象时极易触发 Panic
method := h.itool.MethodByName("Info")
results := method.Call([]reflect.Value{reflect.ValueOf(ctx)})
infoVal := results[0].Elem() // 💥 触发：reflect.Value.Elem on struct
```

#### ✅ 修改后 (解释器内聚执行)
我们通过在解释器内部预置辅助函数，彻底避开了宿主反射：
```go
// 1. 在解释器启动时注入辅助工具
_, err = i.Eval(`
func RunTool(id string, input string) (string, error) {
    t := activeTools[id]
    // 所有的反射调用、参数转换全部在解释器内部完成
    // 解释器内部执行自己生成的代码是 100% 安全且天然适配的
    ...
}
`)

// 2. 宿主环境只需进行极简调用
func (h *hostProxy) InvokableRun(ctx context.Context, input string, ...) (string, error) {
    runFunc, _ := h.i.Eval("einoutils.RunTool")
    // 传递的是基础类型 (string)，Yaegi 处理基础类型转换是非常稳健的
    results := runFunc.Call([]reflect.Value{reflect.ValueOf(h.toolID), reflect.ValueOf(input)})
    ...
}
```

---

## 3. 关键修复点 (Bug Hotfixes)

### 3.1 路径重映射 (Import Mapping)
AI 模型经常会在生成的代码中乱写导入路径：
- `github.com/eino-tools/eino-sdk-go/tool` (虚假路径)
- `github.com/cloudwego/eino/components/tool` (真实路径)

**解决方案**：在 [loader.go](file:///c:/Users/songyipeng/Desktop/claude-project/rag/rag/internal/agent/tools/generated/loader.go) 的加载环节加入强制替换逻辑：
```go
code = strings.ReplaceAll(code, "\"github.com/cloudwego/eino/components/tool\"", "\"einotool\"")
// 将真实的路径强行映射到我们在解释器内部预置的 Mock 包名称上
```

### 3.2 JSON 截断预防
为了解决 `json object is not closed` 报错：
1.  **缩短 Prompt**：去除废话，把 Token 留给代码产出。
2.  **强制 Mock**：在指令中明确“逻辑简洁，必要时提供 mock”，防止模型为了写出完整的爬虫逻辑而导致输出超长。

---

## 4. 最终达成的效果

1.  **元数据感知**：生成的工具在前端会显示正确的中文描述。
2.  **热插拔**：删除 [generated/](file:///c:/Users/songyipeng/Desktop/claude-project/rag/rag/internal/service/tool_codegen_service.go#33-39) 里的 [.go](file:///c:/Users/songyipeng/Desktop/claude-project/rag/rag/cmd/server/main.go) 文件，工具列表立即消失；创建文件，立即出现。
3.  **零污染**：主程序不再因为脚本错误而崩溃（通过 `recover` 捕获异常）。

---

## 5. 开发建议
- **新增工具**：遵循 [tool_generation.md](file:///c:/Users/songyipeng/Desktop/claude-project/rag/rag/internal/agent/skills/tool_generation.md) 的模板。
- **调试日志**：关注 `[DynamicLoader]` 和 `[ToolCodegen]` 前缀的日志，它们记录了从文件扫弦到 JSON 提取的每一步脉络。
