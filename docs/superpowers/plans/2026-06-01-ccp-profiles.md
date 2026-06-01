# ccp — Per-path Profile Router Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

> **⚠️ Commit policy (overrides skill default):** This repo's owner forbids autonomous commits. The `Commit` steps below are real, but the executor MUST get explicit per-turn authorization from the user before running `git commit`, and MUST NOT add any `Co-Authored-By:` / tool-attribution trailer. If unauthorized, stage the work and report instead.

**Goal:** Rebrand `dsctl`→`ccp` and turn the binary deepseek↔official toggle into a named-profile engine that routes each directory to an official Anthropic account or a DeepSeek/provider profile, fully CLI-configurable.

**Architecture:** Pure-function libs (`lib/paths.sh` resolver, `lib/profiles.sh` storage CRUD, `lib/env.sh` env-delta emitter) are sourced by a single binary `bin/ccp`. The binary owns dispatch, migration, install, and the generated shell function + precmd hook. The shell function/hook `eval` the binary's env delta — the only place the parent shell's env is mutated. Profiles live under `~/.config/ccp/profiles/<name>/`.

**Tech Stack:** Bash (runs under `bash`, BSD coreutils on macOS — no `grep -P`), `awk -F'\t'`, a hand-rolled bash test harness (`tests/run.sh`), shellcheck + `bash -n` in CI.

---

## File Structure

| File | Responsibility | Action |
|------|----------------|--------|
| `lib/paths.sh` | Path normalization + most-specific-wins resolver returning a **profile name** | Modify (rename `ds_*`→`ccp_*`, change rule format to `path<TAB>profile`) |
| `lib/profiles.sh` | Profile storage CRUD (meta, key, cc-home dirs) — operates on an explicit `ccp_home` arg | Create |
| `lib/env.sh` | `ccp_env_delta` — emits eval-able unset+export lines for a profile | Create |
| `bin/ccp` | CLI dispatch, `profile`/`path`/`env`/`hook`/`resolve`/`status`/`config`/`migrate`/`install`/`uninstall`/`completion`/`menu`/`doctor`/`help`, generated `ccp()` shell fn + `_ccp_autocheck` hook | Create (rebrand of `bin/dsctl`) |
| `install.sh` | Copy `bin/ccp` → `~/.local/bin`, libs → `~/.local/lib/ccp` | Modify |
| `tests/run.sh` | Test harness: sources libs, runs `test_*` functions, optional name filter, non-zero exit on failure | Create |
| `.github/workflows/ci.yml` | shellcheck new files + `bash -n` + run `tests/run.sh` | Modify |
| `CLAUDE.md` | Update guidance to ccp/profiles | Modify (final task) |

**Naming conventions locked here (used across all tasks):**
- Config root variable: `CCP_HOME` (default `~/.config/ccp`).
- Profile dir: `$CCP_HOME/profiles/<name>/` containing `meta`, `api_key` (provider only, mode 600), `cc-home/` (official only).
- Rules file: `$CCP_HOME/rules.tsv`, lines `path<TAB>profile`.
- Profiles index: `$CCP_HOME/profiles.tsv`, lines `name<TAB>type`.
- Reserved profile name: `default` (never stored on disk; it is the fallback).
- `meta` file format: plain `key=value` lines, **parsed not sourced** (no code execution).
- Managed env vars (the exact set, referenced verbatim in `lib/env.sh`):
  `CLAUDE_CONFIG_DIR ANTHROPIC_BASE_URL ANTHROPIC_AUTH_TOKEN ANTHROPIC_MODEL ANTHROPIC_DEFAULT_OPUS_MODEL ANTHROPIC_DEFAULT_SONNET_MODEL ANTHROPIC_DEFAULT_HAIKU_MODEL CLAUDE_CODE_SUBAGENT_MODEL CLAUDE_CODE_EFFORT_LEVEL CCP_PROFILE`

---

## Phase 1 — Test harness + path engine

### Task 1: Test harness

**Files:**
- Create: `tests/run.sh`

- [ ] **Step 1: Write the harness**

```bash
#!/usr/bin/env bash
# tests/run.sh — harness de tests para ccp.
# Cada test es una función shell llamada test_*.
# Uso:  bash tests/run.sh            # corre todos
#       bash tests/run.sh resolve    # corre los test_* cuyo nombre contiene 'resolve'
set -uo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# shellcheck source=/dev/null
source "$ROOT/lib/paths.sh"
[[ -f "$ROOT/lib/profiles.sh" ]] && { source "$ROOT/lib/profiles.sh"; }
[[ -f "$ROOT/lib/env.sh" ]]      && { source "$ROOT/lib/env.sh"; }

_pass=0; _fail=0

assert_eq() { # got want msg
  if [[ "$1" == "$2" ]]; then _pass=$((_pass+1));
  else _fail=$((_fail+1)); printf 'FAIL: %s\n  got:  [%s]\n  want: [%s]\n' "$3" "$1" "$2" >&2; fi
}
assert_rc() { # rc want msg
  if [[ "$1" == "$2" ]]; then _pass=$((_pass+1));
  else _fail=$((_fail+1)); printf 'FAIL: %s (rc got=%s want=%s)\n' "$3" "$1" "$2" >&2; fi
}

# mktemp dir helper que se autolimpia al salir
TMPROOT="$(mktemp -d)"
trap 'rm -rf "$TMPROOT"' EXIT
newdir() { local d; d="$(mktemp -d "$TMPROOT/XXXXXX")"; printf '%s' "$d"; }

# ---- los test_* se definen abajo o en archivos sourced ----
# (este archivo crece tarea a tarea)

# ---- runner ----
_filter="${1:-}"
_tests="$(declare -F | awk '{print $3}' | grep '^test_' | { [[ -n "$_filter" ]] && grep -- "$_filter" || cat; } | sort)"
for fn in $_tests; do "$fn"; done
printf '\n%s%d passed, %d failed%s\n' "" "$_pass" "$_fail" ""
[[ "$_fail" -eq 0 ]]
```

- [ ] **Step 2: Run the empty harness**

Run: `bash tests/run.sh`
Expected: prints `0 passed, 0 failed` and exits 0 (no `test_*` yet; `lib/paths.sh` still has old `ds_*` funcs — that's fine, harness just sources it).

- [ ] **Step 3: Commit** *(authorization required — see policy note)*

```bash
git add tests/run.sh
git commit -m "test: add bash test harness for ccp"
```

---

### Task 2: `ccp_norm_path`, `ccp_is_ancestor`, `ccp_depth`

These are mechanical renames of the existing `ds_*` helpers (bodies unchanged). Locking the new names early so every later task references `ccp_*`.

**Files:**
- Modify: `lib/paths.sh` (replace the three helper definitions + header)
- Modify: `tests/run.sh` (append tests)

- [ ] **Step 1: Write failing tests** (append above the `# ---- runner ----` line in `tests/run.sh`)

```bash
test_norm_path_tilde() {
  assert_eq "$(ccp_norm_path '~/x')" "$HOME/x" "tilde expands"
}
test_norm_path_dotdot() {
  assert_eq "$(ccp_norm_path '/a/b/../c')" "/a/c" ".. collapses"
}
test_norm_path_root() {
  assert_eq "$(ccp_norm_path '/')" "/" "root stays root"
}
test_is_ancestor() {
  ccp_is_ancestor /a /a/b/c; assert_rc "$?" 0 "/a ancestor of /a/b/c"
  ccp_is_ancestor /a/b /a;   assert_rc "$?" 1 "/a/b not ancestor of /a"
}
test_depth() {
  assert_eq "$(ccp_depth /a/b/c)" "3" "depth 3"
  assert_eq "$(ccp_depth /)" "0" "root depth 0"
}
```

- [ ] **Step 2: Run to verify they fail**

Run: `bash tests/run.sh norm; bash tests/run.sh ancestor; bash tests/run.sh depth`
Expected: FAILs with `ccp_norm_path: command not found` style errors (functions not defined yet).

- [ ] **Step 3: Rename helpers in `lib/paths.sh`**

Replace the file header comment and the three helper functions. The new top of `lib/paths.sh`:

```bash
#!/usr/bin/env bash
# ============================================================
#  lib/paths.sh — motor de reglas de paths para ccp
#
#  Resolución para un path P:
#    - Entre las reglas cuyo path es P o ancestro de P, gana la MÁS
#      ESPECÍFICA (ruta más profunda). Paths únicos => sin empates.
#    - Sin regla aplicable -> "default".
#
#  Formato de DS_RULES_FILE (rules.tsv), una regla por línea:
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
```

Delete the old `ds_norm_path`, `ds_is_ancestor`, `ds_depth` definitions (they are replaced above). Leave `ds_resolve`, `ds_rule_add`, `ds_rule_del` for now — Task 3 replaces them.

- [ ] **Step 4: Run to verify pass**

Run: `bash tests/run.sh norm; bash tests/run.sh ancestor; bash tests/run.sh depth`
Expected: all `passed, 0 failed`.

- [ ] **Step 5: Commit** *(authorization required)*

```bash
git add lib/paths.sh tests/run.sh
git commit -m "refactor: rename path helpers ds_* -> ccp_*"
```

---

### Task 3: `ccp_resolve`, `ccp_rule_set`, `ccp_rule_del`

New rule format `path<TAB>profile`; resolver returns the profile name (or `default`).

**Files:**
- Modify: `lib/paths.sh` (replace `ds_resolve`/`ds_rule_add`/`ds_rule_del`)
- Modify: `tests/run.sh`

- [ ] **Step 1: Write failing tests**

```bash
test_resolve_empty_is_default() {
  local rf; rf="$(newdir)/rules.tsv"; : > "$rf"
  assert_eq "$(ccp_resolve /any/path "$rf")" "default" "no rules => default"
}
test_resolve_most_specific_wins() {
  local rf; rf="$(newdir)/rules.tsv"
  ccp_rule_set /a work "$rf"
  ccp_rule_set /a/b/c deepseek "$rf"
  assert_eq "$(ccp_resolve /a/x "$rf")"       "work"     "inherit ancestor"
  assert_eq "$(ccp_resolve /a/b/c/z "$rf")"   "deepseek" "deeper wins"
}
test_resolve_carveout_default() {
  local rf; rf="$(newdir)/rules.tsv"
  ccp_rule_set /a deepseek "$rf"
  ccp_rule_set /a/b default "$rf"
  assert_eq "$(ccp_resolve /a/b/x "$rf")" "default" "default carve-out wins"
}
test_rule_set_replaces() {
  local rf; rf="$(newdir)/rules.tsv"
  ccp_rule_set /a work "$rf"
  ccp_rule_set /a personal "$rf"   # replace, not append
  assert_eq "$(grep -c . "$rf")" "1" "one line after replace"
  assert_eq "$(ccp_resolve /a "$rf")" "personal" "replaced value"
}
test_rule_del() {
  local rf; rf="$(newdir)/rules.tsv"
  ccp_rule_set /a work "$rf"
  ccp_rule_del /a "$rf"
  assert_eq "$(ccp_resolve /a "$rf")" "default" "deleted => default"
}
```

- [ ] **Step 2: Run to verify they fail**

Run: `bash tests/run.sh resolve; bash tests/run.sh rule`
Expected: FAILs (`ccp_resolve`/`ccp_rule_set`/`ccp_rule_del` not defined).

- [ ] **Step 3: Replace the resolver + rule mutators in `lib/paths.sh`**

```bash
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
```

Delete the old `ds_resolve`, `ds_rule_add`, `ds_rule_del` definitions.

- [ ] **Step 4: Run to verify pass**

Run: `bash tests/run.sh`
Expected: all path tests `passed, 0 failed`.

- [ ] **Step 5: Commit** *(authorization required)*

```bash
git add lib/paths.sh tests/run.sh
git commit -m "feat: path resolver returns profile name; rules are path<TAB>profile"
```

---

## Phase 2 — Profile storage

### Task 4: `lib/profiles.sh` — existence, add, type, get

**Files:**
- Create: `lib/profiles.sh`
- Modify: `tests/run.sh`

- [ ] **Step 1: Write failing tests**

```bash
test_profile_add_official() {
  local h; h="$(newdir)"
  ccp_profile_add_official "$h" work
  ccp_profile_exists "$h" work; assert_rc "$?" 0 "work exists"
  assert_eq "$(ccp_profile_type "$h" work)" "official" "type official"
  [[ -d "$h/profiles/work/cc-home" ]]; assert_rc "$?" 0 "cc-home created"
}
test_profile_add_deepseek() {
  local h; h="$(newdir)"
  ccp_profile_add_deepseek "$h" ds "https://api.deepseek.com/anthropic" "pro[1m]" "flash" "max"
  assert_eq "$(ccp_profile_type "$h" ds)" "deepseek" "type deepseek"
  assert_eq "$(ccp_profile_get "$h" ds base_url)" "https://api.deepseek.com/anthropic" "base_url"
  assert_eq "$(ccp_profile_get "$h" ds model_pro)" "pro[1m]" "model_pro"
  assert_eq "$(ccp_profile_get "$h" ds effort)" "max" "effort"
}
test_profile_exists_false() {
  local h; h="$(newdir)"
  ccp_profile_exists "$h" nope; assert_rc "$?" 1 "missing => rc1"
}
```

- [ ] **Step 2: Run to verify they fail**

Run: `bash tests/run.sh profile`
Expected: FAILs (`lib/profiles.sh` does not exist / functions undefined).

- [ ] **Step 3: Create `lib/profiles.sh`**

```bash
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
```

- [ ] **Step 4: Run to verify pass**

Run: `bash tests/run.sh profile`
Expected: `passed, 0 failed`.

- [ ] **Step 5: Commit** *(authorization required)*

```bash
git add lib/profiles.sh tests/run.sh
git commit -m "feat: profile storage (add/exists/type/get)"
```

---

### Task 5: `lib/profiles.sh` — key, list, rm

**Files:**
- Modify: `lib/profiles.sh`
- Modify: `tests/run.sh`

- [ ] **Step 1: Write failing tests**

```bash
test_profile_key() {
  local h; h="$(newdir)"
  ccp_profile_add_deepseek "$h" ds "url" "p" "f" "max"
  ccp_profile_set_key "$h" ds "sk-secret-123"
  assert_eq "$(ccp_profile_get_key "$h" ds)" "sk-secret-123" "key roundtrip"
  local mode; mode="$(stat -f '%Lp' "$h/profiles/ds/api_key" 2>/dev/null || stat -c '%a' "$h/profiles/ds/api_key")"
  assert_eq "$mode" "600" "key file is 600"
}
test_profile_list() {
  local h; h="$(newdir)"
  ccp_profile_add_official "$h" work
  ccp_profile_add_deepseek "$h" ds "url" "p" "f" "max"
  assert_eq "$(ccp_profile_list "$h" | sort | tr '\n' ' ')" "ds official work " "list names sorted"
}
test_profile_rm() {
  local h; h="$(newdir)"
  ccp_profile_add_official "$h" work
  ccp_profile_rm "$h" work
  ccp_profile_exists "$h" work; assert_rc "$?" 1 "removed"
  assert_eq "$(ccp_profile_list "$h")" "" "index empty after rm"
}
```

Note: `ccp_profile_list` prints one `name` per line (the `official` token in the expected string above is the profile *named* such only in this fixture — fix fixture: the test creates profiles named `work` and `ds`, so expected sorted is `ds work`). Correct the expected:

```bash
test_profile_list() {
  local h; h="$(newdir)"
  ccp_profile_add_official "$h" work
  ccp_profile_add_deepseek "$h" ds "url" "p" "f" "max"
  assert_eq "$(ccp_profile_list "$h" | sort | tr '\n' ' ')" "ds work " "list names sorted"
}
```

- [ ] **Step 2: Run to verify they fail**

Run: `bash tests/run.sh 'profile_key\|profile_list\|profile_rm'`
Expected: FAILs (functions undefined).

- [ ] **Step 3: Append to `lib/profiles.sh`**

```bash
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
```

- [ ] **Step 4: Run to verify pass**

Run: `bash tests/run.sh 'profile'`
Expected: `passed, 0 failed`.

- [ ] **Step 5: Commit** *(authorization required)*

```bash
git add lib/profiles.sh tests/run.sh
git commit -m "feat: profile key storage, list, rm"
```

---

## Phase 3 — Env delta

### Task 6: `lib/env.sh` — `ccp_env_delta`

**Files:**
- Create: `lib/env.sh`
- Modify: `tests/run.sh`

- [ ] **Step 1: Write failing tests**

The delta must (a) always begin by unsetting all managed vars, (b) export the right vars per type. Tests assert on the emitted text and on the *effect* of `eval`-ing it in a subshell.

```bash
_MANAGED='CLAUDE_CONFIG_DIR ANTHROPIC_BASE_URL ANTHROPIC_AUTH_TOKEN ANTHROPIC_MODEL ANTHROPIC_DEFAULT_OPUS_MODEL ANTHROPIC_DEFAULT_SONNET_MODEL ANTHROPIC_DEFAULT_HAIKU_MODEL CLAUDE_CODE_SUBAGENT_MODEL CLAUDE_CODE_EFFORT_LEVEL CCP_PROFILE'

test_env_default_unsets_all() {
  local h; h="$(newdir)"
  local out; out="$(ccp_env_delta "$h" default)"
  # debe contener un unset de las vars manejadas y setear CCP_PROFILE=default
  case "$out" in *"unset "*"ANTHROPIC_BASE_URL"*) :;; *) _fail=$((_fail+1)); echo "FAIL: default no unset" >&2;; esac
  # efecto: tras eval, ANTHROPIC_BASE_URL vacía y CCP_PROFILE=default
  local got; got="$(ANTHROPIC_BASE_URL=leak; eval "$out"; printf '%s|%s' "${ANTHROPIC_BASE_URL:-}" "${CCP_PROFILE:-}")"
  assert_eq "$got" "|default" "default clears leak, sets CCP_PROFILE"
}
test_env_official() {
  local h; h="$(newdir)"; ccp_profile_add_official "$h" work
  local out; out="$(ccp_env_delta "$h" work)"
  local got; got="$(eval "$out"; printf '%s|%s' "${CLAUDE_CONFIG_DIR:-}" "${CCP_PROFILE:-}")"
  assert_eq "$got" "$h/profiles/work/cc-home|work" "official exports CLAUDE_CONFIG_DIR + CCP_PROFILE"
}
test_env_deepseek() {
  local h; h="$(newdir)"
  ccp_profile_add_deepseek "$h" ds "https://x/anthropic" "pro[1m]" "flash" "high"
  ccp_profile_set_key "$h" ds "sk-key"
  local out; out="$(ccp_env_delta "$h" ds)"
  local got; got="$(eval "$out"; printf '%s|%s|%s|%s' "${ANTHROPIC_BASE_URL:-}" "${ANTHROPIC_AUTH_TOKEN:-}" "${ANTHROPIC_MODEL:-}" "${CLAUDE_CODE_EFFORT_LEVEL:-}")"
  assert_eq "$got" "https://x/anthropic|sk-key|pro[1m]|high" "deepseek exports provider vars"
  # y NO debe setear CLAUDE_CONFIG_DIR
  local cfg; cfg="$(eval "$out"; printf '%s' "${CLAUDE_CONFIG_DIR:-NONE}")"
  assert_eq "$cfg" "NONE" "deepseek leaves CLAUDE_CONFIG_DIR unset"
}
```

- [ ] **Step 2: Run to verify they fail**

Run: `bash tests/run.sh env`
Expected: FAILs (`ccp_env_delta` undefined).

- [ ] **Step 3: Create `lib/env.sh`**

```bash
#!/usr/bin/env bash
# ============================================================
#  lib/env.sh — emite el delta de entorno (eval-able) para un perfil.
#
#  ccp_env_delta <ccp_home> <profile>
#    Imprime, en este orden:
#      1) un 'unset' de TODAS las vars manejadas (estado limpio)
#      2) los 'export' del perfil objetivo
#    La salida está pensada para `eval "$(ccp_env_delta ...)"` desde la
#    función shell o el hook. Valores quoteados con %q.
#
#  Requiere que lib/profiles.sh esté sourced (usa ccp_profile_*).
# ============================================================

CCP_MANAGED_VARS="CLAUDE_CONFIG_DIR ANTHROPIC_BASE_URL ANTHROPIC_AUTH_TOKEN ANTHROPIC_MODEL ANTHROPIC_DEFAULT_OPUS_MODEL ANTHROPIC_DEFAULT_SONNET_MODEL ANTHROPIC_DEFAULT_HAIKU_MODEL CLAUDE_CODE_SUBAGENT_MODEL CLAUDE_CODE_EFFORT_LEVEL CCP_PROFILE"

ccp_env_delta() { # ccp_home profile
  local home="$1" profile="$2"
  # 1) limpiar siempre
  printf 'unset %s\n' "$CCP_MANAGED_VARS"

  # 2) default => solo marcar
  if [[ "$profile" == "default" ]]; then
    printf 'export CCP_PROFILE=%q\n' "default"
    return 0
  fi

  if ! ccp_profile_exists "$home" "$profile"; then
    printf 'echo "⚠️  ccp: perfil %q no existe; usando default" >&2\n' "$profile"
    printf 'export CCP_PROFILE=%q\n' "default"
    return 0
  fi

  local type; type="$(ccp_profile_type "$home" "$profile")"
  case "$type" in
    official)
      printf 'export CLAUDE_CONFIG_DIR=%q\n' "$home/profiles/$profile/cc-home"
      printf 'export CCP_PROFILE=%q\n' "$profile"
      ;;
    deepseek)
      local base_url model_pro model_flash effort key
      base_url="$(ccp_profile_get "$home" "$profile" base_url)"
      model_pro="$(ccp_profile_get "$home" "$profile" model_pro)"
      model_flash="$(ccp_profile_get "$home" "$profile" model_flash)"
      effort="$(ccp_profile_get "$home" "$profile" effort)"
      printf 'export ANTHROPIC_BASE_URL=%q\n' "$base_url"
      if key="$(ccp_profile_get_key "$home" "$profile")"; then
        printf 'export ANTHROPIC_AUTH_TOKEN=%q\n' "$key"
      else
        printf 'echo "⚠️  ccp: perfil %q sin API key (ccp key %q)" >&2\n' "$profile" "$profile"
      fi
      printf 'export ANTHROPIC_MODEL=%q\n' "$model_pro"
      printf 'export ANTHROPIC_DEFAULT_OPUS_MODEL=%q\n' "$model_pro"
      printf 'export ANTHROPIC_DEFAULT_SONNET_MODEL=%q\n' "$model_pro"
      printf 'export ANTHROPIC_DEFAULT_HAIKU_MODEL=%q\n' "$model_flash"
      printf 'export CLAUDE_CODE_SUBAGENT_MODEL=%q\n' "$model_flash"
      printf 'export CLAUDE_CODE_EFFORT_LEVEL=%q\n' "$effort"
      printf 'export CCP_PROFILE=%q\n' "$profile"
      ;;
    *)
      printf 'echo "⚠️  ccp: tipo de perfil desconocido (%q)" >&2\n' "$type"
      printf 'export CCP_PROFILE=%q\n' "default"
      ;;
  esac
}
```

- [ ] **Step 4: Run to verify pass**

Run: `bash tests/run.sh env`
Expected: `passed, 0 failed`.

- [ ] **Step 5: Commit** *(authorization required)*

```bash
git add lib/env.sh tests/run.sh
git commit -m "feat: env delta emitter (default/official/deepseek)"
```

---

## Phase 4 — Binary scaffold + internal commands

### Task 7: `bin/ccp` scaffold (lib loading, helpers, paths, dispatch)

This creates the binary by copying the structure of `bin/dsctl` and adapting it. Subsequent tasks add `cmd_*` functions. We build incrementally and verify with `bash -n` + a smoke invocation.

**Files:**
- Create: `bin/ccp`

- [ ] **Step 1: Write the scaffold**

```bash
#!/usr/bin/env bash
# ============================================================
#  ccp — Claude Code profile/account router (ex-dsctl)
#  Enruta Claude Code a una cuenta oficial o a un provider
#  (DeepSeek) por terminal y por PATH, vía perfiles con nombre.
# ============================================================
set -o pipefail
CCP_VERSION="3.0.0"

# --- localizar libs (junto al binario o instaladas) ---
_self="${BASH_SOURCE[0]}"
while [[ -h "$_self" ]]; do _self="$(readlink "$_self")"; done
CCP_DIR="$(cd "$(dirname "$_self")/.." && pwd)"
_libdir="$CCP_DIR/lib"
[[ -f "$_libdir/paths.sh" ]] || _libdir="$HOME/.local/lib/ccp"
for _l in paths.sh profiles.sh env.sh; do
  # shellcheck disable=SC1090
  [[ -f "$_libdir/$_l" ]] && source "$_libdir/$_l"
done

# --- rutas de config ---
CCP_HOME="${CCP_HOME:-$HOME/.config/ccp}"
CCP_RULES_FILE="$CCP_HOME/rules.tsv"
CCP_CONF_FILE="$CCP_HOME/config"
# defaults usados al crear perfiles deepseek nuevos
CCP_BASE_URL_DEFAULT="https://api.deepseek.com/anthropic"
CCP_MODEL_PRO_DEFAULT="deepseek-v4-pro[1m]"
CCP_MODEL_FLASH_DEFAULT="deepseek-v4-flash"
CCP_EFFORT_DEFAULT="max"

# --- colores ---
if [[ -t 1 && -z "${NO_COLOR:-}" ]]; then
  C_RESET=$'\033[0m'; C_BOLD=$'\033[1m'; C_DIM=$'\033[2m'
  C_RED=$'\033[31m'; C_GRN=$'\033[32m'; C_YEL=$'\033[33m'
  C_BLU=$'\033[34m'; C_MAG=$'\033[35m'; C_CYN=$'\033[36m'
else
  C_RESET=""; C_BOLD=""; C_DIM=""; C_RED=""; C_GRN=""
  C_YEL=""; C_BLU=""; C_MAG=""; C_CYN=""
fi
say()  { printf '%s\n' "$*"; }
ok()   { printf '%s✅ %s%s\n' "$C_GRN" "$*" "$C_RESET"; }
warn() { printf '%s⚠️  %s%s\n' "$C_YEL" "$*" "$C_RESET" >&2; }
err()  { printf '%s❌ %s%s\n' "$C_RED" "$*" "$C_RESET" >&2; }
info() { printf '%s%s%s\n' "$C_CYN" "$*" "$C_RESET"; }
hr()   { printf '%s──────────────────────────────────────────────%s\n' "$C_DIM" "$C_RESET"; }

_path_arg() { if [[ -z "$1" ]]; then printf '%s' "$PWD"; else printf '%s' "$1"; fi; }
_repo_root() { git rev-parse --show-toplevel 2>/dev/null; }

# --- dispatcher (las cmd_* se añaden en tareas siguientes) ---
main() {
  local cmd="$1"; shift 2>/dev/null
  case "$cmd" in
    version|--version|-v) say "ccp v$CCP_VERSION" ;;
    *) err "Comando no implementado aún: '$cmd'"; return 1 ;;
  esac
}
main "$@"
```

- [ ] **Step 2: Syntax + smoke check**

Run: `bash -n bin/ccp && bash bin/ccp version`
Expected: prints `ccp v3.0.0`, exit 0.

- [ ] **Step 3: Commit** *(authorization required)*

```bash
git add bin/ccp
git commit -m "feat: bin/ccp scaffold (lib loading, helpers, dispatch)"
```

---

### Task 8: `_resolve` / `_env` / `_hook` internal commands

**Files:**
- Modify: `bin/ccp` (add `cmd_resolve`, `cmd_env`, `cmd_hook`; wire dispatch)
- Modify: `tests/run.sh` (integration tests invoking the binary)

- [ ] **Step 1: Write failing integration tests**

These run the binary against a temp `CCP_HOME`.

```bash
_ccp() { CCP_HOME="$1" bash "$ROOT/bin/ccp" "${@:2}"; }

test_bin_resolve_default() {
  local h; h="$(newdir)"
  local out rc
  out="$(_ccp "$h" resolve /tmp/whatever)"; rc=$?
  assert_eq "$out" "default" "resolve prints default"
  assert_rc "$rc" 1 "default => exit 1"
}
test_bin_resolve_nondefault_exit0() {
  local h; h="$(newdir)"
  _ccp "$h" profile add ds --deepseek --base-url url --pro p --flash f --effort max >/dev/null
  _ccp "$h" path set /tmp/zone ds >/dev/null
  local out rc; out="$(_ccp "$h" resolve /tmp/zone/x)"; rc=$?
  assert_eq "$out" "ds" "resolve prints profile"
  assert_rc "$rc" 0 "non-default => exit 0"
}
test_bin_hook_emits_eval() {
  local h; h="$(newdir)"
  _ccp "$h" profile add work --official >/dev/null
  _ccp "$h" path set /tmp/wz work >/dev/null
  local out; out="$(_ccp "$h" _hook /tmp/wz/sub)"
  local got; got="$(eval "$out"; printf '%s' "${CCP_PROFILE:-}")"
  assert_eq "$got" "work" "_hook delta sets CCP_PROFILE=work"
}
```

Note: `profile add` and `path set` are implemented in Tasks 9–10. To keep Task 8 independently runnable, also add a minimal direct test that does not need those commands:

```bash
test_bin_env_default() {
  local h; h="$(newdir)"
  local out; out="$(_ccp "$h" _env default)"
  case "$out" in *"export CCP_PROFILE=default"*) _pass=$((_pass+1));;
    *) _fail=$((_fail+1)); echo "FAIL: _env default" >&2;; esac
}
```

- [ ] **Step 2: Run to verify they fail**

Run: `bash tests/run.sh bin_env_default`
Expected: FAIL (`_env` not implemented → "Comando no implementado").

- [ ] **Step 3: Add the internal commands to `bin/ccp`** (insert before `main()`)

```bash
# resolve: imprime el perfil del path; exit 0=no-default, 1=default
cmd_resolve() {
  local v; v="$(ccp_resolve "${1:-$PWD}" "$CCP_RULES_FILE")"
  printf '%s\n' "$v"
  [[ "$v" != "default" ]]
}
# _env: emite el delta de entorno (eval-able) para un perfil
cmd_env() { ccp_env_delta "$CCP_HOME" "${1:-default}"; }
# _hook: resuelve el perfil de un path y emite su delta (1 fork)
cmd_hook() {
  local prof; prof="$(ccp_resolve "${1:-$PWD}" "$CCP_RULES_FILE")"
  ccp_env_delta "$CCP_HOME" "$prof"
}
```

And in `main()`'s `case`, add (before the `*)` arm):

```bash
    resolve)   cmd_resolve "$@" ;;
    _resolve)  cmd_resolve "$@" ;;
    _env)      cmd_env "$@" ;;
    _hook)     cmd_hook "$@" ;;
```

- [ ] **Step 4: Run to verify pass**

Run: `bash tests/run.sh bin_env_default; bash -n bin/ccp`
Expected: `bin_env_default` passes; syntax OK. (`bin_resolve_*` and `bin_hook_*` still fail until Tasks 9–10 — that's expected; do not run them yet.)

- [ ] **Step 5: Commit** *(authorization required)*

```bash
git add bin/ccp tests/run.sh
git commit -m "feat: ccp _resolve/_env/_hook internal commands"
```

---

## Phase 5 — Profile & path CLI commands

### Task 9: `cmd_profile` (add/list/show/rm)

**Files:**
- Modify: `bin/ccp`

- [ ] **Step 1: Write failing tests** (these are the `test_bin_resolve_nondefault_exit0` / `test_bin_hook_emits_eval` from Task 8 plus new ones)

```bash
test_bin_profile_add_list() {
  local h; h="$(newdir)"
  _ccp "$h" profile add work --official >/dev/null
  _ccp "$h" profile add ds --deepseek --base-url u --pro p --flash f --effort max >/dev/null
  assert_eq "$(_ccp "$h" profile list | grep -c .)" "2" "two profiles listed"
}
test_bin_profile_show() {
  local h; h="$(newdir)"
  _ccp "$h" profile add ds --deepseek --base-url u --pro p --flash f --effort high >/dev/null
  _ccp "$h" profile show ds | grep -q "high"; assert_rc "$?" 0 "show prints effort"
}
test_bin_profile_rm() {
  local h; h="$(newdir)"
  _ccp "$h" profile add work --official >/dev/null
  _ccp "$h" profile rm work >/dev/null
  assert_eq "$(_ccp "$h" profile list | grep -c .)" "0" "removed"
}
```

- [ ] **Step 2: Run to verify they fail**

Run: `bash tests/run.sh bin_profile`
Expected: FAILs ("Comando no implementado").

- [ ] **Step 3: Add `cmd_profile` to `bin/ccp`** (before `main()`)

```bash
cmd_profile() {
  mkdir -p "$CCP_HOME"
  local sub="$1"; shift 2>/dev/null
  case "$sub" in
    add)      _profile_add "$@" ;;
    rm|del)   _profile_rm "$@" ;;
    list|ls|"") _profile_list_pretty ;;
    show)     _profile_show "$@" ;;
    login)    _profile_login "$@" ;;
    *) err "profile: subcomando desconocido '$sub'"; say "Usa: add | rm | list | show | login"; return 1 ;;
  esac
}

_profile_add() {
  local name="$1"; shift 2>/dev/null
  [[ -z "$name" ]] && { err "Uso: ccp profile add <nombre> --official|--deepseek [opts]"; return 1; }
  [[ "$name" == "default" ]] && { err "'default' es un perfil reservado."; return 1; }
  local kind="" base="$CCP_BASE_URL_DEFAULT" pro="$CCP_MODEL_PRO_DEFAULT"
  local flash="$CCP_MODEL_FLASH_DEFAULT" effort="$CCP_EFFORT_DEFAULT"
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --official) kind=official ;;
      --deepseek) kind=deepseek ;;
      --base-url) base="$2"; shift ;;
      --pro)      pro="$2"; shift ;;
      --flash)    flash="$2"; shift ;;
      --effort)   effort="$2"; shift ;;
      *) err "opción desconocida: $1"; return 1 ;;
    esac
    shift
  done
  case "$kind" in
    official)
      ccp_profile_add_official "$CCP_HOME" "$name"
      ok "Perfil oficial '$name' creado."
      info "Loguéate una vez:  ccp profile login $name   (corre /login dentro)" ;;
    deepseek)
      ccp_profile_add_deepseek "$CCP_HOME" "$name" "$base" "$pro" "$flash" "$effort"
      ok "Perfil deepseek '$name' creado."
      info "Añade su API key:  ccp key $name" ;;
    *) err "Especifica --official o --deepseek"; return 1 ;;
  esac
}

_profile_rm() {
  local name="$1"
  [[ -z "$name" ]] && { err "Uso: ccp profile rm <nombre>"; return 1; }
  ccp_profile_exists "$CCP_HOME" "$name" || { warn "No existe el perfil '$name'."; return 1; }
  ccp_profile_rm "$CCP_HOME" "$name"
  ok "Perfil '$name' eliminado. (Las reglas que lo usaban ahora resuelven a su ancestro.)"
}

_profile_list() { ccp_profile_list "$CCP_HOME"; }

_profile_list_pretty() {
  hr; printf ' %sPerfiles%s\n' "$C_BOLD" "$C_RESET"; hr
  local any=0 name type
  while read -r name; do
    [[ -z "$name" ]] && continue
    type="$(ccp_profile_type "$CCP_HOME" "$name")"
    case "$type" in
      official) printf '   🔑 %-16s %sofficial%s\n' "$name" "$C_CYN" "$C_RESET" ;;
      deepseek) printf '   🟢 %-16s %sdeepseek%s\n' "$name" "$C_GRN" "$C_RESET" ;;
      *)        printf '   ?  %-16s %s\n' "$name" "$type" ;;
    esac
    any=1
  done < <(ccp_profile_list "$CCP_HOME" | sort)
  (( any )) || say "   (sin perfiles — todo usa 'default')"
  printf '   %s⚪ %-16s login ~/.claude (reservado)%s\n' "$C_DIM" "default" "$C_RESET"
  hr
}

_profile_show() {
  local name="$1"
  [[ -z "$name" ]] && { err "Uso: ccp profile show <nombre>"; return 1; }
  ccp_profile_exists "$CCP_HOME" "$name" || { err "No existe '$name'."; return 1; }
  hr; printf ' %sPerfil: %s%s\n' "$C_BOLD" "$name" "$C_RESET"; hr
  local type; type="$(ccp_profile_type "$CCP_HOME" "$name")"
  printf ' Tipo:        %s\n' "$type"
  if [[ "$type" == "deepseek" ]]; then
    printf ' Base URL:    %s\n' "$(ccp_profile_get "$CCP_HOME" "$name" base_url)"
    printf ' Modelo pro:  %s\n' "$(ccp_profile_get "$CCP_HOME" "$name" model_pro)"
    printf ' Modelo flash:%s\n' "$(ccp_profile_get "$CCP_HOME" "$name" model_flash)"
    printf ' Effort:      %s\n' "$(ccp_profile_get "$CCP_HOME" "$name" effort)"
    printf ' API key:     %s\n' "$([[ -f "$CCP_HOME/profiles/$name/api_key" ]] && echo 'OK ✅' || echo 'falta ❌ (ccp key '"$name"')')"
  elif [[ "$type" == "official" ]]; then
    local cch="$CCP_HOME/profiles/$name/cc-home"
    printf ' Config dir:  %s\n' "$cch"
    printf ' Login:       %s\n' "$([[ -f "$cch/.claude.json" ]] && echo 'configurado ✅' || echo 'pendiente ❌ (ccp profile login '"$name"')')"
  fi
  hr
}
```

`_profile_login` is added in Task 12 (official-dir phase); add a temporary stub now so dispatch is complete:

```bash
_profile_login() { err "ccp profile login se implementa en la fase de dirs oficiales (Task 12)."; return 1; }
```

Wire dispatch — add to `main()` case:

```bash
    profile|account) cmd_profile "$@" ;;
```

- [ ] **Step 4: Run to verify pass**

Run: `bash tests/run.sh bin_profile; bash -n bin/ccp`
Expected: `bin_profile_*` pass; syntax OK.

- [ ] **Step 5: Commit** *(authorization required)*

```bash
git add bin/ccp tests/run.sh
git commit -m "feat: ccp profile add/list/show/rm"
```

---

### Task 10: `cmd_path` (set/rm/list/test/clear/edit + legacy include/exclude) and `cmd_key`

**Files:**
- Modify: `bin/ccp`

- [ ] **Step 1: Write failing tests**

```bash
test_bin_path_set_test() {
  local h; h="$(newdir)"
  _ccp "$h" profile add work --official >/dev/null
  _ccp "$h" path set /tmp/p1 work >/dev/null
  assert_eq "$(_ccp "$h" path test /tmp/p1/x)" "work" "path test prints profile"
}
test_bin_path_set_unknown_profile_rejected() {
  local h; h="$(newdir)"
  local out rc; out="$(_ccp "$h" path set /tmp/p2 ghost 2>&1)"; rc=$?
  assert_rc "$rc" 1 "unknown profile rejected"
}
test_bin_path_legacy_include() {
  local h; h="$(newdir)"
  _ccp "$h" profile add deepseek --deepseek --base-url u --pro p --flash f --effort max >/dev/null
  _ccp "$h" path include /tmp/leg >/dev/null   # legacy => assigns 'deepseek'
  assert_eq "$(_ccp "$h" path test /tmp/leg/x)" "deepseek" "legacy include => deepseek"
}
test_bin_key_sets_profile_key() {
  local h; h="$(newdir)"
  _ccp "$h" profile add ds --deepseek --base-url u --pro p --flash f --effort max >/dev/null
  _ccp "$h" key ds sk-abc >/dev/null
  assert_eq "$(ccp_profile_get_key "$h" ds)" "sk-abc" "ccp key <profile> stores"
}
```

- [ ] **Step 2: Run to verify they fail**

Run: `bash tests/run.sh 'bin_path\|bin_key'`
Expected: FAILs.

- [ ] **Step 3: Add `cmd_path` and `cmd_key` to `bin/ccp`**

```bash
cmd_path() {
  mkdir -p "$CCP_HOME"; touch "$CCP_RULES_FILE"
  local sub="$1"; shift 2>/dev/null
  case "$sub" in
    set)
      local p prof="$2"
      p="$(ccp_norm_path "$(_path_arg "$1")")"
      [[ -z "$prof" ]] && { err "Uso: ccp path set <ruta> <perfil>"; return 1; }
      if [[ "$prof" != "default" ]] && ! ccp_profile_exists "$CCP_HOME" "$prof"; then
        err "El perfil '$prof' no existe (ccp profile add $prof ...)."; return 1
      fi
      ccp_rule_set "$p" "$prof" "$CCP_RULES_FILE"
      ok "REGLA: $p  ->  $prof  (y subcarpetas)" ;;
    include|inc|add)   # legacy: include => perfil 'deepseek'
      local p; p="$(ccp_norm_path "$(_path_arg "$1")")"
      ccp_profile_exists "$CCP_HOME" deepseek || { err "No hay perfil 'deepseek' (legacy). Usa 'ccp path set <ruta> <perfil>'."; return 1; }
      ccp_rule_set "$p" deepseek "$CCP_RULES_FILE"
      ok "REGLA (legacy): $p  ->  deepseek" ;;
    exclude|exc)       # legacy: exclude => perfil 'default'
      local p; p="$(ccp_norm_path "$(_path_arg "$1")")"
      ccp_rule_set "$p" default "$CCP_RULES_FILE"
      ok "REGLA (legacy): $p  ->  default" ;;
    rm|remove|del)
      local p; p="$(ccp_norm_path "$(_path_arg "$1")")"
      ccp_rule_del "$p" "$CCP_RULES_FILE"; ok "Regla eliminada: $p" ;;
    list|ls|"") cmd_path_list ;;
    test|check)
      local p v; p="$(ccp_norm_path "$(_path_arg "$1")")"
      v="$(ccp_resolve "$p" "$CCP_RULES_FILE")"
      printf '%s\n' "$v"
      [[ "$v" != "default" ]] ;;
    clear) : > "$CCP_RULES_FILE"; ok "Todas las reglas eliminadas." ;;
    edit)  "${EDITOR:-nano}" "$CCP_RULES_FILE" ;;
    *) err "path: subcomando desconocido '$sub'"; say "Usa: set | rm | list | test | clear | edit" ;;
  esac
}

cmd_path_list() {
  hr; printf ' %sReglas de PATH%s\n' "$C_BOLD" "$C_RESET"; hr
  if [[ ! -s "$CCP_RULES_FILE" ]]; then
    say "  (sin reglas — todo usa 'default')"; hr; return
  fi
  local path profile
  while IFS=$'\t' read -r path profile; do
    [[ -z "$path" || "$path" == \#* ]] && continue
    printf '   %-40s -> %s%s%s\n' "$path" "$C_GRN" "$profile" "$C_RESET"
  done < <(sort -t$'\t' -k1 "$CCP_RULES_FILE")
  hr
  info "Regla efectiva para el cwd:"
  printf '   %s -> %s\n' "$PWD" "$(ccp_resolve "$PWD" "$CCP_RULES_FILE")"
  hr
}

# ccp key <perfil> [API_KEY]   (key de un perfil deepseek)
cmd_key() {
  local name="$1"; shift 2>/dev/null
  [[ -z "$name" ]] && { err "Uso: ccp key <perfil> [API_KEY]"; return 1; }
  ccp_profile_exists "$CCP_HOME" "$name" || { err "No existe el perfil '$name'."; return 1; }
  [[ "$(ccp_profile_type "$CCP_HOME" "$name")" == "deepseek" ]] || { err "'$name' no es un perfil deepseek."; return 1; }
  local key="$1"
  if [[ -z "$key" ]]; then printf 'Pega la API key de %s (oculta): ' "$name"; read -rs key; echo; fi
  [[ -z "$key" ]] && { err "No ingresaste ninguna key."; return 1; }
  ccp_profile_set_key "$CCP_HOME" "$name" "$key"
  ok "API key guardada para '$name' (600)."
}
```

Wire dispatch — add to `main()` case:

```bash
    path) cmd_path "$@" ;;
    key)  cmd_key "$@" ;;
```

- [ ] **Step 4: Run to verify pass**

Run: `bash tests/run.sh 'bin_path\|bin_key\|bin_resolve\|bin_hook'; bash -n bin/ccp`
Expected: all pass (Task 8's `bin_resolve_nondefault_exit0` and `bin_hook_emits_eval` now pass too).

- [ ] **Step 5: Commit** *(authorization required)*

```bash
git add bin/ccp tests/run.sh
git commit -m "feat: ccp path set/list/test + legacy include/exclude + per-profile key"
```

---

## Phase 6 — Official dirs, shell function/hook, status, migration, install

### Task 11: `cmd_status` (+ `--json`)

**Files:**
- Modify: `bin/ccp`

- [ ] **Step 1: Write failing tests**

```bash
test_bin_status_json_default() {
  local h; h="$(newdir)"
  local out; out="$(_ccp "$h" status --json)"
  case "$out" in *'"profile":"default"'*) _pass=$((_pass+1));;
    *) _fail=$((_fail+1)); echo "FAIL: status json profile default: $out" >&2;; esac
}
test_bin_status_json_profile_type() {
  local h; h="$(newdir)"
  _ccp "$h" profile add ds --deepseek --base-url u --pro p --flash f --effort max >/dev/null
  _ccp "$h" path set /tmp/sz ds >/dev/null
  local out; out="$(cd /tmp/sz 2>/dev/null && CCP_HOME="$h" bash "$ROOT/bin/ccp" status --json)"
  case "$out" in *'"profile":"ds"'*'"profile_type":"deepseek"'*) _pass=$((_pass+1));;
    *) _fail=$((_fail+1)); echo "FAIL: status json profile_type: $out" >&2;; esac
}
```

- [ ] **Step 2: Run to verify they fail**

Run: `bash tests/run.sh bin_status`
Expected: FAILs.

- [ ] **Step 3: Add `cmd_status` to `bin/ccp`**

```bash
_json_esc() { local s="$1"; s="${s//\\/\\\\}"; s="${s//\"/\\\"}"; printf '%s' "$s"; }

cmd_status() {
  local rule type
  rule="$(ccp_resolve "$PWD" "$CCP_RULES_FILE")"
  if [[ "$rule" == "default" ]]; then type="default"; else type="$(ccp_profile_type "$CCP_HOME" "$rule" 2>/dev/null || echo default)"; fi
  if [[ "$1" == "--json" ]]; then
    local active="${CCP_PROFILE:-default}" repo; repo="$(_repo_root)"
    printf '{"active":"%s","profile":"%s","profile_type":"%s","cwd":"%s","repo":"%s"}\n' \
      "$(_json_esc "$active")" "$(_json_esc "$rule")" "$(_json_esc "$type")" \
      "$(_json_esc "$PWD")" "$(_json_esc "${repo:-}")"
    return 0
  fi
  hr; printf ' %sEstado de ccp en esta terminal%s\n' "$C_BOLD" "$C_RESET"; hr
  printf ' Perfil activo (terminal): %s\n' "${CCP_PROFILE:-default}"
  printf ' Perfil del cwd (regla):   %s  (%s)\n' "$rule" "$type"
  printf ' Cwd:                      %s\n' "$PWD"
  local repo; repo="$(_repo_root)"; printf ' Repo:                     %s\n' "${repo:-no es git}"
  hr
}
```

Wire dispatch:

```bash
    status) cmd_status "$@" ;;
```

- [ ] **Step 4: Run to verify pass**

Run: `bash tests/run.sh bin_status; bash -n bin/ccp`
Expected: pass.

- [ ] **Step 5: Commit** *(authorization required)*

```bash
git add bin/ccp tests/run.sh
git commit -m "feat: ccp status + --json (profile-aware)"
```

---

### Task 12: Official dir seeding (selective symlink) + `ccp profile login`

**Files:**
- Modify: `bin/ccp` (replace `_profile_login` stub; add `_seed_official_home`; call seeding from `_profile_add` official branch)

- [ ] **Step 1: Write failing test**

Seeding symlinks shareable items from a *source* `~/.claude` into the profile's `cc-home`. To test without touching the real `~/.claude`, `_seed_official_home` takes the source dir as an overridable variable `CCP_CLAUDE_SRC` (default `$HOME/.claude`).

```bash
test_seed_official_symlinks() {
  local h; h="$(newdir)"
  local src; src="$(newdir)"
  mkdir -p "$src/plugins" "$src/commands" "$src/agents"
  printf 'x' > "$src/CLAUDE.md"
  printf '{"k":1}' > "$src/settings.json"
  CCP_HOME="$h" CCP_CLAUDE_SRC="$src" bash "$ROOT/bin/ccp" profile add work --official >/dev/null
  local cch="$h/profiles/work/cc-home"
  [[ -L "$cch/plugins" ]]; assert_rc "$?" 0 "plugins symlinked"
  [[ -L "$cch/CLAUDE.md" ]]; assert_rc "$?" 0 "CLAUDE.md symlinked"
  # settings.json copiado (no symlink) y separado
  [[ -f "$cch/settings.json" && ! -L "$cch/settings.json" ]]; assert_rc "$?" 0 "settings.json copied not linked"
  # credenciales/history NO se tocan (no existen)
  [[ ! -e "$cch/.claude.json" ]]; assert_rc "$?" 0 "no creds seeded"
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `bash tests/run.sh seed_official`
Expected: FAIL (no symlinks; seeding not implemented).

- [ ] **Step 3: Implement seeding + login in `bin/ccp`**

Add near the other helpers:

```bash
# Siembra un cc-home oficial: symlinks de lo compartible, copia de settings.json.
# Fuente override-able para tests: CCP_CLAUDE_SRC (default ~/.claude).
_seed_official_home() { # cc_home_dir
  local cch="$1"; local src="${CCP_CLAUDE_SRC:-$HOME/.claude}"
  mkdir -p "$cch"
  [[ -d "$src" ]] || return 0
  local item
  for item in plugins commands agents skills CLAUDE.md; do
    if [[ -e "$src/$item" && ! -e "$cch/$item" ]]; then
      ln -s "$src/$item" "$cch/$item"
    fi
  done
  # settings.json: COPIA (CC escribe ahí; no compartir writes entre cuentas)
  if [[ -f "$src/settings.json" && ! -e "$cch/settings.json" ]]; then
    cp "$src/settings.json" "$cch/settings.json"
  fi
}
```

In `_profile_add`, replace the `official)` branch body with:

```bash
    official)
      ccp_profile_add_official "$CCP_HOME" "$name"
      _seed_official_home "$CCP_HOME/profiles/$name/cc-home"
      ok "Perfil oficial '$name' creado (plugins/skills symlinked, settings copiado)."
      info "Loguéate una vez:  ccp profile login $name   (corre /login dentro)" ;;
```

Replace the `_profile_login` stub with:

```bash
_profile_login() {
  local name="$1"
  [[ -z "$name" ]] && { err "Uso: ccp profile login <nombre>"; return 1; }
  ccp_profile_exists "$CCP_HOME" "$name" || { err "No existe '$name'."; return 1; }
  [[ "$(ccp_profile_type "$CCP_HOME" "$name")" == "official" ]] || { err "'$name' no es oficial."; return 1; }
  command -v claude >/dev/null 2>&1 || { err "Claude Code no está instalado."; return 1; }
  local cch="$CCP_HOME/profiles/$name/cc-home"
  info "Abriendo Claude Code con el config dir de '$name'."
  info "Dentro, corre  /login  con la cuenta de este perfil, luego /quit."
  CLAUDE_CONFIG_DIR="$cch" command claude
}
```

- [ ] **Step 4: Run to verify pass**

Run: `bash tests/run.sh seed_official; bash -n bin/ccp`
Expected: pass.

- [ ] **Step 5: Commit** *(authorization required)*

```bash
git add bin/ccp tests/run.sh
git commit -m "feat: official profile cc-home seeding + ccp profile login"
```

---

### Task 13: Shell init heredoc (`ccp()` fn + `_ccp_autocheck` hook) + completion + install/uninstall

**Files:**
- Modify: `bin/ccp` (add `_print_shell_init`, `cmd_install`, `cmd_uninstall`, `cmd_completion`; wire dispatch)

This task has limited unit-testability (it generates rc text). We test that the generated text is syntactically valid bash and contains the markers, and that `ccp()` dispatch logic works when sourced.

- [ ] **Step 1: Write failing tests**

```bash
test_shell_init_valid_bash() {
  local out; out="$(bash "$ROOT/bin/ccp" completion-shellinit 2>/dev/null)"
  # debe parsear como bash
  printf '%s' "$out" | bash -n -; assert_rc "$?" 0 "shell init parses"
  case "$out" in *">>> ccp shell init >>>"*) _pass=$((_pass+1));;
    *) _fail=$((_fail+1)); echo "FAIL: missing marker" >&2;; esac
}
test_shell_fn_use_evals_env() {
  # sourcing el init define ccp(); 'ccp use <perfil>' debe exportar CCP_PROFILE
  local h; h="$(newdir)"
  CCP_HOME="$h" bash "$ROOT/bin/ccp" profile add work --official >/dev/null
  local script; script="$(newdir)/t.sh"
  {
    echo "export CCP_HOME='$h'"
    echo "export PATH='$(dirname "$ROOT/bin/ccp")':\"\$PATH\""
    bash "$ROOT/bin/ccp" completion-shellinit
    echo "ccp use work >/dev/null 2>&1"
    echo "printf '%s' \"\${CCP_PROFILE:-}\""
  } > "$script"
  assert_eq "$(bash "$script")" "work" "ccp use work sets CCP_PROFILE in shell"
}
```

Note: `completion-shellinit` is an internal subcommand that just prints `_print_shell_init` (added so tests and `install` share one source of truth).

- [ ] **Step 2: Run to verify they fail**

Run: `bash tests/run.sh 'shell_init\|shell_fn'`
Expected: FAILs.

- [ ] **Step 3: Add the shell-init generator + install/uninstall/completion**

```bash
_print_shell_init() {
cat <<'SHELL_INIT'
# >>> ccp shell init >>>
ccp() {
  case "$1" in
    use)
      shift; eval "$(command ccp _env "${1:-default}")" ;;
    default|off)
      eval "$(command ccp _env default)" ;;
    on)   # legacy alias
      eval "$(command ccp _env deepseek)" ;;
    run)
      shift
      eval "$(command ccp _hook "$PWD")"
      if [[ $# -gt 0 ]]; then command "$@"; else command claude; fi ;;
    *) command ccp "$@" ;;
  esac
}

# Hook: aplica el perfil del PWD al cambiar de carpeta (cache por PWD).
_ccp_autocheck() {
  command -v ccp >/dev/null 2>&1 || return
  [[ "$PWD" == "${_CCP_LAST_PWD:-}" ]] && return
  _CCP_LAST_PWD="$PWD"
  eval "$(command ccp _hook "$PWD" 2>/dev/null)"
}
if [[ -n "$ZSH_VERSION" ]]; then
  typeset -ag precmd_functions
  [[ " ${precmd_functions[*]} " == *" _ccp_autocheck "* ]] || precmd_functions+=(_ccp_autocheck)
elif [[ -n "$BASH_VERSION" ]]; then
  [[ "$PROMPT_COMMAND" == *_ccp_autocheck* ]] || PROMPT_COMMAND="_ccp_autocheck;${PROMPT_COMMAND:-}"
fi

if command -v ccp >/dev/null 2>&1; then
  if [[ -n "$ZSH_VERSION" ]]; then
    eval "$(ccp completion zsh)" 2>/dev/null
  elif [[ -n "$BASH_VERSION" ]]; then
    eval "$(ccp completion bash)" 2>/dev/null
  fi
fi
# <<< ccp shell init <<<
SHELL_INIT
}

cmd_install() {
  mkdir -p "$CCP_HOME"
  local rc shell_name; shell_name="$(basename "${SHELL:-bash}")"
  case "$shell_name" in zsh) rc="$HOME/.zshrc";; *) rc="$HOME/.bashrc";; esac
  [[ -n "$1" ]] && rc="$1"
  # ofrecer quitar el init viejo de dsctl
  if grep -q "dsctl shell init" "$rc" 2>/dev/null; then
    warn "Detecté el init viejo de dsctl en $rc."
    info "Quítalo con:  dsctl uninstall   (o edita el bloque '# >>> dsctl shell init >>>')"
  fi
  if grep -q "ccp shell init" "$rc" 2>/dev/null; then
    ok "El init de ccp ya está en $rc"
  else
    { echo; echo "# ccp — router de perfiles de Claude Code"; _print_shell_init; } >> "$rc"
    ok "Init añadido a $rc"
  fi
  hr; info "Recarga con:  source $rc"; hr
}

cmd_uninstall() {
  local rc shell_name; shell_name="$(basename "${SHELL:-bash}")"
  case "$shell_name" in zsh) rc="$HOME/.zshrc";; *) rc="$HOME/.bashrc";; esac
  [[ -n "$1" ]] && rc="$1"
  if [[ ! -f "$rc" ]] || ! grep -q "ccp shell init" "$rc"; then warn "No encontré el init en $rc"; return 0; fi
  local tmp; tmp="$(mktemp)"
  awk '
    /# ccp — router de perfiles de Claude Code/ {skip=1}
    /# >>> ccp shell init >>>/ {skip=1}
    skip==0 {print}
    /# <<< ccp shell init <<</ {skip=0; next}
  ' "$rc" > "$tmp" && mv "$tmp" "$rc"
  ok "Init de ccp removido de $rc"
}

cmd_completion() {
  case "$1" in
    bash)
cat <<'COMPLETION_BASH'
_ccp() {
  local cur prev; cur="${COMP_WORDS[COMP_CWORD]}"; prev="${COMP_WORDS[COMP_CWORD-1]}"
  local top="install uninstall key path profile status config doctor menu completion resolve version help use default on off run"
  if [[ $COMP_CWORD -eq 1 ]]; then COMPREPLY=( $(compgen -W "$top" -- "$cur") ); return; fi
  case "${COMP_WORDS[1]}" in
    profile) [[ $COMP_CWORD -eq 2 ]] && COMPREPLY=( $(compgen -W "add rm list show login" -- "$cur") )
             [[ $COMP_CWORD -eq 3 && "${COMP_WORDS[2]}" =~ ^(rm|show|login)$ ]] && COMPREPLY=( $(compgen -W "$(ccp profile list 2>/dev/null)" -- "$cur") ) ;;
    path)    [[ $COMP_CWORD -eq 2 ]] && COMPREPLY=( $(compgen -W "set rm list test clear edit" -- "$cur") )
             [[ $COMP_CWORD -eq 3 && "${COMP_WORDS[2]}" =~ ^(set|rm|test)$ ]] && COMPREPLY=( $(compgen -d -- "$cur") )
             [[ $COMP_CWORD -eq 4 && "${COMP_WORDS[2]}" == "set" ]] && COMPREPLY=( $(compgen -W "default $(ccp profile list 2>/dev/null)" -- "$cur") ) ;;
    use)     COMPREPLY=( $(compgen -W "default $(ccp profile list 2>/dev/null)" -- "$cur") ) ;;
    key)     COMPREPLY=( $(compgen -W "$(ccp profile list 2>/dev/null)" -- "$cur") ) ;;
    completion) COMPREPLY=( $(compgen -W "bash zsh" -- "$cur") ) ;;
  esac
}
complete -F _ccp ccp
COMPLETION_BASH
      ;;
    zsh)
cat <<'COMPLETION_ZSH'
if ! whence compdef >/dev/null 2>&1; then autoload -Uz compinit && compinit -C; fi
_ccp() {
  local -a top; top=(install uninstall key path profile status config doctor menu completion resolve version help use default on off run)
  if (( CURRENT == 2 )); then compadd -- $top; return; fi
  case "${words[2]}" in
    profile) (( CURRENT == 3 )) && compadd -- add rm list show login
             (( CURRENT == 4 )) && [[ "${words[3]}" =~ ^(rm|show|login)$ ]] && compadd -- ${(f)"$(ccp profile list 2>/dev/null)"} ;;
    path)    (( CURRENT == 3 )) && compadd -- set rm list test clear edit
             (( CURRENT == 3 )) || { [[ "${words[3]}" =~ ^(set|rm|test)$ ]] && _path_files -/ }
             (( CURRENT == 4 )) && [[ "${words[3]}" == set ]] && compadd -- default ${(f)"$(ccp profile list 2>/dev/null)"} ;;
    use)     compadd -- default ${(f)"$(ccp profile list 2>/dev/null)"} ;;
    key)     compadd -- ${(f)"$(ccp profile list 2>/dev/null)"} ;;
    completion) compadd -- bash zsh ;;
  esac
}
compdef _ccp ccp
COMPLETION_ZSH
      ;;
    shellinit|*) _print_shell_init ;;
  esac
}
```

Wire dispatch — add to `main()` case:

```bash
    install)   cmd_install "$@" ;;
    uninstall) cmd_uninstall "$@" ;;
    completion) cmd_completion "$@" ;;
    completion-shellinit) _print_shell_init ;;
```

- [ ] **Step 4: Run to verify pass**

Run: `bash tests/run.sh 'shell_init\|shell_fn'; bash -n bin/ccp`
Expected: pass.

- [ ] **Step 5: Commit** *(authorization required)*

```bash
git add bin/ccp tests/run.sh
git commit -m "feat: ccp shell init (fn+hook), completion, install/uninstall"
```

---

### Task 14: Migration from dsctl + auto-trigger

**Files:**
- Modify: `bin/ccp` (add `cmd_migrate`, `_migrate_if_needed`; call from `main()`)

- [ ] **Step 1: Write failing tests**

Migration reads an old dsctl home (override-able via `DSCTL_HOME_SRC` for tests, default `~/.config/dsctl`) and writes into `CCP_HOME`.

```bash
test_migrate_creates_deepseek_profile() {
  local old; old="$(newdir)"
  printf 'DS_BASE_URL="https://api.deepseek.com/anthropic"\nDS_MODEL_PRO="pro[1m]"\nDS_MODEL_FLASH="flash"\nDS_EFFORT="high"\n' > "$old/config"
  printf 'sk-old-key' > "$old/api_key"
  printf 'include\t/a\nexclude\t/a/b\n' > "$old/rules.tsv"
  local h; h="$(newdir)/ccp"   # must NOT pre-exist
  CCP_HOME="$h" DSCTL_HOME_SRC="$old" bash "$ROOT/bin/ccp" migrate >/dev/null
  assert_eq "$(ccp_profile_type "$h" deepseek)" "deepseek" "deepseek profile created"
  assert_eq "$(ccp_profile_get "$h" deepseek effort)" "high" "effort migrated"
  assert_eq "$(ccp_profile_get_key "$h" deepseek)" "sk-old-key" "key migrated"
  assert_eq "$(ccp_resolve /a/x "$h/rules.tsv")"   "deepseek" "include -> deepseek"
  assert_eq "$(ccp_resolve /a/b/y "$h/rules.tsv")" "default"  "exclude -> default"
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `bash tests/run.sh migrate`
Expected: FAIL.

- [ ] **Step 3: Add migration to `bin/ccp`**

```bash
cmd_migrate() {
  local old="${DSCTL_HOME_SRC:-$HOME/.config/dsctl}"
  [[ -d "$old" ]] || { warn "No hay config de dsctl en $old; nada que migrar."; return 0; }
  mkdir -p "$CCP_HOME"
  # 1) perfil deepseek desde config viejo
  local base="$CCP_BASE_URL_DEFAULT" pro="$CCP_MODEL_PRO_DEFAULT" flash="$CCP_MODEL_FLASH_DEFAULT" effort="$CCP_EFFORT_DEFAULT"
  if [[ -f "$old/config" ]]; then
    # parse seguro de DS_*="..." (no source)
    local line k v
    while IFS= read -r line; do
      case "$line" in
        DS_BASE_URL=*)    v="${line#DS_BASE_URL=}";    base="${v%\"}";  base="${base#\"}" ;;
        DS_MODEL_PRO=*)   v="${line#DS_MODEL_PRO=}";   pro="${v%\"}";   pro="${pro#\"}" ;;
        DS_MODEL_FLASH=*) v="${line#DS_MODEL_FLASH=}"; flash="${v%\"}"; flash="${flash#\"}" ;;
        DS_EFFORT=*)      v="${line#DS_EFFORT=}";      effort="${v%\"}";effort="${effort#\"}" ;;
      esac
    done < "$old/config"
  fi
  ccp_profile_add_deepseek "$CCP_HOME" deepseek "$base" "$pro" "$flash" "$effort"
  [[ -f "$old/api_key" ]] && ccp_profile_set_key "$CCP_HOME" deepseek "$(cat "$old/api_key")"
  # 2) reglas include->deepseek, exclude->default
  if [[ -f "$old/rules.tsv" ]]; then
    cp "$old/rules.tsv" "$CCP_HOME/rules.dsctl.bak"   # backup
    : > "$CCP_RULES_FILE"
    local kind path
    while IFS=$'\t' read -r kind path; do
      [[ -z "$kind" || "$kind" == \#* || -z "$path" ]] && continue
      case "$kind" in
        include) ccp_rule_set "$path" deepseek "$CCP_RULES_FILE" ;;
        exclude) ccp_rule_set "$path" default  "$CCP_RULES_FILE" ;;
      esac
    done < "$old/rules.tsv"
  fi
  ok "Migrado desde dsctl: perfil 'deepseek' + reglas (backup en rules.dsctl.bak)."
}

# auto-migrar si hay dsctl viejo y no existe CCP_HOME todavía
_migrate_if_needed() {
  local old="${DSCTL_HOME_SRC:-$HOME/.config/dsctl}"
  [[ -d "$old" && ! -d "$CCP_HOME" ]] || return 0
  cmd_migrate >&2
}
```

In `main()`, call `_migrate_if_needed` at the very top (before the `case`):

```bash
main() {
  _migrate_if_needed
  local cmd="$1"; shift 2>/dev/null
  case "$cmd" in
    ...
    migrate) cmd_migrate "$@" ;;
    ...
```

Note: the `migrate` test sets `CCP_HOME` to a non-existent dir, so `_migrate_if_needed` would also fire — that is fine and idempotent enough here because the explicit `migrate` runs after. To avoid double-run noise in the test, `_migrate_if_needed` writes to stderr only and the explicit `cmd_migrate` re-running `ccp_profile_add_deepseek` simply overwrites identically. Acceptable.

- [ ] **Step 4: Run to verify pass**

Run: `bash tests/run.sh migrate; bash tests/run.sh; bash -n bin/ccp`
Expected: migrate passes; full suite green.

- [ ] **Step 5: Commit** *(authorization required)*

```bash
git add bin/ccp tests/run.sh
git commit -m "feat: auto-migrate dsctl config to ccp profiles"
```

---

### Task 15: `cmd_config`, `cmd_doctor`, `cmd_menu`, `cmd_help`

**Files:**
- Modify: `bin/ccp`

- [ ] **Step 1: Write failing tests** (help/version surface only — menu/doctor are interactive/environmental)

```bash
test_bin_help_mentions_profile() {
  local out; out="$(bash "$ROOT/bin/ccp" help)"
  case "$out" in *"profile"*"path set"*) _pass=$((_pass+1));;
    *) _fail=$((_fail+1)); echo "FAIL: help missing profile/path set" >&2;; esac
}
test_bin_unknown_cmd_exit1() {
  bash "$ROOT/bin/ccp" bogus >/dev/null 2>&1; assert_rc "$?" 1 "unknown cmd exit 1"
}
test_bin_config_defaults() {
  local h; h="$(newdir)"
  local out; out="$(_ccp "$h" config show)"
  case "$out" in *"deepseek"*) _pass=$((_pass+1));;
    *) _fail=$((_fail+1)); echo "FAIL: config show" >&2;; esac
}
```

- [ ] **Step 2: Run to verify they fail**

Run: `bash tests/run.sh 'bin_help\|bin_unknown\|bin_config'`
Expected: FAILs.

- [ ] **Step 3: Add the commands to `bin/ccp`**

```bash
_load_config() {
  CCP_BASE_URL="$CCP_BASE_URL_DEFAULT"; CCP_MODEL_PRO="$CCP_MODEL_PRO_DEFAULT"
  CCP_MODEL_FLASH="$CCP_MODEL_FLASH_DEFAULT"; CCP_EFFORT="$CCP_EFFORT_DEFAULT"
  # parse seguro (no source)
  if [[ -f "$CCP_CONF_FILE" ]]; then
    local line
    while IFS= read -r line; do
      case "$line" in
        base_url=*)    CCP_BASE_URL="${line#base_url=}" ;;
        model_pro=*)   CCP_MODEL_PRO="${line#model_pro=}" ;;
        model_flash=*) CCP_MODEL_FLASH="${line#model_flash=}" ;;
        effort=*)      CCP_EFFORT="${line#effort=}" ;;
      esac
    done < "$CCP_CONF_FILE"
  fi
}
_save_config() {
  mkdir -p "$CCP_HOME"
  { printf 'base_url=%s\n' "$CCP_BASE_URL"; printf 'model_pro=%s\n' "$CCP_MODEL_PRO"
    printf 'model_flash=%s\n' "$CCP_MODEL_FLASH"; printf 'effort=%s\n' "$CCP_EFFORT"; } > "$CCP_CONF_FILE"
  chmod 600 "$CCP_CONF_FILE"
}
cmd_config() {
  _load_config
  case "$1" in
    show|"")
      hr; printf ' %sDefaults para perfiles deepseek nuevos%s\n' "$C_BOLD" "$C_RESET"; hr
      printf ' Base URL:    %s\n' "$CCP_BASE_URL"
      printf ' Modelo pro:  %s  %s(deepseek)%s\n' "$CCP_MODEL_PRO" "$C_DIM" "$C_RESET"
      printf ' Modelo flash:%s\n' "$CCP_MODEL_FLASH"
      printf ' Effort:      %s\n' "$CCP_EFFORT"; hr
      info "Cambiar: ccp config set <base_url|model_pro|model_flash|effort> <valor>" ;;
    set)
      local k="$2" v="$3"; [[ -z "$k" || -z "$v" ]] && { err "Uso: ccp config set <clave> <valor>"; return 1; }
      case "$k" in
        base_url) CCP_BASE_URL="$v";; model_pro) CCP_MODEL_PRO="$v";;
        model_flash) CCP_MODEL_FLASH="$v";; effort) CCP_EFFORT="$v";;
        *) err "Clave desconocida: $k"; return 1;;
      esac
      _save_config; ok "Config: $k = $v" ;;
    reset) rm -f "$CCP_CONF_FILE"; ok "Defaults restaurados." ;;
    *) err "Uso: ccp config [show|set|reset]" ;;
  esac
}

cmd_doctor() {
  hr; printf ' %sDiagnóstico%s\n' "$C_BOLD" "$C_RESET"; hr
  command -v node  >/dev/null 2>&1 && ok "Node.js: $(node --version)" || warn "Node.js no encontrado."
  command -v claude>/dev/null 2>&1 && ok "Claude Code: $(claude --version 2>/dev/null || echo instalado)" || warn "Claude Code no instalado."
  command -v git   >/dev/null 2>&1 && ok "git: $(git --version | awk '{print $3}')" || warn "git no encontrado."
  [[ -f "$_libdir/paths.sh" ]] && ok "Librerías en $_libdir" || err "libs no encontradas."
  type ccp >/dev/null 2>&1 && ok "Función 'ccp' cargada." || warn "Función 'ccp' NO cargada (ccp install && source rc)."
  local name type
  while read -r name; do
    [[ -z "$name" ]] && continue
    type="$(ccp_profile_type "$CCP_HOME" "$name")"
    if [[ "$type" == "official" ]]; then
      [[ -f "$CCP_HOME/profiles/$name/cc-home/.claude.json" ]] && ok "Perfil '$name' (oficial): logueado." || warn "Perfil '$name' (oficial): SIN login (ccp profile login $name)."
    elif [[ "$type" == "deepseek" ]]; then
      [[ -f "$CCP_HOME/profiles/$name/api_key" ]] && ok "Perfil '$name' (deepseek): key OK." || warn "Perfil '$name' (deepseek): SIN key (ccp key $name)."
    fi
  done < <(ccp_profile_list "$CCP_HOME")
  hr
}

cmd_help() {
  cat <<EOF
${C_BOLD}ccp${C_RESET} v$CCP_VERSION — router de perfiles y cuentas de Claude Code

${C_BOLD}TERMINAL (función shell)${C_RESET}
  ccp use <perfil>            activa un perfil en esta terminal
  ccp default | off           vuelve a tu login ~/.claude
  ccp run [cmd]               corre cmd/claude con el perfil del cwd
  ccp on                      (legacy) alias de 'use deepseek'

${C_BOLD}PERFILES${C_RESET}
  ccp profile add <n> --official            crea cuenta oficial (config dir propio)
  ccp profile add <n> --deepseek [opts]     crea provider (--base-url --pro --flash --effort)
  ccp profile login <n>                     abre claude para /login (perfiles oficiales)
  ccp profile list | show <n> | rm <n>
  ccp key <perfil> [API_KEY]                guarda la key de un perfil deepseek

${C_BOLD}REGLAS DE PATH${C_RESET}
  ccp path set <ruta> <perfil>   asigna ruta (y subcarpetas) a un perfil
  ccp path rm <ruta>             quita la regla
  ccp path list | test <ruta> | clear | edit
                                 (gana la regla más específica; sin regla => 'default')

${C_BOLD}SCRIPTING${C_RESET}
  ccp resolve [ruta]          imprime el perfil (exit 0=no-default, 1=default)
  ccp status [--json]         estado de la terminal + regla del cwd

${C_BOLD}OTROS${C_RESET}
  ccp install [rc] | uninstall [rc] | migrate
  ccp config show|set|reset | completion bash|zsh | doctor | menu | version | help

${C_BOLD}EJEMPLO${C_RESET}
  ccp profile add work --official && ccp profile login work
  ccp profile add personal --official && ccp profile login personal
  ccp path set ~/work work
  ccp path set ~/personal personal
  ccp path set ~/labs deepseek        # tras 'ccp profile add deepseek --deepseek' + 'ccp key deepseek'
  cd ~/work && claude                 # arranca como 'work' (auto, por el hook)
EOF
}

cmd_menu() {
  while true; do
    clear 2>/dev/null || true
    printf '%s' "$C_MAG"
    cat <<'BANNER'
   ╔════════════════════════════════════════════╗
   ║   ccp · Claude Code profile router           ║
   ╚════════════════════════════════════════════╝
BANNER
    printf '%s' "$C_RESET"
    printf '   Terminal: %s%s%s   cwd → %s\n\n' "$C_BOLD" "${CCP_PROFILE:-default}" "$C_RESET" "$(ccp_resolve "$PWD" "$CCP_RULES_FILE")"
    say "   ${C_BOLD}1)${C_RESET} Perfiles (list)"
    say "   ${C_BOLD}2)${C_RESET} Reglas de PATH (list)"
    say "   ${C_BOLD}3)${C_RESET} Estado (status)"
    say "   ${C_BOLD}4)${C_RESET} Diagnóstico (doctor)"
    say "   ${C_BOLD}5)${C_RESET} Instalar shell init"
    say "   ${C_BOLD}6)${C_RESET} Ayuda"
    say "   ${C_BOLD}0)${C_RESET} Salir"
    echo; printf '   Opción: '; read -r opt; echo
    case "$opt" in
      1) cmd_profile list ;;
      2) cmd_path_list ;;
      3) cmd_status ;;
      4) cmd_doctor ;;
      5) cmd_install ;;
      6) cmd_help ;;
      0) break ;;
      *) warn "Opción inválida." ;;
    esac
    echo; printf '   %s[Enter para continuar]%s' "$C_DIM" "$C_RESET"; read -r _
  done
}
```

Wire dispatch — add to `main()` case (and set `menu` as default for empty cmd):

```bash
    config) cmd_config "$@" ;;
    doctor) cmd_doctor ;;
    menu)   cmd_menu ;;
    help|--help|-h) cmd_help ;;
    "")     cmd_menu ;;
    *) err "Comando desconocido: '$cmd'"; echo; cmd_help; return 1 ;;
```

- [ ] **Step 4: Run to verify pass**

Run: `bash tests/run.sh 'bin_help\|bin_unknown\|bin_config'; bash tests/run.sh; bash -n bin/ccp`
Expected: all green.

- [ ] **Step 5: Commit** *(authorization required)*

```bash
git add bin/ccp tests/run.sh
git commit -m "feat: ccp config/doctor/menu/help"
```

---

## Phase 7 — Install, CI, docs, end-to-end

### Task 16: Update `install.sh`

**Files:**
- Modify: `install.sh`

- [ ] **Step 1: Rewrite `install.sh`**

```bash
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
ok "Binario  -> $BIN_DIR/ccp"
ok "Librerías-> $LIB_DIR/{paths,profiles,env}.sh"

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
```

- [ ] **Step 2: Syntax check + dry install to temp**

Run:
```bash
bash -n install.sh
CCP_BIN_DIR="$(mktemp -d)" CCP_LIB_DIR="$(mktemp -d)" bash install.sh
```
Expected: prints the install paths, exit 0.

- [ ] **Step 3: Commit** *(authorization required)*

```bash
git add install.sh
git commit -m "build: install ccp binary + three libs"
```

---

### Task 17: Update CI workflow

**Files:**
- Modify: `.github/workflows/ci.yml`

- [ ] **Step 1: Read the current workflow**

Run: `cat .github/workflows/ci.yml`
Expected: see the current shellcheck + smoke-test steps referencing `bin/dsctl` and `lib/paths.sh`.

- [ ] **Step 2: Replace the lint + smoke-test steps**

Set the relevant steps to:

```yaml
      - name: shellcheck
        run: shellcheck -S warning bin/ccp lib/paths.sh lib/profiles.sh lib/env.sh install.sh tests/run.sh || true

      - name: syntax check
        run: bash -n bin/ccp && bash -n lib/paths.sh && bash -n lib/profiles.sh && bash -n lib/env.sh

      - name: test suite
        run: bash tests/run.sh
```

(Keep the rest of the workflow — checkout, runner — as-is. The `|| true` on shellcheck preserves the existing "non-blocking lint" gate documented in CLAUDE.md.)

- [ ] **Step 3: Run the test suite locally to mirror CI**

Run: `bash tests/run.sh`
Expected: `N passed, 0 failed`, exit 0.

- [ ] **Step 4: Commit** *(authorization required)*

```bash
git add .github/workflows/ci.yml
git commit -m "ci: lint+test ccp libs and binary"
```

---

### Task 18: End-to-end manual verification + CLAUDE.md update

**Files:**
- Modify: `CLAUDE.md`

- [ ] **Step 1: End-to-end smoke (manual, against a scratch CCP_HOME)**

Run:
```bash
export CCP_HOME="$(mktemp -d)/ccp"
bash bin/ccp profile add work --official
bash bin/ccp profile add labs --deepseek --base-url https://api.deepseek.com/anthropic --pro 'deepseek-v4-pro[1m]' --flash deepseek-v4-flash --effort max
bash bin/ccp key labs sk-test
bash bin/ccp path set /tmp/workzone work
bash bin/ccp path set /tmp/labzone labs
bash bin/ccp profile list
echo "--- resolve workzone ---"; bash bin/ccp resolve /tmp/workzone/x; echo "rc=$?"
echo "--- resolve labzone ---";  bash bin/ccp resolve /tmp/labzone/x;  echo "rc=$?"
echo "--- env for work ---";     bash bin/ccp _env work
echo "--- hook for labzone ---"; bash bin/ccp _hook /tmp/labzone/x
```
Expected: `work` (rc=0), `labs` (rc=0); `_env work` exports `CLAUDE_CONFIG_DIR=.../profiles/work/cc-home`; `_hook` for labzone exports `ANTHROPIC_BASE_URL` + `ANTHROPIC_AUTH_TOKEN=sk-test`.

- [ ] **Step 2: Update `CLAUDE.md`**

Replace the "What this is", "Commands", and command-name references so they describe `ccp`. Specifically:

- Title line under "## What this is":

```markdown
`ccp` is a Bash CLI that routes Claude Code to a named **profile** per terminal and per directory — an official Anthropic account (its own `CLAUDE_CONFIG_DIR`) or a DeepSeek/compatible provider. Scoping is per-terminal and per-directory, never global. User-facing strings are in Spanish.
```

- The "## Commands" lint/smoke block:

```bash
shellcheck -S warning bin/ccp lib/*.sh install.sh tests/run.sh   # lint (CI gate, non-blocking)
bash -n bin/ccp && bash -n lib/paths.sh && bash -n lib/profiles.sh && bash -n lib/env.sh
bash tests/run.sh                                                # full test suite (CI gate)
```

- Add to the architecture section a note pointing at the three libs and the `ccp _env`/`_hook` eval mechanism, and that `ds`/`dsctl` are superseded (auto-migrated on first `ccp` run; old rc block removable via `dsctl uninstall`).

- [ ] **Step 3: Final full verification**

Run: `bash tests/run.sh && bash -n bin/ccp && shellcheck -S warning bin/ccp lib/*.sh || true`
Expected: test suite green; no syntax errors.

- [ ] **Step 4: Commit** *(authorization required)*

```bash
git add CLAUDE.md
git commit -m "docs: update CLAUDE.md for ccp profile router"
```

---

## Open items carried from spec (resolve during execution)

- **12.2 Non-interactive login:** Task 12's `_profile_login` launches the full `claude` REPL with the right `CLAUDE_CONFIG_DIR` and instructs `/login`. If a future CC version exposes a `claude login` subcommand, swap the last line. No blocker.
- **12.3 Symlink writability:** Task 12 symlinks `plugins/ commands/ agents/ skills/ CLAUDE.md` and **copies** `settings.json`. If CC writes into `plugins/` per-account (it shouldn't for shared marketplace plugins), revisit. Documented, low risk.
- **12.1 settings.json:** decided = copy (Task 12). Revisit only if account-specific approvals must stay isolated *and* synced — out of scope for v1.

## Notes for the executor

- Tests are bash. Run a single group with `bash tests/run.sh <substring>`; run all with `bash tests/run.sh`.
- macOS BSD coreutils: no `grep -P`. The `stat` mode check in Task 5 handles both BSD (`stat -f '%Lp'`) and GNU (`stat -c '%a'`).
- Never `source` profile `meta` or `config` files — they are parsed line-by-line by design (no code execution from config).
- The `ccp` shell function and `_ccp_autocheck` hook `eval` the binary's output; that output is generated from local config and quoted with `%q`. Do not introduce un-quoted interpolation into `lib/env.sh`.
- Honor the repo commit policy: get explicit authorization before each `git commit`; no `Co-Authored-By` trailer.
