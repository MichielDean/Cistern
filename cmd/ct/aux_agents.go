package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	cobra "github.com/spf13/cobra"

	"github.com/MichielDean/cistern/internal/cistern"
	"github.com/MichielDean/cistern/internal/provider"
)

// --- shared auxiliary-agent session machinery ---

// buildAgentPrompt assembles the final LLM prompt for an auxiliary agent
// session. If contextBlock is non-empty it is placed first so the agent sees
// context before its instructions and the user's request.
func buildAgentPrompt(contextBlock, systemPrompt, userPrompt string) string {
	if contextBlock == "" {
		return systemPrompt + "\n\n" + userPrompt
	}
	return contextBlock + "\n\n" + systemPrompt + "\n\n" + userPrompt
}

// renderTranscript formats persisted conversation messages as a plain-text
// transcript. Pure function — shared by CLI, dashboard, and TUI resume paths
// via buildResumePrompt.
func renderTranscript(messages []cistern.FilterMessage) string {
	var sb strings.Builder
	for _, msg := range messages {
		if msg.Role == "user" {
			sb.WriteString("USER: ")
		} else {
			sb.WriteString("ASSISTANT: ")
		}
		sb.WriteString(msg.Content)
		sb.WriteString("\n\n")
	}
	return sb.String()
}

// buildResumePrompt renders the persisted transcript plus the new user
// message as the prompt for a fresh aux-agent session. Pure function, no I/O.
// For empty messages it returns exactly newMessage; otherwise the PRIOR
// CONVERSATION block precedes the new message so the agent reliably sees
// chronology.
func buildResumePrompt(messages []cistern.FilterMessage, newMessage string) string {
	if len(messages) == 0 {
		return newMessage
	}
	return "=== PRIOR CONVERSATION (transcript) ===\n" +
		renderTranscript(messages) +
		"=== END PRIOR CONVERSATION ===\n\n" + newMessage
}

// runAuxAgentRound runs one new-or-resume round for an aux agent, persisting
// the transcript to filter_sessions. cisternID is empty for a new round.
//
// New round (cisternID == ""): creates a filter_sessions row with the agent
// column set to agentName and the caller-supplied title/description, and runs
// the agent on a fresh opencode session. Title and description are explicit
// parameters — prompts start with context or system text, never a "Title: "
// header, so the row title can never be derived by parsing the prompt.
//
// Resume round (cisternID != ""): loads the persisted transcript from the DB
// and runs a fresh opencode session whose prompt is the transcript plus the
// new message (buildResumePrompt). Session chaining lives in cistern's DB —
// never in opencode's server. Returns an error naming the missing session ID
// when the row does not exist, and rejects resumes where the session is owned
// by a different aux agent (title/description are unused on resume; legacy
// rows without an agent column read as "filter").
func runAuxAgentRound(preset provider.ProviderPreset, agentName, agentsMd, contextBlock, title, description, newMessage, cisternID string) (filterSessionResult, error) {
	c, err := cistern.New(resolveDBPath(), "")
	if err != nil {
		return filterSessionResult{}, fmt.Errorf("%s: open cistern db: %w", agentName, err)
	}
	defer c.Close()

	var prompt string
	var session *cistern.FilterSession
	// priorMessages holds the persisted transcript for resume rounds; the
	// persisted transcript after this round must be prior + new messages.
	var priorMessages []cistern.FilterMessage

	if cisternID == "" {
		session, err = c.CreateAgentSession(agentName, title, description)
		if err != nil {
			return filterSessionResult{}, fmt.Errorf("%s: create session row: %w", agentName, err)
		}
		prompt = buildAgentPrompt(contextBlock, "", newMessage)
	} else {
		session, err = c.GetFilterSession(cisternID)
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				return filterSessionResult{}, fmt.Errorf("%s: resume %s: not found in cistern db", agentName, cisternID)
			}
			return filterSessionResult{}, fmt.Errorf("%s: resume %s: %w", agentName, cisternID, err)
		}
		if session.Agent != agentName {
			return filterSessionResult{}, fmt.Errorf("%s: resume %s: session is owned by agent %q", agentName, cisternID, session.Agent)
		}
		var messages []cistern.FilterMessage
		if session.Messages != "" && session.Messages != "[]" {
			if err := json.Unmarshal([]byte(session.Messages), &messages); err != nil {
				// Degraded but functional: proceed with an empty transcript
				// rather than failing the round, but never silently.
				slog.Default().Warn("aux-agent: unparsable persisted transcript",
					"agent", agentName,
					"session_id", cisternID,
					"error", err)
			}
		}
		priorMessages = messages

		prompt = buildResumePrompt(messages, buildAgentPrompt(contextBlock, "", newMessage))
	}

	result, err := agentSessionRun(preset, agentName, agentsMd, prompt)
	if err != nil {
		if cisternID == "" {
			// The round failed before any exchange was recorded; do not leave
			// an empty row behind.
			c.DeleteFilterSession(session.ID) //nolint:errcheck
		}
		return filterSessionResult{}, err
	}

	// Persist prior transcript (empty for a new round) plus this round's
	// exchange, so resume-after-resume keeps full chronology.
	transcript := append(priorMessages,
		cistern.FilterMessage{Role: "user", Content: newMessage},
		cistern.FilterMessage{Role: "assistant", Content: result.Text})
	msgJSON, err := json.Marshal(transcript)
	if err != nil {
		return filterSessionResult{}, fmt.Errorf("%s: marshal transcript: %w", agentName, err)
	}
	if err := c.UpdateFilterSessionMessages(session.ID, string(msgJSON), result.Text, result.SessionID); err != nil {
		return filterSessionResult{}, fmt.Errorf("%s: persist transcript: %w", agentName, err)
	}

	return filterSessionResult{
		SessionID: result.SessionID,
		Text:      result.Text,
		CisternID: session.ID,
	}, nil
}

// gatherDropletContext fetches a droplet and its notes from the cistern DB and
// renders them as a delimited context block for the triage agent.
func gatherDropletContext(dbPath, dropletID string) (string, error) {
	c, err := cistern.New(dbPath, "")
	if err != nil {
		return "", fmt.Errorf("triage: open cistern db: %w", err)
	}
	defer c.Close()

	item, err := c.Get(dropletID)
	if err != nil {
		return "", fmt.Errorf("triage: fetch droplet %s: %w", dropletID, err)
	}
	notes, err := c.GetNotes(dropletID)
	if err != nil {
		return "", fmt.Errorf("triage: fetch notes for %s: %w", dropletID, err)
	}

	var sb strings.Builder
	sb.WriteString("=== DROPLET CONTEXT ===\n")
	fmt.Fprintf(&sb, "\n--- DROPLET %s ---\n\n", item.ID)
	fmt.Fprintf(&sb, "ID:          %s\n", item.ID)
	fmt.Fprintf(&sb, "Title:       %s\n", item.Title)
	fmt.Fprintf(&sb, "Repo:        %s\n", item.Repo)
	fmt.Fprintf(&sb, "Status:      %s\n", item.Status)
	fmt.Fprintf(&sb, "Priority:    %d\n", item.Priority)
	fmt.Fprintf(&sb, "Stage:       %s\n", item.CurrentCataractae)
	if item.Description != "" {
		fmt.Fprintf(&sb, "\nDescription:\n%s\n", item.Description)
	}
	if len(notes) > 0 {
		fmt.Fprintf(&sb, "\nNotes:\n")
		for _, n := range notes {
			fmt.Fprintf(&sb, "  [%s] %s\n", n.CataractaeName, n.Content)
		}
	}
	sb.WriteString("\n=== END DROPLET CONTEXT ===")
	return sb.String(), nil
}

// --- ct triage command ---

var (
	triageTitle        string
	triageDescription  string
	triageResume       string
	triageOutputFormat string
)

var triageCmd = &cobra.Command{
	Use:   "triage [droplet-id]",
	Short: "Run a triage interview — categorise, verify, and brief droplets",
	Long: `ct triage starts a triage conversation for a droplet (pooled, failed, or rough).
The agent categorises the work, verifies the claim against the codebase, and
sharpens the description into an agent brief you can confirm.

Triage a droplet (fetches its current state and notes):
  ct triage <droplet-id>

Triage session for an idea without a droplet:
  ct triage --title 'what went wrong in the apply phase'

Continue a triage session (resume accepts the cistern session ID printed to
stderr by the prior round):
  ct triage --resume <cistern-session-id> 'your feedback here'

Use --output-format json for scriptable output (session_id + text).`,
	RunE: func(cmd *cobra.Command, args []string) error {
		preset := resolveFilterPreset("")

		if triageResume != "" {
			if len(args) == 0 {
				return fmt.Errorf("feedback argument required: ct triage --resume <id> '<feedback>'")
			}
			result, err := runAuxAgentRound(preset, "triage", triageAgentMD(), "", "", "", strings.Join(args, " "), triageResume)
			if err != nil {
				return err
			}
			return printFilterResult(result, triageOutputFormat)
		}

		switch {
		case len(args) == 1 && args[0] != "":
		case len(args) > 1:
			return fmt.Errorf("usage: ct triage <droplet-id> | ct triage --title \"<idea>\"")
		}

		dropletID := ""
		if len(args) == 1 {
			dropletID = args[0]
		}
		contextBlock := ""
		if dropletID != "" {
			var err error
			contextBlock, err = gatherDropletContext(resolveDBPath(), dropletID)
			if err != nil {
				return err
			}
		} else if triageTitle == "" {
			return fmt.Errorf("pass a droplet id, or --title for a new idea: ct triage <id> | ct triage --title \"<idea>\"")
		}

		// Session title: explicit --title when given, otherwise derived from
		// the triaged droplet. Never parsed from the prompt.
		title := triageTitle
		if title == "" && dropletID != "" {
			title = fmt.Sprintf("Triage: %s", dropletID)
		}
		userPrompt := ""
		if triageTitle != "" {
			userPrompt = "Title: " + triageTitle
			if triageDescription != "" {
				userPrompt += "\nDescription: " + triageDescription
			}
		}
		prompt := buildAgentPrompt(contextBlock, triageSystemPrompt, userPrompt)
		result, err := runAuxAgentRound(preset, "triage", triageAgentMD(), "", title, triageDescription, prompt, "")
		if err != nil {
			return err
		}
		return printFilterResult(result, triageOutputFormat)
	},
}

// --- ct wizard command ---

var (
	wizardTitle        string
	wizardDescription  string
	wizardResume       string
	wizardOutputFormat string
)

var wizardCmd = &cobra.Command{
	Use:   "wizard",
	Short: "Design an interactive bash wizard for steps only a human can do",
	Long: `ct wizard starts a wizard-design conversation: it scopes a manual procedure
(setting up credentials, provisioning infra, a one-off migration), maps each
stage's journey, and authors a bash script that walks a human through it.

New session:
  ct wizard --title 'provision the production database'

Continue a wizard-design session (resume accepts the cistern session ID
printed to stderr by the prior round):
  ct wizard --resume <cistern-session-id> 'your feedback here'

Use --output-format json for scriptable output (session_id + text).`,
	RunE: func(cmd *cobra.Command, args []string) error {
		preset := resolveFilterPreset("")

		if wizardResume != "" {
			if len(args) == 0 {
				return fmt.Errorf("feedback argument required: ct wizard --resume <id> '<feedback>'")
			}
			result, err := runAuxAgentRound(preset, "wizard", wizardAgentMD(), "", "", "", strings.Join(args, " "), wizardResume)
			if err != nil {
				return err
			}
			return printFilterResult(result, wizardOutputFormat)
		}

		if wizardTitle == "" {
			return fmt.Errorf("--title is required (or use --resume to continue an existing session)")
		}
		userPrompt := "Title: " + wizardTitle
		if wizardDescription != "" {
			userPrompt += "\nDescription: " + wizardDescription
		}
		prompt := buildAgentPrompt("", wizardSystemPrompt, userPrompt)
		result, err := runAuxAgentRound(preset, "wizard", wizardAgentMD(), "", wizardTitle, wizardDescription, prompt, "")
		if err != nil {
			return err
		}
		return printFilterResult(result, wizardOutputFormat)
	},
}

func init() {
	triageCmd.Flags().StringVar(&triageTitle, "title", "", "rough idea to triage (alternative to a droplet id)")
	triageCmd.Flags().StringVar(&triageDescription, "description", "", "context for the triage idea")
	triageCmd.Flags().StringVar(&triageResume, "resume", "", "resume an existing triage session by cistern session ID")
	triageCmd.Flags().StringVar(&triageOutputFormat, "output-format", "human", "output format: human or json")
	rootCmd.AddCommand(triageCmd)

	wizardCmd.Flags().StringVar(&wizardTitle, "title", "", "what the wizard should cover")
	wizardCmd.Flags().StringVar(&wizardDescription, "description", "", "context for the wizard design")
	wizardCmd.Flags().StringVar(&wizardResume, "resume", "", "resume an existing wizard-design session by cistern session ID")
	wizardCmd.Flags().StringVar(&wizardOutputFormat, "output-format", "human", "output format: human or json")
	rootCmd.AddCommand(wizardCmd)
}

// triageAgentMD returns the AGENTS.md content for the triage agent.
func triageAgentMD() string {
	return `<!-- triage agent — categorises, verifies, and briefs droplets -->

# Role: Triage Agent

You are a triage interviewer. Verify claims against the codebase, categorise
droplets, and sharpen descriptions into agent briefs. See your instructions for
the full state machine and method.

` + triageSystemPrompt
}

// wizardAgentMD returns the AGENTS.md content for the wizard agent.
func wizardAgentMD() string {
	return `<!-- wizard agent — designs and authors human-walkthrough bash wizards -->

# Role: Wizard Agent

You design interactive bash wizards for manual procedures only a human can
perform. See your instructions for the method and template library contract.

` + wizardSystemPrompt
}
