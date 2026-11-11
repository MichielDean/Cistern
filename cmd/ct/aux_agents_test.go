package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MichielDean/cistern/internal/cistern"
	"github.com/MichielDean/cistern/internal/provider"
)

func execCmdAux(t *testing.T, args ...string) error {
	t.Helper()
	rootCmd.SetArgs(args)
	return rootCmd.Execute()
}

func opencodePresetForAux() provider.ProviderPreset {
	return provider.ProviderPreset{
		Name:             "opencode",
		Command:          "opencode",
		Subcommand:       "run",
		Args:             []string{"--dangerously-skip-permissions"},
		DefaultModel:     "ollama/glm-5.1:cloud",
		ModelFlag:        "--model",
		AgentFlag:        "--agent",
		PromptPositional: true,
	}
}

// --- ct triage flag validation ---

// TestTriageCmd_RequiresDropletIDOrTitle verifies that ct triage without a
// droplet id and without --title returns a usage error.
func TestTriageCmd_RequiresDropletIDOrTitle(t *testing.T) {
	t.Cleanup(func() {
		triageTitle = ""
		triageResume = ""
		triageDescription = ""
	})
	err := execCmdAux(t, "triage")
	if err == nil {
		t.Fatal("expected error when no droplet id or --title given, got nil")
	}
	if !strings.Contains(err.Error(), "--title") || !strings.Contains(err.Error(), "droplet") {
		t.Errorf("error %q should mention droplet and --title", err.Error())
	}
}

// TestTriageCmd_Resume_RequiresFeedback verifies that ct triage --resume
// without a feedback argument returns an error mentioning "feedback".
func TestTriageCmd_Resume_RequiresFeedback(t *testing.T) {
	t.Cleanup(func() {
		triageTitle = ""
		triageResume = ""
		triageDescription = ""
	})
	err := execCmdAux(t, "triage", "--resume", "some-session")
	if err == nil {
		t.Fatal("expected error for --resume without feedback, got nil")
	}
	if !strings.Contains(err.Error(), "feedback") {
		t.Errorf("error %q does not mention feedback", err.Error())
	}
}

// --- ct wizard flag validation ---

// TestWizardCmd_TitleRequired verifies that ct wizard without --title (and
// without --resume) returns an error mentioning --title.
func TestWizardCmd_TitleRequired(t *testing.T) {
	t.Cleanup(func() {
		wizardTitle = ""
		wizardResume = ""
		wizardDescription = ""
	})
	err := execCmdAux(t, "wizard")
	if err == nil {
		t.Fatal("expected error when --title is missing, got nil")
	}
	if !strings.Contains(err.Error(), "--title") {
		t.Errorf("error %q does not mention --title", err.Error())
	}
}

// TestWizardCmd_Resume_RequiresFeedback verifies that ct wizard --resume
// without a feedback argument returns an error mentioning "feedback".
func TestWizardCmd_Resume_RequiresFeedback(t *testing.T) {
	t.Cleanup(func() {
		wizardTitle = ""
		wizardResume = ""
		wizardDescription = ""
	})
	err := execCmdAux(t, "wizard", "--resume", "some-session")
	if err == nil {
		t.Fatal("expected error for --resume without feedback, got nil")
	}
	if !strings.Contains(err.Error(), "feedback") {
		t.Errorf("error %q does not mention feedback", err.Error())
	}
}

// --- gatherDropletContext ---

// TestGatherDropletContext_IncludesDropletAndNotes verifies that the triage
// context block contains the droplet identity, description, and notes.
func TestGatherDropletContext_IncludesDropletAndNotes(t *testing.T) {
	db := filepath.Join(t.TempDir(), "test.db")
	c, err := cistern.New(db, "ct")
	if err != nil {
		t.Fatal(err)
	}
	item, err := c.Add("myrepo", "Apply phase fails on duplicates", "dedup rail failed", 2)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.AddNote(item.ID, "cataractae", "blocked: DB locked"); err != nil {
		t.Fatal(err)
	}
	c.Close()

	got, err := gatherDropletContext(db, item.ID)
	if err != nil {
		t.Fatalf("gatherDropletContext: %v", err)
	}
	for _, want := range []string{
		"=== DROPLET CONTEXT ===",
		item.ID,
		"Apply phase fails on duplicates",
		"dedup rail failed",
		"blocked: DB locked",
		"=== END DROPLET CONTEXT ===",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("context missing %q, got:\n%s", want, got)
		}
	}
}

// TestGatherDropletContext_MissingDropletErrors verifies a clear error for a
// droplet id that does not exist.
func TestGatherDropletContext_MissingDropletErrors(t *testing.T) {
	db := filepath.Join(t.TempDir(), "test.db")
	c, err := cistern.New(db, "ct")
	if err != nil {
		t.Fatal(err)
	}
	c.Close()
	if _, err := gatherDropletContext(db, "no-such-id"); err == nil {
		t.Fatal("expected error for missing droplet, got nil")
	}
}

// --- buildAgentPrompt ---

// TestBuildAgentPrompt_AssemblesContextAndSystem verifies ordering: context
// block, then system prompt, then user prompt.
func TestBuildAgentPrompt_AssemblesContextAndSystem(t *testing.T) {
	got := buildAgentPrompt("=== CTX ===", "SYS PROMPT", "user ask")
	ctxIdx := strings.Index(got, "=== CTX ===")
	sysIdx := strings.Index(got, "SYS PROMPT")
	userIdx := strings.Index(got, "user ask")
	if ctxIdx == -1 || sysIdx == -1 || userIdx == -1 {
		t.Fatalf("missing sections in:\n%s", got)
	}
	if !(ctxIdx < sysIdx && sysIdx < userIdx) {
		t.Errorf("expected context < system < user, got:\n%s", got)
	}
}

// TestBuildAgentPrompt_EmptyContextDropsHeader verifies no leading blank section.
func TestBuildAgentPrompt_EmptyContextDropsHeader(t *testing.T) {
	got := buildAgentPrompt("", "SYS", "user")
	if strings.Contains(got, "=== END") {
		t.Errorf("unexpected context markers, got:\n%s", got)
	}
	if !strings.Contains(got, "SYS") || !strings.Contains(got, "user") {
		t.Errorf("system and user sections missing, got:\n%s", got)
	}
}

// --- auxiliary agent command construction ---

// TestBuildAgentRunCommand_TriageAgentFlag verifies the agent flag carries the
// triage agent name and its AGENTS.md identity is written to the temp dir.
func TestBuildAgentRunCommand_TriageAgentFlag(t *testing.T) {
	preset := opencodePresetForAux()
	_, args, _, tmpDir, err := buildAgentRunCommand(preset, "triage", triageAgentMD(), "Test prompt", "")
	if err != nil {
		t.Fatalf("buildAgentRunCommand: %v", err)
	}
	if tmpDir != "" {
		defer os.RemoveAll(tmpDir)
	}
	hasAgent := false
	for i, arg := range args {
		if arg == "--agent" && i+1 < len(args) && args[i+1] == "triage" {
			hasAgent = true
		}
	}
	if !hasAgent {
		t.Errorf("args must carry --agent triage, got: %v", args)
	}
}

// TestBuildAgentRunCommand_WizardAgentFlag verifies the same for the wizard agent.
func TestBuildAgentRunCommand_WizardAgentFlag(t *testing.T) {
	preset := opencodePresetForAux()
	_, args, _, tmpDir, err := buildAgentRunCommand(preset, "wizard", wizardAgentMD(), "Test prompt", "")
	if err != nil {
		t.Fatalf("buildAgentRunCommand: %v", err)
	}
	if tmpDir != "" {
		defer os.RemoveAll(tmpDir)
	}
	hasAgent := false
	for i, arg := range args {
		if arg == "--agent" && i+1 < len(args) && args[i+1] == "wizard" {
			hasAgent = true
		}
	}
	if !hasAgent {
		t.Errorf("args must carry --agent wizard, got: %v", args)
	}
}

// --- prompt methodology locks ---

// TestTriagePrompt_StateMachine verifies the triage prompt carries the
// triage state machine (adapted from mattpocock/skills "triage").
func TestTriagePrompt_StateMachine(t *testing.T) {
	for _, want := range []string{
		"ready-for-agent",
		"needs-info",
		"wontfix",
		"agent brief",
		"Verify the claim",
	} {
		if !strings.Contains(triageSystemPrompt, want) {
			t.Errorf("triageSystemPrompt missing %q", want)
		}
	}
}

// TestWizardPrompt_TemplateLibraryContract verifies the wizard prompt carries
// the mp-wizard template-library contract (adapted from mattpocock/skills).
func TestWizardPrompt_TemplateLibraryContract(t *testing.T) {
	for _, want := range []string{
		"stage",
		"mp-wizard",
		"secret",
		"shellcheck",
	} {
		if !strings.Contains(wizardSystemPrompt, want) {
			t.Errorf("wizardSystemPrompt missing %q", want)
		}
	}
}
