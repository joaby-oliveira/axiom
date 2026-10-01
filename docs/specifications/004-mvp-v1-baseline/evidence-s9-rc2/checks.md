# Deterministic checks — T24 rc.2 preparation (2026-09-29)

Host: macOS 27.0 (26A428) / arm64, go1.26.1, Python 3.14.4. Worktree HEAD
`859969a07f3807822580431a05b6c78b07691fb1` plus the tooling/documentation
changes of this checkpoint; no product Go file changed. These are local checks,
not published-binary black-box or real Runtime/Provider acceptance.

| Command | Exit |
|---|---|
| `go test ./... -count=1` | 0 |
| `go test -race ./...` | 0 |
| `go vet ./...` | 0 |
| `go build ./...` | 0 |
| `go mod verify` | 0 |
| `go vet scripts/s9-graph-runner.go` | 0 |
| `bash -n scripts/test-s9-rc-acceptance.sh` | 0 |
| `bash -n scripts/test-s7-native.sh` | 0 |
| `python3 -B scripts/test_s9_rc_evidence.py` (17 tests) | 0 |
| `python3 -B scripts/test_s9_rc_envelope.py` (9 tests) | 0 |
| `python3 -B docs/specifications/004-mvp-v1-baseline/evidence-s9-rc/audit.py` (structural) | 0 |
| `./scripts/validate-repository.sh .` | 0 |
| `./scripts/check-sensitive-files.sh .` | 0 |
| `gitleaks dir . --redact --no-banner` | 0 (no leaks) |
| `git diff --check` | 0 |

Read-only remote checks: `./scripts/release.sh verify --tag v0.1.2-rc.2 --download`
(exit 0, [output](release-verification.txt)); `gh api` release/tag/latest/run
reads and `git ls-remote` (all exit 0, recorded beside this file).

Plan-only generation: `test-s9-rc-acceptance.sh … --evidence-dir <lab>/evidence/A-macos-arm64`
(exit 0, no process started) and `s9-rc-envelope.py --plan` for B–F (exit 0).

Not run: `scripts/test-s7-native.sh` (the native suite on this row was not
part of this preparation; Linux rows unavailable), any `--execute-install`, any
envelope phase, any Runtime or Provider call.
