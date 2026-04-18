package evaluate

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type DryRunConfig struct {
	Caller        Caller
	CodebaseDir   string
	CataractaeDir string
	Description   string
	Title         string
}

type DryRunResult struct {
	Config              DryRunConfig
	ArchitectBrief      string
	CisternCodeV1       string
	ReviewFindings      string
	CisternCodeV2       string
	QAFindings          string
	CisternCodeFinal    string
	VibeCode            string
	CisternResult       *Result
	VibeResult          *Result
	CisternResultPreRev *Result
}

func (c DryRunConfig) loadRole(role string) string {
	local := filepath.Join(c.CodebaseDir, "cataractae", role, "INSTRUCTIONS.md")
	if data, err := os.ReadFile(local); err == nil && len(data) > 0 {
		return string(data)
	}
	if c.CataractaeDir != "" {
		fallback := filepath.Join(c.CataractaeDir, "cataractae", role, "INSTRUCTIONS.md")
		if data, err := os.ReadFile(fallback); err == nil && len(data) > 0 {
			return string(data)
		}
	}
	return ""
}

func DryRun(cfg DryRunConfig) (*DryRunResult, error) {
	toolCaller, ok := cfg.Caller.(*APICaller)
	if !ok {
		return nil, fmt.Errorf("dry-run requires APICaller (Ollama backend)")
	}
	if cfg.CodebaseDir == "" {
		cfg.CodebaseDir = "."
	}
	tools := codebaseTools()
	result := &DryRunResult{Config: cfg}

	architectInstr := cfg.loadRole("architect")
	implementerInstr := cfg.loadRole("implementer")
	reviewerInstr := cfg.loadRole("reviewer")
	qaInstr := cfg.loadRole("qa")

	// Stage 1: Architect (tools: yes — needs to explore the codebase)
	fmt.Fprintf(os.Stderr, "Step 1/8: Architect producing design brief...\n")
	archPrompt := architectInstr + "\n\n## Task\n\nTitle: " + cfg.Title + "\n\n" + cfg.Description
	brief, err := toolCaller.CallWithTools(archPrompt, tools, cfg.CodebaseDir)
	if err != nil {
		return nil, fmt.Errorf("architect: %w", err)
	}
	result.ArchitectBrief = brief

	// Stage 2: Implementer (tools: yes — needs to read patterns)
	fmt.Fprintf(os.Stderr, "Step 2/8: Implementer producing code...\n")
	briefForImpl := brief
	if len(briefForImpl) > 8000 {
		briefForImpl = briefForImpl[:8000] + "\n... (brief truncated)"
	}
	implPrompt := implementerInstr + "\n\n## DESIGN_BRIEF.md (mandatory contract)\n\n" + briefForImpl +
		"\n\n## Task\n\nTitle: " + cfg.Title + "\n\n" + cfg.Description
	cisternV1, err := toolCaller.CallWithTools(implPrompt, tools, cfg.CodebaseDir)
	if err != nil {
		return nil, fmt.Errorf("implementer pass 1: %w", err)
	}
	result.CisternCodeV1 = cisternV1

	// Stage 3: Reviewer (tools: NO — the code only exists in conversation, not on disk)
	fmt.Fprintf(os.Stderr, "Step 3/8: Reviewer examining code...\n")
	revPrompt := reviewerInstr +
		"\n\n## Code to review (newly written — not yet on disk)\n\n```\n" + cisternV1 +
		"\n```\n\nIMPORTANT: The code above was just written by the implementer. " +
		"It does not exist on the filesystem yet — review it from the text above only. " +
		"Do not use Read/Glob/Grep to try to find it on disk."
	review, err := toolCaller.Call(revPrompt)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: reviewer failed: %v\n", err)
		review = ""
	}
	result.ReviewFindings = review

	// Stage 4: Fix review findings (tools: yes — may need to check patterns)
	cisternV2 := cisternV1
	passedReview := review != "" && strings.Contains(strings.ToUpper(review), "VERDICT: PASS")
	if !passedReview && review != "" {
		fmt.Fprintf(os.Stderr, "Step 4/8: Fixing review findings...\n")
		fixPrompt := implementerInstr +
			"\n\n## Code review findings to fix\n\n" + review +
			"\n\n## Current code\n\n```\n" + cisternV1 +
			"\n```\n\nFix ALL findings above. Produce the complete corrected code " +
			"using the same --- FILE: path --- format."
		fixed, err := toolCaller.CallWithTools(fixPrompt, tools, cfg.CodebaseDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: review fix failed: %v\n", err)
		} else if len(fixed) > 200 {
			cisternV2 = fixed
		}
	} else {
		fmt.Fprintf(os.Stderr, "Step 4/8: Review passed.\n")
	}
	result.CisternCodeV2 = cisternV2

	// Stage 5: QA (tools: NO — same reason, code not on disk)
	fmt.Fprintf(os.Stderr, "Step 5/8: QA examining code...\n")
	qaReviewPrompt := qaInstr +
		"\n\n## Code to verify (newly written — not yet on disk)\n\n```\n" + cisternV2 +
		"\n```\n\nIMPORTANT: The code above was just written. It does not exist on the filesystem yet — verify it from the text above only."
	qaReview, err := toolCaller.Call(qaReviewPrompt)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: QA failed: %v\n", err)
		qaReview = ""
	}
	result.QAFindings = qaReview

	// Stage 6: Fix QA findings
	cisternFinal := cisternV2
	passedQA := qaReview != "" && strings.Contains(strings.ToUpper(qaReview), "VERDICT: PASS")
	if !passedQA && qaReview != "" {
		fmt.Fprintf(os.Stderr, "Step 6/8: Fixing QA findings...\n")
		fixPrompt := implementerInstr +
			"\n\n## QA findings to fix\n\n" + qaReview +
			"\n\n## Current code\n\n```\n" + cisternV2 +
			"\n```\n\nFix ALL findings above. Produce the complete corrected code " +
			"using the same --- FILE: path --- format."
		fixed, err := toolCaller.CallWithTools(fixPrompt, tools, cfg.CodebaseDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: QA fix failed: %v\n", err)
		} else if len(fixed) > 200 {
			cisternFinal = fixed
		}
	} else {
		fmt.Fprintf(os.Stderr, "Step 6/8: QA passed.\n")
	}
	result.CisternCodeFinal = cisternFinal

	// Stage 7: Vibe-coded (tools: yes — needs codebase access)
	fmt.Fprintf(os.Stderr, "Step 7/8: Vibe-coded implementation...\n")
	vibePrompt := "You are a software engineer. Implement the following feature for this Go codebase.\n\n" +
		"Explore the codebase using the Read, Glob, and Grep tools to understand patterns, then implement.\n\n" +
		"## Task\n\nTitle: " + cfg.Title + "\n\n" + cfg.Description
	vibeCode, err := toolCaller.CallWithTools(vibePrompt, tools, cfg.CodebaseDir)
	if err != nil {
		return nil, fmt.Errorf("vibe-coded: %w", err)
	}
	result.VibeCode = vibeCode

	// Stage 8: Evaluate both
	fmt.Fprintf(os.Stderr, "Step 8/8: Evaluating outputs...\n")
	cEval, err := cfg.Caller.Call(ScoringPrompt() + "\n\n## Code (Cistern — post review+QA):\n\n```\n" + cisternFinal + "\n```")
	if err != nil {
		return nil, fmt.Errorf("cistern eval: %w", err)
	}
	cResult, err := ParseEvaluationResult(cEval)
	if err != nil {
		return nil, fmt.Errorf("parse cistern: %w", err)
	}
	cResult.Source = "cistern-dry-run"
	cResult.Model = cfg.Caller.ModelName()
	result.CisternResult = cResult

	if len(cisternV1) > 200 && len(cisternV1) != len(cisternFinal) {
		preEval, err := cfg.Caller.Call(ScoringPrompt() + "\n\n## Code (Cistern — BEFORE review):\n\n```\n" + cisternV1 + "\n```")
		if err == nil {
			preResult, err := ParseEvaluationResult(preEval)
			if err == nil {
				preResult.Source = "cistern-pre-review"
				preResult.Model = cfg.Caller.ModelName()
				result.CisternResultPreRev = preResult
			}
		}
	}

	vEval, err := cfg.Caller.Call(ScoringPrompt() + "\n\n## Code (vibe-coded):\n\n```\n" + vibeCode + "\n```")
	if err != nil {
		return nil, fmt.Errorf("vibe eval: %w", err)
	}
	vResult, err := ParseEvaluationResult(vEval)
	if err != nil {
		return nil, fmt.Errorf("parse vibe: %w", err)
	}
	vResult.Source = "vibe-coded-dry-run"
	vResult.Model = cfg.Caller.ModelName()
	result.VibeResult = vResult

	return result, nil
}