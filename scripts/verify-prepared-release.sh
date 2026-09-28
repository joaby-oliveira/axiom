#!/usr/bin/env bash
# Verifies one prepared release set (the workflow artifact of
# release-artifacts.yml) before it may be previewed or published. Read-only.
# Run from a clean checkout of the exact revision. It requires:
# - with --run: that run is a successful workflow_dispatch of
#   .github/workflows/release-artifacts.yml on main of --repo;
# - the tag and revision to pass release-preflight.sh now;
# - verify-release-artifacts.sh to accept the prepared artifacts here and to
#   report the prepared release-evidence.txt (host-only version_smoke fields
#   ignored);
# - release-notes.md to equal the notes regenerated from the revision.
# Prints the preflight facts and prepared=verified.
set -euo pipefail

umask 077

repository_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)
tag=
revision=
prepared=
repository=
run=
main_ref=origin/main
remote=origin
while (($#)); do
  case "$1" in
    --tag) tag=${2:-}; shift 2 ;;
    --revision) revision=${2:-}; shift 2 ;;
    --prepared) prepared=${2:-}; shift 2 ;;
    --repo) repository=${2:-}; shift 2 ;;
    --run) run=${2:-}; shift 2 ;;
    --main-ref) main_ref=${2:-}; shift 2 ;;
    --remote) remote=${2:-}; shift 2 ;;
    *) printf 'prepared_release_error: invalid argument\n' >&2; exit 1 ;;
  esac
done

fail() {
  printf 'prepared_release_error: %s\n' "$1" >&2
  exit 1
}

[[ "$prepared" == /* && -d "$prepared" && ! -L "$prepared" ]] || fail 'absolute prepared directory required'
for file in release-evidence.txt release-notes.md; do
  [[ -f "$prepared/$file" && ! -L "$prepared/$file" ]] || fail "prepared set lacks $file"
done
[[ -d "$prepared/artifacts" && ! -L "$prepared/artifacts" ]] || fail 'prepared set lacks artifacts/'

[[ "$repository" =~ ^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$ ]] || fail 'repository must be OWNER/NAME'
if [[ -n "$run" ]]; then
  [[ "$run" =~ ^[0-9]+$ ]] || fail 'numeric run id required'
  metadata=$(gh api "repos/$repository/actions/runs/$run") || fail "cannot read workflow run $run"
  jq -e '.path == ".github/workflows/release-artifacts.yml" and .event == "workflow_dispatch"
    and .head_branch == "main" and .status == "completed" and .conclusion == "success"' <<<"$metadata" >/dev/null \
    || fail "run $run is not a successful release-artifacts.yml dispatch from main"
fi

temporary=$(mktemp -d)
trap 'rm -rf -- "$temporary"' EXIT

"$repository_root/scripts/release-preflight.sh" --tag "$tag" --revision "$revision" --main-ref "$main_ref" --remote "$remote" \
  >"$temporary/preflight" || exit 1
version=$(awk -F= '$1 == "version" {print $2}' "$temporary/preflight")

"$repository_root/scripts/verify-release-artifacts.sh" --dir "$prepared/artifacts" --version "$version" --revision "$revision" \
  >"$temporary/evidence" || fail 'prepared artifacts fail verification at this revision'
normalize() { sed -E 's/ version_smoke=[a-z_]+$//' "$1"; }
cmp -s <(normalize "$prepared/release-evidence.txt") <(normalize "$temporary/evidence") \
  || fail 'prepared release-evidence.txt differs from re-verification'

"$repository_root/scripts/release-notes.sh" --tag "$tag" --revision "$revision" --repo "$repository" \
  >"$temporary/notes" || exit 1
cmp -s "$prepared/release-notes.md" "$temporary/notes" || fail 'prepared release notes differ from the notes of this revision'

grep -v '^result=' "$temporary/preflight"
printf 'prepared_run=%s\n' "${run:-none}"
printf 'prepared=verified\n'
