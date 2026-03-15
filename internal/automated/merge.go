package automated

import (
	"context"
	"fmt"
	"strings"
)

// Merge squash-merges the PR for this item. If gh pr merge exits 0, the item
// is done — no post-merge verification needed. The scheduler marks it closed.
func (e *Executor) Merge(ctx context.Context, bc BeadContext) (*StepOutcome, error) {
	prURL := metaString(bc.Metadata, MetaPRURL)
	if prURL == "" {
		return &StepOutcome{
			Result: ResultFail,
			Notes:  "no pr_url in bead metadata",
		}, nil
	}

	out, err := e.ExecFn(ctx, bc.WorkDir, "gh", "pr", "merge", prURL, "--squash", "--delete-branch")
	if err != nil {
		// Check if the PR was already merged (auto-merge fired before us).
		stateOut, stateErr := e.ExecFn(ctx, bc.WorkDir, "gh", "pr", "view", prURL, "--json", "state", "--jq", ".state")
		if stateErr == nil && strings.TrimSpace(string(stateOut)) == "MERGED" {
			return &StepOutcome{
				Result: ResultPass,
				Notes:  fmt.Sprintf("already merged: %s", prURL),
			}, nil
		}
		return &StepOutcome{
			Result: ResultFail,
			Notes:  fmt.Sprintf("gh pr merge failed: %s: %s", err, out),
		}, nil
	}

	return &StepOutcome{
		Result: ResultPass,
		Notes:  fmt.Sprintf("merged: %s", prURL),
	}, nil
}
