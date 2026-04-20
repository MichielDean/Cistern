# Design Brief: Web UI — Droplet creation, edit, and issues

## Requirements Summary

Build the droplet creation form (`/app/droplets/new`), enhanced metadata editing with diff confirmation on the detail page, a rename quick-action with inline editing, full issue management (filing, resolving, rejecting, filtering, sorting), and close/reopen actions with confirmation dialogs. This completes the CRUD surface for droplets in the Cistern web UI.

## Existing Patterns to Follow

### React / Component Patterns

- **Functional components only.** Every component in `web/src/components/` and `web/src/pages/` is a named export from a functional component — see `StatusBadge.tsx:14`, `ActionDialog.tsx:16`, `AddNoteModal.tsx:11`, `RestartModal.tsx:12`, `EditMetadataModal.tsx:12`, `IssuesList.tsx:13`.
- **Named exports**, not default exports. Every component uses `export function ComponentName` — see `DropletDetail.tsx:26`, `DropletsList.tsx:23`, `AddNoteModal.tsx:11`, `StatusBadge.tsx:14`.
- **Barrel re-exports** from `web/src/components/index.ts` — every component is re-exported there. New components must be added to this file.
- **Props as typed interface** defined immediately before the component function — see `ActionDialog.tsx:4-14`, `AddNoteModal.tsx:4-9`, `EditMetadataModal.tsx:6-10`, `IssuesList.tsx:6-11`.

### API / Data Fetching

- **`apiFetch<T>` helper** at `web/src/hooks/useApi.ts:18-29` wraps `fetch` with auth headers, error handling (`API error ${status}: ${body}`), and 204 handling. ALL API calls use this helper.
- **`useXxx` hooks** return `{ data, loading, error }` for reads — see `useDroplets`, `useDroplet`, `useDropletIssues`, `useDropletDependencies`, `useRepos`, `useRepoSteps`.
- **Mutation functions** are standalone async exports (not hooks) for one-shot operations — see `addNote`, `editDroplet`, `createDroplet`, `addIssue`, `resolveIssue`, `rejectIssue`, `addDependency`, `removeDependency` in `web/src/hooks/useApi.ts:300-354`.
- **`useDropletMutation`** hook at `web/src/hooks/useApi.ts:250-263` wraps action POSTs (`pass`, `recirculate`, `pool`, etc.) with `mutate(id, action, body?)`.
- **Type definitions** live in `web/src/api/types.ts`. Every API response and request body is typed as an `interface` with snake_case for JSON fields (matching Go backend JSON tags) and camelCase in TypeScript — see `DropletIssue.flagged_by`, `CreateDropletRequest.depends_on`.

### Styling / UI

- **Tailwind CSS** with custom `cistern-*` color tokens — `cistern-bg`, `cistern-surface`, `cistern-border`, `cistern-fg`, `cistern-muted`, `cistern-accent`, `cistern-green`, `cistern-red`, `cistern-yellow`. See `StatusBadge.tsx:7-12`, `IssuesList.tsx:68-71`, `ActionDialog.tsx:53-54`.
- **Font:** `font-mono` used for IDs, labels, headings, and badges throughout — see `DropletDetail.tsx:113`, `IssuesList.tsx:73`.
- **Modal overlay pattern:** `fixed inset-0 bg-black/60 flex items-center justify-center z-50` for backdrop, `bg-cistern-surface border border-cistern-border rounded-lg p-6 max-w-md w-full mx-4` for modal body, `onClick={onClose}` on backdrop, `onClick={(e) => e.stopPropagation()}` on modal body — see `ActionDialog.tsx:52-53`, `AddNoteModal.tsx:43-44`, `RestartModal.tsx:44-45`, `EditMetadataModal.tsx:53-54`, `IssuesList.tsx:99-100`, `DropletDetail.tsx:296-298`.
- **Form input styling:** `bg-cistern-bg border border-cistern-border rounded px-2 py-1.5 text-sm text-cistern-fg` for inputs, `resize-y min-h-[80px]` for textareas — see `AddNoteModal.tsx:49-50`, `EditMetadataModal.tsx:86-89`, `IssuesList.tsx:104-107`.
- **Button styles:** primary = `bg-cistern-accent text-cistern-bg font-medium disabled:opacity-50`, secondary/cancel = `border border-cistern-border text-cistern-muted hover:text-cistern-fg`, danger = `bg-cistern-red text-white` — see `ActionDialog.tsx:95-102`, `AddNoteModal.tsx:55-56`, `IssuesList.tsx:310`.
- **Label styling:** `block text-xs font-mono text-cistern-muted uppercase tracking-wider mb-1` — see `EditMetadataModal.tsx:59`, `ActionDialog.tsx:61`, `RestartModal.tsx:52`.
- **Section card pattern:** `bg-cistern-surface border border-cistern-border rounded-lg p-4` — see `DropletDetail.tsx:160`, `181`, `190`, `209`.
- **Section heading:** `text-sm font-mono text-cistern-muted uppercase tracking-wider mb-3` or `mb-2` — see `DropletDetail.tsx:161`, `184`, `192`.

### Routing

- **React Router v6** with `createBrowserRouter` in `web/src/main.tsx:11-24`. New route: `{ path: 'droplets/new', element: <CreateDroplet /> }` as a child of the `/app` layout.
- Routes are defined as a flat array of relative paths — see `main.tsx:16-23`.

### Error Handling

- **API errors:** `apiFetch` throws `Error('API error ${status}: ${body}')` — `useApi.ts:24`. Components catch and display as `text-sm text-cistern-red font-mono` — see `EditMetadataModal.tsx:94`, `AddNoteModal.tsx:54`, `IssuesList.tsx:109`, `DropletDetail.tsx:268-289`.
- **Loading states:** `text-center py-4 text-cistern-muted font-mono text-sm` with ellipsis — see `IssuesList.tsx:29`, `DependenciesList.tsx:43-44`, `DropletDetail.tsx:91-94`.
- **Empty states:** `text-center py-4 text-cistern-muted font-mono text-sm` — see `IssuesList.tsx:62`.

### State Management in Components

- **Modal pattern:** `const [showXxxModal, setShowXxxModal] = useState(false)` in the parent, passed as `open`/`onClose` props — see `DropletDetail.tsx:50-53`, `EditMetadataModal.tsx:8-9`.
- **Form reset on open:** `useEffect` that resets form state when `open` flips to `true` — see `EditMetadataModal.tsx:20-29`, `AddNoteModal.tsx:16-22`, `RestartModal.tsx:17-24`, `ActionDialog.tsx:22-29`.
- **Submitting guard:** `const [submitting, setSubmitting] = useState(false)` with `disabled={submitting}` on submit button — universal pattern.

### Naming Conventions

- **Files:** PascalCase component files matching component name — `EditMetadataModal.tsx`, `AddNoteModal.tsx`, `ActionDialog.tsx`, `RestartModal.tsx`, `IssuesList.tsx`, `StatusBadge.tsx`.
- **Component names:** PascalCase matching filename — `EditMetadataModal`, `AddNoteModal`, `ActionDialog`.
- **Hooks:** `useXxx` prefix — `useDroplets`, `useDroplet`, `useDropletIssues`, `useSearchDroplets`.
- **API function names:** camelCase verbs — `addNote`, `editDroplet`, `createDroplet`, `addIssue`, `resolveIssue`.

### Collection Types

- Arrays are used throughout — `Droplet[]`, `DropletIssue[]`, `DropletDependency[]`, `CataractaeNote[]`, `string[]` (for steps). No `Set` or `Map` usage in frontend code.

### Testing

- **Vitest** with `@testing-library/react` and `@testing-library/jest-dom` — `web/vitest.config.ts`, `web/package.json:10,29-32`.
- **Setup file:** `web/src/test/setup.ts:1` imports `@testing-library/jest-dom`.
- **Test location:** `web/src/__tests__/` with `.tsx` or `.ts` extensions matching the component name — e.g., `EditMetadataModal.test.tsx`, `ActionDialog.test.tsx`, `RestartModal.test.tsx`, `useApi.test.ts`.
- **Test pattern:** `describe('ComponentName', () => { ... })` with `it('description', ...)` — see `EditMetadataModal.test.tsx:29`, `ActionDialog.test.tsx:5`, `RestartModal.test.tsx:22`.
- **Mock fetch pattern:** `mockFetch` helper returning `{ ok, status, json(), text() }` — see `EditMetadataModal.test.tsx:20-27`, `useApi.test.ts:24-31`.
- **BeforeEach/afterEach:** `localStorage.clear()` + `vi.restoreAllMocks()` — universal pattern in test files.
- **Last fetch body helper:** `getLastFetchBody()` in `RestartModal.test.tsx:14-20`.
- **Imports:** `import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'` and `import { render, screen, fireEvent, act } from '@testing-library/react'`.

## Reusability Requirements

### New components and their reusability

| Component | Reusable? | Reason |
|-----------|-----------|--------|
| `CreateDropletForm.tsx` | Yes — used by the page route | Form logic encapsulated; page just renders it |
| `ComplexitySelector.tsx` | Yes — used by both `CreateDropletForm` and `EditMetadataModal` | Same radio group pattern for complexity in both create and edit contexts |
| `RenameInput.tsx` | Specific to `DropletDetail` | Inline editable title is detail-page only |
| `FileIssueModal.tsx` | Yes — replaces inline `FileIssueButton` in `DropletDetail.tsx:263-317` | Extract the existing inline modal into a proper component |
| `IssueCard.tsx` | Yes — used by `IssuesList` | Card rendering for a single issue |
| `IssueFilters.tsx` | Yes — used by `IssuesList` | Filter/sort controls extracted from `IssuesList` |
| `CloseModal.tsx` | Specific to `DropletDetail` | Confirmation dialog for close action |
| `ReopenModal.tsx` | Specific to `DropletDetail` | Confirmation dialog for reopen action |

### ComplexitySelector — must be shared

The complexity radio group appears in both `CreateDropletForm` and `EditMetadataModal`. The current `EditMetadataModal` uses a raw number input for complexity (`EditMetadataModal.tsx:80-82`). The enhanced version must replace this with a shared `ComplexitySelector` component that includes stage visualization. Both `CreateDropletForm` and `EditMetadataModal` must import and render the same `ComplexitySelector`.

## Coupling Requirements

- No shared mutable package-level state. All state lives in React component state or hooks.
- API hooks (`useApi.ts`) are the single source of truth for data fetching. New mutation functions (`renameDroplet`, `closeDroplet`, `reopenDroplet`) must follow the existing standalone async function pattern (like `editDroplet`, `addNote`) — not wrapped in a hook unless they need reactive state.
- `ComplexitySelector` must accept its value and onChange as props, not fetch pipeline steps internally. The parent provides steps data (already available via `useRepoSteps` in `DropletDetail`).

## DRY Requirements

### Modal overlay pattern (5+ occurrences)

The backdrop + centered card pattern appears in:
1. `web/src/components/ActionDialog.tsx:52-53`
2. `web/src/components/AddNoteModal.tsx:43-44`
3. `web/src/components/RestartModal.tsx:44-45`
4. `web/src/components/EditMetadataModal.tsx:53-54`
5. `web/src/components/IssuesList.tsx:99-100`

Extract a `ModalOverlay` component:
```tsx
interface ModalOverlayProps {
  open: boolean;
  onClose: () => void;
  children: React.ReactNode;
}
```
Replace the `fixed inset-0 bg-black/60 ...` pattern in all 5 locations plus all new modals (CloseModal, ReopenModal, FileIssueModal).

### Form state reset pattern (5+ occurrences)

The `useEffect(() => { if (open) { reset state... } }, [open])` pattern appears in:
1. `web/src/components/EditMetadataModal.tsx:20-29`
2. `web/src/components/AddNoteModal.tsx:16-22`
3. `web/src/components/RestartModal.tsx:17-24`
4. `web/src/components/ActionDialog.tsx:22-29`
5. `web/src/components/IssuesList.tsx:269-275` (inline in `FileIssueButton`)

This pattern is acceptable as-is — each component has different state to reset. Do NOT extract a generic hook; the DRY violation is in the modal overlay JSX, not the reset logic.

### Error display pattern

The `text-sm text-cistern-red font-mono` error display appears in every modal. This is a minor pattern but not a named helper candidate — it's a single className string, not 5+ lines of repeated code.

### Flagged-by color mapping

The `flagged_by` badge in `IssuesList.tsx:73` currently renders `issue.flagged_by` as plain text with `text-cistern-accent`. The requirements specify color-coded badges by role. Extract a `flaggedByBadgeColor(flagged_by: string): string` utility if more than 2 locations need it (currently only `IssuesList.tsx` renders this). Since the enhanced `IssueCard` and `FileIssueModal` will also display flagged-by, extract it as a utility:
```ts
// web/src/utils/issueColors.ts
export function flaggedByBadgeClasses(flaggedBy: string): string { ... }
```

## Migration Requirements

Not applicable — this is a frontend-only change. No database migrations are involved.

## Test Requirements

### New test files

| Test file | What it covers |
|-----------|----------------|
| `web/src/__tests__/CreateDropletForm.test.tsx` | Renders form with repo dropdown, validates title required + repo required, submits `createDroplet()` with correct payload, cancels navigation, shows loading state |
| `web/src/__tests__/ComplexitySelector.test.tsx` | Renders three radio options, shows pipeline stages for each complexity level, calls onChange with selected value |
| `web/src/__tests__/RenameInput.test.tsx` | Shows title as text, switches to input on click, saves on Enter, cancels on Escape, calls `renameDroplet()` API |
| `web/src/__tests__/FileIssueModal.test.tsx` | Opens/closes, validates description required, submits `addIssue()` with flagged_by, shows error on failure |
| `web/src/__tests__/IssueCard.test.tsx` | Renders issue with all fields, shows resolve/reject buttons for open issues, shows evidence for resolved/rejected, opens resolve modal, opens reject modal |
| `web/src/__tests__/IssueFilters.test.tsx` | Toggles open/all filter, filters by flagged_by role dropdown, sorts newest/oldest |
| `web/src/__tests__/CloseModal.test.tsx` | Opens/closes, shows confirmation warning, calls close API on confirm |
| `web/src/__tests__/ReopenModal.test.tsx` | Opens/closes, shows confirmation, calls reopen API on confirm |
| `web/src/__tests__/ModalOverlay.test.tsx` | Renders nothing when `open=false`, renders children when `open=true`, calls `onClose` on backdrop click, does not call `onClose` on content click |

### Updated test files

| Test file | What changes |
|-----------|-------------|
| `web/src/__tests__/EditMetadataModal.test.tsx` | Add test for diff confirmation display before save, test that `ComplexitySelector` renders instead of raw number input |
| `web/src/__tests__/useApi.test.ts` | Add tests for `renameDroplet`, `closeDroplet`, `reopenDroplet` mutation functions |

### Specific test cases

- `CreateDropletForm.test.tsx`: `it('shows validation error when title is empty on submit')`, `it('shows validation error when repo is empty on submit')`, `it('submits createDroplet with correct payload and redirects')`, `it('cancels and navigates back to droplets list')`, `it('renders repo dropdown from useRepos')`, `it('renders complexity selector with pipeline stage descriptions')`, `it('shows dependencies multi-select with type-ahead search')`
- `ComplexitySelector.test.tsx`: `it('renders three complexity options with stage descriptions')`, `it('calls onChange when selecting a complexity level')`, `it('shows standard pipeline stages for complexity 1')`, `it('shows full pipeline stages for complexity 2')`, `it('shows critical pipeline stages for complexity 3')`
- `RenameInput.test.tsx`: `it('renders title as text by default')`, `it('switches to input mode on click')`, `it('saves on Enter key')`, `it('cancels on Escape key')`, `it('saves on blur')`, `it('calls renameDroplet API on save')`
- `FileIssueModal.test.tsx`: `it('does not submit when description is empty')`, `it('submits addIssue with description and flagged_by')`, `it('shows default flagged_by as empty/all')`, `it('displays error on API failure')`
- `CloseModal.test.tsx`: `it('calls useDropletMutation mutate with close action')`, `it('displays confirmation warning text')`
- `ReopenModal.test.tsx`: `it('calls useDropletMutation mutate with reopen action')`, `it('displays confirmation text')`

## Forbidden Patterns

- **Inline modal JSX in parent components.** The `FileIssueButton` pattern in `DropletDetail.tsx:263-317` embeds a full modal inside a parent component. New modals MUST be separate component files with `open`/`onClose` props, matching `AddNoteModal.tsx`, `RestartModal.tsx`, and `EditMetadataModal.tsx`.
- **Raw `<input type="number">` for complexity.** The current `EditMetadataModal.tsx:80-82` uses a number input for complexity. This must be replaced by the shared `ComplexitySelector` radio group in both create and edit contexts.
- **Unprotected navigation on form submit.** When submitting `createDroplet`, the form must handle errors before navigating. Do not navigate on failure.
- **State derived from props without reset.** If a component receives `droplet` props and initializes state from them, it MUST use the `useEffect(() => { if (open) { reset state } }, [open, ...deps])` pattern — see `EditMetadataModal.tsx:20-29`.
- **`any` type usage.** Forbidden. All API responses and form state must use typed interfaces from `api/types.ts`.

## API Surface Checklist

- [ ] **`POST /api/droplets` via `createDroplet`** — Contract: accepts `CreateDropletRequest` (repo: string required, title: string required, description?: string, priority?: number, complexity?: number, depends_on?: string[]), returns `Droplet`. Already exists in `useApi.ts:314-319`. The `CreateDropletForm` must validate that `repo` and `title` are non-empty before calling this function.

- [ ] **`PATCH /api/droplets/{id}` via `editDroplet`** — Contract: accepts `EditDropletRequest` (title?, description?, complexity?, priority?), returns `Droplet`. Already exists in `useApi.ts:307-312`. The enhanced `EditMetadataModal` must show a diff of changed fields before calling this function. The `ComplexitySelector` replaces the raw number input.

- [ ] **`POST /api/droplets/{id}/rename` via `renameDroplet`** — Contract: accepts `{ title: string }`, returns `Droplet`. New mutation function needed in `useApi.ts`. The `RenameInput` component must call this on blur or Enter, and revert on Escape.

- [ ] **`POST /api/droplets/{id}/close` via `useDropletMutation`** — Contract: accepts `{ notes?: string }`, returns void (204). Already handled via `useDropletMutation.mutate(id, 'close', body)`. The `CloseModal` must show confirmation warning before calling.

- [ ] **`POST /api/droplets/{id}/reopen` via `useDropletMutation`** — Contract: accepts `{ notes?: string }`, returns void (204). Already handled via `useDropletMutation.mutate(id, 'reopen', body)`. The `ReopenModal` must show confirmation before calling.

- [ ] **`POST /api/droplets/{id}/issues` via `addIssue`** — Contract: accepts `AddIssueRequest` (description: string required, flagged_by?: string), returns `DropletIssue`. Already exists in `useApi.ts:321-326`. The `FileIssueModal` must validate `description` is non-empty before calling.

- [ ] **`POST /api/issues/{id}/resolve` via `resolveIssue`** — Contract: accepts `ResolveIssueRequest` (evidence: string required), returns void (204). Already exists in `useApi.ts:328-333`. The `IssueCard` resolve dialog must validate evidence is non-empty.

- [ ] **`POST /api/issues/{id}/reject` via `rejectIssue`** — Contract: accepts `ResolveIssueRequest` (evidence: string required), returns void (204). Already exists in `useApi.ts:335-340`. The `IssueCard` reject dialog must validate evidence is non-empty.

- [ ] **`GET /api/repos` via `useRepos`** — Contract: returns `RepoInfo[]`. Already exists in `useApi.ts:270-286`. Must populate the repo dropdown in `CreateDropletForm`.

- [ ] **`GET /api/repos/{name}/steps` via `useRepoSteps`** — Contract: returns `string[]`. Already exists in `useApi.ts:355-372`. Must drive the pipeline stage visualization in `ComplexitySelector`.

- [ ] **`GET /api/droplets/search?query=...` via `useSearchDroplets`** — Contract: accepts `query`, optional `status` and `priority`, returns `DropletSearchResponse`. Already exists in `useApi.ts:288-298`. Must power the type-ahead dependency search in `CreateDropletForm`.

- [ ] **`GET /api/droplets/{id}/issues?open=&flagged_by=` via `useDropletIssues`** — Contract: accepts optional `open` boolean and `flagged_by` string filter, returns `DropletIssue[]`. Already exists in `useApi.ts:175-216`. Enhanced `IssueFilters` must accept `sort` (newest/oldest) via client-side sorting since the API does not support a sort param (see `useDropletIssues` filter params).

- [ ] **Route `/app/droplets/new`** — Must be added to `web/src/main.tsx` as `{ path: 'droplets/new', element: <CreateDroplet /> }` before the `droplets/:id` route to avoid route collision (React Router v6 matches first matching pattern).

- [ ] **`CreateDroplet` page component** — Contract: renders `CreateDropletForm`, wraps in the standard page layout (`flex-1 overflow-y-auto p-4 md:p-6 space-y-4`), uses `useNavigate` for redirect to `/app/droplets/{id}` on success and `/app/droplets` on cancel.

- [ ] **`ComplexitySelector` component** — Contract: renders three radio options (standard/1, full/2, critical/3) with pipeline stage descriptions. When a repo is selected, uses `useRepoSteps` to show actual pipeline stages. Falls back to hardcoded stage names (Standard: implement → delivery; Full: implement → review → qa → docs → delivery; Critical: implement → review → qa → security-review → docs → delivery) when no repo is selected or steps haven't loaded. Accepts `value: number`, `onChange: (v: number) => void`, `repoName?: string`.

- [ ] **`RenameInput` component** — Contract: displays the droplet title as inline text. On click, switches to an `<input>`. On Enter or blur, calls `renameDroplet(id, { title })` API. On Escape, reverts to original text. Shows a brief loading indicator during save. Accepts `dropletId: string`, `title: string`, `onRenamed: () => void`.

- [ ] **`FileIssueModal` component** — Contract: modal with description textarea (required) and flagged-by dropdown. Options for flagged-by: `implement`, `review`, `qa`, `security-review`, `docs`, `delivery`. On submit, calls `addIssue(dropletId, { description, flagged_by })`. Accepts `open: boolean`, `onClose: () => void`, `dropletId: string`, `onFiled: () => void`. Replaces the inline `FileIssueButton` in `DropletDetail.tsx:263-317`.

- [ ] **`IssueCard` component** — Contract: renders a single issue card with ID (clickable to expand), flagged-by badge with color, description (multi-line), status badge (open=yellow, resolved=green, rejected=red), evidence (if resolved/rejected), timestamps. Shows resolve/reject buttons for open issues. Accepts `issue: DropletIssue`, `onResolve: (id: string) => void`, `onReject: (id: string) => void`.

- [ ] **`IssueFilters` component** — Contract: renders a toggle for open/all, a dropdown for flagged_by role, and a sort select (newest/oldest). Accepts `filters: { openOnly: boolean, flaggedBy: string, sort: 'newest' | 'oldest' }`, `onFilterChange: (filters) => void`.

- [ ] **`CloseModal` component** — Contract: modal showing confirmation warning "This will mark the droplet as delivered". Calls `useDropletMutation.mutate(id, 'close')` on confirm. Accepts `open: boolean`, `onClose: () => void`, `dropletId: string`, `onConfirmed: () => void`.

- [ ] **`ReopenModal` component** — Contract: modal showing confirmation. Calls `useDropletMutation.mutate(id, 'reopen')` on confirm. Accepts `open: boolean`, `onClose: () => void`, `dropletId: string`, `onConfirmed: () => void`.

- [ ] **`ModalOverlay` component** — Contract: renders `fixed inset-0 bg-black/60 flex items-center justify-center z-50` backdrop with `onClick={onClose}`, inner `bg-cistern-surface border border-cistern-border rounded-lg p-6 max-w-md w-full mx-4` card with `onClick={(e) => e.stopPropagation()}`. Returns null when `open` is false. Accepts `open: boolean`, `onClose: () => void`, `children: React.ReactNode`. All modals (existing and new) should be migrated to use this.

- [ ] **Diff display in `EditMetadataModal`** — Before calling `editDroplet`, compare current form values against original `droplet` props and show a summary of changed fields. Contract: the modal must show which fields changed and their old/new values, requiring user confirmation before submitting the PATCH request.

- [ ] **`renameDroplet` function in `useApi.ts`** — New standalone async function following the pattern of `editDroplet`. Contract: `renameDroplet(id: string, body: { title: string }): Promise<Droplet>` calls `POST /api/droplets/{id}/rename` with JSON body `{ title }`.

- [ ] **`closeDroplet` function in `useApi.ts`** — Optional convenience wrapper. The existing `useDropletMutation.mutate(id, 'close')` already works, but a standalone function `closeDroplet(id: string): Promise<void>` following the pattern of `addNote` is acceptable for clarity.

- [ ] **`reopenDroplet` function in `useApi.ts`** — Same as above. Optional convenience wrapper for `useDropletMutation.mutate(id, 'reopen')`.