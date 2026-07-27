package supervisor

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/JoseAFlores777/ccp/internal/core"
)

// supervisor_test.go — el bucle completo contra un `claude` falso.
//
// La pieza que hace posible probar esto en milisegundos es el script `fakeClaude`:
// lee un PLAN (una línea por lanzamiento: código de salida, dónde emitir, y qué
// emitir) y se comporta como el claude real en lo único que le importa al
// supervisor — escribe su transcript en $CLAUDE_CONFIG_DIR, imprime NDJSON por
// stdout y muere con un código. Así se reproducen escenarios que en producción
// tardan horas (una ventana de 5h que se agota, un cooldown que vence) sin un
// solo sleep.
//
// Todos los tests usan Poll de milisegundos y homes de t.TempDir(): nunca tocan
// ~/.config/ccp.

// --- infraestructura --------------------------------------------------------

// safeBuf es un buffer con candado. Hace falta porque en headless el detector
// de stream-json copia la salida del hijo desde su propia goroutine, que puede
// seguir viva un instante después de que Run vuelva; leer el buffer sin candado
// sería una carrera detectable con -race.
type safeBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *safeBuf) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *safeBuf) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

// fakeScript es el `claude` de mentira. Deliberadamente POSIX sh (nada de
// bashismos) porque el supervisor lo lanza por PATH absoluto igual que al real.
//
// Contrato con el test, todo por entorno:
//
//	FAKE_PLAN    archivo con una línea por lanzamiento: <exit>|<dónde>|<json>|<modo>
//	FAKE_COUNT   archivo contador de lanzamientos (lo lleva el propio script)
//	FAKE_LOG     bitácora "<n> <perfil> <uuid>" por lanzamiento
//	FAKE_SLUG    slug del proyecto, para saber dónde escribir el transcript
//	FAKE_RELEASE archivo-testigo que suelta a un lanzamiento en modo "wait"
//
// El modo "wait" es lo que hace probable el temporizador de regreso: el hijo se
// queda VIVO (sin emitir nada) hasta que el test crea FAKE_RELEASE o hasta que
// el supervisor lo mata. Las esperas son rodajas de 50ms —no un `sleep 30`—
// para que el SIGTERM lo recoja al instante y no queden huérfanos; el tope de
// iteraciones es un seguro contra un test que se cuelgue.
//
// El uuid de sesión lo saca de sus PROPIOS argumentos (--session-id/--resume),
// que es justo lo que verifica que BuildArgs le pasó el selector correcto.
const fakeScript = `#!/bin/sh
sid=""
prev=""
for a in "$@"; do
  case "$prev" in
    --session-id|--resume) sid="$a" ;;
  esac
  prev="$a"
done

dir="$CLAUDE_CONFIG_DIR/projects/$FAKE_SLUG"
mkdir -p "$dir"
f="$dir/$sid.jsonl"
if [ ! -f "$f" ]; then
  printf '{"type":"user","sessionId":"%s","uuid":"m1","parentUuid":null}\n' "$sid" > "$f"
fi

n=0
if [ -f "$FAKE_COUNT" ]; then read -r n < "$FAKE_COUNT"; fi
n=$((n+1))
printf '%s\n' "$n" > "$FAKE_COUNT"
printf '%s %s %s\n' "$n" "$CCP_PROFILE" "$sid" >> "$FAKE_LOG"

code=0
where=""
emit=""
mode=""
i=0
while IFS='|' read -r c w e m; do
  i=$((i+1))
  if [ "$i" -eq "$n" ]; then
    code="$c"
    where="$w"
    emit="$e"
    mode="$m"
    break
  fi
done < "$FAKE_PLAN"

if [ -n "$emit" ]; then
  if [ "$where" = "transcript" ]; then
    printf '%s\n' "$emit" >> "$f"
  else
    printf '%s\n' "$emit"
  fi
fi

if [ "$mode" = "wait" ] || [ "$mode" = "busy" ]; then
  i=0
  while [ "$i" -lt 400 ]; do
    if [ -n "$FAKE_RELEASE" ] && [ -f "$FAKE_RELEASE" ]; then break; fi
    if [ "$mode" = "busy" ]; then
      printf '{"type":"user","sessionId":"%s","uuid":"t%s","parentUuid":"m1"}\n' "$sid" "$i" >> "$f"
    fi
    sleep 0.05
    i=$((i+1))
  done
fi

exit "$code"
`

// runStep es una línea del plan del falso claude (step ya lo usa policy_test.go).
type runStep struct {
	exit  int
	where string // "stdout" | "transcript" | "" (no emitir)
	emit  string
	wait  bool // quedarse vivo hasta que el test libere o el supervisor mate
	// busy es wait + ESCRIBIR en el transcript cada rodaja: es la sesión que el
	// usuario está usando ahora mismo, la que el guard de inactividad protege.
	busy bool
}

// limitStdout es la línea que `claude -p --output-format stream-json` imprime
// cuando topa el límite. El `tag` solo hace legible qué lanzamiento la emitió:
// desde que runner.seen tiene alcance por lanzamiento, dos límites byte-idénticos
// en perfiles distintos rotan igual —y eso lo prueba
// TestRunLimiteIdenticoEnDosPerfilesSigueRotando, que a propósito NO usa tag.
func limitStdout(tag string) string {
	return `{"type":"api_retry","error":"rate_limit","error_status":429,"message":"You've hit your session limit (` + tag + `)"}`
}

// limitTranscript es la forma que toma el mismo suceso en el jsonl (la única
// visible en modo interactive).
func limitTranscript(tag string) string {
	return `{"type":"assistant","isApiErrorMessage":true,"apiErrorStatus":429,"message":"You've hit your session limit (` + tag + `)"}`
}

type env struct {
	home    string
	cwd     string
	bin     string
	count   string
	log     string
	release string
	out     *safeBuf
	errb    *safeBuf
}

// unblock suelta al lanzamiento que esté en modo "wait" para que salga por su
// cuenta con el código de su plan.
func (e *env) unblock(t *testing.T) {
	t.Helper()
	if err := os.WriteFile(e.release, []byte("go\n"), 0o644); err != nil {
		t.Error(err)
	}
}

// setIdleAge envejece el mtime de los transcripts de `profile` en `age`, que es
// como se le dice al guard de inactividad si la sesión está viva o callada.
//
// Por qué así y no durmiendo: el guard mide con el reloj de PARED (un mtime lo
// es) y no con el reloj inyectado, así que un test no puede fingir silencio
// moviendo `clock`. Pedir un `return_idle` de milisegundos y dormirlos de verdad
// tampoco sirve —fue lo que hacía este test y fallaba bajo carga: entre el
// lanzamiento y la primera oportunidad ya se habían cumplido, y el guard nunca
// llegaba a aplazar nada—. Fijando el mtime, "ocioso" y "activo" son estados que
// el test decide, no una carrera con el planificador.
func (e *env) setIdleAge(t *testing.T, profile string, age time.Duration) {
	t.Helper()
	ccHome, err := core.CCHome(e.home, profile)
	if err != nil {
		t.Fatal(err)
	}
	dir := core.ProjectDir(ccHome, core.SlugForCwd(e.cwd))
	paths, err := filepath.Glob(filepath.Join(dir, "*.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) == 0 {
		t.Fatalf("no hay ningún transcript en %s", dir)
	}
	when := time.Now().Add(-age)
	for _, p := range paths {
		if err := os.Chtimes(p, when, when); err != nil {
			t.Fatal(err)
		}
	}
}

// launches cuenta cuántas veces se lanzó el falso claude.
func (e *env) launches(t *testing.T) int {
	t.Helper()
	data, err := os.ReadFile(e.count)
	if err != nil {
		return 0
	}
	n := 0
	for _, c := range strings.TrimSpace(string(data)) {
		n = n*10 + int(c-'0')
	}
	return n
}

// profiles devuelve el perfil con el que corrió cada lanzamiento, en orden.
func (e *env) profiles(t *testing.T) []string {
	t.Helper()
	data, err := os.ReadFile(e.log)
	if err != nil {
		return nil
	}
	var out []string
	for _, ln := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		f := strings.Fields(ln)
		if len(f) >= 2 {
			out = append(out, f[1])
		}
	}
	return out
}

type seed struct {
	fallback    []string
	maxHops     int
	cooldown    string // duración del respaldo de cooldown ("" => 1h)
	minDwell    string // "" => "0s"
	returnCheck string // periodo del temporizador de regreso ("" => default 10m)
	returnIdle  string // silencio exigido para el regreso ("" => "1ns")
	plan        []runStep
}

// setup siembra un home de ccp completo (tres perfiles official, una regla que
// hace primario a p1 en el cwd, y el bloque auto_handoff) más el claude falso.
func setup(t *testing.T, s seed) *env {
	t.Helper()

	home := t.TempDir()
	cwd := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatal(err)
	}

	cooldown := s.cooldown
	if cooldown == "" {
		cooldown = "1h"
	}
	minDwell := s.minDwell
	if minDwell == "" {
		minDwell = "0s"
	}
	// El guard de inactividad se deja ARMADO pero trivialmente satisfecho salvo
	// que el test lo estudie: "1ns" ejercita el camino real (stat del transcript,
	// comparación contra el reloj de pared) sin obligar a cada test del
	// temporizador a esperar 90 segundos de silencio de verdad. Ponerlo a "0s"
	// —que lo desactiva— sería peor: dejaría el guard sin cobertura en los tests
	// que hacen volver la sesión a casa.
	returnIdle := s.returnIdle
	if returnIdle == "" {
		returnIdle = "1ns"
	}
	cfg := &core.Config{
		Version: core.SchemaVersion,
		Profiles: map[string]core.Profile{
			"p1": {Type: "official"},
			"p2": {Type: "official"},
			"p3": {Type: "official"},
		},
		Rules: []core.Rule{{Path: cwd, Profile: "p1"}},
		AutoHandoff: &core.AutoHandoff{
			Enabled: true,
			Policies: map[string]core.AutoPolicy{
				"default": {
					Fallback:    s.fallback,
					MinDwell:    minDwell,
					MaxHops:     s.maxHops,
					ReturnCheck: s.returnCheck,
					ReturnIdle:  returnIdle,
					Cooldown:    core.AutoCooldown{Fallback: cooldown},
				},
			},
		},
	}
	if err := core.Save(home, cfg); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	bin := filepath.Join(dir, "fake-claude")
	if err := os.WriteFile(bin, []byte(fakeScript), 0o755); err != nil {
		t.Fatal(err)
	}

	var lines []string
	for _, st := range s.plan {
		mode := ""
		if st.wait {
			mode = "wait"
		}
		if st.busy {
			mode = "busy"
		}
		lines = append(lines, strconv.Itoa(st.exit)+"|"+st.where+"|"+st.emit+"|"+mode)
	}
	plan := filepath.Join(dir, "plan")
	if err := os.WriteFile(plan, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	e := &env{
		home:    home,
		cwd:     cwd,
		bin:     bin,
		count:   filepath.Join(dir, "count"),
		log:     filepath.Join(dir, "log"),
		release: filepath.Join(dir, "release"),
		out:     &safeBuf{},
		errb:    &safeBuf{},
	}
	t.Setenv("FAKE_PLAN", plan)
	t.Setenv("FAKE_COUNT", e.count)
	t.Setenv("FAKE_LOG", e.log)
	t.Setenv("FAKE_SLUG", core.SlugForCwd(cwd))
	t.Setenv("FAKE_RELEASE", e.release)
	return e
}

// --- reloj inyectado --------------------------------------------------------

// clock es el reloj falso de los tests del temporizador de regreso: lo que en
// producción son cinco horas de ventana aquí es un `advance` desde el test.
//
// Con candado porque lo leen dos goroutines: la del bucle (o.Now, dentro de
// Run) y la del propio test, que es quien lo mueve mientras Run está bloqueado.
type clock struct {
	mu sync.Mutex
	t  time.Time
}

func newClock(at time.Time) *clock { return &clock{t: at} }

func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *clock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

// awaitCond sondea `cond` hasta que se cumple. Devuelve false si se agota el
// plazo, en vez de fallar: lo llaman goroutines auxiliares, y t.Fatal fuera de
// la goroutine del test es ilegal.
func awaitCond(cond func() bool) bool {
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(2 * time.Millisecond)
	}
	return false
}

// settle es la ventana en la que se comprueba que algo NO pasa. Con Poll de 5ms
// y return_check de 1ms, un temporizador mal armado dispararía en el primer
// tick; 80ms son ~16 oportunidades de equivocarse.
const settle = 80 * time.Millisecond

// opts arma unas Options de test: headless (el modo determinista, con el
// stream-json como sensor) y Poll de milisegundos.
func (e *env) opts(session string) Options {
	return Options{
		Home:      e.home,
		Cwd:       e.cwd,
		Headless:  true,
		Session:   session,
		ClaudeBin: e.bin,
		Out:       e.out,
		Err:       e.errb,
		Poll:      5 * time.Millisecond,
	}
}

const seedSession = "aaaaaaaa-1111-4111-8111-aaaaaaaaaaaa"

func handoffs(t *testing.T, home string) *core.Handoffs {
	t.Helper()
	h, err := core.LoadHandoffs(home)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

// --- (a) sin límite ---------------------------------------------------------

func TestRunSinLimiteUnSoloLanzamiento(t *testing.T) {
	e := setup(t, seed{
		fallback: []string{"p2"},
		plan:     []runStep{{exit: 0}},
	})

	res, err := Run(context.Background(), e.opts(seedSession))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, quería 0 (out=%q err=%q)", res.ExitCode, e.out.String(), e.errb.String())
	}
	if len(res.Hops) != 0 {
		t.Fatalf("no debería rotar: %+v", res.Hops)
	}
	if got := e.launches(t); got != 1 {
		t.Fatalf("lanzamientos = %d, quería 1", got)
	}
	if res.Profile != "p1" || res.Session != seedSession {
		t.Fatalf("terminó en %s/%s, quería p1/%s", res.Profile, res.Session, seedSession)
	}
	if h := handoffs(t, e.home); len(h.Active) != 0 || len(h.Archived) != 0 {
		t.Fatalf("no debería haber marcadores: %+v / %+v", h.Active, h.Archived)
	}
}

// --- (b) un hop y vuelta a casa al terminar ---------------------------------

func TestRunUnHopYCierreDelMarcador(t *testing.T) {
	e := setup(t, seed{
		fallback: []string{"p2"},
		maxHops:  3,
		plan: []runStep{
			{exit: 1, where: "stdout", emit: limitStdout("p1")},
			{exit: 0},
		},
	})

	res, err := Run(context.Background(), e.opts(seedSession))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, quería 0 (out=%q err=%q)", res.ExitCode, e.out.String(), e.errb.String())
	}
	if len(res.Hops) != 1 || res.Hops[0].From != "p1" || res.Hops[0].To != "p2" {
		t.Fatalf("hops = %+v, quería un p1→p2", res.Hops)
	}
	if got := e.profiles(t); len(got) != 2 || got[0] != "p1" || got[1] != "p2" {
		t.Fatalf("perfiles lanzados = %v, quería [p1 p2]", got)
	}
	// La sesión volvió a casa: uuid NUEVO en el primario y marcador archivado.
	if res.Profile != "p1" {
		t.Fatalf("terminó en %s, quería p1", res.Profile)
	}
	if res.Session == seedSession || res.Session == "" {
		t.Fatalf("la vuelta a casa debe dar un uuid nuevo, dio %q", res.Session)
	}
	h := handoffs(t, e.home)
	if len(h.Active) != 0 {
		t.Fatalf("el marcador debía archivarse: %+v", h.Active)
	}
	if len(h.Archived) != 1 || h.Archived[0].From != "p1" || h.Archived[0].To != "p2" {
		t.Fatalf("archivado inesperado: %+v", h.Archived)
	}
	if h.Archived[0].ReturnedAs != res.Session {
		t.Fatalf("returned_as = %q, quería %q", h.Archived[0].ReturnedAs, res.Session)
	}
	// El transcript de vuelta existe en el primario con el uuid nuevo.
	cc := filepath.Join(e.home, "profiles", "p1", "cc-home")
	back := filepath.Join(core.ProjectDir(cc, core.SlugForCwd(e.cwd)), res.Session+".jsonl")
	if _, err := os.Stat(back); err != nil {
		t.Fatalf("no se escribió el transcript de vuelta: %v", err)
	}
	if tr := e.out.String(); !strings.Contains(tr, "handoff a p2") {
		t.Fatalf("la traza no reporta el hop: %q", tr)
	}
}

// --- (c) vuelta al primario a mitad de la rotación --------------------------

// Con el cooldown vencido, la regla del péndulo manda: el segundo salto NO va al
// siguiente préstamo (p3, que está fresco) sino de vuelta al primario.
func TestRunSegundoHopVuelveAlPrimario(t *testing.T) {
	e := setup(t, seed{
		fallback: []string{"p2", "p3"},
		maxHops:  4,
		cooldown: "0s", // el primario queda disponible en cuanto se marca agotado
		plan: []runStep{
			{exit: 1, where: "stdout", emit: limitStdout("p1")},
			{exit: 1, where: "stdout", emit: limitStdout("p2")},
			{exit: 0},
		},
	})

	res, err := Run(context.Background(), e.opts(seedSession))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, quería 0 (out=%q err=%q)", res.ExitCode, e.out.String(), e.errb.String())
	}
	if len(res.Hops) != 2 {
		t.Fatalf("hops = %+v, quería 2", res.Hops)
	}
	if res.Hops[1].To != "p1" {
		t.Fatalf("el segundo salto fue a %s, quería volver a p1", res.Hops[1].To)
	}
	if got := e.profiles(t); len(got) != 3 || got[2] != "p1" {
		t.Fatalf("perfiles lanzados = %v, quería terminar en p1", got)
	}
	// Volver a casa cierra el marcador: nada activo y el uuid cambió.
	if h := handoffs(t, e.home); len(h.Active) != 0 || len(h.Archived) != 1 {
		t.Fatalf("marcadores inesperados: %+v / %+v", h.Active, h.Archived)
	}
	if res.Hops[1].Session == seedSession {
		t.Fatalf("la vuelta a casa debe cambiar el uuid")
	}
	if tr := e.out.String(); !strings.Contains(tr, "volviendo a p1") {
		t.Fatalf("la traza no reporta la vuelta: %q", tr)
	}
}

// --- (d) todos agotados -----------------------------------------------------

func TestRunTodosAgotadosSaleCon75(t *testing.T) {
	e := setup(t, seed{
		fallback: []string{"p2", "p3"},
		maxHops:  6,
		cooldown: "1h",
		plan: []runStep{
			{exit: 1, where: "stdout", emit: limitStdout("p1")},
			{exit: 1, where: "stdout", emit: limitStdout("p2")},
			{exit: 1, where: "stdout", emit: limitStdout("p3")},
		},
	})

	res, err := Run(context.Background(), e.opts(seedSession))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.ExitCode != ParkedExitCode || !res.Parked {
		t.Fatalf("ExitCode = %d parked=%v, quería 75/true (out=%q)", res.ExitCode, res.Parked, e.out.String())
	}
	if len(res.Hops) != 2 {
		t.Fatalf("hops = %+v, quería 2", res.Hops)
	}
	if got := e.launches(t); got != 3 {
		t.Fatalf("lanzamientos = %d, quería 3", got)
	}
	tr := e.out.String()
	if !strings.Contains(tr, "cooldown") {
		t.Fatalf("falta la tabla de cooldowns: %q", tr)
	}
	for _, p := range []string{"p1", "p2", "p3"} {
		if !strings.Contains(tr, p) {
			t.Fatalf("la tabla no menciona %s: %q", p, tr)
		}
	}
	// El marcador se queda VIVO: la conversación está en p3 y el usuario debe
	// poder reanudarla ahí cuando la ventana reabra.
	if h := handoffs(t, e.home); len(h.Active) != 1 || h.Active[0].To != "p3" {
		t.Fatalf("marcador esperado p1→p3 vivo: %+v", h.Active)
	}
}

// --- (e) max_hops corta la rotación ----------------------------------------

func TestRunMaxHopsCortaLaRotacion(t *testing.T) {
	e := setup(t, seed{
		fallback: []string{"p2", "p3"},
		maxHops:  1,
		cooldown: "1h",
		plan: []runStep{
			{exit: 1, where: "stdout", emit: limitStdout("p1")},
			{exit: 1, where: "stdout", emit: limitStdout("p2")},
			{exit: 1, where: "stdout", emit: limitStdout("p3")},
		},
	})

	res, err := Run(context.Background(), e.opts(seedSession))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.ExitCode != ParkedExitCode || !res.Parked {
		t.Fatalf("ExitCode = %d parked=%v, quería 75/true", res.ExitCode, res.Parked)
	}
	if len(res.Hops) != 1 {
		t.Fatalf("hops = %+v, quería 1 (max_hops=1)", res.Hops)
	}
	if got := e.launches(t); got != 2 {
		t.Fatalf("lanzamientos = %d, quería 2", got)
	}
	if got := e.profiles(t); len(got) == 3 && got[2] == "p3" {
		t.Fatalf("p3 no debía usarse con max_hops=1: %v", got)
	}
	if tr := e.out.String(); !strings.Contains(tr, "max_hops") {
		t.Fatalf("la traza no explica el tope de préstamos: %q", tr)
	}
}

// --- (f) dry-run ------------------------------------------------------------

func TestRunDryRunNoLanzaNada(t *testing.T) {
	e := setup(t, seed{
		fallback: []string{"p2", "p3"},
		plan:     []runStep{{exit: 7}},
	})
	o := e.opts(seedSession)
	o.DryRun = true

	res, err := Run(context.Background(), o)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, quería 0", res.ExitCode)
	}
	if got := e.launches(t); got != 0 {
		t.Fatalf("--dry-run no debe lanzar nada, lanzó %d", got)
	}
	tr := e.out.String()
	for _, want := range []string{"primario: p1", "p2 → p3", "umbral", "--dry-run"} {
		if !strings.Contains(tr, want) {
			t.Fatalf("el dry-run no reporta %q: %q", want, tr)
		}
	}
}

// --- (g) Ctrl-C no rota -----------------------------------------------------

// Un 130 no rota AUNQUE el sensor haya visto un límite: el usuario interrumpió
// a propósito y mover su conversación de perfil sería lo contrario de lo pedido.
func TestRunCtrlCNoRota(t *testing.T) {
	e := setup(t, seed{
		fallback: []string{"p2"},
		maxHops:  3,
		plan: []runStep{
			{exit: 130, where: "stdout", emit: limitStdout("p1")},
			{exit: 0},
		},
	})

	res, err := Run(context.Background(), e.opts(seedSession))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.ExitCode != 130 {
		t.Fatalf("ExitCode = %d, quería 130 (out=%q)", res.ExitCode, e.out.String())
	}
	if len(res.Hops) != 0 {
		t.Fatalf("no debía rotar: %+v", res.Hops)
	}
	if got := e.launches(t); got != 1 {
		t.Fatalf("lanzamientos = %d, quería 1", got)
	}
	if tr := e.out.String(); !strings.Contains(tr, "Ctrl-C") {
		t.Fatalf("la traza no menciona la interrupción: %q", tr)
	}
}

// --- (h) el temporizador de return_check ------------------------------------
//
// Los cinco tests siguientes son el contrato de UC-7: estando prestado, la
// sesión vuelve a casa CUANDO la ventana del primario reabre, no cuando algo
// más se rompe. Todos usan reloj inyectado (o.Now) y un hijo en modo "wait",
// que es lo que permite reproducir «son las 3am y el primario acaba de
// liberarse» sin dormir cinco horas.

// El caso que motivó la feature: nadie topa ningún límite nuevo, simplemente
// pasa el tiempo y el primario recupera su ventana. Sin temporizador, esta
// corrida se quedaba en p2 hasta que el usuario volviera por la mañana.
func TestRunReturnCheckVuelveAlPrimarioSinLimiteNuevo(t *testing.T) {
	e := setup(t, seed{
		fallback:    []string{"p2", "p3"},
		maxHops:     4,
		cooldown:    "1h",
		returnCheck: "10m",
		plan: []runStep{
			{exit: 1, where: "stdout", emit: limitStdout("p1")}, // p1 topa su límite
			{exit: 0, wait: true},                               // p2: vivo hasta que lo maten
			{exit: 0},                                           // de vuelta en p1: termina
		},
	})
	clk := newClock(t0)
	o := e.opts(seedSession)
	o.Now = clk.Now

	done := make(chan struct{})
	go func() {
		defer close(done)
		// En cuanto la conversación está prestada en p2, el reloj cruza el
		// cooldown del primario: es la ventana de 5h que reabre de madrugada.
		if !awaitCond(func() bool { return e.launches(t) >= 2 }) {
			t.Error("el segundo lanzamiento nunca ocurrió")
			return
		}
		clk.advance(2 * time.Hour)
	}()

	res, err := Run(context.Background(), o)
	<-done
	if err != nil {
		t.Fatalf("Run: %v (out=%q err=%q)", err, e.out.String(), e.errb.String())
	}
	if res.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, quería 0 (out=%q err=%q)", res.ExitCode, e.out.String(), e.errb.String())
	}
	if got := e.profiles(t); len(got) != 3 || got[2] != "p1" {
		t.Fatalf("perfiles lanzados = %v, quería [p1 p2 p1]", got)
	}
	if len(res.Hops) != 2 {
		t.Fatalf("hops = %+v, quería 2 (p1→p2 por límite, p2→p1 por return_check)", res.Hops)
	}
	back := res.Hops[1]
	if back.From != "p2" || back.To != "p1" {
		t.Fatalf("el segundo movimiento fue %s→%s, quería p2→p1", back.From, back.To)
	}
	if !strings.Contains(back.Reason, "return_check") {
		t.Fatalf("Reason = %q, quería que nombrara return_check", back.Reason)
	}
	// El marcador se archiva y la sesión vive con uuid NUEVO en el primario.
	h := handoffs(t, e.home)
	if len(h.Active) != 0 || len(h.Archived) != 1 {
		t.Fatalf("marcadores = %+v / %+v, quería el préstamo archivado", h.Active, h.Archived)
	}
	if res.Session == seedSession || res.Session == "" {
		t.Fatalf("la vuelta a casa debe dar un uuid nuevo, dio %q", res.Session)
	}
	if h.Archived[0].ReturnedAs != res.Session {
		t.Fatalf("returned_as = %q, quería %q", h.Archived[0].ReturnedAs, res.Session)
	}
	tr := e.out.String()
	if !strings.Contains(tr, "return_check: p1 ya liberó su ventana") || !strings.Contains(tr, "volviendo a p1") {
		t.Fatalf("la traza no explica por qué se movió: %q", tr)
	}
	// Mismo vocabulario que la vuelta a casa POR LÍMITE (ver
	// TestTrazaIdaYVueltaYVueltaAIrHablaUnSoloIdioma): el suceso es el mismo, así
	// que el presupuesto se cuenta igual y con la misma explicación de por qué el
	// número no sube.
	if !strings.Contains(tr, "volviendo a p1 (vuelta a casa, no gasta préstamo: siguen 1/4)") {
		t.Fatalf("el regreso por temporizador no usa el vocabulario del presupuesto: %q", tr)
	}
}

// El temporizador mira el cooldown, no el reloj: mientras el primario siga
// agotado no hay regreso por mucho que corra el tiempo.
func TestRunReturnCheckNoDisparaConElPrimarioEnCooldown(t *testing.T) {
	e := setup(t, seed{
		fallback:    []string{"p2"},
		maxHops:     4,
		cooldown:    "1h",
		returnCheck: "1ms", // el periodo no es la variable bajo estudio
		plan: []runStep{
			{exit: 1, where: "stdout", emit: limitStdout("p1")},
			{exit: 0, wait: true},
			{exit: 0},
		},
	})
	clk := newClock(t0)
	o := e.opts(seedSession)
	o.Now = clk.Now

	done := make(chan struct{})
	go func() {
		defer close(done)
		if !awaitCond(func() bool { return e.launches(t) >= 2 }) {
			t.Error("el segundo lanzamiento nunca ocurrió")
			return
		}
		clk.advance(59 * time.Minute) // un minuto antes de que reabra el primario
		time.Sleep(settle)
		if n := e.launches(t); n != 2 {
			t.Errorf("volvió al primario con el cooldown aún vigente (lanzamientos=%d)", n)
		}
		clk.advance(2 * time.Minute) // ahora sí: el cooldown de 1h ya venció
	}()

	res, err := Run(context.Background(), o)
	<-done
	if err != nil {
		t.Fatalf("Run: %v (err=%q)", err, e.errb.String())
	}
	if res.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, quería 0 (out=%q)", res.ExitCode, e.out.String())
	}
	if len(res.Hops) != 2 || res.Hops[1].To != "p1" {
		t.Fatalf("hops = %+v, quería volver a p1 solo tras vencer el cooldown", res.Hops)
	}
}

// Estando ya en casa, la corrida no se mueve por mucho que el reloj salte cinco
// horas: ni relanzamientos, ni hops, ni marcadores, ni una línea de traza.
//
// OJO con lo que este test NO prueba, porque antes decía probarlo: no verifica
// que el temporizador «ni se arma». Esa propiedad no tiene efecto observable —
// con el ticker armado a la fuerza, decideReturn contesta returnNotYet
// (Chain.ReturnDue es false mientras current == primary) y la corrida sale
// idéntica byte a byte. Aquí se comprueba el RESULTADO de la defensa en
// profundidad; el guard concreto que arma el ticker lo cubre
// TestArmReturnTickerSoloConPrestamoVivo, y su efecto en una corrida real lo
// cubre TestRunPrestamoDegradadoNoArmaElTemporizadorDeRegreso (return_guard_test.go),
// que es el único escenario donde armar de más SÍ mueve la conversación.
func TestRunEstandoEnCasaNadaSeMueve(t *testing.T) {
	e := setup(t, seed{
		fallback:    []string{"p2"},
		maxHops:     4,
		cooldown:    "1h",
		returnCheck: "1ms",
		plan:        []runStep{{exit: 0, wait: true}},
	})
	clk := newClock(t0)
	o := e.opts(seedSession)
	o.Now = clk.Now

	done := make(chan struct{})
	go func() {
		defer close(done)
		if !awaitCond(func() bool { return e.launches(t) >= 1 }) {
			t.Error("no arrancó el primer lanzamiento")
			return
		}
		clk.advance(5 * time.Hour)
		time.Sleep(settle)
		e.unblock(t)
	}()

	res, err := Run(context.Background(), o)
	<-done
	if err != nil {
		t.Fatalf("Run: %v (err=%q)", err, e.errb.String())
	}
	if res.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, quería 0", res.ExitCode)
	}
	if got := e.launches(t); got != 1 {
		t.Fatalf("lanzamientos = %d, quería 1: nadie debía mover una sesión que ya está en casa", got)
	}
	if len(res.Hops) != 0 || res.Session != seedSession {
		t.Fatalf("no debía moverse nada: hops=%+v session=%q", res.Hops, res.Session)
	}
	if h := handoffs(t, e.home); len(h.Active) != 0 || len(h.Archived) != 0 {
		t.Fatalf("no debía haber marcadores: %+v / %+v", h.Active, h.Archived)
	}
	if tr := e.out.String(); strings.Contains(tr, "return_check") {
		t.Fatalf("la traza reporta un regreso que no ocurrió: %q", tr)
	}
}

// --no-return es exactamente eso: la sesión se queda en el préstamo aunque el
// primario lleve horas libre.
func TestRunReturnCheckNoReturnDesactivaElTemporizador(t *testing.T) {
	e := setup(t, seed{
		fallback:    []string{"p2"},
		maxHops:     4,
		cooldown:    "1h",
		returnCheck: "1ms",
		plan: []runStep{
			{exit: 1, where: "stdout", emit: limitStdout("p1")},
			{exit: 0, wait: true},
		},
	})
	clk := newClock(t0)
	o := e.opts(seedSession)
	o.Now = clk.Now
	o.NoReturn = true

	done := make(chan struct{})
	go func() {
		defer close(done)
		if !awaitCond(func() bool { return e.launches(t) >= 2 }) {
			t.Error("el segundo lanzamiento nunca ocurrió")
			return
		}
		clk.advance(5 * time.Hour)
		time.Sleep(settle)
		if n := e.launches(t); n != 2 {
			t.Errorf("--no-return no impidió el regreso (lanzamientos=%d)", n)
		}
		e.unblock(t) // el hijo termina por su cuenta y cierra la corrida
	}()

	res, err := Run(context.Background(), o)
	<-done
	if err != nil {
		t.Fatalf("Run: %v (err=%q)", err, e.errb.String())
	}
	if res.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, quería 0 (out=%q)", res.ExitCode, e.out.String())
	}
	if got := e.launches(t); got != 2 {
		t.Fatalf("lanzamientos = %d, quería 2 (sin relanzar en el primario)", got)
	}
	if len(res.Hops) != 1 || res.Hops[0].To != "p2" {
		t.Fatalf("hops = %+v, quería solo el préstamo p1→p2", res.Hops)
	}
	if tr := e.out.String(); strings.Contains(tr, "return_check") {
		t.Fatalf("con --no-return no debía haber regreso por temporizador: %q", tr)
	}
}

// min_dwell gobierna el regreso igual que la rotación: el primario puede llevar
// libre un rato, pero no se arranca al usuario del préstamo recién llegado.
func TestRunReturnCheckRespetaMinDwell(t *testing.T) {
	e := setup(t, seed{
		fallback:    []string{"p2"},
		maxHops:     4,
		cooldown:    "1s", // el primario reabre casi al instante
		minDwell:    "30m",
		returnCheck: "1ms",
		plan: []runStep{
			// El primer hijo sigue VIVO tras avisar del límite: es la única forma
			// de que min_dwell gobierne la rotación (a un hijo ya muerto no se le
			// puede esperar) y de que la llegada a p2 quede fechada por el reloj
			// falso y no por el arranque.
			{exit: 1, where: "stdout", emit: limitStdout("p1"), wait: true},
			{exit: 0, wait: true},
			{exit: 0},
		},
	})
	clk := newClock(t0)
	o := e.opts(seedSession)
	o.Now = clk.Now

	done := make(chan struct{})
	go func() {
		defer close(done)
		// min_dwell también frena el PRIMER salto: hay que dejar pasar el reloj
		// para que la sesión llegue siquiera a estar prestada.
		if !awaitCond(func() bool { return strings.Contains(e.errb.String(), "antes de rotar") }) {
			t.Error("no se llegó a esperar el min_dwell de la rotación")
			return
		}
		clk.advance(31 * time.Minute)
		if !awaitCond(func() bool { return e.launches(t) >= 2 }) {
			t.Error("el segundo lanzamiento nunca ocurrió")
			return
		}
		// Prestados desde hace 5 minutos y el primario ya libre: aún no toca.
		clk.advance(5 * time.Minute)
		time.Sleep(settle)
		if n := e.launches(t); n != 2 {
			t.Errorf("volvió al primario a los 5m con min_dwell de 30m (lanzamientos=%d): %q", n, e.out.String())
		}
		clk.advance(30 * time.Minute)
	}()

	res, err := Run(context.Background(), o)
	<-done
	if err != nil {
		t.Fatalf("Run: %v (err=%q)", err, e.errb.String())
	}
	if res.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, quería 0 (out=%q)", res.ExitCode, e.out.String())
	}
	if len(res.Hops) != 2 || res.Hops[1].To != "p1" {
		t.Fatalf("hops = %+v, quería volver a p1 tras cumplir min_dwell", res.Hops)
	}
	if !strings.Contains(e.errb.String(), "antes de volver") {
		t.Fatalf("no se avisó de la espera por min_dwell del regreso: %q", e.errb.String())
	}
}

// La decisión de diseño, hecha test: max_hops cuenta PRÉSTAMOS. Con presupuesto
// 2, una ida + una vuelta + otra ida caben; si la vuelta cobrara hop, la segunda
// ida no existiría y la corrida moriría con 75 en el tercer lanzamiento.
func TestRunVueltaACasaNoConsumePresupuestoDeMaxHops(t *testing.T) {
	e := setup(t, seed{
		fallback:    []string{"p2"},
		maxHops:     2,
		cooldown:    "1h",
		returnCheck: "1ms",
		plan: []runStep{
			{exit: 1, where: "stdout", emit: limitStdout("p1-a")}, // préstamo 1
			{exit: 0, wait: true}, // p2 hasta que el reloj lo saque
			{exit: 1, where: "stdout", emit: limitStdout("p1-b")}, // en casa, vuelve a topar
			{exit: 0}, // préstamo 2: termina
		},
	})
	clk := newClock(t0)
	o := e.opts(seedSession)
	o.Now = clk.Now

	done := make(chan struct{})
	go func() {
		defer close(done)
		if !awaitCond(func() bool { return e.launches(t) >= 2 }) {
			t.Error("el segundo lanzamiento nunca ocurrió")
			return
		}
		clk.advance(2 * time.Hour)
	}()

	res, err := Run(context.Background(), o)
	<-done
	if err != nil {
		t.Fatalf("Run: %v (err=%q)", err, e.errb.String())
	}
	if res.ExitCode != 0 || res.Parked {
		t.Fatalf("ExitCode = %d parked=%v, quería 0/false: la vuelta no debe gastar presupuesto (out=%q)",
			res.ExitCode, res.Parked, e.out.String())
	}
	if got := e.profiles(t); len(got) != 4 || got[0] != "p1" || got[1] != "p2" || got[2] != "p1" || got[3] != "p2" {
		t.Fatalf("perfiles lanzados = %v, quería [p1 p2 p1 p2]", got)
	}
	if len(res.Hops) != 3 {
		t.Fatalf("hops = %+v, quería 3 (préstamo, regreso, préstamo)", res.Hops)
	}
}

// --- extras -----------------------------------------------------------------

// Una salida no-cero SIN evidencia de límite se propaga tal cual: `ccp session`
// tiene que ser sustituible por `claude` dentro de un script.
func TestRunPropagaExitCodeSinLimite(t *testing.T) {
	e := setup(t, seed{
		fallback: []string{"p2"},
		plan:     []runStep{{exit: 42}},
	})

	res, err := Run(context.Background(), e.opts(seedSession))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.ExitCode != 42 {
		t.Fatalf("ExitCode = %d, quería 42", res.ExitCode)
	}
	if len(res.Hops) != 0 || e.launches(t) != 1 {
		t.Fatalf("no debía rotar ni relanzar: %+v / %d", res.Hops, e.launches(t))
	}
}

// Modo interactive: el único sensor reactivo es el transcript. Además se
// comprueba la trampa del encadenado: HandoffChain COPIA el jsonl al perfil
// destino, así que el watcher del segundo lanzamiento vuelve a leer la línea del
// límite desde el byte 0 — y no debe provocar un segundo salto.
func TestRunInteractiveDetectaPorTranscriptSinRebote(t *testing.T) {
	e := setup(t, seed{
		fallback: []string{"p2", "p3"},
		maxHops:  4,
		cooldown: "1h",
		plan: []runStep{
			{exit: 1, where: "transcript", emit: limitTranscript("p1")},
			{exit: 0},
		},
	})
	o := e.opts(seedSession)
	o.Headless = false

	res, err := Run(context.Background(), o)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, quería 0 (out=%q err=%q)", res.ExitCode, e.out.String(), e.errb.String())
	}
	if len(res.Hops) != 1 || res.Hops[0].To != "p2" {
		t.Fatalf("hops = %+v, quería exactamente uno a p2", res.Hops)
	}
	if got := e.launches(t); got != 2 {
		t.Fatalf("lanzamientos = %d, quería 2 (la línea vieja no debe re-disparar)", got)
	}
}

// En interactive el hijo tiene que HEREDAR los descriptores del padre, no
// recibir un pipe: os/exec solo pasa el fd cuando el writer es un *os.File, y
// con cualquier otro io.Writer monta un os.Pipe() + goroutine de copia. Si la
// salida del supervisor se envolviera antes de dársela al hijo, claude vería
// `process.stdout.isTTY === false` (adiós TUI, colores y SIGWINCH) con stdin sí
// siendo tty, y además cmd.Wait dejaría de terminar con el hijo para esperar el
// EOF de los pipes (WaitDelay), reportando exit 1 en sesiones que salieron bien.
func TestRunInteractiveElHijoHeredaLosFds(t *testing.T) {
	e := setup(t, seed{fallback: []string{"p2"}, plan: []runStep{{exit: 0}}})

	dir := t.TempDir()
	probe := filepath.Join(dir, "probe")
	script := "#!/bin/sh\n" +
		"if [ -p /dev/fd/1 ]; then echo STDOUT=PIPE; else echo STDOUT=FILE; fi\n" +
		"if [ -p /dev/fd/2 ]; then echo STDERR=PIPE >&2; else echo STDERR=FILE >&2; fi\n"
	if err := os.WriteFile(probe, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	outF, err := os.Create(filepath.Join(dir, "stdout"))
	if err != nil {
		t.Fatal(err)
	}
	defer outF.Close()
	errF, err := os.Create(filepath.Join(dir, "stderr"))
	if err != nil {
		t.Fatal(err)
	}
	defer errF.Close()

	o := e.opts(seedSession)
	o.Headless = false
	o.ClaudeBin = probe
	o.Out = outF
	o.Err = errF

	if _, err := Run(context.Background(), o); err != nil {
		t.Fatalf("Run: %v", err)
	}
	out, err := os.ReadFile(outF.Name())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "STDOUT=FILE") {
		t.Fatalf("el hijo no heredó stdout: %q", string(out))
	}
	errOut, err := os.ReadFile(errF.Name())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(errOut), "STDERR=FILE") {
		t.Fatalf("el hijo no heredó stderr: %q", string(errOut))
	}
}

// limitStdoutResets es el límite del stream-json con un resets_at explícito, en
// epoch. Sirve para dar a cada perfil una ventana propia y así provocar el orden
// de disponibilidad que hace volver la cadena a un fallback ya visitado.
func limitStdoutResets(tag string, epoch int64) string {
	return `{"type":"api_retry","error":"rate_limit","error_status":429,"resets_at":` +
		strconv.FormatInt(epoch, 10) +
		`,"message":"You've hit your weekly limit (` + tag + `)"}`
}

// Volver a un fallback ya visitado es normal en cuanto vence su cooldown (el
// primario puede tener una ventana semanal y el préstamo una de 5h). El destino
// conserva la copia del jsonl del paso anterior, que es un PREFIJO de la
// conversación de ahora: pisarla es lo correcto y no puede abortar la corrida.
func TestRunReentraEnUnFallbackYaVisitado(t *testing.T) {
	// El primario reabre en 2100: nunca vuelve a estar disponible durante el test.
	farFuture := time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC).Unix()
	e := setup(t, seed{
		fallback: []string{"p2", "p3"},
		maxHops:  5,
		cooldown: "0s", // sin resets_at, un perfil se recupera al instante
		plan: []runStep{
			{exit: 1, where: "stdout", emit: limitStdoutResets("p1", farFuture)},
			{exit: 1, where: "transcript", emit: limitTranscript("p2")},
			{exit: 1, where: "transcript", emit: limitTranscript("p3")},
			{exit: 0},
		},
	})

	res, err := Run(context.Background(), e.opts(seedSession))
	if err != nil {
		t.Fatalf("Run: %v (out=%q err=%q)", err, e.out.String(), e.errb.String())
	}
	if res.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, quería 0 (err=%q)", res.ExitCode, e.errb.String())
	}
	if len(res.Hops) != 3 {
		t.Fatalf("hops = %+v, quería 3 (p1→p2, p2→p3, p3→p2)", res.Hops)
	}
	if res.Hops[2].From != "p3" || res.Hops[2].To != "p2" {
		t.Fatalf("el tercer salto fue %s→%s, quería p3→p2", res.Hops[2].From, res.Hops[2].To)
	}
	if got := e.launches(t); got != 4 {
		t.Fatalf("lanzamientos = %d, quería 4", got)
	}
	// La copia vieja del jsonl en p2 quedó pisada por la versión crecida.
	cc := filepath.Join(e.home, "profiles", "p2", "cc-home")
	data, err := os.ReadFile(filepath.Join(core.ProjectDir(cc, core.SlugForCwd(e.cwd)), seedSession+".jsonl"))
	if err != nil {
		t.Fatalf("no se pudo leer el transcript en p2: %v", err)
	}
	if n := strings.Count(strings.TrimSpace(string(data)), "\n") + 1; n != 3 {
		t.Fatalf("el transcript de p2 tiene %d líneas, quería las 3 de la conversación crecida: %q", n, string(data))
	}
}

// Un límite detectado ANTES de que exista el jsonl (el sensor proactivo puede
// disparar en una sesión donde el usuario no ha escrito ningún turno) no puede
// tumbar la corrida: no hay conversación que migrar, así que se rota igual con
// una sesión nueva en vez de abortar con exit 1 y el hijo ya muerto.
func TestRunHopSinTranscriptRotaIgual(t *testing.T) {
	e := setup(t, seed{
		fallback: []string{"p2"},
		maxHops:  3,
		plan:     []runStep{{exit: 0}}, // el plan no se usa: el script es otro
	})

	dir := t.TempDir()
	emit := filepath.Join(dir, "emit")
	if err := os.WriteFile(emit, []byte(limitStdout("sin-transcript")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Un claude que NUNCA escribe su jsonl: es lo que pasa cuando el límite llega
	// antes del primer turno del usuario.
	mute := filepath.Join(dir, "mudo")
	script := "#!/bin/sh\n" +
		"n=0\n" +
		"if [ -f \"$FAKE_COUNT\" ]; then read -r n < \"$FAKE_COUNT\"; fi\n" +
		"n=$((n+1))\n" +
		"printf '%s\\n' \"$n\" > \"$FAKE_COUNT\"\n" +
		"printf '%s %s\\n' \"$n\" \"$CCP_PROFILE\" >> \"$FAKE_LOG\"\n" +
		"if [ \"$n\" = \"1\" ]; then cat \"$FAKE_EMIT\"; exit 1; fi\n" +
		"exit 0\n"
	if err := os.WriteFile(mute, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FAKE_EMIT", emit)

	o := e.opts(seedSession)
	o.ClaudeBin = mute

	res, err := Run(context.Background(), o)
	if err != nil {
		t.Fatalf("Run: %v (err=%q)", err, e.errb.String())
	}
	if res.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, quería 0 (out=%q err=%q)", res.ExitCode, e.out.String(), e.errb.String())
	}
	if len(res.Hops) != 1 || res.Hops[0].To != "p2" {
		t.Fatalf("hops = %+v, quería uno a p2", res.Hops)
	}
	if got := e.profiles(t); len(got) != 2 || got[1] != "p2" {
		t.Fatalf("perfiles lanzados = %v, quería [p1 p2]", got)
	}
	// Nada que prestar ⇒ ningún marcador: no hubo conversación que devolver.
	if h := handoffs(t, e.home); len(h.Active) != 0 || len(h.Archived) != 0 {
		t.Fatalf("no debía haber marcadores: %+v / %+v", h.Active, h.Archived)
	}
	if res.Session == seedSession {
		t.Fatalf("sin transcript que migrar, la sesión debe arrancar de cero con uuid nuevo")
	}
}

// Al terminar bien, los sentinels de la sesión son señales ya consumidas: si se
// quedan en disco nadie los borra nunca (el hook los escribe con o sin
// supervisor) y el directorio crece sin tope.
func TestRunSalidaLimpiaLimpiaLosSentinels(t *testing.T) {
	e := setup(t, seed{fallback: []string{"p2"}, plan: []runStep{{exit: 0}}})

	// Sentinel anterior al arranque: el watcher lo ignora (filtra por `since`),
	// así que no rota — pero es exactamente el que se quedaría para siempre.
	if err := core.WriteSentinel(e.home, core.Sentinel{
		Profile: "p1",
		Session: seedSession,
		Event:   core.LimitEvent{Window: core.WindowSession, Source: "hook"},
		At:      time.Now().Add(-time.Minute),
	}); err != nil {
		t.Fatal(err)
	}

	res, err := Run(context.Background(), e.opts(seedSession))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, quería 0", res.ExitCode)
	}
	left, err := core.ReadSentinels(e.home, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(left) != 0 {
		t.Fatalf("sentinels sin reclamar tras salir bien: %+v", left)
	}
}

// Cancelar el contexto mata al hijo y devuelve el error del contexto sin dejar
// el bucle girando ni procesos huérfanos.
func TestRunContextoCanceladoMataAlHijo(t *testing.T) {
	e := setup(t, seed{
		fallback: []string{"p2"},
		plan:     []runStep{{exit: 0}},
	})
	// Un plan que duerme: se sobreescribe el script por uno que se queda quieto
	// hasta que lo maten, para tener la garantía de que el hijo sigue vivo cuando
	// se cancela el contexto.
	sleeper := filepath.Join(t.TempDir(), "sleeper")
	if err := os.WriteFile(sleeper, []byte("#!/bin/sh\nsleep 30\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	o := e.opts(seedSession)
	o.ClaudeBin = sleeper

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(30 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	res, err := Run(ctx, o)
	if err == nil {
		t.Fatalf("se esperaba el error del contexto; res=%+v", res)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("la cancelación tardó %s: el hijo no murió a tiempo", elapsed)
	}
	if res.ExitCode == 0 {
		t.Fatalf("ExitCode = 0 tras una cancelación: %+v", res)
	}
}

// La política mal configurada falla ANTES de lanzar nada: mejor un error al
// arrancar que descubrirlo a las 3am cuando toque saltar.
func TestRunPolíticaInexistenteNoLanza(t *testing.T) {
	e := setup(t, seed{fallback: []string{"p2"}, plan: []runStep{{exit: 0}}})
	o := e.opts(seedSession)
	o.Policy = "nocturna"

	res, err := Run(context.Background(), o)
	if err == nil {
		t.Fatal("se esperaba error de política inexistente")
	}
	if res.ExitCode != 1 {
		t.Fatalf("ExitCode = %d, quería 1", res.ExitCode)
	}
	if got := e.launches(t); got != 0 {
		t.Fatalf("no debía lanzar nada, lanzó %d", got)
	}
}

// Sin uuid explícito el supervisor pre-asigna uno y lanza con --session-id: sin
// conocerlo de antemano no podría vigilar el transcript ni prestar la sesión.
func TestRunPreAsignaSesion(t *testing.T) {
	e := setup(t, seed{fallback: []string{"p2"}, plan: []runStep{{exit: 0}}})
	o := e.opts("")

	res, err := Run(context.Background(), o)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(res.Session) != 36 {
		t.Fatalf("Session = %q, quería un uuid", res.Session)
	}
	cc := filepath.Join(e.home, "profiles", "p1", "cc-home")
	tr := filepath.Join(core.ProjectDir(cc, core.SlugForCwd(e.cwd)), res.Session+".jsonl")
	if _, err := os.Stat(tr); err != nil {
		t.Fatalf("el hijo no recibió el uuid pre-asignado: %v", err)
	}
}

// --- dos perfiles con el MISMO texto de límite -----------------------------

// El caso normal, no el raro: dos cuentas oficiales que agotan su ventana de 5h
// producen exactamente el mismo mensaje de CC, sin nada que las distinga. Con el
// filtro de duplicados de alcance «corrida entera» que había antes, el segundo
// límite se descartaba por repetido, el bucle caía en `out.exited && limit ==
// nil` y la corrida moría con el exit 1 de claude en vez de aparcar con 75.
//
// Los demás tests de rotación no lo cazaban porque etiquetan el mensaje con el
// perfil (limitStdout("p1") ≠ limitStdout("p2")), que es justo lo que la realidad
// no hace.
func TestRunLimiteIdenticoEnDosPerfilesSigueRotando(t *testing.T) {
	const mismoTexto = `{"type":"api_retry","error":"rate_limit","error_status":429,"message":"You've hit your session limit"}`
	e := setup(t, seed{
		fallback: []string{"p2"},
		maxHops:  4,
		cooldown: "1h",
		plan: []runStep{
			{exit: 1, where: "stdout", emit: mismoTexto},
			{exit: 1, where: "stdout", emit: mismoTexto},
		},
	})

	res, err := Run(context.Background(), e.opts(seedSession))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.ExitCode != ParkedExitCode || !res.Parked {
		t.Fatalf("ExitCode = %d parked = %v, quería 75/true (out=%q)", res.ExitCode, res.Parked, e.out.String())
	}
	if len(res.Hops) != 1 {
		t.Fatalf("hops = %+v, quería 1 (el salto a p2)", res.Hops)
	}
	if got := e.launches(t); got != 2 {
		t.Fatalf("lanzamientos = %d, quería 2", got)
	}
	if tr := e.out.String(); !strings.Contains(tr, "cooldown") {
		t.Fatalf("falta la tabla de cooldowns: %q", tr)
	}
}

// --no-return apaga el regreso a MEDIA sesión, no la limpieza al terminar.
//
// La distinción no es un tecnicismo: si al salir con 0 el préstamo se quedara
// abierto, el usuario acabaría con una conversación terminada viviendo en una
// cuenta ajena, detrás de un marcador que hay que acordarse de cerrar mañana —
// y con su repo resolviendo entretanto al perfil que se la prestó. La forma de
// dejarla ahí a propósito es Ctrl-C (exit 130), que sí conserva el marcador.
func TestRunNoReturnCierraElPrestamoAlTerminar(t *testing.T) {
	e := setup(t, seed{
		fallback: []string{"p2"},
		maxHops:  4,
		cooldown: "1h",
		plan: []runStep{
			{exit: 1, where: "stdout", emit: limitStdout("p1")},
			{exit: 0},
		},
	})
	o := e.opts(seedSession)
	o.NoReturn = true

	res, err := Run(context.Background(), o)
	if err != nil {
		t.Fatalf("Run: %v (err=%q)", err, e.errb.String())
	}
	if res.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, quería 0 (out=%q)", res.ExitCode, e.out.String())
	}
	// El hijo NO se relanza en el primario: el regreso a media sesión sigue apagado.
	if got := e.launches(t); got != 2 {
		t.Fatalf("lanzamientos = %d, quería 2", got)
	}
	// Pero el préstamo se cierra: cero activos y la conversación de vuelta en p1
	// con uuid nuevo.
	h := handoffs(t, e.home)
	if len(h.Active) != 0 {
		t.Fatalf("marcador vivo tras salir con 0: %+v", h.Active)
	}
	if len(h.Archived) != 1 || h.Archived[0].From != "p1" || h.Archived[0].To != "p2" {
		t.Fatalf("archivado inesperado: %+v", h.Archived)
	}
	if res.Profile != "p1" {
		t.Fatalf("Profile = %q, quería p1 (la conversación vuelve a casa)", res.Profile)
	}
	if res.Session == seedSession || res.Session != h.Archived[0].ReturnedAs {
		t.Fatalf("Session = %q, quería el uuid nuevo %q", res.Session, h.Archived[0].ReturnedAs)
	}
}
