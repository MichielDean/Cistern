# Design Brief: Remove all non-opencode providers from core

## Requirements Summary

Remove claude, ollama-claude, codex, gemini, and copilot provider presets from preset.go. Make opencode the only and default provider (replace claude as fallback). Remove legacy buildClaudeCmd and claudePath from session.go. Remove SupportsAddDir/--add-dir path (--add-dir is Claude-specific). Remove --resume and --continue flags from preset (Claude-specific). Simplify InstrFile() to always return AGENTS.md instead of defaulting to CLAUDE.md. Remove ollama-claude preset. Remove PromptFileTemplate CATARACTAE.md references for codex/copilot. Update proc.go to remove claude/codex agent detection (keep opencode). Update scheduler.go to remove hardcoded CLAUDE.md references. Update drought_hooks.go CLAUDE.md fallback. Update context.go to remove provider-specific instruction file logic (always AGENTS.md). Update aqueduct/provider.go to default to opencode and simplify provider resolution. Remove NonInteractive.PrintFlag and AllowedToolsFlag (Claude-specific). Goal: reduce surface area so only opencode is supported as a provider.

## Existing Patterns to Follow

### Naming Conventions

- Exported functions use PascalCase; unexported structs use lowercase field names. See `internal/provider/preset.go:42-104` (`ProviderPreset` exported, all fields PascalCase because the struct is exported).
- Error wrapping uses `fmt.Errorf("pkg: context: %w", err)`. See `internal/provider/preset.go:256` (`fmt.Errorf("provider: read %s: %w", path, err)`) and `internal/aqueduct/provider.go:50` (`fmt.Errorf("aqueduct: unknown provider %q", name)`).
- Variables for test injection use `Fn` suffix (e.g., `resolveCommandFn`). See `internal/cataractae/session.go:238`.

### Error Handling

- Errors are wrapped with domain context using `fmt.Errorf("pkg: context: %w", err)`. See `internal/provider/preset.go:256`, `internal/aqueduct/parse.go:16`.
- Operational errors use `slog.Error`/`slog.Warn`. See `internal/cataractae/session.go:351-354` (`slog.Default().Error("spawn: failed to write prompt file", ...)`).
- No `fmt.Fprintf(os.Stderr)` outside of CLI command files (`cmd/ct/`).

### Logging

- Uses `slog.Default().Info/Warn/Error` with structured key-value pairs. See `internal/cataractae/session.go:102-109`, `internal/castellarius/scheduler.go:1763-1775`.
- CLI commands in `cmd/ct/` use `fmt.Fprintf(os.Stderr, ...)` for user-facing output. See `cmd/ct/evaluate.go:193-196`.

### Testing

- Table-driven tests using `t.Run(name, func(t *testing.T) {...})`. See `internal/cataractae/smoke_test.go:30-108`, `internal/provider/preset_test.go:82-106`.
- Test helpers use `t.Helper()`. See `internal/cataractae/smoke_test.go:208-216` (`builtinPreset`).
- Variable overrides for test isolation: see `internal/cataractae/session.go:238` (`resolveCommandFn`), `session.go:574` (`claudePathFn`), `session.go:243` (`execTmuxNewSession`).
- `t.Setenv` for environment variable tests. See `internal/cataractae/smoke_test.go:21` (`t.Setenv("CLAUDE_PATH", "claude")`).

### Idiom Fit

- Constructor-style initialization via `NewXxx` functions. See `internal/evaluate/evaluate.go:101` (`NewLLMCaller`).
- Package-level function variables for test injectability. See `internal/cataractae/session.go:238,243,250,574`.
- `slices.IndexFunc` for finding items in slices. See `internal/aqueduct/provider.go:33,46`.

## Reusability Requirements

- `NonInteractiveConfig` fields `PrintFlag` and `AllowedToolsFlag` are **Claude-specific** and will be removed from the struct. After removal, only `Subcommand` and `PromptFlag` remain — both are generic and used by opencode's `NonInteractive` config.
- `ResumeStyle` type and `ResumeStyleSubcommand` constant are **codex-specific**. Remove the type and both constants (`ResumeStyleFlag`, `ResumeStyleSubcommand`).
- `ContinueFlag` field on `ProviderPreset` is **Claude-specific**. Remove the field.
- `AddDirFlag` and `SupportsAddDir` fields on `ProviderPreset` are **Claude-specific** (only claude/ollama-claude presets set `AddDirFlag` and `SupportsAddDir: true`). Remove both fields. Context injection for opencode uses `PromptFileTemplate` instead — see `internal/cataractae/session.go:372-408`.
- `ResumeFlag` field on `ProviderPreset` is used by `cmd/ct/filter.go:102` for the filter feature's resume path. Opencode does not set ResumeFlag. **Keep the field** — custom user presets may still need it, and the filter code depends on it with a fallback to `"--resume"` at `filter.go:103-104`.
- `LLMCaller` in `internal/evaluate/evaluate.go:90-99` uses `PrintFlag` and `AllowedToolsFlag` via its constructor `NewLLMCaller`. These fields must be removed from `LLMCaller` too, along with the corresponding constructor parameter and usage at `evaluate.go:139-144`.

## Coupling Requirements

No new shared mutable package-level state is introduced. Current package-level vars that are being **removed**:

- `internal/cataractae/session.go:574` — `claudePathFn` variable. **Delete**.
- `internal/cataractae/session.go:584-593` — `claudePath` function. **Delete**.

Package-level vars that are **kept** (they support test injectability for remaining functionality):

- `resolveCommandFn` — `session.go:238` — used by `buildPresetCmd` for path resolution.
- `execTmuxNewSession` — `session.go:243` — used by `spawn` for tmux interaction.
- `execTmuxKillServer` — `session.go:250` — used by dead-server recovery.
- `execTmuxPipePaneCmd` — `session.go:261` — used by session logging.
- `protocolSkillPathFn` — `parse.go:119` — used by skill injection.

The `builtins` var at `preset.go:115` is a package-level `[]ProviderPreset` slice. It is read-only after initialization. No mutation occurs. **Keep** — it is an immutable table, not shared mutable state.

## DRY Requirements

No new repeated patterns are introduced. The removal is simplification — eliminating code paths rather than adding them.

The following patterns become unnecessary and are removed entirely:

1. **buildClaudeCmd** at `session.go:219-233` — single use, deleted.
2. **claudePath** at `session.go:584-593` — single use, deleted.
3. **CLAUDE.md backward compat** in `parse.go:61-63` — removed along with the guard logic.
4. **SupportsAddDir branching** in `session.go:470-491` and `session.go:496` — simplified: opencode always uses the prompt-injection path (no AddDir).

## Migration Requirements

This change has no database migrations. It is purely Go code restructuring — removing code, not adding tables or columns.

However, there is a **file system migration concern**: existing deployments may have `CLAUDE.md` files in `~/.cistern/cataractae/<identity>/`. The `GenerateCataractaeFiles` function will now write `AGENTS.md` instead. The brief prescribes:

1. The default `instructionsFile` changes from `"CLAUDE.md"` to `"AGENTS.md"` at `parse.go:66`.
2. Remove the CLAUDE.md backward-compatibility guard at `parse.go:61-63` (since claude provider no longer exists, there is no reason to preserve stale CLAUDE.md files).
3. The `ensureCataractaeIntegrity` function at `scheduler.go:1743-1777` must check for `AGENTS.md` instead of `CLAUDE.md`.
4. The `drought_hooks.go:399` must stat `AGENTS.md` instead of `CLAUDE.md`.

## Test Requirements

All test files must be updated to reflect the removal. The following test files require changes (see API Surface Checklist for specific test function names):

### Test naming convention
Tests use `TestXxx_YyyZzz` format. See `internal/proc/proc_test.go:96` (`TestClaudeAliveUnderPIDIn_EmptyPID_ReturnsFalse`), `internal/provider/preset_test.go:11` (`TestBuiltins_ReturnsExpectedPresetNames`).

### Specific test updates required

| File | Change |
|------|--------|
| `internal/provider/preset_test.go` | Delete `TestBuiltins_ClaudePreset` (line 30), `TestBuiltins_CodexPreset` (line 43), `TestBuiltins_GeminiPreset` (line 54), `TestBuiltins_CopilotPreset` (line 65). Update `TestBuiltins_ReturnsExpectedPresetNames` line 12: `want` becomes `[]string{"opencode"}`. Remove claude/codex/gemini/copilot rows from NonInteractive table (lines 93-96), SupportsAddDir table (lines 117-120). Rename `TestResolvePreset_DefaultsToClaudeWhenEmpty` (line 426) to `TestResolvePreset_DefaultsToOpencodeWhenEmpty`. Update default assertion from `"claude"` to `"opencode"`. Rename `TestResolvePreset_UnknownNameFallsBackToClaude` (line 446) similarly. Update all test tables that reference removed preset names. |
| `internal/proc/proc_test.go` | Remove claude/codex entries from `IsAgentCmdline` table (lines 75-80). Remove "claude" and "codex" from agent name loops in `TestClaudeAliveUnderPIDIn_*` tests (lines 115-118, 134, 152). Rename all `TestClaudeAliveUnderPIDIn_*` functions to `TestAgentAliveUnderPIDIn_*`. |
| `internal/cataractae/smoke_test.go` | Remove `t.Setenv("CLAUDE_PATH", "claude")` at line 21. Delete test cases for "claude" (line 41), "codex" (line 50), "gemini" (line 60), "copilot" (line 70). Keep "opencode" and "custom user preset". |
| `internal/cataractae/session_test.go` | Remove buildClaudeCmd tests (lines 56-76, 84-92). Update all `CLAUDE.md` references to `AGENTS.md`. Remove `CLAUDE_PATH` env var usage (lines 189, 197, 406-407, 926). Remove codex-specific test cases (lines 554, 564, 600). |
| `internal/cataractae/context_test.go` | Update GEMINI.md/CLAUDE.md exclusion test at line 768 to use AGENTS.md only. |
| `internal/aqueduct/provider_test.go` | Update `TestResolveProvider_DefaultsToClaudeWhenNoProvider` (line 13) to expect "opencode" as default. Update codex/gemini test cases. |
| `internal/aqueduct/workflow_test.go` | Update CLAUDE.md references to AGENTS.md. Remove backward-compat tests at lines 558-602. Update default instructionsFile test at line 607 to expect AGENTS.md. |
| `cmd/ct/doctor_test.go` | Remove claude/codex/gemini/copilot install hint rows from `TestProviderInstallHint_KnownPreset_ReturnsHint`. Remove codex-specific env/CLAUDE.md tests. Update `checkClaudeMdIntegrity` calls to use renamed function and AGENTS.md. |
| `cmd/ct/castellarius_test.go` | Remove `usesClaude` tests (lines 520-573). |
| `cmd/ct/init_test.go` | Update CLAUDE.md references to AGENTS.md (lines 102-109). Remove claude OAuth references (lines 495-503). |
| `internal/castellarius/drought_hooks_test.go` | Update CLAUDE.md references to AGENTS.md (lines 183-247). |

### Delete entire test file

- `internal/testutil/fakeclaude/main.go` — entire file. This binary exists solely to produce a `/proc/<pid>/cmdline` whose argv[0] is "claude" for testing `ClaudeAliveUnderPIDIn`. No longer needed.

## Forbidden Patterns

- Inline string constants for instruction filenames in scheduler/drought_hooks code — must use `preset.InstrFile()` or the constant `"AGENTS.md"` consistently. Currently `scheduler.go:1760` and `drought_hooks.go:399` hardcode `"CLAUDE.md"` — must not repeat this pattern with a different hardcoded string. Use `providerPreset.InstrFile()` where a preset is available, or the constant where it is not.
- Package-level mutable vars for config — existing `claudePathFn` at `session.go:574` is being removed, not replicated.
- Silently swallowing errors — existing pattern at `drought_hooks.go:428-431` logs a warning and falls back. This is acceptable. Do not remove the warning.
- Mixed CLAUDE.md/AGENTS.md references in the same code path — after this change, only AGENTS.md should appear.

## API Surface Checklist

### Provider Preset (`internal/provider/preset.go`)

- [ ] `builtins` var (line 115) contains exactly one entry: the opencode preset. All other entries (claude, ollama-claude, codex, gemini, copilot) are removed.
- [ ] `InstrFile()` (line 107) returns `"AGENTS.md"` when `InstructionsFile` is empty, not `"CLAUDE.md"`.
- [ ] `ResolvePreset("")` (line 229) falls back to `"opencode"` instead of `"claude"`.
- [ ] `NonInteractiveConfig.PrintFlag` field is removed from the struct. No preset uses it after claude removal.
- [ ] `NonInteractiveConfig.AllowedToolsFlag` field is removed from the struct. No preset uses it after claude removal.
- [ ] `ResumeStyle` type, `ResumeStyleFlag` constant, and `ResumeStyleSubcommand` constant (lines 32-40) are removed. Only codex used `ResumeStyleSubcommand`.
- [ ] `ContinueFlag` field (line 83) is removed from `ProviderPreset`. Only claude/ollama-claude set it.
- [ ] `AddDirFlag` field (line 58) is removed from `ProviderPreset`. Only claude/ollama-claude set it.
- [ ] `SupportsAddDir` field (line 103) is removed from `ProviderPreset`. Only claude/ollama-claude set it to true.
- [ ] `ResumeFlag` field (line 76) is **kept** — used by `cmd/ct/filter.go:102` for the filter resume feature, with fallback to `"--resume"` at line 103-104.
- [ ] `Builtins()` returns a slice with exactly one entry. Contract: `len(Builtins()) == 1 && Builtins()[0].Name == "opencode"`.
- [ ] `MergePresets(base, overrides)` still works — overridden entries matching by Name, unknown names appended.
- [ ] `LoadUserPresets(path)` still works — reads JSON, merges onto builtins. Preserves the extension point for custom user presets.
- [ ] `ResolvePreset("opencode")` returns the opencode preset. `ResolvePreset("unknown")` falls back to opencode (not claude). `ResolvePreset("")` returns opencode.

### Session (`internal/cataractae/session.go`)

- [ ] `buildClaudeCmd` method (lines 219-233) is deleted.
- [ ] `claudePathFn` variable (line 574) is deleted.
- [ ] `claudePath` function (lines 584-593) is deleted.
- [ ] The `else` branch at lines 91-93 in `spawn()` (`s.buildClaudeCmd(skillsDir)`) is removed. `s.Preset.Name` is always non-empty after provider resolution defaults to opencode.
- [ ] The `cmdPath := claudePathFn()` call at line 97 is removed. Only `resolveCommandFn(s.Preset.Command)` remains.
- [ ] `presetBaseParts` (line 305) no longer appends `AddDirFlag` — remove the `if preset.AddDirFlag != ""` block at lines 313-315.
- [ ] `buildPrompt` (line 464) removes the `SupportsAddDir` branching. The `if !s.Preset.SupportsAddDir` block at lines 470-480 becomes the only path. Remove the `else` branch at lines 482-491.
- [ ] `buildPrompt` removes the `SupportsAddDir` guard at line 496. Skills are always injected as text (opencode has no AddDirFlag).
- [ ] `Session.Identity` comment at line 29 is updated: "Used to locate cataractae/<identity>/AGENTS.md" instead of "CLAUDE.md".
- [ ] `Session.TemplateCtx` comment at line 45 is updated: "render AGENTS.md as a Go template" instead of "CLAUDE.md".
- [ ] `Session.Preset` comment at line 37 is updated: remove "When Name is empty, spawn falls back to the legacy claude hard-coded path."
- [ ] `buildPresetCmd` comment at line 336 is updated: remove "e.g. claude uses -p" reference.

### Process Detection (`internal/proc/proc.go`)

- [ ] `ClaudeAliveUnderPIDIn` function (line 16) is renamed to `AgentAliveUnderPIDIn`. Parameter signature unchanged.
- [ ] `IsAgentCmdline` (line 94) removes "claude", "codex" from the switch and removes the `strings.HasPrefix(base, "claude-")` default case (line 107). Only `"opencode"` remains.
- [ ] `IsClaudeCmdline` function (line 113) is deleted entirely — backward-compat wrapper no longer needed.
- [ ] `AgentAliveUnderPIDIn` returns true only when an opencode process descendant is found.

### Cataractae Integrity (`internal/castellarius/scheduler.go`)

- [ ] `ensureCataractaeIntegrity` (line 1743) reads `AGENTS.md` instead of `CLAUDE.md`. Variable renamed from `claudePath` to `agentsPath` (line 1760). Path: `filepath.Join(cataractaeDir, identity, "AGENTS.md")`.
- [ ] Log messages at lines 1763, 1771, 1773, 1775 reference `AGENTS.md` instead of `CLAUDE.md`.
- [ ] Comment at line 379 references `AGENTS.md` instead of `CLAUDE.md`.

### Drought Hooks (`internal/castellarius/drought_hooks.go`)

- [ ] Line 399: variable renamed from `claudePath` to `agentsPath`, path uses `"AGENTS.md"` instead of `"CLAUDE.md"`.
- [ ] Line 430: log message references `AGENTS.md` instead of `CLAUDE.md`.
- [ ] Lines 77, 251: comments reference `AGENTS.md` instead of `CLAUDE.md`.

### Cataractae Context (`internal/cataractae/context.go`)

- [ ] Line 54: `InstructionsFile` comment simplified to `"e.g. "AGENTS.md" for opencode"`. Remove `"CLAUDE.md" for claude, "GEMINI.md" for gemini` references.

### Provider Resolution (`internal/aqueduct/provider.go`)

- [ ] Line 27: default name changes from `"claude"` to `"opencode"`.
- [ ] Line 17: comment updated — `"opencode"` instead of `"claude"` as the default.
- [ ] Lines 58-59: comments updated — remove gemini/codex examples, use generic language.

### Cataractae File Generation (`internal/aqueduct/parse.go`)

- [ ] Line 66: default `instructionsFile` changes from `"CLAUDE.md"` to `"AGENTS.md"`.
- [ ] Lines 54-55: doc comment updated — `"AGENTS.md"`, not `"CLAUDE.md"`.
- [ ] Lines 61-63: CLAUDE.md backward-compatibility doc comment removed entirely.

### Aqueduct Types (`internal/aqueduct/types.go`)

- [ ] Line 98: `Name` comment updated — `"e.g. "opencode"` instead of `"claude", "opencode"`. Default reference changed from `"claude"` to `"opencode"`.

### Doctor Command (`cmd/ct/doctor.go`)

- [ ] `providerInstallHint` (line 429): remove "claude", "codex", "gemini" cases (lines 431-436). Keep "opencode" case (lines 437-438).
- [ ] `inferLLMProviderFromPreset` (line 446): remove "claude", "codex", "gemini", "copilot" cases (lines 448-457). Keep "opencode" → "ollama" case (lines 454-455).
- [ ] `checkClaudeMdIntegrity` (line 462) is renamed to `checkInstructionsFileIntegrity`. Parameter unchanged (`path string`). Error messages reference "instructions file" instead of "CLAUDE.md". Function body unchanged (reads file, checks for sentinel).

### Castellarius Startup (`cmd/ct/castellarius.go`)

- [ ] `startupRequiredEnvVars` (line 675): remove `usesClaude` return value and concept. Function signature changes to `func startupRequiredEnvVars(cfgPath string) []string`. Remove `usesClaude` variable (line 691). When no repos resolved, return `nil` instead of `nil, true` (line 701-703). All callers must be updated.

### Evaluate Package (`internal/evaluate/evaluate.go`)

- [ ] `LLMCaller` struct (line 90): remove `PrintFlag` and `AllowedToolsFlag` fields.
- [ ] `NewLLMCaller` constructor (line 101): remove `printFlag` and `allowedToolsFlag` parameters.
- [ ] `Call` method (lines 116+): remove the `c.PrintFlag` and `c.AllowedToolsFlag` usage at lines 139-144.

### Fake Agent (`internal/testutil/fakeagent/main.go`)

- [ ] Comments updated: remove claude-specific flag references (lines 4, 110, 112).
- [ ] Line 124: remove `"claude auth status"` handling or adapt for opencode.

### Non-Go Files

- [ ] `.gitignore` (line 4): remove `.claude/` entry.
- [ ] `docker-entrypoint.sh` (lines 9-13): remove `~/.claude` symlink setup.
- [ ] `README.md`: remove CLAUDE.md, GEMINI.md, claude/codex/gemini/copilot provider references. Simplify provider documentation to show only opencode.
- [ ] `cataractae/delivery/INSTRUCTIONS.md` (line 194): update "CLAUDE.md for claude, GEMINI.md for gemini" to just "AGENTS.md".
- [ ] `skills/cistern/references/commands.md` (lines 292, 298, 534): update CLAUDE.md/GEMINI.md references.
- [ ] `skills/cistern/SKILL.md` (line 176): remove `~/.claude/.credentials.json` reference.
- [ ] `internal/skills/cistern-git/SKILL.md` (lines 16-17, 43): simplify "AGENTS.md for opencode/codex, CLAUDE.md for claude" to just "AGENTS.md".

### CLAUDE.md Backward Compatibility in parse.go

- [ ] Remove the CLAUDE.md preservation logic. After removing claude provider, there is no scenario where a user switches back to claude. The current code at `parse.go:61-63` says "when instructionsFile differs from CLAUDE.md, any pre-existing CLAUDE.md is left untouched". This guard is now dead code since `instructionsFile` will always be `"AGENTS.md"`. **Delete** this conditional and any corresponding test code (`workflow_test.go:558-602`).

### Spawn Path: Remove Legacy Fallback

- [ ] In `session.go:spawn()`, after the removal of `buildClaudeCmd`, the `if s.Preset.Name != ""` / `else` branching at lines 86-93 simplifies. Since `Preset` is always populated (via `ResolvePreset` which defaults to opencode), the else branch is unreachable. **Remove** the else branch and the conditional guard — `buildPresetCmd` is always called.

### SupportsAddDir Removal: Prompt Building Simplification

- [ ] In `session.go:buildPrompt()`, remove the `if !s.Preset.SupportsAddDir` conditional at line 470 and its `else` branch at lines 481-491. Since opencode does not support AddDir, the prompt-preamble path (lines 473-480) is the only code path.
- [ ] Remove the `if !s.Preset.SupportsAddDir && len(s.Skills) > 0` guard at line 496. Simplify to `if len(s.Skills) > 0` — skills are always injected as text.

### Files to Delete

- [ ] `internal/testutil/fakeclaude/main.go` — entire file. Produced a fake "claude" binary for proc tests. No longer needed.