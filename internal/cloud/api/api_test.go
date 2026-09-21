package api

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
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

// La bóveda es opaca para el servidor: el KDF viaja como JSON crudo (ni se
// reinterpreta ni se escapa como cadena) y las envolturas en base64. Si alguien
// cambia KDF a map[string]any, un parámetro que este binario no conozca se
// perdería al reenviarlo.
func TestVaultViajaOpaca(t *testing.T) {
	v := Vault{
		KDF:            json.RawMessage(`{"alg":"argon2id","time":3,"futuro":{"x":1}}`),
		PassphraseWrap: []byte{0xde, 0xad},
		RecoveryWrap:   []byte{0xbe, 0xef},
		SignPub:        []byte{1, 2, 3},
	}
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	got := string(b)
	if !strings.Contains(got, `"kdf":{"alg":"argon2id","time":3,"futuro":{"x":1}}`) {
		t.Errorf("el KDF no viajó crudo: %s", got)
	}
	if !strings.Contains(got, `"passphrase_wrap":"3q0="`) {
		t.Errorf("las envolturas no viajan en base64: %s", got)
	}
	var back Vault
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	if string(back.KDF) != string(v.KDF) {
		t.Errorf("KDF ida y vuelta = %s", back.KDF)
	}
}

// Snapshot empotra SnapshotMeta sin etiqueta: el cuerpo de GET
// /v1/snapshots/{id} es un solo objeto plano. Poner ahí un `json:"meta"` —o un
// campo con nombre— anidaría los metadatos y rompería a todo cliente publicado.
func TestSnapshotAplanaSusMetadatos(t *testing.T) {
	s := Snapshot{
		SnapshotMeta: SnapshotMeta{ID: strings.Repeat("a", 64), Created: time.Unix(0, 0).UTC()},
		Manifest:     []byte{7},
		Sig:          []byte{8},
	}
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	var flat map[string]json.RawMessage
	if err := json.Unmarshal(b, &flat); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"id", "parent", "device_id", "device_name", "created", "size", "pinned", "manifest", "sig"} {
		if _, ok := flat[k]; !ok {
			t.Errorf("falta la clave %q en %s", k, b)
		}
	}
}

// El cuerpo de error sin blobs que falten no debe llevar "missing": el cliente
// distingue «faltan estos» de «no falta ninguno» por la presencia de la lista.
func TestErrorOmiteMissingVacio(t *testing.T) {
	b, err := json.Marshal(Error{Code: CodeNotFound, Message: "no está"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "missing") {
		t.Errorf("Error vacío arrastra missing: %s", b)
	}
}

// Los topes existen por límites ajenos (Cloudflare corta los cuerpos de más de
// 100 MB); si alguien los sube por encima, el fallo aparecería en producción y
// no aquí.
func TestTopesCabenEnCloudflare(t *testing.T) {
	const cloudflare = 100 << 20
	if MaxBlobBytes >= cloudflare || MaxManifestBytes >= cloudflare {
		t.Errorf("topes %d/%d no caben en el límite de %d", MaxBlobBytes, MaxManifestBytes, cloudflare)
	}
	if MaxPresignIDs <= 0 {
		t.Errorf("MaxPresignIDs = %d", MaxPresignIDs)
	}
}
