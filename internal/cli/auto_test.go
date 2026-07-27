package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

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

// La barra propia enseña LAS DOS ventanas, etiquetadas. Enseñar solo el máximo
// —lo que hacía antes— daba un número sin unidad: un `31%` que tanto podía ser
// «te quedan horas» (5h) como «te quedan días» (7d), y que cambiaba de ventana
// sin avisar en cuanto la otra la adelantaba. Las etiquetas son las mismas que
// ya usa `ccp auto status`, que hablaba de 5h/7d desde el principio.
func TestStatusLineSinEnvueltoImprimeLineaPropia(t *testing.T) {
	home := autoTestHome(t)
	t.Setenv("CCP_PROFILE", "work")
	t.Setenv("COLUMNS", "100")
	var out, errb bytes.Buffer
	if code := runStatusLine(strings.NewReader(statusLineStdin), nil, &out, &errb); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	const want = "work  5h ▏█████████░▏ 88%  7d ▏█░░░░░░░░░▏ 10%"
	if got := strings.TrimSpace(out.String()); got != want {
		t.Errorf("línea = %q, want %q", got, want)
	}
	rl, _, ok := core.ReadRateLimits(home, "work")
	if !ok {
		t.Fatal("no se persistió la muestra")
	}
	if rl.FiveHour.UsedPercentage != 87.6 {
		t.Errorf("muestra = %+v", rl)
	}
}

// Una ventana sin dato no se inventa: se omite. El bug conocido de CC 2.1.220
// (five_hour a 0 con seven_day poblado) es justo este caso, y pintar «5h 0%»
// diría lo contrario de lo que sabemos — que de esa ventana no sabemos nada.
func TestStatusLineConUnaSolaVentanaOmiteLaOtra(t *testing.T) {
	autoTestHome(t)
	t.Setenv("CCP_PROFILE", "work")
	t.Setenv("COLUMNS", "100")
	casos := []struct {
		nombre string
		stdin  string
		quiere string
	}{
		{
			"solo 7d",
			`{"session_id":"s1","rate_limits":{"seven_day":{"used_percentage":31}}}`,
			"work  7d ▏███░░░░░░░▏ 31%",
		},
		{
			"solo 5h",
			`{"session_id":"s1","rate_limits":{"five_hour":{"used_percentage":14}}}`,
			"work  5h ▏█░░░░░░░░░▏ 14%",
		},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			var out, errb bytes.Buffer
			if code := runStatusLine(strings.NewReader(c.stdin), nil, &out, &errb); code != 0 {
				t.Fatalf("exit = %d", code)
			}
			if got := strings.TrimSpace(out.String()); got != c.quiere {
				t.Errorf("línea = %q, want %q", got, c.quiere)
			}
		})
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

// TestStatusLineDegradaPorAncho pinea los TRES niveles.
//
// Degradar y no truncar es la decisión: una línea cortada a mitad de medidor no
// dice menos, dice algo falso (un medidor sin su delimitador derecho se lee como
// más vacío de lo que está). El ancho sale de COLUMNS y nunca de la tty — dentro
// de CC el stdout es un pipe, así que no hay tty a la que preguntar.
//
// Los cortes de aquí no son redondos (46/45, 34/33) A PROPÓSITO: son el ancho que
// MIDE cada nivel con este perfil y esta muestra. El nivel ya no se elige contra
// umbrales fijos de columnas —eso desbordaba en cuanto el nombre del perfil
// pasaba de cuatro letras— sino montando la línea y midiéndola.
func TestStatusLineDegradaPorAncho(t *testing.T) {
	autoTestHome(t)
	t.Setenv("CCP_PROFILE", "work")
	const (
		completo = "work  5h ▏█████████░▏ 88%  7d ▏█░░░░░░░░░▏ 10%" // 46 runas
		medio    = "work · 5h ▏████▏88% · 7d ▏█░░░▏10%"             // 34 runas
		compacto = "work · 5h 88% · 7d 10%"                         // 22 runas
	)
	casos := []struct {
		nombre  string
		columns string
		quiere  string
	}{
		// A 10 celdas el 88% redondea a 9 llenas y el 10% a 1.
		{"completo", "100", completo},
		{"completo justo", "46", completo},
		// A 4 celdas el 88% redondea a 4 (3.5 hacia arriba) y el 10% a 0.4, que
		// el suelo de una celda sube a 1: la resolución baja con el ancho, pero
		// «hay consumo» no desaparece. El número exacto sigue ahí al lado.
		{"medio", "45", medio},
		{"medio justo", "34", medio},
		{"compacto", "33", compacto},
		// Ni el compacto cabe: se entrega igual. Recortar empezaría por el
		// nombre del perfil, que es el dato que la barra existe para dar.
		{"mas estrecho que el compacto", "10", compacto},
		// COLUMNS ausente o basura cae al default de 80 ⇒ aquí cabe el completo.
		// Es deliberado: quedarse sin barra por no saber el ancho sería peor.
		{"sin COLUMNS", "", completo},
		{"COLUMNS basura", "ancho", completo},
		{"COLUMNS cero", "0", completo},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			t.Setenv("COLUMNS", c.columns)
			var out, errb bytes.Buffer
			if code := runStatusLine(strings.NewReader(statusLineStdin), nil, &out, &errb); code != 0 {
				t.Fatalf("exit = %d", code)
			}
			if got := strings.TrimSpace(out.String()); got != c.quiere {
				t.Errorf("línea = %q, want %q", got, c.quiere)
			}
		})
	}
}

// TestStatusLineDegradaPorNombreDePerfil es la mitad del ancho que se había
// olvidado: el nombre del perfil lo elige el usuario y entra en la línea igual
// que los medidores.
//
// Con umbrales fijos de columnas, COLUMNS=80 (que es lo que hay en la ruta real,
// porque COLUMNS no se exporta) daba SIEMPRE nivel completo, y un perfil de 40
// caracteres producía una línea de 82 columnas que la terminal partía. Ahora el
// nombre largo degrada la barra igual que lo haría una terminal estrecha, y por
// eso los escalones medio y compacto son alcanzables sin tocar COLUMNS.
func TestStatusLineDegradaPorNombreDePerfil(t *testing.T) {
	autoTestHome(t)
	// 40 caracteres: con medidor completo la línea mediría 82 > 80.
	const largo = "perfil-larguisimo-de-produccion-en-emco1"
	t.Setenv("CCP_PROFILE", largo)
	t.Setenv("COLUMNS", "") // default 80, el caso real

	var out, errb bytes.Buffer
	if code := runStatusLine(strings.NewReader(statusLineStdin), nil, &out, &errb); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	want := largo + " · 5h ▏████▏88% · 7d ▏█░░░▏10%"
	if got := strings.TrimSpace(out.String()); got != want {
		t.Errorf("línea = %q, want %q", got, want)
	}
}

// TestStatusBarCabeEnElAncho es la invariante, no un caso concreto: si el nivel
// compacto cabe en el presupuesto, la línea entregada NO puede pasarse de él.
//
// Es la regresión del bug que tenía la escalera anterior: elegía nivel por
// umbrales fijos y los tres niveles desbordaban su propio umbral en cuanto el
// perfil o la cuenta atrás crecían. Un test de cadena exacta no lo habría pillado
// —los que había fijaban perfil "work" y sin resets_at, justo el caso que no
// desborda—, así que lo que se comprueba es la propiedad, sobre una matriz que
// incluye lo que el usuario controla: nombre de perfil y presencia de reloj.
func TestStatusBarCabeEnElAncho(t *testing.T) {
	autoTestHome(t)
	now := time.Date(2026, 7, 27, 10, 0, 0, 0, time.UTC)
	muestras := map[string]core.RateLimits{
		"sin reloj": {
			FiveHour: core.Windowed{UsedPercentage: 88},
			SevenDay: core.Windowed{UsedPercentage: 10},
		},
		"con reloj": {
			FiveHour: core.Windowed{UsedPercentage: 88, ResetsAt: now.Add(2*time.Hour + 13*time.Minute)},
			SevenDay: core.Windowed{UsedPercentage: 10, ResetsAt: now.Add(50 * time.Hour)},
		},
		"solo 7d": {
			SevenDay: core.Windowed{UsedPercentage: 10, ResetsAt: now.Add(50 * time.Hour)},
		},
	}
	perfiles := []string{"a", "work", "trabajo-emco-produccion", strings.Repeat("x", 60)}
	for _, cols := range []int{20, 30, 40, 50, 60, 80, 120} {
		for nombreMuestra, rl := range muestras {
			for _, perfil := range perfiles {
				línea := statusBarRender(perfil, rl, now, cols, false)
				ancho := utf8.RuneCountInString(línea)
				mínimo := utf8.RuneCountInString(statusBarRender(perfil, rl, now, 0, false))
				// Si ni el compacto cabía, entregar el compacto es lo correcto:
				// la invariante solo aplica cuando hay algo que quepa.
				if mínimo > cols {
					continue
				}
				if ancho > cols {
					t.Errorf("perfil=%q muestra=%q COLUMNS=%d: ancho=%d se pasa (%q)",
						perfil, nombreMuestra, cols, ancho, línea)
				}
			}
		}
	}
}

// TestStatusLineSiempreSaleCero es el contrato duro del sensor: pase lo que pase
// con el stdin, exit 0. No es celo defensivo abstracto — el productor es Claude
// Code, el consumidor es la UI de Claude Code, y un exit distinto de 0 deja al
// usuario sin barra y con un error que se repite en cada refresco.
func TestStatusLineSiempreSaleCero(t *testing.T) {
	autoTestHome(t)
	t.Setenv("CCP_PROFILE", "work")
	casos := []struct {
		nombre string
		stdin  string
	}{
		{"vacío", ""},
		{"json truncado", `{"rate_limits":{"five_hour":{"used_percentage":`},
		{"binario", "\x00\x01\x02\xff\xfe sin sentido \x00"},
		{"porcentajes imposibles", `{"rate_limits":{"five_hour":{"used_percentage":-40},"seven_day":{"used_percentage":9e99}}}`},
		{"resets_at ilegible", `{"rate_limits":{"five_hour":{"used_percentage":50,"resets_at":"mañana"}}}`},
		// El tope de lectura (4 MiB) corta a mitad de JSON: el parseo falla y la
		// barra se queda en el perfil, pero el comando sale igual.
		{"muy grande", `{"rate_limits":` + strings.Repeat("x", 5<<20)},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			var out, errb bytes.Buffer
			if code := runStatusLine(strings.NewReader(c.stdin), nil, &out, &errb); code != 0 {
				t.Fatalf("exit = %d, want 0", code)
			}
			// Y sale UNA línea: la barra de CC es de una sola línea, y un salto
			// de más la partiría.
			if n := strings.Count(out.String(), "\n"); n != 1 {
				t.Errorf("saltos de línea = %d, want 1 (salida %q)", n, out.String())
			}
			// Y ESA línea es corta. «Salir 0» no basta: con el porcentaje sin
			// acotar, el caso de los porcentajes imposibles salía 0 y con una
			// sola línea... de 300 caracteres. El daño era el mismo.
			if n := utf8.RuneCountInString(strings.TrimSpace(out.String())); n > 200 {
				t.Errorf("ancho = %d runas, una barra de estado no puede medir eso (salida %q)", n, out.String())
			}
		})
	}
}

// TestStatusLineOmiteResetVencido: un resets_at en el pasado no produce cuenta
// atrás.
//
// Es la misma filosofía que Windowed.HasData. Hay un bug conocido de CC en el que
// una ventana llega vacía o con el reset caducado; pintar "·0m" ahí afirmaría «ya
// reabrió», cuando lo único que sabemos es que no sabemos. Y una ventana con dato
// pero SIN resets_at sale con su porcentaje y sin reloj: el consumo lo medimos,
// el momento de reapertura no.
func TestStatusLineOmiteResetVencido(t *testing.T) {
	autoTestHome(t)
	t.Setenv("CCP_PROFILE", "work")
	t.Setenv("COLUMNS", "100")

	now := time.Now()
	// El medio minuto de colchón evita que los microsegundos que tarda
	// runStatusLine en llamar a time.Now() bajen el resultado a "2h13m".
	futuro := now.Add(2*time.Hour + 14*time.Minute + 30*time.Second).UTC().Format(time.RFC3339)
	pasado := now.Add(-time.Hour).UTC().Format(time.RFC3339)
	stdin := fmt.Sprintf(
		`{"rate_limits":{"five_hour":{"used_percentage":2,"resets_at":%q},"seven_day":{"used_percentage":59,"resets_at":%q}}}`,
		futuro, pasado)

	var out, errb bytes.Buffer
	if code := runStatusLine(strings.NewReader(stdin), nil, &out, &errb); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	// 5h lleva reloj (futuro), 7d no (vencido). El medidor del 2% enseña UNA
	// celda: 0.2 de 10 redondea a 0, pero el suelo de RenderGauge la sube a 1
	// porque «no has gastado nada» y «has empezado a gastar» tienen que
	// distinguirse de un vistazo — que es para lo que existe el medidor. La
	// proporción exacta la sigue dando el número de al lado.
	const want = "work  5h ▏█░░░░░░░░░▏ 2% ·2h14m  7d ▏██████░░░░▏ 59%"
	if got := strings.TrimSpace(out.String()); got != want {
		t.Errorf("línea = %q, want %q", got, want)
	}
}

// TestStatusLineRespetaNoColor: con NO_COLOR la barra no lleva ni un ESC, y el
// medidor sigue diciendo lo mismo.
//
// Esa segunda mitad es la que justifica haber elegido medidor de bloques y no un
// punto de color: la información va en la forma, el color solo la subraya.
func TestStatusLineRespetaNoColor(t *testing.T) {
	autoTestHome(t) // ya fija NO_COLOR=1
	t.Setenv("CCP_PROFILE", "work")
	t.Setenv("COLUMNS", "100")

	// 95% ⇒ severidad crítica: el caso que más ganas tendría de teñirse.
	stdin := `{"rate_limits":{"five_hour":{"used_percentage":95},"seven_day":{"used_percentage":30}}}`
	var out, errb bytes.Buffer
	if code := runStatusLine(strings.NewReader(stdin), nil, &out, &errb); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	const want = "work  5h ▏██████████▏ 95%  7d ▏███░░░░░░░▏ 30%"
	if got := strings.TrimSpace(out.String()); got != want {
		t.Errorf("línea = %q, want %q", got, want)
	}
}

// TestStatusLineTineAunqueStdoutSeaPipe es la regresión de un semáforo que
// existía y no se veía nunca.
//
// El gate general del paquete (useColor) exige que el destino sea un dispositivo
// de caracteres. Pero Claude Code invoca el statusLine capturando su stdout por
// un PIPE —así es como recoge la línea para pintarla—, así que useColor decía que
// no en el 100 % de las ejecuciones reales: verde, ámbar y rojo eran código
// muerto en producción. Quien renderiza aquí no es la terminal, es CC.
//
// Por eso el assert se hace a través de runStatusLine sobre un bytes.Buffer (que
// no es *os.File, o sea el peor caso para useColor) y NO llamando a severityTint
// a mano: un test que llame al tinte directo pasa en verde con el bug puesto.
func TestStatusLineTineAunqueStdoutSeaPipe(t *testing.T) {
	autoTestHome(t)
	t.Setenv("CCP_PROFILE", "work")
	t.Setenv("COLUMNS", "100")
	t.Setenv("NO_COLOR", "") // el usuario NO lo prohibió

	stdin := `{"rate_limits":{"five_hour":{"used_percentage":95},"seven_day":{"used_percentage":30}}}`
	var out, errb bytes.Buffer
	if code := runStatusLine(strings.NewReader(stdin), nil, &out, &errb); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	// Medidor y porcentaje van del MISMO color: son el mismo dato dicho dos
	// veces, y teñir solo uno invitaría a pensar que miden cosas distintas.
	want := "work  5h " +
		ansiRed + "▏██████████▏" + ansiReset + " " + ansiRed + "95%" + ansiReset +
		"  7d " +
		ansiGreen + "▏███░░░░░░░▏" + ansiReset + " " + ansiGreen + "30%" + ansiReset
	if got := strings.TrimSpace(out.String()); got != want {
		t.Errorf("línea = %q, want %q", got, want)
	}
}

// TestSeverityTintPorNivel fija los tres tintes por nombre.
func TestSeverityTintPorNivel(t *testing.T) {
	for _, c := range []struct {
		sev    severity
		quiere string
	}{
		{sevOK, ansiGreen},
		{sevWarn, ansiAmber},
		{sevCrit, ansiRed},
	} {
		if got := severityTint(true, "88%", c.sev); got != c.quiere+"88%"+ansiReset {
			t.Errorf("severityTint(true, %v) = %q", c.sev, got)
		}
		if got := severityTint(false, "88%", c.sev); got != "88%" {
			t.Errorf("severityTint(false, %v) = %q, quiere el fragmento tal cual", c.sev, got)
		}
	}
}

// TestStatusLineAcotaPorcentajesImposibles: el NÚMERO se acota igual que el
// medidor.
//
// El sensor no puede fallar, pero «no fallar» no es solo salir 0. Con el
// porcentaje crudo, un `used_percentage: 9e99` de una muestra corrupta pintaba el
// medidor lleno (correcto) y a su lado 300 dígitos: una barra de estado de
// cientos de columnas, que es exactamente el daño que el contrato de salir 0
// existe para evitar. Y un -40 pintaba el medidor vacío junto a un "-40%" en
// verde, o sea el medidor y el número contradiciéndose.
//
// El test mira el CONTENIDO a propósito: el de «siempre sale cero» ya cubría este
// stdin y pasaba en verde porque solo aseveraba el código de salida.
func TestStatusLineAcotaPorcentajesImposibles(t *testing.T) {
	autoTestHome(t)
	t.Setenv("CCP_PROFILE", "work")
	t.Setenv("COLUMNS", "100")

	stdin := `{"rate_limits":{"five_hour":{"used_percentage":-40},"seven_day":{"used_percentage":9e99}}}`
	var out, errb bytes.Buffer
	if code := runStatusLine(strings.NewReader(stdin), nil, &out, &errb); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	const want = "work  5h ▏░░░░░░░░░░▏ 0%  7d ▏██████████▏ 100%"
	if got := strings.TrimSpace(out.String()); got != want {
		t.Errorf("línea = %q, want %q", got, want)
	}
}

// TestStatusBarSeverityUmbrales fija los cortes del semáforo por nombre.
//
// El crítico se compara contra core.DefaultAutoThreshold, no contra un 90
// literal: si alguien mueve el umbral del motor, este test tiene que caer para
// que la barra se mueva con él. Que la barra diga «verde» mientras `ccp session`
// rota de perfil sería la peor de las incoherencias posibles.
func TestStatusBarSeverityUmbrales(t *testing.T) {
	casos := []struct {
		pct  float64
		want severity
	}{
		{0, sevOK},
		{69.9, sevOK},
		{70, sevWarn},
		{89.9, sevWarn},
		{float64(core.DefaultAutoThreshold), sevCrit},
		{100, sevCrit},
	}
	for _, c := range casos {
		if got := statusBarSeverity(c.pct); got != c.want {
			t.Errorf("statusBarSeverity(%v) = %v, want %v", c.pct, got, c.want)
		}
	}
	if statusBarCritPct != core.DefaultAutoThreshold {
		t.Errorf("el rojo de la barra (%d) debe seguir al umbral del motor (%d)",
			statusBarCritPct, core.DefaultAutoThreshold)
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
