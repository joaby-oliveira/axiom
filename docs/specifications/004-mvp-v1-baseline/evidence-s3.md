# Evidence — MVP Slice S3: Intent to GitHub Work Item

## Claim and authority boundary

This record covers the explicitly authorized Specification 004 Slice S3 only:
T08–T09. Work started from `main` revision
`9c16322` (the S2 PR #84 merge). S4–S7, workflow execution/projection, issue
comments/labels/closure, repository or Git mutation, release publication, and
final MVP acceptance were not authorized and were not performed.

The results below establish technical implementation Evidence. They do not imply
human acceptance. Separate exact per-run authority was supplied for one bounded,
reviewed, public-safe real GitHub observation. That observation created Issue
[#90](https://github.com/rgomids/axiom/issues/90) and published its exact local
Work Item link. It did not authorize T10, S4, cleanup, or any other effect.

## Delivered behavior

### T08 — read-only structured Intent draft

- Intent or an explicit problem is normalized with desired outcome, context,
  scope, constraints, non-goals, and acceptance expectations into a
  provider-neutral draft.
- Every section retains `user` or `axiom` authorship. Missing questions are
  deterministic and limited to materially absent sections.
- Preview binds Project/repository/provider target, normalized draft, rendered
  provider document, exact effect set (including the durable create-attempt
  fence), expected local revision, correlation,
  provenance, and digest. Preparing it performs no Provider or local write.
- UTF-8/control, per-field and aggregate size, and structural secret-sentinel
  checks occur before rendering or effects. Provider rendering indents section
  content, preserving Markdown and HTML-like input as authored data rather than
  Issue structure.
- Guided CLI prompts only missing values, prints the complete preview, and treats
  EOF or any answer other than exact `yes` as cancellation without authority.

### T09 — exact create/select authority and truthful linkage

- Create requires the exact reviewed preview digest plus
  `--authorize-external`. Selection first reads and validates the exact Issue,
  then requires its separately reviewed digest plus `--authorize-local` for the
  protected local publication.
- The application port uses provider-neutral resource/external identity. GitHub
  Issue number, URL, state, request paths, and rendering remain in the
  `internal/githubissues` adapter.
- The adapter invokes an absolute `gh` executable with a 15-second deadline and
  256 KiB captured-output limit. Untrusted title/body are JSON on stdin, never
  shell-interpolated or placed in arguments. Responses must match the exact
  `https://github.com/<owner>/<repository>/issues/<number>` identity and
  `OPEN|CLOSED` state.
- Before the first POST, application code searches for the exact deterministic
  correlation marker and durably publishes a target-scoped create-attempt fence.
  A `pending` fence survives process restart and permits reconciliation only:
  zero matches stays non-mutating, one valid match is reused, and
  multiple/invalid matches fail closed. Only a Provider response that explicitly
  proves no effect transitions the fence to `retry_allowed`; no time heuristic
  or blind create retry exists.
- A confirmed Provider effect followed by local read/write/conflict/recovery
  failure is canonical `partial` and retains the exact external reference.
  Committed-but-uncertain protected publication also remains truthful partial.
- The provider-neutral domain link maps at the local adapter boundary to the
  existing Work Item `formatVersion: 1` fields `providerRepository` and `number`.
  New filenames hash `provider + resource + externalID` under the
  Project/repository scope. Existing `repositoryKey-externalID` v1 records remain
  readable and updatable only when their decoded identity exactly matches; no v2
  wire schema or bulk migration exists. Two resources with the same Issue number
  cannot overwrite each other, and an unqualified ambiguous lookup fails closed.
- `gh api --include` supplies bounded response status and headers. `401` is
  unauthenticated/non-retryable; `429` and structured rate-limited `403` are
  retryable; `5xx` is unavailable/retryable; ordinary `403` and deterministic
  `400`/`404`/`410`/`422` failures are non-retryable. Missing reliable HTTP
  metadata is ambiguous for create because it cannot prove absence of an effect.
  Create timeout, unknown transport failure, invalid success response, and `5xx`
  remain ambiguous and require reconciliation; no blind create retry was added.

## Deterministic observations

| Condition | Test seam | Observed truth |
|---|---|---|
| incomplete or cancelled interview | application/CLI spies | ordered missing questions or `interrupted`; zero Provider effects |
| stale/absent create authority | exact preview digest test | `denied_authority`; zero create and local saves |
| existing or newly written v1 Work Item | protected store codec tests | old wire fields retained and mapped to provider-neutral domain identity |
| v1-unrepresentable provider/external ID | pre-publication adapter validation | unsafe input rejected before state-root creation |
| stored identity differs from requested path | protected store decode test | unsafe record rejected before use |
| metacharacter/Markdown input | fake executable argument/stdin capture | content only in JSON stdin and indented authored sections |
| 401/403/404/422/429/500/503 | included-status adapter matrix | only explicit rate-limit and 5xx cases are retryable |
| timeout, oversized, unstructured or mismatched response | bounded adapter fake | uncertain create outcomes are ambiguous/reconciliation-only; read-only invalid content fails closed |
| ambiguous create | sequenced reconciliation fake | second reconciliation before result; no blind retry |
| ambiguous create across two executable processes | installed-binary fake `gh` ledger plus protected state root | second process performs reconciliation only; total POST count remains one |
| later reconciliation after ambiguous create | two application service executions sharing durable-store semantics | one exact match is linked; total create count remains one |
| existing/multiple correlation match | reconciliation ledger | one valid Issue reused; multiple matches fail before create |
| same external ID in two Provider resources | protected store identity tests | both exact links remain distinct; unqualified lookup fails closed |
| create-attempt stale revision | protected store CAS test | stale update conflicts without changing pending/confirmed truth |
| local revision changed after select preview | store revision fault | stale digest denied; zero save |
| confirmed Issue plus local failure | store fault injection | canonical `partial` with exact Issue reference |
| executable create/select | isolated fake `gh` black box | preview has zero mutation; one authorized create/select link |

## Bounded real-provider observation

The mandatory observation ran from `2026-09-23T02:34:38Z` through
`2026-09-23T03:47:22Z` against implementation revision
`dcddc49c8c51a3eeb515279c7b11712f8cf912bd`.

- Provider/resource: `github` / `rgomids/axiom`
- Project ID: `b6e613a6-dffc-4c9f-8b29-4af63119e6f5`
- Preview digest:
  `d3be82df29047141950e012b439c6d438f45e97db73fc7a9e62f478b129be8e3`
- Correlation:
  `fed8a7fbc4fd1660733a0ad53decfadab41a2077255d97a5e294785e99729ea5`
- Expected effects: `create_provider_work_item`,
  `publish_local_work_item_link`
- Observed effects: exactly one GitHub Issue create and exactly one matching
  protected local Work Item publication
- Issue: `#90`, `https://github.com/rgomids/axiom/issues/90`, state `OPEN`
- Issue attributes: title/body byte-equivalent to preview; zero labels,
  comments, assignees, or milestone; no closure
- Invalid digest: `denied_authority`; zero Provider effects and zero local Work
  Item effects
- Correlation count: `0` before create, `1` after create, `1` after final suite;
  no duplicate Issue was created
- Local record: `formatVersion: 1`, `externalId: 90`,
  `providerRepository: rgomids/axiom`, state `OPEN`
- Local record SHA-256:
  `7056cd648293f2c0ece08e849fcdfb6f4dda5249eb38526274e00f5413c39467`

Validation initially stopped when repository drift was detected. At that
checkpoint no Provider mutation had occurred. After explicit operator authority,
unrelated `landing_page/index.html` work was preserved in stash
`a312ca1833b9d0fe6c87c3c5b1dbde063f5e69ad`, the branch returned to the exact
reviewed clean revision, and the regenerated preview was byte-identical. The
Provider observation then executed exactly once. No T10/S4+ behavior and no
Issue cleanup or closure occurred.

## Reconciled PR snapshot

- Real-provider observation snapshot:
  `dcddc49c8c51a3eeb515279c7b11712f8cf912bd`
- Final reconciled PR validation snapshot:
  `abf1cf649d0090c6332ca1fd9459d27cee2bd977`
- `origin/main` integrated:
  `aaf54becd443a5fd72fee5a62cbb82da58b4ff51`
- Final validation: `12/12` commands `PASS`

The real-provider snapshot remains the revision that exercised exact authority.
The reconciled snapshot is the merge tree on which the complete local suite was
rerun after integrating `main`; the later Evidence-only publication commit does
not replace either historical fact.

The later PR review remediation adds deterministic local/process tests for the
durable create-attempt fence and collision-free persisted identity. It does not
repeat or broaden the historical real-provider mutation and does not alter the
Issue #90 observation facts above.

## Validation environment and commands

Implementation and tests ran on macOS 27.0/arm64 with Go 1.26.1. The local
validation commands below were executed successfully during implementation and
review remediation. The final validated commit is
`7ca31e001d27807535264366a44a56ffa906fbf0` (`7ca31e0`,
`fix: preserve S3 work item compatibility`). GitHub Actions workflow
`POC verification` run
[`35763290464`](https://github.com/rgomids/axiom/actions/runs/35763290464)
validated that exact commit successfully; jobs `verify (ubuntu-24.04)` and
`verify (macos-15)` both completed successfully. The validation commands are:

```bash
go test ./...
go test -race ./...
go vet ./...
go build ./...
go mod verify
./scripts/validate-repository.sh .
./scripts/check-sensitive-files.sh .
./scripts/check-sensitive-files.sh --staged .
./scripts/dogfood-poc.sh
gitleaks detect --source . --no-git
gitleaks git --staged --redact --no-banner
git diff --check
```

The Go suites include pure unit tests, application/store integration, adapter
process-boundary tests, CLI tests, and installed executable-style black-box
tests. Every command above exited zero; Gitleaks reported no leaks. The final
technical suite after the real observation passed `12/12`. Provider traffic in
the deterministic tests uses synthetic fake executables and temporary protected
roots; the separately documented bounded observation is the only real GitHub
mutation in this Evidence.

### PR review remediation validation — 2026-09-23

The durable create-attempt and persisted-identity corrections were validated on
macOS 27.0/arm64 with Go 1.26.1. The following commands exited zero:

```bash
go test ./...
go test -race ./...
go vet ./...
go build ./...
go mod verify
./scripts/validate-repository.sh .
./scripts/check-sensitive-files.sh .
./scripts/check-sensitive-files.sh --staged .
./scripts/dogfood-poc.sh
gitleaks detect --source . --no-git --redact --no-banner
gitleaks git --staged --redact --no-banner
git diff --check
```

The final dogfood run reported binary SHA-256
`05af88e3bdc37ff1c0547574b7305d15320601ba989937c9245f351be9cca8e4`
and workflow SHA-256
`09bd10109ec5ef0e79f3900304880ecc3095c469bf58655875bfcffe5b15ab3e`.
Its result was `pass`. All Provider behavior in this remediation validation used
the controlled fake executable; no new real GitHub mutation was authorized or
performed.

## Limitations and remaining gates

- The mandatory bounded real GitHub observation is complete. Issue #90 remains
  open; no cleanup/closure was authorized or performed.
- Historical POC comment/complete and workflow code remain compatibility inputs;
  they are not S3 completion behavior and were not advanced.
- GitHub Issues is the only implemented Work Item capability. The application
  boundary is provider-neutral; a generic provider framework or second Provider
  remains explicitly out of scope.

## Acceptance status

T08 is technically complete. T09 is technically complete, including its
mandatory bounded real-provider observation. S3 technical completion criteria
are satisfied. Human acceptance is not claimed. No authority exists here to
start T10 or any S4–S7 Task; this Evidence does not declare the MVP complete.
