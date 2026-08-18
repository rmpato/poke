#!/usr/bin/env bash
# Regenerate internal/curlargs/options.go from the curl binary on this machine.
#
# Arity is established empirically rather than by parsing help text: curl's
# --help output does not mark arguments consistently across versions, but
# invoking an option bare makes curl tell us the truth:
#
#     $ curl --proxy
#     curl: option --proxy: requires parameter
#
# Usage: scripts/gen-curl-options.sh [path-to-curl] > /tmp/arity.txt
set -euo pipefail
CURL="${1:-curl}"

"$CURL" --help all 2>/dev/null |
  grep -Eo '^[[:space:]]+(-[a-zA-Z0-9#:*], )?--[a-zA-Z0-9.-]+' |
  sed -E 's/^[[:space:]]+//' |
  tr ',' '\n' | tr -d ' ' | grep -E '^-' | sort -u |
  while read -r opt; do
    # NB: capture into a variable rather than piping into `grep -q`; under
    # `pipefail` the early exit of grep -q SIGPIPEs head and poisons the status.
    # `|| true`: curl exits non-zero here by design and `set -e` would abort.
    msg=$("$CURL" "$opt" 2>&1 >/dev/null | head -1) || true
    case "$msg" in
      *"requires parameter"*) echo "$opt V" ;;
      *) echo "$opt B" ;;
    esac
  done
