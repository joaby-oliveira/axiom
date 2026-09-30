#!/usr/bin/env bash
# Verifies one complete Axiom release artifact set produced by
# build-release-archives.sh against the exact checked-out source revision.
# Read-only: it extracts into a private temporary directory, never publishes,
# and prints closed key=value Evidence on success.
set -euo pipefail

umask 077

repository_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)
directory=
version=
revision=
while (($#)); do
  case "$1" in
    --dir) directory=${2:-}; shift 2 ;;
    --version) version=${2:-}; shift 2 ;;
    --revision) revision=${2:-}; shift 2 ;;
    *) printf 'release_verify_error: invalid argument\n' >&2; exit 1 ;;
  esac
done

fail() {
  printf 'release_verify_error: %s\n' "$1" >&2
  exit 1
}

[[ "$directory" == /* && -d "$directory" && ! -L "$directory" ]] || fail 'absolute artifact directory required'
[[ "$version" =~ ^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*)?$ ]] || fail 'exact semantic version required'
[[ "$revision" =~ ^[0-9a-f]{40}$ ]] || fail 'full source revision required'
[[ $(git -C "$repository_root" rev-parse --verify HEAD) == "$revision" ]] || fail 'checkout is not the artifact source revision'
[[ -z $(git -C "$repository_root" status --porcelain --untracked-files=normal) ]] || fail 'checkout is not clean'
command -v go >/dev/null 2>&1 || fail 'go toolchain required to read embedded build information'

digest() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  else
    shasum -a 256 "$1" | awk '{print $1}'
  fi
}

rows='macos-27:darwin:arm64 linux:linux:amd64 linux:linux:arm64 windows:windows:amd64'
skill_names='axiom-project-configure axiom-project-show axiom-work-item-create axiom-work-item-run axiom-work-item-status'
temporary=$(mktemp -d)
trap 'rm -rf -- "$temporary"' EXIT

# Closed top-level set: SHA256SUMS plus exactly one archive per supported row.
{
  printf 'SHA256SUMS\n'
  for row in $rows; do
    IFS=: read -r platform _ architecture <<<"$row"
    printf 'axiom-%s-%s-%s.tar.gz\n' "$version" "$platform" "$architecture"
  done
} | LC_ALL=C sort >"$temporary/expected-files"
find "$directory" -mindepth 1 -maxdepth 1 -exec basename {} \; | LC_ALL=C sort >"$temporary/actual-files"
cmp -s "$temporary/expected-files" "$temporary/actual-files" || fail 'artifact set is not exactly SHA256SUMS plus one archive per supported row'
while IFS= read -r name; do
  [[ -f "$directory/$name" && ! -L "$directory/$name" ]] || fail "artifact is not a regular file: $name"
done <"$temporary/actual-files"

checksums="$directory/SHA256SUMS"
[[ $(wc -l <"$checksums" | tr -d ' ') == 4 ]] || fail 'SHA256SUMS must list exactly the four archives'
grep -v '^SHA256SUMS$' "$temporary/expected-files" >"$temporary/expected-archives"
awk '{print $2}' "$checksums" | LC_ALL=C sort >"$temporary/listed-archives"
cmp -s "$temporary/expected-archives" "$temporary/listed-archives" || fail 'SHA256SUMS names do not match the archive set'
while IFS= read -r line; do
  [[ "$line" =~ ^[0-9a-f]{64}\ \ axiom-[0-9A-Za-z.-]+\.tar\.gz$ ]] || fail 'SHA256SUMS line malformed'
  [[ $(digest "$directory/${line#*  }") == "${line%%  *}" ]] || fail "checksum mismatch: ${line#*  }"
done <"$checksums"

printf 'evidenceVersion=1\n'
printf 'product=Axiom\n'
printf 'version=%s\n' "$version"
printf 'revision=%s\n' "$revision"
printf 'sha256sums=%s\n' "$(digest "$checksums")"

for row in $rows; do
  IFS=: read -r platform goos architecture <<<"$row"
  executable=axiom
  installer=install.sh
  installer_source=install-release.sh
  if [[ "$goos" == windows ]]; then
    executable=axiom.exe
    installer=install.ps1
    installer_source=install-release.ps1
  fi
  bundle="axiom-$version-$platform-$architecture"
  archive="$directory/$bundle.tar.gz"

  # Closed entry set with regular files and directories only.
  {
    printf '%s\n' "$bundle" "$bundle/LICENSE" "$bundle/MANIFEST.sha256" "$bundle/$executable" "$bundle/$installer" \
      "$bundle/release-metadata.txt" "$bundle/skills" "$bundle/skills-manifest.txt"
    for name in $skill_names; do
      printf '%s\n' "$bundle/skills/$name" "$bundle/skills/$name/SKILL.md"
    done
  } | LC_ALL=C sort >"$temporary/expected-entries"
  tar -tzf "$archive" | sed 's#/$##' | LC_ALL=C sort >"$temporary/actual-entries"
  cmp -s "$temporary/expected-entries" "$temporary/actual-entries" || fail "archive entries are not the closed bundle set: $bundle"
  tar -tvzf "$archive" | awk 'substr($1,1,1) != "-" && substr($1,1,1) != "d" {exit 1}' || fail "archive links or special files: $bundle"
  [[ $(tar -tvzf "$archive" | awk -v path="$bundle/$executable" '$NF == path {print $1}') == -rwx------ ]] || fail "axiom executable mode is not 0700: $bundle"

  extracted="$temporary/extract-$platform-$architecture"
  mkdir "$extracted"
  tar -xzf "$archive" -C "$extracted"
  root="$extracted/$bundle"

  (
    cd "$root"
    find . -type f ! -path './MANIFEST.sha256' -print | sed 's#^./##' | LC_ALL=C sort >"$temporary/bundle-files"
    awk '{print $2}' MANIFEST.sha256 | LC_ALL=C sort >"$temporary/manifest-files"
    cmp -s "$temporary/bundle-files" "$temporary/manifest-files" || exit 1
    while read -r hash name extra; do
      [[ -z "${extra:-}" && "$hash" =~ ^[0-9a-f]{64}$ && $(digest "$name") == "$hash" ]] || exit 1
    done <MANIFEST.sha256
  ) || fail "bundle MANIFEST.sha256 is incomplete or wrong: $bundle"

  printf 'formatVersion=1\nproduct=Axiom\nversion=%s\nrevision=%s\nsourceState=clean\nrelease=true\nplatform=%s\ngoos=%s\narchitecture=%s\nskillSetVersion=1\n' \
    "$version" "${revision:0:12}" "$platform" "$goos" "$architecture" >"$temporary/expected-metadata"
  cmp -s "$temporary/expected-metadata" "$root/release-metadata.txt" || fail "release metadata does not match version/revision/row: $bundle"

  cmp -s "$repository_root/LICENSE" "$root/LICENSE" || fail "LICENSE differs from source: $bundle"
  cmp -s "$repository_root/scripts/$installer_source" "$root/$installer" || fail "installer differs from source: $bundle"
  (
    printf 'formatVersion=1\nskillSetVersion=1\nbinaryCompatibility=1\n'
    for skill in "$repository_root"/internal/codexruntime/skills/*; do
      name=$(basename "$skill")
      cmp -s "$skill/SKILL.md" "$root/skills/$name/SKILL.md" || exit 1
      printf 'skill.%s=%s\n' "$name" "$(digest "$skill/SKILL.md")"
    done
  ) >"$temporary/expected-skills" || fail "skill differs from source: $bundle"
  cmp -s "$temporary/expected-skills" "$root/skills-manifest.txt" || fail "skills-manifest.txt differs from source: $bundle"

  # Executable format for the declared row, read from the file header.
  header=$(od -An -tx1 -N20 "$root/$executable" | tr -d ' \n')
  case "$goos:$architecture" in
    darwin:arm64) [[ "$header" == cffaedfe0c000001* ]] ;;
    linux:amd64) [[ "$header" == 7f454c46020101* && "${header:36:4}" == 3e00 ]] ;;
    linux:arm64) [[ "$header" == 7f454c46020101* && "${header:36:4}" == b700 ]] ;;
    windows:amd64) [[ "$header" == 4d5a* ]] ;;
    *) false ;;
  esac || fail "axiom executable format does not match $goos/$architecture: $bundle"

  # Embedded Go build information binds the executable to this revision.
  go version -m "$root/$executable" >"$temporary/buildinfo"
  tab=$'\t'
  for expected in "build${tab}GOOS=$goos" "build${tab}GOARCH=$architecture" "build${tab}CGO_ENABLED=0" "build${tab}-trimpath=true" "build${tab}vcs.revision=$revision" "build${tab}vcs.modified=false" "path${tab}github.com/rgomids/axiom/cmd/lingo"; do
    grep -Fxq -- "${tab}$expected" "$temporary/buildinfo" || fail "embedded build information lacks '$expected': $bundle"
  done
  LC_ALL=C grep -aFq -- "$version" "$root/$executable" || fail "executable does not embed the release version: $bundle"

  # The executable for this host's row reports its exact provenance.
  smoke=not_host_architecture
  if [[ $(go env GOHOSTOS):$(go env GOHOSTARCH) == "$goos:$architecture" ]]; then
    (cd / && "$root/$executable" --json version) >"$temporary/version.json" || fail "axiom version failed: $bundle"
    grep -Fq '"status":"success","result":"Axiom build information","provenance":{"product":"Axiom","version":"'"$version"'","revision":"'"${revision:0:12}"'","sourceState":"clean"}' "$temporary/version.json" \
      || fail "axiom version provenance mismatch: $bundle"
    smoke=pass
  fi

  printf 'archive=%s sha256=%s axiom_sha256=%s manifest_sha256=%s version_smoke=%s\n' \
    "$bundle.tar.gz" "$(digest "$archive")" "$(digest "$root/$executable")" "$(digest "$root/MANIFEST.sha256")" "$smoke"
done

printf 'publication=none\n'
printf 'result=pass\n'
