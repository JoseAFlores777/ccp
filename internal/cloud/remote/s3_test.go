package remote_test

import (
	"strings"
	"testing"

	"github.com/JoseAFlores777/ccp/internal/cloud/remote"
)

func conCredenciales(t *testing.T) {
	t.Helper()
	t.Setenv(remote.EnvS3AccessKey, "llave")
	t.Setenv(remote.EnvS3SecretKey, "secreto")
}

func TestS3FromURLLeeLaDireccionYNoLasCredenciales(t *testing.T) {
	conCredenciales(t)
	c, err := remote.S3FromURL("s3://ccp-snaps/casa/mac?region=auto&endpoint=https://cuenta.r2.cloudflarestorage.com")
	if err != nil {
		t.Fatal(err)
	}
	if c.Bucket != "ccp-snaps" || c.Prefix != "casa/mac" || c.Region != "auto" ||
		c.Endpoint != "https://cuenta.r2.cloudflarestorage.com" {
		t.Fatalf("config = %+v", c)
	}
	if c.AccessKey != "llave" || c.SecretKey != "secreto" {
		t.Fatalf("credenciales = %q/%q", c.AccessKey, c.SecretKey)
	}
}

// Sin región no se puede firmar, así que hay una por defecto; el endpoint no
// la tiene porque contra AWS de verdad no hace falta.
func TestS3FromURLPoneRegionPorDefecto(t *testing.T) {
	conCredenciales(t)
	t.Setenv("AWS_REGION", "")
	t.Setenv("AWS_DEFAULT_REGION", "")
	c, err := remote.S3FromURL("s3://ccp-snaps")
	if err != nil || c.Region != "us-east-1" || c.Prefix != "" {
		t.Fatalf("config = %+v, %v", c, err)
	}
}

// La URL del destino se guarda en la configuración, se lista y se imprime: si
// llevara la clave dentro, acabaría en la pantalla y en un snapshot. Se
// rechaza en vez de aceptarla y callarse.
func TestS3FromURLRechazaCredencialesEnLaURL(t *testing.T) {
	conCredenciales(t)
	_, err := remote.S3FromURL("s3://llave:secreto@ccp-snaps/casa")
	if err == nil || !strings.Contains(err.Error(), remote.EnvS3AccessKey) {
		t.Fatalf("err = %v", err)
	}
}

// Sin credenciales no se puede ni preguntar si el bucket existe. El error dice
// los nombres de las variables porque es lo primero que uno va a buscar.
func TestS3FromURLSinCredencialesDiceQueDefinir(t *testing.T) {
	for _, n := range []string{remote.EnvS3AccessKey, remote.EnvS3SecretKey, "AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY"} {
		t.Setenv(n, "")
	}
	_, err := remote.S3FromURL("s3://ccp-snaps")
	if err == nil || !strings.Contains(err.Error(), remote.EnvS3AccessKey) ||
		!strings.Contains(err.Error(), "AWS_ACCESS_KEY_ID") {
		t.Fatalf("err = %v", err)
	}
}

func TestS3SeNombraSinCredenciales(t *testing.T) {
	b := remote.NewS3(remote.S3Config{Bucket: "ccp-snaps", Prefix: "casa", Region: "us-east-1",
		AccessKey: "llave", SecretKey: "secreto"})
	if b.Name() != "s3://ccp-snaps/casa" {
		t.Fatalf("Name = %q", b.Name())
	}
	if strings.Contains(b.Name(), "secreto") || strings.Contains(b.Name(), "llave") {
		t.Fatalf("Name enseña credenciales: %q", b.Name())
	}
}
