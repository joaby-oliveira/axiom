#!/usr/bin/env bash
# Decides, from Git facts only, whether one tag may be published from one exact
# revision. Read-only: it never creates tags, releases or refs. Prints closed
# key=value Evidence ending in result=pass, or release_preflight_error on
# stderr with exit 1.
#
# Contract (see CONTRIBUTING.md "Release flow"):
# - the revision is a full SHA on the first-parent history of the main ref;
# - stable vX.Y.Z: the revision is the release commit, i.e. its
#   .release-please-manifest.json records X.Y.Z and its first parent does not
#   (the merged Release PR); X.Y.Z is newer than every published stable tag;
# - release candidate vX.Y.Z-rc.N: the manifest at the revision is not newer
#   than X.Y.Z, vX.Y.Z does not exist, X.Y.Z is newer than every stable tag and
#   N is greater than every existing vX.Y.Z-rc.* number;
# - an existing tag is accepted only when it already points at the revision
#   (convergent rerun); it is never moved.
set -euo pipefail

repository_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)
tag=
revision=
main_ref=origin/main
remote=origin
while (($#)); do
  case "$1" in
    --tag) tag=${2:-}; shift 2 ;;
    --revision) revision=${2:-}; shift 2 ;;
    --main-ref) main_ref=${2:-}; shift 2 ;;
    --remote) remote=${2:-}; shift 2 ;;
    *) printf 'release_preflight_error: invalid argument\n' >&2; exit 1 ;;
  esac
done

fail() {
  printf 'release_preflight_error: %s\n' "$1" >&2
  exit 1
}

tag_facts=$("$repository_root/scripts/release-tag-version.sh" "$tag") || exit 1
version=$(awk -F= '$1 == "version" {print $2}' <<<"$tag_facts")
channel=$(awk -F= '$1 == "channel" {print $2}' <<<"$tag_facts")
core=${version%%-*}

[[ "$revision" =~ ^[0-9a-f]{40}$ ]] || fail 'full source revision required'
git -C "$repository_root" cat-file -e "$revision^{commit}" 2>/dev/null || fail 'revision is not a known commit'
main_sha=$(git -C "$repository_root" rev-parse --verify --quiet "$main_ref^{commit}") || fail "main ref not found: $main_ref"
# grep -q must not close a pipe under pipefail; read the complete list.
grep -Fxq "$revision" <(git -C "$repository_root" rev-list --first-parent "$main_sha") \
  || fail 'revision is not on the first-parent history of main'

# compare_core A B prints -1, 0 or 1 for MAJOR.MINOR.PATCH values.
compare_core() {
  local -a a b
  IFS=. read -r -a a <<<"$1"
  IFS=. read -r -a b <<<"$2"
  local i
  for i in 0 1 2; do
    if ((10#${a[i]} < 10#${b[i]})); then printf -- '-1'; return; fi
    if ((10#${a[i]} > 10#${b[i]})); then printf '1'; return; fi
  done
  printf '0'
}

# manifest_version REV prints the root version recorded by Release Please, or
# "none" when the manifest is absent. Any other shape fails closed.
manifest_version() {
  local content
  if ! content=$(git -C "$repository_root" show "$1:.release-please-manifest.json" 2>/dev/null); then
    printf 'none'
    return
  fi
  content=$(tr -d ' \t\r\n' <<<"$content")
  [[ "$content" =~ ^\{\"\.\":\"((0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*))\"\}$ ]] \
    || fail 'release manifest is malformed'
  printf '%s' "${BASH_REMATCH[1]}"
}

manifest=$(manifest_version "$revision")
[[ "$manifest" != none ]] || fail 'revision predates the release flow (.release-please-manifest.json missing)'

# Remote tags, peeled to commits. Tags outside the SemVer release policy (for
# example the historical v0.1.0-poc.1) are ignored.
if ! remote_tags=$(git -C "$repository_root" ls-remote --tags "$remote" 2>/dev/null); then
  fail "cannot read tags from remote: $remote"
fi
# Bash 3.2 compatible "name commit" table; peeled ^{} entries win.
tag_table=$(awk '
  NF == 2 {
    name = $2; sub(/^refs\/tags\//, "", name)
    if (name ~ /\^\{\}$/) { sub(/\^\{\}$/, "", name); peeled[name] = $1 }
    else if (!(name in plain)) { plain[name] = $1 }
  }
  END {
    for (name in plain) print name, ((name in peeled) ? peeled[name] : plain[name])
  }' <<<"$remote_tags")
tag_commit() {
  awk -v name="$1" '$1 == name {print $2}' <<<"$tag_table"
}

tag_state=absent
existing=$(tag_commit "$tag")
if [[ -n "$existing" ]]; then
  [[ "$existing" == "$revision" ]] || fail "tag already exists at another revision: $tag"
  tag_state=present
fi

latest_stable=none
max_rc=0
numeric='(0|[1-9][0-9]*)'
while read -r name _; do
  [[ -n "${name:-}" ]] || continue
  if [[ "$name" =~ ^v(${numeric}\.${numeric}\.${numeric})$ ]]; then
    candidate=${BASH_REMATCH[1]}
    if [[ "$latest_stable" == none || $(compare_core "$candidate" "$latest_stable") == 1 ]]; then
      latest_stable=$candidate
    fi
  elif [[ "$name" =~ ^v${core//./\\.}-rc\.${numeric}$ && "$name" != "$tag" ]]; then
    if ((10#${BASH_REMATCH[1]} > max_rc)); then max_rc=$((10#${BASH_REMATCH[1]})); fi
  fi
done <<<"$tag_table"

newer_than_stable() {
  [[ "$latest_stable" == none || $(compare_core "$core" "$latest_stable") == 1 ]]
}

make_latest=false
release_commit=
if [[ "$channel" == stable ]]; then
  [[ "$manifest" == "$version" ]] \
    || fail "revision does not record version $version in .release-please-manifest.json (merge the Release PR first)"
  parent=$(git -C "$repository_root" rev-parse --verify --quiet "$revision^1") || fail 'release commit has no parent'
  parent_manifest=$(manifest_version "$parent")
  [[ "$parent_manifest" != "$version" ]] \
    || fail 'revision is not the release commit that introduced this version'
  release_commit=$revision
  if [[ "$tag_state" == absent ]]; then
    newer_than_stable || fail "version is not newer than the latest stable release v$latest_stable"
  fi
  if [[ "$latest_stable" == none || $(compare_core "$core" "$latest_stable") != -1 ]]; then
    make_latest=true
  fi
else
  if [[ $(compare_core "$manifest" "$core") == 1 ]]; then
    fail "revision already records a newer version $manifest"
  fi
  [[ -z $(tag_commit "v$core") ]] || fail "stable v$core already exists; a release candidate cannot follow it"
  if [[ "$tag_state" == absent ]]; then
    newer_than_stable || fail "version is not newer than the latest stable release v$latest_stable"
    rc_number=${version##*-rc.}
    ((10#$rc_number > max_rc)) || fail "release candidate number must be greater than existing rc.$max_rc"
  fi
fi

printf 'preflightVersion=1\n'
printf 'tag=%s\n' "$tag"
printf 'version=%s\n' "$version"
printf 'channel=%s\n' "$channel"
printf 'prerelease=%s\n' "$([[ "$channel" == rc ]] && printf true || printf false)"
printf 'revision=%s\n' "$revision"
printf 'main=%s\n' "$main_sha"
printf 'manifest_version=%s\n' "$manifest"
printf 'release_commit=%s\n' "${release_commit:-not_applicable}"
printf 'latest_stable=%s\n' "$latest_stable"
printf 'tag_state=%s\n' "$tag_state"
printf 'make_latest=%s\n' "$make_latest"
printf 'result=pass\n'
