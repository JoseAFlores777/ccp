package remote

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awshttp "github.com/aws/aws-sdk-go-v2/aws/transport/http"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"

	"github.com/JoseAFlores777/ccp/internal/cloud/api"
	"github.com/JoseAFlores777/ccp/internal/cloud/blobs"
)

// S3Config es un bucket S3 compatible: R2, un MinIO, un Garage, el Alarik de
// la pila de pruebas o S3 de verdad.
type S3Config struct {
	Bucket string
	// Prefix es la carpeta dentro del bucket, sin barra al final. Vacío = la
	// raíz. Permite compartir bucket con otra cosa sin mezclarse.
	Prefix   string
	Region   string
	Endpoint string
	// Las credenciales NO salen de la URL: salen del entorno (S3FromURL). Un
	// destino se guarda en la configuración y se enseña en la pantalla.
	AccessKey string
	SecretKey string
}

// Variables de entorno de las credenciales. Las propias de ccp primero, para
// poder tener un bucket de snapshots distinto del de trabajo, y las de AWS
// después, que es lo que ya hay configurado en la mayoría de las máquinas.
const (
	EnvS3AccessKey = "CCP_SYNC_S3_ACCESS_KEY"
	EnvS3SecretKey = "CCP_SYNC_S3_SECRET_KEY"
)

func firstEnv(names ...string) string {
	for _, n := range names {
		if v := os.Getenv(n); v != "" {
			return v
		}
	}
	return ""
}

// S3FromURL lee `s3://bucket/prefijo?region=…&endpoint=…`. El endpoint y la
// región van en la URL porque son parte de la dirección; las credenciales no,
// porque la URL se guarda, se lista y se imprime.
func S3FromURL(raw string) (S3Config, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return S3Config{}, fmt.Errorf("URL de destino inválida: %w", err)
	}
	if u.User != nil {
		return S3Config{}, errors.New("la URL del destino no lleva credenciales: ponlas en " +
			EnvS3AccessKey + " y " + EnvS3SecretKey)
	}
	if u.Host == "" {
		return S3Config{}, errors.New("falta el bucket: s3://bucket/prefijo")
	}
	q := u.Query()
	c := S3Config{
		Bucket: u.Host, Prefix: strings.Trim(u.Path, "/"),
		Region:   firstEnv2(q.Get("region"), "AWS_REGION", "AWS_DEFAULT_REGION"),
		Endpoint: firstEnv2(q.Get("endpoint"), "AWS_ENDPOINT_URL_S3", "AWS_ENDPOINT_URL"),
		// Sin claves no se puede ni preguntar si el bucket existe, así que el
		// error dice los nombres: es lo primero que uno busca.
		AccessKey: firstEnv(EnvS3AccessKey, "AWS_ACCESS_KEY_ID"),
		SecretKey: firstEnv(EnvS3SecretKey, "AWS_SECRET_ACCESS_KEY"),
	}
	if c.Region == "" {
		c.Region = "us-east-1"
	}
	if c.AccessKey == "" || c.SecretKey == "" {
		return S3Config{}, fmt.Errorf("faltan las credenciales del bucket: define %s y %s (o AWS_ACCESS_KEY_ID y AWS_SECRET_ACCESS_KEY)",
			EnvS3AccessKey, EnvS3SecretKey)
	}
	return c, nil
}

func firstEnv2(v string, names ...string) string {
	if v != "" {
		return v
	}
	return firstEnv(names...)
}

// OpenS3 abre el bucket de una URL.
func OpenS3(raw string) (Objects, error) {
	c, err := S3FromURL(raw)
	if err != nil {
		return nil, err
	}
	return NewS3(c), nil
}

// S3 es el transporte sobre un bucket. Solo usa lo que cualquier S3
// compatible implementa: HEAD, GET, PUT y ListObjectsV2.
type S3 struct {
	c      *s3.Client
	cfg    S3Config
	prefix string
}

// NewS3 construye el cliente.
func NewS3(cfg S3Config) *S3 {
	client := s3.New(s3.Options{
		Region:      cfg.Region,
		Credentials: credentials.NewStaticCredentialsProvider(cfg.AccessKey, cfg.SecretKey, ""),
		// Un endpoint propio es casi siempre un S3 autoalojado, que exige el
		// estilo de ruta; contra AWS de verdad se deja el de host virtual,
		// que es el único que no está en retirada.
		UsePathStyle: cfg.Endpoint != "",
		// Las sumas «flexibles» que el SDK añade por defecto desde 2025 meten
		// cabeceras que un S3 compatible puede no aceptar (igual que en
		// internal/cloud/blobs).
		RequestChecksumCalculation: aws.RequestChecksumCalculationWhenRequired,
		ResponseChecksumValidation: aws.ResponseChecksumValidationWhenRequired,
	}, func(o *s3.Options) {
		if cfg.Endpoint != "" {
			o.BaseEndpoint = aws.String(cfg.Endpoint)
		}
	})
	p := cfg.Prefix
	if p != "" {
		p += "/"
	}
	return &S3{c: client, cfg: cfg, prefix: p}
}

var _ Objects = (*S3)(nil)

// Name es la URL sin credenciales: es lo que se guarda y lo que se enseña.
func (b *S3) Name() string { return "s3://" + b.cfg.Bucket + "/" + b.cfg.Prefix }

func (b *S3) key(k string) string { return b.prefix + k }

// notFound reconoce un 404 venga modelado o pelado: HEAD no trae cuerpo, así
// que algunos S3 compatibles no llegan a modelar el error.
func notFound(err error) bool {
	var nf *types.NotFound
	var nk *types.NoSuchKey
	var re *awshttp.ResponseError
	return errors.As(err, &nf) || errors.As(err, &nk) ||
		(errors.As(err, &re) && re.HTTPStatusCode() == http.StatusNotFound)
}

func (b *S3) Stat(ctx context.Context, key string) (int64, bool, error) {
	k := b.key(key)
	out, err := b.c.HeadObject(ctx, &s3.HeadObjectInput{Bucket: &b.cfg.Bucket, Key: &k})
	if err != nil {
		if notFound(err) {
			return 0, false, nil
		}
		return 0, false, err
	}
	return aws.ToInt64(out.ContentLength), true, nil
}

func (b *S3) Get(ctx context.Context, key string) ([]byte, bool, error) {
	k := b.key(key)
	out, err := b.c.GetObject(ctx, &s3.GetObjectInput{Bucket: &b.cfg.Bucket, Key: &k})
	if err != nil {
		if notFound(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	defer func() { _ = out.Body.Close() }()
	// Acotado aunque el objeto diga medir poco: el tamaño lo declara el otro
	// lado, y creérselo es la memoria del proceso.
	if n := aws.ToInt64(out.ContentLength); n > api.MaxBlobBytes {
		return nil, false, fmt.Errorf("%s pasa del tope de %d bytes", key, int64(api.MaxBlobBytes))
	}
	data, err := blobs.ReadCapped(out.Body, api.MaxBlobBytes)
	if err != nil {
		return nil, false, err
	}
	return data, true, nil
}

func (b *S3) Put(ctx context.Context, key string, data []byte) error {
	k := b.key(key)
	_, err := b.c.PutObject(ctx, &s3.PutObjectInput{Bucket: &b.cfg.Bucket, Key: &k,
		Body: bytes.NewReader(data), ContentLength: aws.Int64(int64(len(data)))})
	return err
}

// List pagina hasta el final. Un bucket devuelve como mucho 1000 claves por
// respuesta, y quedarse en la primera página daría una historia incompleta
// —que es exactamente lo que la verificación de la cadena llama robo—.
func (b *S3) List(ctx context.Context, prefix string) ([]string, error) {
	p := b.key(prefix)
	if !strings.HasSuffix(p, "/") {
		p += "/"
	}
	var out []string
	pages := s3.NewListObjectsV2Paginator(b.c, &s3.ListObjectsV2Input{Bucket: &b.cfg.Bucket, Prefix: &p})
	for pages.HasMorePages() {
		page, err := pages.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, obj := range page.Contents {
			k := aws.ToString(obj.Key)
			if strings.HasPrefix(k, b.prefix) {
				out = append(out, strings.TrimPrefix(k, b.prefix))
			}
		}
	}
	return out, nil
}
