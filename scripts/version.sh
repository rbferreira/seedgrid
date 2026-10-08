#!/usr/bin/env sh
# Derives the build version from git tags (ADR-024, "versioning the builds
# for real" instead of the default "dev").
#
# Format (merge chosen by the operator — without the "ugly" commit hash):
#   - on exact tag:            v1.0.0
#   - N commits after the tag: v1.0.0-N
#   - no tag/git available:    dev  (fallback)
#
# Used by build-*.sh as the VERSION default; injected into the binary via
# -ldflags "-X main.version=...". Can be overridden by exporting VERSION.
set -eu

# git describe needs to run inside the repo tree; anchor to the script's dir.
cd "$(dirname "$0")"

# `git describe --tags` → "v1.0.0" (exact tag) or "v1.0.0-3-gabc123" (3 commits
# later). The sed removes the "-g<hash>" suffix to get "v1.0.0-3".
v="$(git describe --tags 2>/dev/null | sed 's/-g[0-9a-f]*$//' || true)"
[ -n "${v:-}" ] || v="dev"

printf '%s' "$v"
