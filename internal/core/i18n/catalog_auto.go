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
  test [--profile <n>]     inject a synthetic StopFailure and check the detection path
  chain [show|add|rm|mv|set]  read and edit the loan chain (chain help for details)`,
		Es: `Uso: ccp auto <subcomando>

  init [--force]           siembra el bloque auto_handoff en ccp.yaml
  install [<perfil>...]    instala la capa de sensores (hooks + statusLine) y regenera
  uninstall [<perfil>...]  la quita y regenera
  status [--json]          política resuelta, sensores instalados, últimas muestras, cooldowns
  test [--profile <n>]     inyecta un StopFailure sintético y comprueba la ruta de detección
  chain [show|add|rm|mv|set]  lee y edita la cadena de préstamos (chain help para el detalle)`,
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

	// --- chain ---
	//
	// Las etiquetas de la izquierda se alinean a 11 columnas (fallback/allow_from)
	// para que las dos claves del yaml que este comando toca se lean como dos
	// filas de la misma tabla, no como dos frases sueltas.
	"cli.auto.chain_usage": {
		En: `Usage: ccp auto chain <subcommand> [--for <profile> | --shared | --policy <name>]

  show                     effective chain (after the allow_from gate)
  list [--json]            every profile's chain: own or inherited
  add <profile>... [--at N] [--no-allow]
                           append to the chain (or insert at 1-based position N)
                           and authorise the loan
  rm <profile>...          take profiles out of the chain
  mv <profile> <pos>       move a profile to the 1-based position pos
  set <a,b,c>              replace the whole chain (order = preference)
  reset                    drop the profile's own chain: inherit again
  policy <name>|--none     pin a policy to this profile (thresholds, cooldown)

WHICH chain. By default every subcommand works on the OWN chain of the cwd's
primary — one chain per profile, which is the point. --for <profile> targets
another profile without cd'ing there; --shared (or --policy <name>) targets the
policy's shared list, which every profile with no chain of its own inherits.

The order IS the preference: the supervisor walks the chain top to bottom.
'add' also touches allow_from[<profile>] —and only that entry— because a profile
in the chain without its allow_from is never used, silently. --no-allow turns
that off.

The first mutation on an inherited chain FORKS it: the profile gets its own copy
and stops tracking the shared list. It is reported when it happens, and 'reset'
undoes it.`,
		Es: `Uso: ccp auto chain <subcomando> [--for <perfil> | --shared | --policy <nombre>]

  show                     cadena efectiva (tras el gate allow_from)
  list [--json]            la cadena de cada perfil: propia o heredada
  add <perfil>... [--at N] [--no-allow]
                           añade al final de la cadena (o en la posición N, 1-based)
                           y autoriza el préstamo
  rm <perfil>...           saca perfiles de la cadena
  mv <perfil> <pos>        mueve un perfil a la posición pos (1-based)
  set <a,b,c>              reemplaza la cadena entera (el orden = la preferencia)
  reset                    quita la cadena propia del perfil: vuelve a heredar
  policy <nombre>|--none   liga una política a este perfil (umbral, cooldown)

QUÉ cadena. Por defecto todos los subcomandos trabajan sobre la cadena PROPIA
del primario del cwd — una cadena por perfil, que es de lo que se trata.
--for <perfil> apunta a otro perfil sin cambiar de carpeta; --shared (o
--policy <nombre>) apunta a la lista compartida de la política, que es la que
hereda todo perfil sin cadena propia.

El orden ES la preferencia: el supervisor recorre la cadena de arriba abajo.
'add' toca además allow_from[<perfil>] —y solo esa entrada— porque un perfil
en la cadena sin su allow_from no se usa nunca, en silencio. --no-allow lo apaga.

La primera mutación sobre una cadena heredada la BIFURCA: el perfil se queda con
su propia copia y deja de seguir la lista compartida. Se avisa cuando pasa, y
'reset' lo deshace.`,
	},
	// --- destino de la mutación: qué clave del yaml se escribió ---
	//
	// Estas líneas son la mitad del valor del comando desde que hay más de una
	// cadena. Sin ellas, «fallback: a, b» vale igual para «le cambié la cadena a
	// este repo» que para «se la cambié a todos», y son cosas muy distintas.
	"cli.auto.chain_target": {
		En: "target     %s",
		Es: "destino    %s",
	},
	"cli.auto.chain_target_own": {
		En: "own chain of %s",
		Es: "cadena propia de %s",
	},
	"cli.auto.chain_target_shared": {
		En: "shared list of policy %s (every profile with no chain of its own)",
		Es: "lista compartida de la política %s (la que hereda todo perfil sin cadena propia)",
	},
	// La bifurcación se avisa porque es la consecuencia que nadie pidió: a partir
	// de aquí los cambios de la lista compartida dejan de llegarle a ese perfil.
	"cli.auto.chain_forked": {
		En: "%s now has a chain of its own: changes to policy %s's list will no longer reach it (it inherited: %s)",
		Es: "%s pasa a tener cadena propia: los cambios de la lista de la política %s ya no le llegarán (heredaba: %s)",
	},
	"cli.auto.chain_reset_done": {
		En: "reset      %s inherits the chain of policy %s again",
		Es: "reset      %s vuelve a heredar la cadena de la política %s",
	},
	"cli.auto.chain_policy_bound": {
		En: "policy     %s now uses policy %s (thresholds, dwell, cooldown)",
		Es: "policy     %s pasa a usar la política %s (umbral, permanencia, cooldown)",
	},
	"cli.auto.chain_policy_cleared": {
		En: "policy     %s no longer pins a policy: back to %s",
		Es: "policy     %s deja de ligar política: vuelve a %s",
	},
	"cli.auto.chain_policy_pinned": {
		En: "(pinned to %s)",
		Es: "(ligada a %s)",
	},
	// De dónde sale la cadena. Sin esto, «cadena: (ninguna)» no distingue «este
	// perfil no presta a nadie» de «la lista compartida está vacía», y el arreglo
	// de cada una está en un sitio distinto del yaml.
	// «origen» y no «cadena»: la línea de al lado ya se llama así (status_fallback
	// = "cadena      %s"), y dos líneas que empiezan igual con contenidos
	// distintos se leen mal a la primera y se parsean mal siempre.
	"cli.auto.chain_src_own": {
		En: "source      %s's own chain",
		Es: "origen      cadena propia de %s",
	},
	"cli.auto.chain_src_inherited": {
		En: "source      inherited from policy %s",
		Es: "origen      heredada de la política %s",
	},
	// Alineada a 12 columnas como sus vecinas (política/origen/cadena): esta
	// línea se lee en bloque con ellas y una etiqueta descuadrada delata que se
	// añadió después.
	"cli.auto.chain_show_profile": {
		En: "profile     %s",
		Es: "perfil      %s",
	},
	"cli.auto.chain_list_own": {
		En: "own",
		Es: "propia",
	},
	"cli.auto.chain_list_inherited": {
		En: "inherited",
		Es: "heredada",
	},
	"cli.auto.chain_list_policy": {
		En: "· policy %s",
		Es: "· política %s",
	},
	"cli.auto.chain_list_missing": {
		En: "profiles that no longer exist: %s",
		Es: "perfiles que ya no existen: %s",
	},
	"cli.auto.chain_list_orphan": {
		En: "%s has a chain but is no longer a profile: `ccp auto chain reset --for %[1]s`",
		Es: "%s tiene cadena pero ya no es un perfil: `ccp auto chain reset --for %[1]s`",
	},
	"cli.auto.chain_note_self": {
		En: "%s is the owner of this chain: nobody lends to themselves, so it does nothing",
		Es: "%s es el dueño de esta cadena: nadie se presta a sí mismo, así que no hace nada",
	},
	"cli.auto.chain_reset_usage": {
		En: "ccp auto chain reset takes no arguments (use --for <profile> to target another one)",
		Es: "ccp auto chain reset no lleva argumentos (usa --for <perfil> para apuntar a otro)",
	},
	"cli.auto.chain_policy_usage": {
		En: "ccp auto chain policy needs exactly one argument: <policy name> or --none",
		Es: "ccp auto chain policy necesita exactamente un argumento: <nombre de política> o --none",
	},
	"cli.auto.chain_err_chain_profile": {
		En: "%s's own chain points at profile %q, which does not exist",
		Es: "la cadena propia de %s apunta al perfil %q, que no existe",
	},
	"cli.auto.chain_err_chain_policy": {
		En: "%s pins policy %q, which does not exist (there are: %s)",
		Es: "%s liga la política %q, que no existe (hay: %s)",
	},
	"cli.auto.chain_err_no_chain": {
		En: "%s has no chain of its own: it already inherits policy %s's",
		Es: "%s no tiene cadena propia: ya hereda la de la política %s",
	},
	"cli.auto.chain_err_target_clash": {
		En: "--for and --shared/--policy name different targets: pick one",
		Es: "--for y --shared/--policy nombran destinos distintos: elige uno",
	},
	"cli.auto.chain_err_duplicate_own": {
		En: "%q is already in %s's chain",
		Es: "%q ya está en la cadena de %s",
	},
	"cli.auto.chain_err_not_in_chain_own": {
		En: "%q is not in %s's chain (there is: %s)",
		Es: "%q no está en la cadena de %s (hay: %s)",
	},
	// El estado que faltaba en el sensor: corre, pero el payload del statusLine
	// de Claude Code no trae el consumo. Decir «sin muestra» ahí mandaba a
	// reinstalar un sensor que ya estaba puesto y funcionando.
	"cli.auto.status_cc_silent": {
		En: "sensor runs; Claude Code %s does not report usage",
		Es: "el sensor corre; Claude Code %s no informa del consumo",
	},
	"cli.auto.chain_unknown_sub": {
		En: "ccp auto chain: unknown subcommand %q",
		Es: "ccp auto chain: subcomando desconocido %q",
	},
	"cli.auto.chain_mv_usage": {
		En: "ccp auto chain mv needs exactly two arguments: <profile> <position>",
		Es: "ccp auto chain mv necesita exactamente dos argumentos: <perfil> <posición>",
	},
	"cli.auto.chain_bad_pos": {
		En: "%q is not a valid position: use an integer >= 1 (positions are 1-based)",
		Es: "%q no es una posición válida: usa un entero >= 1 (las posiciones son 1-based)",
	},
	// Una bandera que no aplica al subcomando se RECHAZA en vez de ignorarse: el
	// usuario que escribe `--at 3 rm x` cree haber pedido algo concreto.
	"cli.auto.chain_flag_only": {
		En: "ccp auto chain: %s only applies to `%s`",
		Es: "ccp auto chain: %s solo aplica a `%s`",
	},
	"cli.auto.chain_show_extra": {
		En: "ccp auto chain show takes no arguments (leftover: %s) — did you mean `ccp auto chain add`?",
		Es: "ccp auto chain show no lleva argumentos (sobra: %s) — ¿querías `ccp auto chain add`?",
	},

	// Líneas del parte de una mutación. Cada clave del yaml tiene la suya: quien
	// lee la salida tiene que poder decir qué cambió en `fallback` y qué cambió en
	// `allow_from` sin abrir el archivo.
	"cli.auto.chain_fallback": {
		En: "fallback   %s",
		Es: "fallback   %s",
	},
	"cli.auto.chain_removed": {
		En: "removed    %s",
		Es: "quitados   %s",
	},
	"cli.auto.chain_moved": {
		En: "moved      %s to position %d",
		Es: "movido     %s a la posición %d",
	},
	"cli.auto.chain_allow_added": {
		En: "allow_from %s: %s",
		Es: "allow_from %s: %s",
	},
	// El estado que faltaba: gate declarado y esta mutación no lo movió (mv, o un
	// rm de algo que no estaba autorizado). Antes se pintaba como SILENCIO, que se
	// lee igual que «no me he fijado» — y con `set` significaba que la cadena podía
	// quedarse sin ningún destino autorizado sin una sola línea que lo dijera.
	"cli.auto.chain_allow_unchanged": {
		En: "allow_from %s: unchanged",
		Es: "allow_from %s: sin cambios",
	},
	// Estado 3 del gate: estaba declarado SIN entrada para este primario, o sea
	// deny total. Se dice que la entrada es NUEVA porque el efecto no es «uno más»
	// sino «se abre el primero».
	"cli.auto.chain_allow_created": {
		En: "allow_from %s: new entry %s (it had none: everything was denied)",
		Es: "allow_from %s: entrada nueva %s (no tenía: todo estaba denegado)",
	},
	// Estado 1: no hay gate. No se toca nada, y se explica por qué para que no
	// parezca que se olvidó.
	"cli.auto.chain_allow_nogate": {
		En: "allow_from untouched: no gate is declared, so nothing was blocking the loan",
		Es: "allow_from sin tocar: no hay gate declarado, así que nada bloqueaba el préstamo",
	},
	"cli.auto.chain_allow_skipped": {
		En: "allow_from untouched (--no-allow): the loan from %s may still be denied",
		Es: "allow_from sin tocar (--no-allow): el préstamo desde %s puede seguir denegado",
	},
	"cli.auto.chain_note_primary": {
		En: "%s is the primary for this cwd: it stays in the chain, but it is implicit and never a loan to itself",
		Es: "%s es el primario de este cwd: se queda en la cadena, pero es implícito y nunca se presta a sí mismo",
	},
	// Sin esta línea, un `add` que solo abre el gate imprimiría una cadena idéntica
	// a la anterior y parecería un no-op.
	"cli.auto.chain_note_already": {
		En: "%s was already in the chain: the chain did not change, only the allow_from gate opened",
		Es: "%s ya estaba en la cadena: la cadena no cambió, solo se abrió el gate de allow_from",
	},
	"cli.auto.chain_effective": {
		En: "effective chain from this repo:",
		Es: "cadena efectiva desde este repo:",
	},
	// La variante con nombre existe por `--for`: cerrar la operación con «desde
	// este repo» después de haberle tocado la cadena a OTRO perfil enseña una
	// lista que no tiene nada que ver con lo que se acaba de escribir.
	"cli.auto.chain_effective_for": {
		En: "effective chain for %s:",
		Es: "cadena efectiva de %s:",
	},
	"cli.auto.chain_target_profile": {
		En: "profile %s",
		Es: "perfil %s",
	},
	"cli.auto.chain_denied": {
		En: "denied by allow_from: %s",
		Es: "denegados por allow_from: %s",
	},
	"cli.auto.chain_effective_unavailable": {
		En: "saved, but the effective chain cannot be resolved: %v",
		Es: "guardado, pero la cadena efectiva no se puede resolver: %v",
	},

	// Errores tipados que devuelve core/auto_chain.go.
	"cli.auto.chain_err_no_policy": {
		En: "policy %q does not exist in auto_handoff.policies (there are: %s)",
		Es: "la política %q no existe en auto_handoff.policies (hay: %s)",
	},
	"cli.auto.chain_err_duplicate": {
		En: "%q is already in the chain of policy %q",
		Es: "%q ya está en la cadena de la política %q",
	},
	"cli.auto.chain_err_not_in_chain": {
		En: "%q is not in the chain of policy %q (there are: %s)",
		Es: "%q no está en la cadena de la política %q (hay: %s)",
	},
	"cli.auto.chain_err_range": {
		En: "position %d is out of range (valid: %d..%d)",
		Es: "la posición %d está fuera de rango (válido: %d..%d)",
	},
	"cli.auto.chain_err_empty": {
		En: "name at least one profile",
		Es: "nombra al menos un perfil",
	},

	// Errores de VALIDACIÓN de la política. Los devuelve el mismo *ChainError, así
	// que salen traducidos vengan de `auto chain show`, de `auto status --json`,
	// de `ccp session` o de la revalidación de `ccp config edit`. Antes cada una
	// de esas superficies reenviaba la prosa castellana del core.
	"cli.auto.chain_err_disabled": {
		En: "auto_handoff is disabled (enabled: false in ccp.yaml); set it to true or run `ccp auto init --force`",
		Es: "auto_handoff está deshabilitado (enabled: false en ccp.yaml); ponlo en true o corre `ccp auto init --force`",
	},
	"cli.auto.chain_err_fallback_profile": {
		En: "policy %q: fallback profile %q does not exist",
		Es: "política %q: el perfil de fallback %q no existe",
	},
	"cli.auto.chain_err_threshold": {
		En: "policy %q: threshold %d is out of range (1..100)",
		Es: "política %q: threshold %d fuera de rango (1..100)",
	},
	"cli.auto.chain_err_max_hops": {
		En: "policy %q: max_hops %d cannot be negative",
		Es: "política %q: max_hops %d no puede ser negativo",
	},
	// El %v final es el error de time.ParseDuration: es de la stdlib y no se
	// traduce, pero la coordenada (política + clave + valor) sí, que es lo que el
	// usuario necesita para encontrar la línea del yaml.
	"cli.auto.chain_err_duration": {
		En: "policy %q: invalid %s (%q): %v",
		Es: "política %q: %s inválido (%q): %v",
	},
	"cli.auto.chain_err_duration_neg": {
		En: "policy %q: invalid %s (%q): it cannot be negative",
		Es: "política %q: %s inválido (%q): no puede ser negativo",
	},
	"cli.auto.chain_err_cooldown": {
		En: "policy %q: unknown cooldown.strategy %q (use %q or %q)",
		Es: "política %q: cooldown.strategy %q desconocida (usa %q o %q)",
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
