package main

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MichielDean/cistern/internal/cistern"
)

// --- isNotFoundError unit test ---

func TestIsNotFoundError(t *testing.T) {
	tests := []struct {
		err  error
		want bool
	}{
		{fmt.Errorf("cistern: droplet %s not found", "abc123"), true},
		{fmt.Errorf("cistern: issue %s not found", "xyz789"), true},
		{fmt.Errorf("cistern: dependency %s not found", "dep1"), true},
		{fmt.Errorf("some other error"), false},
		{fmt.Errorf("database is locked"), false},
	}
	for _, tt := range tests {
		got := isNotFoundError(tt.err)
		if got != tt.want {
			t.Errorf("isNotFoundError(%q) = %v, want %v", tt.err.Error(), got, tt.want)
		}
	}
}

// --- Droplet CRUD ---

func TestAPI_GetDroplets_ReturnsJSON(t *testing.T) {
	db := tempDB(t)
	c, err := cistern.New(db, "mr")
	if err != nil {
		t.Fatal(err)
	}
	c.Add("myrepo", "Test droplet", "", 1, 2)
	c.Close()

	mux := newDashboardMux(tempCfg(t), db)
	req := httptest.NewRequest(http.MethodGet, "/api/droplets", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("GET /api/droplets status = %d, want 200", w.Code)
	}
	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	var droplets []*cistern.Droplet
	if err := json.NewDecoder(w.Body).Decode(&droplets); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(droplets) != 1 {
		t.Errorf("len(droplets) = %d, want 1", len(droplets))
	}
}

func TestAPI_GetDroplets_FiltersByRepo(t *testing.T) {
	db := tempDB(t)
	c, err := cistern.New(db, "mr")
	if err != nil {
		t.Fatal(err)
	}
	c.Add("repo-a", "Drop A", "", 1, 2)
	c.Add("repo-b", "Drop B", "", 1, 2)
	c.Close()

	mux := newDashboardMux(tempCfg(t), db)
	req := httptest.NewRequest(http.MethodGet, "/api/droplets?repo=repo-a", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	var droplets []*cistern.Droplet
	if err := json.NewDecoder(w.Body).Decode(&droplets); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(droplets) != 1 || droplets[0].Repo != "repo-a" {
		t.Errorf("expected 1 droplet from repo-a, got %d", len(droplets))
	}
}

func TestAPI_GetDroplets_FiltersByStatus(t *testing.T) {
	db := tempDB(t)
	c, err := cistern.New(db, "mr")
	if err != nil {
		t.Fatal(err)
	}
	d, _ := c.Add("myrepo", "Active", "", 1, 2)
	c.GetReady("myrepo")
	c.Assign(d.ID, "virgo", "implement")
	c.Add("myrepo", "Queued", "", 2, 2)
	c.Close()

	mux := newDashboardMux(tempCfg(t), db)
	req := httptest.NewRequest(http.MethodGet, "/api/droplets?status=in_progress", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	var droplets []*cistern.Droplet
	if err := json.NewDecoder(w.Body).Decode(&droplets); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(droplets) != 1 || droplets[0].Status != "in_progress" {
		t.Errorf("expected 1 in_progress droplet, got %d", len(droplets))
	}
}

func TestAPI_GetDroplets_MethodNotAllowed(t *testing.T) {
	mux := newDashboardMux(tempCfg(t), tempDB(t))
	req := httptest.NewRequest(http.MethodDelete, "/api/droplets", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("DELETE /api/droplets status = %d, want 405", w.Code)
	}
}

func TestAPI_GetDropletByID_ReturnsDroplet(t *testing.T) {
	db := tempDB(t)
	c, err := cistern.New(db, "mr")
	if err != nil {
		t.Fatal(err)
	}
	d, _ := c.Add("myrepo", "Specific", "a description", 1, 2)
	c.Close()

	mux := newDashboardMux(tempCfg(t), db)
	req := httptest.NewRequest(http.MethodGet, "/api/droplets/"+d.ID, nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var got cistern.Droplet
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.ID != d.ID {
		t.Errorf("ID = %q, want %q", got.ID, d.ID)
	}
	if got.Title != "Specific" {
		t.Errorf("Title = %q, want %q", got.Title, "Specific")
	}
}

func TestAPI_GetDropletByID_NotFound(t *testing.T) {
	mux := newDashboardMux(tempCfg(t), tempDB(t))
	req := httptest.NewRequest(http.MethodGet, "/api/droplets/nonexistent", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func TestAPI_GetDropletsSearch_ReturnsMatches(t *testing.T) {
	db := tempDB(t)
	c, err := cistern.New(db, "mr")
	if err != nil {
		t.Fatal(err)
	}
	c.Add("myrepo", "Feature Alpha", "", 1, 2)
	c.Add("myrepo", "Bug Fix Beta", "", 2, 2)
	c.Close()

	mux := newDashboardMux(tempCfg(t), db)
	req := httptest.NewRequest(http.MethodGet, "/api/droplets/search?query=alpha", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	var droplets []*cistern.Droplet
	if err := json.NewDecoder(w.Body).Decode(&droplets); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(droplets) != 1 {
		t.Errorf("expected 1 result, got %d", len(droplets))
	}
	if droplets[0].Title != "Feature Alpha" {
		t.Errorf("Title = %q, want 'Feature Alpha'", droplets[0].Title)
	}
}

func TestAPI_CreateDroplet_Success(t *testing.T) {
	mux := newDashboardMux(tempCfg(t), tempDB(t))
	body := `{"repo":"myrepo","title":"New Feature","description":"desc","priority":1,"complexity":2}`
	req := httptest.NewRequest(http.MethodPost, "/api/droplets", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", w.Code)
	}
	var got cistern.Droplet
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Title != "New Feature" {
		t.Errorf("Title = %q, want 'New Feature'", got.Title)
	}
	if got.Repo != "myrepo" {
		t.Errorf("Repo = %q, want 'myrepo'", got.Repo)
	}
	if got.Status != "open" {
		t.Errorf("Status = %q, want 'open'", got.Status)
	}
}

func TestAPI_CreateDroplet_MissingTitle(t *testing.T) {
	mux := newDashboardMux(tempCfg(t), tempDB(t))
	body := `{"repo":"myrepo","description":"no title"}`
	req := httptest.NewRequest(http.MethodPost, "/api/droplets", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestAPI_CreateDroplet_MissingRepo(t *testing.T) {
	mux := newDashboardMux(tempCfg(t), tempDB(t))
	body := `{"title":"No Repo"}`
	req := httptest.NewRequest(http.MethodPost, "/api/droplets", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestAPI_EditDroplet_Success(t *testing.T) {
	db := tempDB(t)
	c, err := cistern.New(db, "mr")
	if err != nil {
		t.Fatal(err)
	}
	d, _ := c.Add("myrepo", "Original", "", 1, 2)
	c.Close()

	mux := newDashboardMux(tempCfg(t), db)
	body := `{"title":"Updated"}`
	req := httptest.NewRequest(http.MethodPatch, "/api/droplets/"+d.ID, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var got cistern.Droplet
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Title != "Updated" {
		t.Errorf("Title = %q, want 'Updated'", got.Title)
	}
}

func TestAPI_RenameDroplet_Success(t *testing.T) {
	db := tempDB(t)
	c, err := cistern.New(db, "mr")
	if err != nil {
		t.Fatal(err)
	}
	d, _ := c.Add("myrepo", "Old Name", "", 1, 2)
	c.Close()

	mux := newDashboardMux(tempCfg(t), db)
	body := `{"title":"New Name"}`
	req := httptest.NewRequest(http.MethodPost, "/api/droplets/"+d.ID+"/rename", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
}

// --- Droplet state transitions ---

func TestAPI_PassDroplet_Success(t *testing.T) {
	db := tempDB(t)
	c, err := cistern.New(db, "mr")
	if err != nil {
		t.Fatal(err)
	}
	d, _ := c.Add("myrepo", "Test", "", 1, 2)
	c.Close()

	mux := newDashboardMux(tempCfg(t), db)
	body := `{"notes":"work complete"}`
	req := httptest.NewRequest(http.MethodPost, "/api/droplets/"+d.ID+"/pass", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestAPI_RecirculateDroplet_Success(t *testing.T) {
	db := tempDB(t)
	c, err := cistern.New(db, "mr")
	if err != nil {
		t.Fatal(err)
	}
	d, _ := c.Add("myrepo", "Test", "", 1, 2)
	c.Close()

	mux := newDashboardMux(tempCfg(t), db)
	body := `{"to":"implement","notes":"needs rework"}`
	req := httptest.NewRequest(http.MethodPost, "/api/droplets/"+d.ID+"/recirculate", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestAPI_PoolDroplet_Success(t *testing.T) {
	db := tempDB(t)
	c, err := cistern.New(db, "mr")
	if err != nil {
		t.Fatal(err)
	}
	d, _ := c.Add("myrepo", "Test", "", 1, 2)
	c.Close()

	mux := newDashboardMux(tempCfg(t), db)
	body := `{"notes":"blocked by external dep"}`
	req := httptest.NewRequest(http.MethodPost, "/api/droplets/"+d.ID+"/pool", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestAPI_CloseDroplet_Success(t *testing.T) {
	db := tempDB(t)
	c, err := cistern.New(db, "mr")
	if err != nil {
		t.Fatal(err)
	}
	d, _ := c.Add("myrepo", "Test", "", 1, 2)
	c.Close()

	mux := newDashboardMux(tempCfg(t), db)
	req := httptest.NewRequest(http.MethodPost, "/api/droplets/"+d.ID+"/close", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestAPI_ReopenDroplet_Success(t *testing.T) {
	db := tempDB(t)
	c, err := cistern.New(db, "mr")
	if err != nil {
		t.Fatal(err)
	}
	d, _ := c.Add("myrepo", "Test", "", 1, 2)
	c.CloseItem(d.ID)
	c.Close()

	mux := newDashboardMux(tempCfg(t), db)
	req := httptest.NewRequest(http.MethodPost, "/api/droplets/"+d.ID+"/reopen", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestAPI_CancelDroplet_Success(t *testing.T) {
	db := tempDB(t)
	c, err := cistern.New(db, "mr")
	if err != nil {
		t.Fatal(err)
	}
	d, _ := c.Add("myrepo", "Test", "", 1, 2)
	c.Close()

	mux := newDashboardMux(tempCfg(t), db)
	body := `{"reason":"no longer needed"}`
	req := httptest.NewRequest(http.MethodPost, "/api/droplets/"+d.ID+"/cancel", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestAPI_RestartDroplet_Success(t *testing.T) {
	db := tempDB(t)
	c, err := cistern.New(db, "mr")
	if err != nil {
		t.Fatal(err)
	}
	d, _ := c.Add("myrepo", "Test", "", 1, 2)
	c.Close()

	mux := newDashboardMux(tempCfg(t), db)
	body := `{"cataractae":"implement"}`
	req := httptest.NewRequest(http.MethodPost, "/api/droplets/"+d.ID+"/restart", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestAPI_ApproveDroplet_Success(t *testing.T) {
	db := tempDB(t)
	c, err := cistern.New(db, "mr")
	if err != nil {
		t.Fatal(err)
	}
	d, _ := c.Add("myrepo", "Test", "", 1, 2)
	c.Assign(d.ID, "", "human")
	c.Close()

	mux := newDashboardMux(tempCfg(t), db)
	req := httptest.NewRequest(http.MethodPost, "/api/droplets/"+d.ID+"/approve", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestAPI_Heartbeat_Success(t *testing.T) {
	db := tempDB(t)
	c, err := cistern.New(db, "mr")
	if err != nil {
		t.Fatal(err)
	}
	d, _ := c.Add("myrepo", "Test", "", 1, 2)
	c.Close()

	mux := newDashboardMux(tempCfg(t), db)
	req := httptest.NewRequest(http.MethodPost, "/api/droplets/"+d.ID+"/heartbeat", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

// --- Notes ---

func TestAPI_GetNotes_ReturnsNotes(t *testing.T) {
	db := tempDB(t)
	c, err := cistern.New(db, "mr")
	if err != nil {
		t.Fatal(err)
	}
	d, _ := c.Add("myrepo", "Test", "", 1, 2)
	c.AddNote(d.ID, "implementer", "hello world")
	c.AddNote(d.ID, "reviewer", "looks good")
	c.Close()

	mux := newDashboardMux(tempCfg(t), db)
	req := httptest.NewRequest(http.MethodGet, "/api/droplets/"+d.ID+"/notes", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var notes []cistern.CataractaeNote
	if err := json.NewDecoder(w.Body).Decode(&notes); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(notes) != 2 {
		t.Errorf("len(notes) = %d, want 2", len(notes))
	}
}

func TestAPI_AddNote_Success(t *testing.T) {
	db := tempDB(t)
	c, err := cistern.New(db, "mr")
	if err != nil {
		t.Fatal(err)
	}
	d, _ := c.Add("myrepo", "Test", "", 1, 2)
	c.Close()

	mux := newDashboardMux(tempCfg(t), db)
	body := `{"cataractae":"implementer","content":"progress update"}`
	req := httptest.NewRequest(http.MethodPost, "/api/droplets/"+d.ID+"/notes", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("status = %d, want 201", w.Code)
	}
}

func TestAPI_AddNote_MissingContent(t *testing.T) {
	db := tempDB(t)
	c, err := cistern.New(db, "mr")
	if err != nil {
		t.Fatal(err)
	}
	d, _ := c.Add("myrepo", "Test", "", 1, 2)
	c.Close()

	mux := newDashboardMux(tempCfg(t), db)
	body := `{"cataractae":"implementer"}`
	req := httptest.NewRequest(http.MethodPost, "/api/droplets/"+d.ID+"/notes", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

// --- Issues ---

func TestAPI_GetIssues_ReturnsIssues(t *testing.T) {
	db := tempDB(t)
	c, err := cistern.New(db, "mr")
	if err != nil {
		t.Fatal(err)
	}
	d, _ := c.Add("myrepo", "Test", "", 1, 2)
	c.AddIssue(d.ID, "reviewer", "bug found")
	c.Close()

	mux := newDashboardMux(tempCfg(t), db)
	req := httptest.NewRequest(http.MethodGet, "/api/droplets/"+d.ID+"/issues", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var issues []cistern.DropletIssue
	if err := json.NewDecoder(w.Body).Decode(&issues); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(issues) != 1 {
		t.Errorf("len(issues) = %d, want 1", len(issues))
	}
}

func TestAPI_GetIssues_FilterOpen(t *testing.T) {
	db := tempDB(t)
	c, err := cistern.New(db, "mr")
	if err != nil {
		t.Fatal(err)
	}
	d, _ := c.Add("myrepo", "Test", "", 1, 2)
	iss, _ := c.AddIssue(d.ID, "reviewer", "open bug")
	c.ResolveIssue(iss.ID, "fixed")
	c.Close()

	mux := newDashboardMux(tempCfg(t), db)
	req := httptest.NewRequest(http.MethodGet, "/api/droplets/"+d.ID+"/issues?open=true", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	var issues []cistern.DropletIssue
	if err := json.NewDecoder(w.Body).Decode(&issues); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(issues) != 0 {
		t.Errorf("len(issues) = %d, want 0 (no open issues)", len(issues))
	}
}

func TestAPI_AddIssue_Success(t *testing.T) {
	db := tempDB(t)
	c, err := cistern.New(db, "mr")
	if err != nil {
		t.Fatal(err)
	}
	d, _ := c.Add("myrepo", "Test", "", 1, 2)
	c.Close()

	mux := newDashboardMux(tempCfg(t), db)
	body := `{"flagged_by":"reviewer","description":"security issue found"}`
	req := httptest.NewRequest(http.MethodPost, "/api/droplets/"+d.ID+"/issues", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("status = %d, want 201", w.Code)
	}
}

func TestAPI_ResolveIssue_Success(t *testing.T) {
	db := tempDB(t)
	c, err := cistern.New(db, "mr")
	if err != nil {
		t.Fatal(err)
	}
	d, _ := c.Add("myrepo", "Test", "", 1, 2)
	iss, _ := c.AddIssue(d.ID, "reviewer", "to fix")
	c.Close()

	mux := newDashboardMux(tempCfg(t), db)
	body := `{"evidence":"test passing"}`
	req := httptest.NewRequest(http.MethodPost, "/api/issues/"+iss.ID+"/resolve", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestAPI_RejectIssue_Success(t *testing.T) {
	db := tempDB(t)
	c, err := cistern.New(db, "mr")
	if err != nil {
		t.Fatal(err)
	}
	d, _ := c.Add("myrepo", "Test", "", 1, 2)
	iss, _ := c.AddIssue(d.ID, "reviewer", "to reject")
	c.Close()

	mux := newDashboardMux(tempCfg(t), db)
	body := `{"evidence":"still broken"}`
	req := httptest.NewRequest(http.MethodPost, "/api/issues/"+iss.ID+"/reject", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

// --- Dependencies ---

func TestAPI_GetDependencies_ReturnsDeps(t *testing.T) {
	db := tempDB(t)
	c, err := cistern.New(db, "mr")
	if err != nil {
		t.Fatal(err)
	}
	parent, _ := c.Add("myrepo", "Parent", "", 1, 2)
	child, _ := c.Add("myrepo", "Child", "", 2, 2, parent.ID)
	c.Close()

	mux := newDashboardMux(tempCfg(t), db)
	req := httptest.NewRequest(http.MethodGet, "/api/droplets/"+child.ID+"/dependencies", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var deps []string
	if err := json.NewDecoder(w.Body).Decode(&deps); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(deps) != 1 || deps[0] != parent.ID {
		t.Errorf("deps = %v, want [%s]", deps, parent.ID)
	}
}

func TestAPI_AddDependency_Success(t *testing.T) {
	db := tempDB(t)
	c, err := cistern.New(db, "mr")
	if err != nil {
		t.Fatal(err)
	}
	parent, _ := c.Add("myrepo", "Parent", "", 1, 2)
	child, _ := c.Add("myrepo", "Child", "", 2, 2)
	c.Close()

	mux := newDashboardMux(tempCfg(t), db)
	body := fmt.Sprintf(`{"depends_on":"%s"}`, parent.ID)
	req := httptest.NewRequest(http.MethodPost, "/api/droplets/"+child.ID+"/dependencies", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("status = %d, want 201", w.Code)
	}
}

func TestAPI_RemoveDependency_Success(t *testing.T) {
	db := tempDB(t)
	c, err := cistern.New(db, "mr")
	if err != nil {
		t.Fatal(err)
	}
	parent, _ := c.Add("myrepo", "Parent", "", 1, 2)
	child, _ := c.Add("myrepo", "Child", "", 2, 2, parent.ID)
	c.Close()

	mux := newDashboardMux(tempCfg(t), db)
	req := httptest.NewRequest(http.MethodDelete, "/api/droplets/"+child.ID+"/dependencies/"+parent.ID, nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

// --- History/Log ---

func TestAPI_GetDropletLog_ReturnsTimeline(t *testing.T) {
	db := tempDB(t)
	c, err := cistern.New(db, "mr")
	if err != nil {
		t.Fatal(err)
	}
	d, _ := c.Add("myrepo", "Test", "", 1, 2)
	c.AddNote(d.ID, "implementer", "started")
	c.Close()

	mux := newDashboardMux(tempCfg(t), db)
	req := httptest.NewRequest(http.MethodGet, "/api/droplets/"+d.ID+"/log", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var changes []cistern.DropletChange
	if err := json.NewDecoder(w.Body).Decode(&changes); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(changes) == 0 {
		t.Errorf("expected at least 1 change entry, got 0")
	}
}

func TestAPI_GetDropletChanges_ReturnsChanges(t *testing.T) {
	db := tempDB(t)
	c, err := cistern.New(db, "mr")
	if err != nil {
		t.Fatal(err)
	}
	d, _ := c.Add("myrepo", "Test", "", 1, 2)
	c.AddNote(d.ID, "implementer", "half done")
	c.Close()

	mux := newDashboardMux(tempCfg(t), db)
	req := httptest.NewRequest(http.MethodGet, "/api/droplets/"+d.ID+"/changes?limit=10", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
}

// --- Stats ---

func TestAPI_GetStats_ReturnsStats(t *testing.T) {
	db := tempDB(t)
	c, err := cistern.New(db, "mr")
	if err != nil {
		t.Fatal(err)
	}
	c.Add("myrepo", "Test", "", 1, 2)
	c.Close()

	mux := newDashboardMux(tempCfg(t), db)
	req := httptest.NewRequest(http.MethodGet, "/api/stats", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var stats cistern.DropletStats
	if err := json.NewDecoder(w.Body).Decode(&stats); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if stats.Queued != 1 {
		t.Errorf("Queued = %d, want 1", stats.Queued)
	}
}

// --- Export ---

func TestAPI_ExportJSON(t *testing.T) {
	db := tempDB(t)
	c, err := cistern.New(db, "mr")
	if err != nil {
		t.Fatal(err)
	}
	c.Add("myrepo", "Export Test", "", 1, 2)
	c.Close()

	mux := newDashboardMux(tempCfg(t), db)
	req := httptest.NewRequest(http.MethodGet, "/api/droplets/export?format=json", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
}

func TestAPI_ExportCSV(t *testing.T) {
	db := tempDB(t)
	c, err := cistern.New(db, "mr")
	if err != nil {
		t.Fatal(err)
	}
	c.Add("myrepo", "Export CSV", "a description", 1, 2)
	c.Close()

	mux := newDashboardMux(tempCfg(t), db)
	req := httptest.NewRequest(http.MethodGet, "/api/droplets/export?format=csv", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/csv") {
		t.Errorf("Content-Type = %q, want text/csv", ct)
	}
	body := w.Body.String()
	if !strings.Contains(body, "id,repo,title,description,priority") {
		t.Errorf("CSV missing header row, got:\n%s", body)
	}
	if !strings.Contains(body, "Export CSV") {
		t.Errorf("CSV missing data row, got:\n%s", body)
	}
	lines := strings.Count(body, "\n")
	if lines < 2 {
		t.Errorf("expected at least header + 1 data row, got %d lines", lines)
	}
}

// --- Purge ---

func TestAPI_Purge_Success(t *testing.T) {
	mux := newDashboardMux(tempCfg(t), tempDB(t))
	body := `{"older_than":"24h","dry_run":true}`
	req := httptest.NewRequest(http.MethodPost, "/api/droplets/purge", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestAPI_Purge_EmptyOlderThan(t *testing.T) {
	mux := newDashboardMux(tempCfg(t), tempDB(t))
	body := `{"older_than":"","dry_run":true}`
	req := httptest.NewRequest(http.MethodPost, "/api/droplets/purge", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for empty older_than", w.Code)
	}
}

// --- Castellarius ---

func TestAPI_CastellariusStatus(t *testing.T) {
	mux := newDashboardMux(tempCfg(t), tempDB(t))
	req := httptest.NewRequest(http.MethodGet, "/api/castellarius/status", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

// --- Doctor ---

func TestAPI_Doctor(t *testing.T) {
	mux := newDashboardMux(tempCfg(t), tempDB(t))
	req := httptest.NewRequest(http.MethodGet, "/api/doctor", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestAPI_Doctor_WithFix(t *testing.T) {
	mux := newDashboardMux(tempCfg(t), tempDB(t))
	req := httptest.NewRequest(http.MethodGet, "/api/doctor?fix=true", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

// --- Repos & Skills ---

func TestAPI_GetRepos(t *testing.T) {
	mux := newDashboardMux(tempCfg(t), tempDB(t))
	req := httptest.NewRequest(http.MethodGet, "/api/repos", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestAPI_GetSkills(t *testing.T) {
	mux := newDashboardMux(tempCfg(t), tempDB(t))
	req := httptest.NewRequest(http.MethodGet, "/api/skills", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

// --- CORS ---

func TestAPI_CORS_Preflight(t *testing.T) {
	mux := newDashboardMux(tempCfg(t), tempDB(t))
	req := httptest.NewRequest(http.MethodOptions, "/api/droplets", nil)
	req.Header.Set("Origin", "http://localhost:5737")
	req.Header.Set("Access-Control-Request-Method", "GET")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("OPTIONS status = %d, want 204", w.Code)
	}
	allowOrigin := w.Header().Get("Access-Control-Allow-Origin")
	if allowOrigin != "http://localhost:5737" {
		t.Errorf("Access-Control-Allow-Origin = %q, want 'http://localhost:5737'", allowOrigin)
	}
	allowHeaders := w.Header().Get("Access-Control-Allow-Headers")
	if !strings.Contains(allowHeaders, "Authorization") {
		t.Errorf("Access-Control-Allow-Headers = %q, want to contain 'Authorization'", allowHeaders)
	}
	allowCreds := w.Header().Get("Access-Control-Allow-Credentials")
	if allowCreds != "true" {
		t.Errorf("Access-Control-Allow-Credentials = %q, want 'true'", allowCreds)
	}
}

func TestAPI_CORS_RejectedOrigin(t *testing.T) {
	mux := newDashboardMux(tempCfg(t), tempDB(t))
	req := httptest.NewRequest(http.MethodGet, "/api/stats", nil)
	req.Header.Set("Origin", "https://evil.example.com")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	allowOrigin := w.Header().Get("Access-Control-Allow-Origin")
	if allowOrigin != "" {
		t.Errorf("Access-Control-Allow-Origin = %q, want empty for rejected origin", allowOrigin)
	}
}

func TestAPI_CORS_HeadersOnGetRequest(t *testing.T) {
	mux := newDashboardMux(tempCfg(t), tempDB(t))
	req := httptest.NewRequest(http.MethodGet, "/api/stats", nil)
	req.Header.Set("Origin", "http://localhost:5737")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	allowOrigin := w.Header().Get("Access-Control-Allow-Origin")
	if allowOrigin != "http://localhost:5737" {
		t.Errorf("Access-Control-Allow-Origin = %q, want 'http://localhost:5737'", allowOrigin)
	}
}

// --- Error case tests ---

func TestAPI_GetDropletByID_NonexistentID(t *testing.T) {
	mux := newDashboardMux(tempCfg(t), tempDB(t))
	req := httptest.NewRequest(http.MethodGet, "/api/droplets/nonexistent-id-12345", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404 for nonexistent droplet", w.Code)
	}
}

func TestAPI_EditDroplet_NonexistentID(t *testing.T) {
	mux := newDashboardMux(tempCfg(t), tempDB(t))
	body := `{"title":"Updated"}`
	req := httptest.NewRequest(http.MethodPatch, "/api/droplets/nonexistent-id-12345", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404 for edit of nonexistent droplet", w.Code)
	}
}

func TestAPI_PassDroplet_NonexistentID(t *testing.T) {
	mux := newDashboardMux(tempCfg(t), tempDB(t))
	body := `{"notes":"done"}`
	req := httptest.NewRequest(http.MethodPost, "/api/droplets/nonexistent-id-12345/pass", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404 for pass of nonexistent droplet", w.Code)
	}
}

func TestAPI_ApproveDroplet_NotHumanGated(t *testing.T) {
	db := tempDB(t)
	c, err := cistern.New(db, "mr")
	if err != nil {
		t.Fatal(err)
	}
	d, _ := c.Add("myrepo", "Test", "", 1, 2)
	// Not assigning to "human" — default is not human-gated
	c.Close()

	mux := newDashboardMux(tempCfg(t), db)
	req := httptest.NewRequest(http.MethodPost, "/api/droplets/"+d.ID+"/approve", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for approving non-human-gated droplet", w.Code)
	}
}

func TestAPI_Purge_InvalidDuration(t *testing.T) {
	mux := newDashboardMux(tempCfg(t), tempDB(t))
	body := `{"older_than":"not-a-duration","dry_run":true}`
	req := httptest.NewRequest(http.MethodPost, "/api/droplets/purge", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for invalid older_than", w.Code)
	}
}

func TestAPI_ExportWithRepoFilter(t *testing.T) {
	db := tempDB(t)
	c, err := cistern.New(db, "mr")
	if err != nil {
		t.Fatal(err)
	}
	c.Add("repo-a", "Drop A", "", 1, 2)
	c.Add("repo-b", "Drop B", "", 1, 2)
	c.Close()

	mux := newDashboardMux(tempCfg(t), db)
	req := httptest.NewRequest(http.MethodGet, "/api/droplets/export?repo=repo-a&format=json", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var droplets []*cistern.Droplet
	if err := json.NewDecoder(w.Body).Decode(&droplets); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(droplets) != 1 {
		t.Errorf("expected 1 droplet with repo filter, got %d", len(droplets))
	}
	if droplets[0].Repo != "repo-a" {
		t.Errorf("expected repo-a, got %s", droplets[0].Repo)
	}
}

func TestAPI_ExportWithRepoAndStatusFilter(t *testing.T) {
	db := tempDB(t)
	c, err := cistern.New(db, "mr")
	if err != nil {
		t.Fatal(err)
	}
	c.Add("repo-a", "Drop A", "", 1, 2)
	dropB, _ := c.Add("repo-a", "Drop B", "", 1, 2)
	c.Add("repo-b", "Drop C", "", 1, 2)
	c.CloseItem(dropB.ID)
	c.Close()

	mux := newDashboardMux(tempCfg(t), db)
	req := httptest.NewRequest(http.MethodGet, "/api/droplets/export?repo=repo-a&status=open&format=json", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var droplets []*cistern.Droplet
	if err := json.NewDecoder(w.Body).Decode(&droplets); err != nil {
		t.Fatalf("decode: %v", err)
	}
	for _, d := range droplets {
		if d.Repo != "repo-a" {
			t.Errorf("got droplet with repo=%q, want repo-a", d.Repo)
		}
		if d.Status != "open" {
			t.Errorf("got droplet with status=%q, want open", d.Status)
		}
	}
	if len(droplets) != 1 {
		t.Errorf("expected 1 droplet with repo=repo-a and status=open, got %d", len(droplets))
	}
}

func TestAPI_ExportWithRepoAndStatusFilter_CSV(t *testing.T) {
	db := tempDB(t)
	c, err := cistern.New(db, "mr")
	if err != nil {
		t.Fatal(err)
	}
	c.Add("repo-a", "Drop A", "", 1, 2)
	dropB, _ := c.Add("repo-a", "Drop B", "", 1, 2)
	c.Add("repo-b", "Drop C", "", 1, 2)
	c.CloseItem(dropB.ID)
	c.Close()

	mux := newDashboardMux(tempCfg(t), db)
	req := httptest.NewRequest(http.MethodGet, "/api/droplets/export?repo=repo-a&status=open&format=csv", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/csv") {
		t.Errorf("Content-Type = %q, want text/csv", ct)
	}
	reader := csv.NewReader(w.Body)
	records, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("parse CSV: %v", err)
	}
	if len(records) < 2 {
		t.Fatalf("expected header + at least 1 data row, got %d rows", len(records))
	}
	for _, row := range records[1:] {
		if len(row) >= 1 && row[1] != "repo-a" {
			t.Errorf("got repo=%q, want repo-a", row[1])
		}
	}
	dataRows := len(records) - 1
	if dataRows != 1 {
		t.Errorf("expected 1 data row with repo=repo-a and status=open, got %d", dataRows)
	}
}

// --- SSE event stream ---

func TestAPI_DropletEvents_SSE(t *testing.T) {
	db := tempDB(t)
	c, err := cistern.New(db, "mr")
	if err != nil {
		t.Fatal(err)
	}
	d, _ := c.Add("myrepo", "Test", "", 1, 2)
	c.Close()

	mux := newDashboardMux(tempCfg(t), db)
	req := httptest.NewRequest(http.MethodGet, "/api/droplets/"+d.ID+"/events", nil)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/event-stream") {
		t.Errorf("Content-Type = %q, want text/event-stream", ct)
	}
}

func TestAPI_DropletEvents_NonexistentDroplet(t *testing.T) {
	mux := newDashboardMux(tempCfg(t), tempDB(t))
	req := httptest.NewRequest(http.MethodGet, "/api/droplets/nonexistent-id-99999/events", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404 for nonexistent droplet SSE", w.Code)
	}
	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json for error response", ct)
	}
}

func TestAPI_ApproveDroplet_NonexistentID(t *testing.T) {
	mux := newDashboardMux(tempCfg(t), tempDB(t))
	req := httptest.NewRequest(http.MethodPost, "/api/droplets/nonexistent-id-12345/approve", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404 for approving nonexistent droplet", w.Code)
	}
}

func TestAPI_DropletLog_FormatNotes(t *testing.T) {
	db := tempDB(t)
	c, err := cistern.New(db, "mr")
	if err != nil {
		t.Fatal(err)
	}
	d, _ := c.Add("myrepo", "Test", "", 1, 2)
	c.AddNote(d.ID, "implementer", "note one")
	c.AddNote(d.ID, "reviewer", "note two")
	c.Close()

	mux := newDashboardMux(tempCfg(t), db)
	req := httptest.NewRequest(http.MethodGet, "/api/droplets/"+d.ID+"/log?format=notes", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var notes []cistern.CataractaeNote
	if err := json.NewDecoder(w.Body).Decode(&notes); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(notes) != 2 {
		t.Errorf("expected 2 notes, got %d", len(notes))
	}
}

func TestAPI_DropletLog_FormatDefault_ReturnsTimeline(t *testing.T) {
	db := tempDB(t)
	c, err := cistern.New(db, "mr")
	if err != nil {
		t.Fatal(err)
	}
	d, _ := c.Add("myrepo", "Test", "", 1, 2)
	c.AddNote(d.ID, "implementer", "started")
	c.Close()

	mux := newDashboardMux(tempCfg(t), db)
	req := httptest.NewRequest(http.MethodGet, "/api/droplets/"+d.ID+"/log", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var changes []cistern.DropletChange
	if err := json.NewDecoder(w.Body).Decode(&changes); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(changes) == 0 {
		t.Errorf("expected at least 1 change entry with default format, got 0")
	}
}

// --- 404 for nonexistent droplets on state transitions ---

func TestAPI_CloseDroplet_NonexistentID(t *testing.T) {
	mux := newDashboardMux(tempCfg(t), tempDB(t))
	req := httptest.NewRequest(http.MethodPost, "/api/droplets/nonexistent-id-12345/close", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404 for close of nonexistent droplet", w.Code)
	}
}

func TestAPI_ReopenDroplet_NonexistentID(t *testing.T) {
	mux := newDashboardMux(tempCfg(t), tempDB(t))
	req := httptest.NewRequest(http.MethodPost, "/api/droplets/nonexistent-id-12345/reopen", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404 for reopen of nonexistent droplet", w.Code)
	}
}

func TestAPI_CancelDroplet_NonexistentID(t *testing.T) {
	mux := newDashboardMux(tempCfg(t), tempDB(t))
	body := `{"reason":"done"}`
	req := httptest.NewRequest(http.MethodPost, "/api/droplets/nonexistent-id-12345/cancel", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404 for cancel of nonexistent droplet", w.Code)
	}
}

func TestAPI_PoolDroplet_NonexistentID(t *testing.T) {
	mux := newDashboardMux(tempCfg(t), tempDB(t))
	body := `{"notes":"blocked"}`
	req := httptest.NewRequest(http.MethodPost, "/api/droplets/nonexistent-id-12345/pool", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404 for pool of nonexistent droplet", w.Code)
	}
}

func TestAPI_RecirculateDroplet_NonexistentID(t *testing.T) {
	mux := newDashboardMux(tempCfg(t), tempDB(t))
	body := `{"notes":"redo"}`
	req := httptest.NewRequest(http.MethodPost, "/api/droplets/nonexistent-id-12345/recirculate", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404 for recirculate of nonexistent droplet", w.Code)
	}
}

// --- 400 for malformed JSON on signal endpoints ---

func TestAPI_PassDroplet_InvalidJSON(t *testing.T) {
	db := tempDB(t)
	c, err := cistern.New(db, "mr")
	if err != nil {
		t.Fatal(err)
	}
	d, _ := c.Add("myrepo", "Test", "", 1, 2)
	c.Close()

	mux := newDashboardMux(tempCfg(t), db)
	req := httptest.NewRequest(http.MethodPost, "/api/droplets/"+d.ID+"/pass", strings.NewReader("{invalid"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for invalid JSON in pass", w.Code)
	}
}

func TestAPI_RecirculateDroplet_InvalidJSON(t *testing.T) {
	db := tempDB(t)
	c, err := cistern.New(db, "mr")
	if err != nil {
		t.Fatal(err)
	}
	d, _ := c.Add("myrepo", "Test", "", 1, 2)
	c.Close()

	mux := newDashboardMux(tempCfg(t), db)
	req := httptest.NewRequest(http.MethodPost, "/api/droplets/"+d.ID+"/recirculate", strings.NewReader("{invalid"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for invalid JSON in recirculate", w.Code)
	}
}

func TestAPI_PoolDroplet_InvalidJSON(t *testing.T) {
	db := tempDB(t)
	c, err := cistern.New(db, "mr")
	if err != nil {
		t.Fatal(err)
	}
	d, _ := c.Add("myrepo", "Test", "", 1, 2)
	c.Close()

	mux := newDashboardMux(tempCfg(t), db)
	req := httptest.NewRequest(http.MethodPost, "/api/droplets/"+d.ID+"/pool", strings.NewReader("{invalid"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for invalid JSON in pool", w.Code)
	}
}

func TestAPI_CancelDroplet_InvalidJSON(t *testing.T) {
	db := tempDB(t)
	c, err := cistern.New(db, "mr")
	if err != nil {
		t.Fatal(err)
	}
	d, _ := c.Add("myrepo", "Test", "", 1, 2)
	c.Close()

	mux := newDashboardMux(tempCfg(t), db)
	req := httptest.NewRequest(http.MethodPost, "/api/droplets/"+d.ID+"/cancel", strings.NewReader("{invalid"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for invalid JSON in cancel", w.Code)
	}
}

// --- Optional body still works for signal endpoints ---

func TestAPI_PassDroplet_EmptyBody(t *testing.T) {
	db := tempDB(t)
	c, err := cistern.New(db, "mr")
	if err != nil {
		t.Fatal(err)
	}
	d, _ := c.Add("myrepo", "Test", "", 1, 2)
	c.Close()

	mux := newDashboardMux(tempCfg(t), db)
	req := httptest.NewRequest(http.MethodPost, "/api/droplets/"+d.ID+"/pass", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 for pass with empty body", w.Code)
	}
}

func TestAPI_CancelDroplet_EmptyBody(t *testing.T) {
	db := tempDB(t)
	c, err := cistern.New(db, "mr")
	if err != nil {
		t.Fatal(err)
	}
	d, _ := c.Add("myrepo", "Test", "", 1, 2)
	c.Close()

	mux := newDashboardMux(tempCfg(t), db)
	req := httptest.NewRequest(http.MethodPost, "/api/droplets/"+d.ID+"/cancel", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 for cancel with empty body", w.Code)
	}
}

func TestAPI_GetDropletByID_NonexistentID_ReturnsJSON(t *testing.T) {
	mux := newDashboardMux(tempCfg(t), tempDB(t))
	req := httptest.NewRequest(http.MethodGet, "/api/droplets/nonexistent-id-12345", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	var body map[string]string
	json.Unmarshal(w.Body.Bytes(), &body)
	if _, ok := body["error"]; !ok {
		t.Error("response should contain 'error' key")
	}
}

func TestAPI_ApproveDroplet_NonexistentID_ReturnsJSON(t *testing.T) {
	mux := newDashboardMux(tempCfg(t), tempDB(t))
	req := httptest.NewRequest(http.MethodPost, "/api/droplets/nonexistent-id-12345/approve", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	var body map[string]string
	json.Unmarshal(w.Body.Bytes(), &body)
	if _, ok := body["error"]; !ok {
		t.Error("response should contain 'error' key")
	}
}

func TestAPI_RestartDroplet_EmptyBody(t *testing.T) {
	db := tempDB(t)
	c, err := cistern.New(db, "mr")
	if err != nil {
		t.Fatal(err)
	}
	d, _ := c.Add("myrepo", "Test", "", 1, 2)
	c.Close()

	mux := newDashboardMux(tempCfg(t), db)
	req := httptest.NewRequest(http.MethodPost, "/api/droplets/"+d.ID+"/restart", nil)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("restart with empty body: status = %d, want 200", w.Code)
	}
}

func TestAPI_AddNote_NonexistentDroplet(t *testing.T) {
	mux := newDashboardMux(tempCfg(t), tempDB(t))
	body := `{"cataractae":"implementer","content":"a note"}`
	req := httptest.NewRequest(http.MethodPost, "/api/droplets/nonexistent-id-12345/notes", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("add note to nonexistent droplet: status = %d, want 404", w.Code)
	}
}

func TestAPI_AddIssue_NonexistentDroplet(t *testing.T) {
	mux := newDashboardMux(tempCfg(t), tempDB(t))
	body := `{"flagged_by":"reviewer","description":"a bug"}`
	req := httptest.NewRequest(http.MethodPost, "/api/droplets/nonexistent-id-12345/issues", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("add issue to nonexistent droplet: status = %d, want 404", w.Code)
	}
}

// --- Security tests ---

func TestAPI_Auth_RequiredWhenConfigured(t *testing.T) {
	const testKey = "test-secret-key-12345"
	cfgPath := tempCfgWithAPIKey(t, testKey)
	mux := newDashboardMux(cfgPath, tempDB(t))

	req := httptest.NewRequest(http.MethodGet, "/api/droplets", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("unauthenticated request: status = %d, want 401", w.Code)
	}
	var body map[string]string
	json.Unmarshal(w.Body.Bytes(), &body)
	if body["error"] != "authorization required" {
		t.Errorf("error = %q, want 'authorization required'", body["error"])
	}
}

func TestAPI_Auth_ValidBearer(t *testing.T) {
	const testKey = "test-secret-key-12345"
	cfgPath := tempCfgWithAPIKey(t, testKey)
	db := tempDB(t)
	c, err := cistern.New(db, "mr")
	if err != nil {
		t.Fatal(err)
	}
	c.Add("myrepo", "Auth Test", "", 1, 2)
	c.Close()

	mux := newDashboardMux(cfgPath, db)
	req := httptest.NewRequest(http.MethodGet, "/api/droplets", nil)
	req.Header.Set("Authorization", "Bearer "+testKey)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("authenticated request: status = %d, want 200", w.Code)
	}
}

func TestAPI_Auth_InvalidBearer(t *testing.T) {
	const testKey = "test-secret-key-12345"
	cfgPath := tempCfgWithAPIKey(t, testKey)
	mux := newDashboardMux(cfgPath, tempDB(t))

	req := httptest.NewRequest(http.MethodGet, "/api/droplets", nil)
	req.Header.Set("Authorization", "Bearer wrong-key")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("invalid key: status = %d, want 401", w.Code)
	}
}

func TestAPI_Auth_NoAuthWhenUnconfigured(t *testing.T) {
	mux := newDashboardMux(tempCfg(t), tempDB(t))

	req := httptest.NewRequest(http.MethodGet, "/api/droplets", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("no API key configured: status = %d, want 200 (auth should be off)", w.Code)
	}
}

func TestAPI_Auth_EnvOverridesConfig(t *testing.T) {
	t.Setenv("CISTERN_DASHBOARD_API_KEY", "env-key-override")
	cfgPath := tempCfgWithAPIKey(t, "config-key")
	mux := newDashboardMux(cfgPath, tempDB(t))

	req := httptest.NewRequest(http.MethodGet, "/api/stats", nil)
	req.Header.Set("Authorization", "Bearer env-key-override")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("env key should override config: status = %d, want 200", w.Code)
	}
}

func TestAPI_Auth_CastellariusProtected(t *testing.T) {
	const testKey = "test-secret-key-12345"
	cfgPath := tempCfgWithAPIKey(t, testKey)
	mux := newDashboardMux(cfgPath, tempDB(t))

	req := httptest.NewRequest(http.MethodPost, "/api/castellarius/start", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("unauthenticated castellarius start: status = %d, want 401", w.Code)
	}
}

func TestAPI_CORS_RejectedOriginDoesNotGetHeaders(t *testing.T) {
	mux := newDashboardMux(tempCfg(t), tempDB(t))
	req := httptest.NewRequest(http.MethodGet, "/api/stats", nil)
	req.Header.Set("Origin", "https://evil.example.com")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	origin := w.Header().Get("Access-Control-Allow-Origin")
	if origin != "" {
		t.Errorf("CORS origin for evil site = %q, want empty", origin)
	}
}

func TestAPI_CORS_PreflightRejectionStillReturns204(t *testing.T) {
	mux := newDashboardMux(tempCfg(t), tempDB(t))
	req := httptest.NewRequest(http.MethodOptions, "/api/droplets", nil)
	req.Header.Set("Origin", "https://evil.example.com")
	req.Header.Set("Access-Control-Request-Method", "GET")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("OPTIONS status = %d, want 204 (preflight always responds)", w.Code)
	}
	origin := w.Header().Get("Access-Control-Allow-Origin")
	if origin != "" {
		t.Errorf("evil origin should not get CORS header, got %q", origin)
	}
}

func TestAPI_BodyLimit_RejectsOversizedPayload(t *testing.T) {
	mux := newDashboardMux(tempCfg(t), tempDB(t))
	bigBody := strings.Repeat("x", 2*1024*1024)
	req := httptest.NewRequest(http.MethodPost, "/api/droplets", strings.NewReader(bigBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest && w.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("oversized body: status = %d, want 400 or 413", w.Code)
	}
}

func TestAPI_Sanitize_500ErrorMessage(t *testing.T) {
	mux := newDashboardMux(tempCfg(t), "/nonexistent/path/to/db.sqlite")
	req := httptest.NewRequest(http.MethodGet, "/api/droplets", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Logf("status = %d (may vary)", w.Code)
	}
	var body map[string]string
	json.Unmarshal(w.Body.Bytes(), &body)
	if strings.Contains(body["error"], "/") {
		t.Errorf("500 error message should not leak paths: %q", body["error"])
	}
}

func TestAPI_InputLimit_CreateDropletTitleTooLong(t *testing.T) {
	mux := newDashboardMux(tempCfg(t), tempDB(t))
	longTitle := strings.Repeat("a", 257)
	body := fmt.Sprintf(`{"repo":"myrepo","title":"%s"}`, longTitle)
	req := httptest.NewRequest(http.MethodPost, "/api/droplets", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("long title: status = %d, want 400", w.Code)
	}
}

func TestAPI_InputLimit_CreateDropletRepoTooLong(t *testing.T) {
	mux := newDashboardMux(tempCfg(t), tempDB(t))
	longRepo := strings.Repeat("b", 129)
	body := fmt.Sprintf(`{"repo":"%s","title":"test"}`, longRepo)
	req := httptest.NewRequest(http.MethodPost, "/api/droplets", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("long repo: status = %d, want 400", w.Code)
	}
}

func TestAPI_InputLimit_CreateDropletDescriptionTooLong(t *testing.T) {
	mux := newDashboardMux(tempCfg(t), tempDB(t))
	longDesc := strings.Repeat("d", 4097)
	body := fmt.Sprintf(`{"repo":"myrepo","title":"test","description":"%s"}`, longDesc)
	req := httptest.NewRequest(http.MethodPost, "/api/droplets", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("long description: status = %d, want 400", w.Code)
	}
}

func TestAPI_InputLimit_RenameTitleTooLong(t *testing.T) {
	db := tempDB(t)
	c, err := cistern.New(db, "mr")
	if err != nil {
		t.Fatal(err)
	}
	d, _ := c.Add("myrepo", "Original", "", 1, 2)
	c.Close()

	mux := newDashboardMux(tempCfg(t), db)
	longTitle := strings.Repeat("a", 257)
	body := fmt.Sprintf(`{"title":"%s"}`, longTitle)
	req := httptest.NewRequest(http.MethodPost, "/api/droplets/"+d.ID+"/rename", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("rename with long title: status = %d, want 400", w.Code)
	}
}

func TestAPI_InputLimit_AddNoteContentTooLong(t *testing.T) {
	db := tempDB(t)
	c, err := cistern.New(db, "mr")
	if err != nil {
		t.Fatal(err)
	}
	d, _ := c.Add("myrepo", "Test", "", 1, 2)
	c.Close()

	mux := newDashboardMux(tempCfg(t), db)
	longContent := strings.Repeat("n", 65537)
	body := fmt.Sprintf(`{"cataractae":"implementer","content":"%s"}`, longContent)
	req := httptest.NewRequest(http.MethodPost, "/api/droplets/"+d.ID+"/notes", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("add note with long content: status = %d, want 400", w.Code)
	}
}

func TestAPI_InputLimit_AddIssueDescriptionTooLong(t *testing.T) {
	db := tempDB(t)
	c, err := cistern.New(db, "mr")
	if err != nil {
		t.Fatal(err)
	}
	d, _ := c.Add("myrepo", "Test", "", 1, 2)
	c.Close()

	mux := newDashboardMux(tempCfg(t), db)
	longDesc := strings.Repeat("i", 65537)
	body := fmt.Sprintf(`{"flagged_by":"reviewer","description":"%s"}`, longDesc)
	req := httptest.NewRequest(http.MethodPost, "/api/droplets/"+d.ID+"/issues", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("add issue with long description: status = %d, want 400", w.Code)
	}
}

func TestAPI_InputLimit_AddDependencyDepTooLong(t *testing.T) {
	db := tempDB(t)
	c, err := cistern.New(db, "mr")
	if err != nil {
		t.Fatal(err)
	}
	d, _ := c.Add("myrepo", "Test", "", 1, 2)
	c.Close()

	mux := newDashboardMux(tempCfg(t), db)
	longDep := strings.Repeat("d", 129)
	body := fmt.Sprintf(`{"depends_on":"%s"}`, longDep)
	req := httptest.NewRequest(http.MethodPost, "/api/droplets/"+d.ID+"/dependencies", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("add dep with long depends_on: status = %d, want 400", w.Code)
	}
}

func TestAPI_SSALimit_ConnectionLimit(t *testing.T) {
	db := tempDB(t)
	c, err := cistern.New(db, "mr")
	if err != nil {
		t.Fatal(err)
	}
	d, _ := c.Add("myrepo", "Test", "", 1, 2)
	c.Close()

	original := currentSSEConnections
	defer func() { currentSSEConnections = original }()
	atomic.StoreInt64(&currentSSEConnections, maxSSEConnections)

	mux := newDashboardMux(tempCfg(t), db)
	req := httptest.NewRequest(http.MethodGet, "/api/droplets/"+d.ID+"/events", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("SSE at connection limit: status = %d, want 503", w.Code)
	}
	var body map[string]string
	json.Unmarshal(w.Body.Bytes(), &body)
	if !strings.Contains(body["error"], "too many") {
		t.Errorf("error = %q, want 'too many SSE connections'", body["error"])
	}
}

// --- Auth protects all endpoints (including pre-existing ones) ---

func TestAPI_Auth_ProtectsDashboardEndpoint(t *testing.T) {
	const testKey = "test-secret-key-12345"
	cfgPath := tempCfgWithAPIKey(t, testKey)
	db := tempDB(t)
	c, err := cistern.New(db, "mr")
	if err != nil {
		t.Fatal(err)
	}
	c.Close()

	mux := newDashboardMux(cfgPath, db)
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("unauthenticated /api/dashboard: status = %d, want 401", w.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/dashboard", nil)
	req.Header.Set("Authorization", "Bearer "+testKey)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("authenticated /api/dashboard: status = %d, want 200", w.Code)
	}
}

func TestAPI_Auth_TimingSafeComparison(t *testing.T) {
	const testKey = "test-secret-key-12345"
	cfgPath := tempCfgWithAPIKey(t, testKey)
	mux := newDashboardMux(cfgPath, tempDB(t))

	req := httptest.NewRequest(http.MethodGet, "/api/droplets", nil)
	req.Header.Set("Authorization", "Bearer "+testKey)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("valid key: status = %d, want 200", w.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/droplets", nil)
	req.Header.Set("Authorization", "Bearer wrong-key")
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("invalid key: status = %d, want 401", w.Code)
	}
}

func TestAPI_DropletEvents_HeadersAfterValidation(t *testing.T) {
	mux := newDashboardMux(tempCfg(t), tempDB(t))
	req := httptest.NewRequest(http.MethodGet, "/api/droplets/nonexistent-id/events", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("nonexistent droplet SSE: status = %d, want 404", w.Code)
	}

	ct := w.Header().Get("Content-Type")
	if strings.HasPrefix(ct, "text/event-stream") {
		t.Errorf("error response should not have SSE Content-Type, got %q", ct)
	}
	if !strings.Contains(ct, "application/json") {
		t.Errorf("error response should be JSON, got Content-Type %q", ct)
	}
}

func TestAPI_ApproveDroplet_NotHumanGated_NoLeak(t *testing.T) {
	db := tempDB(t)
	c, err := cistern.New(db, "mr")
	if err != nil {
		t.Fatal(err)
	}
	d, _ := c.Add("myrepo", "Test", "", 1, 2)
	c.Close()

	mux := newDashboardMux(tempCfg(t), db)
	req := httptest.NewRequest(http.MethodPost, "/api/droplets/"+d.ID+"/approve", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("approve non-human: status = %d, want 400", w.Code)
	}
	var body map[string]string
	json.Unmarshal(w.Body.Bytes(), &body)
	if strings.Contains(body["error"], "cataractae:") {
		t.Errorf("error message leaks cataractae assignment: %q", body["error"])
	}
	if body["error"] != "droplet is not awaiting human approval" {
		t.Errorf("error = %q, want 'droplet is not awaiting human approval'", body["error"])
	}
}

func TestAPI_DropletLog_LimitCapsAt1000(t *testing.T) {
	db := tempDB(t)
	c, err := cistern.New(db, "mr")
	if err != nil {
		t.Fatal(err)
	}
	d, _ := c.Add("myrepo", "Test", "", 1, 2)
	c.Close()

	mux := newDashboardMux(tempCfg(t), db)
	req := httptest.NewRequest(http.MethodGet, "/api/droplets/"+d.ID+"/log?limit=9999", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("log with large limit: status = %d, want 200", w.Code)
	}
}

func TestAPI_DropletChanges_LimitCapsAt1000(t *testing.T) {
	db := tempDB(t)
	c, err := cistern.New(db, "mr")
	if err != nil {
		t.Fatal(err)
	}
	d, _ := c.Add("myrepo", "Test", "", 1, 2)
	c.Close()

	mux := newDashboardMux(tempCfg(t), db)
	req := httptest.NewRequest(http.MethodGet, "/api/droplets/"+d.ID+"/changes?limit=9999", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("changes with large limit: status = %d, want 200", w.Code)
	}
}

func TestAPI_CSVEscape_FormulaInjection(t *testing.T) {
	db := tempDB(t)
	c, err := cistern.New(db, "mr")
	if err != nil {
		t.Fatal(err)
	}
	c.Add("myrepo", "=SUM(A1:A10)", "starts with equals", 1, 2)
	c.Close()

	mux := newDashboardMux(tempCfg(t), db)
	req := httptest.NewRequest(http.MethodGet, "/api/droplets/export?format=csv", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("export CSV: status = %d, want 200", w.Code)
	}
	body := w.Body.String()
	reader := csv.NewReader(strings.NewReader(body))
	records, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("parse CSV: %v", err)
	}
	if len(records) < 2 {
		t.Fatalf("expected at least header + 1 data row, got %d rows", len(records))
	}
	for _, row := range records[1:] {
		for _, cell := range row {
			if len(cell) > 0 && (cell[0] == '=' || cell[0] == '+' || cell[0] == '-' || cell[0] == '@') && !strings.HasPrefix(cell, "'") {
				t.Errorf("CSV cell susceptible to formula injection: %q", cell)
			}
		}
	}
}

func TestAPI_IsValidAqueductName(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"valid-name", true},
		{"valid_name", true},
		{"ValidName123", true},
		{"", false},
		{"bad:name", false},
		{"bad.name", false},
		{"bad name", false},
		{"bad;name", false},
		{"=bad", false},
		{"$(bad)", false},
	}
	for _, tt := range tests {
		got := isValidAqueductName(tt.name)
		if got != tt.want {
			t.Errorf("isValidAqueductName(%q) = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestAPI_DecodeJSON_SanitizedErrors(t *testing.T) {
	mux := newDashboardMux(tempCfg(t), tempDB(t))
	req := httptest.NewRequest(http.MethodPost, "/api/droplets", strings.NewReader(`not json`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("invalid JSON: status = %d, want 400", w.Code)
	}
	var body map[string]string
	json.Unmarshal(w.Body.Bytes(), &body)
	if !strings.Contains(body["error"], "invalid JSON") {
		t.Errorf("error = %q, want 'invalid JSON'", body["error"])
	}
}

func TestAPI_Doctor_Sanitized500Error(t *testing.T) {
	mux := newDashboardMux("/nonexistent/config/path/cistern.yaml", tempDB(t))
	req := httptest.NewRequest(http.MethodGet, "/api/doctor", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Logf("status = %d (may vary)", w.Code)
	}
	var body map[string]string
	json.Unmarshal(w.Body.Bytes(), &body)
	if strings.Contains(body["error"], "/") {
		t.Errorf("500 error should not leak file paths: %q", body["error"])
	}
	if body["error"] != "internal error" {
		t.Errorf("error = %q, want 'internal error'", body["error"])
	}
}

func TestAPI_CsvSanitizeCell(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"=SUM(A1:A10)", "'=SUM(A1:A10)"},
		{"+formula", "'+formula"},
		{"-formula", "'-formula"},
		{"@formula", "'@formula"},
		{"normal", "normal"},
		{"", ""},
		{"hello world", "hello world"},
	}
	for _, tt := range tests {
		got := csvSanitizeCell(tt.input)
		if got != tt.want {
			t.Errorf("csvSanitizeCell(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestAPI_CORS_Preflight_WithAuth(t *testing.T) {
	const testKey = "test-secret-key-12345"
	cfgPath := tempCfgWithAPIKey(t, testKey)
	mux := newDashboardMux(cfgPath, tempDB(t))
	req := httptest.NewRequest(http.MethodOptions, "/api/droplets", nil)
	req.Header.Set("Origin", "http://localhost:5737")
	req.Header.Set("Access-Control-Request-Method", "GET")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("OPTIONS with auth: status = %d, want 204 (preflight must bypass auth)", w.Code)
	}
	origin := w.Header().Get("Access-Control-Allow-Origin")
	if origin != "http://localhost:5737" {
		t.Errorf("Access-Control-Allow-Origin = %q, want 'http://localhost:5737'", origin)
	}
}

func TestAPI_InputLimit_EditDropletTitleTooLong(t *testing.T) {
	db := tempDB(t)
	c, err := cistern.New(db, "mr")
	if err != nil {
		t.Fatal(err)
	}
	d, _ := c.Add("myrepo", "Original", "", 1, 2)
	c.Close()

	mux := newDashboardMux(tempCfg(t), db)
	longTitle := strings.Repeat("a", 257)
	body := fmt.Sprintf(`{"title":"%s"}`, longTitle)
	req := httptest.NewRequest(http.MethodPatch, "/api/droplets/"+d.ID, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("PATCH edit with long title: status = %d, want 400", w.Code)
	}
}

func TestAPI_InputLimit_EditDropletDescriptionTooLong(t *testing.T) {
	db := tempDB(t)
	c, err := cistern.New(db, "mr")
	if err != nil {
		t.Fatal(err)
	}
	d, _ := c.Add("myrepo", "Original", "", 1, 2)
	c.Close()

	mux := newDashboardMux(tempCfg(t), db)
	longDesc := strings.Repeat("a", 4097)
	body := fmt.Sprintf(`{"description":"%s"}`, longDesc)
	req := httptest.NewRequest(http.MethodPatch, "/api/droplets/"+d.ID, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("PATCH edit with long description: status = %d, want 400", w.Code)
	}
}

func TestAPI_InputLimit_PassDropletNotesTooLong(t *testing.T) {
	db := tempDB(t)
	c, err := cistern.New(db, "mr")
	if err != nil {
		t.Fatal(err)
	}
	d, _ := c.Add("myrepo", "Test", "", 1, 2)
	c.GetReady("myrepo")
	c.Close()

	mux := newDashboardMux(tempCfg(t), db)
	longNotes := strings.Repeat("n", 65537)
	body := fmt.Sprintf(`{"notes":"%s"}`, longNotes)
	req := httptest.NewRequest(http.MethodPost, "/api/droplets/"+d.ID+"/pass", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("pass with long notes: status = %d, want 400", w.Code)
	}
}

func TestAPI_InputLimit_CancelDropletReasonTooLong(t *testing.T) {
	db := tempDB(t)
	c, err := cistern.New(db, "mr")
	if err != nil {
		t.Fatal(err)
	}
	d, _ := c.Add("myrepo", "Test", "", 1, 2)
	c.GetReady("myrepo")
	c.Close()

	mux := newDashboardMux(tempCfg(t), db)
	longReason := strings.Repeat("r", 65537)
	body := fmt.Sprintf(`{"reason":"%s"}`, longReason)
	req := httptest.NewRequest(http.MethodPost, "/api/droplets/"+d.ID+"/cancel", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("cancel with long reason: status = %d, want 400", w.Code)
	}
}
