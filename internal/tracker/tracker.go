// Package tracker defines the TrackerProvider interface and ExternalIssue struct
// for modular issue tracker integrations in Cistern.
package tracker

import "time"

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

// TrackerConfig holds connection parameters for one external tracker
// as declared in the cistern.yaml trackers section.
type TrackerConfig struct {
	// Name is the provider type (e.g. "jira"). Must match a registered
	// constructor.
	Name string `yaml:"name"`
	// BaseURL is the root URL of the tracker instance
	// (e.g. "https://company.atlassian.net").
	BaseURL string `yaml:"base_url"`
	// TokenEnv is the environment variable that holds the API token.
	TokenEnv string `yaml:"token_env"`
	// UserEnv is the environment variable that holds the username.
	// Required for providers that use Basic Auth (e.g. Jira Cloud).
	UserEnv string `yaml:"user_env,omitempty"`
	// PriorityMap maps tracker-native priority names (e.g. "High") to
	// Cistern priorities (1=highest, 2=normal, 3=low). When absent the
	// provider's built-in defaults are used.
	PriorityMap map[string]int `yaml:"priority_map,omitempty"`
	// HTTPTimeout overrides the default HTTP client timeout for this provider.
	// Zero means use the provider's built-in default (typically 30s).
	HTTPTimeout time.Duration `yaml:"http_timeout,omitempty"`
}

// Constructor is a factory function that builds a TrackerProvider from a
// TrackerConfig. Implementations register themselves via Register.
type Constructor func(cfg TrackerConfig) (TrackerProvider, error)

var registry = map[string]Constructor{}

// Register adds a Constructor to the global registry under name.
// Providers call this from their init() function.
func Register(name string, fn Constructor) {
	registry[name] = fn
}

// Resolve returns the Constructor for the given provider name and whether it
// was found. The lookup is case-sensitive.
func Resolve(name string) (Constructor, bool) {
	fn, ok := registry[name]
	return fn, ok
}
