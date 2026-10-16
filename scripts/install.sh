#!/usr/bin/env bash
# Downloads a stalemark release, verifies its sha256 against checksums.txt and installs the binary.
# Usage: install.sh [dest-dir]   env: VERSION (default latest), GITHUB_TOKEN/GH_TOKEN (optional)
set -euo pipefail

dest=${1:-${RUNNER_TEMP:-.}/stalemark-bin}
version=${VERSION:-latest}
auth=()
token=${GITHUB_TOKEN:-${GH_TOKEN:-}}
[ -n "$token" ] && auth=(-H "Authorization: Bearer $token")

api=https://api.github.com/repos/jnvr1/stalemark
# Assets are fetched through the API asset endpoint: the releases/download URL is a 404 on
# private repositories, the API works on both. STALEMARK_BASE_URL bypasses the API entirely so
# the action workflow can test a checksum mismatch against a local file:// dir.
json=""
if [ -z "${STALEMARK_BASE_URL:-}" ]; then
  rel=releases/latest
  [ "$version" = latest ] || rel="releases/tags/v${version#v}"
  # ${auth[@]+...}: an empty array under set -u is an error on bash 3.2 (macOS /bin/bash).
  json=$(curl -fsSL ${auth[@]+"${auth[@]}"} -H "Accept: application/vnd.github+json" "$api/$rel")
  version=$(printf '%s' "$json" | sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' | head -n1)
fi
version=${version#v}
[ -n "$version" ] && [ "$version" != latest ] || { echo "could not resolve stalemark version" >&2; exit 1; }

# fetch <asset name> <dest>: by id through the API, or directly under STALEMARK_BASE_URL.
fetch() {
  if [ -n "${STALEMARK_BASE_URL:-}" ]; then curl -fsSL "$STALEMARK_BASE_URL/$1" -o "$2"; return; fi
  id=$(printf '%s' "$json" | tr -d '\n' |
    grep -o '"url": *"[^"]*/releases/assets/[0-9]*"[^}]*"name": *"'"$1"'"' | head -n1 |
    sed 's|.*/releases/assets/\([0-9]*\)".*|\1|')
  [ -n "$id" ] || { echo "release v$version has no asset $1" >&2; exit 1; }
  curl -fsSL ${auth[@]+"${auth[@]}"} -H "Accept: application/octet-stream" "$api/releases/assets/$id" -o "$2"
}

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

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
fetch "$name" "$tmp/$name"
fetch checksums.txt "$tmp/checksums.txt"

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
