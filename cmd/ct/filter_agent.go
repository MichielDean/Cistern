package main

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/MichielDean/cistern/internal/provider"
)

const responseFileName = "RESPONSE.md"

// filterAgentTmux spawns an opencode filter session inside a tmux session,
// waits for completion, and returns the agent's text response.
//
// The agent writes its response to RESPONSE.md in the workdir. We read that
// file after the tmux session exits — no PTY parsing, no ANSI stripping.
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

	// 4. Build the agent command
	cmdStr, envPairs, err := buildFilterTmuxCommand(preset, tmpDir)
	if err != nil {
		return filterSessionResult{}, fmt.Errorf("filter: build command: %w", err)
	}

	// 5. Spawn tmux session
	tmuxArgs := []string{"new-session", "-d", "-s", sessionID, "-c", tmpDir}
	for _, kv := range envPairs {
		tmuxArgs = append(tmuxArgs, "-e", kv)
	}
	tmuxArgs = append(tmuxArgs, cmdStr)

	slog.Default().Info("filter: spawning tmux session",
		"session", sessionID,
		"workdir", tmpDir,
		"command", cmdStr)

	if out, err := exec.Command("tmux", tmuxArgs...).CombinedOutput(); err != nil {
		return filterSessionResult{}, fmt.Errorf("filter: tmux spawn failed: %w: %s", err, strings.TrimSpace(string(out)))
	}

	// Check if session actually started
	if !isSessionAlive(sessionID) {
		return filterSessionResult{}, fmt.Errorf("filter: session %s died immediately", sessionID)
	}

	// 6. Set remain-on-exit off so tmux cleans up when the agent exits
	exec.Command("tmux", "set-window-option", "-t", sessionID, "remain-on-exit", "off").Run()

	// 7. Wait for session to exit, polling every 2 seconds with configurable timeout
	timeout := filterTimeout()
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

	// 8. Read the response file written by the agent
	responsePath := filepath.Join(tmpDir, responseFileName)
	responseData, err := os.ReadFile(responsePath)
	if err != nil {
		if os.IsNotExist(err) {
			return filterSessionResult{}, fmt.Errorf("filter: agent did not write %s in %s", responseFileName, tmpDir)
		}
		return filterSessionResult{}, fmt.Errorf("filter: read response file: %w", err)
	}

	response := strings.TrimSpace(string(responseData))

	slog.Default().Info("filter: completed",
		"session", sessionID,
		"text_len", len(response))

	return filterSessionResult{
		Text: response,
	}, nil
}

// filterAgentResume resumes an existing filter tmux session by sending a message
// via tmux send-keys. This is used for --resume rounds.
// The agent writes its updated response to RESPONSE.md in the workdir.
func filterAgentResume(preset provider.ProviderPreset, sessionID, message string) (filterSessionResult, error) {
	if !isSessionAlive(sessionID) {
		return filterSessionResult{}, fmt.Errorf("filter: session %s not found (may have exited)", sessionID)
	}

	// Find the workdir by looking up the tmux session's current directory
	workdir, err := tmuxSessionWorkdir(sessionID)
	if err != nil {
		return filterSessionResult{}, fmt.Errorf("filter: find workdir for session %s: %w", sessionID, err)
	}

	// Remove the old RESPONSE.md so we can detect when the agent writes a new one
	responsePath := filepath.Join(workdir, responseFileName)
	os.Remove(responsePath)

	// Send the message as keystrokes to the tmux session
	exec.Command("tmux", "send-keys", "-t", sessionID, message, "Enter").Run()

	// Wait for RESPONSE.md to appear (poll with timeout)
	timeout := filterTimeout()
	deadline := time.Now().Add(timeout)
	pollInterval := 2 * time.Second

	for time.Now().Before(deadline) {
		if _, err := os.Stat(responsePath); err == nil {
			// File appeared — give it a moment to finish writing
			time.Sleep(1 * time.Second)
			break
		}
		if !isSessionAlive(sessionID) {
			break
		}
		time.Sleep(pollInterval)
	}

	// Read the response file
	responseData, err := os.ReadFile(responsePath)
	if err != nil {
		if os.IsNotExist(err) {
			return filterSessionResult{}, fmt.Errorf("filter: agent did not write %s after resume", responseFileName)
		}
		return filterSessionResult{}, fmt.Errorf("filter: read response file: %w", err)
	}

	response := strings.TrimSpace(string(responseData))
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
// This gives the filter agent clear instructions about its role and
// tells it to write results to RESPONSE.md for reliable extraction.
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

## CRITICAL: Writing Your Response

You MUST write your complete response to a file called RESPONSE.md in your working directory. This is the primary output mechanism — your response will be read from that file.

After generating your response, write it to RESPONSE.md using your file write tool. The file must contain your full analysis and droplet specifications.

If you cannot write files, include your response in your text output as a fallback.
`
}

// isSessionAlive checks if a tmux session with the given ID is running.
var isSessionAlive = func(sessionID string) bool {
	return exec.Command("tmux", "has-session", "-t", sessionID).Run() == nil
}

// tmuxSessionWorkdir returns the working directory of a tmux session.
func tmuxSessionWorkdir(sessionID string) (string, error) {
	out, err := exec.Command("tmux", "display-message", "-t", sessionID, "-p", "#{session_path}").Output()
	if err != nil {
		return "", fmt.Errorf("tmux display-message: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

func homeDir() string {
	if h, err := os.UserHomeDir(); err == nil {
		return h
	}
	return "/root"
}

// shellQuote wraps a string in single quotes for safe shell interpolation.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}