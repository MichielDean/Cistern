<p align="center">
  <img src="cistern_logo.png" alt="Cistern Logo" />
</p>

Cistern is an agentic delivery system built around a water metaphor. Droplets of work enter the cistern, flow through named aqueducts cataractae by cataractae, and what emerges at the other end is clean enough to ship.

## The Vocabulary

| Term | Meaning |
|---|---|
| **Droplet** | A unit of work — one issue, one feature, one fix. The atomic thing that flows. |
| **Complexity** | A droplet's weight: standard, full, or critical. All droplets run through all cataractae. |
| **Filtration** | Optional LLM refinement step. Refine a raw idea before it enters the Cistern. |
| **Cistern** | The reservoir. Droplets queue here waiting to flow into the aqueduct. |
| **Drought** | Idle state. The cistern is dry. Drought protocols run maintenance automatically. A drought may also be a forced maintenance window where processing is stopped. |
| **Aqueduct** | The full pipeline — from intake through cataractae gates to delivery. Named aqueducts are independent instances the Castellarius routes droplets into. |
| **Castellarius** | The overseer. Watches all aqueducts, routes droplets into aqueducts, runs drought protocols. External to the cistern — pure state machine, no AI. |
| **Cataractae** | A gate along the aqueduct. Each cataractae implements, reviews, or diverts (LLMs working). |
| **Recirculate** | Send a droplet back to a previous cataractae for further processing — revision from reviewer or QA. |
| **Delivered** | A droplet that made it: PR merged, delivered. |
| **Pooled** | A droplet that cannot currently flow forward. |

![Cistern](Cistern.png)

## Quick Start

```bash
# Install
curl -sSL https://raw.githubusercontent.com/MichielDean/cistern/main/install.sh | bash

# Initialize — creates ~/.cistern/cistern.yaml and default aqueduct files
ct init

# Add a droplet to the cistern
ct droplet add --title "Add retry logic to fetch" --repo myproject

# Add a critical droplet (runs all cataractae including security review + human gate)
ct droplet add --title "Rewrite auth layer" --repo myproject --complexity critical

# Wake the Castellarius — he watches the cistern and routes droplets automatically
ct castellarius start

# After rebuilding ct (go build), restart the Castellarius to pick up changes:
# ct binary changes → restart required (long-running process uses old binary)
# feature.yaml / AGENTS.md / skills changes → no restart (read per spawn)

# See the overall picture
ct status

# See what's in the cistern
ct droplet list

# Watch the live flow-graph dashboard
ct dashboard
```

## How It Works

Every droplet flows through the same sequence of cataractae, regardless of complexity level:

```
All:      implement → review → qa → security-review → docs → delivery → done
```

All droplets flow through the same pipeline and auto-merge after delivery.

Filtration is an optional pre-intake step that refines vague ideas before they enter the pipeline. Use `ct droplet add --filter` to filtrate while adding, or `ct filter` to refine ideas standalone before deciding to add them.

1. **Implement** (`implement`) — Reads the droplet description, implements the feature, writes tests, commits. Verifies every concrete deliverable from the description exists in the commit before signaling pass.

2. **Adversarial Review** (`review`) — Reviews a diff with full codebase access. Checks for bugs, security issues, missing tests, logic errors, and orphaned code (unreferenced files, imports, or type values left behind by deletions). Also looks for duplicate implementations, broken contracts, pattern violations, and unnecessary complexity (redundant code, dead variables, unclear names, consolidatable logic).

3. **QA** (`qa`) — Active verification with full codebase access: runs tests, checks each deliverable exists via `grep`, verifies CLI flags, checks mirror file consistency. Recirculates to implement on any failure.

4. **Security Review** (`security-review`) — Adversarial security audit of the diff with full codebase access. Traces call chains to verify auth checks, audits cumulative exposure, and checks for auth bypass, injection, prompt injection, exposed secrets, resource safety, and path traversal.

5. **Docs** (`docs`) — Reviews the diff and updates documentation for all user-visible changes: README, CHANGELOG, CLI reference, config docs. Skips if there are no user-visible changes.

6. **Delivery** (`delivery`) — Owns all git operations: rebase, PR creation, CI monitoring, PR review response, and merge. One agent handles the full branch-to-merged lifecycle. If a delivery agent stalls, the Castellarius detects and recovers automatically — see [Automatic Stuck Delivery Recovery](#automatic-stuck-delivery-recovery).

## Pipeline Behaviors

- **Recirculation** — A cataractae sends the droplet back upstream to a prior cataractae for another pass when issues are found. No retry limits. The water flows until it's pure.
- **Auto-merge** — After delivery, droplets auto-merge to main. All complexity levels flow through the same pipeline and auto-merge identically.

## Complexity Levels

Set complexity when adding a droplet with `--complexity` (or `-x`). Complexity levels indicate the scrutiny level used during review and QA, but all droplets run through the same pipeline and auto-merge identically:

| Level | Name | Purpose |
|---|---|---|
| 1 | standard | Minimal changes — suitable for simple fixes |
| 2 | full *(default)* | Regular features — standard scrutiny |
| 3 | critical | High-impact changes — maximum scrutiny (security review, etc.) |

```bash
ct droplet add --title "Add pagination to list endpoint" --repo myproject --complexity standard
ct droplet add --title "Implement JWT refresh" --repo myproject --complexity full
ct droplet add --title "Replace auth middleware" --repo myproject --complexity critical
```

Accepts numeric (`1`–`3`) or named values.

## Two-Phase Review

The review step uses a structured two-phase protocol that prevents reviewer anchoring and ensures prior issues are actually fixed.

**Phase 1 — Verify prior issues.** If the droplet has been recirculated, the reviewer checks each previously filed issue first: mark it `RESOLVED` with evidence (test name, line number) or `UNRESOLVED` with the gap. The reviewer cannot skip to fresh review until all prior issues are assessed.

**Phase 2 — Fresh review.** After verifying prior work, the reviewer performs a clean-slate review of the diff. New findings are filed as structured issues via `ct droplet issue add`.

This protocol prevents common failure modes: rubber-stamping recirculations, anchoring on prior notes, or missing regressions introduced during fixes.

## Issue Tracking

Cistern maintains a `droplet_issues` table for structured findings from review. Each issue has a description, a filer, and a resolution state.

```bash
ct droplet issue add <id> "<description>"                    File a finding against a droplet
ct droplet issue list <id>                                   List all issues for a droplet
ct droplet issue list <id> --open                            List only open issues
ct droplet issue list <id> --flagged-by <cataractae-name>    List issues filed by a specific cataractae
ct droplet issue resolve <issue-id> --evidence ""            Resolve with proof (reviewer only — not implementer)
ct droplet issue reject <issue-id> --evidence ""             Reject as invalid with proof (reviewer only)
```

Key invariants:
- Implementers cannot resolve or reject issues — only reviewer cataractae may.
- Droplets can be passed regardless of open issues — reviewers and QA use issues for feedback, not as a gate.
- Resolution requires evidence (test name, line reference, or command output).

## Named Aqueducts

Each repo in `cistern.yaml` gets a set of named aqueducts — independent processing lanes that run concurrently. Configure the names under `names:` for each repo:

```yaml
repos:
  - name: myproject
    url: https://github.com/org/myproject
    workflow_path: aqueduct/feature.yaml
    cataractae: 2
    names:
      - virgo
      - marcia
```

Repo names are validated case-insensitively — `ct droplet add --repo myproject` and `ct droplet add --repo MYPROJECT` both map to the canonical name `myproject` in the config.

Aqueduct names are **concurrency slots** — they control how many droplets run in parallel per repo. Each active droplet gets its own isolated git worktree at `~/.cistern/sandboxes/<repo>/<droplet-id>/` on branch `feat/<droplet-id>`. Worktrees are created when a droplet enters the `implement` step and removed once it reaches a terminal state (`done`, `pooled`, or `human`).

All per-droplet worktrees share a single primary clone object store at `~/.cistern/sandboxes/<repo>/_primary/` — objects are shared, only the working tree is per-droplet, keeping disk cost low. Each tmux session is named `<repo>-<aqueduct>`. Every `tmux ls` shows the cistern in motion:

```
myproject-virgo: 1 windows (review)
myproject-marcia: 1 windows (implement)
```

Before dispatching a droplet, the Castellarius checks the worktree for uncommitted files. If files are dirty (excluding `CONTEXT.md`, `.current-stage`, and the provider's InstructionsFile `AGENTS.md`), the droplet is recirculated with a diagnostic note rather than spawning an agent into inconsistent state.

By convention, aqueduct names are drawn from historic Roman aqueducts (`virgo`, `marcia`, `claudia`, `traiana`, `julia`, `appia`, `anio`, `tepula`, `alexandrina`, …), but any names work.

## Customizing Cataractae Definitions

Each cataractae is a self-contained directory under `cataractae/<identity>/` in your aqueduct repo:

```
cataractae/
  implementer/
    PERSONA.md                               # Who this cataractae is — role, guardrails (hand-authored, stable)
    INSTRUCTIONS.md                          # Task protocol and steps (hand-authored)
    AGENTS.md                                # Generated: concatenated from PERSONA.md + INSTRUCTIONS.md
    PIPELINE_POSITION.md                     # Generated: describes role, predecessor, successor in the workflow
    skills/cataractae-protocol/SKILL.md      # Generated: injected universal behavioral protocol
  reviewer/
  qa/
  ...
```

The generated files (`AGENTS.md`, `PIPELINE_POSITION.md`, and injected skills) are generated artifacts — edit `PERSONA.md` and `INSTRUCTIONS.md` directly and regenerate. The instructions filename is always `AGENTS.md` for the opencode provider.

```bash
ct cataractae add <name>            # Scaffold a new cataractae directory with template files; auto-generates the provider's instructions file
ct cataractae list                  # See all cataractae definitions and how to edit them
ct cataractae edit implementer      # Open INSTRUCTIONS.md in $EDITOR, save, instructions file regenerates
ct cataractae generate              # Regenerate provider instructions files (AGENTS.md) from source files
ct cataractae status                # Show which cataractae are actively processing droplets
```

The `aqueduct.yaml` holds routing configuration (which cataractae run at each step, skill references, timeouts, model selection). Persona and instruction content lives in the directory files, not inline in YAML.

### Per-step model selection

Each cataractae step can specify an LLM model with the optional `model:` field:

```yaml
cataractae:
  - name: implement
    type: agent
    identity: implementer
    context: full_codebase

  - name: review
    type: agent
    identity: reviewer
    context: full_codebase
```

Valid values are any string accepted by the configured provider's CLI. If `model:` is omitted, the agent uses the `provider.model:` default from `cistern.yaml`, or the CLI's own default if neither is set. `ct doctor` validates that the value is a non-empty string when present.

### Instructions Templates

Cataractae instructions can use Go template syntax to render content at spawn time. This allows `AGENTS.md` (or `PERSONA.md`/`INSTRUCTIONS.md` that generate `AGENTS.md`) to reference the current step's routing, droplet metadata, and pipeline structure. Templates are rendered before the file is sent to the agent — agents never see raw template markers.

**Template variables available at render time:**

```
{{.Step.Name}}              Current step name (e.g., 'implement', 'review')
{{.Step.Position}}          0-based step index in the pipeline
{{.Step.IsFirst}}           true if this is the first step
{{.Step.IsLast}}            true if this is the last step
{{.Step.OnPass}}            Name of next step after pass, or 'done'
{{.Step.OnFail}}            Name of fail target, or 'pooled'
{{.Step.OnRecirculate}}     Name of recirculate target (empty if not configured)
{{.Step.OnPool}}             Name of pool target (empty if not configured)
{{.Step.ValidOutcomes}}     Slice of valid ct droplet commands with descriptions
{{.Step.SkippedFor}}        Complexity levels this step is skipped for
{{.Droplet.ID}}             Work item ID (e.g., 'ci-amg37')
{{.Droplet.Title}}          Work item title
{{.Droplet.Description}}    Full work item description
{{.Droplet.Complexity}}     Complexity level (standard, full, critical)
{{.Pipeline}}               Ordered slice of all step names
```

**Example template fragment (in AGENTS.md or INSTRUCTIONS.md):**

```markdown
## Signaling Outcomes

**Pass (work complete):**
{{if .Step.OnPass}}
- ct droplet pass {{.Droplet.ID}} — advance to {{.Step.OnPass}}
{{else}}
- ct droplet pass {{.Droplet.ID}} — work complete
{{end}}

{{if .Step.OnRecirculate}}
**Recirculate (send back for revision):**
- ct droplet recirculate {{.Droplet.ID}} — return to {{.Step.OnRecirculate}}
{{end}}

**Pool (cannot currently proceed):**
- ct droplet pool {{.Droplet.ID}} — cannot currently proceed
```

**Static files pass through unchanged** — if `AGENTS.md` contains no template markers, it is used as-is. This maintains backward compatibility.

**Previewing templates:**

Authors can preview rendered output before deployment:

```bash
ct cataractae render --step implement                    # Render with sample droplet data
ct cataractae render --step review --droplet ci-amg37    # Render with specific droplet context
```

## Skills

Skills are reusable knowledge packages injected into cataractae at spawn time. Opencode receives skill content as text in the prompt preamble. Skills keep cataractae prompts concise by factoring out shared conventions.

```bash
ct skills install <name> <url>   Install a skill from a URL
ct skills list                   List installed skills and which cataractae reference them
ct skills update <name>          Re-fetch from source URL
ct skills update                 Re-fetch all skills
ct skills remove <name>          Remove a skill
```

Skills are referenced by name in your aqueduct YAML under each cataractae's `skills:` list. They live in `~/.cistern/skills/<name>/SKILL.md`. Skills bundled with the repo live under `skills/` and are deployed automatically into `~/.cistern/skills/` by the `git_sync` drought hook — no manual install required.

`ct skills update` re-fetches skills from their source URL. Skills managed by `git_sync` (recorded as `source_url:local`) are skipped — they stay in sync via `git_sync` automatically.

**Built-in skills:**

| Skill | Purpose | Cataractae |
|---|---|---|
| `cistern-droplet-state` | Signal pass/recirculate/block with `ct` CLI | All |
| `cistern-git` | Git conventions: exclude CONTEXT.md and InstructionsFile, merge-base diff, no stash | implement, docs, delivery |
| `cistern-github` | PR creation, CI checks, squash-merge, and automatic conflict resolution for Cistern delivery | implement, review, delivery |
| `cistern-reviewer` | Adversarial code review for Go, TypeScript/Next.js, and TypeScript/React — all findings equal, recirculate on any finding, pass only when nothing remains | review |

The `cistern-git` skill encodes hard-won rules: always use `git add -A -- ':!CONTEXT.md' ':!AGENTS.md'`, always use merge-base diff (`git diff $(git merge-base HEAD origin/main)..HEAD`) instead of two-dot — two-dot includes other PRs that merged to main after branching on unrebased branches, never stash in per-droplet worktrees.

## Drought Protocols

When the cistern is dry, Cistern runs maintenance automatically. Configure in `~/.cistern/cistern.yaml`:

```yaml
# Drought protocols — run when Cistern is idle
drought_hooks:
  - name: sync-workflow
    action: git_sync             # Pull aqueduct.yaml + cataractae source files from origin/main
    restart_if_updated: true     # Hot-reload the Castellarius when the workflow changes

  - name: sync-cataractae
    action: cataractae_generate  # Regenerate AGENTS.md files from PERSONA.md + INSTRUCTIONS.md

  - name: prune-worktrees
    action: worktree_prune       # Prune stale aqueduct registrations

  # - name: git-sync
  #   action: git_sync         # Fetch origin/main: redeploy aqueduct.yaml and skills/ into ~/.cistern/skills/

  # - name: vacuum-cistern
  #   action: db_vacuum          # Compact the cistern database

  # - name: custom
  #   action: shell
  #   command: "echo $(date): cistern dry >> ~/.cistern/drought.log"
```

| Action | What it does |
|---|---|
| `git_sync` | Fetches `origin/main` (with 30s timeout) and deploys `aqueduct.yaml`, `cataractae/<role>/PERSONA.md`, `cataractae/<role>/INSTRUCTIONS.md`, and `skills/` to `~/.cistern/`. Resets the `_primary` clone's working tree to `origin/main` so new worktrees always inherit current files. Safe for agent worktrees (droplet ID directories) — they are never reset and retain in-progress work. Skips files that are already up to date. **Must be the first drought hook** so roles and skills are available to subsequent hooks. |
| `cataractae_generate` | Regenerates the instructions file (`AGENTS.md`) for each cataractae from its `PERSONA.md` + `INSTRUCTIONS.md`. Run after `git_sync` to pick up new source files. |
| `worktree_prune` | Runs `git worktree prune` on the repo's primary clone to remove stale worktree registrations. |
| `db_vacuum` | Flushes the SQLite WAL file back into the main database using `PRAGMA wal_checkpoint(TRUNCATE)`. This reclaims space without requiring an exclusive lock, making it safe to run while agents are active. |
| `shell` | Runs an arbitrary shell command. Use for custom maintenance. |

Protocols fire once on the `flowing → idle` transition, not on every tick. Safe to add your own.

**Note on `git_sync` positioning:** The `git_sync` hook must come before `cataractae_generate` and any skill-referencing hooks. It deploys fresh role definitions and skills from `origin/main`; subsequent hooks depend on these being up to date. The Castellarius logs a warning if `git_sync` is not first.

## Installation

```bash
curl -sSL https://raw.githubusercontent.com/MichielDean/cistern/main/install.sh | bash
```

Requirements:
- Go 1.22+
- `opencode` CLI installed and configured
- `git`, `tmux`
- `gh` CLI installed and authenticated (`gh auth login`) — required for delivery, optional for initial setup

The Castellarius automatically manages agent credentials. `ct doctor` verifies that the agent CLI is available and authenticated.

## Credentials

Cistern uses the opencode CLI for agent sessions. Configure credentials based on your provider:

**For opencode (default):** The opencode CLI manages its own configuration. Ensure it is installed and available on PATH:

```bash
opencode                  # Configure once (follows opencode setup)
ct castellarius start     # Starts the Castellarius
ct status                 # Confirm running
```

**For API key authentication:** Add provider-specific keys to `~/.cistern/env`:

```bash
# If your provider requires an API key:
echo 'GH_TOKEN=ghp_...' >> ~/.cistern/env
chmod 600 ~/.cistern/env
```

`ct init` creates `~/.cistern/env` automatically with the correct permissions (600). The file is added to `~/.cistern/.gitignore` so it is never accidentally committed.

`ct doctor` verifies that the agent CLI is available and checks that `~/.cistern/env` exists with required credential variables. `ct doctor --fix` can create and populate `~/.cistern/env` for missing credentials.

## Configuration

```bash
ct init                        # Create ~/.cistern/ with default config and aqueduct files
ct aqueduct validate           # Check config and all aqueduct files
ct doctor                      # Full health check
ct doctor --fix                # Auto-repair common configuration issues
```

Config lives at `~/.cistern/cistern.yaml`. Key options:

```yaml
# Heartbeat: how often the Castellarius scans for stalled sessions
heartbeat_interval: 30s

# Stall detection: threshold for inactivity before marking a droplet as stalled
# Monitors three progress signals: newest note timestamp, worktree file mtime,
# and session log mtime. Droplet is stalled if all three are older than this threshold.
# When detected: (1) a diagnostic note is appended, (2) if the droplet has an assignee
# with prior session history, the session is automatically re-spawned with --continue
# to allow the agent to resume; (3) further diagnostic notes are suppressed until
# one of the signals advances. Re-spawn failures are automatically retried on the next
# heartbeat tick.
# Default: 45 minutes
stall_threshold_minutes: 45

# Exponential backoff for quick session exits and provider degradation detection
# When a session exits quickly (within this threshold) without an outcome,
# trigger per-droplet exponential backoff. When 3+ sessions fail across 2+ aqueducts
# within 5 minutes, fast-forward all affected droplets to max backoff (provider appears degraded).
# Defaults: 30s for quick-exit threshold, 30m for max backoff
quick_exit_threshold_seconds: 30
max_backoff_minutes: 30

# Dashboard UI: CSS font-family string used by the web and TUI dashboards
# Omit to default to a sensible monospace font stack for terminal rendering
dashboard_font_family: 'Liberation Mono, DejaVu Sans Mono, Menlo, Consolas, monospace'

# Dashboard REST API authentication
# When set, all /api/ endpoints require Bearer token auth.
# Also settable via CISTERN_DASHBOARD_API_KEY environment variable.
# When unset, all endpoints are open (a warning is logged at startup).
# dashboard_api_key: 'your-secret-api-key-here'

# Dashboard CORS allowed origins
# Defaults to localhost variants when unset.
# dashboard_allowed_origins:
#   - 'http://localhost:3000'
#   - 'http://localhost:5737'

# Rate limit: protect the delivery cataractae API endpoint
# Omit to use defaults (60 req/min per IP, 120 req/min per token)
# rate_limit:
#   per_ip_requests: 60
#   per_token_requests: 120
#   window: 1m

# Drought protocols run when the cistern goes idle
drought_hooks:
  - name: sync-workflow
    action: git_sync
    restart_if_updated: true
  - name: sync-cataractae
    action: cataractae_generate
  - name: prune-worktrees
    action: worktree_prune

# External issue tracker integrations — configure providers to import droplets from external trackers
# trackers:
#   - name: jira                              # Provider name (e.g. "jira", "linear")
#     url: https://myorg.atlassian.net        # Base URL of the tracker instance
#     email: user@example.com                 # User email (for authentication, tracker-dependent)
#     token: my-api-token                     # Literal API token (TokenEnv preferred for production)
#     # OR use an environment variable for the token:
#     # token_env: JIRA_TOKEN                 # Reads from $JIRA_TOKEN at runtime (takes precedence)
#
#   - name: linear
#     url: https://linear.app
#     token_env: LINEAR_TOKEN                 # Reads from $LINEAR_TOKEN at runtime
```

See `cistern.yaml` in this repo for all options.

## Provider Configuration

Cistern uses the opencode agent CLI as its default and only provider. Configure the provider in `~/.cistern/cistern.yaml` using the top-level `provider:` block or on a per-repo basis.

**Built-in preset:**

| Name | CLI | Env variable required | Instructions file |
|---|---|---|---|
| `opencode` *(default)* | `opencode` | — | `AGENTS.md` |

**Top-level provider (applies to all repos):**

```yaml
provider:
  name: opencode          # built-in preset name, or 'custom'
  model: ""               # default model passed to opencode (empty = opencode default)
  command: ""              # override the executable (e.g. a wrapper script)
  args: []                # extra args appended to the preset's fixed args
  env: {}                 # extra env vars injected into the agent process
```

**Per-repo override (overrides the top-level for that repo only):**

```yaml
repos:
  - name: myproject
    url: https://github.com/org/myproject
    workflow_path: aqueduct/feature.yaml
    cataractae: 2
    names:
      - virgo
    provider:
      name: opencode      # this repo uses opencode (same as default, shown for illustration)
      model: ""             # uses opencode's default model
```

**Resolution order:** built-in preset defaults → top-level `provider:` overrides → repo-level `provider:` overrides. When a repo specifies a different `name:` than the top-level, top-level field overrides are not applied — only the repo-level overrides take effect.

If no `provider:` block is present, the `opencode` preset is used. Existing configs work unchanged.

The configured provider is also used for **filtration** (`ct droplet add --filter`). There is no separate API key or config for filtration — the same preset, binary, and env var requirements apply to both cataractae sessions and the filtration pass.

## Tracker Configuration

Cistern can integrate with external issue trackers to import droplets from work items in systems like Jira. Tracker providers fetch issue metadata and convert it to Cistern droplets.

Configure trackers in `~/.cistern/cistern.yaml` at the top level with a `trackers:` list:

```yaml
trackers:
  - name: jira
    url: https://myorg.atlassian.net
    email: user@example.com
    token_env: JIRA_API_TOKEN    # reads token from environment variable
```

Each tracker entry requires:

- **name** *(required)* — provider identifier: `jira`, `linear`, etc.
- **url** *(required for jira)* — base URL of the tracker instance
- **email** *(required for jira)* — user email for basic auth (Jira Cloud)
- **token** *(optional)* — literal API token (not recommended for production)
- **token_env** *(recommended)* — environment variable name holding the API token (e.g., `JIRA_API_TOKEN`)

**Token precedence:** When both `token` and `token_env` are set, the environment variable takes precedence. Use `token_env` in production to avoid storing secrets in config files.

### Jira Cloud

The Jira provider integrates with Jira Cloud using REST API v3. It requires:

- A Jira Cloud instance URL (e.g., `https://myorg.atlassian.net`)
- A user email address
- An API token (generated at https://id.atlassian.com/manage-profile/security/api-tokens)

Example configuration:

```yaml
trackers:
  - name: jira
    url: https://myorg.atlassian.net
    email: ci-user@example.com
    token_env: JIRA_API_TOKEN    # export JIRA_API_TOKEN="your-api-token" before running Cistern
```

When a droplet is imported from Jira, the provider fetches the issue by key (e.g., `PROJ-123`) and maps:

- Issue **summary** → droplet **title**
- Issue **description** (converted from ADF to plain text) → droplet **description**
- Issue **priority** → normalized priority (1–4: Highest/High=1, Medium=2, Low=3, Lowest=4)
- Issue **labels** → droplet **labels**
- Issue **key** and **base URL** → droplet **source URL** (link back to Jira)

## Docker

Cistern ships a multi-stage Dockerfile. The image includes `tmux`, `git`, `gh`, `opencode`, and both `ct` and `aqueduct` binaries.

```bash
docker build -t cistern .

# Run the Castellarius — mount ~/.cistern for config, auth, and the database
docker run -v ~/.cistern:/root/.cistern cistern
```

The `/root/.cistern` volume persists config, skills, the SQLite database, and gh auth state across container restarts. `GH_CONFIG_DIR` is set automatically to `/root/.cistern/auth/gh`.

## CLI Reference

```
# Castellarius — the overseer that watches the cistern and routes droplets
ct castellarius start          Wake the Castellarius (start processing)
ct castellarius status         Show aqueduct flow — which are flowing, which are idle; includes per-repo queue depth, active session counts, Castellarius health (last tick time), and stage age per droplet

# Dashboard
ct dashboard                   Live TUI aqueduct arch diagram with cistern and recent flow
ct dashboard --web             HTTP web dashboard on 127.0.0.1:5737 — two modes:
                               • / — renders the real TUI via xterm.js (full ANSI, box-drawing,
                                 animations, pinch-to-zoom on mobile)
                               • /app/ — React SPA dashboard with live SSE updates, aqueduct
                                 arch visualization, droplet queue, pool, recent flow, and
                                 live terminal peek via WebSocket
ct dashboard --web --addr 127.0.0.1:8080  Custom listen address (must include hostname or IP)

## Web UI (React SPA)

The `/app/` route serves a React single-page application providing an alternative
to the xterm.js terminal dashboard. It connects to the same `/api/dashboard/events`
SSE stream used by the TUI. WebSocket connections for live terminal peek use
in-band authentication (auth token sent as the first WebSocket message after
upgrade) rather than URL query parameters, preventing token leakage in server
access logs and browser history.

**Features:**
- Live aqueduct visualization — CSS-based pipeline arch showing cataractae stages,
  flowing droplet position, and animated water-flow effect for active droplets
- Real-time updating via SSE with adaptive polling (2s active, 5s idle)
- Cistern counts (flowing, queued, delivered, pooled), Castellarius status
- Queue view with blocked-by indicators, pooled droplets, orphaned warnings
- Recent flow — last 5 delivered/pooled droplets
- Live terminal peek — click an aqueduct to open a slideover viewing the agent's
  tmux session via WebSocket; search within output (Ctrl+F), auto-scroll with
  manual override toggle, highlighted search matches (capped at 200 highlights)
- Droplets list (`/app/droplets`) — filterable, sortable, paginated table of all
  droplets with status badges, search, and repo filter; click any row for detail
- Create droplet (`/app/droplets/new`) — multi-field form with repo dropdown
  (populated from /api/repos), title (required), description, priority,
  complexity radio group (standard/full/critical) with pipeline stage visualization
  showing which cataractae each level runs through, dependency type-ahead search,
  form validation, and cancel/submit actions
- Droplet detail (`/app/droplets/:id`) — copyable ID, status badge, pipeline position
  indicator with progress bar, real-time notes timeline (SSE), signal action dialogs
  (pass/recirculate/pool), dependencies with add/remove (resolves/blocked_by/blocks),
  issue tracker with file/resolve/reject, modals for notes/metadata/restart,
  inline rename on title click (save on Enter/blur, cancel on Escape), close/reopen
  confirmation dialogs, submit guards (buttons disabled during mutations with
  spinner), error toasts on all async action failures
- **Castellarius control page** (`/app/castellarius`) — view running/stopped status, PID, uptime; start/stop/restart the daemon; aqueduct table with flow status, current droplet, step, and elapsed time; auto-refreshes every 5 seconds
- **Doctor page** (`/app/doctor`) — run health checks with pass/fail/warn indicators; re-run or fix from the UI; grouped by category with summary card
- **Logs page** (`/app/logs`) — real-time log viewer with SSE streaming; source selector (castellarius.log and other available logs); log level color coding (INFO=cyan, WARN=yellow, ERROR=red, DEBUG=dim); search/filter, auto-scroll toggle, line numbers; file size and last-modified metadata
- **Filter/Refine page** (`/app/filter`) — interactive LLM-assisted droplet refinement; multi-turn chat with Cistern's filter model to turn rough ideas into well-specified droplets; session management (new/resume past sessions); spec preview panel showing refined title, description, and complexity; accept button files the droplet and redirects to detail page
- **Import page** (`/app/import`) — import droplets from external issue trackers; Jira integration with provider/key/repo form fields, auto-fill title and description from the tracker issue, complexity and priority selection; credential setup note linking to Doctor page
- **Export button** (on Droplets list) — download droplets as JSON or CSV; format selector; applies current filters (status, repo, priority); includes auth token in download URL when API key is configured
- **Repos & Skills page** (`/app/repos`) — browse configured repositories with aqueduct chains; view installed skills with source URLs and install dates; display-only (install/uninstall remains CLI-only)
- Issue management (on detail page) — file issues with description and flagged-by
  role dropdown, resolve/reject open issues with evidence, issue cards with status
  badges (open=yellow, resolved=green, rejected=red), filters by status and
  flagged-by role, sort by newest/oldest
- Dark theme matching the Cistern color palette
- Responsive sidebar navigation that collapses on mobile
- Toast notification system — success/error/info toasts with auto-dismiss (3s),
  truncation of long messages; all API errors displayed as toasts
- Skeleton loading screens — card, row, and table skeleton variants replace
  plain "Loading…" text across dashboard, droplet detail, castellarius, doctor,
  logs, and repos pages
- Error boundary — catches React render errors app-wide, shows fallback UI
  with "Try Again" button (full page reload)
- 404 catch-all route — unknown `/app/` paths show a 404 page with link back
  to dashboard
- Command palette — press Ctrl+K to open a searchable command palette for
  quick navigation to any page; disabled when focus is in input/textarea fields
- Network status indicator — header shows connection state: "Live" (connected),
  "Reconnecting…" (disconnected), "Connected" (brief flash on reconnection)
- Cross-UI navigation — "Classic Dashboard" link in SPA header points to `/`,
  "New UI" link in xterm.js TUI points to `/app/`
- 401 auth interceptor — on API 401 responses, clears stored API key and
  redirects to login; works with `cistern:auth-expired` custom event
- Keyboard accessibility — sidebar nav links have focus:ring styles;
  Escape closes peek/search/modals; Ctrl+F opens peek search
- Authentication — when `CISTERN_DASHBOARD_API_KEY` is configured, a login page
  is shown; the API key is stored in localStorage and sent via Bearer header
  on REST calls, as a `token` query parameter on SSE connections, and via
  in-band `{"type":"auth","token":"..."}` WebSocket message after upgrade
  (peek endpoint only; query parameters are no longer used for WebSocket auth)

**Build integration:**
```bash
# Using make (recommended)
make web-build              # Build React SPA → cmd/ct/assets/web/
make web-dev                # Vite dev server (proxies API calls to Go server)
make build                  # Build Go binary (includes embedded web assets)

# Or run directly
cd web && npm run build     # Outputs to cmd/ct/assets/web/ (embedded in Go binary)
cd web && npm run dev       # Vite dev server (proxies API calls to Go server)
```

The SPA assets are embedded via `//go:embed` and served at `/app/`. The existing
`/` route (xterm.js TUI) is unchanged.

## REST API

The web dashboard exposes a REST API at `/api/` that mirrors all TUI operations. Every CLI command has a corresponding HTTP endpoint.

**Authentication**: When `dashboard_api_key` is configured (or `CISTERN_DASHBOARD_API_KEY` env var is set), all `/api/` endpoints require a `Bearer` token in the `Authorization` header. Without an API key, all endpoints are open (a warning is logged at startup). The Castellarius start/stop/restart endpoints always require auth regardless of configuration.

**CORS**: Allowed origins default to `localhost` variants. Configure `dashboard_allowed_origins` in `cistern.yaml` to allow additional origins. The API handles CORS preflight (OPTIONS) requests automatically.

### Droplet CRUD

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/droplets` | List droplets (query params: `?repo=&status=&sort=&page=&per_page=&output=json`). Response: `{droplets, total, page, per_page}`. Sort: `priority` (default), `created_at`, `updated_at`, `title`. `per_page` capped at 500, defaults to 50 |
| `GET` | `/api/droplets/search` | Search droplets (query params: `?query=&status=&priority=&page=&per_page=`). Response: `{droplets, total, page, per_page}`. `per_page` capped at 500, defaults to 50 |
| `GET` | `/api/droplets/{id}` | Get single droplet detail |
| `POST` | `/api/droplets` | Create droplet (JSON body: `repo`, `title`, `description`, `priority`, `complexity`, `depends_on`) |
| `PATCH` | `/api/droplets/{id}` | Edit mutable fields (JSON body: `title`, `description`, `priority`, `complexity`) |
| `POST` | `/api/droplets/{id}/rename` | Rename droplet (JSON body: `title`) |

### Droplet state transitions

| Method | Endpoint | Description |
|--------|----------|-------------|
| `POST` | `/api/droplets/{id}/pass` | Signal pass (optional JSON body: `notes`) |
| `POST` | `/api/droplets/{id}/recirculate` | Signal recirculate (JSON body: `to`, `notes`) |
| `POST` | `/api/droplets/{id}/pool` | Signal pool (optional JSON body: `notes`) |
| `POST` | `/api/droplets/{id}/close` | Close/deliver droplet |
| `POST` | `/api/droplets/{id}/reopen` | Reopen a closed droplet |
| `POST` | `/api/droplets/{id}/cancel` | Cancel droplet (JSON body: `reason`) |
| `POST` | `/api/droplets/{id}/restart` | Restart at step (optional JSON body: `cataractae`) |
| `POST` | `/api/droplets/{id}/approve` | Approve human-gated droplet |
| `POST` | `/api/droplets/{id}/heartbeat` | Record agent heartbeat |

### Notes

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/droplets/{id}/notes` | List notes |
| `POST` | `/api/droplets/{id}/notes` | Add note (JSON body: `cataractae`, `content`) |

### Issues

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/droplets/{id}/issues` | List issues (query params: `?open=true&flagged_by=`) |
| `POST` | `/api/droplets/{id}/issues` | File issue (JSON body: `flagged_by`, `description`) |
| `POST` | `/api/issues/{id}/resolve` | Resolve issue (JSON body: `evidence`) |
| `POST` | `/api/issues/{id}/reject` | Reject issue (JSON body: `evidence`) |

### Dependencies

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/droplets/{id}/dependencies` | List dependencies — returns `[{depends_on, type}]` where type is `resolves` (delivered forward dep), `blocked_by` (undelivered forward dep), or `blocks` (reverse dep: droplets that depend on this one) |
| `POST` | `/api/droplets/{id}/dependencies` | Add dependency (JSON body: `depends_on`) — returns updated dependency list with typed entries |
| `DELETE` | `/api/droplets/{id}/dependencies/{dep_id}` | Remove dependency |

### History & Stats

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/droplets/{id}/log` | Event timeline (query params: `?format=notes&limit=`) |
| `GET` | `/api/droplets/{id}/changes` | Ordered changes (query params: `?limit=`) |
| `GET` | `/api/stats` | Droplet counts by status |

### Export & Purge

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/droplets/export` | Export droplets (query params: `?format=json|csv&status=&priority=&repo=`) |
| `POST` | `/api/droplets/purge` | Delete old completed droplets (JSON body: `older_than`, `dry_run`) |

### SSE (Server-Sent Events)

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/droplets/{id}/events` | Real-time droplet updates (SSE stream, max 64 concurrent connections) |

### Filter Sessions

| Method | Endpoint | Description |
|--------|----------|-------------|
| `POST` | `/api/filter/new` | Create a new filter session (JSON body: `title`, `description`). Rate-limited (10 req/min per IP) |
| `POST` | `/api/filter/{session_id}/resume` | Send a message and get LLM response (JSON body: `message`). Rate-limited (10 req/min per IP) |
| `GET` | `/api/filter/sessions` | List past filter sessions |
| `GET` | `/api/filter/{session_id}` | Get session history and spec snapshot |

### Import

| Method | Endpoint | Description |
|--------|----------|-------------|
| `POST` | `/api/import` | Import a tracker issue as a droplet (JSON body: `provider`, `key`, `repo`, `complexity`, `priority`). Rate-limited (10 req/min per IP) |
| `GET` | `/api/import/preview` | Preview tracker issue before importing (query params: `?provider=&key=`). Rate-limited (10 req/min per IP) |

### Castellarius, Doctor, Repos & Skills

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/castellarius/status` | Current Castellarius status (running, PID, uptime, aqueducts, farm status) |
| `POST` | `/api/castellarius/start` | Start the daemon (requires auth) |
| `POST` | `/api/castellarius/stop` | Stop the daemon (requires auth) |
| `POST` | `/api/castellarius/restart` | Restart the daemon (requires auth) |
| `GET` | `/api/doctor` | Run health check (query param: `?fix=true`) |
| `GET` | `/api/repos` | List configured repos with aqueduct chains |
| `GET` | `/api/repos/{name}/steps` | List pipeline step names for a repo |
| `GET` | `/api/skills` | List installed skills |

### Logs

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/logs` | Get recent log lines (query params: `?lines=500&source=castellarius`) |
| `GET` | `/api/logs/events` | SSE stream of new log lines (query param: `?source=castellarius`) |
| `GET` | `/api/logs/sources` | List available log sources with file size and last-modified time |

### Input validation

All endpoints enforce field length limits:

| Field | Max length |
|-------|-----------|
| `title` | 256 |
| `repo` | 128 |
| `description` | 4096 |
| `notes` / `reason` | 65536 |
| `content` (issues/notes) | 65536 |
| `depends_on` | 128 |
| `key` (import) | 128 |

Import keys are also validated to contain only alphanumeric characters, hyphens, and underscores (prevents path traversal). Filter/import endpoints are rate-limited at 10 requests per minute per IP.

Request bodies are capped at 1 MiB. Aqueduct names in WebSocket/SSE endpoints are validated to prevent tmux injection. CSV export sanitizes cells to prevent formula injection.

# Status — observe the system
ct status                      Overall status: cistern level, aqueduct flow, cataractae chains; shows (stage X) age per droplet
ct status --json               Machine-readable JSON: flowing/queued counts, cataractae, aqueduct info
ct status --watch              Continuously refresh every 5 seconds (Ctrl-C to stop)
ct status --watch --interval N  Refresh every N seconds (min 1)
ct aqueduct status             Aqueduct definitions: repos and their cataractae chains

# Aqueduct — inspect and validate aqueduct definitions
ct aqueduct validate           Validate cistern.yaml and all referenced workflow files
ct aqueduct inspect            JSON snapshot of current Cistern state
ct aqueduct inspect --table    Human-readable table instead of JSON

# Filtration — refine ideas before adding droplets
ct filter --title 'rough idea'                          Start a new filtration session
ct filter --title 'idea' --description '...'           New session with description
ct filter --resume <id> 'feedback'                      Continue refining a session
ct filter --resume <id> --file --repo <repo>           Persist refined session to cistern
ct filter --output-format json                         Machine-readable output (with --title or --resume)

# Droplets — manage work items
ct droplet add --title "..." --repo myproject                     Add a droplet
ct droplet add --title "..." --repo myproject --filter            LLM-assisted filtration before adding
ct droplet add --title "..." --repo myproject --filter --yes      Non-interactive filtration (agent use)
ct droplet add --title "..." --depends-on <id>                    Add with dependency on another droplet
ct droplet add --title "..." --complexity standard                 Set complexity (standard/full/critical or 1–3)
ct droplet add --title "..." --priority 1                         Set priority (1=highest)
ct droplet list                                                   List active droplets
ct droplet list --all                                             Include delivered droplets (dimmed)
ct droplet list --watch                                           Live-refresh every 2 seconds (Ctrl-C to stop)
ct droplet list --status in_progress                              Filter by status
ct droplet list --output json                                     JSON output
ct droplet search --query "retry"                                 Search by title substring
ct droplet search --status in_progress --priority 1               Filter by status and priority
ct droplet search --output json                                   JSON search output
ct droplet export --format json                                   Export all droplets as JSON
ct droplet export --format csv --status delivered                 Export delivered droplets as CSV
ct droplet show <id>                                              Show droplet details and notes
ct droplet rename <id> "New title"                                Rename a droplet
ct droplet edit <id> -t "new title"                               Edit title
ct droplet edit <id> -x critical -p 1                              Edit complexity and priority
ct droplet edit <id> --description "updated desc"                  Edit description
ct droplet edit <id> --description -                               Read description from stdin
ct droplet edit <id>                                               Interactive: open in $EDITOR (vi default)
ct droplet note <id> "What you found"                             Add a note to a droplet
ct droplet stats                                                  Show droplet counts by status
ct droplet deps <id>                                              List dependency chain for a droplet
ct droplet deps <id> --add <dep-id>                               Add a dependency
ct droplet deps <id> --remove <dep-id>                            Remove a dependency
ct droplet close <id>                                             Mark delivered
ct droplet reopen <id>                                            Return to cistern (status=open, cataractae unchanged)
ct droplet restart <id>                                         Restart from current cataractae
ct droplet restart <id> --cataractae delivery                   Re-enter at a specific cataractae (recovery)
ct droplet restart <id> --cataractae delivery --notes "..."     Re-enter with a recovery note
ct droplet purge --older-than 30d                                 Delete old delivered/pooled droplets
ct droplet purge --older-than 24h --dry-run                       Preview what would be purged
ct droplet pool <id> --notes "..."                               Mark a droplet pooled

# Tail — stream droplet events in real time
ct droplet tail <id>                                        Show last 20 events and exit
ct droplet tail <id> --follow                               Stream events continuously (like tail -f); exits on terminal state
ct droplet tail <id> --lines 50                             Show last 50 events on start
ct droplet tail <id> --format json                          Output events as NDJSON (one JSON object per line)

# Log — chronological activity timeline for a droplet
ct droplet log <id>                                         Show activity log (creation, transitions, signals, heartbeat, notes)
ct droplet log <id> --format json                           Output as NDJSON (one JSON object per line)

# History — alias for ct droplet log
ct droplet history <id>                                     Show event timeline (identical output to ct droplet log)
ct droplet history <id> --format json                       Output as NDJSON

# Droplet outcomes — used by agent cataractae to signal completion
ct droplet pass <id>                                              Advance to next cataractae
ct droplet pass <id> --notes "..."                                Advance with notes
ct droplet recirculate <id>                                       Send back to previous cataractae
ct droplet recirculate <id> --to implement                        Send back to a named cataractae
ct droplet recirculate <id> --notes "..."                         Recirculate with notes
ct droplet pool <id>                                             Mark as pooled — cannot proceed
ct droplet pool <id> --notes "..."                               Pool with notes
ct droplet cancel <id> --reason "..."                            Cancel — won't be implemented (reason required)

# Human gate — critical droplets pause here before delivery
ct droplet approve <id>                                           Approve a critical droplet for delivery

# Peek — observe live agent output
ct droplet peek <id>                                              Attach read-only to the live tmux session (or show last notes if session ended); header shows (stage X) age
ct droplet peek <id> --snapshot                                   Capture a static snapshot instead of live attach
ct droplet peek <id> --snapshot --lines 100                       With --snapshot: show only last 100 lines (default: full scrollback)
ct droplet peek <id> --snapshot --follow                          With --snapshot: re-capture every 3 seconds (Ctrl-C to stop)
ct droplet peek <id> --raw                                        Read the session log file directly without requiring tmux (useful for programmatic consumption)

# Droplet issues — structured findings from review
ct droplet issue add <id> "<description>"                         File a finding
ct droplet issue list <id>                                        List all issues
ct droplet issue list <id> --open                                 List only open issues
ct droplet issue list <id> --flagged-by <cataractae-name>         List issues filed by a specific cataractae
ct droplet issue resolve <issue-id> --evidence "..."              Resolve with proof (reviewer only)
ct droplet issue reject <issue-id> --evidence "..."               Reject as still present (reviewer only)

# Cataractae — manage cataractae definitions
ct cataractae add <name>             Scaffold a new cataractae directory with PERSONA.md and INSTRUCTIONS.md; auto-generates the provider's instructions file
ct cataractae list                   See all cataractae definitions
ct cataractae status                 Show which cataractae are active and what they're processing
ct cataractae edit <cataractae>       Edit cataractae definition in $EDITOR
ct cataractae generate               Regenerate provider instructions files (AGENTS.md) from source files
ct cataractae render --step <name>   Preview rendered template for a step with sample droplet data
ct cataractae render --step <name> --droplet <id>  Preview with specific droplet context

# Skills — manage cataractae skills
ct skills install <name> <url>       Install a skill from a URL
ct skills list                       List installed skills and which cataractae reference them
ct skills update <name>              Re-fetch a skill from its source URL
ct skills update                     Re-fetch all skills
ct skills remove <name>              Remove a skill

# Utilities
ct doctor                      Full health check (prerequisites, config, instructions file integrity, skills)
ct doctor --fix                Auto-repair common issues
ct doctor --skills             List all skills referenced by any aqueduct and their install status
ct version                     Print version string
ct version --json              Machine-readable: {"version":"...","commit":"..."}
ct update                      Pull latest main and rebuild ct in-place; warns if Castellarius is running
ct update --dry-run            Show what would change without building
ct update --repo-path PATH     Override repo path (default: sibling of binary or CT_REPO_PATH env)
```

---

## Automatic Stuck Delivery Recovery

The Castellarius detects and recovers stuck delivery agents automatically — no human intervention required for the common failure modes.

A delivery agent is considered **stuck** when all of the following are true:
- The droplet has been in the `delivery` step for longer than 1.5× the delivery `timeout_minutes` (default: 60 m → 90 m)
- The agent's tmux session is still alive
- No outcome has been written yet

Every 5 minutes, the Castellarius scans all active delivery droplets and recovers any that qualify:

| PR State | Recovery Action |
|---|---|
| **MERGED** | Signals pass — agent just didn't notice |
| **OPEN**, branch behind main | Rebase onto `origin/main`, push, enable auto-merge, signal pass |
| **OPEN**, CI failing | Recirculate for another pipeline pass |
| **OPEN**, all checks green | Attempt direct merge → auto-merge, signal pass |
| **CLOSED** (not merged) | Recirculate with notes |
| No PR found | Recirculate with notes |

Recovery actions are noted on the droplet (`ct droplet show <id>`) and logged by the Castellarius. Recovery is idempotent — safe to trigger multiple times.

The stuck threshold is configurable via `timeout_minutes` on the `delivery` step in your aqueduct YAML. The check fires at 1.5× that value.

---

## Automatic Dispatch-Loop Recovery

The Castellarius detects and recovers droplets stuck in a **dispatch loop** — where the Castellarius repeatedly tries to spawn an agent but fails every time, leaving no tmux session and no progress.

A droplet is considered dispatch-looping when it accumulates **5 or more dispatch failures within any 2-minute window** with no successful agent spawn.

When a dispatch loop is detected, the Castellarius attempts ordered self-recovery before the next dispatch:

| Failure Pattern | Recovery Action |
|---|---|
| Dirty worktree | `git reset --hard HEAD && git clean -fd` on the droplet worktree |
| Worktree missing or corrupt | Remove and recreate the worktree from the primary clone |
| Feature branch missing from git (pathspec error) | Remove stale worktree directory and create a fresh branch from origin/main; if fresh-branch creation fails, pool the droplet |
| No applicable pattern found | Note the failure and retry next cycle |

After **3 failed self-fix attempts**, the droplet is pooled with a note describing the failure. Use `ct droplet show <id>` to inspect the recovery history, then `ct droplet restart <id> --cataractae <step>` to re-enter once the underlying issue is resolved.

Recovery attempts are attached as notes on the droplet and logged by the Castellarius with the prefix `dispatch-loop recovery:`. A successful agent spawn resets all counters — a droplet that recovers cleanly leaves no permanent trace.

---

## Recovery

When a delivery fails mid-flight (merge conflict, CI failure, permission issue) or a droplet gets
incorrectly marked delivered before the PR actually merged, use `ct droplet restart` to send it
back into the pipeline at the exact cataractae it needs:

```bash
# Re-enter delivery after manually resolving conflicts
ct droplet restart sc-uvfhw --cataractae delivery

# Re-enter with a note explaining why
ct droplet restart sc-uvfhw --cataractae delivery \
  --notes "PR #157 had webhook store signature conflict — resolved manually, re-entering delivery"

# Send back to implement if the feature itself needs rework
ct droplet restart sc-gh7lg --cataractae implement \
  --notes "GetMe and UpdateMe handlers collided with main — needs clean rewrite"
```

`restart` clears the assignee, outcome, and sets status back to `open` at the named cataractae.
The Castellarius picks it up on the next tick. Works from any terminal state: delivered, pooled, or open.
Cataractae names are validated against the aqueduct config — if the config cannot be loaded, any name is accepted.
Without `--cataractae`, the droplet restarts from its current stage. If the droplet has no current stage,
`--cataractae` must be provided.

This differs from `reopen` (which returns to `open` with the cataractae unchanged) and
`recirculate` (which is an agent-issued signal during active processing). `restart` is for
human-initiated recovery after something went wrong.

## OpenClaw Integration

An [AgentSkills](https://agentskills.io)-compatible skill lives in `openclaw/cistern/`. It teaches
OpenClaw bots how to interact with a Cistern installation — vocabulary, `ct` commands, pipeline
overview, and troubleshooting.

**Install on any OpenClaw bot:**

```bash
cp -r openclaw/cistern ~/.openclaw/skills/cistern
```

The skill gates on `ct` being present on `PATH`, so it only surfaces when Cistern is installed.
Once installed, your OpenClaw agent will automatically understand droplets, aqueducts, cataractae,
and how to manage work through the pipeline.

**Contents:**

| File | Purpose |
|------|---------|
| `SKILL.md` | Core skill — vocabulary, key commands, pipeline overview |
| `references/commands.md` | Full `ct` command reference |
| `references/setup.md` | Install, config, and binary rebuild instructions |
| `references/troubleshooting.md` | Daemon, stuck droplets, DB recovery |
