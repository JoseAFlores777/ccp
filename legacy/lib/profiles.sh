#!/usr/bin/env bash
# ============================================================
#  lib/profiles.sh — almacenamiento de perfiles de ccp.
#
#  Layout (raíz = ccp_home, normalmente ~/.config/ccp):
#    <ccp_home>/profiles/<name>/meta        (key=value, NO se sourcea)
#    <ccp_home>/profiles/<name>/api_key     (600, solo provider)
#    <ccp_home>/profiles/<name>/cc-home/    (solo oficial = CLAUDE_CONFIG_DIR)
#    <ccp_home>/profiles.tsv                (índice name<TAB>type)
#
#  El perfil reservado "default" NUNCA se guarda en disco.
# ============================================================

_ccp_profiles_dir() { printf '%s/profiles' "$1"; }
_ccp_profile_dir()  { printf '%s/profiles/%s' "$1" "$2"; }

ccp_profile_exists() { # ccp_home name
  [[ -f "$(_ccp_profile_dir "$1" "$2")/meta" ]]
}

ccp_profile_type() { # ccp_home name
  ccp_profile_get "$1" "$2" type
}

# lee un campo del meta (parse, no source). Imprime todo lo tras el primer '='.
ccp_profile_get() { # ccp_home name key
  local meta; meta="$(_ccp_profile_dir "$1" "$2")/meta"
  [[ -f "$meta" ]] || return 1
  awk -F= -v k="$3" '$1==k{sub(/^[^=]*=/,""); print; found=1; exit} END{exit !found}' "$meta"
}

# añade entrada al índice profiles.tsv (reemplaza si ya existe)
_ccp_index_set() { # ccp_home name type
  local idx="$1/profiles.tsv"; mkdir -p "$1"; touch "$idx"
  local tmp; tmp="$(mktemp)"
  awk -F'\t' -v n="$2" '$1 != n' "$idx" > "$tmp" && mv "$tmp" "$idx"
  printf '%s\t%s\n' "$2" "$3" >> "$idx"
}

ccp_profile_add_official() { # ccp_home name
  local d; d="$(_ccp_profile_dir "$1" "$2")"
  mkdir -p "$d/cc-home"
  printf 'type=official\n' > "$d/meta"
  _ccp_index_set "$1" "$2" official
}

ccp_profile_add_deepseek() { # ccp_home name base_url model_pro model_flash effort
  local d; d="$(_ccp_profile_dir "$1" "$2")"
  mkdir -p "$d"
  {
    printf 'type=deepseek\n'
    printf 'base_url=%s\n'    "$3"
    printf 'model_pro=%s\n'   "$4"
    printf 'model_flash=%s\n' "$5"
    printf 'effort=%s\n'      "$6"
  } > "$d/meta"
  _ccp_index_set "$1" "$2" deepseek
}

ccp_profile_set_key() { # ccp_home name key
  local d; d="$(_ccp_profile_dir "$1" "$2")"; mkdir -p "$d"
  printf '%s' "$3" > "$d/api_key"; chmod 600 "$d/api_key"
}

ccp_profile_get_key() { # ccp_home name
  local f; f="$(_ccp_profile_dir "$1" "$2")/api_key"
  [[ -f "$f" ]] || return 1
  cat "$f"
}

ccp_profile_list() { # ccp_home   (un nombre por línea)
  local dir; dir="$(_ccp_profiles_dir "$1")"
  [[ -d "$dir" ]] || return 0
  local p
  for p in "$dir"/*/; do
    [[ -f "$p/meta" ]] || continue
    basename "$p"
  done
}

ccp_profile_rm() { # ccp_home name
  rm -rf "$(_ccp_profile_dir "$1" "$2")"
  local idx="$1/profiles.tsv"
  [[ -f "$idx" ]] || return 0
  local tmp; tmp="$(mktemp)"
  awk -F'\t' -v n="$2" '$1 != n' "$idx" > "$tmp" && mv "$tmp" "$idx"
}
