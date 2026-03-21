package cataractae

import (
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
