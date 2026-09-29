# Contribution Policy

## Core rule

The Axiom repository agent is a contributor and MUST follow the repository
contribution workflow.

The canonical workflow lives in [CONTRIBUTING.md](../../CONTRIBUTING.md). The
agent must also use the real [Pull Request template](../../.github/PULL_REQUEST_TEMPLATE.md),
follow [documentation governance](../../docs/documentation.md), and apply the
[security policy](security.md). This policy defines agent behavior; it does not
duplicate those sources.

## Required behavior

For every contribution, the agent must:

- use the current branch and commit conventions;
- keep one small, coherent scope and exclude unrelated changes;
- respect Specifications, ADRs, explicit authority, and Task/Slice boundaries;
- run applicable repository validations and record reproducible Evidence from
  commands actually executed;
- reconcile affected documentation;
- assess security impact and state known limitations;
- preserve human approval boundaries.

Before opening a Pull Request, inspect the branch, complete diff, commits, PR
title and description, related references, Evidence, documentation impact,
security impact, limitations, and absence of out-of-scope changes.

For an Axiom-managed Work Item under the approved Issue #94 amendment, the agent
must also:

- preserve Repository artifacts as technical truth, local Execution/workflow as
  canonical workflow truth, and Provider state as projection/recovery signal;
- validate exact current/target canonical Execution gate, current/expected derived
  lifecycle stage, revision, prerequisites, references, blockers, and authority
  before any local transition;
- keep blocked/decision/approval/recovery conditions as auxiliary flags rather
  than inventing stage combinations;
- treat merge, CI, review, Issue closure, and Provider metadata as non-authority
  for workflow advance or human acceptance;
- keep Provider history bounded, idempotent, and reference-first;
- resolve Project metadata policy before asking, and keep provider-specific fields
  inside the adapter;
- return `recovery_required` for missing, insufficient, or contradictory local
  truth instead of reconstructing an Execution or inferring progress.

These rules govern the approved amendment, but do not by themselves authorize
T26–T29 implementation. S6 implementation remains separately gated and starts only
after explicit authorization.

When opening a Pull Request, use the repository template and replace every
placeholder with truthful content or an objective not-applicable explanation.

## Prohibited behavior

The agent must not:

- push directly to `main`;
- open an empty or context-free Pull Request;
- use generic final commit messages such as `update`, `changes`, `fix`, or
  `wip`;
- include unrelated changes;
- treat green CI as sufficient Evidence;
- treat technical merge as human acceptance;
- advance or accept a Work Item from Provider labels, Issue/PR state, or green CI;
- reconstruct missing local workflow truth from Provider history or Repository
  artifacts;
- advance to another Task or Slice without explicit authority;
- merge its own Pull Request without explicit human authorization.

Future changes to the canonical contribution sources apply automatically. When
this policy conflicts with them, stop and reconcile the conflict instead of
inventing a parallel workflow.
