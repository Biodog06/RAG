# Unit Testing Guide (Technology-Agnostic)

## Table of Contents

1. Purpose
2. Unit-Test Scope
3. Isolation and Test Doubles
4. Scenario Design
5. AAA Pattern
6. Assertion Strategy
7. Determinism and Flakiness Prevention
8. Maintainability and Refactoring
9. Methodology Mapping (TDD/BDD/Test-After)
10. Review Checklist

## 1. Purpose

Use this guide when creating or modifying unit tests. The goal is to validate a single unit's behavior in isolation, with deterministic results and clear diagnostics on failure.

## 2. Unit-Test Scope

A test is a unit test when all conditions below are true:

- It exercises one unit (function, method, class, module-level behavior).
- It validates observable behavior (output, state transition, emitted effect, externally visible interaction).
- It replaces external dependencies with test doubles.
- It runs quickly and independently of execution order.

Exclude from unit tests:

- Real network calls.
- Real database/filesystem usage (unless explicitly defined as an in-memory double).
- Cross-service or end-to-end workflows.

## 3. Isolation and Test Doubles

Use doubles intentionally:

- `Stub`: returns controlled data for a dependency call.
- `Mock`: verifies interaction contract (call count, arguments, sequence when required).
- `Fake`: lightweight working implementation (for example, in-memory repository).
- `Spy`: records interactions for later assertions.

Selection rule:

- Need deterministic input -> use `Stub` or `Fake`.
- Need interaction verification -> use `Mock` or `Spy`.
- Need both -> combine minimal doubles, avoid over-mocking.

Pseudo-code:

```text
Arrange:
  payment_gateway_stub := returns_success(transaction_id="tx-123")
  notifier_spy := records_messages()
  unit := create_service(gateway=payment_gateway_stub, notifier=notifier_spy)
```

## 4. Scenario Design

Design a minimal scenario matrix per behavior:

- Happy path: valid input and expected success.
- Boundary path: edge values and limits.
- Validation path: invalid or missing input.
- Failure path: dependency error/timeout/rejection.
- Contract path: side effects and interaction obligations.

Keep one behavior per test. If two assertions belong to different behaviors, split into separate tests.

## 5. AAA Pattern

Use `Arrange -> Act -> Assert` consistently.

- `Arrange`: prepare data, doubles, and unit configuration.
- `Act`: execute one business action.
- `Assert`: verify expected behavior only.

Pseudo-code:

```text
test "should_reject_request_when_amount_is_negative":
  Arrange:
    unit := create_unit()
    input := request(amount=-1)
  Act:
    result := unit.execute(input)
  Assert:
    expect(result.error_code).to_equal("INVALID_AMOUNT")
```

## 6. Assertion Strategy

Prefer assertions with strong signal:

- Assert externally visible outcomes, not internal implementation details.
- Assert exact contract values for critical behavior.
- Assert interaction details only when they are business-relevant.
- Keep assertions focused; avoid large assertion blocks that hide failure reason.

Bad pattern:

- Verifying private/internal calls with no contract value.
- Asserting many unrelated fields in one test.

Better pattern:

- One test for one behavior, with minimal but decisive assertions.

## 7. Determinism and Flakiness Prevention

Remove unstable inputs from tests:

- Control time with clock doubles.
- Control randomness with deterministic seed/provider.
- Avoid shared mutable state between tests.
- Avoid dependence on execution order.
- Avoid sleeps and timing-based assertions unless unavoidable; if unavoidable, isolate and bound them.

Pseudo-code:

```text
Arrange:
  fixed_clock := time_provider("2026-01-01T10:00:00Z")
  random_stub := returns_sequence([42])
  unit := create_unit(clock=fixed_clock, random=random_stub)
```

## 8. Maintainability and Refactoring

Keep tests easy to evolve with production code:

- Prefer descriptive, behavior-based names.
- Use helper/builders only when they reduce repetition without hiding intent.
- Avoid branching logic in tests.
- Refactor duplication after behavior is covered and tests are green.
- Keep setup local to each test unless shared setup clearly improves readability.

## 9. Methodology Mapping (TDD/BDD/Test-After)

These rules are methodology-neutral:

- `TDD`: write failing test first, then implement.
- `BDD`: express behavior in domain language, still keep AAA internals.
- `Test-after`: add/adjust tests immediately after code change, before finalizing.

Independent of methodology, maintain:

- isolated unit scope,
- explicit scenario coverage,
- deterministic execution,
- behavior-first assertions.

## 10. Review Checklist

Use this checklist before finishing:

- Test validates one behavior.
- `Act` has one business action.
- No real external dependency is used.
- Failure message is understandable and actionable.
- Test is deterministic and order-independent.
- Assertions verify contract, not incidental internals.
- Names and structure are readable for a new contributor.
