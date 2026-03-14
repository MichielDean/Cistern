package runner

import (
	"context"
	"fmt"

	"github.com/MichielDean/bullet-farm/internal/automated"
	"github.com/MichielDean/bullet-farm/internal/queue"
	"github.com/MichielDean/bullet-farm/internal/scheduler"
	"github.com/MichielDean/bullet-farm/internal/workflow"
)

// Adapter wraps Runner instances to implement scheduler.StepRunner.
type Adapter struct {
	runners  map[string]*Runner // keyed by repo name
	executor *automated.Executor
}

// NewAdapter creates an Adapter with a Runner for each configured repo.
func NewAdapter(configs []workflow.RepoConfig, workflows map[string]*workflow.Workflow, queueClients map[string]*queue.Client) (*Adapter, error) {
	runners := make(map[string]*Runner, len(configs))
	for _, repo := range configs {
		wf, ok := workflows[repo.Name]
		if !ok {
			return nil, fmt.Errorf("adapter: no workflow for repo %q", repo.Name)
		}
		client, ok := queueClients[repo.Name]
		if !ok {
			return nil, fmt.Errorf("adapter: no queue client for repo %q", repo.Name)
		}
		r, err := New(Config{
			Repo:        repo,
			Workflow:    wf,
			QueueClient: client,
		})
		if err != nil {
			return nil, fmt.Errorf("adapter: runner for %q: %w", repo.Name, err)
		}
		runners[repo.Name] = r
	}
	return &Adapter{
		runners:  runners,
		executor: automated.New(),
	}, nil
}

// Run implements scheduler.StepRunner by delegating to the appropriate Runner.
func (a *Adapter) Run(ctx context.Context, req scheduler.StepRequest) (*scheduler.Outcome, error) {
	// Automated steps are handled by the automated executor, not Claude.
	if req.Step.Type == workflow.StepTypeAutomated {
		return a.runAutomated(ctx, req), nil
	}

	r, ok := a.runners[req.RepoConfig.Name]
	if !ok {
		return nil, fmt.Errorf("adapter: no runner for repo %q", req.RepoConfig.Name)
	}

	// Find the worker by name.
	var worker *Worker
	r.mu.Lock()
	for _, w := range r.workers {
		if w.Name == req.WorkerName {
			worker = w
			break
		}
	}
	r.mu.Unlock()

	if worker == nil {
		return nil, fmt.Errorf("adapter: worker %q not found in repo %q", req.WorkerName, req.RepoConfig.Name)
	}

	step := req.Step
	outcome, err := r.RunStep(worker, req.Item, &step)
	if err != nil {
		return nil, err
	}

	return convertOutcome(outcome), nil
}

// runAutomated dispatches an automated step through the automated executor.
func (a *Adapter) runAutomated(ctx context.Context, req scheduler.StepRequest) *scheduler.Outcome {
	bc := automated.BeadContext{
		ID:          req.Item.ID,
		Title:       req.Item.Title,
		Description: req.Item.Description,
	}
	result := a.executor.RunStep(ctx, req.Step.Name, bc)
	return &scheduler.Outcome{
		Result: scheduler.Result(result.Result),
		Notes:  result.Notes,
	}
}

// convertOutcome maps a runner.Outcome to a scheduler.Outcome.
func convertOutcome(ro *Outcome) *scheduler.Outcome {
	so := &scheduler.Outcome{
		Result: scheduler.Result(ro.Result),
		Notes:  ro.Notes,
	}
	if len(ro.Annotations) > 0 {
		for k, v := range ro.Annotations {
			so.Annotations = append(so.Annotations, scheduler.Annotation{
				File:    k,
				Comment: v,
			})
		}
	}
	return so
}
