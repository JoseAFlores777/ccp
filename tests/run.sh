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
  assert_eq "$cfg" "NONE" "deepseek leaves CLAUDE_CONFIG_DIR unset"
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
  [[ -L "$cch/CLAUDE.md" ]]; assert_rc "$?" 0 "CLAUDE.md symlinked"
  [[ -f "$cch/settings.json" && ! -L "$cch/settings.json" ]]; assert_rc "$?" 0 "settings.json copied not linked"
  [[ ! -e "$cch/.claude.json" ]]; assert_rc "$?" 0 "no creds seeded"
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
  case "$out" in *"profile"*"path set"*) _pass=$((_pass+1));;
    *) _fail=$((_fail+1)); echo "FAIL: help missing profile/path set" >&2;; esac
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

# ---- runner ----
_filter="${1:-}"
_tests="$(declare -F | awk '{print $3}' | grep '^test_' | { [[ -n "$_filter" ]] && grep -- "$_filter" || cat; } | sort)"
for fn in $_tests; do "$fn"; done
printf '\n%s%d passed, %d failed%s\n' "" "$_pass" "$_fail" ""
[[ "$_fail" -eq 0 ]]
