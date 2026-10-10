#!/usr/bin/env sh
# Builds the seedgrid/controller:dev image: compiles the binary via `docker run` and builds the
# final image with a Dockerfile without RUN steps (Dockerfile.runtime). It also works
# where `docker build` with RUN fails — e.g., Docker inside a system container, where
# the AppArmor profile doesn't apply to the build. On a regular Docker host the standard
# multi-stage Dockerfile (`docker compose build`) yields the same result.
#
# Extra `docker run` options go in SEEDGRID_DOCKER_RUN_OPTS — e.g., Docker inside a
# system container with AppArmor requires "--security-opt apparmor=unconfined".
set -eu

CP_DIR="$(cd "$(dirname "$0")/../control-plane" && pwd)"
VERSION="${VERSION:-$(sh "$(dirname "$0")/version.sh")}"

echo "==> compiling binary (docker run)"
# shellcheck disable=SC2086 # SEEDGRID_DOCKER_RUN_OPTS are multiple options, intentionally unquoted
docker run --rm ${SEEDGRID_DOCKER_RUN_OPTS:-} \
  -v "$CP_DIR:/src" -w /src \
  golang:1.26-alpine \
  sh -c "apk add --no-cache -q git && go mod download && CGO_ENABLED=0 go build -trimpath -ldflags '-s -w -X main.version=${VERSION}' -o bin/seedgrid-controller ./cmd/seedgrid-controller"

echo "==> building seedgrid/controller:dev image (Dockerfile.runtime, no RUN)"
docker build -t seedgrid/controller:dev -f "$CP_DIR/Dockerfile.runtime" "$CP_DIR"

echo "==> done: seedgrid/controller:dev"
