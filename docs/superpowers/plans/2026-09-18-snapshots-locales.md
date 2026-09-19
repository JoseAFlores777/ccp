# Snapshots locales de la configuración — plan de implementación

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** que `ccp snapshot` guarde, liste, compare, restaure y exporte un
histórico de **toda** la configuración de Claude de esta máquina: ccp, `~/.claude`,
cada perfil, las ventanas de Desktop y lo local de cada proyecto. Todo en un
almacén local direccionado por contenido, con los secretos sellados. Es la base
sobre la que la nube (plan `2026-09-18-nube-f1-boveda-y-sync.md`) sube y baja
snapshots: el formato es el mismo.

**Architecture:** tres capas, cada una con un solo trabajo.
- `internal/vault`: primitivas criptográficas sin estado (XChaCha20-Poly1305,
  Argon2id).
- `internal/snapshot`: el formato y el almacén (blobs, manifiestos, diff,
  retención, `.ccpsnap`). **No importa `core`**, porque lo compartirá el backend.
- `internal/core/snapshot_layout.go` y `snapshot_restore.go`: el único sitio que
  sabe qué archivo de la máquina corresponde a cada ruta lógica, y cómo vuelve
  cada cosa a su sitio, regenerando después los perfiles.

La CLI (`internal/cli/snapshot.go`) y `ccp serve` solo orquestan y pintan.

**Tech Stack:** Go 1.24 (CI) · `golang.org/x/crypto` (chacha20poly1305, argon2;
dependencia nueva) · stdlib para tar/gzip/json · dispatch a mano, sin cobra.

**Spec:** `docs/superpowers/specs/2026-09-18-config-unificada-snapshots-nube-design.md`
§2 (clases de elemento), §8 (snapshots locales), §11 (portabilidad) y §15
(contrato).

## Global Constraints

- **Commits: nunca autónomos.** El paso «Commit» de cada tarea significa *dejar el
  árbol listo, mostrar el mensaje propuesto y esperar autorización explícita del
  usuario en ese turno*. Es una regla global del usuario y gana sobre este plan.
  Nunca añadir trailers `Co-Authored-By` ni ninguna atribución a herramientas.
- **Gates de CI, todos verdes antes de proponer commit:** `gofmt -l internal cmd`
  no imprime nada · `go vet ./...` · `go test ./...` · `go run
  github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2 run --timeout 5m`.
- **CI compila con Go 1.24** (`GO_VERSION` en `.github/workflows/ci.yml`). Si un
  `go get …@latest` sube la directiva `go` de `go.mod` por encima de `1.24.0`, baja
  la versión de esa dependencia hasta la última que no la suba. No se toca CI en
  este plan.
- **Bilingüe obligatorio.** Todo texto de usuario nuevo va como clave en
  `internal/core/i18n/catalog_snapshot.go` (prefijo `cli.snapshot.`) con `En` y
  `Es`. `TestCatalogComplete` falla si falta uno.
- **`internal/core` e `internal/snapshot` no hacen I/O de presentación.** Devuelven
  datos; formatear es de `internal/cli`.
- **Tests nunca tocan el estado real.** Toda prueba fija con `t.Setenv`:
  - `CCP_HOME` a un temporal (o pasa `home` explícito);
  - `CCP_CLAUDE_SRC` a un temporal;
  - `CCP_DESKTOP_DEFAULT_DATA_DIR` a un temporal: si no, se leería la config real
    de Desktop;
  - `HOME` a un temporal.

  Nada puede leer ni escribir `~/.config/ccp`, `~/.claude`, `~/.claude.json` ni
  `~/Library/Application Support/Claude`.
- **El contrato golden no se toca.** Ninguna tarea modifica `_env`, `_hook`,
  `resolve`, `path test`, `status --json`, `completion bash|zsh` ni
  `completion-shellinit`.
  - `snapshot` **no** se añade a la completion, igual que hoy `backup` y `serve`.
    Añadirlo exige actualizar el oráculo bash y regenerar el golden; es una tarea
    aparte.
  - `internal/golden/parity_test.go` debe seguir verde.
- **`ccp.yaml` no cambia de schema.** Este plan no añade claves a `ccp.yaml`.

## Estructura de archivos

| archivo | responsabilidad |
|---|---|
| `internal/vault/vault.go` **(nuevo)** | `NewKey`, `Seal`/`Open` (XChaCha20-Poly1305), `KDFParams`/`DeriveKey` (Argon2id). |
| `internal/vault/vault_test.go` **(nuevo)** | ida y vuelta, alteraciones, derivación. |
| `internal/snapshot/fsutil.go` **(nuevo)** | `Hash`, `Short`, `validHash`, gzip, escritura atómica. |
| `internal/snapshot/store.go` **(nuevo)** | `Store`: `Open`, clave del almacén, `PutBlob`/`GetBlob`/`HasBlob`. |
| `internal/snapshot/manifest.go` **(nuevo)** | `Class`, `Item`, `Manifest`, id por contenido, `SaveManifest`/`LoadManifest`/`List`/`Resolve`/`SetPin`. |
| `internal/snapshot/capture.go` **(nuevo)** | `Source`, `Meta`, `Capture`, `ItemsOf`, `ErrNoChanges`. |
| `internal/snapshot/diff.go` **(nuevo)** | `Diff`. |
| `internal/snapshot/retention.go` **(nuevo)** | `Policy`, `Retain`, `Prune`. |
| `internal/snapshot/archive.go` **(nuevo)** | `.ccpsnap`: `Export`/`Import`. |
| `internal/snapshot/*_test.go` **(nuevos)** | uno por archivo. |
| `internal/core/claude_json.go` **(nuevo)** | `ClaudeJSONConfig` / `ClaudeJSONApplyConfig`: la parte de configuración de un `.claude.json`. |
| `internal/core/snapshot_layout.go` **(nuevo)** | `SnapshotSources`, `SnapshotCapture`, `SnapshotLive`, identidad de proyectos. |
| `internal/core/snapshot_restore.go` **(nuevo)** | `SnapshotRestore`: plan, snapshot previo, escritura, regeneración. |
| `internal/core/*_test.go` **(nuevos)** | `claude_json_test.go`, `snapshot_layout_test.go`, `snapshot_restore_test.go`. |
| `internal/core/i18n/catalog_snapshot.go` **(nuevo)** | claves `cli.snapshot.*`. |
| `internal/core/i18n/catalog_cli.go` | sección SNAPSHOTS en `cli.help.body` (En y Es). |
| `internal/cli/main_test.go` **(nuevo)** | `TestMain`: apaga el snapshot automático y aísla la ventana `default` de Desktop. |
| `internal/cli/snapshot.go` **(nuevo)** | `ccp snapshot …`, `autoSnapshot`, `maybeDailySnapshot`. |
| `internal/cli/snapshot_test.go` **(nuevo)** | los subcomandos de punta a punta. |
| `internal/cli/cli.go` | `case "snapshot"`, snapshot diario, snapshot antes de `backup restore`. |
| `internal/cli/profile.go` | snapshot antes de `profile rm`. |
| `internal/cli/serve_snapshot.go` **(nuevo)** | métodos `snapshot.*` de `ccp serve`. |
| `internal/cli/serve_methods.go` / `serve_ops.go` | registro; snapshot antes de `profiles.remove` y `backup.restore`. |
| `docs/adr/0012-snapshots-content-addressed.md` **(nuevo)** | la decisión de formato. |
| `README.md`, `README.es.md`, `CHANGELOG.md`, `CLAUDE.md` | documentación. |

---

### Task 1: `internal/vault` — sellado y derivación de claves

Las primitivas criptográficas de ccp en un paquete sin estado. Este plan usa
`Seal`/`Open` (los secretos del almacén y del `.ccpsnap`) y `DeriveKey` (la frase
del `.ccpsnap`). El plan de la nube añade encima envolturas y firmas.

**Files:**
- Create: `internal/vault/vault.go`
- Create: `internal/vault/vault_test.go`
- Modify: `go.mod`, `go.sum` (dependencia `golang.org/x/crypto`)

**Interfaces:**
- Produces: `vault.KeySize`, `vault.ErrOpen`, `vault.NewKey() ([]byte, error)`,
  `vault.Seal(key, plaintext, ad []byte) ([]byte, error)`,
  `vault.Open(key, sealed, ad []byte) ([]byte, error)`,
  `vault.KDFParams{Salt []byte; Time uint32; MemoryKiB uint32; Threads uint8}`,
  `vault.NewKDFParams() (KDFParams, error)`,
  `vault.DeriveKey(passphrase []byte, p KDFParams) ([]byte, error)`.

- [ ] **Step 1: Escribir los tests**

```go
package vault

import (
	"bytes"
	"errors"
	"testing"
)

func mustKey(t *testing.T) []byte {
	t.Helper()
	k, err := NewKey()
	if err != nil {
		t.Fatalf("NewKey: %v", err)
	}
	if len(k) != KeySize {
		t.Fatalf("clave de %d bytes, quiero %d", len(k), KeySize)
	}
	return k
}

func TestSealOpenRoundTrip(t *testing.T) {
	key := mustKey(t)
	sealed, err := Seal(key, []byte("sk-secreto"), []byte("ad"))
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	if bytes.Contains(sealed, []byte("sk-secreto")) {
		t.Fatal("el texto sellado contiene el claro")
	}
	got, err := Open(key, sealed, []byte("ad"))
	if err != nil || string(got) != "sk-secreto" {
		t.Fatalf("Open = %q, %v", got, err)
	}
}

// Dos sellados del mismo texto no se parecen: el nonce es nuevo cada vez.
func TestSealUsesFreshNonce(t *testing.T) {
	key := mustKey(t)
	a, _ := Seal(key, []byte("x"), nil)
	b, _ := Seal(key, []byte("x"), nil)
	if bytes.Equal(a, b) {
		t.Fatal("dos sellados idénticos: el nonce se repite")
	}
}

// Open no distingue por qué falla: clave, datos asociados o bytes alterados
// dan el mismo ErrOpen, para no regalar un oráculo.
func TestOpenRejects(t *testing.T) {
	key := mustKey(t)
	sealed, _ := Seal(key, []byte("hola"), []byte("ad"))
	flipped := bytes.Clone(sealed)
	flipped[len(flipped)-1] ^= 1
	cases := map[string]struct {
		key, data, ad []byte
	}{
		"otra clave":      {mustKey(t), sealed, []byte("ad")},
		"otro ad":         {key, sealed, []byte("otro")},
		"un bit cambiado": {key, flipped, []byte("ad")},
		"truncado":        {key, sealed[:10], []byte("ad")},
	}
	for name, c := range cases {
		if _, err := Open(c.key, c.data, c.ad); !errors.Is(err, ErrOpen) {
			t.Errorf("%s: err = %v, quiero ErrOpen", name, err)
		}
	}
}

func TestDeriveKey(t *testing.T) {
	p := KDFParams{Salt: bytes.Repeat([]byte{7}, 16), Time: 1, MemoryKiB: 8 * 1024, Threads: 1}
	a, err := DeriveKey([]byte("frase larga de prueba"), p)
	if err != nil {
		t.Fatalf("DeriveKey: %v", err)
	}
	b, _ := DeriveKey([]byte("frase larga de prueba"), p)
	if !bytes.Equal(a, b) || len(a) != KeySize {
		t.Fatal("misma frase y parámetros deben dar la misma clave de KeySize bytes")
	}
	other := p
	other.Salt = bytes.Repeat([]byte{8}, 16)
	if c, _ := DeriveKey([]byte("frase larga de prueba"), other); bytes.Equal(a, c) {
		t.Fatal("otra sal debe dar otra clave")
	}
}

// Los parámetros llegan también de archivos importados: los absurdos se
// rechazan, y los carísimos también (una sal corta debilita; 64 GiB de memoria
// tumbaría la máquina de quien importa).
func TestDeriveKeyRejectsBadParams(t *testing.T) {
	good := KDFParams{Salt: bytes.Repeat([]byte{1}, 16), Time: 1, MemoryKiB: 8 * 1024, Threads: 1}
	bad := []KDFParams{
		{Salt: []byte{1}, Time: 1, MemoryKiB: 8 * 1024, Threads: 1},
		{Salt: good.Salt, Time: 0, MemoryKiB: 8 * 1024, Threads: 1},
		{Salt: good.Salt, Time: 1, MemoryKiB: 1024, Threads: 1},
		{Salt: good.Salt, Time: 1, MemoryKiB: 8 * 1024, Threads: 0},
		{Salt: good.Salt, Time: 1, MemoryKiB: 64 * 1024 * 1024, Threads: 1},
		{Salt: good.Salt, Time: 100, MemoryKiB: 8 * 1024, Threads: 1},
	}
	for i, p := range bad {
		if _, err := DeriveKey([]byte("x"), p); err == nil {
			t.Errorf("caso %d: parámetros inválidos aceptados: %+v", i, p)
		}
	}
}

func TestNewKDFParams(t *testing.T) {
	p, err := NewKDFParams()
	if err != nil {
		t.Fatalf("NewKDFParams: %v", err)
	}
	q, _ := NewKDFParams()
	if len(p.Salt) != 16 || bytes.Equal(p.Salt, q.Salt) {
		t.Fatal("cada llamada debe traer una sal nueva de 16 bytes")
	}
}
```

- [ ] **Step 2: Comprobar que falla**

Run: `go test ./internal/vault/`
Expected: FAIL, no compila (`undefined: NewKey`, …).

- [ ] **Step 3: Añadir la dependencia**

Run: `go get golang.org/x/crypto@latest`
Después, `grep '^go ' go.mod` debe seguir diciendo `go 1.24.0`. Si subió, deshaz
(`git checkout go.mod go.sum`) y fija una versión anterior con
`go get golang.org/x/crypto@v0.XX.0`, bajando hasta que no la suba.

- [ ] **Step 4: Implementar `internal/vault/vault.go`**

```go
// Package vault reúne las primitivas criptográficas de ccp: claves aleatorias,
// sellado autenticado y derivación de claves desde una frase. No guarda nada ni
// sabe dónde vive cada clave: eso lo decide quien llama.
package vault

import (
	"crypto/rand"
	"errors"
	"fmt"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/chacha20poly1305"
)

// KeySize es el tamaño de toda clave simétrica de ccp.
const KeySize = chacha20poly1305.KeySize

// ErrOpen es el único error de Open ante datos que no se pueden abrir. No dice
// si fue la clave, los datos asociados o una alteración: distinguirlo le daría
// un oráculo a quien manipula los datos.
var ErrOpen = errors.New("vault: no se pudo abrir (clave equivocada o datos alterados)")

// NewKey devuelve una clave aleatoria de KeySize bytes.
func NewKey() ([]byte, error) {
	k := make([]byte, KeySize)
	if _, err := rand.Read(k); err != nil {
		return nil, fmt.Errorf("vault: sin entropía: %w", err)
	}
	return k, nil
}

// Seal cifra y autentica plaintext con XChaCha20-Poly1305. ad no se cifra pero
// queda autenticado: abrir con otro ad falla, y así un sellado no se puede mover
// a otro contexto. El nonce (24 bytes aleatorios: XChaCha los admite sin riesgo
// práctico de colisión) va delante del texto cifrado.
func Seal(key, plaintext, ad []byte) ([]byte, error) {
	aead, err := chacha20poly1305.NewX(key)
	if err != nil {
		return nil, fmt.Errorf("vault: clave inválida: %w", err)
	}
	nonce := make([]byte, aead.NonceSize(), aead.NonceSize()+len(plaintext)+aead.Overhead())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("vault: sin entropía: %w", err)
	}
	return aead.Seal(nonce, nonce, plaintext, ad), nil
}

// Open deshace Seal. Cualquier fallo de autenticación es ErrOpen.
func Open(key, sealed, ad []byte) ([]byte, error) {
	aead, err := chacha20poly1305.NewX(key)
	if err != nil {
		return nil, fmt.Errorf("vault: clave inválida: %w", err)
	}
	ns := aead.NonceSize()
	if len(sealed) < ns+aead.Overhead() {
		return nil, ErrOpen
	}
	pt, err := aead.Open(nil, sealed[:ns], sealed[ns:], ad)
	if err != nil {
		return nil, ErrOpen
	}
	return pt, nil
}

// KDFParams son los parámetros de Argon2id. Viajan junto a lo que protegen: sin
// ellos no se vuelve a derivar la misma clave.
type KDFParams struct {
	Salt      []byte `json:"salt"`
	Time      uint32 `json:"time"`
	MemoryKiB uint32 `json:"memory_kib"`
	Threads   uint8  `json:"threads"`
}

// Límites de lo que DeriveKey acepta. El mínimo protege la frase; el máximo
// protege a quien importa un archivo ajeno, cuyos parámetros no controla.
const (
	minSaltLen   = 16
	minMemoryKiB = 8 * 1024
	maxMemoryKiB = 4 * 1024 * 1024
	maxTime      = 64
)

// NewKDFParams devuelve los parámetros por defecto con una sal nueva:
// 3 pasadas, 64 MiB, 4 hilos (la recomendación de RFC 9106 para uso interactivo).
func NewKDFParams() (KDFParams, error) {
	salt := make([]byte, minSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return KDFParams{}, fmt.Errorf("vault: sin entropía: %w", err)
	}
	return KDFParams{Salt: salt, Time: 3, MemoryKiB: 64 * 1024, Threads: 4}, nil
}

// DeriveKey deriva una clave de KeySize bytes de passphrase con Argon2id.
func DeriveKey(passphrase []byte, p KDFParams) ([]byte, error) {
	if len(p.Salt) < minSaltLen || p.Time < 1 || p.Time > maxTime ||
		p.MemoryKiB < minMemoryKiB || p.MemoryKiB > maxMemoryKiB || p.Threads < 1 {
		return nil, fmt.Errorf("vault: parámetros de derivación fuera de rango (sal %d bytes, %d pasadas, %d KiB, %d hilos)",
			len(p.Salt), p.Time, p.MemoryKiB, p.Threads)
	}
	return argon2.IDKey(passphrase, p.Salt, p.Time, p.MemoryKiB, p.Threads, KeySize), nil
}
```

- [ ] **Step 5: Comprobar que pasa**

Run: `go test ./internal/vault/ && go vet ./internal/vault/`
Expected: PASS.

- [ ] **Step 6: Gates y commit**

Gates de CI (arriba). Mensaje propuesto:
`feat(vault): sellado XChaCha20-Poly1305 y derivación Argon2id`

---

### Task 2: `internal/snapshot` — el almacén de blobs

Blobs direccionados por contenido: el nombre del blob es el sha256 del contenido
en claro. Los de clase `secret` se guardan sellados con una clave del almacén
(`store.key`, 0600) que nunca sale de la máquina. Cada objeto se describe a sí
mismo con una cabecera, así que para leerlo no hace falta saber desde qué
manifiesto se llegó a él.

**Files:**
- Create: `internal/snapshot/fsutil.go`
- Create: `internal/snapshot/store.go`
- Create: `internal/snapshot/store_test.go`

**Interfaces:**
- Consumes: `vault.Seal`, `vault.Open`, `vault.NewKey`, `vault.KeySize` (Task 1).
- Produces: `snapshot.Open(dir) (*Store, error)`, `(*Store).Dir()`,
  `(*Store).PutBlob(data []byte, secret bool) (string, error)`,
  `(*Store).GetBlob(hash) ([]byte, error)`, `(*Store).HasBlob(hash) bool`,
  `snapshot.Hash([]byte) string`, `snapshot.Short(string) string`,
  `snapshot.ErrNotFound`, `snapshot.ErrCorrupt`, `snapshot.MaxBlobSize`. Internos
  que usan las tareas siguientes: `validHash`, `isHex`, `gz`, `gunzip`,
  `writeAtomic`, `writeTemp`, `(*Store).objPath`, `(*Store).encode`, `readFlag`,
  `(*Store).blobs`, `(*Store).deleteBlob`.

- [ ] **Step 1: Escribir los tests**

```go
package snapshot

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func openTemp(t *testing.T) *Store {
	t.Helper()
	st, err := Open(filepath.Join(t.TempDir(), "snapshots"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return st
}

func TestPutGetBlob(t *testing.T) {
	st := openTemp(t)
	for _, secret := range []bool{false, true} {
		data := []byte(fmt.Sprintf("contenido (secreto=%v)", secret))
		h, err := st.PutBlob(data, secret)
		if err != nil {
			t.Fatalf("PutBlob: %v", err)
		}
		if h != Hash(data) {
			t.Fatalf("hash %s, quiero %s", h, Hash(data))
		}
		got, err := st.GetBlob(h)
		if err != nil || !bytes.Equal(got, data) {
			t.Fatalf("GetBlob = %q, %v", got, err)
		}
		if !st.HasBlob(h) {
			t.Fatal("HasBlob = false tras PutBlob")
		}
	}
}

func TestGetBlobMissing(t *testing.T) {
	st := openTemp(t)
	if _, err := st.GetBlob(Hash([]byte("nunca guardado"))); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, quiero ErrNotFound", err)
	}
}

// Un blob secreto solo lo abre el almacén que tiene la clave: copiado a otro
// almacén es ilegible. Uno normal se lee en cualquiera.
func TestSecretBlobNeedsStoreKey(t *testing.T) {
	a, b := openTemp(t), openTemp(t)
	plain, _ := a.PutBlob([]byte("normal"), false)
	secret, _ := a.PutBlob([]byte("sk-123"), true)
	for _, h := range []string{plain, secret} {
		src, _ := a.objPath(h)
		dst, _ := b.objPath(h)
		data, _ := os.ReadFile(src)
		os.MkdirAll(filepath.Dir(dst), 0o700)
		os.WriteFile(dst, data, 0o600)
	}
	if _, err := b.GetBlob(plain); err != nil {
		t.Fatalf("blob normal en otro almacén: %v", err)
	}
	if _, err := b.GetBlob(secret); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("blob secreto en otro almacén: err = %v, quiero ErrCorrupt", err)
	}
}

// Un contenido que ya estaba en claro y llega como secreto se reescribe sellado.
func TestPutBlobUpgradesToSealed(t *testing.T) {
	st := openTemp(t)
	h, _ := st.PutBlob([]byte("x"), false)
	p, _ := st.objPath(h)
	if f, _ := readFlag(p); f != objPlain {
		t.Fatalf("flag inicial %d, quiero %d", f, objPlain)
	}
	if _, err := st.PutBlob([]byte("x"), true); err != nil {
		t.Fatal(err)
	}
	if f, _ := readFlag(p); f != objSealed {
		t.Fatalf("no se reselló: flag %d", f)
	}
}

// Si el contenido en disco no es el que el hash nombra, GetBlob lo detecta.
func TestGetBlobDetectsTampering(t *testing.T) {
	st := openTemp(t)
	h, _ := st.PutBlob([]byte("original"), false)
	p, _ := st.objPath(h)
	enc, _ := st.encode(h, []byte("alterado"), false)
	os.WriteFile(p, enc, 0o600)
	if _, err := st.GetBlob(h); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("err = %v, quiero ErrCorrupt", err)
	}
}

func TestGetBlobRejectsBadHash(t *testing.T) {
	st := openTemp(t)
	for _, h := range []string{"", "../../etc/passwd", "ABC", strings.Repeat("g", 64), strings.Repeat("A", 64)} {
		if _, err := st.GetBlob(h); err == nil || errors.Is(err, ErrNotFound) {
			t.Errorf("GetBlob(%q) = %v, quiero error de hash inválido", h, err)
		}
	}
}

func TestOpenKeepsKeyAndPermissions(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "snapshots")
	a, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	b, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(a.key, b.key) {
		t.Fatal("la clave cambió al reabrir")
	}
	if fi, _ := os.Stat(filepath.Join(dir, "store.key")); fi.Mode().Perm() != 0o600 {
		t.Fatalf("store.key con permisos %o", fi.Mode().Perm())
	}
	if di, _ := os.Stat(dir); di.Mode().Perm() != 0o700 {
		t.Fatalf("almacén con permisos %o", di.Mode().Perm())
	}
	h, _ := a.PutBlob([]byte("s"), true)
	if _, err := b.GetBlob(h); err != nil {
		t.Fatalf("un segundo Open no abre lo sellado por el primero: %v", err)
	}
}

func TestOpenRejectsBadKeyFile(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "snapshots")
	os.MkdirAll(dir, 0o700)
	os.WriteFile(filepath.Join(dir, "store.key"), []byte("corta"), 0o600)
	if _, err := Open(dir); err == nil {
		t.Fatal("Open aceptó una clave de 5 bytes")
	}
}
```

- [ ] **Step 2: Comprobar que falla**

Run: `go test ./internal/snapshot/`
Expected: FAIL, no compila.

- [ ] **Step 3: Implementar `internal/snapshot/fsutil.go`**

```go
package snapshot

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Hash es el id de contenido de data: sha256 en hexadecimal minúsculo.
func Hash(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// Short abrevia un id a los 12 caracteres con los que se enseña.
func Short(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
}

func isHex(s string) bool {
	for _, c := range s {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

// validHash acepta solo un sha256 en hex minúsculo. Los hashes llegan también
// de manifiestos importados: sin esta comprobación, «../../x» sería una ruta.
func validHash(h string) bool { return len(h) == 64 && isHex(h) }

func gz(data []byte) ([]byte, error) {
	var b bytes.Buffer
	w := gzip.NewWriter(&b)
	if _, err := w.Write(data); err != nil {
		return nil, fmt.Errorf("snapshot: gzip: %w", err)
	}
	if err := w.Close(); err != nil {
		return nil, fmt.Errorf("snapshot: gzip: %w", err)
	}
	return b.Bytes(), nil
}

func gunzip(body []byte) ([]byte, error) {
	r, err := gzip.NewReader(bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	defer r.Close()
	data, err := io.ReadAll(io.LimitReader(r, MaxBlobSize+1))
	if err != nil {
		return nil, err
	}
	if len(data) > MaxBlobSize {
		return nil, fmt.Errorf("snapshot: blob de más de %d bytes", MaxBlobSize)
	}
	return data, nil
}

// writeTemp escribe data en un temporal de dir con permisos perm y devuelve su
// ruta. Los temporales empiezan por «.», así que ningún listado los confunde con
// un blob o un manifiesto.
func writeTemp(dir string, data []byte, perm os.FileMode) (string, error) {
	f, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return "", fmt.Errorf("snapshot: no se pudo crear un temporal en %s: %w", dir, err)
	}
	name := f.Name()
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		_ = os.Remove(name)
		return "", fmt.Errorf("snapshot: no se pudo escribir %s: %w", name, err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(name)
		return "", fmt.Errorf("snapshot: no se pudo cerrar %s: %w", name, err)
	}
	if err := os.Chmod(name, perm); err != nil {
		_ = os.Remove(name)
		return "", fmt.Errorf("snapshot: no se pudo proteger %s: %w", name, err)
	}
	return name, nil
}

// writeAtomic sustituye path de golpe (temporal + rename): quien lee ve el
// archivo viejo o el nuevo, nunca uno a medias.
func writeAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("snapshot: no se pudo crear %s: %w", dir, err)
	}
	tmp, err := writeTemp(dir, data, perm)
	if err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("snapshot: no se pudo escribir %s: %w", path, err)
	}
	return nil
}
```

- [ ] **Step 4: Implementar `internal/snapshot/store.go`**

```go
// Package snapshot es el formato y el almacén de los snapshots de configuración
// de ccp (spec docs/superpowers/specs/2026-09-18-config-unificada-snapshots-nube-design.md §8).
//
// No sabe dónde vive nada en la máquina: recibe fuentes ya resueltas
// (core/snapshot_layout.go) y guarda blobs direccionados por contenido y
// manifiestos que los listan. Lo compartirá el backend de la nube, así que NO
// importa internal/core.
package snapshot

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/JoseAFlores777/ccp/internal/vault"
)

// Cabecera de cada objeto en disco: una marca y un byte que dice si el cuerpo
// va sellado.
const (
	objMagic       = "CCPB1"
	objPlain  byte = 0
	objSealed byte = 1
	// MaxBlobSize acota un blob descomprimido. Frena una bomba gzip dentro de un
	// .ccpsnap ajeno; ningún archivo de configuración se le acerca.
	MaxBlobSize = 1 << 30
)

var (
	// ErrNotFound: el blob o el snapshot pedido no está en este almacén.
	ErrNotFound = errors.New("snapshot: no encontrado")
	// ErrCorrupt: un objeto o un manifiesto no pasa su propia verificación.
	ErrCorrupt = errors.New("snapshot: objeto corrupto")
)

// Store es un almacén de snapshots en un directorio:
//
//	<dir>/store.key               clave que sella los blobs secretos (0600)
//	<dir>/objects/ab/cdef…        blobs; el nombre es el sha256 del contenido
//	<dir>/snaps/<fecha>-<id>.json manifiestos
type Store struct {
	dir string
	key []byte
}

// Open abre el almacén de dir, creándolo (0700) y generando su clave la primera vez.
func Open(dir string) (*Store, error) {
	for _, d := range []string{dir, filepath.Join(dir, "objects"), filepath.Join(dir, "snaps")} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			return nil, fmt.Errorf("snapshot: no se pudo crear %s: %w", d, err)
		}
	}
	// MkdirAll no toca un directorio que ya existía: el 0700 se reafirma.
	if err := os.Chmod(dir, 0o700); err != nil {
		return nil, fmt.Errorf("snapshot: no se pudo proteger %s: %w", dir, err)
	}
	key, err := loadOrCreateKey(filepath.Join(dir, "store.key"))
	if err != nil {
		return nil, err
	}
	return &Store{dir: dir, key: key}, nil
}

// Dir devuelve el directorio del almacén.
func (s *Store) Dir() string { return s.dir }

func loadOrCreateKey(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	switch {
	case err == nil:
		if len(data) != vault.KeySize {
			return nil, fmt.Errorf("snapshot: %s mide %d bytes y debería medir %d", path, len(data), vault.KeySize)
		}
		return data, nil
	case !errors.Is(err, fs.ErrNotExist):
		return nil, fmt.Errorf("snapshot: no se pudo leer %s: %w", path, err)
	}
	key, err := vault.NewKey()
	if err != nil {
		return nil, err
	}
	// Temporal + enlace duro: el enlace es atómico y falla si el destino ya
	// existe. Dos procesos que abren el almacén a la vez acaban con la misma
	// clave, y ninguno lee un archivo a medio escribir.
	tmp, err := writeTemp(filepath.Dir(path), key, 0o600)
	if err != nil {
		return nil, err
	}
	defer os.Remove(tmp)
	if err := os.Link(tmp, path); err != nil {
		if errors.Is(err, fs.ErrExist) {
			return loadOrCreateKey(path)
		}
		return nil, fmt.Errorf("snapshot: no se pudo crear %s: %w", path, err)
	}
	return key, nil
}

func (s *Store) objPath(hash string) (string, error) {
	if !validHash(hash) {
		return "", fmt.Errorf("snapshot: hash inválido %q", hash)
	}
	return filepath.Join(s.dir, "objects", hash[:2], hash[2:]), nil
}

// PutBlob guarda data y devuelve su hash. Si ya estaba no se reescribe, salvo
// que estuviera en claro y ahora llegue como secreto: el mismo contenido no
// puede quedar sin sellar por haber llegado antes por otro camino.
func (s *Store) PutBlob(data []byte, secret bool) (string, error) {
	h := Hash(data)
	p, err := s.objPath(h)
	if err != nil {
		return "", err
	}
	if flag, err := readFlag(p); err == nil && (flag == objSealed || !secret) {
		return h, nil
	}
	enc, err := s.encode(h, data, secret)
	if err != nil {
		return "", err
	}
	if err := writeAtomic(p, enc, 0o600); err != nil {
		return "", err
	}
	return h, nil
}

func (s *Store) encode(hash string, data []byte, secret bool) ([]byte, error) {
	body, err := gz(data)
	if err != nil {
		return nil, err
	}
	flag := objPlain
	if secret {
		// El hash va como dato asociado: un cuerpo sellado copiado bajo otro
		// nombre no abre.
		if body, err = vault.Seal(s.key, body, []byte(hash)); err != nil {
			return nil, err
		}
		flag = objSealed
	}
	out := make([]byte, 0, len(objMagic)+1+len(body))
	out = append(out, objMagic...)
	out = append(out, flag)
	return append(out, body...), nil
}

// readFlag lee solo la cabecera de un objeto.
func readFlag(path string) (byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	hdr := make([]byte, len(objMagic)+1)
	if _, err := io.ReadFull(f, hdr); err != nil {
		return 0, err
	}
	if string(hdr[:len(objMagic)]) != objMagic {
		return 0, ErrCorrupt
	}
	return hdr[len(objMagic)], nil
}

// GetBlob devuelve el contenido del blob hash, verificado: si lo que hay en
// disco no es exactamente lo que ese hash nombra, es ErrCorrupt.
func (s *Store) GetBlob(hash string) ([]byte, error) {
	p, err := s.objPath(hash)
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(p)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("%w: blob %s", ErrNotFound, Short(hash))
	}
	if err != nil {
		return nil, fmt.Errorf("snapshot: no se pudo leer el blob %s: %w", Short(hash), err)
	}
	if len(raw) < len(objMagic)+1 || string(raw[:len(objMagic)]) != objMagic {
		return nil, fmt.Errorf("%w: %s", ErrCorrupt, Short(hash))
	}
	body := raw[len(objMagic)+1:]
	if raw[len(objMagic)] == objSealed {
		if body, err = vault.Open(s.key, body, []byte(hash)); err != nil {
			return nil, fmt.Errorf("%w: %s", ErrCorrupt, Short(hash))
		}
	}
	data, err := gunzip(body)
	if err != nil || Hash(data) != hash {
		return nil, fmt.Errorf("%w: %s", ErrCorrupt, Short(hash))
	}
	return data, nil
}

// HasBlob dice si el blob está en el almacén, sin verificarlo.
func (s *Store) HasBlob(hash string) bool {
	p, err := s.objPath(hash)
	if err != nil {
		return false
	}
	_, err = os.Stat(p)
	return err == nil
}

// blobs lista los blobs del almacén con su fecha de modificación.
func (s *Store) blobs() (map[string]time.Time, error) {
	out := map[string]time.Time{}
	root := filepath.Join(s.dir, "objects")
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || d.Name()[0] == '.' {
			return nil
		}
		h := filepath.Base(filepath.Dir(p)) + d.Name()
		if !validHash(h) {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		out[h] = info.ModTime()
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("snapshot: no se pudieron listar los blobs: %w", err)
	}
	return out, nil
}

func (s *Store) deleteBlob(hash string) error {
	p, err := s.objPath(hash)
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("snapshot: no se pudo borrar el blob %s: %w", Short(hash), err)
	}
	return nil
}
```

- [ ] **Step 5: Comprobar que pasa**

Run: `go test ./internal/snapshot/ && go vet ./internal/snapshot/`
Expected: PASS.

- [ ] **Step 6: Gates y commit**

Mensaje propuesto: `feat(snapshot): almacén de blobs direccionado por contenido`

---

### Task 3: `internal/snapshot` — manifiestos

El manifiesto lista los elementos del snapshot por **ruta lógica**
(`claude/settings.json`, `ccp/profiles/work/overlay/CLAUDE.md`…), que no depende
de la máquina. Su id es el sha256 de su contenido, sin la etiqueta ni el fijado,
que se pueden cambiar después. Un manifiesto alterado en disco no carga.

**Files:**
- Create: `internal/snapshot/manifest.go`
- Create: `internal/snapshot/manifest_test.go`

**Interfaces:**
- Consumes: `Hash`, `Short`, `isHex`, `validHash`, `writeAtomic`, `ErrNotFound`,
  `ErrCorrupt` (Task 2).
- Produces: `snapshot.FormatVersion`, `snapshot.Class` + `ClassAuthored`/`ClassSecret`/`ClassState`,
  `snapshot.Item`, `snapshot.Manifest`,
  `(*Store).SaveManifest(*Manifest) error`, `(*Store).LoadManifest(ref) (*Manifest, error)`,
  `(*Store).Resolve(ref) (string, error)`, `(*Store).List() ([]*Manifest, error)`,
  `(*Store).Latest() (*Manifest, error)`, `(*Store).LatestTime() (time.Time, bool)`,
  `(*Store).DeleteManifest(id) error`,
  `(*Store).SetPin(ref string, pinned bool, label *string) (*Manifest, error)`.
  Internos: `validLPath`, `validClass`, `validate`, `(*Manifest).computeID`.

- [ ] **Step 1: Escribir los tests**

```go
package snapshot

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var t0 = time.Date(2026, 9, 18, 22, 30, 0, 0, time.UTC)

func testManifest(t *testing.T, st *Store, at time.Time) *Manifest {
	t.Helper()
	h1, _ := st.PutBlob([]byte("version: 2\n"), false)
	h2, _ := st.PutBlob([]byte("sk-1"), true)
	return &Manifest{
		Format: FormatVersion, Created: at, Machine: "mac", CCPVersion: "2.18.0", Trigger: "manual",
		Items: []Item{
			{LPath: "ccp/ccp.yaml", Hash: h1, Size: 11, Mode: 0o644, Class: ClassAuthored},
			{LPath: "ccp/profiles/deep/api_key", Hash: h2, Size: 4, Mode: 0o600, Class: ClassSecret},
		},
	}
}

func TestSaveLoadManifest(t *testing.T) {
	st := openTemp(t)
	m := testManifest(t, st, t0)
	if err := st.SaveManifest(m); err != nil {
		t.Fatalf("SaveManifest: %v", err)
	}
	if !validHash(m.ID) {
		t.Fatalf("id %q no es un sha256", m.ID)
	}
	got, err := st.LoadManifest(m.ID[:8])
	if err != nil {
		t.Fatalf("LoadManifest: %v", err)
	}
	a, _ := json.Marshal(m)
	b, _ := json.Marshal(got)
	if string(a) != string(b) {
		t.Fatalf("ida y vuelta distinta:\n%s\n%s", a, b)
	}
}

// Fijar o etiquetar no cambia el id: el id nombra el contenido.
func TestSetPinKeepsID(t *testing.T) {
	st := openTemp(t)
	m := testManifest(t, st, t0)
	st.SaveManifest(m)
	label := "antes de migrar"
	got, err := st.SetPin(m.ID, true, &label)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != m.ID || !got.Pinned || got.Label != label {
		t.Fatalf("SetPin = %+v", got)
	}
	again, _ := st.LoadManifest(m.ID)
	if !again.Pinned || again.Label != label {
		t.Fatal("el fijado no se guardó")
	}
	list, _ := st.List()
	if len(list) != 1 {
		t.Fatalf("SetPin duplicó el manifiesto: %d", len(list))
	}
}

// Un manifiesto cuyo contenido ya no corresponde a su id no carga.
func TestLoadManifestRejectsTampering(t *testing.T) {
	st := openTemp(t)
	m := testManifest(t, st, t0)
	st.SaveManifest(m)
	path := st.manifestPath(m)
	data, _ := os.ReadFile(path)
	data = []byte(strings.Replace(string(data), `"ccp/ccp.yaml"`, `"ccp/otra.yaml"`, 1))
	os.WriteFile(path, data, 0o600)
	if _, err := st.LoadManifest(m.ID); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("err = %v, quiero ErrCorrupt", err)
	}
}

func TestResolveAndList(t *testing.T) {
	st := openTemp(t)
	old := testManifest(t, st, t0)
	st.SaveManifest(old)
	newer := testManifest(t, st, t0.Add(time.Hour))
	st.SaveManifest(newer)

	if id, err := st.Resolve("latest"); err != nil || id != newer.ID {
		t.Fatalf("latest = %s, %v", id, err)
	}
	if id, err := st.Resolve(old.ID); err != nil || id != old.ID {
		t.Fatalf("id completo = %s, %v", id, err)
	}
	for _, bad := range []string{"abc", "zzzz", "../x"} {
		if _, err := st.Resolve(bad); err == nil {
			t.Errorf("Resolve(%q) sin error", bad)
		}
	}
	list, err := st.List()
	if err != nil || len(list) != 2 || list[0].ID != newer.ID {
		t.Fatalf("List = %d elementos (primero %v), %v", len(list), list, err)
	}
	if at, ok := st.LatestTime(); !ok || !at.Equal(newer.Created) {
		t.Fatalf("LatestTime = %v, %v", at, ok)
	}
	if err := st.DeleteManifest(newer.ID); err != nil {
		t.Fatal(err)
	}
	if l, _ := st.Latest(); l.ID != old.ID {
		t.Fatal("tras borrar, Latest debe ser el viejo")
	}
}

func TestLatestEmpty(t *testing.T) {
	st := openTemp(t)
	if _, err := st.Latest(); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, quiero ErrNotFound", err)
	}
	if _, ok := st.LatestTime(); ok {
		t.Fatal("LatestTime en un almacén vacío")
	}
}

func TestSaveManifestRejectsInvalid(t *testing.T) {
	st := openTemp(t)
	good := testManifest(t, st, t0)
	cases := map[string]func(m *Manifest){
		"ruta con ..":   func(m *Manifest) { m.Items[0].LPath = "ccp/../x" },
		"ruta absoluta": func(m *Manifest) { m.Items[0].LPath = "/etc/x" },
		"ruta sucia":    func(m *Manifest) { m.Items[0].LPath = "ccp//x" },
		"desordenado":   func(m *Manifest) { m.Items[0], m.Items[1] = m.Items[1], m.Items[0] },
		"repetido":      func(m *Manifest) { m.Items[1].LPath = m.Items[0].LPath },
		"clase rara":    func(m *Manifest) { m.Items[0].Class = "cache" },
		"hash raro":     func(m *Manifest) { m.Items[0].Hash = "nope" },
		"formato nuevo": func(m *Manifest) { m.Format = FormatVersion + 1 },
		"sin fecha":     func(m *Manifest) { m.Created = time.Time{} },
		"id ajeno":      func(m *Manifest) { m.ID = strings.Repeat("a", 64) },
	}
	for name, mutate := range cases {
		m := *good
		m.Items = append([]Item(nil), good.Items...)
		mutate(&m)
		if err := st.SaveManifest(&m); err == nil {
			t.Errorf("%s: SaveManifest aceptó un manifiesto inválido", name)
		}
	}
	if files, _ := os.ReadDir(filepath.Join(st.Dir(), "snaps")); len(files) != 0 {
		t.Fatalf("un manifiesto inválido llegó a disco: %d archivos", len(files))
	}
}
```

- [ ] **Step 2: Comprobar que falla**

Run: `go test ./internal/snapshot/ -run 'Manifest|Resolve|Latest|SetPin'`
Expected: FAIL, no compila.

- [ ] **Step 3: Implementar `internal/snapshot/manifest.go`**

```go
package snapshot

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// FormatVersion es la versión del formato de manifiesto que este binario escribe y lee.
const FormatVersion = 1

// Class es la clase de un elemento (spec §2). Decide si se sella y si viaja.
// Lo generado, la caché y lo atado a la máquina no tienen clase: no se capturan.
type Class string

const (
	ClassAuthored Class = "authored" // configuración que escribe el usuario
	ClassSecret   Class = "secret"   // claves y tokens: siempre sellados
	ClassState    Class = "state"    // conversaciones y préstamos: solo si se piden
)

func validClass(c Class) bool { return c == ClassAuthored || c == ClassSecret || c == ClassState }

// Item es un archivo del snapshot, nombrado por su ruta lógica.
type Item struct {
	LPath string            `json:"lpath"`
	Hash  string            `json:"hash"`
	Size  int64             `json:"size"`
	Mode  uint32            `json:"mode"`
	Class Class             `json:"class"`
	Meta  map[string]string `json:"meta,omitempty"`
}

// Manifest describe un snapshot. El id es el sha256 de su contenido: dos
// capturas iguales tienen el mismo id y ningún manifiesto se puede editar sin
// que se note.
type Manifest struct {
	Format     int       `json:"format"`
	ID         string    `json:"id"`
	Parent     string    `json:"parent,omitempty"`
	Created    time.Time `json:"created"`
	Machine    string    `json:"machine"`
	// Home es el HOME de la máquina de origen. El plan de la nube lo usa para
	// traducir rutas absolutas (reglas de carpeta, proyectos) al restaurar en
	// otra máquina. omitempty: un manifiesto sin él conserva su id.
	Home       string    `json:"home,omitempty"`
	CCPVersion string    `json:"ccp_version"`
	Trigger    string    `json:"trigger"`
	Items      []Item    `json:"items"`
	// Label y Pinned se pueden cambiar después de capturar, por eso no entran
	// en el id.
	Label  string `json:"label,omitempty"`
	Pinned bool   `json:"pinned,omitempty"`
}

// snapTimeLayout lleva nanosegundos de ancho fijo: el orden alfabético de los
// nombres es el cronológico incluso entre capturas del mismo segundo (una
// captura y el snapshot de seguridad de un restore que la sigue al instante).
// Con segundos, «latest» y el padre de la cadena quedarían al azar.
const snapTimeLayout = "20060102T150405.000000000Z"

var snapNameRe = regexp.MustCompile(`^(\d{8}T\d{6}\.\d{9}Z)-([0-9a-f]{64})\.json$`)

func (m *Manifest) computeID() (string, error) {
	c := *m
	c.ID, c.Label, c.Pinned = "", "", false
	data, err := json.Marshal(&c)
	if err != nil {
		return "", fmt.Errorf("snapshot: no se pudo serializar el manifiesto: %w", err)
	}
	return Hash(data), nil
}

// validLPath: ruta relativa con «/», limpia, sin «..» ni componentes vacíos.
// Llega también de archivos importados, así que es la primera barrera contra una
// ruta que se salga de donde debe.
func validLPath(l string) bool {
	if l == "" || strings.HasPrefix(l, "/") || strings.Contains(l, `\`) || path.Clean(l) != l {
		return false
	}
	for _, part := range strings.Split(l, "/") {
		if part == "" || part == "." || part == ".." {
			return false
		}
	}
	return true
}

func validate(m *Manifest) error {
	if m.Format != FormatVersion {
		return fmt.Errorf("snapshot: formato %d no soportado (este ccp conoce el %d); actualiza ccp", m.Format, FormatVersion)
	}
	if m.Created.IsZero() {
		return errors.New("snapshot: manifiesto sin fecha")
	}
	for i, it := range m.Items {
		if !validLPath(it.LPath) {
			return fmt.Errorf("snapshot: ruta lógica inválida %q", it.LPath)
		}
		if i > 0 && m.Items[i-1].LPath >= it.LPath {
			return fmt.Errorf("snapshot: elementos desordenados o repetidos en %q", it.LPath)
		}
		if !validHash(it.Hash) {
			return fmt.Errorf("snapshot: hash inválido en %q", it.LPath)
		}
		if !validClass(it.Class) {
			return fmt.Errorf("snapshot: clase %q desconocida en %q", it.Class, it.LPath)
		}
	}
	return nil
}

func (s *Store) manifestPath(m *Manifest) string {
	return filepath.Join(s.dir, "snaps", m.Created.UTC().Format(snapTimeLayout)+"-"+m.ID+".json")
}

// SaveManifest valida y escribe m. Si m.ID está vacío lo calcula; si viene
// relleno (un manifiesto importado o re-etiquetado) tiene que corresponder a su
// contenido.
func (s *Store) SaveManifest(m *Manifest) error {
	if err := validate(m); err != nil {
		return err
	}
	id, err := m.computeID()
	if err != nil {
		return err
	}
	switch {
	case m.ID == "":
		m.ID = id
	case m.ID != id:
		return fmt.Errorf("snapshot: el id %s no corresponde a su contenido", Short(m.ID))
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("snapshot: no se pudo serializar el manifiesto: %w", err)
	}
	return writeAtomic(s.manifestPath(m), append(data, '\n'), 0o600)
}

type snapEntry struct {
	name string
	id   string
	at   time.Time
}

// entries lista los manifiestos por nombre, del más nuevo al más viejo.
func (s *Store) entries() ([]snapEntry, error) {
	des, err := os.ReadDir(filepath.Join(s.dir, "snaps"))
	if err != nil {
		return nil, fmt.Errorf("snapshot: no se pudo listar el almacén: %w", err)
	}
	var out []snapEntry
	for _, de := range des {
		mm := snapNameRe.FindStringSubmatch(de.Name())
		if mm == nil {
			continue
		}
		at, err := time.Parse(snapTimeLayout, mm[1])
		if err != nil {
			continue
		}
		out = append(out, snapEntry{name: de.Name(), id: mm[2], at: at})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].name > out[j].name })
	return out, nil
}

// Resolve traduce una referencia —«latest», un id completo o un prefijo de al
// menos 4 caracteres hexadecimales— al id completo.
func (s *Store) Resolve(ref string) (string, error) {
	es, err := s.entries()
	if err != nil {
		return "", err
	}
	if ref == "latest" {
		if len(es) == 0 {
			return "", fmt.Errorf("%w: el almacén está vacío", ErrNotFound)
		}
		return es[0].id, nil
	}
	if len(ref) < 4 || !isHex(ref) {
		return "", fmt.Errorf("snapshot: %q no es un id (usa «latest» o al menos 4 caracteres hexadecimales)", ref)
	}
	var found []string
	for _, e := range es {
		if strings.HasPrefix(e.id, ref) {
			found = append(found, e.id)
		}
	}
	switch len(found) {
	case 0:
		return "", fmt.Errorf("%w: ningún snapshot empieza por %s", ErrNotFound, ref)
	case 1:
		return found[0], nil
	default:
		return "", fmt.Errorf("snapshot: %s es ambiguo (%d snapshots); añade más caracteres", ref, len(found))
	}
}

// LoadManifest carga el manifiesto de ref y comprueba que corresponde a su id.
func (s *Store) LoadManifest(ref string) (*Manifest, error) {
	id, err := s.Resolve(ref)
	if err != nil {
		return nil, err
	}
	es, err := s.entries()
	if err != nil {
		return nil, err
	}
	for _, e := range es {
		if e.id == id {
			return s.readManifest(e.name)
		}
	}
	return nil, fmt.Errorf("%w: %s", ErrNotFound, Short(id))
}

func (s *Store) readManifest(name string) (*Manifest, error) {
	data, err := os.ReadFile(filepath.Join(s.dir, "snaps", name))
	if err != nil {
		return nil, fmt.Errorf("snapshot: no se pudo leer %s: %w", name, err)
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("%w: %s no es un manifiesto válido: %v", ErrCorrupt, name, err)
	}
	if err := validate(&m); err != nil {
		return nil, fmt.Errorf("%w: %s: %v", ErrCorrupt, name, err)
	}
	id, err := m.computeID()
	if err != nil {
		return nil, err
	}
	if id != m.ID || !strings.Contains(name, m.ID) {
		return nil, fmt.Errorf("%w: el manifiesto %s no corresponde a su id", ErrCorrupt, name)
	}
	return &m, nil
}

// List devuelve todos los manifiestos, del más nuevo al más viejo. Uno dañado
// hace fallar la lista entera a propósito: prune y restore trabajan sobre ella,
// y decidir sobre una lista incompleta podría borrar o pisar lo que no toca.
func (s *Store) List() ([]*Manifest, error) {
	es, err := s.entries()
	if err != nil {
		return nil, err
	}
	out := make([]*Manifest, 0, len(es))
	for _, e := range es {
		m, err := s.readManifest(e.name)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, nil
}

// Latest devuelve el manifiesto más reciente, o ErrNotFound si no hay ninguno.
func (s *Store) Latest() (*Manifest, error) {
	es, err := s.entries()
	if err != nil {
		return nil, err
	}
	if len(es) == 0 {
		return nil, fmt.Errorf("%w: el almacén está vacío", ErrNotFound)
	}
	return s.readManifest(es[0].name)
}

// LatestTime da la fecha del snapshot más reciente leyendo solo nombres de
// archivo: es lo que consulta el snapshot diario, en cada comando.
func (s *Store) LatestTime() (time.Time, bool) {
	es, err := s.entries()
	if err != nil || len(es) == 0 {
		return time.Time{}, false
	}
	return es[0].at, true
}

// DeleteManifest borra el manifiesto id. Sus blobs los recoge Prune.
func (s *Store) DeleteManifest(id string) error {
	es, err := s.entries()
	if err != nil {
		return err
	}
	for _, e := range es {
		if e.id == id {
			if err := os.Remove(filepath.Join(s.dir, "snaps", e.name)); err != nil {
				return fmt.Errorf("snapshot: no se pudo borrar %s: %w", Short(id), err)
			}
			return nil
		}
	}
	return fmt.Errorf("%w: %s", ErrNotFound, Short(id))
}

// SetPin fija o suelta un snapshot y, si label no es nil, cambia su etiqueta.
// Ni lo uno ni lo otro cambia el id.
func (s *Store) SetPin(ref string, pinned bool, label *string) (*Manifest, error) {
	m, err := s.LoadManifest(ref)
	if err != nil {
		return nil, err
	}
	m.Pinned = pinned
	if label != nil {
		m.Label = *label
	}
	if err := s.SaveManifest(m); err != nil {
		return nil, err
	}
	return m, nil
}
```

- [ ] **Step 4: Comprobar que pasa**

Run: `go test ./internal/snapshot/ && go vet ./internal/snapshot/`
Expected: PASS.

- [ ] **Step 5: Gates y commit**

Mensaje propuesto: `feat(snapshot): manifiestos con id por contenido`

---

### Task 4: `internal/snapshot` — capturar y comparar

`Capture` convierte fuentes en blobs y un manifiesto encadenado al anterior.
Si nada cambió desde el último, no crea otro, para que los disparadores
automáticos no llenen el historial de copias iguales. `ItemsOf` es la misma
cuenta sin guardar nada: la «foto» del estado vivo contra la que comparan
`diff` y `restore`.

**Files:**
- Create: `internal/snapshot/capture.go`
- Create: `internal/snapshot/diff.go`
- Create: `internal/snapshot/capture_test.go`

**Interfaces:**
- Consumes: `(*Store).PutBlob`, `(*Store).Latest`, `(*Store).SaveManifest`, `validLPath`, `validClass` (Tasks 2-3).
- Produces: `snapshot.Source{LPath, Class, Mode, Meta, Read}`, `snapshot.Meta{Created, Machine, Home, CCPVersion, Trigger, Label}`,
  `snapshot.ErrNoChanges`, `snapshot.Capture(st, srcs, meta, force) (*Manifest, error)`,
  `snapshot.ItemsOf(srcs) ([]Item, error)`, `snapshot.ChangeKind` + `ChangeAdded`/`ChangeRemoved`/`ChangeModified`,
  `snapshot.Change{LPath, Kind, From, To}`, `snapshot.Diff(from, to []Item) []Change`.

- [ ] **Step 1: Escribir los tests**

```go
package snapshot

import (
	"errors"
	"io/fs"
	"reflect"
	"testing"
	"time"
)

func src(lpath string, class Class, data string) Source {
	return Source{LPath: lpath, Class: class, Mode: 0o644, Read: func() ([]byte, error) { return []byte(data), nil }}
}

func meta(at time.Time) Meta {
	return Meta{Created: at, Machine: "mac", CCPVersion: "2.18.0", Trigger: "manual"}
}

func TestCaptureChainsAndDedups(t *testing.T) {
	st := openTemp(t)
	srcs := []Source{src("claude/settings.json", ClassAuthored, `{"a":1}`), src("ccp/ccp.yaml", ClassAuthored, "version: 2\n")}

	m1, err := Capture(st, srcs, meta(t0), false)
	if err != nil {
		t.Fatalf("primera captura: %v", err)
	}
	if m1.Parent != "" || len(m1.Items) != 2 || m1.Items[0].LPath != "ccp/ccp.yaml" {
		t.Fatalf("m1 = %+v", m1)
	}

	same, err := Capture(st, srcs, meta(t0.Add(time.Hour)), false)
	if !errors.Is(err, ErrNoChanges) || same.ID != m1.ID {
		t.Fatalf("captura sin cambios = %v, %v; quiero el mismo snapshot y ErrNoChanges", same, err)
	}

	srcs[0] = src("claude/settings.json", ClassAuthored, `{"a":2}`)
	m2, err := Capture(st, srcs, meta(t0.Add(2*time.Hour)), false)
	if err != nil || m2.Parent != m1.ID {
		t.Fatalf("m2 = %+v, %v; quiero padre %s", m2, err, m1.ID)
	}

	forced, err := Capture(st, srcs, meta(t0.Add(3*time.Hour)), true)
	if err != nil || forced.ID == m2.ID || forced.Parent != m2.ID {
		t.Fatalf("captura forzada = %+v, %v", forced, err)
	}
}

// Un archivo que desaparece entre listarlo y leerlo no entra, y no rompe la captura.
func TestCaptureSkipsVanishedFiles(t *testing.T) {
	st := openTemp(t)
	gone := Source{LPath: "claude/agents/x.md", Class: ClassAuthored, Read: func() ([]byte, error) { return nil, fs.ErrNotExist }}
	m, err := Capture(st, []Source{src("ccp/ccp.yaml", ClassAuthored, "v"), gone}, meta(t0), false)
	if err != nil || len(m.Items) != 1 {
		t.Fatalf("m = %+v, %v", m, err)
	}
}

func TestCaptureRejectsBadSources(t *testing.T) {
	st := openTemp(t)
	bad := map[string][]Source{
		"repetida":  {src("a", ClassAuthored, "1"), src("a", ClassAuthored, "2")},
		"con ..":    {src("../a", ClassAuthored, "1")},
		"clase":     {src("a", "cache", "1")},
		"sin nada":  {},
		"otro fallo": {{LPath: "a", Class: ClassAuthored, Read: func() ([]byte, error) { return nil, errors.New("disco") }}},
	}
	for name, srcs := range bad {
		if _, err := Capture(st, srcs, meta(t0), false); err == nil {
			t.Errorf("%s: Capture sin error", name)
		}
	}
}

func TestItemsOfMatchesCapture(t *testing.T) {
	st := openTemp(t)
	srcs := []Source{src("b", ClassSecret, "sk"), src("a", ClassAuthored, "x")}
	m, _ := Capture(st, srcs, meta(t0), false)
	items, err := ItemsOf(srcs)
	if err != nil || !reflect.DeepEqual(items, m.Items) {
		t.Fatalf("ItemsOf = %+v, %v; quiero %+v", items, err, m.Items)
	}
}

func TestDiff(t *testing.T) {
	a := []Item{{LPath: "keep", Hash: "1"}, {LPath: "mod", Hash: "2"}, {LPath: "old", Hash: "3"}, {LPath: "perm", Hash: "4", Mode: 0o644}}
	b := []Item{{LPath: "keep", Hash: "1"}, {LPath: "mod", Hash: "9"}, {LPath: "new", Hash: "5"}, {LPath: "perm", Hash: "4", Mode: 0o600}}
	got := Diff(a, b)
	want := []struct {
		l string
		k ChangeKind
	}{{"mod", ChangeModified}, {"new", ChangeAdded}, {"old", ChangeRemoved}, {"perm", ChangeModified}}
	if len(got) != len(want) {
		t.Fatalf("Diff = %+v", got)
	}
	for i, w := range want {
		if got[i].LPath != w.l || got[i].Kind != w.k {
			t.Errorf("cambio %d = %s %s, quiero %s %s", i, got[i].Kind, got[i].LPath, w.k, w.l)
		}
	}
	if len(Diff(a, a)) != 0 {
		t.Fatal("Diff de algo consigo mismo no es vacío")
	}
}
```

- [ ] **Step 2: Comprobar que falla**

Run: `go test ./internal/snapshot/ -run 'Capture|ItemsOf|Diff'`
Expected: FAIL, no compila.

- [ ] **Step 3: Implementar `internal/snapshot/capture.go`**

```go
package snapshot

import (
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"sort"
	"time"
)

// Source es un archivo por capturar, ya resuelto por quien conoce el disco.
// Read se llama una sola vez, al capturar: la memoria máxima es la del archivo
// más grande, no la de todos. Si devuelve fs.ErrNotExist, el archivo desapareció
// entre listarlo y leerlo y simplemente no entra.
type Source struct {
	LPath string
	Class Class
	Mode  uint32
	Meta  map[string]string
	Read  func() ([]byte, error)
}

// Meta son los datos del snapshot que no salen de los archivos.
type Meta struct {
	Created    time.Time
	Machine    string
	Home       string
	CCPVersion string
	Trigger    string
	Label      string
}

// ErrNoChanges: la captura sería idéntica al último snapshot. Capture devuelve
// ese último junto con este error.
var ErrNoChanges = errors.New("snapshot: sin cambios desde el último")

// Capture guarda cada fuente como blob y escribe un manifiesto cuyo padre es el
// último snapshot. Sin force, si los elementos son los mismos que los del último,
// no escribe nada y devuelve ese último con ErrNoChanges.
func Capture(st *Store, srcs []Source, meta Meta, force bool) (*Manifest, error) {
	items, err := collect(srcs, st.PutBlob)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, errors.New("snapshot: no hay nada que capturar")
	}
	latest, err := st.Latest()
	if err != nil && !errors.Is(err, ErrNotFound) {
		return nil, err
	}
	if latest != nil && !force && sameItems(latest.Items, items) {
		return latest, ErrNoChanges
	}
	m := &Manifest{
		Format:     FormatVersion,
		Created:    meta.Created.UTC(),
		Machine:    meta.Machine,
		Home:       meta.Home,
		CCPVersion: meta.CCPVersion,
		Trigger:    meta.Trigger,
		Label:      meta.Label,
		Items:      items,
	}
	if latest != nil {
		m.Parent = latest.ID
	}
	if err := st.SaveManifest(m); err != nil {
		return nil, err
	}
	return m, nil
}

// ItemsOf calcula los elementos de unas fuentes sin guardar nada.
func ItemsOf(srcs []Source) ([]Item, error) {
	return collect(srcs, func(data []byte, _ bool) (string, error) { return Hash(data), nil })
}

func collect(srcs []Source, put func([]byte, bool) (string, error)) ([]Item, error) {
	seen := make(map[string]bool, len(srcs))
	items := make([]Item, 0, len(srcs))
	for _, s := range srcs {
		if !validLPath(s.LPath) {
			return nil, fmt.Errorf("snapshot: ruta lógica inválida %q", s.LPath)
		}
		if seen[s.LPath] {
			return nil, fmt.Errorf("snapshot: ruta lógica repetida %q", s.LPath)
		}
		seen[s.LPath] = true
		if !validClass(s.Class) {
			return nil, fmt.Errorf("snapshot: clase %q desconocida en %q", s.Class, s.LPath)
		}
		data, err := s.Read()
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("snapshot: no se pudo leer %s: %w", s.LPath, err)
		}
		h, err := put(data, s.Class == ClassSecret)
		if err != nil {
			return nil, err
		}
		it := Item{LPath: s.LPath, Hash: h, Size: int64(len(data)), Mode: s.Mode, Class: s.Class}
		if len(s.Meta) > 0 {
			it.Meta = maps.Clone(s.Meta)
		}
		items = append(items, it)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].LPath < items[j].LPath })
	return items, nil
}

func sameItems(a, b []Item) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		x, y := a[i], b[i]
		if x.LPath != y.LPath || x.Hash != y.Hash || x.Mode != y.Mode || x.Class != y.Class || !maps.Equal(x.Meta, y.Meta) {
			return false
		}
	}
	return true
}
```

- [ ] **Step 4: Implementar `internal/snapshot/diff.go`**

```go
package snapshot

import "sort"

// ChangeKind es el tipo de cambio de un elemento entre dos estados.
type ChangeKind string

const (
	ChangeAdded    ChangeKind = "added"
	ChangeRemoved  ChangeKind = "removed"
	ChangeModified ChangeKind = "modified"
)

// Change es un elemento que cambia entre dos estados.
type Change struct {
	LPath string     `json:"lpath"`
	Kind  ChangeKind `json:"kind"`
	From  *Item      `json:"from,omitempty"`
	To    *Item      `json:"to,omitempty"`
}

// Diff dice qué cambia para ir de from a to, por ruta lógica y en orden.
// Un cambio de permisos o de clase cuenta como modificación: restaurar lo deshace.
func Diff(from, to []Item) []Change {
	a := make(map[string]Item, len(from))
	for _, it := range from {
		a[it.LPath] = it
	}
	b := make(map[string]Item, len(to))
	for _, it := range to {
		b[it.LPath] = it
	}
	var out []Change
	for l, x := range a {
		y, ok := b[l]
		switch {
		case !ok:
			out = append(out, Change{LPath: l, Kind: ChangeRemoved, From: &x})
		case x.Hash != y.Hash || x.Mode != y.Mode || x.Class != y.Class:
			out = append(out, Change{LPath: l, Kind: ChangeModified, From: &x, To: &y})
		}
	}
	for l, y := range b {
		if _, ok := a[l]; !ok {
			out = append(out, Change{LPath: l, Kind: ChangeAdded, To: &y})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].LPath < out[j].LPath })
	return out
}
```

- [ ] **Step 5: Comprobar que pasa**

Run: `go test ./internal/snapshot/ && go vet ./internal/snapshot/`
Expected: PASS.

- [ ] **Step 6: Gates y commit**

Mensaje propuesto: `feat(snapshot): captura encadenada, dedup y diff`

---

### Task 5: `internal/snapshot` — retención y poda

Retención al estilo restic (spec §8.3): del más nuevo al más viejo, se conserva
el último snapshot de cada uno de los últimos N días, N semanas ISO y N meses
**que tengan snapshots**. Además, siempre el más reciente y todo lo fijado o
etiquetado. `Prune` borra el resto y después los blobs que ya nadie usa, salvo
los recién escritos: pueden ser de una captura en curso que aún no escribió su
manifiesto.

**Files:**
- Create: `internal/snapshot/retention.go`
- Create: `internal/snapshot/retention_test.go`

**Interfaces:**
- Consumes: `(*Store).List`, `(*Store).DeleteManifest`, `(*Store).blobs`, `(*Store).deleteBlob` (Tasks 2-3).
- Produces: `snapshot.Policy{Daily, Weekly, Monthly int}`, `snapshot.DefaultPolicy`,
  `snapshot.Retain(ms []*Manifest, p Policy) map[string]bool`,
  `snapshot.PruneReport{Deleted []string; Kept, BlobsDeleted int}`,
  `snapshot.Prune(st, p, now time.Time, grace time.Duration, dryRun bool) (PruneReport, error)`.

- [ ] **Step 1: Escribir los tests**

```go
package snapshot

import (
	"sort"
	"testing"
	"time"
)

func at(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}

// Semanas ISO de 2026: el 17 y el 18 de septiembre caen en la 38 y el 10 en la 37.
func TestRetain(t *testing.T) {
	ms := []*Manifest{
		{ID: "A", Created: at("2026-09-18T18:00:00Z")},
		{ID: "B", Created: at("2026-09-18T10:00:00Z")},
		{ID: "C", Created: at("2026-09-17T12:00:00Z")},
		{ID: "D", Created: at("2026-09-10T12:00:00Z")},
		{ID: "E", Created: at("2026-08-01T12:00:00Z")},
		{ID: "F", Created: at("2026-01-01T12:00:00Z"), Pinned: true},
		{ID: "G", Created: at("2025-12-01T12:00:00Z"), Label: "antes de migrar"},
		{ID: "H", Created: at("2025-11-01T12:00:00Z")},
	}
	keep := Retain(ms, Policy{Daily: 2, Weekly: 2, Monthly: 2})
	var got []string
	for id := range keep {
		got = append(got, id)
	}
	sort.Strings(got)
	want := "A C D E F G"
	if s := join(got); s != want {
		t.Fatalf("conservados = %s, quiero %s", s, want)
	}
}

// Con la política a cero sigue conservándose el último: podar nunca deja el
// historial vacío.
func TestRetainAlwaysKeepsLatest(t *testing.T) {
	ms := []*Manifest{{ID: "viejo", Created: at("2026-01-01T00:00:00Z")}, {ID: "nuevo", Created: at("2026-09-01T00:00:00Z")}}
	keep := Retain(ms, Policy{})
	if !keep["nuevo"] || keep["viejo"] {
		t.Fatalf("keep = %v", keep)
	}
}

func join(s []string) string {
	out := ""
	for i, x := range s {
		if i > 0 {
			out += " "
		}
		out += x
	}
	return out
}

func TestPrune(t *testing.T) {
	st := openTemp(t)
	var ms []*Manifest
	for i, day := range []string{"2026-09-16T12:00:00Z", "2026-09-17T12:00:00Z", "2026-09-18T12:00:00Z"} {
		m, err := Capture(st, []Source{
			src("ccp/ccp.yaml", ClassAuthored, "comun"),
			src("claude/settings.json", ClassAuthored, string(rune('a'+i))),
		}, meta(at(day)), false)
		if err != nil {
			t.Fatal(err)
		}
		ms = append(ms, m)
	}
	latest := ms[2]

	// dry-run: informa pero no toca nada.
	rep, err := Prune(st, Policy{Daily: 1}, time.Now(), 0, true)
	if err != nil || len(rep.Deleted) != 2 || rep.Kept != 1 {
		t.Fatalf("dry-run = %+v, %v", rep, err)
	}
	if list, _ := st.List(); len(list) != 3 {
		t.Fatal("el dry-run borró manifiestos")
	}

	// Con periodo de gracia, los blobs recién escritos no se tocan…
	rep, err = Prune(st, Policy{Daily: 1}, time.Now(), time.Hour, false)
	if err != nil || len(rep.Deleted) != 2 || rep.BlobsDeleted != 0 {
		t.Fatalf("prune con gracia = %+v, %v", rep, err)
	}
	// …y pasado el periodo, se recogen los que ya no usa nadie.
	rep, err = Prune(st, Policy{Daily: 1}, time.Now().Add(2*time.Hour), time.Hour, false)
	if err != nil || rep.BlobsDeleted != 2 {
		t.Fatalf("prune tras la gracia = %+v, %v", rep, err)
	}
	for _, it := range latest.Items {
		if _, err := st.GetBlob(it.Hash); err != nil {
			t.Fatalf("se borró un blob del snapshot conservado (%s): %v", it.LPath, err)
		}
	}
}
```

- [ ] **Step 2: Comprobar que falla**

Run: `go test ./internal/snapshot/ -run 'Retain|Prune'`
Expected: FAIL, no compila.

- [ ] **Step 3: Implementar `internal/snapshot/retention.go`**

```go
package snapshot

import (
	"fmt"
	"sort"
	"time"
)

// Policy dice cuántos periodos de cada tipo conservar.
type Policy struct {
	Daily   int
	Weekly  int
	Monthly int
}

// DefaultPolicy: 7 días, 4 semanas y 6 meses (spec §8.3).
var DefaultPolicy = Policy{Daily: 7, Weekly: 4, Monthly: 6}

// Retain devuelve los ids que se conservan. Recorre del más nuevo al más viejo
// y, por cada tipo de periodo, se queda con el primero (el más reciente) de cada
// uno de los últimos N periodos que tengan snapshots: una semana sin snapshots
// no gasta un hueco. Siempre se conserva el más reciente y todo lo fijado o
// etiquetado.
func Retain(ms []*Manifest, p Policy) map[string]bool {
	sorted := append([]*Manifest(nil), ms...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Created.After(sorted[j].Created) })
	keep := map[string]bool{}
	if len(sorted) > 0 {
		keep[sorted[0].ID] = true
	}
	for _, m := range sorted {
		if m.Pinned || m.Label != "" {
			keep[m.ID] = true
		}
	}
	bucket := func(n int, key func(time.Time) string) {
		last, count := "", 0
		for _, m := range sorted {
			if count >= n {
				return
			}
			k := key(m.Created.UTC())
			if k == last {
				continue
			}
			keep[m.ID] = true
			last = k
			count++
		}
	}
	bucket(p.Daily, func(t time.Time) string { return t.Format("2006-01-02") })
	bucket(p.Weekly, func(t time.Time) string { y, w := t.ISOWeek(); return fmt.Sprintf("%d-W%02d", y, w) })
	bucket(p.Monthly, func(t time.Time) string { return t.Format("2006-01") })
	return keep
}

// PruneReport resume una poda.
type PruneReport struct {
	Deleted      []string
	Kept         int
	BlobsDeleted int
}

// Prune borra los manifiestos que Retain no conserva y después los blobs que ya
// no referencia ninguno de los que quedan. Un blob modificado hace menos de
// grace no se toca aunque parezca huérfano: puede ser de una captura en curso
// que aún no escribió su manifiesto.
func Prune(st *Store, p Policy, now time.Time, grace time.Duration, dryRun bool) (PruneReport, error) {
	var rep PruneReport
	ms, err := st.List()
	if err != nil {
		return rep, err
	}
	keep := Retain(ms, p)
	live := map[string]bool{}
	for _, m := range ms {
		if keep[m.ID] {
			rep.Kept++
			for _, it := range m.Items {
				live[it.Hash] = true
			}
			continue
		}
		rep.Deleted = append(rep.Deleted, m.ID)
		if !dryRun {
			if err := st.DeleteManifest(m.ID); err != nil {
				return rep, err
			}
		}
	}
	blobs, err := st.blobs()
	if err != nil {
		return rep, err
	}
	for h, mtime := range blobs {
		if live[h] || now.Sub(mtime) < grace {
			continue
		}
		rep.BlobsDeleted++
		if !dryRun {
			if err := st.deleteBlob(h); err != nil {
				return rep, err
			}
		}
	}
	return rep, nil
}
```

- [ ] **Step 4: Comprobar que pasa**

Run: `go test ./internal/snapshot/ && go vet ./internal/snapshot/`
Expected: PASS. En `TestPrune`, los dos blobs recogidos son los `settings.json`
«a» y «b»; `ccp.yaml` («comun») lo sigue usando el snapshot conservado.

- [ ] **Step 5: Gates y commit**

Mensaje propuesto: `feat(snapshot): retención por periodos y poda de blobs huérfanos`

---

### Task 6: `internal/snapshot` — archivos `.ccpsnap`

Un `.ccpsnap` es un snapshot portable en un solo archivo, un tar.gz con este
orden:
1. `ccpsnap.json`: cabecera.
2. `manifest.json`.
3. `objects/<hash>`: los blobs.

Sin frase, los blobs que solo usan elementos secretos **no se incluyen**. Con
frase, van sellados con una clave derivada de ella (Argon2id), cuyos parámetros
viajan en la cabecera. Al importar, todo se verifica antes de guardar el
manifiesto:
- que el id corresponda al contenido;
- que cada objeto esté nombrado en el manifiesto y corresponda a su hash;
- que los secretos abran con la frase.

**Files:**
- Create: `internal/snapshot/archive.go`
- Create: `internal/snapshot/archive_test.go`

**Interfaces:**
- Consumes: `(*Store).LoadManifest`, `(*Store).GetBlob`, `(*Store).PutBlob`, `(*Store).HasBlob`, `(*Store).SaveManifest`, `validate`, `gz`, `gunzip`, `vault.*`.
- Produces: `snapshot.Export(st, ref string, w io.Writer, passphrase []byte) (*Manifest, error)`,
  `snapshot.Import(st, r io.Reader, passphrase func() ([]byte, error)) (*ImportReport, error)`,
  `snapshot.ImportReport{Manifest *Manifest; Missing []string}`, `snapshot.ErrPassphrase`.

- [ ] **Step 1: Escribir los tests**

```go
package snapshot

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"testing"
)

func archiveFixture(t *testing.T) (*Store, *Manifest) {
	t.Helper()
	st := openTemp(t)
	m, err := Capture(st, []Source{
		src("ccp/ccp.yaml", ClassAuthored, "version: 2\n"),
		src("ccp/profiles/deep/api_key", ClassSecret, "sk-muy-secreto"),
	}, meta(t0), false)
	if err != nil {
		t.Fatal(err)
	}
	return st, m
}

func pass(p string) func() ([]byte, error) {
	return func() ([]byte, error) { return []byte(p), nil }
}

func TestExportImportWithoutSecrets(t *testing.T) {
	st, m := archiveFixture(t)
	var buf bytes.Buffer
	if _, err := Export(st, m.ID, &buf, nil); err != nil {
		t.Fatalf("Export: %v", err)
	}
	if bytes.Contains(gunzipAll(t, buf.Bytes()), []byte("sk-muy-secreto")) {
		t.Fatal("el archivo sin frase contiene el secreto")
	}
	dst := openTemp(t)
	rep, err := Import(dst, &buf, func() ([]byte, error) { t.Fatal("no debería pedir frase"); return nil, nil })
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if rep.Manifest.ID != m.ID || len(rep.Missing) != 1 || rep.Missing[0] != "ccp/profiles/deep/api_key" {
		t.Fatalf("rep = %+v", rep)
	}
	if _, err := dst.LoadManifest(m.ID); err != nil {
		t.Fatalf("el manifiesto importado no carga: %v", err)
	}
}

func TestExportImportWithSecrets(t *testing.T) {
	st, m := archiveFixture(t)
	var buf bytes.Buffer
	if _, err := Export(st, m.ID, &buf, []byte("frase de prueba larga")); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(gunzipAll(t, buf.Bytes()), []byte("sk-muy-secreto")) {
		t.Fatal("el secreto viaja en claro")
	}
	raw := buf.Bytes()

	if _, err := Import(openTemp(t), bytes.NewReader(raw), pass("otra frase")); !errors.Is(err, ErrPassphrase) {
		t.Fatalf("frase equivocada: err = %v, quiero ErrPassphrase", err)
	}
	dst := openTemp(t)
	rep, err := Import(dst, bytes.NewReader(raw), pass("frase de prueba larga"))
	if err != nil || len(rep.Missing) != 0 {
		t.Fatalf("Import = %+v, %v", rep, err)
	}
	for _, it := range m.Items {
		if _, err := dst.GetBlob(it.Hash); err != nil {
			t.Fatalf("%s no llegó: %v", it.LPath, err)
		}
	}
}

// Un archivo con algo que su manifiesto no nombra, o con un objeto que no
// corresponde a su hash, se rechaza entero.
func TestImportRejectsForeignOrTamperedObjects(t *testing.T) {
	st, m := archiveFixture(t)
	var buf bytes.Buffer
	Export(st, m.ID, &buf, nil)

	extra := rewriteArchive(t, buf.Bytes(), func(name string, body []byte) (string, []byte) { return name, body }, "objects/"+Hash([]byte("intruso")))
	if _, err := Import(openTemp(t), bytes.NewReader(extra), pass("")); err == nil {
		t.Fatal("aceptó un objeto que el manifiesto no nombra")
	}
	tampered := rewriteArchive(t, buf.Bytes(), func(name string, body []byte) (string, []byte) {
		if name == "objects/"+m.Items[0].Hash {
			b, _ := gz([]byte("otro contenido"))
			return name, b
		}
		return name, body
	}, "")
	if _, err := Import(openTemp(t), bytes.NewReader(tampered), pass("")); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("objeto alterado: err = %v, quiero ErrCorrupt", err)
	}
}

func gunzipAll(t *testing.T, data []byte) []byte {
	t.Helper()
	r, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	out, _ := io.ReadAll(r)
	return out
}

// rewriteArchive reescribe un .ccpsnap pasando cada entrada por edit y, si
// extraName no está vacío, añade al final una entrada con ese nombre.
func rewriteArchive(t *testing.T, data []byte, edit func(string, []byte) (string, []byte), extraName string) []byte {
	t.Helper()
	gzr, _ := gzip.NewReader(bytes.NewReader(data))
	tr := tar.NewReader(gzr)
	var out bytes.Buffer
	gzw := gzip.NewWriter(&out)
	tw := tar.NewWriter(gzw)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		body, _ := io.ReadAll(tr)
		name, body := edit(hdr.Name, body)
		writeEntry(tw, name, body)
	}
	if extraName != "" {
		b, _ := gz([]byte("intruso"))
		writeEntry(tw, extraName, b)
	}
	tw.Close()
	gzw.Close()
	return out.Bytes()
}
```

- [ ] **Step 2: Comprobar que falla**

Run: `go test ./internal/snapshot/ -run 'Export|Import'`
Expected: FAIL, no compila.

- [ ] **Step 3: Implementar `internal/snapshot/archive.go`**

```go
package snapshot

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/JoseAFlores777/ccp/internal/vault"
)

const (
	archiveFormat   = 1
	archiveHeader   = "ccpsnap.json"
	archiveManifest = "manifest.json"
	archiveObjects  = "objects/"
	secretsOmitted  = "omitted"
	secretsSealed   = "sealed"
)

type archiveHead struct {
	Format   int              `json:"format"`
	Snapshot string           `json:"snapshot"`
	Secrets  string           `json:"secrets"`
	KDF      *vault.KDFParams `json:"kdf,omitempty"`
}

// ErrPassphrase: la frase no abre los secretos del archivo (o están alterados).
var ErrPassphrase = errors.New("snapshot: la frase no abre los secretos del archivo")

// secretOnly marca con true los blobs que SOLO usan elementos secretos: esos son
// los que se sellan (o se omiten). Un contenido que también es un archivo normal
// ya viaja en claro por ese otro camino, y sellarlo no protegería nada.
func secretOnly(items []Item) map[string]bool {
	out := map[string]bool{}
	for _, it := range items {
		if it.Class != ClassSecret {
			out[it.Hash] = false
			continue
		}
		if _, seen := out[it.Hash]; !seen {
			out[it.Hash] = true
		}
	}
	return out
}

// Export escribe el snapshot ref en w como .ccpsnap. Sin passphrase, los blobs
// secretos no se incluyen; con ella, van sellados con una clave derivada.
func Export(st *Store, ref string, w io.Writer, passphrase []byte) (*Manifest, error) {
	m, err := st.LoadManifest(ref)
	if err != nil {
		return nil, err
	}
	head := archiveHead{Format: archiveFormat, Snapshot: m.ID, Secrets: secretsOmitted}
	var key []byte
	if len(passphrase) > 0 {
		p, err := vault.NewKDFParams()
		if err != nil {
			return nil, err
		}
		if key, err = vault.DeriveKey(passphrase, p); err != nil {
			return nil, err
		}
		head.Secrets, head.KDF = secretsSealed, &p
	}
	gzw := gzip.NewWriter(w)
	tw := tar.NewWriter(gzw)
	if err := writeJSONEntry(tw, archiveHeader, head); err != nil {
		return nil, err
	}
	if err := writeJSONEntry(tw, archiveManifest, m); err != nil {
		return nil, err
	}
	sealed := secretOnly(m.Items)
	hashes := make([]string, 0, len(sealed))
	for h := range sealed {
		hashes = append(hashes, h)
	}
	sort.Strings(hashes)
	for _, h := range hashes {
		if sealed[h] && key == nil {
			continue
		}
		data, err := st.GetBlob(h)
		if errors.Is(err, ErrNotFound) {
			continue // nunca llegó a este almacén (p. ej. se importó sin secretos)
		}
		if err != nil {
			return nil, err
		}
		body, err := gz(data)
		if err != nil {
			return nil, err
		}
		if sealed[h] {
			if body, err = vault.Seal(key, body, []byte(h)); err != nil {
				return nil, err
			}
		}
		if err := writeEntry(tw, archiveObjects+h, body); err != nil {
			return nil, err
		}
	}
	if err := tw.Close(); err != nil {
		return nil, fmt.Errorf("snapshot: tar: %w", err)
	}
	if err := gzw.Close(); err != nil {
		return nil, fmt.Errorf("snapshot: gzip: %w", err)
	}
	return m, nil
}

func writeEntry(tw *tar.Writer, name string, data []byte) error {
	if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o600, Size: int64(len(data)), Format: tar.FormatPAX}); err != nil {
		return fmt.Errorf("snapshot: tar: %w", err)
	}
	if _, err := tw.Write(data); err != nil {
		return fmt.Errorf("snapshot: tar: %w", err)
	}
	return nil
}

func writeJSONEntry(tw *tar.Writer, name string, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("snapshot: %s: %w", name, err)
	}
	return writeEntry(tw, name, data)
}

// ImportReport resume una importación. Missing son las rutas lógicas cuyos datos
// no venían en el archivo (se exportó sin secretos) ni estaban ya en este
// almacén: el snapshot existe, pero esas rutas no se podrán restaurar.
type ImportReport struct {
	Manifest *Manifest
	Missing  []string
}

// Import lee un .ccpsnap de r y lo añade al almacén. passphrase solo se llama si
// el archivo trae secretos sellados, y una sola vez.
func Import(st *Store, r io.Reader, passphrase func() ([]byte, error)) (*ImportReport, error) {
	gzr, err := gzip.NewReader(r)
	if err != nil {
		return nil, fmt.Errorf("snapshot: no es un .ccpsnap: %w", err)
	}
	defer gzr.Close()
	tr := tar.NewReader(gzr)

	var head archiveHead
	if err := readJSONEntry(tr, archiveHeader, &head); err != nil {
		return nil, err
	}
	if head.Format != archiveFormat {
		return nil, fmt.Errorf("snapshot: .ccpsnap de formato %d; este ccp conoce el %d", head.Format, archiveFormat)
	}
	var m Manifest
	if err := readJSONEntry(tr, archiveManifest, &m); err != nil {
		return nil, err
	}
	if err := validate(&m); err != nil {
		return nil, err
	}
	id, err := m.computeID()
	if err != nil {
		return nil, err
	}
	if id != m.ID || m.ID != head.Snapshot {
		return nil, fmt.Errorf("%w: el manifiesto del archivo no corresponde a su id", ErrCorrupt)
	}

	sealed := secretOnly(m.Items)
	got := map[string]bool{}
	var key []byte
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("snapshot: .ccpsnap dañado: %w", err)
		}
		h, isObj := strings.CutPrefix(hdr.Name, archiveObjects)
		isSealed, named := sealed[h]
		if !isObj || !named || hdr.Typeflag != tar.TypeReg {
			return nil, fmt.Errorf("snapshot: el archivo trae algo que su manifiesto no nombra: %q", hdr.Name)
		}
		body, err := io.ReadAll(io.LimitReader(tr, MaxBlobSize+1))
		if err != nil {
			return nil, fmt.Errorf("snapshot: .ccpsnap dañado: %w", err)
		}
		if isSealed {
			if head.Secrets != secretsSealed || head.KDF == nil {
				return nil, fmt.Errorf("%w: un secreto sellado sin parámetros de derivación", ErrCorrupt)
			}
			if key == nil {
				p, err := passphrase()
				if err != nil {
					return nil, err
				}
				if key, err = vault.DeriveKey(p, *head.KDF); err != nil {
					return nil, err
				}
			}
			if body, err = vault.Open(key, body, []byte(h)); err != nil {
				return nil, ErrPassphrase
			}
		}
		data, err := gunzip(body)
		if err != nil || Hash(data) != h {
			return nil, fmt.Errorf("%w: %s", ErrCorrupt, Short(h))
		}
		if _, err := st.PutBlob(data, isSealed); err != nil {
			return nil, err
		}
		got[h] = true
	}
	rep := &ImportReport{Manifest: &m}
	for _, it := range m.Items {
		if !got[it.Hash] && !st.HasBlob(it.Hash) {
			rep.Missing = append(rep.Missing, it.LPath)
		}
	}
	if err := st.SaveManifest(&m); err != nil {
		return nil, err
	}
	return rep, nil
}

func readJSONEntry(tr *tar.Reader, want string, v any) error {
	hdr, err := tr.Next()
	if err != nil {
		return fmt.Errorf("snapshot: .ccpsnap sin %s: %w", want, err)
	}
	if hdr.Name != want {
		return fmt.Errorf("snapshot: .ccpsnap: esperaba %s y encontré %s", want, hdr.Name)
	}
	data, err := io.ReadAll(io.LimitReader(tr, 64<<20))
	if err != nil {
		return fmt.Errorf("snapshot: .ccpsnap: %s: %w", want, err)
	}
	if err := json.Unmarshal(data, v); err != nil {
		return fmt.Errorf("snapshot: .ccpsnap: %s inválido: %w", want, err)
	}
	return nil
}
```

- [ ] **Step 4: Comprobar que pasa**

Run: `go test ./internal/snapshot/ && go vet ./internal/snapshot/`
Expected: PASS.

- [ ] **Step 5: Gates y commit**

Mensaje propuesto: `feat(snapshot): exportar e importar .ccpsnap con secretos sellados por frase`

---

### Task 7: `core` — la parte de configuración de un `.claude.json`

`~/.claude.json` y cada `cc-home/.claude.json` mezclan configuración con decenas
de claves de estado. En esta máquina hay unas 60: `machineID`, cachés,
contadores, `oauthAccount`. Un snapshot guarda **solo la configuración**, y
restaurarla la **fusiona** con el archivo vivo: nunca lo sustituye, porque
Claude Code lo reescribe constantemente.

**Files:**
- Create: `internal/core/claude_json.go`
- Create: `internal/core/claude_json_test.go`

**Interfaces:**
- Produces: `core.ClaudeJSONConfig(data []byte) ([]byte, error)` (nil si no hay
  configuración), `core.ClaudeJSONApplyConfig(live, cfg []byte) ([]byte, error)`.
  Interno: `claudeJSONProjectKeys`, `jsonEmpty`.

- [ ] **Step 1: Escribir los tests**

```go
package core

import (
	"encoding/json"
	"reflect"
	"testing"
)

const liveClaudeJSON = `{
  "machineID": "m-123",
  "numStartups": 42,
  "oauthAccount": {"emailAddress": "x@y.z"},
  "mcpServers": {"github": {"command": "npx", "args": ["-y", "gh"], "env": {"TOKEN": "t"}}},
  "projects": {
    "/repo/a": {"allowedTools": ["Bash(ls)"], "mcpServers": {}, "history": [1, 2], "hasTrustDialogAccepted": true},
    "/repo/b": {"mcpServers": {"local": {"command": "node"}}, "enabledMcpjsonServers": ["x"]},
    "/repo/c": {"history": []}
  }
}`

func decode(t *testing.T, data []byte) map[string]any {
	t.Helper()
	var v map[string]any
	if err := json.Unmarshal(data, &v); err != nil {
		t.Fatalf("JSON inválido: %v\n%s", err, data)
	}
	return v
}

func TestClaudeJSONConfigKeepsOnlyConfig(t *testing.T) {
	sub, err := ClaudeJSONConfig([]byte(liveClaudeJSON))
	if err != nil {
		t.Fatal(err)
	}
	got := decode(t, sub)
	want := decode(t, []byte(`{
	  "mcpServers": {"github": {"command": "npx", "args": ["-y", "gh"], "env": {"TOKEN": "t"}}},
	  "projects": {
	    "/repo/a": {"allowedTools": ["Bash(ls)"]},
	    "/repo/b": {"mcpServers": {"local": {"command": "node"}}, "enabledMcpjsonServers": ["x"]}
	  }
	}`))
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("subconjunto = %v\nquiero %v", got, want)
	}
}

func TestClaudeJSONConfigNothingToKeep(t *testing.T) {
	sub, err := ClaudeJSONConfig([]byte(`{"machineID":"x","mcpServers":{},"projects":{"/r":{"history":[]}}}`))
	if err != nil || sub != nil {
		t.Fatalf("sub = %s, %v; quiero nil", sub, err)
	}
	if _, err := ClaudeJSONConfig([]byte(`[1]`)); err == nil {
		t.Fatal("aceptó un .claude.json que no es un objeto")
	}
}

// Restaurar deja la configuración exactamente como en el snapshot y no toca
// nada más: el estado de Claude Code (machineID, contadores, historial) sigue
// siendo el vivo.
func TestClaudeJSONApplyConfig(t *testing.T) {
	sub, _ := ClaudeJSONConfig([]byte(liveClaudeJSON))
	live := []byte(`{
	  "machineID": "m-999",
	  "numStartups": 50,
	  "mcpServers": {"otro": {"command": "x"}},
	  "projects": {
	    "/repo/a": {"allowedTools": ["Bash(rm)"], "history": [1, 2, 3]},
	    "/repo/d": {"mcpServers": {"nuevo": {"command": "y"}}, "history": [9]}
	  }
	}`)
	out, err := ClaudeJSONApplyConfig(live, sub)
	if err != nil {
		t.Fatal(err)
	}
	got := decode(t, out)
	want := decode(t, []byte(`{
	  "machineID": "m-999",
	  "numStartups": 50,
	  "mcpServers": {"github": {"command": "npx", "args": ["-y", "gh"], "env": {"TOKEN": "t"}}},
	  "projects": {
	    "/repo/a": {"allowedTools": ["Bash(ls)"], "history": [1, 2, 3]},
	    "/repo/b": {"mcpServers": {"local": {"command": "node"}}, "enabledMcpjsonServers": ["x"]},
	    "/repo/d": {"history": [9]}
	  }
	}`))
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("fusión = %v\nquiero %v", got, want)
	}
}

func TestClaudeJSONApplyConfigToMissingFile(t *testing.T) {
	sub, _ := ClaudeJSONConfig([]byte(liveClaudeJSON))
	out, err := ClaudeJSONApplyConfig(nil, sub)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decode(t, out), decode(t, sub)) {
		t.Fatalf("sobre un archivo que no existe, el resultado debe ser la configuración tal cual:\n%s", out)
	}
	if _, err := ClaudeJSONApplyConfig([]byte(`[1]`), sub); err == nil {
		t.Fatal("fusionó sobre algo que no es un objeto")
	}
}
```

- [ ] **Step 2: Comprobar que falla**

Run: `go test ./internal/core/ -run ClaudeJSON`
Expected: FAIL, no compila.

- [ ] **Step 3: Implementar `internal/core/claude_json.go`**

```go
package core

// claude_json.go — la parte de CONFIGURACIÓN de un .claude.json.
//
// Ese archivo mezcla lo que el usuario configura (los MCP de scope user, y por
// proyecto sus MCP, sus aprobaciones de .mcp.json y sus herramientas
// permitidas) con decenas de claves de estado que Claude Code reescribe
// constantemente (machineID, cachés, contadores, oauthAccount). Un snapshot
// guarda solo lo primero, y restaurar lo FUSIONA con el archivo vivo.
// Sustituir el archivo entero pisaría la identidad de la máquina y el estado de
// una sesión en marcha.

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// claudeJSONProjectKeys son las claves de configuración de cada proyecto.
var claudeJSONProjectKeys = []string{"mcpServers", "enabledMcpjsonServers", "disabledMcpjsonServers", "allowedTools"}

// jsonEmpty dice si raw es null, "", {} o [], con o sin espacios.
func jsonEmpty(raw json.RawMessage) bool {
	var b bytes.Buffer
	if err := json.Compact(&b, raw); err != nil {
		return false
	}
	switch b.String() {
	case "", "null", "{}", "[]", `""`:
		return true
	}
	return false
}

// ClaudeJSONConfig extrae la configuración de un .claude.json. Devuelve nil si
// no hay ninguna que guardar.
func ClaudeJSONConfig(data []byte) ([]byte, error) {
	var top map[string]json.RawMessage
	if err := json.Unmarshal(data, &top); err != nil {
		return nil, fmt.Errorf(".claude.json no es un objeto JSON: %w", err)
	}
	out := map[string]any{}
	if raw, ok := top["mcpServers"]; ok && !jsonEmpty(raw) {
		out["mcpServers"] = raw
	}
	if raw, ok := top["projects"]; ok && !jsonEmpty(raw) {
		var projects map[string]map[string]json.RawMessage
		if err := json.Unmarshal(raw, &projects); err != nil {
			return nil, fmt.Errorf(".claude.json: projects: %w", err)
		}
		picked := map[string]map[string]json.RawMessage{}
		for path, p := range projects {
			keep := map[string]json.RawMessage{}
			for _, k := range claudeJSONProjectKeys {
				if v, ok := p[k]; ok && !jsonEmpty(v) {
					keep[k] = v
				}
			}
			if len(keep) > 0 {
				picked[path] = keep
			}
		}
		if len(picked) > 0 {
			out["projects"] = picked
		}
	}
	if len(out) == 0 {
		return nil, nil
	}
	b, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return nil, fmt.Errorf(".claude.json: %w", err)
	}
	return append(b, '\n'), nil
}

// ClaudeJSONApplyConfig deja la configuración de live exactamente como dice cfg
// (lo que cfg no trae, se quita) y conserva intacto todo lo demás. live vacío
// significa que el archivo no existe.
func ClaudeJSONApplyConfig(live, cfg []byte) ([]byte, error) {
	top := map[string]json.RawMessage{}
	if len(bytes.TrimSpace(live)) > 0 {
		if err := json.Unmarshal(live, &top); err != nil {
			return nil, fmt.Errorf(".claude.json no es un objeto JSON: %w", err)
		}
		if top == nil {
			top = map[string]json.RawMessage{}
		}
	}
	var want struct {
		MCPServers json.RawMessage                       `json:"mcpServers"`
		Projects   map[string]map[string]json.RawMessage `json:"projects"`
	}
	if err := json.Unmarshal(cfg, &want); err != nil {
		return nil, fmt.Errorf("configuración del snapshot inválida: %w", err)
	}
	if len(want.MCPServers) > 0 {
		top["mcpServers"] = want.MCPServers
	} else {
		delete(top, "mcpServers")
	}

	rawProjects, hadProjects := top["projects"]
	projects := map[string]map[string]json.RawMessage{}
	if hadProjects && !jsonEmpty(rawProjects) {
		if err := json.Unmarshal(rawProjects, &projects); err != nil {
			return nil, fmt.Errorf(".claude.json: projects: %w", err)
		}
	}
	for _, p := range projects {
		for _, k := range claudeJSONProjectKeys {
			delete(p, k)
		}
	}
	for path, keys := range want.Projects {
		p := projects[path]
		if p == nil {
			p = map[string]json.RawMessage{}
			projects[path] = p
		}
		for _, k := range claudeJSONProjectKeys {
			if v, ok := keys[k]; ok {
				p[k] = v
			}
		}
	}
	if len(projects) > 0 || hadProjects {
		b, err := json.Marshal(projects)
		if err != nil {
			return nil, fmt.Errorf(".claude.json: projects: %w", err)
		}
		top["projects"] = b
	}
	out, err := json.MarshalIndent(top, "", "  ")
	if err != nil {
		return nil, fmt.Errorf(".claude.json: %w", err)
	}
	return append(out, '\n'), nil
}
```

Nota: `json.MarshalIndent` ordena las claves, así que el archivo fusionado queda
con las claves en orden alfabético. A Claude Code le da igual el orden, pero el
diff de texto del primer restore será ruidoso. Se acepta.

- [ ] **Step 4: Comprobar que pasa**

Run: `go test ./internal/core/ -run ClaudeJSON && go vet ./internal/core/`
Expected: PASS.

- [ ] **Step 5: Gates y commit**

Mensaje propuesto: `feat(core): extraer y fusionar la configuración de un .claude.json`

---

### Task 8: `core` — qué captura un snapshot en esta máquina

El mapa entre rutas lógicas y archivos reales (spec §8.1). Es el único sitio que
hay que tocar el día que ccp gestione un archivo nuevo. Lo generado (`cc-home/`),
la caché de plugins y lo atado a la máquina (tokens, `machineID`, sesión de
Desktop) no se capturan nunca.

**Files:**
- Create: `internal/core/snapshot_layout.go`
- Create: `internal/core/snapshot_layout_test.go`

**Interfaces:**
- Consumes: `Load`, `yamlPath`, `cfgOverlayDir`, `cfgInstrFile`, `apiKeyPath`, `ccHomePath`, `DesktopDataDir`, `DesktopUserDataDir`, `handoffsPath`, `ClaudeJSONConfig` (Task 7), `snapshot.*` (Tasks 2-4).
- Produces:
  - `core.SnapshotStoreDir(home) string`, `core.OpenSnapshotStore(home) (*snapshot.Store, error)`;
  - `core.SnapshotSourceOpts{WithState bool}`;
  - `core.SnapshotSources(home, src string, o SnapshotSourceOpts) ([]snapshot.Source, error)`;
  - `core.SnapshotCaptureOpts{Trigger, Label string; WithState, Force bool; Now time.Time; Machine string}`;
  - `core.SnapshotCapture(home, src string, st *snapshot.Store, o SnapshotCaptureOpts) (*snapshot.Manifest, error)`;
  - `core.SnapshotLive(home, src string, o SnapshotSourceOpts) ([]snapshot.Item, error)`;
  - internos: `projectKey`, `gitOriginURL`, `normalizeRemote`, `projectLocalFiles`.

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

// snapFixture monta una máquina falsa completa:
//   - un ~/.claude con configuración y una caché de plugins;
//   - un ~/.claude.json con MCP y estado;
//   - un perfil official y otro deepseek;
//   - la config de las ventanas de Desktop;
//   - un repo con regla de carpeta y archivos locales.
func snapFixture(t *testing.T) (home, src, repo string) {
	t.Helper()
	home = t.TempDir()
	src = filepath.Join(t.TempDir(), ".claude")
	desktopDefault := t.TempDir()
	t.Setenv("CCP_CLAUDE_SRC", src)
	t.Setenv("CCP_DESKTOP_DEFAULT_DATA_DIR", desktopDefault)
	t.Setenv("HOME", t.TempDir())
	write := func(p, s string, mode os.FileMode) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(s), mode); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(src, "settings.json"), `{"theme":"dark"}`, 0o644)
	write(filepath.Join(src, "agents", "revisor.md"), "# revisor\n", 0o644)
	write(filepath.Join(src, "plugins", "installed_plugins.json"), `{"plugins":{}}`, 0o644)
	write(filepath.Join(src, "plugins", "cache", "x", "big.bin"), "cache", 0o644)
	write(src+".json", liveClaudeJSON, 0o600)
	if err := os.Symlink(filepath.Join(src, "agents", "revisor.md"), filepath.Join(src, "agents", "alias.md")); err != nil {
		t.Fatal(err)
	}

	if err := ProfileAddOfficial(home, "work"); err != nil {
		t.Fatal(err)
	}
	if err := CfgInitOverlay(home, "work"); err != nil {
		t.Fatal(err)
	}
	write(cfgInstrFile(home, "work"), "# work\n", 0o644)
	write(filepath.Join(ccHomePath(home, "work"), ".claude.json"), `{"machineID":"w","mcpServers":{"jira":{"type":"http","url":"https://j"}}}`, 0o600)
	write(filepath.Join(DesktopDataDir(home, "work"), "claude_desktop_config.json"), `{"mcpServers":{}}`, 0o600)
	if err := ProfileAddDeepseek(home, "deep", BuiltinDefaults()); err != nil {
		t.Fatal(err)
	}
	if err := ProfileSetKey(home, "deep", "sk-1"); err != nil {
		t.Fatal(err)
	}
	write(filepath.Join(desktopDefault, "claude_desktop_config.json"), `{"mcpServers":{"obsidian":{"command":"node"}}}`, 0o600)

	repo = t.TempDir()
	write(filepath.Join(repo, ".git", "config"), "[core]\n\tbare = false\n[remote \"origin\"]\n\turl = git@github.com:Org/App.git\n", 0o644)
	write(filepath.Join(repo, ".claude", "settings.local.json"), `{"permissions":{"allow":["Bash(make)"]}}`, 0o644)
	if _, err := RuleSet(home, repo, "work"); err != nil {
		t.Fatal(err)
	}
	return home, src, repo
}

func sourcesByLPath(t *testing.T, srcs []snapshot.Source) map[string]snapshot.Source {
	t.Helper()
	out := map[string]snapshot.Source{}
	for _, s := range srcs {
		out[s.LPath] = s
	}
	return out
}

func TestSnapshotSources(t *testing.T) {
	home, src, repo := snapFixture(t)
	srcs, err := SnapshotSources(home, src, SnapshotSourceOpts{})
	if err != nil {
		t.Fatalf("SnapshotSources: %v", err)
	}
	got := sourcesByLPath(t, srcs)
	key := projectKey(repo, "git@github.com:Org/App.git")
	want := map[string]snapshot.Class{
		"ccp/ccp.yaml":                              snapshot.ClassAuthored,
		"ccp/profiles/work/overlay/CLAUDE.md":       snapshot.ClassAuthored,
		"ccp/profiles/work/cc-home/.claude.json":    snapshot.ClassSecret,
		"ccp/profiles/deep/api_key":                 snapshot.ClassSecret,
		"desktop/work/claude_desktop_config.json":   snapshot.ClassSecret,
		"desktop/default/claude_desktop_config.json": snapshot.ClassSecret,
		"claude/settings.json":                      snapshot.ClassAuthored,
		"claude/agents/revisor.md":                  snapshot.ClassAuthored,
		"claude/plugins/installed_plugins.json":     snapshot.ClassAuthored,
		"claude/.claude.json":                       snapshot.ClassSecret,
		"project/" + key + "/.claude/settings.local.json": snapshot.ClassAuthored,
	}
	for l, class := range want {
		s, ok := got[l]
		if !ok {
			t.Errorf("falta %s", l)
			continue
		}
		if s.Class != class {
			t.Errorf("%s: clase %s, quiero %s", l, s.Class, class)
		}
	}
	for l := range got {
		for _, forbidden := range []string{"plugins/cache", "agents/alias.md", "cc-home/agents", "cc-home/settings.json", "cc-home/CLAUDE.md", "handoffs.yaml"} {
			if strings.Contains(l, forbidden) {
				t.Errorf("se capturó %s, que no es configuración del usuario", l)
			}
		}
	}
	p := got["project/"+key+"/.claude/settings.local.json"]
	if p.Meta["path"] != repo || p.Meta["remote"] != "git@github.com:Org/App.git" {
		t.Errorf("meta del proyecto = %v", p.Meta)
	}
	// De ~/.claude.json solo viaja la configuración.
	data, err := got["claude/.claude.json"].Read()
	if err != nil || strings.Contains(string(data), "machineID") || !strings.Contains(string(data), "github") {
		t.Errorf("claude/.claude.json = %s, %v", data, err)
	}
}

func TestSnapshotSourcesWithState(t *testing.T) {
	home, src, _ := snapFixture(t)
	transcript := filepath.Join(ccHomePath(home, "work"), "projects", "-repo", "abc.jsonl")
	os.MkdirAll(filepath.Dir(transcript), 0o755)
	os.WriteFile(transcript, []byte(`{"uuid":"1"}`+"\n"), 0o600)
	os.WriteFile(handoffsPath(home), []byte("version: 2\n"), 0o644)

	without, _ := SnapshotSources(home, src, SnapshotSourceOpts{})
	with, err := SnapshotSources(home, src, SnapshotSourceOpts{WithState: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range []string{"ccp/profiles/work/cc-home/projects/-repo/abc.jsonl", "ccp/handoffs.yaml"} {
		if _, ok := sourcesByLPath(t, without)[l]; ok {
			t.Errorf("%s capturado sin WithState", l)
		}
		if s, ok := sourcesByLPath(t, with)[l]; !ok || s.Class != snapshot.ClassState {
			t.Errorf("%s con WithState: %+v, %v", l, s, ok)
		}
	}
}

func TestProjectKey(t *testing.T) {
	same := []string{"git@github.com:Org/App.git", "https://github.com/org/app", "ssh://git@github.com/Org/App.git"}
	for _, r := range same[1:] {
		if projectKey("/a", r) != projectKey("/b", same[0]) {
			t.Errorf("%s y %s deberían ser el mismo proyecto", r, same[0])
		}
	}
	if projectKey("/a", "") == projectKey("/b", "") {
		t.Error("sin remoto, dos rutas distintas deben ser proyectos distintos")
	}
}

func TestSnapshotCapture(t *testing.T) {
	home, src, _ := snapFixture(t)
	st, err := OpenSnapshotStore(home)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)
	m, err := SnapshotCapture(home, src, st, SnapshotCaptureOpts{Trigger: "manual", Now: now, Machine: "test"})
	if err != nil {
		t.Fatalf("SnapshotCapture: %v", err)
	}
	if m.Trigger != "manual" || m.CCPVersion != Version || len(m.Items) < 10 {
		t.Fatalf("m = %+v", m)
	}
	live, err := SnapshotLive(home, src, SnapshotSourceOpts{})
	if err != nil || len(snapshot.Diff(m.Items, live)) != 0 {
		t.Fatalf("recién capturado, el estado vivo debe ser idéntico: %v", err)
	}
}
```

- [ ] **Step 2: Comprobar que falla**

Run: `go test ./internal/core/ -run 'SnapshotSources|ProjectKey|SnapshotCapture'`
Expected: FAIL, no compila.

- [ ] **Step 3: Implementar `internal/core/snapshot_layout.go`**

```go
package core

// snapshot_layout.go — qué captura un snapshot en ESTA máquina (spec 2026-09-18
// §8.1). internal/snapshot no sabe nada del disco: aquí vive el mapa entre rutas
// lógicas y rutas reales, y es lo único que hay que tocar el día que ccp
// gestione un archivo nuevo. El camino de vuelta está en snapshot_restore.go.
//
//	ccp/ccp.yaml                                   authored
//	ccp/handoffs.yaml                              state (solo con WithState)
//	ccp/profiles/<p>/overlay/…                     authored
//	ccp/profiles/<p>/api_key                       secret
//	ccp/profiles/<p>/cc-home/.claude.json          secret: solo su configuración
//	ccp/profiles/<p>/cc-home/projects/…            state (solo con WithState)
//	claude/{settings.json,CLAUDE.md,keybindings.json}            authored
//	claude/{agents,commands,skills,output-styles,hooks}/…        authored
//	claude/plugins/{installed_plugins,known_marketplaces}.json   authored
//	claude/.claude.json                            secret: solo su configuración
//	desktop/<p>/claude_desktop_config.json         secret (default incluido)
//	project/<clave>/{.claude/settings.local.json,CLAUDE.local.md}  authored
//
// Lo generado (cc-home/settings.json y CLAUDE.md, espejos, lanzadores), la caché
// y lo atado a la máquina (tokens, machineID, sesión de Desktop) no se capturan
// nunca: se regeneran o se recrean.

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/JoseAFlores777/ccp/internal/snapshot"
)

// SnapshotStoreDir es el almacén local de snapshots: <home>/snapshots.
func SnapshotStoreDir(home string) string { return filepath.Join(home, "snapshots") }

// OpenSnapshotStore abre (o crea) el almacén local.
func OpenSnapshotStore(home string) (*snapshot.Store, error) {
	return snapshot.Open(SnapshotStoreDir(home))
}

// SnapshotSourceOpts elige qué se captura además de la configuración.
type SnapshotSourceOpts struct {
	// WithState añade conversaciones (cc-home/projects) y handoffs.yaml. Pesan y
	// son sensibles, por eso no van por defecto (spec D4).
	WithState bool
}

// Del ~/.claude global, la configuración del usuario. De plugins/ entran solo
// sus dos índices: el resto es caché que se vuelve a descargar.
var (
	claudeGlobalFiles = []string{"settings.json", "CLAUDE.md", "keybindings.json",
		"plugins/installed_plugins.json", "plugins/known_marketplaces.json"}
	claudeGlobalTrees = []string{"agents", "commands", "skills", "output-styles", "hooks"}
	// Lo local (no versionado) de un proyecto. Lo versionado ya tiene su historia en git.
	projectLocalFiles = []string{".claude/settings.local.json", "CLAUDE.local.md"}
)

// SnapshotSources enumera lo que un snapshot captura en esta máquina.
func SnapshotSources(home, src string, o SnapshotSourceOpts) ([]snapshot.Source, error) {
	cfg, err := Load(home)
	if err != nil {
		return nil, err
	}
	var (
		out      []snapshot.Source
		firstErr error
	)
	keep := func(ss []snapshot.Source, err error) {
		out = append(out, ss...)
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}
	file := func(lpath, abs string, class snapshot.Class) {
		keep(fileSource(lpath, abs, class, nil))
	}

	file("ccp/ccp.yaml", yamlPath(home), snapshot.ClassAuthored)
	if o.WithState {
		file("ccp/handoffs.yaml", handoffsPath(home), snapshot.ClassState)
	}

	names := make([]string, 0, len(cfg.Profiles))
	for n := range cfg.Profiles {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		base := "ccp/profiles/" + n
		keep(treeSources(base+"/overlay", cfgOverlayDir(home, n), snapshot.ClassAuthored))
		file(base+"/api_key", apiKeyPath(home, n), snapshot.ClassSecret)
		keep(claudeJSONSource(base+"/cc-home/.claude.json", filepath.Join(ccHomePath(home, n), ".claude.json")))
		file("desktop/"+n+"/claude_desktop_config.json", filepath.Join(DesktopDataDir(home, n), "claude_desktop_config.json"), snapshot.ClassSecret)
		if o.WithState {
			keep(treeSources(base+"/cc-home/projects", filepath.Join(ccHomePath(home, n), "projects"), snapshot.ClassState))
		}
	}

	for _, f := range claudeGlobalFiles {
		file("claude/"+f, filepath.Join(src, filepath.FromSlash(f)), snapshot.ClassAuthored)
	}
	for _, d := range claudeGlobalTrees {
		keep(treeSources("claude/"+d, filepath.Join(src, d), snapshot.ClassAuthored))
	}
	// El .claude.json del perfil default vive junto a ~/.claude, no dentro.
	keep(claudeJSONSource("claude/.claude.json", src+".json"))
	if dir, err := DesktopUserDataDir(home, "default"); err == nil {
		file("desktop/default/claude_desktop_config.json", filepath.Join(dir, "claude_desktop_config.json"), snapshot.ClassSecret)
	}
	keep(projectSources(cfg.Rules))

	if firstErr != nil {
		return nil, firstErr
	}
	sort.Slice(out, func(i, j int) bool { return out[i].LPath < out[j].LPath })
	return out, nil
}

// fileSource devuelve la fuente de un archivo regular, o nada si no existe o no
// es regular. Un symlink nunca se sigue: apunta a algo que se captura por su
// propia ruta o que no es del usuario.
func fileSource(lpath, abs string, class snapshot.Class, meta map[string]string) ([]snapshot.Source, error) {
	fi, err := os.Lstat(abs)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("no se pudo inspeccionar %s: %w", abs, err)
	}
	if !fi.Mode().IsRegular() {
		return nil, nil
	}
	return []snapshot.Source{{
		LPath: lpath, Class: class, Mode: uint32(fi.Mode().Perm()), Meta: meta,
		Read: func() ([]byte, error) { return os.ReadFile(abs) },
	}}, nil
}

// treeSources devuelve una fuente por archivo regular bajo dir, con ruta lógica
// prefix/<relativa>. No sigue symlinks (tampoco si dir lo es) ni entra en .git.
func treeSources(prefix, dir string, class snapshot.Class) ([]snapshot.Source, error) {
	fi, err := os.Lstat(dir)
	if errors.Is(err, fs.ErrNotExist) || (err == nil && !fi.IsDir()) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("no se pudo inspeccionar %s: %w", dir, err)
	}
	var out []snapshot.Source
	err = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		ss, err := fileSource(prefix+"/"+filepath.ToSlash(rel), p, class, nil)
		out = append(out, ss...)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("no se pudo recorrer %s: %w", dir, err)
	}
	return out, nil
}

// claudeJSONSource es la fuente de la parte de configuración de un
// .claude.json. Se marca con meta merge=claude-json: al restaurar se fusiona.
func claudeJSONSource(lpath, abs string) ([]snapshot.Source, error) {
	data, err := os.ReadFile(abs)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("no se pudo leer %s: %w", abs, err)
	}
	sub, err := ClaudeJSONConfig(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", abs, err)
	}
	if sub == nil {
		return nil, nil
	}
	return []snapshot.Source{{
		LPath: lpath, Class: snapshot.ClassSecret, Mode: 0o600,
		Meta: map[string]string{"merge": "claude-json"},
		Read: func() ([]byte, error) { return sub, nil },
	}}, nil
}

// projectSources recorre las carpetas con regla y captura sus archivos locales.
// La clave del proyecto sale del remoto de git (el mismo repo en otra ruta u
// otra máquina es el mismo proyecto) o, sin remoto, de la ruta. Si dos clones
// del mismo remoto tienen regla, el segundo se identifica por su ruta para que
// no choquen.
func projectSources(rules []Rule) ([]snapshot.Source, error) {
	var out []snapshot.Source
	used := map[string]string{} // clave -> ruta que la usa
	for _, r := range rules {
		remote := gitOriginURL(r.Path)
		key := projectKey(r.Path, remote)
		if owner, taken := used[key]; taken && owner != r.Path {
			key = projectKey(r.Path, "")
		}
		if _, dup := used[key]; dup {
			continue
		}
		used[key] = r.Path
		meta := map[string]string{"path": r.Path}
		if remote != "" {
			meta["remote"] = remote
		}
		for _, rel := range projectLocalFiles {
			ss, err := fileSource("project/"+key+"/"+rel, filepath.Join(r.Path, filepath.FromSlash(rel)), snapshot.ClassAuthored, meta)
			if err != nil {
				return nil, err
			}
			out = append(out, ss...)
		}
	}
	return out, nil
}

// projectKey es la identidad portable de un proyecto: 12 hex del sha256 de su
// remoto normalizado, o de su ruta si no tiene.
func projectKey(path, remote string) string {
	id := path
	if remote != "" {
		id = normalizeRemote(remote)
	}
	sum := sha256.Sum256([]byte(id))
	return hex.EncodeToString(sum[:])[:12]
}

// normalizeRemote hace equivalentes las formas de escribir un mismo remoto:
// git@github.com:Org/App.git, https://github.com/org/app y
// ssh://git@github.com/Org/App.git son github.com/org/app.
func normalizeRemote(u string) string {
	u = strings.ToLower(strings.TrimSpace(u))
	u = strings.TrimSuffix(u, ".git")
	for _, p := range []string{"https://", "http://", "ssh://", "git://"} {
		u = strings.TrimPrefix(u, p)
	}
	if at := strings.Index(u, "@"); at >= 0 {
		u = u[at+1:]
	}
	return strings.Replace(u, ":", "/", 1)
}

// gitOriginURL lee la url de «origin» de <dir>/.git/config sin ejecutar git.
// Si dir no es la raíz de un repo (o es un worktree, con .git como archivo),
// devuelve "" y el proyecto se identifica por su ruta.
func gitOriginURL(dir string) string {
	f, err := os.Open(filepath.Join(dir, ".git", "config"))
	if err != nil {
		return ""
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	inOrigin := false
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, "[") {
			inOrigin = line == `[remote "origin"]`
			continue
		}
		if !inOrigin {
			continue
		}
		if k, v, ok := strings.Cut(line, "="); ok && strings.TrimSpace(k) == "url" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// SnapshotCaptureOpts son las opciones de una captura.
type SnapshotCaptureOpts struct {
	Trigger   string // manual | daily | pre-restore | pre-profile-rm | pre-backup-restore | …
	Label     string
	WithState bool
	Force     bool // captura aunque no haya cambios
	Now       time.Time
	Machine   string
}

// SnapshotCapture toma un snapshot del estado vivo. Si no cambió nada desde el
// último devuelve ese último y snapshot.ErrNoChanges (salvo Force).
func SnapshotCapture(home, src string, st *snapshot.Store, o SnapshotCaptureOpts) (*snapshot.Manifest, error) {
	srcs, err := SnapshotSources(home, src, SnapshotSourceOpts{WithState: o.WithState})
	if err != nil {
		return nil, err
	}
	// El HOME del usuario (no el de ccp): con él, otra máquina sabe qué prefijo
	// de las rutas absolutas tiene que traducir.
	userHome, _ := os.UserHomeDir()
	return snapshot.Capture(st, srcs, snapshot.Meta{
		Created: o.Now, Machine: o.Machine, Home: userHome, CCPVersion: Version, Trigger: o.Trigger, Label: o.Label,
	}, o.Force)
}

// SnapshotLive calcula los elementos del estado vivo sin guardar nada.
func SnapshotLive(home, src string, o SnapshotSourceOpts) ([]snapshot.Item, error) {
	srcs, err := SnapshotSources(home, src, o)
	if err != nil {
		return nil, err
	}
	return snapshot.ItemsOf(srcs)
}
```

- [ ] **Step 4: Comprobar que pasa**

Run: `go test ./internal/core/ -run 'SnapshotSources|ProjectKey|SnapshotCapture' && go vet ./internal/core/`
Expected: PASS.

Si `ProfileAddOfficial` del fixture falla porque el `~/.claude` falso aún no
tiene `commands/` o `plugins/`, crea esos directorios en `snapFixture` antes de
añadir el perfil, como hace `homeConPerfil` en `internal/cli/desktop_test.go`.

- [ ] **Step 5: Gates y commit**

Mensaje propuesto: `feat(core): qué captura un snapshot en esta máquina`

---

### Task 9: `core` — restaurar un snapshot

Restaurar sigue siempre los mismos pasos:
1. **Plan.** Qué se escribe, qué se fusiona, qué ya está igual y qué no se puede
   restaurar (y por qué).
2. **Snapshot previo del estado actual.** Es la red para deshacer, y si no se
   puede tomar no se restaura nada.
3. **Escritura.** `ccp.yaml` primero, validado. Las partes de `.claude.json` se
   fusionan; el resto se escribe con tmp+rename.
4. **Regenerar los perfiles afectados** (`seedCCHome` + `CfgRegenerate`). Es lo
   que hoy le falta a `ccp backup restore` (spec B2).

No se borra nada que exista en vivo y no esté en el snapshot. El plan de la nube
añade el mapeo de rutas entre máquinas; aquí un proyecto cuya carpeta no existe
se omite con motivo `project_missing`.

**Files:**
- Create: `internal/core/snapshot_restore.go`
- Create: `internal/core/snapshot_restore_test.go`

**Interfaces:**
- Consumes:
  - de Task 8: `SnapshotCapture`;
  - de core: `configFromBytes` (`backup.go`), `acquireLock` (`store.go`),
    `writeFileAtomic` (`transcript.go`), `seedCCHome`, `CfgRegenerate`,
    `validProfileName` (`profile_rename.go`), `ClaudeJSONConfig`,
    `ClaudeJSONApplyConfig`;
  - de snapshot: `(*Store).LoadManifest`, `(*Store).GetBlob`.
- Produces:
  - `core.SnapshotRestoreOpts{Only []string; DryRun bool; Now time.Time; Machine string}`;
  - `core.SnapshotRestoreStep{LPath, Action, Reason string}`: `Action` es
    `write` | `merge` | `same` | `skip`; `Reason` es `missing_blob` |
    `project_missing` | `invalid` | `unreadable`;
  - `core.SnapshotRestoreReport{Snapshot, PreSnapshot string; Steps []SnapshotRestoreStep; Regenerated []string}`;
  - `core.SnapshotRestore(home, src string, st *snapshot.Store, ref string, o SnapshotRestoreOpts) (*SnapshotRestoreReport, error)`.

- [ ] **Step 1: Escribir los tests**

```go
package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/JoseAFlores777/ccp/internal/snapshot"
)

var snapNow = time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)

func captureFixture(t *testing.T) (home, src, repo string, st *snapshot.Store, m *snapshot.Manifest) {
	t.Helper()
	home, src, repo = snapFixture(t)
	st, err := OpenSnapshotStore(home)
	if err != nil {
		t.Fatal(err)
	}
	m, err = SnapshotCapture(home, src, st, SnapshotCaptureOpts{Trigger: "manual", Now: snapNow, Machine: "test"})
	if err != nil {
		t.Fatal(err)
	}
	return home, src, repo, st, m
}

func readStr(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("leer %s: %v", p, err)
	}
	return string(b)
}

func actions(rep *SnapshotRestoreReport) map[string]string {
	out := map[string]string{}
	for _, s := range rep.Steps {
		out[s.LPath] = s.Action
		if s.Reason != "" {
			out[s.LPath] += ":" + s.Reason
		}
	}
	return out
}

func TestSnapshotRestoreRoundTrip(t *testing.T) {
	home, src, _, st, m := captureFixture(t)
	os.WriteFile(cfgInstrFile(home, "work"), []byte("# roto\n"), 0o644)
	os.Remove(filepath.Join(src, "agents", "revisor.md"))
	os.WriteFile(src+".json", []byte(`{"machineID":"m-999","mcpServers":{}}`), 0o600)

	plan, err := SnapshotRestore(home, src, st, m.ID, SnapshotRestoreOpts{DryRun: true, Now: snapNow.Add(time.Hour), Machine: "test"})
	if err != nil {
		t.Fatalf("dry-run: %v", err)
	}
	if got := readStr(t, cfgInstrFile(home, "work")); got != "# roto\n" {
		t.Fatal("el dry-run escribió")
	}
	a := actions(plan)
	for l, want := range map[string]string{
		"ccp/profiles/work/overlay/CLAUDE.md": "write",
		"claude/agents/revisor.md":            "write",
		"claude/.claude.json":                 "merge",
		"claude/settings.json":                "same",
	} {
		if a[l] != want {
			t.Errorf("plan[%s] = %q, quiero %q", l, a[l], want)
		}
	}
	if plan.PreSnapshot != "" {
		t.Error("un dry-run no toma snapshot previo")
	}

	rep, err := SnapshotRestore(home, src, st, m.ID, SnapshotRestoreOpts{Now: snapNow.Add(time.Hour), Machine: "test"})
	if err != nil {
		t.Fatalf("restore: %v", err)
	}
	if got := readStr(t, cfgInstrFile(home, "work")); got != "# work\n" {
		t.Errorf("overlay = %q", got)
	}
	if got := readStr(t, filepath.Join(src, "agents", "revisor.md")); got != "# revisor\n" {
		t.Errorf("agente = %q", got)
	}
	var cj map[string]any
	json.Unmarshal([]byte(readStr(t, src+".json")), &cj)
	if cj["machineID"] != "m-999" || cj["mcpServers"].(map[string]any)["github"] == nil {
		t.Errorf("~/.claude.json fusionado mal: %v", cj)
	}
	// La red: el estado de antes quedó en un snapshot nuevo.
	if rep.PreSnapshot == "" || rep.PreSnapshot == m.ID {
		t.Fatalf("PreSnapshot = %q", rep.PreSnapshot)
	}
	pre, err := st.LoadManifest(rep.PreSnapshot)
	if err != nil || pre.Trigger != "pre-restore" {
		t.Fatalf("snapshot previo = %+v, %v", pre, err)
	}
	// Se tocó ~/.claude: se regeneran todos los perfiles.
	if !slices.Equal(rep.Regenerated, []string{"deep", "work"}) {
		t.Errorf("Regenerated = %v", rep.Regenerated)
	}
	if _, err := os.Stat(filepath.Join(ccHomePath(home, "work"), "CLAUDE.md")); err != nil {
		t.Errorf("no se regeneró cc-home/CLAUDE.md: %v", err)
	}
}

func TestSnapshotRestoreOnly(t *testing.T) {
	home, src, _, st, m := captureFixture(t)
	os.WriteFile(cfgInstrFile(home, "work"), []byte("# roto\n"), 0o644)
	os.Remove(filepath.Join(src, "agents", "revisor.md"))

	rep, err := SnapshotRestore(home, src, st, m.ID, SnapshotRestoreOpts{Only: []string{"claude/agents"}, Now: snapNow.Add(time.Hour), Machine: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if got := readStr(t, cfgInstrFile(home, "work")); got != "# roto\n" {
		t.Error("--only restauró algo fuera del filtro")
	}
	if _, err := os.Stat(filepath.Join(src, "agents", "revisor.md")); err != nil {
		t.Error("no restauró lo que pedía el filtro")
	}
	for _, s := range rep.Steps {
		if !strings.HasPrefix(s.LPath, "claude/agents/") {
			t.Errorf("paso fuera del filtro: %s", s.LPath)
		}
	}
	if _, err := SnapshotRestore(home, src, st, m.ID, SnapshotRestoreOpts{Only: []string{"no/existe"}, DryRun: true}); err == nil {
		t.Error("un filtro que no coincide con nada debe ser un error")
	}
}

func TestSnapshotRestoreSkipsMissingProject(t *testing.T) {
	home, src, repo, st, m := captureFixture(t)
	os.RemoveAll(repo)
	rep, err := SnapshotRestore(home, src, st, m.ID, SnapshotRestoreOpts{DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	key := projectKey(repo, "git@github.com:Org/App.git")
	if got := actions(rep)["project/"+key+"/.claude/settings.local.json"]; got != "skip:project_missing" {
		t.Fatalf("proyecto borrado = %q", got)
	}
}

// Un ccp.yaml de un ccp más nuevo no se restaura: se aborta y no se escribe nada.
func TestSnapshotRestoreRefusesNewerSchema(t *testing.T) {
	home, src, _ := snapFixture(t)
	st, _ := OpenSnapshotStore(home)
	h, _ := st.PutBlob([]byte("version: 99\nprofiles: {}\n"), false)
	m := &snapshot.Manifest{Format: snapshot.FormatVersion, Created: snapNow, Machine: "x", CCPVersion: "9.0.0", Trigger: "manual",
		Items: []snapshot.Item{{LPath: "ccp/ccp.yaml", Hash: h, Size: 1, Mode: 0o644, Class: snapshot.ClassAuthored}}}
	if err := st.SaveManifest(m); err != nil {
		t.Fatal(err)
	}
	before := readStr(t, yamlPath(home))
	if _, err := SnapshotRestore(home, src, st, m.ID, SnapshotRestoreOpts{Now: snapNow.Add(time.Hour), Machine: "t"}); err == nil {
		t.Fatal("restauró un ccp.yaml de schema 99")
	}
	if readStr(t, yamlPath(home)) != before {
		t.Fatal("ccp.yaml cambió pese al error")
	}
	if list, _ := st.List(); len(list) != 1 {
		t.Fatalf("se tomó un snapshot previo pese a abortar en el plan: %d snapshots", len(list))
	}
}

// Las rutas lógicas llegan también de archivos importados: ninguna puede
// escribir fuera de su sitio.
func TestSnapshotTargetRejectsEscapes(t *testing.T) {
	home, src, repo := snapFixture(t)
	bad := []snapshot.Item{
		{LPath: "ccp/profiles/../x/api_key"},
		{LPath: "desktop/../../x/claude_desktop_config.json"},
		{LPath: "project/k/.ssh/authorized_keys", Meta: map[string]string{"path": repo}},
		{LPath: "project/k/CLAUDE.local.md", Meta: map[string]string{"path": "relativa"}},
		{LPath: "otra/cosa"},
	}
	for _, it := range bad {
		if _, err := snapshotTarget(home, src, it); err == nil {
			t.Errorf("snapshotTarget(%s) sin error", it.LPath)
		}
	}
}
```

- [ ] **Step 2: Comprobar que falla**

Run: `go test ./internal/core/ -run 'SnapshotRestore|SnapshotTarget'`
Expected: FAIL, no compila.

- [ ] **Step 3: Implementar `internal/core/snapshot_restore.go`**

```go
package core

// snapshot_restore.go — el camino de vuelta de snapshot_layout.go: adónde va
// cada ruta lógica y cómo se escribe. Las rutas lógicas pueden venir de un
// .ccpsnap ajeno, así que cada una se valida contra la lista cerrada de
// destinos conocidos antes de tocar el disco.

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/JoseAFlores777/ccp/internal/snapshot"
)

// errProjectMissing: la carpeta del proyecto no existe en esta máquina.
var errProjectMissing = errors.New("la carpeta del proyecto no existe")

// snapTarget es el destino real de un elemento.
type snapTarget struct {
	path    string
	merge   bool   // .claude.json: se fusiona, no se sustituye
	ccpYAML bool   // ccp.yaml: se valida y se escribe bajo el lock
	profile string // perfil que hay que regenerar después, o ""
	global  bool   // toca lo compartido: se regeneran todos los perfiles
}

// snapshotTarget resuelve el destino de it en esta máquina, o error si la ruta
// lógica no es una de las conocidas.
func snapshotTarget(home, src string, it snapshot.Item) (snapTarget, error) {
	bad := fmt.Errorf("ruta lógica desconocida: %s", it.LPath)
	parts := strings.Split(it.LPath, "/")
	rest := func(from int) (string, error) {
		if from >= len(parts) {
			return "", bad
		}
		rel := filepath.FromSlash(strings.Join(parts[from:], "/"))
		if !filepath.IsLocal(rel) {
			return "", bad
		}
		return rel, nil
	}
	switch parts[0] {
	case "ccp":
		switch {
		case len(parts) == 2 && parts[1] == "ccp.yaml":
			return snapTarget{path: yamlPath(home), ccpYAML: true, global: true}, nil
		case len(parts) == 2 && parts[1] == "handoffs.yaml":
			return snapTarget{path: handoffsPath(home)}, nil
		case len(parts) >= 4 && parts[1] == "profiles" && validProfileName(parts[2]):
			n := parts[2]
			switch {
			case parts[3] == "overlay":
				rel, err := rest(4)
				return snapTarget{path: filepath.Join(cfgOverlayDir(home, n), rel), profile: n}, err
			case parts[3] == "api_key" && len(parts) == 4:
				return snapTarget{path: apiKeyPath(home, n), profile: n}, nil
			case parts[3] == "cc-home" && len(parts) == 5 && parts[4] == ".claude.json":
				return snapTarget{path: filepath.Join(ccHomePath(home, n), ".claude.json"), merge: true, profile: n}, nil
			case parts[3] == "cc-home" && len(parts) >= 6 && parts[4] == "projects":
				rel, err := rest(5)
				return snapTarget{path: filepath.Join(ccHomePath(home, n), "projects", rel)}, err
			}
		}
	case "claude":
		if len(parts) == 2 && parts[1] == ".claude.json" {
			return snapTarget{path: src + ".json", merge: true, global: true}, nil
		}
		rel, err := rest(1)
		return snapTarget{path: filepath.Join(src, rel), global: true}, err
	case "desktop":
		if len(parts) == 3 && parts[2] == "claude_desktop_config.json" && validProfileName(parts[1]) {
			dir, err := DesktopUserDataDir(home, parts[1])
			if err != nil {
				return snapTarget{}, err
			}
			return snapTarget{path: filepath.Join(dir, "claude_desktop_config.json")}, nil
		}
	case "project":
		if len(parts) < 3 || !slices.Contains(projectLocalFiles, strings.Join(parts[2:], "/")) {
			return snapTarget{}, bad
		}
		dir := it.Meta["path"]
		if !filepath.IsAbs(dir) {
			return snapTarget{}, fmt.Errorf("%s: el snapshot no dice dónde está el proyecto", it.LPath)
		}
		if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
			return snapTarget{}, errProjectMissing
		}
		return snapTarget{path: filepath.Join(dir, filepath.FromSlash(strings.Join(parts[2:], "/")))}, nil
	}
	return snapTarget{}, bad
}

// SnapshotRestoreOpts son las opciones de una restauración.
type SnapshotRestoreOpts struct {
	Only    []string // prefijos de ruta lógica; vacío = todo
	DryRun  bool
	Now     time.Time
	Machine string
}

// SnapshotRestoreStep es lo que se hace (o no) con un elemento.
type SnapshotRestoreStep struct {
	LPath  string `json:"lpath"`
	Action string `json:"action"`           // write | merge | same | skip
	Reason string `json:"reason,omitempty"` // missing_blob | project_missing | invalid | unreadable
}

// SnapshotRestoreReport resume una restauración (o su plan, en dry-run).
type SnapshotRestoreReport struct {
	Snapshot    string                `json:"snapshot"`
	PreSnapshot string                `json:"pre_snapshot,omitempty"`
	Steps       []SnapshotRestoreStep `json:"steps"`
	Regenerated []string              `json:"regenerated"`
}

func selectItems(items []snapshot.Item, only []string) []snapshot.Item {
	if len(only) == 0 {
		return items
	}
	var out []snapshot.Item
	for _, it := range items {
		for _, p := range only {
			p = strings.TrimSuffix(p, "/")
			if it.LPath == p || strings.HasPrefix(it.LPath, p+"/") {
				out = append(out, it)
				break
			}
		}
	}
	return out
}

// SnapshotRestore devuelve la configuración al estado del snapshot ref.
func SnapshotRestore(home, src string, st *snapshot.Store, ref string, o SnapshotRestoreOpts) (*SnapshotRestoreReport, error) {
	m, err := st.LoadManifest(ref)
	if err != nil {
		return nil, err
	}
	items := selectItems(m.Items, o.Only)
	if len(items) == 0 {
		return nil, fmt.Errorf("ningún elemento del snapshot %s coincide con %s", snapshot.Short(m.ID), strings.Join(o.Only, ", "))
	}

	type pending struct {
		it   snapshot.Item
		tgt  snapTarget
		data []byte
	}
	rep := &SnapshotRestoreReport{Snapshot: m.ID, Steps: []SnapshotRestoreStep{}, Regenerated: []string{}}
	var todo []pending
	for _, it := range items {
		step := SnapshotRestoreStep{LPath: it.LPath}
		tgt, err := snapshotTarget(home, src, it)
		switch {
		case errors.Is(err, errProjectMissing):
			step.Action, step.Reason = "skip", "project_missing"
		case err != nil:
			step.Action, step.Reason = "skip", "invalid"
		default:
			data, gerr := st.GetBlob(it.Hash)
			if gerr != nil {
				step.Action, step.Reason = "skip", "missing_blob"
				break
			}
			// Un ccp.yaml que este binario no puede leer aborta todo el restore
			// antes de tocar nada: restaurar el resto contra el ccp.yaml viejo
			// dejaría perfiles y reglas desparejados.
			if tgt.ccpYAML {
				if err := checkSnapshotCCPYAML(data); err != nil {
					return nil, err
				}
			}
			same, herr := targetHolds(tgt, data)
			switch {
			case herr != nil:
				step.Action, step.Reason = "skip", "unreadable"
			case same:
				step.Action = "same"
			default:
				step.Action = "write"
				if tgt.merge {
					step.Action = "merge"
				}
				todo = append(todo, pending{it, tgt, data})
			}
		}
		rep.Steps = append(rep.Steps, step)
	}
	if o.DryRun || len(todo) == 0 {
		return rep, nil
	}

	// La red: el estado actual queda en un snapshot antes de tocar nada.
	pre, err := SnapshotCapture(home, src, st, SnapshotCaptureOpts{Trigger: "pre-restore", Now: o.Now, Machine: o.Machine})
	if err != nil && !errors.Is(err, snapshot.ErrNoChanges) {
		return nil, fmt.Errorf("no se pudo guardar el estado actual; no se restauró nada: %w", err)
	}
	rep.PreSnapshot = pre.ID

	// ccp.yaml primero: la regeneración de después se lee contra él.
	sort.SliceStable(todo, func(i, j int) bool { return todo[i].tgt.ccpYAML && !todo[j].tgt.ccpYAML })
	regen, all := map[string]bool{}, false
	for _, p := range todo {
		if err := applySnapshotItem(home, p.tgt, p.it, p.data); err != nil {
			return rep, fmt.Errorf("%s: %w (el estado anterior está en el snapshot %s)", p.it.LPath, err, snapshot.Short(pre.ID))
		}
		all = all || p.tgt.global
		if p.tgt.profile != "" {
			regen[p.tgt.profile] = true
		}
	}

	cfg, err := Load(home)
	if err != nil {
		return rep, err
	}
	names := make([]string, 0, len(cfg.Profiles))
	for n := range cfg.Profiles {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		if !all && !regen[n] {
			continue
		}
		if err := seedCCHome(home, n); err != nil {
			return rep, err
		}
		if err := CfgRegenerate(home, n, src); err != nil {
			return rep, err
		}
		rep.Regenerated = append(rep.Regenerated, n)
	}
	return rep, nil
}

// targetHolds dice si el destino ya contiene exactamente data. Para un
// .claude.json se compara solo su parte de configuración.
func targetHolds(tgt snapTarget, data []byte) (bool, error) {
	live, err := os.ReadFile(tgt.path)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !tgt.merge {
		return bytes.Equal(live, data), nil
	}
	sub, err := ClaudeJSONConfig(live)
	if err != nil {
		return false, err
	}
	return bytes.Equal(sub, data), nil
}

// checkSnapshotCCPYAML exige que el ccp.yaml del snapshot parsee y que su schema
// sea uno que este binario conoce.
func checkSnapshotCCPYAML(data []byte) error {
	c, err := configFromBytes(data)
	if err != nil {
		return fmt.Errorf("el ccp.yaml del snapshot no es válido: %w", err)
	}
	if c.Version > SchemaVersion {
		return fmt.Errorf("el ccp.yaml del snapshot usa schema %d y este ccp conoce hasta el %d; actualiza ccp", c.Version, SchemaVersion)
	}
	return nil
}

func applySnapshotItem(home string, tgt snapTarget, it snapshot.Item, data []byte) error {
	switch {
	case tgt.ccpYAML:
		if err := checkSnapshotCCPYAML(data); err != nil {
			return err
		}
		unlock, err := acquireLock(home)
		if err != nil {
			return err
		}
		defer unlock()
		return writeFileAtomic(tgt.path, data, 0o644)
	case tgt.merge:
		live, err := os.ReadFile(tgt.path)
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		out, err := ClaudeJSONApplyConfig(live, data)
		if err != nil {
			return err
		}
		return writeFileAtomic(tgt.path, out, 0o600)
	default:
		mode := os.FileMode(it.Mode) & 0o777
		if mode == 0 {
			mode = 0o644
		}
		if it.Class == snapshot.ClassSecret {
			mode = 0o600
		}
		return writeFileAtomic(tgt.path, data, mode)
	}
}
```

- [ ] **Step 4: Comprobar que pasa**

Run: `go test ./internal/core/ -run 'Snapshot|ClaudeJSON' && go vet ./internal/core/`
Expected: PASS. En `TestSnapshotRestoreRefusesNewerSchema` el error llega en la
planificación (`checkSnapshotCCPYAML`), antes del snapshot previo: no queda
ningún snapshot `pre-restore` en el almacén.

- [ ] **Step 5: Gates y commit**

Mensaje propuesto: `feat(core): restaurar un snapshot con plan, red previa y regeneración`

---

### Task 10: CLI — `ccp snapshot create | list | show | diff`

La puerta de entrada. Esta tarea también deja listos el aislamiento de los tests
del paquete (`TestMain`), el catálogo bilingüe completo y la sección de ayuda.
Las tareas 11 y 12 solo añaden métodos.

**Files:**
- Create: `internal/cli/main_test.go`
- Create: `internal/core/i18n/catalog_snapshot.go`
- Create: `internal/cli/snapshot.go`
- Create: `internal/cli/snapshot_test.go`
- Modify: `internal/cli/cli.go` (`case "snapshot"`)
- Modify: `internal/core/i18n/catalog_cli.go` (sección SNAPSHOTS de `cli.help.body`, En y Es)

**Interfaces:**
- Consumes: `core.OpenSnapshotStore`, `core.SnapshotCapture`, `core.SnapshotLive`, `core.ClaudeSrc`, `snapshot.*`; de `cli`: `ccpHome`, `ensureMigrated`, `currentLang`, `okLine`, `warnLine`, `errLine`, `mute`, `boldLine`, `humanBytes` (`desktop.go:828`), `homeConPerfil` (test, `desktop_test.go:16`).
- Produces:
  - `dispatchSnapshot(args, stdout, stderr) int`, `snapCmd` (y sus métodos),
    `parseSnapArgs`, `snapArgs.val`;
  - `snapSummary` / `snapSummaryOf`, `snapJSON`, `snapMachine`, `countClass`;
  - en tests, `snapEnv` y `snapRun`, que reusan las tareas 11 a 13.

- [ ] **Step 1: Aislar el paquete en los tests — `internal/cli/main_test.go`**

```go
package cli

import (
	"os"
	"testing"
)

// TestMain aísla a todo el paquete de lo que el snapshot automático leería fuera
// de CCP_HOME: ~/.claude y la ventana `default` de Desktop. Por defecto el
// snapshot automático va APAGADO en los tests para que los que ya existían no
// cambien de comportamiento; los que lo prueban lo encienden con
// t.Setenv("CCP_NO_AUTO_SNAPSHOT", "").
func TestMain(m *testing.M) {
	os.Setenv("CCP_NO_AUTO_SNAPSHOT", "1")
	var tmp string
	if os.Getenv("CCP_DESKTOP_DEFAULT_DATA_DIR") == "" {
		if d, err := os.MkdirTemp("", "ccp-desktop-default-*"); err == nil {
			tmp = d
			os.Setenv("CCP_DESKTOP_DEFAULT_DATA_DIR", d)
		}
	}
	code := m.Run()
	if tmp != "" {
		os.RemoveAll(tmp)
	}
	os.Exit(code)
}
```

Run: `go test ./internal/cli/`
Expected: PASS. Aún no hay nada nuevo que probar; el `TestMain` no debe romper
ningún test existente.

- [ ] **Step 2: Escribir los tests de esta tarea — `internal/cli/snapshot_test.go`**

```go
package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// snapEnv monta un CCP_HOME con el perfil official «work» y un ~/.claude falso
// con un settings.json. homeConPerfil fija CCP_HOME y CCP_CLAUDE_SRC; HOME y la
// ventana default de Desktop también son temporales.
func snapEnv(t *testing.T) (home, src string) {
	t.Helper()
	home = homeConPerfil(t, "work", "official")
	src = os.Getenv("CCP_CLAUDE_SRC")
	t.Setenv("HOME", t.TempDir())
	t.Setenv("CCP_DESKTOP_DEFAULT_DATA_DIR", t.TempDir())
	t.Setenv("CCP_LANG", "es")
	if err := os.WriteFile(filepath.Join(src, "settings.json"), []byte(`{"theme":"dark"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	return home, src
}

func snapRun(t *testing.T, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errb bytes.Buffer
	code = Dispatch(args, &out, &errb)
	return code, out.String(), errb.String()
}

func TestSnapshotCreateListShow(t *testing.T) {
	snapEnv(t)
	code, out, errs := snapRun(t, "snapshot", "create")
	if code != 0 || !strings.Contains(out, "guardado") {
		t.Fatalf("create: %d %q %q", code, out, errs)
	}
	if code, out, _ = snapRun(t, "snapshot", "create"); code != 0 || !strings.Contains(out, "Sin cambios") {
		t.Fatalf("segundo create: %d %q", code, out)
	}
	code, out, _ = snapRun(t, "snapshot", "list", "--json")
	var list []snapSummary
	if code != 0 || json.Unmarshal([]byte(out), &list) != nil || len(list) != 1 || list[0].Trigger != "manual" {
		t.Fatalf("list --json: %d %q", code, out)
	}
	if code, out, _ = snapRun(t, "snapshot", "show", list[0].ID[:8], "--json"); code != 0 || !strings.Contains(out, `"claude/settings.json"`) {
		t.Fatalf("show: %d %q", code, out)
	}
	if code, out, _ = snapRun(t, "snapshot", "list"); code != 0 || !strings.Contains(out, list[0].ID[:12]) {
		t.Fatalf("list en texto: %d %q", code, out)
	}
	// Con etiqueta se guarda aunque no haya cambios: quien etiqueta quiere ese punto.
	if code, out, _ = snapRun(t, "snapshot", "create", "-m", "antes de migrar"); code != 0 || !strings.Contains(out, "guardado") {
		t.Fatalf("create -m: %d %q", code, out)
	}
}

func TestSnapshotListEmptyIsArray(t *testing.T) {
	snapEnv(t)
	if code, out, _ := snapRun(t, "snapshot", "list", "--json"); code != 0 || strings.TrimSpace(out) != "[]" {
		t.Fatalf("list --json vacío = %d %q", code, out)
	}
}

func TestSnapshotDiffAgainstLive(t *testing.T) {
	_, src := snapEnv(t)
	snapRun(t, "snapshot", "create")
	os.WriteFile(filepath.Join(src, "settings.json"), []byte(`{"theme":"light"}`), 0o644)
	code, out, _ := snapRun(t, "snapshot", "diff", "latest")
	if code != 0 || !strings.Contains(out, "~ claude/settings.json") {
		t.Fatalf("diff: %d %q", code, out)
	}
}

func TestSnapshotBadInput(t *testing.T) {
	snapEnv(t)
	for _, args := range [][]string{
		{"snapshot", "nope"},
		{"snapshot", "create", "--nope"},
		{"snapshot", "show"},
		{"snapshot", "show", "zz"},
	} {
		if code, _, _ := snapRun(t, args...); code != 1 {
			t.Errorf("%v: código %d, quiero 1", args, code)
		}
	}
}
```

- [ ] **Step 3: Comprobar que falla**

Run: `go test ./internal/cli/ -run Snapshot`
Expected: FAIL, no compila (`undefined: snapSummary`).

- [ ] **Step 4: El catálogo — `internal/core/i18n/catalog_snapshot.go`**

Tiene todas las claves del plan; las tareas 11 y 12 usan las que aquí aún no se usan.

```go
package i18n

// catalog_snapshot.go — prosa de `ccp snapshot` y del snapshot automático
// (internal/cli/snapshot.go). Prefijo `cli.snapshot.` y nada más.

func init() { register(catalogSnapshot) }

var catalogSnapshot = map[string]map[Lang]string{
	"cli.snapshot.usage": {
		En: `Usage: ccp snapshot <subcommand>

  create [-m <label>] [--with-state] [--json]    take a snapshot of the whole configuration
  list [--json]                                  history, newest first
  show <id> [--json]                             what a snapshot contains
  diff <id> [<id>] [--json]                      changes from a snapshot to another (or to now)
  restore <id> [--only <path>]... [--dry-run | --yes] [--json]
                                                 bring the configuration back to a snapshot
  pin <id> [-m <label>] | unpin <id>             keep a snapshot forever (or stop keeping it)
  prune [--dry-run] [--json]                     drop old snapshots (keeps 7 days, 4 weeks, 6 months)
  export <id> <file.ccpsnap> [--with-secrets]    one portable file (secrets need a passphrase)
  import <file.ccpsnap>                          add an exported snapshot to this machine

<id> is "latest", a full id or at least its first 4 characters.
--with-state also captures conversations and loans (large and sensitive).
CCP_SNAPSHOT_PASSPHRASE gives the passphrase without asking.`,
		Es: `Uso: ccp snapshot <subcomando>

  create [-m <etiqueta>] [--with-state] [--json] guarda un snapshot de toda la configuración
  list [--json]                                  el histórico, del más nuevo al más viejo
  show <id> [--json]                             qué contiene un snapshot
  diff <id> [<id>] [--json]                      cambios de un snapshot a otro (o a ahora)
  restore <id> [--only <ruta>]... [--dry-run | --yes] [--json]
                                                 devuelve la configuración a un snapshot
  pin <id> [-m <etiqueta>] | unpin <id>          conserva un snapshot para siempre (o deja de hacerlo)
  prune [--dry-run] [--json]                     borra los viejos (conserva 7 días, 4 semanas, 6 meses)
  export <id> <archivo.ccpsnap> [--with-secrets] un solo archivo portable (los secretos piden una frase)
  import <archivo.ccpsnap>                       añade a esta máquina un snapshot exportado

<id> es «latest», un id completo o al menos sus 4 primeros caracteres.
--with-state captura también conversaciones y préstamos (pesan y son sensibles).
CCP_SNAPSHOT_PASSPHRASE da la frase sin preguntarla.`,
	},
	"cli.snapshot.unknown_sub": {En: "snapshot: unknown subcommand '%s'", Es: "snapshot: subcomando desconocido '%s'"},
	"cli.snapshot.unknown_opt": {En: "snapshot: unknown option or extra argument '%s'", Es: "snapshot: opción desconocida o argumento de más '%s'"},
	"cli.snapshot.need_id":     {En: "snapshot: this needs a snapshot id (or «latest»)", Es: "snapshot: hace falta el id de un snapshot (o «latest»)"},
	"cli.snapshot.need_file":   {En: "snapshot: this needs a file", Es: "snapshot: hace falta un archivo"},
	"cli.snapshot.created": {
		En: "Snapshot %s saved (%d items, %d secrets sealed).",
		Es: "Snapshot %s guardado (%d elementos, %d secretos sellados).",
	},
	"cli.snapshot.unchanged": {
		En: "No changes: the latest snapshot (%s) already matches your configuration.",
		Es: "Sin cambios: el último snapshot (%s) ya refleja tu configuración.",
	},
	"cli.snapshot.list_empty":  {En: "No snapshots yet. Take one with: ccp snapshot create", Es: "Aún no hay snapshots. Toma uno con: ccp snapshot create"},
	"cli.snapshot.list_header": {En: "ID\tDATE\tTRIGGER\tITEMS\tLABEL", Es: "ID\tFECHA\tDISPARADOR\tELEMENTOS\tETIQUETA"},
	"cli.snapshot.pinned_mark": {En: "[pinned]", Es: "[fijado]"},
	"cli.snapshot.show_header": {En: "Snapshot %s · %s · %s · %s", Es: "Snapshot %s · %s · %s · %s"},
	"cli.snapshot.show_parent": {En: "after %s", Es: "después de %s"},
	"cli.snapshot.show_label":  {En: "label: %s", Es: "etiqueta: %s"},
	"cli.snapshot.class_authored": {En: "config", Es: "config"},
	"cli.snapshot.class_secret":   {En: "secret", Es: "secreto"},
	"cli.snapshot.class_state":    {En: "state", Es: "estado"},
	"cli.snapshot.diff_none":      {En: "No differences.", Es: "Sin diferencias."},

	"cli.snapshot.restore_plan": {En: "Restore plan for %s:", Es: "Plan para restaurar %s:"},
	"cli.snapshot.act_write":    {En: "write", Es: "escribir"},
	"cli.snapshot.act_merge":    {En: "merge", Es: "fusionar"},
	"cli.snapshot.act_skip":     {En: "skip", Es: "omitir"},
	"cli.snapshot.reason_missing_blob": {
		En: "the snapshot has no data for it (exported without secrets?)",
		Es: "el snapshot no tiene sus datos (¿se exportó sin secretos?)",
	},
	"cli.snapshot.reason_project_missing": {
		En: "the project folder does not exist on this machine",
		Es: "la carpeta del proyecto no existe en esta máquina",
	},
	"cli.snapshot.reason_invalid":    {En: "not a path ccp knows how to restore", Es: "no es una ruta que ccp sepa restaurar"},
	"cli.snapshot.reason_unreadable": {En: "the current file could not be read", Es: "no se pudo leer el archivo actual"},
	"cli.snapshot.restore_same":      {En: "%d already identical.", Es: "%d ya idénticos."},
	"cli.snapshot.restore_nothing": {
		En: "Nothing to restore: everything already matches the snapshot.",
		Es: "Nada que restaurar: todo coincide ya con el snapshot.",
	},
	"cli.snapshot.restore_confirm": {
		En: "Nothing was changed. Run it again with --yes to apply it (or --dry-run to just see it).",
		Es: "No se cambió nada. Ejecútalo de nuevo con --yes para aplicarlo (o con --dry-run para solo verlo).",
	},
	"cli.snapshot.restored": {
		En: "Restored. Your previous configuration is in snapshot %s.",
		Es: "Restaurado. La configuración anterior quedó en el snapshot %s.",
	},
	"cli.snapshot.regenerated": {En: "Profiles regenerated: %s", Es: "Perfiles regenerados: %s"},

	"cli.snapshot.pruned": {
		En: "Deleted %d snapshots and %d unused blobs; %d kept.",
		Es: "Borrados %d snapshots y %d blobs sin uso; se conservan %d.",
	},
	"cli.snapshot.prune_dry": {
		En: "Would delete %d snapshots and %d unused blobs; %d would be kept.",
		Es: "Se borrarían %d snapshots y %d blobs sin uso; se conservarían %d.",
	},
	"cli.snapshot.pinned":   {En: "Snapshot %s pinned: prune will never delete it.", Es: "Snapshot %s fijado: prune nunca lo borrará."},
	"cli.snapshot.unpinned": {En: "Snapshot %s unpinned.", Es: "Snapshot %s ya no está fijado."},

	"cli.snapshot.exported": {En: "Snapshot %s exported to %s (without secrets).", Es: "Snapshot %s exportado a %s (sin secretos)."},
	"cli.snapshot.exported_secrets": {
		En: "Snapshot %s exported to %s; secrets sealed with your passphrase.",
		Es: "Snapshot %s exportado a %s; los secretos van sellados con tu frase.",
	},
	"cli.snapshot.imported": {En: "Snapshot %s imported (%d items).", Es: "Snapshot %s importado (%d elementos)."},
	"cli.snapshot.imported_missing": {
		En: "%d items came without data (the file was exported without secrets); they cannot be restored.",
		Es: "%d elementos llegaron sin datos (el archivo se exportó sin secretos); no se podrán restaurar.",
	},
	"cli.snapshot.pass_prompt":   {En: "Passphrase: ", Es: "Frase: "},
	"cli.snapshot.pass_confirm":  {En: "Repeat it: ", Es: "Repítela: "},
	"cli.snapshot.pass_mismatch": {En: "the passphrases do not match", Es: "las frases no coinciden"},
	"cli.snapshot.pass_short":    {En: "the passphrase needs at least %d characters", Es: "la frase necesita al menos %d caracteres"},
	"cli.snapshot.pass_needed": {
		En: "a passphrase is needed: set CCP_SNAPSHOT_PASSPHRASE or run it in a terminal",
		Es: "hace falta una frase: define CCP_SNAPSHOT_PASSPHRASE o ejecútalo en una terminal",
	},

	"cli.snapshot.auto_saved": {En: "Safety snapshot: %s", Es: "Snapshot de seguridad: %s"},
	"cli.snapshot.auto_failed": {
		En: "Could not save the safety snapshot, so nothing was done: %v (CCP_NO_AUTO_SNAPSHOT=1 skips it)",
		Es: "No se pudo guardar el snapshot de seguridad y no se hizo nada: %v (CCP_NO_AUTO_SNAPSHOT=1 lo salta)",
	},
}
```

- [ ] **Step 5: La ayuda — `internal/core/i18n/catalog_cli.go`**

En la versión inglesa de `cli.help.body`, sustituye el bloque BACKUP:

```
BACKUP
  ccp backup export [file] [--with-secrets]
  ccp backup restore <file> [--overwrite | --force]

SCRIPTING
```

por:

```
BACKUP
  ccp backup export [file] [--with-secrets]
  ccp backup restore <file> [--overwrite | --force]

SNAPSHOTS                         full history of your configuration (ccp snapshot help)
  ccp snapshot create [-m <label>]   save one now (also daily and before risky changes)
  ccp snapshot list | show <id> | diff <id> [<id>]
  ccp snapshot restore <id> [--only <path>] [--dry-run | --yes]
  ccp snapshot export <id> <file> [--with-secrets] | import <file>

SCRIPTING
```

y en la española, el mismo bloque con `[archivo]`/`<archivo>` por:

```
BACKUP
  ccp backup export [archivo] [--with-secrets]
  ccp backup restore <archivo> [--overwrite | --force]

SNAPSHOTS                         histórico completo de tu configuración (ccp snapshot help)
  ccp snapshot create [-m <etiqueta>]  guarda uno ahora (también a diario y antes de cambios con riesgo)
  ccp snapshot list | show <id> | diff <id> [<id>]
  ccp snapshot restore <id> [--only <ruta>] [--dry-run | --yes]
  ccp snapshot export <id> <archivo> [--with-secrets] | import <archivo>

SCRIPTING
```

- [ ] **Step 6: Implementar `internal/cli/snapshot.go`**

```go
package cli

// snapshot.go — `ccp snapshot …` (spec 2026-09-18 §8). La lógica está en
// internal/snapshot (formato y almacén) y core/snapshot_layout.go (qué se
// captura y adónde vuelve); aquí solo se orquesta y se pinta.

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/JoseAFlores777/ccp/internal/core"
	"github.com/JoseAFlores777/ccp/internal/core/i18n"
	"github.com/JoseAFlores777/ccp/internal/snapshot"
)

// dispatchSnapshot maneja `ccp snapshot <subcomando> …`.
func dispatchSnapshot(args []string, stdout, stderr io.Writer) int {
	home, err := ccpHome()
	if err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}
	if err := ensureMigrated(home); err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}
	lang := currentLang()
	sub := ""
	if len(args) > 0 {
		sub, args = args[0], args[1:]
	}
	if sub == "help" || sub == "-h" || sub == "--help" {
		fmt.Fprintln(stdout, i18n.T(lang, "cli.snapshot.usage"))
		return 0
	}
	st, err := core.OpenSnapshotStore(home)
	if err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}
	c := snapCmd{home: home, lang: lang, st: st, out: stdout, err: stderr}
	switch sub {
	case "create":
		return c.create(args)
	case "list", "ls", "":
		return c.list(args)
	case "show":
		return c.show(args)
	case "diff":
		return c.diff(args)
	default:
		return c.usage("cli.snapshot.unknown_sub", sub)
	}
}

type snapCmd struct {
	home string
	lang i18n.Lang
	st   *snapshot.Store
	out  io.Writer
	err  io.Writer
}

func (c snapCmd) fail(err error) int {
	fmt.Fprintf(c.err, "Error: %v\n", err)
	return 1
}

// usage pinta el motivo y la ayuda del subcomando, y devuelve 1.
func (c snapCmd) usage(key string, a ...any) int {
	fmt.Fprintln(c.err, i18n.T(c.lang, key, a...))
	fmt.Fprintln(c.err, i18n.T(c.lang, "cli.snapshot.usage"))
	return 1
}

// snapArgs son los argumentos de un subcomando ya separados.
type snapArgs struct {
	pos   []string
	flags map[string]bool
	vals  map[string][]string
}

// val devuelve el último valor dado a cualquiera de keys («-m» y «--label» son
// la misma opción).
func (a snapArgs) val(keys ...string) string {
	v := ""
	for _, k := range keys {
		if l := a.vals[k]; len(l) > 0 {
			v = l[len(l)-1]
		}
	}
	return v
}

// parseSnapArgs separa posicionales, opciones sin valor (bools) y con valor
// (valued, que pueden repetirse). Si algo no encaja devuelve ok=false y el
// argumento culpable.
func parseSnapArgs(args, bools, valued []string) (a snapArgs, bad string, ok bool) {
	a = snapArgs{flags: map[string]bool{}, vals: map[string][]string{}}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case slices.Contains(bools, arg):
			a.flags[arg] = true
		case slices.Contains(valued, arg):
			if i+1 >= len(args) {
				return a, arg, false
			}
			i++
			a.vals[arg] = append(a.vals[arg], args[i])
		case strings.HasPrefix(arg, "-") && arg != "-":
			return a, arg, false
		default:
			a.pos = append(a.pos, arg)
		}
	}
	return a, "", true
}

// snapSummary es la forma JSON estable de un snapshot en list/create.
type snapSummary struct {
	ID        string `json:"id"`
	Parent    string `json:"parent"`
	Created   string `json:"created"`
	Machine   string `json:"machine"`
	Trigger   string `json:"trigger"`
	Label     string `json:"label"`
	Pinned    bool   `json:"pinned"`
	Items     int    `json:"items"`
	Secrets   int    `json:"secrets"`
	Unchanged bool   `json:"unchanged,omitempty"`
}

func snapSummaryOf(m *snapshot.Manifest, unchanged bool) snapSummary {
	return snapSummary{
		ID: m.ID, Parent: m.Parent, Created: m.Created.UTC().Format(time.RFC3339),
		Machine: m.Machine, Trigger: m.Trigger, Label: m.Label, Pinned: m.Pinned,
		Items: len(m.Items), Secrets: countClass(m, snapshot.ClassSecret), Unchanged: unchanged,
	}
}

func countClass(m *snapshot.Manifest, class snapshot.Class) int {
	n := 0
	for _, it := range m.Items {
		if it.Class == class {
			n++
		}
	}
	return n
}

func snapJSON(stdout, stderr io.Writer, v any) int {
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}
	return 0
}

// snapMachine es el nombre con el que un snapshot recuerda de qué máquina salió.
func snapMachine() string {
	if h, err := os.Hostname(); err == nil && h != "" {
		return h
	}
	return "unknown"
}

func (c snapCmd) create(args []string) int {
	a, bad, ok := parseSnapArgs(args, []string{"--with-state", "--json"}, []string{"-m", "--label"})
	if !ok {
		return c.usage("cli.snapshot.unknown_opt", bad)
	}
	if len(a.pos) > 0 {
		return c.usage("cli.snapshot.unknown_opt", a.pos[0])
	}
	src, err := core.ClaudeSrc()
	if err != nil {
		return c.fail(err)
	}
	label := a.val("-m", "--label")
	m, err := core.SnapshotCapture(c.home, src, c.st, core.SnapshotCaptureOpts{
		Trigger: "manual", Label: label, WithState: a.flags["--with-state"],
		// Quien etiqueta quiere ESE punto aunque no haya cambios.
		Force: label != "", Now: time.Now(), Machine: snapMachine(),
	})
	unchanged := errors.Is(err, snapshot.ErrNoChanges)
	if err != nil && !unchanged {
		return c.fail(err)
	}
	if a.flags["--json"] {
		return snapJSON(c.out, c.err, snapSummaryOf(m, unchanged))
	}
	if unchanged {
		fmt.Fprintln(c.out, i18n.T(c.lang, "cli.snapshot.unchanged", snapshot.Short(m.ID)))
		return 0
	}
	fmt.Fprintln(c.out, okLine(c.out, i18n.T(c.lang, "cli.snapshot.created",
		snapshot.Short(m.ID), len(m.Items), countClass(m, snapshot.ClassSecret))))
	return 0
}

func (c snapCmd) list(args []string) int {
	a, bad, ok := parseSnapArgs(args, []string{"--json"}, nil)
	if !ok {
		return c.usage("cli.snapshot.unknown_opt", bad)
	}
	if len(a.pos) > 0 {
		return c.usage("cli.snapshot.unknown_opt", a.pos[0])
	}
	ms, err := c.st.List()
	if err != nil {
		return c.fail(err)
	}
	if a.flags["--json"] {
		out := make([]snapSummary, 0, len(ms))
		for _, m := range ms {
			out = append(out, snapSummaryOf(m, false))
		}
		return snapJSON(c.out, c.err, out)
	}
	if len(ms) == 0 {
		fmt.Fprintln(c.out, i18n.T(c.lang, "cli.snapshot.list_empty"))
		return 0
	}
	tw := tabwriter.NewWriter(c.out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, i18n.T(c.lang, "cli.snapshot.list_header"))
	for _, m := range ms {
		label := m.Label
		if m.Pinned {
			label = strings.TrimSpace(i18n.T(c.lang, "cli.snapshot.pinned_mark") + " " + label)
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%s\n", snapshot.Short(m.ID),
			m.Created.Local().Format("2006-01-02 15:04"), m.Trigger, len(m.Items), label)
	}
	_ = tw.Flush()
	return 0
}

func (c snapCmd) show(args []string) int {
	a, bad, ok := parseSnapArgs(args, []string{"--json"}, nil)
	if !ok {
		return c.usage("cli.snapshot.unknown_opt", bad)
	}
	if len(a.pos) != 1 {
		return c.usage("cli.snapshot.need_id")
	}
	m, err := c.st.LoadManifest(a.pos[0])
	if err != nil {
		return c.fail(err)
	}
	if a.flags["--json"] {
		return snapJSON(c.out, c.err, m)
	}
	fmt.Fprintln(c.out, boldLine(c.out, i18n.T(c.lang, "cli.snapshot.show_header",
		snapshot.Short(m.ID), m.Created.Local().Format("2006-01-02 15:04"), m.Machine, m.Trigger)))
	if m.Parent != "" {
		fmt.Fprintln(c.out, mute(c.out, i18n.T(c.lang, "cli.snapshot.show_parent", snapshot.Short(m.Parent))))
	}
	if m.Label != "" {
		fmt.Fprintln(c.out, i18n.T(c.lang, "cli.snapshot.show_label", m.Label))
	}
	tw := tabwriter.NewWriter(c.out, 0, 0, 2, ' ', 0)
	for _, it := range m.Items {
		fmt.Fprintf(tw, "  %s\t%s\t%s\n", i18n.T(c.lang, "cli.snapshot.class_"+string(it.Class)), humanBytes(it.Size), it.LPath)
	}
	_ = tw.Flush()
	return 0
}

func (c snapCmd) diff(args []string) int {
	a, bad, ok := parseSnapArgs(args, []string{"--json", "--with-state"}, nil)
	if !ok {
		return c.usage("cli.snapshot.unknown_opt", bad)
	}
	if len(a.pos) < 1 || len(a.pos) > 2 {
		return c.usage("cli.snapshot.need_id")
	}
	from, err := c.st.LoadManifest(a.pos[0])
	if err != nil {
		return c.fail(err)
	}
	var to []snapshot.Item
	if len(a.pos) == 2 {
		m, err := c.st.LoadManifest(a.pos[1])
		if err != nil {
			return c.fail(err)
		}
		to = m.Items
	} else {
		src, err := core.ClaudeSrc()
		if err != nil {
			return c.fail(err)
		}
		// Si el snapshot capturó estado, «ahora» también lo incluye: si no, cada
		// conversación saldría como borrada.
		withState := a.flags["--with-state"] || countClass(from, snapshot.ClassState) > 0
		if to, err = core.SnapshotLive(c.home, src, core.SnapshotSourceOpts{WithState: withState}); err != nil {
			return c.fail(err)
		}
	}
	changes := snapshot.Diff(from.Items, to)
	if a.flags["--json"] {
		out := make([]map[string]string, 0, len(changes))
		for _, ch := range changes {
			out = append(out, map[string]string{"lpath": ch.LPath, "kind": string(ch.Kind)})
		}
		return snapJSON(c.out, c.err, out)
	}
	if len(changes) == 0 {
		fmt.Fprintln(c.out, i18n.T(c.lang, "cli.snapshot.diff_none"))
		return 0
	}
	marks := map[snapshot.ChangeKind]string{snapshot.ChangeAdded: "+", snapshot.ChangeRemoved: "-", snapshot.ChangeModified: "~"}
	for _, ch := range changes {
		fmt.Fprintf(c.out, "%s %s\n", marks[ch.Kind], ch.LPath)
	}
	return 0
}
```

- [ ] **Step 7: Engancharlo en `internal/cli/cli.go`**

En `Dispatch`, junto a `case "backup":`:

```go
	case "backup":
		return dispatchBackup(rest, stdout, stderr)
	// `snapshot` no muta el entorno del shell padre: entra por el `*)` del rc sin
	// reinstalarlo. No está en la completion, igual que backup (el texto de la
	// completion es contrato golden).
	case "snapshot":
		return dispatchSnapshot(rest, stdout, stderr)
```

- [ ] **Step 8: Comprobar que pasa**

Run: `go test ./internal/cli/ -run Snapshot && go test ./internal/core/i18n/ && go vet ./internal/cli/`
Expected: PASS. `TestCatalogComplete` sigue verde.

- [ ] **Step 9: Gates y commit**

Además de los gates, `go test ./internal/golden/` debe seguir verde.
Mensaje propuesto: `feat(cli): ccp snapshot create, list, show y diff`

---

### Task 11: CLI — `restore | prune | pin | unpin | export | import`

`restore` sin `--yes` **enseña el plan y no toca nada**: sale con 1 y dice cómo
aplicarlo. Con `--dry-run` enseña el plan y sale con 0. Solo `--yes` aplica. Es
la misma regla que `desktop rm --yes`: lo irreversible se pide explícitamente.

**Files:**
- Modify: `internal/cli/snapshot.go`
- Modify: `internal/cli/snapshot_test.go`

**Interfaces:**
- Consumes: `core.SnapshotRestore` (Task 9), `snapshot.Prune`/`DefaultPolicy` (Task 5), `snapshot.Export`/`Import` (Task 6), `(*Store).SetPin` (Task 3), `promptSecret` (`key.go`), `isatty`.
- Produces: métodos `restore`, `prune`, `pin`, `export`, `importArchive` de
  `snapCmd`; `readSnapPassphrase`, `snapMinPassphrase`, `planWrites`,
  `(snapCmd).printPlan`.

- [ ] **Step 1: Escribir los tests (añadir a `snapshot_test.go`)**

```go
func TestSnapshotRestoreNeedsYes(t *testing.T) {
	_, src := snapEnv(t)
	settings := filepath.Join(src, "settings.json")
	snapRun(t, "snapshot", "create")
	_, out, _ := snapRun(t, "snapshot", "list", "--json")
	var list []snapSummary
	json.Unmarshal([]byte(out), &list)
	// El id concreto y no «latest»: tras restaurar, el snapshot de seguridad
	// pasa a ser el más reciente.
	id := list[0].ID[:12]
	os.WriteFile(settings, []byte(`{"theme":"light"}`), 0o644)

	code, out, errs := snapRun(t, "snapshot", "restore", id)
	if code != 1 || !strings.Contains(out, "claude/settings.json") || !strings.Contains(errs, "--yes") {
		t.Fatalf("sin --yes: %d %q %q", code, out, errs)
	}
	if b, _ := os.ReadFile(settings); string(b) != `{"theme":"light"}` {
		t.Fatal("sin --yes se escribió")
	}
	if code, _, _ = snapRun(t, "snapshot", "restore", id, "--dry-run"); code != 0 {
		t.Fatalf("--dry-run: %d", code)
	}
	code, out, errs = snapRun(t, "snapshot", "restore", id, "--yes")
	if code != 0 || !strings.Contains(out, "Restaurado") {
		t.Fatalf("--yes: %d %q %q", code, out, errs)
	}
	if b, _ := os.ReadFile(settings); string(b) != `{"theme":"dark"}` {
		t.Fatalf("settings.json = %s", b)
	}
	// Ya coincide: repetir no hace nada y no es un error.
	if code, out, _ = snapRun(t, "snapshot", "restore", id, "--yes"); code != 0 || !strings.Contains(out, "Nada que restaurar") {
		t.Fatalf("restore repetido: %d %q", code, out)
	}
	// El estado de antes (light) quedó en el snapshot de seguridad, que ahora es el último.
	_, out, _ = snapRun(t, "snapshot", "list", "--json")
	json.Unmarshal([]byte(out), &list)
	if len(list) != 2 || list[0].Trigger != "pre-restore" {
		t.Fatalf("tras restaurar: %q", out)
	}
}

func TestSnapshotPinAndPrune(t *testing.T) {
	snapEnv(t)
	snapRun(t, "snapshot", "create")
	if code, out, _ := snapRun(t, "snapshot", "pin", "latest", "-m", "bueno"); code != 0 || !strings.Contains(out, "fijado") {
		t.Fatalf("pin: %d %q", code, out)
	}
	code, out, _ := snapRun(t, "snapshot", "prune", "--dry-run", "--json")
	var rep struct {
		Kept   int  `json:"kept"`
		DryRun bool `json:"dry_run"`
	}
	if code != 0 || json.Unmarshal([]byte(out), &rep) != nil || rep.Kept != 1 || !rep.DryRun {
		t.Fatalf("prune --dry-run --json: %d %q", code, out)
	}
	if code, out, _ = snapRun(t, "snapshot", "unpin", "latest"); code != 0 || !strings.Contains(out, "ya no está fijado") {
		t.Fatalf("unpin: %d %q", code, out)
	}
}

func TestSnapshotExportImport(t *testing.T) {
	snapEnv(t)
	t.Setenv("CCP_SNAPSHOT_PASSPHRASE", "frase de prueba larga")
	snapRun(t, "snapshot", "create")
	_, out, _ := snapRun(t, "snapshot", "list", "--json")
	var src []snapSummary
	json.Unmarshal([]byte(out), &src)

	file := filepath.Join(t.TempDir(), "config.ccpsnap")
	if code, out, errs := snapRun(t, "snapshot", "export", "latest", file, "--with-secrets"); code != 0 || !strings.Contains(out, "sellados") {
		t.Fatalf("export: %d %q %q", code, out, errs)
	}
	if fi, err := os.Stat(file); err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("un .ccpsnap con secretos debe quedar 0600: %v %v", fi, err)
	}

	// Otra «máquina»: un CCP_HOME vacío.
	t.Setenv("CCP_HOME", t.TempDir())
	if code, out, errs := snapRun(t, "snapshot", "import", file); code != 0 || !strings.Contains(out, "importado") {
		t.Fatalf("import: %d %q %q", code, out, errs)
	}
	_, out, _ = snapRun(t, "snapshot", "list", "--json")
	var dst []snapSummary
	if json.Unmarshal([]byte(out), &dst) != nil || len(dst) != 1 || dst[0].ID != src[0].ID {
		t.Fatalf("tras importar, list = %q; quiero el id %s", out, src[0].ID)
	}
}

// Sin frase no se puede exportar con secretos ni en un pipe ni con una frase corta.
func TestSnapshotExportNeedsPassphrase(t *testing.T) {
	snapEnv(t)
	snapRun(t, "snapshot", "create")
	file := filepath.Join(t.TempDir(), "x.ccpsnap")
	t.Setenv("CCP_SNAPSHOT_PASSPHRASE", "corta")
	if code, _, errs := snapRun(t, "snapshot", "export", "latest", file, "--with-secrets"); code != 1 || !strings.Contains(errs, "12") {
		t.Fatalf("frase corta: %d %q", code, errs)
	}
	if _, err := os.Stat(file); err == nil {
		t.Fatal("se escribió el archivo pese al error")
	}
}
```

- [ ] **Step 2: Comprobar que falla**

Run: `go test ./internal/cli/ -run 'SnapshotRestore|SnapshotPin|SnapshotExport'`
Expected: FAIL (subcomandos desconocidos: código 1 con otro texto).

- [ ] **Step 3: Añadir los casos al `switch` de `dispatchSnapshot`**

```go
	case "restore":
		return c.restore(args)
	case "prune":
		return c.prune(args)
	case "pin":
		return c.pin(args, true)
	case "unpin":
		return c.pin(args, false)
	case "export":
		return c.export(args)
	case "import":
		return c.importArchive(args)
```

- [ ] **Step 4: Implementar los métodos (al final de `snapshot.go`)**

Añade `"github.com/mattn/go-isatty"` a los imports (ya es dependencia directa: la
usa `key.go`).

```go
func (c snapCmd) restore(args []string) int {
	a, bad, ok := parseSnapArgs(args, []string{"--dry-run", "--yes", "--json"}, []string{"--only"})
	if !ok {
		return c.usage("cli.snapshot.unknown_opt", bad)
	}
	if len(a.pos) != 1 {
		return c.usage("cli.snapshot.need_id")
	}
	src, err := core.ClaudeSrc()
	if err != nil {
		return c.fail(err)
	}
	o := core.SnapshotRestoreOpts{Only: a.vals["--only"], DryRun: true, Now: time.Now(), Machine: snapMachine()}
	plan, err := core.SnapshotRestore(c.home, src, c.st, a.pos[0], o)
	if err != nil {
		return c.fail(err)
	}
	if !a.flags["--yes"] || a.flags["--dry-run"] {
		if a.flags["--json"] {
			snapJSON(c.out, c.err, plan)
		} else {
			c.printPlan(plan)
		}
		if a.flags["--dry-run"] || planWrites(plan) == 0 {
			return 0
		}
		if !a.flags["--json"] {
			fmt.Fprintln(c.err, warnLine(c.err, i18n.T(c.lang, "cli.snapshot.restore_confirm")))
		}
		return 1
	}
	if planWrites(plan) == 0 {
		if a.flags["--json"] {
			return snapJSON(c.out, c.err, plan)
		}
		fmt.Fprintln(c.out, i18n.T(c.lang, "cli.snapshot.restore_nothing"))
		return 0
	}
	o.DryRun = false
	rep, err := core.SnapshotRestore(c.home, src, c.st, a.pos[0], o)
	if err != nil {
		return c.fail(err)
	}
	if a.flags["--json"] {
		return snapJSON(c.out, c.err, rep)
	}
	c.printPlan(rep)
	fmt.Fprintln(c.out, okLine(c.out, i18n.T(c.lang, "cli.snapshot.restored", snapshot.Short(rep.PreSnapshot))))
	if len(rep.Regenerated) > 0 {
		fmt.Fprintln(c.out, mute(c.out, i18n.T(c.lang, "cli.snapshot.regenerated", strings.Join(rep.Regenerated, ", "))))
	}
	return 0
}

// planWrites cuenta los pasos que escribirían algo.
func planWrites(r *core.SnapshotRestoreReport) int {
	n := 0
	for _, s := range r.Steps {
		if s.Action == "write" || s.Action == "merge" {
			n++
		}
	}
	return n
}

func (c snapCmd) printPlan(r *core.SnapshotRestoreReport) {
	fmt.Fprintln(c.out, boldLine(c.out, i18n.T(c.lang, "cli.snapshot.restore_plan", snapshot.Short(r.Snapshot))))
	same := 0
	for _, s := range r.Steps {
		switch s.Action {
		case "same":
			same++
		case "write", "merge":
			fmt.Fprintf(c.out, "  %s  %s\n", i18n.T(c.lang, "cli.snapshot.act_"+s.Action), s.LPath)
		case "skip":
			fmt.Fprintf(c.out, "  %s  %s  (%s)\n", i18n.T(c.lang, "cli.snapshot.act_skip"), s.LPath,
				i18n.T(c.lang, "cli.snapshot.reason_"+s.Reason))
		}
	}
	if same > 0 {
		fmt.Fprintln(c.out, mute(c.out, i18n.T(c.lang, "cli.snapshot.restore_same", same)))
	}
}

func (c snapCmd) prune(args []string) int {
	a, bad, ok := parseSnapArgs(args, []string{"--dry-run", "--json"}, nil)
	if !ok {
		return c.usage("cli.snapshot.unknown_opt", bad)
	}
	if len(a.pos) > 0 {
		return c.usage("cli.snapshot.unknown_opt", a.pos[0])
	}
	dry := a.flags["--dry-run"]
	rep, err := snapshot.Prune(c.st, snapshot.DefaultPolicy, time.Now(), time.Hour, dry)
	if err != nil {
		return c.fail(err)
	}
	if a.flags["--json"] {
		deleted := rep.Deleted
		if deleted == nil {
			deleted = []string{}
		}
		return snapJSON(c.out, c.err, map[string]any{
			"deleted": deleted, "kept": rep.Kept, "blobs_deleted": rep.BlobsDeleted, "dry_run": dry,
		})
	}
	key := "cli.snapshot.pruned"
	if dry {
		key = "cli.snapshot.prune_dry"
	}
	fmt.Fprintln(c.out, i18n.T(c.lang, key, len(rep.Deleted), rep.BlobsDeleted, rep.Kept))
	return 0
}

func (c snapCmd) pin(args []string, pinned bool) int {
	a, bad, ok := parseSnapArgs(args, nil, []string{"-m", "--label"})
	if !ok {
		return c.usage("cli.snapshot.unknown_opt", bad)
	}
	if len(a.pos) != 1 {
		return c.usage("cli.snapshot.need_id")
	}
	var label *string
	if l := a.val("-m", "--label"); l != "" {
		label = &l
	}
	m, err := c.st.SetPin(a.pos[0], pinned, label)
	if err != nil {
		return c.fail(err)
	}
	key := "cli.snapshot.pinned"
	if !pinned {
		key = "cli.snapshot.unpinned"
	}
	fmt.Fprintln(c.out, okLine(c.out, i18n.T(c.lang, key, snapshot.Short(m.ID))))
	return 0
}

// snapMinPassphrase: una frase más corta no aguanta un ataque de diccionario
// contra un archivo que puede acabar en cualquier sitio.
const snapMinPassphrase = 12

// readSnapPassphrase pide la frase de un .ccpsnap. CCP_SNAPSHOT_PASSPHRASE la
// da sin preguntar (scripts y tests); si no, se pide por la terminal sin eco,
// como `ccp key`. confirm (al exportar) exige longitud mínima y repetirla.
func readSnapPassphrase(lang i18n.Lang, w io.Writer, confirm bool) ([]byte, error) {
	check := func(p string) error {
		if confirm && len([]rune(p)) < snapMinPassphrase {
			return errors.New(i18n.T(lang, "cli.snapshot.pass_short", snapMinPassphrase))
		}
		return nil
	}
	if p := os.Getenv("CCP_SNAPSHOT_PASSPHRASE"); p != "" {
		return []byte(p), check(p)
	}
	if !isatty.IsTerminal(os.Stdin.Fd()) {
		return nil, errors.New(i18n.T(lang, "cli.snapshot.pass_needed"))
	}
	p1, err := promptSecret(w, i18n.T(lang, "cli.snapshot.pass_prompt"))
	if err != nil {
		return nil, err
	}
	if err := check(p1); err != nil {
		return nil, err
	}
	if confirm {
		p2, err := promptSecret(w, i18n.T(lang, "cli.snapshot.pass_confirm"))
		if err != nil {
			return nil, err
		}
		if p1 != p2 {
			return nil, errors.New(i18n.T(lang, "cli.snapshot.pass_mismatch"))
		}
	}
	return []byte(p1), nil
}

func (c snapCmd) export(args []string) int {
	a, bad, ok := parseSnapArgs(args, []string{"--with-secrets"}, nil)
	if !ok {
		return c.usage("cli.snapshot.unknown_opt", bad)
	}
	if len(a.pos) != 2 {
		return c.usage("cli.snapshot.need_file")
	}
	var pass []byte
	if a.flags["--with-secrets"] {
		p, err := readSnapPassphrase(c.lang, c.err, true)
		if err != nil {
			return c.fail(err)
		}
		pass = p
	}
	dest := a.pos[1]
	perm := os.FileMode(0o644)
	if pass != nil {
		perm = 0o600
	}
	tmp := dest + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, perm)
	if err != nil {
		return c.fail(err)
	}
	m, err := snapshot.Export(c.st, a.pos[0], f, pass)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(tmp, dest)
	}
	if err != nil {
		_ = os.Remove(tmp)
		return c.fail(err)
	}
	key := "cli.snapshot.exported"
	if pass != nil {
		key = "cli.snapshot.exported_secrets"
	}
	fmt.Fprintln(c.out, okLine(c.out, i18n.T(c.lang, key, snapshot.Short(m.ID), dest)))
	return 0
}

func (c snapCmd) importArchive(args []string) int {
	a, bad, ok := parseSnapArgs(args, nil, nil)
	if !ok {
		return c.usage("cli.snapshot.unknown_opt", bad)
	}
	if len(a.pos) != 1 {
		return c.usage("cli.snapshot.need_file")
	}
	f, err := os.Open(a.pos[0])
	if err != nil {
		return c.fail(err)
	}
	defer f.Close()
	rep, err := snapshot.Import(c.st, f, func() ([]byte, error) { return readSnapPassphrase(c.lang, c.err, false) })
	if err != nil {
		return c.fail(err)
	}
	fmt.Fprintln(c.out, okLine(c.out, i18n.T(c.lang, "cli.snapshot.imported", snapshot.Short(rep.Manifest.ID), len(rep.Manifest.Items))))
	if len(rep.Missing) > 0 {
		fmt.Fprintln(c.out, warnLine(c.out, i18n.T(c.lang, "cli.snapshot.imported_missing", len(rep.Missing))))
	}
	return 0
}
```

- [ ] **Step 5: Comprobar que pasa**

Run: `go test ./internal/cli/ -run Snapshot && go vet ./internal/cli/`
Expected: PASS.

- [ ] **Step 6: Gates y commit**

Mensaje propuesto: `feat(cli): ccp snapshot restore, prune, pin, export e import`

---

### Task 12: snapshots automáticos

Dos disparadores (spec §8.3):
- **Antes de lo destructivo**: `profile rm` y `backup restore`, en CLI y en `serve`.
  Si el snapshot no sale, la operación **no sigue**: una red que falla en
  silencio es peor que no tenerla.
- **El diario**: la primera vez que se usa un comando de gestión tras 20 horas.
  Es best-effort: el comando que se pidió nunca falla porque la copia del día no
  salió. Una marca en el almacén (`.daily-check`) evita volver a leer y hashear
  todo en cada comando cuando no hubo cambios.

Los comandos de scripting (`_env`, `_hook`, `resolve`, `status`, `path`,
`completion`) no disparan nada: corren en cada prompt o desde otros programas.
`CCP_NO_AUTO_SNAPSHOT=1` apaga los dos disparadores.

**Files:**
- Modify: `internal/cli/snapshot.go` (`autoSnapshot`, `withSafetySnapshot`, `dailySnapshotCmds`, `maybeDailySnapshot`)
- Modify: `internal/cli/cli.go` (llamada al diario en `Dispatch`; red antes de `backup restore`)
- Modify: `internal/cli/profile.go` (red antes de `profile rm`)
- Modify: `internal/cli/serve_methods.go` (`srvProfilesRemove`), `internal/cli/serve_ops.go` (`srvBackupRestore`)
- Modify: `internal/cli/snapshot_test.go`

**Interfaces:**
- Consumes: `core.SnapshotCapture`, `core.OpenSnapshotStore`, `core.SnapshotStoreDir`, `(*Store).LatestTime`.
- Produces: `autoSnapshot(home, trigger string) (string, error)`,
  `withSafetySnapshot(home, trigger string, lang i18n.Lang, stderr io.Writer) bool`,
  `maybeDailySnapshot(now time.Time)`, `dailySnapshotCmds`, `dailySnapshotEvery`.

- [ ] **Step 1: Escribir los tests (añadir a `snapshot_test.go`)**

Añade `"github.com/JoseAFlores777/ccp/internal/core"` a los imports del test.

```go
func TestProfileRmTakesSafetySnapshot(t *testing.T) {
	home, _ := snapEnv(t)
	t.Setenv("CCP_NO_AUTO_SNAPSHOT", "")
	code, _, errs := snapRun(t, "profile", "rm", "work")
	if code != 0 || !strings.Contains(errs, "Snapshot de seguridad") {
		t.Fatalf("profile rm: %d %q", code, errs)
	}
	st, _ := core.OpenSnapshotStore(home)
	m, err := st.Latest()
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range m.Items {
		if it.LPath == "ccp/ccp.yaml" {
			data, _ := st.GetBlob(it.Hash)
			if !strings.Contains(string(data), "work") {
				t.Fatalf("el snapshot de seguridad no tiene el perfil borrado:\n%s", data)
			}
			return
		}
	}
	t.Fatal("el snapshot de seguridad no tiene ccp.yaml")
}

func TestDailySnapshotOncePerDay(t *testing.T) {
	home, _ := snapEnv(t)
	t.Setenv("CCP_NO_AUTO_SNAPSHOT", "")
	snapRun(t, "profile", "list")
	snapRun(t, "profile", "list")
	st, _ := core.OpenSnapshotStore(home)
	ms, err := st.List()
	if err != nil || len(ms) != 1 || ms[0].Trigger != "daily" {
		t.Fatalf("tras dos comandos: %d snapshots (%v)", len(ms), err)
	}
}

// Un comando de scripting no toca el almacén: ni siquiera lo crea.
func TestScriptingCommandsSkipDailySnapshot(t *testing.T) {
	home, _ := snapEnv(t)
	t.Setenv("CCP_NO_AUTO_SNAPSHOT", "")
	snapRun(t, "status")
	snapRun(t, "resolve", "/")
	if _, err := os.Stat(core.SnapshotStoreDir(home)); err == nil {
		t.Fatal("un comando de scripting creó el almacén de snapshots")
	}
}

func TestAutoSnapshotOffByEnv(t *testing.T) {
	home, _ := snapEnv(t) // TestMain deja CCP_NO_AUTO_SNAPSHOT=1
	if code, _, errs := snapRun(t, "profile", "rm", "work"); code != 0 || strings.Contains(errs, "Snapshot de seguridad") {
		t.Fatalf("con el snapshot automático apagado: %d %q", code, errs)
	}
	if _, err := os.Stat(core.SnapshotStoreDir(home)); err == nil {
		t.Fatal("con CCP_NO_AUTO_SNAPSHOT=1 se creó el almacén")
	}
}
```

- [ ] **Step 2: Comprobar que falla**

Run: `go test ./internal/cli/ -run 'SafetySnapshot|DailySnapshot|ScriptingCommands|AutoSnapshotOff'`
Expected: FAIL. Sin el hook no hay snapshot, y el mensaje «Snapshot de
seguridad» no aparece.

- [ ] **Step 3: Implementar (al final de `internal/cli/snapshot.go`)**

Añade `"path/filepath"` a los imports.

```go
// autoSnapshot guarda el estado antes de una operación que borra o sustituye
// configuración y devuelve su id ("" si está apagado). Si no sale, quien llama
// NO sigue. Si no hubo cambios desde el último snapshot, ese último ya es la red.
func autoSnapshot(home, trigger string) (string, error) {
	if os.Getenv("CCP_NO_AUTO_SNAPSHOT") == "1" {
		return "", nil
	}
	src, err := core.ClaudeSrc()
	if err != nil {
		return "", err
	}
	st, err := core.OpenSnapshotStore(home)
	if err != nil {
		return "", err
	}
	m, err := core.SnapshotCapture(home, src, st, core.SnapshotCaptureOpts{Trigger: trigger, Now: time.Now(), Machine: snapMachine()})
	if err != nil && !errors.Is(err, snapshot.ErrNoChanges) {
		return "", err
	}
	return m.ID, nil
}

// withSafetySnapshot es autoSnapshot para la CLI: informa del id por stderr o,
// si falla, pinta por qué y devuelve false.
func withSafetySnapshot(home, trigger string, lang i18n.Lang, stderr io.Writer) bool {
	id, err := autoSnapshot(home, trigger)
	if err != nil {
		fmt.Fprintln(stderr, errLine(stderr, i18n.T(lang, "cli.snapshot.auto_failed", err)))
		return false
	}
	if id != "" {
		fmt.Fprintln(stderr, mute(stderr, i18n.T(lang, "cli.snapshot.auto_saved", snapshot.Short(id))))
	}
	return true
}

// dailySnapshotCmds son los comandos de gestión, los que teclea una persona.
var dailySnapshotCmds = map[string]bool{
	"profile": true, "account": true, "instruct": true, "backup": true, "config": true,
	"desktop": true, "auto": true, "handoff": true, "session": true, "snapshot": true,
	"doctor": true, "lang": true, "upgrade": true, "update": true,
}

// dailySnapshotEvery: 20 horas y no 24, para que quien abre ccp a la misma hora
// cada día no se salte un día por minutos.
const dailySnapshotEvery = 20 * time.Hour

// maybeDailySnapshot toma el snapshot del día si toca. Nunca falla ni imprime.
func maybeDailySnapshot(now time.Time) {
	if os.Getenv("CCP_NO_AUTO_SNAPSHOT") == "1" {
		return
	}
	home, err := ccpHome()
	if err != nil {
		return
	}
	if _, err := os.Stat(filepath.Join(home, "ccp.yaml")); err != nil {
		return // sin configuración aún: nada que guardar
	}
	marker := filepath.Join(core.SnapshotStoreDir(home), ".daily-check")
	if fi, err := os.Stat(marker); err == nil && now.Sub(fi.ModTime()) < dailySnapshotEvery {
		return
	}
	st, err := core.OpenSnapshotStore(home)
	if err != nil {
		return
	}
	touch := func() {
		if os.WriteFile(marker, nil, 0o600) == nil {
			_ = os.Chtimes(marker, now, now)
		}
	}
	if last, ok := st.LatestTime(); ok && now.Sub(last) < dailySnapshotEvery {
		touch()
		return
	}
	src, err := core.ClaudeSrc()
	if err != nil {
		return
	}
	_, err = core.SnapshotCapture(home, src, st, core.SnapshotCaptureOpts{Trigger: "daily", Now: now, Machine: snapMachine()})
	if err == nil || errors.Is(err, snapshot.ErrNoChanges) {
		touch()
	}
}
```

- [ ] **Step 4: Enganchar los disparadores**

`internal/cli/cli.go`, en `Dispatch`, justo antes del `switch cmd {`:

```go
	// Snapshot del día (spec §8.3): solo en comandos de gestión y best-effort.
	if dailySnapshotCmds[cmd] {
		maybeDailySnapshot(time.Now())
	}
```

`internal/cli/cli.go`, en `dispatchBackup`, rama `restore`, justo antes de
`opts.Now = time.Now()`:

```go
		if !withSafetySnapshot(home, "pre-backup-restore", lang, stderr) {
			return 1
		}
```

`internal/cli/profile.go`, en la rama `case "rm", "del":`, justo antes de
`core.ProfileRm`:

```go
		if !withSafetySnapshot(home, "pre-profile-rm", lang, stderr) {
			return 1
		}
```

`internal/cli/serve_methods.go`, en `srvProfilesRemove`, justo antes de
`return nil, core.ProfileRm(s.home, p.Name)`:

```go
	if _, err := autoSnapshot(s.home, "pre-profile-rm"); err != nil {
		return nil, fmt.Errorf("no se pudo guardar el snapshot de seguridad y no se borró nada: %w", err)
	}
```

`internal/cli/serve_ops.go`, en `srvBackupRestore`, justo antes de
`rep, err := core.BackupRestore(...)`:

```go
	if _, err := autoSnapshot(s.home, "pre-backup-restore"); err != nil {
		return nil, fmt.Errorf("no se pudo guardar el snapshot de seguridad y no se restauró nada: %w", err)
	}
```

Si `fmt` aún no está importado en alguno de esos archivos, añádelo.

- [ ] **Step 5: Comprobar que pasa, y que no se rompió nada**

Run: `go test ./internal/cli/ && go test ./internal/golden/`
Expected: PASS. El `TestMain` mantiene apagado el snapshot automático en el
resto de tests del paquete, y el golden no ejecuta ningún comando de gestión.

- [ ] **Step 6: Gates y commit**

Mensaje propuesto: `feat(cli): snapshot de seguridad antes de borrar o restaurar, y uno diario`

---

### Task 13: `ccp serve` — métodos `snapshot.*`

Lo mismo que la CLI, como datos para la GUI. Todo son métodos nuevos, así que el
protocolo sigue en `1`. La pantalla de la GUI (evolucionar P-17 «Copias» a
«Snapshots») va en el plan de la GUI, no aquí.

**Files:**
- Create: `internal/cli/serve_snapshot.go`
- Modify: `internal/cli/serve_methods.go` (registro)
- Modify: `internal/cli/serve_test.go`

**Interfaces:**
- Consumes: `params`, `badParams`, `server` (`serve.go`), `snapSummaryOf` (Task 10), `core.*`, `snapshot.*`.
- Produces: `snapshot.list` · `snapshot.show {id}` · `snapshot.diff {from, to?}` ·
  `snapshot.create {label?, with_state?}` · `snapshot.restore {id, only?, dry_run?}` ·
  `snapshot.prune {dry_run?}` · `snapshot.pin {id, pinned, label?}` ·
  `snapshot.export {id, dest, passphrase?}` · `snapshot.import {archive, passphrase?}`.

- [ ] **Step 1: Escribir el test (añadir a `serve_test.go`)**

```go
func TestServeSnapshot(t *testing.T) {
	serveEnv(t)
	os.WriteFile(filepath.Join(os.Getenv("CCP_CLAUDE_SRC"), "settings.json"), []byte(`{}`), 0o644)

	_, r := serveRun(t, req(1, "snapshot.create", map[string]any{"label": "desde la gui"}))
	if r["1"].Error != nil {
		t.Fatalf("snapshot.create: %+v", r["1"].Error)
	}
	_, r = serveRun(t, req(2, "snapshot.list", nil))
	var list []snapSummary
	if err := json.Unmarshal(r["2"].Result, &list); err != nil || len(list) != 1 || list[0].Label != "desde la gui" {
		t.Fatalf("snapshot.list = %s, %v", r["2"].Result, err)
	}
	_, r = serveRun(t, req(3, "snapshot.restore", map[string]any{"id": "latest", "dry_run": true}))
	var plan core.SnapshotRestoreReport
	if err := json.Unmarshal(r["3"].Result, &plan); err != nil || len(plan.Steps) == 0 || plan.PreSnapshot != "" {
		t.Fatalf("snapshot.restore dry_run = %s, %v", r["3"].Result, err)
	}
	_, r = serveRun(t, req(4, "snapshot.show", map[string]any{}))
	if r["4"].Error == nil || r["4"].Error.Code != "invalid_params" {
		t.Fatalf("snapshot.show sin id: %+v", r["4"])
	}
}
```

- [ ] **Step 2: Comprobar que falla**

Run: `go test ./internal/cli/ -run TestServeSnapshot`
Expected: FAIL (método desconocido).

- [ ] **Step 3: Implementar `internal/cli/serve_snapshot.go`**

```go
package cli

// serve_snapshot.go — los métodos `snapshot.*` de `ccp serve`. Mismo motor que
// `ccp snapshot`; devuelven datos, nunca prosa.

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/JoseAFlores777/ccp/internal/core"
	"github.com/JoseAFlores777/ccp/internal/snapshot"
)

func srvSnapshotStore(s *server) (*snapshot.Store, string, error) {
	st, err := core.OpenSnapshotStore(s.home)
	if err != nil {
		return nil, "", err
	}
	src, err := core.ClaudeSrc()
	if err != nil {
		return nil, "", err
	}
	return st, src, nil
}

func srvSnapshotList(s *server, _ json.RawMessage) (any, error) {
	st, _, err := srvSnapshotStore(s)
	if err != nil {
		return nil, err
	}
	ms, err := st.List()
	if err != nil {
		return nil, err
	}
	out := make([]snapSummary, 0, len(ms))
	for _, m := range ms {
		out = append(out, snapSummaryOf(m, false))
	}
	return out, nil
}

func srvSnapshotShow(s *server, raw json.RawMessage) (any, error) {
	p, err := params[struct {
		ID string `json:"id"`
	}](raw)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(p.ID) == "" {
		return nil, badParams("falta el id del snapshot")
	}
	st, _, err := srvSnapshotStore(s)
	if err != nil {
		return nil, err
	}
	m, err := st.LoadManifest(p.ID)
	if err != nil {
		return nil, err
	}
	return m, nil
}

func srvSnapshotDiff(s *server, raw json.RawMessage) (any, error) {
	p, err := params[struct {
		From string `json:"from"`
		To   string `json:"to"`
	}](raw)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(p.From) == "" {
		return nil, badParams("falta el snapshot de origen")
	}
	st, src, err := srvSnapshotStore(s)
	if err != nil {
		return nil, err
	}
	from, err := st.LoadManifest(p.From)
	if err != nil {
		return nil, err
	}
	var to []snapshot.Item
	if p.To != "" {
		m, err := st.LoadManifest(p.To)
		if err != nil {
			return nil, err
		}
		to = m.Items
	} else {
		withState := countClass(from, snapshot.ClassState) > 0
		if to, err = core.SnapshotLive(s.home, src, core.SnapshotSourceOpts{WithState: withState}); err != nil {
			return nil, err
		}
	}
	changes := snapshot.Diff(from.Items, to)
	if changes == nil {
		changes = []snapshot.Change{}
	}
	return changes, nil
}

func srvSnapshotCreate(s *server, raw json.RawMessage) (any, error) {
	p, err := params[struct {
		Label     string `json:"label"`
		WithState bool   `json:"with_state"`
	}](raw)
	if err != nil {
		return nil, err
	}
	st, src, err := srvSnapshotStore(s)
	if err != nil {
		return nil, err
	}
	m, err := core.SnapshotCapture(s.home, src, st, core.SnapshotCaptureOpts{
		Trigger: "manual", Label: p.Label, WithState: p.WithState,
		Force: p.Label != "", Now: time.Now(), Machine: snapMachine(),
	})
	unchanged := errors.Is(err, snapshot.ErrNoChanges)
	if err != nil && !unchanged {
		return nil, err
	}
	return snapSummaryOf(m, unchanged), nil
}

func srvSnapshotRestore(s *server, raw json.RawMessage) (any, error) {
	p, err := params[struct {
		ID     string   `json:"id"`
		Only   []string `json:"only"`
		DryRun bool     `json:"dry_run"`
	}](raw)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(p.ID) == "" {
		return nil, badParams("falta el id del snapshot")
	}
	st, src, err := srvSnapshotStore(s)
	if err != nil {
		return nil, err
	}
	return core.SnapshotRestore(s.home, src, st, p.ID, core.SnapshotRestoreOpts{
		Only: p.Only, DryRun: p.DryRun, Now: time.Now(), Machine: snapMachine(),
	})
}

func srvSnapshotPrune(s *server, raw json.RawMessage) (any, error) {
	p, err := params[struct {
		DryRun bool `json:"dry_run"`
	}](raw)
	if err != nil {
		return nil, err
	}
	st, _, err := srvSnapshotStore(s)
	if err != nil {
		return nil, err
	}
	rep, err := snapshot.Prune(st, snapshot.DefaultPolicy, time.Now(), time.Hour, p.DryRun)
	if err != nil {
		return nil, err
	}
	deleted := rep.Deleted
	if deleted == nil {
		deleted = []string{}
	}
	return map[string]any{"deleted": deleted, "kept": rep.Kept, "blobs_deleted": rep.BlobsDeleted, "dry_run": p.DryRun}, nil
}

func srvSnapshotPin(s *server, raw json.RawMessage) (any, error) {
	p, err := params[struct {
		ID     string  `json:"id"`
		Pinned bool    `json:"pinned"`
		Label  *string `json:"label"`
	}](raw)
	if err != nil {
		return nil, err
	}
	st, _, err := srvSnapshotStore(s)
	if err != nil {
		return nil, err
	}
	m, err := st.SetPin(p.ID, p.Pinned, p.Label)
	if err != nil {
		return nil, err
	}
	return snapSummaryOf(m, false), nil
}

func srvSnapshotExport(s *server, raw json.RawMessage) (any, error) {
	p, err := params[struct {
		ID         string `json:"id"`
		Dest       string `json:"dest"`
		Passphrase string `json:"passphrase"`
	}](raw)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(p.ID) == "" || strings.TrimSpace(p.Dest) == "" {
		return nil, badParams("faltan el id y el archivo de destino")
	}
	if p.Passphrase != "" && len([]rune(p.Passphrase)) < snapMinPassphrase {
		return nil, badParams("la frase necesita al menos %d caracteres", snapMinPassphrase)
	}
	st, _, err := srvSnapshotStore(s)
	if err != nil {
		return nil, err
	}
	perm := os.FileMode(0o644)
	var pass []byte
	if p.Passphrase != "" {
		perm, pass = 0o600, []byte(p.Passphrase)
	}
	tmp := p.Dest + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, perm)
	if err != nil {
		return nil, err
	}
	m, err := snapshot.Export(st, p.ID, f, pass)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(tmp, p.Dest)
	}
	if err != nil {
		_ = os.Remove(tmp)
		return nil, err
	}
	return map[string]any{"id": m.ID, "dest": p.Dest, "with_secrets": pass != nil}, nil
}

func srvSnapshotImport(s *server, raw json.RawMessage) (any, error) {
	p, err := params[struct {
		Archive    string `json:"archive"`
		Passphrase string `json:"passphrase"`
	}](raw)
	if err != nil {
		return nil, err
	}
	st, _, err := srvSnapshotStore(s)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(p.Archive)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	rep, err := snapshot.Import(st, f, func() ([]byte, error) {
		if p.Passphrase == "" {
			return nil, fmt.Errorf("este archivo trae secretos sellados: hace falta la frase")
		}
		return []byte(p.Passphrase), nil
	})
	if err != nil {
		return nil, err
	}
	missing := rep.Missing
	if missing == nil {
		missing = []string{}
	}
	return map[string]any{"snapshot": snapSummaryOf(rep.Manifest, false), "missing": missing}, nil
}
```

Registro, en `serveRegistry` (`internal/cli/serve_methods.go`), junto a `backup.*`:

```go
		"snapshot.list":    r(srvSnapshotList),
		"snapshot.show":    r(srvSnapshotShow),
		"snapshot.diff":    r(srvSnapshotDiff),
		"snapshot.create":  w(srvSnapshotCreate),
		"snapshot.restore": w(srvSnapshotRestore),
		"snapshot.prune":   w(srvSnapshotPrune),
		"snapshot.pin":     w(srvSnapshotPin),
		"snapshot.export":  w(srvSnapshotExport),
		"snapshot.import":  w(srvSnapshotImport),
```

`snapshot.restore` va como escritura aunque se pida en dry-run: el registro no
distingue por parámetros, y serializarlo con las demás escrituras es lo seguro.

- [ ] **Step 4: Comprobar que pasa**

Run: `go test ./internal/cli/ -run TestServe && go vet ./internal/cli/`
Expected: PASS.

- [ ] **Step 5: Gates y commit**

Mensaje propuesto: `feat(serve): métodos snapshot.* para la GUI`

---

### Task 14: documentación y ADR

**Files:**
- Create: `docs/adr/0012-snapshots-content-addressed.md`
- Modify: `README.md`, `README.es.md` (sección de snapshots, junto a la de backup)
- Modify: `CHANGELOG.md` (entrada en *Unreleased*)
- Modify: `CLAUDE.md` (arquitectura y ubicaciones)
- Modify: `docs/superpowers/specs/2026-09-18-config-unificada-snapshots-nube-design.md` (estado de §8)

- [ ] **Step 1: El ADR**

```markdown
# ADR 0012 — Snapshots direccionados por contenido, con rutas lógicas y clases

Fecha: 2026-09-18 · Estado: aceptado · Spec: 2026-09-18-config-unificada-snapshots-nube §8

## Contexto

`ccp backup` guarda ccp.yaml y los overlays en un tar.gz. No cubre `~/.claude`,
ni la configuración de los `.claude.json`, ni Desktop, ni los proyectos. Tampoco
guarda historial: cada copia es un archivo suelto que alguien tiene que acordarse
de hacer. Y lo que viene después, subirlo a la nube y traerlo a otra máquina,
necesita un formato que se pueda sincronizar por partes y que no dependa de las
rutas de una máquina concreta.

## Decisión

- **Almacén direccionado por contenido** en `~/.config/ccp/snapshots`. Cada
  archivo es un blob cuyo nombre es el sha256 de su contenido, y cada snapshot es
  un manifiesto que los lista. Dos snapshots que comparten un archivo lo guardan
  una vez, así que un snapshot diario cuesta casi nada. El id del manifiesto es
  el hash de su contenido: un manifiesto alterado no carga.
- **Rutas lógicas**, no rutas de disco (`claude/settings.json`,
  `ccp/profiles/work/overlay/CLAUDE.md`, `project/<clave>/…`). El único mapa
  entre ambas está en `core/snapshot_layout.go` y `snapshot_restore.go`, y los
  proyectos se identifican por su remoto de git.
- **Clases de elemento**:
  - `authored`: se captura siempre.
  - `secret`: se captura siempre y va sellada con la clave del almacén.
  - `state`: solo con `--with-state`.
  - Lo generado, la caché y lo atado a la máquina no se capturan nunca.
- **De `.claude.json` solo la configuración**, y restaurarla es fusionar.
- **Restaurar**: primero el plan; luego un snapshot previo, sin el cual no se
  escribe; luego escritura atómica y regeneración de perfiles. No se borra nada
  que no esté en el snapshot.
- **`.ccpsnap`** para mover un snapshot a mano. Los secretos solo viajan sellados
  con una frase (Argon2id + XChaCha20-Poly1305).
- **El paquete `internal/snapshot` no importa `core`**: lo compartirá el backend
  de la nube.

## Consecuencias

- Lo que el plan de la nube sube es este mismo formato, cifrado por completo con
  la clave de la bóveda.
- `ccp backup` sigue existiendo tal cual. Unificarlo con `snapshot export` es una
  decisión aparte.
- El almacén crece con cada cambio. `ccp snapshot prune` (7 días, 4 semanas,
  6 meses; lo fijado nunca se borra) lo mantiene acotado.
```

- [ ] **Step 2: README, CHANGELOG, CLAUDE.md**

- **README.md / README.es.md**: una sección «Snapshots», tras la de backup, con:
  - qué captura y qué no (la tabla de clases);
  - los comandos;
  - el snapshot diario y el de seguridad, con `CCP_NO_AUTO_SNAPSHOT=1` para
    apagarlos;
  - que `restore` sin `--yes` solo enseña el plan;
  - que `export --with-secrets` pide una frase.
- **CHANGELOG.md**, en *Unreleased*: `ccp snapshot` (create, list, show, diff,
  restore, prune, pin, export, import), el snapshot diario, el de seguridad antes
  de `profile rm` y `backup restore`, y los métodos `snapshot.*` de `ccp serve`.
- **CLAUDE.md**:
  - en *Architecture*, un apartado corto para `internal/vault`,
    `internal/snapshot` y `core/snapshot_layout.go` + `snapshot_restore.go`, con
    las reglas que no se negocian:
    - `internal/snapshot` no importa `core`;
    - ninguna ruta lógica escribe fuera de la lista cerrada de destinos;
    - no se restaura sin snapshot previo;
    - de `.claude.json` solo viaja la configuración;
  - en *Config & state locations*, la entrada `snapshots/`: `store.key` 0600,
    `objects/`, `snaps/`, `.daily-check`;
  - en *Conventions*, que `snapshot` no está en la completion (igual que
    `backup`) y por qué.

- [ ] **Step 3: El spec**

En §8 del spec, añade bajo el título:
`> **Implementado (plan 2026-09-18-snapshots-locales).** La GUI (P-17 → Snapshots) queda para el plan de la GUI.`

- [ ] **Step 4: Gates finales y commit**

Todos los gates de CI, más `bash legacy/tests/run.sh` y
`bash testdata/golden/capture.sh --check`: ambos deben seguir verdes, porque este
plan no toca el contrato.
Mensaje propuesto: `docs: snapshots locales (ADR 0012, README, CHANGELOG, CLAUDE.md)`

---

## Fuera de este plan

- **GUI.** La pantalla de snapshots (P-17 «Copias» evoluciona a «Snapshots»),
  que consume los métodos `snapshot.*`, va en el plan de la GUI.
- **`snapshot` en la completion.** Exige actualizar el oráculo bash y
  regenerar el golden.
- **Mapeo de rutas entre máquinas** (spec §11), un proyecto en otra ruta u otro
  HOME. Llega con el plan de la nube, que es donde un snapshot cambia de máquina
  de verdad.
- **Arreglar `ccp backup restore`** para que regenere los perfiles (spec B2). Va
  en el plan de la Fase 0; aquí solo gana el snapshot de seguridad previo.
- **Aviso en `ccp doctor`** cuando el último snapshot tiene más de N días (el
  diario falla en silencio a propósito). Es una tarea pequeña que puede ir en la
  Fase 0.

