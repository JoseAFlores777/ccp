package core

import (
	"os/exec"
	"path/filepath"
	"sort"
)

// DoctorCheck es un resultado de diagnóstico: OK indica si el chequeo pasó,
// Label es el texto humano (en español) ya formateado por core. cli decide el
// glyph/color según OK respetando NO_COLOR / non-TTY.
type DoctorCheck struct {
	OK    bool
	Label string
}

// lookPath se inyecta en tests para no depender del PATH real de la máquina.
var lookPath = exec.LookPath

// HasLogin indica si un perfil oficial ya tiene sesión iniciada: existe el
// archivo cc-home/.claude.json. Para perfiles no oficiales no aplica el
// concepto de login; el caller decide qué mostrar. Read-only, sin imprimir.
func HasLogin(home, name string) bool {
	return fileExists(filepath.Join(ccHomePath(home, name), ".claude.json"))
}

// Doctor reproduce cmd_doctor del bash: chequea node/claude/git en PATH y, por
// cada perfil, su estado de login (official: cc-home/.claude.json) o key
// (deepseek: api_key). Devuelve la lista de chequeos en orden estable; cli la
// presenta. No imprime nada: lógica pura sobre home + PATH.
func Doctor(home string) ([]DoctorCheck, error) {
	var checks []DoctorCheck

	for _, bin := range []string{"node", "claude", "git"} {
		if _, err := lookPath(bin); err == nil {
			checks = append(checks, DoctorCheck{OK: true, Label: bin + ": encontrado en PATH."})
		} else {
			checks = append(checks, DoctorCheck{OK: false, Label: bin + ": no encontrado en PATH."})
		}
	}

	c, err := Load(home)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(c.Profiles))
	for name := range c.Profiles {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		p := c.Profiles[name]
		switch p.Type {
		case "official":
			claudeJSON := filepath.Join(ccHomePath(home, name), ".claude.json")
			if fileExists(claudeJSON) {
				checks = append(checks, DoctorCheck{
					OK: true, Label: "Perfil '" + name + "' (oficial): logueado."})
			} else {
				checks = append(checks, DoctorCheck{
					OK: false, Label: "Perfil '" + name + "' (oficial): SIN login (ccp profile login " + name + ")."})
			}
		case "deepseek":
			if _, ok := GetKey(home, name); ok {
				checks = append(checks, DoctorCheck{
					OK: true, Label: "Perfil '" + name + "' (deepseek): key OK."})
			} else {
				checks = append(checks, DoctorCheck{
					OK: false, Label: "Perfil '" + name + "' (deepseek): SIN key (ccp key " + name + ")."})
			}
		}
	}

	return checks, nil
}
