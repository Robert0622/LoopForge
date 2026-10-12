#!/usr/bin/env bash
# One-command local start, same spirit as Grasp's start.sh:
# prefer the published image, fall back to building from this checkout,
# and always enable demo mode so the first run works without API keys.
set -euo pipefail
root="$(cd "$(dirname "$0")" && pwd)"
cd "$root"

usage() {
  cat <<'EOF'
Usage: ./start.sh [-d] [up|down|logs|pull|build]

  -d        Detached (default for up)
  up        Start LoopForge with docker compose (default)
  down      Stop the stack
  logs      Follow container logs
  pull      Refresh the published image
  build     Build the image from this checkout and start

Demo mode is on by default (LOOPFORGE_DEMO_MODE=true). Override in .env.
EOF
}

detached=1
cmd=up
while [[ $# -gt 0 ]]; do
  case "$1" in
    -d) detached=1; shift ;;
    -h|--help) usage; exit 0 ;;
    up|down|logs|pull|build) cmd=$1; shift ;;
    *) echo "unknown argument: $1" >&2; usage; exit 1 ;;
  esac
done

export LOOPFORGE_DEMO_MODE="${LOOPFORGE_DEMO_MODE:-true}"
compose=(docker compose)
[[ -f .env ]] || { [[ -f .env.example ]] && cp .env.example .env || true; }

case "$cmd" in
  pull)
    "${compose[@]}" pull loopforge
    ;;
  down)
    "${compose[@]}" down
    ;;
  logs)
    "${compose[@]}" logs -f loopforge
    ;;
  build)
    "${compose[@]}" up -d --build
    echo
    echo "LoopForge is up. Open http://localhost:${LOOPFORGE_PORT:-8080}"
    if [[ "${LOOPFORGE_DEMO_MODE}" == "true" && -z "${LOOPFORGE_ADMIN_PASSWORD:-}" ]]; then
      echo "Demo login: admin / demo1234"
    else
      echo "Admin password (first start only):"
      "${compose[@]}" logs loopforge 2>/dev/null | grep -E 'Created admin account|admin password' | tail -3 || true
    fi
    ;;
  up)
    if ! "${compose[@]}" pull loopforge >/tmp/loopforge-pull.log 2>&1; then
      echo "Published image unavailable; building from this checkout…"
      "${compose[@]}" up -d --build
    else
      if [[ "$detached" -eq 1 ]]; then
        "${compose[@]}" up -d
      else
        "${compose[@]}" up
      fi
    fi
    echo
    echo "LoopForge is up. Open http://localhost:${LOOPFORGE_PORT:-8080}"
    if [[ "${LOOPFORGE_DEMO_MODE}" == "true" && -z "${LOOPFORGE_ADMIN_PASSWORD:-}" ]]; then
      echo "Demo login: admin / demo1234"
    else
      echo "Admin password (first start only):"
      "${compose[@]}" logs loopforge 2>/dev/null | grep -E 'Created admin account|admin password' | tail -3 || true
    fi
    echo
    echo "Tip: use the chat box on 工作总览 to describe a requirement in one sentence."
    ;;
esac
