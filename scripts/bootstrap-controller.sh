#!/usr/bin/env sh
# Installs and updates the Control Plane on the host (Phase 7): Postgres + Controller + Portal +
# Observability (Prometheus/Alertmanager/node-exporter). Idempotent.
#
# Installation:
#   sh scripts/bootstrap-controller.sh
#
# Update:
#   git pull && git fetch --tags && sh scripts/bootstrap-controller.sh
#   (the fetch --tags is required: without it, a new tag pointing to an already
#   downloaded commit won't arrive, and the version comes out as the previous + counter (#181))
#
# There is nothing to edit beforehand: on a new install the script creates the
# deploy/control-plane/.env and generates the secrets (DB password, registration token for the
# Agents, admin password, and internal webhook tokens), and displays the login at the end.
# Those who prefer to choose the values create the .env from the .env.example before running:
# whatever is already filled in is respected.
set -eu

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
CP_DIR="$REPO_ROOT/deploy/control-plane"
ENV_FILE="$CP_DIR/.env"

# Reads the value of a key from the .env (the last one, if repeated).
env_get() { sed -n "s/^$1=//p" "$ENV_FILE" | tail -n 1; }

# Writes key=value to the .env, IN PLACE of the existing line (the comment that explains
# it stays above it) or at the end, if the key doesn't exist. Rewrites with cat >, not mv,
# to preserve file ownership and permissions.
env_set() {
  K="$1" V="$2" awk '
    BEGIN { k = ENVIRON["K"]; v = ENVIRON["V"]; done = 0 }
    index($0, k "=") == 1 { if (!done) print k "=" v; done = 1; next }
    { print }
    END { if (!done) print k "=" v }' "$ENV_FILE" > "$ENV_FILE.tmp"
  cat "$ENV_FILE.tmp" > "$ENV_FILE"
  rm -f "$ENV_FILE.tmp"
}

rand_hex() { head -c "$1" /dev/urandom | od -An -tx1 | tr -d ' \n'; }

# Empty or the text from .env.example ("change-...") = not yet configured.
is_placeholder() {
  case "$1" in '' | change-*) return 0 ;; *) return 1 ;; esac
}

if [ ! -f "$ENV_FILE" ]; then
  cp "$CP_DIR/.env.example" "$ENV_FILE"
  chmod 600 "$ENV_FILE"
  echo "==> $ENV_FILE created from example"
fi

# Secrets. Only generated on a NEW INSTALL (without the Postgres volume): with the DB already
# initialized, its password and the admin operator are already written there, and changing the .env
# would leave the Control Plane without access to its own DB.
FRESH=1
docker volume inspect seedgrid-postgres-data >/dev/null 2>&1 && FRESH=0
ADMIN_PASSWORD_GENERATED=""
for spec in POSTGRES_PASSWORD:24 SEEDGRID_AGENT_REGISTRATION_TOKEN:32 SEEDGRID_ADMIN_PASSWORD:12; do
  key="${spec%%:*}"
  bytes="${spec##*:}"
  is_placeholder "$(env_get "$key")" || continue
  if [ "$FRESH" -eq 1 ]; then
    value="$(rand_hex "$bytes")"
    env_set "$key" "$value"
    echo "==> $key generated"
    [ "$key" = SEEDGRID_ADMIN_PASSWORD ] && ADMIN_PASSWORD_GENERATED="$value"
  else
    echo "WARNING: $key is empty or set to the example value, and the database already exists —" >&2
    echo "       kept as is. Set a strong value in $ENV_FILE." >&2
  fi
done

# Internal webhook token (#116): the same value in the CP (SEEDGRID_INTEGRATION_TOKEN
# in the .env) and in Alertmanager (mounted credentials_file). The .env value prevails if the operator
# defined it; otherwise the file's; otherwise a new one. In the end both sides match —
# the .env.example has an empty line, so "the line exists" doesn't mean "configured".
SECRETS_DIR="$REPO_ROOT/deploy/observability/secrets"
TOK_FILE="$SECRETS_DIR/integration-token"
mkdir -p "$SECRETS_DIR"
if [ ! -w "$SECRETS_DIR" ]; then
  echo "ERROR: $SECRETS_DIR is not writable by $(id -un). Run as the user who owns" >&2
  echo "the clone, or fix it once: chown -R \"$(id -un)\" $SECRETS_DIR" >&2
  exit 1
fi
ENV_TOK="$(env_get SEEDGRID_INTEGRATION_TOKEN)"
if [ -n "$ENV_TOK" ]; then
  TOK="$ENV_TOK"
elif [ -s "$TOK_FILE" ]; then
  TOK="$(cat "$TOK_FILE")"
else
  TOK="$(rand_hex 32)"
fi
printf '%s' "$TOK" > "$TOK_FILE"
# 644, not 600: Alertmanager reads the file as `nobody` (container uid,
# unrelated to the host owner) — 600 blocks reading and every webhook
# (firing and resolved) fails silently with "permission denied".
chmod 644 "$TOK_FILE"
[ "$ENV_TOK" = "$TOK" ] || env_set SEEDGRID_INTEGRATION_TOKEN "$TOK"

# Binary version (ADR-024). Compose does NOT run git: without this export, the
# `args: VERSION: ${VERSION:-dev}` in compose falls back to default and the controller reports
# "dev" on /healthz (#175).
VERSION="$(sh "$REPO_ROOT/scripts/version.sh")"
export VERSION
echo "==> build version: $VERSION"

echo "==> build da imagem do Controller"
sh "$REPO_ROOT/scripts/build-controller.sh"

# Shared volume CP <-> Prometheus (file_sd targets, ADR-017). Idempotent.
# The controller runs as UID 65532 (distroless nonroot image) — the volume needs
# to be writable by it (otherwise the targets writer fails with permission denied).
docker volume create seedgrid-prom-targets >/dev/null
# shellcheck disable=SC2086 # SEEDGRID_DOCKER_RUN_OPTS (see build-controller.sh) are multiple options
docker run --rm ${SEEDGRID_DOCKER_RUN_OPTS:-} -v seedgrid-prom-targets:/t busybox chown -R 65532:65532 /t >/dev/null

echo "==> Control Plane (Postgres + Controller)"
( cd "$CP_DIR" && docker compose up -d )

echo "==> Observability (Prometheus + Alertmanager + node-exporter)"
( cd "$REPO_ROOT/deploy/observability" && docker compose up -d )

echo "==> build + deploy do Portal (nginx)"
sh "$REPO_ROOT/scripts/build-portal.sh"
( cd "$REPO_ROOT/deploy/portal" && docker compose up -d )

# Each update rebuilds the images and leaves the previous ones untagged (#43). Only SeedGrid's go:
# a plain `docker image prune` would also take untagged images from whatever else runs on this
# host. The build cache isn't touched for the same reason (Deployment §43 says how to clear it).
# A failed cleanup doesn't fail the update.
echo "==> removing SeedGrid's previous images"
docker image prune -f --filter label=seedgrid.image >/dev/null || true

IP="$(hostname -I 2>/dev/null | awk '{print $1}')"
ADMIN_USER="$(env_get SEEDGRID_ADMIN_USERNAME)"
echo "==> done."
echo "    Portal:  http://${IP:-<this-host-ip>}"
if [ -n "$ADMIN_PASSWORD_GENERATED" ]; then
  echo "    Login:   ${ADMIN_USER:-admin} / $ADMIN_PASSWORD_GENERATED"
  echo "             (password generated now; stored in $ENV_FILE — change it in the Portal, under Profile)"
else
  echo "    Login:   ${ADMIN_USER:-admin} — the password is the SEEDGRID_ADMIN_PASSWORD from $ENV_FILE"
  echo "             (or the one you set in the Portal, if already changed)"
fi
echo "    Nodes:   in the Portal, Health & Metrics → Add Node shows the onboarding command."
echo "    The API is served by the Portal (/api). API port 8080 and Prometheus port 9090"
echo "    only listen on 127.0.0.1 on this host."
# Those who ran with sudo have a .env that only root can read: the validation command needs the same.
if [ -n "${SUDO_USER:-}" ]; then
  echo "    Validate: sudo sh scripts/acceptance.sh"
else
  echo "    Validate: sh scripts/acceptance.sh"
fi
