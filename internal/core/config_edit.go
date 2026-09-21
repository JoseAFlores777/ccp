package core

// config_edit.go — leer y escribir un elemento de configuración por su
// referencia (spec §7, C1). Va aparte de config_items.go por tamaño: allí se
// mira, aquí se toca.
//
// Nada se escribe a mano: cada tipo va al escritor que ya existía —
// SettingsLayerSet para una clave de settings, OverlayEnvSet/Del para el env de
// un perfil (que valida y regenera), los archivos de artefacto tal cual— y la
// regeneración se decide en un solo sitio (cfgRegenFor). Lo que este archivo
// añade es el enrutado: de una referencia a un archivo, un camino JSON y un
// escritor.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ConfigValue es el valor de un elemento. Text para lo que es un archivo, JSON
// para una clave; Exists distingue «vale null» de «no está».
type ConfigValue struct {
	Format string `json:"format"`
	Text   string `json:"text,omitempty"`
	JSON   any    `json:"json,omitempty"`
	Exists bool   `json:"exists"`
	// Extras son los archivos anexos de una skill (rutas relativas a su
	// carpeta, sin el SKILL.md). Una skill es una carpeta y este valor solo
	// lleva un archivo, así que quien la copie a otra capa tiene que saber
	// qué NO viaja: sin este dato «llevar a…» dejaba una skill rota en el
	// destino y los anexos huérfanos —e invisibles, porque el inventario
	// exige SKILL.md— en el origen. Nunca se rellena para otros tipos.
	Extras []string `json:"extras,omitempty"`
}

// ConfigWrite cuenta qué se tocó. Regenerated nunca es nil.
type ConfigWrite struct {
	File        string   `json:"file"`
	Regenerated []string `json:"regenerated"`
	// MCP son las proyecciones que hizo esa regeneración, cada una con el
	// perfil al que pertenece (C2): tocar la capa global regenera varios, y
	// «pendiente de reiniciar la ventana» hay que decirlo con nombre. MCPErr
	// las explica si alguna falló: una proyección rota no impide regenerar,
	// pero tiene que decirse.
	MCP    []MCPProjected `json:"mcp,omitempty"`
	MCPErr string         `json:"mcp_error,omitempty"`
}

// cfgRefFormat deduce el formato de una referencia sin mirar el disco: una
// entrada de lista si la referencia la nombra, un archivo si el tipo es de
// archivo y no hay clave, y si no una clave JSON.
func cfgRefFormat(ref ConfigRef) string {
	if ref.Entry != "" {
		return CfgFormatEntry
	}
	if ref.Key == "" && cfgFileType(ref.Type) {
		return CfgFormatText
	}
	return CfgFormatJSON
}

// cfgFileType dice si ese tipo puede ser un archivo suelto. styles y hooks
// están en los dos lados: un .md o un .sh, y también una clave de settings.json.
func cfgFileType(typ string) bool {
	switch typ {
	case CfgTypeInstructions, CfgTypeSkills, CfgTypeAgents, CfgTypeCommands,
		CfgTypeStyles, CfgTypeHooks:
		return true
	}
	return false
}

// cfgArtifactDir es el subdirectorio de cada tipo de artefacto y el nombre del
// archivo dentro, tal como los lee Claude Code.
func cfgArtifactPath(root, typ, name string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("hace falta el nombre del elemento")
	}
	if strings.Contains(name, "..") || filepath.IsAbs(name) {
		return "", fmt.Errorf("nombre no válido: %q", name)
	}
	switch typ {
	case CfgTypeAgents:
		return filepath.Join(root, "agents", name+".md"), nil
	case CfgTypeCommands:
		return filepath.Join(root, "commands", name+".md"), nil
	case CfgTypeStyles:
		return filepath.Join(root, "output-styles", name+".md"), nil
	case CfgTypeSkills:
		return filepath.Join(root, "skills", name, "SKILL.md"), nil
	case CfgTypeHooks:
		return filepath.Join(root, "hooks", name), nil
	}
	return "", fmt.Errorf("el tipo %q no se guarda como archivo", typ)
}

// cfgFileTypeInLayer rechaza crear un archivo que esa capa no lee. Es una
// negativa a propósito y no una comodidad: un hook suelto en el overlay de un
// perfil (que solo proyecta agents, commands, skills y output-styles) o un
// output-style en el .claude de un repo se escribirían sin dar error y sin que
// nadie los leyera nunca, que es la peor forma de perder una configuración.
func cfgFileTypeInLayer(level, typ string) error {
	ok := map[string][]string{
		"global":  {CfgTypeAgents, CfgTypeCommands, CfgTypeSkills, CfgTypeStyles, CfgTypeHooks},
		"profile": {CfgTypeAgents, CfgTypeCommands, CfgTypeSkills, CfgTypeStyles},
		"project": {CfgTypeAgents, CfgTypeCommands, CfgTypeSkills},
	}[level]
	for _, t := range ok {
		if t == typ {
			return nil
		}
	}
	if len(ok) == 0 {
		return fmt.Errorf("la capa %s solo tiene MCP; %s no vive ahí", level, typ)
	}
	return fmt.Errorf("la capa %s no lee %s como archivo (sus tipos: %s)", level, typ, strings.Join(ok, ", "))
}

// cfgTarget es una referencia ya resuelta contra las raíces.
type cfgTarget struct {
	File   string
	Path   []string // camino JSON; vacío = el archivo entero
	Format string
	Layer  ConfigLayer
}

// cfgKeyPath parte la clave del inventario en un camino JSON sin romper los
// nombres que llevan puntos: solo el primer tramo (y el del proyecto) son
// separadores, el resto es el nombre tal cual.
func cfgKeyPath(key string) []string {
	for _, p := range []string{"env.", "hooks.", "mcpServers.", "plugins.", "permissions."} {
		if rest, ok := strings.CutPrefix(key, p); ok {
			return []string{strings.TrimSuffix(p, "."), rest}
		}
	}
	if rest, ok := strings.CutPrefix(key, "projects."); ok {
		if proj, srv, ok := strings.Cut(rest, ".mcpServers."); ok {
			return []string{"projects", proj, "mcpServers", srv}
		}
	}
	if key == "" {
		return nil
	}
	return []string{key}
}

// cfgLayerRoot es el directorio de configuración de una capa: donde viven sus
// artefactos y, con los nombres de siempre, sus archivos.
func cfgLayerRoot(r InventoryRoots, layer ConfigLayer) (string, error) {
	switch layer.Level {
	case "global":
		if r.ClaudeSrc == "" {
			return "", fmt.Errorf("no hay raíz global (ClaudeSrc vacío)")
		}
		return r.ClaudeSrc, nil
	case "profile":
		if r.CCPHome == "" {
			return "", fmt.Errorf("no hay raíz de ccp (CCPHome vacío)")
		}
		return cfgOverlayDir(r.CCPHome, layer.Name), nil
	case "project":
		return filepath.Join(filepath.Clean(layer.Name), ".claude"), nil
	case "desktop":
		dir := cfgDesktopDir(r, layer.Name)
		if dir == "" {
			return "", fmt.Errorf("no se sabe dónde vive la ventana %q", layer.Name)
		}
		return dir, nil
	}
	return "", fmt.Errorf("capa desconocida %q", layer.Level)
}

// cfgSourceFor decide en qué archivo vive (o va a vivir) un elemento cuando la
// referencia no lo trae. Es el mapa de «qué se escribe dónde» de cada capa, y
// está en un solo sitio a propósito: repartirlo entre la GUI y el CLI es cómo
// se acaba escribiendo un MCP de perfil en el cc-home, que la proyección pisa.
func cfgSourceFor(r InventoryRoots, layer ConfigLayer, ref ConfigRef) (string, error) {
	root, err := cfgLayerRoot(r, layer)
	if err != nil {
		return "", err
	}
	if cfgRefFormat(ref) == CfgFormatText {
		if ref.Type == CfgTypeInstructions {
			if layer.Level == "desktop" {
				return "", fmt.Errorf("la capa desktop solo tiene MCP; las instrucciones son del perfil")
			}
			switch layer.Level {
			case "project":
				return filepath.Join(filepath.Clean(layer.Name), "CLAUDE.md"), nil
			default:
				return filepath.Join(root, "CLAUDE.md"), nil
			}
		}
		if err := cfgFileTypeInLayer(layer.Level, ref.Type); err != nil {
			return "", err
		}
		return cfgArtifactPath(root, ref.Type, ref.Name)
	}
	if ref.Type == CfgTypeMCP {
		switch layer.Level {
		case "global":
			return r.ClaudeSrc + ".json", nil
		case "profile":
			return MCPProfileFile(r.CCPHome, layer.Name), nil
		case "project":
			return filepath.Join(filepath.Clean(layer.Name), ".mcp.json"), nil
		case "desktop":
			return filepath.Join(root, "claude_desktop_config.json"), nil
		}
	}
	// La misma negativa que cfgFileTypeInLayer, aquí por la rama JSON: sin
	// ella todo tipo que no sea MCP caía al settings.json genérico y acababa
	// escrito en el data dir de la ventana, que solo aporta
	// claude_desktop_config.json. Nadie lo leería nunca.
	if layer.Level == "desktop" {
		return "", fmt.Errorf("la capa desktop solo tiene MCP; %s no vive ahí", ref.Type)
	}
	if layer.Level == "profile" {
		return cfgSettingsFile(r.CCPHome, layer.Name), nil
	}
	return filepath.Join(root, "settings.json"), nil
}

// cfgRefPath es el camino JSON de una referencia: el de su clave o, si no la
// trae (un elemento que aún no existe), el que le toca por tipo y nombre.
func cfgRefPath(ref ConfigRef) []string {
	if ref.Key != "" {
		return cfgKeyPath(ref.Key)
	}
	switch ref.Type {
	case CfgTypeEnv:
		return []string{"env", ref.Name}
	case CfgTypeMCP:
		return []string{"mcpServers", ref.Name}
	case CfgTypeStatusLine:
		return []string{"statusLine"}
	case CfgTypePermissions:
		if list, _, ok := strings.Cut(ref.Name, ":"); ok {
			return []string{"permissions", list}
		}
	}
	if ref.Name != "" && ref.Type == CfgTypeSettings {
		return []string{ref.Name}
	}
	return nil
}

// cfgResolve convierte una referencia en la dirección que se lee o se escribe.
func cfgResolve(r InventoryRoots, ref ConfigRef) (cfgTarget, error) {
	write, err := cfgWriteLayer(ref.Layer)
	if err != nil {
		// managed y plugin no son capas escribibles, pero SÍ se leen: la
		// pantalla enseña lo que aplica aunque no deje tocarlo.
		if ref.Layer.Level != "managed" && ref.Layer.Level != "plugin" {
			return cfgTarget{}, err
		}
		write = ref.Layer
	}
	t := cfgTarget{File: ref.Source, Format: cfgRefFormat(ref), Layer: write, Path: cfgRefPath(ref)}
	if t.File == "" {
		if t.File, err = cfgSourceFor(r, write, ref); err != nil {
			return cfgTarget{}, err
		}
	}
	return t, nil
}

// ConfigItemGet lee el valor de un elemento. Un archivo o una clave que no
// están no son un error: Exists lo dice, que es lo que el editor necesita para
// abrir en blanco en vez de fallar.
func ConfigItemGet(r InventoryRoots, ref ConfigRef) (ConfigValue, error) {
	t, err := cfgResolve(r, ref)
	if err != nil {
		return ConfigValue{}, err
	}
	v := ConfigValue{Format: t.Format}
	b, err := os.ReadFile(t.File)
	if os.IsNotExist(err) {
		return v, nil
	}
	if err != nil {
		return v, fmt.Errorf("no se pudo leer %s: %w", t.File, err)
	}
	if t.Format == CfgFormatText {
		v.Text, v.Exists = string(b), true
		v.Extras = cfgSkillExtras(t.File)
		return v, nil
	}
	doc, err := decodeOverlayObject(b)
	if err != nil {
		return v, fmt.Errorf("%s no es JSON válido: %w", t.File, err)
	}
	if len(t.Path) == 0 { // el archivo entero (keybindings.json)
		v.JSON, v.Exists = doc, true
		return v, nil
	}
	found, ok := jsonLookup(doc, t.Path)
	if !ok {
		return v, nil
	}
	if t.Format == CfgFormatEntry {
		list, _ := found.([]any)
		for _, e := range list {
			if fmt.Sprint(e) == ref.Entry {
				v.Text, v.Exists = ref.Entry, true
				return v, nil
			}
		}
		return v, nil
	}
	v.JSON, v.Exists = found, true
	return v, nil
}

// cfgEditable rechaza lo que se ve pero no se edita, con la misma frase que la
// lista enseñó. Es la última barrera: la GUI ya no ofrece el botón, pero serve
// acepta cualquier referencia que le llegue.
func cfgEditable(r InventoryRoots, t cfgTarget, ref ConfigRef) error {
	switch t.Layer.Level {
	case "managed":
		return fmt.Errorf("%s: lo fija managed-settings, ccp no lo toca", ref.Name)
	case "plugin":
		return fmt.Errorf("%s: lo trae el plugin %s", ref.Name, t.Layer.Name)
	}
	if ref.Type == CfgTypePlugins && strings.HasPrefix(ref.Key, "plugins.") {
		return fmt.Errorf("%s: %s (enciéndelo o apágalo en enabledPlugins)", ref.Name, cfgWhyInstalled)
	}
	if ref.Type != CfgTypeMCP {
		return nil
	}
	if t.Layer.Level == "desktop" {
		dir := cfgDesktopDir(r, t.Layer.Name)
		if dir != "" && cfgMCPIsManaged(filepath.Join(dir, ".ccp-managed-mcp.json"), ref.Name) {
			return fmt.Errorf("%s lo proyecta ccp en esa ventana; edítalo en %s",
				ref.Name, MCPProfileFile(r.CCPHome, t.Layer.Name))
		}
		return nil
	}
	if t.Layer.Level != "profile" {
		return nil
	}
	// Los dos destinos de la proyección: lo que ccp escribe allí se edita en su
	// capa, o el siguiente sync se lo come sin decir nada.
	if t.File == filepath.Join(ccHomePath(r.CCPHome, t.Layer.Name), ".claude.json") {
		return fmt.Errorf("%s vive en el cc-home, que reescriben Claude Code y la proyección; "+
			"declara los MCP del perfil en %s", ref.Name, MCPProfileFile(r.CCPHome, t.Layer.Name))
	}
	return nil
}

// cfgRefuseClash niega la escritura cuando el destino ya tiene otro contenido.
// No compara contra el disco crudo sino contra ConfigItemGet, que es lo que la
// pantalla enseñaría de ese elemento: una clave dentro de un settings.json no
// choca porque el archivo exista, choca porque la clave esté con otro valor.
// Escribir lo mismo que ya hay no es una pérdida y pasa.
func cfgRefuseClash(r InventoryRoots, t cfgTarget, ref ConfigRef, v ConfigValue) error {
	cur, err := ConfigItemGet(r, ref)
	if err != nil {
		// Un destino ilegible es justo el caso en el que no se pisa a ciegas.
		return err
	}
	if !cur.Exists || cfgSameValue(t.Format, cur, v) {
		return nil
	}
	return fmt.Errorf("%s ya está en %s con otro contenido (%s): míralo primero, y si de verdad quieres reemplazarlo dilo explícitamente",
		ref.Name, cfgLayerWord(t.Layer), t.File)
}

// cfgLayerWord nombra la capa para el mensaje, sin depender del catálogo i18n
// (core no habla de presentación).
func cfgLayerWord(l ConfigLayer) string {
	if l.Name == "" {
		return l.Level
	}
	return l.Level + " " + l.Name
}

// cfgSameValue compara el valor que hay con el que se quiere escribir. El JSON
// pasa por una normalización con UseNumber porque los dos lados llegan de
// decodificadores distintos (el disco y el NDJSON de serve).
func cfgSameValue(format string, cur, v ConfigValue) bool {
	if format == CfgFormatText || format == CfgFormatEntry {
		return cur.Text == v.Text
	}
	return jsonEqual(cfgNormalizeJSON(cur.JSON), cfgNormalizeJSON(v.JSON))
}

func cfgNormalizeJSON(v any) any {
	b, err := json.Marshal(v)
	if err != nil {
		return v
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	var out any
	if err := dec.Decode(&out); err != nil {
		return v
	}
	return out
}

// ConfigItemPutOpts modula la escritura. IfAbsent es la barrera de las acciones
// de capa («llevar a…»): esa ruta no edita lo que hay en el destino, trae lo de
// otra capa, así que pisar un contenido distinto lo borra sin copia —no pasa
// por withSafetySnapshot ni por el rescate del drift—. Editar de frente sigue
// sin barrera: ahí el usuario mira lo que cambia.
type ConfigItemPutOpts struct{ IfAbsent bool }

// ConfigItemPut escribe el valor de un elemento y regenera lo que toque.
// Devuelve el archivo tocado aunque la regeneración falle: el cambio está en
// disco y decir lo contrario sería peor que el fallo.
func ConfigItemPut(r InventoryRoots, ref ConfigRef, v ConfigValue) (ConfigWrite, error) {
	return ConfigItemPutWith(r, ref, v, ConfigItemPutOpts{})
}

// ConfigItemPutWith es ConfigItemPut con opciones.
func ConfigItemPutWith(r InventoryRoots, ref ConfigRef, v ConfigValue, opts ConfigItemPutOpts) (ConfigWrite, error) {
	t, err := cfgResolve(r, ref)
	if err != nil {
		return ConfigWrite{}, err
	}
	if err := cfgEditable(r, t, ref); err != nil {
		return ConfigWrite{}, err
	}
	if err := cfgRefuseProjectSecret(t, v); err != nil {
		return ConfigWrite{}, err
	}
	if opts.IfAbsent {
		if err := cfgRefuseClash(r, t, ref, v); err != nil {
			return ConfigWrite{}, err
		}
	}
	switch t.Format {
	case CfgFormatText:
		if err := cfgWriteText(t.File, v.Text); err != nil {
			return ConfigWrite{}, err
		}
	case CfgFormatEntry:
		entry := v.Text
		if entry == "" && v.JSON != nil {
			entry = fmt.Sprint(v.JSON)
		}
		if entry == "" {
			return ConfigWrite{}, fmt.Errorf("una entrada de %s no puede estar vacía", ref.Key)
		}
		if err := cfgEntryWrite(t, ref.Entry, entry); err != nil {
			return ConfigWrite{}, err
		}
	default:
		if done, err := cfgPutProfileEnv(r, t, v.JSON, false); done {
			if err != nil {
				return ConfigWrite{}, err
			}
			return ConfigWrite{File: t.File, Regenerated: []string{t.Layer.Name}}, nil
		}
		if err := cfgKeyWrite(r, t, v.JSON, false); err != nil {
			return ConfigWrite{}, err
		}
	}
	return cfgAfterWrite(r, t)
}

// ConfigItemDelete quita un elemento. Un archivo de artefacto se borra; una
// clave se quita del JSON; una entrada de lista se saca de la lista.
func ConfigItemDelete(r InventoryRoots, ref ConfigRef) (ConfigWrite, error) {
	t, err := cfgResolve(r, ref)
	if err != nil {
		return ConfigWrite{}, err
	}
	if err := cfgEditable(r, t, ref); err != nil {
		return ConfigWrite{}, err
	}
	switch t.Format {
	case CfgFormatText:
		if err := cfgRemoveText(t.File); err != nil {
			return ConfigWrite{}, err
		}
	case CfgFormatEntry:
		if err := cfgEntryWrite(t, ref.Entry, ""); err != nil {
			return ConfigWrite{}, err
		}
	default:
		if len(t.Path) == 0 {
			return ConfigWrite{}, fmt.Errorf("no se borra un archivo entero desde aquí: %s", t.File)
		}
		if done, err := cfgPutProfileEnv(r, t, nil, true); done {
			if err != nil {
				return ConfigWrite{}, err
			}
			return ConfigWrite{File: t.File, Regenerated: []string{t.Layer.Name}}, nil
		}
		if err := cfgKeyWrite(r, t, nil, true); err != nil {
			return ConfigWrite{}, err
		}
	}
	return cfgAfterWrite(r, t)
}

// cfgWriteText escribe un archivo de texto conservando modo y enlace, como el
// overlay: un CLAUDE.md enlazado a un repo de dotfiles sigue enlazado.
func cfgWriteText(file, text string) error {
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return err
	}
	return writeOverlayPreservingLink(file, []byte(text))
}

// cfgRemoveText borra el archivo de un artefacto. En una skill borra su
// SKILL.md y, si la carpeta queda vacía, también la carpeta: sin SKILL.md ya no
// es una skill, pero los archivos anexos que el usuario dejara ahí no son
// nuestros para borrarlos.
func cfgRemoveText(file string) error {
	if err := os.Remove(file); err != nil && !os.IsNotExist(err) {
		return err
	}
	if filepath.Base(file) == "SKILL.md" {
		_ = os.Remove(filepath.Dir(file))
	}
	return nil
}

// cfgKeyWrite escribe (o borra) una clave. Pasa por SettingsLayerSet cuando el
// archivo ES el settings de la capa, que es el escritor que ya existía; para
// todo lo demás (el .mcp.json de un repo, el config de una ventana) usa el
// mismo motor sin la noción de capa.
func cfgKeyWrite(r InventoryRoots, t cfgTarget, value any, del bool) error {
	if len(t.Path) == 0 {
		return fmt.Errorf("hace falta una clave que escribir en %s", t.File)
	}
	if layer, ok := cfgSettingsLayer(r, t); ok {
		_, err := SettingsLayerSet(r.CCPHome, r.ClaudeSrc, layer, t.Path, value, del)
		return err
	}
	_, err := jsonFileSet(t.File, t.Path, value, del)
	return err
}

// cfgSettingsLayer dice si el archivo es el settings.json de una capa editable.
func cfgSettingsLayer(r InventoryRoots, t cfgTarget) (SettingsLayer, bool) {
	switch t.Layer.Level {
	case "global":
		if r.ClaudeSrc != "" && t.File == filepath.Join(r.ClaudeSrc, "settings.json") {
			return "global", true
		}
	case "profile":
		if r.CCPHome != "" && t.File == cfgSettingsFile(r.CCPHome, t.Layer.Name) {
			return SettingsLayer("profile:" + t.Layer.Name), true
		}
	}
	return "", false
}

// cfgPutProfileEnv desvía el env de un perfil a OverlayEnvSet/Del, que además de
// escribir VALIDA el overlay resultante y regenera. El segundo valor dice si se
// ocupó del caso. El env de un perfil es lo único que tiene escritor propio
// porque es lo único que puede dejar un perfil sin arrancar.
func cfgPutProfileEnv(r InventoryRoots, t cfgTarget, value any, del bool) (bool, error) {
	if t.Layer.Level != "profile" || len(t.Path) != 2 || t.Path[0] != "env" {
		return false, nil
	}
	if r.CCPHome == "" {
		return true, fmt.Errorf("no hay raíz de ccp (CCPHome vacío)")
	}
	if del {
		return true, overlayEnvDel(r.CCPHome, t.Layer.Name, r.ClaudeSrc, t.Path[1])
	}
	s, ok := value.(string)
	if !ok {
		return true, fmt.Errorf("env.%s tiene que ser texto, no %T", t.Path[1], value)
	}
	return true, overlayEnvSet(r.CCPHome, t.Layer.Name, r.ClaudeSrc, t.Path[1], s)
}

// cfgEntryWrite reescribe una lista de permisos cambiando una entrada: old por
// want, want sola si old está vacía, y sin want se quita old. La lista entera es la
// única operación que el archivo admite, igual que con los hooks.
func cfgEntryWrite(t cfgTarget, old, want string) error {
	// Una referencia con Entry pero sin Key (o de un tipo que no es una lista
	// dentro de un JSON) deja el Path vacío, y jsonSetPath no admite uno vacío.
	// serve acepta cualquier referencia que le llegue, así que la comprobación
	// va aquí, como en cfgKeyWrite: un error de parámetro, no un panic.
	if len(t.Path) == 0 {
		return fmt.Errorf("hace falta una clave de lista donde escribir la entrada en %s", t.File)
	}
	b, err := os.ReadFile(t.File)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	doc, err := decodeOverlayObject(b)
	if err != nil {
		return fmt.Errorf("%s no es JSON válido; no se toca: %w", t.File, err)
	}
	cur, _ := jsonLookup(doc, t.Path)
	list, _ := cur.([]any)
	out := make([]any, 0, len(list)+1)
	replaced := false
	for _, e := range list {
		if old != "" && fmt.Sprint(e) == old {
			if want != "" {
				out, replaced = append(out, want), true
			}
			continue
		}
		out = append(out, e)
	}
	if want != "" && !replaced {
		out = append(out, want)
	}
	_, err = jsonFileSet(t.File, t.Path, out, false)
	return err
}

// cfgAfterWrite regenera lo que el cambio deja desfasado y dice a quién tocó.
// La regla vive aquí y en ningún otro sitio: lo generado (cc-home/settings.json,
// cc-home/CLAUDE.md, la proyección de MCP y de artefactos) sale de las capas,
// así que tocar una capa sin regenerar deja al perfil leyendo lo de antes.
func cfgAfterWrite(r InventoryRoots, t cfgTarget) (ConfigWrite, error) {
	names, err := cfgRegenTargets(r, t)
	if err != nil {
		return ConfigWrite{File: t.File, Regenerated: []string{}}, err
	}
	return cfgRegenerateAll(r, t.File, names)
}

// cfgRegenerateAll regenera esos perfiles y recoge sus proyecciones de MCP. Lo
// usan las escrituras de archivo (cfgAfterWrite) y las que no tocan ningún
// archivo de capa sino el bloque `mcp:` de ccp.yaml (destinos y apagados, C2):
// las dos dejan desfasado lo generado, así que las dos terminan aquí.
//
// La deriva de /config que encuentre la regeneración se queda pendiente como
// siempre (nadie tira un informe), menos la parte de MCP: esa se DEVUELVE, y
// dejarla además pendiente la enseñaría dos veces.
func cfgRegenerateAll(r InventoryRoots, file string, names []string) (ConfigWrite, error) {
	w := ConfigWrite{File: file, Regenerated: []string{}}
	for _, n := range names {
		d, err := CfgRegenerateReport(r.CCPHome, n, r.ClaudeSrc)
		for _, p := range d.MCP {
			w.MCP = append(w.MCP, MCPProjected{Profile: n, MCPProjection: p.normalized()})
		}
		if d.MCPErr != "" && w.MCPErr == "" {
			w.MCPErr = d.MCPErr
		}
		rest := d
		rest.MCP, rest.MCPErr = nil, ""
		if !rest.Empty() {
			_ = savePendingDrift(r.CCPHome, rest)
		}
		if err != nil {
			return w, fmt.Errorf("se escribió %s pero %s quedó sin regenerar: %w — reinténtalo con: ccp profile sync %s",
				file, n, err, n)
		}
		w.Regenerated = append(w.Regenerated, n)
	}
	return w, nil
}

// cfgRegenTargets son los perfiles que hay que regenerar tras tocar un archivo.
// Un cambio en el global los alcanza a TODOS (su settings generado es
// global ⊕ overlay); uno en un overlay, solo al suyo; los de proyecto y los de
// una ventana de Desktop no generan nada.
func cfgRegenTargets(r InventoryRoots, t cfgTarget) ([]string, error) {
	if r.CCPHome == "" || r.ClaudeSrc == "" {
		return nil, nil
	}
	switch t.Layer.Level {
	case "profile":
		if t.Layer.Name == "" || t.Layer.Name == "default" {
			return nil, nil
		}
		if !strings.HasPrefix(t.File, cfgOverlayDir(r.CCPHome, t.Layer.Name)+string(filepath.Separator)) {
			return nil, nil
		}
		return []string{t.Layer.Name}, nil
	case "global":
		if !cfgGlobalFeedsProfiles(r.ClaudeSrc, t.File) {
			return nil, nil
		}
		cfg, err := Load(r.CCPHome)
		if err != nil {
			// Sin ccp.yaml no hay perfiles que regenerar; con uno ilegible,
			// tampoco se puede saber, y callarlo sería decir que no hacía falta.
			if os.IsNotExist(err) {
				return nil, nil
			}
			return nil, err
		}
		return invSortedProfiles(cfg), nil
	}
	return nil, nil
}

// cfgGlobalFeedsProfiles dice si ese archivo del global alimenta lo que ccp
// genera en cada perfil: los dos que se fusionan (settings.json y CLAUDE.md),
// el .claude.json del que salen los MCP de la capa global y los árboles de
// artefactos, que se espejan en el cc-home de quien declare los suyos.
func cfgGlobalFeedsProfiles(src, file string) bool {
	if file == filepath.Join(src, "settings.json") || file == filepath.Join(src, "CLAUDE.md") || file == src+".json" {
		return true
	}
	for _, d := range profileArtifactDirs {
		if strings.HasPrefix(file, filepath.Join(src, d)+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

// cfgSecretName dice si el nombre de una variable suena a credencial, con las
// mismas palabras que usa la barrera de los MCP: una sola definición de «esto
// es un secreto» para los dos caminos.
func cfgSecretName(name string) bool {
	l := strings.ToLower(name)
	for _, w := range mcpSecretWords {
		if strings.Contains(l, w) {
			return true
		}
	}
	return false
}

// cfgRefuseProjectSecret niega escribir un env.* con pinta de credencial en la
// capa de proyecto. Ahí el destino es <repo>/.claude/settings.json, que viaja
// en el commit: si además se elige «quitarlo» del origen, la única copia del
// token queda en un archivo versionado. Es la hermana de la barrera de MCPPut
// (mcp_crud.go), y vive en core porque serve acepta cualquier referencia que le
// llegue, no solo las que ofrece la GUI.
func cfgRefuseProjectSecret(t cfgTarget, v ConfigValue) error {
	if t.Layer.Level != "project" || len(t.Path) == 0 || t.Path[0] != "env" {
		return nil
	}
	vals := map[string]any{}
	switch {
	case len(t.Path) >= 2:
		vals[t.Path[1]] = v.JSON
	default:
		m, _ := v.JSON.(map[string]any)
		for k, e := range m {
			vals[k] = e
		}
	}
	bad := []string{}
	for _, k := range invSortedKeys(vals) {
		s, _ := vals[k].(string)
		if s == "" || mcpFromEnvironment(s) || !cfgSecretName(k) {
			continue
		}
		bad = append(bad, "env."+k)
	}
	if len(bad) == 0 {
		return nil
	}
	return fmt.Errorf("%s: %s van en claro y el settings.json del repo viaja en el commit; "+
		"escribe ${VARIABLE} y deja el valor en tu entorno, o declara esto en un perfil",
		cfgLayerWord(t.Layer), strings.Join(bad, ", "))
}

// cfgSkillExtras lista lo que hay en la carpeta de una skill además de su
// SKILL.md, en rutas relativas a la carpeta y ordenadas. Un error al mirar se
// traga a propósito: esto informa, y un Get que fallara por no poder listar un
// directorio dejaría el editor sin abrir por algo accesorio.
func cfgSkillExtras(file string) []string {
	if filepath.Base(file) != "SKILL.md" {
		return nil
	}
	dir := filepath.Dir(file)
	var out []string
	_ = filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || p == file {
			return nil //nolint:nilerr // un anexo que no se puede mirar no es un fallo del Get
		}
		if rel, err := filepath.Rel(dir, p); err == nil {
			out = append(out, filepath.ToSlash(rel))
		}
		return nil
	})
	sort.Strings(out)
	return out
}
