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

# Seed Node artifacts served at /node/* (ADR-017, ADR-027): the binaries are built here,
# and the manifest and the bootstrap files come from node-artifacts.sh, the same script the
# release image uses.
echo "==> Seed Node artifacts (binaries per architecture + manifest + bootstrap)"
NODE_DIR="$PORTAL_DIR/dist/node"
mkdir -p "$NODE_DIR"
AGENT_VERSION="${VERSION:-$(sh "$REPO_ROOT/scripts/version.sh")}"

for arch in amd64 arm64; do
  echo "  -> seedgrid-agent-$arch"
  GOARCH="$arch" OUT="bin/seedgrid-agent-$arch" VERSION="$AGENT_VERSION" \
    sh "$REPO_ROOT/scripts/build-agent.sh" >/dev/null
  cp "$REPO_ROOT/agent/bin/seedgrid-agent-$arch" "$NODE_DIR/seedgrid-agent-$arch"
done
sh "$REPO_ROOT/scripts/node-artifacts.sh" "$NODE_DIR" "$AGENT_VERSION"

echo "==> building seedgrid/portal:dev image (nginx, Dockerfile without RUN)"
docker build -t seedgrid/portal:dev -f "$REPO_ROOT/deploy/portal/Dockerfile.runtime" "$REPO_ROOT"

echo "==> done: seedgrid/portal:dev"
