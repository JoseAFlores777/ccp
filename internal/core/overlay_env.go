package core

// overlay_env.go — escritura de variables de entorno en el overlay de un perfil.
// Es la única API de escritura nueva de la vista de perfil: las reglas usan
// InstructRuleAdd/Rm y los hooks InstructAdd, que ya existen.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
)

// OverlayEnvSet fija env.<key> = val en el overlay del perfil y regenera su
// cc-home.
func OverlayEnvSet(home, name, key, val string) error {
	if key == "" {
		return fmt.Errorf("la variable no puede estar vacía")
	}
	return overlayEnvMutate(home, name, func(env map[string]any) {
		env[key] = val
	})
}

// OverlayEnvDel quita env.<key>. Borrar una clave que no está es un no-op.
func OverlayEnvDel(home, name, key string) error {
	if key == "" {
		return fmt.Errorf("la variable no puede estar vacía")
	}
	return overlayEnvMutate(home, name, func(env map[string]any) {
		delete(env, key)
	})
}

// overlayEnvMutate es el camino común: lee, muta, VALIDA, escribe y regenera.
// El orden importa — mismo invariante que ProfileConfig: si el resultado no
// valida, el último overlay bueno no se toca.
func overlayEnvMutate(home, name string, fn func(map[string]any)) error {
	if name == "default" {
		return fmt.Errorf("'default' = tu config GLOBAL; edítala directamente, no tiene overlay")
	}
	if err := CfgInitOverlay(home, name); err != nil {
		return err
	}
	file := cfgSettingsFile(home, name)
	data, err := os.ReadFile(file)
	if err != nil {
		return fmt.Errorf("no se pudo leer el overlay de %q: %w", name, err)
	}

	doc := map[string]any{}
	if len(bytes.TrimSpace(data)) > 0 {
		dec := json.NewDecoder(bytes.NewReader(data))
		dec.UseNumber()
		if err := dec.Decode(&doc); err != nil {
			return fmt.Errorf("el overlay de %q no es JSON válido: %w — arréglalo con: ccp profile config %s", name, err, name)
		}
	}

	env, _ := doc["env"].(map[string]any)
	if env == nil {
		env = map[string]any{}
	}
	fn(env)
	if len(env) == 0 {
		delete(doc, "env")
	} else {
		doc["env"] = env
	}

	out, err := marshalIndent(doc)
	if err != nil {
		return fmt.Errorf("no se pudo serializar el overlay de %q: %w", name, err)
	}
	if err := CfgValidateBytes(out); err != nil {
		return fmt.Errorf("el overlay resultante no valida: %w", err)
	}
	// 0o644, no 0o600: el overlay no guarda secretos (la api_key vive aparte,
	// en profiles/<name>/api_key con 0600) — mismo modo que ya usa
	// CfgInitOverlay al crear el archivo (cfg.go:147).
	if err := os.WriteFile(file, out, 0o644); err != nil {
		return fmt.Errorf("no se pudo escribir el overlay de %q: %w", name, err)
	}

	src, err := claudeSrc()
	if err != nil {
		return err
	}
	return CfgRegenerate(home, name, src)
}
