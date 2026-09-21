package core

// profile_artifacts.go — skills, agents, commands y output-styles POR PERFIL
// (spec §6.2, Fase B). Se declaran en overlay/<dir>/ y la regeneración los
// proyecta al cc-home, que es lo que leen el CLI y la pestaña Code.
//
// La forma del destino es la que Desktop exige (ADR 0008): directorios reales
// con symlinks solo en las hojas. Por eso se reutiliza mirrorTree, que además da
// gratis la precedencia: se espeja primero el overlay y luego el global, y como
// mirrorTree nunca pisa una entrada que ya existe, si chocan gana el perfil.
//
// seedCCHome NO cambia: el oráculo bash exige que justo después de `profile add`
// el cc-home tenga symlinks de directorio, y eso sigue siendo cierto mientras el
// perfil no declare nada propio. La conversión ocurre la primera vez que hay algo
// en overlay/<dir>.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// profileArtifactDirs son los cuatro tipos que un perfil puede declarar. plugins
// no está: un plugin se instala en el global y se activa por `enabledPlugins`.
var profileArtifactDirs = []string{"agents", "commands", "skills", "output-styles"}

// dirHasFiles dice si un directorio existe y tiene algo dentro.
func dirHasFiles(dir string) bool {
	es, err := os.ReadDir(dir)
	return err == nil && len(es) > 0
}

// ProjectProfileArtifacts proyecta al cc-home los artefactos declarados por el
// perfil, uniéndolos con los del global. Devuelve los directorios que tocó.
// Un directorio que el perfil no declara no se toca: sigue siendo el symlink que
// sembró seedCCHome (o el espejo que dejó Desktop).
func ProjectProfileArtifacts(home, name, src string) ([]string, error) {
	if name == "" || name == "default" {
		return nil, nil
	}
	cch := ccHomePath(home, name)
	if _, err := os.Stat(cch); err != nil {
		return nil, nil
	}
	var touched []string
	for _, d := range profileArtifactDirs {
		ov := filepath.Join(cfgOverlayDir(home, name), d)
		if !dirHasFiles(ov) {
			continue
		}
		dst := filepath.Join(cch, d)
		// Si el destino es el symlink de la siembra, se sustituye por un
		// directorio real: un symlink de directorio bajo el config root es lo que
		// Desktop rechaza, y además no podría llevar las dos fuentes a la vez.
		if fi, err := os.Lstat(dst); err == nil && fi.Mode()&os.ModeSymlink != 0 {
			if err := os.Remove(dst); err != nil {
				return touched, fmt.Errorf("no se pudo quitar el symlink %s: %w", dst, err)
			}
		}
		// Lo que el overlay declara manda: se retira antes el enlace al global
		// que lo sombreaba, porque mirrorTree no pisa lo que ya existe.
		for _, p := range overlayShadowedLeaves(ov, dst, filepath.Join(src, d)) {
			if err := os.Remove(p); err != nil {
				return touched, fmt.Errorf("no se pudo quitar el enlace al global %s: %w", p, err)
			}
		}
		if err := mirrorTree(ov, dst); err != nil {
			return touched, err
		}
		if g := filepath.Join(src, d); dirHasFiles(g) {
			if err := mirrorTree(g, dst); err != nil {
				return touched, err
			}
		}
		touched = append(touched, d)
	}
	return touched, nil
}

// overlayShadowedLeaves son las hojas que el overlay declara y cuyo destino
// sigue siendo el symlink al global. Existen porque mirrorTree salta toda
// entrada que ya está en el destino: si el global se espejó primero, declarar
// después esa misma hoja en el perfil no cambiaba nada y el override no se
// aplicaba nunca, contra el «si chocan, gana el perfil» de arriba. Se miran
// SOLO los enlaces que apuntan dentro del global: un archivo real o un enlace a
// otro sitio es del usuario y se respeta, igual que en pruneDangling.
func overlayShadowedLeaves(ov, dst, gdir string) []string {
	entries, err := os.ReadDir(ov)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		sp, dp := filepath.Join(ov, e.Name()), filepath.Join(dst, e.Name())
		info, serr := os.Stat(sp)
		if serr != nil {
			continue // colgado en el overlay: mirrorTree tampoco lo espejaría
		}
		if info.IsDir() {
			out = append(out, overlayShadowedLeaves(sp, dp, gdir)...)
			continue
		}
		fi, lerr := os.Lstat(dp)
		if lerr != nil || fi.Mode()&os.ModeSymlink == 0 {
			continue
		}
		target, rerr := os.Readlink(dp)
		if rerr != nil || !strings.HasPrefix(target, gdir+string(os.PathSeparator)) {
			continue
		}
		out = append(out, dp)
	}
	return out
}
