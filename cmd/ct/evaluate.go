package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/MichielDean/cistern/internal/evaluate"
	"github.com/MichielDean/cistern/internal/evaluate/benchmarks"
	"github.com/MichielDean/cistern/internal/provider"
	"github.com/spf13/cobra"
)

var evaluateCmd = &cobra.Command{
	Use:   "evaluate",
	Short: "Score code changes against the Cistern quality rubric",
	Long: `Evaluate scores a diff or PR against the Cistern quality rubric.

This produces structured scores across 8 dimensions, each on a 0-5 scale:
  - contract_correctness: Does every method do what its signature promises?
  - integration_coverage: Do new code paths have integration tests?
  - coupling: Is new code coupled to specific entities when it could be generic?
  - migration_safety: Do migrations follow safe practices?
  - idiom_fit: Does the code use the framework's idiomatic patterns?
  - dry: Are repeated patterns extracted into helpers?
  - naming_clarity: Are types and methods honestly named?
  - error_messages: Are error messages actionable?

Use --diff to score an existing diff, --pr to score a PR, or leave flags
empty to score the current branch against main.`,
	RunE: runEvaluate,
}

var (
	evalDiff   string
	evalBase   string
	evalHead   string
	evalPR     int
	evalTicket string
	evalSource string
	evalBranch string
	evalCommit string
	evalModel  string
	evalOutput string
	evalFormat string
)

var evaluateBenchCmd = &cobra.Command{
	Use:   "benchmark",
	Short: "Run synthetic benchmark items through both Cistern and vibe-coded modes",
	Long: `Run the Cistern benchmark suite: synthetic work items of representative
complexity scored against the quality rubric. Each item produces two scores --
one for the Cistern pipeline approach, one for a vibe-coded single-shot.
Compare the results to measure pipeline effectiveness over time.

By default runs all benchmark items. Use --item to run a specific one.`,
	RunE: runEvaluateBenchmark,
}

var (
	evalBenchItem string
)

func init() {
	rootCmd.AddCommand(evaluateCmd)
	evaluateCmd.AddCommand(evaluateBenchCmd)

	evaluateCmd.Flags().StringVarP(&evalDiff, "diff", "d", "", "Raw diff content to evaluate")
	evaluateCmd.Flags().StringVar(&evalBase, "base", "main", "Base branch for diff (default: main)")
	evaluateCmd.Flags().StringVar(&evalHead, "head", "", "Head branch for diff (default: current branch)")
	evaluateCmd.Flags().IntVarP(&evalPR, "pr", "p", 0, "PR number to evaluate")
	evaluateCmd.Flags().StringVarP(&evalTicket, "ticket", "t", "", "Jira/ticket ID for comparative evaluation")
	evaluateCmd.Flags().StringVar(&evalSource, "source", "cistern", "Source label (e.g., 'cistern' or 'vibe-coded')")
	evaluateCmd.Flags().StringVar(&evalBranch, "branch", "", "Branch name (default: current branch)")
	evaluateCmd.Flags().StringVar(&evalCommit, "commit", "", "Commit SHA (default: HEAD)")
	evaluateCmd.Flags().StringVar(&evalModel, "model", "", "LLM model to use for evaluation (default: auto-detect)")
	evaluateCmd.Flags().StringVarP(&evalOutput, "output", "o", "", "Output file path (default: stdout)")
	evaluateCmd.Flags().StringVarP(&evalFormat, "format", "f", "json", "Output format: json or markdown")

	evaluateBenchCmd.Flags().StringVar(&evalBenchItem, "item", "", "Run a specific benchmark item by ID (default: all)")
	evaluateBenchCmd.Flags().StringVar(&evalModel, "model", "", "LLM model to use for evaluation (default: auto-detect)")
	evaluateBenchCmd.Flags().StringVarP(&evalOutput, "output", "o", "", "Output file path (default: stdout)")
	evaluateBenchCmd.Flags().StringVarP(&evalFormat, "format", "f", "markdown", "Output format: json or markdown")
}

func runEvaluate(cmd *cobra.Command, args []string) error {
	diff, err := resolveDiff()
	if err != nil {
		return err
	}

	if diff == "" {
		return fmt.Errorf("no diff provided -- use --diff, --base/--head, or --pr")
	}

	if evalSource == "" {
		evalSource = "unknown"
	}

	if evalBranch == "" {
		evalBranch = currentBranch()
	}

	if evalCommit == "" {
		evalCommit = "HEAD"
	}

	caller, err := resolveLLMCaller()
	if err != nil {
		return fmt.Errorf("resolving LLM caller: %w", err)
	}

	result, err := evaluate.EvaluateWithLLM(diff, caller, evalSource, evalTicket, evalBranch, evalCommit)
	if err != nil {
		return fmt.Errorf("evaluation failed: %w", err)
	}

	result.Timestamp = time.Now().UTC().Format(time.RFC3339)

	return writeResult(result)
}

func runEvaluateBenchmark(cmd *cobra.Command, args []string) error {
	caller, err := resolveLLMCaller()
	if err != nil {
		return fmt.Errorf("resolving LLM caller: %w", err)
	}

	items := benchmarks.DefaultItems()

	if evalBenchItem != "" {
		found := false
		for _, item := range items {
			if item.ID == evalBenchItem {
				items = []benchmarks.Item{item}
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("benchmark item %q not found", evalBenchItem)
		}
	}

	var allResults []struct {
		Item       benchmarks.Item
		CisternResult *evaluate.Result
		VibeResult    *evaluate.Result
	}

	for _, item := range items {
		fmt.Fprintf(os.Stderr, "Evaluating benchmark item: %s (%s)\n", item.ID, item.Title)

		cisternResult, err := evaluateWithItem(item, caller, "cistern")
		if err != nil {
			fmt.Fprintf(os.Stderr, "  Cistern evaluation failed: %v\n", err)
			continue
		}

		vibeResult, err := evaluateWithItem(item, caller, "vibe-coded")
		if err != nil {
			fmt.Fprintf(os.Stderr, "  Vibe-coded evaluation failed: %v\n", err)
			continue
		}

		allResults = append(allResults, struct {
			Item          benchmarks.Item
			CisternResult *evaluate.Result
			VibeResult    *evaluate.Result
		}{Item: item, CisternResult: cisternResult, VibeResult: vibeResult})
	}

	if len(allResults) == 0 {
		return fmt.Errorf("no benchmark results produced")
	}

	return writeBenchmarkResults(allResults)
}

func evaluateWithItem(item benchmarks.Item, caller *evaluate.LLMCaller, source string) (*evaluate.Result, error) {
	prompt := fmt.Sprintf(`You are reviewing code that was produced by a "%s" approach for the following work item:

## Work Item: %s
### Complexity: %s

%s

Produce the code for this work item. Then evaluate your own output against the rubric below.
Score honestly — you are evaluating yourself, so be extra critical.

%s`, source, item.Title, item.Complexity, item.Description, evaluate.ScoringPrompt())

	response, err := caller.Call(prompt)
	if err != nil {
		return nil, fmt.Errorf("LLM call for %s: %w", source, err)
	}

	result, err := evaluate.ParseEvaluationResult(response)
	if err != nil {
		return nil, fmt.Errorf("parsing %s response: %w\n\nRaw:\n%s", source, err, response)
	}

	result.Source = source
	result.Ticket = item.ID
	result.Timestamp = time.Now().UTC().Format(time.RFC3339)

	return result, nil
}

func resolveLLMCaller() (*evaluate.LLMCaller, error) {
	preset := provider.ResolvePreset("claude")
	if evalModel != "" {
		preset.DefaultModel = evalModel
	}

	model := preset.DefaultModel
	if model == "" {
		model = "claude-sonnet-4-20250514"
	}

	return evaluate.NewLLMCaller(
		preset.Command,
		preset.Args,
		preset.NonInteractive.PrintFlag,
		preset.NonInteractive.PromptFlag,
		preset.ModelFlag,
		model,
		preset.NonInteractive.AllowedToolsFlag,
		"",
	), nil
}

func resolveDiff() (string, error) {
	if evalDiff != "" {
		return evalDiff, nil
	}

	if evalPR > 0 {
		input := evaluate.DiffInput{
			Source:   evaluate.DiffFromPR,
			PRNumber: evalPR,
		}
		return input.GetDiff()
	}

	if evalHead == "" {
		evalHead = currentBranch()
	}
	input := evaluate.DiffInput{
		Source:     evaluate.DiffFromBranches,
		BaseBranch: evalBase,
		HeadBranch: evalHead,
	}
	return input.GetDiff()
}

func currentBranch() string {
	out, err := exec.Command("git", "branch", "--show-current").Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(out))
}

func writeResult(r *evaluate.Result) error {
	var output []byte
	var err error

	switch strings.ToLower(evalFormat) {
	case "markdown", "md":
		output = []byte(formatMarkdown(r))
	case "json":
		output, err = json.MarshalIndent(r, "", "  ")
		if err != nil {
			return fmt.Errorf("marshaling result: %w", err)
		}
	default:
		return fmt.Errorf("unknown format: %s (use json or markdown)", evalFormat)
	}

	if evalOutput != "" {
		if err := os.WriteFile(evalOutput, output, 0644); err != nil {
			return fmt.Errorf("writing output: %w", err)
		}
		fmt.Fprintf(os.Stderr, "Evaluation written to %s\n", evalOutput)
	} else {
		fmt.Println(string(output))
	}

	return nil
}

func writeBenchmarkResults(results []struct {
	Item          benchmarks.Item
	CisternResult *evaluate.Result
	VibeResult    *evaluate.Result
}) error {
	switch strings.ToLower(evalFormat) {
	case "json":
		data, err := json.MarshalIndent(results, "", "  ")
		if err != nil {
			return fmt.Errorf("marshaling results: %w", err)
		}
		if evalOutput != "" {
			return os.WriteFile(evalOutput, data, 0644)
		}
		fmt.Println(string(data))
		return nil

	case "markdown", "md":
		var sb strings.Builder
		sb.WriteString("# Cistern Benchmark Results\n\n")

		cisternTotal := 0
		vibeTotal := 0
		maxTotal := 0

		for _, r := range results {
			sb.WriteString(fmt.Sprintf("## %s: %s\n\n", r.Item.ID, r.Item.Title))
			sb.WriteString(evaluate.FormatComparison(r.CisternResult, r.VibeResult))
			sb.WriteString("\n\n---\n\n")

			cisternTotal += r.CisternResult.TotalScore
			vibeTotal += r.VibeResult.TotalScore
			maxTotal += r.CisternResult.MaxScore
		}

		sb.WriteString("## Aggregate\n\n")
		sb.WriteString(fmt.Sprintf("| Source | Total | Max | Percentage |\n"))
		sb.WriteString(fmt.Sprintf("|---|---|---|---|\n"))
		sb.WriteString(fmt.Sprintf("| Cistern | %d | %d | %.0f%% |\n",
			cisternTotal, maxTotal, float64(cisternTotal)/float64(maxTotal)*100))
		sb.WriteString(fmt.Sprintf("| Vibe-coded | %d | %d | %.0f%% |\n",
			vibeTotal, maxTotal, float64(vibeTotal)/float64(maxTotal)*100))
		delta := cisternTotal - vibeTotal
		sb.WriteString(fmt.Sprintf("\nOverall delta: %+d\n", delta))

		if evalOutput != "" {
			return os.WriteFile(evalOutput, []byte(sb.String()), 0644)
		}
		fmt.Println(sb.String())
		return nil

	default:
		return fmt.Errorf("unknown format: %s (use json or markdown)", evalFormat)
	}
}

func formatMarkdown(r *evaluate.Result) string {
	var sb strings.Builder

	sb.WriteString("# Code Quality Evaluation\n\n")
	sb.WriteString(fmt.Sprintf("- **Source:** %s\n", r.Source))
	if r.Ticket != "" {
		sb.WriteString(fmt.Sprintf("- **Ticket:** %s\n", r.Ticket))
	}
	sb.WriteString(fmt.Sprintf("- **Branch:** %s\n", r.Branch))
	sb.WriteString(fmt.Sprintf("- **Model:** %s\n", r.Model))
	sb.WriteString(fmt.Sprintf("- **Score:** %d/%d (%.0f%%)\n", r.TotalScore, r.MaxScore, r.Percentage()))
	sb.WriteString(fmt.Sprintf("- **Evaluated:** %s\n\n", r.Timestamp))

	sb.WriteString("| Dimension | Score | Evidence |\n")
	sb.WriteString("|---|---|---|\n")
	for _, s := range r.Scores {
		sb.WriteString(fmt.Sprintf("| %s | %d/5 | %s |\n", s.Dimension, s.Score, s.Evidence))
	}

	if r.Notes != "" {
		sb.WriteString(fmt.Sprintf("\n## Notes\n\n%s\n", r.Notes))
	}

	return sb.String()
}