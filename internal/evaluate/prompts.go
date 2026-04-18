package evaluate

import (
	"fmt"
	"os/exec"
	"strings"
)

func codebaseTools() []Tool {
	return []Tool{
		{
			Type: "function",
			Function: ToolFunction{
				Name:        "Read",
				Description: "Read a file from the codebase. Returns file contents.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"file_path": map[string]any{"type": "string", "description": "Path relative to codebase root"},
					},
					"required": []string{"file_path"},
				},
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name:        "Glob",
				Description: "Find files matching a glob pattern. Returns matching file paths.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"pattern": map[string]any{"type": "string", "description": "Glob pattern"},
					},
					"required": []string{"pattern"},
				},
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name:        "Grep",
				Description: "Search file contents with a regex pattern. Returns matching lines.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"pattern": map[string]any{"type": "string", "description": "Regex pattern"},
						"path":   map[string]any{"type": "string", "description": "Directory (optional)"},
					},
					"required": []string{"pattern"},
				},
			},
		},
	}
}

func readCodebaseFiles(dir string) (string, error) {
	cmd := exec.Command("git", "ls-files", "--", "*.go")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git ls-files: %w", err)
	}
	allFiles := strings.TrimSpace(string(out))
	if allFiles == "" {
		return "", nil
	}

	coreSet := map[string]bool{
		"internal/cistern/client.go":        true,
		"internal/tracker/tracker.go":       true,
		"internal/tracker/jira.go":           true,
		"internal/provider/preset.go":       true,
		"internal/cataractae/context.go":     true,
		"internal/cataractae/runner.go":      true,
		"internal/cataractae/session.go":     true,
		"internal/castellarius/scheduler.go": true,
		"cmd/ct/cistern.go":                  true,
		"go.mod":                             true,
	}

	var coreList, otherList []string
	for _, f := range strings.Split(allFiles, "\n") {
		if strings.HasSuffix(f, "_test.go") || strings.Contains(f, "testutil/") || strings.Contains(f, "mock") {
			continue
		}
		if coreSet[f] || strings.HasPrefix(f, "internal/cistern/") || strings.HasPrefix(f, "internal/tracker/") || strings.HasPrefix(f, "internal/provider/") {
			coreList = append(coreList, f)
		} else if !strings.Contains(f, "tui.go") && !strings.Contains(f, "dashboard") {
			otherList = append(otherList, f)
		}
	}

	var sb strings.Builder
	sb.WriteString("## Core Files (full content)\n\n")
	for _, f := range coreList {
		content := readFileContent(dir, f)
		if content == "" {
			continue
		}
		if len(content) > 15000 {
			content = content[:15000] + "\n... (truncated)"
		}
		sb.WriteString(fmt.Sprintf("### %s\n\n```go\n%s\n```\n\n", f, content))
	}

	sb.WriteString("## Other Files (signatures only)\n\n")
	for _, f := range otherList {
		content := readFileContent(dir, f)
		if content == "" {
			continue
		}
		sigs := extractSignatures(content)
		if sigs == "" {
			continue
		}
		sb.WriteString(fmt.Sprintf("### %s\n\n```go\n%s\n```\n\n", f, sigs))
	}

	return sb.String(), nil
}

func extractSignatures(content string) string {
	var lines []string
	braceDepth := 0
	inBody := false
	for _, line := range strings.Split(content, "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "package ") || strings.HasPrefix(t, "import ") ||
			t == "import (" || strings.HasPrefix(t, "type ") || strings.HasPrefix(t, "func ") ||
			strings.HasPrefix(t, "const ") || strings.HasPrefix(t, "var ") ||
			strings.HasPrefix(t, "//") || t == "" || strings.HasPrefix(t, "interface {") || braceDepth == 0 {
			lines = append(lines, line)
			if strings.HasPrefix(t, "func ") || strings.HasPrefix(t, "type ") {
				braceDepth += strings.Count(t, "{") - strings.Count(t, "}")
				inBody = braceDepth > 0
			}
		} else if inBody {
			braceDepth += strings.Count(t, "{") - strings.Count(t, "}")
			if braceDepth <= 0 {
				inBody = false
				braceDepth = 0
				lines = append(lines, "}", "")
			}
		}
	}
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n")
}

func readFileContent(dir, path string) string {
	cmd := exec.Command("cat", path)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return string(out)
}