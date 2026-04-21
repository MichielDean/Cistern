# Design Brief: Promote Scheduler Events from Notes to Structured Events

## Requirements Summary

Replace six categories of scheduler-sourced `AddNote()` calls with structured `RecordEvent()` calls, giving each failure mode a distinct event type with a JSON payload. The `loop-recovery-pending` marker notes are retained as notes because `loopRecoveryPendingCount()` queries the notes table. The `ct droplet log` command must display the new event types meaningfully. The circuit breaker must stop scanning notes for exit-no-outcome markers and instead query events.

## Existing Patterns to Follow

### ORM / Query

The codebase uses raw `database/sql` with parameterized queries — no ORM. Queries use `?` placeholders for SQLite. See `internal/cistern/client.go:637-639` for the `RecordEvent` INSERT pattern:

```go
_, err := exec.Exec(
    `INSERT INTO events (droplet_id, event_type, payload, created_at) VALUES (?, ?, ?, ?)`,
    id, eventType, payload, time.Now().UTC(),
)
```

Event reads use `c.db.Query` with `rows.Scan` — see `GetDropletChanges` at `internal/cistern/client.go:1252-1277`.

For the circuit breaker replacement, a new query method must follow the same raw-SQL pattern. The existing `ListRecentEvents` (`internal/cistern/client.go:1217-1238`) shows the query-and-scan pattern for events.

### Naming Conventions

Event type constants use `Event` prefix with PascalCase, mapped to lowercase snake_case string values — see `internal/cistern/client.go:21-32`:

```go
EventCreate      = "create"
EventDispatch    = "dispatch"
EventPass        = "pass"
EventRecirculate = "recirculate"
```

New constants **must** follow this exact pattern: `EventExitNoOutcome = "exit_no_outcome"`, etc. The string values use snake_case (underscores, not hyphens) consistent with the existing convention.

The `validEventTypes` map (`internal/cistern/client.go:34-45`) must be updated to include all new types.

Unexported structs use unexported fields. The `CataractaeNote` struct (`internal/cistern/client.go:102-109`) is exported because it crosses package boundaries. New structs should follow the same export rule: export if and only if the struct is consumed outside its defining package.

### Error Handling

`RecordEvent` wraps errors with `fmt.Errorf("cistern: ...", ...)` — see `internal/cistern/client.go:642`. Scheduler code uses `s.logger.Warn` / `s.logger.Error` for logging — see `internal/castellarius/scheduler.go:586-587` for the `addNote` error pattern. The new `addEvent` helper must follow the same pattern: log errors via `s.logger.Warn` if `RecordEvent` fails, never silently swallow.

The `addNote` helper currently swallows errors at the call site by logging only within the helper (`scheduler.go:584-588`). The new `addEvent` helper must follow the same approach: if `RecordEvent` returns an error, log at Warn level but do not propagate the error (these are diagnostic events, not control-flow-critical).

### Collection Types

The codebase uses `[]cistern.CataractaeNote` (slices, not maps) for note collections — see `GetNotes` return type at `internal/cistern/client.go:800`. The `validEventTypes` map uses `map[string]bool` for constant-time lookup (`internal/cistern/client.go:34`). New lookup patterns must use the same types.

### Migrations

No migration is needed for this change. The events table already exists with the `event_type TEXT` column (`internal/cistern/schema.sql:30`) and the `validEventTypes` map is the enforcement mechanism, not a CHECK constraint. The new event types are simply new string values added to the Go constants and map — no DDL or DML changes required.

If any future change did require a migration, it would follow: numbered files (`018_xxx.sql`), all identifiers double-quoted, DDL and DML in separate files, DML wrapped in transactions, embedded via `//go:embed migrations/*.sql` — see `internal/cistern/migrate.go:77-158`.

### Idiom Fit

Use `encoding/json` for payload marshaling — already imported in `client.go`. Use `fmt.Sprintf` for constructing messages — already used throughout `scheduler.go`. Use `time.Now().UTC()` for timestamps — established pattern in `client.go:639`.

The scheduler already has access to `client CisternClient` which includes `RecordEvent` in its interface (`scheduler.go:67`). No new imports or dependencies are needed.

### Testing

Existing test conventions: table-driven tests in `internal/cistern/client_test.go`, mock-based tests in `internal/castellarius/scheduler_test.go` using `mockClient`. The mock currently stubs `RecordEvent` as `return nil` (`scheduler_test.go:291-293`). Tests use `t.Errorf` and `t.Fatalf` for assertions.

The `TestValidEventTypes_ContainsAllConstants` test (`client_test.go:3371-3385`) **must** be updated to include the new event type constants — it verifies that every `Event*` constant has a matching `validEventTypes` entry and that the map has no extras. This is a required test update, not optional.

## Reusability Requirements

The seven new event types are **specific to the scheduler/heartbeat domain**. They have no meaning outside `internal/castellarius`. The `RecordEvent` API is already generic (accepts any string matching `validEventTypes`). No new generic utilities are needed.

The new `CountEventsByType` method on `cistern.Client` is generic — it queries events by type within a time window. It could be reused by any future code that needs to count events (e.g., a dashboard metric). The method signature must accept parameters, not hardcode the `exit_no_outcome` type.

## Coupling Requirements

The `circuitBreaker` method (`scheduler.go:1440-1498`) currently calls `client.GetNotes()` to count exit-no-outcome markers by scanning `notes` for `n.CataractaeName == "scheduler" && strings.Contains(n.Content, "Session exited without outcome")`. After this change, exit-no-outcome is an event, not a note, so the circuit breaker must count events instead.

**Solution**: Add a `CountEventsByType(id, eventType string, since time.Time) (int, error)` method to `cistern.Client` that counts events matching a type created after a cutoff. This method is generic (event type is a parameter) and avoids hardcoding `exit_no_outcome` in the method name.

The `CisternClient` interface in `scheduler.go:33-68` must be extended with `CountEventsByType`. The `mockClient` in `scheduler_test.go` must implement the new interface method.

The `addEvent` helper on `Castellarius` follows the same pattern as `addNote` — it is a method on the struct, not a standalone function, because it needs access to `s.logger`. No shared mutable state is introduced.

## DRY Requirements

### addEvent helper

Seven call sites currently use `s.addNote(client, item.ID, "scheduler", msg)`. Six of these will switch to `s.recordEvent(client, item.ID, eventType, payload)`. The `addNote` helper pattern (`scheduler.go:581-588`) must be mirrored as an `addEvent` helper. Both are one-liner wrappers that log on error, so extracting further is unnecessary — neither has 5+ lines repeated.

### JSON payload construction

Each of the seven event sites constructs a unique JSON payload. There is no repeated 5+ line block across these sites. Each payload is:

- `exit_no_outcome`: `{"session":"...","worker":"...","cataractae":"..."}`
- `stall`: `{"cataractae":"...","elapsed":"...","heartbeat":"..."}`
- `recovery`: `{"cataractae":"..."}`
- `circuit_breaker`: `{"death_count":N,"window":"..."}`
- `loop_recovery`: `{"from":"...","to":"...","issue":"..."}`
- `auto_promote`: `{"cataractae":"...","routed_to":"..."}`
- `no_route`: `{"cataractae":"..."}`

Each is a unique key-value set. No DRY extraction warranted.

### remapPayload functions in droplet_log.go

The `remapEvent` function (`droplet_log.go:146-171`) already has a switch with one `remapPayload*` function per event type. Each `remapPayload*` function unmarshals the JSON and formats a human-readable string. The seven new cases will need seven new `remapPayload*` functions. No repeated 5+ line block exists across these functions — each has unique field extraction logic.

## Migration Requirements

No database migration is needed. The events table schema (`internal/cistern/schema.sql:27-33`) already stores arbitrary `event_type TEXT` and `payload TEXT`. The `validEventTypes` map in Go code is the enforcement point, not a SQL constraint.

The `TestValidEventTypes_ContainsAllConstants` test must be updated to include the new constants — this acts as the schema validation for event types.

## Test Requirements

### client_test.go — new method

Test file: `internal/cistern/client_test.go`

- `TestCountEventsByType_CountsMatchingEvents`: insert 3 `exit_no_outcome` events and 1 `stall` event; assert `CountEventsByType(id, "exit_no_outcome", cutoff)` returns 3.
- `TestCountEventsByType_RespectsCutoff`: insert 2 `exit_no_outcome` events, one before cutoff and one after; assert only the recent one is counted.
- `TestCountEventsByType_ZeroWhenNone`: assert returns 0 when no events of the given type exist.
- `TestCountEventsByType_ZeroWhenWrongType`: insert `stall` events; assert `CountEventsByType(id, "exit_no_outcome", cutoff)` returns 0.

### client_test.go — validEventTypes

- Update `TestValidEventTypes_ContainsAllConstants` (`client_test.go:3371-3385`) to include all new `Event*` constants in the `expected` slice.

### scheduler_test.go — mock update

- The `mockClient.RecordEvent` stub (`scheduler_test.go:291-293`) currently returns `nil`. It must be updated to record calls so tests can assert on `RecordEvent` invocations — mirror the `mockClient.AddNote` pattern (`scheduler_test.go:155-168`) using an `attachedEvent` struct or `[]recordedEvent` slice.
- The `mockClient` must also implement `CountEventsByType(id, eventType string, since time.Time) (int, error)` — return a pre-configured count from a map field.

### scheduler_test.go — heartbeat tests

Existing test assertions check `client.attached` for notes with prefixes like `[scheduler:exit-no-outcome]`, `[scheduler:stall]`, `[scheduler:recovery]`. These must be updated to assert on `RecordEvent` calls with the correct event type and JSON payload instead.

Specific tests requiring update:

| Test | File:Line | Current Assertion | New Assertion |
|------|-----------|-------------------|---------------|
| `TestHeartbeatRepo_ExitNoOutcome_WritesNote` | `scheduler_test.go:~2547-2552` | `client.attached[0].notes` contains `[scheduler:exit-no-outcome]` | `client.events[0].eventType == "exit_no_outcome"` + payload has `session`, `worker`, `cataractae` keys |
| `TestHeartbeatRepo_StallAndOrphan` | `scheduler_test.go:~2124-2132` | `client.attached[0].notes` starts with `stallNotePrefix` | `client.events[0].eventType == "stall"` + payload has `cataractae`, `elapsed`, `heartbeat` |
| `TestHeartbeatRepo_StallAndOrphan` | `scheduler_test.go:~2130-2132` | `client.attached[1].notes` contains `[scheduler:recovery]` | `client.events[1].eventType == "recovery"` + payload has `cataractae` |
| `TestHeartbeatRepo_OrphanRecovery_SecondTick` | `scheduler_test.go:~2331` | `client.attached[1].notes` contains `[scheduler:recovery]` | Check events instead |
| `TestTick_ImplementRecirculate_ReviewerIssueFirstCycle_WritesPendingNote` | `scheduler_test.go:~916-924` | `client.attached` contains `loop-recovery-pending` note | Assert `RecordEvent` called with `"loop_recovery"` event type + keep the `AddNote` assertion for the pending marker |
| `TestTick_ImplementRecirculate_ReviewerIssueSecondCycle_RoutesToReviewer` | `scheduler_test.go:~1006-1014` | `client.attached` contains `[scheduler:loop-recovery]` note | Assert `RecordEvent` called with `"loop_recovery"` event type |
| Circuit breaker tests | `scheduler_test.go` (search for `circuit`) | Checks `client.attached` for `[circuit-breaker]` note | Assert `RecordEvent` called with `"circuit_breaker"` event type + `Pool` call |

### scheduler_test.go — auto_promote and no_route tests

Tests for the routing logic (`TestTick_Recirculate*`) that currently assert on `client.attached` notes with `[scheduler:routing]` must assert `RecordEvent` calls with `"auto_promote"` or `"no_route"` event types.

### droplet_log.go — remapEvent tests

Test file: `cmd/ct/droplet_log_test.go` (if it exists) or new tests in `cmd/ct/`

- `TestRemapEvent_ExitNoOutcome`: verify `remapEvent("exit_no_outcome", payload)` returns `"exit_no_outcome"`, human-readable detail.
- `TestRemapEvent_Stall`: verify `remapEvent("stall", payload)` returns `"stall"`, human-readable detail with elapsed and heartbeat.
- `TestRemapEvent_Recovery`: verify `remapEvent("recovery", payload)` returns `"recovery"`, human-readable detail.
- `TestRemapEvent_CircuitBreaker`: verify `remapEvent("circuit_breaker", payload)` returns `"circuit_breaker"`, human-readable detail with death_count and window.
- `TestRemapEvent_LoopRecovery`: verify `remapEvent("loop_recovery", payload)` returns `"loop_recovery"`, human-readable detail with from, to, issue.
- `TestRemapEvent_AutoPromote`: verify `remapEvent("auto_promote", payload)` returns `"auto_promote"`, human-readable detail with cataractae and routed_to.
- `TestRemapEvent_NoRoute`: verify `remapEvent("no_route", payload)` returns `"no_route"`, human-readable detail with cataractae.

### coverage_gaps_test.go

- `scheduler_test.go` line ~296 references `wantNotes` — update to also track `wantEvents` where applicable.
- Any test that checks `attached` notes for scheduler-sourced content must be split: `loop-recovery-pending` markers stay as `AddNote` assertions; all other scheduler-sourced notes become `RecordEvent` assertions.

## Forbidden Patterns

- **Inline migrations in Go code** — not applicable here (no DDL/DML changes needed), but if they were, use `embed.FS` with numbered `.sql` files per `internal/cistern/migrate.go:77-158`.
- **Scanning notes for event-type data** — the circuit breaker (`scheduler.go:1460-1468`) currently scans `cataractae_notes` for `"Session exited without outcome"`. This pattern is fragile string matching on free-text notes. The replacement must use `CountEventsByType`, not replicate the note-scanning pattern on events.
- **Adding event types without `validEventTypes` entry** — every `Event*` constant must appear in the `validEventTypes` map, enforced by `TestValidEventTypes_ContainsAllConstants` (`client_test.go:3371`).
- **Adding `RecordEvent` calls with invalid JSON payloads** — `RecordEvent` validates JSON at runtime (`client.go:634`). Payloads must be constructed via `json.Marshal(map[string]any{...})`, not via `fmt.Sprintf` of JSON strings.
- **Package-level mutable state** — new `CountEventsByType` method is on `*Client`, not a package-level function. New event type constants are in the `cistern` package's const block (`client.go:21`), not package-level vars.
- **PascalCase fields on unexported structs** — not applicable (all new structs, if any, follow the `DropletChange`/`RecentEvent` exported-struct pattern).
- **Silent error swallowing** — the `addEvent` helper must log errors with `s.logger.Warn`, matching `addNote` at `scheduler.go:586`.
- **Shadowing Go builtins** — do not name any variable `min`, `max`, or `any`.

## API Surface Checklist

- [ ] **New event type constants** (`internal/cistern/client.go:21-32`): Add `EventExitNoOutcome = "exit_no_outcome"`, `EventStall = "stall"`, `EventRecovery = "recovery"`, `EventCircuitBreaker = "circuit_breaker"`, `EventLoopRecovery = "loop_recovery"`, `EventAutoPromote = "auto_promote"`, `EventNoRoute = "no_route"`. Each constant must appear in `validEventTypes` map. Contract: `RecordEvent(id, EventXxx, payload)` accepts the constant and inserts a row; `RecordEvent` rejects unknown types with `fmt.Errorf("cistern: unknown event type %q", eventType)`.
- [ ] **`CountEventsByType(id, eventType string, since time.Time) (int, error)`** on `*Client`: Returns count of events with the given `eventType` for droplet `id` created after `since`. Contract: returns 0 (never an error) when no matching events exist. Returns error only on database failure. Uses parameterized SQL `SELECT COUNT(*) FROM "events" WHERE "droplet_id" = ? AND "event_type" = ? AND "created_at" > ?`.
- [ ] **`CisternClient` interface update** (`scheduler.go:33-68`): Add `CountEventsByType(id, eventType string, since time.Time) (int, error)` method. All mock implementations must be updated.
- [ ] **`addEvent` helper** (`scheduler.go` near `addNote` at line 581): `func (s *Castellarius) addEvent(client CisternClient, dropletID, eventType, payload string)` — calls `client.RecordEvent(dropletID, eventType, payload)` and logs errors at Warn level via `s.logger.Warn`. Contract: never panics, never returns error — errors are logged and swallowed, matching `addNote` behavior. Payload must be valid JSON (constructed via `json.Marshal`), matching `RecordEvent`'s validation contract.
- [ ] **Replace 7 `addNote` call sites with `addEvent`** in `scheduler.go`:
  - Line 825: `loop_recovery` event with payload `{"from":"<step>","to":"<step>","issue":"<id>"}`
  - Line 852: `auto_promote` event with payload `{"cataractae":"<step>","routed_to":"<on_pass>"}`
  - Line 862: `no_route` event with payload `{"cataractae":"<step>"}`
  - Line ~1346: `exit_no_outcome` event with payload `{"session":"<id>","worker":"<assignee>","cataractae":"<step>"}`
  - Line ~1370: `stall` event with payload `{"cataractae":"<step>","elapsed":"<dur>","heartbeat":"<ts>"}`
  - Line ~1385: `recovery` event with payload `{"cataractae":"<step>"}`
  - Line ~1476: `circuit_breaker` event with payload `{"death_count":<n>,"window":"<dur>"}`
- [ ] **Keep line 835 as `addNote`**: The `loop-recovery-pending` marker at `scheduler.go:835` must remain as `addNote` — `loopRecoveryPendingCount` (`scheduler.go:1087-1098`) scans `cataractae_notes` for the `[scheduler:loop-recovery-pending]` prefix. This note must NOT be converted to an event.
- [ ] **Circuit breaker update** (`scheduler.go:1440-1498`): Replace the `GetNotes` + string-scanning loop (lines 1455-1468) with `CountEventsByType(item.ID, EventExitNoOutcome, cutoff)`. Contract: `CountEventsByType` returns the count directly (integer), no string matching needed. The `circuit_breaker` addNote at line 1476 becomes an `addEvent` for `EventCircuitBreaker`.
- [ ] **Remove `stallNotePrefix` constant** (`scheduler.go:28`): The `[scheduler:stall]` prefix constant is no longer needed once stall is an event type. Verify no other code references it before removing.
- [ ] **`remapEvent` updates** (`cmd/ct/droplet_log.go:146-171`): Add 7 new cases to the switch: `"exit_no_outcome"`, `"stall"`, `"recovery"`, `"circuit_breaker"`, `"loop_recovery"`, `"auto_promote"`, `"no_route"`. Each case returns `(eventType, remapPayloadXxx(detail))` where `remapPayloadXxx` unmarshals JSON and formats a human-readable string, following the existing `remapPayloadReason` pattern (`droplet_log.go:173-184`).
- [ ] **Mock `RecordEvent` must record invocations** (`scheduler_test.go:291-293`): Change from `return nil` to appending to a `[]recordedEvent` slice with `eventType` and `payload` fields, mirroring `mockClient.AddNote` recording pattern at `scheduler_test.go:155-168`.
- [ ] **Mock `CountEventsByType`** (`scheduler_test.go`): Add method returning pre-configured count from a map field `eventCounts map[string]int` keyed by `"dropletID:eventType"`.
- [ ] **All existing scheduler tests pass** with assertions updated from note-matching to event-matching. Specifically: every test that asserts `client.attached[i].notes` contains `[scheduler:stall]`, `[scheduler:recovery]`, `[scheduler:exit-no-outcome]`, `[scheduler:routing] Auto-promoted`, `[scheduler:routing] cataractae=...`, `[circuit-breaker]`, or `[scheduler:loop-recovery] detected` must be updated to assert `RecordEvent` calls instead. Tests asserting `[scheduler:loop-recovery-pending]` must continue asserting `AddNote` calls unchanged.
- [ ] **`ct droplet log` displays structured scheduler events meaningfully**: Each new event type must produce a readable log line, not raw JSON. Verified by `remapEvent` test cases per the test requirements above.