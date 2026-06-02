package core

import "fmt"

// El bloque `defaults` de ccp.yaml es la plantilla que SOLO siembra perfiles
// deepseek nuevos (ProfileAddDeepseek). Editar defaults NUNCA muta perfiles
// existentes: estos guardan sus 4 campos completos (sin herencia en runtime).

// BuiltinDefaults devuelve los valores de fábrica del bloque `defaults`,
// espejo de los CCP_*_DEFAULT del bash. `reset` restaura exactamente esto.
func BuiltinDefaults() Defaults {
	return Defaults{
		BaseURL:    "https://api.deepseek.com/anthropic",
		ModelPro:   "deepseek-chat",
		ModelFlash: "deepseek-chat",
		Effort:     "high",
		Editor:     "nano",
	}
}

// GetDefaults lee el bloque `defaults` de ccp.yaml. Si el archivo no existe o
// el bloque está vacío, devuelve los valores built-in: la lectura nunca
// inventa un perfil ni escribe a disco.
func GetDefaults(home string) (Defaults, error) {
	c, err := Load(home)
	if err != nil {
		return Defaults{}, err
	}
	return effectiveDefaults(c.Defaults), nil
}

// effectiveDefaults completa con built-ins los campos vacíos del bloque leído,
// de modo que `config show` nunca muestre líneas en blanco aunque ccp.yaml solo
// declare algunos campos (o ninguno).
func effectiveDefaults(d Defaults) Defaults {
	b := BuiltinDefaults()
	if d.BaseURL == "" {
		d.BaseURL = b.BaseURL
	}
	if d.ModelPro == "" {
		d.ModelPro = b.ModelPro
	}
	if d.ModelFlash == "" {
		d.ModelFlash = b.ModelFlash
	}
	if d.Effort == "" {
		d.Effort = b.Effort
	}
	if d.Editor == "" {
		d.Editor = b.Editor
	}
	return d
}

// configKeys son las claves que `config set` acepta (excluye editor, que tiene
// su propio subcomando). El orden no importa.
var configKeys = map[string]struct{}{
	"base_url":    {},
	"model_pro":   {},
	"model_flash": {},
	"effort":      {},
}

// SetDefault fija una clave del bloque `defaults` en ccp.yaml y persiste.
// Acepta base_url|model_pro|model_flash|effort. Rechaza claves desconocidas y
// valores vacíos (espeja la validación de `cmd_config set` del bash).
func SetDefault(home, key, value string) error {
	if key == "" || value == "" {
		return fmt.Errorf("Uso: ccp config set <clave> <valor>")
	}
	if _, ok := configKeys[key]; !ok {
		return fmt.Errorf("clave desconocida: %s", key)
	}
	c, err := Load(home)
	if err != nil {
		return err
	}
	// Sembramos los campos hermanos vacíos con built-ins para que el bloque
	// `defaults` persistido quede siempre completo (no strings vacíos sueltos).
	// Editor NO se rellena: "" sigue significando "usa $EDITOR".
	c.Defaults = fillProviderDefaults(c.Defaults)
	switch key {
	case "base_url":
		c.Defaults.BaseURL = value
	case "model_pro":
		c.Defaults.ModelPro = value
	case "model_flash":
		c.Defaults.ModelFlash = value
	case "effort":
		c.Defaults.Effort = value
	}
	return Save(home, c)
}

// fillProviderDefaults completa con built-ins los 4 campos de provider
// (base_url/model_pro/model_flash/effort) si están vacíos, dejando Editor
// intacto. Se usa al persistir para no escribir strings vacíos.
func fillProviderDefaults(d Defaults) Defaults {
	editor := d.Editor
	d = effectiveDefaults(d)
	d.Editor = editor
	return d
}

// SetEditor fija el campo `editor` del bloque `defaults` y persiste. Un valor
// vacío es rechazado (para limpiarlo se edita ccp.yaml a mano).
func SetEditor(home, editor string) error {
	if editor == "" {
		return fmt.Errorf("Uso: ccp config editor <comando>")
	}
	c, err := Load(home)
	if err != nil {
		return err
	}
	c.Defaults = fillProviderDefaults(c.Defaults)
	c.Defaults.Editor = editor
	return Save(home, c)
}

// GetEditor resuelve el editor a usar: editor configurado en defaults, si no
// $EDITOR, si no "nano" (espeja _resolve_editor del bash). envEditor es el
// valor de $EDITOR (el caller lo pasa para no acoplar core a os.Getenv).
func GetEditor(home, envEditor string) (string, error) {
	c, err := Load(home)
	if err != nil {
		return "", err
	}
	if c.Defaults.Editor != "" {
		return c.Defaults.Editor, nil
	}
	if envEditor != "" {
		return envEditor, nil
	}
	return "nano", nil
}

// ResetDefaults restaura el bloque `defaults` a los valores built-in y
// persiste. No toca perfiles ni reglas.
func ResetDefaults(home string) error {
	c, err := Load(home)
	if err != nil {
		return err
	}
	c.Defaults = BuiltinDefaults()
	return Save(home, c)
}
