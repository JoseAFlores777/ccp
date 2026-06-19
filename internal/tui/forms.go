package tui

import (
	"fmt"
	"time"

	"github.com/JoseAFlores777/ccp/internal/core"
	"github.com/JoseAFlores777/ccp/internal/core/i18n"
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
func formAddProfile(home string, defs core.Defaults, lang i18n.Lang) action {
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
				Title(i18n.T(lang, "tui.form.profile_type")).
				Options(
					huh.NewOption(i18n.T(lang, "tui.form.profile_type_official"), "official"),
					huh.NewOption(i18n.T(lang, "tui.form.profile_type_deepseek"), "deepseek"),
					huh.NewOption(i18n.T(lang, "tui.form.profile_type_kimi"), "kimi"),
					huh.NewOption(i18n.T(lang, "tui.form.profile_type_glm"), "glm"),
				).
				Value(&ptype),
			huh.NewInput().
				Title(i18n.T(lang, "tui.form.profile_name")).
				Value(&name).
				Validate(func(s string) error {
					if s == "" {
						return fmt.Errorf("%s", i18n.T(lang, "tui.form.name_empty"))
					}
					if s == "default" {
						return fmt.Errorf("%s", i18n.T(lang, "tui.form.name_reserved"))
					}
					return nil
				}),
		),
		huh.NewGroup(
			huh.NewInput().Title(i18n.T(lang, "tui.form.base_url")).Value(&baseURL),
			huh.NewInput().Title(i18n.T(lang, "tui.form.model_pro")).Value(&modelPro),
			huh.NewInput().Title(i18n.T(lang, "tui.form.model_flash")).Value(&modelFlash),
			huh.NewInput().Title(i18n.T(lang, "tui.form.effort")).Value(&effort),
			huh.NewInput().Title(i18n.T(lang, "tui.form.api_key_optional")).EchoMode(huh.EchoModePassword).Value(&apiKey),
		).WithHideFunc(func() bool { return ptype == "official" }),
	)

	apply := func() (string, error) {
		if ptype == "official" {
			if err := core.ProfileAddOfficial(home, name); err != nil {
				return "", err
			}
			return i18n.T(lang, "tui.form.official_created", name), nil
		}
		// Proveedor compatible (deepseek/kimi/glm). Los campos del form se
		// pre-siembran con los defaults deepseek; para kimi/glm partimos del
		// preset y respetamos solo lo que el usuario editó (difiere del seed).
		d := core.PresetDefaults(ptype)
		if ptype == "deepseek" {
			d = core.Defaults{BaseURL: baseURL, ModelPro: modelPro, ModelFlash: modelFlash, Effort: effort}
		} else {
			if baseURL != defs.BaseURL {
				d.BaseURL = baseURL
			}
			if modelPro != defs.ModelPro {
				d.ModelPro = modelPro
			}
			if modelFlash != defs.ModelFlash {
				d.ModelFlash = modelFlash
			}
			if effort != defs.Effort {
				d.Effort = effort
			}
		}
		if err := core.ProfileAddProvider(home, name, ptype, d); err != nil {
			return "", err
		}
		if apiKey != "" {
			if err := core.ProfileSetKey(home, name, apiKey); err != nil {
				return "", fmt.Errorf("%s: %w", i18n.T(lang, "tui.form.set_key_failed"), err)
			}
		}
		return i18n.T(lang, "tui.form.provider_created", ptype, name), nil
	}
	return action{form: form, apply: apply}
}

// formDeleteProfile confirma y borra el perfil dado.
func formDeleteProfile(home, name string, lang i18n.Lang) action {
	var confirm bool
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title(i18n.T(lang, "tui.form.delete_profile_title", name)).
				Description(i18n.T(lang, "tui.form.delete_profile_desc")).
				Affirmative(i18n.T(lang, "tui.form.confirm_yes_delete")).
				Negative(i18n.T(lang, "tui.form.confirm_cancel")).
				Value(&confirm),
		),
	)
	apply := func() (string, error) {
		if !confirm {
			return i18n.T(lang, "tui.form.delete_canceled"), nil
		}
		if err := core.ProfileRm(home, name); err != nil {
			return "", err
		}
		return i18n.T(lang, "tui.form.profile_deleted", name), nil
	}
	return action{form: form, apply: apply}
}

// formSetKey pide la API key de un perfil deepseek.
func formSetKey(home, name string, lang i18n.Lang) action {
	var key string
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title(i18n.T(lang, "tui.form.api_key_for", name)).
				EchoMode(huh.EchoModePassword).
				Value(&key).
				Validate(func(s string) error {
					if s == "" {
						return fmt.Errorf("%s", i18n.T(lang, "tui.form.key_empty"))
					}
					return nil
				}),
		),
	)
	apply := func() (string, error) {
		if err := core.ProfileSetKey(home, name, key); err != nil {
			return "", err
		}
		return i18n.T(lang, "tui.form.key_saved", name), nil
	}
	return action{form: form, apply: apply}
}

// formAddRule pide path + perfil y registra la regla (deepest-wins en runtime).
func formAddRule(home string, profiles []string, lang i18n.Lang) action {
	var path string
	var profile string
	if len(profiles) > 0 {
		profile = profiles[0]
	}
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title(i18n.T(lang, "tui.form.rule_path")).
				Value(&path).
				Validate(func(s string) error {
					if s == "" {
						return fmt.Errorf("%s", i18n.T(lang, "tui.form.path_empty"))
					}
					return nil
				}),
			huh.NewSelect[string]().
				Title(i18n.T(lang, "tui.form.rule_profile")).
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
		return i18n.T(lang, "tui.form.rule_saved", norm, profile), nil
	}
	return action{form: form, apply: apply}
}

// formDeleteRule confirma y borra la regla del path dado.
func formDeleteRule(home, path string, lang i18n.Lang) action {
	var confirm bool
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title(i18n.T(lang, "tui.form.delete_rule_title", path)).
				Affirmative(i18n.T(lang, "tui.form.confirm_yes_delete")).
				Negative(i18n.T(lang, "tui.form.confirm_cancel")).
				Value(&confirm),
		),
	)
	apply := func() (string, error) {
		if !confirm {
			return i18n.T(lang, "tui.form.delete_canceled"), nil
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
			return "", fmt.Errorf("%s", i18n.T(lang, "tui.form.no_rule_for", path))
		}
		if err := core.Save(home, c); err != nil {
			return "", err
		}
		return i18n.T(lang, "tui.form.rule_deleted", path), nil
	}
	return action{form: form, apply: apply}
}

// formBackupExport corre el wizard de export (destino + secretos).
func formBackupExport(home string, lang i18n.Lang) action {
	var dest string
	var withSecrets bool
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title(i18n.T(lang, "tui.form.dest_file")).
				Value(&dest).
				Validate(func(s string) error {
					if s == "" {
						return fmt.Errorf("%s", i18n.T(lang, "tui.form.dest_empty"))
					}
					return nil
				}),
			huh.NewConfirm().
				Title(i18n.T(lang, "tui.form.include_secrets_title")).
				Description(i18n.T(lang, "tui.form.include_secrets_desc")).
				Affirmative(i18n.T(lang, "tui.form.with_secrets")).
				Negative(i18n.T(lang, "tui.form.without_secrets")).
				Value(&withSecrets),
		),
	)
	apply := func() (string, error) {
		if err := core.BackupExport(home, dest, withSecrets, time.Now()); err != nil {
			return "", err
		}
		if withSecrets {
			return i18n.T(lang, "tui.form.backup_with_secrets", dest), nil
		}
		return i18n.T(lang, "tui.form.backup_safe", dest), nil
	}
	return action{form: form, apply: apply}
}

// formBackupRestore corre el wizard de restore (archivo + modo de colisión).
// --force se considera destructivo y se confirma explícitamente.
func formBackupRestore(home string, lang i18n.Lang) action {
	var archive string
	var mode = "skip"
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title(i18n.T(lang, "tui.form.backup_file")).
				Value(&archive).
				Validate(func(s string) error {
					if s == "" {
						return fmt.Errorf("%s", i18n.T(lang, "tui.form.restore_empty"))
					}
					return nil
				}),
			huh.NewSelect[string]().
				Title(i18n.T(lang, "tui.form.collision_policy")).
				Options(
					huh.NewOption(i18n.T(lang, "tui.form.collision_skip"), "skip"),
					huh.NewOption(i18n.T(lang, "tui.form.collision_overwrite"), "overwrite"),
					huh.NewOption(i18n.T(lang, "tui.form.collision_force"), "force"),
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
		return i18n.T(lang, "tui.form.restore_done",
			rep.SnapshotDir, len(rep.Created), len(rep.Overwritten), len(rep.Skipped), rep.RulesAdded), nil
	}
	return action{form: form, apply: apply}
}
