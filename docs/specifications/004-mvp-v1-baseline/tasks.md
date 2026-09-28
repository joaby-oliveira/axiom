# Tasks — Specification 004: Usable MVP v1 Baseline

## Status and authority

**Tasks: Approved — human approval recorded on 2026-09-20.**

**Issue #94 Tasks amendment (T26–T29 / S6 placement): Approved — human approval recorded on 2026-09-24.**

**S1 Implementation: Delivered on `main` through PR #83. This is technical delivery, not an inferred human MVP acceptance.**

**S2 Implementation: Delivered on `main` through PR #84 with reproducible S2 Evidence. This is technical delivery, not inferred human MVP acceptance.**

**S3 Implementation: Delivered on `main` through PR #85 with deterministic Evidence and one bounded real-provider observation. Human acceptance is not inferred.**

**S4 Implementation: Delivered on `main` through PR #91 with deterministic, fake-Provider, and authorized bounded real-provider Evidence. Human acceptance is not inferred.**

**S5 Implementation: Delivered on `main` through PR #93 with deterministic Evidence and bounded real Codex Runtime observation. Human acceptance is not inferred.**

**S6 Implementation: Explicitly authorized on 2026-09-24. T26–T29 are technically complete at implementation commit `56beb4fc310894ff8de128f52c6a96d22711bec8`; reproducible Evidence is recorded in [S6 Evidence](evidence-s6.md). Human acceptance is not inferred.**

**S7 Implementation: Explicitly authorized on 2026-09-25 (Issue #80); technically complete. Final S7 Evidence was executed at `ceb6d0288039793d15a98003d5b30a28a2f19122`, after the T18 Evidence retirement record (HD-S7-T18) implemented at `ffc2515`. T16–T22 are technically complete; T22 is complete under the revised S7 acceptance scope (HD-S7-T22): macOS 27.0/arm64/APFS passes natively, and the Ubuntu 26.04 amd64/ext4 and arm64/ext4 native rows were not executed and are deferred to the T24 clean-environment RC acceptance matrix, where they remain mandatory. T20 keeps the documented skill-set receipt limitation (`partial`/`refresh_required`) as a future release-format evolution. Evidence is recorded in [S7 Evidence](evidence-s7.md). Human acceptance is not inferred.**

**Issue #97 Tasks amendment (T30–T36 / S8, with T23–T25 moved to S9): Approved by explicit human decision on 2026-09-26. T23–T25 retain their IDs and historical RC objective; T24/T25 are expanded for S8 validation/Evidence.**

**S8 Implementation: T30–T36 implementation authorized on 2026-09-27 by explicit human decision (Issue #97 comment #5852650410); the separately operator-authorized T36 lab run is recorded as S8 ready for human review on the corrected local candidate. See [T36 Evidence](evidence-t36/README.md). Human acceptance and S9 implementation/release remain separate.**

**Issue #81 S9 product-scope amendment: direction recorded by explicit human
decision on 2026-09-27. Proposed new Tasks T37–T40 add public `axiom` CLI
identity, automated release artifacts, stable idempotent remote installation/
owned upgrade, and Codex+Claude first-run bootstrap before historical T23–T25.
This Task amendment is proposed for human review and is not implementation
authority.**

**S9 T37–T40 implementation: delivered on `integration/s9-productization`
(PR #106, unmerged) under the operator's 2026-09-28 local implementation
authority; S8/T36 remains the gate for entering RC/acceptance (T23–T25). The
delivered behavior and its validation kinds (confirmed, synthetic, not run) are
recorded in [S9 Evidence](evidence-s9.md); implementation choices recorded there
are not Specification requirements. T23–T25 are not started.**

Approved artifact: `main` at `c7f756209c608ff1f1a88947dcc425d07daaa831`, merge
of [PR #72](https://github.com/rgomids/axiom/pull/72). Human approval in PR #72
accepted the corrected final DAG of 25 Tasks as reconciled with the approved
[Plan](plan.md) and concluded the Tasks phase. It does not authorize T01 or any
other Task, start implementation, grant local or external mutation authority,
publish an installer/release, or make the final MVP acceptance decision.

A subsequent explicit human decision authorized T01 implementation. On 2026-09-21,
T01 was explicitly accepted and merged in PR #74. The same human decision moved
normal MVP delivery governance to the approved S1–S7 Slice boundary: T01–T25
remain the internal decomposition, while an authorized Slice may progress through
its approved Tasks without a new Task-by-Task human authorization unless a
separate human-decision gate is triggered.

Authority chain:

```text
Specification 004 — Approved
-> ADR-0001..ADR-0008 — Accepted
-> Plan — Approved
-> Tasks — Approved
-> S1 Implementation — Delivered on main through PR #83
   -> T01 — Accepted / merged in PR #74
   -> T02–T03 — Merged in PR #83 with reproducible S1 Evidence
-> S2 Implementation — Delivered on main through PR #84
   -> T04–T07 — Merged with reproducible S2 Evidence
-> S3 Implementation — Delivered on main through PR #85
   -> T08 — Implementation and deterministic Evidence complete
   -> T09 — Implementation, deterministic tests, and bounded real-provider observation complete
   -> Human acceptance — Not inferred
-> S4 Implementation — Delivered on main through PR #91
   -> T10–T13 — Application, store, adapter, CLI, black-box and dogfood checks complete
   -> Real-provider projection observation — Complete under exact per-run authority
   -> Human acceptance — Not inferred
-> S5 Implementation — Delivered on main through PR #93
   -> Human acceptance — Not inferred
-> S6 Implementation — authorized; T26–T29 technically complete in the delivery branch
   -> Human acceptance — Not inferred
-> S7 Implementation — authorized 2026-09-25; technically complete (Evidence at ceb6d02)
   -> T16, T17, T19, T21 — technically complete
   -> T18 — technically complete, including 365-day Evidence retirement (HD-S7-T18)
   -> T20 — technically complete; skill-set receipt refresh is a documented future release-format evolution
   -> T22 — complete under revised S7 scope (HD-S7-T22): macOS 27.0/arm64/APFS native pass; Ubuntu 26.04 amd64/arm64 ext4 not executed, deferred to T24
   -> Human acceptance — Not inferred
-> S8 Specification/ADR/Plan/Tasks amendment + ADR-0009 — Approved; implementation authorized 2026-09-27 (Issue #97 comment #5852650410)
   -> T30–T36 — Technical candidate ready for human review; real T36 Evidence recorded
-> S9 Release candidate acceptance (T23–T25, with T24/T25 reconciled for S8) — Not authorized
   -> T37–T40 — implemented and locally validated in PR #106 (unmerged); Ubuntu Evidence synthetic, macOS 27 and real Runtimes not exercised; native acceptance remains T24
   -> T23–T25 — Not started
```

The accepted POC and current Go packages are implementation inputs and historical
Evidence, not the v1 contract. Existing behavior may be reused only where it
satisfies the approved Specification, Plan, ADRs, and the Task-specific Evidence
below. Within an authorized Slice, completion or acceptance of one Task does not
require a new human authorization for the next approved Task. Explicit human
gates remain required for new/changed Specification scope, durable architectural
decisions, material authority/side-effect changes, destructive or external
mutation boundaries called out by the approved Tasks, prerelease/release
publication, required real-provider acceptance runs, and final MVP acceptance.

## Shared delivery rules

Every Task is a vertical delivery unit: it includes the smallest necessary domain,
application, adapter, presentation, tests, and Evidence changes for its observable
outcome. Package names in the Plan describe likely impact, not permission to split
work by technical layer or freeze a public API.

All Tasks inherit these constraints. Evidence is proportional to risk: ordinary
bounded implementation may rely on tests, CI, and PR evidence; stronger
reproducible Evidence remains required where a Task crosses security,
filesystem/recovery, Provider/external mutation, native-platform,
migration/upgrade, or RC boundaries.

All Tasks inherit these constraints:

- `Project != Repository`, `Axiom != Lingo`, `Execution != Agent`,
  `Provider != Transport`, `Integration != MCP`, `Skill != workflow truth`, and
  `Evidence != raw chat history`.
- Runtime skills remain thin. CLI and Runtime surfaces consume the same
  application result and never own separate workflow, authority, persistence,
  recovery, provenance, or status rules.
- Local mutation requires exact authority bound to target, expected revision,
  preview digest, and effect set. External Provider mutation requires separate
  exact authority. Stale observation invalidates authority.
- Confirmed effects remain confirmed. Pre-commit failure preserves prior
  authority; post-commit secondary failure is truthful `partial` or another
  applicable canonical status, never invented rollback.
- Mutable Axiom-owned local state follows ADR-0007: one logical protocol,
  deterministic coordination, private preparation, protected publication, one
  logical commit point, prior/new complete generations, fail-closed readers,
  owned-only cleanup, and `recovery_required` on uncertainty.
- The implementation may choose syscalls, lock libraries, filenames/layouts,
  Go packages, journal encoding, and bounded retry details during the authorized
  Task. Those mechanisms are not architectural contracts and must be justified by
  Evidence on the supported native targets.
- ADR-0008 stays the accepted compatibility contract for existing sequential
  Executions. Accepted ADR-0009 owns new parent/child DAG, attempt, scheduler,
  coordination and integration semantics; its acceptance does not authorize
  implementation.
- Tests use deterministic fault seams and process barriers. Sleeps may enforce a
  bounded timeout but are not the primary race oracle.
- Secret values, raw chat, unrestricted reasoning, and unbounded external output
  never enter state, output, logs, artifacts, Provider content, backups, or
  Evidence. Synthetic sentinels prove rejection and non-leak behavior.
- Windows, package managers, automatic update, signing/notarization, remote
  collaboration/synchronization, arbitrary Runtime/Provider plugins, distributed
  multi-machine execution, and in-place historical POC migration remain outside
  this Specification.

## Requirement labels used by this artifact

Specification 004 has numbered FR and AC identifiers but expresses security and
NFR clauses as normative lists. The labels below are traceability aliases only;
they do not add requirements or change approved text.

| Alias | Existing normative clause |
|---|---|
| MVP-SEC-01 | Exact authority and least privilege at every local/external boundary |
| MVP-SEC-02 | Structural secret rejection, bounded sanitization, and no credential leakage |
| MVP-SEC-03 | Untrusted Intent, Provider content, paths, documents, and generated text remain data |
| MVP-SEC-04 | Bounded/time-limited Provider execution and strict response validation |
| MVP-SEC-05 | No shell interpolation, hooks, implicit Git/network mutation, or permission expansion |
| MVP-SEC-06 | Restrictive ownership/modes plus supported ACL/link checks for every managed root/object |
| MVP-SEC-07 | Portable export allowlisted by construction; local state never excluded only by ignore rules |
| MVP-SEC-08 | Unsupported filesystem/ownership semantics fail closed |
| MVP-SEC-09 | Safe explicit reference-aware cleanup; no automatic deletion for capacity recovery |
| MVP-NFR-01 | Deterministic equivalent input/state outcomes except declared time/identity fields |
| MVP-NFR-02 | Bounded, scannable user summaries |
| MVP-NFR-03 | Bounded file I/O and captured external output |
| MVP-NFR-04 | Restrictive local-state/artifact permissions and unsafe-condition rejection |
| MVP-NFR-05 | Reproducibility without maintainer-local state |
| MVP-NFR-06 | Correlation across result, artifact, Evidence, state, and Provider without secrets |
| MVP-NFR-07 | Native Evidence on macOS 27/arm64/APFS, Ubuntu 26.04/amd64/ext4, and Ubuntu 26.04/arm64/ext4 where applicable |

Inherited `SEC-001`–`SEC-005` retain their definitions in Specification 002.

## Dependency DAG

The table is the authoritative DAG. Dependencies are completion prerequisites;
every Task also requires a future explicit implementation authorization. Stable
IDs do not imply permission to execute in numeric order.

| Task | Slice | Observable delivery unit | Depends on |
|---|---|---|---|
| T01 | S1 | Canonical completion and provenance | None |
| T02 | S1 | Durable bounded detail artifact create/read | T01 |
| T03 | S1 | Shared protected publication and fail-closed reading | T02 |
| T04 | S2 | Checksummed release archive and owned installation | T01, T03 |
| T05 | S2 | Codex skill-set compatibility and first run | T04 |
| T06 | S2 | Guided/non-interactive Project setup preview | T01, T05 |
| T07 | S2 | Authorized Project setup publication | T03, T06 |
| T08 | S3 | Intent interview and structured Work Item draft | T01, T07 |
| T09 | S3 | Authorized GitHub Work Item create/select and linkage | T02, T03, T08 |
| T10 | S4 | Minimal Execution start, status, and resume authority | T03, T09 |
| T11 | S4 | Sequential workflow transition truth | T10 |
| T12 | S4 | Idempotent GitHub stage/comment projection | T11 |
| T13 | S4 | Workflow interruption, concurrency, and projection convergence | T12 |
| T14 | S5 | Strict CLI selector path | T13 |
| T15 | S5 | Thin Codex selector path and completion convergence | T02, T05, T13, T14 |
| T26 | S6 | Deterministic Work Item lifecycle projection from canonical gates | T15 |
| T27 | S6 | GitHub lifecycle/flag projection and bounded history | T26 |
| T28 | S6 | Project metadata policy for Work Items and Pull Requests | T07, T09, T15, T27 |
| T29 | S6 | Missing-local-state reconciliation and Slice convergence | T03, T13, T27, T28 |
| T16 | S7 | Compatibility inspection and historical POC classification | T29 |
| T17 | S7 | Authorized POC backup/export/reconfigure path | T16 |
| T18 | S7 | Reference-aware artifact cleanup and capacity recovery | T02, T10, T15, T29 |
| T19 | S7 | Guided local recovery | T03, T10, T18 |
| T20 | S7 | Owned install upgrade and resumable partial state | T04, T05, T16, T19 |
| T21 | S7 | Cross-slice security and bounded-I/O regression | T09, T13, T15, T17, T18, T20, T29 |
| T22 | S7 | Exact-target native filesystem/install/upgrade Evidence | T19, T20, T21 |
| T30 | S8 | Operator Runtime/Model Profile configuration and capability resolution | T22 |
| T31 | S8 | Agent Planner proposal and deterministic DAG validation | T30 |
| T32 | S8 | Parent/child lineage, envelopes, authority and execution-control persistence | T31 |
| T33 | S8 | Dependency scheduler, isolation, cancellation and retry | T32 |
| T34 | S8 | Structured coordination and bounded child publication | T32 |
| T35 | S8 | Integration/Reconciliation Execution and parent roll-up | T33, T34 |
| T36 | S8 | Codex + Claude acceptance journey and graph Evidence | T35 |
| T37 | S9 | Public `axiom` CLI and distribution identity | T36 |
| T38 | S9 | Automated supported-platform release artifact pipeline | T37 |
| T39 | S9 | Stable remote installer and convergent owned upgrade | T38 |
| T40 | S9 | Codex + Claude first-run bootstrap | T39 |
| T23 | S9 | Identified RC archives and authorized prerelease publication | T40 |
| T24 | S9 | Clean-environment CLI/Codex/Claude/GitHub acceptance matrix | T23 |
| T25 | S9 | Versioned RC Evidence, documentation reconciliation, and human gate | T24 |

The critical path is `T01 -> T02 -> T03 -> T04 -> T05 -> T06 -> T07 -> T08
-> T09 -> T10 -> T11 -> T12 -> T13 -> T14 -> T15`, followed by S6
convergence, S7, S8, and S9. The approved amended slice order is `S1 -> S2 -> S3
-> S4 -> S5 -> S6 -> S7 -> S8 -> S9`: every S8 Task waits for S7/T22, and
S9 productization starts only after S8/T36. Within S8, T33 and T34 can proceed
independently after T32; T35 waits for both, then T36 closes graph Evidence.
The proposed S9 path is T37 -> T38 -> T39 -> T40 -> T23 -> T24 -> T25.

```mermaid
flowchart LR
    T01 --> T02 --> T03 --> T04 --> T05 --> T06 --> T07 --> T08 --> T09
    T09 --> T10 --> T11 --> T12 --> T13 --> T14 --> T15
    T01 --> T04
    T01 --> T06
    T01 --> T08
    T02 --> T09
    T02 --> T15
    T03 --> T07
    T03 --> T09
    T03 --> T10
    T05 --> T15
    T13 --> T15
    T15 --> T26 --> T27 --> T28 --> T29
    T07 --> T28
    T09 --> T28
    T15 --> T28
    T03 --> T29
    T13 --> T29
    T27 --> T29
    T29 --> T16 --> T17
    T02 --> T18
    T10 --> T18
    T15 --> T18
    T29 --> T18
    T03 --> T19
    T10 --> T19
    T18 --> T19
    T04 --> T20
    T05 --> T20
    T16 --> T20
    T19 --> T20
    T09 --> T21
    T13 --> T21
    T15 --> T21
    T17 --> T21
    T18 --> T21
    T20 --> T21
    T29 --> T21
    T19 --> T22
    T20 --> T22
    T21 --> T22 --> T30 --> T31 --> T32
    T32 --> T33 --> T35
    T32 --> T34 --> T35
    T35 --> T36 --> T37 --> T38 --> T39 --> T40 --> T23 --> T24 --> T25
```

## Task definitions

### T01 — Canonical completion and provenance

- **Objective:** `lingo version` and representative no-effect commands expose one bounded semantic result with truthful build provenance in human and JSON forms.
- **Slice:** S1 — Result, provenance and protected local substrate.
- **Dependencies:** None.
- **Requirements:** FR-023; AC-12, AC-15; MVP-NFR-01, MVP-NFR-02, MVP-NFR-06.
- **ADRs / decisions:** ADR-0003; HD-1. Preserve application-owned classification and Axiom-authored versus transported-user-text provenance.
- **Affected boundaries:** completion/provenance contracts, CLI/JSON renderers, build metadata, current POC status adapters.
- **Expected implementation:** Define the seven canonical terminal statuses and exact result fields; construct one immutable process provenance value; adapt `version` plus selected existing read-only operations without changing their domain effects.
- **Authority and side effects:** Read-only. Tests permit process execution of the built local binary only; no filesystem, Git, network, Provider, Runtime-install, or shell-profile mutation.
- **Explicit non-goals:** Detail-artifact storage, workflow redesign, release signing, Provider rendering, or declaring development builds to be releases.
- **Mandatory tests:** Unit status/effect classification matrix; released/development/dirty/unknown-revision provenance; human/JSON semantic equivalence; renderer failure after simulated confirmed effect; bounded output; transported-user sentinel attribution.
- **Expected Evidence:** Test command/exits, golden semantic matrix, exact build inputs, output byte counts, and static proof that renderers do not classify outcomes or hardcode versions.
- **Completion criteria:** All seven statuses and six result fields are stable and equivalent across initial surfaces; no confirmed effect can disappear during rendering.
- **Risks / gates:** Public status/field changes become compatibility-sensitive. Human review required before expanding the approved field set or changing status meaning.

### T02 — Durable bounded detail artifact create/read

**Implementation checkpoint:** complete in the S1 delivery branch; see
[S1 Evidence](evidence-s1.md). This records technical completion, not human
acceptance.

- **Objective:** A forced detailed result publishes exactly one valid machine-local Markdown artifact and resolves it by stable ID; routine compact success creates none.
- **Slice:** S1.
- **Dependencies:** T01.
- **Requirements:** FR-023, FR-033; AC-13, AC-14; MVP-SEC-02, MVP-SEC-06, MVP-SEC-09; MVP-NFR-02–MVP-NFR-04, MVP-NFR-06; SEC-001, SEC-005.
- **ADRs / decisions:** ADR-0005, ADR-0006, ADR-0007; HD-2, HD-3.
- **Affected boundaries:** detail-artifact ownership, metadata/content codec, local correlation, lookup, completion detail reference, Evidence reference seam.
- **Expected implementation:** Deliver create/read for the Plan's v1 artifact contract and initial limits: 1 MiB Markdown, 64 KiB metadata, 256 KiB captured output, 10,000 live objects, 1 GiB aggregate content. Enforce stable opaque identity, digest, restrictive publication, sanitization, retention class, and explicit capacity outcome without eviction.
- **Authority and side effects:** Artifact create authority covers one exact state root/object and expected capacity observation. Tests write only isolated roots. No portable/Repository writes, Provider calls, cleanup, or automatic deletion.
- **Failure / recovery:** Incomplete publication yields no usable reference. Required artifact failure before primary effect is `failure`; after confirmed primary effect is `partial`; optional-detail failure preserves truthful primary status. Ambiguous capacity/reference state fails closed.
- **Explicit non-goals:** Cleanup implementation, portable publication, automatic splitting policy as permanent architecture, operation-attempt entity, or treating every artifact as Evidence.
- **Mandatory tests:** Codec/metadata limits; sanitization/sentinel rejection; count/byte exhaustion; duplicate/collision; interruption at preparation/publication; restrictive mode/ACL/link/type; digest mismatch; lookup by ID only; no-artifact compact outcomes.
- **Expected Evidence:** Unit and local integration matrices with artifact IDs/digests/sizes, capacity ledgers, tree hashes, fault stages, permissions, and proof portable/Repository trees are unchanged.
- **Completion criteria:** Valid complete artifacts are addressable and correlated; invalid/incomplete/over-capacity state is never returned or auto-evicted; Plan defaults remain labeled initial guardrails.
- **Risks / gates:** Measured dogfood data may justify later reviewed limit changes; it cannot silently alter ADR-0006 ownership/retention invariants.

### T03 — Shared protected publication and fail-closed reading

**Implementation checkpoint:** complete in the S1 delivery branch; see
[S1 Evidence](evidence-s1.md). This records technical completion, not human
acceptance and not authority for S2.

- **Objective:** Existing Project, installation, Work Item, workflow, and artifact stores expose one consistent old-or-new commit truth and deterministic `recovery_required` behavior under supported faults.
- **Slice:** S1.
- **Dependencies:** T02.
- **Requirements:** FR-022–FR-026; AC-17; MVP-SEC-01, MVP-SEC-06, MVP-SEC-08; MVP-NFR-01, MVP-NFR-03, MVP-NFR-04; SEC-003–SEC-005.
- **ADRs / decisions:** ADR-0004–ADR-0007; HD-3.
- **Affected boundaries:** consumer-owned local-store ports, readers, publication coordination, revisions/authority, recovery metadata, existing POC adapters.
- **Expected implementation:** Select bounded platform mechanisms behind narrow ports; document one total coordination order; require expected revisions; prepare privately; validate complete bytes/identity/permissions/links; protect create/update; define commit; confirm canonical generation; retain bounded prior/new recovery facts; clean only positively owned protocol objects.
- **Authority and side effects:** Exact local mutation authority only. Tests permit isolated-root writes and controlled process termination. No external Provider/Git/Runtime mutation and no recursive root deletion.
- **Failure / recovery:** Exercise F0–F8. Pre-commit retains prior authority; confirmed post-commit remains committed; unknown/contradictory state is preserved; readers fail closed; cleanup failure never changes commit truth.
- **Explicit non-goals:** Architectural commitment to syscall, lock package, filename/layout, journal encoding, retry algorithm, database, physical power-loss durability, or malicious arbitrary same-UID interleaving defense.
- **Mandatory tests:** Unit protocol state/classification; adapter integration for create/update/multi-object publication; two-process barrier races; stale authority; short/zero writes; ENOSPC/EDQUOT; permission, collision, replacement, symlink/hard-link, rename/sync/cleanup faults; old/new reader matrix.
- **Expected Evidence:** Per-store F0–F8 table, commit point, revisions/digests, process exits, outside-root hashes, chosen-mechanism rationale, assumptions/exclusions, and native Evidence obligation carried to T22.
- **Completion criteria:** Every mutable v1 local store preserves ADR-0007 semantics; no reader accepts mixed/unknown state; mechanisms remain replaceable behind observable invariants.
- **Risks / gates:** If a selected mechanism changes authority, commit point, recovery meaning, or persisted cross-boundary identity, stop with **Human decision required**.

### T04 — Checksummed release archive and owned installation

**Implementation checkpoint:** complete in the S2 delivery branch; see
[S2 Evidence](evidence-s2.md). This records technical completion, not human
acceptance, release publication, T20 upgrade completion, or T22/T24 native Evidence.

- **Objective:** An exact-version archive installs an inspectable `lingo` plus receipt into an explicit user-owned destination after checksum verification; equivalent reinstall is a no-op and conflicts are preserved.
- **Slice:** S2 — Install, first run and Project setup.
- **Dependencies:** T01, T03.
- **Requirements:** FR-026, FR-031–FR-035; AC-01, AC-19; MVP-SEC-01, MVP-SEC-05, MVP-SEC-06, MVP-SEC-08; MVP-NFR-05, MVP-NFR-07; SEC-002–SEC-005.
- **ADRs / decisions:** ADR-0003, ADR-0005, ADR-0007; HD-1, HD-3, HD-4.
- **Affected boundaries:** release build metadata, target archives, checksums, installer, binary destination, closed installation receipt.
- **Expected implementation:** Build archives for macOS 27/arm64, Ubuntu 26.04/amd64, and Ubuntu 26.04/arm64; include binary, notices, release metadata, compatible skill manifest; verify exact checksum before publication; record closed receipt; refuse unsupported platform, unsafe destination, foreign/mismatched content, unsafe links/ownership/types.
- **Authority and side effects:** Exact local install authority bound to version, checksum, destination, receipt revision, and effect set. Allowed: owned binary/receipt publication. Forbidden: shell-profile edits, PATH mutation, network access not covered by selected exact-version source, package-manager changes, unowned replacement.
- **Failure / recovery:** Pre-publish failure leaves prior install. Confirmed binary with receipt failure is `partial` and reports exact state; uncertainty is recovery-required. No automatic rollback over unknown content.
- **Explicit non-goals:** GitHub Release publication, automatic update, package managers, signing/notarization, Windows, final RC claim.
- **Mandatory tests:** Archive manifest/checksum; clean install; equivalent reinstall; mismatch/foreign/symlink/hard-link/type/ownership/ACL/platform cases; receipt closed schema; interruption and F0–F8 applicable stages; version/provenance output.
- **Expected Evidence:** Build commands/exits, archive/entry digests, install side-effect ledger, receipt bytes/revision, no-op counts, conflict tree hashes, and target-specific Evidence deferred explicitly to T22/T24.
- **Completion criteria:** Exact supported archives install safely from clean roots with truthful version/provenance and no implicit environment mutation.
- **Risks / gates:** Distribution trust claims remain checksum integrity only. Broader trust topology requires **Human decision required**.

### T05 — Codex skill-set compatibility and first run

**Implementation checkpoint:** complete in the S2 delivery branch; see
[S2 Evidence](evidence-s2.md). The owned five-skill set and first-run compatibility
surface are implemented; broader Runtime evolution remains outside S2.

- **Objective:** First run reports binary/state/Codex compatibility and directs explicit Project setup; the complete owned five-skill set installs or upgrades without overwriting user-modified content.
- **Slice:** S2.
- **Dependencies:** T04.
- **Requirements:** FR-032–FR-036; AC-01, AC-02, AC-19; MVP-SEC-01, MVP-SEC-05, MVP-SEC-06; MVP-NFR-01, MVP-NFR-05.
- **ADRs / decisions:** ADR-0003, ADR-0007; HD-1.
- **Affected boundaries:** Codex Runtime adapter, skill-set manifest/digests, first-run CLI, install receipt references.
- **Expected implementation:** Version and verify the five thin hyphen-case skills; inspect exact ownership/digests; install the complete compatible set; refuse foreign/modified files; render PATH and `project configure` guidance without CWD inference or shell-profile mutation.
- **Authority and side effects:** Separate exact skill-install authority. Allowed: owned skill files/skill receipt. Forbidden: invoking Codex, editing runtime settings/profile, deleting unknown files, or configuring a Project.
- **Failure / recovery:** Partial skill publication reports installed/missing/conflicting digests and safe resume. Binary compatibility remains observable; no incompatible set is reported ready.
- **Explicit non-goals:** New Runtime, skill-owned business logic, automatic plugin/install permission expansion, or general Runtime discovery.
- **Mandatory tests:** Known/equivalent/older-owned/modified/foreign/missing skill matrices; binary-skill version mismatch; partial interruption; first-run clean/existing/incompatible state; thin-template/static command checks.
- **Expected Evidence:** Manifest and skill digests, ownership decisions, first-run human/JSON results, modified-file preservation hashes, and zero Project/Codex invocation ledger.
- **Completion criteria:** Compatibility is detectable, install is idempotent for owned content, and first run leads to explicit setup without ambient identity.
- **Risks / gates:** Host invocation syntax remains validated Codex contract; changing skill naming or adding another Runtime requires separate scope.

### T06 — Guided/non-interactive Project setup preview

**Implementation checkpoint:** complete in the S2 delivery branch; see
[S2 Evidence](evidence-s2.md). Preview remains read-only and does not derive
Project identity or capability from CWD or Git.

- **Objective:** Complete flags or a missing-only interview produce the same validated Project/local proposal and a separated safe preview before any write.
- **Slice:** S2.
- **Dependencies:** T01, T05.
- **Requirements:** FR-001–FR-007, FR-030, FR-035; AC-02–AC-05; MVP-SEC-01–MVP-SEC-03, MVP-SEC-07; MVP-NFR-01, MVP-NFR-02; SEC-001, SEC-002.
- **ADRs / decisions:** ADR-0001, ADR-0003, ADR-0004; HD-2.
- **Affected boundaries:** Project configuration use case, guided input, portable/local proposal, provider capability declaration, preview/authority request.
- **Expected implementation:** Accept explicit Project selector/new slug/name, zero or more Repository declarations and independent absolute bindings, and explicit Work Item capability. Reuse supplied values; ask only missing/materially ambiguous facts; materialize complete proposed state; show portable/local destinations, revisions, diffs, capabilities, effects, and preview digest.
- **Authority and side effects:** Preview is read-only. Tests deny filesystem writes, Provider/network/Git/process/secret reads. CWD and Git remotes never become identity or default capability.
- **Failure / recovery:** Invalid/unknown/conflicting input is `validation_failure`; cancellation/denial has zero effects; unsupported/missing capability remains explicit and blocks only the Work Item journey.
- **Explicit non-goals:** Publication, GitHub authentication, repository cloning, credential resolution, Runtime invocation, or universal Provider configuration.
- **Mandatory tests:** Guided/full-input equivalence; multi-Repository unrelated paths/providers; missing-only question counts; duplicate/conflicting selectors; absent/unsupported capability; cancellation; portable/local secret/path sentinel separation; unrelated CWD black-box.
- **Expected Evidence:** Proposal/diff goldens, prompt ledger, denied-side-effect ledger, stable issue order, and proof each input reaches existing domain/application validation rather than CLI rules.
- **Completion criteria:** One complete validated preview exists before mutation; the same facts yield equivalent application behavior regardless of input mode.
- **Risks / gates:** A new portable field or ownership meaning must follow ADR-0004 compatibility and human review.

### T07 — Authorized Project setup publication

**Implementation checkpoint:** complete in the S2 delivery branch; see
[S2 Evidence](evidence-s2.md). Portable and local publication remain separately
truthful; this does not authorize T08 or any other S3 work.

- **Objective:** Confirmed Project setup publishes complete portable intent and ID-addressed local bindings, then resolves from unrelated CWD; equivalent replay is a no-op.
- **Slice:** S2.
- **Dependencies:** T03, T06.
- **Requirements:** FR-001–FR-007, FR-022–FR-026, FR-030, FR-033, FR-035; AC-02–AC-05, AC-17; MVP-SEC-01, MVP-SEC-06–MVP-SEC-08; MVP-NFR-01, MVP-NFR-04–MVP-NFR-06; SEC-001–SEC-005.
- **ADRs / decisions:** ADR-0001, ADR-0003–ADR-0007; HD-2, HD-3.
- **Affected boundaries:** Project configure CLI/application, manifest and installation stores, native roots, resolver, shared publication protocol.
- **Expected implementation:** Bind authority to T06 preview/revisions/effects; revalidate and publish complete portable/local generations in truthful order; preserve independent Repository paths; resolve by UUID/slug; report capability readiness separately from portable validity.
- **Authority and side effects:** Allowed only exact portable/local objects. Forbidden: associated Repository writes, Git effects, Provider mutation, credential access, shell-profile edits, unknown cleanup.
- **Failure / recovery:** Stale authority/revision yields `denied_authority`; portable commit plus local failure is `partial` with recovery action; ambiguity is recovery-required; no false cross-root transaction or rollback.
- **Explicit non-goals:** Full Specification 002 remaining DAG, slug alias service, Git backing/sync, Provider connection, workflow start.
- **Mandatory tests:** Create/no-op/conflict; multi-Repository separation; concurrent setup; portable/local fault stages; source/binding replacement; unsafe roots/links/ownership; portable hashes on local failure; unrelated-CWD resolve.
- **Expected Evidence:** Application integration and CLI black-box commands/exits, revisions/digests, exact writes, no-op counts, outside-target hashes, process-race outcomes, and native obligation carried to T22.
- **Completion criteria:** Explicit setup is review-gated, idempotent, complete, separately truthful across roots, and immediately resolvable without CWD.
- **Risks / gates:** Cross-root effects remain separate commits. A proposal for implicit transactionality or repository ownership requires **Human decision required**.

### T08 — Intent interview and structured Work Item draft

**Implementation checkpoint:** complete in the S3 delivery branch; see
[S3 Evidence](evidence-s3.md). Preview remains read-only and binds the normalized
draft, target, effects, expected local revision, correlation, and provenance.

- **Objective:** Intent plus supplied facts becomes a bounded provider-neutral draft whose complete normalized form is reviewed before external mutation.
- **Slice:** S3 — Intent to GitHub Work Item.
- **Dependencies:** T01, T07.
- **Requirements:** FR-008–FR-012; AC-06, AC-07, AC-16; MVP-SEC-02, MVP-SEC-03; MVP-NFR-01–MVP-NFR-03.
- **ADRs / decisions:** ADR-0002, ADR-0003; preserve Provider capability boundary.
- **Affected boundaries:** Work Item draft values/use case, interview, GitHub renderer seam, provenance attribution, completion result.
- **Expected implementation:** Capture problem, desired outcome, context, scope, constraints, non-goals, and acceptance expectations; mark supplied versus Axiom-authored sections; ask only material missing questions; render deterministic bounded preview, target, effect set, and digest.
- **Authority and side effects:** Read-only. No GitHub/Provider mutation, local linkage write, network/process execution, or automatic Provider fallback.
- **Failure / recovery:** Incomplete/cancelled input returns actionable non-success with zero Provider effects; untrusted content remains escaped data; oversized input fails before rendering/mutation.
- **Explicit non-goals:** General Specification generator, universal Provider CRUD, autonomous scope decisions, or raw chat retention.
- **Mandatory tests:** Field completeness and question minimization; supplied-field reuse; deterministic ordering; injection/metacharacter payloads; secret/sanitization and bounds; authorship/provenance separation; unsupported capability.
- **Expected Evidence:** Unit/application transcript fixtures containing only synthetic data, question/effect ledger, normalized draft digest, output sizes, and zero-side-effect spies.
- **Completion criteria:** Reviewable structured draft is complete, bounded, provider-neutral, provenance-correct, and no mutation occurs.
- **Risks / gates:** Model-assisted text may propose content but cannot supply authority or bypass deterministic completeness/bounds.

### T09 — Authorized GitHub Work Item create/select and linkage

**Implementation checkpoint:** implementation, deterministic tests, and the
separately authorized bounded real GitHub observation are complete in the S3
delivery branch; see [S3 Evidence](evidence-s3.md). Issue #90 records the
historical observation. Current PR review corrections remain subject to human
review and do not imply human acceptance or authorize T10/S4+.

- **Objective:** Exact authority creates or selects one GitHub Issue, validates its identity/state, and persists one exact local Work Item reference; denial/cancel is zero-effect and confirmed external/local failure is truthful `partial`.
- **Slice:** S3.
- **Dependencies:** T02, T03, T08.
- **Requirements:** FR-008–FR-012, FR-017, FR-023; AC-06, AC-07, AC-16, AC-20; MVP-SEC-01–MVP-SEC-05; MVP-NFR-01–MVP-NFR-03, MVP-NFR-06; SEC-001, SEC-002, SEC-004.
- **ADRs / decisions:** ADR-0003, ADR-0006, ADR-0007; Provider != domain authority.
- **Affected boundaries:** workflow-specific WorkItemCapability port, GitHub Issues adapter, local link store, external-effect correlation, completion/artifact output.
- **Expected implementation:** Bind authority to complete draft/target/effects; durably fence the target before one bounded time-limited create; strictly validate response/reference/state; persist link with expected revision and provider/resource/external identity; after an ambiguous create, reconcile the persisted correlation without another mutation.
- **Authority and side effects:** Separate explicit GitHub mutation authority. Allowed: one reviewed Issue create/update effect and exact local linkage. Forbidden: repository/Git mutation, issue close, labels/comments, arbitrary command interpolation, blind create retry, credential persistence.
- **Failure / recovery:** Unauthenticated/rate-limited/unavailable maps to truthful effect knowledge; invalid response fails closed; confirmed create plus local failure is `partial` with Issue reference; ambiguous create persists across processes and permits reconciliation only until resolved.
- **Explicit non-goals:** Workflow projection, issue closure as acceptance, generic provider framework, second Provider.
- **Mandatory tests:** Fake-provider side-effect ledger; denial/cancel; strict responses; time/output bounds; shell metacharacters; rate-limit/transient/ambiguous cases; confirmed-effect/local-fault injection; duplicate prevention; one separately authorized bounded real GitHub observation.
- **Expected Evidence:** Unit/application/adapter results, exact sanitized commands or API requests, response validation, external/local effect ledger, Issue reference, local revision/digest, limitation and cleanup ownership.
- **Completion criteria:** One reviewed Intent reaches one truthful linked Work Item without duplicate mutation or GitHub leakage into application semantics.
- **Risks / gates:** Real GitHub mutation requires per-run human authority and public-safe content. Task completion never closes the Issue or implies human acceptance.

### T10 — Minimal Execution start, status, and resume authority

**Implementation checkpoint:** deterministic implementation complete in the S4
delivery branch; see [S4 Evidence](evidence-s4.md). This records technical
completion, not human acceptance.

- **Objective:** A linked Work Item starts one stable machine-local Execution and status/resume reads only committed local truth.
- **Slice:** S4 — Workflow truth and Provider projection.
- **Dependencies:** T03, T09.
- **Requirements:** FR-013, FR-016, FR-022–FR-026; AC-09; MVP-SEC-01, MVP-SEC-06, MVP-SEC-08; MVP-NFR-01, MVP-NFR-03, MVP-NFR-06; SEC-002–SEC-005.
- **ADRs / decisions:** ADR-0003, ADR-0005–ADR-0008.
- **Affected boundaries:** minimal Execution record/codec/store, workflow start/status/resume use cases, artifact/Evidence references, completion correlation.
- **Expected implementation:** Allocate opaque stable Execution ID; persist Project ID, optional Repository key, exact Work Item reference, Runtime ID, workflow/format version, stage, expected revision, bounded transitions, timestamps, provenance, references, terminal facts; refuse unsupported/malformed state.
- **Authority and side effects:** Exact local create/update authority. No Provider projection, external completion, portable/Repository write, or Runtime ownership. Resume cannot infer from chat or Provider metadata.
- **Failure / recovery:** Duplicate equivalent start returns existing lineage; conflicting scope/version fails; interrupted/uncertain publication is recovery-required; stale revision cannot resume/transition.
- **Explicit non-goals:** Generic Execution graph, Agent identity/plan, scheduler, child executions, unbounded event sourcing, POC in-place migration.
- **Mandatory tests:** Identity stability; closed codec/version/bounds; create/idempotent/conflict; stale revision; malformed/newer/older; interruption/resume; portable exclusion; artifact correlation without identity change.
- **Expected Evidence:** Unit/application/local-store tests, record revisions/digests, process exits, no-Provider ledger, and static check excluding graph/agent ownership concepts.
- **Completion criteria:** Start/status/resume use one stable, versioned, bounded local authority and cannot be reconstructed from external state.
- **Risks / gates:** Identity, authority, lifecycle, or cross-boundary semantic changes require **Human decision required**; encoding/path changes alone do not.

### T11 — Sequential workflow transition truth

**Implementation checkpoint:** deterministic implementation complete in the S4
delivery branch; see [S4 Evidence](evidence-s4.md). This records technical
completion, not human acceptance.

- **Objective:** One Execution advances only the approved sequential gate from an exact revision, records bounded outcome/references, and preserves truthful terminal state.
- **Slice:** S4.
- **Dependencies:** T10.
- **Requirements:** FR-013, FR-016, FR-022–FR-025; AC-09, AC-12; MVP-SEC-01, MVP-SEC-03; MVP-NFR-01–MVP-NFR-03, MVP-NFR-06; SEC-002, SEC-004, SEC-005.
- **ADRs / decisions:** ADR-0003, ADR-0006–ADR-0008.
- **Affected boundaries:** workflow transition application, Execution transition history, artifact/Evidence reference validation, completion classification.
- **Expected implementation:** Enforce intake through completion gate list; represent clarification pass without mandatory artifact; validate current stage, expected revision, bounded outcome, authority facts, provenance, references, next action; persist local transition before any projection.
- **Authority and side effects:** Local workflow-transition authority only. No GitHub label/comment/closure, Repository write, or Runtime inference. Referenced artifacts/Evidence are read/validated, not blindly trusted.
- **Failure / recovery:** Failed/interrupted transition does not advance; confirmed transition survives later rendering/artifact/projection failure; already-completed transition is idempotent or explicit conflict according to exact revision/effect identity.
- **Explicit non-goals:** DAG/multi-agent orchestration, automatic Plan/Task acceptance, Provider-owned gates, raw model transcript.
- **Mandatory tests:** Gate/transition table; wrong/skipped/duplicate/stale transitions; clarification no-artifact pass; bounded history/references; terminal status/effects; interrupted/resume; post-commit renderer/artifact failure.
- **Expected Evidence:** Unit/application matrices mapping each gate/revision/outcome to state digest, completion result, references, and denied external effects.
- **Completion criteria:** Local Execution is the sole transition truth and no failure or secondary effect advances/rewinds it incorrectly.
- **Risks / gates:** Changing gate semantics or adding graph behavior requires human-approved scope.

### T12 — Idempotent GitHub stage/comment projection

**Implementation checkpoint:** implementation, deterministic fake-Provider
Evidence, and the mandatory authorized real projection observation are complete
in the S4 delivery branch; see [S4 Evidence](evidence-s4.md). This records
technical completion, not human acceptance.

- **Objective:** Explicit authority reconciles one committed transition into exactly one `axiom:stage:<stage>` label and at most one semantically unique bounded transition comment.
- **Slice:** S4.
- **Dependencies:** T11.
- **Requirements:** FR-013–FR-017; AC-08, AC-09, AC-20; MVP-SEC-01–MVP-SEC-05; MVP-NFR-01–MVP-NFR-03, MVP-NFR-06; SEC-001, SEC-002, SEC-004.
- **ADRs / decisions:** ADR-0003, ADR-0006–ADR-0008.
- **Affected boundaries:** projection plan, GitHub label/comment adapter, projection keys/bookkeeping, completion/detail artifact.
- **Expected implementation:** Build projection from committed Execution revision; inspect current Provider state; remove only obsolete Axiom stage labels; preserve non-Axiom content; validate responses; post bounded provenance-marked comment with stage/outcome/references/next; persist intended/confirmed effects.
- **Authority and side effects:** Exact GitHub label/comment authority bound to Work Item, Execution revision, projection key, preview, and effects. Forbidden: Issue closure, non-Axiom label removal, local-stage mutation from Provider state, duplicate comment, blind retry.
- **Failure / recovery:** Provider error after local commit is `partial` or `retryable_failure` by confirmed effects; ambiguous response triggers read/reconcile; stale local revision is denied; Provider state never repairs local truth.
- **Explicit non-goals:** Provider as workflow source, generic status taxonomy, webhooks, bidirectional sync, second Provider.
- **Mandatory tests:** Projection-plan unit tests; fake GitHub label/comment ledger; stale/duplicate/replay/ambiguous/invalid response; non-Axiom preservation; confirmed comment plus bookkeeping failure; bounded output/injection; authorized real projection observation.
- **Expected Evidence:** Before/after Provider snapshots, projection keys, request/response validation, exact local revision, side-effect ledger, comment sizes/digests, and no-duplicate replay proof.
- **Completion criteria:** Reconciliation converges without advancing local truth or duplicating Axiom projection effects.
- **Risks / gates:** Append-only comments cannot roll back; Evidence must preserve confirmed effect and safe next action.

### T13 — Workflow interruption, concurrency, and projection convergence

**Implementation checkpoint:** deterministic concurrency, interruption, and
projection convergence implementation plus the real-provider observation
inherited from T12 are complete in the S4 delivery branch; see
[S4 Evidence](evidence-s4.md). Human acceptance is not inferred.

- **Objective:** Two processes, interruption, retry, and projection/local failures converge on one committed Execution revision and truthful Provider projection.
- **Slice:** S4.
- **Dependencies:** T12.
- **Requirements:** FR-013–FR-017, FR-022–FR-025; AC-08, AC-09, AC-17, AC-20; MVP-SEC-01, MVP-SEC-04, MVP-SEC-06, MVP-SEC-08; MVP-NFR-01, MVP-NFR-03, MVP-NFR-06; SEC-002–SEC-005.
- **ADRs / decisions:** ADR-0005, ADR-0007, ADR-0008; HD-3.
- **Affected boundaries:** Execution store, projection bookkeeping, GitHub adapter, completion classification, recovery observation.
- **Expected implementation:** Integrate transition and projection through separate commit/effect phases; add deterministic barriers/fault seams; preserve one winning revision, stable projection key, confirmed-effect ledger, and resumable reconciliation.
- **Authority and side effects:** Tests use fakes/isolated state by default. Any real Provider observation needs exact per-run authority. No sleep-based race proof, duplicate mutation, or automatic recovery cleanup.
- **Failure / recovery:** Cover interruption before/after local commit, before/after Provider effect, before bookkeeping, and during cleanup; ambiguity remains recovery-required or reconcile-first; local truth never follows a later Provider marker.
- **Explicit non-goals:** Distributed transaction, remote lock, exactly-once transport claim, malicious same-UID arbitrary interleavings.
- **Mandatory tests:** Two-process barrier races; stale authority; cancellation; F0–F8 relevant stages; Provider unavailable/rate-limited/invalid/ambiguous; confirmed Provider/local failure; replay after crash; no mixed reader.
- **Expected Evidence:** Deterministic stage table with revisions, process exits, Provider/local ledgers, state/artifact digests, retry boundary, and native obligation carried to T22.
- **Completion criteria:** All supported interruption/concurrency paths preserve one authoritative local revision and idempotent reconcilable projection.
- **Risks / gates:** Claims are bounded to supported local processes and Provider observation; no physical/distributed durability claim.

### T14 — Strict CLI selector path

**Implementation checkpoint:** deterministic application, parser, CLI, and
executable black-box coverage is complete in the S5 delivery branch; see
[S5 Evidence](evidence-s5.md). This includes unsupported single-hyphen duplicate
flags and a protected-state fixture proving ambiguous Project slug rejection with
zero store/Provider/Runtime effects. Human acceptance is not inferred.

- **Objective:** Fully specified CLI selectors reach the correct Project/Repository/Work Item/Execution without questions; partial input asks only missing/ambiguous values and invalid input never falls back to CWD.
- **Slice:** S5 — Codex selector path and completion convergence.
- **Dependencies:** T13.
- **Requirements:** FR-018–FR-021; AC-10, AC-11; MVP-SEC-03, MVP-SEC-05; MVP-NFR-01, MVP-NFR-02.
- **ADRs / decisions:** ADR-0001, ADR-0003, ADR-0008.
- **Affected boundaries:** CLI parsing, selector validation/resolution, guided input adapter, operation-shaped application dispatch.
- **Expected implementation:** Accept exact Project UUID/slug, Project-scoped Repository key, linked Work Item reference, and known operation inputs; reject unknown/duplicate/conflicting keys; reuse valid values; dispatch explicit selectors to existing use cases.
- **Authority and side effects:** Selector validation is read-only. Mutation commands still require their own exact authority; a valid selector grants none. No Git remote/CWD/provider fallback.
- **Failure / recovery:** Invalid/ambiguous/missing closed-mode input is deterministic `validation_failure`; no store/Provider/Runtime side effects occur before complete resolution.
- **Explicit non-goals:** General query language, global repository discovery, command alias compatibility promises, domain rules in presentation.
- **Mandatory tests:** Full/partial/ambiguous/unknown/duplicate/conflict matrix; unrelated CWD; slug/UUID equivalence; Repository scoping; Work Item exact reference; zero-question fully supplied path; denied-side-effect spies.
- **Expected Evidence:** Parser/application unit tests and CLI black-box commands/exits, prompt counts, resolved references, safe output, and zero-effect ledger.
- **Completion criteria:** CLI selection is strict, explicit, missing-only, and semantically identical to direct application invocation.
- **Risks / gates:** New selector meanings that collapse Project/Repository/Provider identities require review.

### T15 — Thin Codex selector path and completion convergence

**Implementation checkpoint:** skill-set v2, deterministic compatibility/thin-
adapter checks, seven-status/detail/provenance regression, and bounded real Codex
Runtime observation are complete in the S5 delivery branch; see
[S5 Evidence](evidence-s5.md). The seven-status test consumes central completion
results through the real CLI JSON renderer and validates every canonical field at
the Codex-facing contract. Human acceptance is not inferred.

- **Objective:** Installed Codex skills pass explicit selectors to Lingo and produce the same semantic result, provenance, detail reference, and all seven terminal statuses as direct CLI use.
- **Slice:** S5.
- **Dependencies:** T02, T05, T13, T14.
- **Requirements:** FR-018–FR-021, FR-036; AC-10–AC-16; MVP-SEC-02, MVP-SEC-03, MVP-SEC-05; MVP-NFR-01–MVP-NFR-03, MVP-NFR-05, MVP-NFR-06; SEC-001, SEC-002.
- **ADRs / decisions:** ADR-0003, ADR-0006, ADR-0008.
- **Affected boundaries:** five Codex skills, host argument adaptation, Lingo CLI/JSON, completion/detail/provenance renderers.
- **Expected implementation:** Evolve skills only for existing operations; accept/reuse selectors; ask missing-only host questions; pass explicit flags; render canonical result; reference artifacts without copying details or duplicating business rules.
- **Authority and side effects:** Skill text grants no authority. Tests may invoke installed skills in an isolated Codex environment; external GitHub mutations use fakes except separately authorized bounded observation in T24.
- **Failure / recovery:** Host/skill incompatibility fails before operation; invalid selectors never trigger fallback; interrupted/partial/retryable results preserve exact effects and next action.
- **Explicit non-goals:** Runtime-owned workflow, second Runtime, literal colon skill names, generic agent graph, automatic permission/config changes.
- **Mandatory tests:** Host contract discovery/invocation; fully supplied zero-question and partial missing-only; unknown/conflicting arguments; CLI/skill seven-status semantic matrix; detail lookup; provenance/authorship; incompatible skill set.
- **Expected Evidence:** Installed skill digests/version, real bounded Codex discovery/invocation, CLI/Runtime normalized-result comparison, prompts, exits, references, and limitations.
- **Completion criteria:** Skills are thin deterministic adapters and all observed semantics converge with CLI/application truth.
- **Risks / gates:** Real Codex invocation is required Evidence but cannot substitute for deterministic tests or grant Provider authority.

### T26 — Deterministic Work Item lifecycle projection from canonical gates

**Implementation status:** explicitly authorized on 2026-09-24 and technically
complete at `56beb4fc310894ff8de128f52c6a96d22711bec8`; human acceptance is not inferred.

- **Objective:** Derive exactly one provider-neutral Work Item lifecycle stage from the canonical Execution gate/history and required revisioned human facts, without persisting a second lifecycle state.
- **Slice:** S6 — Durable Work Item lifecycle and metadata governance.
- **Dependencies:** T15.
- **Requirements:** FR-013, FR-016, FR-038–FR-041; AC-09, AC-25, AC-26, AC-29; MVP-SEC-01, MVP-SEC-03; MVP-NFR-01, MVP-NFR-06; SEC-002, SEC-004, SEC-005.
- **ADRs / decisions:** ADR-0003, ADR-0006–ADR-0008. Preserve one Execution lineage and local workflow authority.
- **Affected boundaries:** provider-neutral derived lifecycle value, canonical gate/fact mapping, Execution transition references, completion and acceptance facts.
- **Expected implementation:** Preserve S4's closed gates as the sole state machine; implement the Specification's total derivation table; validate revisioned planning authority, implementation authority, review start, and terminal human acceptance facts; keep blocked/needs-decision/needs-approval/recovery-required as separate conditions; add no independent lifecycle field or transition history.
- **Authority and side effects:** Derivation is read-only. Existing canonical gate operations retain their exact local authority. Recording human acceptance requires exact local decision authority against a terminally completed Execution. No Provider mutation, PR/Issue close, Repository write, or inferred human decision. A valid selector, merge, CI, review, or Provider state grants no gate or acceptance authority.
- **Failure / recovery:** Expected absent start facts at `plan`, `implementation`, and `review` derive `specified`, `planned`, and `implemented`. A valid `blocked` condition remains independent and does not change the derived stage or return `recovery_required`; when it applies to the next canonical transition, that transition is denied before mutation, the gate remains unchanged, and no authority is inferred. Stale, incompatible, skipped, contradictory, unknown, insufficient, reference-missing, out-of-order, or scope-mismatched facts return `recovery_required` before local or Provider mutation. A confirmed canonical transition survives later derivation/projection/rendering failure.
- **Explicit non-goals:** General workflow engine, custom stages, second Execution lineage, graph/orchestration behavior, or Provider-owned gates.
- **Mandatory tests:** Complete gate/fact-to-stage table; intentional absent-fact boundaries; valid blocked snapshots at every stage; blocked-transition denial with unchanged gate/revision and zero effects; every inconsistent prerequisite combination; stale/skipped/duplicate/conflicting canonical transition; independent auxiliary-condition matrix; terminal explicit acceptance decision; merge/CI/closure/Provider non-authority; old-record compatibility and replay.
- **Expected Evidence:** Gate/history/fact/revision-to-stage matrix, prerequisite/authority facts, zero-effect ledgers for every inconsistent case, canonical transition references, and proof S4 records/Evidence remain unchanged.
- **Completion criteria:** Every valid Execution snapshot derives exactly one stage without a lifecycle write; inconsistent snapshots fail closed; valid canonical transitions commit once; external technical signals advance nothing.
- **Risks / gates:** Any change to Execution identity/lifecycle, source-of-truth ownership, or general stage configurability requires **Human decision required** and renewed ADR assessment.

### T27 — GitHub lifecycle/flag projection and bounded history

- **Objective:** Reconcile the stage derived from committed canonical Execution truth to exactly one GitHub stage label, applicable independent flags, and one bounded idempotent transition comment without changing local authority.
- **Slice:** S6.
- **Dependencies:** T26.
- **Requirements:** FR-013–FR-017, FR-038–FR-042; AC-08, AC-09, AC-25–AC-29; MVP-SEC-01–MVP-SEC-05; MVP-NFR-01–MVP-NFR-03, MVP-NFR-06; SEC-001, SEC-002, SEC-004.
- **ADRs / decisions:** ADR-0003, ADR-0006–ADR-0008. Extend S4 projection; do not rewrite it.
- **Affected boundaries:** projection plan/observation, GitHub label/comment adapter, auxiliary flags, legacy S4-label recognition, intended/confirmed ledger.
- **Expected implementation:** Project the exact ten-label namespace and four independent flags; detect zero/multiple/unknown/contradictory markers; reuse exact preview/authority/reinspection; post reference-first comments under the existing 16 KiB and 16-reference limits; replace only positively identified legacy S4 labels under reviewed effects.
- **Authority and side effects:** Separate exact GitHub authority bound to Work Item, canonical Execution revision and derived stage, projection key, observation digest, preview digest, and ordered effects. Forbidden: Issue closure, foreign-content removal, unknown namespaced-label deletion, duplicate comment, or local transition from Provider state.
- **Failure / recovery:** Drift or unknown namespaced content is `recovery_required`; ambiguous effects are reinspected; confirmed effect plus bookkeeping failure is truthful `partial`; replay converges without duplication.
- **Explicit non-goals:** Webhooks, bidirectional sync, arbitrary labels/stages, general Provider framework, or dense artifact publication in comments.
- **Mandatory tests:** Exact label enum; exactly-one-stage matrix; all flag combinations independent of stage; foreign/unknown preservation; legacy S4 replacement preview; bounded/reference-only comment; replay/ambiguous/partial/stale-authority cases; bounded real GitHub observation only under separate per-run authority.
- **Expected Evidence:** Before/preview/after/replay snapshots, exact effect ledger, comment bytes/digest/references, projection keys, foreign-content hashes, and historical S4 non-mutation statement.
- **Completion criteria:** A conforming managed Issue converges to one lifecycle marker and correct flags/history; drift never advances local truth or causes unreviewed deletion.
- **Risks / gates:** Provider comments remain append-only. Unknown Axiom-like metadata is preserved for human decision rather than normalized heuristically.

### T28 — Project metadata policy for Work Items and Pull Requests

- **Objective:** Resolve required/default Work Item and Pull Request metadata from explicit inputs, Project policy, and validated context, prompting only for unresolved mandatory values and keeping concrete GitHub fields in the adapter.
- **Slice:** S6.
- **Dependencies:** T07, T09, T15, T27.
- **Requirements:** FR-003, FR-006, FR-011, FR-012, FR-044; AC-04–AC-07, AC-31; MVP-SEC-01–MVP-SEC-05, MVP-SEC-07; MVP-NFR-01, MVP-NFR-02, MVP-NFR-06; SEC-001, SEC-002.
- **ADRs / decisions:** ADR-0001, ADR-0003, ADR-0004, ADR-0007. Project intent remains portable; Provider observation remains local.
- **Affected boundaries:** Project metadata-policy contract, capability resolution, Work Item/PR operation previews, GitHub issue/PR adapter effects, guided missing-input collection.
- **Expected implementation:** Use Specification 002 `schemaVersion: 1`'s existing `policies` references without adding manifest fields; validate one closed `policyVersion: 1`, `kind: work-item-metadata` document with the bounded Work Item/PR intention sets; reject duplicate/unknown/unsupported policy forms without migration; produce resolved/optional/required-missing/unsupported results; apply deterministic precedence; preview exact sources/effects; let GitHub map labels, assignee, milestone, supported Project/status, and PR labels/assignee/review metadata.
- **Authority and side effects:** Resolution is read-only. Any Work Item/PR mutation retains a separate exact Provider authority. Policy never grants review approval, merge, Issue closure, or workflow acceptance.
- **Failure / recovery:** Unsupported optional capability is explicit; unsupported or missing mandatory capability/value blocks before mutation; changed Provider/Project observation invalidates authority; partial confirmed effects remain inspectable/reconcilable.
- **Explicit non-goals:** Generic custom fields, universal metadata CRUD, sophisticated GitHub Projects automation, second Provider implementation, credential storage, or automatic reviewer approval.
- **Mandatory tests:** Unchanged closed `axiom.yaml` v1 schema; absent/unconfigured/empty/one-policy references; policy version/kind/closed-field/duplicate-kind validation; resolution precedence; required/default/optional/unsupported matrix; zero-question fully resolved path; missing-only prompt counts; Work Item versus PR capability separation; adapter field isolation; stale preview, denial, partial, and no-effect cases.
- **Expected Evidence:** Policy/result fixtures, prompt and capability ledgers, exact GitHub-effect previews through fakes, zero-effect denial cases, portable/local separation hashes, and no GitHub concepts in domain contracts.
- **Completion criteria:** Deterministically resolvable metadata needs no question; unresolved mandatory metadata blocks precisely; GitHub-specific fields remain adapter-owned.
- **Risks / gates:** A generic metadata/custom-field schema or portable Provider observation requires **Human decision required** and renewed ADR assessment.

### T29 — Missing-local-state reconciliation and Slice convergence

- **Objective:** Inspect Provider projection/history, Repository artifacts, and available local generations and return either current truth, one exact ADR-0007 recovery plan, or `recovery_required` with human decision—never a synthesized Execution or transition.
- **Slice:** S6.
- **Dependencies:** T03, T13, T27, T28.
- **Requirements:** FR-017, FR-022–FR-026, FR-038, FR-043, FR-044; AC-17, AC-20, AC-27, AC-28, AC-30, AC-31; MVP-SEC-01–MVP-SEC-04, MVP-SEC-06, MVP-SEC-08; MVP-NFR-01, MVP-NFR-03, MVP-NFR-04, MVP-NFR-06; SEC-002–SEC-005.
- **ADRs / decisions:** ADR-0005–ADR-0008; Provider is a recovery signal, not workflow authority.
- **Affected boundaries:** recovery inspector/result, local canonical/prior/stage records, Repository reference validation, Provider projection/history observation, Slice Evidence.
- **Expected implementation:** Correlate exact Work Item/Execution/projection identities, revisions, digests, lifecycle markers, flags, comments, and artifact references; classify aligned current truth, validated local-generation recovery, insufficient evidence, and contradiction; require fresh exact authority for any ADR-0007 recovery mutation.
- **Authority and side effects:** Inspection is read-only. Recovery mutation remains separately authorized and local-only. Provider/Repository observations cannot create state, replay work, clear blockers, or grant implementation/acceptance authority.
- **Failure / recovery:** Missing canonical state plus only Provider/Repository signals is `recovery_required`; a validated owned prior/new generation yields a recovery plan, not automatic mutation; contradictions/unknown content are preserved for review.
- **Explicit non-goals:** Silent Execution reconstruction, heuristic stage selection, remote workflow truth, automatic recovery/cleanup, or broad migration.
- **Mandatory tests:** Current-local/aligned projection; absent local with Provider only; validated prior/new generation; multiple/unknown labels; missing/changed artifacts; contradictory history; stale recovery authority; zero-write inspection; full T26–T28 convergence and regression.
- **Expected Evidence:** Input-source/digest matrix, reconciliation classifications, proposed exact recovery effects, tree hashes proving read-only inspection, human-decision outputs, full repository validation, and bounded fake/real Provider separation.
- **Completion criteria:** Every supported evidence combination yields a deterministic non-invented result; no missing/contradictory state advances workflow; S6 contracts converge end to end.
- **Risks / gates:** If safe recovery requires reconstructing Execution identity/history from external data, stop with **Human decision required**; that would change ADR-0008.

### T16 — Compatibility inspection and historical POC classification

- **Objective:** Read-only inspection distinguishes absent v1, valid v1, recognized POC, malformed, unsupported older, unsupported newer, and recovery-required state with actionable next steps.
- **Slice:** S7 — Recovery, cleanup and upgrade.
- **Dependencies:** T29.
- **Requirements:** FR-026–FR-030, FR-032, FR-036; AC-18, AC-19; MVP-SEC-02, MVP-SEC-06–MVP-SEC-08; MVP-NFR-01, MVP-NFR-03–MVP-NFR-05; SEC-001, SEC-003–SEC-005.
- **ADRs / decisions:** ADR-0004–ADR-0008; HD-4.
- **Affected boundaries:** compatibility inspector, v1/POC signatures, installation/Project/Work Item/workflow/skill readers, completion/detail output.
- **Expected implementation:** Define complete positive POC signatures from the merged historical revision; inspect exact roots/categories without secrets; validate closed versions and recovery markers; report separate clean-install/export/reconfigure/backup options.
- **Authority and side effects:** Strictly read-only. No overwrite, migration, backup, export, install, cleanup, or implicit treatment of uncertainty as absence.
- **Failure / recovery:** Partial or ambiguous signatures are preserved/review-required; malformed/newer/older remains distinct; recovery marker wins over ordinary read.
- **Explicit non-goals:** In-place POC migration, arbitrary historical compatibility, downgrade, generic N-1 support.
- **Mandatory tests:** Golden fixtures for every classification; partial/unknown/mixed POC-v1 roots; size/type/link/ownership/ACL faults; sentinel sanitization; zero-write tree hash.
- **Expected Evidence:** Classification matrix, observed format/signature facts, exact zero-write ledger, safe output/detail digest, and fixture provenance to historical POC revision.
- **Completion criteria:** No incompatible or uncertain state is overwritten or reported absent; each supported classification has a safe next action.
- **Risks / gates:** Any proposed in-place POC migration requires **Human decision required** and a separate compatibility/rollback decision.

### T17 — Authorized POC backup/export/reconfigure path

- **Objective:** Recognized POC state can be backed up locally, portable Project intent exported to an empty target, and clean v1 reconfiguration guided without deleting or migrating POC workflow/Execution history.
- **Slice:** S7.
- **Dependencies:** T16.
- **Requirements:** FR-027–FR-030; AC-18; MVP-SEC-01–MVP-SEC-03, MVP-SEC-06–MVP-SEC-09; MVP-NFR-03–MVP-NFR-06; SEC-001–SEC-005.
- **ADRs / decisions:** ADR-0004–ADR-0007; HD-4.
- **Affected boundaries:** backup manifest, portable export allowlist, clean-root reconfiguration guidance, artifact/completion/Evidence correlation.
- **Expected implementation:** Preview source/target/categories/digests/space/effects; require authority; write restrictive digest-manifested backup to empty target; export only validated portable content to separate empty target; verify; direct explicit clean v1 setup.
- **Authority and side effects:** Separate exact backup/export authorities. Allowed: new owned objects in explicit empty targets. Forbidden: source mutation/deletion, credential/local/workflow/Execution export into portable intent, in-place migration, overwrite, network/Provider/Git effects.
- **Failure / recovery:** Partial backup/export preserves source and reports exact complete objects; unsafe/occupied/cross-filesystem/uncertain target fails closed; retry uses expected revisions and owned manifests.
- **Explicit non-goals:** Automatic cleanup, generic migration engine, POC workflow conversion, downgrade.
- **Mandatory tests:** Allowlist; secret/local sentinel exclusion; empty-target/collision/traversal/link/ownership/ACL; ENOSPC/EDQUOT/interruption; digest manifest; stale authority; retry/no-op; post-export v1 validation.
- **Expected Evidence:** Preview/authority digest, source/target hashes, backup/export manifest, omitted categories, process exits, fault stage, permissions, and recovery action.
- **Completion criteria:** Recognized POC content is preserved and portable intent can seed explicit clean v1 reconfiguration without silent conversion.
- **Risks / gates:** Export is not migration and does not promise continuity of local Execution/Provider state.

### T18 — Reference-aware artifact cleanup and capacity recovery

- **Objective:** Operator previews and explicitly removes only eligible owned artifacts while active, Evidence-referenced, recovery-related, and uncertain content remains protected; capacity exhaustion never triggers eviction.
- **Slice:** S7.
- **Dependencies:** T02, T10, T15, T29.
- **Requirements:** FR-023, FR-025, FR-033; AC-13, AC-14, AC-17; MVP-SEC-01, MVP-SEC-02, MVP-SEC-06, MVP-SEC-08, MVP-SEC-09; MVP-NFR-01, MVP-NFR-03, MVP-NFR-06; SEC-001, SEC-003–SEC-005.
- **ADRs / decisions:** ADR-0005–ADR-0008; HD-2, HD-3.
- **Affected boundaries:** retention/reference view, cleanup preview/authority, artifact store, cleanup record, Execution/Evidence references.
- **Expected implementation:** Enforce active/evidence/diagnostic/preserved_review classes and Plan defaults; calculate eligibility from authoritative references; lock/revalidate identity/type/link/digest/revision; remove exact objects; retain bounded 64 KiB cleanup records for initial 90-day policy; expose reclaimed capacity.
- **Authority and side effects:** Separate cleanup authority bound to exact artifact IDs/revisions/digests/effects. Forbidden: recursive broad-root deletion, age-only deletion, automatic eviction, live-reference removal, unknown-content mutation.
- **Failure / recovery:** Changed reference/identity/ownership/lock denies authority; partial removal reports exact removed/preserved identities; uncertain state becomes/remains preserved_review; cleanup-record failure cannot invent successful audit state.
- **Explicit non-goals:** Permanent architectural retention values, portable artifact cleanup, legal deletion guarantees, payload retention inside audit record.
- **Mandatory tests:** Eligibility matrix; active/Evidence/cross-artifact/recovery references; 30/365/90-day policy boundaries with fake clock; stale authority; race barriers; link/replacement/ACL; cleanup-record bound; capacity exhaustion before/after cleanup; no auto-delete.
- **Expected Evidence:** Preview and authorization, reference graph snapshot, removed/preserved IDs/digests, bytes/count reclaimed, cleanup record, outside-root hashes, and measured dogfood volume/lookup cost.
- **Evidence retirement (HD-S7-T18, 2026-09-26):** The 365-day Evidence window starts only at an explicit, separately authorized retirement recorded outside `metadata.json` v1 (see Plan retention). Retirement is denied while any authoritative reference is live; a later re-reference supersedes it; missing, corrupt, stale, or other-revision retirements preserve. Mandatory tests additionally cover 365d−1s/365d/365d+1s, referenced denial, stale authority, changed revision/digest, corrupt/missing record, re-reference and re-retirement, publication interruption/recovery, and link/type/permission confinement.
- **Completion criteria:** Cleanup is explicit, exact, reference-aware, auditable, and cannot convert uncertainty or pressure into deletion authority.
- **Risks / gates:** Dogfood recommendations may change defaults only through reviewed Plan/release reconciliation, not silently during implementation.

### T19 — Guided local recovery

- **Objective:** Read-only inspection explains interrupted publication; fresh authority restores/finalizes only a recognized complete generation while ambiguous/unknown state remains preserved for operator review.
- **Slice:** S7.
- **Dependencies:** T03, T10, T18.
- **Requirements:** FR-022–FR-029; AC-17, AC-18; MVP-SEC-01, MVP-SEC-06, MVP-SEC-08, MVP-SEC-09; MVP-NFR-01, MVP-NFR-03, MVP-NFR-04, MVP-NFR-06; SEC-002–SEC-005.
- **ADRs / decisions:** ADR-0004–ADR-0008; HD-3, HD-4.
- **Affected boundaries:** recovery inspector/plan, Project/local/Execution/artifact/receipt stores, completion/detail output.
- **Expected implementation:** List canonical/prior/stage/marker identities, revisions, protocol stage, ownership, references, and proposed safe action; bind fresh authority; reacquire coordination; recheck confinement/references; restore prior pre-commit or finalize committed generation; clean exact owned protocol objects.
- **Authority and side effects:** Inspection read-only. Mutation authority is separate per exact recovery plan. Forbidden: heuristic repair, recursive deletion, synthesizing mixed state, Provider/Git effect, stale-authority retry.
- **Failure / recovery:** Contradictory/unknown/corrupt state remains preserved_review/recovery-required; failed recovery reports actual authoritative generation and residual objects; confirmed commit is never rolled back by cleanup failure.
- **Explicit non-goals:** Automatic recovery, physical media repair, arbitrary historical migration, general backup restore engine.
- **Mandatory tests:** Every F0–F8 recognized state; corrupt/unknown/contradictory marker; stale authority; concurrent recovery/reader/writer with barriers; prior/new generation selection; ownership/link/ACL/replacement; cleanup fault.
- **Expected Evidence:** Before/plan/after recovery records, generations/revisions/digests, authority digest, process exits, preserved unknown IDs, and no outside-root mutation.
- **Completion criteria:** Recognized recovery is deterministic and bounded; ambiguity never yields success, deletion, or false rollback.
- **Risks / gates:** Recovery availability may be lower by design; weakening fail-closed readers requires **Human decision required**.

### T20 — Owned install upgrade and resumable partial state

- **Objective:** Exact-version upgrade previews and verifies binary, receipt, skills, state compatibility, space, and effects; authorized publication reports each confirmed effect and safely resumes mixed owned state.
- **Slice:** S7.
- **Dependencies:** T04, T05, T16, T19.
- **Requirements:** FR-026–FR-029, FR-031–FR-037; AC-18, AC-19; MVP-SEC-01, MVP-SEC-05, MVP-SEC-06, MVP-SEC-08; MVP-NFR-01, MVP-NFR-03–MVP-NFR-07; SEC-002–SEC-005.
- **ADRs / decisions:** ADR-0003–ADR-0007; HD-1, HD-3, HD-4.
- **Affected boundaries:** upgrade inspector/plan, binary/receipt/skill publication, compatibility verification, completion/detail/Evidence.
- **Expected implementation:** Follow Plan ordering: read-only preflight; exact preview/revisions/effects; authority; stage/verify complete target; publish/re-read binary; publish receipt; publish skill files; verify clean-v1 compatibility; resume from validated owned partial state.
- **Authority and side effects:** Exact local upgrade authority per root/effect. Allowed: owned binary/receipt/skill replacement. Forbidden: auto-update, downgrade/lossy conversion, shell-profile/package-manager changes, unowned replacement, POC in-place migration, network source substitution.
- **Failure / recovery:** No cross-root transaction claim. Binary/receipt/skill outcomes are individually confirmed; later failure is `partial` with safe resume; unknown/newer/malformed/recovery state blocks before mutation.
- **Explicit non-goals:** Public release publication, automatic rollback over confirmed effects, state schema migration absent separate decision, Windows.
- **Mandatory tests:** Preflight matrix; equivalent no-op; unsupported downgrade/newer/older/POC; stale authority; checksum/space/ownership/link conflict; interruption after each ordered effect; resume; incompatible skill set; final verification.
- **Expected Evidence:** Source/target versions and checksums, preview digest, effect-by-effect ledger, receipt/skill revisions, tree hashes, faults/exits, and native target Evidence carried to T22/T24.
- **Completion criteria:** Upgrade never silently leaves or reports an unusable mixed state; owned partial state is inspectable and resumable.
- **Risks / gates:** Any mutating persisted-state migration or new trust topology requires **Human decision required**.

### T21 — Cross-slice security and bounded-I/O regression

- **Objective:** Public-safe adversarial tests prove rejection/non-effects for secrets, untrusted data, traversal/link/replacement, Provider responses, command injection, and bounded inputs/outputs across delivered slices.
- **Slice:** S7.
- **Dependencies:** T09, T13, T15, T17, T18, T20, T29.
- **Requirements:** FR-004, FR-011, FR-016, FR-020, FR-023–FR-025, FR-030, FR-032, FR-034; AC-07, AC-09, AC-11–AC-14, AC-16, AC-17, AC-20, AC-22, AC-23; MVP-SEC-01–MVP-SEC-09; MVP-NFR-01–MVP-NFR-06; SEC-001–SEC-005.
- **ADRs / decisions:** ADR-0001, ADR-0003–ADR-0008; HD-2–HD-4.
- **Affected boundaries:** all codecs, application authorities, filesystem adapters, CLI/skills, GitHub/command adapter, backup/export/cleanup/upgrade.
- **Expected implementation:** Consolidate reusable synthetic fixtures, denial spies, bounded buffers, deterministic fault hooks, process barriers, and public-safe side-effect ledgers without moving business rules into a test framework.
- **Authority and side effects:** Isolated roots and controlled fakes only. Real Provider/Runtime observations remain T24 and separately authorized. Attack strings are data, never executed. No actual secrets.
- **Failure / recovery:** Every rejected case records phase, status, zero/confirmed effects, and preserved state; scanner success is supplemental and never proof of absence.
- **Explicit non-goals:** Penetration-test claim, arbitrary same-UID defense, physical durability, production credentials, sleep-based race proof.
- **Mandatory tests:** Structural secret rejection/non-leak sentinels; sanitization/truncation; traversal/absolute/alias; symlink/hard-link/ancestor/leaf replacement; unsafe ownership/ACL/type; stale authority; shell metacharacters; strict/oversized/timeout Provider responses; confirmed external/local failure; interruption/recovery ambiguity; cleanup safety.
- **Expected Evidence:** Requirement-to-case matrix, commands/exits, denied/confirmed side-effect ledgers, before/after hashes, sentinel scan across outputs/files/Evidence, bounds, platform assumptions, and review findings.
- **Completion criteria:** Every security/NFR alias and inherited SEC requirement has executable owner Evidence; no unsupported claim is marked passed.
- **Risks / gates:** A blocker/critical/major finding must be fixed or explicitly waived before T22/T23.

### T22 — Exact-target native filesystem/install/upgrade Evidence

- **Objective:** Native runs prove applicable filesystem, concurrency, installation, cleanup, recovery, and upgrade behavior on the native row operated in S7 (macOS 27/arm64/APFS). Under HD-S7-T22 (2026-09-26) the two Ubuntu 26.04 rows remain supported targets whose native Evidence obligation moves to T24; a missing row remains an explicit RC/release blocker.
- **Slice:** S7.
- **Dependencies:** T19, T20, T21.
- **Requirements:** FR-022–FR-037; AC-01, AC-14, AC-17–AC-19, AC-22, AC-23; MVP-SEC-06, MVP-SEC-08, MVP-SEC-09; MVP-NFR-03–MVP-NFR-07; SEC-003–SEC-005.
- **ADRs / decisions:** ADR-0005–ADR-0007; HD-1, HD-3, HD-4.
- **Affected boundaries:** APFS/ext4 adapters, native roots, installer/upgrade, artifact/cleanup/recovery, Evidence capture.
- **Expected implementation:** Provide one versioned native suite that runs on every approved row; execute it on macOS 27/arm64/default case-insensitive APFS for S7; record exact point release/kernel/image/filesystem/primitives; record the Ubuntu 26.04/amd64/ext4 and Ubuntu 26.04/arm64/ext4 rows as not executed and deferred to T24 (never `pass`); preserve current macOS 15/Ubuntu 24.04 runs as historical only.
- **Authority and side effects:** Tests operate on isolated owned roots/accounts/VMs. No production state or Provider mutation. Workflow/CI changes require their own reviewed implementation scope; preview runner availability is never assumed sufficient.
- **Failure / recovery:** A failing or unavailable macOS 27 row blocks S7 completion; the deferred Ubuntu rows block RC acceptance and release claims for those targets (T24); cross-compilation cannot substitute; unsupported filesystem/ownership semantics fail closed; test leftovers remain identified and safely cleanable.
- **Explicit non-goals:** Windows, other Linux distributions/architectures/filesystems, case-sensitive APFS, network/FUSE/overlay/removable storage, physical power-loss testing.
- **Mandatory tests:** T03 F0–F8; two-process barriers; traversal/link/replacement; ownership/mode/ACL; old/new readers; artifact capacity/cleanup; recovery; clean install/reinstall; ordered upgrade/partial resume on the macOS 27/arm64/APFS row for S7; the same suite on both Ubuntu 26.04 rows under T24.
- **Expected Evidence:** Per-row immutable report with candidate revision, environment inventory, commands/exits, filesystem observations, fault/concurrency tables, hashes/references, limitations, unavailable cases, and sanitized artifacts.
- **Completion criteria (revised by HD-S7-T22):** The macOS 27/arm64/APFS row has native passing Evidence for applicable claims, and both Ubuntu 26.04 rows are recorded as not executed and deferred to T24. Deferral is not a pass: no Ubuntu behavior is claimed, and the Ubuntu obligation stays blocking for RC acceptance/release, never downgraded to a documentation note.
- **Risks / gates:** Preview runner drift/capacity needs recorded backstop/rerun path. Platform expansion requires reviewed support/Evidence change.

### T30 — Operator Runtime/Model Profile configuration and capability resolution

- **Objective:** One versioned machine-local operator configuration resolves a child capability request to exactly one installed, enabled, available and allowlisted Runtime/Model Profile or an actionable blocker.
- **Slice:** S8 — Multi-runtime agent planning and multi-agent execution.
- **Dependencies:** T22.
- **Requirements:** FR-048, FR-049, FR-054, FR-055, FR-061; AC-34, AC-39; MVP-SEC-01–MVP-SEC-06, MVP-SEC-08; MVP-NFR-01–MVP-NFR-06.
- **ADRs / decisions:** ADR-0003, ADR-0005–ADR-0007, ADR-0009.
- **Affected boundaries:** operator configuration, Runtime adapter inventory, Model Profiles, capability/availability observations, credential references, resolution result.
- **Expected implementation:** Define closed versioned configuration and request/result contracts; separate declared/installed/enabled/available/proven capability; support Codex and Claude behind consumer-owned ports; keep concrete model names inside local Model Profiles; resolve deterministically without vendor ranking or implicit fallback.
- **Authority and side effects:** Configuration mutation requires exact local authority. Resolution is read-only and never invokes a Runtime, reads credential values, expands allowlists, installs adapters or falls back.
- **Failure / recovery:** Missing/corrupt/newer config, unavailable adapter, unknown capability/profile, stale observation or no allowed match blocks with zero dispatch. Preserve prior complete configuration under ADR-0007.
- **Explicit non-goals:** Universal plugin discovery, automatic installation/purchase, portable credentials, provider-neutral price estimation, Runtime execution.
- **Mandatory tests:** Closed-schema/version matrix; allow/deny/no-match; availability/capability distinctions; stale revision; secret sentinel non-leak; two fake adapters; no-fallback and zero-process/network effects.
- **Expected Evidence:** Sanitized configuration digest/revision, inventory/observation references, resolution matrix, denial/non-effect ledger and compatibility facts for Codex and Claude.
- **Completion criteria:** Resolution is deterministic, operator-bounded, secret-free and incapable of implicit Runtime/model selection.
- **Risks / gates:** Concrete Runtime/model support and credential mechanisms require human review if they broaden approved trust or installation boundaries.

### T31 — Agent Planner proposal and deterministic DAG validation

- **Objective:** An approved Plan becomes one bounded reviewable parent/child DAG proposal derived from capabilities, dependencies, risks and validation obligations rather than a fixed team template.
- **Slice:** S8.
- **Dependencies:** T30.
- **Requirements:** FR-045, FR-046, FR-049, FR-061; AC-32, AC-33; MVP-SEC-01–MVP-SEC-03; MVP-NFR-01–MVP-NFR-03.
- **ADRs / decisions:** ADR-0009; preserve `Execution != Agent` and `Role != Model`.
- **Affected boundaries:** approved Plan input, normalized work/capability proposal, DAG nodes/edges, validation, preview and graph digest.
- **Expected implementation:** Normalize Plan-derived work, propose minimal nodes and dependencies, require integration/validation ownership, validate finite acyclic topology, scopes/effects/execution controls/capabilities, then render exact graph preview/digest before authority.
- **Authority and side effects:** Proposal/validation are read-only. Planner output grants no authority, allocates no Execution, chooses no unallowed Runtime, spawns no child and mutates no repository.
- **Failure / recovery:** Cycle, missing node/output, unresolved dependency, fixed-role assumption, unsafe overlap, absent integration owner, unsupported capability or oversized graph returns validation failure with zero effects.
- **Explicit non-goals:** Frontend/backend/QA/infra template, dynamic autonomous spawning, implementation, dispatch, model-owned approval.
- **Mandatory tests:** Representative Plan fixtures; variable topologies; cycle/orphan/overlap/bounds; deterministic ordering/digest; prompt/content injection as data; zero-effect spies.
- **Expected Evidence:** Plan fixture digest, normalized proposal, validation matrix, preview/digest and proof no Runtime/process/filesystem/Provider effects occurred.
- **Completion criteria:** Proposed topology is minimal, explainable, deterministically valid and ready for explicit review without starting work.
- **Risks / gates:** Any material graph/scope decision unresolved by approved Plan remains **Human decision required**.

### T32 — Parent/child lineage, envelopes, authority and execution-control persistence

- **Objective:** Authorized graph publication creates one revisioned parent plus bounded child Executions with immutable lineage and least-privilege envelopes, while retaining ADR-0008 sequential compatibility.
- **Slice:** S8.
- **Dependencies:** T31.
- **Requirements:** FR-047, FR-050, FR-054, FR-059–FR-061; AC-33, AC-39, AC-42, AC-43; MVP-SEC-01–MVP-SEC-03, MVP-SEC-06, MVP-SEC-08; MVP-NFR-01, MVP-NFR-03–MVP-NFR-06.
- **ADRs / decisions:** ADR-0005–ADR-0009.
- **Affected boundaries:** graph/parent/child identity, attempts, lineage, child envelope, timeout/maximum-attempt controls, local publication, compatibility readers.
- **Expected implementation:** Bind authority to reviewed graph digest/revisions/effects; allocate opaque identities; publish complete graph and child envelopes under ADR-0007; enforce child subsets plus timeout/maximum-attempt controls; dual-read existing ADR-0008 records without invented lineage.
- **Authority and side effects:** Exact local publication only. No dispatch, worktree creation, command/Runtime/Provider/Git effects, authority inheritance by implication or execution-control expansion.
- **Failure / recovery:** Stale preview/revision, invalid subset, invalid execution controls or publication interruption preserves prior authority or reports partial/recovery-required; unknown state is never overwritten.
- **Explicit non-goals:** Portable/shared graph, migration of sequential records, Runtime execution, general event platform.
- **Mandatory tests:** Identity uniqueness/stability; parent/child/graph revision; authority subset and timeout/maximum-attempt validation; stale replay/no-op; F0–F8 publication faults; old sequential reader fixtures; unsafe root/link/ownership cases.
- **Expected Evidence:** Preview/authority digest, graph and envelope hashes/revisions, exact write ledger, fault/concurrency matrix and unchanged ADR-0008 fixture identities.
- **Completion criteria:** One safe machine-local graph authority exists, every child is bounded, and existing sequential truth remains unchanged.
- **Risks / gates:** ADR-0009 is Accepted; T32 implementation still requires explicit S8 authority. Migration or portable graph authority requires a new human decision.

### T33 — Dependency scheduler, isolation, cancellation and retry

- **Objective:** Only dependency-ready, non-conflicting children dispatch into confined isolated workspaces with truthful timeout, cancellation, retry and partial-state handling.
- **Slice:** S8.
- **Dependencies:** T32.
- **Requirements:** FR-051, FR-054, FR-056–FR-058; AC-33, AC-35, AC-39–AC-41; MVP-SEC-01–MVP-SEC-06, MVP-SEC-08; MVP-NFR-01–MVP-NFR-06.
- **ADRs / decisions:** ADR-0005, ADR-0007, ADR-0009.
- **Affected boundaries:** dependency readiness, effect conflict, process invocation, workspace/worktree lifecycle, command environment, cancellation, attempt/retry state.
- **Expected implementation:** Deterministic ready-set selection; explicit argv/environment/cwd; bounded output and timeout; isolated worktree/workspace creation/inspection; conflict serialization; stop-new-dispatch cancellation; new-attempt retry after effect reconciliation.
- **Authority and side effects:** Child envelope authorizes only named local/process effects. No implicit Git hooks/network/push/merge, shared checkout mutation, credential expansion, child spawning or automatic timeout/attempt growth.
- **Failure / recovery:** Runtime unavailable, timeout, signal uncertainty, stale base, foreign mutation, worktree collision, ambiguous external effect or cleanup failure remains explicit; confirmed effects persist and retry blocks until safe.
- **Explicit non-goals:** Distributed scheduler, arbitrary shell, automatic merge, destructive workspace cleanup, guaranteed hard kill of every Runtime.
- **Mandatory tests:** DAG readiness/order; two concurrent independent children; dependency/conflict serialization; argv/injection; path/link/ownership confinement; timeout/cancel races; attempt retry/ambiguity; process barriers and fault injection.
- **Expected Evidence:** Dispatch/ready-set ledger, overlap timing proof, workspace identities/tree hashes, command/effect ledger, cancellation/retry outcomes and outside-target hashes.
- **Completion criteria:** Scheduler never dispatches unready/unauthorized work, proves safe parallelism, and preserves truthful uncertain/partial state.
- **Risks / gates:** Destructive cleanup or new Git/network effects require separately reviewed authority.

### T34 — Structured coordination and bounded child publication

- **Objective:** Children exchange typed bounded coordination and publish validated artifacts/results correlated to graph lineage without raw chat becoming workflow truth.
- **Slice:** S8.
- **Dependencies:** T32.
- **Requirements:** FR-052, FR-055, FR-059; AC-37, AC-39, AC-42; MVP-SEC-02, MVP-SEC-03, MVP-SEC-06, MVP-SEC-09; MVP-NFR-01–MVP-NFR-06.
- **ADRs / decisions:** ADR-0006, ADR-0007, ADR-0009.
- **Affected boundaries:** coordination record types, revisions, provenance, sanitization, retention, artifact/result publication, usage observations.
- **Expected implementation:** Closed schemas for question/request, answer, contract proposal/acceptance, blocker, dependency resolution, artifact publication, progress and result; validate correlation/revision/size/content; retain references under artifact policy; render optional conversational projection.
- **Authority and side effects:** Coordination cannot grant authority, change graph/allowlist/execution controls, execute content or publish outside the child envelope. Raw prompts/chat/reasoning and credential values are rejected.
- **Failure / recovery:** Malformed, oversized, stale, forged-lineage, injection-bearing or unsupported content is rejected/quarantined as data; interrupted publication follows ADR-0007; capacity requires safe reference-aware action.
- **Explicit non-goals:** Chat archive, shared scratchpad as truth, hidden reasoning capture, universal message bus, unbounded telemetry.
- **Mandatory tests:** Every record type and transition; stale/duplicate/idempotent replay; injection/metacharacters; secret sentinel; bounds/quotas; retention/reference; child/parent correlation; measured/partial/unavailable usage.
- **Expected Evidence:** Structural fixtures, record/artifact digests and revisions, rejected-input/non-effect ledger, retention observations and raw-chat absence proof.
- **Completion criteria:** Required coordination and publications are inspectable, bounded, provenance-correct and cannot alter control state implicitly.
- **Risks / gates:** New retained content classes or portable coordination require privacy/security and human review.

### T35 — Integration/Reconciliation Execution and parent roll-up

- **Objective:** One bounded Integration/Reconciliation child validates and integrates authorized child results, runs combined validation and produces deterministic parent outcome truth.
- **Slice:** S8.
- **Dependencies:** T33, T34.
- **Requirements:** FR-053, FR-058, FR-059; AC-38, AC-41, AC-42; MVP-SEC-01–MVP-SEC-06, MVP-SEC-08; MVP-NFR-01–MVP-NFR-06.
- **ADRs / decisions:** ADR-0005–ADR-0007, ADR-0009.
- **Affected boundaries:** child result verification, base/drift/conflict inspection, authorized integration, combined validation, roll-up and completion.
- **Expected implementation:** Verify lineage/envelopes/artifacts and workspace bases; preview ordered integration effects; detect drift/conflict; apply only authorized changes; run declared validation; roll up required/optional child outcomes and references into canonical completion.
- **Authority and side effects:** Exact integration authority bound to child results, target revision and effect set. No child/self merge, push, Provider mutation, scope waiver, fabricated success or automatic conflict choice.
- **Failure / recovery:** Missing/unknown/failed/cancelled child, stale base, foreign changes, conflict, validation failure or post-integration reporting failure yields truthful blocker/partial/recovery-required state while preserving confirmed effects.
- **Explicit non-goals:** Autonomous conflict resolution, release publication, human acceptance, unrelated refactoring or cleanup.
- **Mandatory tests:** Valid multi-child integration; missing/forged/stale result; clean/conflict/foreign-drift matrices; combined validation pass/fail; required/optional roll-up; interruption/replay and confirmed-effect reporting.
- **Expected Evidence:** Integration preview/digest, source/result tree hashes, applied-effect ledger, conflicts, validation commands/exits, parent roll-up and untouched foreign-change proof.
- **Completion criteria:** Only declared integration owns combined delivery and parent truth never exceeds verified child/integration outcomes.
- **Risks / gates:** Conflict decisions that change behavior/scope remain **Human decision required**.

### T36 — Codex + Claude acceptance journey and graph Evidence

- **Objective:** One small real Axiom engineering activity proves the complete S8 graph using Codex and Claude and produces sanitized per-child/parent Evidence ready for human review.
- **Slice:** S8.
- **Dependencies:** T35.
- **Requirements:** FR-045–FR-061; AC-32–AC-43; MVP-SEC-01–MVP-SEC-09; MVP-NFR-01–MVP-NFR-07.
- **ADRs / decisions:** ADR-0001–ADR-0009. ADR acceptance does not authorize execution.
- **Affected boundaries:** installed Runtimes, operator configuration, Planner, graph/store, scheduler, worktrees, coordination, integration, completion, Evidence and compatibility.
- **Expected implementation:** Run approved Plan -> Agent Planner -> reviewed graph -> at least two independent children, one through Codex and one through Claude -> observed concurrency when dependencies permit -> isolated workspaces/worktrees -> structured coordination -> Integration/Reconciliation -> combined validation -> parent Evidence/result. The activity must be real engineering work in Axiom, not children creating artificial marker files. Also exercise no-match, denial, timeout/cancel, maximum-attempt retry ambiguity, partial roll-up and sequential compatibility.
- **Authority and side effects:** The approved real-run envelope is local: create local worktrees/workspaces; modify files only inside isolated environments; invoke Codex and Claude; run tests/builds/linters/validators; create artifacts/Evidence; perform local Integration/Reconciliation; and create/use a local test branch when needed. Forbidden without separate authority: push, remote merge, PR/Issue mutation, release, deploy/publication, secret mutation, credential provisioning, Runtime installation, or purchase/limit expansion. Model Profile values, exact repository/worktree targets, commands and cleanup disposition remain recorded in the reviewed run envelope.
- **Failure / recovery:** Unavailable Codex/Claude path, partial cancellation, exhausted maximum attempts, ambiguous effect, integration conflict or missing Evidence remains explicit and blocks S8 technical completion; missing usage/cost is `unavailable`, never estimated, and does not become budget enforcement. No substitute simulation for required real paths.
- **Explicit non-goals:** RC publication, S9 acceptance, benchmark/ranking, unlimited load, model quality claims beyond the bounded journey.
- **Mandatory tests:** Full deterministic T30–T35 suites; real Codex + Claude black-box graph with at least one independent child per Runtime; observed concurrency; structured coordination; integration; cancellation/retry/partial cases; security review; repository validation; ADR-0008 fixture replay.
- **Expected Evidence:** Exact revisions/environment/config digests; Runtime/Model Profile/version provenance; graph/lineage/envelopes/authority/timeout/maximum attempts; commands/exits/timing; coordination/artifact/result references; observational usage/cost source/unit/status or `unavailable`; integration hashes/validation; limitations and cleanup disposition.
- **Completion criteria:** AC-32–AC-43 have inspectable Evidence, Codex and Claude are both proven on real independent children, no required check is hidden, and outcome is `S8 ready for human review`, never human-accepted or release-authorized.
- **Risks / gates:** Concrete model names remain local Model Profile configuration. Activity selection occurs during T36 preparation within the approved real-Axiom-work boundary. Technical success does not authorize T23/S9 or human acceptance.

### T37 — Public `axiom` CLI and distribution identity

- **Objective:** A user installation exposes the product through the canonical
  `axiom` executable and public documentation no longer requires `lingo` as the
  shell entrypoint.
- **Slice:** S9 — Productization, distribution, Runtime bootstrap and release acceptance.
- **Dependencies:** T36.
- **Requirements:** FR-062; AC-44, AC-48; MVP-NFR-01, MVP-NFR-05.
- **ADRs / decisions:** Preserve `Axiom != Lingo`; this Task changes the public
  executable/distribution identity, not the domain or all internal package names.
- **Affected boundaries:** Go executable build target, release archive naming,
  installation receipts, help/version/provenance output, documentation, tests and
  compatibility checks.
- **Expected implementation:** Produce/install an executable named `axiom`
  directly. Reuse the existing application/CLI composition rather than duplicating
  command handlers. A user-created shell alias is not the primary contract.
  Internal `cmd/lingo`/package naming may be migrated only where necessary for a
  clean build/distribution boundary.
- **Authority and side effects:** Repository-local implementation/tests only.
  Installation tests use isolated user-owned roots. No release publication.
- **Failure / recovery:** Existing owned installation metadata and compatibility
  readers must distinguish the new public binary identity without silently
  rewriting unknown/foreign state.
- **Explicit non-goals:** Renaming every Lingo concept/package, shell-profile
  mutation, package managers, stable release publication.
- **Mandatory tests:** `axiom version`, `axiom --json version`, representative
  commands from unrelated CWD, archive entry/receipt identity, old owned
  installation compatibility, and absence of required shell aliasing.
- **Expected Evidence:** Built executable name/hash, help/version output, archive
  manifest/receipt bytes, compatibility matrix and command exits.
- **Completion criteria:** The supported installed product is directly invokable
  as `axiom`; public acceptance instructions require no `lingo` command.
- **Risks / gates:** Any persisted-format change beyond necessary compatible
  identity metadata requires explicit review.

### T38 — Automated supported-platform release artifact pipeline

- **Objective:** One clean exact revision deterministically drives automated build
  preparation for every supported release row with closed manifests/checksums and
  truthful provenance.
- **Slice:** S9.
- **Dependencies:** T37.
- **Requirements:** FR-063; AC-45; HD-1; MVP-NFR-05, MVP-NFR-07.
- **ADRs / decisions:** Artifact preparation is deterministic; publication remains
  separately authority-bearing under T23.
- **Affected boundaries:** repository CI/release workflow, cross/native build
  commands, archive naming/layout, manifests/checksums, provenance and artifact
  retention.
- **Expected implementation:** Add a versioned release-build workflow that starts
  from one clean revision/version input, builds the approved macOS 27/arm64,
  Ubuntu 26.04/amd64 and Ubuntu 26.04/arm64 artifacts, verifies archive contents,
  emits `SHA256SUMS`/closed metadata and uploads only preparatory workflow
  artifacts. Reuse the existing release archive builder where contracts match.
- **Authority and side effects:** CI may build and retain preparatory artifacts.
  It must not create tags, GitHub Releases, prereleases or stable promotion without
  the separate T23 authority.
- **Failure / recovery:** Missing/failed target, dirty/ambiguous revision,
  inconsistent manifest or checksum blocks candidate completeness; partial build
  output is never described as a complete release set.
- **Explicit non-goals:** Signing/notarization, package managers, Windows,
  automatic stable publication.
- **Mandatory tests:** clean/dirty revision, exact-version input, full three-row
  matrix, archive content/mode checks, checksum verification, rerun equivalence,
  missing-row failure and CI no-publication proof.
- **Expected Evidence:** workflow revision/run, per-row commands/exits, archive and
  entry digests, `SHA256SUMS`, provenance and no-publication effect ledger.
- **Completion criteria:** One automated run can prepare the complete verified
  supported artifact set from one exact revision without publication authority.
- **Risks / gates:** Native Evidence requirements remain distinct from
  cross-compilation/build success and are still closed by T24.

### T39 — Stable remote installer and convergent owned upgrade

- **Objective:** A supported clean macOS/Linux machine installs Axiom from one
  stable HTTPS bootstrap without cloning/building, and rerunning that bootstrap
  converges safely.
- **Slice:** S9.
- **Dependencies:** T38.
- **Requirements:** FR-064, FR-065; AC-44, AC-46, AC-48; HD-1, HD-3, HD-4.
- **ADRs / decisions:** ADR-0005 and ADR-0007 safety boundaries; reuse T20 owned
  upgrade/recovery semantics instead of bypassing ownership checks.
- **Affected boundaries:** bootstrap script/endpoint, platform detection, release
  asset selection/download, checksum verification, owned binary/receipt update,
  install/upgrade recovery and user-facing diagnostics.
- **Expected implementation:** Create the canonical bootstrap at
  `scripts/install.sh` and expose it through the repository's stable raw URL,
  suitable for `curl -fsSL <url> | sh`. Detect the exact supported row, resolve
  one published release under the FR-064 selection policy (Issue #81): no
  selector or `--channel stable` installs the latest published stable
  `vX.Y.Z` release and never falls back to an RC (no stable release fails with
  zero effects and points to `--version`); `--version <tag>` resolves exactly
  that stable or `vX.Y.Z-rc.N` tag; release candidates are selected only by
  exact version, and a floating RC selector is not required; `--channel` with
  `--version`, or any unsupported selector, is a zero-effect input error; drafts
  are never installable. Download the matching artifact and checksum data,
  verify before mutation, and invoke/reuse the protected installation path. For
  a recognized owned install: same resolved version is a no-op, newer upgrades
  through the protected path, older is refused as an automatic downgrade.
  Foreign/modified/unsafe/ambiguous state refuses unchanged. The install and its
  Evidence bind to the exact resolved tag, asset and asset SHA-256, not the
  selector. T24 pins the exact RC with `--version vX.Y.Z-rc.N`.
- **Authority and side effects:** Network read plus bounded user-owned local
  installation. No shell-profile mutation, privilege escalation, credential
  mutation or release publication unless separately decided.
- **Failure / recovery:** Unsupported row, network failure, missing asset,
  checksum mismatch, stale/foreign receipt, modified target, unsafe root or
  interrupted upgrade remains explicit and recoverable under existing contracts.
- **Explicit non-goals:** background auto-update daemon, package managers,
  Runtime installation, automatic PATH/profile edits, signing/authenticity claims.
- **Mandatory tests:** three supported selection rows; unsupported row; equivalent
  reinstall; older owned upgrade; downgrade refusal; modified/foreign target;
  checksum mismatch; interrupted/resumed upgrade; unavailable asset/network;
  stable-only default and `--channel stable` equivalence; no-stable-release
  failure without RC fallback; exact stable and exact-RC `--version` pinning;
  `--channel`/`--version` conflict and unsupported selector with zero effects;
  draft release never selected.
- **Expected Evidence:** bootstrap bytes/digest/URL, selector and exact resolved
  release identity, selected asset/checksum, platform facts, install/upgrade effect ledger, receipt revision, no-op/refusal
  tree hashes and recovery result.
- **Completion criteria:** A clean supported user can install a pinned published
  Axiom version with one remote bootstrap, and repeated execution is convergent
  without weakening fail-closed ownership guarantees.
- **Risks / gates:** The bootstrap source/hosting decision is fixed at
  `scripts/install.sh` in this repository and the release-selection/version
  policy is fixed by FR-064/FR-065 (Issue #81; exact-version-only RCs decided
  2026-09-28). A floating RC channel or an explicit downgrade mechanism is
  post-MVP and needs its own decision.

### T40 — Codex + Claude first-run bootstrap

- **Objective:** `axiom first-run` discovers supported Runtimes already installed
  on the machine and idempotently configures every detected Axiom integration.
- **Slice:** S9.
- **Dependencies:** T39.
- **Requirements:** FR-066, FR-067; AC-47, AC-49; FR-048, FR-049; AC-34, AC-36.
- **ADRs / decisions:** Runtime remains distinct from model; operator allowlists
  and machine-local Model Profiles remain authoritative. Maintainer `CLAUDE.md`
  bootstrap is not sufficient product Runtime installation.
- **Affected boundaries:** Runtime discovery, Codex integration installer/status,
  Claude integration installer/status, first-run aggregation, compatibility
  metadata, diagnostics and Evidence.
- **Expected implementation:** Add the product Claude integration installation/
  status path needed by the delivered Claude Runtime adapter, preserve the Codex
  path, and make first-run enumerate all supported Runtime observations before
  applying only the missing/owned Axiom integration state. All Axiom Runtime
  skills are user-global by product rule. Preserve Codex's user-global skill root;
  install Claude skills under the Claude user-global
  `skills/<name>/SKILL.md` hierarchy rooted at `~/.claude` or the effective
  `CLAUDE_CONFIG_DIR`. Project-local Runtime skills are not used as a fallback.
  Cover Codex-only, Claude-only, both and neither. Do not silently install Runtime
  binaries, change credentials, provision secrets, buy capacity or choose an
  unapproved Model Profile.
- **Authority and side effects:** Only bounded Axiom-owned Runtime integration
  files/state inside the current user's Runtime-native global roots for detected
  local Runtimes. Tests use isolated roots where possible; real Runtime invocation
  remains separately gated.
- **Failure / recovery:** One unavailable/failed Runtime integration is reported
  independently; confirmed integration effects remain truthful; rerun converges
  owned partial state and refuses foreign/modified integration state.
- **Explicit non-goals:** General Runtime plugin ecosystem, credential/login
  automation, Runtime package installation, model/vendor ranking.
- **Mandatory tests:** no Runtime, Codex-only, Claude-only, both; equivalent rerun;
  partial prior install; foreign/modified integration state; binary/integration
  compatibility; unavailable Runtime; first-run human/JSON result.
- **Expected Evidence:** discovery observations, integration digests/statuses,
  effect ledger, no-provisioning proof, rerun/refusal matrices and sanitized
  first-run output.
- **Completion criteria:** One `axiom first-run` configures all and only supported
  detected Runtimes and leaves the machine in truthful usable/blocked state for
  S8 Runtime resolution.
- **Risks / gates:** Each future supported Runtime must declare and validate its
  Runtime-native user-global skill root before product support; no Project-local
  fallback is allowed merely because a global integration is unavailable or
  unsafe. The concrete Runtime-presence signal, first-run exit semantics and
  Claude ownership/receipt mechanics are implementation choices reviewed with
  T40; they must satisfy FR-066/AC-47 without weakening fail-closed ownership
  and are not fixed by this Task amendment.

### T23 — Identified RC archives and authorized prerelease publication

- **Objective:** One clean revision produces immutable checksummed RC archives for all supported targets and, with exact authority, publishes them plus `SHA256SUMS` and instructions as an identified GitHub prerelease candidate.
- **Slice:** S9 — Productization, distribution, Runtime bootstrap and release acceptance.
- **Dependencies:** T40.
- **Requirements:** FR-031–FR-037; AC-01, AC-19, AC-21, AC-23, AC-24; MVP-SEC-01, MVP-SEC-02, MVP-SEC-04–MVP-SEC-06; MVP-NFR-05–MVP-NFR-07.
- **ADRs / decisions:** ADR-0003, ADR-0005, ADR-0007; HD-1, HD-3.
- **Affected boundaries:** release build, archives/checksums/metadata, GitHub Releases distribution adapter, install documentation, RC identity.
- **Expected implementation:** Use the T38 pipeline for a clean revision-bound build of one `vX.Y.Z-rc.N` candidate; reproduce target manifests; verify internal skill compatibility; generate checksums; preview exact release/tag/assets/body/effects; publish once as a non-draft GitHub prerelease (`prerelease=true`); read back asset identities/sizes/checksums.
- **Authority and side effects:** **Human gate before execution.** Exact GitHub prerelease/tag/asset mutation authority required. Allowed only named RC release assets/metadata. Forbidden: final stable release claim, overwrite/delete unrelated release/assets/tags, signing/authenticity claim, automatic latest pointer, Provider issues/workflow mutation.
- **Failure / recovery:** Confirmed tag/release/assets remain reported; later asset/metadata failure is `partial` with exact read-back and resume plan; ambiguous response reconciles before retry; local failure never invents remote rollback.
- **Explicit non-goals:** Human RC acceptance, stable release publication, package managers, signing/notarization, Windows, automatic update.
- **Mandatory tests:** Reproducible manifest/checksum validation; clean/dirty build; archive content/permissions; missing target refusal; fake release adapter failure/ambiguity/duplicate; authorized real read-back after publication.
- **Expected Evidence:** Revision/source state, build commands/exits, per-target archive and entry hashes, `SHA256SUMS`, release URL/asset IDs/read-back, effect ledger, limitations, and explicit candidate status.
- **Completion criteria:** The exact RC consumed by T24 is immutable, traceable, published through the approved adapter, and not represented as human-accepted/final.
- **Risks / gates:** Distribution authenticity remains unclaimed. Any signing or alternate trust topology requires **Human decision required**.

### T24 — Clean-environment CLI/Codex/Claude/GitHub acceptance matrix

- **Objective:** Each supported clean environment completes the published install-to-completion journey through direct CLI, Codex and Claude, including representative denial, failure, interruption, recovery, cleanup, and upgrade paths.
- **Slice:** S9.
- **Dependencies:** T23.
- **Requirements:** FR-001–FR-067; AC-01–AC-23, AC-25–AC-49; MVP-SEC-01–MVP-SEC-09; MVP-NFR-01–MVP-NFR-07; SEC-001–SEC-005; HD-1–HD-4.
- **ADRs / decisions:** ADR-0001–ADR-0009.
- **Affected boundaries:** stable remote installer, public `axiom` CLI, published archive/receipts, Codex and Claude Runtime adapters/first-run bootstrap, Project, Work Item, Execution/workflow, GitHub projection, completion/artifacts/Evidence, recovery/cleanup/upgrade.
- **Expected implementation:** From isolated accounts/VMs with no Axiom roots, follow published instructions only: stable remote install pinned to the exact T23 candidate with `--version vX.Y.Z-rc.N` (never a floating selector) -> `axiom version` provenance -> Codex/Claude discovery/bootstrap -> `axiom first-run` -> Project/metadata policy -> Intent -> authorized GitHub Work Item -> gated lifecycle/flags/bounded history -> workflow/projection -> approved multi-runtime graph -> isolated parallel children -> structured coordination -> integration -> parent Evidence/completion -> one real Axiom engineering dogfood activity through Axiom -> missing-local-state inspection -> reinstall/owned upgrade -> recovery/cleanup checks.
- **Authority and side effects:** **Human gate before each real run.** Exact authority required for bounded GitHub Issue/label/comment effects plus Codex and Claude invocation. Allowed effects and cleanup ownership listed before execution. Forbidden: closing work as human acceptance, unrelated repository/Git mutation, credential publication, release promotion.
- **Failure / recovery:** Exercise all seven statuses, invalid selectors, denial, interruption/resume, retryable Provider failure, confirmed Provider/local failure, recovery-required, capacity exhaustion, POC detection/export-reconfigure, and partial upgrade. Preserve external effects/references truthfully.
- **Explicit non-goals:** Automated human acceptance, broad provider/runtime coverage, production workload/load test, historical CI substitution.
- **Mandatory tests:** Full black-box journey on all three support rows; the T22 native filesystem/install/upgrade suite (`scripts/test-s7-native.sh` or its versioned successor) on Ubuntu 26.04/amd64/ext4 and Ubuntu 26.04/arm64/ext4, deferred from S7 by HD-S7-T22; CLI/Runtime semantic matrix; real bounded Codex and Claude discovery/invocation; replay or reproduce the T36 graph Evidence against the exact RC with one independent child per Runtime, observed concurrency, isolation, coordination and Integration/Reconciliation; real bounded GitHub create/projection; controlled fake failure/non-effect cases; documentation replay.
- **Expected Evidence:** Candidate/environment identity, commands/exits, hashes/references, prompt counts, Provider/Codex/Claude observations, exact T36 Evidence references or RC rerun records, side-effect ledger, artifact/Evidence IDs/digests, platform facts, exclusions, unexecuted cases, and cleanup disposition.
- **Completion criteria:** Every AC-01–AC-23 and AC-25–AC-49 has inspectable future Evidence on required scope/platforms, including Codex + Claude RC Evidence, the S9 productization and dogfood Evidence, and native passing Evidence for both Ubuntu 26.04 rows deferred from T22, all against the same exact pinned RC; AC-24 remains the separate human decision and failures or unavailable rows block RC readiness.
- **Risks / gates:** Real credentials stay outside Evidence. Test success prepares review only and never fills AC-24's human decision.

### T25 — Versioned RC Evidence, documentation reconciliation, and human gate

- **Objective:** Publish a sanitized, auditable RC report and reconcile only delivered behavior, then stop for explicit human accept/reject decision.
- **Slice:** S9.
- **Dependencies:** T24.
- **Requirements:** FR-001–FR-067 Evidence closure; AC-21–AC-49; MVP-SEC-02; MVP-NFR-05–MVP-NFR-07; SEC-001, SEC-005; Constitution II–VI.
- **ADRs / decisions:** ADR-0001–ADR-0009; HD-1–HD-4.
- **Affected boundaries:** Specification Evidence/index, README, commands, architecture/operations docs, CHANGELOG, RC report, final human gate.
- **Expected implementation:** Map every claim to source revision, command/test, exit/result, artifact/reference/digest, environment, limitation, and review finding; include measured artifact sizes/counts and recommendation on initial limits/retention; reconcile docs to verified behavior only.
- **Authority and side effects:** Documentation/repository changes only until explicit human decision. Forbidden: self-acceptance, stable release promotion, issue closure as acceptance, new feature/ADR, retroactive Evidence rewrite.
- **Failure / recovery:** Missing/failed/unverified Evidence remains explicit and blocks recommendation; sensitive/publication uncertainty stops report publication; earlier Evidence remains preserved.
- **Explicit non-goals:** Implementation fixes bundled into reconciliation, automatic waivers, hiding limitations, claiming logs/chat/compilation alone as behavioral Evidence.
- **Mandatory tests:** Full FR/AC/security/NFR/ADR coverage audit; DAG/task Evidence links; documentation/status/link checks; repository/test/race/vet/build/module/security validations; public-data review; independent severity-based engineering/security review.
- **Expected Evidence:** Versioned RC report, complete matrices, exact validation commands/exits, diff and sensitive-file scans, open findings/limitations, and a clearly empty human decision field awaiting reviewer action.
- **Completion criteria:** Report is complete enough for a human to accept or reject; automation records no acceptance. Final state before decision remains `RC ready for human review`, not Released/Accepted.
- **Risks / gates:** **Human decision required:** accept RC, reject RC with findings, or request bounded remediation. Recommendation must state trade-offs and unresolved limitations without making the decision.

## Requirement ownership

T24 performs the end-to-end audit and T25 reconciles retained Evidence. The owners
below implement or verify each approved requirement according to its
recorded lifecycle status; a table reference alone is
not completion Evidence.

### Functional requirements

| Requirement | Responsible Tasks |
|---|---|
| FR-001 | T06, T07, T14, T24 |
| FR-002 | T06, T24 |
| FR-003 | T06, T07, T17, T24 |
| FR-004 | T06, T07, T24 |
| FR-005 | T06, T07, T24 |
| FR-006 | T06, T07, T24 |
| FR-007 | T06, T07, T24 |
| FR-008 | T08, T09, T24 |
| FR-009 | T08, T24 |
| FR-010 | T08, T09, T24 |
| FR-011 | T08, T09, T21, T24 |
| FR-012 | T08, T09, T24 |
| FR-013 | T10–T13, T24 |
| FR-014 | T12, T13, T24 |
| FR-015 | T12, T24 |
| FR-016 | T01, T10–T13, T21, T24 |
| FR-017 | T09, T12, T13, T24 |
| FR-018 | T14, T15, T24 |
| FR-019 | T14, T15, T24 |
| FR-020 | T14, T15, T21, T24 |
| FR-021 | T14, T15, T24 |
| FR-022 | T03, T07, T10, T11, T13, T19, T22, T24 |
| FR-023 | T01–T03, T09, T11–T13, T18–T22, T24 |
| FR-024 | T03, T07, T17–T22, T24 |
| FR-025 | T03, T10, T13, T16, T19, T22, T24 |
| FR-026 | T03, T04, T10, T16, T20, T24 |
| FR-027 | T17, T20, T24 |
| FR-028 | T17, T20, T24 |
| FR-029 | T16, T17, T20, T24 |
| FR-030 | T06, T07, T17, T21, T24 |
| FR-031 | T04, T20, T23, T24 |
| FR-032 | T04, T05, T16, T20, T24 |
| FR-033 | T02, T04, T05, T07, T10, T18, T24 |
| FR-034 | T04, T05, T20, T24 |
| FR-035 | T04–T07, T24 |
| FR-036 | T05, T15, T16, T20, T24 |
| FR-037 | T20, T22, T24 |
| FR-038 | T26–T29, T24 |
| FR-039 | T26, T27, T29, T24 |
| FR-040 | T26, T29, T24 |
| FR-041 | T26, T27, T29, T24 |
| FR-042 | T27, T29, T24 |
| FR-043 | T29, T19, T24 |
| FR-044 | T28, T29, T24 |
| FR-045 | T31, T36, T24 |
| FR-046 | T31, T36, T24 |
| FR-047 | T32, T36, T24 |
| FR-048 | T30, T36, T24 |
| FR-049 | T30, T31, T36, T24 |
| FR-050 | T32, T36, T24 |
| FR-051 | T33, T36, T24 |
| FR-052 | T34, T36, T24 |
| FR-053 | T35, T36, T24 |
| FR-054 | T30, T32, T33, T36, T24 |
| FR-055 | T30, T34, T36, T24 |
| FR-056 | T33, T36, T24 |
| FR-057 | T33, T36, T24 |
| FR-058 | T33, T35, T36, T24 |
| FR-059 | T32, T34–T36, T24 |
| FR-060 | T32, T36, T24 |
| FR-061 | T30–T32, T36, T24 |
| FR-062 | T37, T24 |
| FR-063 | T38, T23, T24 |
| FR-064 | T39, T24 |
| FR-065 | T39, T24 |
| FR-066 | T40, T24 |
| FR-067 | T40, T24, T25 |

### Acceptance criteria

| Acceptance | Responsible Tasks | Required future Evidence |
|---|---|---|
| AC-01 | T04, T05, T22–T24 | Native clean install, checksum, provenance, Codex compatibility |
| AC-02 | T05–T07, T24 | First-run/setup black-box from unrelated CWD |
| AC-03 | T06, T07, T24 | Guided/full-input normalized proposal and state equivalence |
| AC-04 | T06, T07, T24 | Multi-Repository portable/local preview and publication hashes |
| AC-05 | T06, T07, T24 | Missing/unsupported capability block with zero fallback |
| AC-06 | T08, T09, T24 | Intent interview, reviewed draft, authorized GitHub effect |
| AC-07 | T08, T09, T21, T24 | Supplied-field reuse and cancel/denial zero-effect ledger |
| AC-08 | T12, T13, T24 | One marker and idempotent comment reconciliation |
| AC-09 | T10–T13, T24 | Local/Provider interruption, resume, stale-revision truth |
| AC-10 | T14, T15, T24 | Fully specified zero-question CLI/Codex invocations |
| AC-11 | T14, T15, T21, T24 | Strict input rejection and no CWD fallback |
| AC-12 | T01, T11, T15, T24 | Seven-status CLI/Runtime semantic matrix |
| AC-13 | T02, T15, T18, T24 | Compact result versus one referenced artifact |
| AC-14 | T02, T18, T21, T22, T24 | Sanitized/bounded/addressable/correlated local artifact |
| AC-15 | T01, T15, T24 | Equivalent provenance on every supported surface/state |
| AC-16 | T08, T09, T15, T21, T24 | User/Axiom authorship separation |
| AC-17 | T03, T13, T19, T21, T22, T24 | F0–F8 old/new/recovery matrix and unknown preservation |
| AC-18 | T16, T17, T20, T22, T24 | Version/POC detection, backup/export/reconfigure, no overwrite |
| AC-19 | T04, T05, T20, T22, T24 | Owned install/reinstall/upgrade and conflict refusal |
| AC-20 | T09, T12, T13, T21, T24 | Confirmed Provider effect plus local failure `partial` |
| AC-21 | T23, T24, T25 | Complete exact-RC journey on all support rows |
| AC-22 | T20–T24 | Denial/interruption/retry/recovery/upgrade matrix |
| AC-23 | T21–T25 | Versioned sanitized Evidence with environment/limitations |
| AC-24 | T23–T25 | Automation prepares decision; explicit human accept/reject only |
| AC-25 | T26, T27, T24 | Closed lifecycle enum and exact GitHub label mapping |
| AC-26 | T26, T29, T24 | Read-only derivation and inconsistent-fact zero-effect matrix |
| AC-27 | T27, T29, T24 | Exactly one stage, independent flags, foreign-content preservation |
| AC-28 | T27, T29, T24 | Bounded history and no-duplicate replay |
| AC-29 | T26, T27, T24 | Human-only acceptance and technical/Provider non-authority |
| AC-30 | T29, T19, T24 | Exact local-generation recovery or `recovery_required` |
| AC-31 | T28, T29, T24 | Metadata policy resolution, missing-only prompts, adapter isolation |
| AC-32 | T31, T36, T24 | Plan-derived finite graph with work-derived roles/capabilities |
| AC-33 | T31–T33, T36, T24 | Invalid graph/scope/effect/escalation zero-dispatch matrix |
| AC-34 | T30, T36, T24 | Exact allowlisted resolution and no implicit fallback |
| AC-35 | T33, T36, T24 | Observed isolated concurrency and dependency/conflict serialization |
| AC-36 | T30, T36, T24 | Codex + Claude real Runtime paths with independent-child provenance |
| AC-37 | T34, T36, T24 | Structured coordination and bounded publication |
| AC-38 | T35, T36, T24 | Integration ownership, conflict detection and combined validation |
| AC-39 | T30, T32–T34, T36, T24 | Enforced timeout/attempt controls and truthful observational usage/cost availability |
| AC-40 | T33, T36, T24 | Cancellation and correlated safe retry |
| AC-41 | T33, T35, T36, T24 | Deterministic partial/non-success parent roll-up |
| AC-42 | T32, T34–T36, T24 | Complete sanitized parent/child Evidence |
| AC-43 | T32, T36, T24 | ADR-0008 sequential compatibility replay |
| AC-44 | T37, T39, T24 | Clean checksum-verified remote install and direct public `axiom` invocation |
| AC-45 | T38, T23, T24 | Complete automated native artifact/checksum/provenance set |
| AC-46 | T39, T24 | Equivalent no-op, older owned upgrade and foreign/unsafe/ambiguous refusal |
| AC-47 | T40, T24 | Codex-only/Claude-only/both/neither first-run with zero Runtime/credential provisioning |
| AC-48 | T37–T40, T24 | Exact RC journey begins through remote installer and public `axiom` on every support row |
| AC-49 | T40, T24, T25 | Real Axiom engineering activity coordinated through Axiom with Codex+Claude integrated Evidence |

### Security, NFR, human decisions, and ADRs

| Requirement / decision | Responsible Tasks |
|---|---|
| MVP-SEC-01 | T03–T40 where mutation occurs; audited by T21/T24/T36/T39/T40 |
| MVP-SEC-02 | T02, T06, T08, T09, T16–T18, T21, T23–T25, T27–T36 |
| MVP-SEC-03 | T06, T08, T09, T11, T14, T15, T17, T21, T24, T26–T36 |
| MVP-SEC-04 | T09, T12, T13, T21, T23, T24, T27–T29, T33, T35, T36 |
| MVP-SEC-05 | T04–T06, T09, T14, T15, T20, T21, T23, T24, T27, T28, T30, T33–T36 |
| MVP-SEC-06 | T02–T07, T10, T13, T16–T24, T29–T36 |
| MVP-SEC-07 | T06, T07, T16, T17, T21, T24 |
| MVP-SEC-08 | T03, T04, T07, T10, T13, T16–T22, T24, T29–T36 |
| MVP-SEC-09 | T02, T17–T19, T21, T22, T24, T34, T36 |
| MVP-NFR-01 | T01–T40; audited by T21/T24/T36 |
| MVP-NFR-02 | T01, T02, T06, T08, T14, T15, T24, T27, T28 |
| MVP-NFR-03 | T02–T05, T08–T40 where I/O occurs |
| MVP-NFR-04 | T02–T07, T10, T16–T22, T24, T30–T36 |
| MVP-NFR-05 | T04, T05, T07, T15–T17, T20, T22–T25, T30–T36 |
| MVP-NFR-06 | T01, T02, T07, T09–T13, T15, T18–T36 |
| MVP-NFR-07 | T04, T20, T22–T25, T36 |
| SEC-001 | T02, T06, T08, T09, T16–T18, T21, T24, T25, T27, T28 |
| SEC-002 | T03–T07, T09–T15, T17–T21, T23, T24, T26–T29 |
| SEC-003 | T03, T04, T07, T10, T13, T16–T22, T24, T29 |
| SEC-004 | T03, T07, T09–T13, T17–T22, T24, T26–T29 |
| SEC-005 | T02–T07, T10, T13, T16–T22, T24, T25, T26, T29 |
| HD-1 | T04, T05, T20, T22–T25 |
| HD-2 | T02, T06, T07, T10, T11, T15, T18, T21, T24, T25 |
| HD-3 | T02–T04, T07, T13, T17–T24 |
| HD-4 | T04, T16, T17, T20, T22–T25 |
| ADR-0001 | T06, T07, T14, T21, T24, T25, T28 |
| ADR-0002 | T08, T24, T25 |
| ADR-0003 | T01, T04–T15, T20, T23–T29 |
| ADR-0004 | T03, T06, T07, T16, T17, T19–T21, T24, T25, T28 |
| ADR-0005 | T02–T04, T07, T10, T13, T16–T24, T29 |
| ADR-0006 | T02, T03, T09–T13, T15, T18, T19, T21, T24–T29 |
| ADR-0007 | T02–T05, T07, T09–T13, T17–T24, T26–T29 |
| ADR-0008 | T10–T15, T18, T19, T21, T24–T27, T29 |
| ADR-0009 (Accepted 2026-09-26; T30–T36 implementation authorized 2026-09-27; real T36 run separately authorized, executed and evidenced) | T30–T36, T24, T25 |

No approved FR/AC, approved security/NFR clause, inherited SEC requirement, HD
decision, or Accepted ADR is silently deferred. ADR-0009 acceptance is recorded
without implying implementation authority. T25 is
documentary closure, not the sole owner of behavior. Windows and declared
Specification non-goals are excluded by scope rather than omitted requirements.

## Human decisions and implementation-detail gates

Issue #97 introduces one new durable cross-cutting decision. Accepted ADR-0009
covers parent/child identity and lineage, DAG dependencies, authority envelopes,
Runtime resolution, isolation/integration ownership, coordination, retry/
cancellation and Evidence correlation. ADR-0001–ADR-0008 remain the approved
historical baseline; ADR-0009 governs new S8 graph work only.
The approved Issue #94 contract extends the existing local workflow and Provider
projection; it does not change their source-of-truth model, create another
Execution lifecycle, or introduce a generic metadata schema. Human approval of
the amended Specification/Plan/Tasks and explicit S6 implementation authority
were recorded on 2026-09-24.

Future Task implementation may choose reversible mechanisms such as syscalls,
lock library, local filenames/layout, journal encoding, Go package/interface shape,
and bounded retry implementation, provided Evidence proves the approved behavior.
Stop the affected work with **Human decision required** if implementation instead
requires a different release trust topology, storage engine, graph semantics
beyond accepted ADR-0009, automatic cleanup, portable artifact exchange, in-place historical
migration, changed commit/authority semantics, or another durable expensive-to-
reverse choice. Record problem, options, recommendation, trade-offs,
reversibility, and future impact; do not create or accept an ADR automatically.

Task-specific external gates remain explicit:

- T09/T12 real GitHub observations require exact reviewed Provider authority.
- T15 real Codex invocation and T24 real Codex + Claude invocations require
  isolated compatible Runtime state.
- T27 real GitHub lifecycle/flag/history observation requires exact reviewed
  Provider authority.
- T28 Work Item/PR metadata mutation requires an operation-specific exact preview
  and separate Provider authority.
- T29 mutating local recovery requires fresh exact ADR-0007 recovery authority;
  its ordinary reconciliation inspection remains read-only.
- T30–T35 were implemented under the explicit 2026-09-27 S8 authority. T33/T35
  concrete Git delivery and the separately authorized T36 lab journey are
  recorded in S8 Evidence. The corrected candidate is ready for human review.
- Each T36 run uses the approved local-only boundary and requires its own
  reviewed run envelope naming Model Profiles, repositories/worktrees, commands,
  timeout/maximum attempts, exact local effects and cleanup. External effects need
  their own authority.
- T23 requires exact GitHub prerelease/tag/asset publication authority.
- T24 requires per-run Provider mutation authority and declared cleanup ownership.
- T25 stops before human RC acceptance/rejection and stable release promotion.

## Tasks-phase validation boundary

This artifact defines future implementation and Evidence obligations. Current
Tasks-phase validation can prove document structure, complete traceability, valid
dependencies, an acyclic DAG, status consistency, repository hygiene, and absence
of production-code changes. It cannot prove future APFS/ext4 behavior, GitHub or
Codex integrations, installer/upgrade correctness, security boundaries, or RC
acceptance. Those claims remain assigned to the Tasks above.

Tasks-phase validation on 2026-09-20 established:

- 25 unique complete Task definitions; exact table/body/Mermaid dependency
  equivalence; valid references; acyclic DAG; at least one Task in every slice
  S1–S7; and programmatic enforcement of the S4 -> S5 and S5 -> S6 gates;
- complete ownership for FR-001–FR-037, AC-01–AC-24, MVP security/NFR aliases,
  inherited SEC-001–SEC-005, HD-1–HD-4, and ADR-0001–ADR-0008;
- Markdown-only scope across the five authorized paths, valid relative links,
  balanced fences/tables, and no production-code or implementation change;
- passing repository validation, Go test/vet/build/module verification, whitespace
  checks, staged sensitive-file scan, and staged `gitleaks` scan.

These checks validate decomposition and repository hygiene only. All behavioral,
native-platform, Provider, Runtime, filesystem, security, and RC Evidence remains
future work under explicit Task authority.

The Issue #94 amendment adds four approved Tasks without rewriting the 2026-09-20
25-Task validation record. Amendment validation proves 29 unique definitions,
table/body/Mermaid equivalence, an acyclic DAG, complete FR-038–FR-044 and
AC-25–AC-31 ownership, and the gates `S5 -> S6 -> S7 -> S8`. These are
document-structure claims only; they do not prove or authorize T26–T29 behavior.

The approved Issue #97 amendment adds seven Task definitions without rewriting
prior Task history. It preserves T23–T25 IDs and historical RC objective while
expanding T24/T25 for S8 validation/Evidence. Amendment validation must prove 36 unique definitions,
table/body/Mermaid equivalence, an acyclic DAG, complete FR-045–FR-061 and
AC-32–AC-43 ownership, and gates `S7 -> S8 -> S9`. These are documentation claims
only. The Specification, ADR-0009, Plan and Tasks amendment were explicitly
approved on 2026-09-26; that approval does not authorize T30–T36 implementation.

**Tasks: Approved — human approval recorded on 2026-09-20.**

**S1 Implementation — Delivered on `main` through PR #83.**

**T01 — Accepted / merged in PR #74.**

**T02–T03 — Merged in PR #83 with reproducible S1 Evidence.**

**S2 Implementation — Delivered on `main` through PR #84 with reproducible S2 Evidence.**

**S3 Implementation — Delivered on `main` through PR #85 with deterministic and bounded real-provider Evidence. Human acceptance is not inferred.**

**S4 Implementation — Technically complete in PR #91 with deterministic,
fake-Provider, and authorized real-provider Evidence. Human acceptance is not
inferred.**

**S5 Implementation — Delivered on `main` through PR #93 with deterministic and
bounded real Codex Runtime Evidence. Human acceptance is not inferred.**

**S6 (T26–T29) — Explicitly authorized and technically complete at implementation commit `56beb4fc310894ff8de128f52c6a96d22711bec8`, with reproducible [S6 Evidence](evidence-s6.md). Human acceptance is not inferred.**

**S7 (T16–T22) — Explicitly authorized on 2026-09-25 and technically complete; final Evidence at implementation commit `ceb6d0288039793d15a98003d5b30a28a2f19122` (T18 retirement at `ffc2515`). T22 is complete under the revised S7 acceptance scope (HD-S7-T22); the Ubuntu 26.04 amd64/arm64 ext4 native rows are deferred to T24 and remain mandatory before RC acceptance or release claims for those targets. See [S7 Evidence](evidence-s7.md). Human acceptance is not inferred.**

**S8 (T30–T36) — Implementation authorized on 2026-09-27 by explicit human
decision (Issue #97 comment #5852650410); the real T36 Runtime run later received
its own exact authority and was executed. Neither authority includes push, merge,
Provider mutation, release, deploy, credential provisioning, S9 or human
acceptance. T30–T32 and the deterministic
T34 foundation exist at `21f4f7c` plus the publication read-back correction
`e654f83`; T33 and T35 are technically complete at `e884a46` plus composition
fix `7bfadc3`, with concrete Git workspace/integration adapters, combined
validation and application composition.
The separately operator-authorized real Codex + Claude T36 lab run produced
`real_run_recorded` Evidence after the clean-state coordination fix. See
[T36 Evidence](evidence-t36/README.md). The corrected candidate is S8 ready for
human review; human acceptance is not inferred.**

**S9 (proposed T37–T40 plus historical T23–T25) — Productization,
distribution, Runtime bootstrap and release acceptance. Product scope recorded
2026-09-27; Task amendment pending human approval. T37–T40 are implemented in
PR #106 (unmerged) and recorded in [S9 Evidence](evidence-s9.md).
T23–T25, publication and acceptance are not authorized or started.**
