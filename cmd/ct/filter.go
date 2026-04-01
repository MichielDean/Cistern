package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/MichielDean/cistern/internal/provider"
	"github.com/spf13/cobra"
)

// claudeJSONOutput is the envelope returned by claude --print --output-format json.
// The Result field contains the assistant's raw text response; SessionID identifies
// the conversation so it can be resumed.
type claudeJSONOutput struct {
	Type      string `json:"type"`
	Subtype   string `json:"subtype"`
	IsError   bool   `json:"is_error"`
	Result    string `json:"result"`
	SessionID string `json:"session_id"`
}

// filterSessionResult holds the parsed output from a filtration LLM invocation.
type filterSessionResult struct {
	SessionID string
	Text      string
}

var (
	filterTitle        string
	filterDescription  string
	filterResume       string
	filterOutputFormat string
)

var filterCmd = &cobra.Command{
	Use:   "filter",
	Short: "Run filtration LLM pass — refine ideas without persisting to the cistern",
	Long: `ct filter starts a refinement conversation to help you think through and spec
out work items before adding them to the cistern. At each round the agent asks
probing questions to sharpen the spec.

New session:
  ct filter --title 'rough idea' [--description '...']

Continue refinement:
  ct filter --resume <session-id> 'your feedback here'

When satisfied, file droplets manually using ct droplet add.

Use --output-format json for scriptable output (session_id + text).`,
	RunE: func(cmd *cobra.Command, args []string) error {
		preset := resolveFilterPreset("")

		if filterResume != "" {
			// --resume: feedback refinement pass.
			if len(args) == 0 {
				return fmt.Errorf("feedback argument required: ct filter --resume <id> '<feedback>'")
			}
			result, err := invokeFilterResume(preset, filterResume, strings.Join(args, " "))
			if err != nil {
				return err
			}
			return printFilterResult(result, filterOutputFormat)
		}

		// New session: --title is required.
		if filterTitle == "" {
			return fmt.Errorf("--title is required (or use --resume to continue an existing session)")
		}
		contextBlock := gatherFilterContext(filterContextConfig{
			DBPath: resolveDBPath(),
			Title:  filterTitle,
			Desc:   filterDescription,
		})
		result, err := invokeFilterNew(preset, filterTitle, filterDescription, contextBlock)
		if err != nil {
			return err
		}
		return printFilterResult(result, filterOutputFormat)
	},
}

// invokeFilterNew starts a new filtration session and returns the agent's text
// response with session_id. contextBlock is prepended before the system prompt
// so the LLM sees codebase context first.
func invokeFilterNew(preset provider.ProviderPreset, title, description, contextBlock string) (filterSessionResult, error) {
	userPrompt := "Title: " + title
	if description != "" {
		userPrompt += "\nDescription: " + description
	}
	return callFilterAgent(preset, nil, buildFilterPrompt(contextBlock, userPrompt))
}

// invokeFilterResume resumes an existing filtration session with the given message
// and returns the updated response with session_id.
func invokeFilterResume(preset provider.ProviderPreset, sessionID, message string) (filterSessionResult, error) {
	resumeFlag := preset.ResumeFlag
	if resumeFlag == "" {
		resumeFlag = "--resume"
	}
	extraArgs := []string{resumeFlag, sessionID}
	return callFilterAgent(preset, extraArgs, message)
}

// callFilterAgent invokes the preset command with --print --output-format json,
// optional extraArgs (e.g. --resume <id>), and the given prompt.
// When the preset defines NonInteractive.AllowedToolsFlag, read-only file tools
// (Glob, Grep, Read) are enabled so the agent can discover context on demand.
// It returns the raw text response and the session_id from the JSON envelope.
// If the agent does not support --output-format json, the raw stdout becomes the text
// (session_id will be empty in that case).
func callFilterAgent(preset provider.ProviderPreset, extraArgs []string, prompt string) (filterSessionResult, error) {
	for _, key := range preset.EnvPassthrough {
		if os.Getenv(key) == "" {
			return filterSessionResult{}, fmt.Errorf("%s is not set", key)
		}
	}

	// Build args: [Subcommand] [preset.Args...] [--allowedTools ...] [extraArgs...] [PrintFlag] [--output-format json] [PromptFlag prompt]
	var args []string
	if preset.NonInteractive.Subcommand != "" {
		args = append(args, preset.NonInteractive.Subcommand)
	}
	args = append(args, preset.Args...)
	if preset.NonInteractive.AllowedToolsFlag != "" {
		args = append(args, preset.NonInteractive.AllowedToolsFlag, "Glob,Grep,Read")
	}
	args = append(args, extraArgs...)
	if preset.NonInteractive.PrintFlag != "" {
		args = append(args, preset.NonInteractive.PrintFlag)
	}
	args = append(args, "--output-format", "json")
	if preset.NonInteractive.PromptFlag != "" {
		args = append(args, preset.NonInteractive.PromptFlag)
	}
	args = append(args, prompt)

	ctx, cancel := context.WithTimeout(context.Background(), 600*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, preset.Command, args...)
	if len(preset.ExtraEnv) > 0 {
		env := os.Environ()
		for k, v := range preset.ExtraEnv {
			env = append(env, k+"="+v)
		}
		cmd.Env = env
	}

	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return filterSessionResult{}, fmt.Errorf("agent exec failed (exit %d): %s", ee.ExitCode(), strings.TrimSpace(string(ee.Stderr)))
		}
		return filterSessionResult{}, fmt.Errorf("agent exec failed: %w", err)
	}

	var envelope claudeJSONOutput
	if err := json.Unmarshal(out, &envelope); err != nil {
		// Fallback: the preset may not support --output-format json; use raw output as text.
		return filterSessionResult{Text: strings.TrimSpace(string(out))}, nil
	}
	if envelope.IsError {
		return filterSessionResult{}, fmt.Errorf("agent returned error: %s", envelope.Result)
	}

	return filterSessionResult{
		SessionID: envelope.SessionID,
		Text:      envelope.Result,
	}, nil
}

// printFilterResult writes the filtration result to stdout. Human format prints
// the agent's text directly. --output-format json emits a JSON object with
// session_id and text.
func printFilterResult(result filterSessionResult, outputFormat string) error {
	if outputFormat == "json" {
		type jsonOut struct {
			SessionID string `json:"session_id"`
			Text      string `json:"text,omitempty"`
		}
		out := jsonOut{
			SessionID: result.SessionID,
			Text:      result.Text,
		}
		data, err := json.MarshalIndent(out, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal output: %w", err)
		}
		fmt.Println(string(data))
		return nil
	}

	// Human-readable: print text to stdout, session_id to stderr.
	fmt.Println(result.Text)
	if result.SessionID != "" {
		fmt.Fprintln(os.Stderr, result.SessionID)
	}
	return nil
}

func init() {
	filterCmd.Flags().StringVar(&filterTitle, "title", "", "rough idea title (required for new sessions)")
	filterCmd.Flags().StringVar(&filterDescription, "description", "", "rough idea description")
	filterCmd.Flags().StringVar(&filterResume, "resume", "", "resume an existing filtration session by ID")
	filterCmd.Flags().StringVar(&filterOutputFormat, "output-format", "human", "output format: human or json")
	rootCmd.AddCommand(filterCmd)
}
