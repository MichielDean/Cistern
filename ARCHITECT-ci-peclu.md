# Architecture: Rebuild ct droplet log from structured events

**Droplet:** ci-peclu
**Scope:** `internal/cistern/event_types.go`, `internal/cistern/client.go`, `cmd/ct/droplet_log.go`, `cmd/ct/droplet_history.go`

## Problem

`ct droplet log` derives its timeline from a UNION query mixing events and
notes. The CLI then parses these back into typed entries via string-matching
(`strings.Cut(ch.Value, ": ")`) and a big `remapEvent` switch. This creates
several problems:

1. **String format coupling** — `GetDropletChanges` concatenates `"note" AS kind,
   cataractae_name || ': ' || content AS value` and `"event" AS kind,
   event_type || ': ' || payload AS value` in SQL. The CLI must split them back.
2. **Synthetic entries** — `buildLogEntries` fabricates "created" and "heartbeat"
   entries from the `Droplet` struct fields because the events table didn't
   always have a create event. Now it does (since ci-meaol/ci-pia3m).
3. **Notes mixed into timeline** — cataractae_notes appear as `kind=note` in the
   event stream, making scheduler activity indistinguishable from lifecycle
   events in the JSON output.

## Design

### 1. New file: `internal/cistern/event_types.go`

Extract event-related constants, types, and display logic from `client.go`.

```go
package cistern

// Event type constants (moved from client.go lines 21-39)
const (
    EventCreate         = "create"
    EventDispatch       = "dispatch"
    // ... all 16 constants unchanged
)

// ValidEventTypes (moved from client.go lines 41-59)
var ValidEventTypes = map[string]bool{ ... }

// DisplayInfo maps an event_type and its JSON payload to human-readable
// eventLabel and detail strings. This replaces the remapEvent + remapPayload*
// functions in cmd/ct/droplet_log.go.
func DisplayInfo(eventType, payload string) (eventLabel, detail string)
```

**DisplayInfo** — port the full `remapEvent` switch and all `remapPayload*`
functions into this single method. Logic is identical; just moves from
`cmd/ct` to `internal/cistern` so it's testable from the package tests and
reusable by the dashboard.

**recordEvent** — leave in `client.go` (it needs the `executor` interface), but
have it call `ValidEventTypes` from the new file. Move the `validEventTypes`
map to the exported `ValidEventTypes` (capital V). Update `recordEvent` to
reference `ValidEventTypes` instead.

### 2. New type + query: `TimelineEntry` and `GetDropletTimeline`

Add to `internal/cistern/client.go`:

```go
// TimelineEntry is a single row from the events table, returned by
// GetDropletTimeline.
type TimelineEntry struct {
    Time      time.Time `json:"time"`
    EventType string    `json:"event_type"`
    Payload   string    `json:"payload"`
}

// GetDropletTimeline returns the event timeline for a droplet, ordered
// oldest-first. This replaces the UNION query in GetDropletChanges for
// log display.
func (c *Client) GetDropletTimeline(id string) ([]TimelineEntry, error) {
    rows, err := c.db.Query(`
        SELECT created_at, event_type, payload
        FROM events
        WHERE droplet_id = ?
        ORDER BY created_at ASC`, id)
    // ... scan into []TimelineEntry
}
```

### 3. Rewrite `buildLogEntries` in `cmd/ct/droplet_log.go`

**Before:** Calls `GetDropletChanges` (UNION query), parses `Kind=note/event`
and `Value="prefix: suffix"` strings, synthesizes create/heartbeat entries.

**After:**

```go
func runLog(out io.Writer, id string) error {
    // ...
    timeline, err := c.GetDropletTimeline(id)  // events only
    // ...
    notes, err := c.GetNotes(id)                // notes separately
    // ...
    entries := buildLogEntries(timeline, notes)
    // ...
}

func buildLogEntries(timeline []cistern.TimelineEntry, notes []cistern.CataractaeNote) []logEntry {
    var entries []logEntry

    // Lifecycle events from events table
    for _, te := range timeline {
        eventLabel, detail := cistern.DisplayInfo(te.EventType, te.Payload)
        entries = append(entries, logEntry{
            sortTime: te.Time,
            Time:     te.Time.Format("2006-01-02 15:04:05"),
            Event:    eventLabel,
            Detail:   detail,
        })
    }

    // Notes appended at end, clearly separated
    for _, n := range notes {
        entries = append(entries, logEntry{
            sortTime:   n.CreatedAt,
            Time:       n.CreatedAt.Format("2006-01-02 15:04:05"),
            Cataractae: n.CataractaeName,
            Event:      "note",
            Detail:     n.Content,
        })
    }

    sort.SliceStable(entries, func(i, j int) bool {
        return entries[i].sortTime.Before(entries[j].sortTime)
    })

    return entries
}
```

**Key changes:**
- No more `DropletChange` parsing with `strings.Cut`
- No more synthetic "created" entry (create events exist in events table since
  ci-meaol)
- No more synthetic "heartbeat" entry (removed per spec)
- Notes come from `GetNotes()` and are type `note` with `Cataractae` populated
- `DisplayInfo()` is in `internal/cistern`, not duplicated in `cmd/ct`

**Drop the `remapEvent` and all `remapPayload*` functions** from
`cmd/ct/droplet_log.go`. They're replaced by `cistern.DisplayInfo()`.

### 4. Update `GetDropletChanges` — `internal/cistern/client.go`

The `DropletChange` type is used by `ct droplet tail` and the dashboard API.
The spec says `Kind` becomes `"event"` always and `Value` becomes
`event_type + ": " + payload`. This preserves the format for `ct droplet tail`
consumers.

```go
// GetDropletChanges returns recent events for a droplet, ordered
// oldest-first. Each entry has Kind="event" and Value="event_type: payload".
// Used by ct droplet tail and the dashboard API.
func (c *Client) GetDropletChanges(id string, limit int) ([]DropletChange, error) {
    rows, err := c.db.Query(`
        SELECT created_at, event_type, COALESCE(payload, '')
        FROM events
        WHERE droplet_id = ?
        ORDER BY created_at ASC
        LIMIT ?`, id, limit)
    if err != nil {
        return nil, fmt.Errorf("cistern: get droplet changes %s: %w", id, err)
    }
    defer rows.Close()

    var changes []DropletChange
    for rows.Next() {
        var t time.Time
        var eventType, payload string
        if err := rows.Scan(&t, &eventType, &payload); err != nil {
            return nil, fmt.Errorf("cistern: scan droplet change: %w", err)
        }
        value := eventType
        if payload != "" && payload != "{}" {
            value = eventType + ": " + payload
        }
        changes = append(changes, DropletChange{
            Time:  t,
            Kind:  "event",
            Value: value,
        })
    }
    return changes, rows.Err()
}
```

**Compatibility note:** `ct droplet tail` uses `DropletChange.Kind` to print
`[%s]` and `DropletChange.Value` as the message. With this change, `Kind` is
always `"event"` and `Value` is `"dispatch: {...}"` etc. The tail output format
changes from `[note] implement: wrote code` to `[event] dispatch: {...}`. This
is acceptable because notes are no longer mixed into the event stream. The
dashboard API endpoint `/api/droplets/{id}/changes` also returns these and
will similarly see only events.

### 5. Update `ListRecentEvents` — `internal/cistern/client.go`

```go
func (c *Client) ListRecentEvents(limit int) ([]RecentEvent, error) {
    rows, err := c.db.Query(`
        SELECT droplet_id, event_type, created_at
        FROM events
        ORDER BY created_at DESC
        LIMIT ?`, limit)
    // ... unchanged scan logic
}
```

Just remove the `UNION ALL SELECT ... FROM cataractae_notes` half. The
`RecentEvent.Event` field already shows `event_type` directly, so no schema
change needed.

### 6. Update `DropletChange.Kind` comment

```go
type DropletChange struct {
    Time  time.Time `json:"time"`
    Kind  string    `json:"kind"`  // always "event"
    Value string    `json:"value"` // "event_type: payload" or "event_type"
}
```

### 7. Remove synthetic create/heartbeat from `buildLogEntries`

The current code at lines 105-137 of `droplet_log.go` adds:
- A synthetic "created" entry if no `create` event exists in the changes
- A synthetic "heartbeat" entry from `item.LastHeartbeatAt`

Both are dropped. Create events now always come from the events table (since
ci-meaol added `RecordEvent` on `Add()`). Heartbeat display is removed per
the spec ("Drop synthetic heartbeat entry").

This also means `runLog()` no longer needs to call `c.Get(id)` for the log.
However, `printLogText` uses the `Droplet` for the header line. We still need
`Get()` for the header, but `buildLogEntries` no longer takes a `*Droplet`.

### 8. Signature changes

| Function | Old signature | New signature |
|---|---|---|
| `buildLogEntries` | `(item *Droplet, changes []DropletChange) []logEntry` | `(timeline []TimelineEntry, notes []CataractaeNote) []logEntry` |
| `runLog` | calls `GetDropletChanges + buildLogEntries` | calls `GetDropletTimeline + GetNotes + buildLogEntries` |
| `runHistory` | same as runLog | same as runLog |

## Files to create

- **`internal/cistern/event_types.go`** — Event constants, `ValidEventTypes`,
  `DisplayInfo()` function (ported from `remapEvent` + all `remapPayload*`)

## Files to modify

- **`internal/cistern/client.go`**
  - Move event constants (lines 21-39) and `validEventTypes` map (lines 41-59)
    to `event_types.go`
  - Export `validEventTypes` → `ValidEventTypes`
  - Move `RecordEvent`/`recordEvent` to `event_types.go` (they reference
    `executor`, so keep a thin wrapper in client.go or pass `executor` as param)
  - Add `TimelineEntry` struct
  - Add `GetDropletTimeline(id string) ([]TimelineEntry, error)` method
  - Rewrite `GetDropletChanges` to events-only query (drop UNION)
  - Rewrite `ListRecentEvents` to events-only query (drop UNION)
  - Update `DropletChange.Kind` comment

- **`cmd/ct/droplet_log.go`**
  - Rewrite `buildLogEntries` to take `[]TimelineEntry, []CataractaeNote`
  - Remove `remapEvent`, all `remapPayload*` functions (moved to DisplayInfo)
  - Remove synthetic create/heartbeat logic
  - Update `runLog` to call `GetDropletTimeline` + `GetNotes`

- **`cmd/ct/droplet_history.go`**
  - Update `runHistory` to call `GetDropletTimeline` + `GetNotes` + `buildLogEntries`

## Files that reference `GetDropletChanges` / `DropletChange` (downstream impacts)

These callers need to be checked for breaking changes but most should work
since `Kind="event"` and `Value="event_type: payload"` preserves the format:

1. **`cmd/ct/droplet_tail.go`** — Uses `DropletChange.Kind` and `.Value` in
   `printChange`. Works as-is. The `%s [%s] %s\n` format will show
   `[event] dispatch: {...}` instead of `[note] implement: wrote code`.

2. **`cmd/ct/dashboard_web.go`** — API endpoint returns `[]DropletChange` as
   JSON. Consumers will see `kind` always `"event"` instead of mixed
   `"note"`/`"event"`. This is intentional.

3. **`cmd/ct/inspect.go`** — Uses `ListRecentEvents`, which now returns only
   events. No code change needed (it reads `Event` field directly).

## Test changes

- **`cmd/ct/droplet_log_test.go`** — All tests that use
  `buildLogEntries(item, changes)` must be updated to the new signature
  `buildLogEntries(timeline, notes)`. Tests that rely on synthetic create/heartbeat
  entries need adjustment (they should now only see entries from the events
  table + notes).

- **`internal/cistern/client_test.go`** — Add tests for `GetDropletTimeline`.
  Update `TestGetDropletChanges*` tests to expect `Kind` always `"event"` and
  no note entries. Add tests for `DisplayInfo`.

- **`cmd/ct/droplet_tail_test.go`** — Update if tests rely on `kind=note`
  entries. Tail still works — just never emits `kind=note`.

- **`cmd/ct/dashboard_api_test.go`** — Update tests checking for `kind=note`
  in JSON responses. All entries will be `kind=event`.

## Acceptance criteria mapping

| Criterion | How it's met |
|---|---|
| ct droplet log shows clean typed-event timeline, no string-matching hacks | `DisplayInfo()` in internal/cistern replaces `remapEvent` string parsing; `GetDropletTimeline` returns structured `TimelineEntry` |
| ct droplet tail --follow still works | `GetDropletChanges` returns events-only, `Kind="event"`, format preserved |
| ct droplet log --format json outputs structured entries with event_type and payload | `TimelineEntry` has `EventType` and `Payload` fields; notes have `Cataractae`/`Detail` |
| Agent notes accessible via ct droplet log but clearly separated | `GetNotes()` called separately, note entries appended after events with `Event=note` |
| No kind=note entries in timeline for scheduler activity | `GetDropletTimeline` queries only events table; scheduler activity is in events |
| All existing tests pass | Test suite updated per above |

## Implementation order

1. Create `internal/cistern/event_types.go` with constants, `ValidEventTypes`,
   and `DisplayInfo()`
2. Add `TimelineEntry` and `GetDropletTimeline` to `client.go`
3. Rewrite `GetDropletChanges` and `ListRecentEvents` in `client.go`
4. Rewrite `buildLogEntries` and `runLog` in `cmd/ct/droplet_log.go`
5. Update `runHistory` in `cmd/ct/droplet_history.go`
6. Update all tests
7. Run full test suite