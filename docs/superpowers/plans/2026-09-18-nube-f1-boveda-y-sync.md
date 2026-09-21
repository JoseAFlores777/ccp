# Nube F1: bóveda, dispositivos y sincronización de snapshots — plan de implementación

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** que `ccp cloud` suba los snapshots locales a `https://ccp.joseiz.com` y
los baje en otra máquina, **cifrados de punta a punta**: el servidor guarda solo
texto sellado, ids opacos y firmas.
- La identidad la da el Keycloak del stack `ccp-cloud`.
- El cifrado lo da una **bóveda** que se abre con una frase distinta de la
  contraseña.

Al terminar, una segunda Mac se desbloquea con la frase, trae el historial de la
primera y lo restaura con `ccp snapshot restore`, traduciendo el `HOME` si la otra
máquina lo tiene distinto.

**Architecture:**

```
ccp (CLI) ──────────────────────────────────────────────────────────────────┐
  internal/cli/cloud.go ─► internal/cloud/client (login, API, push/pull)     │
                              │ usa internal/cloud/crypt (AK, sellos, firma) │
                              │ y internal/snapshot (almacén local)          │
                              ▼                                              │
     https://ccp.joseiz.com ─► cmd/ccp-cloud ─► internal/cloud/server        │
                                  ├─ auth: JWT de Keycloak (go-oidc)         │
                                  ├─ internal/cloud/store (Postgres, pgx)    │
                                  └─ internal/cloud/blobs (Alarik, S3)       │
     https://ccp-s3.joseiz.com ◄── URLs prefirmadas: el cliente sube y baja ─┘
                                   los blobs directamente, no pasan por el API
```

`internal/cloud/api` son los tipos del protocolo, compartidos por ambos lados.
El cliente **no importa** `server`, `store` ni `blobs`, así que el binario `ccp`
no arrastra pgx ni el SDK de AWS.

**Tech Stack:** Go 1.24 · `crypto/hkdf`, `crypto/ed25519` (stdlib) ·
`golang.org/x/oauth2` (concesión de dispositivo) ·
`github.com/coreos/go-oidc/v3` (verificar JWT) · `github.com/jackc/pgx/v5` ·
`github.com/aws/aws-sdk-go-v2` (`service/s3`, `credentials`) · `golang.org/x/time/rate`.
Postgres 17, Alarik 1.0.0-beta-16 y Keycloak 26.7.4, ya desplegados en Dokploy
(`deploy/ccp-cloud/`).

**Spec:** `docs/superpowers/specs/2026-09-18-config-unificada-snapshots-nube-design.md`
§10 (backend) y §11 (portabilidad). **Depende de:**
`docs/superpowers/plans/2026-09-18-snapshots-locales.md` implementado (usa
`internal/vault`, `internal/snapshot`, `core.SnapshotRestore` y
`Manifest.Home`).

## Global Constraints

- **Commits: nunca autónomos.** «Commit» = dejar el árbol listo, mostrar el
  mensaje propuesto y esperar autorización explícita del usuario en ese turno.
  Sin trailers `Co-Authored-By` ni atribución a herramientas.
- **Gates de CI, todos verdes antes de proponer commit:** `gofmt -l internal cmd`
  vacío · `go vet ./...` · `go test ./...` · `go run
  github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2 run --timeout 5m`.
- **Go 1.24 en CI.** Cada `go get` nuevo se comprueba igual que en el plan de
  snapshots: si sube la directiva `go` de `go.mod` por encima de `1.24.0`, se
  baja la versión de esa dependencia.
- **`go test ./...` no necesita red, Docker ni Keycloak.** El servidor se prueba
  con implementaciones en memoria y un emisor OIDC falso
  (`internal/cloud/oidctest`). Lo que toca Postgres o Alarik de verdad va tras
  `//go:build integration` (Task 13).
- **El servidor nunca recibe nada que permita abrir un dato:** ni la AK, ni la
  frase, ni el código de recuperación, ni una clave derivada. Lo cifrado se
  cifra en el cliente. Cualquier revisión que vea un secreto cruzando la red
  bloquea el commit.
- **Ningún secreto en logs**, ni en el cliente ni en el servidor. Los tokens y
  las URLs prefirmadas tampoco se registran enteros.
- **Bilingüe:** textos de la CLI en `internal/core/i18n/catalog_cloud.go`
  (prefijo `cli.cloud.`) con `En` y `Es`. Los errores del servidor van en
  español técnico: los lee quien opera, no el usuario final.
- **Tests nunca tocan el estado real:** `CCP_HOME`, `CCP_CLAUDE_SRC`, `HOME` y
  `CCP_DESKTOP_DEFAULT_DATA_DIR` siempre en temporales; `CCP_NO_BROWSER=1` en
  todo test que haga login.
- **El contrato golden no se toca;** `cloud` no entra en la completion, igual que
  `snapshot` y `backup`.
- **Secretos del despliegue:** nunca en git ni en el chat. El compose referencia
  las variables que Dokploy ya generó (`CCP_DB_PASSWORD`, `CCP_S3_ACCESS_KEY`,
  `CCP_S3_SECRET_KEY`). **Nunca se reimporta la plantilla**, porque regeneraría
  todas las contraseñas (`deploy/ccp-cloud/README.md`).

## Decisiones de este plan (precisan el spec)

1. **La AK vive en el equipo en `~/.config/ccp/cloud/vault.key` (0600).**
   - El spec (§10.2) propone envolverla por dispositivo con X25519 y guardar la
     clave del dispositivo en el Keychain. Para un mismo usuario de macOS, un
     archivo 0600 y un Keychain accesible por `security` protegen lo mismo.
   - La envoltura por dispositivo solo hace falta para aprobar un equipo desde
     otro, y eso llega en F3. En F1 un equipo nuevo se desbloquea con la frase o
     con el código de recuperación.
   - Se anota en el spec (Task 15).
2. **Las métricas Prometheus y la GC de blobs huérfanos en Alarik se quedan para
   F4.** F1 trae logs estructurados, `/healthz`, `/readyz`, límites de tasa y de
   tamaño, y auditoría en Postgres.
3. **Migraciones SQL embebidas con un migrador propio** de unas 40 líneas, con
   lock consultivo, en lugar de `goose`. Una dependencia menos para lo mismo.
4. **La DSN de Postgres se arma por campos** (`CCP_CLOUD_DB_*`), nunca como URL:
   una contraseña generada con `@` o `/` rompería una URL.
5. **El JWKS se lee por la red interna** (`http://keycloak:8080/…`) y de forma
   perezosa: el API arranca aunque Keycloak esté caído, y en ese caso responde
   401 hasta que vuelva.

## Estructura de archivos

| archivo | responsabilidad |
|---|---|
| `internal/vault/keys.go` **(nuevo)** | `DeriveSubkey` (HKDF), `NewRecoveryCode`, `RecoveryKey`. |
| `internal/cloud/crypt/crypt.go` **(nuevo)** | `Account` (subclaves de la AK, ids opacos, sellar blobs y manifiestos, firmar) y la bóveda (`NewVault`, `UnlockPassphrase`, `UnlockRecovery`). |
| `internal/cloud/api/api.go` **(nuevo)** | tipos del protocolo `/v1`, cabeceras, límites, `ValidID`. |
| `internal/core/snapshot_home.go` **(nuevo)** | `translateHome`: el HOME de origen → el de esta máquina, al restaurar. |
| `internal/cloud/oidctest/oidctest.go` **(nuevo)** | emisor OIDC falso: discovery, JWKS, concesión de dispositivo, tokens RS256. |
| `internal/cloud/store/store.go` **(nuevo)** | interfaz `Store`, tipos, errores. |
| `internal/cloud/store/mem.go` **(nuevo)** | implementación en memoria. |
| `internal/cloud/store/pg.go` **(nuevo)** | implementación Postgres (pgxpool) + `Migrate`. |
| `internal/cloud/store/migrations/0001_init.sql` **(nuevo)** | esquema. |
| `internal/cloud/blobs/blobs.go` **(nuevo)** | interfaz `Blobs`, implementación en memoria con servidor HTTP de prefirmado falso. |
| `internal/cloud/blobs/s3.go` **(nuevo)** | implementación S3 (Alarik). |
| `internal/cloud/server/*.go` **(nuevos)** | configuración, auth, middleware, handlers, límites, readiness. |
| `cmd/ccp-cloud/main.go` **(nuevo)** | binario del servidor: env → dependencias → HTTP con apagado ordenado; `healthcheck`. |
| `internal/cloud/client/*.go` **(nuevos)** | archivos locales, login por dispositivo, tokens persistentes, API, push y pull. |
| `internal/cli/cloud.go` **(nuevo)** | `ccp cloud …`. |
| `internal/core/i18n/catalog_cloud.go` **(nuevo)** | claves `cli.cloud.*`. |
| `deploy/ccp-cloud/Dockerfile` **(nuevo)** | imagen del API (distroless, sin root). |
| `deploy/ccp-cloud/docker-compose.yml` | servicio `api`. |
| `deploy/ccp-cloud/test-stack.yml` **(nuevo)** | Postgres + Alarik locales para los tests de integración. |
| `.github/workflows/cloud-image.yml` **(nuevo)** | publica `ghcr.io/joseaflores777/ccp-cloud:<tag>` al etiquetar `cloud-v*`. |
| `docs/adr/0013-cloud-end-to-end-encryption.md`, `docs/adr/0015-identity-keycloak-vault-separate.md` **(nuevos)** | decisiones. |

---

### Task 1: `internal/vault` — subclaves y código de recuperación

**Files:**
- Create: `internal/vault/keys.go`
- Create: `internal/vault/keys_test.go`

**Interfaces:**
- Produces: `vault.DeriveSubkey(master []byte, info string) ([]byte, error)`,
  `vault.NewRecoveryCode() (string, error)`, `vault.RecoveryKey(code string) ([]byte, error)`.

- [ ] **Step 1: Escribir los tests**

```go
package vault

import (
	"bytes"
	"regexp"
	"strings"
	"testing"
)

func TestDeriveSubkey(t *testing.T) {
	master := bytes.Repeat([]byte{9}, KeySize)
	a, err := DeriveSubkey(master, "ccp/v1/data")
	if err != nil || len(a) != KeySize {
		t.Fatalf("DeriveSubkey = %d bytes, %v", len(a), err)
	}
	again, _ := DeriveSubkey(master, "ccp/v1/data")
	other, _ := DeriveSubkey(master, "ccp/v1/sign")
	if !bytes.Equal(a, again) || bytes.Equal(a, other) {
		t.Fatal("mismo uso debe dar la misma subclave, y usos distintos, subclaves distintas")
	}
	if _, err := DeriveSubkey([]byte("corta"), "x"); err == nil {
		t.Fatal("aceptó una clave maestra corta")
	}
}

func TestRecoveryCode(t *testing.T) {
	code, err := NewRecoveryCode()
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`^[A-Z2-7]{4}(-[A-Z2-7]{4}){7}$`).MatchString(code) {
		t.Fatalf("formato del código: %q", code)
	}
	k1, err := RecoveryKey(code)
	if err != nil || len(k1) != KeySize {
		t.Fatalf("RecoveryKey = %v", err)
	}
	// Se acepta escrito a mano: minúsculas, sin guiones, con espacios.
	messy := strings.ToLower(strings.ReplaceAll(code, "-", " "))
	if k2, err := RecoveryKey(messy); err != nil || !bytes.Equal(k1, k2) {
		t.Fatalf("código escrito a mano: %v", err)
	}
	other, _ := NewRecoveryCode()
	if k3, _ := RecoveryKey(other); bytes.Equal(k1, k3) {
		t.Fatal("dos códigos distintos dan la misma clave")
	}
	for _, bad := range []string{"", "ABCD", code + "-ABCD", "0000-1111-2222-3333-4444-5555-6666-7777"} {
		if _, err := RecoveryKey(bad); err == nil {
			t.Errorf("RecoveryKey(%q) sin error", bad)
		}
	}
}
```

- [ ] **Step 2: Comprobar que falla**

Run: `go test ./internal/vault/ -run 'Subkey|Recovery'`
Expected: FAIL, no compila.

- [ ] **Step 3: Implementar `internal/vault/keys.go`**

```go
package vault

import (
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"errors"
	"fmt"
	"strings"
)

// DeriveSubkey deriva de master una subclave de KeySize bytes para un uso (info).
// Con usos distintos las claves son independientes: filtrar la de firmar no da
// la de cifrar.
func DeriveSubkey(master []byte, info string) ([]byte, error) {
	if len(master) != KeySize {
		return nil, fmt.Errorf("vault: la clave maestra mide %d bytes y debería medir %d", len(master), KeySize)
	}
	return hkdf.Key(sha256.New, master, nil, info, KeySize)
}

// Un código de recuperación son 20 bytes aleatorios (160 bits) en base32, en 8
// grupos de 4 («ABCD-EFGH-…»). Tiene entropía de sobra para no necesitar
// Argon2: se deriva con HKDF. Se enseña una sola vez y nunca sale del equipo.
const recoveryBytes = 20

var recoveryEnc = base32.StdEncoding.WithPadding(base32.NoPadding)

// NewRecoveryCode genera un código de recuperación nuevo.
func NewRecoveryCode() (string, error) {
	raw := make([]byte, recoveryBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("vault: sin entropía: %w", err)
	}
	s := recoveryEnc.EncodeToString(raw)
	groups := make([]string, 0, len(s)/4)
	for i := 0; i < len(s); i += 4 {
		groups = append(groups, s[i:i+4])
	}
	return strings.Join(groups, "-"), nil
}

// ErrRecoveryCode: el texto no es un código de recuperación.
var ErrRecoveryCode = errors.New("vault: código de recuperación inválido")

// RecoveryKey convierte un código (con o sin guiones o espacios, en cualquier
// caja) en su clave.
func RecoveryKey(code string) ([]byte, error) {
	clean := strings.ToUpper(strings.NewReplacer("-", "", " ", "").Replace(strings.TrimSpace(code)))
	raw, err := recoveryEnc.DecodeString(clean)
	if err != nil || len(raw) != recoveryBytes {
		return nil, ErrRecoveryCode
	}
	return hkdf.Key(sha256.New, raw, nil, "ccp/v1/recovery", KeySize)
}
```

- [ ] **Step 4: Comprobar que pasa**

Run: `go test ./internal/vault/ && go vet ./internal/vault/`
Expected: PASS.

- [ ] **Step 5: Gates y commit**

Mensaje propuesto: `feat(vault): subclaves HKDF y código de recuperación`

---

### Task 2: `internal/cloud/crypt` — la cuenta y la bóveda

Todo lo que sale de la AK:
- tres subclaves: datos, ids y firma;
- los ids opacos de blobs y snapshots;
- el sellado de blobs y manifiestos;
- la firma que encadena cada snapshot con su padre.

Y la bóveda: dos envolturas de la AK, que el servidor guarda sin poder abrirlas,
más la clave pública de firma, con la que un equipo comprueba al desbloquear que
abrió la bóveda de su cuenta.

**Files:**
- Create: `internal/cloud/crypt/crypt.go`
- Create: `internal/cloud/crypt/crypt_test.go`

**Interfaces:**
- Consumes: `vault.*` (Task 1 y plan de snapshots), `snapshot.MaxBlobSize`.
- Produces: `crypt.Account` + `NewAccount(ak)`, `BlobID`, `SnapshotID`, `SealBlob`,
  `OpenBlob`, `SealManifest`, `OpenManifest`, `Sign`, `Verify`, `SignPublic`;
  `crypt.Wraps{KDF vault.KDFParams; Passphrase, Recovery, SignPub []byte}`;
  `crypt.NewVault(passphrase) (ak []byte, recovery string, w Wraps, err error)`;
  `crypt.UnlockPassphrase(w, passphrase) ([]byte, error)`;
  `crypt.UnlockRecovery(w, code) ([]byte, error)`;
  `crypt.ErrSignature`, `crypt.ErrWrongSecret`.

- [ ] **Step 1: Escribir los tests**

```go
package crypt

import (
	"bytes"
	"errors"
	"testing"

	"github.com/JoseAFlores777/ccp/internal/vault"
)

func acct(t *testing.T) (*Account, []byte) {
	t.Helper()
	ak, err := vault.NewKey()
	if err != nil {
		t.Fatal(err)
	}
	a, err := NewAccount(ak)
	if err != nil {
		t.Fatal(err)
	}
	return a, ak
}

func TestIDsAreOpaqueAndStable(t *testing.T) {
	a, ak := acct(t)
	b, _ := NewAccount(ak)
	other, _ := acct(t)
	h := "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	if a.BlobID(h) != b.BlobID(h) {
		t.Fatal("la misma cuenta debe dar el mismo id: dos equipos deduplican")
	}
	if a.BlobID(h) == h || a.BlobID(h) == other.BlobID(h) {
		t.Fatal("el id no puede ser el hash en claro ni coincidir entre cuentas")
	}
	if a.BlobID(h) == a.SnapshotID(h) {
		t.Fatal("un blob y un snapshot con el mismo origen no pueden compartir id")
	}
	if a.SnapshotID("") != "" {
		t.Fatal("sin padre, el id del padre es vacío")
	}
}

func TestSealOpenBlobAndManifest(t *testing.T) {
	a, _ := acct(t)
	id := a.BlobID("x")
	sealed, err := a.SealBlob(id, []byte("contenido"))
	if err != nil {
		t.Fatal(err)
	}
	if got, err := a.OpenBlob(id, sealed); err != nil || string(got) != "contenido" {
		t.Fatalf("OpenBlob = %q, %v", got, err)
	}
	if _, err := a.OpenBlob(a.BlobID("otro"), sealed); err == nil {
		t.Fatal("un blob abrió bajo otro id")
	}
	m, _ := a.SealManifest("snap", []byte(`{"format":1}`))
	if _, err := a.OpenBlob("snap", m); err == nil {
		t.Fatal("un manifiesto se abrió como blob")
	}
	if got, err := a.OpenManifest("snap", m); err != nil || string(got) != `{"format":1}` {
		t.Fatalf("OpenManifest = %q, %v", got, err)
	}
}

func TestSignVerify(t *testing.T) {
	a, _ := acct(t)
	other, _ := acct(t)
	sealed := []byte("manifiesto sellado")
	sig := a.Sign("id", "padre", sealed)
	if err := a.Verify("id", "padre", sealed, sig); err != nil {
		t.Fatalf("Verify: %v", err)
	}
	for name, err := range map[string]error{
		"otro padre":   a.Verify("id", "otro", sealed, sig),
		"otro id":      a.Verify("otro", "padre", sealed, sig),
		"otro cuerpo":  a.Verify("id", "padre", []byte("cambiado"), sig),
		"otra cuenta":  other.Verify("id", "padre", sealed, sig),
	} {
		if !errors.Is(err, ErrSignature) {
			t.Errorf("%s: err = %v, quiero ErrSignature", name, err)
		}
	}
}

func TestVaultRoundTrip(t *testing.T) {
	ak, code, w, err := NewVault([]byte("frase de la bóveda larga"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(w.Passphrase, ak) || bytes.Contains(w.Recovery, ak) {
		t.Fatal("una envoltura contiene la AK en claro")
	}
	got, err := UnlockPassphrase(w, []byte("frase de la bóveda larga"))
	if err != nil || !bytes.Equal(got, ak) {
		t.Fatalf("UnlockPassphrase: %v", err)
	}
	if got, err := UnlockRecovery(w, code); err != nil || !bytes.Equal(got, ak) {
		t.Fatalf("UnlockRecovery: %v", err)
	}
	if _, err := UnlockPassphrase(w, []byte("otra frase cualquiera")); !errors.Is(err, ErrWrongSecret) {
		t.Fatalf("frase equivocada: %v", err)
	}
	if _, err := UnlockRecovery(w, "ABCD-EFGH"); !errors.Is(err, ErrWrongSecret) {
		t.Fatalf("código mal formado: %v", err)
	}
	// Una bóveda cuya clave pública no corresponde se rechaza aunque abra:
	// sería la bóveda de otra cuenta con la misma frase.
	_, _, other, _ := NewVault([]byte("frase de la bóveda larga"))
	w.SignPub = other.SignPub
	if _, err := UnlockPassphrase(w, []byte("frase de la bóveda larga")); err == nil {
		t.Fatal("aceptó una bóveda cuya clave pública no es la suya")
	}
}
```

- [ ] **Step 2: Comprobar que falla**

Run: `go test ./internal/cloud/crypt/`
Expected: FAIL, no compila.

- [ ] **Step 3: Implementar `internal/cloud/crypt/crypt.go`**

```go
// Package crypt es la criptografía de la nube de ccp (spec §10.1-10.2): la
// clave de cuenta (AK) y todo lo que sale de ella. El servidor nunca ve la AK ni
// nada que permita abrir lo que guarda: recibe textos sellados, ids opacos y
// firmas. Lo usa el cliente; el servidor no importa este paquete.
package crypt

import (
	"bytes"
	"compress/gzip"
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"

	"github.com/JoseAFlores777/ccp/internal/snapshot"
	"github.com/JoseAFlores777/ccp/internal/vault"
)

// Usos de la AK (info de HKDF) y datos asociados de las envolturas. Cambiar
// cualquiera deja ilegible todo lo ya subido: son parte del formato.
const (
	infoData   = "ccp/v1/data"
	infoIDs    = "ccp/v1/ids"
	infoSign   = "ccp/v1/sign"
	adPassWrap = "ccp/v1/wrap/passphrase"
	adRecWrap  = "ccp/v1/wrap/recovery"
)

var (
	// ErrSignature: la firma no corresponde. El snapshot fue alterado, o no es de esta cuenta.
	ErrSignature = errors.New("crypt: la firma no corresponde: el snapshot fue alterado o no es de esta cuenta")
	// ErrWrongSecret: la frase o el código no abren la bóveda.
	ErrWrongSecret = errors.New("crypt: la frase o el código no abren la bóveda")
)

// Account son las claves derivadas de una AK.
type Account struct {
	data []byte
	ids  []byte
	sign ed25519.PrivateKey
}

// NewAccount deriva las claves de uso de la AK.
func NewAccount(ak []byte) (*Account, error) {
	data, err := vault.DeriveSubkey(ak, infoData)
	if err != nil {
		return nil, err
	}
	ids, err := vault.DeriveSubkey(ak, infoIDs)
	if err != nil {
		return nil, err
	}
	seed, err := vault.DeriveSubkey(ak, infoSign)
	if err != nil {
		return nil, err
	}
	return &Account{data: data, ids: ids, sign: ed25519.NewKeyFromSeed(seed)}, nil
}

func (a *Account) mac(kind, v string) string {
	m := hmac.New(sha256.New, a.ids)
	m.Write([]byte(kind))
	m.Write([]byte{0})
	m.Write([]byte(v))
	return hex.EncodeToString(m.Sum(nil))
}

// BlobID es el id en la nube del blob local localHash. Es opaco para el
// servidor, que no puede comprobar si guardas un archivo concreto, y estable
// para la cuenta: dos equipos con el mismo archivo lo suben una sola vez.
func (a *Account) BlobID(localHash string) string { return a.mac("blob", localHash) }

// SnapshotID es el id en la nube del snapshot local localID ("" si no hay).
func (a *Account) SnapshotID(localID string) string {
	if localID == "" {
		return ""
	}
	return a.mac("snapshot", localID)
}

// SealBlob comprime y sella el contenido de un blob. El id va como dato
// asociado: un blob copiado bajo otro id no abre.
func (a *Account) SealBlob(id string, data []byte) ([]byte, error) {
	var b bytes.Buffer
	w := gzip.NewWriter(&b)
	if _, err := w.Write(data); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return vault.Seal(a.data, b.Bytes(), []byte("blob:"+id))
}

// OpenBlob deshace SealBlob.
func (a *Account) OpenBlob(id string, sealed []byte) ([]byte, error) {
	body, err := vault.Open(a.data, sealed, []byte("blob:"+id))
	if err != nil {
		return nil, err
	}
	r, err := gzip.NewReader(bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	defer r.Close()
	data, err := io.ReadAll(io.LimitReader(r, snapshot.MaxBlobSize+1))
	if err != nil {
		return nil, err
	}
	if len(data) > snapshot.MaxBlobSize {
		return nil, fmt.Errorf("crypt: blob de más de %d bytes", snapshot.MaxBlobSize)
	}
	return data, nil
}

// SealManifest sella el JSON de un manifiesto bajo el id del snapshot.
func (a *Account) SealManifest(id string, manifest []byte) ([]byte, error) {
	return vault.Seal(a.data, manifest, []byte("manifest:"+id))
}

// OpenManifest deshace SealManifest.
func (a *Account) OpenManifest(id string, sealed []byte) ([]byte, error) {
	return vault.Open(a.data, sealed, []byte("manifest:"+id))
}

// signed es lo que se firma: id, padre y el hash del manifiesto sellado. Quien
// no tiene la AK no puede ni fabricar un snapshot ni reordenar la cadena.
func signed(id, parent string, sealedManifest []byte) []byte {
	sum := sha256.Sum256(sealedManifest)
	return []byte("ccp/v1/snapshot\n" + id + "\n" + parent + "\n" + hex.EncodeToString(sum[:]))
}

// Sign firma un snapshot sellado.
func (a *Account) Sign(id, parent string, sealedManifest []byte) []byte {
	return ed25519.Sign(a.sign, signed(id, parent, sealedManifest))
}

// Verify comprueba la firma de un snapshot. La clave pública sale de la AK, no
// del servidor: un servidor comprometido no puede sustituirla.
func (a *Account) Verify(id, parent string, sealedManifest, sig []byte) error {
	if !ed25519.Verify(a.SignPublic(), signed(id, parent, sealedManifest), sig) {
		return ErrSignature
	}
	return nil
}

// SignPublic es la clave pública de firma de la cuenta.
func (a *Account) SignPublic() ed25519.PublicKey {
	return a.sign.Public().(ed25519.PublicKey)
}

// Wraps es lo que el servidor guarda de la bóveda: dos envolturas de la AK que
// no sabe abrir y la clave pública de firma.
type Wraps struct {
	KDF        vault.KDFParams
	Passphrase []byte
	Recovery   []byte
	SignPub    []byte
}

// NewVault crea una AK nueva y sus dos envolturas: una con la frase (Argon2id)
// y otra con un código de recuperación, que se devuelve para enseñarlo UNA vez.
func NewVault(passphrase []byte) (ak []byte, recovery string, w Wraps, err error) {
	if ak, err = vault.NewKey(); err != nil {
		return nil, "", Wraps{}, err
	}
	if w.KDF, err = vault.NewKDFParams(); err != nil {
		return nil, "", Wraps{}, err
	}
	kek, err := vault.DeriveKey(passphrase, w.KDF)
	if err != nil {
		return nil, "", Wraps{}, err
	}
	if w.Passphrase, err = vault.Seal(kek, ak, []byte(adPassWrap)); err != nil {
		return nil, "", Wraps{}, err
	}
	if recovery, err = vault.NewRecoveryCode(); err != nil {
		return nil, "", Wraps{}, err
	}
	rk, err := vault.RecoveryKey(recovery)
	if err != nil {
		return nil, "", Wraps{}, err
	}
	if w.Recovery, err = vault.Seal(rk, ak, []byte(adRecWrap)); err != nil {
		return nil, "", Wraps{}, err
	}
	acct, err := NewAccount(ak)
	if err != nil {
		return nil, "", Wraps{}, err
	}
	w.SignPub = acct.SignPublic()
	return ak, recovery, w, nil
}

// UnlockPassphrase abre la bóveda con la frase.
func UnlockPassphrase(w Wraps, passphrase []byte) ([]byte, error) {
	kek, err := vault.DeriveKey(passphrase, w.KDF)
	if err != nil {
		return nil, err
	}
	ak, err := vault.Open(kek, w.Passphrase, []byte(adPassWrap))
	if err != nil {
		return nil, ErrWrongSecret
	}
	return checkAK(ak, w)
}

// UnlockRecovery abre la bóveda con el código de recuperación.
func UnlockRecovery(w Wraps, code string) ([]byte, error) {
	rk, err := vault.RecoveryKey(code)
	if err != nil {
		return nil, ErrWrongSecret
	}
	ak, err := vault.Open(rk, w.Recovery, []byte(adRecWrap))
	if err != nil {
		return nil, ErrWrongSecret
	}
	return checkAK(ak, w)
}

// checkAK exige que la AK abierta corresponda a la clave pública guardada.
func checkAK(ak []byte, w Wraps) ([]byte, error) {
	acct, err := NewAccount(ak)
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(acct.SignPublic(), w.SignPub) {
		return nil, errors.New("crypt: la bóveda abre, pero su clave no corresponde a la cuenta")
	}
	return ak, nil
}
```

- [ ] **Step 4: Comprobar que pasa**

Run: `go test ./internal/cloud/crypt/ && go vet ./internal/cloud/crypt/`
Expected: PASS. `TestVaultRoundTrip` tarda unos segundos: Argon2id con 64 MiB,
tres veces. Es lo esperado.

- [ ] **Step 5: Gates y commit**

Mensaje propuesto: `feat(cloud): claves de cuenta, sellado, firma y bóveda`

---

### Task 3: `internal/cloud/api` — el protocolo `/v1`

Los tipos que cruzan la red, compartidos por el servidor y el cliente. Los
`[]byte` viajan en base64 (así los codifica `encoding/json`). La bóveda es opaca
para el servidor: `KDF` llega como JSON crudo y se devuelve tal cual.

**Files:**
- Create: `internal/cloud/api/api.go`
- Create: `internal/cloud/api/api_test.go`

**Interfaces:**
- Produces:
  - constantes: `Version`, `HeaderDevice`, `MaxBlobBytes`, `MaxManifestBytes`,
    `MaxPresignIDs`;
  - `ValidID(string) bool`;
  - tipos: `Info`, `Me`, `Vault`, `DeviceIn`, `Device`, `PresignReq`,
    `PresignItem`, `SnapshotIn`, `SnapshotMeta`, `Snapshot`, `Error`;
  - códigos de error: `CodeBadRequest`, `CodeUnauthorized`, `CodeForbidden`,
    `CodeNotFound`, `CodeConflict`, `CodeMissingBlobs`, `CodeTooLarge`,
    `CodeRateLimited`, `CodeInternal`.

- [ ] **Step 1: Escribir el test**

```go
package api

import (
	"strings"
	"testing"
)

func TestValidID(t *testing.T) {
	good := strings.Repeat("ab", 32)
	if !ValidID(good) {
		t.Fatal("rechazó un id válido")
	}
	for _, bad := range []string{"", good[:63], good + "a", strings.ToUpper(good), strings.Repeat("g", 64), "../" + good[3:]} {
		if ValidID(bad) {
			t.Errorf("ValidID(%q) = true", bad)
		}
	}
}
```

- [ ] **Step 2: Implementar `internal/cloud/api/api.go`**

```go
// Package api son los tipos del protocolo /v1 entre `ccp` y el backend de la
// nube. Cambiar la forma de un tipo existente rompe a los clientes publicados:
// se añade, no se cambia.
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
)

// ValidID acepta solo 64 caracteres hexadecimales en minúscula: los ids de blobs
// y snapshots (HMAC-SHA256) y nada que pueda convertirse en una ruta.
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

// Vault es lo que el servidor guarda de la bóveda. Todo opaco para él.
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

// Snapshot es un snapshot completo (GET /v1/snapshots/{id}).
type Snapshot struct {
	SnapshotMeta
	Manifest []byte `json:"manifest"`
	Sig      []byte `json:"sig"`
}

// Error es el cuerpo de toda respuesta de error.
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
```

- [ ] **Step 3: Comprobar que pasa**

Run: `go test ./internal/cloud/api/ && go vet ./internal/cloud/api/`
Expected: PASS.

- [ ] **Step 4: Gates y commit**

Mensaje propuesto: `feat(cloud): tipos del protocolo /v1`

---

### Task 4: `core` — traducir el HOME al restaurar en otra máquina

Un snapshot de `/Users/ana` restaurado donde el usuario es `/Users/jose` traería
rutas que no existen. Pasa en tres sitios: las reglas de carpeta de `ccp.yaml`,
los comandos de hooks y de MCP, y la ruta de cada proyecto. `Manifest.Home` (plan
de snapshots) dice cuál era el HOME de origen, y `SnapshotRestore` lo cambia por
el de esta máquina en el contenido de todo lo que no es `state`.

Solo se reemplaza el HOME seguido de `/`, de comilla o de fin de línea:
`/Users/ana` no toca `/Users/anabel`. Las conversaciones (`state`) no se
reescriben: son historia.

**Files:**
- Create: `internal/core/snapshot_home.go`
- Create: `internal/core/snapshot_home_test.go`
- Modify: `internal/core/snapshot_restore.go` (aplica la traducción en el plan; `SnapshotRestoreReport` gana `HomeFrom`/`HomeTo`)
- Modify: `internal/cli/snapshot.go` (`printPlan` avisa de la traducción)
- Modify: `internal/core/i18n/catalog_snapshot.go` (clave `cli.snapshot.home_translated`)

**Interfaces:**
- Produces: `translateHome(data []byte, from, to string) []byte`;
  `SnapshotRestoreReport.HomeFrom`, `SnapshotRestoreReport.HomeTo`
  (`json:"home_from,omitempty"`, `json:"home_to,omitempty"`).

- [ ] **Step 1: Escribir los tests**

```go
package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JoseAFlores777/ccp/internal/snapshot"
)

func TestTranslateHome(t *testing.T) {
	in := "rules:\n  - path: /Users/ana/app\n    profile: work\n  - path: /Users/ana\n" +
		`{"command":"/Users/ana/bin/x","cwd":"/Users/ana","other":"/Users/anabel/y"}` + "\n"
	got := string(translateHome([]byte(in), "/Users/ana", "/home/jose"))
	for _, want := range []string{"/home/jose/app", "path: /home/jose\n", `"/home/jose/bin/x"`, `"cwd":"/home/jose"`, "/Users/anabel/y"} {
		if !strings.Contains(got, want) {
			t.Errorf("falta %q en:\n%s", want, got)
		}
	}
	if string(translateHome([]byte(in), "/Users/ana", "/Users/ana")) != in {
		t.Error("con el mismo HOME no debe cambiar nada")
	}
	bin := []byte{0xff, 0xfe, '/', 'U'}
	if string(translateHome(bin, "/U", "/X")) != string(bin) {
		t.Error("tocó un contenido que no es texto")
	}
}

// Un snapshot hecho con otro HOME se restaura con las rutas de esta máquina:
// la regla de carpeta apunta al proyecto aquí, y el proyecto se encuentra.
func TestSnapshotRestoreTranslatesHome(t *testing.T) {
	home, src, _ := snapFixture(t)
	newHome := os.Getenv("HOME") // snapFixture lo fijó a un temporal
	st, _ := OpenSnapshotStore(home)

	oldHome := "/Users/ana"
	yaml := "version: 2\nprofiles:\n  work:\n    type: official\nrules:\n  - path: " + oldHome + "/proyectos/app\n    profile: work\n"
	local := `{"permissions":{"allow":["Bash(make)"]}}`
	hy, _ := st.PutBlob([]byte(yaml), false)
	hl, _ := st.PutBlob([]byte(local), false)
	key := projectKey(oldHome+"/proyectos/app", "")
	m := &snapshot.Manifest{
		Format: snapshot.FormatVersion, Created: snapNow, Machine: "mac-de-ana", Home: oldHome,
		CCPVersion: Version, Trigger: "manual",
		Items: []snapshot.Item{
			{LPath: "ccp/ccp.yaml", Hash: hy, Size: int64(len(yaml)), Mode: 0o644, Class: snapshot.ClassAuthored},
			{LPath: "project/" + key + "/.claude/settings.local.json", Hash: hl, Size: int64(len(local)), Mode: 0o644,
				Class: snapshot.ClassAuthored, Meta: map[string]string{"path": oldHome + "/proyectos/app"}},
		},
	}
	if err := st.SaveManifest(m); err != nil {
		t.Fatal(err)
	}
	app := filepath.Join(newHome, "proyectos", "app")
	if err := os.MkdirAll(app, 0o755); err != nil {
		t.Fatal(err)
	}

	rep, err := SnapshotRestore(home, src, st, m.ID, SnapshotRestoreOpts{Now: snapNow.Add(time.Hour), Machine: "mac-de-jose"})
	if err != nil {
		t.Fatalf("restore: %v", err)
	}
	if rep.HomeFrom != oldHome || rep.HomeTo != newHome {
		t.Errorf("HomeFrom/HomeTo = %q/%q", rep.HomeFrom, rep.HomeTo)
	}
	cfg, err := Load(home)
	if err != nil || len(cfg.Rules) != 1 || cfg.Rules[0].Path != app {
		t.Fatalf("reglas tras restaurar = %+v, %v; quiero %s", cfg.Rules, err, app)
	}
	if got := readStr(t, filepath.Join(app, ".claude", "settings.local.json")); got != local {
		t.Fatalf("settings.local.json = %q", got)
	}
}
```

- [ ] **Step 2: Comprobar que falla**

Run: `go test ./internal/core/ -run 'TranslateHome'`
Expected: FAIL, no compila.

- [ ] **Step 3: Implementar `internal/core/snapshot_home.go`**

```go
package core

import (
	"bytes"
	"unicode/utf8"
)

// translateHome cambia, en el contenido de un archivo de texto, el HOME de la
// máquina de origen (from) por el de esta (to). Solo cuando va seguido de «/»,
// de comilla o de fin de línea: /Users/ana no toca /Users/anabel. Un contenido
// que no es UTF-8 válido no se toca.
func translateHome(data []byte, from, to string) []byte {
	if from == "" || to == "" || from == to || !utf8.Valid(data) {
		return data
	}
	out := data
	for _, suffix := range []string{"/", `"`, "'", "\n"} {
		out = bytes.ReplaceAll(out, []byte(from+suffix), []byte(to+suffix))
	}
	return out
}
```

- [ ] **Step 4: Aplicarlo en `SnapshotRestore` (`internal/core/snapshot_restore.go`)**

1. Añade a `SnapshotRestoreReport`:
   ```go
   	HomeFrom    string                `json:"home_from,omitempty"`
   	HomeTo      string                `json:"home_to,omitempty"`
   ```
2. Tras `items := selectItems(...)` y la comprobación de vacío, calcula la
   traducción:
   ```go
   	// Otra máquina, otro HOME: las rutas absolutas se traducen al restaurar
   	// (spec §11).
   	userHome, _ := os.UserHomeDir()
   	homeFrom, homeTo := "", ""
   	if m.Home != "" && userHome != "" && m.Home != userHome {
   		homeFrom, homeTo = m.Home, userHome
   	}
   ```
   y rellena `HomeFrom: homeFrom, HomeTo: homeTo` al construir `rep`.
3. En el bucle, **antes** de `snapshotTarget`, traduce la ruta del proyecto con
   un helper para rutas sueltas (añádelo a `snapshot_home.go`):
   ```go
   // translateHomePath es translateHome para una ruta suelta: el HOME exacto o
   // el HOME seguido de «/».
   func translateHomePath(p, from, to string) string {
   	switch {
   	case from == "" || to == "" || from == to:
   		return p
   	case p == from:
   		return to
   	case strings.HasPrefix(p, from+"/"):
   		return to + p[len(from):]
   	}
   	return p
   }
   ```
   ```go
   	if p, ok := it.Meta["path"]; ok && homeFrom != "" {
   		meta := maps.Clone(it.Meta)
   		meta["path"] = translateHomePath(p, homeFrom, homeTo)
   		it.Meta = meta
   	}
   ```
   Pruébalo en `TestTranslateHome`: `/Users/ana` → `/home/jose`;
   `/Users/ana/x` → `/home/jose/x`; `/Users/anabel` no cambia.
4. Tras `data, gerr := st.GetBlob(it.Hash)` sin error, y antes de
   `checkSnapshotCCPYAML` y `targetHolds`:
   ```go
   	if it.Class != snapshot.ClassState {
   		data = translateHome(data, homeFrom, homeTo)
   	}
   ```
   El `data` traducido es lo que se compara y lo que se escribe; `pending` ya lo
   guarda.

Añade `"maps"` a los imports si usas `maps.Clone`.

- [ ] **Step 5: Avisar en la CLI**

`internal/core/i18n/catalog_snapshot.go`:

```go
	"cli.snapshot.home_translated": {
		En: "Paths from %s rewritten to %s (this machine's HOME).",
		Es: "Rutas de %s reescritas a %s (el HOME de esta máquina).",
	},
```

`internal/cli/snapshot.go`, al final de `printPlan`:

```go
	if r.HomeFrom != "" {
		fmt.Fprintln(c.out, mute(c.out, i18n.T(c.lang, "cli.snapshot.home_translated", r.HomeFrom, r.HomeTo)))
	}
```

- [ ] **Step 6: Comprobar que pasa**

Run: `go test ./internal/core/ ./internal/cli/ && go vet ./internal/core/ ./internal/cli/`
Expected: PASS. Los tests del plan de snapshots siguen verdes, porque en ellos
`Manifest.Home` coincide con el HOME del test o está vacío.

- [ ] **Step 7: Gates y commit**

Mensaje propuesto: `feat(core): traducir el HOME de origen al restaurar un snapshot de otra máquina`

---

### Task 5: `internal/cloud/oidctest` — un Keycloak de mentira

Un emisor OIDC que corre dentro del test. Hace cuatro cosas:
- publica discovery y JWKS;
- emite tokens RS256;
- atiende la concesión de dispositivo y la aprueba al instante;
- responde al refresco de tokens.

Imita las **rutas de Keycloak**, así que el código de producción no tiene ramas
de test. Es un paquete normal, no `_test`, porque lo usan los tests del
servidor, del cliente y de la CLI.

**Files:**
- Create: `internal/cloud/oidctest/oidctest.go`
- Create: `internal/cloud/oidctest/oidctest_test.go`

**Interfaces:**
- Produces:
  - `oidctest.ClientID` (`"ccp-cli"`) y `oidctest.Audience` (`"ccp-api"`);
  - `oidctest.New(t testing.TB) *Issuer`;
  - campos y métodos de `Issuer`: `URL`, `JWKSURL()`, `As(sub, email)`,
    `AccessToken()`, `Token(aud []string, ttl time.Duration)`,
    `ForeignToken()` (firmado con otra clave).

- [ ] **Step 1: Escribir el test**

```go
package oidctest

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestIssuerEndpoints(t *testing.T) {
	iss := New(t)
	resp, err := http.Get(iss.URL + "/.well-known/openid-configuration")
	if err != nil {
		t.Fatal(err)
	}
	var d map[string]any
	json.NewDecoder(resp.Body).Decode(&d)
	resp.Body.Close()
	if d["issuer"] != iss.URL || d["jwks_uri"] != iss.JWKSURL() || d["device_authorization_endpoint"] == nil {
		t.Fatalf("discovery = %v", d)
	}
	resp, err = http.PostForm(iss.URL+"/protocol/openid-connect/auth/device", url.Values{"client_id": {ClientID}})
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("device: %v %v", resp, err)
	}
	resp.Body.Close()
	resp, _ = http.PostForm(iss.URL+"/protocol/openid-connect/token", url.Values{
		"grant_type": {"urn:ietf:params:oauth:grant-type:device_code"}, "device_code": {"dc-1"}, "client_id": {ClientID},
	})
	var tok map[string]any
	json.NewDecoder(resp.Body).Decode(&tok)
	resp.Body.Close()
	if at, _ := tok["access_token"].(string); strings.Count(at, ".") != 2 || tok["refresh_token"] == nil {
		t.Fatalf("token = %v", tok)
	}
}
```

- [ ] **Step 2: Implementar `internal/cloud/oidctest/oidctest.go`**

```go
// Package oidctest es un Keycloak de mentira para los tests de la nube. Sirve
// discovery y JWKS, emite tokens RS256 y atiende la concesión de dispositivo
// aprobándola al momento. Las rutas son las de Keycloak (/realms/ccp/…) para
// que el código de producción no necesite ninguna rama de test.
package oidctest

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

const (
	// ClientID es el cliente público de la CLI (el de realm-ccp.json).
	ClientID = "ccp-cli"
	// Audience es la audiencia que exige el API.
	Audience = "ccp-api"
)

// Issuer es el emisor falso.
type Issuer struct {
	URL     string
	key     *rsa.PrivateKey
	foreign *rsa.PrivateKey
	mu      sync.Mutex
	sub     string
	email   string
	issued  int
}

// New arranca un emisor que se para al acabar el test.
func New(t testing.TB) *Issuer {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	i := &Issuer{key: key, foreign: foreign, sub: "user-1", email: "ana@example.com"}
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	i.URL = srv.URL + "/realms/ccp"
	mux.HandleFunc("GET /realms/ccp/.well-known/openid-configuration", i.discovery)
	mux.HandleFunc("GET /realms/ccp/protocol/openid-connect/certs", i.jwks)
	mux.HandleFunc("POST /realms/ccp/protocol/openid-connect/auth/device", i.device)
	mux.HandleFunc("POST /realms/ccp/protocol/openid-connect/token", i.token)
	return i
}

// JWKSURL es donde publica sus claves (la ruta de Keycloak).
func (i *Issuer) JWKSURL() string { return i.URL + "/protocol/openid-connect/certs" }

// As cambia la identidad de los tokens que emita a partir de ahora.
func (i *Issuer) As(sub, email string) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.sub, i.email = sub, email
}

// AccessToken es un token de acceso normal para el API, válido 5 minutos.
func (i *Issuer) AccessToken() string {
	return i.Token([]string{Audience, "account"}, 5*time.Minute)
}

// Token emite un token con las audiencias y la duración dadas (una duración
// negativa lo emite ya caducado).
func (i *Issuer) Token(aud []string, ttl time.Duration) string {
	return i.sign(i.key, i.claims(aud, ttl))
}

// ForeignToken es un token con las claims correctas firmado con otra clave.
func (i *Issuer) ForeignToken() string {
	return i.sign(i.foreign, i.claims([]string{Audience}, 5*time.Minute))
}

func (i *Issuer) claims(aud []string, ttl time.Duration) map[string]any {
	i.mu.Lock()
	sub, email := i.sub, i.email
	i.mu.Unlock()
	now := time.Now()
	return map[string]any{
		"iss": i.URL, "sub": sub, "email": email, "aud": aud, "azp": ClientID, "typ": "Bearer",
		"iat": now.Unix(), "exp": now.Add(ttl).Unix(),
	}
}

var b64 = base64.RawURLEncoding

func (i *Issuer) sign(key *rsa.PrivateKey, claims map[string]any) string {
	hdr, _ := json.Marshal(map[string]string{"alg": "RS256", "typ": "JWT", "kid": "test"})
	body, _ := json.Marshal(claims)
	signing := b64.EncodeToString(hdr) + "." + b64.EncodeToString(body)
	sum := sha256.Sum256([]byte(signing))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, sum[:])
	if err != nil {
		panic(err)
	}
	return signing + "." + b64.EncodeToString(sig)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (i *Issuer) discovery(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, 200, map[string]any{
		"issuer":                                i.URL,
		"jwks_uri":                              i.JWKSURL(),
		"authorization_endpoint":                i.URL + "/protocol/openid-connect/auth",
		"token_endpoint":                        i.URL + "/protocol/openid-connect/token",
		"device_authorization_endpoint":         i.URL + "/protocol/openid-connect/auth/device",
		"id_token_signing_alg_values_supported": []string{"RS256"},
	})
}

func (i *Issuer) jwks(w http.ResponseWriter, _ *http.Request) {
	pub := i.key.PublicKey
	writeJSON(w, 200, map[string]any{"keys": []map[string]string{{
		"kty": "RSA", "kid": "test", "use": "sig", "alg": "RS256",
		"n": b64.EncodeToString(pub.N.Bytes()), "e": b64.EncodeToString(big.NewInt(int64(pub.E)).Bytes()),
	}}})
}

func (i *Issuer) device(w http.ResponseWriter, r *http.Request) {
	if r.FormValue("client_id") != ClientID {
		writeJSON(w, 400, map[string]string{"error": "invalid_client"})
		return
	}
	writeJSON(w, 200, map[string]any{
		"device_code": "dc-1", "user_code": "ABCD-EFGH",
		"verification_uri":          i.URL + "/device",
		"verification_uri_complete": i.URL + "/device?user_code=ABCD-EFGH",
		"expires_in":                600,
		// 1 y no 0: con 0, x/oauth2 espera 5 segundos entre intentos.
		"interval": 1,
	})
}

func (i *Issuer) token(w http.ResponseWriter, r *http.Request) {
	switch r.FormValue("grant_type") {
	case "urn:ietf:params:oauth:grant-type:device_code":
		if r.FormValue("device_code") != "dc-1" {
			writeJSON(w, 400, map[string]string{"error": "invalid_grant"})
			return
		}
	case "refresh_token":
		if r.FormValue("refresh_token") == "" {
			writeJSON(w, 400, map[string]string{"error": "invalid_grant"})
			return
		}
	default:
		writeJSON(w, 400, map[string]string{"error": "unsupported_grant_type"})
		return
	}
	i.mu.Lock()
	i.issued++
	n := i.issued
	i.mu.Unlock()
	writeJSON(w, 200, map[string]any{
		"access_token": i.AccessToken(), "token_type": "Bearer", "expires_in": 300,
		// Cada respuesta trae un refresh token nuevo: el cliente tiene que
		// guardar el último, como con Keycloak.
		"refresh_token": fmt.Sprintf("rt-%d", n), "scope": "openid offline_access",
	})
}
```

- [ ] **Step 3: Comprobar que pasa**

Run: `go test ./internal/cloud/oidctest/ && go vet ./internal/cloud/oidctest/`
Expected: PASS.

- [ ] **Step 4: Gates y commit**

Mensaje propuesto: `test(cloud): emisor OIDC falso con las rutas de Keycloak`

---

### Task 6: `internal/cloud/store` — interfaz, memoria y contrato

La persistencia del servidor detrás de una interfaz. Hay una batería de pruebas
de **contrato** que cumplen tanto la implementación en memoria (esta tarea) como
la de Postgres (Task 7, bajo `integration`).

**Files:**
- Create: `internal/cloud/store/store.go`
- Create: `internal/cloud/store/mem.go`
- Create: `internal/cloud/store/contract_test.go`
- Create: `internal/cloud/store/mem_test.go`

**Interfaces:**
- Produces:
  - errores `store.ErrNotFound` y `store.ErrConflict`;
  - tipos `User`, `Vault`, `Device`, `Blob` y `Snapshot`;
  - la interfaz `store.Store` (métodos abajo), `store.NewMem() *Mem`, y
    `store.IsUUID(string) bool`;
  - interno de test: `runContract(t, Store)`.

- [ ] **Step 1: La interfaz — `internal/cloud/store/store.go`**

```go
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
```

- [ ] **Step 2: El contrato — `internal/cloud/store/contract_test.go`**

```go
package store

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

func hexID(c byte) string { return strings.Repeat(string(c), 64) }

// runContract es lo que toda implementación de Store tiene que cumplir.
func runContract(t *testing.T, s Store) {
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond) // Postgres guarda microsegundos

	u, err := s.UpsertUser(ctx, "sub-1", "ana@x")
	if err != nil || !IsUUID(u.ID) {
		t.Fatalf("UpsertUser = %+v, %v", u, err)
	}
	if again, _ := s.UpsertUser(ctx, "sub-1", "ana@nuevo"); again.ID != u.ID || again.Email != "ana@nuevo" {
		t.Fatalf("el mismo sub debe ser el mismo usuario, con el email al día: %+v", again)
	}
	other, _ := s.UpsertUser(ctx, "sub-2", "otro@x")

	// Bóveda: una por usuario, y no se sobrescribe.
	if _, err := s.Vault(ctx, u.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Vault sin crear: %v", err)
	}
	v := Vault{KDF: []byte(`{"time":3,"memory_kib":65536}`), PassphraseWrap: []byte{1, 2}, RecoveryWrap: []byte{3}, SignPub: []byte{4}}
	if err := s.CreateVault(ctx, u.ID, v); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateVault(ctx, u.ID, v); !errors.Is(err, ErrConflict) {
		t.Fatalf("segunda bóveda: %v", err)
	}
	got, err := s.Vault(ctx, u.ID)
	if err != nil || string(got.PassphraseWrap) != string(v.PassphraseWrap) || string(got.SignPub) != string(v.SignPub) {
		t.Fatalf("Vault = %+v, %v", got, err)
	}
	var a, b any
	json.Unmarshal(got.KDF, &a)
	json.Unmarshal(v.KDF, &b)
	if !reflect.DeepEqual(a, b) { // jsonb reformatea el texto: se compara el valor
		t.Fatalf("KDF = %s", got.KDF)
	}
	if _, err := s.Vault(ctx, other.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("la bóveda de un usuario se ve desde otro")
	}

	// Dispositivos.
	d, err := s.CreateDevice(ctx, u.ID, Device{Name: "mac", Platform: "darwin/arm64", CCPVersion: "2.19.0"})
	if err != nil || !IsUUID(d.ID) {
		t.Fatalf("CreateDevice = %+v, %v", d, err)
	}
	d2, _ := s.CreateDevice(ctx, u.ID, Device{Name: "mac2", Platform: "darwin/arm64"})
	if list, _ := s.Devices(ctx, u.ID); len(list) != 2 {
		t.Fatalf("Devices = %d", len(list))
	}
	if list, _ := s.Devices(ctx, other.ID); len(list) != 0 {
		t.Fatal("los dispositivos de un usuario se ven desde otro")
	}
	if seen, err := s.SeenDevice(ctx, u.ID, d.ID, now); err != nil || !seen.LastSeen.Equal(now) {
		t.Fatalf("SeenDevice = %+v, %v", seen, err)
	}
	if _, err := s.SeenDevice(ctx, other.ID, d.ID, now); !errors.Is(err, ErrNotFound) {
		t.Fatal("otro usuario usó un dispositivo ajeno")
	}
	if _, err := s.SeenDevice(ctx, u.ID, "no-es-un-uuid", now); !errors.Is(err, ErrNotFound) {
		t.Fatalf("id mal formado: %v", err)
	}
	if err := s.RevokeDevice(ctx, u.ID, d.ID, now); err != nil {
		t.Fatal(err)
	}
	if seen, _ := s.SeenDevice(ctx, u.ID, d.ID, now); !seen.Revoked {
		t.Fatal("el dispositivo revocado no aparece como revocado")
	}

	// Blobs y snapshots.
	ba, bb, bc := hexID('a'), hexID('b'), hexID('c')
	if known, _ := s.KnownBlobs(ctx, u.ID, []string{ba, bb}); len(known) != 0 {
		t.Fatalf("KnownBlobs sin nada = %v", known)
	}
	s1 := Snapshot{ID: hexID('1'), DeviceID: d2.ID, Created: now, Manifest: []byte("m1"), Sig: []byte("s1"), Size: 32}
	created, err := s.CommitSnapshot(ctx, u.ID, s1, []Blob{{ba, 10}, {bb, 20}}, []string{ba, bb})
	if err != nil || !created {
		t.Fatalf("CommitSnapshot = %v, %v", created, err)
	}
	if created, err := s.CommitSnapshot(ctx, u.ID, s1, nil, []string{ba, bb}); err != nil || created {
		t.Fatalf("repetir un snapshot = %v, %v; quiero false, nil", created, err)
	}
	if known, _ := s.KnownBlobs(ctx, u.ID, []string{ba, bb, bc}); len(known) != 2 || known[ba] != 10 {
		t.Fatalf("KnownBlobs = %v", known)
	}
	if known, _ := s.KnownBlobs(ctx, other.ID, []string{ba}); len(known) != 0 {
		t.Fatal("los blobs de un usuario se ven desde otro")
	}
	s2 := Snapshot{ID: hexID('2'), Parent: s1.ID, DeviceID: d2.ID, Created: now.Add(time.Minute), Manifest: []byte("m2"), Sig: []byte("s2"), Size: 10}
	if _, err := s.CommitSnapshot(ctx, u.ID, s2, nil, []string{ba}); err != nil {
		t.Fatalf("snapshot que reusa un blob: %v", err)
	}
	s3 := Snapshot{ID: hexID('3'), DeviceID: d2.ID, Created: now, Manifest: []byte("m3"), Sig: []byte("s3")}
	if _, err := s.CommitSnapshot(ctx, u.ID, s3, nil, []string{bc}); err == nil {
		t.Fatal("aceptó un snapshot que referencia un blob sin registrar")
	}
	if _, err := s.Snapshot(ctx, u.ID, s3.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("el snapshot fallido quedó a medias")
	}

	list, err := s.Snapshots(ctx, u.ID, "", 10)
	if err != nil || len(list) != 2 || list[0].ID != s2.ID || list[0].DeviceName != "mac2" || list[0].Manifest != nil {
		t.Fatalf("Snapshots = %+v, %v", list, err)
	}
	if byDev, _ := s.Snapshots(ctx, u.ID, d.ID, 10); len(byDev) != 0 {
		t.Fatal("el filtro por dispositivo no filtra")
	}
	if l, _ := s.Snapshots(ctx, u.ID, "", 1); len(l) != 1 {
		t.Fatal("el límite no limita")
	}
	full, err := s.Snapshot(ctx, u.ID, s1.ID)
	if err != nil || string(full.Manifest) != "m1" || string(full.Sig) != "s1" || !full.Created.Equal(now) {
		t.Fatalf("Snapshot = %+v, %v", full, err)
	}
	if _, err := s.Snapshot(ctx, other.ID, s1.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("un snapshot se ve desde otro usuario")
	}

	if err := s.Audit(ctx, u.ID, d2.ID, "snapshot.commit", map[string]any{"id": s1.ID}); err != nil {
		t.Fatalf("Audit: %v", err)
	}
	if err := s.Ping(ctx); err != nil {
		t.Fatalf("Ping: %v", err)
	}
}
```

`internal/cloud/store/mem_test.go`:

```go
package store

import "testing"

func TestMemContract(t *testing.T) { runContract(t, NewMem()) }
```

- [ ] **Step 3: Comprobar que falla**

Run: `go test ./internal/cloud/store/`
Expected: FAIL, no compila (`undefined: NewMem`).

- [ ] **Step 4: Implementar `internal/cloud/store/mem.go`**

```go
package store

import (
	"context"
	"crypto/rand"
	"fmt"
	"maps"
	"sort"
	"sync"
	"time"
)

// Mem es un Store en memoria: los tests del servidor y del cliente corren sin
// Postgres. Cumple el mismo contrato (contract_test.go).
type Mem struct {
	mu      sync.Mutex
	users   map[string]User               // por sub
	vaults  map[string]Vault              // por usuario
	devices map[string]map[string]Device  // usuario -> id
	blobs   map[string]map[string]int64   // usuario -> blob -> tamaño
	snaps   map[string]map[string]Snapshot
	refs    map[string]map[string][]string // usuario -> snapshot -> blobs
	audit   []string
}

// NewMem devuelve un Store vacío.
func NewMem() *Mem {
	return &Mem{
		users: map[string]User{}, vaults: map[string]Vault{}, devices: map[string]map[string]Device{},
		blobs: map[string]map[string]int64{}, snaps: map[string]map[string]Snapshot{}, refs: map[string]map[string][]string{},
	}
}

func newUUID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func (m *Mem) UpsertUser(_ context.Context, sub, email string) (User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	u, ok := m.users[sub]
	if !ok {
		u = User{ID: newUUID(), Sub: sub}
	}
	u.Email = email
	m.users[sub] = u
	return u, nil
}

func (m *Mem) Vault(_ context.Context, userID string) (Vault, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.vaults[userID]
	if !ok {
		return Vault{}, ErrNotFound
	}
	return v, nil
}

func (m *Mem) CreateVault(_ context.Context, userID string, v Vault) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.vaults[userID]; ok {
		return ErrConflict
	}
	v.Created = time.Now().UTC()
	m.vaults[userID] = v
	return nil
}

func (m *Mem) CreateDevice(_ context.Context, userID string, d Device) (Device, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now().UTC()
	d.ID, d.Created, d.LastSeen, d.Revoked = newUUID(), now, now, false
	if m.devices[userID] == nil {
		m.devices[userID] = map[string]Device{}
	}
	m.devices[userID][d.ID] = d
	return d, nil
}

func (m *Mem) Devices(_ context.Context, userID string) ([]Device, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Device, 0, len(m.devices[userID]))
	for _, d := range m.devices[userID] {
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Created.Before(out[j].Created) })
	return out, nil
}

func (m *Mem) SeenDevice(_ context.Context, userID, deviceID string, at time.Time) (Device, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	d, ok := m.devices[userID][deviceID]
	if !ok {
		return Device{}, ErrNotFound
	}
	d.LastSeen = at
	m.devices[userID][deviceID] = d
	return d, nil
}

func (m *Mem) RevokeDevice(_ context.Context, userID, deviceID string, _ time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	d, ok := m.devices[userID][deviceID]
	if !ok {
		return ErrNotFound
	}
	d.Revoked = true
	m.devices[userID][deviceID] = d
	return nil
}

func (m *Mem) KnownBlobs(_ context.Context, userID string, ids []string) (map[string]int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := map[string]int64{}
	for _, id := range ids {
		if size, ok := m.blobs[userID][id]; ok {
			out[id] = size
		}
	}
	return out, nil
}

func (m *Mem) CommitSnapshot(_ context.Context, userID string, s Snapshot, newBlobs []Blob, refs []string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.snaps[userID][s.ID]; ok {
		return false, nil
	}
	d, ok := m.devices[userID][s.DeviceID]
	if !ok {
		return false, ErrNotFound
	}
	// Todo o nada: se comprueba antes de escribir nada.
	incoming := map[string]int64{}
	for _, b := range newBlobs {
		incoming[b.ID] = b.Size
	}
	for _, r := range refs {
		if _, known := m.blobs[userID][r]; !known {
			if _, now := incoming[r]; !now {
				return false, fmt.Errorf("store: el snapshot referencia el blob %s, que no está registrado", r)
			}
		}
	}
	if m.blobs[userID] == nil {
		m.blobs[userID] = map[string]int64{}
		m.snaps[userID] = map[string]Snapshot{}
		m.refs[userID] = map[string][]string{}
	}
	maps.Copy(m.blobs[userID], incoming)
	s.DeviceName = d.Name
	m.snaps[userID][s.ID] = s
	m.refs[userID][s.ID] = append([]string(nil), refs...)
	return true, nil
}

func (m *Mem) Snapshots(_ context.Context, userID, deviceID string, limit int) ([]Snapshot, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []Snapshot
	for _, s := range m.snaps[userID] {
		if deviceID != "" && s.DeviceID != deviceID {
			continue
		}
		s.Manifest, s.Sig = nil, nil
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Created.After(out[j].Created) })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (m *Mem) Snapshot(_ context.Context, userID, id string) (Snapshot, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.snaps[userID][id]
	if !ok {
		return Snapshot{}, ErrNotFound
	}
	return s, nil
}

func (m *Mem) Audit(_ context.Context, userID, deviceID, action string, _ map[string]any) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.audit = append(m.audit, userID+" "+deviceID+" "+action)
	return nil
}

func (m *Mem) Ping(context.Context) error { return nil }
```

- [ ] **Step 5: Comprobar que pasa**

Run: `go test ./internal/cloud/store/ && go vet ./internal/cloud/store/`
Expected: PASS.

- [ ] **Step 6: Gates y commit**

Mensaje propuesto: `feat(cloud): interfaz de persistencia, implementación en memoria y contrato`

---

### Task 7: `internal/cloud/store` — Postgres y migraciones

La implementación de producción. Las migraciones van embebidas y el migrador
toma un lock consultivo, así que dos instancias arrancando a la vez no chocan.
**`CommitSnapshot` es una transacción**, y de ahí salen dos garantías:
- un snapshot a medias no existe;
- una referencia a un blob no registrado la rechaza la clave foránea.

**Files:**
- Create: `internal/cloud/store/migrations/0001_init.sql`
- Create: `internal/cloud/store/pg.go`
- Create: `internal/cloud/store/pg_integration_test.go` (`//go:build integration`)
- Modify: `go.mod`, `go.sum` (`github.com/jackc/pgx/v5`)

**Interfaces:**
- Produces: `store.OpenPG(ctx, dsn, password string) (*PG, error)`,
  `(*PG).Migrate(ctx) error`, `(*PG).Close()`, y `*PG` implementa `Store`.

- [ ] **Step 1: El esquema — `internal/cloud/store/migrations/0001_init.sql`**

```sql
-- Esquema inicial de ccp-cloud. Nada de lo que hay aquí se puede abrir sin la
-- clave de cuenta del usuario: bóvedas, manifiestos y firmas son opacos.

CREATE TABLE users (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    sub        text NOT NULL UNIQUE,
    email      text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE vaults (
    user_id         uuid PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    kdf             jsonb NOT NULL,
    passphrase_wrap bytea NOT NULL,
    recovery_wrap   bytea NOT NULL,
    sign_pub        bytea NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE devices (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    name        text NOT NULL,
    platform    text NOT NULL DEFAULT '',
    ccp_version text NOT NULL DEFAULT '',
    created_at  timestamptz NOT NULL DEFAULT now(),
    last_seen   timestamptz NOT NULL DEFAULT now(),
    revoked_at  timestamptz
);
CREATE INDEX devices_user ON devices (user_id);

CREATE TABLE blobs (
    user_id    uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    id         text NOT NULL,
    size       bigint NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, id)
);

CREATE TABLE snapshots (
    user_id     uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    id          text NOT NULL,
    parent      text NOT NULL DEFAULT '',
    device_id   uuid NOT NULL REFERENCES devices (id),
    created     timestamptz NOT NULL,
    manifest    bytea NOT NULL,
    sig         bytea NOT NULL,
    size        bigint NOT NULL DEFAULT 0,
    pinned      boolean NOT NULL DEFAULT false,
    uploaded_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, id)
);
CREATE INDEX snapshots_user_created ON snapshots (user_id, created DESC);

CREATE TABLE snapshot_blobs (
    user_id     uuid NOT NULL,
    snapshot_id text NOT NULL,
    blob_id     text NOT NULL,
    PRIMARY KEY (user_id, snapshot_id, blob_id),
    FOREIGN KEY (user_id, snapshot_id) REFERENCES snapshots (user_id, id) ON DELETE CASCADE,
    FOREIGN KEY (user_id, blob_id) REFERENCES blobs (user_id, id)
);

-- Solo inserción: el servidor nunca actualiza ni borra filas de aquí.
CREATE TABLE audit_log (
    id        bigserial PRIMARY KEY,
    user_id   uuid REFERENCES users (id) ON DELETE SET NULL,
    device_id uuid,
    action    text NOT NULL,
    detail    jsonb NOT NULL DEFAULT '{}',
    at        timestamptz NOT NULL DEFAULT now()
);
```

- [ ] **Step 2: Añadir la dependencia**

Run: `go get github.com/jackc/pgx/v5@latest`, con la comprobación de la directiva
`go` de siempre.

- [ ] **Step 3: Implementar `internal/cloud/store/pg.go`**

```go
package store

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

// migrateLock es el lock consultivo del migrador: dos instancias que arrancan a
// la vez migran una detrás de otra.
const migrateLock = 727001

// PG es el Store de producción.
type PG struct{ pool *pgxpool.Pool }

// OpenPG abre el pool. dsn es de palabras clave (host=… user=… dbname=…) y la
// contraseña va aparte: una contraseña generada con «@» o «/» rompería una URL.
func OpenPG(ctx context.Context, dsn, password string) (*PG, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("store: DSN inválida: %w", err)
	}
	if password != "" {
		cfg.ConnConfig.Password = password
	}
	cfg.MaxConns = 10
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("store: no se pudo abrir Postgres: %w", err)
	}
	return &PG{pool: pool}, nil
}

// Close cierra el pool.
func (p *PG) Close() { p.pool.Close() }

// Ping comprueba que Postgres responde.
func (p *PG) Ping(ctx context.Context) error { return p.pool.Ping(ctx) }

// Migrate aplica, en orden, las migraciones que falten.
func (p *PG) Migrate(ctx context.Context) error {
	conn, err := p.pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()
	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock($1)`, migrateLock); err != nil {
		return fmt.Errorf("store: lock del migrador: %w", err)
	}
	defer func() { _, _ = conn.Exec(context.Background(), `SELECT pg_advisory_unlock($1)`, migrateLock) }()
	if _, err := conn.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version integer PRIMARY KEY, applied_at timestamptz NOT NULL DEFAULT now())`); err != nil {
		return err
	}
	entries, err := fs.ReadDir(migrationFS, "migrations")
	if err != nil {
		return err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, e := range entries {
		v, err := strconv.Atoi(strings.SplitN(e.Name(), "_", 2)[0])
		if err != nil {
			return fmt.Errorf("store: migración con nombre inválido: %s", e.Name())
		}
		var done bool
		if err := conn.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = $1)`, v).Scan(&done); err != nil {
			return err
		}
		if done {
			continue
		}
		sql, err := migrationFS.ReadFile("migrations/" + e.Name())
		if err != nil {
			return err
		}
		tx, err := conn.Begin(ctx)
		if err != nil {
			return err
		}
		// Sin argumentos, pgx usa el protocolo simple: admite varias sentencias.
		if _, err := tx.Exec(ctx, string(sql)); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("store: migración %s: %w", e.Name(), err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO schema_migrations (version) VALUES ($1)`, v); err != nil {
			_ = tx.Rollback(ctx)
			return err
		}
		if err := tx.Commit(ctx); err != nil {
			return err
		}
	}
	return nil
}

func notFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

func (p *PG) UpsertUser(ctx context.Context, sub, email string) (User, error) {
	var u User
	err := p.pool.QueryRow(ctx, `INSERT INTO users (sub, email) VALUES ($1, $2)
		ON CONFLICT (sub) DO UPDATE SET email = EXCLUDED.email
		RETURNING id::text, sub, email`, sub, email).Scan(&u.ID, &u.Sub, &u.Email)
	return u, err
}

func (p *PG) Vault(ctx context.Context, userID string) (Vault, error) {
	if !IsUUID(userID) {
		return Vault{}, ErrNotFound
	}
	var v Vault
	var kdf string
	err := p.pool.QueryRow(ctx, `SELECT kdf::text, passphrase_wrap, recovery_wrap, sign_pub, created_at
		FROM vaults WHERE user_id = $1::uuid`, userID).Scan(&kdf, &v.PassphraseWrap, &v.RecoveryWrap, &v.SignPub, &v.Created)
	v.KDF = []byte(kdf)
	return v, notFound(err)
}

func (p *PG) CreateVault(ctx context.Context, userID string, v Vault) error {
	if !json.Valid(v.KDF) {
		return fmt.Errorf("store: KDF no es JSON")
	}
	tag, err := p.pool.Exec(ctx, `INSERT INTO vaults (user_id, kdf, passphrase_wrap, recovery_wrap, sign_pub)
		VALUES ($1::uuid, $2::jsonb, $3, $4, $5) ON CONFLICT (user_id) DO NOTHING`,
		userID, string(v.KDF), v.PassphraseWrap, v.RecoveryWrap, v.SignPub)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrConflict
	}
	return nil
}

const deviceCols = `id::text, name, platform, ccp_version, created_at, last_seen, revoked_at IS NOT NULL`

func scanDevice(row pgx.Row) (Device, error) {
	var d Device
	err := row.Scan(&d.ID, &d.Name, &d.Platform, &d.CCPVersion, &d.Created, &d.LastSeen, &d.Revoked)
	return d, notFound(err)
}

func (p *PG) CreateDevice(ctx context.Context, userID string, d Device) (Device, error) {
	return scanDevice(p.pool.QueryRow(ctx, `INSERT INTO devices (user_id, name, platform, ccp_version)
		VALUES ($1::uuid, $2, $3, $4) RETURNING `+deviceCols, userID, d.Name, d.Platform, d.CCPVersion))
}

func (p *PG) Devices(ctx context.Context, userID string) ([]Device, error) {
	rows, err := p.pool.Query(ctx, `SELECT `+deviceCols+` FROM devices WHERE user_id = $1::uuid ORDER BY created_at`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Device{}
	for rows.Next() {
		d, err := scanDevice(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (p *PG) SeenDevice(ctx context.Context, userID, deviceID string, at time.Time) (Device, error) {
	if !IsUUID(deviceID) {
		return Device{}, ErrNotFound
	}
	return scanDevice(p.pool.QueryRow(ctx, `UPDATE devices SET last_seen = $3
		WHERE user_id = $1::uuid AND id = $2::uuid RETURNING `+deviceCols, userID, deviceID, at))
}

func (p *PG) RevokeDevice(ctx context.Context, userID, deviceID string, at time.Time) error {
	if !IsUUID(deviceID) {
		return ErrNotFound
	}
	tag, err := p.pool.Exec(ctx, `UPDATE devices SET revoked_at = COALESCE(revoked_at, $3)
		WHERE user_id = $1::uuid AND id = $2::uuid`, userID, deviceID, at)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (p *PG) KnownBlobs(ctx context.Context, userID string, ids []string) (map[string]int64, error) {
	rows, err := p.pool.Query(ctx, `SELECT id, size FROM blobs WHERE user_id = $1::uuid AND id = ANY($2::text[])`, userID, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int64{}
	for rows.Next() {
		var id string
		var size int64
		if err := rows.Scan(&id, &size); err != nil {
			return nil, err
		}
		out[id] = size
	}
	return out, rows.Err()
}

func (p *PG) CommitSnapshot(ctx context.Context, userID string, s Snapshot, newBlobs []Blob, refs []string) (bool, error) {
	if !IsUUID(s.DeviceID) {
		return false, ErrNotFound
	}
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback(ctx) }() // no-op tras Commit

	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM snapshots WHERE user_id = $1::uuid AND id = $2)`, userID, s.ID).Scan(&exists); err != nil {
		return false, err
	}
	if exists {
		return false, tx.Commit(ctx)
	}
	var devOK bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM devices WHERE user_id = $1::uuid AND id = $2::uuid)`, userID, s.DeviceID).Scan(&devOK); err != nil {
		return false, err
	}
	if !devOK {
		return false, ErrNotFound
	}
	if len(newBlobs) > 0 {
		ids := make([]string, len(newBlobs))
		sizes := make([]int64, len(newBlobs))
		for i, b := range newBlobs {
			ids[i], sizes[i] = b.ID, b.Size
		}
		if _, err := tx.Exec(ctx, `INSERT INTO blobs (user_id, id, size)
			SELECT $1::uuid, unnest($2::text[]), unnest($3::bigint[]) ON CONFLICT DO NOTHING`, userID, ids, sizes); err != nil {
			return false, err
		}
	}
	if _, err := tx.Exec(ctx, `INSERT INTO snapshots (user_id, id, parent, device_id, created, manifest, sig, size)
		VALUES ($1::uuid, $2, $3, $4::uuid, $5, $6, $7, $8)`,
		userID, s.ID, s.Parent, s.DeviceID, s.Created, s.Manifest, s.Sig, s.Size); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return false, nil // otro commit del mismo snapshot ganó la carrera
		}
		return false, err
	}
	if len(refs) > 0 {
		// La clave foránea rechaza una referencia a un blob sin registrar.
		if _, err := tx.Exec(ctx, `INSERT INTO snapshot_blobs (user_id, snapshot_id, blob_id)
			SELECT $1::uuid, $2, unnest($3::text[]) ON CONFLICT DO NOTHING`, userID, s.ID, refs); err != nil {
			return false, fmt.Errorf("store: referencias del snapshot: %w", err)
		}
	}
	return true, tx.Commit(ctx)
}

const snapCols = `s.id, s.parent, s.device_id::text, d.name, s.created, s.size, s.pinned`

func (p *PG) Snapshots(ctx context.Context, userID, deviceID string, limit int) ([]Snapshot, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := p.pool.Query(ctx, `SELECT `+snapCols+` FROM snapshots s JOIN devices d ON d.id = s.device_id
		WHERE s.user_id = $1::uuid AND ($2 = '' OR s.device_id::text = $2)
		ORDER BY s.created DESC LIMIT $3`, userID, deviceID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Snapshot{}
	for rows.Next() {
		var s Snapshot
		if err := rows.Scan(&s.ID, &s.Parent, &s.DeviceID, &s.DeviceName, &s.Created, &s.Size, &s.Pinned); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (p *PG) Snapshot(ctx context.Context, userID, id string) (Snapshot, error) {
	var s Snapshot
	err := p.pool.QueryRow(ctx, `SELECT `+snapCols+`, s.manifest, s.sig FROM snapshots s JOIN devices d ON d.id = s.device_id
		WHERE s.user_id = $1::uuid AND s.id = $2`, userID, id).
		Scan(&s.ID, &s.Parent, &s.DeviceID, &s.DeviceName, &s.Created, &s.Size, &s.Pinned, &s.Manifest, &s.Sig)
	return s, notFound(err)
}

func (p *PG) Audit(ctx context.Context, userID, deviceID, action string, detail map[string]any) error {
	d, err := json.Marshal(detail)
	if err != nil {
		return err
	}
	_, err = p.pool.Exec(ctx, `INSERT INTO audit_log (user_id, device_id, action, detail)
		VALUES ($1::uuid, NULLIF($2, '')::uuid, $3, $4::jsonb)`, userID, deviceID, action, string(d))
	return err
}
```

- [ ] **Step 4: El test de integración — `internal/cloud/store/pg_integration_test.go`**

```go
//go:build integration

package store

import (
	"context"
	"os"
	"testing"
)

// Con la pila de pruebas levantada (deploy/ccp-cloud/test-stack.yml):
//
//	CCP_CLOUD_TEST_DSN='host=127.0.0.1 port=55432 user=postgres password=test dbname=postgres sslmode=disable' \
//	  go test -tags integration ./internal/cloud/store/
func TestPGContract(t *testing.T) {
	dsn := os.Getenv("CCP_CLOUD_TEST_DSN")
	if dsn == "" {
		t.Skip("CCP_CLOUD_TEST_DSN no está definida")
	}
	ctx := context.Background()
	pg, err := OpenPG(ctx, dsn, "")
	if err != nil {
		t.Fatal(err)
	}
	defer pg.Close()
	// Base limpia en cada ejecución: el contrato crea usuarios con subs fijos.
	for _, tbl := range []string{"audit_log", "snapshot_blobs", "snapshots", "blobs", "devices", "vaults", "users", "schema_migrations"} {
		_, _ = pg.pool.Exec(ctx, "DROP TABLE IF EXISTS "+tbl+" CASCADE")
	}
	if err := pg.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if err := pg.Migrate(ctx); err != nil {
		t.Fatalf("Migrate dos veces: %v", err)
	}
	runContract(t, pg)
}
```

- [ ] **Step 5: Comprobar que compila y que los tests sin etiqueta siguen verdes**

Run: `go vet ./internal/cloud/store/ && go vet -tags integration ./internal/cloud/store/ && go test ./internal/cloud/store/`
Expected: PASS. El test de Postgres se ejecuta en la Task 13.

- [ ] **Step 6: Gates y commit**

Mensaje propuesto: `feat(cloud): persistencia en Postgres con migraciones embebidas`

---

### Task 8: `internal/cloud/blobs` — el almacenamiento de blobs

El servidor **nunca** lee ni escribe blobs. Firma URLs para que el cliente los
suba y baje directamente de Alarik, y antes de aceptar un snapshot comprueba con
`HEAD` que los blobs existen. Las claves van por usuario (`u/<usuario>/b/<id>`):
conocer el id de un blob ajeno no da acceso a él.

La implementación en memoria (`blobstest`) monta un servidor HTTP que valida sus
propias URLs firmadas, y así los tests del cliente suben y bajan de verdad. Un
contrato común lo cumplen la memoria y S3 (este último, bajo `integration`
contra Alarik).

**Files:**
- Create: `internal/cloud/blobs/blobs.go`
- Create: `internal/cloud/blobs/s3.go`
- Create: `internal/cloud/blobs/s3_integration_test.go` (`//go:build integration`)
- Create: `internal/cloud/blobs/blobstest/blobstest.go`
- Create: `internal/cloud/blobs/blobstest/blobstest_test.go`
- Modify: `go.mod`, `go.sum` (`github.com/aws/aws-sdk-go-v2`, `…/service/s3`, `…/credentials`)

**Interfaces:**
- Produces:
  - `blobs.Blobs` (interfaz: `PresignPut`, `PresignGet`, `Head`, `Ping`) y
    `blobs.Key(userID, blobID) string`;
  - `blobs.S3Config`, `blobs.NewS3(S3Config) *S3`;
  - `blobstest.New(t) *Mem` y sus métodos de test `Set(key, data)` y
    `Get(key) ([]byte, bool)`;
  - `blobstest.RunContract(t, blobs.Blobs)`.

- [ ] **Step 1: La interfaz — `internal/cloud/blobs/blobs.go`**

```go
// Package blobs es el almacenamiento de blobs sellados del backend. El servidor
// nunca los lee: firma URLs para que el cliente los suba y baje directamente, y
// comprueba que existen antes de aceptar un snapshot.
package blobs

import (
	"context"
	"time"
)

// Blobs es lo que el servidor necesita del almacenamiento.
type Blobs interface {
	PresignPut(ctx context.Context, key string, ttl time.Duration) (string, error)
	PresignGet(ctx context.Context, key string, ttl time.Duration) (string, error)
	// Head dice si key existe y cuánto mide. Que no exista no es un error.
	Head(ctx context.Context, key string) (size int64, exists bool, err error)
	Ping(ctx context.Context) error
}

// Key es la clave de un blob en el bucket, separada por usuario.
func Key(userID, blobID string) string { return "u/" + userID + "/b/" + blobID }
```

- [ ] **Step 2: El contrato y la memoria — `internal/cloud/blobs/blobstest/blobstest.go`**

```go
// Package blobstest da un almacenamiento de blobs en memoria para los tests,
// con un servidor HTTP que valida sus propias URLs firmadas, y el contrato que
// cumple toda implementación de blobs.Blobs.
package blobstest

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/JoseAFlores777/ccp/internal/cloud/api"
	"github.com/JoseAFlores777/ccp/internal/cloud/blobs"
)

// Mem es un blobs.Blobs en memoria servido por HTTP.
type Mem struct {
	mu     sync.Mutex
	data   map[string][]byte
	secret []byte
	base   string
}

// New arranca el almacenamiento; se para al acabar el test.
func New(t testing.TB) *Mem {
	t.Helper()
	m := &Mem{data: map[string][]byte{}, secret: make([]byte, 32)}
	if _, err := rand.Read(m.secret); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(m.serve))
	t.Cleanup(srv.Close)
	m.base = srv.URL
	return m
}

func (m *Mem) mac(method, key string, exp int64) string {
	h := hmac.New(sha256.New, m.secret)
	h.Write([]byte(method + "\n" + key + "\n" + strconv.FormatInt(exp, 10)))
	return hex.EncodeToString(h.Sum(nil))
}

func (m *Mem) presign(method, key string, ttl time.Duration) string {
	exp := time.Now().Add(ttl).Unix()
	q := url.Values{"exp": {strconv.FormatInt(exp, 10)}, "sig": {m.mac(method, key, exp)}}
	return m.base + "/o/" + key + "?" + q.Encode()
}

func (m *Mem) serve(w http.ResponseWriter, r *http.Request) {
	key := strings.TrimPrefix(r.URL.Path, "/o/")
	exp, _ := strconv.ParseInt(r.URL.Query().Get("exp"), 10, 64)
	if time.Now().Unix() > exp || !hmac.Equal([]byte(r.URL.Query().Get("sig")), []byte(m.mac(r.Method, key, exp))) {
		http.Error(w, "firma inválida o caducada", http.StatusForbidden)
		return
	}
	switch r.Method {
	case http.MethodPut:
		body, err := io.ReadAll(io.LimitReader(r.Body, api.MaxBlobBytes+1))
		if err != nil || len(body) > api.MaxBlobBytes {
			http.Error(w, "demasiado grande", http.StatusRequestEntityTooLarge)
			return
		}
		m.Set(key, body)
	case http.MethodGet:
		data, ok := m.Get(key)
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(data)
	default:
		http.Error(w, "método no admitido", http.StatusMethodNotAllowed)
	}
}

// Set guarda data en key saltándose las URLs (para preparar o alterar un test).
func (m *Mem) Set(key string, data []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[key] = bytes.Clone(data)
}

// Get lee key saltándose las URLs.
func (m *Mem) Get(key string) ([]byte, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	d, ok := m.data[key]
	return bytes.Clone(d), ok
}

func (m *Mem) PresignPut(_ context.Context, key string, ttl time.Duration) (string, error) {
	return m.presign(http.MethodPut, key, ttl), nil
}

func (m *Mem) PresignGet(_ context.Context, key string, ttl time.Duration) (string, error) {
	return m.presign(http.MethodGet, key, ttl), nil
}

func (m *Mem) Head(_ context.Context, key string) (int64, bool, error) {
	d, ok := m.Get(key)
	return int64(len(d)), ok, nil
}

func (m *Mem) Ping(context.Context) error { return nil }

// RunContract es lo que toda implementación de blobs.Blobs tiene que cumplir:
// una URL de subida sirve para subir, una de bajada devuelve lo subido y Head
// lo ve.
func RunContract(t *testing.T, b blobs.Blobs) {
	t.Helper()
	ctx := context.Background()
	key := blobs.Key("00000000-0000-4000-8000-000000000000", strings.Repeat("ab", 32))
	if _, ok, err := b.Head(ctx, key); err != nil || ok {
		t.Fatalf("Head antes de subir = %v, %v", ok, err)
	}
	put, err := b.PresignPut(ctx, key, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	body := []byte("contenido sellado de prueba")
	req, _ := http.NewRequestWithContext(ctx, http.MethodPut, put, bytes.NewReader(body))
	req.ContentLength = int64(len(body))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		t.Fatalf("PUT prefirmado = %d", resp.StatusCode)
	}
	if size, ok, err := b.Head(ctx, key); err != nil || !ok || size != int64(len(body)) {
		t.Fatalf("Head tras subir = %d %v %v", size, ok, err)
	}
	get, _ := b.PresignGet(ctx, key, time.Minute)
	resp, err = http.Get(get)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || !bytes.Equal(got, body) {
		t.Fatalf("GET prefirmado = %d %q", resp.StatusCode, got)
	}
	// Una URL de subida no sirve para bajar.
	resp, err = http.Get(put)
	if err == nil {
		resp.Body.Close()
		if resp.StatusCode == 200 {
			t.Fatal("una URL de subida sirvió para bajar")
		}
	}
	if err := b.Ping(ctx); err != nil {
		t.Fatalf("Ping: %v", err)
	}
}
```

`internal/cloud/blobs/blobstest/blobstest_test.go`:

```go
package blobstest

import "testing"

func TestMemContract(t *testing.T) { RunContract(t, New(t)) }
```

- [ ] **Step 3: Comprobar que pasa**

Run: `go test ./internal/cloud/blobs/...`
Expected: PASS.

- [ ] **Step 4: S3 (Alarik) — `internal/cloud/blobs/s3.go`**

Run antes: `go get github.com/aws/aws-sdk-go-v2@latest github.com/aws/aws-sdk-go-v2/service/s3@latest github.com/aws/aws-sdk-go-v2/credentials@latest`,
con la comprobación de la directiva `go`.

```go
package blobs

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awshttp "github.com/aws/aws-sdk-go-v2/aws/transport/http"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// S3Config configura el almacenamiento S3-compatible.
type S3Config struct {
	// Endpoint es la dirección interna (http://alarik:8080) para HEAD y ping.
	Endpoint string
	// PublicEndpoint es la dirección pública (https://ccp-s3.joseiz.com) con la
	// que se firman las URLs. La firma SigV4 incluye el host: una URL firmada
	// para el nombre interno no valdría desde fuera.
	PublicEndpoint string
	Region         string
	Bucket         string
	AccessKey      string
	SecretKey      string
}

// S3 implementa Blobs sobre cualquier S3 compatible. Solo usa el subconjunto
// que prueba el contrato (PUT/GET prefirmados, HEAD de objeto y de bucket), así
// que cambiar Alarik por Garage, SeaweedFS o R2 es cambiar el endpoint.
type S3 struct {
	internal *s3.Client
	presign  *s3.PresignClient
	bucket   string
}

// NewS3 construye el cliente.
func NewS3(c S3Config) *S3 {
	creds := credentials.NewStaticCredentialsProvider(c.AccessKey, c.SecretKey, "")
	client := func(endpoint string) *s3.Client {
		return s3.New(s3.Options{
			Region:       c.Region,
			Credentials:  creds,
			BaseEndpoint: aws.String(endpoint),
			UsePathStyle: true,
			// Las sumas de comprobación «flexibles» que el SDK añade por defecto
			// desde 2025 meten cabeceras en las URLs prefirmadas que un S3
			// compatible puede no aceptar. Solo cuando la operación las exige.
			RequestChecksumCalculation: aws.RequestChecksumCalculationWhenRequired,
			ResponseChecksumValidation: aws.ResponseChecksumValidationWhenRequired,
		})
	}
	return &S3{internal: client(c.Endpoint), presign: s3.NewPresignClient(client(c.PublicEndpoint)), bucket: c.Bucket}
}

func (b *S3) PresignPut(ctx context.Context, key string, ttl time.Duration) (string, error) {
	req, err := b.presign.PresignPutObject(ctx, &s3.PutObjectInput{Bucket: &b.bucket, Key: &key}, s3.WithPresignExpires(ttl))
	if err != nil {
		return "", err
	}
	return req.URL, nil
}

func (b *S3) PresignGet(ctx context.Context, key string, ttl time.Duration) (string, error) {
	req, err := b.presign.PresignGetObject(ctx, &s3.GetObjectInput{Bucket: &b.bucket, Key: &key}, s3.WithPresignExpires(ttl))
	if err != nil {
		return "", err
	}
	return req.URL, nil
}

func (b *S3) Head(ctx context.Context, key string) (int64, bool, error) {
	out, err := b.internal.HeadObject(ctx, &s3.HeadObjectInput{Bucket: &b.bucket, Key: &key})
	if err != nil {
		var nf *types.NotFound
		var re *awshttp.ResponseError
		if errors.As(err, &nf) || (errors.As(err, &re) && re.HTTPStatusCode() == http.StatusNotFound) {
			return 0, false, nil
		}
		return 0, false, err
	}
	return aws.ToInt64(out.ContentLength), true, nil
}

func (b *S3) Ping(ctx context.Context) error {
	_, err := b.internal.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: &b.bucket})
	return err
}
```

`internal/cloud/blobs/s3_integration_test.go`:

```go
//go:build integration

package blobs_test

import (
	"os"
	"testing"

	"github.com/JoseAFlores777/ccp/internal/cloud/blobs"
	"github.com/JoseAFlores777/ccp/internal/cloud/blobs/blobstest"
)

// La prueba de contrato del almacenamiento (spec §10.4) contra un Alarik real,
// el de deploy/ccp-cloud/test-stack.yml.
func TestS3Contract(t *testing.T) {
	ep := os.Getenv("CCP_CLOUD_TEST_S3_ENDPOINT")
	if ep == "" {
		t.Skip("CCP_CLOUD_TEST_S3_ENDPOINT no está definida")
	}
	blobstest.RunContract(t, blobs.NewS3(blobs.S3Config{
		Endpoint: ep, PublicEndpoint: ep, Region: "us-east-1", Bucket: "ccp-blobs",
		AccessKey: os.Getenv("CCP_CLOUD_TEST_S3_ACCESS_KEY"), SecretKey: os.Getenv("CCP_CLOUD_TEST_S3_SECRET_KEY"),
	}))
}
```

- [ ] **Step 5: Comprobar que compila y que los tests sin etiqueta siguen verdes**

Run: `go vet ./internal/cloud/blobs/... && go vet -tags integration ./internal/cloud/blobs/... && go test ./internal/cloud/blobs/...`
Expected: PASS.

- [ ] **Step 6: Gates y commit**

Mensaje propuesto: `feat(cloud): almacenamiento de blobs con URLs prefirmadas (S3 y memoria)`

---

### Task 9: `internal/cloud/server` — el API

**Rutas:**

| ruta | autenticación | qué hace |
|---|---|---|
| `GET /healthz` | — | 204 si el proceso vive |
| `GET /readyz` | — | Postgres, almacenamiento y JWKS alcanzables (caché de 5 s; el detalle del fallo va al log, no a la respuesta) |
| `GET /v1/info` | — | dónde autenticarse (emisor, cliente) |
| `GET /v1/me` | token | crea el usuario en su primera petición; dice si tiene bóveda |
| `POST /v1/devices` | token | registra este equipo |
| `GET /v1/devices` · `DELETE /v1/devices/{id}` | token + dispositivo | lista, revoca |
| `GET /v1/vault` · `PUT /v1/vault` | token + dispositivo | lee o crea (una vez) la bóveda |
| `POST /v1/blobs/presign` | token + dispositivo | URLs para subir o bajar blobs |
| `POST /v1/snapshots` | token + dispositivo | publica un snapshot (verifica con HEAD que sus blobs existen) |
| `GET /v1/snapshots` · `GET /v1/snapshots/{id}` | token + dispositivo | lista, lee |

«Dispositivo» significa la cabecera `X-CCP-Device` con un dispositivo de ese
usuario **no revocado**. Se comprueba en cada petición, así que revocar surte
efecto aunque el token siga vivo 5 minutos.

Límites:
- tasa por usuario: 20 peticiones por segundo, ráfaga de 200;
- tamaño del cuerpo: 64 KiB para dispositivos y bóveda, 1 MiB para prefirmado y
  32 MiB para un snapshot;
- manifiesto sellado: 16 MiB;
- blob sellado: 64 MiB.

**Files:**
- Create: `internal/cloud/server/auth.go`
- Create: `internal/cloud/server/server.go`
- Create: `internal/cloud/server/handlers.go`
- Create: `internal/cloud/server/server_test.go`
- Modify: `go.mod`, `go.sum` (`github.com/coreos/go-oidc/v3`, `golang.org/x/time`)

**Interfaces:**
- Consumes: `store.Store`, `blobs.Blobs`, `api.*`; `oidctest` y `blobstest` (en los tests).
- Produces: `server.Identity`, `server.Verifier`,
  `server.NewOIDCVerifier(issuer, jwksURL, audience string) Verifier`,
  `server.Config`, `server.New(Config) http.Handler`.

- [ ] **Step 1: Escribir los tests — `internal/cloud/server/server_test.go`**

```go
package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/JoseAFlores777/ccp/internal/cloud/api"
	"github.com/JoseAFlores777/ccp/internal/cloud/blobs/blobstest"
	"github.com/JoseAFlores777/ccp/internal/cloud/oidctest"
	"github.com/JoseAFlores777/ccp/internal/cloud/store"
)

type env struct {
	t   *testing.T
	iss *oidctest.Issuer
	st  *store.Mem
	bl  *blobstest.Mem
	url string
}

func newEnv(t *testing.T) *env {
	t.Helper()
	iss := oidctest.New(t)
	st := store.NewMem()
	bl := blobstest.New(t)
	h := New(Config{
		Store: st, Blobs: bl, Verifier: NewOIDCVerifier(iss.URL, iss.JWKSURL(), oidctest.Audience),
		Issuer: iss.URL, ClientID: oidctest.ClientID,
	})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return &env{t: t, iss: iss, st: st, bl: bl, url: srv.URL}
}

// call hace una petición al API y decodifica la respuesta en out (si no es nil).
func (e *env) call(method, path, token, device string, in, out any) int {
	e.t.Helper()
	var body io.Reader
	if in != nil {
		b, _ := json.Marshal(in)
		body = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, e.url+path, body)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if device != "" {
		req.Header.Set(api.HeaderDevice, device)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer resp.Body.Close()
	if out != nil {
		json.NewDecoder(resp.Body).Decode(out)
	}
	return resp.StatusCode
}

func (e *env) newDevice(token, name string) string {
	e.t.Helper()
	var d api.Device
	if code := e.call("POST", "/v1/devices", token, "", api.DeviceIn{Name: name, Platform: "darwin/arm64"}, &d); code != 201 {
		e.t.Fatalf("POST /v1/devices = %d", code)
	}
	return d.ID
}

func id(c string) string { return strings.Repeat(c, 64/len(c)) }

func TestHealthAndInfo(t *testing.T) {
	e := newEnv(t)
	if code := e.call("GET", "/healthz", "", "", nil, nil); code != 204 {
		t.Fatalf("/healthz = %d", code)
	}
	var info api.Info
	if code := e.call("GET", "/v1/info", "", "", nil, &info); code != 200 || info.Issuer != e.iss.URL || info.ClientID != oidctest.ClientID || info.APIVersion != api.Version {
		t.Fatalf("/v1/info = %d %+v", code, info)
	}
}

func TestAuthRejects(t *testing.T) {
	e := newEnv(t)
	for name, tok := range map[string]string{
		"sin token":         "",
		"otra clave":        e.iss.ForeignToken(),
		"caducado":          e.iss.Token([]string{oidctest.Audience}, -time.Minute),
		"otra audiencia":    e.iss.Token([]string{"account"}, time.Minute),
		"basura":            "no.es.un-jwt",
	} {
		if code := e.call("GET", "/v1/me", tok, "", nil, nil); code != 401 {
			t.Errorf("%s: /v1/me = %d, quiero 401", name, code)
		}
	}
	var me api.Me
	if code := e.call("GET", "/v1/me", e.iss.AccessToken(), "", nil, &me); code != 200 || me.Email != "ana@example.com" || me.HasVault {
		t.Fatalf("/v1/me = %d %+v", code, me)
	}
}

func TestDeviceRequiredAndRevocation(t *testing.T) {
	e := newEnv(t)
	tok := e.iss.AccessToken()
	if code := e.call("GET", "/v1/vault", tok, "", nil, nil); code != 400 {
		t.Fatalf("sin cabecera de dispositivo = %d, quiero 400", code)
	}
	if code := e.call("GET", "/v1/vault", tok, "00000000-0000-4000-8000-000000000000", nil, nil); code != 403 {
		t.Fatalf("dispositivo desconocido = %d, quiero 403", code)
	}
	dev := e.newDevice(tok, "mac")
	var list []api.Device
	if code := e.call("GET", "/v1/devices", tok, dev, nil, &list); code != 200 || len(list) != 1 || list[0].Name != "mac" {
		t.Fatalf("GET /v1/devices = %d %+v", code, list)
	}
	other := e.newDevice(tok, "otra")
	if code := e.call("DELETE", "/v1/devices/"+other, tok, dev, nil, nil); code != 204 {
		t.Fatalf("revocar = %d", code)
	}
	if code := e.call("GET", "/v1/devices", tok, other, nil, nil); code != 403 {
		t.Fatalf("dispositivo revocado = %d, quiero 403", code)
	}
}

func TestVaultIsCreatedOnce(t *testing.T) {
	e := newEnv(t)
	tok := e.iss.AccessToken()
	dev := e.newDevice(tok, "mac")
	if code := e.call("GET", "/v1/vault", tok, dev, nil, nil); code != 404 {
		t.Fatalf("sin bóveda = %d", code)
	}
	v := api.Vault{KDF: json.RawMessage(`{"time":3}`), PassphraseWrap: []byte("p"), RecoveryWrap: []byte("r"), SignPub: bytes.Repeat([]byte{1}, 32)}
	if code := e.call("PUT", "/v1/vault", tok, dev, v, nil); code != 201 {
		t.Fatalf("crear bóveda = %d", code)
	}
	if code := e.call("PUT", "/v1/vault", tok, dev, v, nil); code != 409 {
		t.Fatalf("segunda bóveda = %d, quiero 409", code)
	}
	var got api.Vault
	if code := e.call("GET", "/v1/vault", tok, dev, nil, &got); code != 200 || string(got.PassphraseWrap) != "p" {
		t.Fatalf("GET /v1/vault = %d %+v", code, got)
	}
	var me api.Me
	e.call("GET", "/v1/me", tok, "", nil, &me)
	if !me.HasVault {
		t.Fatal("/v1/me no ve la bóveda")
	}
	bad := v
	bad.SignPub = []byte{1}
	if code := e.call("PUT", "/v1/vault", e.iss.AccessToken(), dev, bad, nil); code != 400 && code != 409 {
		t.Fatalf("bóveda inválida = %d", code)
	}
}

func put(t *testing.T, url string, body []byte) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPut, url, bytes.NewReader(body))
	resp, err := http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode/100 != 2 {
		t.Fatalf("PUT prefirmado: %v %v", resp, err)
	}
	resp.Body.Close()
}

func TestPresignCommitAndFetch(t *testing.T) {
	e := newEnv(t)
	tok := e.iss.AccessToken()
	dev := e.newDevice(tok, "mac")
	blob := id("ab")

	var items []api.PresignItem
	if code := e.call("POST", "/v1/blobs/presign", tok, dev, api.PresignReq{Op: "put", IDs: []string{blob}}, &items); code != 200 || len(items) != 1 || items[0].Exists || items[0].URL == "" {
		t.Fatalf("presign put = %d %+v", code, items)
	}
	put(t, items[0].URL, []byte("sellado"))

	in := api.SnapshotIn{ID: id("cd"), Created: time.Now(), Manifest: []byte("m"), Sig: bytes.Repeat([]byte{1}, 64), Blobs: []string{blob}}
	var meta api.SnapshotMeta
	if code := e.call("POST", "/v1/snapshots", tok, dev, in, &meta); code != 201 || meta.DeviceName != "mac" || meta.Size != int64(len("sellado")+1) {
		t.Fatalf("commit = %d %+v", code, meta)
	}
	if code := e.call("POST", "/v1/snapshots", tok, dev, in, nil); code != 200 {
		t.Fatalf("commit repetido = %d, quiero 200", code)
	}
	e.call("POST", "/v1/blobs/presign", tok, dev, api.PresignReq{Op: "put", IDs: []string{blob}}, &items)
	if !items[0].Exists || items[0].URL != "" {
		t.Fatalf("un blob ya publicado no se vuelve a pedir: %+v", items)
	}
	e.call("POST", "/v1/blobs/presign", tok, dev, api.PresignReq{Op: "get", IDs: []string{blob}}, &items)
	resp, err := http.Get(items[0].URL)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if string(got) != "sellado" {
		t.Fatalf("GET prefirmado = %q", got)
	}
	var list []api.SnapshotMeta
	if code := e.call("GET", "/v1/snapshots", tok, dev, nil, &list); code != 200 || len(list) != 1 {
		t.Fatalf("listar = %d %+v", code, list)
	}
	var full api.Snapshot
	if code := e.call("GET", "/v1/snapshots/"+in.ID, tok, dev, nil, &full); code != 200 || string(full.Manifest) != "m" {
		t.Fatalf("leer = %d %+v", code, full)
	}
}

func TestCommitMissingBlobs(t *testing.T) {
	e := newEnv(t)
	tok := e.iss.AccessToken()
	dev := e.newDevice(tok, "mac")
	in := api.SnapshotIn{ID: id("cd"), Created: time.Now(), Manifest: []byte("m"), Sig: bytes.Repeat([]byte{1}, 64), Blobs: []string{id("ab")}}
	var apiErr api.Error
	if code := e.call("POST", "/v1/snapshots", tok, dev, in, &apiErr); code != 409 || apiErr.Code != api.CodeMissingBlobs || len(apiErr.Missing) != 1 {
		t.Fatalf("commit sin subir = %d %+v", code, apiErr)
	}
}

func TestUsersAreIsolated(t *testing.T) {
	e := newEnv(t)
	tokA := e.iss.AccessToken()
	devA := e.newDevice(tokA, "mac-a")
	blob := id("ab")
	var items []api.PresignItem
	e.call("POST", "/v1/blobs/presign", tokA, devA, api.PresignReq{Op: "put", IDs: []string{blob}}, &items)
	put(t, items[0].URL, []byte("de A"))
	in := api.SnapshotIn{ID: id("cd"), Created: time.Now(), Manifest: []byte("m"), Sig: bytes.Repeat([]byte{1}, 64), Blobs: []string{blob}}
	e.call("POST", "/v1/snapshots", tokA, devA, in, nil)

	e.iss.As("user-2", "otro@example.com")
	tokB := e.iss.AccessToken()
	devB := e.newDevice(tokB, "mac-b")
	if code := e.call("GET", "/v1/snapshots/"+in.ID, tokB, devB, nil, nil); code != 404 {
		t.Fatalf("B lee un snapshot de A = %d", code)
	}
	var list []api.SnapshotMeta
	if e.call("GET", "/v1/snapshots", tokB, devB, nil, &list); len(list) != 0 {
		t.Fatalf("B ve los snapshots de A: %+v", list)
	}
	e.call("POST", "/v1/blobs/presign", tokB, devB, api.PresignReq{Op: "get", IDs: []string{blob}}, &items)
	if items[0].Exists || items[0].URL != "" {
		t.Fatalf("B obtiene una URL para un blob de A: %+v", items)
	}
	if code := e.call("GET", "/v1/devices", tokB, devA, nil, nil); code != 403 {
		t.Fatalf("B usa el dispositivo de A = %d", code)
	}
}

func TestLimitsAndValidation(t *testing.T) {
	e := newEnv(t)
	tok := e.iss.AccessToken()
	dev := e.newDevice(tok, "mac")
	big := api.SnapshotIn{ID: id("cd"), Created: time.Now(), Manifest: make([]byte, api.MaxManifestBytes+1), Sig: bytes.Repeat([]byte{1}, 64)}
	if code := e.call("POST", "/v1/snapshots", tok, dev, big, nil); code != 413 {
		t.Fatalf("manifiesto enorme = %d, quiero 413", code)
	}
	for name, req := range map[string]api.PresignReq{
		"op rara":       {Op: "delete", IDs: []string{id("ab")}},
		"id inválido":   {Op: "put", IDs: []string{"../x"}},
		"sin ids":       {Op: "put"},
		"demasiados":    {Op: "put", IDs: make([]string, api.MaxPresignIDs+1)},
	} {
		if code := e.call("POST", "/v1/blobs/presign", tok, dev, req, nil); code != 400 {
			t.Errorf("%s: presign = %d, quiero 400", name, code)
		}
	}
	bad := api.SnapshotIn{ID: "../x", Created: time.Now(), Manifest: []byte("m"), Sig: bytes.Repeat([]byte{1}, 64)}
	if code := e.call("POST", "/v1/snapshots", tok, dev, bad, nil); code != 400 {
		t.Fatalf("id de snapshot inválido = %d", code)
	}
}
```

- [ ] **Step 2: Comprobar que falla**

Run: `go test ./internal/cloud/server/`
Expected: FAIL, no compila.

- [ ] **Step 3: Dependencias**

Run: `go get github.com/coreos/go-oidc/v3@latest golang.org/x/time@latest`, con la
comprobación de la directiva `go`.

- [ ] **Step 4: `internal/cloud/server/auth.go`**

```go
package server

import (
	"context"
	"errors"
	"fmt"

	"github.com/coreos/go-oidc/v3/oidc"
)

// Identity es quién hace la petición, según Keycloak.
type Identity struct {
	Sub   string
	Email string
}

// Verifier valida un token de acceso.
type Verifier interface {
	Verify(ctx context.Context, raw string) (Identity, error)
}

type oidcVerifier struct{ v *oidc.IDTokenVerifier }

// NewOIDCVerifier valida JWT del emisor issuer cuya audiencia incluya audience.
// Las claves se leen de jwksURL de forma perezosa y se refrescan solas cuando
// Keycloak rota: el API arranca aunque Keycloak esté caído y responde 401 hasta
// que vuelva.
func NewOIDCVerifier(issuer, jwksURL, audience string) Verifier {
	ks := oidc.NewRemoteKeySet(context.Background(), jwksURL)
	return &oidcVerifier{v: oidc.NewVerifier(issuer, ks, &oidc.Config{ClientID: audience})}
}

func (o *oidcVerifier) Verify(ctx context.Context, raw string) (Identity, error) {
	tok, err := o.v.Verify(ctx, raw)
	if err != nil {
		return Identity{}, err
	}
	var c struct {
		Email string `json:"email"`
	}
	if err := tok.Claims(&c); err != nil {
		return Identity{}, fmt.Errorf("claims: %w", err)
	}
	if tok.Subject == "" {
		return Identity{}, errors.New("token sin sub")
	}
	return Identity{Sub: tok.Subject, Email: c.Email}, nil
}
```

- [ ] **Step 5: `internal/cloud/server/server.go`**

```go
// Package server es el API HTTP de la nube de ccp (spec §10). Guarda y sirve
// datos opacos: autentica con Keycloak, autoriza por usuario y dispositivo, y
// nunca recibe nada que le permita abrir lo que guarda.
package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"github.com/JoseAFlores777/ccp/internal/cloud/api"
	"github.com/JoseAFlores777/ccp/internal/cloud/blobs"
	"github.com/JoseAFlores777/ccp/internal/cloud/store"
)

// Config son las dependencias del servidor.
type Config struct {
	Store    store.Store
	Blobs    blobs.Blobs
	Verifier Verifier
	Issuer   string // lo que /v1/info anuncia
	ClientID string // el cliente público de la CLI
	Log      *slog.Logger
	Now      func() time.Time
	// Ready comprueba las dependencias para /readyz; nil = siempre listo.
	Ready func(ctx context.Context) error
	// PerUserRate y PerUserBurst limitan las peticiones por usuario (0 = por defecto).
	PerUserRate  rate.Limit
	PerUserBurst int
}

const presignTTL = 15 * time.Minute

type srv struct {
	cfg      Config
	mu       sync.Mutex
	limiters map[string]*rate.Limiter
	readyMu  sync.Mutex
	readyAt  time.Time
	readyErr error
}

// New construye el handler del API.
func New(c Config) http.Handler {
	if c.Log == nil {
		c.Log = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	if c.Now == nil {
		c.Now = time.Now
	}
	if c.PerUserRate == 0 {
		c.PerUserRate = 20
	}
	if c.PerUserBurst == 0 {
		c.PerUserBurst = 200
	}
	s := &srv{cfg: c, limiters: map[string]*rate.Limiter{}}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	mux.HandleFunc("GET /readyz", s.readyz)
	mux.HandleFunc("GET /v1/info", s.info)
	mux.Handle("GET /v1/me", s.authed(s.me, false))
	mux.Handle("POST /v1/devices", s.authed(s.createDevice, false))
	mux.Handle("GET /v1/devices", s.authed(s.listDevices, true))
	mux.Handle("DELETE /v1/devices/{id}", s.authed(s.revokeDevice, true))
	mux.Handle("GET /v1/vault", s.authed(s.getVault, true))
	mux.Handle("PUT /v1/vault", s.authed(s.putVault, true))
	mux.Handle("POST /v1/blobs/presign", s.authed(s.presign, true))
	mux.Handle("POST /v1/snapshots", s.authed(s.commitSnapshot, true))
	mux.Handle("GET /v1/snapshots", s.authed(s.listSnapshots, true))
	mux.Handle("GET /v1/snapshots/{id}", s.authed(s.getSnapshot, true))
	return s.logged(mux)
}

type reqCtx struct {
	user   store.User
	device store.Device
}

type handler func(w http.ResponseWriter, r *http.Request, rc reqCtx)

// authed exige un token válido y, si needDevice, un dispositivo del usuario no
// revocado. El usuario se crea en su primera petición.
func (s *srv) authed(h handler, needDevice bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok || raw == "" {
			writeError(w, http.StatusUnauthorized, api.CodeUnauthorized, "falta el token")
			return
		}
		id, err := s.cfg.Verifier.Verify(r.Context(), raw)
		if err != nil {
			s.cfg.Log.Info("token rechazado", "err", err, "req", reqID(r))
			writeError(w, http.StatusUnauthorized, api.CodeUnauthorized, "token inválido o caducado")
			return
		}
		if !s.allow(id.Sub) {
			writeError(w, http.StatusTooManyRequests, api.CodeRateLimited, "demasiadas peticiones; espera un momento")
			return
		}
		user, err := s.cfg.Store.UpsertUser(r.Context(), id.Sub, id.Email)
		if err != nil {
			s.internal(w, r, err)
			return
		}
		rc := reqCtx{user: user}
		if needDevice {
			dev := r.Header.Get(api.HeaderDevice)
			if !store.IsUUID(dev) {
				writeError(w, http.StatusBadRequest, api.CodeBadRequest, "falta la cabecera "+api.HeaderDevice)
				return
			}
			d, err := s.cfg.Store.SeenDevice(r.Context(), user.ID, dev, s.cfg.Now())
			switch {
			case errors.Is(err, store.ErrNotFound):
				writeError(w, http.StatusForbidden, api.CodeForbidden, "dispositivo desconocido")
				return
			case err != nil:
				s.internal(w, r, err)
				return
			case d.Revoked:
				writeError(w, http.StatusForbidden, api.CodeForbidden, "dispositivo revocado")
				return
			}
			rc.device = d
		}
		h(w, r, rc)
	})
}

func (s *srv) allow(sub string) bool {
	s.mu.Lock()
	l, ok := s.limiters[sub]
	if !ok {
		l = rate.NewLimiter(s.cfg.PerUserRate, s.cfg.PerUserBurst)
		s.limiters[sub] = l
	}
	s.mu.Unlock()
	return l.Allow()
}

type ctxKey struct{}

func reqID(r *http.Request) string {
	id, _ := r.Context().Value(ctxKey{}).(string)
	return id
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

// logged da un id a cada petición, registra método, ruta (sin query: las URLs
// no llevan secretos, pero así no hay que pensarlo), estado y duración, y
// convierte un pánico en un 500.
func (s *srv) logged(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b := make([]byte, 8)
		_, _ = rand.Read(b)
		id := hex.EncodeToString(b)
		w.Header().Set("X-Request-Id", id)
		r = r.WithContext(context.WithValue(r.Context(), ctxKey{}, id))
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		start := time.Now()
		defer func() {
			if p := recover(); p != nil {
				s.cfg.Log.Error("pánico", "panic", p, "req", id)
				writeError(sw, http.StatusInternalServerError, api.CodeInternal, "error interno")
			}
			s.cfg.Log.Info("petición", "req", id, "method", r.Method, "path", r.URL.Path,
				"status", sw.status, "ms", time.Since(start).Milliseconds())
		}()
		next.ServeHTTP(sw, r)
	})
}

func (s *srv) internal(w http.ResponseWriter, r *http.Request, err error) {
	s.cfg.Log.Error("error interno", "err", err, "req", reqID(r))
	writeError(w, http.StatusInternalServerError, api.CodeInternal, "error interno (req "+reqID(r)+")")
}

func (s *srv) audit(r *http.Request, rc reqCtx, action string, detail map[string]any) {
	if err := s.cfg.Store.Audit(r.Context(), rc.user.ID, rc.device.ID, action, detail); err != nil {
		s.cfg.Log.Error("auditoría", "err", err, "req", reqID(r), "action", action)
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, api.Error{Code: code, Message: msg})
}

// decode lee un cuerpo JSON de como mucho max bytes. Ignora campos desconocidos
// a propósito: un cliente más nuevo puede mandar campos que este servidor aún
// no conoce.
func decode(w http.ResponseWriter, r *http.Request, max int64, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, max)
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			writeError(w, http.StatusRequestEntityTooLarge, api.CodeTooLarge, "cuerpo demasiado grande")
			return false
		}
		writeError(w, http.StatusBadRequest, api.CodeBadRequest, "JSON inválido")
		return false
	}
	return true
}
```

- [ ] **Step 6: `internal/cloud/server/handlers.go`**

```go
package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/JoseAFlores777/ccp/internal/cloud/api"
	"github.com/JoseAFlores777/ccp/internal/cloud/blobs"
	"github.com/JoseAFlores777/ccp/internal/cloud/store"
)

func (s *srv) info(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, api.Info{APIVersion: api.Version, Issuer: s.cfg.Issuer, ClientID: s.cfg.ClientID})
}

// readyz no dice qué falló: la respuesta es pública y el detalle va al log.
func (s *srv) readyz(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Ready == nil {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
		return
	}
	s.readyMu.Lock()
	if s.readyAt.IsZero() || s.cfg.Now().Sub(s.readyAt) >= 5*time.Second {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		s.readyErr, s.readyAt = s.cfg.Ready(ctx), s.cfg.Now()
		cancel()
	}
	err := s.readyErr
	s.readyMu.Unlock()
	if err != nil {
		s.cfg.Log.Warn("no preparado", "err", err)
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *srv) me(w http.ResponseWriter, r *http.Request, rc reqCtx) {
	_, err := s.cfg.Store.Vault(r.Context(), rc.user.ID)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		s.internal(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, api.Me{UserID: rc.user.ID, Email: rc.user.Email, HasVault: err == nil})
}

func toAPIDevice(d store.Device) api.Device {
	return api.Device{ID: d.ID, Name: d.Name, Platform: d.Platform, CCPVersion: d.CCPVersion,
		Created: d.Created, LastSeen: d.LastSeen, Revoked: d.Revoked}
}

func (s *srv) createDevice(w http.ResponseWriter, r *http.Request, rc reqCtx) {
	var in api.DeviceIn
	if !decode(w, r, 64<<10, &in) {
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || len(in.Name) > 100 || len(in.Platform) > 100 || len(in.CCPVersion) > 50 {
		writeError(w, http.StatusBadRequest, api.CodeBadRequest, "nombre, plataforma o versión inválidos")
		return
	}
	d, err := s.cfg.Store.CreateDevice(r.Context(), rc.user.ID, store.Device{Name: in.Name, Platform: in.Platform, CCPVersion: in.CCPVersion})
	if err != nil {
		s.internal(w, r, err)
		return
	}
	rc.device = d
	s.audit(r, rc, "device.create", map[string]any{"name": d.Name})
	writeJSON(w, http.StatusCreated, toAPIDevice(d))
}

func (s *srv) listDevices(w http.ResponseWriter, r *http.Request, rc reqCtx) {
	ds, err := s.cfg.Store.Devices(r.Context(), rc.user.ID)
	if err != nil {
		s.internal(w, r, err)
		return
	}
	out := make([]api.Device, 0, len(ds))
	for _, d := range ds {
		out = append(out, toAPIDevice(d))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *srv) revokeDevice(w http.ResponseWriter, r *http.Request, rc reqCtx) {
	id := r.PathValue("id")
	err := s.cfg.Store.RevokeDevice(r.Context(), rc.user.ID, id, s.cfg.Now())
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, api.CodeNotFound, "dispositivo no encontrado")
		return
	}
	if err != nil {
		s.internal(w, r, err)
		return
	}
	s.audit(r, rc, "device.revoke", map[string]any{"device": id})
	w.WriteHeader(http.StatusNoContent)
}

func (s *srv) getVault(w http.ResponseWriter, r *http.Request, rc reqCtx) {
	v, err := s.cfg.Store.Vault(r.Context(), rc.user.ID)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, api.CodeNotFound, "esta cuenta aún no tiene bóveda")
		return
	}
	if err != nil {
		s.internal(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, api.Vault{KDF: json.RawMessage(v.KDF), PassphraseWrap: v.PassphraseWrap, RecoveryWrap: v.RecoveryWrap, SignPub: v.SignPub})
}

func (s *srv) putVault(w http.ResponseWriter, r *http.Request, rc reqCtx) {
	var in api.Vault
	if !decode(w, r, 64<<10, &in) {
		return
	}
	okWrap := func(b []byte) bool { return len(b) > 0 && len(b) <= 4096 }
	if !json.Valid(in.KDF) || len(in.KDF) == 0 || in.KDF[0] != '{' || !okWrap(in.PassphraseWrap) || !okWrap(in.RecoveryWrap) || len(in.SignPub) != 32 {
		writeError(w, http.StatusBadRequest, api.CodeBadRequest, "bóveda inválida")
		return
	}
	err := s.cfg.Store.CreateVault(r.Context(), rc.user.ID, store.Vault{KDF: in.KDF, PassphraseWrap: in.PassphraseWrap, RecoveryWrap: in.RecoveryWrap, SignPub: in.SignPub})
	if errors.Is(err, store.ErrConflict) {
		writeError(w, http.StatusConflict, api.CodeConflict, "esta cuenta ya tiene bóveda; desbloquéala en lugar de crear otra")
		return
	}
	if err != nil {
		s.internal(w, r, err)
		return
	}
	s.audit(r, rc, "vault.create", nil)
	w.WriteHeader(http.StatusCreated)
}

// uniqueIDs valida y deduplica una lista de ids.
func uniqueIDs(ids []string) ([]string, bool) {
	seen := make(map[string]bool, len(ids))
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if !api.ValidID(id) {
			return nil, false
		}
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out, true
}

func (s *srv) presign(w http.ResponseWriter, r *http.Request, rc reqCtx) {
	var in api.PresignReq
	if !decode(w, r, 1<<20, &in) {
		return
	}
	ids, ok := uniqueIDs(in.IDs)
	if !ok || len(ids) == 0 || len(in.IDs) > api.MaxPresignIDs || (in.Op != "put" && in.Op != "get") {
		writeError(w, http.StatusBadRequest, api.CodeBadRequest, "op debe ser put o get, con entre 1 y 500 ids válidos")
		return
	}
	known, err := s.cfg.Store.KnownBlobs(r.Context(), rc.user.ID, ids)
	if err != nil {
		s.internal(w, r, err)
		return
	}
	out := make([]api.PresignItem, 0, len(ids))
	for _, id := range ids {
		_, exists := known[id]
		it := api.PresignItem{ID: id, Exists: exists}
		key := blobs.Key(rc.user.ID, id)
		switch {
		case in.Op == "put" && !exists:
			it.URL, err = s.cfg.Blobs.PresignPut(r.Context(), key, presignTTL)
		case in.Op == "get" && exists:
			it.URL, err = s.cfg.Blobs.PresignGet(r.Context(), key, presignTTL)
		}
		if err != nil {
			s.internal(w, r, err)
			return
		}
		out = append(out, it)
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *srv) commitSnapshot(w http.ResponseWriter, r *http.Request, rc reqCtx) {
	var in api.SnapshotIn
	// base64 infla ×4/3; el doble del manifiesto máximo cubre eso y los ids.
	if !decode(w, r, 2*api.MaxManifestBytes, &in) {
		return
	}
	if len(in.Manifest) > api.MaxManifestBytes {
		writeError(w, http.StatusRequestEntityTooLarge, api.CodeTooLarge, "manifiesto demasiado grande")
		return
	}
	ids, ok := uniqueIDs(in.Blobs)
	if !ok || !api.ValidID(in.ID) || (in.Parent != "" && !api.ValidID(in.Parent)) ||
		len(in.Manifest) == 0 || len(in.Sig) != 64 || in.Created.IsZero() || len(ids) > 200_000 {
		writeError(w, http.StatusBadRequest, api.CodeBadRequest, "snapshot inválido")
		return
	}
	ctx := r.Context()
	known, err := s.cfg.Store.KnownBlobs(ctx, rc.user.ID, ids)
	if err != nil {
		s.internal(w, r, err)
		return
	}
	var unknown []string
	for _, id := range ids {
		if _, ok := known[id]; !ok {
			unknown = append(unknown, id)
		}
	}
	sizes, missing, err := s.headAll(ctx, rc.user.ID, unknown)
	if err != nil {
		s.cfg.Log.Error("almacenamiento", "err", err, "req", reqID(r))
		writeError(w, http.StatusServiceUnavailable, api.CodeInternal, "el almacenamiento no responde; reintenta")
		return
	}
	if len(missing) > 0 {
		writeJSON(w, http.StatusConflict, api.Error{Code: api.CodeMissingBlobs, Message: "faltan blobs por subir", Missing: missing})
		return
	}
	total := int64(len(in.Manifest))
	newBlobs := make([]store.Blob, 0, len(sizes))
	for id, size := range sizes {
		if size > api.MaxBlobBytes {
			writeError(w, http.StatusRequestEntityTooLarge, api.CodeTooLarge, "un blob supera el máximo")
			return
		}
		newBlobs = append(newBlobs, store.Blob{ID: id, Size: size})
		total += size
	}
	for _, size := range known {
		total += size
	}
	snap := store.Snapshot{ID: in.ID, Parent: in.Parent, DeviceID: rc.device.ID, Created: in.Created.UTC(),
		Manifest: in.Manifest, Sig: in.Sig, Size: total}
	created, err := s.cfg.Store.CommitSnapshot(ctx, rc.user.ID, snap, newBlobs, ids)
	if err != nil {
		s.internal(w, r, err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
		s.audit(r, rc, "snapshot.commit", map[string]any{"id": in.ID, "blobs": len(ids), "size": total})
	}
	writeJSON(w, status, api.SnapshotMeta{ID: in.ID, Parent: in.Parent, DeviceID: rc.device.ID, DeviceName: rc.device.Name,
		Created: snap.Created, Size: total})
}

// headAll comprueba en el almacenamiento, 8 en paralelo, que existen los blobs.
func (s *srv) headAll(ctx context.Context, userID string, ids []string) (map[string]int64, []string, error) {
	type result struct {
		id     string
		size   int64
		exists bool
		err    error
	}
	results := make(chan result, len(ids))
	sem := make(chan struct{}, 8)
	var wg sync.WaitGroup
	for _, id := range ids {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			size, ok, err := s.cfg.Blobs.Head(ctx, blobs.Key(userID, id))
			results <- result{id, size, ok, err}
		}()
	}
	wg.Wait()
	close(results)
	sizes := map[string]int64{}
	var missing []string
	for res := range results {
		if res.err != nil {
			return nil, nil, res.err
		}
		if !res.exists {
			missing = append(missing, res.id)
			continue
		}
		sizes[res.id] = res.size
	}
	sort.Strings(missing)
	return sizes, missing, nil
}

func toAPIMeta(sn store.Snapshot) api.SnapshotMeta {
	return api.SnapshotMeta{ID: sn.ID, Parent: sn.Parent, DeviceID: sn.DeviceID, DeviceName: sn.DeviceName,
		Created: sn.Created, Size: sn.Size, Pinned: sn.Pinned}
}

func (s *srv) listSnapshots(w http.ResponseWriter, r *http.Request, rc reqCtx) {
	dev := r.URL.Query().Get("device")
	if dev != "" && !store.IsUUID(dev) {
		writeError(w, http.StatusBadRequest, api.CodeBadRequest, "device inválido")
		return
	}
	limit := 100
	if l := r.URL.Query().Get("limit"); l != "" {
		n, err := strconv.Atoi(l)
		if err != nil || n < 1 || n > 1000 {
			writeError(w, http.StatusBadRequest, api.CodeBadRequest, "limit debe estar entre 1 y 1000")
			return
		}
		limit = n
	}
	list, err := s.cfg.Store.Snapshots(r.Context(), rc.user.ID, dev, limit)
	if err != nil {
		s.internal(w, r, err)
		return
	}
	out := make([]api.SnapshotMeta, 0, len(list))
	for _, sn := range list {
		out = append(out, toAPIMeta(sn))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *srv) getSnapshot(w http.ResponseWriter, r *http.Request, rc reqCtx) {
	id := r.PathValue("id")
	if !api.ValidID(id) {
		writeError(w, http.StatusNotFound, api.CodeNotFound, "snapshot no encontrado")
		return
	}
	sn, err := s.cfg.Store.Snapshot(r.Context(), rc.user.ID, id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, api.CodeNotFound, "snapshot no encontrado")
		return
	}
	if err != nil {
		s.internal(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, api.Snapshot{SnapshotMeta: toAPIMeta(sn), Manifest: sn.Manifest, Sig: sn.Sig})
}
```

- [ ] **Step 7: Comprobar que pasa**

Run: `go test ./internal/cloud/server/ && go vet ./internal/cloud/server/`
Expected: PASS.

- [ ] **Step 8: Gates y commit**

Mensaje propuesto: `feat(cloud): API /v1 con auth de Keycloak, dispositivos, bóveda, blobs y snapshots`

---

### Task 10: `cmd/ccp-cloud` — el binario del servidor

Lee la configuración del entorno. Espera a Postgres (hasta 60 s) y migra. Arma
el handler y sirve con apagado ordenado ante SIGTERM. `ccp-cloud healthcheck`
es el healthcheck de la imagen: distroless no trae ni shell ni curl.

**Files:**
- Create: `cmd/ccp-cloud/main.go`
- Create: `cmd/ccp-cloud/main_test.go`

**Interfaces:**
- Consumes: `store.OpenPG`/`Migrate`, `blobs.NewS3`, `server.New`, `server.NewOIDCVerifier`.
- Produces: el binario. Variables de entorno:
  - requeridas: `CCP_CLOUD_OIDC_ISSUER`, `CCP_CLOUD_DB_PASSWORD`,
    `CCP_CLOUD_S3_ENDPOINT`, `CCP_CLOUD_S3_PUBLIC_ENDPOINT`,
    `CCP_CLOUD_S3_ACCESS_KEY`, `CCP_CLOUD_S3_SECRET_KEY`;
  - con valor por defecto: `CCP_CLOUD_ADDR` (`:8080`), `CCP_CLOUD_OIDC_JWKS`
    (`<issuer>/protocol/openid-connect/certs`), `CCP_CLOUD_OIDC_AUDIENCE`
    (`ccp-api`), `CCP_CLOUD_OIDC_CLIENT_ID` (`ccp-cli`), `CCP_CLOUD_DB_HOST`
    (`postgres`), `CCP_CLOUD_DB_PORT` (`5432`), `CCP_CLOUD_DB_USER` (`ccp`),
    `CCP_CLOUD_DB_NAME` (`ccp`), `CCP_CLOUD_DB_SSLMODE` (`disable`),
    `CCP_CLOUD_S3_REGION` (`us-east-1`), `CCP_CLOUD_S3_BUCKET` (`ccp-blobs`).

- [ ] **Step 1: Escribir el test**

```go
package main

import (
	"strings"
	"testing"
)

func TestLoadConfigListsWhatIsMissing(t *testing.T) {
	for _, k := range []string{"CCP_CLOUD_OIDC_ISSUER", "CCP_CLOUD_DB_PASSWORD", "CCP_CLOUD_S3_ENDPOINT",
		"CCP_CLOUD_S3_PUBLIC_ENDPOINT", "CCP_CLOUD_S3_ACCESS_KEY", "CCP_CLOUD_S3_SECRET_KEY"} {
		t.Setenv(k, "")
	}
	_, err := loadConfig()
	if err == nil || !strings.Contains(err.Error(), "CCP_CLOUD_DB_PASSWORD") || !strings.Contains(err.Error(), "CCP_CLOUD_S3_SECRET_KEY") {
		t.Fatalf("err = %v", err)
	}
	t.Setenv("CCP_CLOUD_OIDC_ISSUER", "https://auth/realms/ccp")
	t.Setenv("CCP_CLOUD_DB_PASSWORD", "p@ss/w:rd") // símbolos que romperían una URL
	t.Setenv("CCP_CLOUD_S3_ENDPOINT", "http://alarik:8080")
	t.Setenv("CCP_CLOUD_S3_PUBLIC_ENDPOINT", "https://s3")
	t.Setenv("CCP_CLOUD_S3_ACCESS_KEY", "a")
	t.Setenv("CCP_CLOUD_S3_SECRET_KEY", "s")
	c, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if c.jwks != "https://auth/realms/ccp/protocol/openid-connect/certs" || c.addr != ":8080" || strings.Contains(c.dsn(), "p@ss") {
		t.Fatalf("config = %+v, dsn %q", c, c.dsn())
	}
}
```

- [ ] **Step 2: Implementar `cmd/ccp-cloud/main.go`**

```go
// Command ccp-cloud es el backend de la nube de ccp (spec §10). Se configura por
// variables de entorno (deploy/ccp-cloud/docker-compose.yml) y guarda solo datos
// opacos: nunca recibe nada que le permita abrirlos.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/JoseAFlores777/ccp/internal/cloud/blobs"
	"github.com/JoseAFlores777/ccp/internal/cloud/server"
	"github.com/JoseAFlores777/ccp/internal/cloud/store"
)

// version la fija el build (-ldflags "-X main.version=…").
var version = "dev"

func main() {
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		os.Exit(healthcheck())
	}
	log := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	if err := run(log); err != nil {
		log.Error("ccp-cloud terminó con error", "err", err)
		os.Exit(1)
	}
}

type config struct {
	addr, issuer, jwks, audience, clientID                  string
	dbHost, dbUser, dbPassword, dbName, dbSSLMode           string
	dbPort                                                  int
	s3                                                      blobs.S3Config
}

// dsn es la DSN de palabras clave, SIN contraseña (va aparte a OpenPG).
func (c config) dsn() string {
	return fmt.Sprintf("host=%s port=%d user=%s dbname=%s sslmode=%s", c.dbHost, c.dbPort, c.dbUser, c.dbName, c.dbSSLMode)
}

func loadConfig() (config, error) {
	var c config
	var missing []string
	get := func(k, def string) string {
		if v := os.Getenv(k); v != "" {
			return v
		}
		if def == "" {
			missing = append(missing, k)
		}
		return def
	}
	c.addr = get("CCP_CLOUD_ADDR", ":8080")
	c.issuer = strings.TrimSuffix(get("CCP_CLOUD_OIDC_ISSUER", ""), "/")
	c.jwks = get("CCP_CLOUD_OIDC_JWKS", c.issuer+"/protocol/openid-connect/certs")
	c.audience = get("CCP_CLOUD_OIDC_AUDIENCE", "ccp-api")
	c.clientID = get("CCP_CLOUD_OIDC_CLIENT_ID", "ccp-cli")
	c.dbHost = get("CCP_CLOUD_DB_HOST", "postgres")
	port, err := strconv.Atoi(get("CCP_CLOUD_DB_PORT", "5432"))
	if err != nil {
		return c, fmt.Errorf("CCP_CLOUD_DB_PORT no es un número")
	}
	c.dbPort = port
	c.dbUser = get("CCP_CLOUD_DB_USER", "ccp")
	c.dbPassword = get("CCP_CLOUD_DB_PASSWORD", "")
	c.dbName = get("CCP_CLOUD_DB_NAME", "ccp")
	c.dbSSLMode = get("CCP_CLOUD_DB_SSLMODE", "disable")
	c.s3 = blobs.S3Config{
		Endpoint: get("CCP_CLOUD_S3_ENDPOINT", ""), PublicEndpoint: get("CCP_CLOUD_S3_PUBLIC_ENDPOINT", ""),
		Region: get("CCP_CLOUD_S3_REGION", "us-east-1"), Bucket: get("CCP_CLOUD_S3_BUCKET", "ccp-blobs"),
		AccessKey: get("CCP_CLOUD_S3_ACCESS_KEY", ""), SecretKey: get("CCP_CLOUD_S3_SECRET_KEY", ""),
	}
	if len(missing) > 0 {
		return c, fmt.Errorf("faltan variables de entorno: %s", strings.Join(missing, ", "))
	}
	return c, nil
}

func run(log *slog.Logger) error {
	c, err := loadConfig()
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pg, err := openPG(ctx, log, c)
	if err != nil {
		return err
	}
	defer pg.Close()
	if err := pg.Migrate(ctx); err != nil {
		return fmt.Errorf("migraciones: %w", err)
	}
	bl := blobs.NewS3(c.s3)
	jwksHTTP := &http.Client{Timeout: 3 * time.Second}
	ready := func(ctx context.Context) error {
		if err := pg.Ping(ctx); err != nil {
			return fmt.Errorf("postgres: %w", err)
		}
		if err := bl.Ping(ctx); err != nil {
			return fmt.Errorf("almacenamiento: %w", err)
		}
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, c.jwks, nil)
		resp, err := jwksHTTP.Do(req)
		if err != nil {
			return fmt.Errorf("keycloak: %w", err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("keycloak: el JWKS respondió %d", resp.StatusCode)
		}
		return nil
	}
	h := server.New(server.Config{
		Store: pg, Blobs: bl, Verifier: server.NewOIDCVerifier(c.issuer, c.jwks, c.audience),
		Issuer: c.issuer, ClientID: c.clientID, Log: log, Ready: ready,
	})
	hs := &http.Server{Addr: c.addr, Handler: h, ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout: 5 * time.Minute, WriteTimeout: 5 * time.Minute, IdleTimeout: 2 * time.Minute}
	errc := make(chan error, 1)
	go func() {
		log.Info("ccp-cloud escuchando", "addr", c.addr, "version", version)
		errc <- hs.ListenAndServe()
	}()
	select {
	case err := <-errc:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
	}
	sctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	log.Info("apagando")
	return hs.Shutdown(sctx)
}

// openPG espera a Postgres hasta 60 s: en un arranque en frío del stack, la base
// puede tardar más que el healthcheck en aceptar conexiones de verdad.
func openPG(ctx context.Context, log *slog.Logger, c config) (*store.PG, error) {
	deadline := time.Now().Add(60 * time.Second)
	for {
		pg, err := store.OpenPG(ctx, c.dsn(), c.dbPassword)
		if err == nil {
			if err = pg.Ping(ctx); err == nil {
				return pg, nil
			}
			pg.Close()
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("postgres no responde: %w", err)
		}
		log.Warn("esperando a postgres", "err", err)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
}

// healthcheck pregunta a /healthz del propio proceso. Sale 0 si responde bien.
func healthcheck() int {
	addr := os.Getenv("CCP_CLOUD_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	if strings.HasPrefix(addr, ":") {
		addr = "127.0.0.1" + addr
	}
	resp, err := (&http.Client{Timeout: 3 * time.Second}).Get("http://" + addr + "/healthz")
	if err != nil {
		return 1
	}
	resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return 1
	}
	return 0
}
```

- [ ] **Step 3: Comprobar que pasa y que compila**

Run: `go test ./cmd/ccp-cloud/ && go build -o /dev/null ./cmd/ccp-cloud && go vet ./cmd/ccp-cloud/`
Expected: PASS.

- [ ] **Step 4: Gates y commit**

Mensaje propuesto: `feat(cloud): binario ccp-cloud con migraciones, readiness y apagado ordenado`

---

### Task 11: `internal/cloud/client` — sesión, API, push y pull

El lado de la máquina:
- **Archivos locales** en `<home>/cloud`, todos 0600:
  - `config.json`;
  - `token.json`, el refresh token, que es la credencial del dispositivo;
  - `vault.key`, la AK desbloqueada;
  - `state.json`, qué snapshots locales están ya arriba.
- **Login** por concesión de dispositivo.
- **Un cliente HTTP** que renueva el token y **guarda el refresh token nuevo**
  cada vez que Keycloak lo rota.
- **`Push`**: sube, del más viejo al más nuevo, los snapshots locales que faltan.
- **`Pull`**:
  1. verifica la firma con la clave de la cuenta, nunca con algo que diga el
     servidor;
  2. abre el manifiesto;
  3. baja los blobs que faltan y los comprueba contra su hash.

**Files:**
- Create: `internal/cloud/client/files.go`
- Create: `internal/cloud/client/auth.go`
- Create: `internal/cloud/client/api.go`
- Create: `internal/cloud/client/sync.go`
- Create: `internal/cloud/client/client_test.go`
- Modify: `go.mod`, `go.sum` (`golang.org/x/oauth2`)

**Interfaces:**
- Consumes: `crypt.*` (Task 2), `api.*` (Task 3), `snapshot.*`; en los tests,
  `server.New`, `store.NewMem`, `blobstest.New` y `oidctest.New`.
- Produces:
  - archivos locales: `client.Files` + `NewFiles(home)`, `Config` (con
    `TokenURL`, `DeviceURL`), `State`, y los errores `ErrNotLoggedIn` y
    `ErrLocked`;
  - autenticación: `Discover`, `OAuthConfig`, `Login`, `HTTPClient`,
    `Session(ctx, files) (Config, *API, error)`;
  - bóveda: `VaultToAPI`, `VaultFromAPI`, `Account(files)`;
  - `API`: métodos `WithDevice`, `Me`, `RegisterDevice`, `Devices`,
    `RevokeDevice`, `Vault`, `PutVault`, `Presign`, `CommitSnapshot`,
    `Snapshots`, `Snapshot`, `PutBlob`, `GetBlob`; errores `*APIError`;
  - sincronización: `PushReport`, `Push`, `Pull`.

- [x] **Step 1: Escribir los tests — `internal/cloud/client/client_test.go`**

```go
package client

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/JoseAFlores777/ccp/internal/cloud/api"
	"github.com/JoseAFlores777/ccp/internal/cloud/blobs/blobstest"
	"github.com/JoseAFlores777/ccp/internal/cloud/crypt"
	"github.com/JoseAFlores777/ccp/internal/cloud/oidctest"
	"github.com/JoseAFlores777/ccp/internal/cloud/server"
	"github.com/JoseAFlores777/ccp/internal/cloud/store"
	"github.com/JoseAFlores777/ccp/internal/snapshot"
)

type rig struct {
	iss *oidctest.Issuer
	url string
}

// newRig levanta un API de verdad (server.New) con persistencia y
// almacenamiento en memoria y un Keycloak falso. wrap permite interponer algo
// en la persistencia (p. ej. un servidor que altera lo que devuelve).
func newRig(t *testing.T, wrap func(store.Store) store.Store) *rig {
	t.Helper()
	iss := oidctest.New(t)
	var st store.Store = store.NewMem()
	if wrap != nil {
		st = wrap(st)
	}
	h := server.New(server.Config{
		Store: st, Blobs: blobstest.New(t), Verifier: server.NewOIDCVerifier(iss.URL, iss.JWKSURL(), oidctest.Audience),
		Issuer: iss.URL, ClientID: oidctest.ClientID,
	})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return &rig{iss: iss, url: srv.URL}
}

// machine es un equipo con la sesión iniciada: su directorio de nube, su API
// con dispositivo y su almacén de snapshots.
func (r *rig) machine(t *testing.T, name string) (Files, *API, *snapshot.Store) {
	t.Helper()
	ctx := context.Background()
	files := NewFiles(t.TempDir())
	ep, err := Discover(ctx, http.DefaultClient, r.url)
	if err != nil {
		t.Fatal(err)
	}
	oc := OAuthConfig(ep.Info.ClientID, ep.TokenURL, ep.DeviceAuthURL)
	tok, err := Login(ctx, oc, func(string, string) {})
	if err != nil {
		t.Fatal(err)
	}
	if err := files.SaveToken(tok); err != nil {
		t.Fatal(err)
	}
	a := NewAPI(r.url, HTTPClient(ctx, oc, files, tok), "")
	d, err := a.RegisterDevice(ctx, api.DeviceIn{Name: name, Platform: "test"})
	if err != nil {
		t.Fatal(err)
	}
	st, err := snapshot.Open(filepath.Join(t.TempDir(), "snapshots"))
	if err != nil {
		t.Fatal(err)
	}
	return files, a.WithDevice(d.ID), st
}

func src(lpath, data string, class snapshot.Class) snapshot.Source {
	return snapshot.Source{LPath: lpath, Class: class, Mode: 0o644, Read: func() ([]byte, error) { return []byte(data), nil }}
}

const phrase = "frase de la bóveda larga"

// seed crea la bóveda desde la máquina A y un snapshot local con un secreto.
func seed(t *testing.T, a *API, st *snapshot.Store) (*crypt.Account, string, *snapshot.Manifest) {
	t.Helper()
	ctx := context.Background()
	ak, code, w, err := crypt.NewVault([]byte(phrase))
	if err != nil {
		t.Fatal(err)
	}
	v, err := VaultToAPI(w)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.PutVault(ctx, v); err != nil {
		t.Fatal(err)
	}
	acct, _ := crypt.NewAccount(ak)
	m, err := snapshot.Capture(st, []snapshot.Source{
		src("ccp/ccp.yaml", "version: 2\n", snapshot.ClassAuthored),
		src("ccp/profiles/deep/api_key", "sk-1", snapshot.ClassSecret),
	}, snapshot.Meta{Created: time.Now(), Machine: "mac-a", Trigger: "manual"}, false)
	if err != nil {
		t.Fatal(err)
	}
	return acct, code, m
}

func TestPushPullAcrossMachines(t *testing.T) {
	ctx := context.Background()
	r := newRig(t, nil)
	filesA, apiA, stA := r.machine(t, "mac-a")
	acctA, code, m := seed(t, apiA, stA)

	rep, err := Push(ctx, apiA, acctA, stA, filesA, "")
	if err != nil || rep.Snapshots != 1 || rep.Uploaded != 2 {
		t.Fatalf("Push = %+v, %v", rep, err)
	}
	if rep, err := Push(ctx, apiA, acctA, stA, filesA, ""); err != nil || rep.Snapshots != 0 {
		t.Fatalf("segundo Push = %+v, %v", rep, err)
	}

	filesB, apiB, stB := r.machine(t, "mac-b")
	v, err := apiB.Vault(ctx)
	if err != nil {
		t.Fatal(err)
	}
	w, _ := VaultFromAPI(v)
	ak, err := crypt.UnlockPassphrase(w, []byte(phrase))
	if err != nil {
		t.Fatal(err)
	}
	if akR, err := crypt.UnlockRecovery(w, code); err != nil || !bytes.Equal(akR, ak) {
		t.Fatalf("el código de recuperación no abre la misma bóveda: %v", err)
	}
	acctB, _ := crypt.NewAccount(ak)
	list, err := apiB.Snapshots(ctx, "", 10)
	if err != nil || len(list) != 1 || list[0].DeviceName != "mac-a" {
		t.Fatalf("Snapshots = %+v, %v", list, err)
	}
	got, missing, err := Pull(ctx, apiB, acctB, stB, filesB, list[0].ID)
	if err != nil || got.ID != m.ID || len(missing) != 0 {
		t.Fatalf("Pull = %v, %v, %v", got, missing, err)
	}
	if data, err := stB.GetBlob(m.Items[1].Hash); err != nil || string(data) != "sk-1" {
		t.Fatalf("el secreto no llegó: %q %v", data, err)
	}
	// Lo bajado no se vuelve a subir.
	if rep, err := Push(ctx, apiB, acctB, stB, filesB, ""); err != nil || rep.Snapshots != 0 {
		t.Fatalf("Push en B tras Pull = %+v, %v", rep, err)
	}
}

// tamper es un servidor comprometido: altera el manifiesto que devuelve.
type tamper struct{ store.Store }

func (t *tamper) Snapshot(ctx context.Context, userID, id string) (store.Snapshot, error) {
	s, err := t.Store.Snapshot(ctx, userID, id)
	if err == nil && len(s.Manifest) > 0 {
		s.Manifest = bytes.Clone(s.Manifest)
		s.Manifest[len(s.Manifest)-1] ^= 1
	}
	return s, err
}

func TestPullDetectsTampering(t *testing.T) {
	ctx := context.Background()
	r := newRig(t, func(s store.Store) store.Store { return &tamper{Store: s} })
	filesA, apiA, stA := r.machine(t, "mac-a")
	acct, _, _ := seed(t, apiA, stA)
	if _, err := Push(ctx, apiA, acct, stA, filesA, ""); err != nil {
		t.Fatal(err)
	}
	list, _ := apiA.Snapshots(ctx, "", 1)
	_, _, err := Pull(ctx, apiA, acct, stA, filesA, list[0].ID)
	if !errors.Is(err, crypt.ErrSignature) {
		t.Fatalf("un manifiesto alterado: err = %v, quiero ErrSignature", err)
	}
}

// Keycloak rota el refresh token: el cliente guarda el nuevo, o la próxima vez
// no podría renovar.
func TestRefreshedTokenIsPersisted(t *testing.T) {
	ctx := context.Background()
	r := newRig(t, nil)
	files := NewFiles(t.TempDir())
	ep, _ := Discover(ctx, http.DefaultClient, r.url)
	oc := OAuthConfig(ep.Info.ClientID, ep.TokenURL, ep.DeviceAuthURL)
	tok, err := Login(ctx, oc, func(string, string) {})
	if err != nil {
		t.Fatal(err)
	}
	first := tok.RefreshToken
	tok.Expiry = time.Now().Add(-time.Minute) // obliga a renovar
	files.SaveToken(tok)
	a := NewAPI(r.url, HTTPClient(ctx, oc, files, tok), "")
	if _, err := a.Me(ctx); err != nil {
		t.Fatal(err)
	}
	saved, err := files.LoadToken()
	if err != nil || saved.RefreshToken == first {
		t.Fatalf("refresh token guardado = %q (antes %q), %v", saved.RefreshToken, first, err)
	}
}

func TestFilesPermissions(t *testing.T) {
	files := NewFiles(t.TempDir())
	if _, err := files.LoadConfig(); !errors.Is(err, ErrNotLoggedIn) {
		t.Fatalf("sin config: %v", err)
	}
	if _, err := files.LoadAK(); !errors.Is(err, ErrLocked) {
		t.Fatalf("sin vault.key: %v", err)
	}
	files.SaveAK(bytes.Repeat([]byte{1}, 32))
	files.SaveConfig(Config{Server: "https://x"})
	for _, name := range []string{"vault.key", "config.json"} {
		fi, err := os.Stat(filepath.Join(files.Dir, name))
		if err != nil || fi.Mode().Perm() != 0o600 {
			t.Fatalf("%s: %v %v", name, fi, err)
		}
	}
	if di, _ := os.Stat(files.Dir); di.Mode().Perm() != 0o700 {
		t.Fatalf("el directorio de la nube tiene permisos %o", di.Mode().Perm())
	}
}
```

(Añade `"os"` a los imports del test.)

- [x] **Step 2: Comprobar que falla**

Run: `go test ./internal/cloud/client/`
Expected: FAIL, no compila.

- [x] **Step 3: Dependencia**

Run: `go get golang.org/x/oauth2@latest`, con la comprobación de la directiva `go`.

- [x] **Step 4: `internal/cloud/client/files.go`**

```go
// Package client es el lado de la máquina de la nube de ccp: login por
// dispositivo contra Keycloak, el API /v1 y la sincronización de snapshots. Lo
// usa la CLI y no importa nada del servidor.
package client

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"golang.org/x/oauth2"

	"github.com/JoseAFlores777/ccp/internal/vault"
)

var (
	// ErrNotLoggedIn: este equipo no ha iniciado sesión en la nube.
	ErrNotLoggedIn = errors.New("este equipo no ha iniciado sesión en la nube")
	// ErrLocked: la bóveda no está desbloqueada en este equipo.
	ErrLocked = errors.New("la bóveda está bloqueada en este equipo")
)

// Files son los archivos de la nube de este equipo, en <home>/cloud (0700).
// Todos 0600:
//
//	config.json  servidor, emisor, cuenta y dispositivo
//	token.json   el refresh token: la credencial del dispositivo
//	vault.key    la clave de cuenta desbloqueada
//	state.json   qué snapshots locales están ya en la nube
type Files struct{ Dir string }

// NewFiles devuelve los archivos de la nube de un CCP_HOME.
func NewFiles(home string) Files { return Files{Dir: filepath.Join(home, "cloud")} }

// Config es la sesión de este equipo.
type Config struct {
	Server     string `json:"server"`
	Issuer     string `json:"issuer"`
	ClientID   string `json:"client_id"`
	TokenURL   string `json:"token_url"`
	DeviceURL  string `json:"device_url"`
	UserID     string `json:"user_id"`
	Email      string `json:"email"`
	DeviceID   string `json:"device_id"`
	DeviceName string `json:"device_name"`
}

// State recuerda qué hay arriba: id local -> id en la nube.
type State struct {
	Pushed map[string]string `json:"pushed"`
}

func (f Files) path(name string) string { return filepath.Join(f.Dir, name) }

func (f Files) writeRaw(name string, data []byte) error {
	if err := os.MkdirAll(f.Dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(f.Dir, ".tmp-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), f.path(name))
}

func (f Files) write(name string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return f.writeRaw(name, append(data, '\n'))
}

func (f Files) read(name string, v any) error {
	data, err := os.ReadFile(f.path(name))
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, v); err != nil {
		return fmt.Errorf("%s está dañado: %w", f.path(name), err)
	}
	return nil
}

// LoadConfig lee la sesión, o ErrNotLoggedIn.
func (f Files) LoadConfig() (Config, error) {
	var c Config
	if err := f.read("config.json", &c); errors.Is(err, fs.ErrNotExist) {
		return c, ErrNotLoggedIn
	} else if err != nil {
		return c, err
	}
	return c, nil
}

// SaveConfig guarda la sesión.
func (f Files) SaveConfig(c Config) error { return f.write("config.json", c) }

// LoadToken lee el token, o ErrNotLoggedIn.
func (f Files) LoadToken() (*oauth2.Token, error) {
	var t oauth2.Token
	if err := f.read("token.json", &t); errors.Is(err, fs.ErrNotExist) {
		return nil, ErrNotLoggedIn
	} else if err != nil {
		return nil, err
	}
	return &t, nil
}

// SaveToken guarda el token.
func (f Files) SaveToken(t *oauth2.Token) error { return f.write("token.json", t) }

// LoadAK lee la clave de cuenta, o ErrLocked.
func (f Files) LoadAK() ([]byte, error) {
	data, err := os.ReadFile(f.path("vault.key"))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, ErrLocked
	}
	if err != nil {
		return nil, err
	}
	if len(data) != vault.KeySize {
		return nil, fmt.Errorf("%s está dañado", f.path("vault.key"))
	}
	return data, nil
}

// SaveAK guarda la clave de cuenta desbloqueada.
func (f Files) SaveAK(ak []byte) error { return f.writeRaw("vault.key", ak) }

// LoadState lee qué está arriba (vacío si nunca se subió nada).
func (f Files) LoadState() (State, error) {
	s := State{Pushed: map[string]string{}}
	if err := f.read("state.json", &s); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return s, err
	}
	if s.Pushed == nil {
		s.Pushed = map[string]string{}
	}
	return s, nil
}

// SaveState guarda qué está arriba.
func (f Files) SaveState(s State) error { return f.write("state.json", s) }

// Forget borra la credencial y la bóveda de este equipo, y olvida su dispositivo.
func (f Files) Forget() error {
	for _, name := range []string{"token.json", "vault.key"} {
		if err := os.Remove(f.path(name)); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	c, err := f.LoadConfig()
	if err != nil {
		return nil
	}
	c.DeviceID, c.DeviceName = "", ""
	return f.SaveConfig(c)
}
```

- [x] **Step 5: `internal/cloud/client/auth.go`**

```go
package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"

	"golang.org/x/oauth2"

	"github.com/JoseAFlores777/ccp/internal/cloud/api"
	"github.com/JoseAFlores777/ccp/internal/cloud/crypt"
	"github.com/JoseAFlores777/ccp/internal/vault"
)

// Endpoints es lo que hace falta para autenticarse contra un servidor.
type Endpoints struct {
	Info          api.Info
	TokenURL      string
	DeviceAuthURL string
}

func getJSON(ctx context.Context, hc *http.Client, url string, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s respondió %d", url, resp.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(v)
}

// Discover lee /v1/info del servidor y el discovery de su emisor.
func Discover(ctx context.Context, hc *http.Client, server string) (Endpoints, error) {
	var e Endpoints
	if err := getJSON(ctx, hc, strings.TrimSuffix(server, "/")+"/v1/info", &e.Info); err != nil {
		return e, fmt.Errorf("%s no responde como una nube de ccp: %w", server, err)
	}
	if e.Info.APIVersion != api.Version {
		return e, fmt.Errorf("el servidor habla la versión %d del protocolo y este ccp la %d; actualiza ccp", e.Info.APIVersion, api.Version)
	}
	var d struct {
		Token  string `json:"token_endpoint"`
		Device string `json:"device_authorization_endpoint"`
	}
	if err := getJSON(ctx, hc, e.Info.Issuer+"/.well-known/openid-configuration", &d); err != nil {
		return e, fmt.Errorf("no se pudo leer el emisor %s: %w", e.Info.Issuer, err)
	}
	if d.Token == "" || d.Device == "" {
		return e, errors.New("el emisor no ofrece la concesión de dispositivo")
	}
	e.TokenURL, e.DeviceAuthURL = d.Token, d.Device
	return e, nil
}

// OAuthConfig es la configuración OAuth del cliente público de la CLI. Pide
// offline_access: la sesión offline es la credencial de larga vida del equipo.
func OAuthConfig(clientID, tokenURL, deviceURL string) *oauth2.Config {
	return &oauth2.Config{
		ClientID: clientID,
		Scopes:   []string{"openid", "offline_access"},
		Endpoint: oauth2.Endpoint{TokenURL: tokenURL, DeviceAuthURL: deviceURL, AuthStyle: oauth2.AuthStyleInParams},
	}
}

// Login hace la concesión de dispositivo: show recibe la URL y el código para
// enseñárselos a la persona, y Login espera a que lo apruebe.
func Login(ctx context.Context, cfg *oauth2.Config, show func(url, code string)) (*oauth2.Token, error) {
	da, err := cfg.DeviceAuth(ctx)
	if err != nil {
		return nil, fmt.Errorf("no se pudo iniciar el login: %w", err)
	}
	uri := da.VerificationURIComplete
	if uri == "" {
		uri = da.VerificationURI
	}
	show(uri, da.UserCode)
	tok, err := cfg.DeviceAccessToken(ctx, da)
	if err != nil {
		return nil, fmt.Errorf("el login no se completó: %w", err)
	}
	if tok.RefreshToken == "" {
		return nil, errors.New("el emisor no devolvió un refresh token (¿el cliente tiene offline_access?)")
	}
	return tok, nil
}

// persisting guarda el token cada vez que el refresh token cambia: Keycloak lo
// rota, y quedarse con el viejo dejaría al equipo sin poder renovar.
type persisting struct {
	mu    sync.Mutex
	src   oauth2.TokenSource
	files Files
	last  string
}

func (p *persisting) Token() (*oauth2.Token, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	t, err := p.src.Token()
	if err != nil {
		return nil, err
	}
	if t.RefreshToken != "" && t.RefreshToken != p.last {
		if err := p.files.SaveToken(t); err != nil {
			return nil, err
		}
		p.last = t.RefreshToken
	}
	return t, nil
}

// HTTPClient es un cliente HTTP que añade el token y lo renueva al caducar.
func HTTPClient(ctx context.Context, cfg *oauth2.Config, files Files, tok *oauth2.Token) *http.Client {
	return oauth2.NewClient(ctx, &persisting{src: cfg.TokenSource(ctx, tok), files: files, last: tok.RefreshToken})
}

// Session abre la sesión guardada de este equipo.
func Session(ctx context.Context, files Files) (Config, *API, error) {
	c, err := files.LoadConfig()
	if err != nil {
		return c, nil, err
	}
	tok, err := files.LoadToken()
	if err != nil {
		return c, nil, err
	}
	if c.DeviceID == "" {
		return c, nil, ErrNotLoggedIn
	}
	oc := OAuthConfig(c.ClientID, c.TokenURL, c.DeviceURL)
	return c, NewAPI(c.Server, HTTPClient(ctx, oc, files, tok), c.DeviceID), nil
}

// Account abre la cuenta con la AK desbloqueada de este equipo.
func Account(files Files) (*crypt.Account, error) {
	ak, err := files.LoadAK()
	if err != nil {
		return nil, err
	}
	return crypt.NewAccount(ak)
}

// VaultToAPI convierte las envolturas en lo que se sube.
func VaultToAPI(w crypt.Wraps) (api.Vault, error) {
	kdf, err := json.Marshal(w.KDF)
	if err != nil {
		return api.Vault{}, err
	}
	return api.Vault{KDF: kdf, PassphraseWrap: w.Passphrase, RecoveryWrap: w.Recovery, SignPub: w.SignPub}, nil
}

// VaultFromAPI convierte lo bajado en envolturas.
func VaultFromAPI(v api.Vault) (crypt.Wraps, error) {
	var kdf vault.KDFParams
	if err := json.Unmarshal(v.KDF, &kdf); err != nil {
		return crypt.Wraps{}, fmt.Errorf("parámetros de la bóveda dañados: %w", err)
	}
	return crypt.Wraps{KDF: kdf, Passphrase: v.PassphraseWrap, Recovery: v.RecoveryWrap, SignPub: v.SignPub}, nil
}
```

- [x] **Step 6: `internal/cloud/client/api.go`**

```go
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/JoseAFlores777/ccp/internal/cloud/api"
)

// API es el cliente de /v1.
type API struct {
	base   string
	hc     *http.Client // con token
	raw    *http.Client // para las URLs prefirmadas: sin token
	device string
}

// NewAPI construye el cliente. hc debe añadir el token (HTTPClient).
func NewAPI(server string, hc *http.Client, device string) *API {
	return &API{base: strings.TrimSuffix(server, "/"), hc: hc, raw: &http.Client{Timeout: 5 * time.Minute}, device: device}
}

// WithDevice devuelve una copia que se identifica como el dispositivo id.
func (a *API) WithDevice(id string) *API {
	c := *a
	c.device = id
	return &c
}

// APIError es una respuesta de error del servidor.
type APIError struct {
	Status int
	api.Error
}

func (e *APIError) Error() string { return fmt.Sprintf("la nube respondió %d: %s", e.Status, e.Message) }

func (a *API) do(ctx context.Context, method, path string, in, out any) error {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, a.base+path, body)
	if err != nil {
		return err
	}
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if a.device != "" {
		req.Header.Set(api.HeaderDevice, a.device)
	}
	resp, err := a.hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		e := &APIError{Status: resp.StatusCode}
		_ = json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&e.Error)
		if e.Message == "" {
			e.Message = resp.Status
		}
		return e
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 64<<20)).Decode(out)
}

func (a *API) Me(ctx context.Context) (api.Me, error) {
	var m api.Me
	return m, a.do(ctx, http.MethodGet, "/v1/me", nil, &m)
}

func (a *API) RegisterDevice(ctx context.Context, in api.DeviceIn) (api.Device, error) {
	var d api.Device
	return d, a.do(ctx, http.MethodPost, "/v1/devices", in, &d)
}

func (a *API) Devices(ctx context.Context) ([]api.Device, error) {
	var ds []api.Device
	return ds, a.do(ctx, http.MethodGet, "/v1/devices", nil, &ds)
}

func (a *API) RevokeDevice(ctx context.Context, id string) error {
	return a.do(ctx, http.MethodDelete, "/v1/devices/"+url.PathEscape(id), nil, nil)
}

func (a *API) Vault(ctx context.Context) (api.Vault, error) {
	var v api.Vault
	return v, a.do(ctx, http.MethodGet, "/v1/vault", nil, &v)
}

func (a *API) PutVault(ctx context.Context, v api.Vault) error {
	return a.do(ctx, http.MethodPut, "/v1/vault", v, nil)
}

func (a *API) Presign(ctx context.Context, op string, ids []string) ([]api.PresignItem, error) {
	var out []api.PresignItem
	return out, a.do(ctx, http.MethodPost, "/v1/blobs/presign", api.PresignReq{Op: op, IDs: ids}, &out)
}

func (a *API) CommitSnapshot(ctx context.Context, in api.SnapshotIn) (api.SnapshotMeta, error) {
	var m api.SnapshotMeta
	return m, a.do(ctx, http.MethodPost, "/v1/snapshots", in, &m)
}

func (a *API) Snapshots(ctx context.Context, device string, limit int) ([]api.SnapshotMeta, error) {
	q := url.Values{"limit": {strconv.Itoa(limit)}}
	if device != "" {
		q.Set("device", device)
	}
	var out []api.SnapshotMeta
	return out, a.do(ctx, http.MethodGet, "/v1/snapshots?"+q.Encode(), nil, &out)
}

func (a *API) Snapshot(ctx context.Context, id string) (api.Snapshot, error) {
	var s api.Snapshot
	return s, a.do(ctx, http.MethodGet, "/v1/snapshots/"+url.PathEscape(id), nil, &s)
}

// PutBlob sube body a una URL prefirmada, con reintentos ante fallos de red o 5xx.
func (a *API) PutBlob(ctx context.Context, u string, body []byte) error {
	return retry(ctx, func() (bool, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPut, u, bytes.NewReader(body))
		if err != nil {
			return false, err
		}
		req.ContentLength = int64(len(body))
		resp, err := a.raw.Do(req)
		if err != nil {
			return true, err
		}
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
		resp.Body.Close()
		if resp.StatusCode/100 == 2 {
			return false, nil
		}
		return resp.StatusCode >= 500, fmt.Errorf("el almacenamiento respondió %d al subir", resp.StatusCode)
	})
}

// GetBlob baja de una URL prefirmada, con reintentos.
func (a *API) GetBlob(ctx context.Context, u string) ([]byte, error) {
	var out []byte
	err := retry(ctx, func() (bool, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return false, err
		}
		resp, err := a.raw.Do(req)
		if err != nil {
			return true, err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return resp.StatusCode >= 500, fmt.Errorf("el almacenamiento respondió %d al bajar", resp.StatusCode)
		}
		data, err := io.ReadAll(io.LimitReader(resp.Body, api.MaxBlobBytes+1))
		if err != nil {
			return true, err
		}
		if len(data) > api.MaxBlobBytes {
			return false, fmt.Errorf("el almacenamiento devolvió un blob de más de %d bytes", api.MaxBlobBytes)
		}
		out = data
		return false, nil
	})
	return out, err
}

// retry reintenta fn hasta 4 veces, con espera exponencial desde 500 ms,
// mientras diga que el fallo es reintentable.
func retry(ctx context.Context, fn func() (retryable bool, err error)) error {
	var err error
	for attempt := 0; attempt < 4; attempt++ {
		var again bool
		if again, err = fn(); err == nil || !again {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(1<<attempt) * 500 * time.Millisecond):
		}
	}
	return err
}
```

- [x] **Step 7: `internal/cloud/client/sync.go`**

```go
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

// PushReport resume una subida.
type PushReport struct {
	Snapshots int
	Uploaded  int
	Bytes     int64
	// Missing: rutas cuyo contenido no está en este equipo (snapshots
	// importados sin secretos). TooLarge: rutas que superan MaxBlobBytes
	// sellado. Ninguna de las dos se sube; el snapshot sí.
	Missing  []string
	TooLarge []string
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Push sube, del más viejo al más nuevo, los snapshots locales que la nube aún
// no tiene. only (un id local completo), si no está vacío, limita a ese.
func Push(ctx context.Context, a *API, acct *crypt.Account, st *snapshot.Store, files Files, only string) (PushReport, error) {
	var rep PushReport
	state, err := files.LoadState()
	if err != nil {
		return rep, err
	}
	ms, err := st.List()
	if err != nil {
		return rep, err
	}
	for i := len(ms) - 1; i >= 0; i-- {
		m := ms[i]
		if only != "" && m.ID != only {
			continue
		}
		if _, done := state.Pushed[m.ID]; done {
			continue
		}
		cloudID, err := pushOne(ctx, a, acct, st, m, &rep)
		if err != nil {
			return rep, fmt.Errorf("snapshot %s: %w", snapshot.Short(m.ID), err)
		}
		state.Pushed[m.ID] = cloudID
		if err := files.SaveState(state); err != nil {
			return rep, err
		}
		rep.Snapshots++
	}
	return rep, nil
}

func pushOne(ctx context.Context, a *API, acct *crypt.Account, st *snapshot.Store, m *snapshot.Manifest, rep *PushReport) (string, error) {
	local := map[string]string{} // id en la nube -> hash local
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
	upload := func(ids []string) error {
		for start := 0; start < len(ids); start += api.MaxPresignIDs {
			items, err := a.Presign(ctx, "put", ids[start:min(start+api.MaxPresignIDs, len(ids))])
			if err != nil {
				return err
			}
			for _, pi := range items {
				hash, ok := local[pi.ID]
				if pi.Exists || !ok {
					continue
				}
				data, err := st.GetBlob(hash)
				if err != nil {
					return err
				}
				sealed, err := acct.SealBlob(pi.ID, data)
				if err != nil {
					return err
				}
				if len(sealed) > api.MaxBlobBytes {
					rep.TooLarge = append(rep.TooLarge, lpathsOf(hash)...)
					delete(local, pi.ID)
					continue
				}
				if err := a.PutBlob(ctx, pi.URL, sealed); err != nil {
					return err
				}
				rep.Uploaded++
				rep.Bytes += int64(len(sealed))
			}
		}
		return nil
	}
	if err := upload(sortedKeys(local)); err != nil {
		return "", err
	}
	js, err := json.Marshal(m)
	if err != nil {
		return "", err
	}
	cloudID, parent := acct.SnapshotID(m.ID), acct.SnapshotID(m.Parent)
	sealed, err := acct.SealManifest(cloudID, js)
	if err != nil {
		return "", err
	}
	in := api.SnapshotIn{ID: cloudID, Parent: parent, Created: m.Created, Manifest: sealed,
		Sig: acct.Sign(cloudID, parent, sealed), Blobs: sortedKeys(local)}
	_, err = a.CommitSnapshot(ctx, in)
	var apiErr *APIError
	if errors.As(err, &apiErr) && apiErr.Code == api.CodeMissingBlobs {
		// El servidor no encuentra blobs que se subieron (una subida que falló
		// sin avisar): se suben otra vez y se reintenta una sola vez.
		if err := upload(apiErr.Missing); err != nil {
			return "", err
		}
		_, err = a.CommitSnapshot(ctx, in)
	}
	return cloudID, err
}

// Pull baja el snapshot cloudID de la nube al almacén local. Devuelve el
// manifiesto y las rutas que no tenían datos en la nube.
func Pull(ctx context.Context, a *API, acct *crypt.Account, st *snapshot.Store, files Files, cloudID string) (*snapshot.Manifest, []string, error) {
	sn, err := a.Snapshot(ctx, cloudID)
	if err != nil {
		return nil, nil, err
	}
	// La clave pública sale de la AK: un servidor comprometido no puede colar
	// un snapshot ni cambiar su padre.
	if err := acct.Verify(sn.ID, sn.Parent, sn.Manifest, sn.Sig); err != nil {
		return nil, nil, err
	}
	js, err := acct.OpenManifest(sn.ID, sn.Manifest)
	if err != nil {
		return nil, nil, fmt.Errorf("no se pudo abrir el manifiesto: %w", err)
	}
	var m snapshot.Manifest
	if err := json.Unmarshal(js, &m); err != nil {
		return nil, nil, fmt.Errorf("manifiesto ilegible: %w", err)
	}
	if acct.SnapshotID(m.ID) != sn.ID {
		return nil, nil, errors.New("el manifiesto no corresponde a su id en la nube")
	}
	need := map[string][]snapshot.Item{} // id en la nube -> elementos que lo usan
	for _, it := range m.Items {
		if !st.HasBlob(it.Hash) {
			bid := acct.BlobID(it.Hash)
			need[bid] = append(need[bid], it)
		}
	}
	var missing []string
	ids := sortedKeys(need)
	for start := 0; start < len(ids); start += api.MaxPresignIDs {
		items, err := a.Presign(ctx, "get", ids[start:min(start+api.MaxPresignIDs, len(ids))])
		if err != nil {
			return nil, nil, err
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
				return nil, nil, err
			}
			data, err := acct.OpenBlob(pi.ID, body)
			if err != nil || snapshot.Hash(data) != its[0].Hash {
				return nil, nil, fmt.Errorf("el blob de %s llegó alterado", its[0].LPath)
			}
			secret := false
			for _, it := range its {
				secret = secret || it.Class == snapshot.ClassSecret
			}
			if _, err := st.PutBlob(data, secret); err != nil {
				return nil, nil, err
			}
		}
	}
	if err := st.SaveManifest(&m); err != nil {
		return nil, nil, err
	}
	state, err := files.LoadState()
	if err != nil {
		return nil, nil, err
	}
	state.Pushed[m.ID] = sn.ID // ya está arriba: no se vuelve a subir
	if err := files.SaveState(state); err != nil {
		return nil, nil, err
	}
	sort.Strings(missing)
	return &m, missing, nil
}
```

- [x] **Step 8: Comprobar que pasa**

Run: `go test ./internal/cloud/... && go vet ./internal/cloud/...`
Expected: PASS. Cada login del test tarda en torno a un segundo por el intervalo
de sondeo de la concesión de dispositivo.

- [x] **Step 9: Gates y commit**

Mensaje propuesto: `feat(cloud): cliente con login por dispositivo, push y pull verificados`

---

### Task 12: CLI — `ccp cloud …`

```
ccp cloud login <servidor> [--name <equipo>]   inicia sesión (concesión de dispositivo) y registra este equipo
ccp cloud logout                               revoca este equipo y borra su token y su bóveda local
ccp cloud status [--json]                      servidor, cuenta, equipo, bóveda, pendientes de subir
ccp cloud init                                 crea la bóveda (primera máquina) y enseña el código de recuperación
ccp cloud unlock [--recovery]                  desbloquea la bóveda en este equipo
ccp cloud push [<snapshot>] [--json]           sube los snapshots locales que faltan
ccp cloud pull [<id>|latest] [--device <n>]    baja un snapshot de la nube al almacén local
ccp cloud list [--json]                        snapshots en la nube, de todos los equipos
ccp cloud devices [--json]                     equipos de la cuenta
ccp cloud revoke <dispositivo>                 revoca otro equipo
```

**Files:**
- Create: `internal/cli/cloud.go`
- Create: `internal/cli/cloud_test.go`
- Create: `internal/core/i18n/catalog_cloud.go`
- Modify: `internal/cli/cli.go` (`case "cloud"`)
- Modify: `internal/cli/snapshot.go` (`"cloud": true` en `dailySnapshotCmds`)
- Modify: `internal/core/i18n/catalog_cli.go` (sección NUBE de la ayuda, En y Es)

**Interfaces:**
- Consumes: `client.*`, `crypt.*`; de `cli`: `parseSnapArgs`, `promptSecret`,
  `snapMinPassphrase`, `snapMachine`, `snapJSON`, `okLine`, `warnLine`, `mute`,
  `boldLine`, `humanBytes`, `snapEnv` y `snapRun` (tests).
- Produces: `dispatchCloud`, `cloudCmd`, `readCloudSecret`, `openBrowser`.

- [ ] **Step 1: Escribir el test de punta a punta — `internal/cli/cloud_test.go`**

```go
package cli

import (
	"encoding/json"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/JoseAFlores777/ccp/internal/cloud/blobs/blobstest"
	"github.com/JoseAFlores777/ccp/internal/cloud/oidctest"
	"github.com/JoseAFlores777/ccp/internal/cloud/server"
	"github.com/JoseAFlores777/ccp/internal/cloud/store"
)

// cloudServer levanta el API real con persistencia y almacenamiento en memoria
// y un Keycloak falso. Devuelve su URL (http://127.0.0.1:…, que la CLI acepta
// sin https).
func cloudServer(t *testing.T) string {
	t.Helper()
	iss := oidctest.New(t)
	srv := httptest.NewServer(server.New(server.Config{
		Store: store.NewMem(), Blobs: blobstest.New(t),
		Verifier: server.NewOIDCVerifier(iss.URL, iss.JWKSURL(), oidctest.Audience),
		Issuer:   iss.URL, ClientID: oidctest.ClientID,
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

var recoveryRe = regexp.MustCompile(`[A-Z2-7]{4}(-[A-Z2-7]{4}){7}`)

func TestCloudEndToEnd(t *testing.T) {
	url := cloudServer(t)
	t.Setenv("CCP_NO_BROWSER", "1")
	t.Setenv("CCP_CLOUD_PASSPHRASE", "frase de la bóveda larga")

	// Máquina A.
	snapEnv(t)
	code, out, errs := snapRun(t, "cloud", "login", url, "--name", "mac-a")
	if code != 0 || !strings.Contains(out, "ccp cloud init") {
		t.Fatalf("login A: %d %q %q", code, out, errs)
	}
	code, out, errs = snapRun(t, "cloud", "init")
	if code != 0 || !recoveryRe.MatchString(out) {
		t.Fatalf("init: %d %q %q", code, out, errs)
	}
	if code, out, errs = snapRun(t, "snapshot", "create"); code != 0 {
		t.Fatalf("snapshot create: %d %q %q", code, out, errs)
	}
	if code, out, errs = snapRun(t, "cloud", "push"); code != 0 || !strings.Contains(out, "Subidos 1") {
		t.Fatalf("push: %d %q %q", code, out, errs)
	}

	// Máquina B: otro CCP_HOME, la misma cuenta.
	snapEnv(t)
	if code, out, errs = snapRun(t, "cloud", "login", url, "--name", "mac-b"); code != 0 || !strings.Contains(out, "ccp cloud unlock") {
		t.Fatalf("login B: %d %q %q", code, out, errs)
	}
	if code, out, errs = snapRun(t, "cloud", "unlock"); code != 0 {
		t.Fatalf("unlock: %d %q %q", code, out, errs)
	}
	if code, out, errs = snapRun(t, "cloud", "pull"); code != 0 || !strings.Contains(out, "ccp snapshot restore") {
		t.Fatalf("pull: %d %q %q", code, out, errs)
	}
	_, out, _ = snapRun(t, "snapshot", "list", "--json")
	var list []snapSummary
	if json.Unmarshal([]byte(out), &list) != nil || len(list) != 1 {
		t.Fatalf("tras pull, snapshot list = %q", out)
	}
	_, out, _ = snapRun(t, "cloud", "status", "--json")
	var st struct {
		LoggedIn    bool   `json:"logged_in"`
		Vault       string `json:"vault"`
		PendingPush int    `json:"pending_push"`
	}
	if json.Unmarshal([]byte(out), &st) != nil || !st.LoggedIn || st.Vault != "unlocked" || st.PendingPush != 0 {
		t.Fatalf("status --json = %q", out)
	}
	if code, out, _ = snapRun(t, "cloud", "devices"); code != 0 || !strings.Contains(out, "mac-a") || !strings.Contains(out, "mac-b") {
		t.Fatalf("devices: %d %q", code, out)
	}
	if code, out, _ = snapRun(t, "cloud", "logout"); code != 0 {
		t.Fatalf("logout: %d %q", code, out)
	}
	if code, _, errs = snapRun(t, "cloud", "push"); code != 1 || !strings.Contains(errs, "ccp cloud login") {
		t.Fatalf("push tras logout: %d %q", code, errs)
	}
}

func TestCloudUnlockWithRecoveryCode(t *testing.T) {
	url := cloudServer(t)
	t.Setenv("CCP_NO_BROWSER", "1")
	t.Setenv("CCP_CLOUD_PASSPHRASE", "frase de la bóveda larga")
	snapEnv(t)
	snapRun(t, "cloud", "login", url)
	_, out, _ := snapRun(t, "cloud", "init")
	code := recoveryRe.FindString(out)

	snapEnv(t)
	snapRun(t, "cloud", "login", url)
	t.Setenv("CCP_CLOUD_PASSPHRASE", "otra frase que no es")
	if c, _, errs := snapRun(t, "cloud", "unlock"); c != 1 || !strings.Contains(errs, "no abre") {
		t.Fatalf("frase equivocada: %d %q", c, errs)
	}
	t.Setenv("CCP_CLOUD_RECOVERY", code)
	if c, out, errs := snapRun(t, "cloud", "unlock", "--recovery"); c != 0 {
		t.Fatalf("unlock --recovery: %d %q %q", c, out, errs)
	}
}

func TestCloudRequiresHTTPS(t *testing.T) {
	snapEnv(t)
	if code, _, errs := snapRun(t, "cloud", "login", "http://ccp.example.com"); code != 1 || !strings.Contains(errs, "https") {
		t.Fatalf("login por http: %d %q", code, errs)
	}
}
```

- [ ] **Step 2: Comprobar que falla**

Run: `go test ./internal/cli/ -run Cloud`
Expected: FAIL (comando desconocido).

- [ ] **Step 3: El catálogo — `internal/core/i18n/catalog_cloud.go`**

```go
package i18n

// catalog_cloud.go — prosa de `ccp cloud` (internal/cli/cloud.go). Prefijo
// `cli.cloud.` y nada más.

func init() { register(catalogCloud) }

var catalogCloud = map[string]map[Lang]string{
	"cli.cloud.usage": {
		En: `Usage: ccp cloud <subcommand>

  login <server> [--name <machine>]   sign in (device code) and register this machine
  logout                              revoke this machine and delete its token and local vault
  status [--json]                     server, account, machine, vault, pending uploads
  init                                create the vault (first machine) and show the recovery code
  unlock [--recovery]                 unlock the vault on this machine
  push [<snapshot>] [--json]          upload the local snapshots the cloud does not have
  pull [<id>|latest] [--device <n>]   download a snapshot into the local store
  list [--json]                       snapshots in the cloud, from every machine
  devices [--json]                    machines of the account
  revoke <device>                     revoke another machine

Everything is encrypted on this machine before it leaves: the server cannot read it.
CCP_CLOUD_PASSPHRASE and CCP_CLOUD_RECOVERY give the secrets without asking.`,
		Es: `Uso: ccp cloud <subcomando>

  login <servidor> [--name <equipo>]  inicia sesión (código de dispositivo) y registra este equipo
  logout                              revoca este equipo y borra su token y su bóveda local
  status [--json]                     servidor, cuenta, equipo, bóveda, pendientes de subir
  init                                crea la bóveda (primera máquina) y enseña el código de recuperación
  unlock [--recovery]                 desbloquea la bóveda en este equipo
  push [<snapshot>] [--json]          sube los snapshots locales que la nube no tiene
  pull [<id>|latest] [--device <n>]   baja un snapshot al almacén local
  list [--json]                       snapshots en la nube, de todos los equipos
  devices [--json]                    equipos de la cuenta
  revoke <dispositivo>                revoca otro equipo

Todo se cifra en este equipo antes de salir: el servidor no puede leerlo.
CCP_CLOUD_PASSPHRASE y CCP_CLOUD_RECOVERY dan los secretos sin preguntarlos.`,
	},
	"cli.cloud.unknown_sub": {En: "cloud: unknown subcommand '%s'", Es: "cloud: subcomando desconocido '%s'"},
	"cli.cloud.unknown_opt": {En: "cloud: unknown option or extra argument '%s'", Es: "cloud: opción desconocida o argumento de más '%s'"},
	"cli.cloud.need_server": {En: "cloud: which server? ccp cloud login https://…", Es: "cloud: ¿qué servidor? ccp cloud login https://…"},
	"cli.cloud.need_https": {
		En: "cloud: the server must use https (plain http is only accepted for localhost)",
		Es: "cloud: el servidor tiene que usar https (http solo se acepta para localhost)",
	},
	"cli.cloud.login_open": {
		En: "Open %s in your browser and confirm the code %s. Waiting…",
		Es: "Abre %s en el navegador y confirma el código %s. Esperando…",
	},
	"cli.cloud.login_ok":    {En: "Signed in as %s; this machine is «%s».", Es: "Sesión iniciada como %s; este equipo es «%s»."},
	"cli.cloud.next_init":   {En: "Next, on your first machine: ccp cloud init", Es: "Siguiente paso, en tu primera máquina: ccp cloud init"},
	"cli.cloud.next_unlock": {En: "This account already has a vault. Unlock it here: ccp cloud unlock", Es: "Esta cuenta ya tiene bóveda. Desbloquéala aquí: ccp cloud unlock"},
	"cli.cloud.logged_out": {
		En: "Signed out: this machine was revoked and its token and local vault deleted.",
		Es: "Sesión cerrada: este equipo quedó revocado y se borraron su token y su bóveda local.",
	},
	"cli.cloud.not_logged_in": {
		En: "This machine is not signed in to the cloud: ccp cloud login <server>",
		Es: "Este equipo no ha iniciado sesión en la nube: ccp cloud login <servidor>",
	},
	"cli.cloud.locked": {En: "The vault is locked on this machine: ccp cloud unlock", Es: "La bóveda está bloqueada en este equipo: ccp cloud unlock"},

	"cli.cloud.status_server":  {En: "Server:   %s", Es: "Servidor:   %s"},
	"cli.cloud.status_account": {En: "Account:  %s", Es: "Cuenta:     %s"},
	"cli.cloud.status_device":  {En: "Machine:  %s", Es: "Equipo:     %s"},
	"cli.cloud.status_vault":   {En: "Vault:    %s", Es: "Bóveda:     %s"},
	"cli.cloud.status_pending": {En: "Pending:  %d snapshots to upload", Es: "Pendientes: %d snapshots por subir"},
	"cli.cloud.vault_unlocked": {En: "unlocked on this machine", Es: "desbloqueada en este equipo"},
	"cli.cloud.vault_locked":   {En: "locked (ccp cloud unlock)", Es: "bloqueada (ccp cloud unlock)"},
	"cli.cloud.vault_missing":  {En: "not created (ccp cloud init)", Es: "sin crear (ccp cloud init)"},
	"cli.cloud.vault_unknown":  {En: "unknown (no connection)", Es: "desconocido (sin conexión)"},

	"cli.cloud.vault_exists": {
		En: "This account already has a vault. Unlock it with: ccp cloud unlock",
		Es: "Esta cuenta ya tiene bóveda. Desbloquéala con: ccp cloud unlock",
	},
	"cli.cloud.no_vault": {
		En: "This account has no vault yet. Create it on your first machine with: ccp cloud init",
		Es: "Esta cuenta aún no tiene bóveda. Créala en tu primera máquina con: ccp cloud init",
	},
	"cli.cloud.vault_created":  {En: "Vault created and unlocked on this machine.", Es: "Bóveda creada y desbloqueada en este equipo."},
	"cli.cloud.recovery_title": {En: "RECOVERY CODE — shown only this once:", Es: "CÓDIGO DE RECUPERACIÓN — se enseña solo esta vez:"},
	"cli.cloud.recovery_hint": {
		En: "Keep it off this machine (a password manager, paper). It opens the vault if you forget the passphrase; without the passphrase or this code, your cloud data cannot be recovered.",
		Es: "Guárdalo fuera de este equipo (un gestor de contraseñas, papel). Abre la bóveda si olvidas la frase; sin la frase ni este código, tus datos en la nube no se pueden recuperar.",
	},
	"cli.cloud.pass_prompt":     {En: "Vault passphrase: ", Es: "Frase de la bóveda: "},
	"cli.cloud.pass_confirm":    {En: "Repeat it: ", Es: "Repítela: "},
	"cli.cloud.pass_mismatch":   {En: "the passphrases do not match", Es: "las frases no coinciden"},
	"cli.cloud.pass_short":      {En: "the passphrase needs at least %d characters", Es: "la frase necesita al menos %d caracteres"},
	"cli.cloud.pass_needed":     {En: "a secret is needed: set %s or run it in a terminal", Es: "hace falta un secreto: define %s o ejecútalo en una terminal"},
	"cli.cloud.recovery_prompt": {En: "Recovery code: ", Es: "Código de recuperación: "},
	"cli.cloud.unlocked":        {En: "Vault unlocked on this machine.", Es: "Bóveda desbloqueada en este equipo."},
	"cli.cloud.unlock_wrong":    {En: "That passphrase (or code) does not open the vault.", Es: "Esa frase (o código) no abre la bóveda."},

	"cli.cloud.pushed":         {En: "Uploaded %d snapshots (%d blobs, %s).", Es: "Subidos %d snapshots (%d blobs, %s)."},
	"cli.cloud.push_nothing":   {En: "Nothing to upload: the cloud has every snapshot of this machine.", Es: "Nada que subir: la nube tiene todos los snapshots de este equipo."},
	"cli.cloud.push_missing":   {En: "%d items were not uploaded because their data is not on this machine.", Es: "%d elementos no se subieron porque sus datos no están en este equipo."},
	"cli.cloud.push_too_large": {En: "Not uploaded (over 64 MiB): %s", Es: "No se subieron (más de 64 MiB): %s"},
	"cli.cloud.pulled":         {En: "Snapshot %s downloaded (here it is %s).", Es: "Snapshot %s bajado (en este equipo es %s)."},
	"cli.cloud.pull_hint":      {En: "To apply it: ccp snapshot restore %s", Es: "Para aplicarlo: ccp snapshot restore %s"},
	"cli.cloud.pull_missing":   {En: "%d items had no data in the cloud; they cannot be restored.", Es: "%d elementos no tenían datos en la nube; no se podrán restaurar."},
	"cli.cloud.pull_none":      {En: "The cloud has no snapshots (from that machine).", Es: "La nube no tiene snapshots (de ese equipo)."},
	"cli.cloud.pull_ambiguous": {En: "No single cloud snapshot starts with %s.", Es: "No hay un único snapshot en la nube que empiece por %s."},
	"cli.cloud.list_header":    {En: "ID\tDATE\tMACHINE\tSIZE\tHERE", Es: "ID\tFECHA\tEQUIPO\tTAMAÑO\tAQUÍ"},
	"cli.cloud.list_here":      {En: "yes", Es: "sí"},
	"cli.cloud.devices_header": {En: "ID\tNAME\tPLATFORM\tLAST SEEN\tSTATUS", Es: "ID\tNOMBRE\tPLATAFORMA\tÚLTIMO CONTACTO\tESTADO"},
	"cli.cloud.device_this":    {En: "this machine", Es: "este equipo"},
	"cli.cloud.device_revoked": {En: "revoked", Es: "revocado"},
	"cli.cloud.device_active":  {En: "active", Es: "activo"},
	"cli.cloud.device_unknown": {En: "No single device starts with %s.", Es: "No hay un único dispositivo que empiece por %s."},
	"cli.cloud.revoked":        {En: "Device «%s» revoked: it can no longer use the cloud.", Es: "Dispositivo «%s» revocado: ya no puede usar la nube."},
	"cli.cloud.revoke_self":    {En: "That is this machine; to disconnect it use: ccp cloud logout", Es: "Ese es este equipo; para desconectarlo usa: ccp cloud logout"},
}
```

Y en `cli.help.body` (En y Es), tras la sección SNAPSHOTS del plan de snapshots:

```
CLOUD                             end-to-end encrypted; the server cannot read it
  ccp cloud login <server> | init | unlock | status
  ccp cloud push | pull [<id>|latest] | list | devices | revoke <device> | logout
```

```
NUBE                              cifrada de punta a punta; el servidor no puede leerla
  ccp cloud login <servidor> | init | unlock | status
  ccp cloud push | pull [<id>|latest] | list | devices | revoke <dispositivo> | logout
```

- [ ] **Step 4: Implementar `internal/cli/cloud.go`**

```go
package cli

// cloud.go — `ccp cloud …` (spec 2026-09-18 §10). La lógica está en
// internal/cloud/client; aquí se orquesta y se pinta.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/mattn/go-isatty"

	"github.com/JoseAFlores777/ccp/internal/cloud/api"
	"github.com/JoseAFlores777/ccp/internal/cloud/client"
	"github.com/JoseAFlores777/ccp/internal/cloud/crypt"
	"github.com/JoseAFlores777/ccp/internal/core"
	"github.com/JoseAFlores777/ccp/internal/core/i18n"
	"github.com/JoseAFlores777/ccp/internal/snapshot"
)

func dispatchCloud(args []string, stdout, stderr io.Writer) int {
	home, err := ccpHome()
	if err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}
	if err := ensureMigrated(home); err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	c := cloudCmd{ctx: ctx, home: home, lang: currentLang(), files: client.NewFiles(home), out: stdout, err: stderr}
	sub := ""
	if len(args) > 0 {
		sub, args = args[0], args[1:]
	}
	switch sub {
	case "login":
		return c.login(args)
	case "logout":
		return c.logout(args)
	case "status", "":
		return c.status(args)
	case "init":
		return c.initVault(args)
	case "unlock":
		return c.unlock(args)
	case "push":
		return c.push(args)
	case "pull":
		return c.pull(args)
	case "list", "ls":
		return c.list(args)
	case "devices":
		return c.devices(args)
	case "revoke":
		return c.revoke(args)
	case "help", "-h", "--help":
		fmt.Fprintln(stdout, i18n.T(c.lang, "cli.cloud.usage"))
		return 0
	default:
		return c.usage("cli.cloud.unknown_sub", sub)
	}
}

type cloudCmd struct {
	ctx   context.Context
	home  string
	lang  i18n.Lang
	files client.Files
	out   io.Writer
	err   io.Writer
}

func (c cloudCmd) usage(key string, a ...any) int {
	fmt.Fprintln(c.err, i18n.T(c.lang, key, a...))
	fmt.Fprintln(c.err, i18n.T(c.lang, "cli.cloud.usage"))
	return 1
}

// fail traduce los errores con solución conocida a su mensaje.
func (c cloudCmd) fail(err error) int {
	switch {
	case errors.Is(err, client.ErrNotLoggedIn):
		fmt.Fprintln(c.err, i18n.T(c.lang, "cli.cloud.not_logged_in"))
	case errors.Is(err, client.ErrLocked):
		fmt.Fprintln(c.err, i18n.T(c.lang, "cli.cloud.locked"))
	case errors.Is(err, crypt.ErrWrongSecret):
		fmt.Fprintln(c.err, i18n.T(c.lang, "cli.cloud.unlock_wrong"))
	default:
		fmt.Fprintf(c.err, "Error: %v\n", err)
	}
	return 1
}

func (c cloudCmd) args(args, bools, valued []string, maxPos int) (snapArgs, bool) {
	a, bad, ok := parseSnapArgs(args, bools, valued)
	if !ok {
		c.usage("cli.cloud.unknown_opt", bad)
		return a, false
	}
	if len(a.pos) > maxPos {
		c.usage("cli.cloud.unknown_opt", a.pos[maxPos])
		return a, false
	}
	return a, true
}

// openBrowser abre la URL del login en macOS. Best-effort: la URL también se
// imprime. CCP_NO_BROWSER lo apaga (tests, SSH).
func openBrowser(u string) {
	if os.Getenv("CCP_NO_BROWSER") != "" || runtime.GOOS != "darwin" {
		return
	}
	_ = exec.Command("open", u).Start()
}

// readCloudSecret lee un secreto de la variable env o, en una terminal, sin eco.
// confirm pide longitud mínima y repetirlo (al crear la bóveda).
func readCloudSecret(c cloudCmd, env, promptKey string, confirm bool) (string, error) {
	if v := os.Getenv(env); v != "" {
		if confirm && len([]rune(v)) < snapMinPassphrase {
			return "", errors.New(i18n.T(c.lang, "cli.cloud.pass_short", snapMinPassphrase))
		}
		return v, nil
	}
	if !isatty.IsTerminal(os.Stdin.Fd()) {
		return "", errors.New(i18n.T(c.lang, "cli.cloud.pass_needed", env))
	}
	v, err := promptSecret(c.err, i18n.T(c.lang, promptKey))
	if err != nil {
		return "", err
	}
	if !confirm {
		return v, nil
	}
	if len([]rune(v)) < snapMinPassphrase {
		return "", errors.New(i18n.T(c.lang, "cli.cloud.pass_short", snapMinPassphrase))
	}
	again, err := promptSecret(c.err, i18n.T(c.lang, "cli.cloud.pass_confirm"))
	if err != nil {
		return "", err
	}
	if again != v {
		return "", errors.New(i18n.T(c.lang, "cli.cloud.pass_mismatch"))
	}
	return v, nil
}

func (c cloudCmd) login(args []string) int {
	a, ok := c.args(args, nil, []string{"--name"}, 1)
	if !ok {
		return 1
	}
	server := ""
	if len(a.pos) == 1 {
		server = strings.TrimSuffix(a.pos[0], "/")
	} else if prev, err := c.files.LoadConfig(); err == nil {
		server = prev.Server
	}
	if server == "" {
		server = os.Getenv("CCP_CLOUD_URL")
	}
	if server == "" {
		return c.usage("cli.cloud.need_server")
	}
	local := strings.HasPrefix(server, "http://127.0.0.1") || strings.HasPrefix(server, "http://localhost")
	if !strings.HasPrefix(server, "https://") && !local {
		fmt.Fprintln(c.err, i18n.T(c.lang, "cli.cloud.need_https"))
		return 1
	}
	ep, err := client.Discover(c.ctx, http.DefaultClient, server)
	if err != nil {
		return c.fail(err)
	}
	oc := client.OAuthConfig(ep.Info.ClientID, ep.TokenURL, ep.DeviceAuthURL)
	tok, err := client.Login(c.ctx, oc, func(u, code string) {
		fmt.Fprintln(c.out, i18n.T(c.lang, "cli.cloud.login_open", u, code))
		openBrowser(u)
	})
	if err != nil {
		return c.fail(err)
	}
	if err := c.files.SaveToken(tok); err != nil {
		return c.fail(err)
	}
	cl := client.NewAPI(server, client.HTTPClient(c.ctx, oc, c.files, tok), "")
	me, err := cl.Me(c.ctx)
	if err != nil {
		return c.fail(err)
	}
	cfg := client.Config{Server: server, Issuer: ep.Info.Issuer, ClientID: ep.Info.ClientID,
		TokenURL: ep.TokenURL, DeviceURL: ep.DeviceAuthURL, UserID: me.UserID, Email: me.Email}
	// Si este equipo ya estaba registrado con esta cuenta, se reusa su dispositivo.
	if prev, err := c.files.LoadConfig(); err == nil && prev.Server == server && prev.UserID == me.UserID && prev.DeviceID != "" {
		cfg.DeviceID, cfg.DeviceName = prev.DeviceID, prev.DeviceName
	}
	if cfg.DeviceID == "" {
		name := a.val("--name")
		if name == "" {
			name = snapMachine()
		}
		d, err := cl.RegisterDevice(c.ctx, api.DeviceIn{Name: name, Platform: runtime.GOOS + "/" + runtime.GOARCH, CCPVersion: core.Version})
		if err != nil {
			return c.fail(err)
		}
		cfg.DeviceID, cfg.DeviceName = d.ID, d.Name
	}
	if err := c.files.SaveConfig(cfg); err != nil {
		return c.fail(err)
	}
	fmt.Fprintln(c.out, okLine(c.out, i18n.T(c.lang, "cli.cloud.login_ok", me.Email, cfg.DeviceName)))
	if me.HasVault {
		fmt.Fprintln(c.out, i18n.T(c.lang, "cli.cloud.next_unlock"))
	} else {
		fmt.Fprintln(c.out, i18n.T(c.lang, "cli.cloud.next_init"))
	}
	return 0
}

func (c cloudCmd) logout(args []string) int {
	if _, ok := c.args(args, nil, nil, 0); !ok {
		return 1
	}
	// Revocar arriba es best-effort: sin conexión, el equipo se olvida igual.
	if cfg, cl, err := client.Session(c.ctx, c.files); err == nil {
		_ = cl.RevokeDevice(c.ctx, cfg.DeviceID)
	}
	if err := c.files.Forget(); err != nil {
		return c.fail(err)
	}
	fmt.Fprintln(c.out, okLine(c.out, i18n.T(c.lang, "cli.cloud.logged_out")))
	return 0
}

func (c cloudCmd) pendingPush() int {
	st, err := core.OpenSnapshotStore(c.home)
	if err != nil {
		return 0
	}
	ms, err := st.List()
	if err != nil {
		return 0
	}
	state, _ := c.files.LoadState()
	n := 0
	for _, m := range ms {
		if _, ok := state.Pushed[m.ID]; !ok {
			n++
		}
	}
	return n
}

func (c cloudCmd) status(args []string) int {
	a, ok := c.args(args, []string{"--json"}, nil, 0)
	if !ok {
		return 1
	}
	cfg, cl, err := client.Session(c.ctx, c.files)
	loggedIn := err == nil
	vault := "unknown"
	if loggedIn {
		if _, err := c.files.LoadAK(); err == nil {
			vault = "unlocked"
		} else {
			ctx, cancel := context.WithTimeout(c.ctx, 5*time.Second)
			me, err := cl.Me(ctx)
			cancel()
			switch {
			case err != nil:
				vault = "unknown"
			case me.HasVault:
				vault = "locked"
			default:
				vault = "missing"
			}
		}
	}
	pending := c.pendingPush()
	if a.flags["--json"] {
		return snapJSON(c.out, c.err, map[string]any{
			"logged_in": loggedIn, "server": cfg.Server, "email": cfg.Email, "device_id": cfg.DeviceID,
			"device_name": cfg.DeviceName, "vault": vault, "pending_push": pending,
		})
	}
	if !loggedIn {
		fmt.Fprintln(c.out, i18n.T(c.lang, "cli.cloud.not_logged_in"))
		return 0
	}
	fmt.Fprintln(c.out, i18n.T(c.lang, "cli.cloud.status_server", cfg.Server))
	fmt.Fprintln(c.out, i18n.T(c.lang, "cli.cloud.status_account", cfg.Email))
	fmt.Fprintln(c.out, i18n.T(c.lang, "cli.cloud.status_device", cfg.DeviceName))
	fmt.Fprintln(c.out, i18n.T(c.lang, "cli.cloud.status_vault", i18n.T(c.lang, "cli.cloud.vault_"+vault)))
	fmt.Fprintln(c.out, i18n.T(c.lang, "cli.cloud.status_pending", pending))
	return 0
}

func (c cloudCmd) initVault(args []string) int {
	if _, ok := c.args(args, nil, nil, 0); !ok {
		return 1
	}
	_, cl, err := client.Session(c.ctx, c.files)
	if err != nil {
		return c.fail(err)
	}
	if me, err := cl.Me(c.ctx); err != nil {
		return c.fail(err)
	} else if me.HasVault {
		fmt.Fprintln(c.err, i18n.T(c.lang, "cli.cloud.vault_exists"))
		return 1
	}
	pass, err := readCloudSecret(c, "CCP_CLOUD_PASSPHRASE", "cli.cloud.pass_prompt", true)
	if err != nil {
		return c.fail(err)
	}
	ak, code, w, err := crypt.NewVault([]byte(pass))
	if err != nil {
		return c.fail(err)
	}
	v, err := client.VaultToAPI(w)
	if err != nil {
		return c.fail(err)
	}
	if err := cl.PutVault(c.ctx, v); err != nil {
		var apiErr *client.APIError
		if errors.As(err, &apiErr) && apiErr.Code == api.CodeConflict {
			fmt.Fprintln(c.err, i18n.T(c.lang, "cli.cloud.vault_exists"))
			return 1
		}
		return c.fail(err)
	}
	if err := c.files.SaveAK(ak); err != nil {
		return c.fail(err)
	}
	fmt.Fprintln(c.out, okLine(c.out, i18n.T(c.lang, "cli.cloud.vault_created")))
	fmt.Fprintln(c.out)
	fmt.Fprintln(c.out, boldLine(c.out, i18n.T(c.lang, "cli.cloud.recovery_title")))
	fmt.Fprintln(c.out, "    "+boldLine(c.out, code))
	fmt.Fprintln(c.out, i18n.T(c.lang, "cli.cloud.recovery_hint"))
	return 0
}

func (c cloudCmd) unlock(args []string) int {
	a, ok := c.args(args, []string{"--recovery"}, nil, 0)
	if !ok {
		return 1
	}
	_, cl, err := client.Session(c.ctx, c.files)
	if err != nil {
		return c.fail(err)
	}
	v, err := cl.Vault(c.ctx)
	var apiErr *client.APIError
	if errors.As(err, &apiErr) && apiErr.Code == api.CodeNotFound {
		fmt.Fprintln(c.err, i18n.T(c.lang, "cli.cloud.no_vault"))
		return 1
	}
	if err != nil {
		return c.fail(err)
	}
	w, err := client.VaultFromAPI(v)
	if err != nil {
		return c.fail(err)
	}
	var ak []byte
	if a.flags["--recovery"] {
		code, err := readCloudSecret(c, "CCP_CLOUD_RECOVERY", "cli.cloud.recovery_prompt", false)
		if err != nil {
			return c.fail(err)
		}
		ak, err = crypt.UnlockRecovery(w, code)
		if err != nil {
			return c.fail(err)
		}
	} else {
		pass, err := readCloudSecret(c, "CCP_CLOUD_PASSPHRASE", "cli.cloud.pass_prompt", false)
		if err != nil {
			return c.fail(err)
		}
		ak, err = crypt.UnlockPassphrase(w, []byte(pass))
		if err != nil {
			return c.fail(err)
		}
	}
	if err := c.files.SaveAK(ak); err != nil {
		return c.fail(err)
	}
	fmt.Fprintln(c.out, okLine(c.out, i18n.T(c.lang, "cli.cloud.unlocked")))
	return 0
}

// ready abre la sesión, la cuenta y el almacén local: lo que necesitan push y pull.
func (c cloudCmd) ready() (*client.API, *crypt.Account, *snapshot.Store, error) {
	_, cl, err := client.Session(c.ctx, c.files)
	if err != nil {
		return nil, nil, nil, err
	}
	acct, err := client.Account(c.files)
	if err != nil {
		return nil, nil, nil, err
	}
	st, err := core.OpenSnapshotStore(c.home)
	if err != nil {
		return nil, nil, nil, err
	}
	return cl, acct, st, nil
}

func (c cloudCmd) push(args []string) int {
	a, ok := c.args(args, []string{"--json"}, nil, 1)
	if !ok {
		return 1
	}
	cl, acct, st, err := c.ready()
	if err != nil {
		return c.fail(err)
	}
	only := ""
	if len(a.pos) == 1 {
		if only, err = st.Resolve(a.pos[0]); err != nil {
			return c.fail(err)
		}
	}
	rep, err := client.Push(c.ctx, cl, acct, st, c.files, only)
	if err != nil {
		return c.fail(err)
	}
	if a.flags["--json"] {
		return snapJSON(c.out, c.err, rep)
	}
	if rep.Snapshots == 0 {
		fmt.Fprintln(c.out, i18n.T(c.lang, "cli.cloud.push_nothing"))
	} else {
		fmt.Fprintln(c.out, okLine(c.out, i18n.T(c.lang, "cli.cloud.pushed", rep.Snapshots, rep.Uploaded, humanBytes(rep.Bytes))))
	}
	if len(rep.Missing) > 0 {
		fmt.Fprintln(c.out, warnLine(c.out, i18n.T(c.lang, "cli.cloud.push_missing", len(rep.Missing))))
	}
	if len(rep.TooLarge) > 0 {
		fmt.Fprintln(c.out, warnLine(c.out, i18n.T(c.lang, "cli.cloud.push_too_large", strings.Join(rep.TooLarge, ", "))))
	}
	return 0
}

func (c cloudCmd) pull(args []string) int {
	a, ok := c.args(args, nil, []string{"--device"}, 1)
	if !ok {
		return 1
	}
	cl, acct, st, err := c.ready()
	if err != nil {
		return c.fail(err)
	}
	device := ""
	if name := a.val("--device"); name != "" {
		d, err := c.findDevice(cl, name)
		if err != nil {
			return c.fail(err)
		}
		device = d.ID
	}
	list, err := cl.Snapshots(c.ctx, device, 1000)
	if err != nil {
		return c.fail(err)
	}
	if len(list) == 0 {
		fmt.Fprintln(c.out, i18n.T(c.lang, "cli.cloud.pull_none"))
		return 0
	}
	target := list[0].ID // «latest» por defecto: la lista viene del más nuevo al más viejo
	if len(a.pos) == 1 && a.pos[0] != "latest" {
		var found []string
		for _, s := range list {
			if strings.HasPrefix(s.ID, a.pos[0]) {
				found = append(found, s.ID)
			}
		}
		if len(found) != 1 || len(a.pos[0]) < 4 {
			fmt.Fprintln(c.err, i18n.T(c.lang, "cli.cloud.pull_ambiguous", a.pos[0]))
			return 1
		}
		target = found[0]
	}
	m, missing, err := client.Pull(c.ctx, cl, acct, st, c.files, target)
	if err != nil {
		return c.fail(err)
	}
	fmt.Fprintln(c.out, okLine(c.out, i18n.T(c.lang, "cli.cloud.pulled", snapshot.Short(target), snapshot.Short(m.ID))))
	if len(missing) > 0 {
		fmt.Fprintln(c.out, warnLine(c.out, i18n.T(c.lang, "cli.cloud.pull_missing", len(missing))))
	}
	fmt.Fprintln(c.out, mute(c.out, i18n.T(c.lang, "cli.cloud.pull_hint", snapshot.Short(m.ID))))
	return 0
}

func (c cloudCmd) list(args []string) int {
	a, ok := c.args(args, []string{"--json"}, nil, 0)
	if !ok {
		return 1
	}
	_, cl, err := client.Session(c.ctx, c.files)
	if err != nil {
		return c.fail(err)
	}
	list, err := cl.Snapshots(c.ctx, "", 1000)
	if err != nil {
		return c.fail(err)
	}
	state, _ := c.files.LoadState()
	here := map[string]bool{}
	for _, cloudID := range state.Pushed {
		here[cloudID] = true
	}
	if a.flags["--json"] {
		type row struct {
			api.SnapshotMeta
			Here bool `json:"here"`
		}
		out := make([]row, 0, len(list))
		for _, s := range list {
			out = append(out, row{s, here[s.ID]})
		}
		return snapJSON(c.out, c.err, out)
	}
	tw := tabwriter.NewWriter(c.out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, i18n.T(c.lang, "cli.cloud.list_header"))
	for _, s := range list {
		mark := ""
		if here[s.ID] {
			mark = i18n.T(c.lang, "cli.cloud.list_here")
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", snapshot.Short(s.ID), s.Created.Local().Format("2006-01-02 15:04"), s.DeviceName, humanBytes(s.Size), mark)
	}
	_ = tw.Flush()
	return 0
}

func (c cloudCmd) findDevice(cl *client.API, ref string) (api.Device, error) {
	ds, err := cl.Devices(c.ctx)
	if err != nil {
		return api.Device{}, err
	}
	var found []api.Device
	for _, d := range ds {
		if d.Name == ref || strings.HasPrefix(d.ID, ref) {
			found = append(found, d)
		}
	}
	if len(found) != 1 {
		return api.Device{}, errors.New(i18n.T(c.lang, "cli.cloud.device_unknown", ref))
	}
	return found[0], nil
}

func (c cloudCmd) devices(args []string) int {
	a, ok := c.args(args, []string{"--json"}, nil, 0)
	if !ok {
		return 1
	}
	cfg, cl, err := client.Session(c.ctx, c.files)
	if err != nil {
		return c.fail(err)
	}
	ds, err := cl.Devices(c.ctx)
	if err != nil {
		return c.fail(err)
	}
	if a.flags["--json"] {
		return snapJSON(c.out, c.err, ds)
	}
	tw := tabwriter.NewWriter(c.out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, i18n.T(c.lang, "cli.cloud.devices_header"))
	for _, d := range ds {
		status := i18n.T(c.lang, "cli.cloud.device_active")
		switch {
		case d.ID == cfg.DeviceID:
			status = i18n.T(c.lang, "cli.cloud.device_this")
		case d.Revoked:
			status = i18n.T(c.lang, "cli.cloud.device_revoked")
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", d.ID[:8], d.Name, d.Platform, d.LastSeen.Local().Format("2006-01-02 15:04"), status)
	}
	_ = tw.Flush()
	return 0
}

func (c cloudCmd) revoke(args []string) int {
	a, ok := c.args(args, nil, nil, 1)
	if !ok {
		return 1
	}
	if len(a.pos) != 1 {
		return c.usage("cli.cloud.device_unknown", "")
	}
	cfg, cl, err := client.Session(c.ctx, c.files)
	if err != nil {
		return c.fail(err)
	}
	d, err := c.findDevice(cl, a.pos[0])
	if err != nil {
		return c.fail(err)
	}
	if d.ID == cfg.DeviceID {
		fmt.Fprintln(c.err, i18n.T(c.lang, "cli.cloud.revoke_self"))
		return 1
	}
	if err := cl.RevokeDevice(c.ctx, d.ID); err != nil {
		return c.fail(err)
	}
	fmt.Fprintln(c.out, okLine(c.out, i18n.T(c.lang, "cli.cloud.revoked", d.Name)))
	return 0
}
```

Engánchalo en `internal/cli/cli.go`, junto a `case "snapshot":`:

```go
	// `cloud` tampoco toca el entorno del shell padre (entra por el `*)` del rc)
	// ni está en la completion.
	case "cloud":
		return dispatchCloud(rest, stdout, stderr)
```

y añade `"cloud": true` a `dailySnapshotCmds` en `internal/cli/snapshot.go`.

- [ ] **Step 5: Comprobar que pasa**

Run: `go test ./internal/cli/ -run Cloud && go test ./internal/... && go vet ./...`
Expected: PASS.

- [ ] **Step 6: El binario `ccp` no arrastra el servidor**

Run: `go list -deps ./cmd/ccp | grep -E 'jackc/pgx|aws-sdk-go-v2|go-oidc|internal/cloud/(server|store|blobs)$'`
Expected: **ninguna línea**. Si aparece alguna, algo de `internal/cli` o de
`internal/cloud/client` importa el lado del servidor fuera de un `_test.go`.

- [ ] **Step 7: Gates y commit**

Mensaje propuesto: `feat(cli): ccp cloud (login, init, unlock, push, pull, list, devices, revoke, logout)`

---

### Task 13: integración contra Postgres y Alarik reales

Lo que la memoria no puede probar se prueba aquí, contra una pila local
desechable:
- el SQL, las migraciones y la transacción del commit;
- que Alarik acepta las URLs prefirmadas del SDK (la **prueba de contrato del
  almacenamiento** del spec);
- un push/pull completo sobre ambos.

Todo va bajo `//go:build integration`: `go test ./...` no lo toca. El script lo
ejecuta con `-p 1`, porque el test de la persistencia borra tablas.

**Files:**
- Create: `deploy/ccp-cloud/test-stack.yml`
- Create: `deploy/ccp-cloud/integration.sh`
- Create: `internal/cloud/client/integration_test.go` (`//go:build integration`)
- Modify: `internal/cloud/client/client_test.go` (`newRig` pasa a llamar a `newRigWith`)
- Modify: `.github/workflows/ci.yml` (job `cloud-integration`)

- [ ] **Step 1: La pila — `deploy/ccp-cloud/test-stack.yml`**

```yaml
# Pila local para los tests de integración de la nube (go test -tags integration).
# Credenciales fijas de prueba: esta pila no se despliega en ningún sitio y solo
# escucha en 127.0.0.1.
name: ccp-cloud-test
services:
  postgres:
    image: postgres:17.11-alpine
    environment:
      - POSTGRES_PASSWORD=test
    ports:
      - "127.0.0.1:55432:5432"
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -h 127.0.0.1 -U postgres"]
      interval: 2s
      retries: 30
  alarik:
    image: ghcr.io/achtungsoftware/alarik:1.0.0-beta-16
    environment:
      - API_BASE_URL=http://127.0.0.1:58080
      - CONSOLE_BASE_URL=http://127.0.0.1:53000
      - ADMIN_USERNAME=test
      - ADMIN_PASSWORD=test-password-123
      - JWT=dGVzdC1qd3QtdGVzdC1qd3QtdGVzdC1qd3QtdGVzdC1qd3Q=
      - ALLOW_ACCOUNT_CREATION=false
      - DEFAULT_ACCESS_KEY=CCPTESTACCESSKEY0001
      - DEFAULT_SECRET_KEY=ccp-test-secret-key-0000000000000000000
      - DEFAULT_BUCKETS=ccp-blobs
      - ALARIK_REGION=us-east-1
    ports:
      - "127.0.0.1:58080:8080"
```

- [ ] **Step 2: El script — `deploy/ccp-cloud/integration.sh`**

```bash
#!/usr/bin/env bash
# Levanta Postgres + Alarik locales, corre los tests de integración de la nube y
# lo desmonta todo (también si fallan). Uso: bash deploy/ccp-cloud/integration.sh
set -euo pipefail
here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
root="$(cd "${here}/../.." && pwd)"
compose=(docker compose -f "${here}/test-stack.yml")

cleanup() { "${compose[@]}" down -v >/dev/null 2>&1 || true; }
trap cleanup EXIT

"${compose[@]}" up -d --wait postgres
"${compose[@]}" up -d alarik
# Alarik no trae healthcheck: listo cuando responde (un 403 sin firma vale).
for _ in $(seq 1 60); do
  code="$(curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:58080/ || true)"
  [[ "${code}" != "000" ]] && break
  sleep 1
done

export CCP_CLOUD_TEST_DSN='host=127.0.0.1 port=55432 user=postgres password=test dbname=postgres sslmode=disable'
export CCP_CLOUD_TEST_S3_ENDPOINT='http://127.0.0.1:58080'
export CCP_CLOUD_TEST_S3_ACCESS_KEY='CCPTESTACCESSKEY0001'
export CCP_CLOUD_TEST_S3_SECRET_KEY='ccp-test-secret-key-0000000000000000000'

cd "${root}"
go test -tags integration -p 1 -count=1 ./internal/cloud/...
```

- [ ] **Step 3: Push/pull sobre la pila real — `internal/cloud/client/integration_test.go`**

Primero refactoriza `newRig` en `client_test.go` para que acepte la persistencia
y el almacenamiento:

```go
func newRig(t *testing.T, wrap func(store.Store) store.Store) *rig {
	var st store.Store = store.NewMem()
	if wrap != nil {
		st = wrap(st)
	}
	return newRigWith(t, st, blobstest.New(t))
}

func newRigWith(t *testing.T, st store.Store, bl blobs.Blobs) *rig {
	t.Helper()
	iss := oidctest.New(t)
	h := server.New(server.Config{
		Store: st, Blobs: bl, Verifier: server.NewOIDCVerifier(iss.URL, iss.JWKSURL(), oidctest.Audience),
		Issuer: iss.URL, ClientID: oidctest.ClientID,
	})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return &rig{iss: iss, url: srv.URL}
}
```

(importa `github.com/JoseAFlores777/ccp/internal/cloud/blobs`). Después:

```go
//go:build integration

package client

import (
	"context"
	"os"
	"testing"

	"github.com/JoseAFlores777/ccp/internal/cloud/blobs"
	"github.com/JoseAFlores777/ccp/internal/cloud/crypt"
	"github.com/JoseAFlores777/ccp/internal/cloud/store"
)

// El flujo completo con Postgres y Alarik de verdad: SQL, transacción del commit,
// URLs prefirmadas por el SDK de AWS y aceptadas por Alarik.
func TestPushPullOnRealStack(t *testing.T) {
	dsn, ep := os.Getenv("CCP_CLOUD_TEST_DSN"), os.Getenv("CCP_CLOUD_TEST_S3_ENDPOINT")
	if dsn == "" || ep == "" {
		t.Skip("falta la pila de integración (deploy/ccp-cloud/integration.sh)")
	}
	ctx := context.Background()
	pg, err := store.OpenPG(ctx, dsn, "")
	if err != nil {
		t.Fatal(err)
	}
	defer pg.Close()
	if err := pg.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	bl := blobs.NewS3(blobs.S3Config{Endpoint: ep, PublicEndpoint: ep, Region: "us-east-1", Bucket: "ccp-blobs",
		AccessKey: os.Getenv("CCP_CLOUD_TEST_S3_ACCESS_KEY"), SecretKey: os.Getenv("CCP_CLOUD_TEST_S3_SECRET_KEY")})
	r := newRigWith(t, pg, bl)
	// Un sub único por ejecución: la base no se vacía entre ejecuciones.
	r.iss.As("it-"+t.Name()+"-"+os.Getenv("GITHUB_RUN_ID"), "it@example.com")

	filesA, apiA, stA := r.machine(t, "it-a")
	acct, _, m := seed(t, apiA, stA)
	if rep, err := Push(ctx, apiA, acct, stA, filesA, ""); err != nil || rep.Snapshots != 1 {
		t.Fatalf("Push = %+v, %v", rep, err)
	}
	filesB, apiB, stB := r.machine(t, "it-b")
	list, err := apiB.Snapshots(ctx, "", 1)
	if err != nil || len(list) != 1 {
		t.Fatalf("Snapshots = %v, %v", list, err)
	}
	v, _ := apiB.Vault(ctx)
	w, _ := VaultFromAPI(v)
	ak, err := crypt.UnlockPassphrase(w, []byte(phrase))
	if err != nil {
		t.Fatal(err)
	}
	acctB, _ := crypt.NewAccount(ak)
	got, missing, err := Pull(ctx, apiB, acctB, stB, filesB, list[0].ID)
	if err != nil || got.ID != m.ID || len(missing) != 0 {
		t.Fatalf("Pull = %v %v %v", got, missing, err)
	}
}
```

Nota: `TestPGContract` (Task 7) borra las tablas al empezar. Con `-p 1`, los
paquetes corren de uno en uno y el orden no importa: cada test migra lo que
necesita.

- [ ] **Step 4: Ejecutarlo**

Run: `bash deploy/ccp-cloud/integration.sh`
Expected: PASS en `internal/cloud/store` (`TestPGContract`), `internal/cloud/blobs`
(`TestS3Contract`) e `internal/cloud/client` (`TestPushPullOnRealStack`).

Si `TestS3Contract` falla en el PUT prefirmado, lo más probable es que Alarik
rechace alguna cabecera o parámetro que añade el SDK. Mira el cuerpo del 4xx.
Si es un problema de sumas de comprobación, confirma que `NewS3` fija
`RequestChecksumCalculation: WhenRequired`. Es el hallazgo que esta prueba
existe para encontrar **antes** de desplegar.

- [ ] **Step 5: El job de CI — `.github/workflows/cloud-integration` en `ci.yml`**

Añade un job que corra en `ubuntu-latest` con `actions/setup-go` (misma
`GO_VERSION`) y ejecute `bash deploy/ccp-cloud/integration.sh`. Los runners de
GitHub ya traen Docker y `docker compose`. **Que no bloquee** los demás jobs
(sin `needs:` hacia ellos): si Alarik (beta) tiene un mal día, el resto del CI
sigue dando señal.

- [ ] **Step 6: Gates y commit**

Mensaje propuesto: `test(cloud): integración contra Postgres y Alarik reales`

---

### Task 14: desplegar el API

Imagen distroless publicada en GHCR al etiquetar `cloud-v*`. Se añade como
servicio `api` al compose del stack `ccp-cloud` y se sirve en
`https://ccp.joseiz.com`. Las contraseñas **ya existen** en el `.env` del compose
(las generó la plantilla); el servicio solo las referencia.

**Files:**
- Create: `deploy/ccp-cloud/Dockerfile`
- Create: `.github/workflows/cloud-image.yml`
- Modify: `deploy/ccp-cloud/docker-compose.yml` (servicio `api`)
- Modify: `deploy/ccp-cloud/README.md` (API, dominio, pasos)

- [ ] **Step 1: `deploy/ccp-cloud/Dockerfile`**

```dockerfile
# syntax=docker/dockerfile:1
# Imagen del API de ccp-cloud. Se construye desde la raíz del repo:
#   docker build -f deploy/ccp-cloud/Dockerfile --build-arg VERSION=cloud-v0.1.0 .
FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/ccp-cloud ./cmd/ccp-cloud

# distroless: sin shell ni paquetes, y sin root. El healthcheck es el propio
# binario (`ccp-cloud healthcheck`).
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/ccp-cloud /ccp-cloud
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/ccp-cloud"]
```

Run: `docker build -f deploy/ccp-cloud/Dockerfile -t ccp-cloud:local .`
Expected: la imagen se construye.

- [ ] **Step 2: `.github/workflows/cloud-image.yml`**

```yaml
# Publica la imagen del API al etiquetar cloud-vX.Y.Z. Separado de release.yml:
# el binario ccp y el servidor tienen ciclos de versión distintos.
name: cloud-image
on:
  push:
    tags: ["cloud-v*"]
permissions:
  contents: read
  packages: write
jobs:
  image:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: docker/setup-qemu-action@v3
      - uses: docker/setup-buildx-action@v3
      - uses: docker/login-action@v3
        with:
          registry: ghcr.io
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}
      - uses: docker/build-push-action@v6
        with:
          context: .
          file: deploy/ccp-cloud/Dockerfile
          platforms: linux/amd64,linux/arm64
          push: true
          build-args: VERSION=${{ github.ref_name }}
          tags: ghcr.io/joseaflores777/ccp-cloud:${{ github.ref_name }}
```

- [ ] **Step 3: El servicio `api` en `deploy/ccp-cloud/docker-compose.yml`**

Añade al comentario de cabecera la línea
`#   api       backend /v1 (cmd/ccp-cloud); https://ccp.joseiz.com` y este
servicio:

```yaml
  api:
    image: ghcr.io/joseaflores777/ccp-cloud:cloud-v0.1.0
    restart: unless-stopped
    environment:
      - CCP_CLOUD_OIDC_ISSUER=https://ccp-auth.joseiz.com/realms/ccp
      # JWKS por la red interna: si Keycloak cae, el API sigue arriba y responde
      # 401 hasta que vuelva.
      - CCP_CLOUD_OIDC_JWKS=http://keycloak:8080/realms/ccp/protocol/openid-connect/certs
      - CCP_CLOUD_DB_HOST=postgres
      - CCP_CLOUD_DB_USER=ccp
      - CCP_CLOUD_DB_NAME=ccp
      - CCP_CLOUD_DB_PASSWORD=${CCP_DB_PASSWORD}
      - CCP_CLOUD_S3_ENDPOINT=http://alarik:8080
      - CCP_CLOUD_S3_PUBLIC_ENDPOINT=https://ccp-s3.joseiz.com
      - CCP_CLOUD_S3_BUCKET=ccp-blobs
      - CCP_CLOUD_S3_ACCESS_KEY=${CCP_S3_ACCESS_KEY}
      - CCP_CLOUD_S3_SECRET_KEY=${CCP_S3_SECRET_KEY}
    expose:
      - "8080"
    depends_on:
      postgres:
        condition: service_healthy
      alarik:
        condition: service_started
    healthcheck:
      test: ["CMD", "/ccp-cloud", "healthcheck"]
      interval: 15s
      timeout: 5s
      retries: 5
      start_period: 20s
```

- [ ] **Step 4: Publicar la imagen (con el usuario)**

1. El usuario autoriza la etiqueta: `git tag cloud-v0.1.0 && git push origin cloud-v0.1.0`.
2. Espera a que `cloud-image` termine en verde.
3. En GitHub → Packages → `ccp-cloud` → *Package settings*, pon la visibilidad en
   **Public**. Si prefieres privada, registra en Dokploy el registro `ghcr.io`
   con un token de solo lectura (Settings → Registry).

- [ ] **Step 5: Aplicarlo en Dokploy (con el usuario)**

**Nunca reimportes la plantilla**: regeneraría todas las contraseñas.
1. **Compose.** Actualízalo con el archivo del repo. Con el MCP de Dokploy:
   `compose-update` sobre `composeId` `kZznoqlgDhkYW3W-nEyNr` pasando **solo**
   `composeFile` (el contenido de `deploy/ccp-cloud/docker-compose.yml`); sin
   `env`, para que las variables generadas no se toquen. Desde la interfaz es lo
   mismo: pegar el compose en la pestaña del compose, sin tocar *Environment*.
2. **Dominio.** `ccp.joseiz.com` → servicio `api`, puerto `8080`, https con Let's
   Encrypt (`domain-create`). Es de un solo nivel, como los otros: el certificado
   de Cloudflare lo cubre.
3. **Deploy**, y vigila que los contenedores terminen `healthy`. En el primer
   despliegue del stack, el 2026-09-18, se reiniciaron **todos** los contenedores
   del servidor dos veces sin causa aclarada. Esta vez comprueba antes y después
   la salud de los demás servicios (`auth.joseiz.com`, n8n…) y, si se repite,
   para y avisa al usuario antes de seguir.

- [ ] **Step 6: Humo**

```bash
curl -s -o /dev/null -w '%{http_code}\n' https://ccp.joseiz.com/healthz   # 204
curl -s https://ccp.joseiz.com/readyz                                       # {"status":"ok"}
curl -s https://ccp.joseiz.com/v1/info                                      # issuer ccp-auth…/realms/ccp, client ccp-cli
curl -s -o /dev/null -w '%{http_code}\n' https://ccp.joseiz.com/v1/me      # 401 sin token
```

Si `/readyz` da 503, el motivo está en los logs del contenedor `api`:
- **«keycloak»**: el JWKS interno no responde. Cambia `CCP_CLOUD_OIDC_JWKS` a la
  URL pública
  (`https://ccp-auth.joseiz.com/realms/ccp/protocol/openid-connect/certs`) y
  vuelve a desplegar.
- **«almacenamiento»**: revisa que el bucket `ccp-blobs` exista y que las claves
  coincidan.

- [ ] **Step 7: La prueba de verdad (con el usuario, en su Mac)**

```bash
ccp cloud login https://ccp.joseiz.com      # aprueba en el navegador: usuario + OTP del realm ccp
ccp cloud init                              # frase de la bóveda; guarda el código de recuperación
ccp snapshot create -m "primera subida"
ccp cloud push
ccp cloud status
```

Esperado: `push` dice «Subidos 1 snapshots…» y `status` muestra la bóveda
desbloqueada y 0 pendientes.

- [ ] **Step 8: Commit**

Mensaje propuesto: `feat(deploy): imagen del API y servicio api en el stack ccp-cloud`

---

### Task 15: documentación, ADRs y spec

**Files:**
- Create: `docs/adr/0013-cloud-end-to-end-encryption.md`
- Create: `docs/adr/0015-identity-keycloak-vault-separate.md`
- Modify: `README.md`, `README.es.md` (sección Nube)
- Modify: `CHANGELOG.md`
- Modify: `CLAUDE.md`
- Modify: `docs/superpowers/specs/2026-09-18-config-unificada-snapshots-nube-design.md`

- [ ] **Step 1: ADR 0013**

```markdown
# ADR 0013 — La nube cifra de punta a punta: el servidor no puede leer la configuración

Fecha: 2026-09-18 · Estado: aceptado · Spec: 2026-09-18-config-unificada-snapshots-nube §10.1

## Contexto

Un snapshot trae claves de API, tokens de MCP, y hooks y comandos que se
ejecutan en las máquinas del usuario. Un servidor capaz de leerlos y
escribirlos es, si lo comprometen, ejecución remota de código en todas esas
máquinas, además de una copia en claro de sus credenciales.

## Decisión

- Todo se cifra en el cliente, con subclaves (HKDF) de una clave de cuenta
  (AK) que el servidor nunca recibe:
  - blobs y manifiestos van sellados con XChaCha20-Poly1305, y su id como dato
    asociado;
  - los ids son HMAC de los hashes locales: el servidor deduplica sin saber qué
    guarda;
  - cada snapshot va firmado con Ed25519 (id, padre y hash del manifiesto
    sellado).
- El cliente verifica con la clave pública **derivada de la AK**, nunca con una
  que diga el servidor. Un servidor comprometido puede negar el servicio, pero no
  colar, alterar ni reordenar un snapshot.
- Los blobs van directos entre el cliente y el almacenamiento, con URLs
  prefirmadas. El API no los lee nunca.

## Consecuencias

- Sin la AK no hay recuperación posible. La frase y el código de recuperación
  (ADR 0015) son la única salida, y la CLI lo dice al crear la bóveda.
- El servidor no puede ofrecer búsqueda ni diff sobre el contenido: el portal
  (F2) descifra en el navegador.
- Metadatos visibles: fechas, tamaños, nombre de cada equipo y el grafo de
  padres. Se aceptan.
```

- [ ] **Step 2: ADR 0015**

```markdown
# ADR 0015 — Identidad en Keycloak y cifrado en la bóveda: dos secretos, dos dueños

Fecha: 2026-09-18 · Estado: aceptado · Spec: §10.2 (D9)

## Contexto

La autenticación la da Keycloak (realm `ccp` del stack `ccp-cloud`). Pero la
contraseña se escribe en la página de Keycloak, así que ccp no la ve y no puede
derivar de ella la clave de cifrado. Además, un reset de contraseña en Keycloak
destruiría los datos.

## Decisión

- **«¿Quién eres?» es de Keycloak**: concesión de dispositivo para la CLI,
  sesión offline como credencial del equipo, MFA, y usuarios gestionados en la
  consola.
- **«¿Puedes leer esto?» es de la bóveda**. Una AK aleatoria con dos envolturas
  que el servidor guarda sin poder abrir:
  - una con una **frase de bóveda** (Argon2id), distinta de la contraseña de
    Keycloak;
  - otra con un **código de recuperación** de 160 bits, que se enseña una sola
    vez.

  La clave pública de firma también se guarda, para que un equipo confirme al
  desbloquear que abrió la bóveda de su cuenta.
- **En F1, cada equipo guarda la AK desbloqueada en `cloud/vault.key` (0600).**
  Para un mismo usuario de macOS, un archivo 0600 y un Keychain accesible por
  `security` protegen lo mismo. La envoltura por dispositivo (X25519) llega con
  la aprobación de un equipo desde otro (F3).
- **Revocar un equipo** lo expulsa del API en la siguiente petición: el servidor
  comprueba el dispositivo en cada una. **No borra la AK que ese equipo ya
  tiene.** Si se teme una filtración, hay que rotar la AK (F4).

## Consecuencias

- Hay dos secretos que recordar. Es el precio de que el servidor no pueda leer.
- Perder la frase **y** el código de recuperación hace irrecuperables los datos
  de la nube; los snapshots locales siguen siendo la fuente primaria.
```

- [ ] **Step 3: README, CHANGELOG, CLAUDE.md, spec**

- **README.md / README.es.md**: sección «Nube» tras «Snapshots»:
  - qué es: snapshots cifrados de punta a punta en tu propio servidor;
  - primer equipo: `login` → `init` → `push`;
  - otro equipo: `login` → `unlock` → `pull` → `snapshot restore`;
  - qué ve el servidor y qué no;
  - el código de recuperación;
  - cómo se traduce el HOME entre máquinas;
  - enlace a `deploy/ccp-cloud/README.md` para montar el servidor.
- **CHANGELOG.md**, *Unreleased*:
  - `ccp cloud`;
  - el backend `ccp-cloud` (imagen `ghcr.io/joseaflores777/ccp-cloud`);
  - la traducción del HOME en `snapshot restore`.
- **CLAUDE.md**, un apartado *Cloud* en *Architecture* con las reglas que no se
  negocian:
  - el cliente no importa `server`, `store` ni `blobs`
    (`go list -deps ./cmd/ccp` lo comprueba);
  - el servidor nunca recibe nada que abra un dato;
  - la firma se verifica con la clave derivada de la AK;
  - revocar se comprueba en cada petición;
  - `ccp-cloud` se configura por entorno y la DSN va por campos;
  - nunca se reimporta la plantilla de Dokploy;
  - los tests de integración van tras `integration`.

  Y en *Config & state locations*, `cloud/` con sus cuatro archivos, todos 0600.
- **Spec**:
  - §10.2: una nota «F1 guarda la AK en `cloud/vault.key` (0600); la envoltura
    X25519 por dispositivo llega en F3 con la aprobación entre equipos»;
  - §10.4: «migraciones SQL embebidas con migrador propio en lugar de goose»;
  - §12: la fila F1, **implementada**.

- [ ] **Step 4: Gates finales y commit**

Todos los gates, más `bash legacy/tests/run.sh`,
`bash testdata/golden/capture.sh --check` y, si Docker está disponible,
`bash deploy/ccp-cloud/integration.sh`.
Mensaje propuesto: `docs: nube F1 (ADR 0013 y 0015, README, CHANGELOG, CLAUDE.md, spec)`

---

## Fuera de este plan

| Qué | Dónde |
|---|---|
| Portal web: historial, diff y descarga en el navegador (WASM para la criptografía) | F2 |
| Restaurar desde el portal: revisiones firmadas, agente (LaunchAgent), confirmación local de lo ejecutable, estado de aplicación | F3 |
| Aprobar un equipo desde otro (envoltura X25519 por dispositivo, código corto en ambas pantallas) | F3 |
| Rotación de la AK, grupos, auditoría en el portal, cuotas por usuario | F4 |
| Métricas Prometheus; GC de blobs huérfanos en Alarik con periodo de gracia; réplica del bucket; backups programados de los volúmenes | F4 (los backups de volúmenes pueden adelantarse: son configuración de Dokploy, no código) |
| `cloud` y `snapshot` en la completion | tarea aparte (oráculo bash + golden) |
| Pantallas de la GUI para `cloud` (P-21) | plan de la GUI |

