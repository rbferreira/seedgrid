#!/usr/bin/env sh
# Fails if a marker from the original development environment (IPs, IPv6 prefix, machine
# number, self-hosted runner) returns to the repository. They were added one by one, in tests and
# comments, and nothing prevented the next (#5). The list is kept in scripts/homelab-markers.txt.
#
# Usage: sh scripts/check-homelab.sh   (from anywhere inside the repository; runs in CI)
set -eu

ROOT="$(git rev-parse --show-toplevel)"
cd "$ROOT"
LIST=scripts/homelab-markers.txt
TAB="$(printf '\t')"
FAIL=0

# Excluded from the check: the list itself and the scripts that reference it. Code,
# configuration, README, CHANGELOG and docs/ are all checked.
while IFS="$TAB" read -r pat hint; do
  case "$pat" in '' | '#'*) continue ;; esac
  status=0
  hits="$(git grep -nIE -e "$pat" -- . \
    ':!scripts/homelab-markers.txt' ':!scripts/check-homelab.sh' ':!scripts/test-check-homelab.sh')" || status=$?
  # git grep: 0 = found, 1 = not found, >1 = error (e.g.: invalid regex) — errors don't pass silently.
  if [ "$status" -gt 1 ]; then
    echo "ERROR: git grep failed (status $status) with pattern: $pat" >&2
    exit 2
  fi
  if [ -n "$hits" ]; then
    printf '%s\n' "$hits" | while IFS= read -r line; do
      printf '%s\n    -> marca do ambiente original; use: %s\n' "$line" "$hint"
    done
    FAIL=1
  fi
done < "$LIST"

if [ "$FAIL" -ne 0 ]; then
  echo "check-homelab: there are traces of the original environment (list in $LIST)." >&2
  exit 1
fi
echo "check-homelab: ok"
