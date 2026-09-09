// Package supervisor es el bucle de `ccp session`: lanza `claude`, vigila si
// topa un límite de uso, presta la sesión a otro perfil y la relanza allí, hasta
// que el trabajo termina o se acaban los perfiles.
//
// El reparto de responsabilidades del paquete es deliberado:
//
//	policy.go    decide A DÓNDE saltar (puro, sin reloj ni disco)
//	detect.go    decide CUÁNDO hay que saltar (sensores, cada uno con su ruido)
//	launcher.go  sabe arrancar y matar al hijo (señales, tty, pipes)
//	supervisor.go pega las tres piezas y es el ÚNICO que muta estado en disco
//	              (marcadores de handoff) — todo lo demás es observación.
//
// Esa separación es lo que hace auditable la parte peligrosa: rotar de perfil
// solo (a las 3am, sin nadie mirando) implica copiar transcripts y reescribir
// handoffs.yaml. Concentrarlo en un archivo con un bucle explícito permite
// razonar sobre la invariante que de verdad importa: NUNCA hay dos hijos vivos a
// la vez, y un handoff fallido para el bucle en vez de dejarlo girando.
package supervisor

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/JoseAFlores777/ccp/internal/core"
)

// ParkedExitCode es el código de salida de «todos los perfiles agotados».
//
// 75 es EX_TEMPFAIL de sysexits.h: «fallo temporal, reintenta». Es exactamente
// la semántica que queremos para un cron o un CI que invoque `ccp session -p`:
// no es un error del usuario (que sería 1) ni un fallo del modelo, es «vuelve
// dentro de un rato». Se exporta porque el CLI lo mapea y un literal 75 suelto
// en dos paquetes es una divergencia esperando a ocurrir.
const ParkedExitCode = 75

const (
	// termGrace es lo que se le da al hijo para morir por las buenas tras el
	// SIGTERM. Generoso a propósito: claude corre hooks SessionEnd y termina de
	// escribir el jsonl, y ese jsonl es justo lo que el handoff va a copiar al
	// perfil destino. Matarlo antes partiría la conversación por la mitad.
	termGrace = 10 * time.Second

	// exitDrain es la ventana que se le da a los sensores para entregar el
	// evento que EXPLICA una salida con código no-cero.
	//
	// Hace falta porque la carrera es real y frecuente: `claude -p` imprime la
	// línea `api_retry` del rate limit y muere acto seguido. El evento y el
	// Wait del hijo quedan listos casi a la vez, y `select` elige al azar entre
	// canales listos — sin esta espera, la mitad de las veces propagaríamos el
	// exit code en vez de rotar. 150ms es imperceptible para un humano y sobra
	// para un traspaso entre goroutines.
	exitDrain = 150 * time.Millisecond

	// exitSIGINT es el 130 de bash/zsh (128 + SIGINT): el Ctrl-C del usuario.
	exitSIGINT = 130
)

// Options configura una corrida del supervisor.
type Options struct {
	Home      string
	Cwd       string
	Policy    string
	Headless  bool
	Yolo      bool
	MaxHops   int    // 0 = el de la política
	Session   string // uuid a reanudar; "" => se pre-asigna uno nuevo
	Args      []string
	ClaudeBin string // "claude" por defecto
	Now       func() time.Time
	Out, Err  io.Writer
	Stdin     io.Reader
	DryRun    bool
	NoReturn  bool
	Poll      time.Duration // 0 => 2s (los tests lo bajan)

	// childOut/childErr son Out/Err SIN el envoltorio con mutex, y son los que
	// recibe el proceso hijo. Los rellena normalize; nadie de fuera los fija.
	childOut, childErr io.Writer
}

// Hop es un salto ya ejecutado: el registro de que la sesión cambió de perfil.
type Hop struct {
	From, To string
	At       time.Time
	Reason   string
	Session  string // uuid con el que continúa la sesión DESPUÉS del salto
}

// Result es el desenlace de la corrida.
type Result struct {
	ExitCode int
	Hops     []Hop
	Session  string // uuid con el que terminó
	Profile  string // perfil en el que terminó
	Parked   bool   // salió por «todos agotados»
}

// normalize aplica los defaults y valida lo mínimo. Se llama sobre la COPIA que
// recibe Run, así que no muta el Options del llamador.
//
// Out/Err se envuelven en un writer con mutex porque tienen DOS escritores
// concurrentes: la traza del bucle y el `tee` del detector de stream-json, que
// corre en su propia goroutine copiando la salida del hijo. Sin el candado, un
// hop en mitad de una línea NDJSON produciría salida entrelazada (y una carrera
// de datos de libro en los tests).
//
// El envoltorio es SOLO para los escritores en-proceso. Los originales se
// guardan aparte (childOut/childErr) porque son los que se le pasan al hijo: os/exec
// hereda el descriptor únicamente cuando el writer es un *os.File, y con
// cualquier otro io.Writer monta un os.Pipe() con goroutine de copia. Envolver
// lo que va al hijo le daría a claude un pipe en vez de la tty —sin TUI, sin
// colores, sin SIGWINCH— y ataría el cmd.Wait al EOF de esos pipes (un nieto
// vivo, un MCP stdio, lo dejaría colgado hasta WaitDelay y devolvería exit 1 por
// una sesión que terminó en 0). El hijo es otro PROCESO: escribe al fd por su
// cuenta y no compite con el mutex de nadie.
func (o *Options) normalize() error {
	if strings.TrimSpace(o.Home) == "" {
		return fmt.Errorf("supervisor: falta el home de ccp")
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.Poll <= 0 {
		o.Poll = defaultPoll
	}
	if strings.TrimSpace(o.ClaudeBin) == "" {
		o.ClaudeBin = "claude"
	}
	if strings.TrimSpace(o.Cwd) == "" {
		wd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("supervisor: no se pudo determinar el cwd: %w", err)
		}
		o.Cwd = wd
	}
	lockedOut, lockedErr := lockWriter(o.Out), lockWriter(o.Err)
	o.childOut, o.childErr = childWriter(o.Out, lockedOut), childWriter(o.Err, lockedErr)
	o.Out, o.Err = lockedOut, lockedErr
	return nil
}

// runner es el estado vivo de una corrida. Existe para no arrastrar ocho
// parámetros por cada función del bucle.
type runner struct {
	o     Options
	cfg   *core.Config
	rc    core.ResolvedChain
	chain *Chain

	// seen deduplica eventos por contenido DENTRO DE UN LANZAMIENTO. Lo resetea
	// launchAndWatch en cada hijo; nunca sobrevive a un hop.
	//
	// No es una optimización, es una protección: cubre el MISMO límite llegando
	// por dos sensores a la vez, y una reescritura del jsonl que devuelva viejas
	// líneas al tramo vigilado. Lo que la línea base del watcher
	// (transcriptBaseline) ya resuelve por su cuenta es el otro caso: el 429 que
	// viajó dentro de la copia del jsonl al perfil destino.
	//
	// El alcance por lanzamiento es deliberado y se pagó caro: con un `seen` de
	// corrida entera, dos perfiles que topan su límite con un mensaje
	// byte-idéntico —el caso NORMAL, no el raro: dos cuentas agotando su ventana
	// de 5h producen exactamente el mismo texto de CC— dejaban mudo al segundo.
	// Y sin evento no hay rotación: el bucle cae en `out.exited && out.limit ==
	// nil` y propaga el código del hijo, así que la cadena moría con el exit 1
	// de claude en vez de aparcar con 75 y su tabla de cooldowns.
	seen map[string]bool

	// copied recuerda a qué perfiles ESTA corrida ya copió el transcript de esta
	// sesión (clave perfil+uuid). Es lo que distingue «el destino tiene una
	// conversación ajena con el mismo uuid» —que no se pisa jamás— de «el destino
	// tiene una copia vieja de ESTA misma conversación», que es exactamente lo que
	// hay al volver a un fallback cuyo cooldown ya venció.
	copied map[string]bool
}

// copyKey identifica la copia del transcript de `session` dentro de `profile`.
func copyKey(profile, session string) string { return profile + "\x00" + session }

// ownsCopy reporta si la copia que hay en `profile` la dejó esta misma corrida.
func (r *runner) ownsCopy(profile, session string) bool {
	return r.copied[copyKey(profile, session)]
}

// transcriptPath es el jsonl de `session` dentro del cc-home de `profile`.
func (r *runner) transcriptPath(profile, session string) (string, error) {
	cc, err := core.CCHome(r.o.Home, profile)
	if err != nil {
		return "", err
	}
	return core.ProjectDir(cc, core.SlugForCwd(r.o.Cwd)) + "/" + session + ".jsonl", nil
}

// hasTranscript reporta si `profile` ya tiene el jsonl de esta sesión. Un error
// resolviendo el cc-home cuenta como «no hay»: el handoff fallaría igual y la
// degradación es la misma.
func (r *runner) hasTranscript(profile, session string) bool {
	path, err := r.transcriptPath(profile, session)
	if err != nil {
		return false
	}
	_, err = os.Stat(path)
	return err == nil
}

// adoptHome lleva la conversación al primario cuando NO hay marcador vivo que
// cerrar. Devuelve el uuid con el que sigue la sesión y si ese uuid es
// reanudable (`--resume`) o hay que arrancar de cero (`--session-id`).
//
// El estado que cubre es el préstamo DEGRADADO: la rotación ocurrió antes de que
// existiera el jsonl (el sensor proactivo puede disparar con la sesión recién
// abierta), así que no hubo nada que prestar y nadie abrió handoff. Volver de ahí
// no puede usar ninguna de las dos herramientas normales: HandoffEndSession exige
// marcador y HandoffChain crearía uno INVERTIDO. Los dos subcasos:
//
//   - sin transcript en el perfil actual: no hay nada que migrar; se arranca en
//     el primario con uuid nuevo (uno nuevo, no el de antes, para no chocar con
//     un jsonl que ese perfil pudiera tener de una sesión anterior);
//   - con transcript: la conversación NACIÓ durante el préstamo degradado y es
//     tan real como cualquier otra; se lleva a casa como sesión nueva.
func (r *runner) adoptHome(from, primary, session string) (string, bool, error) {
	if !r.hasTranscript(from, session) {
		newID, err := core.NewUUID()
		if err != nil {
			return "", false, err
		}
		r.traceNoTranscript(from, primary)
		return newID, false, nil
	}
	newID, err := core.HandoffAdoptHome(r.o.Home, from, primary, r.o.Cwd, session)
	if err != nil {
		return "", false, err
	}
	r.traceAdoptHome(from, primary, newID)
	return newID, true, nil
}

// Run corre el bucle lanzar → vigilar → detectar → handoff → relanzar.
func Run(ctx context.Context, o Options) (Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := o.normalize(); err != nil {
		return Result{ExitCode: 1}, err
	}

	// El Config se carga UNA vez y se reutiliza en cada relanzamiento: el
	// entorno del hijo (EnvForChild) debe salir de la misma foto que resolvió la
	// cadena. Recargarlo por hop dejaría que una edición de ccp.yaml a media
	// rotación cambiara las reglas del juego en marcha.
	cfg, err := core.Load(o.Home)
	if err != nil {
		return Result{ExitCode: 1}, err
	}
	rc, err := core.ResolveAutoChain(o.Home, cfg, o.Policy, o.Cwd)
	if err != nil {
		return Result{ExitCode: 1}, err
	}

	r := &runner{o: o, cfg: cfg, rc: rc, seen: map[string]bool{}, copied: map[string]bool{}}

	if o.DryRun {
		r.traceDryRun()
		return Result{Profile: rc.Primary, Session: o.Session}, nil
	}

	// La sesión se PRE-ASIGNA antes de lanzar (--session-id) en vez de dejar que
	// claude invente el uuid: sin conocerlo de antemano no sabríamos qué
	// transcript vigilar ni qué sesión prestar cuando toque rotar, y adivinarlo
	// por mtime es una carrera con cualquier otra sesión abierta en el repo.
	session := o.Session
	resume := true
	if session == "" {
		if session, err = core.NewUUID(); err != nil {
			return Result{ExitCode: 1}, err
		}
		resume = false
	}

	start := o.Now()
	r.chain = NewChain(rc, o.MaxHops, o.NoReturn, start)

	res := Result{Session: session, Profile: r.chain.Current()}
	enteredAt := start

	// marker/markerLive son el reflejo en memoria de handoffs.yaml: si hay un
	// préstamo vivo, quién lo prestó (marker.From) es a dónde vuelve la sesión.
	var marker core.Marker
	markerLive := false

	for {
		// La conversación está prestada Y su marcador la devuelve al primario:
		// solo entonces tiene sentido vigilar si el primario ya reabrió. La
		// segunda condición es la misma que usa la rama `home` de la rotación —
		// un marcador cuyo From no es el primario (rotación degradada, sin
		// transcript) no describe un camino de vuelta a casa.
		onLoan := markerLive && marker.From == r.chain.Primary()

		out, err := r.launchAndWatch(ctx, session, resume, onLoan)
		res.Profile = r.chain.Current()
		res.Session = session
		if err != nil {
			// Fallo de arranque, del Wait, o contexto cancelado. El hijo ya está
			// muerto (launchAndWatch no vuelve con un proceso vivo), así que
			// basta con propagar.
			res.ExitCode = out.code
			if res.ExitCode == 0 {
				res.ExitCode = 1
			}
			return res, err
		}

		// Solo UNO de los dos desenlaces terminales consulta out.exited, y la
		// asimetría es el fondo del asunto: la pregunta no es quién dio el golpe
		// sino qué códigos puede producir nuestro SIGTERM.
		//
		// El 130 no puede. Un hijo al que matamos sale 143, o sale con lo que su
		// handler elija, y elegir justo el código de Ctrl-C sería patológico. Así
		// que ahí el código basta y sigue mandando él solo, igual que antes: el
		// usuario interrumpió a propósito y rotar sería lo contrario de lo pedido.
		//
		// El 0 sí puede, y por eso es el único ambiguo. Un claude que atrape
		// SIGTERM para correr sus hooks SessionEnd —justo lo que termGrace le
		// concede— y salga con 0 después de que lo matáramos por un límite es, por
		// el código, indistinguible de uno que terminó su trabajo. Leerlo como
		// «fin feliz» cierra el préstamo y retorna: la rotación se apaga entera,
		// siempre, sin un solo síntoma. `exited` (o sea: la señal ni llegó a
		// salir, el hijo ya estaba muerto cuando fuimos a matarlo) es lo que los
		// separa, y deja intacta la carrera que documentaba el comentario viejo —
		// el hijo que imprime el límite y muere en el mismo instante devuelve
		// signaled=false y vuelve a ser un exit propio.
		//
		// Queda una ventana: entre que el hijo llama a exit() y que nuestro
		// cosechador recoge el estado, la señal sale contra un zombi y se cuenta
		// como golpe nuestro. Un fin feliz genuino que coincida ahí con un límite
		// en vuelo rota un préstamo de más. Es el intercambio correcto: cuesta un
		// relanzamiento en una carrera de microsegundos, y evita apagar la función
		// entera de forma sistemática.
		switch {
		case out.exited && out.code == 0:
			// Fin feliz. Si la conversación está prestada, devolverla es parte
			// del trabajo: dejar el marcador vivo obligaría al usuario a
			// acordarse de `ccp handoff end` mañana por la mañana, y mientras
			// tanto su repo seguiría resolviendo al perfil prestado.
			res.ExitCode = 0
			// Los sentinels de esta sesión ya son señales consumidas. Reclamarlos
			// también en las salidas limpias (no solo al rotar) es lo que evita que
			// el directorio de estado acumule un archivo por límite topado para
			// siempre: el hook los escribe corra o no el supervisor.
			_ = core.ClearSentinels(o.Home, session)
			if !markerLive {
				r.traceDone(r.chain.Current(), out.code)
				return res, nil
			}
			m, newID, herr := core.HandoffEndSession(o.Home, o.Cwd, session, o.Now())
			if herr != nil {
				// El trabajo del hijo SÍ terminó bien (el exit 0 se conserva),
				// pero el marcador quedó vivo y el usuario tiene que saberlo: se
				// devuelve el error para que el CLI lo reporte.
				r.warnf("la sesión terminó pero no se pudo devolver a %s: %v", marker.From, herr)
				return res, fmt.Errorf("no se pudo cerrar el handoff automático: %w", herr)
			}
			// Aquí se RETORNA, así que lo único que debe quedar escrito es lo que
			// se devuelve. Apagar `markerLive` o mover `session` sería estado
			// muerto: nadie vuelve a leerlos y el linter lo señala (ineffassign).
			// La coherencia no la da el asignar por simetría, la da el return.
			res.Session, res.Profile = newID, m.From
			r.traceReturned(m.From, newID)
			r.traceDone(m.From, 0)
			return res, nil

		case out.code == exitSIGINT:
			// Ctrl-C. El usuario paró la sesión a propósito; rotar aquí sería
			// exactamente lo contrario de lo que pidió. El marcador se deja VIVO
			// a posta: la conversación sigue donde está y él decide si la reanuda
			// o la cierra.
			res.ExitCode = exitSIGINT
			// El marcador se conserva, los sentinels no: son de un límite ya visto
			// y nadie más los va a consumir.
			_ = core.ClearSentinels(o.Home, session)
			r.traceInterrupted(r.chain.Current(), markerLive, marker)
			return res, nil

		case out.exited && out.limit == nil:
			// Murió por su cuenta y ningún sensor vio un límite: no es nuestro
			// problema, es el del usuario. Propagar el código tal cual es lo que
			// hace que `ccp session -p …` sea sustituible por `claude -p …` en
			// un script.
			res.ExitCode = out.code
			r.traceDone(r.chain.Current(), out.code)
			return res, nil
		}

		// --- vuelta a casa por return_check -----------------------------
		//
		// Va ANTES de la rotación y sin exigir `out.limit`: aquí nadie topó nada.
		// El perfil que se deja sigue teniendo cuerda —lo abandonamos por gusto,
		// no por agotamiento— así que NO se llama a MarkExhausted: desterrarlo
		// una hora por haber salido voluntariamente sería quemar un préstamo que
		// puede hacer falta dentro de diez minutos.
		if out.returnHome {
			now := o.Now()
			from := r.chain.Current()
			prevSession := session
			target := r.chain.Primary()
			viaMarker := markerLive
			resumeHome := true

			if viaMarker {
				m, newID, herr := core.HandoffEndSession(o.Home, o.Cwd, session, now)
				if herr != nil {
					res.ExitCode = 1
					return res, fmt.Errorf("no se pudo devolver la sesión a %s: %w", marker.From, herr)
				}
				target = m.From
				marker, markerLive = core.Marker{}, false
				session = newID
			} else {
				// Defensa en profundidad. El temporizador SOLO se arma con préstamo
				// vivo (ver `onLoan`), así que llegar aquí sin marcador no debería
				// ocurrir. Si ocurriera, llamar a HandoffEndSession sería el peor
				// desenlace posible: no encontraría marcador, moriría con «no hay
				// handoff activo…», el mensaje envolvente saldría con un %s VACÍO
				// (marker está a cero) y la corrida entera se iría con exit 1 por un
				// movimiento que era opcional. Degradar al camino de adopción lleva
				// la conversación a casa igual, sin inventar ningún marcador.
				newID, resumable, herr := r.adoptHome(from, target, session)
				if herr != nil {
					res.ExitCode = 1
					return res, fmt.Errorf("no se pudo llevar la sesión a casa (%s → %s): %w", from, target, herr)
				}
				session, resumeHome = newID, resumable
			}

			// Advance DESPUÉS del handoff, igual que en la rotación: si hubiera
			// fallado, la cadena queda como estaba. No consume presupuesto (ver
			// policy.go): max_hops cuenta préstamos, y esto es cerrar uno.
			r.chain.Advance(target, now)
			resume = resumeHome
			_ = core.ClearSentinels(o.Home, prevSession)

			res.Hops = append(res.Hops, Hop{
				From: from, To: target, At: now,
				Reason:  returnHomeReason(target),
				Session: session,
			})
			res.Profile, res.Session = target, session
			r.traceReturnHome(from, target, now.Sub(enteredAt))
			if viaMarker {
				// El uuid cambió al volver: es lo que el usuario teclearía en
				// `claude --resume` para seguir a mano desde aquí. En la rama
				// degradada lo dice ya adoptHome, con el matiz de que no hubo
				// préstamo que devolver.
				r.traceReturned(target, session)
			}
			enteredAt = now
			continue
		}

		if out.limit == nil {
			// Defensivo: solo se llega aquí matando al hijo por un límite, así
			// que limit no debería ser nil. Si lo es, salir es más seguro que
			// rotar sin motivo conocido.
			res.ExitCode = out.code
			return res, nil
		}

		// --- rotación ---------------------------------------------------
		ev := *out.limit
		now := o.Now()
		from := r.chain.Current()
		r.chain.MarkExhausted(from, ev.ResetsAt, now)

		target, ok := r.chain.Next(now)
		if !ok {
			r.traceParked(now)
			res.Parked = true
			res.ExitCode = ParkedExitCode
			return res, nil
		}

		prevSession := session
		// backHome es «este salto termina en casa», con marcador o sin él; home es
		// el subcaso con préstamo VIVO, el único que cierra un handoff. Separarlos
		// es lo que impide el marcador invertido: sin esta distinción, volver al
		// primario desde un préstamo degradado caía en `default:` y HandoffChain
		// creaba un marcador ACTIVO {From: préstamo, To: primario} que nunca se
		// cierra estando en casa — y que en la siguiente salida limpia
		// back-sincronizaría la conversación HACIA el perfil prestado, dejando el
		// transcript donde el usuario no está y una traza que miente sobre dónde
		// reanudar.
		backHome := target == r.chain.Primary()
		home := markerLive && target == marker.From
		// El relanzamiento reanuda salvo que no haya conversación que migrar (ver
		// más abajo): ahí se arranca de cero.
		resumeNext := true
		switch {
		case home:
			// Volver a casa NO es un handoff más: es cerrar el préstamo. La
			// sesión regresa al primario como uuid NUEVO (HandoffEndSession
			// reescribe el sessionId) y el bucle continúa con ese uuid.
			_, newID, herr := core.HandoffEndSession(o.Home, o.Cwd, session, now)
			if herr != nil {
				res.ExitCode = 1
				return res, fmt.Errorf("no se pudo devolver la sesión a %s: %w", marker.From, herr)
			}
			marker, markerLive = core.Marker{}, false
			session = newID

		case backHome:
			// A casa sin marcador que cerrar: el préstamo se hizo por la rama
			// degradada (no había jsonl que migrar) y por eso nadie abrió handoff.
			// La conversación puede haber nacido DURANTE ese préstamo, así que
			// adoptHome decide si hay algo que traerse o si se arranca de cero.
			newID, resumable, herr := r.adoptHome(from, target, session)
			if herr != nil {
				res.ExitCode = 1
				return res, fmt.Errorf("no se pudo llevar la sesión a casa (%s → %s): %w", from, target, herr)
			}
			session = newID
			resumeNext = resumable

		case !r.hasTranscript(from, session):
			// Todavía no hay jsonl: el límite llegó antes del primer turno (el
			// sensor proactivo puede disparar con la sesión recién abierta). No hay
			// nada que copiar, así que el handoff sobra — y tratar el «no encuentro
			// la sesión» de HandoffChain como fatal daría el peor desenlace posible:
			// el hijo ya está muerto, y encima ni se rota ni se sale con 75. Se
			// arranca en el destino como sesión nueva; el uuid se renueva para no
			// chocar con un jsonl que ese perfil pudiera tener de antes.
			newID, herr := core.NewUUID()
			if herr != nil {
				res.ExitCode = 1
				return res, herr
			}
			session = newID
			resumeNext = false
			r.traceNoTranscript(from, target)

		default:
			// force solo cuando la copia que hay en el destino la dejó ESTA corrida
			// (volvimos a un fallback ya visitado): entonces es un prefijo de la
			// conversación de ahora y pisarla es lo correcto. En cualquier otro caso
			// una colisión significa conversación AJENA con el mismo uuid, y ahí se
			// para y se avisa aunque cueste la rotación.
			m, herr := core.HandoffChain(o.Home, from, target, o.Cwd, session, true, r.ownsCopy(target, session), now)
			if herr != nil {
				// Un handoff fallido NO se reintenta con otro destino: si el
				// estado en disco quedó a medias, seguir rotando lo empeora. El
				// usuario prefiere una sesión parada a un bucle quemando
				// perfiles mientras duerme.
				res.ExitCode = 1
				return res, fmt.Errorf("handoff %s → %s: %w", from, target, herr)
			}
			marker, markerLive = m, true
			r.copied[copyKey(target, session)] = true
		}

		// Advance va DESPUÉS del handoff: si el handoff hubiera fallado, la
		// cadena tiene que quedar como estaba (ver policy.go).
		r.chain.Advance(target, now)
		resume = resumeNext

		// Los sentinels de la sesión anterior son señales YA consumidas; si se
		// quedaran, el watcher del siguiente lanzamiento las leería como nuevas
		// (el filtro es por sesión, y en el hop normal el uuid ni siquiera
		// cambia) y provocaría un salto en cadena instantáneo.
		_ = core.ClearSentinels(o.Home, prevSession)

		res.Hops = append(res.Hops, Hop{
			From: from, To: target, At: now,
			Reason:  hopReason(ev),
			Session: session,
		})
		res.Profile, res.Session = target, session
		// La traza dice «vuelta a casa» para TODO salto que termine en el primario,
		// con marcador o sin él: lo que el usuario necesita saber es dónde acabó su
		// conversación, no qué estructura interna la movió.
		r.traceHop(from, target, now.Sub(enteredAt), ev, backHome)
		if home {
			// El uuid cambió al volver: es lo que el usuario teclearía en
			// `claude --resume` si quisiera seguir a mano desde aquí. En el backHome
			// degradado lo dice adoptHome, que además aclara que no hubo préstamo.
			r.traceReturned(target, session)
		}
		enteredAt = now
	}
}

// childOutcome es lo que devuelve una vigilancia completa de un hijo.
type childOutcome struct {
	code   int
	exited bool             // salió por su cuenta (no lo matamos nosotros)
	limit  *core.LimitEvent // el límite detectado, si lo hubo

	// returnHome distingue la OTRA razón de matar al hijo: el temporizador de
	// `return_check` vio que el primario ya liberó su ventana. No es un límite
	// —el perfil actual sigue teniendo cuerda— así que el bucle no debe marcarlo
	// agotado ni tratarlo como rotación: solo cerrar el préstamo.
	returnHome bool
}

// waitResult transporta el resultado del Wait desde su goroutine.
type waitResult struct {
	code int
	err  error
}

// launchAndWatch lanza UN hijo y lo vigila hasta que muere o hasta que hay que
// matarlo por un límite. Cuando vuelve, el hijo está muerto sin excepción: esa
// es la invariante que garantiza que nunca haya dos claude vivos a la vez.
// `onLoan` dice que la conversación está PRESTADA y que el marcador vivo la
// devuelve al primario: es la condición para armar el temporizador de regreso.
func (r *runner) launchAndWatch(ctx context.Context, session string, resume, onLoan bool) (childOutcome, error) {
	o := r.o
	profile := r.chain.Current()

	ccHome, err := core.CCHome(o.Home, profile)
	if err != nil {
		return childOutcome{}, err
	}
	// El transcript se calcula ANTES de lanzar porque el watcher tiene que estar
	// escuchando desde el primer byte: CC crea el archivo al primer turno y una
	// respuesta que falle por rate limit puede llegar en segundos.
	transcript := core.ProjectDir(ccHome, core.SlugForCwd(o.Cwd)) + "/" + session + ".jsonl"

	// Línea base del transcript, tomada ANTES de lanzar: al reanudar, todo lo que
	// ya está escrito es historia (el 429 de la sesión de ayer, o el que provocó
	// el hop anterior y viajó en la copia del jsonl). Sin ella el watcher lo
	// leería como un límite de ahora y rotaría con la ventana ya reabierta.
	var baseline int64
	if resume {
		baseline = transcriptBaseline(transcript)
	}

	// El filtro de duplicados empieza limpio en cada hijo: su trabajo es que un
	// límite no cuente dos veces DENTRO de este lanzamiento (dos sensores, una
	// relectura), no impedir que el perfil siguiente reporte el suyo con el mismo
	// texto. Ver el comentario de runner.seen.
	r.seen = map[string]bool{}

	proc, err := Launch(ctx, LaunchSpec{
		Bin:      o.ClaudeBin,
		Args:     BuildArgs(session, resume, o.Headless, o.Yolo, o.Args),
		Env:      core.EnvForChild(os.Environ(), o.Home, profile, r.cfg),
		Dir:      o.Cwd,
		Headless: o.Headless,
		Stdin:    o.Stdin,
		Stdout:   o.childOut,
		Stderr:   o.childErr,
	})
	if err != nil {
		return childOutcome{}, err
	}
	r.traceLaunch(profile, session, resume)

	// Los sensores se eligen por modo, no por gusto:
	//   - headless: el stream-json es el sensor bueno (ve el api_retry en el
	//     mismo instante en que ocurre) y además hace de tee de la salida.
	//   - interactive: no hay stdout que parsear —lo tiene la TUI— así que el
	//     único reactivo es el transcript, y se añade el proactivo de uso para
	//     poder rotar ANTES de que el usuario pierda un turno.
	// El sentinel del hook StopFailure va en los dos: es la señal más fiable
	// cuando está instalada, y no cuesta nada cuando no lo está.
	since := o.Now()
	var sensors []Detector
	if o.Headless {
		sensors = append(sensors, NewStreamDetector(proc.StdoutPipe(), o.Out))
	} else {
		sensors = append(sensors, NewUsageWatcher(o.Home, profile, ccHome, r.rc.Policy.Threshold, o.Poll))
	}
	sensors = append(sensors,
		NewTranscriptWatcher(transcript, baseline, o.Poll),
		NewSentinelWatcher(o.Home, session, since, o.Poll),
	)
	det := MergeDetectors(sensors...)
	defer func() {
		_ = det.Close()
		// Cerrar el pipe desbloquea al detector de stream si seguía en un Read;
		// Launch no lo cierra a propósito (ver launcher.go) para no truncar la
		// última línea, que es justo donde llega el rate limit.
		if c, ok := proc.StdoutPipe().(io.Closer); ok {
			_ = c.Close()
		}
	}()

	waitCh := make(chan waitResult, 1)
	go func() {
		code, werr := proc.Wait()
		waitCh <- waitResult{code: code, err: werr}
	}()

	events := det.Events()
	var limit *core.LimitEvent
	var dwellTicker *time.Ticker
	var dwellC <-chan time.Time
	defer func() {
		if dwellTicker != nil {
			dwellTicker.Stop()
		}
	}()

	// Temporizador de regreso: el único vigilante que mira al PRIMARIO en vez de
	// al perfil actual. Los sensores solo saben decir «esto se agotó»; nadie
	// avisa de que una ventana ajena reabrió, así que sin este reloj la vuelta a
	// casa quedaba a expensas de que algo MÁS disparase (y a las 3am, con el
	// usuario dormido, no dispara nada: la sesión se queda en el préstamo hasta
	// la mañana).
	//
	// Cuándo se arma lo decide armReturnTicker (ver ahí las tres condiciones y su
	// porqué). El tick corre a o.Poll y el periodo real lo impone
	// `lastReturnCheck`: así el periodo es el de la política (10m por defecto) sin
	// que el reloj del bucle dependa de él, y los tests pueden mover un reloj
	// falso sin esperar minutos.
	var returnTicker *time.Ticker
	var returnC <-chan time.Time
	lastReturnCheck := o.Now()
	returnWaited, returnBusyWaited := false, false
	if armReturnTicker(onLoan, o.NoReturn, r.rc.Policy.ReturnCheck) {
		returnTicker = time.NewTicker(o.Poll)
		returnC = returnTicker.C
	}
	defer func() {
		if returnTicker != nil {
			returnTicker.Stop()
		}
	}()

	// kill cierra la vigilancia matando al hijo. `returnHome` distingue las dos
	// razones de hacerlo (rotación por límite / vuelta a casa por return_check);
	// el trato del hijo es idéntico, lo que cambia es lo que hará el bucle.
	kill := func(returnHome bool) (childOutcome, error) {
		signaled, terr := proc.Terminate(termGrace)
		if terr != nil {
			r.warnf("%v", terr)
		}
		w := <-waitCh
		// `exited` es «murió por su cuenta», no «murió»: si la señal no llegó a
		// salir es que ya estaba muerto cuando fuimos a matarlo, y eso es la
		// carrera que el switch de desenlaces documenta y quiere seguir tratando
		// como salida propia del hijo.
		return childOutcome{code: w.code, exited: !signaled, limit: limit, returnHome: returnHome}, w.err
	}

	for {
		select {
		case <-ctx.Done():
			// Cancelación (SIGTERM al supervisor, timeout del CLI): el hijo se
			// va con nosotros. Sin esto quedaría un claude huérfano escribiendo
			// en la terminal del usuario.
			out, _ := kill(false)
			return out, ctx.Err()

		case w := <-waitCh:
			if limit == nil && w.code != 0 && w.code != exitSIGINT {
				// Murió con código raro: puede que el sensor tenga la línea que
				// lo explica todavía en vuelo (ver exitDrain).
				limit = r.drainLimit(events)
			}
			return childOutcome{code: w.code, exited: true, limit: limit}, w.err

		case ev, ok := <-events:
			if !ok {
				// Todos los sensores cerraron (en la práctica solo pasa en
				// headless cuando el hijo cerró su stdout). Se anula el canal
				// para que el select no gire en vacío y se sigue esperando al
				// Wait, que es quien manda.
				events = nil
				continue
			}
			if !r.fresh(ev) {
				continue
			}
			if limit != nil {
				continue // el primer límite manda; el resto de la ráfaga es ruido
			}
			e := ev
			limit = &e
			// La permanencia exigible depende de QUIÉN avisó: entera para el
			// sensor proactivo, techo corto para los reactivos (ver DwellFor).
			if r.chain.DwellSatisfiedFor(o.Now(), r.chain.DwellFor(ev.Source)) {
				return kill(false)
			}
			// Dwell sin cumplir: se deja al hijo VIVO mientras se espera. Es
			// mejor que matarlo ya y esperar apagados — puede que termine el
			// turno en curso, y si no, al menos el usuario ve progreso.
			r.traceDwellWait(ev)
			dwellTicker = time.NewTicker(o.Poll)
			dwellC = dwellTicker.C

		case <-dwellC:
			// Mismo criterio que arriba: el evento que abrió la espera es el que
			// manda, y es el que sigue en `limit`.
			if limit != nil && r.chain.DwellSatisfiedFor(o.Now(), r.chain.DwellFor(limit.Source)) {
				return kill(false)
			}

		case <-returnC:
			now := o.Now()
			if now.Sub(lastReturnCheck) < r.rc.Policy.ReturnCheck {
				continue // el tick es de o.Poll; el periodo es el de la política
			}
			lastReturnCheck = now
			// Toda la decisión vive en decideReturn (ver ahí el porqué de cada
			// regla); aquí solo se traduce a acción sobre el hijo y a traza. El
			// reloj de las reglas de política es el inyectable (o.Now) y el de la
			// inactividad es el de PARED (time.Now), porque se compara contra el
			// mtime de un archivo.
			act, queued := r.decideReturn(events, limit, transcript, now, time.Now())
			switch act {
			case returnWaitDwell:
				// Misma cortesía que con la rotación por límite: no se arranca al
				// usuario del préstamo a los treinta segundos de llegar.
				if !returnWaited {
					returnWaited = true
					r.traceReturnDwellWait(r.chain.Primary())
				}
			case returnWaitBusy:
				if !returnBusyWaited {
					returnBusyWaited = true
					r.traceReturnBusyWait(r.chain.Primary())
				}
			case returnRotate:
				// Había un límite encolado que este mismo select podría haber
				// descartado: gana el límite (ver decideReturn). El dwell ya está
				// cumplido, así que se rota sin más espera.
				limit = queued
				return kill(false)
			case returnHomeNow:
				return kill(true)
			}
		}
	}
}

// armReturnTicker decide si este lanzamiento lleva temporizador de regreso.
//
// Está extraída —y no en línea dentro del `select`— porque es un guard SIN
// efecto observable cuando funciona: si el ticker se armara de más, decideReturn
// contestaría `returnNotYet` y no pasaría nada visible, así que ningún test de
// integración puede distinguir «no se armó» de «se armó y no hizo nada». Lo que
// sí se puede hacer es preguntarle a la condición directamente, y eso exige que
// la condición tenga nombre.
//
// Las tres condiciones, y por qué cada una:
//
//   - `onLoan`: hay marcador vivo Y su `From` es el primario. Es la única forma
//     de que exista un camino de vuelta a casa. Estando en casa no hay nada que
//     devolver; y en un préstamo DEGRADADO (rotación sin transcript ⇒ nadie abrió
//     handoff) tampoco: armarlo ahí movería la conversación por un motivo que
//     nadie pidió. Recuérdese que la ÚNICA acción de este temporizador al vencer
//     es matar al hijo (SIGTERM) para relanzarlo en otro perfil — armarlo donde
//     no toca no es un desperdicio de CPU, es apuntar a un proceso sano.
//   - `!noReturn`: `--no-return` es exactamente «no me devuelvas la sesión».
//   - `check > 0`: `return_check: 0s` es el opt-out por política; sin periodo no
//     hay sondeo posible.
func armReturnTicker(onLoan, noReturn bool, check time.Duration) bool {
	return onLoan && !noReturn && check > 0
}

// returnAction es lo que decideReturn le manda hacer a la rama del temporizador
// de regreso. Es un tipo y no tres bools porque las cinco salidas son
// EXCLUYENTES y el orden en que se evalúan es la política misma.
type returnAction int

const (
	returnNotYet    returnAction = iota // no toca (o hay una rotación ya en curso)
	returnWaitDwell                     // el primario está libre, pero min_dwell manda
	returnWaitBusy                      // el primario está libre, pero la sesión está VIVA
	returnRotate                        // había un límite encolado: gana la rotación
	returnHomeNow                       // se cumple todo: a casa
)

// decideReturn concentra la decisión del temporizador de `return_check`: qué
// hacer cuando vence el periodo y la sesión está prestada.
//
// Está extraída del select porque es la única decisión del supervisor que mata
// un proceso SANO —la rotación por límite mata uno que ya no puede trabajar— y
// eso obliga a poder probar cada regla por separado, sin lanzar nada.
//
// El orden de las reglas es deliberado:
//
//  1. límite ya desencolado -> dejar ganar la rotación pendiente. Es gratis
//     (Next() prefiere el primario, así que acabaremos en casa igual) y conserva
//     el agotamiento que este perfil acaba de reportar, que es lo que alimenta su
//     cooldown.
//  2. el primario sigue en cooldown -> nada que hacer.
//  3. min_dwell sin cumplir -> esperar; no se arranca al usuario de un préstamo
//     al que acaba de llegar.
//  4. la sesión NO está ociosa -> esperar. Ver sessionIdle: este es el guard que
//     impide terminar una conversación viva por reloj.
//  5. un límite todavía ENCOLADO -> gana el límite. Sin esta comprobación el
//     evento se perdería: matar al hijo cierra los detectores, y un LimitEvent
//     que el select no llegó a sacar del canal (la elección entre canales listos
//     es aleatoria: ~50% de las veces gana returnC) nunca pasaría por
//     MarkExhausted. El perfil que dejamos quedaría como fresco y el siguiente
//     Next() lo volvería a elegir, quemando un hop en una cuenta que sigue en 429.
//  6. todo lo demás -> a casa.
//
// `now` es el reloj de la política (inyectable en tests); `wall` es el de pared,
// el único con el que tiene sentido comparar un mtime.
func (r *runner) decideReturn(events <-chan core.LimitEvent, limit *core.LimitEvent, transcript string, now, wall time.Time) (returnAction, *core.LimitEvent) {
	if limit != nil {
		return returnNotYet, nil
	}
	if !r.chain.ReturnDue(now) {
		return returnNotYet, nil
	}
	if !r.chain.DwellSatisfied(now) {
		return returnWaitDwell, nil
	}
	if !sessionIdle(transcript, r.rc.Policy.ReturnIdle, wall) {
		return returnWaitBusy, nil
	}
	if queued := r.pendingLimit(events); queued != nil {
		return returnRotate, queued
	}
	return returnHomeNow, nil
}

// sessionIdle reporta si la conversación lleva al menos `idle` sin actividad.
//
// La señal es el mtime del transcript, y es la única disponible y barata: el
// jsonl crece con CADA turno y con cada resultado de herramienta, así que su
// mtime es el latido de la sesión. No hace falta hablar con el hijo ni con la
// tty (que en headless ni existe), y funciona igual en los dos modos — porque el
// riesgo es el mismo: da igual que no haya un humano delante, una tool call
// cortada a la mitad es igual de frágil con `-p` que sin él.
//
// Dos decisiones que parecen detalles y no lo son:
//
//   - `idle <= 0` desactiva el guard. Es el opt-out explícito de la política
//     (`return_idle: 0s`), no un accidente: quien lo escribe acepta el riesgo.
//   - un transcript que NO existe NO cuenta como ocioso. La tentación es tratarlo
//     como «silencio absoluto», pero significa lo contrario: no sabemos nada de
//     esa sesión, y matar por ignorancia es exactamente lo que este guard evita.
//     El caso legítimo de «llevo mucho rato sin transcript» no es un regreso
//     proactivo sino una rotación degradada, que va por otro camino.
//
// El `now` es de PARED a propósito (el llamador pasa time.Now(), no o.Now()): un
// mtime es una marca del reloj del sistema de archivos, y compararla contra un
// reloj inyectado daría edades absurdas —negativas o de días— en cuanto un test
// o un futuro caller mueva el reloj de la política.
func sessionIdle(transcript string, idle time.Duration, now time.Time) bool {
	if idle <= 0 {
		return true
	}
	st, err := os.Stat(transcript)
	if err != nil {
		return false
	}
	return now.Sub(st.ModTime()) >= idle
}

// pendingLimit saca del canal, SIN bloquear, el primer evento fresco que ya esté
// encolado. Devuelve nil si no hay nada (o si lo que había era repetido).
//
// El `default` es la clave: esto corre en la rama del temporizador, con el hijo
// todavía vivo, y quedarse esperando un evento que quizá no llegue nunca
// convertiría el regreso en un cuelgue.
func (r *runner) pendingLimit(events <-chan core.LimitEvent) *core.LimitEvent {
	for {
		select {
		case ev, ok := <-events:
			if !ok {
				return nil
			}
			if !r.fresh(ev) {
				continue
			}
			e := ev
			return &e
		default:
			return nil
		}
	}
}

// drainLimit espera hasta exitDrain a que un sensor entregue el evento que
// explica una muerte con código no-cero. Devuelve nil si no llega nada.
func (r *runner) drainLimit(events <-chan core.LimitEvent) *core.LimitEvent {
	if events == nil {
		return nil
	}
	t := time.NewTimer(exitDrain)
	defer t.Stop()
	for {
		select {
		case ev, ok := <-events:
			if !ok {
				return nil
			}
			if !r.fresh(ev) {
				continue
			}
			e := ev
			return &e
		case <-t.C:
			return nil
		}
	}
}

// fresh marca el evento como visto y reporta si es la PRIMERA vez que aparece.
// La clave es el contenido, no el sensor de origen: el mismo límite llega por
// transcript y por stream-json, y el del transcript vuelve a llegar tras un hop
// porque el jsonl se copia al perfil destino (ver runner.seen).
func (r *runner) fresh(ev core.LimitEvent) bool {
	key := string(ev.Window) + "\x00" +
		ev.ResetsAt.UTC().Format(time.RFC3339Nano) + "\x00" +
		strings.TrimSpace(ev.Detail)
	if r.seen[key] {
		return false
	}
	r.seen[key] = true
	return true
}

// lockedWriter serializa las escrituras de la traza y del tee del hijo.
type lockedWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (l *lockedWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.Write(p)
}

func lockWriter(w io.Writer) io.Writer {
	if w == nil {
		return io.Discard
	}
	return &lockedWriter{w: w}
}

// childWriter elige qué writer recibe el HIJO.
//
// Un *os.File va tal cual: es el único caso en que os/exec hereda el descriptor
// en vez de montar un pipe con goroutine de copia, y es justo lo que hace falta
// para que claude vea una tty de verdad. Cualquier otro writer (tests, buffers)
// se entrega YA envuelto en el mutex: ahí la copia la hace una goroutine nuestra
// que sí compite con la traza, así que el candado sigue siendo necesario. El nil
// se traduce a io.Discard, no a os.Stdout: quien no quiso salida no debe
// encontrarse con la del hijo.
func childWriter(raw, locked io.Writer) io.Writer {
	if raw == nil {
		return io.Discard
	}
	if f, ok := raw.(*os.File); ok {
		return f
	}
	return locked
}
