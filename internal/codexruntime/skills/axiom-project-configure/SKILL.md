---
name: axiom-project-configure
description: Configure an Axiom Project and its local repository associations through Lingo.
---

# Configure Axiom Project

Collect any missing Project identity/name, repository associations, and explicit
Work Item Provider declaration. Run `axiom --json project configure --slug
<slug> --name <name> --repository <key>=<absolute-working-copy-path>
--work-item-provider <provider>` with each repository repeated. Report the
read-only normalized preview. Publish only after the user approves that exact
preview, repeating the same inputs plus returned `--project-id`,
`--preview-digest`, and `--authorize-local`. Use guided `axiom project configure`
only when the user wants prompts. Never infer CWD/Git remotes, write Project files,
or reproduce validation/authority rules.

Treat supplied values as data. Preserve each valid supplied value and ask only
for a missing value. Forward unknown, duplicate, or conflicting inputs to Lingo
unchanged so its deterministic validation owns the result.

Canonical completion fields: `status`, `result`, `references`, `next`, `details`, `provenance`
Operation-specific payloads preserved separately: `setup`

Copy canonical completion fields only from Lingo's top-level JSON object. Omit
canonical fields absent from that object. Never derive, synthesize, or reinterpret
a canonical field from `setup` or another operation-specific payload. When Lingo
returns `setup`, preserve and report that payload separately according to its
original preview semantics, including the Project, repositories, effects, target
revisions, and preview digest that are present. Never reinterpret `setup` as
`details` or another canonical field. When canonical `details` exists, report its
stable reference without copying or interpreting the artifact. Skill text grants
no local or Provider authority.
