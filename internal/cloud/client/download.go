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

// openSnapshot baja un snapshot, comprueba su firma y abre su manifiesto. Lo
// comparten `pull` (al almacén) y `pull -o` (a un archivo): bajar a un archivo
// no puede ser la puerta de atrás que se salta la firma.
func openSnapshot(ctx context.Context, a *API, acct *crypt.Account, cloudID string) (*snapshot.Manifest, error) {
	sn, err := a.Snapshot(ctx, cloudID)
	if err != nil {
		return nil, err
	}
	// La clave pública sale de la AK: un servidor comprometido no puede colar
	// un snapshot ni cambiar su padre.
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
		return nil, errors.New("el manifiesto no corresponde a su id en la nube")
	}
	return &m, nil
}

// fetchBlobs baja los contenidos de need (id en la nube -> elementos que lo
// usan), los abre y se los pasa a sink. Devuelve las rutas lógicas que la nube
// no tenía, ordenadas.
func fetchBlobs(ctx context.Context, a *API, acct *crypt.Account, need map[string][]snapshot.Item,
	sink func(its []snapshot.Item, data []byte) error) ([]string, error) {
	var missing []string
	ids := sortedKeys(need)
	for start := 0; start < len(ids); start += api.MaxPresignIDs {
		items, err := a.Presign(ctx, "get", ids[start:min(start+api.MaxPresignIDs, len(ids))])
		if err != nil {
			return nil, err
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
				return nil, err
			}
			data, err := acct.OpenBlob(pi.ID, body)
			if err != nil || snapshot.Hash(data) != its[0].Hash {
				return nil, fmt.Errorf("el blob de %s llegó alterado", its[0].LPath)
			}
			if err := sink(its, data); err != nil {
				return nil, err
			}
		}
	}
	sort.Strings(missing)
	return missing, nil
}

// Download baja un snapshot de la nube a la MEMORIA, sin tocar el almacén
// local: es lo que necesita `ccp cloud pull -o`, que escribe un archivo y no
// deja rastro en este equipo (puede que ni siquiera tenga snapshots propios).
// Devuelve el manifiesto, sus contenidos como BlobGetter y las rutas que la
// nube no tenía.
func Download(ctx context.Context, a *API, acct *crypt.Account, cloudID string) (*snapshot.Manifest, snapshot.BlobGetter, []string, error) {
	m, err := openSnapshot(ctx, a, acct, cloudID)
	if err != nil {
		return nil, nil, nil, err
	}
	need := map[string][]snapshot.Item{}
	for _, it := range m.Items {
		bid := acct.BlobID(it.Hash)
		need[bid] = append(need[bid], it)
	}
	mem := make(map[string][]byte, len(need))
	missing, err := fetchBlobs(ctx, a, acct, need, func(its []snapshot.Item, data []byte) error {
		mem[its[0].Hash] = data
		return nil
	})
	if err != nil {
		return nil, nil, nil, err
	}
	get := func(hash string) ([]byte, error) {
		data, ok := mem[hash]
		if !ok {
			return nil, snapshot.ErrNotFound
		}
		return data, nil
	}
	return m, get, missing, nil
}
