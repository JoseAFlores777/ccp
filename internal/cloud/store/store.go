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
	Audit(ctx context.Context, userID, deviceID, action string, detail map[string]any) error
	Ping(ctx context.Context) error
}

var uuidRe = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// IsUUID dice si s es un UUID en minúsculas (los ids de usuario y dispositivo).
func IsUUID(s string) bool { return uuidRe.MatchString(s) }
