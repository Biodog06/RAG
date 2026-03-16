# CI/CD 流程说明

本项目的 GitHub CI 流程已在 `.github/workflows/ci.yml` 中定义。

## 包含的阶段

1. **Backend Tests (Go)**:
   - 自动运行 `go test ./...`。
   - 生成测试覆盖率报告。

2. **Frontend Quality (Vue 3)**:
   - 运行 `pnpm lint` 检查代码规范。
   - 运行 `pnpm typecheck` 检查 TypeScript 类型。
   - 尝试执行 `pnpm build` 确认构建通过。

3. **Automated Code Review (Agent)**:
   - 在 Pull Request 阶段触发。
   - 与我们定义的 `Code Review Agent` 联动，对变更进行自动扫描。

## 如何新增 CI 检查？

如果需要新增检查项（如 Docker 构建镜像验证），请直接修改 `.github/workflows/ci.yml`。

## 秘钥管理 (Secrets)

如果您的测试依赖外部 API（如 DeepSeek 或 Elasticsearch），请在 GitHub Repo 的 `Settings > Secrets and variables > Actions` 中配置对应的环境变量。
