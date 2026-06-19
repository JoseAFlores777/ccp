package i18n

func init() {
	register(catalogTUI)
}

// catalogTUI agrupa la prosa de cara al usuario del paquete tui (títulos de
// panel, hints, footer de teclas, línea de estado, prompts/labels/botones de
// formularios, mensajes de confirmación y de estado). ES = el literal español
// actual byte-a-byte; solo EN es traducción nueva. Keys namespaced tui.*.
var catalogTUI = map[string]map[Lang]string{
	// --- tui.go: forms / commands / status helpers ---
	"tui.form.canceled": {
		En: "Canceled.",
		Es: "Cancelado.",
	},
	"tui.cmd.help": {
		En: "Commands: backup-export, backup-restore, doctor, sync, install",
		Es: "Comandos: backup-export, backup-restore, doctor, sync, install",
	},
	"tui.cmd.synced_all": {
		En: "All profiles re-synced.",
		Es: "Todos los perfiles re-sincronizados.",
	},
	"tui.cmd.doctor_done": {
		En: "diagnostics (doctor) executed",
		Es: "diagnóstico (doctor) ejecutado",
	},
	"tui.cmd.install_done": {
		En: "shell init (install) executed",
		Es: "shell init (install) ejecutado",
	},
	"tui.cmd.unknown": {
		En: "Unknown command: '%s'",
		Es: "Comando desconocido: '%s'",
	},
	"tui.shell.no_binary": {
		En: "could not find the 'ccp' binary in PATH",
		Es: "no se encontró el binario 'ccp' en PATH",
	},
	"tui.status.error_prefix": {
		En: "Error: ",
		Es: "Error: ",
	},
	"tui.form.eyebrow": {
		En: "form",
		Es: "formulario",
	},
	"tui.form.esc_cancels": {
		En: "esc cancels",
		Es: "esc cancela",
	},
	"tui.logo.tagline": {
		En: "profiles for Claude Code",
		Es: "perfiles para Claude Code",
	},

	// --- dashboard.go: panels, hints, footer ---
	"tui.profiles.title": {
		En: "Profiles",
		Es: "Perfiles",
	},
	"tui.profiles.hint": {
		En: "a:add d:delete s:key e:config l:login enter:detail",
		Es: "a:añadir d:borrar s:key e:config l:login enter:detalle",
	},
	"tui.profiles.empty": {
		En: "(no profiles — press 'a' to add)",
		Es: "(sin perfiles — pulsa 'a' para añadir)",
	},
	"tui.ptype.official": {
		En: "official",
		Es: "oficial",
	},
	"tui.ptype.deepseek": {
		En: "provider",
		Es: "proveedor",
	},
	"tui.ptype.kimi": {
		En: "provider",
		Es: "proveedor",
	},
	"tui.ptype.glm": {
		En: "provider",
		Es: "proveedor",
	},
	"tui.ptype.default": {
		En: "default",
		Es: "default",
	},
	"tui.profiles.not_provider": {
		En: "'%s' is not a provider (set key only applies to deepseek/kimi/glm)",
		Es: "'%s' no es un proveedor (set key solo aplica a deepseek/kimi/glm)",
	},
	"tui.profiles.not_official": {
		En: "'%s' is not official (login only applies to official)",
		Es: "'%s' no es official (login solo aplica a official)",
	},
	"tui.profiles.config_regen": {
		En: "Config for '%s' regenerated (global ⊕ overlay).",
		Es: "Config de '%s' regenerada (global ⊕ overlay).",
	},
	"tui.profiles.login_done": {
		En: "login for '%s' completed",
		Es: "login de '%s' completado",
	},
	"tui.profiles.health_logged_in": {
		En: "✓ logged in",
		Es: "✓ logueado",
	},
	"tui.profiles.health_no_login": {
		En: "✗ no login",
		Es: "✗ sin login",
	},
	"tui.profiles.health_key": {
		En: "✓ key",
		Es: "✓ key",
	},
	"tui.profiles.health_no_key": {
		En: "✗ no key",
		Es: "✗ sin key",
	},
	"tui.rules.title": {
		En: "Rules",
		Es: "Reglas",
	},
	"tui.rules.hint": {
		En: "a:add d:delete",
		Es: "a:añadir d:borrar",
	},
	"tui.rules.empty": {
		En: "(no rules — 'a' to add)",
		Es: "(sin reglas — 'a' para añadir)",
	},
	"tui.status.title": {
		En: "Status",
		Es: "Estado",
	},
	"tui.status.hint": {
		En: "r:recompute",
		Es: "r:recomputar",
	},
	"tui.status.not_git": {
		En: "not git",
		Es: "no es git",
	},
	"tui.status.active": {
		En: "Active profile (terminal)",
		Es: "Perfil activo (terminal)",
	},
	"tui.status.cwd_rule": {
		En: "Profile for cwd (rule)",
		Es: "Perfil del cwd (regla)",
	},
	"tui.status.cwd": {
		En: "Cwd",
		Es: "Cwd",
	},
	"tui.status.repo": {
		En: "Repo",
		Es: "Repo",
	},
	"tui.cmd.hint": {
		En: "   (tab completes · esc cancels)",
		Es: "   (tab completa · esc cancela)",
	},
	"tui.footer.keys": {
		En: "tab: panel · j/k: navigate · enter: detail · : commands · L: lang · q: quit",
		Es: "tab: panel · j/k: navegar · enter: detalle · : comandos · L: idioma · q: salir",
	},

	// --- forms.go: profile add ---
	"tui.form.profile_type": {
		En: "Profile type",
		Es: "Tipo de perfil",
	},
	"tui.form.profile_type_official": {
		En: "official (Anthropic account)",
		Es: "official (cuenta Anthropic)",
	},
	"tui.form.profile_type_deepseek": {
		En: "deepseek (compatible provider)",
		Es: "deepseek (provider compatible)",
	},
	"tui.form.profile_type_kimi": {
		En: "kimi (Moonshot provider)",
		Es: "kimi (provider Moonshot)",
	},
	"tui.form.profile_type_glm": {
		En: "glm (Z.ai provider)",
		Es: "glm (provider Z.ai)",
	},
	"tui.form.profile_name": {
		En: "Profile name",
		Es: "Nombre del perfil",
	},
	"tui.form.name_empty": {
		En: "the name cannot be empty",
		Es: "el nombre no puede estar vacío",
	},
	"tui.form.name_reserved": {
		En: "'default' is reserved",
		Es: "'default' es reservado",
	},
	"tui.form.base_url": {
		En: "Base URL",
		Es: "Base URL",
	},
	"tui.form.model_pro": {
		En: "Model pro",
		Es: "Modelo pro",
	},
	"tui.form.model_flash": {
		En: "Model flash",
		Es: "Modelo flash",
	},
	"tui.form.effort": {
		En: "Effort",
		Es: "Effort",
	},
	"tui.form.api_key_optional": {
		En: "API key (optional now)",
		Es: "API key (opcional ahora)",
	},
	"tui.form.deepseek_created": {
		En: "deepseek profile '%s' created.",
		Es: "Perfil deepseek '%s' creado.",
	},
	"tui.form.provider_created": {
		En: "%s profile '%s' created.",
		Es: "Perfil %s '%s' creado.",
	},
	"tui.form.set_key_failed": {
		En: "profile created, but set key failed",
		Es: "perfil creado, pero falló set key",
	},
	"tui.form.official_created": {
		En: "official profile '%s' created. Log in: login.",
		Es: "Perfil official '%s' creado. Inicia sesión: login.",
	},

	// --- forms.go: delete profile ---
	"tui.form.delete_profile_title": {
		En: "Delete profile '%s'?",
		Es: "¿Borrar el perfil '%s'?",
	},
	"tui.form.delete_profile_desc": {
		En: "Removes its cc-home and config. Not reversible.",
		Es: "Elimina su cc-home y config. No reversible.",
	},
	"tui.form.confirm_yes_delete": {
		En: "Yes, delete",
		Es: "Sí, borrar",
	},
	"tui.form.confirm_cancel": {
		En: "Cancel",
		Es: "Cancelar",
	},
	"tui.form.delete_canceled": {
		En: "Deletion canceled.",
		Es: "Borrado cancelado.",
	},
	"tui.form.profile_deleted": {
		En: "Profile '%s' deleted.",
		Es: "Perfil '%s' borrado.",
	},

	// --- forms.go: set key ---
	"tui.form.api_key_for": {
		En: "API key for '%s'",
		Es: "API key para '%s'",
	},
	"tui.form.key_empty": {
		En: "you did not enter any key",
		Es: "no ingresaste ninguna key",
	},
	"tui.form.key_saved": {
		En: "API key for '%s' saved (chmod 600).",
		Es: "API key de '%s' guardada (chmod 600).",
	},

	// --- forms.go: add rule ---
	"tui.form.rule_path": {
		En: "Absolute path of the rule",
		Es: "Path absoluto de la regla",
	},
	"tui.form.path_empty": {
		En: "the path cannot be empty",
		Es: "el path no puede estar vacío",
	},
	"tui.form.rule_profile": {
		En: "Profile for this path",
		Es: "Perfil para este path",
	},
	"tui.form.rule_saved": {
		En: "Rule %s → %s saved.",
		Es: "Regla %s → %s guardada.",
	},

	// --- forms.go: delete rule ---
	"tui.form.delete_rule_title": {
		En: "Delete the rule for '%s'?",
		Es: "¿Borrar la regla para '%s'?",
	},
	"tui.form.no_rule_for": {
		En: "there was no rule for %s",
		Es: "no había regla para %s",
	},
	"tui.form.rule_deleted": {
		En: "Rule for %s deleted.",
		Es: "Regla para %s borrada.",
	},

	// --- forms.go: backup export ---
	"tui.form.dest_file": {
		En: "Destination file (.tar.gz)",
		Es: "Archivo destino (.tar.gz)",
	},
	"tui.form.dest_empty": {
		En: "specify a destination file",
		Es: "indica un archivo destino",
	},
	"tui.form.include_secrets_title": {
		En: "Include secrets (api_key + login)?",
		Es: "¿Incluir secretos (api_key + login)?",
	},
	"tui.form.include_secrets_desc": {
		En: "A backup with secrets must NOT be shared or pushed to a repo.",
		Es: "Un backup con secretos NO debe compartirse ni subirse a un repo.",
	},
	"tui.form.with_secrets": {
		En: "With secrets",
		Es: "Con secretos",
	},
	"tui.form.without_secrets": {
		En: "Without secrets",
		Es: "Sin secretos",
	},
	"tui.form.backup_with_secrets": {
		En: "Backup WITH SECRETS at %s (chmod 600; do not share it).",
		Es: "Backup CON SECRETOS en %s (chmod 600; no lo compartas).",
	},
	"tui.form.backup_safe": {
		En: "Backup at %s (no secrets; safe to share).",
		Es: "Backup en %s (sin secretos; seguro de compartir).",
	},

	// --- forms.go: backup restore ---
	"tui.form.backup_file": {
		En: "Backup file (.tar.gz)",
		Es: "Archivo de backup (.tar.gz)",
	},
	"tui.form.restore_empty": {
		En: "specify the file to restore",
		Es: "indica el archivo a restaurar",
	},
	"tui.form.collision_policy": {
		En: "Collision policy",
		Es: "Política ante colisiones",
	},
	"tui.form.collision_skip": {
		En: "Skip existing (non-destructive)",
		Es: "Saltar existentes (no destructivo)",
	},
	"tui.form.collision_overwrite": {
		En: "Overwrite existing (--overwrite)",
		Es: "Sobrescribir existentes (--overwrite)",
	},
	"tui.form.collision_force": {
		En: "Force all (--force, destructive)",
		Es: "Forzar todo (--force, destructivo)",
	},
	"tui.form.restore_done": {
		En: "Restore OK. Reversible snapshot: %s (created %d, replaced %d, skipped %d, rules +%d).",
		Es: "Restore OK. Snapshot reversible: %s (creados %d, reemplazados %d, saltados %d, reglas +%d).",
	},
}
