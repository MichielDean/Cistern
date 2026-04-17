package evaluate

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// DiffSource represents where the diff comes from.
type DiffSource int

const (
	DiffFromBranches DiffSource = iota
	DiffFromPR
	DiffFromRaw
)

// DiffInput specifies what to evaluate.
type DiffInput struct {
	Source     DiffSource
	BaseBranch string
	HeadBranch string
	PRNumber   int
	RawDiff    string
	WorkDir    string
}

// GetDiff returns the diff content based on the source.
func (d DiffInput) GetDiff() (string, error) {
	switch d.Source {
	case DiffFromBranches:
		return d.getBranchDiff()
	case DiffFromPR:
		return d.getPRDiff()
	case DiffFromRaw:
		return d.RawDiff, nil
	default:
		return "", fmt.Errorf("unknown diff source: %d", d.Source)
	}
}

func (d DiffInput) getBranchDiff() (string, error) {
	if d.BaseBranch == "" {
		d.BaseBranch = "main"
	}
	if d.HeadBranch == "" {
		return "", fmt.Errorf("head branch is required for branch diff")
	}
	mergeBase, err := exec.Command("git", "merge-base", d.HeadBranch, d.BaseBranch).Output()
	if err != nil {
		return "", fmt.Errorf("git merge-base: %w", err)
	}
	cmd := exec.Command("git", "diff", strings.TrimSpace(string(mergeBase))+".."+d.HeadBranch)
	if d.WorkDir != "" {
		cmd.Dir = d.WorkDir
	}
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git diff: %w", err)
	}
	return string(out), nil
}

func (d DiffInput) getPRDiff() (string, error) {
	if d.PRNumber == 0 {
		return "", fmt.Errorf("PR number is required for PR diff")
	}
	cmd := exec.Command("gh", "pr", "diff", fmt.Sprintf("%d", d.PRNumber))
	if d.WorkDir != "" {
		cmd.Dir = d.WorkDir
	}
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("gh pr diff: %w", err)
	}
	return string(out), nil
}

// Evaluate is the placeholder for evaluating without an LLM.
// Use EvaluateWithLLM for actual LLM-based evaluation.
func Evaluate(diff string, model string, source string, ticket string, branch string, commit string) (*Result, error) {
	if diff == "" {
		return nil, fmt.Errorf("diff is empty -- nothing to evaluate")
	}

	result := &Result{
		Source:     source,
		Ticket:     ticket,
		Branch:     branch,
		Commit:     commit,
		Model:      model,
		Scores:     []Score{},
		TotalScore: 0,
		MaxScore:   len(AllDimensions()) * 5,
		Notes:      "Evaluation not yet implemented -- rubric and scoring structure is defined",
	}

	return result, nil
}

// LLMCaller invokes an LLM in non-interactive mode and returns the response.
type LLMCaller struct {
	Command          string
	Args            []string
	PrintFlag       string
	PromptFlag      string
	ModelFlag       string
	Model           string
	AllowedToolsFlag string
	WorkDir         string
}

// NewLLMCaller creates a caller from a provider preset.
// It uses the non-interactive mode (--print -p) for single-shot evaluation.
func NewLLMCaller(command string, args []string, printFlag, promptFlag, modelFlag, model, allowedToolsFlag, workDir string) *LLMCaller {
	return &LLMCaller{
		Command:          command,
		Args:            args,
		PrintFlag:       printFlag,
		PromptFlag:      promptFlag,
		ModelFlag:       modelFlag,
		Model:           model,
		AllowedToolsFlag: allowedToolsFlag,
		WorkDir:         workDir,
	}
}

// Call sends the prompt to the LLM and returns the response text.
func (c *LLMCaller) Call(prompt string) (string, error) {
	parts := []string{c.Command}
	parts = append(parts, c.Args...)

	if c.Model != "" && c.ModelFlag != "" {
		parts = append(parts, c.ModelFlag, c.Model)
	}

	if c.PrintFlag != "" {
		parts = append(parts, c.PrintFlag)
	}

	if c.AllowedToolsFlag != "" {
		parts = append(parts, c.AllowedToolsFlag, "Glob,Grep,Read")
	}

	if c.PromptFlag != "" {
		parts = append(parts, c.PromptFlag, prompt)
	} else {
		return "", fmt.Errorf("no prompt flag configured for LLM caller")
	}

	cmd := exec.Command(parts[0], parts[1:]...)
	if c.WorkDir != "" {
		cmd.Dir = c.WorkDir
	}

	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("LLM call failed (exit %d): %s", exitErr.ExitCode(), string(exitErr.Stderr))
		}
		return "", fmt.Errorf("LLM call failed: %w", err)
	}

	return strings.TrimSpace(string(out)), nil
}

// EvaluateWithLLM uses an LLM to score the given diff against the rubric.
func EvaluateWithLLM(diff string, caller *LLMCaller, source string, ticket string, branch string, commit string) (*Result, error) {
	if diff == "" {
		return nil, fmt.Errorf("diff is empty -- nothing to evaluate")
	}
	if caller == nil {
		return nil, fmt.Errorf("LLM caller is required")
	}

	prompt := ScoringPrompt() + "\n\n## Diff to evaluate:\n\n```\n" + diff + "\n```"

	response, err := caller.Call(prompt)
	if err != nil {
		return nil, fmt.Errorf("LLM evaluation call failed: %w", err)
	}

	result, err := ParseEvaluationResult(response)
	if err != nil {
		return nil, fmt.Errorf("parsing LLM response: %w\n\nRaw response:\n%s", err, response)
	}

	result.Source = source
	result.Ticket = ticket
	result.Branch = branch
	result.Commit = commit
	result.Model = caller.Model

	return result, nil
}

// FormatComparison produces a markdown comparison of two evaluation results.
func FormatComparison(cisternResult, vibeResult *Result) string {
	var sb strings.Builder

	sb.WriteString("# Pipeline Effectiveness Comparison\n\n")

	dims := AllDimensions()
	sb.WriteString("| Dimension | Cistern | Vibe-coded | Delta |\n")
	sb.WriteString("|---|---|---|---|\n")

	cisternScores := scoresByDimension(cisternResult)
	vibeScores := scoresByDimension(vibeResult)

	for _, d := range dims {
		cs := cisternScores[d]
		vs := vibeScores[d]
		delta := cs - vs
		deltaStr := fmt.Sprintf("%+d", delta)
		if delta > 0 {
			deltaStr = fmt.Sprintf("**+%d**", delta)
		} else if delta < 0 {
			deltaStr = fmt.Sprintf("**%d**", delta)
		}
		sb.WriteString(fmt.Sprintf("| %s | %d/5 | %d/5 | %s |\n", d, cs, vs, deltaStr))
	}

	cs := cisternResult.TotalScore
	vs := vibeResult.TotalScore
	delta := cs - vs
	sb.WriteString(fmt.Sprintf("\n| **Total** | **%d/%d** | **%d/%d** | **%+d** |\n",
		cs, cisternResult.MaxScore, vs, vibeResult.MaxScore, delta))

	sb.WriteString(fmt.Sprintf("\nCistern: %.0f%% | Vibe-coded: %.0f%%\n",
		cisternResult.Percentage(), vibeResult.Percentage()))

	if cisternResult.Notes != "" || vibeResult.Notes != "" {
		sb.WriteString("\n## Cistern Notes\n\n")
		sb.WriteString(cisternResult.Notes + "\n")
		sb.WriteString("\n## Vibe-coded Notes\n\n")
		sb.WriteString(vibeResult.Notes + "\n")
	}

	return sb.String()
}

func scoresByDimension(r *Result) map[Dimension]int {
	m := make(map[Dimension]int)
	for _, s := range r.Scores {
		m[s.Dimension] = s.Score
	}
	return m
}

// MarshalForStorage serializes a Result to JSON for persistent storage.
func MarshalForStorage(r *Result) ([]byte, error) {
	return json.MarshalIndent(r, "", "  ")
}

// ParseEvaluationResult parses the LLM's JSON response into a Result.
func ParseEvaluationResult(body string) (*Result, error) {
	var result Result
	if err := json.Unmarshal([]byte(body), &result); err != nil {
		return nil, fmt.Errorf("parsing evaluation result: %w", err)
	}

	validDims := make(map[Dimension]bool)
	for _, d := range AllDimensions() {
		validDims[d] = true
	}

	totalScore := 0
	for _, s := range result.Scores {
		if !validDims[s.Dimension] {
			return nil, fmt.Errorf("unknown dimension: %s", s.Dimension)
		}
		if s.Score < 0 || s.Score > 5 {
			return nil, fmt.Errorf("score for %s must be 0-5, got %d", s.Dimension, s.Score)
		}
		totalScore += s.Score
	}

	result.TotalScore = totalScore
	result.MaxScore = len(AllDimensions()) * 5

	return &result, nil
}