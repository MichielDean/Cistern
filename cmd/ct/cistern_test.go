package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/MichielDean/citadel/internal/queue"
)

func TestCisternListOutputFlag(t *testing.T) {
	// Set up a temp DB.
	dir := t.TempDir()
	db := filepath.Join(dir, "test.db")
	t.Setenv("CT_DB", db)

	// Verify default flag value is "table".
	f := cisternListCmd.Flags().Lookup("output")
	if f == nil {
		t.Fatal("--output flag not registered")
	}
	if f.DefValue != "table" {
		t.Fatalf("expected default 'table', got %q", f.DefValue)
	}

	// Test json output with empty queue.
	t.Run("json empty", func(t *testing.T) {
		old := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		listOutput = "json"
		listRepo = ""
		listStatus = ""
		err := cisternListCmd.RunE(cisternListCmd, nil)

		w.Close()
		os.Stdout = old

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		var buf bytes.Buffer
		buf.ReadFrom(r)
		out := buf.String()

		var items []*queue.WorkItem
		if err := json.Unmarshal([]byte(out), &items); err != nil {
			t.Fatalf("output is not valid JSON: %v\noutput: %s", err, out)
		}
		if len(items) != 0 {
			t.Fatalf("expected empty array, got %d items", len(items))
		}
	})

	// Test json output with one item.
	t.Run("json with items", func(t *testing.T) {
		c, err := queue.New(db, "ts")
		if err != nil {
			t.Fatal(err)
		}
		item, err := c.Add("github.com/test/repo", "Test item", "", 1, 3)
		c.Close()
		if err != nil {
			t.Fatal(err)
		}

		old := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		listOutput = "json"
		listRepo = ""
		listStatus = ""
		err = cisternListCmd.RunE(cisternListCmd, nil)

		w.Close()
		os.Stdout = old

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		var buf bytes.Buffer
		buf.ReadFrom(r)
		out := buf.String()

		var items []*queue.WorkItem
		if err := json.Unmarshal([]byte(out), &items); err != nil {
			t.Fatalf("output is not valid JSON: %v\noutput: %s", err, out)
		}
		if len(items) != 1 {
			t.Fatalf("expected 1 item, got %d", len(items))
		}
		if items[0].ID != item.ID {
			t.Fatalf("expected ID %q, got %q", item.ID, items[0].ID)
		}
	})

	// Test invalid output flag.
	t.Run("invalid output flag", func(t *testing.T) {
		listOutput = "csv"
		err := cisternListCmd.RunE(cisternListCmd, nil)
		if err == nil {
			t.Fatal("expected error for invalid --output value")
		}
	})

	// Reset flag.
	listOutput = "table"
}

func TestParseComplexity(t *testing.T) {
	tests := []struct {
		input   string
		want    int
		wantErr bool
	}{
		{"1", 1, false},
		{"2", 2, false},
		{"3", 3, false},
		{"4", 4, false},
		{"trivial", 1, false},
		{"standard", 2, false},
		{"full", 3, false},
		{"critical", 4, false},
		{"", 3, false},
		{"5", 0, true},
		{"foo", 0, true},
	}

	for _, tt := range tests {
		got, err := parseComplexity(tt.input)
		if tt.wantErr {
			if err == nil {
				t.Errorf("parseComplexity(%q) = %d, want error", tt.input, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseComplexity(%q) error: %v", tt.input, err)
			continue
		}
		if got != tt.want {
			t.Errorf("parseComplexity(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

func TestComplexityName(t *testing.T) {
	tests := []struct {
		level int
		want  string
	}{
		{1, "trivial"},
		{2, "standard"},
		{3, "full"},
		{4, "critical"},
		{0, "full"},
		{99, "full"},
	}
	for _, tt := range tests {
		got := complexityName(tt.level)
		if got != tt.want {
			t.Errorf("complexityName(%d) = %q, want %q", tt.level, got, tt.want)
		}
	}
}
