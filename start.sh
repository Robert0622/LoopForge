#!/usr/bin/env bash
# One-command local start: detect what is missing, install what we can, start
# LoopForge with Docker Compose, wait until healthy, print the login.
#
#   ./start.sh            start (builds this checkout; release tags pull the published image)
#   ./start.sh -y         same, install missing dependencies without prompting
#   ./start.sh doctor     check the host, then run the end-to-end self-test
#   ./start.sh --help     all commands
set -euo pipefail
root="$(cd "$(dirname "$0")" && pwd)"
cd "$root"

YES="${LOOPFORGE_YES:-0}"
SOURCE_IMAGE="loopforge:source"
PUBLISHED_REPO="ghcr.io/tencent/loopforge"

usage() {
  cat <<'EOF'
Usage: ./start.sh [-y|--yes] [command]

  (none) | up   Install what is missing, start LoopForge, wait until healthy
  doctor        Check the host; when LoopForge is running, also run the
                end-to-end self-test (login, demo run, share-link approval)
  warm          Build the default sandbox image now (first real run skips it)
  logs          Follow LoopForge logs
  down          Stop LoopForge (data is kept in the loopforge-data volume)
  restart       down + up
  reset         Stop LoopForge and DELETE its data volume (fresh start)
  pull          Use the published image instead of building this checkout
  build         Force a rebuild of this checkout

  -y, --yes     Install missing dependencies without prompting

Settings live in .env (created from .env.example on first run). Variables set
on the command line win over .env, e.g. LOOPFORGE_PORT=9090 ./start.sh
EOF
}

log()  { printf '%s\n' "$*"; }
warn() { printf 'warning: %s\n' "$*" >&2; }
die()  { printf 'error: %s\n' "$*" >&2; exit 1; }

confirm() {
  local prompt="$1"
  if [[ "$YES" == "1" ]]; then
    return 0
  fi
  if [[ ! -t 0 ]]; then
    die "$prompt (re-run with -y / LOOPFORGE_YES=1 for non-interactive install)"
  fi
  local reply=""
  read -r -p "$prompt [y/N] " reply || true
  [[ "$reply" == "y" || "$reply" == "Y" || "$reply" == "yes" ]]
}

have() { command -v "$1" >/dev/null 2>&1; }

sudo_cmd() {
  if [[ "$(id -u)" -eq 0 ]]; then
    "$@"
  elif have sudo; then
    sudo "$@"
  else
    die "need root or sudo to install: $*"
  fi
}

os_family() {
  case "$(uname -s)" in
    Linux*)  echo linux ;;
    Darwin*) echo macos ;;
    MINGW*|MSYS*|CYGWIN*) echo windows ;;
    *)       echo other ;;
  esac
}

# Loads .env without overriding variables already set in the environment.
ensure_env_file() {
  if [[ ! -f .env && -f .env.example ]]; then
    cp .env.example .env
    log "created .env from .env.example"
  fi
  if [[ -f .env ]]; then
    local line key value
    while IFS= read -r line || [[ -n "$line" ]]; do
      line="${line#"${line%%[![:space:]]*}"}"
      [[ -z "$line" || "$line" == \#* ]] && continue
      line="${line#export }"
      [[ "$line" == *=* ]] || continue
      key="${line%%=*}"
      value="${line#*=}"
      [[ "$key" =~ ^[A-Za-z_][A-Za-z0-9_]*$ ]] || continue
      value="${value%\"}"; value="${value#\"}"
      value="${value%\'}"; value="${value#\'}"
      if [[ -z "${!key+x}" ]]; then
        export "$key=$value"
      fi
    done < .env
  fi
  export LOOPFORGE_DEMO_MODE="${LOOPFORGE_DEMO_MODE:-true}"
  export LOOPFORGE_PORT="${LOOPFORGE_PORT:-8080}"
}

# Writes KEY=VALUE into .env, replacing an existing (possibly commented) line.
set_env_value() {
  local key="$1" value="$2"
  [[ -f .env ]] || touch .env
  if grep -qE "^[#[:space:]]*${key}=" .env; then
    local tmp
    tmp="$(mktemp)"
    awk -v k="$key" -v v="$value" 'BEGIN{done=0} { if (!done && $0 ~ "^[#[:space:]]*" k "=") { print k "=" v; done=1 } else print }' .env > "$tmp"
    cat "$tmp" > .env
    rm -f "$tmp"
  else
    printf '%s=%s\n' "$key" "$value" >> .env
  fi
}

install_pkg_linux() {
  local pkg="$1"
  if have apt-get; then
    sudo_cmd apt-get update -y
    sudo_cmd DEBIAN_FRONTEND=noninteractive apt-get install -y "$pkg"
  elif have dnf; then
    sudo_cmd dnf install -y "$pkg"
  elif have yum; then
    sudo_cmd yum install -y "$pkg"
  elif have zypper; then
    sudo_cmd zypper install -y "$pkg"
  elif have pacman; then
    sudo_cmd pacman -Sy --noconfirm "$pkg"
  else
    return 1
  fi
}

ensure_curl() {
  have curl && return 0
  log "curl is missing"
  case "$(os_family)" in
    linux)
      confirm "Install curl with the system package manager?" || die "curl is required"
      install_pkg_linux curl || die "could not install curl"
      ;;
    macos)
      if have brew; then
        confirm "Install curl with Homebrew?" || die "curl is required"
        brew install curl
      else
        die "curl is required (install Xcode CLT or Homebrew first)"
      fi
      ;;
    *) die "curl is required" ;;
  esac
  have curl || die "curl still missing after install"
}

ensure_git() {
  have git && return 0
  log "git is missing (optional for start, needed to clone updates)"
  case "$(os_family)" in
    linux)
      confirm "Install git?" || { warn "continuing without git"; return 0; }
      install_pkg_linux git || warn "could not install git"
      ;;
    macos)
      if have brew; then
        confirm "Install git with Homebrew?" || return 0
        brew install git || true
      else
        warn "install git via Xcode CLT: xcode-select --install"
      fi
      ;;
  esac
}

# When the user is not in the docker group yet, prefer `sudo docker`.
DOCKER=(docker)
docker_bin() { "${DOCKER[@]}" "$@"; }

prefer_sudo_docker_if_needed() {
  if command -v docker >/dev/null 2>&1 && docker info >/dev/null 2>&1; then
    DOCKER=(docker)
    return 0
  fi
  if have sudo && sudo docker info >/dev/null 2>&1; then
    warn "current user cannot talk to Docker socket; using sudo docker for this session"
    warn "to drop sudo later: sudo usermod -aG docker \"$USER\" && newgrp docker"
    DOCKER=(sudo docker)
    return 0
  fi
  return 1
}

docker_ok() {
  have docker && { docker_bin info >/dev/null 2>&1 || prefer_sudo_docker_if_needed; }
}

compose_ok() {
  docker_bin compose version >/dev/null 2>&1 || have docker-compose
}

compose_cmd() {
  if docker_bin compose version >/dev/null 2>&1; then
    COMPOSE=("${DOCKER[@]}" compose)
  elif have docker-compose; then
    if [[ "${DOCKER[0]}" == "sudo" ]]; then
      COMPOSE=(sudo docker-compose)
    else
      COMPOSE=(docker-compose)
    fi
  else
    die "docker compose not found"
  fi
}

start_docker_daemon_linux() {
  if docker_bin info >/dev/null 2>&1 || prefer_sudo_docker_if_needed; then
    return 0
  fi
  log "Docker is installed but the daemon is not running"
  if have systemctl; then
    confirm "Start the Docker service now?" || die "start Docker (systemctl start docker) and re-run"
    sudo_cmd systemctl enable --now docker || sudo_cmd service docker start || true
  elif have service; then
    confirm "Start the Docker service now?" || die "start Docker and re-run"
    sudo_cmd service docker start || true
  else
    die "start the Docker daemon, then re-run ./start.sh"
  fi
  prefer_sudo_docker_if_needed || die "Docker daemon still unreachable"
}

install_docker_linux() {
  log "Installing Docker Engine (official convenience script)…"
  ensure_curl
  # get.docker.com installs docker + compose plugin on common distros.
  sudo_cmd sh -c 'curl -fsSL https://get.docker.com | sh'
  if have systemctl; then
    sudo_cmd systemctl enable --now docker || true
  fi
  if have usermod && [[ "$(id -u)" -ne 0 ]]; then
    sudo_cmd usermod -aG docker "$USER" || true
    warn "added $USER to the docker group — if 'docker' still fails, log out/in or run: newgrp docker"
  fi
}

install_docker_macos() {
  if have brew; then
    confirm "Install Docker Desktop with Homebrew? (needs a GUI login afterwards)" || die "install Docker Desktop from https://docs.docker.com/desktop/"
    brew install --cask docker
    warn "open Docker Desktop once from Applications, wait until it is running, then re-run ./start.sh"
    if [[ -d /Applications/Docker.app ]]; then
      open -a Docker || true
    fi
    die "finish Docker Desktop startup, then re-run ./start.sh"
  fi
  die "install Docker Desktop from https://docs.docker.com/desktop/ and re-run ./start.sh"
}

ensure_docker() {
  if have docker; then
    prefer_sudo_docker_if_needed || true
  fi
  if docker_ok && compose_ok; then
    log "✓ Docker + Compose ready"
    return 0
  fi

  if have docker && ! docker_bin info >/dev/null 2>&1; then
    case "$(os_family)" in
      linux) start_docker_daemon_linux ;;
      macos)
        if [[ -d /Applications/Docker.app ]]; then
          log "starting Docker Desktop…"
          open -a Docker || true
          for _ in $(seq 1 60); do
            docker info >/dev/null 2>&1 && break
            sleep 2
          done
        fi
        docker info >/dev/null 2>&1 || die "open Docker Desktop and wait until it is running, then re-run"
        DOCKER=(docker)
        ;;
      *) die "start Docker and re-run ./start.sh" ;;
    esac
  fi

  if ! have docker; then
    log "Docker is missing"
    case "$(os_family)" in
      linux)
        confirm "Install Docker Engine automatically?" || die "install Docker, then re-run ./start.sh"
        install_docker_linux
        start_docker_daemon_linux
        ;;
      macos) install_docker_macos ;;
      windows) die "install Docker Desktop for Windows, then run ./start.sh from Git Bash or WSL" ;;
      *) die "unsupported OS for auto-install; install Docker manually" ;;
    esac
  fi

  if ! compose_ok; then
    log "Docker Compose plugin is missing"
    case "$(os_family)" in
      linux)
        confirm "Install docker-compose-plugin?" || die "install Docker Compose plugin"
        if have apt-get; then
          sudo_cmd DEBIAN_FRONTEND=noninteractive apt-get install -y docker-compose-plugin || install_docker_linux
        else
          install_docker_linux
        fi
        ;;
      *) die "install Docker Compose (included with Docker Desktop)" ;;
    esac
  fi

  docker_ok || die "Docker is still not usable"
  compose_ok || die "Docker Compose is still not usable"
  log "✓ Docker + Compose ready"
}

port_in_use() {
  (exec 3<>"/dev/tcp/127.0.0.1/$1") 2>/dev/null
}

stack_running() {
  [[ -n "$("${COMPOSE[@]}" ps -q --status running loopforge 2>/dev/null)" ]]
}

# Our own running stack may hold the port; anything else means pick another.
choose_port() {
  local port="$LOOPFORGE_PORT"
  if stack_running; then
    local published
    published="$("${COMPOSE[@]}" port loopforge 8080 2>/dev/null | tail -1)"
    if [[ "${published##*:}" == "$port" ]]; then
      return 0
    fi
  fi
  port_in_use "$port" || return 0
  local candidate
  for candidate in $(seq $((port + 1)) $((port + 50))); do
    if ! port_in_use "$candidate"; then
      warn "port $port is already in use; using $candidate instead (saved to .env as LOOPFORGE_PORT)"
      set_env_value LOOPFORGE_PORT "$candidate"
      export LOOPFORGE_PORT="$candidate"
      return 0
    fi
  done
  die "ports $port-$((port + 50)) are all in use; set LOOPFORGE_PORT in .env"
}

# A release checkout (tag platform-vX.Y.Z) or an explicit LOOPFORGE_VERSION /
# LOOPFORGE_IMAGE runs the published image; any other checkout builds its own
# code so what runs matches what was cloned.
IMAGE_MODE=build
resolve_image() {
  local forced="${1:-}"
  if [[ "$forced" == "pull" ]]; then
    IMAGE_MODE=pull
    export LOOPFORGE_IMAGE="${LOOPFORGE_IMAGE:-$PUBLISHED_REPO:${LOOPFORGE_VERSION:-latest}}"
  elif [[ "$forced" == "build" ]]; then
    IMAGE_MODE=build
    export LOOPFORGE_IMAGE="$SOURCE_IMAGE"
  elif [[ -n "${LOOPFORGE_IMAGE:-}" || -n "${LOOPFORGE_VERSION:-}" ]]; then
    IMAGE_MODE=pull
    export LOOPFORGE_IMAGE="${LOOPFORGE_IMAGE:-$PUBLISHED_REPO:$LOOPFORGE_VERSION}"
  else
    local tag=""
    if have git && [[ -d .git || -f .git ]]; then
      tag="$(git describe --exact-match --tags --match 'platform-v*' 2>/dev/null || true)"
    fi
    if [[ -n "$tag" ]]; then
      IMAGE_MODE=release
      export LOOPFORGE_IMAGE="$PUBLISHED_REPO:${tag#platform-v}"
    else
      IMAGE_MODE=build
      export LOOPFORGE_IMAGE="$SOURCE_IMAGE"
    fi
  fi
}

reachable() {
  curl -fsS -o /dev/null --connect-timeout 4 --max-time 8 "$1" 2>/dev/null
}

# Networks that cannot reach the default Go / npm registries get mirrors for
# the image build; explicit LOOPFORGE_GOPROXY / LOOPFORGE_NPM_REGISTRY win.
detect_build_mirrors() {
  if [[ -z "${LOOPFORGE_GOPROXY:-}" ]] && ! reachable "https://proxy.golang.org/github.com/gin-gonic/gin/@v/list"; then
    export LOOPFORGE_GOPROXY="https://goproxy.cn,direct"
    warn "proxy.golang.org is unreachable; building with GOPROXY=$LOOPFORGE_GOPROXY"
  fi
  if [[ -z "${LOOPFORGE_NPM_REGISTRY:-}" ]] && ! reachable "https://registry.npmjs.org/@openai%2fcodex"; then
    export LOOPFORGE_NPM_REGISTRY="https://registry.npmmirror.com"
    warn "registry.npmjs.org is unreachable; building with npm registry $LOOPFORGE_NPM_REGISTRY"
  fi
}

check_disk() {
  local dir avail_kb
  dir="$(docker_bin info --format '{{.DockerRootDir}}' 2>/dev/null || true)"
  [[ -n "$dir" && -d "$dir" ]] || dir="/"
  avail_kb="$(df -Pk "$dir" 2>/dev/null | awk 'NR==2{print $4}')"
  [[ -n "$avail_kb" ]] || return 0
  if (( avail_kb < 6 * 1024 * 1024 )); then
    warn "only $((avail_kb / 1024 / 1024)) GB free under $dir; the LoopForge and sandbox images need about 6 GB"
  else
    log "✓ disk: $((avail_kb / 1024 / 1024)) GB free"
  fi
}

check_host() {
  log "Checking this machine ($(uname -s) $(uname -m))"
  ensure_curl
  ensure_git
  ensure_docker
  compose_cmd
  check_disk
}

start_stack() {
  local forced="${1:-}"
  resolve_image "$forced"
  choose_port
  case "$IMAGE_MODE" in
    pull|release)
      log "pulling $LOOPFORGE_IMAGE …"
      if docker_bin pull "$LOOPFORGE_IMAGE"; then
        "${COMPOSE[@]}" up -d --no-build
        return 0
      fi
      [[ "$IMAGE_MODE" == "release" ]] || die "could not pull $LOOPFORGE_IMAGE"
      warn "published image $LOOPFORGE_IMAGE is not available; building this checkout instead"
      export LOOPFORGE_IMAGE="$SOURCE_IMAGE"
      ;;
  esac
  detect_build_mirrors
  if docker_bin image inspect "$SOURCE_IMAGE" >/dev/null 2>&1; then
    log "building this checkout (cached layers make this quick after the first time)…"
  else
    log "building this checkout — the first build downloads Go/Node dependencies and takes a few minutes…"
  fi
  "${COMPOSE[@]}" up -d --build
}

wait_for_health() {
  local url="http://127.0.0.1:${LOOPFORGE_PORT}/api/health" i
  printf 'waiting for %s ' "$url"
  for i in $(seq 1 120); do
    if curl -fsS -o /dev/null "$url" 2>/dev/null; then
      echo "ok"
      return 0
    fi
    printf '.'
    sleep 1
  done
  echo
  "${COMPOSE[@]}" logs --tail 40 loopforge >&2 || true
  die "LoopForge did not become healthy in 120s (logs above; more with ./start.sh logs)"
}

print_ready() {
  local password_hint
  if [[ -n "${LOOPFORGE_ADMIN_PASSWORD:-}" ]]; then
    password_hint="admin / (LOOPFORGE_ADMIN_PASSWORD from .env)"
  elif [[ "${LOOPFORGE_DEMO_MODE}" == "true" ]]; then
    password_hint="admin / demo1234"
  else
    password_hint="admin / see: ./start.sh logs | grep 'Created admin account'"
  fi
  cat <<EOF

==============================================
 LoopForge is up
   UI      http://localhost:${LOOPFORGE_PORT}
   login   ${password_hint}
==============================================
 Try it:      工作总览 → 快速开始 → 「一键开跑」(no API key needed)
 Real work:   follow the credential wizard, then dispatch with SOP「快速修复」
 Self-test:   ./start.sh doctor
 Logs / stop: ./start.sh logs | ./start.sh down

EOF
  if [[ "${LOOPFORGE_PREWARM_SANDBOX:-true}" == "true" ]]; then
    log "The default sandbox image builds in the background now; the first real run will not wait for it."
  fi
  if [[ "$HAD_DATA" == "1" ]]; then
    log "Note: reusing the existing data volume, so accounts and passwords are the ones set before (./start.sh reset starts fresh)."
  fi
}

HAD_DATA=0
detect_existing_data() {
  local project
  project="$("${COMPOSE[@]}" config 2>/dev/null | awk '/^name:/{print $2; exit}')"
  [[ -n "$project" ]] || return 0
  if [[ -n "$(docker_bin volume ls -q --filter "label=com.docker.compose.project=$project" --filter "label=com.docker.compose.volume=loopforge-data" 2>/dev/null)" ]]; then
    HAD_DATA=1
  fi
}

run_self_test() {
  if ! stack_running; then
    log "LoopForge is not running; start it with ./start.sh to run the end-to-end self-test"
    return 0
  fi
  log
  log "End-to-end self-test (inside the LoopForge container):"
  "${COMPOSE[@]}" exec -T loopforge loopforge --self-test
}

cmd_up() {
  check_host
  detect_existing_data
  start_stack "${1:-}"
  wait_for_health
  print_ready
}

# --- args ---
cmd=up
while [[ $# -gt 0 ]]; do
  case "$1" in
    -y|--yes) YES=1; export LOOPFORGE_YES=1; shift ;;
    -h|--help) usage; exit 0 ;;
    up|doctor|warm|down|logs|pull|build|restart|reset) cmd=$1; shift ;;
    -d) shift ;;
    *) die "unknown argument: $1 (try --help)" ;;
  esac
done

ensure_env_file

case "$cmd" in
  up) cmd_up ;;
  pull) cmd_up pull ;;
  build) cmd_up build ;;
  doctor)
    check_host
    resolve_image
    log "  image:  $LOOPFORGE_IMAGE ($IMAGE_MODE)"
    log "  port:   $LOOPFORGE_PORT"
    run_self_test
    ;;
  warm)
    check_host
    stack_running || die "start LoopForge first: ./start.sh"
    "${COMPOSE[@]}" exec -T loopforge loopforge --prewarm-sandbox
    ;;
  logs)
    compose_cmd
    "${COMPOSE[@]}" logs -f loopforge
    ;;
  down)
    compose_cmd
    "${COMPOSE[@]}" down
    ;;
  restart)
    compose_cmd
    "${COMPOSE[@]}" down || true
    cmd_up
    ;;
  reset)
    compose_cmd
    confirm "Stop LoopForge and delete all its data (requirements, runs, accounts)?" || die "aborted"
    "${COMPOSE[@]}" down -v
    log "data removed; run ./start.sh to start fresh"
    ;;
esac
