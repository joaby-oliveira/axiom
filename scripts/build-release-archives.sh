#!/usr/bin/env bash
set -euo pipefail

umask 077

repository_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)
version=
output=
development=false

while (($#)); do
  case "$1" in
    --version) version=${2:-}; shift 2 ;;
    --output) output=${2:-}; shift 2 ;;
    --development) development=true; shift ;;
    *) printf 'release_build_error: invalid argument\n' >&2; exit 1 ;;
  esac
done

if [[ ! "$version" =~ ^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*)?(\+[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*)?$ ]]; then
  printf 'release_build_error: exact semantic version required\n' >&2
  exit 1
fi
without_build=${version%%+*}
if [[ "$without_build" == *-* ]]; then
  prerelease=${without_build#*-}
  IFS=. read -r -a identifiers <<<"$prerelease"
  for identifier in "${identifiers[@]}"; do
    if [[ "$identifier" =~ ^[0-9]+$ && ${#identifier} -gt 1 && "$identifier" == 0* ]]; then
      printf 'release_build_error: exact semantic version required\n' >&2
      exit 1
    fi
  done
fi
if [[ "$output" != /* || "$output" == / ]]; then
  printf 'release_build_error: absolute output directory required\n' >&2
  exit 1
fi

revision=$(git -C "$repository_root" rev-parse --verify HEAD)
source_state=clean
release=true
if [[ -n $(git -C "$repository_root" status --porcelain --untracked-files=normal) ]]; then
  source_state=dirty
fi
if [[ "$development" == true ]]; then
  release=false
elif [[ "$source_state" != clean ]]; then
  printf 'release_build_error: release archive requires clean source\n' >&2
  exit 1
fi

if [[ -e "$output" && ! -d "$output" ]]; then
  printf 'release_build_error: output is not a directory\n' >&2
  exit 1
fi
mkdir -p -- "$output"
if find "$output" -mindepth 1 -maxdepth 1 -print -quit | grep -q .; then
  printf 'release_build_error: output directory must be empty\n' >&2
  exit 1
fi
work=$(mktemp -d)
trap 'rm -rf -- "$work"' EXIT
checksums="$output/SHA256SUMS"
: >"$checksums"

digest() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  else
    shasum -a 256 "$1" | awk '{print $1}'
  fi
}

build_target() {
  local platform=$1 goos=$2 arch=$3
  local bundle="axiom-${version}-${platform}-${arch}"
  local root="$work/$bundle"
  mkdir -p "$root/skills"
  # Build from the repository root so the module is this checkout, not the
  # caller's working directory.
  (cd "$repository_root" && GOOS="$goos" GOARCH="$arch" CGO_ENABLED=0 go build -trimpath \
    -ldflags "-s -w -X main.buildVersion=$version -X main.buildRevision=${revision:0:12} -X main.buildSourceState=$source_state -X main.buildRelease=$release" \
    -o "$root/axiom" ./cmd/lingo)
  chmod 700 "$root/axiom"
  cp "$repository_root/LICENSE" "$root/LICENSE"
  cp "$repository_root/scripts/install-release.sh" "$root/install.sh"
  chmod 700 "$root/install.sh"
  local skills_manifest="$root/skills-manifest.txt"
  {
    printf 'formatVersion=1\n'
    printf 'skillSetVersion=1\n'
    printf 'binaryCompatibility=1\n'
    for skill in "$repository_root"/internal/codexruntime/skills/*; do
      local name
      name=$(basename "$skill")
      mkdir -p "$root/skills/$name"
      cp "$skill/SKILL.md" "$root/skills/$name/SKILL.md"
      printf 'skill.%s=%s\n' "$name" "$(digest "$root/skills/$name/SKILL.md")"
    done
  } >"$skills_manifest"
  {
    printf 'formatVersion=1\n'
    printf 'product=Axiom\n'
    printf 'version=%s\n' "$version"
    printf 'revision=%s\n' "${revision:0:12}"
    printf 'sourceState=%s\n' "$source_state"
    printf 'release=%s\n' "$release"
    printf 'platform=%s\n' "$platform"
    printf 'goos=%s\n' "$goos"
    printf 'architecture=%s\n' "$arch"
    printf 'skillSetVersion=1\n'
  } >"$root/release-metadata.txt"
  {
    printf '%s  axiom\n' "$(digest "$root/axiom")"
    printf '%s  LICENSE\n' "$(digest "$root/LICENSE")"
    printf '%s  install.sh\n' "$(digest "$root/install.sh")"
    printf '%s  release-metadata.txt\n' "$(digest "$root/release-metadata.txt")"
    printf '%s  skills-manifest.txt\n' "$(digest "$skills_manifest")"
    while IFS= read -r file; do
      relative=${file#"$root/"}
      printf '%s  %s\n' "$(digest "$file")" "$relative"
    done < <(find "$root/skills" -type f | LC_ALL=C sort)
  } >"$root/MANIFEST.sha256"
  local archive="$output/$bundle.tar.gz"
  # macOS bsdtar otherwise embeds AppleDouble ._* entries for extended
  # attributes; they are unlisted in MANIFEST.sha256 and break other hosts.
  COPYFILE_DISABLE=1 tar -C "$work" -czf "$archive" "$bundle"
  printf '%s  %s\n' "$(digest "$archive")" "$(basename "$archive")" >>"$checksums"
}

build_target macos-27 darwin arm64
build_target ubuntu-26.04 linux amd64
build_target ubuntu-26.04 linux arm64

printf 'release_build_success: %s\n' "$output"
