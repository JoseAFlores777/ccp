package portal

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/JoseAFlores777/ccp/internal/cloud/crypt"
	"github.com/JoseAFlores777/ccp/internal/vault"
)

// Los vectores que el portal tiene que reproducir en el navegador. Se generan
// aquí, con el mismo código que usa `ccp`, en vez de copiarlos a mano: un
// golden escrito a mano envejece sin avisar, y lo que hay que probar es que la
// criptografía del navegador abre LO QUE ESTE BINARIO SELLA.
type jsVectors struct {
	Argon2 struct {
		Password  string `json:"password"`
		Salt      []byte `json:"salt"`
		Time      uint32 `json:"time"`
		MemoryKiB uint32 `json:"memory_kib"`
		Threads   uint8  `json:"threads"`
		Key       []byte `json:"key"`
	} `json:"argon2"`
	Subkey struct {
		Master []byte `json:"master"`
		Info   string `json:"info"`
		Key    []byte `json:"key"`
	} `json:"subkey"`
	Recovery struct {
		Code string `json:"code"`
		Key  []byte `json:"key"`
	} `json:"recovery"`
	Vault struct {
		KDF            vault.KDFParams `json:"kdf"`
		PassphraseWrap []byte          `json:"passphrase_wrap"`
		RecoveryWrap   []byte          `json:"recovery_wrap"`
		SignPub        []byte          `json:"sign_pub"`
		Passphrase     string          `json:"passphrase"`
		RecoveryCode   string          `json:"recovery_code"`
		AK             []byte          `json:"ak"`
	} `json:"vault"`
	Manifest struct {
		ID       string `json:"id"`
		Parent   string `json:"parent"`
		Sealed   []byte `json:"sealed"`
		Sig      []byte `json:"sig"`
		Plain    []byte `json:"plain"`
		BadSig   []byte `json:"bad_sig"`
		DataInfo string `json:"data_info"`
	} `json:"manifest"`
}

func buildVectors(t *testing.T) jsVectors {
	t.Helper()
	var v jsVectors
	// Parámetros mínimos para el vector suelto de Argon2id: el algoritmo es el
	// mismo con 8 MiB que con 64, y el vector de la bóveda ya paga los de
	// verdad.
	p := vault.KDFParams{Salt: []byte("0123456789abcdef"), Time: 1, MemoryKiB: 8 * 1024, Threads: 1}
	key, err := vault.DeriveKey([]byte("la frase de la bóveda"), p)
	if err != nil {
		t.Fatal(err)
	}
	v.Argon2.Password, v.Argon2.Salt = "la frase de la bóveda", p.Salt
	v.Argon2.Time, v.Argon2.MemoryKiB, v.Argon2.Threads, v.Argon2.Key = p.Time, p.MemoryKiB, p.Threads, key

	v.Subkey.Master, v.Subkey.Info = key, "ccp/v1/data"
	if v.Subkey.Key, err = vault.DeriveSubkey(key, v.Subkey.Info); err != nil {
		t.Fatal(err)
	}
	return v
}

func buildVaultVectors(t *testing.T, v *jsVectors) {
	t.Helper()
	const phrase = "frase de bóveda con acentos áéí"
	ak, code, w, err := crypt.NewVault([]byte(phrase))
	if err != nil {
		t.Fatal(err)
	}
	v.Vault.KDF, v.Vault.PassphraseWrap, v.Vault.RecoveryWrap = w.KDF, w.Passphrase, w.Recovery
	v.Vault.SignPub, v.Vault.Passphrase, v.Vault.RecoveryCode, v.Vault.AK = w.SignPub, phrase, code, ak

	rk, err := vault.RecoveryKey(code)
	if err != nil {
		t.Fatal(err)
	}
	v.Recovery.Code, v.Recovery.Key = code, rk

	acct, err := crypt.NewAccount(ak)
	if err != nil {
		t.Fatal(err)
	}
	const id = "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff"
	const parent = "ffeeddccbbaa99887766554433221100ffeeddccbbaa99887766554433221100"
	plain := []byte(`{"format":1,"id":"local","items":[{"lpath":"ccp/ccp.yaml","hash":"ab","size":12}]}`)
	sealed, err := acct.SealManifest(id, plain)
	if err != nil {
		t.Fatal(err)
	}
	sig := acct.Sign(id, parent, sealed)
	bad := append([]byte(nil), sig...)
	bad[0] ^= 0xff
	v.Manifest.ID, v.Manifest.Parent, v.Manifest.Sealed = id, parent, sealed
	v.Manifest.Sig, v.Manifest.Plain, v.Manifest.BadSig = sig, plain, bad
	v.Manifest.DataInfo = "ccp/v1/data"
}

// TestCriptoDelNavegador es el único sitio donde se comprueba que el portal
// abre de verdad lo que el cliente sella: Argon2id, XChaCha20-Poly1305, HKDF y
// Ed25519 escritos en JS contra los vectores de este binario. Sin node no se
// puede comprobar, así que se salta — decir «ok» porque no se pudo mirar es
// justo lo que no hace el doctor de Desktop (ADR 0009).
func TestCriptoDelNavegador(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("sin node: no se puede ejecutar la criptografía del portal")
	}
	v := buildVectors(t)
	buildVaultVectors(t, &v)
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "vectors.json")
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	out, err := exec.Command(node, "crypto_test.mjs", path).CombinedOutput()
	t.Logf("node tardó %s\n%s", time.Since(start).Round(time.Millisecond), out)
	if err != nil {
		t.Fatalf("la criptografía del portal no reproduce los vectores: %v", err)
	}
}
