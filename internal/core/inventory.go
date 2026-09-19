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
	// SecretHash sirve para comparar capas en memoria (la Task 4) y no sale
	// nunca en JSON: el hash de un token corto es un oráculo para adivinarlo.
	SecretHash string `json:"-"`
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
	inv Inventory
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
	w := &invWalker{inv: Inventory{Items: []InvItem{}, Probes: []InvProbe{}}}
	// Una raíz vacía no se recorre: Load("") leería ./ccp.yaml del cwd.
	if r.CCPHome != "" {
		w.walkCCP(r.CCPHome)
	}
	if r.ClaudeSrc != "" {
		w.walkClaudeGlobal(r.ClaudeSrc)
	}
	return w.inv
}

// walkCCP: los perfiles, las reglas y el overlay de cada perfil. Todo lo
// escribió ccp (Managed), aunque el overlay lo edite el usuario.
func (w *invWalker) walkCCP(home string) {
	yml := yamlPath(home)
	if _, err := os.Stat(yml); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			w.probe(yml, "missing", nil)
		} else {
			w.probe(yml, "unknown", err)
		}
		return
	}
	cfg, err := Load(home)
	if err != nil {
		w.probe(yml, "unknown", err)
		return
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
		w.add(InvItem{Kind: "rule-path", Scope: InvScope{Level: "profile", Name: rl.Profile}, Name: rl.Path,
			Source: yml, Key: "rules", Managed: true, Editable: true, AppliesTo: []string{InvAppliesCLI},
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
