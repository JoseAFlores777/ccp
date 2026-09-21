package server

// Grupos de dispositivos (spec §10.3): «todas mis Macs». Un grupo es una
// etiqueta con miembros, y no autoriza nada. Aplicar a un grupo sigue siendo
// publicar N revisiones, una FIRMADA por máquina: si el grupo mandara,
// cambiar quién está dentro —algo que el servidor sí puede hacer, porque la
// membresía no va firmada— cambiaría a quién obedece una orden ya firmada.

import (
	"errors"
	"net/http"
	"sort"
	"strings"

	"github.com/JoseAFlores777/ccp/internal/cloud/api"
	"github.com/JoseAFlores777/ccp/internal/cloud/store"
)

func toGroup(g store.Group) api.Group {
	m := g.Members
	if m == nil {
		m = []string{}
	}
	return api.Group{ID: g.ID, Name: g.Name, Members: m, Created: g.Created, Updated: g.Updated}
}

// groupInput valida lo que llega y devuelve el grupo listo para el almacén.
// Un equipo revocado no entra: no va a volver a preguntar, así que la orden
// que se le publique se queda pendiente para siempre en el portal.
func (s *srv) groupInput(w http.ResponseWriter, r *http.Request, rc reqCtx, in api.GroupIn) (store.Group, bool) {
	name := strings.TrimSpace(in.Name)
	if name == "" || len(name) > api.MaxGroupNameLen || len(in.Members) > api.MaxGroupMembers {
		writeError(w, http.StatusBadRequest, api.CodeBadRequest,
			"un grupo necesita un nombre de menos de 64 caracteres y como mucho 200 equipos")
		return store.Group{}, false
	}
	for _, id := range in.Members {
		if !store.IsUUID(id) {
			writeError(w, http.StatusBadRequest, api.CodeBadRequest, "miembro inválido")
			return store.Group{}, false
		}
	}
	devs, err := s.cfg.Store.Devices(r.Context(), rc.user.ID)
	if err != nil {
		s.internal(w, r, err)
		return store.Group{}, false
	}
	revocado := map[string]bool{}
	for _, d := range devs {
		revocado[d.ID] = d.Revoked
	}
	for _, id := range in.Members {
		rev, existe := revocado[id]
		if !existe {
			writeError(w, http.StatusNotFound, api.CodeNotFound, "dispositivo no encontrado")
			return store.Group{}, false
		}
		if rev {
			writeError(w, http.StatusConflict, api.CodeConflict, "ese dispositivo está revocado")
			return store.Group{}, false
		}
	}
	return store.Group{Name: name, Members: in.Members}, true
}

// groupError traduce lo que puede devolver el almacén. El nombre repetido es
// un 409 y no un 400: la petición estaba bien, lo que pasa es que ya existe.
func (s *srv) groupError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, api.CodeNotFound, "grupo no encontrado")
	case errors.Is(err, store.ErrConflict):
		writeError(w, http.StatusConflict, api.CodeConflict, "ya hay un grupo con ese nombre")
	default:
		s.internal(w, r, err)
	}
}

func (s *srv) createGroup(w http.ResponseWriter, r *http.Request, rc reqCtx) {
	var in api.GroupIn
	if !decode(w, r, 64<<10, &in) {
		return
	}
	g, ok := s.groupInput(w, r, rc, in)
	if !ok {
		return
	}
	got, err := s.cfg.Store.CreateGroup(r.Context(), rc.user.ID, g)
	if err != nil {
		s.groupError(w, r, err)
		return
	}
	s.audit(r, rc, "group.create", map[string]any{"id": got.ID, "name": got.Name, "members": len(got.Members)})
	writeJSON(w, http.StatusCreated, toGroup(got))
}

func (s *srv) listGroups(w http.ResponseWriter, r *http.Request, rc reqCtx) {
	gs, err := s.cfg.Store.Groups(r.Context(), rc.user.ID)
	if err != nil {
		s.internal(w, r, err)
		return
	}
	out := make([]api.Group, 0, len(gs))
	for _, g := range gs {
		out = append(out, toGroup(g))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *srv) getGroup(w http.ResponseWriter, r *http.Request, rc reqCtx) {
	g, err := s.cfg.Store.Group(r.Context(), rc.user.ID, r.PathValue("id"))
	if err != nil {
		s.groupError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toGroup(g))
}

func (s *srv) updateGroup(w http.ResponseWriter, r *http.Request, rc reqCtx) {
	var in api.GroupIn
	if !decode(w, r, 64<<10, &in) {
		return
	}
	g, ok := s.groupInput(w, r, rc, in)
	if !ok {
		return
	}
	g.ID = r.PathValue("id")
	got, err := s.cfg.Store.UpdateGroup(r.Context(), rc.user.ID, g)
	if err != nil {
		s.groupError(w, r, err)
		return
	}
	s.audit(r, rc, "group.update", map[string]any{"id": got.ID, "name": got.Name, "members": len(got.Members)})
	writeJSON(w, http.StatusOK, toGroup(got))
}

func (s *srv) deleteGroup(w http.ResponseWriter, r *http.Request, rc reqCtx) {
	id := r.PathValue("id")
	if err := s.cfg.Store.DeleteGroup(r.Context(), rc.user.ID, id); err != nil {
		s.groupError(w, r, err)
		return
	}
	s.audit(r, rc, "group.delete", map[string]any{"id": id})
	w.WriteHeader(http.StatusNoContent)
}

// groupStatus es el estado por dispositivo dentro del grupo: qué orden le tocó
// a cada equipo y cómo acabó. Se pide el historial ENTERO del grupo (limit 0)
// porque lo que se cuenta es la última de CADA equipo: una ventana dejaría
// fuera justo la del que lleva más tiempo sin recibir nada.
//
// Salen dos clases de equipo y la segunda es la que importa: los que siguen en
// el grupo, y los que ya no están pero tienen una orden publicada con su
// etiqueta. Sacar a alguien del grupo NO retira la orden que ya tiene puesta,
// así que esconderle aquí dejaría una orden viva sin nadie que la contara.
func (s *srv) groupStatus(w http.ResponseWriter, r *http.Request, rc reqCtx) {
	g, err := s.cfg.Store.Group(r.Context(), rc.user.ID, r.PathValue("id"))
	if err != nil {
		s.groupError(w, r, err)
		return
	}
	devs, err := s.cfg.Store.Devices(r.Context(), rc.user.ID)
	if err != nil {
		s.internal(w, r, err)
		return
	}
	revs, err := s.cfg.Store.GroupRevisions(r.Context(), rc.user.ID, g.ID, 0)
	if err != nil {
		s.internal(w, r, err)
		return
	}
	// Vienen de la más nueva a la más vieja, así que la primera de cada equipo
	// es la suya.
	ultima := map[string]store.Revision{}
	for _, rev := range revs {
		if _, ya := ultima[rev.DeviceID]; !ya {
			ultima[rev.DeviceID] = rev
		}
	}
	miembro := map[string]bool{}
	for _, id := range g.Members {
		miembro[id] = true
	}
	out := []api.GroupMemberStatus{}
	for _, d := range devs {
		rev, tiene := ultima[d.ID]
		if !miembro[d.ID] && !tiene {
			continue
		}
		m := api.GroupMemberStatus{DeviceID: d.ID, DeviceName: d.Name, Revoked: d.Revoked, Member: miembro[d.ID]}
		if tiene {
			m.Revision, m.State, m.Reason = rev.ID, rev.State, rev.Reason
			m.Created, m.Updated = rev.Created, rev.Updated
		}
		out = append(out, m)
	}
	// Primero quien sigue dentro: es la lista que el usuario tiene en la
	// cabeza, y los ex-miembros son la nota al pie.
	sort.SliceStable(out, func(i, j int) bool { return out[i].Member && !out[j].Member })
	writeJSON(w, http.StatusOK, api.GroupStatus{Group: toGroup(g), Members: out})
}
