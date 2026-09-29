---
name: go-style-guide
description: >
  Project Go style rules enforced by golangci-lint v2 (all linters) + pre-commit,
  plus this gateway's own patterns (logging, context and timeouts, HTTP handlers,
  metrics). Apply when writing, editing, or reviewing any .go file in this repo.
  Consult before generating Go code, not after lint fails.
---

# Go Style Guide — gotilert

Rules derived from `.golangci.yaml` (golangci-lint v2, `default: all`) and verified
against the existing codebase in `cmd/` and `internal/`. §1–§15 are what the linters
enforce, §16–§18 are review rules, and §19–§23 are this gateway's own patterns.

golangci-lint is not on `PATH` in every environment; run it through pre-commit
(`pre-commit run golangci-lint-full --all-files`). Never commit with `--no-verify` —
fix the underlying issue instead.

Linters that are **disabled** in `.golangci.yaml`, so their rules do not apply:

| Disabled linter | Reason |
| --- | --- |
| `exhaustruct`, `exhaustruct_v5` | Requires every struct field to be set; too noisy for short-lived structs |
| `gomodguard` | Replaced by `gomodguard_v2`, which is enabled |
| `gochecknoglobals` | Package-level state is allowed (`messageID`, the logger singleton) |
| `nonamedreturns` | Named returns are allowed |
| `wsl` | The deprecated v4 linter; its successor `wsl_v5` stays enabled |

The formatters (`gci`, `gofmt`, `gofumpt`, `goimports`, `golines`) run in the
`golangci-lint-fmt` hook and in CI.

---

## 1. Import grouping

Three groups, separated by blank lines — enforced by both `gci` (explicit
`sections: standard, default, prefix(github.com/leinardi/gotilert)`) and `goimports`
(`local-prefixes: github.com/leinardi/gotilert`) simultaneously, and they must agree.
`internal/server/message.go` has stdlib and local; third-party (e.g.
`github.com/prometheus/client_golang/prometheus` in `internal/metrics`) goes between them:

```go
import (
    "encoding/json"
    "errors"
    "fmt"
    "net/http"
    "strings"
    "sync/atomic"
    "time"

    "github.com/leinardi/gotilert/internal/gotify"
    "github.com/leinardi/gotilert/internal/logger"
)
```

Within each group imports are sorted alphabetically; no blank lines within a group.

---

## 2. Error handling

### 2a. No inline error assignment in `if` (`noinlineerr`)

```go
// Wrong
if err := cfg.validateServer(); err != nil {

// Right (config.Validate)
err := cfg.validateServer()
if err != nil {
    return err
}

err = cfg.validateLogging() // = not := for the second and later assignments
```

The comma-ok form is fine and lint-clean:
`if stErr, ok := errors.AsType[alertmanager.HTTPStatusError](postErr); ok {`.

### 2b. Wrap errors with `%w` (`errorlint`)

```go
data, err := os.ReadFile(path)
if err != nil {
    return nil, fmt.Errorf("read config file %q: %w", path, err)
}
```

The prefix is a short, lowercase phrase naming the operation, not a full sentence: no
capital letters, no trailing period. Compare with `errors.Is`/`errors.As` (or Go 1.26's
`errors.AsType[T]`, used throughout `shouldRetry`), never `==` on error values. Prefer
flat code with early returns; no `else` after a `return` (`revive`'s `indent-error-flow`).

### 2c. Errors are sentinels, detail is wrapped (`err113`)

`err113` flags every `errors.New` inside a function body and every `fmt.Errorf` without a
`%w` verb. Declare the error once as a package-level sentinel (in the package's
`errors.go`; `config` keeps its own at the top of `config.go`) and wrap the detail:

```go
return fmt.Errorf("%w: %q", ErrAlertmanagerURLInvalidScheme, parsed.Scheme)
return fmt.Errorf("%w: %w", ErrDoRequest, err)
```

Export a sentinel (`Err…`) when callers need `errors.Is`. When an error must carry
structured data, implement `error` on a private struct and expose an interface, as
`statusError` / `HTTPStatusError` in `internal/alertmanager/client.go` do. Suppress with
`//nolint:err113` only when no sentinel can fit, and say why (§3).

### 2d. Aggregating multiple errors

Use `errors.Join` over a slice of wrapped errors. `config.Validate` returns the first
violation instead; keep one style per function.

### 2e. Ignoring errors explicitly

When an error return cannot be acted upon, assign it to the blank identifier:

```go
_, _ = io.WriteString(responseWriter, okBody)
defer func() { _ = resp.Body.Close() }()
```

---

## 3. `nolint` directives

`nolintlint` enforces three things:

- **Specific**: name every linter — no bare `//nolint`
- **Explanation required**: every directive needs `// reason`
- **No unused**: remove directives when the code no longer triggers that linter

The explanation says why the fix does not apply here, not which rule fired (§17). A
statement gets an inline directive; a function or type gets it on the preceding line:

```go
//nolint:gocritic // slog.Handler requires slog.Record by value; cannot change the signature.
func (handler *PlainTextHandler) Handle(_ context.Context, record slog.Record) error {
```

Multiple linters are comma-separated with no spaces. Name all that fire: `gocyclo` and
`cyclop` measure the same thing, and `gocognit` often joins them.

---

## 4. Complexity limits

| Linter | Threshold | Note |
| --- | --- | --- |
| `gocyclo` | 15 | Cyclomatic complexity |
| `cyclop` | 15 | Same metric, different linter — both fire together |
| `gocognit` | 35 | Cognitive complexity |
| `funlen` | 50 statements | Lines are disabled (`lines: -1`) |

Prefer extracting helpers over suppressing: `config.Validate` delegates to one
`validate…` method per section. Test files are exempt from `funlen`, `gocognit`,
`gocyclo`, `maintidx` and the `cyclop` "calculated cyclomatic complexity" check.

---

## 5. Magic numbers (`mnd`)

Numbers 0, 1, 2, 3 are allowed everywhere. Any other literal in an `argument`, `case`,
`condition`, or `return` position needs a named constant. Any literal used more than once,
or that needs explaining, is a constant too:

```go
const (
    defaultHTTPTimeout      = 5 * time.Second
    maxErrorBodyBytes       = 64 * 1024
    defaultRetryMaxAttempts = 3
    defaultRetryInitial     = 200 * time.Millisecond
    defaultRetryMaxBackoff  = 1 * time.Second
)
```

`strings.SplitN` is excluded from mnd checks. Test files are fully exempt from `mnd`.

---

## 6. Type aliases

Use `any` instead of `interface{}`; `gofmt`'s rewrite rule changes it anyway.

---

## 7. Struct size (`gocritic hugeParam`)

Structs over ~80 bytes passed by value trigger `hugeParam`. Pass by pointer
(`server.New(opts *Options)`, `alertmanager.New(opts *Options)`) — or suppress when an
interface fixes the signature (`slog.Handler`, §3). The same applies to `rangeValCopy`:
iterate large slices by index.

---

## 8. Line length (`lll`)

Max 140 characters. `golines` wraps automatically. Test files are exempt.

---

## 9. Forbidden packages (`depguard`)

| Forbidden | Use instead |
| --- | --- |
| `github.com/sirupsen/logrus` (rule `logger`; allowed only in `internal/logger`) | `github.com/leinardi/gotilert/internal/logger` (`logger.L()`, backed by `log/slog`) |
| `github.com/pkg/errors` (rule `forbidden-forks`) | stdlib `errors` + `fmt.Errorf(...%w...)` |
| `github.com/instana/testify` (rule `forbidden-forks`) | `github.com/stretchr/testify` |

---

## 10. Comments and `godox`

- `FIXME` is flagged by `godox`. `TODO` is allowed.
- gocritic's `whyNoLint` check is disabled, but every `//nolint` still needs an
  explanation (`require-explanation`).
- Exported symbols in new code get a doc comment beginning with the symbol name
  (`// HealthFunc returns whether the service is healthy…`). No linter asks for it here,
  and some existing ones (`alertmanager.Client`, `metrics.Metrics`) have none; do not add
  them to code you did not otherwise change.
- Inline comments explain *why*, not *what* (§17).

---

## 11. Duplication (`dupl`)

Avoid copy-pasting blocks longer than ~100 tokens (`threshold: 100`). Test files are
exempt.

---

## 12. Shadowing (`govet shadow`)

`govet` shadow detection is enabled. Name errors after their source rather than reusing
`err` in a nested scope: `postErr`, `readyErr`, `readErr`, `sleepErr`, `ctxErr`.

---

## 13. Variable naming (`varnamelen`)

`varnamelen` flags a name shorter than 3 characters whose last use is more than 5 lines
from its declaration (defaults: `min-name-length: 3`, `max-distance: 5`). Test files are
exempt.

- **Receivers are exempt**: `(m *Metrics)`, `(e *statusError)` are fine; most of the code
  still uses the long form (`(client *Client)`, `(handler *PlainTextHandler)`).
- **Parameters are checked like locals.** A one-letter parameter passes in a three-line
  function and is flagged as soon as the body grows, so name them from the start:

  ```go
  // Wrong
  func withRequestLogging(m *metrics.Metrics, h http.Handler) http.Handler

  // Right
  func withRequestLogging(metricsCollector *metrics.Metrics, next http.Handler) http.Handler
  ```

The codebase leans long: `responseWriter`, `messageIdentifier`, `metricsCollector`.

---

## 14. `modernize` — no pointer-boxing helpers

The `modernize` linter (`newexpr` check) flags any function whose sole purpose is to return
a pointer to its argument — the generic `func ptr[T any](v T) *T` included — at the
declaration and at every call site. Go 1.26's `new` takes an expression: write
`new(true)` or `new(int64(5))`. Taking the address of a local is fine too.

---

## 15. Constant strings (`goconst`)

String literals appearing 3+ times with length ≥ 2 become a named constant. Test files are
exempt. Existing examples: the route constants (`metricsPath`, `healthzPath`, `readyzPath`,
`messagePath`) and the metric label fallbacks (`unmatchedRoute`, `otherMethod`) in
`internal/server/http.go`; the severity and log level names in `internal/config/config.go`.

---

## 16. Reuse before writing

| Need | Use | Not |
| --- | --- | --- |
| A logger | `logger.L()`; `logger.Configure` in `main`, `logger.Set` for tests | a package-local `slog.New`, `log.Printf`, or a logger parameter |
| The token of a request | `extractToken` / `authenticate` (`internal/server/message.go`) | a second header or query lookup |
| A JSON or error response | `writeJSON`, `writeJSONError`, `writePlainText` (`internal/server`) | setting `Content-Type` and encoding by hand |
| Naming a token in an error | `tokenKeyForError` (`internal/config`) | the token itself |
| Severity names | `canonicalSeverity`, `validateSeverity` (`internal/config`); `severityForPriority` (`cmd/gotilert`) | a second alias table or lookup |
| Copying or merging label maps | `copyLabels`, `copySeverityMap`, `mergeStringMap` (`cmd/gotilert/main.go`) | sharing the config's map with a request |
| A timeout that must not extend the caller's deadline | `withBoundedTimeout` (`cmd/gotilert`) | a bare `context.WithTimeout` on the request context |
| A retry wait | `computeBackoff` + `sleepWithContext` (`internal/alertmanager`) | `time.Sleep` |
| Deciding whether to retry | `shouldRetry` (`ShouldRetry` for the external tests) | a second classification at a call site |
| Recording a metric | the `*metrics.Metrics` methods (`ObserveRequest`, `IncForwarded`, `IncUpstreamFailure`), all nil-safe | a new registry, or `prometheus.DefaultRegisterer` |
| Metric label values for a request | `routeLabel`, `methodLabel` (`internal/server/http.go`) | `request.URL.Path` or `request.Method` as sent |
| A well-known Gotify extra | `ExtrasAnnotations` + `extrasStringAtPath` (`internal/gotify`) | type assertions at the call site |
| A server in a test | `newTestServer`, `mustJSON` (`internal/server/auth_integration_test.go`); `minimalValidConfig` (`internal/config/config_test.go`) | a per-file copy |

---

## 17. Comments carry rationale; history goes in the commit

A comment says **why the code is the way it is** — the constraint, the failure it avoids,
the alternative that was rejected. It does not narrate what changed, when, or at whose
request; that belongs in the commit body.

```go
// Bad — history in the code.
// Changed after the Gotify client bug report; used to reject unknown fields.

// Good — rationale in the code (parseJSON).
// Compatibility: do NOT DisallowUnknownFields (Gotify clients may send extras, etc.)
```

The same rule makes `//nolint` explanations useful: say why the fix does not apply here.

---

## 18. Waiting in tests: classify before you write a sleep

There are no `time.Sleep` calls in this repo's tests today; the tests call handlers and the
client synchronously against `httptest` servers and count requests with `atomic.Int32`. Keep it
that way. If a test ever has to wait, decide the class first:

- **Positive eventual — never a sleep.** Wait on a channel or poll with a deadline and fail
  naming what never happened. There is no shared helper yet: add one when the first site
  needs it, not speculatively.
- **Negative assertion — bounded and commented.** Give the wrong behavior a bounded window,
  assert it did not appear, and say in a comment that this is what the wait is.
- **Real elapsed window — the duration is the point.** A backoff step. Name it as a constant
  or a multiple of the interval under test (`defaultRetryInitial`), never a literal chosen by
  feel.
- **Ordering barrier with no quiescence signal** — say in a comment why no seam exists.

A sleep whose comment says "give X time to Y" where Y is observable, or one added to make a
flaky test pass, is always wrong.

---

## 19. Project layout, naming, license header

```text
gotilert/
├── cmd/gotilert/          # flags, config loading, wiring, and the forwarder (message → alert)
├── internal/
│   ├── alertmanager/      # Alertmanager client: auth, TLS option, retries, readiness probe
│   ├── config/            # YAML config, validation and normalization
│   ├── gotify/            # Gotify request parsing, extras → annotations
│   ├── logger/            # slog setup and the plain handler
│   ├── metrics/           # Prometheus metrics on a private registry
│   └── server/            # HTTP server: /message, /healthz, /readyz, /metrics
├── deployments/docker/    # Dockerfile, compose example
└── examples/gotilert.yaml # annotated configuration
```

- `cmd/` wires, and also holds the forwarder: label merging, annotations, severity mapping.
- Sentinels live in the package's `errors.go`, shared types in its `types.go`.
- Every `.go` file starts with the MIT license block (`Copyright (c) 2025 Roberto
  Leinardi`), then the `package` line; a package doc comment (`// Package server exposes …`)
  goes once per package.
- `version`, `commit`, `date` in `cmd/gotilert/version.go` are `var`s so `-ldflags -X` can
  set them.

---

## 20. Logging

- `log/slog` only, through `logger.L()`. `run` configures it from the CLI flags first, then
  again from the config file where no flag overrode it (`applyLoggingConfig`).
- Structured key/value fields, never `fmt.Sprintf` in a log call:

  ```go
  logger.L().Error("forward to alertmanager failed", logArgs...)
  ```

  Established keys: `"err"`, `"app"`, `"upstream"`, `"upstream_status"`, `"upstream_body"`,
  `"method"`, `"path"`, `"status"`, `"duration"`. Reuse them.
- Never log a token, a password, a bearer token or a full request URL: the token may be in the
  query string, which is why the request log records `request.URL.Path` only.
- There is no fatal level: `run` returns an error and `main` exits once.

---

## 21. Context and timeouts

- A function that does I/O takes `ctx context.Context` first; outbound requests use
  `http.NewRequestWithContext`.
- Every wait on Alertmanager is bounded: the per-attempt `http.Client.Timeout`, the whole
  forward under `withBoundedTimeout`, and `/readyz` under `defaultReadyTimeout`. A new
  outbound call without a deadline is a finding.
- A wait between attempts selects on `ctx.Done()` (`sleepWithContext`), and `PostAlerts`
  checks `ctx.Err()` after every attempt, so a cancelled or expired context ends the retry
  loop at once.

---

## 22. HTTP server and handlers

- Build an `http.Server` in `server.New` with explicit `ReadTimeout`, `WriteTimeout` and
  `IdleTimeout` (`ReadHeaderTimeout` falls back to `ReadTimeout`); never the package-level
  `http.ListenAndServe`. Shut down with `server.Shutdown` under the configured timeout.
- Handlers are closures returning `http.HandlerFunc` (`healthHandler`, `messageHandler`);
  routes are registered in `server.New`; an unused `*http.Request` is named `_`.
- `/message` authenticates before it reads the body, then caps the body with
  `http.MaxBytesReader`. Error bodies are `{"error": …}` via `writeJSONError`, and an
  upstream failure returns the generic `ErrUpstreamFailed`, never the upstream response.

---

## 23. Prometheus metrics

- All metrics live in `internal/metrics` on a private `prometheus.Registry`, registered
  explicitly in `metrics.New()` (no `init()`), exposed with `promhttp.HandlerFor`.
- Names are `gotilert_…`; a rename or a label change breaks dashboards (see the
  `adversarial-review` invariants).
- Label values come from bounded sets only: app names from the config, route patterns
  (`routeLabel`), standard methods (`methodLabel`), status codes.
- The recording methods on `*Metrics` return early on a nil receiver, so callers need no
  guard.

---

## What to avoid

- logrus or `pkg/errors` (§9).
- `log.Fatal` or `os.Exit` outside `main`.
- A token, credential or raw query string in a log line or an error message (§20).
- An outbound call without a deadline, or a retry wait that ignores `ctx` (§21).
- A request-controlled value as a metric label (§23).
- `interface{}` (§6) and pointer-boxing helpers (§14).
- Designing for hypothetical requirements: no configurability or abstractions for features
  that do not exist yet.
- Skipping or suppressing pre-commit hooks (`--no-verify`).
- Adding comments to code you did not change (§10).

---

## Quick checklist before submitting Go code

- [ ] Imports in 3 groups: stdlib / third-party / local, alphabetical within each
- [ ] No `if err := f(); err != nil` — split to two lines
- [ ] All errors wrapped with `%w`; `errors.Is` / `errors.AsType` for classification
- [ ] No `errors.New` or `%w`-less `fmt.Errorf` in a function body: wrap a package-level sentinel
- [ ] `any` not `interface{}`; `new(expr)`, not a pointer-boxing helper
- [ ] Numbers other than 0–3 extracted to named constants (non-test code)
- [ ] Each `//nolint` names specific linters and explains why the fix does not apply
- [ ] No `FIXME` comments
- [ ] Function statement count ≤ 50 (non-test code)
- [ ] No shadowed variables; errors named after their source
- [ ] Checked §16 for an existing helper before writing a new one
- [ ] Comments say why, not what changed — history is in the commit body (§17)
- [ ] No guessed `time.Sleep` in tests (§18)
- [ ] No token or credential reaches a log line or an error message
- [ ] Every outbound call bounded by a deadline; retry waits honor `ctx`
- [ ] Metric labels from bounded values only; any metric change reflected in the docs
