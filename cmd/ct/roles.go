package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/MichielDean/citadel/internal/workflow"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var rolesCmd = &cobra.Command{
	Use:   "roles",
	Short: "Manage agent role definitions",
}

// --- roles generate ---

var rolesGenerateWorkflow string

var rolesGenerateCmd = &cobra.Command{
	Use:   "generate",
	Short: "Generate CLAUDE.md files from workflow role definitions",
	RunE:  runRolesGenerate,
}

func runRolesGenerate(cmd *cobra.Command, args []string) error {
	wfPath := rolesGenerateWorkflow
	if wfPath == "" {
		// Try to find workflow from config.
		cfgPath := resolveConfigPath()
		cfg, err := workflow.ParseFarmConfig(cfgPath)
		if err != nil {
			return fmt.Errorf("loading config: %w (use --workflow to specify a workflow file directly)", err)
		}
		if len(cfg.Repos) == 0 {
			return fmt.Errorf("no repos configured")
		}
		// Use the first repo's workflow.
		wfPath = cfg.Repos[0].WorkflowPath
		if !filepath.IsAbs(wfPath) {
			wfPath = filepath.Join(filepath.Dir(cfgPath), wfPath)
		}
	}

	w, err := workflow.ParseWorkflow(wfPath)
	if err != nil {
		return fmt.Errorf("parse workflow: %w", err)
	}

	if len(w.Roles) == 0 {
		fmt.Println("no roles defined in workflow")
		return nil
	}

	rolesDir := citadelRolesDir()
	written, err := workflow.GenerateRoleFiles(w, rolesDir)
	if err != nil {
		return err
	}

	for _, path := range written {
		fmt.Printf("wrote %s\n", path)
	}
	fmt.Printf("\n%d role(s) generated in %s\n", len(written), rolesDir)
	return nil
}

// --- roles list ---

var rolesListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all roles defined in the workflow YAML",
	RunE:  runRolesList,
}

func runRolesList(cmd *cobra.Command, args []string) error {
	wfPath := rolesGenerateWorkflow
	if wfPath == "" {
		cfgPath := resolveConfigPath()
		cfg, err := workflow.ParseFarmConfig(cfgPath)
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}
		if len(cfg.Repos) == 0 {
			return fmt.Errorf("no repos configured")
		}
		wfPath = cfg.Repos[0].WorkflowPath
		if !filepath.IsAbs(wfPath) {
			wfPath = filepath.Join(filepath.Dir(cfgPath), wfPath)
		}
	}

	w, err := workflow.ParseWorkflow(wfPath)
	if err != nil {
		return fmt.Errorf("parse workflow: %w", err)
	}

	if len(w.Roles) == 0 {
		fmt.Println("no roles defined in workflow")
		return nil
	}

	// Sort keys for stable output.
	keys := make([]string, 0, len(w.Roles))
	for k := range w.Roles {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		role := w.Roles[k]
		desc := role.Description
		if len(desc) > 40 {
			desc = desc[:37] + "..."
		}
		fmt.Printf("  %-20s %-40s ct roles edit %s\n", k, desc, k)
	}
	return nil
}

// --- roles edit ---

var rolesEditCmd = &cobra.Command{
	Use:   "edit",
	Short: "Edit a role's instructions in $EDITOR",
	RunE:  runRolesEdit,
}

func runRolesEdit(cmd *cobra.Command, args []string) error {
	wfPath := rolesGenerateWorkflow
	if wfPath == "" {
		cfgPath := resolveConfigPath()
		cfg, err := workflow.ParseFarmConfig(cfgPath)
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}
		if len(cfg.Repos) == 0 {
			return fmt.Errorf("no repos configured")
		}
		wfPath = cfg.Repos[0].WorkflowPath
		if !filepath.IsAbs(wfPath) {
			wfPath = filepath.Join(filepath.Dir(cfgPath), wfPath)
		}
	}

	// Read the raw YAML to preserve structure.
	data, err := os.ReadFile(wfPath)
	if err != nil {
		return fmt.Errorf("read workflow: %w", err)
	}

	w, err := workflow.ParseWorkflow(wfPath)
	if err != nil {
		return fmt.Errorf("parse workflow: %w", err)
	}

	if len(w.Roles) == 0 {
		fmt.Println("no roles defined in workflow")
		return nil
	}

	// Sort keys for stable ordering.
	keys := make([]string, 0, len(w.Roles))
	for k := range w.Roles {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Print numbered list.
	fmt.Println("Select a role to edit:")
	for i, k := range keys {
		fmt.Printf("  %d. %s — %s\n", i+1, k, w.Roles[k].Name)
	}
	fmt.Print("\nEnter number: ")

	var input string
	fmt.Scanln(&input)
	idx, err := strconv.Atoi(input)
	if err != nil || idx < 1 || idx > len(keys) {
		return fmt.Errorf("invalid selection: %q", input)
	}

	selectedKey := keys[idx-1]
	role := w.Roles[selectedKey]

	// Write instructions to temp file.
	tmpFile, err := os.CreateTemp("", "citadel-role-*.md")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	if _, err := tmpFile.WriteString(role.Instructions); err != nil {
		tmpFile.Close()
		return fmt.Errorf("write temp file: %w", err)
	}
	tmpFile.Close()

	// Open in $EDITOR.
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}
	editorCmd := exec.Command(editor, tmpPath)
	editorCmd.Stdin = os.Stdin
	editorCmd.Stdout = os.Stdout
	editorCmd.Stderr = os.Stderr
	if err := editorCmd.Run(); err != nil {
		return fmt.Errorf("editor: %w", err)
	}

	// Read back edited content.
	edited, err := os.ReadFile(tmpPath)
	if err != nil {
		return fmt.Errorf("read edited file: %w", err)
	}

	// Update role in the workflow.
	role.Instructions = string(edited)
	w.Roles[selectedKey] = role

	// Re-parse the raw data into a generic structure to preserve
	// non-role fields, then update roles and re-serialize.
	var raw map[string]interface{}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("parse raw yaml: %w", err)
	}
	raw["roles"] = w.Roles

	out, err := yaml.Marshal(raw)
	if err != nil {
		return fmt.Errorf("marshal yaml: %w", err)
	}
	if err := os.WriteFile(wfPath, out, 0o644); err != nil {
		return fmt.Errorf("write workflow: %w", err)
	}

	// Regenerate CLAUDE.md.
	rolesDir := citadelRolesDir()
	written, err := workflow.GenerateRoleFiles(w, rolesDir)
	if err != nil {
		return err
	}

	fmt.Printf("\nUpdated %s and regenerated:\n", wfPath)
	for _, path := range written {
		fmt.Printf("  %s\n", path)
	}
	return nil
}

// --- roles reset ---

var rolesResetCmd = &cobra.Command{
	Use:   "reset [role]",
	Short: "Restore a role to its built-in default (with confirmation)",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runRolesReset,
}

func runRolesReset(cmd *cobra.Command, args []string) error {
	wfPath := rolesGenerateWorkflow
	if wfPath == "" {
		cfgPath := resolveConfigPath()
		cfg, err := workflow.ParseFarmConfig(cfgPath)
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}
		if len(cfg.Repos) == 0 {
			return fmt.Errorf("no repos configured")
		}
		wfPath = cfg.Repos[0].WorkflowPath
		if !filepath.IsAbs(wfPath) {
			wfPath = filepath.Join(filepath.Dir(cfgPath), wfPath)
		}
	}

	// Read the raw YAML to preserve structure.
	data, err := os.ReadFile(wfPath)
	if err != nil {
		return fmt.Errorf("read workflow: %w", err)
	}

	w, err := workflow.ParseWorkflow(wfPath)
	if err != nil {
		return fmt.Errorf("parse workflow: %w", err)
	}

	rolesDir := citadelRolesDir()

	if len(args) == 1 {
		// Reset a single role.
		roleName := args[0]
		builtin, ok := workflow.BuiltinRoles[roleName]
		if !ok {
			return fmt.Errorf("no built-in default for role %q", roleName)
		}

		fmt.Printf("Reset %s to built-in default? [y/N] ", roleName)
		reader := bufio.NewReader(os.Stdin)
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(strings.ToLower(input))
		if input != "y" && input != "yes" {
			fmt.Println("aborted")
			return nil
		}

		// Update in workflow.
		role := w.Roles[roleName]
		role.Name = builtin.Name
		role.Description = builtin.Description
		role.Instructions = builtin.Instructions
		w.Roles[roleName] = role

		if err := writeWorkflowRoles(wfPath, data, w); err != nil {
			return err
		}

		written, err := workflow.GenerateRoleFiles(w, rolesDir)
		if err != nil {
			return err
		}
		for _, path := range written {
			if strings.Contains(path, roleName) {
				fmt.Printf("Reset %s -> %s regenerated\n", roleName, path)
			}
		}
		return nil
	}

	// No arg — list all resettable roles and prompt for all.
	resettable := make([]string, 0)
	for k := range workflow.BuiltinRoles {
		resettable = append(resettable, k)
	}
	sort.Strings(resettable)

	if len(resettable) == 0 {
		fmt.Println("no built-in defaults available")
		return nil
	}

	fmt.Println("Resettable roles:")
	for _, k := range resettable {
		b := workflow.BuiltinRoles[k]
		fmt.Printf("  %-20s %s\n", k, b.Description)
	}
	fmt.Print("\nReset all to defaults? [y/N] ")
	reader := bufio.NewReader(os.Stdin)
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(strings.ToLower(input))
	if input != "y" && input != "yes" {
		fmt.Println("aborted")
		return nil
	}

	if w.Roles == nil {
		w.Roles = make(map[string]workflow.RoleDefinition)
	}
	for _, k := range resettable {
		b := workflow.BuiltinRoles[k]
		w.Roles[k] = workflow.RoleDefinition{
			Name:         b.Name,
			Description:  b.Description,
			Instructions: b.Instructions,
		}
	}

	if err := writeWorkflowRoles(wfPath, data, w); err != nil {
		return err
	}

	written, err := workflow.GenerateRoleFiles(w, rolesDir)
	if err != nil {
		return err
	}
	for _, path := range written {
		fmt.Printf("Reset -> %s regenerated\n", path)
	}
	return nil
}

// writeWorkflowRoles updates the roles section of a workflow YAML file.
func writeWorkflowRoles(wfPath string, originalData []byte, w *workflow.Workflow) error {
	var raw map[string]interface{}
	if err := yaml.Unmarshal(originalData, &raw); err != nil {
		return fmt.Errorf("parse raw yaml: %w", err)
	}
	raw["roles"] = w.Roles

	out, err := yaml.Marshal(raw)
	if err != nil {
		return fmt.Errorf("marshal yaml: %w", err)
	}
	if err := os.WriteFile(wfPath, out, 0o644); err != nil {
		return fmt.Errorf("write workflow: %w", err)
	}
	return nil
}

// citadelRolesDir returns ~/.citadel/roles.
func citadelRolesDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", "roles")
	}
	return filepath.Join(home, ".citadel", "roles")
}

func init() {
	rolesGenerateCmd.Flags().StringVar(&rolesGenerateWorkflow, "workflow", "", "path to workflow YAML file")
	rolesListCmd.Flags().StringVar(&rolesGenerateWorkflow, "workflow", "", "path to workflow YAML file")
	rolesEditCmd.Flags().StringVar(&rolesGenerateWorkflow, "workflow", "", "path to workflow YAML file")

	rolesResetCmd.Flags().StringVar(&rolesGenerateWorkflow, "workflow", "", "path to workflow YAML file")

	rolesCmd.AddCommand(rolesGenerateCmd, rolesListCmd, rolesEditCmd, rolesResetCmd)
	rootCmd.AddCommand(rolesCmd)
}
