package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInit_CreatesDirectoryStructure(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	initForce = false

	if err := initCmd.RunE(initCmd, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	citadelDir := filepath.Join(home, ".citadel")
	for _, dir := range []string{
		citadelDir,
		filepath.Join(citadelDir, "workflows"),
		filepath.Join(citadelDir, "roles"),
	} {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			t.Errorf("expected directory to exist: %s", dir)
		}
	}
}

func TestInit_WritesCitadelYAML(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	initForce = false

	if err := initCmd.RunE(initCmd, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	configFile := filepath.Join(home, ".citadel", "citadel.yaml")
	data, err := os.ReadFile(configFile)
	if err != nil {
		t.Fatalf("citadel.yaml not created: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("citadel.yaml is empty")
	}
	// Verify it matches the embedded template.
	if string(data) != string(defaultCitadelConfig) {
		t.Error("citadel.yaml content does not match embedded template")
	}
}

func TestInit_CopiesWorkflowFiles(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	initForce = false

	if err := initCmd.RunE(initCmd, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	workflowsDir := filepath.Join(home, ".citadel", "workflows")
	for _, name := range []string{"feature.yaml", "bug.yaml"} {
		path := filepath.Join(workflowsDir, name)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("expected workflow file to exist: %s", name)
		}
	}

	// Verify feature.yaml content matches embedded template.
	featureData, err := os.ReadFile(filepath.Join(workflowsDir, "feature.yaml"))
	if err != nil {
		t.Fatalf("read feature.yaml: %v", err)
	}
	if string(featureData) != string(defaultFeatureWorkflow) {
		t.Error("feature.yaml content does not match embedded template")
	}
}

func TestInit_GeneratesRoles(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	initForce = false

	if err := initCmd.RunE(initCmd, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	rolesDir := filepath.Join(home, ".citadel", "roles")
	entries, err := os.ReadDir(rolesDir)
	if err != nil {
		t.Fatalf("read roles dir: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("no roles were generated")
	}

	// Each role should have a CLAUDE.md.
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		claudeMD := filepath.Join(rolesDir, entry.Name(), "CLAUDE.md")
		if _, err := os.Stat(claudeMD); os.IsNotExist(err) {
			t.Errorf("missing CLAUDE.md for role %q", entry.Name())
		}
	}
}

func TestInit_SkipsExistingFilesWithoutForce(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	initForce = false

	// First run to create files.
	if err := initCmd.RunE(initCmd, nil); err != nil {
		t.Fatalf("first run error: %v", err)
	}

	// Overwrite citadel.yaml with sentinel content.
	configFile := filepath.Join(home, ".citadel", "citadel.yaml")
	sentinel := []byte("# sentinel — must not be overwritten")
	if err := os.WriteFile(configFile, sentinel, 0o644); err != nil {
		t.Fatalf("write sentinel: %v", err)
	}

	// Second run without --force must not overwrite.
	initForce = false
	if err := initCmd.RunE(initCmd, nil); err != nil {
		t.Fatalf("second run error: %v", err)
	}

	data, err := os.ReadFile(configFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(sentinel) {
		t.Error("citadel.yaml was overwritten without --force")
	}
}

func TestInit_ForceOverwritesExistingFiles(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// First run to create files.
	initForce = false
	if err := initCmd.RunE(initCmd, nil); err != nil {
		t.Fatalf("first run error: %v", err)
	}

	// Overwrite citadel.yaml with sentinel content.
	configFile := filepath.Join(home, ".citadel", "citadel.yaml")
	sentinel := []byte("# sentinel — must be overwritten with --force")
	if err := os.WriteFile(configFile, sentinel, 0o644); err != nil {
		t.Fatalf("write sentinel: %v", err)
	}

	// Run with --force — must overwrite.
	initForce = true
	defer func() { initForce = false }()

	if err := initCmd.RunE(initCmd, nil); err != nil {
		t.Fatalf("force run error: %v", err)
	}

	data, err := os.ReadFile(configFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) == string(sentinel) {
		t.Error("citadel.yaml was not overwritten with --force")
	}
	if string(data) != string(defaultCitadelConfig) {
		t.Error("citadel.yaml does not match embedded template after --force")
	}
}

func TestInit_IdempotentWithoutForce(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	initForce = false

	// Run twice — both must succeed with no errors.
	for i := 0; i < 2; i++ {
		if err := initCmd.RunE(initCmd, nil); err != nil {
			t.Fatalf("run %d error: %v", i+1, err)
		}
	}
}
