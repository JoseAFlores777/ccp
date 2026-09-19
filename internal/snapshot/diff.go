package snapshot

import "sort"

// ChangeKind es el tipo de cambio de un elemento entre dos estados.
type ChangeKind string

const (
	ChangeAdded    ChangeKind = "added"
	ChangeRemoved  ChangeKind = "removed"
	ChangeModified ChangeKind = "modified"
)

// Change es un elemento que cambia entre dos estados.
type Change struct {
	LPath string     `json:"lpath"`
	Kind  ChangeKind `json:"kind"`
	From  *Item      `json:"from,omitempty"`
	To    *Item      `json:"to,omitempty"`
}

// Diff dice qué cambia para ir de from a to, por ruta lógica y en orden.
// Un cambio de permisos o de clase cuenta como modificación: restaurar lo deshace.
func Diff(from, to []Item) []Change {
	a := make(map[string]Item, len(from))
	for _, it := range from {
		a[it.LPath] = it
	}
	b := make(map[string]Item, len(to))
	for _, it := range to {
		b[it.LPath] = it
	}
	var out []Change
	for l, x := range a {
		y, ok := b[l]
		switch {
		case !ok:
			out = append(out, Change{LPath: l, Kind: ChangeRemoved, From: &x})
		case x.Hash != y.Hash || x.Mode != y.Mode || x.Class != y.Class:
			out = append(out, Change{LPath: l, Kind: ChangeModified, From: &x, To: &y})
		}
	}
	for l, y := range b {
		if _, ok := a[l]; !ok {
			out = append(out, Change{LPath: l, Kind: ChangeAdded, To: &y})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].LPath < out[j].LPath })
	return out
}
