#!/usr/bin/env bash
# ============================================================
#  lib/instruct.sh — destino + CRUD para 'ccp instruct'
#  (respaldo de /ccp:remember-* , /ccp:recall , /ccp:forget).
#
#  Funciones puras: reciben ccp_home / src / repo_root explícitos.
#  No tocan ~/.config/ccp ni ~/.claude reales (tests usan temp dirs).
#
#  6 tipos de artefacto -> estructura OFICIAL de Claude Code:
#    rule    -> CLAUDE.md (línea dentro de un bloque con marcadores)
#    agent   -> agents/<slug>.md       (archivo, lo escribe Claude)
#    command -> commands/<slug>.md     (archivo, lo escribe Claude)
#    skill   -> skills/<slug>/         (dir, lo crea Claude)
#    hook    -> settings.json .hooks   (entrada JSON, jq-merge)
#    mcp     -> settings/.mcp.json .mcpServers (entrada JSON)
#
#  Scope: global (~/.claude) | profile (overlay) | project (repo/.claude).
#  En profile solo rule/hook/mcp (agents/commands/skills van symlinkeados
#  desde global; ver docs/adr/0005).
# ============================================================

CCP_INSTR_BEGIN='<!-- >>> ccp instructions >>> -->'
CCP_INSTR_END='<!-- <<< ccp instructions <<< -->'

# ccp_instruct_dest <scope> <type> <ccp_home> <profile> <src> <repo_root>
# Imprime la ruta destino (archivo o dir). rc:
#   0 ok | 1 scope/type desconocido
#   2 profile scope con perfil 'default'/vacío
#   3 tipo no soportado en profile (agent/command/skill)
#   4 project scope sin repo_root
ccp_instruct_dest() {
  local scope="$1" type="$2" home="$3" prof="$4" src="$5" root="$6"
  case "$scope" in
    global)
      case "$type" in
        rule)     printf '%s/CLAUDE.md' "$src" ;;
        hook|mcp) printf '%s/settings.json' "$src" ;;
        agent)    printf '%s/agents' "$src" ;;
        command)  printf '%s/commands' "$src" ;;
        skill)    printf '%s/skills' "$src" ;;
        *) return 1 ;;
      esac ;;
    profile)
      [[ -n "$prof" && "$prof" != "default" ]] || return 2
      local ov="$home/profiles/$prof/overlay"
      case "$type" in
        rule)     printf '%s/CLAUDE.md' "$ov" ;;
        hook|mcp) printf '%s/settings.overlay.json' "$ov" ;;
        agent|command|skill) return 3 ;;
        *) return 1 ;;
      esac ;;
    project)
      [[ -n "$root" ]] || return 4
      case "$type" in
        rule)    printf '%s/.claude/CLAUDE.md' "$root" ;;
        hook)    printf '%s/.claude/settings.json' "$root" ;;
        mcp)     printf '%s/.mcp.json' "$root" ;;
        agent)   printf '%s/.claude/agents' "$root" ;;
        command) printf '%s/.claude/commands' "$root" ;;
        skill)   printf '%s/.claude/skills' "$root" ;;
        *) return 1 ;;
      esac ;;
    *) return 1 ;;
  esac
}

# ---- bloque de reglas (tipo 'rule') --------------------------------------

# asegura que <file> existe y contiene el bloque (idempotente).
ccp_instruct_block_ensure() { # file
  local f="$1"
  mkdir -p "$(dirname "$f")"
  [[ -e "$f" ]] || : > "$f"
  if ! grep -qF "$CCP_INSTR_BEGIN" "$f"; then
    { [[ -s "$f" ]] && printf '\n'; printf '%s\n%s\n' "$CCP_INSTR_BEGIN" "$CCP_INSTR_END"; } >> "$f"
  fi
}

# lista las instrucciones (texto sin '- '), una por línea, en orden.
ccp_instruct_rule_list() { # file
  local f="$1"
  [[ -f "$f" ]] || return 0
  awk -v b="$CCP_INSTR_BEGIN" -v e="$CCP_INSTR_END" '
    $0 == b { inb=1; next }
    $0 == e { inb=0 }
    inb && /^- / { line=$0; sub(/^- /, "", line); print line }
  ' "$f"
}

# agrega una instrucción (bullet) si no es duplicado exacto. rc 0 añadió, 9 dup.
ccp_instruct_rule_add() { # file text
  local f="$1" text="$2"
  ccp_instruct_block_ensure "$f"
  if ccp_instruct_rule_list "$f" | grep -qxF "$text"; then return 9; fi
  local tmp; tmp="$(mktemp)"
  _CCP_LINE="- $text" awk -v end="$CCP_INSTR_END" '
    BEGIN { line=ENVIRON["_CCP_LINE"] }
    $0 == end { print line }
    { print }
  ' "$f" > "$tmp" && mv "$tmp" "$f"
}

# borra la instrucción N (1-based) dentro del bloque. rc 0 ok, 1 fuera de rango.
ccp_instruct_rule_rm() { # file index
  local f="$1" idx="$2"
  [[ -f "$f" ]] || return 1
  local n; n="$(ccp_instruct_rule_list "$f" | grep -c .)"
  [[ "$idx" =~ ^[0-9]+$ && "$idx" -ge 1 && "$idx" -le "$n" ]] || return 1
  local tmp; tmp="$(mktemp)"
  awk -v b="$CCP_INSTR_BEGIN" -v e="$CCP_INSTR_END" -v target="$idx" '
    $0 == b { inb=1; print; next }
    $0 == e { inb=0; print; next }
    inb && /^- / { c++; if (c == target) next; print; next }
    { print }
  ' "$f" > "$tmp" && mv "$tmp" "$f"
}

# ---- manifest de artefactos creados por ccp ------------------------------
# formato por fila: scope<TAB>profile<TAB>type<TAB>ref<TAB>desc
#   profile = '-' salvo scope=profile
#   ref     = ruta de archivo (agent/command/skill) o nombre de entrada (mcp/hook)

ccp_instruct_manifest_file() { # scope ccp_home repo_root
  case "$1" in
    global|profile) printf '%s/authored.tsv' "$2" ;;
    project)        printf '%s/.claude/ccp-authored.tsv' "$3" ;;
    *) return 1 ;;
  esac
}

ccp_instruct_manifest_add() { # manifest scope profile type ref desc
  local m="$1"; shift
  mkdir -p "$(dirname "$m")"; touch "$m"
  printf '%s\t%s\t%s\t%s\t%s\n' "$1" "$2" "$3" "$4" "$5" >> "$m"
}

# imprime "type<TAB>ref<TAB>desc" de las filas que matchean (scope[,profile]).
ccp_instruct_manifest_list() { # manifest scope profile
  local m="$1" scope="$2" prof="$3"
  [[ -f "$m" ]] || return 0
  awk -F'\t' -v s="$scope" -v p="$prof" '
    $1==s && (s!="profile" || $2==p) { printf "%s\t%s\t%s\n", $3, $4, $5 }
  ' "$m"
}

# borra la fila N (1-based, mismo orden que _list) e imprime "type<TAB>ref".
ccp_instruct_manifest_rm() { # manifest scope profile index
  local m="$1" scope="$2" prof="$3" idx="$4"
  [[ -f "$m" ]] || return 1
  [[ "$idx" =~ ^[0-9]+$ && "$idx" -ge 1 ]] || return 1
  # encuentra el número de línea físico de la N-ésima fila que matchea.
  local target; target="$(awk -F'\t' -v s="$scope" -v p="$prof" -v want="$idx" '
    $1==s && (s!="profile" || $2==p) { c++; if (c==want) { print NR; exit } }
  ' "$m")"
  [[ -n "$target" ]] || return 1
  sed -n "${target}p" "$m" | cut -f3,4
  local tmp; tmp="$(mktemp)"
  awk -v t="$target" 'NR!=t' "$m" > "$tmp" && mv "$tmp" "$m"
}
