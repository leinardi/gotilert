# AGENTS.md

## What this is

A Gotify-compatible gateway that forwards notifications to Prometheus Alertmanager. It implements a small subset of Gotify's
app API (`POST /message`, JSON and form), authenticates each request with an app token, maps the message to a firing alert and
posts it to Alertmanager's `/api/v2/alerts`, with bounded retries on transient errors and upstream `429`/`5xx`. No users, no UI, no
message history. The README is the user-facing contract: endpoints, token sources, configuration keys and the field-to-label and
extras-to-annotation mapping.

## Common commands

```bash
make go-build      # CGO_ENABLED=0 build into ./dist/
make go-test       # go test -race ./...
make go-vet
make go-tidy       # go mod tidy + go mod verify
make audit-deps    # govulncheck over every package (network required); also runs in CI and as a pre-commit hook on go.mod/go.sum changes
make check         # pre-commit on all files
make check-stage   # pre-commit on the staging area only
make docker-build
make docker-run
```

Single test:

```bash
go test ./internal/server -run TestAuthTokenPrecedence -v
```

The Makefile pulls shared snippets from `leinardi/make-common@v1` into `.mk/` on first run. To refresh: `make mk-common-update`.
Project targets live in the local `.mk/*.mk` files listed in `MK_LOCAL_FILES`, never as recipes in the Makefile.

## Layout

- `cmd/gotilert/` — flags, config loading, wiring, and the forwarder that turns a parsed message into an alert and logs upstream
  failures.
- `internal/server/` — the HTTP server: `/message` (token extraction and app resolution in `message.go`), `/healthz`, `/readyz`,
  request logging and metrics.
- `internal/gotify/` — parsing the Gotify request (JSON and form) and the well-known `extras` → annotation mapping.
- `internal/alertmanager/` — the Alertmanager client: auth, TLS options, retries and the readiness probe.
- `internal/config/` — the YAML config and its validation (`defaults.ttl > 0`, `severityFromPriority`, per-app tokens).
- `internal/logger/`, `internal/metrics/` — slog setup; Prometheus metrics.
- `deployments/docker/` — the Dockerfile (bases pinned by digest) and a compose example.
- `docs/release.md` — how a release is cut and recovered.

## Invariants

The full list, with what counts as a finding, is in `.agents/skills/adversarial-review/SKILL.md`. The ones to keep in mind while
writing code:

- **Every `/message` is authenticated.** A missing or unknown token is rejected before anything is parsed or forwarded; tokens are
  secrets and never logged.
- **The README is the contract.** Endpoints, token sources, configuration keys, and the label and annotation names an alert carries
  change together with the README; Alertmanager routes and dashboards key on them.
- **Every message is its own alert.** The `gotilert_id` label makes each `POST /message` a distinct alert; Alertmanager
  deduplicates by labels, so removing it silently merges notifications.
- **Bounded forwarding.** Retries are bounded and backed off; nothing waits on Alertmanager without a timeout.

## Conventions worth knowing

- Version strings (`version`, `commit`, `date`) live in `cmd/gotilert/version.go` and are filled by `-ldflags -X main.version=...`
  from `GO_LDFLAGS` in `.mk/go.mk`.
- Every Go file carries the MIT license header.
- A new `//nolint`, `# shellcheck disable=` or `# hadolint ignore=` explains why the fix does not apply here, not which rule fired.

## Project skills

Skills live in `.agents/skills/` (symlinked as `.claude/skills`). Load them before the work, not after review:

- `go-style-guide` — before any `.go` edit.
- `adversarial-review` — for any review request ("review my diff", "is this ready to merge").

## Commit messages

All commits MUST be Conventional Commits 1.0.0 **with a scope**: `<type>(<scope>)[!]: <description>`, optional blank-line body and
footers. Enforced by the `conventional-pre-commit` `commit-msg` hook (`--force-scope`) and by the `conventional-commits` CI job.
Types: `feat`, `fix`, `docs`, `test`, `refactor`, `perf`, `build`, `ci`, `chore`, `style`, `revert`. Breaking changes use `!` before
`:` or a `BREAKING CHANGE:` footer. Release notes are not built from these messages: `gh release create --generate-notes` lists the
merged pull requests by title. Examples: `fix(alertmanager): retry on 503`, `feat(gotify): map the click URL extra`.

Release versions are derived by `svu` from the commits since the last tag, so a wrong type ships a wrong version:

| Release | Commit | Example |
| --- | --- | --- |
| major | any type with `!` before the colon, or a `BREAKING CHANGE:` footer | `feat(config)!: rename the apps key` |
| minor | `feat` | `feat(gotify): map the click URL extra` |
| patch | `fix` | `fix(alertmanager): retry on 503` |
| none | everything else: `perf`, `refactor`, `build`, `ci`, `chore`, `docs`, `style`, `test`, `revert` | `perf(server): ...` |

The highest bump among the commits wins; with only "none" commits since the last tag, a release with no version fails with
"nothing to bump".

**Pick the type by whether the change should ship, not by what kind of change it is.** Anything that changes the shipped binary or
image and that users should receive is `fix` (or `feat`), even when it is a performance improvement, a refactor or a revert. Use
`perf`, `refactor`, `style` and `revert` only when the commit is deliberately not meant to trigger a release on its own. A `revert`
of a shipped `feat` or `fix` is itself a `fix`. `svu` matches `feat`/`fix` anywhere in the subject (e.g. `prefix:` counts as
`fix:`), so avoid a word ending in `feat` or `fix` directly before a colon in other subjects. PRs land as merge commits, so every
commit counts, not just the PR title. See [`docs/release.md`](docs/release.md).
