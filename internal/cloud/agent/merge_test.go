package agent

import (
	"math/rand"
	"strconv"
	"testing"

	"github.com/JoseAFlores777/ccp/internal/snapshot"
)

// it construye un elemento de manifiesto con el hash que se le pasa: en estos
// tests el hash ES el contenido, no hace falta más.
func it(lpath, hash string) snapshot.Item {
	return snapshot.Item{LPath: lpath, Hash: hash, Mode: 0o644, Class: snapshot.ClassAuthored}
}

func items(xs ...snapshot.Item) []snapshot.Item { return xs }

// only devuelve la decisión de lpath, o falla: cada test mira una ruta.
func only(t *testing.T, ds []Decision, lpath string) Decision {
	t.Helper()
	for _, d := range ds {
		if d.LPath == lpath {
			return d
		}
	}
	t.Fatalf("no hay decisión para %s; hay %v", lpath, ds)
	return Decision{}
}

func TestMergeAplicaLoQueNadieTocoAqui(t *testing.T) {
	ds := Merge(
		items(it("claude/CLAUDE.md", "a")),
		items(it("claude/CLAUDE.md", "a")),
		items(it("claude/CLAUDE.md", "b")),
	)
	if d := only(t, ds, "claude/CLAUDE.md"); d.Action != ActionTake {
		t.Fatalf("esperaba take, salió %s", d.Action)
	}
}

func TestMergeConservaLoLocalCuandoLaOrdenNoLoToca(t *testing.T) {
	ds := Merge(
		items(it("claude/CLAUDE.md", "a")),
		items(it("claude/CLAUDE.md", "mío")),
		items(it("claude/CLAUDE.md", "a")),
	)
	if d := only(t, ds, "claude/CLAUDE.md"); d.Action != ActionKeep {
		t.Fatalf("esperaba keep, salió %s", d.Action)
	}
}

func TestMergeChocaCuandoCambianLosDos(t *testing.T) {
	ds := Merge(
		items(it("claude/settings.json", "a")),
		items(it("claude/settings.json", "mío")),
		items(it("claude/settings.json", "suyo")),
	)
	d := only(t, ds, "claude/settings.json")
	if d.Action != ActionConflict {
		t.Fatalf("esperaba conflict, salió %s", d.Action)
	}
	if d.Base == nil || d.Local == nil || d.Desired == nil {
		t.Fatal("un conflicto tiene que traer los tres lados para poder enseñarlo")
	}
}

// Los dos llegaron al mismo sitio por su cuenta: no hay nada que hacer y
// tampoco nada que contar. Emitirlo llenaría la revisión de ruido.
func TestMergeCallaCuandoLosDosYaCoinciden(t *testing.T) {
	ds := Merge(
		items(it("claude/CLAUDE.md", "a")),
		items(it("claude/CLAUDE.md", "b")),
		items(it("claude/CLAUDE.md", "b")),
	)
	if len(ds) != 0 {
		t.Fatalf("esperaba silencio, salió %v", ds)
	}
}

func TestMergeAltaYBaja(t *testing.T) {
	ds := Merge(
		items(it("claude/viejo.md", "a")),
		items(it("claude/viejo.md", "a")),
		items(it("claude/nuevo.md", "n")),
	)
	if d := only(t, ds, "claude/nuevo.md"); d.Action != ActionTake || d.Base != nil {
		t.Fatalf("un alta es take sin base: %+v", d)
	}
	if d := only(t, ds, "claude/viejo.md"); d.Action != ActionRemove || d.Desired != nil {
		t.Fatalf("una baja es remove sin deseado: %+v", d)
	}
}

// Un cambio de permisos o de clase es un cambio: restaurar lo deshace, así que
// la reconciliación tiene que verlo igual que ve un cambio de contenido.
func TestMergeVeElModoYLaClase(t *testing.T) {
	base := it("ccp/profiles/x/api_key", "k")
	base.Class, base.Mode = snapshot.ClassSecret, 0o600
	desired := base
	desired.Mode = 0o644
	ds := Merge(items(base), items(base), items(desired))
	if d := only(t, ds, base.LPath); d.Action != ActionTake {
		t.Fatalf("esperaba take, salió %s", d.Action)
	}
}

// Una orden sin base es un restore: el agente pasa lo local como base y
// entonces nada puede chocar.
func TestMergeSinBaseEsUnRestore(t *testing.T) {
	local := items(it("claude/CLAUDE.md", "mío"), it("claude/settings.json", "s"))
	ds := Merge(local, local, items(it("claude/CLAUDE.md", "suyo"), it("claude/settings.json", "s")))
	if len(ds) != 1 {
		t.Fatalf("esperaba una sola decisión, salieron %v", ds)
	}
	if ds[0].Action != ActionTake {
		t.Fatalf("esperaba take, salió %s", ds[0].Action)
	}
}

func TestMergeVieneOrdenado(t *testing.T) {
	ds := Merge(nil, nil, items(it("z", "1"), it("a", "1"), it("m", "1")))
	if len(ds) != 3 || ds[0].LPath != "a" || ds[1].LPath != "m" || ds[2].LPath != "z" {
		t.Fatalf("esperaba a, m, z: %v", ds)
	}
}

// Propiedades del merge (spec §10.5.9). Con semilla fija: un fallo se
// reproduce, que es lo que hace útil un test de propiedad.
func randomSides(rnd *rand.Rand) (base, local, desired []snapshot.Item) {
	vals := []string{"", "a", "b", "c"}
	for i := 0; i < 8; i++ {
		lpath := "claude/f" + strconv.Itoa(i) + ".md"
		add := func(dst *[]snapshot.Item) {
			if v := vals[rnd.Intn(len(vals))]; v != "" {
				*dst = append(*dst, it(lpath, v))
			}
		}
		add(&base)
		add(&local)
		add(&desired)
	}
	return base, local, desired
}

func TestMergePropiedades(t *testing.T) {
	rnd := rand.New(rand.NewSource(20260921))
	for i := 0; i < 500; i++ {
		base, local, desired := randomSides(rnd)
		ds := Merge(base, local, desired)
		b, l, d := byLPath(base), byLPath(local), byLPath(desired)
		for _, dec := range ds {
			lp := dec.LPath
			// Nunca se escribe encima de un cambio local: take y remove solo
			// salen cuando lo local sigue siendo la base.
			if (dec.Action == ActionTake || dec.Action == ActionRemove) && !same(at(l, lp), at(b, lp)) {
				t.Fatalf("%s: %s pisando un cambio local", lp, dec.Action)
			}
			// Y nunca se emite nada donde no hay nada que hacer.
			if same(at(l, lp), at(d, lp)) {
				t.Fatalf("%s: emitida con local == deseado", lp)
			}
		}
		// Aplicar lo que no choca y volver a reconciliar no vuelve a pedirlo.
		after := apply(local, ds)
		for _, dec := range Merge(base, after, desired) {
			if dec.Action == ActionTake || dec.Action == ActionRemove {
				t.Fatalf("%s: %s repetida tras aplicarla", dec.LPath, dec.Action)
			}
		}
		// Sin base (base = local) nada puede chocar: una orden absoluta gana.
		for _, dec := range Merge(local, local, desired) {
			if dec.Action == ActionConflict || dec.Action == ActionKeep {
				t.Fatalf("%s: %s en una orden sin base", dec.LPath, dec.Action)
			}
		}
	}
}

// apply devuelve lo local después de aplicar lo que no choca.
func apply(local []snapshot.Item, ds []Decision) []snapshot.Item {
	m := byLPath(local)
	for _, d := range ds {
		switch d.Action {
		case ActionTake:
			m[d.LPath] = *d.Desired
		case ActionRemove:
			delete(m, d.LPath)
		}
	}
	out := make([]snapshot.Item, 0, len(m))
	for _, x := range m {
		out = append(out, x)
	}
	return out
}
