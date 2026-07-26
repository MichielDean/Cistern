package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/MichielDean/cistern/internal/cistern"
)

// This file holds behavioral coverage for pool-reason display on the dashboard
// (PR #501). The PoolReasons map was populated by fetchDashboardData and
// surfaced via the GET /api/droplets/{id} endpoint's pool_reason field, but
// neither path had Go-level tests verifying the reason reaches the consumer.

// --- fetchDashboardData: PoolReasons map ---

// TestFetchDashboardData_PoolReasons_PopulatedForPooledDroplets verifies that
// when a droplet is pooled with a reason, fetchDashboardData populates the
// PoolReasons map with that reason keyed by droplet ID.
//
// Given: a database with one pooled droplet (pooled with reason "blocked by dep")
// When:  fetchDashboardData is called
// Then:  data.PoolReasons contains an entry mapping the droplet ID to the reason
func TestFetchDashboardData_PoolReasons_PopulatedForPooledDroplets(t *testing.T) {
	cfgPath := tempCfg(t)
	dbPath := tempDB(t)

	c, err := cistern.New(dbPath, "mr")
	if err != nil {
		t.Fatal(err)
	}
	pooled, _ := c.Add("myrepo", "Stuck Feature", "", 1)
	c.GetReady("myrepo") // transition to in_progress so Pool is allowed
	if err := c.Pool(pooled.ID, "blocked by upstream dependency"); err != nil {
		t.Fatalf("Pool: %v", err)
	}
	c.Close()

	data, err := fetchDashboardData(cfgPath, dbPath)
	if err != nil {
		t.Fatalf("fetchDashboardData: %v", err)
	}

	if len(data.PoolReasons) == 0 {
		t.Fatal("PoolReasons map is empty — expected at least one entry for the pooled droplet")
	}
	got, ok := data.PoolReasons[pooled.ID]
	if !ok {
		t.Fatalf("PoolReasons has no entry for pooled droplet %q; map = %v", pooled.ID, data.PoolReasons)
	}
	if got != "blocked by upstream dependency" {
		t.Errorf("PoolReasons[%q] = %q, want %q", pooled.ID, got, "blocked by upstream dependency")
	}
}

// TestFetchDashboardData_PoolReasons_EmptyWhenNoPooledDroplets verifies that
// the PoolReasons map is empty (but non-nil) when there are no pooled droplets.
//
// Given: a database with only delivered and open droplets (none pooled)
// When:  fetchDashboardData is called
// Then:  data.PoolReasons is an empty map (not nil) — the dashboard can safely
//
//	range over it without nil checks.
func TestFetchDashboardData_PoolReasons_EmptyWhenNoPooledDroplets(t *testing.T) {
	cfgPath := tempCfg(t)
	dbPath := tempDB(t)

	c, err := cistern.New(dbPath, "mr")
	if err != nil {
		t.Fatal(err)
	}
	delivered, _ := c.Add("myrepo", "Done Feature", "", 1)
	c.GetReady("myrepo")
	c.CloseItem(delivered.ID)
	c.Close()

	data, err := fetchDashboardData(cfgPath, dbPath)
	if err != nil {
		t.Fatalf("fetchDashboardData: %v", err)
	}

	if data.PoolReasons == nil {
		t.Fatal("PoolReasons map is nil — expected empty map for no pooled droplets")
	}
	if len(data.PoolReasons) != 0 {
		t.Errorf("PoolReasons len = %d, want 0 when no droplets are pooled", len(data.PoolReasons))
	}
}

// TestFetchDashboardData_PoolReasons_MultiplePooledDroplets verifies that
// multiple pooled droplets each get their own entry in the PoolReasons map.
//
// Given: a database with two pooled droplets, each pooled with a distinct reason
// When:  fetchDashboardData is called
// Then:  data.PoolReasons contains both droplet IDs mapped to their reasons
func TestFetchDashboardData_PoolReasons_MultiplePooledDroplets(t *testing.T) {
	cfgPath := tempCfg(t)
	dbPath := tempDB(t)

	c, err := cistern.New(dbPath, "mr")
	if err != nil {
		t.Fatal(err)
	}
	first, _ := c.Add("myrepo", "First Stuck", "", 1)
	c.GetReady("myrepo")
	c.Pool(first.ID, "reason one")

	second, _ := c.Add("myrepo", "Second Stuck", "", 1)
	c.GetReady("myrepo")
	c.Pool(second.ID, "reason two")
	c.Close()

	data, err := fetchDashboardData(cfgPath, dbPath)
	if err != nil {
		t.Fatalf("fetchDashboardData: %v", err)
	}

	if len(data.PoolReasons) != 2 {
		t.Fatalf("PoolReasons len = %d, want 2", len(data.PoolReasons))
	}
	if got := data.PoolReasons[first.ID]; got != "reason one" {
		t.Errorf("PoolReasons[%q] = %q, want %q", first.ID, got, "reason one")
	}
	if got := data.PoolReasons[second.ID]; got != "reason two" {
		t.Errorf("PoolReasons[%q] = %q, want %q", second.ID, got, "reason two")
	}
}

// --- GET /api/droplets/{id}: pool_reason field ---

// TestAPI_GetDropletByID_PooledDroplet_IncludesPoolReason verifies that the
// GET /api/droplets/{id} endpoint includes a "pool_reason" field in the JSON
// response when the droplet is pooled and a pool reason exists.
//
// Given: a pooled droplet with reason "blocked by external dependency"
// When:  GET /api/droplets/{id} is called
// Then:  the response body contains a "pool_reason" field with the reason text
func TestAPI_GetDropletByID_PooledDroplet_IncludesPoolReason(t *testing.T) {
	db := tempDB(t)
	c, err := cistern.New(db, "mr")
	if err != nil {
		t.Fatal(err)
	}
	d, _ := c.Add("myrepo", "Pooled Feature", "", 1)
	c.GetReady("myrepo")
	if err := c.Pool(d.ID, "blocked by external dependency"); err != nil {
		t.Fatalf("Pool: %v", err)
	}
	c.Close()

	mux := newDashboardMux(tempCfg(t), db)
	req := httptest.NewRequest(http.MethodGet, "/api/droplets/"+d.ID, nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var obj map[string]any
	if err := json.NewDecoder(w.Body).Decode(&obj); err != nil {
		t.Fatalf("decode: %v", err)
	}
	got, ok := obj["pool_reason"]
	if !ok {
		t.Fatal("response missing 'pool_reason' field for pooled droplet")
	}
	if got != "blocked by external dependency" {
		t.Errorf("pool_reason = %v, want %q", got, "blocked by external dependency")
	}
}

// TestAPI_GetDropletByID_NonPooledDroplet_OmitsPoolReason verifies that the
// GET /api/droplets/{id} endpoint does NOT include a "pool_reason" field when
// the droplet is not pooled — the field is only added for pooled droplets.
//
// Given: an open (non-pooled) droplet
// When:  GET /api/droplets/{id} is called
// Then:  the response body has no "pool_reason" field
func TestAPI_GetDropletByID_NonPooledDroplet_OmitsPoolReason(t *testing.T) {
	db := tempDB(t)
	c, err := cistern.New(db, "mr")
	if err != nil {
		t.Fatal(err)
	}
	d, _ := c.Add("myrepo", "Active Feature", "", 1)
	c.Close()

	mux := newDashboardMux(tempCfg(t), db)
	req := httptest.NewRequest(http.MethodGet, "/api/droplets/"+d.ID, nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	body := w.Body.String()
	if strings.Contains(body, "pool_reason") {
		t.Errorf("non-pooled droplet response should not contain 'pool_reason', got: %s", body)
	}
}

// TestAPI_GetDropletByID_PooledDroplet_NoReason_OmitsPoolReason verifies that
// when a droplet is pooled but has no recorded pool reason (e.g. pooled via a
// path that didn't record a 'pool' event), the endpoint omits the field rather
// than returning an empty string.
//
// Given: a pooled droplet with no pool event in the events table
// When:  GET /api/droplets/{id} is called
// Then:  the response body has no "pool_reason" field (empty reason is omitted)
func TestAPI_GetDropletByID_PooledDroplet_NoReason_OmitsPoolReason(t *testing.T) {
	db := tempDB(t)
	c, err := cistern.New(db, "mr")
	if err != nil {
		t.Fatal(err)
	}
	d, _ := c.Add("myrepo", "Pooled No Reason", "", 1)
	c.GetReady("myrepo")
	// Pool with an empty reason — the pool event is recorded but parsePayload
	// returns no "reason" key, so GetPoolReason returns "".
	c.Pool(d.ID, "")
	c.Close()

	mux := newDashboardMux(tempCfg(t), db)
	req := httptest.NewRequest(http.MethodGet, "/api/droplets/"+d.ID, nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var obj map[string]any
	if err := json.NewDecoder(w.Body).Decode(&obj); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if reason, ok := obj["pool_reason"]; ok && reason != "" {
		t.Errorf("pool_reason = %v, want absent or empty (no reason recorded)", reason)
	}
}
