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
ct castellarius start       # Start the daemon
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

## Web Dashboard (optional)

```bash
cp ~/.cistern/sandboxes/cistern/lobsterdog/cistern-web.service \
   ~/.config/systemd/user/
systemctl --user daemon-reload
systemctl --user enable --now cistern-web
# Visit http://localhost:5737
```
