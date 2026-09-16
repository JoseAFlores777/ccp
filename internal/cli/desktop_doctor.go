package cli

import (
	"fmt"
	"io"
	"os/exec"
	"runtime"
	"strings"

	"github.com/JoseAFlores777/ccp/internal/core"
	"github.com/JoseAFlores777/ccp/internal/core/i18n"
)

// desktop_doctor.go — `ccp desktop doctor`.
//
// Contesta, en un comando, las preguntas que el 2026-09-15 hubo que resolver a
// mano con lsappinfo, ps, codesign y du: qué ventanas hay vivas, si alguna está
// usando la identidad del Claude principal, si alguna escribe su historial en el
// sitio equivocado, si algún espejo se quedó desfasado y si un data dir guarda
// sesiones de más de una cuenta.
//
// No repara nada, y es deliberado: reparar (borrar lanzadores, reconstruir
// bundles, desregistrar de LaunchServices) convertiría un diagnóstico en una
// segunda fuente de pérdida de datos. Diagnostica y dice qué tecla resuelve.

func desktopDoctor(args []string, stdout, stderr io.Writer) int {
	var profile string
	asJSON := false
	for _, a := range args {
		switch {
		case a == "--json":
			asJSON = true
		case a == "--help" || a == "-h":
			fmt.Fprintln(stdout, i18n.T(currentLang(), "cli.desktop.doctor.usage"))
			return 0
		case strings.HasPrefix(a, "-"):
			fmt.Fprintln(stderr, i18n.T(currentLang(), "cli.desktop.unknown_flag", a))
			return 1
		default:
			profile = a
		}
	}

	home := resolveHome()
	cfg, err := loadCfg(home)
	if err != nil {
		fmt.Fprintf(stderr, "[error] %v\n", err)
		return 1
	}
	lang := i18n.Resolve(cfg.Lang)
	if profile != "" {
		if _, ok := cfg.Profiles[profile]; !ok && profile != "default" {
			fmt.Fprintf(stderr, "[error] %v\n", fmt.Errorf("no existe el perfil %q", profile))
			return 1
		}
	}
	appsDir, _ := core.DesktopAppsDir()

	findings := core.DesktopAudit(core.DesktopAuditOptions{
		Home: home, Cfg: cfg, AppsDir: appsDir, Profile: profile,
		Probes: core.DesktopProbes{
			Procs:           desktopProcsProbe,
			RunningIdentity: desktopRunningIdentity,
		},
	})

	if asJSON {
		b, jerr := core.DesktopFindingsJSON(findings)
		if jerr != nil {
			fmt.Fprintf(stderr, "[error] %v\n", jerr)
			return 1
		}
		fmt.Fprintln(stdout, string(b))
		return desktopDoctorExit(findings)
	}

	if len(findings) == 0 {
		fmt.Fprintln(stdout, okLine(stdout, i18n.T(lang, "cli.desktop.doctor.clean")))
		return 0
	}
	for _, f := range findings {
		line := desktopFindingText(f, lang)
		switch f.Severity {
		case core.DesktopSevError:
			fmt.Fprintln(stdout, errLine(stdout, line))
		case core.DesktopSevWarn, core.DesktopSevUnknown:
			fmt.Fprintln(stdout, warnLine(stdout, line))
		default:
			fmt.Fprintln(stdout, mute(stdout, line))
		}
	}
	return desktopDoctorExit(findings)
}

// desktopDoctorExit: 1 solo si hay algo roto AHORA. Un espejo desfasado o una
// sonda ausente no son motivo para que un script falle.
func desktopDoctorExit(fs []core.DesktopFinding) int {
	if core.DesktopWorst(fs) == core.DesktopSevError {
		return 1
	}
	return 0
}

func desktopFindingText(f core.DesktopFinding, lang i18n.Lang) string {
	key := "cli.desktop.doctor.f." + f.Code
	if f.Profile == "" {
		// Un hallazgo sin perfil es global (p. ej. `ps` no disponible): con la
		// clave normal se imprimiría «''» donde debería ir un nombre.
		key += ".global"
	}
	// i18n.T devuelve la propia key cuando no existe: un código nuevo sin texto
	// se enseña crudo en vez de desaparecer de la salida.
	if txt := i18n.T(lang, key, f.Profile, f.Detail); txt != key {
		return txt
	}
	return strings.TrimSpace(f.Code + " " + f.Profile + " " + f.Detail)
}

// desktopProcsProbe adapta la sonda de procesos al contrato de core: el segundo
// valor distingue «no hay ninguno» de «no pude mirar», que es la diferencia
// entre decir OK y decir Unknown.
func desktopProcsProbe() ([]core.DesktopProc, bool) {
	if runtime.GOOS == "windows" {
		return nil, false
	}
	if _, err := exec.LookPath("ps"); err != nil {
		return nil, false
	}
	return desktopProcesses(), true
}

// desktopRunningIdentity pregunta a LaunchServices con qué bundle id tiene
// registrada la ventana de ESE lanzador. Es la sonda que destapa el secuestro:
// si responde `com.anthropic.claudefordesktop` en vez del id propio del
// lanzador, macOS está tratando esa ventana como el Claude principal, y
// entonces `open -a Claude` la activa a ella.
func desktopRunningIdentity(app string) (string, bool) {
	if runtime.GOOS != "darwin" {
		return "", false
	}
	out, err := exec.Command("lsappinfo", "list").Output()
	if err != nil {
		return "", false
	}
	// lsappinfo imprime por app un bloque con `bundleID=` y `bundle path=`. Se
	// busca el bloque cuyo path cuelgue de este .app (el espejo cuelga de él) y
	// se devuelve el id con el que está registrado.
	var id string
	for _, line := range strings.Split(string(out), "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "bundleID=") {
			id = strings.Trim(strings.TrimPrefix(t, "bundleID="), `" `)
			continue
		}
		if strings.HasPrefix(t, "bundle path=") {
			p := strings.Trim(strings.TrimPrefix(t, "bundle path="), `" `)
			if p == app || strings.HasPrefix(p, app+"/") {
				return id, true
			}
		}
	}
	return "", true // sonda disponible y sin ventana de ese lanzador: no hay nada que objetar
}
