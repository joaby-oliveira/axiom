#!/usr/bin/env bash
# Maintainer release orchestration used by the $axiom-release skill.
#
#   release.sh status  [--tag vX.Y.Z[-rc.N]] [--revision SHA] [--prepared-run ID]
#   release.sh prepare --tag TAG [--revision SHA]
#   release.sh publish --tag TAG --revision SHA --prepared-run ID --preview-digest DIGEST --authorize-publication
#   release.sh verify  --tag TAG [--download]
#
# PREPARE: `prepare` dispatches release-artifacts.yml (read-only token; it
# builds, verifies and retains the exact artifact set, and publishes nothing),
# waits for it, then prints the publication envelope of that prepared set.
# status with --prepared-run downloads the prepared set, re-verifies it in a
# clean clone of the revision and prints the envelope and its preview_digest.
# PUBLISH: `publish` dispatches publish-release.yml only with
# --authorize-publication and a DIGEST equal to the envelope recomputed now;
# the workflow recomputes it again from the same prepared bytes before the
# first effect. status and verify are read-only apart from `git fetch` of main
# and temporary files. This script never creates tags or releases itself and
# never approves the `release` environment.
# Release rules live in release-preflight.sh, publish-release.sh and the
# workflows; this script only gathers facts and chooses the next step.
set -euo pipefail

umask 077

repository_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)
scripts=$repository_root/scripts
workflow=publish-release.yml
prepare_workflow=release-artifacts.yml
command=${1:-}
[[ -n "$command" ]] && shift
tag=
revision=
preview_digest=
prepared_run=
authorized=false
download=false
while (($#)); do
  case "$1" in
    --tag) tag=${2:-}; shift 2 ;;
    --revision) revision=${2:-}; shift 2 ;;
    --preview-digest) preview_digest=${2:-}; shift 2 ;;
    --prepared-run) prepared_run=${2:-}; shift 2 ;;
    --authorize-publication) authorized=true; shift ;;
    --download) download=true; shift ;;
    *) printf 'release_error: invalid argument\n' >&2; exit 1 ;;
  esac
done

fail() {
  printf 'release_error: %s\n' "$1" >&2
  exit 1
}

temporary=$(mktemp -d)
trap 'rm -rf -- "$temporary"' EXIT

digest_stdin() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum | awk '{print $1}'
  else
    shasum -a 256 | awk '{print $1}'
  fi
}

value() {
  awk -F= -v key="$1" '$1 == key {sub(/^[^=]*=/, ""); print; exit}' "$2"
}

command -v gh >/dev/null 2>&1 || fail 'gh is required'
command -v jq >/dev/null 2>&1 || fail 'jq is required'
repository=$(gh repo view --json nameWithOwner --jq .nameWithOwner 2>/dev/null) || fail 'cannot resolve the GitHub repository'
[[ "$repository" =~ ^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$ ]] || fail 'unexpected repository name'

# ci_state SHA prints success, pending, failure or missing for the required
# checks named by the versioned main ruleset.
ci_state() {
  local runs name conclusion result=success
  runs=$(gh api "repos/$repository/commits/$1/check-runs?per_page=100") || { printf 'unknown'; return; }
  while IFS= read -r name; do
    conclusion=$(jq -r --arg name "$name" \
      '[.check_runs[] | select(.name == $name)] | sort_by(.started_at) | last | if . == null then "missing" elif .status != "completed" then "pending" else .conclusion end' <<<"$runs")
    case "$conclusion" in
      success) ;;
      pending) [[ "$result" == success ]] && result=pending ;;
      missing) [[ "$result" == success || "$result" == pending ]] && result=missing ;;
      *) result=failure ;;
    esac
  done < <(jq -r '.rules[] | select(.type == "required_status_checks") | .parameters.required_status_checks[].context' \
    "$repository_root/.github/rulesets/main.json")
  printf '%s' "$result"
}

# release_environment prints protected only when the `release` environment
# that gates publish-release.yml requires a reviewer.
release_environment() {
  local environment
  environment=$(gh api "repos/$repository/environments/release" 2>/dev/null) || { printf 'missing'; return; }
  if jq -e 'any(.protection_rules[]?; .type == "required_reviewers")' <<<"$environment" >/dev/null; then
    printf 'protected'
  else
    printf 'unprotected'
  fi
}

# release_commit_for VERSION prints the first-parent main commit whose
# Release Please manifest introduced VERSION, or nothing.
release_commit_for() {
  local sha
  while IFS= read -r sha; do
    if [[ $(git -C "$repository_root" show "$sha:.release-please-manifest.json" 2>/dev/null | tr -d ' \t\r\n') == "{\".\":\"$1\"}" ]] \
      && [[ $(git -C "$repository_root" show "$sha^1:.release-please-manifest.json" 2>/dev/null | tr -d ' \t\r\n') != "{\".\":\"$1\"}" ]]; then
      printf '%s' "$sha"
      return
    fi
  done < <(git -C "$repository_root" log --first-parent --format=%H origin/main -- .release-please-manifest.json)
}

# prepared_envelope downloads the prepared set of $prepared_run, verifies it
# in a clean clone of $revision with that revision's scripts, and writes the
# publication envelope to $temporary/envelope. On refusal it sets reason.
prepared_envelope() {
  local dir=$temporary/prepared clone=$temporary/source origin_url
  [[ "$prepared_run" =~ ^[0-9]+$ ]] || { reason='prepared run id must be numeric'; return 1; }
  rm -rf -- "$dir" "$clone"
  mkdir "$dir"
  printf 'prepared_run=%s\n' "$prepared_run" >>"$temporary/status"
  if ! gh run download "$prepared_run" --repo "$repository" --name "axiom-release-$tag" --dir "$dir" >/dev/null 2>&1; then
    reason="cannot download axiom-release-$tag from run $prepared_run"
    return 1
  fi
  origin_url=$(git -C "$repository_root" remote get-url origin)
  git clone --quiet --no-hardlinks "$repository_root" "$clone"
  git -C "$clone" checkout --quiet --detach "$revision"
  git -C "$clone" fetch --quiet "$repository_root" "+refs/remotes/origin/main:refs/remotes/origin/main"
  if ! "$clone/scripts/verify-prepared-release.sh" --tag "$tag" --revision "$revision" --prepared "$dir" \
    --repo "$repository" --run "$prepared_run" --remote "$origin_url" >"$temporary/prepared-facts" 2>"$temporary/prepared-error"; then
    reason=$(sed -E 's/^[a-z_]+_error: //' "$temporary/prepared-error" | head -n 1)
    return 1
  fi
  if ! "$clone/scripts/publish-release.sh" --envelope --repo "$repository" --tag "$tag" --revision "$revision" \
    --make-latest "$(value make_latest "$temporary/prepared-facts")" --prepared-run "$prepared_run" --dir "$dir/artifacts" \
    --evidence "$dir/release-evidence.txt" --notes "$dir/release-notes.md" >"$temporary/envelope" 2>"$temporary/envelope-error"; then
    reason=$(sed 's/^release_publish_error: //' "$temporary/envelope-error" | head -n 1)
    return 1
  fi
}

# find_run WORKFLOW SINCE prints the newest dispatch run of WORKFLOW created
# at or after SINCE, as JSON, or nothing.
find_run() {
  local run=
  for _ in 1 2 3 4 5 6 7 8 9 10; do
    run=$(gh run list --repo "$repository" --workflow "$1" --event workflow_dispatch --limit 10 \
      --json databaseId,url,createdAt | jq -c --arg since "$2" '[.[] | select(.createdAt >= $since)] | sort_by(.createdAt) | last // empty')
    [[ -n "$run" ]] && break
    sleep "${AXIOM_RELEASE_RETRY_DELAY:-3}"
  done
  printf '%s' "$run"
}

# status writes key=value facts and the next step to $temporary/status.
status() {
  local out=$temporary/status next reason='' main head branch worktree open merged
  : >"$out"
  git -C "$repository_root" fetch --quiet origin main || fail 'cannot fetch origin main'
  main=$(git -C "$repository_root" rev-parse --verify origin/main)
  head=$(git -C "$repository_root" rev-parse --verify HEAD)
  branch=$(git -C "$repository_root" branch --show-current)
  worktree=clean
  [[ -z $(git -C "$repository_root" status --porcelain --untracked-files=normal) ]] || worktree=dirty
  open=$(gh pr list --repo "$repository" --state open --label 'autorelease: pending' --json number,url,title --limit 10) \
    || fail 'cannot list open Release PRs'
  merged=$(gh pr list --repo "$repository" --state merged --label 'autorelease: pending' --json number,url,title,mergeCommit --limit 10) \
    || fail 'cannot list merged Release PRs'
  {
    printf 'statusVersion=1\n'
    printf 'repository=%s\n' "$repository"
    printf 'root=%s\n' "$repository_root"
    printf 'branch=%s\n' "${branch:-detached}"
    printf 'head=%s\n' "$head"
    printf 'worktree=%s\n' "$worktree"
    printf 'main=%s\n' "$main"
    printf 'main_ci=%s\n' "$(ci_state "$main")"
    printf 'release_environment=%s\n' "$(release_environment)"
    printf 'open_release_prs=%s\n' "$(jq -r 'map("#\(.number)") | join(",") | if . == "" then "none" else . end' <<<"$open")"
    printf 'unpublished_release_prs=%s\n' "$(jq -r 'map("#\(.number)") | join(",") | if . == "" then "none" else . end' <<<"$merged")"
  } >>"$out"

  if [[ -z "$tag" ]]; then
    if (($(jq 'length' <<<"$merged") > 1)); then
      next=blocked; reason='more than one merged Release PR awaits publication'
    elif (($(jq 'length' <<<"$merged") == 1)); then
      revision=$(jq -r '.[0].mergeCommit.oid' <<<"$merged")
      local manifest
      manifest=$(git -C "$repository_root" show "$revision:.release-please-manifest.json" 2>/dev/null | tr -d ' \t\r\n' | sed -n 's/^{"\.":"\([0-9.]*\)"}$/\1/p')
      [[ -n "$manifest" ]] && tag=v$manifest
      [[ -n "$tag" ]] || { next=blocked; reason='merged Release PR has no readable manifest version'; }
    elif (($(jq 'length' <<<"$open") > 0)); then
      next=review_release_pr; reason="human review, approval and squash merge of $(jq -r '.[0].url' <<<"$open")"
    else
      next=none; reason='no Release PR yet; Release Please opens one after a user-facing commit reaches main'
    fi
  fi

  if [[ -n "$tag" && -z "${next:-}" ]]; then
    local facts channel version
    facts=$("$scripts/release-tag-version.sh" "$tag") || fail "invalid tag: $tag"
    version=$(awk -F= '$1 == "version" {print $2}' <<<"$facts")
    channel=$(awk -F= '$1 == "channel" {print $2}' <<<"$facts")
    if [[ -z "$revision" ]]; then
      if [[ "$channel" == stable ]]; then
        revision=$(release_commit_for "$version")
        if [[ -z "$revision" ]]; then
          if jq -e --arg v "$version" 'any(.[]; .title | contains($v))' <<<"$open" >/dev/null; then
            next=review_release_pr; reason="the Release PR for $version must be reviewed and merged first"
          else
            next=blocked; reason="no Release PR prepares $version; use a Release-As: $version commit footer or wait for Release Please"
          fi
        fi
      else
        revision=$main
      fi
    fi
  fi

  if [[ -n "$tag" && -z "${next:-}" ]]; then
    printf 'tag=%s\nrevision=%s\n' "$tag" "$revision" >>"$out"
    if ! "$scripts/release-preflight.sh" --tag "$tag" --revision "$revision" --main-ref origin/main >"$temporary/preflight" 2>"$temporary/preflight-error"; then
      next=blocked; reason=$(sed 's/^release_preflight_error: //' "$temporary/preflight-error" | head -n 1)
    else
      local ci
      ci=$(ci_state "$revision")
      printf 'revision_ci=%s\n' "$ci" >>"$out"
      grep -E '^(channel|prerelease|manifest_version|latest_stable|tag_state|make_latest)=' "$temporary/preflight" >>"$out"
      if ! "$scripts/publish-release.sh" --check --repo "$repository" --tag "$tag" --revision "$revision" \
        --make-latest "$(value make_latest "$temporary/preflight")" >"$temporary/remote" 2>"$temporary/remote-error"; then
        # A refused check still reports the remote state it classified (for
        # example orphan_conflict), so status never shows it as absent.
        grep -E '^(publication_state|release_id|orphan_[a-z_]+|orphan_asset\.[^=]+)=' "$temporary/remote" >>"$out" || true
        next=blocked; reason=$(sed 's/^release_publish_error: //' "$temporary/remote-error" | head -n 1)
      else
        grep -E '^(publication_state|release_id)=' "$temporary/remote" >>"$out"
        if [[ $(value publication_state "$temporary/remote") == published ]]; then
          next=verify_published; reason='already published; run release.sh verify'
        elif [[ "$ci" != success ]]; then
          next=blocked; reason="required CI on $revision is $ci"
        elif [[ $(release_environment) != protected ]]; then
          next=blocked; reason='the release environment is missing or has no required reviewers (docs/security/repository-security.md)'
        elif [[ -z "$prepared_run" ]]; then
          next=prepare
          reason="build and verify the exact set first: release.sh prepare --tag $tag --revision $revision (no publication)"
        elif prepared_envelope; then
          next=authorize_publication
          reason='human authorization required for the exact publication envelope below'
          grep -v '^preview_digest=' "$temporary/envelope" | sed 's/^/preview./' >>"$out"
          grep '^preview_digest=' "$temporary/envelope" >>"$out"
        else
          next=blocked
        fi
      fi
    fi
  fi
  printf 'next_action=%s\n' "$next"
  [[ -n "$reason" ]] && printf 'reason=%s\n' "$reason"
  true
}

case "$command" in
  status)
    status_line=$(status)
    cat "$temporary/status"
    printf '%s\n' "$status_line"
    ;;
  prepare)
    [[ -n "$tag" ]] || fail 'prepare requires --tag'
    [[ -z "$prepared_run" ]] || fail 'prepare starts a new preparation; use status --prepared-run for an existing one'
    status_line=$(status)
    grep -Fxq 'next_action=prepare' <<<"$status_line" \
      || { cat "$temporary/status"; fail "preparation is not the next step: $(tr '\n' ' ' <<<"$status_line")"; }
    revision=$(value revision "$temporary/status")
    dispatched_at=$(date -u +%Y-%m-%dT%H:%M:%SZ)
    gh workflow run "$prepare_workflow" --repo "$repository" --ref main -f "tag=$tag" -f "revision=$revision" >/dev/null \
      || fail 'preparation dispatch failed'
    printf 'effect=workflow_dispatched workflow=%s tag=%s revision=%s publication=none\n' "$prepare_workflow" "$tag" "$revision"
    run=$(find_run "$prepare_workflow" "$dispatched_at")
    [[ -n "$run" ]] || fail 'cannot find the preparation run'
    prepared_run=$(jq -r '.databaseId' <<<"$run")
    printf 'prepare_run_url=%s\n' "$(jq -r '.url' <<<"$run")"
    gh run watch "$prepared_run" --repo "$repository" --exit-status >/dev/null || fail "preparation run $prepared_run failed"
    status_line=$(status)
    cat "$temporary/status"
    printf '%s\n' "$status_line"
    ;;
  publish)
    [[ -n "$tag" && "$revision" =~ ^[0-9a-f]{40}$ ]] || fail 'publish requires --tag and a full --revision'
    [[ "$authorized" == true ]] \
      || fail 'publication requires explicit human authorization (--authorize-publication) for the reviewed envelope; nothing was dispatched'
    [[ "$preview_digest" =~ ^[0-9a-f]{64}$ ]] || fail 'publish requires the --preview-digest of the authorized envelope'
    [[ "$prepared_run" =~ ^[0-9]+$ ]] || fail 'publish requires the --prepared-run of the authorized envelope'
    status_line=$(status)
    grep -Fxq 'next_action=authorize_publication' <<<"$status_line" \
      || fail "publication is not the next step: $(tr '\n' ' ' <<<"$status_line")"
    grep -Fxq "preview_digest=$preview_digest" "$temporary/status" \
      || fail "preview changed; review and authorize again (current $(grep '^preview_digest=' "$temporary/status"))"
    dispatched_at=$(date -u +%Y-%m-%dT%H:%M:%SZ)
    gh workflow run "$workflow" --repo "$repository" --ref main -f "tag=$tag" -f "revision=$revision" \
      -f "prepared_run=$prepared_run" -f "preview_digest=$preview_digest" >/dev/null || fail 'workflow dispatch failed'
    printf 'effect=workflow_dispatched workflow=%s tag=%s revision=%s prepared_run=%s preview_digest=%s\n' \
      "$workflow" "$tag" "$revision" "$prepared_run" "$preview_digest"
    run=$(find_run "$workflow" "$dispatched_at")
    if [[ -n "$run" ]]; then
      printf 'run_id=%s\nrun_url=%s\n' "$(jq -r '.databaseId' <<<"$run")" "$(jq -r '.url' <<<"$run")"
    else
      printf 'run_id=unknown\n'
    fi
    printf 'next_action=human_approves_release_environment_then_watch\n'
    ;;
  verify)
    [[ -n "$tag" ]] || fail 'verify requires --tag'
    git -C "$repository_root" fetch --quiet origin main || fail 'cannot fetch origin main'
    tag_sha=$(git -C "$repository_root" ls-remote --tags origin "refs/tags/$tag^{}" "refs/tags/$tag" | awk 'NR == 1 {print $1}')
    [[ -n "$tag_sha" ]] || fail "tag $tag is not published"
    peeled=$(git -C "$repository_root" ls-remote --tags origin "refs/tags/$tag^{}" | awk '{print $1}')
    [[ -n "$peeled" ]] && tag_sha=$peeled
    "$scripts/release-preflight.sh" --tag "$tag" --revision "$tag_sha" --main-ref origin/main >"$temporary/preflight"
    "$scripts/publish-release.sh" --check --repo "$repository" --tag "$tag" --revision "$tag_sha" \
      --make-latest "$(value make_latest "$temporary/preflight")" >"$temporary/remote"
    [[ $(value publication_state "$temporary/remote") == published ]] || fail "release $tag is not published"
    latest=$(gh api "repos/$repository/releases/latest" 2>/dev/null | jq -r '.tag_name // empty') || latest=
    if [[ $(value make_latest "$temporary/preflight") == true ]]; then
      [[ "$latest" == "$tag" ]] || fail "latest release is '${latest:-none}', expected $tag"
    else
      [[ "$latest" != "$tag" ]] || fail 'release must not be latest'
    fi
    grep -v '^result=' "$temporary/remote"
    printf 'latest=%s\n' "${latest:-none}"
    if [[ "$download" == true ]]; then
      mkdir "$temporary/artifacts"
      gh release download "$tag" --repo "$repository" --dir "$temporary/artifacts" >/dev/null || fail 'cannot download release assets'
      git clone --quiet --no-hardlinks "$repository_root" "$temporary/source"
      git -C "$temporary/source" checkout --quiet --detach "$tag_sha"
      "$temporary/source/scripts/verify-release-artifacts.sh" --dir "$temporary/artifacts" \
        --version "$(value version "$temporary/preflight")" --revision "$tag_sha" >"$temporary/verified"
      grep -Ev '^(result|publication)=' "$temporary/verified"
      printf 'artifacts_verified=pass\n'
    fi
    printf 'result=pass\n'
    ;;
  *)
    fail 'usage: release.sh status|prepare|publish|verify [options]'
    ;;
esac
