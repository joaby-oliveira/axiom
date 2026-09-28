---
name: axiom-work-item-create
description: Create or select a GitHub-backed Axiom Work Item through Lingo.
---

# Create Axiom Work Item

Collect the Project selector, Project repository key, explicit GitHub
`owner/repository`, and these structured sections: problem, desired outcome,
context, scope, constraints, non-goals, and acceptance expectations. Run
`axiom --json work-item create` with those facts first, without authority. Report
the returned draft, target, effects, expected revision, and digest for review.

Only after the human grants authority for that exact preview, repeat the same
facts with `--preview-digest <digest> --authorize-external`. Changed facts require
a new preview. For an existing linked or candidate Issue, use exact selector
`github:<owner>/<repository>#<number>`. Run `axiom --json work-item select
--project <uuid-or-slug> --repository <project-scoped-key> --work-item
<exact-selector>` first; after review, repeat with its digest and
`--authorize-local`.
Never call GitHub directly, infer a Git remote, retry an ambiguous create blindly,
or treat linkage as human acceptance. Ask only for missing or Lingo-reported
ambiguous inputs. Do not parse identity, classify status, inspect persistence, or
grant authority. Preserve transported user text as user-authored.

Canonical completion fields: `status`, `result`, `references`, `next`, `details`, `provenance`
Operation-specific payloads preserved separately: `draft`, `selection`, `workItem`

Copy canonical completion fields only from Lingo's top-level JSON object. Omit
canonical fields absent from that object. Never derive, synthesize, or reinterpret
a canonical field from `draft`, `selection`, `workItem`, or another
operation-specific payload. Preserve and report those returned payloads separately
according to their original semantics. In particular, the create preview must
retain the complete `draft`, its `target`, `effects`, expected revision, preview
digest, and every other review/authorization fact returned by Lingo. Never
reinterpret `draft`, `target`, `effects`, `selection`, or `workItem` as `details`
or another canonical field. Keep every nested field in its original Lingo JSON
position; do not extract, duplicate, rename, or relocate payload fields.
