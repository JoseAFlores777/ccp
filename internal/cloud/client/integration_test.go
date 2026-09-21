//go:build integration

package client

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"testing"

	"github.com/JoseAFlores777/ccp/internal/cloud/blobs"
	"github.com/JoseAFlores777/ccp/internal/cloud/crypt"
	"github.com/JoseAFlores777/ccp/internal/cloud/store"
)

// El flujo completo contra la pila de verdad (deploy/ccp-cloud/test-stack.yml):
// el SQL, las migraciones y la transacción del commit los pone Postgres, y las
// URLs prefirmadas que firma el SDK de AWS las tiene que aceptar Alarik. Es lo
// que el Store de memoria y el blobstest no pueden probar por construcción.
func TestPushPullOnRealStack(t *testing.T) {
	dsn, ep := os.Getenv("CCP_CLOUD_TEST_DSN"), os.Getenv("CCP_CLOUD_TEST_S3_ENDPOINT")
	if dsn == "" || ep == "" {
		t.Skip("falta la pila de integración (deploy/ccp-cloud/integration.sh)")
	}
	ctx := context.Background()
	pg, err := store.OpenPG(ctx, dsn, "")
	if err != nil {
		t.Fatal(err)
	}
	defer pg.Close()
	if err := pg.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	// Endpoint interno y público son el mismo: en la pila de pruebas no hay dos
	// nombres, así que la firma SigV4 vale desde donde se hizo.
	bl := blobs.NewS3(blobs.S3Config{
		Endpoint: ep, PublicEndpoint: ep, Region: "us-east-1", Bucket: "ccp-blobs",
		AccessKey: os.Getenv("CCP_CLOUD_TEST_S3_ACCESS_KEY"),
		SecretKey: os.Getenv("CCP_CLOUD_TEST_S3_SECRET_KEY"),
	})
	r := newRigWith(t, pg, bl)
	// Un sub nuevo en cada vuelta: la base sobrevive al test y puede sobrevivir
	// a la ejecución entera (la pila se puede levantar a mano), así que repetir
	// el sub haría que la segunda vuelta viera los snapshots de la primera.
	r.iss.As("it-"+t.Name()+"-"+randomSub(t), "it@example.com")

	filesA, apiA, stA := r.machine(t, "it-a")
	acctA, _, m := seed(t, apiA, stA)
	rep, err := Push(ctx, apiA, acctA, stA, filesA, "")
	if err != nil || rep.Snapshots != 1 || rep.Uploaded != 2 {
		t.Fatalf("Push = %+v, %v", rep, err)
	}

	filesB, apiB, stB := r.machine(t, "it-b")
	list, err := apiB.Snapshots(ctx, "", 10)
	if err != nil || len(list) != 1 || list[0].DeviceName != "it-a" {
		t.Fatalf("Snapshots = %+v, %v", list, err)
	}
	v, err := apiB.Vault(ctx)
	if err != nil {
		t.Fatal(err)
	}
	w, err := VaultFromAPI(v)
	if err != nil {
		t.Fatal(err)
	}
	ak, err := crypt.UnlockPassphrase(w, []byte(phrase))
	if err != nil {
		t.Fatal(err)
	}
	acctB, _ := crypt.NewAccount(ak)
	got, missing, err := Pull(ctx, apiB, acctB, stB, filesB, list[0].ID)
	if err != nil || got.ID != m.ID || len(missing) != 0 {
		t.Fatalf("Pull = %v, %v, %v", got, missing, err)
	}
	// El secreto viaja sellado y vuelve a salir entero: es el único modo de
	// saber que el PUT y el GET prefirmados de Alarik movieron los mismos bytes.
	if data, err := stB.GetBlob(m.Items[1].Hash); err != nil || string(data) != "sk-1" {
		t.Fatalf("el secreto no llegó: %q %v", data, err)
	}
}

// randomSub da el sufijo único del sub. GITHUB_RUN_ID no sirve: en local está
// vacío y dos ejecuciones seguidas compartirían usuario.
func randomSub(t *testing.T) string {
	t.Helper()
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(b[:])
}
