package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestVersionCmd_JsonFlag(t *testing.T) {
	f := versionCmd.Flags().Lookup("json")
	if f == nil {
		t.Fatal("--json flag not registered on version command")
	}
	if f.DefValue != "false" {
		t.Fatalf("expected default false, got %q", f.DefValue)
	}
}

// TestVersionCmd_DefaultValues_PlainOutput verifies that with the default variable
// values (version="dev", commit="unknown"), ct version prints "ct dev".
func TestVersionCmd_DefaultValues_PlainOutput(t *testing.T) {
	savedVersion, savedCommit := version, commit
	defer func() { version, commit = savedVersion, savedCommit }()

	version = "dev"
	commit = "unknown"

	if err := versionCmd.Flags().Set("json", "false"); err != nil {
		t.Fatalf("failed to reset --json flag: %v", err)
	}

	output := captureStdout(t, func() {
		versionCmd.Run(versionCmd, []string{})
	})

	got := strings.TrimSpace(output)
	if got != "ct dev" {
		t.Errorf("expected 'ct dev', got %q", got)
	}
}

// TestVersionCmd_JsonOutput verifies that --json emits valid JSON containing the
// version and commit values currently set in the version variables.
func TestVersionCmd_JsonOutput(t *testing.T) {
	savedVersion, savedCommit := version, commit
	defer func() {
		version, commit = savedVersion, savedCommit
		_ = versionCmd.Flags().Set("json", "false")
	}()

	version = "1.2.3"
	commit = "abc1234"

	if err := versionCmd.Flags().Set("json", "true"); err != nil {
		t.Fatalf("failed to set --json flag: %v", err)
	}

	output := captureStdout(t, func() {
		versionCmd.Run(versionCmd, []string{})
	})

	var got map[string]string
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &got); err != nil {
		t.Fatalf("json.Unmarshal failed on output %q: %v", output, err)
	}
	if got["version"] != "1.2.3" {
		t.Errorf("expected version 1.2.3, got %q", got["version"])
	}
	if got["commit"] != "abc1234" {
		t.Errorf("expected commit abc1234, got %q", got["commit"])
	}
}

// TestVersionCmd_PlainOutput verifies that without --json the command prints "ct <version>".
func TestVersionCmd_PlainOutput(t *testing.T) {
	savedVersion := version
	defer func() { version = savedVersion }()

	version = "2.0.0"

	if err := versionCmd.Flags().Set("json", "false"); err != nil {
		t.Fatalf("failed to reset --json flag: %v", err)
	}

	output := captureStdout(t, func() {
		versionCmd.Run(versionCmd, []string{})
	})

	got := strings.TrimSpace(output)
	if got != "ct 2.0.0" {
		t.Errorf("expected 'ct 2.0.0', got %q", got)
	}
}
