// Package provider defines ProviderPreset — the data model that describes how
// to launch any agent CLI supported by Cistern.
package provider

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"slices"
)

// NonInteractiveConfig describes how to invoke an agent CLI in single-shot
// (non-interactive) mode, used by filtration. An empty struct means the preset
// does not define non-interactive invocation.
type NonInteractiveConfig struct {
	// Subcommand is the positional subcommand inserted after Command
	// (e.g. "run" for opencode). Empty means no subcommand.
	Subcommand string `json:"subcommand,omitempty"`
	// PromptFlag is the flag used to pass the combined prompt (e.g. "-p").
	PromptFlag string `json:"prompt_flag,omitempty"`
	// FormatArgs are the flag arguments that request JSON output from the agent
	// (e.g. ["--format", "json"]). When empty/nil, callFilterAgent omits format
	// arguments entirely — no implicit fallback. The elements are appended to the
	// args list after preset.Args and extraArgs but before PromptFlag.
	FormatArgs []string `json:"format_args,omitempty"`
}

// ProviderPreset describes how to launch and interact with a specific agent CLI.
type ProviderPreset struct {
	// Name is the canonical identifier for this provider (e.g. "opencode").
	Name string `json:"name"`
	// Command is the executable to invoke (e.g. "opencode").
	Command string `json:"command"`
	// Args are fixed arguments always appended to the command.
	Args []string `json:"args,omitempty"`
	// Subcommand is the positional subcommand inserted after Command
	// and before flags (e.g. "run" for opencode). Empty means no subcommand.
	Subcommand string `json:"subcommand,omitempty"`
	// EnvPassthrough lists environment variable names to forward into the agent process.
	EnvPassthrough []string `json:"env_passthrough,omitempty"`
	// ModelFlag is the CLI flag used to select the model (e.g. "--model").
	ModelFlag string `json:"model_flag,omitempty"`
	// PromptFlag is the CLI flag used to pass the prompt to the agent (e.g. "-p").
	// When empty, no prompt flag is appended and the prompt must be delivered via
	// an alternative mechanism (stdin, instructions file, etc.).
	PromptFlag string `json:"prompt_flag,omitempty"`
	// PromptPositional indicates that the prompt should be appended as a
	// positional argument rather than via a flag. When true and PromptFileTemplate
	// is set, the short prompt is appended as a positional argument after the
	// subcommand and flags. This is for CLIs like opencode where the prompt
	// is a positional argument, not a flag value.
	PromptPositional bool `json:"prompt_positional,omitempty"`
	// PermissionsFlag is the CLI flag used to grant additional permissions.
	PermissionsFlag string `json:"permissions_flag,omitempty"`
	// InstructionsFile is the filename the agent reads for task instructions (e.g. "AGENTS.md").
	InstructionsFile string `json:"instructions_file,omitempty"`
	// ReadyPromptPrefix is text that signals the agent is ready to receive input.
	ReadyPromptPrefix string `json:"ready_prompt_prefix,omitempty"`
	// ResumeFlag is the CLI flag used to resume a previous session (e.g. "--resume").
	ResumeFlag string `json:"resume_flag,omitempty"`
	// ExtraEnv maps additional environment variable names to values injected into
	// the agent process. These are set in addition to (and may override) EnvPassthrough.
	ExtraEnv map[string]string `json:"extra_env,omitempty"`
	// DefaultModel is the model value passed via ModelFlag when launching the agent.
	// An empty string means the agent's own default is used.
	DefaultModel string `json:"default_model,omitempty"`
	// NonInteractive describes how to invoke this agent in single-shot
	// (non-interactive) mode for filtration.
	NonInteractive NonInteractiveConfig `json:"non_interactive,omitempty"`
	// PromptFileTemplate is the filename within the worktree where the full prompt
	// is written before spawn. When set, the full prompt content is written to this
	// file and a short --prompt referencing it is used instead. This avoids tmux
	// command-line length limits for providers with very long prompts.
	// The template may contain "{identity}" as a placeholder for the resolved
	// identity file path.
	// For providers that support AgentFlag, this is typically empty — the prompt
	// is delivered via the agent markdown file instead.
	PromptFileTemplate string `json:"prompt_file_template,omitempty"`
	// SupportsAddDir indicates whether this provider supports the AddDirFlag for
	// filesystem-based context injection (e.g. SKILL.md, instructions files).
	// When false, context must be injected as text in the prompt preamble.
	SupportsAddDir bool `json:"supports_add_dir,omitempty"`
	// AgentFlag is the CLI flag used to select a named agent (e.g. "--agent").
	// When set, Cistern writes the cataractae prompt as an agent markdown file
	// under OPENCODE_CONFIG_DIR and passes the identity name via this flag.
	// This avoids writing any instructions file to the worktree root, preventing
	// contamination of the repository's own AGENTS.md/CLAUDE.md.
	AgentFlag string `json:"agent_flag,omitempty"`
}

// InstrFile returns InstructionsFile, defaulting to "AGENTS.md" when empty.
func (p ProviderPreset) InstrFile() string {
	if p.InstructionsFile != "" {
		return p.InstructionsFile
	}
	return "AGENTS.md"
}

// builtins is the canonical set of provider presets shipped with Cistern.
var 	builtins = []ProviderPreset{
	{
		Name:               "opencode",
		Command:            "opencode",
		Subcommand:         "run",
		Args:               []string{"--dangerously-skip-permissions"},
		ModelFlag:          "--model",
		DefaultModel:       "ollama/glm-5.1:cloud",
		PromptPositional:   true,
		InstructionsFile:   "AGENTS.md",
		AgentFlag:          "--agent",
		ResumeFlag:         "-s",
		NonInteractive:     NonInteractiveConfig{
			Subcommand: "run",
			FormatArgs: []string{"--format", "json"},
		},
	},
}

// cloneSliceFields deep-copies all slice fields of a ProviderPreset so the
// copy does not alias the original's backing arrays. This includes nested slice
// fields inside NonInteractiveConfig.
func cloneSliceFields(p *ProviderPreset) {
	p.Args = slices.Clone(p.Args)
	p.EnvPassthrough = slices.Clone(p.EnvPassthrough)
	p.NonInteractive.FormatArgs = slices.Clone(p.NonInteractive.FormatArgs)
}

// Builtins returns a deep copy of the built-in provider preset slice.
// Callers may safely modify the returned slice and its fields without affecting the originals.
func Builtins() []ProviderPreset {
	out := make([]ProviderPreset, len(builtins))
	for i, p := range builtins {
		cloneSliceFields(&p)
		p.ExtraEnv = maps.Clone(p.ExtraEnv)
		out[i] = p
	}
	return out
}

// MergePresets applies overrides on top of base and returns the merged slice.
// Entries in overrides that match a base preset by Name replace it; entries
// with unknown names are appended. Neither slice is modified.
func MergePresets(base, overrides []ProviderPreset) []ProviderPreset {
	result := make([]ProviderPreset, len(base))
	for i, p := range base {
		cloneSliceFields(&p)
		result[i] = p
	}
	for _, u := range overrides {
		idx := slices.IndexFunc(result, func(p ProviderPreset) bool {
			return p.Name == u.Name
		})
		cloneSliceFields(&u)
		if idx >= 0 {
			result[idx] = u
		} else {
			result = append(result, u)
		}
	}
	return result
}

// ResolvePreset returns the built-in preset matching name.
// If name is empty or no preset matches, the "opencode" preset is returned as
// the default fallback.
func ResolvePreset(name string) ProviderPreset {
	builtins := Builtins()
	for _, p := range builtins {
		if p.Name == name {
			return p
		}
	}
	// Default: fall back to the opencode preset by explicit name lookup.
	for _, p := range builtins {
		if p.Name == "opencode" {
			return p
		}
	}
	// Unreachable: opencode is always in the built-in set.
	return builtins[0]
}

// LoadUserPresets reads a JSON array of ProviderPreset values from path and
// merges them on top of the built-in presets. A user entry with a Name that
// matches a built-in replaces the built-in; entries with unknown names are
// appended. If path does not exist the built-ins are returned unchanged.
func LoadUserPresets(path string) ([]ProviderPreset, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return Builtins(), nil
	}
	if err != nil {
		return nil, fmt.Errorf("provider: read %s: %w", path, err)
	}

	var user []ProviderPreset
	if err := json.Unmarshal(data, &user); err != nil {
		return nil, fmt.Errorf("provider: parse %s: %w", path, err)
	}

	return MergePresets(Builtins(), user), nil
}
