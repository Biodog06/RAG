---
description: Issue Manager Agent - 通过对话快速创建并推送 GitHub Issue
---

# Issue Manager Agent 流程规范

本工作流用于通过对话的方式快速梳理需求并将其转化为 GitHub Issue。

## 执行流程

### 1. 需求对话 (Requirement Gathering)
- **目标**: 通过与用户对话，明确任务边界。
- **操作**: 
  - 询问用户："您想实现什么功能或修复什么问题？"
  - 确认：
    - 核心功能描述。
    - 预期行为/验收标准（Acceptance Criteria）。
    - 优先级/标签。

### 2. 编写 Issue (Drafting)
- **要求**: 将对话内容整理为标准化的 GitHub Issue 模板。
- **模板结构**:
  ```markdown
  ## Description
  ...概述...
  
  ## Acceptance Criteria
  - [ ] 准则1
  - [ ] 准则2
  
  ## Technical Notes (Optional)
  ...建议的实现路径...
  ```

### 3. 推送至 GitHub (Pusing)
- **操作**: 为了确保 Markdown 格式完美呈现（尤其是代码块和列表），建议先将 Body 内容保存为临时 `.md` 文件，并使用 `--body-file` 参数：
  `gh issue create --title "<Title>" --body-file "temp_body.md" --label "<Labels>"`。
- **issue 格式**: 参考 .claude\skills\github-issue-creator 中的skill。

### 4. 任务联通 (Handoff)
- **操作**: 告知用户任务已就绪。
- **推荐接续动作**: 建议用户接下来运行 `Repo Flow Manager` 来自动化执行该 Issue。

## 使用场景
- 当用户有一个新点子时。
- 当发现一个 Bug 需要记录时。
- 当需要拆分大任务为子任务时。

## 注意事项
- 尽量通过简短的追问获取高质量的验收标准。
- 确保 Issue 标题简洁且专业。
- **格式维护**: 必须确保在 GitHub 呈现的内容是标准的 Markdown 格式。若内容包含代码块或复杂列表，**严禁**直接通过命令行字符串拼接，必须使用临时文件 (`--body-file`)。
- **编码提示**: 在 Windows 系统下生成临时文件时，需显式指定 `utf8` 编码（如 PowerShell 的 `Out-File -Encoding utf8`），防止中文显示为乱码。
