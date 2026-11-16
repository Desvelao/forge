# Project scaffolding CLI

A single static Go binary that creates projects from templates, keeps a local store of templates, and can scaffold straight from a git URL.

The binary's name is injected at build time; `<app>` below stands for it (`forge` in published releases).

## Install

Linux and macOS (amd64, arm64):

```
curl -fsSL https://raw.githubusercontent.com/Desvelao/forge/main/install.sh | sh
```

The script downloads the latest release, verifies its SHA-256 checksum and installs `forge` into `~/.local/bin` (`/usr/local/bin` when run as root). Options are environment variables:

```
curl -fsSL https://raw.githubusercontent.com/Desvelao/forge/main/install.sh | FORGE_VERSION=v1.2.3 FORGE_INSTALL_DIR=/usr/local/bin sh
```

You can read the script first: [install.sh](install.sh).

### Manual install (including Windows)

Download `forge-<os>-<arch>.tar.gz` (`.zip` on Windows) and `checksums.txt` from the [Releases page](https://github.com/Desvelao/forge/releases), then verify and extract:

```
sha256sum -c --ignore-missing checksums.txt     # macOS: shasum -a 256 -c --ignore-missing checksums.txt
tar -xzf forge-linux-amd64.tar.gz
install -m 755 forge ~/.local/bin/forge
```

On Windows (PowerShell), compare `(Get-FileHash forge-windows-amd64.zip).Hash` with the matching line in `checksums.txt`, extract with `Expand-Archive`, and put `forge.exe` in a directory on your `PATH`.

To uninstall, delete the binary and the template store (`$FORGE_HOME/templates`, or the default store location).

### Releasing

Pushing a tag runs `.github/workflows/release.yml`, which lints, tests, builds the six binaries, publishes a GitHub release with the archives and `checksums.txt`, and re-downloads the uploaded files to verify the sums. Tags containing `-` (e.g. `v1.2.0-rc1`) are marked as pre-releases.

```
git tag v1.2.3 && git push origin v1.2.3
```

## Build

Only Docker and make are needed on the host. Every target runs inside the dev container.

```
make build APP_NAME=<app>     # one binary into dist/ (GOOS/GOARCH overridable)
make dist  APP_NAME=<app>     # Linux, macOS, Windows × amd64, arm64
make test
make lint
make golden                   # regenerate golden test output
make check-name APP_NAME=<app>
make run APP_NAME=<app> ARGS="template list"   # throwaway store under /tmp
make shell
```

`APP_NAME` can also live in a git-ignored `.env` file (`APP_NAME=...`).

## Use

```
<app> template add https://github.com/acme/go-service-template --name go-service
<app> template add ./my-template            # a local directory is copied into the store
<app> template list -l
<app> new go-service ./billing --var module=github.com/acme/billing
<app> new https://github.com/acme/cli-template.git ./mytool --ref v2.1.0
<app> new ./my-template ./try --dry-run     # authoring loop
<app> template update [name]
<app> template remove <name> [--yes]
<app> template manifest <name>
cd "$(<app> template dir <name> --print)"
```

`template add` takes a git URL (`--ref` picks a branch, tag or commit) or a local directory (copied, `.git` skipped, symlinks rejected). Local copies are not git clones, so `template update` skips them; re-run `add --force` to refresh one.

Set `<APP>_HOME` (the app name upper-cased) to relocate the store to `$<APP>_HOME/templates`.

## Template format

A template is a directory with `template.yml` and a `template/` directory holding the files to generate. Only files ending in `.tmpl` are rendered (and written without the suffix); everything else is copied unchanged. Every path segment is rendered too. See the fixture in `testdata/templates/basic`.

Free-text variables can be validated by the CLI with `pattern` (a Go regexp), `message` (shown instead of `must match <pattern>`) and `max_length`:

```yaml
- name: app_id
  required: true
  pattern: '^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)+$'
  message: must be a dotted lowercase id, e.g. io.github.me.app
```

The check runs on `--var` values, on defaults (templated ones after rendering) and on interactive answers, which are asked again until valid. An empty optional value skips it, and a literal default that fails it is rejected when the manifest loads.
