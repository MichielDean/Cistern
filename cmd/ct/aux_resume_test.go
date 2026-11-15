package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MichielDean/cistern/internal/cistern"
	"github.com/MichielDean/cistern/internal/provider"
)

// fakeAgentScript writes a fake opencode binary that appends the positional
// prompt (its last argument) to a log file and replies with a numbered NDJSON
// text event. Returns the script path. The reply counter persists between
// calls, so successive rounds get distinguishable output.
func fakeAgentScript(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "prompts.log")
	countPath := filepath.Join(dir, "count")
	scriptPath := filepath.Join(dir, "opencode")
	script := fmt.Sprintf(`#!/bin/sh
prompt=""
for a in "$@"; do prompt="$a"; done
printf '%%s\n%%s' "$prompt" >> %q
n=$(cat %q 2>/dev/null || echo 0)
n=$((n+1))
echo "$n" > %q
printf '{"type":"text","part":{"text":"round %%d reply"}}\n' "$n"
`, logPath, countPath, countPath)
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return scriptPath
}

// fakeAgentPreset returns a provider preset whose command is the fake agent
// script. AgentFlag is empty so no temp project dir is created.
func fakeAgentPreset(t *testing.T) provider.ProviderPreset {
	return provider.ProviderPreset{
		Name:             "fake-opencode",
		Command:          fakeAgentScript(t),
		Subcommand:       "run",
		PromptPositional: true,
	}
}

// readPromptLog reads the recorded prompts from the fake agent's log.
func readPromptLog(t *testing.T, preset provider.ProviderPreset) string {
	t.Helper()
	logPath := filepath.Join(filepath.Dir(preset.Command), "prompts.log")
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read prompt log: %v", err)
	}
	return string(data)
}

// testCisternDB creates a temp cistern DB and points CT_DB at it. Returns the
// DB path.
func testCisternDB(t *testing.T) string {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	c, err := cistern.New(dbPath, "")
	if err != nil {
		t.Fatal(err)
	}
	c.Close()
	t.Setenv("CT_DB", dbPath)
	return dbPath
}

// --- no -s flag invariant ---

// TestBuildAgentRunCommand_NeverEmitsResumeFlag verifies the resume seam is
// gone: the constructed opencode command never carries the -s/--session
// resume flag or a continue flag. Session chaining lives in cistern's DB.
func TestBuildAgentRunCommand_NeverEmitsResumeFlag(t *testing.T) {
	preset := opencodePresetForAux()
	_, args, _, tmpDir, err := buildAgentRunCommand(preset, "filter", filterAgentsMD(), "test prompt")
	if err != nil {
		t.Fatalf("buildAgentRunCommand: %v", err)
	}
	if tmpDir != "" {
		defer os.RemoveAll(tmpDir)
	}
	resumeFlags := []string{"-s", "--session", "-c", "--continue"}
	for _, flag := range resumeFlags {
		for _, arg := range args {
			if arg == flag || strings.HasPrefix(arg, "--session=") {
				t.Errorf("args must not contain resume flag %q, got: %v", flag, args)
			}
		}
	}
}

// --- buildResumePrompt ordering ---

// TestBuildResumePrompt_PriorConversationPrecedesNewMessage verifies prompt
// ordering: the prior transcript block comes first, and the new message
// follows it so the agent sees chronology.
func TestBuildResumePrompt_PriorConversationPrecedesNewMessage(t *testing.T) {
	messages := []cistern.FilterMessage{
		{Role: "user", Content: "the spec keyword is KUMQUAT"},
		{Role: "assistant", Content: "noted the kumquat"},
	}
	got := buildResumePrompt(messages, "what was the keyword?")

	idxPrior := strings.Index(got, "PRIOR CONVERSATION")
	idxOld := strings.Index(got, "KUMQUAT")
	idxEnd := strings.Index(got, "END PRIOR CONVERSATION")
	idxNew := strings.Index(got, "what was the keyword?")

	if idxPrior < 0 || idxOld < 0 || idxEnd < 0 || idxNew < 0 {
		t.Fatalf("resume prompt missing expected sections:\n%s", got)
	}
	if !(idxPrior < idxOld && idxOld < idxEnd && idxEnd < idxNew) {
		t.Errorf("prompt not ordered prior-transcript-then-new-message:\n%s", got)
	}
	if !strings.Contains(got, "USER:") || !strings.Contains(got, "ASSISTANT:") {
		t.Errorf("transcript must label roles, got:\n%s", got)
	}
}

// TestBuildResumePrompt_EmptyMessagesReturnsNewMessage verifies the new-round
// path: no transcript block is injected when there are no prior messages.
func TestBuildResumePrompt_EmptyMessagesReturnsNewMessage(t *testing.T) {
	got := buildResumePrompt(nil, "fresh prompt")
	if got != "fresh prompt" {
		t.Errorf("buildResumePrompt(empty) = %q, want %q", got, "fresh prompt")
	}
}

// --- CLI continuity: round-1 + two resumes ---

// TestRunAuxAgentRound_CLIContinuity verifies the droplet's acceptance shape:
// a round-1 call followed by two --resume calls succeed back-to-back, each
// resume receives the prior transcript in its prompt (transcript continuity),
// and the persisted transcript grows to include every round.
func TestRunAuxAgentRound_CLIContinuity(t *testing.T) {
	dbPath := testCisternDB(t)
	preset := fakeAgentPreset(t)

	// Round 1: new session.
	r1, err := runAuxAgentRound(preset, "filter", filterAgentsMD(), "", "", "", "the spec keyword is KUMQUAT", "")
	if err != nil {
		t.Fatalf("round 1: %v", err)
	}
	if r1.CisternID == "" {
		t.Fatal("round 1 must return the cistern session ID")
	}

	// Resume 1: the prompt must reference the prior round's content from the
	// DB transcript (not any opencode server session).
	r2, err := runAuxAgentRound(preset, "filter", filterAgentsMD(), "", "", "", "what was the keyword?", r1.CisternID)
	if err != nil {
		t.Fatalf("resume 1: %v", err)
	}

	// Resume 2: cumulative transcript — prior+prior+new.
	r3, err := runAuxAgentRound(preset, "filter", filterAgentsMD(), "", "", "", "and the description?", r1.CisternID)
	if err != nil {
		t.Fatalf("resume 2: %v", err)
	}
	if r3.CisternID != r1.CisternID {
		t.Errorf("resumes must chain to the same cistern row, got %q then %q", r2.CisternID, r3.CisternID)
	}

	// The last logged prompt must carry the full prior transcript: both the
	// round-1 keyword and resume-1's message, in order, before the new text.
	log := readPromptLog(t, preset)
	idxKeyword := strings.LastIndex(log, "KUMQUAT")
	idxResume1 := strings.LastIndex(log, "what was the keyword?")
	idxNew := strings.LastIndex(log, "and the description?")
	if idxKeyword < 0 || idxResume1 < 0 || idxNew < 0 {
		t.Fatalf("resume prompt missing continuity markers:\n%s", log)
	}
	if !(idxKeyword < idxResume1 && idxResume1 < idxNew) {
		t.Errorf("prior transcript not ordered before new message:\n%s", log)
	}

	// Persisted transcript must hold all six messages across three rounds.
	c, err := cistern.New(dbPath, "")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	session, err := c.GetFilterSession(r1.CisternID)
	if err != nil {
		t.Fatal(err)
	}
	var messages []cistern.FilterMessage
	if err := json.Unmarshal([]byte(session.Messages), &messages); err != nil {
		t.Fatalf("unmarshal transcript: %v", err)
	}
	if len(messages) != 6 {
		t.Fatalf("transcript length = %d, want 6 (3 rounds x 2 messages): %s", len(messages), session.Messages)
	}
	for _, want := range []string{"the spec keyword is KUMQUAT", "what was the keyword?", "and the description?", "round 3 reply"} {
		found := false
		for _, m := range messages {
			found = found || strings.Contains(m.Content, want)
		}
		if !found {
			t.Errorf("transcript missing %q", want)
		}
	}
}

// TestRunAuxAgentRound_ResumeMissingSessionErrors verifies a resume against a
// nonexistent cistern session names the missing ID instead of failing opencode
// with "Agent not found".
func TestRunAuxAgentRound_ResumeMissingSessionErrors(t *testing.T) {
	testCisternDB(t)
	preset := fakeAgentPreset(t)
	_, err := runAuxAgentRound(preset, "filter", filterAgentsMD(), "", "", "", "hello", "nosuchsession")
	if err == nil {
		t.Fatal("expected error for missing session")
	}
	if !strings.Contains(err.Error(), "nosuchsession") || !strings.Contains(err.Error(), "not found") {
		t.Errorf("error must name the missing session ID, got: %v", err)
	}
}

// --- explicit title threading (reviewer issue ci-7utyd-mvsb1) ---

// TestRunAuxAgentRound_PersistsExplicitTitle verifies that a new round
// persists the caller-supplied title and description on the filter_sessions
// row. Production prompts start with a context block, never a 'Title: '
// header, so parsing the assembled prompt must never be the source of the
// row title again.
func TestRunAuxAgentRound_PersistsExplicitTitle(t *testing.T) {
	dbPath := testCisternDB(t)
	preset := fakeAgentPreset(t)

	r, err := runAuxAgentRound(preset, "filter", filterAgentsMD(), "CONTEXT FIRST", "Rework ingest", "the ingest stage drops errors", "Refactor the ingest loop", "")
	if err != nil {
		t.Fatalf("runAuxAgentRound: %v", err)
	}

	c, err := cistern.New(dbPath, "")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	session, err := c.GetFilterSession(r.CisternID)
	if err != nil {
		t.Fatal(err)
	}
	if session.Title != "Rework ingest" {
		t.Errorf("session.Title = %q, want %q (title must come from the explicit param, not prompt parsing)", session.Title, "Rework ingest")
	}
	if session.Description != "the ingest stage drops errors" {
		t.Errorf("session.Description = %q, want %q", session.Description, "the ingest stage drops errors")
	}

	// Sanity: the assembled prompt still begins with the context block.
	log := readPromptLog(t, preset)
	if !strings.HasPrefix(strings.TrimSpace(log), "CONTEXT FIRST") {
		t.Errorf("prompt must start with the context block:\n%s", log)
	}
}

// --- agent-ownership check on resume (reviewer issue ci-7utyd-p6bvq) ---

// TestRunAuxAgentRound_ResumeRejectsForeignAgent verifies that resuming a
// session with the wrong agent is rejected: ct filter --resume
// <triage-session-id> must fail instead of replaying a triage transcript
// through the filter agent.
func TestRunAuxAgentRound_ResumeRejectsForeignAgent(t *testing.T) {
	dbPath := testCisternDB(t)
	preset := fakeAgentPreset(t)

	// Seed a triage-owned session directly.
	c, err := cistern.New(dbPath, "")
	if err != nil {
		t.Fatal(err)
	}
	triageSession, err := c.CreateAgentSession("triage", "Triage idea", "")
	c.Close()
	if err != nil {
		t.Fatal(err)
	}

	_, err = runAuxAgentRound(preset, "filter", filterAgentsMD(), "", "", "", "resume this", triageSession.ID)
	if err == nil {
		t.Fatal("expected error when resuming a triage session with the filter agent")
	}
	if !strings.Contains(err.Error(), "triage") {
		t.Errorf("error must name the owning agent %q, got: %v", "triage", err)
	}

	// The matching agent still resumes fine.
	if _, err := runAuxAgentRound(preset, "triage", "md", "", "", "", "resume this", triageSession.ID); err != nil {
		t.Errorf("triage resume of triage session must succeed, got: %v", err)
	}
}

// --- triage/wizard persistence ---

// TestRunAuxAgentRound_TriageWizardPersistence verifies triage and wizard
// rounds persist their transcript with the agent discriminator set, and that
// those rows are hidden from the filter-visible list.
func TestRunAuxAgentRound_TriageWizardPersistence(t *testing.T) {
	dbPath := testCisternDB(t)

	for _, agent := range []string{"triage", "wizard"} {
		preset := fakeAgentPreset(t) // fresh fake per agent so the reply counter is per-agent
		r, err := runAuxAgentRound(preset, agent, "md", "", agent+" idea", "", "persist me", "")
		if err != nil {
			t.Fatalf("%s: %v", agent, err)
		}
		c, err := cistern.New(dbPath, "")
		if err != nil {
			t.Fatal(err)
		}
		session, err := c.GetFilterSession(r.CisternID)
		c.Close()
		if err != nil {
			t.Fatalf("%s: %v", agent, err)
		}
		if session.Agent != agent {
			t.Errorf("agent = %q, want %q", session.Agent, agent)
		}
		var messages []cistern.FilterMessage
		if err := json.Unmarshal([]byte(session.Messages), &messages); err != nil {
			t.Fatalf("%s: unmarshal transcript: %v", agent, err)
		}
		if len(messages) != 2 || !strings.Contains(messages[1].Content, "round 1 reply") {
			t.Errorf("%s: transcript not persisted correctly: %s", agent, session.Messages)
		}
	}

	// Neither row may leak into the filter session list.
	c, err := cistern.New(dbPath, "")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	list, err := c.ListFilterSessions()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 0 {
		t.Errorf("triage/wizard sessions hidden from filter list, got %d rows", len(list))
	}
}

// --- dashboard resume handler ---

// TestDashboardFilterResume_UsesRunAuxAgentRound verifies the dashboard
// resume endpoint replays the persisted transcript through a fresh session
// (no -s resume) and appends the new exchange to the stored transcript.
func TestDashboardFilterResume_UsesRunAuxAgentRound(t *testing.T) {
	dbPath := testCisternDB(t)
	fake := fakeAgentPreset(t)

	// Intercept PATH so resolveFilterPreset's "opencode" command resolves to
	// the fake agent; the dashboard handler resolves its own preset.
	path := filepath.Dir(fake.Command) + string(os.PathListSeparator) + os.Getenv("PATH")
	t.Setenv("PATH", path)

	// Seed a session with one exchange.
	r1, err := runAuxAgentRound(fake, "filter", filterAgentsMD(), "", "", "", "first: ORANGE", "")
	if err != nil {
		t.Fatal(err)
	}

	cfgPath := tempCfg(t)
	mux := newDashboardMux(cfgPath, dbPath)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	body, _ := json.Marshal(map[string]string{"message": "second: BANANA"})
	resp, err := http.Post(srv.URL+"/api/filter/"+r1.CisternID+"/resume", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("resume status = %d, want 200", resp.StatusCode)
	}
	var out struct {
		SessionID        string `json:"session_id"`
		AssistantMessage string `json:"assistant_message"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out.SessionID != r1.CisternID {
		t.Errorf("session_id = %q, want %q", out.SessionID, r1.CisternID)
	}
	if !strings.Contains(out.AssistantMessage, "round 2 reply") {
		t.Errorf("assistant_message = %q, want round-2 reply", out.AssistantMessage)
	}

	// Transcript holds both exchanges; the resume prompt carried the prior one.
	c, err := cistern.New(dbPath, "")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	session, err := c.GetFilterSession(r1.CisternID)
	if err != nil {
		t.Fatal(err)
	}
	var messages []cistern.FilterMessage
	if err := json.Unmarshal([]byte(session.Messages), &messages); err != nil {
		t.Fatal(err)
	}
	if len(messages) != 4 {
		t.Errorf("transcript length = %d, want 4: %s", len(messages), session.Messages)
	}
}

// --- TUI resume prompt ---

// TestFilterPanel_ResumePromptFromHistory_UsesBuildResumePrompt verifies the
// TUI builds its follow-up prompt from the in-memory transcript via
// buildResumePrompt — a fresh session prompt, never a server-side resume key.
func TestFilterPanel_ResumePromptFromHistory_UsesBuildResumePrompt(t *testing.T) {
	p := filterPanel{
		history: []filterConvEntry{
			{role: "user", text: "spec keyword is MELON"},
			{role: "assistant", text: "melon noted"},
		},
		sessionID: "cistern-or-opencode-id",
	}
	got := p.resumePromptFromHistory("next question")
	if !strings.Contains(got, "PRIOR CONVERSATION") {
		t.Errorf("resume prompt must use buildResumePrompt transcript block:\n%s", got)
	}
	idxPrior := strings.Index(got, "MELON")
	idxNew := strings.Index(got, "next question")
	if idxPrior < 0 || idxNew < 0 || idxPrior > idxNew {
		t.Errorf("transcript must precede new message:\n%s", got)
	}
}
