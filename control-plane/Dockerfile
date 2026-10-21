# SeedGrid Control Plane — multi-stage build, the image published on every release (ADR-042).
# The build stage runs on the build machine's platform and Go cross-compiles to the target
# one, so a multi-arch build needs no emulation.

FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS build
WORKDIR /src

# Dependencies first, so a source change does not download them again. go.sum is committed:
# the build uses exactly the versions in the repository.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION=dev
ARG TARGETOS TARGETARCH
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath \
    -ldflags "-s -w -X main.version=${VERSION}" \
    -o /out/seedgrid-controller ./cmd/seedgrid-controller

# Same base as Dockerfile.runtime, so both paths run as the same user (UID 65532, the one
# bootstrap-controller.sh gives the targets volume to). distroless/static carries the CA
# certificates and the time zones the controller needs.
FROM gcr.io/distroless/static:nonroot
# The label lets the bootstrap remove the previous build of this image, and only SeedGrid's (#43).
LABEL seedgrid.image=controller
COPY --from=build /out/seedgrid-controller /usr/local/bin/seedgrid-controller
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/seedgrid-controller"]
