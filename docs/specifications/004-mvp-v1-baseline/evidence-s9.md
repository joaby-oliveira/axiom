# Evidence — MVP Slice S9: productization T37–T39 (local)

## Claim and authority boundary

This record describes local implementation of Specification 004 Slice S9 Tasks
T37, T38 and T39, tracked by [Issue #81](https://github.com/rgomids/axiom/issues/81).
The scope reference is the S9 amendment proposed in
[PR #104](https://github.com/rgomids/axiom/pull/104) (FR-062–FR-065,
AC-44–AC-46). PR #104 is still open and unmerged; its head `67bade8` was merged
locally into `integration/s9-productization` (`c75d832`) so this Evidence and
the Task status are reconciled against it. The operator
explicitly authorized T37–T39 implementation while S8/T36 is completed in
parallel and moved the S8/T36 gate from "start T37" to "enter RC/acceptance".

That authority covers local code, tests, documentation, deterministic validation
and this Evidence. It does not cover T40, T23–T25, push, pull requests, merge,
tags, GitHub Releases or prereleases, deploy, credential or secret changes,
Runtime installation, or acceptance of S9 or the MVP. None of those happened.
No release was published, so the remote bootstrap has not installed any real
published Axiom release; T24 owns native clean-environment acceptance.

| Task | State | Commits |
|---|---|---|
| T37 public `axiom` CLI and distribution identity | technically complete locally | `096acb0`, `e4361fa` |
| T38 automated release artifact pipeline | technically complete locally; workflow not run on GitHub | `90f1eab` |
| T39 stable remote installer and owned upgrade | technically complete locally; release candidates exact-version only by human decision | `eb88832` plus the RC-decision and row-selection reconciliation on the integration branch |

The integration branch is based on `main` at
`d3e0a7a8ebe038b3b801bbcf339486a859da5d5a` (unchanged since T37 started).

## T37 — public `axiom` executable

- Release archives, `MANIFEST.sha256`, `install-release.sh`, the Go owned
  upgrade (`internal/install`) and `install-axiom.sh` build and publish the
  executable as `axiom`. Receipt destination, staging names
  (`.axiom-binary-stage.*`, `.axiom-source-stage.*`) and the PATH notice follow.
- Help, recovery/compatibility/upgrade diagnostics and the five Codex skills
  invoke `axiom`. The previous skill digests and skill-set receipt stay known,
  so an installed pre-T37 skill set upgrades.
- Unchanged: `cmd/lingo`, internal Lingo packages, `LINGO_*` variables, state
  roots and internal staging prefixes. A `lingo` executable is never removed.
  A pre-`axiom` release archive or receipt is not recognized as owned and is
  preserved; there is no migration from such an installation.
- Fixes found while exercising the Ubuntu row: `install-release.sh` read owner,
  mode and links with `stat -f … || stat -c …`, which on GNU coreutils prints
  filesystem status and refused every Linux install; and
  `build-release-archives.sh` built from the caller's working directory.

Acceptance observations (clean commit `e4361fa`, synthetic Ubuntu 26.04/amd64
row, see [limits](#limits-and-open-decisions)): every archive lists
`-rwx------ axiom-0.1.0-rc.1-<row>/axiom` and no `lingo`;
`install-release.sh` printed `install_status=installed` and wrote
`destination=<bin>/axiom`, `version=0.1.0-rc.1`, `revision=e4361fac5944`,
`sourceState=clean`, `release=true`; in `env -i … bash --noprofile --norc`,
`type -t axiom` is `file` and `axiom --json version` reports
`{"product":"Axiom","version":"0.1.0-rc.1","revision":"e4361fac5944","sourceState":"clean"}`.
Tests: `internal/cli` help test, `internal/codexruntime`
`TestPreAxiomExecutableSkillsAndReceiptRemainUpgradeable`, `internal/install`
pre-`axiom` archive and receipt refusals, `test-install-axiom.sh` (no alias,
legacy `lingo` preserved), `test-release-archives.sh` (archive entries, mode,
clean-clone release provenance).

## T38 — release artifact pipeline

- `scripts/release-tag-version.sh` maps `vX.Y.Z` / `vX.Y.Z-rc.N` to metadata
  version `X.Y.Z[-rc.N]`; metadata keeps SemVer without `v`.
- `.github/workflows/release-artifacts.yml` (manual dispatch, one `tag` input):
  `contents: read` only, actions pinned by commit, `persist-credentials: false`,
  no Go cache, tag passed only through the environment; requires HEAD to equal
  `GITHUB_SHA` and a clean tree; runs `go mod verify`, the existing
  `build-release-archives.sh`, and `verify-release-artifacts.sh`; retains
  `artifacts/` and `release-evidence.txt` as a workflow artifact. No tag,
  release, prerelease, `latest` or repository effect.
- `scripts/verify-release-artifacts.sh` checks the closed file set, checksums,
  closed bundle entries with a `0700` `axiom`, `MANIFEST.sha256`, exact
  metadata, source-identical `LICENSE`/`install.sh`/skills, per-row executable
  format, embedded build information (`vcs.revision`, `vcs.modified=false`,
  `GOOS`/`GOARCH`, `CGO_ENABLED=0`, `-trimpath`) and host-row provenance.

Observations: `test-release-pipeline.sh` passed (tag vectors, dirty source
refused, full three-row matrix, rerun with identical executables and bundle
manifests, missing row, missing checksum line, checksum mismatch, extra file,
symlinked archive, wrong version or revision, `lingo` bundle, development
build, workflow no-publication checks). A local replay of the workflow `run:`
steps with `bash --noprofile --norc -eo pipefail` passed for `v0.1.0-rc.1` and
`v0.1.0` and refused an injected tag, a `GITHUB_SHA` mismatch and a dirty tree.
The workflow itself, including `actions/upload-artifact`, has not run on GitHub.

## T39 — remote installer and protected owned upgrade

- `scripts/install.sh` (POSIX `sh`): default and `--channel stable` resolve the
  latest stable release from the `Location` of `/releases/latest` only;
  `--version` resolves exactly one tag; `--channel` and `--version` are
  mutually exclusive. Release candidates are exact-version only (human
  decision 2026-09-28): `--channel rc` fails before any request and names
  `--version vX.Y.Z-rc.N`; there is no newest-RC discovery, HTML scraping, API
  JSON parsing, parser dependency or channel index in S9. Exact host row before any download; HTTPS-only
  downloads; archive digest verified against `SHA256SUMS` before reading it;
  bundle `install.sh` and `release-metadata.txt` must match the bundle manifest
  and the resolved version/row of a clean release build; then the bundle's
  release installer runs. Prints tag, asset, asset SHA-256, row, revision and
  receipt. No `sudo`, profile or `PATH` edits, Runtime installation or
  credential access. Dependencies: `sh`, `bash`, `curl`, `tar`, `awk`, `grep`,
  `mktemp`, `sha256sum` or `shasum`.
- `install-release.sh`: a different binary in an owned installation is handed
  to the verified candidate's `axiom upgrade` (preview, exact digest, apply),
  so version order (`downgrade_refused`, `divergent_equivalent_version`), state
  compatibility, space, skills and resume stay in the T20 protected path. Only
  an interrupted owned upgrade of the same archive resumes. Lock-free refusals
  are now decided before any directory is created, and a refused concurrent
  installer no longer removes the other installer's lock.

`scripts/test-install-bootstrap.sh` (fake `curl`, local release fixtures, clean
release builds) passed all 48 cases under `dash` and `bash --posix` on the
synthetic Ubuntu 26.04/amd64 row: conflict (both orders), eight invalid
versions, invalid channel, duplicate and missing values, `--channel rc`
refusal naming `--version`, unsafe directory input, unsupported OS and
architecture, row selection (with `uname`/`sw_vers` shims, each of macOS
27/arm64, Ubuntu 26.04/amd64 and Ubuntu 26.04/arm64 requests exactly its own
asset and nothing else), exact stable, same-version no-op, default stable owned upgrade, explicit stable,
downgrade refusal, exact RC, stable below installed RC refused, no stable
release, RC behind `latest`, unpublished version, missing asset, missing
checksum, checksum mismatch, mislabeled asset, development build, three network
failures, foreign target, legacy `lingo` untouched, modified binary, invalid
receipt, unsafe permissions, symlinked destination, held lock, two concurrent
installers, interrupted upgrade resume, and interrupted first install. Every
refusal was checked for an unchanged HOME tree and an empty temporary directory.
On this unsupported native host the selector/host tier passes and the suite
exits `78` (blocked).

Live read-only checks against GitHub (synthetic row, no release published):
the default selector reported that no stable release exists (the real
redirect targets `/releases`), `--version v0.1.0-rc.1` reported no published
`SHA256SUMS` (HTTP 404), and `--channel rc` and the conflict were refused before
any request. HOME stayed empty.

## Validation

Run on the integration branch after the PR #104 reconciliation and RC
decision (the commit recording this line), Ubuntu 24.04 container, Go 1.26.0,
as root unless stated:

| Command | Result |
|---|---|
| `go test -race ./...` | pass as an unprivileged user; as root only `internal/local` `TestPortableStoreRejectsUserSymlinkAncestor` fails, identically on unmodified `main` (a root-owned symlink counts as system-owned) |
| `go vet ./...`, `go build ./...`, `go mod verify`, `gofmt -l .` | pass |
| `./scripts/validate-repository.sh .` | pass |
| `./scripts/test-release-archives.sh` | pass (native install section also passes on the synthetic row) |
| `./scripts/test-release-pipeline.sh` | pass |
| `./scripts/test-install-axiom.sh`, `./scripts/test-codex-skills.sh`, `./scripts/dogfood-poc.sh` | pass |
| `./scripts/test-install-bootstrap.sh` | selector/host tier pass, then `78` blocked natively; 48/48 on the synthetic row with `dash` and `bash --posix` |
| `./scripts/test-s7-native.sh` (synthetic row) | every install and upgrade step passes, including `installer-owned-upgrade`; only the root-only test above fails |

## Security review

Reviewed: release provenance (revision, clean state and version bound in
metadata, build information and verifier), archive and checksum validation
(digest before any read, closed entry sets, regular files only, manifests),
path confinement (fixed member names, canonical absolute directories, no `=`
or newlines), symlinks and hard links (existing installer checks unchanged;
symlinked destinations refused before creation), ownership, permissions and
ACLs (unchanged), TOCTOU (the upgrade re-previews under its own lock and
compares the digest), network failure (explicit, no effects), interruption
(only the same archive resumes), foreign installations (preserved, including
`lingo`), downgrade (refused), and version/channel ambiguity (strict tags,
exclusive selectors, no RC fallback, non-tag redirects refused). Two
pre-existing defects were fixed (GNU `stat`, lock removal on refusal). No
blocking finding remains. Accepted, documented risks: integrity relies on
HTTPS and GitHub-hosted `SHA256SUMS` (no signing, an S9 non-goal); the
candidate binary runs from a private temporary directory to perform the owned
upgrade, so a `noexec` temporary directory fails the upgrade before any effect.

## Limits and open decisions

- **Decided — release candidates are exact-version only** (human decision
  2026-09-28). `--channel stable` stays on `/releases/latest`; RCs install only
  with `--version vX.Y.Z-rc.N`; `--channel rc` fails with that explanation. No
  newest-RC discovery, HTML scraping, API JSON parsing, parser dependency or
  channel index in S9; a channel index may be designed separately after the
  MVP.
- Ubuntu Evidence here is a **synthetic** row: a private mount namespace with a
  replaced `/etc/os-release` on an Ubuntu 24.04 kernel and userland. It is not
  native acceptance, and macOS 27 was not exercised. T24 remains required.
- The release workflow has not run on GitHub; the artifact upload step is
  unexercised.
- Archives are content-identical on rerun but not byte-identical (tar
  timestamps); the published `SHA256SUMS` is the one from the published run.
- A default `~/.local/bin` that already exists with broader permissions than
  `0700` is refused; pass `--bin-dir`.
- The source installer and the release installer both target
  `~/.local/bin/axiom` by default and refuse each other's binary.
- Not done: T40, T23–T25, publication, S9 completion, MVP acceptance.

## Acceptance status by validation kind

| Claim | Kind |
|---|---|
| Go packages, release build, verifier, workflow `run:` replay, installer and bootstrap logic | confirmed by executed deterministic tests in this container |
| Install, no-op, upgrade, downgrade, recovery and refusal on Ubuntu 26.04/amd64 | **synthetic** row only (Ubuntu 24.04 userland, replaced `/etc/os-release`) |
| Ubuntu 26.04/arm64 and macOS 27/arm64 | asset selection only (shimmed); installation **untested** |
| `/releases/latest` behavior with no stable release; unpublished RC `SHA256SUMS` 404 | confirmed by live read-only requests |
| Release workflow on GitHub Actions, artifact upload | **untested** (no dispatch authority) |
| Native Ubuntu 26.04 amd64/arm64 and macOS 27 acceptance | **blocked** (T24; see below) |

## T24 native acceptance — blocked

T24 is not executable yet and no row is accepted:

| Row | Status | Blocker |
|---|---|---|
| macOS 27.0 / arm64 | blocked | no macOS 27 host available here; needs a published RC |
| Ubuntu 26.04 / amd64 | blocked | only a synthetic row exists here; needs a clean native Ubuntu 26.04 VM/account and a published RC |
| Ubuntu 26.04 / arm64 | blocked | no arm64 host available here; needs a published RC |

Per the versioned DAG (T39 -> T40 -> T23 -> T24), T24 installs one exact
published RC with `--version vX.Y.Z-rc.N` on every row. That requires T40,
S8/T36 technical completion (the gate for entering RC/acceptance), and T23 with
explicit publication authority. The S7 Ubuntu 26.04 native rows deferred to T24
remain mandatory. Synthetic results above do not satisfy any T24 row.

## Next S9 task — T40 needs a decision

T40 (Codex + Claude first-run bootstrap) is the next executable Task in the
DAG and was not started. Today `axiom first-run` is a read-only alias of
`runtime codex status`, and no Runtime discovery exists (S8 Runtime
observations are supplied by configuration). Before implementation, these
contract points need a human decision; the Issue #81 review already listed the
"exact Claude product-integration install/status contract" as open, and PR #104
fixed only the Claude skill root:

1. the discovery signal for "Runtime present" (executable on `PATH`, the
   Runtime's user configuration root, or both);
2. whether `first-run` itself writes Runtime integrations (FR-066 wording) or
   stays a preview that `runtime <id> install` applies, and its terminal status
   and exit code for Codex-only, Claude-only, both and neither;
3. the Claude integration ownership contract: reuse of the five thin skills
   (currently Runtime-neutral except one "Codex's current working directory"
   phrase), a skill-set receipt and known-digest upgrade set like Codex, and
   conflict handling under `~/.claude/skills` or `CLAUDE_CONFIG_DIR/skills`.

The unmerged S8 branch `agent/t36-real-runtime-evidence` (no pull request) adds
`runtime profile validate` and records first-run readiness findings that T40
should consume; a trial merge into this branch conflicts only in
`CHANGELOG.md`, but its help and docs text still say `lingo` and will need the
T37 `axiom` wording when integrated.
