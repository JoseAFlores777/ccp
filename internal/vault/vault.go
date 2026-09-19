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
