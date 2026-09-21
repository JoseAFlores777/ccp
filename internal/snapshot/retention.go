package snapshot

import (
	"errors"
	"fmt"
	"sort"
	"time"
)

// Policy dice cuántos periodos de cada tipo conservar.
type Policy struct {
	Daily   int
	Weekly  int
	Monthly int
}

// DefaultPolicy: 7 días, 4 semanas y 6 meses (spec §8.3).
var DefaultPolicy = Policy{Daily: 7, Weekly: 4, Monthly: 6}

// Retain devuelve los ids que se conservan. Recorre del más nuevo al más viejo
// y, por cada tipo de periodo, se queda con el primero (el más reciente) de cada
// uno de los últimos N periodos que tengan snapshots: una semana sin snapshots
// no gasta un hueco. Siempre se conserva el más reciente y todo lo fijado o
// etiquetado.
func Retain(ms []*Manifest, p Policy) map[string]bool {
	sorted := append([]*Manifest(nil), ms...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Created.After(sorted[j].Created) })
	keep := map[string]bool{}
	if len(sorted) > 0 {
		keep[sorted[0].ID] = true
	}
	for _, m := range sorted {
		if m.Pinned || m.Label != "" {
			keep[m.ID] = true
		}
	}
	bucket := func(n int, key func(time.Time) string) {
		last, count := "", 0
		for _, m := range sorted {
			if count >= n {
				return
			}
			k := key(m.Created.UTC())
			if k == last {
				continue
			}
			keep[m.ID] = true
			last = k
			count++
		}
	}
	bucket(p.Daily, func(t time.Time) string { return t.Format("2006-01-02") })
	bucket(p.Weekly, func(t time.Time) string { y, w := t.ISOWeek(); return fmt.Sprintf("%d-W%02d", y, w) })
	bucket(p.Monthly, func(t time.Time) string { return t.Format("2006-01") })
	return keep
}

// PruneReport resume una poda.
type PruneReport struct {
	Deleted      []string
	Kept         int
	BlobsDeleted int
}

// Prune borra los manifiestos que Retain no conserva y después los blobs que ya
// no referencia ninguno de los que quedan. Un blob modificado hace menos de
// grace no se toca aunque parezca huérfano: puede ser de una captura en curso
// que aún no escribió su manifiesto.
func Prune(st *Store, p Policy, now time.Time, grace time.Duration, dryRun bool) (PruneReport, error) {
	return PruneOnly(st, p, now, grace, dryRun, nil)
}

// ErrPruneStale dice que el plan que se confirmó ya no describe el almacén.
var ErrPruneStale = errors.New("snapshot: el plan de poda ya no vale")

// PruneOnly es Prune anclada a los ids de un dry-run previo: borra solo esos y
// deja intacto lo demás aunque la retención, recalculada ahora, quisiera
// llevarse más. Entre enseñar el plan y confirmarlo puede nacer otro snapshot
// (una captura diaria, la foto de seguridad de otra operación) y ese ocuparía
// el hueco diario, dejando sin razones al que lo tenía: se borraría un
// manifiesto que nadie llegó a ver en el diálogo. Si alguno de los ids ya no
// está o ha dejado de sobrar, no se borra nada: el plan mostrado mintió y toca
// volver a calcularlo. only nil = sin ancla (el camino de la CLI).
func PruneOnly(st *Store, p Policy, now time.Time, grace time.Duration, dryRun bool, only []string) (PruneReport, error) {
	var rep PruneReport
	ms, err := st.List()
	if err != nil {
		return rep, err
	}
	keep := Retain(ms, p)
	if only != nil {
		want := map[string]bool{}
		for _, id := range only {
			want[id] = true
		}
		have := map[string]bool{}
		for _, m := range ms {
			if !keep[m.ID] {
				have[m.ID] = true
			}
		}
		for id := range want {
			if !have[id] {
				return rep, fmt.Errorf("%w: %s ya no está entre los que sobran", ErrPruneStale, id)
			}
		}
		// Lo que sobra pero no se consintió se conserva: caerá en la próxima poda.
		for id := range have {
			if !want[id] {
				keep[id] = true
			}
		}
	}
	live := map[string]bool{}
	for _, m := range ms {
		if keep[m.ID] {
			rep.Kept++
			for _, it := range m.Items {
				live[it.Hash] = true
			}
			continue
		}
		rep.Deleted = append(rep.Deleted, m.ID)
		if !dryRun {
			if err := st.DeleteManifest(m.ID); err != nil {
				return rep, err
			}
		}
	}
	blobs, err := st.blobs()
	if err != nil {
		return rep, err
	}
	for h, mtime := range blobs {
		if live[h] || now.Sub(mtime) < grace {
			continue
		}
		rep.BlobsDeleted++
		if !dryRun {
			if err := st.deleteBlob(h); err != nil {
				return rep, err
			}
		}
	}
	return rep, nil
}
