package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/MichielDean/cistern/internal/provider"
	"github.com/spf13/cobra"
)

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
			result, err := filterAgentRunResume(preset, filterResume, strings.Join(args, " "))
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
	prompt := buildFilterPrompt(contextBlock, userPrompt)
	result, err := filterAgentRun(preset, prompt)
	if err != nil {
		return filterSessionResult{}, err
	}
	// The NDJSON output from opencode run --format json already provides
	// session_id in each event, so tryParseAgentJSON is no longer needed
	// for the primary path. Keep it as a fallback for any embedded JSON.
	if result.SessionID == "" {
		if envelope := tryParseAgentJSON(result.Text); envelope != nil {
			result.SessionID = envelope.SessionID
			if envelope.Result != "" {
				result.Text = envelope.Result
			}
		}
	}
	return result, nil
}

// invokeFilterResume resumes an existing filtration session with the given message
// and returns the updated response with session_id.
func invokeFilterResume(preset provider.ProviderPreset, sessionID, message string) (filterSessionResult, error) {
	result, err := filterAgentRunResume(preset, sessionID, message)
	if err != nil {
		return filterSessionResult{}, err
	}
	if result.SessionID == "" {
		if envelope := tryParseAgentJSON(result.Text); envelope != nil {
			result.SessionID = envelope.SessionID
			if envelope.Result != "" {
				result.Text = envelope.Result
			}
		}
	}
	return result, nil
}

// agentJSONEnvelope is the envelope returned by the agent when it produces
// structured JSON output. This is the same format as opencode --format json.
type agentJSONEnvelope struct {
	Type      string `json:"type"`
	Subtype   string `json:"subtype"`
	IsError   bool   `json:"is_error"`
	Result    string `json:"result"`
	SessionID string `json:"session_id"`
}

// tryParseAgentJSON attempts to parse a JSON envelope from the agent's text output.
// Returns nil if the text is not valid JSON or does not match the expected structure.
func tryParseAgentJSON(text string) *agentJSONEnvelope {
	// The agent may produce NDJSON (one JSON event per line) or a single JSON object.
	// Try to find a line that looks like a complete/envelope event.
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line[0] != '{' {
			continue
		}
		var envelope agentJSONEnvelope
		if err := json.Unmarshal([]byte(line), &envelope); err == nil {
			if envelope.Type != "" && envelope.Result != "" {
				return &envelope
			}
		}
	}
	// Try parsing the entire text as a single JSON envelope
	var envelope agentJSONEnvelope
	if err := json.Unmarshal([]byte(strings.TrimSpace(text)), &envelope); err == nil {
		if envelope.Type != "" {
			return &envelope
		}
	}
	return nil
}

// printFilterResult writes the filtration result to stdout. Human format prints
// the agent's text directly. --output-format json (user-facing ct filter flag)
// emits a JSON object with session_id and text.
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

const filterTimeoutEnvKey = "CT_FILTER_TIMEOUT"

// filterTimeout returns the timeout for a filter session. Defaults to 10 minutes.
// Can be overridden with CT_FILTER_TIMEOUT environment variable (in seconds).
func filterTimeout() time.Duration {
	if v := os.Getenv(filterTimeoutEnvKey); v != "" {
		if d, err := time.ParseDuration(v + "s"); err == nil {
			return d
		}
	}
	return 10 * time.Minute
}

// callFilterAgent is a stub retained for backward compatibility with test code.
// The direct-exec approach has been replaced by opencode run --format json
// (filterAgentRun). This function always returns an error.
//
// Deprecated: Use filterAgentRun instead.
func callFilterAgent(preset provider.ProviderPreset, extraArgs []string, prompt string) (filterSessionResult, error) {
	return filterSessionResult{}, fmt.Errorf("callFilterAgent is deprecated: filter now uses opencode run --format json (filterAgentRun)")
}

func init() {
	filterCmd.Flags().StringVar(&filterTitle, "title", "", "rough idea title (required for new sessions)")
	filterCmd.Flags().StringVar(&filterDescription, "description", "", "rough idea description")
	filterCmd.Flags().StringVar(&filterResume, "resume", "", "resume an existing filtration session by ID")
	filterCmd.Flags().StringVar(&filterOutputFormat, "output-format", "human", "output format: human or json")
	rootCmd.AddCommand(filterCmd)
}
