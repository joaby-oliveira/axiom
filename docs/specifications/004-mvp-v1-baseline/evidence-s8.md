# Evidence — MVP Slice S8: Multi-runtime Execution Graph Foundation

## Claim and authority boundary

This record describes existing implementation work for Specification 004 Slice
S8, tracked by [Issue #97](https://github.com/rgomids/axiom/issues/97). S8/T30–T36
implementation was authorized by explicit human decision on 2026-09-27 in
[Issue #97 comment #5852650410](https://github.com/rgomids/axiom/issues/97#issuecomment-5852650410).
That authority covers local code, tests, documentation, deterministic validation
and corresponding Evidence within the approved Specification 004, ADR-0009, Plan
and Tasks. It explicitly does not authorize the real T36 Codex + Claude run beyond
its own gate, push, merge, PR/Issue closure, Provider mutation, release, deploy,
credential provisioning or secret mutation, Runtime installation, S9/T23–T25 or
human acceptance of S8 or the MVP. The implementation foundation
is commit `21f4f7c365c89f48dbfe641beb765a14903a69ad`, with publication read-back
correction `e654f833d1786fef89dcd619533c8d36bc63c5e0`, based on `main` at
`8a9ca19fd260bc19f5bb288b449e8dc6df618194`; the review remediation described
below belongs to the same revision as this Evidence update.

The real T36 Codex + Claude acceptance graph was **not executed**. No Runtime
process, Runtime installation, credential read/provisioning, worktree creation,
Git integration, Provider mutation, release, deploy, merge, Issue closure, S9
work or human acceptance occurred. `command -v` observed both executable paths;
it did not prove Runtime availability, capability, version, Model Profile
allowlisting or real execution.

Current result:

| Task | State at `21f4f7c` |
|---|---|
| T30 | deterministic contracts, protected local configuration store, resolution and Codex/Claude adapter boundaries implemented; real operator configuration and live availability observations remain T36 inputs |
| T31 | deterministic proposal/validation foundation implemented and tested |
| T32 | graph/lineage/envelope publication foundation implemented; protected graph store and ADR-0008 non-migration observation tested |
| T33 | deterministic scheduler/process-control foundation implemented; concrete Git worktree creation/inspection and a real Runtime attempt remain incomplete |
| T34 | structured coordination contracts and protected stream store implemented and tested |
| T35 | preview/authority/roll-up foundation implemented with fake integration and combined-validation ports; concrete Git integration/reconciliation remains incomplete |
| T36 | envelope and Evidence schemas prepared; the real run remains separately gated and was not attempted |

Therefore S8 is **not technically complete** and is not ready for human
acceptance. T33 and T35 concrete Git boundaries, application/CLI composition,
the reviewed local Runtime/Model configuration and the real T36 graph remain
blocking.

## Environment and identity

| Fact | Observation |
|---|---|
| Repository | `rgomids/axiom` |
| Branch | `agent/s8-multi-runtime-execution` |
| Implementation commits | foundation `21f4f7c365c89f48dbfe641beb765a14903a69ad`; first read-back correction `e654f833d1786fef89dcd619533c8d36bc63c5e0`; review-remediation base `bcc4a46711a68e56389bd90d6b00d4a06e481a7f` and source revision containing this record |
| Go | `go1.26.1 darwin/arm64` |
| macOS | 27.0, build `26A428` |
| Architecture | `arm64` |
| Gitleaks | 8.30.1 |
| Codex path observation | `/Users/rgomids/.local/bin/codex` (`command -v` only) |
| Claude path observation | `/Users/rgomids/.local/bin/claude` (`command -v` only) |

Selected source digests for the review-remediated source represented by this
record:

| Path | SHA-256 |
|---|---|
| `internal/runtimeprofile/runtimeprofile.go` | `bb8e630280f4d40182739ac1f02a0a77bd29d285655ec0ff086b39120f54089d` |
| `internal/runtimeadapter/adapters.go` | `93d3a30a7f8b6bc0da40e63a1b3c8a2f4f7226f44807ed8e6651927d9223d355` |
| `internal/executiongraph/planner.go` | `5cf86aea3701ac6752d13ae886996b9fbc05a9eeb0482e0c4b5317ccac66212a` |
| `internal/executiongraph/graph.go` | `7381f570a8c52b140de93c3402b1d79f16869c67635cab9d2f610254442041a2` |
| `internal/executiongraph/scheduler.go` | `4cc4c9915e1e1b51cdb769419e0e6727e58a7d9a09aa4a1d2f91f2e83369f793` |
| `internal/executiongraph/integration.go` | `b5dc296f9985fcf1795a06ecae51d522b64d1f2aa077db3ce587fd76eb173950` |
| `internal/executiongraph/acceptance.go` | `d8e4ff97259d441d0f49bbf0abf6d1e3c4e276a715e0266c89cfaf59d6a18a23` |
| `internal/coordination/coordination.go` | `fe4c401dfc36e2f375b5342592c554f3adb04bf398b730ba76bc99e9f778daf0` |

## Implemented deterministic contracts

### T30 — Runtime/Model Profile resolution

- Closed format v1 accepts only the approved `codex` and `claude` adapters.
- Configuration separates enabled adapters, local concrete Model Profiles,
  allowlists, optional role/complexity preferences and credential references.
- Resolution observes installed/available/proven capability facts through a
  read-only consumer port and returns exactly one allowed match or a blocker.
- Multiple matches block instead of using vendor/model ranking. Missing,
  disabled, unavailable, declared-only capability and stale-revision cases do
  not fall back.
- Protected local create/update/load uses the existing ADR-0007 publication
  protocol. Interrupted publication keeps readers fail-closed and preserves the
  prior canonical generation.
- Concrete adapters render explicit Codex/Claude argument vectors from local
  profiles. Credential values are resolved at invocation time behind a reference
  and are not stored in graph records.

### T31–T32 — Proposal, validation and graph publication

- Planner input is structured approved work, not a fixed team template.
- Validation rejects cycle, missing dependency, orphan, unsafe parallel overlap,
  invalid scope/control, absent integration owner and unresolvable capability
  before identity allocation or publication.
- Preview ordering and digest are deterministic.
- Publication allocates opaque parent, child and attempt identities; binds graph
  revision, proposal digest, authority ceiling, exact child subsets, local
  Runtime/Model resolution references, isolated workspace, timeout and maximum
  attempts.
- Decoder revalidates dependency acyclicity, lineage, one integration owner,
  Project consistency, authority subsets, identity uniqueness and attempt state.
- Graph records live under a separate `graphs/v1` store. The graph-store test
  verifies no `executions/` state is synthesized; existing ADR-0008 readers and
  their full repository suite remain unchanged.
- Graph publication runs an F0–F8 injected-fault matrix. A reader sees either a
  complete valid graph, not-found, or `recovery_required`, never mixed state.
- Publication read-back canonically encodes and digests the entire intended and
  loaded graph. Any structurally valid change to effects, authority references,
  Runtime resolution, workspace or controls fails publication confirmation.

### T33 — Scheduler foundation

- Ready selection consumes the committed graph only. Dependencies and the
  integration child remain blocked until their contracts are satisfied.
- Independent non-conflicting children start concurrently in tests; conflicting
  effects are deterministically serialized.
- Workspace and invocation ports fail before process start. Invocation requires
  absolute executable and CWD, exact Runtime correlation, bounded environment,
  bounded output and rejects shell executables.
- Attempt `running` state is published before process invocation; final outcome
  replaces that attempt and advances storage revision.
- Timeout/cancellation is bounded. Unobservable full stop remains `unknown`;
  confirmed stop may be `cancelled`. Cancellation prevents new dispatch.
- Retry requires an explicit child ID, remaining maximum attempts and a
  reconciled outcome. `unknown` itself blocks redispatch even if a stored
  ambiguity flag is inconsistent; every permitted retry gets a new attempt
  identity and preserves prior attempts.

Concrete Git worktree creation, base/ownership inspection and cleanup disposition
are still missing. The workspace port and fake tests are not proof of isolated
real worktrees.

### T34 — Structured coordination

- Nine closed record kinds cover question/request, answer, contract proposal,
  contract acceptance, blocker, dependency resolution, artifact publication,
  progress and result.
- Records bind parent/child/graph/attempt lineage, monotonic revision, previous
  digest, provenance, bounded fields and optional usage observation.
- Unknown/control/raw-chat/reasoning/credential fields, stale revisions,
  unsupported kinds, forged lineage and oversized values are rejected.
- Field values are single-line structured data. Deterministic high-signal checks
  reject private-key boundaries, bearer credentials, explicit password/token/API
  key/secret assignments and known credential formats without rejecting ordinary
  discussion of token budgets or secret-handling policy.
- Usage is only `measured`, `partial` or `unavailable`; unavailable values cannot
  carry an estimate.
- Protected streams are bounded to 64 records and use ADR-0007 publication.

### T35 — Integration foundation

- Preview verifies child lineage, latest successful attempt, envelope digest,
  base revision, result tree, artifacts and exact effect set.
- Missing required output, forged lineage, stale base, conflict or foreign drift
  blocks before apply. Missing optional output requires an explicit waiver
  reference bound to that exact child. Ordered structured waiver records are
  included in the preview and therefore in its digest.
- Apply authority binds preview digest, target revision and exact ordered effects.
- Only the declared integration child reaches the integration and combined-
  validation ports. Parent roll-up cannot become `success` without confirmed
  integration plus passing validation references.

Concrete Git result inspection, patch application, conflict preview and combined
command execution remain ports exercised with fakes. T35 is therefore partial.

### T36 — Evidence status derivation

- The caller-controlled `RealRuntimeRun` boolean no longer exists.
- `real_run_recorded` derives only from structured Codex and Claude execution
  records correlated to independent graph children, successful persisted
  attempts, exact dispatch timestamps, Runtime/Model Profile resolutions and
  result references.
- Independence is not treated as concurrency: at least one Codex/Claude pair of
  independent children must show strict temporal overlap derived from their
  dispatch records (`codex.StartedAt < claude.EndedAt` and
  `claude.StartedAt < codex.EndedAt`); touching windows (`EndedAt == StartedAt`)
  and sequential or dependent children do not qualify.
- Coordination is structured Evidence derived from canonical records, not
  caller-asserted facts. Callers supply canonical encoded coordination records
  (`Evidence.CoordinationRecords`); `BuildEvidence` rejects any caller-populated
  `Coordination` and derives each `CoordinationEvidence` (record ID, kind,
  parent, graph revision, child, optional attempt, digest, reference, decision)
  only through the acceptance-owned `CoordinationVerifier` port. The production
  implementation, `coordination.AcceptanceVerifier`, accepts only bytes that
  decode strictly, pass `ValidRecord` (digest recomputed from content) and
  re-encode to the identical canonical form; content altered after digesting,
  an arbitrary SHA-256 digest and non-canonical encodings are rejected. Lineage
  is then validated against the graph, so a resealed record with foreign parent,
  graph revision, child or attempt is still rejected. The verifier is chosen by
  the composition root; it is not proof against arbitrary in-process code. The
  journey requires a `question_request` answered by a correlated `answer`, or a
  `contract_proposal` followed by a correlated `contract_acceptance` with
  decision `accepted`, exchanged between two distinct children during
  Runtime-executed attempts.
- Successful Integration/Reconciliation roll-up, a digested result tree,
  successful combined validation linked to declared validation references and
  integration references are also mandatory.
- Missing journey facts remain `deterministic_preparation_only`; foreign
  Runtime/Profile/attempt claims and coordination records with foreign parent,
  graph revision, child or attempt (including an attempt of another child),
  tampered or non-canonical content, arbitrary digests, caller-asserted facts
  or duplicate identity are rejected. Journey tests publish records through
  `coordination.Service` and verify them through `coordination.AcceptanceVerifier`.

These are deterministic contract tests only. No real Codex + Claude Runtime
journey was executed, and no `real_run_recorded` Evidence exists.

## Final validation commands

| Command | Result |
|---|---|
| `go test ./... -count=1` | exit 0; all packages passed |
| `go test -race ./internal/runtimeprofile ./internal/runtimeadapter ./internal/executiongraph ./internal/coordination ./internal/local -count=1` | exit 0; all five packages passed |
| `go vet ./...` | exit 0 |
| `go build ./...` | exit 0 |
| `go mod verify` | exit 0; all modules verified |
| `./scripts/validate-repository.sh .` | exit 0; repository/package/bootstrap validators passed |
| `./scripts/check-sensitive-files.sh .` | exit 0; worktree passed |
| `gitleaks dir . --redact --no-banner` | exit 0; no leaks found |
| `git diff --check` | exit 0 |

These validations cover the review-remediated source represented by this record.
They prove deterministic checks only; they do not supply real Runtime Evidence,
T36 real-run authority or human acceptance.

## Proposed T36 real-run envelope — draft, not authorized

This draft selects a small real Axiom change: add a read-only
`lingo runtime profile validate` command and its command documentation. It is not
an executable authority record yet because concrete local Model Profiles and the
concrete Git worktree/integration adapters are unresolved.

| Field | Proposed value |
|---|---|
| Base revision | implementation/review head after T33/T35 concrete adapters land |
| Repository | `/Users/rgomids/Projects/axiom` |
| Parent scope | one local-only Axiom change; no Provider/network/release effects |
| Child A | Codex; Model Profile ID `t36-codex-implementation`; implement CLI/service/tests under `internal/cli`, `cmd/lingo` and directly required composition |
| Child B | Claude; Model Profile ID `t36-claude-documentation`; update `docs/commands.md` for the exact delivered read-only command |
| Dependencies | A and B independent; Integration/Reconciliation depends on both |
| Integration child | local integration profile; validate lineage, clean bases and exact diffs; integrate A then B; run combined validators |
| Worktrees | three absent paths under a reviewed dedicated parent directory: one per child and one integration target; exact paths must be regenerated from the final graph |
| Allowed effects | A: named CLI/service/test paths only; B: `docs/commands.md` only; integration: those exact child effect sets only |
| Forbidden effects | push, merge, Provider mutation, release, deploy, secret/config mutation, Runtime install, purchase/limit change, unrelated files |
| Timeout | 20 minutes per implementation/documentation child; 15 minutes integration |
| Maximum attempts | 1 initial + at most 1 explicitly reviewed retry per child; no retry after ambiguous effect |
| Commands | exact Codex/Claude argv rendered from the reviewed local Model Profiles; no shell; exact `cwd`; bounded environment/output |
| Combined validators | `go test ./... -count=1`; targeted race tests; `go vet ./...`; `./scripts/validate-repository.sh .`; `./scripts/check-sensitive-files.sh .`; `git diff --check`; Gitleaks |
| Expected Evidence | config/graph/envelope digests; Runtime/Profile/version observations; overlap timing; worktree/base/tree hashes; structured coordination exchange records; child results; integration preview/effects; validator exits; parent roll-up; usage status; limitations |
| Cleanup disposition | preserve all three worktrees and graph/Evidence pending human review; cleanup requires a later exact decision |

Before this envelope can be approved, implementation must add the concrete Git
workspace and Integration/Reconciliation adapters plus application composition;
the operator must publish exact local `t36-codex-implementation`,
`t36-claude-documentation` and integration profiles; and the generated envelope
must replace every descriptive path/argv with exact digested values. No real
Runtime run may begin from this draft.
