package core

import (
	"fmt"
	"os"
	"path/filepath"
)

// desktop_launch.go — la decisión de CÓMO lanzar, separada de lanzar.
//
// PlanDesktop es pura: recibe el SO, el buscador de rutas y el entorno como
// datos, y devuelve qué ejecutable con qué argumentos. Mismo patrón que
// editor.go y supervisor.BuildArgs, y por la misma razón: un test puede afirmar
// «en darwin con el .app en /Applications los args son exactamente estos» sin
// abrir una sola ventana. Ejecutar es cosa de internal/cli.

// DesktopPlan es el lanzamiento resuelto de una instancia.
type DesktopPlan struct {
	Profile string // perfil al que pertenece la instancia
	App     string // .app (darwin) o binario (linux) resuelto
	DataDir string // --user-data-dir; "" en default (no se reubica)
	Bin     string // ejecutable a invocar
	Args    []string
	Env     []EnvVar // delta del perfil (lo que aísla el Code tab)
	Fresh   bool     // el data dir aún no existe: primer arranque
}

// DesktopHost son las dependencias externas de PlanDesktop, inyectables.
type DesktopHost struct {
	GOOS     string
	LookPath func(string) (string, error)
	Stat     func(string) (os.FileInfo, error)
	AppHint  string // ruta explícita (--app, CCP_DESKTOP_APP o config)
}

// macOSDefaultApps son las ubicaciones donde vive Claude.app, en orden.
var macOSDefaultApps = []string{
	"/Applications/Claude.app",
	"/Applications/Claude for Desktop.app",
}

// linuxDesktopBins son los nombres del build de escritorio en Linux (no
// oficial: lo empaqueta la comunidad, de ahí que haya más de un nombre).
var linuxDesktopBins = []string{"claude-desktop", "claude"}

// PlanDesktop resuelve el lanzamiento de `name`.
//
// Los dos aislamientos van juntos a propósito: `--user-data-dir` (identidad de
// la app) y el delta de entorno del perfil (que lleva CLAUDE_CONFIG_DIR y por
// tanto aísla el Code tab). Emitir uno sin el otro produce justo el híbrido que
// este comando existe para evitar.
func PlanDesktop(h DesktopHost, home, name string, cfg *Config) (DesktopPlan, error) {
	if err := DesktopEligible(cfg, name); err != nil {
		return DesktopPlan{}, err
	}

	plan := DesktopPlan{Profile: name, DataDir: DesktopDataDir(home, name)}

	if plan.DataDir != "" {
		if _, err := h.Stat(plan.DataDir); err != nil {
			plan.Fresh = true
		}
	}

	// El delta del perfil. Para `default` esto es solo CCP_PROFILE: NO se emite
	// un CLAUDE_CONFIG_DIR vacío para "limpiarlo", y no es un descuido. Desktop
	// resuelve el config root con `lT() ?? join(homedir(),".claude")`, y `??`
	// solo cae ante null/undefined: una cadena vacía SÍ pasa el filtro y lo
	// dejaría con el config root en "". Ausente y vacío no son lo mismo aquí.
	vars, _ := EnvPairs(home, name, cfg)
	for _, v := range vars {
		if v.Value == "" {
			continue
		}
		plan.Env = append(plan.Env, v)
	}

	app, err := resolveDesktopApp(h)
	if err != nil {
		return DesktopPlan{}, err
	}
	plan.App = app

	switch h.GOOS {
	case "darwin":
		// `open -n` en vez de ejecutar Contents/MacOS/Claude directo: pasa por
		// LaunchServices, así que la ventana no queda colgando del proceso de
		// la terminal y sobrevive a cerrarla. El `--env` de open es lo que
		// mete el delta en el entorno del proceso Desktop.
		plan.Bin = "open"
		plan.Args = []string{"-n", "-a", app}
		for _, v := range plan.Env {
			plan.Args = append(plan.Args, "--env", v.Name+"="+v.Value)
		}
		if plan.DataDir != "" {
			plan.Args = append(plan.Args, "--args", "--user-data-dir="+plan.DataDir)
		}

	case "linux":
		// Sin LaunchServices: se ejecuta el binario y el entorno se pasa por
		// exec.Cmd.Env (EnvForChild), no por argumentos.
		plan.Bin = app
		if plan.DataDir != "" {
			plan.Args = []string{"--user-data-dir=" + plan.DataDir}
		}

	default:
		return DesktopPlan{}, fmt.Errorf(
			"ccp desktop no soporta %s (solo macOS y Linux)", h.GOOS)
	}

	return plan, nil
}

// resolveDesktopApp busca la app. Un hint explícito gana siempre y NO se
// valida contra la lista: si el usuario dice dónde está su copia, el trabajo de
// ccp es creerle, no discutir.
func resolveDesktopApp(h DesktopHost) (string, error) {
	if h.AppHint != "" {
		if _, err := h.Stat(h.AppHint); err != nil {
			return "", fmt.Errorf("no existe la app indicada: %s", h.AppHint)
		}
		return h.AppHint, nil
	}

	switch h.GOOS {
	case "darwin":
		candidates := macOSDefaultApps
		if userHome, err := os.UserHomeDir(); err == nil {
			candidates = append(candidates, filepath.Join(userHome, "Applications", "Claude.app"))
		}
		for _, c := range candidates {
			if _, err := h.Stat(c); err == nil {
				return c, nil
			}
		}
		return "", fmt.Errorf(
			"no se encontró Claude.app (probado en %v); indícala con --app o `ccp config desktop-app <ruta>`",
			macOSDefaultApps)

	case "linux":
		for _, b := range linuxDesktopBins {
			if p, err := h.LookPath(b); err == nil {
				return p, nil
			}
		}
		return "", fmt.Errorf(
			"no se encontró el binario de Claude Desktop en el PATH (probado %v); indícalo con --app",
			linuxDesktopBins)

	default:
		return "", fmt.Errorf("ccp desktop no soporta %s (solo macOS y Linux)", h.GOOS)
	}
}

// DesktopInstance es una instancia vista desde disco, para `ccp desktop list`.
type DesktopInstance struct {
	Profile string
	DataDir string
	Exists  bool
	Bytes   int64 // tamaño en disco; 0 si no existe
}

// DesktopList recorre los perfiles elegibles y reporta su instancia.
// `default` se incluye porque también tiene una (la de siempre), aunque su
// DataDir sea "" — omitirla daría la impresión de que la cuenta normal del
// usuario no cuenta como instancia, que es justo el malentendido a evitar.
func DesktopList(home string, cfg *Config) ([]DesktopInstance, error) {
	names, err := ProfileList(home)
	if err != nil {
		return nil, err
	}

	out := []DesktopInstance{{Profile: "default", DataDir: "", Exists: true}}
	for _, n := range names {
		if DesktopEligible(cfg, n) != nil {
			continue
		}
		dir := DesktopDataDir(home, n)
		inst := DesktopInstance{Profile: n, DataDir: dir}
		if fi, err := os.Stat(dir); err == nil && fi.IsDir() {
			inst.Exists = true
			inst.Bytes = dirSize(dir)
		}
		out = append(out, inst)
	}
	return out, nil
}

// dirSize suma el tamaño del árbol. Los errores se ignoran a propósito: esto
// alimenta una columna informativa, y fallar el comando entero porque un
// archivo de caché desapareció a mitad del recorrido sería absurdo.
func dirSize(root string) int64 {
	var total int64
	_ = filepath.Walk(root, func(_ string, fi os.FileInfo, err error) error {
		if err != nil || fi == nil || fi.IsDir() {
			return nil //nolint:nilerr // ver comentario
		}
		total += fi.Size()
		return nil
	})
	return total
}
