# Plan — Native Windows Support

## Status

**Proposed.** Depends on acceptance of Specification 005 and ADR-0010.

## Delivery plan

1. Define Windows filesystem adapters behind the existing local, install,
   Runtime, and Git-workspace boundaries. Use Win32 handles to reject reparse
   points, compare volume serial plus file ID, inspect owner/DACL, acquire an
   exclusive lock, and publish a replacement only after all checks succeed.
2. Add Windows-native state-root, identity, free-space, locking, no-replace, and
   recovery implementations. POSIX implementations and their supported
   guarantees remain unchanged.
3. Add ZIP bundle build/verification and `install.ps1` bootstrap/bundle
   installers. The bootstrap validates SHA-256 before extraction; bundle
   installation uses the same owned-install receipt protocol.
4. Extend release checks and CI with a Windows amd64 compile/test job and a
   release-artifact contract row. Native Windows acceptance runs are required
   before a release claims Windows support.
5. Update both README variants and command reference only after the delivered
   path is verified. Until then documentation MUST continue to state the current
   supported rows.

## Test plan

Unit tests cover path canonicalization, reparse-point refusal, DACL/owner
classification, lock contention, file identity changes, ZIP validation, checksum
selection, receipt/upgrade matrices, and Windows state-root selection. Integration
tests cover clean install, no-op reinstall, changed binary, downgrade, interrupted
publication/recovery, `first-run`, and a complete local Project/workflow journey.

## Rollout and rollback

Windows is an additive release row. A failed Windows artifact blocks that release
row rather than substituting a POSIX binary or a WSL path. Removing a future row
requires a new documented compatibility decision; it MUST NOT delete user state.

## Architecture check

The plan retains domain and portable-manifest packages. OS-dependent checks stay
at local filesystem, distribution, Runtime filesystem, and Git-workspace adapter
boundaries; no Windows API reaches domain or provider code.
