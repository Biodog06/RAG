---
name: create-unit-tests
description: Design, create, and modify unit tests as black-box behavioral checks using AAA and FIRST principles with test doubles for external dependencies. Use when asked to add new unit tests, update existing unit tests after production-code changes, refactor brittle or flaky unit tests, improve coverage, fix failing unit tests, review unit-test quality, or explain unit-testing strategy in any language/framework and regardless of methodology (TDD, BDD, or test-after).
---

# Creating Unit Tests

## Goal

Produce deterministic, maintainable unit tests that verify one observable behavior at a time while keeping the unit isolated from external systems.

## Operating Rules

- The skill MUST treat the unit under test as a black box and MUST validate contracts via inputs, outputs, observable state, and externally visible interactions.
- The skill MUST follow `FIRST`: keep tests fast, independent, repeatable, self-validating, and timely.
- The skill MUST structure every test as `Arrange -> Act -> Assert`, with one business action in `Act`.
- Each test MUST validate one behavior; unrelated behaviors MUST be split into separate tests.
- The skill MUST use test doubles (`stub`, `mock`, `fake`, `spy`) for process-external dependencies (database, network, filesystem, clock, randomness, queues, environment).
- The skill MUST keep boundaries explicit between `unit`, `integration`, `contract`, and `e2e` tests.
- Assertions MUST be specific and behavior-focused.
- The skill SHOULD prefer behavior-level checks over implementation-detail checks unless the implementation detail is itself part of the contract.
- The skill MUST NOT mock the subject under test.
- The skill SHOULD avoid mock overuse and weak interaction-only tests when outcome assertions can express the behavior more directly.
- The skill SHOULD keep test data readable, minimal, and local where practical.
- The skill SHOULD organize tests so affected tests are easy to locate from the changed production code, usually by following the surrounding project structure or convention.
- The skill MUST avoid coupling to implementation details (private methods, internal call order unless order is contractually required, incidental data structures).

## Workflow

1. The skill MUST define the behavior contract before writing assertions.
2. The skill SHOULD build a scenario matrix covering happy path, boundary values, invalid input, dependency failure, and idempotency or ordering when relevant.
3. The skill MUST prepare minimal fixtures and isolate dependencies with test doubles.
4. The skill MUST implement tests with explicit naming (`should_<expected_behavior>_when_<condition>` or equivalent project convention).
5. The skill MUST run tests and harden against flakiness by removing time, order, randomness, and global-state dependencies. The skill MUST NOT use `sleep()` as synchronization when stronger synchronization is feasible.
6. The skill SHOULD refactor test structure only after tests are green and readable.

## Coverage Guidance

- The skill MUST NOT treat coverage percentage alone as evidence of sufficient regression protection.

## Pseudo-Code Template

```text
test "should_return_error_when_dependency_times_out":
  Arrange:
    dependency_stub := timeout_error
    unit := create_unit(dependency=dependency_stub)
    input := valid_request()
  Act:
    result := unit.execute(input)
  Assert:
    expect(result.error_code).to_equal("DEPENDENCY_TIMEOUT")
    expect(result.data).to_be_empty()
```

## Methodology Neutrality

- These rules MUST apply for any delivery style: TDD, BDD, or test-after implementation.
- If project methodology is unspecified, the skill MUST default to behavior-first test design and explicit scenario coverage.

## References

- Read `.agents/skills/create-unit-tests/references/unit-testing-guide.md` for detailed heuristics, anti-patterns, pseudo-code patterns, and review checklists.
