# Tasks — Native Windows Support

## Status

**Proposed.** Implementation requires explicit approval after Specification 005,
this Task decomposition, and ADR-0010 review.

| Task | Objective | Evidence |
|---|---|---|
| W1 | Introduce a Windows filesystem-security adapter preserving anchored path, DACL, reparse-point, identity, lock, publication, and recovery invariants. | Windows unit/fault/concurrency matrix; POSIX regression suite. |
| W2 | Port install, Runtime skill, Git-workspace, and local-state callers to the adapter without changing Portable Manifest formats. | Native Project, first-run, upgrade, and workflow integration tests. |
| W3 | Produce deterministic Windows ZIP artifacts and PowerShell bootstrap/bundle installation with checksum verification. | Artifact contents/provenance, negative installer matrix, clean-account install. |
| W4 | Add Windows CI, native acceptance Evidence, and user documentation after successful verification. | CI logs, clean Windows journey, docs/link review, security review. |

Each task may be independently reviewed, but W3 and W4 depend on W1 and W2.
