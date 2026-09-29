# S9 acceptance preflight — 2026-09-28

## Scope and authority

This is a pre-T23 checkpoint, not a T24 acceptance result or T25 closure.
The operator requested S9 acceptance orchestration and authorized non-destructive
local validation and coherent local commits. Remote mutations, release publication,
Runtime invocation and final human acceptance retain their explicit gates.
Prior favorable human product feedback is context only: it does not fill AC-24.

Sources: [Specification](spec.md), [Plan](plan.md), [Tasks T23–T25](tasks.md#t23--identified-rc-archives-and-authorized-prerelease-publication),
[historical S9 implementation Evidence](evidence-s9.md), and executable state at
`0135a6b8a2a6d91d14eb75af34e4a44dbdcf73c8`. Preserve the historical Evidence;
its earlier no-release/unmerged statements are not this live observation.

## Repository and candidate

- Repository: `rgomids/axiom`, public GitHub repository.
- Primary checkout: `main`, HEAD `084cc384d4c9de3f5244a0867d22548d350c84d5`,
  clean, three commits behind fetched `origin/main`; preserved.
- Isolated acceptance checkout: source HEAD
  `0135a6b8a2a6d91d14eb75af34e4a44dbdcf73c8`, clean before validation;
  documentation branch `agent/s9-acceptance` created after validation.
- Observed stable: [v0.1.1](https://github.com/rgomids/axiom/releases/tag/v0.1.1),
  latest, source revision above; manifest `0.1.1`.
- Proposed candidate: `v0.1.2-rc.1`, same source revision, tag absent,
  `prerelease=true`, `make_latest=false`. No candidate bytes prepared remotely yet.
- Required CI `verify (linux)`, `verify (macos)`, `release-contract`: success.
  Release environment protected by a required reviewer; no pending Release PR.

| Task | Checkpoint state |
|---|---|
| T37 | Implemented on source revision; historical deterministic Evidence retained |
| T38 | Implemented; local pipeline rerun PASS; proposed RC preparation NOT RUN |
| T39 | Implemented; published proposed-RC install NOT RUN |
| T40 | Implemented; historical fake discovery/bootstrap matrix; real proposed-RC invocation NOT RUN |
| T23 | BLOCKED: native-suite fixture remediation needs integration; preparation/publication authority pending; immutability controls absent |
| T24 | BLOCKED by T23; required native Ubuntu environments not demonstrated |
| T25 | NOT RUN; depends on complete T24 |
| Human acceptance | PENDING; no acceptance or MVP completion claimed |

## Local validation

Executed at the exact clean source revision above on macOS 27.0 arm64:

| Command | Exit / result |
|---|---|
| `git fetch origin main` | 0; remote main identified |
| `scripts/release.sh status` | 0; main CI success, release environment protected |
| `scripts/release-preflight.sh --tag v0.1.2-rc.1 --revision 0135a6b8a2a6d91d14eb75af34e4a44dbdcf73c8 --main-ref origin/main` | 0 / PASS |
| `scripts/release.sh status --tag v0.1.2-rc.1 --revision 0135a6b8a2a6d91d14eb75af34e4a44dbdcf73c8` | 0; `next_action=prepare` |
| `./scripts/test-release-flow.sh` | 0 / PASS; controlled fake publication/authority/failure matrix |
| `./scripts/test-release-pipeline.sh` | 0 / PASS; clean/dirty/content/checksum/provenance matrix |
| `git diff --check` before checkpoint changes | 0 |

Pipeline repeat preserves executable/manifest content; tar/gzip bytes differ.
Only the exact remotely prepared archive bytes can be authorized for publication.
Logs remain local temporary files, not public raw-log imports. Their SHA-256:
release-flow `5f9f591be6312106a1bff565b53a4f9324481c0a121f3b5179bf0538bb8c66be`;
release-pipeline `1590242d78d2a93d874969093e77525c5e78d9b1ab82ec3bde0fb8e2d7ff60be`.

## Environment facts and limitations

| Required native row | Observation | T24 result |
|---|---|---|
| macOS 27.0 / arm64 / APFS | Native host build `26A428`; APFS ownership enabled | NOT RUN; clean account/VM journey not constructed |
| Ubuntu 26.04 / amd64 / ext4 | No native local environment demonstrated | BLOCKED |
| Ubuntu 26.04 / arm64 / ext4 | No native local environment demonstrated | BLOCKED |

`uname`, `sw_vers`, `df`, `diskutil info` and executable-format inspection
confirmed the host facts. Codex and Claude Mach-O arm64 executables exist;
neither was invoked. VMware Fusion `vmrun list` exited 0 with zero running VMs;
limited registered guest inventory did not identify Ubuntu. This does not prove
absence of external hosts. Cross-builds, historical CI and synthetic rows do not
substitute for native T24. Available disk space was approximately 10.7 GB.

## Immutability finding

Read-only `gh api repos/rgomids/axiom/immutable-releases` returned
`enabled=false`, `enforced_by_owner=false`. The ruleset inventory contained
`default`, with no `release-tags`. `publish-release.sh` reports `immutable` but
does not require true before returning success. Publication success alone would
therefore not establish the T23 immutable-candidate obligation.

Do not bypass this finding. Repository controls require separately authorized
administrative effects and read-back. Proposed controls: enable immutable future
releases and create the versioned `release-tags` ruleset. No controls were changed.
Ruleset file SHA-256:
`3e70a496102a974247dcade3780e756f1d2b152b2a9f42a1b3db307ebe97ce04`.

## F10 preflight failure and bounded local remediation

At the source revision above,
`umask 077; go test ./internal/local -run '^TestCoordinationLatestRejectsUnsafeHierarchy$' -count=1`
exited 1: `s8_stores_test.go:251: unsafe latest exists=false err=<nil>`.
The native suite uses this restrictive umask. `os.Mkdir(root, 0755)` is filtered
to `0700`, so the test accidentally constructs a safe directory while expecting
`ErrUnsafe`. Classification: test defect, not demonstrated product failure.

Bounded remediation sets the fixture's mode explicitly with checked
`os.Chmod(root, 0755)`. Only this test changes; production behavior is preserved.
Original failure remains recorded. No release was prepared or invalidated.
Integrate the remediation through an explicitly authorized remote review before
choosing the final candidate source revision. Do not assume the original proposed
SHA remains the candidate after integration.

Validation after the fixture correction, all exit 0:

- `umask 077; go test ./internal/local -run '^TestCoordinationLatestRejectsUnsafeHierarchy$' -count=1`
- `umask 022; go test ./internal/local -run '^TestCoordinationLatestRejectsUnsafeHierarchy$' -count=1`
- `umask 077; go test ./internal/local`
- `go test -race ./internal/local`
- `git diff --check`

Checkpoint hygiene: `./scripts/validate-repository.sh .`,
`gitleaks detect --source . --no-git --redact --no-banner`, and staged
`./scripts/check-sensitive-files.sh --staged .` passed (exit 0).
These checks are scoped preflight/remediation validation, not the final T25 suite.

## First external authority envelope: preparation only

This envelope is a proposal for the original source revision, not authorization
to dispatch. Recompute it after the F10 remediation is integrated and re-preflight
the resulting exact main revision.

| Field | Exact proposed value |
|---|---|
| Repository | `rgomids/axiom` |
| Operation | `scripts/release.sh prepare` dispatch and read-back |
| Workflow / ref | `.github/workflows/release-artifacts.yml` / `main` |
| Inputs | tag `v0.1.2-rc.1`; revision `0135a6b8a2a6d91d14eb75af34e4a44dbdcf73c8` |
| Workflow SHA-256 at revision | `b5fdddb62c3c63fb28fa67bbe6eb1a9f1d5537ff39ad893e9a3150f0e3cc75cb` |
| Effects | One preparation run; build/verify; upload `axiom-release-v0.1.2-rc.1` |
| Artifact contents | Three supported archives, `SHA256SUMS`, preflight, provenance, preparation metadata, release notes |
| Permissions / runner | `contents:read` / `ubuntu-24.04` |
| Disposition | Retain workflow artifact 30 days; no remote deletion |
| Excluded effects | Tags, Releases, latest, Issues, labels, comments, credentials, repository settings, Runtime invocation |
| Risk | Actions capacity consumption; prepared source/artifacts retained by GitHub |

The publication envelope and digest must be computed after preparation and
authorized separately. Local fixture archives are not publishable substitutes.

## Execution plan and recovery

| T24 block | Existing support | Required RC execution |
|---|---|---|
| Public install, provenance, convergence | `test-install-bootstrap.sh`, `test-release-archives.sh`, native suite | `scripts/install.sh --version v0.1.2-rc.1`, direct `axiom`, receipts/hashes on each clean row |
| Four-state first-run | `TestExecutableFirstRunRuntimeMatrix`, bootstrap suites | Codex-only, Claude-only, both, neither; isolated native global integration roots |
| Project, Intent, selectors, metadata | CLI/project/work-item suites, `dogfood-poc.sh` | Published CLI journey from unrelated CWD; portable/local separation and bounded GitHub create |
| Workflow, flags/history, projection | Workflow/local/GitHub adapter suites | RC-bound real projection/read-back; denial/interruption/partial/failure cases |
| Seven statuses, detail/provenance | CLI/Runtime/security suites | RC replay with IDs/digests; deterministic fakes only for controlled failure cases |
| Recovery, POC, cleanup, upgrade, capacity | `test-s7-native.sh`, `test-s7-security.sh` | Native Ubuntu rows plus RC install journey; fixture rebuilds alone are insufficient |
| Graph and real engineering dogfood | Graph/coordination/Runtime/Git suites, historical T36 helper | Fresh RC-bound Codex+Claude graph, actual overlap, isolated children, persisted coordination, integration and parent Evidence |
| T25 coverage and review | Repository/security/Go validators | Full FR/AC/security/NFR/ADR audit and independent reviews only after T24 |

The historical T36 lab helper source was located locally. It hardcodes lab/state,
Project identity, Model Profiles, activity and authority; its old executable must
not run as a new acceptance journey. Recover/adapt source into a fresh bounded lab,
preserve historical state and obtain a new exact Runtime envelope. There is no
versioned independently runnable real T24/T36 acceptance harness. `dogfood-poc.sh`
installs source, and the native suite builds fixture versions; neither substitutes
for the published pinned-RC entrypoint. These are runner adaptation requirements,
not grounds to mark prior product implementation failed.

1. Local F10 fixture correction and validation are complete. Obtain exact remote
   branch/PR authority for its review; human merge remains separate. Re-identify
   main source revision and authorize its preparation envelope before PREPARE.
2. Obtain separate administrative authority for required immutability controls;
   read back applied state. Prepare exact publication envelope and request authority.
3. Publish only authorized bytes, satisfy human environment gate, read back IDs,
   sizes, hashes, tag/revision, prerelease/latest and immutable state; finish T23.
4. Run T24 against only that RC on all native rows. Collect install/provenance,
   first-run four-state matrix, CLI lifecycle/status/failure/recovery/upgrade,
   real bounded Provider, Codex/Claude graph and real Axiom engineering dogfood,
   parent/child coordination/integration/reconciliation and documentation replay.
   Real Provider/Runtime effects require their own envelopes and cleanup ownership.
5. Only after T24 passes, close FR/AC/security/NFR/ADR coverage, final checks and
   independent engineering/security reviews for T25; request RC-bound human decision.

Subagents performed independent read-only release, matrix and environment audits;
release agent also ran the two deterministic release suites and scoped fixture
regression/package/race checks. Models and
effort inherited from the orchestrator, with no upgrade: principal model identity
was not safely exposed for selecting a lower tier. Final T25 independent reviews
have not run. No remote mutations, Runtime runs, acceptance decisions or cleanup
effects were performed by this preflight. Next action: human authority for remote
review of the exact locally committed patch and checkpoint.
