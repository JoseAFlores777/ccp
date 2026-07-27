package supervisor

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/JoseAFlores777/ccp/internal/core"
)

// testPoll es el periodo de muestreo de todos los tests. Deliberadamente
// diminuto: la suite entera tiene que correr en segundos, y los sensores por
// polling son quien marca ese ritmo.
const testPoll = 5 * time.Millisecond

// recvEvent espera un evento con tope de tiempo. El tope es generoso (los
// sensores muestrean cada 5ms) porque un test que falla por lentitud del
// runner es peor que uno lento.
func recvEvent(t *testing.T, ch <-chan core.LimitEvent) core.LimitEvent {
	t.Helper()
	select {
	case ev, ok := <-ch:
		if !ok {
			t.Fatalf("el canal de eventos se cerró antes de emitir")
		}
		return ev
	case <-time.After(2 * time.Second):
		t.Fatalf("no llegó ningún evento en 2s")
	}
	return core.LimitEvent{}
}

// expectNoEvent verifica que NO llega nada en `d`. Es el test de las
// invariantes «una sola vez»: sin él, un sensor que re-emite pasaría igual.
func expectNoEvent(t *testing.T, ch <-chan core.LimitEvent, d time.Duration) {
	t.Helper()
	select {
	case ev, ok := <-ch:
		if !ok {
			return // cerrado == no hay más eventos, que es lo que se pide
		}
		t.Fatalf("evento inesperado: %+v", ev)
	case <-time.After(d):
	}
}

// expectClosed comprueba que el canal acaba cerrado, drenando lo que quede.
func expectClosed(t *testing.T, ch <-chan core.LimitEvent) {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for {
		select {
		case _, ok := <-ch:
			if !ok {
				return
			}
		case <-deadline:
			t.Fatalf("el canal de eventos no se cerró")
		}
	}
}

// waitGoroutines espera a que el número de goroutines vuelva al nivel previo.
// Es la comprobación de fugas: los sensores arrancan goroutines propias y
// ninguna puede sobrevivir al cierre.
func waitGoroutines(t *testing.T, base int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		n := runtime.NumGoroutine()
		if n <= base {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("goroutines sin terminar: %d > %d (fuga)", n, base)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// ---------------------------------------------------------------------------
// stream-json
// ---------------------------------------------------------------------------

// líneas de muestra del stream-json. La del medio es el límite; las otras son
// tráfico normal que el detector debe copiar sin reportar nada.
const (
	streamLimitLine  = `{"type":"api_retry","error":{"type":"rate_limit","message":"You've hit your weekly limit"},"resets_at":1753500000}`
	streamNormalLine = `{"type":"assistant","message":{"content":[{"type":"text","text":"hola"}]}}`
	streamNoiseLine  = `no soy json`
)

func TestStreamDetectorTeeIsByteExactAndEmitsOnce(t *testing.T) {
	base := runtime.NumGoroutine()

	// A propósito: CRLF en una línea, una línea vacía, basura no-JSON y una
	// última línea SIN salto final. Todo eso tiene que llegar al tee idéntico.
	input := streamNormalLine + "\n" +
		streamNoiseLine + "\r\n" +
		"\n" +
		streamLimitLine + "\n" +
		streamNormalLine // sin \n final

	var tee bytes.Buffer
	d := NewStreamDetector(strings.NewReader(input), &tee)

	ev := recvEvent(t, d.Events())
	if ev.Source != "stream-json" {
		t.Errorf("Source = %q, quería stream-json", ev.Source)
	}
	if ev.Window != core.WindowWeekly {
		t.Errorf("Window = %q, quería weekly", ev.Window)
	}
	if ev.ResetsAt.IsZero() {
		t.Errorf("ResetsAt vacío; la línea traía resets_at")
	}

	// El canal se cierra al agotarse el reader: es la señal de «el hijo terminó».
	expectClosed(t, d.Events())

	if got := tee.String(); got != input {
		t.Errorf("el tee no es byte a byte:\n got: %q\nwant: %q", got, input)
	}
	if err := d.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
	waitGoroutines(t, base)
}

func TestStreamDetectorGarbageDoesNotPanicNorEmit(t *testing.T) {
	input := "no json\n{}\n{\"type\":\"api_retry\"}\n[]\n{\"error\":\"overloaded\"}\n{roto\n"
	var tee bytes.Buffer
	d := NewStreamDetector(strings.NewReader(input), &tee)
	defer d.Close()

	// `api_retry` sin evidencia de límite y `overloaded` NO son rate limit: rotar
	// ahí movería la conversación del usuario por un fallo que se arregla solo.
	// El cierre del canal es además la barrera que hace seguro leer el tee.
	expectClosed(t, d.Events())
	if tee.String() != input {
		t.Errorf("el tee alteró la salida: %q", tee.String())
	}
}

func TestStreamDetectorNilTeeAndNilReader(t *testing.T) {
	d := NewStreamDetector(strings.NewReader(streamLimitLine+"\n"), nil)
	if ev := recvEvent(t, d.Events()); ev.Window != core.WindowWeekly {
		t.Errorf("Window = %q", ev.Window)
	}
	expectClosed(t, d.Events())

	// Un reader nil no debe panicar: cierra sin emitir.
	d2 := NewStreamDetector(nil, nil)
	expectClosed(t, d2.Events())
}

// ---------------------------------------------------------------------------
// transcript
// ---------------------------------------------------------------------------

const transcriptLimitLine = `{"type":"system","isApiErrorMessage":true,"apiErrorStatus":429,"message":"You've hit your session limit"}`

// transcriptLimitLine2 es OTRO límite, distinguible del anterior por la ventana:
// hace falta para separar «lo que ya estaba en el archivo» de «lo que se escribió
// después» sin depender del orden de llegada.
const transcriptLimitLine2 = `{"type":"system","isApiErrorMessage":true,"apiErrorStatus":429,"message":"You've hit your weekly limit"}`

// appendLine añade texto crudo al archivo (sin salto: lo pone el llamador).
func appendRaw(t *testing.T, path, s string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	if _, err := f.WriteString(s); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close %s: %v", path, err)
	}
}

func TestTranscriptWatcherGrowsBetweenPolls(t *testing.T) {
	base := runtime.NumGoroutine()
	path := filepath.Join(t.TempDir(), "sesion.jsonl")
	appendRaw(t, path, streamNormalLine+"\n")

	d := NewTranscriptWatcher(path, 0, testPoll)

	// Todavía no hay límite en el archivo.
	expectNoEvent(t, d.Events(), 50*time.Millisecond)

	appendRaw(t, path, transcriptLimitLine+"\n")
	ev := recvEvent(t, d.Events())
	if ev.Source != "transcript" {
		t.Errorf("Source = %q, quería transcript", ev.Source)
	}
	if ev.Window != core.WindowSession {
		t.Errorf("Window = %q, quería session", ev.Window)
	}

	// No re-emite lo ya visto por mucho que siga puliendo el archivo.
	expectNoEvent(t, d.Events(), 100*time.Millisecond)

	// Línea a medio escribir: no se consume (parsearla daría falso negativo y
	// avanzar el offset la perdería para siempre).
	appendRaw(t, path, transcriptLimitLine[:40])
	expectNoEvent(t, d.Events(), 50*time.Millisecond)

	// Al completarla sí se emite: la línea no se perdió.
	appendRaw(t, path, transcriptLimitLine[40:]+"\n")
	if ev := recvEvent(t, d.Events()); ev.Window != core.WindowSession {
		t.Errorf("segunda detección: Window = %q", ev.Window)
	}

	if err := d.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
	expectClosed(t, d.Events())
	waitGoroutines(t, base)
}

func TestTranscriptWatcherFileAppearsLater(t *testing.T) {
	base := runtime.NumGoroutine()
	path := filepath.Join(t.TempDir(), "todavia-no.jsonl")

	// El supervisor arranca el watcher ANTES de lanzar a claude, así que el
	// jsonl no existe aún. Eso no puede ser un error terminal.
	d := NewTranscriptWatcher(path, 0, testPoll)
	expectNoEvent(t, d.Events(), 50*time.Millisecond)

	appendRaw(t, path, transcriptLimitLine+"\n")
	if ev := recvEvent(t, d.Events()); ev.Source != "transcript" {
		t.Errorf("Source = %q", ev.Source)
	}

	_ = d.Close()
	expectClosed(t, d.Events())
	waitGoroutines(t, base)
}

// En una sesión REANUDADA (`ccp session --session <uuid>`, o el relanzamiento
// tras un hop) el jsonl ya existe y todo lo que contiene es historia: puede
// llevar el 429 de ayer. Arrancar en el byte 0 lo leería como un límite de AHORA
// y rotaría de perfil por una ventana que ya reabrió.
func TestTranscriptWatcherSkipsPreexistingHistory(t *testing.T) {
	base := runtime.NumGoroutine()
	path := filepath.Join(t.TempDir(), "sesion.jsonl")
	appendRaw(t, path, streamNormalLine+"\n"+transcriptLimitLine+"\n")

	d := NewTranscriptWatcher(path, transcriptBaseline(path), testPoll)
	expectNoEvent(t, d.Events(), 80*time.Millisecond)

	// Lo que se escriba DESPUÉS de la línea base sí es de esta corrida.
	appendRaw(t, path, transcriptLimitLine2+"\n")
	if ev := recvEvent(t, d.Events()); ev.Window != core.WindowWeekly {
		t.Errorf("Window = %q, quería weekly (la línea nueva)", ev.Window)
	}

	// Una reescritura del jsonl no puede reabrir la puerta a la historia
	// descartada: el rebobinado se detiene en la línea base.
	if err := os.WriteFile(path, []byte(transcriptLimitLine+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	expectNoEvent(t, d.Events(), 80*time.Millisecond)

	_ = d.Close()
	expectClosed(t, d.Events())
	waitGoroutines(t, base)
}

// Una línea mayor que el techo del Scanner (8MB) no puede dejar ciego al watcher
// para siempre: se salta y se sigue leyendo lo que venga detrás.
func TestTranscriptWatcherSkipsOversizedLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sesion.jsonl")
	huge := `{"type":"user","blob":"` + strings.Repeat("x", 9*1024*1024) + `"}`
	appendRaw(t, path, huge+"\n")

	d := NewTranscriptWatcher(path, 0, testPoll)
	defer d.Close()

	appendRaw(t, path, transcriptLimitLine+"\n")
	if ev := recvEvent(t, d.Events()); ev.Window != core.WindowSession {
		t.Errorf("Window = %q, quería session (la línea detrás de la gigante)", ev.Window)
	}
}

func TestTranscriptWatcherTruncationRewinds(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sesion.jsonl")
	// Arranca largo para que la reescritura de después sea inequívocamente más
	// corta que el offset alcanzado.
	appendRaw(t, path, strings.Repeat(streamNormalLine+"\n", 8)+transcriptLimitLine+"\n")

	d := NewTranscriptWatcher(path, 0, testPoll)
	defer d.Close()
	recvEvent(t, d.Events())

	// Reescritura completa del jsonl (lo hace `--resume` en algunas versiones):
	// el archivo encoge por debajo del offset y el watcher debe rebobinar en vez
	// de quedarse leyendo más allá del final para siempre.
	if err := os.WriteFile(path, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	appendRaw(t, path, transcriptLimitLine+"\n")
	recvEvent(t, d.Events())
}

// El archivo VACÍO no rebobina. Se interroga a tailOnce directamente porque el
// estado que cubre dura microsegundos —la ventana entre el truncado y la
// escritura de un `os.WriteFile`— y ganarla desde un watcher real es una
// carrera: TestTranscriptWatcherSkipsPreexistingHistory la perdía de vez en
// cuando y emitía la historia descartada como si fuera un límite de ahora.
//
// La razón por la que importa: con tamaño 0, `min64(floor, size)` vale 0 haga lo
// que haga la línea base, así que ese único poll es el que puede tirar el offset
// por debajo del suelo. Un poll de espera lo evita entero.
func TestTailOnceNoRebobinaConArchivoVacio(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sesion.jsonl")
	appendRaw(t, path, streamNormalLine+"\n"+transcriptLimitLine+"\n")
	floor := transcriptBaseline(path)

	d := newBaseDetector()
	offset := floor

	// El truncado a medias: el contenido ya no está, pero la línea base sí manda.
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if !d.tailOnce(path, &offset, floor) {
		t.Fatal("tailOnce pidió terminar con el archivo vacío")
	}
	if offset != floor {
		t.Fatalf("offset = %d, quería %d: un archivo vacío no puede mover la línea base", offset, floor)
	}

	// Y cuando el escritor termina, lo que quedó más corto que el suelo se salta
	// igual: es la historia que la línea base descartó, no una línea de esta
	// corrida.
	if err := os.WriteFile(path, []byte(transcriptLimitLine+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if !d.tailOnce(path, &offset, floor) {
		t.Fatal("tailOnce pidió terminar tras la reescritura")
	}
	expectNoEvent(t, d.Events(), 20*time.Millisecond)
}

// ---------------------------------------------------------------------------
// sentinels
// ---------------------------------------------------------------------------

func TestSentinelWatcherEmitsOncePerSentinel(t *testing.T) {
	base := runtime.NumGoroutine()
	home := t.TempDir()
	since := time.Now().Add(-time.Minute)

	d := NewSentinelWatcher(home, "sesion-A", since, testPoll)

	expectNoEvent(t, d.Events(), 50*time.Millisecond)

	// Sentinel de OTRA sesión: no es nuestro (dos terminales pueden supervisar
	// contra el mismo home) y no debe disparar nada.
	if err := core.WriteSentinel(home, core.Sentinel{
		Profile: "work",
		Session: "sesion-B",
		Event:   core.LimitEvent{Window: core.WindowWeekly, Source: "hook"},
		At:      time.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	expectNoEvent(t, d.Events(), 50*time.Millisecond)

	resets := time.Date(2026, 7, 26, 18, 0, 0, 0, time.UTC)
	if err := core.WriteSentinel(home, core.Sentinel{
		Profile: "work",
		Session: "sesion-A",
		Event:   core.LimitEvent{Window: core.WindowSession, ResetsAt: resets, Source: "hook", Detail: "StopFailure"},
		At:      time.Now(),
	}); err != nil {
		t.Fatal(err)
	}

	ev := recvEvent(t, d.Events())
	if ev.Source != "hook" || ev.Window != core.WindowSession {
		t.Errorf("evento = %+v", ev)
	}
	if !ev.ResetsAt.Equal(resets) {
		t.Errorf("ResetsAt = %v, quería %v", ev.ResetsAt, resets)
	}

	// El sentinel sigue en disco: releerlo no puede volver a emitirlo.
	expectNoEvent(t, d.Events(), 100*time.Millisecond)

	_ = d.Close()
	expectClosed(t, d.Events())
	waitGoroutines(t, base)
}

func TestSentinelWatcherEmptySessionMatchesAll(t *testing.T) {
	home := t.TempDir()
	d := NewSentinelWatcher(home, "", time.Now().Add(-time.Minute), testPoll)
	defer d.Close()

	if err := core.WriteSentinel(home, core.Sentinel{
		Profile: "personal",
		Session: "cualquiera",
		Event:   core.LimitEvent{Window: core.WindowWeekly},
		At:      time.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	// Sin `source` en el sentinel, el watcher lo etiqueta como hook para que la
	// traza del supervisor diga de dónde salió.
	if ev := recvEvent(t, d.Events()); ev.Source != "hook" {
		t.Errorf("Source = %q, quería hook", ev.Source)
	}
}

func TestSentinelWatcherIgnoresOlderThanSince(t *testing.T) {
	home := t.TempDir()
	old := time.Now().Add(-time.Hour)
	if err := core.WriteSentinel(home, core.Sentinel{
		Profile: "work",
		Session: "s1",
		Event:   core.LimitEvent{Window: core.WindowSession},
		At:      old,
	}); err != nil {
		t.Fatal(err)
	}
	// `since` posterior al sentinel: es de una sesión anterior del mismo perfil,
	// reaccionar a él haría rotar nada más arrancar.
	d := NewSentinelWatcher(home, "s1", time.Now().Add(-time.Minute), testPoll)
	defer d.Close()
	expectNoEvent(t, d.Events(), 80*time.Millisecond)
}

// ---------------------------------------------------------------------------
// uso (proactivo)
// ---------------------------------------------------------------------------

func TestUsageWatcherCrossesThresholdOnce(t *testing.T) {
	base := runtime.NumGoroutine()
	home := t.TempDir()
	ccHome := t.TempDir()
	// Ventana VIVA (reset en el futuro): una ya reseteada no cuenta como agotada,
	// y con una fecha fija el test caducaría solo.
	resets := time.Now().Add(2 * time.Hour).UTC().Truncate(time.Second)

	below := core.RateLimits{FiveHour: core.Windowed{UsedPercentage: 40, ResetsAt: resets}}
	if err := core.WriteRateLimits(home, "work", below, time.Now()); err != nil {
		t.Fatal(err)
	}

	d := NewUsageWatcher(home, "work", ccHome, 90, testPoll)
	expectNoEvent(t, d.Events(), 50*time.Millisecond)

	above := core.RateLimits{
		FiveHour: core.Windowed{UsedPercentage: 95, ResetsAt: resets},
		SevenDay: core.Windowed{UsedPercentage: 10, ResetsAt: resets.Add(48 * time.Hour)},
	}
	if err := core.WriteRateLimits(home, "work", above, time.Now()); err != nil {
		t.Fatal(err)
	}

	ev := recvEvent(t, d.Events())
	if ev.Window != core.WindowSession {
		t.Errorf("Window = %q, quería session", ev.Window)
	}
	if ev.Source != "statusline" {
		t.Errorf("Source = %q, quería statusline", ev.Source)
	}
	// El ResetsAt tiene que ser el de la ventana DISPARADA (la de 5h), no el de
	// la semanal: es lo que alimenta el cooldown del Chain.
	if !ev.ResetsAt.Equal(resets) {
		t.Errorf("ResetsAt = %v, quería %v (el de five_hour)", ev.ResetsAt, resets)
	}
	if !strings.Contains(ev.Detail, "95%") {
		t.Errorf("Detail = %q, quería el porcentaje", ev.Detail)
	}

	// El porcentaje sigue alto: sin la memoria por ventana esto emitiría un
	// evento por tick e inundaría el bucle.
	expectNoEvent(t, d.Events(), 150*time.Millisecond)

	// Al bajar del umbral y volver a subir sí se re-arma: la ventana se reseteó.
	if err := core.WriteRateLimits(home, "work", below, time.Now()); err != nil {
		t.Fatal(err)
	}
	expectNoEvent(t, d.Events(), 50*time.Millisecond)
	if err := core.WriteRateLimits(home, "work", above, time.Now()); err != nil {
		t.Fatal(err)
	}
	recvEvent(t, d.Events())

	_ = d.Close()
	expectClosed(t, d.Events())
	waitGoroutines(t, base)
}

func TestUsageWatcherFallsBackToCachedUsage(t *testing.T) {
	home := t.TempDir()
	ccHome := t.TempDir()

	// Sin muestra del statusLine, el respaldo es <ccHome>/.claude.json.
	body := `{"cachedUsageUtilization":{"seven_day":{"utilization":97,"resets_at":"2026-08-01T00:00:00Z"}}}`
	if err := os.WriteFile(filepath.Join(ccHome, ".claude.json"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	d := NewUsageWatcher(home, "work", ccHome, 90, testPoll)
	defer d.Close()

	ev := recvEvent(t, d.Events())
	if ev.Window != core.WindowWeekly {
		t.Errorf("Window = %q, quería weekly", ev.Window)
	}
	if !strings.Contains(ev.Detail, ".claude.json") {
		t.Errorf("Detail = %q, quería la procedencia", ev.Detail)
	}
}

func TestUsageWatcherStaleSampleFallsBack(t *testing.T) {
	home := t.TempDir()
	ccHome := t.TempDir()

	// Muestra vieja y tranquilizadora del statusLine: es de una terminal que ya
	// no corre, así que no debe tapar al respaldo, que sí ve el límite.
	stale := core.RateLimits{FiveHour: core.Windowed{UsedPercentage: 5, ResetsAt: time.Now()}}
	if err := core.WriteRateLimits(home, "work", stale, time.Now().Add(-2*usageSampleTTL)); err != nil {
		t.Fatal(err)
	}
	body := `{"cachedUsageUtilization":{"seven_day":{"utilization":99,"resets_at":"2026-08-01T00:00:00Z"}}}`
	if err := os.WriteFile(filepath.Join(ccHome, ".claude.json"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	d := NewUsageWatcher(home, "work", ccHome, 90, testPoll)
	defer d.Close()
	if ev := recvEvent(t, d.Events()); ev.Window != core.WindowWeekly {
		t.Errorf("Window = %q, quería weekly", ev.Window)
	}
}

// Una muestra al 100% cuya ventana YA reabrió no es un perfil agotado: es un
// dato caducado. Rotar por ella abandona la cuenta correcta —disponible— y la
// destierra el cooldown de respaldo entero, porque el Chain descarta el
// resets_at pasado.
func TestUsageWatcherIgnoraVentanaYaReseteada(t *testing.T) {
	home := t.TempDir()
	ccHome := t.TempDir()

	vencida := core.RateLimits{FiveHour: core.Windowed{
		UsedPercentage: 100,
		ResetsAt:       time.Now().Add(-3 * time.Hour),
	}}
	if err := core.WriteRateLimits(home, "work", vencida, time.Now()); err != nil {
		t.Fatal(err)
	}

	d := NewUsageWatcher(home, "work", ccHome, 90, testPoll)
	defer d.Close()
	expectNoEvent(t, d.Events(), 100*time.Millisecond)

	// La misma ventana pero viva sí dispara: el sensor no se ha quedado mudo.
	viva := core.RateLimits{FiveHour: core.Windowed{
		UsedPercentage: 100,
		ResetsAt:       time.Now().Add(3 * time.Hour),
	}}
	if err := core.WriteRateLimits(home, "work", viva, time.Now()); err != nil {
		t.Fatal(err)
	}
	if ev := recvEvent(t, d.Events()); ev.Window != core.WindowSession {
		t.Errorf("Window = %q, quería session", ev.Window)
	}
}

// El caché de .claude.json no lleva fecha dentro, así que se fecha por el mtime:
// un archivo que CC no toca desde hace horas describe otra ventana (o el día
// anterior) y no puede decidir una rotación.
func TestUsageWatcherIgnoraCacheRancio(t *testing.T) {
	home := t.TempDir()
	ccHome := t.TempDir()

	path := filepath.Join(ccHome, ".claude.json")
	body := `{"cachedUsageUtilization":{"seven_day":{"utilization":99,"resets_at":"2099-01-01T00:00:00Z"}}}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-2 * cachedUsageTTL)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}

	d := NewUsageWatcher(home, "work", ccHome, 90, testPoll)
	defer d.Close()
	expectNoEvent(t, d.Events(), 100*time.Millisecond)

	// Tocarlo (CC vivo escribiendo su caché) lo devuelve al juego.
	now := time.Now()
	if err := os.Chtimes(path, now, now); err != nil {
		t.Fatal(err)
	}
	if ev := recvEvent(t, d.Events()); ev.Window != core.WindowWeekly {
		t.Errorf("Window = %q, quería weekly", ev.Window)
	}
}

func TestUsageWatcherWithoutDataStaysQuiet(t *testing.T) {
	d := NewUsageWatcher(t.TempDir(), "work", t.TempDir(), 0, testPoll)
	defer d.Close()
	// threshold 0 cae al default; sin fuentes no hay nada que reportar.
	expectNoEvent(t, d.Events(), 80*time.Millisecond)
}

// ---------------------------------------------------------------------------
// merge
// ---------------------------------------------------------------------------

// syncBuffer es un io.Writer leíble desde otra goroutine sin carrera: hace falta
// para observar el tee MIENTRAS el detector sigue escribiendo (bytes.Buffer a
// secas dispararía el detector de carreras).
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.Write(p)
}

func (s *syncBuffer) len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.Len()
}

func (s *syncBuffer) string() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.String()
}

// fakeDetector es un Detector controlable por el test.
type fakeDetector struct {
	ch     chan core.LimitEvent
	mu     sync.Mutex
	closes int
	err    error
}

func newFakeDetector() *fakeDetector {
	return &fakeDetector{ch: make(chan core.LimitEvent, 4)}
}

func (f *fakeDetector) Events() <-chan core.LimitEvent { return f.ch }

func (f *fakeDetector) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closes++
	return f.err
}

func (f *fakeDetector) closeCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.closes
}

func TestMergeDetectorsFanInAndClose(t *testing.T) {
	base := runtime.NumGoroutine()
	a, b, c := newFakeDetector(), newFakeDetector(), newFakeDetector()

	// El nil intercalado es el caso real: el supervisor arma la lista de
	// sensores según el modo y en interactive no hay detector de stream-json.
	m := MergeDetectors(a, nil, b, c)

	a.ch <- core.LimitEvent{Source: "transcript"}
	b.ch <- core.LimitEvent{Source: "hook"}
	c.ch <- core.LimitEvent{Source: "statusline"}

	got := map[string]bool{}
	for i := 0; i < 3; i++ {
		got[recvEvent(t, m.Events()).Source] = true
	}
	for _, want := range []string{"transcript", "hook", "statusline"} {
		if !got[want] {
			t.Errorf("falta el evento de %s (got %v)", want, got)
		}
	}

	// El fusionado se cierra solo cuando TODOS los orígenes se han cerrado: con
	// dos cerrados todavía puede llegar un evento del tercero.
	close(a.ch)
	close(b.ch)
	expectNoEvent(t, m.Events(), 50*time.Millisecond)
	close(c.ch)
	expectClosed(t, m.Events())

	if err := m.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
	for i, f := range []*fakeDetector{a, b, c} {
		if n := f.closeCount(); n != 1 {
			t.Errorf("detector %d: Close llamado %d veces, quería 1", i, n)
		}
	}
	waitGoroutines(t, base)
}

func TestMergeCloseIsIdempotentAndUnblocksSources(t *testing.T) {
	base := runtime.NumGoroutine()
	a, b := newFakeDetector(), newFakeDetector()
	m := MergeDetectors(a, b)

	// Cerrar sin que nadie haya leído nada: las goroutines de fan-in tienen que
	// soltar aunque tengan un evento en la mano.
	a.ch <- core.LimitEvent{Source: "transcript"}

	if err := m.Close(); err != nil {
		t.Errorf("primer Close: %v", err)
	}
	if err := m.Close(); err != nil {
		t.Errorf("segundo Close: %v", err)
	}
	// Idempotente de verdad: el segundo Close no vuelve a cerrar a los hijos.
	if n := a.closeCount(); n != 1 {
		t.Errorf("Close en cascada llamado %d veces, quería 1", n)
	}
	waitGoroutines(t, base)
}

func TestMergeCloseAggregatesErrors(t *testing.T) {
	boom := errors.New("boom")
	a := newFakeDetector()
	a.err = boom
	b := newFakeDetector()

	m := MergeDetectors(a, b)
	err := m.Close()
	if !errors.Is(err, boom) {
		t.Errorf("Close = %v, quería envolver %v", err, boom)
	}
	// Un origen que falla al cerrar no puede impedir cerrar a los demás.
	if b.closeCount() != 1 {
		t.Errorf("el segundo origen no se cerró")
	}
}

func TestMergeOfRealDetectorsClosesWhenAllDo(t *testing.T) {
	base := runtime.NumGoroutine()
	home := t.TempDir()
	path := filepath.Join(t.TempDir(), "sesion.jsonl")

	m := MergeDetectors(
		NewStreamDetector(strings.NewReader(streamLimitLine+"\n"), nil),
		NewTranscriptWatcher(path, 0, testPoll),
		NewSentinelWatcher(home, "s1", time.Now().Add(-time.Minute), testPoll),
		NewUsageWatcher(home, "work", t.TempDir(), 90, testPoll),
	)

	if ev := recvEvent(t, m.Events()); ev.Source != "stream-json" {
		t.Errorf("Source = %q", ev.Source)
	}
	if err := m.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
	expectClosed(t, m.Events())
	waitGoroutines(t, base)
}

// ---------------------------------------------------------------------------
// cierre
// ---------------------------------------------------------------------------

func TestEveryDetectorClosesTwiceWithoutPanic(t *testing.T) {
	base := runtime.NumGoroutine()
	home := t.TempDir()
	path := filepath.Join(t.TempDir(), "sesion.jsonl")

	ds := []Detector{
		NewStreamDetector(strings.NewReader(streamNormalLine+"\n"), nil),
		NewTranscriptWatcher(path, 0, testPoll),
		NewSentinelWatcher(home, "s1", time.Now(), testPoll),
		NewUsageWatcher(home, "work", t.TempDir(), 90, testPoll),
		MergeDetectors(newFakeDetector()),
	}
	for i, d := range ds {
		if err := d.Close(); err != nil {
			t.Errorf("detector %d primer Close: %v", i, err)
		}
		if err := d.Close(); err != nil {
			t.Errorf("detector %d segundo Close: %v", i, err)
		}
	}
	// Todos menos el fusionado (que espera a su origen, que sigue abierto)
	// cierran su canal por su cuenta tras el Close.
	for i, d := range ds[:len(ds)-1] {
		t.Logf("comprobando cierre del detector %d", i)
		expectClosed(t, d.Events())
	}
	waitGoroutines(t, base)
}

func TestDetectorsDoNotBlockWithoutConsumer(t *testing.T) {
	// El detector de stream-json también copia la salida visible del usuario: si
	// se bloqueara esperando a que alguien lea sus eventos, congelaría la
	// terminal. Con más eventos que hueco en el buffer, el tee tiene que salir
	// entero igual.
	var sb strings.Builder
	for i := 0; i < eventBuffer*3; i++ {
		sb.WriteString(streamLimitLine + "\n")
	}
	input := sb.String()

	tee := &syncBuffer{}
	d := NewStreamDetector(strings.NewReader(input), tee)
	defer d.Close()

	// Nadie lee d.Events(): aun así el tee se completa.
	deadline := time.Now().Add(2 * time.Second)
	for tee.len() < len(input) && time.Now().Before(deadline) {
		time.Sleep(2 * time.Millisecond)
	}
	if got := tee.string(); got != input {
		t.Fatalf("el tee se quedó a medias (%d de %d bytes): el detector bloqueó", len(got), len(input))
	}
}

func TestNormalizePoll(t *testing.T) {
	cases := []struct {
		in, want time.Duration
	}{
		{0, defaultPoll},
		{-time.Second, defaultPoll},
		{time.Nanosecond, time.Millisecond},
		{testPoll, testPoll},
	}
	for _, c := range cases {
		if got := normalizePoll(c.in); got != c.want {
			t.Errorf("normalizePoll(%v) = %v, quería %v", c.in, got, c.want)
		}
	}
}
