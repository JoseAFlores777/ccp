#!/usr/bin/env bash
# ============================================================
#  lib/cfg.sh — profile config (overlay + merge) para ccp.
#
#  Cada perfil official|deepseek tiene un cc-home (= CLAUDE_CONFIG_DIR).
#  Su "profile config" es una capa baseline:
#    - instrucciones: overlay/CLAUDE.md   -> importada via @import en cc-home/CLAUDE.md
#    - settings:      overlay/settings.overlay.json -> jq-merge sobre el global
#                                                       => cc-home/settings.json
#
#  Funciones puras: reciben ccp_home explícito; no tocan ~/.config/ccp real.
#  El "global" (base de la herencia) se pasa como <src> (default ~/.claude,
#  override CCP_CLAUDE_SRC), igual que el seeding del binario.
# ============================================================

ccp_cfg_overlay_dir()   { printf '%s/profiles/%s/overlay' "$1" "$2"; }
ccp_cfg_instr_file()    { printf '%s/profiles/%s/overlay/CLAUDE.md' "$1" "$2"; }
ccp_cfg_settings_file() { printf '%s/profiles/%s/overlay/settings.overlay.json' "$1" "$2"; }
ccp_cfg_cchome()        { printf '%s/profiles/%s/cc-home' "$1" "$2"; }

# crea overlay/ con archivos vacíos si faltan (idempotente).
ccp_cfg_init_overlay() { # home name
  local d; d="$(ccp_cfg_overlay_dir "$1" "$2")"
  mkdir -p "$d"
  [[ -e "$d/CLAUDE.md" ]] || : > "$d/CLAUDE.md"
  [[ -e "$d/settings.overlay.json" ]] || printf '{}\n' > "$d/settings.overlay.json"
}
