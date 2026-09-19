package core

// mcp_layers.go — los MCP por capas (spec 2026-09-18 §6.1, Fase B). Global =
// mcpServers de ~/.claude.json (el scope user oficial de default); perfil =
// overlay/mcp.json con la forma de .mcp.json; proyecto = <repo>/.mcp.json, que
// Claude Code lee solo por cwd y ccp no proyecta. Lo que solo le importa a ccp
// (destinos, apagados) vive en el bloque `mcp:` de ccp.yaml.

import (
	"fmt"
	"slices"
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
