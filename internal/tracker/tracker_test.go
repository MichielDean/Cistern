package tracker

import (
	"errors"
	"testing"
)

// fakeProvider is a test double implementing TrackerProvider.
type fakeProvider struct {
	name   string
	issues map[string]*ExternalIssue
	err    error
}

func (f *fakeProvider) Name() string { return f.name }

func (f *fakeProvider) FetchIssue(key string) (*ExternalIssue, error) {
	if f.err != nil {
		return nil, f.err
	}
	issue, ok := f.issues[key]
	if !ok {
		return nil, errors.New("tracker: issue not found: " + key)
	}
	return issue, nil
}

// TestExternalIssue_FieldsAreMappedCorrectly verifies all ExternalIssue fields can be set and read.
func TestExternalIssue_FieldsAreMappedCorrectly(t *testing.T) {
	issue := ExternalIssue{
		Key:         "PROJ-123",
		Title:       "Fix the login bug",
		Description: "The login page crashes on submit",
		Priority:    2,
		Labels:      []string{"bug", "login"},
		SourceURL:   "https://example.atlassian.net/browse/PROJ-123",
	}

	if issue.Key != "PROJ-123" {
		t.Errorf("Key = %q, want %q", issue.Key, "PROJ-123")
	}
	if issue.Title != "Fix the login bug" {
		t.Errorf("Title = %q, want %q", issue.Title, "Fix the login bug")
	}
	if issue.Description != "The login page crashes on submit" {
		t.Errorf("Description = %q, want %q", issue.Description, "The login page crashes on submit")
	}
	if issue.Priority != 2 {
		t.Errorf("Priority = %d, want 2", issue.Priority)
	}
	if len(issue.Labels) != 2 || issue.Labels[0] != "bug" || issue.Labels[1] != "login" {
		t.Errorf("Labels = %v, want [bug login]", issue.Labels)
	}
	if issue.SourceURL != "https://example.atlassian.net/browse/PROJ-123" {
		t.Errorf("SourceURL = %q, want expected URL", issue.SourceURL)
	}
}

// TestExternalIssue_PriorityRange verifies all normalized priority values (1–4) are representable.
func TestExternalIssue_PriorityRange(t *testing.T) {
	for _, p := range []int{1, 2, 3, 4} {
		issue := ExternalIssue{Priority: p}
		if issue.Priority != p {
			t.Errorf("Priority = %d, want %d", issue.Priority, p)
		}
	}
}

// TestExternalIssue_EmptyLabels verifies that zero-value Labels is nil and safe to range over.
func TestExternalIssue_EmptyLabels(t *testing.T) {
	var issue ExternalIssue
	count := 0
	for range issue.Labels {
		count++
	}
	if count != 0 {
		t.Errorf("ranging over nil Labels yielded %d iterations, want 0", count)
	}
}

// TestTrackerProvider_Name_ReturnsProviderName verifies Name() returns the provider identifier.
func TestTrackerProvider_Name_ReturnsProviderName(t *testing.T) {
	tests := []struct {
		name string
	}{
		{"jira"},
		{"linear"},
		{"github"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var p TrackerProvider = &fakeProvider{name: tt.name}
			if got := p.Name(); got != tt.name {
				t.Errorf("Name() = %q, want %q", got, tt.name)
			}
		})
	}
}

// TestTrackerProvider_FetchIssue_ReturnsIssueForValidKey verifies FetchIssue returns the correct issue.
func TestTrackerProvider_FetchIssue_ReturnsIssueForValidKey(t *testing.T) {
	want := &ExternalIssue{
		Key:      "PROJ-1",
		Title:    "Test issue",
		Priority: 1,
	}
	var p TrackerProvider = &fakeProvider{
		name:   "jira",
		issues: map[string]*ExternalIssue{"PROJ-1": want},
	}

	got, err := p.FetchIssue("PROJ-1")
	if err != nil {
		t.Fatalf("FetchIssue: unexpected error: %v", err)
	}
	if got.Key != want.Key {
		t.Errorf("Key = %q, want %q", got.Key, want.Key)
	}
	if got.Title != want.Title {
		t.Errorf("Title = %q, want %q", got.Title, want.Title)
	}
	if got.Priority != want.Priority {
		t.Errorf("Priority = %d, want %d", got.Priority, want.Priority)
	}
}

// TestTrackerProvider_FetchIssue_ReturnsErrorForUnknownKey verifies FetchIssue
// returns an error when the key is not found.
func TestTrackerProvider_FetchIssue_ReturnsErrorForUnknownKey(t *testing.T) {
	var p TrackerProvider = &fakeProvider{
		name:   "jira",
		issues: map[string]*ExternalIssue{},
	}

	_, err := p.FetchIssue("UNKNOWN-999")
	if err == nil {
		t.Fatal("FetchIssue: expected error for unknown key, got nil")
	}
}

// TestTrackerProvider_FetchIssue_PropagatesProviderError verifies FetchIssue
// surfaces errors returned by the underlying provider.
func TestTrackerProvider_FetchIssue_PropagatesProviderError(t *testing.T) {
	wantErr := errors.New("connection refused")
	var p TrackerProvider = &fakeProvider{name: "jira", err: wantErr}

	_, err := p.FetchIssue("PROJ-1")
	if !errors.Is(err, wantErr) {
		t.Errorf("FetchIssue: error = %v, want %v", err, wantErr)
	}
}

// TestTrackerProvider_FetchIssue_ReturnsNilResultOnError verifies FetchIssue
// returns a nil issue pointer when an error occurs.
func TestTrackerProvider_FetchIssue_ReturnsNilResultOnError(t *testing.T) {
	var p TrackerProvider = &fakeProvider{name: "jira", err: errors.New("timeout")}

	got, _ := p.FetchIssue("PROJ-1")
	if got != nil {
		t.Errorf("FetchIssue: result = %+v, want nil on error", got)
	}
}
