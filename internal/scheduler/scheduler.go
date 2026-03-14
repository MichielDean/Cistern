// Package scheduler implements the core scheduling loop for bullet farm.
//
// It polls the beads work queue for each configured repo, assigns work to
// idle named workers, runs workflow steps via an injected StepRunner, reads
// outcomes, and routes to the next step via deterministic workflow rules.
// No AI in the scheduler — pure state machine.
package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/MichielDean/bullet-farm/internal/bd"
	"github.com/MichielDean/bullet-farm/internal/workflow"
)

// BeadClient is the interface for interacting with the beads work queue.
// *bd.Client satisfies this interface.
type BeadClient interface {
	GetReady(rig string) (*bd.Bead, error)
	UpdateStep(id, step string) error
	IncrementAttempts(id, step string) (int, error)
	AttachNotes(id, fromStep, notes string) error
	GetNotes(id string) ([]bd.StepNote, error)
	Escalate(id, reason string) error
}

// StepRunner executes a single workflow step.
// The scheduler calls Run and reads the returned Outcome to decide routing.
// Implementations handle agent spawning, automated commands, etc.
type StepRunner interface {
	Run(ctx context.Context, req StepRequest) (*Outcome, error)
}

// StepRequest contains everything needed to execute a workflow step.
type StepRequest struct {
	Bead       *bd.Bead
	Step       workflow.WorkflowStep
	Workflow   *workflow.Workflow
	RepoConfig workflow.RepoConfig
	WorkerName string
	Notes      []bd.StepNote // context from previous steps
}

// Scheduler is the core loop that polls for work, assigns it to workers,
// and routes outcomes through workflow steps.
type Scheduler struct {
	config       workflow.FarmConfig
	workflows    map[string]*workflow.Workflow
	clients      map[string]BeadClient
	pools        map[string]*WorkerPool
	runner       StepRunner
	logger       *slog.Logger
	pollInterval time.Duration
}

// Option configures a Scheduler.
type Option func(*Scheduler)

// WithLogger sets the logger.
func WithLogger(l *slog.Logger) Option {
	return func(s *Scheduler) { s.logger = l }
}

// WithPollInterval sets how often the scheduler polls for work.
func WithPollInterval(d time.Duration) Option {
	return func(s *Scheduler) { s.pollInterval = d }
}

// New creates a Scheduler from a FarmConfig.
// Workflows are loaded from each RepoConfig.WorkflowPath.
// Each repo gets its own bd.Client and WorkerPool.
func New(config workflow.FarmConfig, runner StepRunner, opts ...Option) (*Scheduler, error) {
	s := &Scheduler{
		config:       config,
		workflows:    make(map[string]*workflow.Workflow),
		clients:      make(map[string]BeadClient),
		pools:        make(map[string]*WorkerPool),
		runner:       runner,
		logger:       slog.Default(),
		pollInterval: 10 * time.Second,
	}
	for _, o := range opts {
		o(s)
	}

	for _, repo := range config.Repos {
		wf, err := workflow.ParseWorkflow(repo.WorkflowPath)
		if err != nil {
			return nil, fmt.Errorf("load workflow for %s: %w", repo.Name, err)
		}
		s.workflows[repo.Name] = wf
		s.clients[repo.Name] = bd.NewClient("", repo.BdPrefix)

		names := repo.Names
		if len(names) == 0 {
			names = defaultWorkerNames(repo.Workers)
		}
		s.pools[repo.Name] = NewWorkerPool(repo.Name, names)
	}

	return s, nil
}

// NewFromParts creates a Scheduler with pre-built components (for testing).
func NewFromParts(
	config workflow.FarmConfig,
	workflows map[string]*workflow.Workflow,
	clients map[string]BeadClient,
	runner StepRunner,
	opts ...Option,
) *Scheduler {
	s := &Scheduler{
		config:       config,
		workflows:    workflows,
		clients:      clients,
		pools:        make(map[string]*WorkerPool),
		runner:       runner,
		logger:       slog.Default(),
		pollInterval: 10 * time.Second,
	}
	for _, o := range opts {
		o(s)
	}

	for _, repo := range config.Repos {
		names := repo.Names
		if len(names) == 0 {
			names = defaultWorkerNames(repo.Workers)
		}
		s.pools[repo.Name] = NewWorkerPool(repo.Name, names)
	}

	return s
}

func defaultWorkerNames(n int) []string {
	if n <= 0 {
		n = 1
	}
	names := make([]string, n)
	for i := range names {
		names[i] = fmt.Sprintf("worker-%d", i)
	}
	return names
}

// Run starts the scheduler loop. It blocks until ctx is cancelled.
func (s *Scheduler) Run(ctx context.Context) error {
	s.logger.Info("scheduler starting",
		"repos", len(s.config.Repos),
		"max_total_workers", s.config.MaxTotalWorkers,
	)

	ticker := time.NewTicker(s.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("scheduler stopping")
			return ctx.Err()
		case <-ticker.C:
			s.tick(ctx)
		}
	}
}

// Tick runs a single poll cycle across all repos. Exported for testing.
func (s *Scheduler) Tick(ctx context.Context) {
	s.tick(ctx)
}

func (s *Scheduler) tick(ctx context.Context) {
	for _, repo := range s.config.Repos {
		if err := ctx.Err(); err != nil {
			return
		}
		s.tickRepo(ctx, repo)
	}
}

func (s *Scheduler) tickRepo(ctx context.Context, repo workflow.RepoConfig) {
	pool := s.pools[repo.Name]
	client := s.clients[repo.Name]
	wf := s.workflows[repo.Name]

	for {
		worker := pool.IdleWorker()
		if worker == nil {
			return
		}

		if s.totalBusy() >= s.config.MaxTotalWorkers {
			return
		}

		bead, err := client.GetReady(repo.BdPrefix)
		if err != nil {
			s.logger.Error("poll failed", "repo", repo.Name, "error", err)
			return
		}
		if bead == nil {
			return
		}

		step := currentStep(bead, wf)
		if step == nil {
			s.logger.Error("no step found", "repo", repo.Name, "bead", bead.ID)
			return
		}

		pool.Assign(worker, bead.ID, step.Name)
		go s.runStep(ctx, worker, pool, bead, *step, repo)
	}
}

func (s *Scheduler) totalBusy() int {
	total := 0
	for _, pool := range s.pools {
		total += pool.BusyCount()
	}
	return total
}

// currentStep determines which workflow step a bead is at.
// If the bead has a "step" metadata key, look up that step.
// Otherwise, start at the first step in the workflow.
func currentStep(bead *bd.Bead, wf *workflow.Workflow) *workflow.WorkflowStep {
	if bead.Metadata != nil {
		if stepName, ok := bead.Metadata["step"].(string); ok && stepName != "" {
			return lookupStep(wf, stepName)
		}
	}
	if len(wf.Steps) > 0 {
		return &wf.Steps[0]
	}
	return nil
}

func lookupStep(wf *workflow.Workflow, name string) *workflow.WorkflowStep {
	for i := range wf.Steps {
		if wf.Steps[i].Name == name {
			return &wf.Steps[i]
		}
	}
	return nil
}

func (s *Scheduler) runStep(
	ctx context.Context,
	worker *Worker,
	pool *WorkerPool,
	bead *bd.Bead,
	step workflow.WorkflowStep,
	repo workflow.RepoConfig,
) {
	defer pool.Release(worker)

	client := s.clients[repo.Name]
	wf := s.workflows[repo.Name]

	s.logger.Info("step starting",
		"repo", repo.Name,
		"bead", bead.ID,
		"step", step.Name,
		"worker", worker.Name,
	)

	// Update bead to current step.
	if err := client.UpdateStep(bead.ID, step.Name); err != nil {
		s.logger.Error("update step failed", "bead", bead.ID, "error", err)
		return
	}

	// Increment attempts and check retry budget.
	attempts, err := client.IncrementAttempts(bead.ID, step.Name)
	if err != nil {
		s.logger.Error("increment attempts failed", "bead", bead.ID, "error", err)
		return
	}

	if step.MaxIterations > 0 && attempts > step.MaxIterations {
		reason := fmt.Sprintf("step %q exceeded max iterations (%d)", step.Name, step.MaxIterations)
		s.logger.Warn("escalating", "bead", bead.ID, "reason", reason)
		if err := client.Escalate(bead.ID, reason); err != nil {
			s.logger.Error("escalate failed", "bead", bead.ID, "error", err)
		}
		return
	}

	// Gather prior notes for context forwarding.
	notes, err := client.GetNotes(bead.ID)
	if err != nil {
		s.logger.Error("get notes failed", "bead", bead.ID, "error", err)
		notes = nil
	}

	req := StepRequest{
		Bead:       bead,
		Step:       step,
		Workflow:   wf,
		RepoConfig: repo,
		WorkerName: worker.Name,
		Notes:      notes,
	}

	// Apply step timeout.
	stepCtx := ctx
	if step.TimeoutMinutes > 0 {
		var cancel context.CancelFunc
		stepCtx, cancel = context.WithTimeout(ctx, time.Duration(step.TimeoutMinutes)*time.Minute)
		defer cancel()
	}

	// Execute the step.
	outcome, err := s.runner.Run(stepCtx, req)
	if err != nil {
		// Agent crash or timeout: bead stays at current step for requeue.
		s.logger.Error("step execution failed",
			"repo", repo.Name,
			"bead", bead.ID,
			"step", step.Name,
			"worker", worker.Name,
			"error", err,
		)
		return
	}

	s.logger.Info("step completed",
		"repo", repo.Name,
		"bead", bead.ID,
		"step", step.Name,
		"result", outcome.Result,
	)

	// Attach notes from this step.
	if outcome.Notes != "" {
		if err := client.AttachNotes(bead.ID, step.Name, outcome.Notes); err != nil {
			s.logger.Error("attach notes failed", "bead", bead.ID, "error", err)
		}
	}

	// Route to next step.
	next := route(step, outcome.Result)
	if next == "" {
		reason := fmt.Sprintf("no route from step %q for result %q", step.Name, outcome.Result)
		s.logger.Warn("no route", "bead", bead.ID, "step", step.Name, "result", outcome.Result)
		if err := client.Escalate(bead.ID, reason); err != nil {
			s.logger.Error("escalate failed", "bead", bead.ID, "error", err)
		}
		return
	}

	if isTerminal(next) {
		s.handleTerminal(client, bead.ID, next, step.Name)
		return
	}

	// Advance bead to next step.
	if err := client.UpdateStep(bead.ID, next); err != nil {
		s.logger.Error("advance step failed", "bead", bead.ID, "next", next, "error", err)
	}
}

// route determines the next step name based on the outcome result.
func route(step workflow.WorkflowStep, result Result) string {
	switch result {
	case ResultPass:
		return step.OnPass
	case ResultFail:
		return step.OnFail
	case ResultRevision:
		return step.OnRevision
	case ResultEscalate:
		return step.OnEscalate
	default:
		return step.OnFail
	}
}

// isTerminal returns true if the target is a terminal state.
func isTerminal(name string) bool {
	switch strings.ToLower(name) {
	case "done", "blocked", "human", "escalate":
		return true
	}
	return false
}

func (s *Scheduler) handleTerminal(client BeadClient, beadID, terminal, fromStep string) {
	s.logger.Info("reached terminal", "bead", beadID, "terminal", terminal, "from_step", fromStep)

	switch strings.ToLower(terminal) {
	case "done":
		if err := client.UpdateStep(beadID, "done"); err != nil {
			s.logger.Error("mark done failed", "bead", beadID, "error", err)
		}
	case "blocked", "human", "escalate":
		reason := fmt.Sprintf("reached terminal %q from step %q", terminal, fromStep)
		if err := client.Escalate(beadID, reason); err != nil {
			s.logger.Error("escalate at terminal failed", "bead", beadID, "error", err)
		}
	}
}

// WriteContext writes a CONTEXT.md file with notes from previous steps.
// Call this before spawning the next agent to provide context from prior steps.
func WriteContext(dir string, notes []bd.StepNote) error {
	if len(notes) == 0 {
		return nil
	}

	var b []byte
	b = append(b, "# Context from Previous Steps\n\n"...)
	for _, n := range notes {
		header := n.FromStep
		if header == "" {
			header = "unknown"
		}
		b = append(b, fmt.Sprintf("## Step: %s\n\n%s\n\n", header, n.Text)...)
	}

	return os.WriteFile(filepath.Join(dir, "CONTEXT.md"), b, 0o644)
}
