package supervisor

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JoseAFlores777/ccp/internal/core"
)

// return_guard_test.go — los cuatro contratos del regreso proactivo que el resto
// de la suite no cubría.
//
// Todos giran alrededor de la misma asimetría: la rotación por límite mata un
// hijo que YA NO PUEDE trabajar (su perfil devuelve 429), mientras que el regreso
// por `return_check` mata uno que funciona. El primero es forzoso; el segundo es
// discrecional, y por eso necesita guards que el primero no.

// --- D2: el regreso proactivo no interrumpe una sesión viva ------------------

// El fallo que cubre: con los defaults (min_dwell 20m + return_check 10m)
// CUALQUIER préstamo de más de veinte minutos se terminaba en el instante en que
// vencía el cooldown del primario. Con el usuario tecleando, a mitad de un turno,
// o con una tool call en vuelo — que al reanudar se re-ejecuta y puede no ser
// idempotente.
//
// Aquí el hijo prestado está VIVO y escribiendo en su transcript (modo "busy"),
// el reloj cruza el cooldown del primario y el temporizador vence una y otra vez:
// no debe haber tercer lanzamiento. El regreso llega igual, pero por la puerta
// buena — cuando el hijo termina solo.
func TestRunReturnCheckNoInterrumpeUnaSesionActiva(t *testing.T) {
	e := setup(t, seed{
		fallback:    []string{"p2"},
		maxHops:     4,
		cooldown:    "1h",
		returnCheck: "1ms",
		// 30s de silencio exigido: el hijo "busy" toca el transcript cada 50ms,
		// así que la sesión NUNCA llega a estar ociosa durante el test.
		returnIdle: "30s",
		plan: []runStep{
			{exit: 1, where: "stdout", emit: limitStdout("p1")},
			{exit: 0, busy: true},
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
		// El primario lleva dos horas libre. El periodo de `return_check` se mide
		// con el reloj inyectado, así que cada `advance` es UNA oportunidad de
		// volver: se le dan varias seguidas, todas con el hijo escribiendo.
		clk.advance(2 * time.Hour)
		for i := 0; i < 4; i++ {
			time.Sleep(settle)
			if n := e.launches(t); n != 2 {
				t.Errorf("mató una sesión ACTIVA para volver a casa (lanzamientos=%d): %q", n, e.out.String())
				break
			}
			clk.advance(time.Minute)
		}
		// El usuario termina: ahora sí, la vuelta a casa ocurre sin robar nada.
		e.unblock(t)
	}()

	res, err := Run(context.Background(), o)
	<-done
	if err != nil {
		t.Fatalf("Run: %v (err=%q)", err, e.errb.String())
	}
	if res.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, quería 0 (out=%q err=%q)", res.ExitCode, e.out.String(), e.errb.String())
	}
	if got := e.launches(t); got != 2 {
		t.Fatalf("lanzamientos = %d, quería 2: no debía relanzarse nada por el temporizador", got)
	}
	if len(res.Hops) != 1 || res.Hops[0].To != "p2" {
		t.Fatalf("hops = %+v, quería solo el préstamo p1→p2", res.Hops)
	}
	// El préstamo se cierra igual al terminar el hijo: la sesión acaba en casa.
	if res.Profile != "p1" {
		t.Fatalf("terminó en %s, quería p1 (el marcador se cierra al salir bien)", res.Profile)
	}
	if !strings.Contains(e.errb.String(), "la sesión sigue activa") {
		t.Fatalf("no se explicó por qué no se volvió: %q", e.errb.String())
	}
}

// La otra mitad del contrato: el guard aplaza el regreso, no lo cancela. En
// cuanto la sesión calla el tiempo exigido, la vuelta a casa ocurre sola.
func TestRunReturnCheckVuelveCuandoLaSesionQuedaOciosa(t *testing.T) {
	e := setup(t, seed{
		fallback:    []string{"p2"},
		maxHops:     4,
		cooldown:    "1h",
		returnCheck: "1ms",
		// Una hora de silencio exigido, decidida por mtime (ver setIdleAge): el
		// hijo en modo "wait" no toca el transcript, así que su edad es la que el
		// test le ponga y cada oportunidad del temporizador es determinista.
		returnIdle: "1h",
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
		// Primera oportunidad: el primario ya está libre pero la conversación
		// acaba de moverse (transcript recién tocado), así que el guard la aplaza.
		e.setIdleAge(t, "p2", 0)
		clk.advance(2 * time.Hour)
		if !awaitCond(func() bool { return strings.Contains(e.errb.String(), "la sesión sigue activa") }) {
			t.Error("el guard de inactividad no aplazó el primer regreso")
			return
		}
		// La sesión calla: el transcript envejece más allá de `return_idle` y se
		// le da una segunda oportunidad al temporizador. Ahora sí toca volver.
		e.setIdleAge(t, "p2", 2*time.Hour)
		clk.advance(time.Minute)
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
		t.Fatalf("hops = %+v, quería volver a p1 en cuanto la sesión calló", res.Hops)
	}
	if got := e.profiles(t); len(got) != 3 || got[2] != "p1" {
		t.Fatalf("perfiles lanzados = %v, quería [p1 p2 p1]", got)
	}
}

// El guard, aislado: las tres respuestas de sessionIdle y el porqué de cada una.
func TestSessionIdle(t *testing.T) {
	dir := t.TempDir()
	tr := filepath.Join(dir, "s.jsonl")
	if err := os.WriteFile(tr, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(tr)
	if err != nil {
		t.Fatal(err)
	}
	mtime := st.ModTime()

	if sessionIdle(tr, time.Minute, mtime.Add(30*time.Second)) {
		t.Error("30s de silencio con return_idle 1m no es una sesión ociosa")
	}
	if !sessionIdle(tr, time.Minute, mtime.Add(90*time.Second)) {
		t.Error("90s de silencio con return_idle 1m sí es una sesión ociosa")
	}
	// El borde exacto cuenta como ocioso: la ventana ya se cumplió.
	if !sessionIdle(tr, time.Minute, mtime.Add(time.Minute)) {
		t.Error("el borde exacto de return_idle debe contar como ocioso")
	}
	// Sin transcript no sabemos NADA, y no saber no es estar ocioso.
	if sessionIdle(filepath.Join(dir, "no-existe.jsonl"), time.Minute, mtime.Add(time.Hour)) {
		t.Error("un transcript ausente no puede considerarse ocioso")
	}
	// return_idle 0s es el opt-out explícito de la política.
	if !sessionIdle(filepath.Join(dir, "no-existe.jsonl"), 0, mtime) {
		t.Error("return_idle 0s debe desactivar el guard")
	}
}

// --- D4: el guard que ARMA el temporizador, preguntado por su nombre ---------

// Por qué este contrato necesita un test propio y no le vale uno de integración:
// armar el temporizador de más NO tiene efecto observable en casi ningún estado.
// Estando en casa, `Chain.ReturnDue` es false (current == primary), así que
// decideReturn contesta returnNotYet y la corrida sale idéntica byte a byte con
// el ticker armado o sin armar. Por eso el viejo
// `TestRunReturnCheckNoDisparaSinPrestamoVivo` decía cubrir «el temporizador ni
// se arma» y en realidad pasaba en verde con el guard quitado: lo que probaba era
// ReturnDue. Hoy ese test se llama `TestRunEstandoEnCasaNadaSeMueve` (dice lo que
// prueba) y la condición del armado se interroga aquí, directamente.
//
// El único estado donde armar de más SÍ mueve la conversación —préstamo
// degradado, sin marcador que cerrar— lo cubre end-to-end
// `TestRunPrestamoDegradadoNoArmaElTemporizadorDeRegreso`, más abajo.
func TestArmReturnTickerSoloConPrestamoVivo(t *testing.T) {
	const check = 10 * time.Minute
	casos := []struct {
		nombre   string
		onLoan   bool
		noReturn bool
		check    time.Duration
		quiere   bool
	}{
		// El único caso que arma: hay marcador vivo cuyo From es el primario, el
		// regreso está habilitado y la política da un periodo de sondeo.
		{"préstamo vivo y regreso habilitado", true, false, check, true},

		// Sin préstamo vivo no existe camino de vuelta a casa: o ya estamos en
		// casa (nada que devolver) o el préstamo es DEGRADADO (nadie abrió
		// handoff). Armar ahí apunta un SIGTERM a un hijo sano por un movimiento
		// que nadie pidió.
		{"sin préstamo vivo", false, false, check, false},
		// Y ninguna de las otras dos condiciones puede rescatarlo: la ausencia de
		// préstamo manda por sí sola, con el periodo que sea.
		{"sin préstamo, periodo mínimo", false, false, time.Nanosecond, false},

		// --no-return es literalmente «no me devuelvas la sesión».
		{"--no-return", true, true, check, false},

		// return_check: 0s es el opt-out por política; sin periodo no hay sondeo.
		{"return_check 0s", true, false, 0, false},
		// Un periodo negativo solo puede venir de una política corrupta; se trata
		// como desactivado en vez de como «sondea sin parar».
		{"return_check negativo", true, false, -time.Minute, false},
	}
	for _, c := range casos {
		if got := armReturnTicker(c.onLoan, c.noReturn, c.check); got != c.quiere {
			t.Errorf("%s: armReturnTicker(onLoan=%v, noReturn=%v, check=%v) = %v, quería %v",
				c.nombre, c.onLoan, c.noReturn, c.check, got, c.quiere)
		}
	}
}

// --- D5: un límite encolado no se pierde al volver a casa --------------------

// El fallo que cubre: la abstención ante un límite pendiente solo miraba el
// evento YA desencolado. Un LimitEvent que un sensor acababa de poner en el canal
// y que el `select` todavía no había sacado se perdía entero cuando ganaba la
// rama del regreso (Go elige al azar entre canales listos, así que ~50% de las
// veces): el hijo moría, los detectores se cerraban y el agotamiento del perfil
// que dejábamos nunca pasaba por MarkExhausted. El siguiente Next() lo elegiría
// otra vez como préstamo fresco y quemaría un hop en una cuenta que sigue en 429.
//
// El test carga el canal ANTES de preguntar, que es exactamente el estado que la
// carrera produce, y exige que gane el límite.
func TestDecideReturnUnLimiteEncoladoGanaAlRegreso(t *testing.T) {
	r := newReturnRunner(t, "1ns")
	tr := writeTranscript(t, t.TempDir())

	events := make(chan core.LimitEvent, 2)
	events <- core.LimitEvent{Window: core.WindowSession, Source: "sentinel", Detail: "límite en vuelo"}

	act, queued := r.decideReturn(events, nil, tr, t0, time.Now())
	if act != returnRotate {
		t.Fatalf("acción = %v, quería returnRotate: el límite encolado tiene que ganar", act)
	}
	if queued == nil || queued.Detail != "límite en vuelo" {
		t.Fatalf("evento devuelto = %+v, quería el que estaba encolado", queued)
	}
	// Y con el canal vacío el regreso sigue ocurriendo: el drenado no puede
	// convertirse en una excusa permanente para no volver.
	act, queued = r.decideReturn(events, nil, tr, t0, time.Now())
	if act != returnHomeNow || queued != nil {
		t.Fatalf("acción = %v/%+v, quería returnHomeNow con el canal vacío", act, queued)
	}
}

// Las demás reglas de la decisión, cada una en su estado. Es lo que garantiza
// que el orden (límite desencolado → cooldown → min_dwell → inactividad →
// límite encolado → a casa) no se reordene sin querer.
func TestDecideReturnOrdenDeLasReglas(t *testing.T) {
	tr := writeTranscript(t, t.TempDir())
	events := make(chan core.LimitEvent) // vacío y sin cerrar: nunca hay nada encolado

	// (1) un límite ya desencolado gana: la rotación pendiente conserva el
	// agotamiento del perfil, y Next() nos traerá a casa igual.
	r := newReturnRunner(t, "1ns")
	if act, _ := r.decideReturn(events, &core.LimitEvent{}, tr, t0, time.Now()); act != returnNotYet {
		t.Errorf("con un límite en curso la acción fue %v, quería returnNotYet", act)
	}

	// (2) el primario todavía en cooldown: no toca volver.
	r = newReturnRunner(t, "1ns")
	r.chain.MarkExhausted("p1", time.Time{}, t0)
	if act, _ := r.decideReturn(events, nil, tr, t0, time.Now()); act != returnNotYet {
		t.Errorf("con el primario en cooldown la acción fue %v, quería returnNotYet", act)
	}

	// (3) min_dwell sin cumplir: el usuario acaba de llegar al préstamo.
	r = newReturnRunner(t, "1ns")
	r.rc.Policy.MinDwell = time.Hour
	r.chain.policy.MinDwell = time.Hour
	if act, _ := r.decideReturn(events, nil, tr, t0.Add(time.Minute), time.Now()); act != returnWaitDwell {
		t.Errorf("con min_dwell sin cumplir la acción fue %v, quería returnWaitDwell", act)
	}

	// (4) la sesión está viva: es el guard de D2.
	r = newReturnRunner(t, "1h")
	if act, _ := r.decideReturn(events, nil, tr, t0, time.Now()); act != returnWaitBusy {
		t.Errorf("con la sesión activa la acción fue %v, quería returnWaitBusy", act)
	}

	// (5) todo cumplido: a casa.
	r = newReturnRunner(t, "1ns")
	if act, _ := r.decideReturn(events, nil, tr, t0, time.Now()); act != returnHomeNow {
		t.Errorf("con todo cumplido la acción fue %v, quería returnHomeNow", act)
	}
}

// newReturnRunner arma un runner mínimo (cadena p1 → p2, ya prestada a p2) para
// interrogar a decideReturn sin lanzar ningún proceso.
func newReturnRunner(t *testing.T, returnIdle string) *runner {
	t.Helper()
	idle, err := time.ParseDuration(returnIdle)
	if err != nil {
		t.Fatal(err)
	}
	rc := core.ResolvedChain{
		Primary:  "p1",
		Fallback: []string{"p2"},
		Policy: core.EffectivePolicy{
			Name:             "default",
			MaxHops:          4,
			ReturnCheck:      time.Millisecond,
			ReturnIdle:       idle,
			CooldownStrategy: core.CooldownResetsAt,
			CooldownFallback: time.Hour,
		},
	}
	r := &runner{rc: rc, seen: map[string]bool{}, copied: map[string]bool{}}
	r.chain = NewChain(rc, 0, false, t0)
	r.chain.Advance("p2", t0) // la sesión está prestada
	return r
}

// writeTranscript deja un jsonl recién escrito (mtime = ahora) en dir.
func writeTranscript(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "sesion.jsonl")
	if err := os.WriteFile(path, []byte("{\"type\":\"user\"}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// --- D3 / D7: el préstamo DEGRADADO ------------------------------------------

// degradedScript es un `claude` que empieza SIN transcript (el límite llega antes
// del primer turno, que es cuando el sensor proactivo puede disparar con la
// sesión recién abierta) y a partir del segundo lanzamiento sí escribe su jsonl.
//
// Es el único camino que produce un préstamo sin marcador: HandoffChain no llega
// a correr, así que `ccp handoff list` no ve nada — y aun así la conversación
// existe, porque nació DESPUÉS, ya dentro del préstamo.
const degradedScript = `#!/bin/sh
sid=""
prev=""
for a in "$@"; do
  case "$prev" in
    --session-id|--resume) sid="$a" ;;
  esac
  prev="$a"
done

n=0
if [ -f "$FAKE_COUNT" ]; then read -r n < "$FAKE_COUNT"; fi
n=$((n+1))
printf '%s\n' "$n" > "$FAKE_COUNT"
printf '%s %s %s\n' "$n" "$CCP_PROFILE" "$sid" >> "$FAKE_LOG"

if [ "$n" = "1" ]; then
  printf '%s\n' "$FAKE_LIMIT1"
  exit 1
fi

dir="$CLAUDE_CONFIG_DIR/projects/$FAKE_SLUG"
mkdir -p "$dir"
f="$dir/$sid.jsonl"
if [ ! -f "$f" ]; then
  printf '{"type":"user","sessionId":"%s","uuid":"m1","parentUuid":null}\n' "$sid" > "$f"
fi

if [ "$n" = "2" ]; then
  if [ -n "$FAKE_LIMIT2" ]; then printf '%s\n' "$FAKE_LIMIT2"; fi
  if [ "$FAKE_HOLD" = "1" ]; then
    i=0
    while [ "$i" -lt 400 ]; do
      if [ -f "$FAKE_RELEASE" ]; then break; fi
      sleep 0.05
      i=$((i+1))
    done
    exit 0
  fi
  exit 1
fi

exit 0
`

// D3 — el guard `onLoan` del temporizador es load-bearing.
//
// Con un préstamo DEGRADADO no hay marcador vivo, así que no hay préstamo que
// cerrar: el temporizador de regreso NO debe armarse por mucho que el primario
// esté libre. Sin el guard (`onLoan := true`) el reloj arranca al hijo del
// terminal para un movimiento que nadie pidió — y antes de la defensa en
// profundidad de la rama returnHome, además mataba la corrida entera con exit 1
// y un mensaje con el %s vacío ("no se pudo devolver la sesión a : …").
func TestRunPrestamoDegradadoNoArmaElTemporizadorDeRegreso(t *testing.T) {
	e := setup(t, seed{
		fallback:    []string{"p2"},
		maxHops:     4,
		cooldown:    "0s", // el primario vuelve a estar disponible al instante
		returnCheck: "1ms",
		// El guard de inactividad se DESACTIVA a propósito: si no, taparía lo que
		// este test estudia (con un reloj falso congelado, el primer chequeo cae
		// antes de que el transcript exista y ya no hay más chequeos). La única
		// variable aquí es `onLoan`.
		returnIdle: "0s",
		plan:       []runStep{{exit: 0}},
	})
	t.Setenv("FAKE_LIMIT1", limitStdout("p1-sin-transcript"))
	t.Setenv("FAKE_LIMIT2", "")
	t.Setenv("FAKE_HOLD", "1")

	bin := filepath.Join(t.TempDir(), "degradado")
	if err := os.WriteFile(bin, []byte(degradedScript), 0o755); err != nil {
		t.Fatal(err)
	}
	clk := newClock(t0)
	o := e.opts(seedSession)
	o.ClaudeBin = bin
	o.Now = clk.Now

	done := make(chan struct{})
	go func() {
		defer close(done)
		if !awaitCond(func() bool { return e.launches(t) >= 2 }) {
			t.Error("el segundo lanzamiento nunca ocurrió")
			return
		}
		clk.advance(2 * time.Hour)
		time.Sleep(settle)
		if n := e.launches(t); n != 2 {
			t.Errorf("el temporizador se armó sin préstamo vivo (lanzamientos=%d): %q", n, e.out.String())
		}
		e.unblock(t)
	}()

	res, err := Run(context.Background(), o)
	<-done
	if err != nil {
		t.Fatalf("Run: %v (out=%q err=%q)", err, e.out.String(), e.errb.String())
	}
	if res.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, quería 0 (out=%q err=%q)", res.ExitCode, e.out.String(), e.errb.String())
	}
	if got := e.launches(t); got != 2 {
		t.Fatalf("lanzamientos = %d, quería 2", got)
	}
	if len(res.Hops) != 1 || res.Hops[0].To != "p2" {
		t.Fatalf("hops = %+v, quería solo la rotación degradada p1→p2", res.Hops)
	}
	if res.Profile != "p2" {
		t.Fatalf("terminó en %s, quería p2: la conversación se quedó donde estaba", res.Profile)
	}
	if h := handoffs(t, e.home); len(h.Active) != 0 || len(h.Archived) != 0 {
		t.Fatalf("un préstamo degradado no crea marcadores: %+v / %+v", h.Active, h.Archived)
	}
}

// D7 — volver al primario NUNCA puede crear un marcador invertido.
//
// El estado: préstamo degradado (sin marcador) en p2, donde la conversación SÍ
// nace, y luego p2 topa su límite con el primario ya libre. Antes, ese salto caía
// en `default:` y HandoffChain creaba un marcador ACTIVO {From: p2, To: p1} que
// nunca se cierra estando en casa. La consecuencia se veía al terminar: el
// `handoff end` implícito back-sincronizaba la conversación HACIA p2, res.Profile
// decía p2, la traza decía "sesión devuelta a p2" y el transcript acababa en el
// cc-home de p2 — o sea, el usuario tenía que estar en p2 para teclear
// `claude --resume`, justo al revés de donde había trabajado.
func TestRunVueltaDesdePrestamoDegradadoNoCreaMarcadorInvertido(t *testing.T) {
	e := setup(t, seed{
		fallback:    []string{"p2"},
		maxHops:     4,
		cooldown:    "0s", // p1 vuelve a estar disponible en cuanto se marca agotado
		returnCheck: "0s", // sin temporizador: el movimiento lo dispara el límite de p2
		plan:        []runStep{{exit: 0}},
	})
	t.Setenv("FAKE_LIMIT1", limitStdout("p1-sin-transcript"))
	t.Setenv("FAKE_LIMIT2", limitStdout("p2-agotado"))
	t.Setenv("FAKE_HOLD", "0")

	bin := filepath.Join(t.TempDir(), "degradado")
	if err := os.WriteFile(bin, []byte(degradedScript), 0o755); err != nil {
		t.Fatal(err)
	}
	o := e.opts(seedSession)
	o.ClaudeBin = bin

	res, err := Run(context.Background(), o)
	if err != nil {
		t.Fatalf("Run: %v (out=%q err=%q)", err, e.out.String(), e.errb.String())
	}
	if res.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, quería 0 (out=%q err=%q)", res.ExitCode, e.out.String(), e.errb.String())
	}
	if got := e.profiles(t); len(got) != 3 || got[2] != "p1" {
		t.Fatalf("perfiles lanzados = %v, quería [p1 p2 p1]", got)
	}
	// La verdad #1: la conversación acabó en el primario, y eso es lo que reporta.
	if res.Profile != "p1" {
		t.Fatalf("res.Profile = %s, quería p1: la sesión terminó en el primario", res.Profile)
	}
	// La verdad #2: no queda ningún marcador, ni vivo ni archivado. No hubo
	// préstamo registrado, así que no hay préstamo que cerrar ni que historiar.
	h := handoffs(t, e.home)
	if len(h.Active) != 0 {
		t.Fatalf("marcador ACTIVO tras volver a casa: %+v", h.Active)
	}
	if len(h.Archived) != 0 {
		t.Fatalf("archivado inesperado (el préstamo degradado nunca existió): %+v", h.Archived)
	}
	// La verdad #3: el uuid que reporta es el que reanuda la conversación, y el
	// transcript está en el cc-home del primario.
	p1cc := filepath.Join(e.home, "profiles", "p1", "cc-home")
	back := filepath.Join(core.ProjectDir(p1cc, core.SlugForCwd(e.cwd)), res.Session+".jsonl")
	if _, err := os.Stat(back); err != nil {
		t.Fatalf("la conversación no llegó a p1 con el uuid reportado: %v", err)
	}
	// La verdad #4: la traza no puede decir que la sesión se fue a p2.
	tr := e.out.String()
	if strings.Contains(tr, "devuelta a p2") {
		t.Fatalf("la traza manda al usuario al perfil equivocado: %q", tr)
	}
	if !strings.Contains(tr, "sin marcador de préstamo") || !strings.Contains(tr, "volviendo a p1") {
		t.Fatalf("la traza no explica la vuelta a casa: %q", tr)
	}
}
