//go:build integration

package remote_test

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"testing"

	"github.com/JoseAFlores777/ccp/internal/cloud/remote"
)

// El mismo contrato que cumple la carpeta, contra un S3 de verdad (el Alarik
// de deploy/ccp-cloud/test-stack.yml):
//
//	CCP_CLOUD_TEST_S3_ENDPOINT=http://127.0.0.1:58080 \
//	  CCP_CLOUD_TEST_S3_ACCESS_KEY=<clave> CCP_CLOUD_TEST_S3_SECRET_KEY=<secreto> \
//	  go test -tags integration ./internal/cloud/remote/
//
// Lo que esto prueba y el test de unidad no puede: que ListObjectsV2 pagina
// como se espera, que un HEAD de algo que no está llega como 404 modelado o
// pelado, y que el estilo de ruta con endpoint propio es el correcto.
func TestS3CumpleElContrato(t *testing.T) {
	ep := os.Getenv("CCP_CLOUD_TEST_S3_ENDPOINT")
	if ep == "" {
		t.Skip("CCP_CLOUD_TEST_S3_ENDPOINT no está definida")
	}
	abrir := func(t *testing.T) remote.Remote {
		t.Helper()
		// Un prefijo nuevo por subtest: el bucket sobrevive a la ejecución, y
		// compartir prefijo haría que un test viera la historia de otro.
		b := make([]byte, 8)
		if _, err := rand.Read(b); err != nil {
			t.Fatal(err)
		}
		return remote.New(remote.NewS3(remote.S3Config{
			Bucket: "ccp-blobs", Prefix: "sync-test/" + hex.EncodeToString(b), Region: "us-east-1",
			Endpoint:  ep,
			AccessKey: os.Getenv("CCP_CLOUD_TEST_S3_ACCESS_KEY"),
			SecretKey: os.Getenv("CCP_CLOUD_TEST_S3_SECRET_KEY"),
		}))
	}
	runContract(t, abrir)
	runChainContract(t, abrir)
}
