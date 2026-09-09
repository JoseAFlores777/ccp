package core

import (
	"fmt"
	"os"
	"time"
)

// handoff_chain.go — el encadenado de handoffs (lo que `ccp handoff forward`
// rechaza a propósito) y el recorte del historial.
//
// El forward manual bloquea la cadena multi-nivel porque, con un humano al
// mando, encadenar es casi siempre un error de operación: la sesión se aleja un
// salto más de su perfil origen y `handoff end` tendría que deshacer N niveles
// para devolverla. El supervisor de `ccp session` está en la situación opuesta:
// rotar de perfil en perfil ES su trabajo, y lo hace sin nadie mirando.
//
// La forma de levantar la invariante sin reintroducir el problema que motivó
// bloquearla es NO apilar niveles: el marcador se muta en sitio, conservando
// `From` (el primario) y `Since` (cuándo empezó el préstamo). Así la cadena
// personal-1 → work-1 → kimi sigue siendo UN marcador `personal-1 → kimi`, y
// `handoff end` devuelve la conversación a casa en un solo paso a las 3am, que
// es exactamente lo que se necesita cuando nadie está despierto para arreglarlo.
//
// Lo que NO se levanta es el fan-out: prestar la misma sesión desde dos perfiles
// a la vez sigue siendo un error, porque partiría la conversación en dos
// transcripts que divergen y ningún `end` podría reconciliarlos.

// HandoffChain presta la sesión de `from` a `to` para el supervisor.
//
// Si la sesión YA está prestada y su marcador apunta a `from` (a.To == from &&
// a.Session == sessionUUID) —el caso que HandoffForward rechaza con «handoff
// encadenado no soportado»— el marcador se ACTUALIZA en sitio: To = to, y `to`
// se añade a Hops. From y Since no cambian. Si no hay marcador previo, equivale
// a un HandoffForward con marcador, sembrando Auto y Hops.
//
// No devuelve emit shell: el supervisor lanza a `claude` como proceso hijo y le
// aplica el entorno con EnvForChild, así que no hay ninguna shell interactiva
// que haga `eval`.
func HandoffChain(home, from, to, cwd, sessionUUID string, auto, force bool, now time.Time) (Marker, error) {
	if from == to {
		return Marker{}, fmt.Errorf("el perfil destino es el mismo que el origen (%s)", to)
	}
	// Mismo orden que HandoffForward y por la misma razón: el gate de versión va
	// ANTES de cualquier efecto en disco. Si handoffs.yaml lo escribió un ccp más
	// nuevo, la escritura se negará igual al final, y descubrirlo entonces dejaría
	// el jsonl ya copiado en el perfil destino sin marcador que lo referencie
	// —basura permanente en el picker de ese perfil.
	if err := ensureHandoffsWritable(home); err != nil {
		return Marker{}, err
	}
	cfg, err := Load(home)
	if err != nil {
		return Marker{}, err
	}
	if to != "default" {
		if _, ok := cfg.Profiles[to]; !ok {
			return Marker{}, fmt.Errorf("perfil destino desconocido: %s", to)
		}
	}
	if from != "default" {
		if _, ok := cfg.Profiles[from]; !ok {
			return Marker{}, fmt.Errorf("perfil origen desconocido: %s", from)
		}
	}
	// El uuid se concatena justo debajo para formar srcPath. Aquí llega de la
	// política del supervisor, no del teclado, pero la validación se mantiene: el
	// supervisor lo hereda de `--session <uuid>` del usuario, y un `..` copiaría
	// un jsonl ajeno al perfil destino y dejaría un marcador apuntando fuera del
	// home.
	if err := validateSessionID(sessionUUID); err != nil {
		return Marker{}, err
	}
	slug := SlugForCwd(cwd)
	fromCC, err := CCHome(home, from)
	if err != nil {
		return Marker{}, err
	}
	toCC, err := CCHome(home, to)
	if err != nil {
		return Marker{}, err
	}
	// En el hop encadenado, `from` es el perfil que HOY tiene la sesión (el `To`
	// del marcador vivo), así que el jsonl está en SU cc-home: el transcript que
	// se copia es el que creció durante ese préstamo, no el original del primario.
	srcPath := ProjectDir(fromCC, slug) + "/" + sessionUUID + ".jsonl"
	if _, err := os.Stat(srcPath); err != nil {
		return Marker{}, fmt.Errorf("no encuentro la sesión %s en %s", sessionUUID, from)
	}

	// Validar la invariante, copiar el jsonl y mutar el marcador van bajo el MISMO
	// flock, igual que en HandoffForward: entre leer la lista y escribirla pasa una
	// copia completa del transcript (segundos si es grande), y otra terminal
	// haciendo handoff a la vez pisaría el archivo con su snapshot rancio. Un
	// marcador perdido es irrecuperable: sin from/to esa sesión ya no vuelve a casa.
	var out Marker
	err = UpdateHandoffs(home, func(h *Handoffs) error {
		chain := -1
		if i := FindActiveSession(h, sessionUUID); i >= 0 {
			a := h.Active[i]
			// El fan-out SIGUE prohibido: si la sesión está prestada a otro sitio,
			// `from` no es quien la tiene y copiar desde él duplicaría la
			// conversación en dos perfiles que ya divergieron.
			if a.To != from {
				return fmt.Errorf("esa sesión ya está en vuelo: %s → %s (desde %s); termínala con `ccp handoff end` o elige otra", a.From, a.To, a.Since)
			}
			chain = i
		}
		if _, err := CopyTranscript(srcPath, ProjectDir(toCC, slug), force); err != nil {
			return err
		}
		if chain >= 0 {
			m := &h.Active[chain]
			m.To = to
			m.Hops = append(m.Hops, to)
			// Auto solo se ENCIENDE, nunca se apaga: si el marcador nació del
			// supervisor, sigue siendo suyo aunque un hop posterior lo encadene a
			// mano — la limpieza automática debe seguir reconociéndolo como propio.
			if auto {
				m.Auto = true
			}
			// El título se deja como estaba: readAITitle sobre el transcript ya
			// crecido devolvería el aiTitle del momento, y sobrescribirlo perdería
			// el nombre con el que el usuario reconoce el préstamo en `handoff list`.
			out = *m
			return nil
		}
		out = Marker{
			Session: sessionUUID, Slug: slug, Cwd: cwd,
			From: from, To: to,
			Title: readAITitle(srcPath),
			Since: now.UTC().Format(time.RFC3339),
			Auto:  auto,
			// El primer destino ya cuenta como hop: Hops es el rastro de destinos,
			// y sembrarlo vacío haría que un marcador de un salto y uno de cero
			// saltos (imposible) fueran indistinguibles.
			Hops: []string{to},
		}
		h.Active = append(h.Active, out)
		return nil
	})
	if err != nil {
		return Marker{}, err
	}
	return out, nil
}

// HandoffEndSession es el núcleo de HandoffEnd sin presentación: resuelve el
// marcador (por cwd o por uuid explícito), hace el back-sync del transcript
// hacia el perfil ORIGEN como una sesión NUEVA (uuid nuevo, sessionId reescrito,
// aiTitle prefijado con el destino) y archiva el marcador. Devuelve el marcador
// cerrado y el uuid nuevo en el origen.
//
// Existe separada porque el supervisor necesita exactamente esto y nada más: no
// tiene shell a la que emitirle un delta de entorno, pero sí necesita saber a qué
// uuid seguir (el nuevo) cuando la rotación vuelve al primario. HandoffEnd queda
// como el envoltorio que añade el emit eval-able.
//
// No destructivo: ni el transcript del origen ni el del destino se borran. Si hay
// 2+ activos para el cwd devuelve ErrAmbiguousHandoff SIN tocar nada.
func HandoffEndSession(home, cwd, sessionFlag string, now time.Time) (Marker, string, error) {
	// Gate de versión primero: sin esto el fallo llegaría tras RewriteSession, con
	// la sesión de vuelta ya escrita en el origen y el marcador todavía activo.
	if err := ensureHandoffsWritable(home); err != nil {
		return Marker{}, "", err
	}
	// Resolver, reescribir el transcript y archivar el marcador van bajo el
	// MISMO flock: la ventana entre leer y escribir contiene un RewriteSession
	// completo, y sin el lock sostenido un `end` concurrente resucitaría este
	// marcador (ya back-synced) desde su copia rancia de la lista.
	var m Marker
	var newID string
	err := UpdateHandoffs(home, func(h *Handoffs) error {
		idx, _, err := ResolveActive(h, cwd, sessionFlag)
		if err != nil {
			return err
		}
		m = h.Active[idx]

		toCC, err := CCHome(home, m.To)
		if err != nil {
			return err
		}
		fromCC, err := CCHome(home, m.From)
		if err != nil {
			return err
		}
		srcPath := ProjectDir(toCC, m.Slug) + "/" + m.Session + ".jsonl"
		if _, err := os.Stat(srcPath); err != nil {
			return fmt.Errorf("no encuentro la sesión %s en %s; el marcador queda activo (si el transcript ya no existe, descártalo)", m.Session, m.To)
		}
		newID, err = NewUUID()
		if err != nil {
			return err
		}
		dstPath := ProjectDir(fromCC, m.Slug) + "/" + newID + ".jsonl"
		if err := RewriteSession(srcPath, dstPath, m.Session, newID, m.To); err != nil {
			return err // RewriteSession ya validó; no se archiva el marcador
		}

		h.Archived = append(h.Archived, ArchivedMarker{
			Session: m.Session, From: m.From, To: m.To, Slug: m.Slug,
			ReturnedAs: newID, Since: m.Since, Ended: now.UTC().Format(time.RFC3339),
		})
		h.Active = append(h.Active[:idx], h.Active[idx+1:]...)
		return nil
	})
	if err != nil {
		return Marker{}, "", err
	}
	return m, newID, nil
}

// HandoffAdoptHome lleva al perfil `to` una conversación que vive en `from` SIN
// marcador de préstamo, como sesión NUEVA (uuid nuevo, sessionId reescrito,
// aiTitle prefijado con `from`). Devuelve el uuid nuevo.
//
// Es la vuelta a casa del caso DEGRADADO del supervisor: cuando la rotación
// ocurrió antes de que existiera el jsonl no hubo nada que prestar, así que no
// hay marcador — pero la conversación puede haber NACIDO durante ese préstamo, y
// entonces sí hay algo que traer de vuelta. Sin esta función el supervisor solo
// tenía dos herramientas para volver al primario y las dos eran incorrectas
// aquí: HandoffEndSession exige un marcador vivo (y sin él revienta), y
// HandoffChain crearía un marcador ACTIVO INVERTIDO {From: préstamo, To:
// primario} que nunca se cierra estando en casa — con el efecto perverso de que
// el siguiente `end` back-sincronizaría la conversación HACIA el perfil prestado
// y le diría al usuario que su sesión vive donde no está.
//
// Deliberadamente NO toca handoffs.yaml: no abre marcador (no hay préstamo que
// cerrar después) ni escribe en `archived` (el historial responde «¿con qué uuid
// volvió aquel préstamo?», y aquí no hubo préstamo registrado). Lo único que
// hace es mover la conversación y devolver el uuid con el que se reanuda, que es
// lo que el supervisor tiene que poder decirle al usuario.
//
// No destructivo: el transcript de `from` se conserva.
func HandoffAdoptHome(home, from, to, cwd, sessionUUID string) (string, error) {
	if from == to {
		return "", fmt.Errorf("el perfil destino es el mismo que el origen (%s)", to)
	}
	cfg, err := Load(home)
	if err != nil {
		return "", err
	}
	for _, name := range []string{from, to} {
		if name == "default" {
			continue
		}
		if _, ok := cfg.Profiles[name]; !ok {
			return "", fmt.Errorf("perfil desconocido: %s", name)
		}
	}
	// El uuid se concatena para formar srcPath: mismo saneado que HandoffChain,
	// por la misma razón (llega heredado de `--session <uuid>` del usuario).
	if err := validateSessionID(sessionUUID); err != nil {
		return "", err
	}

	// Invariante: adoptar es EXCLUSIVO de «no hay marcador». Si lo hay, la sesión
	// está prestada de verdad y lo que toca es HandoffEndSession, que además
	// archiva. Adoptar por encima de un marcador vivo dejaría la conversación
	// duplicada y el marcador apuntando a una copia congelada.
	h, err := LoadHandoffs(home)
	if err != nil {
		return "", err
	}
	if i := FindActiveSession(h, sessionUUID); i >= 0 {
		return "", fmt.Errorf("la sesión %s tiene un handoff activo (%s → %s); ciérrala con `ccp handoff end`",
			sessionUUID, h.Active[i].From, h.Active[i].To)
	}

	slug := SlugForCwd(cwd)
	fromCC, err := CCHome(home, from)
	if err != nil {
		return "", err
	}
	toCC, err := CCHome(home, to)
	if err != nil {
		return "", err
	}
	srcPath := ProjectDir(fromCC, slug) + "/" + sessionUUID + ".jsonl"
	if _, err := os.Stat(srcPath); err != nil {
		return "", fmt.Errorf("no encuentro la sesión %s en %s", sessionUUID, from)
	}
	newID, err := NewUUID()
	if err != nil {
		return "", err
	}
	dstPath := ProjectDir(toCC, slug) + "/" + newID + ".jsonl"
	if err := RewriteSession(srcPath, dstPath, sessionUUID, newID, from); err != nil {
		return "", err
	}
	return newID, nil
}

// HandoffPrune recorta h.Archived a los `keep` más recientes y devuelve cuántas
// entradas quitó. keep == 0 borra todo el historial.
//
// Hace falta porque `archived` no tiene tope: cada handoff que termina añade una
// entrada y nada la quita nunca. Con el supervisor rotando solo, un día de
// trabajo puede dejar decenas de entradas —el archivo crece sin límite y
// `handoff list` se vuelve ilegible— cuando el historial solo interesa para
// responder «¿con qué uuid volvió aquella sesión?», una pregunta que caduca.
//
// Se conservan las MÁS RECIENTES: Archived está en orden de inserción (las
// nuevas al final), así que el recorte es por la cabeza.
func HandoffPrune(home string, keep int) (int, error) {
	if keep < 0 {
		return 0, fmt.Errorf("keep no puede ser negativo: %d", keep)
	}
	// El gate explícito es necesario aunque prune no tenga efectos previos en
	// disco: un handoffs.yaml de versión futura se LEE como vacío, así que sin él
	// el camino «nada que recortar» devolvería 0 y nil, y el usuario creería que
	// su historial ya estaba limpio en vez de saber que su ccp es viejo.
	if err := ensureHandoffsWritable(home); err != nil {
		return 0, err
	}
	var removed int
	err := UpdateHandoffs(home, func(h *Handoffs) error {
		if keep >= len(h.Archived) {
			removed = 0
			// Sin cambios reales no se reescribe el archivo: un prune no-op no debe
			// tocar el mtime ni arriesgar una escritura por nada.
			return ErrHandoffsUnchanged
		}
		removed = len(h.Archived) - keep
		if keep == 0 {
			h.Archived = nil
			return nil
		}
		// Copia a un slice nuevo en vez de re-slicear: h.Archived se serializa a
		// continuación y quedarse con la cola del array original mantendría vivas
		// en memoria las entradas descartadas sin ninguna ganancia.
		h.Archived = append([]ArchivedMarker(nil), h.Archived[removed:]...)
		return nil
	})
	if err != nil {
		return 0, err
	}
	return removed, nil
}
