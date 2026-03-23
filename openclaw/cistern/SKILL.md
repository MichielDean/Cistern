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
| **Filtration** | Optional LLM refinement step before implementation |

## Key Commands

```bash
# Status
ct status                        # Pipeline overview
ct droplet list                  # All droplets
ct droplet list --status pending # Filter by status
ct droplet show <id>             # Detail view

# Add work
ct droplet add --title "Fix login bug" --repo ScaledTest
ct droplet add --title "..." --repo <repo> --complexity standard --priority 2

# Daemon control
ct castellarius start
ct castellarius status
journalctl --user -u cistern-castellarius -f   # Live logs

# Cataractae
ct cataractae list               # List all stages
ct cataractae generate           # Generate missing stages

# Dashboard
ct dashboard                     # Live TUI (requires tmux)
```

See [references/commands.md](references/commands.md) for the full command reference.

## Adding a Droplet

```bash
ct droplet add \
  --title "Short imperative description" \
  --repo <repo-name> \
  [--complexity trivial|standard|full|critical] \
  [--priority 1-4] \
  [--depends-on <id>]
```

Complexity controls which stages run:
- `trivial` (1): skip review + qa — fast lane
- `standard` (2): skip qa
- `full` (3): all stages — default
- `critical` (4): requires human approval before merge

**Rule:** Never file a droplet without the user's confirmation first.

## Pipeline

```
implement → adversarial-review → qa → security-review → docs → delivery
```

Castellarius routes each droplet through the stages configured for its aqueduct. Completed droplets move to the next stage automatically; failed ones can be restarted.

```bash
ct droplet restart <id>   # Retry a failed droplet
ct droplet escalate <id>  # Escalate priority
```

## Troubleshooting

| Symptom | Check |
|---------|-------|
| Castellarius not running | `ct castellarius status` → `ct castellarius start` |
| Droplet stuck in a stage | `ct droplet show <id>` — check last error |
| Logs for a failed stage | `journalctl --user -u cistern-castellarius -f` |
| Binary out of date | Rebuild: see [references/setup.md](references/setup.md) |

See [references/troubleshooting.md](references/troubleshooting.md) for detailed recovery workflows.

## Worktree Rule

**Never edit `~/cistern` directly.** That's the primary clone — touching it corrupts all agent worktrees.

All manual work goes in the dedicated lobsterdog worktree:
```bash
cd ~/.cistern/sandboxes/cistern/lobsterdog
git checkout -B lobsterdog-work origin/main   # Sync before starting
```
