#!/usr/bin/env bash
set -euo pipefail

repository_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)
temporary=$(mktemp -d)
if [[ -z "$temporary" || ! -d "$temporary" ]]; then
  exit 1
fi
dirty_probe="$repository_root/.axiom-install-dirty-test-$$"
trap 'rm -rf -- "$temporary"; rm -f -- "$dirty_probe"' EXIT

export AXIOM_BIN_DIR="$temporary/bin"
export AXIOM_INSTALL_STATE_ROOT="$temporary/state"

case ":$PATH:" in
  *":$AXIOM_BIN_DIR:"*) exit 1 ;;
esac
# A legacy or foreign lingo executable beside the destination is preserved.
mkdir -p "$AXIOM_BIN_DIR"
chmod 700 "$AXIOM_BIN_DIR"
printf '#!/bin/sh\necho legacy\n' >"$AXIOM_BIN_DIR/lingo"
legacy_before=$(shasum -a 256 "$AXIOM_BIN_DIR/lingo" | awk '{print $1}')
first_stderr="$temporary/first.stderr"
first=$($repository_root/scripts/install-axiom.sh 2>"$first_stderr")
case "$first" in
  *'"result":"installed"'*'"pathConfigured":false'*) ;;
  *) exit 1 ;;
esac
if ! grep -Fq 'export PATH=' "$first_stderr" || ! grep -Fq "$AXIOM_BIN_DIR" "$first_stderr"; then
  exit 1
fi

if ! grep -Fq 'axiom is not on PATH' "$first_stderr" || grep -Fq 'lingo' "$first_stderr"; then
  exit 1
fi
if [[ ! -f "$AXIOM_BIN_DIR/axiom" || -L "$AXIOM_BIN_DIR/axiom" || ! -x "$AXIOM_BIN_DIR/axiom" ]]; then
  exit 1
fi
if [[ $(shasum -a 256 "$AXIOM_BIN_DIR/lingo" | awk '{print $1}') != "$legacy_before" ]]; then
  exit 1
fi
# The public contract is a real executable resolved from PATH by a fresh
# non-interactive shell with an empty environment: no alias or function.
if [[ $(cd / && env -i HOME="$temporary" PATH="$AXIOM_BIN_DIR:/usr/bin:/bin" bash --noprofile --norc -c 'type -t axiom') != file ]]; then
  exit 1
fi
version=$(cd / && env -i HOME="$temporary" PATH="$AXIOM_BIN_DIR:/usr/bin:/bin" sh -c 'axiom version')
if [[ $(PATH="$AXIOM_BIN_DIR:$PATH" command -v axiom) != "$AXIOM_BIN_DIR/axiom" ]]; then
  exit 1
fi
case "$version" in
  *'status: success'*'result: Axiom build information'*'provenance: Axiom development'*'source=clean'*) ;;
  *) exit 1 ;;
esac

second_stderr="$temporary/second.stderr"
second=$(PATH="$AXIOM_BIN_DIR:$PATH" "$repository_root/scripts/install-axiom.sh" 2>"$second_stderr")
case "$second" in
  *'"result":"unchanged"'*'"pathConfigured":true'*) ;;
  *) exit 1 ;;
esac
if [[ -s "$second_stderr" ]]; then
  exit 1
fi

printf 'tampered\n' >>"$AXIOM_BIN_DIR/axiom"
before=$(shasum -a 256 "$AXIOM_BIN_DIR/axiom" | awk '{print $1}')
if "$repository_root/scripts/install-axiom.sh" >/dev/null 2>&1; then
  exit 1
fi
after=$(shasum -a 256 "$AXIOM_BIN_DIR/axiom" | awk '{print $1}')
if [[ "$before" != "$after" ]]; then
  exit 1
fi

export AXIOM_BIN_DIR="$temporary/dirty-bin"
export AXIOM_INSTALL_STATE_ROOT="$temporary/dirty-state"
: >"$dirty_probe"
dirty=$($repository_root/scripts/install-axiom.sh 2>"$temporary/dirty.stderr")
case "$dirty" in
  *'"result":"installed"'*'"dirty":true'*'"pathConfigured":false'*) ;;
  *) exit 1 ;;
esac
dirty_version=$(PATH="$AXIOM_BIN_DIR:$PATH" axiom --json version)
case "$dirty_version" in
  *'"version":"development"'*'"sourceState":"dirty"'*) ;;
  *) exit 1 ;;
esac
if ! grep -Fq 'dirty=true' "$AXIOM_INSTALL_STATE_ROOT/axiom.receipt"; then
  exit 1
fi

printf '%s\n' 'PASS: Axiom installer reports PATH setup, dirty metadata, idempotence, and conflicts'
