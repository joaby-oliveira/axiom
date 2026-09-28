#!/usr/bin/env bash
# Maps one public release tag to the semantic version recorded in release
# metadata. Tags are vMAJOR.MINOR.PATCH (stable) or vMAJOR.MINOR.PATCH-rc.N
# (release candidate); the semantic version is the tag without its leading v.
set -euo pipefail

if (($# != 1)); then
  printf 'release_tag_error: exactly one tag required\n' >&2
  exit 1
fi
tag=$1
numeric='(0|[1-9][0-9]*)'
if [[ ! "$tag" =~ ^v${numeric}\.${numeric}\.${numeric}(-rc\.${numeric})?$ ]]; then
  printf 'release_tag_error: tag must be vMAJOR.MINOR.PATCH or vMAJOR.MINOR.PATCH-rc.N\n' >&2
  exit 1
fi
channel=stable
[[ "$tag" == *-rc.* ]] && channel=rc
printf 'tag=%s\nversion=%s\nchannel=%s\n' "$tag" "${tag#v}" "$channel"
