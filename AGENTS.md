# AGENTS.md

## What this is

A Gotify-compatible gateway that forwards notifications to Prometheus Alertmanager. It implements a
small subset of Gotify's app API (`POST /message`, JSON and form), authenticates each request with
an app token, maps the message to a firing alert and posts it to Alertmanager's `/api/v2/alerts`,
with bounded retries on transient errors and upstream `429`/`5xx`. No users, no UI, no message
history. The README is the user-facing contract: endpoints, token sources, configuration keys and
the field-to-label and extras-to-annotation mapping.

## Common commands

```bash
make go-build      # CGO_ENABLED=0 build into ./dist/
make go-test       # go test -race ./... (the meta-tests under test/ included)
make go-vet
make go-lint       # golangci-lint (all linters) at the pre-commit pin, over the whole module
make verify-go     # go-build, go-test, go-vet, go-lint
make verify        # every verify-* target
make go-tidy       # go mod tidy + go mod verify
make audit-deps    # govulncheck -test over every package (network required); also runs in CI and as a pre-commit hook on go.mod/go.sum changes
make check         # pre-commit on all files: the authoritative local gate, as CI's pre-commit-all
make check-stage   # pre-commit on the staging area only
make docker-build
make docker-run
```

Single test:

```bash
go test ./internal/server -run TestAuthTokenPrecedence -v
```

`make check` lints the Dockerfile with droast (Dockerfile Roast). pre-commit builds droast from
source at its pinned rev, which needs a Rust toolchain (`cargo`) locally. Its required ShellCheck
bridge runs the `shellcheck` on PATH, so install ShellCheck locally at the release
`.github/actions/setup-shellcheck` pins (CI installs it with that action). The label meta-test in
`test/dockerfiles` runs the `droast` on PATH, at the release `.github/actions/setup-droast` pins
(the hook's rev); without it, or without `shellcheck`, the test skips locally.

The Makefile pulls shared snippets from `leinardi/make-common@v1` into `.mk/` on first run. To
refresh: `make mk-common-update`. Project targets live in the local `.mk/*.mk` files listed in
`MK_LOCAL_FILES`, never as recipes in the Makefile.

## Layout

- `cmd/gotilert/` — flags, config loading, wiring, and the forwarder that turns a parsed message
  into an alert and logs upstream failures.
- `internal/server/` — the HTTP server: `/message` (token extraction and app resolution in
  `message.go`), `/healthz`, `/readyz`, request logging and metrics.
- `internal/gotify/` — parsing the Gotify request (JSON and form) and the well-known `extras` →
  annotation mapping.
- `internal/alertmanager/` — the Alertmanager client: auth, TLS options, retries and the readiness
  probe.
- `internal/config/` — the YAML config and its validation (`defaults.ttl > 0`,
  `severityFromPriority`, per-app tokens).
- `internal/logger/`, `internal/metrics/` — slog setup; Prometheus metrics.
- `deployments/docker/` — the Dockerfile (bases pinned by tag) and a compose example; the build
  context is the repository root, so `.dockerignore` sits there.
- `docs/release.md` — how a release is cut and recovered.
- `test/` — meta-tests, all ordinary Go packages: `test/toolchain` (the Go line, the droast and
  ShellCheck pins, the whole-tree hooks, no vacuous gate flags), `test/dockerfiles` (the image's
  invariants and the droast label schema), `test/docs` (Markdown links), `test/licenseheaders` and
  `test/suppressions` (drive the two `scripts/check-*.sh` hooks), and `test/internal/` (`gitenv`,
  `toolgate`). A meta-test that needs a tool gets it through `toolgate`: a skip locally, a failure
  with `GOTILERT_REQUIRE_TOOLS=1`, which CI's `go-test` job sets.
- `scripts/` — the `check-*` hooks, `bootstrap-mk-common.sh` (make-common's), and
  `hooks/gofmt-on-edit.sh` (the Claude Code edit hook in `.claude/settings.json`).

### Generated or vendored: never hand-edit

- `.mk/{docker,help,go,password,pre-commit}.mk` (the Makefile's `MK_COMMON_FILES`) and
  `scripts/bootstrap-mk-common.sh`: fetched from make-common; refresh them with
  `make mk-common-update`, and port a change upstream.

## Invariants

The full list, with what counts as a finding, is in `.agents/skills/adversarial-review/SKILL.md`.
The ones to keep in mind while writing code:

- **Every `/message` is authenticated.** A missing or unknown token is rejected before anything is
  parsed or forwarded; tokens are secrets and never logged.
- **The README is the contract.** Endpoints, token sources, configuration keys, and the label and
  annotation names an alert carries change together with the README; Alertmanager routes and
  dashboards key on them.
- **Every message is its own alert.** The `gotilert_id` label makes each `POST /message` a distinct
  alert; Alertmanager deduplicates by labels, so removing it silently merges notifications.
- **Bounded forwarding.** Retries are bounded and backed off; nothing waits on Alertmanager without
  a timeout.

## Conventions worth knowing

- Version strings (`version`, `commit`, `date`) live in `cmd/gotilert/version.go` and are filled by
  `-ldflags -X main.version=...` from `GO_LDFLAGS` in `.mk/go.mk`.
- Every source file carries the MIT license header (`/* … */` in Go, `#` lines elsewhere), with the
  year of its first commit. `scripts/check-license-headers.sh` (the always-run
  `check-license-headers` hook) fails on any tracked Go or shell file, Dockerfile, Makefile or
  repo-local `.mk` fragment without it; the make-common files are exempt.
- A new `//nolint`, `# shellcheck disable=` or `# droast ignore=` explains why the fix does not
  apply here, not which rule fired; droast also requires its `reason="..."`.

## Suppressions are ratcheted

Every in-file suppression is counted per rule against `.suppressions-baseline` by the
`check-suppressions` pre-commit hook (`scripts/check-suppressions.sh`): `//nolint:<linter>`,
`# shellcheck disable=`, `# droast ignore=` and `# droast global ignore=`.

Adding one is allowed, but never silently: the hook refuses a commit that adds a suppression line
without changing the baseline in the same commit, including a swap or a moved file that leaves the
count unchanged. Prefer fixing the finding. If a suppression really is right, write it with a reason
(`//nolint:<linter> // reason`), stage it, run `scripts/check-suppressions.sh --update`, and commit
the baseline alongside it. A count that has fallen below its baseline fails the same way, with the
same fix: the file describes the repository, not a past one.

## Validation matrix

Run before considering an edit done. `make check` applies to every row.

| You edited | Run |
| --- | --- |
| `**/*.go` | `make verify-go`, then `make check-stage` |
| `go.mod`, `go.sum` | `make go-tidy`, `make audit-deps` (network; the commit hook runs it too), `go test ./test/toolchain/` |
| `deployments/docker/Dockerfile`, `droast.toml`, `.dockerignore` | `go test ./test/dockerfiles/ ./test/toolchain/` (droast runs in `make check`); `make docker-build` (needs `docker login dhi.io`) |
| `scripts/check-license-headers.sh` | `go test ./test/licenseheaders/` |
| `scripts/check-suppressions.sh`, `.suppressions-baseline` | `go test ./test/suppressions/` |
| `scripts/hooks/gofmt-on-edit.sh` | shellcheck (in `make check`); no test package: run it once by editing a `.go` file in Claude Code |
| `scripts/bootstrap-mk-common.sh`, `.mk/{docker,help,go,password,pre-commit}.mk` | never by hand: `make mk-common-update`, then `make help` |
| `.mk/audit-deps.mk`, `.mk/docker-run.mk`, `.mk/verify.mk`, `Makefile` | `make help`, `make check`, `go test ./test/toolchain/` |
| `.github/scripts/binary-attestation-status.sh` | `bash .github/scripts/binary-attestation-status.test.sh` (also the `release-scripts` CI job) |
| `.github/workflows/**`, `.github/actions/**` | `pre-commit run actionlint --all-files`, `go test ./test/toolchain/` (pins, vacuous flags) |
| `.pre-commit-config.yaml`, `.golangci.yaml` | `make check`, `go test ./test/toolchain/` |
| `test/**` | `go test ./test/...`; break the guarded thing once and see the test fail |
| `**/*.md` | `go test -count=1 ./test/docs/` (every relative link resolves); prose wraps at 100 columns (markdownlint) |

## Project skills

Skills live in `.agents/skills/` (symlinked as `.claude/skills`). Load them before the work, not
after review:

- `go-style-guide` — before any `.go` edit.
- `adversarial-review` — for any review request ("review my diff", "is this ready to merge").

## Commit messages

All commits MUST be Conventional Commits 1.0.0 **with a scope**:
`<type>(<scope>)[!]: <description>`, optional blank-line body and footers. Enforced by the
`conventional-pre-commit` `commit-msg` hook (`--force-scope`) and by the `conventional-commits` CI
job. Types: `feat`, `fix`, `docs`, `test`, `refactor`, `perf`, `build`, `ci`, `chore`, `style`,
`revert`. Breaking changes use `!` before `:` or a `BREAKING CHANGE:` footer. Release notes are not
built from these messages: `gh release create --generate-notes` lists the merged pull requests by
title. Examples: `fix(alertmanager): retry on 503`, `feat(gotify): map the click URL extra`.

Release versions are derived by `svu` from the commits since the last tag, so a wrong type ships a
wrong version:

| Release | Commit | Example |
| --- | --- | --- |
| major | any type with `!` before the colon, or a `BREAKING CHANGE:` footer | `feat(config)!: rename the apps key` |
| minor | `feat` | `feat(gotify): map the click URL extra` |
| patch | `fix` | `fix(alertmanager): retry on 503` |
| none | everything else: `perf`, `refactor`, `build`, `ci`, `chore`, `docs`, `style`, `test`, `revert` | `perf(server): ...` |

The highest bump among the commits wins; with only "none" commits since the last tag, a release with
no version fails with "nothing to bump".

**Pick the type by whether the change should ship, not by what kind of change it is.** Anything that
changes the shipped binary or image and that users should receive is `fix` (or `feat`), even when it
is a performance improvement, a refactor or a revert. Use `perf`, `refactor`, `style` and `revert`
only when the commit is deliberately not meant to trigger a release on its own. A `revert` of a
shipped `feat` or `fix` is itself a `fix`. `svu` matches `feat`/`fix` anywhere in the subject (e.g.
`prefix:` counts as `fix:`), so avoid a word ending in `feat` or `fix` directly before a colon in
other subjects. PRs land as merge commits, so every commit counts, not just the PR title. See
[`docs/release.md`](docs/release.md).
