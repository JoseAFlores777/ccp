// Package api son los tipos del protocolo /v1 entre `ccp` y el backend de la
// nube. Cambiar la forma de un tipo existente rompe a los clientes publicados:
// se añade, no se cambia. Los `[]byte` viajan en base64 (así los codifica
// encoding/json) y la bóveda es opaca para el servidor: su `KDF` llega como
// JSON crudo y se devuelve tal cual.
package api

import (
	"encoding/json"
	"time"
)

const (
	// Version del protocolo; el cliente la comprueba en /v1/info.
	Version = 1
	// HeaderDevice lleva el id del dispositivo en cada petición autenticada.
	HeaderDevice = "X-CCP-Device"
	// MaxBlobBytes: tope de un blob sellado. Cloudflare corta los cuerpos de
	// más de 100 MB, y 64 MiB deja margen.
	MaxBlobBytes = 64 << 20
	// MaxManifestBytes: tope de un manifiesto sellado.
	MaxManifestBytes = 16 << 20
	// MaxPresignIDs: ids por petición de prefirmado.
	MaxPresignIDs = 500
	// MaxCommitIDs: blobs por snapshot al publicarlo. El servidor comprueba
	// uno a uno en el almacenamiento los que no conoce, así que el tope es lo
	// que cabe comprobar dentro del timeout de escritura; 20 000 está muy por
	// encima de cualquier configuración real y aun así se verifica en minutos.
	MaxCommitIDs = 20_000
)

// ValidID acepta solo 64 caracteres hexadecimales en minúscula: los ids de
// blobs y snapshots (HMAC-SHA256) y nada que pueda convertirse en una ruta.
func ValidID(s string) bool {
	if len(s) != 64 {
		return false
	}
	for _, c := range s {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

// Info es la respuesta de GET /v1/info (sin autenticar): dónde autenticarse.
type Info struct {
	APIVersion int    `json:"api_version"`
	Issuer     string `json:"issuer"`
	ClientID   string `json:"client_id"`
}

// Me es la respuesta de GET /v1/me.
type Me struct {
	UserID   string `json:"user_id"`
	Email    string `json:"email"`
	HasVault bool   `json:"has_vault"`
}

// Vault es lo que el servidor guarda de la bóveda. Todo opaco para él: el KDF
// es JSON crudo para que un parámetro que este binario no conozca sobreviva al
// viaje de ida y vuelta.
type Vault struct {
	KDF            json.RawMessage `json:"kdf"`
	PassphraseWrap []byte          `json:"passphrase_wrap"`
	RecoveryWrap   []byte          `json:"recovery_wrap"`
	SignPub        []byte          `json:"sign_pub"`
}

// DeviceIn registra un dispositivo (POST /v1/devices).
type DeviceIn struct {
	Name       string `json:"name"`
	Platform   string `json:"platform"`
	CCPVersion string `json:"ccp_version"`
}

// Device es un dispositivo de la cuenta.
type Device struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	Platform   string    `json:"platform"`
	CCPVersion string    `json:"ccp_version"`
	Created    time.Time `json:"created"`
	LastSeen   time.Time `json:"last_seen"`
	Revoked    bool      `json:"revoked"`
}

// PresignReq pide URLs prefirmadas (POST /v1/blobs/presign). Op es "put" o "get".
type PresignReq struct {
	Op  string   `json:"op"`
	IDs []string `json:"ids"`
}

// PresignItem es la respuesta para un id. En "put", Exists=true significa que
// ya está en el servidor y no hay URL. En "get", Exists=false significa que no
// existe.
type PresignItem struct {
	ID     string `json:"id"`
	Exists bool   `json:"exists"`
	URL    string `json:"url,omitempty"`
}

// SnapshotIn publica un snapshot (POST /v1/snapshots). Blobs son todos los ids
// que referencia el manifiesto y ya se subieron.
type SnapshotIn struct {
	ID       string    `json:"id"`
	Parent   string    `json:"parent"`
	Created  time.Time `json:"created"`
	Manifest []byte    `json:"manifest"`
	Sig      []byte    `json:"sig"`
	Blobs    []string  `json:"blobs"`
}

// SnapshotMeta son los metadatos visibles de un snapshot.
type SnapshotMeta struct {
	ID         string    `json:"id"`
	Parent     string    `json:"parent"`
	DeviceID   string    `json:"device_id"`
	DeviceName string    `json:"device_name"`
	Created    time.Time `json:"created"`
	Size       int64     `json:"size"`
	Pinned     bool      `json:"pinned"`
}

// Snapshot es un snapshot completo (GET /v1/snapshots/{id}). Empotra los
// metadatos sin etiqueta a propósito: el cuerpo es un solo objeto plano.
type Snapshot struct {
	SnapshotMeta
	Manifest []byte `json:"manifest"`
	Sig      []byte `json:"sig"`
}

// Error es el cuerpo de toda respuesta de error. Missing solo aparece cuando
// faltan blobs, porque el cliente distingue por su presencia.
type Error struct {
	Code    string   `json:"code"`
	Message string   `json:"message"`
	Missing []string `json:"missing,omitempty"`
}

// Códigos de Error.
const (
	CodeBadRequest   = "bad_request"
	CodeUnauthorized = "unauthorized"
	CodeForbidden    = "forbidden"
	CodeNotFound     = "not_found"
	CodeConflict     = "conflict"
	CodeMissingBlobs = "missing_blobs"
	CodeTooLarge     = "too_large"
	CodeRateLimited  = "rate_limited"
	CodeInternal     = "internal"
)
