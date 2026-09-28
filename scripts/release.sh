#!/usr/bin/env bash
# Maintainer release orchestration used by the $axiom-release skill.
#
#   release.sh status  [--tag vX.Y.Z[-rc.N]] [--revision SHA]
#   release.sh publish --tag TAG --revision SHA --preview-digest DIGEST --authorize-publication
#   release.sh verify  --tag TAG [--download]
#
# status and verify are read-only apart from `git fetch` of main and temporary
# files. publish only dispatches .github/workflows/publish-release.yml, and
# only when --authorize-publication is present and DIGEST equals a freshly
# recomputed publication preview, so authority is bound to the exact reviewed
# tag, revision, channel, latest pointer and remote state. It never creates
# tags or releases itself and never approves the `release` environment.
# Release rules live in release-preflight.sh, publish-release.sh and the
# workflows; this script only gathers facts and chooses the next step.
set -euo pipefail

umask 077

repository_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)
scripts=$repository_root/scripts
workflow=publish-release.yml
command=${1:-}
[[ -n "$command" ]] && shift
tag=
revision=
preview_digest=
authorized=false
download=false
while (($#)); do
  case "$1" in
    --tag) tag=${2:-}; shift 2 ;;
    --revision) revision=${2:-}; shift 2 ;;
    --preview-digest) preview_digest=${2:-}; shift 2 ;;
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
        next=blocked; reason=$(sed 's/^release_publish_error: //' "$temporary/remote-error" | head -n 1)
      else
        grep -E '^(publication_state|release_id)=' "$temporary/remote" >>"$out"
        if [[ $(value publication_state "$temporary/remote") == published ]]; then
          next=verify_published; reason='already published; run release.sh verify'
        elif [[ "$ci" != success ]]; then
          next=blocked; reason="required CI on $revision is $ci"
        elif [[ $(release_environment) != protected ]]; then
          next=blocked; reason='the release environment is missing or has no required reviewers (docs/security/repository-security.md)'
        else
          {
            printf 'repository=%s\n' "$repository"
            printf 'workflow=%s\nref=main\n' "$workflow"
            grep -E '^(tag|version|channel|prerelease|revision|make_latest)=' "$temporary/preflight"
            grep -E '^(publication_state|release_id)=' "$temporary/remote"
          } >"$temporary/preview"
          next=authorize_publication
          reason='human authorization required for the exact preview below'
          sed 's/^/preview./' "$temporary/preview" >>"$out"
          printf 'preview_digest=%s\n' "$(digest_stdin <"$temporary/preview")" >>"$out"
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
  publish)
    [[ -n "$tag" && "$revision" =~ ^[0-9a-f]{40}$ ]] || fail 'publish requires --tag and a full --revision'
    [[ "$authorized" == true ]] \
      || fail 'publication requires explicit human authorization (--authorize-publication) for the reviewed preview; nothing was dispatched'
    [[ "$preview_digest" =~ ^[0-9a-f]{64}$ ]] || fail 'publish requires the --preview-digest shown by status'
    status_line=$(status)
    grep -Fxq 'next_action=authorize_publication' <<<"$status_line" \
      || fail "publication is not the next step: $(tr '\n' ' ' <<<"$status_line")"
    grep -Fxq "preview_digest=$preview_digest" "$temporary/status" \
      || fail 'preview changed since it was authorized; review the new status and authorize again'
    dispatched_at=$(date -u +%Y-%m-%dT%H:%M:%SZ)
    gh workflow run "$workflow" --repo "$repository" --ref main -f "tag=$tag" -f "revision=$revision" >/dev/null \
      || fail 'workflow dispatch failed'
    printf 'effect=workflow_dispatched workflow=%s tag=%s revision=%s\n' "$workflow" "$tag" "$revision"
    run=
    for _ in 1 2 3 4 5 6 7 8 9 10; do
      run=$(gh run list --repo "$repository" --workflow "$workflow" --event workflow_dispatch --limit 10 \
        --json databaseId,url,createdAt | jq -c --arg since "$dispatched_at" '[.[] | select(.createdAt >= $since)] | sort_by(.createdAt) | first // empty')
      [[ -n "$run" ]] && break
      sleep "${AXIOM_RELEASE_RETRY_DELAY:-3}"
    done
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
    fail 'usage: release.sh status|publish|verify [options]'
    ;;
esac
