package cataractae

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildClaudeCmd_ContainsAddDir(t *testing.T) {
	s := &Session{ID: "test", WorkDir: "/tmp"}
	skillsDir := "/home/user/.cistern/skills"
	cmd := s.buildClaudeCmd(skillsDir)
	if !strings.Contains(cmd, "--add-dir") {
		t.Errorf("claudeCmd missing --add-dir flag: %s", cmd)
	}
	if !strings.Contains(cmd, skillsDir) {
		t.Errorf("claudeCmd missing skillsDir %q: %s", skillsDir, cmd)
	}
}

func TestBuildClaudeCmd_QuotesPathWithSpaces(t *testing.T) {
	s := &Session{ID: "test", WorkDir: "/tmp"}
	skillsDir := "/home/john doe/.cistern/skills"
	cmd := s.buildClaudeCmd(skillsDir)

	// Unquoted form must not appear — it would split at the space.
	if strings.Contains(cmd, "--add-dir /home/john doe/") {
		t.Errorf("claudeCmd contains unquoted path with space — will break shell: %s", cmd)
	}
	// Shell-quoted form must be present.
	want := "--add-dir '/home/john doe/.cistern/skills'"
	if !strings.Contains(cmd, want) {
		t.Errorf("claudeCmd missing shell-quoted skillsDir\nwant substring: %s\ngot: %s", want, cmd)
	}
}

func TestBuildClaudeCmd_WithModel(t *testing.T) {
	s := &Session{ID: "test", WorkDir: "/tmp", Model: "haiku"}
	cmd := s.buildClaudeCmd("/home/user/.cistern/skills")
	if !strings.Contains(cmd, "--model haiku") {
		t.Errorf("claudeCmd missing --model flag: %s", cmd)
	}
}

func TestBuildClaudeCmd_WithoutModel(t *testing.T) {
	s := &Session{ID: "test", WorkDir: "/tmp"}
	cmd := s.buildClaudeCmd("/home/user/.cistern/skills")
	if strings.Contains(cmd, "--model") {
		t.Errorf("claudeCmd should not contain --model when model is empty: %s", cmd)
	}
}

func TestShellQuote(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"/home/user/.cistern/skills", "'/home/user/.cistern/skills'"},
		{"/home/john doe/.cistern/skills", "'/home/john doe/.cistern/skills'"},
		{"it's a path", "'it'\\''s a path'"},
	}
	for _, tt := range tests {
		got := shellQuote(tt.input)
		if got != tt.want {
			t.Errorf("shellQuote(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestBuildPrompt_WithIdentity_FileFound(t *testing.T) {
	dir := t.TempDir()
	identityDir := filepath.Join(dir, ".cistern", "cataractae", "implementer")
	if err := os.MkdirAll(identityDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(identityDir, "CLAUDE.md"),
		[]byte("# Implementer\n\nYou implement things.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", dir)

	s := &Session{ID: "test", WorkDir: dir, Identity: "implementer"}
	prompt := s.buildPrompt()

	if !strings.Contains(prompt, "## Your Role") {
		t.Error("prompt missing '## Your Role' section when identity file is present")
	}
	if !strings.Contains(prompt, "You implement things.") {
		t.Error("prompt missing identity file content")
	}
	if !strings.Contains(prompt, baseCataractaePrompt) {
		t.Error("prompt missing constitutional base")
	}
}

func TestBuildPrompt_WithIdentity_FileMissing(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir) // no CLAUDE.md at cistern identity path

	s := &Session{ID: "test", WorkDir: dir, Identity: "implementer"}
	prompt := s.buildPrompt()

	// Fallback: prompt contains a pointer to the missing path.
	if !strings.Contains(prompt, "Read") {
		t.Error("prompt missing 'Read' fallback pointer when identity file is missing")
	}
	if !strings.Contains(prompt, "implementer") {
		t.Error("prompt missing identity name in fallback")
	}
	if strings.Contains(prompt, "## Your Role") {
		t.Error("prompt should not contain '## Your Role' when identity file is missing")
	}
}

func TestResolveIdentityPath_CisternHome(t *testing.T) {
	dir := t.TempDir()
	cisternPath := filepath.Join(dir, ".cistern", "cataractae", "reviewer", "CLAUDE.md")
	if err := os.MkdirAll(filepath.Dir(cisternPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cisternPath, []byte("# Reviewer"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", dir)

	s := &Session{Identity: "reviewer"}
	got := s.resolveIdentityPath()
	if got != cisternPath {
		t.Errorf("resolveIdentityPath = %q, want %q", got, cisternPath)
	}
}

func TestResolveIdentityPath_FallbackSandbox(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir) // no CLAUDE.md at cistern identity path

	s := &Session{Identity: "implementer"}
	got := s.resolveIdentityPath()
	want := "cataractae/implementer/CLAUDE.md"
	if got != want {
		t.Errorf("resolveIdentityPath = %q, want %q", got, want)
	}
}

func TestClaudePath_EnvOverride(t *testing.T) {
	t.Setenv("CLAUDE_PATH", "/usr/local/bin/my-claude")
	got := claudePath()
	if got != "/usr/local/bin/my-claude" {
		t.Errorf("claudePath() = %q, want %q", got, "/usr/local/bin/my-claude")
	}
}

func TestClaudePath_LookPath(t *testing.T) {
	t.Setenv("CLAUDE_PATH", "")
	// Place a fake "claude" executable on PATH so exec.LookPath finds it.
	dir := t.TempDir()
	fakeClaude := filepath.Join(dir, "claude")
	if err := os.WriteFile(fakeClaude, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))

	got := claudePath()
	if got != fakeClaude {
		t.Errorf("claudePath() = %q, want %q", got, fakeClaude)
	}
}
