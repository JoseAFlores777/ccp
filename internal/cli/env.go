package cli

import (
	"fmt"
	"io"

	"github.com/JoseAFlores777/ccp/internal/core"
	"github.com/JoseAFlores777/ccp/internal/core/i18n"
)

// env.go cablea la frontera binario↔shell: los comandos cuya salida el shell
// hace `eval` (_env, _hook) y el `resolve` scriptable. Son el contrato
// congelado — su salida debe coincidir byte-a-byte con el oráculo bash.

// safeDefaultDelta es el delta de entorno que emitimos cuando no se puede
// cargar la config (migración/lectura fallida) en un comando eval-able: limpia
// las managed vars y marca default, de modo que el shell nunca quede roto.
func safeDefaultDelta() string {
	return "unset " + core.CCPManagedVars + "\nexport CCP_PROFILE=default\n"
}

// cmdResolve imprime el perfil que aplica al path (o al cwd) y fija el exit
// code: 0 = una regla no-default ganó, 1 = default. Espeja cmd_resolve.
func cmdResolve(args []string, stdout, stderr io.Writer) int {
	home := resolveHome()
	cfg, err := loadCfg(home)
	if err != nil {
		fmt.Fprintf(stderr, "[error] %v\n", err)
		return 1
	}
	query := currentDir()
	if len(args) > 0 && args[0] != "" {
		query = args[0]
	}
	prof := core.Resolve(query, cfg.Rules)
	fmt.Fprintln(stdout, prof)
	if prof == "default" {
		return 1
	}
	return 0
}

// cmdEnv emite el delta de entorno (eval-able) de un perfil nombrado. Espeja
// cmd_env: `_env [perfil]`, default "default".
func cmdEnv(args []string, stdout, stderr io.Writer) int {
	home := resolveHome()
	profile := "default"
	if len(args) > 0 && args[0] != "" {
		profile = args[0]
	}
	cfg, err := loadCfg(home)
	if err != nil {
		warnConfigFallback(stderr, err)
		io.WriteString(stdout, safeDefaultDelta())
		return 0
	}
	io.WriteString(stdout, core.EnvDelta(home, profile, cfg))
	return 0
}

// cmdHook resuelve el perfil del path (o cwd) y emite su delta en un solo fork.
// Espeja cmd_hook: lo llama el hook _ccp_autocheck en cada cambio de PWD.
func cmdHook(args []string, stdout, stderr io.Writer) int {
	home := resolveHome()
	cfg, err := loadCfg(home)
	if err != nil {
		warnConfigFallback(stderr, err)
		io.WriteString(stdout, safeDefaultDelta())
		return 0
	}
	query := currentDir()
	if len(args) > 0 && args[0] != "" {
		query = args[0]
	}
	prof := core.Resolve(query, cfg.Rules)
	io.WriteString(stdout, core.EnvDelta(home, prof, cfg))
	// cfg ya está cargado: el idioma sale de ahí en vez de currentLang(), que
	// releería ccp.yaml. El hook corre en cada `cd`, así que el fork ha de ser
	// lo más barato posible.
	io.WriteString(stdout, handoffHookNotice(home, query, i18n.Resolve(cfg.Lang)))
	return 0
}

// handoffHookNotice devuelve la línea `echo … >&2` que recuerda los handoffs
// vivos de este proyecto, o "" si no hay. Va en el emit (no a os.Stderr) porque
// el hook se evalúa: es la shell la que imprime. _ccp_autocheck cachea por
// $PWD, así que aparece una vez por `cd`, no en cada prompt.
//
// El aviso se emite DESPUÉS del EnvDelta a propósito: si el delta tuviera un
// problema de parseo el eval no aplicaría nada, y un echo previo daría la
// impresión contraria.
//
// El escapado (core.QuoteInDoubleQuotes) se aplica solo a los DATOS (nombres de
// perfil, fechas del marcador), nunca a la plantilla i18n: esa lleva sus
// backticks ya escapados a mano. Sin él, un perfil llamado `x$(cmd)` ejecutaría
// cmd en cada `cd` del usuario. La implementación vive en core y es única para
// todo el repo: dos copias de la defensa anti-inyección divergen.
func handoffHookNotice(home, cwd string, lang i18n.Lang) string {
	h, err := core.LoadHandoffs(home)
	if err != nil {
		return ""
	}
	// Versión futura: la lista se leyó como vacía, así que callar sería afirmar
	// que este repo no tiene handoffs cuando la verdad es que no podemos saberlo.
	// El aviso no lleva datos interpolados (no hay ninguno fiable que mostrar),
	// así que la plantilla va tal cual — sin `$`, comillas ni backticks.
	if err := h.CheckUsable(); err != nil {
		return "echo \"⚠️  " + i18n.T(lang, "cli.handoff.hook_future_version") + "\" >&2\n"
	}
	if len(h.Active) == 0 {
		return ""
	}
	idxs := core.ActiveForCwd(h, cwd)
	switch len(idxs) {
	case 0:
		return ""
	case 1:
		m := h.Active[idxs[0]]
		return "echo \"↳ " + i18n.T(lang, "cli.handoff.hook_notice",
			core.QuoteInDoubleQuotes(m.From), core.QuoteInDoubleQuotes(m.To),
			core.QuoteInDoubleQuotes(m.Since)) + "\" >&2\n"
	default:
		return "echo \"↳ " + i18n.T(lang, "cli.handoff.hook_notice_many", len(idxs)) + "\" >&2\n"
	}
}

// warnConfigFallback avisa de que ccp.yaml no se pudo leer y que el emit cae al
// delta seguro de `default`. El idioma sale de i18n.Resolve("") (solo CCP_LANG)
// y no de currentLang(): estamos justo en la rama en la que leer la config ha
// fallado, así que currentLang() repetiría el intento para acabar en el mismo
// fallback — y esto corre en cada `cd` del usuario.
func warnConfigFallback(stderr io.Writer, err error) {
	fmt.Fprintf(stderr, "%s\n", i18n.T(i18n.Resolve(""), "cli.env.config_fallback", err))
}
