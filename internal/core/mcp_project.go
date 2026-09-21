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
		if !slices.Equal(readManaged(managedPath), now) {
			_ = writeManaged(managedPath, now)
		}
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
