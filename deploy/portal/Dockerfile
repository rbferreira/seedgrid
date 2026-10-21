# SeedGrid Portal — multi-stage build, the image published on every release (ADR-042).
# Build context: repository root.
#
# Builds the SPA and the Agent binaries for both architectures, so the image is also the Nodes'
# download server (/node/, ADR-017 and ADR-027) for the same version. The build stages run on
# the build machine's platform and Go cross-compiles: a multi-arch build needs no emulation.
#
# Where `docker build` with RUN fails (Docker inside LXC), scripts/build-portal.sh builds the
# same content with `docker run` and Dockerfile.runtime.

FROM --platform=$BUILDPLATFORM node:22-alpine AS spa
WORKDIR /src/portal
COPY portal/package.json portal/package-lock.json ./
RUN npm ci
COPY portal/ ./
RUN npm run build

FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS node-artifacts
ARG VERSION=dev
WORKDIR /src
COPY agent/ agent/
COPY scripts/ scripts/
COPY deploy/agent/ deploy/agent/
RUN cd agent && for arch in amd64 arm64; do \
      CGO_ENABLED=0 GOARCH=$arch go build -trimpath \
        -ldflags "-s -w -X main.version=${VERSION}" \
        -o /out/node/seedgrid-agent-$arch ./cmd/seedgrid-agent || exit 1; \
    done
RUN sh scripts/node-artifacts.sh /out/node "${VERSION}"

FROM nginx:1.27-alpine
# The label lets the bootstrap remove the previous build of this image, and only SeedGrid's (#43).
LABEL seedgrid.image=portal
COPY deploy/portal/nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=spa /src/portal/dist /usr/share/nginx/html
COPY --from=node-artifacts /out/node /usr/share/nginx/html/node
EXPOSE 80
