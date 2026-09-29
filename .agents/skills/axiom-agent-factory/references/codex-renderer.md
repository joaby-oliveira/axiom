# Codex Renderer

## Role

Transform a validated Axiom agent blueprint into a Codex-oriented repository harness.

## Base structure

Prefer:

```text
AGENTS.md
.agents/
└── skills/
    └── <skill>/
        ├── SKILL.md
        └── references/   # only when needed
```

Codex-specific configuration may be introduced when there is a concrete runtime requirement. Do not add configuration merely to make the tree look complete.

## `AGENTS.md`

It should contain:

- mission;
- short durable global rules;
- source hierarchy;
- routing to skills/context/policies;
- definition of done;
- safety constraints.

It should not contain every procedural workflow.

## Skills

Create a skill when a procedure has:

- contextual activation;
- multiple steps;
- inputs;
- output;
- validation;
- meaningful failure modes.

A skill should include:

- purpose;
- when to use;
- when not to use;
- inputs/preconditions;
- procedure;
- validation;
- expected output.

## Progressive disclosure

Do not force every supporting reference into the initial prompt.

Use:

```text
AGENTS.md
→ relevant SKILL.md
→ references when required
→ scripts/templates as needed
```

## Isolation

A Codex-only package must not accidentally contain:

```text
CLAUDE.md
.claude/
```

unless the user explicitly asks for a multi-target package.

The renderer never emits `CLAUDE.md` for a Codex-only package.
`scripts/validate-agent-package.sh` tolerates only the exact root bootstrap
`CLAUDE.md` containing `@AGENTS.md` (the repository's own maintainer bootstrap,
see `AGENTS.md`); any other Claude artifact fails validation.

## Conservative defaults

Do not silently:

- expand sandbox permissions;
- enable unrestricted network;
- add hooks;
- add external MCP dependencies;
- embed credentials;
- execute destructive setup.

## Validation

The generated harness should be understandable by opening `AGENTS.md` alone and following its routing.
