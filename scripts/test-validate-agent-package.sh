#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"
VALIDATOR="$ROOT/scripts/validate-agent-package.sh"

fail() {
  printf 'FAIL: %s\n' "$1" >&2
  exit 1
}

new_package() {
  local package="$1"
  mkdir -p "$package/.agents/skills/example"
  printf '# Example Agent\n' > "$package/AGENTS.md"
  printf '%s\n' '---' 'name: example' 'description: Test skill.' '---' '# Example' \
    > "$package/.agents/skills/example/SKILL.md"
}

[[ -x "$VALIDATOR" ]] || fail "agent-package validator is missing or not executable"

TEST_ROOT="$(mktemp -d "${TMPDIR:-/tmp}/axiom-agent-package.XXXXXX")"
trap 'rm -rf "$TEST_ROOT"' EXIT

SAFE_PACKAGE="$TEST_ROOT/safe"
new_package "$SAFE_PACKAGE"
printf 'EXAMPLE_VALUE=replace-me\n' > "$SAFE_PACKAGE/.env.example"
"$VALIDATOR" "$SAFE_PACKAGE" >/dev/null 2>&1 \
  || fail "regular package should pass validation"

POINTER_PACKAGE="$TEST_ROOT/pointer"
new_package "$POINTER_PACKAGE"
printf '@AGENTS.md\n' > "$POINTER_PACKAGE/CLAUDE.md"
"$VALIDATOR" "$POINTER_PACKAGE" >/dev/null 2>&1 \
  || fail "exact CLAUDE.md pointer to AGENTS.md should pass validation"

CLAUDE_PACKAGE="$TEST_ROOT/claude"
new_package "$CLAUDE_PACKAGE"
printf '@AGENTS.md\nAlways use Claude-specific instructions.\n' > "$CLAUDE_PACKAGE/CLAUDE.md"
if "$VALIDATOR" "$CLAUDE_PACKAGE" >/dev/null 2>&1; then
  fail "CLAUDE.md with its own instructions should fail validation"
fi

TARGET_PACKAGE="$TEST_ROOT/claude-target"
new_package "$TARGET_PACKAGE"
printf '@README.md\n' > "$TARGET_PACKAGE/CLAUDE.md"
if "$VALIDATOR" "$TARGET_PACKAGE" >/dev/null 2>&1; then
  fail "CLAUDE.md pointing to another file should fail validation"
fi

NESTED_PACKAGE="$TEST_ROOT/nested-claude"
new_package "$NESTED_PACKAGE"
printf '@AGENTS.md\n' > "$NESTED_PACKAGE/.agents/skills/example/CLAUDE.md"
if "$VALIDATOR" "$NESTED_PACKAGE" >/dev/null 2>&1; then
  fail "nested CLAUDE.md should fail validation"
fi

DOT_CLAUDE_PACKAGE="$TEST_ROOT/dot-claude"
new_package "$DOT_CLAUDE_PACKAGE"
mkdir "$DOT_CLAUDE_PACKAGE/.claude"
if "$VALIDATOR" "$DOT_CLAUDE_PACKAGE" >/dev/null 2>&1; then
  fail ".claude directory should fail validation"
fi

LINKED_PACKAGE="$TEST_ROOT/linked"
new_package "$LINKED_PACKAGE"
printf '# Outside content\n' > "$TEST_ROOT/outside.md"
ln -sf "$TEST_ROOT/outside.md" "$LINKED_PACKAGE/AGENTS.md"

if "$VALIDATOR" "$LINKED_PACKAGE" >/dev/null 2>&1; then
  fail "package containing a symlink should fail validation"
fi

ENV_PACKAGE="$TEST_ROOT/env"
new_package "$ENV_PACKAGE"
printf 'REAL_VALUE=do-not-package\n' > "$ENV_PACKAGE/.env.production"

if "$VALIDATOR" "$ENV_PACKAGE" >/dev/null 2>&1; then
  fail "package containing .env.* should fail validation"
fi

KEY_FILE_PACKAGE="$TEST_ROOT/key-file"
new_package "$KEY_FILE_PACKAGE"
printf 'test-only\n' > "$KEY_FILE_PACKAGE/signing.key"

if "$VALIDATOR" "$KEY_FILE_PACKAGE" >/dev/null 2>&1; then
  fail "package containing a private-key filename should fail validation"
fi

KEY_CONTENT_PACKAGE="$TEST_ROOT/key-content"
new_package "$KEY_CONTENT_PACKAGE"
printf '%s%s\n%s\n' '-----BEGIN ' 'PRIVATE KEY-----' 'test-only' \
  > "$KEY_CONTENT_PACKAGE/notes.txt"

if "$VALIDATOR" "$KEY_CONTENT_PACKAGE" >/dev/null 2>&1; then
  fail "package containing private-key content should fail validation"
fi

printf 'PASS: agent-package validator behavior is valid\n'
