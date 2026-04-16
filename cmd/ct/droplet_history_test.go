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

func setupHistoryTestDB(t *testing.T) *cistern.Client {
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

func runHistoryCapture(t *testing.T, id string) (string, error) {
	t.Helper()
	var buf bytes.Buffer
	err := runHistory(&buf, id)
	return buf.String(), err
}

func TestDropletHistory_ShowsCreationAndNotes(t *testing.T) {
	c := setupHistoryTestDB(t)
	item, err := c.Add("myrepo", "History task", "do something", 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	c.AddNote(item.ID, "implement", "wrote the code")
	c.AddNote(item.ID, "review", "looks good")

	out, err := runHistoryCapture(t, item.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(out, "created") {
		t.Errorf("history output missing 'created' event: %s", out)
	}
	if !strings.Contains(out, "implement") {
		t.Errorf("history output missing 'implement' note: %s", out)
	}
	if !strings.Contains(out, "review") {
		t.Errorf("history output missing 'review' note: %s", out)
	}
	if !strings.Contains(out, "wrote the code") {
		t.Errorf("history output missing note content: %s", out)
	}
}

func TestDropletHistory_ShowsPoolEvent(t *testing.T) {
	c := setupHistoryTestDB(t)
	item, err := c.Add("myrepo", "Pool history task", "", 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	c.Pool(item.ID, "needs human review")

	out, err := runHistoryCapture(t, item.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(out, "pooled") {
		t.Errorf("history output missing 'pooled' event: %s", out)
	}
}

func TestDropletHistory_ShowsHeader(t *testing.T) {
	c := setupHistoryTestDB(t)
	item, err := c.Add("myrepo", "Header history task", "desc", 1, 2)
	if err != nil {
		t.Fatal(err)
	}

	out, err := runHistoryCapture(t, item.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(out, item.ID) {
		t.Errorf("history output missing droplet ID: %s", out)
	}
	if !strings.Contains(out, "Header history task") {
		t.Errorf("history output missing droplet title: %s", out)
	}
}

func TestDropletHistory_ChronologicalOrder(t *testing.T) {
	c := setupHistoryTestDB(t)
	item, err := c.Add("myrepo", "Order history task", "", 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	c.AddNote(item.ID, "implement", "first note")
	time.Sleep(10 * time.Millisecond)
	c.AddNote(item.ID, "review", "second note")

	out, err := runHistoryCapture(t, item.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	firstIdx := strings.Index(out, "first note")
	secondIdx := strings.Index(out, "second note")
	if firstIdx == -1 || secondIdx == -1 {
		t.Fatalf("history output missing expected notes: %s", out)
	}
	if firstIdx > secondIdx {
		t.Errorf("notes not in chronological order: first note at %d, second at %d", firstIdx, secondIdx)
	}
}

func TestDropletHistory_NonexistentDroplet(t *testing.T) {
	_ = setupHistoryTestDB(t)

	_, err := runHistoryCapture(t, "nonexistent-id")
	if err == nil {
		t.Error("expected error for nonexistent droplet")
	}
}

func TestDropletHistory_EmptyDroplet(t *testing.T) {
	c := setupHistoryTestDB(t)
	item, err := c.Add("myrepo", "Empty history task", "", 1, 2)
	if err != nil {
		t.Fatal(err)
	}

	out, err := runHistoryCapture(t, item.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(out, "created") {
		t.Errorf("history output should show creation event even for empty droplet: %s", out)
	}
}

func TestDropletHistory_JsonFormat(t *testing.T) {
	historyFmt = "json"
	t.Cleanup(func() { historyFmt = "text" })

	c := setupHistoryTestDB(t)
	item, err := c.Add("myrepo", "Json history task", "", 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	c.AddNote(item.ID, "implement", "wrote code")

	var buf bytes.Buffer
	err = runHistory(&buf, item.ID)
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

func TestDropletHistory_InvalidFormat(t *testing.T) {
	historyFmt = "xml"
	t.Cleanup(func() { historyFmt = "text" })

	c := setupHistoryTestDB(t)
	item, err := c.Add("myrepo", "Format history task", "", 1, 2)
	if err != nil {
		t.Fatal(err)
	}

	_, err = runHistoryCapture(t, item.ID)
	if err == nil {
		t.Error("expected error for invalid format")
	}
	if !strings.Contains(err.Error(), "format must be text or json") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestDropletHistory_ShowsHeartbeat(t *testing.T) {
	c := setupHistoryTestDB(t)
	item, err := c.Add("myrepo", "Heartbeat history task", "", 1, 2)
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

	out, err := runHistoryCapture(t, item.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(out, "heartbeat") {
		t.Errorf("history output missing 'heartbeat' event: %s", out)
	}
	if !strings.Contains(out, "last heartbeat recorded") {
		t.Errorf("history output missing heartbeat detail: %s", out)
	}
}

func TestDropletHistory_ProducesSameOutputAsLog(t *testing.T) {
	c := setupHistoryTestDB(t)
	item, err := c.Add("myrepo", "Comparison task", "compare log vs history", 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	c.AddNote(item.ID, "implement", "doing stuff")
	c.Pool(item.ID, "blocked")

	logFmt = "text"
	var logBuf bytes.Buffer
	err = runLog(&logBuf, item.ID)
	if err != nil {
		t.Fatalf("runLog error: %v", err)
	}

	historyFmt = "text"
	var historyBuf bytes.Buffer
	err = runHistory(&historyBuf, item.ID)
	if err != nil {
		t.Fatalf("runHistory error: %v", err)
	}

	if logBuf.String() != historyBuf.String() {
		t.Errorf("history and log output differ:\nLOG:\n%s\nHISTORY:\n%s", logBuf.String(), historyBuf.String())
	}
}
