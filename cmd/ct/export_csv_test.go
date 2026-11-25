package main

import (
	"encoding/csv"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/MichielDean/cistern/internal/cistern"
)

const exportCSVHeader = "id,repo,title,description,priority,status,assignee,current_cataractae,outcome,assigned_aqueduct,last_reviewed_commit,created_at,updated_at"

// seedExportDB opens a database at dbPath and adds two droplets with distinct
// repos, titles and statuses.
func seedExportDB(t *testing.T, dbPath string) {
	t.Helper()
	c, err := cistern.New(dbPath, "")
	if err != nil {
		t.Fatalf("cistern.New: %v", err)
	}
	defer c.Close()
	if _, err := c.Add("repoa", "alpha work", "first droplet", 1); err != nil {
		t.Fatalf("Add alpha: %v", err)
	}
	if _, err := c.Add("repob", "beta work", "second droplet", 2); err != nil {
		t.Fatalf("Add beta: %v", err)
	}
}

func TestDashboardWebMux_DropletsExport_CSV(t *testing.T) {
	dbPath := tempDB(t)
	seedExportDB(t, dbPath)
	mux := newDashboardMux(tempCfg(t), dbPath)

	req := httptest.NewRequest(http.MethodGet, "/api/droplets/export", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("GET /api/droplets/export status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); !strings.Contains(ct, "text/csv") {
		t.Errorf("Content-Type = %q, want text/csv", ct)
	}
	if cd := w.Header().Get("Content-Disposition"); cd != `attachment; filename="droplets.csv"` {
		t.Errorf("Content-Disposition = %q, want attachment; filename=\"droplets.csv\"", cd)
	}
	if !strings.HasPrefix(w.Body.String(), exportCSVHeader+"\n") {
		t.Errorf("body should start with CSV header, got: %q", w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "alpha work") || !strings.Contains(w.Body.String(), "beta work") {
		t.Errorf("body should contain both seeded droplets, got: %q", w.Body.String())
	}
}

func TestDashboardWebMux_DropletsExport_CSVParsesAsStructuredRows(t *testing.T) {
	dbPath := tempDB(t)
	seedExportDB(t, dbPath)
	mux := newDashboardMux(tempCfg(t), dbPath)

	req := httptest.NewRequest(http.MethodGet, "/api/droplets/export?q=alpha&format=csv", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	records, err := csv.NewReader(strings.NewReader(w.Body.String())).ReadAll()
	if err != nil {
		t.Fatalf("body is not valid CSV: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("rows = %d, want 2 (header + 1 filtered droplet)", len(records))
	}
	if records[1][2] != "alpha work" {
		t.Errorf("title column = %q, want %q", records[1][2], "alpha work")
	}
	if records[1][4] != "1" {
		t.Errorf("priority column = %q, want q-filtered droplet with priority 1", records[1][4])
	}
}

func TestDashboardWebMux_DropletsExport_StatusFilter(t *testing.T) {
	dbPath := tempDB(t)
	seedExportDB(t, dbPath)
	c, err := cistern.New(dbPath, "")
	if err != nil {
		t.Fatalf("cistern.New: %v", err)
	}
	items, err := c.Search("beta", "", 0)
	if err != nil || len(items) != 1 {
		c.Close()
		t.Fatalf("could not look up seeded beta droplet: %v (%d items)", err, len(items))
	}
	c.Close()

	mux := newDashboardMux(tempCfg(t), dbPath)
	req := httptest.NewRequest(http.MethodGet, "/api/droplets/export?status=delivered", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if strings.Contains(w.Body.String(), "beta work") || strings.Contains(w.Body.String(), "alpha work") {
		t.Errorf("queued-only filter should exclude seeded open droplets, got: %q", w.Body.String())
	}
}

func TestDashboardWebMux_DropletsExport_JSONFormat(t *testing.T) {
	dbPath := tempDB(t)
	seedExportDB(t, dbPath)
	mux := newDashboardMux(tempCfg(t), dbPath)

	req := httptest.NewRequest(http.MethodGet, "/api/droplets/export?format=json", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	if !strings.Contains(w.Body.String(), "alpha work") {
		t.Errorf("json body should contain seeded droplets, got: %q", w.Body.String())
	}
}

func TestDashboardWebMux_DropletsExport_InvalidFormat(t *testing.T) {
	mux := newDashboardMux(tempCfg(t), tempDB(t))

	req := httptest.NewRequest(http.MethodGet, "/api/droplets/export?format=xml", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestDashboardWebMux_DropletsExport_InvalidPriority(t *testing.T) {
	mux := newDashboardMux(tempCfg(t), tempDB(t))

	req := httptest.NewRequest(http.MethodGet, "/api/droplets/export?priority=high", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestDashboardWebMux_DropletsExport_MethodNotAllowed(t *testing.T) {
	mux := newDashboardMux(tempCfg(t), tempDB(t))

	req := httptest.NewRequest(http.MethodPost, "/api/droplets/export", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", w.Code)
	}
}
