# S9 polish findings — observed during T36

No S9 implementation performed.

## 1

Observação: CLI does not expose Model Profile or Command Profile creation

Passo: Runtime setup

Impacto: Operator must compose typed Go APIs and persist RuntimeProfileStore configuration.

Evidence: onboarding.json; command-profiles.json; artifacts/runner/main.go

Melhoria possível: Provide bounded setup and inspection CLI with existing invariants.

## 2

Observação: first-run checks only Codex skill compatibility

Passo: first-run before and after profile setup

Impacto: Success does not prove Claude authentication, model resolution, or Project setup.

Evidence: onboarding.json

Melhoria possível: Report separate readiness dimensions.

## 3

Observação: Portable Project catalog must be separate from sample repository directory

Passo: project configure

Impacto: Pre-existing permissive directory or repository at slug produces generic setup inspection failure.

Evidence: onboarding.json

Melhoria possível: Document protected catalog root and surface actionable diagnostics.

## 4

Observação: Claude argv variadic --tools consumed prompt

Passo: Runtime Command Profile probe

Impacto: Initial probe failed before authentication or model invocation.

Evidence: artifacts/runner/main.go

Melhoria possível: Render supported argv with explicit option delimiter.

## 5

Observação: Claude macOS auth depends on identity environment beyond HOME

Passo: Runtime probe under explicit allowlist

Impacto: HOME/PATH-only environment reports no login; USER/LOGNAME/SHELL restore existing auth without credential access.

Evidence: runtime-probes.json

Melhoria possível: Document required runtime environment and distinguish discovery from authentication.

## 6

Observação: S8 operator workflow requires custom Go composition

Passo: Graph preparation and dispatch

Impacto: No packaged CLI for graph publication, dispatch, coordination or Evidence export.

Evidence: artifacts/runner/main.go; run-envelope.json

Melhoria possível: Add operator commands in separately authorized polish work.

## 7

Observação: Captured Runtime output reference is digest-only

Passo: OSProcessRunner and scheduler

Impacto: Process output is discarded after hashing; troubleshooting cannot dereference process-output hash from persisted state.

Evidence: source/axiom/internal/executiongraph/scheduler.go

Melhoria possível: Provide bounded sanitized output artifact storage with verified digest and no raw reasoning.


Evidence names in these observations refer to retained operator lab artifacts.
No S9 work is implemented by this record.
