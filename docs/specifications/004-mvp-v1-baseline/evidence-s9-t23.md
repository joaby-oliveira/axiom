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
