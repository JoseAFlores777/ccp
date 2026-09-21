package core

// doctor_projection.go — los hallazgos del doctor que nacen de la proyección
// (spec §6.4). Todos contestan a la misma pregunta: ¿lo que el usuario declaró
// está donde las apps lo leen?
//
// Regla heredada del ADR 0009 y que aquí también manda: solo se emite un
// hallazgo cuando se ha podido MIRAR. Una fuente que no se puede leer no
// produce un «todo bien», produce silencio o un error con su ruta — un doctor
// que bendice lo que no vio convierte una duda en falsa certeza.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/JoseAFlores777/ccp/internal/core/i18n"
)

// Códigos estables de los hallazgos (los lee la GUI, no solo el humano).
const (
	DoctorProjectionStale     = "projection_stale"
	DoctorDesktopRestart      = "desktop_restart_pending"
	DoctorMCPCommandMissing   = "mcp_command_missing"
	DoctorMCPOnlyDesktop      = "mcp_unmanaged_only_desktop"
	DoctorCCHomeSymlinkNonLef = "cc_home_symlink_nonleaf"
)

// doctorProjection recorre los perfiles no-`default` y devuelve SOLO lo que hay
// que arreglar: un perfil sano no añade filas. Es lo contrario que los chequeos
// clásicos (PATH, login), que siempre dicen algo, y es deliberado: estos miran
// cinco cosas por perfil y listarlas todas en verde escondería las rojas.
func doctorProjection(l i18n.Lang, home string, cfg *Config) []DoctorCheck {
	src, err := claudeSrc()
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(cfg.Profiles))
	for n := range cfg.Profiles {
		names = append(names, n)
	}
	sort.Strings(names)

	var out []DoctorCheck
	bad := func(code, key string, args ...any) {
		out = append(out, DoctorCheck{Code: code, Label: i18n.T(l, key, args...)})
	}
	for _, name := range names {
		chk, err := ProfileProjectionCheck(home, name)
		switch {
		case err != nil:
			bad(DoctorProjectionStale, "doctor.projection_error", name, err.Error())
		case chk.Err != "":
			bad(DoctorProjectionStale, "doctor.projection_error", name, chk.Err)
		case chk.Stale():
			bad(DoctorProjectionStale, "doctor.projection_stale", name, name)
		}
		if DesktopProjectionPending(home, name) {
			bad(DoctorDesktopRestart, "doctor.desktop_restart_pending", name, name)
		}
		if miss := doctorMissingCommands(home, src, name, cfg); len(miss) > 0 {
			bad(DoctorMCPCommandMissing, "doctor.mcp_command_missing", name, strings.Join(miss, ", "))
		}
		if only := doctorOnlyDesktopMCP(home, src, name, cfg); len(only) > 0 {
			bad(DoctorMCPOnlyDesktop, "doctor.mcp_only_desktop", name, strings.Join(only, ", "), name)
		}
		if links := doctorCCHomeNonLeafLinks(home, name); len(links) > 0 {
			bad(DoctorCCHomeSymlinkNonLef, "doctor.cc_home_symlink_nonleaf", name, strings.Join(links, ", "), name)
		}
	}
	return out
}

// doctorEffectiveMCP son los MCP efectivos de un perfil, o nil si alguna capa no
// se pudo leer (ese caso ya lo cuenta projection_stale con su error).
func doctorEffectiveMCP(home, src, name string, cfg *Config) []MCPEntry {
	global, profile, err := ReadMCPLayers(home, src, name)
	if err != nil {
		return nil
	}
	return MCPEffective(cfg, global, profile, name)
}

// doctorMissingCommands: un MCP stdio cuyo command no resuelve no arranca, y
// nadie se entera hasta que falla dentro de Claude Code.
func doctorMissingCommands(home, src, name string, cfg *Config) []string {
	var out []string
	for _, e := range doctorEffectiveMCP(home, src, name, cfg) {
		if e.Kind() != "stdio" {
			continue
		}
		cmd, _ := e.Def["command"].(string)
		if cmd == "" {
			continue
		}
		if mcpMissingCommand(cmd, "", lookPath) != "" {
			out = append(out, e.Name+" ("+cmd+")")
		}
	}
	return out
}

// doctorOnlyDesktopMCP: lo que está en el claude_desktop_config.json de la
// ventana y en ninguna capa declarada. Funciona hoy en esta máquina y no existe
// en ningún sitio más: ni en el CLI, ni en un snapshot, ni en otro equipo.
func doctorOnlyDesktopMCP(home, src, name string, cfg *Config) []string {
	dir := DesktopDataDir(home, name)
	if dir == "" {
		return nil
	}
	b, err := os.ReadFile(filepath.Join(dir, "claude_desktop_config.json"))
	if err != nil {
		return nil
	}
	var doc struct {
		MCPServers map[string]json.RawMessage `json:"mcpServers"`
	}
	if json.Unmarshal(b, &doc) != nil {
		return nil // un archivo roto no es un MCP sin declarar; no se inventa
	}
	declared := map[string]bool{}
	for _, e := range doctorEffectiveMCP(home, src, name, cfg) {
		declared[e.Name] = true
	}
	var out []string
	for n := range doc.MCPServers {
		if !declared[n] {
			out = append(out, n)
		}
	}
	sort.Strings(out)
	return out
}

// doctorCCHomeNonLeafLinks: un symlink de DIRECTORIO bajo el config root es lo
// que Desktop rechaza («symlink at a non-leaf component»), y con él la pestaña
// Code de ese perfil no abre. Solo se mira en un perfil que tiene ventana: sin
// ella esa es la forma que siembra `profile add` y que el oráculo bash exige,
// así que acusarla sería acusar al contrato.
func doctorCCHomeNonLeafLinks(home, name string) []string {
	dir := DesktopDataDir(home, name)
	if dir == "" {
		return nil
	}
	if _, err := os.Stat(dir); err != nil {
		return nil
	}
	cch := ccHomePath(home, name)
	entries, err := os.ReadDir(cch)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if e.Type()&os.ModeSymlink == 0 {
			continue
		}
		fi, err := os.Stat(filepath.Join(cch, e.Name()))
		if err != nil || !fi.IsDir() {
			continue // hoja (o colgado): la regla de Desktop exime los archivos
		}
		out = append(out, e.Name())
	}
	sort.Strings(out)
	return out
}
