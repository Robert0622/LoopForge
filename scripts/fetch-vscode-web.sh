#!/bin/sh
# Fetch the vscode-web dist (Code-OSS web build) into web/vscode-web.
set -eu
VERSION="${1:-1.91.1}"
ROOT="$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
cd "$TMP"
npm pack "vscode-web@${VERSION}"
tar -xzf "vscode-web-${VERSION}.tgz"
DEST="$ROOT/web/vscode-web"
find "$DEST" -mindepth 1 ! -name README.md -exec rm -rf {} + 2>/dev/null || true
mkdir -p "$DEST"
rsync -a --exclude '*.map' package/dist/ "$DEST/"
printf '%s\n' "$VERSION" > "$DEST/VERSION"
echo "fetched vscode-web ${VERSION} -> web/vscode-web"
