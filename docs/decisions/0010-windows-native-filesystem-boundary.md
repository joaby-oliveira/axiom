# ADR-0010 — Windows native filesystem boundary

## Status

**Proposed.**

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
user storage on Windows 10+ amd64 until native Evidence expands that matrix.

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
security and file-identity code, a ZIP/PowerShell distribution path, and Windows
CI increase maintenance and native-test cost. Existing Windows state is not
adopted or migrated automatically.

## Revisit when

The supported filesystem matrix, code-signing topology, Windows ARM64, network
storage, or a requirement to adopt pre-existing Windows installations changes.
