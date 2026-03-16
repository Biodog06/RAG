---
name: issue-manager
description: |
  对话式需求整理并自动创建 GitHub Issue。处于常驻开启状态，准备捕获用户的新点子。
  Triggers on: create issue, github issue, new requirement, 需求, 创建任务, 创建issue
classification: workflow
next-skill: repo-flow-manager
---

# Issue Manager Skill

## 概述
作为研发流水线的入口，该技能负责将用户的模糊想法转化为结构化的 GitHub Issue。

## 核心逻辑
1. **自动捕获**: 当用户提到“想做一个功能”或“发现一个bug”时自动激活。
2. **结构化引导**: 引导用户提供背景和验收标准（AC）。
3. **自动同步**: 使用 `gh issue create` 将结果推送至 GitHub。

## 联动钩子 (Hooks)
- **Next Step**: 完成 Issue 创建后，自动触发 `repo-flow-manager` 询问是否开始执行。
