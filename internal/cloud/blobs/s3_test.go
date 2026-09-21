package blobs

import (
	"net/url"
	"strings"
	"testing"
	"time"
)

var _ Blobs = (*S3)(nil)

func testS3() *S3 {
	return NewS3(S3Config{
		Endpoint:       "http://alarik:8080",
		PublicEndpoint: "https://ccp-s3.example.com",
		Region:         "us-east-1",
		Bucket:         "ccp-blobs",
		AccessKey:      "llave",
		SecretKey:      "secreto",
	})
}

// La firma SigV4 incluye el host, así que las URLs que se le dan al cliente
// tienen que salir del endpoint público: firmadas con el nombre interno no
// valdrían desde fuera, y el fallo solo se vería en producción.
func TestPresignUsaElEndpointPublico(t *testing.T) {
	b := testS3()
	key := Key("u1", strings.Repeat("ab", 32))
	for _, c := range []struct {
		nombre string
		url    func() (string, error)
	}{
		{"put", func() (string, error) { return b.PresignPut(t.Context(), key, time.Minute) }},
		{"get", func() (string, error) { return b.PresignGet(t.Context(), key, time.Minute) }},
	} {
		raw, err := c.url()
		if err != nil {
			t.Fatalf("%s: %v", c.nombre, err)
		}
		u, err := url.Parse(raw)
		if err != nil {
			t.Fatalf("%s: %v", c.nombre, err)
		}
		if u.Host != "ccp-s3.example.com" || u.Scheme != "https" {
			t.Errorf("%s firmado para %s://%s", c.nombre, u.Scheme, u.Host)
		}
		// Estilo de ruta: el bucket va en la ruta, no en el nombre del host
		// (un S3 compatible no tiene DNS por bucket).
		if want := "/ccp-blobs/" + key; u.Path != want {
			t.Errorf("%s: ruta %q, se esperaba %q", c.nombre, u.Path, want)
		}
		if u.Query().Get("X-Amz-Signature") == "" {
			t.Errorf("%s: sin firma: %s", c.nombre, raw)
		}
	}
}
