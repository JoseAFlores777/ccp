package i18n

func init() {
	register(map[string]map[Lang]string{
		"lang.current": {
			En: "Language: %s (source: %s)",
			Es: "Idioma: %s (fuente: %s)",
		},
		"lang.set": {
			En: "Language set to %s.",
			Es: "Idioma cambiado a %s.",
		},
		"lang.invalid": {
			En: "Unknown language %q. Use 'en' or 'es'.",
			Es: "Idioma desconocido %q. Usa 'en' o 'es'.",
		},
	})

	register(catalogCLI)
}

// catalogCLI agrupa la prosa de terminal del paquete cli (mensajes de éxito,
// errores al usuario, cabeceras de sección, ayuda). ES = el literal español
// actual byte-a-byte; solo EN es traducción nueva.
var catalogCLI = map[string]map[Lang]string{
	// --- cli.go: dispatch + backup ---
	"cli.err.shell_only": {
		En: "[error] '%s' only works via the 'ccp' shell function (run 'ccp install' and reload your shell).",
		Es: "[error] '%s' solo funciona vía la función shell 'ccp' (corre 'ccp install' y recarga tu shell).",
	},
	"cli.err.unknown_cmd": {
		En: "Unknown command: '%s'",
		Es: "Comando desconocido: '%s'",
	},
	"cli.backup.unknown_opt": {
		En: "backup export: unknown option '%s'",
		Es: "backup export: opción desconocida '%s'",
	},
	"cli.backup.usage_export": {
		En: "Usage: ccp backup export <file.tar.gz> [--with-secrets]",
		Es: "Uso: ccp backup export <archivo.tar.gz> [--with-secrets]",
	},
	"cli.backup.written_secrets": {
		En: "Backup written to %s (chmod 600).",
		Es: "Backup escrito en %s (chmod 600).",
	},
	"cli.backup.warn_secrets": {
		En: "WARNING: this backup contains SECRETS (api_key + login credentials). Do not share it or push it to a repo.",
		Es: "ADVERTENCIA: este backup contiene SECRETOS (api_key + credenciales de login). No lo compartas ni lo subas a un repo.",
	},
	"cli.backup.written_safe": {
		En: "Backup written to %s (no secrets; safe to share).",
		Es: "Backup escrito en %s (sin secretos; seguro de compartir).",
	},
	"cli.backup.restore_unknown_opt": {
		En: "backup restore: unknown option '%s'",
		Es: "backup restore: opción desconocida '%s'",
	},
	"cli.backup.usage_restore": {
		En: "Usage: ccp backup restore <file.tar.gz> [--overwrite | --force]",
		Es: "Uso: ccp backup restore <archivo.tar.gz> [--overwrite | --force]",
	},
	"cli.backup.restore_done": {
		En: "Restore complete. Reversible snapshot at %s",
		Es: "Restore completado. Snapshot reversible en %s",
	},
	"cli.backup.restore_created": {
		En: "  Created:     %s",
		Es: "  Creados:     %s",
	},
	"cli.backup.restore_overwritten": {
		En: "  Replaced:    %s",
		Es: "  Reemplazados: %s",
	},
	"cli.backup.restore_skipped": {
		En: "  Skipped:     %s (use --overwrite to replace)",
		Es: "  Saltados:    %s (usa --overwrite para reemplazar)",
	},
	"cli.backup.restore_rules": {
		En: "  Rules added: %d",
		Es: "  Reglas añadidas: %d",
	},
	"cli.backup.unknown_sub": {
		En: "backup: unknown subcommand '%s' (export|restore)",
		Es: "backup: subcomando desconocido '%s' (export|restore)",
	},

	// --- profile.go ---
	"cli.profile.usage_rm": {
		En: "Usage: ccp profile rm <name>",
		Es: "Uso: ccp profile rm <nombre>",
	},
	"cli.profile.removed": {
		En: "Profile removed: %s",
		Es: "Perfil eliminado: %s",
	},
	"cli.profile.usage_show": {
		En: "Usage: ccp profile show <name>",
		Es: "Uso: ccp profile show <nombre>",
	},
	"cli.profile.config_updated": {
		En: "Config for '%s' updated (global ⊕ overlay).",
		Es: "Config de '%s' actualizada (global ⊕ overlay).",
	},
	"cli.profile.synced_all": {
		En: "All profiles re-synced.",
		Es: "Todos los perfiles re-sincronizados.",
	},
	"cli.profile.synced_one": {
		En: "Profile '%s' re-synced (global ⊕ overlay).",
		Es: "Perfil '%s' re-sincronizado (global ⊕ overlay).",
	},
	"cli.profile.unknown_sub": {
		En: "profile: unknown subcommand '%s'",
		Es: "profile: subcomando desconocido '%s'",
	},
	"cli.profile.sub_help": {
		En: "Use: add | rm | list | show | login | config | sync",
		Es: "Usa: add | rm | list | show | login | config | sync",
	},
	"cli.profile.usage_add": {
		En: "Usage: ccp profile add <name> --official|--deepseek|--kimi|--glm [opts]",
		Es: "Uso: ccp profile add <nombre> --official|--deepseek|--kimi|--glm [opts]",
	},
	"cli.profile.reserved_default": {
		En: "[error] 'default' is a reserved profile.",
		Es: "[error] 'default' es un perfil reservado.",
	},
	"cli.profile.unknown_opt": {
		En: "[error] unknown option: %s",
		Es: "[error] opción desconocida: %s",
	},
	"cli.profile.official_created": {
		En: "Official profile '%s' created (plugins/skills symlinked, config generated).",
		Es: "Perfil oficial '%s' creado (plugins/skills symlinked, config generada).",
	},
	"cli.profile.official_login_hint": {
		En: "Log in once:  ccp profile login %s   (run /login inside)",
		Es: "Loguéate una vez:  ccp profile login %s   (corre /login dentro)",
	},
	"cli.profile.deepseek_created": {
		En: "deepseek profile '%s' created (cc-home + config generated).",
		Es: "Perfil deepseek '%s' creado (cc-home + config generada).",
	},
	"cli.profile.deepseek_key_hint": {
		En: "Add its API key:  ccp key %s",
		Es: "Añade su API key:  ccp key %s",
	},
	"cli.profile.provider_created": {
		En: "%s profile '%s' created (cc-home + config generated).",
		Es: "Perfil %s '%s' creado (cc-home + config generada).",
	},
	"cli.profile.provider_key_hint": {
		En: "Add its API key:  ccp key %s",
		Es: "Añade su API key:  ccp key %s",
	},
	"cli.profile.specify_kind": {
		En: "[error] Specify --official, --deepseek, --kimi or --glm",
		Es: "[error] Especifica --official, --deepseek, --kimi o --glm",
	},
	"cli.profile.usage_login": {
		En: "Usage: ccp profile login <name>",
		Es: "Uso: ccp profile login <nombre>",
	},
	"cli.profile.not_found": {
		En: "[error] '%s' does not exist.",
		Es: "[error] No existe '%s'.",
	},
	"cli.profile.not_official": {
		En: "[error] '%s' is not official.",
		Es: "[error] '%s' no es oficial.",
	},
	"cli.profile.claude_missing": {
		En: "[error] Claude Code is not installed.",
		Es: "[error] Claude Code no está instalado.",
	},
	"cli.profile.login_opening": {
		En: "Opening Claude Code with the config dir of '%s'.",
		Es: "Abriendo Claude Code con el config dir de '%s'.",
	},
	"cli.profile.login_inside": {
		En: "Inside, run  /login  with this profile's account, then /quit.",
		Es: "Dentro, corre  /login  con la cuenta de este perfil, luego /quit.",
	},

	// --- present.go: profile type labels ---
	"cli.ptype.official": {En: "official", Es: "oficial"},
	"cli.ptype.deepseek": {En: "DeepSeek (provider)", Es: "proveedor"},
	"cli.ptype.kimi":     {En: "Kimi (provider)", Es: "proveedor"},
	"cli.ptype.glm":      {En: "GLM (provider)", Es: "proveedor"},
	"cli.ptype.default":  {En: "default", Es: "default"},

	// --- status.go ---
	"cli.status.not_git": {
		En: "not git",
		Es: "no es git",
	},
	"cli.status.header": {
		En: "ccp status in this terminal",
		Es: "Estado de ccp en esta terminal",
	},
	"cli.status.active": {
		En: " Active profile (terminal): %s\n",
		Es: " Perfil activo (terminal): %s\n",
	},
	"cli.status.rule": {
		En: " Profile for cwd (rule):   %s  %s\n",
		Es: " Perfil del cwd (regla):   %s  %s\n",
	},
	"cli.status.cwd": {
		En: " Cwd:                      %s\n",
		Es: " Cwd:                      %s\n",
	},
	"cli.status.repo": {
		En: " Repo:                     %s\n",
		Es: " Repo:                     %s\n",
	},

	// --- doctor.go ---
	"cli.doctor.header": {
		En: "Diagnostics",
		Es: "Diagnóstico",
	},

	// --- key.go ---
	"cli.key.usage": {
		En: "Usage: ccp key <profile> [API_KEY]",
		Es: "Uso: ccp key <perfil> [API_KEY]",
	},
	"cli.key.prompt": {
		En: "Paste the API key for %s (hidden): ",
		Es: "Pega la API key de %s (oculta): ",
	},
	"cli.key.empty": {
		En: "[error] You did not enter a key.",
		Es: "[error] No ingresaste ninguna key.",
	},
	"cli.key.saved": {
		En: "API key saved for '%s' (600).",
		Es: "API key guardada para '%s' (600).",
	},

	// --- path.go ---
	"cli.path.usage_set": {
		En: "Usage: ccp path set <path> <profile>",
		Es: "Uso: ccp path set <ruta> <perfil>",
	},
	"cli.path.rule_set": {
		En: "RULE: %s  ->  %s  (and subfolders)",
		Es: "REGLA: %s  ->  %s  (y subcarpetas)",
	},
	"cli.path.usage_rm": {
		En: "Usage: ccp path rm <path>",
		Es: "Uso: ccp path rm <ruta>",
	},
	"cli.path.rule_removed": {
		En: "Rule removed: %s",
		Es: "Regla eliminada: %s",
	},
	"cli.path.cleared": {
		En: "All rules removed.",
		Es: "Todas las reglas eliminadas.",
	},
	"cli.path.unknown_sub": {
		En: "path: unknown subcommand '%s'",
		Es: "path: subcomando desconocido '%s'",
	},
	"cli.path.sub_help": {
		En: "Use: set | rm | list | test | clear | edit",
		Es: "Usa: set | rm | list | test | clear | edit",
	},
	"cli.path.list_header": {
		En: "PATH rules",
		Es: "Reglas de PATH",
	},
	"cli.path.list_empty": {
		En: "  (no rules — everything uses 'default')",
		Es: "  (sin reglas — todo usa 'default')",
	},
	"cli.path.effective": {
		En: "Effective rule for cwd:",
		Es: "Regla efectiva para el cwd:",
	},

	// --- install.go ---
	"cli.install.old_dsctl": {
		En: "Detected the old dsctl init in %s.",
		Es: "Detecté el init viejo de dsctl en %s.",
	},
	"cli.install.old_dsctl_hint": {
		En: "  Remove it with:  ccp uninstall   (or edit the '# >>> dsctl shell init >>>' block)",
		Es: "  Quítalo con:  ccp uninstall   (o edita el bloque '# >>> dsctl shell init >>>')",
	},
	"cli.install.already": {
		En: "ccp init is already in %s",
		Es: "El init de ccp ya está en %s",
	},
	"cli.install.reload": {
		En: "Reload with:  source %s",
		Es: "Recarga con:  source %s",
	},
	"cli.install.open_fail": {
		En: "[error] could not open %s: %v",
		Es: "[error] no se pudo abrir %s: %v",
	},
	"cli.install.added": {
		En: "Init added to %s",
		Es: "Init añadido a %s",
	},
	"cli.install.refreshed": {
		En: "ccp init refreshed in %s (shell function was out of date)",
		Es: "Init de ccp actualizado en %s (la función de shell estaba desfasada)",
	},
	"cli.uninstall.not_found": {
		En: "ccp init not found in %s",
		Es: "No encontré el init en %s",
	},
	"cli.uninstall.removed": {
		En: "ccp init removed from %s",
		Es: "Init de ccp removido de %s",
	},
	"cli.upgrade.usage": {
		En: "Usage: ccp upgrade [--pull] [--no-sync]",
		Es: "Uso: ccp upgrade [--pull] [--no-sync]",
	},
	"cli.upgrade.no_source": {
		En: "[error] No registered source. Run 'bash install.sh' from the repo once.",
		Es: "[error] No hay fuente registrada. Corre 'bash install.sh' desde el repo una vez.",
	},
	"cli.upgrade.bad_repo": {
		En: "[error] Invalid registered repo: %s (re-run install.sh).",
		Es: "[error] Repo registrado inválido: %s (re-corre install.sh).",
	},
	"cli.upgrade.git_missing": {
		En: "[error] git is not installed (--pull unavailable).",
		Es: "[error] git no está instalado (--pull no disponible).",
	},
	"cli.upgrade.git_pull": {
		En: "git pull in %s...",
		Es: "git pull en %s...",
	},
	"cli.upgrade.git_pull_fail": {
		En: "[error] git pull failed.",
		Es: "[error] git pull falló.",
	},
	"cli.upgrade.reinstalling": {
		En: "Reinstalling from %s...",
		Es: "Reinstalando desde %s...",
	},
	"cli.upgrade.install_fail": {
		En: "[error] install.sh failed.",
		Es: "[error] install.sh falló.",
	},
	"cli.upgrade.syncing": {
		En: "Syncing profiles (overlay migration + regen)...",
		Es: "Sincronizando perfiles (migración overlay + regen)...",
	},
	"cli.upgrade.sync_trouble": {
		En: "profile sync had issues; check with 'ccp doctor'.",
		Es: "profile sync tuvo problemas; revisa con 'ccp doctor'.",
	},
	"cli.upgrade.done": {
		En: "ccp updated.",
		Es: "ccp actualizado.",
	},
	"cli.upgrade.completions": {
		En: "New completions: open a new terminal or 'source' your rc.",
		Es: "Completions nuevos: abre una terminal nueva o 'source' tu rc.",
	},
	"cli.upgrade.stale_rc": {
		En: "ccp's shell-init changed in this version (your rc has the old one).",
		Es: "El shell-init de ccp cambió en esta versión (tu rc tiene el viejo).",
	},
	"cli.upgrade.stale_rc_hint": {
		En: "  Update it:  ccp uninstall && ccp install && source %s",
		Es: "  Actualízalo:  ccp uninstall && ccp install && source %s",
	},

	// --- instruct.go ---
	"cli.instruct.unknown_sub": {
		En: "instruct: unknown subcommand '%s' (add|list|rm|dest|record)",
		Es: "instruct: subcomando desconocido '%s' (add|list|rm|dest|record)",
	},
	"cli.instruct.usage_add": {
		En: "Usage: ccp instruct add <scope> <type> <text>",
		Es: "Uso: ccp instruct add <scope> <type> <texto>",
	},
	"cli.instruct.rule_dup": {
		En: "That instruction already existed in %s (not duplicated).",
		Es: "Ya existía esa instrucción en %s (no se duplica).",
	},
	"cli.instruct.rule_added": {
		En: "Instruction added (%s/rule) -> %s",
		Es: "Instrucción añadida (%s/rule) -> %s",
	},
	"cli.instruct.mcp_added": {
		En: "MCP '%s' added (%s) -> %s",
		Es: "MCP '%s' añadido (%s) -> %s",
	},
	"cli.instruct.hook_added": {
		En: "Hook '%s' added (%s) -> %s",
		Es: "Hook '%s' añadido (%s) -> %s",
	},
	"cli.instruct.hook_note": {
		En: "Note: hook removal is not automatic (they live in arrays without a stable id).",
		Es: "Aviso: el borrado de hooks no es automático (viven en arrays sin id estable).",
	},
	"cli.instruct.hook_rm_profile": {
		En: "  To remove it: 'ccp profile config %s settings' or edit the overlay by hand.",
		Es: "  Para quitarlo: 'ccp profile config %s settings' o edita el overlay a mano.",
	},
	"cli.instruct.hook_rm_other": {
		En: "  To remove it: edit %s by hand.",
		Es: "  Para quitarlo: edita %s a mano.",
	},
	"cli.instruct.usage_list": {
		En: "Usage: ccp instruct list <scope>",
		Es: "Uso: ccp instruct list <scope>",
	},
	"cli.instruct.list_empty": {
		En: "   (no instructions or artifacts in %s)",
		Es: "   (sin instrucciones ni artefactos en %s)",
	},
	"cli.instruct.usage_rm": {
		En: "Usage: ccp instruct rm <scope> <index>",
		Es: "Uso: ccp instruct rm <scope> <index>",
	},
	"cli.instruct.rm_rule": {
		En: "Instruction #%d removed (%s).",
		Es: "Instrucción #%d eliminada (%s).",
	},
	"cli.instruct.rm_hook_note": {
		En: "Note: hook '%s' was removed from the manifest, but its JSON entry is still in the file.",
		Es: "Aviso: el hook '%s' se quitó del manifiesto, pero su entrada JSON sigue en el archivo.",
	},
	"cli.instruct.rm_artifact": {
		En: "Artifact #%d removed from the manifest (%s/%s): %s",
		Es: "Artefacto #%d eliminado del manifiesto (%s/%s): %s",
	},
	"cli.instruct.usage_dest": {
		En: "Usage: ccp instruct dest <scope> <type>",
		Es: "Uso: ccp instruct dest <scope> <type>",
	},
	"cli.instruct.usage_record": {
		En: "Usage: ccp instruct record <scope> <type> <ref> <desc>",
		Es: "Uso: ccp instruct record <scope> <type> <ref> <desc>",
	},
	"cli.instruct.recorded": {
		En: "Artifact recorded (%s/%s): %s",
		Es: "Artefacto registrado (%s/%s): %s",
	},

	// --- config.go ---
	"cli.config.reset": {
		En: "Defaults restored.",
		Es: "Defaults restaurados.",
	},
	"cli.config.tpl_header": {
		En: "Template for new deepseek profiles",
		Es: "Plantilla para perfiles deepseek nuevos",
	},
	"cli.config.base_url": {
		En: " Base URL:    %s\n",
		Es: " Base URL:    %s\n",
	},
	"cli.config.model_pro": {
		En: " Model pro:   %s\n",
		Es: " Modelo pro:  %s\n",
	},
	"cli.config.model_flash": {
		En: " Model flash: %s\n",
		Es: " Modelo flash:%s\n",
	},
	"cli.config.effort": {
		En: " Effort:      %s\n",
		Es: " Effort:      %s\n",
	},
	"cli.config.editor": {
		En: " Editor:      %s\n",
		Es: " Editor:      %s\n",
	},
	"cli.config.usage_set": {
		En: "Usage: ccp config set <key> <value>",
		Es: "Uso: ccp config set <clave> <valor>",
	},
	"cli.config.set_ok": {
		En: "Config: %s = %s",
		Es: "Config: %s = %s",
	},
	"cli.config.editor_set": {
		En: "Editor: %s",
		Es: "Editor: %s",
	},
	"cli.config.usage": {
		En: "Usage: ccp config [show|set|reset|editor]",
		Es: "Uso: ccp config [show|set|reset|editor]",
	},

	// --- handoff.go ---
	"cli.handoff.shell_only": {
		En: "`ccp handoff` only works via the ccp shell function.\nRefresh it:  ccp install && source ~/.zshrc   (see `ccp help` → TROUBLESHOOTING)",
		Es: "`ccp handoff` solo funciona vía la función shell de ccp.\nRefréscala:  ccp install && source ~/.zshrc   (ve `ccp help` → SOLUCIÓN DE PROBLEMAS)",
	},
	"cli.handoff.no_active": {
		En: "No active handoff.",
		Es: "Sin handoff activo.",
	},
	"cli.handoff.status_active": {
		En: "Active handoff: %s → %s · session %s · since %s",
		Es: "Handoff activo: %s → %s · sesión %s · desde %s",
	},
	"cli.handoff.list_header": {
		En: "Handoff history:",
		Es: "Historial de handoffs:",
	},
	"cli.handoff.list_row": {
		En: "  %s → %s · %s → %s · ended %s",
		Es: "  %s → %s · %s → %s · terminó %s",
	},
	"cli.handoff.list_empty": {
		En: "No handoffs recorded.",
		Es: "Sin handoffs registrados.",
	},
	"cli.handoff.unknown_sub": {
		En: "handoff: unknown subcommand: %s",
		Es: "handoff: subcomando desconocido: %s",
	},

	// --- help.go ---
	"cli.help.tagline": {
		En: "ccp v%s — profiles for Claude Code\n\n",
		Es: "ccp v%s — perfiles para Claude Code\n\n",
	},
	"cli.help.logo_tagline": {
		En: "profiles for Claude Code",
		Es: "perfiles para Claude Code",
	},
	"cli.help.body": {
		En: `TERMINAL (shell function)
  ccp use <profile>           activate a profile in this terminal
  ccp default | off           go back to your ~/.claude login
  ccp run [cmd]               run cmd/claude with the cwd's profile

HANDOFF (shell function)
  ccp handoff [<to>]          continue this session under another profile (TUI pickers)
  ccp handoff <to> --session <uuid>   skip the pickers (scriptable)
  ccp handoff end             bring the updated context back to the origin
  ccp handoff status | list   in-flight handoff + history

PROFILES
  ccp profile add <n> --official            create official account
  ccp profile add <n> --deepseek [opts]     create DeepSeek provider (--base-url --pro --flash --effort)
  ccp profile add <n> --kimi [opts]         create Kimi (Moonshot) provider
  ccp profile add <n> --glm [opts]          create GLM (Z.ai) provider
  ccp profile login <n>                     /login (official profiles)
  ccp profile config <n>                    edit the profile's config
  ccp profile sync [<n>]                    re-merge the global into the cc-home(s)
  ccp profile list | show <n> | rm <n>
  ccp key <profile> [API_KEY]               store the key of a deepseek profile

PATH RULES
  ccp path set <path> <profile>  assign path (and subfolders) to a profile
  ccp path rm <path>             remove the rule
  ccp path list | test <path> | clear | edit

INSTRUCTIONS
  ccp instruct add|list|rm <scope> ...      Claude memory (used by /ccp:*)

BACKUP
  ccp backup export [file] [--with-secrets]
  ccp backup restore <file> [--overwrite | --force]

SCRIPTING
  ccp resolve [path]          print the path's profile (exit 0=rule, 1=default)
  ccp status [--json]         terminal status
  ccp completion bash|zsh     autocompletion scripts

LIFE CYCLE
  ccp install | uninstall     add/remove the shell-init block from the rc
  ccp upgrade [--pull]        reinstall from the registered source + sync
  ccp doctor                  diagnostics
  ccp config [show|set|reset|editor]

TROUBLESHOOTING
  "ccp handoff only works via the ccp shell function"
    your rc has an out-of-date shell function. Refresh it:
      ccp install          (rewrites the block in place if it drifted)
      source ~/.zshrc      (or ~/.bashrc, or open a new terminal)
  'use'/'handoff'/'run' do nothing, or the profile won't switch on cd
    the shell function isn't loaded in this terminal. Same fix:
      ccp install && source ~/.zshrc
  still stuck? run 'ccp doctor' and check 'ccp status'
`,
		Es: `TERMINAL (función shell)
  ccp use <perfil>            activa un perfil en esta terminal
  ccp default | off           vuelve a tu login ~/.claude
  ccp run [cmd]               corre cmd/claude con el perfil del cwd

HANDOFF (función shell)
  ccp handoff [<destino>]     continúa esta sesión bajo otro perfil (pickers TUI)
  ccp handoff <destino> --session <uuid>   salta los pickers (scriptable)
  ccp handoff end             trae el contexto actualizado de vuelta al origen
  ccp handoff status | list   handoff en vuelo + historial

PERFILES
  ccp profile add <n> --official            crea cuenta oficial
  ccp profile add <n> --deepseek [opts]     crea provider DeepSeek (--base-url --pro --flash --effort)
  ccp profile add <n> --kimi [opts]         crea provider Kimi (Moonshot)
  ccp profile add <n> --glm [opts]          crea provider GLM (Z.ai)
  ccp profile login <n>                     /login (perfiles oficiales)
  ccp profile config <n>                    edita la config del perfil
  ccp profile sync [<n>]                    re-mergea el global en el/los cc-home
  ccp profile list | show <n> | rm <n>
  ccp key <perfil> [API_KEY]                guarda la key de un perfil deepseek

REGLAS DE PATH
  ccp path set <ruta> <perfil>   asigna ruta (y subcarpetas) a un perfil
  ccp path rm <ruta>             quita la regla
  ccp path list | test <ruta> | clear | edit

INSTRUCCIONES
  ccp instruct add|list|rm <scope> ...      memoria de Claude (lo usan /ccp:*)

BACKUP
  ccp backup export [archivo] [--with-secrets]
  ccp backup restore <archivo> [--overwrite | --force]

SCRIPTING
  ccp resolve [ruta]          imprime el perfil del path (exit 0=regla, 1=default)
  ccp status [--json]         estado de la terminal
  ccp completion bash|zsh     scripts de autocompletado

CICLO DE VIDA
  ccp install | uninstall     añade/quita el bloque shell-init del rc
  ccp upgrade [--pull]        reinstala desde la fuente registrada + sync
  ccp doctor                  diagnóstico
  ccp config [show|set|reset|editor]

SOLUCIÓN DE PROBLEMAS
  "ccp handoff only works via the ccp shell function"
    tu rc tiene una función shell desfasada. Refréscala:
      ccp install          (reescribe el bloque en sitio si quedó viejo)
      source ~/.zshrc      (o ~/.bashrc, o abre una terminal nueva)
  'use'/'handoff'/'run' no hacen nada, o el perfil no cambia al hacer cd
    la función shell no está cargada en esta terminal. Mismo arreglo:
      ccp install && source ~/.zshrc
  ¿sigue fallando? corre 'ccp doctor' y revisa 'ccp status'
`,
	},
}
