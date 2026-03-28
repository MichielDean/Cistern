package castellarius

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestWriteHealthFile_CreatesFileWithCorrectSchema verifies that after writeHealthFile
// is called, a valid JSON health file exists at {dbDir}/castellarius.health with the
// expected lastTickAt and pollIntervalSec fields.
func TestWriteHealthFile_CreatesFileWithCorrectSchema(t *testing.T) {
	dir := t.TempDir()
	s := &Castellarius{
		dbPath:       filepath.Join(dir, "cistern.db"),
		pollInterval: 10 * time.Second,
		logger:       discardLogger(),
	}

	before := time.Now().UTC().Add(-time.Second)
	s.writeHealthFile()
	after := time.Now().UTC().Add(time.Second)

	healthPath := filepath.Join(dir, "castellarius.health")
	data, err := os.ReadFile(healthPath)
	if err != nil {
		t.Fatalf("health file not created: %v", err)
	}

	var hf HealthFile
	if err := json.Unmarshal(data, &hf); err != nil {
		t.Fatalf("health file parse failed: %v", err)
	}

	if hf.PollIntervalSec != 10 {
		t.Errorf("PollIntervalSec: got %d, want 10", hf.PollIntervalSec)
	}
	if hf.LastTickAt.Before(before) || hf.LastTickAt.After(after) {
		t.Errorf("LastTickAt %v outside expected range [%v, %v]", hf.LastTickAt, before, after)
	}
}

// TestWriteHealthFile_OverwritesPreviousFile verifies that a second call to writeHealthFile
// updates the file rather than failing or leaving stale data.
func TestWriteHealthFile_OverwritesPreviousFile(t *testing.T) {
	dir := t.TempDir()
	s := &Castellarius{
		dbPath:       filepath.Join(dir, "cistern.db"),
		pollInterval: 30 * time.Second,
		logger:       discardLogger(),
	}

	s.writeHealthFile()
	time.Sleep(2 * time.Millisecond) // ensure second write has a later timestamp
	s.writeHealthFile()

	data, err := os.ReadFile(filepath.Join(dir, "castellarius.health"))
	if err != nil {
		t.Fatalf("health file missing after second write: %v", err)
	}
	var hf HealthFile
	if err := json.Unmarshal(data, &hf); err != nil {
		t.Fatalf("health file parse failed: %v", err)
	}
	if hf.PollIntervalSec != 30 {
		t.Errorf("PollIntervalSec: got %d, want 30", hf.PollIntervalSec)
	}
}

// TestWriteHealthFile_NoTmpFileLeftBehind verifies that the .tmp file is cleaned up
// and only castellarius.health remains after a successful write.
func TestWriteHealthFile_NoTmpFileLeftBehind(t *testing.T) {
	dir := t.TempDir()
	s := &Castellarius{
		dbPath:       filepath.Join(dir, "cistern.db"),
		pollInterval: 5 * time.Second,
		logger:       discardLogger(),
	}

	s.writeHealthFile()

	tmpPath := filepath.Join(dir, "castellarius.health.tmp")
	if _, err := os.Stat(tmpPath); !os.IsNotExist(err) {
		t.Errorf("expected .tmp file to be absent after write, got: %v", err)
	}
}

// TestWriteHealthFile_EmptyDBPath_DoesNotPanic verifies that writeHealthFile
// handles an empty dbPath gracefully without panicking.
func TestWriteHealthFile_EmptyDBPath_DoesNotPanic(t *testing.T) {
	s := &Castellarius{
		dbPath:       "",
		pollInterval: 10 * time.Second,
		logger:       discardLogger(),
	}
	// Should not panic; errors are logged and swallowed.
	s.writeHealthFile()
}

// TestReadHealthFile_ReturnsHealthWhenFilePresent verifies that a well-formed health
// file is parsed correctly and returned with the expected field values.
func TestReadHealthFile_ReturnsHealthWhenFilePresent(t *testing.T) {
	dir := t.TempDir()
	now := time.Now().UTC().Truncate(time.Second)

	hf := HealthFile{
		LastTickAt:      now,
		PollIntervalSec: 30,
	}
	b, err := json.Marshal(hf)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "castellarius.health"), b, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	got, err := ReadHealthFile(dir)
	if err != nil {
		t.Fatalf("ReadHealthFile: %v", err)
	}
	if got.PollIntervalSec != 30 {
		t.Errorf("PollIntervalSec: got %d, want 30", got.PollIntervalSec)
	}
	if !got.LastTickAt.Equal(now) {
		t.Errorf("LastTickAt: got %v, want %v", got.LastTickAt, now)
	}
}

// TestReadHealthFile_ReturnsErrorWhenFileMissing verifies that ReadHealthFile returns
// a non-nil error when castellarius.health does not exist.
func TestReadHealthFile_ReturnsErrorWhenFileMissing(t *testing.T) {
	dir := t.TempDir()
	_, err := ReadHealthFile(dir)
	if err == nil {
		t.Fatal("expected error for missing health file, got nil")
	}
}

// TestReadHealthFile_ReturnsErrorOnMalformedJSON verifies that ReadHealthFile returns
// an error when the file exists but contains invalid JSON.
func TestReadHealthFile_ReturnsErrorOnMalformedJSON(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "castellarius.health"), []byte("not json{{"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, err := ReadHealthFile(dir)
	if err == nil {
		t.Fatal("expected error for malformed JSON, got nil")
	}
}

// --- Drought fields schema tests ---

// TestHealthFile_DroughtRunning_MarshalsFalseByDefault verifies that the zero
// value of HealthFile serializes droughtRunning as false (not absent).
func TestHealthFile_DroughtRunning_MarshalsFalseByDefault(t *testing.T) {
	hf := HealthFile{LastTickAt: time.Now().UTC(), PollIntervalSec: 10}
	b, err := json.Marshal(hf)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(b), `"droughtRunning":false`) {
		t.Errorf("expected droughtRunning:false in JSON, got: %s", b)
	}
}

// TestHealthFile_DroughtStartedAt_NullWhenNil verifies that a nil DroughtStartedAt
// marshals to droughtStartedAt:null in JSON.
func TestHealthFile_DroughtStartedAt_NullWhenNil(t *testing.T) {
	hf := HealthFile{LastTickAt: time.Now().UTC(), PollIntervalSec: 10}
	b, err := json.Marshal(hf)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(b), `"droughtStartedAt":null`) {
		t.Errorf("expected droughtStartedAt:null in JSON, got: %s", b)
	}
}

// TestHealthFile_DroughtRunning_TrueAndStartedAt_RoundTrip verifies that
// droughtRunning:true and a non-nil droughtStartedAt survive a JSON round-trip.
func TestHealthFile_DroughtRunning_TrueAndStartedAt_RoundTrip(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	hf := HealthFile{
		LastTickAt:       now,
		PollIntervalSec:  30,
		DroughtRunning:   true,
		DroughtStartedAt: &now,
	}
	b, err := json.Marshal(hf)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got HealthFile
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !got.DroughtRunning {
		t.Error("DroughtRunning: expected true after round-trip, got false")
	}
	if got.DroughtStartedAt == nil {
		t.Fatal("DroughtStartedAt: expected non-nil after round-trip, got nil")
	}
	if !got.DroughtStartedAt.Equal(now) {
		t.Errorf("DroughtStartedAt: got %v, want %v", *got.DroughtStartedAt, now)
	}
}

// TestWriteHealthFile_IncludesDroughtRunningTrue_WhenDroughtActive verifies that
// writeHealthFile writes droughtRunning:true when the scheduler tracks drought activity.
func TestWriteHealthFile_IncludesDroughtRunningTrue_WhenDroughtActive(t *testing.T) {
	dir := t.TempDir()
	s := &Castellarius{
		dbPath:       filepath.Join(dir, "cistern.db"),
		pollInterval: 10 * time.Second,
		logger:       discardLogger(),
	}
	now := time.Now().UTC()
	s.droughtRunning.Store(true)
	s.droughtStartedAt.Store(&now)

	s.writeHealthFile()

	data, err := os.ReadFile(filepath.Join(dir, "castellarius.health"))
	if err != nil {
		t.Fatalf("health file not created: %v", err)
	}
	var hf HealthFile
	if err := json.Unmarshal(data, &hf); err != nil {
		t.Fatalf("health file parse failed: %v", err)
	}
	if !hf.DroughtRunning {
		t.Error("DroughtRunning: expected true, got false")
	}
	if hf.DroughtStartedAt == nil {
		t.Error("DroughtStartedAt: expected non-nil, got nil")
	}
}

// TestWriteHealthFile_DroughtRunningFalse_WhenNoActiveDrought verifies that
// writeHealthFile writes droughtRunning:false when the scheduler has no active drought.
func TestWriteHealthFile_DroughtRunningFalse_WhenNoActiveDrought(t *testing.T) {
	dir := t.TempDir()
	s := &Castellarius{
		dbPath:       filepath.Join(dir, "cistern.db"),
		pollInterval: 10 * time.Second,
		logger:       discardLogger(),
	}

	s.writeHealthFile()

	data, err := os.ReadFile(filepath.Join(dir, "castellarius.health"))
	if err != nil {
		t.Fatalf("health file not created: %v", err)
	}
	var hf HealthFile
	if err := json.Unmarshal(data, &hf); err != nil {
		t.Fatalf("health file parse failed: %v", err)
	}
	if hf.DroughtRunning {
		t.Error("DroughtRunning: expected false, got true")
	}
	if hf.DroughtStartedAt != nil {
		t.Errorf("DroughtStartedAt: expected nil, got %v", hf.DroughtStartedAt)
	}
}
