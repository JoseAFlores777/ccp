package remote

// sync.go — subir y bajar snapshots de un destino (spec §9, E2). Es el mismo
// trabajo que internal/cloud/client hace contra el API, escrito una sola vez
// contra la interfaz Remote: la carpeta, el bucket y la nube pasan por aquí.
//
// Dos diferencias con el camino de la nube, y las dos son por lo que el
// destino NO tiene:
//
//   - No hay prefirmadas. Se pregunta en lote qué falta (Missing) y se suben
//     esos, que es lo que la carpeta puede contestar sin inventarse un API.
//   - No hay fijado aparte. El servidor tiene retención y por eso `pin` es una
//     ruta suya; una carpeta no poda nada, así que el `pinned` viaja dentro
//     del registro y no hay nada que poner al día después de subir.
//
// El estado local —qué snapshots de este equipo están ya en el destino— se
// guarda con los mismos archivos que la nube (client.Files) apuntando al
// directorio del destino: son el mismo dato y dos formatos serían dos formas
// de olvidarse de uno.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/JoseAFlores777/ccp/internal/cloud/api"
	"github.com/JoseAFlores777/ccp/internal/cloud/client"
	"github.com/JoseAFlores777/ccp/internal/cloud/crypt"
	"github.com/JoseAFlores777/ccp/internal/snapshot"
)

// ErrNoSnapshots: el destino aún no tiene ninguno. No es un fallo —es lo que
// contesta una carpeta recién creada—, así que quien llama decide qué dice.
var ErrNoSnapshots = errors.New("el destino no tiene ningún snapshot")

// PushReport resume una subida. Las etiquetas json son el contrato de
// `ccp sync push --json`; las dos listas se inicializan en Push para que nunca
// salgan como null (un consumidor hace `.missing | length`).
type PushReport struct {
	Snapshots int   `json:"snapshots"`
	Uploaded  int   `json:"uploaded"`
	Bytes     int64 `json:"bytes"`
	// Missing: rutas cuyo contenido no está en ESTE equipo (un snapshot
	// importado sin secretos). TooLarge: rutas que pasan de api.MaxBlobBytes
	// selladas. Ninguna se sube; el snapshot sí, porque perder la historia
	// entera por un archivo que nunca estuvo es peor.
	Missing  []string `json:"missing"`
	TooLarge []string `json:"too_large"`
}

func keysOf[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Push sube, del más viejo al más nuevo, los snapshots locales que el destino
// aún no tiene. only (un id local entero), si no está vacío, limita a ese.
func Push(ctx context.Context, r Remote, acct *crypt.Account, st *snapshot.Store, f client.Files, only string) (PushReport, error) {
	rep := PushReport{Missing: []string{}, TooLarge: []string{}}
	state, err := f.LoadState()
	if err != nil {
		return rep, err
	}
	ms, err := st.List()
	if err != nil {
		return rep, err
	}
	// st.List() va del más nuevo al más viejo y el orden importa: el padre de
	// un snapshot tiene que estar arriba antes que él, o la cadena tiene un
	// hueco hasta el siguiente push.
	for i := len(ms) - 1; i >= 0; i-- {
		m := ms[i]
		if only != "" && m.ID != only {
			continue
		}
		if _, done := state.Pushed[m.ID]; done {
			continue
		}
		rid, err := pushOne(ctx, r, acct, st, m, &rep)
		if err != nil {
			return rep, fmt.Errorf("snapshot %s: %w", snapshot.Short(m.ID), err)
		}
		state.Pushed[m.ID] = rid
		// Se persiste snapshot a snapshot: un push cortado a la mitad no
		// vuelve a subir lo que ya llegó.
		if err := f.SaveState(state); err != nil {
			return rep, err
		}
		rep.Snapshots++
	}
	return rep, nil
}

func pushOne(ctx context.Context, r Remote, acct *crypt.Account, st *snapshot.Store,
	m *snapshot.Manifest, rep *PushReport) (string, error) {
	local := map[string]string{} // id en el destino -> hash local
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
	// upload sube los que el destino diga que le faltan. Preguntar primero no
	// es una optimización cosmética: en un bucket cada subida se paga, y en
	// una carpeta sincronizada reescribir un objeto que ya está lo vuelve a
	// mandar por la red del servicio que la sincroniza.
	upload := func(ids []string) error {
		falta, err := r.Missing(ctx, ids)
		if err != nil {
			return err
		}
		for _, id := range falta {
			hash, ok := local[id]
			if !ok {
				continue
			}
			data, err := st.GetBlob(hash)
			if err != nil {
				return err
			}
			sealed, err := acct.SealBlob(id, data)
			if err != nil {
				return err
			}
			// El tope es el del formato, no el de un destino: el mismo
			// snapshot puede acabar en una carpeta y en la nube, y un blob
			// que allí no cabe no puede contar como subido aquí.
			if len(sealed) > api.MaxBlobBytes {
				rep.TooLarge = append(rep.TooLarge, lpathsOf(hash)...)
				delete(local, id)
				continue
			}
			if err := r.PutBlob(ctx, id, sealed); err != nil {
				return err
			}
			rep.Uploaded++
			rep.Bytes += int64(len(sealed))
		}
		return nil
	}
	if err := upload(keysOf(local)); err != nil {
		return "", err
	}
	js, err := json.Marshal(m)
	if err != nil {
		return "", err
	}
	rid, parent := acct.SnapshotID(m.ID), acct.SnapshotID(m.Parent)
	sealed, err := acct.SealManifest(rid, js)
	if err != nil {
		return "", err
	}
	in := api.SnapshotIn{ID: rid, Parent: parent, Created: m.Created, Manifest: sealed,
		Sig: acct.Sign(rid, parent, sealed), Blobs: keysOf(local),
		Pinned: m.Pinned || m.Label != ""}
	_, err = r.CommitSnapshot(ctx, in)
	var falta *MissingBlobsError
	if errors.As(err, &falta) {
		// El destino no encuentra blobs que se subieron (una subida que se
		// perdió sin avisar, o una carpeta a medio sincronizar): se suben
		// otra vez y se reintenta UNA sola vez.
		if err := upload(falta.IDs); err != nil {
			return "", err
		}
		_, err = r.CommitSnapshot(ctx, in)
	}
	return rid, err
}

// Pick traduce «latest», el vacío o un prefijo de id al id entero de un
// snapshot del destino. Lo comparten `pull` y `apply`: dos formas de nombrar
// el mismo snapshot serían dos formas de equivocarse de snapshot.
//
// Se resuelve con la cadena, que sale de los registros y no de un índice: en
// una carpeta donde escriben dos equipos a la vez el índice es justo lo que se
// queda atrás.
func Pick(ctx context.Context, r Remote, ref string) (string, error) {
	links, err := r.Chain(ctx)
	if err != nil {
		return "", err
	}
	if len(links) == 0 {
		return "", ErrNoSnapshots
	}
	if ref == "" || ref == "latest" {
		return links[len(links)-1].ID, nil // la cadena viene del más viejo al más nuevo
	}
	ref = strings.ToLower(ref)
	var hit string
	for _, l := range links {
		if !strings.HasPrefix(l.ID, ref) {
			continue
		}
		if hit != "" {
			return "", fmt.Errorf("%q nombra a más de un snapshot del destino", ref)
		}
		hit = l.ID
	}
	if hit == "" {
		return "", fmt.Errorf("no hay ningún snapshot %q en %s", ref, r.Name())
	}
	return hit, nil
}

// Pull baja el snapshot rid del destino al almacén local. Devuelve el
// manifiesto y las rutas lógicas que el destino no tenía.
func Pull(ctx context.Context, r Remote, acct *crypt.Account, st *snapshot.Store,
	f client.Files, rid string) (*snapshot.Manifest, []string, error) {
	m, err := openSnapshot(ctx, r, acct, rid)
	if err != nil {
		return nil, nil, err
	}
	need := map[string][]snapshot.Item{} // id en el destino -> elementos que lo usan
	for _, it := range m.Items {
		if !st.HasBlob(it.Hash) {
			bid := acct.BlobID(it.Hash)
			need[bid] = append(need[bid], it)
		}
	}
	ausentes, err := r.Fetch(ctx, keysOf(need), func(id string, sealed []byte) error {
		its := need[id]
		if len(its) == 0 {
			return nil
		}
		data, err := acct.OpenBlob(id, sealed)
		if err != nil || snapshot.Hash(data) != its[0].Hash {
			return fmt.Errorf("el blob de %s llegó alterado", its[0].LPath)
		}
		secret := false
		for _, it := range its {
			secret = secret || it.Class == snapshot.ClassSecret
		}
		_, err = st.PutBlob(data, secret)
		return err
	})
	if err != nil {
		return nil, nil, err
	}
	var missing []string
	for _, id := range ausentes {
		for _, it := range need[id] {
			missing = append(missing, it.LPath)
		}
	}
	sort.Strings(missing)
	if err := st.SaveManifest(m); err != nil {
		return nil, nil, err
	}
	state, err := f.LoadState()
	if err != nil {
		return nil, nil, err
	}
	// Ya está allí: no se vuelve a subir. Y queda marcado como BAJADO, que es
	// lo que distingue «esta máquina lo publicó» de «esta máquina lo recibió».
	state.Pushed[m.ID] = rid
	state.Pulled[m.ID] = true
	if err := f.SaveState(state); err != nil {
		return nil, nil, err
	}
	return m, missing, nil
}

// openSnapshot trae el manifiesto, comprueba la firma y lo abre. La clave
// pública sale de la AK, así que quien opera la carpeta o el bucket no puede
// colar un snapshot ni cambiarle el padre.
func openSnapshot(ctx context.Context, r Remote, acct *crypt.Account, rid string) (*snapshot.Manifest, error) {
	sn, err := r.Snapshot(ctx, rid)
	if err != nil {
		return nil, err
	}
	if err := acct.Verify(sn.ID, sn.Parent, sn.Manifest, sn.Sig); err != nil {
		return nil, err
	}
	js, err := acct.OpenManifest(sn.ID, sn.Manifest)
	if err != nil {
		return nil, fmt.Errorf("no se pudo abrir el manifiesto: %w", err)
	}
	var m snapshot.Manifest
	if err := json.Unmarshal(js, &m); err != nil {
		return nil, fmt.Errorf("manifiesto ilegible: %w", err)
	}
	if acct.SnapshotID(m.ID) != sn.ID {
		return nil, errors.New("el manifiesto no corresponde a su id en el destino")
	}
	return &m, nil
}
