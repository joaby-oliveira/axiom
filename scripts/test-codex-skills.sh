#!/usr/bin/env bash
set -euo pipefail

repository_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)
validator=${CODEX_SKILL_VALIDATOR:-"${HOME}/.codex/skills/.system/skill-creator/scripts/quick_validate.py"}
python=${CODEX_SKILL_PYTHON:-python3}
if [[ -f "$validator" ]] && "$python" -c 'import yaml' >/dev/null 2>&1; then
  for directory in "$repository_root"/internal/codexruntime/skills/*; do
    "$python" "$validator" "$directory"
  done
else
  printf '%s\n' 'WARN: bundled Codex validator dependency unavailable; Go contract validation remains active' >&2
fi

go test ./internal/codexruntime ./internal/runtimebootstrap ./cmd/lingo ./internal/cli
printf '%s\n' 'PASS: Codex runtime skills and delegation surface are valid'
