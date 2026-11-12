#!/usr/bin/env bash
set -euo pipefail

# Cistern installer
# Usage: curl -sSf https://raw.githubusercontent.com/MichielDean/Cistern/main/install.sh | bash

REPO='github.com/MichielDean/cistern'
CT_DIR="${HOME}/.cistern"
MIN_GO_VERSION="1.22"

DEPLOY_ONLY=false
if [ "${1:-}" = "--deploy-only" ]; then
  DEPLOY_ONLY=true
fi

# --- colors (disabled if not a tty) ---
if [ -t 1 ]; then
  RED='\033[0;31m'
  GREEN='\033[0;32m'
  YELLOW='\033[1;33m'
  BLUE='\033[0;34m'
  BOLD='\033[1m'
  NC='\033[0m'
else
  RED='' GREEN='' YELLOW='' BLUE='' BOLD='' NC=''
fi

info()  { printf "${GREEN}>>>${NC} %s\n" "$*"; }
warn()  { printf "${YELLOW}warning:${NC} %s\n" "$*"; }
error() { printf "${RED}error:${NC} %s\n" "$*" >&2; }
fatal() { error "$@"; exit 1; }
step()  { printf "\n${BLUE}━━━${NC} ${BOLD}%s${NC}\n" "$*"; }

# resolve_gobin returns the directory where `go install` places binaries.
resolve_gobin() {
  local gobin
  gobin="$(go env GOBIN 2>/dev/null)"
  if [ -z "${gobin}" ]; then
    gobin="$(go env GOPATH 2>/dev/null)/bin"
  fi
  echo "${gobin}"
}

# --- check_go: require go >= MIN_GO_VERSION ---
check_go() {
  if ! command -v go &>/dev/null; then
    fatal "Go is not installed. Install Go ${MIN_GO_VERSION}+ from https://go.dev/dl/ and try again."
  fi

  local go_version
  go_version="$(go version | sed -E 's/^go version go([0-9]+\.[0-9]+).*/\1/')"

  if [ -z "${go_version}" ]; then
    warn "Could not parse Go version. Continuing anyway."
    return
  fi

  local go_major go_minor min_major min_minor
  go_major="${go_version%%.*}"
  go_minor="${go_version#*.}"
  min_major="${MIN_GO_VERSION%%.*}"
  min_minor="${MIN_GO_VERSION#*.}"

  if [ "${go_major}" -lt "${min_major}" ] || \
     { [ "${go_major}" -eq "${min_major}" ] && [ "${go_minor}" -lt "${min_minor}" ]; }; then
    fatal "Go ${go_version} found, but ${MIN_GO_VERSION}+ is required. Upgrade at https://go.dev/dl/"
  fi

  info "Go ${go_version} ✓"
}

# --- check_deps: verify runtime dependencies ---
check_deps() {
  local missing=()

  # tmux is required to spawn agent sessions.
  if ! command -v tmux &>/dev/null; then
    missing+=("tmux")
  else
    info "tmux $(tmux -V | awk '{print $2}') ✓"
  fi

  # opencode CLI is required to run AI steps.
  if ! command -v opencode &>/dev/null; then
    missing+=("opencode")
  else
    info "opencode CLI ✓"
  fi

  # git is required for sandbox management.
  if ! command -v git &>/dev/null; then
    missing+=("git")
  else
    info "git $(git --version | awk '{print $3}') ✓"
  fi

  if [ ${#missing[@]} -gt 0 ]; then
    printf "\n"
    error "Missing required dependencies:"
    for dep in "${missing[@]}"; do
      printf "  • %s\n" "${dep}" >&2
    done
    printf "\n"
    if [[ " ${missing[*]} " == *"tmux"* ]]; then
      printf "  Install tmux:  apt install tmux  /  brew install tmux\n" >&2
    fi
    if [[ " ${missing[*]} " == *"opencode"* ]]; then
      printf "  Install opencode:  go install github.com/opencode-ai/opencode@latest\n" >&2
    fi
    printf "\n"
    fatal "Install missing dependencies and re-run."
  fi
}

# --- configure_git: ensure go module fetching works for private repos ---
configure_git() {
  # Rewrite HTTPS GitHub URLs to SSH so `go install` works with SSH key auth.
  local existing
  existing="$(git config --global url."git@github.com:".insteadOf 2>/dev/null || true)"
  if [ "${existing}" != "https://github.com/" ]; then
    git config --global url."git@github.com:".insteadOf "https://github.com/"
    info "Git configured to use SSH for GitHub ✓"
  else
    info "Git SSH rewrite already configured ✓"
  fi

  # Tell the Go toolchain to skip the public checksum database for this repo.
  export GOPRIVATE="${GOPRIVATE:+${GOPRIVATE},}github.com/MichielDean/cistern"
  info "GOPRIVATE set ✓"
}

# --- assert_ct_version: fail install if installed binary diverges from local source ---
# Compares the version reported by the installed binary against `git describe`
# from the local checkout. Only called in local mode, where git ground truth exists.
assert_ct_version() {
  local ct_bin="${1}" script_dir="${2}"

  local expected_version
  # Same expression the local build injects via -ldflags (see install_ct_from_local),
  # so 'dev' in git-less environments matches by construction while any real
  # source/binary divergence still fails.
  expected_version="$(git -C "${script_dir}" describe --tags --always 2>/dev/null || echo 'dev')"

  local installed_version
  if ! installed_version="$("${ct_bin}" version 2>/dev/null | sed 's/^ct //')"; then
    fatal "could not read version from installed binary at ${ct_bin}"
  fi

  if [ "${installed_version}" != "${expected_version}" ]; then
    fatal "installed ct version '${installed_version}' does not match source version '${expected_version}' — install aborted"
  fi

  info "ct version matches source (${expected_version}) ✓"
}

# --- install_ct_from_local: build ct from the source tree the installer lives in ---
# Builds ./cmd/ct from script_dir, injecting the git-derived version via -ldflags
# so `ct version` equals `git describe --tags --always` by construction.
install_ct_from_local() {
  local script_dir="${1}" ct_bin="${2}"

  local ct_version
  ct_version="$(git -C "${script_dir}" describe --tags --always 2>/dev/null || echo 'dev')"
  info "Building ct from local source (${script_dir}) at ${ct_version}..."

  (cd "${script_dir}" && CGO_ENABLED=1 go build -o "${ct_bin}" -ldflags "-X main.version=${ct_version}" ./cmd/ct)
}

# --- install_ct: build from local checkout when present, otherwise go install @latest ---
install_ct() {
  local script_dir
  script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

  local gobin ct_bin
  gobin="$(resolve_gobin)"
  ct_bin="${gobin}/ct"

  # Local mode requires BOTH a go.mod in script_dir AND a cmd/ct directory.
  # Under curl|bash, BASH_SOURCE points at the piped script (often just '.'),
  # so script_dir collapses to cwd; any Go project dir would then falsely
  # trigger local mode and try to build ./cmd/ct from the wrong tree. Requiring
  # cmd/ct to exist in the same tree rules that out. (ci-fxuo8-nzoar)
  if [[ -f "${script_dir}/go.mod" && -d "${script_dir}/cmd/ct" ]]; then
    # Local mode: we are running inside a repo checkout. Build from this source
    # tree so the binary always matches the code the user is looking at — the
    # @latest form resolves via the Go module proxy, which lags origin/main.
    install_ct_from_local "${script_dir}" "${ct_bin}"
    assert_ct_version "${ct_bin}" "${script_dir}"
  else
    # Remote mode: piped via curl|bash with no repo checkout — fall back to
    # the Go module proxy. Version assertion is skipped: there is no local
    # ground truth to compare against.
    info "No repo checkout detected — installing ct from the Go module proxy (@latest)..."
    CGO_ENABLED=1 GOPRIVATE="github.com/MichielDean/cistern" go install "${REPO}/cmd/ct@latest"
    warn "skipping version assertion — remote build has no local git ground truth"
  fi

  if [ ! -x "${ct_bin}" ]; then
    fatal "ct binary not found at ${ct_bin} after install — check your Go setup"
  fi

  local installed_version
  installed_version="$(${ct_bin} version 2>/dev/null | sed 's/^ct //' || echo 'dev')"
  info "ct ${installed_version} installed at ${ct_bin}"
}

# --- ensure_path: add Go bin dir to shell profile if not already there ---
ensure_path() {
  local gobin
  gobin="$(resolve_gobin)"

  # Detect the user's shell profile file.
  local profile=""
  local shell_name
  shell_name="$(basename "${SHELL:-bash}")"
  case "${shell_name}" in
    zsh)  profile="${ZDOTDIR:-${HOME}}/.zshrc" ;;
    bash) profile="${HOME}/.bashrc" ;;
    *)    profile="${HOME}/.profile" ;;
  esac

  local export_line="export PATH=\"\${PATH}:${gobin}\""

  if grep -qF "${gobin}" "${profile}" 2>/dev/null; then
    info "${gobin} already in ${profile} ✓"
  else
    printf '\n# Added by Cistern installer\n%s\n' "${export_line}" >> "${profile}"
    info "Added ${gobin} to ${profile}"
    # Export for the remainder of this script session.
    export PATH="${PATH}:${gobin}"
  fi
}

# --- setup_dirs: create ~/.cistern structure ---
setup_dirs() {
  info "Setting up ${CT_DIR}..."
  mkdir -p "${CT_DIR}/sandboxes" "${CT_DIR}/logs"
}

# --- deploy_skills: deploy skills + cataractae definitions from the local
# checkout into ~/.cistern. Out-of-band guarantee that the runtime reads current
# content regardless of whether the git_sync drought hook ever runs.
deploy_skills() {
  local skills_dst="${CT_DIR}/skills"
  local cataractae_dst="${CT_DIR}/cataractae"
  mkdir -p "${skills_dst}" "${cataractae_dst}"

  info "Deploying skills and cataractae definitions..."

  # Skills ship in two trees: internal/skills/ (mp-* and cistern-* skills) and
  # skills/ (shared skills like cistern/ and external-repo-guidelines).
  # Each skill is a directory containing SKILL.md. Clean-replace each so
  # removed skills do not linger.
  local script_dir
  script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

  for prefix in internal/skills skills; do
    local src_tree="${script_dir}/${prefix}"
    if [ ! -d "${src_tree}" ]; then
      continue
    fi
    for skill_dir in "${src_tree}"/*/; do
      local name
      name="$(basename "${skill_dir}")"
      if [ ! -f "${skill_dir}/SKILL.md" ]; then
        continue
      fi
      rm -rf "${skills_dst}/${name}"
      cp -rf "${skill_dir}" "${skills_dst}/${name}"
    done
  done

  # Cataractae definitions (PERSONA.md + INSTRUCTIONS.md per identity) ship in
  # cataractae/<identity>/. Deploy each so the runtime generates AGENTS.md
  # from current definitions.
  local cataractae_src="${script_dir}/cataractae"
  if [ -d "${cataractae_src}" ]; then
    for identity_dir in "${cataractae_src}"/*/; do
      local identity
      identity="$(basename "${identity_dir}")"
      if [ ! -f "${identity_dir}/PERSONA.md" ] && [ ! -f "${identity_dir}/INSTRUCTIONS.md" ]; then
        continue
      fi
      rm -rf "${cataractae_dst}/${identity}"
      cp -rf "${identity_dir}" "${cataractae_dst}/${identity}"
    done
  fi

  # Regenerate AGENTS.md from the deployed cataractae definitions + aqueduct config.
  # ct cataractae generate reads from ~/.cistern/cataractae/ and writes AGENTS.md
  # into each identity dir. Requires the ct binary and cistern.yaml (both
  # established by earlier steps).
  local gobin
  gobin="$(resolve_gobin)"
  if command -v "${gobin}/ct" &>/dev/null && [ -f "${CT_DIR}/cistern.yaml" ]; then
    if "${gobin}/ct" cataractae generate >/dev/null 2>&1; then
      info "Cataractae AGENTS.md files regenerated ✓"
    else
      warn "ct cataractae generate failed — AGENTS.md files may be stale. Run manually: ct cataractae generate"
    fi
  else
    warn "ct binary or cistern.yaml not ready — skipping AGENTS.md regeneration. Run manually: ct cataractae generate"
  fi

  local count
  count=$(find "${skills_dst}" -name SKILL.md | wc -l)
  info "Deployed ${count} skill(s) to ${skills_dst}"
}

# --- create_config: initialize cistern if not already done ---
create_config() {
  local cfg="${CT_DIR}/cistern.yaml"

  if [ -f "${cfg}" ]; then
    info "Config already exists at ${cfg} — skipping"
    return
  fi

  info "Initializing Cistern..."
  local gobin
  gobin="$(resolve_gobin)"
  "${gobin}/ct" init > /dev/null
  info "Config and aqueduct files created"
}

# --- setup_systemd_service: install and enable the Castellarius as a user service ---
setup_systemd_service() {
  # Requires systemd with user session support.
  if ! command -v systemctl &>/dev/null; then
    warn "systemctl not found — skipping service install. Run the Castellarius manually: ct castellarius start"
    return
  fi
  if ! systemctl --user status &>/dev/null 2>&1; then
    warn "systemd user session not available — skipping service install."
    return
  fi

  local gobin
  gobin="$(resolve_gobin)"
  local ct_bin="${gobin}/ct"
  local service_dir="${HOME}/.config/systemd/user"
  local service_file="${service_dir}/cistern-castellarius.service"

  mkdir -p "${service_dir}"

  # Write service file with resolved paths for this user.
  cat > "${service_file}" <<UNIT
[Unit]
Description=Cistern Castellarius — aqueduct scheduler
After=network.target

[Service]
Type=simple
ExecStart=${ct_bin} castellarius start

# Always restart — covers both intentional exits (self-restart on config change)
# and unexpected crashes. systemd is the process supervisor.
Restart=always
RestartSec=5

# Crash-loop protection: stop after 10 failures in 2 minutes.
# Reset manually: systemctl --user reset-failed cistern-castellarius
StartLimitIntervalSec=120
StartLimitBurst=10
StartLimitAction=none

# Graceful shutdown — let in-flight agents settle.
TimeoutStopSec=15
KillMode=mixed
KillSignal=SIGTERM

# Persistent log alongside the journal.
StandardOutput=append:${HOME}/.cistern/castellarius.log
StandardError=append:${HOME}/.cistern/castellarius.log

Environment=HOME=${HOME}
Environment=PATH=${gobin}:/usr/local/go/bin:/usr/local/bin:/usr/bin:/bin

[Install]
WantedBy=default.target
UNIT

  info "Castellarius service file written to ${service_file}"

  systemctl --user daemon-reload

  # Enable linger so the service survives SSH session end and starts on boot.
  if command -v loginctl &>/dev/null; then
    if loginctl show-user "${USER}" 2>/dev/null | grep -q "Linger=no"; then
      loginctl enable-linger "${USER}" && info "Linger enabled for ${USER} ✓"
    else
      info "Linger already enabled ✓"
    fi
  fi

  systemctl --user enable cistern-castellarius
  info "cistern-castellarius.service enabled ✓"
  info "Start it after configuring your repos: systemctl --user start cistern-castellarius"
}

# --- setup_web_service: install and enable the web dashboard as a user service ---
setup_web_service() {
  if ! command -v systemctl &>/dev/null; then
    return
  fi
  if ! systemctl --user status &>/dev/null 2>&1; then
    return
  fi

  local gobin
  gobin="$(resolve_gobin)"
  local ct_bin="${gobin}/ct"
  local service_dir="${HOME}/.config/systemd/user"
  local service_file="${service_dir}/cistern-web.service"

  # Source the repo's unit file rather than maintaining a duplicate here.
  # The repo file ships with /home/lobsterdog paths; substitute them for this
  # user's HOME and gobin so the installer stays portable and the repo file
  # remains the single source of truth (no drift between the two).
  local script_dir
  script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
  local repo_unit="${script_dir}/cistern-web.service"
  if [[ ! -f "${repo_unit}" ]]; then
    warn "cistern-web.service not found at ${repo_unit} — skipping web service install"
    return
  fi

  mkdir -p "${service_dir}"

  sed \
    -e "s|/home/lobsterdog/go/bin/ct|${ct_bin}|g" \
    -e "s|/home/lobsterdog/go/bin|${gobin}|g" \
    -e "s|/home/lobsterdog/\.cistern|${HOME}/.cistern|g" \
    -e "s|HOME=/home/lobsterdog|HOME=${HOME}|g" \
    "${repo_unit}" > "${service_file}"

  info "Web dashboard service file written to ${service_file}"

  systemctl --user daemon-reload
  systemctl --user enable cistern-web
  info "cistern-web.service enabled ✓"
}

# --- add_shell_completion: write completion for bash/zsh ---
add_shell_completion() {
  local gobin ct_bin shell_name
  gobin="$(resolve_gobin)"
  ct_bin="${gobin}/ct"
  shell_name="$(basename "${SHELL:-bash}")"

  case "${shell_name}" in
    bash)
      local comp_dir="${HOME}/.local/share/bash-completion/completions"
      mkdir -p "${comp_dir}"
      if "${ct_bin}" completion bash > "${comp_dir}/ct" 2>/dev/null; then
        info "Bash completion installed"
      fi
      ;;
    zsh)
      local comp_dir="${HOME}/.zsh/completions"
      mkdir -p "${comp_dir}"
      if "${ct_bin}" completion zsh > "${comp_dir}/_ct" 2>/dev/null; then
        info "Zsh completion installed"
      fi
      if ! grep -q 'fpath.*\.zsh/completions' "${HOME}/.zshrc" 2>/dev/null; then
        warn "Add to your .zshrc:  fpath=(~/.zsh/completions \$fpath)"
      fi
      ;;
    *)
      warn "Shell completion not configured for ${shell_name}. Run: ct completion --help"
      ;;
  esac
}

# --- print_success ---
print_success() {
  printf "\n"
  printf "${GREEN}${BOLD}✓ Cistern installed${NC}\n"
  printf "\n"
  printf "${BOLD}Next steps:${NC}\n"
  printf "\n"
  printf "  1. Edit your config:\n"
  printf "     ${BLUE}%s/cistern.yaml${NC}\n" "${CT_DIR}"
  printf "     Add at least one repo with a URL and operator names.\n"
  printf "\n"
  printf "  2. Add a droplet:\n"
  printf "     ${BLUE}ct droplet add --title \"My first task\" --repo <repo-name>${NC}\n"
  printf "\n"
  printf "  3. Wake the Castellarius:\n"
  if command -v systemctl &>/dev/null && systemctl --user is-enabled cistern-castellarius &>/dev/null 2>&1; then
    printf "     ${BLUE}systemctl --user start cistern-castellarius${NC}\n"
    printf "\n"
    printf "  Manage the Castellarius:\n"
    printf "     ${BLUE}systemctl --user status cistern-castellarius${NC}  — health check\n"
    printf "     ${BLUE}systemctl --user restart cistern-castellarius${NC} — restart\n"
    printf "     ${BLUE}tail -f ~/.cistern/castellarius.log${NC}           — live log\n"
  else
    printf "     ${BLUE}ct castellarius start${NC}\n"
  fi
  printf "\n"
  printf "${BOLD}Paths:${NC}\n"
  printf "  Config:     %s/cistern.yaml\n" "${CT_DIR}"
  printf "  Aqueducts:  %s/aqueduct/\n" "${CT_DIR}"
  printf "  Cataractae: %s/cataractae/\n" "${CT_DIR}"
  printf "  Queue DB:   %s/cistern.db\n" "${CT_DIR}"
  printf "  Sandboxes:  %s/sandboxes/\n" "${CT_DIR}"
  printf "\n"
  printf "${BOLD}Docs:${NC} https://github.com/MichielDean/Cistern\n"
  printf "\n"
}

main() {
  if $DEPLOY_ONLY; then
    step "Deploying skills and cataractae definitions"
    deploy_skills
    echo "==> Deployment complete (deploy-only mode)."
    exit 0
  fi

  printf "${BOLD}Cistern Installer${NC}\n\n"
  step "Checking Go"
  check_go
  step "Checking dependencies"
  check_deps
  step "Configuring git"
  configure_git
  step "Installing ct"
  install_ct
  step "Updating PATH"
  ensure_path
  step "Setting up directories"
  setup_dirs
  step "Initializing config"
  create_config
  step "Deploying skills and cataractae definitions"
  deploy_skills
  step "Shell completion"
  add_shell_completion
  step "Setting up systemd service"
  setup_systemd_service
  step "Setting up web dashboard service"
  setup_web_service
  print_success
}

main "$@"
