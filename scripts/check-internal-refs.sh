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

# Exact lines that may mention a matched name on purpose (public adoption note).
allowed=(
  'README.md:LoopForge grew out of the R&D SOPs of Tencent teams and is used by multiple project teams inside Tencent, including SkillHub and teams with demanding quality requirements.'
  'README.zh-CN.md:LoopForge 源自腾讯团队的研发 SOP，腾讯内部已有多个项目团队在使用，包括 SkillHub，以及一些对质量要求很高的项目团队。'
)

# Vendored bundles and binary-like assets can match by accident. cli/ is
# checked by its own scripts/validate.sh.
matches=$(git grep --untracked -nIiE "${args[@]}" -- . \
  ':!scripts/check-internal-refs.sh' \
  ':(exclude)cli' \
  ':!web/monaco/**' \
  ':!web/vendor/**' \
  ':!**/*.svg' \
  ':!e2e/package-lock.json' || true)
filtered=""
while IFS= read -r m; do
  [[ -z "$m" ]] && continue
  key="${m%%:*}:${m#*:*:}"
  for a in "${allowed[@]}"; do [[ "$key" == "$a" ]] && continue 2; done
  filtered+="$m"$'\n'
done <<<"$matches"
matches="${filtered%$'\n'}"
if [[ -n "$matches" ]]; then
  echo "$matches"
  echo >&2
  echo "Internal references found; remove them or see CONTRIBUTING.md." >&2
  exit 1
fi
echo "No internal references found."
