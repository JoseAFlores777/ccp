#!/usr/bin/env bash
# build-sidecar.sh — compila el ccp de este repo para la plataforma de destino
# y lo deja donde Tauri espera su externalBin: src-tauri/binaries/ccp-<triple>.
#
# La app usa el ccp instalado si entiende `serve`; este es el respaldo que viaja
# dentro del .app para que funcione aunque el instalado sea viejo o no exista.
#
#   TAURI_ENV_TARGET_TRIPLE (lo pone `tauri build --target …`) o el host de rustc.

set -euo pipefail

GUI_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
REPO="$(cd "${GUI_DIR}/.." && pwd)"

triple="${TAURI_ENV_TARGET_TRIPLE:-}"
if [ -z "${triple}" ] && ! command -v rustc >/dev/null 2>&1 && [ -f "${HOME}/.cargo/env" ]; then
  # shellcheck disable=SC1091
  . "${HOME}/.cargo/env"
fi
if [ -z "${triple}" ]; then
  triple="$(rustc -vV 2>/dev/null | sed -n 's/^host: //p' || true)"
fi
if [ -z "${triple}" ]; then
  case "$(uname -s)/$(uname -m)" in
    Darwin/arm64) triple=aarch64-apple-darwin ;;
    Darwin/x86_64) triple=x86_64-apple-darwin ;;
    Linux/aarch64) triple=aarch64-unknown-linux-gnu ;;
    Linux/x86_64) triple=x86_64-unknown-linux-gnu ;;
  esac
fi
if [ -z "${triple}" ]; then
  echo "no sé para qué plataforma compilar (¿falta rustc?)" >&2
  exit 1
fi

case "${triple}" in
  aarch64-apple-darwin) goos=darwin goarch=arm64 ;;
  x86_64-apple-darwin) goos=darwin goarch=amd64 ;;
  aarch64-unknown-linux-gnu) goos=linux goarch=arm64 ;;
  x86_64-unknown-linux-gnu) goos=linux goarch=amd64 ;;
  *) echo "plataforma no soportada: ${triple}" >&2; exit 1 ;;
esac

ver="$(cd "${REPO}" && git describe --tags --always --dirty 2>/dev/null || true)"
ver="${ver#v}"
ldflags="-s -w"
if [ -n "${ver}" ]; then
  ldflags="${ldflags} -X github.com/JoseAFlores777/ccp/internal/core.Version=${ver}"
fi

out="${GUI_DIR}/src-tauri/binaries/ccp-${triple}"
mkdir -p "$(dirname "${out}")"
(cd "${REPO}" && GOOS="${goos}" GOARCH="${goarch}" CGO_ENABLED=0 go build -trimpath -ldflags "${ldflags}" -o "${out}" ./cmd/ccp)
echo "sidecar: ${out} (v${ver:-dev})"
