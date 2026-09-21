package portal

import (
	"encoding/json"
	"fmt"
	"io"
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
	// Con contenido de verdad, sellado con esta misma bóveda: sin él el editor
	// no tiene nada que abrir, y lo que se quiere mirar aquí es justo eso.
	blobs := map[string][]byte{}
	item := func(lpath, contenido string, mode uint32, class snapshot.Class) snapshot.Item {
		hash := snapshot.Hash([]byte(contenido))
		id := acct.BlobID(hash)
		sealed, err := acct.SealBlob(id, []byte(contenido))
		if err != nil {
			t.Fatal(err)
		}
		blobs[id] = sealed
		return snapshot.Item{LPath: lpath, Hash: hash, Size: int64(len(contenido)), Mode: mode, Class: class}
	}
	const a = snapshot.ClassAuthored
	base := []snapshot.Item{
		item("ccp/ccp.yaml", "version: 2\nrules:\n  - path: /Users/tu/repos/web\n    profile: work\n", 0o644, a),
		item("claude/CLAUDE.md", "# Instrucciones globales\n\nResponde en español.\n", 0o644, a),
		item("claude/settings.json", `{"model":"opus","permissions":{"allow":["Bash(git status)"],"deny":[]},"env":{"EDITOR":"vim"}}`, 0o644, a),
		item("claude/agents/revisor.md", "---\nname: revisor\n---\n\nRevisa el diff.\n", 0o644, a),
		item("ccp/profiles/work/overlay/CLAUDE.md", "# Perfil work\n\nNo toques producción.\n", 0o644, a),
		item("ccp/profiles/work/overlay/settings.overlay.json", `{"statusLine":{"type":"command","command":"ccp status"}}`, 0o644, a),
		item("ccp/profiles/work/api_key", "sk-secreto-de-mentira", 0o600, snapshot.ClassSecret),
		item("desktop/work/claude_desktop_config.json", `{"mcpServers":{"obsidian":{"command":"node","args":["mcp.js"]}}}`, 0o600, snapshot.ClassSecret),
		item("project/github.com~tu~web/CLAUDE.local.md", "# Solo en este repo\n", 0o644, a),
	}
	crecido := append(append([]snapshot.Item{}, base...),
		item("ccp/profiles/personal/overlay/CLAUDE.md", "# Perfil personal\n", 0o644, a))
	crecido[0] = item("ccp/ccp.yaml", "version: 2\nrules:\n  - path: /Users/tu/repos/web\n    profile: work\n  - path: /Users/tu/repos/api\n    profile: personal\n", 0o644, a)
	p1 := mk(devs[0].ID, "macbook", 72*time.Hour, base, "")
	p2 := mk(devs[0].ID, "macbook", 20*time.Hour, crecido, p1)
	// El deseado es lo que tiene el OTRO equipo: así el MacBook aparece con
	// deriva respecto a la revisión que el portal le ha publicado.
	deseado := mk(devs[1].ID, "mac-mini", 2*time.Hour, append(append([]snapshot.Item{}, crecido...),
		snapshot.Item{LPath: "ccp/profiles/deepseek/overlay/settings.overlay.json", Hash: "g1", Size: 60, Mode: 0o644, Class: snapshot.ClassAuthored}), p2)
	mk(devs[1].ID, "mac-mini", 26*time.Hour, base, "")
	_ = recovery
	serveDemo(t, acct, w, devs, snaps, blobs, deseado, frase)
}

// serveDemo es el API falso de la demo: contesta lo justo para que la SPA
// tenga algo que pintar. No valida tokens a propósito — lo que se quiere mirar
// aquí son las pantallas, no el login.
func serveDemo(t *testing.T, acct *crypt.Account, w crypt.Wraps, devs []api.Device, snaps []sealedSnap,
	blobs map[string][]byte, deseado, frase string) {
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
	// El camino del editor: bajar contenidos, subir los editados, publicar el
	// snapshot y la revisión. Guarda de verdad en memoria, así que se puede
	// editar, publicar y volver a abrir lo publicado.
	mux.HandleFunc("GET /v1/blobs/{id}", func(rw http.ResponseWriter, r *http.Request) {
		b, ok := blobs[r.PathValue("id")]
		if !ok {
			http.Error(rw, `{"code":"not_found","message":"no está"}`, http.StatusNotFound)
			return
		}
		rw.Header().Set("Content-Type", "application/octet-stream")
		_, _ = rw.Write(b)
	})
	mux.HandleFunc("PUT /v1/blobs/{id}", func(rw http.ResponseWriter, r *http.Request) {
		b, err := io.ReadAll(io.LimitReader(r.Body, api.MaxBlobBytes))
		if err != nil {
			http.Error(rw, `{"code":"bad_request","message":"cuerpo ilegible"}`, http.StatusBadRequest)
			return
		}
		blobs[r.PathValue("id")] = b
		rw.WriteHeader(http.StatusCreated)
	})
	// `exists` sale del REGISTRO, no del bucket, como en el servidor de verdad:
	// un blob recién subido no está registrado hasta que se acepta el snapshot
	// que lo nombra. La demo lo imita porque, contestando desde el bucket,
	// tapaba que el portal dejara fuera del commit justo lo editado.
	conocidos := map[string]bool{}
	for id := range blobs {
		conocidos[id] = true
	}
	mux.HandleFunc("POST /v1/blobs/presign", func(rw http.ResponseWriter, r *http.Request) {
		var in api.PresignReq
		_ = json.NewDecoder(r.Body).Decode(&in)
		out := []api.PresignItem{}
		for _, id := range in.IDs {
			out = append(out, api.PresignItem{ID: id, Exists: conocidos[id]})
		}
		send(rw, out)
	})
	mux.HandleFunc("POST /v1/snapshots", func(rw http.ResponseWriter, r *http.Request) {
		var in api.SnapshotIn
		if !demoSnapshot(t, rw, r, acct, &in) {
			return
		}
		for _, id := range in.Blobs {
			if _, ok := blobs[id]; !ok {
				t.Errorf("el portal comprometió un snapshot que nombra un blob que no subió: %s", id)
				http.Error(rw, `{"code":"missing_blobs","message":"faltan blobs"}`, http.StatusConflict)
				return
			}
			conocidos[id] = true
		}
		meta := api.SnapshotMeta{ID: in.ID, Parent: in.Parent, DeviceID: devs[0].ID, DeviceName: "portal",
			Created: in.Created, Size: int64(len(in.Manifest))}
		snaps = append(snaps, sealedSnap{meta, api.Snapshot{SnapshotMeta: meta, Manifest: in.Manifest, Sig: in.Sig}})
		rw.WriteHeader(http.StatusCreated)
		send(rw, meta)
	})
	mux.HandleFunc("POST /v1/revisions", func(rw http.ResponseWriter, r *http.Request) {
		var in api.RevisionIn
		_ = json.NewDecoder(r.Body).Decode(&in)
		parts := crypt.RevisionParts{ID: in.ID, Prev: in.Prev, Device: in.DeviceID,
			Snapshot: in.Snapshot, Base: in.Base, Body: in.Body}
		if err := acct.VerifyRevision(parts, in.Sig); err != nil {
			t.Errorf("el portal publicó una revisión que no verifica: %v", err)
			http.Error(rw, `{"code":"bad_request","message":"firma inválida"}`, http.StatusBadRequest)
			return
		}
		t.Logf("revisión para %s sobre el snapshot %s (base %s)", in.DeviceID, in.Snapshot[:12], in.Base[:12])
		rw.WriteHeader(http.StatusCreated)
		send(rw, api.Revision{RevisionMeta: api.RevisionMeta{ID: in.ID, DeviceID: in.DeviceID,
			Snapshot: in.Snapshot, Base: in.Base, Created: in.Created, State: api.RevPending}})
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

// demoSnapshot valida lo que sube el portal como lo validaría el agente al
// bajarlo: firma, manifiesto que abre y id que corresponde. La demo es donde
// se mira la pantalla, pero si de paso puede decir «esto no lo podría aplicar
// ninguna máquina», lo dice.
func demoSnapshot(t *testing.T, rw http.ResponseWriter, r *http.Request, acct *crypt.Account, in *api.SnapshotIn) bool {
	t.Helper()
	if err := json.NewDecoder(r.Body).Decode(in); err != nil {
		http.Error(rw, `{"code":"bad_request","message":"cuerpo ilegible"}`, http.StatusBadRequest)
		return false
	}
	if err := acct.Verify(in.ID, in.Parent, in.Manifest, in.Sig); err != nil {
		t.Errorf("el portal subió un snapshot que no verifica: %v", err)
		http.Error(rw, `{"code":"bad_request","message":"firma inválida"}`, http.StatusBadRequest)
		return false
	}
	raw, err := acct.OpenManifest(in.ID, in.Manifest)
	if err != nil {
		t.Errorf("el manifiesto del portal no abre: %v", err)
		http.Error(rw, `{"code":"bad_request","message":"no abre"}`, http.StatusBadRequest)
		return false
	}
	var m snapshot.Manifest
	if err := json.Unmarshal(raw, &m); err != nil || acct.SnapshotID(m.ID) != in.ID {
		t.Errorf("el manifiesto no corresponde a su id: %v", err)
		http.Error(rw, `{"code":"bad_request","message":"id que no corresponde"}`, http.StatusBadRequest)
		return false
	}
	t.Logf("snapshot del portal: %s, %d elementos", in.ID[:12], len(m.Items))
	return true
}
