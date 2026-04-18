package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/MichielDean/cistern/internal/evaluate"
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
empty to score the current branch against main. Use --store to persist the
result for trend analysis.`,
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
	evalStore  bool
	evalDBPath string
)

var evaluateBenchCmd = &cobra.Command{
	Use:   "benchmark",
	Short: "Score real merged PRs against the quality rubric",
	Long: `Score real merged PRs against the Cistern quality rubric to measure
pipeline effectiveness over time. Instead of synthetic work items, this
evaluates actual code that landed in the repo.

Use --pr to score a specific PR, or --merged-since to score all merged
PRs since a date. Results are automatically stored for trend analysis.`,
	RunE: runEvaluateBenchmark,
}

var (
	evalBenchPRs       string
	evalBenchMergedSince string
)

var evaluateTrendCmd = &cobra.Command{
	Use:   "trend",
	Short: "Show evaluation score trends over time",
	Long: `Query historical evaluation results and display trends.
Shows how scores change over time, broken down by source and dimension.
Use --source to filter by source label, --since to limit date range.`,
	RunE: runEvaluateTrend,
}

var (
	evalTrendSource string
	evalTrendSince  string
	evalTrendLimit  int
)

var evaluateDryRunCmd = &cobra.Command{
	Use:   "dry-run",
	Short: "Simulate the pipeline on a work item and compare cistern vs vibe-coded",
	Long: `Run a dry-run simulation: architect produces a brief, implementer
produces code with that brief, then a vibe-coded one-shot produces code
without any brief. Both outputs are evaluated against the rubric.

This is throwaway — no commits, no PRs, no files written. Use it to
measure whether pipeline changes (instruction rewrites, new cataractae)
actually improve output quality before shipping them.

Use --item to pick a built-in work item, or --title and --description
for a custom task. Use --codebase-dir to point at the repo the
implementation should target (default: current directory).`,
	RunE: runEvaluateDryRun,
}

var (
	evalDryRunItem       string
	evalDryRunTitle      string
	evalDryRunDesc       string
	evalDryRunCodebaseDir string
)

func init() {
	rootCmd.AddCommand(evaluateCmd)
	evaluateCmd.AddCommand(evaluateBenchCmd)
	evaluateCmd.AddCommand(evaluateTrendCmd)
	evaluateCmd.AddCommand(evaluateDryRunCmd)

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
	evaluateCmd.Flags().BoolVar(&evalStore, "store", false, "Persist result to evaluation database")
	evaluateCmd.Flags().StringVar(&evalDBPath, "eval-db", "", "Path to evaluation database (default: ~/.cistern/evaluations.db)")

	evaluateBenchCmd.Flags().StringVar(&evalBenchPRs, "pr", "", "Comma-separated PR numbers to evaluate")
	evaluateBenchCmd.Flags().StringVar(&evalBenchMergedSince, "merged-since", "", "Evaluate all merged PRs since date (YYYY-MM-DD)")
	evaluateBenchCmd.Flags().StringVar(&evalSource, "source", "cistern", "Source label for evaluated PRs")
	evaluateBenchCmd.Flags().StringVar(&evalModel, "model", "", "LLM model to use for evaluation (default: auto-detect)")
	evaluateBenchCmd.Flags().StringVarP(&evalOutput, "output", "o", "", "Output file path (default: stdout)")
	evaluateBenchCmd.Flags().StringVarP(&evalFormat, "format", "f", "markdown", "Output format: json or markdown")
	evaluateBenchCmd.Flags().StringVar(&evalDBPath, "eval-db", "", "Path to evaluation database (default: ~/.cistern/evaluations.db)")

	evaluateTrendCmd.Flags().StringVar(&evalTrendSource, "source", "", "Filter by source label (comma-separated)")
	evaluateTrendCmd.Flags().StringVar(&evalTrendSince, "since", "", "Only show results since date (YYYY-MM-DD)")
	evaluateTrendCmd.Flags().IntVar(&evalTrendLimit, "limit", 50, "Maximum number of results to show")
	evaluateTrendCmd.Flags().StringVarP(&evalFormat, "format", "f", "markdown", "Output format: json or markdown")
	evaluateTrendCmd.Flags().StringVarP(&evalOutput, "output", "o", "", "Output file path (default: stdout)")
	evaluateTrendCmd.Flags().StringVar(&evalDBPath, "eval-db", "", "Path to evaluation database (default: ~/.cistern/evaluations.db)")

	evaluateDryRunCmd.Flags().StringVar(&evalDryRunItem, "item", "", "Built-in work item ID (e.g., dry-001, dry-002)")
	evaluateDryRunCmd.Flags().StringVar(&evalDryRunTitle, "title", "", "Custom work item title")
	evaluateDryRunCmd.Flags().StringVar(&evalDryRunDesc, "description", "", "Custom work item description")
	evaluateDryRunCmd.Flags().StringVar(&evalDryRunCodebaseDir, "codebase-dir", "", "Target codebase directory (default: current directory)")
	evaluateDryRunCmd.Flags().StringVar(&evalModel, "model", "", "LLM model to use (default: auto-detect)")
	evaluateDryRunCmd.Flags().BoolVar(&evalStore, "store", false, "Persist results to evaluation database")
	evaluateDryRunCmd.Flags().StringVarP(&evalOutput, "output", "o", "", "Output file path (default: stdout)")
	evaluateDryRunCmd.Flags().StringVarP(&evalFormat, "format", "f", "markdown", "Output format: json or markdown")
	evaluateDryRunCmd.Flags().StringVar(&evalDBPath, "eval-db", "", "Path to evaluation database (default: ~/.cistern/evaluations.db)")
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
	if evalPR > 0 {
		result.PRNumber = evalPR
	}

	if evalStore {
		if err := storeResult(result); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to store result: %v\n", err)
		} else {
			fmt.Fprintf(os.Stderr, "Result stored to evaluation database\n")
		}
	}

	return writeResult(result)
}

func runEvaluateBenchmark(cmd *cobra.Command, args []string) error {
	prNumbers, err := resolveBenchmarkPRs()
	if err != nil {
		return err
	}
	if len(prNumbers) == 0 {
		return fmt.Errorf("no PRs to evaluate -- use --pr or --merged-since")
	}

	caller, err := resolveLLMCaller()
	if err != nil {
		return fmt.Errorf("resolving LLM caller: %w", err)
	}

	storage, err := evaluate.OpenStorage(evalDBPath)
	if err != nil {
		return fmt.Errorf("opening eval database: %w", err)
	}
	defer storage.Close()

	var results []*evaluate.Result

	for _, prNum := range prNumbers {
		fmt.Fprintf(os.Stderr, "Evaluating PR #%d...\n", prNum)

		input := evaluate.DiffInput{
			Source:   evaluate.DiffFromPR,
			PRNumber: prNum,
		}
		diff, err := input.GetDiff()
		if err != nil {
			fmt.Fprintf(os.Stderr, "  Failed to get diff for PR #%d: %v\n", prNum, err)
			continue
		}

		result, err := evaluate.EvaluateWithLLM(diff, caller, evalSource, "", fmt.Sprintf("pr-%d", prNum), "")
		if err != nil {
			fmt.Fprintf(os.Stderr, "  Evaluation failed for PR #%d: %v\n", prNum, err)
			continue
		}

		result.Timestamp = time.Now().UTC().Format(time.RFC3339)
		result.PRNumber = prNum

		if _, err := storage.Store(result); err != nil {
			fmt.Fprintf(os.Stderr, "  Failed to store result for PR #%d: %v\n", prNum, err)
		} else {
			fmt.Fprintf(os.Stderr, "  Stored result for PR #%d: %d/%d (%.0f%%)\n",
				prNum, result.TotalScore, result.MaxScore, result.Percentage())
		}

		results = append(results, result)
	}

	if len(results) == 0 {
		return fmt.Errorf("no results produced")
	}

	return writeBenchmarkSummary(results)
}

func runEvaluateTrend(cmd *cobra.Command, args []string) error {
	storage, err := evaluate.OpenStorage(evalDBPath)
	if err != nil {
		return fmt.Errorf("opening eval database: %w", err)
	}
	defer storage.Close()

	var sources []string
	if evalTrendSource != "" {
		sources = strings.Split(evalTrendSource, ",")
		for i := range sources {
			sources[i] = strings.TrimSpace(sources[i])
		}
	}

	switch strings.ToLower(evalFormat) {
	case "json":
		return writeTrendJSON(storage, sources)
	case "markdown", "md":
		return writeTrendMarkdown(storage, sources)
	default:
		return fmt.Errorf("unknown format: %s (use json or markdown)", evalFormat)
	}
}

type dryRunWorkItem struct {
	ID          string
	Title       string
	Description string
}

func dryRunItems() []dryRunWorkItem {
	return []dryRunWorkItem{
		{
			ID:    "dry-001",
			Title: "Add Slack notification provider with retry and rate limiting",
			Description: `Add a Slack webhook notification provider to internal/tracker/ (or a new internal/notifier/ package).
Create a Notifier interface with a Slack implementation. The Slack notifier must:
- Send messages via Slack incoming webhooks (POST to webhook URL)
- Retry up to 3 times with exponential backoff on 5xx or network errors
- Rate limit to 1 message per second (use a simple token bucket)
- Read webhook URL from config (URL field + URL_ENV for env var override, same pattern as TrackerConfig)
- Return structured errors: "slack: unexpected status 429 for webhook <name>" not "request failed"
- Include integration tests using httptest.NewServer
- Not couple to any other provider type in the codebase
The Notifier interface must be generic enough for future email/PagerDuty providers.`,
		},
		{
			ID:    "dry-002",
			Title: "Add droplet tagging with color labels",
			Description: `Add a tagging system for droplets so users can organize work by category.
Add a droplet_tags table (droplet_id TEXT, tag TEXT, color TEXT, PRIMARY KEY).
Add CLI commands: ct droplet tag <id> --tag <name> --color <hex>, ct droplet tags <id>, ct droplet untag <id> --tag <name>.
Tags must be stored in the existing SQLite database. The tag model must live in a new internal/tags package.
The CLI commands must be added alongside existing droplet subcommands in cmd/ct/.
Add tests for the tag store (CRUD operations).
Do NOT modify the Droplet struct in internal/cistern/ — tags are a separate concern joined by droplet_id.
Error messages: "tag <name> already exists on droplet <id>" not "duplicate key".`,
		},
		{
			ID:    "dry-003",
			Title: "Add webhook event bus for droplet state changes",
			Description: `Add an event bus that fires webhooks when droplets change state.
Create internal/events/ package with:
- An EventBus that accepts DropletEvent structs (droplet_id, old_state, new_state, timestamp)
- A WebhookSink that POSTs events as JSON to configured URLs
- A default LogSink that writes to stdout (for development)
- Sinks are registered at startup via config in cistern.yaml
The EventBus must not block the caller — dispatch is async.
WebhookSink must retry on failure (same pattern as tracker providers).
Add a table event_webhooks (id TEXT, url TEXT, event_types TEXT) for webhook registration.
Config via cistern.yaml under a new events: key.
Do NOT couple EventBus to any specific sink type.
Add tests with httptest.NewServer for WebhookSink.
Error messages: "webhook <url> returned 500 for event <type>" not "request failed".`,
		},
	}
}

func runEvaluateDryRun(cmd *cobra.Command, args []string) error {
	var item dryRunWorkItem

	if evalDryRunItem != "" {
		found := false
		for _, it := range dryRunItems() {
			if it.ID == evalDryRunItem {
				item = it
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("unknown work item %q — available: dry-001, dry-002, dry-003", evalDryRunItem)
		}
	} else if evalDryRunTitle != "" {
		item = dryRunWorkItem{
			Title:       evalDryRunTitle,
			Description: evalDryRunDesc,
		}
	} else {
		return fmt.Errorf("specify --item (dry-001, dry-002, dry-003) or --title and --description")
	}

	caller, err := resolveLLMCaller()
	if err != nil {
		return fmt.Errorf("resolving LLM caller: %w", err)
	}

	codebaseDir := evalDryRunCodebaseDir
	if codebaseDir == "" {
		codebaseDir = "."
	}

	cfg := evaluate.DryRunConfig{
		Caller:        caller,
		CodebaseDir:   codebaseDir,
		CataractaeDir: codebaseDir,
		Description:   item.Description,
		Title:         item.Title,
	}

	result, err := evaluate.DryRun(cfg)
	if err != nil {
		return fmt.Errorf("dry-run failed: %w", err)
	}

	if evalStore {
		storage, err := evaluate.OpenStorage(evalDBPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to open eval database: %v\n", err)
		} else {
			defer storage.Close()
			result.CisternResult.Timestamp = time.Now().UTC().Format(time.RFC3339)
			result.VibeResult.Timestamp = time.Now().UTC().Format(time.RFC3339)
			if _, err := storage.Store(result.CisternResult); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to store cistern result: %v\n", err)
			}
			if _, err := storage.Store(result.VibeResult); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to store vibe result: %v\n", err)
			}
			fmt.Fprintf(os.Stderr, "Results stored to evaluation database\n")
		}
	}

	return writeDryRunResult(result, item)
}

func writeDryRunResult(r *evaluate.DryRunResult, item dryRunWorkItem) error {
	switch strings.ToLower(evalFormat) {
	case "json":
		output := map[string]any{
			"item":               item,
			"architect_brief":    r.ArchitectBrief,
			"cistern_code_v1":   r.CisternCodeV1,
			"review_findings":    r.ReviewFindings,
			"cistern_code_v2":   r.CisternCodeV2,
			"qa_findings":        r.QAFindings,
			"cistern_code_final": r.CisternCodeFinal,
			"vibe_code":          r.VibeCode,
			"cistern_result":     r.CisternResult,
			"vibe_result":        r.VibeResult,
		}
		if r.CisternResultPreRev != nil {
			output["cistern_result_pre_review"] = r.CisternResultPreRev
		}
		data, err := json.MarshalIndent(output, "", "  ")
		if err != nil {
			return fmt.Errorf("marshaling result: %w", err)
		}
		if evalOutput != "" {
			return os.WriteFile(evalOutput, data, 0644)
		}
		fmt.Println(string(data))
		return nil

	case "markdown", "md":
		var sb strings.Builder
		sb.WriteString("# Dry-Run: Full Pipeline Effectiveness Test\n\n")
		sb.WriteString(fmt.Sprintf("## Work Item: %s\n\n%s\n\n", item.Title, item.Description))

		sb.WriteString("## Pipeline Comparison (after review + QA)\n\n")
		sb.WriteString(evaluate.FormatComparison(r.CisternResult, r.VibeResult))
		sb.WriteString("\n\n---\n\n")

		if r.CisternResultPreRev != nil {
			sb.WriteString("## Review Impact (before vs after review)\n\n")
			sb.WriteString(evaluate.FormatComparison(r.CisternResultPreRev, r.CisternResult))
			sb.WriteString("\n\n---\n\n")
		}

		sb.WriteString("## Stages\n\n")
		sb.WriteString("| Stage | Output Length |\n")
		sb.WriteString("|---|---|\n")
		sb.WriteString(fmt.Sprintf("| Architect brief | %d chars |\n", len(r.ArchitectBrief)))
		sb.WriteString(fmt.Sprintf("| Implementer v1 | %d chars |\n", len(r.CisternCodeV1)))
		if r.ReviewFindings != "" {
			sb.WriteString(fmt.Sprintf("| Review findings | %d chars |\n", len(r.ReviewFindings)))
		}
		sb.WriteString(fmt.Sprintf("| Implementer v2 (post-review) | %d chars |\n", len(r.CisternCodeV2)))
		if r.QAFindings != "" {
			sb.WriteString(fmt.Sprintf("| QA findings | %d chars |\n", len(r.QAFindings)))
		}
		sb.WriteString(fmt.Sprintf("| Final cistern code | %d chars |\n", len(r.CisternCodeFinal)))
		sb.WriteString(fmt.Sprintf("| Vibe-coded code | %d chars |\n", len(r.VibeCode)))
		sb.WriteString("\n---\n\n")

		sb.WriteString("## Review Findings\n\n")
		reviewLines := strings.Split(r.ReviewFindings, "\n")
		reviewPreview := reviewLines
		if len(reviewLines) > 40 {
			reviewPreview = append(reviewLines[:40], "...(truncated)")
		}
		for _, line := range reviewPreview {
			sb.WriteString(line + "\n")
		}

		if evalOutput != "" {
			return os.WriteFile(evalOutput, []byte(sb.String()), 0644)
		}
		fmt.Println(sb.String())
		return nil

	default:
		return fmt.Errorf("unknown format: %s (use json or markdown)", evalFormat)
	}
}

func resolveBenchmarkPRs() ([]int, error) {
	if evalBenchPRs != "" {
		var nums []int
		for _, s := range strings.Split(evalBenchPRs, ",") {
			s = strings.TrimSpace(s)
			var n int
			if _, err := fmt.Sscanf(s, "%d", &n); err != nil {
				return nil, fmt.Errorf("invalid PR number %q: %w", s, err)
			}
			nums = append(nums, n)
		}
		return nums, nil
	}

	if evalBenchMergedSince != "" {
		return fetchMergedPRs(evalBenchMergedSince)
	}

	return nil, nil
}

func fetchMergedPRs(since string) ([]int, error) {
	args := []string{"pr", "list", "--state", "merged", "--json", "number,title,mergedAt"}
	if since != "" {
		args = append(args, "--search", fmt.Sprintf("merged:>=%s", since))
	}
	out, err := exec.Command("gh", args...).Output()
	if err != nil {
		return nil, fmt.Errorf("gh pr list: %w", err)
	}

	var prs []struct {
		Number   int    `json:"number"`
		Title    string `json:"title"`
		MergedAt string `json:"mergedAt"`
	}
	if err := json.Unmarshal(out, &prs); err != nil {
		return nil, fmt.Errorf("parse gh pr list: %w", err)
	}

	nums := make([]int, len(prs))
	for i, pr := range prs {
		nums[i] = pr.Number
	}
	return nums, nil
}

func storeResult(r *evaluate.Result) error {
	storage, err := evaluate.OpenStorage(evalDBPath)
	if err != nil {
		return err
	}
	defer storage.Close()
	_, err = storage.Store(r)
	return err
}

func resolveLLMCaller() (evaluate.Caller, error) {
	caller, err := evaluate.AutoCaller(evalModel)
	if err != nil {
		return nil, fmt.Errorf("no LLM available: %w", err)
	}
	return caller, nil
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

func writeBenchmarkSummary(results []*evaluate.Result) error {
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

		totalScore := 0
		maxScore := 0

		for _, r := range results {
			prLabel := ""
			if r.PRNumber > 0 {
				prLabel = fmt.Sprintf(" (PR #%d)", r.PRNumber)
			}
			sb.WriteString(fmt.Sprintf("## %s%s: %d/%d (%.0f%%)\n\n",
				r.Source, prLabel, r.TotalScore, r.MaxScore, r.Percentage()))

			sb.WriteString("| Dimension | Score | Evidence |\n")
			sb.WriteString("|---|---|---|\n")
			for _, s := range r.Scores {
				sb.WriteString(fmt.Sprintf("| %s | %d/5 | %s |\n", s.Dimension, s.Score, s.Evidence))
			}
			sb.WriteString("\n---\n\n")

			totalScore += r.TotalScore
			maxScore += r.MaxScore
		}

		if maxScore > 0 {
			sb.WriteString("## Aggregate\n\n")
			sb.WriteString(fmt.Sprintf("| Source | Total | Max | Percentage |\n"))
			sb.WriteString(fmt.Sprintf("|---|---|---|---|\n"))
			sb.WriteString(fmt.Sprintf("| %s | %d | %d | %.0f%% |\n",
				evalSource, totalScore, maxScore, float64(totalScore)/float64(maxScore)*100))
		}

		if evalOutput != "" {
			return os.WriteFile(evalOutput, []byte(sb.String()), 0644)
		}
		fmt.Println(sb.String())
		return nil

	default:
		return fmt.Errorf("unknown format: %s (use json or markdown)", evalFormat)
	}
}

func writeTrendJSON(storage *evaluate.Storage, sources []string) error {
	points, err := storage.Trend(sources, evalTrendSince)
	if err != nil {
		return fmt.Errorf("querying trend: %w", err)
	}
	if len(points) == 0 {
		fmt.Println("[]")
		return nil
	}

	avgs, err := storage.AverageByDimension(sources, evalTrendSince)
	if err != nil {
		return fmt.Errorf("querying averages: %w", err)
	}

	output := map[string]any{
		"trend":    points,
		"averages": avgs,
	}
	data, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling trend: %w", err)
	}
	if evalOutput != "" {
		return os.WriteFile(evalOutput, data, 0644)
	}
	fmt.Println(string(data))
	return nil
}

func writeTrendMarkdown(storage *evaluate.Storage, sources []string) error {
	points, err := storage.Trend(sources, evalTrendSince)
	if err != nil {
		return fmt.Errorf("querying trend: %w", err)
	}
	if len(points) == 0 {
		fmt.Println("No evaluation results found.")
		return nil
	}

	avgs, err := storage.AverageByDimension(sources, evalTrendSince)
	if err != nil {
		return fmt.Errorf("querying averages: %w", err)
	}

	var sb strings.Builder
	sb.WriteString("# Evaluation Trend\n\n")

	sb.WriteString("| Date | Source | Score | Max | % |\n")
	sb.WriteString("|---|---|---|---|---|\n")
	for _, p := range points {
		date := p.EvaluatedAt
		if len(date) > 10 {
			date = date[:10]
		}
		sb.WriteString(fmt.Sprintf("| %s | %s | %d | %d | %.0f%% |\n",
			date, p.Source, p.TotalScore, p.MaxScore, p.Percentage))
	}

	if len(avgs) > 0 {
		sb.WriteString("\n## Average Scores by Dimension\n\n")
		sb.WriteString("| Dimension | Avg Score | Out of |\n")
		sb.WriteString("|---|---|---|\n")
		for _, d := range evaluate.AllDimensions() {
			if avg, ok := avgs[d]; ok {
				sb.WriteString(fmt.Sprintf("| %s | %.1f | 5 |\n", d, avg))
			}
		}

		totalAvg := 0.0
		for _, avg := range avgs {
			totalAvg += avg
		}
		maxAvg := float64(len(evaluate.AllDimensions())) * 5.0
		sb.WriteString(fmt.Sprintf("\n**Overall average:** %.1f / %.0f (%.0f%%)\n",
			totalAvg, maxAvg, totalAvg/maxAvg*100))
	}

	if evalOutput != "" {
		return os.WriteFile(evalOutput, []byte(sb.String()), 0644)
	}
	fmt.Println(sb.String())
	return nil
}

func formatMarkdown(r *evaluate.Result) string {
	var sb strings.Builder

	sb.WriteString("# Code Quality Evaluation\n\n")
	sb.WriteString(fmt.Sprintf("- **Source:** %s\n", r.Source))
	if r.Ticket != "" {
		sb.WriteString(fmt.Sprintf("- **Ticket:** %s\n", r.Ticket))
	}
	if r.PRNumber > 0 {
		sb.WriteString(fmt.Sprintf("- **PR:** #%d\n", r.PRNumber))
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