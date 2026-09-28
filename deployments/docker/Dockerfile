# syntax=docker/dockerfile:1.7

# Every base image is pinned by index digest: the release scans the image it
# builds and a recovery re-run rebuilds it, so a tag that moved in between would
# ship something nobody scanned. Dependabot bumps the tag and the digest together.

########################
# deps: cache modules
########################
FROM --platform=$BUILDPLATFORM golang:1.27.1-alpine@sha256:8a5910f31396cd4d89662f56c68b3ae31d374308270a1c3bd96672ee5ed43414 AS deps
WORKDIR /src
RUN --mount=type=cache,target=/var/cache/apk \
	apk add --no-cache ca-certificates git
COPY go.mod go.sum ./
# Use a named cache so we can reuse it in the build stage (no COPY needed)
RUN --mount=type=cache,target=/go/pkg/mod,id=gomodcache \
	go mod download

########################
# build: compile static binary
########################
FROM --platform=$BUILDPLATFORM golang:1.27.1-alpine@sha256:8a5910f31396cd4d89662f56c68b3ae31d374308270a1c3bd96672ee5ed43414 AS build
ARG VERSION="dev"
ARG COMMIT="none"
ARG DATE="unknown"

# Buildx provides these automatically for multi-arch builds
ARG TARGETOS
ARG TARGETARCH

ENV CGO_ENABLED=0 \
    GOOS=${TARGETOS} \
    GOARCH=${TARGETARCH}

WORKDIR /src
RUN --mount=type=cache,target=/var/cache/apk \
	apk add --no-cache ca-certificates

# Bring in source
COPY . .

# Reuse the same module & build caches (BuildKit)
RUN --mount=type=cache,target=/go/pkg/mod,id=gomodcache \
	--mount=type=cache,target=/root/.cache/go-build \
	go build -trimpath -buildvcs=false \
		-ldflags="-s -w -X 'main.version=${VERSION}' -X 'main.commit=${COMMIT}' -X 'main.date=${DATE}'" \
		-o /out/gotilert ./cmd/gotilert

########################
# certs: minimal CA bundle
########################
FROM --platform=$BUILDPLATFORM alpine:3.24.1@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b AS certs
RUN --mount=type=cache,target=/var/cache/apk \
	apk add --no-cache ca-certificates
# leave at /etc/ssl/certs/ca-certificates.crt

########################
# runtime: distroless static nonroot
########################
FROM gcr.io/distroless/static:nonroot@sha256:e2e927ec666bae08560abb3c55d0659eceabb657f56b6782ab500a9fc7f555e3

# hadolint ignore=DL3050
LABEL org.opencontainers.image.title="gotilert" \
	org.opencontainers.image.description="Gotify-compatible gateway that forwards notifications to Alertmanager" \
	org.opencontainers.image.source="https://github.com/leinardi/gotilert" \
	org.opencontainers.image.licenses="MIT"

# Copy the binary and CA bundle
COPY --from=build /out/gotilert /usr/local/bin/gotilert
COPY --from=certs /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# Run as non-root (UID/GID 65532 in distroless)
USER nonroot:nonroot

# Document the metrics port (no EXPOSE required for security, but okay to include)
EXPOSE 8008

# Distroless has no shell; use absolute path
ENTRYPOINT ["/usr/local/bin/gotilert"]
