# Contributing to agx

Thanks for your interest in improving **agx**. This guide covers how to get set up, the conventions we follow, and what a good pull request looks like.

By participating, you agree to abide by our [Code of Conduct](CODE_OF_CONDUCT.md).

## Getting started

```sh
git clone https://github.com/khanakia/agx.git
cd agx
task check   # gofmt, go vet, staticcheck, go test -race, build (needs Go 1.25+, Task, staticcheck)
```

## Ways to contribute

- **Report a bug** — open an issue using the bug template; include steps to reproduce, your OS/version, and what you expected.
- **Request a feature** — open an issue using the feature template; describe the problem first, then your proposed solution.
- **Send a pull request** — for anything non-trivial, open an issue first so we can agree on the approach before you write code.

## Branches & commits

- Branch off `main`. Use a short descriptive branch name (`fix/...`, `feat/...`, `docs/...`).
- Write commits in **Conventional Commit** style: `type(scope): description` (`feat`, `fix`, `docs`, `refactor`, `test`, `chore`).
- Keep commits focused; one logical change per commit where practical.

## Pull request checklist

- [ ] The change is scoped and described (link the issue it closes).
- [ ] `task check` is green (lint, format, race tests, build).
- [ ] New behavior has tests where it makes sense.
- [ ] Docs/README/CHANGELOG updated if the change is user-facing.
- [ ] No unrelated files or formatting churn.

## Changelog

User-facing changes go under `[Unreleased]` in [CHANGELOG.md](CHANGELOG.md), following [Keep a Changelog](https://keepachangelog.com/).

## Questions

Open a [discussion or issue](https://github.com/khanakia/agx/issues). We're happy to help you land your first contribution.
