package core

// live.go — el estado en vivo de las sesiones supervisadas (`ccp session`).
//
// El supervisor decide en memoria: en qué cuenta está la conversación, cuáles
// están agotadas y hasta cuándo, cuántos préstamos lleva. Fuera del proceso eso
// era invisible —en disco solo quedaba el marcador del préstamo—, así que nadie
// podía contestar «¿dónde está ahora mi conversación y cuándo vuelve a casa?»
// sin mirar la terminal. Este archivo es esa foto, publicada en cada momento que
// cambia algo (arranque, lanzamiento, salto, vuelta, parada, final), para que la
// app la pinte y `ccp auto live` la cuente.
//
// Reglas:
//   - Es una caché de presentación: borrarla no cambia lo que hace el
//     supervisor, solo lo que se ve. Por eso escribirla nunca puede fallar una
//     sesión (el supervisor ignora sus errores).
//   - Una sesión que dice «running» pero cuyo proceso ya no existe se lee como
//     «lost»: el supervisor murió sin poder escribir su final (un kill -9, la
//     Mac apagada). Decir «corriendo» ahí sería la clase de falsa certeza que
//     ADR 0009 prohíbe.
//   - Los estados terminados se podan a los 7 días.

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"
)

// LiveVersion es la versión del formato. Añadir campos no la sube.
const LiveVersion = 1

// Estados de una sesión en vivo.
const (
	LiveRunning = "running" // el hijo corre (o se está relanzando)
	LiveParked  = "parked"  // todas las cuentas agotadas: salió con 75
	LiveDone    = "done"    // terminó (con el código del hijo)
	LiveFailed  = "failed"  // terminó con un error de ccp
	LiveLost    = "lost"    // decía running pero su proceso ya no existe
)

// liveKeep es cuánto se conserva una sesión terminada.
const liveKeep = 7 * 24 * time.Hour

// LiveHop es un salto ya hecho.
type LiveHop struct {
	From    string    `json:"from"`
	To      string    `json:"to"`
	At      time.Time `json:"at"`
	Reason  string    `json:"reason"`
	Session string    `json:"session"`
	Home    bool      `json:"home"` // vuelta a casa (no gasta préstamo)
}

// LiveCooldown es una cuenta agotada y el instante en que vuelve a poder usarse.
type LiveCooldown struct {
	Profile string    `json:"profile"`
	Until   time.Time `json:"until"`
}

// LiveSession es la foto de una sesión supervisada.
type LiveSession struct {
	Version   int       `json:"version"`
	ID        string    `json:"id"`
	PID       int       `json:"pid"`
	State     string    `json:"state"`
	ExitCode  int       `json:"exit_code"`
	Error     string    `json:"error,omitempty"`
	StartedAt time.Time `json:"started_at"`
	UpdatedAt time.Time `json:"updated_at"`

	Cwd      string   `json:"cwd"`
	Origin   string   `json:"origin,omitempty"` // conversación de la que se copió (--fork)
	Session  string   `json:"session"`          // uuid con el que corre ahora
	Sessions []string `json:"sessions"`         // todos los uuid que ha usado, en orden

	Primary string    `json:"primary"`
	Current string    `json:"current"`
	Since   time.Time `json:"since"` // desde cuándo está en Current
	Chain   []string  `json:"chain"` // la principal y sus respaldos, en orden

	LoansUsed   int    `json:"loans_used"`
	MaxHops     int    `json:"max_hops"`
	Policy      string `json:"policy"`
	ReturnCheck int    `json:"return_check_s"` // segundos entre comprobaciones de vuelta
	ReturnIdle  int    `json:"return_idle_s"`  // silencio exigido para volver, en segundos
	NoReturn    bool   `json:"no_return"`
	Headless    bool   `json:"headless"`

	Hops      []LiveHop      `json:"hops"`
	Cooldowns []LiveCooldown `json:"cooldowns"`
}

// LiveDir es donde viven los archivos de estado.
func LiveDir(home string) string {
	return filepath.Join(AutoStateDir(home), "live")
}

// NewLiveID da un id ordenable por arranque y único por proceso.
func NewLiveID(start time.Time, pid int) string {
	return fmt.Sprintf("%d-%d", start.UnixNano(), pid)
}

// WriteLiveSession publica la foto (tmp+rename: la app puede leer a la vez).
func WriteLiveSession(home string, s LiveSession) error {
	dir := LiveDir(home)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	s.Version = LiveVersion
	if s.Hops == nil {
		s.Hops = []LiveHop{}
	}
	if s.Cooldowns == nil {
		s.Cooldowns = []LiveCooldown{}
	}
	if s.Sessions == nil {
		s.Sessions = []string{}
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	name, err := sanitizeAutoName("sesión", s.ID)
	if err != nil {
		return err
	}
	path := filepath.Join(dir, name+".json")
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// ReadLiveSessions devuelve las sesiones conocidas, la más reciente primero.
// Un archivo ilegible se salta (es una caché); una sesión «running» cuyo
// proceso no existe sale como «lost».
func ReadLiveSessions(home string) ([]LiveSession, error) {
	entries, err := os.ReadDir(LiveDir(home))
	if errors.Is(err, os.ErrNotExist) {
		return []LiveSession{}, nil
	}
	if err != nil {
		return nil, err
	}
	out := []LiveSession{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(LiveDir(home), e.Name()))
		if err != nil {
			continue
		}
		var s LiveSession
		if json.Unmarshal(data, &s) != nil {
			continue
		}
		if s.State == LiveRunning && !pidAlive(s.PID) {
			s.State = LiveLost
		}
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].StartedAt.After(out[j].StartedAt) })
	return out, nil
}

// PruneLiveSessions borra las sesiones terminadas (o perdidas) de hace más de
// una semana. Nunca toca una que corre.
func PruneLiveSessions(home string, now time.Time) {
	list, err := ReadLiveSessions(home)
	if err != nil {
		return
	}
	for _, s := range list {
		if s.State == LiveRunning || now.Sub(s.UpdatedAt) < liveKeep {
			continue
		}
		if name, err := sanitizeAutoName("sesión", s.ID); err == nil {
			_ = os.Remove(filepath.Join(LiveDir(home), name+".json"))
		}
	}
}

// Involves dice si la sesión en vivo tiene que ver con la conversación `uuid`:
// la usó en algún momento o es la copia de ella.
func (s LiveSession) Involves(uuid string) bool {
	if uuid == "" {
		return false
	}
	if s.Origin == uuid || s.Session == uuid {
		return true
	}
	for _, x := range s.Sessions {
		if x == uuid {
			return true
		}
	}
	return false
}

// pidAlive: señal 0 a un proceso comprueba que existe sin molestarle. EPERM
// también es «existe» (es de otro usuario).
func pidAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}
