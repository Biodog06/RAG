---
description: Pipeline Orchestrator - 协调 Issue-Manager, Repo-Flow-Manager 和 Code-Review 的全生命周期流程
---

# 研发流水线联动指南 (Orchestrator)

本指南说明了如何让三个智能体在 Claude 中高效联动，形成从“想法”到“高质量合并”的闭环。

## 联动链路图

```mermaid
graph TD
    A[用户想法] -->|对话| B(Issue Manager Agent)
    B -->|推送| C[GitHub Issue]
    C -->|分配任务| D(Repo Flow Manager)
    D -->|执行| E[技术文档 + 代码 + 单元测试]
    E -->|创建| F[GitHub Pull Request]
    F -->|触发| G(Code Review Agent)
    G -->|反馈| H{审查通过?}
    H -- 否 --> D
    H -- 是 --> I[合并代码并关闭 Issue]
```

## 自动化联动机制

为了实现高效协作，Claude 会根据上下文或关键词自动识别并建议运行对应工作流。

| 流程 | 激活关键词 | 触发时机 |
| :--- | :--- | :--- |
| **Issue Manager** | `新需求`, `创建任务` | 初始需求讨论阶段。 |
| **Repo Flow Manager** | `开始开发`, `领取任务` | 当 Issue 创建成功，或者准备开始编码时。 |
| **Code Review Agent** | `代码审查`, `提交代码` | 当代码完成、创建 PR 或需要质量检查时。 |

### 3. 质量把控 (PR -> Merge)
- **指令**: "开启代码审查" 或 "启动 Code Review"。
- **联动点**: 
  - **手动**: 您可以直接要求对当前的 PR 进行审查。
  - **自动 (CI)**: `code-review.md` 中定义的逻辑可以集成在 GitHub Action 中。

## 智能体间的上下文传递
- **仓库**: 所有智能体都基于当前工作区的 Git 仓库。
- **引用**: `repo-flow-manager` 会在 PR 描述中自动包含 `Closes #<IssueID>`。
- **文档**: `repo-flow-manager` 生成的技术文档存储在 `docs/tech/`，作为 `code-review` 审查背景的重要输入。

---

## 协作秘籍
如果您想一气呵成，可以直接下令：
> "启动全流程：先帮我梳理 [XX功能] 的需求并创建 Issue，然后立即领取该任务开始开发，完成后提交 PR 并进行自我审查。"

Claude 会识别意图并依次调用对应工作流的逻辑。
