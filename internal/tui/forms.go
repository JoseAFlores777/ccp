package tui

import (
	"fmt"
	"time"

	"github.com/JoseAFlores777/ccp/internal/core"
	"github.com/charmbracelet/huh"
)

// forms.go — constructores de formularios huh para cada acción de la TUI. Cada
// constructor devuelve un *huh.Form y un `apply func() (string, error)` que
// ejecuta la acción contra internal/core al completar el form. La TUI nunca
// duplica lógica: estos apply solo llaman al core (o a `ccp <subcmd>` cuando no
// hay equivalente en core en esta rama). El string devuelto es el mensaje de
// éxito que la barra de estado muestra.
//
// Patrón: el modelo raíz embebe el *huh.Form como subvista; al pasar a
// StateCompleted invoca apply(); al StateAborted descarta. El foco vuelve al
// panel sin salir ni limpiar pantalla.

// action agrupa el form a mostrar y el callback que lo materializa.
type action struct {
	form  *huh.Form
	apply func() (string, error)
}

// profileOptions arma las opciones de un select de perfiles (incluye "default"
// si includeDefault). Devuelve []huh.Option[string].
func profileOptions(names []string, includeDefault bool) []huh.Option[string] {
	opts := make([]huh.Option[string], 0, len(names)+1)
	if includeDefault {
		opts = append(opts, huh.NewOption("default", "default"))
	}
	for _, n := range names {
		opts = append(opts, huh.NewOption(n, n))
	}
	return opts
}

// formAddProfile pide tipo + nombre (+ campos deepseek) y crea el perfil.
func formAddProfile(home string, defs core.Defaults) action {
	var ptype = "official"
	var name string
	var baseURL = defs.BaseURL
	var modelPro = defs.ModelPro
	var modelFlash = defs.ModelFlash
	var effort = defs.Effort
	var apiKey string

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Tipo de perfil").
				Options(
					huh.NewOption("official (cuenta Anthropic)", "official"),
					huh.NewOption("deepseek (provider compatible)", "deepseek"),
				).
				Value(&ptype),
			huh.NewInput().
				Title("Nombre del perfil").
				Value(&name).
				Validate(func(s string) error {
					if s == "" {
						return fmt.Errorf("el nombre no puede estar vacío")
					}
					if s == "default" {
						return fmt.Errorf("'default' es reservado")
					}
					return nil
				}),
		),
		huh.NewGroup(
			huh.NewInput().Title("Base URL").Value(&baseURL),
			huh.NewInput().Title("Modelo pro").Value(&modelPro),
			huh.NewInput().Title("Modelo flash").Value(&modelFlash),
			huh.NewInput().Title("Effort").Value(&effort),
			huh.NewInput().Title("API key (opcional ahora)").EchoMode(huh.EchoModePassword).Value(&apiKey),
		).WithHideFunc(func() bool { return ptype != "deepseek" }),
	)

	apply := func() (string, error) {
		if ptype == "deepseek" {
			d := core.Defaults{BaseURL: baseURL, ModelPro: modelPro, ModelFlash: modelFlash, Effort: effort}
			if err := core.ProfileAddDeepseek(home, name, d); err != nil {
				return "", err
			}
			if apiKey != "" {
				if err := core.ProfileSetKey(home, name, apiKey); err != nil {
					return "", fmt.Errorf("perfil creado, pero falló set key: %w", err)
				}
			}
			return fmt.Sprintf("Perfil deepseek '%s' creado.", name), nil
		}
		if err := core.ProfileAddOfficial(home, name); err != nil {
			return "", err
		}
		return fmt.Sprintf("Perfil official '%s' creado. Inicia sesión: login.", name), nil
	}
	return action{form: form, apply: apply}
}

// formDeleteProfile confirma y borra el perfil dado.
func formDeleteProfile(home, name string) action {
	var confirm bool
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title(fmt.Sprintf("¿Borrar el perfil '%s'?", name)).
				Description("Elimina su cc-home y config. No reversible.").
				Affirmative("Sí, borrar").
				Negative("Cancelar").
				Value(&confirm),
		),
	)
	apply := func() (string, error) {
		if !confirm {
			return "Borrado cancelado.", nil
		}
		if err := core.ProfileRm(home, name); err != nil {
			return "", err
		}
		return fmt.Sprintf("Perfil '%s' borrado.", name), nil
	}
	return action{form: form, apply: apply}
}

// formSetKey pide la API key de un perfil deepseek.
func formSetKey(home, name string) action {
	var key string
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title(fmt.Sprintf("API key para '%s'", name)).
				EchoMode(huh.EchoModePassword).
				Value(&key).
				Validate(func(s string) error {
					if s == "" {
						return fmt.Errorf("no ingresaste ninguna key")
					}
					return nil
				}),
		),
	)
	apply := func() (string, error) {
		if err := core.ProfileSetKey(home, name, key); err != nil {
			return "", err
		}
		return fmt.Sprintf("API key de '%s' guardada (chmod 600).", name), nil
	}
	return action{form: form, apply: apply}
}

// formAddRule pide path + perfil y registra la regla (deepest-wins en runtime).
func formAddRule(home string, profiles []string) action {
	var path string
	var profile string
	if len(profiles) > 0 {
		profile = profiles[0]
	}
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Path absoluto de la regla").
				Value(&path).
				Validate(func(s string) error {
					if s == "" {
						return fmt.Errorf("el path no puede estar vacío")
					}
					return nil
				}),
			huh.NewSelect[string]().
				Title("Perfil para este path").
				Options(profileOptions(profiles, true)...).
				Value(&profile),
		),
	)
	apply := func() (string, error) {
		c, err := core.Load(home)
		if err != nil {
			return "", err
		}
		norm := core.NormalizePath(path)
		// 'path set' reemplaza si ya existe; deepest-wins en runtime.
		replaced := false
		for i := range c.Rules {
			if c.Rules[i].Path == norm {
				c.Rules[i].Profile = profile
				replaced = true
				break
			}
		}
		if !replaced {
			c.Rules = append(c.Rules, core.Rule{Path: norm, Profile: profile})
		}
		if err := core.Save(home, c); err != nil {
			return "", err
		}
		return fmt.Sprintf("Regla %s → %s guardada.", norm, profile), nil
	}
	return action{form: form, apply: apply}
}

// formDeleteRule confirma y borra la regla del path dado.
func formDeleteRule(home, path string) action {
	var confirm bool
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title(fmt.Sprintf("¿Borrar la regla para '%s'?", path)).
				Affirmative("Sí, borrar").
				Negative("Cancelar").
				Value(&confirm),
		),
	)
	apply := func() (string, error) {
		if !confirm {
			return "Borrado cancelado.", nil
		}
		c, err := core.Load(home)
		if err != nil {
			return "", err
		}
		out := c.Rules[:0]
		removed := false
		for _, r := range c.Rules {
			if r.Path == path {
				removed = true
				continue
			}
			out = append(out, r)
		}
		c.Rules = out
		if !removed {
			return "", fmt.Errorf("no había regla para %s", path)
		}
		if err := core.Save(home, c); err != nil {
			return "", err
		}
		return fmt.Sprintf("Regla para %s borrada.", path), nil
	}
	return action{form: form, apply: apply}
}

// formBackupExport corre el wizard de export (destino + secretos).
func formBackupExport(home string) action {
	var dest string
	var withSecrets bool
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Archivo destino (.tar.gz)").
				Value(&dest).
				Validate(func(s string) error {
					if s == "" {
						return fmt.Errorf("indica un archivo destino")
					}
					return nil
				}),
			huh.NewConfirm().
				Title("¿Incluir secretos (api_key + login)?").
				Description("Un backup con secretos NO debe compartirse ni subirse a un repo.").
				Affirmative("Con secretos").
				Negative("Sin secretos").
				Value(&withSecrets),
		),
	)
	apply := func() (string, error) {
		if err := core.BackupExport(home, dest, withSecrets, time.Now()); err != nil {
			return "", err
		}
		if withSecrets {
			return fmt.Sprintf("Backup CON SECRETOS en %s (chmod 600; no lo compartas).", dest), nil
		}
		return fmt.Sprintf("Backup en %s (sin secretos; seguro de compartir).", dest), nil
	}
	return action{form: form, apply: apply}
}

// formBackupRestore corre el wizard de restore (archivo + modo de colisión).
// --force se considera destructivo y se confirma explícitamente.
func formBackupRestore(home string) action {
	var archive string
	var mode = "skip"
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Archivo de backup (.tar.gz)").
				Value(&archive).
				Validate(func(s string) error {
					if s == "" {
						return fmt.Errorf("indica el archivo a restaurar")
					}
					return nil
				}),
			huh.NewSelect[string]().
				Title("Política ante colisiones").
				Options(
					huh.NewOption("Saltar existentes (no destructivo)", "skip"),
					huh.NewOption("Sobrescribir existentes (--overwrite)", "overwrite"),
					huh.NewOption("Forzar todo (--force, destructivo)", "force"),
				).
				Value(&mode),
		),
	)
	apply := func() (string, error) {
		opts := core.RestoreOpts{Now: time.Now()}
		switch mode {
		case "overwrite":
			opts.Overwrite = true
		case "force":
			opts.Force = true
		}
		rep, err := core.BackupRestore(home, archive, opts)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("Restore OK. Snapshot reversible: %s (creados %d, reemplazados %d, saltados %d, reglas +%d).",
			rep.SnapshotDir, len(rep.Created), len(rep.Overwritten), len(rep.Skipped), rep.RulesAdded), nil
	}
	return action{form: form, apply: apply}
}
