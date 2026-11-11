package main

import (
	"fmt"
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

// invokeAgentSessionNew starts a new auxiliary-agent session and returns the
// agent's text response with session_id.
func invokeAgentSessionNew(preset provider.ProviderPreset, agentName, agentsMd, prompt string) (filterSessionResult, error) {
	result, err := agentSessionRun(preset, agentName, agentsMd, prompt, "")
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

// invokeAgentSessionResume resumes an auxiliary-agent session with the given
// message and returns the updated response with session_id.
func invokeAgentSessionResume(preset provider.ProviderPreset, agentName, agentsMd, sessionID, message string) (filterSessionResult, error) {
	result, err := agentSessionResume(preset, agentName, agentsMd, sessionID, message)
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

Continue a triage session:
  ct triage --resume <session-id> 'your feedback here'

Use --output-format json for scriptable output (session_id + text).`,
	RunE: func(cmd *cobra.Command, args []string) error {
		preset := resolveFilterPreset("")

		if triageResume != "" {
			if len(args) == 0 {
				return fmt.Errorf("feedback argument required: ct triage --resume <id> '<feedback>'")
			}
			result, err := invokeAgentSessionResume(preset, "triage", triageAgentMD(), triageResume, strings.Join(args, " "))
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

		userPrompt := ""
		if triageTitle != "" {
			userPrompt = "Title: " + triageTitle
			if triageDescription != "" {
				userPrompt += "\nDescription: " + triageDescription
			}
		}
		prompt := buildAgentPrompt(contextBlock, triageSystemPrompt, userPrompt)
		result, err := invokeAgentSessionNew(preset, "triage", triageAgentMD(), prompt)
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

Continue a wizard-design session:
  ct wizard --resume <session-id> 'your feedback here'

Use --output-format json for scriptable output (session_id + text).`,
	RunE: func(cmd *cobra.Command, args []string) error {
		preset := resolveFilterPreset("")

		if wizardResume != "" {
			if len(args) == 0 {
				return fmt.Errorf("feedback argument required: ct wizard --resume <id> '<feedback>'")
			}
			result, err := invokeAgentSessionResume(preset, "wizard", wizardAgentMD(), wizardResume, strings.Join(args, " "))
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
		result, err := invokeAgentSessionNew(preset, "wizard", wizardAgentMD(), prompt)
		if err != nil {
			return err
		}
		return printFilterResult(result, wizardOutputFormat)
	},
}

func init() {
	triageCmd.Flags().StringVar(&triageTitle, "title", "", "rough idea to triage (alternative to a droplet id)")
	triageCmd.Flags().StringVar(&triageDescription, "description", "", "context for the triage idea")
	triageCmd.Flags().StringVar(&triageResume, "resume", "", "resume an existing triage session by ID")
	triageCmd.Flags().StringVar(&triageOutputFormat, "output-format", "human", "output format: human or json")
	rootCmd.AddCommand(triageCmd)

	wizardCmd.Flags().StringVar(&wizardTitle, "title", "", "what the wizard should cover")
	wizardCmd.Flags().StringVar(&wizardDescription, "description", "", "context for the wizard design")
	wizardCmd.Flags().StringVar(&wizardResume, "resume", "", "resume an existing wizard-design session by ID")
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
