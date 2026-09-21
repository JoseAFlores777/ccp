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
// PortalClientID se añadió con el portal (F2-3): es otro cliente público del
// mismo realm, y la pestaña lo necesita ANTES de tener sesión, así que no
// puede salir de ningún endpoint autenticado. Campo nuevo, no forma cambiada:
// un cliente viejo lo ignora.
type Info struct {
	APIVersion     int    `json:"api_version"`
	Issuer         string `json:"issuer"`
	ClientID       string `json:"client_id"`
	PortalClientID string `json:"portal_client_id,omitempty"`
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

// MaxChainLinks es el tope de eslabones que devuelve GET /v1/snapshots/chain.
// La cadena se pide entera —verificar media cadena no verifica nada— y un
// eslabón son unos 200 bytes, así que 20 000 caben en unos pocos megabytes y
// están muy por encima de cualquier historia real.
const MaxChainLinks = 20_000

// ChainLink es un eslabón de la historia (GET /v1/snapshots/chain): lo justo
// para verificar la cadena entera de un tirón, sin bajar un solo manifiesto.
//
// Digest es el sha256 hexadecimal del manifiesto SELLADO, que es lo que entra
// en la firma (`crypt.ManifestDigest`). Lo calcula el servidor sobre los bytes
// que guarda, no lo manda el cliente: un digest de fiar no puede venir de quien
// se quiere comprobar. Si alguna vez dejara de coincidir con el del firmante,
// ninguna firma verificaría y el cliente lo diría de la cadena entera.
type ChainLink struct {
	ID       string    `json:"id"`
	Parent   string    `json:"parent"`
	DeviceID string    `json:"device_id"`
	Created  time.Time `json:"created"`
	Digest   string    `json:"digest"`
	Sig      []byte    `json:"sig"`
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

// Estados de una revisión deseada. Son parte del protocolo: el portal los
// pinta y el almacén guarda exactamente estas cadenas.
//
// `superseded` no estaba en el plan (§10.3 nombra cinco) y hizo falta el
// sexto: publicar una orden nueva sobre otra que la máquina aún no había
// recogido tenía que cerrar la vieja, y llamar «fallida» a una orden que nunca
// llegó a ejecutarse es mentir justo donde el usuario busca el motivo.
const (
	// RevPending: publicada, la máquina aún no ha informado. Solo la cabeza de
	// la cadena de un dispositivo puede estar así.
	RevPending = "pending"
	// RevApplied: aplicada entera.
	RevApplied = "applied"
	// RevPartial: aplicado lo que no hacía falta confirmar; lo ejecutable
	// (hooks, `command` de MCP, `statusLine`, permisos que amplían) espera.
	RevPartial = "partial"
	// RevConflict: el merge a tres bandas chocó y hay que resolverlo a mano.
	RevConflict = "conflict"
	// RevFailed: no se pudo aplicar.
	RevFailed = "failed"
	// RevSuperseded: la reemplazó otra revisión antes de que la máquina
	// informara. No es un fallo de la máquina.
	RevSuperseded = "superseded"
)

// RevStateReported dice si state es un resultado que puede informar la máquina
// destinataria. `pending` y `superseded` los pone el servidor, no el cliente.
func RevStateReported(state string) bool {
	switch state {
	case RevApplied, RevPartial, RevConflict, RevFailed:
		return true
	}
	return false
}

// MaxRevisionBytes es el tope del cuerpo de una revisión (el conjunto de
// cambios, sellado). Una configuración entera viaja como snapshot y aquí solo
// va su id; lo que llega por este campo es un delta, y 256 KiB ya es enorme
// para uno.
const MaxRevisionBytes = 256 << 10

// RevisionIn publica una revisión deseada (POST /v1/revisions): el estado al
// que se pide que llegue un dispositivo. Va firmada con la clave de cuenta,
// que el servidor no tiene: puede negarse a servirla, pero no fabricarla ni
// cambiarle el destinatario sin que la máquina lo note al verificar.
type RevisionIn struct {
	ID string `json:"id"`
	// Prev es la revisión anterior de ESE dispositivo ("" la primera). Debe
	// ser la cabeza actual: encadenar es lo que impide que el servidor quite
	// un eslabón o reordene sin que se vea.
	Prev     string `json:"prev"`
	DeviceID string `json:"device_id"`
	// Snapshot es el snapshot deseado ("" si la revisión solo trae cambios).
	Snapshot string `json:"snapshot"`
	// Base es el último aplicado sobre el que van esos cambios.
	Base string `json:"base"`
	// Body es el conjunto de cambios, sellado. Opaco para el servidor.
	Body    []byte    `json:"body"`
	Sig     []byte    `json:"sig"`
	Created time.Time `json:"created"`
}

// RevisionMeta son los metadatos visibles de una revisión deseada.
type RevisionMeta struct {
	ID         string    `json:"id"`
	Prev       string    `json:"prev"`
	DeviceID   string    `json:"device_id"`
	DeviceName string    `json:"device_name"`
	Snapshot   string    `json:"snapshot"`
	Base       string    `json:"base"`
	Created    time.Time `json:"created"`
	State      string    `json:"state"`
	Reason     string    `json:"reason"`
	Updated    time.Time `json:"updated"`
	// By es el dispositivo que la publicó (el portal también es uno).
	By string `json:"by"`
}

// Revision es una revisión deseada completa (GET /v1/revisions/{id} y
// /v1/revisions/pending). Los listados no traen Body ni Sig: para verificar
// una firma hay que pedir la revisión entera.
type Revision struct {
	RevisionMeta
	Body []byte `json:"body"`
	Sig  []byte `json:"sig"`
}

// RevisionStateIn informa del resultado de aplicar una revisión
// (POST /v1/revisions/{id}/state). Solo la manda el dispositivo destinatario.
type RevisionStateIn struct {
	State  string `json:"state"`
	Reason string `json:"reason"`
}

// MaxReasonLen es el tope del motivo de un estado. Es un mensaje para una
// persona, no un volcado de log.
const MaxReasonLen = 2000
