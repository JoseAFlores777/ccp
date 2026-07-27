package cli

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JoseAFlores777/ccp/internal/core"
	"github.com/JoseAFlores777/ccp/internal/core/i18n"
	"github.com/JoseAFlores777/ccp/internal/supervisor"
)

// session_test.go — parseo, traducción de exit codes y una corrida completa de
// `ccp session` contra un claude falso.
//
// Todo corre sobre t.TempDir() con CCP_HOME apuntando ahí: ni un solo test toca
// ~/.config/ccp.

// --- parseo -----------------------------------------------------------------

func TestParseSessionFlags(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want sessionFlags
	}{
		{
			name: "sin argumentos",
			args: nil,
			want: sessionFlags{},
		},
		{
			name: "forma larga completa",
			args: []string{"--headless", "--policy", "noche", "--max-hops", "3",
				"--yolo", "--session", "uuid-1", "--dry-run", "--no-return",
				"--claude-bin", "/bin/fake"},
			want: sessionFlags{
				headless: true, policy: "noche", maxHops: 3, yolo: true,
				session: "uuid-1", dryRun: true, noReturn: true, claudeBin: "/bin/fake",
			},
		},
		{
			name: "forma pegada con =",
			args: []string{"--policy=noche", "--max-hops=0", "--session=uuid-2", "--claude-bin=/bin/fake"},
			want: sessionFlags{policy: "noche", maxHops: 0, session: "uuid-2", claudeBin: "/bin/fake"},
		},
		{
			name: "-p corto y alias largo de yolo",
			args: []string{"-p", "--dangerously-skip-permissions"},
			want: sessionFlags{headless: true, yolo: true},
		},
		{
			// El orden no significa nada: los flags son un conjunto, no una
			// secuencia. Si esto se rompiera, `--policy` tras `--dry-run` dejaría
			// de aplicarse y el dry-run mentiría sobre la cadena.
			name: "orden mezclado",
			args: []string{"--dry-run", "--session", "uuid-3", "-p", "--max-hops", "9", "--policy", "dia"},
			want: sessionFlags{dryRun: true, session: "uuid-3", headless: true, maxHops: 9, policy: "dia"},
		},
		{
			name: "-- pasa args tal cual",
			args: []string{"-p", "--", "-p", "resume esto"},
			want: sessionFlags{headless: true, args: []string{"-p", "resume esto"}},
		},
		{
			// La frontera es absoluta: después de `--` nada se interpreta,
			// aunque sean flags que ccp conoce de sobra.
			name: "-- con flags que parecen de ccp",
			args: []string{"--policy", "dia", "--", "--policy", "otra", "--dry-run", "--claude-bin"},
			want: sessionFlags{policy: "dia", args: []string{"--policy", "otra", "--dry-run", "--claude-bin"}},
		},
		{
			name: "-- sin nada detrás",
			args: []string{"--yolo", "--"},
			want: sessionFlags{yolo: true, args: []string{}},
		},
		{
			name: "-- como primer argumento",
			args: []string{"--", "hola mundo"},
			want: sessionFlags{args: []string{"hola mundo"}},
		},
		{
			name: "ayuda",
			args: []string{"--help"},
			want: sessionFlags{help: true},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseSessionFlags(tc.args, i18n.En)
			if err != nil {
				t.Fatalf("parseSessionFlags(%q): %v", tc.args, err)
			}
			if !sameSessionFlags(got, tc.want) {
				t.Fatalf("parseSessionFlags(%q)\n got %+v\nwant %+v", tc.args, got, tc.want)
			}
		})
	}
}

func TestParseSessionFlagsErrores(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string // fragmento esperado del mensaje
	}{
		{"flag desconocido", []string{"--policia", "x"}, "--policia"},
		{"flag corto desconocido", []string{"-x"}, "-x"},
		// Sin este error, `ccp session "arregla el bug"` arrancaría una sesión
		// interactiva vacía y el prompt se perdería sin rastro.
		{"posicional suelto", []string{"arregla el bug"}, "arregla el bug"},
		{"posicional tras flags", []string{"-p", "prompt"}, "prompt"},
		{"policy sin valor", []string{"--policy"}, "--policy"},
		{"policy vacía", []string{"--policy="}, "--policy"},
		{"session sin valor", []string{"-p", "--session"}, "--session"},
		{"claude-bin sin valor", []string{"--claude-bin"}, "--claude-bin"},
		{"max-hops sin valor", []string{"--max-hops"}, "--max-hops"},
		{"max-hops no numérico", []string{"--max-hops", "muchos"}, "muchos"},
		{"max-hops negativo", []string{"--max-hops", "-2"}, "-2"},
		{"booleano con valor", []string{"--dry-run=false"}, "--dry-run"},
		{"headless con valor", []string{"-p=1"}, "-p"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseSessionFlags(tc.args, i18n.En)
			if err == nil {
				t.Fatalf("parseSessionFlags(%q) no falló", tc.args)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("mensaje %q no menciona %q", err.Error(), tc.want)
			}
		})
	}
}

// sameSessionFlags compara dos sessionFlags incluyendo la cola de args (nil y
// slice vacío se consideran distintos a propósito solo en longitud, no en tipo).
func sameSessionFlags(a, b sessionFlags) bool {
	if a.headless != b.headless || a.policy != b.policy || a.maxHops != b.maxHops ||
		a.yolo != b.yolo || a.session != b.session || a.dryRun != b.dryRun ||
		a.noReturn != b.noReturn || a.claudeBin != b.claudeBin || a.help != b.help {
		return false
	}
	if len(a.args) != len(b.args) {
		return false
	}
	for i := range a.args {
		if a.args[i] != b.args[i] {
			return false
		}
	}
	return true
}

// --- exit codes -------------------------------------------------------------

func TestSessionExitCode(t *testing.T) {
	cases := []struct {
		name string
		res  supervisor.Result
		err  error
		want int
	}{
		{"ok", supervisor.Result{ExitCode: 0}, nil, 0},
		{"código del hijo", supervisor.Result{ExitCode: 42}, nil, 42},
		{"ctrl-c del hijo", supervisor.Result{ExitCode: 130}, nil, 130},
		{"parked", supervisor.Result{ExitCode: 75, Parked: true}, nil, supervisor.ParkedExitCode},
		// Parked manda sobre el ExitCode del Result: si el supervisor aparcara
		// sin haber puesto el 75, el contrato con el cron seguiría en pie.
		{"parked sin código", supervisor.Result{Parked: true}, nil, 75},
		{"error de uso", supervisor.Result{ExitCode: 1}, errors.New("boom"), 1},
		// El error manda aunque el hijo hubiera salido con 0: el 2 significa
		// «handoffs.yaml quedó desincronizado», y perderlo dejaría a un wrapper
		// creyendo que todo fue bien.
		{"error de I/O de handoffs", supervisor.Result{}, fmt.Errorf("cerrar: %w", core.ErrHandoffIO), 2},
		{"error de I/O tras exit 0", supervisor.Result{ExitCode: 0}, fmt.Errorf("x: %w", core.ErrHandoffIO), 2},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := sessionExitCode(tc.res, tc.err); got != tc.want {
				t.Fatalf("sessionExitCode = %d, quería %d", got, tc.want)
			}
		})
	}
}

// --- superficie del comando -------------------------------------------------

// seedSession deja un CCP_HOME con dos perfiles official, una regla que hace
// primario a p1 en el cwd y (si auto) el bloque auto_handoff sembrado a mano.
// Devuelve home y cwd, ambos existentes en disco.
func seedSession(t *testing.T, auto bool) (home, cwd string) {
	t.Helper()
	home = t.TempDir()
	cwd = filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := &core.Config{
		Version: core.SchemaVersion,
		Profiles: map[string]core.Profile{
			"p1": {Type: "official"},
			"p2": {Type: "official"},
		},
		Rules: []core.Rule{{Path: cwd, Profile: "p1"}},
	}
	if auto {
		cfg.AutoHandoff = &core.AutoHandoff{
			Enabled: true,
			Policies: map[string]core.AutoPolicy{
				"default": {Fallback: []string{"p2"}, MinDwell: "0s"},
			},
		}
	}
	if err := core.Save(home, cfg); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CCP_HOME", home)
	t.Setenv("PWD", cwd)
	t.Setenv("CCP_LANG", "en")
	return home, cwd
}

func runSession(t *testing.T, args ...string) (string, string, int) {
	t.Helper()
	var out, errb bytes.Buffer
	code := cmdSession(args, &out, &errb)
	return out.String(), errb.String(), code
}

func TestCmdSessionAyuda(t *testing.T) {
	seedSession(t, false)
	out, _, code := runSession(t, "--help")
	if code != 0 {
		t.Fatalf("code = %d, quería 0", code)
	}
	// La ayuda tiene que nombrar `--claude-bin` y decir que es interno: un flag
	// invisible en la ayuda es una trampa para quien lo encuentre en un script.
	if !strings.Contains(out, "--claude-bin") || !strings.Contains(out, "internal") {
		t.Fatalf("la ayuda no documenta --claude-bin como interno:\n%s", out)
	}
}

func TestCmdSessionFlagDesconocido(t *testing.T) {
	seedSession(t, true)
	_, errs, code := runSession(t, "--nope")
	if code != 1 {
		t.Fatalf("code = %d, quería 1", code)
	}
	if !strings.Contains(errs, "--nope") {
		t.Fatalf("stderr no menciona el flag: %q", errs)
	}
}

func TestCmdSessionSinBloqueAutoHandoff(t *testing.T) {
	seedSession(t, false)
	_, errs, code := runSession(t, "--dry-run")
	if code != 1 {
		t.Fatalf("code = %d, quería 1", code)
	}
	if !strings.Contains(errs, "ccp auto init") {
		t.Fatalf("stderr no apunta a `ccp auto init`: %q", errs)
	}
}

func TestCmdSessionAutoHandoffDeshabilitado(t *testing.T) {
	home, _ := seedSession(t, true)
	cfg, err := core.Load(home)
	if err != nil {
		t.Fatal(err)
	}
	cfg.AutoHandoff.Enabled = false
	if err := core.Save(home, cfg); err != nil {
		t.Fatal(err)
	}
	_, errs, code := runSession(t, "--dry-run")
	if code != 1 {
		t.Fatalf("code = %d, quería 1", code)
	}
	if !strings.Contains(errs, "auto_handoff") {
		t.Fatalf("stderr no explica que está deshabilitado: %q", errs)
	}
}

func TestCmdSessionPoliticaInexistente(t *testing.T) {
	seedSession(t, true)
	_, errs, code := runSession(t, "--dry-run", "--policy", "fantasma")
	if code != 1 {
		t.Fatalf("code = %d, quería 1", code)
	}
	if !strings.Contains(errs, "fantasma") {
		t.Fatalf("stderr no nombra la política: %q", errs)
	}
}

func TestCmdSessionDryRun(t *testing.T) {
	seedSession(t, true)
	out, _, code := runSession(t, "--dry-run")
	if code != 0 {
		t.Fatalf("code = %d, quería 0", code)
	}
	// El plan lo imprime el supervisor; aquí solo se comprueba que llegó al
	// stdout del comando y que no se lanzó nada.
	if !strings.Contains(out, "p1") || !strings.Contains(out, "p2") {
		t.Fatalf("el plan no menciona la cadena:\n%s", out)
	}
	if !strings.Contains(out, "--dry-run") {
		t.Fatalf("el plan no dice que no se lanza nada:\n%s", out)
	}
}

// fakeClaudeSession es un `claude` de mentira: apunta sus argumentos y el perfil
// con el que lo lanzaron, y sale con el código que le pidan. POSIX sh puro.
const fakeClaudeSession = `#!/bin/sh
printf '%s\n' "$CCP_PROFILE" >> "$FAKE_SESSION_LOG"
printf '%s\n' "$*" >> "$FAKE_SESSION_LOG"
exit ${FAKE_SESSION_EXIT:-0}
`

// TestCmdSessionE2E corre el comando entero contra el claude falso: sin límites
// detectados no hay rotación, así que el exit es el del hijo y handoffs.yaml no
// debe quedar con ningún marcador vivo.
func TestCmdSessionE2E(t *testing.T) {
	home, _ := seedSession(t, true)

	dir := t.TempDir()
	bin := filepath.Join(dir, "fake-claude")
	if err := os.WriteFile(bin, []byte(fakeClaudeSession), 0o755); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(dir, "log")
	t.Setenv("FAKE_SESSION_LOG", logPath)

	uuid := "aaaaaaaa-1111-4111-8111-aaaaaaaaaaaa"
	_, errs, code := runSession(t, "-p", "--claude-bin", bin, "--session", uuid, "--", "hola")
	if code != 0 {
		t.Fatalf("code = %d, quería 0 (stderr=%q)", code, errs)
	}

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("el claude falso no llegó a correr: %v", err)
	}
	got := string(data)
	// El perfil sale de la regla del cwd, no del entorno del test: es la prueba
	// de que EnvForChild aplicó el delta del primario.
	if !strings.HasPrefix(got, "p1\n") {
		t.Fatalf("el hijo no corrió con p1:\n%s", got)
	}
	// La cola de `--` llega intacta y el modo headless añadió lo suyo.
	for _, want := range []string{"--resume " + uuid, "-p", "--output-format stream-json", "hola"} {
		if !strings.Contains(got, want) {
			t.Fatalf("los args del hijo no contienen %q:\n%s", want, got)
		}
	}

	// Sin rotación no hay préstamo: ni un marcador vivo ni historial archivado.
	h, err := core.LoadHandoffs(home)
	if err != nil {
		t.Fatal(err)
	}
	if len(h.Active) != 0 || len(h.Archived) != 0 {
		t.Fatalf("quedaron marcadores: %+v", h)
	}
}

// TestCmdSessionPropagaExitCodeDelHijo fija la sustituibilidad: sin límite
// detectado, `ccp session -p …` devuelve el código de claude tal cual, que es lo
// que permite meterlo en un script donde antes iba `claude -p …`.
func TestCmdSessionPropagaExitCodeDelHijo(t *testing.T) {
	seedSession(t, true)

	dir := t.TempDir()
	bin := filepath.Join(dir, "fake-claude")
	if err := os.WriteFile(bin, []byte(fakeClaudeSession), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FAKE_SESSION_LOG", filepath.Join(dir, "log"))
	t.Setenv("FAKE_SESSION_EXIT", "7")

	_, _, code := runSession(t, "-p", "--claude-bin", bin)
	if code != 7 {
		t.Fatalf("code = %d, quería 7", code)
	}
}
