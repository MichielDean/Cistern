package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/MichielDean/cistern/internal/cistern"
	"github.com/MichielDean/cistern/internal/cataracta"
	"github.com/MichielDean/cistern/internal/castellarius"
	"github.com/MichielDean/cistern/internal/aqueduct"
	"github.com/spf13/cobra"
)

var configPath string

// ─────────────────────────────────────────────────────────────────────────────
// ct castellarius — manage the Castellarius (the aqueduct overseer)
// ─────────────────────────────────────────────────────────────────────────────

var castellariusCmd = &cobra.Command{
	Use:   "castellarius",
	Short: "Manage the Castellarius — the overseer that watches the cistern and routes droplets",
}

// ct castellarius start

var castellariusStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Wake the Castellarius — validate config, then watch the cistern and route droplets automatically",
	Long: `Wake the Castellarius.

The Castellarius is a pure state machine — no AI. It watches the cistern for
droplets, assigns them to named operators, routes them through the configured
aqueduct (implement → review → qa → merge), and resolves outcomes.

The Castellarius runs until Ctrl-C. As long as work is in the cistern it will
keep dispatching droplets into aqueducts automatically.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfgPath := resolveConfigPath()
		cfg, err := aqueduct.ParseAqueductConfig(cfgPath)
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}

		// Resolve relative workflow paths against the config file's directory so
		// that both the adapter and the scheduler see consistent absolute paths.
		cfgDir := filepath.Dir(cfgPath)
		for i := range cfg.Repos {
			if !filepath.IsAbs(cfg.Repos[i].WorkflowPath) {
				cfg.Repos[i].WorkflowPath = filepath.Join(cfgDir, cfg.Repos[i].WorkflowPath)
			}
		}

		workflows := make(map[string]*aqueduct.Workflow, len(cfg.Repos))
		for _, repo := range cfg.Repos {
			if repo.WorkflowPath == "" {
				return fmt.Errorf("repo %q: workflow_path is required", repo.Name)
			}
			w, err := aqueduct.ParseWorkflow(repo.WorkflowPath)
			if err != nil {
				return fmt.Errorf("repo %q workflow %q: %w", repo.Name, repo.WorkflowPath, err)
			}
			workflows[repo.Name] = w
		}

		dbPath := resolveDBPath()
		queueClients := make(map[string]*cistern.Client, len(cfg.Repos))
		for _, repo := range cfg.Repos {
			c, err := cistern.New(dbPath, repo.Prefix)
			if err != nil {
				return fmt.Errorf("queue for %q: %w", repo.Name, err)
			}
			queueClients[repo.Name] = c
		}

		adapter, err := cataracta.NewAdapter(cfg.Repos, workflows, queueClients)
		if err != nil {
			return fmt.Errorf("runner adapter: %w", err)
		}

		sched, err := castellarius.New(*cfg, dbPath, adapter)
		if err != nil {
			return fmt.Errorf("castellarius: %w", err)
		}

		fmt.Println("Castellarius awake. Watching the cistern.")
		for _, repo := range cfg.Repos {
			w := workflows[repo.Name]
			names := repoWorkerNames(repo)
			fmt.Printf("  %s: aqueduct=%q (%d cataractae), operators=%d (%s)\n",
				repo.Name, w.Name, len(w.Cataractae), repo.Cataractae, strings.Join(names, ", "))
		}
		fmt.Println("Ctrl-C to dismiss the Castellarius.")

		ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
		defer cancel()
		if err := sched.Run(ctx); errors.Is(err, context.Canceled) {
			return nil
		}
		return err
	},
}

// ct castellarius status — operator/worker-centric view

var castellariusStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show operator assignments — who is working on what, and idle capacity",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfgPath := resolveConfigPath()
		cfg, err := aqueduct.ParseAqueductConfig(cfgPath)
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}

		dbPath := resolveDBPath()
		c, err := cistern.New(dbPath, "")
		if err != nil {
			return fmt.Errorf("cistern: %w", err)
		}
		defer c.Close()

		allItems, err := c.List("", "")
		if err != nil {
			return fmt.Errorf("list droplets: %w", err)
		}

		assignee := map[string]*cistern.Droplet{}
		for _, item := range allItems {
			if item.Status == "in_progress" && item.Assignee != "" {
				assignee[item.Assignee] = item
			}
		}

		tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintln(tw, "OPERATOR\tREPO\tDROPLET\tCATARACTA\tELAPSED")
		fmt.Fprintln(tw, "────────\t────\t───────\t─────────\t───────")

		totalBusy := 0
		for _, repo := range cfg.Repos {
			for _, name := range repoWorkerNames(repo) {
				if item, ok := assignee[name]; ok {
					elapsed := int(time.Since(item.UpdatedAt).Minutes())
					fmt.Fprintf(tw, "%s\t%s\t%s\t[%s]\t%dm\n",
						name, repo.Name, item.ID, item.CurrentCataracta, elapsed)
					totalBusy++
				} else {
					fmt.Fprintf(tw, "%s\t%s\t—\t—\t—\n", name, repo.Name)
				}
			}
		}
		tw.Flush()

		totalWorkers := 0
		for _, repo := range cfg.Repos {
			totalWorkers += len(repoWorkerNames(repo))
		}
		fmt.Printf("\n%d of %d operators busy\n", totalBusy, totalWorkers)
		return nil
	},
}

// ─────────────────────────────────────────────────────────────────────────────
// ct aqueduct — inspect and validate aqueduct definitions (workflows)
// ─────────────────────────────────────────────────────────────────────────────

var aqueductCmd = &cobra.Command{
	Use:   "aqueduct",
	Short: "Inspect and validate aqueducts — the full pipeline from intake to delivery",
}

// ct aqueduct status — workflow definition view

var aqueductStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show configured aqueducts — repos, workflows, and their cataracta step chains",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfgPath := resolveConfigPath()
		cfg, err := aqueduct.ParseAqueductConfig(cfgPath)
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}

		cfgDir := filepath.Dir(cfgPath)
		fmt.Printf("Aqueducts (%d configured)\n\n", len(cfg.Repos))

		for _, repo := range cfg.Repos {
			fmt.Printf("  %s\n", repo.Name)
			fmt.Printf("    URL         : %s\n", repo.URL)
			fmt.Printf("    Workflow    : %s\n", repo.WorkflowPath)

			wfPath := repo.WorkflowPath
			if !filepath.IsAbs(wfPath) {
				wfPath = filepath.Join(cfgDir, wfPath)
			}
			if wf, err := aqueduct.ParseWorkflow(wfPath); err == nil {
				steps := make([]string, len(wf.Cataractae))
				for i, s := range wf.Cataractae {
					steps[i] = s.Name
				}
				fmt.Printf("    Steps       : %s\n", strings.Join(steps, " → "))
			} else {
				fmt.Printf("    Steps       : (could not load: %v)\n", err)
			}

			names := repoWorkerNames(repo)
			fmt.Printf("    Operators   : %d (%s)\n", len(names), strings.Join(names, ", "))
			fmt.Println()
		}
		return nil
	},
}

// ct aqueduct validate — config and workflow validation

var aqueductValidateCmd = &cobra.Command{
	Use:   "validate [path]",
	Short: "Validate cistern.yaml and all referenced workflow files",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		path := resolveConfigPath()
		if len(args) > 0 {
			path = args[0]
		}

		cfg, err := aqueduct.ParseAqueductConfig(path)
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

			if _, err := aqueduct.ParseWorkflow(wfPath); err != nil {
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

// ─────────────────────────────────────────────────────────────────────────────
// ct status — overall system status (combines all views)
// ─────────────────────────────────────────────────────────────────────────────

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Overall system status — cistern level, operator assignments, and aqueduct info",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfgPath := resolveConfigPath()
		cfg, err := aqueduct.ParseAqueductConfig(cfgPath)
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}

		dbPath := resolveDBPath()
		c, err := cistern.New(dbPath, "")
		if err != nil {
			return fmt.Errorf("cistern: %w", err)
		}
		defer c.Close()

		allItems, err := c.List("", "")
		if err != nil {
			return fmt.Errorf("list droplets: %w", err)
		}

		flowing, queued, done := 0, 0, 0
		assignee := map[string]*cistern.Droplet{}
		for _, item := range allItems {
			switch item.Status {
			case "in_progress":
				flowing++
				if item.Assignee != "" {
					assignee[item.Assignee] = item
				}
			case "open":
				queued++
			case "closed":
				done++
			}
		}

		// ── Cistern ──────────────────────────────────────────────────────────
		fmt.Printf("Cistern     %d flowing  %d queued  %d done\n\n", flowing, queued, done)

		// ── Castellarius / operators ──────────────────────────────────────────
		allWorkers := 0
		for _, repo := range cfg.Repos {
			allWorkers += len(repoWorkerNames(repo))
		}
		fmt.Printf("Castellarius  %d of %d operators busy\n", len(assignee), allWorkers)

		tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		for _, repo := range cfg.Repos {
			for _, name := range repoWorkerNames(repo) {
				if item, ok := assignee[name]; ok {
					elapsed := int(time.Since(item.UpdatedAt).Minutes())
					fmt.Fprintf(tw, "  %s\t%s\t[%s]\t%dm\n", name, item.ID, item.CurrentCataracta, elapsed)
				} else {
					fmt.Fprintf(tw, "  %s\t—\tdry\t\n", name)
				}
			}
		}
		tw.Flush()

		// ── Aqueducts ─────────────────────────────────────────────────────────
		fmt.Println()
		cfgDir := filepath.Dir(cfgPath)
		fmt.Printf("Aqueducts\n")
		for _, repo := range cfg.Repos {
			wfPath := repo.WorkflowPath
			if !filepath.IsAbs(wfPath) {
				wfPath = filepath.Join(cfgDir, wfPath)
			}
			stepCount := "?"
			if wf, err := aqueduct.ParseWorkflow(wfPath); err == nil {
				stepCount = fmt.Sprintf("%d", len(wf.Cataractae))
			}
			fmt.Printf("  %-20s  %s  (%s steps)\n", repo.Name, repo.WorkflowPath, stepCount)
		}

		return nil
	},
}

// ─────────────────────────────────────────────────────────────────────────────

func init() {
	castellariusStartCmd.Flags().StringVar(&configPath, "config", "", "path to cistern.yaml (default: ~/.cistern/cistern.yaml)")

	castellariusCmd.AddCommand(castellariusStartCmd, castellariusStatusCmd)
	aqueductCmd.AddCommand(aqueductStatusCmd, aqueductValidateCmd)

	rootCmd.AddCommand(castellariusCmd, aqueductCmd, statusCmd)
}

func resolveConfigPath() string {
	if configPath != "" {
		return configPath
	}
	if env := os.Getenv("CT_CONFIG"); env != "" {
		return env
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "cistern.yaml"
	}
	return filepath.Join(home, ".cistern", "cistern.yaml")
}

// repoWorkerNames returns the configured worker names for a repo,
// falling back to worker-0, worker-1, etc.
func repoWorkerNames(repo aqueduct.RepoConfig) []string {
	if len(repo.Names) > 0 {
		return repo.Names
	}
	names := make([]string, repo.Cataractae)
	for i := range names {
		names[i] = fmt.Sprintf("worker-%d", i)
	}
	return names
}
