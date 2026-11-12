---
name: cistern
description: Manage a local Cistern installation — an agentic workflow orchestrator that routes work through LLM-powered pipelines. Use when the user wants to: (1) add, view, or manage droplets (units of work), (2) check pipeline status or aqueduct health, (3) start or restart the Castellarius daemon, (4) view or interact with the dashboard, (5) troubleshoot stuck or failed work, (6) understand Cistern's pipeline stages or vocabulary. Triggers on: "add droplet", "cistern status", "ct status", "ct droplet", "castellarius", "aqueduct", "cataractae", "check the pipeline", or any question about Cistern.
metadata: {"openclaw": {"requires": {"bins": ["ct"]}, "emoji": "🏛️"}}
---

# Cistern

Cistern is an agentic workflow orchestrator. It routes units of work called **droplets** through configurable pipelines called **aqueducts**, where each stage is a **cataractae** handled by an LLM-powered agent.

## Vocabulary

| Term | Meaning |
|------|---------|
| **Droplet** | Atomic unit of work — always say "droplet", never "task/item/ticket" |
| **Cistern** | The reservoir — droplets queue here before processing |
| **Aqueduct** | Named pipeline (e.g., `virgo`, `marcia`, `julia`, `appia`) |
| **Cataractae** | A stage within an aqueduct (implement → review → qa → delivery) |
| **Castellarius** | The overseer daemon — routes droplets, manages pipelines |
| **Recirculate** | Send a droplet back for revision |
| **Drought** | Idle state — maintenance hooks run here |
| **Filtration** | LLM refinement that sharpens a rough idea into well-specified droplets before they enter the pipeline |

Never say "drop/item/task/ticket/issue" for work units — always **droplet**.

## Installation

- **CLI:** `~/go/bin/ct`
- **Config:** `~/.cistern/cistern.yaml`
- **DB:** `~/.cistern/cistern.db`
- **Log:** `~/.cistern/castellarius.log`
- **Dashboard:** `http://192.168.0.138:5737` (ttyd)
- **Build:** `cd ~/cistern && COMMIT=$(git rev-parse --short HEAD) && PATH="/usr/local/go/bin:$PATH" go build -ldflags "-X main.version=${COMMIT} -X main.commit=${COMMIT}" -o ~/go/bin/ct ./cmd/ct/`

## Repos configured

| Repo | Prefix | Aqueducts |
|------|--------|-----------|
| cistern | `ci-` | virgo, marcia |
| ScaledTest | `st-` | julia, appia |
| PortfolioWebsite | `pw-` | anio |
| lobsterdog | `ld-` | claudia, palatinus, traiana |

**Aqueduct configs live ONLY in `~/.cistern/cistern.yaml` and `~/source/cistern/aqueduct/aqueduct.yaml`.** The `git_sync` drought hook copies the pipeline definition from each repo's `origin/main` into `~/.cistern/aqueduct/`, overwriting local edits — but since lobsterdog, ScaledTest, and PortfolioWebsite don't have their own aqueduct.yaml, they all share the cistern repo's. Never create `aqueduct/aqueduct.yaml` in lobsterdog, ScaledTest, or PortfolioWebsite — it was specifically removed from lobsterdog (ca958e9). To add or change aqueduct names, edit only `~/.cistern/cistern.yaml` (names, cataractae count, max_cataractae) and restart Castellarius.

## Worktree Rule

**Never edit `~/cistern` directly.** That's the primary clone — touching it corrupts all agent worktrees.

All manual Cistern work goes in the primary clone at `~/cistern`:
```bash
cd ~/cistern
git checkout -B lobsterdog-work origin/main   # Always sync before starting
```
ScaledTest worktree: `~/.cistern/sandboxes/ScaledTest/lobsterdog`

⚠️ Do NOT use `~/.cistern/sandboxes/cistern/` for manual work — that entire directory is owned by the Castellarius pipeline. A `lobsterdog` worktree there was removed 2026-04-02; placing anything inside the sandbox root risks collision with the pipeline.

## Pipeline

```
implement → review → qa → security-review → docs → delivery
```

| Priority | Meaning |
|----------|---------|
| 1 | Critical — maximum scrutiny (security review included) |
| 2 | Standard — default scrutiny |
| 3 | Low — minimal scrutiny |

## Adding a Droplet

**Always get explicit confirmation before filing any droplet.**

### Filtration first — every droplet goes through `ct filter`

Every droplet goes through filtration before filing, regardless of how clear the requirements seem. Filtration is a **thinking tool** that uses the grilling methodology underneath: the filter agent works a design tree in frontier rounds, asking you questions whose prerequisites are settled (each with a recommended answer), and you answer until the frontier is empty. Filing is always done separately with `ct droplet add` after the spec is grilled clear.

**⚠️ Filtration pitfalls:**
- **Run `ct filter` with provider API keys unset** — ct filter uses the opencode CLI, which manages its own credentials. Exporting API keys in the environment causes authentication conflicts.
- **Run `ct filter` from `~/cistern`** — it uses `--allowedTools` to read codebase context. Running from another directory gives the agent no context.
- **Pass the title only** — provide full context in the first interactive turn rather than via `--description`.
- **`--file` and `--repo` flags are removed** — they no longer exist on `ct filter`

**Troubleshooting `ct filter` failures:**
- **"Session not found" error**: Caused by `OPENCODE_SERVER_USERNAME`, `OPENCODE_SERVER_PASSWORD`, `OPENCODE_PID`, or `OPENCODE` env vars being set. These cause opencode to connect to an existing server session instead of creating a new one. `ct filter` automatically unsets these vars in the subprocess environment. If running `opencode run` manually, unset them: `unset OPENCODE_SERVER_USERNAME OPENCODE_SERVER_PASSWORD OPENCODE_PID OPENCODE`
- **Empty response**: The `opencode run --format json` command must produce NDJSON output on stdout. If stderr shows errors but stdout is empty, check that `--dangerously-skip-permissions` is included and the model is valid
- **Timeout (10 min default)**: Increase with `CT_FILTER_TIMEOUT` env var (in seconds): `CT_FILTER_TIMEOUT=600 ct filter --title "..."`

**Step 1 — Start (from ~/cistern, no provider API keys exported):**
```bash
cd ~/cistern
ct filter --title "Rough idea"
```

**Step 2 — Resume with answers (repeat until the frontier is empty):**
```bash
ct filter --resume <session-id> "answers and context..."
```

**Step 3 — When spec is approved and frontier is empty, file manually:**
```bash
# File each droplet explicitly, wiring deps with --depends-on
ct droplet add --title "First droplet" --repo <repo> --priority 2 \
  --description "..."
ct droplet add --title "Second droplet" --repo <repo> --priority 2 \
  --description "..." --depends-on <first-id>
```

**Rules:**
- Always run `ct filter` before any `ct droplet add`, every time, regardless of how clear the requirements seem
- Use `ct droplet add --filter` only when you want fire-and-forget behavior with no conversation
- Grilling methodology (adapted from mattpocock/skills): the agent works a design tree in frontier rounds — at each response it outputs the numbered spec plus every question whose prerequisites are settled, each with a recommended answer ("yes" accepts the recommendation). The session is done when the frontier is empty — nothing left silently assumed. Facts are looked up in the repo. Stop when the spec is concrete, not when a round count hits a number.
- After each round, present the updated spec as a numbered list with dependencies stated in plain text (e.g. "Droplet 2 requires droplet 1 to be delivered first")
- After each session, give a recommendation: ready to file, or needs more passes? Say why.
- Get explicit "yes" before filing any droplet
- File follow-up droplets with `--depends-on <id>` rather than injecting notes into flowing work

### Telegram buttons during filtration

When running on Telegram (`channel=telegram`), use inline buttons at decision points instead of waiting for typed responses. Send buttons with:

```bash
CHAT_ID=8569372105
openclaw message send --channel telegram --target "$CHAT_ID" \
  --message "<summary of current spec>" \
  --buttons '<buttons-json>'
```

**After each filtration round** (present the numbered spec, then):
```json
[ [{"text":"✅ Ready to file","callback_data":"filter:file"},
   {"text":"🔄 Another round","callback_data":"filter:continue"}],
  [{"text":"❌ Cancel","callback_data":"filter:cancel"}] ]
```

Button click responses arrive as `callback_data: filter:file` etc. Map them:
- `filter:file` → file each droplet manually with `ct droplet add`, wiring `--depends-on` explicitly; confirm each ID after filing
- `filter:continue` → ask what to refine, do another round
- `filter:cancel` → confirm cancellation, do not file

### Triage and wizard entrypoints

```bash
ct triage <droplet-id>                            # verify + brief a pooled/failed droplet
ct triage --title "rough idea"                    # triage an idea before filing
ct triage --resume <session-id> "answers..."      # continue the triage interview
ct wizard --title "provision the prod database"   # design a bash wizard for a human-only procedure
ct wizard --resume <session-id> "answers..."      # continue a wizard-design session
```

- **Triage** (adapted from mattpocock/skills "triage"): fetches the droplet and its notes; the agent categorises (bug/enhancement), verifies the claim against the codebase before recommending (reproduce bugs, run the commands), grills what is vague, and rewrites descriptions as agent briefs (Category / Verified / Goal / Acceptance criteria / Constraints / Context). Outcome states: needs-triage, needs-info, ready-for-agent, ready-for-human, wontfix (recommends `ct droplet cancel`).
- **Wizard** (adapted from mattpocock/skills "wizard"): for steps only a human can do — credentials, CI secrets, unfamiliar dashboards, one-off migrations. The agent scopes stages from the repo (.env files, `.github/workflows/*` secrets references), maps each stage's journey, and authors a bash script from the mp-wizard template library. Never invent steps that may not exist; verify statically (bash -n, shellcheck).

Both use the same provider/session machinery as `ct filter` and support `--output-format json`.

## Key Commands

```bash
# Status
ct status
ct droplet list
ct droplet list --repo <repo>
ct droplet show <id>

# Manage
ct droplet restart <id>                           # Restart from current cataractae
ct droplet restart <id> --cataractae delivery     # Re-enter at a specific cataractae
ct droplet pool <id>
ct droplet cancel <id> --reason "..."
ct droplet note <id> "..."
ct droplet deps <id> --add <dep-id>

# Daemon
ct castellarius start/status
journalctl --user -u cistern-castellarius -f
systemctl --user restart cistern-castellarius

# Cataractae
ct cataractae list
ct cataractae generate

# Dashboard (restart after rebuild)
systemctl --user restart cistern-web.service
```

## Infrastructure

- Castellarius: systemd user service `cistern-castellarius.service` (Restart=always)
- Auth: Opencode CLI manages its own credentials — no provider API keys needed in the service environment
- `start-castellarius.sh` just runs `exec ct castellarius start` — no credential setup
- Web dashboard: port 5737, managed by `cistern-web.service`
- Self-restart: git_sync drought hook + binary mtime detection → os.Exit(0) → systemd restarts

## Troubleshooting

Run `ct doctor` first — it checks daemon status and common config issues automatically.

| Symptom | Check |
|---------|-------|
| Castellarius not running | `systemctl --user status cistern-castellarius` → start it |
| Castellarius hung/log quiet | `ct doctor` flags it automatically; manual check: `cat ~/.cistern/castellarius.health` (shows last tick time + poll interval) |
| Sessions failing auth | `ct doctor` checks agent CLI auth automatically; also verify environment variables are correct for the configured provider |
| Droplet stuck | `ct droplet show <id>` — check notes; `ct droplet restart <id>` |
| Logs | `journalctl --user -u cistern-castellarius -f` or `cat ~/.cistern/castellarius.log` |
| Dashboard stale after rebuild | `systemctl --user restart cistern-web.service` |
| Binary out of date | Rebuild: `cd ~/cistern && COMMIT=$(git rev-parse --short HEAD) && PATH="/usr/local/go/bin:$PATH" go build -ldflags "-X main.version=${COMMIT} -X main.commit=${COMMIT}" -o ~/go/bin/ct ./cmd/ct/` |
