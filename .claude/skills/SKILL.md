---
name: repo-flow-manager
description: 规范化 GitHub 交付流程（Issue→分支→提交→PR→验证→合并→关闭）。用户要求按 Issue 开发、提 PR/合并/关 Issue 时必用。
---

# Repo Flow Manager（Issue→PR→CI→Merge）

本技能用于在 **GitHub 仓库**中执行一套可重复的工程化流程：从发现问题/需求开始，自动拆解为 Issue；再创建分支、实现修复、补充 PR 说明；运行本地检查/CI；最终合并分支并关闭对应 Issue。

## Mandatory Trigger
- 用户明确要求：按 Issue 继续修复/开发、把需求拆成 Issue、创建 PR 并合并/关闭 Issue。
- 需要形成规范化交付：Issue 编号、PR 说明、验证记录、合并与关闭都有可追溯产物。
- 需要批量回归：多项修复点一次性收敛为一个 PR（或拆分为多个 PR）。

## Required Workflow
1. 同步信息与范围
   - 收集修复点（用户描述 / 文档 / TODO / BUG 回归），明确验收标准与影响范围。
   - 映射到 Issue：优先复用现有 Issue；没有则创建 Issue。
2. 创建 Issue（若需要）
   - 批量创建（API 版）：`scripts/ops/create_github_issues_api.ps1`（需要 `GITHUB_TOKEN` 或 `GH_TOKEN`）。
   - 批量创建（gh CLI 版）：`scripts/ops/create_github_issues.ps1`（需要本机已安装 `gh`）。
3. 创建分支与提交策略
   - 基准分支：默认 `master`。
   - 分支名建议：`fix/<topic>` / `feat/<topic>`。
   - 提交规范（中文化 + 粒度要求）：
     - 提交信息必须中文，格式：`类型(范围): 中文说明`
       - 类型：`feat` / `fix` / `chore` / `docs` / `refactor` / `test`
       - 范围：模块/子系统（如 `authz`、`api`、`recipe`、`social`、`commerce`、`ai`）
       - 中文说明：动宾短句，说明“做了什么/为什么做”
     - 单次提交不要过小：
       - 每个提交应形成一个“可理解的最小闭环”，例如：新增一个接口 + DTO/VO + 单测/验证点
       - 仅修改 1-2 行且不构成闭环的提交禁止单独提交（应合并到同一提交或同一 PR 内）
     - 单次提交也不要过大：
       - 避免把不相关模块揉在同一提交；按主题拆分（错误码/鉴权/搜索/审计等）
     - 示例：
       - `feat(authz): 增加 /api/auth/me 输出角色与能力集`
       - `chore(api): 统一错误响应补齐 requestId 与 traceId`
       - `docs(workflow): 补充 PR 验证清单与回滚说明`
4. 实现与验证（本地优先）
   - 选择验证集合（按改动范围“命中即执行”），并在 PR 中粘贴关键输出片段：
     - 前端（命中 `fronted/`）：
       - `npm --prefix fronted ci`
       - `npm --prefix fronted run build`
     - Go（命中 `go-edge/`）：
       - 在 `go-edge` 目录执行：`go test ./...`
     - Java（命中 `src/main/java/` 或 `pom.xml`）：
       - 本机需要 Java 21：`./mvnw -DskipTests package` 或 `./mvnw test`
       - 若本机不满足 Java 21：必须明确标注“由 CI（Java 21）验证”，并提供 CI 结果或可复现实验步骤
     - 数据库/SQL（命中 `sql/` 或 `src/main/resources/mapper/`）：
       - 必须说明是否需要迁移、是否兼容旧数据、回滚策略
   - 手工冒烟（有 UI/接口行为变更时必做，写清步骤与结果）：
     - 登录/鉴权：未登录、登录后、token 过期/401 等路径
     - 关键页面：首页/详情/发布/搜索/社交（按改动点命中即可）
     - 文件上传/资源 URL：至少验证一条真实资源可正常访问与渲染
   - 验证记录格式（建议直接贴到 PR 的 Verification 段落）：
     - 命令：`<command>`
     - 结果：成功/失败（失败必须解释原因与后续处理）
     - 证据：关键输出片段（如 build 成功摘要、测试通过汇总），必要时附截图
5. PR 内容（强制）
   - PR 必须包含：Background / Problem / Acceptance Criteria / Solution / Result / Verification / Impact & Risk / Related Issue。
   - 可复用模板：`.github/PULL_REQUEST_TEMPLATE.md` 与 `scripts/ops/pr_body_*.md`。
6. 创建 PR / 合并 / 关闭 Issue（尽量自动化）
   - 通用脚本：`scripts/ops/github_pr_merge_close.ps1`（创建 PR、合并 PR、关闭 Issue）。
   - 专用脚本：若存在 `scripts/ops/pr_flow_*.ps1`，优先复用并按需扩展。
   - 运行前要求：分支已 push；token 通过环境变量注入（不要粘贴到聊天）。
7. 失败降级策略（必备）
   - 若脚本支持 `-DryRun`：先 DryRun，确认参数与将要访问的 GitHub API URL。
   - API 调用失败：输出 method + URL + StatusCode（禁止输出 token 或完整 Authorization header）。
   - 自动化不可用：给出浏览器手动创建 PR 的最小步骤，并保留 PR 模板内容与验证记录。

## Inputs
- Repo：默认 `zyyyys123/CulinaryWhispers`
- BaseBranch：默认 `master`
- HeadBranch：目标分支名（必须已 push 到远端）
- IssueNumber：关联 Issue 编号（允许一个或多个；若没有则先创建）
- Token：`GITHUB_TOKEN` 或 `GH_TOKEN`（禁止粘贴到聊天或写入仓库）

## Token 注入（强制）
- 不允许把 token 直接发到聊天里。
- 通过环境变量注入（示例为 PowerShell）：
  - `$env:GITHUB_TOKEN = "<你的 token>"`
  - 或 `$env:GH_TOKEN = "<你的 token>"`
- Token 权限建议：
  - 创建/合并 PR：需要 repo 权限（至少 pull requests: read/write）
  - 关闭 Issue：需要 issues: read/write
  - 若启用 label 自动创建：需要 metadata/labels 相关权限

## 交付清单（DoD）
- Issue：
  - 标题与范围明确，验收标准至少 3 条
  - 标注 labels（area/type/topic 等）
- 分支：
  - 分支名与 Issue 对齐（包含 issue 编号或清晰 topic）
  - 一个分支只做一个主题（不要混 unrelated 需求）
- 提交：
  - 提交信息中文，且单次提交为“最小闭环”
  - 提交里包含必要的验证/用例补齐（测试或手工验证记录）
- PR：
  - PR 描述全中文，必须包含：背景/问题/验收/方案/结果/验证/风险回滚/关联 Issue
  - 必须有验证证据（命令 + 结果 + 关键输出片段/截图）
  - 合并策略默认 squash（除非该主题需要保留多提交历史）
- 合并与关闭：
  - PR 合并后 Issue 自动关闭（`Closes #xx`），并在 Issue/PR 中写明最终结果
  - 若不能自动关闭，必须补评论说明并手工关闭

## Outputs
- PR 链接（或创建结果）、合并方式、关闭的 Issue 列表
- 验证记录（命令 + 结果，或明确标注由 CI 验证）
- 关键改动摘要（影响范围、风险、回滚要点）

## Safety Rules
- 不在日志/输出中打印 token、Authorization header、或任何敏感信息。
- 没有验证记录时不建议合并。
- 不在验收标准不清晰时臆测需求，必须回到 Issue/用户描述中补齐。

## Repo Notes
- PR 合并与关闭：`scripts/ops/github_pr_merge_close.ps1`
- Issue 批量创建：`scripts/ops/create_github_issues_api.ps1` / `scripts/ops/create_github_issues.ps1`
- Issue 评论与关闭：`scripts/ops/github_comment_and_close_issue.ps1`

## 模板与示例（中文）
- PR 模板：`.github/PULL_REQUEST_TEMPLATE.md`
- PR 正文（建议为每个大功能单独建一个文件）：
  - `scripts/ops/pr_body_*.md`

PR 正文最小模板（可直接复制）：
```
## 背景
- 

## 问题
- 

## 验收标准（来自 Issue）
- [ ] 
- [ ] 
- [ ] 

## 解决方案
- 方案概述：
- 关键改动点：
  - 前端：
  - 后端：
  - Go 边缘层：
  - 数据库/SQL：

## 最终结果
- 行为变化：
- 兼容性与影响：

## 测试与验证
- 自动化验证：
  - 命令：
  - 结果：
  - 证据（关键输出片段）：
- 手工冒烟：
  - 步骤：
  - 结果：

## 影响范围与风险
- 影响范围：
- 风险：
- 回滚/降级：

## 关联 Issue
- Closes #xx
```

## 自动化脚本用法（中文）
创建 PR → 合并 → 关闭 Issue（推荐）：
- `scripts/ops/github_pr_merge_close.ps1 -IssueNumber <编号> -HeadBranch <分支> -BaseBranch master -MergeMethod squash`

若需要自定义 PR 正文：
- `scripts/ops/github_pr_merge_close.ps1 -IssueNumber <编号> -HeadBranch <分支> -PrBodyPath <md路径> -MergeMethod squash`

失败时先 DryRun（若脚本支持）：
- 优先输出将访问的 GitHub API URL 与参数解析结果，不输出 token
