package cli

// serve_config_test.go — los métodos del editor de configuración en `ccp serve`
// (spec 2026-09-18 §7, C4). Todo en temporales: CCP_HOME, el ~/.claude de
// origen, el HOME y la ventana de Desktop.

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// serveCfgEnv es serveEnv más el directorio de managed-settings, que el
// inventario mira y que no puede ser el de la máquina real.
func serveCfgEnv(t *testing.T) string {
	t.Helper()
	home := serveEnv(t)
	t.Setenv("CCP_MANAGED_DIR", t.TempDir())
	return home
}

type cfgItemsReply struct {
	Layer map[string]string `json:"layer"`
	Items []struct {
		Ref      map[string]any `json:"ref"`
		Name     string         `json:"name"`
		Format   string         `json:"format"`
		Editable bool           `json:"editable"`
	} `json:"items"`
	Probes []map[string]any `json:"probes"`
}

// El editor completo sobre un agente del overlay de un perfil: se crea, aparece
// en la lista con su tipo, se relee y se borra. El put dice a quién regeneró,
// que es lo que la GUI enseña después de escribir.
func TestServeConfigItemCicloCompleto(t *testing.T) {
	serveCfgEnv(t)
	layer := map[string]string{"level": "profile", "name": "work"}
	ref := map[string]any{"layer": layer, "type": "agents", "name": "revisor"}
	_, got := serveRun(t,
		req(1, "config.item.put", map[string]any{"ref": ref,
			"value": map[string]any{"format": "text", "text": "# Revisor\n"}}),
	)
	var wr struct {
		OK          bool     `json:"ok"`
		File        string   `json:"file"`
		Regenerated []string `json:"regenerated"`
	}
	mustResult(t, got["1"], &wr)
	if !wr.OK || !strings.HasSuffix(wr.File, "agents/revisor.md") {
		t.Fatalf("put = %+v", wr)
	}
	if len(wr.Regenerated) != 1 || wr.Regenerated[0] != "work" {
		t.Errorf("el overlay de un perfil regenera solo el suyo: %v", wr.Regenerated)
	}
	if b, err := os.ReadFile(wr.File); err != nil || string(b) != "# Revisor\n" {
		t.Fatalf("archivo = %q %v", b, err)
	}

	_, got = serveRun(t,
		req(2, "config.items", map[string]any{"layer": layer}),
		req(3, "config.item.get", map[string]any{"ref": ref}),
	)
	var list cfgItemsReply
	mustResult(t, got["2"], &list)
	if list.Items == nil || list.Probes == nil {
		t.Fatalf("items y probes son siempre arrays: %s", got["2"].Result)
	}
	found := false
	for _, it := range list.Items {
		if it.Name == "revisor" && it.Ref["type"] == "agents" {
			found = true
			if !it.Editable || it.Format != "text" {
				t.Errorf("el agente del overlay se edita: %+v", it)
			}
		}
	}
	if !found {
		t.Fatalf("config.items no trae el agente recién creado: %s", got["2"].Result)
	}
	var val struct {
		Format string `json:"format"`
		Text   string `json:"text"`
		Exists bool   `json:"exists"`
	}
	mustResult(t, got["3"], &val)
	if !val.Exists || val.Text != "# Revisor\n" {
		t.Fatalf("get = %+v", val)
	}

	_, got = serveRun(t, req(4, "config.item.delete", map[string]any{"ref": ref}))
	mustResult(t, got["4"], &wr)
	if _, err := os.Stat(wr.File); !os.IsNotExist(err) {
		t.Fatalf("tras delete el archivo sigue: %v", err)
	}
}

// Una capa que no existe no es un fallo del servidor: es un parámetro malo, y
// el mensaje sale de core para que diga lo mismo que el CLI.
func TestServeConfigItemsCapaDesconocida(t *testing.T) {
	serveCfgEnv(t)
	_, got := serveRun(t, req(1, "config.items", map[string]any{
		"layer": map[string]string{"level": "vecino"}}))
	if got["1"].Error == nil || !strings.Contains(got["1"].Error.Message, "vecino") {
		t.Fatalf("capa desconocida: %+v", got["1"])
	}
}

// config.effective es el conmutador «Efectivo» de P-20 y es exactamente lo que
// profiles.effective ya devolvía: se registra el nombre nuevo, no una segunda
// implementación que se desincronice.
func TestServeConfigEffective(t *testing.T) {
	serveCfgEnv(t)
	_, got := serveRun(t,
		req(1, "config.effective", map[string]any{"name": "work"}),
		req(2, "profiles.effective", map[string]any{"name": "work"}),
	)
	var a, b json.RawMessage
	mustResult(t, got["1"], &a)
	mustResult(t, got["2"], &b)
	if string(a) != string(b) {
		t.Fatalf("config.effective ≠ profiles.effective:\n%s\n%s", a, b)
	}
}

type mcpListRow struct {
	Scope     string   `json:"scope"`
	Name      string   `json:"name"`
	Type      string   `json:"type"`
	Targets   []string `json:"targets"`
	AppliesTo []string `json:"applies_to"`
	Disabled  bool     `json:"disabled"`
}

func mcpList(t *testing.T, r serveReply) []mcpListRow {
	t.Helper()
	var rows []mcpListRow
	mustResult(t, r, &rows)
	if rows == nil {
		t.Fatalf("mcp.list devolvió null, no un array: %s", r.Result)
	}
	return rows
}

// El editor de MCP entero por serve: alta en la global, destinos, apagado en un
// perfil y baja. Es el mismo motor que `ccp mcp`, así que lo que se comprueba
// aquí es que la GUI llega a todo sin una segunda implementación.
func TestServeMCPCicloCompleto(t *testing.T) {
	serveCfgEnv(t)
	global := map[string]string{"level": "global"}
	perfil := map[string]string{"level": "profile", "name": "work"}
	_, got := serveRun(t, req(1, "mcp.put", map[string]any{"layer": global, "name": "fs",
		"def": map[string]any{"command": "npx", "args": []string{"-y", "@mcp/fs"}}}))
	var wr struct {
		OK      bool     `json:"ok"`
		File    string   `json:"file"`
		Restart []string `json:"restart_pending"`
	}
	mustResult(t, got["1"], &wr)
	if !wr.OK || wr.File != os.Getenv("CCP_CLAUDE_SRC")+".json" {
		t.Fatalf("la capa global declara en ~/.claude.json: %+v", wr)
	}
	if wr.Restart == nil {
		t.Errorf("restart_pending es siempre un array: %s", got["1"].Result)
	}

	_, got = serveRun(t, req(2, "mcp.list", map[string]any{"layer": global}))
	rows := mcpList(t, got["2"])
	if len(rows) != 1 || rows[0].Name != "fs" || rows[0].Type != "stdio" {
		t.Fatalf("mcp.list global = %+v", rows)
	}
	if len(rows[0].Targets) != 2 {
		t.Errorf("sin entrada en ccp.yaml los destinos son los dos: %v", rows[0].Targets)
	}

	_, got = serveRun(t, req(3, "mcp.setTargets", map[string]any{"name": "fs", "targets": []string{"desktop"}}))
	mustResult(t, got["3"], &wr)
	_, got = serveRun(t, req(4, "mcp.list", map[string]any{"layer": global}))
	rows = mcpList(t, got["4"])
	if len(rows) != 1 || len(rows[0].Targets) != 1 || rows[0].Targets[0] != "desktop" {
		t.Fatalf("tras setTargets = %+v", rows)
	}

	_, got = serveRun(t, req(5, "mcp.disable", map[string]any{"profile": "work", "name": "fs"}))
	mustResult(t, got["5"], &wr)
	_, got = serveRun(t, req(6, "mcp.list", map[string]any{"layer": perfil}))
	rows = mcpList(t, got["6"])
	if len(rows) != 1 || !rows[0].Disabled {
		t.Fatalf("apagado en work = %+v", rows)
	}
	if len(rows[0].AppliesTo) != 0 {
		t.Errorf("un servidor apagado no aplica en ningún sitio: %v", rows[0].AppliesTo)
	}

	_, got = serveRun(t, req(7, "mcp.disable", map[string]any{"profile": "work", "name": "fs", "enabled": true}))
	mustResult(t, got["7"], &wr)
	_, got = serveRun(t, req(8, "mcp.list", map[string]any{"layer": perfil}))
	if rows = mcpList(t, got["8"]); len(rows) != 1 || rows[0].Disabled {
		t.Fatalf("vuelta a encender = %+v", rows)
	}

	_, got = serveRun(t, req(9, "mcp.delete", map[string]any{"layer": global, "name": "fs"}))
	mustResult(t, got["9"], &wr)
	_, got = serveRun(t, req(10, "mcp.list", map[string]any{"layer": global}))
	if rows = mcpList(t, got["10"]); len(rows) != 0 {
		t.Fatalf("tras mcp.delete = %+v", rows)
	}
}

// La ventana de Desktop es un DESTINO de la proyección: escribir ahí lo borraría
// el siguiente sync. La barrera vive en core (C2); aquí se comprueba que serve
// la deja hablar en vez de tragarse el error.
func TestServeMCPNoEscribeEnLaVentana(t *testing.T) {
	serveCfgEnv(t)
	_, got := serveRun(t, req(1, "mcp.put", map[string]any{
		"layer": map[string]string{"level": "desktop", "name": "work"}, "name": "fs",
		"def": map[string]any{"command": "npx"}}))
	if e := got["1"].Error; e == nil || !strings.Contains(e.Message, "destino") {
		t.Fatalf("la capa desktop no declara MCP y tiene que decir dónde va: %+v", got["1"])
	}
}

// Las escrituras van serializadas por writeMu; las lecturas, no. Se afirma por
// nombre porque un registro mal marcado no da síntoma hasta que dos escrituras
// coinciden y una pisa a la otra.
func TestServeEditorMarcaLasEscrituras(t *testing.T) {
	reg := serveRegistry()
	for _, m := range []string{"config.items", "config.item.get", "config.effective", "mcp.list"} {
		if e, ok := reg[m]; !ok || e.write {
			t.Errorf("%s debe ser lectura (ok=%v write=%v)", m, ok, e.write)
		}
	}
	for _, m := range []string{"config.item.put", "config.item.delete", "mcp.put", "mcp.delete",
		"mcp.setTargets", "mcp.disable"} {
		if e, ok := reg[m]; !ok || !e.write {
			t.Errorf("%s debe ser escritura (ok=%v write=%v)", m, ok, e.write)
		}
	}
}
