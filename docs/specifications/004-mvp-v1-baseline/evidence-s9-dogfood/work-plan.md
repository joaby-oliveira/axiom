# S9 dogfood work plan — replace the documentation logo

Work Item: [rgomids/axiom#117](https://github.com/rgomids/axiom/issues/117),
created through the published Axiom `v0.1.2-rc.1` GitHub Provider.
Project `axiom-s9-dogfood` (`c888fda0-866d-4bbd-8412-687e7470ce47`), repository
key `main`, Execution `c020ad7a-99b7-4639-a419-d81d5e9e2256`.

This file is the intake, specification, clarification, plan and tasks reference
for the bounded workflow of that Execution. It is not a new Specification or
ADR: the change is a documentation asset replacement with no product behavior.

## Intent

Replace the main documentation logo `docs/assets/axiom-logo-github.png`
(referenced by `README.md` and `docs/README.pt-BR.md`) with the current app
icon `docs/assets/axiom-logo-app-black.png`, making only the light area outside
the icon outline transparent.

## Specification of the change

- Source: `docs/assets/axiom-logo-app-black.png`, 1254×1254 8-bit RGB,
  SHA-256 `23ccdef4e2c64f7a0014bee45e176a14af2dc1b6a8ae623a8cfa099b30b90836`.
  It stays unchanged.
- Target: `docs/assets/axiom-logo-github.png`, replaced in place; README paths
  unchanged.
- Preserve drawing, colours, outline, proportions and resolution. No redesign.
- Result: valid RGBA PNG with transparent outer corners, identical dimensions,
  and every fully opaque pixel identical to the source.

## Clarifications

None were material. The README `width="460"` attributes are not paths and are
left unchanged to keep the change minimal.

## Plan and tasks

1. Deterministic transformation with standard-library Python
   ([transform_logo.py](transform_logo.py)): flood-fill near-white pixels
   (minimum channel ≥ 200) connected to the image border, make them
   transparent, and un-mix a 3-pixel anti-aliased ring against the icon body
   colour so the outline keeps its colour with fractional alpha. No new
   dependency (Pillow/ImageMagick are not installed on the host).
2. Validate with [validate_logo.py](validate_logo.py): PNG CRC/decode, RGBA,
   dimensions, transparent corners, unchanged interior and opaque pixels,
   composite over white reproducing the source.
3. Run repository validators, review the diff, open the PR, wait for CI, review,
   merge when green with no Blocker/Major finding.
4. Record Evidence; after integration, comment and close #117 as dogfood
   completion (not S9 acceptance).

## Authority

The human operator authorized in the session prompt: local Axiom execution with
the Claude Runtime only (no Codex), Project/Work Item/Execution creation, this
Issue with existing labels and comments, branch/commits/push, the implementation
PR and its merge when gates are green, closing this Issue after real integration,
and a final Evidence/reconciliation PR left for human review. Not authorized:
Codex, release promotion or new release, secrets/credentials, infrastructure,
other Issues, retroactive Evidence changes, and human acceptance of S9.
