---
name: axiom-project-configure
description: Configure an Axiom Project and its local repository associations through Lingo.
---

# Configure Axiom Project

Collect any missing logical Project selector, name, and repository associations.
Run `lingo --json project configure --slug <slug> --name <name> --repository
<key>=<absolute-working-copy-path>` with each repository repeated. Use guided
`lingo project configure` only when the user wants prompts. Report Lingo's
structured result. Do not write Project files or reproduce validation rules.
