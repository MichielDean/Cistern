package scheduler

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/MichielDean/citadel/internal/workflow"

	_ "github.com/mattn/go-sqlite3"
)

// RunIdleHooks executes all configured idle hooks sequentially.
// Errors are logged but do not crash the scheduler.
func RunIdleHooks(hooks []workflow.IdleHook, cfg *workflow.FarmConfig, dbPath string, sandboxRoot string, logger *slog.Logger) {
	for _, hook := range hooks {
		logger.Info("idle hook starting", "hook", hook.Name, "action", hook.Action)
		var err error
		switch hook.Action {
		case "roles_generate":
			err = hookRolesGenerate(cfg, logger)
		case "worktree_prune":
			err = hookWorktreePrune(cfg, sandboxRoot, logger)
		case "db_vacuum":
			err = hookDBVacuum(dbPath, logger)
		case "shell":
			err = hookShell(hook, logger)
		default:
			logger.Warn("idle hook: unknown action", "hook", hook.Name, "action", hook.Action)
			continue
		}
		if err != nil {
			logger.Error("idle hook failed", "hook", hook.Name, "error", err)
		} else {
			logger.Info("idle hook completed", "hook", hook.Name)
		}
	}
}

// hookRolesGenerate checks if any workflow YAML mtime is newer than the oldest
// role file in ~/.citadel/roles/ and regenerates if needed.
func hookRolesGenerate(cfg *workflow.FarmConfig, logger *slog.Logger) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("home dir: %w", err)
	}
	rolesDir := filepath.Join(home, ".citadel", "roles")

	// Find the oldest role file mtime.
	oldestRole := time.Now()
	hasRoles := false
	entries, _ := os.ReadDir(rolesDir)
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		claudePath := filepath.Join(rolesDir, e.Name(), "CLAUDE.md")
		info, err := os.Stat(claudePath)
		if err != nil {
			continue
		}
		hasRoles = true
		if info.ModTime().Before(oldestRole) {
			oldestRole = info.ModTime()
		}
	}

	regenerated := false
	for _, repo := range cfg.Repos {
		if repo.WorkflowPath == "" {
			continue
		}
		wfPath := repo.WorkflowPath
		info, err := os.Stat(wfPath)
		if err != nil {
			logger.Warn("roles_generate: cannot stat workflow", "path", wfPath, "error", err)
			continue
		}

		if !hasRoles || info.ModTime().After(oldestRole) {
			w, err := workflow.ParseWorkflow(wfPath)
			if err != nil {
				logger.Warn("roles_generate: parse workflow failed", "path", wfPath, "error", err)
				continue
			}
			written, err := workflow.GenerateRoleFiles(w, rolesDir)
			if err != nil {
				return fmt.Errorf("generate role files: %w", err)
			}
			for _, path := range written {
				logger.Info("roles_generate: regenerated", "path", path)
			}
			regenerated = true
		}
	}

	if !regenerated {
		logger.Info("roles_generate: roles up to date")
	}
	return nil
}

// hookWorktreePrune runs `git worktree prune` for each repo's sandbox directory.
func hookWorktreePrune(cfg *workflow.FarmConfig, sandboxRoot string, logger *slog.Logger) error {
	for _, repo := range cfg.Repos {
		dir := filepath.Join(sandboxRoot, repo.Name)
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			continue
		}

		cmd := exec.Command("git", "worktree", "prune")
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			logger.Warn("worktree_prune: error", "repo", repo.Name, "error", err, "output", string(out))
			continue
		}
		logger.Info("worktree_prune: pruned", "repo", repo.Name, "output", string(out))
	}
	return nil
}

// hookDBVacuum runs VACUUM on the SQLite queue database.
func hookDBVacuum(dbPath string, logger *slog.Logger) error {
	if dbPath == "" {
		return fmt.Errorf("db_vacuum: no database path configured")
	}

	beforeInfo, _ := os.Stat(dbPath)
	var beforeSize int64
	if beforeInfo != nil {
		beforeSize = beforeInfo.Size()
	}

	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return fmt.Errorf("db_vacuum: open: %w", err)
	}
	defer db.Close()

	if _, err := db.Exec("VACUUM"); err != nil {
		return fmt.Errorf("db_vacuum: %w", err)
	}

	afterInfo, _ := os.Stat(dbPath)
	var afterSize int64
	if afterInfo != nil {
		afterSize = afterInfo.Size()
	}

	logger.Info("db_vacuum: completed",
		"before_bytes", beforeSize,
		"after_bytes", afterSize,
		"freed_bytes", beforeSize-afterSize,
	)
	return nil
}

// hookShell runs a shell command with a timeout.
func hookShell(hook workflow.IdleHook, logger *slog.Logger) error {
	if hook.Command == "" {
		return fmt.Errorf("shell hook %q: command is empty", hook.Name)
	}

	timeout := time.Duration(hook.Timeout) * time.Second
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", hook.Command)
	out, err := cmd.CombinedOutput()
	if len(out) > 0 {
		logger.Info("shell hook output", "hook", hook.Name, "output", string(out))
	}
	if err != nil {
		return fmt.Errorf("shell hook %q: %w", hook.Name, err)
	}
	return nil
}
