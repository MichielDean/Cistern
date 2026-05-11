package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/MichielDean/cistern/internal/provider"
)

// filterAgentRun invokes opencode run --format json to execute the filter agent.
// This is the direct-exec approach: run opencode as a subprocess, capture its
// NDJSON output, and parse the text events to extract the agent's response.
//
// Key insight: OPENCODE_SERVER_* env vars must be unset so opencode doesn't
// try to connect to an existing server (which causes "Session not found" errors).
func filterAgentRun(preset provider.ProviderPreset, prompt string) (filterSessionResult, error) {
	// Build the command
	cmdPath, args, env, tmpDir, err := buildFilterRunCommand(preset, prompt, "")
	if err != nil {
		return filterSessionResult{}, err
	}
	if tmpDir != "" {
		defer os.RemoveAll(tmpDir)
	}

	cmd := exec.Command(cmdPath, args...)
	cmd.Env = env
	cmd.Stderr = os.Stderr // pass stderr through for debugging

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return filterSessionResult{}, fmt.Errorf("filter: create stdout pipe: %w", err)
	}

	slog.Default().Info("filter: running opencode",
		"command", cmdPath,
		"args", strings.Join(args, " "))

	if err := cmd.Start(); err != nil {
		return filterSessionResult{}, fmt.Errorf("filter: start command: %w", err)
	}

	// Parse NDJSON output with timeout
	var textParts []string
	var sessionID string
	scanner := bufio.NewScanner(stdout)
	done := make(chan struct{})
	go func() {
		for scanner.Scan() {
			line := scanner.Text()
			if line == "" {
				continue
			}
			var event map[string]interface{}
			if err := json.Unmarshal([]byte(line), &event); err != nil {
				continue
			}

			// Extract session ID from any event that has one
			if sid, ok := event["sessionID"].(string); ok && sid != "" && sessionID == "" {
				sessionID = sid
			}

			// Extract text from text events
			if eventType, _ := event["type"].(string); eventType == "text" {
				if part, ok := event["part"].(map[string]interface{}); ok {
					if text, ok := part["text"].(string); ok && text != "" {
						textParts = append(textParts, text)
					}
				}
			}
		}
		close(done)
	}()

	// Wait for command to complete with timeout
	cmdDone := make(chan error, 1)
	go func() {
		cmdDone <- cmd.Wait()
	}()

	timeout := filterTimeout()
	select {
	case cmdErr := <-cmdDone:
		// Command completed, wait for scanner to finish
		<-done
		if cmdErr != nil && len(textParts) == 0 {
			return filterSessionResult{}, fmt.Errorf("filter: command failed: %w", cmdErr)
		}
		if cmdErr != nil {
			slog.Default().Warn("filter: command exited with error but produced output",
				"error", cmdErr,
				"text_parts", len(textParts))
		}
	case <-time.After(timeout):
		if cmd.Process != nil {
			cmd.Process.Kill()
		}
		<-done
		return filterSessionResult{}, fmt.Errorf("filter: command timed out after %v", timeout)
	}

	response := strings.TrimSpace(strings.Join(textParts, "\n"))

	slog.Default().Info("filter: completed",
		"session_id", sessionID,
		"text_len", len(response))

	return filterSessionResult{
		SessionID: sessionID,
		Text:      response,
	}, nil
}

// filterAgentRunResume resumes an existing filter session using opencode run
// with the --session flag and --format json.
func filterAgentRunResume(preset provider.ProviderPreset, sessionID, message string) (filterSessionResult, error) {
	cmdPath, args, env, tmpDir, err := buildFilterRunCommand(preset, message, sessionID)
	if err != nil {
		return filterSessionResult{}, err
	}
	if tmpDir != "" {
		defer os.RemoveAll(tmpDir)
	}

	cmd := exec.Command(cmdPath, args...)
	cmd.Env = env
	cmd.Stderr = os.Stderr

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return filterSessionResult{}, fmt.Errorf("filter: create stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return filterSessionResult{}, fmt.Errorf("filter: start command: %w", err)
	}

	var textParts []string
	scanner := bufio.NewScanner(stdout)
	done := make(chan struct{})
	go func() {
		for scanner.Scan() {
			line := scanner.Text()
			if line == "" {
				continue
			}
			var event map[string]interface{}
			if err := json.Unmarshal([]byte(line), &event); err != nil {
				continue
			}
			if eventType, _ := event["type"].(string); eventType == "text" {
				if part, ok := event["part"].(map[string]interface{}); ok {
					if text, ok := part["text"].(string); ok && text != "" {
						textParts = append(textParts, text)
					}
				}
			}
		}
		close(done)
	}()

	cmdDone := make(chan error, 1)
	go func() {
		cmdDone <- cmd.Wait()
	}()

	timeout := filterTimeout()
	select {
	case cmdErr := <-cmdDone:
		<-done
		if cmdErr != nil && len(textParts) == 0 {
			return filterSessionResult{}, fmt.Errorf("filter: resume command failed: %w", cmdErr)
		}
		if cmdErr != nil {
			slog.Default().Warn("filter: resume command exited with error but produced output",
				"error", cmdErr,
				"session_id", sessionID)
		}
	case <-time.After(timeout):
		if cmd.Process != nil {
			cmd.Process.Kill()
		}
		<-done
		return filterSessionResult{}, fmt.Errorf("filter: resume command timed out after %v", timeout)
	}

	response := strings.TrimSpace(strings.Join(textParts, "\n"))

	return filterSessionResult{
		SessionID: sessionID,
		Text:      response,
	}, nil
}

// buildFilterRunCommand constructs the opencode run command and environment.
// If sessionID is non-empty, adds -s flag for resume mode.
// Returns (cmdPath, args, env, tmpDir, error) — tmpDir is empty if no temp dir was created.
func buildFilterRunCommand(preset provider.ProviderPreset, prompt string, sessionID string) (string, []string, []string, string, error) {
	// Resolve command path
	cmdPath := preset.Command
	if p, err := exec.LookPath(preset.Command); err == nil {
		cmdPath = p
	}

	args := []string{"run"}

	// Preset args (e.g., --dangerously-skip-permissions)
	args = append(args, preset.Args...)

	// Model
	if preset.DefaultModel != "" && preset.ModelFlag != "" {
		args = append(args, preset.ModelFlag, preset.DefaultModel)
	}

	// Agent flag
	var tmpDir string
	if preset.AgentFlag != "" {
		args = append(args, preset.AgentFlag, "filter")

		// Create temp workdir with identity files
		var err error
		tmpDir, err = os.MkdirTemp("", "ct-filter-*")
		if err != nil {
			return "", nil, nil, "", fmt.Errorf("filter: create temp dir: %w", err)
		}

		// Write AGENTS.md
		agentsMd := filterAgentsMD()
		if err := os.WriteFile(filepath.Join(tmpDir, "AGENTS.md"), []byte(agentsMd), 0o644); err != nil {
			os.RemoveAll(tmpDir)
			return "", nil, nil, "", fmt.Errorf("filter: write AGENTS.md: %w", err)
		}

		// Write CONTEXT.md with the full prompt
		if err := os.WriteFile(filepath.Join(tmpDir, "CONTEXT.md"), []byte(prompt), 0o644); err != nil {
			os.RemoveAll(tmpDir)
			return "", nil, nil, "", fmt.Errorf("filter: write CONTEXT.md: %w", err)
		}

		// Create identity directory with agent markdown
		identityDir := filepath.Join(tmpDir, "identity", "agents")
		if err := os.MkdirAll(identityDir, 0o755); err != nil {
			os.RemoveAll(tmpDir)
			return "", nil, nil, "", fmt.Errorf("filter: create identity dir: %w", err)
		}
		agentFrontmatter := "---\ndescription: Cistern filter agent — analyzes and refines work item specifications\nmode: primary\n---\n\n"
		agentMdPath := filepath.Join(identityDir, "filter.md")
		if err := os.WriteFile(agentMdPath, []byte(agentFrontmatter+prompt), 0o644); err != nil {
			os.RemoveAll(tmpDir)
			return "", nil, nil, "", fmt.Errorf("filter: write agent markdown: %w", err)
		}
	}

	// Format: always use JSON for parseable output
	args = append(args, "--format", "json")

	// Skip permissions for non-interactive use
	args = append(args, "--dangerously-skip-permissions")

	// Session ID for resume
	if sessionID != "" {
		args = append(args, "-s", sessionID)
	}

	// Prompt: positional arg for opencode (PromptPositional providers)
	// or via flag
	if preset.PromptPositional {
		args = append(args, prompt)
	} else if preset.PromptFlag != "" {
		args = append(args, preset.PromptFlag, prompt)
	}

	// Build environment — clear OPENCODE_SERVER_* vars that interfere
	env := os.Environ()
	env = unsetEnvPrefix(env, "OPENCODE_SERVER_USERNAME=")
	env = unsetEnvPrefix(env, "OPENCODE_SERVER_PASSWORD=")
	env = unsetEnvPrefix(env, "OPENCODE_PID=")
	env = unsetEnvPrefix(env, "OPENCODE=")

	// Set OPENCODE_CONFIG_DIR if using agent flag
	if preset.AgentFlag != "" && tmpDir != "" {
		env = append(env, "OPENCODE_CONFIG_DIR="+filepath.Join(tmpDir, "identity"))
		env = append(env, "OPENCODE_DISABLE_PROJECT_CONFIG=1")
	}

	return cmdPath, args, env, tmpDir, nil
}

// unsetEnvPrefix removes entries that start with the given prefix from the
// environment slice. Used to clear vars like OPENCODE_SERVER_USERNAME= that
// cause "Session not found" errors.
func unsetEnvPrefix(env []string, prefix string) []string {
	var result []string
	for _, e := range env {
		if !strings.HasPrefix(e, prefix) {
			result = append(result, e)
		}
	}
	return result
}

// filterAgentsMD returns the AGENTS.md content for the filter agent.
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