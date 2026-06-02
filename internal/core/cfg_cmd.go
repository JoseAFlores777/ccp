package core

// cfg_cmd.go — orquestación de `profile config` y `profile sync` sobre el motor
// de overlay (cfg.go). La resolución del editor y el lanzamiento del proceso
// viven aquí para que internal/cli solo despache.

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// claudeSrc devuelve la fuente global (el "default"/~/.claude). Override por
// CCP_CLAUDE_SRC, igual que seedCCHome. Si HOME no se puede resolver y no hay
// override, devuelve error.
func claudeSrc() (string, error) {
	if src := os.Getenv("CCP_CLAUDE_SRC"); src != "" {
		return src, nil
	}
	hd, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("no se pudo determinar HOME: %w", err)
	}
	return hd + "/.claude", nil
}

// ResolveEditor replica _resolve_editor del bash: defaults.editor (de ccp.yaml)
// -> $EDITOR -> nano. Devuelve la línea de comando completa como string (puede
// llevar flags, p.ej. "code --wait"); el caller la tokeniza.
func ResolveEditor(home string) string {
	if c, err := Load(home); err == nil && c.Defaults.Editor != "" {
		return c.Defaults.Editor
	}
	if e := os.Getenv("EDITOR"); e != "" {
		return e
	}
	return "nano"
}

// launchEditor tokeniza la línea del editor (split por espacios) y lo ejecuta
// con los archivos dados, conectando stdio del proceso actual. Inyectable en
// tests vía ProfileConfigOpts.Launch.
func launchEditor(editorLine string, files ...string) error {
	fields := strings.Fields(editorLine)
	if len(fields) == 0 {
		fields = []string{"nano"}
	}
	args := append(fields[1:], files...)
	cmd := exec.Command(fields[0], args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// ProfileConfigOpts permite inyectar el lanzador del editor en tests (si Launch
// es nil se usa launchEditor real).
type ProfileConfigOpts struct {
	Launch func(editorLine string, files ...string) error
}

// ProfileConfig abre los archivos overlay (instrucciones + settings) del perfil
// en el editor resuelto y regenera el cc-home tras la edición. Rechaza
// "default" (no tiene overlay; su config es la global). El perfil debe existir.
//
// A diferencia del menú interactivo del bash, abre AMBOS overlays en una sola
// invocación del editor (el flujo TUI rico se construye en #11); esto mantiene
// el core sin estado de terminal. Tras editar, valida el settings overlay y
// solo regenera si el JSON es válido (conservando el último bueno si no).
func ProfileConfig(home, name string, opts ProfileConfigOpts) error {
	if name == "" {
		return fmt.Errorf("Uso: ccp profile config <perfil>")
	}
	if name == "default" {
		return fmt.Errorf("'default' = tu config GLOBAL; edítala directamente, no tiene overlay")
	}
	c, err := Load(home)
	if err != nil {
		return err
	}
	if _, exists := c.Profiles[name]; !exists {
		return fmt.Errorf("no existe el perfil %q", name)
	}

	if err := CfgMigrateLegacy(home, name); err != nil {
		return err
	}
	if err := CfgInitOverlay(home, name); err != nil {
		return err
	}

	src, err := claudeSrc()
	if err != nil {
		return err
	}

	launch := opts.Launch
	if launch == nil {
		launch = launchEditor
	}
	instr := cfgInstrFile(home, name)
	settings := cfgSettingsFile(home, name)
	if err := launch(ResolveEditor(home), instr, settings); err != nil {
		return fmt.Errorf("el editor falló: %w", err)
	}

	if err := CfgValidateJSON(settings); err != nil {
		return fmt.Errorf("%w — NO regeneré cc-home (se conserva el último bueno); reedita: ccp profile config %s", err, name)
	}
	if err := CfgRegenerate(home, name, src); err != nil {
		return err
	}
	return nil
}

// ProfileSync re-mergea global ⊕ overlay para un perfil (si name != "") o para
// todos (si name == ""). Rechaza "default". Migra legacy antes de regenerar.
func ProfileSync(home, name string) error {
	if name == "default" {
		return fmt.Errorf("'default' no tiene cc-home; no se sincroniza")
	}
	src, err := claudeSrc()
	if err != nil {
		return err
	}

	if name != "" {
		c, err := Load(home)
		if err != nil {
			return err
		}
		if _, exists := c.Profiles[name]; !exists {
			return fmt.Errorf("no existe el perfil %q", name)
		}
		if err := CfgMigrateLegacy(home, name); err != nil {
			return err
		}
		return CfgRegenerate(home, name, src)
	}

	names, err := ProfileList(home)
	if err != nil {
		return err
	}
	for _, n := range names {
		if err := CfgMigrateLegacy(home, n); err != nil {
			return err
		}
		if err := CfgRegenerate(home, n, src); err != nil {
			return err
		}
	}
	return nil
}
