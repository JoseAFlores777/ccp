package core

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ccHomePath devuelve <home>/profiles/<name>/cc-home.
func ccHomePath(home, name string) string {
	return filepath.Join(home, "profiles", name, "cc-home")
}

// profileDirPath devuelve <home>/profiles/<name>.
func profileDirPath(home, name string) string {
	return filepath.Join(home, "profiles", name)
}

// ProfileAddOfficial añade un perfil de tipo "official" a ccp.yaml y siembra
// su cc-home. Devuelve error si ya existe o si el nombre es "default".
func ProfileAddOfficial(home, name string) error {
	if name == "" || name == "default" {
		return fmt.Errorf("nombre de perfil inválido: %q", name)
	}
	c, err := Load(home)
	if err != nil {
		return err
	}
	if _, exists := c.Profiles[name]; exists {
		return fmt.Errorf("el perfil %q ya existe", name)
	}
	c.Profiles[name] = Profile{Type: "official"}
	if err := Save(home, c); err != nil {
		return err
	}
	return seedCCHome(home, name)
}

// ProfileAddDeepseek añade un perfil de tipo "deepseek" a ccp.yaml con los 4
// campos explícitos desde d (no herencia en runtime). Siembra el cc-home.
// Devuelve error si ya existe o si el nombre es "default".
func ProfileAddDeepseek(home, name string, d Defaults) error {
	if name == "" || name == "default" {
		return fmt.Errorf("nombre de perfil inválido: %q", name)
	}
	c, err := Load(home)
	if err != nil {
		return err
	}
	if _, exists := c.Profiles[name]; exists {
		return fmt.Errorf("el perfil %q ya existe", name)
	}
	c.Profiles[name] = Profile{
		Type:       "deepseek",
		BaseURL:    d.BaseURL,
		ModelPro:   d.ModelPro,
		ModelFlash: d.ModelFlash,
		Effort:     d.Effort,
	}
	if err := Save(home, c); err != nil {
		return err
	}
	return seedCCHome(home, name)
}

// ProfileRm elimina un perfil de ccp.yaml y borra su directorio
// (<home>/profiles/<name>). Rechaza el perfil reservado "default".
func ProfileRm(home, name string) error {
	if name == "" || name == "default" {
		return fmt.Errorf("no se puede eliminar el perfil reservado %q", name)
	}
	c, err := Load(home)
	if err != nil {
		return err
	}
	if _, exists := c.Profiles[name]; !exists {
		return fmt.Errorf("no existe el perfil %q", name)
	}
	delete(c.Profiles, name)
	if err := Save(home, c); err != nil {
		return err
	}
	dir := profileDirPath(home, name)
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("no se pudo eliminar directorio del perfil %q: %w", name, err)
	}
	return nil
}

// ProfileList devuelve los nombres de todos los perfiles definidos en
// ccp.yaml, ordenados alfabéticamente (espeja ccp_profile_list del bash).
func ProfileList(home string) ([]string, error) {
	c, err := Load(home)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(c.Profiles))
	for name := range c.Profiles {
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}

// ProfileShow devuelve un string de detalle humano para un perfil, espejando
// el formato de _profile_show del bash (NO_COLOR / sin emojis en la salida
// estructurada para comparación de texto).
func ProfileShow(home, name string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("Uso: ccp profile show <nombre>")
	}
	c, err := Load(home)
	if err != nil {
		return "", err
	}
	p, exists := c.Profiles[name]
	if !exists {
		return "", fmt.Errorf("no existe %q", name)
	}

	var sb strings.Builder
	sb.WriteString("---\n")
	fmt.Fprintf(&sb, " Perfil: %s\n", name)
	sb.WriteString("---\n")
	fmt.Fprintf(&sb, " Tipo:        %s\n", p.Type)

	switch p.Type {
	case "deepseek":
		fmt.Fprintf(&sb, " Base URL:    %s\n", p.BaseURL)
		fmt.Fprintf(&sb, " Modelo pro:  %s\n", p.ModelPro)
		fmt.Fprintf(&sb, " Modelo flash:%s\n", p.ModelFlash)
		fmt.Fprintf(&sb, " Effort:      %s\n", p.Effort)
		_, hasKey := GetKey(home, name)
		if hasKey {
			fmt.Fprintf(&sb, " API key:     OK\n")
		} else {
			fmt.Fprintf(&sb, " API key:     falta (ccp key %s)\n", name)
		}
	case "official":
		cch := ccHomePath(home, name)
		fmt.Fprintf(&sb, " Config dir:  %s\n", cch)
		claudeJSON := filepath.Join(cch, ".claude.json")
		if fileExists(claudeJSON) {
			fmt.Fprintf(&sb, " Login:       configurado\n")
		} else {
			fmt.Fprintf(&sb, " Login:       pendiente (ccp profile login %s)\n", name)
		}
	}
	sb.WriteString("---\n")
	return sb.String(), nil
}

// ProfileSetKey valida que el perfil exista y sea de tipo deepseek, luego
// delega en SetKey para escribir el secreto a disco (chmod 600). Nunca
// escribe la clave en ccp.yaml. Espeja las validaciones de cmd_key del bash.
func ProfileSetKey(home, name, key string) error {
	if name == "" || name == "default" {
		return fmt.Errorf("nombre de perfil inválido: %q", name)
	}
	c, err := Load(home)
	if err != nil {
		return err
	}
	p, exists := c.Profiles[name]
	if !exists {
		return fmt.Errorf("no existe el perfil %q", name)
	}
	if p.Type != "deepseek" {
		return fmt.Errorf("%q no es un perfil deepseek", name)
	}
	if key == "" {
		return fmt.Errorf("no ingresaste ninguna key")
	}
	return SetKey(home, name, key)
}

// seedCCHome porta _seed_cc_home del bash: crea <home>/profiles/<name>/cc-home/
// y añade symlinks para plugins/, commands/, agents/, skills/ apuntando al
// directorio fuente (CCP_CLAUDE_SRC o ~/.claude por defecto). Solo crea cada
// symlink si la entrada existe en la fuente y NO existe aún en el cc-home.
// CLAUDE.md y settings.json son responsabilidad de cfg (fuera de este issue).
func seedCCHome(home, name string) error {
	cch := ccHomePath(home, name)
	if err := os.MkdirAll(cch, 0o755); err != nil {
		return fmt.Errorf("no se pudo crear cc-home para %q: %w", name, err)
	}

	src := os.Getenv("CCP_CLAUDE_SRC")
	if src == "" {
		userHome, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("no se pudo determinar HOME para cc-home de %q: %w", name, err)
		}
		src = filepath.Join(userHome, ".claude")
	}

	// Solo crea symlink si la entrada existe en src y aún no existe en cch.
	for _, item := range []string{"plugins", "commands", "agents", "skills"} {
		srcItem := filepath.Join(src, item)
		dstItem := filepath.Join(cch, item)

		// ¿Existe en src?
		if _, err := os.Lstat(srcItem); err != nil {
			continue // no existe en src -> saltar
		}
		// ¿Ya existe en cch (ya sea archivo, directorio o symlink)?
		if _, err := os.Lstat(dstItem); err == nil {
			continue // ya existe -> no sobreescribir
		}
		if err := os.Symlink(srcItem, dstItem); err != nil {
			return fmt.Errorf("no se pudo crear symlink %s -> %s: %w", dstItem, srcItem, err)
		}
	}
	return nil
}
