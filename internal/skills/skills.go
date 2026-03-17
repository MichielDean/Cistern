// Package skills downloads and caches AgentSkills SKILL.md files locally.
package skills

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

// CachePath returns the path where a skill's SKILL.md is (or would be) cached.
func CachePath(name string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", ".cistern", "skills", name, "SKILL.md")
	}
	return filepath.Join(home, ".cistern", "skills", name, "SKILL.md")
}

// Install downloads a skill's SKILL.md from url and caches it at CachePath(name).
// Idempotent: if the file already exists it is not re-downloaded.
func Install(name, url string) error {
	dest := CachePath(name)
	if _, err := os.Stat(dest); err == nil {
		return nil // already cached
	}
	return download(dest, url)
}

// ForceUpdate re-fetches the skill from url regardless of whether it is cached.
func ForceUpdate(name, url string) error {
	return download(CachePath(name), url)
}

func download(dest, url string) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return fmt.Errorf("skills: mkdir %s: %w", filepath.Dir(dest), err)
	}

	resp, err := http.Get(url) //nolint:gosec -- URL comes from trusted workflow config
	if err != nil {
		return fmt.Errorf("skills: fetch %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("skills: fetch %s: HTTP %d", url, resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("skills: read body from %s: %w", url, err)
	}

	if err := os.WriteFile(dest, data, 0o644); err != nil {
		return fmt.Errorf("skills: write %s: %w", dest, err)
	}
	return nil
}
