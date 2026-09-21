package blobs

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awshttp "github.com/aws/aws-sdk-go-v2/aws/transport/http"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// S3Config configura el almacenamiento S3-compatible.
type S3Config struct {
	// Endpoint es la dirección interna (http://alarik:8080) para HEAD y ping.
	Endpoint string
	// PublicEndpoint es la dirección pública (https://ccp-s3.joseiz.com) con la
	// que se firman las URLs. La firma SigV4 incluye el host: una URL firmada
	// para el nombre interno no valdría desde fuera.
	PublicEndpoint string
	Region         string
	Bucket         string
	AccessKey      string
	SecretKey      string
}

// S3 implementa Blobs sobre cualquier S3 compatible. Solo usa el subconjunto
// que prueba el contrato (PUT/GET prefirmados, HEAD de objeto y de bucket), así
// que cambiar Alarik por Garage, SeaweedFS o R2 es cambiar el endpoint.
type S3 struct {
	internal *s3.Client
	presign  *s3.PresignClient
	bucket   string
}

// NewS3 construye el cliente.
func NewS3(c S3Config) *S3 {
	creds := credentials.NewStaticCredentialsProvider(c.AccessKey, c.SecretKey, "")
	client := func(endpoint string) *s3.Client {
		return s3.New(s3.Options{
			Region:       c.Region,
			Credentials:  creds,
			BaseEndpoint: aws.String(endpoint),
			UsePathStyle: true,
			// Las sumas de comprobación «flexibles» que el SDK añade por defecto
			// desde 2025 meten cabeceras en las URLs prefirmadas que un S3
			// compatible puede no aceptar. Solo cuando la operación las exige.
			RequestChecksumCalculation: aws.RequestChecksumCalculationWhenRequired,
			ResponseChecksumValidation: aws.ResponseChecksumValidationWhenRequired,
		})
	}
	return &S3{internal: client(c.Endpoint), presign: s3.NewPresignClient(client(c.PublicEndpoint)), bucket: c.Bucket}
}

// PresignPut da una URL con la que el cliente sube key sin pasar por aquí.
func (b *S3) PresignPut(ctx context.Context, key string, ttl time.Duration) (string, error) {
	req, err := b.presign.PresignPutObject(ctx, &s3.PutObjectInput{Bucket: &b.bucket, Key: &key}, s3.WithPresignExpires(ttl))
	if err != nil {
		return "", err
	}
	return req.URL, nil
}

// PresignGet da una URL con la que el cliente baja key sin pasar por aquí.
func (b *S3) PresignGet(ctx context.Context, key string, ttl time.Duration) (string, error) {
	req, err := b.presign.PresignGetObject(ctx, &s3.GetObjectInput{Bucket: &b.bucket, Key: &key}, s3.WithPresignExpires(ttl))
	if err != nil {
		return "", err
	}
	return req.URL, nil
}

// Head dice si key existe y cuánto mide. Que no exista no es un error: es la
// respuesta a la pregunta que hace el servidor antes de aceptar un snapshot.
func (b *S3) Head(ctx context.Context, key string) (int64, bool, error) {
	out, err := b.internal.HeadObject(ctx, &s3.HeadObjectInput{Bucket: &b.bucket, Key: &key})
	if err != nil {
		var nf *types.NotFound
		var re *awshttp.ResponseError
		// HEAD no trae cuerpo, así que algunos S3 compatibles no llegan a
		// modelar el error: hay que mirar también el 404 pelado.
		if errors.As(err, &nf) || (errors.As(err, &re) && re.HTTPStatusCode() == http.StatusNotFound) {
			return 0, false, nil
		}
		return 0, false, err
	}
	return aws.ToInt64(out.ContentLength), true, nil
}

// Get baja key entera a memoria. Solo lo llama el camino del portal, cuyos
// blobs son configuración de kilobytes; el resto del mundo sigue yendo por URL
// prefirmada y no pasa por aquí.
func (b *S3) Get(ctx context.Context, key string) ([]byte, bool, error) {
	out, err := b.internal.GetObject(ctx, &s3.GetObjectInput{Bucket: &b.bucket, Key: &key})
	if err != nil {
		var nk *types.NoSuchKey
		var re *awshttp.ResponseError
		if errors.As(err, &nk) || (errors.As(err, &re) && re.HTTPStatusCode() == http.StatusNotFound) {
			return nil, false, nil
		}
		return nil, false, err
	}
	defer func() { _ = out.Body.Close() }()
	data, err := io.ReadAll(out.Body)
	if err != nil {
		return nil, false, err
	}
	return data, true, nil
}

// Put sube data a key.
func (b *S3) Put(ctx context.Context, key string, data []byte) error {
	_, err := b.internal.PutObject(ctx, &s3.PutObjectInput{
		Bucket: &b.bucket, Key: &key, Body: bytes.NewReader(data), ContentLength: aws.Int64(int64(len(data)))})
	return err
}

// Ping comprueba que el bucket está ahí (lo usa /readyz).
func (b *S3) Ping(ctx context.Context) error {
	_, err := b.internal.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: &b.bucket})
	return err
}
