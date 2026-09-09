package supervisor

import (
	"time"

	"github.com/JoseAFlores777/ccp/internal/core"
)

// policy.go — la máquina de estados de la rotación.
//
// Chain es deliberadamente PURA: no lee el reloj, no toca disco, no lanza
// procesos. Todo instante entra por parámetro. Esa pureza es lo que permite
// probar en microsegundos escenarios que en producción tardan horas (un
// cooldown de una ventana de 5h, un min_dwell de 20m) y lo que deja el resto
// del supervisor —que sí es I/O sucio: pipes, señales, ttys— sin decisiones
// que razonar.
//
// El modelo NO es un round-robin circular sino un PÉNDULO. El primario es el
// dueño natural del path (core.Resolve del cwd, ya resuelto en
// core.ResolvedChain.Primary); los fallback son PRÉSTAMOS temporales. Por eso
// Next() mira primero al primario siempre, esté la cadena donde esté: en
// cuanto su ventana se reabre, la sesión vuelve a casa aunque queden préstamos
// disponibles. Un round-robin dejaría al usuario trabajando en el perfil
// equivocado durante horas sin motivo — con la cuenta del cliente equivocado,
// que es peor que quedarse parado.

// Chain es la máquina de estados de la rotación: sabe en qué perfil está,
// cuáles quemó, cuándo puede volver al primario y cuándo se acabaron los
// préstamos.
type Chain struct {
	policy   core.EffectivePolicy
	primary  string
	fallback []string // préstamos en orden de preferencia (ya filtrados por allow_from)

	maxHops  int  // tope duro de PRÉSTAMOS; backstop anti-bucle
	noReturn bool // deshabilita el regreso al primario

	current string    // perfil activo
	since   time.Time // instante en que se llegó a `current` (base de MinDwell)
	hops    int       // préstamos ya consumidos (la vuelta a casa no cuenta)

	// cooldowns es perfil -> instante en que vuelve a estar disponible. Un
	// perfil ausente del mapa nunca se agotó; uno presente con instante ya
	// pasado está agotado-pero-recuperado. Guardar el INSTANTE y no un bool
	// es lo que permite que la disponibilidad se derive del `now` que entra
	// por parámetro en vez de necesitar un temporizador propio.
	cooldowns map[string]time.Time
}

// NewChain arranca la cadena en el primario de rc.
//
// maxHops <= 0 significa "usa el de la política" (así el flag --max-hops del
// CLI puede pasarse tal cual sin que el caller tenga que resolver el default).
// Si aun así queda en cero se cae al default global: una cadena sin tope sería
// un bucle infinito potencial, y el backstop no es negociable.
//
// El fallback se COPIA y se sanea (fuera vacíos, el primario y los duplicados)
// aunque ResolveAutoChain ya lo haga: NewChain también se llama con cadenas
// construidas a mano (tests, futuros callers) y un primario colado en la lista
// haría que Next() propusiera saltar a donde ya estamos.
func NewChain(rc core.ResolvedChain, maxHops int, noReturn bool, now time.Time) *Chain {
	if maxHops <= 0 {
		maxHops = rc.Policy.MaxHops
	}
	if maxHops <= 0 {
		maxHops = core.DefaultAutoMaxHops
	}

	seen := map[string]bool{rc.Primary: true}
	fb := make([]string, 0, len(rc.Fallback))
	for _, name := range rc.Fallback {
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		fb = append(fb, name)
	}

	return &Chain{
		policy:    rc.Policy,
		primary:   rc.Primary,
		fallback:  fb,
		maxHops:   maxHops,
		noReturn:  noReturn,
		current:   rc.Primary,
		since:     now,
		hops:      0,
		cooldowns: map[string]time.Time{},
	}
}

// Current es el perfil activo.
func (c *Chain) Current() string { return c.current }

// Primary es el dueño natural del path: a donde vuelve la sesión.
func (c *Chain) Primary() string { return c.primary }

// MarkExhausted apunta que `profile` topó su límite y calcula cuándo vuelve a
// ser elegible.
//
// Reglas del cooldown, en orden:
//   - estrategia "fixed" -> se IGNORA resetsAt y se espera siempre
//     CooldownFallback. Es el modo de los perfiles con API key, donde el
//     resets_at que llegue (si llega) no describe una ventana de suscripción.
//   - resetsAt distinto de cero y futuro -> se usa tal cual: es la señal buena,
//     la que dice exactamente cuándo reabre la ventana.
//   - resetsAt cero (desconocido) o ya pasado -> now + CooldownFallback.
//
// El caso "ya pasado" merece explicación: si aceptáramos un resets_at rancio,
// el perfil quedaría disponible en el mismo instante en que lo marcamos
// agotado, Next() lo volvería a elegir y la cadena entraría en un ping-pong
// que solo pararía MaxHops. Una señal que se contradice a sí misma se trata
// como señal ausente.
func (c *Chain) MarkExhausted(profile string, resetsAt, now time.Time) {
	if profile == "" {
		return
	}
	until := now.Add(c.policy.CooldownFallback)
	if c.policy.CooldownStrategy != core.CooldownFixed && !resetsAt.IsZero() && resetsAt.After(now) {
		until = resetsAt
	}
	c.cooldowns[profile] = until
}

// available reporta si `profile` puede recibir la sesión en `now`. Ausencia de
// entrada = nunca se agotó; presencia = disponible solo cuando el instante ya
// llegó (>=, no >: en el borde exacto la ventana ya reabrió).
func (c *Chain) available(profile string, now time.Time) bool {
	until, marked := c.cooldowns[profile]
	return !marked || !now.Before(until)
}

// ReturnDue reporta si toca volver a casa: estamos fuera del primario, el
// regreso no está deshabilitado y la ventana del primario ya reabrió.
//
// Es la segunda regla de Next() extraída como pregunta independiente, porque el
// supervisor necesita hacérsela SIN que haya ocurrido un límite: es lo que
// consulta el temporizador de `return_check` (ver launchAndWatch). Sin ella, la
// vuelta a casa solo se reevaluaría cuando otro sensor disparase, y una ventana
// que reabre a las 3am no despertaría a nadie.
func (c *Chain) ReturnDue(now time.Time) bool {
	return !c.noReturn && c.current != c.primary && c.available(c.primary, now)
}

// Next elige el destino del siguiente hop.
//
// El orden de las tres reglas ES el «principio de retorno al primario»:
//  1. el primario ya cumplió su cooldown y no estamos en él -> el primario,
//     aunque queden préstamos frescos. Volver a casa siempre gana;
//  2. presupuesto de PRÉSTAMOS agotado -> no hay destino (backstop anti-bucle);
//  3. si no, el primer fallback no agotado en el orden declarado (que ya viene
//     filtrado por allow_from desde ResolveAutoChain: aquí no se re-evalúa el
//     gate, se respeta la lista que llegó).
//
// Que la vuelta a casa vaya ANTES del tope y no después es deliberado, y es la
// misma decisión que Advance() no contando el regreso: `max_hops` es «cuántas
// veces se puede PRESTAR la sesión antes de rendirse», y volver a casa no es un
// préstamo sino su cierre. Con el tope por delante, un presupuesto consumido
// dejaba la conversación abandonada en la cuenta ajena aunque el primario ya
// estuviera libre — exactamente el estado que la política quiere evitar. No
// abre la puerta a un bucle: para volver a casa hay que haber salido, y salir
// SIEMPRE cuesta un préstamo, así que los regresos están acotados por los
// préstamos y el total de relanzamientos por 2·max_hops + 1.
//
// Nunca devuelve el perfil actual: un "salto" a donde ya estamos costaría un
// relanzamiento de claude para nada.
func (c *Chain) Next(now time.Time) (target string, ok bool) {
	if c.ReturnDue(now) {
		return c.primary, true
	}
	if c.hops >= c.maxHops {
		return "", false
	}
	for _, name := range c.fallback {
		if name == c.current {
			continue
		}
		if c.available(name, now) {
			return name, true
		}
	}
	return "", false
}

// Advance registra el salto ya ejecutado: mueve Current, reinicia el reloj de
// permanencia y consume presupuesto SI el salto es un préstamo.
//
// Se llama DESPUÉS de que el handoff haya ocurrido de verdad, no al elegir
// destino: si el handoff falla, la cadena debe quedar como estaba para poder
// reintentar con otro destino sin haber pagado el hop.
//
// El regreso al primario NO consume presupuesto: `max_hops` cuenta PRÉSTAMOS
// (así lo documenta la política), y cobrar también la vuelta convertiría
// `max_hops: 6` en «tres idas y vueltas» aunque solo se hubieran usado dos
// cuentas. El backstop anti-bucle sigue en pie porque cada regreso exige una
// salida previa, y esa sí se cobró.
//
// El cooldown del destino NO se borra: el mapa es historial además de estado,
// y available() ya lo interpreta por tiempo. Borrarlo perdería la traza de que
// ese perfil se quemó antes, que es justo lo que quiere ver el usuario en la
// tabla final.
func (c *Chain) Advance(target string, now time.Time) {
	if target == "" || target == c.current {
		return
	}
	c.current = target
	c.since = now
	if target != c.primary {
		c.hops++
	}
}

// reactiveDwellCap es el techo de permanencia que se exige cuando el límite ya
// ocurrió. Ver DwellFor.
const reactiveDwellCap = 30 * time.Second

// DwellSatisfiedFor reporta si se cumplió `dwell` en el perfil actual.
func (c *Chain) DwellSatisfiedFor(now time.Time, dwell time.Duration) bool {
	if dwell <= 0 {
		return true
	}
	return now.Sub(c.since) >= dwell
}

// DwellSatisfied reporta si se cumplió MinDwell entero en el perfil actual. Es
// el criterio del regreso voluntario (decideReturn), donde nadie tiene prisa.
func (c *Chain) DwellSatisfied(now time.Time) bool {
	return c.DwellSatisfiedFor(now, c.policy.MinDwell)
}

// DwellFor devuelve la permanencia mínima exigible antes de rotar por un evento
// de `source`, y es donde se separan las dos cosas que MinDwell mezclaba.
//
// La justificación clásica de MinDwell —amortiguar la ráfaga, porque statusLine,
// transcript y sentinel pueden reportar el MISMO límite con segundos de
// diferencia— ya no la sostiene él: dentro de un lanzamiento la deduplican
// runner.fresh y, sobre todo, el `if limit != nil { continue }` del select, que
// deja actuar solo al primer límite. Lo que MinDwell hace de verdad es un piso
// de permanencia ENTRE lanzamientos, y eso sigue siendo útil: evita que tres
// perfiles se quemen en un minuto.
//
// Pero aplicarlo entero a un evento REACTIVO es daño puro. Reactivo significa
// que el turno ya falló: el 429 está servido, el usuario está mirando un error,
// y `c.since` se siembra en NewChain, así que en el primer lanzamiento el piso
// son los 20 minutos completos DESDE QUE ARRANCÓ LA CORRIDA. Veinte minutos
// dentro de una cuenta que no responde, teniendo otra fresca al lado.
//
// Así que el dwell entero se reserva para el sensor PROACTIVO ("statusline"),
// que es el único que avisa antes de que nada falle y por tanto el único que
// puede permitirse esperar. Todo lo demás —"transcript", "stream-json", "hook",
// y cualquier origen que no reconozcamos— cae al techo corto. El default es
// deliberadamente el del caso urgente: el Source de un sentinel lo escribe el
// hook a partir de un payload que viene de fuera, y ante un origen que no
// sabemos leer preferimos rotar pronto de más que retener al usuario dentro de
// un límite.
func (c *Chain) DwellFor(source string) time.Duration {
	if source == "statusline" {
		return c.policy.MinDwell
	}
	if c.policy.MinDwell < reactiveDwellCap {
		return c.policy.MinDwell
	}
	return reactiveDwellCap
}

// Hops es el número de PRÉSTAMOS consumidos (las vueltas a casa no cuentan).
func (c *Chain) Hops() int { return c.hops }

// Cooldowns devuelve, por perfil agotado, cuándo vuelve a estar disponible.
// Es lo que alimenta la tabla del mensaje «todos los perfiles agotados».
//
// Devuelve una COPIA: el caller solo formatea, y un mapa interno expuesto es
// una invitación a que la presentación mute el estado de la rotación.
func (c *Chain) Cooldowns() map[string]time.Time {
	out := make(map[string]time.Time, len(c.cooldowns))
	for k, v := range c.cooldowns {
		out[k] = v
	}
	return out
}
