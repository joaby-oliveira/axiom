#!/usr/bin/env bash
set -euo pipefail

umask 077

repository_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)
temporary=$(mktemp -d)
if [[ -z "$temporary" || ! -d "$temporary" ]]; then
  exit 1
fi
trap 'rm -rf -- "$temporary"' EXIT

binary_root="$temporary/bin"
install_state="$temporary/install-state"
portable_root="$temporary/projects"
state_root="$temporary/state"
skills_root="$temporary/global-skills"
unrelated="$temporary/unrelated-cwd"
repository="$temporary/project-repository"
mkdir -p "$unrelated" "$repository"

AXIOM_BIN_DIR="$binary_root" AXIOM_INSTALL_STATE_ROOT="$install_state" \
  "$repository_root/scripts/install-axiom.sh" >"$temporary/install.json"
export PATH="$binary_root:$PATH"
export LINGO_PROJECTS_ROOT="$portable_root"
export LINGO_STATE_ROOT="$state_root"
export AXIOM_CODEX_SKILLS_ROOT="$skills_root"

run_success() {
  local category=$1
  local destination=$2
  shift 2
  axiom --json "$@" >"$destination"
  grep -q '"status":"success"' "$destination"
  grep -q '"category":"'"$category"'"' "$destination"
}

run_failure() {
  local category=$1
  shift
  local output="$temporary/failure-$category.json"
  if axiom --json "$@" >"$output"; then
    exit 1
  fi
  grep -q '"status":"error"' "$output"
  grep -q '"category":"'"$category"'"' "$output"
}

assert_canonical() {
  local output=$1
  local status=$2
  local result=$3
  grep -Fq '"status":"'"$status"'"' "$output"
  grep -Fq '"result":"'"$result"'"' "$output"
  grep -Fq '"provenance":{"product":"Axiom","version":"development","revision":"' "$output"
  grep -Fq '"sourceState":"' "$output"
  if grep -Fq '"category":' "$output"; then
    exit 1
  fi
}

run_canonical_failure() {
  local label=$1
  local status=$2
  local result=$3
  local next=$4
  shift 4
  local output="$temporary/canonical-failure-$label.json"
  if axiom --json "$@" >"$output"; then
    exit 1
  fi
  assert_canonical "$output" "$status" "$result"
  grep -Fq '"next":"'"$next"'"' "$output"
}

cd "$unrelated"
resolved_binary=$(command -v axiom)
if [[ "$resolved_binary" != "$binary_root/axiom" ]]; then
  exit 1
fi
axiom --json version >"$temporary/version.json"
assert_canonical "$temporary/version.json" success "Axiom build information"

if axiom --json first-run >"$temporary/first-run-missing.json"; then
  exit 1
fi
assert_canonical "$temporary/first-run-missing.json" validation_failure "Codex skill compatibility is not ready"
run_success codex_configured "$temporary/runtime-install.json" runtime codex install
axiom --json first-run >"$temporary/runtime-status.json"
assert_canonical "$temporary/runtime-status.json" success "Lingo and Codex skills are compatible"
skill_count=$(find "$skills_root" -name SKILL.md -type f | wc -l | tr -d ' ')
if [[ "$skill_count" != 5 ]]; then
  exit 1
fi
axiom help >"$temporary/help.txt"
for skill in axiom-project-configure axiom-project-show axiom-work-item-create axiom-work-item-run axiom-work-item-status; do
  grep -q "\$${skill}" "$temporary/help.txt"
done
rm -- "$skills_root/axiom-work-item-status/SKILL.md"
rmdir -- "$skills_root/axiom-work-item-status"
if axiom --json runtime codex status >"$temporary/runtime-missing.json"; then
  exit 1
fi
assert_canonical "$temporary/runtime-missing.json" validation_failure "Codex skill compatibility is not ready"
run_success codex_configured "$temporary/runtime-repair.json" runtime codex install

gh_binary="$temporary/gh"
provider_label="$temporary/provider-label"
provider_comment="$temporary/provider-comment.json"
: >"$provider_label"
: >"$provider_comment"
printf '%s\n' '#!/bin/sh' \
  'case "$*" in' \
  '  *search/issues*) printf "%s\n" "{\"total_count\":0,\"items\":[]}" ;;' \
  '*POST*repos/owner/repo/labels*) input=$(cat); value=$(printf "%s" "$input" | sed -n "s/.*\"name\":\"\([^\"]*\)\".*/\1/p"); printf "%s\n" "$value" >"$AXIOM_FAKE_PROVIDER_LABEL"; printf "%s\n" "$input" ;;' \
  '*repos/owner/repo/labels?per_page=100*) if [ -s "$AXIOM_FAKE_PROVIDER_LABEL" ]; then value=$(sed -n "1p" "$AXIOM_FAKE_PROVIDER_LABEL"); printf "[{\"name\":\"%s\"}]\n" "$value"; else printf "%s\n" "[]"; fi ;;' \
  '*POST*issues/7/comments*) cat >"$AXIOM_FAKE_PROVIDER_COMMENT"; printf "%s\n" "{\"id\":1}" ;;' \
  '*issues/7/comments?per_page=100*) if [ -s "$AXIOM_FAKE_PROVIDER_COMMENT" ]; then printf "["; cat "$AXIOM_FAKE_PROVIDER_COMMENT"; printf "]\n"; else printf "%s\n" "[]"; fi ;;' \
  '*POST*issues/7/labels*) input=$(cat); value=$(printf "%s" "$input" | sed -n "s/.*\"labels\":\[\"\([^\"]*\)\"\].*/\1/p"); printf "%s\n" "$value" >"$AXIOM_FAKE_PROVIDER_LABEL"; printf "%s\n" "$input" ;;' \
  '*DELETE*issues/7/labels/*) : >"$AXIOM_FAKE_PROVIDER_LABEL"; printf "%s\n" "{}" ;;' \
  '*issues/7*) if [ -s "$AXIOM_FAKE_PROVIDER_LABEL" ]; then value=$(sed -n "1p" "$AXIOM_FAKE_PROVIDER_LABEL"); labels="[{\"name\":\"$value\"}]"; else labels="[]"; fi; printf "{\"number\":7,\"html_url\":\"https://github.com/owner/repo/issues/7\",\"state\":\"open\",\"labels\":%s}\n" "$labels" ;;' \
  '  *POST*repos/owner/repo/issues*) cat >/dev/null; printf "%s\n" "{\"number\":7,\"html_url\":\"https://github.com/owner/repo/issues/7\",\"state\":\"open\"}" ;;' \
  '  *) exit 1 ;;' \
  'esac' >"$gh_binary"
chmod 700 "$gh_binary"
export AXIOM_GH_BIN="$gh_binary"
export AXIOM_FAKE_PROVIDER_LABEL="$provider_label"
export AXIOM_FAKE_PROVIDER_COMMENT="$provider_comment"

axiom --json project configure --slug dogfood-project --name "Dogfood Project" \
  --repository "main=$repository" --work-item-provider github >"$temporary/project-preview.json"
assert_canonical "$temporary/project-preview.json" success "Project setup preview ready"
project_id=$(sed -n 's/.*"projectId":"\([^"]*\)".*/\1/p' "$temporary/project-preview.json")
preview_digest=$(sed -n 's/.*"digest":"\([^"]*\)".*/\1/p' "$temporary/project-preview.json")
[[ -n "$project_id" && -n "$preview_digest" ]]
axiom --json project configure --project-id "$project_id" --slug dogfood-project \
  --name "Dogfood Project" --repository "main=$repository" \
  --work-item-provider github --preview-digest "$preview_digest" --authorize-local \
  >"$temporary/project-configure.json"
assert_canonical "$temporary/project-configure.json" success "Project setup published"
axiom --json project show --selector dogfood-project >"$temporary/project-show.json"
assert_canonical "$temporary/project-show.json" success "Project resolved"
grep -Fq '"references":["project:' "$temporary/project-show.json"
grep -Fq '"repository:main"' "$temporary/project-show.json"
grep -Fq '"project":{"id":"' "$temporary/project-show.json"
grep -Fq '"repositories":[{"key":"main","path":"'"$repository"'"}]' "$temporary/project-show.json"
run_canonical_failure project-not-found validation_failure "Project was not found" \
  "Provide an existing Project UUID or slug" project show --selector missing-project
mv -- "$repository" "$temporary/moved-repository"
run_canonical_failure repository-unavailable retryable_failure \
  "Project repository is unavailable" \
  "Restore the configured repository binding and retry inspection" \
  project show --selector dogfood-project
mv -- "$temporary/moved-repository" "$repository"

draft_args=(work-item create --project dogfood-project --repository main \
  --provider-repository owner/repo --intent "Dogfood delivery is blocked" \
  --desired-outcome "Dogfood delivery proceeds safely" \
  --context "Synthetic deterministic E2E" --scope "Bounded Work Item change" \
  --constraints "Preserve exact authority" --non-goals "No implicit workflow" \
  --acceptance "Deterministic dogfood passes")
axiom --json "${draft_args[@]}" >"$temporary/work-item-preview.json"
assert_canonical "$temporary/work-item-preview.json" success "Work Item draft ready for review"
work_item_digest=$(sed -n 's/.*"digest":"\([^"]*\)".*/\1/p' "$temporary/work-item-preview.json")
[[ -n "$work_item_digest" ]]
run_canonical_failure work-item-stale denied_authority "Work Item authority denied" \
  "Review the exact preview and grant only the required authority" \
  "${draft_args[@]}" --preview-digest stale --authorize-external
axiom --json "${draft_args[@]}" --preview-digest "$work_item_digest" \
  --authorize-external >"$temporary/work-item.json"
assert_canonical "$temporary/work-item.json" success "GitHub Work Item linked"
grep -Fq '"externalId":"7"' "$temporary/work-item.json"
axiom --json workflow start --project dogfood-project --repository main --number 7 \
  >"$temporary/workflow-start.json"
assert_canonical "$temporary/workflow-start.json" success "Execution workflow operation completed"
execution_id=$(sed -n 's/.*"executionId":"\([^"]*\)".*/\1/p' "$temporary/workflow-start.json")
[[ -n "$execution_id" ]]
revision=1

for gate in intake specification clarification; do
  printf '%s\n' "$gate" >"$repository/$gate.md"
  reference_digest=$(shasum -a 256 "$repository/$gate.md" | awk '{print $1}')
  axiom --json workflow advance --project dogfood-project --repository main --number 7 \
    --expected-revision "$revision" --gate "$gate" --outcome pass \
    --reference "evidence:$gate.md:$reference_digest" >"$temporary/workflow-$gate.json"
  assert_canonical "$temporary/workflow-$gate.json" success "Execution workflow operation completed"
  revision=$((revision + 1))
done

specification_digest=$(shasum -a 256 "$repository/specification.md" | awk '{print $1}')
axiom --json workflow fact --project dogfood-project --repository main --number 7 \
  --expected-revision "$revision" --fact planning-authority --active \
  --reference "specification:specification.md:$specification_digest" --authorize-local \
  >"$temporary/workflow-planning-authority.json"
assert_canonical "$temporary/workflow-planning-authority.json" success "Execution workflow operation completed"
revision=$((revision + 1))

for gate in plan tasks; do
  printf '%s\n' "$gate" >"$repository/$gate.md"
  reference_digest=$(shasum -a 256 "$repository/$gate.md" | awk '{print $1}')
  axiom --json workflow advance --project dogfood-project --repository main --number 7 \
    --expected-revision "$revision" --gate "$gate" --outcome pass \
    --reference "evidence:$gate.md:$reference_digest" >"$temporary/workflow-$gate.json"
  assert_canonical "$temporary/workflow-$gate.json" success "Execution workflow operation completed"
  revision=$((revision + 1))
done

plan_digest=$(shasum -a 256 "$repository/plan.md" | awk '{print $1}')
axiom --json workflow fact --project dogfood-project --repository main --number 7 \
  --expected-revision "$revision" --fact implementation-authority --active \
  --reference "plan:plan.md:$plan_digest" --authorize-local \
  >"$temporary/workflow-implementation-authority.json"
assert_canonical "$temporary/workflow-implementation-authority.json" success "Execution workflow operation completed"
revision=$((revision + 1))

# S6 treats zero lifecycle markers as Provider drift. Seed the bounded fake with
# the exact already-aligned observation; deterministic adapter tests cover label
# creation/replacement effects separately.
printf '%s\n' 'axiom:stage:implementing' >"$provider_label"
axiom --json workflow reconcile --project dogfood-project --repository main --number 7 \
  --expected-revision "$revision" >"$temporary/workflow-projection-preview.json"
assert_canonical "$temporary/workflow-projection-preview.json" success "Execution workflow operation completed"
projection_digest=$(sed -n 's/.*"projection":.*"digest":"\([^"]*\)".*/\1/p' "$temporary/workflow-projection-preview.json")
projection_key=$(sed -n 's/.*"projectionKey":"\([^"]*\)".*/\1/p' "$temporary/workflow-projection-preview.json")
[[ -n "$projection_digest" && -n "$projection_key" ]]
axiom --json workflow reconcile --project dogfood-project --repository main --number 7 \
  --expected-revision "$revision" --preview-digest "$projection_digest" --authorize-external \
  >"$temporary/workflow-projected.json"
assert_canonical "$temporary/workflow-projected.json" success "Execution workflow operation completed"
grep -Fq 'axiom:stage:implementing' "$provider_label"
grep -Fq 'axiom:workflow-projection:' "$provider_comment"
axiom --json workflow reconcile --project dogfood-project --repository main --number 7 \
  --expected-revision "$revision" >"$temporary/workflow-projection-replay.json"
assert_canonical "$temporary/workflow-projection-replay.json" success "Execution workflow operation completed"
grep -Fq '"effects":[]' "$temporary/workflow-projection-replay.json"

printf '%s\n' implementation >"$repository/implementation.md"
implementation_digest=$(shasum -a 256 "$repository/implementation.md" | awk '{print $1}')
if axiom --json workflow advance --project dogfood-project --repository main --number 7 \
  --expected-revision "$revision" --gate implementation --outcome fail \
  --reference "evidence:implementation.md:$implementation_digest" >"$temporary/workflow-interrupted.json"; then
  exit 1
fi
assert_canonical "$temporary/workflow-interrupted.json" interrupted "Execution remains at the current workflow stage"
revision=$((revision + 1))
axiom --json workflow resume --project dogfood-project --repository main --number 7 \
  --expected-revision "$revision" >"$temporary/workflow-resume.json"
assert_canonical "$temporary/workflow-resume.json" success "Execution workflow operation completed"
revision=$((revision + 1))

printf '%s\n' implementation >"$repository/implementation.md"
implementation_digest=$(shasum -a 256 "$repository/implementation.md" | awk '{print $1}')
axiom --json workflow advance --project dogfood-project --repository main --number 7 \
  --expected-revision "$revision" --gate implementation --outcome pass \
  --reference "evidence:implementation.md:$implementation_digest" \
  >"$temporary/workflow-implementation.json"
assert_canonical "$temporary/workflow-implementation.json" success "Execution workflow operation completed"
revision=$((revision + 1))

axiom --json workflow fact --project dogfood-project --repository main --number 7 \
  --expected-revision "$revision" --fact review-started --active \
  --reference "evidence:implementation.md:$implementation_digest" --authorize-local \
  >"$temporary/workflow-review-started.json"
assert_canonical "$temporary/workflow-review-started.json" success "Execution workflow operation completed"
revision=$((revision + 1))

for gate in review evidence reconciliation completion; do
  printf '%s\n' "$gate" >"$repository/$gate.md"
  reference_digest=$(shasum -a 256 "$repository/$gate.md" | awk '{print $1}')
  axiom --json workflow advance --project dogfood-project --repository main --number 7 \
    --expected-revision "$revision" --gate "$gate" --outcome pass \
    --reference "evidence:$gate.md:$reference_digest" >"$temporary/workflow-$gate.json"
  assert_canonical "$temporary/workflow-$gate.json" success "Execution workflow operation completed"
  revision=$((revision + 1))
done

evidence_digest=$(shasum -a 256 "$repository/evidence.md" | awk '{print $1}')
axiom --json workflow fact --project dogfood-project --repository main --number 7 \
  --expected-revision "$revision" --fact human-acceptance --active \
  --reference "evidence:evidence.md:$evidence_digest" --authorize-local \
  >"$temporary/workflow-human-acceptance.json"
assert_canonical "$temporary/workflow-human-acceptance.json" success "Execution workflow operation completed"
revision=$((revision + 1))

axiom --json workflow evidence --project dogfood-project --repository main --number 7 \
  >"$temporary/workflow-evidence.json"
assert_canonical "$temporary/workflow-evidence.json" success "Execution workflow operation completed"
grep -q '"currentGate":"completion"' "$temporary/workflow-evidence.json"
grep -q '"status":"completed"' "$temporary/workflow-evidence.json"
grep -q '"lifecycleStage":"accepted"' "$temporary/workflow-evidence.json"

binary_sha=$(shasum -a 256 "$resolved_binary" | awk '{print $1}')
workflow_record=$(find "$state_root/executions/v1" -name '*.json' -type f -print)
workflow_count=$(printf '%s\n' "$workflow_record" | grep -c .)
if [[ "$workflow_count" != 1 ]]; then
  exit 1
fi
workflow_sha=$(shasum -a 256 "$workflow_record" | awk '{print $1}')
printf '{"evidenceVersion":1,"evidence":"axiom_e2e_dogfood","cwdIndependent":true,"globalSkillCount":%s,"workItem":7,"executionId":"%s","revision":%s,"workflow":"completed","projectionKey":"%s","projectionDigest":"%s","binarySha256":"%s","workflowSha256":"%s","result":"pass"}\n' \
  "$skill_count" "$execution_id" "$revision" "$projection_key" "$projection_digest" "$binary_sha" "$workflow_sha"
