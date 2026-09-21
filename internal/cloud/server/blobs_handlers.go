package server

// blobs_handlers.go — leer y escribir un blob por el MISMO origen que el API.
//
// El resto del mundo sube y baja con URLs prefirmadas y los bytes no pasan por
// aquí; el portal no puede. Su CSP deja salir solo hacia su propio origen y
// hacia Keycloak, y ampliarla no bastaría: el bucket tendría que responder
// CORS a la pestaña, que es configuración de despliegue invisible desde el
// código y que falla en el navegador y en ningún log. Así que estos dos
// endpoints existen para él. Lo que mueven sigue sellado: el servidor no gana
// con ellos ninguna capacidad de leer nada.

import (
	"errors"
	"io"
	"net/http"

	"github.com/JoseAFlores777/ccp/internal/cloud/api"
	"github.com/JoseAFlores777/ccp/internal/cloud/blobs"
)

func (s *srv) getBlob(w http.ResponseWriter, r *http.Request, rc reqCtx) {
	id := r.PathValue("id")
	if !api.ValidID(id) {
		writeError(w, http.StatusNotFound, api.CodeNotFound, "blob no encontrado")
		return
	}
	// La clave lleva dentro al dueño: conocer el id de un blob ajeno no da
	// acceso a él, y por eso no hace falta preguntar de quién es.
	data, ok, err := s.cfg.Blobs.Get(r.Context(), blobs.Key(rc.user.ID, id))
	// Un objeto por encima del tope existe sin que nadie lo haya comprobado:
	// la URL PUT prefirmada no ata el tamaño. Servirlo sería cargarlo entero
	// en memoria, así que se contesta que es demasiado grande en vez de morir.
	if errors.Is(err, blobs.ErrTooLarge) {
		s.cfg.Log.Error("blob por encima del tope", "blob", id, "req", reqID(r))
		writeError(w, http.StatusRequestEntityTooLarge, api.CodeTooLarge, "el blob supera el máximo")
		return
	}
	if err != nil {
		s.cfg.Log.Error("almacenamiento", "err", err, "req", reqID(r))
		writeError(w, http.StatusServiceUnavailable, api.CodeInternal, "el almacenamiento no responde; reintenta")
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, api.CodeNotFound, "blob no encontrado")
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func (s *srv) putBlob(w http.ResponseWriter, r *http.Request, rc reqCtx) {
	id := r.PathValue("id")
	if !api.ValidID(id) {
		writeError(w, http.StatusBadRequest, api.CodeBadRequest, "id de blob inválido")
		return
	}
	data, err := io.ReadAll(io.LimitReader(r.Body, api.MaxBlobBytes+1))
	if err != nil {
		writeError(w, http.StatusBadRequest, api.CodeBadRequest, "no se pudo leer el cuerpo")
		return
	}
	if len(data) > api.MaxBlobBytes {
		writeError(w, http.StatusRequestEntityTooLarge, api.CodeTooLarge, "un blob supera el máximo")
		return
	}
	if len(data) == 0 {
		writeError(w, http.StatusBadRequest, api.CodeBadRequest, "un blob vacío no es un blob")
		return
	}
	key := blobs.Key(rc.user.ID, id)
	// Un blob ya subido no se reescribe: su contenido es el mismo por
	// construcción (el id es su HMAC), así que volver a mandarlo es un
	// reintento, no un cambio. Se responde 200 y no 201 para que el cliente
	// pueda distinguir «lo subí yo» de «ya estaba».
	if _, ok, err := s.cfg.Blobs.Head(r.Context(), key); err == nil && ok {
		w.WriteHeader(http.StatusOK)
		return
	} else if err != nil {
		s.cfg.Log.Error("almacenamiento", "err", err, "req", reqID(r))
		writeError(w, http.StatusServiceUnavailable, api.CodeInternal, "el almacenamiento no responde; reintenta")
		return
	}
	if err := s.cfg.Blobs.Put(r.Context(), key, data); err != nil {
		s.cfg.Log.Error("almacenamiento", "err", err, "req", reqID(r))
		writeError(w, http.StatusServiceUnavailable, api.CodeInternal, "el almacenamiento no responde; reintenta")
		return
	}
	s.audit(r, rc, "blob.put", map[string]any{"id": id, "bytes": len(data)})
	w.WriteHeader(http.StatusCreated)
}
