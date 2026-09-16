package core

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// desktop_audit.go — el motor de `ccp desktop doctor`.
//
// Existe por una razón concreta: el 2026-09-15 el usuario pasó horas creyendo
// que había perdido su historial. No había perdido nada — estaba mirando una
// instancia vacía mientras otra ocupaba la identidad de su Claude principal—,
// pero ninguna pantalla se lo podía decir, porque dos ventanas de Claude se ven
// exactamente igual. Todo lo que hay aquí responde a preguntas que aquel día
// hubo que contestar a mano leyendo `lsappinfo`, `ps` y tamaños de IndexedDB.
//
// Regla innegociable: una sonda que no se puede ejecutar produce Unknown, NUNCA
// OK. Un doctor que dice «todo bien» porque no pudo mirar es peor que no tener
// doctor: convierte una duda en una falsa certeza.
//
// Vive en core (y no en cli) porque necesita readDesktopSource y el lector de
// plists, que no están exportados; llevarlo a cli obligaría a abrir ese parser
// al mundo solo para esto.

// Severidades de un hallazgo. El orden importa: es el de la salida.
const (
	DesktopSevError   = "error"   // algo está roto o miente ahora mismo
	DesktopSevWarn    = "warn"    // funciona, pero se va a romper o cuesta disco
	DesktopSevUnknown = "unknown" // no se pudo comprobar
	DesktopSevInfo    = "info"    // contexto útil
	DesktopSevOK      = "ok"      // comprobado y correcto
)

// Códigos de hallazgo. Estables: los consume `--json`.
const (
	DesktopFindingForeignExec   = "instance_foreign_exec"
	DesktopFindingNoConfigDir   = "instance_no_config_dir"
	DesktopFindingWrongConfig   = "instance_wrong_config_dir"
	DesktopFindingUpdaterOn     = "instance_updater_on"
	DesktopFindingMirrorStale   = "launcher_mirror_stale"
	DesktopFindingMirrorOrphan  = "launcher_mirror_orphan"
	DesktopFindingLauncherGone  = "launcher_profile_gone"
	DesktopFindingMultiAccount  = "instance_multi_account"
	DesktopFindingIdentity      = "launcher_identity_collapsed"
	DesktopFindingProbeMissing  = "probe_unavailable"
	DesktopFindingSchemeHandler = "scheme_handler"
)

// DesktopFinding es un hallazgo del doctor.
type DesktopFinding struct {
	Code     string `json:"code"`
	Severity string `json:"severity"`
	Profile  string `json:"profile,omitempty"`
	Detail   string `json:"detail,omitempty"`
}

// DesktopProbes son las ventanas al sistema. Todas inyectadas: así el motor se
// prueba entero en Linux montando el estado corrupto a mano, y así un test puede
// afirmar «sin esta sonda, el resultado es Unknown».
//
// Una sonda nil significa «no disponible», no «vacío».
type DesktopProbes struct {
	// Procs son los procesos de Claude vivos (nil = no se pudo mirar).
	Procs func() ([]DesktopProc, bool)
	// RunningIdentity dice, para un .app lanzador, con qué bundle id lo tiene
	// registrado LaunchServices ahora mismo (nil = no se pudo mirar).
	RunningIdentity func(app string) (string, bool)
}

// DesktopAuditOptions es lo que audita el doctor.
type DesktopAuditOptions struct {
	Home    string
	Cfg     *Config
	AppsDir string
	Profile string // "" = todos
	Probes  DesktopProbes
}

// DesktopAudit revisa lanzadores e instancias y devuelve lo que encuentra,
// ordenado de lo más grave a lo más anecdótico.
func DesktopAudit(o DesktopAuditOptions) []DesktopFinding {
	var out []DesktopFinding

	procs, procsOK := []DesktopProc(nil), false
	if o.Probes.Procs != nil {
		procs, procsOK = o.Probes.Procs()
	}
	if !procsOK {
		out = append(out, DesktopFinding{
			Code: DesktopFindingProbeMissing, Severity: DesktopSevUnknown, Detail: "ps",
		})
	}

	apps := map[string]*DesktopApp{}
	if o.AppsDir != "" {
		if found, err := ListDesktopApps(o.AppsDir); err == nil {
			for _, a := range found {
				apps[a.Manifest.Profile] = a
			}
		}
	}

	for _, name := range desktopAuditProfiles(o, apps) {
		app := apps[name]
		dataDir := DesktopDataDir(o.Home, name)
		ccHome, _ := CCHome(o.Home, name)

		// 1. El lanzador: ¿sigue teniendo sentido, y su espejo sigue siendo el
		//    de la Claude.app instalada?
		if app != nil {
			if err := DesktopEligible(o.Cfg, name); err != nil {
				out = append(out, DesktopFinding{
					Code: DesktopFindingLauncherGone, Severity: DesktopSevError,
					Profile: name, Detail: err.Error(),
				})
			}
			out = append(out, desktopAuditMirror(app)...)

			// La misma regla que con `ps`: si la sonda no está o no pudo
			// responder, eso es Unknown. Antes «no pude preguntar» era
			// indistinguible de «comprobado y correcto», que es justo la clase
			// de falso OK que este comando existe para no dar.
			id, idOK := "", false
			if o.Probes.RunningIdentity != nil {
				id, idOK = o.Probes.RunningIdentity(app.Path)
			}
			switch {
			case !idOK:
				out = append(out, DesktopFinding{
					Code: DesktopFindingProbeMissing, Severity: DesktopSevUnknown,
					Profile: name, Detail: "lsappinfo",
				})
			case id != "" && id != app.Manifest.BundleID:
				// Aquí está el secuestro: macOS tiene esta ventana registrada
				// con el id del Claude normal.
				out = append(out, DesktopFinding{
					Code: DesktopFindingIdentity, Severity: DesktopSevError,
					Profile: name, Detail: id,
				})
			}
		}

		// 2. Las ventanas vivas de ese perfil.
		if procsOK {
			launcher := ""
			if app != nil {
				launcher = app.Path
			}
			for _, is := range DesktopPreflight(DesktopPreflightInput{
				Profile: name, DataDir: dataDir, CCHome: ccHome,
				LauncherPath: launcher, Procs: procs,
			}) {
				if f, ok := desktopIssueToFinding(is, name); ok {
					out = append(out, f)
				}
			}
		}

		// 3. Las cuentas que guarda el data dir. Esto es lo que habría
		//    ahorrado el susto: «no has perdido 40 sesiones, están bajo otra
		//    cuenta; vuelve a entrar con ella y reaparecen».
		out = append(out, desktopAuditAccounts(name, dataDir)...)
	}

	sort.SliceStable(out, func(i, j int) bool {
		return desktopSevRank(out[i].Severity) < desktopSevRank(out[j].Severity)
	})
	return out
}

// desktopAuditProfiles decide a quién se mira. Los tres orígenes importan:
//
//   - los perfiles de la config, que es lo obvio;
//   - «default», porque su ventana es la del usuario y es la que se queda sin
//     poder abrirse cuando otra le ocupa la identidad — precisamente el síntoma
//     que hay que poder diagnosticar;
//   - los perfiles que solo existen como LANZADOR, porque un lanzador cuyo
//     perfil ya no está en la config es exactamente el huérfano que hay que
//     reportar, y mirando solo cfg.Profiles no se visitaba jamás.
func desktopAuditProfiles(o DesktopAuditOptions, apps map[string]*DesktopApp) []string {
	if o.Profile != "" {
		return []string{o.Profile}
	}
	seen := map[string]bool{"default": true}
	names := []string{"default"}
	add := func(n string) {
		if n != "" && !seen[n] {
			seen[n] = true
			names = append(names, n)
		}
	}
	if o.Cfg != nil {
		for name := range o.Cfg.Profiles {
			add(name)
		}
	}
	for name := range apps {
		add(name)
	}
	sort.Strings(names)
	return names
}

// desktopAuditMirror compara el espejo del lanzador con la Claude.app
// instalada. Dos señales distintas y las dos importan:
//
//   - versión: el espejo se quedó atrás (pasa tras cada actualización);
//   - inodo: los hard links ya no apuntan al mismo fichero que la fuente. Esto
//     ocurre SIN cambio de versión cuando ShipIt reemplaza el bundle entero con
//     un move —lo que hizo el 2026-09-15—, y deja el espejo reteniendo una copia
//     completa del Claude viejo que ya no comparte un solo byte con nadie.
func desktopAuditMirror(app *DesktopApp) []DesktopFinding {
	src, err := readDesktopSource(app.Manifest.SourceApp)
	if err != nil {
		return []DesktopFinding{{
			Code: DesktopFindingProbeMissing, Severity: DesktopSevUnknown,
			Profile: app.Manifest.Profile, Detail: app.Manifest.SourceApp,
		}}
	}
	var out []DesktopFinding
	if src.version != app.Manifest.SourceVersion {
		out = append(out, DesktopFinding{
			Code: DesktopFindingMirrorStale, Severity: DesktopSevWarn,
			Profile: app.Manifest.Profile,
			Detail:  app.Manifest.SourceVersion + " → " + src.version,
		})
	}
	mirrorExe := filepath.Join(app.Path, "Contents", filepath.FromSlash(desktopAppNestedRel),
		"Contents", "MacOS", src.exeName)
	srcExe := filepath.Join(app.Manifest.SourceApp, "Contents", "MacOS", src.exeName)
	mi, merr := os.Stat(mirrorExe)
	si, serr := os.Stat(srcExe)
	switch {
	case merr != nil || serr != nil:
		out = append(out, DesktopFinding{
			Code: DesktopFindingProbeMissing, Severity: DesktopSevUnknown,
			Profile: app.Manifest.Profile, Detail: mirrorExe,
		})
	case !os.SameFile(mi, si):
		out = append(out, DesktopFinding{
			Code: DesktopFindingMirrorOrphan, Severity: DesktopSevWarn,
			Profile: app.Manifest.Profile, Detail: mirrorExe,
		})
	}
	return out
}

// desktopAuditAccounts cuenta las cuentas y las sesiones locales que guarda un
// data dir. Claude indexa las sesiones de la pestaña Code por
// <accountUuid>/<orgUuid>, así que cambiar de cuenta hace que el panel salga
// vacío aunque en disco siga TODO. Decirlo es la mitad del valor de este
// comando.
func desktopAuditAccounts(profile, dataDir string) []DesktopFinding {
	if dataDir == "" {
		return nil
	}
	root := filepath.Join(dataDir, "claude-code-sessions")
	accounts, err := os.ReadDir(root)
	if err != nil || len(accounts) < 2 {
		return nil
	}
	var parts []string
	for _, a := range accounts {
		if !a.IsDir() {
			continue
		}
		n := 0
		orgs, _ := os.ReadDir(filepath.Join(root, a.Name()))
		for _, o := range orgs {
			if !o.IsDir() {
				continue
			}
			files, _ := os.ReadDir(filepath.Join(root, a.Name(), o.Name()))
			for _, f := range files {
				if strings.HasPrefix(f.Name(), "local_") {
					n++
				}
			}
		}
		parts = append(parts, fmt.Sprintf("%s=%d", a.Name(), n))
	}
	if len(parts) < 2 {
		return nil
	}
	sort.Strings(parts)
	return []DesktopFinding{{
		Code: DesktopFindingMultiAccount, Severity: DesktopSevInfo,
		Profile: profile, Detail: strings.Join(parts, " "),
	}}
}

func desktopIssueToFinding(is DesktopIssue, profile string) (DesktopFinding, bool) {
	sev := DesktopSevWarn
	if is.Fatal {
		sev = DesktopSevError
	}
	switch is.Code {
	case DesktopIssueForeignExec:
		return DesktopFinding{Code: DesktopFindingForeignExec, Severity: sev, Profile: profile, Detail: is.Detail}, true
	case DesktopIssueNoConfigDir:
		return DesktopFinding{Code: DesktopFindingNoConfigDir, Severity: sev, Profile: profile, Detail: is.Detail}, true
	case DesktopIssueWrongConfigDir:
		return DesktopFinding{Code: DesktopFindingWrongConfig, Severity: sev, Profile: profile, Detail: is.Detail}, true
	case DesktopIssueUpdaterOn:
		return DesktopFinding{Code: DesktopFindingUpdaterOn, Severity: DesktopSevWarn, Profile: profile, Detail: is.Detail}, true
	}
	return DesktopFinding{}, false // "ya hay una ventana" no es un hallazgo del doctor
}

func desktopSevRank(s string) int {
	switch s {
	case DesktopSevError:
		return 0
	case DesktopSevWarn:
		return 1
	case DesktopSevUnknown:
		return 2
	case DesktopSevInfo:
		return 3
	}
	return 4
}

// DesktopFindingsJSON serializa los hallazgos. Siempre un array, nunca null:
// es el contrato de las superficies --json de ccp.
func DesktopFindingsJSON(fs []DesktopFinding) ([]byte, error) {
	if fs == nil {
		fs = []DesktopFinding{}
	}
	return json.MarshalIndent(fs, "", "  ")
}

// DesktopWorst devuelve la severidad más grave presente.
func DesktopWorst(fs []DesktopFinding) string {
	worst := DesktopSevOK
	for _, f := range fs {
		if desktopSevRank(f.Severity) < desktopSevRank(worst) {
			worst = f.Severity
		}
	}
	return worst
}
