package workflow

// StepType classifies what runs in a workflow step.
type StepType string

const (
	StepTypeAgent     StepType = "agent"
	StepTypeAutomated StepType = "automated"
	StepTypeGate      StepType = "gate"
	StepTypeHuman     StepType = "human"
)

// ContextLevel controls what context an agent step receives.
type ContextLevel string

const (
	ContextFullCodebase ContextLevel = "full_codebase"
	ContextDiffOnly     ContextLevel = "diff_only"
	ContextSpecOnly     ContextLevel = "spec_only"
)

// WorkflowStep defines a single step in a workflow pipeline.
type WorkflowStep struct {
	Name    string       `yaml:"name"`
	Type    StepType     `yaml:"type"`
	Role    string       `yaml:"role,omitempty"`
	Model   string       `yaml:"model,omitempty"`
	Context ContextLevel `yaml:"context,omitempty"`

	TimeoutMinutes int    `yaml:"timeout_minutes,omitempty"`
	SkipFor        []int  `yaml:"skip_for,omitempty"` // complexity levels that skip this step
	OnPass         string `yaml:"on_pass,omitempty"`
	OnFail         string `yaml:"on_fail,omitempty"`
	OnRevision     string `yaml:"on_revision,omitempty"`
	OnEscalate     string `yaml:"on_escalate,omitempty"`
}

// RoleDefinition defines an agent role in YAML.
type RoleDefinition struct {
	Name         string `yaml:"name"`
	Description  string `yaml:"description"`
	Instructions string `yaml:"instructions"`
}

// ComplexityLevel defines skip rules for a single complexity tier.
type ComplexityLevel struct {
	Level        int      `yaml:"level"`
	SkipSteps    []string `yaml:"skip_steps"`
	RequireHuman bool     `yaml:"require_human,omitempty"`
}

// ComplexityConfig holds the four complexity tiers for a workflow.
type ComplexityConfig struct {
	Trivial  ComplexityLevel `yaml:"trivial"`
	Standard ComplexityLevel `yaml:"standard"`
	Full     ComplexityLevel `yaml:"full"`
	Critical ComplexityLevel `yaml:"critical"`
}

// SkipStepsForLevel returns step names that should be skipped for the given
// complexity level, derived from each step's skip_for field.
func (wf *Workflow) SkipStepsForLevel(level int) []string {
	var skipped []string
	for _, step := range wf.Steps {
		for _, cx := range step.SkipFor {
			if cx == level {
				skipped = append(skipped, step.Name)
				break
			}
		}
	}
	return skipped
}

// SkipStepsForLevel on ComplexityConfig is kept for backward compat but delegates.
func (cc ComplexityConfig) SkipStepsForLevel(level int) []string {
	switch level {
	case 1:
		return cc.Trivial.SkipSteps
	case 2:
		return cc.Standard.SkipSteps
	case 4:
		return cc.Critical.SkipSteps
	default:
		return cc.Full.SkipSteps
	}
}

// RequireHumanForLevel returns whether a human gate is required for a given complexity level.
func (cc ComplexityConfig) RequireHumanForLevel(level int) bool {
	switch level {
	case 4:
		return cc.Critical.RequireHuman
	default:
		return false
	}
}

// Workflow is a named sequence of steps parsed from a YAML file.
type Workflow struct {
	Name       string                    `yaml:"name"`
	Steps      []WorkflowStep            `yaml:"steps"`
	Roles      map[string]RoleDefinition `yaml:"roles,omitempty"`
	Complexity ComplexityConfig          `yaml:"complexity"`
}

// RepoConfig defines a repository managed by the farm.
type RepoConfig struct {
	Name         string   `yaml:"name"`
	URL          string   `yaml:"url"`
	WorkflowPath string   `yaml:"workflow_path"`
	Workers      int      `yaml:"workers"`
	Names        []string `yaml:"names,omitempty"`
	Prefix       string   `yaml:"prefix"`
}

// IdleHook defines an action to run when the scheduler enters idle state.
type IdleHook struct {
	Name    string `yaml:"name"`
	Action  string `yaml:"action"`                    // built-in: "roles_generate", "worktree_prune", "db_vacuum" | "shell"
	Command string `yaml:"command,omitempty"`         // only for action: shell
	Timeout int    `yaml:"timeout_seconds,omitempty"` // default 30s
}

// FarmConfig is the top-level configuration for a Citadel instance.
type FarmConfig struct {
	Repos                 []RepoConfig `yaml:"repos"`
	MaxTotalWorkers       int          `yaml:"max_total_workers"`
	HandoffTokenThreshold int          `yaml:"handoff_token_threshold"`
	RetentionDays         int          `yaml:"retention_days"`
	CleanupInterval       string       `yaml:"cleanup_interval"`
	IdleHooks             []IdleHook   `yaml:"idle_hooks,omitempty"`
}
