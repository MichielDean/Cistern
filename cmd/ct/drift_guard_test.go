package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/MichielDean/cistern/internal/aqueduct"
)

// --- aqueduct skill wiring drift guard ---

// repoRoot resolves the repository root from the test's location so the test
// works from any checkout and in CI.
func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for dir := wd; ; dir = filepath.Dir(dir) {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		if dir == filepath.Dir(dir) {
			t.Fatal("could not locate repo root (no go.mod upward)")
		}
	}
}

// shippedSkills returns the SKILL.md directory names under internal/skills/.
func shippedSkills(t *testing.T, root string) map[string]bool {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(root, "internal", "skills"))
	if err != nil {
		t.Fatalf("read internal/skills: %v", err)
	}
	skills := map[string]bool{}
	for _, e := range entries {
		if e.IsDir() {
			skills[e.Name()] = true
		}
	}
	return skills
}

// TestAqueductSkillsShip verifies every skill referenced by the repository's
// aqueduct (aqueduct/aqueduct.yaml) ships in internal/skills/. Without this,
// a wiring change passes locally (where skills installed ad hoc) and breaks
// at runtime — validateWorkflowSkills fails at agent spawn.
func TestAqueductSkillsShip(t *testing.T) {
	root := repoRoot(t)

	workflow, err := aqueduct.ParseWorkflow(filepath.Join(root, "aqueduct", "aqueduct.yaml"))
	if err != nil {
		t.Fatalf("parse aqueduct/aqueduct.yaml: %v", err)
	}

	shipped := shippedSkills(t, root)
	var missing []string
	for _, step := range workflow.Cataractae {
		for _, skill := range step.Skills {
			name := skill.Name
			if name == "" || shipped[name] {
				continue
			}
			missing = append(missing, step.Name+" -> "+name)
		}
	}
	if len(missing) > 0 {
		t.Errorf("aqueduct references skills that do not ship in internal/skills/ — add skill dirs or fix wiring:\n  %s",
			strings.Join(missing, "\n  "))
	}
}

// TestShippedDefaultsNoRetiredModels verifies the shipped/evaluated config
// surfaces never pin a retired provider model. glm-5.1:cloud was retired by
// the upstream provider on 2026-09-25 and silently killed every cataractae
// spawn on a live host for 46 hours before anyone noticed (the loop rate is
// now caught by evals/metrics, but the default must never reintroduce it).
func TestShippedDefaultsNoRetiredModels(t *testing.T) {
	root := repoRoot(t)
	retired := []string{"glm-5.1:cloud"}

	surfaces := []string{
		filepath.Join(root, "cmd", "ct", "assets", "cistern.yaml"),
		filepath.Join(root, "cistern.yaml"),
		filepath.Join(root, "aqueduct", "aqueduct.yaml"),
	}
	for _, path := range surfaces {
		data, err := os.ReadFile(path)
		if err != nil {
			continue // file may not exist in this repo layout
		}
		for _, model := range retired {
			if strings.Contains(string(data), model) {
				t.Errorf("%s pins retired model %q — every spawn using it fails (see evals/metrics BASELINE)", path, model)
			}
		}
	}
}

// TestCataractaeInstructionsReferenceShippedSkills verifies the stage identity
// files only point at skill names that ship — catching instruction edits that
// name a skill nobody deployed (the "skill missing" spawn failure class).
func TestCataractaeInstructionsReferenceShippedSkills(t *testing.T) {
	root := repoRoot(t)
	shipped := shippedSkills(t, root)

	identities, err := os.ReadDir(filepath.Join(root, "cataractae"))
	if err != nil {
		t.Fatalf("read cataractae/: %v", err)
	}
	var missing []string
	seen := 0
	for _, identity := range identities {
		if !identity.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(root, "cataractae", identity.Name(), "INSTRUCTIONS.md"))
		if err != nil {
			continue // identity without instructions is fine
		}
		for _, match := range skillNamePattern.FindAllStringSubmatch(string(data), -1) {
			seen++
			name := match[1]
			if !shipped[name] {
				missing = append(missing, identity.Name()+": "+strings.TrimSpace(match[0]))
			}
		}
	}
	if len(missing) > 0 {
		t.Errorf("INSTRUCTIONS.md files reference skills that do not ship:\n  %s",
			strings.Join(missing, "\n  "))
	}
	if seen < 5 {
		t.Fatalf("reference scan matched only %d skill pointers — pattern is likely stale; cataractae instructions reference skills via **name** and `` `name` skill`` mentions. Fix the pattern before trusting this guard.", seen)
	}
}

// skillNamePattern matches skill-name tokens wherever they are referenced in
// INSTRUCTIONS.md (bold **name**, `name` skill, or plain prose). Any mention
// of a cistern-/mp- skill we do not ship is a drift signal.
var skillNamePattern = regexp.MustCompile(`\b((?:cistern|mp)-[a-z0-9-]+)\b`)
