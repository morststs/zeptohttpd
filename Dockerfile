# syntax=docker/dockerfile:1

# The build stage always runs on the native platform of the builder and
# cross-compiles, which is much faster than emulating the target platform.
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS build

ARG TARGETOS
ARG TARGETARCH
ARG TARGETVARIANT
ARG VERSION=dev

WORKDIR /build
COPY ./src /build

# Vet and test once on the build platform; a failure stops the image.
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    go vet ./... && go test ./...

# TARGETVARIANT is "v6"/"v7" for 32-bit arm and empty elsewhere; GOARM wants
# the bare number and is ignored for every other architecture.
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    CGO_ENABLED=0 \
    GOOS="${TARGETOS}" \
    GOARCH="${TARGETARCH}" \
    GOARM="${TARGETVARIANT#v}" \
    go build -ldflags="-s -w -X main.version=${VERSION}" -trimpath -o /out/zeptohttpd .

FROM scratch

ARG VERSION=dev
LABEL org.opencontainers.image.title="zeptohttpd" \
      org.opencontainers.image.description="Tiny static HTTP server for development and operation checks." \
      org.opencontainers.image.source="https://github.com/morststs/zeptohttpd" \
      org.opencontainers.image.licenses="Apache-2.0" \
      org.opencontainers.image.version="${VERSION}"

COPY --from=build /out/zeptohttpd /zeptohttpd
COPY ./public /public
COPY ./config/config.json /config.json

WORKDIR /
# Unprivileged. Docker allows binding port 80 from a non-root user by default
# (net.ipv4.ip_unprivileged_port_start=0); on platforms that do not, run with
# -e ZEPTO_ADDR=:8080.
USER 65532:65532
EXPOSE 80
ENTRYPOINT ["/zeptohttpd"]
