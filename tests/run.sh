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
[[ -f "$ROOT/lib/cfg.sh" ]]      && { source "$ROOT/lib/cfg.sh"; }
[[ -f "$ROOT/lib/instruct.sh" ]] && { source "$ROOT/lib/instruct.sh"; }

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
  ccp_rule_set /a personal "$rf"
  assert_eq "$(grep -c . "$rf")" "1" "one line after replace"
  assert_eq "$(ccp_resolve /a "$rf")" "personal" "replaced value"
}
test_rule_del() {
  local rf; rf="$(newdir)/rules.tsv"
  ccp_rule_set /a work "$rf"
  ccp_rule_del /a "$rf"
  assert_eq "$(ccp_resolve /a "$rf")" "default" "deleted => default"
}

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
  assert_eq "$(ccp_profile_list "$h" | sort | tr '\n' ' ')" "ds work " "list names sorted"
}
test_profile_rm() {
  local h; h="$(newdir)"
  ccp_profile_add_official "$h" work
  ccp_profile_rm "$h" work
  ccp_profile_exists "$h" work; assert_rc "$?" 1 "removed"
  assert_eq "$(ccp_profile_list "$h")" "" "index empty after rm"
}

_MANAGED='CLAUDE_CONFIG_DIR ANTHROPIC_BASE_URL ANTHROPIC_AUTH_TOKEN ANTHROPIC_MODEL ANTHROPIC_DEFAULT_OPUS_MODEL ANTHROPIC_DEFAULT_SONNET_MODEL ANTHROPIC_DEFAULT_HAIKU_MODEL CLAUDE_CODE_SUBAGENT_MODEL CLAUDE_CODE_EFFORT_LEVEL CCP_PROFILE'

test_env_default_unsets_all() {
  local h; h="$(newdir)"
  local out; out="$(ccp_env_delta "$h" default)"
  case "$out" in *"unset "*"ANTHROPIC_BASE_URL"*) :;; *) _fail=$((_fail+1)); echo "FAIL: default no unset" >&2;; esac
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
  local cfg; cfg="$(eval "$out"; printf '%s' "${CLAUDE_CONFIG_DIR:-NONE}")"
  assert_eq "$cfg" "$h/profiles/ds/cc-home" "deepseek now exports its cc-home as CLAUDE_CONFIG_DIR"
}

_ccp() { CCP_HOME="$1" bash "$ROOT/bin/ccp" "${@:2}"; }

test_bin_resolve_default() {
  local h; h="$(newdir)"
  local out rc
  out="$(_ccp "$h" resolve /tmp/whatever)"; rc=$?
  assert_eq "$out" "default" "resolve prints default"
  assert_rc "$rc" 1 "default => exit 1"
}
test_bin_env_default() {
  local h; h="$(newdir)"
  local out; out="$(_ccp "$h" _env default)"
  case "$out" in *"export CCP_PROFILE=default"*) _pass=$((_pass+1));;
    *) _fail=$((_fail+1)); echo "FAIL: _env default" >&2;; esac
}

test_bin_profile_add_list() {
  local h; h="$(newdir)"
  _ccp "$h" profile add work --official >/dev/null
  _ccp "$h" profile add ds --deepseek --base-url u --pro p --flash f --effort max >/dev/null
  assert_eq "$(_ccp "$h" profile list | grep -c .)" "2" "two profiles listed"
}
test_bin_profile_show() {
  local h; h="$(newdir)"
  _ccp "$h" profile add ds --deepseek --base-url u --pro p --flash f --effort high >/dev/null
  local out; out="$(_ccp "$h" profile show ds)"
  case "$out" in *high*) _pass=$((_pass+1));; *) _fail=$((_fail+1)); echo "FAIL: show prints effort" >&2;; esac
}
test_bin_profile_rm() {
  local h; h="$(newdir)"
  _ccp "$h" profile add work --official >/dev/null
  _ccp "$h" profile rm work >/dev/null
  assert_eq "$(_ccp "$h" profile list | grep -c .)" "0" "removed"
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
  _ccp "$h" path include /tmp/leg >/dev/null
  assert_eq "$(_ccp "$h" path test /tmp/leg/x)" "deepseek" "legacy include => deepseek"
}
test_bin_key_sets_profile_key() {
  local h; h="$(newdir)"
  _ccp "$h" profile add ds --deepseek --base-url u --pro p --flash f --effort max >/dev/null
  _ccp "$h" key ds sk-abc >/dev/null
  assert_eq "$(ccp_profile_get_key "$h" ds)" "sk-abc" "ccp key <profile> stores"
}

test_bin_status_json_default() {
  local h; h="$(newdir)"
  local out; out="$(_ccp "$h" status --json)"
  case "$out" in *'"profile":"default"'*) _pass=$((_pass+1));;
    *) _fail=$((_fail+1)); echo "FAIL: status json profile default: $out" >&2;; esac
}
test_bin_status_json_profile_type() {
  local h; h="$(newdir)"
  local zone; zone="$(newdir)/sz"; mkdir -p "$zone"
  _ccp "$h" profile add ds --deepseek --base-url u --pro p --flash f --effort max >/dev/null
  _ccp "$h" path set "$zone" ds >/dev/null
  local out; out="$(cd "$zone" && CCP_HOME="$h" bash "$ROOT/bin/ccp" status --json)"
  case "$out" in *'"profile":"ds"'*'"profile_type":"deepseek"'*) _pass=$((_pass+1));;
    *) _fail=$((_fail+1)); echo "FAIL: status json profile_type: $out" >&2;; esac
}

test_seed_official_symlinks() {
  local h; h="$(newdir)"
  local src; src="$(newdir)"
  mkdir -p "$src/plugins" "$src/commands" "$src/agents"
  printf 'x' > "$src/CLAUDE.md"
  printf '{"k":1}' > "$src/settings.json"
  CCP_HOME="$h" CCP_CLAUDE_SRC="$src" bash "$ROOT/bin/ccp" profile add work --official >/dev/null
  local cch="$h/profiles/work/cc-home"
  [[ -L "$cch/plugins" ]]; assert_rc "$?" 0 "plugins symlinked"
  [[ -f "$cch/CLAUDE.md" && ! -L "$cch/CLAUDE.md" ]]; assert_rc "$?" 0 "CLAUDE.md is generated real file"
  case "$(cat "$cch/CLAUDE.md")" in *"@$src/CLAUDE.md"*) _pass=$((_pass+1));;
    *) _fail=$((_fail+1)); echo "FAIL: cc-home CLAUDE.md missing global @import" >&2;; esac
  [[ -f "$cch/settings.json" && ! -L "$cch/settings.json" ]]; assert_rc "$?" 0 "settings.json generated not linked"
  [[ -f "$h/profiles/work/overlay/settings.overlay.json" ]]; assert_rc "$?" 0 "overlay seeded"
  [[ ! -e "$cch/.claude.json" ]]; assert_rc "$?" 0 "no creds seeded"
}

test_seed_deepseek_gets_cchome() {
  local h; h="$(newdir)"; local src; src="$(newdir)"
  mkdir -p "$src/plugins"; printf 'G' > "$src/CLAUDE.md"; printf '{"m":1}' > "$src/settings.json"
  CCP_HOME="$h" CCP_CLAUDE_SRC="$src" bash "$ROOT/bin/ccp" profile add ds --deepseek --base-url u --pro p --flash f --effort max >/dev/null
  local cch="$h/profiles/ds/cc-home"
  [[ -L "$cch/plugins" ]]; assert_rc "$?" 0 "deepseek cc-home has symlinked plugins"
  [[ -f "$cch/CLAUDE.md" && -f "$cch/settings.json" ]]; assert_rc "$?" 0 "deepseek cc-home generated config"
}

test_shell_init_valid_bash() {
  local out; out="$(bash "$ROOT/bin/ccp" completion-shellinit 2>/dev/null)"
  printf '%s' "$out" | bash -n -; assert_rc "$?" 0 "shell init parses"
  case "$out" in *">>> ccp shell init >>>"*) _pass=$((_pass+1));;
    *) _fail=$((_fail+1)); echo "FAIL: missing marker" >&2;; esac
}
test_shell_fn_use_evals_env() {
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

test_migrate_creates_deepseek_profile() {
  local old; old="$(newdir)"
  printf 'DS_BASE_URL="https://api.deepseek.com/anthropic"\nDS_MODEL_PRO="pro[1m]"\nDS_MODEL_FLASH="flash"\nDS_EFFORT="high"\n' > "$old/config"
  printf 'sk-old-key' > "$old/api_key"
  printf 'include\t/a\nexclude\t/a/b\n' > "$old/rules.tsv"
  local h; h="$(newdir)/ccp"
  CCP_HOME="$h" DSCTL_HOME_SRC="$old" bash "$ROOT/bin/ccp" migrate >/dev/null 2>&1
  assert_eq "$(ccp_profile_type "$h" deepseek)" "deepseek" "deepseek profile created"
  assert_eq "$(ccp_profile_get "$h" deepseek effort)" "high" "effort migrated"
  assert_eq "$(ccp_profile_get_key "$h" deepseek)" "sk-old-key" "key migrated"
  assert_eq "$(ccp_resolve /a/x "$h/rules.tsv")"   "deepseek" "include -> deepseek"
  assert_eq "$(ccp_resolve /a/b/y "$h/rules.tsv")" "default"  "exclude -> default"
}

test_bin_help_mentions_profile() {
  local h; h="$(newdir)"
  local out; out="$(_ccp "$h" help)"
  case "$out" in *"profile config"*"path set"*) _pass=$((_pass+1));;
    *) _fail=$((_fail+1)); echo "FAIL: help missing profile config/path set" >&2;; esac
}
test_bin_unknown_cmd_exit1() {
  local h; h="$(newdir)"
  _ccp "$h" bogus >/dev/null 2>&1; assert_rc "$?" 1 "unknown cmd exit 1"
}
test_bin_config_defaults() {
  local h; h="$(newdir)"
  local out; out="$(_ccp "$h" config show)"
  case "$out" in *"deepseek"*) _pass=$((_pass+1));;
    *) _fail=$((_fail+1)); echo "FAIL: config show" >&2;; esac
}

test_bin_config_set_used_by_profile_add() {
  local h; h="$(newdir)"
  _ccp "$h" config set base_url "https://custom/anthropic" >/dev/null
  _ccp "$h" profile add ds --deepseek >/dev/null
  assert_eq "$(ccp_profile_get "$h" ds base_url)" "https://custom/anthropic" "profile add uses configured default base_url"
}

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

test_bin_config_editor_set_show() {
  local h; h="$(newdir)"
  _ccp "$h" config editor "code -w" >/dev/null
  local out; out="$(_ccp "$h" config show)"
  case "$out" in *"code -w"*) _pass=$((_pass+1));;
    *) _fail=$((_fail+1)); echo "FAIL: config show missing editor" >&2;; esac
}

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

test_bin_profile_config_default_no_tty() {
  local h; h="$(newdir)"; local src; src="$(newdir)"
  printf 'G' > "$src/CLAUDE.md"; printf '{}' > "$src/settings.json"
  local rc; CCP_HOME="$h" CCP_CLAUDE_SRC="$src" bash "$ROOT/bin/ccp" profile config default </dev/null >/dev/null 2>&1; rc=$?
  assert_rc "$rc" 1 "default config without tty => error"
}

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

test_bin_profile_config_preserves_legacy_settings() {
  command -v jq >/dev/null 2>&1 || { _pass=$((_pass+1)); return; }
  local h; h="$(newdir)"; local src; src="$(newdir)"
  printf '{"model":"opus"}' > "$src/settings.json"; printf 'G' > "$src/CLAUDE.md"
  CCP_HOME="$h" CCP_CLAUDE_SRC="$src" bash "$ROOT/bin/ccp" profile add work --official >/dev/null
  # simula estado LEGACY pre-overlay: sin overlay, con settings.json copia real (custom) en cc-home
  rm -rf "$h/profiles/work/overlay"
  printf '{"custom":"keep"}' > "$h/profiles/work/cc-home/settings.json"
  # editor no-op (no cambia el overlay)
  local fe; fe="$(newdir)/fe"; printf '#!/usr/bin/env bash\n:\n' > "$fe"; chmod +x "$fe"
  CCP_HOME="$h" CCP_CLAUDE_SRC="$src" bash "$ROOT/bin/ccp" config editor "$fe" >/dev/null
  CCP_HOME="$h" CCP_CLAUDE_SRC="$src" bash "$ROOT/bin/ccp" profile config work settings >/dev/null
  assert_eq "$(jq -r '.custom' "$h/profiles/work/overlay/settings.overlay.json")" "keep" "legacy settings preserved into overlay"
  assert_eq "$(jq -r '.custom' "$h/profiles/work/cc-home/settings.json")" "keep" "legacy custom key survives merge into cc-home"
}
test_bin_profile_sync_default_rejected() {
  local h; h="$(newdir)"
  local rc; _ccp "$h" profile sync default >/dev/null 2>&1; rc=$?
  assert_rc "$rc" 1 "sync default => error (no cc-home)"
}

test_install_records_source() {
  local bd ld h; bd="$(newdir)"; ld="$(newdir)"; h="$(newdir)"
  CCP_BIN_DIR="$bd" CCP_LIB_DIR="$ld" CCP_HOME="$h" bash "$ROOT/install.sh" >/dev/null 2>&1
  assert_eq "$(cat "$h/install-source")" "$ROOT" "install.sh records the source repo path"
}
test_bin_upgrade_no_source_errors() {
  local h; h="$(newdir)"
  local rc; _ccp "$h" upgrade >/dev/null 2>&1; rc=$?
  assert_rc "$rc" 1 "upgrade without recorded source => error"
}
test_bin_upgrade_bad_arg() {
  local h; h="$(newdir)"; printf '%s' "$ROOT" > "$h/install-source"
  local rc; _ccp "$h" upgrade --bogus >/dev/null 2>&1; rc=$?
  assert_rc "$rc" 1 "upgrade with unknown flag => error"
}
test_bin_upgrade_runs() {
  # instala en dirs temporales (registra la fuente = $ROOT), luego upgrade --no-sync
  local bd ld h; bd="$(newdir)"; ld="$(newdir)"; h="$(newdir)"
  CCP_BIN_DIR="$bd" CCP_LIB_DIR="$ld" CCP_HOME="$h" bash "$ROOT/install.sh" >/dev/null 2>&1
  # rc temporal SIN bloque => _upgrade_check_rc es no-op (no toca ningún rc real)
  local rcfile; rcfile="$(newdir)/rc"; : > "$rcfile"
  local out; out="$(CCP_BIN_DIR="$bd" CCP_HOME="$h" CCP_RC="$rcfile" bash "$ROOT/bin/ccp" upgrade --no-sync 2>&1)"
  case "$out" in *"actualizado"*) _pass=$((_pass+1));;
    *) _fail=$((_fail+1)); echo "FAIL: upgrade run did not report: $out" >&2;; esac
  [[ -x "$bd/ccp" ]]; assert_rc "$?" 0 "upgrade reinstalled the binary"
}

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
  assert_eq "$(ccp_instruct_dest project command /h ds /src /root)" "/root/.claude/commands" "proj command"
  assert_eq "$(ccp_instruct_dest project skill   /h ds /src /root)" "/root/.claude/skills"   "proj skill"
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

# ===== instruct: bloque de reglas =====
test_instruct_rule_add_creates_block() {
  local f; f="$(newdir)/CLAUDE.md"; printf '# Mi config\n\ncontenido previo\n' > "$f"
  ccp_instruct_rule_add "$f" "responde en español"
  case "$(cat "$f")" in *"contenido previo"*) _pass=$((_pass+1));;
    *) _fail=$((_fail+1)); echo "FAIL: preserva contenido previo" >&2;; esac
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
test_instruct_rule_add_backslash_safe() {
  local f; f="$(newdir)/CLAUDE.md"; : > "$f"
  ccp_instruct_rule_add "$f" 'usa la ruta C:\temp\x'
  assert_eq "$(ccp_instruct_rule_list "$f")" 'usa la ruta C:\temp\x' "backslashes intactos"
  ccp_instruct_rule_add "$f" 'usa la ruta C:\temp\x'; assert_rc "$?" 9 "dedup con backslash => rc9"
  assert_eq "$(ccp_instruct_rule_list "$f" | grep -c .)" "1" "no duplica con backslash"
}

# ===== instruct: binario (scope rule) =====
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
  local rc; CCP_HOME="$h" CCP_CLAUDE_SRC="$s" CCP_PROFILE="" bash "$ROOT/bin/ccp" instruct add profile rule "x" >/dev/null 2>&1; rc=$?
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
  case "$out" in *" 1) [rule] uno"*) _pass=$((_pass+1));;
    *) _fail=$((_fail+1)); echo "FAIL: list numerado: $out" >&2;; esac
  _ccp_instr "$h" "$s" "" instruct rm global 1 >/dev/null
  local rem; rem="$(_ccp_instr "$h" "$s" "" instruct list global)"
  case "$rem" in *"dos"*) _pass=$((_pass+1));;
    *) _fail=$((_fail+1)); echo "FAIL: rm dejó 'dos'" >&2;; esac
  case "$rem" in *"uno"*) _fail=$((_fail+1)); echo "FAIL: 'uno' debió borrarse" >&2;; *) _pass=$((_pass+1));; esac
}
test_bin_instruct_project_fallback_cwd() {
  local h s; h="$(newdir)"; s="$(newdir)"
  local d; d="$(newdir)"
  ( cd "$d" && CCP_HOME="$h" CCP_CLAUDE_SRC="$s" bash "$ROOT/bin/ccp" instruct add project rule "regla repo" >/dev/null )
  case "$(cat "$d/.claude/CLAUDE.md" 2>/dev/null)" in *"regla repo"*) _pass=$((_pass+1));;
    *) _fail=$((_fail+1)); echo "FAIL: fallback a cwd para project" >&2;; esac
}

test_bin_instruct_dest_subcmd() {
  local h s; h="$(newdir)"; s="$(newdir)"
  assert_eq "$(_ccp_instr "$h" "$s" "" instruct dest global agent)" "$s/agents" "dest global agent"
}
test_bin_instruct_record_and_list_global() {
  local h s; h="$(newdir)"; s="$(newdir)"
  _ccp_instr "$h" "$s" "" instruct add global rule "una regla" >/dev/null
  _ccp_instr "$h" "$s" "" instruct record global agent "$s/agents/sec.md" "auditor seguridad" >/dev/null
  local out; out="$(_ccp_instr "$h" "$s" "" instruct list global)"
  case "$out" in *"una regla"*"auditor seguridad"*) _pass=$((_pass+1));;
    *) _fail=$((_fail+1)); echo "FAIL: list combina rule+manifest: $out" >&2;; esac
}
test_bin_instruct_rm_deletes_file_artifact() {
  local h s; h="$(newdir)"; s="$(newdir)"
  mkdir -p "$s/agents"; printf 'x' > "$s/agents/sec.md"
  _ccp_instr "$h" "$s" "" instruct record global agent "$s/agents/sec.md" "auditor" >/dev/null
  _ccp_instr "$h" "$s" "" instruct rm global 1 >/dev/null
  [[ ! -e "$s/agents/sec.md" ]]; assert_rc "$?" 0 "rm borró el archivo del artefacto"
}
test_bin_instruct_rm_rule_then_manifest_indexing() {
  local h s; h="$(newdir)"; s="$(newdir)"
  mkdir -p "$s/agents"; printf 'x' > "$s/agents/a.md"
  _ccp_instr "$h" "$s" "" instruct add global rule "regla1" >/dev/null
  _ccp_instr "$h" "$s" "" instruct record global agent "$s/agents/a.md" "agente1" >/dev/null
  # index 1 = la regla (no el manifest). Borrarla deja el agente intacto.
  _ccp_instr "$h" "$s" "" instruct rm global 1 >/dev/null
  [[ -e "$s/agents/a.md" ]]; assert_rc "$?" 0 "rm 1 borró la regla, NO el agente"
  local out; out="$(_ccp_instr "$h" "$s" "" instruct list global)"
  case "$out" in *"agente1"*) _pass=$((_pass+1));;
    *) _fail=$((_fail+1)); echo "FAIL: el agente debe seguir listado: $out" >&2;; esac
}

test_install_copies_commands() {
  local bd ld h cd; bd="$(newdir)"; ld="$(newdir)"; h="$(newdir)"; cd="$(newdir)/claude"
  CCP_BIN_DIR="$bd" CCP_LIB_DIR="$ld" CCP_HOME="$h" CCP_CLAUDE_SRC="$cd" \
    bash "$ROOT/install.sh" >/dev/null 2>&1
  [[ -f "$cd/commands/ccp/remember-global.md" ]]; assert_rc "$?" 0 "install copió remember-global"
  [[ -f "$cd/commands/ccp/forget.md" ]];          assert_rc "$?" 0 "install copió forget"
}

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
  ccp_instruct_manifest_add "$m" profile work  hook /ov/work/settings.overlay.json  "hook A"
  ccp_instruct_manifest_add "$m" profile other hook /ov/other/settings.overlay.json "hook B"
  assert_eq "$(ccp_instruct_manifest_list "$m" profile work | grep -c .)" "1" "filtra por perfil activo"
  assert_eq "$(ccp_instruct_manifest_list "$m" profile work)" "hook	/ov/work/settings.overlay.json	hook A" "desc round-trips para el perfil filtrado"
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

# ---- runner ----
_filter="${1:-}"
_tests="$(declare -F | awk '{print $3}' | grep '^test_' | { [[ -n "$_filter" ]] && grep -- "$_filter" || cat; } | sort)"
for fn in $_tests; do "$fn"; done
printf '\n%s%d passed, %d failed%s\n' "" "$_pass" "$_fail" ""
[[ "$_fail" -eq 0 ]]
