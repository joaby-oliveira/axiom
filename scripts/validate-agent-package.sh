#!/usr/bin/env bash
set -euo pipefail

TARGET="${1:-.}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"

fail() {
  printf 'FAIL: %s\n' "$1" >&2
  exit 1
}

warn() {
  printf 'WARN: %s\n' "$1" >&2
}

pass() {
  printf 'PASS: %s\n' "$1"
}

[[ -d "$TARGET" ]] || fail "target directory does not exist: $TARGET"
[[ -f "$TARGET/AGENTS.md" ]] || fail "AGENTS.md is required"

if find "$TARGET" -path "$TARGET/.git" -prune -o -type l -print -quit | grep -q .; then
  fail "symlinks are not allowed in Codex packages"
fi

# Only the approved root bootstrap CLAUDE.md ("@AGENTS.md") is tolerated;
# the Codex renderer never emits it. Any other Claude artifact fails.
"$SCRIPT_DIR/check-claude-bootstrap.sh" "$TARGET" || fail "Claude artifacts found in Codex package"

"$SCRIPT_DIR/check-sensitive-files.sh" --directory "$TARGET"

if find "$TARGET" -path "$TARGET/.git" -prune -o -type d -empty -print -quit | grep -q .; then
  warn "empty directories exist"
fi

if grep -RInE 'TODO: fill|PLACEHOLDER|YOUR_[A-Z_]+_HERE|changeme' \
  "$TARGET/AGENTS.md" "$TARGET/.agents" 2>/dev/null | grep -q .; then
  fail "vague placeholder detected"
fi

if [[ -d "$TARGET/.agents/skills" ]]; then
  while IFS= read -r skilldir; do
    [[ -f "$skilldir/SKILL.md" ]] || fail "skill missing SKILL.md: $skilldir"
  done < <(find "$TARGET/.agents/skills" -mindepth 1 -maxdepth 1 -type d)
fi

pass "Codex agent package structure looks valid"
