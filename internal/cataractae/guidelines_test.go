package cataractae

import (
	"os"
	"path/filepath"
	"testing"
)

// TestExtractGuidelines_AGENTSMD_Exists verifies that when AGENTS.md exists
// in the primary dir, it is stored to the guidelines directory.
func TestExtractGuidelines_AGENTSMD_Exists(t *testing.T) {
	primaryDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(primaryDir, "AGENTS.md"), []byte("# Conventions\nUse Kotlin style"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Override guidelinesDir to use temp dir.
	guidelinesDir := filepath.Join(t.TempDir(), "testrepo", "guidelines")
	origDirFn := guidelinesDirFn
	guidelinesDirFn = func(repoName string) (string, error) {
		if err := os.MkdirAll(guidelinesDir, 0o755); err != nil {
			return "", err
		}
		return guidelinesDir, nil
	}
	t.Cleanup(func() { guidelinesDirFn = origDirFn })

	if err := ExtractGuidelines(primaryDir, "testrepo"); err != nil {
		t.Fatalf("ExtractGuidelines: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(guidelinesDir, "AGENTS.md"))
	if err != nil {
		t.Fatalf("AGENTS.md not found in guidelines dir: %v", err)
	}
	if string(data) != "# Conventions\nUse Kotlin style" {
		t.Errorf("AGENTS.md content mismatch: got %q", string(data))
	}
}

// TestExtractGuidelines_CONTRIBUTINGMD_Exists verifies that CONTRIBUTING.md
// in the primary dir root is stored.
func TestExtractGuidelines_CONTRIBUTINGMD_Exists(t *testing.T) {
	primaryDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(primaryDir, "CONTRIBUTING.md"), []byte("# Contributing\nPlease write tests"), 0o644); err != nil {
		t.Fatal(err)
	}

	guidelinesDir := filepath.Join(t.TempDir(), "testrepo2", "guidelines")
	origDirFn := guidelinesDirFn
	guidelinesDirFn = func(repoName string) (string, error) {
		if err := os.MkdirAll(guidelinesDir, 0o755); err != nil {
			return "", err
		}
		return guidelinesDir, nil
	}
	t.Cleanup(func() { guidelinesDirFn = origDirFn })

	if err := ExtractGuidelines(primaryDir, "testrepo2"); err != nil {
		t.Fatalf("ExtractGuidelines: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(guidelinesDir, "CONTRIBUTING.md"))
	if err != nil {
		t.Fatalf("CONTRIBUTING.md not found in guidelines dir: %v", err)
	}
	if string(data) != "# Contributing\nPlease write tests" {
		t.Errorf("CONTRIBUTING.md content mismatch: got %q", string(data))
	}
}

// TestExtractGuidelines_GithubContributing verifies that .github/CONTRIBUTING.md
// is extracted and stored as "github-CONTRIBUTING.md" (with path prefix flattened).
func TestExtractGuidelines_GithubContributing(t *testing.T) {
	primaryDir := t.TempDir()
	githubDir := filepath.Join(primaryDir, ".github")
	if err := os.MkdirAll(githubDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(githubDir, "CONTRIBUTING.md"), []byte("# GitHub Contributing\nFork and PR"), 0o644); err != nil {
		t.Fatal(err)
	}

	guidelinesDir := filepath.Join(t.TempDir(), "testrepo3", "guidelines")
	origDirFn := guidelinesDirFn
	guidelinesDirFn = func(repoName string) (string, error) {
		if err := os.MkdirAll(guidelinesDir, 0o755); err != nil {
			return "", err
		}
		return guidelinesDir, nil
	}
	t.Cleanup(func() { guidelinesDirFn = origDirFn })

	if err := ExtractGuidelines(primaryDir, "testrepo3"); err != nil {
		t.Fatalf("ExtractGuidelines: %v", err)
	}

	// .github/CONTRIBUTING.md is stored as "github-CONTRIBUTING.md"
	data, err := os.ReadFile(filepath.Join(guidelinesDir, "github-CONTRIBUTING.md"))
	if err != nil {
		t.Fatalf("github-CONTRIBUTING.md from .github not found: %v", err)
	}
	if string(data) != "# GitHub Contributing\nFork and PR" {
		t.Errorf("github-CONTRIBUTING.md content mismatch: got %q", string(data))
	}
}

// TestExtractGuidelines_NoFiles verifies that when no guideline files exist
// in the primary dir, ExtractGuidelines returns nil error and no files are stored.
func TestExtractGuidelines_NoFiles(t *testing.T) {
	primaryDir := t.TempDir()

	guidelinesDir := filepath.Join(t.TempDir(), "emptyrepo", "guidelines")
	origDirFn := guidelinesDirFn
	guidelinesDirFn = func(repoName string) (string, error) {
		if err := os.MkdirAll(guidelinesDir, 0o755); err != nil {
			return "", err
		}
		return guidelinesDir, nil
	}
	t.Cleanup(func() { guidelinesDirFn = origDirFn })

	if err := ExtractGuidelines(primaryDir, "emptyrepo"); err != nil {
		t.Fatalf("ExtractGuidelines returned error for no files: %v", err)
	}

	entries, err := os.ReadDir(guidelinesDir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected no files in guidelines dir, got %d", len(entries))
	}
}

// TestExtractGuidelines_PartialFiles verifies that when only AGENTS.md exists
// (no CONTRIBUTING.md), only AGENTS.md is stored without error.
func TestExtractGuidelines_PartialFiles(t *testing.T) {
	primaryDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(primaryDir, "AGENTS.md"), []byte("only this one"), 0o644); err != nil {
		t.Fatal(err)
	}

	guidelinesDir := filepath.Join(t.TempDir(), "partialrepo", "guidelines")
	origDirFn := guidelinesDirFn
	guidelinesDirFn = func(repoName string) (string, error) {
		if err := os.MkdirAll(guidelinesDir, 0o755); err != nil {
			return "", err
		}
		return guidelinesDir, nil
	}
	t.Cleanup(func() { guidelinesDirFn = origDirFn })

	if err := ExtractGuidelines(primaryDir, "partialrepo"); err != nil {
		t.Fatalf("ExtractGuidelines: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(guidelinesDir, "AGENTS.md"))
	if err != nil {
		t.Fatalf("AGENTS.md not found: %v", err)
	}
	if string(data) != "only this one" {
		t.Errorf("AGENTS.md content mismatch: got %q", string(data))
	}

	if _, err := os.Stat(filepath.Join(guidelinesDir, "CONTRIBUTING.md")); !os.IsNotExist(err) {
		t.Error("CONTRIBUTING.md should not exist when only AGENTS.md was in the repo")
	}
}

// TestLoadGuidelines_ForkRepo verifies that stored guideline files are loaded
// and returned sorted by filename.
func TestLoadGuidelines_ForkRepo(t *testing.T) {
	guidelinesDir := filepath.Join(t.TempDir(), "loadrepo", "guidelines")
	origDirFn := guidelinesDirFn
	guidelinesDirFn = func(repoName string) (string, error) {
		if err := os.MkdirAll(guidelinesDir, 0o755); err != nil {
			return "", err
		}
		return guidelinesDir, nil
	}
	t.Cleanup(func() { guidelinesDirFn = origDirFn })

	if err := os.MkdirAll(guidelinesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(guidelinesDir, "CONTRIBUTING.md"), []byte("C file"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(guidelinesDir, "AGENTS.md"), []byte("A file"), 0o644); err != nil {
		t.Fatal(err)
	}

	guidelines, err := LoadGuidelines("loadrepo")
	if err != nil {
		t.Fatalf("LoadGuidelines: %v", err)
	}
	if len(guidelines) != 2 {
		t.Fatalf("expected 2 guidelines, got %d", len(guidelines))
	}
	// Must be sorted: AGENTS.md before CONTRIBUTING.md
	if guidelines[0].Filename != "AGENTS.md" {
		t.Errorf("first guideline should be AGENTS.md, got %s", guidelines[0].Filename)
	}
	if guidelines[1].Filename != "CONTRIBUTING.md" {
		t.Errorf("second guideline should be CONTRIBUTING.md, got %s", guidelines[1].Filename)
	}
	if guidelines[0].Content != "A file" {
		t.Errorf("AGENTS.md content mismatch: got %q", guidelines[0].Content)
	}
	if guidelines[1].Content != "C file" {
		t.Errorf("CONTRIBUTING.md content mismatch: got %q", guidelines[1].Content)
	}
}

// TestLoadGuidelines_NoDirectory verifies that when no guidelines directory
// exists, LoadGuidelines returns nil without error.
func TestLoadGuidelines_NoDirectory(t *testing.T) {
	origDirFn := guidelinesDirFn
	guidelinesDirFn = func(repoName string) (string, error) {
		// Return a path that does not exist and don't create it.
		return filepath.Join(os.TempDir(), "nonexistent-guidelines-"+repoName), nil
	}
	t.Cleanup(func() { guidelinesDirFn = origDirFn })

	guidelines, err := LoadGuidelines("norepo")
	if err != nil {
		t.Fatalf("LoadGuidelines returned error for nonexistent dir: %v", err)
	}
	if guidelines != nil {
		t.Errorf("expected nil guidelines for nonexistent directory, got %d items", len(guidelines))
	}
}

// TestLoadGuidelines_EmptyDirectory verifies that an empty guidelines directory
// returns nil without error.
func TestLoadGuidelines_EmptyDirectory(t *testing.T) {
	guidelinesDir := filepath.Join(t.TempDir(), "emptyreposts", "guidelines")
	origDirFn := guidelinesDirFn
	guidelinesDirFn = func(repoName string) (string, error) {
		if err := os.MkdirAll(guidelinesDir, 0o755); err != nil {
			return "", err
		}
		return guidelinesDir, nil
	}
	t.Cleanup(func() { guidelinesDirFn = origDirFn })

	guidelines, err := LoadGuidelines("emptyreposts")
	if err != nil {
		t.Fatalf("LoadGuidelines: %v", err)
	}
	if guidelines != nil {
		t.Errorf("expected nil guidelines for empty directory, got %d items", len(guidelines))
	}
}

// TestGuidelinesPath verifies that guidelinesPath returns the expected path.
func TestGuidelinesPath(t *testing.T) {
	// Override to a deterministic path.
	origDirFn := guidelinesDirFn
	testDir := filepath.Join(t.TempDir(), "myrepo", "guidelines")
	guidelinesDirFn = func(repoName string) (string, error) {
		return testDir, nil
	}
	t.Cleanup(func() { guidelinesDirFn = origDirFn })

	got := guidelinesPath("myrepo", "AGENTS.md")
	expected := filepath.Join(testDir, "AGENTS.md")
	if got != expected {
		t.Errorf("guidelinesPath(%q, %q) = %q, want %q", "myrepo", "AGENTS.md", got, expected)
	}
}

// TestExtractGuidelines_GithubContributing_DoesNotClobberRoot verifies that
// when both root CONTRIBUTING.md and .github/CONTRIBUTING.md exist, both are
// stored — the .github version as "github-CONTRIBUTING.md" so they don't clobber
// each other.
func TestExtractGuidelines_GithubContributing_DoesNotClobberRoot(t *testing.T) {
	primaryDir := t.TempDir()

	// Create both root CONTRIBUTING.md and .github/CONTRIBUTING.md
	if err := os.WriteFile(filepath.Join(primaryDir, "CONTRIBUTING.md"), []byte("root contributing"), 0o644); err != nil {
		t.Fatal(err)
	}
	githubDir := filepath.Join(primaryDir, ".github")
	if err := os.MkdirAll(githubDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(githubDir, "CONTRIBUTING.md"), []byte("github contributing"), 0o644); err != nil {
		t.Fatal(err)
	}

	guidelinesDir := filepath.Join(t.TempDir(), "clobberrepo", "guidelines")
	origDirFn := guidelinesDirFn
	guidelinesDirFn = func(repoName string) (string, error) {
		if err := os.MkdirAll(guidelinesDir, 0o755); err != nil {
			return "", err
		}
		return guidelinesDir, nil
	}
	t.Cleanup(func() { guidelinesDirFn = origDirFn })

	if err := ExtractGuidelines(primaryDir, "clobberrepo"); err != nil {
		t.Fatalf("ExtractGuidelines: %v", err)
	}

	// Root version stored as CONTRIBUTING.md
	rootData, err := os.ReadFile(filepath.Join(guidelinesDir, "CONTRIBUTING.md"))
	if err != nil {
		t.Fatalf("CONTRIBUTING.md not found: %v", err)
	}
	if string(rootData) != "root contributing" {
		t.Errorf("root CONTRIBUTING.md content mismatch: got %q, want %q", string(rootData), "root contributing")
	}

	// .github version stored as github-CONTRIBUTING.md (not clobbered)
	ghData, err := os.ReadFile(filepath.Join(guidelinesDir, "github-CONTRIBUTING.md"))
	if err != nil {
		t.Fatalf("github-CONTRIBUTING.md not found: %v", err)
	}
	if string(ghData) != "github contributing" {
		t.Errorf("github-CONTRIBUTING.md content mismatch: got %q, want %q", string(ghData), "github contributing")
	}
}

// TestStorageName verifies that candidate paths are converted to flat, non-hidden
// filenames that avoid clobbering.
func TestStorageName(t *testing.T) {
	tests := []struct {
		candidate string
		want      string
	}{
		{"AGENTS.md", "AGENTS.md"},
		{"CONTRIBUTING.md", "CONTRIBUTING.md"},
		{".github/CONTRIBUTING.md", "github-CONTRIBUTING.md"},
	}
	for _, tt := range tests {
		got := storageName(tt.candidate)
		if got != tt.want {
			t.Errorf("storageName(%q) = %q, want %q", tt.candidate, got, tt.want)
		}
	}
}

// TestLoadGuidelines_WithAllThreeFiles verifies that LoadGuidelines correctly
// loads all three guideline files, including the github- prefixed variant.
func TestLoadGuidelines_WithAllThreeFiles(t *testing.T) {
	guidelinesDir := filepath.Join(t.TempDir(), "allfiles", "guidelines")
	origDirFn := guidelinesDirFn
	guidelinesDirFn = func(repoName string) (string, error) {
		if err := os.MkdirAll(guidelinesDir, 0o755); err != nil {
			return "", err
		}
		return guidelinesDir, nil
	}
	t.Cleanup(func() { guidelinesDirFn = origDirFn })

	if err := os.MkdirAll(guidelinesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(guidelinesDir, "AGENTS.md"), []byte("agents content"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(guidelinesDir, "CONTRIBUTING.md"), []byte("root contributing"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(guidelinesDir, "github-CONTRIBUTING.md"), []byte("github contributing"), 0o644); err != nil {
		t.Fatal(err)
	}

	guidelines, err := LoadGuidelines("allfiles")
	if err != nil {
		t.Fatalf("LoadGuidelines: %v", err)
	}
	if len(guidelines) != 3 {
		t.Fatalf("expected 3 guidelines, got %d", len(guidelines))
	}
	// Sorted: AGENTS.md < CONTRIBUTING.md < github-CONTRIBUTING.md
	if guidelines[0].Filename != "AGENTS.md" {
		t.Errorf("first should be AGENTS.md, got %s", guidelines[0].Filename)
	}
	if guidelines[1].Filename != "CONTRIBUTING.md" {
		t.Errorf("second should be CONTRIBUTING.md, got %s", guidelines[1].Filename)
	}
	if guidelines[2].Filename != "github-CONTRIBUTING.md" {
		t.Errorf("third should be github-CONTRIBUTING.md, got %s", guidelines[2].Filename)
	}
}

// TestExtractGuidelines_ClearsStaleFiles verifies that ExtractGuidelines removes
// stale guideline files from a previous extraction when those files no longer
// exist in the primary clone directory. This prevents outdated conventions from
// being injected into CONTEXT.md.
func TestExtractGuidelines_ClearsStaleFiles(t *testing.T) {
	guidelinesDir := filepath.Join(t.TempDir(), "stalerepo", "guidelines")
	origDirFn := guidelinesDirFn
	guidelinesDirFn = func(repoName string) (string, error) {
		if err := os.MkdirAll(guidelinesDir, 0o755); err != nil {
			return "", err
		}
		return guidelinesDir, nil
	}
	t.Cleanup(func() { guidelinesDirFn = origDirFn })

	// Pre-populate the guidelines directory with a stale CONTRIBUTING.md
	// that no longer exists in the upstream repo.
	if err := os.MkdirAll(guidelinesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(guidelinesDir, "CONTRIBUTING.md"), []byte("stale content"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Primary dir only has AGENTS.md — no CONTRIBUTING.md.
	primaryDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(primaryDir, "AGENTS.md"), []byte("fresh agents"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := ExtractGuidelines(primaryDir, "stalerepo"); err != nil {
		t.Fatalf("ExtractGuidelines: %v", err)
	}

	// The stale CONTRIBUTING.md should have been removed.
	if _, err := os.Stat(filepath.Join(guidelinesDir, "CONTRIBUTING.md")); !os.IsNotExist(err) {
		t.Error("stale CONTRIBUTING.md should have been removed")
	}

	// AGENTS.md should exist with fresh content.
	data, err := os.ReadFile(filepath.Join(guidelinesDir, "AGENTS.md"))
	if err != nil {
		t.Fatalf("AGENTS.md not found: %v", err)
	}
	if string(data) != "fresh agents" {
		t.Errorf("AGENTS.md content mismatch: got %q, want %q", string(data), "fresh agents")
	}

	// Only one file should remain (AGENTS.md).
	entries, err := os.ReadDir(guidelinesDir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("expected 1 file in guidelines dir, got %d", len(entries))
	}
}