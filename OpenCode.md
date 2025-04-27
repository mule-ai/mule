# Mule AI Project Guidelines

This document provides guidelines for working with the Mule AI codebase.

## Commands

*   **Build:** `make build`
*   **Format:** `make fmt`
*   **Lint:** `make lint` (uses `golangci-lint`)
*   **Run all tests:** `make test`
*   **Run specific test function:** `go test -run '^TestSpecificFunctionName$' ./path/to/package/...`
*   **Tidy dependencies:** `make tidy`
*   **Run application:** `make run`
*   **Clean build artifacts:** `make clean`

## Code Style

*   **Formatting:** Use `gofmt`. Run `make fmt`.
*   **Imports:** Standard Go import grouping (std lib, third-party, local). Enforced by linter.
*   **Naming:** Standard Go conventions (CamelCase for exported, camelCase for unexported).
*   **Types:** Use Go's static typing.
*   **Error Handling:** Use standard `if err != nil { return ..., err }` pattern. Wrap errors with `fmt.Errorf` or similar for context where appropriate.
*   **Linting:** Adhere to `golangci-lint` rules. Run `make lint`.
*   **Dependencies:** Managed via `go.mod`. Run `make tidy` after adding/removing dependencies.
*   **Logging:** Use the structured logger provided in `pkg/log`.

Remember to run `make fmt` and `make lint` before committing changes.
