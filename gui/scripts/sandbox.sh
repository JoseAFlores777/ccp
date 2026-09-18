#!/usr/bin/env bash
# sandbox.sh — prepara un ccp de pruebas para desarrollar la interfaz sin tocar
# la configuración real: un HOME y un CCP_HOME propios bajo gui/.sandbox, un
# binario compilado de este repo y datos de ejemplo (cuentas, reglas, rotación,
# conversaciones, préstamos y muestras de uso).
#
#   npm run sandbox        # (re)crea gui/.sandbox
#   npm run dev:sandbox    # la interfaz en el navegador contra ese sandbox
#
# Nada de esto sale de gui/.sandbox: HOME apunta ahí mientras se siembra, así
# que ni ~/.claude, ni ~/.config/ccp, ni el rc de la shell se tocan.

set -euo pipefail

GUI_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
REPO="$(cd "${GUI_DIR}/.." && pwd)"
SANDBOX="${CCP_SANDBOX:-${GUI_DIR}/.sandbox}"

case "${SANDBOX}" in
  */.sandbox|*/.sandbox/) ;;
  *) echo "CCP_SANDBOX debe terminar en /.sandbox (por seguridad: este script lo borra entero)" >&2; exit 1 ;;
esac

rm -rf "${SANDBOX}"
mkdir -p "${SANDBOX}/bin" "${SANDBOX}/home" "${SANDBOX}/ccp"

echo "· compilando ccp de este repo"
(cd "${REPO}" && go build -o "${SANDBOX}/bin/ccp" ./cmd/ccp)

export HOME="${SANDBOX}/home"
export CCP_HOME="${SANDBOX}/ccp"
export CCP_LANG=es
unset CCP_PROFILE CLAUDE_CONFIG_DIR ANTHROPIC_BASE_URL ANTHROPIC_AUTH_TOKEN || true
CCP="${SANDBOX}/bin/ccp"

# iso_at <minutos desde ahora> — fecha UTC en RFC 3339, con date de BSD o de GNU.
iso_at() {
  local m="$1"
  if date -u -v+0M +%s >/dev/null 2>&1; then
    if [ "${m}" -ge 0 ]; then date -u -v+"${m}"M +%Y-%m-%dT%H:%M:%SZ; else date -u -v"${m}"M +%Y-%m-%dT%H:%M:%SZ; fi
  else
    date -u -d "${m} minutes" +%Y-%m-%dT%H:%M:%SZ
  fi
}

ms_at() {
  local m="$1"
  echo $(( ( $(date +%s) + m * 60 ) * 1000 ))
}

echo "· carpetas y ~/.claude"
mkdir -p "${HOME}/Trabajo/ccp/internal" "${HOME}/Trabajo/personal/finanzas" "${HOME}/Trabajo/cliente-x" \
  "${HOME}/Labs/bench" "${HOME}/.claude/commands" "${HOME}/.claude/agents" "${HOME}/.claude/skills" "${HOME}/.claude/plugins"
git -C "${HOME}/Trabajo/ccp" init -q 2>/dev/null || true
cat > "${HOME}/.claude/CLAUDE.md" <<'EOF'
# Instrucciones globales de prueba

Responde siempre en español.
EOF
cat > "${HOME}/.claude/settings.json" <<'EOF'
{
  "env": { "BASH_DEFAULT_TIMEOUT_MS": "120000", "DISABLE_TELEMETRY": "1" },
  "permissions": { "allow": ["Bash(git status)", "Bash(go test:*)"] },
  "statusLine": { "type": "command", "command": "starship prompt" }
}
EOF
echo '{"oauthAccount":{"emailAddress":"yo@example.com"}}' > "${HOME}/.claude.json"

echo "· cuentas"
"${CCP}" profile add trabajo --official >/dev/null
"${CCP}" profile add personal --official >/dev/null
"${CCP}" profile add nueva --official >/dev/null
"${CCP}" profile add deepseek-lab --deepseek >/dev/null
"${CCP}" profile add kimi-lab --kimi >/dev/null
"${CCP}" profile add vieja --official >/dev/null
"${CCP}" key deepseek-lab sk-sandbox-no-es-una-key-real >/dev/null
for p in trabajo personal; do
  echo '{"oauthAccount":{"emailAddress":"'"${p}"'@example.com"}}' > "${CCP_HOME}/profiles/${p}/cc-home/.claude.json"
done

echo "· reglas"
"${CCP}" path set "${HOME}/Trabajo" trabajo >/dev/null
"${CCP}" path set "${HOME}/Trabajo/personal" personal >/dev/null
"${CCP}" path set "${HOME}/Trabajo/cliente-x" default >/dev/null
"${CCP}" path set "${HOME}/Labs" deepseek-lab >/dev/null
"${CCP}" path set "${HOME}/Archivo/2024" vieja >/dev/null
"${CCP}" profile rm vieja >/dev/null   # deja la regla huérfana a propósito

echo "· rotación"
"${CCP}" auto init >/dev/null
(cd "${HOME}/Trabajo/ccp" && "${CCP}" auto chain set personal deepseek-lab kimi-lab >/dev/null)
"${CCP}" auto install trabajo personal >/dev/null

echo "· muestras de uso"
RL="${CCP_HOME}/state/auto/rate-limits"
mkdir -p "${RL}"
cat > "${RL}/trabajo.json" <<EOF
{"sampled_at":"$(iso_at -3)","limits":{"five_hour":{"used_percentage":72,"resets_at":"$(iso_at 150)"},"seven_day":{"used_percentage":41,"resets_at":"$(iso_at 4000)"}}}
EOF
cat > "${RL}/personal.json" <<EOF
{"sampled_at":"$(iso_at -12)","limits":{"five_hour":{"used_percentage":10,"resets_at":"$(iso_at 245)"},"seven_day":{"used_percentage":18,"resets_at":"$(iso_at 6000)"}}}
EOF

echo "· conversaciones"
# transcript <cc-home> <cwd> <uuid> <título> <minutos atrás>
transcript() {
  local cch="$1" cwd="$2" uuid="$3" title="$4" ago="$5"
  local slug dir ts
  slug="$(printf '%s' "${cwd}" | sed 's/[^A-Za-z0-9]/-/g')"
  dir="${cch}/projects/${slug}"
  ts="$(iso_at "-${ago}")"
  mkdir -p "${dir}"
  cat > "${dir}/${uuid}.jsonl" <<EOF
{"type":"user","sessionId":"${uuid}","cwd":"${cwd}","timestamp":"${ts}","uuid":"a${uuid:1}","message":{"role":"user","content":"Empecemos con ${title}"}}
{"type":"assistant","sessionId":"${uuid}","cwd":"${cwd}","timestamp":"${ts}","uuid":"b${uuid:1}","parentUuid":"a${uuid:1}","message":{"role":"assistant","content":[{"type":"text","text":"De acuerdo."}]}}
{"type":"ai-title","aiTitle":"${title}","sessionId":"${uuid}"}
EOF
  touch -t "$(date -v-"${ago}"M +%Y%m%d%H%M.%S 2>/dev/null || date -d "-${ago} minutes" +%Y%m%d%H%M.%S)" "${dir}/${uuid}.jsonl"
}

TR="${CCP_HOME}/profiles/trabajo/cc-home"
PE="${CCP_HOME}/profiles/personal/cc-home"
DS="${CCP_HOME}/profiles/deepseek-lab/cc-home"
U1=8f2a1c4d-1111-4c4d-9a1b-0a1b2c3d4e51
U2=b41e9077-2222-4c4d-9a1b-0a1b2c3d4e52
U3=2c7d5ab0-3333-4c4d-9a1b-0a1b2c3d4e53
U4=f09b3e15-4444-4c4d-9a1b-0a1b2c3d4e54
U5=55a0c8e2-5555-4c4d-9a1b-0a1b2c3d4e55
U6=6d1f0c3b-6666-4c4d-9a1b-0a1b2c3d4e56
U7=7e2a4b19-7777-4c4d-9a1b-0a1b2c3d4e57
transcript "${TR}" "${HOME}/Trabajo/ccp" "${U1}" "Refactor del resolver de rutas" 8
transcript "${TR}" "${HOME}/Trabajo/ccp" "${U2}" "Revisión del ADR 0009" 60
transcript "${PE}" "${HOME}/Trabajo/ccp" "${U2}" "Revisión del ADR 0009" 55
transcript "${PE}" "${HOME}/Trabajo/personal/finanzas" "${U3}" "Presupuesto anual" 1500
transcript "${DS}" "${HOME}/Labs/bench" "${U4}" "Bench de embeddings" 180
transcript "${HOME}/.claude" "${HOME}" "${U5}" "Notas de la reunión de plataforma" 2900
transcript "${TR}" "${HOME}/Trabajo/ccp" "${U6}" "Migración de settings" 22
transcript "${DS}" "${HOME}/Trabajo/ccp" "${U6}" "Migración de settings" 20
transcript "${TR}" "${HOME}/Trabajo/ccp" "${U7}" "Limpieza de goldens" 4300

echo "· índice de Desktop"
desk_index() {
  local data="$1" uuid="$2" title="$3" cwd="$4" ago="$5"
  local d="${data}/claude-code-sessions/acct-0001/org-0001"
  mkdir -p "${d}"
  cat > "${d}/local_${uuid:0:8}.json" <<EOF
{"cliSessionId":"${uuid}","title":"${title}","cwd":"${cwd}","lastActivityAt":$(ms_at "-${ago}"),"isArchived":false}
EOF
}
desk_index "${CCP_HOME}/profiles/trabajo/desktop" "${U2}" "Revisión del ADR 0009" "${HOME}/Trabajo/ccp" 60
desk_index "${CCP_HOME}/profiles/personal/desktop" "${U3}" "Presupuesto anual" "${HOME}/Trabajo/personal/finanzas" 1500
desk_index "${HOME}/Library/Application Support/Claude" "${U5}" "Notas de la reunión de plataforma" "${HOME}" 2900

echo "· préstamos"
SLUG="$(printf '%s' "${HOME}/Trabajo/ccp" | sed 's/[^A-Za-z0-9]/-/g')"
cat > "${CCP_HOME}/handoffs.yaml" <<EOF
version: 2
active:
  - session: ${U2}
    slug: ${SLUG}
    cwd: ${HOME}/Trabajo/ccp
    from: trabajo
    to: personal
    title: Revisión del ADR 0009
    since: "$(iso_at -60)"
  - session: ${U6}
    slug: ${SLUG}
    cwd: ${HOME}/Trabajo/ccp
    from: trabajo
    to: deepseek-lab
    title: Migración de settings
    since: "$(iso_at -22)"
    auto: true
    hops: [personal, deepseek-lab]
  - session: ${U7}
    slug: ${SLUG}
    cwd: ${HOME}/Trabajo/ccp
    from: trabajo
    to: personal
    title: Limpieza de goldens
    since: "$(iso_at -4300)"
archived:
  - session: 1a2b3c4d-8888-4c4d-9a1b-0a1b2c3d4e58
    from: personal
    to: trabajo
    slug: ${SLUG}
    returned_as: 9b8a7c6d-9999-4c4d-9a1b-0a1b2c3d4e59
    since: "$(iso_at -3000)"
    ended: "$(iso_at -2900)"
  - session: 3c4d5e6f-aaaa-4c4d-9a1b-0a1b2c3d4e5a
    from: deepseek-lab
    to: trabajo
    slug: ${SLUG}
    returned_as: ""
    since: "$(iso_at -7300)"
    ended: "$(iso_at -7200)"
EOF

echo "· memoria"
"${CCP}" instruct add global rule "Responde siempre en español salvo que el repo esté en inglés." >/dev/null
"${CCP}" instruct add global rule "No toques archivos bajo testdata/golden sin pedirlo antes." >/dev/null
(cd "${HOME}/Trabajo/ccp" && CCP_REPO_ROOT="${HOME}/Trabajo/ccp" "${CCP}" instruct add project rule "Los ADR van en docs/adr con numeración de cuatro dígitos." >/dev/null) || true
(CCP_PROFILE=trabajo "${CCP}" instruct add profile rule "En esta cuenta, nunca ejecutes migraciones contra producción." >/dev/null) || true

cat > "${SANDBOX}/env" <<EOF
export CCP_SANDBOX='${SANDBOX}'
export HOME='${HOME}'
export CCP_HOME='${CCP_HOME}'
export CCP_GUI_BIN='${CCP}'
EOF

echo
echo "Sandbox listo en ${SANDBOX}"
echo "  npm run dev:sandbox    → http://localhost:1420"
