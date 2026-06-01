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

# ---- runner ----
_filter="${1:-}"
_tests="$(declare -F | awk '{print $3}' | grep '^test_' | { [[ -n "$_filter" ]] && grep -- "$_filter" || cat; } | sort)"
for fn in $_tests; do "$fn"; done
printf '\n%s%d passed, %d failed%s\n' "" "$_pass" "$_fail" ""
[[ "$_fail" -eq 0 ]]
