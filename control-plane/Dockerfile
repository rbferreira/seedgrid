# SeedGrid Control Plane — build multi-stage
# Builds a static binary and runs it in a minimal image.

FROM golang:1.26-alpine AS build
WORKDIR /src
RUN apk add --no-cache git

# Source code
COPY . .

# Resolves dependencies and generates go.sum (network available during build).
RUN go mod tidy

ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath \
    -ldflags "-s -w -X main.version=${VERSION}" \
    -o /out/seedgrid-controller ./cmd/seedgrid-controller

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata && adduser -D -u 10001 seedgrid
USER seedgrid
COPY --from=build /out/seedgrid-controller /usr/local/bin/seedgrid-controller
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/seedgrid-controller"]
