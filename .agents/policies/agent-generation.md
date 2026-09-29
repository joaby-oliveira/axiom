# Agent Generation Policy

## Core rule

Agent generation must follow:

```text
anamnese/context
→ blueprint
→ artifact plan
→ renderer
→ validation
→ package
```

Do not render directly from an ambiguous request.

## Blueprint

The normalized blueprint should describe concepts such as:

- purpose;
- project context;
- target runtime;
- repository mode;
- constraints;
- workflows;
- skills;
- specialist roles;
- policies;
- providers/capabilities;
- validation commands;
- definition of done;
- risk/approval model;
- package scope.

Keep it vendor-neutral whenever possible.

## Codex renderer

For native Codex packages:

- `AGENTS.md` is the main entrypoint;
- reusable workflows belong under `.agents/skills/<skill>/SKILL.md`;
- avoid Claude-specific files in Codex-only packages (the repository's own
  maintainer bootstrap is governed by `AGENTS.md`, not by this renderer rule);
- do not enable experimental permissions/hooks automatically;
- keep entrypoint concise;
- avoid duplicating the same rules across multiple files.

## Generation quality

Generated packages must:

- contain complete useful files;
- have no vague placeholders;
- have no secrets;
- contain no unrelated vendor artifacts;
- expose assumptions;
- be internally consistent;
- be extractable at the intended repository root;
- include deterministic validation when feasible.

## Progressive disclosure

Prefer:

```text
AGENTS.md metadata/routing
→ SKILL.md workflow
→ references only when needed
→ scripts/templates only when needed
```

This reduces context and token waste.
