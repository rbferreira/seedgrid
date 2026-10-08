#!/usr/bin/env sh
# Compiles the Agent binary in a Go container.
# The static binary (CGO disabled) is placed in agent/bin/seedgrid-agent.
#
# On normal Docker hosts (RPi/VM) the same command works; for ARM64 use
# GOARCH=arm64 (the binary is static and cross-compiles without C dependencies).
# Extra `docker run` options go in SEEDGRID_DOCKER_RUN_OPTS — e.g.: Docker inside
# a system container with AppArmor requires "--security-opt apparmor=unconfined".
set -eu

AGENT_DIR="$(cd "$(dirname "$0")/../agent" && pwd)"
VERSION="${VERSION:-$(sh "$(dirname "$0")/version.sh")}"
GOARCH="${GOARCH:-}"
# OUT allows generating one binary per architecture without one overwriting the other (ADR-027:
# the Control Plane publishes seedgrid-agent-amd64 and -arm64). Default keeps the historical path.
OUT="${OUT:-bin/seedgrid-agent}"

# shellcheck disable=SC2086 # SEEDGRID_DOCKER_RUN_OPTS are multiple options, intentionally without quotes
docker run --rm ${SEEDGRID_DOCKER_RUN_OPTS:-} \
  -e "GOARCH=${GOARCH}" \
  -v "$AGENT_DIR:/src" -w /src \
  golang:1.23-alpine \
  sh -c "CGO_ENABLED=0 go build -trimpath -ldflags '-s -w -X main.version=${VERSION}' -o ${OUT} ./cmd/seedgrid-agent"

echo "==> done: agent/${OUT}"
