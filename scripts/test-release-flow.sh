#!/usr/bin/env bash
# Release flow contract tests: release-preflight.sh, release-notes.sh,
# publish-release.sh, the release.sh authority boundary used by the
# $axiom-release skill, and the static no-publication/least-privilege shape of
# the CI, Release PR and publication workflows.
#
# GitHub is a stateful fake `gh` on PATH backed by local JSON files; Git
# history and remote tags come from a local fixture repository and bare
# remote. No network access, tag, release or repository mutation happens.
set -euo pipefail

umask 077

repository_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)
temporary=$(mktemp -d)
temporary=$(cd "$temporary" && pwd -P)
trap 'rm -rf -- "$temporary"' EXIT
export AXIOM_RELEASE_RETRY_DELAY=0

digest() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  else
    shasum -a 256 "$1" | awk '{print $1}'
  fi
}

failures=0
check() {
  local label=$1
  shift
  if "$@" >/dev/null; then
    printf 'ok: %s\n' "$label"
  else
    printf 'FAIL: %s\n' "$label" >&2
    failures=$((failures + 1))
  fi
}

# expect_failure LABEL PATTERN COMMAND... requires a non-zero exit whose
# stderr contains PATTERN.
expect_failure() {
  local label=$1 pattern=$2
  shift 2
  if "$@" >"$temporary/out" 2>"$temporary/err"; then
    printf 'FAIL: %s unexpectedly succeeded\n' "$label" >&2
    failures=$((failures + 1))
    return
  fi
  if grep -Fq -- "$pattern" "$temporary/err"; then
    printf 'ok: %s\n' "$label"
  else
    printf 'FAIL: %s did not report "%s":\n' "$label" "$pattern" >&2
    cat "$temporary/err" >&2
    failures=$((failures + 1))
  fi
}

# --- Fake GitHub --------------------------------------------------------------
tools=$temporary/tools
state=$temporary/github
mkdir -p "$tools"
export FAKE_GH_STATE=$state
export PATH="$tools:$PATH"
cat >"$tools/gh" <<'EOF'
#!/usr/bin/env bash
# Stateful fake of the gh subset used by the release scripts.
set -euo pipefail
s=$FAKE_GH_STATE
repo=rgomids/axiom
log() { printf '%s\n' "$*" >>"$s/ledger"; }
not_found() { printf 'gh: Not Found (HTTP 404)\n' >&2; exit 1; }
maybe_fail() {
  if [[ -n "${FAKE_GH_FAIL_ON:-}" && "$*" == *"$FAKE_GH_FAIL_ON"* ]]; then
    printf 'gh: injected failure (HTTP 502)\n' >&2
    exit 1
  fi
}
digest() { if command -v sha256sum >/dev/null 2>&1; then sha256sum "$1" | awk '{print $1}'; else shasum -a 256 "$1" | awk '{print $1}'; fi; }
release_file() { printf '%s/releases/%s.json' "$s" "$1"; }
all_releases() {
  if compgen -G "$s/releases/*.json" >/dev/null; then jq -s 'sort_by(.id) | reverse' "$s"/releases/*.json; else printf '[]'; fi
}
tag_sha() { awk -v t="$1" '$1 == t {print $2}' "$s/tags" 2>/dev/null; }

case "${1:-}" in
  repo) printf '%s\n' "$repo"; exit 0 ;;
  pr)
    shift 2
    kind=open
    while (($#)); do case "$1" in --state) kind=$2; shift 2 ;; *) shift ;; esac; done
    cat "$s/pr-$kind.json" 2>/dev/null || printf '[]\n'
    exit 0 ;;
  workflow)
    log "workflow ${*:2}"
    exit 0 ;;
  run)
    printf '[{"databaseId":4242,"url":"https://github.com/%s/actions/runs/4242","createdAt":"%s"}]\n' "$repo" "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
    exit 0 ;;
  release)
    # gh release download TAG --repo R --dir DIR
    tag=$3 dir=
    shift 3
    while (($#)); do case "$1" in --dir) dir=$2; shift 2 ;; *) shift ;; esac; done
    id=$(all_releases | jq -r --arg t "$tag" '.[] | select(.tag_name == $t and .draft == false) | .id')
    [[ -n "$id" ]] || not_found
    jq -r '.assets[] | "\(.id) \(.name)"' "$(release_file "$id")" | while read -r aid name; do cp "$s/assets/$aid" "$dir/$name"; done
    exit 0 ;;
  api) shift ;;
  *) printf 'fake gh: unsupported command %s\n' "$*" >&2; exit 2 ;;
esac

method=GET input= endpoint= fields=()
while (($#)); do
  case "$1" in
    --method|-X) method=$2; shift 2 ;;
    --input) input=$2; shift 2 ;;
    -H|--jq) shift 2 ;;
    -f|-F) fields+=("$2"); shift 2 ;;
    --paginate) shift ;;
    *) endpoint=$1; shift ;;
  esac
done
maybe_fail "$method $endpoint"
path=${endpoint#https://uploads.github.com/}
path=${path#repos/$repo/}

case "$method $path" in
  "GET releases?per_page=100") all_releases ;;
  "GET releases/latest")
    [[ -s "$s/latest" ]] || not_found
    all_releases | jq --arg t "$(cat "$s/latest")" '.[] | select(.tag_name == $t)' ;;
  "GET releases/assets/"*)
    id=${path#releases/assets/}
    [[ -f "$s/assets/$id" ]] || not_found
    cat "$s/assets/$id" ;;
  "DELETE releases/assets/"*)
    id=${path#releases/assets/}
    log "DELETE asset $id"
    for f in "$s"/releases/*.json; do
      jq --argjson id "$id" '.assets |= map(select(.id != $id))' "$f" >"$f.new" && mv "$f.new" "$f"
    done
    rm -f "$s/assets/$id" ;;
  "GET releases/"*)
    id=${path#releases/}
    [[ -f $(release_file "$id") ]] || not_found
    cat "$(release_file "$id")" ;;
  "POST releases")
    id=$(( $(cat "$s/next-id" 2>/dev/null || echo 100) + 1 ))
    echo "$id" >"$s/next-id"
    log "POST release $(jq -r .tag_name "$input") draft=$(jq -r .draft "$input") prerelease=$(jq -r .prerelease "$input")"
    jq --argjson id "$id" --arg repo "$repo" '. + {id: $id, assets: [],
      upload_url: "https://uploads.github.com/repos/\($repo)/releases/\($id)/assets{?name,label}",
      html_url: "https://github.com/\($repo)/releases/tag/\(.tag_name)", immutable: false}' "$input" >"$(release_file "$id")"
    cat "$(release_file "$id")" ;;
  "PATCH releases/"*)
    id=${path#releases/}
    f=$(release_file "$id")
    [[ -f "$f" ]] || not_found
    log "PATCH release $id $(jq -c 'del(.body)' "$input")"
    jq -s '.[0] * (.[1] | del(.make_latest))' "$f" "$input" >"$f.new" && mv "$f.new" "$f"
    if [[ $(jq -r .draft "$f") == false ]]; then
      t=$(jq -r .tag_name "$f")
      [[ -n $(tag_sha "$t") ]] || printf '%s %s\n' "$t" "$(jq -r .target_commitish "$f")" >>"$s/tags"
      [[ $(jq -r '.make_latest // empty' "$input") == true ]] && printf '%s' "$t" >"$s/latest"
      [[ "${FAKE_GH_IMMUTABLE:-}" == 1 ]] && { jq '.immutable = true' "$f" >"$f.new" && mv "$f.new" "$f"; }
    fi
    cat "$f" ;;
  "POST releases/"*"/assets?name="*)
    rest=${path#releases/}
    id=${rest%%/*}
    name=${path##*name=}
    f=$(release_file "$id")
    aid=$(( $(cat "$s/next-asset" 2>/dev/null || echo 500) + 1 ))
    echo "$aid" >"$s/next-asset"
    mkdir -p "$s/assets"
    cp "$input" "$s/assets/$aid"
    log "UPLOAD $name"
    jq --argjson aid "$aid" --arg name "$name" --argjson size "$(wc -c <"$input" | tr -d ' ')" --arg d "sha256:$(digest "$input")" \
      '.assets += [{id: $aid, name: $name, size: $size, state: "uploaded", digest: $d}]' "$f" >"$f.new" && mv "$f.new" "$f"
    printf '{"id":%s}\n' "$aid" ;;
  "GET git/matching-refs/tags/"*)
    prefix=${path#git/matching-refs/tags/}
    awk -v p="$prefix" 'index($1, p) == 1 {printf "%s{\"ref\":\"refs/tags/%s\",\"object\":{\"type\":\"commit\",\"sha\":\"%s\"}}", (n++ ? "," : ""), $1, $2} BEGIN {printf "["} END {print "]"}' "$s/tags" 2>/dev/null || printf '[]\n' ;;
  "GET commits/"*"/check-runs?per_page=100")
    sha=${path#commits/}
    sha=${sha%%/*}
    cat "$s/checks-$sha.json" 2>/dev/null || printf '{"check_runs":[]}\n' ;;
  "GET commits/"*"/pulls") cat "$s/pulls.json" 2>/dev/null || printf '[]\n' ;;
  "POST issues/"*"/labels")
    number=${path#issues/}
    number=${number%%/*}
    log "LABEL add $number ${fields[*]}"
    jq --argjson n "$number" '(.[] | select(.number == $n) | .labels) += [{name: "autorelease: tagged"}]' "$s/pulls.json" >"$s/pulls.new" && mv "$s/pulls.new" "$s/pulls.json" ;;
  "DELETE issues/"*"/labels/"*)
    number=${path#issues/}
    number=${number%%/*}
    log "LABEL remove $number ${path##*/}"
    jq --argjson n "$number" '(.[] | select(.number == $n) | .labels) |= map(select(.name != "autorelease: pending"))' "$s/pulls.json" >"$s/pulls.new" && mv "$s/pulls.new" "$s/pulls.json" ;;
  "GET environments/release")
    [[ -f "$s/environment.json" ]] || not_found
    cat "$s/environment.json" ;;
  *) printf 'fake gh: unsupported api %s %s\n' "$method" "$path" >&2; exit 2 ;;
esac
EOF
chmod 700 "$tools/gh"

reset_github() {
  rm -rf -- "$state"
  mkdir -p "$state/releases" "$state/assets"
  : >"$state/ledger"
  : >"$state/tags"
}
mutations() {
  grep -Ec '^(POST|PATCH|DELETE|UPLOAD|LABEL|workflow)' "$state/ledger" || true
}

# --- Fixture repository -----------------------------------------------------------
fixture=$temporary/fixture
remote=$temporary/remote.git
git init -q --bare "$remote"
git init -q -b main "$fixture"
git -C "$fixture" config user.email release-test@example.invalid
git -C "$fixture" config user.name 'Release Test'
git -C "$fixture" config commit.gpgsign false
mkdir -p "$fixture/scripts" "$fixture/.github/rulesets"
for script in release-tag-version.sh release-preflight.sh release-notes.sh publish-release.sh release.sh; do
  cp "$repository_root/scripts/$script" "$fixture/scripts/$script"
done
cp "$repository_root/.github/rulesets/main.json" "$fixture/.github/rulesets/main.json"
printf '# Changelog\n\n## [2026-09-28]\n\n- curated history\n' >"$fixture/CHANGELOG.md"
commit() { git -C "$fixture" add -A && git -C "$fixture" commit -q -m "$1" && git -C "$fixture" rev-parse HEAD; }
c0=$(commit 'chore: base')
printf '{\n  ".": "0.0.0"\n}\n' >"$fixture/.release-please-manifest.json"
c1=$(commit 'ci(release): adopt release please')
printf 'feature\n' >"$fixture/feature.txt"
c2=$(commit 'feat(cli): add feature')
printf '{\n  ".": "0.1.0"\n}\n' >"$fixture/.release-please-manifest.json"
printf '# Changelog\n\n## [0.1.0](https://github.com/rgomids/axiom/compare/v0.0.0...v0.1.0) (2026-10-01)\n\n\n### Features\n\n* **cli:** add feature\n\n## [2026-09-28]\n\n- curated history\n' >"$fixture/CHANGELOG.md"
c3=$(commit 'chore(main): release 0.1.0')
printf 'fix\n' >"$fixture/fix.txt"
c4=$(commit 'fix(cli): repair feature')
git -C "$fixture" checkout -q -b side "$c2"
printf 'side\n' >"$fixture/side.txt"
side=$(commit 'feat: side branch')
git -C "$fixture" checkout -q main
git -C "$fixture" remote add origin "$remote"
git -C "$fixture" push -q origin main
git -C "$fixture" fetch -q origin

preflight() { "$fixture/scripts/release-preflight.sh" --main-ref origin/main "$@"; }
remote_tag() { git -C "$fixture" push -q -f origin "$2:refs/tags/$1"; }
remote_untag() { git -C "$fixture" push -q origin ":refs/tags/$1"; }

# --- 1. Preflight: SemVer, revision and Release PR binding ---------------------------
preflight --tag v0.1.0 --revision "$c3" >"$temporary/pf"
check 'stable at release commit passes' grep -Fxq 'result=pass' "$temporary/pf"
check 'stable is latest and not prerelease' bash -c "grep -Fxq make_latest=true '$temporary/pf' && grep -Fxq prerelease=false '$temporary/pf' && grep -Fxq release_commit=$c3 '$temporary/pf'"
preflight --tag v0.1.0-rc.1 --revision "$c2" >"$temporary/pf"
check 'RC on main passes as prerelease, never latest' bash -c "grep -Fxq prerelease=true '$temporary/pf' && grep -Fxq make_latest=false '$temporary/pf' && grep -Fxq channel=rc '$temporary/pf'"
check 'RC at the release commit passes' preflight --tag v0.1.0-rc.1 --revision "$c3"
for invalid in 0.1.0 v0.1 v01.0.0 v0.1.0-beta.1 v0.1.0-rc.01 v0.1.0-poc.1 'v0.1.0;id'; do
  expect_failure "invalid tag $invalid" 'release_tag_error' preflight --tag "$invalid" --revision "$c3"
done
expect_failure 'stable before its Release PR' 'merge the Release PR first' preflight --tag v0.1.0 --revision "$c2"
expect_failure 'stable after the release commit' 'not the release commit' preflight --tag v0.1.0 --revision "$c4"
expect_failure 'stable version not recorded' 'does not record version 0.2.0' preflight --tag v0.2.0 --revision "$c3"
expect_failure 'short revision' 'full source revision required' preflight --tag v0.1.0 --revision "${c3:0:12}"
expect_failure 'unknown revision' 'not a known commit' preflight --tag v0.1.0 --revision "$(printf 'a%.0s' {1..40})"
expect_failure 'revision before the release flow' 'predates the release flow' preflight --tag v0.1.0-rc.1 --revision "$c0"
expect_failure 'revision off main' 'not on the first-parent history of main' preflight --tag v0.1.0-rc.1 --revision "$side"
expect_failure 'RC older than recorded version' 'already records a newer version 0.1.0' preflight --tag v0.0.9-rc.1 --revision "$c3"
remote_tag v0.1.0-rc.1 "$c1"
preflight --tag v0.1.0-rc.1 --revision "$c1" >"$temporary/pf"
check 'rerun with existing tag at the same revision' grep -Fxq 'tag_state=present' "$temporary/pf"
expect_failure 'existing tag at another revision' 'tag already exists at another revision' preflight --tag v0.1.0-rc.1 --revision "$c2"
check 'next RC number passes' preflight --tag v0.1.0-rc.2 --revision "$c2"
remote_tag v0.1.0-rc.3 "$c2"
expect_failure 'RC number not increasing' 'greater than existing rc.3' preflight --tag v0.1.0-rc.2 --revision "$c2"
remote_tag v0.1.0-poc.1 "$c0"
check 'non-policy tags are ignored' preflight --tag v0.1.0-rc.4 --revision "$c2"
remote_tag v0.1.0 "$c3"
preflight --tag v0.1.0 --revision "$c3" >"$temporary/pf"
check 'stable rerun stays latest' bash -c "grep -Fxq tag_state=present '$temporary/pf' && grep -Fxq make_latest=true '$temporary/pf'"
expect_failure 'RC after its stable' 'stable v0.1.0 already exists' preflight --tag v0.1.0-rc.5 --revision "$c3"
expect_failure 'version not newer than stable' 'not newer than the latest stable release v0.1.0' preflight --tag v0.0.9-rc.1 --revision "$c1"
for name in v0.1.0 v0.1.0-rc.1 v0.1.0-rc.3 v0.1.0-poc.1; do remote_untag "$name"; done
printf '{".": "0.1"}\n' >"$fixture/.release-please-manifest.json"
malformed=$(commit 'chore: malformed manifest')
git -C "$fixture" push -q origin main && git -C "$fixture" fetch -q origin
expect_failure 'malformed manifest' 'release manifest is malformed' preflight --tag v0.2.0-rc.1 --revision "$malformed"
git -C "$fixture" reset -q --hard "$c4"
git -C "$fixture" push -q -f origin main && git -C "$fixture" fetch -q origin

# --- 2. Deterministic release notes ------------------------------------------------------
notes() { "$fixture/scripts/release-notes.sh" --repo rgomids/axiom "$@"; }
notes --tag v0.1.0 --revision "$c3" >"$temporary/notes-stable"
check 'stable notes use the Release Please section only' bash -c "grep -Fq '* **cli:** add feature' '$temporary/notes-stable' && ! grep -Fq 'curated history' '$temporary/notes-stable'"
check 'stable notes bind revision and exact install' bash -c "grep -Fq '$c3' '$temporary/notes-stable' && grep -Fq -- '--version v0.1.0' '$temporary/notes-stable'"
notes --tag v0.1.0-rc.1 --revision "$c2" >"$temporary/notes-rc"
check 'RC notes state prerelease and exact-tag install' bash -c "grep -Fq 'prerelease' '$temporary/notes-rc' && grep -Fq -- '--version v0.1.0-rc.1' '$temporary/notes-rc'"
cmp -s "$temporary/notes-rc" <(notes --tag v0.1.0-rc.1 --revision "$c2") && check 'notes are deterministic' true || check 'notes are deterministic' false
expect_failure 'stable notes without a changelog section' 'no Release Please section' notes --tag v0.1.0 --revision "$c2"

# --- 3. Publication state machine ------------------------------------------------------------
# make_set DIR VERSION REVISION SALT writes a synthetic artifact set and the
# verification Evidence publish-release.sh requires.
make_set() {
  local dir=$1 version=$2 rev=$3 salt=$4 row
  rm -rf -- "$dir"
  mkdir -p "$dir/artifacts"
  : >"$dir/sums"
  {
    printf 'evidenceVersion=1\nproduct=Axiom\nversion=%s\nrevision=%s\n' "$version" "$rev"
    for row in macos-27-arm64 ubuntu-26.04-amd64 ubuntu-26.04-arm64; do
      printf '%s %s %s\n' "$version" "$row" "$salt" >"$dir/artifacts/axiom-$version-$row.tar.gz"
      printf '%s  %s\n' "$(digest "$dir/artifacts/axiom-$version-$row.tar.gz")" "axiom-$version-$row.tar.gz" >>"$dir/sums"
    done
  } >"$dir/evidence.head"
  cp "$dir/sums" "$dir/artifacts/SHA256SUMS"
  {
    cat "$dir/evidence.head"
    printf 'sha256sums=%s\n' "$(digest "$dir/artifacts/SHA256SUMS")"
    awk '{printf "archive=%s sha256=%s axiom_sha256=x manifest_sha256=x version_smoke=not_host_architecture\n", $2, $1}' "$dir/sums"
    printf 'publication=none\nresult=pass\n'
  } >"$dir/evidence.txt"
  printf 'notes for %s\n' "$version" >"$dir/notes.md"
}
publish() {
  local dir=$1
  shift
  "$fixture/scripts/publish-release.sh" --repo rgomids/axiom --dir "$dir/artifacts" --evidence "$dir/evidence.txt" --notes "$dir/notes.md" "$@"
}
release_json() { jq -s --arg t "$1" '[.[] | select(.tag_name == $t)] | .[0]' "$state"/releases/*.json; }

reset_github
make_set "$temporary/rc" 0.1.0-rc.1 "$c2" first
"$fixture/scripts/publish-release.sh" --check --repo rgomids/axiom --tag v0.1.0-rc.1 --revision "$c2" --make-latest false >"$temporary/check"
check 'check reports absent with zero effects' bash -c "grep -Fxq publication_state=absent '$temporary/check' && [[ \$(grep -Ec '^(POST|PATCH|DELETE|UPLOAD)' '$state/ledger' || true) == 0 ]]"
publish "$temporary/rc" --tag v0.1.0-rc.1 --revision "$c2" --make-latest false >"$temporary/pub"
check 'RC publishes as prerelease' bash -c "grep -Fxq publication=published '$temporary/pub' && [[ \$(jq -s '.[0] | .draft == false and .prerelease == true' $state/releases/*.json) == true ]]"
check 'RC is not latest' bash -c "[[ ! -s '$state/latest' ]] && grep -Fxq latest=none '$temporary/pub'"
check 'tag created only at publication, at the revision' grep -Fxq "v0.1.0-rc.1 $c2" "$state/tags"
check 'draft created before any upload, published last' bash -c "head -n 1 '$state/ledger' | grep -q '^POST release v0.1.0-rc.1 draft=true prerelease=true' && grep -E '^(PATCH|UPLOAD)' '$state/ledger' | tail -n 1 | grep -q '\"draft\":false'"
check 'exactly four assets uploaded' bash -c "[[ \$(grep -c '^UPLOAD' '$state/ledger') == 4 ]]"
check 'RC does not touch Release PR labels' grep -Fxq 'release_pr=not_applicable' "$temporary/pub"

before=$(mutations)
make_set "$temporary/rc-rebuild" 0.1.0-rc.1 "$c2" rebuilt
publish "$temporary/rc-rebuild" --tag v0.1.0-rc.1 --revision "$c2" --make-latest false >"$temporary/pub"
check 'rerun after publication converges without effects' bash -c "grep -Fxq publication=already_published '$temporary/pub' && [[ $(mutations) == $before ]]"
check 'no duplicate release after rerun' bash -c "[[ \$(jq -s '[.[] | select(.tag_name == \"v0.1.0-rc.1\")] | length' $state/releases/*.json) == 1 ]]"

asset=$(release_json v0.1.0-rc.1 | jq -r '.assets[] | select(.name | endswith("amd64.tar.gz")) | .id')
printf 'tampered\n' >>"$state/assets/$asset"
f=$(grep -l '"v0.1.0-rc.1"' "$state"/releases/*.json)
jq --argjson id "$asset" '(.assets[] | select(.id == $id) | .digest) = null' "$f" >"$f.new" && mv "$f.new" "$f"
before=$(mutations)
expect_failure 'inconsistent published release is a conflict' 'published asset differs from SHA256SUMS' \
  publish "$temporary/rc" --tag v0.1.0-rc.1 --revision "$c2" --make-latest false
check 'published release is never modified' bash -c "[[ $(mutations) == $before ]]"

expect_failure 'RC can never be latest' 'never latest' publish "$temporary/rc" --tag v0.1.0-rc.2 --revision "$c2" --make-latest true
expect_failure 'invalid tag before any effect' 'release_tag_error' publish "$temporary/rc" --tag 0.1.0 --revision "$c2" --make-latest false

# Local set: incomplete, checksum and revision mismatches fail before GitHub.
reset_github
make_set "$temporary/bad" 0.1.0-rc.2 "$c2" x
rm "$temporary/bad/artifacts/axiom-0.1.0-rc.2-ubuntu-26.04-arm64.tar.gz"
expect_failure 'incomplete artifact set' 'not exactly the verified set' publish "$temporary/bad" --tag v0.1.0-rc.2 --revision "$c2" --make-latest false
make_set "$temporary/bad" 0.1.0-rc.2 "$c2" x
printf 'x\n' >>"$temporary/bad/artifacts/axiom-0.1.0-rc.2-macos-27-arm64.tar.gz"
expect_failure 'invalid checksum' 'checksum mismatch' publish "$temporary/bad" --tag v0.1.0-rc.2 --revision "$c2" --make-latest false
make_set "$temporary/bad" 0.1.0-rc.2 "$c2" x
printf '0000  extra\n' >>"$temporary/bad/artifacts/SHA256SUMS"
expect_failure 'SHA256SUMS not the verified one' 'SHA256SUMS differs from verified evidence' publish "$temporary/bad" --tag v0.1.0-rc.2 --revision "$c2" --make-latest false
make_set "$temporary/bad" 0.1.0-rc.2 "$c2" x
expect_failure 'evidence for another revision' 'evidence does not name this revision' publish "$temporary/bad" --tag v0.1.0-rc.2 --revision "$c3" --make-latest false
make_set "$temporary/bad" 0.1.0-rc.2 "$c2" x
sed -i.bak 's/^result=pass$/result=fail/' "$temporary/bad/evidence.txt"
expect_failure 'failed verification evidence' 'not a passing verification' publish "$temporary/bad" --tag v0.1.0-rc.2 --revision "$c2" --make-latest false
check 'local refusals made no GitHub effect' bash -c "[[ $(mutations) == 0 ]]"

# Remote conflicts fail closed before any effect.
make_set "$temporary/rc2" 0.1.0-rc.2 "$c2" x
printf 'v0.1.0-rc.2 %s\n' "$c1" >"$state/tags"
expect_failure 'tag at another revision' 'exists at another revision' publish "$temporary/rc2" --tag v0.1.0-rc.2 --revision "$c2" --make-latest false
: >"$state/tags"
for id in 1 2; do
  printf '{"id":%s,"tag_name":"v0.1.0-rc.2","target_commitish":"%s","draft":true,"prerelease":true,"assets":[]}\n' "$id" "$c2" >"$state/releases/$id.json"
done
expect_failure 'duplicate releases' 'more than one release uses v0.1.0-rc.2' publish "$temporary/rc2" --tag v0.1.0-rc.2 --revision "$c2" --make-latest false
rm "$state/releases/2.json"
jq '.target_commitish = "main"' "$state/releases/1.json" >"$state/r" && mv "$state/r" "$state/releases/1.json"
expect_failure 'draft for another revision' 'draft release targets another revision' publish "$temporary/rc2" --tag v0.1.0-rc.2 --revision "$c2" --make-latest false
jq --arg c "$c2" '.target_commitish = $c | .prerelease = false' "$state/releases/1.json" >"$state/r" && mv "$state/r" "$state/releases/1.json"
expect_failure 'draft with the wrong channel' 'wrong prerelease flag' publish "$temporary/rc2" --tag v0.1.0-rc.2 --revision "$c2" --make-latest false
jq '.prerelease = true | .assets = [{"id":9,"name":"notes.txt","state":"uploaded","size":1,"digest":null}]' "$state/releases/1.json" >"$state/r" && mv "$state/r" "$state/releases/1.json"
expect_failure 'draft with a foreign asset' 'asset outside the verified set' publish "$temporary/rc2" --tag v0.1.0-rc.2 --revision "$c2" --make-latest false
check 'remote refusals made no GitHub effect' bash -c "[[ $(mutations) == 0 ]]"

# Stable: interrupted upload leaves an unpublished draft; the rerun with a
# rebuilt set reconciles it and publishes once as latest.
reset_github
printf '[{"number":7,"merged_at":"2026-10-01T00:00:00Z","labels":[{"name":"autorelease: pending"}]}]\n' >"$state/pulls.json"
make_set "$temporary/stable" 0.1.0 "$c3" first
FAKE_GH_FAIL_ON='assets?name=axiom-0.1.0-ubuntu-26.04-amd64' expect_failure 'interrupted upload' 'draft left for a rerun' \
  publish "$temporary/stable" --tag v0.1.0 --revision "$c3" --make-latest true
check 'partial state is an unpublished draft without tag' bash -c "[[ \$(jq -s '.[0].draft' $state/releases/*.json) == true && ! -s '$state/tags' && ! -s '$state/latest' ]]"
"$fixture/scripts/publish-release.sh" --check --repo rgomids/axiom --tag v0.1.0 --revision "$c3" --make-latest true >"$temporary/check"
check 'check reports the partial draft' grep -Fxq publication_state=draft "$temporary/check"
make_set "$temporary/stable-rebuild" 0.1.0 "$c3" rebuilt
cp "$temporary/stable/artifacts/axiom-0.1.0-macos-27-arm64.tar.gz" "$temporary/stable-rebuild/artifacts/"
( cd "$temporary/stable-rebuild/artifacts" && for f in axiom-*.tar.gz; do printf '%s  %s\n' "$(digest "$f")" "$f"; done ) >"$temporary/stable-rebuild/sums"
cp "$temporary/stable-rebuild/sums" "$temporary/stable-rebuild/artifacts/SHA256SUMS"
{
  cat "$temporary/stable-rebuild/evidence.head"
  printf 'sha256sums=%s\n' "$(digest "$temporary/stable-rebuild/artifacts/SHA256SUMS")"
  awk '{printf "archive=%s sha256=%s\n", $2, $1}' "$temporary/stable-rebuild/sums"
  printf 'publication=none\nresult=pass\n'
} >"$temporary/stable-rebuild/evidence.txt"
: >"$state/ledger"
FAKE_GH_IMMUTABLE=1 publish "$temporary/stable-rebuild" --tag v0.1.0 --revision "$c3" --make-latest true >"$temporary/pub"
check 'rerun keeps identical draft assets' grep -Fxq 'draft_asset_kept=axiom-0.1.0-macos-27-arm64.tar.gz' "$temporary/pub"
check 'rerun replaces mismatched draft assets' grep -Fq 'effect=draft_asset_deleted name=SHA256SUMS' "$temporary/pub"
check 'stable published once, not prerelease, latest' bash -c "grep -Fxq publication=published '$temporary/pub' && grep -Fxq latest=v0.1.0 '$temporary/pub' && [[ \$(jq -s 'length == 1 and (.[0] | .draft == false and .prerelease == false)' $state/releases/*.json) == true ]]"
expected_digests=$(for f in "$temporary/stable-rebuild/artifacts"/*; do printf 'sha256:%s\n' "$(digest "$f")"; done | LC_ALL=C sort | paste -sd, -)
published_digests=$(jq -r '.assets[].digest' "$state"/releases/*.json | LC_ALL=C sort | paste -sd, -)
check 'published assets are exactly the rebuilt set' test "$expected_digests" == "$published_digests"
check 'immutable release reported' grep -Fxq immutable=true "$temporary/pub"
check 'Release PR handed to tagged' bash -c "grep -Fxq release_pr=7 '$temporary/pub' && [[ \$(jq -r '[.[0].labels[].name] | join(\",\")' '$state/pulls.json') == 'autorelease: tagged' ]]"
before=$(mutations)
publish "$temporary/stable-rebuild" --tag v0.1.0 --revision "$c3" --make-latest true >"$temporary/pub"
check 'stable rerun converges without effects' bash -c "grep -Fxq publication=already_published '$temporary/pub' && [[ $(mutations) == $before ]]"
rm "$state/latest"
expect_failure 'published stable that is not latest is reported' 'expected v0.1.0' publish "$temporary/stable-rebuild" --tag v0.1.0 --revision "$c3" --make-latest true

# --- 4. release.sh: next step and authority boundary ---------------------------------------------
release() { (cd "$fixture" && "$fixture/scripts/release.sh" "$@"); }
green() {
  printf '{"check_runs":[{"name":"verify (linux)","status":"completed","conclusion":"success","started_at":"1"},{"name":"verify (macos)","status":"completed","conclusion":"success","started_at":"1"},{"name":"release-contract","status":"completed","conclusion":"success","started_at":"1"}]}\n' >"$state/checks-$1.json"
}
reset_github
printf '{"protection_rules":[{"type":"required_reviewers"}]}\n' >"$state/environment.json"
green "$c2"
green "$c3"
green "$c4"

release status >"$temporary/status"
check 'no Release PR: nothing to publish' grep -Fxq next_action=none "$temporary/status"
printf '[{"number":8,"url":"https://github.com/rgomids/axiom/pull/8","title":"chore(main): release 0.1.0"}]\n' >"$state/pr-open.json"
release status >"$temporary/status"
check 'open Release PR needs human review and merge' grep -Fxq next_action=review_release_pr "$temporary/status"
rm "$state/pr-open.json"
printf '[{"number":8,"url":"https://github.com/rgomids/axiom/pull/8","title":"chore(main): release 0.1.0","mergeCommit":{"oid":"%s"}}]\n' "$c3" >"$state/pr-merged.json"
release status >"$temporary/status"
check 'merged Release PR resolves the stable tag and release commit' bash -c "grep -Fxq tag=v0.1.0 '$temporary/status' && grep -Fxq revision=$c3 '$temporary/status' && grep -Fxq next_action=authorize_publication '$temporary/status'"
check 'status reports repository, CI and environment facts' bash -c "grep -Fxq main_ci=success '$temporary/status' && grep -Fxq release_environment=protected '$temporary/status' && grep -Fxq worktree=clean '$temporary/status'"
rm "$state/pr-merged.json"

release status --tag v0.1.0-rc.1 >"$temporary/status"
check 'RC status previews publication of main' bash -c "grep -Fxq next_action=authorize_publication '$temporary/status' && grep -Fxq revision=$c4 '$temporary/status'"
digest_value=$(awk -F= '$1 == "preview_digest" {print $2}' "$temporary/status")
: >"$state/ledger"
expect_failure 'publish without authorization is refused' 'requires explicit human authorization' \
  release publish --tag v0.1.0-rc.1 --revision "$c4" --preview-digest "$digest_value"
expect_failure 'publish without the preview digest is refused' 'preview-digest' \
  release publish --tag v0.1.0-rc.1 --revision "$c4" --authorize-publication
expect_failure 'publish with a stale preview is refused' 'preview changed' \
  release publish --tag v0.1.0-rc.1 --revision "$c4" --preview-digest "$(printf '0%.0s' {1..64})" --authorize-publication
rm "$state/environment.json"
expect_failure 'unprotected release environment blocks publication' 'publication is not the next step' \
  release publish --tag v0.1.0-rc.1 --revision "$c4" --preview-digest "$digest_value" --authorize-publication
printf '{"protection_rules":[{"type":"required_reviewers"}]}\n' >"$state/environment.json"
rm "$state/checks-$c4.json"
release status --tag v0.1.0-rc.1 >"$temporary/status"
check 'missing CI blocks publication' bash -c "grep -Fxq next_action=blocked '$temporary/status' && grep -Fq 'required CI' '$temporary/status'"
green "$c4"
check 'refusals dispatched nothing' bash -c "[[ \$(grep -c '^workflow' '$state/ledger' || true) == 0 ]]"
release publish --tag v0.1.0-rc.1 --revision "$c4" --preview-digest "$digest_value" --authorize-publication >"$temporary/dispatch"
check 'authorized exact preview dispatches the publish workflow once' bash -c "[[ \$(grep -c '^workflow run publish-release.yml' '$state/ledger') == 1 ]] && grep -Fq 'tag=v0.1.0-rc.1' '$state/ledger' && grep -Fq 'revision=$c4' '$state/ledger' && grep -Fxq run_id=4242 '$temporary/dispatch'"
release status --tag v0.2.0 >"$temporary/status"
check 'stable without a Release PR is blocked' bash -c "grep -Fxq next_action=blocked '$temporary/status' && grep -Fq 'no Release PR prepares 0.2.0' '$temporary/status'"
release status --tag v0.1.0-rc.1 --revision "$side" >"$temporary/status"
check 'preflight refusal is reported as blocked' grep -Fq 'reason=revision is not on the first-parent history of main' "$temporary/status"

# Published: verify reports the release instead of publishing again.
make_set "$temporary/rc" 0.1.0-rc.1 "$c4" first
publish "$temporary/rc" --tag v0.1.0-rc.1 --revision "$c4" --make-latest false >/dev/null
remote_tag v0.1.0-rc.1 "$c4"
release status --tag v0.1.0-rc.1 >"$temporary/status"
check 'published release routes to verification' grep -Fxq next_action=verify_published "$temporary/status"
release verify --tag v0.1.0-rc.1 >"$temporary/verify"
check 'verify confirms the published prerelease' bash -c "grep -Fxq publication_state=published '$temporary/verify' && grep -Fxq latest=none '$temporary/verify' && grep -Fxq result=pass '$temporary/verify'"
remote_untag v0.1.0-rc.1
printf 'dirty\n' >"$fixture/dirty.txt"
release status >"$temporary/status"
check 'dirty worktree is reported' grep -Fxq worktree=dirty "$temporary/status"
rm "$fixture/dirty.txt"

# --- 5. Workflow and skill static contracts ------------------------------------------------------
workflows=$repository_root/.github/workflows
triggers() { sed -n '/^on:/,/^[a-z]/p' "$1" | grep -E '^  [a-z_]+:' | tr -d ' :' | LC_ALL=C sort | paste -sd, -; }
check 'CI runs on pull requests, pushes to main and dispatch only' test "$(triggers "$workflows/ci.yml")" == pull_request,push,workflow_dispatch
check 'CI push trigger is main only' bash -c "sed -n '/^  push:/,/^  [a-z]/p' '$workflows/ci.yml' | grep -Fxq '      - main'"
check 'CI has a read-only token and no publication path' bash -c "grep -A1 '^permissions:' '$workflows/ci.yml' | tail -n 1 | grep -Fxq '  contents: read' && ! grep -Eiq 'contents: write|gh release|git tag|git push|publish-release|secrets\\.|id-token' '$workflows/ci.yml'"
check 'Release PR workflow never creates tags or releases' bash -c "grep -Fxq '          skip-github-release: true' '$workflows/release-please.yml' && [[ \$(jq -r '.\"skip-github-release\"' '$repository_root/release-please-config.json') == true ]] && ! grep -Eiq 'gh release|git tag|git push|softprops|secrets\\.' '$workflows/release-please.yml'"
check 'Release PR workflow runs on main pushes and dispatch only' test "$(triggers "$workflows/release-please.yml")" == push,workflow_dispatch
check 'publication runs only on explicit dispatch' test "$(triggers "$workflows/publish-release.yml")" == workflow_dispatch
check 'publication token is scoped to the gated publish job' bash -c "grep -Fxq 'permissions: {}' '$workflows/publish-release.yml' && [[ \$(grep -c 'contents: write' '$workflows/publish-release.yml') == 1 ]] && sed -n '/^  publish:/,\$p' '$workflows/publish-release.yml' | grep -Fxq '    environment: release' && sed -n '/^  publish:/,\$p' '$workflows/publish-release.yml' | grep -Fq 'contents: write'"
check 'publication requires dispatch from main and preflight before build' bash -c "grep -Fq 'refs/heads/main' '$workflows/publish-release.yml' && grep -Fxq '    needs: preflight' '$workflows/publish-release.yml' && grep -Fxq '    needs: [preflight, build]' '$workflows/publish-release.yml'"
check 'no workflow uses repository secrets' bash -c "! grep -Fq 'secrets.' $workflows/*.yml"
pinned=true
while IFS= read -r line; do
  [[ "$line" =~ uses:\ [A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+@[0-9a-f]{40}\ \#\ v[0-9.]+$ ]] || { printf 'unpinned: %s\n' "$line" >&2; pinned=false; }
done < <(grep -hE '^\s+(- )?uses:' "$workflows"/*.yml)
check 'every action is pinned by SHA' "$pinned"
check 'checkouts never persist credentials' bash -c "[[ \$(grep -c 'actions/checkout@' $workflows/*.yml | awk -F: '{s+=\$2} END {print s}') == \$(grep -c 'persist-credentials: false' $workflows/*.yml | awk -F: '{s+=\$2} END {print s}') ]]"
ci_contexts=$(printf 'release-contract\nverify (linux)\nverify (macos)\n')
check 'ruleset requires exactly the CI job checks' bash -c "[[ \$(jq -r '.rules[] | select(.type == \"required_status_checks\") | .parameters.required_status_checks[].context' '$repository_root/.github/rulesets/main.json' | LC_ALL=C sort) == '$ci_contexts' ]] && grep -Fq 'name: verify (\${{ matrix.platform }})' '$workflows/ci.yml' && grep -Fxq '          - platform: linux' '$workflows/ci.yml' && grep -Fxq '          - platform: macos' '$workflows/ci.yml' && grep -Fxq '    name: release-contract' '$workflows/ci.yml'"
check 'ruleset: squash only, reviews, code owners, threads, no direct bypass' bash -c "jq -e '(.rules[] | select(.type == \"pull_request\") | .parameters | .allowed_merge_methods == [\"squash\"] and .required_approving_review_count >= 1 and .require_code_owner_review and .required_review_thread_resolution) and ([.rules[].type] | index(\"deletion\") and index(\"non_fast_forward\")) and all(.bypass_actors[]; .bypass_mode != \"always\")' '$repository_root/.github/rulesets/main.json' >/dev/null"
check 'CODEOWNERS covers the repository' grep -Eq '^\* @[A-Za-z0-9-]+$' "$repository_root/.github/CODEOWNERS"
skill=$repository_root/.agents/skills/axiom-release/SKILL.md
check 'axiom-release skill exists and is routed' bash -c "grep -Fxq 'name: axiom-release' '$skill' && grep -Fq '.agents/skills/axiom-release/SKILL.md' '$repository_root/AGENTS.md'"
check 'skill publishes only through release.sh after human authorization' bash -c "grep -Fq 'scripts/release.sh publish' '$skill' && grep -Fq -- '--authorize-publication' '$skill' && ! grep -Eiq 'gh release (create|upload|edit|delete)|git tag|git push .*--tags|pending_deployments|--force' '$skill'"
check 'skill is not a product Runtime skill' bash -c "[[ ! -e '$repository_root/internal/codexruntime/skills/axiom-release' ]]"

if ((failures > 0)); then
  printf 'FAIL: %s release flow check(s) failed\n' "$failures" >&2
  exit 1
fi
printf 'PASS: release preflight, notes, publication state machine, authority boundary and workflow contracts\n'
