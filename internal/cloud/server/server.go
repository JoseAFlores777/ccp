// Package server es el API HTTP de la nube de ccp (spec §10). Guarda y sirve
// datos opacos: autentica con Keycloak, autoriza por usuario y dispositivo, y
// nunca recibe nada que le permita abrir lo que guarda.
package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"github.com/JoseAFlores777/ccp/internal/cloud/api"
	"github.com/JoseAFlores777/ccp/internal/cloud/blobs"
	"github.com/JoseAFlores777/ccp/internal/cloud/store"
)

// Config son las dependencias del servidor.
type Config struct {
	Store    store.Store
	Blobs    blobs.Blobs
	Verifier Verifier
	Issuer   string // lo que /v1/info anuncia
	ClientID string // el cliente público de la CLI
	Log      *slog.Logger
	Now      func() time.Time
	// Ready comprueba las dependencias para /readyz; nil = siempre listo.
	Ready func(ctx context.Context) error
	// PerUserRate y PerUserBurst limitan las peticiones por usuario (0 = por defecto).
	PerUserRate  rate.Limit
	PerUserBurst int
}

const presignTTL = 15 * time.Minute

type srv struct {
	cfg      Config
	mu       sync.Mutex
	limiters map[string]*rate.Limiter
	readyMu  sync.Mutex
	readyAt  time.Time
	readyErr error
}

// New construye el handler del API.
func New(c Config) http.Handler {
	if c.Log == nil {
		c.Log = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	if c.Now == nil {
		c.Now = time.Now
	}
	if c.PerUserRate == 0 {
		c.PerUserRate = 20
	}
	if c.PerUserBurst == 0 {
		c.PerUserBurst = 200
	}
	s := &srv{cfg: c, limiters: map[string]*rate.Limiter{}}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	mux.HandleFunc("GET /readyz", s.readyz)
	mux.HandleFunc("GET /v1/info", s.info)
	mux.Handle("GET /v1/me", s.authed(s.me, false))
	mux.Handle("POST /v1/devices", s.authed(s.createDevice, false))
	mux.Handle("GET /v1/devices", s.authed(s.listDevices, true))
	mux.Handle("DELETE /v1/devices/{id}", s.authed(s.revokeDevice, true))
	mux.Handle("GET /v1/vault", s.authed(s.getVault, true))
	mux.Handle("PUT /v1/vault", s.authed(s.putVault, true))
	mux.Handle("POST /v1/blobs/presign", s.authed(s.presign, true))
	mux.Handle("POST /v1/snapshots", s.authed(s.commitSnapshot, true))
	mux.Handle("GET /v1/snapshots", s.authed(s.listSnapshots, true))
	mux.Handle("GET /v1/snapshots/{id}", s.authed(s.getSnapshot, true))
	return s.logged(mux)
}

type reqCtx struct {
	user   store.User
	device store.Device
	// session es el `sid` del token: la sesión de Keycloak desde la que se
	// habla. Ata los equipos que se den de alta con este token a la credencial
	// que los pidió, que es lo que hace que revocar corte de verdad.
	session string
}

type handler func(w http.ResponseWriter, r *http.Request, rc reqCtx)

// authed exige un token válido y, si needDevice, un dispositivo del usuario no
// revocado. El usuario se crea en su primera petición.
func (s *srv) authed(h handler, needDevice bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok || raw == "" {
			writeError(w, http.StatusUnauthorized, api.CodeUnauthorized, "falta el token")
			return
		}
		id, err := s.cfg.Verifier.Verify(r.Context(), raw)
		if err != nil {
			s.cfg.Log.Info("token rechazado", "err", err, "req", reqID(r))
			writeError(w, http.StatusUnauthorized, api.CodeUnauthorized, "token inválido o caducado")
			return
		}
		if !s.allow(id.Sub) {
			writeError(w, http.StatusTooManyRequests, api.CodeRateLimited, "demasiadas peticiones; espera un momento")
			return
		}
		user, err := s.cfg.Store.UpsertUser(r.Context(), id.Sub, id.Email)
		if err != nil {
			s.internal(w, r, err)
			return
		}
		rc := reqCtx{user: user, session: id.SessionID}
		if needDevice {
			dev := r.Header.Get(api.HeaderDevice)
			if !store.IsUUID(dev) {
				writeError(w, http.StatusBadRequest, api.CodeBadRequest, "falta la cabecera "+api.HeaderDevice)
				return
			}
			d, err := s.cfg.Store.SeenDevice(r.Context(), user.ID, dev, s.cfg.Now())
			switch {
			case errors.Is(err, store.ErrNotFound):
				writeError(w, http.StatusForbidden, api.CodeForbidden, "dispositivo desconocido")
				return
			case err != nil:
				s.internal(w, r, err)
				return
			case d.Revoked:
				writeError(w, http.StatusForbidden, api.CodeForbidden, "dispositivo revocado")
				return
			}
			rc.device = d
		}
		h(w, r, rc)
	})
}

func (s *srv) allow(sub string) bool {
	s.mu.Lock()
	l, ok := s.limiters[sub]
	if !ok {
		l = rate.NewLimiter(s.cfg.PerUserRate, s.cfg.PerUserBurst)
		s.limiters[sub] = l
	}
	s.mu.Unlock()
	return l.Allow()
}

type ctxKey struct{}

func reqID(r *http.Request) string {
	id, _ := r.Context().Value(ctxKey{}).(string)
	return id
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

// logged da un id a cada petición, registra método, ruta (sin query: las URLs
// no llevan secretos, pero así no hay que pensarlo), estado y duración, y
// convierte un pánico en un 500.
func (s *srv) logged(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b := make([]byte, 8)
		_, _ = rand.Read(b)
		id := hex.EncodeToString(b)
		w.Header().Set("X-Request-Id", id)
		r = r.WithContext(context.WithValue(r.Context(), ctxKey{}, id))
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		start := time.Now()
		defer func() {
			if p := recover(); p != nil {
				s.cfg.Log.Error("pánico", "panic", p, "req", id)
				writeError(sw, http.StatusInternalServerError, api.CodeInternal, "error interno")
			}
			s.cfg.Log.Info("petición", "req", id, "method", r.Method, "path", r.URL.Path,
				"status", sw.status, "ms", time.Since(start).Milliseconds())
		}()
		next.ServeHTTP(sw, r)
	})
}

func (s *srv) internal(w http.ResponseWriter, r *http.Request, err error) {
	s.cfg.Log.Error("error interno", "err", err, "req", reqID(r))
	writeError(w, http.StatusInternalServerError, api.CodeInternal, "error interno (req "+reqID(r)+")")
}

func (s *srv) audit(r *http.Request, rc reqCtx, action string, detail map[string]any) {
	if err := s.cfg.Store.Audit(r.Context(), rc.user.ID, rc.device.ID, action, detail); err != nil {
		s.cfg.Log.Error("auditoría", "err", err, "req", reqID(r), "action", action)
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, api.Error{Code: code, Message: msg})
}

// decode lee un cuerpo JSON de como mucho max bytes. Ignora campos desconocidos
// a propósito: un cliente más nuevo puede mandar campos que este servidor aún
// no conoce.
func decode(w http.ResponseWriter, r *http.Request, max int64, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, max)
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			writeError(w, http.StatusRequestEntityTooLarge, api.CodeTooLarge, "cuerpo demasiado grande")
			return false
		}
		writeError(w, http.StatusBadRequest, api.CodeBadRequest, "JSON inválido")
		return false
	}
	return true
}
