package workflow

import (
	"fmt"
	"strings"
)

var validStepTypes = map[StepType]bool{
	StepTypeAgent:     true,
	StepTypeAutomated: true,
	StepTypeGate:      true,
	StepTypeHuman:     true,
}

var validContextLevels = map[ContextLevel]bool{
	ContextFullCodebase: true,
	ContextDiffOnly:     true,
	ContextSpecOnly:     true,
}

// Validate checks a Workflow for structural errors.
func Validate(w *Workflow) error {
	if w.Name == "" {
		return fmt.Errorf("workflow name is required")
	}
	if len(w.Steps) == 0 {
		return fmt.Errorf("workflow %q has no steps", w.Name)
	}

	stepNames := make(map[string]bool, len(w.Steps))
	for _, s := range w.Steps {
		if s.Name == "" {
			return fmt.Errorf("workflow %q: step name is required", w.Name)
		}
		if stepNames[s.Name] {
			return fmt.Errorf("workflow %q: duplicate step name %q", w.Name, s.Name)
		}
		stepNames[s.Name] = true
	}

	for _, s := range w.Steps {
		if err := validateStep(w, s, stepNames); err != nil {
			return err
		}
	}

	if err := checkCircularRoutes(w); err != nil {
		return err
	}

	return nil
}

func validateStep(w *Workflow, s WorkflowStep, stepNames map[string]bool) error {
	// Default type to agent if not specified.
	if s.Type == "" {
		s.Type = StepTypeAgent
	}

	if !validStepTypes[s.Type] {
		return fmt.Errorf("workflow %q step %q: unknown type %q", w.Name, s.Name, s.Type)
	}

	if s.Context != "" && !validContextLevels[s.Context] {
		return fmt.Errorf("workflow %q step %q: unknown context %q", w.Name, s.Name, s.Context)
	}

	if s.MaxIterations < 0 {
		return fmt.Errorf("workflow %q step %q: max_iterations must be >= 1, got %d", w.Name, s.Name, s.MaxIterations)
	}
	if s.MaxIterations == 0 {
		// 0 means unset, which is fine — runtime will use defaults.
	} else if s.MaxIterations < 1 {
		return fmt.Errorf("workflow %q step %q: max_iterations must be >= 1, got %d", w.Name, s.Name, s.MaxIterations)
	}

	// Validate step references in routing fields.
	for _, ref := range stepRefs(s) {
		if ref.target == "" {
			continue
		}
		if !isTerminal(ref.target) && !stepNames[ref.target] {
			return fmt.Errorf("workflow %q step %q: %s references unknown step %q", w.Name, s.Name, ref.field, ref.target)
		}
	}

	return nil
}

type stepRef struct {
	field  string
	target string
}

func stepRefs(s WorkflowStep) []stepRef {
	return []stepRef{
		{"on_pass", s.OnPass},
		{"on_fail", s.OnFail},
		{"on_revision", s.OnRevision},
		{"on_escalate", s.OnEscalate},
	}
}

// isTerminal returns true for built-in terminal states that are not step names.
func isTerminal(name string) bool {
	switch strings.ToLower(name) {
	case "done", "blocked", "human", "escalate":
		return true
	}
	return false
}

// ValidateFarmConfig checks a FarmConfig for structural errors.
func ValidateFarmConfig(cfg *FarmConfig) error {
	if len(cfg.Repos) == 0 {
		return fmt.Errorf("farm config: at least one repo is required")
	}

	if cfg.MaxTotalWorkers <= 0 {
		return fmt.Errorf("farm config: max_total_workers must be > 0, got %d", cfg.MaxTotalWorkers)
	}

	repoNames := make(map[string]bool, len(cfg.Repos))
	bdPrefixes := make(map[string]string, len(cfg.Repos))

	for i, repo := range cfg.Repos {
		if repo.Name == "" {
			return fmt.Errorf("farm config: repo[%d] name is required", i)
		}
		if repoNames[repo.Name] {
			return fmt.Errorf("farm config: duplicate repo name %q", repo.Name)
		}
		repoNames[repo.Name] = true

		if repo.BdPrefix != "" {
			if other, ok := bdPrefixes[repo.BdPrefix]; ok {
				return fmt.Errorf("farm config: repos %q and %q share bd_prefix %q", other, repo.Name, repo.BdPrefix)
			}
			bdPrefixes[repo.BdPrefix] = repo.Name
		}

		// Determine effective worker count.
		workers := repo.Workers
		if len(repo.Names) > 0 {
			if workers > 0 && workers != len(repo.Names) {
				return fmt.Errorf("farm config: repo %q: workers=%d but names has %d entries", repo.Name, workers, len(repo.Names))
			}
			workers = len(repo.Names)
		}
		if workers <= 0 {
			return fmt.Errorf("farm config: repo %q: workers must be > 0", repo.Name)
		}
	}

	return nil
}

// checkCircularRoutes detects dead-end cycles: groups of steps where no step
// has any route to a terminal state. Intentional loops (e.g., implement ->
// review -> implement) are allowed as long as some path exits the cycle.
func checkCircularRoutes(w *Workflow) error {
	// A step "can terminate" if it has any route to a terminal state, or if it
	// has a route to another step that can terminate. We compute this via
	// backward propagation from terminal-reachable steps.

	stepSet := make(map[string]bool, len(w.Steps))
	// routes maps step name -> all targets (including terminals).
	routes := make(map[string][]string, len(w.Steps))
	for _, s := range w.Steps {
		stepSet[s.Name] = true
		for _, ref := range stepRefs(s) {
			if ref.target != "" {
				routes[s.Name] = append(routes[s.Name], ref.target)
			}
		}
	}

	// Mark steps that can reach a terminal. Start with steps that directly
	// route to a terminal, then propagate backward.
	canTerminate := make(map[string]bool, len(w.Steps))

	// Seed: steps with at least one terminal route.
	for name, targets := range routes {
		for _, t := range targets {
			if isTerminal(t) {
				canTerminate[name] = true
				break
			}
		}
	}

	// Also seed steps with no routes at all (implicit terminal — step just stops).
	for _, s := range w.Steps {
		if len(routes[s.Name]) == 0 {
			canTerminate[s.Name] = true
		}
	}

	// Reverse adjacency for backward propagation.
	revAdj := make(map[string][]string, len(w.Steps))
	for name, targets := range routes {
		for _, t := range targets {
			if stepSet[t] {
				revAdj[t] = append(revAdj[t], name)
			}
		}
	}

	// BFS backward from terminable steps.
	queue := make([]string, 0, len(canTerminate))
	for name := range canTerminate {
		queue = append(queue, name)
	}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, pred := range revAdj[cur] {
			if !canTerminate[pred] {
				canTerminate[pred] = true
				queue = append(queue, pred)
			}
		}
	}

	// Any step that cannot reach a terminal is part of a dead-end cycle.
	for _, s := range w.Steps {
		if !canTerminate[s.Name] {
			return fmt.Errorf("workflow %q: circular route detected: step %q has no path to a terminal state", w.Name, s.Name)
		}
	}

	return nil
}
