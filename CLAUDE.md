# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

PaiSmart (派聪明) is an enterprise-level AI knowledge base management system using RAG (Retrieval Augmented Generation) technology. This is the **Go backend implementation** of the system, which also includes a Vue 3 frontend.

**Technology Stack:**
- **Framework:** Gin (HTTP), WebSocket (real-time communication)
- **Databases:** MySQL (metadata), Redis (caching + search result cache), Elasticsearch 8.10.0 (vector search)
- **Message Queue:** Kafka (async document processing)
- **Object Storage:** MinIO (file storage)
- **Document Processing:** Apache Tika (text extraction)
- **AI Services:** DeepSeek/Ollama (LLM), Alibaba DashScope (embeddings), BGE/Jina/Cohere-compatible (rerank)
- **Segmentation:** `go-ego/gse` (Chinese word segmentation, optional)
- **Security:** JWT authentication, role-based authorization

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
2. Extract text via Apache Tika
3. Split text into chunks: **1000 chars, 100 overlap** (using `[]rune` for Unicode safety)
4. Save chunks to MySQL `document_vector` table (idempotent: deletes old records first)
5. Generate embeddings via DashScope (OpenAI-compatible protocol)
6. Index to Elasticsearch with vector field

**Alternative splitter:** `pkg/splitter/markdown.go` — header-aware Markdown splitting that preserves section hierarchy context (not used in default pipeline, available for custom use).

### Hybrid Search Pipeline (`internal/service/search_service.go`)

7-step pipeline for each query:

1. **Resolve org tags** — get user's effective org tags including hierarchy
2. **Keyword extraction** — LLM extracts core/optional keywords (3-tier fallback: LLM → `gse` segmenter → simple split)
3. **Cache check** — Redis lookup by normalized core keywords (`search_cache:<kw1>|<kw2>` key, 24h TTL)
4. **Vectorize** — embed raw query via DashScope
5. **Parallel retrieval** — concurrent vector (KNN) + keyword (BM25) search in ES, each recalling 60 docs
6. **RRF fusion** — Reciprocal Rank Fusion (k=60) merges both recall lists
7. **Rerank** — top-50 candidates sent to rerank model (falls back to RRF order if disabled/fails)
8. **Return top-K** — fetch filenames from MySQL, async write cache

ES index name is hardcoded as `knowledge_base` in `executeEsQuery`.

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

## Key Gotchas

1. **ES index mapping:** Must be created with proper vector field mapping before first use. See `pkg/es/client.go`.
2. **Chunk size:** Processor uses **1000 chars / 100 overlap** (not 500/50 as in some docs).
3. **Chinese text:** Always use `[]rune` slicing, never byte slicing.
4. **Rerank disabled:** When `rerank.enable=false`, the client returns identity scores and original order — RRF ranking is preserved.
5. **Seed files:** Place files in `initfile/` directory at project root; they are auto-imported as public documents owned by `admin` user on startup (idempotent via MD5 check).
6. **MinIO bucket:** Must exist before upload. The `minio-init` Docker Compose service creates it automatically.
7. **Kafka consumer:** Runs in a background goroutine with manual offset commit; failures retry up to a threshold before skipping.
