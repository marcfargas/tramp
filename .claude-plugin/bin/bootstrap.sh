#!/usr/bin/env bash
# Bootstrap script for tramp MCP server.
# Downloads the correct binary for the current platform on first run,
# then execs it with the given arguments.
set -euo pipefail

REPO="marcfargas/tramp"
VERSION="0.1.0"
INSTALL_DIR="${CLAUDE_PLUGIN_ROOT:-.}/.claude-plugin/bin"
BINARY_NAME="tramp"

# Detect OS
case "$(uname -s)" in
  Linux*)  OS="linux" ;;
  Darwin*) OS="darwin" ;;
  MINGW*|MSYS*|CYGWIN*) OS="windows" ;;
  *) echo "Unsupported OS: $(uname -s)" >&2; exit 1 ;;
esac

# Detect arch
case "$(uname -m)" in
  x86_64|amd64)  ARCH="amd64" ;;
  aarch64|arm64)  ARCH="arm64" ;;
  *) echo "Unsupported architecture: $(uname -m)" >&2; exit 1 ;;
esac

# Binary path
if [ "$OS" = "windows" ]; then
  BINARY="${INSTALL_DIR}/tramp.exe"
  ARCHIVE="tramp_${VERSION}_${OS}_${ARCH}.zip"
else
  BINARY="${INSTALL_DIR}/tramp"
  ARCHIVE="tramp_${VERSION}_${OS}_${ARCH}.tar.gz"
fi

# Download if not present
if [ ! -x "$BINARY" ]; then
  URL="https://github.com/${REPO}/releases/download/v${VERSION}/${ARCHIVE}"
  echo "Downloading tramp v${VERSION} for ${OS}/${ARCH}..." >&2

  TMP_DIR=$(mktemp -d)
  trap 'rm -rf "$TMP_DIR"' EXIT

  if command -v curl >/dev/null 2>&1; then
    curl -fsSL "$URL" -o "${TMP_DIR}/${ARCHIVE}"
  elif command -v wget >/dev/null 2>&1; then
    wget -q "$URL" -O "${TMP_DIR}/${ARCHIVE}"
  else
    echo "Error: curl or wget required" >&2
    exit 1
  fi

  mkdir -p "$INSTALL_DIR"

  if [ "$OS" = "windows" ]; then
    unzip -q "${TMP_DIR}/${ARCHIVE}" -d "$TMP_DIR"
  else
    tar xzf "${TMP_DIR}/${ARCHIVE}" -C "$TMP_DIR"
  fi

  mv "${TMP_DIR}/tramp"* "$BINARY"
  chmod +x "$BINARY"

  echo "Installed tramp to ${BINARY}" >&2
fi

# Exec the binary with all arguments
exec "$BINARY" "$@"
