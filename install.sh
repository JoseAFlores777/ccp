#!/usr/bin/env bash
# ============================================================
#  install.sh — instalador de dsctl
# ============================================================
set -euo pipefail
C_GRN=$'\033[32m'; C_YEL=$'\033[33m'; C_CYN=$'\033[36m'; C_RST=$'\033[0m'
ok(){ printf '%s✅ %s%s\n' "$C_GRN" "$*" "$C_RST"; }
info(){ printf '%s%s%s\n' "$C_CYN" "$*" "$C_RST"; }
warn(){ printf '%s⚠️  %s%s\n' "$C_YEL" "$*" "$C_RST"; }

SRC_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BIN_DIR="${DSCTL_BIN_DIR:-$HOME/.local/bin}"
LIB_DIR="${DSCTL_LIB_DIR:-$HOME/.local/lib/dsctl}"

[[ -f "$SRC_DIR/bin/dsctl" ]] || { echo "No encuentro bin/dsctl"; exit 1; }

mkdir -p "$BIN_DIR" "$LIB_DIR"
install -m 0755 "$SRC_DIR/bin/dsctl" "$BIN_DIR/dsctl"
install -m 0644 "$SRC_DIR/lib/paths.sh" "$LIB_DIR/paths.sh"
ok "Binario  -> $BIN_DIR/dsctl"
ok "Librería -> $LIB_DIR/paths.sh"

if ! printf '%s' "$PATH" | tr ':' '\n' | grep -qx "$BIN_DIR"; then
  warn "$BIN_DIR no está en tu PATH. Añade a tu rc:"
  echo "    export PATH=\"\$HOME/.local/bin:\$PATH\""
fi
echo
info "Siguiente:"
echo "    dsctl install        # función 'ds' + hook en tu shell"
echo "    source ~/.bashrc     # (o ~/.zshrc)"
echo "    dsctl key            # tu API key DeepSeek"
echo "    dsctl                # menú interactivo"
