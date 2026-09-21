package core

// snapshot_port.go — portabilidad entre máquinas (spec §11). Un snapshot de
// otra máquina trae rutas absolutas que aquí no significan nada: el HOME lo
// traduce snapshot_home.go, pero un proyecto vive donde el dueño de ESTA
// máquina lo haya clonado. Aquí se decide dónde cae cada uno, y qué le queda
// por hacer a mano después de aplicarlo (logins, comandos que no existen).
//
// El mapeo se hace EN la máquina y no en el portal: es el único sitio donde
// hay un disco que mirar.

import (
	"encoding/json"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/JoseAFlores777/ccp/internal/snapshot"
)

// De dónde sale la ruta de un proyecto en esta máquina.
const (
	// ProjectMapSnapshot: la ruta que traía el snapshot, traducida de HOME,
	// existe aquí. Es la que el usuario ya usa.
	ProjectMapSnapshot = "snapshot"
	// ProjectMapRemote: no existe, pero un candidato tiene el mismo remoto.
	ProjectMapRemote = "remote"
	// ProjectMapManual: la eligió la persona.
	ProjectMapManual = "manual"
	// ProjectMapMissing: aquí no está. Sus archivos no se restauran.
	ProjectMapMissing = "missing"
)

// ProjectMapping dice dónde cae en esta máquina un proyecto del snapshot.
type ProjectMapping struct {
	Key    string `json:"key"`
	Remote string `json:"remote,omitempty"`
	// From es la ruta que traía el snapshot, ya traducida al HOME de aquí.
	From   string `json:"from,omitempty"`
	Path   string `json:"path,omitempty"`
	Source string `json:"source"`
	// Files son las rutas lógicas que dependen de este mapeo: sin ellas, quien
	// enseña el plan solo puede nombrar el proyecto por 12 hex de su clave.
	Files []string `json:"files"`
	// Clone es el remoto que habría que clonar cuando no está. Se dice, no se
	// ejecuta: clonar un repo es traerse código de la red.
	Clone string `json:"clone,omitempty"`
}

// ProjectMapInputs es lo que SnapshotProjects no puede calcular solo. Todo
// inyectado: una máquina entera cabe en un test sin tocar disco.
type ProjectMapInputs struct {
	// HomeTo es el HOME de esta máquina; el de origen sale del manifiesto.
	HomeTo string
	// Candidates son carpetas donde puede estar el repo: las de las reglas,
	// las de los `projects` de los .claude.json y las raíces habituales.
	Candidates []string
	// Manual es lo que decidió la persona: clave -> ruta.
	Manual map[string]string
	IsDir  func(string) bool
	Remote func(string) string
}

func (in ProjectMapInputs) isDir(p string) bool {
	if in.IsDir != nil {
		return in.IsDir(p)
	}
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}

func (in ProjectMapInputs) remote(p string) string {
	if in.Remote != nil {
		return in.Remote(p)
	}
	return gitOriginURL(p)
}

// SnapshotProjects resuelve, para cada proyecto del manifiesto, dónde cae en
// esta máquina. Puro: ordena por clave para que dos llamadas den lo mismo.
func SnapshotProjects(m *snapshot.Manifest, in ProjectMapInputs) []ProjectMapping {
	byKey := map[string]*ProjectMapping{}
	var orden []string
	for _, it := range m.Items {
		parts := strings.Split(it.LPath, "/")
		if len(parts) < 3 || parts[0] != "project" {
			continue
		}
		key := parts[1]
		pm, ok := byKey[key]
		if !ok {
			pm = &ProjectMapping{Key: key, Remote: it.Meta["remote"], Files: []string{}}
			// El HOME de origen se traduce igual que en el restore: sin esto,
			// la ruta de otra máquina no existe aquí nunca y todo es «missing».
			if p := it.Meta["path"]; p != "" && m.Home != "" && in.HomeTo != "" && m.Home != in.HomeTo {
				pm.From = translateHomePath(p, m.Home, in.HomeTo)
			} else {
				pm.From = it.Meta["path"]
			}
			byKey[key], orden = pm, append(orden, key)
		}
		pm.Files = append(pm.Files, it.LPath)
	}
	sort.Strings(orden)
	out := make([]ProjectMapping, 0, len(orden))
	for _, key := range orden {
		pm := byKey[key]
		sort.Strings(pm.Files)
		resolveProject(pm, in)
		out = append(out, *pm)
	}
	return out
}

// resolveProject elige la ruta, en orden: lo que dijo la persona, la ruta que
// ya existe aquí, un clon con el mismo remoto. La ruta existente va ANTES que
// la búsqueda por remoto porque es donde el usuario está trabajando: un
// segundo clon que aparezca en las raíces habituales no la desplaza.
func resolveProject(pm *ProjectMapping, in ProjectMapInputs) {
	if p := in.Manual[pm.Key]; p != "" {
		pm.Path, pm.Source = p, ProjectMapManual
		return
	}
	if pm.From != "" && in.isDir(pm.From) {
		pm.Path, pm.Source = pm.From, ProjectMapSnapshot
		return
	}
	if pm.Remote != "" {
		want := normalizeRemote(pm.Remote)
		cands := append([]string(nil), in.Candidates...)
		sort.Strings(cands)
		for _, c := range cands {
			if c == "" || !in.isDir(c) {
				continue
			}
			if r := in.remote(c); r != "" && normalizeRemote(r) == want {
				pm.Path, pm.Source = c, ProjectMapRemote
				return
			}
		}
		pm.Clone = pm.Remote
	}
	pm.Source = ProjectMapMissing
}

// ProjectMapInputsFor lee de disco lo que el mapeo necesita: el HOME de aquí y
// las carpetas donde puede estar un repo. Los `projects` de los .claude.json y
// las reglas son lo que esta máquina ya conoce; las raíces habituales se
// recorren poco hondo (un repo no suele estar a cinco niveles) y solo se
// apuntan las que tienen .git, para no pasearse por medio disco.
func ProjectMapInputsFor(home, src string) ProjectMapInputs {
	in := ProjectMapInputs{Manual: map[string]string{}}
	in.HomeTo, _ = os.UserHomeDir()
	seen := map[string]bool{}
	add := func(p string) {
		if p == "" || !filepath.IsAbs(p) || seen[p] {
			return
		}
		seen[p] = true
		in.Candidates = append(in.Candidates, p)
	}
	jsons := []string{src + ".json"}
	if cfg, err := Load(home); err == nil {
		for _, r := range cfg.Rules {
			add(r.Path)
		}
		for n := range cfg.Profiles {
			if n != "default" {
				jsons = append(jsons, filepath.Join(ccHomePath(home, n), ".claude.json"))
			}
		}
	}
	sort.Strings(jsons)
	for _, j := range jsons {
		for _, p := range claudeJSONProjects(j) {
			add(p)
		}
	}
	for _, p := range scanProjectRoots(in.HomeTo) {
		add(p)
	}
	sort.Strings(in.Candidates)
	return in
}

// claudeJSONProjects son las carpetas que un .claude.json recuerda. No se
// filtran por existencia aquí: quien las usa ya comprueba que sean carpetas.
func claudeJSONProjects(path string) []string {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var top struct {
		Projects map[string]json.RawMessage `json:"projects"`
	}
	if err := json.Unmarshal(b, &top); err != nil {
		return nil
	}
	out := make([]string, 0, len(top.Projects))
	for p := range top.Projects {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// projectRoots son las carpetas donde la gente guarda sus repos. Buscar en
// todo el HOME sería recorrer Fotos y node_modules; buscar solo aquí y solo
// dos niveles encuentra ~/code/app y ~/src/org/app sin pasearse por el disco.
var projectRoots = []string{"code", "src", "dev", "projects", "Projects", "repos", "Developer",
	"work", "Documents", "Documents/GitHub"}

// scanProjectRoots devuelve las carpetas con .git que cuelgan de las raíces
// habituales, hasta dos niveles. Un error de lectura se ignora: esto es una
// sugerencia, y una carpeta sin permiso no puede tumbar el asistente.
func scanProjectRoots(home string) []string {
	if home == "" {
		return nil
	}
	var out []string
	for _, r := range projectRoots {
		root := filepath.Join(home, filepath.FromSlash(r))
		for _, d := range projectDirs(root) {
			if isGitDir(d) {
				out = append(out, d)
				continue
			}
			for _, d2 := range projectDirs(d) {
				if isGitDir(d2) {
					out = append(out, d2)
				}
			}
		}
	}
	sort.Strings(out)
	return out
}

func projectDirs(dir string) []string {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range ents {
		if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
			out = append(out, filepath.Join(dir, e.Name()))
		}
	}
	return out
}

func isGitDir(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil
}

// Lo que queda por hacer a mano después de aplicar un snapshot ajeno (§11).
const (
	// RestorePendingLogin: un perfil official sin sesión en esta máquina. Los
	// tokens no viajan nunca (§11), así que esto es normal, no un fallo.
	RestorePendingLogin = "login"
	// RestorePendingCommand: un comando que la configuración nombra (un MCP,
	// un hook, la barra de estado) y aquí no existe.
	RestorePendingCommand = "command"
	// RestorePendingProject: un proyecto que no se encontró en esta máquina.
	RestorePendingProject = "project"
)

// RestorePendingItem es una tarea para la persona, no para ccp.
type RestorePendingItem struct {
	Kind string `json:"kind"`
	Name string `json:"name"`
	// Where dice dónde aparece: el perfil, la ruta lógica o el remoto a clonar.
	Where string `json:"where,omitempty"`
}

// SnapshotPendingInputs es lo que SnapshotPending no puede calcular solo.
type SnapshotPendingInputs struct {
	Projects []ProjectMapping
	// LoggedIn: perfiles official con sesión en ESTA máquina (HasLogin).
	LoggedIn map[string]bool
	LookPath func(string) (string, error)
}

// SnapshotPendingInputsFor lee de disco lo que hace falta para los pendientes.
// Los perfiles que mira son los del snapshot, no los de aquí; el mapa se
// rellena con lo que esta máquina sabe y un perfil que aún no existe cuenta
// como «sin sesión», que es lo que será en cuanto se aplique.
func SnapshotPendingInputsFor(home string, projects []ProjectMapping) SnapshotPendingInputs {
	in := SnapshotPendingInputs{Projects: projects, LoggedIn: map[string]bool{}, LookPath: exec.LookPath}
	if cfg, err := Load(home); err == nil {
		for n, p := range cfg.Profiles {
			if p.Type == "official" {
				in.LoggedIn[n] = HasLogin(home, n)
			}
		}
	}
	return in
}

// SnapshotPending mira lo que el snapshot TRAE —no lo que ya hay aquí— y dice
// qué queda a mano: iniciar sesión en cada perfil official y traer los
// comandos que sus MCP y sus hooks nombran. Se hace antes de aplicar porque
// entonces todavía es una lista de cosas que hacer, y no la explicación de por
// qué algo no funcionó.
func SnapshotPending(st *snapshot.Store, m *snapshot.Manifest, in SnapshotPendingInputs) []RestorePendingItem {
	out := []RestorePendingItem{}
	read := func(it snapshot.Item) []byte {
		data, err := st.GetBlob(it.Hash)
		if err != nil {
			// Sin datos no se puede comprobar nada. Callar es lo correcto:
			// inventar un pendiente sobre un archivo que no se ha leído es
			// mandar a instalar algo que quizá ya está.
			return nil
		}
		return data
	}
	for _, it := range m.Items {
		if it.LPath == "ccp/ccp.yaml" {
			out = append(out, pendingLogins(read(it), in)...)
		}
	}
	cmds := map[string]string{} // comando -> primera ruta lógica donde sale
	var orden []string
	note := func(cmd, where string) {
		if cmd == "" {
			return
		}
		if _, ya := cmds[cmd]; !ya {
			cmds[cmd], orden = where, append(orden, cmd)
		}
	}
	for _, it := range m.Items {
		for _, c := range snapshotCommands(it, read) {
			if miss := pendingCommand(c, in.LookPath); miss != "" {
				note(miss, it.LPath)
			}
		}
	}
	sort.Strings(orden)
	for _, c := range orden {
		out = append(out, RestorePendingItem{Kind: RestorePendingCommand, Name: c, Where: cmds[c]})
	}
	for _, p := range in.Projects {
		if p.Source == ProjectMapMissing {
			out = append(out, RestorePendingItem{Kind: RestorePendingProject, Name: p.Key, Where: p.Clone})
		}
	}
	return out
}

// pendingLogins lee el ccp.yaml del snapshot y saca los perfiles official que
// aquí no tienen sesión. Un ccp.yaml ilegible no produce pendientes: el
// restore lo rechazará antes por su cuenta.
func pendingLogins(data []byte, in SnapshotPendingInputs) []RestorePendingItem {
	if len(data) == 0 {
		return nil
	}
	cfg, err := configFromBytes(data)
	if err != nil {
		return nil
	}
	var names []string
	for n, p := range cfg.Profiles {
		if n != "default" && p.Type == "official" && !in.LoggedIn[n] {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	out := make([]RestorePendingItem, 0, len(names))
	for _, n := range names {
		out = append(out, RestorePendingItem{Kind: RestorePendingLogin, Name: n, Where: n})
	}
	return out
}

// snapshotCommands saca de un elemento del snapshot los comandos que nombra:
// los `command` de los MCP stdio, los de los hooks y el de la barra de estado.
// Solo se abren los archivos que pueden traerlos; leer y parsear cada CLAUDE.md
// para nada sería pagar el blob entero por archivo.
func snapshotCommands(it snapshot.Item, read func(snapshot.Item) []byte) []string {
	base := path.Base(it.LPath)
	var mcp, settings bool
	switch base {
	case ".claude.json", "claude_desktop_config.json", "mcp.json":
		mcp = true
	case "settings.json", "settings.overlay.json", "settings.local.json":
		settings = true
	default:
		return nil
	}
	data := read(it)
	if len(data) == 0 {
		return nil
	}
	var top map[string]any
	if err := json.Unmarshal(data, &top); err != nil {
		return nil
	}
	var out []string
	if mcp {
		for _, name := range invSortedKeys(invMCPMap(top)) {
			srv, _ := invMCPMap(top)[name].(map[string]any)
			if c, _ := srv["command"].(string); c != "" {
				out = append(out, c)
			}
		}
	}
	if settings {
		out = append(out, hookCommands(top)...)
	}
	return out
}

// hookCommands recorre hooks.<evento>[].hooks[].command y statusLine.command.
// Son líneas de shell, no rutas: el comando es el primer campo.
func hookCommands(top map[string]any) []string {
	var out []string
	hooks, _ := top["hooks"].(map[string]any)
	for _, ev := range invSortedKeys(hooks) {
		entries, _ := hooks[ev].([]any)
		for _, e := range entries {
			em, _ := e.(map[string]any)
			inner, _ := em["hooks"].([]any)
			for _, h := range inner {
				hm, _ := h.(map[string]any)
				if c, _ := hm["command"].(string); c != "" {
					out = append(out, firstWord(c))
				}
			}
		}
	}
	if sl, ok := top["statusLine"].(map[string]any); ok {
		if c, _ := sl["command"].(string); c != "" {
			out = append(out, firstWord(c))
		}
	}
	return out
}

// firstWord es el ejecutable de una línea de shell. No pretende ser un parser:
// con comillas o con una tubería delante se devuelve "" y no se inventa un
// pendiente sobre algo que no se ha entendido.
func firstWord(line string) string {
	line = strings.TrimSpace(line)
	if line == "" || strings.ContainsAny(line, "\"'`$|;&(){}<>") {
		return ""
	}
	if i := strings.IndexAny(line, " \t"); i >= 0 {
		return line[:i]
	}
	return line
}

// pendingCommand devuelve el comando si aquí no existe, o "". Comparte regla
// con el inventario (mcpMissingCommand): absoluto que no está, o relativo
// fuera del PATH; una variable sin expandir no cuenta.
func pendingCommand(cmd string, look func(string) (string, error)) string {
	if look == nil {
		look = exec.LookPath
	}
	return mcpMissingCommand(cmd, "", look)
}

// ProjectKey es la identidad portable de un proyecto (§11), la misma que usan
// los snapshots: el remoto de git normalizado, o la ruta si no tiene.
func ProjectKey(path, remote string) string { return projectKey(path, remote) }
