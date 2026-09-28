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
| T39 stable remote installer and owned upgrade | implemented; validated with local fixtures; no published release exists | `eb88832`, `c9ec3a3`, `9e474e4`, `7d78156`, `eafaa95` |
| T40 Codex + Claude first-run bootstrap | implemented; validated with fake Runtime executables; real Runtimes never invoked | `721acd0`, `9c14299`, `e127892` |

## Requirement traceability

| Requirement | Implementation | Tests | Evidence kind | Status |
|---|---|---|---|---|
| FR-062 public `axiom` identity | `build-release-archives.sh`, `install-release.sh`, `install-axiom.sh`, `internal/install`, help, diagnostics, skills | `internal/cli` help test, `TestPreAxiomExecutableSkillsAndReceiptRemainUpgradeable`, `test-release-archives.sh`, `test-install-axiom.sh`, `verify-release-artifacts.sh` | confirmed (local deterministic tests; native macOS 27 host; synthetic Ubuntu row) | met for delivered surfaces |
| FR-063 release artifacts from one revision | `release-artifacts.yml`, `release-tag-version.sh`, `verify-release-artifacts.sh` | `test-release-pipeline.sh`, local replay of the workflow `run:` steps | confirmed locally; GitHub workflow **not run** | implemented; GitHub execution unverified |
| FR-064 stable remote installation and selection policy | `scripts/install.sh`; default `~/.local/bin` accepted when only its owner can write it (F9) | `test-install-bootstrap.sh` (fake `curl`, local release fixtures, `--bin-dir` mode matrix) | confirmed on native macOS 27/arm64 host and synthetic Ubuntu 26.04/amd64 row; live GitHub read-only checks | selection/verification logic met; installation of a published release **not run** (none exists) |
| FR-065 convergent reinstall/upgrade | `install-release.sh` handing to the candidate's protected `axiom upgrade` | `test-install-bootstrap.sh`, `test-s7-native.sh`, `internal/install`, `internal/local` publication-directory tests | confirmed with local fixtures (native macOS 27 host, synthetic Ubuntu) | met for local fixtures |
| FR-066 multi-runtime first run | `internal/runtimebootstrap`, `internal/codexruntime` integration descriptor, `cmd/lingo/runtime_bootstrap.go` | `internal/runtimebootstrap`, `internal/codexruntime`, executable matrix `TestExecutableFirstRunRuntimeMatrix`, `dogfood-poc.sh`, `test-release-archives.sh` | confirmed with fake `codex`/`claude` executables and simulated N→N+1 skill releases; **real Runtime not run** | met for the four-state matrix and post-upgrade convergence (F6 fixed); see findings F7, F8 |
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

### Binary directory permissions (finding F9)

**Observed.** `install-release.sh` (inherited from S7) required an existing
`--bin-dir` to be exactly `0700`, and the Go owned upgrade checked the binary
directory with the same owner-only rule (`local.CheckPrivateDirectory` and
`ReadOwnedFile`). A user's `~/.local/bin` is commonly `0755` (created by other
tools or Runtime installers), so the default remote install of FR-064/FR-065
refused with `unsafe destination ownership, permissions, ACL, or type`, and an
owned upgrade into it would have refused with `unsafe_target`
(`test-install-bootstrap.sh` asserted that refusal as
`unsafe-destination-permissions`).

**Cause.** The installer applied the rule for Axiom-owned state to a directory
Axiom publishes into but does not own. [ADR-0005](../../decisions/0005-bounded-local-filesystem-threat-model.md)
property 9 requires restrictive ownership/permissions for machine-local
**state, staging, recovery metadata and artifacts**, and failing closed on
unsafe observed conditions; Plan §13 step 1 requires validating ownership,
type, mode/ACL and ancestry. Same-UID actors are out of scope; for a shared
binary directory the unsafe condition is **mutation by another principal**
(replacing `axiom`, planting entries), not read or search access to a
directory whose only Axiom content is a public, owner-only `0700` executable.
This is the same analysis as the skill root (F1). It corrects the
interpretation of properties ADR-0005/ADR-0007 already require; it is not a new
architectural decision, so no ADR or Specification text changed.

**Fix (`eafaa95`).** For an existing binary directory, both the release
installer (`publication_directory`) and the Go upgrade
(`local.CheckPublicationDirectory`, `local.ReadPublishedFile`) require: a real
directory (not a symlink; the Go path also walks the canonical path through
anchored directory objects and re-verifies the opened directory), owned by the
current UID, without group or other write, and without any extended ACL (any
ACL fails closed, including one granting write). The receipt directory is
Axiom-owned state and keeps the exact owner-only `0700` rule. A missing binary
directory is still created `0700`; staging, the published `axiom` (`0700`), the
receipt (`0600`), link-count, ownership and ACL checks on files are unchanged.

| Existing `--bin-dir` mode | Group/other can mutate entries | Result |
|---|---|---|
| `0700` | no | accepted |
| `0750` | no | accepted |
| `0755` | no | accepted |
| `0702`, `0720` | yes | refused before any effect |
| `0770`, `0775`, `0777` | yes | refused before any effect |
| symlink, foreign owner, extended ACL | — | refused before any effect |

Known asymmetry, fail-closed: on Linux the shell ACL heuristic (`ls -ld` mode
string) also refuses a binary directory carrying setgid/sticky bits, which the
Go check would accept; the installer refuses first.

**Tests.** `test-install-bootstrap.sh`: for `0700`, `0750` and `0755`, clean
install, reinstall no-op (unchanged snapshot), owned upgrade `1.0.0`→`1.1.0`
and downgrade refusal, each asserting the directory mode is untouched and the
binary/receipt/state modes stay `0700`/`0600`/`0700`; a foreign `axiom` in a
`0755` directory is preserved; `0702`, `0720`, `0770`, `0775`, `0777`, a
symlinked directory (existing case), a foreign-owned directory (`/usr/bin`,
`not_run` as root) and a `0755` directory with a mutation ACL (macOS
`chmod +a`; Linux `setfacl` when available, else `not_run`) are refused with
an unchanged HOME. `internal/install`:
`TestUpgradeAcceptsBinaryDirectoryOnlyTheOwnerCanWrite` (apply for each
accepted mode, downgrade refused) and refusal cases for six writable modes, a
symlinked binary directory and a `0755` receipt directory. `internal/local`:
`TestPublicationDirectoryModeMatrix` (ten modes; the private rule still refuses
every non-`0700` mode), symlink/foreign-owner/missing and file-rule tests, and
ACL tests on macOS and Linux. `test-release-archives.sh` keeps its `0770`
binary-directory and ACL refusals.

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
| Claude ownership: shared skill set and existing Codex ownership mechanics, the shared history of skill sets published through the Runtime integration (no Claude-only history), a receipt with `runtime=claude`, the skill root and each skill digest | Absent skills install; current content is a no-op; a skill set published earlier through the shared integration upgrades (F6, below); unknown or modified content is preserved and fails Claude; a pre-shared Codex-only revision found in the Claude root is a conflict (`TestClaudeHasNoInventedHistory`, `TestSharedHistoryDoesNotAdoptCodexOnlyRevisionsForClaude`); the receipt never authorizes overwriting changed content (`TestModifiedOwnedSkillIsNotRepairedDespiteReceipt`); a partial prior owned install converges (`TestPartialPriorOwnedInstallConverges`). See finding F8. |

### Runtime convergence after an upgrade (finding F6)

**Observed.** `axiom upgrade` publishes skill files only to the Codex root.
Each Runtime integration recognized as owned only this binary's skill set and
its own earlier history; Codex had one (`legacySkillDigests`,
`legacyReceiptWires`), Claude's was empty by design. When release N+1 changes
the shared skill text, Codex is upgraded while Claude keeps revision N, which
no Claude history knew, so the next `axiom first-run` failed Claude with
`claude_skill_conflict` and reruns could never converge that Axiom-owned
state. The same gap applied to the Codex skill-set receipt whenever a revision
was not also appended to `legacyReceiptWires`.

**Cause.** Ownership history was per Runtime and maintained by hand, so a
revision published to every Runtime could be registered for Codex only.

**Fix (`e127892`).** One `sharedSkillHistory` (in `internal/codexruntime`)
lists every earlier skill set published through the shared Runtime
integration, each as its skill-set version, binary compatibility and per-skill
digests. Every Runtime installer consults it: `knownDigest` (install,
`InspectUpgrade`, inventory) and `matchesLegacyReceipt`, which derives each
Runtime's exact receipt bytes for those revisions (the Claude receipt still
binds the skill root). The Codex-only pre-shared history is frozen
(`TestRuntimeOnlyHistoryIsFrozen`), and
`TestEveryPublishedSharedRevisionStaysOwned` pins the manifest digest of every
published shared skill set, so changing the skill text without recording the
replaced revision fails the build. The history is empty today: the embedded
skill set (manifest `98c861ec…`) is the first one published to Claude, and no
release exists yet. Ownership is still exact evidence only, never a content
comparison: modified, foreign, ambiguous or unsafe skills are refused,
pre-shared Codex-only revisions are not adopted in a Claude root, and `Install`
now refuses before any change when a skill would be replaced beside a receipt
that is neither absent, this binary's receipt nor an earlier Axiom receipt for
that Runtime and root (previously the skills were replaced first and the
receipt then failed as `partial`). When every skill is already current and only
the receipt is unrecognized, the earlier behavior is kept (F8). Receipt bytes
of the current skill set are byte-identical before and after the change. The
partial `axiom upgrade` next action now names `axiom first-run` (or
`axiom runtime codex install` when Codex is not on `PATH`).

**Tests** (`internal/codexruntime/shared_history_test.go`, simulating the N+1
binary by replacing the embedded skills and appending revision N to the shared
history):

| Case | Result |
|---|---|
| Claude revision N → N+1 → first-run (`TestClaudeRevisionNConvergesAfterUpgradeToNPlusOne`) | inventory `upgradable`/receipt `legacy`; converges to N+1, receipt refreshed, `Ready`; rerun `unchanged` with an identical tree |
| Claude revision N modified by the user, replaced by foreign content, with an extra file, or `0644` → N+1 (`TestModifiedClaudeRevisionNIsRefusedAfterUpgrade`) | `claude_skill_conflict`; tree byte- and mode-identical |
| Codex + Claude at N; `PublishSkill` (the `axiom upgrade` primitive, renamed from `PublishUpgradeSkill` by F13) moves Codex to N+1 leaving its receipt at N; then first-run (`TestCodexAndClaudeConvergeTogetherAfterUpgrade`) | both converge to N+1 with refreshed receipts; rerun idempotent |
| Partial Claude state: no receipt, one skill missing, one already N+1, the rest N (`TestPartialClaudeStateConvergesAfterUpgrade`) | converges; rerun `unchanged` |
| Old valid receipt (Claude N for this root) | accepted and replaced (first case) |
| Invalid receipt: Claude N receipt of another root, the Codex receipt, a foreign receipt, a `0644` receipt (`TestClaudeReceiptMustBeAxiomEvidenceForThisRoot`) | `claude_skill_conflict`; skills and receipt unchanged |
| Pre-shared Codex-only revision in a Claude root after N+1 (`TestSharedHistoryDoesNotAdoptCodexOnlyRevisionsForClaude`) | conflict; content unchanged |

**Limit.** The N+1 binary is simulated inside the test process; no second real
release binary with different skill text exists, and `axiom upgrade` itself
was exercised for Codex by `internal/install` and `test-s7-native.sh`, not
chained with a real Claude root.

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

## Filesystem object identity (finding F13, final review)

**Finding.** Validating a directory by pathname (ownership, mode, ACL,
ancestry) and then mutating it in a SEPARATE step by re-deriving the same
pathname leaves a window in which the two operations can land on different
filesystem objects if the directory, or one of its ancestors, is replaced in
between. This is distinct from F9 (which established the accept/refuse
policy for a directory Axiom does not own) and from the ancestor-checking
gap F9's own Evidence had already flagged as residual ("Unsafe ancestors are
not checked, before or after this change"). It is ADR-0005 property 3
("controlled ancestor/leaf replacement... must be detected and rejected"),
which the implementation had not yet closed for either the new publication
root (T39) or the Runtime skill root (T40).

**Exploit scenario.** A shared, multi-principal parent directory:

```
/shared/                 (mutable by another principal)
└── user-bin/            mode 0755, owner correct
```

`CheckPublicationDirectory("/shared/user-bin")` validated the leaf and its
ancestors were never checked; the actual publication then reopened
`/shared/user-bin` by pathname to stage and rename the binary. If `/shared`
let another principal replace the `user-bin` entry, or if a component were
swapped between the read-only validation and the later reopen, the
publication could target an object different from the one that was
validated. The same shape existed for the Runtime skill root (`skillRootDirectory`
validated by a single `Lstat`+`Open` on the full path, with no per-component
walk and no ancestor check at all) and for `internal/install`'s binary and
receipt directories, whose `Apply()` validated once (inside `preview()`) and
mutated later through fresh `os.OpenFile`/`os.Rename` calls by pathname.

**Cause.** Two related gaps:
1. `anchoredRoot` (`internal/local`) validated each path component was a
   real, non-symlinked, identity-consistent directory, but never evaluated
   whether a CONTAINER ancestor's own ownership/mode let another principal
   replace the next component. `internal/codexruntime`'s directory checks
   were weaker still: a single `os.Lstat`+`os.OpenFile` on the full path,
   with no per-component walk.
2. Even with ancestors checked, `CheckPublicationDirectory`/`CheckPrivateDirectory`
   (and `codexruntime`'s equivalents) opened an anchored root, validated it,
   and then CLOSED it, returning only a boolean. Callers that then needed to
   mutate reopened the same pathname independently — `internal/install`'s
   `publishEffect` via `os.OpenFile(stage,...)`/`os.Rename(...)`, and
   `codexruntime`'s `installOne`/`PublishUpgradeSkill`/`publishReceipt` via
   fresh `os.OpenRoot`/`os.Lstat` calls, some of them more than once per
   skill. The validated object and the mutated object were never
   provably the same object.

**Contract violated.** ADR-0005 property 3 (controlled ancestor/leaf
replacement must be detected and rejected) and, derivatively, property 9
(restrictive ownership/permissions for Axiom-owned state) once a mutation
could land somewhere other than the validated, owned location.

**Fix.**

*Ancestor safety* (`b540b61`, `2c7948e`): `internal/local`'s
`anchoredRoot` and a mirrored `anchoredRoot` added to `internal/codexruntime`
(which previously had no per-component walk at all, and needed a
`trustedCanonical` step added too, matching `internal/local`'s handling of
trusted root-owned symlinks such as macOS's `/var` -> `/private/var`) now
check every container from `/` down to (but not including) the final
target: `ancestorSafe` requires the container be owned by root or the
current user, and disallow group/other write unless the sticky bit
restricts removal/rename of existing entries to each entry's own owner. A
container owned by neither root nor the current user is never trusted,
sticky or not, since its owner already has unilateral control over what it
contains. Ordinary system directories (`/`, `/Users`, `/home`, a `0755`
`$HOME`) pass because they are not group/other-writable; `/tmp`-style
`1777` directories pass because of the sticky bit, not because of who owns
them; a hypothetical root-owned `0777` directory WITHOUT sticky is refused,
since without sticky any principal could replace an entry inside it
regardless of the container's owner.

*Object-identity binding* (`b540b61`, `03d30a4`, `2c7948e`, the primary fix): `internal/local` gained
`AnchoredDirectory`, an open, validated handle to one directory whose
`ReadFile`/`Stage`/`Rename`/`Remove`/`Mkdir`/`CreateExclusive`/`Sync`
methods resolve against the directory object the handle was opened
against — never by re-deriving its pathname — via Go's `os.Root`, which
binds later operations to the opened directory's file descriptor
(`*at` syscalls), immune to a later rename, unlink-and-recreate, or
symlink swap at that pathname. `CheckPrivateDirectory`/`CheckPublicationDirectory`/
`ReadOwnedFile`/`ReadPublishedFile` are now thin, one-shot wrappers over it
(no duplicated logic). `internal/install`'s `Apply()` opens `ReceiptDir`
once (`local.OpenOwnedDirectory`) and uses that same handle for the install
lock, every marker write/removal, and the receipt file publication for the
rest of the call; `BinaryDir` is opened once, lazily
(`local.OpenPublicationDirectory`), and reused for the binary effect and any
binary-directory leftover cleanup. `publishEffect` and `writeMarker` now take
an already-opened `AnchoredDirectory` and do their whole stage/reread/rename/confirm
sequence through it. `internal/codexruntime` mirrors this for the skill
root: `Service.Install` opens the skill root once (`ensureRoot` now returns
the opened, anchored `*os.Root`) and threads it through `installableOne`/
`installOne`/`publishReceiptIn`; a new `UpgradeSession` (returned by
`LockForUpgrade`, replacing a bare unlock closure) holds the anchored skill
root for the whole upgrade, and `PublishSkill` (renamed from
`PublishUpgradeSkill`) and the new `RemoveSkillLeftover` open each skill's
own subdirectory once (`privateChild`) and do the reread/stage/rename/confirm
sequence through that same child object.

*Detect and reject, not tolerate* (re-review of F13): binding to the object
confined the mutation but still let an operation finish in the displaced
original object and report success while the authorized pathname showed a
different object — a confined but untruthful commit (ADR-0005 property 3
requires changed directory identity and controlled ancestor/leaf replacement
to be detected and rejected; ADR-0007 invariant 7 forbids confirming an effect
whose canonical object is no longer the authorized one). Each anchored handle
now also records the identity of every directory on its canonical pathname at
open time (`anchor` in `internal/local` and, mirrored, `internal/codexruntime`),
and `StillAtPath`/`anchor.verify` re-walks that pathname from `/` without
following symlinks, requiring every component to still be the recorded object
and to end at the open handle. The proof is taken at the declared inspection and
commit boundaries:

- `local.AnchoredDirectory`'s `CreateExclusive`, `Mkdir`, `Stage` and `Rename`
  refuse with `local.ErrReplaced` (an `ErrUnsafe`) before acting; `Remove`
  stays unguarded because it only discards the handle's own stage, lock or
  marker in the anchored object.
- `internal/install`: `publishEffect` reports a replacement before the rename
  as `target_changed` (nothing committed) and one seen when confirming after
  the rename as `publication_uncertain` (not a confirmed effect); marker
  writes confirm at the pathname; `Apply` proves `ReceiptDir` and `BinaryDir`
  again before clearing the operation marker and before declaring success,
  otherwise `partial` with `final_verification_failed` and the marker kept.
- `internal/codexruntime`: `Install` and `LockForUpgrade` prove the skill root
  before and after taking the lock; each skill create/replace and the receipt
  stage/rename prove the root and the skill directory (`childStillAt`) before
  the commit and again after it. With nothing changed yet the result is
  `<runtime>_skill_root_unavailable`; after a change, `partial`
  (`skill_install_partial` / `skill_receipt_incomplete`), never `applied`.
  `PublishSkill` returns `ErrTargetReplaced` before a commit and a
  non-confirmed error after one.

The remaining window between a proof and its syscall is the arbitrary same-UID
interleaving ADR-0005 explicitly excludes; the proof is not a lock.

**Test seam.** No simulated seam was needed: Go's `os.Root` is itself the
capability, so the tests physically replace the validated directory (rename
it away and put a new directory or a symlink in its place; replace an
ancestor; replace an ancestor and move the original leaf back under it)
between opening the `AnchoredDirectory`/`UpgradeSession` (or between two
effects, through the existing `afterEffect`/`afterSkill` hooks) and the
later mutation, and assert the operation is refused, the displaced original
object gains no committed effect, the replacement stays intact, and no
success is reported:

| Layer | Test | Result |
|---|---|---|
| `internal/local` | `TestAnchoredDirectoryRefusesMutationAfterReplacement` (publication and owned handles × leaf replaced by a directory, by a symlink; parent replaced; parent replaced with the original leaf moved back under it) | `StillAtPath`, `Stage`, `CreateExclusive`, `Mkdir` refuse with `ErrReplaced`; neither the original nor the replacement gains an entry |
| `internal/local` | `TestAnchoredDirectoryRefusesCommitAfterReplacement` | a stage prepared before the replacement is not renamed into either object; the handle still removes its own stage |
| `internal/local` | `TestAnchoredDirectoryAcceptsSameObjectRestoredAtPath` | moving the same object away and back is not a replacement |
| `internal/local` | `TestAncestorSafeOwnershipAndStickyMatrix`, `TestPublicationDirectoryRefusesUnsafeAncestorRealFilesystem`, `TestPublicationDirectoryRefusesUnsafeGrandparent` | root/self-owned, no group/other write, or sticky → accepted; group/other-writable without sticky, or foreign-owned → refused, including two levels up |
| `internal/install` | `TestPublishEffectRefusesReplacedDirectory` (root replaced by a directory holding a foreign binary, by a symlink; ancestor replaced) | `target_changed`; the original keeps its prior binary and no stage; the replacement is byte-for-byte intact |
| `internal/install` | `TestPublishEffectRefusesWhenTargetFileChangedAfterOpen` | the pre-existing expected-revision check still refuses a raced target-file change; stage cleaned up |
| `internal/install` | `TestApplyRefusesReceiptDirectoryReplacedMidOperation` | `ReceiptDir` replaced after the binary commit: `partial`, `target_changed`, only the binary confirmed; the original receipt is unchanged and the replacement stays empty |
| `internal/install` | `TestApplyDoesNotDeclareSuccessAfterBinaryDirectoryReplaced` | `BinaryDir` replaced after every effect: `partial`, `final_verification_failed`, never `success`; the operation marker is kept |
| `internal/codexruntime` | `TestUpgradeSessionRefusesReplacedRoot` (root replaced by a directory, by a symlink; ancestor replaced) | `ErrTargetReplaced`; no skill in the original root; the replacement stays empty |
| `internal/codexruntime` | `TestInstallRefusesRootReplacedBeforeAnyChange` / `TestInstallIsPartialWhenRootReplacedAfterAChange` | `codex_skill_root_unavailable` with nothing changed, `codex_skill_install_partial` after a change; the install never completes in the original root, no receipt anywhere, the replacement stays empty |
| `internal/codexruntime` | `TestInstallRefusesForeignSkillUntouchedDespiteRootReplacement` | a foreign skill directory is refused, left byte-for-byte unchanged; no receipt published |
| `internal/codexruntime` | `TestPublishSkillRefusesSkillDirectoryReplacedBySymlink` | a skill directory replaced by a symlink between lock and publish is refused, not followed |

**Shell (`scripts/install-release.sh`).** Bash cannot hold an anchored file
descriptor the way `os.Root` does, so the fix there is the ancestor-safety
check alone (`ancestor_safe`/`ancestors_safe`, mirroring the Go rule exactly,
including the sticky-bit case via `find -perm -1000` since `stat -f %Lp`
omits it on macOS), applied to every container of `--bin-dir` and
`--receipt-dir` before any effect and again immediately before each is
created/used in `prepare_directory`. This narrows the window to the smallest
the script's existing structure allows but does not close it the way the Go
paths do; existing symlink rejection (`safe_components`), the receipt lock,
and re-validation immediately before mutation are unchanged. This residual
gap is accepted and documented, not hidden (`8fffc1a`). `0700`/`0750`/`0755` for
`--bin-dir` itself are still accepted and `0702`/`0720`/`0770`/`0775`/`0777`
still refused (F9 unchanged); new cases cover an unsafe non-sticky ancestor
and grandparent (refused, zero effect), an ordinary `0755` ancestor
(accepted), and a `1777` sticky ancestor mimicking `/tmp` (accepted).

**macOS/Linux.** All Go tests above ran natively on macOS 27.0/arm64,
including under `go test -race` with the default umask and with `umask 077`.
The shell ancestor cases ran under `sh` and `bash --posix` on the same host.
CI additionally exercises the full Go suite on `ubuntu-24.04`; the new shell
ancestor cases were not re-run on a native or synthetic Ubuntu 26.04 row in
this pass (same limitation already recorded for F6/F9).

**State.** Fixed. No ADR or Specification changed: this closes a gap in the
ALREADY-required properties (ADR-0005 property 3), it does not add a new
threat-model commitment.

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

### Native macOS 27.0/arm64 host — 2026-09-28 (F6/F9 final review)

Same host and isolation as the run below (maintainer workstation, **not a
clean environment**; isolated HOME, state and skill roots). All suites ran on
the clean tree at `420d642` (`e127892` F6, `eafaa95` F9, `420d642` docs;
`archive_kind=release`); this Evidence commit changes documentation only.

| Command | Result |
|---|---|
| `go build ./...`, `go vet ./...`, `gofmt -l .` (empty), `go mod verify` | pass |
| `go test ./... -count=1` | pass |
| `go test -race ./... -count=1` (default `umask 022`) | pass |
| `go test -race ./... -count=1` under `umask 077` | only F10 fails (`TestCoordinationLatestRejectsUnsafeHierarchy`, pre-existing on `main`); a new F9 test that first depended on `umask` was fixed before `eafaa95` was final |
| `./scripts/validate-repository.sh .`, `./scripts/check-sensitive-files.sh .` | pass |
| `./scripts/test-install-bootstrap.sh` under `sh` and under `bash --posix` | each `host_row=macos-27-arm64`: 56 cases pass, including the eleven `--bin-dir` cases (`0700`/`0750`/`0755` lifecycle, foreign target in `0755`, five writable modes, foreign owner, mutation ACL); the two Ubuntu row-selection cases `not_run` |
| `./scripts/test-install-axiom.sh`, `./scripts/test-release-archives.sh`, `./scripts/test-release-pipeline.sh`, `./scripts/test-codex-skills.sh`, `./scripts/test-s7-security.sh`, `./scripts/dogfood-poc.sh` | pass |
| `./scripts/test-s7-native.sh` | `native_row=macos-27.0-arm64-apfs`, `source_state=clean`: 32 steps pass (install, no-op, installer owned upgrade, owned upgrade, stale-digest denial, downgrade refusal, interrupted-upgrade resume, skills); only `race` fails, on F10 under the suite's `umask 077` |
| `gitleaks git --log-opts=origin/main..HEAD` | 17 commits, no leaks |
| `git diff --check origin/main...HEAD` | clean |

Not run in this round: any Ubuntu row (synthetic or native) for the F6/F9
changes, a real Codex or Claude, a published release, the GitHub workflow.

### Native macOS 27.0/arm64 host — 2026-09-28 (earlier reconciliation)

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
permissions and ACLs (skill root and binary directory rules above: only
directories Axiom does not own accept read/search by others, never write or
ACLs; the receipt directory and every Axiom-created file keep the owner-only
rule), skill ownership after upgrades (exact shared digests and receipts only;
no skill replaced beside an unrecognized receipt), TOCTOU (the upgrade re-previews under its own lock and compares the
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
| F6 | `axiom upgrade` publishes skills only to the Codex root. After a binary upgrade that changes the shared skill text, Claude kept the previous revision, and the next `first-run` reported `claude_skill_conflict` because only Codex had an ownership history. | fixed (`e127892`): shared revision history known to every Runtime installer, frozen Codex-only history, pinned published revisions; see "Runtime convergence after an upgrade" |
| F7 | `exec.LookPath` discovery misses Runtimes that are installed but not on the invoking `PATH` | open, low; truthful `absent` plus `configurationWithoutExecutable`; explicit `axiom runtime <id> install` path exists |
| F8 | The Claude receipt binds the absolute skill root. A Claude configuration copied or synced to another path yields `claude_skill_receipt_incomplete` (partial) on every run; removing that receipt lets the next run recreate it when all skills are current | open, low; fail-closed; recovery not yet documented in diagnostics |
| F9 | `install-release.sh` (S7) and the Go owned upgrade required an existing `--bin-dir` to be exactly `0700`; a default `~/.local/bin` created `0755` by other tools refused the default remote install | fixed (`eafaa95`): assessed against ADR-0005 as a corrected interpretation (mutation by another principal is the unsafe condition), no ADR/Specification change; receipt directory and Axiom-created files unchanged; see "Binary directory permissions" |
| F10 | `internal/local` `TestCoordinationLatestRejectsUnsafeHierarchy` (S8, on `main`) fails under `umask 077`, which `test-s7-native.sh` uses | open, pre-existing on `main`, outside PR #106; separate fix proposed |
| F11 | The persistent `.axiom-skill-set.lock` is created in a detected Runtime's root even when that Runtime then fails on a conflict | open, low; conflicting and foreign content is never changed |
| F12 | Release workflow `tag` input is not checked against an existing tag at the dispatched revision | open, low; T23 must bind tag to recorded revision before publication |
| F13 | Major, final review: a directory was validated by pathname and later mutated by re-deriving the same pathname, and ancestors were never checked for mutation by another principal (ADR-0005 property 3) — the T39 publication root, `internal/install`'s binary/receipt directories, and the T40 Runtime skill root all shared this shape | fixed: ancestor-safety checks added throughout, object-identity bound via `local.AnchoredDirectory` and `codexruntime.UpgradeSession`/anchored skill root, and a replacement at the authorized pathname detected and rejected at every inspection/commit boundary instead of being tolerated; see "Filesystem object identity" |

## Acceptance status by validation kind

| Kind | Claims |
|---|---|
| **confirmed** (executed deterministic tests) | Go packages; release build, verifier and workflow `run:` replay; bootstrap selection/verification logic; release installer and protected upgrade with local fixtures; T40 four-state matrix with fake Runtime executables; skill root permission matrix; F6 post-upgrade convergence with a simulated N+1 skill set (in-process); F9 binary-directory matrix; F13 object-identity detect-and-reject replacement tests (real filesystem replacement against `os.Root` handles, not simulated) at the `internal/local`, `internal/install`, and `internal/codexruntime` layers, plus the shell ancestor-safety matrix |
| **native** (supported row, local fixtures, not a clean environment, not a published release) | macOS 27.0/arm64 local-fixture validation: install, reinstall no-op, owned upgrade, downgrade refusal, recovery, skills publication, bootstrap cases and the `0700`/`0750`/`0755` binary-directory lifecycle on this host (results above). macOS 27 clean-environment published-RC acceptance is **not run** (T24) |
| **synthetic** | Ubuntu 26.04/amd64 install/upgrade/refusal suites and Ubuntu row selection (Ubuntu 24.04 userland, replaced `/etc/os-release`), on earlier commits only; the F6/F9 changes were **not** re-run on a synthetic or native Ubuntu row (CI runs the Go suites on `ubuntu-24.04`) |
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
T23 with explicit publication authority. Findings F6, F9 and F13, previously
listed here (or identified in final review) as prerequisites, are fixed.
The S7 Ubuntu 26.04 native rows deferred to T24 remain mandatory. Synthetic and
native-host results above do not satisfy any T24 row.
