package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JoseAFlores777/ccp/internal/core"
)

// auto_test.go — `ccp auto` y los dos sensores internos.
//
// Todo corre contra un CCP_HOME de t.TempDir(); CCP_CLAUDE_SRC apunta también a
// un temporal para que ProfileSync no lea el ~/.claude real del usuario.

// autoTestHome monta un home con perfiles y devuelve su ruta.
func autoTestHome(t *testing.T, profiles ...string) string {
	t.Helper()
	home := t.TempDir()
	src := t.TempDir()
	t.Setenv("CCP_HOME", home)
	t.Setenv("CCP_CLAUDE_SRC", src)
	t.Setenv("CCP_LANG", "es")
	t.Setenv("NO_COLOR", "1")

	// El binario de la capa se fija a mano: por defecto el init() del paquete
	// pone la ruta del binario de test, que no dice nada en un assert.
	prev := core.AutoHooksBin()
	core.SetAutoHooksBin("/opt/ccp")
	t.Cleanup(func() { core.SetAutoHooksBin(prev) })

	for _, p := range profiles {
		if err := core.ProfileAddOfficial(home, p); err != nil {
			t.Fatal(err)
		}
	}
	return home
}

// autoSettingsOf lee el settings.json generado de un perfil.
func autoSettingsOf(t *testing.T, home, profile string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(home, "profiles", profile, "cc-home", "settings.json"))
	if err != nil {
		t.Fatalf("settings.json de %q: %v", profile, err)
	}
	return string(data)
}

func TestAutoInitYSegundaVez(t *testing.T) {
	home := autoTestHome(t, "work", "personal")

	var out, errb bytes.Buffer
	if code := dispatchAuto([]string{"init"}, &out, &errb); code != 0 {
		t.Fatalf("init exit = %d, stderr=%s", code, errb.String())
	}
	cfg, err := core.Load(home)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AutoHandoff == nil || !cfg.AutoHandoff.Enabled {
		t.Fatal("init no sembró auto_handoff")
	}

	// Segunda vez sin --force: lo dice y sale 1 (no miente diciendo «hecho»).
	out.Reset()
	errb.Reset()
	if code := dispatchAuto([]string{"init"}, &out, &errb); code != 1 {
		t.Fatalf("init repetido exit = %d, want 1", code)
	}
	if !strings.Contains(errb.String(), "--force") {
		t.Errorf("el mensaje debe apuntar a --force: %q", errb.String())
	}

	// Con --force sí.
	out.Reset()
	errb.Reset()
	if code := dispatchAuto([]string{"init", "--force"}, &out, &errb); code != 0 {
		t.Fatalf("init --force exit = %d, stderr=%s", code, errb.String())
	}
}

func TestAutoInstallUninstallIdempotentes(t *testing.T) {
	home := autoTestHome(t, "work", "personal")
	var out, errb bytes.Buffer
	if code := dispatchAuto([]string{"init"}, &out, &errb); code != 0 {
		t.Fatalf("init: %s", errb.String())
	}

	// install de un perfil concreto.
	out.Reset()
	errb.Reset()
	if code := dispatchAuto([]string{"install", "work"}, &out, &errb); code != 0 {
		t.Fatalf("install exit = %d, stderr=%s", code, errb.String())
	}
	cfg, _ := core.Load(home)
	if !core.AutoHooksEnabled(cfg, "work") || core.AutoHooksEnabled(cfg, "personal") {
		t.Fatalf("hooks = %v, want [work]", cfg.AutoHandoff.Hooks)
	}
	s := autoSettingsOf(t, home, "work")
	if !strings.Contains(s, "/opt/ccp _limit-hook") || !strings.Contains(s, "/opt/ccp _statusline") {
		t.Fatalf("settings.json sin sensores: %s", s)
	}
	// `personal` no se tocó: su settings.json puede no existir aún (solo lo
	// genera una regeneración), pero si existe no puede llevar los sensores.
	if s2, err := os.ReadFile(filepath.Join(home, "profiles", "personal", "cc-home", "settings.json")); err == nil {
		if strings.Contains(string(s2), "_statusline") {
			t.Fatalf("personal no debería tener la capa: %s", s2)
		}
	}

	// install repetido: idempotente en el yaml y en el archivo.
	out.Reset()
	errb.Reset()
	if code := dispatchAuto([]string{"install", "work"}, &out, &errb); code != 0 {
		t.Fatalf("install repetido exit = %d", code)
	}
	cfg, _ = core.Load(home)
	if len(cfg.AutoHandoff.Hooks) != 1 {
		t.Fatalf("install repetido duplicó hooks: %v", cfg.AutoHandoff.Hooks)
	}
	if autoSettingsOf(t, home, "work") != s {
		t.Error("install repetido cambió el settings.json")
	}

	// install sin argumentos: todos los no-default.
	out.Reset()
	errb.Reset()
	if code := dispatchAuto([]string{"install"}, &out, &errb); code != 0 {
		t.Fatalf("install (todos) exit = %d, stderr=%s", code, errb.String())
	}
	cfg, _ = core.Load(home)
	if len(cfg.AutoHandoff.Hooks) != 2 {
		t.Fatalf("hooks = %v, want los 2 perfiles", cfg.AutoHandoff.Hooks)
	}

	// uninstall de uno: el otro sigue.
	out.Reset()
	errb.Reset()
	if code := dispatchAuto([]string{"uninstall", "work"}, &out, &errb); code != 0 {
		t.Fatalf("uninstall exit = %d, stderr=%s", code, errb.String())
	}
	cfg, _ = core.Load(home)
	if core.AutoHooksEnabled(cfg, "work") || !core.AutoHooksEnabled(cfg, "personal") {
		t.Fatalf("hooks = %v, want [personal]", cfg.AutoHandoff.Hooks)
	}
	if s := autoSettingsOf(t, home, "work"); strings.Contains(s, "_statusline") || strings.Contains(s, "_limit-hook") {
		t.Fatalf("uninstall no limpió el settings.json: %s", s)
	}

	// uninstall repetido: no es error, solo lo dice.
	out.Reset()
	errb.Reset()
	if code := dispatchAuto([]string{"uninstall", "work"}, &out, &errb); code != 0 {
		t.Fatalf("uninstall repetido exit = %d", code)
	}
}

func TestAutoInstallSinBloqueAutoHandoff(t *testing.T) {
	autoTestHome(t, "work")
	var out, errb bytes.Buffer
	if code := dispatchAuto([]string{"install"}, &out, &errb); code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.Contains(errb.String(), "auto init") {
		t.Errorf("el error debe apuntar a `ccp auto init`: %q", errb.String())
	}
}

func TestAutoInstallRechazaDefaultYDesconocido(t *testing.T) {
	autoTestHome(t, "work")
	var out, errb bytes.Buffer
	if code := dispatchAuto([]string{"init"}, &out, &errb); code != 0 {
		t.Fatal(errb.String())
	}
	for _, name := range []string{"default", "fantasma"} {
		out.Reset()
		errb.Reset()
		if code := dispatchAuto([]string{"install", name}, &out, &errb); code != 1 {
			t.Errorf("install %s: exit = %d, want 1", name, code)
		}
	}
}

func TestAutoStatusJSON(t *testing.T) {
	home := autoTestHome(t, "work", "personal")
	t.Setenv("PWD", "/repo")
	var out, errb bytes.Buffer
	if code := dispatchAuto([]string{"init"}, &out, &errb); code != 0 {
		t.Fatal(errb.String())
	}
	out.Reset()
	errb.Reset()
	if code := dispatchAuto([]string{"install", "work"}, &out, &errb); code != 0 {
		t.Fatal(errb.String())
	}
	// Muestra del statusLine ya sobre el umbral, con reset conocido.
	reset := time.Now().Add(2 * time.Hour).UTC().Truncate(time.Second)
	if err := core.WriteRateLimits(home, "work", core.RateLimits{
		FiveHour: core.Windowed{UsedPercentage: 97, ResetsAt: reset},
		SevenDay: core.Windowed{UsedPercentage: 40},
	}, time.Now()); err != nil {
		t.Fatal(err)
	}

	out.Reset()
	errb.Reset()
	if code := dispatchAuto([]string{"status", "--json"}, &out, &errb); code != 0 {
		t.Fatalf("status --json exit = %d, stderr=%s", code, errb.String())
	}
	var got autoStatusJSON
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("salida no es JSON (%v): %s", err, out.String())
	}
	if !got.Enabled {
		t.Error("enabled debe ser true tras init")
	}
	if got.Cwd != "/repo" {
		t.Errorf("cwd = %q", got.Cwd)
	}
	if got.Policy == nil || got.Policy.Name != "default" {
		t.Fatalf("policy = %+v", got.Policy)
	}
	if got.Policy.Threshold != core.DefaultAutoThreshold {
		t.Errorf("threshold = %d", got.Policy.Threshold)
	}
	if got.Primary != "default" {
		t.Errorf("primary = %q (sin reglas, el primario es default)", got.Primary)
	}
	var work *autoSensorJSON
	for i := range got.Sensors {
		if got.Sensors[i].Profile == "work" {
			work = &got.Sensors[i]
		}
	}
	if work == nil {
		t.Fatalf("sensors sin work: %+v", got.Sensors)
	}
	if !work.Installed || !work.HasSample {
		t.Errorf("work = %+v, want instalado y con muestra", *work)
	}
	if work.FiveHourPct != 97 {
		t.Errorf("five_hour_pct = %v", work.FiveHourPct)
	}
	if work.CooldownUntil != reset.Format(time.RFC3339) {
		t.Errorf("cooldown_until = %q, want %q", work.CooldownUntil, reset.Format(time.RFC3339))
	}
	// Arrays nunca null: `jq '.denied | length'` debe funcionar siempre.
	if !strings.Contains(out.String(), `"denied"`) || strings.Contains(out.String(), `"denied": null`) {
		t.Errorf("denied debe ser array: %s", out.String())
	}
}

func TestAutoStatusSinConfigurar(t *testing.T) {
	autoTestHome(t, "work")
	var out, errb bytes.Buffer
	// Sin `auto init` la política no resuelve: exit 1, pero el JSON sigue siendo
	// válido y trae el porqué.
	if code := dispatchAuto([]string{"status", "--json"}, &out, &errb); code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	var got autoStatusJSON
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("salida no es JSON: %s", out.String())
	}
	if got.Error == "" || got.Enabled {
		t.Errorf("got = %+v, want error y enabled=false", got)
	}
}

func TestAutoStatusTexto(t *testing.T) {
	autoTestHome(t, "work")
	var out, errb bytes.Buffer
	if code := dispatchAuto([]string{"init"}, &out, &errb); code != 0 {
		t.Fatal(errb.String())
	}
	out.Reset()
	if code := dispatchAuto([]string{"status"}, &out, &errb); code != 0 {
		t.Fatalf("status exit = %d, stderr=%s", code, errb.String())
	}
	for _, want := range []string{"política", "primario", "sensores", "work"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("falta %q en la salida:\n%s", want, out.String())
		}
	}
}

// --- _limit-hook ---

func TestLimitHookStdinBasuraSale0(t *testing.T) {
	home := autoTestHome(t)
	for _, in := range []string{"", "   ", "no soy json", "[1,2,3]", `{"hook_event_name":"Stop"}`} {
		var out, errb bytes.Buffer
		if code := limitHook(strings.NewReader(in), nil, &out, &errb); code != 0 {
			t.Fatalf("stdin %q -> exit %d, want 0", in, code)
		}
		if out.Len() != 0 {
			t.Errorf("el hook no debe escribir en stdout: %q", out.String())
		}
	}
	sent, err := core.ReadSentinels(home, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(sent) != 0 {
		t.Fatalf("basura no debe dejar sentinels: %+v", sent)
	}
}

func TestLimitHookRateLimitDejaSentinel(t *testing.T) {
	home := autoTestHome(t)
	t.Setenv("CCP_PROFILE", "work")
	session := "abababab-abab-4bab-8bab-abababababab"

	cases := []struct {
		name    string
		payload string
		window  core.LimitWindow
	}{
		{
			// Forma estructurada: la que reconoce el parser compartido.
			name: "estructurado 429",
			payload: `{"hook_event_name":"StopFailure","session_id":"` + session + `","cwd":"/repo",
			  "error":{"type":"rate_limit","message":"You've hit your weekly limit"},"apiErrorStatus":429}`,
			window: core.WindowWeekly,
		},
		{
			// Forma pobre: el mensaje como cadena suelta en `error`.
			name:    "prosa suelta",
			payload: `{"hook_event_name":"StopFailure","session_id":"` + session + `","error":"You've hit your 5-hour limit · resets 3pm"}`,
			window:  core.WindowSession,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if err := core.ClearSentinels(home, session); err != nil {
				t.Fatal(err)
			}
			since := time.Now().Add(-time.Second)
			if code := limitHook(strings.NewReader(c.payload), nil, io.Discard, io.Discard); code != 0 {
				t.Fatalf("exit = %d", code)
			}
			sent, err := core.ReadSentinels(home, since)
			if err != nil {
				t.Fatal(err)
			}
			if len(sent) != 1 {
				t.Fatalf("sentinels = %d, want 1", len(sent))
			}
			s := sent[0]
			if s.Profile != "work" || s.Session != session {
				t.Errorf("sentinel = %+v", s)
			}
			if s.Event.Source != "hook" {
				t.Errorf("source = %q, want hook", s.Event.Source)
			}
			if s.Event.Window != c.window {
				t.Errorf("window = %q, want %q", s.Event.Window, c.window)
			}
		})
	}
}

// TestLimitHookSinSesionNoEscribe: un sentinel sin sesión es inconsumible por el
// supervisor, así que no se escribe (sería basura permanente).
func TestLimitHookSinSesionNoEscribe(t *testing.T) {
	home := autoTestHome(t)
	payload := `{"hook_event_name":"StopFailure","error":"You've hit your weekly limit"}`
	if code := limitHook(strings.NewReader(payload), nil, io.Discard, io.Discard); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	sent, _ := core.ReadSentinels(home, time.Time{})
	if len(sent) != 0 {
		t.Fatalf("no debería haber sentinel: %+v", sent)
	}
}

// TestLimitHookSesionDesdeTranscript: sin session_id, el uuid sale del nombre
// del transcript (CC nombra el jsonl con él).
func TestLimitHookSesionDesdeTranscript(t *testing.T) {
	home := autoTestHome(t)
	uuid := "cdcdcdcd-cdcd-4dcd-8dcd-cdcdcdcdcdcd"
	payload := `{"hook_event_name":"StopFailure","transcript_path":"/x/y/` + uuid + `.jsonl",
	  "error":"rate_limit"}`
	if code := limitHook(strings.NewReader(payload), nil, io.Discard, io.Discard); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	sent, _ := core.ReadSentinels(home, time.Time{})
	if len(sent) != 1 || sent[0].Session != uuid {
		t.Fatalf("sentinels = %+v, want sesión %s", sent, uuid)
	}
}

// TestLimitHookNoRotaPorOtrosErrores: `overloaded` no se arregla cambiando de
// cuenta; detectarlo como límite gastaría un hop y movería la conversación.
func TestLimitHookNoRotaPorOtrosErrores(t *testing.T) {
	home := autoTestHome(t)
	payload := `{"hook_event_name":"StopFailure","session_id":"s1","error":{"type":"overloaded","message":"Overloaded, try again"}}`
	if code := limitHook(strings.NewReader(payload), nil, io.Discard, io.Discard); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	sent, _ := core.ReadSentinels(home, time.Time{})
	if len(sent) != 0 {
		t.Fatalf("overloaded no es rate limit: %+v", sent)
	}
}

// TestLimitHookNoRotaPor429Incidental: los tres dígitos «429» dentro de un
// request-id, de un contador o de un número de línea NO son un status HTTP. Un
// falso positivo aquí escribe un sentinel que el supervisor trata como la señal
// más fiable que existe y mueve la conversación del usuario a otra cuenta.
func TestLimitHookNoRotaPor429Incidental(t *testing.T) {
	home := autoTestHome(t)
	payloads := []string{
		// request_id con «429» pegado a otros alfanuméricos, dentro de un objeto:
		// el volcado crudo del objeto lo colaba como si fuera prosa del error.
		`{"hook_event_name":"StopFailure","session_id":"s1","error":{"type":"overloaded","message":"Overloaded","request_id":"req_011CT4291xYz"}}`,
		// Número de línea en un mensaje de compilación.
		`{"hook_event_name":"StopFailure","session_id":"s2","error":"Command failed: exit 1 (línea 4291 de foo.ts)"}`,
	}
	for _, p := range payloads {
		if code := limitHook(strings.NewReader(p), nil, io.Discard, io.Discard); code != 0 {
			t.Fatalf("exit = %d", code)
		}
	}
	sent, _ := core.ReadSentinels(home, time.Time{})
	if len(sent) != 0 {
		t.Fatalf("un 429 incidental no es rate limit: %+v", sent)
	}

	// El 429 de verdad (status HTTP suelto en la prosa, la forma más pobre que
	// entrega CC) sigue detectándose: el arreglo no puede dejar sordo al hook.
	real := `{"hook_event_name":"StopFailure","session_id":"s3","error":"API Error: 429 too many requests"}`
	if code := limitHook(strings.NewReader(real), nil, io.Discard, io.Discard); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	sent, _ = core.ReadSentinels(home, time.Time{})
	if len(sent) != 1 || sent[0].Session != "s3" {
		t.Fatalf("el 429 real debía dejar sentinel: %+v", sent)
	}
}

// Sin bloque auto_handoff, la salida legible no puede afirmar que el bloque
// existe pero está apagado: manda al usuario a buscar en ccp.yaml un
// `enabled: false` que no está, en vez de a `ccp auto init` — y encima
// contradice al error que el mismo comando escribe en stderr.
func TestAutoStatusDistingueAusenteDeDeshabilitado(t *testing.T) {
	home := autoTestHome(t, "work")

	var out, errb bytes.Buffer
	if code := autoStatus(nil, &out, &errb); code != 1 {
		t.Fatalf("exit = %d, quería 1 (sin política no hay gate que pasar)", code)
	}
	if strings.Contains(out.String(), "enabled: false") {
		t.Fatalf("el bloque no existe; no se puede decir que esté deshabilitado:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "ccp auto init") {
		t.Fatalf("la salida debe nombrar la acción que desbloquea:\n%s", out.String())
	}

	// Con el bloque presente y apagado sí es el mensaje de siempre.
	cfg, err := core.Load(home)
	if err != nil {
		t.Fatal(err)
	}
	cfg.AutoHandoff = &core.AutoHandoff{Enabled: false}
	if err := core.Save(home, cfg); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	errb.Reset()
	if code := autoStatus(nil, &out, &errb); code != 1 {
		t.Fatalf("exit = %d, quería 1", code)
	}
	if !strings.Contains(out.String(), "enabled: false") {
		t.Fatalf("con el bloque presente y apagado falta el diagnóstico:\n%s", out.String())
	}
}

// --- _statusline ---

const statusLineStdin = `{"session_id":"s1","rate_limits":{"five_hour":{"used_percentage":87.6},"seven_day":{"used_percentage":10}}}`

func TestStatusLineSinEnvueltoImprimeLineaPropia(t *testing.T) {
	home := autoTestHome(t)
	t.Setenv("CCP_PROFILE", "work")
	var out, errb bytes.Buffer
	if code := runStatusLine(strings.NewReader(statusLineStdin), nil, &out, &errb); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if got := strings.TrimSpace(out.String()); got != "work · 88%" {
		t.Errorf("línea = %q, want %q", got, "work · 88%")
	}
	rl, _, ok := core.ReadRateLimits(home, "work")
	if !ok {
		t.Fatal("no se persistió la muestra")
	}
	if rl.FiveHour.UsedPercentage != 87.6 {
		t.Errorf("muestra = %+v", rl)
	}
}

func TestStatusLineSinDatosImprimeSoloElPerfil(t *testing.T) {
	autoTestHome(t)
	t.Setenv("CCP_PROFILE", "work")
	var out, errb bytes.Buffer
	if code := runStatusLine(strings.NewReader("no soy json"), nil, &out, &errb); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if got := strings.TrimSpace(out.String()); got != "work" {
		t.Errorf("línea = %q, want %q", got, "work")
	}
}

// TestStatusLineEnvuelveComandoDelUsuario comprueba las dos mitades del
// contrato: el envuelto recibe el stdin INTACTO (ya lo habíamos consumido) y su
// stdout se reenvía tal cual, sin adornos nuestros.
func TestStatusLineEnvuelveComandoDelUsuario(t *testing.T) {
	home := autoTestHome(t)
	t.Setenv("CCP_PROFILE", "work")
	dir := t.TempDir()
	script := filepath.Join(dir, "bar.sh")
	// Lee stdin, lo devuelve entre marcas: así el test ve exactamente qué le
	// llegó al hijo.
	if err := os.WriteFile(script, []byte("#!/bin/sh\nprintf 'BAR['\ncat\nprintf ']'\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	var out, errb bytes.Buffer
	code := runStatusLine(strings.NewReader(statusLineStdin), []string{"--", script}, &out, &errb)
	if code != 0 {
		t.Fatalf("exit = %d, stderr=%s", code, errb.String())
	}
	want := "BAR[" + statusLineStdin + "]"
	if out.String() != want {
		t.Fatalf("stdout = %q, want %q", out.String(), want)
	}
	// Y aun envolviendo, la muestra se persistió (es el punto del sensor).
	if _, _, ok := core.ReadRateLimits(home, "work"); !ok {
		t.Error("envolver no debe impedir el muestreo")
	}
}

// TestStatusLineEnvueltoRotoSigueSaliendo0: si el comando del usuario no existe
// o revienta, la barra se queda vacía pero CC no ve un fallo.
func TestStatusLineEnvueltoRotoSigueSaliendo0(t *testing.T) {
	autoTestHome(t)
	var out, errb bytes.Buffer
	if code := runStatusLine(strings.NewReader(statusLineStdin),
		[]string{"--", "/no/existe/este/binario"}, &out, &errb); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
}

// --- ccp auto test ---

func TestAutoTestRutaCompleta(t *testing.T) {
	home := autoTestHome(t, "work")
	var out, errb bytes.Buffer
	if code := dispatchAuto([]string{"init"}, &out, &errb); code != 0 {
		t.Fatal(errb.String())
	}
	out.Reset()
	if code := dispatchAuto([]string{"install", "work"}, &out, &errb); code != 0 {
		t.Fatal(errb.String())
	}
	out.Reset()
	errb.Reset()
	if code := dispatchAuto([]string{"test", "--profile", "work"}, &out, &errb); code != 0 {
		t.Fatalf("auto test exit = %d\nstdout=%s\nstderr=%s", code, out.String(), errb.String())
	}
	if !strings.Contains(out.String(), "OK") {
		t.Errorf("salida sin veredicto:\n%s", out.String())
	}
	// Y no deja rastro: un sentinel sintético olvidado dispararía un hop real.
	sent, _ := core.ReadSentinels(home, time.Time{})
	if len(sent) != 0 {
		t.Fatalf("auto test dejó sentinels: %+v", sent)
	}
}

func TestAutoDispatchDesconocido(t *testing.T) {
	autoTestHome(t)
	var out, errb bytes.Buffer
	if code := dispatchAuto([]string{"fantasma"}, &out, &errb); code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if code := dispatchAuto([]string{}, &out, &errb); code != 0 {
		t.Fatalf("sin subcomando debe imprimir ayuda y salir 0, exit = %d", code)
	}
}
