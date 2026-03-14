package main

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/MichielDean/bullet-farm/internal/workflow"
	"github.com/spf13/cobra"
)

var configPath string

var farmCmd = &cobra.Command{
	Use:   "farm",
	Short: "Farm management commands",
}

// --- farm start ---

var farmStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Load config, validate workflows, and start the scheduler loop",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfgPath := resolveConfigPath()
		cfg, err := workflow.ParseFarmConfig(cfgPath)
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}

		cfgDir := filepath.Dir(cfgPath)
		workflows := make(map[string]*workflow.Workflow, len(cfg.Repos))
		for _, repo := range cfg.Repos {
			if repo.WorkflowPath == "" {
				return fmt.Errorf("repo %q: workflow_path is required", repo.Name)
			}
			wfPath := repo.WorkflowPath
			if !filepath.IsAbs(wfPath) {
				wfPath = filepath.Join(cfgDir, wfPath)
			}
			w, err := workflow.ParseWorkflow(wfPath)
			if err != nil {
				return fmt.Errorf("repo %q workflow %q: %w", repo.Name, repo.WorkflowPath, err)
			}
			workflows[repo.Name] = w
		}

		fmt.Printf("farm: loaded %d repo(s), max_total_workers=%d\n", len(cfg.Repos), cfg.MaxTotalWorkers)
		for _, repo := range cfg.Repos {
			w := workflows[repo.Name]
			fmt.Printf("  %s: workflow=%q (%d steps), workers=%d\n",
				repo.Name, w.Name, len(w.Steps), repo.Workers)
		}

		fmt.Println("farm: scheduler running (ctrl-c to stop)")
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)

		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-sig:
				fmt.Println("\nfarm: shutting down")
				return nil
			case <-ticker.C:
				// Scheduler tick placeholder.
			}
		}
	},
}

// --- farm status ---

var farmStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show repos, workers, and global worker count",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfgPath := resolveConfigPath()
		cfg, err := workflow.ParseFarmConfig(cfgPath)
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}

		cfgDir := filepath.Dir(cfgPath)
		tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintln(tw, "REPO\tWORKFLOW\tWORKERS\tSTATUS")

		totalWorkers := 0
		for _, repo := range cfg.Repos {
			wfName := "-"
			status := "ok"

			wfPath := repo.WorkflowPath
			if wfPath != "" {
				if !filepath.IsAbs(wfPath) {
					wfPath = filepath.Join(cfgDir, wfPath)
				}
				w, err := workflow.ParseWorkflow(wfPath)
				if err != nil {
					wfName = repo.WorkflowPath
					status = "error: " + err.Error()
				} else {
					wfName = w.Name
				}
			}

			totalWorkers += repo.Workers
			fmt.Fprintf(tw, "%s\t%s\t%d\t%s\n", repo.Name, wfName, repo.Workers, status)
		}
		tw.Flush()

		fmt.Printf("\ntotal workers: %d / %d (max)\n", totalWorkers, cfg.MaxTotalWorkers)
		return nil
	},
}

// --- farm config validate ---

var farmConfigCmd = &cobra.Command{
	Use:   "config",
	Short: "Config management",
}

var farmConfigValidateCmd = &cobra.Command{
	Use:   "validate [path]",
	Short: "Validate a farm config and all referenced workflow files",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		path := resolveConfigPath()
		if len(args) > 0 {
			path = args[0]
		}

		cfg, err := workflow.ParseFarmConfig(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "config error: %v\n", err)
			return err
		}

		cfgDir := filepath.Dir(path)
		var errs []error
		for _, repo := range cfg.Repos {
			if repo.Name == "" {
				e := fmt.Errorf("repo entry missing name")
				fmt.Fprintf(os.Stderr, "  error: %v\n", e)
				errs = append(errs, e)
				continue
			}
			if repo.WorkflowPath == "" {
				e := fmt.Errorf("repo %q: workflow_path is required", repo.Name)
				fmt.Fprintf(os.Stderr, "  error: %v\n", e)
				errs = append(errs, e)
				continue
			}

			wfPath := repo.WorkflowPath
			if !filepath.IsAbs(wfPath) {
				wfPath = filepath.Join(cfgDir, wfPath)
			}

			if _, err := workflow.ParseWorkflow(wfPath); err != nil {
				e := fmt.Errorf("repo %q workflow %q: %w", repo.Name, repo.WorkflowPath, err)
				fmt.Fprintf(os.Stderr, "  error: %v\n", e)
				errs = append(errs, e)
			}
		}

		if len(errs) > 0 {
			return fmt.Errorf("validation found %d error(s)", len(errs))
		}

		fmt.Println("config valid:", path)
		return nil
	},
}

func init() {
	farmStartCmd.Flags().StringVar(&configPath, "config", "", "path to farm config (default: ./config.yaml)")

	farmConfigCmd.AddCommand(farmConfigValidateCmd)
	farmCmd.AddCommand(farmStartCmd, farmStatusCmd, farmConfigCmd)
	rootCmd.AddCommand(farmCmd)
}

func resolveConfigPath() string {
	if configPath != "" {
		return configPath
	}
	if env := os.Getenv("BT_CONFIG"); env != "" {
		return env
	}
	return "config.yaml"
}
