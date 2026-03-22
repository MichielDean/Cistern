package skills

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestCachePath_UsesHomeDir(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	got := LocalPath("example")
	want := filepath.Join(tmp, ".cistern", "skills", "example", "SKILL.md")
	if got != want {
		t.Errorf("CachePath = %q, want %q", got, want)
	}
}

func TestInstall_DownloadsAndCaches(t *testing.T) {
	const content = "# My Skill\n\nThis skill does awesome things.\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(content)) //nolint:errcheck
	}))
	defer srv.Close()

	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	if err := Install("my-skill", srv.URL+"/SKILL.md"); err != nil {
		t.Fatalf("Install: %v", err)
	}

	data, err := os.ReadFile(LocalPath("my-skill"))
	if err != nil {
		t.Fatalf("cached file not found: %v", err)
	}
	if string(data) != content {
		t.Errorf("cached content = %q, want %q", string(data), content)
	}
}

func TestInstall_Idempotent(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Write([]byte("# Skill\n")) //nolint:errcheck
	}))
	defer srv.Close()

	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	if err := Install("idempotent-skill", srv.URL+"/SKILL.md"); err != nil {
		t.Fatalf("first Install: %v", err)
	}
	if err := Install("idempotent-skill", srv.URL+"/SKILL.md"); err != nil {
		t.Fatalf("second Install: %v", err)
	}

	if callCount != 1 {
		t.Errorf("HTTP server called %d times, want 1 (idempotent)", callCount)
	}
}

func TestInstall_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	err := Install("missing-skill", srv.URL+"/SKILL.md")
	if err == nil {
		t.Fatal("expected error for HTTP 404, got nil")
	}
}

func TestInstall_PathTraversal(t *testing.T) {
	err := Install("../../evil", "http://example.com/SKILL.md")
	if err == nil {
		t.Fatal("expected error for path-traversal skill name, got nil")
	}
}

func TestForceUpdate_PathTraversal(t *testing.T) {
	err := Update("../escape", "http://example.com/SKILL.md")
	if err == nil {
		t.Fatal("expected error for path-traversal skill name, got nil")
	}
}

func TestInstall_DownloadExceedsMaxSize(t *testing.T) {
	// Serve a body larger than the 1 MiB cap; Install must return an error.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(make([]byte, maxSkillSize+1)) //nolint:errcheck
	}))
	defer srv.Close()

	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	err := Install("oversized-skill", srv.URL+"/SKILL.md")
	if err == nil {
		t.Fatal("expected error for response exceeding max size, got nil")
	}
}

func TestInstall_Update(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Write([]byte("# Updated Skill\n")) //nolint:errcheck
	}))
	defer srv.Close()

	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	// Normal install caches once.
	if err := Install("force-skill", srv.URL+"/SKILL.md"); err != nil {
		t.Fatalf("Install: %v", err)
	}
	// ForceUpdate re-fetches even though cached.
	if err := Update("force-skill", srv.URL+"/SKILL.md"); err != nil {
		t.Fatalf("ForceUpdate: %v", err)
	}

	if callCount != 2 {
		t.Errorf("HTTP server called %d times, want 2 (force re-fetch)", callCount)
	}
}

// --- Deploy tests ---

func TestDeploy_WritesContentToLocalPath(t *testing.T) {
	// Given: no skill installed.
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	content := []byte("# Deployed Skill\nContent from git.\n")

	// When: Deploy is called with skill content.
	changed, err := Deploy("git-skill", content)

	// Then: file written, changed=true, no error.
	if err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	if !changed {
		t.Error("expected changed=true for new skill")
	}
	data, err := os.ReadFile(LocalPath("git-skill"))
	if err != nil {
		t.Fatalf("skill file not created: %v", err)
	}
	if string(data) != string(content) {
		t.Errorf("content = %q, want %q", string(data), string(content))
	}
}

func TestDeploy_UpdatesManifestWithLocalSource(t *testing.T) {
	// Given: no skill installed.
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	// When: Deploy is called.
	if _, err := Deploy("manifest-skill", []byte("# Skill\n")); err != nil {
		t.Fatalf("Deploy: %v", err)
	}

	// Then: manifest records the skill with source_url "local".
	entries, err := ListInstalled()
	if err != nil {
		t.Fatalf("ListInstalled: %v", err)
	}
	var found bool
	for _, e := range entries {
		if e.Name == "manifest-skill" {
			found = true
			if e.SourceURL != "local" {
				t.Errorf("source_url = %q, want %q", e.SourceURL, "local")
			}
		}
	}
	if !found {
		t.Error("skill not found in manifest after Deploy")
	}
}

func TestDeploy_NoOpWhenContentUnchanged(t *testing.T) {
	// Given: skill already deployed with same content.
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	content := []byte("# Skill\nSame content.\n")
	if _, err := Deploy("noop-skill", content); err != nil {
		t.Fatalf("first Deploy: %v", err)
	}

	// When: Deploy called again with identical content.
	changed, err := Deploy("noop-skill", content)

	// Then: no-op (changed=false, no error).
	if err != nil {
		t.Fatalf("second Deploy: %v", err)
	}
	if changed {
		t.Error("expected changed=false when content is identical")
	}
}

func TestDeploy_WritesNewContentWhenChanged(t *testing.T) {
	// Given: skill deployed with v1 content.
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	v1 := []byte("# Skill v1\n")
	v2 := []byte("# Skill v2\n")
	if _, err := Deploy("update-skill", v1); err != nil {
		t.Fatalf("Deploy v1: %v", err)
	}

	// When: Deploy called with new content.
	changed, err := Deploy("update-skill", v2)

	// Then: file updated, changed=true.
	if err != nil {
		t.Fatalf("Deploy v2: %v", err)
	}
	if !changed {
		t.Error("expected changed=true when content differs")
	}
	data, err := os.ReadFile(LocalPath("update-skill"))
	if err != nil {
		t.Fatalf("read skill file: %v", err)
	}
	if string(data) != string(v2) {
		t.Errorf("content = %q, want %q", string(data), string(v2))
	}
}

func TestDeploy_RejectsInvalidName(t *testing.T) {
	// Given: an invalid skill name with path traversal.
	// When: Deploy is called.
	_, err := Deploy("../../evil", []byte("bad"))

	// Then: error returned.
	if err == nil {
		t.Fatal("expected error for invalid skill name, got nil")
	}
}

// --- IsInstalled tests ---

func TestIsInstalled_TrueForInstalledSkill(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	dest := LocalPath("existing-skill")
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(dest, []byte("# Skill\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	if !IsInstalled("existing-skill") {
		t.Error("expected IsInstalled=true for installed skill")
	}
}

func TestIsInstalled_FalseForMissingSkill(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	if IsInstalled("nonexistent-skill") {
		t.Error("expected IsInstalled=false for missing skill")
	}
}

// --- ListInstalled tests ---

func TestListInstalled_EmptyStore(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	entries, err := ListInstalled()
	if err != nil {
		t.Fatalf("ListInstalled: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(entries))
	}
}

func TestListInstalled_SingleSkill(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	if err := saveManifestEntry(ManifestEntry{Name: "solo", SourceURL: "http://example.com/solo"}); err != nil {
		t.Fatalf("saveManifestEntry: %v", err)
	}

	entries, err := ListInstalled()
	if err != nil {
		t.Fatalf("ListInstalled: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Name != "solo" {
		t.Errorf("entry name = %q, want %q", entries[0].Name, "solo")
	}
}

func TestListInstalled_MultipleSkills(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	names := []string{"alpha", "beta", "gamma"}
	for _, name := range names {
		if err := saveManifestEntry(ManifestEntry{Name: name, SourceURL: "http://example.com/" + name}); err != nil {
			t.Fatalf("saveManifestEntry(%s): %v", name, err)
		}
	}

	entries, err := ListInstalled()
	if err != nil {
		t.Fatalf("ListInstalled: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}
}

// --- Remove tests ---

func TestRemove_RemovesDirAndManifest(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	dest := LocalPath("rm-skill")
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(dest, []byte("# Skill\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := saveManifestEntry(ManifestEntry{Name: "rm-skill", SourceURL: "http://example.com"}); err != nil {
		t.Fatalf("saveManifestEntry: %v", err)
	}

	if err := Remove("rm-skill"); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	if IsInstalled("rm-skill") {
		t.Error("expected IsInstalled=false after Remove")
	}
	entries, err := ListInstalled()
	if err != nil {
		t.Fatalf("ListInstalled: %v", err)
	}
	for _, e := range entries {
		if e.Name == "rm-skill" {
			t.Error("skill still present in manifest after Remove")
		}
	}
}

func TestRemove_NonExistentIsNoOp(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	if err := Remove("ghost-skill"); err != nil {
		t.Errorf("Remove of non-existent skill: expected no error, got %v", err)
	}
}

func TestRemove_RejectsInvalidName(t *testing.T) {
	err := Remove("../../evil")
	if err == nil {
		t.Fatal("expected error for invalid skill name, got nil")
	}
}

// --- removeManifestEntry tests ---

func TestRemoveManifestEntry_RemovesEntry(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	if err := saveManifestEntry(ManifestEntry{Name: "to-remove", SourceURL: "http://example.com/remove"}); err != nil {
		t.Fatalf("saveManifestEntry: %v", err)
	}
	if err := saveManifestEntry(ManifestEntry{Name: "keep", SourceURL: "http://example.com/keep"}); err != nil {
		t.Fatalf("saveManifestEntry: %v", err)
	}

	if err := removeManifestEntry("to-remove"); err != nil {
		t.Fatalf("removeManifestEntry: %v", err)
	}

	entries, err := ListInstalled()
	if err != nil {
		t.Fatalf("ListInstalled: %v", err)
	}
	for _, e := range entries {
		if e.Name == "to-remove" {
			t.Error("entry still present after removeManifestEntry")
		}
	}
	if len(entries) != 1 || entries[0].Name != "keep" {
		t.Errorf("unexpected entries after remove: %v", entries)
	}
}

func TestRemoveManifestEntry_HandlesMissingEntry(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	// No manifest exists — removing a nonexistent entry should not error.
	if err := removeManifestEntry("nonexistent"); err != nil {
		t.Errorf("expected no error for missing entry, got %v", err)
	}
}

func TestRemoveManifestEntry_PersistsCorrectly(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	for _, name := range []string{"first", "second", "third"} {
		if err := saveManifestEntry(ManifestEntry{Name: name, SourceURL: "http://example.com/" + name}); err != nil {
			t.Fatalf("saveManifestEntry(%s): %v", name, err)
		}
	}

	if err := removeManifestEntry("second"); err != nil {
		t.Fatalf("removeManifestEntry: %v", err)
	}

	entries, err := ListInstalled()
	if err != nil {
		t.Fatalf("ListInstalled: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries after remove, got %d", len(entries))
	}
	for _, e := range entries {
		if e.Name == "second" {
			t.Error("removed entry still present in manifest")
		}
	}
}
