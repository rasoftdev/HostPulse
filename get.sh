#!/usr/bin/env bash
# HostPulse remote installer (Linux and macOS): clones the repository, builds it, installs the
# `hostpulse` command and deletes everything temporary (the cloned repo and, if needed, a temporary Go).
#
#   curl -fsSL https://raw.githubusercontent.com/rasoftdev/HostPulse/main/get.sh | sudo bash
#   curl -fsSL https://raw.githubusercontent.com/rasoftdev/HostPulse/main/get.sh | sudo bash -s -- uninstall
#
# Without sudo it installs into ~/.local/bin. Optional variables:
#   HOSTPULSE_REPO (git URL), HOSTPULSE_REF (branch or tag, default main), HOSTPULSE_PREFIX (target folder).
set -euo pipefail

REPO="${HOSTPULSE_REPO:-https://github.com/rasoftdev/HostPulse.git}"
REF="${HOSTPULSE_REF:-main}"
GO_VERSION="1.26.0"

say() { printf '\033[1;36m==>\033[0m %s\n' "$*"; }
die() { printf '\033[1;31mERROR:\033[0m %s\n' "$*" >&2; exit 1; }

case "$(uname -s)" in
  Linux)  OS=linux ;;
  Darwin) OS=darwin ;;
  *) die "This installer is for Linux and macOS. On Windows use get.ps1 (see the README)." ;;
esac
case "$(uname -m)" in
  x86_64|amd64)  ARCH=amd64 ;;
  aarch64|arm64) ARCH=arm64 ;;
  *) die "Unsupported architecture: $(uname -m)" ;;
esac

if [ -n "${HOSTPULSE_PREFIX:-}" ]; then
  BIN_DIR="$HOSTPULSE_PREFIX"
elif [ "$(id -u)" -eq 0 ]; then
  BIN_DIR=/usr/local/bin
else
  BIN_DIR="$HOME/.local/bin"
fi

if [ "${1:-}" = "uninstall" ]; then
  say "Uninstalling HostPulse from $BIN_DIR..."
  rm -f "$BIN_DIR/hostpulse"
  say "Done. HostPulse has been removed."
  exit 0
fi

command -v git  >/dev/null || die "git is missing (apt install git / dnf install git / brew install git)."
command -v curl >/dev/null || die "curl is missing."

# Everything temporary lives here and is always deleted on exit (success or failure).
WORK="$(mktemp -d "${TMPDIR:-/tmp}/hostpulse-build.XXXXXX")"
# (Go's module cache is read-only: permissions must be restored before deleting)
trap 'chmod -R u+w "$WORK" 2>/dev/null || true; rm -rf "$WORK"' EXIT

say "Cloning the repository ($REF)..."
git clone --quiet --depth 1 --branch "$REF" "$REPO" "$WORK/repo"

# Go: use the system one if it is recent enough; otherwise a temporary copy inside $WORK.
need_go=1
if command -v go >/dev/null; then
  have="$(go env GOVERSION 2>/dev/null | sed 's/^go//')"
  [ "$(printf '%s\n%s\n' "$have" "$GO_VERSION" | sort -V | head -1)" = "$GO_VERSION" ] && need_go=0
fi
if [ "$need_go" -eq 1 ]; then
  say "Downloading a temporary Go $GO_VERSION (it will be removed when finished)..."
  curl -fsSL "https://go.dev/dl/go${GO_VERSION}.${OS}-${ARCH}.tar.gz" | tar -C "$WORK" -xz
  export PATH="$WORK/go/bin:$PATH"
fi
export GOCACHE="$WORK/gocache" GOPATH="$WORK/gopath" GOTOOLCHAIN=local CGO_ENABLED=0

say "Building..."
( cd "$WORK/repo" && go build -trimpath -ldflags "-s -w" -o "$WORK/hostpulse" ./cmd/hostpulse )

say "Installing into $BIN_DIR..."
mkdir -p "$BIN_DIR"
install -m 0755 "$WORK/hostpulse" "$BIN_DIR/hostpulse"

say "Cleaning up temporary files..."
# (the trap removes $WORK on exit)
echo
echo "HostPulse installed: $BIN_DIR/hostpulse"
case ":$PATH:" in
  *":$BIN_DIR:"*) echo "Run:  hostpulse" ;;
  *) echo "Add $BIN_DIR to your PATH (export PATH=\"$BIN_DIR:\$PATH\") and run:  hostpulse" ;;
esac
