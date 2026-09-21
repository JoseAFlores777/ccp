package client

// Grupos de dispositivos (spec §10.3). El cliente solo los lee y los escribe:
// aplicar a un grupo no es una llamada nueva, es publicar una revisión FIRMADA
// por miembro con la etiqueta del grupo. Ver `ResolveGroup`.

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/JoseAFlores777/ccp/internal/cloud/api"
)

// ErrGroupNotFound: no hay ningún grupo con ese nombre o ese id.
var ErrGroupNotFound = errors.New("grupo no encontrado")

// CreateGroup crea un grupo de dispositivos.
func (a *API) CreateGroup(ctx context.Context, in api.GroupIn) (api.Group, error) {
	var g api.Group
	return g, a.do(ctx, http.MethodPost, "/v1/groups", in, &g)
}

// Groups lista los grupos de la cuenta.
func (a *API) Groups(ctx context.Context) ([]api.Group, error) {
	var gs []api.Group
	return gs, a.do(ctx, http.MethodGet, "/v1/groups", nil, &gs)
}

// Group devuelve un grupo por su id.
func (a *API) Group(ctx context.Context, id string) (api.Group, error) {
	var g api.Group
	return g, a.do(ctx, http.MethodGet, "/v1/groups/"+url.PathEscape(id), nil, &g)
}

// UpdateGroup reescribe nombre y miembros. Los miembros van enteros y no por
// diferencias: mandar la lista que se ve es lo único que no depende de qué
// versión del grupo tenía delante quien la manda.
func (a *API) UpdateGroup(ctx context.Context, id string, in api.GroupIn) (api.Group, error) {
	var g api.Group
	return g, a.do(ctx, http.MethodPut, "/v1/groups/"+url.PathEscape(id), in, &g)
}

// DeleteGroup borra el grupo, nunca las órdenes publicadas con su etiqueta.
func (a *API) DeleteGroup(ctx context.Context, id string) error {
	return a.do(ctx, http.MethodDelete, "/v1/groups/"+url.PathEscape(id), nil, nil)
}

// GroupStatus es el estado por dispositivo dentro del grupo.
func (a *API) GroupStatus(ctx context.Context, id string) (api.GroupStatus, error) {
	var st api.GroupStatus
	return st, a.do(ctx, http.MethodGet, "/v1/groups/"+url.PathEscape(id)+"/status", nil, &st)
}

// ResolveGroup busca un grupo por id, por prefijo de id o por nombre, que es
// como lo escribe una persona en el CLI. El prefijo hace falta porque
// `ccp cloud groups` pinta la columna «ID» recortada a 8 caracteres: sin él, lo
// que el usuario copia de la pantalla es justo lo que el CLI rechaza (igual que
// findDevice, que sí acepta prefijo). El nombre se compara sin distinguir
// mayúsculas porque quien teclea «macs» está nombrando «Macs»; si dos grupos
// empatan así, se dice, en vez de elegir uno por el orden en que vinieron.
func ResolveGroup(ctx context.Context, a *API, ref string) (api.Group, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return api.Group{}, ErrGroupNotFound
	}
	gs, err := a.Groups(ctx)
	if err != nil {
		return api.Group{}, err
	}
	var porNombre, porPrefijo []api.Group
	for _, g := range gs {
		if g.ID == ref {
			return g, nil
		}
		if strings.EqualFold(g.Name, ref) {
			porNombre = append(porNombre, g)
		}
		if strings.HasPrefix(g.ID, ref) {
			porPrefijo = append(porPrefijo, g)
		}
	}
	// El nombre manda sobre el prefijo: es lo que la persona eligió para el
	// grupo, mientras que el id lo puso la nube.
	switch {
	case len(porNombre) == 1:
		return porNombre[0], nil
	case len(porNombre) > 1:
		return api.Group{}, fmt.Errorf("hay %d grupos que se llaman %q: usa su id", len(porNombre), ref)
	case len(porPrefijo) == 1:
		return porPrefijo[0], nil
	case len(porPrefijo) > 1:
		return api.Group{}, fmt.Errorf("hay %d grupos cuyo id empieza por %q: escribe más caracteres", len(porPrefijo), ref)
	}
	return api.Group{}, ErrGroupNotFound
}
