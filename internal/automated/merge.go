package automated

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// prView is used to parse gh pr view --json state output.
type prView struct {
	State string `json:"state"`
}

// Merge squash-merges a PR and verifies the merged state.
// The PR URL is read from bc.Metadata[MetaPRURL].
func (e *Executor) Merge(ctx context.Context, bc BeadContext) (*StepOutcome, error) {
	prURL := metaString(bc.Metadata, MetaPRURL)
	if prURL == "" {
		return &StepOutcome{
			Result: ResultFail,
			Notes:  "no pr_url in bead metadata",
		}, nil
	}

	// Check current state first — treat already-merged as an idempotent success.
	state, err := e.getPRState(ctx, bc.WorkDir, prURL)
	if err != nil {
		return &StepOutcome{
			Result: ResultFail,
			Notes:  fmt.Sprintf("check PR state failed: %s", err),
		}, nil
	}
	if strings.ToUpper(state) == "MERGED" {
		return &StepOutcome{
			Result: ResultPass,
			Notes:  fmt.Sprintf("PR already merged: %s", prURL),
		}, nil
	}

	out, err := e.ExecFn(ctx, bc.WorkDir, "gh", "pr", "merge", prURL, "--squash", "--delete-branch")
	if err != nil {
		// Re-check state — gh pr merge can exit non-zero even on success if the
		// branch was already deleted or the merge raced with another process.
		state2, _ := e.getPRState(ctx, bc.WorkDir, prURL)
		if strings.ToUpper(state2) == "MERGED" {
			return &StepOutcome{
				Result: ResultPass,
				Notes:  fmt.Sprintf("PR merged (verified after non-zero exit): %s", prURL),
			}, nil
		}
		return &StepOutcome{
			Result: ResultFail,
			Notes:  fmt.Sprintf("gh pr merge failed: %s: %s", err, out),
		}, nil
	}

	return &StepOutcome{
		Result: ResultPass,
		Notes:  fmt.Sprintf("PR merged: %s", prURL),
	}, nil
}

func (e *Executor) getPRState(ctx context.Context, dir, prURL string) (string, error) {
	out, err := e.ExecFn(ctx, dir, "gh", "pr", "view", prURL, "--json", "state")
	if err != nil {
		return "", fmt.Errorf("%w: %s", err, out)
	}

	var pv prView
	if err := json.Unmarshal(out, &pv); err != nil {
		return "", fmt.Errorf("parse state: %w", err)
	}
	return pv.State, nil
}
