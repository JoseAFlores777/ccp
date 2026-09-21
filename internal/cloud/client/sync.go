package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"github.com/JoseAFlores777/ccp/internal/cloud/api"
	"github.com/JoseAFlores777/ccp/internal/cloud/crypt"
	"github.com/JoseAFlores777/ccp/internal/snapshot"
)

// PushReport resume una subida.
// Las etiquetas json son el contrato de `ccp cloud push --json`: snake_case
// como el resto del CLI, y las dos listas se inicializan en Push para que
// nunca salgan como null (un consumidor hace `.missing | length`).
type PushReport struct {
	Snapshots int   `json:"snapshots"`
	Uploaded  int   `json:"uploaded"`
	Bytes     int64 `json:"bytes"`
	// Missing: rutas cuyo contenido no está en este equipo (snapshots
	// importados sin secretos). TooLarge: rutas que superan MaxBlobBytes
	// sellado. Ninguna de las dos se sube; el snapshot sí.
	Missing  []string `json:"missing"`
	TooLarge []string `json:"too_large"`
	// Pinned: snapshots que ya estaban arriba y a los que se les puso al día
	// el fijado (campo nuevo, no forma cambiada).
	Pinned int `json:"pinned"`
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Push sube, del más viejo al más nuevo, los snapshots locales que la nube aún
// no tiene. only (un id local completo), si no está vacío, limita a ese.
func Push(ctx context.Context, a *API, acct *crypt.Account, st *snapshot.Store, files Files, only string) (PushReport, error) {
	rep := PushReport{Missing: []string{}, TooLarge: []string{}}
	state, err := files.LoadState()
	if err != nil {
		return rep, err
	}
	ms, err := st.List()
	if err != nil {
		return rep, err
	}
	for i := len(ms) - 1; i >= 0; i-- {
		m := ms[i]
		if only != "" && m.ID != only {
			continue
		}
		if _, done := state.Pushed[m.ID]; done {
			continue
		}
		cloudID, err := pushOne(ctx, a, acct, st, m, &rep)
		if err != nil {
			return rep, fmt.Errorf("snapshot %s: %w", snapshot.Short(m.ID), err)
		}
		state.Pushed[m.ID] = cloudID
		if err := files.SaveState(state); err != nil {
			return rep, err
		}
		rep.Snapshots++
	}
	if err := syncPins(ctx, a, st, state, &rep); err != nil {
		return rep, err
	}
	return rep, nil
}

// syncPins pone al día, de lo que ya está arriba, lo que está fijado. Fijar es
// del cliente porque el servidor no puede saberlo: «fijado» es la bandera del
// manifiesto o el hecho de tener etiqueta, y el manifiesto viaja sellado. Y va
// aquí, en cada `push`, porque fijar DESPUÉS de subir es el caso normal —uno
// se da cuenta de que ese snapshot importa más tarde— y publicarlo otra vez no
// cambiaría nada: el id ya está y el servidor contesta que sí, que ya lo tiene.
//
// Si dos máquinas no opinan lo mismo de un snapshot, gana la última que hace
// push. Un fijado de más no cuesta nada; lo que no puede pasar es que se poda
// algo que alguien dijo que se conserva.
func syncPins(ctx context.Context, a *API, st *snapshot.Store, state State, rep *PushReport) error {
	ms, err := st.List()
	if err != nil {
		return err
	}
	quiere := map[string]bool{} // id en la nube -> fijado que dice esta máquina
	for _, m := range ms {
		if cloudID, up := state.Pushed[m.ID]; up {
			quiere[cloudID] = m.Pinned || m.Label != ""
		}
	}
	if len(quiere) == 0 {
		return nil
	}
	links, err := a.Chain(ctx)
	if err != nil {
		return err
	}
	for _, l := range links {
		// Una lápida ya no tiene contenido que conservar: fijarla no salva nada.
		want, mine := quiere[l.ID]
		if !mine || l.Pruned || l.Pinned == want {
			continue
		}
		if err := a.PinSnapshot(ctx, l.ID, want); err != nil {
			// Se dice en voz alta: un fijado que no llega es un snapshot que
			// la retención del servidor puede podar creyendo que sobra.
			return fmt.Errorf("no se pudo poner al día lo fijado de %s: %w", snapshot.Short(l.ID), err)
		}
		rep.Pinned++
	}
	return nil
}

func pushOne(ctx context.Context, a *API, acct *crypt.Account, st *snapshot.Store, m *snapshot.Manifest, rep *PushReport) (string, error) {
	local := map[string]string{} // id en la nube -> hash local
	for _, it := range m.Items {
		if !st.HasBlob(it.Hash) {
			rep.Missing = append(rep.Missing, it.LPath)
			continue
		}
		local[acct.BlobID(it.Hash)] = it.Hash
	}
	lpathsOf := func(hash string) []string {
		var out []string
		for _, it := range m.Items {
			if it.Hash == hash {
				out = append(out, it.LPath)
			}
		}
		return out
	}
	upload := func(ids []string) error {
		for start := 0; start < len(ids); start += api.MaxPresignIDs {
			items, err := a.Presign(ctx, "put", ids[start:min(start+api.MaxPresignIDs, len(ids))])
			if err != nil {
				return err
			}
			for _, pi := range items {
				hash, ok := local[pi.ID]
				if pi.Exists || !ok {
					continue
				}
				data, err := st.GetBlob(hash)
				if err != nil {
					return err
				}
				sealed, err := acct.SealBlob(pi.ID, data)
				if err != nil {
					return err
				}
				if len(sealed) > api.MaxBlobBytes {
					rep.TooLarge = append(rep.TooLarge, lpathsOf(hash)...)
					delete(local, pi.ID)
					continue
				}
				if err := a.PutBlob(ctx, pi.URL, sealed); err != nil {
					return err
				}
				rep.Uploaded++
				rep.Bytes += int64(len(sealed))
			}
		}
		return nil
	}
	if err := upload(sortedKeys(local)); err != nil {
		return "", err
	}
	js, err := json.Marshal(m)
	if err != nil {
		return "", err
	}
	cloudID, parent := acct.SnapshotID(m.ID), acct.SnapshotID(m.Parent)
	sealed, err := acct.SealManifest(cloudID, js)
	if err != nil {
		return "", err
	}
	in := api.SnapshotIn{ID: cloudID, Parent: parent, Created: m.Created, Manifest: sealed,
		Sig: acct.Sign(cloudID, parent, sealed), Blobs: sortedKeys(local),
		Pinned: m.Pinned || m.Label != ""}
	_, err = a.CommitSnapshot(ctx, in)
	var apiErr *APIError
	if errors.As(err, &apiErr) && apiErr.Code == api.CodeMissingBlobs {
		// El servidor no encuentra blobs que se subieron (una subida que falló
		// sin avisar): se suben otra vez y se reintenta una sola vez.
		if err := upload(apiErr.Missing); err != nil {
			return "", err
		}
		_, err = a.CommitSnapshot(ctx, in)
	}
	return cloudID, err
}

// Pull baja el snapshot cloudID de la nube al almacén local. Devuelve el
// manifiesto y las rutas que no tenían datos en la nube.
func Pull(ctx context.Context, a *API, acct *crypt.Account, st *snapshot.Store, files Files, cloudID string) (*snapshot.Manifest, []string, error) {
	sn, err := a.Snapshot(ctx, cloudID)
	if err != nil {
		return nil, nil, err
	}
	// La clave pública sale de la AK: un servidor comprometido no puede colar
	// un snapshot ni cambiar su padre.
	if err := acct.Verify(sn.ID, sn.Parent, sn.Manifest, sn.Sig); err != nil {
		return nil, nil, err
	}
	js, err := acct.OpenManifest(sn.ID, sn.Manifest)
	if err != nil {
		return nil, nil, fmt.Errorf("no se pudo abrir el manifiesto: %w", err)
	}
	var m snapshot.Manifest
	if err := json.Unmarshal(js, &m); err != nil {
		return nil, nil, fmt.Errorf("manifiesto ilegible: %w", err)
	}
	if acct.SnapshotID(m.ID) != sn.ID {
		return nil, nil, errors.New("el manifiesto no corresponde a su id en la nube")
	}
	need := map[string][]snapshot.Item{} // id en la nube -> elementos que lo usan
	for _, it := range m.Items {
		if !st.HasBlob(it.Hash) {
			bid := acct.BlobID(it.Hash)
			need[bid] = append(need[bid], it)
		}
	}
	var missing []string
	ids := sortedKeys(need)
	for start := 0; start < len(ids); start += api.MaxPresignIDs {
		items, err := a.Presign(ctx, "get", ids[start:min(start+api.MaxPresignIDs, len(ids))])
		if err != nil {
			return nil, nil, err
		}
		for _, pi := range items {
			its := need[pi.ID]
			if len(its) == 0 {
				continue
			}
			if !pi.Exists {
				for _, it := range its {
					missing = append(missing, it.LPath)
				}
				continue
			}
			body, err := a.GetBlob(ctx, pi.URL)
			if err != nil {
				return nil, nil, err
			}
			data, err := acct.OpenBlob(pi.ID, body)
			if err != nil || snapshot.Hash(data) != its[0].Hash {
				return nil, nil, fmt.Errorf("el blob de %s llegó alterado", its[0].LPath)
			}
			secret := false
			for _, it := range its {
				secret = secret || it.Class == snapshot.ClassSecret
			}
			if _, err := st.PutBlob(data, secret); err != nil {
				return nil, nil, err
			}
		}
	}
	if err := st.SaveManifest(&m); err != nil {
		return nil, nil, err
	}
	state, err := files.LoadState()
	if err != nil {
		return nil, nil, err
	}
	state.Pushed[m.ID] = sn.ID // ya está arriba: no se vuelve a subir
	if err := files.SaveState(state); err != nil {
		return nil, nil, err
	}
	sort.Strings(missing)
	return &m, missing, nil
}
