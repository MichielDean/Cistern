package main

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/MichielDean/cistern/internal/cistern"
)

func setupLogTestDB(t *testing.T) *cistern.Client {
	t.Helper()
	dir := t.TempDir()
	db := filepath.Join(dir, "test.db")
	t.Setenv("CT_DB", db)
	c, err := cistern.New(db, "ct")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	return c
}

func runLogCapture(t *testing.T, id string) (string, error) {
	t.Helper()
	var buf bytes.Buffer
	err := runLog(&buf, id)
	return buf.String(), err
}

func TestDropletLog_ShowsCreationAndNotes(t *testing.T) {
	c := setupLogTestDB(t)
	item, err := c.Add("myrepo", "Log task", "do something", 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	c.AddNote(item.ID, "implement", "wrote the code")
	c.AddNote(item.ID, "review", "looks good")

	out, err := runLogCapture(t, item.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(out, "created") {
		t.Errorf("log output missing 'created' event: %s", out)
	}
	if !strings.Contains(out, "implement") {
		t.Errorf("log output missing 'implement' note: %s", out)
	}
	if !strings.Contains(out, "review") {
		t.Errorf("log output missing 'review' note: %s", out)
	}
	if !strings.Contains(out, "wrote the code") {
		t.Errorf("log output missing note content: %s", out)
	}
}

func TestDropletLog_ShowsPoolEvent(t *testing.T) {
	c := setupLogTestDB(t)
	item, err := c.Add("myrepo", "Pool task", "", 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	c.Pool(item.ID, "needs human review")

	out, err := runLogCapture(t, item.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(out, "pooled") {
		t.Errorf("log output missing 'pooled' event: %s", out)
	}
}

func TestDropletLog_ShowsStageAssignment(t *testing.T) {
	c := setupLogTestDB(t)
	item, err := c.Add("myrepo", "Stage task", "", 1, 2)
	if err != nil {
		t.Fatal(err)
	}

	c.GetReadyForAqueduct("myrepo", "default")
	c.Assign(item.ID, "worker-1", "implement")
	c.AddNote(item.ID, "implement", "started implementation")

	out, err := runLogCapture(t, item.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(out, "implement") {
		t.Errorf("log output missing 'implement' cataractae: %s", out)
	}
}

func TestDropletLog_ShowsHeader(t *testing.T) {
	c := setupLogTestDB(t)
	item, err := c.Add("myrepo", "Header task", "desc", 1, 2)
	if err != nil {
		t.Fatal(err)
	}

	out, err := runLogCapture(t, item.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(out, item.ID) {
		t.Errorf("log output missing droplet ID: %s", out)
	}
	if !strings.Contains(out, "Header task") {
		t.Errorf("log output missing droplet title: %s", out)
	}
}

func TestDropletLog_ChronologicalOrder(t *testing.T) {
	c := setupLogTestDB(t)
	item, err := c.Add("myrepo", "Order task", "", 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	c.AddNote(item.ID, "implement", "first note")
	time.Sleep(10 * time.Millisecond)
	c.AddNote(item.ID, "review", "second note")

	out, err := runLogCapture(t, item.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	firstIdx := strings.Index(out, "first note")
	secondIdx := strings.Index(out, "second note")
	if firstIdx == -1 || secondIdx == -1 {
		t.Fatalf("log output missing expected notes: %s", out)
	}
	if firstIdx > secondIdx {
		t.Errorf("notes not in chronological order: first note at %d, second at %d", firstIdx, secondIdx)
	}
}

func TestDropletLog_NonexistentDroplet(t *testing.T) {
	_ = setupLogTestDB(t)

	_, err := runLogCapture(t, "nonexistent-id")
	if err == nil {
		t.Error("expected error for nonexistent droplet")
	}
}

func TestDropletLog_EmptyDroplet(t *testing.T) {
	c := setupLogTestDB(t)
	item, err := c.Add("myrepo", "Empty task", "", 1, 2)
	if err != nil {
		t.Fatal(err)
	}

	out, err := runLogCapture(t, item.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(out, "created") {
		t.Errorf("log output should show creation event even for empty droplet: %s", out)
	}
	if !strings.Contains(out, "Empty task") {
		t.Errorf("log output should show title: %s", out)
	}
}

func TestDropletLog_JsonFormat(t *testing.T) {
	logFmt = "json"
	t.Cleanup(func() { logFmt = "text" })

	c := setupLogTestDB(t)
	item, err := c.Add("myrepo", "Json log task", "", 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	c.AddNote(item.ID, "implement", "wrote code")

	var buf bytes.Buffer
	err = runLog(&buf, item.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) < 2 {
		t.Fatalf("expected at least 2 JSON lines (event + note), got %d", len(lines))
	}

	type logEntry struct {
		Time       string `json:"time"`
		Cataractae string `json:"cataractae"`
		Event      string `json:"event"`
		Detail     string `json:"detail"`
	}

	var first logEntry
	if err := json.Unmarshal([]byte(lines[0]), &first); err != nil {
		t.Fatalf("first line is not valid JSON: %v\nline: %s", err, lines[0])
	}
	if first.Event != "created" {
		t.Errorf("first event should be 'created', got %q", first.Event)
	}

	var note logEntry
	if err := json.Unmarshal([]byte(lines[1]), &note); err != nil {
		t.Fatalf("second line is not valid JSON: %v\nline: %s", err, lines[1])
	}
	if note.Event != "note" {
		t.Errorf("second event should be 'note', got %q", note.Event)
	}
	if note.Cataractae != "implement" {
		t.Errorf("note cataractae should be 'implement', got %q", note.Cataractae)
	}
	if note.Detail != "wrote code" {
		t.Errorf("note detail should be 'wrote code', got %q", note.Detail)
	}
}

func TestDropletLog_InvalidFormat(t *testing.T) {
	logFmt = "xml"
	t.Cleanup(func() { logFmt = "text" })

	c := setupLogTestDB(t)
	item, err := c.Add("myrepo", "Format task", "", 1, 2)
	if err != nil {
		t.Fatal(err)
	}

	_, err = runLogCapture(t, item.ID)
	if err == nil {
		t.Error("expected error for invalid format")
	}
	if !strings.Contains(err.Error(), "format must be text or json") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestDropletLog_NoSyntheticHeartbeat(t *testing.T) {
	c := setupLogTestDB(t)
	item, err := c.Add("myrepo", "No hb task", "", 1, 2)
	if err != nil {
		t.Fatal(err)
	}

	c.GetReadyForAqueduct("myrepo", "default")
	c.Assign(item.ID, "worker-1", "implement")
	c.AddNote(item.ID, "implement", "started")
	err = c.Heartbeat(item.ID)
	if err != nil {
		t.Fatalf("heartbeat failed: %v", err)
	}

	out, err := runLogCapture(t, item.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if strings.Contains(out, "last heartbeat recorded") {
		t.Errorf("log output should not contain synthetic heartbeat detail: %s", out)
	}
}

func TestBuildLogEntries_DisplaysEventsFromTimeline(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)

	timeline := []cistern.TimelineEntry{
		{Time: now, EventType: "create", Payload: `{"repo":"myrepo","title":"Test task","priority":1,"complexity":2}`},
		{Time: now.Add(time.Minute), EventType: "dispatch", Payload: `{"aqueduct":"default","cataractae":"implement","assignee":"worker-1"}`},
		{Time: now.Add(2 * time.Minute), EventType: "pass", Payload: `{"cataractae":"implement","notes":"all good"}`},
	}

	notes := []cistern.CataractaeNote{}

	entries := buildLogEntries(timeline, notes)

	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}
	if entries[0].Event != "created" {
		t.Errorf("first event should be 'created', got %q", entries[0].Event)
	}
	if !strings.Contains(entries[0].Detail, "repo: myrepo") {
		t.Errorf("created entry missing 'repo: myrepo': %s", entries[0].Detail)
	}
	if entries[1].Event != "dispatched" {
		t.Errorf("second event should be 'dispatched', got %q", entries[1].Event)
	}
	if entries[2].Event != "pass" {
		t.Errorf("third event should be 'pass', got %q", entries[2].Event)
	}
}

func TestBuildLogEntries_NotesAfterEvents(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)

	timeline := []cistern.TimelineEntry{
		{Time: now, EventType: "create", Payload: `{}`},
	}

	notes := []cistern.CataractaeNote{
		{ID: 1, DropletID: "test", CataractaeName: "implement", Content: "got started", CreatedAt: now.Add(time.Minute)},
	}

	entries := buildLogEntries(timeline, notes)

	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].Event != "created" {
		t.Errorf("first entry should be lifecycle event 'created', got %q", entries[0].Event)
	}
	if entries[1].Event != "note" {
		t.Errorf("second entry should be 'note', got %q", entries[1].Event)
	}
	if entries[1].Cataractae != "implement" {
		t.Errorf("note cataractae should be 'implement', got %q", entries[1].Cataractae)
	}
	if entries[1].Detail != "got started" {
		t.Errorf("note detail should be 'got started', got %q", entries[1].Detail)
	}
}

func TestBuildLogEntries_SortedByTime(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)

	timeline := []cistern.TimelineEntry{
		{Time: now.Add(2 * time.Minute), EventType: "pass", Payload: `{}`},
		{Time: now, EventType: "create", Payload: `{}`},
	}

	notes := []cistern.CataractaeNote{
		{ID: 1, DropletID: "test", CataractaeName: "implement", Content: "early note", CreatedAt: now.Add(time.Minute)},
	}

	entries := buildLogEntries(timeline, notes)

	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}
	if entries[0].Event != "created" {
		t.Errorf("entry 0 should be 'created', got %q", entries[0].Event)
	}
	if entries[1].Event != "note" {
		t.Errorf("entry 1 should be 'note', got %q", entries[1].Event)
	}
	if entries[2].Event != "pass" {
		t.Errorf("entry 2 should be 'pass', got %q", entries[2].Event)
	}
}

func TestBuildLogEntries_NoDuplicateCreatedEvent(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)

	timeline := []cistern.TimelineEntry{
		{Time: now, EventType: "create", Payload: `{"repo":"myrepo","title":"New task","priority":1,"complexity":2}`},
		{Time: now.Add(time.Minute), EventType: "dispatch", Payload: `{}`},
	}

	notes := []cistern.CataractaeNote{}

	entries := buildLogEntries(timeline, notes)

	createCount := 0
	for _, e := range entries {
		if e.Event == "created" {
			createCount++
		}
	}
	if createCount != 1 {
		t.Errorf("expected exactly 1 created entry when event exists in timeline, got %d", createCount)
	}
}

func TestDisplayInfo_DisplaysHumanReadableDetails(t *testing.T) {
	tests := []struct {
		name     string
		evt      string
		detail   string
		wantEvt  string
		wantSub  string
		wantOmit string
	}{
		{
			name:     "create event shows repo, title, priority, complexity",
			evt:      "create",
			detail:   `{"repo":"myrepo","title":"My task","priority":1,"complexity":2}`,
			wantEvt:  "created",
			wantSub:  "repo: myrepo, title: My task, priority: 1, complexity: 2",
			wantOmit: `"repo"`,
		},
		{
			name:     "dispatch event shows aqueduct, step and assignee",
			evt:      "dispatch",
			detail:   `{"aqueduct":"default","cataractae":"implement","assignee":"alice"}`,
			wantEvt:  "dispatched",
			wantSub:  "aqueduct: default, step: implement, assignee: alice",
			wantOmit: `"cataractae"`,
		},
		{
			name:     "pass event shows cataractae and notes",
			evt:      "pass",
			detail:   `{"cataractae":"reviewer","notes":"all good"}`,
			wantEvt:  "pass",
			wantSub:  "by: reviewer, notes: all good",
			wantOmit: `"cataractae"`,
		},
		{
			name:     "recirculate event shows target and notes",
			evt:      "recirculate",
			detail:   `{"cataractae":"reviewer","target":"implement","notes":"needs fixes"}`,
			wantEvt:  "recirculate",
			wantSub:  "by: reviewer, to: implement, notes: needs fixes",
			wantOmit: `"cataractae"`,
		},
		{
			name:     "restart event shows cataractae",
			evt:      "restart",
			detail:   `{"cataractae":"implement"}`,
			wantEvt:  "restart",
			wantSub:  "by: implement",
			wantOmit: `"cataractae"`,
		},
		{
			name:     "approve event shows cataractae",
			evt:      "approve",
			detail:   `{"cataractae":"manual"}`,
			wantEvt:  "approved",
			wantSub:  "by: manual",
			wantOmit: `"cataractae"`,
		},
		{
			name:     "edit event shows fields",
			evt:      "edit",
			detail:   `{"fields":["title","priority"]}`,
			wantEvt:  "edit",
			wantSub:  "fields: [title priority]",
			wantOmit: `"fields"`,
		},
		{
			name:     "pool event shows reason",
			evt:      "pool",
			detail:   `{"reason":"needs human review"}`,
			wantEvt:  "pooled",
			wantSub:  "reason: needs human review",
			wantOmit: `"reason"`,
		},
		{
			name:     "cancel event shows reason",
			evt:      "cancel",
			detail:   `{"reason":"not needed"}`,
			wantEvt:  "cancelled",
			wantSub:  "reason: not needed",
			wantOmit: `"reason"`,
		},
		{
			name:    "delivered event has no detail",
			evt:     "delivered",
			detail:  `{}`,
			wantEvt: "delivered",
			wantSub: "",
		},
		{
			name:     "dispatch event without aqueduct shows step and assignee",
			evt:      "dispatch",
			detail:   `{"cataractae":"implement","assignee":"alice"}`,
			wantEvt:  "dispatched",
			wantSub:  "step: implement, assignee: alice",
			wantOmit: `"cataractae"`,
		},
		{
			name:    "empty dispatch payload shows no detail",
			evt:     "dispatch",
			detail:  `{}`,
			wantEvt: "dispatched",
			wantSub: "",
		},
		{
			name:     "exit_no_outcome event shows session, worker, step",
			evt:      "exit_no_outcome",
			detail:   `{"session":"sess-1","worker":"alpha","cataractae":"implement"}`,
			wantEvt:  "exit_no_outcome",
			wantSub:  "session: sess-1, worker: alpha, step: implement",
			wantOmit: `"session"`,
		},
		{
			name:     "stall event shows step, elapsed",
			evt:      "stall",
			detail:   `{"cataractae":"implement","elapsed":"45m","heartbeat":"2026-04-21T10:00:00Z"}`,
			wantEvt:  "stall",
			wantSub:  "step: implement, elapsed: 45m",
			wantOmit: `"cataractae"`,
		},
		{
			name:     "recovery event shows step",
			evt:      "recovery",
			detail:   `{"cataractae":"implement"}`,
			wantEvt:  "recovery",
			wantSub:  "by: implement",
			wantOmit: `"cataractae"`,
		},
		{
			name:     "circuit_breaker event shows death count and window",
			evt:      "circuit_breaker",
			detail:   `{"death_count":5,"window":"15m0s"}`,
			wantEvt:  "circuit_breaker",
			wantSub:  "dead sessions: 5, window: 15m0s",
			wantOmit: `"death_count"`,
		},
		{
			name:     "loop_recovery event shows from, to, issue",
			evt:      "loop_recovery",
			detail:   `{"from":"implement","to":"implement","issue":"iss-001"}`,
			wantEvt:  "loop_recovery",
			wantSub:  "from: implement, to: implement, issue: iss-001",
			wantOmit: `"from"`,
		},
		{
			name:     "auto_promote event shows step and routed_to",
			evt:      "auto_promote",
			detail:   `{"cataractae":"implement","routed_to":"review"}`,
			wantEvt:  "auto_promote",
			wantSub:  "step: implement, routed to: review",
			wantOmit: `"cataractae"`,
		},
		{
			name:     "no_route event shows step",
			evt:      "no_route",
			detail:   `{"cataractae":"implement"}`,
			wantEvt:  "no_route",
			wantSub:  "by: implement",
			wantOmit: `"cataractae"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotEvt, gotDetail := cistern.DisplayInfo(tt.evt, tt.detail)
			if gotEvt != tt.wantEvt {
				t.Errorf("DisplayInfo(%q, ...) evt = %q, want %q", tt.evt, gotEvt, tt.wantEvt)
			}
			if tt.wantSub != "" && !strings.Contains(gotDetail, tt.wantSub) {
				t.Errorf("DisplayInfo(%q, ...) detail = %q, want substring %q", tt.evt, gotDetail, tt.wantSub)
			}
			if tt.wantOmit != "" && strings.Contains(gotDetail, tt.wantOmit) {
				t.Errorf("DisplayInfo(%q, ...) detail = %q, should not contain raw JSON key %q", tt.evt, gotDetail, tt.wantOmit)
			}
			if tt.wantSub == "" && gotDetail != "" {
				t.Errorf("DisplayInfo(%q, ...) detail = %q, want empty", tt.evt, gotDetail)
			}
		})
	}
}
