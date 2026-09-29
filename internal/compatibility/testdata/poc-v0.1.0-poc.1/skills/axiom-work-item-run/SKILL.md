---
name: axiom-work-item-run
description: Start or resume the bounded Axiom delivery workflow for a configured Work Item through Lingo.
---

# Run Axiom Work Item

Collect missing Project and Work Item selectors. Run `lingo workflow start` or
`lingo workflow resume` through the `lingo --json` surface with `--project
<selector> --repository <key> --number <number>`. Follow `currentGate` and
`repositoryPath` returned by Lingo. Advance through `lingo --json workflow
advance` with an outcome and repository-relative artifact reference. Persist all
status and Evidence through Lingo; do not duplicate its workflow.
