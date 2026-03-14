package main

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"

	"github.com/MichielDean/citadel/internal/workflow"
	"github.com/spf13/cobra"
)

//go:embed assets/citadel.yaml
var defaultCitadelConfig []byte

//go:embed assets/workflows/feature.yaml
var defaultFeatureWorkflow []byte

//go:embed assets/workflows/bug.yaml
var defaultBugWorkflow []byte

var initForce bool

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Bootstrap a new Citadel installation",
	RunE:  runInit,
}

func runInit(cmd *cobra.Command, args []string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("cannot determine home directory: %w", err)
	}

	citadelDir := filepath.Join(home, ".citadel")
	workflowsDir := filepath.Join(citadelDir, "workflows")
	rolesDir := filepath.Join(citadelDir, "roles")

	// 1. Create directory structure.
	for _, dir := range []string{citadelDir, workflowsDir, rolesDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create directory %s: %w", dir, err)
		}
	}

	// 2. Write citadel.yaml from embedded template.
	configDst := filepath.Join(citadelDir, "citadel.yaml")
	if err := writeFileIfAbsent(configDst, defaultCitadelConfig, initForce); err != nil {
		return err
	}

	// 3. Copy default workflow files.
	workflows := []struct {
		name string
		data []byte
	}{
		{"feature.yaml", defaultFeatureWorkflow},
		{"bug.yaml", defaultBugWorkflow},
	}
	for _, wf := range workflows {
		dst := filepath.Join(workflowsDir, wf.name)
		if err := writeFileIfAbsent(dst, wf.data, initForce); err != nil {
			return err
		}
	}

	// 4. Generate role files from the feature workflow.
	featureWfPath := filepath.Join(workflowsDir, "feature.yaml")
	w, err := workflow.ParseWorkflow(featureWfPath)
	if err != nil {
		return fmt.Errorf("parse feature workflow: %w", err)
	}
	if len(w.Roles) > 0 {
		if _, err := workflow.GenerateRoleFiles(w, rolesDir); err != nil {
			return fmt.Errorf("generate roles: %w", err)
		}
	}

	// 5. Print next-steps message.
	fmt.Printf(`Citadel initialized.
  Config   : ~/.citadel/citadel.yaml
  Workflows: ~/.citadel/workflows/
  Roles    : ~/.citadel/roles/

Next:
  1. Edit ~/.citadel/citadel.yaml — add your repos
  2. ct cistern add --title "Your first drop" --repo yourrepo
  3. ct flow start
`)
	return nil
}

// writeFileIfAbsent writes data to path. If the file already exists and force
// is false, it prints a warning to stderr and skips the write.
func writeFileIfAbsent(path string, data []byte, force bool) error {
	if !force {
		if _, err := os.Stat(path); err == nil {
			fmt.Fprintf(os.Stderr, "warning: %s already exists, skipping (use --force to overwrite)\n", path)
			return nil
		}
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

func init() {
	initCmd.Flags().BoolVar(&initForce, "force", false, "overwrite existing files")
	rootCmd.AddCommand(initCmd)
}
