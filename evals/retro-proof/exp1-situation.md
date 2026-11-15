# Exp1 situation — the actual PR #20 design, reviewed against commit ad8eb3403

Droplet: "Persistent discovery service (primary sync mechanism): a long-lived
process with one cadence loop per provider, scheduling each provider's full
stage-chain (scan → delta insert → enrich → rank) under the single-flight lock.
Add per-provider cadence settings (cadence_<provider>) validated server-side
and read fresh per cycle by the discovery process." This maps to ADR-0005 §2.

DESIGN_BRIEF.md (proposed — the design as actually shipped):

## Cadence Settings
- settingsSave gains cadence_<provider> cases per provider class
  (ADR-0005 §2): ATS-class providers (greenhouse, lever, ashby, workday,
  icims, bamboohr, paylocity, join) accept 15–360 minutes; workable is the
  daily lane — the cadence is fixed, bounds [1440, 1440]; voyager is the
  slower class, bounds [120–1440]. Invalid values are rejected with 409 via
  semanticFailure.

## Discovery Cadence Table
- CADENCE_DEFAULTS_MIN at the top of ingest/discovery.mjs is the one-truth
  cadence table, staggered across the provider list order: greenhouse 30,
  lever 45, ashby 60, workday 60, icims 90, bamboohr 90, paylocity 120,
  join 120, workable 1440, voyager 240. Defaults are MINUTES between cycles
  and live in this one table.
- CADENCE_BOUNDS_MIN mirrors the settingsSave validation bounds: an
  out-of-bounds cadence_<provider> setting is treated as unset by discovery
  (defense in depth — the server rejects it at write time anyway). Both
  tables carry an ADR-0005 §2 cross-reference comment so they stay in sync.
- Provider iteration: allProviders() returns [...ATS_PROVIDERS, VOYAGER] —
  ATS_PROVIDERS is imported from ingest/scan.mjs (never re-declared);
  VOYAGER is declared as its own constant in discovery.mjs.
- Unset/invalid handling: each read site treats an out-of-bounds or unset
  cadence_<provider> setting as "use the CADENCE_DEFAULTS_MIN value" — the
  JS side falls back by its own bounds check against CADENCE_BOUNDS_MIN,
  and the scheduler sleeps per the lookup result.
- Per-provider sync state is written through the ingestctl CLI (Node never
  opens SQL), mirroring how ingest writes today.

## Existing codebase (commit ad8eb3403 — read these files):

### ingest/scan.mjs (relevant excerpt)
```js
// ATS_PROVIDERS: per-provider fan-out order for the scan step. The vendored
// career-ops scanner accepts a provider list.
export const ATS_PROVIDERS = [
  "greenhouse", "lever", "ashby", "workday", "icims", "bamboohr", "paylocity", "join",
];

export async function runScan(config, { voyager = runVoyagerScan, providers = ATS_PROVIDERS } = {}) {
```

### engine/internal/server/server.go — settingsSave (relevant excerpt)
```go
// settingsSave validates and persists one setting key:
// ranking_model non-empty + must have a bakeoffs row (409 otherwise);
// daily_apply_cap integer >= 0; auto_apply_threshold integer stored;
// auto_apply_enabled/longtail_enabled 0|1. Unknown keys → semanticFailure.
func (s *Server) settingsSave(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil { s.failure(w, err, "parse settings form"); return }
	key := r.FormValue("key")
	value := r.FormValue("value")
	switch key {
	case "ranking_model":
		// non-empty + HasBakeoffModel gate ...
	case "daily_apply_cap":
		if n, err := strconv.Atoi(value); err != nil || n < 0 { s.semanticFailure(w, "daily_apply_cap must be an integer >= 0"); return }
	case "auto_apply_enabled", "longtail_enabled":
		// saveBoolToggle with threshold gate ...
	case "notifications_smtp_port":
		if s.saveIntSetting(w, key, value, 1, 65535) { return }
	case "notifications_digest_hour":
		if s.saveIntSetting(w, key, value, 0, 23) { return }
	case "notifications_retry_max":
		if s.saveIntSetting(w, key, value, 0, math.MaxInt) { return }
	default:
		s.semanticFailure(w, "unknown setting "+key)
		return
	}
	s.st.SetSetting(key, value)
	...
}
```
(The full file at engine/internal/server/server.go, 862 lines, contains
`saveIntSetting` at line 704, `settingsSave` at line 722, and per-key
bounds via the cases above. notifier.go carries its own unset-semantics
defaults: retry_max unset → 3, digest hour unset → digest disabled.)

### engine/internal/store/settings.go (relevant excerpt)
```go
// GetSetting returns the value of one platform-setting key. A never-written
// key is ErrSettingNotFound, not "" — an unset and a blank value must not be
// confusable. Callers (rank.mjs via ingestctl) test for it with errors.Is.
func (s *Store) GetSetting(key string) (string, error) { ... }
func (s *Store) SetSetting(key, value string) error { ... }
```

### ingest/discovery.mjs — does not exist yet on this commit (this droplet creates it)
