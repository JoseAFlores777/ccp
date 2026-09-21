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
