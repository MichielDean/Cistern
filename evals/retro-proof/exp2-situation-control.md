# Exp2 situation — control: the post-fix design (PR #36 state)

Droplet: "Rework the settings surface: a single typed settings registry owns
every platform-setting key (kind, bounds, default, gate). settingsSave's
150-line switch becomes lookup + validate + persist. Cadence defaults/bounds
move into the registry; discovery's hand-synced JS tables are replaced by a
generated echo with a CI drift test against ATS_PROVIDERS."

DESIGN_BRIEF.md (proposed — the post-fix design):

## Settings Registry (one deep module)
- New package engine/internal/settings owns every key: kind, bounds, default,
  gate (closures where cross-key). settings.GetBool/GetInt/GetString encode
  "unset IS off" ONCE and are consumed by notifier, autogate and server —
  replacing per-package default constants.
- settingsSave becomes lookup + validate + persist; no per-key switch cases.
- Cadence defaults and bounds live ONLY in the registry (Go side); the
  per-provider entry derives its key set from the provider list.
- Codegen: scripts/gen-settings-echo.mjs parses the registry's rigid marker
  block and writes a checked-in echo ingest/settings-echo.generated.mjs
  exporting CADENCE_DEFAULTS_MIN, CADENCE_BOUNDS_MIN and REGISTRY_VERSION
  (sha of registry.go). discovery.mjs imports the echo instead of declaring
  its own tables.
- Provider list ownership: ATS_PROVIDERS in ingest/scan.mjs (ingest/scan.mjs:88)
  is the canonical owner of provider membership. Nothing else enumerates
  provider names: the registry keys cadence per provider by consuming the same
  list — scripts/gen-settings-echo.mjs is the single derivation point, parsing
  BOTH the registry block and scan.mjs's ATS_PROVIDERS. The CI drift test
  (settings-drift.test.mjs) imports the checked-in echo and scan.mjs directly
  and asserts membership equality (each provider exactly once on both sides).
  A stale echo that no longer matches scan.mjs fails CI.
- Echo freshness: the codegen also stamps REGISTRY_VERSION = sha of registry.go
  into the echo; the drift test recomputes the sha and fails on mismatch, so a
  checked-in echo that was never regenerated after a registry edit fails CI.
- Unset semantics JS-side: no JS default literals — discovery resolves unset/
  out-of-bounds values ONLY through the imported echo tables (whose values
  are the registry's by construction) plus ingestctl's getIntSetting/
  getBoolSetting helpers mirroring the Go getters' one unset interpretation.
  The mirror of unset semantics across languages is itself drift-tested: the
  same CI drift test imports ingestctl.mjs and the echo and asserts the JS
  helper defaults equal the echo (and therefore registry) defaults, so the
  two-language interpretation cannot silently diverge.
- Drift test (CI): asserts the generated provider cadence keys exactly equal
  the ATS_PROVIDERS membership used by discovery (each provider present
  exactly once on both sides); REGISTRY_VERSION changes whenever registry.go
  changes.
- ingestctl gains getIntSetting/getBoolSetting helpers mirroring the Go
  getters, so JS call sites share one unset interpretation.

## Existing codebase (post-fix excerpt):

### ingest/settings-echo.generated.mjs (checked in, generated)
```js
export const CADENCE_DEFAULTS_MIN = {
  greenhouse: 30, lever: 45, ashby: 60, workday: 60, icims: 90,
  bamboohr: 90, paylocity: 120, join: 120, workable: 1440, voyager: 240,
};
export const CADENCE_BOUNDS_MIN = {
  greenhouse: [15, 360], lever: [15, 360], ...
  workable: [1440, 1440],
  voyager: [120, 1440],
};
export const REGISTRY_VERSION = "a91be23...";
```

### engine/internal/settings/registry.go (relevant excerpt)
```go
// Package settings: one typed table of every platform-setting key — kind,
// bounds, default, gate. An unset key's meaning ("unset IS off", per-key
// default) lives only in this package's getters.
func GetInt(st storeSettingReader, key string) (int, bool, error) { ... }
func GetBool(st storeSettingReader, key string) (bool, bool, error) { ... }
// Per-provider cadence entries carry {kind: int, bounds: [15,360], default: 30}
// generated-side echo refresh: scripts/gen-settings-echo.mjs
```

### scripts/gen-settings-echo.mjs — RIGID parse of registry.go marker block
(// BEGIN SETTINGS-ECHO ... // END), writes ingest/settings-echo.generated.mjs.
