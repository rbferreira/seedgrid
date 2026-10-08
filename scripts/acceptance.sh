#!/usr/bin/env sh
# End-to-end acceptance checklist for SeedGrid (Phase 7 / Deployment.md).
# Runs against an already running environment and validates the happy path. Exits != 0 if anything fails.
#
# Usage (on the Control Plane host, as root or with sudo): sh scripts/acceptance.sh
#   or:  API=http://127.0.0.1:8080 ADMIN_USER=admin ADMIN_PASS=... sh scripts/acceptance.sh
set -u

# Loads credentials from the controller's .env if it exists and vars aren't set in the environment.
ENV_FILE="$(cd "$(dirname "$0")/.." && pwd)/deploy/control-plane/.env"
if [ -r "$ENV_FILE" ]; then
  # shellcheck disable=SC1090
  . "$ENV_FILE"
elif [ -f "$ENV_FILE" ] && [ -z "${ADMIN_PASS:-}" ]; then
  # The .env holds secrets and is owned by the user who ran the bootstrap (usually root, mode
  # 600). Without this check the `.` above would fail and the script would die without explaining (#26).
  echo "!! cannot read $ENV_FILE (it holds secrets and belongs to the installer)." >&2
  echo "   Run: sudo sh scripts/acceptance.sh   — or pass ADMIN_PASS=<password> in the environment." >&2
  exit 2
fi
API="${API:-http://127.0.0.1:8080}"
PORTAL="${PORTAL:-http://127.0.0.1}"       # the Portal nginx (port 80)
PROM="${PROM:-http://127.0.0.1:9090}"      # Prometheus
ADMIN_USER="${ADMIN_USER:-${SEEDGRID_ADMIN_USERNAME:-admin}}"
ADMIN_PASS="${ADMIN_PASS:-${SEEDGRID_ADMIN_PASSWORD:-}}"

PASS=0
FAIL=0
ok()   { echo "  ✅ $1"; PASS=$((PASS + 1)); }
bad()  { echo "  ❌ $1"; FAIL=$((FAIL + 1)); }
jqpy() { python3 -c "import sys,json;d=json.load(sys.stdin);print($1)" 2>/dev/null; }

echo "== SeedGrid — end-to-end acceptance ($API) =="

# 1) Health
[ "$(curl -s "$API/healthz" | jqpy "d.get('status')")" = "ok" ] \
  && ok "healthz responds ok" || bad "healthz"

# 2) /metrics exposes domain metrics
curl -s "$API/metrics" | grep -q '^seedgrid_nodes' \
  && ok "/metrics exposes seedgrid_*" || bad "/metrics missing seedgrid_*"

# 3) Operator login
LOGIN=$(curl -s -X POST "$API/api/v1/auth/login" -H 'Content-Type: application/json' \
  -d "{\"username\":\"$ADMIN_USER\",\"password\":\"$ADMIN_PASS\"}")
TOKEN=$(echo "$LOGIN" | jqpy "d['token']")
[ -n "$TOKEN" ] && ok "operator login issues a token" || { bad "login failed"; echo "aborting (no token)"; exit 1; }
AUTH="Authorization: Bearer $TOKEN"
# 3b) Default credential flag present (Portal uses it to warn about admin/admin)
echo "$LOGIN" | jqpy "'default_credentials' in d" | grep -qi true \
  && ok "login exposes default_credentials" || bad "login missing default_credentials"

# 4) ONLINE Nodes with calculated health
NODES=$(curl -s "$API/api/v1/nodes" -H "$AUTH")
ONLINE=$(echo "$NODES" | jqpy "sum(1 for n in d['data'] if n['status']=='ONLINE')")
[ "${ONLINE:-0}" -ge 1 ] && ok "$ONLINE Node(s) ONLINE" || bad "no Node ONLINE"
echo "$NODES" | jqpy "all(n.get('health_score') is not None for n in d['data'])" | grep -qi true \
  && ok "health_score present on every Node" || bad "health_score missing"

# 5) Resources (disk/mem) reported
echo "$NODES" | jqpy "any(n.get('disk_total_bytes') for n in d['data'])" | grep -qi true \
  && ok "host resources (disk) reported" || bad "missing host resources"

# 6) Metrics via CP->Prometheus
SERIES=$(curl -s "$API/api/v1/metrics/query?metric=nodes&range=15m" -H "$AUTH" | jqpy "len(d.get('data',[]))")
[ "${SERIES:-0}" -ge 1 ] && ok "Prometheus metrics served by CP" || bad "metrics/query empty"

# 7) Control Plane Health
SYS=$(curl -s "$API/api/v1/system/health" -H "$AUTH")
[ "$(echo "$SYS" | jqpy "d['database']['status']")" = "ok" ] \
  && ok "system/health: API + Database ok" || bad "system/health"

# 8) Events and Alerts respond
[ "$(curl -s -o /dev/null -w '%{http_code}' "$API/api/v1/events" -H "$AUTH")" = "200" ] \
  && ok "GET /events" || bad "/events"
[ "$(curl -s -o /dev/null -w '%{http_code}' "$API/api/v1/alerts" -H "$AUTH")" = "200" ] \
  && ok "GET /alerts" || bad "/alerts"

# 8b) Distinct bt_port per Node (when there are 2+): unique BT endpoints
NNODES=$(echo "$NODES" | jqpy "len(d['data'])")
if [ "${NNODES:-0}" -ge 2 ]; then
  echo "$NODES" | jqpy "len({n.get('bt_port') for n in d['data'] if n.get('bt_port')})==len([n for n in d['data'] if n.get('bt_port')])" | grep -qi true \
    && ok "bt_port unique per Node" || bad "bt_port duplicated across Nodes"
fi

# 9) Scheduler flow: add torrent (magnet) → selected Node
HASH="acce97a9ceb7a9ceb7a9ceb7a9ceb7a9ceb70001"
RESP=$(curl -s -X POST "$API/api/v1/torrents" -H "$AUTH" -H 'Content-Type: application/json' \
  -d "{\"magnet\":\"magnet:?xt=urn:btih:$HASH&dn=acceptance-test\",\"name\":\"acceptance-test\"}")
NODE=$(echo "$RESP" | jqpy "d.get('node_id','')[:8]")
TID=$(echo "$RESP" | jqpy "d.get('id','')")
[ -n "$NODE" ] && ok "Scheduler chose Node ($NODE) for new torrent" || bad "add torrent / scheduler"

# 10) Operation cycle on the test torrent (pause/resume/samples) and cleanup (remove)
if [ -n "$TID" ]; then
  [ "$(curl -s -o /dev/null -w '%{http_code}' -X POST "$API/api/v1/torrents/$TID/pause" -H "$AUTH")" = "200" ] \
    && ok "pause torrent" || bad "pause"
  [ "$(curl -s -o /dev/null -w '%{http_code}' -X POST "$API/api/v1/torrents/$TID/resume" -H "$AUTH")" = "200" ] \
    && ok "resume torrent" || bad "resume"
  [ "$(curl -s -o /dev/null -w '%{http_code}' "$API/api/v1/torrents/$TID/samples?range=1h" -H "$AUTH")" = "200" ] \
    && ok "torrent time series (samples)" || bad "samples"
  [ "$(curl -s -o /dev/null -w '%{http_code}' -X DELETE "$API/api/v1/torrents/$TID" -H "$AUTH")" = "200" ] \
    && ok "remove torrent (cleanup)" || bad "remove"
else
  bad "no torrent id — skipped pause/resume/samples/remove"
fi

# 11) samples of non-existent torrent → 404
[ "$(curl -s -o /dev/null -w '%{http_code}' "$API/api/v1/torrents/00000000-0000-0000-0000-000000000000/samples" -H "$AUTH")" = "404" ] \
  && ok "samples for non-existent torrent => 404" || bad "samples 404"

# 12) Password change validates strength (non-destructive: short password => 400)
[ "$(curl -s -o /dev/null -w '%{http_code}' -X POST "$API/api/v1/auth/password" -H "$AUTH" \
  -H 'Content-Type: application/json' -d '{"current_password":"x","new_password":"123"}')" = "400" ] \
  && ok "password change rejects weak password (400)" || bad "password weak-check"

# 13) Prometheus scraping the targets (controller + node-exporter UP)
UP=$(curl -s "$PROM/api/v1/query?query=up" | jqpy "sum(1 for r in d['data']['result'] if r['value'][1]=='1')")
[ "${UP:-0}" -ge 1 ] && ok "Prometheus with $UP target(s) UP" || bad "Prometheus with no targets UP (or unreachable at $PROM)"

# 14) Portal serving and API proxy
[ "$(curl -s -o /dev/null -w '%{http_code}' "$PORTAL/")" = "200" ] \
  && ok "Portal responds (HTTP 200)" || bad "Portal at $PORTAL"
[ "$(curl -s "$PORTAL/healthz" | jqpy "d.get('status')")" = "ok" ] \
  && ok "Portal proxies /healthz to CP" || bad "proxy /healthz do Portal"

echo "== result: $PASS ok, $FAIL failures =="
[ "$FAIL" -eq 0 ]
