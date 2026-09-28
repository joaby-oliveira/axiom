# Contributing to Axiom

## Project maturity

Axiom is in early development. The Codex harness, tested Go foundations, and a
bounded local Lingo control plane are available. See [project status](README.md#project-status)
for stable capability context and the [Specifications index](docs/specifications/README.md)
for authoritative scope and lifecycle state. The roadmap expresses direction,
not implementation authorization.

## Before contributing

- Search [existing Issues](https://github.com/rgomids/axiom/issues) before opening a new one.
- Open or discuss an Issue before a significant change. Identify any affected Specification, ADR, or contract.
- Do not implement Tasks without explicit authority. An Issue or roadmap entry is not approval.
- Read [AGENTS.md](AGENTS.md), [documentation governance](docs/documentation.md), and the [Code of Conduct](CODE_OF_CONDUCT.md).
- Never include secrets, credentials, or private data in Issues, commits, PRs, or evidence. Sanitize reproductions and logs.
- Report vulnerabilities through the private process in [SECURITY.md](SECURITY.md), never in a public Issue. See [SUPPORT.md](SUPPORT.md) for other requests.

## Development flow

Axiom uses [GitHub Flow](https://docs.github.com/en/get-started/using-github/github-flow).
`main` is the only long-lived branch and is always releasable; there is no
`develop` branch and no GitFlow.

```text
feature/fix branch -> Pull Request -> required CI -> review + approval -> squash merge -> main
```

Requirements and setup details live in [Getting Started](docs/development/getting-started.md). Git, Bash, standard POSIX utilities, and Go 1.26 or later are required for the checks below. The first Go run may download the dependency pinned in `go.mod`.

```bash
git clone https://github.com/rgomids/axiom.git
cd axiom
./scripts/validate-repository.sh .
go test ./...
```

1. Branch from the current `main` with one small, focused scope, following the branch naming convention below.
2. Consult affected contracts; add or adjust tests appropriate to the change, using test-first development where useful.
3. Keep commits coherent. Do not mix unrelated refactors or changes.
4. Update only affected documentation and include verifiable Evidence: commands, outcomes, and known limits. Raw claims of completion are insufficient.
5. Run the relevant checks below, inspect the diff, and open a Pull Request against `main`. Address review feedback within the agreed scope.
6. After required CI passes, approval is given and every review thread is resolved, the PR is squash-merged. Nobody pushes directly to `main`.

A merge to `main` never publishes a release. It only feeds the next
[Release PR](#release-flow).

## Work Item lifecycle governance

The Issue #94 amendment, approved by human review on 2026-09-24, establishes the
following rules for Axiom-managed Work Items. Approval of the amendment does not by
itself authorize T26–T29 implementation; S6 implementation remains separately gated.

- Repository artifacts remain technical source of truth; local Execution/workflow
  state remains canonical workflow truth; Provider metadata is a durable
  projection and recovery signal only.
- GitHub lifecycle projection uses exactly one canonical `axiom:stage:*` marker
  from the closed Specification 004 set. `axiom:blocked`,
  `axiom:needs-decision`, `axiom:needs-approval`, and
  `axiom:recovery-required` are independent flags, never synthetic stages.
- Do not manually treat labels, Issue closure, merge, green CI, or PR review as
  authority to advance local workflow or record acceptance.
- Transition comments must be bounded and reference Specifications, Plans/Tasks,
  PRs, Evidence, next actions, or blockers instead of copying dense documents,
  logs, or chat.
- Apply Project metadata policy deterministically. Ask only for required Work
  Item or Pull Request metadata still unresolved after policy/context resolution;
  keep concrete GitHub fields in the adapter boundary.
- If local workflow truth is unavailable, inspect and reconcile. Never reconstruct
  an Execution, advance a canonical gate, or infer a derived stage from
  Provider/Repository state; contradictory or insufficient facts require
  `recovery_required` and human decision.

PRs affecting this lifecycle must state the current and target canonical Execution
gates, the current and expected derived lifecycle stages, prerequisites and
authority, auxiliary flags, Provider effects, recovery impact, and whether a human
acceptance decision remains pending. `Not applicable` is valid only with an
objective reason.

## Branch names

Use this format:

```text
<category>/<concise-kebab-case-description>
```

Use a lowercase category that identifies the kind of work or its bounded delivery context. Prefer the commit types below when they fit. Categories such as `agent`, `issue`, `impl`, `release`, and `poc` are also acceptable when they make the workflow context clearer.

Examples:

```text
agent/s3-intent-work-item
docs/spec-004-tasks-approval
issue/62-mvp-specification
fix/pages-deployment-artifact
security/filesystem-destination-ownership
```

Use lowercase ASCII letters, digits, and hyphens; do not use spaces or underscores. Include a relevant Issue, Specification, Task, or Slice identifier when it improves traceability, but do not invent one when none applies. Avoid vague names such as `changes`, `update`, `wip`, or `my-branch`. Each branch must represent one coherent scope, and its name never grants implementation or external-effect authorization. Branches named `release-please--*` belong to the Release PR automation; do not push to them.

## Commit messages

Axiom uses [Conventional Commits](https://www.conventionalcommits.org/en/v1.0.0/):

```text
<type>(<optional-scope>)<optional !>: <concise description>
```

Accepted types are `feat`, `fix`, `docs`, `test`, `refactor`, `perf`, `build`, `ci`, `chore`, `security`, and `revert`. The scope is optional; use it only when it improves understanding.

Examples:

```text
feat(web): add bootstrap landing page
fix(pages): correct deployment artifact path
docs(contributing): define commit message requirements
ci(pages): deploy static site to GitHub Pages
security(filesystem): reject unsafe destination ownership
feat(cli)!: rename the configure command
```

Because PRs are squash-merged, **the PR title becomes the commit on `main`** and
must follow this format; the squash body may carry footers. The types drive the
next version and the changelog:

| On `main` | Next version (while `0.x`) | Next version (from `1.0.0`) | Changelog section |
|---|---|---|---|
| `!` or `BREAKING CHANGE:` footer | minor | major | per type, marked breaking |
| `feat` | minor | minor | Features |
| `fix` | patch | patch | Bug Fixes |
| `security`, `perf`, `revert` | patch | patch | Security, Performance, Reverts |
| `docs`, `test`, `refactor`, `build`, `ci`, `chore` | no release on its own | no release on its own | hidden |

A `Release-As: X.Y.Z` footer forces the next stable version when the computed
one is not what maintainers decided.

Each commit must represent one coherent unit of change. Its subject must explain the observable purpose, not merely identify modified files. Generic messages such as `wip`, `update`, `changes`, `fix`, `misc`, `added stuff`, or equivalents are not acceptable in final history. Do not group unrelated changes in one commit.

Temporary `fixup!`, `squash!`, or WIP commits may exist on a branch; the squash merge replaces them with the reviewed PR title. Add a body when the subject alone does not make the context or rationale clear.

## Validation

Run commands directly from the repository root. The [command reference](docs/commands.md) explains their scope and offline options.

```bash
./scripts/validate-repository.sh .
./scripts/validate-agent-package.sh .
./scripts/check-sensitive-files.sh .
go test ./...
go vet ./...
go build ./...
go mod verify
git diff --check
```

Before every commit, inspect staged paths and content and run `./scripts/check-sensitive-files.sh --staged .`. Follow [repository security](docs/security/repository-security.md), including the dedicated secret scanner when available. Report unavailable checks explicitly. Harness and documentation checks do not prove unimplemented product behavior.

### Required CI

The [CI workflow](.github/workflows/ci.yml) runs on every Pull Request and every
push to `main`. Its jobs are the required checks of the `main` ruleset
([desired state](.github/rulesets/main.json)):

| Required check | Runs |
|---|---|
| `verify (linux)`, `verify (macos)` | `go test -race ./...`, `go vet ./...`, `go build ./...`, `go mod verify`, `./scripts/validate-repository.sh .`, `./scripts/dogfood-poc.sh` |
| `release-contract` | `./scripts/test-release-pipeline.sh`, `./scripts/test-release-flow.sh` |

CI has a read-only token and never publishes anything. Renaming a job changes a
required check: update the ruleset file and the repository ruleset together.

## Pull requests

Use the [PR template](.github/PULL_REQUEST_TEMPLATE.md). A PR without a description is not ready for review. Replace every template placeholder with real information or an objective explanation of why that item is not applicable. The description must provide:

- context and the problem being solved;
- scope and non-goals;
- related Issue, Specification, or ADR when applicable;
- validations executed, their results, and reproducible Evidence;
- documentation and security impact;
- known limitations;
- confirmation that unrelated changes are absent.

CI results do not replace Evidence or the context required in the PR description. Before requesting review, the author must inspect the PR title, description, and commit history.

### Ready for review

A PR is ready for review only when, at minimum:

1. its description is complete;
2. its scope is coherent;
3. relevant Issue, Specification, and ADR references are linked;
4. applicable validations and Evidence are recorded;
5. known limitations are declared;
6. temporary or WIP commits are cleaned up; and
7. the diff contains no unrelated changes.

### Merge policy

`main` is protected by a repository ruleset: changes arrive only through a Pull
Request with at least one approval, code owner review ([CODEOWNERS](.github/CODEOWNERS)),
all review threads resolved and all required checks green. Squash merge is the
only merge method; force-push and deletion of `main` are blocked. Only an
authorized maintainer merges, and an agent never merges its own Pull Request
without explicit human authorization.

A technical merge does not establish human acceptance. Acceptance does not automatically authorize the next Task. Contributions may be declined, split, or reformulated during review. Required explicit authorization must be recorded before work proceeds.

Accepted submissions are licensed under [Apache-2.0](LICENSE), unless explicitly stated otherwise. Contributors must hold the rights necessary to submit their content; identify any different licensing explicitly for review.

## Release flow

```text
main -> Release PR -> review + approval -> squash merge -> explicit publication
     -> tag vX.Y.Z[-rc.N] -> build + verify -> GitHub Release
```

GitHub Releases is the initial distribution channel. Preparing a versioned
state, building/verifying artifacts and publishing are separate steps, and
publication always requires explicit human authority.

### Versioning

Versions follow [SemVer](https://semver.org/). Public tags are:

- stable: `vMAJOR.MINOR.PATCH`, published as a normal GitHub Release, and the
  `latest` release when it is the highest stable version;
- release candidate: `vMAJOR.MINOR.PATCH-rc.N`, published as a GitHub
  prerelease, never `latest`, installable only by its exact tag.

Tags are never moved or deleted, and a published release is never replaced.
[`scripts/release-tag-version.sh`](scripts/release-tag-version.sh) and
[`scripts/release-preflight.sh`](scripts/release-preflight.sh) implement the
tag and ordering rules.

### Release PR

[Release Please](https://github.com/googleapis/release-please)
([workflow](.github/workflows/release-please.yml),
[config](release-please-config.json)) keeps one Release PR open against `main`
once a releasable Conventional Commit lands. It proposes the next SemVer
version, groups the commits into a new `CHANGELOG.md` section and updates
[`.release-please-manifest.json`](.release-please-manifest.json).
It is configured with `skip-github-release`: it never creates tags or releases.

Merging the Release PR is a normal reviewed squash merge. It records the
versioned state on `main` (that squash commit is the *release commit*) and
nothing else. Until that stable version is published, Release Please opens no
new Release PR. Curated dated entries in `CHANGELOG.md` continue as before; the
Release PR inserts the version heading above the entries it releases, and only
the Release PR edits those version headings.

### Publication

[`publish-release.yml`](.github/workflows/publish-release.yml) is the only
workflow that creates tags and GitHub Releases. It runs only when dispatched
from `main` with an exact tag and full revision, and its `publish` job waits for
approval of the protected `release` environment. It then:

1. re-checks the tag, the revision and the Release PR binding with `release-preflight.sh`;
2. builds the complete set with `build-release-archives.sh` from a clean checkout of that revision;
3. verifies it with `verify-release-artifacts.sh` (closed artifact set, `SHA256SUMS`, provenance, exact revision);
4. creates or reconciles one **draft** release bound to the revision, uploads every asset and reads back each digest;
5. publishes once and reads back the release, the tag and the `latest` pointer.

A stable release must be published from its release commit. A release
candidate may be published from any `main` revision that does not record a
newer version (typically `main` before the Release PR, or the release commit
itself). Reruns converge: a matching draft is completed, a consistent
published release is a no-op, and any duplicate, foreign asset, moved tag or
inconsistent release fails closed without changes.

### Authority

| Action | Who |
|---|---|
| merge a feature/fix PR | maintainer with merge authority |
| review, approve and merge the Release PR | maintainer; merging does not publish |
| dispatch publication of an exact tag and revision | maintainer, explicitly |
| approve the `release` environment | maintainer, in GitHub |
| repository settings, rulesets, environments | repository administrator |

Green CI, a merged Release PR or an earlier approval never authorizes a
publication. Agents may prepare, verify and report, and dispatch publication
only after an explicit human authorization of the exact preview; they never
approve the environment, create tags or edit releases by hand.

### `$axiom-release`

The maintainer skill [`axiom-release`](.agents/skills/axiom-release/SKILL.md)
conducts this flow for humans and agents:

```text
$axiom-release              # discover the state and do the next step
$axiom-release v0.2.0-rc.1  # conduct that release candidate
$axiom-release v0.2.0       # conduct that stable release
```

It runs [`scripts/release.sh`](scripts/release.sh) (`status`, `publish`,
`verify`), stops at the authority boundary with the exact publication preview,
and after authorization dispatches the workflow, waits for the run and verifies
the published tag, revision and assets. Detailed commands, settings and
Evidence are in the [command reference](docs/commands.md#release-flow) and
[repository security](docs/security/repository-security.md#release-and-branch-protection).

### Merge versus release

| | Merge to `main` | Release |
|---|---|---|
| Trigger | squash merge of a reviewed PR | explicit dispatch of `publish-release.yml` |
| Effects | commit on `main`; CI; Release PR update | tag, GitHub Release, assets |
| Authority | PR approval and required CI | explicit publication authority and `release` environment approval |
| Reversible | by a new PR | never silently: tags and published releases are immutable |

## AI-assisted contributions

AI-assisted contributions are allowed. The human contributor remains responsible for every submission and must verify code, licenses, sources, and claims. Prompts or raw model outputs do not replace Evidence. Do not submit private content or material whose licensing does not permit its inclusion.
