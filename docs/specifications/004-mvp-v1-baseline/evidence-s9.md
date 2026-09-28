# Evidence — MVP Slice S9: productization T37–T40

## Claim and authority boundary

This record describes the implementation of Specification 004 Slice S9 Tasks
T37, T38, T39 and T40, tracked by [Issue #81](https://github.com/rgomids/axiom/issues/81)
and delivered for review in [PR #106](https://github.com/rgomids/axiom/pull/106).

**Canonical contract.** [PR #104](https://github.com/rgomids/axiom/pull/104)
merged the S9 Specification/Plan/Tasks amendment into `main` at
`2f4563e4bd478a5c6c862fe7cff3dfae512184b4`. From that commit on, `main`'s
[Specification](spec.md) FR-062–FR-067 / AC-44–AC-49, [Plan §13](plan.md) and
[Tasks](tasks.md) T37–T40 / T23–T25 are the only S9 contract. The integration
branch had earlier merged an unmerged copy of PR #104 (`c75d832`) and then
recorded its own "human decision 2026-09-28" text for T40 in FR-066, Plan §13
and Task T40. The reconciliation merge `7bb6418` took `main`'s Specification,
Plan and Task text verbatim and dropped that text: the T40 behavior below is
**implementation behavior**, assessed against FR-066/AC-47, not an approved
requirement.

**Authority.** The operator authorized local T37–T39 implementation on
2026-09-28 in parallel with S8/T36 (S8/T36 gates entry into T23–T25), and T40
followed. This Evidence does not claim authority for, and nothing here
performed: merge, tags, GitHub Releases or prereleases, workflow dispatch,
deploy, credential or secret changes, Runtime installation or invocation,
T23–T25, or acceptance of S9 or the MVP. No release has been published, so the
remote bootstrap has never installed a real published Axiom release.

| Task | State | Commits |
|---|---|---|
| T37 public `axiom` CLI and distribution identity | implemented; validated locally | `096acb0`, `e4361fa` |
| T38 automated release artifact pipeline | implemented; validated locally; workflow never dispatched | `90f1eab` |
| T39 stable remote installer and owned upgrade | implemented; validated with local fixtures; no published release exists | `eb88832`, `c9ec3a3`, `9e474e4`, `7d78156` |
| T40 Codex + Claude first-run bootstrap | implemented; validated with fake Runtime executables; real Runtimes never invoked | `721acd0`, `9c14299` |

## Requirement traceability

| Requirement | Implementation | Tests | Evidence kind | Status |
|---|---|---|---|---|
| FR-062 public `axiom` identity | `build-release-archives.sh`, `install-release.sh`, `install-axiom.sh`, `internal/install`, help, diagnostics, skills | `internal/cli` help test, `TestPreAxiomExecutableSkillsAndReceiptRemainUpgradeable`, `test-release-archives.sh`, `test-install-axiom.sh`, `verify-release-artifacts.sh` | confirmed (local deterministic tests; native macOS 27 host; synthetic Ubuntu row) | met for delivered surfaces |
| FR-063 release artifacts from one revision | `release-artifacts.yml`, `release-tag-version.sh`, `verify-release-artifacts.sh` | `test-release-pipeline.sh`, local replay of the workflow `run:` steps | confirmed locally; GitHub workflow **not run** | implemented; GitHub execution unverified |
| FR-064 stable remote installation and selection policy | `scripts/install.sh` | `test-install-bootstrap.sh` (fake `curl`, local release fixtures) | confirmed on native macOS 27/arm64 host and synthetic Ubuntu 26.04/amd64 row; live GitHub read-only checks | selection/verification logic met; installation of a published release **not run** (none exists) |
| FR-065 convergent reinstall/upgrade | `install-release.sh` handing to the candidate's protected `axiom upgrade` | `test-install-bootstrap.sh`, `test-s7-native.sh`, `internal/install` | confirmed with local fixtures (native macOS 27 host, synthetic Ubuntu) | met for local fixtures |
| FR-066 multi-runtime first run | `internal/runtimebootstrap`, `internal/codexruntime` integration descriptor, `cmd/lingo/runtime_bootstrap.go` | `internal/runtimebootstrap`, `internal/codexruntime`, executable matrix `TestExecutableFirstRunRuntimeMatrix`, `dogfood-poc.sh`, `test-release-archives.sh` | confirmed with fake `codex`/`claude` executables; **real Runtime not run** | met for the four-state matrix; see findings F6–F8 |
| FR-067 self-hosted acceptance | — | — | **not run** | not started (T24/T25) |
| AC-44 | FR-062/FR-064 above | as above | installation from a published release **not run** | open until T23/T24 |
| AC-45 | FR-063 above | as above | local only | open until the workflow runs for T23 |
| AC-46 | FR-065 above | as above | local fixtures only | logic met; native published-release journey is T24 |
| AC-47 | FR-066 above | as above | fake executables only | logic met; real Codex/Claude observation is T24 |
| AC-48 | — | — | **not run** | future T24 with `--version vX.Y.Z-rc.N` |
| AC-49 | — | — | **not run** | future T24/T25; not claimed |

## T37 — public `axiom` executable

- Release archives, `MANIFEST.sha256`, `install-release.sh`, the Go owned
  upgrade (`internal/install`) and `install-axiom.sh` build and publish the
  executable as `axiom`. Receipt destination, staging names
  (`.axiom-binary-stage.*`, `.axiom-source-stage.*`) and the PATH notice follow.
- Help, recovery/compatibility/upgrade diagnostics, `first-run` guidance and
  the five Runtime skills invoke `axiom`. The previous skill digests and
  skill-set receipt stay known, so an installed pre-T37 skill set upgrades.
- Unchanged: `cmd/lingo`, internal Lingo packages, `LINGO_*` variables, state
  roots and internal staging prefixes. A `lingo` executable is never removed.
  A pre-`axiom` release archive or receipt is not recognized as owned and is
  preserved; there is no migration from such an installation.
- Lingo remains an internal concept. Two user-visible texts still name it
  ("Lingo and <Runtime> skills are compatible", "a Lingo detail reference");
  they are not executable identity and were left unchanged.

Earlier observation (clean commit `e4361fa`, synthetic Ubuntu 26.04/amd64 row):
every archive lists `-rwx------ axiom-0.1.0-rc.1-<row>/axiom` and no `lingo`;
`install-release.sh` wrote `destination=<bin>/axiom`, `version=0.1.0-rc.1`,
`revision=e4361fac5944`, `sourceState=clean`, `release=true`; in
`env -i … bash --noprofile --norc`, `type -t axiom` is `file` and
`axiom --json version` reported exactly that provenance.

## T38 — release artifact pipeline

- `scripts/release-tag-version.sh` maps `vX.Y.Z` / `vX.Y.Z-rc.N` to metadata
  version `X.Y.Z[-rc.N]`; metadata keeps SemVer without `v`.
- `.github/workflows/release-artifacts.yml` (manual dispatch, one `tag` input):
  `contents: read` only, actions pinned by commit, `persist-credentials: false`,
  no Go cache, tag passed only through the environment; requires HEAD to equal
  `GITHUB_SHA` and a clean tree; runs `go mod verify`,
  `build-release-archives.sh` for all three rows, and
  `verify-release-artifacts.sh`; retains `artifacts/` and
  `release-evidence.txt` as a workflow artifact. It creates no tag, release,
  prerelease or `latest`, and writes nothing to the repository.
- `scripts/verify-release-artifacts.sh` checks the closed file set, checksums,
  closed bundle entries with a `0700` `axiom`, `MANIFEST.sha256`, exact
  metadata, source-identical `LICENSE`/`install.sh`/skills, per-row executable
  format, embedded build information (`vcs.revision`, `vcs.modified=false`,
  `GOOS`/`GOARCH`, `CGO_ENABLED=0`, `-trimpath`) and host-row provenance.
- Determinism: a rerun from the same revision produces identical executables
  and bundle manifests; archive bytes differ (tar/gzip timestamps). FR-063
  requires repeatable preparation and closed checksums, not byte-identical
  archives; T23 must publish the `SHA256SUMS` of the exact run it publishes.
- The `tag` input labels the prepared set; the workflow does not require that
  tag to exist or to point at the dispatched revision. T23 must bind the
  published tag to the recorded `revision` before publication.

`test-release-pipeline.sh` covers tag vectors, dirty source refusal, the full
three-row matrix, rerun equality of executables and manifests, missing row,
missing checksum line, checksum mismatch, extra file, symlinked archive, wrong
version or revision, a `lingo` bundle, development builds and workflow
no-publication checks. The workflow itself, including `actions/upload-artifact`,
has **not run** on GitHub.

## T39 — remote installer and protected owned upgrade

- `scripts/install.sh` (POSIX `sh`) implements FR-064: no selector and
  `--channel stable` resolve the latest published stable release from the
  `Location` of `/releases/latest` only and never fall back to an RC (no stable
  release is an explicit zero-effect failure naming `--version`);
  `--version vX.Y.Z` / `--version vX.Y.Z-rc.N` resolve exactly that tag;
  `--channel` with `--version` is an input error. There is no RC channel:
  `--channel rc`, like any unsupported selector, fails before any request and
  points to `--version vX.Y.Z-rc.N` (usage no longer advertises `rc`, commit
  `7d78156`). No newest-RC discovery, scraping or API parsing exists.
- Exact host row before any download; HTTPS-only downloads; the archive digest
  is verified against `SHA256SUMS` before the archive is read; the bundle's
  `install.sh` and `release-metadata.txt` must match the bundle manifest and the
  resolved version/row of a clean release build; then the bundle's release
  installer runs. It prints tag, asset, asset SHA-256, row, revision and
  receipt. No `sudo`, profile or `PATH` edits, Runtime installation or
  credential access.
- `install-release.sh` hands a different binary in an owned installation to the
  verified candidate's `axiom upgrade` (preview, exact digest, apply), so the
  same version is a no-op, a newer one is a protected upgrade, an older one is
  `downgrade_refused`, and a divergent same version, foreign, modified, unsafe
  or incompatible state is refused. Only an interrupted owned upgrade of the
  same archive resumes.

Earlier run (synthetic Ubuntu 26.04/amd64 row, `dash` and `bash --posix`):
`test-install-bootstrap.sh` passed all 48 cases, and every refusal left HOME and
the temporary directory unchanged. Live read-only checks against GitHub: the
default selector reported that no stable release exists, `--version
v0.1.0-rc.1` reported no published `SHA256SUMS` (HTTP 404), and `--channel rc`
and the selector conflict were refused before any request.

The earlier Evidence also claimed row selection for both Ubuntu rows. Those two
cases reuse the host's `/etc/os-release`, so they can only run on an Ubuntu
26.04 (native or synthetic) host; on other hosts they are now reported
`not_run` instead of failing (commit `7185a1e`).

## T40 — Codex + Claude first-run bootstrap

Delivered behavior (implementation, measured against FR-066/AC-47):

- `axiom first-run` discovers Codex and Claude, converges Axiom's user-global
  skills for every Runtime it detects, and reports every supported Runtime with
  `present`, `configurationWithoutExecutable`, `state` (`absent`, `configured`,
  `already_configured`, `failed`), `reason` and skill states.
- Codex root: `$HOME/.agents/skills` (existing S7 root). Claude root:
  `<CLAUDE_CONFIG_DIR or ~/.claude>/skills/<skill>/SKILL.md`, read only from the
  process environment; a relative or multi-line value fails Claude only.
  Project-local skills are never used.
- It never runs, installs or authenticates a Runtime, and never reads or
  changes credentials, secrets, subscriptions or Model Profiles. The executable
  matrix proves this with fake `codex`/`claude` executables that would leave a
  mark if run, and by listing HOME afterwards.
- New `axiom runtime claude install|status`, beside `runtime codex`.

Implementation choices assessed (not Specification rules):

| Choice | Assessment |
|---|---|
| Presence = `exec.LookPath` resolves `codex`/`claude` to an absolute path; a configuration directory alone is absence | Satisfies "no invented availability" and is fully reversible. It gives false negatives when a Runtime is installed but not on the invoking process's `PATH` (for example a native installer's `~/.local/bin` not yet on `PATH`, or a desktop-app-bundled executable). Such a Runtime is reported absent, with `configurationWithoutExecutable` when its directory exists, and `axiom runtime <id> install` configures it explicitly. Finding F7, not blocking. |
| Exit `0` when every detected Runtime converges, including none; `1` when any detected Runtime fails; canonical `success` / `partial` / `failure` / `interrupted` | Each required state is distinguishable: none (`success`, `detected=0`), all configured (`success`), partial (`partial`, exit `1`), all failed (`failure`, exit `1`). No-Runtime success matches "Axiom remains installed and reports that no supported Runtime is available". Consistent with the CLI's canonical completion model. |
| Partial success keeps the converged Runtime and does not roll back | Each Runtime converges under its own lock and ownership rules; the result names each Runtime's state and reason; a rerun is a no-op for the converged Runtime and retries only the failed one (executable matrix `one converges while the other conflicts`). Consistent and idempotent. |
| Claude ownership: shared skill set and existing Codex ownership mechanics, a separate empty history of previously owned Claude revisions, a receipt with `runtime=claude`, the skill root and each skill digest | Absent skills install; current content is a no-op; only a registered previous Claude revision upgrades (none exist); unknown or modified content is preserved and fails Claude; a previous Codex revision found in the Claude root is a conflict (`TestClaudeHasNoInventedHistory`); the receipt never authorizes overwriting changed content (`TestModifiedOwnedSkillIsNotRepairedDespiteReceipt`); a partial prior owned install converges (`TestPartialPriorOwnedInstallConverges`). See findings F6 and F8. |

### Skill root permissions (FR-066/AC-47)

The earlier implementation required every Runtime skill root to be mode `0700`.
Runtimes commonly create their roots `0755`; on the macOS 27 maintainer host
Claude Code's `~/.claude/skills` (and Codex's own `~/.codex/skills`) are
`drwxr-xr-x`, so first-run would fail Claude there with
`claude_skill_root_unavailable`, and Codex likewise whenever `~/.agents/skills`
already exists `0755`. Reproduced: the new executable case with `0755` roots
fails before `9c14299` with both Runtimes `failed` and passes after it.

Threat model ([ADR-0005](../../decisions/0005-bounded-local-filesystem-threat-model.md)):
same-UID actors are out of scope; the relevant adversary is another principal.
The root belongs to the Runtime, not to Axiom state (ADR-0005 property 9 covers
Axiom state, staging, recovery metadata and artifacts), and the five Axiom
skills are public content embedded in the binary, so read access by others is
not a confidentiality loss. What must hold is that no other principal can
**mutate** the root: rename, replace or insert entries (for example swap an
Axiom skill directory or plant a symlink).

| Root mode | Group/other can mutate entries | Result |
|---|---|---|
| `0700` | no | accepted |
| `0750` | no (group read/search only) | accepted |
| `0755` | no (read/search only) | accepted |
| `0705`, other read-only variants | no | accepted |
| `0720`, `0770`, `0775` | yes (group write) | refused |
| `0702`, `0757`, `0777` | yes (other write) | refused |

The rule since `9c14299`: the root must be a real directory (not a symlink,
checked with `Lstat` and re-verified on the opened descriptor), owned by the
current UID, without group or other write bits, and without any extended ACL
(an ACL could grant write; any ACL still fails closed). Everything Axiom
creates under the root keeps the private rule: skill directories `0700`,
`SKILL.md`, receipt and lock `0600`, no ACL, no links. A missing root is still
created `0700`. Path traversal is not reachable (fixed skill names, absolute
cleaned roots). Unsafe ancestors are not checked, before or after this change;
a `0700` root under an ancestor writable by another principal was equally
replaceable, so the change does not widen that pre-existing boundary.

Tests: `TestSkillRootAcceptsModesWithoutGroupOrOtherWrite` (Codex and Claude,
ten modes: install, inspect, idempotent rerun, root mode untouched, owned
entries private, refused roots unwritten), `TestSkillRootRefusesSymlinkedRoot`,
and executable cases `Runtime-created 0755 skill roots are configured` (a user's
own Claude skill in the same root is preserved) and `group-writable skill root
fails that Runtime unchanged`. Existing `0770` and skill-directory `0755`
refusals still pass.

## Collateral defects fixed in PR #106

| Defect | Fix | Proof |
|---|---|---|
| `install-release.sh` used `stat -f … \|\| stat -c …`; GNU `stat -f` prints filesystem status, so every Linux install was refused | syntax chosen by kernel (`e4361fa`) | synthetic Ubuntu row install/upgrade suites; native macOS suites still pass |
| `build-release-archives.sh` built the module of the caller's working directory | builds from its own checkout (`90f1eab`) | `test-release-pipeline.sh` from an unrelated CWD |
| a refused concurrent `install-release.sh` removed the running installer's lock | the lock path is cleared before the refusal exits (`eb88832`) | `test-install-bootstrap.sh` held-lock and two-installer cases |
| lock-free refusals created empty destination directories first | unsafe roots and a binary without receipt are refused before any `mkdir` (`eb88832`) | every refusal case checks an unchanged HOME |
| SemVer comparison parsed numeric identifiers as integers and could overflow | numeric identifiers compared by length then lexically (`9e474e4`) | `internal/install` upgrade tests |
| `test-s7-native.sh` ran the owned-upgrade step before isolating Lingo state and skill roots, so the candidate inspected the operator's real HOME | isolation moved before the first installed binary, HOME isolated too (`7185a1e`) | native macOS 27 run below; the unisolated run was read-only and refused before any effect |

## Validation

### Native macOS 27.0/arm64 host — 2026-09-28 (this reconciliation)

Host: macOS 27.0 (26A428), arm64, APFS, Go 1.26.1. This is the maintainer's
workstation, **not a clean environment**: it has real Codex/Claude installs and
prior mixed POC/v1 Lingo state. All suites pin isolated HOME, state and skill
roots; a listing of the real `~/.claude/skills`, `~/.agents/skills`,
`~/.local/bin` and install state was identical before and after the run.

Commits: suites at `7185a1e` (clean tree, `archive_kind=release`); Go checks
at the final Evidence commit.

| Command | Result |
|---|---|
| `go build ./...`, `go vet ./...`, `gofmt -l .`, `go mod verify` | pass |
| `go test ./... -count=1` | pass |
| `go test -race ./... -count=1` | pass under the default `umask 022` (F10 fails only under `umask 077`) |
| `./scripts/validate-repository.sh .` | pass |
| `./scripts/test-release-pipeline.sh` | pass |
| `./scripts/test-release-archives.sh` | pass, including the native macOS install section and the installed binary's first-run of both (fake) Runtimes |
| `./scripts/test-install-axiom.sh` | pass (a first attempt failed only because the working tree was edited while it ran; the clean rerun passed) |
| `./scripts/test-codex-skills.sh`, `./scripts/dogfood-poc.sh` | pass |
| `./scripts/test-install-bootstrap.sh` (`sh`) | `host_row=macos-27-arm64`: 47 cases pass, the two Ubuntu row-selection cases `not_run`, `result=pass` |
| `./scripts/test-s7-native.sh` | `native_row=macos-27.0-arm64-apfs`: 32 steps pass, including `installer-owned-upgrade`, owned upgrade, stale-digest denial, downgrade refusal, interrupted-upgrade resume and skill publication; only the `race` step fails, on F10 (`TestCoordinationLatestRejectsUnsafeHierarchy` under `umask 077`, pre-existing on `main`) |

### Earlier run — synthetic Ubuntu 26.04/amd64 row

On T40 commit `721acd0`, an Ubuntu 24.04 container with a private mount
namespace replacing `/etc/os-release` (Go 1.26.0) passed `go test -race ./...`
(as an unprivileged user), `go vet`, `gofmt`, `go mod verify`,
`validate-repository.sh`, `test-release-archives.sh`, `test-release-pipeline.sh`,
`test-install-axiom.sh`, `test-codex-skills.sh`, `dogfood-poc.sh`,
`test-install-bootstrap.sh` (48/48 under `dash` and `bash --posix`) and
`test-s7-native.sh`. That row is **synthetic**, not native Ubuntu 26.04.

## Security review

Reviewed for this reconciliation: release provenance (revision, clean state and
version bound in metadata, build information and verifier), archive and
checksum validation (digest before any read, closed entry sets, regular files
only, manifests), path confinement (fixed member and skill names, canonical
absolute directories, no `=` or newlines), symlinks and hard links, ownership,
permissions and ACLs (skill root rule above; installer and Axiom-owned entries
unchanged), TOCTOU (the upgrade re-previews under its own lock and compares the
digest; skill checks re-verify the opened descriptor), network failure
(explicit, no effects), interruption (only the same archive resumes), foreign
installations and skills (preserved), downgrade (refused), selector ambiguity
(strict tags, exclusive selectors, no RC fallback, non-tag redirects refused),
Runtime execution (never), credentials (never read), and test isolation (no
suite reads or writes the operator's real Runtime or Lingo roots). Accepted,
documented risks: integrity relies on HTTPS and GitHub-hosted `SHA256SUMS`
(signing is an S9 non-goal); the candidate binary runs from a private temporary
directory for the owned upgrade, so a `noexec` temporary directory fails the
upgrade before any effect.

## Findings

| ID | Finding | Status |
|---|---|---|
| F1 | Skill root `0700` rule refused ordinary Runtime-created `0755` roots | fixed (`9c14299`) |
| F2 | PR #106 recorded T40 implementation choices and `--channel rc` wording as human decisions in Specification/Plan/Tasks | fixed (`7bb6418`); choices assessed above |
| F3 | Bootstrap usage advertised `--channel stable\|rc` | fixed (`7d78156`) |
| F4 | `test-s7-native.sh` owned-upgrade step not isolated from real HOME | fixed (`7185a1e`) |
| F5 | Ubuntu row-selection cases passed only on Ubuntu hosts and were reported as general Evidence | fixed (`7185a1e`, reported `not_run` elsewhere) |
| F6 | `axiom upgrade` publishes skills only to the Codex root. After a binary upgrade that changes the shared skill text, Claude keeps the previous revision, and the next `first-run` reports `claude_skill_conflict` unless that revision was registered in Claude's history (empty today). | **open**; must be resolved before T23 identifies an RC (register each shipped revision in both histories as part of release preparation, or converge Claude in the upgrade path) |
| F7 | `exec.LookPath` discovery misses Runtimes that are installed but not on the invoking `PATH` | open, low; truthful `absent` plus `configurationWithoutExecutable`; explicit `axiom runtime <id> install` path exists |
| F8 | The Claude receipt binds the absolute skill root. A Claude configuration copied or synced to another path yields `claude_skill_receipt_incomplete` (partial) on every run; removing that receipt lets the next run recreate it when all skills are current | open, low; fail-closed; recovery not yet documented in diagnostics |
| F9 | `install-release.sh` (S7 contract) requires an existing `--bin-dir` to be exactly `0700`; a default `~/.local/bin` created `0755` by other tools refuses the default remote install | **open**; the F1 analysis suggests "no group/other write" suffices for a directory Axiom does not own, but this changes the S7 installer contract and needs a decision before T24 (T24 rows will also carry Runtime installs that may create `~/.local/bin`) |
| F10 | `internal/local` `TestCoordinationLatestRejectsUnsafeHierarchy` (S8, on `main`) fails under `umask 077`, which `test-s7-native.sh` uses | open, pre-existing on `main`, outside PR #106; separate fix proposed |
| F11 | The persistent `.axiom-skill-set.lock` is created in a detected Runtime's root even when that Runtime then fails on a conflict | open, low; conflicting and foreign content is never changed |
| F12 | Release workflow `tag` input is not checked against an existing tag at the dispatched revision | open, low; T23 must bind tag to recorded revision before publication |

## Acceptance status by validation kind

| Kind | Claims |
|---|---|
| **confirmed** (executed deterministic tests) | Go packages; release build, verifier and workflow `run:` replay; bootstrap selection/verification logic; release installer and protected upgrade with local fixtures; T40 four-state matrix with fake Runtime executables; skill root permission matrix |
| **native** (supported row, local fixtures, not a clean environment, not a published release) | macOS 27.0/arm64: install, reinstall no-op, owned upgrade, downgrade refusal, recovery, skills publication and bootstrap cases on this host (results above) |
| **synthetic** | Ubuntu 26.04/amd64 install/upgrade/refusal suites and Ubuntu row selection (Ubuntu 24.04 userland, replaced `/etc/os-release`) |
| **real Runtime** | none. Codex and Claude were never invoked; T40 used fake executables |
| **not run** | Ubuntu 26.04/arm64 installation; any native Ubuntu 26.04 row; the GitHub `Release artifacts` workflow and artifact upload; installation of a published release; FR-067/AC-49 dogfooding |
| **blocked** | T24 native acceptance on every row (needs T23 and a published RC) |
| **future T23/T24/T25** | RC identification and authorized prerelease publication (T23); clean-environment matrix pinning `--version vX.Y.Z-rc.N` on every row, real Codex/Claude/GitHub observation and Axiom dogfooding (T24); versioned RC Evidence and the human gate (T25) |

S9 is not complete, and no MVP acceptance is claimed.

## T24 native acceptance — blocked

| Row | Status | Blocker |
|---|---|---|
| macOS 27.0 / arm64 | blocked | a supported host exists, but T24 needs a clean account/VM and a published RC |
| Ubuntu 26.04 / amd64 | blocked | only a synthetic row was exercised; needs a clean native Ubuntu 26.04 VM/account and a published RC |
| Ubuntu 26.04 / arm64 | blocked | no arm64 Ubuntu host exercised; needs a published RC |

T24 installs one exact published RC with `--version vX.Y.Z-rc.N` on every row
(AC-48), never a floating selector. It requires S8/T36 technical completion,
T23 with explicit publication authority, and resolution of findings F6 and F9.
The S7 Ubuntu 26.04 native rows deferred to T24 remain mandatory. Synthetic and
native-host results above do not satisfy any T24 row.
