# Changelog

All notable changes to **claude-usage** are documented here.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- `claude-usage` CLI: shows Claude plan usage limits (current 5-hour session, weekly across all models, weekly per model) for every Claude Code account on the machine at once, as bars with reset countdowns.
- Automatic discovery of `~/.claude` and every `~/.claude-*` config dir; explicit dirs can be passed as arguments.
- Token resolution across `.credentials.json` and the macOS Keychain (`Claude Code-credentials-<hash>`), picking the freshest login; tokens are never refreshed or written.
- `--json` output, `--color auto|always|never` (honours `NO_COLOR`), `--timeout`, `--version`.
- Exit codes: 0 all accounts reported, 1 some account failed, 2 usage error.

[Unreleased]: https://github.com/khanakia/claude-usage/commits/main
