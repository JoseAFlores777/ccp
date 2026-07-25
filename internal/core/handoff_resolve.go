package core

import (
	"errors"
	"fmt"
	"strings"
)

// handoff_resolve.go — selección PURA del marcador sobre el que actúan `end` y
// `resume`. No hace I/O ni pregunta nada: cuando hay empate devuelve los
// candidatos y ErrAmbiguousHandoff, y la capa CLI decide (picker TUI con TTY,
// error pidiendo --session sin TTY). Ese reparto mantiene core sin presentación.

// ErrAmbiguousHandoff señala 2+ handoffs activos para el mismo cwd.
var ErrAmbiguousHandoff = errors.New("varios handoffs activos para este proyecto")

// ActiveForCwd devuelve los índices de h.Active que corresponden a ESTE
// proyecto. El criterio es el `cwd` exacto que guardó el marcador, no el slug:
// SlugForCwd aplana '/' y '-' al mismo carácter, así que /repo/foo-bar y
// /repo/foo/bar comparten slug siendo proyectos distintos, y resolver por slug
// haría que un `end` cerrara el handoff del otro repo (y con él, cambiara la
// shell al perfil equivocado). El slug queda solo como respaldo para marcadores
// que no guardaron cwd.
func ActiveForCwd(h *Handoffs, cwd string) []int {
	if cwd == "" {
		return nil
	}
	slug := SlugForCwd(cwd)
	var out []int
	for i, m := range h.Active {
		if m.Cwd != "" {
			if m.Cwd == cwd {
				out = append(out, i)
			}
			continue
		}
		if m.Slug == slug {
			out = append(out, i)
		}
	}
	return out
}

// FindActiveSession devuelve el índice del activo con ese uuid EXACTO, o -1.
// La comparación exacta es la que necesita la invariante de fan-out del
// forward; para lo que teclea el usuario está resolveSession, que además
// acepta prefijos.
func FindActiveSession(h *Handoffs, session string) int {
	for i, m := range h.Active {
		if m.Session == session {
			return i
		}
	}
	return -1
}

// ambiguousPrefix es el centinela de findActiveByPrefix cuando el prefijo
// empata con más de un activo.
const ambiguousPrefix = -2

// findActiveByPrefix devuelve el índice del ÚNICO activo cuyo uuid empieza por
// p; -1 si ninguno, ambiguousPrefix si varios.
func findActiveByPrefix(h *Handoffs, p string) int {
	found := -1
	for i, m := range h.Active {
		if strings.HasPrefix(m.Session, p) {
			if found >= 0 {
				return ambiguousPrefix
			}
			found = i
		}
	}
	return found
}

// findArchived busca un handoff ya terminado por uuid exacto o prefijo, del más
// reciente al más viejo.
func findArchived(h *Handoffs, s string) *ArchivedMarker {
	for i := len(h.Archived) - 1; i >= 0; i-- {
		if strings.HasPrefix(h.Archived[i].Session, s) {
			return &h.Archived[i]
		}
	}
	return nil
}

// resolveSession traduce lo que el usuario pasó en --session a un índice de
// h.Active. Acepta prefijos porque los hints de error muestran los uuid
// recortados (ShortUUID) y sin TTY --session es la ÚNICA salida de un end/resume
// ambiguo: si el valor que el error sugiere copiar no fuera aceptable, el camino
// de recuperación documentado no existiría.
func resolveSession(h *Handoffs, sessionFlag string) (int, error) {
	if i := FindActiveSession(h, sessionFlag); i >= 0 {
		return i, nil
	}
	switch i := findActiveByPrefix(h, sessionFlag); {
	case i == ambiguousPrefix:
		return -1, fmt.Errorf("el prefijo %s empata con varios handoffs activos; usa el uuid completo%s", sessionFlag, activeHint(h))
	case i >= 0:
		return i, nil
	}
	if a := findArchived(h, sessionFlag); a != nil {
		if a.ReturnedAs == "" {
			return -1, fmt.Errorf("ese handoff ya terminó (descartado el %s)", a.Ended)
		}
		return -1, fmt.Errorf("ese handoff ya terminó (volvió como %s el %s)", a.ReturnedAs, a.Ended)
	}
	return -1, fmt.Errorf("no hay handoff activo con la sesión %s%s", sessionFlag, activeHint(h))
}

// ResolveActive elige el marcador objetivo.
//
//	sessionFlag != ""  => por uuid o prefijo único (si ya terminó, lo dice).
//	1 activo en este cwd  => ese.
//	0 en este cwd         => error, nombrando dónde sí hay activos.
//	2+ en este cwd        => (-1, candidatos, ErrAmbiguousHandoff).
func ResolveActive(h *Handoffs, cwd, sessionFlag string) (int, []Marker, error) {
	if sessionFlag != "" {
		i, err := resolveSession(h, sessionFlag)
		if err != nil {
			return -1, nil, err
		}
		return i, nil, nil
	}
	if len(h.Active) == 0 {
		// Sin «que terminar»: ResolveActive la comparten end, resume y discard, y
		// solo end termina algo. El texto nombra el estado, no la operación.
		return -1, nil, errors.New("no hay ningún handoff activo")
	}
	idxs := ActiveForCwd(h, cwd)
	switch len(idxs) {
	case 1:
		return idxs[0], nil, nil
	case 0:
		return -1, nil, fmt.Errorf("sin handoff activo para este proyecto%s", activeHint(h))
	default:
		cands := make([]Marker, 0, len(idxs))
		for _, i := range idxs {
			cands = append(cands, h.Active[i])
		}
		return -1, cands, ErrAmbiguousHandoff
	}
}

// activeHint arma el sufijo "; activos: <cwd> <sesión> (from → to), …" que
// acompaña a los errores de resolución, para que el usuario vea qué sí hay.
func activeHint(h *Handoffs) string {
	if len(h.Active) == 0 {
		return ""
	}
	parts := make([]string, 0, len(h.Active))
	for _, m := range h.Active {
		parts = append(parts, fmt.Sprintf("%s %s (%s → %s)", m.Cwd, ShortUUID(m.Session), m.From, m.To))
	}
	return "; activos: " + strings.Join(parts, ", ")
}

// ShortUUID recorta un uuid a sus primeros 8 caracteres para display.
func ShortUUID(s string) string {
	if len(s) > 8 {
		return s[:8]
	}
	return s
}
