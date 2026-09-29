# Architecture Decision Records

Este diretório registra decisões técnicas duráveis e difíceis de reverter.

## Index

- [ADR-0001 — Project is not Repository](0001-project-is-not-repository.md) — Accepted, 2026-08-08.
- [ADR-0002 — Axiom Relationship with GitHub Spec-Kit](0002-axiom-speckit-relationship.md) — Accepted, 2026-09-10.
- [ADR-0003 — Lingo as Axiom Local Control Plane](0003-lingo-as-axiom-local-control-plane.md) — Accepted, 2026-09-10; implementation requires an approved Specification.
- [ADR-0004 — Portable Project Manifest](0004-portable-project-manifest.md) — Accepted, 2026-09-12; versioned portable intent, identity/evolution and local-state separation, with explicit Git authority boundaries.
- [ADR-0005 — Bounded local filesystem threat model](0005-bounded-local-filesystem-threat-model.md) — Accepted, 2026-09-20; exact supported local filesystem properties and explicit same-UID/power-loss/media exclusions from HD-3.
- [ADR-0006 — Machine-local detail artifacts](0006-machine-local-detail-artifacts.md) — Accepted, 2026-09-20; central local ownership, Execution-first correlation, Evidence references, retention and safe cleanup from HD-2.
- [ADR-0007 — Local publication and recovery protocol](0007-local-publication-and-recovery-protocol.md) — **Accepted, 2026-09-20**; shared logical publication states, coordination, commit truth, prior/new generations and fail-closed recovery under ADR-0005.
- [ADR-0008 — Minimal machine-local Execution record](0008-minimal-machine-local-execution-record.md) — **Accepted, 2026-09-20**; bounded sequential workflow authority, revisioned transitions, resume and cross-boundary correlation without defining a general Execution graph.
- [ADR-0009 — Parent/child Execution Graph](0009-parent-child-execution-graph.md) — **Accepted, 2026-09-26**; revisioned DAG, lineage, authority, isolation, integration, structured coordination and cross-Runtime Evidence. S8/T30–T36 implementation was authorized on 2026-09-27 (Issue #97 comment #5852650410); the real T36 Runtime run remains separately gated.

## Candidate assessment

| Candidate | Current classification | ADR now? |
|---|---|---|
| Codex-first harness | Accepted bounded product scope for current harness; not a permanent single-runtime commitment. | No. Existing scope is explicit and reversible. |
| Go for future CLI | Go foundations implemented; CLI framework and distribution remain undecided. | Later, with an approved CLI specification and alternatives evidence. |
| Provider abstraction | Accepted boundary principle; concrete ports and adapters remain open. | Later, when a specified integration creates a durable contract. |
| Local versus remote control-plane topology | Local Lingo direction accepted; remote and hybrid alternatives remain revisit paths. | Accepted in ADR-0003; detailed behavior still requires Specification evidence. |
| Portable Project Manifest | Versioned shareable Project intent, currently axiom.yaml, distinct from local state. | Accepted in ADR-0004; concrete v1 contract belongs to Specification 002/Plan. Identity/location, incremental update and optional Git backing authority extend this boundary; concrete storage/sync engines and aggregate ownership remain open. |
| Spec-Kit relationship | Independent Axiom implementation informed by Spec-Kit as a strategic upstream reference. | Accepted in ADR-0002; future adapters or compatibility contracts require separate evidence and approval. |
| Execution and Evidence model | ADR-0006 resolves detail-artifact ownership/correlation; ADR-0008 remains the accepted sequential/historical contract. Issue #97 requires parent/child DAG semantics accepted in ADR-0009. | ADR-0009 is Accepted because graph lineage, authority, dependencies, integration, cancellation/retry, coordination and Evidence are durable/cross-cutting. Its acceptance does not authorize S8 implementation. |
| Local filesystem threat model | Bounded local MVP model with required confinement, process concurrency, deterministic faults, complete canonical state and guided recovery; arbitrary malicious same-UID interleavings and physical/media durability excluded. | Accepted in ADR-0005; concrete mechanisms remain Plan/implementation work. |
| Local publication and recovery protocol | Specification 004 Plan defines shared coordination, private preparation, protected publication, commit-point, prior/new generation and reader/recovery semantics across mutable local stores. | ADR-0007 is Accepted. It consumes ADR-0005 without turning syscalls, filenames, layouts, libraries or Go packages into architecture. |
| Lingo local control plane | Accepted architectural direction separating executable workflows from the Axiom domain and thin runtime skills. | Accepted in ADR-0003; do not implement before an approved Specification. |

ADRs futuros devem registrar, no mínimo:

- status;
- contexto;
- decisão;
- alternativas consideradas;
- consequências e trade-offs;
- evidências ou specifications relacionadas.

Hipóteses de pesquisa não são decisões. Não crie um ADR apenas para preencher a árvore documental.
