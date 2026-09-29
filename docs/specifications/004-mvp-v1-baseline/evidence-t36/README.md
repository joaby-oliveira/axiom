# T36 native Runtime Evidence — 2026-09-27

Result: **S8 ready for human review**, on the corrected local candidate.
This is technical Evidence, not human acceptance, S9 completion or release authority.
The operator explicitly authorized the disposable macOS lab run and subsequently
requested publication as a PR. No Issue transition/closure or merge is inferred.

## Identity and scope

| Fact | Observation |
|---|---|
| Original main | `d3e0a7a8ebe038b3b801bbcf339486a859da5d5a` |
| Corrected run base | `17dd734537789b161172c4677ada84a588b91821` |
| Integrated tree, before publication-only documentation | `ae8c866f95e27190d82bca12fcf40bdda05416d1` |
| Parent / graph revision | `bfdc422a-a1d2-43ea-9aec-e7ea5ff0c05f` / `1` |
| Canonical graph digest | `5f150ed6cb1473930cadfb9faba57db33548ffd309beef5511d38f93820dd491` |
| Runtime configuration digest | `4363f1e0394d2fcb405cac74e093a08e8e46aba88bb7ba5da8dab39d22efc1c6` |
| Evidence digest | `7571f4bfd0d091f025d44c1c301fa411b1cba6247a231319f430e91daeb34191` |
| Host | macOS 27.0 / 26A428 / arm64; native, no container |
| Axiom | fresh source-built development binary inside lab |
| Codex | 0.157.1; `gpt-6-astra`, observed existing configuration and successful probe |
| Claude | 2.1.283; supported `sonnet` alias, accepted by real probe and child |
| Parent / Evidence outcome | `success` / `real_run_recorded` |
| Strict Runtime overlap | 130.649114 seconds |

`LAB` denotes the isolated lab. Source clone, state, portable Project catalog,
installed skills, worktrees, build/test caches, artifacts and Evidence use lab
roots. HOME stays unchanged; existing host Runtime authentication is reused
without copying or reading credential content. The original Axiom checkout
remained at its original HEAD with clean status. Runtime session persistence was
disabled. This is not a forensic audit of vendor-managed caches.

Clean onboarding exercised `version`, `first-run`, Codex skill installation,
Model Profiles through `RuntimeProfileStore`, profile/command resolution, and
Project configure/resolve/show for both sample and real Axiom repositories.
Missing selectors failed explicitly. Profiles were `t36-codex-implementation`,
`t36-claude-documentation`, and `t36-integration`; command argv used installed
executables directly, explicit environment, exact worktree CWD and bounded output.
Children had 20-minute timeouts / maximum 2 attempts; integration had 15 minutes /
maximum 2 attempts. No ambiguous attempt was retried.

## Real delivery and defect

Codex implemented and tested read-only `lingo runtime profile validate` in five
CLI/composition files. Claude changed `docs/commands.md` in its independent
worktree. The children invoked a lab helper backed by `coordination.Service` to
exchange the canonical [question](question.json) and [answer](answer.json) during
their persisted Runtime attempts. Only concrete `LocalService` integration
composed the final delivery, after lineage/base/tree/effect validation and an
authority-bound preview. All nine combined validators passed.

The first graph exposed a clean-install defect: `CoordinationStore.Latest`
recognized `os.ErrNotExist` but not `local.ErrNotFound` from private directory
helpers. First Service publication failed before creating the stream. Existing
tests directly called Store.Publish and missed this boundary. The one-line fix
preserves unsafe/recovery errors. Four missing-hierarchy regression cases failed
before repair; all five hierarchy cases and unsafe-root rejection passed after.

Run 01 is not counted as T36 success: CLI exits and validators passed, but Claude
produced no documentation and neither child published coordination. It remains
preserved locally. Run 02 used fresh graph/worktrees after the fix and a complete
new baseline. Its Runtime and Integration attempts succeeded. Evidence collection
then needed a runner-only correction to extract version tokens from CLI banners;
collection read persisted results without replaying any effects.

## Evidence and validation

- [Canonical Evidence](acceptance.json) contains real dispatch timing, Runtime
  identity/profile provenance, canonical coordination bytes, persisted result
  references, Integration result tree, validator exits/digests and roll-up.
- [Graph projection](graph-projection.json) redacts absolute lab paths. It is
  explicitly **not** canonical graph bytes and cannot reproduce GraphDigest.
- [Validation records](validation.json) retain observed commands/exits and
  seven integrated black-box outcomes. Baseline and combined commands were
  `go test ./... -count=1`, `go test -race ./...`, `go vet ./...`,
  `go build ./...`, `go mod verify`, `./scripts/validate-repository.sh .`,
  `./scripts/check-sensitive-files.sh .`, `git diff --check`, and
  `gitleaks dir . --redact --no-banner`.
- [Negative-case mapping](negative-cases.json) identifies executed tests. 552
  tests/subtests passed in the original scoped suite. Additional public-API
  tests exercised exhausted attempts, absent authority, unavailable Runtime,
  disallowed profile, incompatible capability and no implicit fallback.
- [AC matrix](ac-matrix.md) distinguishes real positive observations from
  deterministic negative and compatibility tests.
- [Seven S9 polish findings](s9-polish-findings.md) are observations only.

Original graph bytes, exact run envelope/argv/environment, source logs, helper
source, binary/configuration digests, CLI final summaries and worktrees remain in
the operator lab. Public files expose sanitized audit records rather than the
whole lab or an independently runnable acceptance harness. No raw reasoning,
credential contents or private provider data are included. Process stdout is
retained only as a digest; separate final summaries and Git results were inspected.
Usage/cost are unavailable, never estimated. Integration profile resolution is
real, but Integration runs concrete local code rather than another LLM process.

The real journey's source/tree identities above remain immutable: publication
adds documentation/Evidence only beyond its observed result. Final PR validation
is recorded in the PR description; green CI or PR publication does not extend
this run's evidence or authorize human acceptance or successor work.
