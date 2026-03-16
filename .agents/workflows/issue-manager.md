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
- **操作**:
- **运行命令**: `gh issue create --title "<Title>" --body "<Body>" --label "<Labels>"`。
  - 获取并返回生成的 Issue 链接和编号给用户。

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
