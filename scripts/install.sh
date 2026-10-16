#!/usr/bin/env bash
# Downloads a stalemark release, verifies its sha256 against checksums.txt and installs the binary.
# Usage: install.sh [dest-dir]   env: VERSION (default latest), GITHUB_TOKEN/GH_TOKEN (optional)
set -euo pipefail

dest=${1:-${RUNNER_TEMP:-.}/stalemark-bin}
version=${VERSION:-latest}
auth=()
token=${GITHUB_TOKEN:-${GH_TOKEN:-}}
[ -n "$token" ] && auth=(-H "Authorization: Bearer $token")

if [ "$version" = latest ]; then
  # ${auth[@]+...}: an empty array under set -u is an error on bash 3.2 (macOS /bin/bash).
  version=$(curl -fsSL ${auth[@]+"${auth[@]}"} https://api.github.com/repos/jnvr1/stalemark/releases/latest |
    sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' | head -n1)
fi
version=${version#v}
[ -n "$version" ] || { echo "could not resolve stalemark version" >&2; exit 1; }

case "$(uname -s)" in
  Linux) os=linux ;;
  Darwin) os=darwin ;;
  MINGW* | MSYS* | CYGWIN*) os=windows ;;
  *) echo "unsupported OS: $(uname -s)" >&2; exit 1 ;;
esac
case "$(uname -m)" in
  x86_64 | amd64) arch=amd64 ;;
  arm64 | aarch64) arch=arm64 ;;
  *) echo "unsupported arch: $(uname -m)" >&2; exit 1 ;;
esac

ext=tar.gz bin=stalemark
[ "$os" = windows ] && ext=zip bin=stalemark.exe
name="stalemark_${version}_${os}_${arch}.${ext}"
# STALEMARK_BASE_URL exists only so the action workflow can test a checksum mismatch against a local file:// dir.
base=${STALEMARK_BASE_URL:-https://github.com/jnvr1/stalemark/releases/download/v$version}

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
curl -fsSL "$base/$name" -o "$tmp/$name"
curl -fsSL "$base/checksums.txt" -o "$tmp/checksums.txt"

want=$(awk -v f="$name" '{ sub(/^\*/, "", $2) } $2 == f { print $1 }' "$tmp/checksums.txt")
[ -n "$want" ] || { echo "no checksum for $name in checksums.txt" >&2; exit 1; }
if command -v sha256sum >/dev/null; then got=$(sha256sum "$tmp/$name"); else got=$(shasum -a 256 "$tmp/$name"); fi
got=${got%% *}
[ "$got" = "$want" ] || { echo "checksum mismatch for $name: expected $want, got $got" >&2; exit 1; }

mkdir -p "$dest"
if [ "$os" = windows ]; then
  # ponytail: Git Bash tar is GNU tar and cannot read zip; PowerShell is present on every Windows runner.
  powershell -NoProfile -Command "Expand-Archive -LiteralPath '$(cygpath -w "$tmp/$name")' -DestinationPath '$(cygpath -w "$tmp")\\x'"
  cp "$tmp/x/$bin" "$dest/$bin"
else
  tar -xf "$tmp/$name" -C "$tmp" "$bin"
  cp "$tmp/$bin" "$dest/$bin"
fi
chmod +x "$dest/$bin"
echo "$dest/$bin"
[ -z "${GITHUB_OUTPUT:-}" ] || echo "bin=$dest/$bin" >> "$GITHUB_OUTPUT"
