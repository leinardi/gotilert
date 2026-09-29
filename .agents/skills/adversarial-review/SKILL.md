---
name: adversarial-review
description: >
  Adversarial code review of a set of changes to this repo — working tree, staged
  diff, a branch vs main, a commit range, or a PR. Language-agnostic (Go, bash,
  Dockerfile, Makefile, YAML, docs). Loads go-style-guide for Go paths, hunts for real
  defects and violations of this gateway's invariants (every message authenticated,
  tokens never leaked, bounded forwarding to Alertmanager, config fails closed, metrics
  and the Gotify API as public contracts), then reports ranked findings. Use whenever
  the user asks to review changes/a diff/a PR/a branch, "check my work before
  committing", "is this ready to merge", or "poke holes in this" — even if they don't
  name a language or say the word "review".
---

# Adversarial Review — gotilert

You are a hostile reviewer. Assume the change is **wrong until proven right**: it hides a
bug, breaks an invariant, or drifts from a contract. Your job is to find the specific
input, state, or path where it fails — not to praise it, not to restyle it. A review that
finds nothing is only credible after you have actively tried to break the code and failed.

This skill is the **entry point for reviewing any change in this repo, in any language**.
It does not replace the domain skills — it routes to them. The domain skills own the rules;
this skill owns the mindset, the routing, and the report.

---

## 1. Establish the diff (what am I reviewing?)

Never review from memory or from the user's description of the change — read the actual
diff. Pick the scope from what the user said, defaulting to the most useful:

| User intent | Command |
| --- | --- |
| "my work" / "before I commit" / uncommitted | `git status --short`, then `git diff HEAD`; read untracked files too, which no diff shows |
| staged changes only | `git diff --staged` |
| a branch / "this PR" / "ready to merge" | `git diff main...HEAD` (merge-base diff; `main` is this repo's default branch) |
| a specific commit range | `git diff <base>..<head>` |
| a GitHub PR number | `gh pr view <n>` for intent, then `gh pr diff <n>` |

Also read `git log --oneline` for the range and any linked issue/PR body — the stated
**intent** is what you check the code against. A change that works but does something other
than what it claims is a finding.

Read every changed file in full, not just the hunks. For non-trivial changes, also read the
callers, implementations, tests and docs of what changed — found with a reference search, not
assumed from the diff: a signature or behavior change is only safe if every call site agrees.

## 2. Route to the domain skills (path → authority)

For each changed path, load the matching skill **before** judging that file — violations
there are findings even when lint is green. Load only what the diff touches.

| Changed path | Load skill | It owns |
| --- | --- | --- |
| any `**/*.go` | `go-style-guide` | style/lint rules golangci-lint enforces, the helpers to reuse, logging, timeouts, handlers and metrics |

No skill matches (Dockerfile, compose, `Makefile`, `.mk/*.mk`, workflows, other YAML, bash,
Markdown)? Fall back to §4 plus the invariants in §3. **Same rigor** — an unmatched language
is not a lighter review.

## 3. Repo invariants — check these on every review, whatever changed

These are the ways this gateway breaks that generic reviewers miss.

### Every `/message` is authenticated, and tokens never leak

- `extractToken` (`internal/server/message.go`) reads the token from `X-Gotify-Key`, then the
  `token` query parameter, then `Authorization: Bearer` (scheme case-insensitive), in that
  order; `TestAuthTokenPrecedence*` pins it. The app is resolved by exact lookup in the
  config's token map. A missing or unknown token is `403` **before** the body is capped,
  parsed or forwarded. A default app, a fallback token, prefix or case-insensitive matching,
  or any path that parses or forwards before `authenticate` is **critical**.
- Tokens are secrets. The request log records `request.URL.Path`, never the query string;
  config errors name a token only as `tokenKeyForError` (`token(len=N)`). Logging
  `request.URL`, `RawQuery`, headers, or a token in an error or response is critical.
- Only `POST` forwards (`405` otherwise); the body is capped at 1 MiB (`http.MaxBytesReader`,
  `MaxBodyBytes`), and `message` is required and `priority` must be ≥ 0.

### The Gotify API is a public contract

Clients built for Gotify talk to this server. The response shape (`gotify.MessageResponse`:
`id`, `appid`, `message`, `title`, `priority`, `date`, `extras`), the accepted content types
(JSON, form, or none, which parses as a form), unknown JSON fields being accepted, the
default priority `5`, and the status codes (`200`, `400`, `403`, `405`, `502`, `500` when
misconfigured) change only deliberately, with the README.

### One message, one alert

- The forwarder (`newForwarder`, `cmd/gotilert/main.go`) posts one alert per message:
  labels are `defaults.labels`, then the app's labels, then the computed `alertname`, `app`,
  `severity`, `priority`, `gotilert_id` — computed wins. `startsAt` is now, `endsAt` is now +
  `defaults.ttl`. The README documents this order; changing it is a contract change.
- `gotilert_id` is what keeps two messages from merging: Alertmanager deduplicates by label
  set. Removing it, or feeding it a value that repeats within one process, merges
  notifications. It comes from an in-memory counter (`messageID`) and starts again at 1 on
  restart; do not rely on it being unique across restarts.
- Annotations are `summary`, `description`, and the four `gotify_*` extras from
  `ExtrasAnnotations`; the extras-to-annotation names are in the README.

### Forwarding is bounded

- `PostAlerts` makes at most `defaultRetryMaxAttempts` (3) attempts with `computeBackoff`
  (200 ms doubling, capped at 1 s) and waits with `sleepWithContext`. `shouldRetry` retries
  only `429`, `5xx`, and `ErrDoRequest`-wrapped network timeouts and `*net.OpError`; never a
  context error, an x509 or TLS record error, or another `4xx`. An unbounded loop, a retry on
  `4xx`, or a wait that ignores `ctx` is a finding.
- Every attempt is bounded by `http.Client.Timeout`, the whole forward by
  `withBoundedTimeout(ctx, alertmanager.timeout)`, the readiness probe by
  `defaultReadyTimeout` (2 s). An upstream error body is read through
  `io.LimitReader(…, maxErrorBodyBytes)` (64 KiB).
- Known gap: an `http.Client.Timeout` error also matches `context.DeadlineExceeded`, so
  `shouldRetry` treats a per-attempt timeout as permanent, although the README promises
  retries on timeouts; `TestShouldRetryTimeoutTrue` uses a synthetic `net.Error` and does not
  catch it. A change to retry or timeouts must be tested against a slow `httptest` server.
- A failed forward is logged with the upstream status and body and counted in
  `gotilert_upstream_failures_total`; the client gets `502` with the generic
  `ErrUpstreamFailed`, never the upstream response.

### Upstream TLS and credentials

- The transport is `http.DefaultTransport.Clone()` with its own `tls.Config`; mutating the
  shared `http.DefaultTransport` is a finding. `InsecureSkipVerify` comes only from
  `alertmanager.tlsConfig.insecureSkipVerify` and defaults to false; any code path that turns
  it on without that key is critical (SECURITY.md explains what it exposes).
- `basicAuth` and `bearerToken` are mutually exclusive (`ErrAlertmanagerAuthExclusive`),
  `basicAuth` needs both fields, and `applyAuth` sends the same credentials to
  `/api/v2/alerts` and `/-/ready`. Credentials never reach a log line; the forwarder logs
  `alertmanager.url` on failure, so credentials in URL userinfo would be logged.
- Both paths are resolved with `ResolveReference` against an absolute path, so a path prefix
  in `alertmanager.url` is dropped. Supporting sub-path deployments is a behavior change, not
  a fix to slip in.

### Config fails closed

`config.LoadFile` → `Validate` → any error ends `run` with a non-zero exit before the server
starts. Validation requires an `http`/`https` URL with a host, a non-empty
`defaults.severityFromPriority`, `defaults.ttl > 0`, non-negative priorities and timeouts,
severities in `info`/`warning`/`critical` (plus `warn`/`crit`), an `appName` per token and no
empty token key; it normalizes in place (canonical severities, default alertname). A new key
that can make forwarding unsafe needs a validation rule and a test. Two behaviors to keep in
mind rather than rediscover:

- `yaml.Unmarshal` ignores unknown keys, so a misspelled `bearerToken` silently means an
  unauthenticated upstream.
- With no `--config.file` at all, `run` logs and exits `0` without serving.

### Metrics are a public contract

`internal/metrics` exposes `gotilert_http_requests_total` and
`gotilert_http_request_duration_seconds` (`method`, `path`, `status`),
`gotilert_forwarded_alerts_total` and `gotilert_upstream_failures_total` (`app`), on a private
registry at `/metrics`. A rename or a label change is a break for existing dashboards; call it
out. Label values are bounded: `path` is the matched route pattern or `unmatched`
(`routeLabel`), `method` a standard method or `OTHER` (`methodLabel`), `app` a configured app
name; `TestRequestMetricsLabelBoundedRoutes` pins it. A label fed by a request-controlled value
is a cardinality finding.

### Endpoint and image hardening

- `server.New` sets `ReadTimeout`, `WriteTimeout` and `IdleTimeout` (defaults 5 s / 10 s /
  60 s); removing one, or switching to the package-level `http.ListenAndServe`, is a finding.
- `/healthz`, `/readyz` and `/metrics` are unauthenticated. `/healthz` does no work;
  `/readyz` makes one bounded GET to Alertmanager per request. A change that makes either do
  more per request, or exposes anything else without a token, is a finding.
- The image (`deployments/docker/Dockerfile`): every base pinned by tag and index digest (the
  build stage on `dhi.io/golang:…-dev@sha256:…`, the runtime on `dhi.io/static:…@sha256:…`,
  and the `# syntax=` frontend line too), and an explicit `USER 65532:65532`, so the uid does
  not follow the base image's default. The runtime relies on the CA bundle `dhi.io/static`
  ships at `/etc/ssl/certs/ca-certificates.crt` — without it HTTPS to Alertmanager fails.
  Adding root, removing the explicit `USER`, a shell or package manager in the runtime stage,
  an unpinned base, or a runtime base without a CA bundle is a finding.

### Release and CI

`docs/release.md` is the contract; `.github/workflows/release.yaml` implements it.

- The `guard` job refuses any ref but `main`. The mode comes from two fail-closed lookups
  (remote tag, `ghcr.io/leinardi/gotilert:<version>`): only "not found" reads as absent.
- The image is built once, scanned by digest, copied with `skopeo copy --all
  --preserve-digests`, checked, attested and signed, and only then tagged `:<version>`. A
  second build, a scan of anything but the published digest, or a version tag before attest
  and sign is a finding.
- Trivy exceptions live only in `.trivyignore`, each with a reason and an `exp:` date.
- Every workflow starts at `permissions: contents: read`; a job asks for more only with a
  comment saying why. Actions are pinned to a full commit SHA, images to a digest.
- Commits are Conventional Commits with a scope (`conventional-pre-commit --force-scope`); the
  type decides the release bump.

## 4. Adversarial passes — language-agnostic

Do not skim for style. Run these passes, each with a "how would I make this fail" framing:

- **Correctness / logic**: off-by-one, inverted conditions (`<` vs `<=`), wrong operator
  precedence, negated guards, early returns that skip cleanup, copy-paste that kept the old
  variable. Trace one concrete failing input end to end rather than asserting "looks fine".
- **Boundaries & nil/empty**: empty slice/map/string, zero, negative, missing key, `nil`
  receiver/pointer, unset optional, first/last element, single-element collection, nil and
  empty treated as the same thing where they mean different things.
- **Aliasing**: a returned slice or map that shares its backing store with internal state, so
  a caller's write changes it; an `append` onto a slice another owner still holds.
- **Errors**: swallowed errors, `err` checked then ignored, wrapped-but-not-returned, `%v`
  where `%w` was needed so `errors.Is`/`errors.As` stop matching, wrong sentinel, panics on
  attacker- or user-controlled input, partial writes left on the error path.
- **Concurrency**: shared state without a lock, lock held across I/O or a channel op, goroutine
  leak, context not honored, map written from two goroutines, TOCTOU between check and use.
- **Resources**: unclosed file/conn/response body, an ignored `Close` error on a write, missing
  `defer`, context/timer leak, unbounded growth, work inside a loop that belongs outside it.
- **Security**: input reaching a command/path/query/HTML without validation, authz check
  missing or after the effect, secret in a log or response, unsafe deserialization, missing
  rate/size limits.
- **Contract drift**: does the code do what the commit message / PR / issue claims? A public
  signature, flag, config key, label, annotation, endpoint, status code or error text changed
  without updating every consumer and the docs (§5 (e)).
- **Tests**: does the diff add or change a test for the behavior it introduces? A test that
  passes against the *old* code, that asserts on a fake's recorded calls instead of the
  behavior they produced, or that was weakened/deleted to make the change pass — all findings.
  A bug fix with no regression test is a gap worth flagging.

Prefer one confirmed, reproducible defect over ten vague "consider"s. If you cannot name the
input and the resulting wrong behavior, it is not yet a finding — keep digging or drop it.

## 5. Always-on passes

These run on **every** review, whatever changed.

### (a) What reaches the gateway

Ask: does this change create a new place where something from outside becomes work or
output — a request value that becomes a label, an upstream call, a log field or a response
body; a config value that changes auth or TLS? If it does, walk the §3 authentication,
forwarding, TLS and metrics items against it.

### (b) Deletion smell

A diff that removes a user-visible surface — an endpoint, a token source, a config key, a
label, an annotation, a metric — and in the same breath rewrites that surface's test to assert
it is *absent* must cite the specification line that retired it. The specification here is the
README, `examples/gotilert.yaml` and SECURITY.md. A commit message is not a specification.

A test flipped from "X happens" to "X does not happen" is not evidence that X should go. Ask:
which spec line retires this surface, and does it change in this diff? If none, this is a
**critical** finding whatever the diff's stated intent was.

### (c) A new suppression has to show its work

Any new `//nolint:`, `# shellcheck disable=` or `# hadolint ignore=` must explain why the fix
does not apply: for a complexity rule, what broke when the extraction was tried; for
`varnamelen`, why the name has to be short. A reason that restates the rule, or no reason, is a
finding. So is a new exclusion in `.golangci.yaml`, and an exclusion, enable or setting there
that matches nothing in this repo.

### (d) Cross-file duplication

Before accepting a new helper, search `internal/**` and `cmd/**` for the one that already
exists, by *behavior*. `go-style-guide` §16 lists them (token extraction, JSON responses,
token redaction, severity mapping, label copying, bounded timeouts, retry waits, nil-safe
metrics, route and method labels).

### (e) Docs drift

- A flag in `parseCLI` (`cmd/gotilert/main.go`) needs the README to agree.
- A config key in `internal/config/config.go` needs its entry in `examples/gotilert.yaml` and,
  when user-facing, the README's Configuration section.
- Endpoints, token sources, label and annotation names are listed in the README. The README
  does not list `/metrics` or the metric names today; a diff that changes metrics should add
  them rather than widen the gap.

A surface the code has and the docs do not mention is a finding; so is a documented one
nothing implements, and a default in the docs that differs from the code.

## 6. Verify before you trust (don't hand-wave the gates)

Use focused tests while investigating (`go test ./internal/server -run TestAuthTokenPrecedence
-v`), then run the gates the change owes and treat a failure it caused as a confirmed finding:

| Diff touched | Run |
| --- | --- |
| any `**/*.go` | `make go-build`, `make go-vet`, `make go-test` (race detector on), then `make check` |
| `go.mod` / `go.sum` | `make go-tidy` and `make audit-deps` (govulncheck; network required) |
| `deployments/docker/Dockerfile` | hadolint via `make check`, plus `make docker-build` |
| `.github/scripts/**` | `bash .github/scripts/binary-attestation-status.test.sh` |
| `.github/workflows/release.yaml` | actionlint via `make check`; only a release dry run exercises it end to end, so say which steps you could not run |
| anything else | `make check` (pre-commit on all files) |

The `*_integration_test.go` files are ordinary tests against `httptest` servers, with no build
tag: `make go-test` runs them. golangci-lint may not be on `PATH`; run it through pre-commit.
Several hooks rewrite files (prettier, markdownlint, the golangci formatters): check `git
status` afterwards and report a rewrite as a finding. If a gate is impractical here (no Docker,
no network for govulncheck), say so and mark that risk unverified.

## 7. Report

Rank by severity, worst first. An authentication bypass, a leaked token or credential, or an
unbounded upstream wait is normally **critical**. Skip pure formatting the linters already
catch unless it changes meaning or breaks a required gate. For each finding:

```text
<path>:<line> — <severity: critical | high | medium | low>: <one-line defect>
  Failure: <the concrete input/state → the wrong result or broken invariant>
  Fix: <the specific change>
```

Findings first, then open questions or assumptions, then a one-line verdict: **block**,
**approve with nits**, or **approve** — plus which verification gates you actually ran and which
you couldn't. If you found nothing, state what you tried to break so the "no findings" is
credible. Be blunt; do not soften a real defect to be polite, and do not invent findings to look
thorough.
