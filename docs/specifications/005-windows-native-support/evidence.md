# Native Windows implementation evidence

## Scope

Implementation for PR #128, requested and authorized on 2026-09-30. This is
implementation/test evidence, not permission to merge or publish a release.

Delivered code includes Windows security-descriptor and handle checks, NTFS and
reparse-point refusal, stable identity, kernel-object directory locks, native
no-replace publication, free-space checks, state-root selection, Runtime skill
and Git-workspace adapters, verified fresh install/reinstall/upgrade, a PowerShell
bootstrap and bundle installer, a fourth release artifact row, and native CI.
Both READMEs document the post-merge interface, not a future specification-only
promise. Older releases without the Windows asset remain uninstallable on Windows.

## Local deterministic results

Native Windows amd64, Go 1.26.0, local NTFS, user-private validation directory:

- `go vet ./...`: passed.
- `GOOS=linux GOARCH=amd64 go build ./...`: passed cross-build; not Linux runtime evidence.
- `go test ./internal/local -count=1 -timeout=3m`: passed, including native ACL,
  junction, hardlink, lock/publication and state-root tests, plus existing
  publication/recovery/concurrency workflows.
- `go test ./internal/install -run TestWindows -count=1 -timeout=60s`: passed
  fresh install, exact reinstall, owned upgrade, foreign binary preservation,
  and interrupted receipt finalization tests.
- Native executable Project configuration and the complete Claude/GitHub-fake
  Work Item/workflow projection regression passed during implementation.
- Execution Graph tests passed after converting synthetic absolute path fixtures
  to host paths.
- PowerShell 5.1 parser accepted the bootstrap.

## Validation limitations / release gates

The local machine's Application Control policy blocks some newly built test and
fixture executables (Windows error 4551), including the offline PowerShell
installer fixture. The policy was not disabled or bypassed. Such runs are not
reported as passes. The Windows CI job runs the complete Go suite and the offline
PowerShell installer contract; Linux/macOS CI retains race and release checks.
Final CI results must be reviewed before merge and before publication.

POSIX chmod/UID and shell-fixture cases remain POSIX-specific. Windows permission
tests inspect real DACLs instead of interpreting Go's synthetic mode bits.
Symlink tests skip only when the OS denies the creation privilege; native junction
refusal is tested independently. Windows may itself prevent an open ancestor from
being renamed, so those POSIX attack fixtures report that prevention explicitly.

No live release was downloaded, published, tagged, or installed into the user's
normal binary directory. Clean-account latest-release acceptance is a release
gate once a release containing the new Windows asset is available. Physical
power-loss durability, remote filesystems, Windows ARM64 and code signing are
outside the approved initial support boundary.

## Security review

- No implicit elevation, policy bypass, persistent PATH edits, credentials or
  Runtime installation.
- Checksummed HTTPS downloads with bounded responses and cancellation, private
  staging, closed Go bundle validation, and foreign-target preservation.
- Atomic staged no-replace fresh publication; existing owned upgrade protocol is
  reused. Interrupted/ambiguous state is preserved rather than adopted silently.
- User/SYSTEM/Administrators trust is explicit; reparse points, alternate streams,
  non-NTFS storage, unsafe DACLs and changed object identities fail closed.
- Native `.exe` Runtime names are supported; shell executable extension aliases
  do not bypass the existing direct-executable restriction.
- LF checkout attributes protect byte-level fixture and embedded-skill digests.
