---
name: adversarial-review
description: >
  Adversarial code review of changes to gotilert — working tree, staged diff, a
  branch vs main, a commit range, or a PR — in any language (Go, bash, Dockerfile,
  Makefile, YAML, docs). Loads go-style-guide for Go paths, hunts for real defects
  and violations of the gateway's invariants (every message authenticated, tokens
  never leaked, bounded forwarding to Alertmanager, config fails closed, metrics and
  the Gotify API as public contracts), then reports ranked findings with a verdict.
  Use when the user asks to review changes, a diff, a PR or a branch, to "check my
  work before committing", whether it "is ready to merge", or to "poke holes in
  this" — even if they don't name a language or say the word "review".
---

# Adversarial Review — gotilert

You are a hostile reviewer. Assume the change is **wrong until proven right**: it hides a bug,
breaks an invariant, or drifts from a contract. Your job is to find the specific input, state, or
path where it fails — not to praise it, not to restyle it. A review that finds nothing is only
credible after you have actively tried to break the code and failed.

## Scope

When invoked directly: review the scope the user gives (`gh pr view <n>` for intent, then `gh pr
diff <n>`); with none, the uncommitted work (`git diff HEAD` plus untracked files, which no diff
shows); on a clean tree, `git diff main...HEAD`. Never review from a description of the change.

Read the stated **intent** (`git log --oneline` for the range, any linked issue or PR body): a
change that works but does something other than what it claims is a finding. Read every changed
file in full, not just the hunks, and for non-trivial changes the callers, implementations, tests
and docs of what changed, found with a reference search, not assumed from the diff: a signature or
behavior change is only safe if every call site agrees.

## Authority

For any `**/*.go` path, load `go-style-guide` before judging it; its rules are findings even when
lint is green. Every other path (Dockerfile, compose, `Makefile`, `.mk/*.mk`, workflows, other YAML,
bash, Markdown) gets the passes below plus the invariants with the **same rigor** — an
unmatched language is not a lighter review.

## Repository invariants

Check these on every review, whatever changed.

These are the ways this gateway breaks that generic reviewers miss.

### Every `/message` is authenticated, and tokens never leak

- `extractToken` (`internal/server/message.go`) reads the token from `X-Gotify-Key`, then the
  `token` query parameter, then `Authorization: Bearer` (scheme case-insensitive), in that order;
  `TestAuthTokenPrecedence*` pins it. The app is resolved by exact lookup in the config's token map.
  A missing or unknown token is `403` **before** the body is capped, parsed or forwarded. A default
  app, a fallback token, prefix or case-insensitive matching, or any path that parses or forwards
  before `authenticate` is **critical**.
- Tokens are secrets. The request log records `request.URL.Path`, never the query string; config
  errors name a token only as `tokenKeyForError` (`token(len=N)`). Logging `request.URL`,
  `RawQuery`, headers, or a token in an error or response is critical.
- Only `POST` forwards (`405` otherwise); the body is capped at 1 MiB (`http.MaxBytesReader`,
  `MaxBodyBytes`), and `message` is required and `priority` must be ≥ 0.

### The Gotify API is a public contract

Clients built for Gotify talk to this server. The response shape (`gotify.MessageResponse`: `id`,
`appid`, `message`, `title`, `priority`, `date`, `extras`), the accepted content types (JSON, form,
or none, which parses as a form), unknown JSON fields being accepted, the default priority `5`, and
the status codes (`200`, `400`, `403`, `405`, `502`, `500` when misconfigured) change only
deliberately, with the README.

### One message, one alert

- The forwarder (`newForwarder`, `cmd/gotilert/main.go`) posts one alert per message: labels are
  `defaults.labels`, then the app's labels, then the computed `alertname`, `app`, `severity`,
  `priority`, `gotilert_id` — computed wins. `startsAt` is now, `endsAt` is now + `defaults.ttl`.
  The README documents this order; changing it is a contract change.
- `gotilert_id` is what keeps two messages from merging: Alertmanager deduplicates by label set.
  Removing it, or feeding it a value that repeats within one process, merges notifications. It comes
  from an in-memory counter (`messageID`) and starts again at 1 on restart; do not rely on it being
  unique across restarts.
- Annotations are `summary`, `description`, and the four `gotify_*` extras from `ExtrasAnnotations`;
  the extras-to-annotation names are in the README.

### Forwarding is bounded

- `PostAlerts` makes at most `defaultRetryMaxAttempts` (3) attempts with `computeBackoff` (200 ms
  doubling, capped at 1 s) and waits with `sleepWithContext`. `shouldRetry` retries only `429`,
  `5xx`, and `ErrDoRequest`-wrapped network timeouts and `*net.OpError`; never a context error, an
  x509 or TLS record error, or another `4xx`. An unbounded loop, a retry on `4xx`, or a wait that
  ignores `ctx` is a finding.
- Every attempt is bounded by `http.Client.Timeout`, the whole forward by
  `withBoundedTimeout(ctx, alertmanager.timeout)`, the readiness probe by `defaultReadyTimeout` (2
  s). An upstream error body is read through `io.LimitReader(…, maxErrorBodyBytes)` (64 KiB).
- A per-attempt `http.Client.Timeout` error is a `*url.Error` that also matches
  `context.DeadlineExceeded`, so `shouldRetry` checks it first and retries it, as the README
  promises. Reordering those checks turns per-attempt timeouts into permanent failures. A change to
  retry or timeouts is tested against a slow `httptest` server, as
  `TestPostAlertsRetriesAfterAttemptTimeout` does; a synthetic `net.Error` does not catch it.
- A failed forward is logged with the upstream status and body and counted in
  `gotilert_upstream_failures_total`; the client gets `502` with the generic `ErrUpstreamFailed`,
  never the upstream response.

### Upstream TLS and credentials

- The transport is `http.DefaultTransport.Clone()` with its own `tls.Config`; mutating the shared
  `http.DefaultTransport` is a finding. `InsecureSkipVerify` comes only from
  `alertmanager.tlsConfig.insecureSkipVerify` and defaults to false; any code path that turns it on
  without that key is critical (SECURITY.md explains what it exposes).
- `basicAuth` and `bearerToken` are mutually exclusive (`ErrAlertmanagerAuthExclusive`), `basicAuth`
  needs both fields, and `applyAuth` sends the same credentials to `/api/v2/alerts` and `/-/ready`.
  Credentials never reach a log line; the forwarder logs `alertmanager.url` on failure, so
  credentials in URL userinfo would be logged.
- Both paths are resolved with `ResolveReference` against an absolute path, so a path prefix in
  `alertmanager.url` is dropped. Supporting sub-path deployments is a behavior change, not a fix to
  slip in.

### Config fails closed

`config.LoadFile` → `Validate` → any error ends `run` with a non-zero exit before the server starts.
Validation requires an `http`/`https` URL with a host, a non-empty `defaults.severityFromPriority`,
`defaults.ttl > 0`, non-negative priorities and timeouts, severities in `info`/`warning`/`critical`
(plus `warn`/`crit`), an `appName` per token and no empty token key; it normalizes in place
(canonical severities, default alertname). A new key that can make forwarding unsafe needs a
validation rule and a test. Two behaviors to keep in mind rather than rediscover:

- `yaml.Unmarshal` ignores unknown keys, so a misspelled `bearerToken` silently means an
  unauthenticated upstream.
- With no `--config.file` at all, `run` logs and exits `0` without serving.

### Metrics are a public contract

`internal/metrics` exposes `gotilert_http_requests_total` and
`gotilert_http_request_duration_seconds` (`method`, `path`, `status`),
`gotilert_forwarded_alerts_total` and `gotilert_upstream_failures_total` (`app`), on a private
registry at `/metrics`. A rename or a label change is a break for existing dashboards; call it out.
Label values are bounded: `path` is the matched route pattern or `unmatched` (`routeLabel`),
`method` a standard method or `OTHER` (`methodLabel`), `app` a configured app name;
`TestRequestMetricsLabelBoundedRoutes` pins it. A label fed by a request-controlled value is a
cardinality finding.

### Endpoint and image hardening

- `server.New` sets `ReadTimeout`, `WriteTimeout` and `IdleTimeout` (defaults 5 s / 10 s / 60 s);
  removing one, or switching to the package-level `http.ListenAndServe`, is a finding.
- `/healthz`, `/readyz` and `/metrics` are unauthenticated. `/healthz` does no work; `/readyz` makes
  one bounded GET to Alertmanager per request. A change that makes either do more per request, or
  exposes anything else without a token, is a finding.
- The image (`deployments/docker/Dockerfile`): every base pinned by tag, not by digest, since dhi.io
  republishes its tags with security fixes (the build stage on `dhi.io/golang:…-dev`, the runtime on
  `dhi.io/static:…`); the `# syntax=` frontend line pinned by tag and index digest; and an explicit
  `USER 65532:65532`, so the uid does not follow the base image's default. The runtime relies on the
  CA bundle `dhi.io/static` ships at `/etc/ssl/certs/ca-certificates.crt` — without it HTTPS to
  Alertmanager fails. Adding root, removing the explicit `USER`, a shell or package manager in the
  runtime stage, a base on `latest` or with no tag, or a runtime base without a CA bundle is a
  finding.

### Release and CI

`docs/release.md` is the contract; `.github/workflows/release.yaml` implements it.

- The `guard` job refuses any ref but `main`. The mode comes from two fail-closed lookups (remote
  tag, `ghcr.io/leinardi/gotilert:<version>`): only "not found" reads as absent.
- The image is built once, scanned by digest, copied with `skopeo copy --all --preserve-digests`,
  checked, attested and signed, and only then tagged `:<version>`. A second build, a scan of
  anything but the published digest, or a version tag before attest and sign is a finding.
- Trivy exceptions live only in `.trivyignore`, each with a reason and an `exp:` date.
- Every workflow starts at `permissions: contents: read`; a job asks for more only with a comment
  saying why. Actions are pinned to a full commit SHA, images to a digest.
- **Commit type matches whether it ships.** `svu` derives the release from the commit types since
  the last tag, and pull requests land as merge commits, so every commit counts, not just the
  title. A change users should receive in the binary or the image (behavior, a security or
  dependency fix, a performance gain), typed `chore`, `docs`, `refactor`, `perf` or `build`, never
  ships on its own: a finding. So is a `feat`/`fix` on a change nobody receives (CI, docs, tests,
  tooling), which cuts a release with nothing in it; a stray `!` or `BREAKING CHANGE:` footer,
  which cuts a major; and a subject with a word ending in `feat` or `fix` right before a colon
  (`prefix:`), which `svu` counts as `fix`. A `revert` of a shipped `feat` or `fix` is itself a
  `fix` (`AGENTS.md`, "Commit messages").

## Adversarial passes

Beyond the generic passes (logic, nil and empty input, aliasing, error handling, concurrency,
resources, security), run these with a "how would I make this fail" framing, and name one
concrete failing input per candidate rather than asserting "looks fine".

- **Contract drift**: a public signature, flag, config key, label, annotation, endpoint, status
  code or error text changed without updating every consumer and the docs (*Docs drift* below).
- **Tests**: a test that passes against the *old* code, that asserts on a fake's recorded calls
  instead of the behavior they produced, or that was weakened or deleted to make the change pass
  is a finding. A bug fix with no regression test is a gap worth flagging.
- **A meta-test that can no longer fail**: the packages under `test/` and the `scripts/check-*.sh`
  hooks guard claims the code cannot check for itself (the Go line, the droast and ShellCheck pins,
  the droast label schema, the image's posture, the licence headers, the suppression counts, the
  Markdown links, no vacuous gate flags). Break the guarded thing once (move the Dockerfile's Go
  line, drop a label, strip a header, add a `//nolint` without the baseline, add `|| true` to a
  recipe) and see whether the test still fails. A check that passes because its file list is empty,
  its pattern no longer matches anything, its loop never ran or its tool was skipped is a false
  pass: a finding. So is a new tool lookup that bypasses `test/internal/toolgate`, and a
  scratch-repository git call without `gitenv.Isolated()`, which under the commit hook writes into
  the real index.

## Always-on passes

These run on **every** review, whatever changed.

### (a) What reaches the gateway

Ask: does this change create a new place where something from outside becomes work or output — a
request value that becomes a label, an upstream call, a log field or a response body; a config value
that changes auth or TLS? If it does, walk the authentication, forwarding, TLS and metrics invariants
against it.

### (b) Deletion smell

A diff that removes a user-visible surface — an endpoint, a token source, a config key, a label, an
annotation, a metric — and in the same breath rewrites that surface's test to assert it is *absent*
must cite the specification line that retired it. The specification here is the README,
`examples/gotilert.yaml` and SECURITY.md. A commit message is not a specification.

A test flipped from "X happens" to "X does not happen" is not evidence that X should go. Ask: which
spec line retires this surface, and does it change in this diff? If none, this is a **critical**
finding whatever the diff's stated intent was.

### (c) A new suppression has to show its work

Any new `//nolint:`, `# shellcheck disable=` or `# droast ignore=` must explain why the fix does
not apply: for a complexity rule, what broke when the extraction was tried; for `varnamelen`, why
the name has to be short. A reason that restates the rule, or no reason, is a finding. So is a new
exclusion in `.golangci.yaml`, and an exclusion, enable or setting there that matches nothing in
this repo.

Suppressions are ratcheted: `scripts/check-suppressions.sh` (the `check-suppressions` hook) refuses
a commit that adds one without changing `.suppressions-baseline`. A diff that raises a count in the
baseline is a decision the reviewer has to agree with, not bookkeeping; check that each raised count
matches a suppression with a real reason.

### (d) Cross-file duplication

Before accepting a new helper, search `internal/**` and `cmd/**` for the one that already exists, by
*behavior*. The *Reuse before writing* table in `go-style-guide` lists them (token extraction, JSON
responses, token redaction, severity mapping, label copying, bounded timeouts, retry waits, nil-safe
metrics, route and method labels).

### (e) Docs drift

- A flag in `parseCLI` (`cmd/gotilert/main.go`) needs the README to agree.
- A config key in `internal/config/config.go` needs its entry in `examples/gotilert.yaml` and, when
  user-facing, the README's Configuration section.
- Endpoints, token sources, label and annotation names are listed in the README. If the README lacks
  the `/metrics` endpoint or the metric names, a diff that changes metrics adds them.

A surface the code has and the docs do not mention is a finding; so is a documented one nothing
implements, and a default in the docs that differs from the code.

## Gates

Do not re-run the repository's gates (`make verify-go`, `make check`, `make go-tidy`, `make
audit-deps`, `make docker-build`, `go test ./test/...`). The implementer is expected to run them and
CI runs them again, but do not assume either happened: report the verification evidence you actually
have (under adversarial-review-loop, `verification.txt` when supplied, which is the implementer's
claim and not proof; when invoked directly, the CI status or gate output in the PR or session) and
what is missing, such as a `go.mod` change with no tidy or govulncheck result, a Dockerfile change
with no image build or `test/dockerfiles` result, or a `release.yaml` change that only a release dry
run exercises end to end. Judge what is missing against `AGENTS.md`'s *Validation matrix*: every
changed path owes the checks its row names. A failure in that evidence caused by the change is a
confirmed finding. Never say that a check you did not see passed. Judge whether the change's tests
prove it: a behaviour change needs a test that fails on the old code; a change to `test/**`,
`scripts/check-*.sh` or `.suppressions-baseline` needs the mutation from the meta-test pass shown to
make the check fail. The `*_integration_test.go` files are ordinary tests against `httptest`
servers, with no build tag. When invoked directly, one focused test (`go test ./internal/server -run
TestAuthTokenPrecedence -v`) may be run to confirm a single suspected finding, never a full suite or
gate. A candidate that is neither reproduced nor traced end to end is dropped: a vague "consider" is
not a finding.

## Report

Rank by severity, worst first. An authentication bypass, a leaked token or credential, or an
unbounded upstream wait is normally **critical**. Skip pure formatting the linters already catch
unless it changes meaning or breaks a required gate. For each finding:

```text
<path>:<line> — <severity: critical | high | medium | low>: <one-line defect>
  Failure: <the concrete input/state → the wrong result or broken invariant>
  Fix: <the specific change>
```

Findings first, then open questions or assumptions, then a one-line verdict: **block**, **approve
with nits**, or **approve**, and one line: `Verification evidence: seen … / missing …`. If you
found nothing, state what you tried to break so the "no findings" is credible. Be blunt; do not
soften a real defect to be polite, and do not invent findings to look thorough.
