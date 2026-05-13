// Package cataractae provides sandbox and context management for workflow steps.
//
// This file implements extraction and loading of repo contributing guidelines.
// For fork-mode repos, AGENTS.md, CONTRIBUTING.md, and .github/CONTRIBUTING.md
// are extracted from the primary clone and stored per-repo on disk so they can
// be injected into CONTEXT.md for all cataractae.
package cataractae

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// RepoGuideline holds a single extracted guideline file for a repository.
type RepoGuideline struct {
	Filename string // e.g., "AGENTS.md", "CONTRIBUTING.md"
	Content  string // full file content
}

// candidateGuidelines lists the filenames to extract from a repo's primary
// clone directory, checked in order. .github/CONTRIBUTING.md is resolved
// relative to primaryDir. The storage name for each candidate is derived by
// replacing path separators with dashes so that root CONTRIBUTING.md and
// .github/CONTRIBUTING.md do not clobber each other.
var candidateGuidelines = []string{"AGENTS.md", "CONTRIBUTING.md", ".github/CONTRIBUTING.md"}

// storageName converts a candidate path to a flat filename for storage.
// Root-level files keep their base name; subdirectory files use the path
// with separators replaced by dashes (e.g., ".github/CONTRIBUTING.md"
// becomes "github-CONTRIBUTING.md"). Leading dots are stripped so files
// are not hidden on disk.
func storageName(candidate string) string {
	dir, base := filepath.Split(candidate)
	if dir == "" {
		return base
	}
	// ".github/CONTRIBUTING.md" → "github-CONTRIBUTING.md"
	cleanDir := strings.TrimPrefix(dir, ".")
	cleanDir = strings.TrimSuffix(cleanDir, "/")
	return cleanDir + "-" + base
}

// guidelinesDirFn returns the path to the guidelines storage directory for a repo.
// Overridable in tests (matches the cataractaeDirFn pattern).
var guidelinesDirFn = func(repoName string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("guidelines: home dir: %w", err)
	}
	dir := filepath.Join(home, ".cistern", "repos", repoName, "guidelines")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("guidelines: mkdir %s: %w", dir, err)
	}
	return dir, nil
}

// guidelinesPath returns the absolute path to a stored guideline file for a repo.
// Never returns an error — home directory resolution failure causes a panic,
// matching the skills.SkillsDir() pattern.
func guidelinesPath(repoName, filename string) string {
	dir, err := guidelinesDirFn(repoName)
	if err != nil {
		panic(fmt.Sprintf("guidelines: cannot resolve path: %v", err))
	}
	return filepath.Join(dir, filename)
}

// LoadGuidelines reads all stored guideline files for a repo from
// ~/.cistern/repos/<repoName>/guidelines/. Returns nil (not []RepoGuideline{})
// when no guidelines directory exists or the directory is empty. Returns a
// non-nil error only for filesystem failures (permission denied on an existing
// directory). Results are sorted by filename for deterministic output.
func LoadGuidelines(repoName string) ([]RepoGuideline, error) {
	dir, err := guidelinesDirFn(repoName)
	if err != nil {
		return nil, fmt.Errorf("guidelines: load %s: %w", repoName, err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("guidelines: read dir %s: %w", dir, err)
	}

	var guidelines []RepoGuideline
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, fmt.Errorf("guidelines: read %s: %w", e.Name(), err)
		}
		guidelines = append(guidelines, RepoGuideline{
			Filename: e.Name(),
			Content:  string(data),
		})
	}

	sort.Slice(guidelines, func(i, j int) bool {
		return guidelines[i].Filename < guidelines[j].Filename
	})

	return guidelines, nil
}

// ExtractGuidelines reads candidate guideline files from the primary clone
// directory and stores them to ~/.cistern/repos/<repoName>/guidelines/.
// Returns nil on success. Returns a non-nil error only for filesystem failures
// on directory creation. Individual file read failures are logged as warnings
// but do not fail the entire operation — extraction is best-effort.
func ExtractGuidelines(primaryDir, repoName string) error {
	dir, err := guidelinesDirFn(repoName)
	if err != nil {
		return fmt.Errorf("guidelines: extract %s: %w", repoName, err)
	}

	for _, candidate := range candidateGuidelines {
		srcPath := filepath.Join(primaryDir, candidate)
		data, err := os.ReadFile(srcPath)
		if err != nil {
			if !os.IsNotExist(err) {
				slog.Default().Warn("guidelines: could not read candidate", "repo", repoName, "file", candidate, "error", err)
			}
			continue
		}

		// Use storageName to avoid clobbering: root CONTRIBUTING.md and
		// .github/CONTRIBUTING.md are stored as distinct files.
		sName := storageName(candidate)
		destPath := filepath.Join(dir, sName)
		if err := os.WriteFile(destPath, data, 0o644); err != nil {
			return fmt.Errorf("guidelines: write %s: %w", destPath, err)
		}
		slog.Default().Info("guidelines: extracted", "repo", repoName, "file", sName)
	}

	return nil
}