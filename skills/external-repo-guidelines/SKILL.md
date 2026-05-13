---
name: external-repo-guidelines
description: Inject external repo contributing guidelines as cataractae context. Use when working on fork-mode repos to follow the upstream project's conventions.
---

# External Repo Guidelines

When working on a fork-mode repo, you have access to contributing guidelines extracted from the upstream repository.

## How to Use

1. Check CONTEXT.md for a `## Contributing Guidelines` section. If present, it contains the upstream repo's own `AGENTS.md`, `CONTRIBUTING.md`, and/or `.github/CONTRIBUTING.md`.

2. Follow the repo's conventions for:
   - Coding style and formatting
   - Test commands and frameworks
   - Commit message format
   - PR description format
   - Any other contributing rules

3. When repo-specific conventions conflict with general Cistern patterns, **prioritize the repo's conventions**. For example, if the repo's `CONTRIBUTING.md` specifies `./gradlew test` as the test command, use that instead of a generic `go test ./...`.

4. If the Contributing Guidelines section is empty or missing, proceed with Cistern's default conventions. The absence of guidelines means either the repo is direct-mode or no guideline files were found in the upstream repository.