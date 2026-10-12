#!/usr/bin/env bash
# Fails when the tree references private hosts, systems or addresses that do
# not belong in a public repository. Add your own patterns (one extended regex
# per line) via LOOPFORGE_EXTRA_REF_PATTERNS=/path/to/file.
set -euo pipefail
cd "$(dirname "$0")/.."

patterns=(
  '[a-z0-9.-]+\.woa\.com'
  '[a-z0-9.-]+\.oa\.com'
  'code\.oa'
  'gongfeng|工蜂'
  '\btapd\b'
  'taihu|太湖'
  '\biwiki\b'
  'zhiyan|智研'
  'skillhub'
  'lightcloud'
  'dns-ai'
  '\bgslb'
  '\bqta\b'
  '\brtx\b'
  '企业微信|\bwecom\b|\bwework\b'
  'mirrors\.tencent'
  'csighub|tlinux'
  '蓝盾|bkdevops'
  '\b9\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}\b'
  'internal edition|open[- ]source edition|内部版|开源版'
)
if [[ -n "${LOOPFORGE_EXTRA_REF_PATTERNS:-}" ]]; then
  while IFS= read -r line; do
    [[ -z "$line" || "$line" == \#* ]] || patterns+=("$line")
  done < "$LOOPFORGE_EXTRA_REF_PATTERNS"
fi

args=()
for p in "${patterns[@]}"; do args+=(-e "$p"); done

# Vendored bundles and binary-like assets can match by accident. cli/ is
# checked by its own scripts/validate.sh.
if git grep --untracked -nIiE "${args[@]}" -- . \
  ':!scripts/check-internal-refs.sh' \
  ':(exclude)cli' \
  ':!web/monaco/**' \
  ':!**/*.svg' \
  ':!e2e/package-lock.json'; then
  echo >&2
  echo "Internal references found; remove them or see CONTRIBUTING.md." >&2
  exit 1
fi
echo "No internal references found."
