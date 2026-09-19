---
name: go-cli-coding
description: "Use when generating, reviewing, or refactoring idiomatic Go for CLI applications. Follow the repo's .editorconfig, Go formatter and linter settings, and prefer imperative command UX with action-oriented help text and outputs. If no rumdl config exists, suggest one before inventing doc-style rules."
argument-hint: "Go CLI code or review task"
---

# Idiomatic Go CLI Coding

## When to Use

- Generate or refactor Go code for CLI apps.
- Review command structure, flags, help text, and output phrasing.
- Check that code and docs follow repository-specific formatting and lint rules.

## Procedure

1. Inspect the repo conventions before changing code.
   - Follow [.editorconfig](../../../.editorconfig) for indentation, line endings, and file-specific whitespace rules.
   - Follow [.golangci.yaml](../../../.golangci.yaml) for Go linting, formatting, and complexity constraints.
   - Follow [mise.toml](../../../mise.toml) for the local toolchain and task entry points.
2. Keep Go code idiomatic.
   - Prefer small, focused functions and explicit error handling.
   - Use standard library types and naming before adding abstractions.
   - Keep interfaces narrow and define them at the consumer boundary.
   - Preserve package comments, doc comments, and exported identifier clarity.
3. Design CLI UX in imperative style.
   - Name commands and subcommands as verbs or commands that tell the user what to do.
   - Use action-oriented help text, examples, and output messages.
   - Prefer commands like `generate`, `validate`, `sync`, `plan`, or `export` over noun-only command names when the domain allows it.
   - Make error messages specific, direct, and corrective.
4. Match repo formatting and lint expectations.
   - Keep code compatible with `gofmt`, `gofumpt`, `goimports`, and `golangci-lint fmt`.
   - Avoid patterns that would trigger the repo's enabled linters, especially complexity, unused code, unchecked errors, and documentation issues.
   - If prose docs, README text, or generated help copy need style guidance and no rumdl configuration exists, suggest adding one rather than inventing ad hoc rules.
5. Validate the result.
   - Check for formatting, lint, and test impact before finishing.
   - Prefer the repo tasks or toolchain already defined in `mise.toml`.

## Decision Rules

- If a change affects CLI behavior, optimize for clarity over cleverness.
- If a change affects command naming, choose the most imperative, user-actionable wording that still fits the domain.
- If a change affects output, make the primary path terse and the failure path explicit.
- If a change touches docs or help text, keep the tone direct and task-oriented.

## Completion Checks

- Commands read naturally as user actions.
- Go code is idiomatic, lint-friendly, and formatted for this repo.
- Error handling is explicit and checked.
- Help text and examples use imperative language.
- Any missing rumdl policy is called out as a follow-up suggestion instead of being assumed.
