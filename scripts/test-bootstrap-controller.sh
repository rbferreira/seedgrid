#!/usr/bin/env sh
# Tests for bootstrap-controller.sh: the configuration part (the .env, the secrets, the webhook
# token and the final message), with docker, hostname and the build scripts replaced by
# stubs. What actually comes up (images, compose) is checked in the clean machine test.
#
# Usage: sh scripts/test-bootstrap-controller.sh   (runs in CI)
set -eu

SRC_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
FAILS=0
T=""
cleanup() { if [ -n "$T" ]; then rm -rf "$T"; fi; }
trap cleanup EXIT

ok() { echo "ok: $1"; }
ko() { echo "FAILED: $1"; FAILS=$((FAILS + 1)); }
check() { # check <description> <command...>
  desc="$1"; shift
  if "$@"; then ok "$desc"; else ko "$desc"; fi
}

# Fake clone: only what bootstrap reads, with stubs in place of what it executes.
# FAKE_PG_VOLUME=0 makes `docker volume inspect seedgrid-postgres-data` return "exists".
setup() {
  cleanup
  T="$(mktemp -d)"
  mkdir -p "$T/scripts" "$T/deploy/control-plane" "$T/deploy/observability" "$T/deploy/portal" "$T/stub"
  cp "$SRC_ROOT/scripts/bootstrap-controller.sh" "$T/scripts/"
  cp "$SRC_ROOT/deploy/control-plane/.env.example" "$T/deploy/control-plane/"
  for s in build-controller.sh build-portal.sh; do printf '#!/bin/sh\nexit 0\n' > "$T/scripts/$s"; done
  printf '#!/bin/sh\necho v0.0.0-test\n' > "$T/scripts/version.sh"
  cat > "$T/stub/docker" <<'EOF'
#!/bin/sh
if [ "$1" = volume ] && [ "$2" = inspect ]; then exit "${FAKE_PG_VOLUME:-1}"; fi
exit 0
EOF
  printf '#!/bin/sh\necho "192.168.0.10 "\n' > "$T/stub/hostname"
  chmod +x "$T/stub/docker" "$T/stub/hostname"
  ENVF="$T/deploy/control-plane/.env"
  TOKF="$T/deploy/observability/secrets/integration-token"
}

run() { # run [VAR=value] — output in $T/out, status in $STATUS
  STATUS=0
  (cd "$T" && env "$@" PATH="$T/stub:$PATH" sh scripts/bootstrap-controller.sh > "$T/out" 2>&1) || STATUS=$?
}

val() { sed -n "s/^$1=//p" "$ENVF" | tail -n 1; }
count() { grep -c "^$1=" "$ENVF" || true; }
is_hex() { printf '%s' "$1" | grep -qxE "[0-9a-f]{$2}"; }
mode() { stat -c %a "$1" 2>/dev/null || stat -f %Lp "$1"; }

# --- 1. Fresh install, no .env: nothing to edit beforehand -------------------------------
setup
run FAKE_PG_VOLUME=1
check "fresh installation: exits successfully" [ "$STATUS" -eq 0 ]
check "creates .env" [ -f "$ENVF" ]
check ".env with mode 600" [ "$(mode "$ENVF")" = 600 ]
check "database password generated (48 hex)" is_hex "$(val POSTGRES_PASSWORD)" 48
check "registration token generated (64 hex)" is_hex "$(val SEEDGRID_AGENT_REGISTRATION_TOKEN)" 64
check "admin password generated (24 hex)" is_hex "$(val SEEDGRID_ADMIN_PASSWORD)" 24
check "webhooks token generated (64 hex)" is_hex "$(val SEEDGRID_INTEGRATION_TOKEN)" 64
check "token file matches .env" [ "$(cat "$TOKF")" = "$(val SEEDGRID_INTEGRATION_TOKEN)" ]
check "token file with mode 644" [ "$(mode "$TOKF")" = 644 ]
for k in POSTGRES_PASSWORD SEEDGRID_AGENT_REGISTRATION_TOKEN SEEDGRID_ADMIN_PASSWORD SEEDGRID_INTEGRATION_TOKEN; do
  check "$k appears exactly once" [ "$(count "$k")" = 1 ]
done
if grep -qE '^[A-Z_]+=change-' "$ENVF"; then ko "leftover example value in .env"; else ok "no example value left"; fi
# The key stays where it was (with the comment explaining it above), not at the end of the file.
if grep -A1 '^POSTGRES_USER=' "$ENVF" | grep -q '^POSTGRES_PASSWORD='; then ok "key rewritten in place"; else ko "POSTGRES_PASSWORD moved out of place"; fi
check "shows Portal on host IP" grep -q 'Portal:  http://192.168.0.10$' "$T/out"
check "shows generated admin password" grep -q "admin / $(val SEEDGRID_ADMIN_PASSWORD)" "$T/out"
if grep -q 'http://[^ ]*:8080' "$T/out"; then ko "announces the API on a port that listens only on 127.0.0.1"; else ok "does not announce 8080 as public"; fi

# --- 2. Second run: nothing changes, and the password is not reprinted -------------------------
cp "$ENVF" "$T/env.before"; cp "$TOKF" "$T/tok.before"
run FAKE_PG_VOLUME=0
check "2nd run: exits successfully" [ "$STATUS" -eq 0 ]
check "2nd run: .env identical" cmp -s "$ENVF" "$T/env.before"
check "2nd run: token identical" cmp -s "$TOKF" "$T/tok.before"
if grep -q "$(val SEEDGRID_ADMIN_PASSWORD)" "$T/out"; then ko "2nd run reprints the password"; else ok "2nd run does not reprint password"; fi
if grep -q 'WARNING' "$T/out"; then ko "2nd run warns without reason"; else ok "2nd run without warning"; fi

# --- 3. Operator values are respected -------------------------------------------
setup
sed -e 's/^POSTGRES_PASSWORD=.*/POSTGRES_PASSWORD=operator-password/' \
    -e 's/^SEEDGRID_AGENT_REGISTRATION_TOKEN=.*/SEEDGRID_AGENT_REGISTRATION_TOKEN=operator-token/' \
    -e 's/^SEEDGRID_ADMIN_PASSWORD=.*/SEEDGRID_ADMIN_PASSWORD=operator-admin/' \
    -e 's/^SEEDGRID_INTEGRATION_TOKEN=.*/SEEDGRID_INTEGRATION_TOKEN=operator-webhook/' \
    "$T/deploy/control-plane/.env.example" > "$ENVF"
run FAKE_PG_VOLUME=1
check "operator values: exits successfully" [ "$STATUS" -eq 0 ]
check "operator database password kept" [ "$(val POSTGRES_PASSWORD)" = operator-password ]
check "operator registration token kept" [ "$(val SEEDGRID_AGENT_REGISTRATION_TOKEN)" = operator-token ]
check "operator admin password kept" [ "$(val SEEDGRID_ADMIN_PASSWORD)" = operator-admin ]
check "operator webhook token goes to file" [ "$(cat "$TOKF")" = operator-webhook ]
if grep -q 'operator-admin' "$T/out"; then ko "prints the password the operator chose"; else ok "does not print the operator password"; fi

# --- 4. Database already exists and .env still has example values: doesn't change, warns -----
setup
cp "$T/deploy/control-plane/.env.example" "$ENVF"
run FAKE_PG_VOLUME=0
check "existing database: exits successfully" [ "$STATUS" -eq 0 ]
check "existing database: database password is NOT changed" [ "$(val POSTGRES_PASSWORD)" = change-this-password ]
check "existing database: admin password is NOT changed" [ "$(val SEEDGRID_ADMIN_PASSWORD)" = change-this-admin-password ]
check "existing database: warns" grep -q 'WARNING: POSTGRES_PASSWORD' "$T/out"
check "existing database: webhook token is still generated" is_hex "$(val SEEDGRID_INTEGRATION_TOKEN)" 64

# --- 5. Token file without the line in .env: the file is the source ---------------------
setup
cp "$T/deploy/control-plane/.env.example" "$ENVF"
mkdir -p "$(dirname "$TOKF")"; printf 'token-already-in-the-file' > "$TOKF"
run FAKE_PG_VOLUME=1
check "token from file goes to .env" [ "$(val SEEDGRID_INTEGRATION_TOKEN)" = token-already-in-the-file ]

# --- 6. The validate command follows who installed (#26) ------------------------------
setup
run FAKE_PG_VOLUME=1 SUDO_USER=ubuntu
check "with sudo: instructs to validate with sudo" grep -q "Validate: sudo sh scripts/acceptance.sh" "$T/out"
setup
(cd "$T" && env -u SUDO_USER FAKE_PG_VOLUME=1 PATH="$T/stub:$PATH" sh scripts/bootstrap-controller.sh > "$T/out" 2>&1) || true
check "without sudo: instructs to validate without sudo" grep -q "Validate: sh scripts/acceptance.sh" "$T/out"

# --- 7. acceptance.sh with unreadable .env says what to do -----------------------------
# You can only make the file unreadable when the test doesn't run as root (root reads everything).
if [ "$(id -u)" -ne 0 ]; then
  setup
  cp "$SRC_ROOT/scripts/acceptance.sh" "$T/scripts/"
  cp "$T/deploy/control-plane/.env.example" "$ENVF"; chmod 000 "$ENVF"
  STATUS=0; (cd "$T" && env -u ADMIN_PASS sh scripts/acceptance.sh > "$T/out" 2>&1) || STATUS=$?
  chmod 600 "$ENVF"
  check "acceptance with unreadable .env: exits 2" [ "$STATUS" -eq 2 ]
  check "acceptance with unreadable .env: instructs to run with sudo" grep -q "Run: sudo sh scripts/acceptance.sh" "$T/out"
else
  echo "skipped (root reads any file): acceptance with unreadable .env"
fi

if [ "$FAILS" -eq 0 ]; then
  echo "== bootstrap-controller: all tests passed"
else
  echo "== bootstrap-controller: $FAILS failure(s)"
  exit 1
fi
