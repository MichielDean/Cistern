package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"text/tabwriter"
	"time"

	"github.com/MichielDean/cistern/internal/cistern"
	"github.com/spf13/cobra"
)

var logFmt string

type logEntry struct {
	sortTime   time.Time `json:"-"`
	Time       string    `json:"time"`
	Cataractae string    `json:"cataractae"`
	Event      string    `json:"event"`
	Detail     string    `json:"detail"`
}

var dropletLogCmd = &cobra.Command{
	Use:   "log <id>",
	Short: "Show chronological activity log for a droplet",
	Long: `Display a timeline of events for a droplet, including stage
transitions, outcome signals, scheduler events, and notes.

Output modes:
  --format text   Tab-aligned table with timestamps (default)
  --format json   One JSON object per line (NDJSON)`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runLog(os.Stdout, args[0])
	},
}

func runLog(out io.Writer, id string) error {
	if logFmt != "text" && logFmt != "json" {
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

	timeline, err := c.GetDropletTimeline(id, 0)
	if err != nil {
		return err
	}

	notes, err := c.GetNotes(id)
	if err != nil {
		return err
	}

	entries := buildLogEntries(timeline, notes)

	if logFmt == "json" {
		return printLogJSON(out, entries)
	}
	return printLogText(out, item, entries)
}

func buildLogEntries(timeline []cistern.TimelineEntry, notes []cistern.CataractaeNote) []logEntry {
	var entries []logEntry

	for _, te := range timeline {
		eventLabel, detail := cistern.DisplayInfo(te.EventType, te.Payload)
		entries = append(entries, logEntry{
			sortTime: te.Time,
			Time:     te.Time.Format("2006-01-02 15:04:05"),
			Event:    eventLabel,
			Detail:   detail,
		})
	}

	for _, n := range notes {
		entries = append(entries, logEntry{
			sortTime:   n.CreatedAt,
			Time:       n.CreatedAt.Format("2006-01-02 15:04:05"),
			Cataractae: n.CataractaeName,
			Event:      "note",
			Detail:     n.Content,
		})
	}

	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].sortTime.Before(entries[j].sortTime)
	})

	return entries
}

func printLogText(out io.Writer, item *cistern.Droplet, entries []logEntry) error {
	fmt.Fprintf(out, "Droplet: %s  Title: %s  Status: %s\n\n", item.ID, item.Title, displayStatusForDroplet(item))

	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "TIME\tCATARACTAE\tEVENT\tDETAIL")
	for _, e := range entries {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", e.Time, e.Cataractae, e.Event, e.Detail)
	}
	return tw.Flush()
}

func printLogJSON(out io.Writer, entries []logEntry) error {
	for _, e := range entries {
		line, err := json.Marshal(e)
		if err != nil {
			return fmt.Errorf("json marshal error: %w", err)
		}
		fmt.Fprintln(out, string(line))
	}
	return nil
}
