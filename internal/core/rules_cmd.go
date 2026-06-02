package core

import (
	"fmt"
	"sort"
)

// rules_cmd.go — mutación de reglas de path sobre ccp.yaml (CRUD), espejo de
// ccp_rule_set / ccp_rule_del / `path clear` del oráculo bash. El motor de
// resolución (deepest-wins) vive en rules.go; aquí solo persistimos.

// RuleSet asigna (o reemplaza) la regla para path → profile en ccp.yaml. El
// path se normaliza; profile debe existir (o ser "default"). Espeja `path set`.
func RuleSet(home, path, profile string) (string, error) {
	if profile == "" {
		return "", fmt.Errorf("Uso: ccp path set <ruta> <perfil>")
	}
	norm := NormalizePath(path)
	if norm == "" {
		return "", fmt.Errorf("ruta inválida")
	}
	c, err := Load(home)
	if err != nil {
		return "", err
	}
	if profile != "default" {
		if _, ok := c.Profiles[profile]; !ok {
			return "", fmt.Errorf("el perfil '%s' no existe (ccp profile add %s ...)", profile, profile)
		}
	}
	replaced := false
	for i := range c.Rules {
		if c.Rules[i].Path == norm {
			c.Rules[i].Profile = profile
			replaced = true
			break
		}
	}
	if !replaced {
		c.Rules = append(c.Rules, Rule{Path: norm, Profile: profile})
	}
	if err := Save(home, c); err != nil {
		return "", err
	}
	return norm, nil
}

// RuleDel elimina la regla cuyo path normalizado coincide. No es error si no
// existe (idempotente). Devuelve el path normalizado. Espeja `path rm`.
func RuleDel(home, path string) (string, error) {
	norm := NormalizePath(path)
	c, err := Load(home)
	if err != nil {
		return "", err
	}
	kept := c.Rules[:0]
	for _, r := range c.Rules {
		if r.Path != norm {
			kept = append(kept, r)
		}
	}
	c.Rules = kept
	if err := Save(home, c); err != nil {
		return "", err
	}
	return norm, nil
}

// RulesClear elimina todas las reglas de path. Espeja `path clear`.
func RulesClear(home string) error {
	c, err := Load(home)
	if err != nil {
		return err
	}
	c.Rules = nil
	return Save(home, c)
}

// RulesList devuelve las reglas ordenadas por path (espeja el `sort` de
// cmd_path_list del bash).
func RulesList(home string) ([]Rule, error) {
	c, err := Load(home)
	if err != nil {
		return nil, err
	}
	out := make([]Rule, len(c.Rules))
	copy(out, c.Rules)
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}
