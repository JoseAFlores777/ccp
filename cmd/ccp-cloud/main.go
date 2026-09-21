// Command ccp-cloud es el backend de la nube de ccp (spec §10). Se configura por
// variables de entorno (deploy/ccp-cloud/docker-compose.yml) y guarda solo datos
// opacos: nunca recibe nada que le permita abrirlos.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/JoseAFlores777/ccp/internal/cloud/blobs"
	"github.com/JoseAFlores777/ccp/internal/cloud/portal"
	"github.com/JoseAFlores777/ccp/internal/cloud/server"
	"github.com/JoseAFlores777/ccp/internal/cloud/store"
)

// version la fija el build (-ldflags "-X main.version=…").
var version = "dev"

func main() {
	// La imagen es distroless: no hay shell ni curl, así que el healthcheck del
	// contenedor es el propio binario preguntándose a sí mismo.
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		os.Exit(healthcheck())
	}
	log := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	if err := run(log); err != nil {
		log.Error("ccp-cloud terminó con error", "err", err)
		os.Exit(1)
	}
}

type config struct {
	addr, issuer, jwks, audience, clientID        string
	dbHost, dbUser, dbPassword, dbName, dbSSLMode string
	portalClientID                                string
	portal                                        bool
	dbPort                                        int
	s3                                            blobs.S3Config
	retention                                     server.Retention
}

// dsn es la DSN de palabras clave, SIN contraseña (va aparte a OpenPG): una
// contraseña con símbolos rompería una URL postgres://, y además así no acaba
// en ningún mensaje de error que imprima la DSN.
func (c config) dsn() string {
	return fmt.Sprintf("host=%s port=%d user=%s dbname=%s sslmode=%s", c.dbHost, c.dbPort, c.dbUser, c.dbName, c.dbSSLMode)
}

// loadConfig junta TODAS las variables que faltan antes de rendirse: arrancar el
// stack para que muera en la siguiente variable es un viaje de ida y vuelta por
// cada una.
func loadConfig() (config, error) {
	var c config
	var missing []string
	get := func(k, def string) string {
		if v := os.Getenv(k); v != "" {
			return v
		}
		if def == "" {
			missing = append(missing, k)
		}
		return def
	}
	c.addr = get("CCP_CLOUD_ADDR", ":8080")
	c.issuer = strings.TrimSuffix(get("CCP_CLOUD_OIDC_ISSUER", ""), "/")
	c.jwks = get("CCP_CLOUD_OIDC_JWKS", c.issuer+"/protocol/openid-connect/certs")
	c.audience = get("CCP_CLOUD_OIDC_AUDIENCE", "ccp-api")
	c.clientID = get("CCP_CLOUD_OIDC_CLIENT_ID", "ccp-cli")
	c.portalClientID = get("CCP_CLOUD_OIDC_PORTAL_CLIENT_ID", "ccp-portal")
	c.portal = get("CCP_CLOUD_PORTAL", "1") != "0"
	c.dbHost = get("CCP_CLOUD_DB_HOST", "postgres")
	port, err := strconv.Atoi(get("CCP_CLOUD_DB_PORT", "5432"))
	if err != nil {
		return c, errors.New("CCP_CLOUD_DB_PORT no es un número")
	}
	c.dbPort = port
	c.dbUser = get("CCP_CLOUD_DB_USER", "ccp")
	c.dbPassword = get("CCP_CLOUD_DB_PASSWORD", "")
	c.dbName = get("CCP_CLOUD_DB_NAME", "ccp")
	c.dbSSLMode = get("CCP_CLOUD_DB_SSLMODE", "disable")
	c.s3 = blobs.S3Config{
		Endpoint: get("CCP_CLOUD_S3_ENDPOINT", ""), PublicEndpoint: get("CCP_CLOUD_S3_PUBLIC_ENDPOINT", ""),
		Region: get("CCP_CLOUD_S3_REGION", "us-east-1"), Bucket: get("CCP_CLOUD_S3_BUCKET", "ccp-blobs"),
		AccessKey: get("CCP_CLOUD_S3_ACCESS_KEY", ""), SecretKey: get("CCP_CLOUD_S3_SECRET_KEY", ""),
	}
	// Retención: sin ninguna de las tres, el servidor guarda TODOS los
	// snapshots, que es lo que dice §10.3.1. Un número mal escrito para el
	// arranque en vez de quedarse en cero: un cero silencioso aquí no poda de
	// más, pero esconde que el despliegue creía estar podando.
	var malas []string
	num := func(k string) int {
		v := os.Getenv(k)
		if v == "" {
			return 0
		}
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			malas = append(malas, k)
			return 0
		}
		return n
	}
	c.retention = server.Retention{
		Daily: num("CCP_CLOUD_RETENTION_DAILY"), Weekly: num("CCP_CLOUD_RETENTION_WEEKLY"),
		Monthly: num("CCP_CLOUD_RETENTION_MONTHLY"),
		// Un blob recién subido no se poda aunque no lo nombre nadie: puede ser
		// de un push a medias, que sube los blobs antes que su snapshot.
		Grace: time.Hour,
		// Y la historia de una cuenta no se recorre en cada publicación.
		Every: 24 * time.Hour,
	}
	if len(malas) > 0 {
		return c, fmt.Errorf("no son un número de periodos: %s", strings.Join(malas, ", "))
	}
	if len(missing) > 0 {
		return c, fmt.Errorf("faltan variables de entorno: %s", strings.Join(missing, ", "))
	}
	return c, nil
}

func run(log *slog.Logger) error {
	c, err := loadConfig()
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pg, err := openPG(ctx, log, c)
	if err != nil {
		return err
	}
	defer pg.Close()
	if err := pg.Migrate(ctx); err != nil {
		return fmt.Errorf("migraciones: %w", err)
	}
	bl := blobs.NewS3(c.s3)
	web, err := buildPortal(c)
	if err != nil {
		return err
	}
	h := server.New(server.Config{
		Store: pg, Blobs: bl, Verifier: server.NewOIDCVerifier(c.issuer, c.jwks, c.audience),
		Issuer: c.issuer, ClientID: c.clientID, PortalClientID: c.portalClientID,
		Portal: web, Log: log, Ready: readyFunc(pg, bl, c.jwks), Retention: c.retention,
	})
	// Los timeouts de lectura/escritura son largos a propósito: por aquí pasan
	// subidas y bajadas de blobs, no solo JSON de unos pocos kilobytes.
	hs := &http.Server{Addr: c.addr, Handler: h, ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout: 5 * time.Minute, WriteTimeout: 5 * time.Minute, IdleTimeout: 2 * time.Minute}
	errc := make(chan error, 1)
	go func() {
		log.Info("ccp-cloud escuchando", "addr", c.addr, "version", version)
		errc <- hs.ListenAndServe()
	}()
	select {
	case err := <-errc:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
	}
	// Contexto propio: el de la señal ya está cancelado y Shutdown necesita
	// margen para que las peticiones en vuelo terminen.
	sctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	log.Info("apagando")
	return hs.Shutdown(sctx)
}

// buildPortal construye el portal web, o nil si el despliegue lo apaga con
// CCP_CLOUD_PORTAL=0. Un emisor que el portal no acepta es un error de
// arranque y no un portal a medias: una pestaña que no puede hablar con
// Keycloak solo falla al pulsar «entrar».
func buildPortal(c config) (http.Handler, error) {
	if !c.portal {
		return nil, nil
	}
	return portal.New(portal.Config{Issuer: c.issuer})
}

// readyFunc es lo que contesta /readyz: las tres dependencias, cada una con su
// nombre, porque «not ready» a secas no dice a quién hay que mirar.
func readyFunc(pg *store.PG, bl *blobs.S3, jwksURL string) func(context.Context) error {
	jwksHTTP := &http.Client{Timeout: 3 * time.Second}
	return func(ctx context.Context) error {
		if err := pg.Ping(ctx); err != nil {
			return fmt.Errorf("postgres: %w", err)
		}
		if err := bl.Ping(ctx); err != nil {
			return fmt.Errorf("almacenamiento: %w", err)
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, jwksURL, nil)
		if err != nil {
			return fmt.Errorf("keycloak: %w", err)
		}
		resp, err := jwksHTTP.Do(req)
		if err != nil {
			return fmt.Errorf("keycloak: %w", err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("keycloak: el JWKS respondió %d", resp.StatusCode)
		}
		return nil
	}
}

// openPG espera a Postgres hasta 60 s: en un arranque en frío del stack, la base
// puede tardar más que su healthcheck en aceptar conexiones de verdad, y morir
// para que el orquestador nos reinicie convierte un arranque en una ristra de
// contenedores caídos.
func openPG(ctx context.Context, log *slog.Logger, c config) (*store.PG, error) {
	deadline := time.Now().Add(60 * time.Second)
	for {
		pg, err := store.OpenPG(ctx, c.dsn(), c.dbPassword)
		if err == nil {
			// OpenPG no conecta (el pool es perezoso): el Ping es lo que dice
			// si Postgres está de verdad ahí.
			if err = pg.Ping(ctx); err == nil {
				return pg, nil
			}
			pg.Close()
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("postgres no responde: %w", err)
		}
		log.Warn("esperando a postgres", "err", err)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
}

// healthcheck pregunta a /healthz del propio proceso. Sale 0 si responde bien.
func healthcheck() int {
	addr := os.Getenv("CCP_CLOUD_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	if strings.HasPrefix(addr, ":") {
		addr = "127.0.0.1" + addr
	}
	resp, err := (&http.Client{Timeout: 3 * time.Second}).Get("http://" + addr + "/healthz")
	if err != nil {
		return 1
	}
	resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return 1
	}
	return 0
}
