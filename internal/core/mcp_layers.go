package core

// mcp_layers.go — los MCP por capas (spec 2026-09-18 §6.1, Fase B). Global =
// mcpServers de ~/.claude.json (el scope user oficial de default); perfil =
// overlay/mcp.json con la forma de .mcp.json; proyecto = <repo>/.mcp.json, que
// Claude Code lee solo por cwd y ccp no proyecta. Lo que solo le importa a ccp
// (destinos, apagados) vive en el bloque `mcp:` de ccp.yaml.

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
)

// Destinos de un MCP.
const (
	MCPTargetCLI     = "cli"
	MCPTargetDesktop = "desktop"
)

// MCPConfig es el bloque `mcp:` de ccp.yaml. Extra conserva las claves que este
// binario no conoce, como hace AutoHandoff.
type MCPConfig struct {
	// Targets por nombre de servidor; sin entrada = [cli, desktop].
	Targets map[string][]string `yaml:"targets,omitempty"`
	// Disabled por perfil: nombres heredados que ese perfil apaga sin borrarlos
	// del global.
	Disabled map[string][]string `yaml:"disabled,omitempty"`
	// DesktopDefault: la ventana default (el Claude del usuario) también recibe
	// proyección. Apagado por defecto, como la barrera del updater (D8).
	DesktopDefault bool `yaml:"desktop_default,omitempty"`

	Extra map[string]any `yaml:",inline"`
}

// MCPTargets devuelve los destinos de un servidor: los declarados o, sin
// declarar, los dos.
func MCPTargets(cfg *Config, server string) []string {
	if cfg != nil && cfg.MCP != nil {
		if t, ok := cfg.MCP.Targets[server]; ok {
			return t
		}
	}
	return []string{MCPTargetCLI, MCPTargetDesktop}
}

// ValidateMCPTargets rechaza un destino que no existe.
func ValidateMCPTargets(ts []string) error {
	for _, t := range ts {
		if t != MCPTargetCLI && t != MCPTargetDesktop {
			return fmt.Errorf("destino de MCP desconocido %q (valen: cli, desktop)", t)
		}
	}
	return nil
}

// MCPDisabled dice si un perfil apaga un servidor heredado.
func MCPDisabled(cfg *Config, profile, server string) bool {
	return cfg != nil && cfg.MCP != nil && slices.Contains(cfg.MCP.Disabled[profile], server)
}

// mcpProfileFile es la capa de MCP de un perfil: overlay/mcp.json, con la forma
// de .mcp.json ({"mcpServers": {…}}), para que el usuario y otras herramientas la
// lean sin ccp (spec §2, regla 2).
func mcpProfileFile(home, name string) string {
	return filepath.Join(cfgOverlayDir(home, name), "mcp.json")
}

// MCPProfileFile expone la ruta a los front-ends.
func MCPProfileFile(home, name string) string { return mcpProfileFile(home, name) }

// ReadMCPLayers devuelve los mcpServers de la capa global (src + ".json") y de la
// del perfil. Un archivo que falta es una capa vacía; uno que no es JSON es un
// error con su ruta: quien regenera no puede proyectar a ciegas.
func ReadMCPLayers(home, src, name string) (global, profile map[string]any, err error) {
	read := func(p string) (map[string]any, error) {
		b, err := os.ReadFile(p)
		if errors.Is(err, fs.ErrNotExist) {
			return map[string]any{}, nil
		}
		if err != nil {
			return nil, fmt.Errorf("no se pudo leer %s: %w", p, err)
		}
		if len(bytes.TrimSpace(b)) == 0 {
			return map[string]any{}, nil
		}
		m, err := decodeJSONObject(b)
		if err != nil {
			return nil, fmt.Errorf("%s no es JSON válido: %w", p, err)
		}
		s, _ := m["mcpServers"].(map[string]any)
		if s == nil {
			s = map[string]any{}
		}
		return s, nil
	}
	if global, err = read(src + ".json"); err != nil {
		return nil, nil, err
	}
	if name == "default" || name == "" {
		return global, map[string]any{}, nil
	}
	if profile, err = read(mcpProfileFile(home, name)); err != nil {
		return nil, nil, err
	}
	return global, profile, nil
}

// MCPEntry es un servidor tal como lo recibe un perfil.
type MCPEntry struct {
	Name     string         `json:"name"`
	Def      map[string]any `json:"-"`      // la definición entera, secretos incluidos: nunca se serializa
	Origin   string         `json:"origin"` // global | overlay
	Targets  []string       `json:"targets"`
	Shadowed bool           `json:"shadowed"` // el perfil tapa uno del global con el mismo nombre
}

// Kind es stdio (hay command) o el type declarado (http/sse), con http por
// defecto si solo hay url.
func (e MCPEntry) Kind() string { return mcpKind(e.Def) }

func mcpKind(def map[string]any) string {
	if t, _ := def["type"].(string); t != "" && t != "stdio" {
		return t
	}
	if c, _ := def["command"].(string); c != "" {
		return "stdio"
	}
	if u, _ := def["url"].(string); u != "" {
		return "http"
	}
	return "stdio"
}

// MCPEffective = global ⊕ perfil − disabled; si el nombre choca, gana el perfil.
// Ordenado por nombre.
func MCPEffective(cfg *Config, global, profile map[string]any, name string) []MCPEntry {
	byName := map[string]MCPEntry{}
	for n, v := range global {
		if d, ok := v.(map[string]any); ok {
			byName[n] = MCPEntry{Name: n, Def: d, Origin: "global"}
		}
	}
	for n, v := range profile {
		if d, ok := v.(map[string]any); ok {
			_, shadow := byName[n]
			byName[n] = MCPEntry{Name: n, Def: d, Origin: "overlay", Shadowed: shadow}
		}
	}
	out := make([]MCPEntry, 0, len(byName))
	for n, e := range byName {
		if MCPDisabled(cfg, name, n) {
			continue
		}
		e.Targets = MCPTargets(cfg, n)
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
