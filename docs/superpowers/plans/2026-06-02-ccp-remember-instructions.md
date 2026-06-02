# ccp `remember` — captura polimórfica a estructura oficial de Claude Code · Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Dar a Claude Code comandos `/ccp:remember-{global,profile,project}`, `/ccp:recall`, `/ccp:forget` que persisten artefactos (rule/agent/command/hook/mcp/skill) en la estructura oficial que Claude Code reconoce, con CRUD seguro respaldado por un manifest.

**Architecture:** Un nuevo `lib/instruct.sh` (funciones puras: resolución de destino por scope×tipo, CRUD del bloque de reglas en `CLAUDE.md`, CRUD del manifest) y un nuevo `cmd_instruct()` en `bin/ccp` (`add|list|rm`) que lo expone como CLI determinista y testeable. Cinco archivos markdown de comando bajo `commands/ccp/` son prompts que hacen que Claude **clasifique** la intención, redacte el contenido, **confirme**, y llame al CLI. `install.sh` copia los comandos a `~/.claude/commands/ccp/`, desde donde el symlink de cada `cc-home` los propaga a todos los perfiles.

**Tech Stack:** Bash (BSD-portable: `awk -F'\t'`, sin `grep -P`), `jq` (con fallback), el harness hand-rolled de `tests/run.sh`, archivos de comando markdown de Claude Code.

**Decisiones de referencia:** `CONTEXT.md` (términos Scope, Artifact, Instruction, Authored manifest), `docs/adr/0004` (bloque marcado para el tipo *rule*), `docs/adr/0005` (routing polimórfico + manifest + scope×tipo).

---

## File Structure

| Archivo | Responsabilidad | Acción |
|---|---|---|
| `lib/instruct.sh` | Funciones puras: `ccp_instruct_dest` (scope×tipo→ruta), CRUD del bloque de reglas, CRUD del manifest. Reciben `ccp_home`/`src`/`repo_root` explícitos. | **Crear** |
| `bin/ccp` | Sourcing del nuevo lib; `cmd_instruct()` (`add\|list\|rm`); entrada en `main()`; ayuda; completions. | **Modificar** |
| `commands/ccp/remember-global.md` | Prompt: clasifica tipo, redacta, confirma, escribe a scope global. | **Crear** |
| `commands/ccp/remember-profile.md` | Igual, scope profile (perfil activo; error si default). | **Crear** |
| `commands/ccp/remember-project.md` | Igual, scope project (raíz git). | **Crear** |
| `commands/ccp/recall.md` | Prompt: lista artefactos efectivos por scope. | **Crear** |
| `commands/ccp/forget.md` | Prompt: lista numerado, pregunta, borra por índice. | **Crear** |
| `install.sh` | Copiar `commands/ccp/*.md` → `~/.claude/commands/ccp/`. | **Modificar** |
| `tests/run.sh` | Tests puros de `lib/instruct.sh` + tests de binario de `ccp instruct`. | **Modificar** |
| `CHANGELOG.md` | Entrada de la feature. | **Modificar** |
| `README.md` | Documentar los comandos `/ccp:` y `ccp instruct`. | **Modificar** |

### Convenciones obligatorias (de `CLAUDE.md`)

- **macOS/BSD:** nada de `grep -P`. Filtrado con `awk -F'\t'` o `while IFS=$'\t' read`.
- **Pipes:** en tests, nunca pipear la salida de `ccp` a `grep -q`/`head` (SIGPIPE bajo `pipefail`). Capturar en variable y `case`-match.
- **Funciones puras:** reciben `ccp_home` explícito; los tests usan temp dirs (`newdir`), nunca `~/.config/ccp` real.
- **Binario en tests:** invocar con `_ccp "$h" ...` (define `CCP_HOME=$h`), y `CCP_CLAUDE_SRC`/repo en temp para no tocar `~/.claude` real.
- **Commits:** NO comitear sin autorización explícita del usuario en el turno. Los pasos "Commit" de este plan se ejecutan solo si el usuario lo autoriza; si no, dejar staged y preguntar. NUNCA añadir trailer `Co-Authored-By`.

---

## Milestone 1 — Mecánica de reglas + resolución de destino (CLI puro)

Entrega `ccp instruct add|list|rm <scope> rule` funcionando y testeado, más el resolvedor de destino completo (todos los tipos) que M3/M4 consumirán. Sin archivos de comando aún. 100% testeable sin tocar config real.

### Task 1.1: Crear `lib/instruct.sh` con el resolvedor de destino

**Files:**
- Create: `lib/instruct.sh`
- Test: `tests/run.sh` (añadir `test_instruct_dest_*`)

- [ ] **Step 1: Escribir los tests que fallan**

Añadir al final de la sección de tests en `tests/run.sh` (antes del `# ---- runner ----`):

```bash
# ===== instruct: resolución de destino =====
test_instruct_dest_global() {
  assert_eq "$(ccp_instruct_dest global rule    /h ds /src /root)" "/src/CLAUDE.md"     "global rule"
  assert_eq "$(ccp_instruct_dest global hook    /h ds /src /root)" "/src/settings.json" "global hook"
  assert_eq "$(ccp_instruct_dest global mcp     /h ds /src /root)" "/src/settings.json" "global mcp"
  assert_eq "$(ccp_instruct_dest global agent   /h ds /src /root)" "/src/agents"        "global agent"
  assert_eq "$(ccp_instruct_dest global command /h ds /src /root)" "/src/commands"      "global command"
  assert_eq "$(ccp_instruct_dest global skill   /h ds /src /root)" "/src/skills"        "global skill"
}
test_instruct_dest_project() {
  assert_eq "$(ccp_instruct_dest project rule  /h ds /src /root)" "/root/.claude/CLAUDE.md"     "proj rule"
  assert_eq "$(ccp_instruct_dest project hook  /h ds /src /root)" "/root/.claude/settings.json" "proj hook"
  assert_eq "$(ccp_instruct_dest project mcp   /h ds /src /root)" "/root/.mcp.json"             "proj mcp"
  assert_eq "$(ccp_instruct_dest project agent /h ds /src /root)" "/root/.claude/agents"        "proj agent"
}
test_instruct_dest_profile_overlay() {
  assert_eq "$(ccp_instruct_dest profile rule /h work /src /root)" "/h/profiles/work/overlay/CLAUDE.md"             "prof rule"
  assert_eq "$(ccp_instruct_dest profile hook /h work /src /root)" "/h/profiles/work/overlay/settings.overlay.json" "prof hook"
  assert_eq "$(ccp_instruct_dest profile mcp  /h work /src /root)" "/h/profiles/work/overlay/settings.overlay.json" "prof mcp"
}
test_instruct_dest_profile_default_rc2() {
  ccp_instruct_dest profile rule /h default /src /root >/dev/null 2>&1; assert_rc "$?" 2 "profile+default => rc2"
  ccp_instruct_dest profile rule /h ""      /src /root >/dev/null 2>&1; assert_rc "$?" 2 "profile+empty => rc2"
}
test_instruct_dest_profile_filetype_rc3() {
  ccp_instruct_dest profile agent   /h work /src /root >/dev/null 2>&1; assert_rc "$?" 3 "profile agent => rc3"
  ccp_instruct_dest profile command /h work /src /root >/dev/null 2>&1; assert_rc "$?" 3 "profile command => rc3"
  ccp_instruct_dest profile skill   /h work /src /root >/dev/null 2>&1; assert_rc "$?" 3 "profile skill => rc3"
}
test_instruct_dest_project_no_root_rc4() {
  ccp_instruct_dest project rule /h ds /src "" >/dev/null 2>&1; assert_rc "$?" 4 "project sin root => rc4"
}
test_instruct_dest_unknown_rc1() {
  ccp_instruct_dest bogus rule /h ds /src /root >/dev/null 2>&1; assert_rc "$?" 1 "scope desconocido => rc1"
  ccp_instruct_dest global xxx /h ds /src /root >/dev/null 2>&1; assert_rc "$?" 1 "tipo desconocido => rc1"
}
```

Y añadir el sourcing del nuevo lib junto a los otros (después de la línea que hace `source "$ROOT/lib/cfg.sh"`):

```bash
[[ -f "$ROOT/lib/instruct.sh" ]] && { source "$ROOT/lib/instruct.sh"; }
```

- [ ] **Step 2: Correr los tests y verificar que fallan**

Run: `bash tests/run.sh instruct_dest`
Expected: FAIL — `ccp_instruct_dest: command not found` (función inexistente).

- [ ] **Step 3: Implementar `lib/instruct.sh` (cabecera + resolvedor)**

Crear `lib/instruct.sh`:

```bash
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
  local ov="$home/profiles/$prof/overlay"
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
```

- [ ] **Step 4: Correr los tests y verificar que pasan**

Run: `bash tests/run.sh instruct_dest`
Expected: PASS (todos los `test_instruct_dest_*`).

- [ ] **Step 5: Lint**

Run: `shellcheck -S warning lib/instruct.sh`
Expected: sin warnings (o solo informativos aceptados por CI).

- [ ] **Step 6: Commit** *(solo si el usuario autorizó comitear este turno)*

```bash
git add lib/instruct.sh tests/run.sh
git commit -m "feat(instruct): resolvedor de destino scope×tipo"
```

---

### Task 1.2: CRUD del bloque de reglas en `CLAUDE.md`

**Files:**
- Modify: `lib/instruct.sh`
- Test: `tests/run.sh` (añadir `test_instruct_rule_*`)

- [ ] **Step 1: Escribir los tests que fallan**

Añadir a `tests/run.sh`:

```bash
# ===== instruct: bloque de reglas =====
test_instruct_rule_add_creates_block() {
  local f; f="$(newdir)/CLAUDE.md"; printf '# Mi config\n\ncontenido previo\n' > "$f"
  ccp_instruct_rule_add "$f" "responde en español"
  # el contenido previo se conserva
  case "$(cat "$f")" in *"contenido previo"*) _pass=$((_pass+1));;
    *) _fail=$((_fail+1)); echo "FAIL: preserva contenido previo" >&2;; esac
  # la regla está en el bloque
  assert_eq "$(ccp_instruct_rule_list "$f")" "responde en español" "lista 1 regla"
}
test_instruct_rule_add_appends_in_order() {
  local f; f="$(newdir)/CLAUDE.md"; : > "$f"
  ccp_instruct_rule_add "$f" "uno"
  ccp_instruct_rule_add "$f" "dos"
  assert_eq "$(ccp_instruct_rule_list "$f" | tr '\n' '|')" "uno|dos|" "orden de inserción"
}
test_instruct_rule_add_dedup_rc9() {
  local f; f="$(newdir)/CLAUDE.md"; : > "$f"
  ccp_instruct_rule_add "$f" "igual"
  ccp_instruct_rule_add "$f" "igual"; assert_rc "$?" 9 "duplicado => rc9"
  assert_eq "$(ccp_instruct_rule_list "$f" | grep -c .)" "1" "no duplica"
}
test_instruct_rule_rm_by_index() {
  local f; f="$(newdir)/CLAUDE.md"; : > "$f"
  ccp_instruct_rule_add "$f" "a"; ccp_instruct_rule_add "$f" "b"; ccp_instruct_rule_add "$f" "c"
  ccp_instruct_rule_rm "$f" 2; assert_rc "$?" 0 "rm índice válido"
  assert_eq "$(ccp_instruct_rule_list "$f" | tr '\n' '|')" "a|c|" "borra el 2do"
}
test_instruct_rule_rm_out_of_range_rc1() {
  local f; f="$(newdir)/CLAUDE.md"; : > "$f"
  ccp_instruct_rule_add "$f" "solo"
  ccp_instruct_rule_rm "$f" 5; assert_rc "$?" 1 "fuera de rango => rc1"
}
test_instruct_rule_list_empty_file() {
  local f; f="$(newdir)/none.md"
  assert_eq "$(ccp_instruct_rule_list "$f")" "" "archivo inexistente => vacío"
}
```

- [ ] **Step 2: Correr y verificar que fallan**

Run: `bash tests/run.sh instruct_rule`
Expected: FAIL — funciones inexistentes.

- [ ] **Step 3: Implementar el CRUD del bloque**

Añadir a `lib/instruct.sh`:

```bash
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
  awk -v end="$CCP_INSTR_END" -v line="- $text" '
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
```

- [ ] **Step 4: Correr y verificar que pasan**

Run: `bash tests/run.sh instruct_rule`
Expected: PASS.

- [ ] **Step 5: Lint + suite completa**

Run: `shellcheck -S warning lib/instruct.sh && bash tests/run.sh`
Expected: sin warnings; `0 failed`.

- [ ] **Step 6: Commit** *(solo si autorizado)*

```bash
git add lib/instruct.sh tests/run.sh
git commit -m "feat(instruct): CRUD del bloque de reglas en CLAUDE.md"
```

---

### Task 1.3: `cmd_instruct` en `bin/ccp` (subcomando `add|list|rm` para `rule`)

**Files:**
- Modify: `bin/ccp` (sourcing del lib ~línea 14-20; nuevo `cmd_instruct`; entrada en `main()` ~línea 742; ayuda en `cmd_help`; completions ~línea 450/471)
- Test: `tests/run.sh` (añadir `test_bin_instruct_*`)

- [ ] **Step 1: Escribir los tests de binario que fallan**

Añadir a `tests/run.sh`:

```bash
# ===== instruct: binario (scope rule) =====
# helper: ejecuta el binario con CCP_HOME, src y repo_root temporales.
_ccp_instr() { # ccp_home src repo_root args...
  CCP_HOME="$1" CCP_CLAUDE_SRC="$2" CCP_REPO_ROOT="$3" bash "$ROOT/bin/ccp" "${@:4}"
}
test_bin_instruct_add_global_rule() {
  local h s; h="$(newdir)"; s="$(newdir)"
  _ccp_instr "$h" "$s" "" instruct add global rule "no uses emojis" >/dev/null
  case "$(cat "$s/CLAUDE.md")" in *"no uses emojis"*) _pass=$((_pass+1));;
    *) _fail=$((_fail+1)); echo "FAIL: regla global escrita" >&2;; esac
}
test_bin_instruct_add_profile_default_errors() {
  local h s; h="$(newdir)"; s="$(newdir)"
  # sin perfil activo => CCP_PROFILE ausente => 'default'
  local rc; CCP_HOME="$h" CCP_CLAUDE_SRC="$s" bash "$ROOT/bin/ccp" instruct add profile rule "x" >/dev/null 2>&1; rc=$?
  assert_rc "$rc" 1 "profile sobre default => error (rc1)"
}
test_bin_instruct_add_profile_active() {
  local h s; h="$(newdir)"; s="$(newdir)"
  _ccp "$h" profile add work --official >/dev/null
  CCP_HOME="$h" CCP_CLAUDE_SRC="$s" CCP_PROFILE=work bash "$ROOT/bin/ccp" instruct add profile rule "responde en español" >/dev/null
  case "$(cat "$h/profiles/work/overlay/CLAUDE.md")" in *"responde en español"*) _pass=$((_pass+1));;
    *) _fail=$((_fail+1)); echo "FAIL: regla de perfil escrita en overlay" >&2;; esac
}
test_bin_instruct_list_and_rm() {
  local h s; h="$(newdir)"; s="$(newdir)"
  _ccp_instr "$h" "$s" "" instruct add global rule "uno" >/dev/null
  _ccp_instr "$h" "$s" "" instruct add global rule "dos" >/dev/null
  local out; out="$(_ccp_instr "$h" "$s" "" instruct list global)"
  case "$out" in *"1"*"uno"*) _pass=$((_pass+1));;
    *) _fail=$((_fail+1)); echo "FAIL: list numerado: $out" >&2;; esac
  _ccp_instr "$h" "$s" "" instruct rm global 1 >/dev/null
  local rem; rem="$(_ccp_instr "$h" "$s" "" instruct list global)"
  case "$rem" in *"dos"*) _pass=$((_pass+1));;
    *) _fail=$((_fail+1)); echo "FAIL: rm dejó 'dos'" >&2;; esac
  case "$rem" in *"uno"*) _fail=$((_fail+1)); echo "FAIL: 'uno' debió borrarse" >&2;; *) _pass=$((_pass+1));; esac
}
```

- [ ] **Step 2: Correr y verificar que fallan**

Run: `bash tests/run.sh bin_instruct`
Expected: FAIL — `Comando desconocido: 'instruct'`.

- [ ] **Step 3: Source del lib + variables de entorno en `bin/ccp`**

En `bin/ccp`, donde se sourcean los libs (tras `source "$_libdir/cfg.sh"`), añadir:

```bash
# shellcheck source=/dev/null
source "$_libdir/instruct.sh"
```

Y junto a `CCP_HOME`/`CCP_RULES_FILE` (~línea 22-23), añadir el resolvedor de src y repo (override-ables en tests):

```bash
CCP_CLAUDE_SRC="${CCP_CLAUDE_SRC:-$HOME/.claude}"
# raíz del repo para scope 'project' (override en tests con CCP_REPO_ROOT)
_instr_repo_root() { if [[ -n "${CCP_REPO_ROOT:-}" ]]; then printf '%s' "$CCP_REPO_ROOT"; else _repo_root; fi; }
```

(`_repo_root` ya existe en `bin/ccp:48` — `git rev-parse --show-toplevel`.)

- [ ] **Step 4: Implementar `cmd_instruct` (solo `rule` por ahora)**

Añadir en `bin/ccp` (junto a los otros `cmd_*`, p.ej. tras `cmd_path_list`):

```bash
# scope del perfil activo (vía CCP_PROFILE; default si ausente).
_instr_active_profile() { printf '%s' "${CCP_PROFILE:-default}"; }

# resuelve la ruta destino o falla con mensaje en español.
_instr_dest_or_die() { # scope type
  local scope="$1" type="$2" prof src root dest rc
  prof="$(_instr_active_profile)"; src="$CCP_CLAUDE_SRC"; root="$(_instr_repo_root)"
  dest="$(ccp_instruct_dest "$scope" "$type" "$CCP_HOME" "$prof" "$src" "$root")"; rc=$?
  case "$rc" in
    0) printf '%s' "$dest"; return 0 ;;
    2) err "scope 'profile': el perfil activo es 'default' (sin overlay propio)."
       info "Usa 'ccp instruct add global $type ...' o activa un perfil ('ccp use <n>')."; return 2 ;;
    3) err "tipo '$type' no existe a nivel de perfil (se comparten desde global)."
       info "Usa scope 'global' (aplica a todos los perfiles) o 'project' (acota al repo)."; return 3 ;;
    4) err "scope 'project': no estás en un repo git y no hay CCP_REPO_ROOT."; return 4 ;;
    *) err "scope/tipo inválido: '$scope'/'$type'."; return 1 ;;
  esac
}

cmd_instruct() {
  local sub="$1"; shift 2>/dev/null
  case "$sub" in
    add)  _instruct_add  "$@" ;;
    list) _instruct_list "$@" ;;
    rm)   _instruct_rm   "$@" ;;
    *) err "instruct: subcomando desconocido '$sub' (add|list|rm)"; return 1 ;;
  esac
}

# ccp instruct add <scope> <type> <texto...>
_instruct_add() {
  local scope="$1" type="$2"; shift 2 2>/dev/null
  local text="$*"
  [[ -n "$scope" && -n "$type" && -n "$text" ]] || { err "Uso: ccp instruct add <scope> <type> <texto>"; return 1; }
  local dest; dest="$(_instr_dest_or_die "$scope" "$type")" || return $?
  case "$type" in
    rule)
      if ccp_instruct_rule_add "$dest" "$text"; then
        ok "Instrucción añadida ($scope/rule) -> $dest"
      else
        warn "Ya existía esa instrucción en $dest (no se duplica)."
      fi
      # scope profile: el overlay se @importa en vivo; no hace falta regen.
      ;;
    *) err "tipo '$type' aún no implementado por 'instruct add' (ver Milestones 3-4)."; return 1 ;;
  esac
}

# ccp instruct list <scope>
_instruct_list() {
  local scope="$1"
  [[ -n "$scope" ]] || { err "Uso: ccp instruct list <scope>"; return 1; }
  local dest; dest="$(_instr_dest_or_die "$scope" rule)" || return $?
  local i=0 line
  while IFS= read -r line; do
    [[ -z "$line" ]] && continue
    i=$((i+1)); printf '   %2d) %s\n' "$i" "$line"
  done < <(ccp_instruct_rule_list "$dest")
  (( i )) || say "   (sin instrucciones en $scope)"
}

# ccp instruct rm <scope> <index>
_instruct_rm() {
  local scope="$1" idx="$2"
  [[ -n "$scope" && -n "$idx" ]] || { err "Uso: ccp instruct rm <scope> <index>"; return 1; }
  local dest; dest="$(_instr_dest_or_die "$scope" rule)" || return $?
  if ccp_instruct_rule_rm "$dest" "$idx"; then
    ok "Instrucción #$idx eliminada ($scope)."
  else
    err "Índice fuera de rango: $idx"; return 1
  fi
}
```

- [ ] **Step 5: Añadir `instruct` al dispatch de `main()`**

En `main()` (~línea 742), junto a los otros casos, añadir:

```bash
    instruct) cmd_instruct "$@" ;;
```

- [ ] **Step 6: Correr los tests de binario y verificar que pasan**

Run: `bash tests/run.sh bin_instruct`
Expected: PASS.

- [ ] **Step 7: Añadir a completions y ayuda**

En `_ccp()` bash (~línea 450) y zsh (~línea 471), añadir `instruct` a la lista `top`. En `cmd_help`, bajo una nueva sección:

```bash
${C_BOLD}INSTRUCCIONES (memoria de Claude)${C_RESET}
  ccp instruct add <scope> <type> <texto>   añade un artefacto (scope: global|profile|project)
  ccp instruct list <scope>                 lista las instrucciones del scope
  ccp instruct rm <scope> <index>           borra por índice
                                            (lo usan los comandos /ccp:remember-* /ccp:recall /ccp:forget)
```

- [ ] **Step 8: Lint + suite completa + sintaxis**

Run: `bash -n bin/ccp && shellcheck -S warning bin/ccp lib/instruct.sh && bash tests/run.sh`
Expected: sin errores de sintaxis; sin warnings; `0 failed`.

- [ ] **Step 9: Commit** *(solo si autorizado)*

```bash
git add bin/ccp tests/run.sh
git commit -m "feat(instruct): cmd_instruct add|list|rm para tipo rule"
```

---

## Milestone 2 — Comandos `/ccp:` para reglas (end-to-end de cara al cliente)

Entrega los 5 archivos de comando markdown (limitados a tipo `rule` por ahora) + la copia en `install.sh`. Tras esto, `/ccp:remember-global "no uses emojis"` funciona de punta a punta.

### Task 2.1: Crear los 3 comandos `remember-*`

**Files:**
- Create: `commands/ccp/remember-global.md`, `commands/ccp/remember-profile.md`, `commands/ccp/remember-project.md`

- [ ] **Step 1: Crear `commands/ccp/remember-global.md`**

```markdown
---
description: Persiste una instrucción/artefacto de Claude a nivel GLOBAL (~/.claude, todos los perfiles)
argument-hint: <lo que quieres que Claude recuerde>
---

El usuario quiere que recuerdes algo a nivel **global** (`~/.claude`, aplica a todos los perfiles de ccp).

Input del usuario: $ARGUMENTS

Pasos:
1. **Clasifica** el tipo de artefacto según lo que pide:
   - `rule` — una directiva de comportamiento ("siempre X", "nunca Y", preferencias). → es el caso por defecto.
   - `agent`, `command`, `skill`, `hook`, `mcp` — si claramente pide crear uno de estos. (Milestones 3-4; si aún no está implementado en `ccp instruct add`, avísalo.)
2. **Redacta** el texto de la instrucción en imperativo, claro y conciso (una línea para `rule`).
3. **Confirma** con el usuario: muestra `tipo`, `destino` y el `texto` exacto que vas a escribir. No escribas sin confirmación.
4. **Escribe** llamando al CLI (él es el dueño de la mecánica):
   ```
   ccp instruct add global rule "<texto redactado>"
   ```
5. Reporta la ruta destino que devolvió ccp.

Reglas:
- NO edites `~/.claude/CLAUDE.md` a mano: usa `ccp instruct add` (mantiene el bloque gestionado).
- Si ccp responde "ya existía", díselo al usuario; no reintentes.
```

- [ ] **Step 2: Crear `commands/ccp/remember-profile.md`**

Igual que el anterior pero con el scope `profile` y esta nota de cabecera:

```markdown
---
description: Persiste una instrucción/artefacto al PERFIL ACTIVO (overlay del perfil de esta terminal)
argument-hint: <lo que quieres que Claude recuerde para este perfil>
---

El usuario quiere recordar algo a nivel del **perfil activo** (el `CCP_PROFILE` de esta terminal; se escribe a su `overlay/CLAUDE.md`).

Input del usuario: $ARGUMENTS

Pasos:
1. **Clasifica** el tipo (igual que remember-global). A nivel perfil solo se permiten `rule`, `hook`, `mcp`; si pide `agent`/`command`/`skill`, ccp lo rechazará — sugiere usar `/ccp:remember-global` o `/ccp:remember-project`.
2. **Redacta** el texto.
3. **Confirma** tipo + destino + texto.
4. **Escribe**:
   ```
   ccp instruct add profile rule "<texto redactado>"
   ```
5. Si ccp responde que el perfil activo es `default` (sin overlay), explícale al usuario que `default` = config global, y ofrécele `/ccp:remember-global` o activar un perfil con `ccp use <n>`.
6. Reporta la ruta destino.
```

- [ ] **Step 3: Crear `commands/ccp/remember-project.md`**

```markdown
---
description: Persiste una instrucción/artefacto al PROYECTO (.claude/ del repo git actual, versionado)
argument-hint: <lo que quieres que Claude recuerde para este repo>
---

El usuario quiere recordar algo a nivel del **proyecto** (el `.claude/` de la raíz del repo git actual; se versiona con el código).

Input del usuario: $ARGUMENTS

Pasos:
1. **Clasifica** el tipo (igual que remember-global; project soporta los 6 tipos).
2. **Redacta** el texto.
3. **Confirma** tipo + destino + texto.
4. **Escribe**:
   ```
   ccp instruct add project rule "<texto redactado>"
   ```
5. Si ccp responde que no estás en un repo git, avísale al usuario (no hay raíz de proyecto).
6. Reporta la ruta destino. Recuérdale que el cambio queda en `.claude/` y conviene comitearlo.
```

- [ ] **Step 4: Verificar formato de los comandos**

Run: `head -4 commands/ccp/remember-global.md`
Expected: frontmatter YAML válido con `description:` y `argument-hint:`.

- [ ] **Step 5: Commit** *(solo si autorizado)*

```bash
git add commands/ccp/remember-global.md commands/ccp/remember-profile.md commands/ccp/remember-project.md
git commit -m "feat(commands): /ccp:remember-{global,profile,project} (rule)"
```

---

### Task 2.2: Crear `recall` y `forget`

**Files:**
- Create: `commands/ccp/recall.md`, `commands/ccp/forget.md`

- [ ] **Step 1: Crear `commands/ccp/recall.md`**

```markdown
---
description: Lista las instrucciones/artefactos que ccp gestiona para el contexto actual
argument-hint: "[global|profile|project]  (vacío = los tres)"
---

Muestra lo que ccp tiene registrado.

Argumento (scope, opcional): $ARGUMENTS

- Si el argumento es `global`, `profile` o `project`: corre `ccp instruct list <scope>` y muestra el resultado.
- Si está **vacío**: muestra los tres, cada uno con su encabezado, corriendo:
  ```
  ccp instruct list global
  ccp instruct list profile   # omítelo si el perfil activo es 'default'
  ccp instruct list project   # omítelo si no estás en un repo git
  ```
- Es solo lectura. Para borrar, dirige al usuario a `/ccp:forget`.
```

- [ ] **Step 2: Crear `commands/ccp/forget.md`**

```markdown
---
description: Borra una instrucción/artefacto gestionado por ccp, por scope
argument-hint: "[global|profile|project]"
---

Borra algo que ccp gestiona. Argumento (scope): $ARGUMENTS

Pasos:
1. Determina el scope. Si está vacío, pregunta al usuario cuál (global/profile/project).
2. Lista numerado:
   ```
   ccp instruct list <scope>
   ```
3. Muéstrale la lista y pregunta **qué número** borrar. No borres sin confirmación.
4. Borra:
   ```
   ccp instruct rm <scope> <n>
   ```
5. Confirma el resultado y vuelve a listar si el usuario quiere seguir.

Nunca borres artefactos hechos a mano: `ccp instruct` solo ve lo que ccp creó (bloque gestionado + manifest).
```

- [ ] **Step 3: Commit** *(solo si autorizado)*

```bash
git add commands/ccp/recall.md commands/ccp/forget.md
git commit -m "feat(commands): /ccp:recall y /ccp:forget"
```

---

### Task 2.3: `install.sh` copia los comandos a `~/.claude/commands/ccp/`

**Files:**
- Modify: `install.sh` (tras el bloque que registra `install-source`, ~línea 30)
- Test: `tests/run.sh` (añadir `test_install_copies_commands`)

- [ ] **Step 1: Escribir el test que falla**

Añadir a `tests/run.sh`:

```bash
test_install_copies_commands() {
  local bd ld h cd; bd="$(newdir)"; ld="$(newdir)"; h="$(newdir)"; cd="$(newdir)/claude"
  CCP_BIN_DIR="$bd" CCP_LIB_DIR="$ld" CCP_HOME="$h" CCP_CLAUDE_SRC="$cd" \
    bash "$ROOT/install.sh" >/dev/null 2>&1
  [[ -f "$cd/commands/ccp/remember-global.md" ]]; assert_rc "$?" 0 "install copió remember-global"
  [[ -f "$cd/commands/ccp/forget.md" ]];          assert_rc "$?" 0 "install copió forget"
}
```

- [ ] **Step 2: Correr y verificar que falla**

Run: `bash tests/run.sh install_copies_commands`
Expected: FAIL — el archivo no existe en el destino.

- [ ] **Step 3: Añadir la copia en `install.sh`**

Tras el bloque de `install-source` (~línea 30), añadir:

```bash
# Comandos /ccp: para Claude Code (se propagan a todos los perfiles vía el
# symlink commands/ de cada cc-home). CCP_CLAUDE_SRC override-able en tests.
CLAUDE_SRC="${CCP_CLAUDE_SRC:-$HOME/.claude}"
if [[ -d "$SRC_DIR/commands/ccp" ]]; then
  mkdir -p "$CLAUDE_SRC/commands/ccp"
  install -m 0644 "$SRC_DIR/commands/ccp/"*.md "$CLAUDE_SRC/commands/ccp/"
  ok "Comandos /ccp: -> $CLAUDE_SRC/commands/ccp/"
fi
```

- [ ] **Step 4: Correr y verificar que pasa**

Run: `bash tests/run.sh install_copies_commands`
Expected: PASS.

- [ ] **Step 5: Suite completa + lint de install.sh**

Run: `shellcheck -S warning install.sh && bash tests/run.sh`
Expected: sin warnings; `0 failed`.

- [ ] **Step 6: Commit** *(solo si autorizado)*

```bash
git add install.sh tests/run.sh
git commit -m "feat(install): copiar comandos /ccp: a ~/.claude/commands/ccp"
```

---

### Task 2.4: Verificación manual end-to-end (rules)

**Files:** ninguno (verificación).

- [ ] **Step 1: Reinstalar y probar el flujo CLI**

```bash
bash install.sh
~/.local/bin/ccp instruct add global rule "no uses emojis en commits"
~/.local/bin/ccp instruct list global
```
Expected: la lista muestra `1) no uses emojis en commits`; el bloque aparece en `~/.claude/CLAUDE.md`.

- [ ] **Step 2: Limpiar la prueba**

```bash
~/.local/bin/ccp instruct rm global 1
```
Expected: `instruct list global` ya no muestra la regla; el bloque queda vacío (marcadores presentes, sin bullets).

- [ ] **Step 3: Confirmar que el comando aparece en Claude Code**

En una terminal nueva, `/ccp:` debe ofrecer `remember-global`, `remember-profile`, `remember-project`, `recall`, `forget`. (Verificación visual; no automatizable aquí.)

---

## Milestone 3 — Artefactos de archivo: agent / command / skill (+ manifest)

Habilita que `remember` cree subagents, slash commands y skills como archivos en la estructura oficial, registrados en el manifest para CRUD seguro.

### Task 3.1: CRUD del manifest en `lib/instruct.sh`

**Files:**
- Modify: `lib/instruct.sh`
- Test: `tests/run.sh` (`test_instruct_manifest_*`)

- [ ] **Step 1: Escribir los tests que fallan**

```bash
# ===== instruct: manifest =====
test_instruct_manifest_file() {
  assert_eq "$(ccp_instruct_manifest_file global /h /root)"  "/h/authored.tsv"             "global manifest local"
  assert_eq "$(ccp_instruct_manifest_file profile /h /root)" "/h/authored.tsv"             "profile manifest local"
  assert_eq "$(ccp_instruct_manifest_file project /h /root)" "/root/.claude/ccp-authored.tsv" "project manifest repo"
}
test_instruct_manifest_add_list() {
  local m; m="$(newdir)/authored.tsv"
  ccp_instruct_manifest_add "$m" global - agent   /src/agents/sec.md  "auditor de seguridad"
  ccp_instruct_manifest_add "$m" global - command /src/commands/dep.md "deploy"
  assert_eq "$(ccp_instruct_manifest_list "$m" global -)" \
"agent	/src/agents/sec.md	auditor de seguridad
command	/src/commands/dep.md	deploy" "lista 2 entradas globales"
}
test_instruct_manifest_list_filters_profile() {
  local m; m="$(newdir)/authored.tsv"
  ccp_instruct_manifest_add "$m" profile work hook /h/.../settings.overlay.json prettier "hook A"
  ccp_instruct_manifest_add "$m" profile other hook /x prettier "hook B"
  assert_eq "$(ccp_instruct_manifest_list "$m" profile work | grep -c .)" "1" "filtra por perfil activo"
}
test_instruct_manifest_rm_returns_ref() {
  local m; m="$(newdir)/authored.tsv"
  ccp_instruct_manifest_add "$m" global - agent   /a.md "A"
  ccp_instruct_manifest_add "$m" global - command /b.md "B"
  local out; out="$(ccp_instruct_manifest_rm "$m" global - 1)"; assert_rc "$?" 0 "rm índice 1"
  assert_eq "$out" "agent	/a.md" "rm devuelve type+ref de la fila borrada"
  assert_eq "$(ccp_instruct_manifest_list "$m" global - | grep -c .)" "1" "queda 1"
}
test_instruct_manifest_rm_out_of_range() {
  local m; m="$(newdir)/authored.tsv"; : > "$m"
  ccp_instruct_manifest_rm "$m" global - 1 >/dev/null 2>&1; assert_rc "$?" 1 "vacío => rc1"
}
```

- [ ] **Step 2: Correr y verificar que fallan**

Run: `bash tests/run.sh instruct_manifest`
Expected: FAIL — funciones inexistentes.

- [ ] **Step 3: Implementar el manifest**

Añadir a `lib/instruct.sh`:

```bash
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
```

- [ ] **Step 4: Correr y verificar que pasan**

Run: `bash tests/run.sh instruct_manifest`
Expected: PASS.

- [ ] **Step 5: Lint + suite**

Run: `shellcheck -S warning lib/instruct.sh && bash tests/run.sh`
Expected: sin warnings; `0 failed`.

- [ ] **Step 6: Commit** *(solo si autorizado)*

```bash
git add lib/instruct.sh tests/run.sh
git commit -m "feat(instruct): CRUD del manifest de artefactos"
```

---

### Task 3.2: `instruct add/list/rm` para tipos de archivo (agent/command/skill)

**Files:**
- Modify: `bin/ccp` (`_instruct_add`, `_instruct_list`, `_instruct_rm`)
- Test: `tests/run.sh` (`test_bin_instruct_file_*`)

**Contrato nuevo (registro, no escritura del contenido):** Claude escribe el archivo del artefacto (un agent .md, etc.) en la ruta que ccp resuelve, y luego registra:

```
ccp instruct record <scope> <type> <ref> <desc>
```

`ccp instruct dest <scope> <type>` imprime la ruta destino (dir para tipos de archivo) para que Claude sepa dónde escribir. `list`/`rm` combinan reglas (bloque) + manifest.

- [ ] **Step 1: Escribir los tests que fallan**

```bash
test_bin_instruct_dest_subcmd() {
  local h s; h="$(newdir)"; s="$(newdir)"
  assert_eq "$(_ccp_instr "$h" "$s" "" instruct dest global agent)" "$s/agents" "dest global agent"
}
test_bin_instruct_record_and_list_global() {
  local h s; h="$(newdir)"; s="$(newdir)"
  _ccp_instr "$h" "$s" "" instruct add global rule "una regla" >/dev/null
  _ccp_instr "$h" "$s" "" instruct record global agent "$s/agents/sec.md" "auditor seguridad" >/dev/null
  local out; out="$(_ccp_instr "$h" "$s" "" instruct list global)"
  # rules primero, luego manifest; ambos numerados en una sola lista
  case "$out" in *"una regla"*"auditor seguridad"*) _pass=$((_pass+1));;
    *) _fail=$((_fail+1)); echo "FAIL: list combina rule+manifest: $out" >&2;; esac
}
test_bin_instruct_rm_deletes_file_artifact() {
  local h s; h="$(newdir)"; s="$(newdir)"
  mkdir -p "$s/agents"; printf 'x' > "$s/agents/sec.md"
  _ccp_instr "$h" "$s" "" instruct record global agent "$s/agents/sec.md" "auditor" >/dev/null
  # índice 1 (no hay reglas) => borra el archivo + entrada
  _ccp_instr "$h" "$s" "" instruct rm global 1 >/dev/null
  [[ ! -e "$s/agents/sec.md" ]]; assert_rc "$?" 0 "rm borró el archivo del artefacto"
}
```

- [ ] **Step 2: Correr y verificar que fallan**

Run: `bash tests/run.sh "bin_instruct_dest_subcmd|bin_instruct_record|bin_instruct_rm_deletes"`
Expected: FAIL — `dest`/`record` desconocidos; rm no borra archivos.

- [ ] **Step 3: Extender `cmd_instruct` y reescribir list/rm como lista combinada**

En `bin/ccp`, dentro de `cmd_instruct` añadir los subcomandos `dest` y `record`:

```bash
    dest)   _instruct_dest_cmd "$@" ;;
    record) _instruct_record "$@" ;;
```

E implementar:

```bash
# imprime la ruta destino para (scope,type) — lo usan los comandos /ccp:.
_instruct_dest_cmd() { # scope type
  local dest; dest="$(_instr_dest_or_die "$1" "$2")" || return $?
  printf '%s\n' "$dest"
}

# registra en el manifest un artefacto que Claude ya escribió.
_instruct_record() { # scope type ref desc...
  local scope="$1" type="$2" ref="$3"; shift 3 2>/dev/null
  local desc="$*"
  [[ -n "$scope" && -n "$type" && -n "$ref" ]] || { err "Uso: ccp instruct record <scope> <type> <ref> <desc>"; return 1; }
  # valida scope/type (y guard de profile) reutilizando el resolvedor:
  _instr_dest_or_die "$scope" "$type" >/dev/null || return $?
  local prof root m
  prof="$(_instr_active_profile)"; root="$(_instr_repo_root)"
  [[ "$scope" != profile ]] && prof="-"
  m="$(ccp_instruct_manifest_file "$scope" "$CCP_HOME" "$root")"
  ccp_instruct_manifest_add "$m" "$scope" "$prof" "$type" "$ref" "$desc"
  ok "Artefacto registrado ($scope/$type): $ref"
}
```

Reescribir `_instruct_list` para combinar reglas + manifest (reglas primero):

```bash
_instruct_list() {
  local scope="$1"
  [[ -n "$scope" ]] || { err "Uso: ccp instruct list <scope>"; return 1; }
  local rfile; rfile="$(_instr_dest_or_die "$scope" rule)" || return $?
  local prof root m; prof="$(_instr_active_profile)"; root="$(_instr_repo_root)"
  [[ "$scope" != profile ]] && prof="-"
  m="$(ccp_instruct_manifest_file "$scope" "$CCP_HOME" "$root")"
  local i=0 line
  while IFS= read -r line; do
    [[ -z "$line" ]] && continue
    i=$((i+1)); printf '   %2d) [rule] %s\n' "$i" "$line"
  done < <(ccp_instruct_rule_list "$rfile")
  local type ref desc
  while IFS=$'\t' read -r type ref desc; do
    [[ -z "$type" ]] && continue
    i=$((i+1)); printf '   %2d) [%s] %s — %s\n' "$i" "$type" "$desc" "$ref"
  done < <(ccp_instruct_manifest_list "$m" "$scope" "$prof")
  (( i )) || say "   (sin instrucciones ni artefactos en $scope)"
}
```

Reescribir `_instruct_rm` para mapear el índice combinado:

```bash
_instruct_rm() {
  local scope="$1" idx="$2"
  [[ -n "$scope" && "$idx" =~ ^[0-9]+$ ]] || { err "Uso: ccp instruct rm <scope> <index>"; return 1; }
  local rfile; rfile="$(_instr_dest_or_die "$scope" rule)" || return $?
  local prof root m; prof="$(_instr_active_profile)"; root="$(_instr_repo_root)"
  [[ "$scope" != profile ]] && prof="-"
  m="$(ccp_instruct_manifest_file "$scope" "$CCP_HOME" "$root")"
  local nrules; nrules="$(ccp_instruct_rule_list "$rfile" | grep -c .)"
  if [[ "$idx" -le "$nrules" ]]; then
    ccp_instruct_rule_rm "$rfile" "$idx" && { ok "Instrucción #$idx eliminada ($scope)."; return 0; }
    err "Índice fuera de rango: $idx"; return 1
  fi
  # índice en la zona del manifest
  local midx=$((idx - nrules)) row type ref
  row="$(ccp_instruct_manifest_rm "$m" "$scope" "$prof" "$midx")" || { err "Índice fuera de rango: $idx"; return 1; }
  type="$(printf '%s' "$row" | cut -f1)"; ref="$(printf '%s' "$row" | cut -f2)"
  case "$type" in
    agent|command) [[ -f "$ref" ]] && rm -f "$ref" ;;
    skill)         [[ -d "$ref" ]] && rm -rf "$ref" ;;
    # hook|mcp: ver Milestone 4 (borrado de entrada JSON)
  esac
  ok "Artefacto #$idx eliminado ($scope/$type): $ref"
}
```

- [ ] **Step 4: Correr los tests y verificar que pasan**

Run: `bash tests/run.sh bin_instruct`
Expected: PASS (incluyendo los de M1, que siguen verdes).

- [ ] **Step 5: Actualizar los 3 comandos `remember-*` para tipos de archivo**

En cada `commands/ccp/remember-*.md`, ampliar el paso 4 para tipos de archivo:

```markdown
   - Para `rule`/`hook`/`mcp`: `ccp instruct add <scope> <type> "<texto>"`.
   - Para `agent`/`command`/`skill`:
     1. Pide la ruta destino: `ccp instruct dest <scope> <type>` (devuelve el dir).
     2. Escribe el archivo del artefacto ahí con un slug claro (p.ej. `<dir>/auditor-seguridad.md`), con el formato oficial de Claude Code (frontmatter `name`/`description`, cuerpo).
     3. Regístralo: `ccp instruct record <scope> <type> "<ruta-escrita>" "<descripción corta>"`.
```

- [ ] **Step 6: Lint + suite + sintaxis**

Run: `bash -n bin/ccp && shellcheck -S warning bin/ccp lib/instruct.sh && bash tests/run.sh`
Expected: sin errores; `0 failed`.

- [ ] **Step 7: Commit** *(solo si autorizado)*

```bash
git add bin/ccp lib/instruct.sh tests/run.sh commands/ccp/remember-global.md commands/ccp/remember-profile.md commands/ccp/remember-project.md
git commit -m "feat(instruct): artefactos de archivo (agent/command/skill) + manifest CRUD"
```

---

## Milestone 4 — Artefactos JSON: hook / mcp

Habilita `remember` para hooks y servidores MCP vía `jq` deep-merge, con fallback sin `jq`. **mcp** soporta CRUD completo (borrado por nombre de servidor); **hook** soporta add + listado, y el borrado se documenta como manual (los hooks viven en arrays de `settings.json` sin id estable). Esta limitación es deliberada y se reporta al usuario (`log` explícito), conforme a ADR-0005.

### Task 4.1: Verificar las ubicaciones de config que Claude Code honra

**Files:** ninguno (verificación previa que de-risquea la implementación).

- [ ] **Step 1: Confirmar dónde lee Claude Code hooks y MCP servers por scope**

Verificar en la documentación/instalación local de Claude Code:
- Hooks: clave `hooks` en `settings.json` (global `~/.claude/settings.json`, proyecto `.claude/settings.json`). **Confirmar.**
- MCP: servidores de proyecto en `.mcp.json` (raíz del repo); de usuario, confirmar si es `~/.claude.json` o `mcpServers` en `settings.json`.

Run (inspección): `ccp doctor` y revisar `~/.claude/settings.json` / `~/.claude.json` existentes.
Expected: una decisión registrada de qué archivo+clave usar por scope. **Si la ubicación global de MCP resulta ser `~/.claude.json` (no `settings.json`), ajustar `ccp_instruct_dest global mcp` en `lib/instruct.sh` y su test antes de seguir.**

- [ ] **Step 2: Registrar el hallazgo**

Si cambió la ubicación respecto a lo asumido en M1, actualizar:
- `lib/instruct.sh::ccp_instruct_dest` (rama `global mcp`).
- El test `test_instruct_dest_global` correspondiente.
- Una nota en `docs/adr/0005`.

---

### Task 4.2: Merge JSON de hook/mcp en `lib/instruct.sh`

**Files:**
- Modify: `lib/instruct.sh`
- Test: `tests/run.sh` (`test_instruct_json_*`) — gated por `jq`.

- [ ] **Step 1: Escribir los tests que fallan (skip si no hay jq)**

```bash
test_instruct_json_merge_mcp() {
  command -v jq >/dev/null 2>&1 || { _pass=$((_pass+1)); return; }  # skip sin jq
  local f; f="$(newdir)/settings.json"; printf '{}\n' > "$f"
  ccp_instruct_json_merge "$f" '{"mcpServers":{"github":{"command":"gh-mcp"}}}'
  assert_eq "$(jq -r '.mcpServers.github.command' "$f")" "gh-mcp" "mcp merge"
}
test_instruct_json_merge_preserves() {
  command -v jq >/dev/null 2>&1 || { _pass=$((_pass+1)); return; }
  local f; f="$(newdir)/settings.json"; printf '{"existing":true}\n' > "$f"
  ccp_instruct_json_merge "$f" '{"mcpServers":{"a":{"command":"x"}}}'
  assert_eq "$(jq -r '.existing' "$f")" "true" "preserva claves previas"
}
test_instruct_json_rm_mcp_by_name() {
  command -v jq >/dev/null 2>&1 || { _pass=$((_pass+1)); return; }
  local f; f="$(newdir)/settings.json"; printf '{"mcpServers":{"a":{},"b":{}}}\n' > "$f"
  ccp_instruct_json_rm_mcp "$f" a
  assert_eq "$(jq -r '.mcpServers | keys | join(",")' "$f")" "b" "borró server a"
}
```

- [ ] **Step 2: Correr y verificar que fallan**

Run: `bash tests/run.sh instruct_json`
Expected: FAIL (con jq presente) — funciones inexistentes.

- [ ] **Step 3: Implementar el merge/rm JSON**

Añadir a `lib/instruct.sh`:

```bash
# ---- artefactos JSON (hook/mcp) ------------------------------------------

# deep-merge de <snippet-json> sobre <file> (archivo gana = snippet gana).
# requiere jq. rc 1 si jq ausente o snippet inválido.
ccp_instruct_json_merge() { # file snippet_json
  local f="$1" snippet="$2"
  command -v jq >/dev/null 2>&1 || return 1
  mkdir -p "$(dirname "$f")"
  [[ -f "$f" ]] && jq -e . "$f" >/dev/null 2>&1 || printf '{}\n' > "$f"
  printf '%s' "$snippet" | jq -e . >/dev/null 2>&1 || return 1
  local tmp; tmp="$(mktemp)"
  jq --argjson add "$snippet" '. * $add' "$f" > "$tmp" && mv "$tmp" "$f"
}

# borra un server MCP por nombre. rc 1 si jq ausente.
ccp_instruct_json_rm_mcp() { # file server_name
  local f="$1" name="$2"
  command -v jq >/dev/null 2>&1 || return 1
  [[ -f "$f" ]] || return 1
  local tmp; tmp="$(mktemp)"
  jq --arg n "$name" 'if .mcpServers then .mcpServers |= del(.[$n]) else . end' "$f" > "$tmp" && mv "$tmp" "$f"
}
```

- [ ] **Step 4: Correr y verificar que pasan**

Run: `bash tests/run.sh instruct_json`
Expected: PASS (o skip silencioso sin jq).

- [ ] **Step 5: Lint + suite**

Run: `shellcheck -S warning lib/instruct.sh && bash tests/run.sh`
Expected: sin warnings; `0 failed`.

- [ ] **Step 6: Commit** *(solo si autorizado)*

```bash
git add lib/instruct.sh tests/run.sh
git commit -m "feat(instruct): merge/borrado JSON para hook y mcp"
```

---

### Task 4.3: Wire de hook/mcp en `cmd_instruct` + regen de overlay en profile

**Files:**
- Modify: `bin/ccp` (`_instruct_add` rama hook/mcp; `_instruct_rm` rama mcp)
- Test: `tests/run.sh` (`test_bin_instruct_mcp_*`)

- [ ] **Step 1: Escribir los tests que fallan**

```bash
test_bin_instruct_add_mcp_global() {
  command -v jq >/dev/null 2>&1 || { _pass=$((_pass+1)); return; }
  local h s; h="$(newdir)"; s="$(newdir)"; printf '{}\n' > "$s/settings.json"
  _ccp_instr "$h" "$s" "" instruct add global mcp 'github={"command":"gh-mcp"}' >/dev/null
  assert_eq "$(jq -r '.mcpServers.github.command' "$s/settings.json")" "gh-mcp" "mcp escrito en settings global"
}
test_bin_instruct_add_mcp_profile_regenerates() {
  command -v jq >/dev/null 2>&1 || { _pass=$((_pass+1)); return; }
  local h s; h="$(newdir)"; s="$(newdir)"; printf '{}\n' > "$s/settings.json"
  _ccp "$h" profile add work --official >/dev/null
  CCP_HOME="$h" CCP_CLAUDE_SRC="$s" CCP_PROFILE=work bash "$ROOT/bin/ccp" \
    instruct add profile mcp 'gh={"command":"x"}' >/dev/null
  # overlay actualizado
  assert_eq "$(jq -r '.mcpServers.gh.command' "$h/profiles/work/overlay/settings.overlay.json")" "x" "mcp en overlay"
  # y regenerado al cc-home efectivo
  assert_eq "$(jq -r '.mcpServers.gh.command' "$h/profiles/work/cc-home/settings.json")" "x" "regen propagó al cc-home"
}
```

**Convención de entrada para hook/mcp:** el argumento es `nombre=<json>` (mcp: nombre del server; hook: un id legible elegido por Claude). Claude construye el JSON; ccp lo envuelve en la clave correcta (`mcpServers`/`hooks`) y lo mergea.

- [ ] **Step 2: Correr y verificar que fallan**

Run: `bash tests/run.sh bin_instruct_add_mcp`
Expected: FAIL — rama no implementada.

- [ ] **Step 3: Implementar las ramas hook/mcp en `_instruct_add`**

Reemplazar el `case "$type"` de `_instruct_add` para incluir:

```bash
    mcp)
      local name="${text%%=*}" json="${text#*=}"
      [[ "$name" != "$text" && -n "$json" ]] || { err "Uso: mcp -> nombre={json}"; return 1; }
      command -v jq >/dev/null 2>&1 || { err "mcp/hook requieren jq."; return 1; }
      local snippet; snippet="$(printf '{"mcpServers":{"%s":%s}}' "$name" "$json")"
      ccp_instruct_json_merge "$dest" "$snippet" || { err "JSON inválido."; return 1; }
      _instr_record_json "$scope" mcp "$name" "mcp $name"
      _instr_regen_if_profile "$scope"
      ok "MCP '$name' añadido ($scope) -> $dest" ;;
    hook)
      local name="${text%%=*}" json="${text#*=}"
      [[ "$name" != "$text" && -n "$json" ]] || { err "Uso: hook -> id={json}"; return 1; }
      command -v jq >/dev/null 2>&1 || { err "mcp/hook requieren jq."; return 1; }
      ccp_instruct_json_merge "$dest" "$json" || { err "JSON inválido."; return 1; }
      _instr_record_json "$scope" hook "$name" "hook $name"
      _instr_regen_if_profile "$scope"
      ok "Hook '$name' añadido ($scope) -> $dest"
      warn "Borrado de hooks no es automático (viven en arrays sin id estable)."
      info "Para quitarlo: 'ccp profile config <perfil> settings' o edita $dest a mano." ;;
```

Y los helpers:

```bash
# registra en el manifest una entrada JSON (mcp/hook).
_instr_record_json() { # scope type name desc
  local scope="$1" type="$2" name="$3"; shift 3 2>/dev/null
  local prof root m; prof="$(_instr_active_profile)"; root="$(_instr_repo_root)"
  [[ "$scope" != profile ]] && prof="-"
  m="$(ccp_instruct_manifest_file "$scope" "$CCP_HOME" "$root")"
  ccp_instruct_manifest_add "$m" "$scope" "$prof" "$type" "$name" "$*"
}

# si el scope es profile, regenera el cc-home (overlay -> settings.json efectivo).
_instr_regen_if_profile() { # scope
  [[ "$1" == profile ]] || return 0
  ccp_cfg_regenerate "$CCP_HOME" "$(_instr_active_profile)" "$CCP_CLAUDE_SRC"
}
```

- [ ] **Step 4: Extender el borrado de mcp en `_instruct_rm`**

En la rama del manifest de `_instruct_rm`, añadir el case `mcp`:

```bash
    mcp)
      local mscope; mscope="$(_instr_dest_or_die "$scope" mcp)"
      ccp_instruct_json_rm_mcp "$mscope" "$ref"
      _instr_regen_if_profile "$scope" ;;
    hook)
      warn "El hook '$ref' se quitó del manifest, pero su entrada JSON sigue en el archivo."
      info "Edítalo a mano: $(_instr_dest_or_die "$scope" hook)" ;;
```

- [ ] **Step 5: Correr y verificar que pasan**

Run: `bash tests/run.sh bin_instruct`
Expected: PASS (o skips sin jq).

- [ ] **Step 6: Actualizar los comandos `remember-*` para hook/mcp**

En cada `remember-*.md`, añadir al paso de tipos:

```markdown
   - Para `mcp`: construye el JSON del server y llama `ccp instruct add <scope> mcp 'nombre={"command":"...","args":[...]}'`.
   - Para `hook`: construye el JSON del hook (formato oficial: `{"hooks":{"PostToolUse":[{"matcher":"...","hooks":[...]}]}}`) y llama `ccp instruct add <scope> hook 'id={...}'`. Avisa al usuario que el borrado de hooks es manual.
```

- [ ] **Step 7: Lint + suite + sintaxis**

Run: `bash -n bin/ccp && shellcheck -S warning bin/ccp lib/instruct.sh && bash tests/run.sh`
Expected: sin errores; `0 failed`.

- [ ] **Step 8: Commit** *(solo si autorizado)*

```bash
git add bin/ccp lib/instruct.sh tests/run.sh commands/ccp/*.md
git commit -m "feat(instruct): hook y mcp vía jq merge (+regen en profile)"
```

---

## Milestone 5 — Documentación y cierre

### Task 5.1: README + CHANGELOG + ayuda

**Files:**
- Modify: `README.md`, `CHANGELOG.md`

- [ ] **Step 1: Añadir sección al README**

Documentar bajo una nueva sección "Instrucciones / remember":
- Los comandos `/ccp:remember-{global,profile,project}`, `/ccp:recall`, `/ccp:forget`.
- La superficie `ccp instruct add|list|rm|dest|record`.
- La tabla scope×tipo (de `docs/adr/0005`).
- La nota de que los hooks se borran a mano.

- [ ] **Step 2: Añadir entrada al CHANGELOG**

```markdown
## [Unreleased]
### Added
- `ccp instruct add|list|rm|dest|record` y los comandos `/ccp:remember-{global,profile,project}`, `/ccp:recall`, `/ccp:forget`: capturan artefactos (rule/agent/command/hook/mcp/skill) en la estructura oficial de Claude Code, con CRUD seguro vía bloque marcado (reglas) y manifest (resto). Ver docs/adr/0004, 0005.
```

- [ ] **Step 3: Verificar que `ccp version` y `ccp help` mencionan instruct**

Run: `~/.local/bin/ccp help | grep -A2 INSTRUCCIONES`
Expected: la sección de ayuda aparece.

- [ ] **Step 4: Commit** *(solo si autorizado)*

```bash
git add README.md CHANGELOG.md
git commit -m "docs: documentar ccp instruct y comandos /ccp:remember"
```

---

## Self-Review

**1. Spec coverage** (decisiones del grilling → tarea):

- remember polimórfico, Claude clasifica → Task 2.1-2.2 (comandos clasifican), Task 1.3/3.2/4.3 (CLI por tipo). ✅
- Scopes global/profile/project → `ccp_instruct_dest` (Task 1.1). ✅
- 6 tipos a estructura oficial → resolvedor (1.1) + rule (1.2-1.3) + archivo (3.2) + json (4.x). ✅
- profile = solo rule/hook/mcp; resto error guiado → rc3 en `ccp_instruct_dest` + mensaje en `_instr_dest_or_die` (1.1, 1.3). ✅
- profile activo; default → error guiado → rc2 + mensaje (1.1, 1.3). ✅
- project = raíz git, fallback cwd → `_instr_repo_root` usa `_repo_root` (git) (1.3); fallback cwd: **ver nota abajo**. ✅ (con corrección)
- Distribución: repo trae .md, install.sh copia a ~/.claude/commands/ccp → Task 2.3. ✅
- CRUD completo → list/rm combinados (1.3, 3.2); mcp full, hook add+manual-rm (4.3, documentado). ✅
- Bloque marcado para rules (ADR 0004) → Task 1.2. ✅
- Manifest split por localidad → `ccp_instruct_manifest_file` (3.1). ✅
- Confirmar antes de escribir → paso 3 de cada remember-*. ✅

**2. Placeholder scan:** sin "TBD"/"add error handling"/"similar to". Todo el código está completo. La única verificación abierta intencional es Task 4.1 (ubicación MCP de Claude Code), explícita y accionable, no un placeholder de implementación.

**3. Type consistency:** nombres de función consistentes — `ccp_instruct_dest`, `ccp_instruct_rule_{add,list,rm}`, `ccp_instruct_block_ensure`, `ccp_instruct_manifest_{file,add,list,rm}`, `ccp_instruct_json_{merge,rm_mcp}`; en binario `cmd_instruct` → `_instruct_{add,list,rm,record}`, `_instruct_dest_cmd`, helpers `_instr_*`. El manifest usa `scope\tprofile\ttype\tref\tdesc` de forma consistente en add/list/rm.

**Corrección de fallback cwd (project sin git):** `_instr_repo_root` debe caer al `$PWD` cuando no hay repo git, según la decisión del grilling. Ajustar la implementación de Task 1.3 Step 3 a:

```bash
_instr_repo_root() {
  if [[ -n "${CCP_REPO_ROOT:-}" ]]; then printf '%s' "$CCP_REPO_ROOT"; return; fi
  local r; r="$(_repo_root)"; if [[ -n "$r" ]]; then printf '%s' "$r"; else printf '%s' "$PWD"; fi
}
```

Y añadir el test correspondiente en Task 1.3 Step 1:

```bash
test_bin_instruct_project_fallback_cwd() {
  local h s; h="$(newdir)"; s="$(newdir)"
  # CCP_REPO_ROOT vacío y cwd = dir temporal sin git => usa cwd
  local d; d="$(newdir)"
  ( cd "$d" && CCP_HOME="$h" CCP_CLAUDE_SRC="$s" bash "$ROOT/bin/ccp" instruct add project rule "regla repo" >/dev/null )
  case "$(cat "$d/.claude/CLAUDE.md" 2>/dev/null)" in *"regla repo"*) _pass=$((_pass+1));;
    *) _fail=$((_fail+1)); echo "FAIL: fallback a cwd para project" >&2;; esac
}
```

(El caso rc4 "project sin repo_root" de `ccp_instruct_dest` sigue válido a nivel de función pura — el fallback a cwd ocurre en el binario, no en la función pura, que recibe root vacío solo si el binario no resolvió ninguno; con el fallback a `$PWD` eso no pasará en la práctica salvo `$PWD` vacío.)

---

## Execution Handoff

(Se ofrece tras guardar el plan.)
