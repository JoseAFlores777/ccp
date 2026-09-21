// Package remote es un destino de snapshots que no es un servidor: una carpeta
// (iCloud Drive, Dropbox, Syncthing, un NAS) o un bucket S3 (spec §9). Guarda
// el mismo almacén de §8.2 que la nube y con el mismo cifrado de §10.2 —blobs
// con ids HMAC, manifiestos sellados y firmados—, así que lo que se sube aquí
// lo entiende cualquier equipo con la frase de la bóveda y nadie más: quien
// opera la carpeta o el bucket mueve bultos que no sabe abrir.
//
// La abstracción es la misma que usa el cliente de la nube. Remote habla en
// los tipos de internal/cloud/api, que es el formato de cable que ya comparten
// los dos lados, de modo que la sincronización se escribe una vez y no dos:
// APIStore pone el API de la nube detrás de esta interfaz sin traducir nada.
package remote

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/JoseAFlores777/ccp/internal/cloud/api"
)

var (
	// ErrNoVault: el destino existe pero aún no tiene bóveda. No es un fallo:
	// es lo que contesta una carpeta vacía, y lo que hay que distinguir de
	// «no se pudo leer» para poder ofrecer crearla.
	ErrNoVault = errors.New("este destino aún no tiene bóveda")
	// ErrVaultExists: ya hay una bóveda. Pisarla dejaría sin abrir todo lo
	// publicado, así que crear otra encima se rechaza (el 409 del servidor).
	ErrVaultExists = errors.New("este destino ya tiene bóveda")
	// ErrNotFound: el snapshot pedido no está en el destino.
	ErrNotFound = errors.New("no está en el destino")
)

// MissingBlobsError dice qué contenidos faltan por subir. Un manifiesto sin
// sus blobs es un registro roto —se puede listar y no se puede restaurar—, así
// que el destino lo rechaza entero y nombra lo que falta para poder reintentar
// solo eso. Es el equivalente del api.CodeMissingBlobs del servidor.
type MissingBlobsError struct{ IDs []string }

func (e *MissingBlobsError) Error() string {
	return fmt.Sprintf("faltan %d blobs por subir: %s", len(e.IDs), strings.Join(shortIDs(e.IDs), ", "))
}

func shortIDs(ids []string) []string {
	out := make([]string, 0, len(ids))
	for i, id := range ids {
		if i == 3 {
			return append(out, "…")
		}
		out = append(out, id[:min(12, len(id))])
	}
	return out
}

// Remote es un destino de snapshots: la nube de ccp, una carpeta o un bucket.
//
// Lo que NO está aquí es tan importante como lo que está: el destino no sabe
// abrir nada de lo que guarda. Recibe bytes ya sellados y los devuelve igual;
// quien tiene la clave de cuenta es el equipo.
type Remote interface {
	// Name nombra el destino para los mensajes. Nunca lleva credenciales:
	// acaba en la configuración, en la pantalla y en los informes.
	Name() string

	// Vault devuelve las envolturas de la clave de cuenta, o ErrNoVault.
	Vault(ctx context.Context) (api.Vault, error)
	// PutVault crea la bóveda, o ErrVaultExists si ya había una.
	PutVault(ctx context.Context, v api.Vault) error

	// Missing devuelve, de ids, los que el destino NO tiene, ordenados. Se
	// pregunta en lote porque en la nube cada pregunta es una petición.
	Missing(ctx context.Context, ids []string) ([]string, error)
	// PutBlob guarda un blob ya sellado. Reescribir el mismo id no pierde
	// nada: el id es el HMAC de su contenido.
	PutBlob(ctx context.Context, id string, sealed []byte) error
	// Fetch entrega a sink los blobs de ids que el destino tiene, y devuelve
	// ordenados los que no tiene. También va en lote, por lo mismo.
	Fetch(ctx context.Context, ids []string, sink func(id string, sealed []byte) error) ([]string, error)

	// CommitSnapshot publica un snapshot. Publicar dos veces el mismo id no
	// es un error —un push cortado se reintenta entero—, pero publicarlo sin
	// sus blobs sí: *MissingBlobsError.
	CommitSnapshot(ctx context.Context, in api.SnapshotIn) (api.SnapshotMeta, error)
	// Snapshot devuelve uno con su manifiesto sellado y su firma, o ErrNotFound.
	Snapshot(ctx context.Context, id string) (api.Snapshot, error)
	// Chain devuelve la historia entera para verificarla de un tirón, sin
	// bajar un solo manifiesto.
	Chain(ctx context.Context) ([]api.ChainLink, error)
}

// sortedUnique ordena y quita repetidos. Los ids llegan de un manifiesto, que
// puede nombrar el mismo contenido en dos rutas.
func sortedUnique(ids []string) []string {
	seen := make(map[string]bool, len(ids))
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	sort.Strings(out)
	return out
}
