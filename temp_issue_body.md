## Description
Add digital guardrails to the tool generation service (`internal/service/tool_codegen_service.go`) to prevent unauthorized or unsafe tool creation. The feature should be restricted to users with the `ADMIN` role only.

## Acceptance Criteria
- [ ] Tool generation intent is only processed if the user has `ADMIN` role.
- [ ] Implement input validation (digital guardrail) to detect and reject potentially malicious or out-of-scope requirements.
- [ ] Add safety constraints to the system prompt for tool generation to ensure generated code follows best practices.
- [ ] Log unauthorized attempts to generate tools.

## Technical Notes
- Modify `tryHandleToolGeneration` in `internal/service/tool_codegen_service.go` to check `user.Role`.
- Add a `validateRequirement` function to check for suspicious patterns in the input.
- Update `systemPrompt` in `generateToolFileFromRequest` to include more strict safety guidelines.
