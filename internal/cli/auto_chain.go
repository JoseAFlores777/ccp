package cli

import (
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/JoseAFlores777/ccp/internal/core"
	"github.com/JoseAFlores777/ccp/internal/core/i18n"
)

// auto_chain.go — `ccp auto chain [show|add|rm|mv|set]`: parseo de banderas,
// impresión y códigos de salida. Toda la semántica vive en core/auto_chain.go.
//
// La regla de presentación de este comando es la que justifica que exista: lo que
// se imprime es qué cambió en CADA clave del yaml por separado (`fallback` y
// `allow_from`), nunca un «hecho» que las resuma. `add` toca las dos, y el usuario
// tiene derecho a ver que le hemos tocado el gate de cumplimiento — si no lo ve,
// el ensanche es silencioso, que es justo lo que no queremos.

// autoChain despacha los subcomandos de `ccp auto chain`. Sin subcomando (o con
// `show`) enseña la cadena EFECTIVA del cwd, o sea la ya filtrada por allow_from:
// es la única que responde a la pregunta que se hace el usuario, «¿a quién le va
// a prestar esto de verdad?».
func autoChain(args []string, stdout, stderr io.Writer) int {
	// El idioma de los errores de PARSEO sale de currentLang(): son anteriores a
	// tener la config cargada.
	lang := currentLang()
	policy := ""
	at := 0
	atSeen := false
	noAllow := false
	var rest []string
	for i := 0; i < len(args); i++ {
		switch a := args[i]; a {
		case "--policy":
			if i+1 >= len(args) {
				fmt.Fprintln(stderr, i18n.T(lang, "cli.auto.flag_needs_value", a))
				return 1
			}
			policy = args[i+1]
			i++
		case "--at":
			if i+1 >= len(args) {
				fmt.Fprintln(stderr, i18n.T(lang, "cli.auto.flag_needs_value", a))
				return 1
			}
			// --at es 1-based; un 0 o un negativo se rechazan AQUÍ porque en core
			// el cero significa «al final» y aceptarlo convertiría un dedo pegado
			// en un append silencioso.
			n, err := strconv.Atoi(strings.TrimSpace(args[i+1]))
			if err != nil || n < 1 {
				fmt.Fprintln(stderr, i18n.T(lang, "cli.auto.chain_bad_pos", args[i+1]))
				return 1
			}
			at = n
			atSeen = true
			i++
		case "--no-allow":
			noAllow = true
		case "--help", "-h":
			fmt.Fprintln(stdout, i18n.T(lang, "cli.auto.chain_usage"))
			return 0
		default:
			if strings.HasPrefix(a, "-") {
				fmt.Fprintln(stderr, i18n.T(lang, "cli.auto.unknown_flag", a))
				return 1
			}
			rest = append(rest, a)
		}
	}

	// El subcomando se toma del PRIMER posicional, no de args[0], y por eso el
	// parseo de banderas va antes. Mirando args[0] a secas, `ccp auto chain --at 1
	// add ds` degradaba a `show`: no añadía nada, no decía nada y salía 0, con una
	// salida que además tenía pinta de éxito. Una bandera delante del subcomando
	// es orden natural en casi cualquier CLI; que aquí significara «no hagas nada»
	// es exactamente la clase de silencio que este comando no se puede permitir.
	sub := "show"
	if len(rest) > 0 {
		sub = rest[0]
		rest = rest[1:]
	}

	// Banderas que solo tienen sentido para algunos subcomandos: tragárselas sin
	// decir nada es la misma trampa por otro lado (`--at 3 rm x` reordenaría en la
	// cabeza del usuario y no en el yaml).
	if atSeen && sub != "add" {
		fmt.Fprintln(stderr, i18n.T(lang, "cli.auto.chain_flag_only", "--at", "add"))
		return 1
	}
	if noAllow && sub != "add" && sub != "rm" && sub != "set" {
		fmt.Fprintln(stderr, i18n.T(lang, "cli.auto.chain_flag_only", "--no-allow", "add|rm|set"))
		return 1
	}

	home := resolveHome()
	// loadCfg (y no core.Load) porque dispara la migración perezosa: core.ChainAdd
	// vuelve a cargar por su cuenta y sobre un home aún sin ccp.yaml fallaría.
	cfg, err := loadCfg(home)
	if err != nil {
		fmt.Fprintf(stderr, "[error] %v\n", err)
		return 1
	}
	lang = i18n.Resolve(cfg.Lang)
	cwd := currentDir()
	opts := core.ChainOpts{Policy: policy, Cwd: cwd, At: at, NoAllow: noAllow}
	names := chainSplitList(rest)

	var (
		res  core.ChainResult
		cerr error
	)
	switch sub {
	case "show":
		// Posicionales sobrantes: descartarlos era cómo un subcomando mal escrito
		// («ccp auto chain shwo add x») acababa enseñando la cadena y saliendo 0.
		if len(names) > 0 {
			fmt.Fprintln(stderr, i18n.T(lang, "cli.auto.chain_show_extra", strings.Join(names, " ")))
			fmt.Fprintln(stderr, i18n.T(lang, "cli.auto.chain_usage"))
			return 1
		}
		return autoChainShow(home, cfg, lang, policy, cwd, stdout, stderr)
	case "help":
		fmt.Fprintln(stdout, i18n.T(lang, "cli.auto.chain_usage"))
		return 0
	case "add":
		res, cerr = core.ChainAdd(home, opts, names)
	case "rm":
		res, cerr = core.ChainRm(home, opts, names)
	case "mv":
		if len(names) != 2 {
			fmt.Fprintln(stderr, i18n.T(lang, "cli.auto.chain_mv_usage"))
			return 1
		}
		pos, err := strconv.Atoi(strings.TrimSpace(names[1]))
		if err != nil {
			fmt.Fprintln(stderr, i18n.T(lang, "cli.auto.chain_bad_pos", names[1]))
			return 1
		}
		res, cerr = core.ChainMv(home, opts, names[0], pos)
	case "set":
		res, cerr = core.ChainSet(home, opts, names)
	default:
		fmt.Fprintln(stderr, i18n.T(lang, "cli.auto.chain_unknown_sub", sub))
		fmt.Fprintln(stderr, i18n.T(lang, "cli.auto.chain_usage"))
		return 1
	}
	if cerr != nil {
		fmt.Fprintf(stderr, "[error] %s\n", chainErrText(lang, cerr))
		return 1
	}

	printChainResult(stdout, lang, res)
	fmt.Fprintln(stdout)
	printChainEffective(stdout, lang, home, policy, cwd)
	return 0
}

// autoChainShow enseña la cadena efectiva del cwd reusando ResolveAutoChain y las
// mismas etiquetas de `ccp auto status`: el filtrado de allow_from no se
// reimplementa aquí ni se pinta con otro formato, porque dos superficies que
// cuentan el mismo dato de dos maneras es cómo se pierde una tarde.
//
// Exit 1 cuando la política no resuelve (bloque ausente, deshabilitado, perfil de
// fallback inexistente), igual que `ccp auto status`: sirve de gate en scripts.
func autoChainShow(home string, cfg *core.Config, lang i18n.Lang, policy, cwd string, stdout, stderr io.Writer) int {
	rc, err := core.ResolveAutoChain(home, cfg, policy, cwd)
	if err != nil {
		// Por chainErrText, no por %v: es LA MISMA condición que los subcomandos
		// mutadores ya traducían, y tenerla en dos idiomas dentro del mismo
		// comando (`add` en inglés, `show` en español) es de las inconsistencias
		// que se ven de un vistazo.
		fmt.Fprintf(stderr, "[error] %s\n", chainErrText(lang, err))
		return 1
	}
	fmt.Fprintln(stdout, "  "+i18n.T(lang, "cli.auto.status_policy", rc.Policy.Name))
	fmt.Fprintln(stdout, "  "+i18n.T(lang, "cli.auto.status_primary", accent(stdout, rc.Primary), cwd))
	fmt.Fprintln(stdout, "  "+i18n.T(lang, "cli.auto.status_fallback", autoJoinOrNone(lang, rc.Fallback)))
	if len(rc.Denied) > 0 {
		fmt.Fprintln(stdout, "  "+mute(stdout, i18n.T(lang, "cli.auto.status_denied", strings.Join(rc.Denied, ", "))))
	}
	return 0
}

// printChainResult desglosa la mutación clave por clave.
//
// El orden es fallback primero y allow_from después porque es el orden en que el
// usuario piensa el cambio («quiero este perfil en la cadena» → «ah, y encima me
// ha hecho falta autorizarlo»). Los estados del gate son excluyentes y cada uno
// tiene su línea propia: no hay ninguna en la que «no se tocó» y «se ensanchó»
// se pinten igual — y por eso el switch acaba en un default que dice «sin
// cambios» en vez de en silencio. El silencio era la rama en la que caían `set`,
// `rm` y `mv`, y con `set` significaba que la cadena podía quedar INERTE (lista
// nueva, ningún destino autorizado) con un `[ok]` delante.
func printChainResult(w io.Writer, lang i18n.Lang, res core.ChainResult) {
	fmt.Fprintln(w, okLine(w, i18n.T(lang, "cli.auto.chain_fallback", autoJoinOrNone(lang, res.Fallback))))
	if len(res.Removed) > 0 {
		fmt.Fprintln(w, okLine(w, i18n.T(lang, "cli.auto.chain_removed", strings.Join(res.Removed, ", "))))
	}
	if res.Moved != "" {
		fmt.Fprintln(w, okLine(w, i18n.T(lang, "cli.auto.chain_moved", res.Moved, res.MovedTo)))
	}
	switch {
	case res.AllowSkipped:
		fmt.Fprintln(w, warnLine(w, i18n.T(lang, "cli.auto.chain_allow_skipped", res.Primary)))
	case res.GateAbsent:
		fmt.Fprintln(w, mute(w, i18n.T(lang, "cli.auto.chain_allow_nogate")))
	case res.AllowCreated && len(res.AllowAdded) > 0:
		fmt.Fprintln(w, okLine(w, i18n.T(lang, "cli.auto.chain_allow_created", res.Primary, chainPlus(res.AllowAdded))))
	case len(res.AllowAdded) > 0 || len(res.AllowRemoved) > 0:
		fmt.Fprintln(w, okLine(w, i18n.T(lang, "cli.auto.chain_allow_added", res.Primary,
			chainDiff(res.AllowAdded, res.AllowRemoved))))
	default:
		fmt.Fprintln(w, mute(w, i18n.T(lang, "cli.auto.chain_allow_unchanged", res.Primary)))
	}
	for _, n := range res.Notes {
		switch n.Kind {
		case core.ChainNotePrimaryImplicit:
			fmt.Fprintln(w, mute(w, i18n.T(lang, "cli.auto.chain_note_primary", n.Profile)))
		case core.ChainNoteAlreadyInChain:
			fmt.Fprintln(w, mute(w, i18n.T(lang, "cli.auto.chain_note_already", n.Profile)))
		}
	}
}

// printChainEffective cierra toda mutación con la cadena efectiva RELEÍDA del
// disco: es la comprobación de que lo escrito es lo que el motor va a leer, y
// deja ver de un vistazo si allow_from sigue bloqueando algo.
//
// Si la política no resuelve (p. ej. auto_handoff con enabled: false) se dice y
// se sale 0 igualmente: la escritura sí ocurrió, y devolver error haría creer que
// el cambio no llegó al yaml.
func printChainEffective(w io.Writer, lang i18n.Lang, home, policy, cwd string) {
	rc, err := core.ResolveAutoChain(home, nil, policy, cwd)
	if err != nil {
		// Traducido, no `%v`: si no, la plantilla inglesa acababa incrustando la
		// prosa castellana del core en su propio hueco.
		fmt.Fprintln(w, mute(w, i18n.T(lang, "cli.auto.chain_effective_unavailable", chainErrText(lang, err))))
		return
	}
	fmt.Fprintln(w, i18n.T(lang, "cli.auto.chain_effective"))
	fmt.Fprintln(w, "  "+autoJoinOrNone(lang, rc.Fallback))
	if len(rc.Denied) > 0 {
		fmt.Fprintln(w, "  "+mute(w, i18n.T(lang, "cli.auto.chain_denied", strings.Join(rc.Denied, ", "))))
	}
}

// chainErrText traduce los errores TIPADOS del core a los dos idiomas. Lo que no
// sea un core.ChainError (I/O, yaml corrupto) se reenvía tal cual: son errores de
// sistema, no de uso, y su texto ya es el del sistema.
//
// Devuelve el mensaje PELADO (sin `[error] `): quien imprime decide el prefijo,
// porque el mismo texto se usa dentro de otras plantillas (la línea de «cadena
// efectiva no disponible», el fallo de validación de `ccp config edit`, el campo
// `error` del JSON de `ccp auto status`).
func chainErrText(lang i18n.Lang, err error) string {
	var ce *core.ChainError
	if !errors.As(err, &ce) {
		return fmt.Sprintf("%v", err)
	}
	switch ce.Kind {
	case core.ChainErrNotConfigured:
		return i18n.T(lang, "cli.auto.not_configured")
	case core.ChainErrNoPolicy:
		return i18n.T(lang, "cli.auto.chain_err_no_policy", ce.Policy, ce.Detail)
	case core.ChainErrNoProfile:
		return i18n.T(lang, "cli.auto.unknown_profile", ce.Profile)
	case core.ChainErrDuplicate:
		return i18n.T(lang, "cli.auto.chain_err_duplicate", ce.Profile, ce.Policy)
	case core.ChainErrNotInChain:
		detail := ce.Detail
		if detail == "" {
			detail = i18n.T(lang, "cli.auto.status_none")
		}
		return i18n.T(lang, "cli.auto.chain_err_not_in_chain", ce.Profile, ce.Policy, detail)
	case core.ChainErrRange:
		return i18n.T(lang, "cli.auto.chain_err_range", ce.Pos, ce.Min, ce.Max)
	case core.ChainErrEmpty:
		return i18n.T(lang, "cli.auto.chain_err_empty")
	case core.ChainErrDisabled:
		return i18n.T(lang, "cli.auto.chain_err_disabled")
	case core.ChainErrFallbackProfile:
		return i18n.T(lang, "cli.auto.chain_err_fallback_profile", ce.Policy, ce.Profile)
	case core.ChainErrThreshold:
		return i18n.T(lang, "cli.auto.chain_err_threshold", ce.Policy, ce.Num)
	case core.ChainErrMaxHops:
		return i18n.T(lang, "cli.auto.chain_err_max_hops", ce.Policy, ce.Num)
	case core.ChainErrDuration:
		return i18n.T(lang, "cli.auto.chain_err_duration", ce.Policy, ce.Key, ce.Value, ce.Cause)
	case core.ChainErrDurationNeg:
		return i18n.T(lang, "cli.auto.chain_err_duration_neg", ce.Policy, ce.Key, ce.Value)
	case core.ChainErrCooldown:
		return i18n.T(lang, "cli.auto.chain_err_cooldown", ce.Policy, ce.Value,
			core.CooldownResetsAt, core.CooldownFixed)
	}
	return fmt.Sprintf("%v", err)
}

// chainSplitList acepta las dos formas de nombrar varios perfiles: separados por
// espacios (`add a b`) y por comas (`set a,b,c`, que es como lo documenta la
// ayuda). Los nombres de perfil no llevan comas, así que no hay ambigüedad.
func chainSplitList(args []string) []string {
	out := make([]string, 0, len(args))
	for _, a := range args {
		for _, p := range strings.Split(a, ",") {
			if p = strings.TrimSpace(p); p != "" {
				out = append(out, p)
			}
		}
	}
	return out
}

// chainPlus prefija cada nombre con '+' para que la línea de allow_from se lea
// como el diff que es (`allow_from personal-1: +personal-deepseek`).
func chainPlus(list []string) string {
	out := make([]string, 0, len(list))
	for _, n := range list {
		out = append(out, "+"+n)
	}
	return strings.Join(out, " ")
}

// chainDiff pinta las dos direcciones del gate en una sola línea, con el mismo
// vocabulario de diff: `+perfil` autoriza, `-perfil` retira. Los ensanches van
// primero porque son los que abren permisos, y son lo que el usuario tiene que
// leer aunque solo mire la primera palabra.
func chainDiff(added, removed []string) string {
	out := make([]string, 0, len(added)+len(removed))
	for _, n := range added {
		out = append(out, "+"+n)
	}
	for _, n := range removed {
		out = append(out, "-"+n)
	}
	return strings.Join(out, " ")
}
