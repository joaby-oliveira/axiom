#!/usr/bin/env bash
set -euo pipefail

umask 077

archive=
checksums=
binary_root=
receipt_root=
while (($#)); do
  case "$1" in
    --archive) archive=${2:-}; shift 2 ;;
    --checksums) checksums=${2:-}; shift 2 ;;
    --bin-dir) binary_root=${2:-}; shift 2 ;;
    --receipt-dir) receipt_root=${2:-}; shift 2 ;;
    *) printf 'install_error: invalid argument\n' >&2; exit 1 ;;
  esac
done

case "$archive:$checksums:$binary_root:$receipt_root" in
  /*:/*:/*:/*) ;;
  *) printf 'install_error: absolute archive and destinations required\n' >&2; exit 1 ;;
esac
for value in "$archive" "$checksums" "$binary_root" "$receipt_root"; do
  case "$value" in *$'\n'*|*$'\r'*|*'='*) printf 'install_error: unsafe path\n' >&2; exit 1 ;; esac
done

digest() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  else
    shasum -a 256 "$1" | awk '{print $1}'
  fi
}

host_matches_release_row() {
  local release_platform=$1 release_goos=$2 release_architecture=$3
  local host_system host_architecture
  host_system=$(uname -s) || return 1
  host_architecture=$(uname -m) || return 1
  case "$release_platform:$release_goos:$release_architecture" in
    macos-27:darwin:arm64)
      [[ "$host_system:$host_architecture" == Darwin:arm64 ]] || return 1
      command -v sw_vers >/dev/null 2>&1 || return 1
      [[ $(sw_vers -productVersion 2>/dev/null) == 27.0 ]]
      ;;
    ubuntu-26.04:linux:amd64)
      [[ "$host_system:$host_architecture" == Linux:x86_64 ]] || return 1
      ubuntu_26_04_host
      ;;
    ubuntu-26.04:linux:arm64)
      [[ "$host_system:$host_architecture" == Linux:aarch64 ]] || return 1
      ubuntu_26_04_host
      ;;
    *) return 1 ;;
  esac
}

ubuntu_26_04_host() {
  local os_release=/etc/os-release os_id os_version
  [[ -r "$os_release" ]] || return 1
  [[ $(grep -c '^ID=' "$os_release") == 1 && $(grep -c '^VERSION_ID=' "$os_release") == 1 ]] || return 1
  os_id=$(awk -F= '$1 == "ID" {print $2}' "$os_release")
  os_version=$(awk -F= '$1 == "VERSION_ID" {print $2}' "$os_release")
  [[ "$os_id" == ubuntu || "$os_id" == '"ubuntu"' ]] || return 1
  [[ "$os_version" == 26.04 || "$os_version" == '"26.04"' ]]
}

archive_name=$(basename "$archive")
expected=$(awk -v name="$archive_name" '$2 == name {print $1}' "$checksums")
if [[ ! "$expected" =~ ^[0-9a-f]{64}$ || $(printf '%s\n' "$expected" | wc -l | tr -d ' ') != 1 ]]; then
  printf 'install_error: exact archive checksum unavailable\n' >&2
  exit 1
fi
actual=$(digest "$archive")
if [[ "$actual" != "$expected" ]]; then
  printf 'install_error: archive checksum mismatch\n' >&2
  exit 1
fi

temporary=$(mktemp -d)
stage=
receipt_stage=
operation_marker=
lock_directory=
binary_committed=false
cleanup() {
  [[ -z "$stage" ]] || rm -f -- "$stage"
  [[ -z "$receipt_stage" ]] || rm -f -- "$receipt_stage"
  if [[ "$binary_committed" == false && -n "$operation_marker" ]]; then rm -f -- "$operation_marker"; fi
  [[ -z "$lock_directory" ]] || rmdir "$lock_directory" 2>/dev/null || true
  rm -rf -- "$temporary"
}
trap cleanup EXIT
staged_archive="$temporary/release.tar.gz"
cp -- "$archive" "$staged_archive"
chmod 600 "$staged_archive"
[[ $(digest "$staged_archive") == "$actual" ]] || { printf 'install_error: archive changed during staging\n' >&2; exit 1; }

while IFS= read -r entry; do
  case "$entry" in /*|../*|*/../*|*/..|*\\*) printf 'install_error: unsafe archive path\n' >&2; exit 1 ;; esac
done < <(tar -tzf "$staged_archive")
if tar -tvzf "$staged_archive" | awk 'substr($1,1,1) != "-" && substr($1,1,1) != "d" {exit 1}'; then :; else
  printf 'install_error: archive links or special files refused\n' >&2
  exit 1
fi
tar -xzf "$staged_archive" -C "$temporary"
bundle=$(find "$temporary" -mindepth 1 -maxdepth 1 -type d)
if [[ -z "$bundle" || $(printf '%s\n' "$bundle" | wc -l | tr -d ' ') != 1 ]]; then
  printf 'install_error: invalid archive root\n' >&2
  exit 1
fi
(
  cd "$bundle"
  while read -r hash name extra; do
    [[ -z "${extra:-}" && "$hash" =~ ^[0-9a-f]{64}$ && -f "$name" && ! -L "$name" ]] || exit 1
    [[ $(digest "$name") == "$hash" ]] || exit 1
  done <MANIFEST.sha256
  find . -type f ! -path './MANIFEST.sha256' -print | sed 's#^./##' | LC_ALL=C sort >"$temporary/actual-files"
  awk '{print $2}' MANIFEST.sha256 | LC_ALL=C sort >"$temporary/manifest-files"
  cmp -s "$temporary/actual-files" "$temporary/manifest-files"
) || { printf 'install_error: bundle manifest mismatch\n' >&2; exit 1; }

[[ -f "$bundle/axiom" && ! -L "$bundle/axiom" ]] || { printf 'install_error: bundle axiom executable missing\n' >&2; exit 1; }

metadata="$bundle/release-metadata.txt"
metadata_fields='formatVersion product version revision sourceState release platform goos architecture skillSetVersion'
[[ $(wc -l <"$metadata" | tr -d ' ') == 10 ]] || { printf 'install_error: release metadata schema mismatch\n' >&2; exit 1; }
for field in $metadata_fields; do
  [[ $(grep -c "^${field}=" "$metadata") == 1 ]] || { printf 'install_error: release metadata schema mismatch\n' >&2; exit 1; }
done
awk -F= '$1 != "formatVersion" && $1 != "product" && $1 != "version" && $1 != "revision" && $1 != "sourceState" && $1 != "release" && $1 != "platform" && $1 != "goos" && $1 != "architecture" && $1 != "skillSetVersion" {exit 1}' "$metadata" || { printf 'install_error: release metadata schema mismatch\n' >&2; exit 1; }
[[ $(awk -F= '$1 == "formatVersion" {print $2}' "$metadata") == 1 ]] || { printf 'install_error: unsupported release metadata\n' >&2; exit 1; }
[[ $(awk -F= '$1 == "product" {print $2}' "$metadata") == Axiom ]] || { printf 'install_error: unsupported release metadata\n' >&2; exit 1; }
version=$(awk -F= '$1 == "version" {print $2}' "$metadata")
[[ "$version" =~ ^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*)?(\+[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*)?$ ]] || { printf 'install_error: unsupported release metadata\n' >&2; exit 1; }
[[ $(awk -F= '$1 == "revision" {print $2}' "$metadata") =~ ^[0-9a-f]{12}$ ]] || { printf 'install_error: unsupported release metadata\n' >&2; exit 1; }
release=$(awk -F= '$1 == "release" {print $2}' "$metadata")
source_state=$(awk -F= '$1 == "sourceState" {print $2}' "$metadata")
[[ "$release" == false || "$release" == true && "$source_state" == clean ]] || { printf 'install_error: unsupported release metadata\n' >&2; exit 1; }

skills_manifest="$bundle/skills-manifest.txt"
[[ $(wc -l <"$skills_manifest" | tr -d ' ') == 8 ]] || { printf 'install_error: skill manifest schema mismatch\n' >&2; exit 1; }
for field in formatVersion skillSetVersion binaryCompatibility; do
  [[ $(grep -c "^${field}=" "$skills_manifest") == 1 ]] || { printf 'install_error: skill manifest schema mismatch\n' >&2; exit 1; }
done
[[ $(awk -F= '$1 == "formatVersion" {print $2}' "$skills_manifest") == 1 && $(awk -F= '$1 == "skillSetVersion" {print $2}' "$skills_manifest") == 1 && $(awk -F= '$1 == "binaryCompatibility" {print $2}' "$skills_manifest") == 1 ]] || { printf 'install_error: unsupported skill manifest\n' >&2; exit 1; }
[[ $(grep -c '^skill\.[a-z0-9-]*=[0-9a-f]\{64\}$' "$skills_manifest") == 5 ]] || { printf 'install_error: skill manifest schema mismatch\n' >&2; exit 1; }
for name in axiom-project-configure axiom-project-show axiom-work-item-create axiom-work-item-run axiom-work-item-status; do
  [[ $(grep -c "^skill\.${name}=" "$skills_manifest") == 1 ]] || { printf 'install_error: skill manifest schema mismatch\n' >&2; exit 1; }
done
while IFS='=' read -r key hash; do
  case "$key" in
    skill.*)
      name=${key#skill.}
      [[ -f "$bundle/skills/$name/SKILL.md" && ! -L "$bundle/skills/$name/SKILL.md" && $(digest "$bundle/skills/$name/SKILL.md") == "$hash" ]] || { printf 'install_error: skill manifest mismatch\n' >&2; exit 1; }
      ;;
  esac
done <"$skills_manifest"

platform=$(awk -F= '$1 == "goos" {print $2}' "$metadata")
architecture=$(awk -F= '$1 == "architecture" {print $2}' "$metadata")
release_platform=$(awk -F= '$1 == "platform" {print $2}' "$metadata")
if ! host_matches_release_row "$release_platform" "$platform" "$architecture"; then
  printf 'install_error: unsupported host for release row %s/%s; exact approved OS, version, distribution, and architecture required\n' "$release_platform" "$architecture" >&2
  exit 1
fi

file_owner() {
  stat -f %u "$1" 2>/dev/null || stat -c %u "$1"
}

file_links() {
  stat -f %l "$1" 2>/dev/null || stat -c %h "$1"
}

file_mode() {
  stat -f %Lp "$1" 2>/dev/null || stat -c %a "$1"
}

private_acl() {
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

private_directory() {
  local directory=$1
  [[ -d "$directory" && ! -L "$directory" && $(file_owner "$directory") == "$(id -u)" && $(file_mode "$directory") == 700 ]] || return 1
  private_acl "$directory"
}

safe_components() {
  local current=$1
  while [[ "$current" != / ]]; do
    [[ ! -L "$current" ]] || return 1
    current=$(dirname "$current")
  done
}

for directory in "$binary_root" "$receipt_root"; do
  safe_components "$directory" || { printf 'install_error: symlink destination refused\n' >&2; exit 1; }
  if [[ -e "$directory" ]]; then
    private_directory "$directory" || { printf 'install_error: unsafe destination ownership, permissions, ACL, or type\n' >&2; exit 1; }
  else
    mkdir -p -- "$directory"
    chmod 700 "$directory"
  fi
  safe_components "$directory" || { printf 'install_error: symlink destination refused\n' >&2; exit 1; }
  private_directory "$directory" || { printf 'install_error: unsafe destination ownership, permissions, ACL, or type\n' >&2; exit 1; }
done

lock_directory="$receipt_root/.axiom-install.lock"
if ! mkdir "$lock_directory" 2>/dev/null; then
  printf 'install_error: concurrent installation refused\n' >&2
  exit 1
fi
if [[ -e "$receipt_root/.axiom-install-operation" || -L "$receipt_root/.axiom-install-operation" ]]; then
  printf 'install_error: recovery_required\n' >&2
  exit 1
fi

destination="$binary_root/axiom"
receipt="$receipt_root/installation.receipt"
new_checksum=$(digest "$bundle/axiom")
expected_receipt="$temporary/expected.receipt"

valid_receipt_schema() {
  local path=$1 installed_at
  local fields='formatVersion destination sha256 product version revision sourceState release platform goos architecture skillSetVersion archiveSha256 skillManifestSha256 installedAt'
  [[ $(wc -l <"$path" | tr -d ' ') == 15 ]] || return 1
  for field in $fields; do
    [[ $(grep -c "^${field}=" "$path") == 1 ]] || return 1
  done
  awk -F= '$1 != "formatVersion" && $1 != "destination" && $1 != "sha256" && $1 != "product" && $1 != "version" && $1 != "revision" && $1 != "sourceState" && $1 != "release" && $1 != "platform" && $1 != "goos" && $1 != "architecture" && $1 != "skillSetVersion" && $1 != "archiveSha256" && $1 != "skillManifestSha256" && $1 != "installedAt" {exit 1}' "$path" || return 1
  installed_at=$(awk -F= '$1 == "installedAt" {print $2}' "$path")
  [[ "$installed_at" =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z$ ]]
}

write_expected_receipt() {
  local installed_at=$1
  {
    printf 'formatVersion=1\n'
    printf 'destination=%s\n' "$destination"
    printf 'sha256=%s\n' "$new_checksum"
    awk -F= '$1 != "formatVersion" {print}' "$bundle/release-metadata.txt"
    printf 'archiveSha256=%s\n' "$actual"
    printf 'skillManifestSha256=%s\n' "$(digest "$bundle/skills-manifest.txt")"
    printf 'installedAt=%s\n' "$installed_at"
  } >"$expected_receipt"
}

if [[ -e "$destination" || -L "$destination" ]]; then
  [[ -f "$destination" && ! -L "$destination" ]] || { printf 'install_error: unsafe binary destination\n' >&2; exit 1; }
  [[ -f "$receipt" && ! -L "$receipt" ]] || { printf 'install_error: foreign binary preserved\n' >&2; exit 1; }
  [[ $(file_owner "$destination") == "$(id -u)" && $(file_owner "$receipt") == "$(id -u)" ]] || { printf 'install_error: unowned installation preserved\n' >&2; exit 1; }
  [[ $(file_mode "$destination") == 700 && $(file_mode "$receipt") == 600 ]] || { printf 'install_error: unsafe installation permissions\n' >&2; exit 1; }
  private_acl "$destination" && private_acl "$receipt" || { printf 'install_error: unsafe installation ACL\n' >&2; exit 1; }
  valid_receipt_schema "$receipt" || { printf 'install_error: invalid receipt schema preserved\n' >&2; exit 1; }
  write_expected_receipt "$(awk -F= '$1 == "installedAt" {print $2}' "$receipt")"
  recorded_checksum=$(awk -F= '$1 == "sha256" {print $2}' "$receipt")
  [[ $(file_links "$destination") == 1 && $(file_links "$receipt") == 1 ]] || { printf 'install_error: hard-linked installation preserved\n' >&2; exit 1; }
  [[ "$recorded_checksum" == "$(digest "$destination")" ]] || { printf 'install_error: modified binary preserved\n' >&2; exit 1; }
  cmp -s "$receipt" "$expected_receipt" || { printf 'install_error: divergent receipt preserved\n' >&2; exit 1; }
  if [[ "$recorded_checksum" == "$new_checksum" ]]; then
    printf 'install_status=unchanged\n'
    exit 0
  fi
  printf 'install_error: owned upgrade requires future upgrade flow\n' >&2
  exit 1
fi
if [[ -e "$receipt" || -L "$receipt" ]]; then
  printf 'install_error: foreign receipt preserved\n' >&2
  exit 1
fi
write_expected_receipt "$(date -u '+%Y-%m-%dT%H:%M:%SZ')"

operation_marker="$receipt_root/.axiom-install-operation"
( set -o noclobber; printf 'formatVersion=1\nstage=prepare\narchiveSha256=%s\n' "$actual" >"$operation_marker" ) 2>/dev/null || { printf 'install_error: recovery_required\n' >&2; exit 1; }
chmod 600 "$operation_marker"
if [[ ${AXIOM_INSTALL_TEST_FAIL_STAGE:-} == before_binary ]]; then
  printf 'install_error: injected pre-commit interruption\n' >&2
  exit 75
fi

stage=$(mktemp "$binary_root/.axiom-binary-stage.XXXXXX")
cp "$bundle/axiom" "$stage"
chmod 700 "$stage"
[[ $(digest "$stage") == "$new_checksum" ]] || { printf 'install_error: staged binary mismatch\n' >&2; exit 1; }
mv -n -- "$stage" "$destination"
if [[ -e "$stage" ]]; then
  printf 'install_error: binary publication conflict\n' >&2
  exit 1
fi
stage=
[[ -f "$destination" && ! -L "$destination" && $(digest "$destination") == "$new_checksum" ]] || { printf 'install_error: binary publication uncertain\n' >&2; exit 1; }
[[ $(file_links "$destination") == 1 ]] || { printf 'install_error: binary publication uncertain\n' >&2; exit 1; }
binary_committed=true
printf 'formatVersion=1\nstage=binary_committed\narchiveSha256=%s\n' "$actual" >"$operation_marker"
chmod 600 "$operation_marker"
if [[ ${AXIOM_INSTALL_TEST_FAIL_STAGE:-} == after_binary ]]; then
  printf 'install_status=partial\n' >&2
  exit 75
fi

receipt_stage=$(mktemp "$receipt_root/.axiom-install-receipt.XXXXXX")
cp "$expected_receipt" "$receipt_stage"
chmod 600 "$receipt_stage"
mv -n -- "$receipt_stage" "$receipt"
if [[ -e "$receipt_stage" ]]; then
  printf 'install_status=partial\n' >&2
  exit 1
fi
receipt_stage=
[[ -f "$receipt" && ! -L "$receipt" ]] || { printf 'install_status=partial\n' >&2; exit 1; }
cmp -s "$receipt" "$expected_receipt" || { printf 'install_status=partial\n' >&2; exit 1; }
rm -f -- "$operation_marker"
operation_marker=
printf 'install_status=installed\n'
