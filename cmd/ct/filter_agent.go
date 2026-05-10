package main

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/MichielDean/cistern/internal/provider"
)

// filterAgentTmux spawns an opencode filter session inside a tmux session,
// waits for completion, and returns the agent's text response.
//
// This replaces the old direct-exec approach (opencode run --format json)
// which doesn't work because opencode run requires an existing session ID
// and doesn't return output on stdout. Instead, we:
//  1. Create a temp workdir with CONTEXT.md
//  2. Write an AGENTS.md with filter-specific instructions
//  3. Spawn opencode in a tmux session (like cataractae do)
//  4. Use pipe-pane to capture PTY output to a log file
//  5. Wait for the tmux session to exit (poll with timeout)
//  6. Read the log file, strip ANSI codes, extract the agent's response
//  7. Clean up: kill tmux session, remove temp dir
func filterAgentTmux(preset provider.ProviderPreset, prompt string) (filterSessionResult, error) {
	sessionID := fmt.Sprintf("filter-%d", time.Now().UnixMilli())

	// 1. Create temp workdir
	tmpDir, err := os.MkdirTemp("", "ct-filter-*")
	if err != nil {
		return filterSessionResult{}, fmt.Errorf("filter: create temp dir: %w", err)
	}
	cleanup := true
	defer func() {
		if cleanup {
			os.RemoveAll(tmpDir)
		}
	}()

	// 2. Write CONTEXT.md with the full prompt
	contextPath := filepath.Join(tmpDir, "CONTEXT.md")
	if err := os.WriteFile(contextPath, []byte(prompt), 0o644); err != nil {
		return filterSessionResult{}, fmt.Errorf("filter: write CONTEXT.md: %w", err)
	}

	// 3. Write AGENTS.md with filter-specific instructions
	agentsMd := filterAgentsMD()
	if err := os.WriteFile(filepath.Join(tmpDir, "AGENTS.md"), []byte(agentsMd), 0o644); err != nil {
		return filterSessionResult{}, fmt.Errorf("filter: write AGENTS.md: %w", err)
	}

	// 3b. Create identity directory with agent markdown file (for --agent flag)
	// The identity directory follows the cataractae pattern:
	// <workdir>/identity/agents/filter.md contains the full prompt with frontmatter
	identityDir := filepath.Join(tmpDir, "identity")
	agentsSubDir := filepath.Join(identityDir, "agents")
	if err := os.MkdirAll(agentsSubDir, 0o755); err != nil {
		return filterSessionResult{}, fmt.Errorf("filter: create agents dir: %w", err)
	}
	agentFrontmatter := strings.Join([]string{
		"---",
		"description: Cistern filter agent — analyzes and refines work item specifications",
		"mode: primary",
		"---",
		"",
	}, "\n")
	agentMdPath := filepath.Join(agentsSubDir, "filter.md")
	agentMdContent := agentFrontmatter + prompt
	if err := os.WriteFile(agentMdPath, []byte(agentMdContent), 0o644); err != nil {
		return filterSessionResult{}, fmt.Errorf("filter: write agent markdown: %w", err)
	}

	// 4. Build the agent command (returns a single string like cataractae's buildPresetCmd)
	cmdStr, envPairs, err := buildFilterTmuxCommand(preset, tmpDir)
	if err != nil {
		return filterSessionResult{}, fmt.Errorf("filter: build command: %w", err)
	}

	// 5. Set up session log — create the file before spawning so pipe-pane
	// can write to it even if the agent exits quickly
	homeDir, _ := os.UserHomeDir()
	logDir := filepath.Join(homeDir, ".cistern", "session-logs")
	os.MkdirAll(logDir, 0o750)
	logPath := filepath.Join(logDir, sessionID+".log")
	// Pre-create the log file to ensure pipe-pane has something to write to
	os.WriteFile(logPath, nil, 0o644)

	// 6. Spawn tmux session
	tmuxArgs := []string{"new-session", "-d", "-s", sessionID, "-c", tmpDir}
	for _, kv := range envPairs {
		tmuxArgs = append(tmuxArgs, "-e", kv)
	}
	tmuxArgs = append(tmuxArgs, cmdStr) // single command string, not separate args

	slog.Default().Info("filter: spawning tmux session",
		"session", sessionID,
		"workdir", tmpDir,
		"command", cmdStr)

	if out, err := exec.Command("tmux", tmuxArgs...).CombinedOutput(); err != nil {
		return filterSessionResult{}, fmt.Errorf("filter: tmux spawn failed: %w: %s", err, strings.TrimSpace(string(out)))
	}

	// Check if session actually started
	if !isSessionAlive(sessionID) {
		// Session died immediately — likely a command error. Try to read log anyway.
		logData, _ := os.ReadFile(logPath)
		return filterSessionResult{}, fmt.Errorf("filter: session %s died immediately; log: %s", sessionID, string(logData))
	}

	// 7. Set remain-on-exit off and attach pipe-pane
	exec.Command("tmux", "set-window-option", "-t", sessionID, "remain-on-exit", "off").Run()
	exec.Command("tmux", "pipe-pane", "-o", "-t", sessionID, "cat >> "+shellQuote(logPath)).Run()

	// Brief pause to let pipe-pane attach before the agent potentially exits
	time.Sleep(500 * time.Millisecond)

	// 8. Wait for session to exit, polling every 2 seconds with a configurable timeout
	timeout := 10 * time.Minute
	deadline := time.Now().Add(timeout)
	pollInterval := 2 * time.Second

	for time.Now().Before(deadline) {
		if !isSessionAlive(sessionID) {
			break
		}
		time.Sleep(pollInterval)
	}

	// If still alive after timeout, kill it
	if isSessionAlive(sessionID) {
		slog.Default().Warn("filter: session timed out, killing", "session", sessionID)
		exec.Command("tmux", "kill-session", "-t", sessionID).Run()
	}

	// 9. Read the log file
	logData, err := os.ReadFile(logPath)
	if err != nil {
		return filterSessionResult{}, fmt.Errorf("filter: read session log: %w", err)
	}

	slog.Default().Info("filter: raw log data",
		"session", sessionID,
		"bytes", len(logData))

	// 10. Clean up the log file
	os.Remove(logPath)

	// 11. Strip ANSI codes and terminal control sequences from PTY output
	rawText := cleanANSI(string(logData))
	response := extractFilterResponse(rawText)

	// Try to parse as JSON (in case the agent produced structured output)
	var sessionID_ string
	text := response

	// 12. Clean up temp dir (deferred above will handle it unless we set cleanup=false)
	_ = cleanup // used by defer

	slog.Default().Info("filter: completed",
		"session", sessionID_,
		"text_len", len(text),
		"raw_len", len(rawText))

	return filterSessionResult{
		SessionID: sessionID_,
		Text:      text,
	}, nil
}

// filterAgentResume resumes an existing filter tmux session by sending a message
// via tmux send-keys. This is used for --resume rounds.
func filterAgentResume(preset provider.ProviderPreset, sessionID, message string) (filterSessionResult, error) {
	if !isSessionAlive(sessionID) {
		return filterSessionResult{}, fmt.Errorf("filter: session %s not found (may have exited)", sessionID)
	}

	// Send the message as keystrokes to the tmux session
	// Use send-keys with the message + Enter
	exec.Command("tmux", "send-keys", "-t", sessionID, message, "Enter").Run()

	// Wait for the agent to respond (poll session log for changes)
	homeDir, _ := os.UserHomeDir()
	logPath := filepath.Join(homeDir, ".cistern", "session-logs", sessionID+".log")

	// Read current log size as baseline
	prevSize := int64(0)
	if info, err := os.Stat(logPath); err == nil {
		prevSize = info.Size()
	}

	// Poll until log stops growing (5-second stability window) or 10-minute timeout
	timeout := 10 * time.Minute
	deadline := time.Now().Add(timeout)
	stableSince := time.Time{}
	stabilityWindow := 5 * time.Second

	for time.Now().Before(deadline) {
		time.Sleep(2 * time.Second)
		if !isSessionAlive(sessionID) {
			break
		}
		curSize := int64(0)
		if info, err := os.Stat(logPath); err == nil {
			curSize = info.Size()
		}
		if curSize > prevSize {
			prevSize = curSize
			stableSince = time.Time{}
		} else {
			if stableSince.IsZero() {
				stableSince = time.Now()
			}
			if time.Since(stableSince) > stabilityWindow {
				break
			}
		}
	}

	// Read and parse the response
	logData, err := os.ReadFile(logPath)
	if err != nil {
		return filterSessionResult{}, fmt.Errorf("filter: read session log: %w", err)
	}

	rawText := cleanANSI(string(logData))
	response := extractFilterResponse(rawText)

	return filterSessionResult{
		SessionID: sessionID,
		Text:      response,
	}, nil
}

// buildFilterTmuxCommand constructs the shell command for spawning the filter agent
// in a tmux session. It mirrors the cataractae session.buildPresetCmd logic.
func buildFilterTmuxCommand(preset provider.ProviderPreset, workDir string) (string, []string, error) {
	var parts []string

	// Resolve command path
	cmdPath := preset.Command
	if p, err := exec.LookPath(preset.Command); err == nil {
		cmdPath = p
	}
	parts = append(parts, shellQuote(cmdPath))

	// Subcommand (e.g., "run" for opencode)
	if preset.Subcommand != "" {
		parts = append(parts, preset.Subcommand)
	}

	// Preset args (e.g., --dangerously-skip-permissions)
	for _, a := range preset.Args {
		parts = append(parts, shellQuote(a))
	}

	// Model
	if preset.DefaultModel != "" && preset.ModelFlag != "" {
		parts = append(parts, preset.ModelFlag, shellQuote(preset.DefaultModel))
	}

	// Agent flag — use filter identity from the identity directory we created
	if preset.AgentFlag != "" {
		parts = append(parts, preset.AgentFlag, "filter")
	}

	// Prompt: short message telling the agent to read CONTEXT.md
	// For PromptPositional providers (opencode), this goes as a positional arg
	// after all flags. For PromptFlag providers, use the flag.
	shortPrompt := "Read CONTEXT.md and follow the instructions in AGENTS.md."
	if preset.PromptPositional {
		parts = append(parts, shellQuote(shortPrompt))
	} else if preset.PromptFlag != "" {
		parts = append(parts, preset.PromptFlag, shellQuote(shortPrompt))
	}

	// Build the full command string (tmux expects a single command argument)
	cmdStr := "exec " + strings.Join(parts, " ")

	// Build environment pairs (like collectEnvArgs in cataractae)
	var envPairs []string

	// Preset-driven env passthrough
	for _, envVar := range preset.EnvPassthrough {
		if val := os.Getenv(envVar); val != "" {
			envPairs = append(envPairs, envVar+"="+val)
		}
	}
	// Extra env from preset
	for k, v := range preset.ExtraEnv {
		envPairs = append(envPairs, k+"="+v)
	}

	// Required: PATH, HOME, USER, SHELL, GIT_EDITOR, GIT_SEQUENCE_EDITOR
	envPairs = append(envPairs,
		"PATH="+os.Getenv("PATH"),
		"HOME="+homeDir(),
		"USER="+os.Getenv("USER"),
		"SHELL="+os.Getenv("SHELL"),
		"GIT_EDITOR=true",
		"GIT_SEQUENCE_EDITOR=true",
	)

	// Always-unset: env vars that interfere with opencode.
	// OPENCODE_SERVER_* causes "session not found" errors when opencode run
	// tries to connect to an authenticated server instead of starting fresh.
	envPairs = append(envPairs,
		"OPENCODE_SERVER_USERNAME=",
		"OPENCODE_SERVER_PASSWORD=",
		"OPENCODE_PID=",
		"OPENCODE=",
	)

	// Set OPENCODE_CONFIG_DIR to the identity directory so --agent filter finds the agent file
	if preset.AgentFlag != "" {
		envPairs = append(envPairs, "OPENCODE_CONFIG_DIR="+filepath.Join(workDir, "identity"))
		envPairs = append(envPairs, "OPENCODE_DISABLE_PROJECT_CONFIG=1")
	}

	return cmdStr, envPairs, nil
}

// filterAgentsMD returns the AGENTS.md content for the filter agent.
// This gives the filter agent clear instructions about its role:
// analyze the spec, ask probing questions, and then stop.
func filterAgentsMD() string {
	return `<!-- filter agent — generates and refines specifications -->

# Role: Filter Agent

You are a software specification analyst. Your job is to review a rough idea in CONTEXT.md and produce clear, actionable specifications that a developer can implement without guessing.

## Instructions

1. Read CONTEXT.md carefully
2. Analyze the idea for completeness, clarity, and feasibility
3. Produce a numbered list of droplets (work items) with:
   - Title (imperative, e.g., "Add user authentication")
   - Description (what, why, acceptance criteria)
   - Dependencies on other droplets (use IDs after filing)
   - Complexity assessment (standard, full, critical)
4. Ask probing questions about anything ambiguous
5. When the spec is concrete and unambiguous, say "Ready to file" and list the final droplets

## Rules

- Every droplet must have clear acceptance criteria — the implementer should not have to guess
- State dependencies explicitly: "Droplet 2 requires droplet 1 to be delivered first"
- Minimum 3 filtration rounds before filing — keep going until the spec is unambiguous
- Be direct. Skip preamble and postamble. Just the spec.
`
}

// cleanANSI removes ANSI escape sequences and terminal control characters from
// raw PTY output. This is more aggressive than stripANSI (which only removes color
// codes) because pipe-pane captures the full PTY stream including cursor movements,
// status lines, and other TUI chrome.
var cleanANSIRegex = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]|\x1b\][^\x07]*\x07|\x1b\[.*?[a-zA-Z]|\r`)

func cleanANSI(s string) string {
	return cleanANSIRegex.ReplaceAllString(s, "")
}

// extractFilterResponse extracts the agent's response text from the raw PTY log.
// It looks for the last substantial block of text after the prompt was sent,
// skipping the opencode TUI chrome (splash screen, status lines, etc.).
func extractFilterResponse(raw string) string {
	lines := strings.Split(raw, "\n")

	// Find lines that look like assistant output (not TUI chrome).
	// The filter agent's response will be substantial text blocks.
	// We look for the last contiguous block of non-empty, non-chrome lines.
	var responseLines []string
	inResponse := false

	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			if inResponse {
				break
			}
			continue
		}
		// Skip TUI chrome lines (opencode status bar, key hints, etc.)
		if isTUICrome(line) {
			if inResponse {
				break
			}
			continue
		}
		inResponse = true
		responseLines = append([]string{lines[i]}, responseLines...)
	}

	if len(responseLines) == 0 {
		return strings.TrimSpace(raw)
	}

	return strings.TrimSpace(strings.Join(responseLines, "\n"))
}

// isTUICrome returns true if a line looks like TUI framework chrome
// (status bars, key bindings, splash text, etc.) rather than agent output.
func isTUICrome(line string) bool {
	// OpenCode TUI patterns
	lower := strings.ToLower(line)
	switch {
	case strings.HasPrefix(lower, "opencode"):
		return true
	case strings.Contains(lower, "press ") && strings.Contains(lower, " to "):
		return true
	case strings.Contains(lower, "session"):
		// Keep "Session not found" as error output, skip session ID headers
		return !strings.Contains(lower, "error") && !strings.Contains(lower, "not found")
	case len(line) < 5 && line != "":
		// Short lines are likely key hints or borders
		return true
	}
	return false
}

func homeDir() string {
	if h, err := os.UserHomeDir(); err == nil {
		return h
	}
	return "/root"
}

// minimalTmuxEnv returns a minimal environment for tmux sessions,
// matching the cataractae approach of not leaking the calling process's env.
func minimalTmuxEnv() []string {
	env := []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + homeDir(),
		"USER=" + os.Getenv("USER"),
		"SHELL=" + os.Getenv("SHELL"),
		"TERM=xterm-256color",
	}
	if tmpdir := os.Getenv("TMPDIR"); tmpdir != "" {
		env = append(env, "TMPDIR="+tmpdir)
	}
	if xdg := os.Getenv("XDG_RUNTIME_DIR"); xdg != "" {
		env = append(env, "XDG_RUNTIME_DIR="+xdg)
	}
	return env
}

// isSessionAlive checks if a tmux session with the given ID is running.
var isSessionAlive = func(sessionID string) bool {
	return exec.Command("tmux", "has-session", "-t", sessionID).Run() == nil
}

// shellQuote wraps a string in single quotes for safe shell interpolation.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}