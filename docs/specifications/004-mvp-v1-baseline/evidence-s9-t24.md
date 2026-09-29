# Exact RC acceptance checkpoint — 2026-09-29

Result: **T24 blocked; T25 blocked; RC not ready for human review.**
Human decision: **PENDING**. This checkpoint records actual preparation and
local deterministic checks; it is not a completed acceptance report.

Candidate: `v0.1.2-rc.1`, source
`f73d6d0c951dd40c5cc97c3794ad7ee5607092b5`. Remote main and the published tag
both resolved to that full revision during this session. Local HEAD equals it;
the worktree is dirty with the pre-existing T23 Evidence update and this work.
No product code, release assets, specification, ADR, credentials or human
acceptance field were changed. No commit, push, PR or Provider mutation occurred.

## Sources and coverage plan

Normative sources: [Specification](spec.md), [Plan](plan.md),
[Tasks T24/T25](tasks.md#t24--clean-environment-clicodexclaudegithub-acceptance-matrix),
[S7](evidence-s7.md), [S8](evidence-s8.md),
[T36](evidence-t36/README.md), [S9 productization](evidence-s9.md),
[T23](evidence-s9-t23.md), and [ADRs](../../decisions/README.md).

[Coverage plan](evidence-s9-rc/coverage.json) maps 190 rows: every AC-01–AC-49,
FR-001–FR-067, MVP-SEC/NFR alias, inherited SEC, HD, ADR, Constitution II–VI,
plus the explicit S8 security and required RC Evidence clauses. It includes
observations, platforms, real/deterministic scope, commands/tooling gaps,
artifacts, authority and statuses. Commands in this plan are future actions,
not claims of execution. AC-24 remains human-only. AC-45/FR-063 have the T23
artifact/publication support; no other acceptance clause is closed by that fact.
Semantic FR/security/NFR/ADR closure remains blocked until T24 produces its
required behavior records. The identifier/link/DAG checks below are structural
checks, not a substitute for that closure.

## Observed release and environments

`./scripts/release.sh verify --tag v0.1.2-rc.1 --download` ran twice in this
session, each exit 0. The second output is retained as
[release-verification.txt](evidence-s9-rc/release-verification.txt).
This official read-only verification downloads release assets into temporary
directories and refreshes local remote-tracking refs; it performs no GitHub
publication. It uses existing GitHub CLI authentication without exporting
credential contents. These effects are separate from the proposed anonymous
installation stage below.

Fresh [release metadata](evidence-s9-rc/release-metadata.json), read via
`gh api repos/rgomids/axiom/releases/tags/v0.1.2-rc.1 --jq ...` (exit 0),
confirms release `398805148`, immutable, prerelease, not draft, with exactly
four assets. [Remote refs](evidence-s9-rc/remote-refs.txt), from
`git ls-remote origin refs/heads/main refs/tags/v0.1.2-rc.1` (exit 0), bind
the full source revision. Assets match the previously published T23 digests.
Release: [v0.1.2-rc.1](https://github.com/rgomids/axiom/releases/tag/v0.1.2-rc.1).

| Required platform | Observation | Acceptance status / missing Evidence |
|---|---|---|
| macOS 27.0 / arm64 | Native host observed by `uname -sm` and `sw_vers -productVersion`; private-root installer stage planned | Blocked before human-authorized run; isolated roots have not been installed. Full account/VM and Runtime/Provider journey still required. |
| Linux / amd64 / ext4 | Archive hashes/format/provenance verified; no native endpoint supplied | Blocked: native clean environment, filesystem facts, exact-RC journey and deferred T22 suite absent. |
| Linux / arm64 / ext4 | Archive hashes/format/provenance verified; no native endpoint supplied | Blocked: native clean environment, filesystem facts, exact-RC journey and deferred T22 suite absent. |

[Initial facts](evidence-s9-rc/initial-state.json) retain the pre-existing T23
file hash and host inventory. Codex and Claude executable paths were located
only. Neither vendor executable was invoked, including for `--version`.
No native Linux run, cross-compilation substitute or synthetic support row is
claimed. No Runtime/Profile availability is inferred from executable presence.
[Host filesystem](evidence-s9-rc/host-filesystem.json), from
`diskutil info -plist /` (exit 0), reports APFS and ownership permissions enabled
for the host root. Case/ACL/fault behavior and the future isolated installation
remain unobserved in this checkpoint.

## Harness and first authority envelope

Added [test-s9-rc-acceptance.sh](../../../scripts/test-s9-rc-acceptance.sh)
and its [collector](../../../scripts/s9-rc-evidence.py). The collector is
maintainer tooling, bound to this exact candidate. Default mode writes a private
plan, envelope, manifest and acceptance checkpoint without starting subprocesses.
It never marks T24/T25 complete. Failed actions retain their exit, bounded output,
digest, timeout and explicit limitation. Existing Evidence is preserved.

The implemented execution stage is deliberately limited to anonymous published
metadata/tag read-back, remote installation into an empty private HOME,
`axiom --json version`, `axiom --json first-run` with neither Runtime on PATH,
then exact reinstall/no-op. It excludes host credentials, GitHub writes,
Runtime invocation and host Axiom roots. It checks immutable release identity,
all four recorded asset digests, exact remote bootstrap hash, short CLI revision,
installed archive/receipt/binary identity and unchanged reinstall inventories.
Timeout is 600 seconds per process; stage attempts 1; installer-owned HTTPS
retries remain bounded. Captured output is capped while streaming at 64 KiB,
with total-byte count and full-stream SHA-256. Download is also size-bounded.

Remote bootstrap bytes are downloaded and checked against the exact source
digest before executing the saved script with `sh --version`; this makes the
public install instructions inspectable without a second unverified bootstrap
download. It is still a remote published-RC install, not a source build. This
stage does not exercise all four first-run cases or the entire native row.

Current proposed [envelope](evidence-s9-rc/macos-arm64-approved-stage-plan/envelope.json):
SHA-256 `5807ece1ca20c7aca9e41da879230c511b4380da522168aa196671dbf955dcb2`.
It materializes every command, target, environment, timeout, attempts, allowed
and forbidden effects, cleanup ownership and retained Evidence; collector bytes
are hash-bound. It has **not been authorized or executed**. The directory name denotes a plan
for approval, not granted authority; its human decision is PENDING.
[Manifest](evidence-s9-rc/macos-arm64-approved-stage-plan/manifest.json): plan-only,
observations empty, Execution IDs empty, human decision PENDING.

After explicit human authorization of that digest only, execution command is:

```bash
./scripts/test-s9-rc-acceptance.sh --version v0.1.2-rc.1 \
  --evidence-dir /Users/rgomids/Projects/axiom/docs/specifications/004-mvp-v1-baseline/evidence-s9-rc/macos-arm64-approved-stage-plan \
  --execute-install \
  --approved-envelope-sha256 5807ece1ca20c7aca9e41da879230c511b4380da522168aa196671dbf955dcb2
```

Earlier plan directories are superseded, unexecuted previews,
preserved for traceability. Its digest cannot authorize the current collector.
Real Codex/Claude/Profile/Project/Intent/GitHub/graph/dogfood stages remain
unimplemented in this collector and require their own concrete envelopes and
human gates. This first envelope authorizes none of them. No dogfood engineering
activity or parent/child Execution/Evidence IDs exist for this RC session.

## Findings and validation

- **Major, acceptance tooling:** `scripts/test-s7-native.sh` still requires
  `ubuntu:26.04:ext4`. Current Specification/Plan supersede Ubuntu-version support
  wording with Linux and retain ext4/native obligations. Other otherwise suitable
  Linux hosts would receive exit 78. No product fix or silent requirement change
  was made; reconcile the suite explicitly before those native runs.
- **Blocker, environment/Evidence:** all three complete exact-RC clean journeys,
  real Codex+Claude/GitHub observations, coordinated engineering work,
  graph/Integration parent result, failure/recovery/upgrade paths and both native
  Linux filesystem rows remain absent. Historical T36 uses another revision and
  is contextual support only.
- **Blocker, local hygiene:** repository validation on the primary checkout
  returns exit 1: `FAIL: unapproved Claude artifact: .claude`. That pre-existing
  empty directory was preserved. A clean source-plus-task-files validation is
  recorded separately; it never converts the primary checkout's failure to pass.

[Deterministic validation](evidence-s9-rc/deterministic-validation.json):
`go test ./... -count=1`, `go test -race ./...`, `go vet ./...`,
`go build ./...`, `go mod verify`, all exit 0 on the exact HEAD with dirty
documentation/tooling recorded. Some race packages were cached; this is reported
in retained output. Product Go files were unchanged. These checks are local,
not published-binary black-box or real Runtime/Provider acceptance.

Offline collector tests: `python3 -B scripts/test_s9_rc_evidence.py`, eleven pass;
they cover plan/no process, existing file/symlink preservation, envelope drift,
Runtime/unsupported-host refusal, failed public read, official short provenance,
live output cap, timeout after stdout closes, capture-error process termination and persisted interruption/unknown-effects disposition. Controlled fixtures are tooling
tests only. `bash -n scripts/test-s9-rc-acceptance.sh` passes.

Additional check commands/exits, clean-clone validation, independent engineering
and security reviews, artifact sizes and retained-file digests are recorded in
[checks.json](evidence-s9-rc/checks.json),
[review.json](evidence-s9-rc/review.json), and
[inventory.json](evidence-s9-rc/inventory.json). Independent engineering/security review found no remaining blocker in the
narrow Stage 1 collector/plan after the recorded fixes; it does not close full
T24/T25. Inventory measures checkpoint files only, not real Axiom artifacts.
The 64 KiB per-command capture cap is a tooling bound. MVP artifact-size and
retention recommendations remain pending real journey measurements; no automatic
cleanup is authorized. Human assessment is independent; no automated acceptance
recommendation or closure is recorded.

Next actions: authorize only the first local envelope if desired; provide native
Linux environments; prepare concrete Runtime/Provider/dogfood envelopes and a
reproducible approved graph runner; collect every missing T24 observation;
then complete T25 semantic audits, final reviews and reconciliation. Any real
defect must be reported before a distinguishable product fix and revalidation.


## Repository contribution handoff — 2026-09-29

After this checkpoint, the human requested creation of a PR for the retained
Evidence and harness. That instruction authorizes their commit, branch push and
PR creation, including the unchanged pre-existing T23 publication record. It
does not authorize the proposed installation envelope, Runtime/Provider runs,
release promotion or human acceptance. Earlier no-commit/no-PR statements record
the collection snapshot above; T24/T25 remain blocked.
