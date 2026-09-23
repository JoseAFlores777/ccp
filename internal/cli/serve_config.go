package cli

// serve_config.go — el editor de configuración de la Fase C visto por máquina
// (spec 2026-09-18 §7, C4): config.items, config.item.get|put|delete,
// config.effective y mcp.list|put|delete|setTargets|disable.
//
// Son altas en el registro, así que el protocolo sigue en 1. Ninguno
// reimplementa nada: la lista sale de core.ConfigItems (C1), las escrituras de
// core.ConfigItemPut/Delete y de MCPPut/MCPDelete/MCPSetTargets/MCPSetEnabled
// (C2), y las filas de MCP de las mismas que pinta `ccp mcp` (C3). Así la GUI
// hereda las barreras —la capa que declara, el secreto en claro del .mcp.json,
// el archivo que esa capa no leería— sin que haya que repetirlas aquí.
//
// La capa llega SIEMPRE en los parámetros: serve no tiene terminal, así que no
// hay perfil activo del que tirar y adivinarlo sería escribir en otro sitio del
// que el usuario está mirando.

import (
	"encoding/json"
	"fmt"

	"github.com/JoseAFlores777/ccp/internal/core"
)

// serveWrite es lo que devuelve una escritura del editor: el ConfigWrite de
// core tal cual (archivo tocado, perfiles regenerados, proyecciones) más
// `restart_pending`, que en core es un método y no viajaría en el JSON. Se
// calcula aquí y no en la GUI porque la regla es de core: con la ventana viva
// la proyección se aplaza y el chat sigue con los MCP de antes (ADR 0016).
type serveWrite struct {
	core.ConfigWrite
	OK      bool     `json:"ok"`
	Restart []string `json:"restart_pending"`
}

func okWrite(w core.ConfigWrite) serveWrite {
	if w.Regenerated == nil {
		w.Regenerated = []string{}
	}
	return serveWrite{ConfigWrite: w, OK: true, Restart: w.RestartPending()}
}

// srvCfgRoots son las raíces del inventario de esta máquina. Cada método las
// pide de nuevo: entre una llamada y la siguiente el usuario puede haber
// creado un perfil desde la terminal.
func srvCfgRoots(s *server) (core.InventoryRoots, error) { return inventoryRoots(s.home) }

type cfgLayerParams struct {
	Layer core.ConfigLayer `json:"layer"`
}

type cfgRefParams struct {
	Ref   core.ConfigRef   `json:"ref"`
	Value core.ConfigValue `json:"value"`
	// IfAbsent lo pide la acción de capa de la GUI: escribe solo si el
	// destino no tiene ya otro contenido. Campo añadido, no forma cambiada:
	// una edición normal lo omite y escribe como siempre.
	IfAbsent bool `json:"if_absent,omitempty"`
}

func srvConfigItems(s *server, raw json.RawMessage) (any, error) {
	p, err := params[cfgLayerParams](raw)
	if err != nil {
		return nil, err
	}
	r, err := srvCfgRoots(s)
	if err != nil {
		return nil, err
	}
	list, err := core.ConfigItems(r, p.Layer)
	if err != nil {
		return nil, err
	}
	return list, nil
}

func srvConfigItemGet(s *server, raw json.RawMessage) (any, error) {
	p, err := params[cfgRefParams](raw)
	if err != nil {
		return nil, err
	}
	r, err := srvCfgRoots(s)
	if err != nil {
		return nil, err
	}
	v, err := core.ConfigItemGet(r, p.Ref)
	if err != nil {
		return nil, err
	}
	return v, nil
}

func srvConfigItemPut(s *server, raw json.RawMessage) (any, error) {
	p, err := params[cfgRefParams](raw)
	if err != nil {
		return nil, err
	}
	r, err := srvCfgRoots(s)
	if err != nil {
		return nil, err
	}
	w, err := core.ConfigItemPutWith(r, p.Ref, p.Value, core.ConfigItemPutOpts{IfAbsent: p.IfAbsent})
	if err != nil {
		return nil, err
	}
	return okWrite(w), nil
}

// srvPreDeleteSnapshot es la red de los borrados del editor. Un «Quitar» de la
// GUI puede llevarse un archivo entero (el CLAUDE.md global) tras un solo
// modal, y por la ruta de serve no pasa ni el snapshot del día, que solo corre
// desde Dispatch. Como en profiles.rm y backup.restore: si no sale, no se
// borra nada.
func srvPreDeleteSnapshot(s *server) error {
	if _, err := autoSnapshot(s.home, "pre-config-delete"); err != nil {
		return fmt.Errorf("no se pudo guardar el snapshot de seguridad y no se borró nada: %w", err)
	}
	return nil
}

func srvConfigItemDelete(s *server, raw json.RawMessage) (any, error) {
	p, err := params[cfgRefParams](raw)
	if err != nil {
		return nil, err
	}
	r, err := srvCfgRoots(s)
	if err != nil {
		return nil, err
	}
	if err := srvPreDeleteSnapshot(s); err != nil {
		return nil, err
	}
	w, err := core.ConfigItemDelete(r, p.Ref)
	if err != nil {
		return nil, err
	}
	return okWrite(w), nil
}

// srvMCPCtx arma el contexto de `ccp mcp` a partir del servidor: la lista de la
// GUI son las mismas filas que las de la terminal, con sus destinos, sus
// apagados y los servidores que el perfil recibe sin estar proyectados.
func srvMCPCtx(s *server) (mcpCtx, error) {
	c := mcpCtx{lang: currentLang(), home: s.home}
	r, err := srvCfgRoots(s)
	if err != nil {
		return c, err
	}
	cfg, err := core.Load(s.home)
	if err != nil {
		return c, err
	}
	c.roots, c.cfg = r, cfg
	return c, nil
}

func srvMCPList(s *server, raw json.RawMessage) (any, error) {
	p, err := params[cfgLayerParams](raw)
	if err != nil {
		return nil, err
	}
	c, err := srvMCPCtx(s)
	if err != nil {
		return nil, err
	}
	rows, err := mcpRowsFor(c, p.Layer)
	if err != nil {
		return nil, err
	}
	return rows, nil
}

type mcpPutParams struct {
	Layer core.ConfigLayer `json:"layer"`
	Name  string           `json:"name"`
	Def   map[string]any   `json:"def"`
	// IfAbsent: igual que en config.item.put, para la acción de capa.
	IfAbsent bool `json:"if_absent,omitempty"`
}

func srvMCPPut(s *server, raw json.RawMessage) (any, error) {
	p, err := params[mcpPutParams](raw)
	if err != nil {
		return nil, err
	}
	r, err := srvCfgRoots(s)
	if err != nil {
		return nil, err
	}
	w, err := core.MCPPutWith(r, p.Layer, p.Name, p.Def, core.ConfigItemPutOpts{IfAbsent: p.IfAbsent})
	if err != nil {
		return nil, err
	}
	return okWrite(w), nil
}

func srvMCPDelete(s *server, raw json.RawMessage) (any, error) {
	p, err := params[mcpPutParams](raw)
	if err != nil {
		return nil, err
	}
	r, err := srvCfgRoots(s)
	if err != nil {
		return nil, err
	}
	if err := srvPreDeleteSnapshot(s); err != nil {
		return nil, err
	}
	w, err := core.MCPDelete(r, p.Layer, p.Name)
	if err != nil {
		return nil, err
	}
	return okWrite(w), nil
}

// srvMCPSetTargets no lleva capa a propósito: los destinos de un servidor viven
// en ccp.yaml por NOMBRE, no en la capa que lo declara, porque el mismo nombre
// declarado en dos capas es el mismo servidor para quien lo consume.
func srvMCPSetTargets(s *server, raw json.RawMessage) (any, error) {
	p, err := params[struct {
		Name    string   `json:"name"`
		Targets []string `json:"targets"`
	}](raw)
	if err != nil {
		return nil, err
	}
	r, err := srvCfgRoots(s)
	if err != nil {
		return nil, err
	}
	w, err := core.MCPSetTargets(r, p.Name, p.Targets)
	if err != nil {
		return nil, err
	}
	return okWrite(w), nil
}

// srvMCPDisable apaga un servidor heredado en UN perfil. Se llama como la
// acción que nombra el diseño, pero lleva `enabled` para poder volver atrás:
// el conmutador de la GUI es uno solo y un segundo método (mcp.enable) sería
// otra ruta que mantener para la misma escritura.
func srvMCPDisable(s *server, raw json.RawMessage) (any, error) {
	p, err := params[struct {
		Profile string `json:"profile"`
		Name    string `json:"name"`
		Enabled bool   `json:"enabled"`
	}](raw)
	if err != nil {
		return nil, err
	}
	r, err := srvCfgRoots(s)
	if err != nil {
		return nil, err
	}
	w, err := core.MCPSetEnabled(r, p.Profile, p.Name, p.Enabled)
	if err != nil {
		return nil, err
	}
	return okWrite(w), nil
}

// srvMCPAdoptDesktop pasa a ccp un MCP escrito a mano en el chat de la ventana
// de un perfil y lo deja con `def` (o como está, sin `def`): ver
// core.MCPAdoptDesktop. Es lo que permite editar desde la app un servidor que
// ccp no escribió.
func srvMCPAdoptDesktop(s *server, raw json.RawMessage) (any, error) {
	p, err := params[struct {
		Profile string         `json:"profile"`
		Name    string         `json:"name"`
		Def     map[string]any `json:"def"`
	}](raw)
	if err != nil {
		return nil, err
	}
	r, err := srvCfgRoots(s)
	if err != nil {
		return nil, err
	}
	w, err := core.MCPAdoptDesktop(r, p.Profile, p.Name, p.Def)
	if err != nil {
		return nil, err
	}
	return okWrite(w), nil
}
