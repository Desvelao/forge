#!/bin/sh
# Installer for forge. Usage:
#   curl -fsSL https://raw.githubusercontent.com/Desvelao/forge/main/install.sh | sh
# Env: FORGE_VERSION (e.g. v1.2.3, default latest), FORGE_INSTALL_DIR,
#      FORGE_BASE_URL (override download location, for testing).
set -eu

REPO="Desvelao/forge"
APP="forge"
BASE="${FORGE_BASE_URL:-https://github.com/$REPO/releases}"

err() { echo "error: $*" >&2; exit 1; }
need() { command -v "$1" >/dev/null 2>&1 || err "$1 is required"; }

need curl
need tar
need uname

case "$(uname -s)" in
  Linux) os=linux ;;
  Darwin) os=darwin ;;
  *) err "unsupported OS $(uname -s); download a release manually from https://github.com/$REPO/releases" ;;
esac
case "$(uname -m)" in
  x86_64|amd64) arch=amd64 ;;
  aarch64|arm64) arch=arm64 ;;
  *) err "unsupported architecture $(uname -m)" ;;
esac

version="${FORGE_VERSION:-}"
if [ -z "$version" ]; then
  url=$(curl -fsSLI -o /dev/null -w '%{url_effective}' "$BASE/latest") || err "cannot resolve latest release"
  version=${url##*/}
fi
case "$version" in v*) ;; *) version="v$version" ;; esac

if [ -n "${FORGE_INSTALL_DIR:-}" ]; then
  dir=$FORGE_INSTALL_DIR
elif [ "$(id -u)" -eq 0 ]; then
  dir=/usr/local/bin
else
  dir=$HOME/.local/bin
fi

archive="$APP-$os-$arch.tar.gz"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT INT TERM

echo "Downloading $APP $version ($os/$arch)..."
curl -fsSL -o "$tmp/$archive" "$BASE/download/$version/$archive" || err "download failed: $BASE/download/$version/$archive"
curl -fsSL -o "$tmp/checksums.txt" "$BASE/download/$version/checksums.txt" || err "checksums.txt download failed"

want=$(awk -v f="$archive" '$2 == f || $2 == "*" f { print $1; exit }' "$tmp/checksums.txt")
[ -n "$want" ] || err "no checksum listed for $archive"
if command -v sha256sum >/dev/null 2>&1; then
  got=$(sha256sum "$tmp/$archive" | awk '{print $1}')
elif command -v shasum >/dev/null 2>&1; then
  got=$(shasum -a 256 "$tmp/$archive" | awk '{print $1}')
else
  err "sha256sum or shasum is required to verify the download"
fi
[ "$got" = "$want" ] || err "checksum mismatch for $archive (expected $want, got $got)"
echo "Checksum OK"

tar -C "$tmp" -xzf "$tmp/$archive" "$APP"
mkdir -p "$dir"
mv "$tmp/$APP" "$dir/$APP.new"
chmod 755 "$dir/$APP.new"
mv -f "$dir/$APP.new" "$dir/$APP"

echo "Installed $dir/$APP"
"$dir/$APP" version || true
case ":$PATH:" in
  *":$dir:"*) ;;
  *) echo "Note: $dir is not on your PATH. Add it, e.g.: export PATH=\"$dir:\$PATH\"" ;;
esac
