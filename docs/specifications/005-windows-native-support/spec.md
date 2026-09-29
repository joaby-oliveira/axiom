# Specification 005 — Native Windows Support

## Status

**Proposed.** A maintainer requested native Windows support on 2026-09-29. This
Specification is the reviewable contract for that change; it does not claim that
Windows is supported until its Plan, Tasks, implementation, and native Evidence
are accepted.

## Problem

Axiom's published installer and release artifacts support macOS and Linux only.
Windows users cannot install or run the CLI natively. The current local-storage
implementation depends on POSIX ownership, ACL, link, lock, and atomic-rename
semantics, so merely distributing an `.exe` would create an unsupported and
potentially unsafe state-management path.

## Outcome

Windows 10 or later on amd64 can install one checksummed native Axiom release,
run `axiom.exe`, use `first-run`, and use the same local workflows as the
supported POSIX rows without weakening the repository's fail-closed local-state
contract.

## Scope and non-goals

In scope:

- a `windows/amd64` release artifact and SHA-256 entry;
- an HTTPS-only PowerShell bootstrap that verifies the selected asset before
  extraction and delegates installation to the verified bundle;
- user-local installation, receipt, upgrade, Runtime skill installation, and
  state roots using Windows-native APIs;
- deterministic Windows CI and native acceptance Evidence.

Out of scope: package managers, MSI installers, automatic updates, code signing,
ARM64 Windows, Windows Server support, WSL as the supported route, shared/network
drives, and changing Portable Project Manifest content.

## Functional requirements

- **FR-W01:** The release pipeline MUST build `windows/amd64`, include it in the
  published checksum set, and retain version/revision provenance equivalent to
  other rows.
- **FR-W02:** `scripts/install.ps1` MUST select only `windows/amd64`, resolve the
  same stable/exact-version policy as the POSIX bootstrap, fetch only HTTPS URLs,
  verify the outer archive SHA-256 before extraction, and stop before install on
  ambiguity, mismatch, or unsupported host.
- **FR-W03:** The bundle installer MUST install into an explicit user-local
  binary directory (default `%LOCALAPPDATA%\\Axiom\\bin`) and receipt directory
  (default `%LOCALAPPDATA%\\Axiom\\install`), never elevate privileges, edit
  `PATH`, install Runtimes, or access credentials.
- **FR-W04:** Machine-local state MUST default to `%LOCALAPPDATA%\\Axiom\\state`.
  Portable manifests and their schema remain platform-neutral.
- **FR-W05:** Before a Windows local mutation, Axiom MUST reject reparse points,
  unavailable owner or DACL information, non-user-owned roots, writable access by
  a non-owner principal, changed object identities, concurrent writers, and
  incomplete/ambiguous prior operations. It MUST report recovery rather than
  overwrite uncertain state.
- **FR-W06:** Axiom MUST preserve the existing verified owned reinstall, upgrade,
  downgrade refusal, foreign-target refusal, and confirmed/partial-effect
  semantics on Windows.

## Acceptance criteria

- **AC-W01:** A clean Windows 10 amd64 account installs the latest stable and an
  exact version via PowerShell; both report the expected `axiom.exe version`.
- **AC-W02:** Tampered archives, missing/duplicate checksum entries, unsupported
  architecture, HTTP redirects, malicious ZIP paths, and invalid release metadata
  are refused before a target mutation.
- **AC-W03:** The Windows DACL/reparse-point/object-identity/concurrent-writer
  matrix is deterministic and fail-closed.
- **AC-W04:** A verified owned reinstall is a no-op; an older, altered, foreign,
  or recovery-marked installation is not silently replaced.
- **AC-W05:** `first-run`, Project configuration, workflow persistence, Runtime
  skill installation, recovery, and upgrade pass on native Windows and preserve
  the existing portable/local boundary.
- **AC-W06:** Linux and macOS release artifacts, installers, and tests retain
  their existing behavior.

## Constitution check

This change adds a public distribution and machine-local security boundary.
It preserves explicit authority, least privilege, no credential provisioning,
portable/local separation, deterministic validation, and separate human
acceptance. ADR-0010 is required before implementation because Windows security
identities and publication primitives are not interchangeable with POSIX modes.
