package sessionlog

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPath_ReturnsExpectedFormat(t *testing.T) {
	p, err := Path("myrepo-alpha")
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(p) != "myrepo-alpha.log" {
		t.Errorf("Base(Path) = %q, want %q", filepath.Base(p), "myrepo-alpha.log")
	}
	if filepath.Ext(p) != ".log" {
		t.Errorf("Ext(Path) = %q, want .log", filepath.Ext(p))
	}
}

func TestEnsureDir_CreatesDirectory(t *testing.T) {
	dir := t.TempDir()
	orig := LogDirFn
	LogDirFn = func() (string, error) { return filepath.Join(dir, "session-logs"), nil }
	t.Cleanup(func() { LogDirFn = orig })

	if err := EnsureDir(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "session-logs")); err != nil {
		t.Errorf("expected directory to exist: %v", err)
	}
}

func TestMtime_ReturnsZeroWhenFileMissing(t *testing.T) {
	dir := t.TempDir()
	orig := LogDirFn
	LogDirFn = func() (string, error) { return dir, nil }
	t.Cleanup(func() { LogDirFn = orig })

	mtime, err := Mtime("nonexistent")
	if err != nil {
		t.Fatal(err)
	}
	if !mtime.IsZero() {
		t.Errorf("Mtime of missing file = %v, want zero", mtime)
	}
}

func TestMtime_ReturnsModificationTime(t *testing.T) {
	dir := t.TempDir()
	orig := LogDirFn
	LogDirFn = func() (string, error) { return dir, nil }
	t.Cleanup(func() { LogDirFn = orig })

	before := time.Now().UTC().Truncate(time.Second)
	logPath := filepath.Join(dir, "test.log")
	if err := os.WriteFile(logPath, []byte("output"), 0o644); err != nil {
		t.Fatal(err)
	}

	mtime, err := Mtime("test")
	if err != nil {
		t.Fatal(err)
	}
	if mtime.Before(before) {
		t.Errorf("Mtime = %v, want >= %v", mtime, before)
	}
}

func TestRead_ReturnsContent(t *testing.T) {
	dir := t.TempDir()
	orig := LogDirFn
	LogDirFn = func() (string, error) { return dir, nil }
	t.Cleanup(func() { LogDirFn = orig })

	content := "line 1\nline 2\n"
	logPath := filepath.Join(dir, "test.log")
	if err := os.WriteFile(logPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	data, err := Read("test")
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != content {
		t.Errorf("Read = %q, want %q", string(data), content)
	}
}

func TestRead_ReturnsErrorWhenMissing(t *testing.T) {
	dir := t.TempDir()
	orig := LogDirFn
	LogDirFn = func() (string, error) { return dir, nil }
	t.Cleanup(func() { LogDirFn = orig })

	_, err := Read("nonexistent")
	if err == nil {
		t.Error("Read of missing file should return error")
	}
	if !os.IsNotExist(err) {
		t.Errorf("error = %v, want IsNotExist", err)
	}
}