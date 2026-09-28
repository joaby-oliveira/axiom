#!/usr/bin/env bash
# S9/T38 release artifact pipeline tests. Release builds and verification run
# in a clean clone of the committed HEAD, exactly as the release workflow
# checks out one revision; uncommitted changes are not part of a release.
# No network, tag, release, or repository mutation is performed.
set -euo pipefail

umask 077

repository_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)
temporary=$(mktemp -d)
temporary=$(cd "$temporary" && pwd -P)
trap 'rm -rf -- "$temporary"' EXIT

digest() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  else
    shasum -a 256 "$1" | awk '{print $1}'
  fi
}

expect_failure() {
  local label=$1 pattern=$2
  shift 2
  if "$@" >"$temporary/out" 2>"$temporary/err"; then
    printf 'FAIL: %s unexpectedly succeeded\n' "$label" >&2
    exit 1
  fi
  if ! grep -Fq -- "$pattern" "$temporary/err"; then
    printf 'FAIL: %s did not report %s\n' "$label" "$pattern" >&2
    cat "$temporary/err" >&2
    exit 1
  fi
}

# 1. Tag -> semantic version contract.
tag_version=$repository_root/scripts/release-tag-version.sh
[[ $("$tag_version" v0.1.0) == $'tag=v0.1.0\nversion=0.1.0\nchannel=stable' ]]
[[ $("$tag_version" v0.1.0-rc.2) == $'tag=v0.1.0-rc.2\nversion=0.1.0-rc.2\nchannel=rc' ]]
[[ $("$tag_version" v10.20.30-rc.12) == $'tag=v10.20.30-rc.12\nversion=10.20.30-rc.12\nchannel=rc' ]]
for invalid in 0.1.0 v0.1 v01.0.0 v0.1.0-rc.01 v0.1.0-rc v0.1.0-beta.1 v0.1.0-poc.1 v0.1.0+build 'v0.1.0-rc.1;touch pwned' ' v0.1.0' ''; do
  expect_failure "tag '$invalid'" 'release_tag_error' "$tag_version" "$invalid"
done
expect_failure 'two tags' 'release_tag_error' "$tag_version" v0.1.0 v0.1.1
[[ ! -e pwned ]]

# 2. Exact clean revision.
revision=$(git -C "$repository_root" rev-parse --verify HEAD)
source="$temporary/source"
git clone -q --no-hardlinks "$repository_root" "$source"
git -C "$source" checkout -q --detach "$revision"
build="$source/scripts/build-release-archives.sh"
verify="$source/scripts/verify-release-artifacts.sh"

: >"$source/untracked-probe"
expect_failure 'dirty source' 'release archive requires clean source' "$build" --version 0.1.0-rc.1 --output "$temporary/dirty"
[[ -z $(find "$temporary/dirty" -type f 2>/dev/null) ]]
rm -- "$source/untracked-probe"
expect_failure 'invalid version' 'exact semantic version required' "$build" --version v0.1.0-rc.1 --output "$temporary/invalid"

# 3. Complete supported artifact set and verification Evidence.
"$build" --version 0.1.0-rc.1 --output "$temporary/first" >/dev/null
"$verify" --dir "$temporary/first" --version 0.1.0-rc.1 --revision "$revision" >"$temporary/evidence"
grep -Fxq "revision=$revision" "$temporary/evidence"
grep -Fxq 'publication=none' "$temporary/evidence"
grep -Fxq 'result=pass' "$temporary/evidence"
for row in macos-27-arm64 ubuntu-26.04-amd64 ubuntu-26.04-arm64; do
  grep -Eq "^archive=axiom-0\\.1\\.0-rc\\.1-$row\\.tar\\.gz sha256=[0-9a-f]{64} " "$temporary/evidence"
done
[[ $(grep -c '^archive=' "$temporary/evidence") == 3 ]]
case "$(go env GOHOSTOS)/$(go env GOHOSTARCH)" in
  linux/amd64|linux/arm64|darwin/arm64) [[ $(grep -c 'version_smoke=pass' "$temporary/evidence") == 1 ]] ;;
esac

# 4. Rerun from the same revision: identical bundle contents and executables.
"$build" --version 0.1.0-rc.1 --output "$temporary/second" >/dev/null
"$verify" --dir "$temporary/second" --version 0.1.0-rc.1 --revision "$revision" >/dev/null
[[ $(grep -o 'axiom_sha256=[0-9a-f]* manifest_sha256=[0-9a-f]*' "$temporary/evidence") == \
   $("$verify" --dir "$temporary/second" --version 0.1.0-rc.1 --revision "$revision" | grep -o 'axiom_sha256=[0-9a-f]* manifest_sha256=[0-9a-f]*') ]]
if cmp -s "$temporary/first/SHA256SUMS" "$temporary/second/SHA256SUMS"; then
  printf 'rerun_archives=byte_identical\n'
else
  printf 'rerun_archives=content_identical_archive_bytes_differ\n'
fi
expect_failure 'non-empty output' 'output directory must be empty' "$build" --version 0.1.0-rc.1 --output "$temporary/first"

# 5. Incomplete, inconsistent, or foreign sets fail closed.
mutate() {
  local name=$1
  rm -rf -- "$temporary/$name"
  cp -R "$temporary/first" "$temporary/$name"
  printf '%s\n' "$temporary/$name"
}
case_dir=$(mutate missing-row)
rm -- "$case_dir/axiom-0.1.0-rc.1-ubuntu-26.04-arm64.tar.gz"
expect_failure 'missing row' 'artifact set is not exactly' "$verify" --dir "$case_dir" --version 0.1.0-rc.1 --revision "$revision"

case_dir=$(mutate missing-checksum)
grep -v 'macos-27-arm64' "$temporary/first/SHA256SUMS" >"$case_dir/SHA256SUMS"
expect_failure 'missing checksum line' 'SHA256SUMS must list exactly the three archives' "$verify" --dir "$case_dir" --version 0.1.0-rc.1 --revision "$revision"

case_dir=$(mutate checksum-mismatch)
printf 'tamper\n' >>"$case_dir/axiom-0.1.0-rc.1-ubuntu-26.04-amd64.tar.gz"
expect_failure 'checksum mismatch' 'checksum mismatch' "$verify" --dir "$case_dir" --version 0.1.0-rc.1 --revision "$revision"

case_dir=$(mutate extra-file)
printf 'extra\n' >"$case_dir/notes.txt"
expect_failure 'extra file' 'artifact set is not exactly' "$verify" --dir "$case_dir" --version 0.1.0-rc.1 --revision "$revision"

case_dir=$(mutate symlinked-archive)
mv "$case_dir/axiom-0.1.0-rc.1-macos-27-arm64.tar.gz" "$temporary/outside.tar.gz"
ln -s "$temporary/outside.tar.gz" "$case_dir/axiom-0.1.0-rc.1-macos-27-arm64.tar.gz"
expect_failure 'symlinked archive' 'artifact is not a regular file' "$verify" --dir "$case_dir" --version 0.1.0-rc.1 --revision "$revision"

expect_failure 'wrong version' 'artifact set is not exactly' "$verify" --dir "$temporary/first" --version 0.1.0-rc.2 --revision "$revision"
expect_failure 'wrong revision' 'checkout is not the artifact source revision' "$verify" --dir "$temporary/first" --version 0.1.0-rc.1 --revision "$(printf '0%.0s' {1..40})"

# A bundle that ships lingo instead of the canonical axiom executable.
case_dir=$(mutate lingo-bundle)
bundle=axiom-0.1.0-rc.1-ubuntu-26.04-amd64
mkdir "$temporary/repack"
tar -xzf "$case_dir/$bundle.tar.gz" -C "$temporary/repack"
mv "$temporary/repack/$bundle/axiom" "$temporary/repack/$bundle/lingo"
(cd "$temporary/repack/$bundle" && find . -type f ! -name MANIFEST.sha256 | sed 's#^./##' | LC_ALL=C sort | while IFS= read -r file; do printf '%s  %s\n' "$(digest "$file")" "$file"; done >MANIFEST.sha256)
COPYFILE_DISABLE=1 tar -C "$temporary/repack" -czf "$case_dir/$bundle.tar.gz" "$bundle"
grep -v "$bundle" "$temporary/first/SHA256SUMS" >"$case_dir/SHA256SUMS"
printf '%s  %s\n' "$(digest "$case_dir/$bundle.tar.gz")" "$bundle.tar.gz" >>"$case_dir/SHA256SUMS"
expect_failure 'lingo bundle' 'archive entries are not the closed bundle set' "$verify" --dir "$case_dir" --version 0.1.0-rc.1 --revision "$revision"

# A development (non-release) build is not a release artifact set.
"$build" --version 0.1.0-rc.1 --output "$temporary/development" --development >/dev/null
expect_failure 'development build' 'release metadata does not match' "$verify" --dir "$temporary/development" --version 0.1.0-rc.1 --revision "$revision"
[[ -z $(git -C "$source" status --porcelain --untracked-files=normal) ]]

# 6. The workflow prepares artifacts only: read-only token, manual exact-tag
# trigger, pinned actions, no publication or repository mutation.
workflow="$repository_root/.github/workflows/release-artifacts.yml"
[[ $(grep -c '^permissions:' "$workflow") == 1 && $(grep -A1 '^permissions:' "$workflow" | tail -n 1) == '  contents: read' ]]
[[ $(grep -Ec '^\s+permissions:' "$workflow") == 0 ]]
[[ $(sed -n '/^on:/,/^[a-z]/p' "$workflow" | grep -Ec '^  [a-z_]+:') == 1 ]]
grep -Eq '^  workflow_dispatch:$' "$workflow"
if grep -Eiq 'gh release|git push|git tag|contents: write|softprops|action-gh-release|create-release|upload-release|releases/|secrets\.|GITHUB_TOKEN|id-token|packages:' "$workflow"; then
  printf 'FAIL: release workflow contains a publication or credential effect\n' >&2
  exit 1
fi
while IFS= read -r line; do
  [[ "$line" =~ uses:\ [A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+@[0-9a-f]{40}\ \#\ v[0-9.]+$ ]] || { printf 'FAIL: unpinned action: %s\n' "$line" >&2; exit 1; }
done < <(grep -E '^\s+(- )?uses:' "$workflow")
[[ $(grep -c 'inputs.tag' "$workflow") == $(grep -c 'RELEASE_TAG: \${{ inputs.tag }}' "$workflow") ]]
grep -Fq 'persist-credentials: false' "$workflow"

printf 'tested_revision=%s\n' "$revision"
printf '%s\n' 'PASS: S9 release artifact pipeline, closed verification, rerun equivalence, and no-publication workflow'
