#!/usr/bin/env sh
# Tests for check-homelab.sh in a disposable git repository: each mark from the list is caught,
# the generic equivalents pass, and the documentation and the CHANGELOG are also checked.
#
# Usage: sh scripts/test-check-homelab.sh   (runs in CI)
set -eu

SRC="$(cd "$(dirname "$0")" && pwd)"
T="$(mktemp -d)"
trap 'rm -rf "$T"' EXIT
FAILS=0

mkdir -p "$T/scripts" "$T/docs" "$T/src"
cp "$SRC/check-homelab.sh" "$SRC/homelab-markers.txt" "$T/scripts/"
git -C "$T" init -q

run() { (cd "$T" && sh scripts/check-homelab.sh >"$T/out" 2>&1); }

# expect <description> <code file contents> <0|1>
expect() {
  printf '%s\n' "$2" >"$T/src/case.txt"
  git -C "$T" add -A
  status=0; run || status=$?
  if [ "$status" -ne "$3" ]; then
    echo "FAILED: $1 — exit $status, expected $3"; sed 's/^/    /' "$T/out"; FAILS=$((FAILS + 1))
  else
    echo "ok: $1"
  fi
}

expect "clean repository passes" "nothing here" 0
expect "original environment IPv4 is matched" "SEEDGRID_CONTROL_PLANE_URL=http://172.16.1.113" 1
expect "another 172.16.x IPv4 too" "ip(\"172.16.0.162\")" 1
expect "original environment IPv6 prefix is matched" "addr := \"2804:13c:60b:c000::1\"" 1
expect "machine number (CT 113) is matched" "# Usage (on CT 113): docker compose up -d" 1
expect "attached machine number (CT113) is matched" "deploy user on CT113" 1
expect "self-hosted runner is matched" "runs-on: [self-hosted, seedgrid-only]" 1
expect "original environment domain is matched" "https://mcp.rafalab.dev" 1
expect "generic private range passes" "SEEDGRID_CONTROL_PLANE_URL=http://192.168.0.113" 0
expect "documentation prefix passes" "addr := \"2001:db8:60b:c000::1\"" 0
expect "\"CTA\"/\"ACT 120\" are not machine numbers" "CTA 123 e ACT 120 e FACT 199" 0
expect "example host passes" "http://controller.example/node" 0

# The output shows the file, the line and what to use instead.
printf 'x\nhttp://172.16.1.113\n' >"$T/src/case.txt"; git -C "$T" add -A; run || true
if grep -q '^src/case.txt:2:' "$T/out" && grep -q 'use: 192.168.0.x' "$T/out"; then
  echo "ok: output points to file:line and the replacement"
else
  echo "FAILED: output missing file:line or replacement"; sed 's/^/    /' "$T/out"; FAILS=$((FAILS + 1))
fi

# docs/, README and CHANGELOG.md are also checked.
printf 'nothing\n' >"$T/src/case.txt"; printf 'Na CT 113, 172.16.1.113\n' >"$T/docs/deploy.md"
git -C "$T" add -A
status=0; run || status=$?
if [ "$status" -eq 1 ] && grep -q '^docs/deploy.md:1:' "$T/out"; then echo "ok: docs/ is checked"; else echo "FAILED: docs/ should be checked (exit $status)"; FAILS=$((FAILS + 1)); fi
rm -f "$T/docs/deploy.md"; printf 'Deploy na CT 113 (172.16.1.113)\n' >"$T/README.md"; git -C "$T" add -A
status=0; run || status=$?
if [ "$status" -eq 1 ] && grep -q '^README.md:1:' "$T/out"; then echo "ok: README.md is checked"; else echo "FAILED: README.md should be checked (exit $status)"; FAILS=$((FAILS + 1)); fi
rm -f "$T/README.md"; printf '## v1.0\n- deploy on CT 113 (172.16.1.113)\n' >"$T/CHANGELOG.md"; git -C "$T" add -A
status=0; run || status=$?
if [ "$status" -eq 1 ] && grep -q '^CHANGELOG.md:2:' "$T/out"; then echo "ok: CHANGELOG.md is checked"; else echo "FAILED: CHANGELOG.md should be checked (exit $status)"; FAILS=$((FAILS + 1)); fi
rm -f "$T/CHANGELOG.md"; git -C "$T" add -A

# Untracked files don't count (CI only sees what's in the repository).
printf 'http://172.16.1.113\n' >"$T/src/loose.txt"
status=0; run || status=$?
if [ "$status" -eq 0 ]; then echo "ok: untracked file does not count"; else echo "FAILED: untracked file counted"; FAILS=$((FAILS + 1)); fi

[ "$FAILS" -eq 0 ] && echo "== check-homelab: all tests passed" || { echo "== check-homelab: $FAILS failure(s)"; exit 1; }
