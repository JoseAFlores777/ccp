package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/JoseAFlores777/ccp/internal/core"
	"github.com/JoseAFlores777/ccp/internal/core/i18n"
	"github.com/JoseAFlores777/ccp/internal/supervisor"
)

// session.go — la cara de terminal de `ccp session`.
//
// El comando es deliberadamente delgado: TODA la inteligencia (a dónde saltar,
// cuándo, con qué entorno) vive en internal/supervisor. Aquí solo se hacen tres
// cosas que el supervisor no debe saber hacer: parsear la línea de comandos,
// traducir su desenlace a un exit code estable, y hablarle al usuario en su
// idioma (internal/supervisor no habla i18n a propósito).
//
// Por qué NO se usa flag.FlagSet: el resto del binario despacha a mano —la
// completion bash/zsh se emite verbatim y no puede depender de un framework— y
// además `flag` se comería el `--` que separa nuestros flags de los de claude
// (para él `--` termina el parseo y el resto son posicionales, pero también
// reordena y acepta formas `-flag` de un guion que aquí serían ambiguas con los
// flags de claude). Parsear a mano es más código y cero sorpresas.

// sessionFlags es la línea de comandos ya interpretada.
type sessionFlags struct {
	headless  bool
	policy    string
	maxHops   int
	yolo      bool
	session   string
	dryRun    bool
	noReturn  bool
	claudeBin string
	help      bool
	args      []string // lo que va DESPUÉS de `--`, sin interpretar
}

// cmdSession implementa `ccp session`. La firma la fija el dispatch de cli.go.
func cmdSession(args []string, stdout, stderr io.Writer) int {
	lang := currentLang()

	f, err := parseSessionFlags(args, lang)
	if err != nil {
		fmt.Fprintf(stderr, "[error] %v\n", err)
		return 1
	}
	if f.help {
		fmt.Fprintln(stdout, i18n.T(lang, "cli.session.usage"))
		return 0
	}

	home := resolveHome()
	// loadCfg = ensureMigrated + Load: `ccp session` puede ser el primer comando
	// Go que corre un usuario que venía del bash, y lanzar claude con un ccp.yaml
	// sin migrar resolvería el perfil equivocado.
	cfg, err := loadCfg(home)
	if err != nil {
		fmt.Fprintf(stderr, "[error] %v\n", err)
		return 1
	}
	cwd := currentDir()

	// Pre-chequeo de la política ANTES de arrancar nada. El supervisor volvería a
	// resolverla igual (y con el mismo error), pero su mensaje es una cadena de
	// core en español y sin la pista de qué comando corregir; aquí se distingue
	// «nunca lo configuraste» de «lo apagaste» y se nombra `ccp auto init`, que
	// es la única acción que desbloquea el comando.
	switch {
	case cfg.AutoHandoff == nil:
		fmt.Fprintf(stderr, "[error] %s\n", i18n.T(lang, "cli.session.not_configured"))
		return 1
	case !cfg.AutoHandoff.Enabled:
		fmt.Fprintf(stderr, "[error] %s\n", i18n.T(lang, "cli.session.disabled"))
		return 1
	}
	if _, err := core.ResolveAutoChain(home, cfg, f.policy, cwd); err != nil {
		// Política inexistente, fallback a un perfil que no existe, duración mal
		// escrita: errores de ccp.yaml, no del supervisor. Fallar aquí evita
		// dejar un claude lanzado y morir en el primer salto, dos horas después.
		// Van por chainErrText, que es el mismo traductor que usa `ccp auto`: son
		// literalmente los mismos errores tipados.
		fmt.Fprintf(stderr, "[error] %s\n", chainErrText(lang, err))
		return 1
	}

	// Sin tty y sin -p el claude interactivo no va a arrancar bien, pero NO se
	// bloquea: hay entornos (multiplexores, editores, contenedores con pty
	// emulada) donde la heurística miente en ambos sentidos. Se avisa y se sigue;
	// si de verdad no hay terminal, quien falla es claude —con su propio
	// mensaje— y no ccp negándose a intentarlo.
	if !f.headless && !sessionHasTTY() {
		fmt.Fprintln(stderr, warnLine(stderr, i18n.T(lang, "cli.session.no_tty")))
	}

	// Out/Err van directos a los writers del dispatch: la traza del supervisor
	// (los saltos, la vuelta a casa, la tabla de cooldowns) ES la salida del
	// comando, no un log secundario. En headless, Out lleva además el
	// stream-json del hijo, ya serializado por el propio supervisor.
	res, runErr := supervisor.Run(context.Background(), supervisor.Options{
		Home:      home,
		Cwd:       cwd,
		Policy:    f.policy,
		Headless:  f.headless,
		Yolo:      f.yolo,
		MaxHops:   f.maxHops,
		Session:   f.session,
		Args:      f.args,
		ClaudeBin: f.claudeBin,
		Out:       stdout,
		Err:       stderr,
		Stdin:     os.Stdin,
		DryRun:    f.dryRun,
		NoReturn:  f.noReturn,
	})
	if runErr != nil {
		fmt.Fprintf(stderr, "[error] %v\n", runErr)
	} else if res.Parked {
		// El supervisor ya trazó QUÉ pasó y cuándo se libera cada perfil; lo que
		// falta es qué significa el 75 para quien lo lea desde un cron.
		fmt.Fprintln(stderr, i18n.T(lang, "cli.session.parked", supervisor.ParkedExitCode))
	}
	return sessionExitCode(res, runErr)
}

// sessionExitCode traduce el desenlace del supervisor al exit code del proceso.
//
// El orden de las ramas ES el contrato:
//   - un error mata el resultado: handoffExit devuelve 2 si lo que falló fue
//     persistir handoffs.yaml (el estado quedó desincronizado y hay que
//     escalarlo) y 1 en cualquier otro fallo de uso/config/arranque;
//   - Parked es 75 (EX_TEMPFAIL): «vuelve más tarde», no «te equivocaste». Un
//     cron distingue así reintentar de avisar a un humano;
//   - si no, el código del hijo tal cual, que es lo que hace `ccp session -p …`
//     sustituible por `claude -p …` dentro de un script.
func sessionExitCode(res supervisor.Result, err error) int {
	if err != nil {
		return handoffExit(err)
	}
	if res.Parked {
		return supervisor.ParkedExitCode
	}
	return res.ExitCode
}

// sessionHasTTY reporta si stdin es un dispositivo de caracteres. Es la misma
// heurística que useColor aplica a la salida (os.ModeCharDevice) en vez de una
// dependencia nueva: aquí solo decide si imprimir un aviso, así que un falso
// negativo cuesta una línea de ruido, no una funcionalidad.
func sessionHasTTY() bool {
	info, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}

// parseSessionFlags interpreta la línea de comandos de `ccp session`, al estilo
// de parseHandoffFlags: a mano, en cualquier orden, y con dos reglas duras.
//
//  1. `--` es una frontera absoluta: todo lo que venga después se copia a
//     Options.Args SIN mirarlo. Es lo que permite `ccp session -p -- -p "hola"`
//     o pasarle a claude un `--session-id` propio sin que ccp lo secuestre.
//  2. Un flag desconocido ANTES de `--` es un error, no algo que reenviar a
//     claude en silencio: `ccp session --polciy foo` tiene que fallar aquí y no
//     arrancar una sesión con la política por defecto que el usuario no pidió.
//
// Se admiten las dos formas de valor (`--policy x` y `--policy=x`) porque ambas
// son reflejo muscular y la segunda no cuesta nada; a los flags booleanos se les
// rechaza el `=valor` explícitamente para que `--dry-run=false` —que en `flag`
// significaría lo contrario de lo que hace aquí— no pase por bueno.
func parseSessionFlags(args []string, lang i18n.Lang) (sessionFlags, error) {
	var f sessionFlags

	// i vive FUERA del for para que los helpers que consumen el argumento
	// siguiente puedan avanzarlo sin depender de la semántica por-iteración de
	// las variables de bucle.
	i := 0
	for ; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			// Sin copia: el subslice es exactamente lo que hay que reenviar, y
			// nadie lo muta después.
			f.args = args[i+1:]
			return f, nil
		}

		name, inline, hasInline := splitSessionFlag(a)

		// value entrega el valor del flag actual (pegado o el siguiente
		// argumento). Un valor vacío se trata como ausente: `--policy=` es un
		// dedo pegado, no una petición de política sin nombre.
		value := func() (string, error) {
			v := inline
			if !hasInline {
				if i+1 >= len(args) {
					return "", fmt.Errorf("%s", i18n.T(lang, "cli.session.flag_needs_value", name))
				}
				i++
				v = args[i]
			}
			if strings.TrimSpace(v) == "" {
				return "", fmt.Errorf("%s", i18n.T(lang, "cli.session.flag_needs_value", name))
			}
			return v, nil
		}
		// bare rechaza el `=valor` en un flag booleano.
		bare := func() error {
			if hasInline {
				return fmt.Errorf("%s", i18n.T(lang, "cli.session.flag_no_value", name))
			}
			return nil
		}

		var err error
		switch name {
		case "-p", "--headless":
			err = bare()
			f.headless = true
		case "--yolo", "--dangerously-skip-permissions":
			// Nunca se persiste: es una decisión por invocación, y guardarla
			// convertiría un descuido de hoy en el modo por defecto de mañana.
			err = bare()
			f.yolo = true
		case "--dry-run":
			err = bare()
			f.dryRun = true
		case "--no-return":
			err = bare()
			f.noReturn = true
		case "-h", "--help":
			err = bare()
			f.help = true
		case "--policy":
			f.policy, err = value()
		case "--session":
			f.session, err = value()
		case "--claude-bin":
			f.claudeBin, err = value()
		case "--max-hops":
			var raw string
			if raw, err = value(); err == nil {
				n, cerr := strconv.Atoi(strings.TrimSpace(raw))
				if cerr != nil || n < 0 {
					err = fmt.Errorf("%s", i18n.T(lang, "cli.session.bad_max_hops", raw))
					break
				}
				f.maxHops = n
			}
		default:
			if strings.HasPrefix(a, "-") {
				err = fmt.Errorf("%s", i18n.T(lang, "cli.session.unknown_flag", a))
				break
			}
			// Un posicional suelto casi siempre es el prompt de headless escrito
			// sin `--`. Decírselo es más útil que reenviarlo: si lo pasáramos a
			// claude, `ccp session "borra todo"` y `ccp session --yolo` se
			// leerían igual de bien y solo uno haría lo que parece.
			err = fmt.Errorf("%s", i18n.T(lang, "cli.session.extra_arg", a))
		}
		if err != nil {
			return f, err
		}
	}
	return f, nil
}

// splitSessionFlag parte `--flag=valor` en sus dos mitades. Solo se aplica a
// argumentos que empiezan por `-`: un posicional con `=` (una asignación en un
// prompt, por ejemplo) no es un flag con valor pegado.
func splitSessionFlag(a string) (name, val string, hasVal bool) {
	if !strings.HasPrefix(a, "-") {
		return a, "", false
	}
	if i := strings.IndexByte(a, '='); i > 0 {
		return a[:i], a[i+1:], true
	}
	return a, "", false
}
