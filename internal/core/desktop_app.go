package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

// desktop_app.go — un lanzador de Claude Desktop con nombre e icono propios
// por perfil: `~/Applications/Claude (<perfil>).app`.
//
// El problema que resuelve: `ccp desktop open` abre instancias aisladas, pero
// todas son el MISMO bundle, así que el Dock, Cmd-Tab y Spotlight las enseñan
// como «Claude» con el mismo icono y no hay forma de saber cuál es cuál. La
// solución obvia —copiar Claude.app y retocar su Info.plist— rompe el Keychain:
// «Claude Safe Storage» (la clave con la que Chromium cifra cookies y sesión)
// tiene una ACL que exige que el proceso valide como código firmado, y un
// bundle con el Info.plist tocado, o cuyo ejecutable en marcha no sea el
// CFBundleExecutable, deja de validar (errSecAuthFailed) y la cuenta se pierde.
//
// La forma que satisface todo a la vez es un bundle de DOS capas:
//
//	Claude (work).app/                       ← lo que ve LaunchServices
//	  Contents/Info.plist                    ← copia PARCHEADA: id propio, nombre,
//	                                           icono, sin claude:// (ver desktopAppPlist)
//	  Contents/MacOS/Claude                  ← CFBundleExecutable: hard link al binario de ccp
//	  Contents/MacOS/Claude-run -> ../ccp/Claude/Contents/MacOS/Claude
//	  Contents/Resources/ccp-icon.icns       ← el icono de Claude.app, tintado
//	  Contents/ccp/Claude/…                  ← espejo PRÍSTINO de Claude.app
//	                                           (directorios reales, archivos hard-link)
//	  Contents/ccp-desktop.json              ← manifiesto (perfil, color, versión…)
//
// LaunchServices lanza `MacOS/Claude`, que es ccp en modo lanzador: calcula el
// entorno del perfil (EnvForChild) y hace exec de `MacOS/Claude-run` con
// `--user-data-dir`. A partir de ahí cada pieza del sistema mira una capa
// distinta, y eso es lo que hace que funcione:
//
//   - LaunchServices/Dock identifican el proceso por el bundle más interno que
//     contiene su ejecutable SEGÚN LA RUTA CON LA QUE SE HIZO EXEC (sin resolver
//     symlinks): `…/Contents/MacOS/Claude-run` está en la capa externa, así que
//     el nombre, el icono y el bundle id son los del lanzador.
//   - Security.framework (Keychain, TCC) valida el proceso por su ruta REAL
//     (proc_pidpath resuelve el symlink): `Contents/ccp/Claude/Contents/MacOS/
//     Claude`, dentro de un bundle idéntico al original —mismo Info.plist, mismo
//     CFBundleExecutable, mismos bytes (hard links)—, que valida igual que
//     /Applications/Claude.app. La sesión se conserva.
//   - Los procesos auxiliares de Chromium corren en un sandbox cuyo BUNDLE_PATH
//     es el bundle principal (la capa externa); como el espejo está DENTRO, la
//     regla `subpath` lo cubre. Con symlinks hacia /Applications no lo cubría
//     y el GPU y la red morían con «Unable to find helper app».
//   - Electron valida app.asar contra `ElectronAsarIntegrity` del bundle
//     principal, con la ruta relativa a su Contents/: por eso el plist externo
//     lleva la entrada duplicada bajo `ccp/Claude/Contents/Resources/app.asar`.
//   - El ejecutable externo es un Mach-O (ccp), no un script: con un script
//     LaunchServices no puede leer la arquitectura y lanza bajo Rosetta, y ahí
//     Chromium no arranca. Y es un HARD LINK (o copia), no un symlink: a un
//     ejecutable symlinkeado LaunchServices directamente no lo lanza —`open`
//     devuelve 0 y no arranca nada.
//
// Actualizaciones: los hard links fijan los inodos de la versión con la que se
// construyó el espejo, así que cuando Claude.app se actualiza el lanzador
// sigue funcionando (con la versión vieja) hasta que ccp lo reconstruye —cosa
// que el propio modo lanzador hace en el siguiente arranque al ver que
// CFBundleVersion cambió (DesktopAppStale). La instancia lanzada no puede
// actualizarse a sí misma: Squirrel busca en la descarga un bundle con el id
// de la app en marcha (el del lanzador, que es propio) y no lo encuentra.
// Quien actualiza es la instancia normal de Claude.app, como siempre.

const (
	desktopAppManifestName = "ccp-desktop.json"
	desktopAppNestedRel    = "ccp/Claude" // bajo Contents/: el espejo prístino
	desktopAppIconName     = "ccp-icon.icns"
	desktopAppExecName     = "Claude"     // CFBundleExecutable: hard link al binario ccp
	desktopAppRunName      = "Claude-run" // symlink al stub del espejo
	desktopAppBundleIDBase = "com.anthropic.claudefordesktop.ccp."
	desktopAppSuffix       = ".app"
)

// DesktopAppManifest es lo que el lanzador recuerda de sí mismo. Vive dentro
// del bundle y no en ccp.yaml a propósito: es estado derivado (se regenera
// entero) y así un ccp anterior no lo pierde al reescribir la config.
type DesktopAppManifest struct {
	Profile       string `json:"profile"`
	Label         string `json:"label"`
	Color         string `json:"color"`
	BundleID      string `json:"bundle_id"`
	SourceApp     string `json:"source_app"`
	SourceVersion string `json:"source_version"`
	CCPBin        string `json:"ccp_bin"`
	CCPBinStamp   string `json:"ccp_bin_stamp"` // tamaño-mtime del binario enlazado
	Home          string `json:"home"`
	Generator     string `json:"generator"`
	Built         string `json:"built"`
}

// DesktopApp es un lanzador encontrado en disco.
type DesktopApp struct {
	Path     string
	Manifest DesktopAppManifest
}

// DesktopAppsDir es donde viven los lanzadores: CCP_DESKTOP_APPS_DIR o
// ~/Applications, que es lo que Spotlight y Launchpad indexan.
func DesktopAppsDir() (string, error) {
	if d := os.Getenv("CCP_DESKTOP_APPS_DIR"); d != "" {
		return d, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("no se pudo determinar HOME: %w", err)
	}
	return filepath.Join(home, "Applications"), nil
}

// DefaultDesktopLabel es el nombre con el que aparece el lanzador si el
// usuario no elige otro. Es también el nombre del directorio .app: Finder y el
// Dock enseñan el nombre en disco, así que los dos tienen que coincidir.
func DefaultDesktopLabel(profile string) string {
	return "Claude (" + profile + ")"
}

var bundleIDUnsafe = regexp.MustCompile(`[^A-Za-z0-9-]+`)

// DesktopBundleID deriva un identificador propio y estable por perfil. Que sea
// DISTINTO del de Claude.app es lo que impide dos cosas: que LaunchServices
// confunda el lanzador con la app al resolver `open -b` o claude://, y que la
// instancia se actualice a sí misma (Squirrel exige que el bundle de la
// descarga tenga el id de la app en marcha).
func DesktopBundleID(profile string) string {
	part := strings.Trim(bundleIDUnsafe.ReplaceAllString(profile, "-"), "-")
	if part == "" {
		part = "profile"
	}
	return desktopAppBundleIDBase + part
}

func desktopAppManifestPath(app string) string {
	return filepath.Join(app, "Contents", desktopAppManifestName)
}

// LoadDesktopApp lee el manifiesto de un .app; error si no es un lanzador de ccp.
func LoadDesktopApp(path string) (*DesktopApp, error) {
	b, err := os.ReadFile(desktopAppManifestPath(path))
	if err != nil {
		return nil, err
	}
	var m DesktopAppManifest
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("manifiesto ilegible en %s: %w", path, err)
	}
	if m.Profile == "" {
		return nil, fmt.Errorf("manifiesto sin perfil en %s", path)
	}
	return &DesktopApp{Path: path, Manifest: m}, nil
}

// ListDesktopApps devuelve los lanzadores de ccp que hay en appsDir. Un .app
// ajeno (sin manifiesto) simplemente no cuenta: nunca se toca.
func ListDesktopApps(appsDir string) ([]*DesktopApp, error) {
	entries, err := os.ReadDir(appsDir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var out []*DesktopApp
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), desktopAppSuffix) || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		app, lerr := LoadDesktopApp(filepath.Join(appsDir, e.Name()))
		if lerr != nil {
			continue
		}
		out = append(out, app)
	}
	return out, nil
}

// FindDesktopApp busca el lanzador de un perfil; (nil, nil) si no lo hay.
func FindDesktopApp(appsDir, profile string) (*DesktopApp, error) {
	apps, err := ListDesktopApps(appsDir)
	if err != nil {
		return nil, err
	}
	for _, a := range apps {
		if a.Manifest.Profile == profile {
			return a, nil
		}
	}
	return nil, nil
}

// DesktopAppOptions es la entrada de BuildDesktopApp.
type DesktopAppOptions struct {
	Home      string
	Profile   string
	Cfg       *Config
	AppsDir   string
	SourceApp string // Claude.app (resuelto por el llamador: --app, CCP_DESKTOP_APP, autodetección)
	Label     string // "" = el actual si existe, si no DefaultDesktopLabel
	Color     string // "" = el actual si existe, si no el primero libre de la paleta
	CCPBin    string // ruta absoluta del binario que hará de lanzador
	Generator string // versión de ccp, informativo
	Force     bool   // reconstruir aunque parezca al día
	Now       func() time.Time
}

// DesktopAppResult dice qué pasó.
type DesktopAppResult struct {
	App     *DesktopApp
	Changed bool
	Reason  string // "created" | "refreshed" | "unchanged"
}

// BuildDesktopApp crea o reconstruye el lanzador del perfil. Es idempotente:
// si ya existe, está al día y no cambian nombre ni color, no toca nada.
//
// La reconstrucción se hace en un directorio temporal HERMANO (mismo volumen,
// que los hard links lo exigen) y se intercambia al final: el lanzador nunca
// está a medias en la ruta que LaunchServices conoce.
func BuildDesktopApp(o DesktopAppOptions) (*DesktopAppResult, error) {
	if o.Profile == "default" {
		return nil, fmt.Errorf("'default' no tiene lanzador: su instancia es la app normal")
	}
	if err := DesktopEligible(o.Cfg, o.Profile); err != nil {
		return nil, err
	}
	if o.AppsDir == "" || o.SourceApp == "" || o.CCPBin == "" || o.Home == "" {
		return nil, fmt.Errorf("BuildDesktopApp: faltan AppsDir/SourceApp/CCPBin/Home")
	}
	if !filepath.IsAbs(o.CCPBin) {
		return nil, fmt.Errorf("el binario de ccp debe ser una ruta absoluta: %s", o.CCPBin)
	}
	if !fileExists(o.CCPBin) {
		return nil, fmt.Errorf("no existe el binario de ccp: %s", o.CCPBin)
	}
	now := o.Now
	if now == nil {
		now = time.Now
	}

	src, err := readDesktopSource(o.SourceApp)
	if err != nil {
		return nil, err
	}

	existing, err := FindDesktopApp(o.AppsDir, o.Profile)
	if err != nil {
		return nil, err
	}

	label := strings.TrimSuffix(strings.TrimSpace(o.Label), desktopAppSuffix)
	if label == "" && existing != nil {
		label = existing.Manifest.Label
	}
	if label == "" {
		label = DefaultDesktopLabel(o.Profile)
	}
	if strings.ContainsAny(label, "/:") || strings.HasPrefix(label, ".") {
		return nil, fmt.Errorf("nombre de lanzador inválido %q (sin '/', ':' ni punto inicial)", label)
	}

	color, err := resolveDesktopColor(o, existing)
	if err != nil {
		return nil, err
	}

	target := filepath.Join(o.AppsDir, label+desktopAppSuffix)
	if fi, serr := os.Lstat(target); serr == nil {
		if fi.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("%s es un symlink; no se sustituye", target)
		}
		if _, lerr := LoadDesktopApp(target); lerr != nil {
			return nil, fmt.Errorf("%s ya existe y no es un lanzador de ccp; no se toca", target)
		}
	}

	m := DesktopAppManifest{
		Profile:       o.Profile,
		Label:         label,
		Color:         color.Name,
		BundleID:      DesktopBundleID(o.Profile),
		SourceApp:     o.SourceApp,
		SourceVersion: src.version,
		CCPBin:        o.CCPBin,
		CCPBinStamp:   fileStamp(o.CCPBin),
		Home:          o.Home,
		Generator:     o.Generator,
		Built:         now().UTC().Format(time.RFC3339),
	}

	if !o.Force && existing != nil && existing.Path == target && desktopManifestSame(existing.Manifest, m) {
		if _, stale := DesktopAppStale(existing, o.SourceApp); !stale {
			return &DesktopAppResult{App: existing, Changed: false, Reason: "unchanged"}, nil
		}
	}

	if err := os.MkdirAll(o.AppsDir, 0o755); err != nil {
		return nil, fmt.Errorf("no se pudo crear %s: %w", o.AppsDir, err)
	}
	tmp, err := os.MkdirTemp(o.AppsDir, ".ccp-build-")
	if err != nil {
		return nil, fmt.Errorf("no se pudo crear el temporal en %s: %w", o.AppsDir, err)
	}
	ok := false
	defer func() {
		if !ok {
			_ = os.RemoveAll(tmp)
		}
	}()

	if err := assembleDesktopApp(tmp, src, m); err != nil {
		return nil, err
	}

	// Intercambio. Un lanzador con otro nombre (cambio de --label) se retira
	// después de tener el nuevo montado, nunca antes.
	if existing != nil && existing.Path != target {
		if err := RemoveDesktopApp(o.AppsDir, existing); err != nil {
			return nil, err
		}
	}
	reason := "created"
	if _, serr := os.Lstat(target); serr == nil {
		reason = "refreshed"
		old := filepath.Join(o.AppsDir, "."+filepath.Base(target)+".ccp-old-"+fmt.Sprint(now().UnixNano()))
		if err := os.Rename(target, old); err != nil {
			return nil, fmt.Errorf("no se pudo retirar el lanzador anterior: %w", err)
		}
		if err := os.Rename(tmp, target); err != nil {
			_ = os.Rename(old, target)
			return nil, fmt.Errorf("no se pudo colocar el lanzador nuevo: %w", err)
		}
		_ = os.RemoveAll(old)
	} else if err := os.Rename(tmp, target); err != nil {
		return nil, fmt.Errorf("no se pudo colocar el lanzador en %s: %w", target, err)
	}
	ok = true

	// LaunchServices cachea Info.plist e icono por bundle; tocar el directorio
	// y volver a registrarlo es lo que hace que el Dock enseñe el nombre y el
	// color nuevos sin reiniciar sesión.
	t := now()
	_ = os.Chtimes(target, t, t)
	lsregister("-f", target)

	app, err := LoadDesktopApp(target)
	if err != nil {
		return nil, err
	}
	return &DesktopAppResult{App: app, Changed: true, Reason: reason}, nil
}

// desktopManifestSame compara lo que decide la forma del lanzador (no la fecha
// ni el generador, que cambian en cada build sin cambiar nada).
func desktopManifestSame(a, b DesktopAppManifest) bool {
	return a.Profile == b.Profile && a.Label == b.Label && a.Color == b.Color &&
		a.BundleID == b.BundleID && a.SourceApp == b.SourceApp &&
		a.SourceVersion == b.SourceVersion && a.CCPBin == b.CCPBin && a.CCPBinStamp == b.CCPBinStamp && a.Home == b.Home
}

// resolveDesktopColor aplica la precedencia --color > el actual > paleta.
// La asignación automática evita los colores que ya usan otros lanzadores,
// que es la única razón de tener una paleta ordenada.
func resolveDesktopColor(o DesktopAppOptions, existing *DesktopApp) (DesktopColor, error) {
	if o.Color != "" {
		return ParseDesktopColor(o.Color)
	}
	if existing != nil && existing.Manifest.Color != "" {
		if c, err := ParseDesktopColor(existing.Manifest.Color); err == nil {
			return c, nil
		}
	}
	apps, _ := ListDesktopApps(o.AppsDir)
	used := map[string]bool{}
	for _, a := range apps {
		if a.Manifest.Profile != o.Profile {
			used[a.Manifest.Color] = true
		}
	}
	return pickDesktopColor(used), nil
}

// pickDesktopColor devuelve el primer color de la paleta que no está en uso.
// El identity (orange) nunca se asigna solo: sería el icono de la app normal.
func pickDesktopColor(used map[string]bool) DesktopColor {
	var first DesktopColor
	for i, c := range DesktopPalette {
		if c.Identity {
			continue
		}
		if i == 0 {
			first = c
		}
		if !used[c.Name] {
			return c
		}
	}
	return first
}

// desktopSource es lo que hace falta leer de Claude.app.
type desktopSource struct {
	app      string
	plist    *plistDict
	exeName  string
	version  string
	iconPath string // "" si el bundle no trae .icns
}

func readDesktopSource(app string) (*desktopSource, error) {
	p, err := readPlist(filepath.Join(app, "Contents", "Info.plist"))
	if err != nil {
		return nil, fmt.Errorf("no se pudo leer el Info.plist de %s: %w", app, err)
	}
	exe := p.String("CFBundleExecutable")
	if exe == "" {
		exe = "Claude"
	}
	if _, err := os.Stat(filepath.Join(app, "Contents", "MacOS", exe)); err != nil {
		return nil, fmt.Errorf("%s no tiene ejecutable Contents/MacOS/%s: %w", app, exe, err)
	}
	version := p.String("CFBundleVersion")
	if version == "" {
		version = p.String("CFBundleShortVersionString")
	}
	src := &desktopSource{app: app, plist: p, exeName: exe, version: version}
	icon := p.String("CFBundleIconFile")
	if icon == "" {
		icon = "electron.icns"
	}
	if !strings.HasSuffix(icon, ".icns") {
		icon += ".icns"
	}
	if ip := filepath.Join(app, "Contents", "Resources", icon); fileExists(ip) {
		src.iconPath = ip
	}
	return src, nil
}

// desktopAppPlist deriva el Info.plist de la capa externa. Se parte del
// original entero (LSEnvironment, NS*UsageDescription, tipos de documento…) y
// se cambia solo lo que identifica al lanzador:
//
//   - id, nombre y nombre visible propios; el ejecutable es el nuestro;
//   - el icono es el nuestro y se quita CFBundleIconName, que en macOS 26
//     apuntaría al de Assets.car y ganaría al .icns;
//   - se quitan los CFBundleURLTypes: claude:// y msauth siguen siendo de
//     Claude.app, y así un lanzador no secuestra los enlaces de la app normal;
//   - ElectronAsarIntegrity gana la entrada con la ruta del espejo anidado.
func desktopAppPlist(src *plistDict, m DesktopAppManifest) *plistDict {
	p := src.Clone()
	p.Set("CFBundleIdentifier", m.BundleID)
	p.Set("CFBundleName", m.Label)
	p.Set("CFBundleDisplayName", m.Label)
	p.Set("CFBundleExecutable", desktopAppExecName)
	p.Set("CFBundleIconFile", desktopAppIconName)
	p.Delete("CFBundleIconName")
	p.Delete("CFBundleURLTypes")
	if integ := p.Dict("ElectronAsarIntegrity"); integ != nil {
		for _, k := range integ.Keys() {
			if strings.HasPrefix(k, desktopAppNestedRel+"/") {
				continue
			}
			v, _ := integ.Get(k)
			integ.Set(desktopAppNestedRel+"/Contents/"+k, clonePlistValue(v))
		}
	}
	return p
}

// assembleDesktopApp monta el bundle completo en dst (un directorio vacío).
func assembleDesktopApp(dst string, src *desktopSource, m DesktopAppManifest) error {
	contents := filepath.Join(dst, "Contents")
	macos := filepath.Join(contents, "MacOS")
	resources := filepath.Join(contents, "Resources")
	nested := filepath.Join(contents, filepath.FromSlash(desktopAppNestedRel))
	for _, d := range []string{macos, resources, filepath.Dir(nested)} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return fmt.Errorf("no se pudo crear %s: %w", d, err)
		}
	}

	if err := hardlinkTree(src.app, nested); err != nil {
		return fmt.Errorf("no se pudo espejar %s: %w", src.app, err)
	}

	// El lanzador es el binario de ccp, enlazado (mismo inodo, cero disco) o
	// copiado si el volumen no permite el link. Nunca symlink: ver la cabecera.
	launcher := filepath.Join(macos, desktopAppExecName)
	if err := os.Link(m.CCPBin, launcher); err != nil {
		if cerr := copyFileMode(m.CCPBin, launcher); cerr != nil {
			return fmt.Errorf("no se pudo enlazar ni copiar %s: %w", m.CCPBin, cerr)
		}
	}
	if err := os.Chmod(launcher, 0o755); err != nil {
		return err
	}
	runTarget := filepath.ToSlash(filepath.Join("..", desktopAppNestedRel, "Contents", "MacOS", src.exeName))
	if err := os.Symlink(runTarget, filepath.Join(macos, desktopAppRunName)); err != nil {
		return err
	}

	if err := os.WriteFile(filepath.Join(contents, "Info.plist"), marshalPlist(desktopAppPlist(src.plist, m)), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(contents, "PkgInfo"), []byte("APPL????"), 0o644); err != nil {
		return err
	}

	if src.iconPath != "" {
		raw, err := os.ReadFile(src.iconPath)
		if err != nil {
			return err
		}
		color, cerr := ParseDesktopColor(m.Color)
		if cerr != nil {
			return cerr
		}
		icon, terr := TintICNS(raw, color)
		if terr != nil {
			// Un .icns que no sabemos tintar (sin trozos PNG) se copia tal
			// cual: el lanzador queda con el icono normal, pero con nombre.
			icon = raw
		}
		if err := os.WriteFile(filepath.Join(resources, desktopAppIconName), icon, 0o644); err != nil {
			return err
		}
	}

	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(contents, desktopAppManifestName), append(b, '\n'), 0o644)
}

// hardlinkTree replica src en dst: directorios reales, archivos como hard link
// (o copia si el volumen no lo permite), symlinks recreados con el mismo
// destino. dst no debe existir.
func hardlinkTree(src, dst string) error {
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, rerr := filepath.Rel(src, p)
		if rerr != nil {
			return rerr
		}
		target := filepath.Join(dst, rel)
		switch {
		case d.Type()&fs.ModeSymlink != 0:
			link, lerr := os.Readlink(p)
			if lerr != nil {
				return lerr
			}
			return os.Symlink(link, target)
		case d.IsDir():
			info, ierr := d.Info()
			if ierr != nil {
				return ierr
			}
			return os.MkdirAll(target, info.Mode().Perm()|0o700)
		default:
			if lerr := os.Link(p, target); lerr == nil {
				return nil
			}
			return copyFileMode(p, target)
		}
	})
}

func copyFileMode(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	info, err := in.Stat()
	if err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, info.Mode().Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

// fileStamp identifica el contenido de un archivo por tamaño y mtime: basta
// para saber si el binario de ccp que el lanzador enlaza es el instalado.
func fileStamp(p string) string {
	fi, err := os.Stat(p)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("%d-%d", fi.Size(), fi.ModTime().UnixNano())
}

// DesktopAppStale dice si el lanzador hay que reconstruirlo, y por qué. Se
// llama en cada arranque desde el modo lanzador: es lo que hace que una
// actualización de Claude.app llegue a los lanzadores sin que nadie lo pida.
func DesktopAppStale(app *DesktopApp, sourceApp string) (string, bool) {
	src, err := readDesktopSource(sourceApp)
	if err != nil {
		return "no se puede leer " + sourceApp, true
	}
	if src.version != app.Manifest.SourceVersion {
		return fmt.Sprintf("Claude.app pasó de %s a %s", app.Manifest.SourceVersion, src.version), true
	}
	nestedExe := filepath.Join(app.Path, "Contents", filepath.FromSlash(desktopAppNestedRel), "Contents", "MacOS", src.exeName)
	if !fileExists(nestedExe) {
		return "falta el espejo de la app", true
	}
	if !fileExists(filepath.Join(app.Path, "Contents", "MacOS", desktopAppExecName)) {
		return "falta el ejecutable del lanzador", true
	}
	if !fileExists(app.Manifest.CCPBin) {
		return "no existe el binario de ccp registrado: " + app.Manifest.CCPBin, true
	}
	if stamp := fileStamp(app.Manifest.CCPBin); stamp != app.Manifest.CCPBinStamp {
		return "ccp cambió desde que se construyó el lanzador", true
	}
	if src.iconPath != "" && !fileExists(filepath.Join(app.Path, "Contents", "Resources", desktopAppIconName)) {
		return "falta el icono", true
	}
	return "", false
}

// RemoveDesktopApp borra un lanzador. Solo borra lo que es nuestro (tiene
// manifiesto) y está dentro de appsDir: nunca puede convertirse en un rm -rf
// sobre una app del usuario.
func RemoveDesktopApp(appsDir string, app *DesktopApp) error {
	clean := filepath.Clean(app.Path)
	if !strings.HasPrefix(clean, filepath.Clean(appsDir)+string(os.PathSeparator)) {
		return fmt.Errorf("%s no está bajo %s; no se borra", app.Path, appsDir)
	}
	if _, err := LoadDesktopApp(clean); err != nil {
		return fmt.Errorf("%s no es un lanzador de ccp; no se borra", app.Path)
	}
	if err := os.RemoveAll(clean); err != nil {
		return err
	}
	lsregister("-u", clean)
	return nil
}

// lsregister registra/desregistra un bundle en LaunchServices (solo macOS,
// siempre best-effort: si falla, el Dock tarda un poco más en enterarse).
func lsregister(args ...string) {
	if runtime.GOOS != "darwin" {
		return
	}
	const bin = "/System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister"
	if _, err := os.Stat(bin); err != nil {
		return
	}
	_ = exec.Command(bin, args...).Run()
}

// --- modo lanzador ------------------------------------------------------

// IsDesktopLauncher reconoce si exe (la ruta con la que se invocó ccp, sin
// resolver symlinks) es el `Contents/MacOS/Claude` de un lanzador, y devuelve
// el .app. Es la primera comprobación de main(): LaunchServices invoca el
// binario sin argumentos y lo único que distingue esa invocación de un
// `ccp` a secas es desde dónde se ejecuta.
func IsDesktopLauncher(exe string) (string, bool) {
	if filepath.Base(exe) != desktopAppExecName {
		return "", false
	}
	macos := filepath.Dir(exe)
	if filepath.Base(macos) != "MacOS" {
		return "", false
	}
	app := filepath.Dir(filepath.Dir(macos))
	if _, err := os.Stat(desktopAppManifestPath(app)); err != nil {
		return "", false
	}
	return app, true
}

// DesktopLauncherPlan es lo que el modo lanzador ejecuta.
type DesktopLauncherPlan struct {
	App     *DesktopApp
	Profile string
	DataDir string
	Bin     string   // Contents/MacOS/Claude-run
	Args    []string // argv completo, Args[0] == Bin
	Env     []string
}

// PlanDesktopLauncher calcula el exec: el mismo par de aislamientos que
// PlanDesktop (user-data-dir + entorno del perfil), pero como datos para
// syscall.Exec en vez de argumentos para `open`. Los valores vacíos se omiten
// por la misma razón que allí: Desktop resuelve el config root con `??`, y una
// cadena vacía SÍ pasa ese filtro.
func PlanDesktopLauncher(app *DesktopApp, home string, cfg *Config, args, environ []string) (*DesktopLauncherPlan, error) {
	profile := app.Manifest.Profile
	if err := DesktopEligible(cfg, profile); err != nil {
		return nil, err
	}
	if profile == "default" {
		return nil, fmt.Errorf("un lanzador no puede apuntar a 'default'")
	}
	managed := managedVarSet()
	var env []string
	for _, kv := range EnvForChild(environ, home, profile, cfg) {
		if i := strings.IndexByte(kv, '='); i >= 0 && managed[kv[:i]] && kv[i+1:] == "" {
			continue
		}
		env = append(env, kv)
	}
	bin := filepath.Join(app.Path, "Contents", "MacOS", desktopAppRunName)
	dataDir := DesktopDataDir(home, profile)
	argv := append([]string{bin, "--user-data-dir=" + dataDir}, args...)
	return &DesktopLauncherPlan{App: app, Profile: profile, DataDir: dataDir, Bin: bin, Args: argv, Env: env}, nil
}

// DesktopAppOpenCommand es cómo se lanza un lanzador desde la CLI: por
// LaunchServices, sin -n. El id de bundle es propio, así que `open -a` activa
// la instancia si ya corre y la lanza si no; el entorno y los argumentos los
// pone el propio lanzador, no hace falta --env ni --args.
func DesktopAppOpenCommand(app *DesktopApp, extra []string) (string, []string) {
	args := []string{"-a", app.Path}
	if len(extra) > 0 {
		args = append(args, "--args")
		args = append(args, extra...)
	}
	return "open", args
}
