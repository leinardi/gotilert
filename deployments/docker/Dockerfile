# syntax=docker/dockerfile:1.7@sha256:a57df69d0ea827fb7266491f2813635de6f17269be881f696fbfdf2d83dda33e

# The syntax line above pins the BuildKit frontend by index digest, so a moved
# 1.7 tag cannot change how this file is built. Dependabot's docker ecosystem
# does not update it: bump it by hand (docs/release.md).
#
# Every base image is pinned by index digest: the release scans the image it
# builds and a recovery re-run rebuilds it, so a tag that moved in between would
# ship something nobody scanned. Dependabot bumps the tag and the digest together.

########################
# build: compile static binary
########################
FROM --platform=$BUILDPLATFORM dhi.io/golang:1.26.8-alpine3.23-dev@sha256:16da8d3c0c7a8276fee2196e3ba764d73c3db22f13cc2dc3a6799e8c0dcf9fe6 AS build
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

# Bring in source
COPY . .

# Module and build caches (BuildKit)
RUN --mount=type=cache,target=/go/pkg/mod,id=gomodcache \
	--mount=type=cache,target=/root/.cache/go-build \
	go build -trimpath -buildvcs=false \
		-ldflags="-s -w -X 'main.version=${VERSION}' -X 'main.commit=${COMMIT}' -X 'main.date=${DATE}'" \
		-o /out/gotilert ./cmd/gotilert

########################
# runtime: dhi.io hardened static base (distroless-style, no shell).
# The binary is CGO_ENABLED=0 so no libc is required. The base ships the CA
# bundle at /etc/ssl/certs/ca-certificates.crt, which HTTPS to Alertmanager needs.
########################
FROM dhi.io/static:20250419@sha256:98ef7a853608577e8d66dad1d25ada75d745d782f28d84e9ecfb85dfeb1f9c98

# hadolint ignore=DL3050
LABEL org.opencontainers.image.title="gotilert" \
	org.opencontainers.image.description="Gotify-compatible gateway that forwards notifications to Alertmanager" \
	org.opencontainers.image.source="https://github.com/leinardi/gotilert" \
	org.opencontainers.image.licenses="MIT"

# Copy the binary
COPY --from=build /out/gotilert /usr/local/bin/gotilert

# Run as a fixed non-root user; the gateway does not need root. The uid is pinned here rather
# than inherited from the base image's default (65532 today), so a base update cannot change it.
USER 65532:65532

# Document the listen port
EXPOSE 8008

# The base has no shell; use absolute path
ENTRYPOINT ["/usr/local/bin/gotilert"]
