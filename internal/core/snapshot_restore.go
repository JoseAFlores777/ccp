package core

// snapshot_restore.go — el camino de vuelta de snapshot_layout.go: adónde va
// cada ruta lógica y cómo se escribe. Las rutas lógicas pueden venir de un
// .ccpsnap ajeno, así que cada una se valida contra la lista cerrada de
// destinos conocidos antes de tocar el disco.

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/JoseAFlores777/ccp/internal/snapshot"
)

// errProjectMissing: la carpeta del proyecto no existe en esta máquina.
var errProjectMissing = errors.New("la carpeta del proyecto no existe")

// snapTarget es el destino real de un elemento.
type snapTarget struct {
	path    string
	merge   bool   // .claude.json: se fusiona, no se sustituye
	ccpYAML bool   // ccp.yaml: se valida y se escribe bajo el lock
	profile string // perfil que hay que regenerar después, o ""
	global  bool   // toca lo compartido: se regeneran todos los perfiles
}

// snapshotTarget resuelve el destino de it en esta máquina, o error si la ruta
// lógica no es una de las conocidas.
func snapshotTarget(home, src string, it snapshot.Item) (snapTarget, error) {
	bad := fmt.Errorf("ruta lógica desconocida: %s", it.LPath)
	parts := strings.Split(it.LPath, "/")
	rest := func(from int) (string, error) {
		if from >= len(parts) {
			return "", bad
		}
		rel := filepath.FromSlash(strings.Join(parts[from:], "/"))
		if !filepath.IsLocal(rel) {
			return "", bad
		}
		return rel, nil
	}
	switch parts[0] {
	case "ccp":
		switch {
		case len(parts) == 2 && parts[1] == "ccp.yaml":
			return snapTarget{path: yamlPath(home), ccpYAML: true, global: true}, nil
		case len(parts) == 2 && parts[1] == "handoffs.yaml":
			return snapTarget{path: handoffsPath(home)}, nil
		case len(parts) >= 4 && parts[1] == "profiles" && validProfileName(parts[2]):
			n := parts[2]
			switch {
			case parts[3] == "overlay":
				rel, err := rest(4)
				return snapTarget{path: filepath.Join(cfgOverlayDir(home, n), rel), profile: n}, err
			case parts[3] == "api_key" && len(parts) == 4:
				return snapTarget{path: apiKeyPath(home, n), profile: n}, nil
			case parts[3] == "cc-home" && len(parts) == 5 && parts[4] == ".claude.json":
				return snapTarget{path: filepath.Join(ccHomePath(home, n), ".claude.json"), merge: true, profile: n}, nil
			case parts[3] == "cc-home" && len(parts) >= 6 && parts[4] == "projects":
				rel, err := rest(5)
				return snapTarget{path: filepath.Join(ccHomePath(home, n), "projects", rel)}, err
			}
		}
	case "claude":
		if len(parts) == 2 && parts[1] == ".claude.json" {
			return snapTarget{path: src + ".json", merge: true, global: true}, nil
		}
		rel, err := rest(1)
		return snapTarget{path: filepath.Join(src, rel), global: true}, err
	case "desktop":
		if len(parts) == 3 && parts[2] == "claude_desktop_config.json" && validProfileName(parts[1]) {
			dir, err := DesktopUserDataDir(home, parts[1])
			if err != nil {
				return snapTarget{}, err
			}
			return snapTarget{path: filepath.Join(dir, "claude_desktop_config.json")}, nil
		}
	case "project":
		if len(parts) < 3 || !slices.Contains(projectLocalFiles, strings.Join(parts[2:], "/")) {
			return snapTarget{}, bad
		}
		dir := it.Meta["path"]
		if !filepath.IsAbs(dir) {
			return snapTarget{}, fmt.Errorf("%s: el snapshot no dice dónde está el proyecto", it.LPath)
		}
		if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
			return snapTarget{}, errProjectMissing
		}
		return snapTarget{path: filepath.Join(dir, filepath.FromSlash(strings.Join(parts[2:], "/")))}, nil
	}
	return snapTarget{}, bad
}

// SnapshotRestoreOpts son las opciones de una restauración.
type SnapshotRestoreOpts struct {
	Only    []string // prefijos de ruta lógica; vacío = todo
	DryRun  bool
	Now     time.Time
	Machine string
	// Projects mapea la clave portable de un proyecto (§11) a su carpeta en
	// ESTA máquina. Lo decide el asistente de portabilidad, que es el único
	// que puede mirar este disco; sin entrada, vale la ruta del snapshot.
	Projects map[string]string
}

// SnapshotRestoreStep es lo que se hace (o no) con un elemento.
type SnapshotRestoreStep struct {
	LPath  string `json:"lpath"`
	Action string `json:"action"`           // write | merge | same | skip
	Reason string `json:"reason,omitempty"` // missing_blob | project_missing | invalid | unreadable
	// Meta viaja tal cual desde el elemento del manifiesto. Un paso de proyecto
	// solo se nombra con 12 hex de su clave, así que sin la ruta (y el remoto)
	// quien enseña el plan no puede decir en qué repo se va a escribir: dos
	// proyectos con regla serían dos casillas iguales.
	Meta map[string]string `json:"meta,omitempty"`
}

// SnapshotRestoreReport resume una restauración (o su plan, en dry-run).
type SnapshotRestoreReport struct {
	Snapshot    string                `json:"snapshot"`
	PreSnapshot string                `json:"pre_snapshot,omitempty"`
	Steps       []SnapshotRestoreStep `json:"steps"`
	Regenerated []string              `json:"regenerated"`
	// HomeFrom/HomeTo solo se rellenan cuando el snapshot viene de una máquina
	// con otro HOME: quien enseña el plan tiene que poder decir que las rutas
	// absolutas se reescriben, porque no es lo que el snapshot guardó.
	HomeFrom string `json:"home_from,omitempty"`
	HomeTo   string `json:"home_to,omitempty"`
}

func selectItems(items []snapshot.Item, only []string) []snapshot.Item {
	if len(only) == 0 {
		return items
	}
	var out []snapshot.Item
	for _, it := range items {
		for _, p := range only {
			p = strings.TrimSuffix(p, "/")
			if it.LPath == p || strings.HasPrefix(it.LPath, p+"/") {
				out = append(out, it)
				break
			}
		}
	}
	return out
}

// SnapshotRestore devuelve la configuración al estado del snapshot ref:
// primero el plan; luego un snapshot del estado actual (sin él no se escribe
// nada); luego la escritura y la regeneración de los perfiles afectados. No
// borra nada que exista en vivo y no esté en el snapshot.
func SnapshotRestore(home, src string, st *snapshot.Store, ref string, o SnapshotRestoreOpts) (*SnapshotRestoreReport, error) {
	m, err := st.LoadManifest(ref)
	if err != nil {
		return nil, err
	}
	items := selectItems(m.Items, o.Only)
	if len(items) == 0 {
		return nil, fmt.Errorf("ningún elemento del snapshot %s coincide con %s", snapshot.Short(m.ID), strings.Join(o.Only, ", "))
	}

	type pending struct {
		it   snapshot.Item
		tgt  snapTarget
		data []byte
	}
	// Otra máquina, otro HOME: las rutas absolutas se traducen al restaurar
	// (spec §11). Sin esto una regla de carpeta de /Users/ana no resuelve nada
	// aquí y el proyecto que la acompaña se salta por «project_missing».
	userHome, _ := os.UserHomeDir()
	homeFrom, homeTo := "", ""
	if m.Home != "" && userHome != "" && m.Home != userHome {
		homeFrom, homeTo = m.Home, userHome
	}
	rep := &SnapshotRestoreReport{
		Snapshot: m.ID, Steps: []SnapshotRestoreStep{}, Regenerated: []string{},
		HomeFrom: homeFrom, HomeTo: homeTo,
	}
	var todo []pending
	for _, it := range items {
		// La ruta del proyecto se traduce antes de resolver el destino: es lo
		// que decide si la carpeta existe en esta máquina.
		if p, ok := it.Meta["path"]; ok && homeFrom != "" {
			meta := maps.Clone(it.Meta)
			meta["path"] = translateHomePath(p, homeFrom, homeTo)
			it.Meta = meta
		}
		// Y después el mapeo, que manda sobre las dos: la persona ya dijo
		// dónde está ese repo aquí, y traducir el HOME no lo encuentra cuando
		// el clon cuelga de otra carpeta.
		if dir := projectMapped(it.LPath, o.Projects); dir != "" {
			meta := maps.Clone(it.Meta)
			if meta == nil {
				meta = map[string]string{}
			}
			meta["path"] = dir
			it.Meta = meta
		}
		step := SnapshotRestoreStep{LPath: it.LPath, Meta: it.Meta}
		tgt, err := snapshotTarget(home, src, it)
		switch {
		case errors.Is(err, errProjectMissing):
			step.Action, step.Reason = "skip", "project_missing"
		case err != nil:
			step.Action, step.Reason = "skip", "invalid"
		default:
			data, gerr := st.GetBlob(it.Hash)
			if gerr != nil {
				step.Action, step.Reason = "skip", "missing_blob"
				break
			}
			// Las conversaciones no se reescriben: son historia, y las rutas
			// que llevan dentro describen dónde ocurrió, no dónde escribir.
			if it.Class != snapshot.ClassState {
				data = translateHome(data, homeFrom, homeTo)
			}
			// Un ccp.yaml que este binario no puede leer aborta todo el restore
			// antes de tocar nada: restaurar el resto contra el ccp.yaml viejo
			// dejaría perfiles y reglas desparejados.
			if tgt.ccpYAML {
				if err := checkSnapshotCCPYAML(data); err != nil {
					return nil, err
				}
			}
			same, herr := targetHolds(tgt, data)
			switch {
			case herr != nil:
				step.Action, step.Reason = "skip", "unreadable"
			case same:
				step.Action = "same"
			default:
				step.Action = "write"
				if tgt.merge {
					step.Action = "merge"
				}
				todo = append(todo, pending{it, tgt, data})
			}
		}
		rep.Steps = append(rep.Steps, step)
	}
	if o.DryRun || len(todo) == 0 {
		return rep, nil
	}

	// La red: el estado actual queda en un snapshot antes de tocar nada. Si algo
	// de lo que se va a escribir es de clase state, la foto tiene que llevar el
	// estado vivo: un transcript se sustituye entero, y es lo único que ccp no
	// sabe reconstruir — sin esto la promesa de «esto se puede deshacer» es falsa
	// justo donde más duele.
	withState := false
	for _, p := range todo {
		if p.it.Class == snapshot.ClassState {
			withState = true
			break
		}
	}
	pre, err := SnapshotCapture(home, src, st, SnapshotCaptureOpts{Trigger: "pre-restore", WithState: withState, Now: o.Now, Machine: o.Machine})
	if err != nil && !errors.Is(err, snapshot.ErrNoChanges) {
		return nil, fmt.Errorf("no se pudo guardar el estado actual; no se restauró nada: %w", err)
	}
	rep.PreSnapshot = pre.ID

	// ccp.yaml primero: la regeneración de después se lee contra él.
	sort.SliceStable(todo, func(i, j int) bool { return todo[i].tgt.ccpYAML && !todo[j].tgt.ccpYAML })
	regen, all := map[string]bool{}, false
	for _, p := range todo {
		if err := applySnapshotItem(home, p.tgt, p.it, p.data); err != nil {
			return rep, fmt.Errorf("%s: %w (el estado anterior está en el snapshot %s)", p.it.LPath, err, snapshot.Short(pre.ID))
		}
		all = all || p.tgt.global
		if p.tgt.profile != "" {
			regen[p.tgt.profile] = true
		}
	}

	cfg, err := Load(home)
	if err != nil {
		return rep, err
	}
	names := make([]string, 0, len(cfg.Profiles))
	for n := range cfg.Profiles {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		if !all && !regen[n] {
			continue
		}
		if err := seedCCHome(home, n); err != nil {
			return rep, err
		}
		if err := CfgRegenerate(home, n, src); err != nil {
			return rep, err
		}
		rep.Regenerated = append(rep.Regenerated, n)
	}
	return rep, nil
}

// targetHolds dice si el destino ya contiene exactamente data. Para un
// .claude.json se compara solo su parte de configuración.
func targetHolds(tgt snapTarget, data []byte) (bool, error) {
	live, err := os.ReadFile(tgt.path)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !tgt.merge {
		return bytes.Equal(live, data), nil
	}
	sub, err := ClaudeJSONConfig(live)
	if err != nil {
		return false, err
	}
	return bytes.Equal(sub, data), nil
}

// checkSnapshotCCPYAML exige que el ccp.yaml del snapshot parsee y que su schema
// sea uno que este binario conoce.
func checkSnapshotCCPYAML(data []byte) error {
	c, err := configFromBytes(data)
	if err != nil {
		return fmt.Errorf("el ccp.yaml del snapshot no es válido: %w", err)
	}
	if c.Version > SchemaVersion {
		return fmt.Errorf("el ccp.yaml del snapshot usa schema %d y este ccp conoce hasta el %d; actualiza ccp", c.Version, SchemaVersion)
	}
	return nil
}

func applySnapshotItem(home string, tgt snapTarget, it snapshot.Item, data []byte) error {
	switch {
	case tgt.ccpYAML:
		if err := checkSnapshotCCPYAML(data); err != nil {
			return err
		}
		unlock, err := acquireLock(home)
		if err != nil {
			return err
		}
		defer unlock()
		return writeFileAtomic(tgt.path, data, 0o644)
	case tgt.merge:
		live, err := os.ReadFile(tgt.path)
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		out, err := ClaudeJSONApplyConfig(live, data)
		if err != nil {
			return err
		}
		return writeFileAtomic(tgt.path, out, 0o600)
	default:
		mode := os.FileMode(it.Mode) & 0o777
		if mode == 0 {
			mode = 0o644
		}
		if it.Class == snapshot.ClassSecret {
			mode = 0o600
		}
		return writeFileAtomic(tgt.path, data, mode)
	}
}

// projectMapped devuelve la carpeta elegida para el proyecto de esta ruta
// lógica, o "" si no hay mapeo (o la ruta no es de un proyecto).
func projectMapped(lpath string, projects map[string]string) string {
	if len(projects) == 0 {
		return ""
	}
	parts := strings.Split(lpath, "/")
	if len(parts) < 3 || parts[0] != "project" {
		return ""
	}
	return projects[parts[1]]
}
