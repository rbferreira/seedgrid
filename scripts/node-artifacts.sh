#!/usr/bin/env sh
# Writes the Seed Node artifacts the Portal serves at /node/ — zero-touch onboarding
# (ADR-017, `curl … | sh`) and Agent self-update (ADR-027).
#
# Usage: sh scripts/node-artifacts.sh <dir> <version>
#
# <dir> must already hold seedgrid-agent-amd64 and seedgrid-agent-arm64. The script adds the
# agents.json manifest and the files the bootstrap downloads. It is shared by
# scripts/build-portal.sh (build on the host) and deploy/portal/Dockerfile (release image),
# so both serve the same /node/.
#
# ⚠️ ONE BINARY PER ARCHITECTURE. Until Jul/2026 a single /node/seedgrid-agent was published,
# cross-compiled to ARM64 because of the RPi 3B+ — but amd64 Nodes appeared, and then
# that path served the wrong binary (an amd64 Node onboarding would fetch ARM64, and the
# self-update would break the entire fleet at once). The name now carries the architecture and
# the manifest says which to download; the ambiguous path no longer exists by design.
set -eu

NODE_DIR="${1:?usage: node-artifacts.sh <dir> <version>}"
AGENT_VERSION="${2:?usage: node-artifacts.sh <dir> <version>}"
REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"

manifest_entries=""
for arch in amd64 arm64; do
  bin="$NODE_DIR/seedgrid-agent-$arch"
  [ -f "$bin" ] || { echo "missing $bin" >&2; exit 1; }
  sum="$(sha256sum "$bin" | cut -d' ' -f1)"
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

cp "$REPO_ROOT/scripts/bootstrap-node.sh"                      "$NODE_DIR/bootstrap-node.sh"
cp "$REPO_ROOT/deploy/agent/docker-compose.yml"                "$NODE_DIR/docker-compose.yml"
cp "$REPO_ROOT/deploy/agent/seedgrid-agent.service"            "$NODE_DIR/seedgrid-agent.service"
cp "$REPO_ROOT/deploy/agent/seedgrid-storage-reattach.sh"      "$NODE_DIR/seedgrid-storage-reattach.sh"
cp "$REPO_ROOT/deploy/agent/seedgrid-storage-reattach.service" "$NODE_DIR/seedgrid-storage-reattach.service"
cp "$REPO_ROOT/deploy/agent/99-seedgrid-storage.rules"         "$NODE_DIR/99-seedgrid-storage.rules"
