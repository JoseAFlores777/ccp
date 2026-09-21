package client

import (
	"context"
	"testing"
	"time"

	"github.com/JoseAFlores777/ccp/internal/cloud/api"
	"github.com/JoseAFlores777/ccp/internal/cloud/crypt"
	"github.com/JoseAFlores777/ccp/internal/vault"
)

// cadena arma n eslabones encadenados y firmados por la cuenta.
func cadena(t *testing.T, n int) (*crypt.Account, []api.ChainLink) {
	t.Helper()
	ak, err := vault.NewKey()
	if err != nil {
		t.Fatal(err)
	}
	a, err := crypt.NewAccount(ak)
	if err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	var out []api.ChainLink
	prev := ""
	for i := range n {
		id := a.SnapshotID(string(rune('a' + i)))
		out = append(out, api.ChainLink{
			ID: id, Parent: prev, Created: base.Add(time.Duration(i) * time.Hour),
			Digest: crypt.ManifestDigest(sello(i)), Sig: a.Sign(id, prev, sello(i)),
		})
		prev = id
	}
	return a, out
}

// sello es el manifiesto sellado del eslabón i: lo que hace falta para volver
// a firmarlo cuando un test le cambia el padre a propósito.
func sello(i int) []byte { return []byte{byte(i)} }

func codes(r ChainReport) []string {
	out := []string{}
	for _, f := range r.Faults {
		out = append(out, f.Code)
	}
	return out
}

func tiene(r ChainReport, code string) bool {
	for _, f := range r.Faults {
		if f.Code == code {
			return true
		}
	}
	return false
}

func TestVerifyChainAceptaUnaCadenaIntacta(t *testing.T) {
	a, links := cadena(t, 4)
	rep := VerifyChain(a, links, []string{links[0].ID, links[3].ID})
	if !rep.OK() {
		t.Fatalf("una cadena intacta no debería tener faltas: %v", codes(rep))
	}
	if rep.Links != 4 || rep.Roots != 1 {
		t.Fatalf("resumen raro: %+v", rep)
	}
}

func TestVerifyChainDetectaQueQuitanUnEslabon(t *testing.T) {
	a, links := cadena(t, 4)
	quitado := links[1]
	sin := []api.ChainLink{links[0], links[2], links[3]}
	rep := VerifyChain(a, sin, nil)
	if len(rep.Faults) != 1 || rep.Faults[0].Code != FaultBrokenLink || rep.Faults[0].Ref != quitado.ID {
		t.Fatalf("un hueco en medio tenía que salir como %s: %+v", FaultBrokenLink, rep.Faults)
	}
	// Y si el hueco está en la cabeza no hay padre roto que delate nada: lo
	// que lo delata es que esta máquina sabe que lo subió.
	rep = VerifyChain(a, links[:3], []string{links[3].ID})
	if len(rep.Faults) != 1 || rep.Faults[0].Code != FaultDropped || rep.Faults[0].ID != links[3].ID {
		t.Fatalf("un snapshot subido y desaparecido tenía que salir como %s: %+v", FaultDropped, rep.Faults)
	}
}

func TestVerifyChainDetectaQueReescribenUnEslabon(t *testing.T) {
	a, links := cadena(t, 3)
	links[1].Digest = crypt.ManifestDigest([]byte("otra cosa"))
	rep := VerifyChain(a, links, nil)
	if len(rep.Faults) != 1 || rep.Faults[0].Code != FaultBadSignature {
		t.Fatalf("un manifiesto cambiado tenía que salir como %s: %+v", FaultBadSignature, rep.Faults)
	}
}

func TestVerifyChainDetectaQueReordenan(t *testing.T) {
	a, links := cadena(t, 3)
	// La fecha no va firmada: lo que ata el orden son los padres. Un hijo con
	// fecha anterior a su padre es una fecha que miente.
	links[2].Created = links[0].Created.Add(-time.Hour)
	rep := VerifyChain(a, links, nil)
	if len(rep.Faults) != 1 || rep.Faults[0].Code != FaultOutOfOrder {
		t.Fatalf("una fecha imposible tenía que salir como %s: %+v", FaultOutOfOrder, rep.Faults)
	}
}

func TestVerifyChainDetectaUnCicloYUnIDRepetido(t *testing.T) {
	a, links := cadena(t, 3)
	// Los padres se muerden la cola, y el eslabón vuelve a firmarse: sin eso
	// lo que saltaría es la firma y el ciclo no llegaría a mirarse.
	links[0].Parent = links[2].ID
	links[0].Sig = a.Sign(links[0].ID, links[0].Parent, sello(0))
	rep := VerifyChain(a, links, nil)
	if !tiene(rep, FaultCycle) {
		t.Fatalf("un ciclo tenía que salir como %s: %v", FaultCycle, codes(rep))
	}
	b, dos := cadena(t, 2)
	rep = VerifyChain(b, append(dos, dos[1]), nil)
	if !tiene(rep, FaultDuplicateID) {
		t.Fatalf("un id repetido tenía que salir como %s: %v", FaultDuplicateID, codes(rep))
	}
}

// Fijar un snapshot DESPUÉS de subirlo es el caso normal: uno se da cuenta de
// que ese importa más tarde. Si el fijado local no llega a la nube, la
// retención del servidor podaría justo lo que dijiste que se conserva.
func TestPushSincronizaElFijadoDeLoQueYaEstaArriba(t *testing.T) {
	ctx := context.Background()
	r := newRig(t, nil)
	files, a, st := r.machine(t, "mac")
	acct, _, m := seed(t, a, st)
	if _, err := Push(ctx, a, acct, st, files, ""); err != nil {
		t.Fatal(err)
	}
	links, err := a.Chain(ctx)
	if err != nil || len(links) != 1 || links[0].Pinned {
		t.Fatalf("recién subido no está fijado: %+v, %v", links, err)
	}
	etiqueta := "antes del viaje"
	if _, err := st.SetPin(m.ID, true, &etiqueta); err != nil {
		t.Fatal(err)
	}
	rep, err := Push(ctx, a, acct, st, files, "")
	if err != nil || rep.Pinned != 1 {
		t.Fatalf("Push = %+v, %v; quiero 1 fijado sincronizado", rep, err)
	}
	if links, _ = a.Chain(ctx); !links[0].Pinned {
		t.Fatalf("el fijado local no llegó a la nube: %+v", links)
	}
	// Y soltarlo también viaja: si no, fijar sería irreversible en la nube.
	// También hay que quitarle la etiqueta: una etiqueta conserva igual que
	// un fijado, aquí y en la poda local.
	sin := ""
	if _, err := st.SetPin(m.ID, false, &sin); err != nil {
		t.Fatal(err)
	}
	if _, err := Push(ctx, a, acct, st, files, ""); err != nil {
		t.Fatal(err)
	}
	if links, _ = a.Chain(ctx); links[0].Pinned {
		t.Fatalf("soltarlo no llegó a la nube: %+v", links)
	}
}

// La poda del servidor y la comprobación de la cadena se cruzan aquí: una
// lápida SIGUE siendo un eslabón. Si se borrara la fila, el hueco sería idéntico
// al que deja un servidor que te quita un snapshot, y esta comprobación pasaría
// a dar un falso positivo cada día hasta que nadie la mirara.
func TestVerifyChainNoConfundeUnaPodaConUnRobo(t *testing.T) {
	a, links := cadena(t, 3)
	links[1].Pruned = true
	todos := []string{links[0].ID, links[1].ID, links[2].ID}
	rep := VerifyChain(a, links, todos)
	if !rep.OK() {
		t.Fatalf("una lápida no es una falta: %v", codes(rep))
	}
	if rep.Pruned != 1 || rep.Links != 3 {
		t.Fatalf("la lápida se cuenta y sigue contando como eslabón: %+v", rep)
	}
}
