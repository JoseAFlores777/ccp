package core

// projection_check.go — la deriva entre lo DECLARADO (las capas: global,
// overlay del perfil, bloque mcp: de ccp.yaml) y lo PROYECTADO (lo que leen de
// verdad el CLI, la pestaña Code y el chat de Desktop). Spec §6.4.
//
// Es el mismo motor de la proyección corriendo en modo check: si el cálculo del
// desfase fuera código aparte, acabaría respondiendo otra cosa que la escritura
// —el mismo bug que evita compartir traceMove en el supervisor—. Por eso aquí no
// hay reglas nuevas, solo las llamadas Check* y el recorrido de artefactos.

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ProjectionCheck es lo que una regeneración cambiaría en un perfil.
type ProjectionCheck struct {
	Profile string          `json:"profile"`
	MCP     []MCPProjection `json:"mcp"`
	// Artifacts son los directorios declarados por el perfil (overlay/<dir>) que
	// todavía no están espejados en el cc-home.
	Artifacts []string `json:"artifacts"`
	// Settings e Instructions son la otra mitad de lo que escribe una
	// regeneración: cc-home/settings.json (global ⊕ overlay ⊕ auto) y
	// cc-home/CLAUDE.md. Son la deriva más común —cambiar el global o el overlay
	// y olvidar el sync— y sin ellas el check decía «todo al día» con el perfil
	// desfasado.
	Settings     bool   `json:"settings"`
	Instructions bool   `json:"instructions"`
	Err          string `json:"error,omitempty"`
}

// Stale dice si hace falta regenerar. Cuenta lo que un `ccp profile sync`
// ARREGLARÍA (escrituras, retiradas, artefactos sin espejar) más lo aplazado a
// que arranque la ventana, que también es destino sin lo declarado. NO cuentan
// los conflictos ni lo que el chat no puede cargar: son estados permanentes que
// decide el usuario, y hacerlos «desfasado» dejaría el check en 1 para siempre
// por algo que ningún sync cambia.
func (c ProjectionCheck) Stale() bool {
	if len(c.Artifacts) > 0 || c.Settings || c.Instructions || c.Err != "" {
		return true
	}
	for _, p := range c.MCP {
		if len(p.Written) > 0 || len(p.Removed) > 0 || p.Deferred {
			return true
		}
	}
	return false
}

// ProfileProjectionCheck calcula el desfase de un perfil sin escribir nada.
// `default` no se proyecta (su destino ES la capa global), así que sale limpio.
func ProfileProjectionCheck(home, name string) (ProjectionCheck, error) {
	c := ProjectionCheck{Profile: name}
	if name == "" || name == "default" {
		return c, nil
	}
	src, err := claudeSrc()
	if err != nil {
		return c, err
	}
	cfg, err := Load(home)
	if err != nil {
		return c, err
	}
	if _, ok := cfg.Profiles[name]; !ok {
		return c, fmt.Errorf("no existe el perfil %q", name)
	}
	global, profile, err := ReadMCPLayers(home, src, name)
	if err != nil {
		c.Err = err.Error()
		return c, nil // una capa rota es deriva que contar, no un fallo del check
	}
	eff := MCPEffective(cfg, global, profile, name)
	for _, check := range []func(string, string, []MCPEntry) (MCPProjection, error){CheckMCPToCLI, CheckMCPToDesktop} {
		p, err := check(home, name, eff)
		if err != nil && c.Err == "" {
			c.Err = err.Error()
		}
		if !p.Empty() {
			c.MCP = append(c.MCP, p)
		}
	}
	c.Artifacts = artifactsPending(home, name, src)
	c.Settings, c.Instructions = generatedPending(home, name, src)
	return c, nil
}

// artifactsPending son los directorios que ProjectProfileArtifacts cambiaría:
// los que el perfil declara y cuyo destino sigue siendo el symlink de la siembra,
// al que le falta alguna hoja, o al que le SOBRA un enlace colgado nuestro.
// mirrorTree no pisa lo que ya existe, pero empieza por pruneDangling, así que
// tiene una escritura que no es «crear lo que falta»: borrar un artefacto del
// overlay (lo que hace `ccp instruct rm profile`) deja un enlace colgado en el
// cc-home que el sync poda. Sin mirarlo, el check juraba limpio un destino que
// el siguiente sync sí cambia.
func artifactsPending(home, name, src string) []string {
	cch := ccHomePath(home, name)
	if _, err := os.Stat(cch); err != nil {
		return nil
	}
	var out []string
	for _, d := range profileArtifactDirs {
		ov := filepath.Join(cfgOverlayDir(home, name), d)
		if !dirHasFiles(ov) {
			continue
		}
		dst := filepath.Join(cch, d)
		fi, err := os.Lstat(dst)
		if err != nil || fi.Mode()&os.ModeSymlink != 0 {
			out = append(out, d)
			continue
		}
		g := filepath.Join(src, d)
		if leavesMissing(ov, dst) || leavesMissing(g, dst) ||
			danglingPrunable(ov, dst) || danglingPrunable(g, dst) {
			out = append(out, d)
		}
	}
	return out
}

// leavesMissing dice si alguna hoja de src no tiene su entrada en dst, con el
// mismo recorrido que mirrorTree (un symlink del origen se resuelve para decidir
// si es directorio).
func leavesMissing(src, dst string) bool {
	entries, err := os.ReadDir(src)
	if err != nil {
		return false
	}
	for _, e := range entries {
		sp, dp := filepath.Join(src, e.Name()), filepath.Join(dst, e.Name())
		info, serr := os.Stat(sp)
		if serr != nil {
			continue // colgado en el origen: mirrorTree tampoco lo espejaría
		}
		if info.IsDir() {
			if leavesMissing(sp, dp) {
				return true
			}
			continue
		}
		if _, err := os.Lstat(dp); err != nil {
			return true
		}
	}
	return false
}

// generatedPending dice si cc-home/settings.json y cc-home/CLAUDE.md difieren
// de lo que la regeneración escribiría. No adopta ni escribe nada: construye el
// contenido con los mismos cfgBuildSettings/cfgBuildClaudeMD del sync y lo
// compara. Un cc-home que todavía no existe no cuenta como deriva (no hay
// perfil que proyectar), igual que en artifactsPending.
func generatedPending(home, name, src string) (settings, instructions bool) {
	cch := ccHomePath(home, name)
	if _, err := os.Stat(cch); err != nil {
		return false, false
	}
	return settingsPending(home, name, src, cch), claudeMDPending(home, name, src, cch)
}

// settingsPending compara el settings.json generado con el que hay. La
// comparación es por VALOR (jsonEqual), no por bytes: Claude Code reescribe el
// archivo con JSON.stringify al tocar /config, y 30.0 → 30 no es algo que un
// sync vaya a arreglar. Si el overlay no se puede leer no hay nada que afirmar,
// y si el destino falta o es JSON inválido sí: la regeneración lo rehace.
func settingsPending(home, name, src, cch string) bool {
	overlayData, err := os.ReadFile(cfgSettingsFile(home, name))
	if err != nil {
		return false
	}
	want, err := cfgBuildSettings(home, name, src, overlayData)
	if err != nil {
		return false
	}
	got, err := os.ReadFile(filepath.Join(cch, "settings.json"))
	if err != nil {
		return true
	}
	if bytes.Equal(want, got) {
		return false
	}
	w, werr := decodeJSONObject(want)
	g, gerr := decodeJSONObject(got)
	if werr != nil || gerr != nil {
		return true
	}
	return !jsonEqual(w, g)
}

// claudeMDPending compara cc-home/CLAUDE.md con el que saldría. Un symlink
// viejo cuenta siempre: la regeneración lo quita antes de escribir.
func claudeMDPending(home, name, src, cch string) bool {
	dst := filepath.Join(cch, "CLAUDE.md")
	if isSymlink(dst) {
		return true
	}
	got, err := os.ReadFile(dst)
	if err != nil {
		return true
	}
	return !bytes.Equal(got, cfgBuildClaudeMD(name, src, cfgInstrFile(home, name)))
}

// danglingPrunable dice si pruneDangling borraría algo de dst, con el MISMO
// recorrido que mirrorTree: el nivel de dst, y luego solo los subdirectorios
// que existen en el origen (por eso un skill entero borrado del overlay no
// cuenta aquí: el sync tampoco lo poda, y el check debe decir lo que el sync
// hará, no lo que nos gustaría que hiciera).
func danglingPrunable(src, dst string) bool {
	entries, err := os.ReadDir(dst)
	if err != nil {
		return false
	}
	pfx := src + string(os.PathSeparator)
	for _, e := range entries {
		p := filepath.Join(dst, e.Name())
		fi, lerr := os.Lstat(p)
		if lerr != nil || fi.Mode()&os.ModeSymlink == 0 {
			continue
		}
		if _, serr := os.Stat(p); serr == nil {
			continue // apunta a algo vivo
		}
		if target, rerr := os.Readlink(p); rerr == nil && strings.HasPrefix(target, pfx) {
			return true
		}
	}
	for _, e := range mustReadDir(src) {
		sp := filepath.Join(src, e.Name())
		info, serr := os.Stat(sp)
		if serr != nil || !info.IsDir() {
			continue
		}
		if danglingPrunable(sp, filepath.Join(dst, e.Name())) {
			return true
		}
	}
	return false
}

// mustReadDir es ReadDir sin error: un origen ilegible no espeja nada, así que
// tampoco poda nada.
func mustReadDir(dir string) []os.DirEntry {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	return entries
}
