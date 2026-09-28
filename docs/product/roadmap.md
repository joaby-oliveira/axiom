# Product Roadmap

## Purpose

Capability direction and dependencies, not a task tracker, changelog or delivery
commitment. Approval and implementation status live in the
[Specification index](../specifications/README.md); detailed scope and Evidence
remain with each Specification. Nothing here authorizes implementation.

## Established foundation

- Axiom owns product/domain/policies/contracts; Lingo is the accepted local-first
  executable control-plane direction under [ADR-0003](../decisions/0003-lingo-as-axiom-local-control-plane.md).
- [ADR-0004](../decisions/0004-portable-project-manifest.md) establishes portable
  Project intent, immutable identity, mutable slug/name and separate local state.
- The Codex-first harness exercises the development lifecycle. Tested Go
  foundations cover Project invariants, application contracts and authority,
  portable manifests and local record codecs. See [Architecture](../architecture/README.md).

The repository now includes an accepted executable E2E Lingo/Codex POC: local
install, global thin skills, Project bindings/resolution, GitHub Work Items and a
bounded sequential workflow. Human acceptance was recorded on 2026-09-20 after
PR #52 merged to `main`. Its commands, limits and acceptance status are recorded
in the [Specification index](../specifications/README.md). Acceptance does not
authorize later roadmap work.

## MVP specification

Manual POC acceptance identified four concrete experience gaps that should inform
the MVP:

- [#54](https://github.com/rgomids/axiom/issues/54) — guided Project setup that
  asks for Project identity, repositories and Work Item Provider instead of
  inferring the caller CWD.
- [#55](https://github.com/rgomids/axiom/issues/55) — Intent-driven Work Item
  creation interview and structured issue draft.
- [#56](https://github.com/rgomids/axiom/issues/56) — provider-visible Axiom
  workflow stage plus bounded structured transition/Evidence comments.
- [#57](https://github.com/rgomids/axiom/issues/57) — explicit Runtime skill
  selectors/arguments with questions only for missing inputs.

The [approved MVP Specification](../specifications/004-mvp-v1-baseline/spec.md),
tracked by [#62](https://github.com/rgomids/axiom/issues/62), now consolidates
these experience gaps with completion output, detailed artifacts, provenance,
persistence/recovery/compatibility, installation/upgrade, and clean-environment
acceptance. Human review on 2026-09-20 recorded the four material directions:
checksummed macOS/Linux binaries with an explicit release OS/architecture matrix,
durable machine-local detail artifacts with an ADR-required ownership boundary,
the bounded filesystem threat model with mandatory Specification 002/security/
architecture reconciliation before Plan, and clean v1 as the compatibility
baseline without automatic POC migration. The Specification was explicitly
approved on 2026-09-20. The authorized reconciliation records
[ADR-0005](../decisions/0005-bounded-local-filesystem-threat-model.md) for HD-3,
[ADR-0006](../decisions/0006-machine-local-detail-artifacts.md) for HD-2, and H13
updates across Specification 002. PR #70 merged that reconciliation and received
explicit human approval. The resulting
[Specification 004 Plan](../specifications/004-mvp-v1-baseline/plan.md) was
explicitly approved on 2026-09-20. Review identified
[ADR-0007](../decisions/0007-local-publication-and-recovery-protocol.md), defining
the shared local publication/recovery protocol, and
[ADR-0008](../decisions/0008-minimal-machine-local-execution-record.md), defining
the bounded machine-local sequential Execution record. Both were Accepted in the
same PR #71 human decision. The resulting
[Tasks artifact](../specifications/004-mvp-v1-baseline/tasks.md) originally decomposed S1–S7
into a corrected final DAG of 25 vertical units with explicit dependency and
Evidence ownership. It was Approved by explicit human decision in
[PR #72](https://github.com/rgomids/axiom/pull/72) on 2026-09-20, concluding the
Tasks phase. S1–S7 are technically delivered on `main`; their Evidence remains in
Specification 004 and technical delivery does not infer human acceptance. T01–T25
remain the approved historical decomposition, with T26–T29 added by the approved
Issue #94 amendment. Provider mutation outside exact authority, successor-slice
implementation, migration execution and release remain separately gated. Delivered
Slice trackers are #75–#80, with #94 owning S6.

Issue [#97](https://github.com/rgomids/axiom/issues/97) records the 2026-09-26
product-scope decision to add `S8 — Multi-runtime agent planning and multi-agent
execution` after delivered S7 and move release-candidate acceptance to S9 while
preserving T23–T25 IDs and historical RC objective while expanding T24/T25 for S8
validation/Evidence. Human review on 2026-09-26 approved Specification
FR-045–FR-061, AC-32–AC-43,
[ADR-0009](../decisions/0009-parent-child-execution-graph.md), the Plan amendment
and T30–T36 Tasks. Codex and Claude are the two concrete S8 acceptance Runtime
paths; token/cost budget governance remains future work. This approval does not
authorize S8 implementation, Runtime dispatch or external effects.

Issue [#81](https://github.com/rgomids/axiom/issues/81) records the 2026-09-27
S9 productization direction. The final MVP slice now closes the gap between a
repository-local implementation and a distributable product before RC acceptance:
the canonical public executable is `axiom`; supported native artifacts are
prepared through repeatable automation; a stable verified macOS/Linux bootstrap
installs without source checkout and converges owned reinstall/upgrade safely;
`axiom first-run` configures every supported detected Runtime, including Codex
and Claude; and S9 acceptance dogfoods real Axiom engineering through Axiom's own
S8 orchestration path. Historical T23–T25 remain the RC/acceptance tail, while
proposed T37–T40 add the productization work before them. Implementation,
publication and final acceptance remain separately gated.

## Complete Specification 002

[Specification 002 — Lingo Project Initialization](../specifications/002-lingo-project-initialization/spec.md)
is the first active vertical slice. Its approved scope connects minimal Project
creation, explicit update, validation, reopening and local installation.
The POC covers only the minimal one-file path. Completing Specification 002 still
requires separately authorized delivery of its remaining application, security,
recovery and acceptance contracts under the
[Plan](../specifications/002-lingo-project-initialization/plan.md).

Minimal init requires identity and schema version, without mandatory Repository,
Runtime or Provider configuration. Update must materialize and validate complete
proposed state before publication. Portable working copy and ID-addressed local
state remain separate; operational installation must prove the safety and recovery
contracts, not merely serialize valid data.

Git backing remains optional and independent of Repository association. Project
mutation, optional local commit and explicitly authorized remote push/sync are
separate operations. Concrete Git execution and synchronization are outside this
slice's delivery; the authority boundary does not select their mechanisms.

## Later capabilities requiring specification

The following possibilities have no approved delivery order. Each needs a bounded
Specification and explicit authority before implementation; dependencies explain
what must be understood first.

| Capability | Dependency or question to resolve |
|---|---|
| Runtime/model ecosystem beyond S8 | Adapter lifecycle and capability reporting beyond the two bounded acceptance Runtime paths |
| Dynamic or distributed orchestration | Portable/shared authority, remote scheduling, graph expansion and multi-machine recovery |
| Additional Runtime skills and adapters | Validate each additional Runtime without generalizing the bounded S8 allowlist into an arbitrary plugin ecosystem |
| Runtime budget governance | Define provider-neutral token/cost/aggregate-parent semantics before any enforcement; observational telemetry remains non-equivalent and may be unavailable |
| Richer Project wizard | Runtime/Integration discovery and setup contracts beyond the minimal guided flow already specified |
| Portable distribution and synchronization | Independent local/remote Git authority, conflict handling, recovery and explicit compatibility policy |

Provider choices remain independent; `Role != Model`, `Execution != Agent`,
`Provider != Transport` and `Integration != MCP`. A selected declaration never
proves connectivity or operational readiness. Evidence and human prioritization
may combine, reorder, split or defer these capabilities.
