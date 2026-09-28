#!/usr/bin/env bash
# Publishes one prepared, verified Axiom artifact set as a GitHub Release.
# Uses the GitHub CLI (`gh`) and `jq`; credentials come only from the caller's
# `gh` environment. Modes:
#   --check     read-only remote publication state (no local set needed);
#   --envelope  read-only publication envelope and preview_digest for the
#               exact local set, notes, prepared run and remote state;
#   (default)   publication, only with --authorized-digest equal to the
#               envelope recomputed here, immediately before the first effect.
#
# Publication order: classify the remote state -> create or reuse one draft
# bound to the exact revision -> replace mismatched draft assets and upload
# missing ones -> read back every asset digest -> publish once -> read back
# release, tag and latest pointer. Published releases are never modified: a
# consistent one is a convergent no-op, any other is a conflict. Duplicate
# releases, foreign draft assets and tags at another revision fail closed.
# Every remote effect is printed as an effect= line.
set -euo pipefail

umask 077

repository_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)
repository=
tag=
revision=
make_latest=
directory=
evidence=
notes=
prepared_run=
authorized_digest=
check=false
envelope_only=false
while (($#)); do
  case "$1" in
    --repo) repository=${2:-}; shift 2 ;;
    --tag) tag=${2:-}; shift 2 ;;
    --revision) revision=${2:-}; shift 2 ;;
    --make-latest) make_latest=${2:-}; shift 2 ;;
    --dir) directory=${2:-}; shift 2 ;;
    --evidence) evidence=${2:-}; shift 2 ;;
    --notes) notes=${2:-}; shift 2 ;;
    --prepared-run) prepared_run=${2:-}; shift 2 ;;
    --authorized-digest) authorized_digest=${2:-}; shift 2 ;;
    --check) check=true; shift ;;
    --envelope) envelope_only=true; shift ;;
    *) printf 'release_publish_error: invalid argument\n' >&2; exit 1 ;;
  esac
done

fail() {
  printf 'release_publish_error: %s\n' "$1" >&2
  exit 1
}

retry_delay=${AXIOM_RELEASE_RETRY_DELAY:-3}
[[ "$repository" =~ ^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$ ]] || fail 'repository must be OWNER/NAME'
tag_facts=$("$repository_root/scripts/release-tag-version.sh" "$tag") || exit 1
version=$(awk -F= '$1 == "version" {print $2}' <<<"$tag_facts")
channel=$(awk -F= '$1 == "channel" {print $2}' <<<"$tag_facts")
prerelease=false
[[ "$channel" == rc ]] && prerelease=true
[[ "$revision" =~ ^[0-9a-f]{40}$ ]] || fail 'full source revision required'
[[ "$make_latest" == true || "$make_latest" == false ]] || fail 'make-latest must be true or false'
[[ "$channel" == stable || "$make_latest" == false ]] || fail 'a release candidate is never latest'
if [[ "$check" == false ]]; then
  [[ "$prepared_run" =~ ^[0-9]+$ ]] || fail 'prepared workflow run id required'
  if [[ "$envelope_only" == false ]]; then
    [[ "$authorized_digest" =~ ^[0-9a-f]{64}$ ]] \
      || fail 'publication requires --authorized-digest: the preview_digest a human authorized'
  fi
fi
command -v gh >/dev/null 2>&1 || fail 'gh is required'
command -v jq >/dev/null 2>&1 || fail 'jq is required'

temporary=$(mktemp -d)
trap 'rm -rf -- "$temporary"' EXIT

digest() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  else
    shasum -a 256 "$1" | awk '{print $1}'
  fi
}

# --- Local verified set -----------------------------------------------------
# The uploaded bytes must be exactly the bytes verify-release-artifacts.sh
# accepted: its Evidence names every archive digest and the SHA256SUMS digest.
local_set=false
if [[ "$check" == false || -n "$directory" ]]; then
  [[ "$directory" == /* && -d "$directory" && ! -L "$directory" ]] || fail 'absolute artifact directory required'
  [[ -f "$evidence" && -f "$notes" ]] || fail 'verification evidence and notes files required'
  grep -Fxq "version=$version" "$evidence" || fail 'evidence does not name this version'
  grep -Fxq "revision=$revision" "$evidence" || fail 'evidence does not name this revision'
  grep -Fxq 'publication=none' "$evidence" && grep -Fxq 'result=pass' "$evidence" || fail 'evidence is not a passing verification'
  awk '$1 ~ /^archive=/ && $2 ~ /^sha256=/ {sub(/^archive=/, "", $1); sub(/^sha256=/, "", $2); print $2 "  " $1}' "$evidence" \
    | LC_ALL=C sort >"$temporary/evidence-archives"
  [[ -s "$temporary/evidence-archives" ]] || fail 'evidence lists no archives'
  LC_ALL=C sort "$directory/SHA256SUMS" >"$temporary/local-sums" 2>/dev/null || fail 'SHA256SUMS missing'
  cmp -s "$temporary/evidence-archives" "$temporary/local-sums" || fail 'SHA256SUMS differs from verified evidence'
  grep -Fxq "sha256sums=$(digest "$directory/SHA256SUMS")" "$evidence" || fail 'SHA256SUMS digest differs from verified evidence'
  { printf 'SHA256SUMS\n'; awk '{print $2}' "$temporary/local-sums"; } | LC_ALL=C sort >"$temporary/expected-names"
  find "$directory" -mindepth 1 -maxdepth 1 -exec basename {} \; | LC_ALL=C sort >"$temporary/local-names"
  cmp -s "$temporary/expected-names" "$temporary/local-names" || fail 'artifact directory is not exactly the verified set'
  : >"$temporary/expected"
  while IFS= read -r name; do
    [[ -f "$directory/$name" && ! -L "$directory/$name" ]] || fail "artifact is not a regular file: $name"
    sum=$(digest "$directory/$name")
    if [[ "$name" != SHA256SUMS ]]; then
      grep -Fxq "$sum  $name" "$temporary/local-sums" || fail "checksum mismatch: $name"
    fi
    printf '%s %s %s\n' "$name" "$sum" "$(wc -c <"$directory/$name" | tr -d ' ')" >>"$temporary/expected"
  done <"$temporary/expected-names"
  local_set=true
fi

# --- Remote facts -------------------------------------------------------------
releases_json() {
  gh api --paginate "repos/$repository/releases?per_page=100" | jq -s 'add // []'
}

# remote_tag_commit prints the commit the tag points at, or nothing.
remote_tag_commit() {
  local refs sha type depth=0
  refs=$(gh api "repos/$repository/git/matching-refs/tags/$tag") || fail 'cannot read remote tags'
  sha=$(jq -r --arg ref "refs/tags/$tag" '[.[] | select(.ref == $ref)] | if length == 1 then .[0].object.sha else empty end' <<<"$refs")
  type=$(jq -r --arg ref "refs/tags/$tag" '[.[] | select(.ref == $ref)] | if length == 1 then .[0].object.type else empty end' <<<"$refs")
  while [[ "$type" == tag ]]; do
    ((depth++ < 3)) || fail 'tag object chain too deep'
    refs=$(gh api "repos/$repository/git/tags/$sha") || fail 'cannot read tag object'
    type=$(jq -r '.object.type' <<<"$refs")
    sha=$(jq -r '.object.sha' <<<"$refs")
  done
  [[ -z "$sha" || "$type" == commit ]] || fail 'tag does not point at a commit'
  printf '%s' "$sha"
}

# asset_sha256 JSON prints the asset's SHA-256, from GitHub's digest field
# when present, otherwise by downloading it.
asset_sha256() {
  local asset=$1 value id
  value=$(jq -r '.digest // empty' <<<"$asset")
  if [[ "$value" == sha256:* ]]; then
    printf '%s' "${value#sha256:}"
    return
  fi
  id=$(jq -r '.id' <<<"$asset")
  gh api -H 'Accept: application/octet-stream' "repos/$repository/releases/assets/$id" >"$temporary/asset-$id" \
    || fail "cannot download asset $id"
  digest "$temporary/asset-$id"
}

# check_published JSON verifies a non-draft release without modifying it.
check_published() {
  local release=$1 sums_asset names
  [[ $(jq -r '.prerelease' <<<"$release") == "$prerelease" ]] || fail 'published release has the wrong prerelease flag'
  [[ "$tag_commit" == "$revision" ]] || fail 'published release tag does not point at the revision'
  sums_asset=$(jq -c '[.assets[] | select(.name == "SHA256SUMS")] | if length == 1 then .[0] else empty end' <<<"$release")
  [[ -n "$sums_asset" ]] || fail 'published release has no single SHA256SUMS asset'
  gh api -H 'Accept: application/octet-stream' "repos/$repository/releases/assets/$(jq -r '.id' <<<"$sums_asset")" \
    >"$temporary/published-sums" || fail 'cannot download published SHA256SUMS'
  [[ $(asset_sha256 "$sums_asset") == $(digest "$temporary/published-sums") ]] || fail 'published SHA256SUMS digest mismatch'
  { printf 'SHA256SUMS\n'; awk '{print $2}' "$temporary/published-sums"; } | LC_ALL=C sort >"$temporary/published-expected"
  jq -r '.assets[].name' <<<"$release" | LC_ALL=C sort >"$temporary/published-names"
  cmp -s "$temporary/published-expected" "$temporary/published-names" || fail 'published assets are not exactly SHA256SUMS plus its archives'
  if [[ "$local_set" == true ]]; then
    cmp -s "$temporary/published-expected" "$temporary/expected-names" || fail 'published asset names differ from the verified set'
  fi
  while read -r sum name; do
    [[ "$sum" =~ ^[0-9a-f]{64}$ && "$name" == "axiom-$version-"*.tar.gz ]] || fail 'published SHA256SUMS line malformed'
    names=$(jq -c --arg name "$name" '[.assets[] | select(.name == $name)] | .[0]' <<<"$release")
    [[ $(asset_sha256 "$names") == "$sum" ]] || fail "published asset differs from SHA256SUMS: $name"
  done <"$temporary/published-sums"
}

check_latest() {
  local latest
  latest=$(gh api "repos/$repository/releases/latest" 2>/dev/null | jq -r '.tag_name // empty') || latest=
  if [[ "$make_latest" == true ]]; then
    [[ "$latest" == "$tag" ]] || fail "latest release is '${latest:-none}', expected $tag"
  else
    [[ "$latest" != "$tag" ]] || fail 'release must not be latest'
  fi
  printf '%s' "${latest:-none}"
}

# label_release_pr hands the merged Release PR from Release Please's pending
# state to tagged, which is what Release Please itself does after tagging.
label_release_pr() {
  local pulls number
  [[ "$channel" == stable ]] || { printf 'release_pr=not_applicable\n'; return; }
  pulls=$(gh api "repos/$repository/commits/$revision/pulls") || fail 'cannot read the release commit pull requests'
  number=$(jq -r '[.[] | select(.merged_at != null) | select(any(.labels[]; .name == "autorelease: pending" or .name == "autorelease: tagged"))] | if length == 1 then .[0].number else empty end' <<<"$pulls")
  if [[ -z "$number" ]]; then
    printf 'release_pr=not_found\n'
    return
  fi
  if jq -e --argjson n "$number" '.[] | select(.number == $n) | any(.labels[]; .name == "autorelease: pending")' <<<"$pulls" >/dev/null; then
    gh api --method POST "repos/$repository/issues/$number/labels" -f 'labels[]=autorelease: tagged' >/dev/null \
      || fail "cannot label Release PR #$number"
    printf 'effect=release_pr_labeled pr=%s label=autorelease:tagged\n' "$number"
    gh api --method DELETE "repos/$repository/issues/$number/labels/autorelease%3A%20pending" >/dev/null \
      || fail "cannot remove pending label from Release PR #$number"
    printf 'effect=release_pr_unlabeled pr=%s label=autorelease:pending\n' "$number"
  fi
  printf 'release_pr=%s\n' "$number"
}

releases=$(releases_json) || fail 'cannot list releases'
matching=$(jq -c --arg tag "$tag" '[.[] | select(.tag_name == $tag)]' <<<"$releases")
count=$(jq 'length' <<<"$matching")
((count <= 1)) || fail "more than one release uses $tag; resolve the duplicates manually"
tag_commit=$(remote_tag_commit)
[[ -z "$tag_commit" || "$tag_commit" == "$revision" ]] || fail "tag $tag exists at another revision"

state=absent
release=
if ((count == 1)); then
  release=$(jq -c '.[0]' <<<"$matching")
  if [[ $(jq -r '.draft' <<<"$release") == true ]]; then
    state=draft
    [[ $(jq -r '.target_commitish' <<<"$release") == "$revision" ]] || fail 'draft release targets another revision'
    [[ $(jq -r '.prerelease' <<<"$release") == "$prerelease" ]] || fail 'draft release has the wrong prerelease flag'
    if [[ "$local_set" == true ]]; then
      jq -r '.assets[].name' <<<"$release" | while IFS= read -r name; do
        grep -q "^$name " "$temporary/expected" || { printf 'release_publish_error: draft has an asset outside the verified set: %s\n' "$name" >&2; exit 1; }
      done
    fi
  else
    state=published
    check_published "$release"
  fi
fi

# --- Publication envelope ---------------------------------------------------
# Deterministic, complete statement of what would be published and of the
# remote state it starts from. Its SHA-256 is the preview_digest that human
# authority binds to; any change requires a new review.
write_envelope() {
  local name sum
  printf 'envelopeVersion=1\n'
  printf 'repository=%s\n' "$repository"
  printf 'tag=%s\n' "$tag"
  printf 'version=%s\n' "$version"
  printf 'channel=%s\n' "$channel"
  printf 'prerelease=%s\n' "$prerelease"
  printf 'revision=%s\n' "$revision"
  printf 'make_latest=%s\n' "$make_latest"
  printf 'prepared_run=%s\n' "$prepared_run"
  printf 'release_notes_sha256=%s\n' "$(digest "$notes")"
  printf 'sha256sums_sha256=%s\n' "$(awk '$1 == "SHA256SUMS" {print $2}' "$temporary/expected")"
  while read -r name sum _; do
    [[ "$name" == SHA256SUMS ]] || printf 'artifact.%s=%s\n' "$name" "$sum"
  done <"$temporary/expected"
  printf 'publication_state=%s\n' "$state"
  printf 'tag_state=%s\n' "$([[ -n "$tag_commit" ]] && printf present || printf absent)"
  printf 'release_id=%s\n' "$([[ -n "$release" ]] && jq -r '.id' <<<"$release" || printf none)"
  if [[ "$state" == draft ]]; then
    while IFS= read -r asset; do
      [[ -n "$asset" ]] || continue
      printf 'draft_asset.%s=%s\n' "$(jq -r '.name' <<<"$asset")" "$(asset_sha256 "$asset")"
    done < <(jq -c '.assets | sort_by(.name) | .[]' <<<"$release")
  fi
  if [[ "$state" == published ]]; then
    printf 'effect=none\n'
    return
  fi
  printf 'effect.release=%s\n' "$([[ "$state" == draft ]] && printf reconcile_draft || printf create_draft)"
  printf 'effect.assets=upload_exact_envelope_set\n'
  printf 'effect.publish=%s\n' "$([[ "$prerelease" == true ]] && printf prerelease || printf release)"
  printf 'effect.tag=%s\n' "$([[ -n "$tag_commit" ]] && printf existing || printf "create_at_revision")"
  printf 'effect.latest=%s\n' "$([[ "$make_latest" == true ]] && printf set || printf unchanged)"
  printf 'effect.release_pr_label=%s\n' "$([[ "$channel" == stable ]] && printf pending_to_tagged || printf none)"
}

if [[ "$check" == false ]]; then
  write_envelope >"$temporary/envelope"
  preview=$(digest "$temporary/envelope")
  if [[ "$envelope_only" == true ]]; then
    cat "$temporary/envelope"
    printf 'preview_digest=%s\n' "$preview"
    exit 0
  fi
  [[ "$preview" == "$authorized_digest" ]] \
    || fail "preview changed; review and authorize again (current preview_digest=$preview)"
fi

printf 'publicationVersion=1\n'
printf 'repository=%s\n' "$repository"
printf 'tag=%s\n' "$tag"
printf 'revision=%s\n' "$revision"
printf 'channel=%s\n' "$channel"
printf 'tag_state=%s\n' "$([[ -n "$tag_commit" ]] && printf present || printf absent)"
printf 'publication_state=%s\n' "$state"
[[ "$check" == false ]] && printf 'authorized_digest=%s\n' "$authorized_digest"
[[ -n "$release" ]] && printf 'release_id=%s\n' "$(jq -r '.id' <<<"$release")"

if [[ "$check" == true ]]; then
  printf 'result=pass\n'
  exit 0
fi

if [[ "$state" == published ]]; then
  latest=$(check_latest)
  printf 'latest=%s\n' "$latest"
  label_release_pr
  printf 'immutable=%s\n' "$(jq -r 'if has("immutable") then .immutable else "unknown" end' <<<"$release")"
  printf 'publication=already_published\n'
  printf 'result=pass\n'
  exit 0
fi

# --- Draft: create or reconcile -----------------------------------------------
if [[ "$state" == absent ]]; then
  jq -n --arg tag "$tag" --arg revision "$revision" --rawfile body "$notes" --argjson prerelease "$prerelease" \
    '{tag_name: $tag, target_commitish: $revision, name: $tag, body: $body, draft: true, prerelease: $prerelease}' \
    >"$temporary/create.json"
  release=$(gh api --method POST "repos/$repository/releases" --input "$temporary/create.json") || fail 'cannot create draft release'
  printf 'effect=draft_created release_id=%s\n' "$(jq -r '.id' <<<"$release")"
else
  jq -n --rawfile body "$notes" '{body: $body}' >"$temporary/body.json"
  release=$(gh api --method PATCH "repos/$repository/releases/$(jq -r '.id' <<<"$release")" --input "$temporary/body.json") \
    || fail 'cannot refresh draft notes'
  printf 'effect=draft_notes_refreshed release_id=%s\n' "$(jq -r '.id' <<<"$release")"
fi
release_id=$(jq -r '.id' <<<"$release")
[[ "$release_id" =~ ^[0-9]+$ ]] || fail 'draft release has no id'
[[ $(jq -r '.draft' <<<"$release") == true ]] || fail 'release is not a draft'

# Draft assets from an earlier interrupted run are kept only when they are
# byte-identical to this verified set; otherwise they are replaced.
while IFS= read -r asset; do
  [[ -n "$asset" ]] || continue
  name=$(jq -r '.name' <<<"$asset")
  expected=$(awk -v name="$name" '$1 == name {print $2}' "$temporary/expected")
  if [[ $(jq -r '.state' <<<"$asset") == uploaded && $(asset_sha256 "$asset") == "$expected" ]]; then
    printf 'draft_asset_kept=%s\n' "$name"
    continue
  fi
  gh api --method DELETE "repos/$repository/releases/assets/$(jq -r '.id' <<<"$asset")" >/dev/null || fail "cannot delete draft asset $name"
  printf 'effect=draft_asset_deleted name=%s\n' "$name"
done < <(jq -c '.assets[]' <<<"$release")

release=$(gh api "repos/$repository/releases/$release_id") || fail 'cannot read draft release'
upload_url=$(jq -r '.upload_url' <<<"$release")
upload_url=${upload_url%%\{*}
[[ "$upload_url" == https://* ]] || fail 'draft release has no upload URL'
while read -r name sum _; do
  if jq -e --arg name "$name" 'any(.assets[]; .name == $name)' <<<"$release" >/dev/null; then
    continue
  fi
  gh api --method POST "$upload_url?name=$name" -H 'Content-Type: application/octet-stream' \
    --input "$directory/$name" >/dev/null || fail "cannot upload $name (draft left for a rerun)"
  printf 'effect=draft_asset_uploaded name=%s sha256=%s\n' "$name" "$sum"
done <"$temporary/expected"

# Read back the complete draft before it becomes public.
release=$(gh api "repos/$repository/releases/$release_id") || fail 'cannot read draft release'
jq -r '.assets[].name' <<<"$release" | LC_ALL=C sort >"$temporary/draft-names"
cmp -s "$temporary/expected-names" "$temporary/draft-names" || fail 'draft assets are not exactly the verified set (draft left for a rerun)'
while read -r name sum size; do
  asset=$(jq -c --arg name "$name" '.assets[] | select(.name == $name)' <<<"$release")
  [[ $(jq -r '.state' <<<"$asset") == uploaded ]] || fail "draft asset not uploaded: $name"
  [[ $(jq -r '.size' <<<"$asset") == "$size" ]] || fail "draft asset size mismatch: $name"
  [[ $(asset_sha256 "$asset") == "$sum" ]] || fail "draft asset digest mismatch: $name"
done <"$temporary/expected"
printf 'draft_verified=%s\n' "$release_id"

tag_commit=$(remote_tag_commit)
[[ -z "$tag_commit" || "$tag_commit" == "$revision" ]] || fail "tag $tag appeared at another revision; draft left unpublished"

# --- Publish once ---------------------------------------------------------------
jq -n --argjson prerelease "$prerelease" --arg latest "$make_latest" \
  '{draft: false, prerelease: $prerelease, make_latest: $latest}' >"$temporary/publish.json"
release=$(gh api --method PATCH "repos/$repository/releases/$release_id" --input "$temporary/publish.json") \
  || fail 'publication request failed; rerun to reconcile the draft or published state'
printf 'effect=release_published release_id=%s\n' "$release_id"

# --- Read back ----------------------------------------------------------------
attempt=0
while :; do
  release=$(gh api "repos/$repository/releases/$release_id") || fail 'cannot read published release'
  tag_commit=$(remote_tag_commit)
  if [[ $(jq -r '.draft' <<<"$release") == false && "$tag_commit" == "$revision" ]]; then
    break
  fi
  ((attempt++ < 5)) || fail 'published release or tag did not read back; rerun to verify'
  sleep "$retry_delay"
done
[[ $(jq -r '.tag_name' <<<"$release") == "$tag" ]] || fail 'published release has the wrong tag'
check_published "$release"
latest=$(check_latest)
printf 'latest=%s\n' "$latest"
label_release_pr
printf 'release_url=%s\n' "$(jq -r '.html_url' <<<"$release")"
printf 'immutable=%s\n' "$(jq -r 'if has("immutable") then .immutable else "unknown" end' <<<"$release")"
printf 'publication=published\n'
printf 'result=pass\n'
