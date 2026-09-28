#!/usr/bin/env bash
set -euo pipefail

umask 077

repository_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)
temporary=$(mktemp -d)
temporary=$(cd "$temporary" && pwd -P)
trap 'rm -rf -- "$temporary"' EXIT

digest_file() {
  shasum -a 256 "$1" | awk '{print $1}'
}

file_mode() {
  if [[ $(uname -s) == Darwin ]]; then stat -f %Lp "$1"; else stat -c %a "$1"; fi
}

file_owner() {
  if [[ $(uname -s) == Darwin ]]; then stat -f %u "$1"; else stat -c %u "$1"; fi
}

file_links() {
  if [[ $(uname -s) == Darwin ]]; then stat -f %l "$1"; else stat -c %h "$1"; fi
}

acl_absent() {
  local path=$1 mode
  case "$(uname -s)" in
    Darwin)
      [[ $(LC_ALL=C ls -lde "$path" | wc -l | tr -d ' ') == 1 ]]
      ;;
    Linux)
      mode=$(LC_ALL=C ls -ld "$path" | awk '{print $1}') || return 1
      [[ "$mode" =~ ^[-d][rwx-]{9}$ ]]
      ;;
    *) return 1 ;;
  esac
}

for invalid_version in 01.2.3 1.2.3. 1.2.3-01; do
  if "$repository_root/scripts/build-release-archives.sh" --version "$invalid_version" --output "$temporary/invalid-$invalid_version" --development >/dev/null 2>&1; then
    exit 1
  fi
done

"$repository_root/scripts/build-release-archives.sh" --version 0.0.0-s2-test --output "$temporary/release" --development >/dev/null

[[ $(find "$temporary/release" -name '*.tar.gz' -type f | wc -l | tr -d ' ') == 3 ]]
[[ $(wc -l <"$temporary/release/SHA256SUMS" | tr -d ' ') == 3 ]]
while read -r hash archive; do
  [[ "$hash" =~ ^[0-9a-f]{64}$ ]]
  [[ -f "$temporary/release/$archive" ]]
  actual=$(shasum -a 256 "$temporary/release/$archive" | awk '{print $1}')
  [[ "$actual" == "$hash" ]]
  listing=$(tar -tzf "$temporary/release/$archive")
  for expected in /axiom /LICENSE /install.sh /release-metadata.txt /skills-manifest.txt /MANIFEST.sha256; do
    grep -Fq "$expected" <<<"$listing"
  done
  # T37: the canonical public executable is axiom; no lingo entry is shipped.
  if grep -Eq '/lingo$' <<<"$listing"; then exit 1; fi
  [[ $(tar -tvzf "$temporary/release/$archive" | awk '$NF ~ /\/axiom$/ {print substr($1,1,4)}') == -rwx ]]
  grep -Eq '^[0-9a-f]{64}  axiom$' <(tar -xOzf "$temporary/release/$archive" "${archive%.tar.gz}/MANIFEST.sha256")
  [[ $(grep -c '/skills/.*/SKILL.md' <<<"$listing") == 5 ]]
done <"$temporary/release/SHA256SUMS"

# A release build from a clean clone of this revision carries exact version
# and revision provenance, reported by the archived executable when invoked
# directly as axiom (host architecture only; no installation involved).
host_bundle=
case "$(uname -s):$(uname -m)" in
  Darwin:arm64) host_bundle=macos-27-arm64 ;;
  Linux:x86_64) host_bundle=ubuntu-26.04-amd64 ;;
  Linux:aarch64) host_bundle=ubuntu-26.04-arm64 ;;
esac
if [[ -n "$host_bundle" ]]; then
  git clone -q --no-hardlinks "$repository_root" "$temporary/clean-source"
  git -C "$temporary/clean-source" checkout -q --detach "$(git -C "$repository_root" rev-parse --verify HEAD)"
  "$temporary/clean-source/scripts/build-release-archives.sh" --version 0.0.0-rc.1 --output "$temporary/clean-release" >/dev/null
  mkdir -p "$temporary/provenance"
  tar -xzf "$temporary/clean-release/axiom-0.0.0-rc.1-$host_bundle.tar.gz" -C "$temporary/provenance"
  expected_revision=$(git -C "$repository_root" rev-parse --verify HEAD | cut -c1-12)
  (cd / && "$temporary/provenance/axiom-0.0.0-rc.1-$host_bundle/axiom" --json version) >"$temporary/provenance.json"
  grep -Fq '"provenance":{"product":"Axiom","version":"0.0.0-rc.1","revision":"'"$expected_revision"'","sourceState":"clean"}' "$temporary/provenance.json"
fi

native=
host_candidate=
case "$(uname -s):$(uname -m)" in
  Darwin:arm64)
    host_candidate="$temporary/release/axiom-0.0.0-s2-test-macos-27-arm64.tar.gz"
    [[ $(sw_vers -productVersion 2>/dev/null) == 27.0 ]] && native=$host_candidate
    ;;
  Linux:x86_64)
    host_candidate="$temporary/release/axiom-0.0.0-s2-test-ubuntu-26.04-amd64.tar.gz"
    if [[ -r /etc/os-release ]] && grep -Eq '^ID=(ubuntu|"ubuntu")$' /etc/os-release && grep -Eq '^VERSION_ID=(26\.04|"26\.04")$' /etc/os-release; then native=$host_candidate; fi
    ;;
  Linux:aarch64)
    host_candidate="$temporary/release/axiom-0.0.0-s2-test-ubuntu-26.04-arm64.tar.gz"
    if [[ -r /etc/os-release ]] && grep -Eq '^ID=(ubuntu|"ubuntu")$' /etc/os-release && grep -Eq '^VERSION_ID=(26\.04|"26\.04")$' /etc/os-release; then native=$host_candidate; fi
    ;;
esac

if [[ -n "$host_candidate" && -z "$native" ]]; then
  if "$repository_root/scripts/install-release.sh" --archive "$host_candidate" --checksums "$temporary/release/SHA256SUMS" --bin-dir "$temporary/unsupported-bin" --receipt-dir "$temporary/unsupported-receipt" >/dev/null 2>"$temporary/unsupported.stderr"; then
    exit 1
  fi
  grep -Fq 'exact approved OS, version, distribution, and architecture required' "$temporary/unsupported.stderr"
  [[ ! -e "$temporary/unsupported-bin/axiom" ]]
fi

if [[ -n "$native" ]]; then
  "$repository_root/scripts/install-release.sh" --archive "$native" --checksums "$temporary/release/SHA256SUMS" --bin-dir "$temporary/bin" --receipt-dir "$temporary/receipt" | grep -Fq 'install_status=installed'
  installed_at=$(awk -F= '$1 == "installedAt" {print $2}' "$temporary/receipt/installation.receipt")
  [[ "$installed_at" =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z$ ]]
  "$temporary/bin/axiom" --json version | grep -Fq '"version":"development"'
  if AXIOM_CODEX_SKILLS_ROOT="$temporary/runtime-skills" "$temporary/bin/axiom" --json first-run >"$temporary/first-run.json"; then
    exit 1
  fi
  grep -Fq '"status":"validation_failure"' "$temporary/first-run.json"
  [[ $(grep -o '"state":"missing"' "$temporary/first-run.json" | wc -l | tr -d ' ') == 5 ]]
  mkdir -p "$temporary/native-bundle"
  tar -xzf "$native" -C "$temporary/native-bundle"
  archived_skills_manifest=$(find "$temporary/native-bundle" -name skills-manifest.txt -type f)
  skill_count=0
  while IFS='=' read -r key hash; do
    case "$key" in
      skill.*)
        name=${key#skill.}
        grep -Fq '"name":"'"$name"'","sha256":"'"$hash"'","state":"missing"' "$temporary/first-run.json"
        skill_count=$((skill_count + 1))
        ;;
    esac
  done <"$archived_skills_manifest"
  [[ "$skill_count" == 5 ]]

  AXIOM_CODEX_SKILLS_ROOT="$temporary/runtime-skills" "$temporary/bin/axiom" --json runtime codex install >"$temporary/runtime-install.json"
  grep -Fq '"status":"success","category":"codex_configured"' "$temporary/runtime-install.json"
  [[ $(grep -o '"state":"equivalent"' "$temporary/runtime-install.json" | wc -l | tr -d ' ') == 5 ]]
  [[ $(find "$temporary/runtime-skills" -mindepth 1 -maxdepth 1 -type d | wc -l | tr -d ' ') == 5 ]]
  while IFS='=' read -r key hash; do
    case "$key" in
      skill.*)
        name=${key#skill.}
        skill_file="$temporary/runtime-skills/$name/SKILL.md"
        [[ -f "$skill_file" && ! -L "$skill_file" && $(digest_file "$skill_file") == "$hash" ]]
        grep -Fq '"name":"'"$name"'","sha256":"'"$hash"'","state":"equivalent"' "$temporary/runtime-install.json"
        ;;
    esac
  done <"$archived_skills_manifest"

  AXIOM_CODEX_SKILLS_ROOT="$temporary/runtime-skills" "$temporary/bin/axiom" --json runtime codex status >"$temporary/runtime-status.json"
  grep -Fq '"status":"success","result":"Lingo and Codex skills are compatible"' "$temporary/runtime-status.json"
  [[ $(grep -o '"state":"equivalent"' "$temporary/runtime-status.json" | wc -l | tr -d ' ') == 5 ]]

  runtime_before=$(find "$temporary/runtime-skills" -type f -print | LC_ALL=C sort | while IFS= read -r file; do printf '%s  %s\n' "$(digest_file "$file")" "${file#"$temporary/runtime-skills/"}"; done)
  AXIOM_CODEX_SKILLS_ROOT="$temporary/runtime-skills" "$temporary/bin/axiom" --json runtime codex install >"$temporary/runtime-reinstall.json"
  grep -Fq '"status":"success","category":"codex_already_configured"' "$temporary/runtime-reinstall.json"
  [[ $(grep -o '"state":"equivalent"' "$temporary/runtime-reinstall.json" | wc -l | tr -d ' ') == 5 ]]
  runtime_after=$(find "$temporary/runtime-skills" -type f -print | LC_ALL=C sort | while IFS= read -r file; do printf '%s  %s\n' "$(digest_file "$file")" "${file#"$temporary/runtime-skills/"}"; done)
  [[ "$runtime_after" == "$runtime_before" ]]

  while IFS= read -r directory; do
    [[ ! -L "$directory" && $(file_mode "$directory") == 700 && $(file_owner "$directory") == "$(id -u)" ]]
    acl_absent "$directory"
  done < <(find "$temporary/runtime-skills" -type d | LC_ALL=C sort)
  while IFS= read -r file; do
    [[ -f "$file" && ! -L "$file" && $(file_mode "$file") == 600 && $(file_owner "$file") == "$(id -u)" && $(file_links "$file") == 1 ]]
    acl_absent "$file"
  done < <(find "$temporary/runtime-skills" -type f | LC_ALL=C sort)

  mkdir -p "$temporary/foreign-skills/axiom-project-configure"
  printf 'foreign skill content\n' >"$temporary/foreign-skills/axiom-project-configure/SKILL.md"
  chmod 700 "$temporary/foreign-skills" "$temporary/foreign-skills/axiom-project-configure"
  chmod 600 "$temporary/foreign-skills/axiom-project-configure/SKILL.md"
  foreign_before=$(digest_file "$temporary/foreign-skills/axiom-project-configure/SKILL.md")
  if AXIOM_CODEX_SKILLS_ROOT="$temporary/foreign-skills" "$temporary/bin/axiom" --json runtime codex install >"$temporary/foreign-install.json"; then
    exit 1
  fi
  grep -Fq '"status":"error","category":"codex_skill_conflict"' "$temporary/foreign-install.json"
  [[ $(digest_file "$temporary/foreign-skills/axiom-project-configure/SKILL.md") == "$foreign_before" ]]

  before=$(shasum -a 256 "$temporary/bin/axiom" | awk '{print $1}')
  sleep 1
  "$repository_root/scripts/install-release.sh" --archive "$native" --checksums "$temporary/release/SHA256SUMS" --bin-dir "$temporary/bin" --receipt-dir "$temporary/receipt" | grep -Fq 'install_status=unchanged'
  [[ $(shasum -a 256 "$temporary/bin/axiom" | awk '{print $1}') == "$before" ]]
  [[ $(awk -F= '$1 == "installedAt" {print $2}' "$temporary/receipt/installation.receipt") == "$installed_at" ]]

  "$repository_root/scripts/install-release.sh" --archive "$native" --checksums "$temporary/release/SHA256SUMS" --bin-dir "$temporary/schema-bin" --receipt-dir "$temporary/schema-receipt" >/dev/null
  printf 'unknown=value\n' >>"$temporary/schema-receipt/installation.receipt"
  if "$repository_root/scripts/install-release.sh" --archive "$native" --checksums "$temporary/release/SHA256SUMS" --bin-dir "$temporary/schema-bin" --receipt-dir "$temporary/schema-receipt" >/dev/null 2>"$temporary/schema.stderr"; then
    exit 1
  fi
  grep -Fq 'invalid receipt schema preserved' "$temporary/schema.stderr"
  grep -Fxq 'unknown=value' "$temporary/schema-receipt/installation.receipt"

  if [[ $(uname -s) == Darwin ]]; then
    mkdir -p "$temporary/wrong-version-tools"
    printf '%s\n' '#!/bin/sh' "printf '%s\\n' '27.1'" >"$temporary/wrong-version-tools/sw_vers"
    chmod 700 "$temporary/wrong-version-tools/sw_vers"
    if PATH="$temporary/wrong-version-tools:$PATH" "$repository_root/scripts/install-release.sh" --archive "$native" --checksums "$temporary/release/SHA256SUMS" --bin-dir "$temporary/wrong-version-bin" --receipt-dir "$temporary/wrong-version-receipt" >/dev/null 2>"$temporary/wrong-version.stderr"; then
      exit 1
    fi
    grep -Fq 'exact approved OS, version, distribution, and architecture required' "$temporary/wrong-version.stderr"
    [[ ! -e "$temporary/wrong-version-bin/axiom" ]]
  fi

  mkdir -p "$temporary/foreign-bin"
  printf 'foreign\n' >"$temporary/foreign-bin/axiom"
  if "$repository_root/scripts/install-release.sh" --archive "$native" --checksums "$temporary/release/SHA256SUMS" --bin-dir "$temporary/foreign-bin" --receipt-dir "$temporary/foreign-receipt" >/dev/null 2>&1; then
    exit 1
  fi
  [[ $(cat "$temporary/foreign-bin/axiom") == foreign ]]

  cp "$native" "$temporary/tampered.tar.gz"
  printf 'tamper\n' >>"$temporary/tampered.tar.gz"
  if "$repository_root/scripts/install-release.sh" --archive "$temporary/tampered.tar.gz" --checksums "$temporary/release/SHA256SUMS" --bin-dir "$temporary/tampered-bin" --receipt-dir "$temporary/tampered-receipt" >/dev/null 2>&1; then
    exit 1
  fi
  [[ ! -e "$temporary/tampered-bin/axiom" ]]

  wrong_platform=$(find "$temporary/release" -name '*.tar.gz' -type f ! -path "$native" | LC_ALL=C sort | head -n 1)
  if "$repository_root/scripts/install-release.sh" --archive "$wrong_platform" --checksums "$temporary/release/SHA256SUMS" --bin-dir "$temporary/wrong-bin" --receipt-dir "$temporary/wrong-receipt" >/dev/null 2>&1; then
    exit 1
  fi
  [[ ! -e "$temporary/wrong-bin/axiom" ]]

  mkdir -p "$temporary/real-bin"
  ln -s "$temporary/real-bin" "$temporary/symlink-bin"
  if "$repository_root/scripts/install-release.sh" --archive "$native" --checksums "$temporary/release/SHA256SUMS" --bin-dir "$temporary/symlink-bin" --receipt-dir "$temporary/symlink-receipt" >/dev/null 2>&1; then
    exit 1
  fi
  [[ ! -e "$temporary/real-bin/axiom" ]]

  mkdir -p "$temporary/permissive-bin" "$temporary/permissive-receipt"
  chmod 770 "$temporary/permissive-bin"
  if "$repository_root/scripts/install-release.sh" --archive "$native" --checksums "$temporary/release/SHA256SUMS" --bin-dir "$temporary/permissive-bin" --receipt-dir "$temporary/permissive-receipt" >/dev/null 2>"$temporary/permissive.stderr"; then
    exit 1
  fi
  grep -Fq 'unsafe destination ownership, permissions, ACL, or type' "$temporary/permissive.stderr"
  [[ $(file_mode "$temporary/permissive-bin") == 770 ]]
  [[ ! -e "$temporary/permissive-bin/axiom" ]]

  if [[ $(uname -s) == Darwin ]]; then
    mkdir -p "$temporary/acl-bin" "$temporary/acl-receipt"
    chmod 700 "$temporary/acl-bin" "$temporary/acl-receipt"
    chmod +a 'everyone allow read,search' "$temporary/acl-bin"
    if "$repository_root/scripts/install-release.sh" --archive "$native" --checksums "$temporary/release/SHA256SUMS" --bin-dir "$temporary/acl-bin" --receipt-dir "$temporary/acl-receipt" >/dev/null 2>"$temporary/acl.stderr"; then
      exit 1
    fi
    grep -Fq 'unsafe destination ownership, permissions, ACL, or type' "$temporary/acl.stderr"
    [[ ! -e "$temporary/acl-bin/axiom" ]]
  fi

  "$repository_root/scripts/install-release.sh" --archive "$native" --checksums "$temporary/release/SHA256SUMS" --bin-dir "$temporary/hardlink-bin" --receipt-dir "$temporary/hardlink-receipt" >/dev/null
  ln "$temporary/hardlink-bin/axiom" "$temporary/hardlink-copy"
  if "$repository_root/scripts/install-release.sh" --archive "$native" --checksums "$temporary/release/SHA256SUMS" --bin-dir "$temporary/hardlink-bin" --receipt-dir "$temporary/hardlink-receipt" >/dev/null 2>&1; then
    exit 1
  fi
  [[ -f "$temporary/hardlink-bin/axiom" && -f "$temporary/hardlink-copy" ]]

  "$repository_root/scripts/install-release.sh" --archive "$native" --checksums "$temporary/release/SHA256SUMS" --bin-dir "$temporary/receipt-link-bin" --receipt-dir "$temporary/receipt-link-state" >/dev/null
  ln "$temporary/receipt-link-state/installation.receipt" "$temporary/receipt-link-copy"
  if "$repository_root/scripts/install-release.sh" --archive "$native" --checksums "$temporary/release/SHA256SUMS" --bin-dir "$temporary/receipt-link-bin" --receipt-dir "$temporary/receipt-link-state" >/dev/null 2>&1; then
    exit 1
  fi
  [[ -f "$temporary/receipt-link-state/installation.receipt" && -f "$temporary/receipt-link-copy" ]]

  "$repository_root/scripts/install-release.sh" --archive "$native" --checksums "$temporary/release/SHA256SUMS" --bin-dir "$temporary/mode-bin" --receipt-dir "$temporary/mode-receipt" >/dev/null
  chmod 755 "$temporary/mode-bin/axiom"
  if "$repository_root/scripts/install-release.sh" --archive "$native" --checksums "$temporary/release/SHA256SUMS" --bin-dir "$temporary/mode-bin" --receipt-dir "$temporary/mode-receipt" >/dev/null 2>&1; then
    exit 1
  fi
  [[ $(file_mode "$temporary/mode-bin/axiom") == 755 ]]

  if AXIOM_INSTALL_TEST_FAIL_STAGE=before_binary "$repository_root/scripts/install-release.sh" --archive "$native" --checksums "$temporary/release/SHA256SUMS" --bin-dir "$temporary/pre-bin" --receipt-dir "$temporary/pre-receipt" >/dev/null 2>&1; then
    exit 1
  fi
  [[ ! -e "$temporary/pre-bin/axiom" ]]
  [[ ! -e "$temporary/pre-receipt/installation.receipt" ]]
  [[ ! -e "$temporary/pre-receipt/.axiom-install-operation" ]]

  if AXIOM_INSTALL_TEST_FAIL_STAGE=after_binary "$repository_root/scripts/install-release.sh" --archive "$native" --checksums "$temporary/release/SHA256SUMS" --bin-dir "$temporary/post-bin" --receipt-dir "$temporary/post-receipt" >/dev/null 2>&1; then
    exit 1
  fi
  [[ -f "$temporary/post-bin/axiom" ]]
  [[ ! -e "$temporary/post-receipt/installation.receipt" ]]
  grep -Fq 'stage=binary_committed' "$temporary/post-receipt/.axiom-install-operation"
  if "$repository_root/scripts/install-release.sh" --archive "$native" --checksums "$temporary/release/SHA256SUMS" --bin-dir "$temporary/post-bin" --receipt-dir "$temporary/post-receipt" >/dev/null 2>"$temporary/post-retry.stderr"; then
    exit 1
  fi
  grep -Fq 'recovery_required' "$temporary/post-retry.stderr"

  mkdir -p "$temporary/locked-receipt/.axiom-install.lock"
  if "$repository_root/scripts/install-release.sh" --archive "$native" --checksums "$temporary/release/SHA256SUMS" --bin-dir "$temporary/locked-bin" --receipt-dir "$temporary/locked-receipt" >/dev/null 2>"$temporary/locked.stderr"; then
    exit 1
  fi
  grep -Fq 'concurrent installation refused' "$temporary/locked.stderr"
  [[ ! -e "$temporary/locked-bin/axiom" ]]
  # A refused installer never releases the lock held by another operation.
  [[ -d "$temporary/locked-receipt/.axiom-install.lock" && ! -e "$temporary/locked-bin" ]]

  [[ $(wc -l <"$temporary/receipt/installation.receipt" | tr -d ' ') == 15 ]]
  for field in formatVersion destination sha256 product version revision sourceState release platform goos architecture skillSetVersion archiveSha256 skillManifestSha256 installedAt; do
    [[ $(grep -c "^${field}=" "$temporary/receipt/installation.receipt") == 1 ]]
  done
fi

printf '%s\n' 'PASS: S2 release archives, ownership, interruption, recovery, and conflict preservation'
