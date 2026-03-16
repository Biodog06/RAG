---
name: repo-flow-manager
description: |
  自动化研发执行器：领取任务 -> 设计方案 -> 编写代码 -> 生成测试 -> 提交 PR。
  处于常驻开启状态，准备领取被分派的任务。
  Triggers on: start development, claim issue, execute task, 领单, 开始开发, 领取任务
classification: workflow
next-skill: code-review
---

# Repo Flow Manager Skill

## 概述
流水线的中游，负责具体的工程实现。它在 Issue 创建后被 `issue-manager` 唤起，或在用户手动领单时激活。

## 核心逻辑
1. **自动领单**: 通过 `gh` 选择任务并创建开发分支。
2. **方案先行**: 在 `docs/tech/` 生成技术文档。
3. **闭环开发**: 实现功能的同时生成单元测试并验证。
4. **自动提交**: 创建并关联 PR。

## 联动钩子 (Hooks)
- **Next Step**: PR 提交成功后，自动触发 `code-review` 进行质量检查。
