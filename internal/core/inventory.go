package core

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Inventario de la configuración de Claude en la máquina (spec §5.1): la de
// ccp y la que ccp no gestiona. Solo lectura y con las raíces inyectadas, igual
// que desktop_audit, para montar una máquina falsa en un test. Hereda la regla
// del doctor (ADR 0009): una fuente que no se puede leer produce `unknown`,
// jamás «vacío», porque un inventario que calla lo que no pudo mirar convierte
// una duda en la certeza de que no hay nada que adoptar.
//
// La función se llama BuildInventory y no Inventory porque en Go un tipo y una
// función no pueden compartir nombre, y el tipo es el contrato de `scan --json`.

// InventoryRoots son las raíces que recorre el inventario. Nada se saca de
// os.Getenv ni de os.UserHomeDir: lo resuelve el front-end.
type InventoryRoots struct {
	Home    string `json:"home"`
	CCPHome string `json:"ccp_home"`
	// ClaudeSrc es ~/.claude (o CCP_CLAUDE_SRC); el ~/.claude.json es
	// ClaudeSrc + ".json", igual que InstructDest.
	ClaudeSrc string `json:"claude_src"`
	// DesktopDefaultDataDir es el data dir de la ventana `default` de Desktop.
	DesktopDefaultDataDir string                       `json:"desktop_default_data_dir"`
	ManagedDir            string                       `json:"managed_dir"`
	RCFiles               []string                     `json:"rc_files"`
	LookPath              func(string) (string, error) `json:"-"`
	// ExtraProjects son rutas de proyecto que hay que recorrer aunque nadie
	// las haya «descubierto»: sin regla y sin entrada en ningún .claude.json.
	// Una carpeta que el usuario nombra (la capa que pide `ccp mcp list
	// --scope project:<ruta>`, el campo de ruta libre de la GUI) es un dato
	// suyo, no un hallazgo; sin esto la pantalla salía vacía justo después de
	// escribir allí.
	ExtraProjects []string `json:"extra_projects,omitempty"`
}

// InvScope dice a qué capa pertenece un elemento.
type InvScope struct {
	Level string `json:"level"` // global|profile|project|desktop|managed|plugin|account
	Name  string `json:"name,omitempty"`
}

// InvItem es un elemento del inventario (spec §5.1).
type InvItem struct {
	Kind   string   `json:"kind"`
	Scope  InvScope `json:"scope"`
	Name   string   `json:"name"`
	Source string   `json:"source"`
	// Key es la clave JSON dentro de Source cuando el elemento es una clave de
	// un archivo (`env.TOKEN`, `permissions.allow`); vacía si es el archivo.
	Key       string   `json:"key,omitempty"`
	Class     string   `json:"class"`
	Managed   bool     `json:"managed"`
	Editable  bool     `json:"editable"`
	Why       string   `json:"why,omitempty"`
	AppliesTo []string `json:"applies_to"`
	// Hash es un sha256 corto del contenido normalizado. En lo que lleva
	// secretos es el de la FORMA (la ruta), nunca el del valor.
	Hash    string   `json:"hash"`
	Secrets []string `json:"secrets,omitempty"`
	// Enabled marca un plugin presente en enabledPlugins.
	Enabled bool `json:"enabled,omitempty"`
	// Missing es el `command` de un MCP stdio que no resuelve (absoluto que no
	// existe o fuera del PATH): alimenta los pendientes del plan de adopción.
	Missing string `json:"missing,omitempty"`
	// Project es la identidad portable del proyecto al que pertenece el item
	// (spec §11): la misma clave que usan los snapshots, para que un proyecto
	// clonado en otra ruta se reconozca por su remoto.
	Project *InvProject `json:"project,omitempty"`
	// SecretHash sirve para comparar capas en memoria (la Task 4) y no sale
	// nunca en JSON: el hash de un token corto es un oráculo para adivinarlo.
	SecretHash string `json:"-"`
}

// InvProject identifica un proyecto: su ruta en esta máquina, su clave
// portable (projectKey) y el remoto origin tal cual está en .git/config.
type InvProject struct {
	Path   string `json:"path"`
	Key    string `json:"key"`
	Remote string `json:"remote,omitempty"`
}

// InvProbe es el resultado de leer una fuente.
type InvProbe struct {
	Source string `json:"source"`
	Status string `json:"status"` // ok|missing|unknown
	Err    string `json:"error,omitempty"`
}

// Inventory es el resultado entero. Items y Probes nunca son nil.
type Inventory struct {
	Items  []InvItem  `json:"items"`
	Probes []InvProbe `json:"probes"`
}

// Clases (§2) y destinos (ADR 0016).
const (
	InvClassAuthored = "authored"

	InvAppliesCLI         = "cli"
	InvAppliesDesktopCode = "desktop-code"
	InvAppliesDesktopChat = "desktop-chat"
)

// invCodeTargets: lo del global y lo de un cc-home llega a la CLI y a la
// pestaña Code de Desktop (ADR 0016 M1); nada de settings llega al chat.
func invCodeTargets() []string { return []string{InvAppliesCLI, InvAppliesDesktopCode} }

// invHash es el sha256 corto de un contenido ya normalizado.
func invHash(b []byte) string {
	sum := sha256.Sum256(b)
	return fmt.Sprintf("%x", sum[:8])
}

// invHashJSON normaliza por re-serialización: json.Marshal ordena las claves
// de los mapas, así que dos archivos con el mismo contenido y otro orden o
// sangrado dan el mismo hash (lo que la Task 4 usa para ver duplicados).
func invHashJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return invHash([]byte(fmt.Sprint(v)))
	}
	return invHash(b)
}

// invHashText normaliza finales de línea y espacio final.
func invHashText(b []byte) string {
	s := strings.ReplaceAll(string(b), "\r\n", "\n")
	return invHash([]byte(strings.TrimRight(s, " \t\n")))
}

// invWalker acumula el recorrido. Cada fuente deja su sonda exactamente una vez.
type invWalker struct {
	inv      Inventory
	lookPath func(string) (string, error)
	// plugins son los instalados según installed_plugins.json, con su ruta en
	// disco y si están activos: de ahí salen los MCP que trae cada uno.
	plugins []invPlugin
	// projects son las rutas de proyecto conocidas, en el orden en que
	// aparecen (reglas y claves `projects` de cada .claude.json), con
	// repeticiones: invProjectPaths las deduplica.
	projects []string
	// ids cachea la identidad de cada proyecto: leer .git/config una vez.
	ids map[string]*InvProject
}

type invPlugin struct {
	id, path string
	enabled  bool
}

func (w *invWalker) probe(src, status string, err error) {
	p := InvProbe{Source: src, Status: status}
	if err != nil {
		p.Err = err.Error()
	}
	w.inv.Probes = append(w.inv.Probes, p)
}

func (w *invWalker) add(it InvItem) {
	if it.Class == "" {
		it.Class = InvClassAuthored
	}
	if it.AppliesTo == nil {
		it.AppliesTo = invCodeTargets()
	}
	w.inv.Items = append(w.inv.Items, it)
}

// readFile lee una fuente y deja su sonda: missing si no existe (no es
// error), unknown si existe y no se puede leer.
func (w *invWalker) readFile(path string) ([]byte, bool) {
	b, err := os.ReadFile(path)
	switch {
	case err == nil:
		w.probe(path, "ok", nil)
		return b, true
	case errors.Is(err, fs.ErrNotExist):
		w.probe(path, "missing", nil)
	default:
		w.probe(path, "unknown", err)
	}
	return nil, false
}

// readJSON es readFile + JSON válido; lo que no lo es queda unknown y no
// produce ningún item. UseNumber: el hash no debe depender de float64.
func (w *invWalker) readJSON(path string) (any, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			w.probe(path, "missing", nil)
		} else {
			w.probe(path, "unknown", err)
		}
		return nil, false
	}
	var v any
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	if err := dec.Decode(&v); err != nil {
		w.probe(path, "unknown", fmt.Errorf("JSON inválido: %w", err))
		return nil, false
	}
	w.probe(path, "ok", nil)
	return v, true
}

// readJSONObject exige además un objeto. La sonda la deja readJSON; si no es
// un objeto se corrige a unknown para que siga habiendo una sola por fuente.
func (w *invWalker) readJSONObject(path string) (map[string]any, bool) {
	v, ok := w.readJSON(path)
	if !ok {
		return nil, false
	}
	m, isMap := v.(map[string]any)
	if !isMap {
		last := &w.inv.Probes[len(w.inv.Probes)-1]
		last.Status, last.Err = "unknown", "no es un objeto JSON"
		return nil, false
	}
	return m, true
}

// isDir sigue los enlaces: seedCCHome siembra symlinks y el espejo de Desktop
// directorios reales, y los dos son el mismo árbol para el inventario.
func (w *invWalker) isDir(e fs.DirEntry, p string) bool {
	if e.IsDir() {
		return true
	}
	if e.Type()&fs.ModeSymlink == 0 {
		return false
	}
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}

// readDir lista un directorio con su sonda. Un directorio que falta es
// missing; uno que no se puede listar, unknown.
func (w *invWalker) readDir(dir string) ([]fs.DirEntry, bool) {
	es, err := os.ReadDir(dir)
	switch {
	case err == nil:
		w.probe(dir, "ok", nil)
		return es, true
	case errors.Is(err, fs.ErrNotExist):
		w.probe(dir, "missing", nil)
	default:
		w.probe(dir, "unknown", err)
	}
	return nil, false
}

func invSortedKeys(m map[string]any) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

// invSettings clasifica un settings (global, overlay o cc-home) clave a clave.
// Es la misma partición que el editor y la vista de perfil: env y permisos se
// ven entrada a entrada porque es a ese grano como se adoptan y se comparan.
func (w *invWalker) invSettings(src string, sc InvScope, managed bool, m map[string]any) {
	base := InvItem{Scope: sc, Source: src, Managed: managed, Editable: true}
	for _, k := range invSortedKeys(m) {
		v := m[k]
		switch k {
		case "env":
			env, ok := v.(map[string]any)
			if !ok {
				break
			}
			for _, name := range invSortedKeys(env) {
				it := base
				it.Kind, it.Name, it.Key = "env", name, "env."+name
				it.Secrets = []string{it.Key}
				// La forma y no el valor: el Hash sale en `scan --json`.
				it.Hash = invHash([]byte(it.Key))
				it.SecretHash = invHashJSON(env[name])
				w.add(it)
			}
			continue
		case "permissions":
			perms, ok := v.(map[string]any)
			if !ok {
				break
			}
			for _, pk := range invSortedKeys(perms) {
				list, isList := perms[pk].([]any)
				if !isList || (pk != "allow" && pk != "deny" && pk != "ask") {
					it := base
					it.Kind, it.Name, it.Key = "settings-key", "permissions."+pk, "permissions."+pk
					it.Hash = invHashJSON(perms[pk])
					w.add(it)
					continue
				}
				for _, e := range list {
					it := base
					it.Kind, it.Name, it.Key = "permission", pk+":"+fmt.Sprint(e), "permissions."+pk
					it.Hash = invHashJSON(it.Name)
					w.add(it)
				}
			}
			continue
		case "hooks":
			hooks, ok := v.(map[string]any)
			if !ok {
				break
			}
			for _, ev := range invSortedKeys(hooks) {
				it := base
				it.Kind, it.Name, it.Key = "hook", ev, "hooks."+ev
				it.Hash = invHashJSON(hooks[ev])
				w.add(it)
			}
			continue
		}
		it := base
		it.Kind, it.Name, it.Key = invSettingsKind(k), k, k
		it.Hash = invHashJSON(v)
		w.add(it)
	}
}

// invSettingsKind es el kind de una clave de primer nivel que se ve entera.
func invSettingsKind(k string) string {
	switch k {
	case "statusLine":
		return "statusline"
	case "outputStyle":
		return "output-style"
	}
	return "settings-key"
}

// BuildInventory recorre las raíces y devuelve lo que encontró. No escribe
// nada y no falla: lo que no pudo leer queda en Probes como unknown.
func BuildInventory(r InventoryRoots) Inventory {
	w := &invWalker{inv: Inventory{Items: []InvItem{}, Probes: []InvProbe{}}, lookPath: r.LookPath}
	// Una raíz vacía no se recorre: Load("") leería ./ccp.yaml del cwd.
	var cfg *Config
	if r.CCPHome != "" {
		cfg = w.walkCCP(r.CCPHome)
	}
	if r.ClaudeSrc != "" {
		w.walkClaudeGlobal(r.ClaudeSrc)
	}
	w.walkMCP(r, cfg)
	// Después de walkMCP: es quien junta los proyectos conocidos.
	owned := invOwnedConfigDirs(r, cfg)
	w.walkProjects(owned)
	w.walkConfigDirs(r, owned)
	return w.inv
}

// walkCCP: los perfiles, las reglas y el overlay de cada perfil. Todo lo
// escribió ccp (Managed), aunque el overlay lo edite el usuario.
func (w *invWalker) walkCCP(home string) *Config {
	yml := yamlPath(home)
	if _, err := os.Stat(yml); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			w.probe(yml, "missing", nil)
		} else {
			w.probe(yml, "unknown", err)
		}
		return nil
	}
	cfg, err := Load(home)
	if err != nil {
		w.probe(yml, "unknown", err)
		return nil
	}
	w.probe(yml, "ok", nil)

	names := make([]string, 0, len(cfg.Profiles))
	for n := range cfg.Profiles {
		names = append(names, n)
	}
	sort.Strings(names)
	// default es implícito: existe aunque ccp.yaml no lo nombre.
	w.add(InvItem{Kind: "profile", Scope: InvScope{Level: "profile", Name: "default"}, Name: "default",
		Source: yml, Managed: true, Editable: false, Why: "implícito", Hash: invHashJSON("default")})
	for _, n := range names {
		w.add(InvItem{Kind: "profile", Scope: InvScope{Level: "profile", Name: n}, Name: n,
			Source: yml, Key: "profiles." + n, Managed: true, Editable: true, Hash: invHashJSON(cfg.Profiles[n])})
	}
	for _, rl := range cfg.Rules {
		// Una regla de carpeta la aplica el hook del shell: solo la CLI la ve.
		// Una cuya carpeta ya no está no aplica a nada: se dice, para que el
		// plan de adopción no la trate como un proyecto vivo.
		why := ""
		if !invExists(rl.Path) {
			why = "la carpeta no existe"
		}
		w.add(InvItem{Kind: "rule-path", Scope: InvScope{Level: "profile", Name: rl.Profile}, Name: rl.Path,
			Source: yml, Key: "rules", Managed: true, Editable: true, Why: why, AppliesTo: []string{InvAppliesCLI},
			Hash: invHashJSON(rl)})
	}
	for _, n := range names {
		sc := InvScope{Level: "profile", Name: n}
		md := cfgInstrFile(home, n)
		if b, ok := w.readFile(md); ok {
			w.add(InvItem{Kind: "rule-instr", Scope: sc, Name: "CLAUDE.md", Source: md,
				Managed: true, Editable: true, Hash: invHashText(b)})
		}
		st := cfgSettingsFile(home, n)
		if m, ok := w.readJSONObject(st); ok {
			w.invSettings(st, sc, true, m)
		}
	}
	return cfg
}

// walkClaudeGlobal: el ~/.claude del usuario. Nada de aquí lo escribió ccp.
func (w *invWalker) walkClaudeGlobal(src string) {
	sc := InvScope{Level: "global"}
	st := filepath.Join(src, "settings.json")
	enabled := map[string]any{}
	if m, ok := w.readJSONObject(st); ok {
		w.invSettings(st, sc, false, m)
		if ep, isMap := m["enabledPlugins"].(map[string]any); isMap {
			enabled = ep
		}
	}
	md := filepath.Join(src, "CLAUDE.md")
	if b, ok := w.readFile(md); ok {
		w.add(InvItem{Kind: "rule-instr", Scope: sc, Name: "CLAUDE.md", Source: md, Editable: true, Hash: invHashText(b)})
	}
	w.walkMarkdown(filepath.Join(src, "agents"), "", "agent", sc)
	w.walkMarkdown(filepath.Join(src, "commands"), "", "command", sc)
	w.walkMarkdown(filepath.Join(src, "output-styles"), "", "output-style", sc)
	w.walkSkills(filepath.Join(src, "skills"), sc)

	hooks := filepath.Join(src, "hooks")
	if es, ok := w.readDir(hooks); ok {
		for _, e := range es {
			if e.IsDir() {
				continue
			}
			p := filepath.Join(hooks, e.Name())
			if b, ok := w.readFile(p); ok {
				w.add(InvItem{Kind: "hook", Scope: sc, Name: e.Name(), Source: p, Editable: true, Hash: invHashText(b)})
			}
		}
	}

	kb := filepath.Join(src, "keybindings.json")
	if v, ok := w.readJSON(kb); ok {
		w.add(InvItem{Kind: "settings-key", Scope: sc, Name: "keybindings", Source: kb, Editable: true, Hash: invHashJSON(v)})
	}

	ip := filepath.Join(src, "plugins", "installed_plugins.json")
	if m, ok := w.readJSONObject(ip); ok {
		// v2 es {"version":2,"plugins":{"n@mkt":[…]}}; v1 era {"n@mkt":{…}} bajo
		// "plugins" también, así que basta con las claves de "plugins".
		plugins, _ := m["plugins"].(map[string]any)
		for _, n := range invSortedKeys(plugins) {
			on, _ := enabled[n].(bool)
			w.plugins = append(w.plugins, invPlugin{id: n, path: invPluginPath(plugins[n]), enabled: on})
			w.add(InvItem{Kind: "plugin", Scope: sc, Name: n, Source: ip, Key: "plugins." + n,
				Editable: true, Enabled: on, Hash: invHashJSON(plugins[n])})
		}
	}
}

// invMaxDepth acota el recorrido de un árbol de .md.
const invMaxDepth = 8

// walkMarkdown recorre un árbol de .md (agents, commands, output-styles). El
// nombre es la ruta relativa sin extensión (`git/pr`), que es como Claude Code
// los distingue. Un subdirectorio ilegible deja su propia sonda unknown.
func (w *invWalker) walkMarkdown(dir, rel, kind string, sc InvScope) {
	es, ok := w.readDir(dir)
	if !ok {
		return
	}
	for _, e := range es {
		p := filepath.Join(dir, e.Name())
		r := e.Name()
		if rel != "" {
			r = rel + "/" + e.Name()
		}
		if w.isDir(e, p) {
			// isDir sigue enlaces, así que un enlace al padre sería un bucle:
			// ningún árbol real de Claude Code pasa de unos pocos niveles.
			if strings.Count(r, "/") < invMaxDepth {
				w.walkMarkdown(p, r, kind, sc)
			}
			continue
		}
		if !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		if b, ok := w.readFile(p); ok {
			w.add(InvItem{Kind: kind, Scope: sc, Name: strings.TrimSuffix(r, ".md"), Source: p,
				Editable: true, Hash: invHashText(b)})
		}
	}
}

// walkSkills: una skill es una carpeta con SKILL.md; sin él no es skill y no
// deja sonda (una carpeta de notas no es una fuente que haya fallado).
func (w *invWalker) walkSkills(dir string, sc InvScope) {
	es, ok := w.readDir(dir)
	if !ok {
		return
	}
	for _, e := range es {
		d := filepath.Join(dir, e.Name())
		if !w.isDir(e, d) {
			continue
		}
		p := filepath.Join(d, "SKILL.md")
		if _, err := os.Stat(p); errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if b, ok := w.readFile(p); ok {
			w.add(InvItem{Kind: "skill", Scope: sc, Name: e.Name(), Source: p, Editable: true, Hash: invHashText(b)})
		}
	}
}

// invPluginPath saca la ruta en disco de una entrada de installed_plugins.json:
// en v2 es una lista de instalaciones (por scope) y en v1 un objeto. Vale la
// primera que la traiga; sin ninguna, el plugin no aporta MCP.
func invPluginPath(v any) string {
	entries, isList := v.([]any)
	if !isList {
		entries = []any{v}
	}
	for _, e := range entries {
		if m, ok := e.(map[string]any); ok {
			if p, _ := m["installPath"].(string); p != "" {
				return p
			}
		}
	}
	return ""
}

// invMCPOpts describe una fuente de MCP: a qué capa pertenece cada entrada,
// dónde aplica y si el usuario puede tocarla desde ccp.
type invMCPOpts struct {
	src, keyPrefix string
	sc             InvScope
	applies        []string
	editable       bool
	why            string
	// desktop: el archivo es claude_desktop_config.json, que solo carga
	// entradas stdio (ADR 0016 M3).
	desktop bool
	// pluginRoot sustituye ${CLAUDE_PLUGIN_ROOT} antes de resolver el comando.
	pluginRoot string
}

// invMCPShape son los campos que definen QUÉ servidor es. Todo lo demás
// (env, headers) es credencial o ajuste de la cuenta y va al SecretHash.
var invMCPShape = []string{"type", "command", "args", "url"}

// invMCPServers añade un item por servidor de un mapa mcpServers.
func (w *invWalker) invMCPServers(servers map[string]any, o invMCPOpts) {
	for _, name := range invSortedKeys(servers) {
		entry, ok := servers[name].(map[string]any)
		if !ok {
			continue
		}
		shape := map[string]any{}
		for _, k := range invMCPShape {
			if v, has := entry[k]; has {
				shape[k] = v
			}
		}
		full := map[string]any{"shape": shape}
		var secrets []string
		for _, sk := range []string{"env", "headers"} {
			m, isMap := entry[sk].(map[string]any)
			if !isMap || len(m) == 0 {
				continue
			}
			full[sk] = m
			for _, k := range invSortedKeys(m) {
				secrets = append(secrets, sk+"."+k)
			}
		}
		typ, _ := entry["type"].(string)
		stdio := typ == "" || typ == "stdio"
		it := InvItem{Kind: "mcp", Scope: o.sc, Name: name, Source: o.src, Key: o.keyPrefix + name,
			Editable: o.editable, Why: o.why, AppliesTo: o.applies, Secrets: secrets,
			Hash: invHashJSON(shape), SecretHash: invHashJSON(full)}
		if o.desktop && !stdio {
			it.Why = "Desktop solo carga stdio en este archivo (M3)"
		}
		if cmd, _ := entry["command"].(string); stdio && cmd != "" {
			it.Missing = w.mcpMissing(cmd, o.pluginRoot)
		}
		w.add(it)
	}
}

// mcpMissing devuelve el comando si no resuelve, o "". Un absoluto se mira en
// disco; uno relativo solo si hay LookPath, porque sin él no se puede saber y
// acusar en falso a un servidor sano ensuciaría los pendientes del plan.
func (w *invWalker) mcpMissing(cmd, pluginRoot string) string {
	return mcpMissingCommand(cmd, pluginRoot, w.lookPath)
}

// mcpMissingCommand es la regla suelta, compartida con el doctor (§6.4): una
// sola definición de «este command no resuelve», porque dos acabarían
// respondiendo cosas distintas sobre el mismo servidor.
func mcpMissingCommand(cmd, pluginRoot string, look func(string) (string, error)) string {
	c := cmd
	if pluginRoot != "" {
		c = strings.ReplaceAll(c, "${CLAUDE_PLUGIN_ROOT}", pluginRoot)
	}
	if strings.Contains(c, "${") {
		// Una variable que no sabemos expandir no es un comando ausente.
		return ""
	}
	if filepath.IsAbs(c) {
		if _, err := os.Stat(c); err != nil {
			return cmd
		}
		return ""
	}
	if look == nil {
		return ""
	}
	if _, err := look(c); err != nil {
		return cmd
	}
	return ""
}

// invMCPMap devuelve el mapa `mcpServers` de un objeto, o nil.
func invMCPMap(m map[string]any) map[string]any {
	s, _ := m["mcpServers"].(map[string]any)
	return s
}

// walkMCP recorre todas las fuentes de MCP con su «dónde aplica» (ADR 0016).
// Van aparte de settings porque un MCP no vive en settings.json: vive en el
// .claude.json que Claude Code reescribe, en el claude_desktop_config.json de
// cada ventana, en .mcp.json de proyecto, en managed y en los plugins.
func (w *invWalker) walkMCP(r InventoryRoots, cfg *Config) {
	code := invCodeTargets()
	// La ventana de Desktop comparte su pool con la pestaña Code (M2).
	win := []string{InvAppliesDesktopChat, InvAppliesDesktopCode}
	// ~/.claude.json: el global de la CLI (perfil default) y de la pestaña Code
	// de la ventana default; y por proyecto, los de `claude mcp add -s local`.
	if r.ClaudeSrc != "" {
		cj := r.ClaudeSrc + ".json"
		if m, ok := w.readJSONObject(cj); ok {
			w.invMCPServers(invMCPMap(m), invMCPOpts{src: cj, keyPrefix: "mcpServers.",
				sc: InvScope{Level: "global"}, applies: code, editable: true})
			ps, _ := m["projects"].(map[string]any)
			for _, p := range invSortedKeys(ps) {
				w.projects = append(w.projects, p)
				pm, _ := ps[p].(map[string]any)
				from := len(w.inv.Items)
				w.invMCPServers(invMCPMap(pm), invMCPOpts{src: cj, keyPrefix: "projects." + p + ".mcpServers.",
					sc: InvScope{Level: "project", Name: p}, applies: code, editable: true})
				w.tagProject(from, p)
			}
		}
	}

	if r.DesktopDefaultDataDir != "" {
		w.invDesktopMCP(filepath.Join(r.DesktopDefaultDataDir, "claude_desktop_config.json"), "default", win)
	}
	if cfg != nil {
		for _, n := range invSortedProfiles(cfg) {
			cj := filepath.Join(ccHomePath(r.CCPHome, n), ".claude.json")
			if m, ok := w.readJSONObject(cj); ok {
				w.invMCPServers(invMCPMap(m), invMCPOpts{src: cj, keyPrefix: "mcpServers.",
					sc: InvScope{Level: "profile", Name: n}, applies: code, editable: true})
				ps, _ := m["projects"].(map[string]any)
				w.projects = append(w.projects, invSortedKeys(ps)...)
			}
			w.invDesktopMCP(filepath.Join(DesktopDataDir(r.CCPHome, n), "claude_desktop_config.json"), n, win)
		}
		for _, rl := range cfg.Rules {
			w.projects = append(w.projects, rl.Path)
		}
	}
	w.projects = append(w.projects, r.ExtraProjects...)
	w.invProjectMCP(code)

	if r.ManagedDir != "" {
		mf := filepath.Join(r.ManagedDir, "managed-mcp.json")
		if m, ok := w.readJSONObject(mf); ok {
			w.invMCPServers(invMCPMap(m), invMCPOpts{src: mf, keyPrefix: "mcpServers.",
				sc: InvScope{Level: "managed"}, applies: code, why: "managed-settings"})
		}
	}
	w.invPluginMCP(code)
}

// invSortedProfiles son los perfiles de ccp.yaml en orden, sin `default`: ese
// no tiene cc-home ni data dir propio (sus fuentes son las globales).
func invSortedProfiles(cfg *Config) []string {
	names := make([]string, 0, len(cfg.Profiles))
	for n := range cfg.Profiles {
		if n != "default" {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	return names
}

// invDesktopMCP lee el claude_desktop_config.json de una ventana. Sus MCP son
// el pool que comparten el chat y la pestaña Code de esa ventana (M2).
func (w *invWalker) invDesktopMCP(path, window string, applies []string) {
	if m, ok := w.readJSONObject(path); ok {
		w.invMCPServers(invMCPMap(m), invMCPOpts{src: path, keyPrefix: "mcpServers.",
			sc: InvScope{Level: "desktop", Name: window}, applies: applies, editable: true, desktop: true})
	}
}

// invProjectMCP lee el .mcp.json de cada proyecto conocido (de los
// ~/.claude.json y de las reglas de ccp), una sola vez por ruta.
func (w *invWalker) invProjectMCP(applies []string) {
	for _, p := range w.invProjectPaths() {
		f := filepath.Join(p, ".mcp.json")
		if !invExists(f) {
			continue
		}
		if m, ok := w.readJSONObject(f); ok {
			from := len(w.inv.Items)
			w.invMCPServers(invMCPMap(m), invMCPOpts{src: f, keyPrefix: "mcpServers.",
				sc: InvScope{Level: "project", Name: p}, applies: applies, editable: true})
			w.tagProject(from, p)
		}
	}
}

// invPluginMCP: los MCP que trae cada plugin instalado y activo. Un plugin
// sin carpeta en disco o sin .mcp.json no aporta nada y no deja sonda: la
// mayoría no trae MCP, y cien sondas «missing» taparían las que importan.
func (w *invWalker) invPluginMCP(applies []string) {
	for _, p := range w.plugins {
		if !p.enabled || p.path == "" {
			continue
		}
		f := filepath.Join(p.path, ".mcp.json")
		if _, err := os.Stat(f); errors.Is(err, fs.ErrNotExist) {
			continue
		}
		m, ok := w.readJSONObject(f)
		if !ok {
			continue
		}
		// El .mcp.json de un plugin admite las dos formas: con la envoltura
		// mcpServers o con los servidores en la raíz.
		servers := invMCPMap(m)
		if servers == nil {
			servers = m
		}
		w.invMCPServers(servers, invMCPOpts{src: f, keyPrefix: "mcpServers.",
			sc: InvScope{Level: "plugin", Name: p.id}, applies: applies,
			why: "lo trae el plugin " + p.id, pluginRoot: p.path})
	}
}
