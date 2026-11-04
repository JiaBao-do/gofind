#!/bin/sh
# gofind installer for Linux and macOS (amd64 / arm64).
#
#   curl -fsSL https://raw.githubusercontent.com/JiaBao-do/gofind/main/install.sh | sh
#
# Environment variables:
#   GOFIND_VERSION      release tag to install (default: latest)
#   GOFIND_INSTALL_DIR  install directory      (default: ~/.local/bin)
#   GOFIND_BINARY       install this local binary instead of downloading
#   GOFIND_UNINSTALL=1  remove gofind and its PATH entry
#   GOFIND_NO_MODIFY_PATH=1  don't touch shell profiles
#
# Offline: put the release binary (e.g. gofind-linux-amd64) next to this script and
# run "sh install.sh" - it is used instead of downloading. Go is never required.
#
# Flags --uninstall, --version <tag>, --dir <path> and --binary <file> do the same.

set -eu

REPO="JiaBao-do/gofind"
VERSION="${GOFIND_VERSION:-latest}"
INSTALL_DIR="${GOFIND_INSTALL_DIR:-$HOME/.local/bin}"
LOCAL_BINARY="${GOFIND_BINARY:-}"
UNINSTALL="${GOFIND_UNINSTALL:-}"
MARKER="# added by gofind installer"

while [ $# -gt 0 ]; do
  case "$1" in
    --uninstall) UNINSTALL=1 ;;
    --version) VERSION="$2"; shift ;;
    --dir) INSTALL_DIR="$2"; shift ;;
    --binary) LOCAL_BINARY="$2"; shift ;;
    -h|--help) sed -n '2,18s/^# \{0,1\}//p' "$0"; exit 0 ;;
    *) echo "unknown option: $1" >&2; exit 1 ;;
  esac
  shift
done

info() { printf '\033[1;34m==>\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33mwarning:\033[0m %s\n' "$*" >&2; }
die()  { printf '\033[1;31merror:\033[0m %s\n' "$*" >&2; exit 1; }

detect_os() {
  case "$(uname -s)" in
    Linux) echo linux ;;
    Darwin) echo darwin ;;
    MINGW*|MSYS*|CYGWIN*) die "on Windows use install.ps1: irm https://raw.githubusercontent.com/$REPO/main/install.ps1 | iex" ;;
    *) die "unsupported OS: $(uname -s)" ;;
  esac
}

detect_arch() {
  arch="$(uname -m)"
  # A shell running under Rosetta reports x86_64 on Apple Silicon; prefer the native build.
  if [ "$(uname -s)" = Darwin ] && [ "$(sysctl -n sysctl.proc_translated 2>/dev/null || echo 0)" = 1 ]; then
    arch=arm64
  fi
  case "$arch" in
    x86_64|amd64) echo amd64 ;;
    aarch64|arm64|armv8*) echo arm64 ;;
    *) die "unsupported architecture: $arch" ;;
  esac
}

download() {
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL "$1" -o "$2"
  elif command -v wget >/dev/null 2>&1; then
    wget -qO "$2" "$1"
  else
    die "curl or wget is required"
  fi
}

sha256() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | cut -d' ' -f1
  elif command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$1" | cut -d' ' -f1
  fi
}

# Shell profile files that should get the PATH line.
profiles() {
  case "$(basename "${SHELL:-sh}")" in
    zsh)  echo "${ZDOTDIR:-$HOME}/.zshrc" ;;
    bash) if [ "$(uname -s)" = Darwin ]; then echo "$HOME/.bash_profile"; else echo "$HOME/.bashrc"; fi ;;
    fish) echo "${XDG_CONFIG_HOME:-$HOME/.config}/fish/config.fish" ;;
    *)    echo "$HOME/.profile" ;;
  esac
}

add_to_path() {
  case ":$PATH:" in *":$INSTALL_DIR:"*) return 0 ;; esac
  [ -n "${GOFIND_NO_MODIFY_PATH:-}" ] && { warn "$INSTALL_DIR is not on your PATH"; return 0; }

  for rc in $(profiles); do
    if [ -f "$rc" ] && grep -q "$MARKER" "$rc" 2>/dev/null; then
      continue
    fi
    mkdir -p "$(dirname "$rc")"
    case "$rc" in
      *config.fish) line="fish_add_path \"$INSTALL_DIR\" $MARKER" ;;
      *)            line="export PATH=\"$INSTALL_DIR:\$PATH\" $MARKER" ;;
    esac
    printf '\n%s\n' "$line" >> "$rc"
    info "added $INSTALL_DIR to PATH in $rc"
  done
  RESTART_HINT=1
}

remove_from_path() {
  for rc in "$HOME/.zshrc" "${ZDOTDIR:-$HOME}/.zshrc" "$HOME/.bashrc" "$HOME/.bash_profile" \
            "$HOME/.profile" "${XDG_CONFIG_HOME:-$HOME/.config}/fish/config.fish"; do
    if [ -f "$rc" ] && grep -q "$MARKER" "$rc"; then
      tmp="$(mktemp)"
      grep -v "$MARKER" "$rc" > "$tmp" && cat "$tmp" > "$rc"
      rm -f "$tmp"
      info "removed PATH entry from $rc"
    fi
  done
}

if [ -n "$UNINSTALL" ]; then
  rm -f "$INSTALL_DIR/gofind" && info "removed $INSTALL_DIR/gofind"
  remove_from_path
  info "gofind uninstalled"
  exit 0
fi

OS="$(detect_os)"
ARCH="$(detect_arch)"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT INT TERM

# Offline install: use a matching binary shipped next to this script.
SCRIPT_DIR="$(cd "$(dirname "$0")" 2>/dev/null && pwd || echo .)"
for dir in "$SCRIPT_DIR" "$SCRIPT_DIR/dist"; do
  if [ -z "$LOCAL_BINARY" ] && [ -f "$dir/gofind-$OS-$ARCH" ]; then
    LOCAL_BINARY="$dir/gofind-$OS-$ARCH"
  fi
done

if [ -n "$LOCAL_BINARY" ]; then
  [ -f "$LOCAL_BINARY" ] || die "binary not found: $LOCAL_BINARY"
  info "installing local build $LOCAL_BINARY"
  cp "$LOCAL_BINARY" "$TMP/gofind"
else
  ASSET="gofind-$OS-$ARCH"
  if [ "$VERSION" = latest ]; then
    BASE="https://github.com/$REPO/releases/latest/download"
  else
    BASE="https://github.com/$REPO/releases/download/$VERSION"
  fi

  info "downloading $ASSET ($VERSION)"
  download "$BASE/$ASSET" "$TMP/gofind" || die "could not download $BASE/$ASSET
  - Check that https://github.com/$REPO/releases has a published release ($VERSION) with this file.
  - From a source checkout you can install your own build instead: make install, or sh install.sh --binary <path>"

  if download "$BASE/checksums.txt" "$TMP/checksums.txt" 2>/dev/null; then
    expected="$(grep " \*\{0,1\}$ASSET\$" "$TMP/checksums.txt" | cut -d' ' -f1)"
    actual="$(sha256 "$TMP/gofind")"
    if [ -n "$expected" ] && [ -n "$actual" ]; then
      [ "$expected" = "$actual" ] || die "checksum mismatch for $ASSET"
      info "checksum verified"
    fi
  else
    warn "checksums.txt not found, skipping verification"
  fi
fi

mkdir -p "$INSTALL_DIR"
chmod 755 "$TMP/gofind"
if [ "$OS" = darwin ]; then
  xattr -d com.apple.quarantine "$TMP/gofind" 2>/dev/null || true
fi
if [ -w "$INSTALL_DIR" ]; then
  mv -f "$TMP/gofind" "$INSTALL_DIR/gofind"
else
  info "$INSTALL_DIR is not writable, using sudo"
  sudo mv -f "$TMP/gofind" "$INSTALL_DIR/gofind"
fi
info "installed $INSTALL_DIR/gofind ($("$INSTALL_DIR/gofind" version 2>/dev/null || echo "$VERSION"))"

existing="$(command -v gofind 2>/dev/null || true)"
case ":$PATH:" in
  *":$INSTALL_DIR:"*)
    if [ -n "$existing" ] && [ "$existing" != "$INSTALL_DIR/gofind" ]; then
      warn "another gofind at $existing comes first on PATH and will be used instead. Remove it to use this install."
    fi ;;
esac

RESTART_HINT=
add_to_path
if [ -n "$RESTART_HINT" ]; then
  echo
  echo "Restart your terminal (or run: export PATH=\"$INSTALL_DIR:\$PATH\") then run: gofind"
else
  echo
  echo "Run: gofind"
fi
