package provider

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestBuiltins_ReturnsExpectedPresetNames verifies the built-in presets are present.
func TestBuiltins_ReturnsExpectedPresetNames(t *testing.T) {
	want := []string{"opencode"}
	got := Builtins()

	if len(got) != len(want) {
		t.Fatalf("Builtins() returned %d presets, want %d", len(got), len(want))
	}

	byName := make(map[string]ProviderPreset)
	for _, p := range got {
		byName[p.Name] = p
	}
	for _, name := range want {
		if _, ok := byName[name]; !ok {
			t.Errorf("built-in preset %q not found", name)
		}
	}
}

// TestBuiltins_OpencodePreset validates each field of the opencode built-in.
func TestBuiltins_OpencodePreset(t *testing.T) {
	got := builtinByName(t, "opencode")

	assertStr(t, "Command", "opencode", got.Command)
	assertStrs(t, "Args", []string{"--dangerously-skip-permissions"}, got.Args)
	assertStr(t, "Subcommand", "run", got.Subcommand)
	assertStr(t, "ModelFlag", "--model", got.ModelFlag)
	assertStr(t, "InstructionsFile", "AGENTS.md", got.InstructionsFile)
	assertStr(t, "AgentFlag", "--agent", got.AgentFlag)
	assertStr(t, "PromptFileTemplate", "", got.PromptFileTemplate)
}

// TestBuiltins_NonInteractiveConfig verifies the NonInteractive fields for the opencode preset.
func TestBuiltins_NonInteractiveConfig(t *testing.T) {
	tests := []struct {
		name       string
		subcommand string
		promptFlag string
	}{
		{"opencode", "run", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := builtinByName(t, tt.name)
			assertStr(t, "NonInteractive.Subcommand", tt.subcommand, p.NonInteractive.Subcommand)
			assertStr(t, "NonInteractive.PromptFlag", tt.promptFlag, p.NonInteractive.PromptFlag)
		})
	}
}

// TestBuiltins_OpencodeFormatArgs verifies the opencode builtin preset has
// empty FormatArgs (removed because opencode --format is a top-level flag,
// not a run subcommand flag; the tmux-based filter approach uses RESPONSE.md
// instead of JSON output).
func TestBuiltins_OpencodeFormatArgs(t *testing.T) {
	p := builtinByName(t, "opencode")
	assertStrs(t, "NonInteractive.FormatArgs", nil, p.NonInteractive.FormatArgs)
}

// TestBuiltins_OpencodeResumeFlag verifies the opencode builtin preset has
// ResumeFlag set to "-s".
func TestBuiltins_OpencodeResumeFlag(t *testing.T) {
	p := builtinByName(t, "opencode")
	assertStr(t, "ResumeFlag", "-s", p.ResumeFlag)
}

// TestBuiltins_ReturnsCopy verifies that mutating the returned slice does not affect the built-ins.
func TestBuiltins_ReturnsCopy(t *testing.T) {
	t.Run("string field mutation is isolated", func(t *testing.T) {
		first := Builtins()
		first[0].Command = "mutated"

		second := Builtins()
		if second[0].Command == "mutated" {
			t.Error("Builtins() returned a reference to internal state, want an independent copy")
		}
	})

	t.Run("slice field mutation is isolated", func(t *testing.T) {
		first := Builtins()
		original := first[0].Args[0]
		first[0].Args[0] = "mutated"

		second := Builtins()
		if second[0].Args[0] != original {
			t.Errorf("Builtins() Args[0] = %q after mutation, want %q — slice field shares backing array with global state", second[0].Args[0], original)
		}
	})

	t.Run("extra_env map mutation is isolated", func(t *testing.T) {
		first := Builtins()
		first[0].ExtraEnv = map[string]string{"injected": "value"}

		second := Builtins()
		if second[0].ExtraEnv != nil {
			t.Error("Builtins() ExtraEnv is not isolated — mutation leaked into global state")
		}
	})

	t.Run("NonInteractive.FormatArgs mutation is isolated", func(t *testing.T) {
		first := Builtins()
		// FormatArgs is nil for opencode preset (removed); skip if empty
		if len(first[0].NonInteractive.FormatArgs) == 0 {
			t.Skip("preset has no FormatArgs to test isolation")
		}
		original := first[0].NonInteractive.FormatArgs[0]
		first[0].NonInteractive.FormatArgs[0] = "mutated"

		second := Builtins()
		if second[0].NonInteractive.FormatArgs[0] == "mutated" {
			t.Error("Builtins() NonInteractive.FormatArgs is not isolated — mutation leaked into global state")
		}
		if second[0].NonInteractive.FormatArgs[0] != original {
			t.Errorf("Builtins() FormatArgs[0] = %q after mutation, want %q", second[0].NonInteractive.FormatArgs[0], original)
		}
	})
}

// TestLoadUserPresets_NoFileReturnsBuiltins verifies that a missing file returns built-ins unchanged.
func TestLoadUserPresets_NoFileReturnsBuiltins(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nonexistent.json")

	presets, err := LoadUserPresets(path)
	if err != nil {
		t.Fatalf("LoadUserPresets: unexpected error: %v", err)
	}

	want := Builtins()
	if len(presets) != len(want) {
		t.Errorf("got %d presets, want %d", len(presets), len(want))
	}
}

// TestLoadUserPresets_OverridesBuiltinByName verifies user entries replace built-ins with matching names.
func TestLoadUserPresets_OverridesBuiltinByName(t *testing.T) {
	override := ProviderPreset{Name: "opencode", Command: "my-opencode"}
	path := writePresetsJSON(t, []ProviderPreset{override})

	presets, err := LoadUserPresets(path)
	if err != nil {
		t.Fatalf("LoadUserPresets: %v", err)
	}

	got := findByName(presets, "opencode")
	if got == nil {
		t.Fatal("opencode preset not found after override")
	}
	assertStr(t, "Command", "my-opencode", got.Command)
}

// TestLoadUserPresets_AppendsUnknownPreset verifies that unknown presets are appended.
func TestLoadUserPresets_AppendsUnknownPreset(t *testing.T) {
	extra := ProviderPreset{Name: "custom", Command: "my-agent"}
	path := writePresetsJSON(t, []ProviderPreset{extra})

	presets, err := LoadUserPresets(path)
	if err != nil {
		t.Fatalf("LoadUserPresets: %v", err)
	}

	got := findByName(presets, "custom")
	if got == nil {
		t.Fatal("custom preset not found after merge")
	}
	assertStr(t, "Command", "my-agent", got.Command)

	// Built-ins must still be present.
	if findByName(presets, "opencode") == nil {
		t.Error("opencode built-in missing after user preset append")
	}
}

// TestLoadUserPresets_InvalidJSONReturnsError verifies that malformed JSON returns an error.
func TestLoadUserPresets_InvalidJSONReturnsError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.json")
	if err := os.WriteFile(path, []byte("not valid json"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadUserPresets(path)
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

// TestLoadUserPresets_MultipleOverridesAndAppends exercises a realistic mixed JSON file.
func TestLoadUserPresets_MultipleOverridesAndAppends(t *testing.T) {
	user := []ProviderPreset{
		{Name: "opencode", Command: "opencode-dev", ModelFlag: "--model-override"},
		{Name: "new-agent", Command: "new-agent-bin", InstructionsFile: "NEW.md"},
	}
	path := writePresetsJSON(t, user)

	presets, err := LoadUserPresets(path)
	if err != nil {
		t.Fatalf("LoadUserPresets: %v", err)
	}

	opencode := findByName(presets, "opencode")
	if opencode == nil {
		t.Fatal("opencode not found")
	}
	assertStr(t, "opencode Command", "opencode-dev", opencode.Command)
	assertStr(t, "opencode ModelFlag", "--model-override", opencode.ModelFlag)

	newAgent := findByName(presets, "new-agent")
	if newAgent == nil {
		t.Fatal("new-agent not found")
	}
	assertStr(t, "new-agent InstructionsFile", "NEW.md", newAgent.InstructionsFile)
}

// TestProviderConfigMerge verifies that MergePresets correctly applies layered
// overrides: repo-level overrides shadow top-level ones, and top-level overrides
// shadow built-in preset defaults.
func TestProviderConfigMerge(t *testing.T) {
	t.Run("top-level shadows built-in defaults", func(t *testing.T) {
		topLevel := []ProviderPreset{
			{Name: "opencode", Command: "opencode-custom", ModelFlag: "--custom-model"},
		}
		merged := MergePresets(Builtins(), topLevel)

		opencode := findByName(merged, "opencode")
		if opencode == nil {
			t.Fatal("opencode not found after top-level override")
		}
		assertStr(t, "Command", "opencode-custom", opencode.Command)
		assertStr(t, "ModelFlag", "--custom-model", opencode.ModelFlag)
	})

	t.Run("repo-level shadows top-level", func(t *testing.T) {
		afterTopLevel := MergePresets(Builtins(), []ProviderPreset{
			{Name: "opencode", Command: "opencode-top-level"},
		})
		afterRepoLevel := MergePresets(afterTopLevel, []ProviderPreset{
			{Name: "opencode", Command: "opencode-repo"},
		})

		opencode := findByName(afterRepoLevel, "opencode")
		if opencode == nil {
			t.Fatal("opencode not found after repo-level override")
		}
		assertStr(t, "Command", "opencode-repo", opencode.Command)
	})

	t.Run("new preset appended survives further merge", func(t *testing.T) {
		afterTopLevel := MergePresets(Builtins(), []ProviderPreset{
			{Name: "project-agent", Command: "project-bin"},
		})
		afterRepoLevel := MergePresets(afterTopLevel, []ProviderPreset{
			{Name: "project-agent", Command: "repo-bin"},
		})

		agent := findByName(afterRepoLevel, "project-agent")
		if agent == nil {
			t.Fatal("project-agent not found after repo-level merge")
		}
		assertStr(t, "Command", "repo-bin", agent.Command)

		// Original built-ins are still intact.
		if findByName(afterRepoLevel, "opencode") == nil {
			t.Error("opencode missing after layered merge")
		}
	})

	t.Run("MergePresets does not mutate base slice", func(t *testing.T) {
		base := Builtins()
		originalCmd := base[0].Command
		MergePresets(base, []ProviderPreset{{Name: base[0].Name, Command: "mutated"}})
		if base[0].Command != originalCmd {
			t.Errorf("MergePresets mutated base slice: Command = %q, want %q", base[0].Command, originalCmd)
		}
	})

	t.Run("MergePresets does not alias override slice fields", func(t *testing.T) {
		override := ProviderPreset{
			Name:           "custom",
			Args:           []string{"--flag"},
			EnvPassthrough: []string{"MY_KEY"},
		}
		merged := MergePresets(Builtins(), []ProviderPreset{override})

		got := findByName(merged, "custom")
		if got == nil {
			t.Fatal("custom not found in merged result")
		}

		override.Args[0] = "mutated-after-merge"
		override.EnvPassthrough[0] = "MUTATED_KEY"

		if got.Args[0] == "mutated-after-merge" {
			t.Error("MergePresets aliased override Args: mutation of original override propagated into merged result")
		}
		if got.EnvPassthrough[0] == "MUTATED_KEY" {
			t.Error("MergePresets aliased override EnvPassthrough: mutation of original override propagated into merged result")
		}
	})
}

// TestUserPresetsJSON writes a providers.json with both an override and a custom
// preset, loads it via LoadUserPresets, and verifies the result merges correctly
// and the custom provider is resolvable.
func TestUserPresetsJSON(t *testing.T) {
	user := []ProviderPreset{
		{
			Name:      "opencode",
			Command:   "opencode-patched",
			Args:      []string{"--dangerously-skip-permissions", "--extra-flag"},
			ModelFlag: "--model",
		},
		{
			Name:             "my-custom-agent",
			Command:          "my-custom-bin",
			Args:             []string{"--no-confirm"},
			EnvPassthrough:   []string{"MY_KEY"},
			InstructionsFile: "MY.md",
		},
	}
	path := writePresetsJSON(t, user)

	presets, err := LoadUserPresets(path)
	if err != nil {
		t.Fatalf("LoadUserPresets: %v", err)
	}

	opencode := findByName(presets, "opencode")
	if opencode == nil {
		t.Fatal("opencode not found after JSON load")
	}
	assertStr(t, "opencode Command", "opencode-patched", opencode.Command)
	assertStrs(t, "opencode Args", []string{"--dangerously-skip-permissions", "--extra-flag"}, opencode.Args)
	assertStr(t, "opencode ModelFlag", "--model", opencode.ModelFlag)

	custom := findByName(presets, "my-custom-agent")
	if custom == nil {
		t.Fatal("my-custom-agent not found after JSON load")
	}
	assertStr(t, "custom Command", "my-custom-bin", custom.Command)
	assertStrs(t, "custom Args", []string{"--no-confirm"}, custom.Args)
	assertStrs(t, "custom EnvPassthrough", []string{"MY_KEY"}, custom.EnvPassthrough)
	assertStr(t, "custom InstructionsFile", "MY.md", custom.InstructionsFile)
}

// TestResolvePreset_DefaultsToOpencodeWhenEmpty verifies that ResolvePreset("")
// returns the "opencode" preset as the default fallback.
func TestResolvePreset_DefaultsToOpencodeWhenEmpty(t *testing.T) {
	p := ResolvePreset("")
	if p.Name != "opencode" {
		t.Errorf("ResolvePreset(\"\") = %q, want %q", p.Name, "opencode")
	}
}

// TestResolvePreset_ReturnsMatchByName verifies that a known name is resolved.
func TestResolvePreset_ReturnsMatchByName(t *testing.T) {
	for _, name := range []string{"opencode"} {
		p := ResolvePreset(name)
		if p.Name != name {
			t.Errorf("ResolvePreset(%q) = %q, want %q", name, p.Name, name)
		}
	}
}

// TestResolvePreset_UnknownNameFallsBackToOpencode verifies the fallback for
// an unknown provider name.
func TestResolvePreset_UnknownNameFallsBackToOpencode(t *testing.T) {
	p := ResolvePreset("does-not-exist")
	if p.Name != "opencode" {
		t.Errorf("ResolvePreset(\"does-not-exist\") = %q, want %q", p.Name, "opencode")
	}
}

// --- helpers ---

func builtinByName(t *testing.T, name string) ProviderPreset {
	t.Helper()
	for _, p := range Builtins() {
		if p.Name == name {
			return p
		}
	}
	t.Fatalf("built-in preset %q not found", name)
	return ProviderPreset{}
}

func findByName(presets []ProviderPreset, name string) *ProviderPreset {
	for i := range presets {
		if presets[i].Name == name {
			return &presets[i]
		}
	}
	return nil
}

func assertStr(t *testing.T, field, want, got string) {
	t.Helper()
	if got != want {
		t.Errorf("%s = %q, want %q", field, got, want)
	}
}

func assertStrs(t *testing.T, field string, want, got []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("%s: got %v (len %d), want %v (len %d)", field, got, len(got), want, len(want))
		return
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("%s[%d] = %q, want %q", field, i, got[i], want[i])
		}
	}
}

func writePresetsJSON(t *testing.T, presets []ProviderPreset) string {
	t.Helper()
	data, err := json.Marshal(presets)
	if err != nil {
		t.Fatalf("marshal test JSON: %v", err)
	}
	path := filepath.Join(t.TempDir(), "providers.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write test JSON: %v", err)
	}
	return path
}
