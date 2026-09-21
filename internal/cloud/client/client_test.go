package client

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
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
	if err := files.SaveToken(tok); err != nil {
		t.Fatal(err)
	}
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
	if err := files.SaveAK(bytes.Repeat([]byte{1}, 32)); err != nil {
		t.Fatal(err)
	}
	if err := files.SaveConfig(Config{Server: "https://x"}); err != nil {
		t.Fatal(err)
	}
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
