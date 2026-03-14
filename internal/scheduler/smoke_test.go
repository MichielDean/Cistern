package scheduler

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/MichielDean/bullet-farm/internal/bd"
	"github.com/MichielDean/bullet-farm/internal/workflow"
)

// featureWorkflow returns the full 4-step feature pipeline matching
// the canonical workflow in testdata/valid_workflow.yaml.
func featureWorkflow() *workflow.Workflow {
	return &workflow.Workflow{
		Name: "feature",
		Steps: []workflow.WorkflowStep{
			{
				Name:           "implement",
				Type:           workflow.StepTypeAgent,
				Role:           "implementer",
				Context:        workflow.ContextFullCodebase,
				MaxIterations:  3,
				TimeoutMinutes: 30,
				OnPass:         "review",
				OnFail:         "blocked",
			},
			{
				Name:       "review",
				Type:       workflow.StepTypeAgent,
				Role:       "reviewer",
				Context:    workflow.ContextDiffOnly,
				OnPass:     "qa",
				OnRevision: "implement",
				OnEscalate: "human",
			},
			{
				Name:    "qa",
				Type:    workflow.StepTypeAgent,
				Role:    "qa",
				Context: workflow.ContextFullCodebase,
				OnPass:  "merge",
				OnFail:  "implement",
			},
			{
				Name:   "merge",
				Type:   workflow.StepTypeAutomated,
				OnPass: "done",
				OnFail: "human",
			},
		},
	}
}

// --- pipeline-aware mocks ---

// pipelineClient tracks a single bead through the entire workflow,
// re-presenting it to GetReady with updated metadata until it reaches
// a terminal state. Unlike the queue-based mockClient, this simulates
// a bead that persists in the work queue until completion.
type pipelineClient struct {
	mu        sync.Mutex
	bead      bd.Bead
	stepLog   []string       // every UpdateStep call in order
	attached  []attachedNote // notes attached by steps
	notes     []bd.StepNote  // accumulated notes (returned by GetNotes)
	escalated string         // non-empty if escalated
	attempts  map[string]int
	terminal  bool
}

func newPipelineClient(b bd.Bead) *pipelineClient {
	return &pipelineClient{
		bead:     b,
		attempts: make(map[string]int),
	}
}

func (c *pipelineClient) GetReady(rig string) (*bd.Bead, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.terminal {
		return nil, nil
	}
	// Return a copy with current metadata.
	b := c.bead
	if c.bead.Metadata != nil {
		b.Metadata = make(map[string]any)
		for k, v := range c.bead.Metadata {
			b.Metadata[k] = v
		}
	}
	return &b, nil
}

func (c *pipelineClient) UpdateStep(id, step string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.stepLog = append(c.stepLog, step)
	if c.bead.Metadata == nil {
		c.bead.Metadata = make(map[string]any)
	}
	c.bead.Metadata["step"] = step
	if step == "done" {
		c.terminal = true
	}
	return nil
}

func (c *pipelineClient) IncrementAttempts(id, step string) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	key := id + ":" + step
	c.attempts[key]++
	return c.attempts[key], nil
}

func (c *pipelineClient) AttachNotes(id, fromStep, notes string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.attached = append(c.attached, attachedNote{id, fromStep, notes})
	c.notes = append(c.notes, bd.StepNote{
		IssueID:  id,
		FromStep: fromStep,
		Text:     notes,
	})
	return nil
}

func (c *pipelineClient) GetNotes(id string) ([]bd.StepNote, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	result := make([]bd.StepNote, len(c.notes))
	copy(result, c.notes)
	return result, nil
}

func (c *pipelineClient) Escalate(id, reason string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.escalated = reason
	c.terminal = true
	return nil
}

// stepSequenceRunner returns outcomes from a per-step queue, supporting
// multiple calls to the same step (e.g., revision loops).
type stepSequenceRunner struct {
	mu       sync.Mutex
	outcomes map[string][]*Outcome
	calls    []StepRequest
	done     chan struct{}
}

func newStepSequenceRunner(outcomes map[string][]*Outcome) *stepSequenceRunner {
	return &stepSequenceRunner{
		outcomes: outcomes,
		done:     make(chan struct{}, 32),
	}
}

func (r *stepSequenceRunner) Run(_ context.Context, req StepRequest) (*Outcome, error) {
	r.mu.Lock()
	defer func() {
		r.mu.Unlock()
		r.done <- struct{}{}
	}()
	r.calls = append(r.calls, req)
	seq := r.outcomes[req.Step.Name]
	if len(seq) == 0 {
		return &Outcome{Result: ResultPass}, nil
	}
	o := seq[0]
	r.outcomes[req.Step.Name] = seq[1:]
	return o, nil
}

func (r *stepSequenceRunner) waitStep(t *testing.T) {
	t.Helper()
	select {
	case <-r.done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for step execution")
	}
}

// --- smoke test helpers ---

func smokeConfig() workflow.FarmConfig {
	return workflow.FarmConfig{
		Repos: []workflow.RepoConfig{
			{
				Name:     "bullet-farm",
				Workers:  1,
				Names:    []string{"smoker"},
				BdPrefix: "bf",
			},
		},
		MaxTotalWorkers: 1,
	}
}

func smokeScheduler(client BeadClient, runner StepRunner) *Scheduler {
	config := smokeConfig()
	workflows := map[string]*workflow.Workflow{"bullet-farm": featureWorkflow()}
	clients := map[string]BeadClient{"bullet-farm": client}
	return NewFromParts(config, workflows, clients, runner)
}

// advanceStep runs one scheduler tick, waits for the step to execute,
// and waits for post-execution routing to complete.
func advanceStep(t *testing.T, sched *Scheduler, runner *stepSequenceRunner) {
	t.Helper()
	sched.Tick(context.Background())
	runner.waitStep(t)
	time.Sleep(50 * time.Millisecond)
}

// --- smoke tests ---

// TestSmoke_FeatureWorkflow_HappyPath drives a bead through the complete
// feature pipeline: implement → review → qa → merge → done.
// Verifies step routing, context levels, notes attachment, and terminal state.
func TestSmoke_FeatureWorkflow_HappyPath(t *testing.T) {
	client := newPipelineClient(bd.Bead{
		ID:          "bf-smoke-1",
		Title:       "Smoke test: add trivial comment",
		Description: "Add a test comment to verify the pipeline end-to-end",
	})

	runner := newStepSequenceRunner(map[string][]*Outcome{
		"implement": {{Result: ResultPass, Notes: "added comment in main.go"}},
		"review":    {{Result: ResultPass, Notes: "diff clean, no issues found"}},
		"qa":        {{Result: ResultPass, Notes: "all tests pass (go test ./...)"}},
		"merge":     {{Result: ResultPass, Notes: "PR #1 merged to main"}},
	})

	sched := smokeScheduler(client, runner)
	for i := 0; i < 4; i++ {
		advanceStep(t, sched, runner)
	}

	// --- verify final state ---

	client.mu.Lock()
	defer client.mu.Unlock()

	if !client.terminal {
		t.Fatal("bead should have reached terminal state")
	}

	// Each step calls UpdateStep twice: once to set current, once to advance.
	// implement→review, review→qa, qa→merge, merge→done
	wantLog := []string{
		"implement", "review",
		"review", "qa",
		"qa", "merge",
		"merge", "done",
	}
	if len(client.stepLog) != len(wantLog) {
		t.Fatalf("step log = %v (len %d), want %v (len %d)",
			client.stepLog, len(client.stepLog), wantLog, len(wantLog))
	}
	for i, want := range wantLog {
		if client.stepLog[i] != want {
			t.Errorf("step log[%d] = %q, want %q", i, client.stepLog[i], want)
		}
	}

	// All 4 steps should have been executed with correct context levels.
	runner.mu.Lock()
	defer runner.mu.Unlock()

	if len(runner.calls) != 4 {
		t.Fatalf("expected 4 runner calls, got %d", len(runner.calls))
	}

	wantSteps := []struct {
		name    string
		context workflow.ContextLevel
		role    string
	}{
		{"implement", workflow.ContextFullCodebase, "implementer"},
		{"review", workflow.ContextDiffOnly, "reviewer"},
		{"qa", workflow.ContextFullCodebase, "qa"},
		{"merge", "", ""},
	}
	for i, want := range wantSteps {
		call := runner.calls[i]
		if call.Step.Name != want.name {
			t.Errorf("call[%d].Step.Name = %q, want %q", i, call.Step.Name, want.name)
		}
		if call.Step.Context != want.context {
			t.Errorf("call[%d].Step.Context = %q, want %q", i, call.Step.Context, want.context)
		}
		if call.Step.Role != want.role {
			t.Errorf("call[%d].Step.Role = %q, want %q", i, call.Step.Role, want.role)
		}
	}

	// Notes from each step should be attached.
	if len(client.attached) != 4 {
		t.Fatalf("expected 4 attached notes, got %d", len(client.attached))
	}
	noteSteps := []string{"implement", "review", "qa", "merge"}
	for i, step := range noteSteps {
		if client.attached[i].fromStep != step {
			t.Errorf("attached[%d].fromStep = %q, want %q", i, client.attached[i].fromStep, step)
		}
	}

	// No escalation.
	if client.escalated != "" {
		t.Errorf("unexpected escalation: %s", client.escalated)
	}
}

// TestSmoke_FeatureWorkflow_RevisionLoop tests the review→implement
// revision loop: review sends "revision" → bead returns to implement →
// second attempt passes review → continues to qa → merge → done.
func TestSmoke_FeatureWorkflow_RevisionLoop(t *testing.T) {
	client := newPipelineClient(bd.Bead{
		ID:    "bf-smoke-2",
		Title: "Smoke test: revision loop",
	})

	runner := newStepSequenceRunner(map[string][]*Outcome{
		"implement": {
			{Result: ResultPass, Notes: "first implementation"},
			{Result: ResultPass, Notes: "addressed review feedback"},
		},
		"review": {
			{Result: ResultRevision, Notes: "missing error handling on line 42"},
			{Result: ResultPass, Notes: "revision looks good"},
		},
		"qa":    {{Result: ResultPass, Notes: "tests pass"}},
		"merge": {{Result: ResultPass, Notes: "merged"}},
	})

	sched := smokeScheduler(client, runner)

	// 6 steps: implement, review(revision), implement, review(pass), qa, merge
	for i := 0; i < 6; i++ {
		advanceStep(t, sched, runner)
	}

	client.mu.Lock()
	defer client.mu.Unlock()

	if !client.terminal {
		t.Fatal("bead should have reached terminal state")
	}

	// Step log for the revision loop.
	wantLog := []string{
		"implement", "review",    // 1st implement → review
		"review", "implement",    // review(revision) → implement
		"implement", "review",    // 2nd implement → review
		"review", "qa",           // review(pass) → qa
		"qa", "merge",            // qa → merge
		"merge", "done",          // merge → done
	}
	if len(client.stepLog) != len(wantLog) {
		t.Fatalf("step log = %v (len %d), want len %d",
			client.stepLog, len(client.stepLog), len(wantLog))
	}
	for i, want := range wantLog {
		if client.stepLog[i] != want {
			t.Errorf("step log[%d] = %q, want %q", i, client.stepLog[i], want)
		}
	}

	// Runner should have been called 6 times.
	runner.mu.Lock()
	defer runner.mu.Unlock()

	if len(runner.calls) != 6 {
		t.Fatalf("expected 6 runner calls, got %d", len(runner.calls))
	}

	// Verify the second review call received prior notes from all earlier steps.
	// At that point: implement(1st), review(1st), implement(2nd) = 3 notes.
	if len(runner.calls[3].Notes) < 3 {
		t.Errorf("second review call should have >= 3 prior notes, got %d",
			len(runner.calls[3].Notes))
	}
}

// TestSmoke_NotesForwarding verifies that each step receives accumulated
// notes from all prior steps via context forwarding.
func TestSmoke_NotesForwarding(t *testing.T) {
	client := newPipelineClient(bd.Bead{
		ID:    "bf-smoke-3",
		Title: "Smoke test: notes forwarding",
	})

	runner := newStepSequenceRunner(map[string][]*Outcome{
		"implement": {{Result: ResultPass, Notes: "impl: wrote the feature"}},
		"review":    {{Result: ResultPass, Notes: "review: code is clean"}},
		"qa":        {{Result: ResultPass, Notes: "qa: 42 tests pass"}},
		"merge":     {{Result: ResultPass, Notes: "merge: PR merged"}},
	})

	sched := smokeScheduler(client, runner)
	for i := 0; i < 4; i++ {
		advanceStep(t, sched, runner)
	}

	runner.mu.Lock()
	defer runner.mu.Unlock()

	// implement (step 0): no prior notes.
	if len(runner.calls[0].Notes) != 0 {
		t.Errorf("implement should have 0 prior notes, got %d", len(runner.calls[0].Notes))
	}

	// review (step 1): 1 note from implement.
	if len(runner.calls[1].Notes) != 1 {
		t.Errorf("review should have 1 prior note, got %d", len(runner.calls[1].Notes))
	} else if runner.calls[1].Notes[0].FromStep != "implement" {
		t.Errorf("review note[0].FromStep = %q, want %q",
			runner.calls[1].Notes[0].FromStep, "implement")
	}

	// qa (step 2): 2 notes (implement + review).
	if len(runner.calls[2].Notes) != 2 {
		t.Errorf("qa should have 2 prior notes, got %d", len(runner.calls[2].Notes))
	}

	// merge (step 3): 3 notes (implement + review + qa).
	if len(runner.calls[3].Notes) != 3 {
		t.Errorf("merge should have 3 prior notes, got %d", len(runner.calls[3].Notes))
	}
}

// TestSmoke_QAFailReturnsToImplement verifies that a QA failure routes
// back to the implement step (not to blocked).
func TestSmoke_QAFailReturnsToImplement(t *testing.T) {
	client := newPipelineClient(bd.Bead{
		ID:    "bf-smoke-4",
		Title: "Smoke test: QA failure loop",
	})

	runner := newStepSequenceRunner(map[string][]*Outcome{
		"implement": {
			{Result: ResultPass, Notes: "first impl"},
			{Result: ResultPass, Notes: "fixed failing tests"},
		},
		"review": {
			{Result: ResultPass, Notes: "looks good"},
			{Result: ResultPass, Notes: "still good"},
		},
		"qa": {
			{Result: ResultFail, Notes: "TestFoo failed: expected 42, got 0"},
			{Result: ResultPass, Notes: "all tests pass now"},
		},
		"merge": {{Result: ResultPass, Notes: "merged"}},
	})

	sched := smokeScheduler(client, runner)

	// 8 steps: impl, review, qa(fail), impl, review, qa(pass), merge, done... wait
	// implement → review → qa(fail) → implement → review → qa(pass) → merge → done
	// That's 7 ticks of work.
	// Actually: each "step" is one tick. So:
	// tick 1: implement(pass)
	// tick 2: review(pass)
	// tick 3: qa(fail) → routes to implement
	// tick 4: implement(pass)
	// tick 5: review(pass)
	// tick 6: qa(pass) → routes to merge
	// tick 7: merge(pass) → done
	for i := 0; i < 7; i++ {
		advanceStep(t, sched, runner)
	}

	client.mu.Lock()
	defer client.mu.Unlock()

	if !client.terminal {
		t.Fatal("bead should have reached terminal state")
	}

	// Verify qa failure routed back to implement (not blocked).
	wantLog := []string{
		"implement", "review",    // 1st implement → review
		"review", "qa",           // 1st review → qa
		"qa", "implement",        // qa(fail) → implement
		"implement", "review",    // 2nd implement → review
		"review", "qa",           // 2nd review → qa
		"qa", "merge",            // qa(pass) → merge
		"merge", "done",          // merge → done
	}
	if len(client.stepLog) != len(wantLog) {
		t.Fatalf("step log = %v (len %d), want len %d",
			client.stepLog, len(client.stepLog), len(wantLog))
	}
	for i, want := range wantLog {
		if client.stepLog[i] != want {
			t.Errorf("step log[%d] = %q, want %q", i, client.stepLog[i], want)
		}
	}
}
