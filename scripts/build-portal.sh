#!/usr/bin/env sh
# Builds the seedgrid/portal:dev image: compiles the SPA in a Node container via `docker run`
# and builds the final nginx image with a Dockerfile without RUN steps (see build-controller.sh).
#
# Extra `docker run` options go in SEEDGRID_DOCKER_RUN_OPTS — e.g.: Docker inside
# a system container with AppArmor requires "--security-opt apparmor=unconfined".
set -eu

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
PORTAL_DIR="$REPO_ROOT/portal"

# ⚠️ DO NOT clean dist/ from here. This script runs as `deploy`, but `dist/assets` is
# written by the build container, which runs as ROOT — an `rm -rf "$PORTAL_DIR/dist"`
# fails with "Permission denied", crashes the job via `set -e` and even deletes it HALF WAY
# (takes index.html and dist/node, leaves the root-owned ones). This is what broke the #135 deploy.
#
# And it is also unnecessary: vite's `emptyOutDir` already cleans dist on every build, from
# inside the container, where there is permission. Against Tailwind scanning its own output, what
# fixes it is the `@source not "../dist"` in index.css (#134) — not the cleanup here.

echo "==> build do SPA (docker run node)"
# shellcheck disable=SC2086 # SEEDGRID_DOCKER_RUN_OPTS are multiple options, purposely unquoted
docker run --rm ${SEEDGRID_DOCKER_RUN_OPTS:-} \
  -v "$PORTAL_DIR:/app" -w /app \
  node:22-alpine \
  sh -c "npm ci && npm run build"

# Seed Node artifacts served at /node/* — zero-touch onboarding (ADR-017, `curl … | sh`)
# and Agent self-update (ADR-027).
#
# ⚠️ ONE BINARY PER ARCHITECTURE. Until Jul/2026 a single /node/seedgrid-agent was published,
# cross-compiled to ARM64 because of the RPi 3B+ — but amd64 Nodes appeared, and then
# that path served the wrong binary (an amd64 Node onboarding would fetch ARM64, and the
# self-update would break the entire fleet at once). The name now carries the architecture and
# the manifest says which to download; the ambiguous path no longer exists by design.
echo "==> Seed Node artifacts (binaries per architecture + manifest + bootstrap)"
NODE_DIR="$PORTAL_DIR/dist/node"
mkdir -p "$NODE_DIR"
AGENT_VERSION="${VERSION:-$(sh "$REPO_ROOT/scripts/version.sh")}"

manifest_entries=""
for arch in amd64 arm64; do
  echo "  -> seedgrid-agent-$arch"
  GOARCH="$arch" OUT="bin/seedgrid-agent-$arch" VERSION="$AGENT_VERSION" \
    sh "$REPO_ROOT/scripts/build-agent.sh" >/dev/null
  cp "$REPO_ROOT/agent/bin/seedgrid-agent-$arch" "$NODE_DIR/seedgrid-agent-$arch"
  sum="$(sha256sum "$NODE_DIR/seedgrid-agent-$arch" | cut -d' ' -f1)"
  [ -n "$manifest_entries" ] && manifest_entries="$manifest_entries,"
  manifest_entries="$manifest_entries
    \"$arch\": { \"file\": \"seedgrid-agent-$arch\", \"sha256\": \"$sum\" }"
done

# Manifest read by the CP (ADR-027) to know the available version and the checksum per
# architecture — this is what allows comparing with each Node's agent_version and building the
# update_agent command without anyone manually typing the checksum.
cat > "$NODE_DIR/agents.json" <<JSON
{
  "version": "$AGENT_VERSION",
  "binaries": {$manifest_entries
  }
}
JSON
echo "  -> agents.json (version $AGENT_VERSION)"

cp "$REPO_ROOT/scripts/bootstrap-node.sh"                    "$NODE_DIR/bootstrap-node.sh"
cp "$REPO_ROOT/deploy/agent/docker-compose.yml"              "$NODE_DIR/docker-compose.yml"
cp "$REPO_ROOT/deploy/agent/seedgrid-agent.service"          "$NODE_DIR/seedgrid-agent.service"
cp "$REPO_ROOT/deploy/agent/seedgrid-storage-reattach.sh"      "$NODE_DIR/seedgrid-storage-reattach.sh"
cp "$REPO_ROOT/deploy/agent/seedgrid-storage-reattach.service" "$NODE_DIR/seedgrid-storage-reattach.service"
cp "$REPO_ROOT/deploy/agent/99-seedgrid-storage.rules"        "$NODE_DIR/99-seedgrid-storage.rules"

echo "==> building seedgrid/portal:dev image (nginx, Dockerfile without RUN)"
docker build -t seedgrid/portal:dev -f "$REPO_ROOT/deploy/portal/Dockerfile" "$REPO_ROOT"

echo "==> done: seedgrid/portal:dev"
