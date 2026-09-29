---
name: axiom-work-item-status
description: Inspect Axiom Work Item workflow status and Evidence through Lingo.
---

# Show Axiom Work Item Status

Collect missing Project and Work Item selectors. Run `lingo workflow status` and,
when requested, `lingo workflow evidence`, through `lingo --json` with `--project
<selector> --repository <key> --number <number>`. Report the structured current
gate, steps, repository and Evidence digests without changing workflow or
provider state.
