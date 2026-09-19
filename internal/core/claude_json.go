package core

// claude_json.go — la parte de CONFIGURACIÓN de un .claude.json.
//
// Ese archivo mezcla lo que el usuario configura (los MCP de scope user, y por
// proyecto sus MCP, sus aprobaciones de .mcp.json y sus herramientas
// permitidas) con decenas de claves de estado que Claude Code reescribe
// constantemente (machineID, cachés, contadores, oauthAccount). Un snapshot
// guarda solo lo primero, y restaurar lo FUSIONA con el archivo vivo.
// Sustituir el archivo entero pisaría la identidad de la máquina y el estado de
// una sesión en marcha.

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// claudeJSONProjectKeys son las claves de configuración de cada proyecto.
var claudeJSONProjectKeys = []string{"mcpServers", "enabledMcpjsonServers", "disabledMcpjsonServers", "allowedTools"}

// jsonEmpty dice si raw es null, "", {} o [], con o sin espacios.
func jsonEmpty(raw json.RawMessage) bool {
	var b bytes.Buffer
	if err := json.Compact(&b, raw); err != nil {
		return false
	}
	switch b.String() {
	case "", "null", "{}", "[]", `""`:
		return true
	}
	return false
}

// ClaudeJSONConfig extrae la configuración de un .claude.json. Devuelve nil si
// no hay ninguna que guardar.
func ClaudeJSONConfig(data []byte) ([]byte, error) {
	var top map[string]json.RawMessage
	if err := json.Unmarshal(data, &top); err != nil {
		return nil, fmt.Errorf(".claude.json no es un objeto JSON: %w", err)
	}
	out := map[string]any{}
	if raw, ok := top["mcpServers"]; ok && !jsonEmpty(raw) {
		out["mcpServers"] = raw
	}
	if raw, ok := top["projects"]; ok && !jsonEmpty(raw) {
		var projects map[string]map[string]json.RawMessage
		if err := json.Unmarshal(raw, &projects); err != nil {
			return nil, fmt.Errorf(".claude.json: projects: %w", err)
		}
		picked := map[string]map[string]json.RawMessage{}
		for path, p := range projects {
			keep := map[string]json.RawMessage{}
			for _, k := range claudeJSONProjectKeys {
				if v, ok := p[k]; ok && !jsonEmpty(v) {
					keep[k] = v
				}
			}
			if len(keep) > 0 {
				picked[path] = keep
			}
		}
		if len(picked) > 0 {
			out["projects"] = picked
		}
	}
	if len(out) == 0 {
		return nil, nil
	}
	b, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return nil, fmt.Errorf(".claude.json: %w", err)
	}
	return append(b, '\n'), nil
}

// ClaudeJSONApplyConfig deja la configuración de live exactamente como dice cfg
// (lo que cfg no trae, se quita) y conserva intacto todo lo demás. live vacío
// significa que el archivo no existe.
func ClaudeJSONApplyConfig(live, cfg []byte) ([]byte, error) {
	top := map[string]json.RawMessage{}
	if len(bytes.TrimSpace(live)) > 0 {
		if err := json.Unmarshal(live, &top); err != nil {
			return nil, fmt.Errorf(".claude.json no es un objeto JSON: %w", err)
		}
		if top == nil {
			top = map[string]json.RawMessage{}
		}
	}
	var want struct {
		MCPServers json.RawMessage                       `json:"mcpServers"`
		Projects   map[string]map[string]json.RawMessage `json:"projects"`
	}
	if err := json.Unmarshal(cfg, &want); err != nil {
		return nil, fmt.Errorf("configuración del snapshot inválida: %w", err)
	}
	if len(want.MCPServers) > 0 {
		top["mcpServers"] = want.MCPServers
	} else {
		delete(top, "mcpServers")
	}

	rawProjects, hadProjects := top["projects"]
	projects := map[string]map[string]json.RawMessage{}
	if hadProjects && !jsonEmpty(rawProjects) {
		if err := json.Unmarshal(rawProjects, &projects); err != nil {
			return nil, fmt.Errorf(".claude.json: projects: %w", err)
		}
	}
	for _, p := range projects {
		for _, k := range claudeJSONProjectKeys {
			delete(p, k)
		}
	}
	for path, keys := range want.Projects {
		p := projects[path]
		if p == nil {
			p = map[string]json.RawMessage{}
			projects[path] = p
		}
		for _, k := range claudeJSONProjectKeys {
			if v, ok := keys[k]; ok {
				p[k] = v
			}
		}
	}
	if len(projects) > 0 || hadProjects {
		b, err := json.Marshal(projects)
		if err != nil {
			return nil, fmt.Errorf(".claude.json: projects: %w", err)
		}
		top["projects"] = b
	}
	out, err := json.MarshalIndent(top, "", "  ")
	if err != nil {
		return nil, fmt.Errorf(".claude.json: %w", err)
	}
	return append(out, '\n'), nil
}
