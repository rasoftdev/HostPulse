#!/usr/bin/env bash
# Instala el comando `hostpulse` en /usr/local/bin (uso: sudo scripts/install.sh)
set -euo pipefail
[ "$(id -u)" -eq 0 ] || { echo "Ejecuta como root: sudo $0"; exit 1; }
here="$(cd "$(dirname "$0")/.." && pwd)"
case "$(uname -m)" in
  x86_64|amd64)  arch=amd64 ;;
  aarch64|arm64) arch=arm64 ;;
  *) echo "Arquitectura no soportada: $(uname -m)"; exit 1 ;;
esac
bin="$here/dist/hostpulse-linux-$arch"
[ -f "$bin" ] || { echo "No existe $bin (ejecuta antes scripts/build.sh)"; exit 1; }
install -m 0755 "$bin" /usr/local/bin/hostpulse
echo "Instalado. Escribe: hostpulse"
