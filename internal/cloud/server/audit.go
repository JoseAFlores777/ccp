package server

// GET /v1/audit — el registro de auditoría de §10.5. Solo lectura: escribirlo
// es de cada operación (srv.audit), y nada lo borra ni lo edita. Lo que sale
// de aquí cuenta quién hizo qué y cuándo; lo que hizo sigue sellado.

import (
	"net/http"
	"strconv"
	"time"

	"github.com/JoseAFlores777/ccp/internal/cloud/api"
	"github.com/JoseAFlores777/ccp/internal/cloud/store"
)

func (s *srv) listAudit(w http.ResponseWriter, r *http.Request, rc reqCtx) {
	q := r.URL.Query()
	f := store.AuditFilter{DeviceID: q.Get("device"), Action: q.Get("action")}
	if f.DeviceID != "" && !store.IsUUID(f.DeviceID) {
		writeError(w, http.StatusBadRequest, api.CodeBadRequest, "device inválido")
		return
	}
	if l := q.Get("limit"); l != "" {
		n, err := strconv.Atoi(l)
		if err != nil || n < 1 || n > api.MaxAuditEntries {
			writeError(w, http.StatusBadRequest, api.CodeBadRequest,
				"limit debe estar entre 1 y "+strconv.Itoa(api.MaxAuditEntries))
			return
		}
		f.Limit = n
	}
	// `since` se rechaza si no se entiende, en vez de tomarse por «desde
	// siempre»: una fecha mal escrita devolvería el registro entero cuando lo
	// que se pedía era un trozo, y nadie se daría cuenta.
	if v := q.Get("since"); v != "" {
		t, err := time.Parse(time.RFC3339Nano, v)
		if err != nil {
			writeError(w, http.StatusBadRequest, api.CodeBadRequest, "since debe ser una fecha RFC3339")
			return
		}
		f.Since = t
	}
	list, err := s.cfg.Store.AuditLog(r.Context(), rc.user.ID, f)
	if err != nil {
		s.internal(w, r, err)
		return
	}
	out := make([]api.AuditEntry, 0, len(list))
	for _, e := range list {
		d := e.Detail
		if d == nil {
			d = map[string]any{}
		}
		out = append(out, api.AuditEntry{ID: e.ID, At: e.At, Device: e.DeviceID,
			DeviceName: e.DeviceName, Action: e.Action, Detail: d})
	}
	writeJSON(w, http.StatusOK, out)
}
