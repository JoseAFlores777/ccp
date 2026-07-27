package core

import (
	"strings"
	"testing"
)

// envpairs_test.go — cubre las dos superficies nuevas (EnvPairs/EnvForChild) y,
// sobre todo, la INVARIANTE que las ata a EnvDelta: si alguien añade una var a
// un preset y solo la mete en una de las dos rutas, el test de equivalencia
// falla antes de que el supervisor lance un `claude` con entorno incompleto.

// envFixture arma un CCP_HOME sintético con un perfil de cada tipo. Se usa
// t.TempDir() (nunca ~/.config/ccp) y se escriben api_keys solo donde el caso lo
// pide, para poder ejercitar el camino «proveedor sin API key».
func envFixture(t *testing.T) (string, *Config) {
	t.Helper()
	// El aviso de perfil inexistente pasa por i18n; fijamos el idioma para que
	// los asserts de prosa no dependan del entorno de quien corre los tests.
	t.Setenv("CCP_LANG", "es")

	home := t.TempDir()
	cfg := &Config{
		Version:  2,
		Lang:     "es",
		Profiles: map[string]Profile{},
	}
	cfg.Profiles["work"] = Profile{Type: "official"}
	for _, id := range ProviderTypes() {
		pre, _ := GetProviderPreset(id)
		cfg.Profiles[id] = Profile{
			Type:       id,
			BaseURL:    pre.BaseURL,
			ModelPro:   pre.ModelPro,
			ModelFlash: pre.ModelFlash,
			Effort:     pre.Effort,
		}
		// "nokey" es el clon sin secreto en disco.
		if err := SetKey(home, id, "sk-"+id); err != nil {
			t.Fatalf("SetKey %s: %v", id, err)
		}
	}
	cfg.Profiles["nokey"] = Profile{
		Type:       "deepseek",
		BaseURL:    "https://api.deepseek.com/anthropic",
		ModelPro:   "deepseek-chat",
		ModelFlash: "deepseek-chat",
		Effort:     "high",
	}
	cfg.Profiles["raro"] = Profile{Type: "ollama-inventado"}
	return home, cfg
}

func envNames(vars []EnvVar) []string {
	out := make([]string, len(vars))
	for i, v := range vars {
		out[i] = v.Name
	}
	return out
}

func envValue(t *testing.T, vars []EnvVar, name string) string {
	t.Helper()
	for _, v := range vars {
		if v.Name == name {
			return v.Value
		}
	}
	t.Fatalf("falta la variable %s en %v", name, envNames(vars))
	return ""
}

// ---------------------------------------------------------------------------
// 1) EnvPairs: un caso por tipo de perfil + los tres caminos degradados.
// ---------------------------------------------------------------------------

func TestEnvPairs(t *testing.T) {
	home, cfg := envFixture(t)

	cases := []struct {
		name      string
		profile   string
		wantNames []string
		wantWarn  string // substring; "" => sin aviso
	}{
		{
			name:      "default solo marca la terminal",
			profile:   "default",
			wantNames: []string{"CCP_PROFILE"},
		},
		{
			name:      "official solo redirige CLAUDE_CONFIG_DIR",
			profile:   "work",
			wantNames: []string{"CLAUDE_CONFIG_DIR", "CCP_PROFILE"},
		},
		{
			name:    "deepseek con api key",
			profile: "deepseek",
			wantNames: []string{
				"CLAUDE_CONFIG_DIR", "ANTHROPIC_BASE_URL", "ANTHROPIC_AUTH_TOKEN",
				"ANTHROPIC_MODEL", "ANTHROPIC_DEFAULT_OPUS_MODEL",
				"ANTHROPIC_DEFAULT_SONNET_MODEL", "ANTHROPIC_DEFAULT_HAIKU_MODEL",
				"CLAUDE_CODE_SUBAGENT_MODEL", "CLAUDE_CODE_EFFORT_LEVEL", "CCP_PROFILE",
			},
		},
		{
			// Sin secreto: el token desaparece de la lista (no se emite vacío,
			// que engañaría al hijo) y se avisa.
			name:    "deepseek sin api key",
			profile: "nokey",
			wantNames: []string{
				"CLAUDE_CONFIG_DIR", "ANTHROPIC_BASE_URL",
				"ANTHROPIC_MODEL", "ANTHROPIC_DEFAULT_OPUS_MODEL",
				"ANTHROPIC_DEFAULT_SONNET_MODEL", "ANTHROPIC_DEFAULT_HAIKU_MODEL",
				"CLAUDE_CODE_SUBAGENT_MODEL", "CLAUDE_CODE_EFFORT_LEVEL", "CCP_PROFILE",
			},
			wantWarn: "sin API key",
		},
		{
			name:    "kimi añade sus Extra antes de CCP_PROFILE",
			profile: "kimi",
			wantNames: []string{
				"CLAUDE_CONFIG_DIR", "ANTHROPIC_BASE_URL", "ANTHROPIC_AUTH_TOKEN",
				"ANTHROPIC_MODEL", "ANTHROPIC_DEFAULT_OPUS_MODEL",
				"ANTHROPIC_DEFAULT_SONNET_MODEL", "ANTHROPIC_DEFAULT_HAIKU_MODEL",
				"CLAUDE_CODE_SUBAGENT_MODEL", "CLAUDE_CODE_EFFORT_LEVEL",
				"ENABLE_TOOL_SEARCH", "CLAUDE_CODE_AUTO_COMPACT_WINDOW", "CCP_PROFILE",
			},
		},
		{
			name:    "glm añade sus Extra antes de CCP_PROFILE",
			profile: "glm",
			wantNames: []string{
				"CLAUDE_CONFIG_DIR", "ANTHROPIC_BASE_URL", "ANTHROPIC_AUTH_TOKEN",
				"ANTHROPIC_MODEL", "ANTHROPIC_DEFAULT_OPUS_MODEL",
				"ANTHROPIC_DEFAULT_SONNET_MODEL", "ANTHROPIC_DEFAULT_HAIKU_MODEL",
				"CLAUDE_CODE_SUBAGENT_MODEL", "CLAUDE_CODE_EFFORT_LEVEL",
				"API_TIMEOUT_MS", "CLAUDE_CODE_AUTO_COMPACT_WINDOW", "CCP_PROFILE",
			},
		},
		{
			name:      "perfil inexistente degrada a default con aviso",
			profile:   "no-existe",
			wantNames: []string{"CCP_PROFILE"},
			wantWarn:  "no existe",
		},
		{
			name:      "tipo desconocido degrada a default con aviso",
			profile:   "raro",
			wantNames: []string{"CCP_PROFILE"},
			wantWarn:  "tipo de perfil desconocido",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			vars, warn := EnvPairs(home, c.profile, cfg)
			got := envNames(vars)
			if strings.Join(got, ",") != strings.Join(c.wantNames, ",") {
				t.Fatalf("nombres/orden distintos:\n got: %v\nwant: %v", got, c.wantNames)
			}
			if c.wantWarn == "" && warn != "" {
				t.Fatalf("aviso inesperado: %q", warn)
			}
			if c.wantWarn != "" && !strings.Contains(warn, c.wantWarn) {
				t.Fatalf("aviso %q no contiene %q", warn, c.wantWarn)
			}
			// Los caminos degradados DEBEN caer a default: exportar el nombre
			// del perfil roto dejaría la terminal creyendo que está en él.
			if c.wantWarn != "" && len(vars) == 1 {
				if v := envValue(t, vars, "CCP_PROFILE"); v != "default" {
					t.Fatalf("CCP_PROFILE=%q, se esperaba default", v)
				}
			}
		})
	}
}

// El token es el único valor que sale de disco (no de ccp.yaml): comprobamos
// que EnvPairs lo lee de verdad y no deja el nombre del perfil por ahí.
func TestEnvPairsToken(t *testing.T) {
	home, cfg := envFixture(t)
	vars, warn := EnvPairs(home, "deepseek", cfg)
	if warn != "" {
		t.Fatalf("aviso inesperado: %q", warn)
	}
	if got := envValue(t, vars, "ANTHROPIC_AUTH_TOKEN"); got != "sk-deepseek" {
		t.Fatalf("token=%q, want sk-deepseek", got)
	}
	if got := envValue(t, vars, "CLAUDE_CONFIG_DIR"); got != home+"/profiles/deepseek/cc-home" {
		t.Fatalf("CLAUDE_CONFIG_DIR=%q", got)
	}
}

// ---------------------------------------------------------------------------
// 2) EnvForChild: limpieza del entorno heredado.
// ---------------------------------------------------------------------------

func TestEnvForChild(t *testing.T) {
	home, cfg := envFixture(t)

	// Un entorno «sucio»: viene de una terminal que estaba en un proveedor.
	base := []string{
		"PATH=/usr/bin",
		"CLAUDE_CONFIG_DIR=/viejo/cc-home",
		"ANTHROPIC_BASE_URL=https://viejo.example",
		"ANTHROPIC_AUTH_TOKEN=sk-viejo",
		"ENABLE_TOOL_SEARCH=false",
		"CCP_PROFILE=viejo",
		"HOME=/home/tester",
		"RAREZA_SIN_IGUAL", // entrada sin '=': se conserva tal cual
	}

	cases := []struct {
		name       string
		profile    string
		wantAbsent []string
		wantPairs  map[string]string
	}{
		{
			name:    "official limpia todo lo del proveedor previo",
			profile: "work",
			wantAbsent: []string{
				"ANTHROPIC_BASE_URL", "ANTHROPIC_AUTH_TOKEN", "ENABLE_TOOL_SEARCH",
			},
			wantPairs: map[string]string{
				"CLAUDE_CONFIG_DIR": home + "/profiles/work/cc-home",
				"CCP_PROFILE":       "work",
				"PATH":              "/usr/bin",
				"HOME":              "/home/tester",
			},
		},
		{
			name:       "default deja el entorno sin ninguna var gestionada salvo la marca",
			profile:    "default",
			wantAbsent: []string{"CLAUDE_CONFIG_DIR", "ANTHROPIC_BASE_URL", "ANTHROPIC_AUTH_TOKEN", "ENABLE_TOOL_SEARCH"},
			wantPairs:  map[string]string{"CCP_PROFILE": "default"},
		},
		{
			name:       "kimi pisa el token viejo con el suyo",
			profile:    "kimi",
			wantAbsent: []string{"API_TIMEOUT_MS"}, // Extra de glm, no de kimi
			wantPairs: map[string]string{
				"ANTHROPIC_AUTH_TOKEN":            "sk-kimi",
				"ENABLE_TOOL_SEARCH":              "false",
				"CLAUDE_CODE_AUTO_COMPACT_WINDOW": "262144",
				"CCP_PROFILE":                     "kimi",
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out := EnvForChild(base, home, c.profile, cfg)

			seen := map[string]int{}
			vals := map[string]string{}
			for _, kv := range out {
				i := strings.IndexByte(kv, '=')
				if i < 0 {
					seen[kv]++
					continue
				}
				seen[kv[:i]]++
				vals[kv[:i]] = kv[i+1:]
			}
			// Sin duplicados: exec.Cmd no define cuál gana si repites un nombre.
			for name, n := range seen {
				if n > 1 {
					t.Errorf("%s aparece %d veces", name, n)
				}
			}
			if seen["RAREZA_SIN_IGUAL"] != 1 {
				t.Errorf("se perdió la entrada sin '=' del entorno base")
			}
			for _, name := range c.wantAbsent {
				if _, ok := vals[name]; ok {
					t.Errorf("%s debería haberse limpiado (=%q)", name, vals[name])
				}
			}
			for name, want := range c.wantPairs {
				if vals[name] != want {
					t.Errorf("%s=%q, want %q", name, vals[name], want)
				}
			}
		})
	}
}

// EnvForChild no debe mutar el slice base que le pasan (os.Environ() se reutiliza
// entre hops del supervisor).
func TestEnvForChildNoMutaBase(t *testing.T) {
	home, cfg := envFixture(t)
	base := []string{"PATH=/usr/bin", "CCP_PROFILE=viejo"}
	before := strings.Join(base, "\x00")
	_ = EnvForChild(base, home, "kimi", cfg)
	if after := strings.Join(base, "\x00"); after != before {
		t.Fatalf("base mutado:\n antes: %q\ndespués: %q", before, after)
	}
}

// Un base vacío (proceso sin entorno) sigue produciendo exactamente el delta.
func TestEnvForChildBaseVacio(t *testing.T) {
	home, cfg := envFixture(t)
	out := EnvForChild(nil, home, "work", cfg)
	want := []string{"CLAUDE_CONFIG_DIR=" + home + "/profiles/work/cc-home", "CCP_PROFILE=work"}
	if strings.Join(out, "\n") != strings.Join(want, "\n") {
		t.Fatalf("got %v, want %v", out, want)
	}
}

// ---------------------------------------------------------------------------
// 3) La invariante: EnvDelta y EnvPairs no pueden divergir.
// ---------------------------------------------------------------------------

// parseEnvDelta descompone la salida eval-able en (nombre de las vars unset,
// pares export, hubo echo de aviso). Se parsea el TEXTO a propósito: si el
// refactor de EnvDelta se revirtiera algún día, este test seguiría siendo la
// red que impide la divergencia.
func parseEnvDelta(t *testing.T, s string) (unset []string, exports []EnvVar, warned bool) {
	t.Helper()
	for _, line := range strings.Split(strings.TrimRight(s, "\n"), "\n") {
		switch {
		case strings.HasPrefix(line, "unset "):
			unset = strings.Fields(strings.TrimPrefix(line, "unset "))
		case strings.HasPrefix(line, "export "):
			rest := strings.TrimPrefix(line, "export ")
			i := strings.IndexByte(rest, '=')
			if i < 0 {
				t.Fatalf("export sin '=': %q", line)
			}
			exports = append(exports, EnvVar{Name: rest[:i], Value: rest[i+1:]})
		case strings.HasPrefix(line, "echo "):
			warned = true
		default:
			t.Fatalf("línea inesperada en el delta: %q", line)
		}
	}
	return unset, exports, warned
}

func TestEnvDeltaCoincideConEnvPairs(t *testing.T) {
	home, cfg := envFixture(t)

	profiles := []string{"default", "work", "deepseek", "nokey", "kimi", "glm", "no-existe", "raro"}
	for _, prof := range profiles {
		t.Run(prof, func(t *testing.T) {
			unset, exports, warned := parseEnvDelta(t, EnvDelta(home, prof, cfg))

			// El unset incondicional debe seguir cubriendo TODO lo gestionado:
			// es lo que garantiza que no queden restos del perfil anterior.
			if strings.Join(unset, " ") != CCPManagedVars {
				t.Errorf("unset distinto de CCPManagedVars:\n got: %q\nwant: %q",
					strings.Join(unset, " "), CCPManagedVars)
			}

			vars, warn := EnvPairs(home, prof, cfg)
			if len(exports) != len(vars) {
				t.Fatalf("EnvDelta exporta %d vars y EnvPairs devuelve %d:\n%v\nvs\n%v",
					len(exports), len(vars), exports, envNames(vars))
			}
			for i, want := range vars {
				if exports[i].Name != want.Name {
					t.Fatalf("var #%d: EnvDelta=%s EnvPairs=%s", i, exports[i].Name, want.Name)
				}
				// El delta lleva el valor pasado por shellQuote; comparamos
				// contra el mismo quoting para validar orden y contenido sin
				// reimplementar el unquote de bash.
				if exports[i].Value != shellQuote(want.Value) {
					t.Fatalf("var %s: delta=%q, quote(pairs)=%q",
						want.Name, exports[i].Value, shellQuote(want.Value))
				}
			}
			if warned != (warn != "") {
				t.Fatalf("aviso descuadrado: delta.echo=%v, EnvPairs.warn=%q", warned, warn)
			}
			// Y el entorno del hijo debe terminar con exactamente esas vars.
			child := EnvForChild(nil, home, prof, cfg)
			if len(child) != len(vars) {
				t.Fatalf("EnvForChild devolvió %d entradas, EnvPairs %d", len(child), len(vars))
			}
		})
	}
}

// Misma invariante sobre el fixture del golden (valores reales del oráculo,
// incluyendo el perfil official 'work' y rutas con __CCP_HOME__ sustituido).
func TestEnvDeltaCoincideConEnvPairsGolden(t *testing.T) {
	home, cfg := materializeHome(t)
	for _, prof := range []string{"default", "work", "deepseek", "kimi", "glm", "nope"} {
		t.Run(prof, func(t *testing.T) {
			_, exports, warned := parseEnvDelta(t, EnvDelta(home, prof, cfg))
			vars, warn := EnvPairs(home, prof, cfg)
			if len(exports) != len(vars) {
				t.Fatalf("delta=%d vars, pairs=%d", len(exports), len(vars))
			}
			for i := range vars {
				if exports[i].Name != vars[i].Name || exports[i].Value != shellQuote(vars[i].Value) {
					t.Fatalf("var #%d: delta=%v pairs=%v", i, exports[i], vars[i])
				}
			}
			if warned != (warn != "") {
				t.Fatalf("aviso descuadrado: %v vs %q", warned, warn)
			}
		})
	}
}
