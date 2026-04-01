package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MichielDean/cistern/internal/provider"
)

// --- callFilterAgent tests ---

// TestCallFilterAgent_ReturnsTextAndSessionID verifies that callFilterAgent
// correctly invokes the agent with --output-format json and returns both the
// text response and the session_id from the JSON envelope.
// Given a preset pointing at fakeagent,
// When callFilterAgent is called with nil extraArgs,
// Then a non-empty text and non-empty session_id are returned.
func TestCallFilterAgent_ReturnsTextAndSessionID(t *testing.T) {
	fakeagentBin := buildTestBin(t, "fakeagent", "github.com/MichielDean/cistern/internal/testutil/fakeagent")

	preset := provider.ProviderPreset{
		Name:    "test",
		Command: fakeagentBin,
		NonInteractive: provider.NonInteractiveConfig{
			PrintFlag:  "--print",
			PromptFlag: "-p",
		},
	}

	result, err := callFilterAgent(preset, nil, "Title: fix auth bug", "")
	if err != nil {
		t.Fatalf("callFilterAgent: unexpected error: %v", err)
	}
	if result.SessionID == "" {
		t.Error("expected non-empty session_id")
	}
	if result.Text == "" {
		t.Error("expected non-empty text response")
	}
}

// TestCallFilterAgent_Resume_PassesExtraArgs verifies that --resume extraArgs are
// forwarded to the agent binary and fakeagent handles them gracefully.
// Given a preset and extraArgs containing --resume,
// When callFilterAgent is called,
// Then a non-empty text and session_id are returned.
func TestCallFilterAgent_Resume_PassesExtraArgs(t *testing.T) {
	fakeagentBin := buildTestBin(t, "fakeagent", "github.com/MichielDean/cistern/internal/testutil/fakeagent")

	preset := provider.ProviderPreset{
		Name:    "test",
		Command: fakeagentBin,
		NonInteractive: provider.NonInteractiveConfig{
			PrintFlag:  "--print",
			PromptFlag: "-p",
		},
	}

	result, err := callFilterAgent(preset, []string{"--resume", "test-session-id"}, "refine further", "")
	if err != nil {
		t.Fatalf("callFilterAgent with --resume: unexpected error: %v", err)
	}
	if result.SessionID == "" {
		t.Error("expected non-empty session_id")
	}
	if result.Text == "" {
		t.Error("expected non-empty text response")
	}
}

// TestCallFilterAgent_AgentExecFailure verifies that when the agent exits non-zero,
// callFilterAgent returns an error containing the agent's stderr output.
// Given a preset pointing at failagent (exits 1),
// When callFilterAgent is called,
// Then an error is returned.
func TestCallFilterAgent_AgentExecFailure(t *testing.T) {
	failagentBin := buildTestBin(t, "failagent", "github.com/MichielDean/cistern/internal/testutil/failagent")

	preset := provider.ProviderPreset{
		Name:    "test-fail",
		Command: failagentBin,
		NonInteractive: provider.NonInteractiveConfig{
			PrintFlag:  "--print",
			PromptFlag: "-p",
		},
	}

	_, err := callFilterAgent(preset, nil, "some prompt", "")
	if err == nil {
		t.Fatal("expected error when agent exits non-zero, got nil")
	}
}

// TestCallFilterAgent_JSONFallback_RawOutput verifies the fallback path where the
// agent exits 0 but returns non-JSON-envelope output.
// callFilterAgent must return the raw output as text with an empty session_id.
// Given a fakeagent in raw_fallback mode (returns raw text despite --output-format),
// When callFilterAgent is called,
// Then non-empty text is returned and session_id is empty.
func TestCallFilterAgent_JSONFallback_RawOutput(t *testing.T) {
	fakeagentBin := buildTestBin(t, "fakeagent", "github.com/MichielDean/cistern/internal/testutil/fakeagent")
	t.Setenv("FAKEAGENT_MODE", "raw_fallback")

	preset := provider.ProviderPreset{
		Name:    "test",
		Command: fakeagentBin,
		NonInteractive: provider.NonInteractiveConfig{
			PrintFlag:  "--print",
			PromptFlag: "-p",
		},
	}

	result, err := callFilterAgent(preset, nil, "Title: fix auth bug", "")
	if err != nil {
		t.Fatalf("callFilterAgent fallback: unexpected error: %v", err)
	}
	if result.SessionID != "" {
		t.Errorf("expected empty session_id in fallback mode, got %q", result.SessionID)
	}
	if result.Text == "" {
		t.Error("expected non-empty text from fallback path")
	}
}

// TestCallFilterAgent_IsErrorEnvelope_ReturnsError verifies that when the agent
// returns a JSON envelope with is_error:true, callFilterAgent returns an error.
// Given a fakeagent in error_envelope mode (returns is_error:true JSON),
// When callFilterAgent is called,
// Then an error mentioning "agent returned error" is returned.
func TestCallFilterAgent_IsErrorEnvelope_ReturnsError(t *testing.T) {
	fakeagentBin := buildTestBin(t, "fakeagent", "github.com/MichielDean/cistern/internal/testutil/fakeagent")
	t.Setenv("FAKEAGENT_MODE", "error_envelope")

	preset := provider.ProviderPreset{
		Name:    "test",
		Command: fakeagentBin,
		NonInteractive: provider.NonInteractiveConfig{
			PrintFlag:  "--print",
			PromptFlag: "-p",
		},
	}

	_, err := callFilterAgent(preset, nil, "Title: fix auth bug", "")
	if err == nil {
		t.Fatal("expected error when agent returns is_error envelope, got nil")
	}
	if !strings.Contains(err.Error(), "agent returned error") {
		t.Errorf("error %q does not mention 'agent returned error'", err.Error())
	}
}

// TestCallFilterAgent_MissingRequiredEnvVar verifies that callFilterAgent returns
// an error without executing the agent when a required env var is absent.
// Given a preset with EnvPassthrough=["MISSING_FILTER_KEY"] and the var unset,
// When callFilterAgent is called,
// Then an error mentioning the key is returned.
func TestCallFilterAgent_MissingRequiredEnvVar(t *testing.T) {
	preset := provider.ProviderPreset{
		Name:           "test",
		Command:        "true",
		EnvPassthrough: []string{"MISSING_FILTER_KEY"},
		NonInteractive: provider.NonInteractiveConfig{PromptFlag: "-p"},
	}
	t.Setenv("MISSING_FILTER_KEY", "")

	_, err := callFilterAgent(preset, nil, "prompt", "")
	if err == nil {
		t.Fatal("expected error for missing env var, got nil")
	}
	if !strings.Contains(err.Error(), "MISSING_FILTER_KEY") {
		t.Errorf("error %q does not mention the missing key", err.Error())
	}
}

// --- invokeFilterNew tests ---

// TestInvokeFilterNew_ReturnsTextAndSessionID verifies that invokeFilterNew
// combines system + user prompts and returns a text response with a session_id.
// Given a preset pointing at fakeagent,
// When invokeFilterNew is called with a title and description,
// Then non-empty text and a non-empty session_id are returned.
func TestInvokeFilterNew_ReturnsTextAndSessionID(t *testing.T) {
	fakeagentBin := buildTestBin(t, "fakeagent", "github.com/MichielDean/cistern/internal/testutil/fakeagent")

	preset := provider.ProviderPreset{
		Name:    "test",
		Command: fakeagentBin,
		NonInteractive: provider.NonInteractiveConfig{
			PrintFlag:  "--print",
			PromptFlag: "-p",
		},
	}

	result, err := invokeFilterNew(preset, "Add user auth", "JWT-based auth with refresh tokens", "", "")
	if err != nil {
		t.Fatalf("invokeFilterNew: unexpected error: %v", err)
	}
	if result.SessionID == "" {
		t.Error("expected non-empty session_id from invokeFilterNew")
	}
	if result.Text == "" {
		t.Error("expected non-empty text response from invokeFilterNew")
	}
}

// TestInvokeFilterNew_TitleOnly verifies that invokeFilterNew works when only
// a title is provided (empty description).
func TestInvokeFilterNew_TitleOnly(t *testing.T) {
	fakeagentBin := buildTestBin(t, "fakeagent", "github.com/MichielDean/cistern/internal/testutil/fakeagent")

	preset := provider.ProviderPreset{
		Name:    "test",
		Command: fakeagentBin,
		NonInteractive: provider.NonInteractiveConfig{
			PrintFlag:  "--print",
			PromptFlag: "-p",
		},
	}

	result, err := invokeFilterNew(preset, "Add user auth", "", "", "")
	if err != nil {
		t.Fatalf("invokeFilterNew title-only: unexpected error: %v", err)
	}
	if result.Text == "" {
		t.Error("expected non-empty text response")
	}
}

// --- invokeFilterResume tests ---

// TestInvokeFilterResume_WithFeedback verifies that invokeFilterResume passes
// the session ID via the preset's ResumeFlag and returns a text response.
// Given a preset with ResumeFlag="--resume" pointing at fakeagent,
// When invokeFilterResume is called with a session ID and feedback,
// Then non-empty text and a non-empty session_id are returned.
func TestInvokeFilterResume_WithFeedback(t *testing.T) {
	fakeagentBin := buildTestBin(t, "fakeagent", "github.com/MichielDean/cistern/internal/testutil/fakeagent")

	preset := provider.ProviderPreset{
		Name:       "test",
		Command:    fakeagentBin,
		ResumeFlag: "--resume",
		NonInteractive: provider.NonInteractiveConfig{
			PrintFlag:  "--print",
			PromptFlag: "-p",
		},
	}

	result, err := invokeFilterResume(preset, "existing-session-123", "Make it more focused")
	if err != nil {
		t.Fatalf("invokeFilterResume: unexpected error: %v", err)
	}
	if result.SessionID == "" {
		t.Error("expected non-empty session_id from invokeFilterResume")
	}
	if result.Text == "" {
		t.Error("expected non-empty text response")
	}
}

// TestInvokeFilterResume_DefaultsToResumeFlag verifies that when ResumeFlag is
// empty in the preset, invokeFilterResume defaults to "--resume".
func TestInvokeFilterResume_DefaultsToResumeFlag(t *testing.T) {
	fakeagentBin := buildTestBin(t, "fakeagent", "github.com/MichielDean/cistern/internal/testutil/fakeagent")

	// No ResumeFlag set — should default to "--resume".
	preset := provider.ProviderPreset{
		Name:    "test",
		Command: fakeagentBin,
		NonInteractive: provider.NonInteractiveConfig{
			PrintFlag:  "--print",
			PromptFlag: "-p",
		},
	}

	result, err := invokeFilterResume(preset, "session-456", "more feedback")
	if err != nil {
		t.Fatalf("invokeFilterResume (default flag): unexpected error: %v", err)
	}
	if result.Text == "" {
		t.Error("expected non-empty text response")
	}
}

// --- ct filter command flag validation tests ---

// TestFilterCmd_NewSession_RequiresTitle verifies that ct filter without --title
// and without --resume returns an error mentioning --title.
func TestFilterCmd_NewSession_RequiresTitle(t *testing.T) {
	t.Setenv("CT_NO_ASCII_LOGO", "1")
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
// Given ct filter --resume <id> with no positional arg,
// When the command is executed,
// Then an error mentioning "feedback" is returned.
func TestFilterCmd_Resume_RequiresFeedback(t *testing.T) {
	t.Setenv("CT_NO_ASCII_LOGO", "1")
	err := execCmd(t, "filter", "--resume", "some-session-id")
	if err == nil {
		t.Fatal("expected error when --resume without feedback, got nil")
	}
	if !strings.Contains(err.Error(), "feedback") {
		t.Errorf("error %q does not mention feedback", err.Error())
	}
}

// --- printFilterResult tests ---

// TestPrintFilterResult_HumanFormat verifies that printFilterResult with "human"
// format does not return an error for a valid result.
func TestPrintFilterResult_HumanFormat(t *testing.T) {
	result := filterSessionResult{
		SessionID: "test-session",
		Text:      "1. Fix login bug\n   Handle edge case in auth flow. No dependencies.",
	}
	if err := printFilterResult(result, "human"); err != nil {
		t.Fatalf("printFilterResult human: unexpected error: %v", err)
	}
}

// TestPrintFilterResult_JSONFormat verifies that printFilterResult with "json"
// format does not return an error for a valid result.
func TestPrintFilterResult_JSONFormat(t *testing.T) {
	result := filterSessionResult{
		SessionID: "session-xyz",
		Text:      "1. Add caching\n   Implement Redis caching. No dependencies.",
	}
	if err := printFilterResult(result, "json"); err != nil {
		t.Fatalf("printFilterResult json: unexpected error: %v", err)
	}
}

// TestFilterJSONOutput_HasRequiredFields verifies that the JSON output structure
// contains session_id and text — the fields required for scripting.
func TestFilterJSONOutput_HasRequiredFields(t *testing.T) {
	type filterJSONOutput struct {
		SessionID string `json:"session_id"`
		Text      string `json:"text,omitempty"`
	}
	out := filterJSONOutput{
		SessionID: "abc123",
		Text:      "1. Refactor auth\n   Clean up the auth module. No dependencies.",
	}
	data, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got map[string]interface{}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, field := range []string{"session_id", "text"} {
		if v, ok := got[field]; !ok || v == "" {
			t.Errorf("JSON output missing required field %q", field)
		}
	}
}

// TestFilterCmd_SkipContextFlag_IsRejected verifies that --skip-context is no
// longer a recognized flag and produces an "unknown flag" error.
// Given any ct filter invocation with --skip-context,
// When the command is executed,
// Then an error containing "unknown flag: --skip-context" is returned.
func TestFilterCmd_SkipContextFlag_IsRejected(t *testing.T) {
	t.Setenv("CT_NO_ASCII_LOGO", "1")
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

// TestFilterCmd_FileFlag_IsRejected verifies that --file is no longer a
// recognized flag and produces an "unknown flag" error.
// Given any ct filter invocation with --file,
// When the command is executed,
// Then an error containing "unknown flag: --file" is returned.
func TestFilterCmd_FileFlag_IsRejected(t *testing.T) {
	t.Setenv("CT_NO_ASCII_LOGO", "1")
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

// TestFilterCmd_RepoFlag_IsRejected verifies that --repo is no longer a
// recognized flag and produces an "unknown flag" error.
// Given any ct filter invocation with --repo,
// When the command is executed,
// Then an error containing "unknown flag: --repo" is returned.
func TestFilterCmd_RepoFlag_IsRejected(t *testing.T) {
	t.Setenv("CT_NO_ASCII_LOGO", "1")
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

// TestFilterCmd_PromptAlwaysHasContextHeader verifies that context is always
// injected into the prompt — there is no flag to bypass it.
// Given a config that routes the filter preset to fakeagent with FAKEAGENT_PROMPT_FILE set,
// When ct filter --title "..." is called,
// Then the captured prompt must contain the codebase context header.
func TestFilterCmd_PromptAlwaysHasContextHeader(t *testing.T) {
	fakeagentBin := buildTestBin(t, "fakeagent", "github.com/MichielDean/cistern/internal/testutil/fakeagent")
	dir := t.TempDir()

	cfgPath := filepath.Join(dir, "cistern.yaml")
	cfgContent := fmt.Sprintf("provider:\n  command: %s\nrepos:\n  - name: testRepo\n    cataractae: 1\n", fakeagentBin)
	if err := os.WriteFile(cfgPath, []byte(cfgContent), 0o644); err != nil {
		t.Fatalf("WriteFile config: %v", err)
	}

	promptFile := filepath.Join(dir, "prompt.txt")
	t.Setenv("CT_CONFIG", cfgPath)
	t.Setenv("CT_DB", filepath.Join(dir, "test.db"))
	t.Setenv("CT_NO_ASCII_LOGO", "1")
	t.Setenv("FAKEAGENT_PROMPT_FILE", promptFile)
	// Reset globals that may be polluted by prior tests.
	filterTitle = ""
	filterResume = ""
	t.Cleanup(func() {
		filterTitle = ""
		filterResume = ""
	})

	if err := execCmd(t, "filter", "--title", "test idea"); err != nil {
		t.Fatalf("filter: unexpected error: %v", err)
	}

	captured, err := os.ReadFile(promptFile)
	if err != nil {
		t.Fatalf("reading captured prompt: %v", err)
	}
	if !strings.Contains(string(captured), "=== CODEBASE CONTEXT ===") {
		t.Errorf("prompt must always contain context header, got:\n%s", captured)
	}
}

// TestInvokeFilterNew_WithContextBlock_IncludesContextInResult verifies that
// invokeFilterNew accepts a non-empty contextBlock without error.
// Given a preset pointing at fakeagent and a non-empty contextBlock,
// When invokeFilterNew is called,
// Then non-empty text and a session_id are returned.
func TestInvokeFilterNew_WithContextBlock_IncludesContextInResult(t *testing.T) {
	fakeagentBin := buildTestBin(t, "fakeagent", "github.com/MichielDean/cistern/internal/testutil/fakeagent")

	preset := provider.ProviderPreset{
		Name:    "test",
		Command: fakeagentBin,
		NonInteractive: provider.NonInteractiveConfig{
			PrintFlag:  "--print",
			PromptFlag: "-p",
		},
	}

	contextBlock := "=== CODEBASE CONTEXT ===\nsome schema here\n=== END CODEBASE CONTEXT ==="
	result, err := invokeFilterNew(preset, "Add feature", "Some description", contextBlock, "")
	if err != nil {
		t.Fatalf("invokeFilterNew with contextBlock: unexpected error: %v", err)
	}
	if result.SessionID == "" {
		t.Error("expected non-empty session_id")
	}
	if result.Text == "" {
		t.Error("expected non-empty text response")
	}
}

// TestCallFilterAgent_WithAllowedTools_PassesToolFlag verifies that when the
// preset's NonInteractive.AllowedToolsFlag is set, callFilterAgent appends
// the flag with "Glob,Grep,Read" to the agent command args.
// Given a preset with AllowedToolsFlag: "--allowedTools" pointing at fakeagent,
// When callFilterAgent is called,
// Then fakeagent receives --allowedTools and Glob,Grep,Read in its args.
func TestCallFilterAgent_WithAllowedTools_PassesToolFlag(t *testing.T) {
	fakeagentBin := buildTestBin(t, "fakeagent", "github.com/MichielDean/cistern/internal/testutil/fakeagent")
	dir := t.TempDir()
	argsFile := filepath.Join(dir, "args.txt")
	t.Setenv("FAKEAGENT_ARGS_FILE", argsFile)

	preset := provider.ProviderPreset{
		Name:    "test",
		Command: fakeagentBin,
		NonInteractive: provider.NonInteractiveConfig{
			PrintFlag:        "--print",
			PromptFlag:       "-p",
			AllowedToolsFlag: "--allowedTools",
		},
	}

	_, err := callFilterAgent(preset, nil, "Title: fix auth bug", "")
	if err != nil {
		t.Fatalf("callFilterAgent: unexpected error: %v", err)
	}

	captured, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("reading args file: %v", err)
	}
	args := string(captured)
	if !strings.Contains(args, "--allowedTools") {
		t.Errorf("expected --allowedTools in agent args, got:\n%s", args)
	}
	if !strings.Contains(args, "Glob,Grep,Read") {
		t.Errorf("expected Glob,Grep,Read in agent args, got:\n%s", args)
	}
}

// TestCallFilterAgent_WithAddDir_PassesDirFlag verifies that when repoPath is
// non-empty and the preset defines AddDirFlag, --add-dir <repoPath> is passed
// to the agent.
// Given a preset with AddDirFlag: "--add-dir" and a non-empty repoPath,
// When callFilterAgent is called,
// Then fakeagent receives --add-dir <repoPath> in its args.
func TestCallFilterAgent_WithAddDir_PassesDirFlag(t *testing.T) {
	fakeagentBin := buildTestBin(t, "fakeagent", "github.com/MichielDean/cistern/internal/testutil/fakeagent")
	dir := t.TempDir()
	argsFile := filepath.Join(dir, "args.txt")
	t.Setenv("FAKEAGENT_ARGS_FILE", argsFile)

	preset := provider.ProviderPreset{
		Name:       "test",
		Command:    fakeagentBin,
		AddDirFlag: "--add-dir",
		NonInteractive: provider.NonInteractiveConfig{
			PrintFlag:  "--print",
			PromptFlag: "-p",
		},
	}

	repoPath := t.TempDir()
	_, err := callFilterAgent(preset, nil, "Title: fix auth bug", repoPath)
	if err != nil {
		t.Fatalf("callFilterAgent with repoPath: unexpected error: %v", err)
	}

	captured, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("reading args file: %v", err)
	}
	args := string(captured)
	if !strings.Contains(args, "--add-dir") {
		t.Errorf("expected --add-dir in agent args, got:\n%s", args)
	}
	if !strings.Contains(args, repoPath) {
		t.Errorf("expected repoPath %q in agent args, got:\n%s", repoPath, args)
	}
}

// TestCallFilterAgent_WithEmptyRepoPath_SkipsAddDirFlag verifies that when
// repoPath is empty, --add-dir is NOT passed even if AddDirFlag is set.
// Given a preset with AddDirFlag and an empty repoPath,
// When callFilterAgent is called with repoPath="",
// Then fakeagent does not receive --add-dir in its args.
func TestCallFilterAgent_WithEmptyRepoPath_SkipsAddDirFlag(t *testing.T) {
	fakeagentBin := buildTestBin(t, "fakeagent", "github.com/MichielDean/cistern/internal/testutil/fakeagent")
	dir := t.TempDir()
	argsFile := filepath.Join(dir, "args.txt")
	t.Setenv("FAKEAGENT_ARGS_FILE", argsFile)

	preset := provider.ProviderPreset{
		Name:       "test",
		Command:    fakeagentBin,
		AddDirFlag: "--add-dir",
		NonInteractive: provider.NonInteractiveConfig{
			PrintFlag:  "--print",
			PromptFlag: "-p",
		},
	}

	_, err := callFilterAgent(preset, nil, "Title: fix auth bug", "")
	if err != nil {
		t.Fatalf("callFilterAgent with empty repoPath: unexpected error: %v", err)
	}

	captured, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("reading args file: %v", err)
	}
	args := string(captured)
	if strings.Contains(args, "--add-dir") {
		t.Errorf("expected no --add-dir when repoPath is empty, got:\n%s", args)
	}
}
