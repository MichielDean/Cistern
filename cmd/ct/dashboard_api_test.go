package main

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/MichielDean/cistern/internal/cistern"
)

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
	req.Header.Set("Origin", "http://localhost:3000")
	req.Header.Set("Access-Control-Request-Method", "GET")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("OPTIONS status = %d, want 204", w.Code)
	}
	allowOrigin := w.Header().Get("Access-Control-Allow-Origin")
	if allowOrigin != "*" {
		t.Errorf("Access-Control-Allow-Origin = %q, want '*'", allowOrigin)
	}
}

func TestAPI_CORS_HeadersOnGetRequest(t *testing.T) {
	mux := newDashboardMux(tempCfg(t), tempDB(t))
	req := httptest.NewRequest(http.MethodGet, "/api/stats", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	allowOrigin := w.Header().Get("Access-Control-Allow-Origin")
	if allowOrigin != "*" {
		t.Errorf("Access-Control-Allow-Origin = %q, want '*'", allowOrigin)
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
	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500 for edit of nonexistent droplet", w.Code)
	}
}

func TestAPI_PassDroplet_NonexistentID(t *testing.T) {
	mux := newDashboardMux(tempCfg(t), tempDB(t))
	body := `{"notes":"done"}`
	req := httptest.NewRequest(http.MethodPost, "/api/droplets/nonexistent-id-12345/pass", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500 for pass of nonexistent droplet", w.Code)
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
