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
      printf 'export CLAUDE_CONFIG_DIR=%q\n' "$home/profiles/$profile/cc-home"
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
