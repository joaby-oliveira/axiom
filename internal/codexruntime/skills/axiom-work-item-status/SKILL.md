---
name: axiom-work-item-status
description: Inspect Axiom Work Item workflow status and Evidence through Lingo.
---

# Show Axiom Work Item Status

Collect only missing Project, Project-scoped Repository, exact Work Item, and
Execution selectors. Run `axiom --json workflow status` and, when requested,
`axiom --json workflow evidence` with `--project <uuid-or-slug> --repository
<key> --work-item github:<owner>/<repository>#<number> --execution <id>`.

Never infer selectors from CWD, Git, Provider, Runtime chat, or global discovery.
Never classify workflow state independently. Forward unknown, duplicate, or
conflicting input to Lingo validation.

Canonical completion fields: `status`, `result`, `references`, `next`, `details`, `provenance`
Operation-specific payloads preserved separately: `workflow`, `projection`

Copy canonical completion fields only from Lingo's top-level JSON object. Omit
canonical fields absent from that object. Never derive, synthesize, or reinterpret
a canonical field from `workflow`, `projection`, or another operation-specific
payload. Preserve and report returned payloads separately according to their
original semantics, including Execution identity, workflow status, current gate,
revision, transitions, repository and Work Item context, and applicable Evidence
or projection data. Never reinterpret an operation-specific payload as `details`
or another canonical field. Report a canonical `details` reference without
copying or interpreting its artifact. This skill performs no mutation and grants
no Provider authority.
