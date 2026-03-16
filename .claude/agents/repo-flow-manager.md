# Repo Flow Manager 自动化流程规范

本工作流用于指导 Claude 在 GitHub 仓库中执行全自动化的工程交付。

## 核心逻辑

### 1. 任务领取 (Task Claiming)
- **目标**: 从 GitHub 问题列表中找到已分配给当前用户的任务并开始工作。
- **操作**:
  1. 使用 `gh issue list --assignee @me --status open` 查看分配给我的任务。
  2. 选择一个任务，记录 Issue 编号。
  3. 执行 `gh issue develop <ISSUE_NUMBER>` 创建对应的开发分支（如果分支已存在则切换）。
  4. 使用 `gh issue view <ISSUE_NUMBER>` 获取任务详情及验收标准。

### 2. 技术方案设计 (Documentation)
- **要求**: 在编写代码前，必须先生成技术文档。
- **操作**:
  1. 在 `docs/tech/` 目录下创建 `ISSUE_<ISSUE_NUMBER>_TECH_PLAN.md`。
  2. 文档应包含：
     - **Context**: 任务背景。
     - **Solution**: 详细设计方案、核心架构改动。
     - **Impact**: 受影响的模块。
     - **Test Plan**: 单元测试覆盖策略。

### 3. 代码执行与辅助验证 (Execution & Safety Gates)
- **要求**: 修改代码并对所有新增和修改的代码生成单元测试，杜绝任何形式的回归错误。
- **操作**:
  1. **影响评估 (Impact Analysis)**:
     - **动作**: 如果修改了公共接口、结构体或全局变量，必须使用 `grep_search` 搜索全项目范围内的引用点。
     - **风险**: 严禁只修改主服务而漏掉工具类（如 `rag-admin.go`）或测试脚本。
  2. **代码与单测生成**:
     - 确保功能逻辑闭环。
     - **强制要求**: 必须为每个受影响的 Go/Vue 文件编写对应的测试文件。
     - **核心规范**: **单测（单元测试）严禁在本地开发环境运行**。Agent 仅负责编写测试代码并确保语法正确，测试的逻辑验证必须交付由 GitHub Actions 等 **CI 环境** 完成。
     - **禁止行为**: 严禁在测试中调用真实的大模型 API。所有网络交互必须使用 Mock/Stub 模拟。
  3. **全量校验 (Full Workspace Validation)**:
     - **强制要求**: 在提交 PR 前，必须执行全量编译校验。
     - **命令**: 
       - Go: `go build ./...` (验证系统完整性，确保无接口调用过期)
  4. 验证本地编译通过。

### 4. 任务完成与提交 (PR & Submit)
- **目标**: 将代码推送到远程仓库并创建 PR。
- **操作**:
  1. 使用正确的 Commit 信息格式提交代码：`feat(scope): desc` 或 `fix(scope): desc`。
  2. 推送分支到远程仓库。
  3. **创建 PR**:
     - **操作**: 参考 `CLAUDE.md` 中的 **Pull Request Standards**。
     - **命令**: `gh pr create --title "feat/fix: <title>" --body "Closes #<ISSUE_NUMBER>\n\n<PR_DETAILS>"`。
  4. 设置 PR 状态（如需要）。

### 5. 质量审查与 CI 验证 (CI Gates)
- **目标**: 确保交付质量。
- **操作**: PR 创建成功后，自动启动 CI 流程（包含单元测试跑测）。Agent 需关注 CI 状态，或启动 `Code Review Agent` 配合 CI 反馈进行深度检查。

## Agent 职责与触发时机

| Agent 名称 | 触发动作 | 触发时机 |
|---|---|---|
| **Repo Flow Manager** | `开始开发`, `领取任务` | 当 Issue 创建成功，或者准备开始编码时。**必须进行影响评估。** |
| **Code Review Agent** | `代码审查`, `提交代码` | 当代码完成、创建 PR 或需要质量检查时。**强制 CI 运行测试。** |

## 注意事项
- **DoD (Definition of Done)**: 技术文档已更新、代码已修改、单元测试已编写、本地编译已通过、PR 已创建。
- **单测无感运行**: Agent 本地不运行测试，直接交付 CI。
- **无感运行**: 尽量减少与用户的非必要交流，尽可能自主完成上述链路。