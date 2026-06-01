#!/usr/bin/env bash
# ============================================================
#  lib/paths.sh — motor de reglas de paths para ccp
#
#  Resolución para un path P:
#    - Entre las reglas cuyo path es P o ancestro de P, gana la MÁS
#      ESPECÍFICA (ruta más profunda). Paths únicos => sin empates.
#    - Sin regla aplicable -> "default".
#
#  Formato de rules.tsv (CCP_RULES_FILE), una regla por línea:
#       /ruta/absoluta<TAB>nombre_de_perfil
# ============================================================

# --- normalizar: ~ inicial, a absoluto, colapsa . y .. textualmente ---
ccp_norm_path() {
  local p="$1"
  [[ -z "$p" ]] && return 1
  [[ "$p" == "~"* ]] && p="${p/#\~/$HOME}"
  [[ "$p" != /* ]] && p="$(pwd)/$p"
  local out=() part
  local IFS='/'
  for part in $p; do
    case "$part" in
      ''|.) continue ;;
      ..)   [[ ${#out[@]} -gt 0 ]] && unset 'out[${#out[@]}-1]' ;;
      *)    out+=("$part") ;;
    esac
  done
  local joined=""
  for part in "${out[@]}"; do joined="$joined/$part"; done
  printf '%s' "${joined:-/}"
}

# --- ¿base es ancestro-o-igual de path? ---
ccp_is_ancestor() {
  local base="$1" path="$2"
  [[ "$base" == "/" ]] && return 0
  [[ "$path" == "$base" ]] && return 0
  [[ "$path" == "$base"/* ]] && return 0
  return 1
}

# --- profundidad (segmentos); raíz=0 ---
ccp_depth() {
  local p="$1" d=0 part
  local IFS='/'
  for part in $p; do [[ -n "$part" ]] && d=$((d+1)); done
  printf '%d' "$d"
}

# --- resolver: imprime el nombre de perfil para un path ---
#  uso: ccp_resolve <path> <rules_file>   (imprime perfil; "default" si nada aplica)
ccp_resolve() {
  local query rules_file
  query="$(ccp_norm_path "$1")" || { printf 'default'; return; }
  rules_file="$2"
  [[ -f "$rules_file" ]] || { printf 'default'; return; }

  local best_depth=-1 best_profile="default"
  local path profile depth
  while IFS=$'\t' read -r path profile; do
    [[ -z "$path" || "$path" == \#* ]] && continue
    [[ -z "$profile" ]] && continue
    if ccp_is_ancestor "$path" "$query"; then
      depth="$(ccp_depth "$path")"
      if (( depth > best_depth )); then
        best_depth="$depth"; best_profile="$profile"
      fi
    fi
  done < "$rules_file"
  printf '%s' "$best_profile"
}

# --- set/replace regla: path -> profile (una por path) ---
#  uso: ccp_rule_set <path> <profile> <rules_file>
ccp_rule_set() {
  local path profile="$2" rules_file="$3"
  path="$(ccp_norm_path "$1")" || return 1
  mkdir -p "$(dirname "$rules_file")"; touch "$rules_file"
  local tmp; tmp="$(mktemp)"
  awk -F'\t' -v p="$path" '$1 != p' "$rules_file" > "$tmp" && mv "$tmp" "$rules_file"
  printf '%s\t%s\n' "$path" "$profile" >> "$rules_file"
}

# --- borrar la regla de un path exacto ---
ccp_rule_del() {
  local path rules_file="$2"
  path="$(ccp_norm_path "$1")" || return 1
  [[ -f "$rules_file" ]] || return 0
  local tmp; tmp="$(mktemp)"
  awk -F'\t' -v p="$path" '$1 != p' "$rules_file" > "$tmp" && mv "$tmp" "$rules_file"
}
