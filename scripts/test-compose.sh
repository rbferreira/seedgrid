#!/usr/bin/env sh
# Starts deploy/compose.yaml from scratch and checks the Control Plane it installs (ADR-042, #30).
#
# Usage: SEEDGRID_VERSION=<image tag> [EXPECT_VERSION=<binary version>] sh scripts/test-compose.sh
#
# The images ghcr.io/rbferreira/seedgrid-{controller,portal}:$SEEDGRID_VERSION must exist
# locally or in the registry. CI builds them from the commit under test.
#
# ⚠️ DESTRUCTIVE: it creates the stack with the real names (containers seedgrid-*, volumes
# seedgrid-*) and removes it WITH its volumes at the end. It refuses to start when a SeedGrid
# database volume already exists, so it never touches a real install.
set -eu

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
: "${SEEDGRID_VERSION:?set SEEDGRID_VERSION to the image tag under test}"
PORT="${SEEDGRID_HTTP_PORT:-8088}"
BASE="http://127.0.0.1:$PORT"
ADMIN_PASS="test-$(head -c 8 /dev/urandom | od -An -tx1 | tr -d ' \n')"

fail() { echo "FAIL: $*" >&2; exit 1; }
ok() { echo "ok: $*"; }

if docker volume inspect seedgrid-postgres-data >/dev/null 2>&1; then
  fail "a seedgrid-postgres-data volume exists on this host; this test would destroy it"
fi

WORK="$(mktemp -d)"
cp "$REPO_ROOT/deploy/compose.yaml" "$WORK/compose.yaml"
cat > "$WORK/.env" <<EOF
SEEDGRID_ADMIN_PASSWORD=$ADMIN_PASS
SEEDGRID_VERSION=$SEEDGRID_VERSION
SEEDGRID_HTTP_PORT=$PORT
EOF
dc() { docker compose --project-directory "$WORK" -f "$WORK/compose.yaml" "$@"; }

cleanup() {
  dc down -v --remove-orphans >/dev/null 2>&1 || true
  rm -rf "$WORK"
}
trap cleanup EXIT

wait_healthz() {
  i=0
  until [ "$(curl -fsS "$BASE/healthz" 2>/dev/null | jq -r .status 2>/dev/null)" = ok ]; do
    i=$((i + 1))
    [ "$i" -le 90 ] || { dc ps -a; dc logs --tail 50; fail "healthz did not answer ok"; }
    sleep 2
  done
}

login() {
  curl -fsS -X POST "$BASE/api/v1/auth/login" -H 'Content-Type: application/json' \
    -d "{\"username\":\"admin\",\"password\":\"$ADMIN_PASS\"}" | jq -r .token
}

secrets_sum() {
  docker run --rm -v seedgrid-secrets:/s:ro alpine:3.20 sh -c 'cd /s && sha256sum *'
}

# 2. Fresh install.
dc up -d
wait_healthz
ok "healthz ok on port $PORT"

if [ -n "${EXPECT_VERSION:-}" ]; then
  v="$(curl -fsS "$BASE/healthz" | jq -r .version)"
  [ "$v" = "$EXPECT_VERSION" ] || fail "healthz version is $v, want $EXPECT_VERSION"
  ok "controller version $v"
fi

TOKEN="$(login)"
[ -n "$TOKEN" ] && [ "$TOKEN" != null ] || fail "admin login with SEEDGRID_ADMIN_PASSWORD"
ok "admin login"

curl -fsS "$BASE/node/agents.json" | jq -e '.binaries.amd64.sha256 and .binaries.arm64.sha256' >/dev/null \
  || fail "the Portal does not serve /node/agents.json"
curl -fsS -o /dev/null "$BASE/node/bootstrap-node.sh" || fail "the Portal does not serve /node/bootstrap-node.sh"
ok "Node artifacts served at /node/"

# 3. The configuration the containers read is a copy of deploy/observability/*.yml. Read from
# inside them: `docker compose config` prints `$$` still escaped, the containers get `$`.
for spec in prometheus:/etc/prometheus/prometheus.yml:prometheus.yml             prometheus:/etc/prometheus/rules.yml:rules.yml             alertmanager:/etc/alertmanager/alertmanager.yml:alertmanager.yml; do
  svc="${spec%%:*}"
  rest="${spec#*:}"
  path="${rest%%:*}"
  file="${rest#*:}"
  dc exec -T "$svc" cat "$path" > "$WORK/inline"
  diff -u "$REPO_ROOT/deploy/observability/$file" "$WORK/inline"     || fail "$path in the $svc container differs from deploy/observability/$file"
done
ok "inline configuration matches deploy/observability"

# 4. Generated secrets: present, and readable only by the container that needs them.
perms="$(docker run --rm -v seedgrid-secrets:/s:ro alpine:3.20 sh -c 'cd /s && stat -c "%n %a %u" *')"
echo "$perms"
for want in "postgres-password 400 999" "database-url 400 65532" "agent-registration-token 400 65532" \
            "integration-token 444 0"; do
  echo "$perms" | grep -qx "$want" || fail "secret file is not '$want'"
done
ok "secret files and their owners"

# 5. Alertmanager reads the webhook token, and the controller requires it.
HOOK="$(dc exec -T alertmanager cat /etc/alertmanager/secrets/integration-token)"
[ -n "$HOOK" ] || fail "Alertmanager cannot read the webhook token"
code="$(curl -s -o /dev/null -w '%{http_code}' -X POST "$BASE/api/v1/integrations/alertmanager" \
  -H 'Content-Type: application/json' -d '{"alerts":[]}')"
[ "$code" = 401 ] || fail "webhook without the token answered $code, want 401"
code="$(curl -s -o /dev/null -w '%{http_code}' -X POST "$BASE/api/v1/integrations/alertmanager" \
  -H 'Content-Type: application/json' -H "Authorization: Bearer $HOOK" -d '{"alerts":[]}')"
case "$code" in 2*) ;; *) fail "webhook with Alertmanager's token answered $code" ;; esac
ok "webhook token shared by Alertmanager and the controller"

# 6. Prometheus scrapes the controller, this host's node-exporter and itself.
i=0
until dc exec -T prometheus wget -qO- http://localhost:9090/api/v1/targets 2>/dev/null \
  | jq -e '[.data.activeTargets[] | select(.health == "up") | .labels.job] | unique == ["node","prometheus","seedgrid-controller"]' >/dev/null; do
  i=$((i + 1))
  [ "$i" -le 30 ] || { dc exec -T prometheus wget -qO- http://localhost:9090/api/v1/targets | jq '.data.activeTargets[] | {job: .labels.job, health, lastError}'; fail "Prometheus targets not up"; }
  sleep 3
done
ok "Prometheus targets up (controller, node-exporter, prometheus)"

# 7. Only the Portal publishes a port.
published="$(dc ps --format json | jq -rs 'map(select((.Publishers // []) | map(select(.PublishedPort > 0)) | length > 0) | .Service) | join(" ")')"
[ "$published" = portal ] || fail "services publishing a port: '$published', want only portal"
ok "only the Portal publishes a port"

# 8. A restart keeps the secrets and the database.
before="$(secrets_sum)"
dc down
dc up -d
wait_healthz
[ "$(secrets_sum)" = "$before" ] || fail "the secrets changed across a restart"
TOKEN="$(login)"
[ -n "$TOKEN" ] && [ "$TOKEN" != null ] || fail "admin login after a restart"
ok "restart keeps the secrets, the database and the login"

echo "== compose: all tests passed"
