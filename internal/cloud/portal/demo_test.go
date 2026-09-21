package portal

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/JoseAFlores777/ccp/internal/cloud/api"
	"github.com/JoseAFlores777/ccp/internal/cloud/crypt"
	"github.com/JoseAFlores777/ccp/internal/snapshot"
)

// sealedSnap es un snapshot de la demo: sus metadatos y el cuerpo sellado.
type sealedSnap struct {
	meta api.SnapshotMeta
	full api.Snapshot
}

// TestDemo levanta el portal con una bóveda de verdad y datos inventados, para
// mirarlo con un navegador. No corre en CI: sin él, la única forma de ver la
// SPA sería desplegar el stack entero, y una pantalla que solo se puede probar
// desplegando no se prueba.
//
//	CCP_PORTAL_DEMO=1 go test ./internal/cloud/portal -run Demo -v
//
// Imprime la URL y la frase de bóveda, y se queda escuchando 15 minutos.
func TestDemo(t *testing.T) {
	if os.Getenv("CCP_PORTAL_DEMO") == "" {
		t.Skip("demo manual: CCP_PORTAL_DEMO=1 para levantarla")
	}
	const frase = "demo"
	ak, recovery, w, err := crypt.NewVault([]byte(frase))
	if err != nil {
		t.Fatal(err)
	}
	acct, err := crypt.NewAccount(ak)
	if err != nil {
		t.Fatal(err)
	}
	devs := []api.Device{
		{ID: "11111111-1111-1111-1111-111111111111", Name: "MacBook Pro", Platform: "darwin/arm64",
			CCPVersion: "2.4.0", Created: time.Now().Add(-720 * time.Hour), LastSeen: time.Now().Add(-4 * time.Minute)},
		{ID: "22222222-2222-2222-2222-222222222222", Name: "Mac mini", Platform: "darwin/arm64",
			CCPVersion: "2.3.1", Created: time.Now().Add(-300 * time.Hour), LastSeen: time.Now().Add(-30 * time.Hour)},
	}
	var snaps []sealedSnap
	mk := func(dev, name string, ago time.Duration, items []snapshot.Item, parent string) string {
		id := fmt.Sprintf("%064x", len(snaps)+1)
		m := snapshot.Manifest{Format: 1, ID: id, Parent: parent, Created: time.Now().Add(-ago),
			Machine: name, CCPVersion: "2.4.0", Trigger: "diaria", Items: items}
		raw, err := json.Marshal(m)
		if err != nil {
			t.Fatal(err)
		}
		body, err := acct.SealManifest(id, raw)
		if err != nil {
			t.Fatal(err)
		}
		var size int64
		for _, it := range items {
			size += it.Size
		}
		meta := api.SnapshotMeta{ID: id, Parent: parent, DeviceID: dev, DeviceName: name,
			Created: m.Created, Size: size}
		snaps = append(snaps, sealedSnap{meta, api.Snapshot{SnapshotMeta: meta, Manifest: body, Sig: acct.Sign(id, parent, body)}})
		return id
	}
	base := []snapshot.Item{
		{LPath: "ccp/ccp.yaml", Hash: "a1", Size: 900, Mode: 0o644, Class: snapshot.ClassAuthored},
		{LPath: "claude/CLAUDE.md", Hash: "b1", Size: 4200, Mode: 0o644, Class: snapshot.ClassAuthored},
		{LPath: "claude/settings.json", Hash: "c1", Size: 800, Mode: 0o644, Class: snapshot.ClassAuthored},
		{LPath: "ccp/profiles/work/overlay/CLAUDE.md", Hash: "d1", Size: 300, Mode: 0o644, Class: snapshot.ClassAuthored},
		{LPath: "ccp/profiles/work/api_key", Hash: "e1", Size: 80, Mode: 0o600, Class: snapshot.ClassSecret},
	}
	crecido := append(append([]snapshot.Item{}, base...),
		snapshot.Item{LPath: "ccp/profiles/personal/overlay/CLAUDE.md", Hash: "f1", Size: 120, Mode: 0o644, Class: snapshot.ClassAuthored})
	crecido[0].Hash = "a2"
	crecido[0].Size = 980
	p1 := mk(devs[0].ID, "macbook", 72*time.Hour, base, "")
	p2 := mk(devs[0].ID, "macbook", 20*time.Hour, crecido, p1)
	// El deseado es lo que tiene el OTRO equipo: así el MacBook aparece con
	// deriva respecto a la revisión que el portal le ha publicado.
	deseado := mk(devs[1].ID, "mac-mini", 2*time.Hour, append(append([]snapshot.Item{}, crecido...),
		snapshot.Item{LPath: "ccp/profiles/deepseek/overlay/settings.overlay.json", Hash: "g1", Size: 60, Mode: 0o644, Class: snapshot.ClassAuthored}), p2)
	mk(devs[1].ID, "mac-mini", 26*time.Hour, base, "")
	_ = recovery
	serveDemo(t, w, devs, snaps, deseado, frase)
}

// serveDemo es el API falso de la demo: contesta lo justo para que la SPA
// tenga algo que pintar. No valida tokens a propósito — lo que se quiere mirar
// aquí son las pantallas, no el login.
func serveDemo(t *testing.T, w crypt.Wraps, devs []api.Device, snaps []sealedSnap, deseado, frase string) {
	t.Helper()
	kdf, err := json.Marshal(w.KDF)
	if err != nil {
		t.Fatal(err)
	}
	send := func(rw http.ResponseWriter, v any) {
		rw.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(rw).Encode(v)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/info", func(rw http.ResponseWriter, _ *http.Request) {
		send(rw, api.Info{APIVersion: api.Version, Issuer: "http://127.0.0.1:1/realms/ccp",
			ClientID: "ccp-cli", PortalClientID: "ccp-portal"})
	})
	mux.HandleFunc("GET /v1/me", func(rw http.ResponseWriter, _ *http.Request) {
		send(rw, api.Me{UserID: "demo", Email: "tu@ejemplo.com", HasVault: true})
	})
	mux.HandleFunc("GET /v1/vault", func(rw http.ResponseWriter, _ *http.Request) {
		send(rw, api.Vault{KDF: kdf, PassphraseWrap: w.Passphrase, RecoveryWrap: w.Recovery, SignPub: w.SignPub})
	})
	mux.HandleFunc("GET /v1/devices", func(rw http.ResponseWriter, _ *http.Request) { send(rw, devs) })
	mux.HandleFunc("POST /v1/devices", func(rw http.ResponseWriter, r *http.Request) {
		var in api.DeviceIn
		_ = json.NewDecoder(r.Body).Decode(&in)
		d := api.Device{ID: "33333333-3333-3333-3333-333333333333", Name: in.Name,
			Platform: in.Platform, Created: time.Now(), LastSeen: time.Now()}
		devs = append(devs, d)
		rw.WriteHeader(http.StatusCreated)
		send(rw, d)
	})
	mux.HandleFunc("GET /v1/snapshots", func(rw http.ResponseWriter, r *http.Request) {
		dev := r.URL.Query().Get("device")
		out := []api.SnapshotMeta{}
		for i := len(snaps) - 1; i >= 0; i-- { // más nuevos primero, como el API
			if dev == "" || snaps[i].meta.DeviceID == dev {
				out = append(out, snaps[i].meta)
			}
		}
		send(rw, out)
	})
	mux.HandleFunc("GET /v1/snapshots/{id}", func(rw http.ResponseWriter, r *http.Request) {
		for _, s := range snaps {
			if s.meta.ID == r.PathValue("id") {
				send(rw, s.full)
				return
			}
		}
		http.Error(rw, `{"code":"not_found","message":"no está"}`, http.StatusNotFound)
	})
	mux.HandleFunc("GET /v1/revisions", func(rw http.ResponseWriter, _ *http.Request) {
		send(rw, []api.RevisionMeta{{ID: strings.Repeat("d", 64), DeviceID: devs[0].ID, DeviceName: devs[0].Name,
			Snapshot: deseado, Created: time.Now().Add(-time.Hour), State: api.RevPending, By: "portal"}})
	})
	h, err := New(Config{Issuer: "http://127.0.0.1:1/realms/ccp"})
	if err != nil {
		t.Fatal(err)
	}
	mux.Handle("/", h)
	srv := httptest.NewServer(mux)
	defer srv.Close()
	t.Logf("portal de demo en %s — frase de bóveda: %q", srv.URL, frase)
	t.Logf("para saltarte el login, en la consola del navegador:\n"+
		`sessionStorage.setItem('ccp.portal.session', JSON.stringify({access:'demo',refresh:'',expires:%d,token:'',end:''}))`,
		time.Now().Add(time.Hour).UnixMilli())
	time.Sleep(15 * time.Minute)
}
