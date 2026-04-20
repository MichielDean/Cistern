package tracker

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"
)

var validIssueKeyRe = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

func init() {
	Register("jira", newJiraProvider)
}

var defaultJiraPriorityMap = map[string]int{
	"Highest": 1,
	"High":    1,
	"Medium":  2,
	"Low":     3,
	"Lowest":  3,
}

type jiraProvider struct {
	cfg         TrackerConfig
	client      *http.Client
	httpTimeout time.Duration
	priorityMap map[string]int
}

func newJiraProvider(cfg TrackerConfig) (TrackerProvider, error) {
	prioMap := make(map[string]int, len(defaultJiraPriorityMap))
	for k, v := range defaultJiraPriorityMap {
		prioMap[k] = v
	}
	timeout := cfg.HTTPTimeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	return &jiraProvider{
		cfg:         cfg,
		httpTimeout: timeout,
		priorityMap: prioMap,
		client:      &http.Client{Timeout: timeout},
	}, nil
}

// Name returns the provider identifier.
func (p *jiraProvider) Name() string {
	return "jira"
}

// jiraIssueResponse is a partial representation of the Jira REST API v3
// issue response used for field extraction.
type jiraIssueResponse struct {
	Key    string `json:"key"`
	Fields struct {
		Summary     string `json:"summary"`
		Description any    `json:"description"` // ADF object (REST v3) or plain string (REST v2)
		Priority    struct {
			Name string `json:"name"`
		} `json:"priority"`
		Labels []string `json:"labels"`
	} `json:"fields"`
}

// FetchIssue retrieves an issue from Jira by key (e.g. "PROJ-123") and maps
// it to an ExternalIssue.
func (p *jiraProvider) FetchIssue(key string) (*ExternalIssue, error) {
	if !validIssueKeyRe.MatchString(key) {
		return nil, fmt.Errorf("tracker: invalid issue key %q: must contain only alphanumeric characters, hyphens, and underscores", key)
	}

	token := os.Getenv(p.cfg.TokenEnv)
	if token == "" {
		return nil, fmt.Errorf("tracker: env var %s is not set", p.cfg.TokenEnv)
	}

	url := strings.TrimRight(p.cfg.BaseURL, "/") + "/rest/api/3/issue/" + key
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("tracker: build request for %s: %w", key, err)
	}
	if p.cfg.UserEnv != "" {
		user := os.Getenv(p.cfg.UserEnv)
		if user == "" {
			return nil, fmt.Errorf("tracker: env var %s is not set", p.cfg.UserEnv)
		}
		req.SetBasicAuth(user, token)
	} else {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("tracker: fetch %s: %w", key, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("tracker: read response for %s: %w", key, err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("tracker: server returned %d for %s: %s",
			resp.StatusCode, key, strings.TrimSpace(string(body)))
	}

	var issue jiraIssueResponse
	if err := json.Unmarshal(body, &issue); err != nil {
		return nil, fmt.Errorf("tracker: parse response for %s: %w", key, err)
	}

	return &ExternalIssue{
		Key:         issue.Key,
		Title:       issue.Fields.Summary,
		Description: extractJiraDescription(issue.Fields.Description),
		Priority:    p.mapPriority(issue.Fields.Priority.Name),
		Labels:      issue.Fields.Labels,
		SourceURL:   strings.TrimRight(p.cfg.BaseURL, "/") + "/browse/" + issue.Key,
	}, nil
}

// mapPriority converts a Jira priority name to a Cistern priority integer.
func (p *jiraProvider) mapPriority(name string) int {
	pm := p.cfg.PriorityMap
	if len(pm) == 0 {
		pm = p.priorityMap
	}
	if v, ok := pm[name]; ok {
		return v
	}
	return 2 // default: normal
}

// extractJiraDescription extracts plain text from a Jira description field.
// Jira REST API v3 returns Atlassian Document Format (ADF); v2 returns a plain
// string. Both are handled.
func extractJiraDescription(raw any) string {
	if raw == nil {
		return ""
	}
	if s, ok := raw.(string); ok {
		return s
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return ""
	}
	return extractADFText(data)
}

type adfNode struct {
	Type    string    `json:"type"`
	Text    string    `json:"text"`
	Content []adfNode `json:"content"`
}

func extractADFText(data []byte) string {
	var doc adfNode
	if err := json.Unmarshal(data, &doc); err != nil {
		return ""
	}
	var sb strings.Builder
	walkADF(&doc, &sb)
	return strings.TrimSpace(sb.String())
}

func walkADF(node *adfNode, sb *strings.Builder) {
	if node.Text != "" {
		sb.WriteString(node.Text)
	}
	for i := range node.Content {
		walkADF(&node.Content[i], sb)
		switch node.Content[i].Type {
		case "paragraph", "heading", "listItem":
			sb.WriteString("\n")
		}
	}
}
