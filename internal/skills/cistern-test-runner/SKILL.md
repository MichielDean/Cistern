---
name: cistern-test-runner
description: Test execution protocol for Cistern cataractae. Detects repo stack and runs the correct test/build commands. Use whenever a cataractae needs to verify code correctness before signaling.
---

# Cistern Test Runner

## Detect the Stack

Check for these files in the repo root:
- `go.mod` → Go
- `package.json` → Node/TypeScript
- `pytest.ini`, `setup.py`, `pyproject.toml` with pytest → Python
- `Makefile` with test target → Make

## Build Commands

| Stack | Build |
|-------|-------|
| Go | `go build ./...` |
| Node/TS | `npm run build` or `tsc --noEmit` |
| Python | (no compile step) |
| Make | `make build` |

## Test Commands

| Stack | Test |
|-------|------|
| Go | `go test ./...` |
| Node/TS | `npm test` |
| Python | `pytest` |
| Make | `make test` |

## Lint Commands

| Stack | Lint |
|-------|------|
| Go | `go vet ./...` |
| Node/TS | `npm run lint` |
| Python | `ruff check .` |
| Make | `make lint` |

## Rules

1. Always run build before tests
2. Run the full test suite (no `-run` filtering) unless diagnosing a specific failure
3. Signal pass only after all tests pass
4. Failing tests = automatic recirculate (for review/QA cataractae) or must-fix (for implementer)