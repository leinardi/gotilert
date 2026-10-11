# syntax=docker/dockerfile:1.7@sha256:a57df69d0ea827fb7266491f2813635de6f17269be881f696fbfdf2d83dda33e

# MIT License
#
# Copyright (c) 2025 Roberto Leinardi
#
# Permission is hereby granted, free of charge, to any person obtaining a copy
# of this software and associated documentation files (the "Software"), to deal
# in the Software without restriction, including without limitation the rights
# to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
# copies of the Software, and to permit persons to whom the Software is
# furnished to do so, subject to the following conditions:
#
# The above copyright notice and this permission notice shall be included in all
# copies or substantial portions of the Software.
#
# THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
# IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
# FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
# AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
# LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
# OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
# SOFTWARE.

# The syntax line at the top pins the BuildKit frontend by index digest, so a moved
# 1.7 tag cannot change how this file is built. Dependabot's docker ecosystem
# does not update it: bump it by hand (docs/release.md).
#
# Base images are pinned by tag, not by digest: dhi.io republishes its tags
# often to ship security fixes, and a rebuild should pick those up. Every
# release build is scanned, so a newer base never ships unscanned.

########################
# build: compile static binary
########################
FROM --platform=$BUILDPLATFORM dhi.io/golang:1.26.9-alpine3.23-dev AS build
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
FROM dhi.io/static:20250419

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
