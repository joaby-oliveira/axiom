# Evidence — MVP Slice S8: Concrete graph delivery ready for T36

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
human acceptance of S8 or the MVP. The deterministic foundation was delivered by
PR #102. Concrete T33/T35 delivery is commits
`e884a468f6b0cc76835dd671ac9f29ea6779919a` and
`7bfadc3c47dcb453a97efc5b77aab0ef1824d3bc`, based on merged `main` at
`5dd9660411daadc100d5260e5ff54a9f9d11f945`. The PR #103 technical-review
remediation (persisted Integration/Reconciliation attempt, fail-closed Git
filter/diff-driver guard, private WorkspaceRoot/control paths) is recorded in
the T33/T35 sections below and in the digest table.

The real T36 Codex + Claude acceptance graph was **not executed**. No Codex or
Claude Runtime process, Runtime installation, credential read/provisioning, Provider mutation,
release, deploy, merge, Issue closure, S9 work or human acceptance occurred.
Deterministic tests did create temporary local Git repositories and worktrees,
execute the test binary as two concurrent bounded child processes, integrate their
real file changes and run a concrete combined validator. Those tests are T33/T35
Evidence, not a substitute for real Codex + Claude T36 Evidence.

Current result:

| Task | Current state |
|---|---|
| T30 | deterministic contracts, protected local configuration store, resolution and Codex/Claude adapter boundaries implemented; real operator configuration and live availability observations remain T36 inputs |
| T31 | deterministic proposal/validation foundation implemented and tested |
| T32 | graph/lineage/envelope publication foundation implemented; protected graph store and ADR-0008 non-migration observation tested |
| T33 | **technically complete**: concrete Git worktree creation/ownership/inspection is composed with the scheduler and `OSProcessRunner`; deterministic real-Git tests cover confinement, drift and concurrent local dispatch |
| T34 | structured coordination contracts and protected stream store implemented and tested |
| T35 | **technically complete**: concrete Git result inspection, conflict preview, authority-bound apply, combined validation and parent roll-up are composed and tested against real temporary repositories |
| T36 | **pending and ready to execute**: exact local Model Profiles/run envelope plus the real Codex + Claude acceptance journey remain the only technical step for S8 |

Therefore S8 is **not technically complete** and is not ready for human
acceptance. T30–T35 are technically complete. Only the separately gated real T36
Codex + Claude journey remains. No `real_run_recorded` claim exists.

## Environment and identity

| Fact | Observation |
|---|---|
| Repository | `rgomids/axiom` |
| Branch | `agent/s8-t33-t35-concrete-git` |
| Base | `5dd9660411daadc100d5260e5ff54a9f9d11f945` |
| Concrete T33/T35 implementation | `e884a468f6b0cc76835dd671ac9f29ea6779919a`, `7bfadc3c47dcb453a97efc5b77aab0ef1824d3bc` |
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
| `internal/executiongraph/scheduler.go` | `74f86a010f9a4cd88c3f2dadf298c78dea64783ec6a44a658db593f0f5200229` |
| `internal/executiongraph/integration.go` | `fa739395154fb2fd07a4e6a08f0a4ec077795b7b298d24a8f5571c0a580ea62a` |
| `internal/executiongraph/acceptance.go` | `9af29ae129f93c72fef2b060ba02e198b7338e2da690d7afb2d3064aed963871` |
| `internal/coordination/coordination.go` | `fe4c401dfc36e2f375b5342592c554f3adb04bf398b730ba76bc99e9f778daf0` |
| `internal/gitworkspace/git.go` | `f72b3febf2a0a2ba4bca72edadcf00a0e608f836ecf9d1c18fb5a6be0e20f8ac` |
| `internal/gitworkspace/validator.go` | `61ac2faa66cd495c8d0039b09b2f4da3822d61bba55c85382cc51505f6879418` |
| `internal/graphapplication/local.go` | `12a72ac6aa163aeddc0ff430c5ce22f986499229cc74708421b9e716d1b8d288` |

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

### T33 — Concrete scheduler and isolated Git workspaces

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

- `gitworkspace.Manager` resolves one exact local repository/common directory,
  requires the authorized base to equal repository `HEAD`, and creates detached
  worktrees with explicit Git argv. Git receives disabled hooks, filesystem
  monitor, credential helper and terminal prompting; no fetch, pull, push, merge,
  rebase or shell is invoked.
- Every workspace is bound to parent, child, graph revision, envelope digest,
  repository, common directory, base commit and base tree in a private sidecar.
  Validation also confirms Git's worktree registry, detached `HEAD`, tree,
  cleanliness and exact canonical path before Runtime dispatch.
- Absolute confinement rejects traversal, outside-root paths, symlink components,
  pre-existing targets, ownership tampering, wrong repository, wrong `HEAD` and
  foreign changes. No destructive cleanup API exists; worktrees remain preserved.
- Implicit Git command execution fails closed. At `NewManager`, before worktree
  creation, before every workspace inspection and before every `worktree`, `add`,
  `apply` or `diff` invocation, the manager reads the effective configuration Git
  would use in that directory (`git config --list --includes -z`: repository,
  worktree-specific and included files; system/global remain excluded) and
  rejects any non-empty `filter.<driver>.clean`, `filter.<driver>.smudge`,
  `filter.<driver>.process`, `diff.external`, `diff.<driver>.command` or
  `diff.<driver>.textconv`. Configuration is never rewritten; diffs additionally
  pass `--no-ext-diff --no-textconv`. Real-Git tests configure a live helper that
  writes a sentinel (a control repository proves it runs under plain Git) and
  show rejection before checkout/staging with no sentinel and an unchanged shared
  checkout, including configuration added by a child after preparation and
  worktree-specific drivers.
- ADR-0005 ownership/permissions: the WorkspaceRoot and
  `.axiom-workspace-owners` must be non-symlink directories owned by the
  effective user with no group/other bits and no extended ACL; owner records must
  be single-link owner-only regular files with the same ownership/ACL checks.
  This reuses the existing `internal/local`/`internal/codexruntime` private-path
  pattern. A permissive pre-existing root or owner directory fails `NewManager`;
  permissive, hard-linked, symlinked or foreign-owned control paths fail every
  later inspection. Foreign ownership is exercised through a test-only effective
  UID override because `chown` to another UID is unavailable unprivileged.
- `graphapplication.LocalService` composes the existing scheduler with the concrete
  manager, `OSProcessRunner`, Runtime invocation resolver and graph store. Runtime
  `cwd` remains the exact validated child worktree. Captured process output remains
  bounded and gains a digest-addressed inspectable reference.
- Real local dispatch testing used two independent worktrees and two concurrent
  child test processes. Dispatch intervals strictly overlapped. Dependency/
  conflict, timeout/cancellation, unknown-effect and new-attempt retry behavior
  remains covered by the scheduler suite.

Concrete worktree observation:

| Fact | Value |
|---|---|
| Test repository | sanitized temporary local Git repository |
| Base revision / tree | `0caedc1c72292e50d3563f5a45f06bf997f273be` / `2c2f1015a5c9f7d2859b824451063ead47484875` |
| Safe workspace paths | `<temp>/workspaces/{a,b,integrate}` |
| Child identities | `00000000-0000-4000-8000-000000000002`, `00000000-0000-4000-8000-000000000003`, `00000000-0000-4000-8000-000000000004` |
| Shared checkout | `HEAD`, tree and porcelain status identical before/after |
| Negative matrix | wrong repository/ownership/HEAD, dirty workspace, collision, outside root and symlink escape rejected; live `clean`/`smudge`/`process` filters, worktree-specific process driver and `diff.external` rejected with no helper execution; permissive/foreign/symlinked root, owner directory and owner record rejected |

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

### T35 — Concrete Git Integration/Reconciliation

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

- Child facts are derived from Git: ownership, base/`HEAD`/tree, changed paths and
  a result tree written through a temporary isolated index. Changed paths outside
  the child's exact `repository-write` effects fail closed.
- Conflict preview applies Git-generated full-index binary patches to a temporary
  index only. The authority-bearing domain preview remains unchanged and binds
  target revision/tree, ordered child results, optional waivers and exact effects.
- Apply re-inspects every child and the clean integration target, compares facts
  with the authorized preview, then applies only those patches with `git apply
  --index` inside the integration worktree. No shared-checkout, remote, Provider or
  automatic conflict-resolution effect exists.
- Concrete combined validation executes declared absolute argv with exact
  integration `cwd`, bounded environment/output, explicit command reference, exit
  code and SHA-256 output digest. It re-inspects the result tree after each command;
  validator mutation/failure cannot produce parent success.
- `graphapplication.LocalService` composes integration preview, authority-bound
  apply, combined validation, parent roll-up and the concrete coordination
  verifier/Evidence builder.
- The Integration/Reconciliation child executes as a real attempt with its own
  opaque identity under the same child Execution. A `running` attempt is persisted
  through `GraphAttemptStore` before apply; the terminal attempt (`succeeded`,
  `failed` after combined-validation failure, or `unknown` with ambiguous effect
  after unconfirmed apply) is persisted before the roll-up reports it. The roll-up
  is derived from the last persisted graph, and `LocalService` adopts that graph.
  Pre-effect persistence failure applies nothing; failure to persist after
  confirmed effects returns `recovery_required` with the canonical attempt still
  `running`, never success. The preview binds the integration attempt count, so
  one authority cannot start a second attempt, and a `running`, `unknown`,
  ambiguous or `succeeded` integration attempt blocks new previews.

Concrete vertical observation:

| Fact | Value |
|---|---|
| Base revision / target tree | `734bfb9cfb1ae64a8300defc92da5925f7197b2e` / `cc1bdfa1894337da8480f5a64a42dbde30c80912` |
| Child result trees | `bbb2a5975fb35c32c9141ae1008edca9f5cd0be5`, `779ccc5fa6114fa6e6d8a1fbd66c8b78d73b6999` |
| Preview / authority correlation | `4ba94284415e8bfe26aa31620e4a0710f71a91270c75d8fe131766d01dcd4460` |
| Result tree | `9bfa5fb747b6bcf332c952af414b51cae58bc382` |
| Applied ledger | exact ordered child `repository-write` effects; references `git-workspace:<child-id>` and `git-tree:<tree>` |
| Combined validator | `git diff --check`; exit `0`; output digest `e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855` |
| Parent roll-up | `success` only after confirmed apply, validator pass and a persisted `succeeded` Integration attempt bound to the result tree |
| Negative matrix | missing child, forged lineage, stale base, foreign target/child change, real conflict, effect mismatch, authority/preview tamper, missing/matched waiver, validator failure (`failed` attempt), apply failure (`unknown` attempt), persistence failure before/after effects and authority replay |
| Shared checkout | `HEAD`, tree and porcelain status identical before/after |

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
- Every roll-up child outcome, including Integration/Reconciliation, must equal
  that child's last persisted attempt; there is no integration-owner exception.
  A `success` roll-up additionally requires every required outcome `succeeded`
  and a persisted `succeeded` integration attempt whose result reference binds
  the roll-up result tree; otherwise `BuildEvidence` rejects the roll-up.
- Missing journey facts remain `deterministic_preparation_only`; foreign
  Runtime/Profile/attempt claims and coordination records with foreign parent,
  graph revision, child or attempt (including an attempt of another child),
  tampered or non-canonical content, arbitrary digests, caller-asserted facts
  or duplicate identity are rejected. Journey tests publish records through
  `coordination.Service` and verify them through `coordination.AcceptanceVerifier`.

T30–T35 now have deterministic and concrete local Evidence. No real Codex +
Claude Runtime journey was executed, and no `real_run_recorded` Evidence exists.

## Final validation commands

| Command | Result |
|---|---|
| `go test ./... -count=1` | exit 0; all packages passed |
| `go test -race ./internal/runtimeprofile ./internal/runtimeadapter ./internal/executiongraph ./internal/coordination ./internal/local ./internal/gitworkspace ./internal/graphapplication -count=1` | exit 0; all seven packages passed |
| `go vet ./...` | exit 0 |
| `go build ./...` | exit 0 |
| `go mod verify` | exit 0; all modules verified |
| `./scripts/validate-repository.sh .` | exit 0; repository/package/bootstrap validators passed |
| `./scripts/check-sensitive-files.sh .` | exit 0; worktree passed |
| `gitleaks dir . --redact --no-banner` | exit 0; no leaks found |
| `git diff --check` | exit 0 |

Separate concrete test commands also passed for `./internal/gitworkspace` and
`./internal/graphapplication`. The same command set was rerun after the PR #103
review remediation with the results above. These validations do not supply real Runtime
Evidence, T36 real-run authority or human acceptance.

## Proposed T36 real-run envelope — draft, not authorized

This draft selects a small real Axiom change: add a read-only
`lingo runtime profile validate` command and its command documentation. It is not
an executable authority record yet because concrete local Model Profiles and the
final reviewed run envelope are operator inputs.

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

T33/T35 no longer block this envelope. The operator must publish exact local
`t36-codex-implementation`,
`t36-claude-documentation` and integration profiles; and the generated envelope
must replace every descriptive path/argv with exact digested values. No real
Runtime run may begin from this draft.
