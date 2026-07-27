package supervisor

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/JoseAFlores777/ccp/internal/core"
)

// trace.go — lo que el usuario ve mientras el supervisor trabaja.
//
// Dos canales, con criterio explícito:
//   - o.Out lleva la TRAZA: los saltos, la vuelta a casa, el desenlace y la
//     tabla de cooldowns. Es el registro de lo que le pasó a su sesión, y es lo
//     que quedará en el log del cron de las 3am.
//   - o.Err lleva la charla operativa: qué se está lanzando, que se está
//     esperando el min_dwell, avisos. Ruido útil en vivo, prescindible después.
//
// La separación importa por el modo headless: ahí o.Out lleva además el
// stream-json del hijo, así que todo lo que se escriba de más queda intercalado
// con la salida que el usuario podría querer procesar. La traza va igualmente a
// Out porque es contrato (el CLI la espera ahí), pero la charla no.
//
// El paquete NO traduce: internal/supervisor no habla i18n a propósito (los
// mensajes finales al usuario los formatea internal/cli, que sí tiene catálogo).
// Lo de aquí es una traza operativa en español, como el resto de internal/core.

func (r *runner) tracef(format string, a ...any) {
	fmt.Fprintf(r.o.Out, format+"\n", a...)
}

func (r *runner) warnf(format string, a ...any) {
	fmt.Fprintf(r.o.Err, "ccp session: "+format+"\n", a...)
}

// traceLaunch anuncia un lanzamiento. Va a Err: en headless, Out es el
// stream-json y esta línea lo ensuciaría sin aportar al registro.
func (r *runner) traceLaunch(profile, session string, resume bool) {
	verb := "nueva sesión"
	if resume {
		verb = "reanudando"
	}
	r.warnf("▶ %s · %s %s", profile, verb, core.ShortUUID(session))
}

// traceDwellWait explica la pausa antes de rotar. Sin esta línea, el usuario ve
// una sesión que "se quedó pensando" tras el aviso de límite y no sabe si el
// supervisor está vivo.
func (r *runner) traceDwellWait(ev core.LimitEvent) {
	r.warnf("límite detectado (%s); esperando min_dwell %s antes de rotar",
		limitLabel(ev), r.rc.Policy.MinDwell)
}

// traceMove es la línea principal de la traza: TODO movimiento de la
// conversación pasa por aquí, lo dispare un límite o el temporizador de regreso.
//
//	personal-cc (4h 03m) ──[uso 94% ≥ umbral 90% · statusline]──→ handoff a app-cc (préstamo 1/6)
//	app-cc (2h 00m) ──[return_check: personal-cc ya liberó su ventana]──→ volviendo a personal-cc (vuelta a casa, no gasta préstamo: siguen 1/6)
//
// Que las dos causas compartan formateador NO es ahorro de código: es lo que
// impide que el mismo suceso se cuente con dos vocabularios. Antes había tres
// textos distintos para dos movimientos —«salto N/M» en la rotación (incluida la
// vuelta a casa POR LÍMITE) y «préstamos N/M» en la vuelta por temporizador— y
// eso hacía la traza ilegible justo donde más importa: como `max_hops` cuenta
// PRÉSTAMOS y la vuelta a casa no consume presupuesto (ver policy.go), dos
// movimientos consecutivos imprimían el MISMO número. El usuario del log de las
// 3am no podía saber cuánto presupuesto le quedaba, y encima el regreso se
// anunciaba como «préstamo» cuando el destino era su propia cuenta.
//
// El criterio, entonces:
//   - el contador se llama siempre «préstamo(s)», que es lo único que se cuenta;
//   - la vuelta a casa NUNCA se presenta como préstamo — es su cierre;
//   - y cuando el número se repite, la línea DICE por qué («no gasta préstamo»),
//     en vez de dejar al lector deducir que no es un error.
//
// Se llama DESPUÉS de Advance, así que r.chain.Hops() ya refleja el estado
// posterior al movimiento: en una ida es «este préstamo», en una vuelta es «los
// que seguían gastados antes y siguen gastados ahora».
func (r *runner) traceMove(from, to, reason string, dwelt time.Duration, home bool) {
	what := fmt.Sprintf("handoff a %s (préstamo %d/%d)", to, r.chain.Hops(), r.chain.maxHops)
	if home {
		what = fmt.Sprintf("volviendo a %s (vuelta a casa, no gasta préstamo: siguen %d/%d)",
			to, r.chain.Hops(), r.chain.maxHops)
	}
	r.tracef("%s (%s) ──[%s]──→ %s", from, shortDur(dwelt), reason, what)
}

// traceHop es el movimiento causado por un límite: el motivo es el evento.
func (r *runner) traceHop(from, to string, dwelt time.Duration, ev core.LimitEvent, home bool) {
	r.traceMove(from, to, limitLabel(ev), dwelt, home)
}

// traceReturnHome es el movimiento que dispara el temporizador de `return_check`.
//
// El motivo va explícito porque este movimiento es el único que ocurre SIN que
// nada haya fallado: el usuario que lea el log de las 3am tiene que poder
// distinguirlo de una rotación por límite de un vistazo.
func (r *runner) traceReturnHome(from, to string, dwelt time.Duration) {
	r.traceMove(from, to, returnHomeReason(to), dwelt, true)
}

// returnHomeReason es el porqué del regreso, apto para Hop.Reason y para la
// traza. Mismo papel que hopReason para los límites.
func returnHomeReason(primary string) string {
	return "return_check: " + primary + " ya liberó su ventana"
}

// traceReturnDwellWait explica la única espera que el usuario podría leer como
// «el primario ya está libre y aquí no se mueve nadie»: min_dwell todavía manda.
// Va a Err (charla operativa) y se imprime una sola vez por lanzamiento.
func (r *runner) traceReturnDwellWait(primary string) {
	r.warnf("%s ya liberó su ventana; esperando min_dwell %s antes de volver",
		primary, r.rc.Policy.MinDwell)
}

// traceReturnBusyWait explica la otra espera del regreso: el primario ya está
// libre pero la conversación sigue VIVA (el transcript se movió hace nada), así
// que no se la interrumpe. Va a Err y se imprime una sola vez por lanzamiento —
// con `return_check` de 10m y una sesión de horas, repetirla llenaría el log de
// la misma línea sin aportar nada nuevo.
func (r *runner) traceReturnBusyWait(primary string) {
	r.warnf("%s ya liberó su ventana; la sesión sigue activa, se volverá cuando lleve %s en silencio",
		primary, r.rc.Policy.ReturnIdle)
}

// traceAdoptHome es la vuelta a casa de un préstamo DEGRADADO: no hubo marcador
// que cerrar (la rotación ocurrió sin transcript que migrar) pero la
// conversación nació después, así que se trae al primario como sesión nueva.
//
// Va a Out —es registro, no charla— y nombra el uuid porque es exactamente lo
// que el usuario teclearía en `claude --resume`. El matiz «sin marcador» está
// dicho a propósito: el préstamo nunca apareció en `ccp handoff list`, y sin
// esta línea el cambio de uuid parecería un fallo.
func (r *runner) traceAdoptHome(from, primary, newID string) {
	r.tracef("sin marcador de préstamo: la conversación de %s se lleva a %s como sesión nueva %s",
		from, primary, core.ShortUUID(newID))
}

// traceReturned informa del uuid NUEVO con el que vive la sesión en el primario.
// Es dato accionable, no adorno: es lo que el usuario teclea en
// `claude --resume` si quiere seguir a mano.
func (r *runner) traceReturned(primary, newID string) {
	r.tracef("sesión devuelta a %s como %s", primary, core.ShortUUID(newID))
}

func (r *runner) traceDone(profile string, code int) {
	if code == 0 {
		r.tracef("%s ──[termina]──→ ✅ exit 0", profile)
		return
	}
	r.tracef("%s ──[termina]──→ exit %d", profile, code)
}

// traceInterrupted deja constancia de que el marcador SIGUE vivo tras un Ctrl-C:
// el usuario tiene que saber que su repo resuelve al perfil prestado hasta que
// cierre el handoff a mano.
func (r *runner) traceInterrupted(profile string, markerLive bool, m core.Marker) {
	r.tracef("%s ──[Ctrl-C]──→ exit %d (sin rotar)", profile, exitSIGINT)
	if markerLive {
		r.tracef("la sesión sigue prestada %s → %s; ciérrala con `ccp handoff end` cuando quieras",
			m.From, m.To)
	}
}

// traceParked imprime el porqué de la parada y la tabla de cooldowns.
//
// Es el mensaje más importante del supervisor: aparece cuando el usuario se ha
// quedado sin cuentas, típicamente de madrugada, y tiene que responder «¿cuándo
// puedo volver?» sin abrir ningún otro comando.
func (r *runner) traceParked(now time.Time) {
	if r.chain.Hops() >= r.chain.maxHops {
		// «préstamos», no «saltos»: es el mismo contador que imprime cada
		// movimiento (ver traceMove), y llamarlo distinto justo en la línea que
		// explica por qué la corrida se para sería el peor sitio para cambiar de
		// vocabulario.
		r.tracef("presupuesto de préstamos agotado (max_hops=%d): no se rota más", r.chain.maxHops)
	} else {
		r.tracef("no queda ningún perfil disponible en la cadena")
	}

	cds := r.chain.Cooldowns()
	if len(cds) == 0 {
		return
	}
	names := make([]string, 0, len(cds))
	width := 0
	for n := range cds {
		names = append(names, n)
		if len(n) > width {
			width = len(n)
		}
	}
	// Orden por instante de liberación (y por nombre al empatar): lo primero que
	// se lee es lo primero que vuelve a estar disponible.
	sort.Slice(names, func(i, j int) bool {
		if cds[names[i]].Equal(cds[names[j]]) {
			return names[i] < names[j]
		}
		return cds[names[i]].Before(cds[names[j]])
	})

	r.tracef("perfiles en cooldown:")
	for _, n := range names {
		until := cds[n]
		if !until.After(now) {
			// Puede pasar: el perfil ya se liberó pero el presupuesto de préstamos
			// se acabó antes. Decirlo evita que el usuario crea que tiene que
			// esperar.
			r.tracef("  %-*s  ya disponible", width, n)
			continue
		}
		r.tracef("  %-*s  libre en %s (%s)", width, n,
			shortDur(until.Sub(now)), until.Local().Format("15:04"))
	}
}

// traceDryRun explica qué haría el supervisor sin lanzar nada.
//
// Incluye las muestras de rate limits que ya haya en disco porque el --dry-run
// se usa justo para responder «¿me va a servir de algo lanzar esto ahora?», y
// esa respuesta depende de cuánto quede en cada cuenta, no solo de la política.
func (r *runner) traceDryRun() {
	pol := r.rc.Policy
	r.tracef("política: %s", pol.Name)
	r.tracef("primario: %s (cwd %s)", r.rc.Primary, r.o.Cwd)
	if len(r.rc.Fallback) == 0 {
		r.tracef("cadena de préstamos: (vacía)")
	} else {
		r.tracef("cadena de préstamos: %s", strings.Join(r.rc.Fallback, " → "))
	}
	if len(r.rc.Denied) > 0 {
		r.tracef("denegados por allow_from: %s", strings.Join(r.rc.Denied, ", "))
	}
	maxHops := r.o.MaxHops
	if maxHops <= 0 {
		maxHops = pol.MaxHops
	}
	r.tracef("umbral %d%% · min_dwell %s · max_hops %d · return_check %s · return_idle %s · cooldown %s (respaldo %s)",
		pol.Threshold, pol.MinDwell, maxHops, pol.ReturnCheck, pol.ReturnIdle,
		pol.CooldownStrategy, pol.CooldownFallback)
	if pol.ReturnIdle <= 0 {
		r.tracef("return_idle 0s: el regreso al primario NO espera a que la sesión esté ociosa")
	}
	if r.o.NoReturn {
		r.tracef("--no-return: la sesión no volverá al primario aunque se libere")
	}

	r.tracef("muestras de rate limits:")
	now := r.o.Now()
	for _, name := range append([]string{r.rc.Primary}, append(append([]string{}, r.rc.Fallback...), r.rc.Denied...)...) {
		rl, sampled, ok := core.ReadRateLimits(r.o.Home, name)
		if !ok {
			r.tracef("  %s: sin muestra", name)
			continue
		}
		age := "instante desconocido"
		if !sampled.IsZero() {
			age = "hace " + shortDur(now.Sub(sampled))
		}
		r.tracef("  %s: 5h %.0f%% · 7d %.0f%% (%s)",
			name, rl.FiveHour.UsedPercentage, rl.SevenDay.UsedPercentage, age)
	}
	r.tracef("--dry-run: no se lanza nada")
}

// hopReason es el texto que queda en Hop.Reason: el porqué del salto, apto para
// un JSON o una tabla, sin adornos.
func hopReason(ev core.LimitEvent) string {
	return limitLabel(ev)
}

// limitLabel resume un LimitEvent en una línea corta.
//
// Se prefiere Detail (que trae el texto real de CC o el porcentaje del sensor
// proactivo) sobre la ventana, porque es lo que permite al usuario distinguir
// «se acabó la ventana de 5h» de «se acabó la semanal» sin abrir el transcript.
func limitLabel(ev core.LimitEvent) string {
	label := strings.TrimSpace(firstLine(ev.Detail))
	if label == "" {
		win := string(ev.Window)
		if win == "" {
			win = string(core.WindowUnknown)
		}
		label = "límite " + win
	}
	const max = 72
	if runes := []rune(label); len(runes) > max {
		label = string(runes[:max-1]) + "…"
	}
	if src := strings.TrimSpace(ev.Source); src != "" {
		label += " · " + src
	}
	return label
}

func firstLine(s string) string {
	if i := strings.IndexAny(s, "\r\n"); i >= 0 {
		return s[:i]
	}
	return s
}

// shortDur formatea una duración para humanos con la precisión que importa en
// cada escala: segundos si es corta, minutos si es media, horas y minutos si es
// larga. time.Duration.String() daría "4h3m12.0034s", que es ruido.
func shortDur(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Round(time.Second).Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	default:
		h := int(d / time.Hour)
		m := int((d % time.Hour) / time.Minute)
		return fmt.Sprintf("%dh %02dm", h, m)
	}
}

// traceNoTranscript explica la rotación DEGRADADA: se detectó el límite antes de
// que la conversación existiera, así que no hubo nada que prestar. Va a Err
// porque no es un salto de la conversación del usuario (no hay conversación),
// pero sin la línea el cambio de uuid parecería un fallo.
func (r *runner) traceNoTranscript(from, to string) {
	r.warnf("sin transcript que migrar en %s: se arranca en %s como sesión nueva", from, to)
}
