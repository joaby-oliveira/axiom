# Evidence — MVP Slice S6: Durable Work Item Lifecycle and Metadata Governance

## Claim and authority boundary

This record covers Specification 004 Slice S6 only: T26–T29. The deterministic
implementation was exercised at clean commit
`56beb4fc310894ff8de128f52c6a96d22711bec8` on branch
`feat/94-s6-work-item-lifecycle`, directly based on the approved amendment merge
`847f21a611e3cecd8bbb4fec0342de9ba23ecd53`. Its Git tree is
`d438fc05e612c641d67c2e4db5eb5d67f7a196c0`.

The first PR CI run found one validation-fixture regression, not a product-state
failure: `scripts/dogfood-poc.sh` still advanced the S4 journey without the S6
facts and began Provider reconciliation with zero lifecycle markers, which S6
correctly classifies as drift. Commit
`7c05fbe56ed3b8ed7432ac0818bab44ae31e5b49` records the four synthetic lifecycle
facts and starts the bounded fake Provider observation already aligned. The
dogfood journey and full repository validation then passed from that clean
revision. Deterministic adapter tests remain the Evidence for label creation and
legacy-label replacement effects.

Canonical local Execution remains the sole workflow truth.
`WorkItemLifecycleStage` is computed read-only from canonical gate history and
revisioned local facts. GitHub labels/comments, Repository artifacts, merge, CI,
review, Issue state, and Provider observations grant no local transition, fact,
or human-acceptance authority.

No real GitHub mutation was authorized or executed for this Evidence run. T27
and T28 Provider behavior used deterministic fakes and pure preview/effect
contracts. No ADR-0007 recovery mutation was executed; T29 inspection and
authority matching are read-only. No S7 work, prerelease/release, Issue closure,
merge, or human MVP acceptance occurred.

## Environment and identity

| Fact | Observation |
|---|---|
| Repository | `rgomids/axiom` |
| Branch | `feat/94-s6-work-item-lifecycle` |
| Exercised commit | `56beb4fc310894ff8de128f52c6a96d22711bec8` |
| Final clean validation commit | `7c05fbe56ed3b8ed7432ac0818bab44ae31e5b49` |
| Source state | clean |
| Go | `go1.26.1 darwin/arm64` |
| macOS | 27.0, build `26A428` |
| Architecture | `arm64` |
| Gitleaks | 8.30.1 |

Relevant content digests at the exercised commit:

| Artifact | SHA-256 |
|---|---|
| `internal/workflow/lifecycle.go` | `5e211d98f29cbd644b3546c308bc32d8b81a00ae9bb4008641382ba1694a33b9` |
| `internal/workitem/metadata.go` | `3d6b14bbe53fb32bf067275bed5c25f8afe5fa8f549dfec2a0d3aecc0fedcc5f` |
| `internal/workflow/reconciliation.go` | `e830b38f82ace6d117e1e216a26fea5eaa44c585af223c1e65d7edb0b3335a01` |
| installed `axiom-work-item-run` skill | `5e1661d06a1caa7f7af6fd8c6253d3742f0cbb26df262a8c8357507e76f85f00` |

## T26 — derived lifecycle and local facts

The gate/fact matrix proves the exact ten values `intake`, `specifying`,
`specified`, `planning`, `planned`, `implementing`, `implemented`, `reviewing`,
`reviewed`, and `accepted`. Expected fact absence at `plan`, `implementation`,
and `review` yields `specified`, `planned`, and `implemented`. Later missing,
duplicate, skipped, reference-missing, scope-mismatched, or out-of-order facts
fail closed as `recovery_required` without persistence.

Planning authority, implementation authority, review start, human acceptance,
and auxiliary conditions are revisioned events in the existing Execution
lineage. They carry the exact Execution scope digest, validated bounded reference,
timestamp, and existing transition provenance. No lifecycle field or independent
history exists in the persisted state DTO. Recording a fact requires the exact
current revision plus explicit local authority.

Blocked snapshots were exercised at every canonical gate. A blocker remains an
orthogonal condition; a passed next-gate transition is denied with unchanged gate
and revision. Acceptance was exercised only after terminal canonical completion
plus the explicit human-acceptance fact. A synthetic Provider observation with a
closed Issue and `axiom:stage:accepted` changed no derived local result.

The executable black-box journey records all four boundary facts through
`workflow fact`, covers interruption/resume, reaches terminal completion, then
records explicit human acceptance. Strict CLI validation and the thin installed
workflow skill preserve explicit selectors and authority.

The clean bounded dogfood journey uses only synthetic data and a local fake
Provider. It completed at revision 17 with `lifecycleStage=accepted`, five
installed skills, one Work Item, and one Execution. This fixture simulates the
explicit acceptance authority for contract testing; it is not human acceptance
of S6 or the MVP.

## T27 — GitHub projection and bounded history

The exact lifecycle-label enum and all 16 combinations of the four independent
flags are deterministic. Preview reads committed local truth, validates one
observed stage, preserves foreign labels/comments, and binds target, Execution
revision, derived lifecycle, observation digest, preview digest, and ordered
effects through the existing projection contract.

Zero, multiple, unknown, or contradictory Axiom lifecycle markers return
`recovery_required`; unknown namespaced content is not deleted. Positively
identified S4 labels may appear only as reviewed removal effects. Historical S4
projection records remain readable with their original labels/comments and are
not rewritten. Comments are provenance-marked, reference-first, idempotent per
Execution revision, limited to 16 KiB and at most 16 transition references.

Existing S4 tests plus S6 tests exercise denial, stale revision/digest, replay,
ambiguous Provider results, confirmed-effect/local-bookkeeping partial truth,
foreign-content preservation, and bounded observations. The real-Provider effect
ledger for this run is exactly empty: `effects=[]`.

## T28 — metadata policy

The recognized portable document is closed to `kind: work-item-metadata` and
integer `policyVersion: 1`. It reuses Specification 002's existing `policies`
references; `axiom.yaml` remains closed at `schemaVersion: 1`. Tests cover absent,
empty, one valid, duplicate recognized kind, unknown field, invalid type/version,
and Work Item versus Pull Request intention separation.

Resolution order is explicit input, policy default/reference, validated context,
then one prompt per unresolved mandatory value. Fully resolved fixtures have zero
prompts; missing-only fixtures have exactly one named prompt. Unsupported optional
capabilities remain explicit; unsupported mandatory capabilities fail before an
effect preview. Concrete GitHub fields (`labels`, `assignees`, `milestone`,
`project_status`, and `requested_reviewers`) exist only in the GitHub adapter.
Metadata previews include target, observation digest, resolved sources,
unsupported optional intentions, ordered effects, and a digest; exact authority
rejects a changed observation.

## T29 — missing-local-state reconciliation

The inspector is a pure read-only function over current state, bounded local
generations, Provider observations, and Repository references. It returns only:

1. validated current local truth, plus the shared T27 projection preview when a
   recognized Provider drift has deterministic reconciliation effects;
2. one exact validated local-generation recovery plan requiring fresh authority;
3. `recovery_required` with reason and human decision.

Aligned Provider observations return current truth with zero effects. Missing or
obsolete flags, one recognized S4 lifecycle label, and a missing projection
comment return current truth plus only the T27 effects needed for convergence.
Zero/unknown/multiple lifecycle labels, Provider-only, Repository-only,
changed-artifact, contradictory, missing-generation, and multiple-generation
fixtures produce no Execution and no transition. A structurally valid generation
whose lifecycle cannot be derived is not a recovery candidate. One semantically
valid generation binds its name and digest; stale authority does not match.
Before/after state and Provider-observation digests in read-only tests are
identical.

## Commands and exit status

All commands below ran from the repository root against the exercised commit and
exited `0`:

```text
go test ./...
go vet ./...
go build ./...
go mod verify
go test -count=1 ./internal/workflow -run 'Lifecycle|Projection|Reconciliation'
go test -count=1 ./internal/workitem -run Metadata
go test -count=1 ./internal/githubissues -run 'Metadata|Projection'
go test -race -count=1 ./internal/workflow ./internal/workitem ./internal/githubissues
./scripts/dogfood-poc.sh
./scripts/validate-repository.sh .
./scripts/validate-agent-package.sh .
./scripts/check-sensitive-files.sh .
./scripts/check-sensitive-files.sh --staged .
gitleaks detect --source . --no-git --redact --no-banner
gitleaks git --staged --redact --no-banner
git diff --check
git diff --cached --check
```

Repository validation included the agent-package validator behavior and
sensitive-file checker behavior. `go mod verify` reported `all modules verified`.
Gitleaks reported no leaks. The final documentation commit is validated again
before publication; it does not change the exercised Go implementation.

After the dogfood remediation, GitHub Actions run `36082452489` completed the
repository POC verification successfully on both `ubuntu-24.04` (59 seconds) and
`macos-15` (1 minute 17 seconds). This CI result supplements rather than replaces
the deterministic local Evidence above.

### PR #96 validation remediation — 2026-09-25

The T29 correction was validated in a modified worktree based on PR head
`06b4a2316d77270a81f8deb58e2fc09a093daa2b`. The patch adds semantic lifecycle
validation for local-generation recovery and makes T27 projection-preview
construction reusable by read-only T29 inspection. No commit identity, clean
source state, Provider effect, recovery mutation, merge, or human acceptance is
claimed for this worktree validation.

The following requested commands ran from the Repository root and exited `0`:

```text
go test ./...
go vet ./...
go build ./...
go mod verify
go test -count=1 ./internal/workflow -run 'Lifecycle|Projection|Reconciliation'
go test -race -count=1 ./internal/workflow ./internal/workitem ./internal/githubissues
./scripts/validate-repository.sh .
./scripts/validate-agent-package.sh .
./scripts/check-sensitive-files.sh .
git diff --check
```

## Requirement and acceptance coverage

| Contract | Evidence owner |
|---|---|
| FR-038–FR-041 / AC-25, AC-26, AC-29 | T26 gate/fact/reference/authority matrices and executable journey |
| FR-042 / AC-27, AC-28 | T27 label/flag/drift/replay/partial/bounded-comment tests |
| FR-043 / AC-30 | T29 current/generation/contradiction/read-only matrices |
| FR-044 / AC-31 | T28 closed-policy, precedence, prompt, capability, and adapter-preview matrices |

ADR-0003 through ADR-0008 remain applicable without amendment. The implementation
does not change Execution identity, canonical gate semantics, WorkflowVersion,
commit authority, Provider non-authority, or ADR-0007 recovery semantics.

## Limitations and unexecuted cases

- No separately authorized real GitHub lifecycle or metadata mutation was run;
  deterministic fake/adapter Evidence does not claim a real Provider effect.
- No local recovery mutation was run; only read-only classification and exact
  recovery-authority matching were exercised.
- Local native execution was macOS arm64. The final POC verification also passed
  on GitHub-hosted Ubuntu 24.04 and macOS 15 runners.
- Technical completion, passing checks, PR review, merge, and Provider state do
  not constitute human acceptance.
