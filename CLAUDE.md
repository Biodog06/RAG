# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

PaiSmart (派聪明) is an enterprise-level AI knowledge base management system using RAG (Retrieval Augmented Generation) technology. This is the **Go backend implementation** of the system, which also includes a Vue 3 frontend.

**Technology Stack:**
- **Framework:** Gin (HTTP), WebSocket (real-time communication)
- **Databases:** MySQL (metadata), Redis (caching + search result cache), Elasticsearch 8.10.0 (vector search)
- **Message Queue:** Kafka (async document processing)
- **Object Storage:** MinIO (file storage)
- **Document Processing:** MinerU (PDF/Doc/PPT intelligent extraction via gRPC), Excelize (Excel data engineering)
- **AI Services:** DeepSeek/Ollama (LLM), Alibaba DashScope (embeddings), BGE/Jina/Cohere-compatible (rerank)
- **Segmentation:** `go-ego/gse` (Chinese word segmentation, optional)
- **Security:** JWT authentication, role-based authorization

## Automated Workflows

This repository uses automated workflows for the development lifecycle. Use these via slash commands (e.g., `/repo-flow-manager`) or by describing the task:

- **`/issue-manager`**: Create and refine GitHub Issues through conversation.
- **`/repo-flow-manager`**: Full development cycle (Claim task -> Design -> Code -> Test -> PR).
- **`/code-review`**: Perform automated quality checks on the current code or a PR.
- **`/master-flow`**: Orchestrate the entire pipeline from requirement to PR.

Detailed workflow definitions are located in `.agents/workflows/`.

## Development Standards

### Testing Guidelines
- **Mandatory Coverage**: All new features and bug fixes MUST include corresponding unit tests.
- **Isolation**: Treat the unit as a black box. Use `AAA` (Arrange-Act-Assert) pattern.
- **Mocks & Stubs**: Use test doubles for external systems (DB, Redis, ES, Kafka, MinIO).
- **CRITICAL**: **NEVER** call real LLM/Embedding APIs (DashScope, DeepSeek) in unit tests. Use mocks.
- **FIRST Principles**: Tests must be Fast, Independent, Repeatable, Self-validating, and Timely.

### Pull Request Standards
- **Readiness**: PRs should only be created if the branch has commits ahead of base and all tests pass.
- **Documentation**: Technology plans should be saved in `docs/tech/ISSUE_<ID>_TECH_PLAN.md`.
- **Commits**: Use conventional commits (e.g., `feat(ui): add search bar`, `fix(pipeline): handle large files`).
- **Description**: PR body must clearly explain the "What" and "Why", and link to the relevant issue.

## Common Commands

### Backend (Go)

```bash
# Build the application
go build -o bin/server cmd/server/main.go

# Run the server directly
go run cmd/server/main.go

# Run all tests
go test ./...

# Run tests for a specific package
go test ./internal/service/...

# Update dependencies
go mod tidy

# Format code
go fmt ./...

# Lint (requires golangci-lint)
golangci-lint run
```

### Frontend (Vue 3)

```bash
cd frontend
pnpm install
pnpm run dev          # Development server (test mode)
pnpm run dev:prod     # Development server (prod mode)
pnpm run build        # Build for production
pnpm run typecheck    # Type checking
pnpm run lint         # Lint and fix
```

See [CI Guide](file:///docs/CI_GUIDE.md) for automated pipeline details.

### Docker & Infrastructure

```bash
cd deployments
docker-compose up -d   # Start all services
docker-compose down    # Stop all services
docker-compose logs -f [service_name]
```

## Architecture

### Initialization Flow (`cmd/server/main.go`)

1. Config (Viper) → Logging (Zap) → Infrastructure (MySQL, Redis, MinIO, ES, Kafka)
2. Repositories → Services → Handlers → Gin routes
3. Background goroutines: Kafka consumer, `initSeedFiles` (auto-imports files from `initfile/` directory on startup, idempotent)

### RAG Document Processing Pipeline

**Upload:**
1. Client uploads chunks → `UploadHandler` stores in MinIO (metadata in MySQL)
2. On merge, Kafka publishes `FileProcessingTask` to `file-processing` topic

**Processing (`pipeline.Processor` consumes Kafka):**
1. Download file from MinIO (`merged/<filename>`)
2. Extract text via MinerU (PDF/Doc/PPT) or Excelize (XLSX, data engineering)
3. Split text into chunks (if not Excel): **自适应策略** (Markdown 1500, Code 1200, Default 1000)
4. Save chunks to MySQL `document_vector` table (idempotent)
5. Generate embeddings via DashScope: **批量向量化** (10个/批次), **并发处理** (5个并发Worker)
6. Index to Elasticsearch: **部分失败容错** (异常分块跳过不阻塞全文件)

**Alternative splitter:** `pkg/splitter/markdown.go` — header-aware Markdown splitting that preserves section hierarchy context (not used in default pipeline, available for custom use).

### Hybrid Search Pipeline (`internal/service/search_service.go`)

7-step pipeline for each query:

1. **Resolve org tags** — get user's effective org tags including hierarchy
2. **Keyword extraction** — LLM extracts core/optional keywords (3-tier fallback)
3. **Cache check** — Redis lookup by normalized core keywords
4. **Vectorize** — embed raw query
5. **Parallel retrieval** — ES 向量检索 (KNN) + 关键词检索 (BM25), **禁用 track_total_hits 优化性能**
6. **RRF fusion** — Reciprocal Rank Fusion (k=60) merges both recall lists
7. **Rerank** — top-50 candidates sent to rerank model
8. **Return top-K** — fetch filenames, **异步写入动态 TTL 缓存** (基于频率: 1h - 7d)

### Security & Observability

- **Rate Limiting:** 全局限流中间件 (10 QPS/用户)，防止 API 滥用
- **Log Masking:** 自动脱敏日志中的 token, password, secret 等敏感词
- **Slow Query Log:** 自动记录耗时超过 2s 的慢搜索请求
- **Retry Mechanism:** ES 交互自动重试 (502/503/504)，Kafka 任务 3 次失败进入逻辑死信状态
- **Connection Pooling:** 优化了 HTTP 与 Elasticsearch 的连接池配置

### Chat Flow (`internal/service/chat_service.go`)

- WebSocket connection authenticated via temporary token (`/api/v1/chat/websocket-token` → `/chat/:token`)
- `ChatService` calls `SearchService.HybridSearch`, feeds results as context to LLM
- LLM response streamed back over WebSocket
- `ContentCacheService` caches assembled context content in-memory

### Multi-Tenancy

Three-tier access control enforced via ES filter on every search:
- **Public:** `is_public=true` — all authenticated users
- **Org:** `org_tag` matches user's effective tags (includes parent tags via hierarchy)
- **Private:** `user_id` matches

### File Upload Protocol

1. **Check** — client sends MD5 → server checks for existing file (instant/fast upload)
2. **Chunk** — upload file in chunks
3. **Merge** — server composes chunks in MinIO, publishes Kafka message

## Package Structure

```
cmd/server/main.go       # Entry point, DI wiring
internal/
  config/                # Viper config structs
  handler/               # Gin HTTP/WebSocket handlers
  middleware/            # AuthMiddleware, AdminAuthMiddleware, RequestLogger
  model/                 # Domain models (GORM entities + ES document + DTOs)
  pipeline/              # Document processing (Tika→chunk→embed→ES)
  repository/            # Data access (GORM + Redis)
  service/               # Business logic; segmenter.go (gse-based), rrf.go (fusion)
pkg/
  database/              # MySQL (GORM), Redis init
  embedding/             # DashScope embedding client
  es/                    # Elasticsearch client + index creation
  kafka/                 # Producer/consumer (segmentio/kafka-go), manual offset commit
  llm/                   # LLM client (DeepSeek/Ollama streaming)
  rerank/                # Rerank client (BGE/Jina/Cohere-compatible HTTP API)
  splitter/              # Markdown-aware text splitter
  storage/               # MinIO client
  tasks/                 # FileProcessingTask type (Kafka payload)
  tika/                  # Apache Tika HTTP client
  token/                 # JWT manager
```

## Configuration (`configs/config.yaml`)

Key settings beyond standard ones:

| Key | Purpose |
|-----|---------|
| `rerank.enable` | Toggle rerank step (falls back to RRF order if false) |
| `rerank.api_key/base_url/model/min_score` | Rerank API settings |
| `segmenter.enabled` | Toggle gse-based Chinese segmentation |
| `segmenter.dict` | Optional custom dictionary path |
| `embedding.dimensions` | Must match model (default: 2048 for text-embedding-v4) |
| `llm.prompt.*` | System prompt rules, reference delimiters, no-result text |

## Testing Requirements

- **Mandatory Coverage**: All new features and bug fixes MUST include corresponding unit tests.
- **No Real AI Calls**: Tests MUST NOT call real LLM/Embedding APIs (DashScope, etc.). Use mocks.
- **Modified Code**: Any modification to existing code should update or add tests to maintain coverage.
- **Go Tests**: Use `go test -v ./...` for backend verification.
- **Frontend Tests**: Use `pnpm run typecheck` and ensure builds pass.
- **Evidence**: Test results should be included in PR descriptions.

## Key Gotchas

1. **ES index mapping:** Must be created with proper vector field mapping before first use. See `pkg/es/client.go`.
2. **Chunk size:** Processor uses **1000 chars / 100 overlap** (not 500/50 as in some docs).
3. **Chinese text:** Always use `[]rune` slicing, never byte slicing.
4. **Rerank disabled:** When `rerank.enable=false`, the client returns identity scores and original order — RRF ranking is preserved.
5. **Seed files:** Place files in `initfile/` directory at project root; they are auto-imported as public documents owned by `admin` user on startup (idempotent via MD5 check).
6. **MinIO bucket:** Must exist before upload. The `minio-init` Docker Compose service creates it automatically.
7. **Kafka consumer:** Runs in a background goroutine with manual offset commit; failures retry up to a threshold before skipping.
