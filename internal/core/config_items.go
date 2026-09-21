package core

// config_items.go — los elementos editables de una capa de configuración
// (spec 2026-09-18 §7, C1). Es lo que pinta P-20: arriba la capa (global ·
// perfil · proyecto · desktop), a la izquierda los tipos, en el centro los
// elementos con su procedencia y su «dónde aplica».
//
// No hay un segundo recorrido del disco: ConfigItems parte de BuildInventory,
// que ya sabe leer todas las fuentes con la regla del doctor (lo que no se pudo
// leer es `unknown`, jamás «vacío»). Aquí se hacen tres cosas que el inventario
// no hace, porque su pregunta es otra («qué hay en esta máquina» para adoptar,
// no «qué puedo editar aquí»):
//
//  1. cada item se clasifica en uno de los tipos de la columna izquierda;
//  2. se añade la DIRECCIÓN de escritura (ConfigRef), que no siempre es el
//     archivo donde el inventario lo encontró: los MCP de un perfil se declaran
//     en overlay/mcp.json y se PROYECTAN a cc-home/.claude.json (§6.1), así que
//     lo proyectado se ve pero no se edita, o el siguiente `profile sync` se
//     comería el cambio sin decir nada;
//  3. lo que aplica pero no se puede tocar dice por qué (lo trae un plugin,
//     managed-settings, lo proyecta ccp).
//
// Las raíces vienen inyectadas, como en inventory.go: una máquina entera cabe
// en un t.TempDir().

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

// ConfigLayer es la capa que se está editando. Name es el perfil, la ruta del
// proyecto o la ventana de Desktop; vacío en global.
type ConfigLayer struct {
	Level string `json:"level"` // global|profile|project|desktop
	Name  string `json:"name,omitempty"`
}

// Tipos: la columna izquierda de P-20. CfgTypeSettings no está en el diseño y
// existe porque el resto de claves de settings.json (model, keybindings…) son
// editables y tienen que caer en algún sitio: sin él desaparecerían de la
// pantalla justo las que el usuario toca con /config.
const (
	CfgTypeInstructions = "instructions"
	CfgTypeMCP          = "mcp"
	CfgTypeSkills       = "skills"
	CfgTypeAgents       = "agents"
	CfgTypeCommands     = "commands"
	CfgTypeHooks        = "hooks"
	CfgTypePermissions  = "permissions"
	CfgTypeEnv          = "env"
	CfgTypePlugins      = "plugins"
	CfgTypeStyles       = "styles"
	CfgTypeStatusLine   = "statusline"
	CfgTypeSettings     = "settings"
)

// cfgTypeOrder fija el orden de la columna izquierda y, con él, el de la lista:
// dos ejecuciones seguidas devuelven lo mismo.
var cfgTypeOrder = []string{
	CfgTypeInstructions, CfgTypeMCP, CfgTypeSkills, CfgTypeAgents, CfgTypeCommands,
	CfgTypeHooks, CfgTypePermissions, CfgTypeEnv, CfgTypePlugins, CfgTypeStyles,
	CfgTypeStatusLine, CfgTypeSettings,
}

// Formatos de valor, que es lo que decide qué editor abre la GUI y cómo viaja
// el valor por serve.
const (
	// CfgFormatText: el elemento ES un archivo (CLAUDE.md, SKILL.md, un agente).
	CfgFormatText = "text"
	// CfgFormatJSON: una clave dentro de un archivo JSON.
	CfgFormatJSON = "json"
	// CfgFormatEntry: una entrada suelta de una lista (un permiso). Se edita
	// reescribiendo la lista, que es la única operación que el archivo admite.
	CfgFormatEntry = "entry"
)

// ConfigRef direcciona un elemento para leerlo o escribirlo. Es la misma pareja
// (archivo, clave) con la que el inventario direcciona todo, más la capa a la
// que se escribe de verdad: para el perfil `default` es la global, porque su
// CLAUDE_CONFIG_DIR es ~/.claude y no tiene overlay que valga.
type ConfigRef struct {
	Layer ConfigLayer `json:"layer"`
	Type  string      `json:"type"`
	// Name es el nombre del elemento. Con Source vacío es lo que decide dónde
	// se CREA (un agente nuevo, una variable nueva): sin él la GUI tendría que
	// saber la ruta de cada tipo en cada capa, que es justo lo que core sabe.
	Name   string `json:"name,omitempty"`
	Source string `json:"source,omitempty"`
	Key    string `json:"key,omitempty"`
	// Entry es la entrada concreta dentro de la clave cuando esta es una lista
	// (un permiso). La lista se reescribe entera: es la única operación que el
	// archivo admite.
	Entry string `json:"entry,omitempty"`
}

// ConfigItem es un elemento de la capa con todo lo que P-20 enseña de él.
type ConfigItem struct {
	Ref  ConfigRef `json:"ref"`
	Name string    `json:"name"`
	// Scope es la procedencia real: de dónde sale, que no siempre es la capa
	// que se mira (un MCP de plugin se ve en la global y viene del plugin).
	Scope     InvScope `json:"scope"`
	Format    string   `json:"format"`
	Editable  bool     `json:"editable"`
	Why       string   `json:"why,omitempty"`
	AppliesTo []string `json:"applies_to"`
	Managed   bool     `json:"managed,omitempty"`
	Enabled   bool     `json:"enabled,omitempty"`
	// Missing es el `command` de un MCP stdio que no resuelve.
	Missing string `json:"missing,omitempty"`
}

// ConfigList es la respuesta entera. Items y Probes nunca son nil: una capa sin
// nada devuelve listas vacías, no null.
type ConfigList struct {
	Layer  ConfigLayer  `json:"layer"`
	Items  []ConfigItem `json:"items"`
	Probes []InvProbe   `json:"probes"`
}

// Motivos por los que un elemento se ve pero no se edita.
const (
	cfgWhyInstalled = "lo instala Claude Code, no ccp"
	cfgWhyProjected = "lo proyecta ccp desde %s"
)

// cfgTypeOf clasifica un item del inventario. El segundo valor es falso para lo
// que no es configuración editable en esta pantalla (los perfiles y las reglas
// de carpeta de ccp, que tienen sus propias pantallas, y los config dirs
// ajenos, que son material de adopción).
func cfgTypeOf(it InvItem) (typ, format string, ok bool) {
	switch it.Kind {
	case "rule-instr":
		return CfgTypeInstructions, CfgFormatText, true
	case "mcp":
		return CfgTypeMCP, CfgFormatJSON, true
	case "skill":
		return CfgTypeSkills, CfgFormatText, true
	case "agent":
		return CfgTypeAgents, CfgFormatText, true
	case "command":
		return CfgTypeCommands, CfgFormatText, true
	case "permission":
		return CfgTypePermissions, CfgFormatEntry, true
	case "env":
		return CfgTypeEnv, CfgFormatJSON, true
	case "plugin":
		return CfgTypePlugins, CfgFormatJSON, true
	case "statusline":
		return CfgTypeStatusLine, CfgFormatJSON, true
	case "hook", "output-style":
		// Los dos llegan por dos caminos: un archivo suelto (hooks/pre.sh,
		// output-styles/terse.md) o una clave de settings.json. El inventario
		// los distingue por Key, y el editor que abre cada uno es distinto.
		t := CfgTypeHooks
		if it.Kind == "output-style" {
			t = CfgTypeStyles
		}
		if it.Key == "" {
			return t, CfgFormatText, true
		}
		return t, CfgFormatJSON, true
	case "settings-key":
		switch {
		case strings.HasPrefix(it.Key, "permissions."):
			return CfgTypePermissions, CfgFormatJSON, true
		case it.Key == "enabledPlugins":
			return CfgTypePlugins, CfgFormatJSON, true
		}
		return CfgTypeSettings, CfgFormatJSON, true
	}
	return "", "", false
}

// cfgItemFrom convierte un item del inventario en uno de la capa. `write` es la
// capa a la que se escribiría, que no es la de procedencia cuando lo que se
// mira es `default` (escribe en la global) o algo que aplica sin ser de aquí.
func cfgItemFrom(it InvItem, write ConfigLayer) (ConfigItem, bool) {
	typ, format, ok := cfgTypeOf(it)
	if !ok {
		return ConfigItem{}, false
	}
	ci := ConfigItem{
		Ref:       ConfigRef{Layer: write, Type: typ, Name: it.Name, Source: it.Source, Key: it.Key},
		Name:      it.Name,
		Scope:     it.Scope,
		Format:    format,
		Editable:  it.Editable,
		Why:       it.Why,
		AppliesTo: it.AppliesTo,
		Managed:   it.Managed,
		Enabled:   it.Enabled,
		Missing:   it.Missing,
	}
	// Un permiso se nombra «allow:Bash(ls)»: el valor suelto es lo que hay que
	// quitar de la lista, y la referencia lo lleva para no tener que reconstruirlo.
	if format == CfgFormatEntry {
		if _, entry, ok := strings.Cut(it.Name, ":"); ok {
			ci.Ref.Entry = entry
		}
	}
	// Un plugin instalado se ve (aplica a todo) pero ccp no lo instala ni lo
	// desinstala: lo editable de un plugin es encenderlo, que es enabledPlugins.
	if it.Kind == "plugin" {
		ci.Editable = false
		if ci.Why == "" {
			ci.Why = cfgWhyInstalled
		}
	}
	// Lo que viene de managed-settings o de un plugin nunca se edita, diga lo
	// que diga el inventario: no es de esta capa, solo aplica en ella.
	if it.Scope.Level == "managed" || it.Scope.Level == "plugin" {
		ci.Editable = false
		ci.Ref.Layer = ConfigLayer{Level: it.Scope.Level, Name: it.Scope.Name}
	}
	return ci, true
}

// cfgWriteLayer es la capa a la que se escribe de verdad lo que se mira, y el
// único sitio donde se decide. `default` no tiene capa propia: su
// CLAUDE_CONFIG_DIR es ~/.claude, así que editarlo es editar la global.
func cfgWriteLayer(layer ConfigLayer) (ConfigLayer, error) {
	switch layer.Level {
	case "global":
		return ConfigLayer{Level: "global"}, nil
	case "profile":
		if layer.Name == "" {
			return ConfigLayer{}, fmt.Errorf("la capa de perfil necesita un nombre")
		}
		if layer.Name == "default" {
			return ConfigLayer{Level: "global"}, nil
		}
		return layer, nil
	case "project", "desktop":
		if layer.Name == "" {
			return ConfigLayer{}, fmt.Errorf("la capa %s necesita un nombre", layer.Level)
		}
		return layer, nil
	}
	return ConfigLayer{}, fmt.Errorf("capa desconocida %q (valen: global, profile, project, desktop)", layer.Level)
}

// cfgInLayer dice si la procedencia de un item cae dentro de la capa que se
// mira. La global recoge además lo que aplica en todas partes sin ser suyo
// (managed-settings y los plugins): es donde el usuario espera verlo, y verlo
// con su motivo es mejor que no verlo.
func cfgInLayer(sc InvScope, layer ConfigLayer) bool {
	switch layer.Level {
	case "global":
		return sc.Level == "global" || sc.Level == "managed" || sc.Level == "plugin"
	case "project":
		return sc.Level == "project" && filepath.Clean(sc.Name) == filepath.Clean(layer.Name)
	default:
		return sc.Level == layer.Level && sc.Name == layer.Name
	}
}

// ConfigItems lista los elementos editables (y los que aplican sin serlo) de
// una capa. No escribe nada y no falla por lo que no pudo leer: eso queda en
// Probes, como en el inventario.
func ConfigItems(r InventoryRoots, layer ConfigLayer) (ConfigList, error) {
	write, err := cfgWriteLayer(layer)
	if err != nil {
		return ConfigList{}, err
	}
	// Lo que se MIRA es la capa pedida, salvo default: sus fuentes son las
	// globales, así que se recorre la global.
	look := layer
	if write.Level == "global" {
		look = write
	}
	// La capa pedida se recorre aunque el inventario no la conociera: una
	// carpeta sin regla y en la que nunca se abrió Claude Code sigue siendo la
	// que el usuario nombró, y sus archivos están ahí.
	if look.Level == "project" {
		r.ExtraProjects = append(append([]string{}, r.ExtraProjects...), filepath.Clean(look.Name))
	}
	inv := BuildInventory(r)
	out := ConfigList{Layer: layer, Items: []ConfigItem{}, Probes: []InvProbe{}}
	used := map[string]bool{}
	declared := cfgDeclaredProfile(r, look, write, &out.Probes, used)
	// Un MCP declarado aquí y ya proyectado al cc-home es UN servidor, no dos:
	// la copia proyectada es el resultado de esta misma fila. Se enseña la
	// declarada, que es la que se edita; que la proyección esté al día lo dice
	// el doctor (projection_stale), no una fila repetida.
	dup := map[string]bool{}
	for _, d := range declared {
		if d.Ref.Type == CfgTypeMCP {
			dup[d.Name] = true
		}
	}
	for _, it := range inv.Items {
		if !cfgInLayer(it.Scope, look) {
			continue
		}
		ci, ok := cfgItemFrom(it, write)
		if !ok {
			continue
		}
		cfgMCPProjected(r, look, &ci)
		if ci.Ref.Type == CfgTypeMCP && !ci.Editable && dup[ci.Name] {
			continue
		}
		out.Items = append(out.Items, ci)
		used[ci.Ref.Source] = true
	}
	out.Items = append(out.Items, declared...)
	out.Probes = append(out.Probes, cfgLayerProbes(r, look, inv.Probes, used)...)
	cfgSortItems(out.Items)
	return out, nil
}

// cfgSortItems ordena por tipo (el orden de la columna izquierda), nombre y
// clave: la lista sale igual en cada ejecución.
func cfgSortItems(items []ConfigItem) {
	rank := map[string]int{}
	for i, t := range cfgTypeOrder {
		rank[t] = i
	}
	sort.SliceStable(items, func(i, j int) bool {
		a, b := items[i], items[j]
		if rank[a.Ref.Type] != rank[b.Ref.Type] {
			return rank[a.Ref.Type] < rank[b.Ref.Type]
		}
		if a.Name != b.Name {
			return a.Name < b.Name
		}
		return a.Ref.Key < b.Ref.Key
	})
}

// cfgLayerProbes se queda con las sondas que hablan de esta capa: las de las
// fuentes que aportaron algún elemento y las de cualquier archivo bajo sus
// raíces. Una sonda `unknown` de otra capa no se cuela, pero ninguna de esta se
// pierde: es lo que distingue «aquí no hay nada» de «no pude mirar».
func cfgLayerProbes(r InventoryRoots, layer ConfigLayer, all []InvProbe, used map[string]bool) []InvProbe {
	roots := cfgLayerRoots(r, layer)
	out := []InvProbe{}
	for _, p := range all {
		if used[p.Source] {
			out = append(out, p)
			continue
		}
		for _, root := range roots {
			if root != "" && strings.HasPrefix(p.Source, root) {
				out = append(out, p)
				break
			}
		}
	}
	return out
}

// cfgLayerRoots son los prefijos de disco de una capa. Cada raíz lleva su
// separador final, y una vacía se descarta: sin eso, un ManagedDir sin
// configurar daba el prefijo "/" y la capa global se quedaba con las sondas de
// la máquina entera.
func cfgLayerRoots(r InventoryRoots, layer ConfigLayer) []string {
	dirs := func(ds ...string) []string {
		out := []string{}
		for _, d := range ds {
			if d != "" {
				out = append(out, d+string(filepath.Separator))
			}
		}
		return out
	}
	switch layer.Level {
	case "global":
		// El ~/.claude.json queda fuera del árbol del cc-home global, así que
		// va suelto: es la capa global de MCP.
		out := dirs(r.ClaudeSrc, r.ManagedDir)
		if r.ClaudeSrc != "" {
			out = append(out, r.ClaudeSrc+".json")
		}
		return out
	case "profile":
		if r.CCPHome == "" {
			return nil
		}
		return dirs(filepath.Join(r.CCPHome, "profiles", layer.Name))
	case "project":
		return dirs(filepath.Clean(layer.Name))
	case "desktop":
		return dirs(cfgDesktopDir(r, layer.Name))
	}
	return nil
}

// cfgDesktopDir es el data dir de una ventana: el del perfil o, para default, el
// del Claude del usuario, que solo conoce quien inyecta las raíces.
func cfgDesktopDir(r InventoryRoots, name string) string {
	if name == "default" {
		return r.DesktopDefaultDataDir
	}
	return DesktopDataDir(r.CCPHome, name)
}

// cfgMCPProjected corrige la editabilidad de un MCP que se ve en una capa pero
// se declara en otra (§6.1). El cc-home/.claude.json de un perfil y el
// claude_desktop_config.json de una ventana son DESTINOS de la proyección: lo
// que ccp escribió allí se ve, pero se edita en su capa, o el siguiente
// `profile sync` se comería el cambio sin decir nada. Y lo que no gestiona ccp
// tampoco se toca desde aquí: ese archivo lo reescribe Claude Code.
func cfgMCPProjected(r InventoryRoots, look ConfigLayer, ci *ConfigItem) {
	if ci.Ref.Type != CfgTypeMCP {
		return
	}
	switch look.Level {
	case "profile":
		if ci.Ref.Source != filepath.Join(ccHomePath(r.CCPHome, look.Name), ".claude.json") {
			return
		}
		ci.Editable = false
		ci.Managed = cfgMCPIsManaged(filepath.Join(ccHomePath(r.CCPHome, look.Name), ".ccp-managed.json"), ci.Name)
		if ci.Managed {
			ci.Why = fmt.Sprintf(cfgWhyProjected, MCPProfileFile(r.CCPHome, look.Name))
		} else {
			ci.Why = fmt.Sprintf("lo reescribe Claude Code; declara los MCP del perfil en %s",
				MCPProfileFile(r.CCPHome, look.Name))
		}
	case "desktop":
		dir := cfgDesktopDir(r, look.Name)
		if dir == "" || !cfgMCPIsManaged(filepath.Join(dir, ".ccp-managed-mcp.json"), ci.Name) {
			return
		}
		ci.Editable, ci.Managed = false, true
		ci.Why = fmt.Sprintf(cfgWhyProjected, MCPProfileFile(r.CCPHome, look.Name))
	}
}

// cfgMCPIsManaged dice si ese nombre está en el registro de lo que ccp escribió
// en un destino. Un registro perdido no es un error: significa «no gestionado»,
// que es la respuesta prudente (se ve, no se edita, y se dice por qué).
func cfgMCPIsManaged(registry, name string) bool {
	for _, n := range readManaged(registry) {
		if n == name {
			return true
		}
	}
	return false
}

// cfgDeclaredProfile añade lo que un perfil DECLARA en su overlay y el
// inventario no lee, porque no es configuración de Claude Code sino la fuente
// desde la que ccp proyecta (§6.1 y §6.2): overlay/mcp.json y
// overlay/{agents,commands,skills,output-styles}. Lo que hay en el cc-home es
// el resultado, y ya se lista como proyectado. Deja sus propias sondas.
func cfgDeclaredProfile(r InventoryRoots, look, write ConfigLayer, probes *[]InvProbe, used map[string]bool) []ConfigItem {
	if look.Level != "profile" || look.Name == "" || look.Name == "default" || r.CCPHome == "" {
		return nil
	}
	sc := InvScope{Level: "profile", Name: look.Name}
	w := &invWalker{inv: Inventory{}, lookPath: r.LookPath}
	file := MCPProfileFile(r.CCPHome, look.Name)
	if m, ok := w.readJSONObject(file); ok {
		used[file] = true
		w.invMCPServers(invMCPMap(m), invMCPOpts{src: file, keyPrefix: "mcpServers.",
			sc: sc, applies: invCodeTargets(), editable: true})
	}
	ov := cfgOverlayDir(r.CCPHome, look.Name)
	for _, d := range []struct{ dir, kind string }{
		{"agents", "agent"}, {"commands", "command"}, {"output-styles", "output-style"},
	} {
		// Un directorio que el perfil no declara no deja sonda: no tenerlo es
		// lo normal, y cuatro «missing» por perfil taparían las que importan.
		p := filepath.Join(ov, d.dir)
		if invExists(p) {
			w.walkMarkdown(p, "", d.kind, sc)
		}
	}
	if p := filepath.Join(ov, "skills"); invExists(p) {
		w.walkSkills(p, sc)
	}
	*probes = append(*probes, w.inv.Probes...)
	out := make([]ConfigItem, 0, len(w.inv.Items))
	for _, it := range w.inv.Items {
		it.Managed = true // lo escribe el usuario en el overlay y lo proyecta ccp
		used[it.Source] = true
		if ci, ok := cfgItemFrom(it, write); ok {
			out = append(out, ci)
		}
	}
	return out
}
