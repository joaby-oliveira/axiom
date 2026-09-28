# Specifications

Specifications define desired outcomes and observable behavior before implementation
plans. Approval gates remain separate: an approved Specification permits planning;
implementation requires an approved Plan/Tasks and explicit authorization for the
bounded delivery unit. Technical merge does not imply human acceptance or next-Task authority.

## 001 — Codex agent harness generation

[Specification](001-codex-agent-harness-generation/spec.md): **Proposed**.

## 002 — Lingo Project Initialization

First active vertical slice, in incremental implementation. Current lifecycle,
reconciled on 2026-09-18 against versioned artifacts, merged PRs and POC issues:

| Artifact / delivery | Status |
|---|---|
| [Specification](002-lingo-project-initialization/spec.md) | **Approved** |
| [Plan](002-lingo-project-initialization/plan.md) | **Approved** |
| [Tasks](002-lingo-project-initialization/tasks.md) | **Approved** |
| T01–T03 | **Accepted / merged** — PRs [#6](https://github.com/rgomids/axiom/pull/6), [#7](https://github.com/rgomids/axiom/pull/7), [#8](https://github.com/rgomids/axiom/pull/8) |
| T04 | **Merged; pending human acceptance/re-review** — [PR #9](https://github.com/rgomids/axiom/pull/9); not marked Accepted |
| POC #16–#18 | **Merged in [PR #27](https://github.com/rgomids/axiom/pull/27)** — executable CLI, minimal init/validate/reopen/name update and separate local install |
| POC #19 | **Closed / historical Evidence** — bounded hardening delivered; HD-3/ADR-0005 retain supported fault/concurrency/confinement proof while classifying physical power loss and arbitrary malicious same-UID interleavings as unsupported guarantees |
| POC #20 | **Closed / not planned for further POC work** — deterministic/E2E Evidence was sufficient for POC acceptance; broader Evidence requirements move to the future MVP scope |
| POC #21 | **Accepted / closed** — E2E dogfooding and limitations reconciled on 2026-09-20 |
| Specification 002 / POC #14 | **Partial implementation; not Accepted/Done** |

PR #27 merged the initial POC baseline; PR #28 merged bounded hardening. Current verification and dogfooding
Evidence, including the residual limitations accepted for POC closure, are recorded in
[POC Evidence](002-lingo-project-initialization/evidence-poc.md). The POC issue
deliveries do not establish completion of the full T05–T21 Task DAG.

PR #9 received a final human re-review on 2026-09-17 reporting no findings and
“Ready to merge”, then merged. That review is recorded; it is not converted here
into an explicit Task Accepted decision. T04 acceptance remains a human lifecycle
gate. Neither its merge nor this documentation reconciliation authorizes T05.

Specification approval originated on 2026-09-11. Subsequent
[Clarifications](002-lingo-project-initialization/clarifications.md) preserve Q1–Q6
and H1–H13 history. H13, derived from Specification 004 HD-3 on 2026-09-20,
reconciles SEC-003/SEC-005 and affected AC/Plan/Task Evidence with the bounded local
filesystem threat model in
[ADR-0005](../decisions/0005-bounded-local-filesystem-threat-model.md). It preserves
confinement, supported link/replacement protection, process concurrency, fault
injection, complete canonical state and recovery while explicitly excluding
malicious same-UID arbitrary interleavings and physical power-loss/media guarantees.
Plan and Tasks approval followed in PRs
[#4](https://github.com/rgomids/axiom/pull/4) and [#5](https://github.com/rgomids/axiom/pull/5).
Dated pre-merge gates in those artifacts and Evidence describe their review-time
state; use this index for the current consolidated lifecycle.

Issue #94's cross-Specification clarification was explicitly approved on
2026-09-24: Work Item metadata policy reuses the existing `schemaVersion: 1`
`policies` document references without adding manifest fields or migration. The
separately versioned policy-document contract is now part of the approved amendment
and does not change the delivered Specification 002 manifest baseline.

Implementation Evidence:
[T01](002-lingo-project-initialization/evidence-t01.md),
[T02](002-lingo-project-initialization/evidence-t02.md),
[T03](002-lingo-project-initialization/evidence-t03.md),
[T04](002-lingo-project-initialization/evidence-t04.md).
T01–T04 established Project domain rules, application contracts and portable/local
codecs. POC #16–#18 delivered the initial CLI, filesystem persistence and local
installation. Specification 003 adds bounded Codex Runtime, GitHub Work Item and
sequential workflow adapters without completing the remaining Specification 002
Task DAG. #19/#20 were closed after POC acceptance without claiming their stronger
proof obligations were satisfied; those residual concerns are inputs to MVP
hardening. #21 was accepted/closed during the 2026-09-20 E2E POC acceptance.
This does not mark Specification 002 or parent #14 complete.

## 003 — E2E Codex POC

Implementation-authorized on 2026-09-19 and tracked by #30–#40.

| Artifact | Status |
|---|---|
| [Specification](003-e2e-codex-poc/spec.md) | **Approved** |
| [Clarifications](003-e2e-codex-poc/clarifications.md) | **Resolved** |
| [Plan](003-e2e-codex-poc/plan.md) | **Approved** |
| [Tasks](003-e2e-codex-poc/tasks.md) | **Approved / authorized** |
| T31–T40 Evidence / reconciliation | **Delivered / merged to `main` in PR #52** |
| [Final report](003-e2e-codex-poc/final-report.md) | **ACCEPTED — 2026-09-20** |

The requested standalone skill spelling `axiom:<skill>` is incompatible with the
validated Codex hyphen-case skill-name contract. The authorized reversible POC
mapping is `$axiom-<skill>`. #30/#40 were explicitly accepted and closed on
2026-09-20 after PR #52 merged to `main`.

Acceptance testing created MVP follow-ups
[#54](https://github.com/rgomids/axiom/issues/54),
[#55](https://github.com/rgomids/axiom/issues/55),
[#56](https://github.com/rgomids/axiom/issues/56) and
[#57](https://github.com/rgomids/axiom/issues/57). They record guided setup,
Intent-driven Work Item creation, provider-visible workflow progress and
argument-driven Runtime skill UX; they do not retroactively expand POC scope.

## 004 — Usable MVP v1 Baseline

[Specification](004-mvp-v1-baseline/spec.md): **Approved — human approval recorded on 2026-09-20**.

Issue [#94](https://github.com/rgomids/axiom/issues/94) defines the amendment for
a durable Work Item lifecycle anchor and metadata governance. Human review on
2026-09-24 explicitly approved FR-038–FR-044, AC-25–AC-31, T26–T29/S6 placement,
and the Specification 002 policy-reference clarification. S6 implementation was
then explicitly authorized on 2026-09-24 and is technically complete at
`56beb4fc310894ff8de128f52c6a96d22711bec8`; human acceptance is not inferred.

[Plan](004-mvp-v1-baseline/plan.md): **Approved — human approval recorded on
2026-09-20** together with ADR-0007 and ADR-0008.

[Tasks](004-mvp-v1-baseline/tasks.md): **Approved — human approval recorded on
2026-09-20 in [PR #72](https://github.com/rgomids/axiom/pull/72)**. The corrected
original final DAG has 25 vertical delivery units across S1–S7 and is reconciled
with the approved Plan. T01–T25 remain the approved historical decomposition.
The approved #94 amendment adds T26–T29 after S5 and renumbers the remaining
Slices to S7/S8. The amended DAG has 29 Tasks across S1–S8; T01–T25 remain the
historical decomposition and T26–T29 are the approved S6 additions. S1 is tracked in
[#75](https://github.com/rgomids/axiom/issues/75): T01 canonical
completion/provenance was accepted and merged in PR #74, and T02–T03 were
delivered through PR #83. S2 was delivered through PR #84 and is tracked in
[#76](https://github.com/rgomids/axiom/issues/76). S3 was explicitly authorized
on 2026-09-22 and is tracked in
[#77](https://github.com/rgomids/axiom/issues/77); T08 and T09 were delivered
through PR #85 and their deterministic tests are recorded in
[S3 Evidence](004-mvp-v1-baseline/evidence-s3.md). T09's separately authorized
bounded real-provider observation created Issue #90 and is complete. Human
acceptance is not inferred. S4 was delivered through PR #91. S5 was explicitly
authorized on 2026-09-23 and delivered through PR #93;
T14–T15 implementation, deterministic tests, and bounded real Codex Runtime
observation are recorded in
[S5 Evidence](004-mvp-v1-baseline/evidence-s5.md). Human acceptance is not
inferred. S6 T26–T29 were explicitly authorized and their technical delivery is
recorded in [S6 Evidence](004-mvp-v1-baseline/evidence-s6.md). S7 T16–T22 were
explicitly authorized on 2026-09-25 and are technically complete, including the
T18 Evidence retirement record (HD-S7-T18) and macOS 27.0 native T22 Evidence;
see [S7 Evidence](004-mvp-v1-baseline/evidence-s7.md). By HD-S7-T22 the Ubuntu
26.04 native rows were not executed and are deferred to the T24
clean-environment RC acceptance matrix, where they remain mandatory.

Issue [#97](https://github.com/rgomids/axiom/issues/97) records the product-scope
decision to add `S8 — Multi-runtime agent planning and multi-agent execution` and
move T23–T25 release-candidate acceptance to S9. T23–T25 retain their IDs and
historical RC objective; T24/T25 are expanded to validate and reconcile S8.
Human review on 2026-09-26 approved FR-045–FR-061, AC-32–AC-43,
[ADR-0009](../decisions/0009-parent-child-execution-graph.md), the amended Plan and
T30–T36 decomposition. Codex and Claude are the concrete S8 acceptance Runtime
paths; concrete models remain local Model Profiles, and token/cost budget governance
is future work. The approved S8 amendment DAG contains 36 Tasks across S1–S9.
S8/T30–T36 implementation was authorized on 2026-09-27 by explicit human decision in
[Issue #97 comment #5852650410](https://github.com/rgomids/axiom/issues/97#issuecomment-5852650410);
that authority does not extend to the real T36 Runtime run beyond its own gate,
push, merge, Provider mutation, release, deploy, credential provisioning, S9 or
human acceptance. The deterministic foundation and concrete T33/T35 delivery are
recorded in [S8 Evidence](004-mvp-v1-baseline/evidence-s8.md). T33 and T35 are
technically complete. The separately operator-authorized real Codex + Claude
[T36 lab journey](004-mvp-v1-baseline/evidence-t36/README.md) produced
`real_run_recorded` Evidence after the clean-state coordination fix. The corrected
local candidate is **S8 ready for human review**. Human acceptance and S9 release
work remain separately gated.

Issue [#81](https://github.com/rgomids/axiom/issues/81) now records the explicit
2026-09-27 S9 product-scope direction: public `axiom` CLI identity, automated
supported-platform release artifacts, a stable verified remote installer with
idempotent reinstall/safe owned upgrade, Codex + Claude first-run bootstrap, and
Axiom dogfooding through the S8 graph before RC acceptance. The proposed
Specification/Plan/Tasks amendment adds T37–T40 ahead of historical T23–T25,
bringing the proposed DAG to 40 Tasks, and reconciles the #81 release-selection
policy: latest stable by default, exact `--version` pins, exact-version-only
release candidates and no automatic downgrade. Product scope is recorded; the Task
amendment, implementation, prerelease publication and final MVP acceptance remain
separately gated.

Issue [#62](https://github.com/rgomids/axiom/issues/62) originally authorized
Specification and reconciliation only. After PR #70 resolved that gate, the
Plan was explicitly requested and authorized as process context for PR #71 only;
no separate GitHub authorization artifact was cited. PR #71 is the auditable
approval point, and neither the request nor checks/merge imply approval. The approved
Specification consolidates #54–#57 and #63–#68 into the clean-environment journey
from installation through explicit human acceptance.
It defines Project/Work Item UX, Provider workflow projection, Runtime selectors,
completion statuses, detailed Markdown artifacts, provenance, persistence,
compatibility, onboarding, acceptance criteria, and required Evidence.

Human review on 2026-09-20 recorded HD-1 through HD-4: checksummed macOS/Linux
binary distribution with an explicit release OS/architecture matrix, durable
machine-local detail artifacts with an ADR-required Execution/correlation
boundary, the bounded filesystem threat model with mandatory Specification
002/security/architecture reconciliation before Plan, and clean v1 as the
compatibility baseline without automatic POC migration. The complete Specification
was explicitly approved on 2026-09-20. HD-3 is formalized by
[ADR-0005](../decisions/0005-bounded-local-filesystem-threat-model.md) and the
dated Specification 002 H13 reconciliation; HD-2 is formalized by
[ADR-0006](../decisions/0006-machine-local-detail-artifacts.md) without adding an
operation-attempt domain entity. PR #70 merged that reconciliation to `main` and
received explicit human approval, resolving the Plan gate. Plan review identified
two new durable decisions:
[ADR-0007](../decisions/0007-local-publication-and-recovery-protocol.md) for the
shared local publication/recovery protocol and
[ADR-0008](../decisions/0008-minimal-machine-local-execution-record.md) for the
bounded sequential Execution record. Both were **Accepted by explicit human
decision on 2026-09-20**, and the Specification 004 Plan was Approved in the same
PR #71 decision. The corrected final 25-Task DAG was then **Approved by explicit
human decision in [PR #72](https://github.com/rgomids/axiom/pull/72) on
2026-09-20**, concluding the Tasks phase. On 2026-09-21 T01 was explicitly
accepted and merged in PR #74 and operational governance moved to the Slice
boundary. T02–T03 were delivered to `main` through PR #83; their reproducible
security/filesystem record is in
[S1 Evidence](004-mvp-v1-baseline/evidence-s1.md). S2 was then explicitly
authorized and T04–T07 were delivered through PR #84 with the reproducible record
in [S2 Evidence](004-mvp-v1-baseline/evidence-s2.md). S3 was explicitly authorized
on 2026-09-22; T08–T09 implementation and deterministic tests are recorded in
[S3 Evidence](004-mvp-v1-baseline/evidence-s3.md). T09's mandatory bounded
real-provider observation is complete at its documented historical revision.
Technical completion and current review corrections remain separate from human
acceptance. S4 was delivered through PR #91. S5 was explicitly authorized on
2026-09-23 and its T14–T15 technical record is in
[S5 Evidence](004-mvp-v1-baseline/evidence-s5.md). S6 technical delivery is
recorded in [S6 Evidence](004-mvp-v1-baseline/evidence-s6.md). S7 technical
delivery is recorded in [S7 Evidence](004-mvp-v1-baseline/evidence-s7.md). The
current T30–T35 delivery is recorded in
[S8 Evidence](004-mvp-v1-baseline/evidence-s8.md), including the real T36 journey.
The corrected candidate is ready for human review, not human-accepted. The
Ubuntu 26.04 native Evidence deferred to T24, now S9,
Provider mutation outside an exact authorized run, prerelease/release publication,
and final MVP acceptance remain separately gated. Accepted operational Slice
trackers are #75–#79 for S1–S5, #94 for S6, and #80 for S7. Issue #97 is the
approved S8 scope tracker; #81 is the S9 tracker with the 2026-09-27
product-scope expansion recorded.
