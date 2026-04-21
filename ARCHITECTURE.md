# Architecture: Web UI — Infrastructure Pages

## Overview

Four new operator-facing pages (Castellarius, Doctor, Logs, Repos/Skills) plus
a shared component library and a Go SSE log-streaming handler.

---

## 1. TypeScript Types — `web/src/api/types.ts` (additions)

```ts
// ── Castellarius ──

export interface AqueductStatus {
  name: string;
  status: 'idle' | 'flowing';
  droplet_id: string | null;
  droplet_title: string | null;
  current_step: string | null;
  elapsed: number;           // nanoseconds
}

export interface CastellariusStatus {
  running: boolean;
  pid: number | null;
  uptime_seconds: number | null;
  aqueducts: AqueductStatus[];
  castellarius_running: boolean;
}

// ── Doctor ──

export type DoctorCheckStatus = 'pass' | 'fail' | 'warn';

export interface DoctorCheck {
  name: string;
  status: DoctorCheckStatus;
  message: string;
  category: string;
}

export interface DoctorResult {
  checks: DoctorCheck[];
  summary: { total: number; passed: number };
  timestamp: string;
}

// ── Logs ──

export interface LogEntry {
  line: number;
  level: 'INFO' | 'WARN' | 'ERROR' | 'DEBUG' | '';
  text: string;
  raw: string;
}

export interface LogSourceInfo {
  name: string;
  path: string;
  size_bytes: number;
  last_modified: string;
}

// ── Repos & Skills ──

export interface RepoInfo {
  name: string;
  prefix: string;
  url: string;
  aqueduct_config: string | null;
  aqueducts: AqueductBrief[];
}

export interface AqueductBrief {
  name: string;
  steps: string[];
}

export interface SkillInfo {
  name: string;
  source_url: string;
  installed_at: string;
}
```

---

## 2. API Client Modules

One file per domain, all under `web/src/api/`. The pattern follows
`useAuth.ts`: each module uses `getAuthHeaders()` for authenticated fetch
and exports async functions + a React hook.

### `web/src/api/castellarius.ts`

```ts
import { getAuthHeaders } from '../hooks/useAuth';
import type { CastellariusStatus } from './types';

export async function fetchCastellariusStatus(): Promise<CastellariusStatus> {
  const resp = await fetch('/api/castellarius/status', { headers: getAuthHeaders() });
  if (!resp.ok) throw new Error(`castellarius status: ${resp.status}`);
  return resp.json();
}

export async function castellariusAction(action: 'start' | 'stop' | 'restart'): Promise<void> {
  // restart requires confirmation at the UI level, not here
  const resp = await fetch(`/api/castellarius/${action}`, {
    method: 'POST',
    headers: getAuthHeaders(),
  });
  if (!resp.ok) {
    const body = await resp.json().catch(() => ({ message: resp.statusText }));
    throw new Error(body.message || `castellarius ${action} failed: ${resp.status}`);
  }
}

export function useCastellariusStatus(intervalMs = 5000) {
  // Returns { status, loading, error, refresh }
  // Auto-fetches on mount and polls at intervalMs.
  // Castellarius page disables auto-poll when unmounted.
}
```

### `web/src/api/doctor.ts`

```ts
import { getAuthHeaders } from '../hooks/useAuth';
import type { DoctorResult } from './types';

export async function fetchDoctor(fix = false): Promise<DoctorResult> {
  const url = fix ? '/api/doctor?fix=true' : '/api/doctor';
  const resp = await fetch(url, { headers: getAuthHeaders() });
  if (!resp.ok) throw new Error(`doctor: ${resp.status}`);
  return resp.json();
}

export function useDoctor() {
  // Returns { result, loading, error, rerun, fix }
  // No auto-poll — manual refresh only.
}
```

### `web/src/api/logs.ts`

```ts
import { getAuthParams, getAuthHeaders } from '../hooks/useAuth';
import type { LogSourceInfo } from './types';

export async function fetchLogHistory(
  lines = 500,
  source = 'castellarius'
): Promise<string[]> {
  const auth = getAuthParams();
  const url = auth
    ? `/api/logs?lines=${lines}&source=${source}&${auth}`
    : `/api/logs?lines=${lines}&source=${source}`;
  const resp = await fetch(url, { headers: getAuthHeaders() });
  if (!resp.ok) throw new Error(`logs: ${resp.status}`);
  return resp.json();
}

export function createLogEventSource(
  source: string,
  onLine: (line: string) => void,
  onError: (err: Error) => void,
): EventSource {
  const auth = getAuthParams();
  const url = auth
    ? `/api/logs/events?source=${source}&${auth}`
    : `/api/logs/events?source=${source}`;
  const es = new EventSource(url);
  es.onmessage = (e) => onLine(e.data);
  es.onerror = () => { onError(new Error('log stream error')); es.close(); };
  return es;
}

export async function fetchLogSources(): Promise<LogSourceInfo[]> {
  const resp = await fetch('/api/logs/sources', { headers: getAuthHeaders() });
  if (!resp.ok) throw new Error(`log sources: ${resp.status}`);
  return resp.json();
}
```

### `web/src/api/repos.ts`

```ts
import { getAuthHeaders } from '../hooks/useAuth';
import type { RepoInfo, SkillInfo } from './types';

export async function fetchRepos(): Promise<RepoInfo[]> {
  const resp = await fetch('/api/repos', { headers: getAuthHeaders() });
  if (!resp.ok) throw new Error(`repos: ${resp.status}`);
  return resp.json();
}

export async function fetchSkills(): Promise<SkillInfo[]> {
  const resp = await fetch('/api/skills', { headers: getAuthHeaders() });
  if (!resp.ok) throw new Error(`skills: ${resp.status}`);
  return resp.json();
}
```

---

## 3. Shared Components

All under `web/src/components/`. Export from `index.ts`.

### `StatusIndicator.tsx`

A reusable running/stopped/error indicator.

```
Props:
  status: 'running' | 'stopped' | 'error'
  label: string
  size?: 'sm' | 'lg'   (default 'sm')

Rendering:
  - running → green circle with pulse-glow CSS animation, green label
  - stopped → muted/gray circle, muted label
  - error   → red circle, red label
  - lg variant → bigger circle (w-4 h-4 vs w-2 h-2), larger text

Uses existing tailwind colors: cistern-green, cistern-muted, cistern-red.
Uses existing pulse-glow CSS class from index.css.
```

### `ActionButton.tsx`

Consistent styled button for control actions with loading/error state.

```
Props:
  label: string
  onClick: () => Promise<void>
  variant?: 'primary' | 'danger' | 'default'   (default 'default')
  disabled?: boolean
  icon?: ReactNode                (optional inline SVG)
  confirm?: string                (if set, click shows confirm dialog before executing)

Rendering:
  - primary  → bg-cistern-accent text-cistern-bg
  - danger   → bg-cistern-red text-cistern-bg
  - default  → border border-cistern-border text-cistern-fg hover:bg-cistern-border/30
  - disabled → opacity-50 cursor-not-allowed
  - loading  → shows spinner SVG icon, onClick disabled
  - confirm  → window.confirm(confirm) before executing onClick
  - All variants: font-mono text-sm px-3 py-1.5 rounded-md transition-colors
```

### `LogViewer.tsx`

Terminal-style log display component.

```
Props:
  entries: LogEntry[]
  autoScroll?: boolean               (default true)
  onAutoScrollChange?: (v: boolean)  // callback when user scrolls up
  maxHeight?: string                  (default '100%')
  searchQuery?: string                // highlight matches

Rendering:
  - Monospace font (font-mono)
  - Dark theme: bg-cistern-bg text-cistern-fg
  - Line numbers in gutter: text-cistern-muted text-right pr-2 select-none
  - Log level color coding:
      INFO  → text-cyan-400
      WARN  → text-cistern-yellow
      ERROR → text-cistern-red
      DEBUG → text-cistern-muted/50
  - Search: filter entries whose raw text includes query (case-insensitive).
    Highlight matching substring with bg-cistern-accent/30.
  - Auto-scroll: when true, useEffect scrolls container to bottom on every
    new entry. When user scrolls up (onScroll handler detects scrollTop < 
    scrollHeight - clientHeight - 40), disable auto-scroll and call 
    onAutoScrollChange(false).
  - Container uses overflow-y-auto with the overflow-anchor CSS property
    for smooth bottom-anchoring.
```

### `HealthCheckCard.tsx`

Displays a single doctor check result.

```
Props:
  check: DoctorCheck

Rendering:
  - Card: bg-cistern-surface border border-cistern-border rounded-lg p-3
  - Left: status icon (green ✓ / red ✕ / yellow ⚠)
  - Right: check name (font-mono text-sm text-cistern-fg)
  - Below: result message (text-xs text-cistern-muted)
  - If status=fail: red left border accent (border-l-2 border-cistern-red)
  - If status=warn: yellow left border accent (border-l-2 border-cistern-yellow)
```

---

## 4. Page Components

### 4a. `CastellariusPage.tsx` — `/app/castellarius`

Layout (single scrollable column):

```
┌──────────────────────────────────────┐
│ Status Indicator                     │
│  [running/stopped] Castellarius      │
│  PID: 12345 · Uptime: 2h15m         │
│                                      │
│ [Start] [Stop] [Restart]             │
└──────────────────────────────────────┘

┌──────────────────────────────────────┐
│ Aqueducts                            │
│ ┌────────────────────────────────┐   │
│ │ name       status   current    │   │
│ │ main       flowing  ci-abc1   │   │
│ │            step: implementer   │   │
│ │            elapsed: 12:45      │   │
│ └────────────────────────────────┘   │
│ ┌────────────────────────────────┐   │
│ │ docs        idle               │   │
│ └────────────────────────────────┘   │
└──────────────────────────────────────┘

Castellarius Watching: [watching/stopped]
```

Hook: `useCastellariusStatus(5000)` — polls every 5 seconds.

Control buttons:
- Start: enabled only when `!status.running`, variant='primary'
- Stop: enabled only when `status.running`, variant='danger'
- Restart: enabled only when `status.running`, variant='danger', confirm="Restart Castellarius? Active droplets will be interrupted."

Each button click → `castellariusAction()` → spinner during execution → toast on success/failure. Toast implemented as a simple `<div>` with auto-dismiss (3s), styled `bg-cistern-surface border border-cistern-border rounded-lg p-3 fixed bottom-4 right-4`.

Each aqueduct row:
- Uses existing `AqueductArch` component (mini mode) for active aqueducts
- Clicking an aqueduct name links to the dashboard (`/app/`) with the aqueduct highlighted

### 4b. `DoctorPage.tsx` — `/app/doctor`

Layout:

```
┌──────────────────────────────────────┐
│ Health Checks     [Re-run] [Fix]     │
│ Summary: 4/5 passed                 │
│ Last checked: 2025-01-15 14:30 UTC   │
└──────────────────────────────────────┘

[Category cards grouped by check.category]
┌──────────────────────────────────────┐
│ Daemon                               │
│ ✓ Daemon running    Running (pid 123)│
│ ✕ Config valid      Missing field X  │
└──────────────────────────────────────┘
```

Hook: `useDoctor()` — fetches on mount, no auto-poll.
Actions: Re-run → `fetchDoctor()`, Fix → `fetchDoctor(true)`.

Summary card at top:
- `text-cistern-green` if all pass
- `text-cistern-yellow` if any warn
- `text-cistern-red` if any fail

Checks grouped by `check.category`, each category a card containing
`HealthCheckCard` rows.

### 4c. `LogsPage.tsx` — `/app/logs`

Layout:

```
┌──────────────────────────────────────┐
│ Source: [▼ castellarius]  Size: 2.1MB │
│ Last modified: 2025-01-15 14:30      │
└──────────────────────────────────────┘

┌──────────────────────────────────────┐
│ [🔍 Filter] [Auto-scroll ✓] [Clear]  │
├──────────────────────────────────────┤
│  1 │ 2025-01-15 14:30:01 INFO  ... │
│  2 │ 2025-01-15 14:30:02 WARN  ... │
│  3 │ 2025-01-15 14:30:03 ERROR ... │
└──────────────────────────────────────┘
```

Data flow:
1. On mount: `fetchLogHistory(500, activeSource)` → parse lines into 
   `LogEntry[]` with line numbers and level extraction.
2. Open SSE: `createLogEventSource(activeSource, onLine, onError)` 
   → append new entries, auto-scroll if enabled.
3. Source selector dropdown: `fetchLogSources()` for metadata (name, size, modified).
   On change → close current SSE, re-fetch history, open new SSE.

Search/filter: simple input field, filters entries whose `raw` includes the
query (case-insensitive). The LogViewer component handles highlight.

### 4d. `ReposSkillsPage.tsx` — `/app/repos`

Layout:

```
## Repositories

┌─────────────────────────────────────────┐
│ my-app                                  │
│ Prefix: myapp | URL: github.com/org/... │
│ Aqueducts:                               │
│   · main: architect → implementer → ... │
│   · docs: docs-writer                   │
└─────────────────────────────────────────┘

## Skills

┌──────────────────────────────────────────┐
│ Name           Source         Installed   │
│ cistern-signaling  /skills/...  2025-01-10│
│ cistern-git        /skills/...  2025-01-10│
└──────────────────────────────────────────┘
```

Two sections: Repos (card-based) and Skills (simple table).

Repos: `fetchRepos()` on mount. Each repo rendered as a card with:
- Name (font-mono font-bold text-cistern-fg)
- Prefix, URL (linked, opens in new tab)
- Aqueducts listed with step chain shown inline

Skills: `fetchSkills()` on mount. Rendered as a striped table with columns:
Name, Source URL, Installed date.

---

## 5. Routing Updates — `web/src/main.tsx`

Replace the 5 placeholder routes with actual page imports:

```tsx
import { CastellariusPage } from './pages/CastellariusPage';
import { DoctorPage } from './pages/DoctorPage';
import { LogsPage } from './pages/LogsPage';
import { ReposSkillsPage } from './pages/ReposSkillsPage';

// In children array, replace:
{ path: 'castellarius', element: <CastellariusPage /> },
{ path: 'doctor', element: <DoctorPage /> },
{ path: 'logs', element: <LogsPage /> },
{ path: 'repos', element: <ReposSkillsPage /> },
```

`PlaceholderPage` stays for `droplets` route.

---

## 6. Component Index Update — `web/src/components/index.ts`

Add exports:
```ts
export { StatusIndicator } from './StatusIndicator';
export { ActionButton } from './ActionButton';
export { LogViewer } from './LogViewer';
export { HealthCheckCard } from './HealthCheckCard';
```

---

## 7. Go Server — Log Streaming Handlers

New handlers in `cmd/ct/dashboard_web.go`, registered in `newDashboardMuxInternalWith`.

### `GET /api/logs`

```go
func handleGetLogs(cfgPath string) http.HandlerFunc {
    // Query params: ?lines=500&source=castellarius
    // Read last N lines from the log file.
    // Default source: castellarius → ~/.cistern/castellarius.log
    // Returns JSON array of strings (log lines).
}
```

### `GET /api/logs/events` (SSE stream)

```go
func handleLogEvents(cfgPath string) http.HandlerFunc {
    // Query params: ?source=castellarius
    // Follows the same pattern as handleDashboardEvents:
    //   - SSE headers, flush support check, connection limit
    //   - Tail the log file using fsnotify or polling (fallback: poll every 500ms)
    //   - On new data, send each line as 'data: <line>\n\n'
    //   - Context cancellation for cleanup
}
```

### `GET /api/logs/sources`

```go
func handleGetLogSources(cfgPath string) http.HandlerFunc {
    // Returns JSON array of LogSourceInfo objects.
    // Currently just castellarius.log, but extensible.
    // Each entry: { name, path, size_bytes, last_modified }
}
```

### Route registration (add near line 1073)

```go
// Logs
mux.HandleFunc("GET /api/logs", handleGetLogs(cfgPath))
mux.HandleFunc("GET /api/logs/events", handleLogEvents(cfgPath))
mux.HandleFunc("GET /api/logs/sources", handleGetLogSources(cfgPath))
```

---

## 8. Castellarius Status Enhancement (Go)

The current `handleCastellariusStatus()` returns only `{ "status": "ok" }`.
It needs to be enhanced (droplet 1's responsibility) to return the full
`CastellariusStatus` struct. The frontend is designed against the full
response shape defined in types.ts. If the backend still returns the stub,
the page should gracefully degrade (show "Running" as unknown, empty
aqueducts table).

The frontend should handle both the stub response and the full response:

```ts
// In useCastellariusStatus:
const running = status?.running ?? (status?.status === 'ok');
```

---

## 9. Doctor Handler Enhancement (Go)

The current `handleDoctor()` returns `{ config_ok: true, repos: [...] }`.
The frontend expects the `DoctorResult` shape with individual check objects
grouped by category. Until the Go handler is fully implemented, the frontend
page should transform the stub response into the expected format:

```ts
// Fallback: if response has config_ok, synthesize a single check
if (result.checks === undefined) {
  result = {
    checks: [{ name: 'Config Valid', status: data.config_ok ? 'pass' : 'fail', message: '...', category: 'Config' }],
    summary: { total: 1, passed: data.config_ok ? 1 : 0 },
    timestamp: new Date().toISOString(),
  };
}
```

---

## 10. File Manifest

### New files

| File | Purpose |
|------|---------|
| `web/src/api/castellarius.ts` | API client + hook for Castellarius |
| `web/src/api/doctor.ts` | API client + hook for Doctor |
| `web/src/api/logs.ts` | API client + SSE hook for Logs |
| `web/src/api/repos.ts` | API client for Repos & Skills |
| `web/src/components/StatusIndicator.tsx` | Running/stopped/error pulse dot |
| `web/src/components/ActionButton.tsx` | Styled button with loading/confirm |
| `web/src/components/LogViewer.tsx` | Terminal log viewer component |
| `web/src/components/HealthCheckCard.tsx` | Doctor check row component |
| `web/src/pages/CastellariusPage.tsx` | Castellarius control page |
| `web/src/pages/DoctorPage.tsx` | Health check page |
| `web/src/pages/LogsPage.tsx` | Log viewer page |
| `web/src/pages/ReposSkillsPage.tsx` | Repos & Skills display page |

### Modified files

| File | Change |
|------|--------|
| `web/src/api/types.ts` | Add CastellariusStatus, AqueductStatus, DoctorCheck, DoctorResult, LogEntry, LogSourceInfo, RepoInfo, AqueductBrief, SkillInfo |
| `web/src/components/index.ts` | Add 4 new component exports |
| `web/src/main.tsx` | Replace 4 placeholder routes with actual page components |
| `cmd/ct/dashboard_web.go` | Add handleGetLogs, handleLogEvents, handleGetLogSources handlers + route registration |

### No changes

| File | Reason |
|------|--------|
| `web/src/pages/Placeholder.tsx` | Kept for droplets route |
| `web/src/components/Sidebar.tsx` | Nav items already in place |
| `web/src/App.tsx` | Layout unchanged |

---

## 11. Design Principles

1. **Existing patterns**: Follow the functional component + named-export + tailwind
   pattern established by Dashboard, AqueductArch, etc. No default exports.
   No component libraries.

2. **Auth**: All API calls use `getAuthHeaders()` for REST and `getAuthParams()`
   for SSE query strings. No new auth logic needed.

3. **Error handling**: Each page shows a centered error state if the initial
   fetch fails (matching Dashboard's pattern). Toasts for action feedback
   only (Castellarius actions).

4. **Loading states**: Every page shows "Loading…" until data arrives.
   ActionButton handles its own loading spinner during async operations.

5. **No external dependencies**: No new npm packages. Build on React + Tailwind
   + react-router-dom (already installed).

6. **Graceful degradation**: Each page handles stub backend responses
   gracefully (see sections 8 & 9) and shows partial data rather than crashing.

7. **Responsive**: All pages use the existing sidebar + main layout. Content
   uses `grid` and `flex-wrap` for responsive adaptation, matching Dashboard
   patterns (e.g., `grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3`).

8. **Cistern palette**: All colors come from the `cistern-*` tailwind theme.
   No custom color values. Font is always `font-mono` for data and labels,
   `font-mono` for headings (matching Dashboard).