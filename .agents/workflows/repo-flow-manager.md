---
description: Repo Flow Manager - 自动化的 GitHub 任务处理流程（领取→方案→实现→测试→PR）
---

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

### 3. 代码执行与单元测试 (Execution & Testing)
- **要求**: 修改代码并对所有新增和修改的代码生成单元测试。
- **操作**:
  1. 根据技术文档修改代码。
  2. **代码生成**: 确保功能逻辑闭环。
  3. **测试生成**: 
     - **开发规范**: 参考 `CLAUDE.md` 中的 **Development Standards** 指导。
     - **强制要求**: 必须为每个受影响的 Go/Vue 文件编写对应的测试文件。
     - **禁止行为**: **严禁在测试中调用真实的大模型 API (DashScope, DeepSeek 等)**。所有网络交互必须使用 Mock/Stub 模拟。
     - **覆盖验证**: 
       - Go: `go test -v -cover ./...`
       - Frontend: `pnpm run typecheck` 或 `npm test`
  4. 验证本地验证通过（编译 + 测试）。

### 4. 任务完成与提交 (PR & Submit)
- **目标**: 将代码推送到远程仓库并创建 PR。
- **操作**:
  1. 使用正确的 Commit 信息格式提交代码：`feat(scope): desc` 或 `fix(scope): desc`。
  2. 推送分支到远程仓库。
  3. **创建 PR**:
     - **操作**: 参考 `CLAUDE.md` 中的 **Pull Request Standards**。
     - **命令**: `gh pr create --title "feat/fix: <title>" --body "Closes #<ISSUE_NUMBER>\n\n<PR_DETAILS>"`。
  4. 设置 PR 状态（如需要）。

### 5. 质量审查 (Next Step)
- **目标**: 确保交付质量。
- **操作**: PR 创建成功后，主动询问或自动启动 `Code Review Agent` 对本次变更进行深度质量检查。

## 注意事项
- **DoD (Definition of Done)**: 技术文档已更新、代码已修改、单元测试已通过、PR 已创建。
- **无感运行**: 尽量减少与用户的非必要交流，尽可能自主完成上述链路。
