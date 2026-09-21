//go:build integration

package blobs_test

import (
	"os"
	"testing"

	"github.com/JoseAFlores777/ccp/internal/cloud/blobs"
	"github.com/JoseAFlores777/ccp/internal/cloud/blobs/blobstest"
)

// La prueba de contrato del almacenamiento (spec §10.4) contra un Alarik real,
// el de deploy/ccp-cloud/test-stack.yml:
//
//	CCP_CLOUD_TEST_S3_ENDPOINT=http://127.0.0.1:58080 \
//	  CCP_CLOUD_TEST_S3_ACCESS_KEY=<clave> CCP_CLOUD_TEST_S3_SECRET_KEY=<secreto> \
//	  go test -tags integration ./internal/cloud/blobs/
func TestS3Contract(t *testing.T) {
	ep := os.Getenv("CCP_CLOUD_TEST_S3_ENDPOINT")
	if ep == "" {
		t.Skip("CCP_CLOUD_TEST_S3_ENDPOINT no está definida")
	}
	// Endpoint y público son el mismo: en la pila de pruebas no hay dos nombres.
	blobstest.RunContract(t, blobs.NewS3(blobs.S3Config{
		Endpoint: ep, PublicEndpoint: ep, Region: "us-east-1", Bucket: "ccp-blobs",
		AccessKey: os.Getenv("CCP_CLOUD_TEST_S3_ACCESS_KEY"), SecretKey: os.Getenv("CCP_CLOUD_TEST_S3_SECRET_KEY"),
	}))
}
