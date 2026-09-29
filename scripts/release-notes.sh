#!/usr/bin/env bash
# Prints the deterministic GitHub Release body for one tag and exact revision.
# Stable notes are the CHANGELOG.md section Release Please wrote for that
# version at the revision; release-candidate notes are fixed text. Both end
# with the source revision, the exact-version install command and the
# integrity-only checksum statement. Read-only.
set -euo pipefail

repository_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)
tag=
revision=
repository=
while (($#)); do
  case "$1" in
    --tag) tag=${2:-}; shift 2 ;;
    --revision) revision=${2:-}; shift 2 ;;
    --repo) repository=${2:-}; shift 2 ;;
    *) printf 'release_notes_error: invalid argument\n' >&2; exit 1 ;;
  esac
done

fail() {
  printf 'release_notes_error: %s\n' "$1" >&2
  exit 1
}

tag_facts=$("$repository_root/scripts/release-tag-version.sh" "$tag") || exit 1
version=$(awk -F= '$1 == "version" {print $2}' <<<"$tag_facts")
channel=$(awk -F= '$1 == "channel" {print $2}' <<<"$tag_facts")
[[ "$revision" =~ ^[0-9a-f]{40}$ ]] || fail 'full source revision required'
[[ "$repository" =~ ^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$ ]] || fail 'repository must be OWNER/NAME'

if [[ "$channel" == stable ]]; then
  changelog=$(git -C "$repository_root" show "$revision:CHANGELOG.md" 2>/dev/null) || fail 'CHANGELOG.md missing at revision'
  # Release Please headings: "## [X.Y.Z](compare-url) (date)" or "## X.Y.Z (date)".
  section=$(awk -v version="$version" '
    BEGIN { plain = "## " version " "; linked = "## [" version "]" }
    found && /^## / { exit }
    found { print; next }
    index($0, plain) == 1 || index($0, linked) == 1 { found = 1 }
  ' <<<"$changelog")
  [[ -n "${section//[[:space:]]/}" ]] || fail "CHANGELOG.md has no Release Please section for $version"
  printf '%s\n' "$section" | sed -e '/./,$!d'
else
  printf 'Release candidate %s. It is a GitHub prerelease: the default and\n' "$tag"
  printf '`--channel stable` installer selectors never choose it; install it only by\n'
  printf 'its exact tag. It is not a stable release and records no human acceptance.\n'
fi

printf '\n---\n\n'
printf -- '- Source revision: `%s`\n' "$revision"
printf -- '- Channel: %s\n' "$([[ "$channel" == rc ]] && printf 'release candidate (prerelease)' || printf 'stable')"
printf -- '- Assets: `SHA256SUMS` and one `axiom-%s-<row>.tar.gz` archive per supported row\n' "$version"
printf '\nInstall exactly this release:\n\n'
printf '```bash\ncurl -fsSL https://raw.githubusercontent.com/%s/main/scripts/install.sh | sh -s -- --version %s\n```\n' "$repository" "$tag"
printf '\nThe installer verifies each archive against `SHA256SUMS` before reading it.\n'
printf 'Checksums provide integrity only; they are not a signature or a publisher\n'
printf 'authenticity claim.\n'
