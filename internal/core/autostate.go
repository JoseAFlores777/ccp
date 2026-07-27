package core

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// autostate.go — estado volátil de los sensores del auto-handoff, en disco bajo
// <home>/state/auto/.
//
// Vive fuera de ccp.yaml y de handoffs.yaml a propósito: son muestras que se
// reescriben cada pocos segundos (statusLine) y señales de un solo uso
// (sentinels del hook). Meterlas en un YAML compartido obligaría a tomar el
// flock global en cada refresco del statusLine — es decir, a bloquear la barra
// de estado de CC detrás de cualquier `ccp` en curso.
//
// Formato: un JSON por archivo, escritura atómica tmp+rename dentro del mismo
// directorio (rename entre directorios distintos no es atómico si cruzan
// sistemas de archivos). Los lectores toleran archivos a medio escribir por
// otros procesos saltándoselos, no fallando.

// autoNameUnsafe casa todo lo que NO es [a-zA-Z0-9._-].
//
// Perfil y sesión llegan por stdin de un hook de Claude Code: son datos ajenos
// que acaban concatenados en un path. Sin este filtro, un `session` de
// "../../../.ssh/authorized_keys" escribiría fuera de AutoStateDir. El filtro es
// allow-list (no blacklist de "..") porque enumerar lo peligroso siempre se
// queda corto.
var autoNameUnsafe = regexp.MustCompile(`[^a-zA-Z0-9._-]`)

// autoDotRun casa cualquier carrera de dos o más puntos. El punto está
// permitido (los nombres de perfil y los uuids lo usan), pero ".." es el
// componente «padre» de un path: aunque tras el filtro ya no queden separadores,
// se colapsa para que ningún nombre generado contenga jamás la secuencia.
var autoDotRun = regexp.MustCompile(`\.{2,}`)

// autoNameMax acota el largo del componente para no chocar con el límite de
// nombre de archivo (255 bytes en la mayoría de FS) al concatenar ts+sesión+perfil.
const autoNameMax = 96

// sanitizeAutoName devuelve un componente de path inocuo, o error si el valor de
// entrada está vacío.
//
// El "." solitario y las carreras de puntos se sustituyen: sobreviven al filtro
// de caracteres (el punto está permitido) pero como componente de path
// significan «este directorio» y «el padre», que es exactamente el escape que se
// quiere evitar.
func sanitizeAutoName(kind, s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", fmt.Errorf("%s vacío: no se puede formar el nombre de archivo del estado auto", kind)
	}
	out := autoDotRun.ReplaceAllString(autoNameUnsafe.ReplaceAllString(s, "_"), "_")
	if out == "." {
		out = "_"
	}
	if len(out) > autoNameMax {
		out = out[:autoNameMax]
	}
	return out, nil
}

// AutoStateDir es <home>/state/auto.
func AutoStateDir(home string) string {
	return filepath.Join(home, "state", "auto")
}

// rateLimitsDir y sentinelsDir separan las dos clases de estado: las muestras se
// sobrescriben por perfil (una por perfil, siempre la última) y los sentinels se
// acumulan hasta que alguien los consume.
func rateLimitsDir(home string) string { return filepath.Join(AutoStateDir(home), "rate-limits") }
func sentinelsDir(home string) string  { return filepath.Join(AutoStateDir(home), "sentinels") }

// rateLimitsFile es el formato en disco de una muestra. `sampled_at` va dentro
// del archivo y no se deduce del mtime porque el mtime lo pisa cualquier copia,
// backup o restauración del directorio de estado.
type rateLimitsFile struct {
	SampledAt time.Time  `json:"sampled_at"`
	Limits    RateLimits `json:"limits"`
}

// writeAtomicJSON serializa v y lo deja en path con tmp+rename.
//
// El tmp lleva sufijo aleatorio para que dos escritores concurrentes (dos
// terminales con el mismo perfil refrescando su statusLine) no se pisen el
// archivo temporal: el rename final es el que arbitra, y el último gana.
func writeAtomicJSON(path string, v any) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("no se pudo crear %s: %w", dir, err)
	}
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("no se pudo serializar %s: %w", path, err)
	}
	data = append(data, '\n')
	tmp := path + "." + randToken() + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("no se pudo escribir %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("no se pudo renombrar %s -> %s: %w", tmp, path, err)
	}
	return nil
}

// randToken da 8 hex aleatorios. Si crypto/rand falla (no debería), cae al
// reloj: la unicidad se degrada pero nada revienta, que es lo que importa en un
// camino que corre desde un hook.
func randToken() string {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%08x", time.Now().UnixNano()&0xffffffff)
	}
	return hex.EncodeToString(b[:])
}

// WriteRateLimits persiste la última muestra del statusLine de un perfil.
func WriteRateLimits(home, profile string, rl RateLimits, now time.Time) error {
	name, err := sanitizeAutoName("perfil", profile)
	if err != nil {
		return err
	}
	if now.IsZero() {
		now = time.Now()
	}
	return writeAtomicJSON(filepath.Join(rateLimitsDir(home), name+".json"), rateLimitsFile{
		SampledAt: now.UTC(),
		Limits:    rl,
	})
}

// ReadRateLimits devuelve la muestra y su antigüedad. ok=false si no hay.
//
// Nunca devuelve error: el llamador (supervisor, `ccp auto status`) trata la
// ausencia y la corrupción igual — no hay dato fiable, se degrada al respaldo
// de ReadCachedUsage.
func ReadRateLimits(home, profile string) (rl RateLimits, sampled time.Time, ok bool) {
	name, err := sanitizeAutoName("perfil", profile)
	if err != nil {
		return RateLimits{}, time.Time{}, false
	}
	data, err := os.ReadFile(filepath.Join(rateLimitsDir(home), name+".json"))
	if err != nil {
		return RateLimits{}, time.Time{}, false
	}
	var f rateLimitsFile
	if json.Unmarshal(data, &f) != nil {
		return RateLimits{}, time.Time{}, false
	}
	return f.Limits, f.SampledAt, true
}

// Sentinel es la señal que deja el hook StopFailure.
type Sentinel struct {
	Profile string     `json:"profile"`
	Session string     `json:"session"`
	Event   LimitEvent `json:"event"`
	At      time.Time  `json:"at"`
}

// sentinelStampLayout es el formato del prefijo temporal del nombre de archivo.
// Ancho fijo y UTC para que el orden lexicográfico coincida con el cronológico:
// así un `ls` ya sale ordenado, ReadSentinels puede DECIDIR SIN ABRIR el archivo
// si le interesa, y la reclamación por edad se hace con el nombre en la mano.
const sentinelStampLayout = "20060102T150405.000000000Z"

// sentinelRetention es la vida máxima de un sentinel en disco.
//
// Hace falta porque el hook StopFailure escribe uno en CUALQUIER `claude` del
// perfil —haya supervisor o no— y el único borrado explícito es el del bloque de
// rotación: sin caducidad, el directorio crece para siempre (prosa de errores
// retenida indefinidamente) y cada poll del supervisor lo recorre entero. Un día
// sobra: un sentinel solo lo consume el supervisor que lo provocó, en segundos.
const sentinelRetention = 24 * time.Hour

// sentinelStamp formatea el instante como prefijo de nombre de archivo.
func sentinelStamp(t time.Time) string {
	return t.UTC().Format(sentinelStampLayout)
}

// sentinelNameTime lee el instante del NOMBRE del archivo. ok=false si el nombre
// no sigue la convención (un archivo puesto a mano): esos se deciden leyendo el
// contenido, como siempre.
func sentinelNameTime(name string) (time.Time, bool) {
	i := strings.IndexByte(name, '-')
	if i <= 0 {
		return time.Time{}, false
	}
	t, err := time.Parse(sentinelStampLayout, name[:i])
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

// pruneSentinels borra las señales anteriores a `cutoff` decidiendo por el
// NOMBRE, sin abrir ningún archivo. Best-effort a propósito: corre desde un hook
// y ningún fallo de limpieza puede impedir escribir la señal que sí importa.
func pruneSentinels(dir string, cutoff time.Time) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		at, ok := sentinelNameTime(e.Name())
		if !ok || !at.Before(cutoff) {
			continue
		}
		_ = os.Remove(filepath.Join(dir, e.Name()))
	}
}

// WriteSentinel deja la señal del hook. El nombre de archivo se sanitiza:
// perfil y sesión pasan por un filtro [a-zA-Z0-9._-] para que un valor ajeno no
// escriba fuera de AutoStateDir.
//
// El nombre incluye un token aleatorio: dos límites en el mismo nanosegundo son
// improbables, pero un test (o un `now` fijo) sí puede repetir el instante, y
// perder señales silenciosamente por colisión de nombre sería el peor fallo
// posible en un detector.
func WriteSentinel(home string, s Sentinel) error {
	prof, err := sanitizeAutoName("perfil", s.Profile)
	if err != nil {
		return err
	}
	sess, err := sanitizeAutoName("sesión", s.Session)
	if err != nil {
		return err
	}
	if s.At.IsZero() {
		s.At = time.Now()
	}
	s.At = s.At.UTC()
	// La reclamación va ANTES de escribir, y con el cutoff medido desde el
	// instante de ESTA señal: así una señal fechada hacia atrás (un test, un reloj
	// movido) no se borra a sí misma nada más nacer.
	pruneSentinels(sentinelsDir(home), s.At.Add(-sentinelRetention))
	name := sentinelStamp(s.At) + "-" + sess + "-" + prof + "-" + randToken() + ".json"
	return writeAtomicJSON(filepath.Join(sentinelsDir(home), name), s)
}

// ReadSentinels devuelve las señales posteriores a `since` (orden cronológico).
//
// Tolerante por diseño: un archivo ilegible, a medio escribir o con JSON basura
// se salta en silencio. El productor es un hook que puede morir a mitad y el
// consumidor es el supervisor, que no puede abortar la sesión del usuario
// porque un archivo de estado quedó truncado.
func ReadSentinels(home string, since time.Time) ([]Sentinel, error) {
	entries, err := os.ReadDir(sentinelsDir(home))
	if err != nil {
		// Directorio ausente == todavía no hubo señales; no es un fallo.
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("no se pudo leer el directorio de sentinels: %w", err)
	}
	type entry struct {
		s    Sentinel
		name string
	}
	var found []entry
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		// Descarte por NOMBRE antes de tocar el disco: el prefijo es el mismo
		// instante que el `At` del contenido, así que un sentinel viejo se salta sin
		// un open/read/unmarshal. Importa porque esto lo llama el supervisor cada
		// poll (2s) durante toda la sesión, y sin el atajo el coste crece con el
		// historial en vez de con lo nuevo. Un nombre no reconocible NO se salta: se
		// decide leyéndolo, como siempre.
		if at, ok := sentinelNameTime(e.Name()); ok && !at.After(since) {
			continue
		}
		var s Sentinel
		data, err := os.ReadFile(filepath.Join(sentinelsDir(home), e.Name()))
		if err != nil || json.Unmarshal(data, &s) != nil {
			continue
		}
		if !s.At.After(since) {
			continue
		}
		found = append(found, entry{s: s, name: e.Name()})
	}
	// El orden lo manda el `At` del contenido, no el nombre: el nombre es solo
	// un índice conveniente y puede haber sido truncado por autoNameMax.
	sort.SliceStable(found, func(i, j int) bool {
		if found[i].s.At.Equal(found[j].s.At) {
			return found[i].name < found[j].name
		}
		return found[i].s.At.Before(found[j].s.At)
	})
	out := make([]Sentinel, 0, len(found))
	for _, f := range found {
		out = append(out, f.s)
	}
	return out, nil
}

// ClearSentinels borra las señales de una sesión (limpieza al terminar un hop).
//
// El emparejamiento va por el campo `session` del contenido, no por el nombre:
// el nombre lleva la versión saneada y truncada, que puede colisionar entre
// sesiones distintas. Solo si el archivo no se deja leer se recurre al nombre,
// para que la basura corrupta de esta sesión no se quede ahí para siempre.
func ClearSentinels(home, session string) error {
	sess, err := sanitizeAutoName("sesión", session)
	if err != nil {
		return err
	}
	dir := sentinelsDir(home)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("no se pudo leer el directorio de sentinels: %w", err)
	}
	var firstErr error
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		match := false
		if data, err := os.ReadFile(path); err == nil {
			var s Sentinel
			if json.Unmarshal(data, &s) == nil {
				match = s.Session == session
			} else {
				match = strings.Contains(e.Name(), "-"+sess+"-")
			}
		} else {
			match = strings.Contains(e.Name(), "-"+sess+"-")
		}
		if !match {
			continue
		}
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) && firstErr == nil {
			firstErr = fmt.Errorf("no se pudo borrar %s: %w", path, err)
		}
	}
	return firstErr
}
