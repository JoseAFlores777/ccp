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
		En: "Commands: backup-export, backup-restore, config, doctor, sync, install",
		Es: "Comandos: backup-export, backup-restore, config, doctor, sync, install",
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
		En: "a:add d:delete r:rename s:key e:view l:login enter:detail",
		Es: "a:añadir d:borrar r:renombrar s:key e:vista l:login enter:detalle",
	},
	"tui.form.rename_profile_title": {
		En: "New name for '%s'",
		Es: "Nombre nuevo para '%s'",
	},
	"tui.form.rename_profile_desc": {
		En: "Its rules, handoff markers, login and API key move with it.",
		Es: "Sus reglas, marcadores de handoff, login y API key se mueven con él.",
	},
	"tui.form.rename_needs_new_name": {
		En: "type a name different from the current one",
		Es: "escribe un nombre distinto al actual",
	},
	"tui.form.profile_renamed": {
		En: "Profile renamed: %s → %s",
		Es: "Perfil renombrado: %s → %s",
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
		En: "tab: panel · j/k: navigate · enter: detail · c: config · : commands · L: lang · q: quit",
		Es: "tab: panel · j/k: navegar · enter: detalle · c: config · : comandos · L: idioma · q: salir",
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

	// --- config_view.go: la vista Config (modeConfig) ---
	"tui.config.eyebrow": {
		En: "configuration",
		Es: "configuración",
	},
	"tui.config.footer": {
		En: "tab: section · j/k: move · enter: act · a/d/J/K: chain · e: editor · esc: back · q: quit",
		Es: "tab: sección · j/k: mover · enter: actuar · a/d/J/K: cadena · e: editor · esc: volver · q: salir",
	},
	"tui.config.empty": {
		En: "(nothing here)",
		Es: "(nada aquí)",
	},
	"tui.config.none": {
		En: "(none)",
		Es: "(ninguno)",
	},
	"tui.config.sec_defaults": {
		En: "Defaults",
		Es: "Defaults",
	},
	"tui.config.sec_auto": {
		En: "Auto-handoff",
		Es: "Auto-handoff",
	},
	"tui.config.sec_chain": {
		En: "Chain",
		Es: "Cadena",
	},
	"tui.config.sec_allow": {
		En: "allow_from",
		Es: "allow_from",
	},
	"tui.config.sec_sensors": {
		En: "Sensors",
		Es: "Sensores",
	},
	"tui.config.hint_defaults": {
		En: "enter:edit e:open ccp.yaml",
		Es: "enter:editar e:abrir ccp.yaml",
	},
	"tui.config.hint_auto": {
		En: "enter:open ccp.yaml e:editor",
		Es: "enter:abrir ccp.yaml e:editor",
	},
	"tui.config.hint_chain": {
		En: "a:add d:remove J/K:reorder e:editor",
		Es: "a:añadir d:quitar J/K:reordenar e:editor",
	},
	"tui.config.hint_allow": {
		En: "enter:toggle authorization e:editor",
		Es: "enter:alternar autorización e:editor",
	},
	"tui.config.hint_sensors": {
		En: "enter:install/uninstall e:editor",
		Es: "enter:instalar/desinstalar e:editor",
	},
	"tui.config.gui_auto_suffix": {
		En: " (auto-detected)",
		Es: " (autodetectado)",
	},
	"tui.config.set_title": {
		En: "New value for '%s'",
		Es: "Valor nuevo para '%s'",
	},
	"tui.config.value_empty": {
		En: "the value cannot be empty",
		Es: "el valor no puede quedar vacío",
	},
	"tui.config.set_ok": {
		En: "defaults.%s = %s",
		Es: "defaults.%s = %s",
	},
	"tui.config.set_failed": {
		En: "could not write defaults.%s",
		Es: "no se pudo escribir defaults.%s",
	},
	"tui.config.auto_unset": {
		En: "not configured",
		Es: "sin configurar",
	},
	"tui.config.auto_missing": {
		En: "there is no auto_handoff block in ccp.yaml yet",
		Es: "todavía no hay bloque auto_handoff en ccp.yaml",
	},
	"tui.config.auto_missing_note": {
		En: "no auto_handoff block — press enter on this section to seed it (ccp auto init)",
		Es: "sin bloque auto_handoff — pulsa enter en esta sección para sembrarlo (ccp auto init)",
	},
	"tui.config.auto_readonly": {
		En: "read-only here: these values are edited in ccp.yaml (enter/e opens it and revalidates)",
		Es: "solo lectura aquí: estos valores se editan en ccp.yaml (enter/e lo abre y revalida)",
	},
	"tui.config.auto_init_ok": {
		En: "auto_handoff seeded in ccp.yaml.",
		Es: "auto_handoff sembrado en ccp.yaml.",
	},
	"tui.config.auto_init_failed": {
		En: "could not seed auto_handoff",
		Es: "no se pudo sembrar auto_handoff",
	},
	"tui.config.policy_invalid": {
		En: "policy '%s' does not validate; open ccp.yaml with 'e' to fix it",
		Es: "la política '%s' no valida; ábrela con 'e' en ccp.yaml para arreglarla",
	},
	"tui.config.policy_is": {
		En: "policy: %s — the order IS the preference (the first one is lent first)",
		Es: "política: %s — el orden ES la preferencia (al primero se le presta antes)",
	},
	"tui.config.chain_now": {
		En: "chain: %s",
		Es: "cadena: %s",
	},
	"tui.config.chain_failed": {
		En: "the chain was not modified",
		Es: "la cadena no se modificó",
	},
	"tui.config.chain_no_candidates": {
		En: "every profile is already in the chain.",
		Es: "todos los perfiles están ya en la cadena.",
	},
	"tui.config.chain_add_title": {
		En: "Add to the chain",
		Es: "Añadir a la cadena",
	},
	"tui.config.chain_add_desc": {
		En: "Same as 'ccp auto chain add': it also authorizes the loan in allow_from.",
		Es: "Igual que 'ccp auto chain add': también autoriza el préstamo en allow_from.",
	},
	"tui.config.chain_rm_title": {
		En: "Remove '%s' from the chain?",
		Es: "¿Quitar '%s' de la cadena?",
	},
	"tui.config.chain_rm_desc": {
		En: "Same as 'ccp auto chain rm': it also withdraws its authorization in allow_from.",
		Es: "Igual que 'ccp auto chain rm': también le retira la autorización en allow_from.",
	},
	// Las dos entradas de la cadena que el core NO cuenta como préstamo. Se
	// nombran en vez de marcarlas «bloqueado»: no es el gate quien las descarta.
	"tui.config.chain_is_primary": {
		En: "this cwd's primary: it is not lent to itself",
		Es: "primario de este cwd: no se presta a sí mismo",
	},
	"tui.config.chain_ignored": {
		En: "repeated: it does not add another loan",
		Es: "repetido: no añade otro préstamo",
	},
	"tui.config.gate_allowed": {
		En: "authorized",
		Es: "autorizado",
	},
	"tui.config.gate_denied": {
		En: "blocked",
		Es: "bloqueado",
	},
	"tui.config.gate_absent": {
		En: "no allow_from gate: every loan is permitted",
		Es: "sin gate allow_from: todos los préstamos permitidos",
	},
	"tui.config.gate_absent_note": {
		En: "allow_from is not declared, so nothing is blocked; adding here will not create the gate",
		Es: "allow_from no está declarado, así que nada se bloquea; añadir aquí no crea el gate",
	},
	"tui.config.gate_deny": {
		En: "%s has no entry: total deny",
		Es: "%s no tiene entrada: deny total",
	},
	"tui.config.gate_deny_note": {
		En: "allow_from is declared but '%s' has no entry: today nothing is lent from here",
		Es: "allow_from está declarado pero '%s' no tiene entrada: hoy no se presta nada desde aquí",
	},
	"tui.config.gate_note": {
		En: "gate of '%s' (the cwd's primary); enter authorizes or withdraws, chain included",
		Es: "gate de '%s' (el primario del cwd); enter autoriza o retira, cadena incluida",
	},
	"tui.config.gate_created": {
		En: "allow_from %s created: %s",
		Es: "allow_from %s creado: %s",
	},
	"tui.config.gate_changed": {
		En: "allow_from %s: %s",
		Es: "allow_from %s: %s",
	},
	"tui.config.gate_unchanged": {
		En: "allow_from %s: unchanged",
		Es: "allow_from %s: sin cambios",
	},
	"tui.config.sensor_on": {
		En: "installed",
		Es: "instalado",
	},
	"tui.config.sensor_off": {
		En: "not installed",
		Es: "no instalado",
	},
	"tui.config.sensor_installed": {
		En: "Sensor layer installed on '%s'.",
		Es: "Capa de sensores instalada en '%s'.",
	},
	"tui.config.sensor_uninstalled": {
		En: "Sensor layer removed from '%s'.",
		Es: "Capa de sensores quitada de '%s'.",
	},
	"tui.config.sensor_default": {
		En: "'default' has no cc-home of its own: the sensor layer cannot be installed on it",
		Es: "'default' no tiene cc-home propio: no se le puede instalar la capa de sensores",
	},
	"tui.config.sensor_sync_failed": {
		En: "hooks written, but '%s' could not be regenerated",
		Es: "hooks escritos, pero no se pudo regenerar '%s'",
	},
	"tui.config.unknown_profile": {
		En: "unknown profile: %s",
		Es: "perfil desconocido: %s",
	},
	"tui.config.edit_failed": {
		En: "the editor did not finish well",
		Es: "el editor no terminó bien",
	},
	"tui.config.edit_valid": {
		En: "%s edited and revalidated.",
		Es: "%s editado y revalidado.",
	},
	"tui.config.edit_no_validate": {
		En: "'%s' does not wait: nothing was revalidated (save and come back).",
		Es: "'%s' no espera: no se revalidó nada (guarda y vuelve).",
	},

	// --- shell.go: ventana de filas ---
	"tui.shell.more_up": {
		En: "↑ %d more",
		Es: "↑ %d más",
	},
	"tui.shell.more_down": {
		En: "↓ %d more",
		Es: "↓ %d más",
	},

	// --- profile_view.go: la vista de perfil (modeProfile) ---
	"tui.profview.eyebrow": {
		En: "profile %s",
		Es: "perfil %s",
	},
	"tui.profview.instructions": {
		En: "Instructions",
		Es: "Instrucciones",
	},
	"tui.profview.instructions_hint": {
		En: "a:add d:delete e:edit file",
		Es: "a:añadir d:borrar e:editar archivo",
	},
	"tui.profview.instructions_empty": {
		En: "(no instructions of its own — 'a' to add)",
		Es: "(sin instrucciones propias — 'a' para añadir)",
	},
	"tui.profview.instructions_sum": {
		En: "%d entries",
		Es: "%d entradas",
	},
	"tui.profview.env": {
		En: "Env",
		Es: "Env",
	},
	"tui.profview.env_hint": {
		En: "a:add enter:edit d:delete e:edit file",
		Es: "a:añadir enter:editar d:borrar e:editar archivo",
	},
	"tui.profview.env_empty": {
		En: "(no variables — 'a' to add)",
		Es: "(sin variables — 'a' para añadir)",
	},
	"tui.profview.env_sum": {
		En: "%d variables",
		Es: "%d variables",
	},
	"tui.profview.effective": {
		En: "Effective",
		Es: "Efectivo",
	},
	"tui.profview.effective_hint": {
		En: "enter:expand a:add hook",
		Es: "enter:desplegar a:añadir hook",
	},
	"tui.profview.effective_sum": {
		En: "permissions · hooks · plugins · sensors",
		Es: "permisos · hooks · plugins · sensores",
	},
	"tui.profview.permissions": {
		En: "Permissions",
		Es: "Permisos",
	},
	"tui.profview.hooks": {
		En: "Hooks",
		Es: "Hooks",
	},
	"tui.profview.plugins": {
		En: "Plugins",
		Es: "Plugins",
	},
	"tui.profview.sensors": {
		En: "Sensors",
		Es: "Sensores",
	},
	"tui.profview.footer": {
		En: "tab: panel · j/k: navigate · e: edit file · esc: back · q: quit",
		Es: "tab: panel · j/k: navegar · e: editar archivo · esc: volver · q: salir",
	},
	"tui.profview.origin_global": {
		En: "global",
		Es: "global",
	},
	"tui.profview.origin_overlay": {
		En: "overlay",
		Es: "overlay",
	},
	"tui.profview.origin_auto": {
		En: "auto",
		Es: "auto",
	},
	"tui.profview.no_file": {
		En: "this box has no editable file: sensors live in auto_handoff.hooks (press 'c')",
		Es: "esta caja no tiene archivo editable: los sensores viven en auto_handoff.hooks (pulsa 'c')",
	},
	"tui.profview.no_delete": {
		En: "hooks live in arrays with no stable id: ccp only removes them from its manifest, not from the JSON — edit the file with 'e'",
		Es: "los hooks viven en arrays sin id estable: ccp solo los saca de su manifiesto, no del JSON — edita el archivo con 'e'",
	},
	"tui.profview.global_row": {
		// Genérico a propósito: la misma clave la usan la fila global de
		// Instrucciones (~/.claude/CLAUDE.md) Y una fila OriginGlobal de Env
		// (~/.claude/settings.json) — las dos son "tu config global, no el
		// overlay de este perfil", solo cambia el archivo.
		En: "that is your global config, not this profile's overlay",
		Es: "eso es tu config global, no el overlay de este perfil",
	},
	"tui.profview.no_overlay_default": {
		En: "'default' = your GLOBAL config; edit it directly, it has no overlay",
		Es: "'default' = tu config GLOBAL; edítala directamente, no tiene overlay",
	},
	"tui.form.hook_id": {
		En: "Hook id (for the manifest, not the event name)",
		Es: "Id del hook (para el manifiesto, no el nombre del evento)",
	},
	"tui.form.hook_id_empty": {
		En: "write an id",
		Es: "escribe un id",
	},
	"tui.form.hook_json": {
		En: `JSON fragment, e.g. {"hooks":{"PostToolUse":[{"matcher":"","hooks":[{"type":"command","command":"..."}]}]}}`,
		Es: `Fragmento JSON, p. ej. {"hooks":{"PostToolUse":[{"matcher":"","hooks":[{"type":"command","command":"..."}]}]}}`,
	},
	"tui.form.hook_json_invalid": {
		En: "invalid JSON",
		Es: "JSON inválido",
	},
	"tui.form.hook_added": {
		En: "Hook '%s' added to '%s' (cc-home regenerated).",
		Es: "Hook '%s' añadido a '%s' (cc-home regenerado).",
	},
	"tui.profview.rule_removed": {
		En: "Rule removed from the overlay.",
		Es: "Regla borrada del overlay.",
	},
	"tui.profview.env_removed": {
		En: "Variable '%s' removed.",
		Es: "Variable '%s' borrada.",
	},
	"tui.form.rule_text": {
		En: "Instruction for this profile",
		Es: "Instrucción para este perfil",
	},
	"tui.form.rule_text_empty": {
		En: "write the instruction",
		Es: "escribe la instrucción",
	},
	"tui.form.rule_added_profile": {
		En: "Instruction added to '%s' (cc-home regenerated).",
		Es: "Instrucción añadida a '%s' (cc-home regenerado).",
	},
	"tui.form.rule_dup": {
		En: "That instruction was already in this profile (not duplicated).",
		Es: "Esa instrucción ya estaba en este perfil (no se duplica).",
	},
	"tui.form.env_key": {
		En: "Variable name",
		Es: "Nombre de la variable",
	},
	"tui.form.env_key_empty": {
		En: "write the variable name",
		Es: "escribe el nombre de la variable",
	},
	"tui.form.env_val": {
		En: "Value for %s",
		Es: "Valor de %s",
	},
	"tui.form.env_saved": {
		En: "%s saved in the overlay of '%s' (cc-home regenerated).",
		Es: "%s guardada en el overlay de '%s' (cc-home regenerado).",
	},
}
