package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
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

	changes, err := c.GetDropletChanges(id, 10000)
	if err != nil {
		return err
	}

	entries := buildLogEntries(item, changes)

	if logFmt == "json" {
		return printLogJSON(out, entries)
	}
	return printLogText(out, item, entries)
}

func buildLogEntries(item *cistern.Droplet, changes []cistern.DropletChange) []logEntry {
	var entries []logEntry

	for _, ch := range changes {
		var evt, detail, cataractae string

		prefix, suffix, found := strings.Cut(ch.Value, ": ")
		if ch.Kind == "note" {
			evt = "note"
			if found {
				cataractae = prefix
				detail = suffix
			} else {
				detail = ch.Value
			}
		} else {
			if found {
				evt = prefix
				detail = suffix
			} else {
				evt = ch.Value
			}
			evt, detail = remapEvent(evt, detail)
		}

		entries = append(entries, logEntry{
			sortTime:   ch.Time,
			Time:       ch.Time.Format("2006-01-02 15:04:05"),
			Cataractae: cataractae,
			Event:      evt,
			Detail:     detail,
		})
	}

	hasCreate := false
	for _, e := range entries {
		if e.Event == "created" {
			hasCreate = true
			break
		}
	}

	if !hasCreate && !item.CreatedAt.IsZero() {
		createPayload, _ := json.Marshal(map[string]any{
			"repo":       item.Repo,
			"title":      item.Title,
			"priority":   item.Priority,
			"complexity": item.Complexity,
		})
		detail := remapPayloadCreate(string(createPayload))
		entries = append(entries, logEntry{
			sortTime: item.CreatedAt,
			Time:     item.CreatedAt.Format("2006-01-02 15:04:05"),
			Event:    "created",
			Detail:   detail,
		})
	}

	if !item.LastHeartbeatAt.IsZero() {
		entries = append(entries, logEntry{
			sortTime:   item.LastHeartbeatAt,
			Time:       item.LastHeartbeatAt.Format("2006-01-02 15:04:05"),
			Cataractae: item.CurrentCataractae,
			Event:      "heartbeat",
			Detail:     "last heartbeat recorded",
		})
	}

	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].sortTime.Before(entries[j].sortTime)
	})

	return entries
}

func remapEvent(evt, detail string) (string, string) {
	switch evt {
	case "create":
		return "created", remapPayloadCreate(detail)
	case "pool":
		return "pooled", remapPayloadReason(detail)
	case "cancel":
		return "cancelled", remapPayloadReason(detail)
	case "dispatch":
		return "dispatched", remapPayloadDispatch(detail)
	case "pass":
		return "pass", remapPayloadCataractaeNotes(detail)
	case "recirculate":
		return "recirculate", remapPayloadRecirculate(detail)
	case "delivered":
		return "delivered", ""
	case "restart":
		return "restart", remapPayloadCataractae(detail)
	case "approve":
		return "approved", remapPayloadCataractae(detail)
	case "edit":
		return "edit", remapPayloadEdit(detail)
	default:
		return evt, detail
	}
}

func remapPayloadReason(detail string) string {
	if detail == "" || detail == "{}" {
		return ""
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(detail), &payload); err == nil {
		if reason, ok := payload["reason"]; ok && reason != "" {
			return "reason: " + fmt.Sprintf("%v", reason)
		}
	}
	return detail
}

func remapPayloadCreate(detail string) string {
	if detail == "" || detail == "{}" {
		return ""
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(detail), &payload); err != nil {
		return detail
	}
	var parts []string
	if repo, ok := payload["repo"]; ok && repo != "" {
		parts = append(parts, fmt.Sprintf("repo: %v", repo))
	}
	if title, ok := payload["title"]; ok && title != "" {
		parts = append(parts, fmt.Sprintf("title: %v", title))
	}
	if priority, ok := payload["priority"]; ok {
		parts = append(parts, fmt.Sprintf("priority: %v", priority))
	}
	if complexity, ok := payload["complexity"]; ok {
		parts = append(parts, fmt.Sprintf("complexity: %v", complexity))
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, ", ")
}

func remapPayloadDispatch(detail string) string {
	if detail == "" || detail == "{}" {
		return ""
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(detail), &payload); err != nil {
		return detail
	}
	var parts []string
	if aqueduct, ok := payload["aqueduct"]; ok && aqueduct != "" {
		parts = append(parts, fmt.Sprintf("aqueduct: %v", aqueduct))
	}
	if cat, ok := payload["cataractae"]; ok && cat != "" {
		parts = append(parts, fmt.Sprintf("step: %v", cat))
	}
	if assignee, ok := payload["assignee"]; ok && assignee != "" {
		parts = append(parts, fmt.Sprintf("assignee: %v", assignee))
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, ", ")
}

func remapPayloadCataractaeNotes(detail string) string {
	if detail == "" || detail == "{}" {
		return ""
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(detail), &payload); err != nil {
		return detail
	}
	var parts []string
	if cat, ok := payload["cataractae"]; ok && cat != "" {
		parts = append(parts, fmt.Sprintf("by: %v", cat))
	}
	if notes, ok := payload["notes"]; ok && notes != "" {
		parts = append(parts, fmt.Sprintf("notes: %v", notes))
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, ", ")
}

func remapPayloadRecirculate(detail string) string {
	if detail == "" || detail == "{}" {
		return ""
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(detail), &payload); err != nil {
		return detail
	}
	var parts []string
	if cat, ok := payload["cataractae"]; ok && cat != "" {
		parts = append(parts, fmt.Sprintf("by: %v", cat))
	}
	if target, ok := payload["target"]; ok && target != "" {
		parts = append(parts, fmt.Sprintf("to: %v", target))
	}
	if notes, ok := payload["notes"]; ok && notes != "" {
		parts = append(parts, fmt.Sprintf("notes: %v", notes))
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, ", ")
}

func remapPayloadCataractae(detail string) string {
	if detail == "" || detail == "{}" {
		return ""
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(detail), &payload); err != nil {
		return detail
	}
	if cat, ok := payload["cataractae"]; ok && cat != "" {
		return fmt.Sprintf("by: %v", cat)
	}
	return ""
}

func remapPayloadEdit(detail string) string {
	if detail == "" || detail == "{}" {
		return ""
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(detail), &payload); err != nil {
		return detail
	}
	if fields, ok := payload["fields"]; ok {
		return fmt.Sprintf("fields: %v", fields)
	}
	return ""
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
