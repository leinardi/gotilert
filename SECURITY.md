# Security policy

## Supported versions

Only the latest release gets security fixes. A fix ships as a new version: released tags and image tags are immutable, so an
existing version is never rebuilt or replaced. Upgrade to the latest release, or follow `ghcr.io/leinardi/gotilert:latest` or the
`:<major>` tag.

## Reporting a vulnerability

Report it privately through GitHub's
[private vulnerability reporting](https://github.com/leinardi/gotilert/security/advisories/new), not in a public issue, a pull
request or a discussion. Include the version or image digest, how gotilert is exposed (internal network, reverse proxy), and the steps or input that trigger the problem.

This is a project maintained in spare time, so reports are handled on a best-effort basis. You will get an answer in the advisory,
and the fix, once released, is credited there unless you prefer otherwise.

## Scope

In scope: the gotilert binary, the container image published to GHCR, the release artifacts, and the workflows that build and
publish them.

Out of scope: vulnerabilities in Gotify clients and Alertmanager themselves and in the base images, which are reported upstream. A base image or dependency
vulnerability that affects a published release is still worth reporting here if the weekly scheduled scan has not caught it.

## Security model

- **App tokens are the only authentication.** Every `POST /message` must carry a configured app token (`X-Gotify-Key`, the `token`
  query parameter or `Authorization: Bearer`); a missing or unknown token is rejected before the body is parsed. Tokens are
  secrets: they are never logged. A token in the query string can still end up in a reverse proxy's access log, so prefer the
  header.
- **No TLS of its own.** The HTTP server speaks plain HTTP and is meant for an internal network. Expose it only behind a reverse
  proxy that terminates TLS and adds rate limiting and access control.
- **Unauthenticated endpoints.** `/healthz`, `/readyz` and `/metrics` have no authentication and are meant for an internal scrape
  network. Request metrics are labelled by route, never by the raw path, so a client cannot grow them without bound.
- **Upstream TLS.** `tlsConfig.insecureSkipVerify` disables certificate verification towards Alertmanager. It exists for
  self-signed homelab setups; with it on, anyone on the path to Alertmanager can read and alter the forwarded alerts and the
  credentials sent with them.
- **Supply chain.** Every GitHub Action is pinned to a commit SHA and every image the workflows and the Dockerfile use to an index
  digest. Each release image is scanned for `HIGH` and `CRITICAL` vulnerabilities before its version is tagged, and the latest
  release is scanned again every week, together with the Go dependencies. The release process is described in
  [docs/release.md](docs/release.md).

## Verifying a release

Release images carry a build provenance attestation and a keyless cosign signature from the release workflow. Verify an image by
digest before you trust it:

```bash
IMAGE=ghcr.io/leinardi/gotilert
DIGEST=sha256:...   # from `docker buildx imagetools inspect $IMAGE:<version>`

gh attestation verify "oci://$IMAGE@$DIGEST" --repo leinardi/gotilert \
  --signer-workflow leinardi/gotilert/.github/workflows/release.yaml --source-ref refs/heads/main

cosign verify "$IMAGE@$DIGEST" \
  --certificate-identity https://github.com/leinardi/gotilert/.github/workflows/release.yaml@refs/heads/main \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

The signature is stored as a Sigstore bundle in an OCI 1.1 referring artifact, so `cosign verify` needs cosign v3, or v2.6 or later
with `--new-bundle-format`.

The release binaries carry a build provenance attestation from the same workflow. Verify a downloaded binary before you run it:

```bash
gh attestation verify gotilert-linux-amd64 --repo leinardi/gotilert \
  --signer-workflow leinardi/gotilert/.github/workflows/release.yaml --source-ref refs/heads/main
```

Releases up to and including v1.0.3 predate this release workflow and carry neither.
