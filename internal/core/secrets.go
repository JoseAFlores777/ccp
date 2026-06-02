package core

import (
	"fmt"
	"os"
	"path/filepath"
)

// Los secretos (api_key de providers) NUNCA entran a ccp.yaml. Viven en
// <home>/profiles/<profile>/api_key con permisos 600. cc-home/.claude.json
// (credenciales OAuth de cuentas oficiales) tampoco se toca aquí.

func apiKeyPath(home, profile string) string {
	return filepath.Join(home, "profiles", profile, "api_key")
}

// SetKey escribe la api_key del perfil en disco con chmod 600. Crea el
// directorio del perfil si hace falta.
func SetKey(home, profile, key string) error {
	if profile == "" || profile == "default" {
		return fmt.Errorf("perfil inválido para api_key: %q", profile)
	}
	path := apiKeyPath(home, profile)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("no se pudo crear directorio del perfil %q: %w", profile, err)
	}
	if err := os.WriteFile(path, []byte(key), 0o600); err != nil {
		return fmt.Errorf("no se pudo escribir api_key de %q: %w", profile, err)
	}
	// Reafirma el modo por si el archivo ya existía con otros permisos.
	if err := os.Chmod(path, 0o600); err != nil {
		return fmt.Errorf("no se pudo aplicar chmod 600 a api_key de %q: %w", profile, err)
	}
	return nil
}

// GetKey lee la api_key del perfil. El segundo valor es false si no existe.
func GetKey(home, profile string) (string, bool) {
	data, err := os.ReadFile(apiKeyPath(home, profile))
	if err != nil {
		return "", false
	}
	return string(data), true
}
