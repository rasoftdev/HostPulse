#!/usr/bin/env bash
# Compila binarios estáticos para Linux, macOS y Windows (amd64 y arm64) en ./dist
set -euo pipefail
cd "$(dirname "$0")/.."
mkdir -p dist
for target in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64; do
  os="${target%/*}"; arch="${target#*/}"; ext=""; [ "$os" = "windows" ] && ext=".exe"
  CGO_ENABLED=0 GOOS=$os GOARCH=$arch go build -trimpath -ldflags "-s -w" -o "dist/hostpulse-$os-$arch$ext" ./cmd/hostpulse
  echo "dist/hostpulse-$os-$arch$ext"
done
