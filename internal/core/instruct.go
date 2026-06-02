package core

// instruct.go — destino + CRUD para `ccp instruct` (respaldo de
// /ccp:remember-* , /ccp:recall , /ccp:forget). Port de lib/instruct.sh.
//
// Funciones puras: reciben home / src / repoRoot / activeProfile explícitos, así
// los tests usan t.TempDir() y nunca tocan ~/.config/ccp ni ~/.claude reales.
//
// 6 tipos de artefacto -> estructura OFICIAL de Claude Code:
//   rule    -> CLAUDE.md (línea dentro de un bloque con marcadores)
//   agent   -> agents/<slug>.md       (archivo, lo escribe Claude)
//   command -> commands/<slug>.md     (archivo, lo escribe Claude)
//   skill   -> skills/<slug>/         (dir, lo crea Claude)
//   hook    -> settings.json .hooks   (entrada JSON, deep-merge vía MergeJSON)
//   mcp     -> settings/.mcp.json .mcpServers (entrada JSON)
//
// Scope: global (~/.claude) | profile (overlay) | project (repo/.claude).
// En profile solo rule/hook (mcp es solo global/project; agents/commands/
// skills van symlinkeados desde global; ver docs/adr/0005).
//
// Manifest de artefactos creados por ccp (asimetría a propósito, ver plan §4):
//   global/profile -> ccp.yaml .authored (machine-local, vía Load/Save)
//   project        -> <repo>/.claude/ccp-authored.tsv (versionado con el repo;
//                     ccp NO reescribe los archivos del repo: append-only TSV)

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Marcadores del bloque gestionado de instrucciones (tipo 'rule'); espejan el
// idioma de `# >>> ccp shell init >>>` y los del bash (ADR-0004).
const (
	instrBegin = "<!-- >>> ccp instructions >>> -->"
	instrEnd   = "<!-- <<< ccp instructions <<< -->"
)

// DestError es un fallo de resolución de destino con un código estable (espeja
// los rc 1..5 de ccp_instruct_dest del bash) para que el CLI mapee a mensajes
// en español y exit codes.
type DestError struct {
	Code int    // 1 scope/tipo inválido | 2 profile=default | 3 tipo no-profile | 4 project sin repo | 5 mcp en profile
	Msg  string // mensaje principal en español (lo emite el CLI por stderr)
	Hint string // sugerencia opcional (info) en español
}

func (e *DestError) Error() string { return e.Msg }

// InstructDest devuelve la ruta destino (archivo o directorio) para
// (scope, typ). Espeja ccp_instruct_dest del bash. activeProfile es el perfil
// activo (CCP_PROFILE; "default" o "" si ninguno); src es la fuente global
// (CCP_CLAUDE_SRC o ~/.claude); repoRoot es la raíz del repo para scope project.
func InstructDest(scope, typ, home, activeProfile, src, repoRoot string) (string, error) {
	switch scope {
	case "global":
		switch typ {
		case "rule":
			return filepath.Join(src, "CLAUDE.md"), nil
		case "hook":
			return filepath.Join(src, "settings.json"), nil
		case "mcp":
			// .mcp.json hermano del cc-home global: <src>.json (espeja el bash).
			return src + ".json", nil
		case "agent":
			return filepath.Join(src, "agents"), nil
		case "command":
			return filepath.Join(src, "commands"), nil
		case "skill":
			return filepath.Join(src, "skills"), nil
		default:
			return "", &DestError{Code: 1, Msg: fmt.Sprintf("scope/tipo inválido: %q/%q", scope, typ)}
		}
	case "profile":
		if activeProfile == "" || activeProfile == "default" {
			return "", &DestError{
				Code: 2,
				Msg:  "scope 'profile': el perfil activo es 'default' (sin overlay propio).",
				Hint: fmt.Sprintf("Usa 'ccp instruct add global %s ...' o activa un perfil ('ccp use <n>').", typ),
			}
		}
		ov := cfgOverlayDir(home, activeProfile)
		switch typ {
		case "rule":
			return filepath.Join(ov, "CLAUDE.md"), nil
		case "hook":
			return filepath.Join(ov, "settings.overlay.json"), nil
		case "mcp":
			return "", &DestError{
				Code: 5,
				Msg:  "scope 'profile': MCP por-perfil no está soportado todavía.",
				Hint: "Usa 'ccp instruct add global mcp ...' (todos los perfiles) o 'project' (este repo).",
			}
		case "agent", "command", "skill":
			return "", &DestError{
				Code: 3,
				Msg:  fmt.Sprintf("tipo '%s' no existe a nivel de perfil (se comparten desde global).", typ),
				Hint: "Usa scope 'global' (aplica a todos los perfiles) o 'project' (acota al repo).",
			}
		default:
			return "", &DestError{Code: 1, Msg: fmt.Sprintf("scope/tipo inválido: %q/%q", scope, typ)}
		}
	case "project":
		if repoRoot == "" {
			return "", &DestError{Code: 4, Msg: "scope 'project': no estás en un repo git y no hay CCP_REPO_ROOT."}
		}
		switch typ {
		case "rule":
			return filepath.Join(repoRoot, ".claude", "CLAUDE.md"), nil
		case "hook":
			return filepath.Join(repoRoot, ".claude", "settings.json"), nil
		case "mcp":
			return filepath.Join(repoRoot, ".mcp.json"), nil
		case "agent":
			return filepath.Join(repoRoot, ".claude", "agents"), nil
		case "command":
			return filepath.Join(repoRoot, ".claude", "commands"), nil
		case "skill":
			return filepath.Join(repoRoot, ".claude", "skills"), nil
		default:
			return "", &DestError{Code: 1, Msg: fmt.Sprintf("scope/tipo inválido: %q/%q", scope, typ)}
		}
	default:
		return "", &DestError{Code: 1, Msg: fmt.Sprintf("scope/tipo inválido: %q/%q", scope, typ)}
	}
}

// --- bloque de reglas (tipo 'rule') --------------------------------------

// instrBlockEnsure garantiza que file existe y contiene el bloque gestionado
// (idempotente). Si el archivo ya tenía contenido se separa el bloque con una
// línea en blanco (espeja ccp_instruct_block_ensure).
func instrBlockEnsure(file string) error {
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return fmt.Errorf("no se pudo crear directorio de %s: %w", file, err)
	}
	data, err := os.ReadFile(file)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("no se pudo leer %s: %w", file, err)
	}
	if bytes.Contains(data, []byte(instrBegin)) {
		return nil
	}
	var buf bytes.Buffer
	buf.Write(data)
	if len(data) > 0 {
		buf.WriteByte('\n')
	}
	fmt.Fprintf(&buf, "%s\n%s\n", instrBegin, instrEnd)
	return os.WriteFile(file, buf.Bytes(), 0o644)
}

// InstructRuleList devuelve las instrucciones (texto sin "- "), en orden, del
// bloque gestionado de file. Archivo inexistente => lista vacía sin error.
func InstructRuleList(file string) ([]string, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("no se pudo leer %s: %w", file, err)
	}
	var out []string
	inBlock := false
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		switch {
		case line == instrBegin:
			inBlock = true
		case line == instrEnd:
			inBlock = false
		case inBlock && strings.HasPrefix(line, "- "):
			out = append(out, strings.TrimPrefix(line, "- "))
		}
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("no se pudo escanear %s: %w", file, err)
	}
	return out, nil
}

// InstructRuleAdd añade una instrucción (bullet) al bloque de file si no es un
// duplicado exacto. Devuelve (added=false, nil) si ya existía (no se duplica).
func InstructRuleAdd(file, text string) (added bool, err error) {
	if err := instrBlockEnsure(file); err != nil {
		return false, err
	}
	existing, err := InstructRuleList(file)
	if err != nil {
		return false, err
	}
	for _, e := range existing {
		if e == text {
			return false, nil
		}
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return false, fmt.Errorf("no se pudo leer %s: %w", file, err)
	}
	var buf bytes.Buffer
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if line == instrEnd {
			fmt.Fprintf(&buf, "- %s\n", text)
		}
		buf.WriteString(line)
		buf.WriteByte('\n')
	}
	if err := sc.Err(); err != nil {
		return false, fmt.Errorf("no se pudo escanear %s: %w", file, err)
	}
	if err := os.WriteFile(file, buf.Bytes(), 0o644); err != nil {
		return false, fmt.Errorf("no se pudo escribir %s: %w", file, err)
	}
	return true, nil
}

// InstructRuleRm borra la instrucción idx (1-based) del bloque de file.
// Devuelve error si idx está fuera de rango.
func InstructRuleRm(file string, idx int) error {
	rules, err := InstructRuleList(file)
	if err != nil {
		return err
	}
	if idx < 1 || idx > len(rules) {
		return fmt.Errorf("índice fuera de rango: %d", idx)
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return fmt.Errorf("no se pudo leer %s: %w", file, err)
	}
	var buf bytes.Buffer
	inBlock := false
	count := 0
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		switch {
		case line == instrBegin:
			inBlock = true
		case line == instrEnd:
			inBlock = false
		case inBlock && strings.HasPrefix(line, "- "):
			count++
			if count == idx {
				continue // saltar la línea a borrar
			}
		}
		buf.WriteString(line)
		buf.WriteByte('\n')
	}
	if err := sc.Err(); err != nil {
		return fmt.Errorf("no se pudo escanear %s: %w", file, err)
	}
	return os.WriteFile(file, buf.Bytes(), 0o644)
}

// --- artefactos JSON (hook/mcp) ------------------------------------------

// instrJSONMerge hace deep-merge de snippet sobre el archivo file (snippet
// gana), reutilizando MergeJSON (la superficie compartida con #7). Crea el
// archivo si falta o si su contenido no es JSON válido (parte de "{}"). Valida
// el snippet antes de tocar disco.
func instrJSONMerge(file string, snippet []byte) error {
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return fmt.Errorf("no se pudo crear directorio de %s: %w", file, err)
	}
	var base []byte
	if b, err := os.ReadFile(file); err == nil && jsonValid(b) {
		base = b
	}
	merged, err := MergeJSON(base, snippet)
	if err != nil {
		return err
	}
	if err := os.WriteFile(file, merged, 0o644); err != nil {
		return fmt.Errorf("no se pudo escribir %s: %w", file, err)
	}
	return nil
}

// instrJSONRmMCP borra un server MCP por nombre del archivo file. No-op (sin
// error) si el archivo no existe o no tiene la clave mcpServers.
func instrJSONRmMCP(file, name string) error {
	data, err := os.ReadFile(file)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("no se pudo leer %s: %w", file, err)
	}
	v, err := unmarshalJSONValue(data)
	if err != nil {
		return fmt.Errorf("JSON inválido en %s: %w", file, err)
	}
	obj, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	if servers, ok := obj["mcpServers"].(map[string]any); ok {
		delete(servers, name)
	}
	out, err := marshalIndent(obj)
	if err != nil {
		return err
	}
	return os.WriteFile(file, out, 0o644)
}

// jsonValid es un helper local (evita exponer encoding/json fuera de cfg.go).
func jsonValid(b []byte) bool { return CfgValidateBytes(b) == nil }
