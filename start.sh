#!/usr/bin/env bash
# One-command local start, Grasp-style:
# detect missing deps → install what we can → pull GHCR (or build) → wait healthy → print login.
#
# Usage:
#   ./start.sh              # ensure deps, start demo stack (detached)
#   ./start.sh -y           # same, auto-install without prompts
#   ./start.sh doctor       # only check / install deps
#   ./start.sh logs|down|pull|build|restart
set -euo pipefail
root="$(cd "$(dirname "$0")" && pwd)"
cd "$root"

YES="${LOOPFORGE_YES:-0}"
PORT="${LOOPFORGE_PORT:-8080}"
IMAGE="${LOOPFORGE_IMAGE:-ghcr.io/tencent/loopforge:${LOOPFORGE_VERSION:-latest}}"
export LOOPFORGE_DEMO_MODE="${LOOPFORGE_DEMO_MODE:-true}"
export LOOPFORGE_PORT="$PORT"

usage() {
  cat <<'EOF'
Usage: ./start.sh [-y|--yes] [up|doctor|down|logs|pull|build|restart]

  -y, --yes   Auto-install missing deps without prompting
  up          Detect deps, start LoopForge (default, detached)
  doctor      Only check / install Docker and friends
  down        Stop the stack
  logs        Follow container logs
  pull        Refresh the published image
  build       Build from this checkout and start
  restart     down + up

Demo mode is on by default (LOOPFORGE_DEMO_MODE=true). Override in .env.
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

ensure_env_file() {
  if [[ ! -f .env && -f .env.example ]]; then
    cp .env.example .env
    log "created .env from .env.example"
  fi
  if [[ -f .env ]]; then
    set -a
    # shellcheck disable=SC1091
    source .env
    set +a
  fi
  export LOOPFORGE_DEMO_MODE="${LOOPFORGE_DEMO_MODE:-true}"
  PORT="${LOOPFORGE_PORT:-8080}"
  export LOOPFORGE_PORT="$PORT"
  IMAGE="${LOOPFORGE_IMAGE:-ghcr.io/tencent/loopforge:${LOOPFORGE_VERSION:-latest}}"
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

doctor() {
  log "LoopForge doctor — checking what you need to run locally"
  log "  os: $(uname -s) $(uname -m)"
  ensure_curl
  ensure_git
  ensure_docker
  compose_cmd
  log "  compose: ${COMPOSE[*]} ($("${COMPOSE[@]}" version 2>/dev/null | head -1 || echo ok))"
  log "  image target: $IMAGE"
  log "  demo mode: $LOOPFORGE_DEMO_MODE"
  log "  port: $PORT"
  if docker_bin image inspect "$IMAGE" >/dev/null 2>&1; then
    log "  ✓ image present: $IMAGE"
  else
    log "  · image not local yet (will pull or build on up)"
  fi
  log "doctor ok"
}

wait_for_health() {
  local url="http://127.0.0.1:${PORT}/api/health"
  local i
  printf 'waiting for %s ' "$url"
  for i in $(seq 1 90); do
    if have curl && curl -fsS -o /dev/null "$url" 2>/dev/null; then
      echo "ok"
      return 0
    fi
    if have wget && wget -q -O /dev/null "$url" 2>/dev/null; then
      echo "ok"
      return 0
    fi
    printf '.'
    sleep 1
  done
  echo ""
  warn "API not healthy in 90s — check: ./start.sh logs"
  return 0
}

print_ready() {
  echo
  echo "=============================================="
  echo " LoopForge is up"
  echo "  UI      http://localhost:${PORT}"
  echo "  health  http://localhost:${PORT}/api/health"
  if [[ "${LOOPFORGE_DEMO_MODE}" == "true" && -z "${LOOPFORGE_ADMIN_PASSWORD:-}" ]]; then
    echo "  login   admin / demo1234   (demo mode)"
  else
    echo "  login   see: ./start.sh logs | grep 'Created admin account'"
  fi
  echo "=============================================="
  echo " Next: open the UI → 工作总览 → 「一键演示」"
  echo " Real work: finish the credential wizard, then use SOP「快速修复」"
  echo " Logs: ./start.sh logs    Stop: ./start.sh down"
  echo
}

pull_or_build_up() {
  compose_cmd
  local mode="${1:-pull}"
  if [[ "$mode" == "build" ]]; then
    log "building image from this checkout…"
    "${COMPOSE[@]}" up -d --build
    return 0
  fi

  log "pulling $IMAGE …"
  if docker_bin pull "$IMAGE" >/tmp/loopforge-pull.log 2>&1; then
    LOOPFORGE_VERSION="${LOOPFORGE_VERSION:-latest}" "${COMPOSE[@]}" up -d
    return 0
  fi

  warn "published image unavailable; building from this checkout (needs a few minutes)…"
  tail -5 /tmp/loopforge-pull.log 2>/dev/null || true
  "${COMPOSE[@]}" up -d --build
}

cmd_up() {
  ensure_env_file
  doctor
  pull_or_build_up pull
  wait_for_health
  print_ready
}

cmd_build() {
  ensure_env_file
  doctor
  pull_or_build_up build
  wait_for_health
  print_ready
}

# --- args ---
cmd=up
while [[ $# -gt 0 ]]; do
  case "$1" in
    -y|--yes) YES=1; export LOOPFORGE_YES=1; shift ;;
    -h|--help) usage; exit 0 ;;
    up|doctor|down|logs|pull|build|restart) cmd=$1; shift ;;
    -d) shift ;; # accepted for Grasp muscle-memory; always detached
    *) die "unknown argument: $1 (try --help)" ;;
  esac
done

ensure_env_file

case "$cmd" in
  doctor) doctor ;;
  pull)
    doctor
    compose_cmd
    log "pulling $IMAGE …"
    docker_bin pull "$IMAGE"
    "${COMPOSE[@]}" pull loopforge || true
    log "done"
    ;;
  down)
    compose_cmd
    "${COMPOSE[@]}" down
    ;;
  logs)
    compose_cmd
    "${COMPOSE[@]}" logs -f loopforge
    ;;
  build) cmd_build ;;
  restart)
    compose_cmd
    "${COMPOSE[@]}" down || true
    cmd_up
    ;;
  up) cmd_up ;;
esac
