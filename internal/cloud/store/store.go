// Package store es la persistencia del backend de la nube. Guarda metadatos y
// datos opacos: nada de lo que hay aquí se puede abrir sin la clave de la
// cuenta, que el servidor nunca tiene.
package store

import (
	"context"
	"errors"
	"regexp"
	"time"
)

var (
	// ErrNotFound: no existe, o no es de este usuario (no se distingue).
	ErrNotFound = errors.New("store: no encontrado")
	// ErrConflict: ya existe y no se puede sobrescribir.
	ErrConflict = errors.New("store: ya existe")
)

// User es una cuenta, identificada por el `sub` de Keycloak.
type User struct {
	ID    string
	Sub   string
	Email string
}

// Vault son las envolturas de la clave de cuenta. Todo opaco.
type Vault struct {
	KDF            []byte // JSON
	PassphraseWrap []byte
	RecoveryWrap   []byte
	SignPub        []byte
	Created        time.Time
}

// Device es un equipo de la cuenta.
type Device struct {
	ID         string
	Name       string
	Platform   string
	CCPVersion string
	Created    time.Time
	LastSeen   time.Time
	Revoked    bool
	// SessionID es la sesión de Keycloak (`sid`) que dio de alta el equipo.
	// Revocar uno revoca a sus hermanos de sesión y cierra la puerta a un alta
	// nueva con el mismo token: sin esto, revocar solo quemaba un id.
	SessionID string
}

// Group es un grupo de dispositivos: un nombre («todas mis Macs») y sus
// miembros. Es una etiqueta y nada más — no autoriza: una revisión sigue
// yendo FIRMADA a una máquina concreta, y publicar «al grupo» es publicar N
// órdenes, una por miembro. Si el grupo mandara, cambiar quién está dentro
// (algo que el servidor sí puede hacer, porque no va firmado) cambiaría a
// quién obedece una orden ya firmada.
type Group struct {
	ID      string
	Name    string
	Members []string
	Created time.Time
	Updated time.Time
}

// Blob es un blob sellado ya verificado en el almacenamiento.
type Blob struct {
	ID   string
	Size int64
}

// Snapshot es un snapshot publicado. En los listados, Manifest y Sig van vacíos.
type Snapshot struct {
	ID         string
	Parent     string
	DeviceID   string
	DeviceName string
	Created    time.Time
	Manifest   []byte
	Sig        []byte
	Size       int64
	Pinned     bool
	// Digest es el sha256 hexadecimal del manifiesto, y solo sale: lo calcula
	// el almacén sobre los bytes que escribe, porque con él se verifica la
	// firma de la cadena y un digest que viniera de fuera comprobaría al
	// manifiesto contra lo que dijera quien lo mandó. Vacío en Snapshot() y en
	// los listados, que ya llevan el manifiesto o no lo necesitan.
	Digest string
	// Pruned marca una lápida: la retención se llevó su manifiesto y sus
	// blobs, y la fila se queda con lo que sostiene la cadena (id, padre,
	// fecha, digest y firma). No se sirve como snapshot —no queda nada que
	// restaurar— pero sí como eslabón: borrarla dejaría un hueco idéntico al
	// que deja un servidor que te quita un snapshot.
	Pruned bool
}

// Revision es una revisión deseada: el estado al que el portal pide que llegue
// un dispositivo. El servidor la guarda y la sirve tal cual; Body y Sig son
// opacos para él y es el cliente quien verifica la firma antes de aplicar
// nada. En los listados, Body y Sig van vacíos.
type Revision struct {
	ID         string
	Prev       string
	DeviceID   string
	DeviceName string
	Snapshot   string
	Base       string
	Body       []byte
	Sig        []byte
	Created    time.Time
	State      string
	Reason     string
	Updated    time.Time
	// By es el dispositivo que la publicó.
	By string
	// Group es el grupo al que se publicó esta orden, si se publicó a uno.
	// NO entra en la firma a propósito: lo firmado ata la orden a SU máquina
	// (Revision.DeviceID), que es lo que impide desviarla. La etiqueta solo
	// sirve para contar después el resultado por dispositivo dentro del
	// grupo, así que un servidor que la cambiara solo estropearía un listado.
	Group string
}

// Store es lo que el servidor necesita persistir. Toda operación va acotada a
// un usuario: no hay forma de pedir algo de otra cuenta.
type Store interface {
	UpsertUser(ctx context.Context, sub, email string) (User, error)
	Vault(ctx context.Context, userID string) (Vault, error)
	CreateVault(ctx context.Context, userID string, v Vault) error
	CreateDevice(ctx context.Context, userID string, d Device) (Device, error)
	Devices(ctx context.Context, userID string) ([]Device, error)
	// SeenDevice marca el último contacto y devuelve el dispositivo. Si está
	// revocado se devuelve igualmente, con Revoked=true: decide quien llama.
	SeenDevice(ctx context.Context, userID, deviceID string, at time.Time) (Device, error)
	RevokeDevice(ctx context.Context, userID, deviceID string, at time.Time) error
	// CreateGroup crea un grupo. Los miembros tienen que ser dispositivos de
	// esta cuenta (ErrNotFound si no) y el nombre no se puede repetir
	// (ErrConflict): en el CLI el nombre ES el asa del grupo.
	CreateGroup(ctx context.Context, userID string, g Group) (Group, error)
	Groups(ctx context.Context, userID string) ([]Group, error)
	Group(ctx context.Context, userID, id string) (Group, error)
	// UpdateGroup cambia nombre y miembros de golpe, con las mismas reglas.
	// Quedarse con el nombre propio no es un choque consigo mismo.
	UpdateGroup(ctx context.Context, userID string, g Group) (Group, error)
	// DeleteGroup borra el grupo, nunca las revisiones que se publicaron con
	// su etiqueta: esas órdenes pasaron de verdad y su historia no cambia.
	DeleteGroup(ctx context.Context, userID, id string) error
	// KnownBlobs devuelve, de ids, los que ya están registrados, con su tamaño.
	KnownBlobs(ctx context.Context, userID string, ids []string) (map[string]int64, error)
	// CommitSnapshot publica un snapshot de forma atómica: registra newBlobs, el
	// snapshot y sus referencias (refs ⊆ ya registrados ∪ newBlobs). Si el
	// snapshot ya existía no cambia nada y devuelve created=false.
	CommitSnapshot(ctx context.Context, userID string, s Snapshot, newBlobs []Blob, refs []string) (created bool, err error)
	// Snapshots lista del más nuevo al más viejo, filtrando por dispositivo si
	// deviceID no está vacío.
	Snapshots(ctx context.Context, userID, deviceID string, limit int) ([]Snapshot, error)
	Snapshot(ctx context.Context, userID, id string) (Snapshot, error)
	// Chain devuelve la historia entera del usuario, del más nuevo al más
	// viejo y sin manifiestos: id, padre, fecha, digest y firma de cada
	// eslabón. Es lo que el cliente necesita para verificar la cadena de un
	// tirón; media cadena no verifica nada, así que el límite es un tope de
	// seguridad (api.MaxChainLinks), no una paginación.
	Chain(ctx context.Context, userID string, limit int) ([]Snapshot, error)
	// PruneSnapshots convierte en lápidas los snapshots ids: les quita el
	// manifiesto y sus referencias, y libera los blobs que ningún snapshot vivo
	// siga usando y que se registraran antes de before. La fila del snapshot NO
	// se borra (ver Snapshot.Pruned). Devuelve los blobs liberados para que
	// quien llama los quite del almacenamiento: primero la base y después el
	// bucket, porque una fila que apunta a un objeto que ya no está es un
	// snapshot roto. Un blob liberado queda APUNTADO como pendiente hasta que
	// se confirme con ForgetBlobs, y cada llamada devuelve también los
	// pendientes de antes: sin esa nota, un borrado que falló en el bucket no
	// lo nombraría nadie nunca más y el objeto ocuparía sitio para siempre.
	// Con ids vacío no poda nada, pero sigue devolviendo los pendientes.
	PruneSnapshots(ctx context.Context, userID string, ids []string, before time.Time) ([]string, error)
	// ForgetBlobs olvida los blobs ids: lo que llama lo hace DESPUÉS de que el
	// objeto se haya ido del bucket, y solo con los que se fueron de verdad.
	ForgetBlobs(ctx context.Context, userID string, ids []string) error
	// SetPinned fija o suelta un snapshot. Un fijado no se poda nunca, y por
	// eso fijar es del cliente: «fijado» es la bandera del manifiesto o el
	// hecho de tener etiqueta, y el manifiesto viaja sellado.
	SetPinned(ctx context.Context, userID, id string, pinned bool) error
	// PublishRevision guarda una revisión deseada como cabeza de la cadena de
	// su dispositivo. r.Prev debe ser la cabeza actual ("" si no hay ninguna)
	// o devuelve ErrConflict, igual que un id repetido; un dispositivo que no
	// existe, ErrNotFound. Si la cabeza seguía pendiente queda RevSuperseded
	// con el id de la nueva por motivo: la orden vieja nunca llegó a la
	// máquina y cerrarla como fallida sería culparla de algo que no hizo.
	PublishRevision(ctx context.Context, userID string, r Revision) (Revision, error)
	// PendingRevision devuelve la cabeza de la cadena del dispositivo si sigue
	// pendiente, entera. ErrNotFound si no hay nada que aplicar.
	PendingRevision(ctx context.Context, userID, deviceID string) (Revision, error)
	// Revision devuelve una revisión entera, con cuerpo y firma.
	Revision(ctx context.Context, userID, id string) (Revision, error)
	// Revisions lista de la más nueva a la más vieja, filtrando por
	// dispositivo si deviceID no está vacío. Sin Body ni Sig.
	Revisions(ctx context.Context, userID, deviceID string, limit int) ([]Revision, error)
	// GroupRevisions lista las revisiones publicadas con la etiqueta de un
	// grupo, de la más nueva a la más vieja y sin Body ni Sig. Es con lo que
	// se cuenta el estado por dispositivo dentro del grupo, y por eso un
	// limit <= 0 las devuelve TODAS: ese estado es la última de CADA equipo, y
	// una ventana dejaría fuera justo la del que lleva más tiempo sin recibir
	// nada — que es el que más falta hace mirar.
	GroupRevisions(ctx context.Context, userID, groupID string, limit int) ([]Revision, error)
	// SetRevisionState anota el resultado. Va acotada al dispositivo
	// destinatario a propósito: que otro equipo cierre una orden ajena sería
	// contar por él lo que no ha hecho. ErrNotFound si no es suya, ErrConflict
	// si ya no estaba pendiente.
	SetRevisionState(ctx context.Context, userID, deviceID, id, state, reason string, at time.Time) (Revision, error)
	Audit(ctx context.Context, userID, deviceID, action string, detail map[string]any) error
	Ping(ctx context.Context) error
}

var uuidRe = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// IsUUID dice si s es un UUID en minúsculas (los ids de usuario y dispositivo).
func IsUUID(s string) bool { return uuidRe.MatchString(s) }
