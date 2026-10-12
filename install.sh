#!/usr/bin/env bash
# One-liner bootstrap (Grasp-style):
#   curl -fsSL https://raw.githubusercontent.com/Tencent/LoopForge/main/install.sh | bash
# Options:
#   curl … | bash -s -- -y              # non-interactive
#   LOOPFORGE_DIR=~/apps/lf bash install.sh
set -euo pipefail

YES="${LOOPFORGE_YES:-0}"
DIR="${LOOPFORGE_DIR:-$HOME/LoopForge}"
REPO="${LOOPFORGE_REPO:-https://github.com/Tencent/LoopForge.git}"
REF="${LOOPFORGE_REF:-main}"

while [[ $# -gt 0 ]]; do
  case "$1" in
    -y|--yes) YES=1; shift ;;
    --dir) DIR="$2"; shift 2 ;;
    --ref) REF="$2"; shift 2 ;;
    *) echo "unknown arg: $1" >&2; exit 1 ;;
  esac
done
export LOOPFORGE_YES="$YES"

need() { command -v "$1" >/dev/null 2>&1 || { echo "error: need $1 on PATH" >&2; exit 1; }; }

if ! command -v git >/dev/null 2>&1; then
  echo "git missing — install git, or clone LoopForge yourself and run ./start.sh -y" >&2
  exit 1
fi
need curl

if [[ -d "$DIR/.git" ]]; then
  echo "updating $DIR ($REF)…"
  git -C "$DIR" fetch --depth 1 origin "$REF"
  git -C "$DIR" checkout -q FETCH_HEAD || git -C "$DIR" checkout -q "$REF"
else
  echo "cloning $REPO → $DIR ($REF)…"
  mkdir -p "$(dirname "$DIR")"
  git clone --depth 1 --branch "$REF" "$REPO" "$DIR" 2>/dev/null \
    || git clone --depth 1 "$REPO" "$DIR"
fi

cd "$DIR"
chmod +x start.sh install.sh 2>/dev/null || true
exec ./start.sh -y
