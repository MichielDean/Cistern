package tracker_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/MichielDean/cistern/internal/tracker"
)

// --- Registry tests ---

func TestRegister_AndResolve_ReturnsRegisteredConstructor(t *testing.T) {
	const name = "test-register-resolve"
	want := &fakeProvider{}
	tracker.Register(name, func(cfg tracker.TrackerConfig) (tracker.TrackerProvider, error) {
		return want, nil
	})

	ctor, ok := tracker.Resolve(name)
	if !ok {
		t.Fatal("expected constructor to be found after Register")
	}
	got, err := ctor(tracker.TrackerConfig{Name: name})
	if err != nil {
		t.Fatalf("constructor error: %v", err)
	}
	if got != want {
		t.Errorf("constructor returned wrong provider instance")
	}
}

func TestResolve_ReturnsFalseForUnknownProvider(t *testing.T) {
	_, ok := tracker.Resolve("no-such-provider-xyz-abc")
	if ok {
		t.Error("expected false for unknown provider name, got true")
	}
}

// --- Jira provider tests ---

func TestJiraProvider_FetchIssue_ReturnsSummaryAndDescription(t *testing.T) {
	srv := jiraServer("PROJ-1", "Fix the bug", "Bug description text", "High", http.StatusOK)
	defer srv.Close()

	ctor, ok := tracker.Resolve("jira")
	if !ok {
		t.Fatal("jira provider not registered")
	}
	t.Setenv("JIRA_TOKEN", "test-token")
	t.Setenv("JIRA_USER", "test@example.com")

	p, err := ctor(tracker.TrackerConfig{
		Name:     "jira",
		BaseURL:  srv.URL,
		TokenEnv: "JIRA_TOKEN",
		UserEnv:  "JIRA_USER",
	})
	if err != nil {
		t.Fatalf("constructor: %v", err)
	}

	issue, err := p.FetchIssue("PROJ-1")
	if err != nil {
		t.Fatalf("FetchIssue: %v", err)
	}

	if issue.Title != "Fix the bug" {
		t.Errorf("Title = %q, want %q", issue.Title, "Fix the bug")
	}
	if issue.Description != "Bug description text" {
		t.Errorf("Description = %q, want %q", issue.Description, "Bug description text")
	}
	if issue.Priority != 1 {
		t.Errorf("Priority = %d, want 1 (High maps to 1 by default)", issue.Priority)
	}
}

func TestJiraProvider_FetchIssue_MapsDefaultPriority_Medium(t *testing.T) {
	srv := jiraServer("PROJ-2", "Medium task", "", "Medium", http.StatusOK)
	defer srv.Close()

	ctor, _ := tracker.Resolve("jira")
	t.Setenv("JIRA_TOKEN_MED", "test-token")

	p, err := ctor(tracker.TrackerConfig{
		Name:     "jira",
		BaseURL:  srv.URL,
		TokenEnv: "JIRA_TOKEN_MED",
	})
	if err != nil {
		t.Fatal(err)
	}

	issue, err := p.FetchIssue("PROJ-2")
	if err != nil {
		t.Fatalf("FetchIssue: %v", err)
	}
	if issue.Priority != 2 {
		t.Errorf("Priority = %d, want 2 (Medium)", issue.Priority)
	}
}

func TestJiraProvider_FetchIssue_UsesPriorityMapOverride(t *testing.T) {
	srv := jiraServer("PROJ-3", "Urgent", "", "High", http.StatusOK)
	defer srv.Close()

	ctor, _ := tracker.Resolve("jira")
	t.Setenv("JIRA_TOKEN_OVR", "tok")

	p, err := ctor(tracker.TrackerConfig{
		Name:        "jira",
		BaseURL:     srv.URL,
		TokenEnv:    "JIRA_TOKEN_OVR",
		PriorityMap: map[string]int{"High": 3},
	})
	if err != nil {
		t.Fatal(err)
	}

	issue, err := p.FetchIssue("PROJ-3")
	if err != nil {
		t.Fatalf("FetchIssue: %v", err)
	}
	if issue.Priority != 3 {
		t.Errorf("Priority = %d, want 3 (overridden via PriorityMap)", issue.Priority)
	}
}

func TestJiraProvider_FetchIssue_ReturnsErrorOnHTTPNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprintln(w, `{"errorMessages":["Issue does not exist"],"errors":{}}`)
	}))
	defer srv.Close()

	ctor, _ := tracker.Resolve("jira")
	t.Setenv("JIRA_TOKEN_404", "tok")

	p, err := ctor(tracker.TrackerConfig{
		Name:     "jira",
		BaseURL:  srv.URL,
		TokenEnv: "JIRA_TOKEN_404",
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = p.FetchIssue("MISSING-1")
	if err == nil {
		t.Fatal("expected error for 404 response, got nil")
	}
}

func TestJiraProvider_FetchIssue_ReturnsErrorWhenTokenEnvNotSet(t *testing.T) {
	ctor, _ := tracker.Resolve("jira")

	p, err := ctor(tracker.TrackerConfig{
		Name:     "jira",
		BaseURL:  "https://example.atlassian.net",
		TokenEnv: "JIRA_TOKEN_NOT_SET_XYZ_UNIQUE",
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = p.FetchIssue("PROJ-1")
	if err == nil {
		t.Fatal("expected error when token env var is unset, got nil")
	}
}

func TestJiraProvider_FetchIssue_DefaultPriority_UnknownName(t *testing.T) {
	srv := jiraServer("PROJ-4", "Task", "", "Blocker", http.StatusOK)
	defer srv.Close()

	ctor, _ := tracker.Resolve("jira")
	t.Setenv("JIRA_TOKEN_UNK", "tok")

	p, err := ctor(tracker.TrackerConfig{
		Name:     "jira",
		BaseURL:  srv.URL,
		TokenEnv: "JIRA_TOKEN_UNK",
	})
	if err != nil {
		t.Fatal(err)
	}

	issue, err := p.FetchIssue("PROJ-4")
	if err != nil {
		t.Fatalf("FetchIssue: %v", err)
	}
	if issue.Priority != 2 {
		t.Errorf("Priority = %d, want 2 (default for unknown priority name)", issue.Priority)
	}
}

func TestJiraProvider_FetchIssue_ReturnsErrorWhenUserEnvSetButEmpty(t *testing.T) {
	// Given: UserEnv is configured but the env var resolves to an empty string.
	srv := jiraServer("PROJ-5", "Some issue", "", "Medium", http.StatusOK)
	defer srv.Close()

	ctor, _ := tracker.Resolve("jira")
	t.Setenv("JIRA_TOKEN_USERENV", "test-token")
	// JIRA_USER_EMPTY is deliberately not set so os.Getenv returns "".
	t.Setenv("JIRA_USER_EMPTY", "")

	p, err := ctor(tracker.TrackerConfig{
		Name:     "jira",
		BaseURL:  srv.URL,
		TokenEnv: "JIRA_TOKEN_USERENV",
		UserEnv:  "JIRA_USER_EMPTY",
	})
	if err != nil {
		t.Fatal(err)
	}

	// When: FetchIssue is called.
	_, err = p.FetchIssue("PROJ-5")

	// Then: an error is returned mentioning the env var, not a silent broken request.
	if err == nil {
		t.Fatal("expected error when UserEnv is set but env var is empty, got nil")
	}
	if !strings.Contains(err.Error(), "JIRA_USER_EMPTY") {
		t.Errorf("error %q does not mention the env var name", err.Error())
	}
}

func TestJiraProvider_FetchIssue_TimesOutOnSlowServer(t *testing.T) {
	// Given: a server that blocks indefinitely (simulates unreachable server).
	done := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-done // block until the test ends
	}))
	defer srv.Close()
	defer close(done)

	ctor, _ := tracker.Resolve("jira")
	t.Setenv("JIRA_TOKEN_TIMEOUT", "test-token")

	p, err := ctor(tracker.TrackerConfig{
		Name:     "jira",
		BaseURL:  srv.URL,
		TokenEnv: "JIRA_TOKEN_TIMEOUT",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Override the HTTP timeout on the constructed provider.
	if setter, ok := p.(interface{ SetHTTPTimeout(time.Duration) }); ok {
		setter.SetHTTPTimeout(50 * time.Millisecond)
	}

	// When: FetchIssue is called.
	_, err = p.FetchIssue("PROJ-X")

	// Then: an error is returned (request timed out).
	if err == nil {
		t.Fatal("expected timeout error from slow server, got nil")
	}
}

func TestJiraProvider_FetchIssue_WithADFDescription(t *testing.T) {
	// Given: a Jira API server that returns an ADF object as the description.
	adfPayload := map[string]any{
		"type":    "doc",
		"version": 1,
		"content": []any{
			map[string]any{
				"type": "paragraph",
				"content": []any{
					map[string]any{
						"type": "text",
						"text": "This is an ADF description",
					},
				},
			},
		},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		resp := map[string]any{
			"key": "PROJ-10",
			"fields": map[string]any{
				"summary":     "ADF issue",
				"description": adfPayload,
				"priority":    map[string]string{"name": "Medium"},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	ctor, _ := tracker.Resolve("jira")
	t.Setenv("JIRA_TOKEN_ADF", "test-token")

	p, err := ctor(tracker.TrackerConfig{
		Name:     "jira",
		BaseURL:  srv.URL,
		TokenEnv: "JIRA_TOKEN_ADF",
	})
	if err != nil {
		t.Fatal(err)
	}

	// When: FetchIssue is called.
	issue, err := p.FetchIssue("PROJ-10")

	// Then: the description is extracted from the ADF structure.
	if err != nil {
		t.Fatalf("FetchIssue: %v", err)
	}
	want := "This is an ADF description"
	if issue.Description != want {
		t.Errorf("Description = %q, want %q", issue.Description, want)
	}
}

// --- helpers ---

type fakeProvider struct{}

func (f *fakeProvider) Name() string {
	return "fake"
}

func (f *fakeProvider) FetchIssue(key string) (*tracker.ExternalIssue, error) {
	return &tracker.ExternalIssue{}, nil
}

// jiraServer returns an httptest.Server that returns a minimal Jira issue response.
func jiraServer(key, summary, description, priority string, statusCode int) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		resp := map[string]any{
			"key": key,
			"fields": map[string]any{
				"summary":     summary,
				"description": description,
				"priority": map[string]string{
					"name": priority,
				},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
}
