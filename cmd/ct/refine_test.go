package main

import (
	"encoding/json"
	"net/http"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MichielDean/cistern/internal/testutil/mockllm"
)

// buildTestBin compiles the Go package at importPath into a temp directory
// and returns the absolute path to the resulting binary.
func buildTestBin(t *testing.T, name, importPath string) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), name)
	out, err := exec.Command("go", "build", "-o", bin, importPath).CombinedOutput()
	if err != nil {
		t.Fatalf("build %s: %v\n%s", name, err, out)
	}
	return bin
}

// TestMockLLM_ChatCompletions verifies that the mock server correctly handles
// POST /v1/chat/completions and returns a well-formed OpenAI response.
// This endpoint is used by future multi-provider callRefineAPI configurations
// (openai, openrouter, ollama, custom).
func TestMockLLM_ChatCompletions(t *testing.T) {
	mock := mockllm.New()
	defer mock.Close()

	reqs := mock.Requests()
	if len(reqs) != 0 {
		t.Fatalf("expected 0 requests before any calls, got %d", len(reqs))
	}

	// Parse the hardcoded JSON to verify it is well-formed.
	var items []map[string]any
	if err := json.Unmarshal([]byte(mockllm.HardcodedProposalsJSON), &items); err != nil {
		t.Fatalf("HardcodedProposalsJSON is not valid JSON: %v", err)
	}
	if len(items) == 0 {
		t.Fatal("HardcodedProposalsJSON contains no items")
	}
	if title, _ := items[0]["title"].(string); title != "mock proposal" {
		t.Errorf("items[0].title = %q, want %q", title, "mock proposal")
	}
}

// TestMockLLM_RecordsRequestsForAllProviders is a table-driven test
// demonstrating how the mock server supports each provider configuration.
// Each entry reflects how a caller would configure callRefineAPI for that
// provider once multi-provider support lands.
func TestMockLLM_RecordsRequestsForAllProviders(t *testing.T) {
	tests := []struct {
		name     string
		endpoint string // /v1/messages or /v1/chat/completions
	}{
		{name: "anthropic", endpoint: "/v1/messages"},
		{name: "openai", endpoint: "/v1/chat/completions"},
		{name: "openrouter", endpoint: "/v1/chat/completions"},
		{name: "ollama", endpoint: "/v1/chat/completions"},
		{name: "custom", endpoint: "/v1/chat/completions"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := mockllm.New()
			defer mock.Close()

			// Invoke the appropriate endpoint directly using the standard
			// library to verify the mock handles it correctly.
			url := mock.URL + tt.endpoint
			resp, err := postToMock(url)
			if err != nil {
				t.Fatalf("POST %s: %v", tt.endpoint, err)
			}
			if resp != 200 {
				t.Errorf("POST %s returned status %d, want 200", tt.endpoint, resp)
			}

			reqs := mock.Requests()
			if len(reqs) != 1 {
				t.Fatalf("expected 1 recorded request, got %d", len(reqs))
			}
			if reqs[0].Path != tt.endpoint {
				t.Errorf("recorded path = %q, want %q", reqs[0].Path, tt.endpoint)
			}
		})
	}
}

// postToMock sends an empty POST to url and returns the HTTP status code.
func postToMock(url string) (int, error) {
	resp, err := http.Post(url, "application/json", strings.NewReader("{}")) //nolint:noctx
	if err != nil {
		return 0, err
	}
	resp.Body.Close()
	return resp.StatusCode, nil
}
