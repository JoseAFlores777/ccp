package core

// mcp_crud.go — alta, baja, edición, destinos y apagado de un MCP (spec §7, C2).
//
// Tres capas DECLARAN servidores y ninguna otra: la global (~/.claude.json), el
// perfil (overlay/mcp.json) y el proyecto (.mcp.json). El cc-home de un perfil y
// el claude_desktop_config.json de una ventana son DESTINOS de la proyección
// (§6.1), así que escribir ahí lo borraría el siguiente sync sin decir nada: se
// niega y se dice dónde va.
//
// Nada se escribe a mano. Se valida la FORMA antes de tocar el archivo (Claude
// Code descarta en silencio la entrada que no entiende, y el usuario se queda
// sin saber por qué ese servidor no aparece), se mira dónde NO deben ir los
// secretos y se enruta por ConfigItemPut/Delete, el escritor de C1, que ya sabe
// en qué archivo vive cada capa y a quién hay que regenerar. Lo que solo le
// importa a ccp —destinos y apagados— vive en el bloque `mcp:` de ccp.yaml y se
// escribe con Load → mutar → Save, como el resto de core.
//
// Toda escritura termina en la proyección y devuelve lo que hizo, para que el
// front pueda avisar de «pendiente de reiniciar ventana» (RestartPending).

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
)

// MCPProjected es una proyección con el perfil al que pertenece. MCPProjection
// dice el destino (cli o desktop) y el archivo, pero no de quién: escribir en la
// capa global regenera varios perfiles a la vez.
type MCPProjected struct {
	Profile string `json:"profile"`
	MCPProjection
}

// RestartPending son los perfiles cuya ventana de Desktop quedó con proyección
// aplazada: con la ventana viva no se escribe su config (ADR 0016 M5) y, aunque
// se escribiera, Desktop no la relee en caliente (M3). Hasta que se reinicie, el
// chat sigue con los MCP de antes, y eso hay que decirlo.
func (w ConfigWrite) RestartPending() []string {
	out := []string{}
	for _, p := range w.MCP {
		if p.Deferred && !slices.Contains(out, p.Profile) {
			out = append(out, p.Profile)
		}
	}
	return out
}

// mcpNameRe es lo que vale como clave de mcpServers. El nombre no es solo una
// clave: Claude Code lo usa para nombrar las herramientas del servidor
// (mcp__<nombre>__<tool>), así que un espacio o una barra dan un servidor que se
// guarda bien y no se puede invocar.
var mcpNameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._+@:-]*$`)

func mcpValidName(name string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("hace falta el nombre del servidor")
	}
	if !mcpNameRe.MatchString(name) {
		return fmt.Errorf("nombre de servidor no válido: %q (letras, números y . _ - + @ :, empezando por letra o número)", name)
	}
	return nil
}

// mcpDeclaredType devuelve el type declarado ("" si no lo hay) y rechaza el que
// no existe: un transporte inventado deja el servidor fuera sin avisar.
func mcpDeclaredType(name string, def map[string]any) (string, error) {
	v, has := def["type"]
	if !has {
		return "", nil
	}
	t, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("%s: type tiene que ser texto", name)
	}
	switch t {
	case "", "stdio", "http", "sse":
		return t, nil
	}
	return "", fmt.Errorf("%s: type %q no existe (valen: stdio, http, sse)", name, t)
}

// mcpStringField lee un campo de texto opcional.
func mcpStringField(name string, def map[string]any, key string) (string, error) {
	v, has := def[key]
	if !has {
		return "", nil
	}
	s, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("%s: %s tiene que ser texto", name, key)
	}
	return s, nil
}

// mcpStringSlice valida una lista de textos (args).
func mcpStringSlice(name string, def map[string]any, key string) error {
	v, has := def[key]
	if !has {
		return nil
	}
	list, ok := v.([]any)
	if !ok {
		return fmt.Errorf("%s: %s tiene que ser una lista de textos", name, key)
	}
	for i, e := range list {
		if _, ok := e.(string); !ok {
			return fmt.Errorf("%s: %s[%d] tiene que ser texto", name, key, i)
		}
	}
	return nil
}

// mcpStringMap valida un mapa de textos (env, headers). Claude Code los pasa al
// proceso o a la petición tal cual: un número ahí no es un valor, es un error
// que solo se ve cuando el servidor no arranca.
func mcpStringMap(name string, def map[string]any, key string) error {
	v, has := def[key]
	if !has {
		return nil
	}
	m, ok := v.(map[string]any)
	if !ok {
		return fmt.Errorf("%s: %s tiene que ser un objeto de textos", name, key)
	}
	for _, k := range invSortedKeys(m) {
		if _, ok := m[k].(string); !ok {
			return fmt.Errorf("%s: %s.%s tiene que ser texto", name, key, k)
		}
	}
	return nil
}

// ValidateMCPServer valida la forma de un servidor: stdio (command + args + env)
// o remoto (http/sse, url + headers). Se valida ANTES de escribir porque una
// entrada mal formada no rompe nada visible — Claude Code la descarta y sigue —,
// así que el único momento en que se puede decir qué está mal es este.
//
// Mezclar las dos formas también se rechaza aunque el archivo lo admita: un
// command en una entrada http no lo ejecuta nadie, y guardarlo es prometer una
// configuración que no existe.
func ValidateMCPServer(name string, def map[string]any) error {
	if err := mcpValidName(name); err != nil {
		return err
	}
	if len(def) == 0 {
		return fmt.Errorf("%s: la definición está vacía; un servidor es un command (stdio) o una url (http/sse)", name)
	}
	typ, err := mcpDeclaredType(name, def)
	if err != nil {
		return err
	}
	cmd, err := mcpStringField(name, def, "command")
	if err != nil {
		return err
	}
	u, err := mcpStringField(name, def, "url")
	if err != nil {
		return err
	}
	if cmd != "" && u != "" {
		return fmt.Errorf("%s: trae command y url a la vez; un servidor es stdio o remoto, no las dos cosas", name)
	}
	// Sin type declarado, el transporte se deduce como lo deduce la proyección
	// (mcpKind): con command es stdio y con url es http. Deducirlo de otra
	// manera aquí haría que ccp rechazara entradas que él mismo proyecta bien.
	kind := typ
	if kind == "" {
		kind = mcpKind(def)
	}
	if kind == "stdio" {
		return mcpValidStdio(name, def, cmd, u)
	}
	return mcpValidRemote(name, def, kind, cmd, u)
}

func mcpValidStdio(name string, def map[string]any, cmd, raw string) error {
	if cmd == "" && raw != "" {
		// Declarado stdio pero con url: el transporte es lo que decide si se
		// ejecuta un proceso o se llama a un servidor, y stdio no llama a nadie.
		return fmt.Errorf("%s: trae url pero se declara stdio; si es remoto, ponle type http o sse", name)
	}
	if cmd == "" {
		return fmt.Errorf("%s: un servidor stdio necesita command (o una url, si es http/sse)", name)
	}
	if _, has := def["headers"]; has {
		return fmt.Errorf("%s: headers es de los servidores http/sse; un stdio lo ignora, así que no se guarda", name)
	}
	if err := mcpStringSlice(name, def, "args"); err != nil {
		return err
	}
	return mcpStringMap(name, def, "env")
}

func mcpValidRemote(name string, def map[string]any, typ, cmd, raw string) error {
	if cmd != "" {
		return fmt.Errorf("%s: un servidor %s no lleva command", name, typ)
	}
	if _, has := def["args"]; has {
		return fmt.Errorf("%s: args es de los servidores stdio", name)
	}
	if _, has := def["env"]; has {
		return fmt.Errorf("%s: env es de los servidores stdio; las credenciales de uno remoto van en headers", name)
	}
	if raw == "" {
		return fmt.Errorf("%s: un servidor %s necesita url", name, typ)
	}
	p, err := url.Parse(raw)
	if err != nil || (p.Scheme != "http" && p.Scheme != "https") || p.Host == "" {
		return fmt.Errorf("%s: url tiene que ser http(s), y es %q", name, raw)
	}
	return mcpStringMap(name, def, "headers")
}

// mcpSecretWords son los nombres que delatan una credencial en env o headers.
// Es una heurística por el NOMBRE y no por el valor a propósito: acertar sobre
// el valor es imposible, y un falso positivo se arregla con ${VARIABLE}, que es
// lo que había que escribir de todas formas.
var mcpSecretWords = []string{"token", "key", "secret", "password", "passwd",
	"auth", "credential", "cookie", "bearer", "pat", "session"}

// mcpExpansionRe es la expansión que Claude Code resuelve al arrancar el
// servidor: ${VARIABLE}, tomada del entorno de quien lo ejecuta.
var mcpExpansionRe = regexp.MustCompile(`\$\{[A-Za-z_][A-Za-z0-9_]*\}`)

// mcpFromEnvironment dice si el valor lo pone el entorno y no el archivo. Un
// «${VAR:-por defecto}» NO cuenta: el valor por defecto se queda escrito, que es
// justo lo que se intenta evitar.
func mcpFromEnvironment(v string) bool {
	return mcpExpansionRe.MatchString(v) && !strings.Contains(v, ":-")
}

// mcpSecretLiterals devuelve los env.X / headers.Y cuyo nombre suena a
// credencial y cuyo valor está escrito en claro en la definición.
func mcpSecretLiterals(def map[string]any) []string {
	out := []string{}
	for _, sect := range []string{"env", "headers"} {
		m, _ := def[sect].(map[string]any)
		for _, k := range invSortedKeys(m) {
			s, _ := m[k].(string)
			if s == "" || mcpFromEnvironment(s) {
				continue
			}
			l := strings.ToLower(k)
			for _, w := range mcpSecretWords {
				if strings.Contains(l, w) {
					out = append(out, sect+"."+k)
					break
				}
			}
		}
	}
	return out
}

// mcpDeclaredLayer comprueba que la capa DECLARA servidores y devuelve la de
// escritura (default escribe en la global, como en todo el editor).
//
// Que la capa EXISTA se mira aquí y no al escribir: el escritor crea los
// directorios que le falten, así que un perfil mal escrito se llevaría su
// servidor a un profiles/<typo>/overlay/mcp.json que no va a leer nadie, y una
// carpeta de proyecto mal escrita se crearía entera. Las dos cosas se guardan
// sin error y se pierden en silencio, que es lo peor que puede pasarle a una
// configuración.
func mcpDeclaredLayer(r InventoryRoots, layer ConfigLayer) (ConfigLayer, error) {
	switch layer.Level {
	case "managed":
		return ConfigLayer{}, fmt.Errorf("los MCP de managed-settings los fija la organización: ccp no los toca")
	case "plugin":
		return ConfigLayer{}, fmt.Errorf("esos MCP los trae el plugin %s: se encienden o se apagan, no se editan", layer.Name)
	}
	write, err := cfgWriteLayer(layer)
	if err != nil {
		return ConfigLayer{}, err
	}
	switch write.Level {
	case "desktop":
		return ConfigLayer{}, fmt.Errorf("la ventana de %s recibe los MCP del perfil, no los declara: "+
			"decláralo en el perfil %s con el destino %q y ccp lo proyecta al chat",
			write.Name, write.Name, MCPTargetDesktop)
	case "profile":
		if r.CCPHome == "" {
			return ConfigLayer{}, fmt.Errorf("no hay raíz de ccp (CCPHome vacío)")
		}
		cfg, err := Load(r.CCPHome)
		if err != nil {
			return ConfigLayer{}, err
		}
		if _, ok := cfg.Profiles[write.Name]; !ok {
			return ConfigLayer{}, fmt.Errorf("no existe el perfil %q: su overlay no lo leería nadie", write.Name)
		}
	case "project":
		dir := filepath.Clean(write.Name)
		if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
			return ConfigLayer{}, fmt.Errorf("no existe la carpeta del proyecto %s", dir)
		}
	}
	return write, nil
}

// MCPPut da de alta o edita un servidor en la capa que lo declara. Un .mcp.json
// viaja en el repo: un secreto en claro ahí se publica con el commit, así que
// en la capa de proyecto se niega y se dice cómo escribirlo.
func MCPPut(r InventoryRoots, layer ConfigLayer, name string, def map[string]any) (ConfigWrite, error) {
	write, err := mcpDeclaredLayer(r, layer)
	if err != nil {
		return ConfigWrite{}, err
	}
	if err := ValidateMCPServer(name, def); err != nil {
		return ConfigWrite{}, err
	}
	if write.Level == "project" {
		if bad := mcpSecretLiterals(def); len(bad) > 0 {
			return ConfigWrite{}, fmt.Errorf("%s: %s van en claro y el .mcp.json viaja en el repo; "+
				"escribe ${VARIABLE} y deja el valor en tu entorno, o declara el servidor en un perfil",
				name, strings.Join(bad, ", "))
		}
	}
	return ConfigItemPut(r, ConfigRef{Layer: write, Type: CfgTypeMCP, Name: name}, ConfigValue{JSON: def})
}

// MCPDelete quita un servidor de la capa que lo declara y lo retira de los
// destinos a los que ccp lo había proyectado.
//
// Sus destinos y sus apagados en ccp.yaml se quedan: el mismo nombre puede
// seguir declarado en otra capa (un perfil que tapa al global), y desde aquí no
// se puede distinguir «ya no existe» de «ya no lo declara ESTA capa».
func MCPDelete(r InventoryRoots, layer ConfigLayer, name string) (ConfigWrite, error) {
	write, err := mcpDeclaredLayer(r, layer)
	if err != nil {
		return ConfigWrite{}, err
	}
	if err := mcpValidName(name); err != nil {
		return ConfigWrite{}, err
	}
	return ConfigItemDelete(r, ConfigRef{Layer: write, Type: CfgTypeMCP, Name: name})
}

// mcpBlock devuelve el bloque `mcp:` de ccp.yaml, creándolo si no estaba.
func mcpBlock(cfg *Config) *MCPConfig {
	if cfg.MCP == nil {
		cfg.MCP = &MCPConfig{}
	}
	return cfg.MCP
}

// mcpTrimBlock quita el bloque cuando se queda sin nada que decir: un `mcp: {}`
// en ccp.yaml es ruido, y lo que vale por defecto no necesita escribirse.
func mcpTrimBlock(cfg *Config) {
	m := cfg.MCP
	if m != nil && len(m.Targets) == 0 && len(m.Disabled) == 0 && len(m.Extra) == 0 && !m.DesktopDefault {
		cfg.MCP = nil
	}
}

// mcpDefaultTargets es lo que vale un servidor sin entrada de destinos.
var mcpDefaultTargets = []string{MCPTargetCLI, MCPTargetDesktop}

// mcpNormalizeTargets valida y ordena los destinos, sin repetidos: la lista se
// compara con la de por defecto, y [desktop cli] y [cli desktop] son la misma.
func mcpNormalizeTargets(ts []string) ([]string, error) {
	if err := ValidateMCPTargets(ts); err != nil {
		return nil, err
	}
	out := []string{}
	for _, t := range mcpDefaultTargets {
		if slices.Contains(ts, t) {
			out = append(out, t)
		}
	}
	return out, nil
}

// mcpProfilesToRegen son los perfiles que deja desfasados un cambio del bloque
// `mcp:`: todos si cambian los destinos (cualquiera puede llevar ese servidor),
// solo el suyo si es un apagado. Sin raíces no se regenera nada, igual que hace
// cfgRegenTargets con los archivos.
func mcpProfilesToRegen(r InventoryRoots, cfg *Config, only string) []string {
	if r.CCPHome == "" || r.ClaudeSrc == "" {
		return nil
	}
	if only != "" {
		return []string{only}
	}
	return invSortedProfiles(cfg)
}

// mcpSaveAndRegen guarda ccp.yaml y regenera. El archivo que se devuelve es el
// ccp.yaml: es donde vive el cambio, y el front enseña la ruta que tocó.
func mcpSaveAndRegen(r InventoryRoots, cfg *Config, names []string) (ConfigWrite, error) {
	if err := Save(r.CCPHome, cfg); err != nil {
		return ConfigWrite{}, err
	}
	return cfgRegenerateAll(r, yamlPath(r.CCPHome), names)
}

// MCPSetTargets fija a dónde va un servidor: al CLI y a la pestaña Code (cli),
// al chat de Desktop (desktop), a los dos o a ninguno. Declarar los dos borra la
// entrada, porque sin entrada ya valen los dos.
func MCPSetTargets(r InventoryRoots, name string, targets []string) (ConfigWrite, error) {
	if err := mcpValidName(name); err != nil {
		return ConfigWrite{}, err
	}
	ts, err := mcpNormalizeTargets(targets)
	if err != nil {
		return ConfigWrite{}, err
	}
	if r.CCPHome == "" {
		return ConfigWrite{}, fmt.Errorf("no hay raíz de ccp (CCPHome vacío)")
	}
	cfg, err := Load(r.CCPHome)
	if err != nil {
		return ConfigWrite{}, err
	}
	block := mcpBlock(cfg)
	if slices.Equal(ts, mcpDefaultTargets) {
		delete(block.Targets, name)
	} else {
		if block.Targets == nil {
			block.Targets = map[string][]string{}
		}
		block.Targets[name] = ts
	}
	mcpTrimBlock(cfg)
	return mcpSaveAndRegen(r, cfg, mcpProfilesToRegen(r, cfg, ""))
}

// MCPSetEnabled apaga (o vuelve a encender) en UN perfil un servidor heredado,
// sin borrarlo de la capa que lo declara: los demás perfiles lo siguen viendo.
func MCPSetEnabled(r InventoryRoots, profile, name string, enabled bool) (ConfigWrite, error) {
	if err := mcpValidName(name); err != nil {
		return ConfigWrite{}, err
	}
	if profile == "" {
		return ConfigWrite{}, fmt.Errorf("hace falta el perfil en el que apagar %s", name)
	}
	if profile == "default" {
		return ConfigWrite{}, fmt.Errorf("default lee ~/.claude.json directamente y no tiene proyección que apagar: "+
			"quita %s de la capa global o dale otros destinos", name)
	}
	if r.CCPHome == "" {
		return ConfigWrite{}, fmt.Errorf("no hay raíz de ccp (CCPHome vacío)")
	}
	cfg, err := Load(r.CCPHome)
	if err != nil {
		return ConfigWrite{}, err
	}
	if _, ok := cfg.Profiles[profile]; !ok {
		return ConfigWrite{}, fmt.Errorf("no existe el perfil %q", profile)
	}
	block := mcpBlock(cfg)
	list := append([]string(nil), block.Disabled[profile]...)
	if enabled {
		list = slices.DeleteFunc(list, func(s string) bool { return s == name })
	} else if !slices.Contains(list, name) {
		list = append(list, name)
	}
	sort.Strings(list)
	if len(list) == 0 {
		delete(block.Disabled, profile)
	} else {
		if block.Disabled == nil {
			block.Disabled = map[string][]string{}
		}
		block.Disabled[profile] = list
	}
	mcpTrimBlock(cfg)
	return mcpSaveAndRegen(r, cfg, mcpProfilesToRegen(r, cfg, profile))
}
