# ADR-0009 — Parent/child Execution Graph

## Status

**Accepted — explicit human decision recorded on 2026-09-26.**

Issue #97 records the product decision to add multi-runtime Agent Planning and
multi-agent execution to Specification 004. Human review accepted the durable
architecture below on 2026-09-26. That decision does not authorize S8
implementation, Runtime dispatch or external effects.

This decision evolves ADR-0008 for new graph executions. ADR-0008
remains the correct historical architecture for S4–S7 and the compatibility
contract for existing sequential Execution records.

## Decision in one sentence

Axiom should represent coordinated work as one revisioned machine-local DAG of a
parent Execution and bounded child Executions, with explicit lineage, dependency,
authority, isolation, integration and Evidence semantics independent of Agent,
Role, Runtime and Model identity.

## Context

ADR-0008 deliberately chose one minimal sequential Execution record and deferred a
general graph until approved requirements existed. Issue #97 now requires Axiom to
plan child work, resolve it across operator-authorized Runtime/Model Profiles,
execute independent work concurrently, coordinate bounded dependencies, integrate
isolated results and reconcile one truthful parent result.

These semantics cross workflow authority, local persistence, Runtime resolution,
repository isolation, cancellation/retry, completion, Evidence and compatibility.
They are expensive to change after child identities and graph revisions appear in
artifacts or retained Evidence, so an ADR is required before implementation.

## Constraints

- `Execution != Agent`: an Execution is durable workflow identity; an Agent or
  Runtime process may perform an attempt but never becomes that identity.
- `Role != Model`: a role/capability request is resolved separately from an
  operator-approved Runtime/Model Profile.
- Axiom owns domain contracts; Lingo coordinates local execution under ADR-0003.
- Project remains distinct from Repository. Graph state remains machine-local and
  never enters portable Project intent by convenience.
- Provider projection, raw Runtime chat and repository state cannot create or
  advance canonical graph truth.
- ADR-0005 confinement, ADR-0006 artifact ownership and ADR-0007 publication and
  recovery apply to every graph-owned record and isolated workspace.
- Existing ADR-0008 records must remain readable and resumable without synthetic
  lineage or migration by inference.

## Decision

### Parent, child and graph semantics

1. A parent is an Execution with one stable opaque identity and one revisioned
   graph. Each node is a child Execution with its own stable opaque identity.
2. A child records exactly one parent identity and graph revision. Parent and
   child identities are never derived from path, role, Agent, Runtime, Model,
   Provider, Work Item or one another.
3. The graph is finite and acyclic. Edges name declared completion prerequisites;
   process timing, chat order and observed file changes do not create dependencies.
4. A graph proposal does not grant authority or start work. Material changes to
   nodes, edges, scope, effects, Runtime/Model constraints or execution controls
   require a new validated revision and applicable operator decision.
5. Existing sequential Executions remain valid under ADR-0008. New sequential work
   may use a one-node graph where graph correlation is required, but existing
   records are not rewritten or assigned invented parents.

### Child scope, authority and resolution

6. Every child has a bounded envelope: required role/capabilities, inputs, expected
   outputs, Project/Repository target, workspace, dependencies, allowed effects,
   authority references, validation obligations, timeout and maximum attempts.
7. Parent authority is a ceiling, not an ambient grant. Each child receives only
   its explicit subset and cannot delegate, widen or refresh it. Stale observations
   invalidate the affected authority before effects.
8. Runtime/Model Profile resolution consumes capability and complexity requirements
   plus operator configuration. It selects only installed, enabled, available and
   allowlisted choices. No-match blocks; fallback requires an explicit newly
   authorized graph/envelope revision. Codex and Claude are the two concrete S8
   acceptance Runtime paths, never domain identities or permanent vendor coupling.
9. Credentials remain behind Runtime/Integration-owned machine-local references.
   They are not copied into graph, coordination, prompt, artifact or Evidence
   content. A child gets only the minimum credential capability for its effects.

### Dependency, isolation and integration

10. A child becomes dispatchable only when its declared prerequisites satisfy the
    edge contract and its scope, authority, workspace, Runtime, timeout and attempt
    policy are valid.
11. Dependency-independent children may run concurrently only when effect sets do
    not conflict. Repository-mutating children use isolated workspaces/worktrees;
    shared targets serialize or block.
12. Child output is untrusted until structurally validated and correlated. No child
    silently merges, rebases, pushes or publishes into the delivery branch.
13. One declared Integration/Reconciliation child owns the bounded integration
    boundary. It validates child results, detects drift/conflict, integrates only
    authorized changes, runs combined validation and publishes the delivery result
    to the parent. It cannot manufacture missing child success or new authority.

### Coordination ownership

14. Axiom owns bounded structured coordination records for question/request,
    answer, contract proposal/acceptance, blocker, dependency resolution, artifact
    publication, progress and result. Each record is typed, revisioned, correlated
    and provenance-marked.
15. Raw chat, hidden reasoning and unrestricted Runtime output are neither workflow
    state nor Evidence. Conversational projections may be rendered from structured
    records but cannot override them.
16. Untrusted child content cannot mutate graph, authority, allowlist, dependency,
    execution-control or human-gate state. Such changes require deterministic
    validation and the same explicit decision as direct operator input.

### Cancellation, retry and partial state

17. Cancellation prevents new dispatch and requests bounded cancellation of
    affected active attempts. Confirmed effects remain confirmed; inability to
    observe a stopped Runtime remains unknown, not cancelled.
18. Retry creates a new attempt identity beneath the same child Execution and
    preserves all earlier attempts. It requires remaining scope, authority and an
    available attempt under the explicit maximum. Ambiguous external effects must
    reconcile before retry.
19. Child outcomes include not-started, running, blocked, succeeded, failed,
    cancelled, skipped and unknown with bounded reasons and references. Exact wire
    values may be refined by approved Tasks, but these meanings cannot collapse.
20. Parent roll-up is deterministic from the graph revision and child outcomes.
    Parent success requires all required dependency contracts plus successful
    Integration/Reconciliation. Optional/waived outcomes that change delivered
    scope require an explicit human decision.

### Evidence, provenance, usage and retention

21. Every attempt retains parent/child/graph identity, child envelope, resolved
    Runtime/Model Profile and observable version, configuration reference,
    authority, timeout/attempt controls, timestamps, result, artifacts, validations,
    coordination references and any measured usage/cost source/unit/status.
22. Runtime-provided usage/cost is observational telemetry, not S8 budget authority.
    Missing data is recorded as `unavailable`; partial data remains `partial`.
    Axiom does not estimate missing values or normalize incompatible cross-Runtime
    units.
23. Parent Evidence summarizes and references child Evidence, integration and roll-
    up facts. It excludes raw chat, hidden reasoning, credentials and unbounded
    output.
24. Graph and coordination records are bounded, machine-local, reference-aware and
    retained while required for resume, reconciliation, Evidence or explicit
    acceptance. Cleanup follows ADR-0006/0007 and preserves uncertainty.

## Alternatives considered

### Keep one sequential Execution and coordinate Agents through chat

Small change, but no durable dependency, authority, lineage, retry, cancellation or
Evidence semantics. Raw chat would become hidden workflow state and cross-Runtime
recovery would be unreliable.

### Treat each child as independent and reconstruct a parent report afterward

Allows parallel execution, but loses one authoritative graph revision, bounded
execution controls, dependency readiness, cancellation ownership and deterministic
partial roll-up. Reconstruction from artifacts or Provider state would repeat the
failure ADR-0008 already rejected.

### Make Agent/Runtime sessions graph nodes

Simplifies dispatch bookkeeping but violates `Execution != Agent`, couples durable
identity to vendor processes, and makes resume, retry and model changes ambiguous.

### Revisioned parent/child Execution DAG with bounded coordination

Adds a versioned local contract but preserves domain identities, exact authority,
Runtime independence, deterministic scheduling truth and inspectable Evidence. This
is the accepted option.

## Consequences

### Positive

- Independent work can run concurrently without making completion order canonical.
- Runtime/Model choice remains operator-controlled and replaceable.
- Child authority, timeouts, attempts and isolated outputs remain attributable.
- Integration and parent completion gain one explicit ownership boundary.
- Sequential ADR-0008 history remains compatible and inspectable.

### Negative / trade-offs

- Axiom owns new versioned graph, attempt and coordination-state formats.
- Correct recovery now spans graph revision, active Runtime observation, worktrees,
  confirmed effects and integration state.
- Cross-Runtime usage/cost cannot always be reduced to one complete comparable
  number.
- Worktree lifecycle and concurrent process cancellation add operational cost.
- Identity/lineage/authority changes after release may require local migration and
  Evidence reconciliation.

## What would invalidate this recommendation

- Product scope changes to remote multi-machine or multi-tenant orchestration.
- A Runtime cannot expose a safe bounded process/cancellation or output contract.
- Required collaboration needs portable shared graph authority instead of local
  Lingo ownership.
- Evidence proves a DAG is insufficient and requires dynamic cyclic negotiation or
  another workflow formalism.

Any such change needs a new specification and explicit architectural review; it
must not be smuggled into implementation details.

## Revisit when

- remote collaboration or durable shared scheduling becomes approved scope;
- dynamic graph expansion is required after dispatch;
- aggregate cost governance needs a provider-neutral monetary accounting contract;
- sequential-record compatibility needs an explicit migration rather than dual
  reading;
- more than one Integration/Reconciliation topology is required.
