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
		t.Fatalf("expected at least 2 JSON lines (header + note), got %d", len(lines))
	}

	type logEntry struct {
		Time       string `json:"time"`
		Cataractae string `json:"cataractae"`
		Event      string `json:"event"`
		Detail     string `json:"detail"`
	}

	var header logEntry
	if err := json.Unmarshal([]byte(lines[0]), &header); err != nil {
		t.Fatalf("first line is not valid JSON: %v\nline: %s", err, lines[0])
	}
	if header.Event != "created" {
		t.Errorf("first event should be 'created', got %q", header.Event)
	}

	var note logEntry
	if err := json.Unmarshal([]byte(lines[1]), &note); err != nil {
		t.Fatalf("second line is not valid JSON: %v\nline: %s", err, lines[1])
	}
	if note.Event != "note" {
		t.Errorf("second event should be 'note', got %q", note.Event)
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

func TestDropletLog_ShowsHeartbeat(t *testing.T) {
	c := setupLogTestDB(t)
	item, err := c.Add("myrepo", "Heartbeat task", "", 1, 2)
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

	refreshed, err := c.Get(item.ID)
	if err != nil {
		t.Fatalf("get failed: %v", err)
	}
	if refreshed.LastHeartbeatAt.IsZero() {
		t.Fatal("LastHeartbeatAt should be set after Heartbeat()")
	}

	out, err := runLogCapture(t, item.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(out, "heartbeat") {
		t.Errorf("log output missing 'heartbeat' event: %s", out)
	}
	if !strings.Contains(out, "last heartbeat recorded") {
		t.Errorf("log output missing heartbeat detail: %s", out)
	}
}

func TestDropletLog_HeartbeatInChronologicalOrder(t *testing.T) {
	c := setupLogTestDB(t)
	item, err := c.Add("myrepo", "Heartbeat order task", "", 1, 2)
	if err != nil {
		t.Fatal(err)
	}

	c.GetReadyForAqueduct("myrepo", "default")
	c.Assign(item.ID, "worker-1", "implement")
	c.AddNote(item.ID, "implement", "early note")
	err = c.Heartbeat(item.ID)
	if err != nil {
		t.Fatalf("heartbeat failed: %v", err)
	}

	time.Sleep(10 * time.Millisecond)
	c.AddNote(item.ID, "implement", "late note")

	out, err := runLogCapture(t, item.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	heartbeatIdx := strings.Index(out, "heartbeat")
	lateNoteIdx := strings.Index(out, "late note")
	if heartbeatIdx == -1 || lateNoteIdx == -1 {
		t.Fatalf("log output missing expected entries: %s", out)
	}
	if heartbeatIdx > lateNoteIdx {
		t.Errorf("heartbeat should appear before late note in chronological order: heartbeat at %d, late note at %d", heartbeatIdx, lateNoteIdx)
	}
}

func TestRemapEvent_DisplaysHumanReadableDetails(t *testing.T) {
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
			name:     "stall event shows step, elapsed, heartbeat",
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
			gotEvt, gotDetail := remapEvent(tt.evt, tt.detail)
			if gotEvt != tt.wantEvt {
				t.Errorf("remapEvent(%q, ...) evt = %q, want %q", tt.evt, gotEvt, tt.wantEvt)
			}
			if tt.wantSub != "" && !strings.Contains(gotDetail, tt.wantSub) {
				t.Errorf("remapEvent(%q, ...) detail = %q, want substring %q", tt.evt, gotDetail, tt.wantSub)
			}
			if tt.wantOmit != "" && strings.Contains(gotDetail, tt.wantOmit) {
				t.Errorf("remapEvent(%q, ...) detail = %q, should not contain raw JSON key %q", tt.evt, gotDetail, tt.wantOmit)
			}
			if tt.wantSub == "" && gotDetail != "" {
				t.Errorf("remapEvent(%q, ...) detail = %q, want empty", tt.evt, gotDetail)
			}
		})
	}
}

func TestBuildLogEntries_SynthesizesCreateEvent_WhenMissing(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	item := &cistern.Droplet{
		ID:         "pre-exist",
		Repo:       "myrepo",
		Title:      "Pre-existing task",
		Priority:   1,
		Complexity: 2,
		Status:     "open",
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	changes := []cistern.DropletChange{
		{Time: now.Add(time.Minute), Kind: "note", Value: "implement: got started"},
	}

	entries := buildLogEntries(item, changes)

	createFound := false
	for _, e := range entries {
		if e.Event == "created" {
			createFound = true
			if !strings.Contains(e.Detail, "repo: myrepo") {
				t.Errorf("synthesized created entry missing repo: %s", e.Detail)
			}
			if !strings.Contains(e.Detail, "title: Pre-existing task") {
				t.Errorf("synthesized created entry missing title: %s", e.Detail)
			}
			if e.Time != now.Format("2006-01-02 15:04:05") {
				t.Errorf("synthesized created entry time = %q, want %q", e.Time, now.Format("2006-01-02 15:04:05"))
			}
			break
		}
	}
	if !createFound {
		t.Errorf("buildLogEntries should synthesize created event for pre-existing droplet; entries: %+v", entries)
	}

	noteFound := false
	for _, e := range entries {
		if e.Event == "note" && strings.Contains(e.Detail, "got started") {
			noteFound = true
		}
	}
	if !noteFound {
		t.Errorf("buildLogEntries should still include notes; entries: %+v", entries)
	}
}

func TestBuildLogEntries_DoesNotSynthesizeCreateEvent_WhenPresent(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	item := &cistern.Droplet{
		ID:         "with-create",
		Repo:       "myrepo",
		Title:      "New task",
		Priority:   1,
		Complexity: 2,
		Status:     "open",
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	changes := []cistern.DropletChange{
		{Time: now, Kind: "event", Value: "create: {\"repo\":\"myrepo\",\"title\":\"New task\",\"priority\":1,\"complexity\":2}"},
		{Time: now.Add(time.Minute), Kind: "note", Value: "implement: got started"},
	}

	entries := buildLogEntries(item, changes)

	createCount := 0
	for _, e := range entries {
		if e.Event == "created" {
			createCount++
		}
	}
	if createCount != 1 {
		t.Errorf("expected exactly 1 created entry when event exists, got %d", createCount)
	}
}

func TestBuildLogEntries_SynthesizedCreateBeforeOtherEntries(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	later := now.Add(2 * time.Minute)
	item := &cistern.Droplet{
		ID:         "pre-exist-order",
		Repo:       "myrepo",
		Title:      "Order task",
		Priority:   1,
		Complexity: 2,
		Status:     "open",
		CreatedAt:  now,
		UpdatedAt:  later,
	}

	changes := []cistern.DropletChange{
		{Time: later, Kind: "note", Value: "implement: started late"},
	}

	entries := buildLogEntries(item, changes)

	if len(entries) < 2 {
		t.Fatalf("expected at least 2 entries, got %d", len(entries))
	}

	if entries[0].Event != "created" {
		t.Errorf("first entry should be 'created', got %q", entries[0].Event)
	}
	if entries[0].Time != now.Format("2006-01-02 15:04:05") {
		t.Errorf("created entry time = %q, want %q", entries[0].Time, now.Format("2006-01-02 15:04:05"))
	}
}
