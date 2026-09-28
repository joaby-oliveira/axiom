# Changelog

## [2026-09-28]

- implementation (S9/T37): the canonical public executable is `axiom`. Release
  archives, `MANIFEST.sha256`, the release installer, the owned upgrade path,
  and the source installer publish `axiom` (receipts, destinations and staging
  names follow). Help, recovery/compatibility/upgrade diagnostics, and the five
  Codex skills invoke `axiom`; the previous skill digests and skill-set receipt
  stay recognized as owned so existing skill installs upgrade. Internal Lingo
  packages, `cmd/lingo`, `LINGO_*` variables and state roots are unchanged. A
  prior `lingo` executable or pre-`axiom` release receipt is preserved, not
  migrated.
- fix: `build-release-archives.sh` builds from its own checkout instead of the
  caller's working directory.

## [2026-09-26]

- implementation: explicit Evidence retirement (HD-S7-T18). `lingo artifact
  retire` previews, then with exact authority publishes
  `artifacts/v1/retirements/<id>.json`, which is separate from metadata v1.
  Cleanup makes Evidence eligible 365 days after a valid retirement bound to
  the exact revision. Referenced, stale, corrupt, or missing retirements
  preserve Evidence, and a re-reference supersedes the retirement.
- docs: close S7 technically. T22 is complete under the revised S7 scope
  (HD-S7-T22). The Ubuntu 26.04 native rows were not executed; they are
  deferred to the T24 clean-environment RC acceptance matrix and remain
  mandatory there.
- implementation: `lingo upgrade` publishes the candidate's verified Codex skill
  files after the binary and receipt. Each file needs an expected digest,
  owned content, and the skill-set lock, and takes part in preview, authority,
  the ledger, interruption, and resume. When skill files change, the
  skill-set receipt is left for the upgraded binary and reported as
  `refresh_required`.
- governance: Codex and Claude are both maintainer runtimes. `AGENTS.md`
  stays the single agent policy; the only allowed runtime bootstrap is a root
  `CLAUDE.md` containing exactly `@AGENTS.md`, enforced by
  `scripts/check-claude-bootstrap.sh`. This is not S8 product multi-runtime
  orchestration.
- fix: the S7 native suite no longer aborts on a clean checkout under macOS
  bash 3.2, and asserts the runtime status result correctly.

- implementation: deliver authorized MVP S7 (T16–T22) maintenance paths:
  read-only `compatibility inspect`; separately authorized POC `compatibility
  backup` and portable `compatibility export`; reference-aware `artifact
  cleanup`; guided `recovery inspect|apply`; and owned `upgrade` with ordered,
  individually confirmed binary/receipt effects and resumable partial state.
- implementation: classify persisted state with the real v1 decoders and a
  frozen `v0.1.0-poc.1` workflow signature; fixtures are produced by the
  historical tag binary via `scripts/generate-poc-fixture.sh`.
- fix: recognize the `v0.1.0-poc.1` `axiom-work-item-create` skill as a known
  legacy Axiom skill; its digest was omitted when the skill changed in S3.
- fix: build release archives with `COPYFILE_DISABLE=1` so macOS `tar` does not
  embed AppleDouble `._*` entries that are absent from `MANIFEST.sha256`.
- security: every maintenance mutation requires the exact current preview
  digest; uncertain, mixed, unsafe, or contradictory state is preserved; the
  inventory never opens non-regular files; Evidence artifacts are not
  age-eligible without a recorded retirement time.
- test: add S7 compatibility, transfer, cleanup, recovery, upgrade, black-box,
  `scripts/test-s7-security.sh`, and `scripts/test-s7-native.sh` coverage.

## [2026-09-24]

- implementation: deliver authorized MVP S6 (T26–T29) with a ten-stage Work
  Item lifecycle derived from canonical Execution gates and revisioned local
  facts, without persisting a second state machine.
- implementation: extend exact GitHub projection with one lifecycle label,
  independent bounded flags, reference-first history, strict drift detection,
  and legacy S4-label recognition while preserving foreign content.
- implementation: add the strict provider-neutral `work-item-metadata` policy
  contract, deterministic resolution and adapter-owned GitHub effect previews;
  add read-only missing-local-state classification and exact ADR-0007 generation
  recovery plans without synthesizing Execution truth.
- security: require exact revisions, validated references, explicit local fact
  authority, digest-bound Provider previews, and fresh exact recovery authority;
  Provider, Repository, CI, merge, review, and Issue state grant no workflow or
  human-acceptance authority.
- test: add lifecycle/fact/flag/drift, metadata schema/precedence/capability,
  reconciliation, adapter, CLI, black-box, and backward-compatibility coverage.

- fix: enforce long-form flags uniformly across Project, Work Item create, and
  existing strict selector parsers; preserve repeatable configuration repositories
  and return canonical selector failures before prompts or application dispatch.

- fix: preserve operation-specific `setup`, `project`, `draft`, `selection`,
  `workItem`, `workflow`, and `projection` payloads separately from canonical
  completion in installed Codex skills; restore the resolved Project payload in
  `project show` without allowing nested data to synthesize canonical fields.

## [2026-09-23]

- fix: require installed Codex skills to copy only canonical top-level completion
  fields, omit absent fields, and never reinterpret operation payloads as
  `details`; preserve controlled upgrade from the prior Axiom-owned v2 digests.
- fix: reject unsupported single-hyphen selector flags before Go `flag.FlagSet`
  parsing so duplicate or mixed `-project`/`--project` and `-execution` forms
  cannot silently overwrite prior values or reach application/store/Provider code.
- test: prove the seven canonical completion statuses preserve result, applicable
  references/next/details, and provenance from central completion through CLI JSON
  to the Codex-facing contract; add an ambiguous Project-slug fixture with zero
  portable, local, Provider, or Runtime effects.
- implementation: add the authorized MVP S5 strict selector path for exact
  Project UUID/slug, Project-scoped Repository, canonical GitHub Work Item, and
  applicable Execution identity without CWD/Git/Provider/Runtime fallback.
- implementation: evolve the five installed Codex skills to skill set v2 and
  binary compatibility v2 as thin `lingo --json` adapters over canonical
  completion, provenance, authorship, detail-reference, and authority semantics.
- security: reject unknown, duplicate, conflicting, malformed, and shell-like
  selector input before application dispatch; exact Work Item and Execution
  references are revalidated against protected local state before any operation.
- test: add missing-only/zero-question prompt counts, UUID/slug equivalence,
  Repository/Work Item/Execution scoping, unrelated-CWD black-box, zero-effect
  failure ledger, detail-reference rendering, skill compatibility, and bounded
  real Codex Runtime observations using isolated roots and no Provider mutation.
- evidence: record S5 deterministic and real Runtime Evidence without claiming
  human acceptance, Provider authority, release readiness, or S6 authority.

- implementation: complete the deterministic implementation scope of authorized
  MVP S4 (T10–T13) with a closed bounded machine-local Execution record, exact
  revision transitions, interruption/resume, validated artifact/Evidence
  references, and local-only completion truth.
- implementation: add preview-digest-bound GitHub stage/comment projection with
  stable per-revision keys, Axiom-owned label replacement, non-Axiom preservation,
  bounded reinspection, intended/confirmed effect bookkeeping, and reconcile-first
  ambiguity handling; GitHub Issue closure is not part of workflow completion.
- test: add application, protected-store, F0–F8, two-process barrier/crash,
  GitHub adapter, CLI black-box, and installed-binary dogfood coverage for one
  winning revision, stale authority, replay, Provider failures, no duplicate
  comment, and truthful partial outcomes.
- evidence: record deterministic S4 Evidence. The required bounded real-provider
  projection observation remains behind separate exact human authority; no human
  acceptance or S5+ authority is inferred.
- fix: bind projection authority to exact Provider/resource and observed Issue
  identity, URL, state, labels, and comment presence so external Issue-state
  changes invalidate stale authority before Provider effects.
- fix: reconcile previously intended Provider effects into the local
  Intended/Confirmed ledger before planning new mutations, allowing a confirmed
  effect plus bookkeeping failure to converge without duplicate mutation.

- fix: reserve a protected durable create-attempt fence before GitHub POST;
  ambiguous or unknown outcomes now remain reconciliation-only across process
  restarts, preventing automatic duplicate creation.
- fix: key new local Work Item records by provider, resource, and external ID
  while retaining exact validated reads and updates for legacy v1 records;
  unqualified same-number collisions fail closed.
- docs: reconcile T08/T09 and S3 status with the completed bounded real-provider
  observation while preserving current human-review, human-acceptance, and
  T10/S4+ gates.

## [2026-09-22]

- implementation: complete the deterministic implementation scope of the
  authorized MVP S3 boundary (T08–T09) with a
  provider-neutral, authorship-preserving Intent draft; deterministic missing-field
  interview; reviewed digest; exact GitHub create/select authority; and generic
  protected local Work Item linkage.
- security: keep draft preview read-only, reject bounded secret/control/oversized
  input before effects, pass untrusted Issue content through JSON stdin, bound
  provider time/output, validate exact GitHub identity/state, and reconcile by
  correlation before every retry boundary to prevent blind duplicate creation.
- test: add unit, adapter, integration, CLI, and executable black-box coverage for
  question minimization, cancellation/denial, provenance, metacharacters, stale
  local revision, timeout/output/rate-limit/ambiguous responses, duplicate
  prevention, truthful confirmed-provider/local-failure partial results, and the
  bounded installed-binary dogfood journey through the reviewed S3 draft.
- fix: preserve the existing Work Item `formatVersion: 1` wire identity while
  mapping it to provider-neutral domain fields; reject providers and external IDs
  that the GitHub-specific v1 adapter cannot represent before writing.
- fix: classify GitHub adapter failures from bounded `gh api --include` HTTP
  metadata: authentication and deterministic 4xx failures are non-retryable,
  rate limits and 5xx failures are explicit retry boundaries, and unknown CLI
  failures fail closed without retry.
- evidence: complete the mandatory bounded T09 real-provider observation under
  exact per-run authority with one GitHub Issue create and one protected local
  Work Item link; this does not imply human acceptance.
- docs: make the repository agent an explicit contributor governed by the
  canonical contribution workflow and Pull Request template.
- docs: establish the English README as the stable canonical landing page, add
  a complete Brazilian Portuguese translation, require same-change
  reconciliation, and validate reciprocal language navigation.
- research: consolidate the completed Spec-Kit experiment findings in Axiom
  Notion discovery and retire the temporary versioned experiment without
  changing ADR-0002 or erasing legitimate historical references.
- fix: make the distributed `darwin && !cgo` binary inspect extended ACL state
  through the open file descriptor, preserving fail-closed volume-capability,
  ownership, permission, type, symlink, and hard-link checks for Codex skills
  and Project state.
- test: execute first run, five-skill install, ready status, equivalent reinstall,
  manifest-digest checks, private mode/ACL checks, and foreign-content refusal
  through the installed binary extracted from the native macOS archive.

## [2026-09-21]

- fix: validate the complete S2 release row against exact macOS 27.0 or Ubuntu
  26.04 host facts, record and preserve `installedAt` in the closed installation
  receipt, and keep equivalent reinstall idempotent.
- security: reject permissive modes and extended ACLs on existing install and
  Codex skill roots; replace the removable Codex lock directory with a private,
  schema-checked advisory lock that distinguishes active concurrency from a
  safely resumable lock released by process death and preserves ambiguous state
  as `recovery_required`.
- test: add exact-version/distro rejection, unsafe root/ACL, closed receipt,
  installation-time preservation, active cross-process lock, real `SIGKILL`,
  abandoned-lock resume, and ambiguous-lock preservation coverage. Scope remains
  S2 T04–T07; no S3+ behavior or release publication was added.

- implementation: complete the authorized MVP S2 boundary (T04–T07) with
  checksummed exact-version archives for the three approved target rows, owned
  local installation, a closed five-skill Codex manifest, explicit first-run
  compatibility, read-only Project setup preview, and digest-bound portable/local
  publication.
- security: refuse checksum, platform, ownership, link, type, stale-authority,
  repository-replacement, concurrent-publication, and foreign-content conflicts;
  preserve confirmed binary or portable effects as truthful partial state without
  shell-profile, Git, Provider, Repository, or ambient-CWD mutation.
- test: add archive/install interruption and recovery cases, skill compatibility
  and partial-resume matrices, guided/full-input equivalence, capability blocking,
  multi-Repository separation, two-process setup concurrency, unrelated-CWD
  resolution, and S2 Evidence. Native target execution remains explicitly deferred
  to T22/T24; S3–S7 remain unauthorized.

- implementation: complete the authorized remainder of MVP S1 with closed,
  bounded machine-local Markdown detail artifacts, opaque identity/correlation,
  digest-checked create/read, explicit capacity failure without eviction, and
  completion-aware required/optional artifact materialization.
- security: add the shared protected local publication state machine with fixed
  broad-to-narrow process coordination, expected byte revisions, private staging,
  versioned recovery markers, protected old-or-new commit points, F0–F8 fault
  classification, and fail-closed readers for artifact, Work Item, workflow,
  Project, and installation state.
- test: add codec/sanitization/limit/capacity/link/type/permission/digest cases,
  stale-authority and old/new matrices, deterministic F0–F8 injection,
  multi-process barrier/crash coverage, and S1 regression Evidence. No S2 work,
  Provider mutation, Git remote mutation, Runtime installation, or release work.
- fix: preserve reconciliable Work Item selection by loading the observed local
  revision before update, keep first selection on the protected create path, and
  expand per-store T03 Evidence with explicit F0–F8 applicability and tests.
- fix: restore strict create-conflict semantics in the shared protected file
  publisher so repeated workflow start loads the existing lineage and reports
  `workflow_already_started`; keep operation-specific idempotency in owning
  stores instead of the publication primitive.

## [2026-09-20]

- implementation: complete explicitly authorized Specification 004 T01 only with
  central immutable completion/provenance contracts, seven closed terminal
  statuses, six semantic result fields, bounded equivalent human/JSON renderers,
  truthful release/development/dirty/unavailable build identity, and canonical
  `version`, `project validate`, and `project show` read-only surfaces.
- test: add status/effect and provenance matrices, golden human/JSON output,
  renderer-failure confirmed-effect preservation, authorship sentinels, output
  bounds, build-flag black-box cases, zero-mutation read-only ledgers, and static
  dependency/renderer checks. T01 Evidence is produced; human acceptance remains
  pending and T02–T25 remain unauthorized.

- approval: record the explicit PR #72 human decision approving the corrected
  final Specification 004 DAG of 25 Tasks as reconciled with the approved Plan.
  The decision concludes the Tasks phase only; implementation, T01 or any other
  Task, Provider mutation, prerelease publication and release remain
  unauthorized.

- planning: decompose the approved Specification 004 Plan into 25 proposed
  vertical Tasks across S1–S7, with an explicit acyclic dependency graph,
  FR-001–FR-037, AC-01–AC-24, security/NFR, SEC-001–SEC-005, HD-1–HD-4 and
  ADR-0001–ADR-0008 ownership, task-level authority/side-effect/recovery rules,
  and future native RC Evidence. The corrected final artifact was approved in
  PR #72; implementation, Provider mutation, prerelease publication and release
  remain unauthorized.

- approval: record the explicit PR #71 human decision accepting ADR-0007 and ADR-0008 and approving the Specification 004 Plan. Authorize the Tasks phase only; implementation, Provider mutation, migration execution, installer/release publication and final MVP acceptance remain unauthorized. Windows support remains a future roadmap follow-up and does not change the current MVP support matrix.

- architecture: propose ADR-0007 for one shared logical local publication and
  recovery protocol, including deterministic coordination, private preparation,
  protected publication, prior/new complete generations, commit truth and
  fail-closed readers. Keep ADR-0005 as the threat-model authority and leave
  syscalls, filenames, layouts, libraries and Go packages to implementation.
- architecture: propose ADR-0008 for the minimal versioned machine-local Execution
  record required by the bounded sequential MVP workflow. Preserve Execution !=
  Agent, local workflow authority, Provider projection separation and the open
  future general Execution graph. Both new ADRs await explicit human review.
- planning: refresh the versioned release matrix from official sources to macOS
  27.0/arm64 and Ubuntu 26.04 LTS/amd64+arm64; separate supported product targets
  from current GitHub-hosted CI availability. Record initial quota/retention
  rationale, capacity-exhaustion outcomes and dogfooding review obligations without
  claiming benchmark Evidence. Plan remains ready for human review and blocked
  from approval/Tasks while ADR-0007/0008 are Proposed.

- planning: add the Specification 004 implementation Plan after PR #70 resolved
  the architecture gate; define vertical delivery slices, GitHub projection,
  Runtime selectors, canonical completion/provenance, machine-local artifact
  layout and retention, bounded APFS/ext4 publication/recovery, clean-v1
  install/upgrade compatibility, RC acceptance, and complete FR/security/AC
  traceability. Plan ready for human review; no Tasks, implementation, Provider
  mutation, migration, or release authorized.

- architecture: accept ADR-0005 directly from Specification 004 HD-3, defining the bounded local filesystem threat model: exact authorized-target confinement, supported traversal/link/replacement protection, process concurrency, deterministic injected faults, complete canonical state, fail-closed uncertainty, restrictive local metadata and guided recovery remain required; malicious same-UID arbitrary interleavings, physical power loss and physical-media durability are explicit unsupported guarantees.
- architecture: accept ADR-0006 directly from Specification 004 HD-2, defining one Axiom-owned machine-local detail-artifact boundary with stable identity, Execution-first/pre-Execution correlation, Evidence references, purpose-based retention and explicit reference-aware safe cleanup; no operation-attempt entity, layout, package or retention duration is selected.
- reconciliation: record H13 and align Specification 002 SEC-003/SEC-005, affected acceptance criteria, Plan, Tasks, conceptual model, indexes, roadmap and historical POC Evidence with HD-3 while preserving the original approval/Evidence history. No Specification 004 Plan, Tasks, implementation, migration, installer or release work started.

- specification: draft Specification 004 for human review, consolidating the MVP
  clean-environment journey, Project/Work Item UX, workflow visibility, Runtime
  selectors, completion output, detailed artifacts, provenance, persistence and
  compatibility, onboarding, acceptance Evidence, explicit non-goals, and four
  human decisions; record HD-1 through HD-4 as checksummed macOS/Linux binary
  distribution, durable machine-local detail artifacts, a bounded threat model
  requiring Specification 002/security/architecture reconciliation before Plan,
  and clean v1 compatibility without automatic POC migration. Record explicit
  human approval of Specification 004 on 2026-09-20; authorize only the required
  HD-3 Specification 002/security/architecture reconciliation and ADR preparation.
  Plan, Tasks, implementation, and release remain gated.

- acceptance: record explicit human acceptance of the merged E2E Codex POC; close #21/#30/#40, preserve #19/#20 as known proof gaps, and capture MVP follow-ups #54–#57 for guided Project setup, Intent-driven Work Item creation, provider-visible workflow progress and argument-driven Runtime skills.
- reconciliation: close the historical POC validation card #53 and POC-scoped #19/#20 after acceptance without claiming their residual proof gaps are solved; carry persistence/recovery hardening into future MVP specification instead of leaving stale POC work open.

- fix: make source installation report deterministic PATH setup guidance without shell-profile mutation, mark dirty-checkout builds in installer/version metadata, and preserve confirmed GitHub Issue identity/state through post-provider Work Item or workflow persistence failures.

## [2026-09-19]

- specification: approve the complete E2E Codex POC journey, implementation plan and #31–#40 task mapping; preserve existing Axiom/Lingo, Project/Repository and portable/local boundaries. Record the supported standalone Codex `$axiom-<skill>` mapping because literal colon names fail the current hyphen-case skill contract; final human acceptance remains separate.

- feature: add a source-checkout Axiom installer that publishes Lingo to an explicit user PATH directory, records checksum ownership, supports safe idempotent reruns, refuses modified/unowned destinations, and exposes build source/revision through `lingo version`.

- feature: add the Codex Runtime bootstrap with five validated user-global `$axiom-*` skills, exact status inspection, idempotent install, conflict/symlink protection and attempt-local rollback. Skills remain thin Lingo entrypoints; no Project or workflow rules are duplicated.

- feature: resolve installed Projects by UUID or slug from protected local state independently of caller CWD; preserve multiple repository bindings and fail explicitly for ambiguous selectors, missing sources, moved repositories and invalid local records.

- feature: configure a complete Project through guided or argument-driven Lingo, publish repository keys in portable intent and absolute working-copy paths only in strict local state, then resolve immediately from any CWD. Preserve separate portable/local commit truth on post-commit failure.

- feature: add bounded GitHub Work Item create/select/show/comment/complete capabilities behind provider-neutral ports, explicit external mutation authority, exact provider-reference validation, protected local linkage and bounded `git`/`gh` transports.

- feature: add the persistent sequential Axiom delivery workflow with fixed Specification-through-Reconciliation gates, repository-anchored artifact digests, inspectable interruption/resume, and explicitly authorized Work Item completion.

- feature: complete equivalent Lingo/Codex entrypoints with a real `project show` command, default human output, explicit typed `--json` payloads, command/skill help, exact thin-skill command templates and controlled upgrades of known prior Axiom skill content.

- test: expand `dogfood-poc.sh` into the complete isolated install, global-skill, unrelated-CWD Project, GitHub Work Item, interruption/resume, Evidence, reconciliation and completion journey; record a real user-global Codex discovery run for #39.

- docs: reconcile README, architecture, setup, roadmap, Specifications and final POC report to the delivered Codex/GitHub/sequential-workflow scope; preserve #19/#20 proof gaps and explicit human acceptance for #21/#30/#40.

## [2026-09-18]

- docs: reconcile Specification 002 and Plan scope for the POC-only Linux/macOS verification workflow; classify its runs as acceptance Evidence, not product CI or release automation. Preserve #19–#21, #14 and SEC-003 human gates.

- fix: check complete private-file writes, staged-file bytes/link count, opened directory identity and platform ACLs before one-file publication; add controlled storage-fault, process-conflict, writer-conflict, ACL and replacement-race regressions. Run POC tests and dogfooding on macOS/Linux in GitHub Actions; document manual recovery and remaining fault/race proof gaps without declaring acceptance.

- fix: anchor minimal POC filesystem operations to private directory handles, reject symlink/hard-link targets, publish create/install without replacement, keep validate read-only, and report interrupted attempts through durable markers as recovery-required. Add process crash-boundary and executable black-box tests plus reproducible dogfooding Evidence; Linux fault proof and human acceptance remain open.

- fix: honor cancellation observed immediately before update/install publication; clear the owned attempt marker and preserve the prior manifest or absent local record. Add deterministic pre-commit cancellation regressions.

- fix: align Lingo's default machine-local root with native macOS Application Support and Linux XDG state directories; reject invalid explicit root overrides. Reconcile Specification 002's POC delivery status while #19–#21 and human acceptance remain pending.

- feature: add the initial local Lingo POC lifecycle: `project init`, `validate`, `reopen`, and explicit name `update` for a minimal one-file portable Project. The adapter uses validated slugs, private staging for create, atomic manifest replacement for update, per-slug coordination and sanitized JSON outcomes. Optional documents, installation/local state, bindings, rename, runtime/provider integration, Git and network remain unsupported.

- feature: add explicit local installation for the minimal Lingo POC. `project install` reads a strict portable manifest and writes an ID-addressed `installation.json` outside the portable root without rewriting portable bytes. Credential resolution, bindings and automatic local-state reconciliation remain unavailable.

- docs: record consolidated POC lifecycle Evidence, reproducible checks and explicit filesystem/security limitations for final human review.

## [2026-09-17]

- docs: adopt Apache-2.0 and establish contribution, conduct, and support policies with structured GitHub Issue and Pull Request templates. No implementation or new Task authorization.

- docs: add the approved Axiom logo and centered README header with verifiable Go 1.26+, main last-commit, and repository-stars badges.

- docs: establish documentation governance with canonical sources, classification, lifecycle and anti-drift rules; align the agent documentation policy and README navigation.

- docs: reconcile product and agent context with the implemented Go foundation; separate roadmap/ADR direction from Specification lifecycle, preserve pending T04 acceptance, and make onboarding portable. Keep README and approved contracts unchanged.

- docs: rewrite the README as a project landing page with problem and audience, verified maturity, conceptual workflow, runnable exploration steps, and links to deeper documentation. Preserve detailed implementation history in existing technical documents; record the absence of a project license file.

## [2026-09-16]

- fix: reject invalid UTF-8 document names at the T02 snapshot producer for T04 lossless JSON metadata; preserve valid U+FFFD, # and ?, retain local defense in depth, and add producer/composition regressions. No T05+ work.

- fix: resolve PR #9 ArtifactDigest.Name compatibility finding: T04 reuses the unchanged T02 document-name predicate, preserves valid names including #, ?, controls and literal U+FFFD without normalization, and rejects malformed Unicode without repair. Add real manifest/Project/snapshot/digest/local-record round-trip and negative boundary regressions; preserve portable protections, H12 and all prior Evidence. No T05+ work.

- fix: apply human-approved H12 / Option B within T04: remove persisted localRevision from local format v1, retain unchanged T02 exact-byte LocalRevision and independent portableRevision metadata. Update closed DTO, mapping, fixtures and regressions; reject obsolete field without migration or reuse.
- docs: reconcile Specification/Clarifications/Plan/Tasks and T04 Evidence with explicit human authority; preserve earlier decisions and Evidence. T04 Ready for human re-review, not Accepted; T05–T21 remain unstarted.
- fix: address PR #9 human review within T04: preserve arbitrary local path text without filesystem policy; map domain identity diagnostics to local schema fields. Add public codec regressions; retain invalid-text rejection and shared domain rules.
- docs: investigate ambiguous persisted localRevision versus T02 exact-byte CAS; record both options and recommend single revision pending explicit human decision. T04 blocked on that decision; no revision-model change or T05+ implementation.
- feature: deliver authorized Specification 002 T04 strict local installation JSON codec with exact formatVersion 1, closed metadata DTOs, safe diagnostics, no partial records, explicit missing/corrupt distinction and consumer-owned T02 metadata reuse. Share existing domain identity validation without changing its rules. Add version/structure/security/round-trip tests and static boundary Evidence; no filesystem persistence or T05+ work.
- docs: record T01–T03 Accepted / merged, verified baseline `1de3b02d97818138c32f6b2a07555cfe1ffd4a9b`, T04 Ready for human implementation review and T05–T21 Not started. T04 merge grants no T05 authority; preserve prior lifecycle history and approved contracts.

## [2026-09-15]

- fix: address PR #8 human review within T03: reject relative/local URI paths in logical references and known credential payloads in identifier fields; bound canonical output during serialization. Add encode/decode, escaped-syntax, writer-bound and positive regressions; retain prior implementation Evidence. T01/T02 contracts unchanged; T04–T21 Not started; human re-review required.

- implementation: deliver Specification 002 T03 strict portable manifest codec, closed DTOs, canonical round trips, declaration-state preservation, resource limits, structural security rejection and sanitized diagnostics. Pin experimentally validated YAML parser v3.0.5; add golden, schema, abuse, bounds, pairwise, port integration, fuzz and dependency-boundary tests. T01/T02 Accepted / merged; T03 Ready for human implementation review; T04–T21 Not started. T03 merge does not authorize T04.

## [2026-09-14]

- implementation: deliver authorized Specification 002 T02 consumer-owned application ports, complete snapshots and revisions, exact preview-bound authority, safe ordered issues and truthful portable/local commit outcomes. Add contract/fault/barrier tests and deterministic dependency checks. T01 Accepted / merged (PR #6); T02 Ready for human implementation review; T03–T21 Not started. No concrete codec, persistence, CLI or Git execution.

- implementation: deliver authorized Specification 002 T01 pure Go domain: immutable Project identity, declaration/reference invariants, conservative locators, complete proposed-state materialization and presence-preserving equivalence. Add behavior tests and domain I/O boundary verification; retain reproducible Evidence. Specification/Plan/Tasks Approved; T01 ready for human implementation review; T02–T21 Not started.

Earlier Tasks review on this date (historical):

- planning: record approved/merged PR #4 and explicit Tasks-only authorization; add 21 Specification 002 Tasks with dependency DAG, FR/SEC/AC ownership, H1–H11/ADR/Plan traceability and future platform/security Evidence obligations. Plan: Approved; Tasks: In review; Implementation: Not authorized. Preserve prior decisions and validation history.

- specification: record PR #4 Git Authority approval as H11; preserve H1–H10 and separate Project mutation, optional local Git commit and explicitly authorized remote sync/push, with independent authority/outcomes and no false rollback or local-state export.
- planning: remove resolved Git Authority review gates; defer concrete Git mechanisms and automation design with future Evidence. Reconcile status references and PR description to H1–H11/current scope. Ready for final human Plan review; no Tasks or Implementation.

Earlier H10 reconciliation on this date (historical):

- planning: reconcile PR #4 atomicity decision as H10 across Plan, Specification, Clarifications and ADR-0004. Preserve logical Project atomicity and pre/post-commit outcomes; leave multi-file transaction, staging, lock and syscall choices to implementation with Linux/macOS Evidence. Project model and init/update contract approved; remaining Git Authority review explicit. No Tasks or Implementation.

Earlier reconciliation on this date (historical):

- specification: reconcile PR #4's latest human decision as H9: partial update intent is allowed; domain materializes and validates complete proposed Project state before application-authorized complete persistence. Preserve H1–H8 and original approval history.
- planning: align FR-018, AC-15, use cases and planned verification; forbid direct partial manifest mutation or validation bypass; identify remaining init/update and Git/authority review concerns. Plan ready for new human review; no Tasks or Implementation.
- architecture: refine Accepted ADR-0004 and conceptual model with the same update contract; preserve confirmed identity, portability, minimal init and separate Git authority boundaries. Update H9 index references and correct stale architectural/context references to ADR-0003's existing 2026-09-10 acceptance, without changing that ADR.

## [2026-09-12]

- specification: reopen/reconcile Specification 002 and Clarifications after subsequent human Plan review; preserve original 2026-09-11 approval and record H1–H8 replacing/refining location, minimum and FR-012/update contracts.
- architecture: extend Accepted ADR-0004 with immutable ID versus mutable slug/name, portable working copy versus local state, incremental mutation and optional dedicated Git backing with explicit remote authority; no new ADR or sync engine.
- planning: reconcile Plan for minimal init, explicit update, safe slug move, rollback/concurrency and ID-addressed state continuity; extend future AC-13–AC-18 and security traceability. Ready for human re-review; Tasks and Implementation remain blocked.

Earlier review in this same date (retained historical entries):

- architecture: record Accepted ADR-0004 — Portable Project Manifest from human Plan review; distinguish the durable versioned contract, current axiom.yaml representation and initial schema details.
- planning: reconcile Specification 002 Plan with distinct absent/unconfigured/empty intent and internal installation.json formatVersion; extend future round-trip, no-op and local-version failure coverage.
- docs: reconcile ADR index, conceptual boundary and lifecycle references; preserve Specification/clarification history and ADR-0001–0003. Plan awaits human re-review and final approval; no Tasks or Implementation.

## [2026-09-11]

- planning: add Specification 002 Plan for human review after PR #3 merged, covering slice architecture, strict manifest, local state, safe persistence, security and AC-to-test Evidence; Tasks and Implementation remain gated by explicit Plan approval.

- specification: approve Specification 002 — Lingo Project Initialization after final human review on 2026-09-11; Q1–Q6 resolved and planning authorized after PR #3 merges, in a new change, with implementation still gated by an approved Plan.

- specification: reconcile Specification 002 with human-approved Q1–Q6 decisions; simplify required configuration, specify UUID v4 and strict axiom.yaml, native Linux/macOS local state, presence-only Runtime observations and conservative Repository matching.
- security: distinguish deterministic secret rejection from optional best-effort scanning; express write-target protection as a behavioral invariant covering traversal, symlink redirection and TOCTOU.
- docs: align journeys, FR/SEC/AC coverage, clarifications, index, roadmap and README; record final Approved status and resolved clarifications, without creating Plan, Tasks or implementation.

## [2026-09-10]

- specification: propose Lingo Project Initialization with portable/local boundaries, deterministic creation and installation behavior, security criteria, acceptance evidence, and open clarifications; stop before planning or implementation.
- docs: link Specification 002 from the index, roadmap and README; reconcile stale README references with Accepted ADR-0003.

- architecture: accept ADR-0002 with an independent Axiom SDD harness, domain, and lifecycle informed by Spec-Kit as a strategic upstream reference.
- architecture: accept ADR-0003 with Lingo as Axiom's local executable control plane while preserving Axiom as product, domain, policies, and contracts; implementation remains gated by an approved Specification.
- architecture: refine Project, Execution, Agent, Provider, Integration, Capability, Runtime, Transport, Agent Profile, Model Profile, Agent Planner, Orchestrator, and Business Context boundaries.
- security: separate portable Project configuration from local state and require credential references instead of versioned secrets.
- docs: record no runtime, architectural, behavioral, or file-format compatibility commitment and preserve deliberate divergence.
- docs: add control-plane direction and roadmap for Project configuration, runtime/model portability, capability negotiation, multi-agent orchestration, thin runtime skills, and a future Project Wizard.
- research: preserve Scenario 001 and Scenario 002 as historical evidence while recording the later human decision.
- planning: document an approximately weekly Spec-Kit Upstream Watch as future work without selecting or implementing its mechanism.

## [2026-08-12]

- research: correct Scenario 002 methodology by separating unexpected findings from manually validated false positives and reporting seeded recall.
- test: require exact stable-reference matches, validate unexpected-finding classifications, and preserve original model execution evidence.
- architecture: keep ADR-0002 Proposed with C as the evidence-supported leading hypothesis; no Scenario 003 or implementation authorized.

## [2026-08-11]

- research: run the controlled Axiom and GitHub Spec-Kit `v0.16.2` comparison and preserve temporary reproducibility evidence.
- architecture: propose ADR-0002 recommending conceptual compatibility without a mandatory Spec-Kit dependency.
- docs: publish durable findings, overlap analysis, strategy ranking, measured metrics, and reconsideration conditions.
- research: add Scenario 002 comparing current Axiom, experimental Axiom-native analyze/converge, and a confined Spec-Kit adapter with accuracy, failure, upgrade, and multi-repository evidence.
- architecture: keep ADR-0002 Proposed and retain C as the leading hypothesis after capability-level B vs C evidence.

## [2026-08-08]

- product: add the normative Axiom Constitution and classified product foundation.
- architecture: define the initial conceptual model, provider boundaries, and accept Project as distinct from Repository in ADR-0001.
- specification: propose the Codex agent harness generation vertical slice.
- research: compare four possible relationships with GitHub Spec-Kit without adopting it.
- dogfooding: simulate a Go pull-request review agent and prioritize P0, P1, P2, and Research gaps.

## [2026-08-07]

- feature: bootstrap the public Axiom repository with its Codex agent harness.
- feature: add product, architecture, decision, research, security, and development documentation.
- security: add repository ignore rules, sensitive-file validation, and public-repository policies.
- test: cover local sensitive-file validation, including staged index content.
