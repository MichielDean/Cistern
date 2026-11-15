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
// SessionID is the per-turn opencode session; CisternID is the filter_sessions
// row the round was persisted to (empty when the caller did not go through
// runAuxAgentRound).
type filterSessionResult struct {
	SessionID string
	Text      string
	CisternID string
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
out work items before adding them to the cistern. At each round the agent asks the
unsettled frontier questions (each with a recommended answer) to sharpen the spec.

New session:
  ct filter --title 'rough idea' [--description '...']

Continue refinement (resume accepts the cistern session ID printed to stderr
by the prior round; the transcript is persisted in cistern's DB):
  ct filter --resume <cistern-session-id> 'your feedback here'

When satisfied, file droplets manually using ct droplet add.

Use --output-format json for scriptable output (session_id + text).`,
	RunE: func(cmd *cobra.Command, args []string) error {
		preset := resolveFilterPreset("")

		if filterResume != "" {
			// --resume: feedback refinement pass. The prior transcript is
			// loaded from filter_sessions and replayed into a fresh opencode
			// session (buildResumePrompt) — no opencode server-side resume.
			if len(args) == 0 {
				return fmt.Errorf("feedback argument required: ct filter --resume <id> '<feedback>'")
			}
			result, err := runAuxAgentRound(preset, "filter", filterAgentsMD(), "", "", "", strings.Join(args, " "), filterResume)
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

// invokeFilterNew starts a new filtration session, persists the round to
// filter_sessions (agent = 'filter', title/description from the explicit
// params), and returns the agent's text response with both the per-turn
// opencode session_id and the cistern session ID. contextBlock is prepended
// before the system prompt so the LLM sees codebase context first.
func invokeFilterNew(preset provider.ProviderPreset, title, description, contextBlock string) (filterSessionResult, error) {
	userPrompt := "Title: " + title
	if description != "" {
		userPrompt += "\nDescription: " + description
	}
	prompt := buildFilterPrompt(contextBlock, userPrompt)
	return runAuxAgentRound(preset, "filter", filterAgentsMD(), "", title, description, prompt, "")
}

// printFilterResult writes the filtration result to stdout. Human format prints
// the agent's text to stdout and the *cistern* session ID to stderr — that is
// the ID to pass to --resume. --output-format json emits a JSON object with
// cistern_session_id, session_id (per-turn opencode session), and text.
func printFilterResult(result filterSessionResult, outputFormat string) error {
	if outputFormat == "json" {
		type jsonOut struct {
			CisternSessionID string `json:"cistern_session_id,omitempty"`
			SessionID        string `json:"session_id,omitempty"`
			Text             string `json:"text,omitempty"`
		}
		out := jsonOut{
			CisternSessionID: result.CisternID,
			SessionID:        result.SessionID,
			Text:             result.Text,
		}
		data, err := json.MarshalIndent(out, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal output: %w", err)
		}
		fmt.Println(string(data))
		return nil
	}

	// Human-readable: print text to stdout, cistern session_id to stderr.
	fmt.Println(result.Text)
	sessionID := result.CisternID
	if sessionID == "" {
		sessionID = result.SessionID
	}
	if sessionID != "" {
		fmt.Fprintln(os.Stderr, sessionID)
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
	filterCmd.Flags().StringVar(&filterResume, "resume", "", "resume an existing filtration session by cistern session ID")
	filterCmd.Flags().StringVar(&filterOutputFormat, "output-format", "human", "output format: human or json")
	rootCmd.AddCommand(filterCmd)
}
