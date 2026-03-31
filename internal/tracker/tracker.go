// Package tracker defines the TrackerProvider interface and ExternalIssue struct
// for modular issue tracker integrations in Cistern.
package tracker

// ExternalIssue represents a work item fetched from an external issue tracker.
// Fields map to Cistern droplet fields when importing an issue.
type ExternalIssue struct {
	// Key is the unique identifier in the source tracker (e.g. "PROJ-123").
	Key string
	// Title maps to the droplet title.
	Title string
	// Description maps to the droplet description.
	Description string
	// Priority is normalized 1–4 across all tracker providers.
	Priority int
	// Labels holds the tracker's tags or labels for the issue.
	Labels []string
	// SourceURL is the canonical link back to the issue in the source tracker.
	SourceURL string
}

// TrackerProvider fetches issues from an external issue tracker.
// All tracker integrations must implement this interface.
type TrackerProvider interface {
	// Name returns the canonical identifier for this provider (e.g. "jira", "linear").
	Name() string
	// FetchIssue retrieves a single issue by its key (e.g. "PROJ-123").
	// Returns a non-nil error if the key cannot be resolved or the request fails.
	FetchIssue(key string) (*ExternalIssue, error)
}
