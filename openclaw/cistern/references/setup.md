# Cistern Setup & Installation

## Install

```bash
curl -fsSL https://raw.githubusercontent.com/MichielDean/Cistern/main/install.sh | bash
```

Or clone and build manually:

```bash
git clone https://github.com/MichielDean/Cistern.git ~/cistern
cd ~/cistern
PATH="/usr/local/go/bin:$PATH" go build -o ~/go/bin/ct ./cmd/ct/
```

## First Run

```bash
ct --help                   # Verify binary works
ct init                     # Create ~/.cistern/ with default config, credentials file, and startup script
```

Set up credentials (choose one):

**Option A: OAuth (Recommended for Claude users)**

Run the Claude CLI once to authenticate — it creates `~/.claude/.credentials.json` with your OAuth token. Castellarius reads this automatically and refreshes on expiry:

```bash
claude                      # Authenticate once
ct castellarius start       # Reads OAuth token automatically
ct status                   # Confirm running
```

**Option B: API Key Authentication**

Add `ANTHROPIC_API_KEY` to `~/.cistern/env`:

```bash
echo 'ANTHROPIC_API_KEY=sk-ant-...' >> ~/.cistern/env
echo 'GH_TOKEN=ghp_...' >> ~/.cistern/env
chmod 600 ~/.cistern/env
ct castellarius start       # Reads from ~/.cistern/env
ct status                   # Confirm running
```

## Configure Repos

Edit `~/.cistern/cistern.yaml` to register repos:

```yaml
repos:
  - name: MyRepo
    path: ~/projects/MyRepo
    prefix: mr
    aqueducts: [virgo]
```

Reload by restarting Castellarius.

## Rebuild Binary

If you have local commits or need to rebuild from a worktree:

```bash
cd <worktree-path>
PATH="/usr/local/go/bin:$PATH" go build -o ~/go/bin/ct ./cmd/ct/
```

## Systemd Service (optional)

Enable auto-start on login:

```bash
cp ~/.cistern/sandboxes/cistern/lobsterdog/cistern-castellarius.service \
   ~/.config/systemd/user/
systemctl --user daemon-reload
systemctl --user enable --now cistern-castellarius
```

The service is configured for graceful shutdown: when systemd sends SIGTERM, the Castellarius stops accepting new work but continues processing in-flight droplets until they signal an outcome (or until a configurable drain timeout is reached). The default drain timeout is 5 minutes — configure it in `~/.cistern/cistern.yaml` with `drain_timeout_minutes`. Make sure systemd's `TimeoutStopSec` is set >= drain timeout + buffer (the default service file uses 360 seconds, suitable for a 5-minute drain).

## Web Dashboard (optional)

```bash
cp ~/.cistern/sandboxes/cistern/lobsterdog/cistern-web.service \
   ~/.config/systemd/user/
systemctl --user daemon-reload
systemctl --user enable --now cistern-web
# Visit http://localhost:5737
```
