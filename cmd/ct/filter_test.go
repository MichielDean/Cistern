package main

import (
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/MichielDean/cistern/internal/provider"
)

// --- callFilterAgent tests (deprecated path — still returns error) ---

// TestCallFilterAgent_Deprecated_AlwaysErrors verifies that the deprecated
// callFilterAgent function always returns an error pointing to the tmux approach.
func TestCallFilterAgent_Deprecated_AlwaysErrors(t *testing.T) {
	preset := provider.ProviderPreset{
		Name:    "test",
		Command: "true",
	}

	_, err := callFilterAgent(preset, nil, "test prompt")
	if err == nil {
		t.Fatal("expected callFilterAgent to return error, got nil")
	}
	if !strings.Contains(err.Error(), "deprecated") {
		t.Errorf("error should mention deprecation, got: %v", err)
	}
}

// --- filterAgentRun tests ---

// TestFilterAgentRun_SignatureExists verifies that filterAgentRun
// compiles and exists. The real integration test runs in CI with opencode.
func TestFilterAgentRun_SignatureExists(t *testing.T) {
	// This test verifies the function signature exists.
	_ = filterAgentRun
	_ = filterAgentRunResume
}

// --- invokeFilterNew tests ---

// TestInvokeFilterNew_WithContextBlock_IncludesContextInResult verifies that
// invokeFilterNew accepts a non-empty contextBlock without panicking.
// This tests that buildFilterPrompt works correctly.
func TestInvokeFilterNew_WithContextBlock_IncludesContextInResult(t *testing.T) {
	// This test verifies buildFilterPrompt, not the full tmux path
	contextBlock := "=== CODEBASE CONTEXT ===\nsome schema here\n=== END CODEBASE CONTEXT ==="
	userPrompt := "Title: Add feature\nDescription: Some description"
	prompt := buildFilterPrompt(contextBlock, userPrompt)

	if !strings.Contains(prompt, contextBlock) {
		t.Errorf("prompt must contain context block, got:\n%s", prompt)
	}
	if !strings.Contains(prompt, userPrompt) {
		t.Errorf("prompt must contain user prompt, got:\n%s", prompt)
	}
}

// --- filterAgentsMD tests ---

// TestFilterAgentMD_ContainsFilterInstructions verifies that the agent
// instructions include the key filter directives.
func TestFilterAgentsMD_ContainsFilterInstructions(t *testing.T) {
	md := filterAgentsMD()
	if !strings.Contains(md, "CONTEXT.md") {
		t.Error("filterAgentsMD must reference CONTEXT.md")
	}
}

// --- printFilterResult tests ---

// TestPrintFilterResult_HumanFormat verifies that printFilterResult with "human"
// format writes result.Text to stdout and result.SessionID to stderr.
func TestPrintFilterResult_HumanFormat(t *testing.T) {
	result := filterSessionResult{
		SessionID: "test-session",
		Text:      "1. Fix login bug\n   Handle edge case in auth flow. No dependencies.",
	}

	rOut, wOut, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe stdout: %v", err)
	}
	rErr, wErr, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe stderr: %v", err)
	}
	oldStdout, oldStderr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = wOut, wErr
	t.Cleanup(func() { os.Stdout, os.Stderr = oldStdout, oldStderr })

	if err := printFilterResult(result, "human"); err != nil {
		t.Fatalf("printFilterResult human: unexpected error: %v", err)
	}

	wOut.Close()
	wErr.Close()
	var bufOut, bufErr strings.Builder
	if _, err := io.Copy(&bufOut, rOut); err != nil {
		t.Fatalf("reading stdout: %v", err)
	}
	if _, err := io.Copy(&bufErr, rErr); err != nil {
		t.Fatalf("reading stderr: %v", err)
	}

	if !strings.Contains(bufOut.String(), result.Text) {
		t.Errorf("stdout %q does not contain expected text %q", bufOut.String(), result.Text)
	}
	if !strings.Contains(bufErr.String(), result.SessionID) {
		t.Errorf("stderr %q does not contain expected session_id %q", bufErr.String(), result.SessionID)
	}
}

// TestPrintFilterResult_JSONFormat verifies that printFilterResult with "json"
// format writes valid JSON to stdout containing session_id and text fields.
func TestPrintFilterResult_JSONFormat(t *testing.T) {
	result := filterSessionResult{
		SessionID: "session-xyz",
		Text:      "1. Add caching\n   Implement Redis caching. No dependencies.",
	}

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	oldStdout := os.Stdout
	os.Stdout = w
	t.Cleanup(func() { os.Stdout = oldStdout })

	if err := printFilterResult(result, "json"); err != nil {
		t.Fatalf("printFilterResult json: unexpected error: %v", err)
	}

	w.Close()
	var buf strings.Builder
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatalf("reading stdout: %v", err)
	}

	var got map[string]interface{}
	if err := json.Unmarshal([]byte(buf.String()), &got); err != nil {
		t.Fatalf("decode JSON output %q: %v", buf.String(), err)
	}
	if got["session_id"] != result.SessionID {
		t.Errorf("session_id: got %q, want %q", got["session_id"], result.SessionID)
	}
	if got["text"] != result.Text {
		t.Errorf("text: got %q, want %q", got["text"], result.Text)
	}
}

// --- ct filter command flag validation tests ---

// TestFilterCmd_NewSession_RequiresTitle verifies that ct filter without --title
// and without --resume returns an error mentioning --title.
func TestFilterCmd_NewSession_RequiresTitle(t *testing.T) {
	err := execCmd(t, "filter")
	if err == nil {
		t.Fatal("expected error when --title is missing, got nil")
	}
	if !strings.Contains(err.Error(), "--title") {
		t.Errorf("error %q does not mention --title", err.Error())
	}
}

// TestFilterCmd_Resume_RequiresFeedback verifies that ct filter --resume
// without a feedback argument returns an error mentioning "feedback".
func TestFilterCmd_Resume_RequiresFeedback(t *testing.T) {
	err := execCmd(t, "filter", "--resume", "some-session-id")
	if err == nil {
		t.Fatal("expected error when --resume without feedback, got nil")
	}
	if !strings.Contains(err.Error(), "feedback") {
		t.Errorf("error %q does not mention feedback", err.Error())
	}
}

// TestFilterCmd_SkipContextFlag_IsRejected verifies that --skip-context is no
// longer a recognized flag.
func TestFilterCmd_SkipContextFlag_IsRejected(t *testing.T) {
	t.Cleanup(func() {
		filterTitle = ""
	})

	err := execCmd(t, "filter", "--title", "test idea", "--skip-context")
	if err == nil {
		t.Fatal("expected error for removed --skip-context flag, got nil")
	}
	if !strings.Contains(err.Error(), "unknown flag: --skip-context") {
		t.Errorf("expected 'unknown flag: --skip-context' error, got: %v", err)
	}
}

// TestFilterCmd_FileFlag_IsRejected verifies that --file is no longer recognized.
func TestFilterCmd_FileFlag_IsRejected(t *testing.T) {
	t.Cleanup(func() {
		filterTitle = ""
	})

	err := execCmd(t, "filter", "--title", "test idea", "--file")
	if err == nil {
		t.Fatal("expected error for removed --file flag, got nil")
	}
	if !strings.Contains(err.Error(), "unknown flag: --file") {
		t.Errorf("expected 'unknown flag: --file' error, got: %v", err)
	}
}

// TestFilterCmd_RepoFlag_IsRejected verifies that --repo is no longer recognized.
func TestFilterCmd_RepoFlag_IsRejected(t *testing.T) {
	t.Cleanup(func() {
		filterTitle = ""
	})

	err := execCmd(t, "filter", "--title", "test idea", "--repo", "SomeRepo")
	if err == nil {
		t.Fatal("expected error for removed --repo flag, got nil")
	}
	if !strings.Contains(err.Error(), "unknown flag: --repo") {
		t.Errorf("expected 'unknown flag: --repo' error, got: %v", err)
	}
}

// TestFilterCmd_PromptAlwaysHasContextHeader verifies that buildFilterPrompt
// always includes the context header. The end-to-end path now uses tmux spawning,
// so this tests the prompt construction directly.
func TestFilterCmd_PromptAlwaysHasContextHeader(t *testing.T) {
	contextBlock := "=== CODEBASE CONTEXT ===\nsome schema here\n=== END CODEBASE CONTEXT ==="
	userPrompt := "Title: test idea"
	prompt := buildFilterPrompt(contextBlock, userPrompt)

	if !strings.Contains(prompt, "=== CODEBASE CONTEXT ===") {
		t.Errorf("prompt must always contain context header, got:\n%s", prompt)
	}
}

// --- buildFilterRunCommand tests ---

// TestBuildFilterRunCommand_ContainsFormatJson verifies that the opencode run
// command includes --format json for parseable output.
func TestBuildFilterRunCommand_ContainsFormatJson(t *testing.T) {
	preset := provider.ProviderPreset{
		Name:             "opencode",
		Command:          "opencode",
		Subcommand:       "run",
		Args:             []string{"--dangerously-skip-permissions"},
		DefaultModel:     "ollama/glm-5.1:cloud",
		ModelFlag:        "--model",
		AgentFlag:        "--agent",
		PromptPositional: true,
	}

	_, args, _, _, err := buildFilterRunCommand(preset, "Test prompt", "")
	if err != nil {
		t.Fatalf("buildFilterRunCommand: %v", err)
	}

	// Must contain --format json
	hasFormat := false
	for _, arg := range args {
		if arg == "--format" {
			hasFormat = true
		}
	}
	if !hasFormat {
		t.Errorf("args must contain --format, got: %v", args)
	}
}

// TestBuildFilterRunCommand_ContainsDangerouslySkipPermissions verifies that
// --dangerously-skip-permissions is included.
func TestBuildFilterRunCommand_ContainsDangerouslySkipPermissions(t *testing.T) {
	preset := provider.ProviderPreset{
		Name:             "opencode",
		Command:          "opencode",
		Subcommand:       "run",
		Args:             []string{"--dangerously-skip-permissions"},
		DefaultModel:     "ollama/glm-5.1:cloud",
		ModelFlag:        "--model",
		AgentFlag:        "--agent",
		PromptPositional: true,
	}

	_, args, _, _, err := buildFilterRunCommand(preset, "Test prompt", "")
	if err != nil {
		t.Fatalf("buildFilterRunCommand: %v", err)
	}

	hasSkipPerms := false
	for _, arg := range args {
		if arg == "--dangerously-skip-permissions" {
			hasSkipPerms = true
		}
	}
	if !hasSkipPerms {
		t.Errorf("args must contain --dangerously-skip-permissions, got: %v", args)
	}
}

// TestBuildFilterRunCommand_EnvUnsets verifies that OPENCODE_SERVER_* env vars
// are removed from the environment to prevent "Session not found" errors.
func TestBuildFilterRunCommand_EnvUnsets(t *testing.T) {
	preset := provider.ProviderPreset{
		Name:             "opencode",
		Command:          "opencode",
		Subcommand:       "run",
		Args:             nil,
		DefaultModel:     "ollama/glm-5.1:cloud",
		ModelFlag:        "--model",
		PromptPositional: true,
	}

	// Set env vars that should be unset
	t.Setenv("OPENCODE_SERVER_USERNAME", "testuser")
	t.Setenv("OPENCODE_SERVER_PASSWORD", "testpass")

	_, _, env, _, err := buildFilterRunCommand(preset, "Test prompt", "")
	if err != nil {
		t.Fatalf("buildFilterRunCommand: %v", err)
	}

	// OPENCODE_SERVER_USERNAME and OPENCODE_SERVER_PASSWORD should NOT be in env
	for _, e := range env {
		if strings.HasPrefix(e, "OPENCODE_SERVER_USERNAME=") {
			t.Errorf("env should not contain OPENCODE_SERVER_USERNAME: %s", e)
		}
		if strings.HasPrefix(e, "OPENCODE_SERVER_PASSWORD=") {
			t.Errorf("env should not contain OPENCODE_SERVER_PASSWORD: %s", e)
		}
	}
}

// --- tryParseAgentJSON tests ---

// TestTryParseAgentJSON_ValidEnvelope verifies parsing a valid JSON envelope.
func TestTryParseAgentJSON_ValidEnvelope(t *testing.T) {
	text := `{"type":"result","subtype":"success","is_error":false,"result":"hello","session_id":"abc123"}`
	envelope := tryParseAgentJSON(text)
	if envelope == nil {
		t.Fatal("expected envelope, got nil")
	}
	if envelope.SessionID != "abc123" {
		t.Errorf("session_id: got %q, want %q", envelope.SessionID, "abc123")
	}
	if envelope.Result != "hello" {
		t.Errorf("result: got %q, want %q", envelope.Result, "hello")
	}
}

// TestTryParseAgentJSON_InvalidJSON returns nil for non-JSON text.
func TestTryParseAgentJSON_InvalidJSON(t *testing.T) {
	text := "This is just plain text, not JSON."
	envelope := tryParseAgentJSON(text)
	if envelope != nil {
		t.Errorf("expected nil for non-JSON text, got: %+v", envelope)
	}
}

// --- unsetEnvPrefix tests ---

// TestUnsetEnvPrefix removes entries with the given prefix.
func TestUnsetEnvPrefix(t *testing.T) {
	env := []string{
		"PATH=/usr/bin",
		"HOME=/home/test",
		"OPENCODE_SERVER_USERNAME=old",
		"OPENCODE_SERVER_PASSWORD=secret",
		"OPENCODE_PID=123",
		"OPENCODE=1",
		"SHELL=/bin/bash",
	}
	cleaned := unsetEnvPrefix(env, "OPENCODE_SERVER_USERNAME=")
	for _, e := range cleaned {
		if e == "OPENCODE_SERVER_USERNAME=old" {
			t.Error("should have removed OPENCODE_SERVER_USERNAME")
		}
	}
	if len(cleaned) != len(env)-1 {
		t.Errorf("expected %d entries, got %d", len(env)-1, len(cleaned))
	}
}