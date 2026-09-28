# Contributing

## Prerequisites

- Go 1.26+
- Docker, for building the container image
- [`pre-commit`](https://pre-commit.com/): install the hooks once with `make pre-commit-install`. It installs both the
  `pre-commit` and the `commit-msg` hooks, so commit messages are checked when you commit, not when the release runs.

## Building and testing

```bash
make go-build      # static binary into ./dist/
make go-test       # go test -race ./...
make audit-deps    # govulncheck over every package (network required)
make docker-build  # the container image, for the host's architecture
```

## Linting

```bash
make check         # the full pre-commit suite on every file (golangci-lint, hadolint, markdownlint, shellcheck, ...)
make check-stage   # the same, on the staged files only
```

CI runs the same hooks, the tests and the vulnerability scan on every pull request (`.github/workflows/ci.yaml`).

## Branches and pull requests

- Branch names: `feat/<short-description>`, `fix/<short-description>`, `chore/<short-description>`.
- Keep pull requests focused: one logical change each.
- All CI checks must pass before merge.

## Commit messages

All commits must follow [Conventional Commits 1.0.0](https://www.conventionalcommits.org/en/v1.0.0/) with a scope:
`<type>(<scope>)[!]: <description>`. The `conventional-pre-commit` hook enforces this on `commit-msg`, and the
`conventional-commits` CI job checks it again on every pull request. Release notes are not built from these messages:
`gh release create --generate-notes` lists the merged pull requests by title.

Common types: `feat`, `fix`, `docs`, `test`, `refactor`, `perf`, `build`, `ci`, `chore`, `style`, `revert`. Use a lower-case,
imperative description:

```text
fix(alertmanager): retry on 503
feat(gotify): map the click URL extra
feat(config)!: rename the apps key
```

Mark breaking changes with `!` before the colon or a `BREAKING CHANGE: <description>` footer.

## Versioning

The release version is derived from these types: since the last release, any `feat` makes the next release a minor, any `fix` a
patch, and `!` or a `BREAKING CHANGE:` footer a major; `build`, `chore`, `ci`, `docs`, `perf`, `refactor`, `revert`, `style` and
`test` bump nothing. See [`docs/release.md`](docs/release.md).

Pick the type by whether the change should ship, not by what kind of change it is. A performance improvement, a refactor or a revert
that changes the shipped binary or image and that users should receive is a `fix` (or a `feat`). Use `perf`, `refactor`, `style` and
`revert` only when the commit is deliberately not meant to trigger a release on its own.

Pull requests are merged with merge commits; squash and rebase merging are disabled. Every commit in a pull request therefore lands
on `main` as it is and counts toward the version, so each commit needs a correct type, not just the pull request as a whole. The
`conventional-commits` CI job checks every one of them. Enabling squash merging would make the pull-request title the commit
subject instead, and would need a CI check on pull-request titles first.

## Reporting security issues

Privately, through GitHub's security advisories: see [SECURITY.md](SECURITY.md).
