# Tech Plan: Tool Generation Digital Guardrails

## Context
The tool generation feature allows users to create new tools for the AI agent on the fly. To ensure system security and prevent abuse, we need to implement digital guardrails.

## Solution
1. **Role-Based Access Control (RBAC)**: Restrict tool generation capabilities to users with the `ADMIN` role.
2. **Input Validation**: Implement a `validateRequirement` function in `tool_codegen_service.go` to scan user queries for dangerous patterns (e.g., `rm -rf`, `drop table`, `exec.command`).
3. **Safety Prompting**: Enhance the system prompt for the tool generation LLM call to include explicit safety constraints and prohibitions against generating malicious code.
4. **Logging**: Log unauthorized or suspicious attempts for audit purposes.

## Impact
- `internal/service/tool_codegen_service.go`: Primary implementation site.
- `internal/agent/skills/tool_generation.md`: Updated to include safety guidelines for the LLM.
- `internal/service/chat_service.go`: Uses the updated `tryHandleToolGeneration`.

## Test Plan
- **Unit Tests**:
    - Test `tryHandleToolGeneration` with both `USER` and `ADMIN` roles.
    - Test `validateRequirement` with safe and unsafe inputs.
    - Verify that unauthorized attempts return a polite error message and don't trigger LLM calls.
- **Manual Verification**:
    - Attempt to generate a tool as a non-admin user.
    - Attempt to generate a tool with a malicious requirement (e.g., "create a tool to delete all files").
