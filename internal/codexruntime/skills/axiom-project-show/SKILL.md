---
name: axiom-project-show
description: Inspect or resolve a configured Axiom Project through Lingo from any working directory.
---

# Show Axiom Project

Collect a Project slug or ID when absent. Run `axiom --json project show
--selector <slug-or-id>` and report its structured Project and repository
associations. Never infer a repository from the Runtime's current working
directory.
Preserve the supplied selector exactly, ask only when it is absent or Lingo
reports it ambiguous, and never resolve identity independently.

Canonical completion fields: `status`, `result`, `references`, `next`, `details`, `provenance`
Operation-specific payloads preserved separately: `project`

Copy canonical completion fields only from Lingo's top-level JSON object. Omit
canonical fields absent from that object. Never derive, synthesize, or reinterpret
a canonical field from `project` or another operation-specific payload. When
Lingo returns `project`, preserve and report that payload separately according to
its original semantics, including its repositories and relevant associations.
Never reinterpret `project` as `details` or another canonical field.
