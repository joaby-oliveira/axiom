#!/bin/sh
# Axiom remote installer bootstrap.
#
#   curl -fsSL https://raw.githubusercontent.com/rgomids/axiom/main/scripts/install.sh | sh
#   ... | sh -s -- --channel stable
#   ... | sh -s -- --version v0.1.0-rc.2
#
# Resolves one published Axiom release, downloads the archive for this exact
# supported host row plus the release SHA256SUMS, verifies the archive digest
# before anything is extracted, and hands the verified archive to the release
# installer it contains (install.sh in the bundle). That installer owns every
# installation effect: ownership, receipts, no-op reinstall, protected owned
# upgrade, downgrade refusal and recovery. This bootstrap only reads the
# network and writes a private temporary directory that it removes.
#
# It never uses sudo, edits shell profiles or PATH, installs Runtimes, or
# touches credentials.
set -eu
umask 077

repository_url=https://github.com/rgomids/axiom
supported_rows='macOS 27.0/arm64, Ubuntu 26.04/amd64, Ubuntu 26.04/arm64'

fail() {
  printf 'install_error: %s\n' "$1" >&2
  exit 1
}

usage() {
  cat <<'EOF'
Usage: install.sh [--channel stable|rc | --version vMAJOR.MINOR.PATCH[-rc.N]]
                  [--bin-dir <absolute-dir>] [--receipt-dir <absolute-dir>]

Without a selector the latest published stable release is installed; release
candidates are never selected implicitly. --channel and --version are mutually
exclusive. Defaults: --bin-dir $HOME/.local/bin and
--receipt-dir ${XDG_STATE_HOME:-$HOME/.local/state}/axiom/install.
EOF
}

newline='
'
channel=
version_tag=
binary_root=
receipt_root=
channel_seen=false
version_seen=false
bin_seen=false
receipt_seen=false
while [ $# -gt 0 ]; do
  case "$1" in
    --channel|--version|--bin-dir|--receipt-dir)
      [ $# -ge 2 ] || fail "$1 requires a value"
      case "$1" in
        --channel) [ "$channel_seen" = false ] || fail '--channel given more than once'; channel_seen=true; channel=$2 ;;
        --version) [ "$version_seen" = false ] || fail '--version given more than once'; version_seen=true; version_tag=$2 ;;
        --bin-dir) [ "$bin_seen" = false ] || fail '--bin-dir given more than once'; bin_seen=true; binary_root=$2 ;;
        --receipt-dir) [ "$receipt_seen" = false ] || fail '--receipt-dir given more than once'; receipt_seen=true; receipt_root=$2 ;;
      esac
      shift 2
      ;;
    -h|--help) usage; exit 0 ;;
    *) usage >&2; fail 'invalid argument' ;;
  esac
done

if [ "$channel_seen" = true ] && [ "$version_seen" = true ]; then
  fail '--channel and --version are mutually exclusive; choose one selector'
fi
if [ "$version_seen" = true ]; then
  case "$version_tag" in *"$newline"*) fail 'invalid --version' ;; esac
  printf '%s\n' "$version_tag" | grep -Eq '^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-rc\.(0|[1-9][0-9]*))?$' \
    || fail '--version must be an exact published tag vMAJOR.MINOR.PATCH or vMAJOR.MINOR.PATCH-rc.N'
  selector="version $version_tag"
else
  [ "$channel_seen" = true ] || channel=stable
  case "$channel" in
    stable) selector='channel stable' ;;
    rc)
      # Discovering the newest published release candidate needs a
      # deterministic channel index that does not exist yet; this bootstrap
      # does not scrape HTML or parse API JSON to guess it.
      fail 'the rc channel is not available yet; install an exact release candidate with --version vMAJOR.MINOR.PATCH-rc.N'
      ;;
    *) fail '--channel must be stable or rc' ;;
  esac
fi

valid_directory() {
  case "$1" in
    /) return 1 ;;
    /*) ;;
    *) return 1 ;;
  esac
  case "$1" in
    *"$newline"*|*'='*|*//*|*/./*|*/../*|*/.|*/..|*/) return 1 ;;
  esac
  case "$1" in *"$(printf '\r')"*) return 1 ;; esac
  return 0
}

if [ -z "$binary_root" ] || [ -z "$receipt_root" ]; then
  [ -n "${HOME:-}" ] && valid_directory "$HOME" || fail 'HOME must be an absolute path when --bin-dir or --receipt-dir is omitted'
fi
[ -n "$binary_root" ] || binary_root=$HOME/.local/bin
if [ -z "$receipt_root" ]; then
  state_home=${XDG_STATE_HOME:-}
  valid_directory "$state_home" || state_home=$HOME/.local/state
  receipt_root=$state_home/axiom/install
fi
valid_directory "$binary_root" || fail '--bin-dir must be an absolute canonical path without newlines or ='
valid_directory "$receipt_root" || fail '--receipt-dir must be an absolute canonical path without newlines or ='

# Exact supported host row; anything else stops before any download.
system=$(uname -s)
machine=$(uname -m)
row=
case "$system:$machine" in
  Darwin:arm64)
    if command -v sw_vers >/dev/null 2>&1 && [ "$(sw_vers -productVersion 2>/dev/null)" = 27.0 ]; then
      row=macos-27-arm64
    fi
    ;;
  Linux:x86_64|Linux:aarch64)
    if [ -r /etc/os-release ] \
      && [ "$(grep -c '^ID=' /etc/os-release)" = 1 ] && [ "$(grep -c '^VERSION_ID=' /etc/os-release)" = 1 ] \
      && grep -Eq '^ID=(ubuntu|"ubuntu")$' /etc/os-release \
      && grep -Eq '^VERSION_ID=(26\.04|"26\.04")$' /etc/os-release; then
      case "$machine" in
        x86_64) row=ubuntu-26.04-amd64 ;;
        aarch64) row=ubuntu-26.04-arm64 ;;
      esac
    fi
    ;;
esac
[ -n "$row" ] || fail "unsupported host $system/$machine; supported rows are exactly $supported_rows"

for tool in curl tar bash awk grep mktemp; do
  command -v "$tool" >/dev/null 2>&1 || fail "required tool not found: $tool"
done
if command -v sha256sum >/dev/null 2>&1; then
  digest() { sha256sum "$1" | awk '{print $1}'; }
elif command -v shasum >/dev/null 2>&1; then
  digest() { shasum -a 256 "$1" | awk '{print $1}'; }
else
  fail 'required tool not found: sha256sum or shasum'
fi

# fetch <url> <output> <max-bytes>: HTTPS only, including redirects.
fetch() {
  curl --proto '=https' --tlsv1.2 --fail --silent --show-error --location \
    --retry 2 --connect-timeout 20 --max-time 300 --max-filesize "$3" \
    --output "$2" "$1"
}

# Resolve the selector to one exact tag.
if [ "$version_seen" = true ]; then
  tag=$version_tag
else
  # GitHub redirects /releases/latest to the latest published non-prerelease,
  # non-draft release, or to /releases when there is none. Only the redirect
  # target is read; the page itself is not fetched or parsed.
  set +e
  location=$(curl --proto '=https' --tlsv1.2 --fail --silent --show-error \
    --retry 2 --connect-timeout 20 --max-time 60 \
    --output /dev/null --write-out '%{redirect_url}' "$repository_url/releases/latest")
  status=$?
  set -e
  [ "$status" -eq 0 ] || fail "network failure while resolving the latest stable release (curl exit $status); nothing was installed"
  case "$location" in
    "$repository_url/releases")
      fail 'no published stable Axiom release exists yet; release candidates are opt-in: use --version vMAJOR.MINOR.PATCH-rc.N'
      ;;
    "$repository_url/releases/tag/"*)
      tag=${location#"$repository_url/releases/tag/"}
      ;;
    *)
      fail 'the latest stable release could not be resolved unambiguously; use --version vMAJOR.MINOR.PATCH'
      ;;
  esac
  case "$tag" in *"$newline"*) fail 'invalid latest stable release tag' ;; esac
  printf '%s\n' "$tag" | grep -Eq '^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$' \
    || fail 'the latest release tag is not a stable vMAJOR.MINOR.PATCH tag; use --version'
fi
version=${tag#v}
asset=axiom-$version-$row.tar.gz
bundle=axiom-$version-$row

work=$(mktemp -d)
[ -n "$work" ] && [ -d "$work" ] || fail 'private temporary directory unavailable'
cleanup() { rm -rf -- "$work"; }
trap cleanup EXIT
trap 'exit 130' HUP INT TERM
chmod 700 "$work"

download_url=$repository_url/releases/download/$tag
set +e
fetch "$download_url/SHA256SUMS" "$work/SHA256SUMS" 65536
status=$?
set -e
case "$status" in
  0) ;;
  22) fail "release $tag has no published SHA256SUMS; nothing was installed" ;;
  *) fail "network failure while downloading SHA256SUMS for $tag (curl exit $status); nothing was installed" ;;
esac
expected=$(awk -v name="$asset" '$2 == name && NF == 2 {print $1}' "$work/SHA256SUMS")
[ "$(printf '%s\n' "$expected" | grep -c .)" = 1 ] && printf '%s\n' "$expected" | grep -Eq '^[0-9a-f]{64}$' \
  || fail "release $tag publishes no single checksum for $asset; nothing was installed"

set +e
fetch "$download_url/$asset" "$work/$asset" 268435456
status=$?
set -e
case "$status" in
  0) ;;
  22) fail "release $tag has no published asset $asset; nothing was installed" ;;
  *) fail "network failure while downloading $asset (curl exit $status); nothing was installed" ;;
esac
actual=$(digest "$work/$asset")
[ "$actual" = "$expected" ] || fail "checksum mismatch for $asset; nothing was installed"

# Only now read the verified archive: take the bundle's release installer and
# release metadata, regular files whose digests must match the bundle
# manifest, and require the metadata to be the resolved version and host row
# of a clean release build.
# Fixed member names without whitespace; expanded unquoted on purpose.
members="$bundle/install.sh $bundle/release-metadata.txt $bundle/MANIFEST.sha256"
listing=$(tar -tvzf "$work/$asset" $members 2>/dev/null) \
  || fail "verified archive lacks its release installer or metadata; nothing was installed"
[ "$(printf '%s\n' "$listing" | awk 'substr($1,1,1) == "-"' | grep -c .)" = 3 ] \
  || fail 'release installer or metadata entry is not a regular file; nothing was installed'
mkdir "$work/bundle"
tar -xzf "$work/$asset" -C "$work/bundle" $members
for member in install.sh release-metadata.txt; do
  path=$work/bundle/$bundle/$member
  [ -f "$path" ] && [ ! -h "$path" ] || fail "bundle $member is not a regular file; nothing was installed"
  [ "$(awk -v name="$member" '$2 == name && NF == 2 {print $1}' "$work/bundle/$bundle/MANIFEST.sha256")" = "$(digest "$path")" ] \
    || fail "bundle $member does not match its manifest; nothing was installed"
done
metadata=$work/bundle/$bundle/release-metadata.txt
platform=${row%-*}
architecture=${row##*-}
for expected_line in "product=Axiom" "version=$version" "sourceState=clean" "release=true" "platform=$platform" "architecture=$architecture"; do
  grep -Fxq "$expected_line" "$metadata" || fail "release metadata does not match $tag for $row; nothing was installed"
done
installer=$work/bundle/$bundle/install.sh

printf 'install_selector=%s\n' "$selector"
printf 'install_tag=%s\n' "$tag"
printf 'install_version=%s\n' "$version"
printf 'install_asset=%s\n' "$asset"
printf 'install_asset_sha256=%s\n' "$actual"
printf 'install_row=%s\n' "$row"

set +e
bash "$installer" --archive "$work/$asset" --checksums "$work/SHA256SUMS" --bin-dir "$binary_root" --receipt-dir "$receipt_root"
status=$?
set -e
[ "$status" -eq 0 ] || exit "$status"

receipt=$receipt_root/installation.receipt
printf 'install_revision=%s\n' "$(awk -F= '$1 == "revision" {print $2}' "$receipt")"
printf 'install_receipt=%s\n' "$receipt"
printf 'install_binary=%s/axiom\n' "$binary_root"
case ":${PATH:-}:" in
  *":$binary_root:"*) ;;
  *)
    quoted=$(printf '%s' "$binary_root" | sed "s/'/'\\\\''/g")
    printf "path_notice: axiom is not on PATH; run: export PATH='%s':\"\$PATH\"\n" "$quoted" >&2
    ;;
esac
