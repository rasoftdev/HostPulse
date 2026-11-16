#!/usr/bin/env bash
# Compila binarios estáticos de Linux (amd64 y arm64) en ./dist
set -euo pipefail
cd "$(dirname "$0")/.."
mkdir -p dist
for arch in amd64 arm64; do
  CGO_ENABLED=0 GOOS=linux GOARCH=$arch go build -trimpath -ldflags "-s -w" -o "dist/hostpulse-linux-$arch" ./cmd/hostpulse
  echo "dist/hostpulse-linux-$arch"
done
