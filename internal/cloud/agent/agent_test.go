package agent

// Una máquina entera cabe en tres t.TempDir(): CCP_HOME, ~/.claude y el
// almacén de snapshots. El servidor es el de verdad (server.New) con
// persistencia en memoria, así que lo que se prueba es el camino completo:
// publicar, recoger, verificar la firma, reconciliar y escribir.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/JoseAFlores777/ccp/internal/cloud/api"
	"github.com/JoseAFlores777/ccp/internal/cloud/blobs/blobstest"
	"github.com/JoseAFlores777/ccp/internal/cloud/client"
	"github.com/JoseAFlores777/ccp/internal/cloud/crypt"
	"github.com/JoseAFlores777/ccp/internal/cloud/oidctest"
	"github.com/JoseAFlores777/ccp/internal/cloud/server"
	"github.com/JoseAFlores777/ccp/internal/cloud/store"
	"github.com/JoseAFlores777/ccp/internal/core"
	"github.com/JoseAFlores777/ccp/internal/snapshot"
)

type maquina struct {
	t    *testing.T
	o    Opts
	url  string
	acct *crypt.Account
	dev  string
	hc   *http.Client // el cliente con token, para montar otra API contra un proxy
}

func write(t *testing.T, path, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// nueva levanta el servidor, registra el equipo, crea la bóveda y devuelve una
// máquina con su configuración vacía lista para capturar.
func nueva(t *testing.T) *maquina {
	t.Helper()
	ctx := context.Background()
	iss := oidctest.New(t)
	h := server.New(server.Config{Store: store.NewMem(), Blobs: blobstest.New(t),
		Verifier: server.NewOIDCVerifier(iss.URL, iss.JWKSURL(), oidctest.Audience),
		Issuer:   iss.URL, ClientID: oidctest.ClientID})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	files := client.NewFiles(t.TempDir())
	ep, err := client.Discover(ctx, http.DefaultClient, srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	oc := client.OAuthConfig(ep.Info.ClientID, ep.TokenURL, ep.DeviceAuthURL)
	tok, err := client.Login(ctx, oc, func(string, string) {})
	if err != nil {
		t.Fatal(err)
	}
	hc := client.HTTPClient(ctx, oc, files, tok)
	a := client.NewAPI(srv.URL, hc, "")
	d, err := a.RegisterDevice(ctx, api.DeviceIn{Name: "mac", Platform: "test"})
	if err != nil {
		t.Fatal(err)
	}
	a = a.WithDevice(d.ID) // desde aquí, todo habla como este dispositivo
	ak, _, w, err := crypt.NewVault([]byte("frase de la bóveda larga"))
	if err != nil {
		t.Fatal(err)
	}
	v, err := client.VaultToAPI(w)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.PutVault(ctx, v); err != nil {
		t.Fatal(err)
	}
	acct, err := crypt.NewAccount(ak)
	if err != nil {
		t.Fatal(err)
	}
	st, err := snapshot.Open(filepath.Join(t.TempDir(), "snapshots"))
	if err != nil {
		t.Fatal(err)
	}
	// El data dir de Desktop de `default` va a un temporal: el inventario lo
	// mira y no se toca el de verdad ni para leer.
	t.Setenv("CCP_DESKTOP_DEFAULT_DATA_DIR", t.TempDir())
	return &maquina{t: t, url: srv.URL, acct: acct, dev: d.ID, hc: hc, o: Opts{
		Home: t.TempDir(), Src: filepath.Join(t.TempDir(), ".claude"),
		API: a, Acct: acct, Store: st, Files: files,
		DeviceID: d.ID, Policy: PolicyAuto, Machine: "mac", Now: time.Now,
	}}
}

// captura toma un snapshot del estado vivo y lo sube; devuelve su id en la nube.
func (m *maquina) captura() string {
	m.t.Helper()
	sn, err := core.SnapshotCapture(m.o.Home, m.o.Src, m.o.Store, core.SnapshotCaptureOpts{
		Trigger: "manual", Force: true, Now: time.Now(), Machine: "mac"})
	if err != nil {
		m.t.Fatal(err)
	}
	if _, err := client.Push(context.Background(), m.o.API, m.acct, m.o.Store, m.o.Files, sn.ID); err != nil {
		m.t.Fatal(err)
	}
	state, err := m.o.Files.LoadState()
	if err != nil {
		m.t.Fatal(err)
	}
	return state.Pushed[sn.ID]
}

// publica pone una revisión deseada para esta máquina, firmada como lo haría
// el portal con la clave de cuenta.
func (m *maquina) publica(id, snap, base string) api.Revision {
	m.t.Helper()
	parts := crypt.RevisionParts{ID: id, Device: m.dev, Snapshot: snap, Base: base}
	rev, err := m.o.API.PublishRevision(context.Background(), api.RevisionIn{
		ID: id, DeviceID: m.dev, Snapshot: snap, Base: base,
		Sig: m.acct.SignRevision(parts), Created: time.Now()})
	if err != nil {
		m.t.Fatal(err)
	}
	return rev
}

func (m *maquina) estado(id string) api.RevisionMeta {
	m.t.Helper()
	list, err := m.o.API.Revisions(context.Background(), m.dev, 10)
	if err != nil {
		m.t.Fatal(err)
	}
	for _, r := range list {
		if r.ID == id {
			return r
		}
	}
	m.t.Fatalf("no está la revisión %s", id)
	return api.RevisionMeta{}
}

func revID(n byte) string {
	b := make([]byte, 64)
	for i := range b {
		b[i] = '0'
	}
	b[63] = "0123456789abcdef"[n%16]
	b[62] = "0123456789abcdef"[(n/16)%16]
	return string(b)
}

// El camino entero: lo que no ejecuta nada se aplica solo; el hook espera a
// una persona y la revisión NO se cierra hasta que la hay.
func TestAgenteAplicaLoInocenteYDejaElHookEsperando(t *testing.T) {
	ctx := context.Background()
	m := nueva(t)
	write(t, filepath.Join(m.o.Src, "CLAUDE.md"), "uno")
	write(t, filepath.Join(m.o.Src, "hooks", "x.sh"), "echo 1")
	snap := m.captura()

	write(t, filepath.Join(m.o.Src, "CLAUDE.md"), "dos")
	write(t, filepath.Join(m.o.Src, "hooks", "x.sh"), "echo 2")

	m.publica(revID(1), snap, "")
	out, err := Once(ctx, m.o)
	if err != nil {
		t.Fatal(err)
	}
	if !out.Waiting || len(out.Pending) != 1 || out.Pending[0].LPath != "claude/hooks/x.sh" {
		t.Fatalf("el hook tenía que quedarse esperando: %+v", out)
	}
	if !has(out.Pending[0].Why, DangerHooks) {
		t.Fatalf("y decir por qué: %+v", out.Pending[0])
	}
	if got := read(t, filepath.Join(m.o.Src, "CLAUDE.md")); got != "uno" {
		t.Fatalf("las instrucciones se aplican solas; quedó %q", got)
	}
	if got := read(t, filepath.Join(m.o.Src, "hooks", "x.sh")); got != "echo 2" {
		t.Fatalf("el hook no se toca sin confirmar; quedó %q", got)
	}
	if st := m.estado(revID(1)); st.State != api.RevPending {
		t.Fatalf("la revisión no se cierra mientras espera a una persona: %s", st.State)
	}
	if out.PreSnapshot == "" {
		t.Fatal("antes de aplicar hay snapshot")
	}
}

// preparaHookPendiente deja la máquina con el hook esperando confirmación.
func preparaHookPendiente(t *testing.T) (*maquina, string) {
	t.Helper()
	m := nueva(t)
	write(t, filepath.Join(m.o.Src, "CLAUDE.md"), "uno")
	write(t, filepath.Join(m.o.Src, "hooks", "x.sh"), "echo 1")
	snap := m.captura()
	write(t, filepath.Join(m.o.Src, "hooks", "x.sh"), "echo 2")
	m.publica(revID(2), snap, "")
	if _, err := Once(context.Background(), m.o); err != nil {
		t.Fatal(err)
	}
	return m, revID(2)
}

func TestConfirmarAplicaElHookYCierraLaRevision(t *testing.T) {
	m, id := preparaHookPendiente(t)
	out, err := Resolve(context.Background(), m.o, []string{"claude/hooks/x.sh"})
	if err != nil {
		t.Fatal(err)
	}
	if out.State != api.RevApplied {
		t.Fatalf("esperaba applied: %+v", out)
	}
	if got := read(t, filepath.Join(m.o.Src, "hooks", "x.sh")); got != "echo 1" {
		t.Fatalf("el hook confirmado se escribe; quedó %q", got)
	}
	if st := m.estado(id); st.State != api.RevApplied {
		t.Fatalf("el portal tiene que verlo aplicado: %+v", st)
	}
	if _, ok, _ := LoadReview(m.o.Files); ok {
		t.Fatal("ya no queda nada que revisar")
	}
}

// Rechazar es una respuesta, no un fallo: se cierra como parcial y se dice qué
// quedó fuera, que es lo que el usuario irá a leer al portal.
func TestRechazarDejaElHookComoEstabaYCierraParcial(t *testing.T) {
	m, id := preparaHookPendiente(t)
	out, err := Resolve(context.Background(), m.o, nil)
	if err != nil {
		t.Fatal(err)
	}
	if out.State != api.RevPartial || out.Reason == "" {
		t.Fatalf("esperaba partial con motivo: %+v", out)
	}
	if got := read(t, filepath.Join(m.o.Src, "hooks", "x.sh")); got != "echo 2" {
		t.Fatalf("lo rechazado no se escribe; quedó %q", got)
	}
	if st := m.estado(id); st.State != api.RevPartial || st.Reason == "" {
		t.Fatalf("el motivo tiene que llegar al portal: %+v", st)
	}
}

// Mientras espera a una persona, el agente no repite el trabajo: ni otro
// snapshot previo ni una lista distinta de la que se le enseñó.
func TestOtraPasadaNoRehaceLoQueEsperaConfirmacion(t *testing.T) {
	m, _ := preparaHookPendiente(t)
	antes, err := m.o.Store.List()
	if err != nil {
		t.Fatal(err)
	}
	out, err := Once(context.Background(), m.o)
	if err != nil {
		t.Fatal(err)
	}
	if !out.Waiting || len(out.Pending) != 1 {
		t.Fatalf("tenía que seguir esperando lo mismo: %+v", out)
	}
	if out.PreSnapshot != "" {
		t.Fatal("no se vuelve a fotografiar nada")
	}
	despues, err := m.o.Store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(antes) != len(despues) {
		t.Fatalf("el almacén creció de %d a %d", len(antes), len(despues))
	}
}

// Tres bandas de verdad: con base, lo que cambió en los dos sitios choca y no
// se escribe; lo que solo cambió en la revisión, sí.
func TestConBaseLoQueCambioEnLosDosSitiosChoca(t *testing.T) {
	ctx := context.Background()
	m := nueva(t)
	write(t, filepath.Join(m.o.Src, "CLAUDE.md"), "uno")
	write(t, filepath.Join(m.o.Src, "output-styles", "s.md"), "estilo 1")
	base := m.captura()

	// El portal parte de la base y cambia las dos cosas…
	write(t, filepath.Join(m.o.Src, "CLAUDE.md"), "del portal")
	write(t, filepath.Join(m.o.Src, "output-styles", "s.md"), "estilo 2")
	deseado := m.captura()
	// …y aquí, mientras tanto, se había tocado solo una.
	write(t, filepath.Join(m.o.Src, "CLAUDE.md"), "mío")
	write(t, filepath.Join(m.o.Src, "output-styles", "s.md"), "estilo 1")

	m.publica(revID(3), deseado, base)
	out, err := Once(ctx, m.o)
	if err != nil {
		t.Fatal(err)
	}
	if out.State != api.RevConflict || len(out.Conflicts) != 1 || out.Conflicts[0] != "claude/CLAUDE.md" {
		t.Fatalf("esperaba un choque en CLAUDE.md: %+v", out)
	}
	if got := read(t, filepath.Join(m.o.Src, "CLAUDE.md")); got != "mío" {
		t.Fatalf("lo que choca no se pisa; quedó %q", got)
	}
	if got := read(t, filepath.Join(m.o.Src, "output-styles", "s.md")); got != "estilo 2" {
		t.Fatalf("lo que solo cambió arriba sí se aplica; quedó %q", got)
	}
	if st := m.estado(revID(3)); st.State != api.RevConflict || st.Reason == "" {
		t.Fatalf("el portal tiene que ver el choque con su motivo: %+v", st)
	}
}

// Una orden que no verifica no es una orden: no se aplica nada y se dice por
// qué, que es justo donde el usuario va a mirar si alguien está intentando algo.
func TestUnaFirmaQueNoVerificaNoTocaNada(t *testing.T) {
	ctx := context.Background()
	m := nueva(t)
	write(t, filepath.Join(m.o.Src, "CLAUDE.md"), "uno")
	snap := m.captura()
	write(t, filepath.Join(m.o.Src, "CLAUDE.md"), "dos")

	sig := m.acct.SignRevision(crypt.RevisionParts{ID: revID(4), Device: m.dev, Snapshot: snap})
	sig[0] ^= 0xff
	if _, err := m.o.API.PublishRevision(ctx, api.RevisionIn{ID: revID(4), DeviceID: m.dev,
		Snapshot: snap, Sig: sig, Created: time.Now()}); err != nil {
		t.Fatal(err)
	}
	out, err := Once(ctx, m.o)
	if err != nil {
		t.Fatal(err)
	}
	if out.State != api.RevFailed {
		t.Fatalf("esperaba failed: %+v", out)
	}
	if got := read(t, filepath.Join(m.o.Src, "CLAUDE.md")); got != "dos" {
		t.Fatalf("no se toca nada; quedó %q", got)
	}
}

// En `manual` ni siquiera unas instrucciones cambian solas.
func TestPoliticaManualLoConfirmaTodo(t *testing.T) {
	ctx := context.Background()
	m := nueva(t)
	m.o.Policy = PolicyManual
	write(t, filepath.Join(m.o.Src, "CLAUDE.md"), "uno")
	snap := m.captura()
	write(t, filepath.Join(m.o.Src, "CLAUDE.md"), "dos")

	m.publica(revID(5), snap, "")
	out, err := Once(ctx, m.o)
	if err != nil {
		t.Fatal(err)
	}
	if !out.Waiting || len(out.Pending) != 1 || out.Pending[0].LPath != "claude/CLAUDE.md" {
		t.Fatalf("en manual espera hasta lo inocente: %+v", out)
	}
	if got := read(t, filepath.Join(m.o.Src, "CLAUDE.md")); got != "dos" {
		t.Fatalf("y no escribe; quedó %q", got)
	}
}

func TestSinRevisionPendienteNoHayNadaQueHacer(t *testing.T) {
	m := nueva(t)
	if _, err := Once(context.Background(), m.o); !errors.Is(err, ErrNothing) {
		t.Fatalf("esperaba ErrNothing, salió %v", err)
	}
	if _, err := Resolve(context.Background(), m.o, nil); !errors.Is(err, ErrNothing) {
		t.Fatalf("revisar sin nada que revisar: %v", err)
	}
}

// El bucle informa de lo que hace y se para al cancelar. Y un error no lo
// mata: la siguiente vuelta lo vuelve a intentar.
func TestElBucleInformaYSeParaAlCancelar(t *testing.T) {
	m := nueva(t)
	write(t, filepath.Join(m.o.Src, "CLAUDE.md"), "uno")
	snap := m.captura()
	write(t, filepath.Join(m.o.Src, "CLAUDE.md"), "dos")
	m.publica(revID(6), snap, "")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	visto := make(chan *Outcome, 4)
	done := make(chan struct{})
	go func() {
		Loop(ctx, m.o, 10*time.Millisecond, func(out *Outcome, err error) {
			if err != nil {
				t.Errorf("no esperaba error: %v", err)
			}
			select {
			case visto <- out:
			default:
			}
		})
		close(done)
	}()
	select {
	case out := <-visto:
		if out.State != api.RevApplied {
			t.Fatalf("esperaba applied: %+v", out)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("el bucle no informó de nada")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("el bucle no se paró")
	}
}

// Un motivo larguísimo se recorta para que el servidor lo acepte: el tope es
// de BYTES, así que contar el «…» fuera del presupuesto dejaba el motivo dos
// bytes por encima y la revisión no se cerraba nunca.
func TestUnMotivoLarguisimoSeRecortaYLaRevisionSeCierra(t *testing.T) {
	ctx := context.Background()
	m := nueva(t)
	write(t, filepath.Join(m.o.Src, "CLAUDE.md"), "uno")
	for i := 0; i < 60; i++ {
		write(t, filepath.Join(m.o.Src, "hooks", fmt.Sprintf("acentuación-%02d.sh", i)), "echo 1")
	}
	snap := m.captura()
	for i := 0; i < 60; i++ {
		write(t, filepath.Join(m.o.Src, "hooks", fmt.Sprintf("acentuación-%02d.sh", i)), "echo 2")
	}
	m.publica(revID(9), snap, "")
	if _, err := Once(ctx, m.o); err != nil {
		t.Fatal(err)
	}
	out, err := Resolve(ctx, m.o, nil) // se rechazan todos: motivo enorme
	if err != nil {
		t.Fatalf("cerrar la revisión no puede fallar por la longitud del motivo: %v", err)
	}
	if len(out.Reason) > api.MaxReasonLen {
		t.Fatalf("el motivo se pasa del tope: %d bytes", len(out.Reason))
	}
	if !utf8.ValidString(out.Reason) {
		t.Fatal("el recorte partió una runa")
	}
	if st := m.estado(revID(9)); st.State != api.RevPartial {
		t.Fatalf("el portal tiene que verla cerrada: %+v", st)
	}
}

// En manual lo pendiente no tiene motivos, pero `why` viaja como [] y nunca
// como null: `review.json` y la respuesta de `cloud.review` los lee la GUI, que
// hace `p.why.map(...)` — un null ahí deja la pantalla Nube en blanco.
func TestPendienteSinMotivosSerializaListaVacia(t *testing.T) {
	ctx := context.Background()
	m := nueva(t)
	m.o.Policy = PolicyManual
	write(t, filepath.Join(m.o.Src, "CLAUDE.md"), "uno")
	snap := m.captura()
	write(t, filepath.Join(m.o.Src, "CLAUDE.md"), "dos")

	m.publica(revID(50), snap, "")
	out, err := Once(ctx, m.o)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Pending) != 1 || out.Pending[0].Why == nil {
		t.Fatalf("why no puede ser nil: %+v", out.Pending)
	}

	b, err := os.ReadFile(filepath.Join(m.o.Files.Dir, ReviewFile))
	if err != nil {
		t.Fatal(err)
	}
	var crudo struct {
		Pending []struct {
			Why json.RawMessage `json:"why"`
		} `json:"pending"`
	}
	if err := json.Unmarshal(b, &crudo); err != nil {
		t.Fatal(err)
	}
	if len(crudo.Pending) != 1 || string(crudo.Pending[0].Why) != "[]" {
		t.Fatalf("review.json debería llevar \"why\": [], lleva %s", b)
	}

	r, ok, err := LoadReview(m.o.Files)
	if err != nil || !ok {
		t.Fatalf("no se pudo releer la revisión: %v %v", ok, err)
	}
	if len(r.Pending) != 1 || r.Pending[0].Why == nil {
		t.Fatalf("al releer, why sigue siendo nil: %+v", r.Pending)
	}
}

// Si informar del resultado falla, `review.json` NO se borra: borrarlo antes
// de cerrar dejaba la revisión pendiente sin nada que la recuerde, y la
// siguiente pasada del agente volvía a preguntar por las mismas rutas que la
// persona acababa de rechazar, en bucle cada --interval.
func TestSiNoSePuedeCerrarLaRevisionElReviewSigueAhi(t *testing.T) {
	ctx := context.Background()
	m := nueva(t)
	m.o.Policy = PolicyManual // todo pasa por confirmación
	write(t, filepath.Join(m.o.Src, "CLAUDE.md"), "uno")
	snap := m.captura()
	write(t, filepath.Join(m.o.Src, "CLAUDE.md"), "dos")
	m.publica(revID(10), snap, "")
	if _, err := Once(ctx, m.o); err != nil {
		t.Fatal(err)
	}
	// Un proxy que deja pasar todo menos el cierre: así falla exactamente la
	// llamada que nos importa, con el resto del camino intacto.
	tgt, err := url.Parse(m.url)
	if err != nil {
		t.Fatal(err)
	}
	rp := httputil.NewSingleHostReverseProxy(tgt)
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/state") {
			http.Error(w, "no hoy", http.StatusInternalServerError)
			return
		}
		rp.ServeHTTP(w, r)
	}))
	defer proxy.Close()
	m.o.API = client.NewAPI(proxy.URL, m.hc, "").WithDevice(m.dev)

	if _, err := Resolve(ctx, m.o, nil); err == nil {
		t.Fatal("esperaba el error de cerrar la revisión")
	}
	if _, ok, err := LoadReview(m.o.Files); err != nil {
		t.Fatal(err)
	} else if !ok {
		t.Fatal("review.json se borró aunque la revisión sigue pendiente")
	}
}
