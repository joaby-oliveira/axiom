#!/usr/bin/env bash
# Regenerates the historical POC compatibility fixture by building and
# running the merged v0.1.0-poc.1 source with deterministic fake git/gh
# executables. Nothing touches real user state or any Provider.
set -euo pipefail
umask 077

repository_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)
tag=v0.1.0-poc.1
expected_revision=242d67c4cf2d4c3efe534dd894cb56a05558e139
fixture="$repository_root/internal/compatibility/testdata/poc-$tag"
neutral_base=/axiom-poc-fixture

[[ $(git -C "$repository_root" rev-parse "$tag^{commit}") == "$expected_revision" ]]
# The POC refuses shared sticky ancestors such as /tmp, so use a user-owned
# temporary root and normalize its machine-specific prefix afterwards.
base=$(mktemp -d)
base=$(cd "$base" && pwd -P)
trap 'rm -rf -- "$base"' EXIT

mkdir "$base/source" "$base/bin" "$base/project-repository" "$base/unrelated"
git -C "$repository_root" archive "$tag" | tar -x -C "$base/source"
(cd "$base/source" && go build -trimpath -o "$base/bin/lingo" ./cmd/lingo)

printf '%s\n' '#!/bin/sh' "printf '%s\\n' 'git@github.com:owner/repo.git'" >"$base/bin/git"
printf '%s\n' '#!/bin/sh' \
  'if [ "$1" = issue ] && [ "$2" = create ]; then printf "%s\n" "https://github.com/owner/repo/issues/7"; exit 0; fi' \
  'if [ "$1" = issue ] && [ "$2" = view ]; then printf "{\"Number\":7,\"URL\":\"https://github.com/owner/repo/issues/7\",\"State\":\"OPEN\"}\n"; exit 0; fi' \
  'if [ "$1" = issue ] && [ "$2" = comment ]; then printf "%s\n" ok; exit 0; fi' \
  'exit 1' >"$base/bin/gh"
chmod 700 "$base/bin/git" "$base/bin/gh"

export LINGO_PROJECTS_ROOT="$base/projects"
export LINGO_STATE_ROOT="$base/state"
export AXIOM_CODEX_SKILLS_ROOT="$base/skills"
export AXIOM_GIT_BIN="$base/bin/git"
export AXIOM_GH_BIN="$base/bin/gh"
lingo="$base/bin/lingo"
cd "$base/unrelated"
"$lingo" --json runtime codex install >/dev/null
"$lingo" --json project configure --slug poc-fixture --name "POC Fixture" --repository "main=$base/project-repository" >/dev/null
"$lingo" --json work-item create --project poc-fixture --repository main --title "POC fixture" --body "Synthetic historical fixture" --authorize-external >/dev/null
"$lingo" --json workflow start --project poc-fixture --repository main --number 7 >/dev/null
for gate in specification clarification; do
  printf '%s\n' "$gate" >"$base/project-repository/$gate.md"
  "$lingo" --json workflow advance --project poc-fixture --repository main --number 7 --gate "$gate" --outcome pass --reference "$gate.md" >/dev/null
done

rm -rf -- "$fixture"
mkdir -p "$fixture"
for tree in projects state skills; do
  cp -R "$base/$tree" "$fixture/$tree"
done
while IFS= read -r file; do
  LC_ALL=C sed -i.bak "s|$base|$neutral_base|g" "$file"
  rm -f -- "$file.bak"
done < <(grep -rlF "$base" "$fixture" || true)
{
  printf 'tag=%s\n' "$tag"
  printf 'revision=%s\n' "$expected_revision"
  printf 'generator=scripts/generate-poc-fixture.sh\n'
  printf 'go=%s\n' "$(go env GOVERSION)"
  printf 'normalized_prefix=%s\n' "$neutral_base"
} >"$fixture/PROVENANCE"
find "$fixture" -type f | LC_ALL=C sort | sed "s|^$fixture/||"
