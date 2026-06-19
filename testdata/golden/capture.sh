#!/usr/bin/env bash
# ============================================================
#  testdata/golden/capture.sh — arnés de fixtures golden.
#
#  Captura la salida del binario bash ORÁCULO (bin/ccp) sobre un CCP_HOME
#  de fixture, para que las slices Go siguientes diff-een contra ella
#  (plan §9, Fase 0). El bash es el oráculo del contrato hasta la paridad.
#
#  Uso:
#    capture.sh            regenera testdata/golden/<caso>/expected/
#    capture.sh --check    corre el oráculo y compara contra lo commiteado
#                          (exit != 0 si difiere) — regression del arnés
#
#  El fixture vive en ccp-home/ con rutas-regla tokenizadas como __ROOT__;
#  este script materializa una copia temporal con rutas reales, corre el
#  oráculo, y REDACTA la ruta temporal a __CCP_HOME__ para que el golden
#  sea estable entre máquinas.
# ============================================================
set -u

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
ORACLE="$REPO_ROOT/legacy/bin/ccp"
CASE="basic"
CASE_DIR="$SCRIPT_DIR/$CASE"
FIXTURE="$CASE_DIR/ccp-home"

MODE="write"
[[ "${1:-}" == "--check" ]] && MODE="check"

[[ -x "$ORACLE" ]] || { echo "no encuentro el oráculo: $ORACLE" >&2; exit 2; }

# --- materializar CCP_HOME temporal con rutas reales ---
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT
cp -R "$FIXTURE/." "$WORK/"
# sustituir el token __ROOT__ de rules.tsv por la ruta temporal real
sed "s|__ROOT__|$WORK|g" "$FIXTURE/rules.tsv" > "$WORK/rules.tsv"
chmod 600 "$WORK/profiles/deepseek/api_key" 2>/dev/null || true
chmod 600 "$WORK/profiles/kimi/api_key" 2>/dev/null || true
chmod 600 "$WORK/profiles/glm/api_key" 2>/dev/null || true
# dirs-regla reales (status hace cd; resolve/hook/path-test son textuales)
mkdir -p "$WORK/repos/work" "$WORK/repos/labs/secret"

# redacta la ruta temporal a un placeholder estable
redact() { sed "s|$WORK|__CCP_HOME__|g"; }

# corre el oráculo (CCP_HOME fijo), captura stdout redactado + exit code
run() { # nombre-caso  pwd  args...
  local name="$1" pwd_dir="$2"; shift 2
  local out code
  # scrub CCP_PROFILE del entorno del operador -> `active` determinista
  out="$( cd "$pwd_dir" && unset CCP_PROFILE && CCP_HOME="$WORK" NO_COLOR=1 "$ORACLE" "$@" 2>/dev/null )"
  code=$?
  out="$(printf '%s' "$out" | redact)"
  emit "$name.out"  "$out"
  emit "$name.code" "$code"
}

DIFF_FOUND=0
emit() { # archivo-relativo-a-expected  contenido
  local rel="$1" content="$2" target="$CASE_DIR/expected/$1"
  if [[ "$MODE" == "check" ]]; then
    local have; have="$(cat "$target" 2>/dev/null || true)"
    if [[ "$have" != "$content" ]]; then
      echo "DIFF: $CASE/expected/$rel" >&2
      diff <(printf '%s\n' "$have") <(printf '%s\n' "$content") >&2 || true
      DIFF_FOUND=1
    fi
  else
    mkdir -p "$(dirname "$target")"
    printf '%s\n' "$content" > "$target"
  fi
}

# ---------------- casos ----------------
# delta de entorno por tipo de perfil (contrato eval-able)
run env-default   "$WORK" _env default
run env-deepseek  "$WORK" _env deepseek
run env-kimi      "$WORK" _env kimi
run env-glm       "$WORK" _env glm
run env-official  "$WORK" _env work
run env-missing   "$WORK" _env nope

# resolución de reglas (deepest-wins) + exit codes (0=no-default, 1=default)
run resolve-official "$WORK" resolve "$WORK/repos/work"
run resolve-deepseek "$WORK" resolve "$WORK/repos/labs/x/y"
run resolve-nested   "$WORK" resolve "$WORK/repos/labs/secret/z"
run resolve-default  "$WORK" resolve "$WORK"

# hook: resuelve perfil del path y emite su delta (1 fork)
run hook-official "$WORK" _hook "$WORK/repos/work"
run hook-deepseek "$WORK" _hook "$WORK/repos/labs/x/y"
run hook-default  "$WORK" _hook "$WORK"

# path test (mismo motor que resolve, exit codes)
run pathtest-hit  "$WORK" path test "$WORK/repos/work"
run pathtest-miss "$WORK" path test "/tmp/no-such-rule-dir"

# status --json (cwd dentro del fixture => repo vacío, determinista)
run status-json "$WORK/repos/work" status --json

# scripts de shell que el rc evalúa (estáticos, sin redacción de ruta)
run completion-bash      "$WORK" completion bash
run completion-zsh       "$WORK" completion zsh
run completion-shellinit "$WORK" completion-shellinit

# Nota: `version` NO entra al golden — el oráculo bash imprime su propia
# versión (v3.1.0), distinta de la del binario Go (v2.0.0). La versión no
# es parte del contrato congelado; cada implementación reporta la suya.

if [[ "$MODE" == "check" ]]; then
  if [[ "$DIFF_FOUND" -ne 0 ]]; then
    echo "golden: diferencias detectadas (arriba)." >&2; exit 1
  fi
  echo "golden: OK — oráculo reproduce los expected commiteados."
else
  echo "golden: expected regenerados en $CASE/expected/ desde el oráculo bash."
fi
