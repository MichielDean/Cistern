package main

import (
	"fmt"
	"io"
	"os"

	"github.com/MichielDean/cistern/internal/cistern"
	"github.com/spf13/cobra"
)

var historyFmt string

var dropletHistoryCmd = &cobra.Command{
	Use:   "history <id>",
	Short: "Show event timeline for a droplet (alias for 'ct droplet log')",
	Long: `Display a chronological timeline of events for a droplet,
including stage transitions, outcome signals, scheduler events, and notes.

This is an alias for 'ct droplet log', providing a more intuitive name
for operators who want a quick human-readable event history.

Output modes:
  --format text   Tab-aligned table with timestamps (default)
  --format json   One JSON object per line (NDJSON)`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runHistory(os.Stdout, args[0])
	},
}

func runHistory(out io.Writer, id string) error {
	if historyFmt != "text" && historyFmt != "json" {
		return fmt.Errorf("--format must be text or json")
	}

	c, err := cistern.New(resolveDBPath(), "")
	if err != nil {
		return err
	}
	defer c.Close()

	item, err := c.Get(id)
	if err != nil {
		return err
	}

	changes, err := c.GetDropletChanges(id, 10000)
	if err != nil {
		return err
	}

	entries := buildLogEntries(item, changes)

	if historyFmt == "json" {
		return printLogJSON(out, entries)
	}
	return printLogText(out, item, entries)
}
