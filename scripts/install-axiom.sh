#!/usr/bin/env bash
set -euo pipefail

umask 077

repository_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)
binary_root=${AXIOM_BIN_DIR:-"${HOME}/.local/bin"}
state_root=${AXIOM_INSTALL_STATE_ROOT:-"${XDG_STATE_HOME:-${HOME}/.local/state}/axiom/install"}
source_url=https://github.com/rgomids/axiom

path_configured=false
case ":${PATH:-}:" in
  *":$binary_root:"*) path_configured=true ;;
esac

report_result() {
  local result=$1
  printf '{"event":"axiom_install","result":"%s","version":"%s","commit":"%s","dirty":%s,"pathConfigured":%s}\n' \
    "$result" "$version" "$commit" "$dirty" "$path_configured"
  if [[ "$path_configured" == false ]]; then
    printf 'path_notice: axiom is not on PATH; run: export PATH=%q:$PATH\n' "$binary_root" >&2
  fi
}

case "$binary_root:$state_root" in
  /*:/*) ;;
  *) printf '%s\n' 'install_error: absolute destinations required' >&2; exit 1 ;;
esac
case "$binary_root$state_root" in
  *$'\n'*|*$'\r'*) printf '%s\n' 'install_error: invalid destination' >&2; exit 1 ;;
esac

ensure_private_directory() {
  local directory=$1
  if [[ -L "$directory" ]]; then
    printf '%s\n' 'install_error: symlink destination refused' >&2
    exit 1
  fi
  mkdir -p -- "$directory"
  chmod 700 "$directory"
}

ensure_private_directory "$binary_root"
ensure_private_directory "$state_root"

destination="$binary_root/axiom"
receipt="$state_root/axiom.receipt"
stage=$(mktemp "$binary_root/.axiom-source-stage.XXXXXX")
receipt_stage=$(mktemp "$state_root/.axiom-source-receipt-stage.XXXXXX")
cleanup() {
  rm -f -- "$stage" "$receipt_stage"
}
trap cleanup EXIT

commit=$(git -C "$repository_root" rev-parse --verify HEAD)
version=development
dirty=false
source_state=clean
if [[ -n $(git -C "$repository_root" status --porcelain --untracked-files=normal) ]]; then
  dirty=true
  source_state=dirty
fi
go build -trimpath \
  -ldflags "-X main.buildVersion=$version -X main.buildRevision=${commit:0:12} -X main.buildSourceState=$source_state -X main.buildRelease=false" \
  -o "$stage" "$repository_root/cmd/lingo"
chmod 700 "$stage"
new_checksum=$(shasum -a 256 "$stage" | awk '{print $1}')

if [[ -e "$destination" || -L "$destination" ]]; then
  if [[ -L "$destination" || ! -f "$destination" ]]; then
    printf '%s\n' 'install_error: existing destination is not a regular file' >&2
    exit 1
  fi
  if cmp -s -- "$stage" "$destination"; then
    report_result unchanged
    exit 0
  fi
  if [[ ! -f "$receipt" || -L "$receipt" ]]; then
    printf '%s\n' 'install_error: unowned destination refused' >&2
    exit 1
  fi
  recorded_destination=$(awk -F= '$1 == "destination" {sub(/^[^=]*=/, ""); print; exit}' "$receipt")
  recorded_checksum=$(awk -F= '$1 == "sha256" {print $2; exit}' "$receipt")
  current_checksum=$(shasum -a 256 "$destination" | awk '{print $1}')
  if [[ "$recorded_destination" != "$destination" || "$recorded_checksum" != "$current_checksum" ]]; then
    printf '%s\n' 'install_error: existing destination differs from owned install' >&2
    exit 1
  fi
fi

mv -f -- "$stage" "$destination"
stage=
{
  printf 'formatVersion=1\n'
  printf 'destination=%s\n' "$destination"
  printf 'sha256=%s\n' "$new_checksum"
  printf 'source=%s\n' "$source_url"
  printf 'commit=%s\n' "$commit"
  printf 'dirty=%s\n' "$dirty"
} >"$receipt_stage"
chmod 600 "$receipt_stage"
mv -f -- "$receipt_stage" "$receipt"
receipt_stage=

report_result installed
