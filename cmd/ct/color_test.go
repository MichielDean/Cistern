package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/MichielDean/cistern/internal/cistern"
)

func TestStatusCode(t *testing.T) {
	tests := []struct {
		status string
		want   string
	}{
		{"flowing", colorGreen},
		{"queued", colorYellow},
		{"awaiting", colorYellow},
		{"stagnant", colorRed},
		{"delivered", colorDim},
		{"unknown", ""},
		{"", ""},
	}
	for _, tt := range tests {
		got := statusCode(tt.status)
		if got != tt.want {
			t.Errorf("statusCode(%q) = %q, want %q", tt.status, got, tt.want)
		}
	}
}

// statusCell — in non-terminal mode (tests) ANSI codes are not injected.
// Expected format: icon + " " + padRight(status, width-2)

func TestStatusCell(t *testing.T) {
	// width=12: textWidth = 12-2 = 10
	tests := []struct {
		status string
		width  int
		want   string
	}{
		// icon(1) + space(1) + padRight(status, 10)
		{"flowing", 12, "● " + padRight("flowing", 10)},
		{"queued", 12, "○ " + padRight("queued", 10)},
		{"awaiting", 12, "⏸ " + padRight("awaiting", 10)},
		{"stagnant", 12, "✗ " + padRight("stagnant", 10)},
		{"delivered", 12, "✓ " + padRight("delivered", 10)},
		// unknown status: icon is " ", no color code
		{"unknown", 12, "  " + padRight("unknown", 10)},
		// tiny width: textWidth clamped to 1
		{"flowing", 2, "● " + padRight("flowing", 1)},
	}
	for _, tt := range tests {
		got := statusCell(tt.status, tt.width)
		if got != tt.want {
			t.Errorf("statusCell(%q, %d) = %q, want %q", tt.status, tt.width, got, tt.want)
		}
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		s    string
		max  int
		want string
	}{
		{"hello", 10, "hello"},    // shorter than max — unchanged
		{"hello", 5, "hello"},     // exactly max — unchanged
		{"hello world", 5, "hell…"}, // longer than max
		{"hello", 1, "…"},         // max <= 1
		{"hi", 1, "…"},            // max <= 1
		{"hello", 0, "…"},         // max <= 1 (0)
		{"", 5, ""},               // empty string
		{"αβγδε", 3, "αβ…"},       // multi-byte runes
	}
	for _, tt := range tests {
		got := truncate(tt.s, tt.max)
		if got != tt.want {
			t.Errorf("truncate(%q, %d) = %q, want %q", tt.s, tt.max, got, tt.want)
		}
	}
}

func TestSkillDesc(t *testing.T) {
	// writeSkillMD writes content to a temp SKILL.md and returns its path.
	writeSkillMD := func(t *testing.T, content string) string {
		t.Helper()
		p := filepath.Join(t.TempDir(), "SKILL.md")
		if err := os.WriteFile(p, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
		return p
	}

	t.Run("yaml frontmatter description", func(t *testing.T) {
		p := writeSkillMD(t, "---\nname: test\ndescription: My skill description\n---\nBody text here.\n")
		got := skillDesc(p)
		if got != "My skill description" {
			t.Errorf("got %q, want %q", got, "My skill description")
		}
	})

	t.Run("yaml frontmatter description truncated", func(t *testing.T) {
		long := strings.Repeat("x", 60)
		p := writeSkillMD(t, "---\ndescription: "+long+"\n---\n")
		got := skillDesc(p)
		want := truncate(long, 50)
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("fallback to first non-blank non-heading line", func(t *testing.T) {
		p := writeSkillMD(t, "---\n---\n# Heading\n\nFirst real line.\n")
		got := skillDesc(p)
		if got != "First real line." {
			t.Errorf("got %q, want %q", got, "First real line.")
		}
	})

	t.Run("no frontmatter fallback", func(t *testing.T) {
		p := writeSkillMD(t, "# Title\n\nDescription paragraph.\n")
		got := skillDesc(p)
		if got != "Description paragraph." {
			t.Errorf("got %q, want %q", got, "Description paragraph.")
		}
	})

	t.Run("missing file", func(t *testing.T) {
		got := skillDesc("/nonexistent/path/SKILL.md")
		if got != "" {
			t.Errorf("expected empty string for missing file, got %q", got)
		}
	})

	t.Run("empty file", func(t *testing.T) {
		p := writeSkillMD(t, "")
		got := skillDesc(p)
		if got != "" {
			t.Errorf("expected empty string for empty file, got %q", got)
		}
	})
}

func TestParseDuration(t *testing.T) {
	tests := []struct {
		input   string
		want    time.Duration
		wantErr bool
	}{
		{"30s", 30 * time.Second, false},
		{"5m", 5 * time.Minute, false},
		{"1h", time.Hour, false},
		{"1h30m", 90 * time.Minute, false},
		{"1d", 24 * time.Hour, false},
		{"30d", 30 * 24 * time.Hour, false},
		{"invalid", 0, true},
		{"1.5d", 0, true},  // non-integer days
		{"", 0, true},
	}
	for _, tt := range tests {
		got, err := parseDuration(tt.input)
		if tt.wantErr {
			if err == nil {
				t.Errorf("parseDuration(%q) = %v, want error", tt.input, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseDuration(%q) error: %v", tt.input, err)
			continue
		}
		if got != tt.want {
			t.Errorf("parseDuration(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestInferPrefix(t *testing.T) {
	tests := []struct {
		repo string
		want string
	}{
		{"github.com/Org/MyRepo", "my"},
		{"github.com/Org/cistern", "ci"},
		{"github.com/Org/ABCTool", "ab"},
		{"NoSlash", "no"},
		{"ab", "ab"},   // len == 2 → returned as-is
		{"a", "a"},     // len == 1 → returned as-is
		{"", "ct"},     // empty → default "ct"
		{"github.com/Org/", "ct"}, // trailing slash → empty last segment
	}
	for _, tt := range tests {
		got := inferPrefix(tt.repo)
		if got != tt.want {
			t.Errorf("inferPrefix(%q) = %q, want %q", tt.repo, got, tt.want)
		}
	}
}

func TestPrintDropletListTerminal(t *testing.T) {
	// Fixture helpers.
	newDroplet := func(id, title, status, cataractae string) *cistern.Droplet {
		return &cistern.Droplet{
			ID:                id,
			Title:             title,
			Status:            status,
			CurrentCataractae: cataractae,
			Complexity:        2,
			UpdatedAt:         time.Now(),
		}
	}

	t.Run("empty lists print only header", func(t *testing.T) {
		out := captureStdout(t, func() {
			printDropletListTerminal(nil, nil, false, 30)
		})
		// Header must contain all column labels.
		for _, col := range []string{"ID", "COMPLEXITY", "TITLE", "STATUS", "ELAPSED", "CATARACTA"} {
			if !strings.Contains(out, col) {
				t.Errorf("header missing column %q:\n%s", col, out)
			}
		}
		// Only one line (the header).
		lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
		if len(lines) != 1 {
			t.Errorf("expected 1 line (header only), got %d:\n%s", len(lines), out)
		}
	})

	t.Run("active droplet appears in output", func(t *testing.T) {
		d := newDroplet("ts-abc12", "My Test Feature", "open", "implement")
		out := captureStdout(t, func() {
			printDropletListTerminal([]*cistern.Droplet{d}, nil, false, 30)
		})
		if !strings.Contains(out, "ts-abc12") {
			t.Errorf("expected droplet ID in output:\n%s", out)
		}
		if !strings.Contains(out, "My Test Feature") {
			t.Errorf("expected droplet title in output:\n%s", out)
		}
		if !strings.Contains(out, "implement") {
			t.Errorf("expected cataractae in output:\n%s", out)
		}
	})

	t.Run("empty cataractae shows em-dash", func(t *testing.T) {
		d := newDroplet("ts-xyz99", "No Gate Yet", "open", "")
		out := captureStdout(t, func() {
			printDropletListTerminal([]*cistern.Droplet{d}, nil, false, 30)
		})
		if !strings.Contains(out, "—") {
			t.Errorf("expected em-dash for empty cataractae:\n%s", out)
		}
	})

	t.Run("showAll=false hides delivered section", func(t *testing.T) {
		d := newDroplet("ts-del01", "Done Feature", "closed", "")
		out := captureStdout(t, func() {
			printDropletListTerminal(nil, []*cistern.Droplet{d}, false, 30)
		})
		if strings.Contains(out, "ts-del01") {
			t.Errorf("dimmed droplet should not appear when showAll=false:\n%s", out)
		}
	})

	t.Run("showAll=true shows delivered section with separator", func(t *testing.T) {
		d := newDroplet("ts-del02", "Done Feature Two", "closed", "")
		out := captureStdout(t, func() {
			printDropletListTerminal(nil, []*cistern.Droplet{d}, true, 30)
		})
		if !strings.Contains(out, "ts-del02") {
			t.Errorf("expected dimmed droplet in output when showAll=true:\n%s", out)
		}
		if !strings.Contains(out, "delivered") {
			t.Errorf("expected 'delivered' separator when showAll=true:\n%s", out)
		}
	})

	t.Run("no panic on nil slices", func(t *testing.T) {
		// Should not panic.
		captureStdout(t, func() {
			printDropletListTerminal(nil, nil, true, 20)
		})
	})

	t.Run("title truncated to titleMax", func(t *testing.T) {
		long := strings.Repeat("A", 80)
		d := newDroplet("ts-trunc", long, "open", "")
		out := captureStdout(t, func() {
			printDropletListTerminal([]*cistern.Droplet{d}, nil, false, 20)
		})
		// The full 80-char title should not appear verbatim.
		if strings.Contains(out, long) {
			t.Errorf("expected title to be truncated but found full title in output")
		}
	})
}
