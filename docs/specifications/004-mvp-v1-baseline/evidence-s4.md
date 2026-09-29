# Evidence — MVP Slice S4: Local Execution and GitHub Projection

## Claim and authority boundary

This record covers the explicitly authorized Specification 004 Slice S4 only:
T10–T13. Work started from `main` revision `c37a297`, the PR #85 merge. T14/S5+
implementation, Issue closure, prerelease/release publication, merge, and final
MVP acceptance were not authorized and were not performed.

The deterministic implementation, fake-Provider observations, and mandatory
bounded real GitHub label/comment observation below are complete. The real
observation used exact per-run human authority recorded on 2026-09-23. This
closes the T12–T13 technical Evidence gate only; technical checks and Provider
effects do not imply human acceptance.

## Delivered behavior

### T10–T11 — local Execution truth

- One opaque stable Execution ID binds exact Project, Repository, linked Work
  Item, Runtime, workflow/format versions, provenance, stage, revision, bounded
  transition history, references, projection ledger, and terminal facts.
- Protected machine-local state lives under `executions/v1`; closed decoding,
  strict identity checks, private modes, expected storage revision, and the shared
  publication protocol reject malformed, newer, stale, mixed, or uncertain state.
- Duplicate equivalent start returns the same lineage. Conflicting scope/version
  fails. Every transition requires its exact Execution revision and current stage.
- The fixed path is intake → specification → clarification → plan → tasks →
  implementation → review → evidence → reconciliation → completion. Failed
  transitions stay at the current stage and require exact-revision resume.
- Artifact references bind artifact ID, Execution ID, and SHA-256. Evidence
  references bind a confined repository-relative regular file and SHA-256, with
  no symlink traversal and a 1 MiB read bound.
- Completion is local truth only. It does not close or otherwise infer state from
  the GitHub Issue.

### T12–T13 — post-commit projection and convergence

- `workflow reconcile` first reads and binds exact Provider, repository/resource,
  Issue external ID, URL, state, bounded repository labels, Issue labels, and
  projection-comment presence, then returns a digest-bound preview. Preparing
  the preview performs no mutation. An external OPEN/CLOSED state change alters
  the observation and preview digests, invalidating stale mutation authority.
- Exact authority binds Work Item, committed Execution revision, stable projection
  key, current stage label, provenance-marked bounded comment, observation, and
  ordered effect set.
- Projection creates/adds the current `axiom:stage:<stage>` label, removes only
  obsolete Axiom stage labels, preserves foreign labels/content, and posts at
  most one comment for the Execution revision. It never advances local truth.
- Intended effects are persisted before Provider calls. Every successful or
  ambiguous mutation is reinspected before confirmation is recorded. Replays
  first reconcile previously Intended but unconfirmed effects from the current
  Provider observation, persist Confirmed/Complete, and only then determine
  whether new Provider effects remain. A confirmed effect followed by local
  bookkeeping failure is canonical `partial`; retry converges the ledger without
  repeating the mutation or duplicating the projection comment.
- Provider execution remains bounded by the existing absolute `gh` process,
  15-second deadline, 256 KiB captured-output limit, JSON stdin, strict response
  identity/state/content checks, and closed error taxonomy.
- Deterministic process barriers prove one winning local revision without using
  sleeps as the race oracle. F0–F8 injection preserves old-or-new truth or
  `recovery_required` according to the shared publication protocol.

## Deterministic observations

| Condition | Evidence seam | Observed truth |
|---|---|---|
| duplicate equivalent/conflicting start | application store spy | one stable lineage; conflict cannot replace it |
| skipped, stale, duplicate, failed, resumed transition | workflow contract matrix | fixed order; exact revision; replay convergence; failure does not advance |
| malformed/newer/incoherent history | closed codec/state validator | rejected before use |
| artifact/Evidence reference mismatch or traversal | real local stores/files | transition denied; no state change |
| two writers from one revision | two executable processes and barrier | one save; one conflict; one authoritative revision |
| crash at staged/committed publication boundaries | killed helper process | reader fails closed with `recovery_required` |
| F0–F8 local-store faults | deterministic publication hooks | prior truth or preserved uncertain marker; no mixed reader |
| missing/stale projection authority | preview digest tests | zero Provider mutation |
| Issue OPEN after preview becomes CLOSED | exact fake observation identity/state | digest changes; old authority denied; zero Provider effects |
| foreign and obsolete stage labels | fake Provider ledger | foreign label preserved; only Axiom-owned obsolete label removed |
| ambiguous Provider response | apply-then-error fake plus reinspection | effect confirmed once; no duplicate mutation |
| success response without observable effect | apply fake plus bounded read | retryable reconcile; effect not falsely confirmed |
| Provider unavailable | inspect failure | local transition remains authoritative and unchanged |
| confirmed comment plus local save failure | Provider/store fault seams | canonical `partial`; retry records Confirmed, sets Complete, and does not repost comment |
| installed-binary dogfood | isolated Project/state/fake `gh` | complete local workflow plus one authorized fake projection converges |

## Commands executed

Deterministic validation ran on macOS 27.0/arm64 with Go 1.26.1 from branch
`agent/s4-workflow-provider-projection`. Initial S4 delivery was based on
`c37a297cb3ee698ca7da0cce51ea98acf2f776f6`; review remediation was validated
from PR head `c0eb6e14ecdb213376651dc0da3026e13f85553b`. Every command below exited `0`:

```bash
go test ./internal/workflow ./internal/local ./internal/githubissues ./cmd/lingo
bash scripts/dogfood-poc.sh
go test ./...
go test -race ./...
go vet ./...
go build ./...
go mod verify
go test -shuffle=on -count=10 ./internal/workflow ./internal/local ./internal/githubissues
./scripts/validate-repository.sh .
./scripts/check-sensitive-files.sh .
./scripts/check-sensitive-files.sh --staged .
git diff --check
```

`gitleaks detect --source . --no-git --redact --no-banner` also exited `0` and
reported no leaks. `go mod verify` reported `all modules verified`.

After the real observation and Evidence reconciliation, the same full suite was
repeated from branch HEAD `3f6321e8369fc9295503d9e8a742fca5c065afd8`.
Repository validation, worktree sensitive-file scan, `git diff --check`, all Go
tests including race and ten shuffled workflow/local/GitHub adapter runs, vet,
build, module verification, installed-binary dogfood, and Gitleaks all exited
`0`. The dogfood remained isolated and used its fake Provider; it did not mutate
Issue #92.

The real-provider observation and that local/full validation were executed from
`3f6321e8369fc9295503d9e8a742fca5c065afd8`. The resulting Evidence and Tasks
reconciliation were then committed as
`b95217a957e563d3882e4ee7acb5f689a9ac1316`. POC verification for that
reconciliation commit completed successfully on both configured GitHub Actions
jobs. The later commit records the results; it is not presented as the commit
from which the observation was executed.

The final isolated installed-binary dogfood observation recorded these fields;
the projection key is written separately below to keep its digest classification
explicit during secret scanning:

```json
{"evidenceVersion":1,"evidence":"axiom_e2e_dogfood","cwdIndependent":true,"globalSkillCount":5,"workItem":7,"executionId":"307c7248-4d99-447d-902c-a98de408a4e6","revision":13,"workflow":"completed","projectionKey":"recorded below","projectionDigest":"8d2b3da0fa3845894c165e79a271e047e3c7b0a201fed46413841b242503f41f","binarySha256":"259e647e00a102aef8602fe3bd2e68dcaa48da3924031769778b2c1a8b6383dd","workflowSha256":"fa73975a6480ab691f0a83be2cc44ba4d11aad1979545032951cb46696e9cf08","result":"pass"}
```

That run used this exact local transition matrix:

| Revision | From | Outcome | To / status |
|---:|---|---|---|
| 1 | start | — | intake / active |
| 2 | intake | pass | specification / active |
| 3 | specification | pass | clarification / active |
| 4 | clarification | pass | plan / active |
| 5 | plan | pass | tasks / active |
| 6 | tasks | pass | implementation / active |
| 7 | implementation | fail | implementation / interrupted |
| 8 | implementation | resumed | implementation / active |
| 9 | implementation | pass | review / active |
| 10 | review | pass | evidence / active |
| 11 | evidence | pass | reconciliation / active |
| 12 | reconciliation | pass | completion / active |
| 13 | completion | pass | completion / completed |

At revision 6 the fake Provider before-state contained no stage label and no
projection comment. Exact fake-Provider authority converged to one
`axiom:stage:implementation` label and one comment carrying projection marker
`d0d66d7194019d8a82c9512cbb9675b9e54f0b57a3aea9713f6a156ad598eca4`.
The persisted local projection record contained the ordered intended effects and
the same confirmed set. Reinspection after every fake effect and a final replay
preview observed zero remaining effects. These are deterministic fake-Provider
facts; the following observation records the separate real GitHub proof.

## Real GitHub Provider observation

### Target and local truth

The controlled run created one explicitly disposable test Issue through the
authorized GitHub account, then used only public Lingo commands to configure an
isolated Project, select the Issue, start one Execution, and advance the normal
sequence to `implementation`:

| Fact | Observed value |
|---|---|
| Repository | `rgomids/axiom` |
| Issue | [#92](https://github.com/rgomids/axiom/issues/92) |
| Test title | `[TEST][AXIOM S4] Real Provider Observation` |
| Project | `76fb1fd4-d3cc-4fc8-9835-f5171be0f096` / `axiom-s4-provider-observation` |
| Repository key | `main` |
| Execution ID | `f557b2a4-6120-46a9-8d12-64b67ee1d62b` |
| Workflow / Runtime | `mvp-v1-sequential` / `codex` |
| Stage / revision / status | `implementation` / `6` / `active` |
| Projection key | `14a97e484e1706a06f0105f7edc71b0887f921e8e9fb5cae247a4c77539d9e4f` |
| Branch HEAD used by Lingo | `3f6321e8369fc9295503d9e8a742fca5c065afd8` (`clean`) |

Transitions were committed in order: revision 1 `intake`; revision 2
`specification`; revision 3 `clarification`; revision 4 `plan`; revision 5
`tasks`; revision 6 `implementation`. No persisted state was edited manually.

### BEFORE snapshot

- Issue #92 was `OPEN`, had no labels, zero comments, and no workflow projection
  marker.
- The repository had 25 labels and no `axiom:stage:*` label. Its observed names
  were `bug`, `cross-cutting`, `documentation`, `duplicate`, `enhancement`,
  `good first issue`, `help wanted`, `invalid`, `question`, `scope:mvp`,
  `slice:s1` through `slice:s7`, `status:active`, `status:done`,
  `status:planned`, `status:superseded`, `type:epic`, `type:historical`,
  `type:slice`, and `wontfix`.
- The Issue title/body explicitly identified disposable S4 Evidence; no generic
  test label or new taxonomy was introduced.

### Read-only preview and exact authority

The first and immediately repeated read-only `workflow reconcile` observations
were identical:

```text
observation digest: 16bef7c44639514bba191122cc9ca224c9130489e554ac0ad8e71dcda0f06745
preview digest:     7f0f7b92a97c751b47cf69607dfaa3486c4d2fe340fe8139f087006d01997bd3
desired label:      axiom:stage:implementation
Issue state:        OPEN
Issue labels:       []
CommentPresent:     false
```

The reviewed ordered effect set, authorized only for that target, Execution
revision, projection key, observation, digest, and order, was:

```json
[
  {"kind":"create_stage_label","value":"axiom:stage:implementation"},
  {"kind":"add_stage_label","value":"axiom:stage:implementation"},
  {"kind":"post_transition_comment","value":"<!-- axiom:workflow-projection:14a97e484e1706a06f0105f7edc71b0887f921e8e9fb5cae247a4c77539d9e4f -->\nAxiom workflow transition\n\n- Stage: `implementation`\n- Outcome: `pass`\n- Next: Execute implementation\n\n_Axiom development · 3f6321e8369f · clean · f557b2a4-6120-46a9-8d12-64b67ee1d62b_"}
]
```

Programmatic review proved three effects only, no `remove_stage_label`, one
projection marker in the comment, Issue `OPEN`, and unchanged branch HEAD before
`--authorize-external` consumed the exact preview digest.

### AFTER snapshot and confirmed ledger

- Issue #92 remained `OPEN`.
- Its only label became `axiom:stage:implementation`; no non-Axiom label existed
  to remove and none was removed.
- The repository label was created with color `5319e7` and description
  `Axiom current workflow stage`.
- Exactly one comment existed, at
  `https://github.com/rgomids/axiom/issues/92#issuecomment-5801613781`, containing
  exactly the projection marker and text above. Its body was 287 bytes with
  SHA-256 `02a9e528d525cf2a6d37c221555a72dc5c7ed058191c8239ee40525f215540fc`.
- The local Execution remained `implementation`, revision `6`, status `active`;
  Provider state did not advance, repair, or reconstruct it.
- The protected `0600` Execution record had SHA-256
  `19f3087c650d0dbf8cd84e4b0ada0b46cc71cdbfeb5575acb13fb44e85fb6fe5`.
  Its projection record had `ExecutionRevision=6`, the projection key above,
  `Intended` equal to the three ordered effects, `Confirmed` equal to the same
  three effects, and `Complete=true`.

The exact Provider delta was therefore one repository label, one Issue-label
association, and one projection comment. No Issue state or foreign content
changed.

### Replay convergence

A read-only replay of `workflow reconcile` for revision 6 observed Issue `OPEN`,
the current Axiom label, and `CommentPresent=true`, returning:

```text
effects = []
observation digest = 6829dbc0a85abca9359e1535f531cef716f647e485bf85954e944c1120208389
preview digest = b1be21e635ca863e8864d754a44fd825ec9192ce6f61c0432855a8450a51357a
```

Immediate reinspection still found one Issue label, one repository
`axiom:stage:*` label, one comment, and exactly one matching projection marker.
No authorized replay mutation was necessary because the public preview already
proved natural convergence.

### Safety assertions

- The Issue was not closed.
- No non-Axiom label was removed; no external comment or content was modified.
- Projection used exact revision/digest authority and no stale authority retry.
- Provider state did not advance or repair local Execution truth.
- Replay created no label or comment and duplicated no mutation.
- No S5 behavior, merge, release, or human-acceptance claim was introduced.
