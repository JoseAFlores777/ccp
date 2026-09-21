package core

// mcp_project.go — la proyección de los MCP declarados a lo que leen de verdad
// las apps (spec §6.1, ADR 0016 D7):
//   - al CLI y a la pestaña Code: cc-home/.claude.json, fusionando (técnica A,
//     medida en M6: Claude Code conserva lo que se escribe desde fuera);
//   - al chat de Desktop: profiles/<n>/desktop/claude_desktop_config.json, solo
//     stdio (M3) y solo con la ventana cerrada (M5).
//
// Regla que no se negocia (spec §2, regla 3): ccp solo toca las entradas cuyo
// nombre registró como suyas. Lo que el usuario añadió a mano en un destino se
// respeta, y si choca con un nombre declarado se informa como conflicto en vez
// de pisarlo.

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
)

// managedFile es el registro de lo que ccp escribió en un destino. Es estado
// DERIVADO: si se pierde, se reconstruye tomando por gestionado todo nombre
// declarado cuyo valor en el destino sea idéntico al declarado (lo que ccp
// habría escrito). Nunca se captura en snapshots ni en backups.
type managedFile struct {
	MCP []string `json:"mcp"`
}

func readManaged(path string) []string {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var m managedFile
	if json.Unmarshal(b, &m) != nil {
		return nil
	}
	return m.MCP
}

func writeManaged(path string, names []string) error {
	sort.Strings(names)
	b, err := json.MarshalIndent(managedFile{MCP: names}, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(path, append(b, '\n'), 0o600)
}

// MCPProjection cuenta qué hizo (o haría) una proyección.
type MCPProjection struct {
	Target        string   `json:"target"` // cli | desktop
	File          string   `json:"file"`
	Written       []string `json:"written"`        // añadidos o actualizados
	Removed       []string `json:"removed"`        // gestionados que ya no son efectivos
	Conflicts     []string `json:"conflicts"`      // el destino ya tenía ese nombre a mano
	RemoteSkipped []string `json:"remote_skipped"` // http/sse que el chat de Desktop no carga (M3)
	Deferred      bool     `json:"deferred"`       // la ventana está corriendo: se aplica al arrancar
}

// Empty dice si no hubo nada que contar.
func (p MCPProjection) Empty() bool {
	return len(p.Written) == 0 && len(p.Removed) == 0 && len(p.Conflicts) == 0 &&
		len(p.RemoteSkipped) == 0 && !p.Deferred
}

// projectMCPInto fusiona los servidores declarados en el mapa mcpServers de un
// destino ya decodificado. Devuelve el informe y si algo cambió.
func projectMCPInto(servers map[string]any, want map[string]any, managed []string, p *MCPProjection) (map[string]any, []string, bool) {
	changed := false
	now := make([]string, 0, len(want))
	names := make([]string, 0, len(want))
	for n := range want {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		cur, exists := servers[n]
		if exists && !slices.Contains(managed, n) {
			// Lo puso el usuario a mano: ni se pisa ni se adopta en silencio.
			p.Conflicts = append(p.Conflicts, n)
			continue
		}
		now = append(now, n)
		if exists && jsonEqual(cur, want[n]) {
			continue
		}
		servers[n] = want[n]
		p.Written = append(p.Written, n)
		changed = true
	}
	// Lo que ccp escribió y ya no es efectivo se retira; lo ajeno, jamás.
	for _, n := range managed {
		if _, still := want[n]; still {
			continue
		}
		if _, exists := servers[n]; exists {
			delete(servers, n)
			p.Removed = append(p.Removed, n)
			changed = true
		}
	}
	sort.Strings(p.Removed)
	return servers, now, changed
}

// mcpWant son los servidores efectivos con ese destino, listos para escribir.
// Para desktop, las entradas remotas se dejan fuera y se informan: el archivo
// del chat solo carga stdio (M3).
func mcpWant(eff []MCPEntry, target string, p *MCPProjection) map[string]any {
	want := map[string]any{}
	for _, e := range eff {
		if !slices.Contains(e.Targets, target) {
			continue
		}
		if target == MCPTargetDesktop && e.Kind() != "stdio" {
			p.RemoteSkipped = append(p.RemoteSkipped, e.Name)
			continue
		}
		want[e.Name] = e.Def
	}
	return want
}

// ProjectMCPToCLI escribe los MCP efectivos en cc-home/.claude.json del perfil.
// `default` no se proyecta: su destino ES la capa global (~/.claude.json).
func ProjectMCPToCLI(home, name string, eff []MCPEntry) (MCPProjection, error) {
	return projectMCPToCLI(home, name, eff, false)
}

// CheckMCPToCLI es la misma proyección en modo check: calcula el informe y no
// escribe nada (ni el destino ni el registro de gestionados). Lo usa
// ProfileProjectionCheck, y con él `ccp profile sync --check`.
func CheckMCPToCLI(home, name string, eff []MCPEntry) (MCPProjection, error) {
	return projectMCPToCLI(home, name, eff, true)
}

func projectMCPToCLI(home, name string, eff []MCPEntry, dry bool) (MCPProjection, error) {
	p := MCPProjection{Target: MCPTargetCLI}
	if name == "" || name == "default" {
		return p, nil
	}
	cch := ccHomePath(home, name)
	if _, err := os.Stat(cch); err != nil {
		return p, nil // sin cc-home no hay a dónde proyectar (perfil a medio crear)
	}
	p.File = filepath.Join(cch, ".claude.json")
	managedPath := filepath.Join(cch, ".ccp-managed.json")

	live, err := os.ReadFile(p.File)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return p, fmt.Errorf("no se pudo leer %s: %w", p.File, err)
	}
	doc := map[string]any{}
	if len(bytes.TrimSpace(live)) > 0 {
		if doc, err = decodeJSONObject(live); err != nil {
			return p, fmt.Errorf("%s no es JSON válido; no se toca: %w", p.File, err)
		}
	}
	servers, _ := doc["mcpServers"].(map[string]any)
	if servers == nil {
		servers = map[string]any{}
	}
	want := mcpWant(eff, MCPTargetCLI, &p)
	managed := reconcileManaged(readManaged(managedPath), servers, want)

	servers, now, changed := projectMCPInto(servers, want, managed, &p)
	if !changed {
		// Aun así se deja el registro al día: un .ccp-managed.json perdido no
		// puede convertir en ajeno lo que ccp sí escribió.
		if !dry && !slices.Equal(readManaged(managedPath), now) {
			_ = writeManaged(managedPath, now)
		}
		return p, nil
	}
	if dry {
		return p, nil
	}
	if len(servers) == 0 {
		delete(doc, "mcpServers")
	} else {
		doc["mcpServers"] = servers
	}
	out, err := marshalIndent(doc)
	if err != nil {
		return p, err
	}
	perm := os.FileMode(0o600)
	if fi, err := os.Stat(p.File); err == nil {
		perm = fi.Mode().Perm()
	}
	if err := writeFileAtomic(p.File, out, perm); err != nil {
		return p, fmt.Errorf("no se pudo escribir %s: %w", p.File, err)
	}
	if err := writeManaged(managedPath, now); err != nil {
		return p, err
	}
	return p, nil
}

// reconcileManaged reconstruye el registro de nombres gestionados cuando falta o
// se quedó corto: un nombre declarado cuyo valor en el destino ya es exactamente
// el declarado lo escribió ccp (o da igual, porque coinciden).
func reconcileManaged(managed []string, servers, want map[string]any) []string {
	out := append([]string(nil), managed...)
	for n, v := range want {
		if slices.Contains(out, n) {
			continue
		}
		if cur, ok := servers[n]; ok && jsonEqual(cur, v) {
			out = append(out, n)
		}
	}
	return out
}

// desktopPendingPath marca que un perfil tiene proyección pendiente porque su
// ventana estaba abierta. Estado derivado, en state/ (fuera de snapshots).
func desktopPendingPath(home, name string) string {
	return filepath.Join(profileStateDir(home, name), "desktop-pending.json")
}

// DesktopProjectionPending dice si ese perfil tiene una proyección esperando al
// próximo arranque de su ventana. Sin data dir NO hay nada pendiente aunque el
// marcador siga en disco: `ccp desktop rm` borra la ventana y no toca state/, y
// un marcador huérfano dejaba al check y al doctor pidiendo para siempre que se
// reiniciara una ventana que ya no existe. Quien la borró porque dejó de usar
// Desktop ahí no va a ejecutar la secuencia que lo limpiaba.
func DesktopProjectionPending(home, name string) bool {
	if !fileExists(desktopPendingPath(home, name)) {
		return false
	}
	_, err := os.Stat(DesktopDataDir(home, name))
	return err == nil
}

// ProjectMCPToDesktop escribe los MCP efectivos con destino desktop en el
// claude_desktop_config.json de la ventana del perfil, conservando preferences y
// cualquier clave desconocida. Dos reglas salen de ADR 0016:
//   - solo stdio: una entrada http/sse la descarta Desktop («Skipped invalid MCP
//     server config entries»), así que no se escribe y se informa (M3);
//   - con la ventana corriendo NO se escribe: Desktop no relee en caliente (M3) y
//     reescribe el archivo desde su copia en memoria (M5), así que se deja
//     pendiente y se aplica al arrancar, donde ya se espeja el cc-home.
//
// `running` lo decide quien llama (core no ejecuta ps).
func ProjectMCPToDesktop(home, name string, eff []MCPEntry, running bool) (MCPProjection, error) {
	return projectMCPToDesktop(home, name, eff, running, false)
}

// CheckMCPToDesktop cuenta el desfase del chat sin escribir. `running` no entra:
// con la ventana abierta la escritura se aplaza, pero el desfase existe igual y
// es justo lo que hay que contar. Deferred sale del marcador que dejó la última
// proyección aplazada, no de una sonda de procesos.
func CheckMCPToDesktop(home, name string, eff []MCPEntry) (MCPProjection, error) {
	p, err := projectMCPToDesktop(home, name, eff, false, true)
	p.Deferred = DesktopProjectionPending(home, name)
	return p, err
}

func projectMCPToDesktop(home, name string, eff []MCPEntry, running, dry bool) (MCPProjection, error) {
	p := MCPProjection{Target: MCPTargetDesktop}
	if name == "" {
		return p, nil
	}
	dir := DesktopDataDir(home, name)
	if _, err := os.Stat(dir); err != nil {
		// Esa ventana no se ha usado nunca (o se borró): nada que proyectar. Y
		// si quedó un marcador de una ventana desaparecida, este es el único
		// paso que vuelve a pasar por aquí, así que se limpia de disco.
		if !dry {
			_ = os.Remove(desktopPendingPath(home, name))
		}
		return p, nil
	}
	p.File = filepath.Join(dir, "claude_desktop_config.json")
	want := mcpWant(eff, MCPTargetDesktop, &p)
	if running && !dry {
		// Lo que se sabe de M5 no basta para escribir con la ventana viva.
		if err := writeFileAtomic(desktopPendingPath(home, name), []byte("{}\n"), 0o600); err != nil {
			return p, err
		}
		p.Deferred = true
		return p, nil
	}

	managedPath := filepath.Join(dir, ".ccp-managed-mcp.json")
	live, err := os.ReadFile(p.File)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return p, fmt.Errorf("no se pudo leer %s: %w", p.File, err)
	}
	doc := map[string]any{}
	if len(bytes.TrimSpace(live)) > 0 {
		if doc, err = decodeJSONObject(live); err != nil {
			return p, fmt.Errorf("%s no es JSON válido; no se toca: %w", p.File, err)
		}
	}
	servers, _ := doc["mcpServers"].(map[string]any)
	if servers == nil {
		servers = map[string]any{}
	}
	managed := reconcileManaged(readManaged(managedPath), servers, want)
	servers, now, changed := projectMCPInto(servers, want, managed, &p)
	if dry {
		return p, nil
	}
	if changed {
		if len(servers) == 0 {
			delete(doc, "mcpServers")
		} else {
			doc["mcpServers"] = servers
		}
		out, err := marshalIndent(doc)
		if err != nil {
			return p, err
		}
		perm := os.FileMode(0o600)
		if fi, err := os.Stat(p.File); err == nil {
			perm = fi.Mode().Perm()
		}
		if err := writeFileAtomic(p.File, out, perm); err != nil {
			return p, fmt.Errorf("no se pudo escribir %s: %w", p.File, err)
		}
	}
	if err := writeManaged(managedPath, now); err != nil {
		return p, err
	}
	_ = os.Remove(desktopPendingPath(home, name)) // ya está aplicado
	return p, nil
}

// ApplyDesktopPending aplica la proyección que quedó aplazada por tener la
// ventana viva. Lo llama el arranque de la ventana (`ccp desktop open` y el
// lanzador del Dock) ANTES de lanzar, que es el único momento en que el archivo
// del chat se puede escribir sin que Desktop lo pise desde su copia en memoria.
// Sin esto el marcador sobrevivía a abrir y cerrar la ventana: el doctor seguía
// pidiendo justo la acción que no lo arreglaba.
//
// Devuelve applied=false cuando no había nada pendiente, para que el front-end
// no anuncie un trabajo que no hizo.
func ApplyDesktopPending(home, name string) (MCPProjection, bool, error) {
	if name == "" || name == "default" || !DesktopProjectionPending(home, name) {
		return MCPProjection{Target: MCPTargetDesktop}, false, nil
	}
	src, err := claudeSrc()
	if err != nil {
		return MCPProjection{Target: MCPTargetDesktop}, false, err
	}
	global, profile, err := ReadMCPLayers(home, src, name)
	if err != nil {
		return MCPProjection{Target: MCPTargetDesktop}, false, err
	}
	cfg, err := Load(home)
	if err != nil {
		return MCPProjection{Target: MCPTargetDesktop}, false, err
	}
	p, err := projectMCPToDesktop(home, name, MCPEffective(cfg, global, profile, name), false, false)
	return p, err == nil, err
}
