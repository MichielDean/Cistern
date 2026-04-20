package tracker

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestExtractJiraDescription_WithNilDescription_ReturnsEmpty(t *testing.T) {
	got := extractJiraDescription(nil)
	if got != "" {
		t.Errorf("extractJiraDescription(nil) = %q, want %q", got, "")
	}
}

func TestExtractJiraDescription_WithADFDoc_ExtractsText(t *testing.T) {
	// Given: a minimal ADF document with a paragraph containing text.
	adf := map[string]any{
		"type":    "doc",
		"version": 1,
		"content": []any{
			map[string]any{
				"type": "paragraph",
				"content": []any{
					map[string]any{
						"type": "text",
						"text": "Hello ADF world",
					},
				},
			},
		},
	}

	// When: extractJiraDescription is called with the ADF object.
	got := extractJiraDescription(adf)

	// Then: the plain text is returned.
	want := "Hello ADF world"
	if got != want {
		t.Errorf("extractJiraDescription(ADF) = %q, want %q", got, want)
	}
}

func TestFetchIssue_PopulatesKeyLabelsSourceURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"key": "PROJ-123",
			"fields": map[string]any{
				"summary": "Test issue",
				"description": map[string]any{
					"type":    "doc",
					"version": 1,
					"content": []any{
						map[string]any{
							"type": "paragraph",
							"content": []any{
								map[string]any{
									"type": "text",
									"text": "Body text",
								},
							},
						},
					},
				},
				"priority": map[string]any{"name": "High"},
				"labels":   []string{"bug", "frontend"},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	p, err := newJiraProvider(TrackerConfig{
		BaseURL:  srv.URL,
		TokenEnv: "TEST_JIRA_TOKEN",
	})
	if err != nil {
		t.Fatalf("newJiraProvider: %v", err)
	}

	t.Setenv("TEST_JIRA_TOKEN", "fake-token")

	issue, err := p.FetchIssue("PROJ-123")
	if err != nil {
		t.Fatalf("FetchIssue: %v", err)
	}

	if issue.Key != "PROJ-123" {
		t.Errorf("Key = %q, want %q", issue.Key, "PROJ-123")
	}
	if len(issue.Labels) != 2 || issue.Labels[0] != "bug" || issue.Labels[1] != "frontend" {
		t.Errorf("Labels = %v, want [bug frontend]", issue.Labels)
	}
	wantURL := srv.URL + "/browse/PROJ-123"
	if issue.SourceURL != wantURL {
		t.Errorf("SourceURL = %q, want %q", issue.SourceURL, wantURL)
	}
}

func TestFetchIssue_RejectsInvalidKey(t *testing.T) {
	tests := []struct {
		name string
		key  string
	}{
		{"slash", "PROJ/123"},
		{"dot", "PROJ.123"},
		{"path traversal", "../etc/passwd"},
		{"space", "PROJ 123"},
		{"empty", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := newJiraProvider(TrackerConfig{
				BaseURL:  "https://example.atlassian.net",
				TokenEnv: "TEST_JIRA_TOKEN_INVALID",
			})
			if err != nil {
				t.Fatal(err)
			}
			_, err = p.FetchIssue(tt.key)
			if err == nil {
				t.Errorf("FetchIssue(%q) should reject invalid key", tt.key)
			}
			if !strings.Contains(err.Error(), "invalid issue key") {
				t.Errorf("error = %q, want 'invalid issue key'", err.Error())
			}
		})
	}
}

func TestFetchIssue_AcceptsValidKeys(t *testing.T) {
	keys := []string{"PROJ-123", "ABC_XYZ-456", "simple", "My-Key_99"}
	for _, key := range keys {
		t.Run(key, func(t *testing.T) {
			if !validIssueKeyRe.MatchString(key) {
				t.Errorf("key %q should be valid but was rejected", key)
			}
		})
	}
}
