// Package blobstest da un almacenamiento de blobs en memoria para los tests,
// con un servidor HTTP que valida sus propias URLs firmadas, y el contrato que
// cumple toda implementación de blobs.Blobs. Firmar de verdad es el punto: así
// los tests del cliente suben y bajan por HTTP como en producción, en vez de
// creerse una URL que nadie comprueba.
package blobstest

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/JoseAFlores777/ccp/internal/cloud/api"
	"github.com/JoseAFlores777/ccp/internal/cloud/blobs"
)

// Mem es un blobs.Blobs en memoria servido por HTTP.
type Mem struct {
	mu     sync.Mutex
	data   map[string][]byte
	secret []byte
	base   string
}

// New arranca el almacenamiento; se para al acabar el test.
func New(t testing.TB) *Mem {
	t.Helper()
	m := &Mem{data: map[string][]byte{}, secret: make([]byte, 32)}
	if _, err := rand.Read(m.secret); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(m.serve))
	t.Cleanup(srv.Close)
	m.base = srv.URL
	return m
}

// mac firma método, clave y caducidad juntos: una URL de subida no vale para
// bajar, ni la de una clave para otra.
func (m *Mem) mac(method, key string, exp int64) string {
	h := hmac.New(sha256.New, m.secret)
	h.Write([]byte(method + "\n" + key + "\n" + strconv.FormatInt(exp, 10)))
	return hex.EncodeToString(h.Sum(nil))
}

func (m *Mem) presign(method, key string, ttl time.Duration) string {
	exp := time.Now().Add(ttl).Unix()
	q := url.Values{"exp": {strconv.FormatInt(exp, 10)}, "sig": {m.mac(method, key, exp)}}
	return m.base + "/o/" + key + "?" + q.Encode()
}

func (m *Mem) serve(w http.ResponseWriter, r *http.Request) {
	key := strings.TrimPrefix(r.URL.Path, "/o/")
	exp, _ := strconv.ParseInt(r.URL.Query().Get("exp"), 10, 64)
	if time.Now().Unix() > exp || !hmac.Equal([]byte(r.URL.Query().Get("sig")), []byte(m.mac(r.Method, key, exp))) {
		http.Error(w, "firma inválida o caducada", http.StatusForbidden)
		return
	}
	switch r.Method {
	case http.MethodPut:
		body, err := io.ReadAll(io.LimitReader(r.Body, api.MaxBlobBytes+1))
		if err != nil || len(body) > api.MaxBlobBytes {
			http.Error(w, "demasiado grande", http.StatusRequestEntityTooLarge)
			return
		}
		m.Set(key, body)
	case http.MethodGet:
		data, ok := m.Get(key)
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(data)
	default:
		http.Error(w, "método no admitido", http.StatusMethodNotAllowed)
	}
}

// Set guarda data en key saltándose las URLs (para preparar o alterar un test).
func (m *Mem) Set(key string, data []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[key] = bytes.Clone(data)
}

// Get lee key saltándose las URLs.
func (m *Mem) Get(key string) ([]byte, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	d, ok := m.data[key]
	return bytes.Clone(d), ok
}

// PresignPut da una URL con la que subir key.
func (m *Mem) PresignPut(_ context.Context, key string, ttl time.Duration) (string, error) {
	return m.presign(http.MethodPut, key, ttl), nil
}

// PresignGet da una URL con la que bajar key.
func (m *Mem) PresignGet(_ context.Context, key string, ttl time.Duration) (string, error) {
	return m.presign(http.MethodGet, key, ttl), nil
}

// Head dice si key existe y cuánto mide.
func (m *Mem) Head(_ context.Context, key string) (int64, bool, error) {
	d, ok := m.Get(key)
	return int64(len(d)), ok, nil
}

// Ping siempre va: la memoria no se cae.
func (m *Mem) Ping(context.Context) error { return nil }

// RunContract es lo que toda implementación de blobs.Blobs tiene que cumplir:
// una URL de subida sirve para subir, una de bajada devuelve lo subido y Head
// lo ve. Lo comprueba por HTTP de verdad, porque el servidor nunca toca el
// contenido: si la URL firmada no vale, nadie más se entera.
func RunContract(t *testing.T, b blobs.Blobs) {
	t.Helper()
	ctx := context.Background()
	key := blobs.Key("00000000-0000-4000-8000-000000000000", strings.Repeat("ab", 32))
	if _, ok, err := b.Head(ctx, key); err != nil || ok {
		t.Fatalf("Head antes de subir = %v, %v", ok, err)
	}
	put, err := b.PresignPut(ctx, key, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	body := []byte("contenido sellado de prueba")
	req, _ := http.NewRequestWithContext(ctx, http.MethodPut, put, bytes.NewReader(body))
	req.ContentLength = int64(len(body))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		t.Fatalf("PUT prefirmado = %d", resp.StatusCode)
	}
	if size, ok, err := b.Head(ctx, key); err != nil || !ok || size != int64(len(body)) {
		t.Fatalf("Head tras subir = %d %v %v", size, ok, err)
	}
	get, err := b.PresignGet(ctx, key, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	resp, err = http.Get(get)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || !bytes.Equal(got, body) {
		t.Fatalf("GET prefirmado = %d %q", resp.StatusCode, got)
	}
	// Una URL de subida no sirve para bajar.
	resp, err = http.Get(put)
	if err == nil {
		resp.Body.Close()
		if resp.StatusCode == 200 {
			t.Fatal("una URL de subida sirvió para bajar")
		}
	}
	if err := b.Ping(ctx); err != nil {
		t.Fatalf("Ping: %v", err)
	}
}
