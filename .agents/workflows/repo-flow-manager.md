---
description: Repo Flow Manager - 规范化 GitHub 交付流程（Issue→分支→提交→PR→验证→合并→关闭）
---

# Repo Flow Manager 流程规范

本工作流用于指导 Antigravity 智能体在 GitHub 仓库中执行标准化的工程交付流程。

## 流程步骤

### 1. 同步信息与范围
- 首先通过分析用户需求，确定当前的开发/修复范围。
- **调用 Skill**: 查看 `file:///.claude/skills/fetch-github-issue/SKILL.md` 以获取或确认现有 Issue 信息。
- 收集所有修复点，明确验收标准。

### 2. 管理 Issue
- 如果没有对应的 Issue，则必须先创建一个。
- **调用 Skill**: 使用 `file:///.claude/skills/github-issue-creator/SKILL.md` 的指导来创建标准化 Issue。
// turbo
- **运行命令**: `powershell -File scripts/ops/create_github_issues_api.ps1` (需注入 `GITHUB_TOKEN`)。

### 3. 分支与提交策略
- 基于 `master` 创建分支：`fix/<topic>` 或 `feat/<topic>`。
- 提交信息必须使用中文，格式：`类型(范围): 中文说明`。
- **强制要求**: 每个提交应为一个“最小闭环”。

### 4. 实现与验证
- **强制要求**: 所有新增和修改的功能代码必须配套对应的单元测试，确保逻辑覆盖。
- 在实现功能后，必须执行本地验证。
- **调用 Skill**: 使用 `file:///.claude/skills/create-unit-tests/SKILL.md` 的指导来编写和补全单元测试。
- **提交要求**: 单元测试文件必须随功能代码一同提交到 PR 中。
- **运行验证**: 
  - 前端: `npm --prefix frontend ci && npm --prefix frontend run build`
  - Go: `go test -v ./...` (观察测试覆盖率)
  - Java (Java 21): `./mvnw test`
- 记录验证证据（命令输出片段，需包含测试通过摘要）。

### 5. 创建 Pull Request
- **调用 Skill**: 使用 `file:///.claude/skills/pull-request/SKILL.md` 指导编写高质量 PR 内容。
- PR 必须包含：Background, Problem, Acceptance Criteria, Solution, Result, Verification, Impact & Risk, Related Issue。
- **运行命令**: `powershell -File scripts/ops/github_pr_merge_close.ps1 -IssueNumber <编号> -HeadBranch <分支> -BaseBranch master`。

### 6. 代码评审
- **调用 Skill**: 在合并前，参考 `file:///.claude/skills/code-review/SKILL.md` 进行自我检查。

### 7. 合并与关闭
- 验证通过后，合并分支。
- 确保 Issue 被自动关闭（使用 `Closes #xx` 语法）。
// turbo
- 如果需要手动关闭，运行: `powershell -File scripts/ops/github_comment_and_close_issue.ps1 -IssueNumber <编号>`。

## 注意事项
- **Token 安全**: 严禁将 `GITHUB_TOKEN` 显式写入任何文件或聊天。
- **提交粒度**: 禁止过大或过碎的提交。
- **DoD (完成标准)**: Issue 有 3 条验收标准，PR 有充分验证证据。
