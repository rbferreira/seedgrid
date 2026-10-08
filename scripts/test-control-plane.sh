#!/usr/bin/env sh
# Runs vet + Control Plane tests in an ephemeral Go container.
#
# Extra `docker run` options (e.g.: AppArmor) go in SEEDGRID_DOCKER_RUN_OPTS, as in build-*.sh.
# If a test Postgres is available, export SEEDGRID_TEST_DATABASE_URL to
# include integration tests; otherwise they are skipped.
#
# Usage:
#   scripts/test-control-plane.sh
#   SEEDGRID_TEST_DATABASE_URL=postgres://... scripts/test-control-plane.sh
set -eu

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"

# shellcheck disable=SC2086 # SEEDGRID_DOCKER_RUN_OPTS are multiple options, intentionally unquoted
docker run --rm ${SEEDGRID_DOCKER_RUN_OPTS:-} \
  --network host \
  -e "SEEDGRID_TEST_DATABASE_URL=${SEEDGRID_TEST_DATABASE_URL:-}" \
  -v "$REPO_ROOT/control-plane:/src" \
  -w /src \
  golang:1.23-alpine \
  sh -c "apk add --no-cache -q git && go mod tidy && go vet ./... && go test ./... -count=1 -p 1"
