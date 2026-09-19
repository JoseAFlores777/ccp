package core

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/JoseAFlores777/ccp/internal/core/i18n"
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

// HasLogin indica si un perfil oficial tiene sesión iniciada: su
// cc-home/.claude.json registra una cuenta (oauthAccount) o una clave de consola
// (primaryApiKey). Que el archivo exista no basta: Claude Code lo crea en su
// primer arranque, antes del /login (spec 2026-09-18, B3). Para perfiles no
// oficiales no aplica el concepto de login; el caller decide qué mostrar.
// Read-only, sin imprimir.
func HasLogin(home, name string) bool {
	return claudeJSONHasLogin(filepath.Join(ccHomePath(home, name), ".claude.json"))
}

// DefaultHasLogin es HasLogin para la cuenta de siempre: el .claude.json junto a
// la fuente global (src + ".json", igual que InstructDest).
func DefaultHasLogin(src string) bool { return claudeJSONHasLogin(src + ".json") }

func claudeJSONHasLogin(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var v struct {
		OAuthAccount  json.RawMessage `json:"oauthAccount"`
		PrimaryAPIKey string          `json:"primaryApiKey"`
	}
	if json.Unmarshal(data, &v) != nil {
		return false
	}
	acct := strings.TrimSpace(string(v.OAuthAccount))
	return (acct != "" && acct != "null" && acct != "{}") || v.PrimaryAPIKey != ""
}

// Doctor reproduce cmd_doctor del bash: chequea node/claude/git en PATH y, por
// cada perfil, su estado de login (official: HasLogin) o key
// (deepseek: api_key). Devuelve la lista de chequeos en orden estable; cli la
// presenta. No imprime nada: lógica pura sobre home + PATH.
func Doctor(l i18n.Lang, home string) ([]DoctorCheck, error) {
	var checks []DoctorCheck

	for _, bin := range []string{"node", "claude", "git"} {
		if _, err := lookPath(bin); err == nil {
			checks = append(checks, DoctorCheck{OK: true, Label: i18n.T(l, "doctor.path_found", bin)})
		} else {
			checks = append(checks, DoctorCheck{OK: false, Label: i18n.T(l, "doctor.path_missing", bin)})
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
			if HasLogin(home, name) {
				checks = append(checks, DoctorCheck{
					OK: true, Label: i18n.T(l, "doctor.official_logged", name)})
			} else {
				checks = append(checks, DoctorCheck{
					OK: false, Label: i18n.T(l, "doctor.official_nologin", name, name)})
			}
		default:
			if !IsProviderType(p.Type) {
				continue
			}
			if _, ok := GetKey(home, name); ok {
				checks = append(checks, DoctorCheck{
					OK: true, Label: i18n.T(l, "doctor.provider_keyok", name, p.Type)})
			} else {
				checks = append(checks, DoctorCheck{
					OK: false, Label: i18n.T(l, "doctor.provider_nokey", name, p.Type, name)})
			}
		}
	}

	return checks, nil
}
