#!/usr/bin/env bash
# ============================================================
#  lib/paths.sh — motor de reglas de paths para dsctl
#
#  Modelo:
#    - INCLUDE: bajo este path (y sus subcarpetas) DeepSeek se enciende.
#    - EXCLUDE: bajo este path NUNCA se enciende, aunque esté dentro
#               de un include.
#
#  Resolución para un path P:
#    1. Se buscan todas las reglas (include/exclude) que sean P o
#       ancestros de P (prefijo de directorio).
#    2. Gana la regla MÁS ESPECÍFICA (la de ruta más larga / más
#       profunda). Empate exacto: exclude gana sobre include.
#    3. Si ninguna regla aplica -> "official" (Claude Code normal).
#
#  Formato de almacenamiento (DS_RULES_FILE), una regla por línea:
#       include<TAB>/ruta/absoluta
#       exclude<TAB>/ruta/absoluta
# ============================================================

# --- normalizar: quita slash final (salvo raíz), resuelve a absoluto ---
ds_norm_path() {
  local p="$1"
  [[ -z "$p" ]] && return 1
  # expandir ~ inicial
  [[ "$p" == "~"* ]] && p="${p/#\~/$HOME}"
  # a absoluto si es relativo
  [[ "$p" != /* ]] && p="$(pwd)/$p"
  # colapsar // y resolver . y .. de forma textual y robusta
  # (no usamos realpath para permitir paths que aún no existen)
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

# --- ¿base es ancestro-o-igual de path? (comparación de segmentos) ---
ds_is_ancestor() {
  local base="$1" path="$2"
  [[ "$base" == "/" ]] && return 0
  [[ "$path" == "$base" ]] && return 0
  [[ "$path" == "$base"/* ]] && return 0
  return 1
}

# --- profundidad de un path (número de segmentos); raíz=0 ---
ds_depth() {
  local p="$1" d=0 part
  local IFS='/'
  for part in $p; do [[ -n "$part" ]] && d=$((d+1)); done
  printf '%d' "$d"
}

# --- resolver veredicto para un path dado, leyendo de un archivo de reglas ---
#  uso: ds_resolve <path> <rules_file>
#  imprime: deepseek | official
ds_resolve() {
  local query rules_file
  query="$(ds_norm_path "$1")" || { printf 'official'; return; }
  rules_file="$2"
  [[ -f "$rules_file" ]] || { printf 'official'; return; }

  local best_depth=-1 best_kind="official"
  local kind path depth
  while IFS=$'\t' read -r kind path; do
    [[ -z "$kind" || "$kind" == \#* ]] && continue
    [[ -z "$path" ]] && continue
    if ds_is_ancestor "$path" "$query"; then
      depth="$(ds_depth "$path")"
      if (( depth > best_depth )); then
        best_depth="$depth"; best_kind="$kind"
      elif (( depth == best_depth )); then
        # empate de especificidad: exclude gana
        [[ "$kind" == "exclude" ]] && best_kind="exclude"
      fi
    fi
  done < "$rules_file"

  case "$best_kind" in
    include) printf 'deepseek' ;;
    *)       printf 'official' ;;
  esac
}

# --- añadir una regla (kind=include|exclude) evitando duplicados ---
#  uso: ds_rule_add <kind> <path> <rules_file>
ds_rule_add() {
  local kind="$1" path rules_file="$3"
  path="$(ds_norm_path "$2")" || return 1
  mkdir -p "$(dirname "$rules_file")"; touch "$rules_file"
  # quitar cualquier regla previa para EXACTAMENTE este path (de cualquier tipo)
  local tmp; tmp="$(mktemp)"
  awk -F'\t' -v p="$path" '$2 != p' "$rules_file" > "$tmp" && mv "$tmp" "$rules_file"
  printf '%s\t%s\n' "$kind" "$path" >> "$rules_file"
}

# --- quitar la regla de un path exacto ---
ds_rule_del() {
  local path rules_file="$2"
  path="$(ds_norm_path "$1")" || return 1
  [[ -f "$rules_file" ]] || return 0
  local tmp; tmp="$(mktemp)"
  awk -F'\t' -v p="$path" '$2 != p' "$rules_file" > "$tmp" && mv "$tmp" "$rules_file"
}
