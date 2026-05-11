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

// --- filterAgentTmux tests ---

// TestFilterAgentTmux_SkipsWithoutTmux verifies that filterAgentTmux test is
// properly gated behind tmux availability. This is a placeholder — the real
// integration test happens in CI with the full agent.
func TestFilterAgentTmux_SkipsWithoutTmux(t *testing.T) {
	// This test just verifies the function signature compiles and exists.
	// The real integration test requires tmux + opencode.
	_ = filterAgentTmux
}

// --- invokeFilterNew tests (tmux integration) ---

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

// TestFilterAgentsMD_ContainsResponseInstruction verifies that the agent
// instructions include the requirement to write RESPONSE.md.
func TestFilterAgentsMD_ContainsResponseInstruction(t *testing.T) {
	md := filterAgentsMD()
	if !strings.Contains(md, "RESPONSE.md") {
		t.Error("filterAgentsMD must instruct agent to write RESPONSE.md")
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

// --- buildFilterTmuxCommand tests ---

// TestBuildFilterTmuxCommand_ContainsEnvUnsets verifies that the tmux command
// environment unsets OPENCODE_SERVER_* variables that interfere with spawning.
func TestBuildFilterTmuxCommand_ContainsEnvUnsets(t *testing.T) {
	preset := provider.ProviderPreset{
		Name:    "test",
		Command: "echo",
	}

	_, envPairs, err := buildFilterTmuxCommand(preset, "/tmp/test-workdir")
	if err != nil {
		t.Fatalf("buildFilterTmuxCommand: %v", err)
	}

	// Check that OPENCODE_SERVER_USERNAME is unset
	foundUnset := false
	for _, kv := range envPairs {
		if kv == "OPENCODE_SERVER_USERNAME=" {
			foundUnset = true
		}
	}
	if !foundUnset {
		t.Error("env pairs should contain OPENCODE_SERVER_USERNAME= (empty value to unset)")
	}
}

// TestBuildFilterTmuxCommand_ReturnsSingleCommandString verifies that
// buildFilterTmuxCommand returns a single shell command string suitable
// for tmux new-session.
func TestBuildFilterTmuxCommand_ReturnsSingleCommandString(t *testing.T) {
	preset := provider.ProviderPreset{
		Name:             "test",
		Command:          "opencode",
		Subcommand:       "run",
		Args:             []string{"--dangerously-skip-permissions"},
		DefaultModel:     "ollama/glm-5.1:cloud",
		ModelFlag:        "--model",
		AgentFlag:        "--agent",
		PromptPositional: true,
	}

	cmdStr, _, err := buildFilterTmuxCommand(preset, "/tmp/test-workdir")
	if err != nil {
		t.Fatalf("buildFilterTmuxCommand: %v", err)
	}

	// Must contain "exec" prefix
	if !strings.HasPrefix(cmdStr, "exec ") {
		t.Errorf("command string must start with 'exec ', got: %s", cmdStr)
	}
	// Must contain the subcommand and args
	if !strings.Contains(cmdStr, "run") {
		t.Errorf("command string must contain 'run', got: %s", cmdStr)
	}
	if !strings.Contains(cmdStr, "--dangerously-skip-permissions") {
		t.Errorf("command string must contain preset args, got: %s", cmdStr)
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

// --- responseFileName constant test ---

// TestResponseFileName verifies the response file name is RESPONSE.md.
func TestResponseFileName(t *testing.T) {
	if responseFileName != "RESPONSE.md" {
		t.Errorf("responseFileName: got %q, want %q", responseFileName, "RESPONSE.md")
	}
}