package jira

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/MichielDean/cistern/internal/aqueduct"
)

// helpers

func newTestProvider(t *testing.T, handler http.Handler) (*Provider, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	p := newWithClient(srv.URL, "user@example.com", "secret-token", srv.Client())
	return p, srv
}

func jiraIssueJSON(key, summary string, descContent []map[string]any, priorityName string, labels []string) string {
	var desc any
	if descContent != nil {
		desc = map[string]any{
			"type":    "doc",
			"version": 1,
			"content": descContent,
		}
	}
	issue := map[string]any{
		"key": key,
		"fields": map[string]any{
			"summary":     summary,
			"description": desc,
			"priority":    map[string]any{"name": priorityName},
			"labels":      labels,
		},
	}
	b, _ := json.Marshal(issue)
	return string(b)
}

// TestProvider_Name_ReturnsJira verifies that Name() returns the canonical provider identifier.
func TestProvider_Name_ReturnsJira(t *testing.T) {
	cfg := aqueduct.TrackerConfig{Name: "jira", URL: "https://example.atlassian.net", Email: "u@e.com", Token: "tok"}
	p := New(cfg)
	if got := p.Name(); got != "jira" {
		t.Errorf("Name() = %q, want %q", got, "jira")
	}
}

// TestProvider_FetchIssue_ReturnsIssue verifies FetchIssue maps all fields correctly on a 200 response.
func TestProvider_FetchIssue_ReturnsIssue(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		body := jiraIssueJSON("PROJ-42", "Fix the login bug",
			[]map[string]any{
				{
					"type": "paragraph",
					"content": []map[string]any{
						{"type": "text", "text": "The login page crashes on submit"},
					},
				},
			},
			"High",
			[]string{"bug", "login"},
		)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(body))
	})
	p, _ := newTestProvider(t, handler)

	issue, err := p.FetchIssue("PROJ-42")
	if err != nil {
		t.Fatalf("FetchIssue: unexpected error: %v", err)
	}

	if issue.Key != "PROJ-42" {
		t.Errorf("Key = %q, want %q", issue.Key, "PROJ-42")
	}
	if issue.Title != "Fix the login bug" {
		t.Errorf("Title = %q, want %q", issue.Title, "Fix the login bug")
	}
	if issue.Description != "The login page crashes on submit" {
		t.Errorf("Description = %q, want %q", issue.Description, "The login page crashes on submit")
	}
	if issue.Priority != 1 {
		t.Errorf("Priority = %d, want 1 (High)", issue.Priority)
	}
	if len(issue.Labels) != 2 || issue.Labels[0] != "bug" || issue.Labels[1] != "login" {
		t.Errorf("Labels = %v, want [bug login]", issue.Labels)
	}
}

// TestProvider_FetchIssue_SourceURL verifies SourceURL is constructed from base URL and key.
func TestProvider_FetchIssue_SourceURL(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(jiraIssueJSON("MYPROJ-7", "Test issue", nil, "Medium", nil)))
	})
	p, srv := newTestProvider(t, handler)

	issue, err := p.FetchIssue("MYPROJ-7")
	if err != nil {
		t.Fatalf("FetchIssue: unexpected error: %v", err)
	}

	wantURL := srv.URL + "/browse/MYPROJ-7"
	if issue.SourceURL != wantURL {
		t.Errorf("SourceURL = %q, want %q", issue.SourceURL, wantURL)
	}
}

// TestProvider_FetchIssue_RequestPath verifies the correct API path is requested.
func TestProvider_FetchIssue_RequestPath(t *testing.T) {
	var gotPath string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.RequestURI()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(jiraIssueJSON("PROJ-1", "Title", nil, "Medium", nil)))
	})
	p, _ := newTestProvider(t, handler)

	if _, err := p.FetchIssue("PROJ-1"); err != nil {
		t.Fatalf("FetchIssue: unexpected error: %v", err)
	}

	if !strings.HasPrefix(gotPath, "/rest/api/3/issue/PROJ-1") {
		t.Errorf("request path = %q, want prefix /rest/api/3/issue/PROJ-1", gotPath)
	}
	if !strings.Contains(gotPath, "fields=") {
		t.Errorf("request path %q missing fields query param", gotPath)
	}
}

// TestProvider_FetchIssue_SetsBasicAuthHeader verifies Basic Auth is sent with email:token credentials.
func TestProvider_FetchIssue_SetsBasicAuthHeader(t *testing.T) {
	var gotAuth string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(jiraIssueJSON("PROJ-1", "T", nil, "Medium", nil)))
	})
	p, _ := newTestProvider(t, handler)

	if _, err := p.FetchIssue("PROJ-1"); err != nil {
		t.Fatalf("FetchIssue: unexpected error: %v", err)
	}

	want := "Basic " + base64.StdEncoding.EncodeToString([]byte("user@example.com:secret-token"))
	if gotAuth != want {
		t.Errorf("Authorization = %q, want %q", gotAuth, want)
	}
}

// TestProvider_FetchIssue_ReturnsError_OnNon200Status verifies FetchIssue returns an error for non-200 responses.
func TestProvider_FetchIssue_ReturnsError_OnNon200Status(t *testing.T) {
	tests := []struct {
		name   string
		status int
	}{
		{"NotFound", http.StatusNotFound},
		{"Unauthorized", http.StatusUnauthorized},
		{"Forbidden", http.StatusForbidden},
		{"InternalServerError", http.StatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				http.Error(w, "error", tt.status)
			})
			p, _ := newTestProvider(t, handler)

			_, err := p.FetchIssue("PROJ-1")
			if err == nil {
				t.Errorf("FetchIssue: expected error for status %d, got nil", tt.status)
			}
		})
	}
}

// TestProvider_FetchIssue_NilResultOnError verifies FetchIssue returns nil issue on error.
func TestProvider_FetchIssue_NilResultOnError(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	})
	p, _ := newTestProvider(t, handler)

	got, _ := p.FetchIssue("PROJ-1")
	if got != nil {
		t.Errorf("FetchIssue: result = %+v, want nil on error", got)
	}
}

// TestProvider_FetchIssue_NullDescription verifies FetchIssue handles null Jira description gracefully.
func TestProvider_FetchIssue_NullDescription(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(jiraIssueJSON("PROJ-10", "No desc", nil, "Low", nil)))
	})
	p, _ := newTestProvider(t, handler)

	issue, err := p.FetchIssue("PROJ-10")
	if err != nil {
		t.Fatalf("FetchIssue: unexpected error: %v", err)
	}
	if issue.Description != "" {
		t.Errorf("Description = %q, want empty string for null description", issue.Description)
	}
}

// TestProvider_FetchIssue_EmptyLabels verifies FetchIssue handles empty labels array.
func TestProvider_FetchIssue_EmptyLabels(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(jiraIssueJSON("PROJ-5", "No labels", nil, "Medium", []string{})))
	})
	p, _ := newTestProvider(t, handler)

	issue, err := p.FetchIssue("PROJ-5")
	if err != nil {
		t.Fatalf("FetchIssue: unexpected error: %v", err)
	}
	if len(issue.Labels) != 0 {
		t.Errorf("Labels = %v, want empty", issue.Labels)
	}
}

// TestProvider_FetchIssue_KeyFromResponse verifies the Key field is taken from the response body.
func TestProvider_FetchIssue_KeyFromResponse(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(jiraIssueJSON("ABC-99", "Title", nil, "Medium", nil)))
	})
	p, _ := newTestProvider(t, handler)

	issue, err := p.FetchIssue("ABC-99")
	if err != nil {
		t.Fatalf("FetchIssue: unexpected error: %v", err)
	}
	if issue.Key != "ABC-99" {
		t.Errorf("Key = %q, want %q", issue.Key, "ABC-99")
	}
}

// TestMapPriority verifies all Jira priority names map to the correct normalized int.
func TestMapPriority(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"Highest", 1},
		{"highest", 1},
		{"High", 1},
		{"high", 1},
		{"Medium", 2},
		{"medium", 2},
		{"Low", 3},
		{"low", 3},
		{"Lowest", 4},
		{"lowest", 4},
		{"", 2},
		{"Unknown", 2},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := mapPriority(tt.input)
			if got != tt.want {
				t.Errorf("mapPriority(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

// TestProvider_FetchIssue_PriorityMapping verifies priority field is mapped correctly from the API response.
func TestProvider_FetchIssue_PriorityMapping(t *testing.T) {
	tests := []struct {
		priorityName string
		wantPriority int
	}{
		{"Highest", 1},
		{"High", 1},
		{"Medium", 2},
		{"Low", 3},
		{"Lowest", 4},
	}
	for _, tt := range tests {
		t.Run(tt.priorityName, func(t *testing.T) {
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(jiraIssueJSON("PROJ-1", "T", nil, tt.priorityName, nil)))
			})
			p, _ := newTestProvider(t, handler)

			issue, err := p.FetchIssue("PROJ-1")
			if err != nil {
				t.Fatalf("FetchIssue: unexpected error: %v", err)
			}
			if issue.Priority != tt.wantPriority {
				t.Errorf("Priority = %d, want %d (priority name: %q)", issue.Priority, tt.wantPriority, tt.priorityName)
			}
		})
	}
}

// TestADFToPlainText verifies ADF document nodes are correctly converted to plain text.
func TestADFToPlainText(t *testing.T) {
	tests := []struct {
		name  string
		doc   *adfDocument
		want  string
	}{
		{
			name: "NilDocument",
			doc:  nil,
			want: "",
		},
		{
			name: "SingleParagraph",
			doc: &adfDocument{
				Type: "doc",
				Content: []adfNode{
					{
						Type: "paragraph",
						Content: []adfNode{
							{Type: "text", Text: "Hello world"},
						},
					},
				},
			},
			want: "Hello world",
		},
		{
			name: "MultipleParagraphs",
			doc: &adfDocument{
				Type: "doc",
				Content: []adfNode{
					{
						Type: "paragraph",
						Content: []adfNode{
							{Type: "text", Text: "First paragraph"},
						},
					},
					{
						Type: "paragraph",
						Content: []adfNode{
							{Type: "text", Text: "Second paragraph"},
						},
					},
				},
			},
			want: "First paragraph\nSecond paragraph",
		},
		{
			name: "EmptyDoc",
			doc: &adfDocument{
				Type:    "doc",
				Content: []adfNode{},
			},
			want: "",
		},
		{
			name: "NestedInlineText",
			doc: &adfDocument{
				Type: "doc",
				Content: []adfNode{
					{
						Type: "paragraph",
						Content: []adfNode{
							{Type: "text", Text: "Hello"},
							{Type: "text", Text: " "},
							{Type: "text", Text: "world"},
						},
					},
				},
			},
			want: "Hello world",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := adfToPlainText(tt.doc)
			if got != tt.want {
				t.Errorf("adfToPlainText() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestNew_UsesConfigFields verifies New() correctly reads URL, Email, and Token from TrackerConfig.
func TestNew_UsesConfigFields(t *testing.T) {
	var gotAuth string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(jiraIssueJSON("P-1", "T", nil, "Medium", nil)))
	})
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	cfg := aqueduct.TrackerConfig{
		Name:  "jira",
		URL:   srv.URL,
		Email: "admin@corp.com",
		Token: "mytoken",
	}
	p := New(cfg)
	p.httpClient = srv.Client()

	if _, err := p.FetchIssue("P-1"); err != nil {
		t.Fatalf("FetchIssue: unexpected error: %v", err)
	}

	want := "Basic " + base64.StdEncoding.EncodeToString([]byte("admin@corp.com:mytoken"))
	if gotAuth != want {
		t.Errorf("Authorization = %q, want %q", gotAuth, want)
	}
}

// TestNew_ResolvesTokenFromEnv verifies New() resolves token via TokenEnv when set.
func TestNew_ResolvesTokenFromEnv(t *testing.T) {
	t.Setenv("TEST_JIRA_TOKEN", "env-token-value")

	var gotAuth string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(jiraIssueJSON("P-1", "T", nil, "Medium", nil)))
	})
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	cfg := aqueduct.TrackerConfig{
		Name:     "jira",
		URL:      srv.URL,
		Email:    "u@e.com",
		TokenEnv: "TEST_JIRA_TOKEN",
	}
	p := New(cfg)
	p.httpClient = srv.Client()

	if _, err := p.FetchIssue("P-1"); err != nil {
		t.Fatalf("FetchIssue: unexpected error: %v", err)
	}

	want := "Basic " + base64.StdEncoding.EncodeToString([]byte("u@e.com:env-token-value"))
	if gotAuth != want {
		t.Errorf("Authorization = %q, want %q", gotAuth, want)
	}
}

// TestProvider_FetchIssue_BaseURLTrailingSlashStripped verifies trailing slashes in the base URL do not produce double slashes.
func TestProvider_FetchIssue_BaseURLTrailingSlashStripped(t *testing.T) {
	var gotPath string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(jiraIssueJSON("PROJ-1", "T", nil, "Medium", nil)))
	})
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	p := newWithClient(srv.URL+"/", "u@e.com", "tok", srv.Client())
	if _, err := p.FetchIssue("PROJ-1"); err != nil {
		t.Fatalf("FetchIssue: unexpected error: %v", err)
	}

	if strings.Contains(gotPath, "//") {
		t.Errorf("request path %q contains double slashes — base URL trailing slash not stripped", gotPath)
	}
}
