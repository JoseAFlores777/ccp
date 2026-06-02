#!/usr/bin/env bash
# ============================================================
#  install.sh — instalador Go-aware de ccp (v2.0)
#
#  Camino efectivo: descarga el binario prebuilt del GitHub Release que
#  corresponde a tu OS/arch y verifica su checksum sha256. Si no hay red/
#  release pero SÍ hay toolchain Go en el repo, cae a `go build`. En la
#  máquina del usuario Go NO está instalado -> el camino release es el real.
#
#  Tras instalar: borra las libs bash viejas (~/.local/lib/ccp), registra la
#  fuente para `ccp upgrade` y copia los comandos /ccp: a ~/.claude/commands.
#
#  Overrides (env):
#    CCP_REPO        slug GitHub (default JoseAFlores777/ccp)
#    CCP_RELEASE     tag a instalar (default: latest)
#    CCP_BIN_DIR     destino del binario (default ~/.local/bin)
#    CCP_LIB_DIR     libs bash a limpiar (default ~/.local/lib/ccp)
#    CCP_HOME        config de ccp (default ~/.config/ccp)
#    CCP_CLAUDE_SRC  raíz de Claude Code (default ~/.claude)
# ============================================================
set -euo pipefail

C_GRN=$'\033[32m'; C_YEL=$'\033[33m'; C_CYN=$'\033[36m'; C_RST=$'\033[0m'
ok(){ printf '%s✅ %s%s\n' "$C_GRN" "$*" "$C_RST"; }
info(){ printf '%s%s%s\n' "$C_CYN" "$*" "$C_RST"; }
warn(){ printf '%s⚠️  %s%s\n' "$C_YEL" "$*" "$C_RST" >&2; }
die(){ printf '❌ %s\n' "$*" >&2; exit 1; }

SRC_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO="${CCP_REPO:-JoseAFlores777/ccp}"
RELEASE="${CCP_RELEASE:-latest}"
BIN_DIR="${CCP_BIN_DIR:-$HOME/.local/bin}"
LIB_DIR="${CCP_LIB_DIR:-$HOME/.local/lib/ccp}"
CCP_HOME="${CCP_HOME:-$HOME/.config/ccp}"
CLAUDE_SRC="${CCP_CLAUDE_SRC:-$HOME/.claude}"

# --- detectar OS/arch -> nombre del asset ccp-<os>-<arch> ---
detect_platform() {
  local os arch
  case "$(uname -s)" in
    Darwin) os=darwin ;;
    Linux)  os=linux ;;
    *) die "OS no soportado: $(uname -s) (solo darwin/linux)" ;;
  esac
  case "$(uname -m)" in
    x86_64|amd64) arch=amd64 ;;
    arm64|aarch64) arch=arm64 ;;
    *) die "Arquitectura no soportada: $(uname -m) (solo amd64/arm64)" ;;
  esac
  printf 'ccp-%s-%s' "$os" "$arch"
}

# --- descarga $1 -> $2 con curl o wget ---
fetch() {
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL "$1" -o "$2"
  elif command -v wget >/dev/null 2>&1; then
    wget -qO "$2" "$1"
  else
    return 127
  fi
}

# --- sha256 de un archivo (portable macOS/Linux) ---
sha256_of() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  elif command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$1" | awk '{print $1}'
  else
    return 127
  fi
}

# --- instala vía GitHub Release con verificación de checksum ---
install_from_release() {
  local asset bin_tmp sums want have base
  asset="$(detect_platform)"
  bin_tmp="$TMP/$asset"; sums="$TMP/checksums.txt"

  info "Descargando $asset desde $REPO ($RELEASE)…"
  if command -v gh >/dev/null 2>&1; then
    # gh autentica (repos privados) y resuelve los redirects de los assets.
    local tag=()
    if [[ "$RELEASE" != "latest" ]]; then tag=("$RELEASE"); fi
    gh release download "${tag[@]}" --repo "$REPO" \
       --pattern "$asset" --pattern "checksums.txt" --dir "$TMP" --clobber >/dev/null 2>&1 || return 1
  else
    # repo público sin gh: descarga directa por URL.
    if [[ "$RELEASE" == "latest" ]]; then
      base="https://github.com/$REPO/releases/latest/download"
    else
      base="https://github.com/$REPO/releases/download/$RELEASE"
    fi
    fetch "$base/$asset" "$bin_tmp" || return 1
    fetch "$base/checksums.txt" "$sums" || return 1
  fi

  # checksums.txt: líneas "<sha256>  <asset>". Extrae la del asset.
  want="$(awk -v a="$asset" '$2==a || $2=="*"a {print $1}' "$sums" | head -n1)"
  [[ -n "$want" ]] || { warn "No hay checksum para $asset en checksums.txt"; return 1; }
  have="$(sha256_of "$bin_tmp")" || die "No hay sha256sum/shasum para verificar el checksum."
  [[ "$have" == "$want" ]] || die "Checksum inválido para $asset (esperado $want, obtenido $have). Abortando."

  install -m 0755 "$bin_tmp" "$BIN_DIR/ccp"
  ok "Binario (release verificado) -> $BIN_DIR/ccp"
  return 0
}

# --- fallback: compila desde el repo si hay toolchain Go ---
install_from_source() {
  command -v go >/dev/null 2>&1 || return 1
  [[ -f "$SRC_DIR/cmd/ccp/main.go" ]] || return 1
  info "Compilando desde el código (go build)…"
  # build a un temp y luego instala: go build -o se niega a sobrescribir un
  # archivo que no creó él (p.ej. el ccp bash legacy o un binario en uso).
  ( cd "$SRC_DIR" && CGO_ENABLED=0 go build -o "$TMP/ccp" ./cmd/ccp ) || return 1
  install -m 0755 "$TMP/ccp" "$BIN_DIR/ccp"
  ok "Binario (go build) -> $BIN_DIR/ccp"
  return 0
}

mkdir -p "$BIN_DIR" "$CCP_HOME"
TMP="$(mktemp -d)"; trap 'rm -rf "$TMP"' EXIT

if ! install_from_release; then
  warn "No se pudo instalar desde release; intentando go build…"
  install_from_source || die "No pude instalar ccp: ni release prebuilt ni toolchain Go disponible.
Instala Go (https://go.dev/dl) o verifica tu conexión y el release de $REPO."
fi

# --- limpiar libs bash viejas (el Go es un binario único, sin libs) ---
if [[ -d "$LIB_DIR" ]]; then
  rm -rf "$LIB_DIR"
  ok "Libs bash viejas eliminadas -> $LIB_DIR"
fi

# --- registrar fuente para 'ccp upgrade' (re-apuntada tras el rename a ccp) ---
printf '%s\n' "$SRC_DIR" > "$CCP_HOME/install-source"
ok "Fuente registrada -> $CCP_HOME/install-source"

# --- comandos /ccp: para Claude Code (se propagan vía symlinks de cada cc-home) ---
if [[ -d "$SRC_DIR/commands/ccp" ]]; then
  mkdir -p "$CLAUDE_SRC/commands/ccp"
  install -m 0644 "$SRC_DIR/commands/ccp/"*.md "$CLAUDE_SRC/commands/ccp/"
  ok "Comandos /ccp: -> $CLAUDE_SRC/commands/ccp/"
fi

if ! printf '%s' "${PATH:-}" | tr ':' '\n' | grep -qx "$BIN_DIR"; then
  warn "$BIN_DIR no está en tu PATH. Añade a tu rc:"
  echo "    export PATH=\"\$HOME/.local/bin:\$PATH\""
fi
echo
info "Siguiente:"
echo "    ccp install          # función 'ccp' + hook en tu shell"
echo "    source ~/.zshrc      # (o ~/.bashrc)"
echo "    ccp profile add work --official && ccp profile login work"
echo "    ccp                  # TUI interactiva"
