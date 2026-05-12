## Description
Currently, the `HybridSearch` process sequentially waits for the LLM to finish Query Rewriting before proceeding to retrieval. This introduces a significant (~1s) overhead for simple, unambiguous queries that could be answered by the original query alone.

This task aims to implement a "Race Mode" or "Speculative Execution" pattern to reduce latency for clear queries.

## Acceptance Criteria
- [ ] **Concurrent Execution**: Modify `HybridSearch` to start the 'Original Query Search' (Fast Path) and 'LLM Query Rewrite' (Slow Path) simultaneously.
- [ ] **Short-circuiting Logic**: Implement a confidence score threshold (e.g., 0.95).
- [ ] **Speculative Return**: If the Fast Path returns results above the threshold within a defined timeout (e.g., 100-200ms), return those results immediately.
- [ ] **Resource Management**: Properly cancel the LLM rewrite context (`context.WithCancel`) when an early return is triggered to save tokens and compute.
- [ ] **Robust Fallback**: Ensure that if the Fast Path results are insufficient, the system gracefully waits for the Slow Path to complete.

## Technical Notes
- Implementation should be localized in `internal/service/search_service.go`.
- Consider using a `select` statement with channels to coordinate the results from the two paths.
- The `ConfidenceThreshold` should be configurable in `config.yaml`.
