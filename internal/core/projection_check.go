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
	"fmt"
	"os"
	"path/filepath"
)

// ProjectionCheck es lo que una regeneración cambiaría en un perfil.
type ProjectionCheck struct {
	Profile string          `json:"profile"`
	MCP     []MCPProjection `json:"mcp"`
	// Artifacts son los directorios declarados por el perfil (overlay/<dir>) que
	// todavía no están espejados en el cc-home.
	Artifacts []string `json:"artifacts"`
	Err       string   `json:"error,omitempty"`
}

// Stale dice si hace falta regenerar. Cuenta lo que un `ccp profile sync`
// ARREGLARÍA (escrituras, retiradas, artefactos sin espejar) más lo aplazado a
// que arranque la ventana, que también es destino sin lo declarado. NO cuentan
// los conflictos ni lo que el chat no puede cargar: son estados permanentes que
// decide el usuario, y hacerlos «desfasado» dejaría el check en 1 para siempre
// por algo que ningún sync cambia.
func (c ProjectionCheck) Stale() bool {
	if len(c.Artifacts) > 0 || c.Err != "" {
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
	return c, nil
}

// artifactsPending son los directorios que ProjectProfileArtifacts cambiaría:
// los que el perfil declara y cuyo destino sigue siendo el symlink de la siembra
// o al que le falta alguna hoja. mirrorTree nunca pisa lo que ya existe, así que
// esas dos son las únicas diferencias que puede producir.
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
		if leavesMissing(ov, dst) || leavesMissing(filepath.Join(src, d), dst) {
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
