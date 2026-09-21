package remote

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/JoseAFlores777/ccp/internal/cloud/api"
	"github.com/JoseAFlores777/ccp/internal/cloud/crypt"
)

// Lo que hay en el destino. Tres cosas y ninguna más:
//
//	remote.json          los parámetros del KDF y las envolturas de la AK
//	objects/<ab>/<id>    cada blob sellado, nombrado por su HMAC
//	snaps/<id>.json      cada snapshot: manifiesto sellado, firma y sus blobs
//
// Los blobs van en dos niveles como en el almacén local: una carpeta con
// decenas de miles de entradas es lenta de listar en cualquier sistema, y en
// una sincronizada además se nota al arrancar el demonio.
const (
	// formatVersion sube solo si un ccp viejo NO podría leer el destino. Un
	// formato más nuevo se rechaza entero, como el ccp.yaml: leer a medias un
	// almacén que no se entiende es peor que no leerlo.
	formatVersion = 1
	keyRemote     = "remote.json"
	prefixObjects = "objects"
	prefixSnaps   = "snaps"
)

// remoteFile es remote.json. Lleva la bóveda tal cual viaja a la nube
// (api.Vault) a propósito: es el mismo formato, así que un equipo que sabe
// abrir una sabe abrir la otra y `client.VaultFromAPI` vale para las dos.
type remoteFile struct {
	Format int       `json:"format"`
	Vault  api.Vault `json:"vault"`
}

// snapRecord es un snapshot en el destino. Es el mismo api.SnapshotIn que se
// manda a la nube, más el tamaño que el servidor calcula al recibirlo.
//
// Lo que NO lleva es quién lo subió: una carpeta no tiene cuentas ni
// dispositivos, y el nombre del equipo sería el único dato en claro de todo el
// almacén. Por eso SnapshotMeta sale de aquí sin dispositivo.
type snapRecord struct {
	Format int `json:"format"`
	api.SnapshotIn
	Size int64 `json:"size"`
}

// Store pone el formato de §8.2 encima de un transporte. La carpeta y el
// bucket comparten TODO menos el transporte, así que esto se escribe una vez.
type Store struct{ o Objects }

// New envuelve un transporte ya abierto.
func New(o Objects) *Store { return &Store{o: o} }

var _ Remote = (*Store)(nil)

func (s *Store) Name() string { return s.o.Name() }

// Objects devuelve el transporte, para quien necesite hablar con él directamente.
func (s *Store) Objects() Objects { return s.o }

func blobKey(id string) (string, error) {
	if !api.ValidID(id) {
		return "", fmt.Errorf("id de blob inválido: %q", id)
	}
	return prefixObjects + "/" + id[:2] + "/" + id, nil
}

func snapKey(id string) (string, error) {
	if !api.ValidID(id) {
		return "", fmt.Errorf("id de snapshot inválido: %q", id)
	}
	return prefixSnaps + "/" + id + ".json", nil
}

// Vault lee las envolturas. Que no haya remote.json es ErrNoVault y no un
// fallo: es lo que contesta una carpeta recién elegida.
func (s *Store) Vault(ctx context.Context) (api.Vault, error) {
	data, ok, err := s.o.Get(ctx, keyRemote)
	if err != nil {
		return api.Vault{}, err
	}
	if !ok {
		return api.Vault{}, ErrNoVault
	}
	var rf remoteFile
	if err := json.Unmarshal(data, &rf); err != nil {
		return api.Vault{}, fmt.Errorf("%s/%s está dañado: %w", s.Name(), keyRemote, err)
	}
	if rf.Format > formatVersion {
		return api.Vault{}, fmt.Errorf("%s lo escribió un ccp más nuevo (formato %d); actualiza ccp", s.Name(), rf.Format)
	}
	return rf.Vault, nil
}

// PutVault crea la bóveda. Si ya hay una, se niega: pisarla dejaría sin abrir
// todo lo que haya publicado, y el caso real no es un ataque sino apuntar dos
// cuentas a la misma carpeta sin darse cuenta. Rotar las envolturas sobre la
// MISMA AK es otra operación (§10.2) y aún no pasa por aquí.
//
// La negativa la decide PutIfAbsent, no la lectura previa. Esa lectura sigue
// estando porque da el error bueno —«ya hay bóveda» frente a «el remote.json
// lo escribió un ccp más nuevo»—, pero entre ella y la escritura cabe el
// `sync remote add` del otro equipo, y con un Put normal el segundo pisaba al
// primero devolviendo nil: a partir de ahí quien abre la carpeta obtiene la
// AK del segundo y lo del primero queda ilegible para siempre, en silencio.
func (s *Store) PutVault(ctx context.Context, v api.Vault) error {
	if _, err := s.Vault(ctx); err == nil {
		return ErrVaultExists
	} else if !errors.Is(err, ErrNoVault) {
		return err
	}
	data, err := json.MarshalIndent(remoteFile{Format: formatVersion, Vault: v}, "", "  ")
	if err != nil {
		return err
	}
	creada, err := s.o.PutIfAbsent(ctx, keyRemote, append(data, '\n'))
	if err != nil {
		return err
	}
	if !creada {
		return ErrVaultExists
	}
	return nil
}

// Missing pregunta por los blobs uno a uno, pero solo por el tamaño: traerse
// el contenido para saber si está costaría el almacén entero en cada push.
func (s *Store) Missing(ctx context.Context, ids []string) ([]string, error) {
	var out []string
	for _, id := range sortedUnique(ids) {
		key, err := blobKey(id)
		if err != nil {
			return nil, err
		}
		_, ok, err := s.o.Stat(ctx, key)
		if err != nil {
			return nil, err
		}
		if !ok {
			out = append(out, id)
		}
	}
	return out, nil
}

// PutBlob guarda un blob ya sellado. El tope es el mismo que el de la nube:
// un destino que acepta lo que el otro rechaza haría que un snapshot subiera
// aquí y no allí, y la misma historia dejaría de poder ir a los dos sitios.
func (s *Store) PutBlob(ctx context.Context, id string, sealed []byte) error {
	key, err := blobKey(id)
	if err != nil {
		return err
	}
	if len(sealed) > api.MaxBlobBytes {
		return fmt.Errorf("el blob %s pasa del tope de %d bytes", id[:12], int64(api.MaxBlobBytes))
	}
	return s.o.Put(ctx, key, sealed)
}

// Fetch entrega lo que hay y devuelve lo que falta. Lo que falta NO es un
// error: un destino puede haber podado un blob, y quien pide tiene que poder
// decir qué rutas se quedaron sin contenido en vez de no restaurar nada.
func (s *Store) Fetch(ctx context.Context, ids []string, sink func(id string, sealed []byte) error) ([]string, error) {
	var missing []string
	for _, id := range sortedUnique(ids) {
		key, err := blobKey(id)
		if err != nil {
			return nil, err
		}
		data, ok, err := s.o.Get(ctx, key)
		if err != nil {
			return nil, err
		}
		if !ok {
			missing = append(missing, id)
			continue
		}
		if err := sink(id, data); err != nil {
			return nil, err
		}
	}
	return missing, nil
}

// commitShapeOK es la misma comprobación de forma que hace el servidor. Aquí
// no hay nadie más que la haga: el destino es la última puerta antes de que
// algo ilegible quede publicado para siempre con el nombre de un id válido.
func commitShapeOK(in api.SnapshotIn, ids []string) error {
	switch {
	case !api.ValidID(in.ID):
		return fmt.Errorf("id de snapshot inválido: %q", in.ID)
	case in.Parent != "" && !api.ValidID(in.Parent):
		return fmt.Errorf("padre inválido: %q", in.Parent)
	case len(in.Manifest) == 0 || len(in.Manifest) > api.MaxManifestBytes:
		return errors.New("manifiesto vacío o demasiado grande")
	case len(in.Sig) != ed25519.SignatureSize:
		return errors.New("firma inválida")
	case in.Created.IsZero():
		return errors.New("al snapshot le falta la fecha")
	case len(ids) > api.MaxCommitIDs:
		return fmt.Errorf("demasiados blobs (%d)", len(ids))
	}
	for _, id := range ids {
		if !api.ValidID(id) {
			return fmt.Errorf("id de blob inválido: %q", id)
		}
	}
	return nil
}

// CommitSnapshot publica. Comprueba antes que están TODOS sus blobs, como el
// servidor: un manifiesto sin sus contenidos se lista pero no se restaura, y
// el que se lo encuentra es el equipo de enfrente, meses después.
//
// Volver a publicar el mismo id no reescribe nada y devuelve lo que ya hay: un
// push que se corta después de subir se reintenta entero, y el id es el HMAC
// del manifiesto, así que dos registros con el mismo id son el mismo snapshot.
func (s *Store) CommitSnapshot(ctx context.Context, in api.SnapshotIn) (api.SnapshotMeta, error) {
	ids := sortedUnique(in.Blobs)
	if err := commitShapeOK(in, ids); err != nil {
		return api.SnapshotMeta{}, err
	}
	key, err := snapKey(in.ID)
	if err != nil {
		return api.SnapshotMeta{}, err
	}
	if rec, ok, err := s.record(ctx, key); err != nil {
		return api.SnapshotMeta{}, err
	} else if ok {
		return metaOf(rec), nil
	}
	total := int64(len(in.Manifest))
	var missing []string
	for _, id := range ids {
		bk, err := blobKey(id)
		if err != nil {
			return api.SnapshotMeta{}, err
		}
		size, ok, err := s.o.Stat(ctx, bk)
		if err != nil {
			return api.SnapshotMeta{}, err
		}
		if !ok {
			missing = append(missing, id)
			continue
		}
		total += size
	}
	if len(missing) > 0 {
		return api.SnapshotMeta{}, &MissingBlobsError{IDs: missing}
	}
	rec := snapRecord{Format: formatVersion, SnapshotIn: in, Size: total}
	rec.Blobs, rec.Created = ids, in.Created.UTC()
	data, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return api.SnapshotMeta{}, err
	}
	if err := s.o.Put(ctx, key, append(data, '\n')); err != nil {
		return api.SnapshotMeta{}, err
	}
	return metaOf(rec), nil
}

func metaOf(r snapRecord) api.SnapshotMeta {
	return api.SnapshotMeta{ID: r.ID, Parent: r.Parent, Created: r.Created, Size: r.Size, Pinned: r.Pinned}
}

// record lee un registro. El formato se comprueba al leer y no solo al
// escribir: el destino es compartido, y el que escribió puede ser un ccp más
// nuevo que el que lee.
func (s *Store) record(ctx context.Context, key string) (snapRecord, bool, error) {
	data, ok, err := s.o.Get(ctx, key)
	if err != nil || !ok {
		return snapRecord{}, false, err
	}
	var rec snapRecord
	if err := json.Unmarshal(data, &rec); err != nil {
		return snapRecord{}, false, fmt.Errorf("%s/%s está dañado: %w", s.Name(), key, err)
	}
	if rec.Format > formatVersion {
		return snapRecord{}, false, fmt.Errorf("%s/%s lo escribió un ccp más nuevo (formato %d)", s.Name(), key, rec.Format)
	}
	return rec, true, nil
}

// Snapshot devuelve uno entero: su manifiesto sellado y su firma.
func (s *Store) Snapshot(ctx context.Context, id string) (api.Snapshot, error) {
	key, err := snapKey(id)
	if err != nil {
		return api.Snapshot{}, err
	}
	rec, ok, err := s.record(ctx, key)
	if err != nil {
		return api.Snapshot{}, err
	}
	if !ok {
		return api.Snapshot{}, fmt.Errorf("el snapshot %s %w", id[:12], ErrNotFound)
	}
	return api.Snapshot{SnapshotMeta: metaOf(rec), Manifest: rec.Manifest, Sig: rec.Sig}, nil
}

// Chain lee la historia entera. Sí, abre todos los registros: un índice sería
// una segunda fuente de verdad, y en una carpeta a la que escriben dos equipos
// a la vez es justo lo que se queda atrás —la lista de blobs y el padre viven
// en cada registro, que es inmutable, y eso no se desincroniza—. Un registro
// son unos kilobytes y los snapshots de una cuenta se cuentan por decenas.
//
// Un registro ilegible NO tumba el listado: se cuenta como eslabón que falta
// (no aparece) y el que lo note será la verificación de la cadena, que es
// quien sabe decir que hay un hueco. Tirar aquí dejaría sin ver los buenos.
func (s *Store) Chain(ctx context.Context) ([]api.ChainLink, error) {
	keys, err := s.o.List(ctx, prefixSnaps)
	if err != nil {
		return nil, err
	}
	links := make([]api.ChainLink, 0, len(keys))
	for _, key := range keys {
		id := strings.TrimSuffix(strings.TrimPrefix(key, prefixSnaps+"/"), ".json")
		if !strings.HasSuffix(key, ".json") || !api.ValidID(id) {
			continue
		}
		rec, ok, err := s.record(ctx, key)
		if err != nil || !ok || rec.ID != id {
			continue
		}
		links = append(links, api.ChainLink{ID: rec.ID, Parent: rec.Parent, Created: rec.Created,
			Digest: crypt.ManifestDigest(rec.Manifest), Sig: rec.Sig, Pinned: rec.Pinned})
		if len(links) == api.MaxChainLinks {
			break
		}
	}
	// Por fecha y, a igualdad, por id: dos snapshots del mismo instante
	// (dos equipos a la vez) tienen que salir siempre en el mismo orden.
	sort.Slice(links, func(i, j int) bool {
		if links[i].Created.Equal(links[j].Created) {
			return links[i].ID < links[j].ID
		}
		return links[i].Created.Before(links[j].Created)
	})
	return links, nil
}
