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

# valida JSON (rc 0 = válido). Sin jq => no-op (rc 0): no podemos validar.
ccp_cfg_validate_json() { # file
  [[ -f "$1" ]] || return 1
  command -v jq >/dev/null 2>&1 || return 0
  jq -e . "$1" >/dev/null 2>&1 || return 1
}

# merge global ⊕ overlay => out (overlay gana).
#   con jq:  deep-merge recursivo de objetos; arrays se reemplazan.
#   sin jq:  snapshot — copia el global si existe, si no el overlay.
# global ausente o inválido => se ignora (solo overlay).
ccp_cfg_merge_settings() { # global_file overlay_file out_file
  local g="$1" o="$2" out="$3"
  if command -v jq >/dev/null 2>&1; then
    local -a files=()
    [[ -f "$g" ]] && jq -e . "$g" >/dev/null 2>&1 && files+=("$g")
    files+=("$o")
    jq -s 'reduce .[] as $x ({}; . * $x)' "${files[@]}" > "$out" || return 1
  else
    if [[ -f "$g" ]]; then cp "$g" "$out"; else cp "$o" "$out"; fi
  fi
}

# escribe cc-home/CLAUDE.md: header + @import del global (si existe) + @import del overlay.
# IMPORTANTE: si cc-home/CLAUDE.md es un symlink viejo, se elimina antes de escribir
# (escribir sobre un symlink corrompería el archivo global apuntado).
ccp_cfg_write_claude_md() { # home name src
  local home="$1" name="$2" src="$3"
  local cch overlay
  cch="$(ccp_cfg_cchome "$home" "$name")"
  overlay="$(ccp_cfg_instr_file "$home" "$name")"
  mkdir -p "$cch"
  [[ -L "$cch/CLAUDE.md" ]] && rm -f "$cch/CLAUDE.md"
  {
    printf '# %s — generado por ccp (no editar a mano; usa: ccp profile config %s)\n\n' "$name" "$name"
    [[ -f "$src/CLAUDE.md" ]] && printf '@%s\n' "$src/CLAUDE.md"
    printf '@%s\n' "$overlay"
  } > "$cch/CLAUDE.md"
}

# regenera el cc-home efectivo desde global ⊕ overlay (idempotente).
ccp_cfg_regenerate() { # home name src
  local home="$1" name="$2" src="$3"
  ccp_cfg_init_overlay "$home" "$name"
  local cch overlay_s
  cch="$(ccp_cfg_cchome "$home" "$name")"
  overlay_s="$(ccp_cfg_settings_file "$home" "$name")"
  mkdir -p "$cch"
  ccp_cfg_write_claude_md "$home" "$name" "$src"
  ccp_cfg_merge_settings "$src/settings.json" "$overlay_s" "$cch/settings.json"
}
