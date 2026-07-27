package i18n

func init() {
	register(catalogAuto)
}

// catalogAuto agrupa la prosa de `ccp auto` (init/install/uninstall/status/test)
// y de los dos comandos internos que alimentan los sensores (`_statusline`,
// `_limit-hook`). Keys namespaced cli.auto.* — register panica ante duplicados,
// así que ningún otro catálogo puede usar ese prefijo.
var catalogAuto = map[string]map[Lang]string{
	// --- dispatch / errores de uso ---
	"cli.auto.usage": {
		En: `Usage: ccp auto <subcommand>

  init [--force]           seed the auto_handoff block in ccp.yaml
  install [<profile>...]   install the sensor layer (hooks + statusLine) and regenerate
  uninstall [<profile>...] remove it and regenerate
  status [--json]          resolved policy, installed sensors, last samples, cooldowns
  test [--profile <n>]     inject a synthetic StopFailure and check the detection path`,
		Es: `Uso: ccp auto <subcomando>

  init [--force]           siembra el bloque auto_handoff en ccp.yaml
  install [<perfil>...]    instala la capa de sensores (hooks + statusLine) y regenera
  uninstall [<perfil>...]  la quita y regenera
  status [--json]          política resuelta, sensores instalados, últimas muestras, cooldowns
  test [--profile <n>]     inyecta un StopFailure sintético y comprueba la ruta de detección`,
	},
	"cli.auto.unknown_sub": {
		En: "ccp auto: unknown subcommand %q",
		Es: "ccp auto: subcomando desconocido %q",
	},
	"cli.auto.unknown_flag": {
		En: "ccp auto: unknown flag %q",
		Es: "ccp auto: opción desconocida %q",
	},
	"cli.auto.flag_needs_value": {
		En: "ccp auto: %s needs a value",
		Es: "ccp auto: %s necesita un valor",
	},

	// --- init ---
	"cli.auto.init_exists": {
		En: "auto_handoff already exists in ccp.yaml; use `ccp auto init --force` to reseed it (manual tweaks will be lost)",
		Es: "auto_handoff ya existe en ccp.yaml; usa `ccp auto init --force` para resembrarlo (se perderán los ajustes manuales)",
	},
	"cli.auto.init_done": {
		En: "auto_handoff seeded in ccp.yaml (policy 'default', %d profile(s) in the chain)",
		Es: "auto_handoff sembrado en ccp.yaml (política 'default', %d perfil(es) en la cadena)",
	},
	"cli.auto.init_next": {
		En: "next: `ccp auto install` to install the sensors in each profile",
		Es: "siguiente: `ccp auto install` para instalar los sensores en cada perfil",
	},

	// --- install / uninstall ---
	"cli.auto.not_configured": {
		En: "auto_handoff is not configured in ccp.yaml; run `ccp auto init` first",
		Es: "auto_handoff no está configurado en ccp.yaml; corre antes `ccp auto init`",
	},
	"cli.auto.no_profiles": {
		En: "there are no non-default profiles to install the sensors into",
		Es: "no hay perfiles no-default en los que instalar los sensores",
	},
	"cli.auto.reject_default": {
		En: "'default' is your global ~/.claude: ccp does not regenerate its settings.json, so the sensor layer cannot be managed there",
		Es: "'default' es tu ~/.claude global: ccp no regenera su settings.json, así que la capa de sensores no se puede gestionar ahí",
	},
	"cli.auto.unknown_profile": {
		En: "profile %q does not exist",
		Es: "el perfil %q no existe",
	},
	"cli.auto.installed": {
		En: "%s: sensors installed (StopFailure + statusLine)",
		Es: "%s: sensores instalados (StopFailure + statusLine)",
	},
	"cli.auto.already_installed": {
		En: "%s: already had the sensors (settings.json regenerated)",
		Es: "%s: ya tenía los sensores (settings.json regenerado)",
	},
	"cli.auto.uninstalled": {
		En: "%s: sensors removed (settings.json back to global ⊕ overlay)",
		Es: "%s: sensores quitados (settings.json vuelve a global ⊕ overlay)",
	},
	"cli.auto.not_installed": {
		En: "%s: did not have the sensors (nothing to remove)",
		Es: "%s: no tenía los sensores (nada que quitar)",
	},
	"cli.auto.regen_failed": {
		En: "%s: could not regenerate cc-home: %v",
		Es: "%s: no se pudo regenerar el cc-home: %v",
	},

	// --- status ---
	"cli.auto.status_header": {
		En: "auto-handoff",
		Es: "auto-handoff",
	},
	"cli.auto.status_disabled": {
		En: "auto_handoff present but disabled (enabled: false)",
		Es: "auto_handoff presente pero deshabilitado (enabled: false)",
	},
	// Estado DISTINTO del anterior: el bloque no existe. Decir «presente pero
	// deshabilitado» mandaría al usuario a buscar un `enabled: false` que no está.
	"cli.auto.status_not_configured": {
		En: "auto_handoff is not in ccp.yaml yet; run `ccp auto init`",
		Es: "auto_handoff todavía no está en ccp.yaml; corre `ccp auto init`",
	},
	"cli.auto.status_policy": {
		En: "policy      %s",
		Es: "política    %s",
	},
	"cli.auto.status_primary": {
		En: "primary     %s  (cwd %s)",
		Es: "primario    %s  (cwd %s)",
	},
	"cli.auto.status_fallback": {
		En: "chain       %s",
		Es: "cadena      %s",
	},
	"cli.auto.status_denied": {
		En: "denied      %s  (allow_from)",
		Es: "denegados   %s  (allow_from)",
	},
	"cli.auto.status_none": {
		En: "(none)",
		Es: "(ninguno)",
	},
	"cli.auto.status_limits": {
		En: "thresholds  %d%% used · min_dwell %s · max_hops %d · return_check %s",
		Es: "umbrales    %d%% de uso · min_dwell %s · max_hops %d · return_check %s",
	},
	"cli.auto.status_return_idle": {
		En: "return_idle %s of silence before the proactive trip home",
		Es: "return_idle %s de silencio antes de volver a casa por reloj",
	},
	"cli.auto.status_cooldown": {
		En: "cooldown    %s (fallback %s)",
		Es: "cooldown    %s (respaldo %s)",
	},
	"cli.auto.status_sensors": {
		En: "sensors",
		Es: "sensores",
	},
	"cli.auto.status_sensor_on": {
		En: "installed",
		Es: "instalados",
	},
	"cli.auto.status_sensor_off": {
		En: "not installed",
		Es: "sin instalar",
	},
	"cli.auto.status_sample": {
		En: "5h %.0f%% · 7d %.0f%% · sampled %s ago",
		Es: "5h %.0f%% · 7d %.0f%% · muestra de hace %s",
	},
	"cli.auto.status_no_sample": {
		En: "no sample yet",
		Es: "aún sin muestra",
	},
	"cli.auto.status_cooldown_until": {
		En: "over threshold, free at %s",
		Es: "sobre el umbral, libre a las %s",
	},

	// --- test ---
	"cli.auto.test_header": {
		En: "auto-handoff detection test · profile %s",
		Es: "prueba de detección de auto-handoff · perfil %s",
	},
	"cli.auto.test_layer_ok": {
		En: "sensor layer installed in %s",
		Es: "capa de sensores instalada en %s",
	},
	"cli.auto.test_layer_missing": {
		En: "sensor layer NOT installed in %s (run `ccp auto install %s`); the synthetic test goes on anyway",
		Es: "capa de sensores NO instalada en %s (corre `ccp auto install %s`); la prueba sintética sigue igual",
	},
	"cli.auto.test_hook_ok": {
		En: "synthetic StopFailure went through `ccp _limit-hook` (exit 0)",
		Es: "el StopFailure sintético pasó por `ccp _limit-hook` (exit 0)",
	},
	"cli.auto.test_hook_fail": {
		En: "`ccp _limit-hook` exited %d — it must ALWAYS exit 0",
		Es: "`ccp _limit-hook` salió %d — debe salir SIEMPRE 0",
	},
	"cli.auto.test_sentinel_ok": {
		En: "sentinel written and read back (window %s)",
		Es: "sentinel escrito y releído (ventana %s)",
	},
	"cli.auto.test_sentinel_missing": {
		En: "no sentinel appeared in %s — the supervisor would be blind to this signal",
		Es: "no apareció ningún sentinel en %s — el supervisor estaría ciego a esta señal",
	},
	"cli.auto.test_cleanup_ok": {
		En: "test sentinels cleaned up",
		Es: "sentinels de prueba limpiados",
	},
	"cli.auto.test_cleanup_fail": {
		En: "could not clean up the test sentinels: %v",
		Es: "no se pudieron limpiar los sentinels de prueba: %v",
	},
	"cli.auto.test_pass": {
		En: "detection path OK",
		Es: "ruta de detección OK",
	},
	"cli.auto.test_fail": {
		En: "detection path BROKEN — `ccp session` would not react to a rate limit",
		Es: "ruta de detección ROTA — `ccp session` no reaccionaría a un rate limit",
	},

	// --- _statusline ---
	// Formato de la línea propia (perfil · uso). Es idéntica en ambos idiomas a
	// propósito: son datos, no prosa, y la barra de estado de CC es estrecha.
	"cli.auto.statusline_usage": {
		En: "%s · %s",
		Es: "%s · %s",
	},
	// La variante ancha une con doble espacio en vez de con `·`: cuando cada
	// ventana ya trae su medidor delimitado, el punto medio sobra y solo suma
	// ruido a una línea que de por sí lleva mucho glifo.
	"cli.auto.statusline_usage_wide": {
		En: "%s  %s",
		Es: "%s  %s",
	},
}
