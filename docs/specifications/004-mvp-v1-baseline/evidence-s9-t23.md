# S9 / T23 — RC gates and administrative authority proposal

## Historical discovery scope and source — 2026-09-28

User authority: revalidate main, reconcile Linux terminology locally and prepare
when prerequisites are valid. Administrative mutations and publication each need
an exact separately authorized envelope. T24, Runtime invocation, stable
promotion and human acceptance are excluded.

- Repository: public `rgomids/axiom`.
- Exact main: `fd97fc7d39810a2ffad9a104e4be8779c726f9ee`.
- PR [#114](https://github.com/rgomids/axiom/pull/114): MERGED;
  merge SHA above, merged at `2026-09-29T02:43:34Z`.
- Primary checkout preserved: clean `main` at
  `084cc384d4c9de3f5244a0867d22548d350c84d5`; only remote-tracking fetch.
- Local reconciliation: isolated managed worktree based on exact main;
  uncommitted documentation changes, no commit/push/PR/merge.
- Required CI checks observed successful: `verify (linux)`, `verify (macos)`,
  `release-contract`, GitHub Actions integration `15368`.
  [CI run](https://github.com/rgomids/axiom/actions/runs/36513936496).
- Candidate `v0.1.2-rc.1`: tag and release absent, channel `rc`,
  `prerelease=true`, `make_latest=false`, manifest `0.1.1`, latest stable `0.1.1`.
- Versioned workflows use full-SHA Actions and minimum job permissions.
  Preparation uses `contents: read`; publish requires human release-environment
  approval and exact prepared-set envelope digest. No rebuild after authorization.

## Gate A — revalidation

`./scripts/release.sh status --tag v0.1.2-rc.1 --revision fd97fc7d39810a2ffad9a104e4be8779c726f9ee`
returned exit 0, `main_ci=success`, `revision_ci=success`,
`release_environment=protected`, `next_action=prepare`.
This script result does not establish the missing administrative controls below.
The operator therefore withholds PREPARE until all requested prerequisites hold.

Read-only remote audit covered rulesets, effective main rules, repository merge
settings, Actions permissions, immutable releases, release environment and its
branch policies. Branch-protection endpoint returns 404; effective ruleset
protection exists, so that response is not evidence of an unprotected branch.

| Control | Current remote state | Desired versioned state |
|---|---|---|
| Ruleset default id 22828068 | Active; no required checks; merge+squash; RepositoryRole 2 bypass always | Strict three CI checks; squash; bypass pull_request |
| Repository merges | squash/merge/rebase true; delete branch false; title COMMIT_OR_PR_TITLE; message COMMIT_MESSAGES | Squash only; delete branch true; title PR_TITLE; message PR_BODY |
| Actions permissions | enabled true, allowed_actions all, sha_pinning_required false | Same, pinning true |
| Workflow token | read; can_approve_pull_request_reviews true | Already aligned |
| Immutable releases | enabled false; enforced_by_owner false | enabled true |
| release-tags | Absent | Versioned active tag ruleset |
| release environment | Reviewer rgomids id 5471480; main branch only; prevent_self_review false | Already aligned |
| Environment admin bypass | can_admins_bypass true | No different value defined by versioned contract |
| CODEOWNERS | * @rgomids | Already aligned |
| Repository / release-environment secrets | Both count zero | Already aligned |

## Gate B — Linux reconciliation

Specification, Plan, Tasks and Specifications index now use Linux for current
support obligations. The historical preflight gets an explicit supersession note;
its original observations remain unchanged. Plan research and runner snapshots
are explicitly historical. Build source already uses `linux` metadata and static
Linux archives for amd64/arm64; no executable behavior changes are needed.
Filesystem, ownership/mode/ACL, atomicity and native T24 Evidence requirements
remain. OS names are macOS/Linux; macOS executable filter remains 27.0/arm64.
No native acceptance is claimed. These edits remain local and must be integrated
before they are treated as versioned release acceptance contracts.

## Gate C — original administrative proposal (authorized and applied below)

Envelope target repository: `rgomids/axiom`. Payload source revision:
`fd97fc7d39810a2ffad9a104e4be8779c726f9ee`. Re-read current state before applying;
any drift requires a revised envelope. Proposed effects are exactly the five
mutations below plus read-back. No environment change, secret, workflow-token
change, tag movement, release mutation, Issue/comment/label or Runtime effect.

### 1. Main ruleset

- **Target:** `repos/rgomids/axiom/rulesets/22828068`.
- **Current state:** deletion/non_fast_forward, approval 1, code-owner review,
  thread resolution and code_quality enabled; no checks; merge+squash;
  RepositoryRole id 2 bypass `always`.
- **Desired state:** exact `.github/rulesets/main.json` at source revision;
  SHA-256 `6ecf62ca4e7d39ec9359513dcfacb2242c9b1d5469b89eaacf3d7aee6ccbaaf2`.
  Checks `verify (linux)`, `verify (macos)`, `release-contract`, integration 15368,
  strict=true; squash only; bypass `pull_request`; other fields as versioned.
- **Exact mutation:**

```bash
gh api --method PUT repos/rgomids/axiom/rulesets/22828068 --input .github/rulesets/main.json
```

- **Expected effect:** enforce CI before merge and confine role bypass to PRs.
- **Risk:** stale/missing CI blocks merges; direct bypass pushes denied.
- **Recovery:** reapply preserved previous request-compatible ruleset payload only
  under separate authority; no automatic protection reduction.

### 2. Release-tags ruleset

- **Target:** `repos/rgomids/axiom/rulesets`, new name `release-tags`.
- **Current state:** absent; only `default` listed.
- **Desired state:** exact `.github/rulesets/release-tags.json` at source revision;
  SHA-256 `3e70a496102a974247dcade3780e756f1d2b152b2a9f42a1b3db307ebe97ce04`.
  Tag target, active, `refs/tags/v*`, deletion/update rules, no bypass actors.
- **Exact mutation:**

```bash
gh api --method POST repos/rgomids/axiom/rulesets --input .github/rulesets/release-tags.json
```

- **Expected effect:** prevent moving/deleting version tags; creation stays allowed.
- **Risk:** corrections require new versions; affects existing v* tags too.
- **Recovery:** changing/disabling the new ruleset needs separate administrative
  authority. Never move/delete tags as automatic recovery.

### 3. Repository merge settings

- **Target:** `repos/rgomids/axiom`.
- **Current state:** allow_squash_merge=true, allow_merge_commit=true,
  allow_rebase_merge=true, squash_merge_commit_title=COMMIT_OR_PR_TITLE,
  squash_merge_commit_message=COMMIT_MESSAGES, delete_branch_on_merge=false.
- **Desired state / Exact mutation:**

```bash
gh api --method PATCH repos/rgomids/axiom \
  -F allow_squash_merge=true -F allow_merge_commit=false -F allow_rebase_merge=false \
  -f squash_merge_commit_title=PR_TITLE -f squash_merge_commit_message=PR_BODY \
  -F delete_branch_on_merge=true
```

- **Expected effect:** future merges use squash and PR title/body; merged branches
  are removed automatically.
- **Risk:** a merged branch can still be referenced by other work; preserve its SHA.
- **Recovery:** PATCH previous values only under separate authority; restoring a
  removed branch requires its preserved SHA and distinct authority.

### 4. Actions SHA pinning

- **Target:** `repos/rgomids/axiom/actions/permissions`.
- **Current state:** enabled=true, allowed_actions=all, sha_pinning_required=false.
- **Desired state / Exact mutation:**

```bash
gh api --method PUT repos/rgomids/axiom/actions/permissions \
  -F enabled=true -f allowed_actions=all -F sha_pinning_required=true
```

- **Expected effect:** require full-SHA Action references.
- **Risk:** future symbolic action refs will fail; current versioned uses are pinned.
- **Recovery:** protection reduction requires separate authority; prefer pinning
  any failing workflow instead.

### 5. Immutable releases

- **Target:** `repos/rgomids/axiom/immutable-releases`.
- **Current state:** enabled=false, enforced_by_owner=false.
- **Desired state:** enabled=true.
- **Exact mutation:**

```bash
gh api --method PUT repos/rgomids/axiom/immutable-releases
```

- **Expected effect:** enable immutability for future published releases.
- **Risk:** published-content corrections require new versions; existing releases
  are not assumed retroactively immutable.
- **Recovery:** disabling repository setting requires separate authority and is
  not assumed to unlock already immutable tags/assets.

## Historical remaining gates before administrative authorization

PREPARE: NOT RUN. No prepared run, archive hashes, notes digest or publication
preview digest exists for this candidate in this execution. Publication envelope
will be generated only from verified remotely prepared bytes after prerequisites
pass, then requires a new explicit human authorization. Administrative approval
cannot authorize RC publication. GitHub release-environment approval remains human.

T23: BLOCKED by administrative controls and integration of local reconciliation.
T24: NOT STARTED. Human acceptance: PENDING.

## Initial local documentation validation

Documentation-only scope; no product code change or native/Runtime acceptance run.
Executed checks, exit 0:

- `./scripts/validate-repository.sh .`
- `./scripts/check-sensitive-files.sh .`
- `gitleaks detect --source . --no-git --redact --no-banner`
- `git diff --check`

Related tracked diffs and new Evidence were reviewed. Primary checkout changes
remain absent. Reconciliation and this Evidence are uncommitted local artifacts;
no administrative action, preparation dispatch or publication was performed.

## Authorized administrative execution — 2026-09-29

Human authority in the current conversation explicitly approved all five Gate C
mutations, including automatic deletion of merged branches, and separately approved
one local reconciliation commit, a dedicated branch push and a PR against main.
No PR merge, PREPARE, PUBLISH or T24 is authorized by this execution record.

Pre-mutation reads confirmed the original envelope's current states and unchanged
main `fd97fc7d39810a2ffad9a104e4be8779c726f9ee`. Both ruleset payload hashes
matched the authorized proposal. Each exact mutation above ran once, followed
immediately by a fresh GET and a deterministic comparison before the next mutation.

Read-back ledger recorded at `2026-09-29T03:04:12.560860+00:00` (UTC):

### Main ruleset PUT / GET; id 22828068

Mutation exit 0; independent read-back exit 0; final comparison PASS.
Canonical read-back SHA-256: `6c7e893dfbf5a340a73e53827013479147e90a17dbcab1abaa3bf308c3dd6669`.

```json
{
  "bypass_actors": [
    {
      "actor_id": 2,
      "actor_type": "RepositoryRole",
      "bypass_mode": "pull_request"
    }
  ],
  "conditions": {
    "ref_name": {
      "exclude": [],
      "include": [
        "~DEFAULT_BRANCH"
      ]
    }
  },
  "enforcement": "active",
  "name": "default",
  "rules": [
    {
      "parameters": {
        "severity": "errors"
      },
      "type": "code_quality"
    },
    {
      "type": "deletion"
    },
    {
      "type": "non_fast_forward"
    },
    {
      "parameters": {
        "allowed_merge_methods": [
          "squash"
        ],
        "dismiss_stale_reviews_on_push": false,
        "require_code_owner_review": true,
        "require_extra_approval_for_unattributed_changes": true,
        "require_last_push_approval": false,
        "required_approving_review_count": 1,
        "required_review_thread_resolution": true,
        "required_reviewers": []
      },
      "type": "pull_request"
    },
    {
      "parameters": {
        "do_not_enforce_on_create": false,
        "required_status_checks": [
          {
            "context": "verify (linux)",
            "integration_id": 15368
          },
          {
            "context": "verify (macos)",
            "integration_id": 15368
          },
          {
            "context": "release-contract",
            "integration_id": 15368
          }
        ],
        "strict_required_status_checks_policy": true
      },
      "type": "required_status_checks"
    }
  ],
  "target": "branch"
}
```

### Release-tags POST / GET; id 24154141

Mutation exit 0; independent read-back exit 0; final comparison PASS.
Canonical read-back SHA-256: `9dea5c3a0e3b9a5e305a808a0ac5b58f452263b72286f218811f3482a2bffe83`.

```json
{
  "bypass_actors": [],
  "conditions": {
    "ref_name": {
      "exclude": [],
      "include": [
        "refs/tags/v*"
      ]
    }
  },
  "enforcement": "active",
  "name": "release-tags",
  "rules": [
    {
      "type": "deletion"
    },
    {
      "type": "update"
    }
  ],
  "target": "tag"
}
```

### Repository merge settings PATCH / GET

Mutation exit 0; independent read-back exit 0; final comparison PASS.
Canonical read-back SHA-256: `39a8b7d969fbd162b1d0680fc0fd09dea4c5c30d9a347cc414338db04b1e6ca0`.

```json
{
  "allow_merge_commit": false,
  "allow_rebase_merge": false,
  "allow_squash_merge": true,
  "delete_branch_on_merge": true,
  "squash_merge_commit_message": "PR_BODY",
  "squash_merge_commit_title": "PR_TITLE"
}
```

### Actions permissions PUT / GET

Mutation exit 0; independent read-back exit 0; final comparison PASS.
Canonical read-back SHA-256: `7153d1d78c9abb148af0c5933d9f30529688ee0d070d64379140af47a95e074d`.

```json
{
  "allowed_actions": "all",
  "enabled": true,
  "sha_pinning_required": true
}
```

### Immutable releases PUT / GET

Mutation exit 0; independent read-back exit 0; final comparison PASS.
Canonical read-back SHA-256: `f4b2b8919d556de186d7b4afe009126b30c99e3edefb005e4e56ab17069b52dd`.

```json
{
  "enabled": true,
  "enforced_by_owner": false
}
```

The first main-ruleset comparison exited 1 because GitHub reordered the `rules`
array (`code_quality` before `required_status_checks`). Inspection showed no field
or parameter difference. Sorting both rule arrays by type produced PASS; no second
PUT or workaround was performed. Other comparisons passed immediately.

No mutation outside the five authorized controls was performed. The release
environment and workflow-token permissions were already aligned and were not
changed. Enabling immutable releases does not establish retroactive immutability
for older releases; post-publication RC immutability remains unverified.

## Current T23 handoff

Administrative prerequisites above are reconciled. Document integration awaits
review and human merge of the dedicated reconciliation PR. PREPARE and PUBLISH
remain NOT RUN; T24 NOT STARTED; human acceptance PENDING. After human merge,
reidentify exact main SHA and rerun the full T23 preflight. Present that new state
and confirm `next_action=prepare` before any preparation dispatch. Publication
still needs explicit authority for its exact prepared-byte envelope.


## Authorized T23 PREPARE — 2026-09-29 (current execution)

This append-only record supersedes the preceding "Current T23 handoff" for
current state. Earlier discovery, administrative execution and pre-PREPARE
observations remain historical and unchanged. The administrative reconciliation
has been integrated in main; this execution uses only the exact revision below.

User authority: PREPARE only for `v0.1.2-rc.1` at
`f73d6d0c951dd40c5cc97c3794ad7ee5607092b5`, including read-only preflight,
preparation dispatch, waiting, artifact download/verification, envelope/digest
calculation and local Evidence updates. No commit, push, PR, publication,
environment approval, T24, Codex/Claude Runtime, stable promotion or human
acceptance was performed.

### Exact source, time and command ledger

- Repository: public `rgomids/axiom`.
- Branch / HEAD: clean `main` at the exact revision before PREPARE.
- `git fetch origin main` and `git rev-parse origin/main`: exit 0; main equals
  the authorized full revision, with no automatic SHA substitution.
- Official PREPARE command started: `2026-09-29T03:26:23Z` (UTC).
- Official PREPARE command completed: `2026-09-29T03:27:51Z` (UTC), exit `0`.
- Workflow created: `2026-09-29T03:26:34Z`; completed/updated: `2026-09-29T03:27:26Z`.
- Prepared run: `36517146216`, attempt `1`; workflow dispatch from main,
  exact head SHA, `.github/workflows/release-artifacts.yml`, conclusion `success`.
- Workflow: [36517146216](https://github.com/rgomids/axiom/actions/runs/36517146216).

Official commands, each exit 0:

```bash
./scripts/release.sh status --tag v0.1.2-rc.1 --revision f73d6d0c951dd40c5cc97c3794ad7ee5607092b5
./scripts/release.sh prepare --tag v0.1.2-rc.1 --revision f73d6d0c951dd40c5cc97c3794ad7ee5607092b5
./scripts/release.sh status --tag v0.1.2-rc.1 --revision f73d6d0c951dd40c5cc97c3794ad7ee5607092b5 --prepared-run 36517146216
```

Additional capture commands, each exit 0:

```bash
gh run download 36517146216 --repo rgomids/axiom --name axiom-release-v0.1.2-rc.1 --dir /tmp/axiom-t23-prepare-20260929/prepared
gh api repos/rgomids/axiom/actions/runs/36517146216
gh api repos/rgomids/axiom/actions/runs/36517146216/artifacts
```

Pre-effect administrative GETs (exit 0) confirmed: main ruleset `22828068`
active with the three expected checks, strict mode, squash-only PRs and
`pull_request` bypass; release-tags `24154141` active with update/deletion
protection; squash-only repository merge settings and
`delete_branch_on_merge=true`; Actions SHA pinning enabled; immutable releases
enabled. No administrative mutation was repeated.

Pre-PREPARE status, verbatim:

```text
statusVersion=1
repository=rgomids/axiom
root=/Users/rgomids/Projects/axiom
branch=main
head=f73d6d0c951dd40c5cc97c3794ad7ee5607092b5
worktree=clean
main=f73d6d0c951dd40c5cc97c3794ad7ee5607092b5
main_ci=success
release_environment=protected
open_release_prs=none
unpublished_release_prs=none
tag=v0.1.2-rc.1
revision=f73d6d0c951dd40c5cc97c3794ad7ee5607092b5
revision_ci=success
channel=rc
prerelease=true
manifest_version=0.1.1
latest_stable=0.1.1
tag_state=absent
make_latest=false
publication_state=absent
next_action=prepare
reason=build and verify the exact set first: release.sh prepare --tag v0.1.2-rc.1 --revision f73d6d0c951dd40c5cc97c3794ad7ee5607092b5 (no publication)
```

### Prepared set and provenance

Workflow container artifact: `axiom-release-v0.1.2-rc.1`, id `11011691241`,
size `6039166` bytes, digest `sha256:7a2eb88893c7272cbc2cd86d752499a9bfe7ba977fb1ec236a811249f22d7573`,
expired `false`, expires `2026-10-29T03:27:20Z`.
This container digest identifies the Actions upload; archive digests below
bind the publication assets. The container digest was captured from GitHub
metadata, not independently recalculated from the ZIP container.

Prepared publication asset inventory (independently measured bytes):

| Asset | Bytes | SHA-256 |
|---|---:|---|
| `SHA256SUMS` | 309 | `d8442bc6f4ff53a26ca29640c5613b951d9c47f06c217b23649000b388213f55` |
| `axiom-0.1.2-rc.1-linux-amd64.tar.gz` | 2137963 | `aa26c3808541cb2f8ee82baa21212fe86f8055eef27ff9d92e00550c514ac949` |
| `axiom-0.1.2-rc.1-linux-arm64.tar.gz` | 1939233 | `58ed74526e82703d72a60a02905df180aeadc5e742fd5e93f179a306500883fc` |
| `axiom-0.1.2-rc.1-macos-27-arm64.tar.gz` | 1978680 | `a474ae70e09d85065831c601afe0e500957cdbeaabc1febbc8a73bf63d53339d` |

Only macOS/arm64, Linux/amd64 and Linux/arm64 archives are present, plus
`SHA256SUMS`. Candidate version `0.1.2-rc.1`; `prerelease=true`,
`make_latest=false`. All binaries embed the full exact source revision; bundle
metadata records its contract-defined 12-character prefix `f73d6d0c951d`.

Prepared metadata, verbatim:

```text
tag=v0.1.2-rc.1
revision=f73d6d0c951dd40c5cc97c3794ad7ee5607092b5
workflowRun=https://github.com/rgomids/axiom/actions/runs/36517146216
workflowRunAttempt=1
workflowRef=rgomids/axiom/.github/workflows/release-artifacts.yml@refs/heads/main
go=go1.26.0
```

Prepared artifact verification Evidence, verbatim:

```text
evidenceVersion=1
product=Axiom
version=0.1.2-rc.1
revision=f73d6d0c951dd40c5cc97c3794ad7ee5607092b5
sha256sums=d8442bc6f4ff53a26ca29640c5613b951d9c47f06c217b23649000b388213f55
archive=axiom-0.1.2-rc.1-macos-27-arm64.tar.gz sha256=a474ae70e09d85065831c601afe0e500957cdbeaabc1febbc8a73bf63d53339d axiom_sha256=c7a843db5500014276d7e01f27911bc98bbaeeb86668700d9d56d0fd5d21e4d0 manifest_sha256=dd67b299f3ddb1560054fb6f21884d5a71b4624c0477f5ff52d19e4b7faf6ad2 version_smoke=not_host_architecture
archive=axiom-0.1.2-rc.1-linux-amd64.tar.gz sha256=aa26c3808541cb2f8ee82baa21212fe86f8055eef27ff9d92e00550c514ac949 axiom_sha256=996415c89a7756c857abbdebf0da152a2acb18c6d056b866bb3e2b742941b37b manifest_sha256=8004ac7ebcdb1c0234a5d7fc20dc150022f218137c9eb635f5e5d80f954bbe0c version_smoke=pass
archive=axiom-0.1.2-rc.1-linux-arm64.tar.gz sha256=58ed74526e82703d72a60a02905df180aeadc5e742fd5e93f179a306500883fc axiom_sha256=f79c9829e26a4c6b14664a215772e27658bae74c83b7240aa1cf979712442d94 manifest_sha256=530734111637abbf921304214e448ba6b1b1de08f46e9fa33a6956ae7a5f83b9 version_smoke=not_host_architecture
publication=none
result=pass
```

`SHA256SUMS`, verbatim:

```text
a474ae70e09d85065831c601afe0e500957cdbeaabc1febbc8a73bf63d53339d  axiom-0.1.2-rc.1-macos-27-arm64.tar.gz
aa26c3808541cb2f8ee82baa21212fe86f8055eef27ff9d92e00550c514ac949  axiom-0.1.2-rc.1-linux-amd64.tar.gz
58ed74526e82703d72a60a02905df180aeadc5e742fd5e93f179a306500883fc  axiom-0.1.2-rc.1-linux-arm64.tar.gz
```

Bundle release metadata for `axiom-0.1.2-rc.1-linux-amd64.tar.gz`, verbatim:

```text
formatVersion=1
product=Axiom
version=0.1.2-rc.1
revision=f73d6d0c951d
sourceState=clean
release=true
platform=linux
goos=linux
architecture=amd64
skillSetVersion=1
```

Bundle release metadata for `axiom-0.1.2-rc.1-linux-arm64.tar.gz`, verbatim:

```text
formatVersion=1
product=Axiom
version=0.1.2-rc.1
revision=f73d6d0c951d
sourceState=clean
release=true
platform=linux
goos=linux
architecture=arm64
skillSetVersion=1
```

Bundle release metadata for `axiom-0.1.2-rc.1-macos-27-arm64.tar.gz`, verbatim:

```text
formatVersion=1
product=Axiom
version=0.1.2-rc.1
revision=f73d6d0c951d
sourceState=clean
release=true
platform=macos-27
goos=darwin
architecture=arm64
skillSetVersion=1
```

Prepared release notes, verbatim (instructions below were captured, not executed):

````markdown
Release candidate v0.1.2-rc.1. It is a GitHub prerelease: the default and
`--channel stable` installer selectors never choose it; install it only by
its exact tag. It is not a stable release and records no human acceptance.

---

- Source revision: `f73d6d0c951dd40c5cc97c3794ad7ee5607092b5`
- Channel: release candidate (prerelease)
- Assets: `SHA256SUMS` and one `axiom-0.1.2-rc.1-<row>.tar.gz` archive per supported row

Install exactly this release:

```bash
curl -fsSL https://raw.githubusercontent.com/rgomids/axiom/main/scripts/install.sh | sh -s -- --version v0.1.2-rc.1
```

The installer verifies each archive against `SHA256SUMS` before reading it.
Checksums provide integrity only; they are not a signature or a publisher
authenticity claim.
````

### Re-verification and independent envelope check

Official `status --prepared-run` downloaded the prepared bytes again and used
a clean clone of the exact revision. The source's official verifier checked
run identity/conclusion, complete supported asset set, archive entry allowlist,
file types/modes, archive and inner manifest hashes, source LICENSE/installer/
skills equality, release metadata, executable architecture and embedded Go
provenance, host version smoke and regenerated notes. It returned
`next_action=authorize_publication`. The initial PREPARE result and separate
status returned the same preview digest.

Independent verification: inline `python3` using `pathlib`, `hashlib`, `json`
and `tarfile`, exit 0 / PASS. It read a separate retained workflow download,
asserted the exact three archive names plus SHA256SUMS, recomputed all four
asset hashes and sizes, compared archive hashes with SHA256SUMS and every
matching `preview.*` digest, checked safe regular/directory archive members,
and read every bundle's version/source/target metadata. It independently
checked run head/conclusion/attempt and prepare tag/revision/workflow identity,
then hashed the exact prepared notes. It reconstructed the entire envelope in
the field order below using measured digests and expected exact identities,
UTF-8 and LF after every line (including the last). Both envelope bytes and
SHA-256 matched the official status. No publication mode was invoked.

To reproduce the final digest over the exact block below (without fences or
`preview_digest`), save it as `envelope.txt` with the final LF and run:

```bash
shasum -a 256 envelope.txt
```

Complete publication envelope (planned effects only; none executed):

```text
envelopeVersion=1
repository=rgomids/axiom
tag=v0.1.2-rc.1
version=0.1.2-rc.1
channel=rc
prerelease=true
revision=f73d6d0c951dd40c5cc97c3794ad7ee5607092b5
make_latest=false
prepared_run=36517146216
release_notes_sha256=de59f07297db936f556de496f4cb3d35a4a37009d2d49ebcddaa9a574adfd632
sha256sums_sha256=d8442bc6f4ff53a26ca29640c5613b951d9c47f06c217b23649000b388213f55
artifact.axiom-0.1.2-rc.1-linux-amd64.tar.gz=aa26c3808541cb2f8ee82baa21212fe86f8055eef27ff9d92e00550c514ac949
artifact.axiom-0.1.2-rc.1-linux-arm64.tar.gz=58ed74526e82703d72a60a02905df180aeadc5e742fd5e93f179a306500883fc
artifact.axiom-0.1.2-rc.1-macos-27-arm64.tar.gz=a474ae70e09d85065831c601afe0e500957cdbeaabc1febbc8a73bf63d53339d
publication_state=absent
tag_state=absent
release_id=none
release_pr=not_applicable
release_pr_label_state=not_applicable
effect.release=create_draft
effect.assets=upload_exact_envelope_set
effect.publish=prerelease
effect.tag=create_at_revision
effect.latest=unchanged
effect.release_pr_label=none
```

`preview_digest=e5a0391733f72dee3e0c550bfcf2c39b4578d56f41cd0afb1f545903af7b3680`

### Side-effect ledger, limitations and authority gate

- One authorized `release-artifacts.yml` workflow dispatch; run `36517146216`.
- GitHub preparation built/verified three archives, rendered notes and Evidence,
  and uploaded the retained Actions artifact. Workflow token: contents read.
- Local effects: fetch refreshed remote-tracking refs; temporary clean clones,
  downloads/verification files under private temporary directories; this local
  tracked Evidence update. Exact preparation bytes were retained locally at
  `/tmp/axiom-t23-prepare-20260929/prepared`; this path is temporary and is not
  a durable repository copy of the binary artifacts.
- Latest observed remote tag/release: absent; release identity `none`.
- Publication `NOT PUBLISHED`; no publish dispatch, tag creation, release
  creation/upload/publication, environment approval, administrative mutation,
  Issue/comment/label effect, commit, push or PR.
- Cross-build/format/provenance checks and version smoke are preparation
  verification only. No native installation/upgrade/acceptance journey or
  real Codex/Claude Runtime execution was performed. T24 `NOT STARTED`;
  human acceptance `PENDING`; post-publication immutability remains unverified.
- Envelope captures remote state observed during this execution. Before any
  future publication, re-download/reverify and recompute the envelope. Any
  byte, tag, revision or remote-state drift invalidates future authority and
  requires a fresh envelope and explicit authorization. Artifact expiry also
  prevents using an unavailable prepared set.

T23 PREPARE: **COMPLETE**. Publication gate: **STOPPED**.
`next_action=authorize_publication`. Required authority: explicit human
permission to publish this exact prepared run, revision, asset set, hashes and
preview digest. Environment approval remains a distinct human action.

Evidence remains a local, uncommitted update; no remote documentation
publication is included in the PREPARE authority.


### Local Evidence validation

- `./scripts/validate-repository.sh .`: exit 1,
  `FAIL: unapproved Claude artifact: .claude`. The primary checkout contains
  an empty, untracked `.claude` directory dated 2026-09-28 17:08 local time,
  before this PREPARE. It is not part of the committed source or this change;
  it was preserved without removal or content changes.
- Repository validation was rerun on a temporary `git clone --no-hardlinks`
  of the exact HEAD with only this Evidence file copied in. Command:
  `/tmp/axiom-t23-prepare-20260929/validation-source/scripts/validate-repository.sh /tmp/axiom-t23-prepare-20260929/validation-source`.
  Exit 0 / PASS. This validates the repository source plus Evidence; it does
  not claim the primary checkout's preexisting directory passed validation.
- `./scripts/check-sensitive-files.sh .`: exit 0 / PASS.
- `gitleaks detect --source . --no-git --redact --no-banner`: exit 0,
  no leaks found.
- `git diff --check`: exit 0; complete task diff reviewed, one Evidence file
  appended, no preexisting tracked changes overwritten.

No behavioral code changed; no additional Go test/build suite was run locally.
The official prepared-byte verification and remote preparation build above are
recorded separately from these documentation checks.


## Authorized PUBLISH dispatch — 2026-09-29

The human replied `autorizado` to the exact publication envelope above.
Authority binds prepared run `36517146216`, tag `v0.1.2-rc.1`, revision
`f73d6d0c951dd40c5cc97c3794ad7ee5607092b5`, the four exact assets and their
hashes, notes digest, prerelease/latest flags, planned effects and preview
`e5a0391733f72dee3e0c550bfcf2c39b4578d56f41cd0afb1f545903af7b3680`.

Before dispatch, official `release.sh status` with the same tag, revision and
prepared run exited 0 and reverified downloaded bytes in a clean exact-source
clone. Main and revision CI were success; environment protected; remote tag
and release absent; digest unchanged; next action authorize_publication. The
local Evidence modification was preserved. It is not used as the build source.

Official authorized command, exit 0:

```bash
./scripts/release.sh publish --tag v0.1.2-rc.1 --revision f73d6d0c951dd40c5cc97c3794ad7ee5607092b5 --prepared-run 36517146216 --preview-digest e5a0391733f72dee3e0c550bfcf2c39b4578d56f41cd0afb1f545903af7b3680 --authorize-publication
```

The command independently recomputed the envelope again, matched the authorized
digest and dispatched exactly one `publish-release.yml` run:
[36517898613](https://github.com/rgomids/axiom/actions/runs/36517898613).
It reported `next_action=human_approves_release_environment_then_watch`.

`gh run watch 36517898613 --repo rgomids/axiom --exit-status` was started;
workflow completion has not been observed. Remote `preflight` passed (18s),
including verification of the retained prepared set. Read-only
`gh run view` and `gh api repos/rgomids/axiom/actions/runs/36517898613/pending_deployments`
confirmed the protected `release` environment requires human review before
`publish` can proceed. The agent did not approve the deployment.

Current publication state: **WAITING FOR HUMAN ENVIRONMENT APPROVAL**.
No publication result is claimed. Published tag/release/assets/latest/immutable
state remain unverified until workflow completion and the official
`./scripts/release.sh verify --tag v0.1.2-rc.1 --download` path can run.
No retry, manual tag/release operation, administrative change, T24, Runtime,
stable promotion, commit/push/PR or human acceptance was performed.
The previous NOT PUBLISHED snapshot remains historical; this record captures
the authorized dispatch and the pending environment gate.


## PUBLISH completion and downloaded-byte verification — 2026-09-29

Human reported environment approval in the conversation (`aprovado`). Read-only
`gh run view 36517898613 --repo rgomids/axiom --json status,conclusion,url,jobs`
confirmed completed/success for both preflight and publish. No deployment approval
was performed by the agent, and no second publication dispatch or retry occurred.

Official post-publication command, exit 0:

```bash
./scripts/release.sh verify --tag v0.1.2-rc.1 --download
```

Complete downloaded-byte verification result:

```text
publicationVersion=1
repository=rgomids/axiom
tag=v0.1.2-rc.1
revision=f73d6d0c951dd40c5cc97c3794ad7ee5607092b5
channel=rc
tag_state=present
publication_state=published
release_id=398805148
latest=v0.1.1
evidenceVersion=1
product=Axiom
version=0.1.2-rc.1
revision=f73d6d0c951dd40c5cc97c3794ad7ee5607092b5
sha256sums=d8442bc6f4ff53a26ca29640c5613b951d9c47f06c217b23649000b388213f55
archive=axiom-0.1.2-rc.1-macos-27-arm64.tar.gz sha256=a474ae70e09d85065831c601afe0e500957cdbeaabc1febbc8a73bf63d53339d axiom_sha256=c7a843db5500014276d7e01f27911bc98bbaeeb86668700d9d56d0fd5d21e4d0 manifest_sha256=dd67b299f3ddb1560054fb6f21884d5a71b4624c0477f5ff52d19e4b7faf6ad2 version_smoke=pass
archive=axiom-0.1.2-rc.1-linux-amd64.tar.gz sha256=aa26c3808541cb2f8ee82baa21212fe86f8055eef27ff9d92e00550c514ac949 axiom_sha256=996415c89a7756c857abbdebf0da152a2acb18c6d056b866bb3e2b742941b37b manifest_sha256=8004ac7ebcdb1c0234a5d7fc20dc150022f218137c9eb635f5e5d80f954bbe0c version_smoke=not_host_architecture
archive=axiom-0.1.2-rc.1-linux-arm64.tar.gz sha256=58ed74526e82703d72a60a02905df180aeadc5e742fd5e93f179a306500883fc axiom_sha256=f79c9829e26a4c6b14664a215772e27658bae74c83b7240aa1cf979712442d94 manifest_sha256=530734111637abbf921304214e448ba6b1b1de08f46e9fa33a6956ae7a5f83b9 version_smoke=not_host_architecture
artifacts_verified=pass
result=pass
```

Independent comparison asserted all four asset names, sizes and GitHub SHA-256
digests equal the prepared inventory and authorized envelope. The downloaded
archive and SHA256SUMS digests emitted by the official verifier were also
compared to the prepared inventory: PASS. Published notes equal the prepared
notes excluding trailing LF normalization in the GitHub release body.

Release metadata read-back via
`gh api repos/rgomids/axiom/releases/tags/v0.1.2-rc.1`, exit 0:

- Release: [v0.1.2-rc.1](https://github.com/rgomids/axiom/releases/tag/v0.1.2-rc.1); id `398805148`.
- Published at `2026-09-29T03:39:39Z`.
- Exact revision: `f73d6d0c951dd40c5cc97c3794ad7ee5607092b5`; remote tag confirmed at that revision.
- Channel: RC; `prerelease=true`, `draft=false`, `immutable=true`.
- `latest=v0.1.1`; RC did not become latest.
- Prepared run `36517146216`; publication run `36517898613`, both success.
- Release PR/label handoff: not applicable for this RC; no label effect.

Publication side-effect ledger from the workflow (sanitized exact event lines):

```text
2026-09-29T03:39:34.4605333Z authorized_digest=e5a0391733f72dee3e0c550bfcf2c39b4578d56f41cd0afb1f545903af7b3680
2026-09-29T03:39:35.0014160Z effect=draft_created release_id=398805148
2026-09-29T03:39:35.8394526Z effect=draft_asset_uploaded name=SHA256SUMS sha256=d8442bc6f4ff53a26ca29640c5613b951d9c47f06c217b23649000b388213f55
2026-09-29T03:39:36.6320860Z effect=draft_asset_uploaded name=axiom-0.1.2-rc.1-linux-amd64.tar.gz sha256=aa26c3808541cb2f8ee82baa21212fe86f8055eef27ff9d92e00550c514ac949
2026-09-29T03:39:37.3580581Z effect=draft_asset_uploaded name=axiom-0.1.2-rc.1-linux-arm64.tar.gz sha256=58ed74526e82703d72a60a02905df180aeadc5e742fd5e93f179a306500883fc
2026-09-29T03:39:38.0004489Z effect=draft_asset_uploaded name=axiom-0.1.2-rc.1-macos-27-arm64.tar.gz sha256=a474ae70e09d85065831c601afe0e500957cdbeaabc1febbc8a73bf63d53339d
2026-09-29T03:39:39.8009833Z effect=release_published release_id=398805148
2026-09-29T03:39:41.0560250Z release_url=https://github.com/rgomids/axiom/releases/tag/v0.1.2-rc.1
2026-09-29T03:39:41.0585350Z immutable=true
2026-09-29T03:39:41.0586098Z result=pass
```

Current state: **PUBLISHED AND VERIFIED**. Exact prepared assets were published;
no rebuild or local worktree content entered publication. The historical
waiting/NOT PUBLISHED snapshots above are preserved and superseded by this
completion record. The authorized digest remains the historical pre-effect
envelope; publication changed remote state as authorized, so it is not a fresh
publication preview and cannot authorize unrelated effects or future versions.

T24: **NOT STARTED**. Stable promotion: **NOT PERFORMED**.
Human acceptance: **PENDING**. Native release-install/upgrade acceptance and
Codex/Claude Runtime journeys remain outside this publication scope.
Evidence changes remain local and uncommitted; no commit, push or PR was made.
