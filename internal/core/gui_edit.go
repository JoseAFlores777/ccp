package core

import (
	"fmt"
	"sort"
	"strings"
)

// gui_edit.go — las operaciones que la interfaz gráfica necesita y que hasta
// ahora solo se hacían editando ccp.yaml a mano. Todas siguen el patrón del
// resto de core (Load → mutar → Save, que escribe atómico bajo flock) y validan
// ANTES de escribir: una GUI que guarda un ccp.yaml que el propio ccp rechaza al
// siguiente comando es peor que no tener GUI.

// ProfileFields son los campos editables de un perfil de proveedor. Un puntero
// nil deja el campo como está.
type ProfileFields struct {
	BaseURL    *string
	ModelPro   *string
	ModelFlash *string
	Effort     *string
}

// ProfileUpdate cambia la URL, los modelos o el esfuerzo de un perfil de
// proveedor (deepseek, kimi, glm). Un perfil official no tiene estos campos: su
// modelo lo elige la cuenta de Anthropic, y guardarle una URL lo convertiría en
// otra cosa sin que el usuario lo pidiera.
func ProfileUpdate(home, name string, f ProfileFields) error {
	if name == "default" {
		return fmt.Errorf("'default' es un perfil reservado: no tiene campos que editar")
	}
	cfg, err := Load(home)
	if err != nil {
		return err
	}
	p, ok := cfg.Profiles[name]
	if !ok {
		return fmt.Errorf("no existe el perfil %q", name)
	}
	if !IsProviderType(p.Type) {
		return fmt.Errorf("el perfil %q es de tipo %q: solo los proveedores tienen URL, modelos y esfuerzo", name, p.Type)
	}
	set := func(dst *string, v *string) {
		if v != nil {
			*dst = strings.TrimSpace(*v)
		}
	}
	set(&p.BaseURL, f.BaseURL)
	set(&p.ModelPro, f.ModelPro)
	set(&p.ModelFlash, f.ModelFlash)
	set(&p.Effort, f.Effort)
	if p.BaseURL == "" {
		return fmt.Errorf("el perfil %q necesita una URL: sin ella Claude Code hablaría con Anthropic con la key de otro proveedor", name)
	}
	cfg.Profiles[name] = p
	return Save(home, cfg)
}

// autoBlock devuelve el bloque auto_handoff o el error que dice cómo crearlo.
func autoBlock(cfg *Config) (*AutoHandoff, error) {
	if cfg.AutoHandoff == nil {
		return nil, fmt.Errorf("no hay bloque auto_handoff en ccp.yaml: créalo con `ccp auto init`")
	}
	return cfg.AutoHandoff, nil
}

// AutoSetEnabled enciende o apaga la rotación automática sin tocar la política.
func AutoSetEnabled(home string, enabled bool) error {
	cfg, err := Load(home)
	if err != nil {
		return err
	}
	block, err := autoBlock(cfg)
	if err != nil {
		return err
	}
	block.Enabled = enabled
	return Save(home, cfg)
}

// AutoAllowReplace sustituye el mapa allow_from entero. nil lo borra, y con él
// el control de permisos: cualquier primario vuelve a poder usar toda la cadena.
//
// Existe porque `ccp auto chain add` solo autoriza desde el primario de la
// carpeta actual, y el mapa de cuentas de la GUI conecta cualquier par. Se
// reemplaza el mapa entero (no una entrada) a propósito: el lienzo edita el
// grafo completo y lo aplica de una vez, y así declarar el primer permiso —que
// deja sin respaldo a todo primario sin entrada— es una decisión que la GUI ve y
// explica antes, no un efecto lateral de tocar una sola flecha.
func AutoAllowReplace(home string, allow map[string][]string) error {
	cfg, err := Load(home)
	if err != nil {
		return err
	}
	block, err := autoBlock(cfg)
	if err != nil {
		return err
	}
	if allow == nil {
		block.AllowFrom = nil
		return Save(home, cfg)
	}
	known := func(n string) bool {
		if n == "default" {
			return true
		}
		_, ok := cfg.Profiles[n]
		return ok
	}
	clean := make(map[string][]string, len(allow))
	for primary, list := range allow {
		primary = strings.TrimSpace(primary)
		if !known(primary) {
			return fmt.Errorf("allow_from: no existe el perfil %q", primary)
		}
		seen := map[string]bool{}
		out := []string{}
		for _, n := range list {
			n = strings.TrimSpace(n)
			if n == "" || n == primary || seen[n] {
				continue
			}
			if !known(n) {
				return fmt.Errorf("allow_from[%s]: no existe el perfil %q", primary, n)
			}
			seen[n] = true
			out = append(out, n)
		}
		clean[primary] = out
	}
	block.AllowFrom = clean
	return Save(home, cfg)
}

// AutoPolicyPatch son los parámetros de una política que se pueden cambiar. Un
// puntero nil deja el valor como está; "" en una duración la devuelve a su valor
// por defecto.
type AutoPolicyPatch struct {
	Threshold        *int
	MinDwell         *string
	MaxHops          *int
	ReturnCheck      *string
	ReturnIdle       *string
	CooldownStrategy *string
	CooldownFallback *string
}

// AutoPolicySet cambia los parámetros de una política, creándola si no existe.
// Valida con la misma función que usa el supervisor (Effective), así que lo que
// se guarda aquí es exactamente lo que `ccp session` aceptará.
func AutoPolicySet(home, name string, p AutoPolicyPatch) error {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "default"
	}
	cfg, err := Load(home)
	if err != nil {
		return err
	}
	block, err := autoBlock(cfg)
	if err != nil {
		return err
	}
	if block.Policies == nil {
		block.Policies = map[string]AutoPolicy{}
	}
	pol := block.Policies[name]
	if p.Threshold != nil {
		pol.Threshold = *p.Threshold
	}
	if p.MaxHops != nil {
		pol.MaxHops = *p.MaxHops
	}
	str := func(dst *string, v *string) {
		if v != nil {
			*dst = strings.TrimSpace(*v)
		}
	}
	str(&pol.MinDwell, p.MinDwell)
	str(&pol.ReturnCheck, p.ReturnCheck)
	str(&pol.ReturnIdle, p.ReturnIdle)
	str(&pol.Cooldown.Strategy, p.CooldownStrategy)
	str(&pol.Cooldown.Fallback, p.CooldownFallback)
	if _, err := pol.Effective(name); err != nil {
		return err
	}
	block.Policies[name] = pol
	return Save(home, cfg)
}

// AutoPolicyNames devuelve los nombres de las políticas, `default` primero.
func AutoPolicyNames(cfg *Config) []string {
	if cfg == nil || cfg.AutoHandoff == nil {
		return nil
	}
	var out []string
	for n := range cfg.AutoHandoff.Policies {
		if n != "default" {
			out = append(out, n)
		}
	}
	sort.Strings(out)
	if _, ok := cfg.AutoHandoff.Policies["default"]; ok {
		out = append([]string{"default"}, out...)
	}
	return out
}
