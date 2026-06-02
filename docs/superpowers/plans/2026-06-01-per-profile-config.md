# Per-Profile Config Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.
>
> **Commits:** This repo's owner forbids autonomous commits. Each `Commit` step is a *proposal*: stage, show the diff, and ask the user before running `git commit`. Never add a `Co-Authored-By` trailer.

**Goal:** Let each profile carry its own Claude Code config (instructions + hooks/permissions/env/MCP) as a baseline layer, editable from the CLI via a configurable editor.

**Architecture:** Every non-`default` profile owns a `cc-home` (= `CLAUDE_CONFIG_DIR`). The profile keeps two **overlay** files (`overlay/CLAUDE.md`, `overlay/settings.overlay.json`). The effective `cc-home/CLAUDE.md` is a generated file that `@import`s the global `~/.claude/CLAUDE.md` then the overlay md; the effective `cc-home/settings.json` is `jq`-deep-merged from global ⊕ overlay (snapshot-copy fallback when `jq` is absent). Regeneration runs at create/edit/`sync` time — never in the per-prompt hook.

**Tech Stack:** Bash (BSD-portable, macOS), `jq` (soft dependency), Claude Code `@import` memory, the existing `tests/run.sh` harness.

**Reference decisions:** `CONTEXT.md` glossary (*profile config*, *overlay*, *path rule*, *cc-home*) and `docs/adr/0001..0003`.

---

## File Structure

| File | Responsibility | Change |
|------|----------------|--------|
| `lib/cfg.sh` | **New.** Pure overlay+merge engine: overlay paths, init, JSON validate, `jq` merge, `@import` CLAUDE.md generation, regenerate, legacy migration. Explicit `ccp_home`/`src` args — never touches real `~/.config/ccp`. | Create |
| `lib/env.sh` | Emit env delta. Deepseek branch now also exports `CLAUDE_CONFIG_DIR` → its `cc-home`. | Modify `:40-59` |
| `bin/ccp` | Source `cfg.sh`; generalize seeding to `_seed_cc_home`; deepseek `add` seeds a cc-home; new `profile config` / `profile sync`; `config editor`; editor resolver; help/menu/completions. | Modify |
| `install.sh` | Copy the new fourth lib `cfg.sh`. | Modify `:19-23` |
| `tests/run.sh` | Source `cfg.sh`; new `test_cfg_*` and `test_bin_profile_config_*`; update `test_env_deepseek` and `test_seed_official_symlinks`. | Modify |

**Interfaces locked by this plan** (names used across tasks — keep identical):

```
ccp_cfg_overlay_dir   <home> <name>            -> .../profiles/<name>/overlay
ccp_cfg_instr_file    <home> <name>            -> .../overlay/CLAUDE.md
ccp_cfg_settings_file <home> <name>            -> .../overlay/settings.overlay.json
ccp_cfg_cchome        <home> <name>            -> .../profiles/<name>/cc-home
ccp_cfg_init_overlay  <home> <name>            (creates empty CLAUDE.md + '{}' overlay)
ccp_cfg_validate_json <file>                   (rc 0 ok; no jq => rc 0)
ccp_cfg_merge_settings <global> <overlay> <out>
ccp_cfg_write_claude_md <home> <name> <src>
ccp_cfg_regenerate    <home> <name> <src>
ccp_cfg_migrate_legacy <home> <name>
```

---

## Task 1: `lib/cfg.sh` — paths + overlay init

**Files:**
- Create: `lib/cfg.sh`
- Test: `tests/run.sh` (append `test_cfg_*`)

- [ ] **Step 1: Source the new lib in the harness**

In `tests/run.sh`, after line 12 (`source "$ROOT/lib/env.sh"`), add:

```bash
[[ -f "$ROOT/lib/cfg.sh" ]]      && { source "$ROOT/lib/cfg.sh"; }
```

- [ ] **Step 2: Write the failing test**

Append to `tests/run.sh` (before the `# ---- runner ----` block):

```bash
test_cfg_paths() {
  assert_eq "$(ccp_cfg_overlay_dir /h work)"   "/h/profiles/work/overlay" "overlay dir"
  assert_eq "$(ccp_cfg_instr_file /h work)"    "/h/profiles/work/overlay/CLAUDE.md" "instr file"
  assert_eq "$(ccp_cfg_settings_file /h work)" "/h/profiles/work/overlay/settings.overlay.json" "settings file"
  assert_eq "$(ccp_cfg_cchome /h work)"        "/h/profiles/work/cc-home" "cchome"
}
test_cfg_init_overlay() {
  local h; h="$(newdir)"
  ccp_cfg_init_overlay "$h" work
  [[ -f "$h/profiles/work/overlay/CLAUDE.md" ]]; assert_rc "$?" 0 "instr created"
  assert_eq "$(cat "$h/profiles/work/overlay/settings.overlay.json")" "{}" "overlay seeded as {}"
}
```

- [ ] **Step 3: Run to verify it fails**

Run: `bash tests/run.sh cfg_paths`
Expected: FAIL — `ccp_cfg_overlay_dir: command not found` (or empty output).

- [ ] **Step 4: Create `lib/cfg.sh` with paths + init**

```bash
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
```

- [ ] **Step 5: Run to verify it passes**

Run: `bash tests/run.sh cfg_paths` then `bash tests/run.sh cfg_init`
Expected: both PASS (`N passed, 0 failed`).

- [ ] **Step 6: Lint**

Run: `shellcheck -S warning lib/cfg.sh`
Expected: no warnings.

- [ ] **Step 7: Commit (propose, then ask)**

```bash
git add lib/cfg.sh tests/run.sh
git commit -m "feat(cfg): overlay paths + init for profile config"
```

---

## Task 2: `lib/cfg.sh` — JSON validate + settings merge

**Files:**
- Modify: `lib/cfg.sh`
- Test: `tests/run.sh`

- [ ] **Step 1: Write the failing test**

Append to `tests/run.sh`:

```bash
test_cfg_validate_json() {
  command -v jq >/dev/null 2>&1 || { _pass=$((_pass+1)); return; }  # sin jq: validador es no-op
  local d; d="$(newdir)"
  printf '{"a":1}' > "$d/good.json"; printf '{bad' > "$d/bad.json"
  ccp_cfg_validate_json "$d/good.json"; assert_rc "$?" 0 "valid json ok"
  ccp_cfg_validate_json "$d/bad.json";  assert_rc "$?" 1 "invalid json rc1"
}
test_cfg_merge_overlay_wins() {
  command -v jq >/dev/null 2>&1 || { _pass=$((_pass+1)); return; }
  local d; d="$(newdir)"
  printf '{"model":"opus","env":{"A":"1"}}' > "$d/global.json"
  printf '{"env":{"B":"2"},"model":"sonnet"}' > "$d/overlay.json"
  ccp_cfg_merge_settings "$d/global.json" "$d/overlay.json" "$d/out.json"
  assert_eq "$(jq -r '.model' "$d/out.json")" "sonnet" "overlay overrides scalar"
  assert_eq "$(jq -r '.env.A' "$d/out.json")" "1" "global key kept (deep merge)"
  assert_eq "$(jq -r '.env.B' "$d/out.json")" "2" "overlay key added (deep merge)"
}
test_cfg_merge_no_global() {
  local d; d="$(newdir)"
  printf '{"env":{"B":"2"}}' > "$d/overlay.json"
  ccp_cfg_merge_settings "$d/missing.json" "$d/overlay.json" "$d/out.json"
  [[ -s "$d/out.json" ]]; assert_rc "$?" 0 "out written even without global"
  if command -v jq >/dev/null 2>&1; then
    assert_eq "$(jq -r '.env.B' "$d/out.json")" "2" "overlay-only merge"
  fi
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `bash tests/run.sh cfg_merge`
Expected: FAIL — `ccp_cfg_merge_settings: command not found`.

- [ ] **Step 3: Add validate + merge to `lib/cfg.sh`**

Append to `lib/cfg.sh`:

```bash
# valida JSON (rc 0 = válido). Sin jq => no-op (rc 0): no podemos validar.
ccp_cfg_validate_json() { # file
  [[ -f "$1" ]] || return 1
  command -v jq >/dev/null 2>&1 || return 0
  jq -e . "$1" >/dev/null 2>&1
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
    jq -s 'reduce .[] as $x ({}; . * $x)' "${files[@]}" > "$out"
  else
    if [[ -f "$g" ]]; then cp "$g" "$out"; else cp "$o" "$out"; fi
  fi
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `bash tests/run.sh cfg_merge` then `bash tests/run.sh cfg_validate`
Expected: PASS.

- [ ] **Step 5: Lint**

Run: `shellcheck -S warning lib/cfg.sh`
Expected: no warnings.

- [ ] **Step 6: Commit (propose, then ask)**

```bash
git add lib/cfg.sh tests/run.sh
git commit -m "feat(cfg): jq deep-merge for settings + json validation"
```

---

## Task 3: `lib/cfg.sh` — generate `cc-home/CLAUDE.md` (@import) + regenerate

**Files:**
- Modify: `lib/cfg.sh`
- Test: `tests/run.sh`

- [ ] **Step 1: Write the failing test**

Append to `tests/run.sh`:

```bash
test_cfg_write_claude_md_imports() {
  local h; h="$(newdir)"; local src; src="$(newdir)"
  printf 'GLOBAL RULES' > "$src/CLAUDE.md"
  ccp_cfg_init_overlay "$h" work
  ccp_cfg_write_claude_md "$h" work "$src"
  local f="$h/profiles/work/cc-home/CLAUDE.md"
  [[ -f "$f" && ! -L "$f" ]]; assert_rc "$?" 0 "cc-home CLAUDE.md is a real file"
  case "$(cat "$f")" in
    *"@$src/CLAUDE.md"*"@$h/profiles/work/overlay/CLAUDE.md"*) _pass=$((_pass+1));;
    *) _fail=$((_fail+1)); echo "FAIL: missing @imports in cc-home CLAUDE.md" >&2;;
  esac
}
test_cfg_write_claude_md_replaces_symlink() {
  local h; h="$(newdir)"; local src; src="$(newdir)"
  printf 'G' > "$src/CLAUDE.md"
  local cch="$h/profiles/work/cc-home"; mkdir -p "$cch"
  ln -s "$src/CLAUDE.md" "$cch/CLAUDE.md"   # estado viejo: symlink
  ccp_cfg_init_overlay "$h" work
  ccp_cfg_write_claude_md "$h" work "$src"
  [[ ! -L "$cch/CLAUDE.md" ]]; assert_rc "$?" 0 "old symlink replaced by real file"
  assert_eq "$(cat "$src/CLAUDE.md")" "G" "global CLAUDE.md NOT clobbered"
}
test_cfg_regenerate() {
  local h; h="$(newdir)"; local src; src="$(newdir)"
  printf 'G' > "$src/CLAUDE.md"; printf '{"model":"opus"}' > "$src/settings.json"
  ccp_cfg_regenerate "$h" work "$src"
  local cch="$h/profiles/work/cc-home"
  [[ -f "$cch/CLAUDE.md" ]]; assert_rc "$?" 0 "regenerate writes CLAUDE.md"
  [[ -f "$cch/settings.json" ]]; assert_rc "$?" 0 "regenerate writes settings.json"
  if command -v jq >/dev/null 2>&1; then
    assert_eq "$(jq -r '.model' "$cch/settings.json")" "opus" "global merged into cc-home settings"
  fi
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `bash tests/run.sh cfg_write_claude` then `bash tests/run.sh cfg_regenerate`
Expected: FAIL — functions not defined.

- [ ] **Step 3: Add generators to `lib/cfg.sh`**

Append to `lib/cfg.sh`:

```bash
# escribe cc-home/CLAUDE.md: header + @import del global (si existe) + @import del overlay.
# IMPORTANTE: si cc-home/CLAUDE.md es un symlink viejo, se elimina antes de escribir
# (escribir sobre un symlink corromper­ía el archivo global apuntado).
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
```

- [ ] **Step 4: Run to verify it passes**

Run: `bash tests/run.sh cfg_write_claude` then `bash tests/run.sh cfg_regenerate`
Expected: PASS.

- [ ] **Step 5: Lint**

Run: `shellcheck -S warning lib/cfg.sh`
Expected: no warnings.

- [ ] **Step 6: Commit (propose, then ask)**

```bash
git add lib/cfg.sh tests/run.sh
git commit -m "feat(cfg): generate cc-home CLAUDE.md via @import + regenerate"
```

---

## Task 4: `lib/cfg.sh` — legacy migration of existing official profiles

**Files:**
- Modify: `lib/cfg.sh`
- Test: `tests/run.sh`

- [ ] **Step 1: Write the failing test**

Append to `tests/run.sh`:

```bash
test_cfg_migrate_legacy() {
  local h; h="$(newdir)"
  local cch="$h/profiles/work/cc-home"; mkdir -p "$cch"
  # estado viejo: settings.json copia (archivo real) + CLAUDE.md symlink
  printf '{"hooks":{"X":1}}' > "$cch/settings.json"
  printf 'G' > "$h/global-claude.md"
  ln -s "$h/global-claude.md" "$cch/CLAUDE.md"
  ccp_cfg_migrate_legacy "$h" work
  local ov="$h/profiles/work/overlay/settings.overlay.json"
  [[ -f "$ov" ]]; assert_rc "$?" 0 "old settings.json moved into overlay"
  [[ ! -e "$cch/settings.json" ]]; assert_rc "$?" 0 "old cc-home settings.json removed"
  [[ ! -L "$cch/CLAUDE.md" ]]; assert_rc "$?" 0 "old CLAUDE.md symlink removed"
  if command -v jq >/dev/null 2>&1; then
    assert_eq "$(jq -r '.hooks.X' "$ov")" "1" "edits preserved in overlay"
  fi
}
test_cfg_migrate_legacy_idempotent() {
  local h; h="$(newdir)"
  ccp_cfg_init_overlay "$h" work
  printf '{"keep":1}' > "$h/profiles/work/overlay/settings.overlay.json"
  ccp_cfg_migrate_legacy "$h" work   # ya migrado: no debe pisar el overlay
  if command -v jq >/dev/null 2>&1; then
    assert_eq "$(jq -r '.keep' "$h/profiles/work/overlay/settings.overlay.json")" "1" "overlay untouched when already migrated"
  else
    _pass=$((_pass+1))
  fi
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `bash tests/run.sh cfg_migrate_legacy`
Expected: FAIL — `ccp_cfg_migrate_legacy: command not found`.

- [ ] **Step 3: Add migration to `lib/cfg.sh`**

Append to `lib/cfg.sh`:

```bash
# convierte un cc-home viejo al modelo overlay. Idempotente: solo actúa si
# detecta el estado viejo (settings.json copia real + CLAUDE.md symlink).
ccp_cfg_migrate_legacy() { # home name
  local home="$1" name="$2"
  local cch overlay_s
  cch="$(ccp_cfg_cchome "$home" "$name")"
  overlay_s="$(ccp_cfg_settings_file "$home" "$name")"
  mkdir -p "$(ccp_cfg_overlay_dir "$home" "$name")"
  # settings.json copia (archivo real, no symlink) y sin overlay aún => muévelo
  if [[ -f "$cch/settings.json" && ! -L "$cch/settings.json" && ! -f "$overlay_s" ]]; then
    mv "$cch/settings.json" "$overlay_s"
  fi
  # CLAUDE.md symlink viejo => quítalo (se regenerará como @import)
  [[ -L "$cch/CLAUDE.md" ]] && rm -f "$cch/CLAUDE.md"
  return 0
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `bash tests/run.sh cfg_migrate_legacy`
Expected: PASS (both tests).

- [ ] **Step 5: Lint**

Run: `shellcheck -S warning lib/cfg.sh`
Expected: no warnings.

- [ ] **Step 6: Commit (propose, then ask)**

```bash
git add lib/cfg.sh tests/run.sh
git commit -m "feat(cfg): migrate legacy official cc-home to overlay model"
```

---

## Task 5: `lib/env.sh` — deepseek exports its `cc-home` as `CLAUDE_CONFIG_DIR`

**Files:**
- Modify: `lib/env.sh:40-59` (deepseek branch)
- Test: `tests/run.sh:139-148` (update `test_env_deepseek`)

- [ ] **Step 1: Update the test to the new behavior**

In `tests/run.sh`, replace the tail of `test_env_deepseek` (the two `cfg`/`CLAUDE_CONFIG_DIR` lines `146-147`):

```bash
  local cfg; cfg="$(eval "$out"; printf '%s' "${CLAUDE_CONFIG_DIR:-NONE}")"
  assert_eq "$cfg" "$h/profiles/ds/cc-home" "deepseek now exports its cc-home as CLAUDE_CONFIG_DIR"
```

- [ ] **Step 2: Run to verify it fails**

Run: `bash tests/run.sh env_deepseek`
Expected: FAIL — got `NONE`, want `.../profiles/ds/cc-home`.

- [ ] **Step 3: Add the export to the deepseek branch**

In `lib/env.sh`, inside `case "$type" in` → `deepseek)`, immediately after the `local base_url ...` declarations and before `printf 'export ANTHROPIC_BASE_URL...`, add:

```bash
      printf 'export CLAUDE_CONFIG_DIR=%q\n' "$home/profiles/$profile/cc-home"
```

(Place it as the first `printf` in the deepseek branch so the var is set alongside the provider vars.)

- [ ] **Step 4: Run to verify it passes**

Run: `bash tests/run.sh env_deepseek` then `bash tests/run.sh env`
Expected: PASS (and `test_env_official`, `test_env_default` still pass).

- [ ] **Step 5: Lint**

Run: `shellcheck -S warning lib/env.sh`
Expected: no warnings.

- [ ] **Step 6: Commit (propose, then ask)**

```bash
git add lib/env.sh tests/run.sh
git commit -m "feat(env): deepseek profiles export their cc-home as CLAUDE_CONFIG_DIR"
```

---

## Task 6: `bin/ccp` — source `cfg.sh` + generalize seeding to `_seed_cc_home`

**Files:**
- Modify: `bin/ccp:16` (lib loop), `bin/ccp:159-175` (`_seed_official_home` → `_seed_cc_home`), `bin/ccp:96-108` (`_profile_add`)
- Test: `tests/run.sh:241-253` (update `test_seed_official_symlinks`), add deepseek-seed test

- [ ] **Step 1: Update the seeding test to overlay model**

In `tests/run.sh`, replace `test_seed_official_symlinks` body lines that assert CLAUDE.md/settings (the `CLAUDE.md symlinked` + `settings.json copied` assertions, `250-251`) with:

```bash
  [[ -f "$cch/CLAUDE.md" && ! -L "$cch/CLAUDE.md" ]]; assert_rc "$?" 0 "CLAUDE.md is generated real file"
  case "$(cat "$cch/CLAUDE.md")" in *"@$src/CLAUDE.md"*) _pass=$((_pass+1));;
    *) _fail=$((_fail+1)); echo "FAIL: cc-home CLAUDE.md missing global @import" >&2;; esac
  [[ -f "$cch/settings.json" && ! -L "$cch/settings.json" ]]; assert_rc "$?" 0 "settings.json generated not linked"
  [[ -f "$h/profiles/work/overlay/settings.overlay.json" ]]; assert_rc "$?" 0 "overlay seeded"
```

Then append a new test:

```bash
test_seed_deepseek_gets_cchome() {
  local h; h="$(newdir)"; local src; src="$(newdir)"
  mkdir -p "$src/plugins"; printf 'G' > "$src/CLAUDE.md"; printf '{"m":1}' > "$src/settings.json"
  CCP_HOME="$h" CCP_CLAUDE_SRC="$src" bash "$ROOT/bin/ccp" profile add ds --deepseek --base-url u --pro p --flash f --effort max >/dev/null
  local cch="$h/profiles/ds/cc-home"
  [[ -L "$cch/plugins" ]]; assert_rc "$?" 0 "deepseek cc-home has symlinked plugins"
  [[ -f "$cch/CLAUDE.md" && -f "$cch/settings.json" ]]; assert_rc "$?" 0 "deepseek cc-home generated config"
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `bash tests/run.sh seed`
Expected: FAIL — deepseek has no cc-home yet; official CLAUDE.md still a symlink.

- [ ] **Step 3: Source `cfg.sh` in the binary**

In `bin/ccp:16`, change the lib loop to include `cfg.sh`:

```bash
for _l in paths.sh profiles.sh env.sh cfg.sh; do
```

- [ ] **Step 4: Replace `_seed_official_home` with `_seed_cc_home`**

Replace the whole `_seed_official_home` function (`bin/ccp:159-175`) with:

```bash
# Siembra un cc-home: symlinks de lo compartible + genera config (overlay model).
# Fuente override-able para tests: CCP_CLAUDE_SRC (default ~/.claude).
_seed_cc_home() { # home name
  local home="$1" name="$2"
  local cch; cch="$(ccp_cfg_cchome "$home" "$name")"
  local src="${CCP_CLAUDE_SRC:-$HOME/.claude}"
  mkdir -p "$cch"
  if [[ -d "$src" ]]; then
    local item
    for item in plugins commands agents skills; do
      if [[ -e "$src/$item" && ! -e "$cch/$item" ]]; then
        ln -s "$src/$item" "$cch/$item"
      fi
    done
  fi
  ccp_cfg_init_overlay "$home" "$name"
  ccp_cfg_regenerate "$home" "$name" "$src"
}
```

(Note: CLAUDE.md and settings.json are no longer symlinked/copied here — they are generated by `ccp_cfg_regenerate`.)

- [ ] **Step 5: Update `_profile_add` to seed both types**

In `bin/ccp:96-107`, change the `case "$kind"` block to:

```bash
  case "$kind" in
    official)
      ccp_profile_add_official "$CCP_HOME" "$name"
      _seed_cc_home "$CCP_HOME" "$name"
      ok "Perfil oficial '$name' creado (plugins/skills symlinked, config generada)."
      info "Loguéate una vez:  ccp profile login $name   (corre /login dentro)" ;;
    deepseek)
      ccp_profile_add_deepseek "$CCP_HOME" "$name" "$base" "$pro" "$flash" "$effort"
      _seed_cc_home "$CCP_HOME" "$name"
      ok "Perfil deepseek '$name' creado (cc-home + config generada)."
      info "Añade su API key:  ccp key $name" ;;
    *) err "Especifica --official o --deepseek"; return 1 ;;
  esac
```

- [ ] **Step 6: Run to verify it passes**

Run: `bash tests/run.sh seed` then `bash tests/run.sh` (full suite)
Expected: PASS, `0 failed`.

- [ ] **Step 7: Lint + syntax**

Run: `shellcheck -S warning bin/ccp` and `bash -n bin/ccp`
Expected: no warnings; parses.

- [ ] **Step 8: Commit (propose, then ask)**

```bash
git add bin/ccp tests/run.sh
git commit -m "feat(ccp): seed managed cc-home for official + deepseek via overlay model"
```

---

## Task 7: `bin/ccp` — editor resolver + `ccp config editor`

**Files:**
- Modify: `bin/ccp:439-481` (`_load_config`, `_save_config`, `cmd_config`)
- Test: `tests/run.sh`

- [ ] **Step 1: Write the failing test**

Append to `tests/run.sh`:

```bash
test_bin_config_editor_set_show() {
  local h; h="$(newdir)"
  _ccp "$h" config editor "code -w" >/dev/null
  local out; out="$(_ccp "$h" config show)"
  case "$out" in *"code -w"*) _pass=$((_pass+1));;
    *) _fail=$((_fail+1)); echo "FAIL: config show missing editor" >&2;; esac
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `bash tests/run.sh config_editor`
Expected: FAIL — editor not stored/shown.

- [ ] **Step 3: Add `editor` to config load/save**

In `bin/ccp`, in `_load_config` (`:439-453`), add a default and a parse case. After `CCP_EFFORT="$CCP_EFFORT_DEFAULT"` add:

```bash
  CCP_EDITOR=""
```

and inside the `while ... case "$line"` block add:

```bash
        editor=*)      CCP_EDITOR="${line#editor=}" ;;
```

In `_save_config` (`:454-459`), add an `editor` line to the written block:

```bash
    printf 'effort=%s\n' "$CCP_EFFORT"; printf 'editor=%s\n' "$CCP_EDITOR"; } > "$CCP_CONF_FILE"
```

- [ ] **Step 4: Add `editor` subcommand + show line to `cmd_config`**

In `cmd_config` (`:460-481`), add an `editor` line to the `show)` block (after the `Effort:` printf):

```bash
      printf ' Editor:      %s\n' "${CCP_EDITOR:-(usa \$EDITOR)}"; hr
```

(remove the now-duplicate `hr` that previously ended the `show)` block) and add a new case before `*)`:

```bash
    editor)
      local e="$2"; [[ -z "$e" ]] && { err "Uso: ccp config editor <comando>"; return 1; }
      CCP_EDITOR="$e"; _save_config; ok "Editor: $e" ;;
```

- [ ] **Step 5: Add the editor resolver helper**

After `cmd_config` (around `:481`), add:

```bash
# editor configurable (ccp config editor ...) -> $EDITOR -> nano
_resolve_editor() { _load_config; printf '%s' "${CCP_EDITOR:-${EDITOR:-nano}}"; }
```

- [ ] **Step 6: Run to verify it passes**

Run: `bash tests/run.sh config` then `bash tests/run.sh` (full)
Expected: PASS, `0 failed` (existing `test_bin_config_defaults` / `test_bin_config_set_used_by_profile_add` still green).

- [ ] **Step 7: Lint + syntax**

Run: `shellcheck -S warning bin/ccp` and `bash -n bin/ccp`
Expected: clean.

- [ ] **Step 8: Commit (propose, then ask)**

```bash
git add bin/ccp tests/run.sh
git commit -m "feat(ccp): configurable editor (ccp config editor) with \$EDITOR fallback"
```

---

## Task 8: `bin/ccp` — `ccp profile config` (menu + direct targets + default)

**Files:**
- Modify: `bin/ccp:64-75` (`cmd_profile` dispatch), add `_profile_config` / `_edit_one` / `_edit_settings` / `_config_default`
- Test: `tests/run.sh`

- [ ] **Step 1: Write the failing test**

Append to `tests/run.sh` (uses a fake editor that writes JSON to the file it's given):

```bash
test_bin_profile_config_settings_target() {
  local h; h="$(newdir)"; local src; src="$(newdir)"
  printf 'G' > "$src/CLAUDE.md"; printf '{"model":"opus"}' > "$src/settings.json"
  CCP_HOME="$h" CCP_CLAUDE_SRC="$src" bash "$ROOT/bin/ccp" profile add work --official >/dev/null
  # editor falso: escribe un overlay válido en el archivo recibido
  local fe; fe="$(newdir)/fakeeditor"
  printf '#!/usr/bin/env bash\nprintf %s '"'"'{"env":{"FOO":"bar"}}'"'"' > "$1"\n' > "$fe"; chmod +x "$fe"
  CCP_HOME="$h" CCP_CLAUDE_SRC="$src" bash "$ROOT/bin/ccp" config editor "$fe" >/dev/null
  CCP_HOME="$h" CCP_CLAUDE_SRC="$src" bash "$ROOT/bin/ccp" profile config work settings >/dev/null
  local ov="$h/profiles/work/overlay/settings.overlay.json"
  case "$(cat "$ov")" in *FOO*) _pass=$((_pass+1));;
    *) _fail=$((_fail+1)); echo "FAIL: overlay not edited" >&2;; esac
  if command -v jq >/dev/null 2>&1; then
    assert_eq "$(jq -r '.env.FOO' "$h/profiles/work/cc-home/settings.json")" "bar" "overlay merged into cc-home after edit"
    assert_eq "$(jq -r '.model' "$h/profiles/work/cc-home/settings.json")" "opus" "global still present after merge"
  fi
}
test_bin_profile_config_bad_json_keeps_last_good() {
  local h; h="$(newdir)"; local src; src="$(newdir)"
  printf '{"model":"opus"}' > "$src/settings.json"
  CCP_HOME="$h" CCP_CLAUDE_SRC="$src" bash "$ROOT/bin/ccp" profile add work --official >/dev/null
  command -v jq >/dev/null 2>&1 || { _pass=$((_pass+1)); return; }  # sin jq no hay validación
  local good="$h/profiles/work/cc-home/settings.json"; cp "$good" "$h/snap.json"
  local fe; fe="$(newdir)/fakeeditor"
  printf '#!/usr/bin/env bash\nprintf %s '"'"'{bad json'"'"' > "$1"\n' > "$fe"; chmod +x "$fe"
  CCP_HOME="$h" CCP_CLAUDE_SRC="$src" bash "$ROOT/bin/ccp" config editor "$fe" >/dev/null
  CCP_HOME="$h" CCP_CLAUDE_SRC="$src" bash "$ROOT/bin/ccp" profile config work settings >/dev/null 2>&1
  assert_eq "$(cat "$good")" "$(cat "$h/snap.json")" "bad json: cc-home settings.json unchanged"
}
test_bin_profile_config_no_tty_requires_target() {
  local h; h="$(newdir)"
  _ccp "$h" profile add work --official >/dev/null
  local rc; _ccp "$h" profile config work </dev/null >/dev/null 2>&1; rc=$?
  assert_rc "$rc" 1 "no tty + no target => error"
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `bash tests/run.sh profile_config`
Expected: FAIL — `profile config` unknown subcommand.

- [ ] **Step 3: Add `config` + `sync` to the dispatcher**

In `cmd_profile` (`:67-74`), add two cases before the `*)` fallback:

```bash
    config)   _profile_config "$@" ;;
    sync)     _profile_sync "$@" ;;
```

and update the usage hint string to: `say "Usa: add | rm | list | show | login | config | sync"`.

- [ ] **Step 4: Implement `_profile_config` and helpers**

Add after `_profile_login` (around `:187`):

```bash
_profile_config() {
  local name="$1" target="$2"
  [[ -z "$name" ]] && { err "Uso: ccp profile config <perfil> [instructions|settings]"; return 1; }
  if [[ "$name" == "default" ]]; then _config_default; return; fi
  ccp_profile_exists "$CCP_HOME" "$name" || { err "No existe '$name'."; return 1; }
  ccp_cfg_init_overlay "$CCP_HOME" "$name"
  ccp_cfg_migrate_legacy "$CCP_HOME" "$name"
  local instr settings
  instr="$(ccp_cfg_instr_file "$CCP_HOME" "$name")"
  settings="$(ccp_cfg_settings_file "$CCP_HOME" "$name")"
  case "$target" in
    instructions|instr|md) _edit_one "$instr" "$name"; return ;;
    settings|json)         _edit_settings "$settings" "$name"; return ;;
    "")                    : ;;
    *) err "target desconocido: '$target' (instructions|settings)"; return 1 ;;
  esac
  if [[ ! -t 0 ]]; then err "Sin TTY: especifica target (instructions|settings)."; return 1; fi
  while true; do
    hr; printf ' %sConfig de perfil: %s%s\n' "$C_BOLD" "$name" "$C_RESET"; hr
    say "   ${C_BOLD}1)${C_RESET} Instrucciones (CLAUDE.md)"
    say "   ${C_BOLD}2)${C_RESET} Settings (hooks/permisos/env/mcp)"
    say "   ${C_BOLD}3)${C_RESET} Ambos"
    say "   ${C_BOLD}0)${C_RESET} Salir"
    printf '   Opción: '; read -r opt
    case "$opt" in
      1) _edit_one "$instr" "$name" ;;
      2) _edit_settings "$settings" "$name" ;;
      3) _edit_one "$instr" "$name"; _edit_settings "$settings" "$name" ;;
      0|"") break ;;
      *) warn "Opción inválida." ;;
    esac
  done
}

# edita instrucciones (sin validación) y regenera cc-home.
_edit_one() { # file name
  local ed; ed="$(_resolve_editor)"
  # shellcheck disable=SC2086
  ${ed:-nano} "$1"
  ccp_cfg_regenerate "$CCP_HOME" "$2" "${CCP_CLAUDE_SRC:-$HOME/.claude}"
  ok "Instrucciones de '$2' actualizadas."
}

# edita settings overlay; valida JSON; solo regenera si es válido.
_edit_settings() { # file name
  local ed; ed="$(_resolve_editor)"
  # shellcheck disable=SC2086
  ${ed:-nano} "$1"
  if ccp_cfg_validate_json "$1"; then
    ccp_cfg_regenerate "$CCP_HOME" "$2" "${CCP_CLAUDE_SRC:-$HOME/.claude}"
    ok "Settings de '$2' aplicados."
  else
    warn "JSON inválido en $1 — NO regeneré cc-home (se conserva el último bueno)."
    warn "Reedita:  ccp profile config $2 settings"
  fi
}

# 'default' = ~/.claude puro: abre los archivos GLOBALES con aviso.
_config_default() {
  local src="${CCP_CLAUDE_SRC:-$HOME/.claude}"
  warn "default = tu config GLOBAL ($src). Editas archivos que afectan a TODOS los perfiles que heredan."
  local ed; ed="$(_resolve_editor)"
  # shellcheck disable=SC2086
  ${ed:-nano} "$src/CLAUDE.md" "$src/settings.json"
}
```

- [ ] **Step 5: Run to verify it passes**

Run: `bash tests/run.sh profile_config` then `bash tests/run.sh` (full)
Expected: PASS, `0 failed`.

- [ ] **Step 6: Lint + syntax**

Run: `shellcheck -S warning bin/ccp` and `bash -n bin/ccp`
Expected: clean (the two `# shellcheck disable=SC2086` are intentional for editor word-splitting).

- [ ] **Step 7: Commit (propose, then ask)**

```bash
git add bin/ccp tests/run.sh
git commit -m "feat(ccp): ccp profile config (menu + targets) with json-validated apply"
```

---

## Task 9: `bin/ccp` — `ccp profile sync`

**Files:**
- Modify: `bin/ccp` (add `_profile_sync`)
- Test: `tests/run.sh`

- [ ] **Step 1: Write the failing test**

Append to `tests/run.sh`:

```bash
test_bin_profile_sync_repulls_global() {
  local h; h="$(newdir)"; local src; src="$(newdir)"
  printf '{"model":"opus"}' > "$src/settings.json"; printf 'G' > "$src/CLAUDE.md"
  CCP_HOME="$h" CCP_CLAUDE_SRC="$src" bash "$ROOT/bin/ccp" profile add work --official >/dev/null
  # cambia el global DESPUÉS de crear el perfil
  printf '{"model":"sonnet"}' > "$src/settings.json"
  CCP_HOME="$h" CCP_CLAUDE_SRC="$src" bash "$ROOT/bin/ccp" profile sync work >/dev/null
  if command -v jq >/dev/null 2>&1; then
    assert_eq "$(jq -r '.model' "$h/profiles/work/cc-home/settings.json")" "sonnet" "sync re-pulled global change"
  else
    _pass=$((_pass+1))
  fi
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `bash tests/run.sh profile_sync`
Expected: FAIL — `sync` unknown subcommand (or no change).

- [ ] **Step 3: Implement `_profile_sync`**

Add after `_config_default`:

```bash
# re-merge global ⊕ overlay para un perfil (o todos si no se da nombre).
_profile_sync() {
  local name="$1" src="${CCP_CLAUDE_SRC:-$HOME/.claude}"
  if [[ -n "$name" ]]; then
    ccp_profile_exists "$CCP_HOME" "$name" || { err "No existe '$name'."; return 1; }
    ccp_cfg_migrate_legacy "$CCP_HOME" "$name"
    ccp_cfg_regenerate "$CCP_HOME" "$name" "$src"
    ok "Perfil '$name' re-sincronizado (global ⊕ overlay)."
    return 0
  fi
  local n
  while read -r n; do
    [[ -z "$n" ]] && continue
    ccp_cfg_migrate_legacy "$CCP_HOME" "$n"
    ccp_cfg_regenerate "$CCP_HOME" "$n" "$src"
  done < <(ccp_profile_list "$CCP_HOME")
  ok "Todos los perfiles re-sincronizados."
}
```

(Dispatch case `sync)` was already added in Task 8 Step 3.)

- [ ] **Step 4: Run to verify it passes**

Run: `bash tests/run.sh profile_sync` then `bash tests/run.sh` (full)
Expected: PASS, `0 failed`.

- [ ] **Step 5: Lint + syntax**

Run: `shellcheck -S warning bin/ccp` and `bash -n bin/ccp`
Expected: clean.

- [ ] **Step 6: Commit (propose, then ask)**

```bash
git add bin/ccp tests/run.sh
git commit -m "feat(ccp): ccp profile sync re-merges global into profile cc-homes"
```

---

## Task 10: `bin/ccp` — help, completions, menu surface

**Files:**
- Modify: `bin/ccp:362-363` + `:382-383` (completions), `bin/ccp:503-542` (help), `bin/ccp:544-575` (menu)
- Test: `tests/run.sh` (extend `test_bin_help_mentions_profile`)

- [ ] **Step 1: Update the help test**

In `tests/run.sh`, change `test_bin_help_mentions_profile` (`:289-294`) match pattern to also require `config`:

```bash
  case "$out" in *"profile config"*"path set"*) _pass=$((_pass+1));;
    *) _fail=$((_fail+1)); echo "FAIL: help missing profile config/path set" >&2;; esac
```

- [ ] **Step 2: Run to verify it fails**

Run: `bash tests/run.sh help`
Expected: FAIL — help lacks `profile config`.

- [ ] **Step 3: Add help lines**

In `cmd_help` PERFILES block (`:513-518`), add after the `profile login` line:

```bash
  ccp profile config <n> [instructions|settings]  edita la config del perfil (editor)
  ccp profile sync [<n>]                    re-mergea el global en el/los cc-home
```

In the OTROS block (`:530-532`), update the config line to mention `editor`:

```bash
  ccp config show|set|reset|editor <cmd> | completion bash|zsh | doctor | menu | version | help
```

- [ ] **Step 4: Update bash + zsh completions**

In bash `_ccp` (`:362`), extend the profile subcommands:

```bash
    profile) [[ $COMP_CWORD -eq 2 ]] && COMPREPLY=( $(compgen -W "add rm list show login config sync" -- "$cur") )
             [[ $COMP_CWORD -eq 3 && "${COMP_WORDS[2]}" =~ ^(rm|show|login|config|sync)$ ]] && COMPREPLY=( $(compgen -W "default $(ccp profile list 2>/dev/null)" -- "$cur") )
             [[ $COMP_CWORD -eq 4 && "${COMP_WORDS[2]}" == "config" ]] && COMPREPLY=( $(compgen -W "instructions settings" -- "$cur") ) ;;
```

In zsh `_ccp` (`:382-383`), extend likewise:

```bash
    profile) (( CURRENT == 3 )) && compadd -- add rm list show login config sync
             (( CURRENT == 4 )) && [[ "${words[3]}" =~ ^(rm|show|login|config|sync)$ ]] && compadd -- default ${(f)"$(ccp profile list 2>/dev/null)"}
             (( CURRENT == 5 )) && [[ "${words[3]}" == config ]] && compadd -- instructions settings ;;
```

- [ ] **Step 5: Add a menu entry (optional surface)**

In `cmd_menu`, add a menu line after option `2)` and a `case` handler. Add to the printed list:

```bash
    say "   ${C_BOLD}7)${C_RESET} Editar config de un perfil"
```

and in the `case "$opt"` add:

```bash
      7) printf '   Perfil: '; read -r _pn; [[ -n "$_pn" ]] && _profile_config "$_pn" ;;
```

- [ ] **Step 6: Run the full suite + lint**

Run: `bash tests/run.sh`
Expected: PASS, `0 failed`.
Run: `shellcheck -S warning bin/ccp` and `bash -n bin/ccp`
Expected: clean.

- [ ] **Step 7: Commit (propose, then ask)**

```bash
git add bin/ccp tests/run.sh
git commit -m "feat(ccp): surface profile config/sync in help, completions, menu"
```

---

## Task 11: `install.sh` — install the fourth lib

**Files:**
- Modify: `install.sh:19-23`

- [ ] **Step 1: Add `cfg.sh` to the install list**

In `install.sh`, after the `env.sh` install line (`:21`), add:

```bash
install -m 0644 "$SRC_DIR/lib/cfg.sh"      "$LIB_DIR/cfg.sh"
```

and update the summary line (`:23`):

```bash
ok "Librerías-> $LIB_DIR/{paths,profiles,env,cfg}.sh"
```

- [ ] **Step 2: Lint + syntax**

Run: `shellcheck -S warning install.sh` and `bash -n install.sh`
Expected: clean.

- [ ] **Step 3: Smoke-test install into a temp prefix**

Run:
```bash
CCP_BIN_DIR="$(mktemp -d)" CCP_LIB_DIR="$(mktemp -d)" bash install.sh
```
Expected: prints `Librerías-> .../{paths,profiles,env,cfg}.sh`; the temp `CCP_LIB_DIR` contains `cfg.sh`.

- [ ] **Step 4: Commit (propose, then ask)**

```bash
git add install.sh
git commit -m "build: install lib/cfg.sh alongside the other libs"
```

---

## Task 12: Docs — CLAUDE.md + README sync

**Files:**
- Modify: `CLAUDE.md` (Architecture → Libraries; Config & state locations)
- Modify: `README.md` (add a "Config por perfil" section)

- [ ] **Step 1: Update `CLAUDE.md` Libraries list**

Add a bullet under `### Libraries` describing `lib/cfg.sh`:

```markdown
- **`lib/cfg.sh`** — profile-config overlay engine. Each non-`default` profile owns a `cc-home`; its *overlay* (`overlay/CLAUDE.md`, `overlay/settings.overlay.json`) is the profile's own contribution. `ccp_cfg_regenerate` writes `cc-home/CLAUDE.md` as `@import`s of global + overlay, and `cc-home/settings.json` as a `jq` deep-merge of global ⊕ overlay (snapshot-copy fallback when `jq` is absent). Regen runs at create/edit/`sync` — never in the hook. `ccp_cfg_migrate_legacy` converts pre-overlay official cc-homes (copied `settings.json` → overlay, symlinked `CLAUDE.md` → generated `@import`).
```

In `### Config & state locations`, add:

```markdown
- `profiles/<name>/overlay/` — the profile's own *profile config*: `CLAUDE.md` (instructions, `@import`ed by `cc-home/CLAUDE.md`) and `settings.overlay.json` (hooks/permissions/env/MCP, `jq`-merged into `cc-home/settings.json`).
```

Note in the deepseek line of `lib/env.sh` description that deepseek now also exports `CLAUDE_CONFIG_DIR`.

- [ ] **Step 2: Add a README section**

In `README.md`, add a section (Spanish, matching tone) documenting:

```markdown
## Config por perfil

Cada perfil (official o deepseek) tiene su propia config de Claude que se aplica como **capa baseline** cuando el perfil está activo:

    ccp profile config <perfil>                 # menú: instrucciones / settings / ambos
    ccp profile config <perfil> instructions    # abre overlay/CLAUDE.md
    ccp profile config <perfil> settings        # abre overlay/settings.overlay.json
    ccp profile sync [<perfil>]                 # re-mergea cambios del global ~/.claude
    ccp config editor "code -w"                 # editor a usar (fallback: $EDITOR)

- **Instrucciones**: `cc-home/CLAUDE.md` hace `@import` del global `~/.claude/CLAUDE.md` y luego de tu overlay.
- **Settings**: `cc-home/settings.json` = global ⊕ overlay (deep-merge con `jq`; sin `jq` cae a snapshot del global).
- **Prioridad real**: es una baseline — la config del repo (`.claude/settings.json`) gana en conflicto; las instrucciones se inyectan siempre como contexto. Ver `docs/adr/0001`.
- `default` no tiene overlay: `ccp profile config default` abre tu `~/.claude` global directo (con aviso).
```

- [ ] **Step 3: Verify no test breakage**

Run: `bash tests/run.sh`
Expected: PASS, `0 failed` (docs-only; suite unaffected).

- [ ] **Step 4: Commit (propose, then ask)**

```bash
git add CLAUDE.md README.md
git commit -m "docs: document per-profile config (overlay, sync, editor)"
```

---

## Final verification

- [ ] **Full lint gate**

Run: `shellcheck -S warning bin/ccp lib/*.sh install.sh tests/run.sh`
Expected: no warnings.

- [ ] **Syntax gate**

Run: `bash -n bin/ccp && bash -n lib/paths.sh && bash -n lib/profiles.sh && bash -n lib/env.sh && bash -n lib/cfg.sh`
Expected: all parse.

- [ ] **Full test suite**

Run: `bash tests/run.sh`
Expected: `N passed, 0 failed`.

- [ ] **Manual smoke (real ccp, temp CCP_HOME)**

```bash
H="$(mktemp -d)/ccp"
CCP_HOME="$H" bash bin/ccp profile add demo --official
CCP_HOME="$H" EDITOR=true bash bin/ccp profile config demo settings   # opens (true = no-op editor)
cat "$H/profiles/demo/cc-home/CLAUDE.md"        # shows @import lines
cat "$H/profiles/demo/cc-home/settings.json"    # shows merged settings
```
Expected: cc-home has generated `CLAUDE.md` (with `@import`) + `settings.json`; `overlay/` has `CLAUDE.md` + `settings.overlay.json`.

---

## Self-Review notes (author)

- **Spec coverage:** instructions ✅ (Task 3, @import), hooks/permissions/env/MCP ✅ (all live in `settings.overlay.json`, Tasks 2/8), inherit global + own ✅ (merge + @import), CLI-editable via editor ✅ (Tasks 7/8), configurable editor ✅ (Task 7), all profiles incl. deepseek ✅ (Task 6), default untouched ✅ (Task 8 `_config_default`), baseline semantics ✅ (ADR-0001, README), regen at edit/create/sync ✅ (no `jq` in hook), legacy migration ✅ (Task 4), bad-JSON safety ✅ (Task 8).
- **Type/name consistency:** `ccp_cfg_*` signatures match across Tasks 1-9; `_seed_cc_home(home,name)` callers updated in Task 6; dispatch cases `config`/`sync` added once (Task 8 Step 3), `_profile_sync` body in Task 9.
- **No placeholders:** every code step is complete and runnable.
- **`jq`-gated tests:** merge/validate assertions skip cleanly when `jq` is absent so CI passes either way; fallback path (snapshot) is still exercised.
