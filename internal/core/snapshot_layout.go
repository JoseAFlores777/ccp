package core

// snapshot_layout.go — qué captura un snapshot en ESTA máquina (spec 2026-09-18
// §8.1). internal/snapshot no sabe nada del disco: aquí vive el mapa entre rutas
// lógicas y rutas reales, y es lo único que hay que tocar el día que ccp
// gestione un archivo nuevo. El camino de vuelta está en snapshot_restore.go.
//
//	ccp/ccp.yaml                                   authored
//	ccp/handoffs.yaml                              state (solo con WithState)
//	ccp/profiles/<p>/overlay/…                     authored
//	ccp/profiles/<p>/api_key                       secret
//	ccp/profiles/<p>/cc-home/.claude.json          secret: solo su configuración
//	ccp/profiles/<p>/cc-home/projects/…            state (solo con WithState)
//	claude/{settings.json,CLAUDE.md,keybindings.json}            authored
//	claude/{agents,commands,skills,output-styles,hooks}/…        authored
//	claude/plugins/{installed_plugins,known_marketplaces}.json   authored
//	claude/.claude.json                            secret: solo su configuración
//	desktop/<p>/claude_desktop_config.json         secret (default incluido)
//	project/<clave>/{.claude/settings.local.json,CLAUDE.local.md}  authored
//
// Lo generado (cc-home/settings.json y CLAUDE.md, espejos, lanzadores), la caché
// y lo atado a la máquina (tokens, machineID, sesión de Desktop) no se capturan
// nunca: se regeneran o se recrean.

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/JoseAFlores777/ccp/internal/snapshot"
)

// SnapshotStoreDir es el almacén local de snapshots: <home>/snapshots.
func SnapshotStoreDir(home string) string { return filepath.Join(home, "snapshots") }

// OpenSnapshotStore abre (o crea) el almacén local.
func OpenSnapshotStore(home string) (*snapshot.Store, error) {
	return snapshot.Open(SnapshotStoreDir(home))
}

// SnapshotSourceOpts elige qué se captura además de la configuración.
type SnapshotSourceOpts struct {
	// WithState añade conversaciones (cc-home/projects) y handoffs.yaml. Pesan y
	// son sensibles, por eso no van por defecto (spec D4).
	WithState bool
}

// Del ~/.claude global, la configuración del usuario. De plugins/ entran solo
// sus dos índices: el resto es caché que se vuelve a descargar.
var (
	claudeGlobalFiles = []string{"settings.json", "CLAUDE.md", "keybindings.json",
		"plugins/installed_plugins.json", "plugins/known_marketplaces.json"}
	claudeGlobalTrees = []string{"agents", "commands", "skills", "output-styles", "hooks"}
	// Lo local (no versionado) de un proyecto. Lo versionado ya tiene su historia en git.
	projectLocalFiles = []string{".claude/settings.local.json", "CLAUDE.local.md"}
)

// SnapshotSources enumera lo que un snapshot captura en esta máquina.
func SnapshotSources(home, src string, o SnapshotSourceOpts) ([]snapshot.Source, error) {
	cfg, err := Load(home)
	if err != nil {
		return nil, err
	}
	var (
		out      []snapshot.Source
		firstErr error
	)
	keep := func(ss []snapshot.Source, err error) {
		out = append(out, ss...)
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}
	file := func(lpath, abs string, class snapshot.Class) {
		keep(fileSource(lpath, abs, class, nil))
	}

	file("ccp/ccp.yaml", yamlPath(home), snapshot.ClassAuthored)
	if o.WithState {
		file("ccp/handoffs.yaml", handoffsPath(home), snapshot.ClassState)
	}

	names := make([]string, 0, len(cfg.Profiles))
	for n := range cfg.Profiles {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		base := "ccp/profiles/" + n
		keep(treeSources(base+"/overlay", cfgOverlayDir(home, n), snapshot.ClassAuthored))
		file(base+"/api_key", apiKeyPath(home, n), snapshot.ClassSecret)
		keep(claudeJSONSource(base+"/cc-home/.claude.json", filepath.Join(ccHomePath(home, n), ".claude.json")))
		file("desktop/"+n+"/claude_desktop_config.json", filepath.Join(DesktopDataDir(home, n), "claude_desktop_config.json"), snapshot.ClassSecret)
		if o.WithState {
			keep(treeSources(base+"/cc-home/projects", filepath.Join(ccHomePath(home, n), "projects"), snapshot.ClassState))
		}
	}

	for _, f := range claudeGlobalFiles {
		file("claude/"+f, filepath.Join(src, filepath.FromSlash(f)), snapshot.ClassAuthored)
	}
	for _, d := range claudeGlobalTrees {
		keep(treeSources("claude/"+d, filepath.Join(src, d), snapshot.ClassAuthored))
	}
	// El .claude.json del perfil default vive junto a ~/.claude, no dentro.
	keep(claudeJSONSource("claude/.claude.json", src+".json"))
	if dir, err := DesktopUserDataDir(home, "default"); err == nil {
		file("desktop/default/claude_desktop_config.json", filepath.Join(dir, "claude_desktop_config.json"), snapshot.ClassSecret)
	}
	keep(projectSources(cfg.Rules))

	if firstErr != nil {
		return nil, firstErr
	}
	sort.Slice(out, func(i, j int) bool { return out[i].LPath < out[j].LPath })
	return out, nil
}

// fileSource devuelve la fuente de un archivo regular, o nada si no existe o no
// es regular. Un symlink nunca se sigue: apunta a algo que se captura por su
// propia ruta o que no es del usuario.
func fileSource(lpath, abs string, class snapshot.Class, meta map[string]string) ([]snapshot.Source, error) {
	fi, err := os.Lstat(abs)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("no se pudo inspeccionar %s: %w", abs, err)
	}
	if !fi.Mode().IsRegular() {
		return nil, nil
	}
	return []snapshot.Source{{
		LPath: lpath, Class: class, Mode: uint32(fi.Mode().Perm()), Meta: meta,
		Read: func() ([]byte, error) { return os.ReadFile(abs) },
	}}, nil
}

// treeSources devuelve una fuente por archivo regular bajo dir, con ruta lógica
// prefix/<relativa>. No sigue symlinks (tampoco si dir lo es) ni entra en .git.
func treeSources(prefix, dir string, class snapshot.Class) ([]snapshot.Source, error) {
	fi, err := os.Lstat(dir)
	if errors.Is(err, fs.ErrNotExist) || (err == nil && !fi.IsDir()) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("no se pudo inspeccionar %s: %w", dir, err)
	}
	var out []snapshot.Source
	err = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		ss, err := fileSource(prefix+"/"+filepath.ToSlash(rel), p, class, nil)
		out = append(out, ss...)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("no se pudo recorrer %s: %w", dir, err)
	}
	return out, nil
}

// claudeJSONSource es la fuente de la parte de configuración de un
// .claude.json. Se marca con meta merge=claude-json: al restaurar se fusiona.
func claudeJSONSource(lpath, abs string) ([]snapshot.Source, error) {
	data, err := os.ReadFile(abs)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("no se pudo leer %s: %w", abs, err)
	}
	sub, err := ClaudeJSONConfig(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", abs, err)
	}
	if sub == nil {
		return nil, nil
	}
	return []snapshot.Source{{
		LPath: lpath, Class: snapshot.ClassSecret, Mode: 0o600,
		Meta: map[string]string{"merge": "claude-json"},
		Read: func() ([]byte, error) { return sub, nil },
	}}, nil
}

// projectSources recorre las carpetas con regla y captura sus archivos locales.
// La clave del proyecto sale del remoto de git (el mismo repo en otra ruta u
// otra máquina es el mismo proyecto) o, sin remoto, de la ruta. Si dos clones
// del mismo remoto tienen regla, el segundo se identifica por su ruta para que
// no choquen.
func projectSources(rules []Rule) ([]snapshot.Source, error) {
	var out []snapshot.Source
	used := map[string]string{} // clave -> ruta que la usa
	for _, r := range rules {
		remote := gitOriginURL(r.Path)
		key := projectKey(r.Path, remote)
		if owner, taken := used[key]; taken && owner != r.Path {
			key = projectKey(r.Path, "")
		}
		if _, dup := used[key]; dup {
			continue
		}
		used[key] = r.Path
		meta := map[string]string{"path": r.Path}
		if remote != "" {
			meta["remote"] = remote
		}
		for _, rel := range projectLocalFiles {
			ss, err := fileSource("project/"+key+"/"+rel, filepath.Join(r.Path, filepath.FromSlash(rel)), snapshot.ClassAuthored, meta)
			if err != nil {
				return nil, err
			}
			out = append(out, ss...)
		}
	}
	return out, nil
}

// projectKey es la identidad portable de un proyecto: 12 hex del sha256 de su
// remoto normalizado, o de su ruta si no tiene.
func projectKey(path, remote string) string {
	id := path
	if remote != "" {
		id = normalizeRemote(remote)
	}
	sum := sha256.Sum256([]byte(id))
	return hex.EncodeToString(sum[:])[:12]
}

// normalizeRemote hace equivalentes las formas de escribir un mismo remoto:
// git@github.com:Org/App.git, https://github.com/org/app y
// ssh://git@github.com/Org/App.git son github.com/org/app.
func normalizeRemote(u string) string {
	u = strings.ToLower(strings.TrimSpace(u))
	u = strings.TrimSuffix(u, ".git")
	for _, p := range []string{"https://", "http://", "ssh://", "git://"} {
		u = strings.TrimPrefix(u, p)
	}
	if at := strings.Index(u, "@"); at >= 0 {
		u = u[at+1:]
	}
	return strings.Replace(u, ":", "/", 1)
}

// gitOriginURL lee la url de «origin» de <dir>/.git/config sin ejecutar git.
// Si dir no es la raíz de un repo (o es un worktree, con .git como archivo),
// devuelve "" y el proyecto se identifica por su ruta.
func gitOriginURL(dir string) string {
	f, err := os.Open(filepath.Join(dir, ".git", "config"))
	if err != nil {
		return ""
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	inOrigin := false
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, "[") {
			inOrigin = line == `[remote "origin"]`
			continue
		}
		if !inOrigin {
			continue
		}
		if k, v, ok := strings.Cut(line, "="); ok && strings.TrimSpace(k) == "url" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// SnapshotCaptureOpts son las opciones de una captura.
type SnapshotCaptureOpts struct {
	Trigger   string // manual | daily | pre-restore | pre-profile-rm | pre-backup-restore | …
	Label     string
	WithState bool
	Force     bool // captura aunque no haya cambios
	Now       time.Time
	Machine   string
}

// SnapshotCapture toma un snapshot del estado vivo. Si no cambió nada desde el
// último devuelve ese último y snapshot.ErrNoChanges (salvo Force).
func SnapshotCapture(home, src string, st *snapshot.Store, o SnapshotCaptureOpts) (*snapshot.Manifest, error) {
	srcs, err := SnapshotSources(home, src, SnapshotSourceOpts{WithState: o.WithState})
	if err != nil {
		return nil, err
	}
	// El HOME del usuario (no el de ccp): con él, otra máquina sabe qué prefijo
	// de las rutas absolutas tiene que traducir.
	userHome, _ := os.UserHomeDir()
	return snapshot.Capture(st, srcs, snapshot.Meta{
		Created: o.Now, Machine: o.Machine, Home: userHome, CCPVersion: Version, Trigger: o.Trigger, Label: o.Label,
	}, o.Force)
}

// SnapshotLive calcula los elementos del estado vivo sin guardar nada.
func SnapshotLive(home, src string, o SnapshotSourceOpts) ([]snapshot.Item, error) {
	srcs, err := SnapshotSources(home, src, o)
	if err != nil {
		return nil, err
	}
	return snapshot.ItemsOf(srcs)
}
