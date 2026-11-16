#!/usr/bin/env bash
# Builds static binaries for Linux, macOS and Windows (amd64 and arm64) into ./dist
set -euo pipefail
cd "$(dirname "$0")/.."
mkdir -p dist
for target in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64; do
  os="${target%/*}"; arch="${target#*/}"; ext=""; [ "$os" = "windows" ] && ext=".exe"
  CGO_ENABLED=0 GOOS=$os GOARCH=$arch go build -trimpath -ldflags "-s -w" -o "dist/hostpulse-$os-$arch$ext" ./cmd/hostpulse
  echo "dist/hostpulse-$os-$arch$ext"
done
