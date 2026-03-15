package workflow

import (
	"path/filepath"
	"strings"
	"testing"
)

func testdataPath(name string) string {
	return filepath.Join("testdata", name)
}

func TestParseValidWorkflow(t *testing.T) {
	w, err := ParseWorkflow(testdataPath("valid_workflow.yaml"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if w.Name != "feature" {
		t.Errorf("name = %q, want %q", w.Name, "feature")
	}
	if len(w.Steps) != 4 {
		t.Fatalf("got %d steps, want 4", len(w.Steps))
	}

	impl := w.Steps[0]
	if impl.Name != "implement" {
		t.Errorf("step[0].Name = %q, want %q", impl.Name, "implement")
	}
	if impl.Type != StepTypeAgent {
		t.Errorf("step[0].Type = %q, want %q", impl.Type, StepTypeAgent)
	}
	if impl.Role != "implementer" {
		t.Errorf("step[0].Role = %q, want %q", impl.Role, "implementer")
	}
	if impl.Model != "sonnet" {
		t.Errorf("step[0].Model = %q, want %q", impl.Model, "sonnet")
	}
	if impl.Context != ContextFullCodebase {
		t.Errorf("step[0].Context = %q, want %q", impl.Context, ContextFullCodebase)
	}
	if impl.TimeoutMinutes != 30 {
		t.Errorf("step[0].TimeoutMinutes = %d, want 30", impl.TimeoutMinutes)
	}
	if impl.OnPass != "review" {
		t.Errorf("step[0].OnPass = %q, want %q", impl.OnPass, "review")
	}
	if impl.OnFail != "blocked" {
		t.Errorf("step[0].OnFail = %q, want %q", impl.OnFail, "blocked")
	}

	review := w.Steps[1]
	if review.OnRevision != "implement" {
		t.Errorf("step[1].OnRevision = %q, want %q", review.OnRevision, "implement")
	}
	if review.OnEscalate != "human" {
		t.Errorf("step[1].OnEscalate = %q, want %q", review.OnEscalate, "human")
	}

	merge := w.Steps[3]
	if merge.Type != StepTypeAutomated {
		t.Errorf("step[3].Type = %q, want %q", merge.Type, StepTypeAutomated)
	}
}

func TestCircularRouteError(t *testing.T) {
	_, err := ParseWorkflow(testdataPath("circular_route.yaml"))
	if err == nil {
		t.Fatal("expected circular route error, got nil")
	}
	if !strings.Contains(err.Error(), "circular route") {
		t.Errorf("error = %q, want it to contain 'circular route'", err)
	}
}

func TestMissingRefError(t *testing.T) {
	_, err := ParseWorkflow(testdataPath("missing_ref.yaml"))
	if err == nil {
		t.Fatal("expected missing ref error, got nil")
	}
	if !strings.Contains(err.Error(), "nonexistent-step") {
		t.Errorf("error = %q, want it to mention 'nonexistent-step'", err)
	}
	if !strings.Contains(err.Error(), "unknown step") {
		t.Errorf("error = %q, want it to contain 'unknown step'", err)
	}
}

func TestUnknownTypeError(t *testing.T) {
	_, err := ParseWorkflow(testdataPath("unknown_type.yaml"))
	if err == nil {
		t.Fatal("expected unknown type error, got nil")
	}
	if !strings.Contains(err.Error(), "unknown type") {
		t.Errorf("error = %q, want it to contain 'unknown type'", err)
	}
	if !strings.Contains(err.Error(), "magic") {
		t.Errorf("error = %q, want it to mention 'magic'", err)
	}
}

func TestParseWorkflowBytes(t *testing.T) {
	yaml := `
name: simple
steps:
  - name: do-thing
    type: gate
    on_pass: done
`
	w, err := ParseWorkflowBytes([]byte(yaml))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if w.Name != "simple" {
		t.Errorf("name = %q, want %q", w.Name, "simple")
	}
	if len(w.Steps) != 1 {
		t.Fatalf("got %d steps, want 1", len(w.Steps))
	}
	if w.Steps[0].Type != StepTypeGate {
		t.Errorf("type = %q, want %q", w.Steps[0].Type, StepTypeGate)
	}
}

func TestValidateEmptyName(t *testing.T) {
	w := &Workflow{Steps: []WorkflowStep{{Name: "x", Type: StepTypeAgent}}}
	err := Validate(w)
	if err == nil || !strings.Contains(err.Error(), "name is required") {
		t.Errorf("expected name required error, got %v", err)
	}
}

func TestValidateNoSteps(t *testing.T) {
	w := &Workflow{Name: "empty"}
	err := Validate(w)
	if err == nil || !strings.Contains(err.Error(), "no steps") {
		t.Errorf("expected no steps error, got %v", err)
	}
}

func TestValidateDuplicateStepName(t *testing.T) {
	w := &Workflow{
		Name: "dup",
		Steps: []WorkflowStep{
			{Name: "a", Type: StepTypeAgent, OnPass: "done"},
			{Name: "a", Type: StepTypeAgent, OnPass: "done"},
		},
	}
	err := Validate(w)
	if err == nil || !strings.Contains(err.Error(), "duplicate step name") {
		t.Errorf("expected duplicate step error, got %v", err)
	}
}

// --- FarmConfig validation tests ---

func TestValidateFarmConfig_Valid(t *testing.T) {
	cfg := &FarmConfig{
		Repos: []RepoConfig{
			{Name: "ScaledTest", Workers: 2, Names: []string{"max", "furiosa"}, Prefix: "st"},
			{Name: "citadel", Workers: 1, Names: []string{"immortan"}, Prefix: "ct"},
		},
		MaxTotalWorkers: 3,
	}
	if err := ValidateFarmConfig(cfg); err != nil {
		t.Fatalf("expected valid config, got: %v", err)
	}
}

func TestValidateFarmConfig_NoRepos(t *testing.T) {
	cfg := &FarmConfig{MaxTotalWorkers: 1}
	err := ValidateFarmConfig(cfg)
	if err == nil || !strings.Contains(err.Error(), "at least one repo") {
		t.Errorf("expected at least one repo error, got %v", err)
	}
}

func TestValidateFarmConfig_MaxTotalWorkersZero(t *testing.T) {
	cfg := &FarmConfig{
		Repos:           []RepoConfig{{Name: "r1", Workers: 1}},
		MaxTotalWorkers: 0,
	}
	err := ValidateFarmConfig(cfg)
	if err == nil || !strings.Contains(err.Error(), "max_total_workers") {
		t.Errorf("expected max_total_workers error, got %v", err)
	}
}

func TestValidateFarmConfig_DuplicateRepoName(t *testing.T) {
	cfg := &FarmConfig{
		Repos: []RepoConfig{
			{Name: "dup", Workers: 1, Prefix: "a"},
			{Name: "dup", Workers: 1, Prefix: "b"},
		},
		MaxTotalWorkers: 2,
	}
	err := ValidateFarmConfig(cfg)
	if err == nil || !strings.Contains(err.Error(), "duplicate repo name") {
		t.Errorf("expected duplicate repo name error, got %v", err)
	}
}

func TestValidateFarmConfig_DuplicatePrefix(t *testing.T) {
	cfg := &FarmConfig{
		Repos: []RepoConfig{
			{Name: "r1", Workers: 1, Prefix: "shared"},
			{Name: "r2", Workers: 1, Prefix: "shared"},
		},
		MaxTotalWorkers: 2,
	}
	err := ValidateFarmConfig(cfg)
	if err == nil || !strings.Contains(err.Error(), "share prefix") {
		t.Errorf("expected shared prefix error, got %v", err)
	}
}

func TestValidateFarmConfig_WorkersNamesMismatch(t *testing.T) {
	cfg := &FarmConfig{
		Repos: []RepoConfig{
			{Name: "r1", Workers: 3, Names: []string{"a", "b"}},
		},
		MaxTotalWorkers: 3,
	}
	err := ValidateFarmConfig(cfg)
	if err == nil || !strings.Contains(err.Error(), "workers=3 but names has 2") {
		t.Errorf("expected workers/names mismatch error, got %v", err)
	}
}

func TestValidateFarmConfig_ZeroWorkers(t *testing.T) {
	cfg := &FarmConfig{
		Repos:           []RepoConfig{{Name: "r1", Workers: 0}},
		MaxTotalWorkers: 1,
	}
	err := ValidateFarmConfig(cfg)
	if err == nil || !strings.Contains(err.Error(), "workers must be > 0") {
		t.Errorf("expected workers > 0 error, got %v", err)
	}
}

func TestValidateFarmConfig_NamesOnly(t *testing.T) {
	// Names specified, workers omitted — should infer worker count from names.
	cfg := &FarmConfig{
		Repos: []RepoConfig{
			{Name: "r1", Names: []string{"a", "b"}},
		},
		MaxTotalWorkers: 2,
	}
	if err := ValidateFarmConfig(cfg); err != nil {
		t.Fatalf("names-only config should be valid, got: %v", err)
	}
}

func TestValidateFarmConfig_MissingRepoName(t *testing.T) {
	cfg := &FarmConfig{
		Repos:           []RepoConfig{{Workers: 1}},
		MaxTotalWorkers: 1,
	}
	err := ValidateFarmConfig(cfg)
	if err == nil || !strings.Contains(err.Error(), "name is required") {
		t.Errorf("expected name required error, got %v", err)
	}
}

func TestTerminalRefsAreValid(t *testing.T) {
	// "done", "blocked", "human", "escalate" should be accepted as targets.
	yaml := `
name: terminals
steps:
  - name: s1
    type: agent
    on_pass: done
    on_fail: blocked
    on_revision: human
    on_escalate: escalate
`
	_, err := ParseWorkflowBytes([]byte(yaml))
	if err != nil {
		t.Fatalf("terminal refs should be valid, got: %v", err)
	}
}
