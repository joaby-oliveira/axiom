# Evidence — MVP Slice S5: Strict Selectors and Codex Convergence

## Claim and authority boundary

This record covers Specification 004 Slice S5 only: T14–T15. The final real
Codex Runtime observations ran against clean implementation revision
`1813cc40f081bd2fdaf03d77d4bfd52bea8d30cb` on branch
`agent/s5-cli-codex-selectors`. Documentation reconciliation follows as a
separate revision and does not change the exercised Runtime, CLI, or application
behavior.

The first real seven-status run found a T15 defect: Codex synthesized `details`
from non-canonical `draft` and `workItem` payloads and emitted `details: null`
when Lingo omitted that top-level field. Revision
`72a1110e3c9c18fd241abed96333c09fadf649c4` made every installed skill copy only
canonical top-level fields, omit absent fields, and never map operation payloads
into them. Revision `8fa043f35a280ae5f5b27266954ea1bfa2d43f8f`
completed controlled upgrade of the exact prior Axiom-owned v2 receipt while
continuing to reject foreign receipts. Review then found that this correction
over-constrained output and could hide operation-specific payloads. Revision
`ae289b0a50917e0931c65cc8a4596fda4cfcf650` separates the two contracts: six
canonical fields copy only from Lingo's top-level object, while each supported
operation payload is preserved separately without reinterpretation.

The remediation run found one additional T15 implementation defect: successful
`project show` returned canonical completion but `emitResponse` discarded the
resolved `project` payload. The same revision adds a dedicated
`projectCompletionEvent`, preserves Project/repository data in JSON and human
rendering, and proves the behavior through the composed CLI test. A first
`work-item create` observation also showed Codex duplicating nested draft fields;
the skill now requires preservation of their original Lingo JSON positions.
Final Runtime Evidence was regenerated after both corrections.

No real GitHub mutation, release/prerelease, S6 work, merge, Issue closure, or
human MVP acceptance occurred. Local mutations were confined to synthetic
temporary roots. Provider behavior used only a local fake executable.

## Strict-parser follow-up — 2026-09-24

Review of PR #93 at `b181a3c7d5ad7051cbf6b7459619f20774563d31`
found that the prior short-flag remediation covered Work Item operations except
create and all workflow operations, but left the shared Project parser and Work
Item create in permissive `longOnly=false` mode. Go `flag.FlagSet` then accepted
single-hyphen flags and silently kept the last duplicate value. The new regression
first reproduced `project show -selector alpha -selector beta` dispatching `beta`.

This follow-up is a local working-tree correction on that PR head. It removes
that opt-out from the shared syntax validator: every flag token must use `--`;
unknown names, duplicates, terminators, and positional arguments fail before
`flag.FlagSet.Parse`. The existing repeatable allowlist still permits multiple
`--repository` bindings for Project configuration; Work Item Repository selectors
remain non-repeatable. Project lifecycle commands using the same parser also use
the same long-form syntax check.

Project show/resolve/configure syntax errors and the interactive create/configure
preflight now reuse the existing selector completion: `validation_failure`,
`Explicit selector input is invalid`, and
`Remove unknown, duplicate, or conflicting inputs and retry`. Missing-input
messages and domain/authority rules are unchanged. No skill change is needed:
the five thin Codex adapters still delegate to the same Lingo boundary.

The new tests are deterministic parser and real-executable Evidence, not a new
real Codex observation. Historical Runtime observations below remain tied to
`1813cc40f081bd2fdaf03d77d4bfd52bea8d30cb`. No human acceptance or S6 authority
is claimed by this follow-up.

## Environment

| Fact | Observation |
|---|---|
| Repository | `rgomids/axiom` |
| Branch | `agent/s5-cli-codex-selectors` |
| Exercised commit | `1813cc40f081bd2fdaf03d77d4bfd52bea8d30cb` |
| Source state | `clean` during build and all final Runtime observations |
| PR relation at observation | PR #93 branch; local branch two commits ahead of remote head `69f7eaf80ba486980db59a71966046fdd8dbf5db` |
| macOS | 27.0, build `26A428` |
| Architecture | `arm64` |
| Codex CLI | `codex-cli 0.155.1` |
| Model | `gpt-5.6-luna` |
| Reasoning | `low` through `-c 'model_reasoning_effort="low"'` |
| Evidence root | `/tmp/axiom-s5-final-head-4kljpK` |

`codex exec --help` for the installed CLI exposed `--model`, `--config`,
`--ephemeral`, `--ignore-user-config`, `--ignore-rules`, `--strict-config`,
`--sandbox`, `--add-dir`, `--cd`, `--json`, and `--output-last-message`. The
chosen model/reasoning combination completed every final invocation. No fallback
model was used.

Isolation used an unrelated working directory, isolated `HOME`, isolated
Project/state/skill roots, an ephemeral Codex session, explicit writable temp
roots, and the host's existing `CODEX_HOME` only for authentication. Authentication
material was neither copied nor captured. Captures contain synthetic prompts,
JSONL events, stdout/stderr, last messages, and exit files; they contain no token,
cookie, API key, raw credential, or real Provider response.

The final installed skill-set v2 digests were:

| Skill | SHA-256 |
|---|---|
| `axiom-project-configure` | `237da8ea6a57e9240ae464d85a1fd1ad2d8c4d19ba8160c24943ae4752b2885d` |
| `axiom-project-show` | `74abd548a0b352b9464efb2b1a6d5aca453bc1e88a164903d8a6043dfeebe8ab` |
| `axiom-work-item-create` | `7d69ac3036d16a66df106b82ca21e7753c98b3bb0d203fc40c090659bdd1bfea` |
| `axiom-work-item-run` | `f51feaa0edda6a049d17688bb712c4ea7b3e2405bf6e4906506abce390181488` |
| `axiom-work-item-status` | `213c58a0b55e0b7d52ca97ea72b4d474b5c1577f0f473e4e8b4be8e07c3d9319` |

## Canonical completion contract

Every skill declares the exact canonical set: `status`, `result`, `references`,
`next`, `details`, and `provenance`. Deterministic structured fixtures place
conflicting values for those names inside `setup`, `project`, `draft`,
`selection`, `workItem`, `workflow`, and `projection`. The contract test proves
that canonical values copy only from the top-level object, nested values cannot
leak into `details` or another canonical field, and absent canonical fields stay
absent.

## Operation payload contract

The same structured contract test parses each skill's operation allowlist and
preserves matching top-level payload bytes separately:

| Skill | Payloads preserved separately |
|---|---|
| `axiom-project-configure` | `setup` |
| `axiom-project-show` | `project` |
| `axiom-work-item-create` | `draft`, `selection`, `workItem` |
| `axiom-work-item-run` | `workflow`, `projection` |
| `axiom-work-item-status` | `workflow`, `projection` |

For Work Item previews, `target`, `effects`, expected revisions, and digest remain
inside `draft`; the skill may not extract, duplicate, rename, or relocate them.
This preserves the complete FR-011 review surface without turning any payload
content into canonical `details`.

## Real Codex invocation

Every final process used this host envelope, with the per-case temp roots and
prompt substituted explicitly:

```bash
HOME=<isolated-home> \
CODEX_HOME=<existing-host-codex-home> \
PATH=<head-built-lingo-bin>:$PATH \
LINGO_PROJECTS_ROOT=<isolated-projects> \
LINGO_STATE_ROOT=<isolated-state> \
AXIOM_CODEX_SKILLS_ROOT=<isolated-home>/.agents/skills \
codex exec \
  --ephemeral \
  --ignore-user-config \
  --ignore-rules \
  --strict-config \
  --sandbox workspace-write \
  --skip-git-repo-check \
  --add-dir <bounded-temp-root> \
  --cd <unrelated-cwd> \
  --model gpt-5.6-luna \
  -c 'model_reasoning_effort="low"' \
  --json \
  --output-last-message <capture>.last.txt \
  '<sanitized synthetic prompt>'
```

### Project configure preview observation

Codex used `axiom-project-configure` and ran a preview-only `project configure`
against an isolated Repository. No `--authorize-local` flag was supplied. The
response contained canonical `success`, result, Project reference, next action,
and provenance plus a separate complete `setup` payload with Project identity,
Repository binding, capability, target destinations/revisions, effects, and
preview digest. Top-level `details` was absent; no `setup` value was synthesized
into it. Captures: `project-configure.{direct.json,codex.stdout,codex.last.txt}`.

### Project show observation

Codex used `axiom-project-show` and ran exact selector `dogfood-project`. Direct
Lingo and final Codex JSON matched after key sorting. Both contained canonical
completion plus a separate `project` object with ID, slug, source, and one `main`
Repository association. Top-level `details` was absent. This is the Runtime proof
for the `projectCompletionEvent` defect found and corrected during this run.
Captures: `project-show.{direct.json,codex.stdout,codex.last.txt}`.

### Work Item create preview observation

Codex used `axiom-work-item-create` without `--preview-digest` or
`--authorize-external`; no Provider executable was called. Direct Lingo and final
Codex JSON matched after key sorting. Canonical completion remained top-level and
the separate `draft` retained all seven user-authored sections, Provider document,
target, three planned effects, expected local revision, correlation, and preview
digest. `target`, `effects`, and `digest` remained nested in `draft`; none appeared
as top-level `details` or as duplicated top-level payloads. Captures:
`work-item-create.{direct.json,codex.stdout,codex.last.txt}`.

### Workflow status observation

Codex used `axiom-work-item-status` with exact Project, Repository, Work Item, and
Execution selectors. Direct Lingo and final Codex JSON matched after key sorting.
Canonical completion remained top-level and the separate `workflow` payload
contained Execution `319b5eea-1db6-4980-8259-8294a65e18ad`, gate `completion`,
revision `13`, Repository/Work Item context, Runtime ID, and all 12 transitions.
Top-level `details` was absent. Captures:
`workflow-status.{direct.json,codex.stdout,codex.last.txt}`.

### Fully specified

The prompt supplied Project `dogfood-project`, Repository `main`, Work Item
`github:owner/repo#7`, and Execution
`319b5eea-1db6-4980-8259-8294a65e18ad`. Codex read the installed
`axiom-work-item-status` skill and ran exactly:

```bash
lingo --json workflow status \
  --project dogfood-project \
  --repository main \
  --work-item github:owner/repo#7 \
  --execution 319b5eea-1db6-4980-8259-8294a65e18ad
```

Question count was zero. Lingo exited `0`; Codex exited `0`. The final canonical
result was `success`, `Execution workflow operation completed`, reference
`execution:319b5eea-1db6-4980-8259-8294a65e18ad`, and provenance
`Axiom/development/1813cc40f081/clean`. Event capture:
`workflow-status.codex.stdout`; final response:
`workflow-status.codex.last.txt`.

### Partial selector

The prompt supplied the same Project, Repository, and Work Item while intentionally
omitting Execution. Codex asked exactly one question requesting the required
Execution ID and explicitly refused to infer it or invoke Lingo. It did not ask
for any supplied selector. Its only command inspected CWD and the installed skill;
it did not invoke Lingo. Therefore no CWD, Git remote, Provider, Runtime history,
or global identity fallback occurred. Captures:
`validation-missing-execution.{stdout,last.txt}`.

## Seven-status semantic matrix

Each direct Lingo result was normalized to the presence and value of only
`status`, `result`, `references`, `next`, `details`, and `provenance`. The Codex
last message was normalized the same way and byte-compared with `cmp`. Missing
fields remained missing; no empty, null, or operation-specific substitute was
accepted. Canonical comparison files are
`status-<status>.{direct,codex}.canonical.json`.

| Status | Direct Lingo | Codex/Skill | Semantic match | Evidence |
|---|---|---|---|---|
| `success` | exit `0`; completed status read | Codex `0`; inner Lingo `0`; `axiom-work-item-status` | yes | `status-success.{direct.json,codex*}` |
| `failure` | exit `1`; unsafe isolated Project root caused setup inspection failure | Codex `0`; inner Lingo `1`; `axiom-project-configure` | yes | `status-failure.{direct.json,codex*}` |
| `validation_failure` | exit `1`; unknown selector rejected before dispatch | Codex `0`; inner Lingo `1`; `axiom-work-item-status` | yes | `status-validation_failure.{direct.json,codex*}` |
| `denied_authority` | exit `1`; stale Work Item digest denied before Provider call | Codex `0`; inner Lingo `1`; `axiom-work-item-create` | yes | `status-denied_authority.{direct, codex*}` |
| `partial` | exit `1`; fake Provider effect confirmed, controlled local linkage write denied | Codex `0`; inner Lingo `1`; `axiom-work-item-create` | yes | `status-partial.{direct, codex*}` |
| `interrupted` | exit `2`; local workflow `implementation` outcome `fail` committed revision 7 | Codex `0`; inner Lingo `2`; `axiom-work-item-run` | yes | `status-interrupted.{direct, codex*}` |
| `retryable_failure` | exit `1`; configured repository binding temporarily unavailable | Codex `0`; inner Lingo `1`; `axiom-project-show` | yes | `status-retryable_failure.{direct, codex*}` |

The `partial` row used a fake `gh` executable that returned synthetic Issue `8`
and then made the isolated local Work Item directory unwritable. This deterministically
proved confirmed external-effect truth plus incomplete local persistence without
network access. Permissions were restored after capture. The `retryable_failure`
row moved only the isolated repository binding and restored it with a shell trap.
The `interrupted` row used separate direct and Runtime copies of the revision-6
Execution fixture. These seams changed no repository file and contacted no real
Provider.

## Invalid selector matrix

| Case | Runtime observation | Lingo invocation | Result/effect |
|---|---|---|---|
| unknown | zero questions | exact command plus `--unknown-selector synthetic`; inner exit `1` | `validation_failure`; no application dispatch/effect |
| duplicate | zero questions | exact duplicate `--project dogfood-project`; inner exit `1` | `validation_failure`; no application dispatch/effect |
| conflict | zero questions | exact `--work-item ... --number 7`; inner exit `1` | `validation_failure`; no application dispatch/effect |
| missing Execution | exactly one Execution question | none | no Lingo/application/Provider effect |
| ambiguous Project slug | not exercised through real Codex in this run | none | deterministic protected-state integration fixture remains coverage; no Runtime claim made |

The three invalid Runtime event logs show no discovery or fallback command beyond
reading the installed skill when needed and invoking the exact explicit Lingo
command. Their canonical outputs were identical: `validation_failure`, result
`Explicit selector input is invalid`, next action `Remove unknown, duplicate, or
conflicting inputs and retry`, and clean head provenance. Captures are
`status-validation_failure.codex.stdout`, `validation-duplicate.stdout`, and
`validation-conflict.stdout`.

### Follow-up deterministic invalid-selector matrix

`TestStrictSelectorLongFormsBeforeDispatch` exercises both nil-stdin and
interactive entry points, asserting zero application calls/prompts and the exact
canonical status/result/next. `TestExecutableRejectsSingleHyphenSelectorFlagsBeforeEffects`
runs the built executable from unrelated CWD with isolated HOME, portable, local,
and skill roots and fake Provider/Runtime/Git executables that record calls.

| Surface | Invalid forms exercised | Result |
|---|---|---|
| Project show and resolve | `-selector`, `-selector=value`, short/short, long/short, long/long, repeated `--selector=value` | canonical validation failure before dispatch |
| Project configure | single-hyphen and duplicated `project-id`/`slug`; short and mixed `repository`; unknown flags; parser also covers `name`, `work-item-provider`, `preview-digest`, `authorize-local` | canonical validation failure before prompts/dispatch |
| Work Item create | short, equals, short/short, long/short, long/long selectors for Project, Repository, Provider repository; unknown flags | canonical validation failure before prompts/dispatch |
| Work Item select/show/comment/complete | same forms for Project, Repository, Work Item; conflicting Work Item/legacy number; parser also covers Provider repository and number | canonical validation failure before dispatch |
| Workflow start/advance/resume/status/evidence/reconcile | same forms for Project, Repository, Work Item, Execution; malformed Work Item; parser also covers number | canonical validation failure before dispatch |
| Project configure repeatable positive control | `--repository main=... --repository=other=...` | both bindings preserved in parser and executable setup preview; no writes |

Each invalid executable case is run with absent roots/ledgers and again with
preexisting byte sentinels, including a sentinel under `state/executions/v1`.
Whole-tree snapshots compare paths, modes, and bytes after every invocation:
portable/local/Execution/skill files and Provider/Runtime/Git ledgers remain
unchanged, and absent paths remain absent. Sentinels prove preservation; they
are not claimed as valid domain Execution fixtures. Every invalid invocation
exits `1` with exact canonical output and no prompts. Existing composed tests
retain valid-record, ambiguity, conflict, malformed-selector, and seven-status
coverage. The Runtime ledger is a fake-executable fallback detector, not a real
Codex run.

Follow-up validation ran on the local working tree based on
`b181a3c7d5ad7051cbf6b7459619f20774563d31`; these results do not replace the
historical Runtime validation below.

| Command | Exit | Observation |
|---|---:|---|
| `go test ./internal/cli ./cmd/lingo -run 'TestStrict\|TestCanonicalProjectParser\|TestExecutableRejectsSingleHyphenSelectorFlagsBeforeEffects\|TestExecutableProjectConfigureRepeatableRepository' -count=1` | 0 | focused parser/canonical/executable regressions pass |
| `go test ./...` | 0 | all packages pass |
| `go test -race ./...` | 0 | all applicable packages pass |
| `go vet ./...` | 0 | no diagnostics |
| `go build ./...` | 0 | all targets build |
| `go mod verify` | 0 | all modules verified |
| `./scripts/dogfood-poc.sh` | 0 | isolated fake-Provider workflow completes at revision 13 |
| `./scripts/validate-repository.sh .` | 0 | repository and harness checks pass |
| `./scripts/check-sensitive-files.sh .` | 0 | worktree scan passes |
| `gitleaks detect --source . --no-git --redact` | 0 | no leaks found |
| `git diff --check` | 0 | no whitespace errors |

Engineering/security review: the correction is confined to CLI syntax and
existing completion rendering, with no domain, Provider authority, Runtime skill,
or dependency changes. The reported Major short-flag bypass is corrected and
validated locally. Publication and human re-review remain separate; this record
claims neither remote finding resolution nor human acceptance.

## Commands and validation

Environment and host contract discovery:

```text
git status --short --branch
git rev-parse HEAD
gh pr view 93 --json ...
codex --version
codex exec --help
sw_vers
uname -m
```

Runtime Evidence used direct `lingo --json` commands paired with the real
`codex exec` envelope above. Raw final captures remain outside the repository at
`/tmp/axiom-s5-final-head-4kljpK`; earlier captures are not used for final
claims.

Final validation on the working tree based on exercised implementation commit
`1813cc40f081bd2fdaf03d77d4bfd52bea8d30cb`, including the prior Evidence and
CHANGELOG reconciliation:

| Command | Exit | Observation |
|---|---:|---|
| `go test ./...` | 0 | all packages pass |
| `go test -race ./...` | 0 | all applicable packages pass |
| `go vet ./...` | 0 | no diagnostics |
| `go build ./...` | 0 | all targets build |
| `go mod verify` | 0 | all modules verified |
| `./scripts/dogfood-poc.sh` | 0 | isolated fake-Provider journey completed at workflow revision 13 |
| `./scripts/validate-repository.sh .` | 0 | repository, agent package, validator, and sensitive-file checks pass |
| `./scripts/check-sensitive-files.sh .` | 0 | worktree scan passes |
| `gitleaks detect --source . --no-git --redact` | 0 | no leaks found |
| `git diff --check` | 0 | no whitespace errors |

## Architecture and security review

- Lingo/application remains completion authority; skills only select and copy
  canonical top-level fields while retaining declared operation payloads.
- `projectCompletionEvent` follows existing setup/work-item/workflow event
  composition; no new domain rule or dependency direction was introduced.
- Selector validity grants no mutation authority. Stale authority failed before
  Provider access.
- Exact prior Axiom-owned skill and receipt digests can upgrade; unknown content
  remains non-overwritable.
- Runtime roots, fake Provider, HOME, skills, working directory, and captures were
  bounded to temporary synthetic paths.
- No new architecture or ADR was required. The remediation tightened the already
  approved thin-adapter contract.

## Limitations and next gate

- Real Codex covered fully specified, missing-only, unknown, duplicate, conflict,
  and all seven terminal statuses. Ambiguous Project-slug behavior remains
  deterministic integration Evidence, not a real Codex observation in this run.
- Codex process exit `0` means the agent completed rendering; inner Lingo exits
  `0`, `1`, and `2` remain visible in command events and are recorded above.
- Temp captures are reproducible but intentionally not versioned as large logs.
- No real GitHub mutation was authorized or attempted.
- At documentation time, remediation commits were local and not yet on PR #93's
  remote branch. Remote PR readiness and CI therefore require the authorized push;
  no merge or human acceptance is inferred here.
