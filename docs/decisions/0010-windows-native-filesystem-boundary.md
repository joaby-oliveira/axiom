# ADR-0010 — Windows native filesystem boundary

## Status

**Accepted for the requested implementation** on 2026-09-30. Upstream acceptance
remains subject to PR review; this is not release-publication authority.

## Context

ADR-0005's implemented checks use POSIX ownership, modes, ACL inspection,
directory descriptors, hard-link counts, and atomic rename primitives. Windows
does not provide equivalent semantics through those APIs. Treating unavailable
checks as safe would violate the local filesystem threat model.

## Decision

Add a Windows-specific filesystem adapter for supported local mutation paths. It
will use Windows handles and security descriptors to prove the authorized object,
reject reparse points and uncertain DACL/owner state, and maintain explicit lock,
staging, commit, and recovery behavior. Windows support is limited to local NTFS
user storage on Windows 10 (1809+) and Windows 11 amd64 until native Evidence
expands that matrix.

Private objects require the current token's ownership identity and reject access by
other untrusted principals; SYSTEM, Administrators and TrustedInstaller remain
trusted like root on POSIX. Normally the owner is the user's SID; an elevated
token's Administrators default-owner SID is also accepted, only when it is that
token's actual default owner. This supports native elevated/CI tokens without
adopting objects owned by another ordinary account. Ancestors can allow traversal/create access, but not
replacement of existing children. Reparse points and multi-link files fail closed.

Directory operations are serialized with a global kernel object named by user
SID, volume serial and file ID. Its last handle disappearing releases the lock
after a process exits; no thread-affine mutex ownership or persistent sidecar is
used. Readers are conservatively serialized too. This avoids Windows directory
share-mode locks blocking the holder's own atomic renames. Runtime skill locks
use byte-range locks on the existing protocol lock file.

Windows shares the `.tar.gz` release format, extracted by the OS-provided
`tar.exe`; introducing ZIP would duplicate the already-verified Go bundle loader.
PowerShell runs the verified staged executable to publish `axiom.exe`, so the
installer does not replace itself. Files are flushed and published atomically;
directory fsync equivalence and physical power-loss durability are not claimed.

## Alternatives considered

1. Ship only a PowerShell downloader. Rejected: it distributes a binary that
   cannot safely run the local workflows.
2. Treat Windows permission metadata as equivalent to POSIX modes. Rejected:
   this is neither semantically correct nor fail-closed.
3. Support Windows through WSL. Rejected: it is not native Windows support and
   changes the supported host and storage boundary.

## Consequences

Positive: Windows users gain a native, checksummed path without silently
weakening local-state protections; POSIX behavior stays isolated. Negative: Win32
security and file-identity code, a PowerShell distribution path, and Windows
CI increase maintenance and native-test cost. Existing Windows state is not
adopted or migrated automatically.

## Revisit when

The supported filesystem matrix, code-signing topology, Windows ARM64, network
storage, or a requirement to adopt pre-existing Windows installations changes.
