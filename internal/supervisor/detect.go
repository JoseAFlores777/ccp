package supervisor

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/JoseAFlores777/ccp/internal/core"
)

// detect.go — los sensores del auto-handoff.
//
// Un Detector es una fuente de `core.LimitEvent` con dos garantías que el bucle
// del supervisor da por hechas:
//
//  1. su canal se CIERRA cuando la fuente se agota (para el detector de
//     stream-json ese cierre es además la señal de «el hijo terminó su salida»,
//     que es como el bucle principal se entera del fin sin esperar al Wait);
//  2. `Close` es idempotente y nunca bloquea, aunque nadie esté leyendo el canal.
//
// Los cuatro sensores existen porque NINGUNO cubre los dos modos de ejecución:
// el stream-json solo existe en headless (`claude -p`), el transcript es lo
// único reactivo que funciona en interactive, el sentinel depende de que el
// perfil tenga la capa de hooks instalada, y el de uso es el único PROACTIVO
// (avisa ANTES de que el turno falle). Se combinan con MergeDetectors según el
// modo, y la redundancia es deliberada: es preferible detectar dos veces el
// mismo límite (el Chain deduplica por perfil) que quedarse ciego.

// Detector emite eventos de límite hasta que se cierra.
type Detector interface {
	Events() <-chan core.LimitEvent
	Close() error
}

// eventBuffer es la holgura del canal de cada sensor. Existe para que un pico de
// eventos (un turno que reintenta varias veces seguidas) no acople la velocidad
// del sensor a la del bucle principal, que entre hop y hop se pasa segundos
// terminando el hijo y reescribiendo marcadores.
const eventBuffer = 16

// defaultPoll es el periodo de muestreo cuando el llamador no fija uno. Dos
// segundos es el compromiso entre reaccionar rápido a un límite y no releer el
// jsonl del transcript (que puede pesar megas) en bucle cerrado.
const defaultPoll = 2 * time.Second

// usageSampleTTL es la antigüedad a partir de la cual una muestra del statusLine
// se considera rancia. El statusLine se refresca cada pocos segundos mientras CC
// está vivo, así que una muestra de hace más de cinco minutos suele significar
// que ESA terminal ya no está corriendo: fiarse de ella sería decidir una
// rotación con datos de otra sesión.
const usageSampleTTL = 5 * time.Minute

// cachedUsageTTL es la antigüedad a partir de la cual el caché de
// <ccHome>/.claude.json deja de ser fuente. El respaldo existe para dos casos —
// perfil sin la capa de sensores, y CC recién arrancado— y en los dos CC está
// CORRIENDO, así que su .claude.json se refresca. Un archivo de hace horas
// significa lo contrario: ese perfil no está en marcha y su última medida es de
// otra ventana (o de otro día).
const cachedUsageTTL = time.Hour

// normalizePoll acota el periodo: un valor no positivo cae al default y uno
// absurdamente pequeño se eleva a 1ms para no convertir el ticker en un bucle
// de espera activa. Los tests bajan el poll a decenas de milisegundos, así que
// el mínimo tiene que ser realmente bajo.
func normalizePoll(d time.Duration) time.Duration {
	switch {
	case d <= 0:
		return defaultPoll
	case d < time.Millisecond:
		return time.Millisecond
	}
	return d
}

// baseDetector es la fontanería común: el canal de salida, el canal de cierre y
// el `sync.Once` que hace idempotente a Close.
//
// El contrato interno es estricto: la goroutine del sensor es la ÚNICA que
// cierra `events` (por eso siempre lo hace con un `defer`), y `Close` solo cierra
// `quit`. Si Close cerrase `events` habría carrera de doble cierre —  panic — en
// cuanto el sensor terminase por su cuenta a la vez que el usuario llama Close.
type baseDetector struct {
	events chan core.LimitEvent
	quit   chan struct{}
	once   sync.Once
}

func newBaseDetector() *baseDetector {
	return &baseDetector{
		events: make(chan core.LimitEvent, eventBuffer),
		quit:   make(chan struct{}),
	}
}

func (b *baseDetector) Events() <-chan core.LimitEvent { return b.events }

// Close señala el cierre y vuelve inmediatamente. No espera a la goroutine a
// propósito: un sensor puede estar bloqueado en un `Read` de un pipe que solo se
// desbloquea cuando muere el hijo, y el supervisor llama a Close JUSTO antes de
// matarlo. Esperar aquí sería un abrazo mortal.
func (b *baseDetector) Close() error {
	b.once.Do(func() { close(b.quit) })
	return nil
}

// emit entrega el evento aunque haya que esperar a que el consumidor lo recoja,
// pero se rinde si el detector se está cerrando. Es el modo de los sensores por
// polling: perder un evento sería perder la única señal de que hay que rotar, y
// la contrapresión aquí solo ralentiza un muestreo, no la salida del usuario.
func (b *baseDetector) emit(ev core.LimitEvent) bool {
	select {
	case b.events <- ev:
		return true
	case <-b.quit:
		return false
	}
}

// offer intenta entregar sin bloquear NUNCA. Es el modo del detector de
// stream-json: ahí la goroutine también es la que copia la salida del hijo a la
// terminal del usuario, así que bloquearse esperando al bucle principal
// congelaría lo que el usuario está viendo. Si el buffer está lleno el evento se
// descarta, y es aceptable: el bucle ya tiene en cola una señal de límite del
// mismo turno y los duplicados no cambian la decisión.
func (b *baseDetector) offer(ev core.LimitEvent) bool {
	select {
	case b.events <- ev:
		return true
	case <-b.quit:
		return false
	default:
		return false
	}
}

// sleep espera d o el cierre, lo que ocurra antes. Devuelve false si hay que
// terminar: así los bucles de polling se escriben como `for { … if !sleep() {
// return } }` y responden a Close en el acto en vez de al final del periodo.
func (b *baseDetector) sleep(d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return true
	case <-b.quit:
		return false
	}
}

// ---------------------------------------------------------------------------
// stream-json (headless)
// ---------------------------------------------------------------------------

// scanLinesKeepEnding es un bufio.SplitFunc que devuelve la línea CON su
// terminador.
//
// bufio.ScanLines no vale aquí: se come el "\n" y además recorta un "\r" final.
// Este detector no solo parsea, también COPIA la salida a la terminal del
// usuario, y esa copia tiene que ser byte a byte — si el token perdiera el
// terminador habría que re-sintetizarlo, y con CRLF (o con una última línea sin
// salto) reconstruiríamos algo distinto de lo que emitió el hijo.
func scanLinesKeepEnding(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if atEOF && len(data) == 0 {
		return 0, nil, nil
	}
	if i := bytes.IndexByte(data, '\n'); i >= 0 {
		return i + 1, data[:i+1], nil
	}
	if atEOF {
		return len(data), data, nil
	}
	return 0, nil, nil // pide más datos
}

// NewStreamDetector parsea el stream-json de `claude -p` mientras lo copia tal
// cual a `tee` (el usuario sigue viendo la salida íntegra).
//
// El orden importa: se escribe a `tee` ANTES de parsear, de modo que un fallo o
// una lentitud del parseo no pueda reordenar ni retener la salida visible.
//
// Al agotarse el reader se cierra el canal de eventos, y ese cierre es la señal
// que usa el bucle principal para saber que el hijo terminó de escribir.
func NewStreamDetector(r io.Reader, tee io.Writer) Detector {
	d := newBaseDetector()
	go func() {
		defer close(d.events)
		if r == nil {
			return
		}
		sc := bufio.NewScanner(r)
		// El stream-json mete el turno entero (con los resultados de tools) en
		// una sola línea: 64KB de arranque y 8MB de techo, igual que el lector
		// de transcripts de internal/core.
		sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
		sc.Split(scanLinesKeepEnding)
		for sc.Scan() {
			raw := sc.Bytes()
			if tee != nil {
				// Errores de escritura ignorados a propósito: si la terminal se
				// cerró, seguir parseando para detectar el límite sigue siendo
				// útil; abortar aquí dejaría al supervisor sin sensor.
				_, _ = tee.Write(raw)
			}
			line := bytes.TrimRight(raw, "\r\n")
			if len(line) == 0 {
				continue
			}
			if ev, ok := core.ParseStreamJSONLine(line); ok {
				d.offer(ev)
			}
		}
		// Si el Scanner se rinde (línea > 8MB, o error de lectura) el resto de
		// la salida se vuelca en crudo: el parseo de ese trozo ya es imposible,
		// pero tragarse la salida del hijo sí sería visible y confuso para el
		// usuario. Es best-effort: lo que quedó dentro del buffer del Scanner
		// se pierde irremediablemente.
		if sc.Err() != nil && tee != nil {
			_, _ = io.Copy(tee, r)
		}
	}()
	return d
}

// ---------------------------------------------------------------------------
// transcript (interactive y headless)
// ---------------------------------------------------------------------------

// NewTranscriptWatcher hace tail del jsonl de la sesión: es el único sensor
// reactivo que funciona en interactive.
//
// Dos invariantes por fiabilidad, porque en interactive no hay nada más:
//   - no perder líneas: solo se consume hasta el último "\n" visto, y la cola
//     parcial (CC escribe el jsonl a trozos) se relee en el siguiente poll;
//   - no re-emitir: el offset solo avanza sobre bytes ya procesados.
//
// `startOffset` es la LÍNEA BASE: los bytes anteriores se dan por historia y no
// se parsean nunca. Hace falta porque en una sesión reanudada (`ccp session
// --session <uuid>`, o el relanzamiento tras un hop) el jsonl ya existe y puede
// arrastrar el 429 de ayer; leerlo desde el byte 0 lo reportaría como un límite
// de AHORA y rotaría de perfil por una ventana que reabrió hace horas. El
// llamador la calcula con transcriptBaseline ANTES de lanzar al hijo, que es el
// único instante en que «lo que hay» es inequívocamente pasado.
//
// Que el archivo aún no exista es normal, no un error: CC lo crea al primer
// turno, y el supervisor arranca el watcher antes de lanzar el hijo.
func NewTranscriptWatcher(path string, startOffset int64, poll time.Duration) Detector {
	d := newBaseDetector()
	p := normalizePoll(poll)
	if startOffset < 0 {
		startOffset = 0
	}
	go func() {
		defer close(d.events)
		offset := startOffset
		for {
			if !d.tailOnce(path, &offset, startOffset) {
				return
			}
			if !d.sleep(p) {
				return
			}
		}
	}()
	return d
}

// transcriptBaseline es el tamaño actual del transcript, o 0 si todavía no
// existe. Es lo que el supervisor pasa como línea base al reanudar.
func transcriptBaseline(path string) int64 {
	st, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return st.Size()
}

// tailOnce lee lo nuevo del transcript y emite lo que sea un límite. Devuelve
// false si hay que terminar (cierre pedido durante la entrega de un evento).
//
// `floor` es la línea base: el rebobinado por truncado nunca baja de ahí, porque
// volver a 0 resucitaría justo la historia que la línea base descartó.
func (d *baseDetector) tailOnce(path string, offset *int64, floor int64) bool {
	f, err := os.Open(path)
	if err != nil {
		// Ausente todavía, o sin permisos: se reintenta en el siguiente poll.
		// Un watcher que aborta aquí dejaría la sesión sin detección para
		// siempre por un fallo transitorio.
		return true
	}
	defer f.Close()

	st, err := f.Stat()
	if err != nil {
		return true
	}
	// Un archivo de tamaño CERO es casi siempre un estado transitorio: quien
	// reescribe el jsonl sin renombrar (un `os.WriteFile`, un `>` de shell)
	// trunca primero y escribe después, y el poll puede caer justo en medio. Si
	// se rebobinara ahí, el offset bajaría a 0 —por debajo de la línea base, que
	// es lo único que `min64` no puede evitar cuando el tamaño es 0— y el
	// contenido que llegue un instante después se leería entero desde el
	// principio, resucitando la historia que la línea base había descartado.
	// Esperar un poll cuesta milisegundos y deja el rebobinado midiendo contra el
	// tamaño real: así una reescritura no atómica se comporta igual que una
	// atómica (tmp+rename), que nunca expone el estado vacío.
	if st.Size() == 0 {
		return true
	}
	// Archivo más corto que el offset ⇒ lo truncaron o lo reemplazaron (un
	// `--resume` que reescribe el jsonl). Se rebobina: es preferible re-procesar
	// líneas viejas —el bucle deduplica por perfil— a quedarse leyendo más allá
	// del final y no ver nada nunca más. Nunca por debajo de la línea base ni del
	// tamaño actual: lo primero re-leería la historia descartada a propósito, lo
	// segundo dejaría el offset más allá del final otra vez.
	if st.Size() < *offset {
		*offset = min64(floor, st.Size())
	}
	if st.Size() == *offset {
		return true
	}
	if _, err := f.Seek(*offset, io.SeekStart); err != nil {
		return true
	}

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	sc.Split(scanLinesKeepEnding)
	for sc.Scan() {
		raw := sc.Bytes()
		if !bytes.HasSuffix(raw, []byte("\n")) {
			// Línea a medio escribir: NO se consume ni se avanza el offset, se
			// relee entera en el siguiente poll. Parsearla ahora daría JSON
			// inválido (falso negativo) y avanzar el offset la perdería.
			break
		}
		*offset += int64(len(raw))
		line := bytes.TrimRight(raw, "\r\n")
		if len(line) == 0 {
			continue
		}
		if ev, ok := core.ParseTranscriptLine(line); ok {
			if !d.emit(ev) {
				return false
			}
		}
	}

	// Una línea por encima del techo del Scanner (8MB: un tool_result enorme, un
	// par de imágenes en base64) hace que Scan devuelva false SIN entregar token,
	// así que el offset no avanza sobre ella. Sin este rescate el watcher releería
	// esa misma línea en cada poll para siempre y ninguna línea posterior —incluido
	// el 429 que motiva la rotación— se parsearía jamás. Se salta a mano y se
	// sigue; cualquier otro error de lectura se reintenta en el siguiente poll.
	if errors.Is(sc.Err(), bufio.ErrTooLong) {
		skipOversizedLine(f, offset)
	}
	return true
}

// skipOversizedLine avanza `offset` más allá del siguiente '\n' leyendo en
// crudo. Si no encuentra el salto (la línea gigante todavía se está escribiendo)
// deja el offset intacto: se reintenta cuando la línea esté completa.
func skipOversizedLine(f *os.File, offset *int64) {
	if _, err := f.Seek(*offset, io.SeekStart); err != nil {
		return
	}
	br := bufio.NewReaderSize(f, 64*1024)
	var n int64
	for {
		chunk, err := br.ReadSlice('\n')
		n += int64(len(chunk))
		if err == nil {
			*offset += n
			return
		}
		if errors.Is(err, bufio.ErrBufferFull) {
			continue
		}
		return // EOF sin '\n'
	}
}

// min64 existe porque el `min` genérico de Go 1.21 no se usa en el resto del
// repo y mezclar estilos aquí no aporta.
func min64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

// ---------------------------------------------------------------------------
// sentinels (hook StopFailure)
// ---------------------------------------------------------------------------

// NewSentinelWatcher vigila los sentinels que deja el hook StopFailure.
//
// `since` acota la ventana temporal (los sentinels de sesiones anteriores no son
// nuestros) y `session` la acota por identidad: dos terminales pueden estar
// supervisando a la vez contra el mismo home, y rotar por el límite de la sesión
// del vecino sería mover la conversación de otro. Con session == "" no se filtra
// (útil para `ccp auto test`).
//
// La deduplicación va por instante + perfil + sesión, no por nombre de archivo:
// el nombre lleva el valor saneado y truncado (ver core.WriteSentinel) y dos
// sentinels distintos pueden compartirlo.
func NewSentinelWatcher(home, session string, since time.Time, poll time.Duration) Detector {
	d := newBaseDetector()
	p := normalizePoll(poll)
	go func() {
		defer close(d.events)
		seen := map[string]bool{}
		for {
			sentinels, err := core.ReadSentinels(home, since)
			if err != nil {
				// Directorio ilegible: se reintenta. Igual que el transcript, un
				// sensor que se rinde es peor que uno que insiste.
				sentinels = nil
			}
			for _, s := range sentinels {
				if session != "" && s.Session != session {
					continue
				}
				key := s.At.UTC().Format(time.RFC3339Nano) + "\x00" + s.Profile + "\x00" + s.Session
				if seen[key] {
					continue
				}
				seen[key] = true
				ev := s.Event
				// El hook es quien lo escribió; si se quedó sin `source` se
				// etiqueta aquí para que la traza diga de dónde vino.
				if ev.Source == "" {
					ev.Source = "hook"
				}
				if ev.Window == "" {
					ev.Window = core.WindowUnknown
				}
				if !d.emit(ev) {
					return
				}
			}
			if !d.sleep(p) {
				return
			}
		}
	}()
	return d
}

// ---------------------------------------------------------------------------
// uso (proactivo)
// ---------------------------------------------------------------------------

// NewUsageWatcher es el sensor PROACTIVO: muestrea la última lectura del
// statusLine (y .claude.json como respaldo) y emite cuando supera el umbral.
//
// Es el único que puede rotar ANTES de que un turno falle, que es la diferencia
// entre «el usuario pierde el turno y hay que reintentarlo» y «el usuario ni se
// entera». A cambio es el menos fiable: depende de un porcentaje que CC publica
// con retraso, por eso el resto de sensores reactivos siguen activos.
//
// Emite UNA sola vez por ventana cruzada. Sin esa memoria emitiría un evento por
// tick mientras el porcentaje siguiera alto — decenas de eventos por minuto que
// inundarían el bucle. El disparo se re-arma cuando el uso vuelve a caer por
// debajo del umbral (la ventana se reseteó).
func NewUsageWatcher(home, profile, ccHome string, threshold int, poll time.Duration) Detector {
	d := newBaseDetector()
	p := normalizePoll(poll)
	if threshold <= 0 || threshold > 100 {
		threshold = core.DefaultAutoThreshold
	}
	go func() {
		defer close(d.events)
		fired := map[core.LimitWindow]bool{}
		for {
			rl, from, ok := sampleUsage(home, profile, ccHome, time.Now())
			if ok {
				if win, hit := rl.ExhaustedAt(threshold, time.Now()); hit && usageDatable(rl, win) {
					if !fired[win] {
						fired[win] = true
						if !d.emit(usageEvent(rl, win, threshold, from)) {
							return
						}
					}
				} else if len(fired) > 0 {
					// Nada por encima del umbral ⇒ las ventanas se resetearon;
					// se re-arma el disparo para poder avisar del siguiente ciclo.
					fired = map[core.LimitWindow]bool{}
				}
			}
			if !d.sleep(p) {
				return
			}
		}
	}()
	return d
}

// sampleUsage escoge la mejor fuente disponible: la muestra que el statusLine
// dejó en el estado del perfil y, si no hay o está rancia, el caché que CC
// escribe en <ccHome>/.claude.json.
//
// El orden no es arbitrario: la muestra del statusLine viene del propio proceso
// de CC en vivo y trae los dos porcentajes; el caché de .claude.json arrastra un
// bug conocido (five_hour a 0 con seven_day poblado), por eso es el respaldo y
// no la fuente primaria.
//
// Las DOS fuentes caducan. El caché no lleva su propio `sampled_at`, así que su
// antigüedad se mide por el mtime del archivo: si CC no lo ha tocado en
// cachedUsageTTL es que ese perfil no está corriendo, y su última medida describe
// otro día. Aceptarla con cualquier antigüedad es lo que hacía que un
// `.claude.json` de ayer al 100% matara la sesión y desterrara una cuenta con la
// ventana ya reseteada.
func sampleUsage(home, profile, ccHome string, now time.Time) (core.RateLimits, string, bool) {
	if rl, sampled, ok := core.ReadRateLimits(home, profile); ok {
		if sampled.IsZero() || now.Sub(sampled) <= usageSampleTTL {
			return rl, "statusLine", true
		}
	}
	if cachedUsageIsFresh(ccHome, now) {
		if rl, ok := core.ReadCachedUsage(ccHome); ok {
			return rl, ".claude.json", true
		}
	}
	return core.RateLimits{}, "", false
}

// cachedUsageIsFresh reporta si <ccHome>/.claude.json se escribió hace poco. Un archivo que
// no se deja estatear se trata como ausente (no como fresco): ante la duda, el
// sensor prefiere quedarse ciego a rotar con datos que no puede fechar.
func cachedUsageIsFresh(ccHome string, now time.Time) bool {
	if strings.TrimSpace(ccHome) == "" {
		return false
	}
	st, err := os.Stat(filepath.Join(ccHome, ".claude.json"))
	if err != nil {
		return false
	}
	// Un mtime del futuro (reloj movido, copia de archivos) cuenta como reciente:
	// descartarlo dejaría al sensor sin fuente por un desajuste de reloj.
	age := now.Sub(st.ModTime())
	return age <= cachedUsageTTL
}

// usageDatable exige que la ventana que disparó traiga `resets_at`. Es la única
// regla que este sensor añade sobre ExhaustedAt, y va aquí y no allí a propósito.
//
// ExhaustedAt la comparten dos oficios. PINTANDO —la barra de estado, `ccp auto
// status`— «95% sin fecha» es información legítima y esconderla sería peor.
// DECIDIENDO no: sin `resets_at` el Chain no tiene ventana que esperar y aplica
// el cooldown de respaldo, así que un `.claude.json` con `usedPercentage: 95` y
// sin fecha —el defecto conocido de CC que documenta sampleUsage— destierra el
// perfil una hora entera por una medida que quizá describa una ventana ya
// cerrada. Y esa rotación es además la degradada: sin dato que fechar, tampoco
// hay nada que confirme que el límite sigue vigente.
//
// El sensor reactivo no necesita este filtro: allí el 429 ya ocurrió.
func usageDatable(rl core.RateLimits, win core.LimitWindow) bool {
	w := rl.FiveHour
	if win == core.WindowWeekly {
		w = rl.SevenDay
	}
	return !w.ResetsAt.IsZero()
}

// usageEvent arma el evento del sensor proactivo.
//
// El ResetsAt sale de la ventana que disparó, no de la otra: es exactamente lo
// que alimenta el cooldown del Chain, y confundir el reset de la semanal con el
// de la sesión mandaría un perfil a la nevera durante días.
func usageEvent(rl core.RateLimits, win core.LimitWindow, threshold int, from string) core.LimitEvent {
	w := rl.FiveHour
	if win == core.WindowWeekly {
		w = rl.SevenDay
	}
	return core.LimitEvent{
		Window:   win,
		ResetsAt: w.ResetsAt,
		// Se queda dentro del vocabulario fijo de Source (ver core.LimitEvent):
		// la procedencia fina va en Detail.
		Source: "statusline",
		Detail: fmt.Sprintf("uso %.0f%% ≥ umbral %d%% (ventana %s, vía %s)", w.UsedPercentage, threshold, win, from),
	}
}

// ---------------------------------------------------------------------------
// fan-in
// ---------------------------------------------------------------------------

// mergedDetector multiplexa varios detectores en uno.
type mergedDetector struct {
	events chan core.LimitEvent
	quit   chan struct{}
	once   sync.Once
	srcs   []Detector
}

// MergeDetectors multiplexa varios detectores en uno.
//
// El canal de salida se cierra cuando TODOS los de entrada se han cerrado, que
// es lo que permite al supervisor tratar el conjunto de sensores como uno solo.
//
// Ojo con la consecuencia: si un sensor no cierra su canal (el de stream-json
// sigue bloqueado en el pipe hasta que el hijo muere), el fusionado tampoco
// cierra. Es lo correcto —todavía puede llegar un evento— pero significa que el
// bucle principal no debe esperar a este cierre para terminar: el que manda es
// el Wait del hijo.
func MergeDetectors(ds ...Detector) Detector {
	m := &mergedDetector{
		events: make(chan core.LimitEvent, eventBuffer),
		quit:   make(chan struct{}),
		srcs:   ds,
	}
	var wg sync.WaitGroup
	for _, d := range ds {
		if d == nil {
			continue // llamador cómodo: `MergeDetectors(a, nilSiHeadless, c)`
		}
		wg.Add(1)
		go func(src Detector) {
			defer wg.Done()
			in := src.Events()
			for {
				select {
				case ev, ok := <-in:
					if !ok {
						return
					}
					select {
					case m.events <- ev:
					case <-m.quit:
						return
					}
				case <-m.quit:
					return
				}
			}
		}(d)
	}
	go func() {
		wg.Wait()
		close(m.events)
	}()
	return m
}

func (m *mergedDetector) Events() <-chan core.LimitEvent { return m.events }

// Close cierra el fusionado y, en cascada, todos sus orígenes. Es idempotente
// (el `once` protege el cierre del canal `quit`, que panicaría al segundo
// cierre) y agrega los errores de los hijos en vez de quedarse con el primero:
// cerrar un sensor no debe impedir cerrar los demás.
func (m *mergedDetector) Close() error {
	var errs []error
	m.once.Do(func() {
		close(m.quit)
		for _, d := range m.srcs {
			if d == nil {
				continue
			}
			if err := d.Close(); err != nil {
				errs = append(errs, err)
			}
		}
	})
	return errors.Join(errs...)
}
