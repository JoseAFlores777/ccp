package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/JoseAFlores777/ccp/internal/cloud/api"
)

// API es el cliente de /v1.
type API struct {
	base   string
	hc     *http.Client // con token
	raw    *http.Client // para las URLs prefirmadas: sin token
	device string
}

// NewAPI construye el cliente. hc debe añadir el token (HTTPClient).
func NewAPI(server string, hc *http.Client, device string) *API {
	return &API{base: strings.TrimSuffix(server, "/"), hc: hc, raw: &http.Client{Timeout: 5 * time.Minute}, device: device}
}

// WithDevice devuelve una copia que se identifica como el dispositivo id.
func (a *API) WithDevice(id string) *API {
	c := *a
	c.device = id
	return &c
}

// APIError es una respuesta de error del servidor. Los campos de api.Error van
// aplanados y no embebidos: ese tipo tiene un campo Message pero también se
// llama Error, y embeberlo chocaría con el método Error() de esta.
type APIError struct {
	Status  int
	Code    string
	Message string
	Missing []string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("la nube respondió %d: %s", e.Status, e.Message)
}

func (a *API) do(ctx context.Context, method, path string, in, out any) error {
	_, err := a.doStatus(ctx, method, path, in, out)
	return err
}

// doStatus es do y además el código de la respuesta: hace falta para el 204 de
// «no hay revisión pendiente», que no es un error y tampoco trae cuerpo.
func (a *API) doStatus(ctx context.Context, method, path string, in, out any) (int, error) {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return 0, err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, a.base+path, body)
	if err != nil {
		return 0, err
	}
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if a.device != "" {
		req.Header.Set(api.HeaderDevice, a.device)
	}
	resp, err := a.hc.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		var body api.Error
		_ = json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&body)
		e := &APIError{Status: resp.StatusCode, Code: body.Code, Message: body.Message, Missing: body.Missing}
		if e.Message == "" {
			e.Message = resp.Status
		}
		return resp.StatusCode, e
	}
	if out == nil || resp.StatusCode == http.StatusNoContent {
		return resp.StatusCode, nil
	}
	return resp.StatusCode, json.NewDecoder(io.LimitReader(resp.Body, 64<<20)).Decode(out)
}

// Me devuelve la cuenta de este token.
func (a *API) Me(ctx context.Context) (api.Me, error) {
	var m api.Me
	return m, a.do(ctx, http.MethodGet, "/v1/me", nil, &m)
}

// RegisterDevice da de alta este equipo en la cuenta.
func (a *API) RegisterDevice(ctx context.Context, in api.DeviceIn) (api.Device, error) {
	var d api.Device
	return d, a.do(ctx, http.MethodPost, "/v1/devices", in, &d)
}

// Devices lista los equipos de la cuenta.
func (a *API) Devices(ctx context.Context) ([]api.Device, error) {
	var ds []api.Device
	return ds, a.do(ctx, http.MethodGet, "/v1/devices", nil, &ds)
}

// RevokeDevice revoca un equipo.
func (a *API) RevokeDevice(ctx context.Context, id string) error {
	return a.do(ctx, http.MethodDelete, "/v1/devices/"+url.PathEscape(id), nil, nil)
}

// Vault baja las envolturas de la bóveda (el servidor no las sabe abrir).
func (a *API) Vault(ctx context.Context) (api.Vault, error) {
	var v api.Vault
	return v, a.do(ctx, http.MethodGet, "/v1/vault", nil, &v)
}

// PutVault sube la bóveda recién creada.
func (a *API) PutVault(ctx context.Context, v api.Vault) error {
	return a.do(ctx, http.MethodPut, "/v1/vault", v, nil)
}

// RewrapVault rota las claves de ACCESO: sube envolturas nuevas sobre la
// misma clave de cuenta. El servidor rechaza la rotación si la clave pública
// de firma no es la que ya tenía, que es su única forma de ver que la AK no
// cambió sin poder abrir nada.
func (a *API) RewrapVault(ctx context.Context, v api.Vault) error {
	return a.do(ctx, http.MethodPut, "/v1/vault/wraps", v, nil)
}

// Presign pide URLs prefirmadas para subir ("put") o bajar ("get") blobs.
func (a *API) Presign(ctx context.Context, op string, ids []string) ([]api.PresignItem, error) {
	var out []api.PresignItem
	return out, a.do(ctx, http.MethodPost, "/v1/blobs/presign", api.PresignReq{Op: op, IDs: ids}, &out)
}

// CommitSnapshot publica un snapshot cuyos blobs ya están subidos.
func (a *API) CommitSnapshot(ctx context.Context, in api.SnapshotIn) (api.SnapshotMeta, error) {
	var m api.SnapshotMeta
	return m, a.do(ctx, http.MethodPost, "/v1/snapshots", in, &m)
}

// Snapshots lista los snapshots de la cuenta, del más nuevo al más viejo.
func (a *API) Snapshots(ctx context.Context, device string, limit int) ([]api.SnapshotMeta, error) {
	q := url.Values{"limit": {strconv.Itoa(limit)}}
	if device != "" {
		q.Set("device", device)
	}
	var out []api.SnapshotMeta
	return out, a.do(ctx, http.MethodGet, "/v1/snapshots?"+q.Encode(), nil, &out)
}

// Chain baja la historia entera para verificarla (VerifyChain). No lleva
// manifiestos: son los eslabones, no el contenido.
func (a *API) Chain(ctx context.Context) ([]api.ChainLink, error) {
	var out []api.ChainLink
	return out, a.do(ctx, http.MethodGet, "/v1/snapshots/chain", nil, &out)
}

// PinSnapshot fija o suelta un snapshot ya publicado.
func (a *API) PinSnapshot(ctx context.Context, id string, pinned bool) error {
	return a.do(ctx, http.MethodPost, "/v1/snapshots/"+url.PathEscape(id)+"/pin", api.PinIn{Pinned: pinned}, nil)
}

// Snapshot baja un snapshot con su manifiesto sellado y su firma.
func (a *API) Snapshot(ctx context.Context, id string) (api.Snapshot, error) {
	var s api.Snapshot
	return s, a.do(ctx, http.MethodGet, "/v1/snapshots/"+url.PathEscape(id), nil, &s)
}

// PutBlob sube body a una URL prefirmada, con reintentos ante fallos de red o 5xx.
func (a *API) PutBlob(ctx context.Context, u string, body []byte) error {
	return retry(ctx, func() (bool, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPut, u, bytes.NewReader(body))
		if err != nil {
			return false, err
		}
		req.ContentLength = int64(len(body))
		resp, err := a.raw.Do(req)
		if err != nil {
			return true, err
		}
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
		resp.Body.Close()
		if resp.StatusCode/100 == 2 {
			return false, nil
		}
		return resp.StatusCode >= 500, fmt.Errorf("el almacenamiento respondió %d al subir", resp.StatusCode)
	})
}

// GetBlob baja de una URL prefirmada, con reintentos.
func (a *API) GetBlob(ctx context.Context, u string) ([]byte, error) {
	var out []byte
	err := retry(ctx, func() (bool, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return false, err
		}
		resp, err := a.raw.Do(req)
		if err != nil {
			return true, err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return resp.StatusCode >= 500, fmt.Errorf("el almacenamiento respondió %d al bajar", resp.StatusCode)
		}
		data, err := io.ReadAll(io.LimitReader(resp.Body, api.MaxBlobBytes+1))
		if err != nil {
			return true, err
		}
		if len(data) > api.MaxBlobBytes {
			return false, fmt.Errorf("el almacenamiento devolvió un blob de más de %d bytes", api.MaxBlobBytes)
		}
		out = data
		return false, nil
	})
	return out, err
}

// retry reintenta fn hasta 4 veces, con espera exponencial desde 500 ms,
// mientras diga que el fallo es reintentable.
func retry(ctx context.Context, fn func() (retryable bool, err error)) error {
	var err error
	for attempt := 0; attempt < 4; attempt++ {
		var again bool
		if again, err = fn(); err == nil || !again {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(1<<attempt) * 500 * time.Millisecond):
		}
	}
	return err
}

// ErrNoRevision es «no hay nada que aplicar». El servidor responde 204 y no
// 404 a propósito, y aquí se distingue por su propio valor: un agente que
// pregunta cada pocos minutos no puede llenar el log de errores que no lo son.
var ErrNoRevision = errors.New("cloud: no hay ninguna revisión pendiente")

// PublishRevision publica una revisión deseada para un dispositivo. La firma
// la pone quien tenga la clave de cuenta; el servidor solo la guarda.
func (a *API) PublishRevision(ctx context.Context, in api.RevisionIn) (api.Revision, error) {
	var out api.Revision
	return out, a.do(ctx, http.MethodPost, "/v1/revisions", in, &out)
}

// PendingRevision devuelve la revisión que ESTE dispositivo tiene que aplicar,
// entera (con cuerpo y firma, que es lo que hay que verificar antes de tocar
// nada). ErrNoRevision cuando no hay ninguna.
func (a *API) PendingRevision(ctx context.Context) (api.Revision, error) {
	var out api.Revision
	code, err := a.doStatus(ctx, http.MethodGet, "/v1/revisions/pending", nil, &out)
	if err != nil {
		return api.Revision{}, err
	}
	if code == http.StatusNoContent {
		return api.Revision{}, ErrNoRevision
	}
	return out, nil
}

// Revision baja una revisión entera por su id.
func (a *API) Revision(ctx context.Context, id string) (api.Revision, error) {
	var out api.Revision
	return out, a.do(ctx, http.MethodGet, "/v1/revisions/"+url.PathEscape(id), nil, &out)
}

// Revisions lista las revisiones de la cuenta, de la más nueva a la más vieja,
// sin cuerpo ni firma. device vacío = todas.
func (a *API) Revisions(ctx context.Context, device string, limit int) ([]api.RevisionMeta, error) {
	q := url.Values{"limit": {strconv.Itoa(limit)}}
	if device != "" {
		q.Set("device", device)
	}
	var out []api.RevisionMeta
	return out, a.do(ctx, http.MethodGet, "/v1/revisions?"+q.Encode(), nil, &out)
}

// SetRevisionState informa del resultado. Solo lo acepta el servidor del
// dispositivo destinatario, y solo una vez: quien la cierra dice la última
// palabra sobre esa orden.
func (a *API) SetRevisionState(ctx context.Context, id, state, reason string) (api.RevisionMeta, error) {
	var out api.RevisionMeta
	return out, a.do(ctx, http.MethodPost, "/v1/revisions/"+url.PathEscape(id)+"/state",
		api.RevisionStateIn{State: state, Reason: reason}, &out)
}

// AuditQuery acota una lectura del registro de auditoría. El cero es «lo
// último de la cuenta».
type AuditQuery struct {
	Device string
	Action string
	Since  time.Time
	Limit  int
}

// Audit lee el registro de auditoría de la cuenta (§10.5): quién hizo qué y
// cuándo. Nunca qué configuración había dentro — el servidor no la puede leer.
func (a *API) Audit(ctx context.Context, q AuditQuery) ([]api.AuditEntry, error) {
	v := url.Values{}
	if q.Device != "" {
		v.Set("device", q.Device)
	}
	if q.Action != "" {
		v.Set("action", q.Action)
	}
	if !q.Since.IsZero() {
		v.Set("since", q.Since.UTC().Format(time.RFC3339Nano))
	}
	if q.Limit > 0 {
		v.Set("limit", strconv.Itoa(q.Limit))
	}
	path := "/v1/audit"
	if len(v) > 0 {
		path += "?" + v.Encode()
	}
	var out []api.AuditEntry
	return out, a.do(ctx, http.MethodGet, path, nil, &out)
}
