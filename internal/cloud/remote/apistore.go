package remote

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"

	"github.com/JoseAFlores777/ccp/internal/cloud/api"
	"github.com/JoseAFlores777/ccp/internal/cloud/client"
)

// APIStore pone la nube de ccp detrás de la misma interfaz que la carpeta.
// Existe para que la abstracción sea de verdad una y no dos: el contrato de
// remote_test corre igual contra las dos, así que lo que una contesta a «no
// hay bóveda» o «faltan blobs» lo contesta igual la otra, y la
// sincronización se puede escribir una sola vez.
//
// Casi todo es reenviar —el API ya habla en los tipos de internal/cloud/api—
// menos los blobs, que allí no se piden por id sino por URL prefirmada.
type APIStore struct {
	a    *client.API
	name string

	// puts guarda las URLs de subida que dejó Missing. No es una
	// optimización de pasada: sin ellas, subir N blobs serían N peticiones
	// de prefirmado además de las N subidas, cuando el servidor las da de
	// 500 en 500. Caducan (15 min del lado del servidor), así que una subida
	// que falla con una URL guardada se reintenta con otra recién pedida.
	mu   sync.Mutex
	puts map[string]string
}

// NewAPIStore envuelve un API ya autenticado. name es lo que se enseña: la
// dirección del servidor, nunca el token.
func NewAPIStore(a *client.API, name string) *APIStore {
	return &APIStore{a: a, name: name, puts: map[string]string{}}
}

var _ Remote = (*APIStore)(nil)

func (s *APIStore) Name() string { return s.name }

// codeIs reconoce un error del API por su código. Los códigos son el contrato
// del servidor; el estado HTTP y el texto no.
func codeIs(err error, code string) bool {
	var e *client.APIError
	return errors.As(err, &e) && e.Code == code
}

func (s *APIStore) Vault(ctx context.Context) (api.Vault, error) {
	v, err := s.a.Vault(ctx)
	if codeIs(err, api.CodeNotFound) {
		return api.Vault{}, ErrNoVault
	}
	return v, err
}

func (s *APIStore) PutVault(ctx context.Context, v api.Vault) error {
	err := s.a.PutVault(ctx, v)
	if codeIs(err, api.CodeConflict) {
		return ErrVaultExists
	}
	return err
}

// Missing pregunta en lotes y se queda con las URLs de subida de lo que falta.
func (s *APIStore) Missing(ctx context.Context, ids []string) ([]string, error) {
	ids = sortedUnique(ids)
	var out []string
	for start := 0; start < len(ids); start += api.MaxPresignIDs {
		items, err := s.a.Presign(ctx, "put", ids[start:min(start+api.MaxPresignIDs, len(ids))])
		if err != nil {
			return nil, err
		}
		for _, it := range items {
			if it.Exists {
				continue
			}
			out = append(out, it.ID)
			s.remember(it.ID, it.URL)
		}
	}
	// El orden de la respuesta es del servidor; el de la interfaz, nuestro.
	sort.Strings(out)
	return out, nil
}

func (s *APIStore) remember(id, url string) {
	if url == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.puts[id] = url
}

// take devuelve y olvida la URL guardada: una URL prefirmada de subida sirve
// para una subida, y guardarla después solo daría reintentos con algo caduco.
func (s *APIStore) take(id string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	url, ok := s.puts[id]
	delete(s.puts, id)
	return url, ok
}

// presignOne pide la URL de un solo id. Devuelve "" si el servidor dice que
// ese blob ya está: en "put", Exists significa que no hay nada que subir.
func (s *APIStore) presignOne(ctx context.Context, op, id string) (string, error) {
	items, err := s.a.Presign(ctx, op, []string{id})
	if err != nil {
		return "", err
	}
	for _, it := range items {
		if it.ID == id {
			return it.URL, nil
		}
	}
	return "", fmt.Errorf("el servidor no contestó por el blob %s", id[:12])
}

// PutBlob sube por la URL que dejó Missing, o pide una si no la hay. Si la
// guardada falla se pide otra y se reintenta UNA vez: lo normal es que haya
// caducado (15 min), y lo que no puede pasar es que un push largo se caiga
// por una URL vieja después de haber subido medio almacén.
func (s *APIStore) PutBlob(ctx context.Context, id string, sealed []byte) error {
	if !api.ValidID(id) {
		return fmt.Errorf("id de blob inválido: %q", id)
	}
	if len(sealed) > api.MaxBlobBytes {
		return fmt.Errorf("el blob %s pasa del tope de %d bytes", id[:12], int64(api.MaxBlobBytes))
	}
	url, guardada := s.take(id)
	if !guardada {
		var err error
		if url, err = s.presignOne(ctx, "put", id); err != nil {
			return err
		}
	}
	if url == "" { // ya estaba arriba
		return nil
	}
	err := s.a.PutBlob(ctx, url, sealed)
	if err == nil || !guardada {
		return err
	}
	fresca, err2 := s.presignOne(ctx, "put", id)
	if err2 != nil || fresca == "" {
		return err
	}
	return s.a.PutBlob(ctx, fresca, sealed)
}

// Fetch baja en lotes, como sube: una petición de prefirmado por cada 500.
func (s *APIStore) Fetch(ctx context.Context, ids []string, sink func(id string, sealed []byte) error) ([]string, error) {
	ids = sortedUnique(ids)
	var missing []string
	for start := 0; start < len(ids); start += api.MaxPresignIDs {
		items, err := s.a.Presign(ctx, "get", ids[start:min(start+api.MaxPresignIDs, len(ids))])
		if err != nil {
			return nil, err
		}
		for _, it := range items {
			if !it.Exists {
				missing = append(missing, it.ID)
				continue
			}
			data, err := s.a.GetBlob(ctx, it.URL)
			if err != nil {
				return nil, err
			}
			if err := sink(it.ID, data); err != nil {
				return nil, err
			}
		}
	}
	sort.Strings(missing)
	return missing, nil
}

func (s *APIStore) CommitSnapshot(ctx context.Context, in api.SnapshotIn) (api.SnapshotMeta, error) {
	meta, err := s.a.CommitSnapshot(ctx, in)
	var e *client.APIError
	if errors.As(err, &e) && e.Code == api.CodeMissingBlobs {
		return meta, &MissingBlobsError{IDs: e.Missing}
	}
	return meta, err
}

func (s *APIStore) Snapshot(ctx context.Context, id string) (api.Snapshot, error) {
	sn, err := s.a.Snapshot(ctx, id)
	if codeIs(err, api.CodeNotFound) {
		return api.Snapshot{}, fmt.Errorf("el snapshot %s %w", id[:min(12, len(id))], ErrNotFound)
	}
	// Un 410 NO se traduce a ErrNotFound: una lápida es otra cosa —el eslabón
	// sigue en la cadena y su contenido lo quitó la retención—, y decir «no
	// está» taparía justo lo que hay que poder contar.
	return sn, err
}

func (s *APIStore) Chain(ctx context.Context) ([]api.ChainLink, error) { return s.a.Chain(ctx) }
