package cli

// mcp.go — `ccp mcp` (spec 2026-09-18 §7, C3): la cara de terminal del editor
// de MCP. No decide nada: el mapa de «qué capa declara qué» vive en core
// (mcp_crud.go, C2) y lo que se lista sale de ConfigItems (C1). Aquí se
// construyen las raíces reales, se traduce la línea de comandos a una
// ConfigLayer y se pinta.
//
// No entra en la completion, como backup, serve y snapshot: ese texto es
// contrato golden y añadir una rama obliga a mover el oráculo bash.

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/JoseAFlores777/ccp/internal/core"
	"github.com/JoseAFlores777/ccp/internal/core/i18n"
)

// mcpCtx es lo que necesita cualquier subcomando: el idioma, la raíz de ccp,
// las raíces de la máquina y el ccp.yaml ya cargado.
type mcpCtx struct {
	lang  i18n.Lang
	home  string
	roots core.InventoryRoots
	cfg   *core.Config
}

func mcpSetup(stderr io.Writer) (mcpCtx, bool) {
	c := mcpCtx{lang: currentLang(), home: resolveHome()}
	if err := ensureMigrated(c.home); err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return c, false
	}
	r, err := inventoryRoots(c.home)
	if err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return c, false
	}
	cfg, err := core.Load(c.home)
	if err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return c, false
	}
	c.roots, c.cfg = r, cfg
	return c, true
}

// mcpScope traduce `--scope`. Sin valor es el perfil ACTIVO de la terminal y no
// la global: quien teclea esto ya está dentro de un perfil, y proponerle la capa
// global por defecto es cómo se acaba declarando en todas partes lo que solo
// quería una. `profile`, `project` y `desktop` sin nombre toman el activo, el
// repo del cwd y la ventana de `default`.
func mcpScope(c mcpCtx, spec string) (core.ConfigLayer, error) {
	if spec == "" {
		return core.ConfigLayer{Level: "profile", Name: activeProfile(c.home, currentDir())}, nil
	}
	level, name, _ := strings.Cut(spec, ":")
	switch level {
	case "global":
		if name != "" {
			return core.ConfigLayer{}, fmt.Errorf("%s", i18n.T(c.lang, "cli.mcp.bad_scope", spec))
		}
		return core.ConfigLayer{Level: "global"}, nil
	case "profile", "perfil":
		if name == "" {
			name = activeProfile(c.home, currentDir())
		}
		return core.ConfigLayer{Level: "profile", Name: name}, nil
	case "project", "proyecto":
		if name == "" {
			name = repoRoot()
		}
		return core.ConfigLayer{Level: "project", Name: core.NormalizePath(name)}, nil
	case "desktop":
		if name == "" {
			name = "default"
		}
		return core.ConfigLayer{Level: "desktop", Name: name}, nil
	}
	return core.ConfigLayer{}, fmt.Errorf("%s", i18n.T(c.lang, "cli.mcp.bad_scope", spec))
}

// mcpLayerLabel es como se nombra una capa en pantalla y en el JSON: el nivel, y
// el nombre detrás de dos puntos cuando lo tiene. Es la misma sintaxis que
// acepta --scope, para poder copiar la fila y usarla.
func mcpLayerLabel(l core.ConfigLayer) string {
	if l.Name == "" {
		return l.Level
	}
	return l.Level + ":" + l.Name
}

func dispatchMCP(args []string, stdout, stderr io.Writer) int {
	lang := currentLang()
	var sub string
	var rest []string
	if len(args) > 0 {
		sub, rest = args[0], args[1:]
	}
	switch sub {
	case "list", "ls":
		return mcpCmdList(rest, stdout, stderr)
	case "add", "set", "put":
		return mcpCmdAdd(rest, stdout, stderr)
	case "rm", "remove", "delete":
		return mcpCmdRm(rest, stdout, stderr)
	case "enable":
		return mcpCmdSwitch(rest, true, stdout, stderr)
	case "disable":
		return mcpCmdSwitch(rest, false, stdout, stderr)
	case "targets":
		return mcpCmdTargets(rest, stdout, stderr)
	case "adopt":
		return mcpCmdAdopt(rest, stdout, stderr)
	case "help", "--help", "-h":
		fmt.Fprintln(stdout, i18n.T(lang, "cli.mcp.usage"))
		return 0
	case "":
		return mcpCmdList(nil, stdout, stderr)
	}
	fmt.Fprintln(stderr, i18n.T(lang, "cli.mcp.unknown_sub", sub))
	fmt.Fprintln(stderr, i18n.T(lang, "cli.mcp.usage"))
	return 1
}

// mcpRow es la forma estable de una fila de `ccp mcp list --json`. Targets y
// AppliesTo son siempre arrays: una lista vacía es «a ningún sitio», que es una
// respuesta, y `null` no lo es.
type mcpRow struct {
	Scope     string   `json:"scope"`
	Name      string   `json:"name"`
	Type      string   `json:"type"`
	Detail    string   `json:"detail,omitempty"`
	Source    string   `json:"source"`
	Targets   []string `json:"targets"`
	AppliesTo []string `json:"applies_to"`
	Editable  bool     `json:"editable"`
	Disabled  bool     `json:"disabled,omitempty"`
	Why       string   `json:"why,omitempty"`
	Missing   string   `json:"missing,omitempty"`
}

// mcpDetail resume la definición en una línea: el comando o la url. Es lo que
// distingue dos servidores con nombres parecidos sin abrir el archivo.
func mcpDetail(def map[string]any) string {
	if u, _ := def["url"].(string); u != "" {
		return u
	}
	cmd, _ := def["command"].(string)
	if args, ok := def["args"].([]any); ok {
		for _, a := range args {
			cmd += " " + fmt.Sprint(a)
		}
	}
	return strings.TrimSpace(cmd)
}

// mcpDefOf lee la definición de un elemento ya listado. Que no se pueda leer no
// es un error de la lista: se enseña la fila sin detalle, porque el nombre y su
// origen siguen siendo información buena.
func mcpDefOf(c mcpCtx, ref core.ConfigRef) map[string]any {
	v, err := core.ConfigItemGet(c.roots, ref)
	if err != nil {
		return nil
	}
	d, _ := v.JSON.(map[string]any)
	return d
}

// mcpRowsFor lista los MCP de una capa. Sale de ConfigItems (C1) para no tener
// dos ideas distintas de «qué hay aquí», y añade lo único que esa lista no
// sabe: los destinos y los apagados, que viven en ccp.yaml y no en ningún
// archivo de Claude Code.
func mcpRowsFor(c mcpCtx, layer core.ConfigLayer) ([]mcpRow, error) {
	list, err := core.ConfigItems(c.roots, layer)
	if err != nil {
		return nil, err
	}
	rows := []mcpRow{}
	seen := map[string]bool{}
	for _, it := range list.Items {
		if it.Ref.Type != core.CfgTypeMCP {
			continue
		}
		def := mcpDefOf(c, it.Ref)
		seen[it.Name] = true
		rows = append(rows, mcpRow{
			Scope: mcpLayerLabel(it.Ref.Layer), Name: it.Name,
			Type: core.MCPEntry{Def: def}.Kind(), Detail: mcpDetail(def), Source: it.Ref.Source,
			Targets: core.MCPTargets(c.cfg, it.Name), AppliesTo: it.AppliesTo,
			Editable: it.Editable, Disabled: core.MCPDisabled(c.cfg, layer.Name, it.Name),
			Why: it.Why, Missing: it.Missing,
		})
	}
	rows = append(rows, mcpUnprojectedRows(c, layer, seen)...)
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].Name < rows[j].Name })
	return rows, nil
}

// mcpAppliesFor traduce los destinos al vocabulario del inventario, para que
// una fila que ccp no proyectó al cc-home diga igualmente dónde acaba.
func mcpAppliesFor(targets []string) []string {
	out := []string{}
	for _, t := range targets {
		if t == core.MCPTargetCLI {
			out = append(out, core.InvAppliesCLI, core.InvAppliesDesktopCode)
		}
		if t == core.MCPTargetDesktop {
			out = append(out, core.InvAppliesDesktopChat)
		}
	}
	return out
}

// mcpUnprojectedRows son los servidores que el perfil recibe pero que no están
// en su cc-home, así que ConfigItems no los ve: los apagados y los que solo van
// al chat. Sin ellos `ccp mcp disable X` haría desaparecer a X de la lista —el
// usuario no tendría desde dónde volver a encenderlo— y un servidor con
// `targets: [desktop]` sería invisible en el perfil que lo usa.
func mcpUnprojectedRows(c mcpCtx, layer core.ConfigLayer, seen map[string]bool) []mcpRow {
	if layer.Level != "profile" {
		return nil
	}
	global, profile, err := core.ReadMCPLayers(c.home, c.roots.ClaudeSrc, layer.Name)
	if err != nil {
		return nil
	}
	names := []string{}
	for n := range global {
		names = append(names, n)
	}
	for n := range profile {
		if _, dup := global[n]; !dup {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	var out []mcpRow
	for _, n := range names {
		if seen[n] {
			continue
		}
		src, def := c.roots.ClaudeSrc+".json", map[string]any{}
		if d, ok := global[n].(map[string]any); ok {
			def = d
		}
		if d, ok := profile[n].(map[string]any); ok {
			def, src = d, core.MCPProfileFile(c.home, layer.Name)
		}
		ts := core.MCPTargets(c.cfg, n)
		off := core.MCPDisabled(c.cfg, layer.Name, n)
		applies := mcpAppliesFor(ts)
		if off {
			applies = []string{}
		}
		out = append(out, mcpRow{
			Scope: mcpLayerLabel(layer), Name: n, Type: core.MCPEntry{Def: def}.Kind(),
			Detail: mcpDetail(def), Source: src, Targets: ts,
			AppliesTo: applies, Disabled: off,
		})
	}
	return out
}

// mcpReport cuenta lo que dejó una escritura: qué archivo, a quién hubo que
// regenerar y —lo que nadie más dice— qué ventana de Desktop se quedó con los
// MCP de antes hasta que se reinicie.
func mcpReport(c mcpCtx, stdout, stderr io.Writer, wr core.ConfigWrite, msg string) int {
	fmt.Fprintln(stdout, okLine(stdout, msg))
	if len(wr.Regenerated) > 0 {
		fmt.Fprintln(stdout, mute(stdout, i18n.T(c.lang, "cli.mcp.regenerated", strings.Join(wr.Regenerated, ", "))))
	}
	// Una proyección descartada es el resultado de ESTA escritura, así que
	// callarla dejaba al usuario creyendo que el perfil ya arrancaba su
	// servidor. Se dice con las mismas frases que `ccp profile sync --check`:
	// leerlo antes y después tiene que comparar.
	for _, m := range wr.MCP {
		dest := i18n.T(c.lang, "cli.profile.mcp_dest_"+m.Target)
		if len(m.Conflicts) > 0 {
			fmt.Fprintln(stderr, warnLine(stderr, i18n.T(c.lang, "cli.profile.mcp_conflict", m.Profile, dest, strings.Join(m.Conflicts, ", "))))
		}
		if len(m.RemoteSkipped) > 0 {
			fmt.Fprintln(stderr, warnLine(stderr, i18n.T(c.lang, "cli.profile.mcp_remote", m.Profile, strings.Join(m.RemoteSkipped, ", "))))
		}
	}
	for _, p := range wr.RestartPending() {
		fmt.Fprintln(stderr, warnLine(stderr, i18n.T(c.lang, "cli.mcp.restart", p)))
	}
	if wr.MCPErr != "" {
		fmt.Fprintln(stderr, warnLine(stderr, i18n.T(c.lang, "cli.mcp.proj_err", wr.MCPErr)))
	}
	return 0
}

// mcpUsage escribe el uso donde toca y devuelve el código de salida.
func mcpUsage(w io.Writer, lang i18n.Lang, msg string) int {
	if msg != "" {
		fmt.Fprintln(w, msg)
	}
	fmt.Fprintln(w, i18n.T(lang, "cli.mcp.usage"))
	return 1
}

func mcpCmdList(args []string, stdout, stderr io.Writer) int {
	lang := currentLang()
	a, bad, ok := parseSnapArgs(args, []string{"--json"}, []string{"--scope", "--profile"})
	if !ok || len(a.pos) > 0 {
		if bad == "" && len(a.pos) > 0 {
			bad = a.pos[0]
		}
		return mcpUsage(stderr, lang, i18n.T(lang, "cli.mcp.unknown_opt", bad))
	}
	c, ok := mcpSetup(stderr)
	if !ok {
		return 1
	}
	layer, err := mcpScopeArg(c, a)
	if err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}
	rows, err := mcpRowsFor(c, layer)
	if err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}
	if a.flags["--json"] {
		return snapJSON(stdout, stderr, rows)
	}
	printMCPRows(stdout, c.lang, layer, rows)
	return 0
}

// mcpScopeArg lee --scope y, como atajo, --profile <n>, que es la capa que más
// se escribe a mano.
func mcpScopeArg(c mcpCtx, a snapArgs) (core.ConfigLayer, error) {
	if p := a.val("--profile"); p != "" {
		return core.ConfigLayer{Level: "profile", Name: p}, nil
	}
	return mcpScope(c, a.val("--scope"))
}

func printMCPRows(w io.Writer, lang i18n.Lang, layer core.ConfigLayer, rows []mcpRow) {
	label := mcpLayerLabel(layer)
	if len(rows) == 0 {
		fmt.Fprintln(w, i18n.T(lang, "cli.mcp.empty", label))
		return
	}
	fmt.Fprintln(w, boldLine(w, label))
	for _, r := range rows {
		targets := strings.Join(r.Targets, " · ")
		if targets == "" {
			targets = "—"
		}
		fmt.Fprintf(w, "  %-22s %-6s %-16s %s\n", r.Name, r.Type, mute(w, targets), mute(w, r.Detail))
		if r.Disabled {
			fmt.Fprintln(w, "      "+warnLine(w, i18n.T(lang, "cli.mcp.off_note", r.Name)))
		}
		if r.Why != "" {
			fmt.Fprintln(w, "      "+mute(w, r.Why))
		}
		if r.Missing != "" {
			fmt.Fprintln(w, "      "+warnLine(w, i18n.T(lang, "cli.mcp.missing", r.Missing)))
		}
		fmt.Fprintln(w, "      "+mute(w, tilde(r.Source)))
	}
}

// mcpSplitCmd parte los argumentos en el primer `--`: lo de delante son las
// banderas y lo de detrás, el comando del servidor tal cual. El booleano
// distingue «no hay --» de «hay -- sin nada detrás», que es un error del usuario
// y no un alta remota.
func mcpSplitCmd(args []string) (head, cmd []string, has bool) {
	for i, a := range args {
		if a == "--" {
			return args[:i], args[i+1:], true
		}
	}
	return args, nil, false
}

// mcpPairs convierte los K=V de --env/--header en un mapa. Un par sin `=` se
// rechaza: guardarlo con valor vacío daría un servidor que arranca y falla por
// una credencial que el usuario cree haber puesto.
func mcpPairs(lang i18n.Lang, flag string, vals []string) (map[string]any, error) {
	if len(vals) == 0 {
		return nil, nil
	}
	out := map[string]any{}
	for _, v := range vals {
		k, val, ok := strings.Cut(v, "=")
		if !ok || k == "" {
			return nil, fmt.Errorf("%s", i18n.T(lang, "cli.mcp.bad_pair", flag, v))
		}
		out[k] = val
	}
	return out, nil
}

// mcpDefFrom arma la definición. Tres formas y una sola por llamada: el comando
// tras `--`, una url remota o el JSON pegado. Mezclarlas es el error más fácil
// de cometer y el más difícil de ver después en el archivo.
func mcpDefFrom(lang i18n.Lang, a snapArgs, raw string, cmd []string, hasCmd bool) (map[string]any, error) {
	url, n := a.val("--url"), 0
	for _, on := range []bool{raw != "", url != "", hasCmd} {
		if on {
			n++
		}
	}
	if n == 0 {
		return nil, fmt.Errorf("%s", i18n.T(lang, "cli.mcp.need_source"))
	}
	if n > 1 {
		return nil, fmt.Errorf("%s", i18n.T(lang, "cli.mcp.one_source"))
	}
	env, err := mcpPairs(lang, "--env", a.vals["--env"])
	if err != nil {
		return nil, err
	}
	headers, err := mcpPairs(lang, "--header", a.vals["--header"])
	if err != nil {
		return nil, err
	}
	if raw != "" {
		if env != nil || headers != nil || a.val("--transport") != "" {
			return nil, fmt.Errorf("%s", i18n.T(lang, "cli.mcp.one_source"))
		}
		var def map[string]any
		if err := json.Unmarshal([]byte(raw), &def); err != nil || def == nil {
			return nil, fmt.Errorf("%s", i18n.T(lang, "cli.mcp.bad_json", err))
		}
		return def, nil
	}
	def := map[string]any{}
	if t := a.val("--transport"); t != "" {
		def["type"] = t
	}
	if hasCmd {
		if len(cmd) == 0 {
			return nil, fmt.Errorf("%s", i18n.T(lang, "cli.mcp.need_source"))
		}
		def["command"] = cmd[0]
		if len(cmd) > 1 {
			def["args"] = mcpAnyList(cmd[1:])
		}
		if env != nil {
			def["env"] = env
		}
		// Los headers de un stdio no se descartan aquí: van a la definición
		// para que ValidateMCPServer diga de quién son. Tragárselos dejaba el
		// servidor guardado sin lo que el usuario creyó haber escrito.
		if headers != nil {
			def["headers"] = headers
		}
		return def, nil
	}
	if _, ok := def["type"]; !ok {
		def["type"] = "http"
	}
	def["url"] = url
	if headers != nil {
		def["headers"] = headers
	}
	// Lo mismo al revés: un --env en un remoto viaja para que el validador
	// explique que las credenciales de uno remoto van en headers.
	if env != nil {
		def["env"] = env
	}
	return def, nil
}

// mcpAnyList: los args van como []any porque así los devuelve el JSON del
// archivo, y ValidateMCPServer comprueba la forma que se guarda, no la que se
// tecleó.
func mcpAnyList(ss []string) []any {
	out := make([]any, len(ss))
	for i, s := range ss {
		out[i] = s
	}
	return out
}

// mcpNameArgs parsea el patrón común de add/rm/enable/disable: un nombre
// posicional y las banderas de capa.
func mcpNameArgs(args []string, sub string, stderr io.Writer, valued []string) (snapArgs, string, int) {
	lang := currentLang()
	a, bad, ok := parseSnapArgs(args, []string{"--json"}, append([]string{"--scope", "--profile"}, valued...))
	if !ok {
		return a, "", mcpUsage(stderr, lang, i18n.T(lang, "cli.mcp.unknown_opt", bad))
	}
	if len(a.pos) == 0 {
		return a, "", mcpUsage(stderr, lang, i18n.T(lang, "cli.mcp.need_name", sub))
	}
	return a, a.pos[0], 0
}

func mcpCmdAdd(args []string, stdout, stderr io.Writer) int {
	lang := currentLang()
	head, cmd, hasCmd := mcpSplitCmd(args)
	a, name, code := mcpNameArgs(head, "add", stderr, []string{"--url", "--transport", "--env", "--header"})
	if name == "" {
		return code
	}
	if len(a.pos) > 2 {
		return mcpUsage(stderr, lang, i18n.T(lang, "cli.mcp.unknown_opt", a.pos[2]))
	}
	raw := ""
	if len(a.pos) == 2 {
		raw = a.pos[1]
	}
	def, err := mcpDefFrom(lang, a, raw, cmd, hasCmd)
	if err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}
	c, ok := mcpSetup(stderr)
	if !ok {
		return 1
	}
	layer, err := mcpScopeArg(c, a)
	if err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}
	wr, err := core.MCPPut(c.roots, layer, name, def)
	if err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}
	if a.flags["--json"] {
		return snapJSON(stdout, stderr, wr)
	}
	return mcpReport(c, stdout, stderr, wr, i18n.T(c.lang, "cli.mcp.added", name, tilde(wr.File)))
}

func mcpCmdRm(args []string, stdout, stderr io.Writer) int {
	a, name, code := mcpNameArgs(args, "rm", stderr, nil)
	if name == "" {
		return code
	}
	c, ok := mcpSetup(stderr)
	if !ok {
		return 1
	}
	layer, err := mcpScopeArg(c, a)
	if err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}
	// Borrar lo que no está y decir «[ok] quitado» es mentir, y además regenera
	// a todo el mundo para nada. El error de capa manda: si MCPDelete lo va a
	// rechazar por dónde se pidió, que hable él.
	if v, gerr := core.ConfigItemGet(c.roots, core.ConfigRef{Layer: layer, Type: core.CfgTypeMCP, Name: name}); gerr == nil && !v.Exists {
		fmt.Fprintln(stderr, i18n.T(c.lang, "cli.mcp.not_there", name, mcpLayerLabel(layer)))
		return 1
	}
	wr, err := core.MCPDelete(c.roots, layer, name)
	if err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}
	if a.flags["--json"] {
		return snapJSON(stdout, stderr, wr)
	}
	return mcpReport(c, stdout, stderr, wr, i18n.T(c.lang, "cli.mcp.removed", name, tilde(wr.File)))
}

// mcpCmdSwitch enciende o apaga en UN perfil un servidor heredado. El perfil,
// si no se dice, es el activo de la terminal: apagar «aquí» es la operación que
// pide esta pantalla, y adivinar otro sería apagarlo donde nadie miraba.
func mcpCmdSwitch(args []string, enabled bool, stdout, stderr io.Writer) int {
	sub := "disable"
	if enabled {
		sub = "enable"
	}
	a, name, code := mcpNameArgs(args, sub, stderr, nil)
	if name == "" {
		return code
	}
	c, ok := mcpSetup(stderr)
	if !ok {
		return 1
	}
	profile := a.val("--profile")
	if profile == "" {
		layer, err := mcpScope(c, a.val("--scope"))
		if err != nil || layer.Level != "profile" {
			fmt.Fprintln(stderr, i18n.T(c.lang, "cli.mcp.bad_scope", a.val("--scope")))
			return 1
		}
		profile = layer.Name
	}
	wr, err := core.MCPSetEnabled(c.roots, profile, name, enabled)
	if err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}
	if a.flags["--json"] {
		return snapJSON(stdout, stderr, wr)
	}
	state := i18n.T(c.lang, "cli.mcp.off")
	if enabled {
		state = i18n.T(c.lang, "cli.mcp.on")
	}
	return mcpReport(c, stdout, stderr, wr, i18n.T(c.lang, "cli.mcp.switched", name, state, profile))
}

// mcpTargetRow es la forma estable de `ccp mcp targets --json`. Declared dice si
// hay entrada en ccp.yaml: sin ella los destinos son los dos por defecto, y esa
// diferencia es la que explica por qué un servidor «vuelve» al chat cuando se le
// devuelven los dos.
type mcpTargetRow struct {
	Name     string   `json:"name"`
	Targets  []string `json:"targets"`
	Declared bool     `json:"declared"`
}

// mcpParseTargets lee «cli», «desktop», «cli,desktop» o «none».
func mcpParseTargets(spec string) []string {
	out := []string{}
	for _, t := range strings.Split(spec, ",") {
		t = strings.TrimSpace(t)
		if t == "" || t == "none" || t == "ninguno" {
			continue
		}
		out = append(out, t)
	}
	return out
}

func mcpCmdTargets(args []string, stdout, stderr io.Writer) int {
	lang := currentLang()
	a, bad, ok := parseSnapArgs(args, []string{"--json"}, []string{"--scope", "--profile"})
	if !ok || len(a.pos) > 2 {
		if bad == "" && len(a.pos) > 2 {
			bad = a.pos[2]
		}
		return mcpUsage(stderr, lang, i18n.T(lang, "cli.mcp.unknown_opt", bad))
	}
	c, ok := mcpSetup(stderr)
	if !ok {
		return 1
	}
	if len(a.pos) == 2 {
		ts := mcpParseTargets(a.pos[1])
		wr, err := core.MCPSetTargets(c.roots, a.pos[0], ts)
		if err != nil {
			fmt.Fprintf(stderr, "Error: %v\n", err)
			return 1
		}
		if a.flags["--json"] {
			return snapJSON(stdout, stderr, wr)
		}
		where := strings.Join(ts, " · ")
		if where == "" {
			where = i18n.T(c.lang, "cli.mcp.targets_none")
		}
		return mcpReport(c, stdout, stderr, wr, i18n.T(c.lang, "cli.mcp.targets_done", a.pos[0], where))
	}
	rows, err := mcpTargetRows(c, a)
	if err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}
	if a.flags["--json"] {
		return snapJSON(stdout, stderr, rows)
	}
	for _, r := range rows {
		where := strings.Join(r.Targets, " · ")
		if where == "" {
			where = i18n.T(c.lang, "cli.mcp.targets_none")
		}
		mark := " "
		if r.Declared {
			mark = "*"
		}
		fmt.Fprintf(stdout, "%s %-22s %s\n", mark, r.Name, where)
	}
	return 0
}

// mcpTargetRows son los servidores que la capa ve MÁS los que ccp.yaml nombra
// aunque no estén ahí: una entrada de destinos que sobrevive al servidor que la
// motivó es justo lo que hay que poder ver para quitarla.
func mcpTargetRows(c mcpCtx, a snapArgs) ([]mcpTargetRow, error) {
	layer, err := mcpScopeArg(c, a)
	if err != nil {
		return nil, err
	}
	rows, err := mcpRowsFor(c, layer)
	if err != nil {
		return nil, err
	}
	out, seen := []mcpTargetRow{}, map[string]bool{}
	declared := func(n string) bool {
		return c.cfg != nil && c.cfg.MCP != nil && c.cfg.MCP.Targets[n] != nil
	}
	for _, r := range rows {
		if seen[r.Name] {
			continue
		}
		seen[r.Name] = true
		out = append(out, mcpTargetRow{Name: r.Name, Targets: r.Targets, Declared: declared(r.Name)})
	}
	if c.cfg != nil && c.cfg.MCP != nil {
		for n := range c.cfg.MCP.Targets {
			if !seen[n] {
				out = append(out, mcpTargetRow{Name: n, Targets: core.MCPTargets(c.cfg, n), Declared: true})
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// mcpCmdAdopt es `ccp mcp adopt <nombre> [--profile <n>]`: pasa a ccp un
// servidor escrito a mano en el chat de la ventana de un perfil (ver
// core.MCPAdoptDesktop). Queda declarado en ese perfil, solo para el chat, tal
// como estaba; desde entonces se edita con `ccp mcp add --scope profile:<n>`.
// Sin --profile, el perfil activo de la terminal, como enable/disable.
func mcpCmdAdopt(args []string, stdout, stderr io.Writer) int {
	a, name, code := mcpNameArgs(args, "adopt", stderr, nil)
	if name == "" {
		return code
	}
	c, ok := mcpSetup(stderr)
	if !ok {
		return 1
	}
	profile := a.val("--profile")
	if profile == "" {
		layer, err := mcpScope(c, a.val("--scope"))
		if err != nil || (layer.Level != "profile" && layer.Level != "desktop") {
			fmt.Fprintln(stderr, i18n.T(c.lang, "cli.mcp.bad_scope", a.val("--scope")))
			return 1
		}
		profile = layer.Name
	}
	wr, err := core.MCPAdoptDesktop(c.roots, profile, name, nil)
	if err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}
	if a.flags["--json"] {
		return snapJSON(stdout, stderr, wr)
	}
	return mcpReport(c, stdout, stderr, wr, i18n.T(c.lang, "cli.mcp.adopted", name, profile))
}
