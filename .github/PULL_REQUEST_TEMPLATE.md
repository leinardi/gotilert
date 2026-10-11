<!-- markdownlint-disable MD041 -->
<!--
Any HTML comment will be stripped when the markdown is rendered, so you don't need to delete them.
-->

## Summary

<!-- Short description of what this PR does. -->

---

## Pull request checklist

- [ ] I am targeting the `main` branch
- [ ] I have **rebased** this branch on top of the destination branch
- [ ] I have executed `make check` and `make verify-go` locally *before creating the commit* and
  they have run successfully
- [ ] Every commit message follows [Conventional
  Commits](https://www.conventionalcommits.org/en/v1.0.0/) with a scope (`type(scope): subject`)
- [ ] Each commit type matches whether the change should ship (`fix`/`feat` for anything users
  receive in the binary or the image; see `AGENTS.md`, "Commit messages")
- [ ] I have performed a self-review of my own code
- [ ] There are no `WIP` commits in this PR

---

## Type of changes

<!-- Tick all that apply: -->

- [ ] 🐛 Bug fix
- [ ] ✨ New feature
- [ ] 🔧 Refactoring
- [ ] 📜 Docs
- [ ] 🧰 CI / tooling / infra
- [ ] Other (describe in Summary)
