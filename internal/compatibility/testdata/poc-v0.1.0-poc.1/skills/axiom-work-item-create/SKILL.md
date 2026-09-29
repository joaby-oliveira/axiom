---
name: axiom-work-item-create
description: Create or select a GitHub-backed Axiom Work Item through Lingo.
---

# Create Axiom Work Item

Collect missing Project and Work Item inputs. Before an external mutation, obtain
explicit authority required by Lingo. Run `lingo --json work-item create
--project <selector> --repository <key> --title <title> --body <body>
--authorize-external`, or select through `lingo --json work-item select
--project <selector> --repository <key> --number <number>`. Report the structured
result. Do not call GitHub directly.
