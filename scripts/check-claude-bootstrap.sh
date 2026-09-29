#!/usr/bin/env bash
# Closed check for the approved maintainer-runtime bootstrap (see AGENTS.md,
# "Maintainer runtimes"). The only allowed Claude artifact is a regular root
# CLAUDE.md whose entire content is "@AGENTS.md" with an optional final newline.
set -euo pipefail

TARGET="${1:-.}"

fail() {
  printf 'FAIL: %s\n' "$1" >&2
  exit 1
}

[[ -d "$TARGET" ]] || fail "target directory does not exist: $TARGET"
ROOT="$(cd "$TARGET" && pwd -P)"
BOOTSTRAP="$ROOT/CLAUDE.md"

# Any Claude instruction or configuration entry other than the root bootstrap,
# of any type and in any letter case, is unapproved.
while IFS= read -r -d '' entry; do
  [[ "$entry" == "$BOOTSTRAP" ]] || fail "unapproved Claude artifact: ${entry#"$ROOT/"}"
done < <(find "$ROOT" -path "$ROOT/.git" -prune -o \
  \( -iname 'CLAUDE.md' -o -iname 'CLAUDE.local.md' -o -iname '.claude' \) -print0)

if [[ -e "$BOOTSTRAP" || -L "$BOOTSTRAP" ]]; then
  [[ -f "$BOOTSTRAP" && ! -L "$BOOTSTRAP" ]] || fail "CLAUDE.md must be a regular file"
  cmp -s "$BOOTSTRAP" <(printf '@AGENTS.md\n') || cmp -s "$BOOTSTRAP" <(printf '@AGENTS.md') \
    || fail "CLAUDE.md must contain exactly @AGENTS.md"
  [[ -f "$ROOT/AGENTS.md" && ! -L "$ROOT/AGENTS.md" ]] || fail "CLAUDE.md requires a regular AGENTS.md"
fi
