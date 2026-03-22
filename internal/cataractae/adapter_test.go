package cataractae

import (
	"context"
	"log/slog"
	"path/filepath"
	"sync"
	"testing"

	"github.com/MichielDean/cistern/internal/aqueduct"
	"github.com/MichielDean/cistern/internal/castellarius"
	"github.com/MichielDean/cistern/internal/cistern"
	"github.com/MichielDean/cistern/internal/gates"
)

// adapterCaptureHandler is a minimal slog.Handler that records log entries.
type adapterCaptureHandler struct {
	mu      sync.Mutex
	records []slog.Record
}

func (h *adapterCaptureHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }
func (h *adapterCaptureHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	h.records = append(h.records, r.Clone())
	h.mu.Unlock()
	return nil
}
func (h *adapterCaptureHandler) WithAttrs(_ []slog.Attr) slog.Handler { return h }
func (h *adapterCaptureHandler) WithGroup(_ string) slog.Handler      { return h }

func (h *adapterCaptureHandler) hasWarn() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, r := range h.records {
		if r.Level == slog.LevelWarn {
			return true
		}
	}
	return false
}

// TestSpawnAutomated_AddNoteError_LogsWarn verifies that when AddNote fails
// during an automated step, the error is logged at WARN level (not silently
// discarded) and spawnAutomated returns the SetOutcome error non-zero.
func TestSpawnAutomated_AddNoteError_LogsWarn(t *testing.T) {
	db, err := cistern.New(filepath.Join(t.TempDir(), "test.db"), "tr")
	if err != nil {
		t.Fatalf("cistern.New: %v", err)
	}
	// Close the DB immediately so that AddNote and SetOutcome both fail.
	db.Close()

	h := &adapterCaptureHandler{}
	a := &Adapter{
		runners:      nil, // not used for automated steps
		executor:     gates.New(),
		queueClients: map[string]*cistern.Client{"testrepo": db},
		logger:       slog.New(h),
	}

	req := castellarius.CataractaeRequest{
		Item:         &cistern.Droplet{ID: "test-id", Title: "Test"},
		Step:         aqueduct.WorkflowCataractae{Name: "noop", Type: aqueduct.CataractaeTypeAutomated},
		RepoConfig:   aqueduct.RepoConfig{Name: "testrepo"},
		AqueductName: "virgo",
	}

	// spawnAutomated is called because step type is automated.
	spawnErr := a.Spawn(context.Background(), req)

	// SetOutcome also fails (closed DB), so Spawn must return an error.
	if spawnErr == nil {
		t.Error("expected Spawn to return error when SetOutcome fails on closed DB")
	}

	// A WARN must have been logged for the AddNote failure.
	if !h.hasWarn() {
		t.Error("expected WARN log for AddNote failure, got none")
	}
}
