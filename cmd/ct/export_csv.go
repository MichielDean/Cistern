package main

import (
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/MichielDean/cistern/internal/cistern"
)

// writeDropletsCSV writes droplets as CSV to w, with a header row.
// Shared by `ct droplet export --format csv` and the dashboard export endpoint.
func writeDropletsCSV(w io.Writer, items []*cistern.Droplet) error {
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"id", "repo", "title", "description", "priority", "status", "assignee", "current_cataractae", "outcome", "assigned_aqueduct", "last_reviewed_commit", "created_at", "updated_at"})
	for _, item := range items {
		_ = cw.Write([]string{
			csvEscapeCell(item.ID),
			csvEscapeCell(item.Repo),
			csvEscapeCell(item.Title),
			csvEscapeCell(item.Description),
			strconv.Itoa(item.Priority),
			csvEscapeCell(item.Status),
			csvEscapeCell(item.Assignee),
			csvEscapeCell(item.CurrentCataractae),
			csvEscapeCell(item.Outcome),
			csvEscapeCell(item.AssignedAqueduct),
			csvEscapeCell(item.LastReviewedCommit),
			item.CreatedAt.Format(time.RFC3339),
			item.UpdatedAt.Format(time.RFC3339),
		})
	}
	cw.Flush()
	return cw.Error()
}

// csvEscapeCell guards against CSV formula injection: spreadsheet apps treat a
// leading =, +, - or @ as a formula, so such cells are prefixed with a single
// quote to force text interpretation. Leading tab and carriage-return characters
// get the same treatment so injection-via-whitespace is also covered.
func csvEscapeCell(cell string) string {
	if cell == "" {
		return ""
	}
	switch cell[0] {
	case '=', '+', '-', '@', '\t', '\r':
		return "'" + cell
	}
	return cell
}

// dropletsExportFormat validates a requested export format ("json" or "csv").
func dropletsExportFormat(format string) (string, error) {
	if format == "" {
		return "csv", nil // csv is the default encoding for the export endpoint
	}
	if format != "json" && format != "csv" {
		return "", fmt.Errorf("format must be json or csv")
	}
	return format, nil
}
