---
description: Code Review Agent - 自动化的代码审查流，适用于 CI 环境
---

# Code Review Agent 流程规范

本工作流专门用于代码审查任务。它可以由 CI 触发，或者手动调用以对分支/PR 进行深度扫描。

## 核心职责
1. **自动扫描项目变更**: 识别 PR 或当前分支中修改过的文件。
2. **多维度自审**: 检查代码质量、安全性、性能和潜在漏洞。
3. **提供改写建议**: 不仅提出问题，还要给出补丁级别的建议。

## 执行流程

### 1. 环境初始化
- **操作**: 自动识别待审查范围。
  - CI 模式: `gh pr view <PR_NUMBER>` 获取变更列表。
  - 本地模式: `git diff master...HEAD`。

### 2. 执行审查 (Review)
- **要求**: 对涉及的代码逻辑进行深度分析。
- **审查重点**: 符合 `CLAUDE.md` 中的开发标准。
- **检查项**:
  - **逻辑正确性**: 是否解决了 Issue 中描述的问题？
  - **测试覆盖**: 是否有配套的单元测试？
  - **最佳实践**: 是否符合仓库的代码风格和安全规范。

### 3. 生成报告 (Reporting)
- **输出**: 在终端输出或更新 PR 评论。
- **报告格式**:
  - `Summary`: 审查概览、Issue 计数。
  - `Critical/Major/Minor`: 分级列出识别的问题。
  - `Action Items`: 一组可以直接执行的改进建议。

### 4. 提交建议 (Comment)
- **操作**: 如果是在 GitHub PR 环境下运行，使用 `gh pr comment <PR_NUMBER> --body-file <REPORT_FILE>` 提交审查结果。

## CI 调用参考
// turbo
- `powershell -Command "antigravity run code-review --pr $env:GITHUB_PR_NUMBER"` (假设 antigravity 支持这种调用方式)
- 或者手动通过对话触发: `/code-review pr <number>`

## 注意事项
- **客观公正**: 审查应保持技术中立。
- **零容忍**: 对安全风险、缺乏单元测试的问题应标记为 Critical。
