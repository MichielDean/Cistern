# Cistern Command Reference

## Droplet Management

```bash
ct droplet list                          # All droplets
ct droplet list --status <status>        # Filter: pending|running|done|failed
ct droplet list --repo <repo>            # Filter by repo
ct droplet show <id>                     # Full detail
ct droplet add --title "..." --repo <r>  # Add new droplet
ct droplet restart <id>                  # Retry failed droplet
ct droplet escalate <id>                 # Bump priority
```

### Add Options

| Flag | Values | Default |
|------|--------|---------|
| `--title` | string (required) | — |
| `--repo` | repo name | — |
| `--complexity` | trivial / standard / full / critical | full |
| `--priority` | 1–4 (4 = highest) | 2 |
| `--depends-on` | droplet ID | — |
| `--description` | multiline text | — |

### Complexity Matrix

| Level | Code | Stages skipped |
|-------|------|---------------|
| trivial | 1 | review, qa |
| standard | 2 | qa |
| full | 3 | none (default) |
| critical | 4 | none + human approval required |

## Castellarius Daemon

```bash
ct castellarius start
ct castellarius stop
ct castellarius status
ct castellarius restart

# System service
systemctl --user start cistern-castellarius
systemctl --user status cistern-castellarius
journalctl --user -u cistern-castellarius -f   # Live log tail
cat ~/.cistern/castellarius.log                # Log file
```

## Cataractae (Pipeline Stages)

```bash
ct cataractae list               # All stages across all aqueducts
ct cataractae list --aqueduct <name>
ct cataractae generate           # Generate any missing stage configs
```

## Aqueducts

```bash
ct aqueduct list                 # All configured aqueducts
ct aqueduct show <name>
```

## Dashboard

```bash
ct dashboard                     # Launch TUI (requires active tmux session)
```

Web dashboard (if configured): `http://<host>:5737`

## Status

```bash
ct status                        # High-level pipeline health
```

## Config

Default config: `~/.cistern/cistern.yaml`
Default DB: `~/.cistern/cistern.db`
