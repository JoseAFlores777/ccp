package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// seedAutoHome escribe un ccp.yaml mínimo con perfiles y reglas para los tests
// de política. Nunca toca ~/.config/ccp: home siempre es un t.TempDir().
func seedAutoHome(t *testing.T, home string, yamlBody string) {
	t.Helper()
	if err := os.WriteFile(yamlPath(home), []byte(yamlBody), 0o644); err != nil {
		t.Fatal(err)
	}
}

const autoBaseYAML = `version: 2
defaults:
  base_url: https://api.deepseek.com/anthropic
  model_pro: deepseek-chat
  model_flash: deepseek-chat
  effort: high
  editor: nano
profiles:
  personal-cc:
    type: official
  app-cc:
    type: official
  emco-cc:
    type: official
  personal-deepseek:
    type: deepseek
rules:
  - path: /work/personal
    profile: personal-cc
  - path: /work/emco
    profile: emco-cc
authored: []
`

// ---------------------------------------------------------------------------
// Round-trip: el bloque debe sobrevivir Save->Load con todos sus campos, y
// convivir con comentarios y claves desconocidas (es la garantía de que un
// `ccp rule set` posterior no borra la política).
// ---------------------------------------------------------------------------

func TestAutoHandoffYAMLRoundTrip(t *testing.T) {
	home := t.TempDir()
	seedAutoHome(t, home, autoBaseYAML+`future_key: keepme
`)

	cfg, err := Load(home)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	cfg.AutoHandoff = &AutoHandoff{
		Enabled: true,
		Policies: map[string]AutoPolicy{
			"default": {
				Fallback:    []string{"app-cc", "personal-deepseek"},
				Threshold:   85,
				MinDwell:    "20m",
				MaxHops:     6,
				ReturnCheck: "10m",
				Cooldown:    AutoCooldown{Strategy: CooldownResetsAt, Fallback: "1h"},
			},
			"trabajo": {Fallback: []string{}},
		},
		AllowFrom: map[string][]string{
			"personal-cc": {"personal-cc", "app-cc"},
			"emco-cc":     {"emco-cc"},
		},
		Hooks: []string{"personal-cc"},
	}
	if err := Save(home, cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}

	raw, err := os.ReadFile(yamlPath(home))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "auto_handoff:") {
		t.Fatalf("ccp.yaml sin bloque auto_handoff:\n%s", raw)
	}
	if !strings.Contains(string(raw), "future_key: keepme") {
		t.Errorf("se perdió la clave desconocida:\n%s", raw)
	}

	got, err := Load(home)
	if err != nil {
		t.Fatalf("Load tras Save: %v", err)
	}
	if got.AutoHandoff == nil {
		t.Fatal("AutoHandoff nil tras round-trip")
	}
	if got.Version != 2 {
		t.Errorf("version = %d, want 2 (el bloque es aditivo)", got.Version)
	}
	pol := got.AutoHandoff.Policies["default"]
	if len(pol.Fallback) != 2 || pol.Fallback[0] != "app-cc" || pol.Fallback[1] != "personal-deepseek" {
		t.Errorf("fallback = %v", pol.Fallback)
	}
	if pol.Threshold != 85 || pol.MinDwell != "20m" || pol.MaxHops != 6 || pol.ReturnCheck != "10m" {
		t.Errorf("política mal deserializada: %+v", pol)
	}
	if pol.Cooldown.Strategy != CooldownResetsAt || pol.Cooldown.Fallback != "1h" {
		t.Errorf("cooldown = %+v", pol.Cooldown)
	}
	if _, ok := got.AutoHandoff.Policies["trabajo"]; !ok {
		t.Error("se perdió la política 'trabajo'")
	}
	if len(got.AutoHandoff.AllowFrom["personal-cc"]) != 2 {
		t.Errorf("allow_from = %v", got.AutoHandoff.AllowFrom)
	}
	if len(got.AutoHandoff.Hooks) != 1 || got.AutoHandoff.Hooks[0] != "personal-cc" {
		t.Errorf("hooks = %v", got.AutoHandoff.Hooks)
	}
	// Extra no debe duplicar la clave ya modelada.
	if _, dup := got.Extra["auto_handoff"]; dup {
		t.Error("auto_handoff duplicado en Extra (falta en knownTopKeys)")
	}
}

// ---------------------------------------------------------------------------
// Effective: defaults y validación.
// ---------------------------------------------------------------------------

func TestAutoPolicyEffectiveDefaults(t *testing.T) {
	eff, err := AutoPolicy{Fallback: []string{"a", "", " b "}}.Effective("default")
	if err != nil {
		t.Fatalf("Effective: %v", err)
	}
	if eff.Name != "default" {
		t.Errorf("name = %q", eff.Name)
	}
	if eff.Threshold != DefaultAutoThreshold {
		t.Errorf("threshold = %d, want %d", eff.Threshold, DefaultAutoThreshold)
	}
	if eff.MinDwell != DefaultAutoMinDwell {
		t.Errorf("min_dwell = %v, want %v", eff.MinDwell, DefaultAutoMinDwell)
	}
	if eff.MaxHops != DefaultAutoMaxHops {
		t.Errorf("max_hops = %d, want %d", eff.MaxHops, DefaultAutoMaxHops)
	}
	if eff.ReturnCheck != DefaultAutoReturnCheck {
		t.Errorf("return_check = %v, want %v", eff.ReturnCheck, DefaultAutoReturnCheck)
	}
	// El guard de inactividad del regreso proactivo viene ARMADO por defecto: es
	// lo único que impide que el temporizador mate una sesión sana por reloj.
	if eff.ReturnIdle != DefaultAutoReturnIdle {
		t.Errorf("return_idle = %v, want %v", eff.ReturnIdle, DefaultAutoReturnIdle)
	}
	if eff.CooldownStrategy != CooldownResetsAt {
		t.Errorf("strategy = %q", eff.CooldownStrategy)
	}
	if eff.CooldownFallback != DefaultAutoCooldown {
		t.Errorf("cooldown fallback = %v", eff.CooldownFallback)
	}
	if len(eff.Fallback) != 2 || eff.Fallback[1] != "b" {
		t.Errorf("fallback = %v (deben caer vacíos y recortarse espacios)", eff.Fallback)
	}
}

func TestAutoPolicyEffectiveOverridesAndErrors(t *testing.T) {
	tests := []struct {
		name    string
		pol     AutoPolicy
		wantErr []string // subcadenas que el mensaje debe contener
		check   func(t *testing.T, e EffectivePolicy)
	}{
		{
			name: "overrides completos",
			pol: AutoPolicy{
				Threshold:   50,
				MinDwell:    "90s",
				MaxHops:     12,
				ReturnCheck: "1m30s",
				ReturnIdle:  "45s",
				Cooldown:    AutoCooldown{Strategy: CooldownFixed, Fallback: "2h"},
			},
			check: func(t *testing.T, e EffectivePolicy) {
				if e.Threshold != 50 || e.MaxHops != 12 {
					t.Errorf("numéricos = %d/%d", e.Threshold, e.MaxHops)
				}
				if e.MinDwell != 90*time.Second || e.ReturnCheck != 90*time.Second {
					t.Errorf("duraciones = %v/%v", e.MinDwell, e.ReturnCheck)
				}
				if e.ReturnIdle != 45*time.Second {
					t.Errorf("return_idle = %v", e.ReturnIdle)
				}
				if e.CooldownStrategy != CooldownFixed || e.CooldownFallback != 2*time.Hour {
					t.Errorf("cooldown = %q/%v", e.CooldownStrategy, e.CooldownFallback)
				}
			},
		},
		{
			name:    "threshold demasiado alto",
			pol:     AutoPolicy{Threshold: 900},
			wantErr: []string{"threshold", "900", "1..100"},
		},
		{
			name:    "threshold negativo",
			pol:     AutoPolicy{Threshold: -5},
			wantErr: []string{"threshold", "-5"},
		},
		{
			name:    "max_hops negativo",
			pol:     AutoPolicy{MaxHops: -1},
			wantErr: []string{"max_hops", "-1"},
		},
		{
			name:    "min_dwell inválido",
			pol:     AutoPolicy{MinDwell: "20 minutos"},
			wantErr: []string{"min_dwell", "20 minutos"},
		},
		{
			name:    "return_check inválido",
			pol:     AutoPolicy{ReturnCheck: "diez"},
			wantErr: []string{"return_check", "diez"},
		},
		{
			name:    "return_idle inválido",
			pol:     AutoPolicy{ReturnIdle: "noventa"},
			wantErr: []string{"return_idle", "noventa"},
		},
		{
			// 0s es el opt-out EXPLÍCITO del guard, no un error: quien lo escribe
			// acepta que el regreso interrumpa un turno en vuelo.
			name: "return_idle 0s desactiva el guard",
			pol:  AutoPolicy{ReturnIdle: "0s"},
			check: func(t *testing.T, e EffectivePolicy) {
				if e.ReturnIdle != 0 {
					t.Errorf("return_idle = %v, want 0", e.ReturnIdle)
				}
			},
		},
		{
			name:    "cooldown.fallback inválido",
			pol:     AutoPolicy{Cooldown: AutoCooldown{Fallback: "1 hora"}},
			wantErr: []string{"cooldown.fallback", "1 hora"},
		},
		{
			name:    "duración negativa",
			pol:     AutoPolicy{MinDwell: "-5m"},
			wantErr: []string{"min_dwell", "negativo"},
		},
		{
			name:    "strategy desconocida",
			pol:     AutoPolicy{Cooldown: AutoCooldown{Strategy: "aleatorio"}},
			wantErr: []string{"strategy", "aleatorio", CooldownResetsAt, CooldownFixed},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			eff, err := tc.pol.Effective("overnight")
			if len(tc.wantErr) > 0 {
				if err == nil {
					t.Fatalf("se esperaba error, got %+v", eff)
				}
				for _, want := range tc.wantErr {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("error %q no menciona %q", err, want)
					}
				}
				if !strings.Contains(err.Error(), "overnight") {
					t.Errorf("error %q no nombra la política", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Effective: %v", err)
			}
			tc.check(t, eff)
		})
	}
}

// ---------------------------------------------------------------------------
// ResolveAutoChain.
// ---------------------------------------------------------------------------

// autoCfg arma un Config en memoria (sin disco) para los casos de resolución.
func autoCfg(ah *AutoHandoff) *Config {
	return &Config{
		Version: 2,
		Profiles: map[string]Profile{
			"personal-cc":       {Type: "official"},
			"app-cc":            {Type: "official"},
			"emco-cc":           {Type: "official"},
			"personal-deepseek": {Type: "deepseek"},
		},
		Rules: []Rule{
			{Path: "/work/personal", Profile: "personal-cc"},
			{Path: "/work/emco", Profile: "emco-cc"},
		},
		AutoHandoff: ah,
	}
}

func fullChainPolicies() map[string]AutoPolicy {
	return map[string]AutoPolicy{
		"default": {Fallback: []string{"personal-cc", "app-cc", "personal-deepseek"}},
	}
}

func TestResolveAutoChain(t *testing.T) {
	tests := []struct {
		name         string
		ah           *AutoHandoff
		policy       string
		cwd          string
		wantErr      []string
		wantPrimary  string
		wantFallback []string
		wantDenied   []string
	}{
		{
			name:    "bloque ausente apunta a ccp auto init",
			ah:      nil,
			cwd:     "/work/personal/repo",
			wantErr: []string{"auto_handoff", "ccp auto init"},
		},
		{
			name:    "bloque deshabilitado",
			ah:      &AutoHandoff{Enabled: false, Policies: fullChainPolicies()},
			cwd:     "/work/personal/repo",
			wantErr: []string{"deshabilitado"},
		},
		{
			name:    "política inexistente lista las que hay",
			ah:      &AutoHandoff{Enabled: true, Policies: fullChainPolicies()},
			policy:  "nocturna",
			cwd:     "/work/personal/repo",
			wantErr: []string{"nocturna", "default"},
		},
		{
			name: "perfil de fallback inexistente",
			ah: &AutoHandoff{Enabled: true, Policies: map[string]AutoPolicy{
				"default": {Fallback: []string{"app-cc", "fantasma"}},
			}},
			cwd:     "/work/personal/repo",
			wantErr: []string{"fantasma", "no existe"},
		},
		{
			name: "duración inválida propaga desde Effective",
			ah: &AutoHandoff{Enabled: true, Policies: map[string]AutoPolicy{
				"default": {Fallback: []string{"app-cc"}, MinDwell: "20x"},
			}},
			cwd:     "/work/personal/repo",
			wantErr: []string{"min_dwell", "20x"},
		},
		{
			name:         "sin gate: allow_from ausente permite todo y filtra el primario",
			ah:           &AutoHandoff{Enabled: true, Policies: fullChainPolicies()},
			cwd:          "/work/personal/repo/sub",
			wantPrimary:  "personal-cc",
			wantFallback: []string{"app-cc", "personal-deepseek"},
		},
		{
			name: "sin gate: allow_from vacío equivale a ausente",
			ah: &AutoHandoff{Enabled: true, Policies: fullChainPolicies(),
				AllowFrom: map[string][]string{}},
			cwd:          "/work/personal/repo",
			wantPrimary:  "personal-cc",
			wantFallback: []string{"app-cc", "personal-deepseek"},
		},
		{
			name: "gate con entrada: filtra los no listados a Denied",
			ah: &AutoHandoff{Enabled: true, Policies: fullChainPolicies(),
				AllowFrom: map[string][]string{
					"personal-cc": {"personal-cc", "app-cc"},
					"emco-cc":     {"emco-cc"},
				}},
			cwd:          "/work/personal/repo",
			wantPrimary:  "personal-cc",
			wantFallback: []string{"app-cc"},
			wantDenied:   []string{"personal-deepseek"},
		},
		{
			name: "gate declarado sin entrada para el primario: deny total",
			ah: &AutoHandoff{Enabled: true, Policies: fullChainPolicies(),
				AllowFrom: map[string][]string{
					"personal-cc": {"personal-cc", "app-cc"},
				}},
			cwd:          "/work/emco/cliente",
			wantPrimary:  "emco-cc",
			wantFallback: nil,
			wantDenied:   []string{"personal-cc", "app-cc", "personal-deepseek"},
		},
		{
			name: "gate con entrada vacía: deny total explícito",
			ah: &AutoHandoff{Enabled: true, Policies: fullChainPolicies(),
				AllowFrom: map[string][]string{"emco-cc": {}}},
			cwd:          "/work/emco/cliente",
			wantPrimary:  "emco-cc",
			wantFallback: nil,
			wantDenied:   []string{"personal-cc", "app-cc", "personal-deepseek"},
		},
		{
			name: "primario listado en fallback se descarta en silencio (y dedup)",
			ah: &AutoHandoff{Enabled: true, Policies: map[string]AutoPolicy{
				"default": {Fallback: []string{"personal-cc", "app-cc", "app-cc", "personal-cc"}},
			}},
			cwd:          "/work/personal",
			wantPrimary:  "personal-cc",
			wantFallback: []string{"app-cc"},
		},
		{
			name: "cwd sin regla: el primario es default y 'default' vale de destino",
			ah: &AutoHandoff{Enabled: true, Policies: map[string]AutoPolicy{
				"default": {Fallback: []string{"default", "app-cc"}},
			}},
			cwd:          "/tmp/suelto",
			wantPrimary:  "default",
			wantFallback: []string{"app-cc"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rc, err := ResolveAutoChain(t.TempDir(), autoCfg(tc.ah), tc.policy, tc.cwd)
			if len(tc.wantErr) > 0 {
				if err == nil {
					t.Fatalf("se esperaba error, got %+v", rc)
				}
				for _, want := range tc.wantErr {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("error %q no menciona %q", err, want)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("ResolveAutoChain: %v", err)
			}
			if rc.Primary != tc.wantPrimary {
				t.Errorf("primary = %q, want %q", rc.Primary, tc.wantPrimary)
			}
			if !equalStrings(rc.Fallback, tc.wantFallback) {
				t.Errorf("fallback = %v, want %v", rc.Fallback, tc.wantFallback)
			}
			if !equalStrings(rc.Denied, tc.wantDenied) {
				t.Errorf("denied = %v, want %v", rc.Denied, tc.wantDenied)
			}
			if rc.Policy.Name == "" {
				t.Error("Policy.Name vacío: el supervisor lo usa en la traza")
			}
		})
	}
}

// TestResolveAutoChainLoadsFromHome cubre el atajo cfg==nil: se lee ccp.yaml del
// home indicado (nunca del real, es un t.TempDir()).
func TestResolveAutoChainLoadsFromHome(t *testing.T) {
	home := t.TempDir()
	seedAutoHome(t, home, autoBaseYAML+`auto_handoff:
  enabled: true
  policies:
    default:
      fallback: [app-cc]
      min_dwell: 5m
`)
	rc, err := ResolveAutoChain(home, nil, "", "/work/personal/repo")
	if err != nil {
		t.Fatalf("ResolveAutoChain: %v", err)
	}
	if rc.Primary != "personal-cc" || !equalStrings(rc.Fallback, []string{"app-cc"}) {
		t.Errorf("rc = %+v", rc)
	}
	if rc.Policy.MinDwell != 5*time.Minute {
		t.Errorf("min_dwell = %v", rc.Policy.MinDwell)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// ---------------------------------------------------------------------------
// AutoInit.
// ---------------------------------------------------------------------------

func TestAutoInitSeedsAndIsIdempotent(t *testing.T) {
	home := t.TempDir()
	seedAutoHome(t, home, autoBaseYAML)

	if err := AutoInit(home, false); err != nil {
		t.Fatalf("AutoInit: %v", err)
	}
	cfg, err := Load(home)
	if err != nil {
		t.Fatal(err)
	}
	ah := cfg.AutoHandoff
	if ah == nil || !ah.Enabled {
		t.Fatalf("bloque no sembrado: %+v", ah)
	}
	pol, ok := ah.Policies["default"]
	if !ok {
		t.Fatal("falta la política 'default'")
	}
	if !equalStrings(pol.Fallback, []string{"app-cc", "emco-cc", "personal-cc", "personal-deepseek"}) {
		t.Errorf("fallback sembrado = %v", pol.Fallback)
	}
	if pol.Threshold != DefaultAutoThreshold || pol.MinDwell != "20m0s" || pol.MaxHops != DefaultAutoMaxHops {
		t.Errorf("defaults sembrados mal: %+v", pol)
	}
	// El gate arranca conservador: cada perfil se permite a sí mismo + los
	// oficiales, nunca al de provider.
	if !equalStrings(ah.AllowFrom["personal-cc"], []string{"personal-cc", "app-cc", "emco-cc"}) {
		t.Errorf("allow_from[personal-cc] = %v", ah.AllowFrom["personal-cc"])
	}
	if !equalStrings(ah.AllowFrom["personal-deepseek"],
		[]string{"personal-deepseek", "app-cc", "emco-cc", "personal-cc"}) {
		t.Errorf("allow_from[personal-deepseek] = %v", ah.AllowFrom["personal-deepseek"])
	}
	for from, to := range ah.AllowFrom {
		for _, dst := range to {
			if dst == "personal-deepseek" && from != "personal-deepseek" {
				t.Errorf("allow_from[%s] incluye el provider: %v", from, to)
			}
		}
	}
	// La política sembrada debe ser válida para Effective (contrato con el supervisor).
	if _, err := pol.Effective("default"); err != nil {
		t.Errorf("la política sembrada no valida: %v", err)
	}

	// Idempotencia: una segunda pasada sin force no toca el archivo.
	before, err := os.ReadFile(yamlPath(home))
	if err != nil {
		t.Fatal(err)
	}
	cfg.AutoHandoff.Policies["default"] = AutoPolicy{Fallback: []string{"app-cc"}, Threshold: 42}
	if err := Save(home, cfg); err != nil {
		t.Fatal(err)
	}
	edited, err := os.ReadFile(yamlPath(home))
	if err != nil {
		t.Fatal(err)
	}
	if err := AutoInit(home, false); err != nil {
		t.Fatalf("AutoInit idempotente: %v", err)
	}
	after, err := os.ReadFile(yamlPath(home))
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(edited) {
		t.Errorf("AutoInit pisó una edición manual:\n--- antes ---\n%s\n--- después ---\n%s", edited, after)
	}

	// force sí regenera: vuelve al estado sembrado original.
	if err := AutoInit(home, true); err != nil {
		t.Fatalf("AutoInit --force: %v", err)
	}
	forced, err := os.ReadFile(yamlPath(home))
	if err != nil {
		t.Fatal(err)
	}
	if string(forced) != string(before) {
		t.Errorf("force no regeneró el bloque:\n--- want ---\n%s\n--- got ---\n%s", before, forced)
	}
}

// TestAutoInitSinPerfiles: un home recién instalado no tiene perfiles; sembrar
// debe funcionar igual (bloque vacío pero válido) en vez de reventar.
func TestAutoInitSinPerfiles(t *testing.T) {
	home := t.TempDir()
	if err := AutoInit(home, false); err != nil {
		t.Fatalf("AutoInit: %v", err)
	}
	cfg, err := Load(home)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AutoHandoff == nil || len(cfg.AutoHandoff.Policies) != 1 {
		t.Fatalf("bloque = %+v", cfg.AutoHandoff)
	}
	if len(cfg.AutoHandoff.Policies["default"].Fallback) != 0 {
		t.Errorf("fallback = %v, want vacío", cfg.AutoHandoff.Policies["default"].Fallback)
	}
	// Sin perfiles, resolver es error de fallback vacío pero NO debe fallar.
	rc, err := ResolveAutoChain(home, cfg, "", "/tmp/x")
	if err != nil {
		t.Fatalf("ResolveAutoChain: %v", err)
	}
	if rc.Primary != "default" || len(rc.Fallback) != 0 {
		t.Errorf("rc = %+v", rc)
	}
}

// TestAutoHandoffPreservaClavesDesconocidas fija que el bloque `auto_handoff`
// sobrevive intacto a un binario que no entiende parte de su contenido.
//
// `auto_handoff` está en knownTopKeys, así que el catch-all de Config protege la
// clave entera pero NO su interior: sin un inline propio en AutoHandoff y en
// AutoPolicy, una clave nueva se perdía en el siguiente Save — y "el siguiente
// Save" es cualquier `ccp rule set` o `ccp profile add`, no una operación de
// auto. O sea: instalar una versión anterior y tocar una regla te borraba parte
// de la política sin decir nada.
//
// El test escribe el yaml a mano porque es exactamente el caso real: el archivo
// lo produjo un ccp más nuevo que el que lo está leyendo.
func TestAutoHandoffPreservaClavesDesconocidas(t *testing.T) {
	home := t.TempDir()
	raw := `version: 2
profiles:
  work:
    type: official
auto_handoff:
  enabled: true
  futuro_del_bloque: hola
  policies:
    default:
      fallback: [work]
      threshold: 90
      futuro_de_politica: 42
`
	if err := os.WriteFile(filepath.Join(home, "ccp.yaml"), []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}

	c, err := Load(home)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.AutoHandoff == nil {
		t.Fatal("no se cargó auto_handoff")
	}
	if got := c.AutoHandoff.Extra["futuro_del_bloque"]; got != "hola" {
		t.Errorf("Extra del bloque = %v, quería \"hola\" (todo: %v)", got, c.AutoHandoff.Extra)
	}
	pol := c.AutoHandoff.Policies["default"]
	if _, ok := pol.Extra["futuro_de_politica"]; !ok {
		t.Errorf("Extra de la política no tiene futuro_de_politica: %v", pol.Extra)
	}
	// Las claves CONOCIDAS no deben filtrarse al catch-all: si lo hicieran, se
	// escribirían dos veces al guardar.
	for _, k := range []string{"enabled", "policies", "fallback", "threshold"} {
		if _, ok := c.AutoHandoff.Extra[k]; ok {
			t.Errorf("Extra del bloque se quedó con la clave conocida %q", k)
		}
		if _, ok := pol.Extra[k]; ok {
			t.Errorf("Extra de la política se quedó con la clave conocida %q", k)
		}
	}

	// Un Save cualquiera (aquí, tras tocar algo ajeno al bloque) las conserva.
	c.Lang = "en"
	if err := Save(home, c); err != nil {
		t.Fatalf("Save: %v", err)
	}
	out, err := os.ReadFile(filepath.Join(home, "ccp.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"futuro_del_bloque", "futuro_de_politica"} {
		if !strings.Contains(string(out), want) {
			t.Errorf("el round-trip perdió %q:\n%s", want, out)
		}
	}
}
