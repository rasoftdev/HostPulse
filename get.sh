#!/usr/bin/env bash
# Instalador remoto de HostPulse (Linux y macOS): clona el repositorio, compila, instala el comando
# `hostpulse` y borra todo lo temporal (el repo clonado y, si hizo falta, el Go temporal).
#
#   curl -fsSL https://raw.githubusercontent.com/rasoftdev/HostPulse/main/get.sh | sudo bash
#   curl -fsSL https://raw.githubusercontent.com/rasoftdev/HostPulse/main/get.sh | sudo bash -s -- uninstall
#
# Sin sudo se instala en ~/.local/bin. Variables opcionales:
#   HOSTPULSE_REPO (URL git), HOSTPULSE_REF (rama o etiqueta, por defecto main), HOSTPULSE_PREFIX (carpeta destino).
set -euo pipefail

REPO="${HOSTPULSE_REPO:-https://github.com/rasoftdev/HostPulse.git}"
REF="${HOSTPULSE_REF:-main}"
GO_VERSION="1.26.0"

say() { printf '\033[1;36m==>\033[0m %s\n' "$*"; }
die() { printf '\033[1;31mERROR:\033[0m %s\n' "$*" >&2; exit 1; }

case "$(uname -s)" in
  Linux)  OS=linux ;;
  Darwin) OS=darwin ;;
  *) die "Este instalador es para Linux y macOS. En Windows usa get.ps1 (ver README)." ;;
esac
case "$(uname -m)" in
  x86_64|amd64)  ARCH=amd64 ;;
  aarch64|arm64) ARCH=arm64 ;;
  *) die "Arquitectura no soportada: $(uname -m)" ;;
esac

if [ -n "${HOSTPULSE_PREFIX:-}" ]; then
  BIN_DIR="$HOSTPULSE_PREFIX"
elif [ "$(id -u)" -eq 0 ]; then
  BIN_DIR=/usr/local/bin
else
  BIN_DIR="$HOME/.local/bin"
fi

if [ "${1:-}" = "uninstall" ]; then
  say "Desinstalando HostPulse de $BIN_DIR..."
  rm -f "$BIN_DIR/hostpulse"
  say "Listo. HostPulse se ha eliminado."
  exit 0
fi

command -v git  >/dev/null || die "Falta git (apt install git / dnf install git / brew install git)."
command -v curl >/dev/null || die "Falta curl."

# Todo lo temporal vive aquí y se borra siempre al salir (con éxito o con error).
WORK="$(mktemp -d "${TMPDIR:-/tmp}/hostpulse-build.XXXXXX")"
# (la caché de módulos de Go es de solo lectura: hay que dar permisos antes de borrar)
trap 'chmod -R u+w "$WORK" 2>/dev/null || true; rm -rf "$WORK"' EXIT

say "Clonando el repositorio ($REF)..."
git clone --quiet --depth 1 --branch "$REF" "$REPO" "$WORK/repo"

# Go: usa el del sistema si es lo bastante reciente; si no, una copia temporal dentro de $WORK.
need_go=1
if command -v go >/dev/null; then
  have="$(go env GOVERSION 2>/dev/null | sed 's/^go//')"
  [ "$(printf '%s\n%s\n' "$have" "$GO_VERSION" | sort -V | head -1)" = "$GO_VERSION" ] && need_go=0
fi
if [ "$need_go" -eq 1 ]; then
  say "Descargando un Go $GO_VERSION temporal (se borrará al terminar)..."
  curl -fsSL "https://go.dev/dl/go${GO_VERSION}.${OS}-${ARCH}.tar.gz" | tar -C "$WORK" -xz
  export PATH="$WORK/go/bin:$PATH"
fi
export GOCACHE="$WORK/gocache" GOPATH="$WORK/gopath" GOTOOLCHAIN=local CGO_ENABLED=0

say "Compilando..."
( cd "$WORK/repo" && go build -trimpath -ldflags "-s -w" -o "$WORK/hostpulse" ./cmd/hostpulse )

say "Instalando en $BIN_DIR..."
mkdir -p "$BIN_DIR"
install -m 0755 "$WORK/hostpulse" "$BIN_DIR/hostpulse"

say "Limpiando archivos temporales..."
# (el trap borra $WORK al salir)
echo
echo "HostPulse instalado: $BIN_DIR/hostpulse"
case ":$PATH:" in
  *":$BIN_DIR:"*) echo "Escribe:  hostpulse" ;;
  *) echo "Añade $BIN_DIR a tu PATH (export PATH=\"$BIN_DIR:\$PATH\") y escribe:  hostpulse" ;;
esac
