package cataractae

import (
	"bytes"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/MichielDean/cistern/internal/cistern"
	"github.com/MichielDean/cistern/internal/provider"
)

type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func captureDefaultSlog(t *testing.T) *syncBuffer {
	t.Helper()
	prev := slog.Default()
	buf := &syncBuffer{}
	l := slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	slog.SetDefault(l)
	t.Cleanup(func() { slog.SetDefault(prev) })
	return buf
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
	if err := os.WriteFile(filepath.Join(identityDir, "AGENTS.md"),
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
	t.Setenv("HOME", dir)

	s := &Session{ID: "test", WorkDir: dir, Identity: "implementer"}
	prompt := s.buildPrompt()

	if !strings.Contains(prompt, "cataractae/implementer/AGENTS.md") {
		t.Error("prompt missing fallback path 'cataractae/implementer/AGENTS.md' when identity file is missing")
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
	cisternPath := filepath.Join(dir, ".cistern", "cataractae", "reviewer", "AGENTS.md")
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
	t.Setenv("HOME", dir)

	s := &Session{Identity: "implementer"}
	got := s.resolveIdentityPath()
	want := "cataractae/implementer/AGENTS.md"
	if got != want {
		t.Errorf("resolveIdentityPath = %q, want %q", got, want)
	}
}

func TestWriteAgentMarkdown_MirrorsToWorktree(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	// The fallback identity root resolves relative to the process working
	// directory — run against a temp cwd so the test never writes repo files.
	tempCwd := t.TempDir()
	prevCwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(tempCwd); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(prevCwd) })

	s := &Session{
		ID:       "test-repo-worker",
		WorkDir:  t.TempDir(),
		Identity: "reviewer",
	}
	agentPath, err := s.writeAgentMarkdown("# reviewer prompt")
	if err != nil {
		t.Fatalf("writeAgentMarkdown: %v", err)
	}
	if _, err := os.Stat(agentPath); err != nil {
		t.Fatalf("identity agent markdown missing: %v", err)
	}

	workAgentPath := filepath.Join(s.WorkDir, ".opencode", "agents", "reviewer.md")
	data, err := os.ReadFile(workAgentPath)
	if err != nil {
		t.Fatalf("worktree agent markdown missing: %v", err)
	}
	if !strings.Contains(string(data), "reviewer prompt") {
		t.Error("worktree agent markdown should mirror the prompt content")
	}
}

func TestWriteAgentMarkdown_EmptyWorkDir(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	tempCwd := t.TempDir()
	prevCwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(tempCwd); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(prevCwd) })

	s := &Session{ID: "test-repo-worker", Identity: "reviewer"}
	agentPath, err := s.writeAgentMarkdown("# reviewer prompt")
	if err != nil {
		t.Fatalf("writeAgentMarkdown with empty WorkDir: %v", err)
	}
	if _, err := os.Stat(agentPath); err != nil {
		t.Fatalf("identity agent markdown missing: %v", err)
	}
}

// buildTestBin compiles the Go package at importPath into a temp directory
// and returns the absolute path to the resulting binary.
func buildTestBin(t *testing.T, name, importPath string) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), name)
	out, err := exec.Command("go", "build", "-o", bin, importPath).CombinedOutput()
	if err != nil {
		t.Fatalf("build %s: %v\n%s", name, err, out)
	}
	return bin
}

// TestFakeagent_SpawnOutcomeCycle exercises the full Session.Spawn →
// Session.isAlive → droplet outcome pipeline using the fakeagent binary.
func TestFakeagent_SpawnOutcomeCycle(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not available — skipping integration test")
	}

	fakeagentBin := buildTestBin(t, "fakeagent", "github.com/MichielDean/cistern/internal/testutil/fakeagent")
	ctBin := buildTestBin(t, "ct", "github.com/MichielDean/cistern/cmd/ct")

	binDir := filepath.Dir(ctBin)
	t.Setenv("PATH", binDir+":"+os.Getenv("PATH"))

	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")
	t.Setenv("CT_DB", dbPath)
	t.Setenv("CT_NO_ASCII_LOGO", "1")

	c, err := cistern.New(dbPath, "fa")
	if err != nil {
		t.Fatalf("cistern.New: %v", err)
	}
	defer c.Close()

	droplet, err := c.Add("testrepo", "fakeagent test", "desc", 1)
	if err != nil {
		t.Fatalf("cistern.Add: %v", err)
	}

	workDir := t.TempDir()
	contextContent := fmt.Sprintf("# Context\n\n## Item: %s\n\n**Title:** fakeagent test\n", droplet.ID)
	if err := os.WriteFile(filepath.Join(workDir, "CONTEXT.md"), []byte(contextContent), 0o644); err != nil {
		t.Fatalf("write CONTEXT.md: %v", err)
	}

	sessionID := "ci-t3xo9-fa-" + droplet.ID
	s := &Session{
		ID:      sessionID,
		WorkDir: workDir,
		Preset:  provider.ResolvePreset("opencode"),
	}
	s.Preset.Command = fakeagentBin

	if err := s.Spawn(); err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	t.Cleanup(func() { exec.Command("tmux", "kill-session", "-t", s.ID).Run() })

	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if !s.isAlive() {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if s.isAlive() {
		t.Fatal("session still alive after 15s — fakeagent did not exit")
	}

	got, err := c.Get(droplet.ID)
	if err != nil {
		t.Fatalf("cistern.Get: %v", err)
	}
	if got.Outcome != "pass" {
		t.Errorf("droplet outcome = %q, want %q", got.Outcome, "pass")
	}
}

// TestResolveIdentityPath_UsesPresetInstructionsFile verifies that resolveIdentityPath
// returns the preset's InstructionsFile rather than always AGENTS.md.
func TestResolveIdentityPath_UsesPresetInstructionsFile(t *testing.T) {
	dir := t.TempDir()
	identityDir := filepath.Join(dir, ".cistern", "cataractae", "implementer")
	if err := os.MkdirAll(identityDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(identityDir, "AGENTS.md"), []byte("# Implementer"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", dir)

	s := &Session{
		Identity: "implementer",
		Preset:   provider.ProviderPreset{InstructionsFile: "AGENTS.md"},
	}
	got := s.resolveIdentityPath()
	want := filepath.Join(identityDir, "AGENTS.md")
	if got != want {
		t.Errorf("resolveIdentityPath = %q, want %q", got, want)
	}
}

// TestResolveIdentityPath_FallbackSandbox_WithPreset verifies that when the cistern
// path does not exist, the sandbox-relative path uses the preset's InstructionsFile.
func TestResolveIdentityPath_FallbackSandbox_WithPreset(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	s := &Session{
		Identity: "reviewer",
		Preset:   provider.ProviderPreset{InstructionsFile: "CUSTOM.md"},
	}
	got := s.resolveIdentityPath()
	want := "cataractae/reviewer/CUSTOM.md"
	if got != want {
		t.Errorf("resolveIdentityPath = %q, want %q", got, want)
	}
}

// TestBuildContextPreamble_ReadsInstructionsFile verifies that buildContextPreamble
// returns the content of the preset's InstructionsFile when it exists.
func TestBuildContextPreamble_ReadsInstructionsFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("# Opencode Role\n\nDo work."), 0o644); err != nil {
		t.Fatal(err)
	}
	preset := provider.ProviderPreset{InstructionsFile: "AGENTS.md"}
	got := buildContextPreamble(dir, preset)
	if got != "# Opencode Role\n\nDo work." {
		t.Errorf("buildContextPreamble = %q, want %q", got, "# Opencode Role\n\nDo work.")
	}
}

// TestBuildContextPreamble_FallsBackToSourceFiles verifies that when InstructionsFile
// is missing, PERSONA.md + INSTRUCTIONS.md are concatenated as fallback.
func TestBuildContextPreamble_FallsBackToSourceFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "PERSONA.md"), []byte("# Role: Coder"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "INSTRUCTIONS.md"), []byte("Write tests."), 0o644); err != nil {
		t.Fatal(err)
	}
	preset := provider.ProviderPreset{InstructionsFile: "AGENTS.md"}
	got := buildContextPreamble(dir, preset)
	if !strings.Contains(got, "# Role: Coder") {
		t.Error("fallback preamble missing PERSONA.md content")
	}
	if !strings.Contains(got, "Write tests.") {
		t.Error("fallback preamble missing INSTRUCTIONS.md content")
	}
}

// TestBuildContextPreamble_EmptyWhenAllMissing verifies that buildContextPreamble
// returns empty string when neither InstructionsFile nor source files exist.
func TestBuildContextPreamble_EmptyWhenAllMissing(t *testing.T) {
	dir := t.TempDir()
	preset := provider.ProviderPreset{InstructionsFile: "AGENTS.md"}
	got := buildContextPreamble(dir, preset)
	if got != "" {
		t.Errorf("buildContextPreamble = %q, want empty string when all files missing", got)
	}
}

// TestBuildContextPreamble_DefaultsToOpencode verifies that empty InstructionsFile
// defaults to reading AGENTS.md.
func TestBuildContextPreamble_DefaultsToOpencode(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("Opencode role content"), 0o644); err != nil {
		t.Fatal(err)
	}
	preset := provider.ProviderPreset{}
	got := buildContextPreamble(dir, preset)
	if got != "Opencode role content" {
		t.Errorf("buildContextPreamble = %q, want %q", got, "Opencode role content")
	}
}

// TestBuildPrompt_NonAddDirProvider_InjectsContextPreamble verifies that buildPrompt
// injects the InstructionsFile content via buildContextPreamble.
func TestBuildPrompt_NonAddDirProvider_InjectsContextPreamble(t *testing.T) {
	dir := t.TempDir()
	identityDir := filepath.Join(dir, ".cistern", "cataractae", "implementer")
	if err := os.MkdirAll(identityDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(identityDir, "AGENTS.md"),
		[]byte("# Implementer (opencode)\n\nYou write code."), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", dir)

	s := &Session{
		ID:       "test",
		WorkDir:  dir,
		Identity: "implementer",
		Preset: provider.ProviderPreset{
			Name:             "opencode",
			InstructionsFile: "AGENTS.md",
		},
	}
	prompt := s.buildPrompt()

	if !strings.Contains(prompt, "## Your Role") {
		t.Error("prompt missing '## Your Role' section")
	}
	if !strings.Contains(prompt, "You write code.") {
		t.Error("prompt missing AGENTS.md content")
	}
	if !strings.Contains(prompt, baseCataractaePrompt) {
		t.Error("prompt missing constitutional base")
	}
}

// TestBuildPrompt_NonAddDirProvider_InjectsSkills verifies that skill content is
// injected into the prompt when Skills is set.
func TestBuildPrompt_NonAddDirProvider_InjectsSkills(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, ".cistern", "skills", "my-skill")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# My Skill\n\nDo the skill thing."), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", dir)

	s := &Session{
		ID:      "test",
		WorkDir: dir,
		Preset: provider.ProviderPreset{
			Name: "opencode",
		},
		Skills: []string{"my-skill"},
	}
	prompt := s.buildPrompt()

	if !strings.Contains(prompt, "my-skill") {
		t.Error("prompt missing skill name")
	}
	if !strings.Contains(prompt, "Do the skill thing.") {
		t.Error("prompt missing skill content")
	}
}

// TestBuildPresetCmd_ModelWithSpaces_IsShellQuoted verifies that a model value
// containing spaces is shell-quoted before being interpolated into the tmux
// command string.
func TestBuildPresetCmd_ModelWithSpaces_IsShellQuoted(t *testing.T) {
	s := &Session{ID: "test", WorkDir: "/tmp", Model: "opencode llama 3.3"}
	preset := provider.ProviderPreset{
		Name:      "myagent",
		Command:   "myagent",
		ModelFlag: "--model",
	}
	cmd, err := s.buildPresetCmd(preset, "/skills")
	if err != nil {
		t.Fatalf("buildPresetCmd: %v", err)
	}
	if strings.Contains(cmd, "--model opencode llama") {
		t.Errorf("buildPresetCmd contains unquoted model with space — will break shell: %s", cmd)
	}
	want := "--model 'opencode llama 3.3'"
	if !strings.Contains(cmd, want) {
		t.Errorf("buildPresetCmd missing shell-quoted model\nwant substring: %s\ngot: %s", want, cmd)
	}
}

// TestBuildPresetCmd_EmptyCommand_ReturnsError verifies that buildPresetCmd
// returns a descriptive error when the preset has no command configured.
func TestBuildPresetCmd_EmptyCommand_ReturnsError(t *testing.T) {
	s := &Session{ID: "test", WorkDir: "/tmp"}
	preset := provider.ProviderPreset{Name: "custom"}
	_, err := s.buildPresetCmd(preset, "/skills")
	if err == nil {
		t.Fatal("expected error for preset with empty Command, got nil")
	}
	if !strings.Contains(err.Error(), "custom") {
		t.Errorf("error %q should mention the preset name", err.Error())
	}
	if !strings.Contains(err.Error(), "no command configured") {
		t.Errorf("error %q should mention 'no command configured'", err.Error())
	}
}

// TestBuildPresetCmd_PromptFlag_AppendedWhenNonEmpty verifies that buildPresetCmd
// uses preset.PromptFlag to deliver the prompt when it is set.
func TestBuildPresetCmd_PromptFlag_AppendedWhenNonEmpty(t *testing.T) {
	s := &Session{ID: "test", WorkDir: "/tmp"}
	preset := provider.ProviderPreset{
		Name:       "myagent",
		Command:    "myagent",
		PromptFlag: "--prompt",
	}
	cmd, err := s.buildPresetCmd(preset, "/skills")
	if err != nil {
		t.Fatalf("buildPresetCmd: %v", err)
	}
	if !strings.Contains(cmd, "--prompt") {
		t.Errorf("buildPresetCmd output missing PromptFlag: %s", cmd)
	}
}

// TestBuildPresetCmd_PromptFlag_OmittedWhenEmpty verifies that buildPresetCmd
// does not append any prompt flag when PromptFlag is empty.
func TestBuildPresetCmd_PromptFlag_OmittedWhenEmpty(t *testing.T) {
	s := &Session{ID: "test", WorkDir: "/tmp"}
	preset := provider.ProviderPreset{
		Name:    "opencode",
		Command: "opencode",
	}
	cmd, err := s.buildPresetCmd(preset, "/skills")
	if err != nil {
		t.Fatalf("buildPresetCmd: %v", err)
	}
	if strings.Contains(cmd, " -p ") || strings.Contains(cmd, " --prompt") {
		t.Errorf("buildPresetCmd with empty PromptFlag should not contain a prompt flag: %s", cmd)
	}
	if !strings.HasPrefix(cmd, "exec '") || !strings.Contains(cmd, "opencode") {
		t.Errorf("buildPresetCmd output = %q, want exec prefix containing 'opencode'", cmd)
	}
}

// TestCollectEnvArgs_GHToken_AlwaysForwarded_PresetPath verifies that GH_TOKEN
// is included in env args when using the preset path.
func TestCollectEnvArgs_GHToken_AlwaysForwarded_PresetPath(t *testing.T) {
	t.Setenv("GH_TOKEN", "ghtoken-preset-123")
	s := &Session{
		ID:     "test",
		Preset: provider.ProviderPreset{Name: "opencode"},
	}
	args := s.collectEnvArgs()
	if !containsEnvPair(args, "GH_TOKEN", "ghtoken-preset-123") {
		t.Errorf("collectEnvArgs (preset path) missing GH_TOKEN; args: %v", args)
	}
}

// TestCollectEnvArgs_GHToken_AlwaysForwarded_LegacyPath verifies that GH_TOKEN
// is included in env args when using the legacy (no-preset) path.
func TestCollectEnvArgs_GHToken_AlwaysForwarded_LegacyPath(t *testing.T) {
	t.Setenv("GH_TOKEN", "ghtoken-legacy-456")

	s := &Session{ID: "test"}
	args := s.collectEnvArgs()
	if !containsEnvPair(args, "GH_TOKEN", "ghtoken-legacy-456") {
		t.Errorf("collectEnvArgs (legacy path) missing GH_TOKEN; args: %v", args)
	}
}

// TestCollectEnvArgs_GHToken_AbsentWhenNotSet verifies that GH_TOKEN is not
// included in env args when it is unset in the environment.
func TestCollectEnvArgs_GHToken_AbsentWhenNotSet(t *testing.T) {
	t.Setenv("GH_TOKEN", "")

	s := &Session{ID: "test", Preset: provider.ProviderPreset{Name: "opencode"}}
	args := s.collectEnvArgs()
	for _, a := range args {
		if strings.Contains(a, "GH_TOKEN") {
			t.Errorf("collectEnvArgs contains GH_TOKEN when unset; args: %v", args)
		}
	}
}

// TestCollectEnvArgs_GitEditorAlwaysSet verifies that GIT_EDITOR and
// GIT_SEQUENCE_EDITOR are always set to "true" so git never opens an
// interactive editor inside agent sessions.
func TestCollectEnvArgs_GitEditorAlwaysSet(t *testing.T) {
	s := &Session{ID: "test", Preset: provider.ProviderPreset{Name: "opencode"}}
	args := s.collectEnvArgs()
	if !containsEnvPair(args, "GIT_EDITOR", "true") {
		t.Errorf("collectEnvArgs missing GIT_EDITOR=true; args: %v", args)
	}
	if !containsEnvPair(args, "GIT_SEQUENCE_EDITOR", "true") {
		t.Errorf("collectEnvArgs missing GIT_SEQUENCE_EDITOR=true; args: %v", args)
	}
}

func containsEnvPair(args []string, key, val string) bool {
	target := key + "=" + val
	for i := 0; i < len(args)-1; i++ {
		if args[i] == "-e" && args[i+1] == target {
			return true
		}
	}
	return false
}

// TestResolveIdentityDir_CisternDirWithInstrFile verifies that when the cistern
// directory exists and contains the instrFile, resolveIdentityDir returns the cistern path.
func TestResolveIdentityDir_CisternDirWithInstrFile(t *testing.T) {
	dir := t.TempDir()
	identityDir := filepath.Join(dir, ".cistern", "cataractae", "implementer")
	if err := os.MkdirAll(identityDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(identityDir, "AGENTS.md"), []byte("# Implementer"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", dir)

	s := &Session{Identity: "implementer"}
	got := s.resolveIdentityDir()
	if got != identityDir {
		t.Errorf("resolveIdentityDir = %q, want %q", got, identityDir)
	}
}

// TestResolveIdentityDir_CisternDirWithoutInstrFile verifies that when the cistern
// directory exists but the instrFile is absent, resolveIdentityDir still returns the
// cistern path.
func TestResolveIdentityDir_CisternDirWithoutInstrFile(t *testing.T) {
	dir := t.TempDir()
	identityDir := filepath.Join(dir, ".cistern", "cataractae", "implementer")
	if err := os.MkdirAll(identityDir, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", dir)

	s := &Session{Identity: "implementer"}
	got := s.resolveIdentityDir()
	if got != identityDir {
		t.Errorf("resolveIdentityDir = %q, want %q", got, identityDir)
	}
}

// TestResolveIdentityDir_FallbackSandbox verifies that when the cistern directory
// does not exist, resolveIdentityDir returns the sandbox-relative path.
func TestResolveIdentityDir_FallbackSandbox(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	s := &Session{Identity: "implementer"}
	got := s.resolveIdentityDir()
	want := filepath.Join("cataractae", "implementer")
	if got != want {
		t.Errorf("resolveIdentityDir = %q, want %q", got, want)
	}
}

// TestBuildPresetCmd_CommandWithSpaces_IsShellQuoted verifies that a command
// path containing spaces is shell-quoted before being interpolated into the
// tmux command string.
func TestBuildPresetCmd_CommandWithSpaces_IsShellQuoted(t *testing.T) {
	orig := resolveCommandFn
	resolveCommandFn = func(cmd string) string { return cmd }
	t.Cleanup(func() { resolveCommandFn = orig })

	s := &Session{ID: "test", WorkDir: "/tmp"}
	preset := provider.ProviderPreset{
		Name:    "myagent",
		Command: "/home/john doe/bin/myagent",
	}
	cmd, err := s.buildPresetCmd(preset, "/skills")
	if err != nil {
		t.Fatalf("buildPresetCmd: %v", err)
	}
	if strings.Contains(cmd, "/home/john doe/bin/myagent") && !strings.Contains(cmd, "'/home/john doe/bin/myagent'") {
		t.Errorf("buildPresetCmd contains unquoted command path with space — will break shell: %s", cmd)
	}
	want := "exec '/home/john doe/bin/myagent'"
	if !strings.HasPrefix(cmd, want) {
		t.Errorf("buildPresetCmd should start with shell-quoted command\nwant prefix: %s\ngot: %s", want, cmd)
	}
}

// TestBuildPresetCmd_ArgsWithSpaces_AreShellQuoted verifies that Args elements
// containing spaces or shell metacharacters are shell-quoted.
func TestBuildPresetCmd_ArgsWithSpaces_AreShellQuoted(t *testing.T) {
	orig := resolveCommandFn
	resolveCommandFn = func(cmd string) string { return cmd }
	t.Cleanup(func() { resolveCommandFn = orig })

	s := &Session{ID: "test", WorkDir: "/tmp"}
	preset := provider.ProviderPreset{
		Name:    "myagent",
		Command: "myagent",
		Args:    []string{"--flag with spaces", "--another$arg"},
	}
	cmd, err := s.buildPresetCmd(preset, "/skills")
	if err != nil {
		t.Fatalf("buildPresetCmd: %v", err)
	}
	for _, want := range []string{"'--flag with spaces'", "'--another$arg'"} {
		if !strings.Contains(cmd, want) {
			t.Errorf("buildPresetCmd missing shell-quoted arg\nwant substring: %s\ngot: %s", want, cmd)
		}
	}
	if strings.Contains(cmd, "--another$arg") && !strings.Contains(cmd, "'--another$arg'") {
		t.Errorf("buildPresetCmd contains bare dollar sign in arg — shell-expansion risk: %s", cmd)
	}
}

// --- spawn logging tests ---

// TestSpawn_LogsFreshSession_WhenNoTmux verifies that spawn emits a structured
// slog entry with session, context_type=fresh, and model fields.
func TestSpawn_LogsFreshSession_WhenNoTmux(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not available")
	}

	buf := captureDefaultSlog(t)

	home := t.TempDir()
	t.Setenv("HOME", home)

	workDir := t.TempDir()
	s := &Session{
		ID:      "test-fresh-session",
		WorkDir: workDir,
		Model:   "haiku",
		Preset:  provider.ProviderPreset{Command: "echo"},
	}
	_ = s.spawn()
	defer func() { exec.Command("tmux", "kill-session", "-t", s.ID).Run() }()

	out := buf.String()
	if !strings.Contains(out, "session=test-fresh-session") {
		t.Errorf("log missing session field; got: %s", out)
	}
	if !strings.Contains(out, "context_type=fresh") {
		t.Errorf("log missing context_type=fresh; got: %s", out)
	}
	if !strings.Contains(out, "model=haiku") {
		t.Errorf("log missing model=haiku; got: %s", out)
	}
}
