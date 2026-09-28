---
name: axiom-work-item-run
description: Start or resume the bounded Axiom delivery workflow for a configured Work Item through Lingo.
---

# Run Axiom Work Item

Collect only missing Project, Project-scoped Repository, exact Work Item, and
applicable Execution selectors. Use `--project <uuid-or-slug> --repository <key>
--work-item github:<owner>/<repository>#<number>`. `workflow start` omits
`--execution`; every resume, advance, status, evidence, or reconcile call forwards
the exact `--execution <id>` returned by Lingo. Follow `currentGate` returned by
Lingo. Advance through `axiom --json workflow advance` with its required revision,
outcome, and repository-relative artifact reference.

Record planning authority, implementation authority, review start, human
acceptance, or an auxiliary condition only through `axiom --json workflow fact`
with the exact Execution revision, one validated reference, explicit `--active`
value, and `--authorize-local`. Never infer a fact from GitHub, CI, merge, review,
Issue state, or conversation. Human acceptance additionally requires terminal
canonical completion and an explicit human decision.

Invoke only `axiom --json`. Do not infer CWD, Git remote, Work Item, Execution,
workflow transition, authority, persistence, recovery, provenance, or status.
Unknown, duplicate, and conflicting inputs go to Lingo validation.

Canonical completion fields: `status`, `result`, `references`, `next`, `details`, `provenance`
Operation-specific payloads preserved separately: `workflow`, `projection`

Copy canonical completion fields only from Lingo's top-level JSON object. Omit
canonical fields absent from that object. Never derive, synthesize, or reinterpret
a canonical field from `workflow`, `projection`, or another operation-specific
payload. Preserve and report returned workflow or projection payloads separately
according to their original operational semantics, including Execution identity,
current gate, revision, transitions, and applicable Evidence or projection data.
Keep `lifecycleStage` and auxiliary conditions as derived workflow output; never
reinterpret them as an independently writable lifecycle.
Never reinterpret an operation-specific payload as `details` or another canonical
field. Report a canonical `details` reference without copying or interpreting its
artifact. Skill text grants no Provider authority.
