package core

// instruct_cmd.go — orquestación de `ccp instruct add|list|rm|dest|record`.
// Combina InstructDest + el bloque de reglas + el merge JSON (hook/mcp) + el
// manifiesto, devolviendo resultados estructurados para que internal/cli formatee
// la salida en español. Port de cmd_instruct/_instruct_* del bash (bin/ccp).

import (
	"fmt"
	"os"
	"strings"
)

// InstructCtx agrupa la resolución del contexto (perfil activo / fuente global /
// raíz del repo) que el CLI calcula una vez y pasa a cada operación. Todo
// explícito: los tests usan t.TempDir() y nunca leen el entorno real.
type InstructCtx struct {
	Home          string // ~/.config/ccp (o CCP_HOME)
	Src           string // fuente global (~/.claude o CCP_CLAUDE_SRC)
	RepoRoot      string // raíz del repo para scope project ("" = no hay)
	ActiveProfile string // CCP_PROFILE ("default" o "" si ninguno)
}

// manifestProfile devuelve el valor de profile a registrar/buscar para un scope:
// el perfil activo solo en scope profile, "-" en cualquier otro.
func (c InstructCtx) manifestProfile(scope string) string {
	if scope == "profile" {
		return c.ActiveProfile
	}
	return "-"
}

// AddResult describe el efecto de InstructAdd para que el CLI emita el mensaje.
type AddResult struct {
	Type        string
	Scope       string
	Dest        string // ruta destino tocada
	Name        string // id/nombre para hook/mcp ("" para rule)
	Duplicate   bool   // rule ya existía (no se duplicó)
	RegenProfil bool   // se regeneró el cc-home (hook en scope profile)
}

// InstructAdd persiste un artefacto rule/hook/mcp en su estructura nativa al
// scope dado, registrando hook/mcp en el manifiesto. Para hook/mcp el texto es
// "nombre={json}". rule/hook/mcp son los únicos tipos que ccp escribe por sí
// mismo; agent/command/skill los escribe Claude y se registran vía InstructRecord.
func InstructAdd(ctx InstructCtx, scope, typ, text string) (*AddResult, error) {
	if scope == "" || typ == "" || text == "" {
		return nil, fmt.Errorf("Uso: ccp instruct add <scope> <type> <texto>")
	}
	dest, err := InstructDest(scope, typ, ctx.Home, ctx.ActiveProfile, ctx.Src, ctx.RepoRoot)
	if err != nil {
		return nil, err
	}
	res := &AddResult{Type: typ, Scope: scope, Dest: dest}

	switch typ {
	case "rule":
		added, err := InstructRuleAdd(dest, text)
		if err != nil {
			return nil, err
		}
		res.Duplicate = !added
		return res, nil

	case "mcp":
		name, json, ok := splitNameJSON(text)
		if !ok {
			return nil, fmt.Errorf("Uso: mcp -> nombre={json}")
		}
		if err := CfgValidateBytes([]byte(json)); err != nil {
			return nil, fmt.Errorf("JSON inválido para el server '%s'", name)
		}
		snippet := fmt.Sprintf(`{"mcpServers":{%s:%s}}`, jsonString(name), json)
		if err := instrJSONMerge(dest, []byte(snippet)); err != nil {
			return nil, err
		}
		if err := manifestAdd(ctx.Home, ctx.RepoRoot, manifestEntry{
			Scope: scope, Profile: ctx.manifestProfile(scope), Type: "mcp", Ref: name, Desc: "mcp " + name,
		}); err != nil {
			return nil, err
		}
		res.Name = name
		return res, nil

	case "hook":
		name, json, ok := splitNameJSON(text)
		if !ok {
			return nil, fmt.Errorf("Uso: hook -> id={json}")
		}
		if err := CfgValidateBytes([]byte(json)); err != nil {
			return nil, fmt.Errorf("JSON inválido para el hook '%s'", name)
		}
		if err := instrJSONMerge(dest, []byte(json)); err != nil {
			return nil, err
		}
		if err := manifestAdd(ctx.Home, ctx.RepoRoot, manifestEntry{
			Scope: scope, Profile: ctx.manifestProfile(scope), Type: "hook", Ref: name, Desc: "hook " + name,
		}); err != nil {
			return nil, err
		}
		if scope == "profile" {
			if err := CfgRegenerate(ctx.Home, ctx.ActiveProfile, ctx.Src); err != nil {
				return nil, err
			}
			res.RegenProfil = true
		}
		res.Name = name
		return res, nil

	default:
		return nil, fmt.Errorf("tipo '%s' no se escribe con 'add' (usa 'record' para agent/command/skill)", typ)
	}
}

// InstructRecord registra en el manifiesto un artefacto que Claude ya escribió
// (agent/command/skill típicamente, pero acepta cualquier tipo válido). Valida
// que (scope,type) tenga destino antes de registrar.
func InstructRecord(ctx InstructCtx, scope, typ, ref, desc string) error {
	if scope == "" || typ == "" || ref == "" {
		return fmt.Errorf("Uso: ccp instruct record <scope> <type> <ref> <desc>")
	}
	if _, err := InstructDest(scope, typ, ctx.Home, ctx.ActiveProfile, ctx.Src, ctx.RepoRoot); err != nil {
		return err
	}
	return manifestAdd(ctx.Home, ctx.RepoRoot, manifestEntry{
		Scope: scope, Profile: ctx.manifestProfile(scope), Type: typ, Ref: ref, Desc: desc,
	})
}

// InstructDestCmd resuelve la ruta destino para (scope,type) — la usan los
// comandos /ccp:* para saber dónde escribir agent/command/skill.
func InstructDestCmd(ctx InstructCtx, scope, typ string) (string, error) {
	return InstructDest(scope, typ, ctx.Home, ctx.ActiveProfile, ctx.Src, ctx.RepoRoot)
}

// ListRow es una fila renderizable de `instruct list`: las reglas van primero
// (numeradas), luego los artefactos del manifiesto. Index es 1-based global.
type ListRow struct {
	Index int
	Type  string // "rule" para instrucciones; el tipo del manifiesto si no
	Text  string // texto de la regla, o "desc — ref" para artefactos
}

// InstructList enumera reglas + manifiesto del scope, en el orden de borrado
// (las reglas ocupan los índices bajos). Espeja _instruct_list del bash.
func InstructList(ctx InstructCtx, scope string) ([]ListRow, error) {
	if scope == "" {
		return nil, fmt.Errorf("Uso: ccp instruct list <scope>")
	}
	rfile, err := InstructDest(scope, "rule", ctx.Home, ctx.ActiveProfile, ctx.Src, ctx.RepoRoot)
	if err != nil {
		return nil, err
	}
	rules, err := InstructRuleList(rfile)
	if err != nil {
		return nil, err
	}
	var rows []ListRow
	i := 0
	for _, r := range rules {
		i++
		rows = append(rows, ListRow{Index: i, Type: "rule", Text: r})
	}
	arts, err := manifestList(ctx.Home, ctx.RepoRoot, scope, ctx.manifestProfile(scope))
	if err != nil {
		return nil, err
	}
	for _, a := range arts {
		i++
		rows = append(rows, ListRow{Index: i, Type: a.Type, Text: fmt.Sprintf("%s — %s", a.Desc, a.Ref)})
	}
	return rows, nil
}

// RmResult describe lo borrado por InstructRm para que el CLI emita el mensaje
// y las advertencias (hook deja su entrada JSON; mcp se borra del JSON).
type RmResult struct {
	Scope   string
	Type    string
	Ref     string // texto de la regla, o ref del artefacto
	WasRule bool
}

// InstructRm borra el idx-ésimo (1-based global) item del scope: las reglas
// ocupan los índices bajos, luego el manifiesto. Para artefactos del manifiesto
// también limpia su estructura nativa cuando aplica (agent/command/skill borran
// el archivo/dir; mcp se quita del JSON; hook deja su entrada — sin id estable).
func InstructRm(ctx InstructCtx, scope string, idx int) (*RmResult, error) {
	if scope == "" || idx < 1 {
		return nil, fmt.Errorf("Uso: ccp instruct rm <scope> <index>")
	}
	rfile, err := InstructDest(scope, "rule", ctx.Home, ctx.ActiveProfile, ctx.Src, ctx.RepoRoot)
	if err != nil {
		return nil, err
	}
	rules, err := InstructRuleList(rfile)
	if err != nil {
		return nil, err
	}
	nRules := len(rules)
	if idx <= nRules {
		ref := rules[idx-1]
		if err := InstructRuleRm(rfile, idx); err != nil {
			return nil, err
		}
		return &RmResult{Scope: scope, Type: "rule", Ref: ref, WasRule: true}, nil
	}

	removed, err := manifestRm(ctx.Home, ctx.RepoRoot, scope, ctx.manifestProfile(scope), idx-nRules)
	if err != nil {
		return nil, err
	}
	res := &RmResult{Scope: scope, Type: removed.Type, Ref: removed.Ref}

	switch removed.Type {
	case "agent", "command":
		if isRegularFile(removed.Ref) {
			_ = os.Remove(removed.Ref)
		}
	case "skill":
		// Guard: nunca borrar "/" o vacío.
		if removed.Ref != "" && removed.Ref != "/" && isDir(removed.Ref) {
			_ = os.RemoveAll(removed.Ref)
		}
	case "mcp":
		if mfile, derr := InstructDest(scope, "mcp", ctx.Home, ctx.ActiveProfile, ctx.Src, ctx.RepoRoot); derr == nil {
			_ = instrJSONRmMCP(mfile, removed.Ref)
		}
	case "hook":
		// El borrado de la entrada JSON del hook NO es automático (los hooks
		// viven en arrays sin id estable); solo se quitó del manifiesto.
	}
	return res, nil
}

// --- helpers ---

// splitNameJSON parte "nombre={json}" en (nombre, json). ok=false si no hay '='
// o falta alguno de los dos lados.
func splitNameJSON(text string) (name, json string, ok bool) {
	i := strings.IndexByte(text, '=')
	if i <= 0 || i == len(text)-1 {
		return "", "", false
	}
	return text[:i], text[i+1:], true
}

// jsonString cita un string como literal JSON (para construir el snippet de mcp
// sin reimportar encoding/json aquí; reutiliza la ruta de marshalIndent).
func jsonString(s string) string {
	out, err := marshalIndent(s)
	if err != nil {
		return `""`
	}
	return strings.TrimRight(string(out), "\n")
}
