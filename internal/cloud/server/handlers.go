package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/JoseAFlores777/ccp/internal/cloud/api"
	"github.com/JoseAFlores777/ccp/internal/cloud/blobs"
	"github.com/JoseAFlores777/ccp/internal/cloud/store"
)

func (s *srv) info(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, api.Info{APIVersion: api.Version, Issuer: s.cfg.Issuer,
		ClientID: s.cfg.ClientID, PortalClientID: s.cfg.PortalClientID})
}

// readyz no dice qué falló: la respuesta es pública y el detalle va al log.
func (s *srv) readyz(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Ready == nil {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
		return
	}
	s.readyMu.Lock()
	if s.readyAt.IsZero() || s.cfg.Now().Sub(s.readyAt) >= 5*time.Second {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		s.readyErr, s.readyAt = s.cfg.Ready(ctx), s.cfg.Now()
		cancel()
	}
	err := s.readyErr
	s.readyMu.Unlock()
	if err != nil {
		s.cfg.Log.Warn("no preparado", "err", err)
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *srv) me(w http.ResponseWriter, r *http.Request, rc reqCtx) {
	_, err := s.cfg.Store.Vault(r.Context(), rc.user.ID)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		s.internal(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, api.Me{UserID: rc.user.ID, Email: rc.user.Email, HasVault: err == nil})
}

func toAPIDevice(d store.Device) api.Device {
	return api.Device{ID: d.ID, Name: d.Name, Platform: d.Platform, CCPVersion: d.CCPVersion,
		Created: d.Created, LastSeen: d.LastSeen, Revoked: d.Revoked}
}

func (s *srv) createDevice(w http.ResponseWriter, r *http.Request, rc reqCtx) {
	var in api.DeviceIn
	if !decode(w, r, 64<<10, &in) {
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || len(in.Name) > 100 || len(in.Platform) > 100 || len(in.CCPVersion) > 50 {
		writeError(w, http.StatusBadRequest, api.CodeBadRequest, "nombre, plataforma o versión inválidos")
		return
	}
	// El alta es el único camino que no exige cabecera de dispositivo, así que
	// era también la puerta de atrás: con el token robado bastaba pedir un id
	// nuevo para deshacer la revocación. Una sesión con algún equipo revocado
	// no da de alta más: para volver hay que iniciar sesión otra vez, que es
	// justo lo que el ladrón no puede hacer.
	revoked, err := s.sessionRevoked(r.Context(), rc)
	if err != nil {
		s.internal(w, r, err)
		return
	}
	if revoked {
		writeError(w, http.StatusForbidden, api.CodeForbidden, "esta sesión está revocada; vuelve a iniciar sesión con ccp cloud login")
		return
	}
	d, err := s.cfg.Store.CreateDevice(r.Context(), rc.user.ID, store.Device{Name: in.Name, Platform: in.Platform, CCPVersion: in.CCPVersion, SessionID: rc.session})
	if err != nil {
		s.internal(w, r, err)
		return
	}
	rc.device = d
	s.audit(r, rc, "device.create", map[string]any{"name": d.Name})
	writeJSON(w, http.StatusCreated, toAPIDevice(d))
}

// sessionRevoked dice si la sesión del token ya tuvo un equipo revocado. Un
// token sin `sid` (un emisor que no lo mande) no ata a nadie: no hay con qué.
func (s *srv) sessionRevoked(ctx context.Context, rc reqCtx) (bool, error) {
	if rc.session == "" {
		return false, nil
	}
	ds, err := s.cfg.Store.Devices(ctx, rc.user.ID)
	if err != nil {
		return false, err
	}
	for _, d := range ds {
		if d.Revoked && d.SessionID == rc.session {
			return true, nil
		}
	}
	return false, nil
}

func (s *srv) listDevices(w http.ResponseWriter, r *http.Request, rc reqCtx) {
	ds, err := s.cfg.Store.Devices(r.Context(), rc.user.ID)
	if err != nil {
		s.internal(w, r, err)
		return
	}
	out := make([]api.Device, 0, len(ds))
	for _, d := range ds {
		out = append(out, toAPIDevice(d))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *srv) revokeDevice(w http.ResponseWriter, r *http.Request, rc reqCtx) {
	id := r.PathValue("id")
	err := s.cfg.Store.RevokeDevice(r.Context(), rc.user.ID, id, s.cfg.Now())
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, api.CodeNotFound, "dispositivo no encontrado")
		return
	}
	if err != nil {
		s.internal(w, r, err)
		return
	}
	// Revocar no es quemar un id, es cortar una credencial: los equipos dados
	// de alta desde la misma sesión de Keycloak comparten el token de refresco,
	// así que dejar uno vivo sería dejarlos todos. Si la cascada falla se
	// responde error aunque el principal ya haya caído: reintentar es
	// idempotente, y decir "expulsado" con un hermano en pie es mentir.
	if err := s.revokeSessionSiblings(r.Context(), rc.user.ID, id); err != nil {
		s.internal(w, r, err)
		return
	}
	s.audit(r, rc, "device.revoke", map[string]any{"device": id})
	w.WriteHeader(http.StatusNoContent)
}

// revokeSessionSiblings revoca los demás equipos de la sesión que dio de alta
// a id. Un equipo sin sesión (alta anterior a este atado) no arrastra a nadie.
func (s *srv) revokeSessionSiblings(ctx context.Context, userID, id string) error {
	ds, err := s.cfg.Store.Devices(ctx, userID)
	if err != nil {
		return err
	}
	var sid string
	for _, d := range ds {
		if d.ID == id {
			sid = d.SessionID
		}
	}
	if sid == "" {
		return nil
	}
	for _, d := range ds {
		if d.ID == id || d.Revoked || d.SessionID != sid {
			continue
		}
		if err := s.cfg.Store.RevokeDevice(ctx, userID, d.ID, s.cfg.Now()); err != nil && !errors.Is(err, store.ErrNotFound) {
			return err
		}
	}
	return nil
}

func (s *srv) getVault(w http.ResponseWriter, r *http.Request, rc reqCtx) {
	v, err := s.cfg.Store.Vault(r.Context(), rc.user.ID)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, api.CodeNotFound, "esta cuenta aún no tiene bóveda")
		return
	}
	if err != nil {
		s.internal(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, api.Vault{KDF: json.RawMessage(v.KDF), PassphraseWrap: v.PassphraseWrap, RecoveryWrap: v.RecoveryWrap, SignPub: v.SignPub})
}

func (s *srv) putVault(w http.ResponseWriter, r *http.Request, rc reqCtx) {
	var in api.Vault
	if !decode(w, r, 64<<10, &in) {
		return
	}
	okWrap := func(b []byte) bool { return len(b) > 0 && len(b) <= 4096 }
	if len(in.KDF) == 0 || in.KDF[0] != '{' || !json.Valid(in.KDF) || !okWrap(in.PassphraseWrap) || !okWrap(in.RecoveryWrap) || len(in.SignPub) != 32 {
		writeError(w, http.StatusBadRequest, api.CodeBadRequest, "bóveda inválida")
		return
	}
	err := s.cfg.Store.CreateVault(r.Context(), rc.user.ID, store.Vault{KDF: in.KDF, PassphraseWrap: in.PassphraseWrap, RecoveryWrap: in.RecoveryWrap, SignPub: in.SignPub})
	if errors.Is(err, store.ErrConflict) {
		writeError(w, http.StatusConflict, api.CodeConflict, "esta cuenta ya tiene bóveda; desbloquéala en lugar de crear otra")
		return
	}
	if err != nil {
		s.internal(w, r, err)
		return
	}
	s.audit(r, rc, "vault.create", nil)
	w.WriteHeader(http.StatusCreated)
}

// uniqueIDs valida y deduplica una lista de ids.
func uniqueIDs(ids []string) ([]string, bool) {
	seen := make(map[string]bool, len(ids))
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if !api.ValidID(id) {
			return nil, false
		}
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out, true
}

func (s *srv) presign(w http.ResponseWriter, r *http.Request, rc reqCtx) {
	var in api.PresignReq
	if !decode(w, r, 1<<20, &in) {
		return
	}
	ids, ok := uniqueIDs(in.IDs)
	if !ok || len(ids) == 0 || len(in.IDs) > api.MaxPresignIDs || (in.Op != "put" && in.Op != "get") {
		writeError(w, http.StatusBadRequest, api.CodeBadRequest, "op debe ser put o get, con entre 1 y 500 ids válidos")
		return
	}
	known, err := s.cfg.Store.KnownBlobs(r.Context(), rc.user.ID, ids)
	if err != nil {
		s.internal(w, r, err)
		return
	}
	out := make([]api.PresignItem, 0, len(ids))
	for _, id := range ids {
		_, exists := known[id]
		it := api.PresignItem{ID: id, Exists: exists}
		key := blobs.Key(rc.user.ID, id)
		switch {
		case in.Op == "put" && !exists:
			it.URL, err = s.cfg.Blobs.PresignPut(r.Context(), key, presignTTL)
		case in.Op == "get" && exists:
			it.URL, err = s.cfg.Blobs.PresignGet(r.Context(), key, presignTTL)
		}
		if err != nil {
			s.internal(w, r, err)
			return
		}
		out = append(out, it)
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *srv) commitSnapshot(w http.ResponseWriter, r *http.Request, rc reqCtx) {
	var in api.SnapshotIn
	// base64 infla ×4/3; el doble del manifiesto máximo cubre eso y los ids.
	if !decode(w, r, 2*api.MaxManifestBytes, &in) {
		return
	}
	if len(in.Manifest) > api.MaxManifestBytes {
		writeError(w, http.StatusRequestEntityTooLarge, api.CodeTooLarge, "manifiesto demasiado grande")
		return
	}
	ids, ok := uniqueIDs(in.Blobs)
	if !ok || !api.ValidID(in.ID) || (in.Parent != "" && !api.ValidID(in.Parent)) ||
		len(in.Manifest) == 0 || len(in.Sig) != 64 || in.Created.IsZero() || len(ids) > api.MaxCommitIDs {
		writeError(w, http.StatusBadRequest, api.CodeBadRequest, "snapshot inválido")
		return
	}
	ctx := r.Context()
	known, err := s.cfg.Store.KnownBlobs(ctx, rc.user.ID, ids)
	if err != nil {
		s.internal(w, r, err)
		return
	}
	var unknown []string
	for _, id := range ids {
		if _, ok := known[id]; !ok {
			unknown = append(unknown, id)
		}
	}
	sizes, missing, err := s.headAll(ctx, rc.user.ID, unknown)
	if err != nil {
		s.cfg.Log.Error("almacenamiento", "err", err, "req", reqID(r))
		writeError(w, http.StatusServiceUnavailable, api.CodeInternal, "el almacenamiento no responde; reintenta")
		return
	}
	if len(missing) > 0 {
		writeJSON(w, http.StatusConflict, api.Error{Code: api.CodeMissingBlobs, Message: "faltan blobs por subir", Missing: missing})
		return
	}
	total := int64(len(in.Manifest))
	newBlobs := make([]store.Blob, 0, len(sizes))
	for id, size := range sizes {
		if size > api.MaxBlobBytes {
			writeError(w, http.StatusRequestEntityTooLarge, api.CodeTooLarge, "un blob supera el máximo")
			return
		}
		newBlobs = append(newBlobs, store.Blob{ID: id, Size: size})
		total += size
	}
	for _, size := range known {
		total += size
	}
	snap := store.Snapshot{ID: in.ID, Parent: in.Parent, DeviceID: rc.device.ID, Created: in.Created.UTC(),
		Manifest: in.Manifest, Sig: in.Sig, Size: total}
	created, err := s.cfg.Store.CommitSnapshot(ctx, rc.user.ID, snap, newBlobs, ids)
	if err != nil {
		s.internal(w, r, err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
		s.audit(r, rc, "snapshot.commit", map[string]any{"id": in.ID, "blobs": len(ids), "size": total})
	}
	writeJSON(w, status, api.SnapshotMeta{ID: in.ID, Parent: in.Parent, DeviceID: rc.device.ID, DeviceName: rc.device.Name,
		Created: snap.Created, Size: total})
}

// headAll comprueba en el almacenamiento, con 8 obreros fijos, que existen los
// blobs. Obreros y no una goroutine por id: el número de ids lo elige quien
// manda la petición, y con el semáforo dentro del `go` el bucle nunca se
// bloqueaba, así que un commit grande dejaba decenas de miles de goroutines
// (y sus pilas) vivas a la vez por cada petición en curso.
func (s *srv) headAll(ctx context.Context, userID string, ids []string) (map[string]int64, []string, error) {
	type result struct {
		id     string
		size   int64
		exists bool
		err    error
	}
	const obreros = 8
	n := min(obreros, len(ids))
	trabajo := make(chan string)
	results := make(chan result, n)
	var wg sync.WaitGroup
	for range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for id := range trabajo {
				size, ok, err := s.cfg.Blobs.Head(ctx, blobs.Key(userID, id))
				results <- result{id, size, ok, err}
			}
		}()
	}
	go func() {
		defer close(trabajo)
		for _, id := range ids {
			select {
			case trabajo <- id:
			case <-ctx.Done():
				return
			}
		}
	}()
	go func() {
		wg.Wait()
		close(results)
	}()
	sizes := map[string]int64{}
	var missing []string
	var firstErr error
	// Se vacía el canal entero aunque haya error: cortar aquí dejaría a los
	// obreros bloqueados escribiendo en un canal que nadie lee.
	for res := range results {
		switch {
		case res.err != nil:
			if firstErr == nil {
				firstErr = res.err
			}
		case !res.exists:
			missing = append(missing, res.id)
		default:
			sizes[res.id] = res.size
		}
	}
	if firstErr != nil {
		return nil, nil, firstErr
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	sort.Strings(missing)
	return sizes, missing, nil
}

func toAPIMeta(sn store.Snapshot) api.SnapshotMeta {
	return api.SnapshotMeta{ID: sn.ID, Parent: sn.Parent, DeviceID: sn.DeviceID, DeviceName: sn.DeviceName,
		Created: sn.Created, Size: sn.Size, Pinned: sn.Pinned}
}

func (s *srv) listSnapshots(w http.ResponseWriter, r *http.Request, rc reqCtx) {
	dev := r.URL.Query().Get("device")
	if dev != "" && !store.IsUUID(dev) {
		writeError(w, http.StatusBadRequest, api.CodeBadRequest, "device inválido")
		return
	}
	limit := 100
	if l := r.URL.Query().Get("limit"); l != "" {
		n, err := strconv.Atoi(l)
		if err != nil || n < 1 || n > 1000 {
			writeError(w, http.StatusBadRequest, api.CodeBadRequest, "limit debe estar entre 1 y 1000")
			return
		}
		limit = n
	}
	list, err := s.cfg.Store.Snapshots(r.Context(), rc.user.ID, dev, limit)
	if err != nil {
		s.internal(w, r, err)
		return
	}
	out := make([]api.SnapshotMeta, 0, len(list))
	for _, sn := range list {
		out = append(out, toAPIMeta(sn))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *srv) getSnapshot(w http.ResponseWriter, r *http.Request, rc reqCtx) {
	id := r.PathValue("id")
	if !api.ValidID(id) {
		writeError(w, http.StatusNotFound, api.CodeNotFound, "snapshot no encontrado")
		return
	}
	sn, err := s.cfg.Store.Snapshot(r.Context(), rc.user.ID, id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, api.CodeNotFound, "snapshot no encontrado")
		return
	}
	if err != nil {
		s.internal(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, api.Snapshot{SnapshotMeta: toAPIMeta(sn), Manifest: sn.Manifest, Sig: sn.Sig})
}
