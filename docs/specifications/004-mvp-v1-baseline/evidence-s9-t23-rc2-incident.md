# S9 / T23 — `v0.1.2-rc.2` orphan release incident

## State — 2026-09-29

**BLOCKED** ([Issue #122](https://github.com/rgomids/axiom/issues/122)). Publication run
[36599923652](https://github.com/rgomids/axiom/actions/runs/36599923652)
made release `399339376` public and immutable, but not bound to the tag
`v0.1.2-rc.2`: GitHub reports `tag_name=untagged-898fac51a51187009dea`, and
the tag `v0.1.2-rc.2` does not exist. No new publication is authorized. No
T24/T25 progress and no human acceptance are inferred from this record. What to
do with release `399339376` is an open human decision.

This record is append-only history of the incident. It does not rewrite the
`v0.1.2-rc.1` records in [T23](evidence-s9-t23.md).

## Authority and timeline

All times UTC, 2026-09-29. Every publication dispatch below was run by the
human maintainer from their own terminal after an explicit authorization of
the exact `preview_digest`; the agent did not approve the `release`
environment.

| Step | Run | Revision | Envelope / result |
|---|---|---|---|
| PREPARE | [36589687454](https://github.com/rgomids/axiom/actions/runs/36589687454) | `7045388d8d95e40b593185c528383f26564cca9e` | `preview_digest=24fd9f3b…bebb7`; superseded when PR #119 merged |
| PUBLISH (stale revision) | [36596760471](https://github.com/rgomids/axiom/actions/runs/36596760471) | `7045388…` | cancelled by the maintainer's instruction while waiting for environment approval; `publish` job never ran |
| PREPARE | [36598035374](https://github.com/rgomids/axiom/actions/runs/36598035374) | `f03fdff55fce36bf8a8768f735dee0a656b3c9d3` | `preview_digest=870fbe62fe72cadaf5d94345cb11f1f6dd6c0fcaf447fa797fbdd5541fa8e731`, `publication_state=absent` |
| PUBLISH attempt 1 | [36599130480](https://github.com/rgomids/axiom/actions/runs/36599130480) | `f03fdff…` | draft `399339376` created, 4 assets uploaded and read back; publishing PATCH failed: `unexpected end of JSON input` |
| status after attempt 1 | — | `f03fdff…` | draft still matched by `tag_name == v0.1.2-rc.2`; new `preview_digest=1b7a84f9508b212629e4f1e4fe6421fc48f310c994d3a25d795466d2196cb68b`, `effect.release=reconcile_draft` |
| PUBLISH attempt 2 | [36599923652](https://github.com/rgomids/axiom/actions/runs/36599923652) | `f03fdff…` | `effect=release_published`, then `published release or tag did not read back` |

Attempt 2 workflow ledger (sanitized event lines):

```text
tag=v0.1.2-rc.2
revision=f03fdff55fce36bf8a8768f735dee0a656b3c9d3
tag_state=absent
publication_state=draft
authorized_digest=1b7a84f9508b212629e4f1e4fe6421fc48f310c994d3a25d795466d2196cb68b
release_id=399339376
effect=draft_notes_refreshed release_id=399339376
draft_asset_kept=axiom-0.1.2-rc.2-linux-amd64.tar.gz
draft_asset_kept=axiom-0.1.2-rc.2-linux-arm64.tar.gz
draft_asset_kept=axiom-0.1.2-rc.2-macos-27-arm64.tar.gz
draft_asset_kept=SHA256SUMS
draft_verified=399339376
effect=release_published release_id=399339376
release_publish_error: published release or tag did not read back; rerun to verify
```

## Observed remote state

Read-only `gh api repos/rgomids/axiom/releases/399339376`,
`gh api repos/rgomids/axiom/git/matching-refs/tags/v0.1.2` and the release
list, after attempt 2:

| Field | Value |
|---|---|
| release id | `399339376` |
| name | `v0.1.2-rc.2` |
| desired tag | `v0.1.2-rc.2` — **absent** (only `refs/tags/v0.1.2-rc.1` matches) |
| effective `tag_name` | `untagged-898fac51a51187009dea` |
| `target_commitish` | `f03fdff55fce36bf8a8768f735dee0a656b3c9d3` |
| draft / prerelease / immutable | `false` / `true` / `true` |
| published_at | `2026-09-29T16:46:01Z` |
| URL | <https://github.com/rgomids/axiom/releases/tag/untagged-898fac51a51187009dea> |
| latest | unchanged (`v0.1.1`) |

Assets — exactly the authorized envelope set, GitHub digests:

| Asset | SHA-256 |
|---|---|
| `SHA256SUMS` | `456128c1a5ca13a77aee5d65a9c7d0ada3d33b0f7f76b0451d76f11c4b4e501e` |
| `axiom-0.1.2-rc.2-linux-amd64.tar.gz` | `a702dc72cd2bbe008c9f64111167317cec1af4cd9847627e5b15754a83a666c0` |
| `axiom-0.1.2-rc.2-linux-arm64.tar.gz` | `5fa0413ae1d1997773ff968d4db1ff19876e01e121855e66643faaef498cc8b4` |
| `axiom-0.1.2-rc.2-macos-27-arm64.tar.gz` | `e29b65d511e2ff2d69cf81be48a8b794da6a57ea61ab5621944023a6ac6bea63` |

After the incident, `release.sh status --tag v0.1.2-rc.2 --prepared-run
36598035374` reported `publication_state=absent`, `effect.release=create_draft`
and `preview_digest=870fbe62…` again: the release matched only by
`tag_name == $tag` had disappeared from the logical state. Authorizing that
envelope would have created a second public release of the same candidate. It
was not authorized.

## Findings

**A — draft identity was not stable (Major).** Before the fix,
`publish-release.sh` refreshed an existing draft with a PATCH carrying only
`body`, published with a PATCH carrying only `draft`/`prerelease`/`make_latest`,
and checked `tag_name` only after publication. Nothing verified the draft's
identity between the two PATCHes.

*Cause — limit of the Evidence.* The run did not record the release identity
between the two PATCHes, so which one detached the release is **not
confirmed**. Facts that bound it:

- `v0.1.2-rc.1` (run 36517898613) used the same code path without the
  body-only refresh: create draft → publishing PATCH without `tag_name` →
  published and tagged correctly.
- After attempt 1 the draft was still found by `tag_name == v0.1.2-rc.2`, so
  the detach happened inside attempt 2, whose only writes were the body-only
  refresh PATCH and the publishing PATCH.
- The most likely cause is the body-only PATCH of a draft whose tag does not
  exist yet; this is a hypothesis, not a reproduced GitHub behavior. The
  earlier `unexpected end of JSON input` of attempt 1 is a separate,
  unexplained failure of the publishing request.

**B — orphan release invisible to discovery (Major).** Remote classification
selected releases only by `tag_name == $tag`, so a release belonging to the
candidate but not bound to its tag read as `absent`, offering `create_draft`.

## Correction (fail-closed)

`scripts/publish-release.sh`:

- every draft write (create, reconcile, publish) sends the full identity
  explicitly: `tag_name`, `target_commitish`, `name`, `draft`, `prerelease`;
- `check_identity` validates `tag_name`, `name`, `target_commitish`,
  `prerelease` and `draft` on the existing draft, on every PATCH/POST
  response, on the read-back before upload and on an independent read-back
  immediately before the publishing PATCH;
- after the publishing PATCH, a public release whose `tag_name` is not the tag
  is reported at once as `orphan_conflict` (no retry loop, no second release);
  success still requires the tag at the revision and the full identity;
- a release that belongs to the candidate but is not bound to its tag —
  `name == $tag`, or `untagged-*` at the revision with this version's
  artifacts — is classified `publication_state=orphan_conflict` with its id,
  tag_name, name, target, flags and asset digests, and `--check`,
  `--envelope` and publication all refuse before any effect.

`scripts/release.sh status` reports the refused check's classified state, so
the orphan appears as `publication_state=orphan_conflict` with
`next_action=blocked`, never as `absent`, and no envelope is offered.

No tag was created, no release or asset was edited or deleted, `latest` was
not touched, and no preparation or publication ran for this correction.

## Regression tests

`scripts/test-release-flow.sh` section 3b models GitHub detaching a release
whose tag does not exist when a PATCH omits `tag_name`, and injects detaches on
a draft PATCH, the publishing PATCH, or an upload. It covers:

1. normal draft creation with the full identity when the tag does not exist;
2. reconcile PATCH preserves `tag_name` and `name`;
3. reconcile PATCH preserves `target_commitish`;
4. an `untagged-*` draft PATCH response fails before any publishing PATCH;
5. a draft whose identity changes before publication fails closed;
6. a valid publication is bound to the tag at the revision;
7. the observed state of release `399339376` is `orphan_conflict`, never
   `absent`, deterministically, with its four asset digests;
8. no second draft/release, tag or other effect in that state;
9. reruns (check, envelope, publish, and the rerun after an orphan publish)
   are idempotent refusals with zero effects;
10. an unrelated `untagged-*` release is not claimed, a release named for the
    tag at another revision and a draft with a foreign name fail closed, and
    the existing foreign-asset/other-revision checks still hold.

The fake's default detach models only a PATCH that omits `tag_name` and leaves
the release a draft, consistent with `v0.1.2-rc.1`. Run against the
pre-correction scripts of `main` (`f03fdff…`), the suite's existing
interrupted-draft reconcile → publish scenario stops with
`release_publish_error: published release or tag did not read back; rerun to
verify` — the exact attempt-2 failure — while the corrected scripts pass the
whole suite. This reproduces the hypothesis in the model; it does not prove
GitHub's behavior.

`release.sh status` with the orphan returns `next_action=blocked`,
`publication_state=orphan_conflict` and no `preview_digest`; `publish` is
refused with zero effects.

## Open decisions and next steps

1. Human decision on release `399339376`: keep it as recorded incident
   history, or remove it under separate explicit authority. While it exists,
   the corrected flow refuses any `v0.1.2-rc.2` preparation envelope and
   publication. Whether GitHub's immutable-release rules restrict reusing any
   name after a delete is not verified.
2. Review and merge of the correction.
3. Only then a new `v0.1.2-rc.2` (or next RC number) flow, prepared from a
   revision that contains the correction: the publish workflow runs the
   revision's own `publish-release.sh`.

## Resolution — 2026-09-29 (read-back, T24 session)

The incident is resolved. The fix merged as
[PR #123](https://github.com/rgomids/axiom/pull/123) (`859969a07f3807822580431a05b6c78b07691fb1`,
"keep draft identity and fail closed on orphan releases") and
[Issue #122](https://github.com/rgomids/axiom/issues/122) is closed.
`GET /releases/399339376` now returns 404. Publication run
[36609926942](https://github.com/rgomids/axiom/actions/runs/36609926942)
(`success`, head `859969a…`) published release `399403900`: tag
`v0.1.2-rc.2` → `859969a…`, `prerelease=true`, `immutable=true`, four assets,
`latest` still `v0.1.1`. `./scripts/release.sh verify --tag v0.1.2-rc.2 --download`
passed. Read-back records: [evidence-s9-rc2](evidence-s9-rc2/README.md).
**T23 for `v0.1.2-rc.2`: PUBLISHED AND VERIFIED.** This section records
read-back only; the authorization of that publication run happened outside
this session.
