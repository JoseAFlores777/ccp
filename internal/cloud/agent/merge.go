package agent

// merge.go — la reconciliación a tres bandas por ruta lógica (spec §10.3).
// Es puro a propósito: decidir qué se escribe en la configuración de alguien a
// partir de una orden que llega de fuera es la parte que hay que poder probar
// sin red, sin disco y sin reloj.

import (
	"sort"

	"github.com/JoseAFlores777/ccp/internal/snapshot"
)

// Action es lo que hay que hacer con una ruta lógica.
type Action string

const (
	// ActionTake: la orden la cambia y aquí nadie la tocó desde la base.
	ActionTake Action = "take"
	// ActionKeep: aquí cambió y la orden no la toca. No se escribe nada; se
	// informa, porque es la «deriva» que el portal enseña.
	ActionKeep Action = "keep"
	// ActionRemove: la orden la quita y aquí nadie la tocó.
	ActionRemove Action = "remove"
	// ActionConflict: cambió en los dos sitios, y distinto. No se escribe.
	ActionConflict Action = "conflict"
)

// Decision es lo que sale de reconciliar una ruta lógica. Los tres elementos
// viajan enteros porque quien la enseña necesita decir de qué a qué.
type Decision struct {
	LPath   string         `json:"lpath"`
	Action  Action         `json:"action"`
	Base    *snapshot.Item `json:"base,omitempty"`
	Local   *snapshot.Item `json:"local,omitempty"`
	Desired *snapshot.Item `json:"desired,omitempty"`
}

// Merge reconcilia a tres bandas y devuelve, ordenadas por ruta lógica, solo
// las rutas en las que hay algo que decir: si lo local ya es lo deseado no se
// emite nada, porque no hay nada que hacer ni que contar.
//
// Una orden sin base («llega a este snapshot», no «aplica estos cambios sobre
// el último») se reconcilia pasando local como base: entonces nada choca y
// todo lo deseado gana, que es justo lo que significa restaurar. La decisión
// de qué es la base vive en el agente, no aquí.
func Merge(base, local, desired []snapshot.Item) []Decision {
	b, l, d := byLPath(base), byLPath(local), byLPath(desired)
	seen := map[string]bool{}
	var out []Decision
	for _, m := range []map[string]snapshot.Item{l, d} {
		for lpath := range m {
			if seen[lpath] {
				continue
			}
			seen[lpath] = true
			if dec, ok := decide(lpath, b, l, d); ok {
				out = append(out, dec)
			}
		}
	}
	// La base también entra: una ruta que ya no está ni aquí ni en la orden no
	// pide nada, pero sin recorrerla una baja que ya se hizo en los dos sitios
	// dependería de en qué mapa apareciera.
	for lpath := range b {
		if !seen[lpath] {
			seen[lpath] = true
			if dec, ok := decide(lpath, b, l, d); ok {
				out = append(out, dec)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].LPath < out[j].LPath })
	return out
}

func byLPath(items []snapshot.Item) map[string]snapshot.Item {
	m := make(map[string]snapshot.Item, len(items))
	for _, x := range items {
		m[x.LPath] = x
	}
	return m
}

func at(m map[string]snapshot.Item, lpath string) *snapshot.Item {
	if x, ok := m[lpath]; ok {
		return &x
	}
	return nil
}

// same compara dos lados. Un cambio de modo o de clase cuenta: restaurar lo
// deshace, así que fingir que no ha pasado nada sería aplicar a medias.
func same(a, b *snapshot.Item) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Hash == b.Hash && a.Mode == b.Mode && a.Class == b.Class
}

func decide(lpath string, b, l, d map[string]snapshot.Item) (Decision, bool) {
	base, local, desired := at(b, lpath), at(l, lpath), at(d, lpath)
	dec := Decision{LPath: lpath, Base: base, Local: local, Desired: desired}
	switch {
	case same(local, desired):
		// Ya se está donde la orden pide, se llegara como se llegara.
		return Decision{}, false
	case same(local, base):
		dec.Action = ActionTake
		if desired == nil {
			dec.Action = ActionRemove
		}
	case same(desired, base):
		dec.Action = ActionKeep
	default:
		dec.Action = ActionConflict
	}
	return dec, true
}
