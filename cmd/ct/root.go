package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

var dbPath string

var rootCmd = &cobra.Command{
	Use:   "ct",
	Short: "Cistern CLI — where droplets flow and code runs clean",
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		displayASCIILogo()
	},
}

func init() {
	rootCmd.PersistentFlags().StringVar(&dbPath, "db", "", "path to queue database (default: ~/.cistern/cistern.db)")
}

func resolveDBPath() string {
	if dbPath != "" {
		return dbPath
	}
	if env := os.Getenv("CT_DB"); env != "" {
		return env
	}
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: cannot determine home directory: %v\n", err)
		os.Exit(1)
	}
	dir := filepath.Join(home, ".cistern")
	os.MkdirAll(dir, 0o755)
	return filepath.Join(dir, "cistern.db")
}

// displayASCIILogo tries to locate and print a cistern ASCII logo file.
// Search order:
// 1. $CT_ASCII_LOGO
// 2. ~/.cistern/cistern_logo_ascii.txt
// 3. ./cistern_logo_ascii.txt (cwd)
func displayASCIILogo() {
	// allow opt-out
	if os.Getenv("CT_NO_ASCII_LOGO") != "" {
		return
	}

	candidates := []string{}
	if env := os.Getenv("CT_ASCII_LOGO"); env != "" {
		candidates = append(candidates, env)
	}
	if home, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates, filepath.Join(home, ".cistern", "cistern_logo_ascii.txt"))
	}
	candidates = append(candidates, "cistern_logo_ascii.txt")

	for _, p := range candidates {
		if p == "" {
			continue
		}
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		fmt.Print(string(data))
		// print a separating newline if not present
		if len(data) == 0 || data[len(data)-1] != '\n' {
			fmt.Println()
		}
		return
	}
}
