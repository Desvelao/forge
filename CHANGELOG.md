# Changelog

All notable changes to this project are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project uses
[Semantic Versioning](https://semver.org/).

## [Unreleased]

## [0.1.0] - 2026-11-16

First release.

### Added
- `new <source> <dest>`: create a project from a stored template name, a git URL (`--ref` for a branch, tag or commit) or a local template directory, with `--var key=value`, `--dry-run` and interactive prompts.
- `template add | list | update | remove | manifest | dir` to manage a local store of templates, relocatable with `<APP>_HOME`.
- Template format: `template.yml` manifest plus a `template/` directory; files ending in `.tmpl` are rendered and every path segment is templated.
- Variable validation with `pattern`, `message` and `max_length`, applied to `--var` values, defaults and interactive answers.
- `version` command and build-time injection of the app name, version and commit.
- Static binaries for Linux, macOS and Windows (amd64, arm64) via `make dist`.
- Tag-triggered GitHub release workflow that lints, tests, builds, publishes archives and `checksums.txt`, and re-verifies the uploaded assets.
- `install.sh` installer for Linux and macOS with SHA-256 verification.

[Unreleased]: https://github.com/Desvelao/forge/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/Desvelao/forge/releases/tag/v0.1.0
