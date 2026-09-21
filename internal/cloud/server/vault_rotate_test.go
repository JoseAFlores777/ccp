package server

import (
	"encoding/json"
	"testing"

	"github.com/JoseAFlores777/ccp/internal/cloud/api"
	"github.com/JoseAFlores777/ccp/internal/cloud/client"
	"github.com/JoseAFlores777/ccp/internal/cloud/crypt"
)

// Rotar las claves de acceso reemplaza las dos envolturas. El servidor no
// puede abrir ninguna de las dos ni antes ni después: lo que comprueba es la
// firma de la rotación —prueba de que quien rota tiene la AK— y que la clave
// pública de firma sea la misma, o sea que la AK no cambió.
func TestVaultRotarLasEnvolturas(t *testing.T) {
	e := newEnv(t)
	tok := e.iss.AccessToken()
	dev := e.newDevice(tok, "mac")

	ak, _, w, err := crypt.NewVault([]byte("frase-larga-del-dueño"))
	if err != nil {
		t.Fatal(err)
	}
	acct, err := crypt.NewAccount(ak)
	if err != nil {
		t.Fatal(err)
	}
	v := vaultAPI(t, w)
	_, w2, err := crypt.RewrapVault(ak, []byte("otra-frase-larga"))
	if err != nil {
		t.Fatal(err)
	}
	nueva := vaultAPI(t, w2)
	rot := api.VaultRewrap{Vault: nueva, Sig: acct.SignRewrap(api.RewrapParts{
		PrevPassphraseWrap: w.Passphrase, PrevRecoveryWrap: w.Recovery,
		KDF: nueva.KDF, PassphraseWrap: nueva.PassphraseWrap, RecoveryWrap: nueva.RecoveryWrap})}

	// Sin bóveda todavía no hay nada que rotar.
	if code := e.call("PUT", "/v1/vault/wraps", tok, dev, rot, nil); code != 404 {
		t.Fatalf("rotar sin bóveda = %d", code)
	}
	if code := e.call("PUT", "/v1/vault", tok, dev, v, nil); code != 201 {
		t.Fatal("no creó la bóveda")
	}
	if code := e.call("PUT", "/v1/vault/wraps", tok, dev, rot, nil); code != 204 {
		t.Fatalf("rotar = %d", code)
	}
	var got api.Vault
	if code := e.call("GET", "/v1/vault", tok, dev, nil, &got); code != 200 {
		t.Fatalf("GET /v1/vault = %d", code)
	}
	if string(got.PassphraseWrap) != string(w2.Passphrase) || string(got.RecoveryWrap) != string(w2.Recovery) {
		t.Fatal("las envolturas guardadas no son las que se subieron")
	}

	// Una firma distinta es una AK distinta, y eso dejaría sin abrir todo lo
	// publicado: se rechaza en vez de guardarlo.
	_, _, otraCaja, err := crypt.NewVault([]byte("bóveda-de-otra-cuenta"))
	if err != nil {
		t.Fatal(err)
	}
	_, w3, err := crypt.RewrapVault(ak, []byte("tercera-frase-larga"))
	if err != nil {
		t.Fatal(err)
	}
	tercera := vaultAPI(t, w3)
	otra := api.VaultRewrap{Vault: tercera, Sig: acct.SignRewrap(api.RewrapParts{
		PrevPassphraseWrap: w2.Passphrase, PrevRecoveryWrap: w2.Recovery,
		KDF: tercera.KDF, PassphraseWrap: tercera.PassphraseWrap, RecoveryWrap: tercera.RecoveryWrap})}
	otra.SignPub = otraCaja.SignPub
	if code := e.call("PUT", "/v1/vault/wraps", tok, dev, otra, nil); code != 409 {
		t.Fatalf("rotar cambiando la firma = %d", code)
	}
	// Y queda apuntado que se rotó, sin nada de lo rotado.
	var log []api.AuditEntry
	if code := e.call("GET", "/v1/audit?action=vault.rewrap", tok, dev, nil, &log); code != 200 || len(log) != 1 {
		t.Fatalf("auditoría de la rotación = %d %+v", code, log)
	}
	if len(log[0].Detail) != 0 {
		t.Fatalf("el registro no guarda nada de la bóveda: %+v", log[0].Detail)
	}
}

// vaultAPI pasa unas envolturas a lo que viaja por la API.
func vaultAPI(t *testing.T, w crypt.Wraps) api.Vault {
	t.Helper()
	kdf, err := json.Marshal(w.KDF)
	if err != nil {
		t.Fatal(err)
	}
	return api.Vault{KDF: kdf, PassphraseWrap: w.Passphrase, RecoveryWrap: w.Recovery, SignPub: w.SignPub}
}

// La clave pública de firma NO es un freno: el servidor se la sirve en claro a
// cualquier dispositivo autenticado, así que copiarla es gratis. Sin una
// prueba de posesión de la AK, quien tenga token y equipo puede sustituir las
// envolturas por las suyas y dejar la bóveda sin abrir para todos: ni la frase
// ni el código del dueño valdrían ya, y no queda copia de lo pisado.
func TestVaultRotarExigeFirmaDeLaAK(t *testing.T) {
	e := newEnv(t)
	tok := e.iss.AccessToken()
	dev := e.newDevice(tok, "mac")

	frase := []byte("frase-del-usuario-legitima")
	ak, _, w, err := crypt.NewVault(frase)
	if err != nil {
		t.Fatal(err)
	}
	buena := vaultAPI(t, w)
	if code := e.call("PUT", "/v1/vault", tok, dev, buena, nil); code != 201 {
		t.Fatalf("crear la bóveda = %d", code)
	}

	// El intruso no tiene la AK: fabrica la suya y copia el sign_pub que el
	// GET le regala.
	_, _, mala, err := crypt.NewVault([]byte("frase-del-intruso"))
	if err != nil {
		t.Fatal(err)
	}
	mala.SignPub = w.SignPub
	robo := api.VaultRewrap{Vault: vaultAPI(t, mala), Sig: make([]byte, 64)}
	if code := e.call("PUT", "/v1/vault/wraps", tok, dev, robo, nil); code != 403 {
		t.Fatalf("rotar sin firmar = %d", code)
	}
	var got api.Vault
	if code := e.call("GET", "/v1/vault", tok, dev, nil, &got); code != 200 {
		t.Fatalf("GET /v1/vault = %d", code)
	}
	tras, err := client.VaultFromAPI(got)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := crypt.UnlockPassphrase(tras, frase); err != nil {
		t.Fatalf("la frase del dueño ya no abre la bóveda: %v", err)
	}

	// Con la AK en la mano sí: la firma ata las envolturas nuevas a las que
	// reemplazan.
	_, w2, err := crypt.RewrapVault(ak, []byte("frase-nueva-del-dueño"))
	if err != nil {
		t.Fatal(err)
	}
	acct, err := crypt.NewAccount(ak)
	if err != nil {
		t.Fatal(err)
	}
	nueva := vaultAPI(t, w2)
	sig := acct.SignRewrap(api.RewrapParts{
		PrevPassphraseWrap: w.Passphrase, PrevRecoveryWrap: w.Recovery,
		KDF: nueva.KDF, PassphraseWrap: nueva.PassphraseWrap, RecoveryWrap: nueva.RecoveryWrap})
	if code := e.call("PUT", "/v1/vault/wraps", tok, dev, api.VaultRewrap{Vault: nueva, Sig: sig}, nil); code != 204 {
		t.Fatalf("rotar firmando = %d", code)
	}
	// Y la misma firma no vale dos veces: ya no describe la bóveda que hay.
	if code := e.call("PUT", "/v1/vault/wraps", tok, dev, api.VaultRewrap{Vault: nueva, Sig: sig}, nil); code != 403 {
		t.Fatalf("repetir la rotación = %d", code)
	}
}
