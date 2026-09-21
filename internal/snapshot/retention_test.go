package snapshot

import (
	"sort"
	"strings"
	"testing"
	"time"
)

func at(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}

// Semanas ISO de 2026: el 17 y el 18 de septiembre caen en la 38 y el 10 en la 37.
func TestRetain(t *testing.T) {
	ms := []*Manifest{
		{ID: "A", Created: at("2026-09-18T18:00:00Z")},
		{ID: "B", Created: at("2026-09-18T10:00:00Z")},
		{ID: "C", Created: at("2026-09-17T12:00:00Z")},
		{ID: "D", Created: at("2026-09-10T12:00:00Z")},
		{ID: "E", Created: at("2026-08-01T12:00:00Z")},
		{ID: "F", Created: at("2026-01-01T12:00:00Z"), Pinned: true},
		{ID: "G", Created: at("2025-12-01T12:00:00Z"), Label: "antes de migrar"},
		{ID: "H", Created: at("2025-11-01T12:00:00Z")},
	}
	keep := Retain(ms, Policy{Daily: 2, Weekly: 2, Monthly: 2})
	var got []string
	for id := range keep {
		got = append(got, id)
	}
	sort.Strings(got)
	if s := strings.Join(got, " "); s != "A C D E F G" {
		t.Fatalf("conservados = %s, quiero A C D E F G", s)
	}
}

// Con la política a cero sigue conservándose el último: podar nunca deja el
// historial vacío.
func TestRetainAlwaysKeepsLatest(t *testing.T) {
	ms := []*Manifest{{ID: "viejo", Created: at("2026-01-01T00:00:00Z")}, {ID: "nuevo", Created: at("2026-09-01T00:00:00Z")}}
	keep := Retain(ms, Policy{})
	if !keep["nuevo"] || keep["viejo"] {
		t.Fatalf("keep = %v", keep)
	}
}

func TestPrune(t *testing.T) {
	st := openTemp(t)
	var ms []*Manifest
	for i, day := range []string{"2026-09-16T12:00:00Z", "2026-09-17T12:00:00Z", "2026-09-18T12:00:00Z"} {
		m, err := Capture(st, []Source{
			src("ccp/ccp.yaml", ClassAuthored, "comun"),
			src("claude/settings.json", ClassAuthored, string(rune('a'+i))),
		}, meta(at(day)), false)
		if err != nil {
			t.Fatal(err)
		}
		ms = append(ms, m)
	}
	latest := ms[2]

	// dry-run: informa pero no toca nada.
	rep, err := Prune(st, Policy{Daily: 1}, time.Now(), 0, true)
	if err != nil || len(rep.Deleted) != 2 || rep.Kept != 1 {
		t.Fatalf("dry-run = %+v, %v", rep, err)
	}
	if list, _ := st.List(); len(list) != 3 {
		t.Fatal("el dry-run borró manifiestos")
	}

	// Con periodo de gracia, los blobs recién escritos no se tocan…
	rep, err = Prune(st, Policy{Daily: 1}, time.Now(), time.Hour, false)
	if err != nil || len(rep.Deleted) != 2 || rep.BlobsDeleted != 0 {
		t.Fatalf("prune con gracia = %+v, %v", rep, err)
	}
	// …y pasado el periodo, se recogen los que ya no usa nadie.
	rep, err = Prune(st, Policy{Daily: 1}, time.Now().Add(2*time.Hour), time.Hour, false)
	if err != nil || rep.BlobsDeleted != 2 {
		t.Fatalf("prune tras la gracia = %+v, %v", rep, err)
	}
	for _, it := range latest.Items {
		if _, err := st.GetBlob(it.Hash); err != nil {
			t.Fatalf("se borró un blob del snapshot conservado (%s): %v", it.LPath, err)
		}
	}
}

// La poda anclada solo borra lo que se enseñó: si entre el dry-run y el
// confirmar nace otro snapshot, la retención querría llevarse uno más y ese no
// se ha consentido.
func TestPruneOnlyAncla(t *testing.T) {
	st := openTemp(t)
	var ms []*Manifest
	for i, day := range []string{"2026-09-16T12:00:00Z", "2026-09-17T12:00:00Z", "2026-09-18T12:00:00Z"} {
		m, err := Capture(st, []Source{
			src("ccp/ccp.yaml", ClassAuthored, "comun"),
			src("claude/settings.json", ClassAuthored, string(rune('a'+i))),
		}, meta(at(day)), false)
		if err != nil {
			t.Fatal(err)
		}
		ms = append(ms, m)
	}
	// Lo que el modal enseñaría: se van los dos viejos.
	dry, err := PruneOnly(st, Policy{Daily: 1}, time.Now(), 0, true, nil)
	if err != nil || len(dry.Deleted) != 2 {
		t.Fatalf("dry-run = %+v, %v", dry, err)
	}
	// Entre el dry-run y el confirmar nace uno nuevo: ahora la retención se
	// llevaría también al que hasta hace un instante era el más reciente.
	if _, err := Capture(st, []Source{
		src("ccp/ccp.yaml", ClassAuthored, "comun"),
		src("claude/settings.json", ClassAuthored, "nuevo"),
	}, meta(at("2026-09-18T18:00:00Z")), false); err != nil {
		t.Fatal(err)
	}
	rep, err := PruneOnly(st, Policy{Daily: 1}, time.Now(), 0, false, dry.Deleted)
	if err != nil {
		t.Fatalf("poda anclada: %v", err)
	}
	if len(rep.Deleted) != 2 {
		t.Fatalf("borró %v, se consintieron %v", rep.Deleted, dry.Deleted)
	}
	list, _ := st.List()
	if len(list) != 2 {
		t.Fatalf("quedan %d manifiestos, se esperaban 2", len(list))
	}
	if _, err := st.LoadManifest(ms[2].ID); err != nil {
		t.Fatalf("se borró un snapshot que nadie enseñó: %v", err)
	}
	// Un id que ya no está (o que la retención ya no quiere borrar) es motivo
	// para parar: el plan mostrado ya no describe el estado del almacén.
	if _, err := PruneOnly(st, Policy{Daily: 1}, time.Now(), 0, false, []string{ms[0].ID}); err == nil {
		t.Fatal("una poda anclada a un id inexistente debería fallar")
	}
}
