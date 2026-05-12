# ISSUE 6: Performance Optimization - Parallel Query Rewriting & Speculative Return

## Context
Currently, the retrieval process is sequential. The `HybridSearch` function performs query normalization, embedding generation, and Elasticsearch search in a single flow. To improve retrieval quality, we want to introduce "LLM Query Rewriting" which provides a more context-aware and search-friendly version of the user query. However, LLM rewriting is slow. 

To maintain low latency while improving quality, we will implement a "Race Mode":
- **Fast Path**: Search with the original user query (normalized).
- **Slow Path**: Search with an LLM-rewritten query.
- **Speculative Return**: If the Fast Path returns high-confidence results quickly, we return them immediately and cancel the Slow Path.

## Solution

### 1. Data Model & Configuration
- Add `llm.Client` dependency to `searchService`.
- Add `ConfidenceThreshold` to `config.yaml` (default: 0.95).
- Add `SpeculativeTimeout` to `config.yaml` (default: 200ms).

### 2. LLM Query Rewriting
- Implement `rewriteQuery(ctx, query)` using `llmClient`.
- Prompt: "Rewrite this user query for better document retrieval in a RAG system. Return ONLY the rewritten query."

### 3. Parallel Execution Logic in `HybridSearch`
- Use `context.WithCancel` for the slow path.
- Start two goroutines:
  - **Goroutine A (Fast Path)**: 
    1. Perform normalization.
    2. Perform embedding generation.
    3. Perform ES search.
    4. If top result score > `ConfidenceThreshold`, send results to a `done` channel.
  - **Goroutine B (Slow Path)**:
    1. Rewrite query with LLM.
    2. Perform normalization on rewritten query.
    3. Perform embedding generation.
    4. Perform ES search.
    5. Send results to the `done` channel.
- Use `select` with a timer for speculative return:
  - If `done` returns Fast Path results within `SpeculativeTimeout` AND score > threshold: Return them, cancel Slow Path.
  - If `done` returns Fast Path results but score < threshold: Wait for Slow Path.
  - If Slow Path finishes: Return Slow Path results.

### 4. Merging Results (Fallback)
- If Slow Path results are available, they are preferred if their score is higher or if Fast Path results were poor.
- In this initial implementation, we will likely just return the first high-confidence result or the slow path result if it finishes.

## Impact
- `internal/service/search_service.go`: Main implementation site.
- `internal/config/config.go`: Configuration struct update.
- `cmd/server/main.go`: Wire up `llmClient` to `SearchService`.

## Test Plan
- Unit test for `HybridSearch` with mocked `embeddingClient`, `esClient`, and `llmClient`.
- Test case 1: Fast path returns high score result quickly -> Slow path cancelled.
- Test case 2: Fast path returns low score result -> Wait for slow path.
- Test case 3: Fast path times out -> Wait for slow path.
