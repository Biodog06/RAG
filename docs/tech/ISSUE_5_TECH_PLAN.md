# Technical Plan - Issue #5: Digital Guardrails and Admin Restriction for Tool Codegen

## Context
The current tool generation feature allows any user to trigger code generation and file creation on the server. This poses a security risk and potential for abuse. We need to restrict this feature to `ADMIN` users and add safety checks on the input requirements.

## Solution
1. **Admin Authorization**: Update `tryHandleToolGeneration` in `internal/service/tool_codegen_service.go` to verify the user's role.
2. **Input Validation (Guardrails)**: Implement a `validateToolRequirement` function to check for:
    - Suspicious keywords (e.g., "rm -rf", "delete database", "os.Exit").
    - Extremely long or nonsensical inputs.
    - Out-of-scope requests (e.g., asking for non-tool related logic).
3. **Prompt Hardening**: Refine the system prompt in `generateToolFileFromRequest` to enforce strict safety boundaries and best practices for the generated Eino tool.
4. **Logging**: Log any unauthorized or rejected tool generation attempts for auditing.

## Impact Analysis
- **Service Layer**: `internal/service/tool_codegen_service.go` will be the primary file modified.
- **API Behavior**: Non-admin users will receive a message stating they don't have permission.
- **Safety**: Increased security by preventing unauthorized file writes and malicious code generation.

## Test Plan
- **Unit Tests**:
    - Test `isToolGenerationIntent` with various inputs.
    - Test `validateToolRequirement` with safe and unsafe inputs.
    - Test `tryHandleToolGeneration` with `ADMIN` vs `USER` role. (Mocking dependencies as needed).
- **Manual Verification**: (To be done by reviewer/CI in integrated environment).
