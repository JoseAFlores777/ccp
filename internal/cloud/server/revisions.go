package server

// Revisiones deseadas (spec §10.3): «el portal propone, la máquina aplica».
// El servidor las guarda y las sirve, y nada más. No tiene la clave de cuenta,
// así que no puede fabricar una orden; y como el dispositivo destinatario va
// dentro de lo firmado, tampoco puede desviar la que ya existe: copiarla a
// otra máquina es fácil, que esa máquina se la crea no.

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/JoseAFlores777/ccp/internal/cloud/api"
	"github.com/JoseAFlores777/ccp/internal/cloud/store"
)

func toRevMeta(r store.Revision) api.RevisionMeta {
	return api.RevisionMeta{ID: r.ID, Prev: r.Prev, DeviceID: r.DeviceID, DeviceName: r.DeviceName,
		Snapshot: r.Snapshot, Base: r.Base, Created: r.Created, State: r.State, Reason: r.Reason,
		Updated: r.Updated, By: r.By, Group: r.Group}
}

func toRevFull(r store.Revision) api.Revision {
	return api.Revision{RevisionMeta: toRevMeta(r), Body: r.Body, Sig: r.Sig}
}

// targetDevice busca el destinatario sin tocar su «último contacto»: la
// revisión la publica otro: el equipo de destino no ha hablado con nadie.
func (s *srv) targetDevice(r *http.Request, userID, deviceID string) (store.Device, error) {
	ds, err := s.cfg.Store.Devices(r.Context(), userID)
	if err != nil {
		return store.Device{}, err
	}
	for _, d := range ds {
		if d.ID == deviceID {
			return d, nil
		}
	}
	return store.Device{}, store.ErrNotFound
}

func (s *srv) publishRevision(w http.ResponseWriter, r *http.Request, rc reqCtx) {
	var in api.RevisionIn
	// base64 infla ×4/3; el doble del cuerpo máximo cubre eso y los ids.
	if !decode(w, r, 2*api.MaxRevisionBytes, &in) {
		return
	}
	if len(in.Body) > api.MaxRevisionBytes {
		writeError(w, http.StatusRequestEntityTooLarge, api.CodeTooLarge, "el cuerpo de la revisión es demasiado grande")
		return
	}
	okID := func(s string) bool { return s == "" || api.ValidID(s) }
	if !api.ValidID(in.ID) || !okID(in.Prev) || !okID(in.Snapshot) || !okID(in.Base) ||
		!store.IsUUID(in.DeviceID) || len(in.Sig) != 64 || in.Created.IsZero() ||
		(in.Group != "" && !store.IsUUID(in.Group)) {
		writeError(w, http.StatusBadRequest, api.CodeBadRequest, "revisión inválida")
		return
	}
	// Una revisión que no nombra ni un snapshot ni un cambio no es una orden:
	// dejarla pasar pondría a una máquina a «aplicar» la nada y a informar de
	// un resultado que no significa nada.
	if in.Snapshot == "" && len(in.Body) == 0 {
		writeError(w, http.StatusBadRequest, api.CodeBadRequest, "una revisión tiene que traer un snapshot o un conjunto de cambios")
		return
	}
	target, err := s.targetDevice(r, rc.user.ID, in.DeviceID)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, api.CodeNotFound, "dispositivo no encontrado")
		return
	}
	if err != nil {
		s.internal(w, r, err)
		return
	}
	// Una etiqueta de un grupo que no existe se rechaza: la orden se pondría
	// igual, pero no saldría en el estado de ningún grupo, y quien la publicó
	// se quedaría mirando una lista vacía sin saber que escribió mal el id.
	if in.Group != "" {
		if _, err := s.cfg.Store.Group(r.Context(), rc.user.ID, in.Group); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeError(w, http.StatusNotFound, api.CodeNotFound, "grupo no encontrado")
				return
			}
			s.internal(w, r, err)
			return
		}
	}
	// A un equipo revocado no se le manda nada: no va a volver a preguntar, y
	// dejar la orden ahí la pinta en el portal como pendiente para siempre.
	if target.Revoked {
		writeError(w, http.StatusConflict, api.CodeConflict, "ese dispositivo está revocado")
		return
	}
	got, err := s.cfg.Store.PublishRevision(r.Context(), rc.user.ID, store.Revision{
		ID: in.ID, Prev: in.Prev, DeviceID: in.DeviceID, Snapshot: in.Snapshot, Base: in.Base,
		Body: in.Body, Sig: in.Sig, Created: in.Created.UTC(), By: rc.device.ID, Group: in.Group})
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, api.CodeNotFound, "dispositivo no encontrado")
		return
	case errors.Is(err, store.ErrConflict):
		writeError(w, http.StatusConflict, api.CodeConflict,
			"la cadena de ese dispositivo ha cambiado o ese id ya existe; vuelve a leer sus revisiones y publica sobre la última")
		return
	case err != nil:
		s.internal(w, r, err)
		return
	}
	s.audit(r, rc, "revision.publish", map[string]any{"id": got.ID, "device": got.DeviceID, "snapshot": got.Snapshot})
	writeJSON(w, http.StatusCreated, toRevFull(got))
}

// pendingRevision es lo que pregunta el agente de cada máquina: «¿qué tengo
// que aplicar?». Sin nada que hacer responde 204 y no 404: no encontrar una
// orden es el caso normal, y un agente que consulta cada pocos minutos llenaría
// el log de errores que no lo son.
func (s *srv) pendingRevision(w http.ResponseWriter, r *http.Request, rc reqCtx) {
	rev, err := s.cfg.Store.PendingRevision(r.Context(), rc.user.ID, rc.device.ID)
	if errors.Is(err, store.ErrNotFound) {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if err != nil {
		s.internal(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toRevFull(rev))
}

func (s *srv) getRevision(w http.ResponseWriter, r *http.Request, rc reqCtx) {
	id := r.PathValue("id")
	if !api.ValidID(id) {
		writeError(w, http.StatusNotFound, api.CodeNotFound, "revisión no encontrada")
		return
	}
	rev, err := s.cfg.Store.Revision(r.Context(), rc.user.ID, id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, api.CodeNotFound, "revisión no encontrada")
		return
	}
	if err != nil {
		s.internal(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toRevFull(rev))
}

func (s *srv) listRevisions(w http.ResponseWriter, r *http.Request, rc reqCtx) {
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
	list, err := s.cfg.Store.Revisions(r.Context(), rc.user.ID, dev, limit)
	if err != nil {
		s.internal(w, r, err)
		return
	}
	out := make([]api.RevisionMeta, 0, len(list))
	for _, rev := range list {
		out = append(out, toRevMeta(rev))
	}
	writeJSON(w, http.StatusOK, out)
}

// setRevisionState recoge el resultado de aplicar una revisión. Solo lo manda
// la máquina destinataria, y el almacén lo acota por dispositivo: que otro
// equipo cierre una orden ajena sería contar por él lo que no ha hecho.
func (s *srv) setRevisionState(w http.ResponseWriter, r *http.Request, rc reqCtx) {
	id := r.PathValue("id")
	var in api.RevisionStateIn
	if !decode(w, r, 64<<10, &in) {
		return
	}
	in.Reason = strings.TrimSpace(in.Reason)
	if !api.ValidID(id) || !api.RevStateReported(in.State) || len(in.Reason) > api.MaxReasonLen {
		writeError(w, http.StatusBadRequest, api.CodeBadRequest,
			"el estado debe ser applied, partial, conflict o failed, con un motivo de menos de 2000 caracteres")
		return
	}
	// Un «no se pudo» sin motivo deja al usuario delante de un callejón sin
	// salida en el portal: es justo donde va a buscar la explicación.
	if in.State != api.RevApplied && in.Reason == "" {
		writeError(w, http.StatusBadRequest, api.CodeBadRequest, "hace falta un motivo para "+in.State)
		return
	}
	rev, err := s.cfg.Store.SetRevisionState(r.Context(), rc.user.ID, rc.device.ID, id, in.State, in.Reason, s.cfg.Now().UTC())
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, api.CodeNotFound, "esa revisión no es de este dispositivo")
		return
	case errors.Is(err, store.ErrConflict):
		writeError(w, http.StatusConflict, api.CodeConflict, "esa revisión ya no está pendiente")
		return
	case err != nil:
		s.internal(w, r, err)
		return
	}
	s.audit(r, rc, "revision.state", map[string]any{"id": id, "state": in.State})
	writeJSON(w, http.StatusOK, toRevMeta(rev))
}
