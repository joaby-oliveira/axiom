# Acceptance criteria — `v0.1.2-rc.2` (T24/T25, 2026-09-29)

Candidate `v0.1.2-rc.2` at `859969a07f3807822580431a05b6c78b07691fb1`; product
tree unchanged by every tooling revision (`git diff --quiet 859969a <tooling> -- cmd internal go.mod go.sum`).

Evidence kinds:

- **R**: real observation against the published RC in this session. Ledgers
  are in [execution/](execution/); the log is in the [checkpoint](README.md#execution-log).
- **D**: deterministic suites at the exact product tree. These are
  `go test ./... -count=1` and `go test -race ./...` (exit 0, [checks](checks.md)),
  plus F4, which re-ran executiongraph/graphapplication/coordination/workflow/
  githubissues/workitem/install/cmd suites (exit 0).
- **H**: human decision or attestation.
- **P**: earlier slice Evidence at other revisions (context only; never the sole basis here).

| AC | Kinds | Evidence |
|---|---|---|
| AC-01 | R | A: published-instruction install with `--version v0.1.2-rc.2`, binary digest = verified release, `axiom --json version` provenance `0.1.2-rc.2`/`859969a07f38`/clean; Codex integration used in B. (The canonical executable is `axiom`, FR-062.) |
| AC-02 | R, D | First-run `next`: "Run project configure with explicit Project inputs"; B/C/D configure only from explicit inputs; CWD-independence black-box tests |
| AC-03 | R, D | Argument-driven configure (D1/D2) and Runtime-skill-driven configure (B, C) publish equivalent Projects; guided-mode equivalence tests |
| AC-04 | R, D | Configure preview separates portable destination from local bindings; apply only with digest + `--authorize-local` (B, D2); multi-repository tests |
| AC-05 | D | `TestWorkItemJourneyRequiresReadyConfiguredCapability` |
| AC-06 | R, D | E1 read-only draft preview → E2 creation of [#125](https://github.com/rgomids/axiom/issues/125) only with digest + `--authorize-external`; bounded-question tests |
| AC-07 | R, D | E1/E2 reused the seven supplied sections verbatim; stale-digest `denied_authority` and cancellation tests |
| AC-08 | R | E4/E6/E9: exactly one `axiom:stage:*` label after each projection (`specifying`→`implementing`→`reviewing`); three provenance comments; F1 replay: zero effects |
| AC-09 | R, D | F3: failed `review` gate → `interrupted` (rev 11), resume → `active` (rev 12); projections only follow committed transitions (E3 observation) |
| AC-10 | R | B/C: fully specified skill invocations completed with no selector round trip |
| AC-11 | D | Invalid/conflicting selector black-box tests (`validation_failure`, no CWD fallback) |
| AC-12 | R, D | Observed: `success`; `failure` (F2 drift); `validation_failure` (E3 projection before transition); `denied_authority` (F3 stale resume); `interrupted` (F3). `partial`/`retryable_failure`: black-box tests |
| AC-13 | D, P | Detail-artifact store tests; S5/S6 Evidence |
| AC-14 | D, P | Artifact confinement/digest/correlation tests; S5/S6 Evidence |
| AC-15 | R, D | Provenance in CLI output, Issue body marker and projection comments (`Axiom 0.1.2-rc.2 · 859969a07f38 · clean`); dev/dirty/unavailable variants: provenance tests |
| AC-16 | R | Issue #125 body labels each user section `Authorship: user` |
| AC-17 | D, P | F0–F8 publication/recovery suites (`internal/local`); S7 native macOS Evidence |
| AC-18 | D, P | Compatibility/POC classification and transfer tests; S7 Evidence |
| AC-19 | R | A: reinstall `unchanged` with identical inventory, owned upgrade rc.1→rc.2, foreign/modified/unsafe refused without effect, downgrade refused |
| AC-20 | D | Confirmed-effect-then-local-failure `partial` black-box tests |
| AC-21 | R, D | Install → first run → Project (Codex, Claude, CLI) → Work Item #125 → workflow (Runtime `claude`) → graph Evidence → review, all against the RC. Completion of #125 follows the human review/merge of dogfood PR [#127](https://github.com/rgomids/axiom/pull/127); the completion gate is covered by the black-box full-lifecycle test |
| AC-22 | R, D | Validation (E3, A refusals), authority denial (F3), interruption/resume (F3), recovery-required drift (F2), supported upgrade (A); retryable external failure: black-box tests |
| AC-23 | R | This record: candidate, environment, argv/exit/timeout/attempt ledgers, digests, exclusions, limitations; no raw chat or secrets |
| AC-24 | H | Human acceptance given on 2026-09-29, conditional on A–F completing without a product defect; effective at the end of this execution. No automation filled it |
| AC-25 | R, D | Lifecycle labels derived from gates + facts (`planning-authority`, `implementation-authority`, `review-started`) |
| AC-26 | R, D | F2: lost established marker → `failure`, zero Provider effects |
| AC-27 | R, D | One stage label per projection; obsolete managed label removed; foreign-content preservation tests |
| AC-28 | R | Comments bounded and reference-first (`evidence:.t24-evidence/graph-acceptance-evidence.json`); replay idempotent (F1) |
| AC-29 | D | Only terminal completion + explicit human fact yields `accepted` (black-box); nothing in this run produced `accepted` |
| AC-30 | R, D | F3 `recovery inspect` success; missing-state recovery tests |
| AC-31 | R, D | Work Item metadata resolved by the GitHub adapter; policy tests |
| AC-32 | R | Graph runs 01–03: approved plan → planner proposal → three-node DAG derived from work units ([run 03](execution/graph-run03/)) |
| AC-33 | D, P | Cycle/dependency/effect/revision/escalation tests; T36 |
| AC-34 | R, D | Profiles resolved only to observed, allowlisted Runtimes (`prepare.json`); no-match/no-fallback tests |
| AC-35 | R | Run 03: Codex and Claude children overlapped (23:18:24–23:18:51 UTC) in isolated worktrees; Integration waited on both |
| AC-36 | R | Run 03 Evidence: Codex 0.157.1 / `t24-codex-implementation`, Claude 2.1.285 / `t24-claude-documentation`, invocation digests |
| AC-37 | R | Canonical question (Codex) and answer (Claude) correlated to the final attempts |
| AC-38 | R, D | Integration preview → authority → apply; ten combined validators pass; result tree `20da4d95…`; conflict/drift tests |
| AC-39 | R, D | Controls 20 m/2 attempts (children), 15 m/2 (integration); usage/cost `unavailable`; enforcement tests |
| AC-40 | R, D | Runs 01/02 failures recorded as non-ambiguous failed attempts; in-place retry refused fail-closed (dirty workspace); cancellation/retry tests |
| AC-41 | R, D | Runs 01/02: the failed Codex child gave non-success graph truth while the Claude sibling's success stayed preserved; roll-up tests |
| AC-42 | R | `acceptance-evidence.json` (SHA-256 `879f36cc…`), status `real_run_recorded` |
| AC-43 | R, D | Sequential workflow Execution `428b87fb…` operated alongside the graphs; compatibility replay tests |
| AC-44 | R, H | macOS 27/arm64: A. Linux amd64/arm64: human-attested manual validation |
| AC-45 | R | T23 rc.2 publication read-back; `release.sh verify --download` pass |
| AC-46 | R | A: no-op reinstall, protected owned upgrade, older version refused |
| AC-47 | R | A: Codex-only, Claude-only, both, none; idempotent; no Runtime install or credential change |
| AC-48 | R, H | Exact `--version v0.1.2-rc.2` pin in every installation (A, B, C, D); Linux rows human-attested ([#126](https://github.com/rgomids/axiom/issues/126)) |
| AC-49 | R | Real dogfood: Axiom Work Item #125 → Codex + Claude graph run 03 → Integration/Reconciliation → PR [#127](https://github.com/rgomids/axiom/pull/127); parent/child Evidence above |

Limitations:

- Linux rows are human-attested, not observed here.
- C used byte-identical host copies of two skills.
- #125/#127 completion is pending human review.
- Usage/cost is unavailable.
- No product defect was found.
