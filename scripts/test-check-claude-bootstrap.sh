#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"
CHECK="$ROOT/scripts/check-claude-bootstrap.sh"

fail() {
  printf 'FAIL: %s\n' "$1" >&2
  exit 1
}

[[ -x "$CHECK" ]] || fail "Claude bootstrap check is missing or not executable"

TEST_ROOT="$(mktemp -d "${TMPDIR:-/tmp}/axiom-claude-bootstrap.XXXXXX")"
trap 'rm -rf "$TEST_ROOT"' EXIT
count=0

new_root() {
  count=$((count + 1))
  root="$TEST_ROOT/case-$count"
  mkdir -p "$root/docs"
  printf '# Agent policy\n' > "$root/AGENTS.md"
}

expect_pass() {
  "$CHECK" "$2" >/dev/null 2>&1 || fail "$1 should pass"
}

expect_fail() {
  if "$CHECK" "$2" >/dev/null 2>&1; then
    fail "$1 should fail"
  fi
}

new_root
expect_pass "absent CLAUDE.md" "$root"

new_root
printf '@AGENTS.md\n' > "$root/CLAUDE.md"
expect_pass "root exact pointer" "$root"

new_root
printf '@AGENTS.md' > "$root/CLAUDE.md"
expect_pass "root exact pointer without final newline" "$root"

new_root
printf '@AGENTS.md\nAlways prefer Claude-only rules.\n' > "$root/CLAUDE.md"
expect_fail "root pointer plus own instructions" "$root"

new_root
printf '@AGENTS.md\n\n' > "$root/CLAUDE.md"
expect_fail "root pointer plus extra blank line" "$root"

new_root
printf '@AGENTS.md\r\n' > "$root/CLAUDE.md"
expect_fail "root pointer with CRLF" "$root"

new_root
printf '@README.md\n' > "$root/CLAUDE.md"
expect_fail "pointer to a different target" "$root"

new_root
printf '@docs/AGENTS.md\n' > "$root/CLAUDE.md"
expect_fail "pointer to a nested target" "$root"

new_root
printf 'Follow AGENTS.md.\n' > "$root/CLAUDE.md"
expect_fail "prose instead of the pointer" "$root"

new_root
printf '@AGENTS.md\n' > "$root/CLAUDE.md"
printf '@AGENTS.md\n' > "$root/docs/CLAUDE.md"
expect_fail "nested CLAUDE.md" "$root"

new_root
printf '@AGENTS.md\n' > "$root/claude.md"
expect_fail "differently cased root claude.md" "$root"

new_root
printf '@AGENTS.md\n' > "$root/CLAUDE.local.md"
expect_fail "CLAUDE.local.md" "$root"

new_root
printf '@AGENTS.md\n' > "$root/CLAUDE.md"
mkdir "$root/.claude"
expect_fail ".claude directory" "$root"

new_root
printf '{}\n' > "$root/.claude"
expect_fail ".claude file" "$root"

new_root
mkdir -p "$root/docs/.claude"
expect_fail "nested .claude directory" "$root"

new_root
ln -s AGENTS.md "$root/CLAUDE.md"
expect_fail "CLAUDE.md symlink to AGENTS.md" "$root"

new_root
printf '@AGENTS.md\n' > "$TEST_ROOT/outside-claude.md"
ln -s "$TEST_ROOT/outside-claude.md" "$root/CLAUDE.md"
expect_fail "CLAUDE.md symlink to an exact pointer outside the root" "$root"

new_root
mkdir "$root/CLAUDE.md"
expect_fail "CLAUDE.md directory" "$root"

new_root
printf '@AGENTS.md\n' > "$root/CLAUDE.md"
rm "$root/AGENTS.md"
expect_fail "pointer without AGENTS.md" "$root"

new_root
printf '@AGENTS.md\n' > "$root/CLAUDE.md"
mv "$root/AGENTS.md" "$root/policy.md"
ln -s policy.md "$root/AGENTS.md"
expect_fail "pointer to a symlinked AGENTS.md" "$root"

# Both repository validators must delegate to this closed check.
grep -Fq '"$ROOT/scripts/check-claude-bootstrap.sh" "$ROOT"' "$ROOT/scripts/validate-repository.sh" \
  || fail "validate-repository.sh does not run the Claude bootstrap check"
grep -Fq '"$SCRIPT_DIR/check-claude-bootstrap.sh" "$TARGET"' "$ROOT/scripts/validate-agent-package.sh" \
  || fail "validate-agent-package.sh does not run the Claude bootstrap check"

printf 'PASS: Claude bootstrap check behavior is valid (%s cases)\n' "$count"
