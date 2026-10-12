#!/usr/bin/env bash
# Starts a throwaway LoopForge instance for the browser tests: fresh SQLite
# database, demo executor enabled, host-process sandbox, known admin password,
# and the example extensions from examples/extensions.
set -euo pipefail
root="$(cd "$(dirname "$0")/.." && pwd)"
data="$(mktemp -d "${TMPDIR:-/tmp}/loopforge-e2e.XXXXXX")"
trap 'rm -rf "$data"' EXIT
export LOOPFORGE_DATA_DIR="$data"
export LOOPFORGE_HTTP_PORT="${E2E_PORT:-18499}"
export LOOPFORGE_ADMIN_PASSWORD="${E2E_ADMIN_PASSWORD:-e2e-admin-pass}"
export LOOPFORGE_DEMO_MODE=true
export LOOPFORGE_TEMPLATE_SANDBOX=local
export LOOPFORGE_MCP_MARKET="$root/examples/extensions/mcp-market.json"
export LOOPFORGE_TEMPLATES="$root/examples/extensions/templates"
export LOOPFORGE_WEB_PLUGINS="$root/examples/extensions/plugins"
cd "$data"
# Stopping the script must stop the server too, otherwise an orphaned
# instance keeps the port and later runs test against its stale database.
"$root/bin/loopforge" &
server=$!
trap 'kill "$server" 2>/dev/null; wait "$server" 2>/dev/null; rm -rf "$data"' EXIT INT TERM
wait "$server"
