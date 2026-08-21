#!/bin/sh
# Install pogo.
#
#   curl -fsSL https://raw.githubusercontent.com/rmpato/poke/main/install.sh | sh
#
# Downloads the latest release for this platform, verifies its checksum, and
# installs the binary. Set POGO_INSTALL_DIR to choose where; set POGO_VERSION
# to pin a version.
#
# This script is POSIX sh on purpose: it has to run under dash, ash and busybox,
# not only bash.
set -eu

REPO="rmpato/poke"
INSTALL_DIR="${POGO_INSTALL_DIR:-}"
VERSION="${POGO_VERSION:-latest}"

say()  { printf '%s\n' "$*"; }
info() { printf '\033[36m==>\033[0m %s\n' "$*"; }
warn() { printf '\033[33mwarning:\033[0m %s\n' "$*" >&2; }
die()  { printf '\033[31merror:\033[0m %s\n' "$*" >&2; exit 1; }

need() {
  command -v "$1" >/dev/null 2>&1 || die "this script needs $1"
}

detect_platform() {
  os=$(uname -s | tr '[:upper:]' '[:lower:]')
  arch=$(uname -m)

  case "$os" in
    linux|darwin) ;;
    *) die "unsupported operating system: $os (pogo supports Linux and macOS)" ;;
  esac

  case "$arch" in
    x86_64|amd64) arch=amd64 ;;
    aarch64|arm64) arch=arm64 ;;
    *) die "unsupported architecture: $arch" ;;
  esac

  printf '%s_%s' "$os" "$arch"
}

# choose_dir picks the first writable directory that is already on PATH, so
# pogo works without the user editing their shell profile.
choose_dir() {
  if [ -n "$INSTALL_DIR" ]; then
    printf '%s' "$INSTALL_DIR"
    return
  fi
  for dir in "$HOME/.local/bin" /usr/local/bin "$HOME/bin"; do
    case ":$PATH:" in
      *":$dir:"*)
        if [ -w "$dir" ] || { [ ! -d "$dir" ] && [ -w "$(dirname "$dir")" ]; }; then
          printf '%s' "$dir"
          return
        fi
        ;;
    esac
  done
  # Nothing writable on PATH: fall back and tell the user what to add.
  printf '%s' "$HOME/.local/bin"
}

resolve_version() {
  if [ "$VERSION" != "latest" ]; then
    printf '%s' "$VERSION"
    return
  fi
  # The releases API reports the tag directly. sed rather than jq, because jq is
  # not on every machine that has curl.
  tag=$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" 2>/dev/null |
        sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' |
        head -1) || tag=""

  # Fall back to the redirect when the API is rate limited (it allows 60
  # unauthenticated requests an hour, which a shared IP can exhaust).
  if [ -z "$tag" ]; then
    url=$(curl -fsSLI -o /dev/null -w '%{url_effective}' \
          "https://github.com/$REPO/releases/latest" 2>/dev/null) || url=""
    case "$url" in
      */releases/tag/*) tag=${url##*/} ;;
    esac
  fi

  [ -n "$tag" ] ||
    die "could not determine the latest version. Set it explicitly:
  POGO_VERSION=v0.1.0 sh install.sh"
  printf '%s' "$tag"
}

verify_checksum() {
  archive=$1 sums=$2 name=$3

  expected=$(grep " $name\$" "$sums" | awk '{print $1}' | head -1)
  [ -n "$expected" ] || die "no checksum published for $name"

  if command -v sha256sum >/dev/null 2>&1; then
    actual=$(sha256sum "$archive" | awk '{print $1}')
  elif command -v shasum >/dev/null 2>&1; then
    actual=$(shasum -a 256 "$archive" | awk '{print $1}')
  else
    warn "no sha256 tool found; skipping checksum verification"
    return 0
  fi

  [ "$actual" = "$expected" ] ||
    die "checksum mismatch for $name
  expected $expected
  actual   $actual"
  info "checksum verified"
}

main() {
  need curl
  need tar

  platform=$(detect_platform)
  tag=$(resolve_version)
  version=${tag#v}
  dir=$(choose_dir)

  name="pogo_${version}_${platform}.tar.gz"
  base="https://github.com/$REPO/releases/download/$tag"

  info "installing pogo $tag ($platform)"

  tmp=$(mktemp -d)
  trap 'rm -rf "$tmp"' EXIT INT TERM

  curl -fsSL "$base/$name" -o "$tmp/$name" ||
    die "could not download $base/$name"
  curl -fsSL "$base/checksums.txt" -o "$tmp/checksums.txt" ||
    die "could not download checksums.txt"

  verify_checksum "$tmp/$name" "$tmp/checksums.txt" "$name"

  tar -xzf "$tmp/$name" -C "$tmp"
  [ -f "$tmp/pogo" ] || die "the archive did not contain pogo"

  mkdir -p "$dir"
  # Install to a temporary name and rename, so an in-use binary is replaced
  # atomically rather than truncated underneath a running process.
  chmod +x "$tmp/pogo"
  if ! mv "$tmp/pogo" "$dir/pogo.new" 2>/dev/null; then
    die "cannot write to $dir
  Try:  POGO_INSTALL_DIR=\$HOME/.local/bin sh install.sh
  Or:   sudo POGO_INSTALL_DIR=$dir sh install.sh"
  fi
  mv "$dir/pogo.new" "$dir/pogo"

  info "installed to $dir"

  case ":$PATH:" in
    *":$dir:"*) ;;
    *)
      warn "$dir is not on your PATH. Add this to your shell profile:"
      say ""
      say "    export PATH=\"$dir:\$PATH\""
      say ""
      ;;
  esac

  say ""
  say "  pogo curl https://api.github.com/zen   # make a request"
  say "  pogo                                   # browse what you have run"
  say ""
  say "pogo stores request history locally, including headers that may carry"
  say "credentials. See: https://github.com/$REPO/blob/main/docs/security.md"
}

main "$@"
