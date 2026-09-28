#!/usr/bin/env bash
# S9/T39 remote installer bootstrap matrix (scripts/install.sh).
#
# Releases are served by a fake `curl` on PATH from local fixtures: no live
# GitHub content is read. Release archives are built from a clean clone of the
# committed HEAD. Selector, host and network refusals run on any host; install,
# reinstall, upgrade and recovery cases need a supported release row and exit
# 78 (blocked) elsewhere. On Linux, AXIOM_TEST_SYNTHETIC_UBUNTU_ROW=1 re-runs
# the suite in a private mount namespace whose /etc/os-release declares Ubuntu
# 26.04; that Evidence is synthetic and never native target acceptance.
set -euo pipefail

umask 077

repository_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)

if [[ "${AXIOM_TEST_SYNTHETIC_UBUNTU_ROW:-}" == 1 && -z "${AXIOM_TEST_SYNTHETIC_ACTIVE:-}" ]]; then
  [[ $(uname -s) == Linux ]] || { printf 'synthetic Ubuntu row requires Linux\n' >&2; exit 1; }
  synthetic=$(mktemp)
  printf 'PRETTY_NAME="Ubuntu 26.04 (synthetic test row)"\nNAME="Ubuntu"\nID=ubuntu\nVERSION_ID="26.04"\n' >"$synthetic"
  chmod 644 "$synthetic"
  namespace=(unshare -m)
  [[ $(id -u) == 0 ]] || namespace=(unshare -r -m)
  status=0
  "${namespace[@]}" env AXIOM_TEST_SYNTHETIC_ACTIVE=1 SYNTHETIC_OS_RELEASE="$synthetic" \
    bash -eo pipefail -c 'mount --bind "$SYNTHETIC_OS_RELEASE" /etc/os-release && exec bash "$0"' "${BASH_SOURCE[0]}" || status=$?
  rm -f -- "$synthetic"
  exit "$status"
fi

temporary=$(mktemp -d)
temporary=$(cd "$temporary" && pwd -P)
trap 'rm -rf -- "$temporary"' EXIT
bootstrap="$repository_root/scripts/install.sh"
# BOOTSTRAP_SHELL selects the POSIX shell, e.g. "bash --posix" for macOS sh.
bootstrap_shell=${BOOTSTRAP_SHELL:-sh}
base=https://github.com/rgomids/axiom
failures=0

digest() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  else
    shasum -a 256 "$1" | awk '{print $1}'
  fi
}

# Fake curl: records every request and serves $fixtures/releases/<tag>/<file>
# and the /releases/latest redirect target from $fixtures/latest-location.
fixtures="$temporary/fixtures"
tools="$temporary/tools"
mkdir -p "$fixtures/releases" "$tools"
cat >"$tools/curl" <<'EOF'
#!/bin/sh
output= write= url= arguments="$*"
while [ $# -gt 0 ]; do
  case "$1" in
    --output) output=$2; shift 2 ;;
    --write-out) write=$2; shift 2 ;;
    --proto|--max-filesize|--retry|--connect-timeout|--max-time) shift 2 ;;
    --*) shift ;;
    *) url=$1; shift ;;
  esac
done
printf '%s\n' "$url" >>"$FAKE_LEDGER"
printf '%s\n' "$arguments" >>"$FAKE_LEDGER.arguments"
case "$url" in *"${FAKE_NETWORK_DOWN:-no-network-failure}"*) exit 7 ;; esac
case "$url" in
  https://github.com/rgomids/axiom/releases/latest)
    [ "$write" = '%{redirect_url}' ] && [ "$output" = /dev/null ] || exit 2
    cat "$FAKE_FIXTURES/latest-location"
    ;;
  https://github.com/rgomids/axiom/releases/download/*)
    file=$FAKE_FIXTURES/releases/${url#https://github.com/rgomids/axiom/releases/download/}
    [ -f "$file" ] || exit 22
    cp "$file" "$output"
    ;;
  *) exit 6 ;;
esac
EOF
chmod 700 "$tools/curl"

step() {
  local label=$1
  shift
  if "$@" >"$temporary/step.log" 2>&1; then
    printf 'case=%s result=pass\n' "$label"
  else
    printf 'case=%s result=fail\n' "$label"
    sed -n '1,30p' "$temporary/step.log" | sed 's/^/  log: /'
    failures=$((failures + 1))
  fi
}

# run_bootstrap <home> <args...>: isolated HOME, fake network, private TMPDIR.
run_bootstrap() {
  local home=$1
  shift
  rm -rf -- "$temporary/tmp"
  mkdir -m 700 "$temporary/tmp"
  : >"$temporary/ledger"
  : >"$temporary/ledger.arguments"
  env -i HOME="$home" PATH="$tools:${EXTRA_PATH:+$EXTRA_PATH:}/usr/local/bin:/usr/bin:/bin" TMPDIR="$temporary/tmp" \
    FAKE_FIXTURES="$fixtures" FAKE_LEDGER="$temporary/ledger" FAKE_NETWORK_DOWN="${FAKE_NETWORK_DOWN:-no-network-failure}" \
    $bootstrap_shell "$bootstrap" "$@" >"$temporary/stdout" 2>"$temporary/stderr"
}

snapshot() {
  local root=$1
  [[ -e "$root" ]] || { printf 'absent\n'; return; }
  find "$root" -print | LC_ALL=C sort | while IFS= read -r path; do
    if [[ -f "$path" && ! -L "$path" ]]; then
      printf '%s %s %s\n' "$(LC_ALL=C ls -ld "$path" | awk '{print $1}')" "$(digest "$path")" "${path#"$root"}"
    else
      printf '%s %s\n' "$(LC_ALL=C ls -ld "$path" | awk '{print $1}')" "${path#"$root"}"
    fi
  done
}

# refused <home> <expected-stderr> <args...>: non-zero, message, no effects.
refused() {
  local home=$1 expected=$2 before
  shift 2
  before=$(snapshot "$home")
  if run_bootstrap "$home" "$@"; then
    printf 'unexpected success\n'
    cat "$temporary/stdout"
    return 1
  fi
  grep -Fq -- "$expected" "$temporary/stderr" || { cat "$temporary/stderr"; return 1; }
  [[ $(snapshot "$home") == "$before" ]] || { printf 'installation state changed\n'; return 1; }
  [[ -z $(find "$temporary/tmp" -mindepth 1 -print -quit) ]] || { printf 'temporary files left behind\n'; return 1; }
}

no_network() {
  [[ ! -s "$temporary/ledger" ]]
}

new_home() {
  local home="$temporary/homes/$1"
  mkdir -p "$home"
  chmod 700 "$home"
  printf '%s\n' "$home"
}

printf 'suite=s9-install-bootstrap-v1\n'
printf 'source_revision=%s\n' "$(git -C "$repository_root" rev-parse HEAD)"
printf 'shell=%s\n' "$bootstrap_shell"

export -f run_bootstrap snapshot digest refused no_network
export temporary tools fixtures bootstrap bootstrap_shell base

# --- Selector, host and input refusals (any host; no network, no effects).
home=$(new_home selectors)
export home
step channel-version-conflict bash -eo pipefail -c 'refused "$home" "mutually exclusive" --channel stable --version v1.0.0 && no_network'
step version-channel-conflict bash -eo pipefail -c 'refused "$home" "mutually exclusive" --version v1.0.0 --channel rc && no_network'
for invalid in 1.0.0 v1.0 v01.0.0 v1.0.0-beta.1 v1.0.0-rc.01 v1.0.0-poc.1 'v1.0.0;id' ''; do
  step "invalid-version:$invalid" bash -eo pipefail -c 'refused "$home" "exact published tag" --version "$1" && no_network' _ "$invalid"
done
step invalid-channel bash -eo pipefail -c 'refused "$home" "--channel must be stable or rc" --channel beta && no_network'
step duplicate-selector bash -eo pipefail -c 'refused "$home" "more than once" --version v1.0.0 --version v1.1.0 && no_network'
step missing-selector-value bash -eo pipefail -c 'refused "$home" "--version requires a value" --version && no_network'
step latest-rc-channel-unavailable bash -eo pipefail -c 'refused "$home" "rc channel is not available yet" --channel rc && no_network'
step unsafe-directory-input bash -eo pipefail -c '
  refused "$home" "--bin-dir must be an absolute canonical path" --bin-dir relative/bin
  refused "$home" "--bin-dir must be an absolute canonical path" --bin-dir "$home/../x"
  refused "$home" "--receipt-dir must be an absolute canonical path" --receipt-dir "$home/state="
  no_network
'

mkdir -p "$temporary/os-tools" "$temporary/arch-tools"
printf '#!/bin/sh\ncase "$1" in -s) echo FreeBSD ;; -m) echo amd64 ;; *) echo FreeBSD ;; esac\n' >"$temporary/os-tools/uname"
printf '#!/bin/sh\ncase "$1" in -s) echo Linux ;; -m) echo riscv64 ;; *) echo Linux ;; esac\n' >"$temporary/arch-tools/uname"
chmod 700 "$temporary/os-tools/uname" "$temporary/arch-tools/uname"
step unsupported-os bash -eo pipefail -c 'EXTRA_PATH="$temporary/os-tools" refused "$home" "unsupported host FreeBSD/amd64" && no_network'
step unsupported-architecture bash -eo pipefail -c 'EXTRA_PATH="$temporary/arch-tools" refused "$home" "unsupported host Linux/riscv64" && no_network'
[[ ! -e "$home/.local" ]] || { printf 'case=selector-home-untouched result=fail\n'; failures=$((failures + 1)); }

# --- Supported row required from here on.
row=
case "$(uname -s):$(uname -m)" in
  Darwin:arm64) [[ $(sw_vers -productVersion 2>/dev/null) == 27.0 ]] && row=macos-27-arm64 ;;
  Linux:x86_64|Linux:aarch64)
    if grep -Eq '^ID=(ubuntu|"ubuntu")$' /etc/os-release 2>/dev/null && grep -Eq '^VERSION_ID=(26\.04|"26\.04")$' /etc/os-release; then
      row=ubuntu-26.04-amd64
      [[ $(uname -m) == aarch64 ]] && row=ubuntu-26.04-arm64
    fi
    ;;
esac
if [[ -z "$row" ]]; then
  printf 'host_row=unsupported\nfailures=%d\nresult=blocked\n' "$failures"
  ((failures == 0)) || exit 1
  exit 78
fi
printf 'host_row=%s%s\n' "$row" "$([[ -n "${AXIOM_TEST_SYNTHETIC_ACTIVE:-}" ]] && printf ' (synthetic os-release)')"

# Release fixtures: clean release builds of the committed HEAD.
source="$temporary/source"
git clone -q --no-hardlinks "$repository_root" "$source"
git -C "$source" checkout -q --detach "$(git -C "$repository_root" rev-parse --verify HEAD)"
publish() {
  local tag=$1 version=${1#v} flag=${2:-}
  "$source/scripts/build-release-archives.sh" --version "$version" --output "$fixtures/releases/$tag" $flag >/dev/null
}
publish v1.0.0
publish v1.1.0
publish v1.2.0-rc.1
publish v3.0.0
publish v4.0.0 --development
rm -- "$fixtures/releases/v3.0.0/axiom-3.0.0-$row.tar.gz"
mkdir "$fixtures/releases/v6.0.0" "$fixtures/releases/v7.0.0" "$fixtures/releases/v8.0.0"
cp "$fixtures/releases/v1.1.0/axiom-1.1.0-$row.tar.gz" "$fixtures/releases/v6.0.0/axiom-6.0.0-$row.tar.gz"
grep -v "$row" "$fixtures/releases/v1.1.0/SHA256SUMS" >"$fixtures/releases/v6.0.0/SHA256SUMS"
cp "$fixtures/releases/v1.1.0/axiom-1.1.0-$row.tar.gz" "$fixtures/releases/v7.0.0/axiom-7.0.0-$row.tar.gz"
printf 'tamper\n' >>"$fixtures/releases/v7.0.0/axiom-7.0.0-$row.tar.gz"
printf '%s  axiom-7.0.0-%s.tar.gz\n' "$(digest "$fixtures/releases/v1.1.0/axiom-1.1.0-$row.tar.gz")" "$row" >"$fixtures/releases/v7.0.0/SHA256SUMS"
cp "$fixtures/releases/v1.1.0/axiom-1.1.0-$row.tar.gz" "$fixtures/releases/v8.0.0/axiom-8.0.0-$row.tar.gz"
printf '%s  axiom-8.0.0-%s.tar.gz\n' "$(digest "$fixtures/releases/v8.0.0/axiom-8.0.0-$row.tar.gz")" "$row" >"$fixtures/releases/v8.0.0/SHA256SUMS"
printf '%s/releases/tag/v1.1.0' "$base" >"$fixtures/latest-location"
revision12=$(git -C "$repository_root" rev-parse --verify HEAD | cut -c1-12)
asset_sha() { awk -v name="axiom-${1#v}-$row.tar.gz" '$2 == name {print $1}' "$fixtures/releases/$1/SHA256SUMS"; }

bin_of() { printf '%s/.local/bin/axiom\n' "$1"; }
receipt_of() { printf '%s/.local/state/axiom/install/installation.receipt\n' "$1"; }
installed_version() { awk -F= '$1 == "version" {print $2}' "$(receipt_of "$1")"; }
export -f bin_of receipt_of installed_version asset_sha
export row revision12

# 1. Exact stable version into a clean HOME, then identical reinstall.
home=$(new_home main)
export home
step exact-stable-version bash -eo pipefail -c '
  run_bootstrap "$home" --version v1.0.0 || { cat "$temporary/stderr"; exit 1; }
  grep -Fxq "install_status=installed" "$temporary/stdout"
  grep -Fxq "install_selector=version v1.0.0" "$temporary/stdout"
  grep -Fxq "install_tag=v1.0.0" "$temporary/stdout"
  grep -Fxq "install_asset=axiom-1.0.0-$row.tar.gz" "$temporary/stdout"
  grep -Fxq "install_asset_sha256=$(asset_sha v1.0.0)" "$temporary/stdout"
  grep -Fxq "install_revision=$revision12" "$temporary/stdout"
  [[ $(installed_version "$home") == 1.0.0 ]]
  grep -Fxq "archiveSha256=$(asset_sha v1.0.0)" "$(receipt_of "$home")"
  grep -Fq "path_notice: axiom is not on PATH" "$temporary/stderr"
  [[ $(cd / && env -i PATH="$home/.local/bin:/usr/bin:/bin" bash --noprofile --norc -c "type -t axiom") == file ]]
  (cd / && "$(bin_of "$home")" --json version) | grep -Fq "\"version\":\"1.0.0\",\"revision\":\"$revision12\",\"sourceState\":\"clean\""
  printf "https://github.com/rgomids/axiom/releases/download/v1.0.0/SHA256SUMS\nhttps://github.com/rgomids/axiom/releases/download/v1.0.0/axiom-1.0.0-%s.tar.gz\n" "$row" | cmp -s - "$temporary/ledger"
  [[ $(grep -c -- "--proto =https" "$temporary/ledger.arguments") == 2 ]]
  [[ -z $(find "$temporary/tmp" -mindepth 1 -print -quit) ]]
'
step same-version-reinstall-no-op bash -eo pipefail -c '
  before=$(snapshot "$home")
  run_bootstrap "$home" --version v1.0.0
  grep -Fxq "install_status=unchanged" "$temporary/stdout"
  [[ $(snapshot "$home") == "$before" ]]
'

# 2. Default and explicit stable resolve the latest stable release, never an
# RC; an older owned installation upgrades through the protected path.
step default-stable-owned-upgrade bash -eo pipefail -c '
  run_bootstrap "$home" || { cat "$temporary/stderr"; exit 1; }
  grep -Fxq "install_selector=channel stable" "$temporary/stdout"
  grep -Fxq "install_tag=v1.1.0" "$temporary/stdout"
  grep -Fxq "install_status=upgraded" "$temporary/stdout"
  [[ $(installed_version "$home") == 1.1.0 ]]
  grep -Fxq "sha256=$(digest "$(bin_of "$home")")" "$(receipt_of "$home")"
  [[ ! -e "$home/.local/state/axiom/install/.axiom-install-operation" && ! -e "$home/.local/state/axiom/install/.axiom-install.lock" ]]
  head -n 1 "$temporary/ledger" | grep -Fxq "https://github.com/rgomids/axiom/releases/latest"
  ! grep -Fq rc "$temporary/ledger"
'
step explicit-stable-equivalent bash -eo pipefail -c '
  before=$(snapshot "$home")
  run_bootstrap "$home" --channel stable
  grep -Fxq "install_selector=channel stable" "$temporary/stdout"
  grep -Fxq "install_tag=v1.1.0" "$temporary/stdout"
  grep -Fxq "install_status=unchanged" "$temporary/stdout"
  [[ $(snapshot "$home") == "$before" ]]
'
step downgrade-refused bash -eo pipefail -c 'refused "$home" "downgrade_refused" --version v1.0.0'
step exact-rc-version bash -eo pipefail -c '
  run_bootstrap "$home" --version v1.2.0-rc.1 || { cat "$temporary/stderr"; exit 1; }
  grep -Fxq "install_tag=v1.2.0-rc.1" "$temporary/stdout"
  grep -Fxq "install_status=upgraded" "$temporary/stdout"
  [[ $(installed_version "$home") == 1.2.0-rc.1 ]]
  (cd / && "$(bin_of "$home")" --json version) | grep -Fq "\"version\":\"1.2.0-rc.1\""
'
step stable-below-installed-rc-refused bash -eo pipefail -c 'refused "$home" "downgrade_refused"'

# 3. Release resolution and artifact failures: zero installation effects.
home=$(new_home failures)
export home
step no-stable-release bash -eo pipefail -c '
  printf "%s/releases" "$base" >"$fixtures/latest-location"
  status=0
  refused "$home" "no published stable Axiom release exists yet" || status=$?
  printf "%s/releases/tag/v1.1.0" "$base" >"$fixtures/latest-location"
  [[ $status == 0 ]] && [[ $(wc -l <"$temporary/ledger" | tr -d " ") == 1 ]]
'
step ambiguous-latest-redirect bash -eo pipefail -c '
  printf "%s/releases/tag/v1.2.0-rc.1" "$base" >"$fixtures/latest-location"
  status=0
  refused "$home" "not a stable vMAJOR.MINOR.PATCH tag" || status=$?
  printf "%s/releases/tag/v1.1.0" "$base" >"$fixtures/latest-location"
  [[ $status == 0 ]]
'
step unpublished-exact-version bash -eo pipefail -c 'refused "$home" "release v9.9.9 has no published SHA256SUMS" --version v9.9.9'
step missing-asset bash -eo pipefail -c 'refused "$home" "has no published asset axiom-3.0.0-$row.tar.gz" --version v3.0.0'
step missing-checksum bash -eo pipefail -c 'refused "$home" "publishes no single checksum" --version v6.0.0 && ! grep -Fq ".tar.gz" "$temporary/ledger"'
step checksum-mismatch bash -eo pipefail -c 'refused "$home" "checksum mismatch for axiom-7.0.0-$row.tar.gz" --version v7.0.0'
step mislabeled-release-asset bash -eo pipefail -c 'refused "$home" "lacks its release installer or metadata" --version v8.0.0'
step development-build-not-installable bash -eo pipefail -c 'refused "$home" "release metadata does not match v4.0.0" --version v4.0.0'
step network-failure-resolving-latest bash -eo pipefail -c 'FAKE_NETWORK_DOWN=/releases/latest refused "$home" "network failure while resolving the latest stable release"'
step network-failure-checksums bash -eo pipefail -c 'FAKE_NETWORK_DOWN=/SHA256SUMS refused "$home" "network failure while downloading SHA256SUMS" --version v1.1.0'
step network-failure-asset bash -eo pipefail -c 'FAKE_NETWORK_DOWN=.tar.gz refused "$home" "network failure while downloading axiom-1.1.0-$row.tar.gz" --version v1.1.0'
[[ ! -e "$home/.local" ]] || { printf 'case=failure-homes-untouched result=fail\n'; failures=$((failures + 1)); }

# 4. Foreign, modified, invalid and unsafe installation state fail closed.
home=$(new_home foreign)
export home
step foreign-target-preserved bash -eo pipefail -c '
  mkdir -p -m 700 "$home/.local" "$home/.local/bin"
  printf "#!/bin/sh\necho foreign\n" >"$home/.local/bin/axiom"; chmod 700 "$home/.local/bin/axiom"
  refused "$home" "foreign binary preserved" --version v1.1.0
'
home=$(new_home legacy-lingo)
export home
step legacy-lingo-untouched bash -eo pipefail -c '
  mkdir -p -m 700 "$home/.local" "$home/.local/bin"
  printf "#!/bin/sh\necho legacy\n" >"$home/.local/bin/lingo"; chmod 700 "$home/.local/bin/lingo"
  before=$(digest "$home/.local/bin/lingo")
  run_bootstrap "$home" --version v1.0.0
  grep -Fxq "install_status=installed" "$temporary/stdout"
  [[ $(digest "$home/.local/bin/lingo") == "$before" ]]
'
home=$(new_home modified)
export home
step modified-owned-binary bash -eo pipefail -c '
  run_bootstrap "$home" --version v1.0.0
  printf "tampered\n" >>"$(bin_of "$home")"
  refused "$home" "modified binary preserved" --version v1.0.0
  refused "$home" "modified binary preserved" --version v1.1.0
'
home=$(new_home receipt)
export home
step invalid-receipt bash -eo pipefail -c '
  run_bootstrap "$home" --version v1.0.0
  printf "unknown=value\n" >>"$(receipt_of "$home")"
  refused "$home" "invalid receipt schema preserved" --version v1.1.0
'
home=$(new_home unsafe)
export home
step unsafe-destination-permissions bash -eo pipefail -c '
  mkdir -p "$home/.local/bin"; chmod 755 "$home/.local/bin"
  refused "$home" "unsafe destination ownership, permissions, ACL, or type" --version v1.1.0
'
home=$(new_home symlink)
export home
step symlinked-destination bash -eo pipefail -c '
  mkdir -p -m 700 "$home/elsewhere" "$home/.local"
  ln -s "$home/elsewhere" "$home/.local/bin"
  refused "$home" "symlink destination refused" --version v1.1.0
  [[ -z $(find "$home/elsewhere" -mindepth 1 -print -quit) ]]
'

# 5. Concurrency and interruption.
home=$(new_home locked)
export home
step concurrent-install-lock bash -eo pipefail -c '
  mkdir -p -m 700 "$home/.local" "$home/.local/state" "$home/.local/state/axiom" "$home/.local/state/axiom/install" "$home/.local/state/axiom/install/.axiom-install.lock"
  refused "$home" "concurrent installation refused" --version v1.1.0
  [[ -d "$home/.local/state/axiom/install/.axiom-install.lock" && ! -e "$home/.local/bin" ]]
'
home=$(new_home race)
export home
step concurrent-install-race bash -eo pipefail -c '
  one="$temporary/race-one" two="$temporary/race-two"
  mkdir -m 700 "$one" "$two"
  env -i HOME="$home" PATH="$tools:/usr/local/bin:/usr/bin:/bin" TMPDIR="$one" FAKE_FIXTURES="$fixtures" FAKE_LEDGER="$one/ledger" $bootstrap_shell "$bootstrap" --version v1.1.0 >"$one/out" 2>"$one/err" &
  first=$!
  env -i HOME="$home" PATH="$tools:/usr/local/bin:/usr/bin:/bin" TMPDIR="$two" FAKE_FIXTURES="$fixtures" FAKE_LEDGER="$two/ledger" $bootstrap_shell "$bootstrap" --version v1.1.0 >"$two/out" 2>"$two/err" &
  second=$!
  status_one=0 status_two=0
  wait "$first" || status_one=$?
  wait "$second" || status_two=$?
  outcomes=$(cat "$one/out" "$one/err" "$two/out" "$two/err" | grep -Eo "install_status=(installed|unchanged)|concurrent installation refused" | sort | tr "\n" ";")
  case "$outcomes" in
    "concurrent installation refused;install_status=installed;"|"install_status=installed;install_status=unchanged;") ;;
    *) printf "outcomes=%s\n" "$outcomes"; exit 1 ;;
  esac
  [[ $(installed_version "$home") == 1.1.0 ]]
  grep -Fxq "sha256=$(digest "$(bin_of "$home")")" "$(receipt_of "$home")"
  [[ ! -e "$home/.local/state/axiom/install/.axiom-install.lock" ]]
'
home=$(new_home interrupted-upgrade)
export home
step interrupted-upgrade-resumes bash -eo pipefail -c '
  run_bootstrap "$home" --version v1.0.0
  state="$home/.local/state/axiom/install"
  mkdir -m 700 "$temporary/next"
  tar -xzf "$fixtures/releases/v1.1.0/axiom-1.1.0-$row.tar.gz" -C "$temporary/next"
  cp "$temporary/next/axiom-1.1.0-$row/axiom" "$home/.local/bin/.axiom-binary-stage.test"
  chmod 700 "$home/.local/bin/.axiom-binary-stage.test"
  mv "$home/.local/bin/.axiom-binary-stage.test" "$(bin_of "$home")"
  printf "formatVersion=1\nstage=binary_committed\narchiveSha256=%s\noperation=upgrade\n" "$(asset_sha v1.1.0)" >"$state/.axiom-install-operation"
  chmod 600 "$state/.axiom-install-operation"
  refused "$home" "recovery_required" --version v1.2.0-rc.1
  run_bootstrap "$home" --version v1.1.0 || { cat "$temporary/stderr"; exit 1; }
  grep -Fxq "install_status=upgraded" "$temporary/stdout"
  [[ $(installed_version "$home") == 1.1.0 && ! -e "$state/.axiom-install-operation" ]]
  run_bootstrap "$home" --version v1.1.0
  grep -Fxq "install_status=unchanged" "$temporary/stdout"
'
home=$(new_home interrupted-install)
export home
step interrupted-first-install-requires-recovery bash -eo pipefail -c '
  env -i HOME="$home" PATH="$tools:/usr/local/bin:/usr/bin:/bin" TMPDIR="$temporary" FAKE_FIXTURES="$fixtures" FAKE_LEDGER="$temporary/ledger" AXIOM_INSTALL_TEST_FAIL_STAGE=after_binary $bootstrap_shell "$bootstrap" --version v1.1.0 >/dev/null 2>&1 && exit 1
  grep -Fxq "stage=binary_committed" "$home/.local/state/axiom/install/.axiom-install-operation"
  refused "$home" "recovery_required" --version v1.1.0
'

printf 'failures=%d\n' "$failures"
if ((failures)); then
  printf 'result=fail\n'
  exit 1
fi
printf 'result=pass\n'
