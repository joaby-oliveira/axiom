#!/usr/bin/env bash
# Maintainer Evidence collector. Default: plan only. No Runtime/Provider writes.
set -euo pipefail
exec python3 "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)/s9-rc-evidence.py" "$@"
