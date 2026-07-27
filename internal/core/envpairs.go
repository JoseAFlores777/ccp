package core

import (
	"fmt"
	"strings"

	"github.com/JoseAFlores777/ccp/internal/core/i18n"
)

// envpairs.go — la MISMA decisión de entorno que env.go, pero en forma de datos.
//
// Por qué existe: `EnvDelta` sirve al shell (texto eval-able, contrato congelado
// byte a byte contra el oráculo bash). El supervisor de auto-handoff, en cambio,
// lanza `claude` como proceso hijo desde Go y necesita `[]string` para
// `exec.Cmd.Env` — pasar por el shell para eso sería absurdo (y reintroduciría
// el quoting como superficie de fallo).
//
// La regla que evita la divergencia: hay UNA sola función que decide qué
// variables van (envPlan). `EnvDelta` la serializa a texto shell y `EnvPairs`
// la expone como pares. Si mañana alguien añade una variable a un proveedor,
// ambas superficies la ganan a la vez.
//
// Nota sobre el tipo: `EnvVar` ya vive en presets.go (mismo shape Name/Value) y
// se reutiliza aquí en vez de declararlo otra vez — Go no admite el duplicado y,
// además, las Extra de los presets YA son EnvVar, así que el plan las copia sin
// convertir.

// envPlan es la decisión de entorno de un perfil antes de darle formato.
//
// warnLine y warnAt existen solo para que `EnvDelta` pueda reproducir su salida
// histórica byte a byte: el aviso de «sin API key» NO va al final, va incrustado
// entre `ANTHROPIC_BASE_URL` y `ANTHROPIC_MODEL` (así lo emitía lib/env.sh).
// Guardar la posición aquí es lo que permite refactorizar EnvDelta sin mover ni
// un byte de la salida que verifica internal/golden/parity_test.go.
type envPlan struct {
	vars []EnvVar // variables a exportar, EN ORDEN DE EMISIÓN
	warn string   // aviso legible para consumidores Go ("" si todo bien)
	// warnLine es la línea `echo "…" >&2` exacta (con el shellQuote que aplicaba
	// el código original); vacía si no hay aviso.
	warnLine string
	// warnAt es el índice de vars ANTES del cual se emite warnLine.
	warnAt int
}

// buildEnvPlan decide qué entorno le corresponde a `profile`. Es la única
// fuente de verdad; ni EnvDelta ni EnvPairs deciden nada por su cuenta.
//
// Los tres caminos degradados (perfil inexistente, proveedor sin api key, tipo
// desconocido) NO son errores duros a propósito: el hook de prompt corre esto en
// cada `cd`, y abortar dejaría la terminal sin entorno. Se avisa y se cae a
// `default`, que siempre es usable.
func buildEnvPlan(home, profile string, cfg *Config) envPlan {
	lang := i18n.Resolve(cfg.Lang)

	// default no tiene cc-home ni proveedor: es el login normal del usuario en
	// ~/.claude, así que basta con marcar la terminal.
	if profile == "default" {
		return envPlan{vars: []EnvVar{{Name: "CCP_PROFILE", Value: "default"}}}
	}

	p, ok := cfg.Profiles[profile]
	if !ok {
		msg := i18n.T(lang, "core.env.profile_missing")
		return envPlan{
			vars:     []EnvVar{{Name: "CCP_PROFILE", Value: "default"}},
			warn:     fmt.Sprintf(msg, profile),
			warnLine: fmt.Sprintf("echo \"%s\" >&2\n", fmt.Sprintf(msg, shellQuote(profile))),
			warnAt:   0,
		}
	}

	switch {
	case p.Type == "official":
		return envPlan{vars: []EnvVar{
			{Name: "CLAUDE_CONFIG_DIR", Value: home + "/profiles/" + profile + "/cc-home"},
			{Name: "CCP_PROFILE", Value: profile},
		}}

	case IsProviderType(p.Type):
		// Proveedor compatible (deepseek/kimi/glm): bloque común + las vars
		// Extra del preset, en el orden del preset (determinista, ver presets.go).
		preset, _ := GetProviderPreset(p.Type)
		plan := envPlan{}
		plan.vars = append(plan.vars,
			EnvVar{Name: "CLAUDE_CONFIG_DIR", Value: home + "/profiles/" + profile + "/cc-home"},
			EnvVar{Name: "ANTHROPIC_BASE_URL", Value: p.BaseURL},
		)
		if key, has := GetKey(home, profile); has {
			plan.vars = append(plan.vars, EnvVar{Name: "ANTHROPIC_AUTH_TOKEN", Value: key})
		} else {
			// El aviso ocupa el hueco del token: por eso warnAt es exactamente
			// la longitud actual de vars (2) y no el final de la lista.
			plan.warn = fmt.Sprintf("⚠️  ccp: perfil %s sin API key (ccp key %s)", profile, profile)
			plan.warnLine = fmt.Sprintf("echo \"⚠️  ccp: perfil %s sin API key (ccp key %s)\" >&2\n",
				shellQuote(profile), shellQuote(profile))
			plan.warnAt = len(plan.vars)
		}
		plan.vars = append(plan.vars,
			EnvVar{Name: "ANTHROPIC_MODEL", Value: p.ModelPro},
			EnvVar{Name: "ANTHROPIC_DEFAULT_OPUS_MODEL", Value: p.ModelPro},
			EnvVar{Name: "ANTHROPIC_DEFAULT_SONNET_MODEL", Value: p.ModelPro},
			EnvVar{Name: "ANTHROPIC_DEFAULT_HAIKU_MODEL", Value: p.ModelFlash},
			EnvVar{Name: "CLAUDE_CODE_SUBAGENT_MODEL", Value: p.ModelFlash},
			EnvVar{Name: "CLAUDE_CODE_EFFORT_LEVEL", Value: p.Effort},
		)
		plan.vars = append(plan.vars, preset.Extra...)
		plan.vars = append(plan.vars, EnvVar{Name: "CCP_PROFILE", Value: profile})
		return plan

	default:
		// Tipo desconocido = ccp.yaml escrito por un binario más nuevo (o a mano).
		// Degradar a default es más seguro que exportar un entorno a medias.
		return envPlan{
			vars:     []EnvVar{{Name: "CCP_PROFILE", Value: "default"}},
			warn:     fmt.Sprintf("⚠️  ccp: tipo de perfil desconocido (%s)", p.Type),
			warnLine: fmt.Sprintf("echo \"⚠️  ccp: tipo de perfil desconocido (%s)\" >&2\n", shellQuote(p.Type)),
			warnAt:   0,
		}
	}
}

// EnvPairs devuelve las variables a exportar para `profile` en el MISMO orden en
// que EnvDelta las emite, más un aviso legible ("" si todo fue bien).
//
// El orden importa aunque el entorno de un proceso sea un conjunto: los tests de
// equivalencia con EnvDelta lo comparan posición a posición, y ese acoplamiento
// es justo lo que impide que las dos superficies se separen sin que nadie lo note.
func EnvPairs(home, profile string, cfg *Config) (vars []EnvVar, warn string) {
	plan := buildEnvPlan(home, profile, cfg)
	return plan.vars, plan.warn
}

// EnvForChild aplica el delta sobre `base` (típicamente os.Environ()) y devuelve
// el slice listo para exec.Cmd.Env.
//
// Quita TODAS las CCPManagedVars antes de añadir las del perfil por la misma
// razón que EnvDelta emite un `unset` incondicional: si el supervisor salta de un
// perfil deepseek a uno oficial, heredar ANTHROPIC_BASE_URL del anterior mandaría
// al hijo al proveedor equivocado con las credenciales del nuevo. Limpiar y
// volver a poner es la única forma de que el entorno del hijo dependa solo del
// perfil destino y no del historial de la terminal.
func EnvForChild(base []string, home, profile string, cfg *Config) []string {
	managed := managedVarSet()

	out := make([]string, 0, len(base)+8)
	for _, kv := range base {
		// Una entrada sin '=' no es una variable válida; se conserva tal cual
		// (no nos toca sanear el entorno del usuario) salvo que sea el nombre
		// pelado de una var gestionada.
		name := kv
		if i := strings.IndexByte(kv, '='); i >= 0 {
			name = kv[:i]
		}
		if managed[name] {
			continue
		}
		out = append(out, kv)
	}

	vars, _ := EnvPairs(home, profile, cfg)
	for _, v := range vars {
		out = append(out, v.Name+"="+v.Value)
	}
	return out
}

// managedVarSet convierte CCPManagedVars (una cadena separada por espacios, así
// la heredamos del bash) en un set para el filtrado.
func managedVarSet() map[string]bool {
	set := make(map[string]bool, 16)
	for _, n := range strings.Fields(CCPManagedVars) {
		set[n] = true
	}
	return set
}
