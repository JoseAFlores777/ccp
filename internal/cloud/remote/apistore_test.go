package remote_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/JoseAFlores777/ccp/internal/cloud/api"
	"github.com/JoseAFlores777/ccp/internal/cloud/blobs/blobstest"
	"github.com/JoseAFlores777/ccp/internal/cloud/client"
	"github.com/JoseAFlores777/ccp/internal/cloud/oidctest"
	"github.com/JoseAFlores777/ccp/internal/cloud/remote"
	"github.com/JoseAFlores777/ccp/internal/cloud/server"
	"github.com/JoseAFlores777/ccp/internal/cloud/store"
)

// abrirNube levanta el API de verdad (server.New) con persistencia y
// almacenamiento en memoria y un Keycloak falso, y devuelve la nube detrás de
// la interfaz de destino. Un usuario distinto por llamada: cada subtest del
// contrato empieza sin bóveda y sin historia.
func abrirNube(t *testing.T) remote.Remote {
	t.Helper()
	iss := oidctest.New(t)
	h := server.New(server.Config{
		Store: store.NewMem(), Blobs: blobstest.New(t),
		Verifier: server.NewOIDCVerifier(iss.URL, iss.JWKSURL(), oidctest.Audience),
		Issuer:   iss.URL, ClientID: oidctest.ClientID,
	})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	ctx := t.Context()
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
	if err := files.SaveToken(tok); err != nil {
		t.Fatal(err)
	}
	a := client.NewAPI(srv.URL, client.HTTPClient(ctx, oc, files, tok), "")
	d, err := a.RegisterDevice(ctx, api.DeviceIn{Name: "equipo-" + t.Name(), Platform: "test"})
	if err != nil {
		t.Fatal(err)
	}
	return remote.NewAPIStore(a.WithDevice(d.ID), srv.URL)
}

// La prueba de que la abstracción es una y no dos: el mismo contrato que
// cumple la carpeta lo cumple la nube, con sus mismos «no hay bóveda», «ya
// hay bóveda» y «faltan blobs».
func TestNubeCumpleElContrato(t *testing.T) {
	runContract(t, abrirNube)
	runChainContract(t, abrirNube)
}
