#!/usr/bin/env bash
# ============================================================
#  install.sh — instalador de ccp
# ============================================================
set -euo pipefail
C_GRN=$'\033[32m'; C_YEL=$'\033[33m'; C_CYN=$'\033[36m'; C_RST=$'\033[0m'
ok(){ printf '%s✅ %s%s\n' "$C_GRN" "$*" "$C_RST"; }
info(){ printf '%s%s%s\n' "$C_CYN" "$*" "$C_RST"; }
warn(){ printf '%s⚠️  %s%s\n' "$C_YEL" "$*" "$C_RST"; }

SRC_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BIN_DIR="${CCP_BIN_DIR:-$HOME/.local/bin}"
LIB_DIR="${CCP_LIB_DIR:-$HOME/.local/lib/ccp}"

[[ -f "$SRC_DIR/bin/ccp" ]] || { echo "No encuentro bin/ccp"; exit 1; }

mkdir -p "$BIN_DIR" "$LIB_DIR"
install -m 0755 "$SRC_DIR/bin/ccp" "$BIN_DIR/ccp"
install -m 0644 "$SRC_DIR/lib/paths.sh"    "$LIB_DIR/paths.sh"
install -m 0644 "$SRC_DIR/lib/profiles.sh" "$LIB_DIR/profiles.sh"
install -m 0644 "$SRC_DIR/lib/env.sh"      "$LIB_DIR/env.sh"
install -m 0644 "$SRC_DIR/lib/cfg.sh"      "$LIB_DIR/cfg.sh"
install -m 0644 "$SRC_DIR/lib/instruct.sh" "$LIB_DIR/instruct.sh"
ok "Binario  -> $BIN_DIR/ccp"
ok "Librerías-> $LIB_DIR/{paths,profiles,env,cfg,instruct}.sh"

# Registra la fuente para 'ccp upgrade' (re-instala desde aquí).
CCP_HOME="${CCP_HOME:-$HOME/.config/ccp}"
mkdir -p "$CCP_HOME"
printf '%s\n' "$SRC_DIR" > "$CCP_HOME/install-source"
ok "Fuente registrada-> $CCP_HOME/install-source"

# Comandos /ccp: para Claude Code (se propagan a todos los perfiles vía el
# symlink commands/ de cada cc-home). CCP_CLAUDE_SRC override-able en tests.
CLAUDE_SRC="${CCP_CLAUDE_SRC:-$HOME/.claude}"
if [[ -d "$SRC_DIR/commands/ccp" ]]; then
  mkdir -p "$CLAUDE_SRC/commands/ccp"
  install -m 0644 "$SRC_DIR/commands/ccp/"*.md "$CLAUDE_SRC/commands/ccp/"
  ok "Comandos /ccp: -> $CLAUDE_SRC/commands/ccp/"
fi

if ! printf '%s' "$PATH" | tr ':' '\n' | grep -qx "$BIN_DIR"; then
  warn "$BIN_DIR no está en tu PATH. Añade a tu rc:"
  echo "    export PATH=\"\$HOME/.local/bin:\$PATH\""
fi
echo
info "Siguiente:"
echo "    ccp install          # función 'ccp' + hook en tu shell"
echo "    source ~/.zshrc      # (o ~/.bashrc)"
echo "    ccp profile add work --official && ccp profile login work"
echo "    ccp                  # menú interactivo"
