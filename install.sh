#!/usr/bin/env bash
# ============================================================
#  install.sh — instalador Go-aware de ccp (v2.0)
#
#  Dos formas de ejecutarlo, ambas soportadas:
#
#    1. Remoto, de un tirón (no hace falta clonar nada):
#         curl -fsSL https://raw.githubusercontent.com/JoseAFlores777/ccp/main/install.sh | bash
#       Al no haber repo local, el script se trae el código a $CCP_HOME/src
#       (git clone --depth 1, o tarball si no hay git) para poder copiar los
#       comandos /ccp: y dejar `ccp upgrade` funcionando.
#
#    2. Desde un checkout:  ./install.sh
#
#  Camino efectivo del binario: descarga el prebuilt del GitHub Release que
#  corresponde a tu OS/arch y verifica su checksum sha256. Si no hay red/
#  release pero SÍ hay toolchain Go y código, cae a `go build`. En la máquina
#  del usuario Go NO está instalado -> el camino release es el real.
#
#  Tras instalar: borra las libs bash viejas (~/.local/lib/ccp), registra la
#  fuente para `ccp upgrade` y copia los comandos /ccp: a ~/.claude/commands.
#
#  Overrides (env):
#    CCP_REPO        slug GitHub (default JoseAFlores777/ccp)
#    CCP_RELEASE     tag a instalar (default: latest)
#    CCP_FROM_SOURCE 1 = salta el release y compila el código (necesita Go)
#    CCP_BIN_DIR     destino del binario (default ~/.local/bin)
#    CCP_LIB_DIR     libs bash a limpiar (default ~/.local/lib/ccp)
#    CCP_HOME        config de ccp (default ~/.config/ccp)
#    CCP_SRC_DIR     dónde dejar el código en modo remoto (default $CCP_HOME/src)
#    CCP_CLAUDE_SRC  raíz de Claude Code (default ~/.claude)
#    CCP_NO_SOURCE   1 = modo remoto sin traerse el código (solo binario)
# ============================================================
set -euo pipefail

C_GRN=$'\033[32m'; C_YEL=$'\033[33m'; C_CYN=$'\033[36m'; C_RST=$'\033[0m'
ok(){ printf '%s✅ %s%s\n' "$C_GRN" "$*" "$C_RST"; }
info(){ printf '%s%s%s\n' "$C_CYN" "$*" "$C_RST"; }
warn(){ printf '%s⚠️  %s%s\n' "$C_YEL" "$*" "$C_RST" >&2; }
die(){ printf '❌ %s\n' "$*" >&2; exit 1; }

# Con `curl | bash` no hay BASH_SOURCE: $0 es "bash" y dirname da ".". Da
# igual: is_checkout() comprueba el contenido, no el nombre.
SRC_DIR="$(cd "$(dirname "${BASH_SOURCE[0]:-$0}")" 2>/dev/null && pwd || echo "")"
REPO="${CCP_REPO:-JoseAFlores777/ccp}"
RELEASE="${CCP_RELEASE:-latest}"
FROM_SOURCE="${CCP_FROM_SOURCE:-0}"
BIN_DIR="${CCP_BIN_DIR:-$HOME/.local/bin}"
LIB_DIR="${CCP_LIB_DIR:-$HOME/.local/lib/ccp}"
CCP_HOME="${CCP_HOME:-$HOME/.config/ccp}"
SRC_CACHE="${CCP_SRC_DIR:-$CCP_HOME/src}"
CLAUDE_SRC="${CCP_CLAUDE_SRC:-$HOME/.claude}"
NO_SOURCE="${CCP_NO_SOURCE:-0}"

# --- ¿$1 es un checkout de ccp utilizable? ---
is_checkout() {
  [[ -n "${1:-}" && -f "$1/install.sh" && -f "$1/cmd/ccp/main.go" ]]
}

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

# --- modo remoto: traerse el código a $SRC_CACHE ---
# El binario NO lo necesita (viene del release), pero sí hacen falta los
# comandos /ccp: y un install.sh en disco para que `ccp upgrade` tenga algo
# que re-ejecutar. Nunca es fatal: si falla, se instala igual el binario.
bootstrap_source() {
  local url="https://github.com/$REPO.git" ref
  if [[ "$RELEASE" == "latest" ]]; then ref="HEAD"; else ref="refs/tags/$RELEASE"; fi

  if command -v git >/dev/null 2>&1; then
    if [[ -d "$SRC_CACHE/.git" ]]; then
      # Checkout ya existente: no pisamos trabajo sin guardar.
      if [[ -n "$(git -C "$SRC_CACHE" status --porcelain 2>/dev/null)" ]]; then
        warn "$SRC_CACHE tiene cambios sin guardar; lo uso tal cual, sin actualizar."
        return 0
      fi
      info "Actualizando el código en ${SRC_CACHE}…"
      git -C "$SRC_CACHE" fetch --depth 1 origin "$ref" >/dev/null 2>&1 \
        && git -C "$SRC_CACHE" reset --hard FETCH_HEAD >/dev/null 2>&1 \
        && return 0
      warn "No pude actualizar $SRC_CACHE; lo uso tal cual."
      return 0
    fi
    if [[ -e "$SRC_CACHE" ]] && [[ -n "$(ls -A "$SRC_CACHE" 2>/dev/null)" ]]; then
      warn "$SRC_CACHE existe y no es un checkout de ccp; no lo toco."
      return 1
    fi
    info "Clonando $REPO -> ${SRC_CACHE}…"
    local args=(clone --depth 1)
    if [[ "$RELEASE" != "latest" ]]; then args+=(--branch "$RELEASE"); fi
    git -c advice.detachedHead=false "${args[@]}" "$url" "$SRC_CACHE" >/dev/null 2>&1 && return 0
    warn "git clone falló; pruebo con el tarball…"
  fi

  # Sin git (o clone fallido): tarball del código.
  info "Descargando el código de $REPO (tarball)…"
  fetch "https://codeload.github.com/$REPO/tar.gz/$ref" "$TMP/src.tar.gz" || return 1
  mkdir -p "$SRC_CACHE"
  tar -xzf "$TMP/src.tar.gz" --strip-components=1 -C "$SRC_CACHE" || return 1
  return 0
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
  is_checkout "$SRC_DIR" || return 1
  info "Compilando desde el código (go build)…"
  # versión: el tag git más cercano (sin la "v", que el CLI antepone); si no
  # hay repo/tags, cae al default compilado en version.go.
  local ver xpkg ldflags=""
  ver="$(cd "$SRC_DIR" && git describe --tags --always --dirty 2>/dev/null || true)"
  ver="${ver#v}"
  if [[ -n "$ver" ]]; then
    xpkg="github.com/JoseAFlores777/ccp/internal/core.Version"
    ldflags="-X ${xpkg}=${ver}"
  fi
  # build a un temp y luego instala: go build -o se niega a sobrescribir un
  # archivo que no creó él (p.ej. el ccp bash legacy o un binario en uso).
  ( cd "$SRC_DIR" && CGO_ENABLED=0 go build -ldflags "$ldflags" -o "$TMP/ccp" ./cmd/ccp ) || return 1
  install -m 0755 "$TMP/ccp" "$BIN_DIR/ccp"
  ok "Binario (go build) -> $BIN_DIR/ccp"
  return 0
}

mkdir -p "$BIN_DIR" "$CCP_HOME"
TMP="$(mktemp -d)"; trap 'rm -rf "$TMP"' EXIT

# Modo remoto (`curl | bash`): no hay checkout alrededor del script, así que
# nos traemos uno. Si no se puede, seguimos: el binario sale del release.
if ! is_checkout "$SRC_DIR"; then
  if [[ "$NO_SOURCE" == "1" ]]; then
    info "CCP_NO_SOURCE=1 -> instalo solo el binario, sin traerme el código."
    SRC_DIR=""
  elif bootstrap_source && is_checkout "$SRC_CACHE"; then
    SRC_DIR="$SRC_CACHE"
    ok "Código -> $SRC_DIR"
  else
    warn "No pude obtener el código de $REPO: instalo solo el binario.
     Sin él no se copian los comandos /ccp: ni queda fuente para 'ccp upgrade'."
    SRC_DIR=""
  fi
fi

# CCP_FROM_SOURCE=1 invierte la prioridad: instala LO QUE HAY EN EL CÓDIGO en
# vez del último release. Es lo que hace falta para probar un cambio antes de
# tagearlo — sin esto, `ccp upgrade` sobre un repo con trabajo nuevo reinstala
# igualmente el binario viejo del release y parece que el cambio no llegó.
if [[ "$FROM_SOURCE" == "1" ]]; then
  info "CCP_FROM_SOURCE=1 -> compilando el código, sin tocar el release."
  install_from_source || die "No pude compilar desde ${SRC_DIR:-(sin código)}: hace falta Go (https://go.dev/dl) y el repo."
elif ! install_from_release; then
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
if is_checkout "$SRC_DIR"; then
  printf '%s\n' "$SRC_DIR" > "$CCP_HOME/install-source"
  ok "Fuente registrada -> $CCP_HOME/install-source"
else
  warn "Sin código en disco: 'ccp upgrade' no tendrá fuente registrada.
     Vuelve a lanzar el instalador cuando tengas red para arreglarlo."
fi

# --- comandos /ccp: para Claude Code (se propagan vía symlinks de cada cc-home) ---
if [[ -n "$SRC_DIR" && -d "$SRC_DIR/commands/ccp" ]]; then
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
