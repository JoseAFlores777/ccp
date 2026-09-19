package core

// adopt.go — la adopción (spec 2026-09-18 §5.2): con el inventario de la máquina,
// un PLAN de lo que ccp puede traer a su modelo y, aparte, su aplicación. El
// plan es puro y se enseña entero antes de tocar nada; aplicar toma primero un
// snapshot de seguridad (Before) y luego va en el orden del spec, porque cada
// paso depende del anterior.

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
)

// Tipos de paso.
const (
	AdoptConfigDir     = "adopt-config-dir"
	AdoptLiftMCPGlobal = "lift-mcp-global"
	AdoptRuleOrphan    = "rule-orphan"
	AdoptLauncher      = "desktop-launcher"
	AdoptLogin         = "login"
	AdoptAPIKey        = "api-key"
	AdoptMCPMissing    = "mcp-command-missing"
	AdoptMCPConflict   = "mcp-conflict"
	AdoptLiftProfile   = "lift-mcp-profile"
)

// AdoptStep es un paso del plan. Key es la clave JSON dentro de From/To que el
// paso mueve (vacía si es el archivo o el directorio entero), igual que InvItem
// separa Source de Key. Los pasos Pending no se aplican: dicen qué falta hacer
// a mano o qué llega con otra fase.
type AdoptStep struct {
	ID      string   `json:"id"`
	Order   int      `json:"order"`
	Kind    string   `json:"kind"`
	Title   string   `json:"title"`
	Detail  string   `json:"detail,omitempty"`
	From    string   `json:"from,omitempty"`
	To      string   `json:"to,omitempty"`
	Key     string   `json:"key,omitempty"`
	Items   []string `json:"items"`
	Default bool     `json:"default"`
	Pending bool     `json:"pending"`
}

// AdoptInputs es lo que el plan necesita saber de la máquina y no está en el
// inventario. Lo rellena AdoptInputsFor; los tests lo montan a mano.
type AdoptInputs struct {
	Cfg       *Config
	LoggedIn  map[string]bool // perfiles official con sesión (HasLogin)
	HasKey    map[string]bool // providers con api_key
	Launchers map[string]bool // perfiles con lanzador de Desktop
	DataDirs  map[string]bool // perfiles cuya ventana de Desktop se ha usado (data dir en disco)
	// ClaudeJSON es el ~/.claude.json de esta máquina (ClaudeSrc + ".json"): el
	// destino de «subir a global», que es el scope user oficial (spec §6.1).
	ClaudeJSON string
}

// AdoptInputsFor lee de disco lo que AdoptPlan no puede calcular solo.
func AdoptInputsFor(home, claudeSrc string, cfg *Config, appsDir string) AdoptInputs {
	in := AdoptInputs{Cfg: cfg, LoggedIn: map[string]bool{}, HasKey: map[string]bool{},
		Launchers: map[string]bool{}, DataDirs: map[string]bool{}}
	if claudeSrc != "" {
		in.ClaudeJSON = claudeSrc + ".json"
	}
	if cfg == nil {
		return in
	}
	for n, p := range cfg.Profiles {
		if n == "default" {
			continue
		}
		switch {
		case p.Type == "official":
			in.LoggedIn[n] = HasLogin(home, n)
		case IsProviderType(p.Type):
			_, in.HasKey[n] = GetKey(home, n)
		}
		if fi, err := os.Stat(DesktopDataDir(home, n)); err == nil && fi.IsDir() {
			in.DataDirs[n] = true
		}
		if appsDir != "" {
			if app, err := FindDesktopApp(appsDir, n); err == nil && app != nil {
				in.Launchers[n] = true
			}
		}
	}
	return in
}

// adoptStepID: kind+from+to+key+items separados por \x00 (así «a»+«bc» y
// «ab»+«c» no coinciden). Kind+from+to no basta: todos los lift-mcp-global de
// un mismo claude_desktop_config.json compartirían ID y `--only <id>` subiría
// todos, tokens incluidos.
func adoptStepID(s AdoptStep) string {
	items := append([]string(nil), s.Items...)
	sort.Strings(items)
	h := sha256.New()
	for _, f := range append([]string{s.Kind, s.From, s.To, s.Key}, items...) {
		h.Write([]byte(f))
		h.Write([]byte{0})
	}
	return fmt.Sprintf("%x", h.Sum(nil))[:10]
}

// Orden del spec §5.2: perfiles → reglas → capas → Desktop → pendientes.
var adoptOrder = map[string]int{
	AdoptConfigDir: 1, AdoptRuleOrphan: 2, AdoptLiftMCPGlobal: 3, AdoptLiftProfile: 3, AdoptMCPConflict: 3,
	AdoptLauncher: 4, AdoptLogin: 5, AdoptAPIKey: 5, AdoptMCPMissing: 5,
}

// AdoptPlan calcula el plan de adopción: puro, determinista y con IDs únicos.
func AdoptPlan(inv Inventory, in AdoptInputs) []AdoptStep {
	var steps []AdoptStep
	add := func(s AdoptStep) {
		if s.Items == nil {
			s.Items = []string{}
		}
		s.Order = adoptOrder[s.Kind]
		steps = append(steps, s)
	}

	// 1. Directorios de config sin gestionar → perfil nuevo (D5: copiar + /login).
	taken := map[string]bool{"default": true}
	if in.Cfg != nil {
		for n := range in.Cfg.Profiles {
			taken[n] = true
		}
	}
	for _, it := range inv.Items {
		if it.Kind != "config-dir" || strings.Contains(it.Why, "la carpeta no existe") {
			continue
		}
		name := adoptProfileName(filepath.Base(it.Source), taken)
		taken[name] = true
		add(AdoptStep{Kind: AdoptConfigDir, From: it.Source, To: "profiles/" + name, Items: []string{name},
			Default: true, Title: fmt.Sprintf("Adoptar %s como el perfil %s", it.Source, name),
			Detail: "Se copia su configuración (settings.json sin env, CLAUDE.md, agents, commands, skills, " +
				"output-styles y los MCP de su .claude.json), nunca los tokens; el original no se toca. " +
				"Después hará falta `ccp profile login " + name + "`."})
	}

	// 2. Reglas que apuntan a carpetas que ya no existen: solo se avisa.
	for _, it := range inv.Items {
		if it.Kind == "rule-path" && strings.Contains(it.Why, "la carpeta no existe") {
			add(AdoptStep{Kind: AdoptRuleOrphan, From: it.Name, Pending: true,
				Title:  fmt.Sprintf("La regla de %s apunta a una carpeta que no existe", it.Name),
				Detail: "Si ya no la usas: ccp path rm " + it.Name})
		}
	}

	// 3. Capas: MCP que solo están en Desktop.
	adoptPlanMCP(inv, in, add)

	// 4. Desktop: ventanas usadas sin lanzador (se ofrecen, no se construyen).
	for _, n := range adoptSortedKeys(in.DataDirs) {
		if !in.Launchers[n] {
			add(AdoptStep{Kind: AdoptLauncher, Items: []string{n}, Pending: true,
				Title:  fmt.Sprintf("La ventana de Desktop de %s no tiene lanzador", n),
				Detail: "Sin él corre como el Claude principal y macOS no la distingue: ccp desktop app " + n})
		}
	}

	// 5. Pendientes que no se pueden automatizar.
	for _, n := range adoptSortedKeys(in.LoggedIn) {
		if !in.LoggedIn[n] {
			add(AdoptStep{Kind: AdoptLogin, Items: []string{n}, Pending: true,
				Title: fmt.Sprintf("%s no tiene sesión iniciada", n), Detail: "ccp profile login " + n})
		}
	}
	for _, n := range adoptSortedKeys(in.HasKey) {
		if !in.HasKey[n] {
			add(AdoptStep{Kind: AdoptAPIKey, Items: []string{n}, Pending: true,
				Title: fmt.Sprintf("%s no tiene api_key", n), Detail: "ccp key " + n})
		}
	}
	for _, it := range inv.Items {
		if it.Kind == "mcp" && it.Missing != "" {
			add(AdoptStep{Kind: AdoptMCPMissing, From: it.Source, Key: it.Key, Items: []string{it.Name}, Pending: true,
				Title:  fmt.Sprintf("%s: el comando %s no está en esta máquina", it.Name, it.Missing),
				Detail: "Instálalo o corrige el comando; mientras tanto ese servidor no arranca."})
		}
	}

	return adoptFinish(steps)
}

// adoptFinish pone IDs, deduplica (dos pasos idénticos son el mismo paso) y
// ordena de forma estable.
func adoptFinish(steps []AdoptStep) []AdoptStep {
	seen := map[string]bool{}
	out := make([]AdoptStep, 0, len(steps))
	for _, s := range steps {
		s.ID = adoptStepID(s)
		if seen[s.ID] {
			continue
		}
		seen[s.ID] = true
		out = append(out, s)
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Order != b.Order {
			return a.Order < b.Order
		}
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		if a.From != b.From {
			return a.From < b.From
		}
		if a.Key != b.Key {
			return a.Key < b.Key
		}
		return strings.Join(a.Items, ",") < strings.Join(b.Items, ",")
	})
	return out
}

func adoptSortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// adoptProfileName deriva el nombre del perfil del directorio (~/.claude-work →
// work) y lo hace único frente a los que ya existen (work-2, work-3…).
func adoptProfileName(base string, taken map[string]bool) string {
	n := strings.TrimPrefix(base, ".claude")
	n = strings.TrimLeft(n, "-_.")
	var b strings.Builder
	for _, r := range strings.ToLower(n) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	n = strings.Trim(b.String(), "-_")
	if n == "" || !validProfileName(n) {
		n = "adoptado"
	}
	cand := n
	for i := 2; taken[cand]; i++ {
		cand = fmt.Sprintf("%s-%d", n, i)
	}
	return cand
}

// adoptPlanMCP: un MCP que está en algún claude_desktop_config.json y en ningún
// destino de la CLI (~/.claude.json ni un cc-home/.claude.json) se propone subir
// a global si está en la ventana default o en todas las ventanas; si solo está
// en la de algún perfil, subirlo a ese perfil es la Fase B (pendiente). Si las
// copias difieren en la forma o en los secretos no se sube nada: «global» se
// proyecta a todos los perfiles (B1), y subir una daría el token de una cuenta
// al CLI de otra. Qué valor va adónde lo decide el usuario.
func adoptPlanMCP(inv Inventory, in AdoptInputs, add func(AdoptStep)) {
	type copyOf struct {
		window string
		it     InvItem
	}
	onCLI := map[string]bool{}
	desk := map[string][]copyOf{}
	hasDefault := false
	for _, it := range inv.Items {
		if it.Kind != "mcp" {
			continue
		}
		switch it.Scope.Level {
		case "global":
			if it.Key == "mcpServers."+it.Name {
				onCLI[it.Name] = true
			}
		case "profile":
			onCLI[it.Name] = true
		case "desktop":
			desk[it.Name] = append(desk[it.Name], copyOf{it.Scope.Name, it})
			if it.Scope.Name == "default" {
				hasDefault = true
			}
		}
	}
	all := map[string]bool{}
	for n := range in.DataDirs {
		all[n] = true
	}
	if hasDefault {
		all["default"] = true
	}
	names := make([]string, 0, len(desk))
	for n := range desk {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, name := range names {
		if onCLI[name] {
			continue
		}
		copies := desk[name]
		// default primero y luego por nombre de ventana: From determinista.
		sort.SliceStable(copies, func(i, j int) bool {
			if (copies[i].window == "default") != (copies[j].window == "default") {
				return copies[i].window == "default"
			}
			return copies[i].window < copies[j].window
		})
		wins := make([]string, 0, len(copies))
		shapes, secrets := map[string]bool{}, map[string]bool{}
		var secretPaths []string
		for _, c := range copies {
			wins = append(wins, c.window)
			shapes[c.it.Hash] = true
			secrets[c.it.SecretHash] = true
			for _, p := range c.it.Secrets {
				if !slices.Contains(secretPaths, p) {
					secretPaths = append(secretPaths, p)
				}
			}
		}
		switch {
		case len(shapes) > 1:
			add(AdoptStep{Kind: AdoptMCPConflict, Key: "mcpServers." + name, Items: append([]string{name}, wins...), Pending: true,
				Title:  fmt.Sprintf("%s está en varias ventanas con configuraciones distintas", name),
				Detail: "Ventanas: " + strings.Join(wins, ", ") + ". No se sube a global: elige cuál vale y déjala en una sola."})
			continue
		case len(secrets) > 1:
			sort.Strings(secretPaths)
			add(AdoptStep{Kind: AdoptMCPConflict, Key: "mcpServers." + name, Items: append([]string{name}, wins...), Pending: true,
				Title: fmt.Sprintf("%s está en varias ventanas con credenciales distintas", name),
				Detail: "Ventanas: " + strings.Join(wins, ", ") + "; sus credenciales (" + strings.Join(secretPaths, ", ") + ") no coinciden" +
					". No se sube a global: daría la credencial de una cuenta a las demás. Cuando llegue el MCP por perfil (Fase B), cada ventana tendrá la suya."})
			continue
		}
		inAll := len(all) > 0
		for w := range all {
			if !slices.Contains(wins, w) {
				inAll = false
			}
		}
		if slices.Contains(wins, "default") || inAll {
			add(AdoptStep{Kind: AdoptLiftMCPGlobal, From: copies[0].it.Source, To: in.ClaudeJSON,
				Key: "mcpServers." + name, Items: []string{name}, Default: true,
				Title: fmt.Sprintf("Subir %s a global", name),
				Detail: "Hoy solo lo ven las ventanas de Desktop (" + strings.Join(wins, ", ") +
					"); en ~/.claude.json lo verá también la CLI. Se copia tal cual, con sus credenciales, de " + copies[0].window + "."})
			continue
		}
		add(AdoptStep{Kind: AdoptLiftProfile, Key: "mcpServers." + name, Items: append([]string{name}, wins...), Pending: true,
			Title:  fmt.Sprintf("%s solo está en la ventana de %s", name, strings.Join(wins, ", ")),
			Detail: "Subirlo a ese perfil llega con el MCP por perfil (Fase B)."})
	}
}

// AdoptApplyOpts: Only nil = los pasos Default; Only vacío pero no nil = nada
// (una GUI sin casillas marcadas no puede aplicar «lo de siempre» por omisión).
// Before es el snapshot de seguridad: si falla, no se aplica nada.
type AdoptApplyOpts struct {
	Only   []string
	Before func() error
}

// AdoptSkip es un paso seleccionado que no se aplicó, con el motivo.
type AdoptSkip struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Reason string `json:"reason"`
}

// AdoptReport cuenta lo que hizo AdoptApply.
type AdoptReport struct {
	Applied []AdoptStep `json:"applied"`
	Skipped []AdoptSkip `json:"skipped"`
	Pending []AdoptStep `json:"pending"`
}

// AdoptApply aplica los pasos seleccionados del plan, en su orden.
func AdoptApply(home string, r InventoryRoots, steps []AdoptStep, o AdoptApplyOpts) (*AdoptReport, error) {
	rep := &AdoptReport{Applied: []AdoptStep{}, Skipped: []AdoptSkip{}, Pending: []AdoptStep{}}
	want := map[string]bool{}
	for _, id := range o.Only {
		want[id] = true
	}
	var sel []AdoptStep
	for _, s := range steps {
		switch {
		case s.Pending:
			rep.Pending = append(rep.Pending, s)
			if want[s.ID] {
				rep.Skipped = append(rep.Skipped, AdoptSkip{s.ID, s.Title, "es un pendiente: se hace a mano"})
			}
		case o.Only == nil && s.Default, want[s.ID]:
			sel = append(sel, s)
		}
	}
	for id := range want {
		if !adoptHasID(steps, id) {
			return nil, fmt.Errorf("el plan no tiene ningún paso con id %q (el plan cambia si cambia la máquina: vuelve a pedirlo)", id)
		}
	}
	if len(sel) == 0 {
		return rep, nil
	}
	if o.Before != nil {
		if err := o.Before(); err != nil {
			return nil, fmt.Errorf("no se pudo guardar el snapshot de seguridad y no se adoptó nada: %w", err)
		}
	}

	var lifts []AdoptStep
	for _, s := range sel {
		switch s.Kind {
		case AdoptLiftMCPGlobal:
			lifts = append(lifts, s)
		case AdoptConfigDir:
			if err := adoptConfigDir(home, r, s, rep); err != nil {
				return rep, err
			}
		default:
			rep.Skipped = append(rep.Skipped, AdoptSkip{s.ID, s.Title, "este tipo de paso no se aplica solo"})
		}
	}
	if len(lifts) > 0 {
		if err := adoptLiftGlobal(lifts, rep); err != nil {
			return rep, err
		}
	}
	return rep, nil
}

func adoptHasID(steps []AdoptStep, id string) bool {
	for _, s := range steps {
		if s.ID == id {
			return true
		}
	}
	return false
}

// adoptLiftGlobal sube a ~/.claude.json, en una sola escritura, los MCP de los
// pasos. Nunca pisa un nombre que ya exista ni toca otra clave: Claude Code
// reescribe ese archivo a menudo y conserva lo escrito desde fuera (M6).
func adoptLiftGlobal(lifts []AdoptStep, rep *AdoptReport) error {
	to := lifts[0].To
	if to == "" {
		return errors.New("no se sabe dónde está ~/.claude.json")
	}
	live, err := os.ReadFile(to)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("no se pudo leer %s: %w", to, err)
	}
	doc := map[string]any{}
	if len(live) > 0 {
		if doc, err = decodeJSONObject(live); err != nil {
			return fmt.Errorf("%s no es JSON válido; no se toca: %w", to, err)
		}
	}
	servers, _ := doc["mcpServers"].(map[string]any)
	if servers == nil {
		servers = map[string]any{}
	}
	changed := false
	for _, s := range lifts {
		name := s.Items[0]
		if _, exists := servers[name]; exists {
			rep.Skipped = append(rep.Skipped, AdoptSkip{s.ID, s.Title, "ya existe en ~/.claude.json"})
			continue
		}
		src, err := os.ReadFile(s.From)
		if err != nil {
			rep.Skipped = append(rep.Skipped, AdoptSkip{s.ID, s.Title, "no se pudo leer " + s.From})
			continue
		}
		m, err := decodeJSONObject(src)
		entry, ok := invMCPMap(m)[name]
		if err != nil || !ok {
			rep.Skipped = append(rep.Skipped, AdoptSkip{s.ID, s.Title, "ya no está en " + s.From})
			continue
		}
		servers[name] = entry
		changed = true
		rep.Applied = append(rep.Applied, s)
	}
	if !changed {
		return nil
	}
	doc["mcpServers"] = servers
	out, err := marshalIndent(doc) // sin escapar <, > ni &: no cambia lo que no toca
	if err != nil {
		return err
	}
	if len(live) == 0 {
		return writeFileAtomic(to, out, 0o600)
	}
	// Conserva el modo y un symlink (dotfiles), como el overlay (cfg_drift.go).
	return writeOverlayPreservingLink(to, out)
}

// adoptConfigDir adopta un CLAUDE_CONFIG_DIR sin gestionar como perfil nuevo
// (D5): copia su configuración a la capa del perfil y deja el original intacto.
// Nunca los tokens: el Llavero los guarda con un nombre que sale de la ruta
// (ADR 0016, M4), así que hará falta un /login.
func adoptConfigDir(home string, r InventoryRoots, s AdoptStep, rep *AdoptReport) error {
	name, dir := s.Items[0], s.From
	if err := ProfileAddOfficial(home, name); err != nil {
		return fmt.Errorf("adoptar %s: %w", dir, err)
	}
	ov := cfgOverlayDir(home, name)

	// settings.json → overlay, sin env (tokens: el mismo motivo que B6).
	if b, err := os.ReadFile(filepath.Join(dir, "settings.json")); err == nil {
		m, derr := decodeJSONObject(b)
		if derr != nil {
			rep.Skipped = append(rep.Skipped, AdoptSkip{s.ID, s.Title, "su settings.json no es JSON válido: no se copió"})
		} else {
			if _, has := m["env"]; has {
				delete(m, "env")
				rep.Skipped = append(rep.Skipped, AdoptSkip{s.ID, s.Title,
					"su env no se copió (suele llevar tokens); ponlo con ccp profile config " + name})
			}
			out, err := marshalIndent(m)
			if err != nil {
				return err
			}
			if err := writeFileAtomic(cfgSettingsFile(home, name), out, 0o644); err != nil {
				return err
			}
		}
	}
	// CLAUDE.md → al final del overlay, fuera del bloque que gestiona ccp.
	if b, err := os.ReadFile(filepath.Join(dir, "CLAUDE.md")); err == nil && len(b) > 0 {
		cur, _ := os.ReadFile(cfgInstrFile(home, name))
		out := append(append(cur, '\n'), b...)
		if err := writeFileAtomic(cfgInstrFile(home, name), out, 0o644); err != nil {
			return err
		}
	}
	// agents, commands, skills, output-styles → overlay/<dir> (la capa de perfil,
	// spec §6.2; la proyecta la Fase B).
	for _, d := range []string{"agents", "commands", "skills", "output-styles"} {
		if err := copyTreeFiles(filepath.Join(dir, d), filepath.Join(ov, d)); err != nil {
			return fmt.Errorf("adoptar %s/%s: %w", dir, d, err)
		}
	}
	// .claude.json → solo su parte de configuración, sobre el del cc-home nuevo.
	if b, err := os.ReadFile(filepath.Join(dir, ".claude.json")); err == nil {
		part, err := ClaudeJSONConfig(b)
		if err != nil {
			rep.Skipped = append(rep.Skipped, AdoptSkip{s.ID, s.Title, "su .claude.json no es JSON válido: no se copiaron sus MCP"})
		} else if part != nil {
			dst := filepath.Join(ccHomePath(home, name), ".claude.json")
			live, _ := os.ReadFile(dst)
			out, err := ClaudeJSONApplyConfig(live, part)
			if err != nil {
				return err
			}
			if err := writeFileAtomic(dst, out, 0o600); err != nil {
				return err
			}
		}
	}
	if err := CfgRegenerate(home, name, r.ClaudeSrc); err != nil {
		return err
	}
	rep.Applied = append(rep.Applied, s)
	rep.Pending = append(rep.Pending, AdoptStep{ID: adoptStepID(AdoptStep{Kind: AdoptLogin, Items: []string{name}}),
		Kind: AdoptLogin, Order: adoptOrder[AdoptLogin], Items: []string{name}, Pending: true,
		Title: fmt.Sprintf("%s no tiene sesión iniciada", name), Detail: "ccp profile login " + name})
	return nil
}

// copyTreeFiles copia los archivos regulares de src a dst (directorios reales,
// 0644/0755). Los symlinks no se siguen ni se recrean: apuntan a algo que no es
// de esta configuración. Un src que no existe no es error.
func copyTreeFiles(src, dst string) error {
	if fi, err := os.Stat(src); err != nil || !fi.IsDir() {
		return nil
	}
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		switch {
		case d.IsDir():
			return os.MkdirAll(target, 0o755)
		case d.Type().IsRegular():
			b, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			return writeFileAtomic(target, b, 0o644)
		}
		return nil
	})
}
