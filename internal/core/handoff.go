package core

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// handoff.go — orquesta el round-trip. Devuelve el string EMIT (eval-able):
// el delta de env del perfil objetivo + una línea CCP_RESUME_ID=<uuid>. La
// shell function hace `( eval "$emit"; claude --resume "$CCP_RESUME_ID" )`.

// ActiveWarnThreshold es el número de handoffs sin cerrar a partir del cual el
// forward avisa (no bloquea: el usuario decide cuántos préstamos lleva vivos).
const ActiveWarnThreshold = 5

// resumeSuffix arma el par de líneas que la función shell necesita: el uuid a
// reanudar y el modo skip-permissions. El `unset` es deliberado: sin él, un
// lanzamiento previo con --yolo en el mismo shell contaminaría el siguiente.
func resumeSuffix(sessionID string, yolo bool) string {
	s := "CCP_RESUME_ID=" + shellQuote(sessionID) + "\n"
	if yolo {
		return s + "CCP_RESUME_YOLO=1\n"
	}
	return s + "unset CCP_RESUME_YOLO\n"
}

// warnLine formatea un echo a stderr eval-able. core nunca escribe a os.Stderr
// directo: los avisos viajan dentro del emit.
//
// Los argumentos string se escapan ANTES de interpolarse: son datos ajenos
// ($PWD guardado en el marcador, nombres de perfil) y el emit se ejecuta con
// `eval` en la shell interactiva del usuario. Un directorio llamado
// `proj$(rm -rf ~)x` es un nombre legal; sin escapar sería ejecución de
// comandos, y un simple `"` rompería el parseo del emit entero (bash y zsh
// parsean todo el input antes de ejecutar nada, así que ni el `export` del
// perfil destino se aplicaría). El texto del `format` lo controla el llamador.
func warnLine(format string, args ...any) string {
	safe := make([]any, len(args))
	for i, a := range args {
		if s, ok := a.(string); ok {
			safe[i] = QuoteInDoubleQuotes(s)
			continue
		}
		safe[i] = a
	}
	return "echo \"⚠️  ccp: " + fmt.Sprintf(format, safe...) + "\" >&2\n"
}

// QuoteInDoubleQuotes neutraliza los cuatro bytes que siguen siendo activos
// dentro de "…" en bash y zsh: \ $ ` y la propia comilla. El backslash va
// primero para no re-escapar los que añaden los demás.
//
// Es la ÚNICA implementación del repo a propósito: la comparte todo lo que
// interpola datos ajenos dentro de un `echo "…"` eval-able (los avisos del
// forward aquí, el recordatorio del hook en internal/cli). Con dos copias, un
// metacarácter añadido a una dejaría la otra sin proteger, y ambos strings los
// evalúa la shell interactiva del usuario.
func QuoteInDoubleQuotes(s string) string {
	return doubleQuoteEscaper.Replace(s)
}

var doubleQuoteEscaper = strings.NewReplacer(
	`\`, `\\`,
	`$`, `\$`,
	"`", "\\`",
	`"`, `\"`,
)

// profileKind devuelve "deepseek" si el perfil es de tipo deepseek; en cualquier
// otro caso (official, default, vacío) devuelve "official".
func profileKind(name string, cfg *Config) string {
	if name != "default" {
		if p, ok := cfg.Profiles[name]; ok && p.Type == "deepseek" {
			return "deepseek"
		}
	}
	return "official"
}

// HandoffForward valida, copia la sesión origen→destino (mismo uuid), añade el
// marcador a la lista de activos y devuelve el emit con el env del DESTINO.
// v2: se permiten N handoffs en vuelo; lo que se bloquea es repetir la MISMA
// sesión (fan-out) y encadenar niveles sobre esa misma sesión.
//
// force se propaga a CopyTranscript: es el escape que el propio error de
// colisión sugiere («usa --force») cuando el destino ya tiene un jsonl con ese
// uuid y contenido distinto.
func HandoffForward(home, from, to, cwd, sessionUUID string, writeMarker, yolo, force bool, now time.Time) (string, error) {
	if from == to {
		return "", fmt.Errorf("el perfil destino es el mismo que el origen (%s)", to)
	}
	// El gate de versión va ANTES de cualquier efecto: si handoffs.yaml es de un
	// ccp más nuevo la escritura se negará igual, y descubrirlo al final dejaría
	// el jsonl ya copiado en el perfil destino sin marcador que lo referencie.
	if err := ensureHandoffsWritable(home); err != nil {
		return "", err
	}
	cfg, err := Load(home)
	if err != nil {
		return "", err
	}
	if to != "default" {
		if _, ok := cfg.Profiles[to]; !ok {
			return "", fmt.Errorf("perfil destino desconocido: %s", to)
		}
	}
	if from != "default" {
		if _, ok := cfg.Profiles[from]; !ok {
			return "", fmt.Errorf("perfil origen desconocido: %s", from)
		}
	}
	// El uuid llega crudo de `--session` (o del nombre de archivo que ofrece el
	// picker) y justo debajo se concatena para formar srcPath. Se valida antes de
	// tocar el disco: un `..` aquí copiaría un jsonl ajeno al perfil destino y
	// dejaría un marcador activo apuntando fuera del home.
	if err := validateSessionID(sessionUUID); err != nil {
		return "", err
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

	// Validar, copiar y anotar el marcador ocurren bajo el MISMO flock: entre la
	// lectura de la lista y su escritura pasa la copia del jsonl (segundos con un
	// transcript grande), y otra terminal haciendo handoff a la vez pisaría el
	// archivo con su snapshot rancio.
	var active int
	var oldest Marker
	err = UpdateHandoffs(home, func(h *Handoffs) error {
		// Invariante 1: sin cadena multi-nivel. La cadena es por SESIÓN, no por
		// repo: solo encadena re-prestar la misma sesión que ya llegó aquí de
		// otro perfil (a.To == from && a.Session == la elegida). Se comprueba
		// antes que el fan-out porque ese caso también cae en la invariante 2 y
		// el mensaje de cadena es el que dice qué hacer. Otras sesiones del mismo
		// repo —incluidas las nacidas ya en este perfil— no forman cadena y se
		// prestan con normalidad (spec §06).
		for _, a := range h.Active {
			if a.To == from && a.Session == sessionUUID {
				return fmt.Errorf("handoff encadenado no soportado: %s ya presta esta sesión a %s; haz `ccp handoff end` primero", a.From, a.To)
			}
		}
		// Invariante 2: la sesión no puede estar ya prestada (sin fan-out).
		if i := FindActiveSession(h, sessionUUID); i >= 0 {
			a := h.Active[i]
			return fmt.Errorf("esa sesión ya está en vuelo: %s → %s (desde %s); termínala con `ccp handoff end` o elige otra", a.From, a.To, a.Since)
		}
		if _, err := CopyTranscript(srcPath, ProjectDir(toCC, slug), force); err != nil {
			return err
		}
		if !writeMarker {
			return ErrHandoffsUnchanged
		}
		h.Active = append(h.Active, Marker{
			Session: sessionUUID, Slug: slug, Cwd: cwd,
			From: from, To: to,
			Title: readAITitle(srcPath),
			Since: now.UTC().Format(time.RFC3339),
		})
		active, oldest = len(h.Active), oldestActive(h)
		return nil
	})
	if err != nil {
		return "", err
	}

	// CCP_RESUME_ID se emite SIEMPRE (incluso con writeMarker=false): la shell
	// function lo necesita para `claude --resume "$CCP_RESUME_ID"`.
	emit := EnvDelta(home, to, cfg) + resumeSuffix(sessionUUID, yolo)
	// Spec §handoff: si origen y destino son de proveedores distintos, advierte
	// pero no bloquea — misma semántica que el warning de cwd-mismatch en HandoffEnd.
	if profileKind(from, cfg) != profileKind(to, cfg) {
		emit = warnLine("handoff entre proveedores distintos (%s → %s); el modelo y el formato de tools pueden diferir", from, to) + emit
	}
	if active >= ActiveWarnThreshold {
		emit = warnLine("%d handoffs sin cerrar (más viejo: %s, desde %s); revísalos con \\`ccp handoff list\\`", active, oldest.Cwd, oldest.Since) + emit
	}
	return emit, nil
}

// oldestActive devuelve el marcador activo con `since` más antiguo. Since es
// RFC3339 en UTC, así que el orden lexicográfico ES el cronológico.
func oldestActive(h *Handoffs) Marker {
	out := h.Active[0]
	for _, m := range h.Active[1:] {
		if m.Since < out.Since {
			out = m
		}
	}
	return out
}

// HandoffEnd toma el marcador resuelto (por cwd o por uuid explícito), hace
// back-sync del transcript (que creció en el destino) hacia el origen como una
// sesión NUEVA (uuid nuevo, sessionId reescrito, aiTitle prefijado con el
// origen), lo archiva y devuelve el emit con el env del ORIGEN + el uuid nuevo.
// No destructivo: ni el original del origen ni el del destino se borran.
// Si hay 2+ activos para el cwd devuelve ErrAmbiguousHandoff SIN tocar nada;
// el caller desambigua y vuelve a llamar con sessionFlag.
func HandoffEnd(home, cwd, sessionFlag string, yolo bool, now time.Time) (string, error) {
	// Gate de versión primero: sin esto el fallo llegaría tras RewriteSession, con
	// la sesión de vuelta ya escrita en el origen y el marcador todavía activo.
	if err := ensureHandoffsWritable(home); err != nil {
		return "", err
	}
	cfg, err := Load(home)
	if err != nil {
		return "", err
	}
	// Resolver, reescribir el transcript y archivar el marcador van bajo el
	// MISMO flock: la ventana entre leer y escribir contiene un RewriteSession
	// completo, y sin el lock sostenido un `end` concurrente resucitaría este
	// marcador (ya back-synced) desde su copia rancia de la lista.
	var m Marker
	var newID string
	err = UpdateHandoffs(home, func(h *Handoffs) error {
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
		return "", err
	}
	emit := EnvDelta(home, m.From, cfg) + resumeSuffix(newID, yolo)
	// Spec §07: si el cwd actual difiere del del marcador, advierte pero permite.
	// Con resolución por cwd esto solo ocurre vía --session explícito.
	if cwd != "" && cwd != m.Cwd {
		emit = warnLine("el cwd actual difiere del marcador del handoff") + emit
	}
	return emit, nil
}

// HandoffResume re-entra a un handoff en vuelo SIN cerrarlo: no copia
// transcripts, no escribe ni archiva marcador. Solo resuelve el marcador,
// verifica que el jsonl sigue en el destino y emite el env del DESTINO con el
// uuid ya prestado. Es lo que hace posible tener N handoffs vivos: sin esto,
// volver a uno exigiría terminarlo.
func HandoffResume(home, cwd, sessionFlag string, yolo bool) (string, error) {
	// Resume no escribe, pero un handoffs.yaml de versión futura se lee como
	// vacío: sin el gate el usuario vería «sin handoff activo para este proyecto»
	// y creería que perdió el marcador, en vez de saber que su ccp es viejo.
	if err := ensureHandoffsWritable(home); err != nil {
		return "", err
	}
	cfg, err := Load(home)
	if err != nil {
		return "", err
	}
	h, err := LoadHandoffs(home)
	if err != nil {
		return "", err
	}
	idx, _, err := ResolveActive(h, cwd, sessionFlag)
	if err != nil {
		return "", err
	}
	m := h.Active[idx]
	toCC, err := CCHome(home, m.To)
	if err != nil {
		return "", err
	}
	srcPath := ProjectDir(toCC, m.Slug) + "/" + m.Session + ".jsonl"
	if _, err := os.Stat(srcPath); err != nil {
		return "", fmt.Errorf("no encuentro la sesión %s en %s; el marcador queda activo (si el transcript ya no existe, descártalo)", m.Session, m.To)
	}
	emit := EnvDelta(home, m.To, cfg) + resumeSuffix(m.Session, yolo)
	if cwd != "" && cwd != m.Cwd {
		emit = warnLine("el cwd actual difiere del marcador del handoff") + emit
	}
	return emit, nil
}

// HandoffDiscard suelta un marcador SIN back-sync: lo saca de la lista de
// activos y lo archiva con `returned_as` vacío. Es la salida para un marcador
// huérfano — si el jsonl del destino desapareció (limpieza de ~/.claude,
// rotación, perfil destino borrado), `end` y `resume` fallan siempre y la
// invariante de cadena bloquea el forward inverso, así que sin esto el marcador
// quedaría activo para siempre, contando para el aviso de acumulación y
// secuestrando la resolución por cwd de ese repo.
//
// No borra ningún transcript: lo que hubiera en el destino sigue en disco y se
// puede reanudar a mano con `claude --resume <uuid>` desde ese perfil.
func HandoffDiscard(home, cwd, sessionFlag string, now time.Time) (Marker, error) {
	if err := ensureHandoffsWritable(home); err != nil {
		return Marker{}, err
	}
	var m Marker
	err := UpdateHandoffs(home, func(h *Handoffs) error {
		idx, _, err := ResolveActive(h, cwd, sessionFlag)
		if err != nil {
			return err
		}
		m = h.Active[idx]
		h.Archived = append(h.Archived, ArchivedMarker{
			Session: m.Session, From: m.From, To: m.To, Slug: m.Slug,
			Since: m.Since, Ended: now.UTC().Format(time.RFC3339),
		})
		h.Active = append(h.Active[:idx], h.Active[idx+1:]...)
		return nil
	})
	if err != nil {
		return Marker{}, err
	}
	return m, nil
}
