package cli

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/JoseAFlores777/ccp/internal/core"
	"github.com/JoseAFlores777/ccp/internal/core/i18n"
)

// session_bootstrap.go — el «configúrame esto» de `ccp session`: detectar lo que
// le falta al repo, enseñarlo TODO JUNTO, preguntar UNA vez y aplicar.
//
// El guard es lo primero que se decide y manda sobre el resto del archivo:
//
//	SIN CONVERSACIÓN POSIBLE —-p/--headless, stdin que no es terminal, o salida
//	redirigida a un archivo— NO se pregunta ni se muta NADA.
//
// `ccp session -p` corre desde cron. Un prompt ahí cuelga el job para siempre
// —nadie va a teclear la respuesta— y una mutación silenciosa ahí cambia la
// configuración del usuario sin que nadie lo vea. Y con la salida redirigida el
// prompt existe pero nadie lo lee, que es peor: el usuario pulsa Enter a ciegas
// y el default de [S/n] es SÍ. En esos modos se avisa por stderr —cada uno con
// su motivo— y se sigue con el comportamiento de hoy: si falta el bloque
// auto_handoff, el comando falla con el mensaje que ya existe.
//
// El otro invariante: el bootstrap NUNCA impide lanzar una sesión. Cualquier
// fallo suyo (detección, escritura, caché) se degrada a «no se hizo nada» y el
// comando sigue su camino normal. Es una comodidad, no un requisito.
//
// Todo lo que se le dice al usuario va por stderr, igual que la charla operativa
// del supervisor: el stdout de `ccp session` es la salida del comando (la traza
// de saltos, y en headless el stream-json del hijo).

// bootstrapBlock es el veredicto del guard: o se puede preguntar, o hay un
// motivo CONCRETO por el que no. Es un enum y no un booleano porque el motivo se
// le dice al usuario, y los tres tienen consejos distintos: a quien corre con -p
// desde una terminal no se le puede sugerir «hazlo desde una terminal» (ya está
// en una), y a quien redirigió la salida no se le puede decir que no tiene
// terminal (la tiene: lo que no tiene es dónde leer la pregunta).
type bootstrapBlock int

const (
	bootstrapAsk             bootstrapBlock = iota // hay con quién hablar
	bootstrapBlockHeadless                         // -p / --headless
	bootstrapBlockNoTTY                            // stdin no es una terminal
	bootstrapBlockRedirected                       // la salida no va a una terminal
)

// bootstrapEnv es el contexto ya resuelto por el llamador. `block` llega
// DECIDIDO —lo calcula sessionBootstrapBlock— para que los tests puedan
// ejercitar la rama interactiva sin una tty de verdad, que bajo `go test` no
// existe, y el guard en sí se prueba por su nombre.
type bootstrapEnv struct {
	home   string
	cwd    string
	block  bootstrapBlock
	force  bool // --setup: preguntar aunque la caché diga que ya se preguntó
	skip   bool // --no-setup
	policy string
	active string // $CCP_PROFILE de esta terminal
	stdin  io.Reader
	w      io.Writer // stderr
	lang   i18n.Lang
}

// blockKey es el aviso que le corresponde a cada motivo. Que sean claves
// distintas es el punto: un solo texto para los tres describía únicamente el
// primero, y a quien pasaba `-p --setup` desde una terminal le contestaba «corre
// ccp session --setup desde una terminal» — un callejón sin salida.
func (b bootstrapBlock) blockKey() string {
	switch b {
	case bootstrapBlockHeadless:
		return "cli.bootstrap.headless"
	case bootstrapBlockRedirected:
		return "cli.bootstrap.redirected"
	}
	return "cli.bootstrap.non_interactive"
}

// sessionBootstrap corre el flujo completo y devuelve el Config con el que hay
// que seguir: el recargado si se aplicó algo, o el mismo que entró en cualquier
// otro caso.
//
// Devolver el Config (en vez de mutar el de entrada) es lo que evita el bug
// obvio de esta fase: el `switch` que valida auto_handoff y el ResolveAutoChain
// que le sigue leen un puntero que acaba de quedarse obsoleto en disco.
func sessionBootstrap(cfg *core.Config, e bootstrapEnv) *core.Config {
	if e.skip {
		return cfg
	}

	// La detección es PURA: no lee disco, no escribe, no pregunta. Por eso puede
	// correr también en el camino no interactivo — lo único que hace allí es dar
	// material para el aviso.
	plan, err := core.BootstrapDetect(cfg, core.BootstrapInput{
		Cwd:    e.cwd,
		Repo:   bootstrapRepoRoot(e.cwd),
		Active: e.active,
		Policy: e.policy,
	})
	if err != nil || !plan.HasGaps() {
		// Un error de detección es un ccp.yaml que el bootstrap no sabe arreglar
		// (política inexistente, duración mal escrita): se calla y deja que
		// `ccp session` lo reporte acto seguido con su traductor, que es el mismo.
		return cfg
	}

	if e.block != bootstrapAsk {
		fmt.Fprintln(e.w, warnLine(e.w, i18n.T(e.lang, e.block.blockKey(),
			bootstrapGapNames(e.lang, plan))))
		return cfg
	}

	// La caché es lo ÚLTIMO que se consulta antes de hablar: si no hubiera huecos
	// no habría nada que recordar, y con --setup la memoria se ignora a propósito.
	if !e.force {
		if _, ok := core.ReadBootstrapMark(e.home, plan.Repo); ok {
			return cfg
		}
	}

	printBootstrapPlan(e.w, e.lang, plan)

	accepted, answered := promptConfirm(e.w, e.stdin, i18n.T(e.lang, "cli.bootstrap.ask"))

	var applied core.BootstrapApplied
	var applyErr error
	switch {
	case accepted:
		applied, applyErr = core.BootstrapApply(e.home, plan)
		printBootstrapApplied(e.w, e.lang, applied)
		if applyErr != nil {
			fmt.Fprintln(e.w, warnLine(e.w, i18n.T(e.lang, "cli.bootstrap.apply_failed",
				bootstrapFailedLabel(e.lang, applyErr), applyErr)))
		}
	case answered:
		fmt.Fprintln(e.w, mute(e.w, i18n.T(e.lang, "cli.bootstrap.declined")))
	default:
		// Ni sí ni no: el stdin se cerró a mitad. No se aplica nada Y no se gasta
		// la memoria — ver abajo.
		fmt.Fprintln(e.w, mute(e.w, i18n.T(e.lang, "cli.bootstrap.unanswered")))
	}

	// La marca se escribe diga el usuario que sí o que no: «preguntar una vez»
	// significa una vez. Best-effort — si la caché no se puede escribir, el único
	// coste es volver a preguntar, y ese es el fallo correcto para una caché.
	//
	// Pero solo si CONTESTÓ. Una pregunta que nadie llegó a responder (EOF a
	// mitad) no puede gastar la promesa: dejaría el repo sin configurar y el
	// flujo mudo para siempre, y el único camino de vuelta —`--setup`— se nombra
	// en el mensaje del «no», que en ese caso no se imprime. La marca recuerda
	// que se RESPONDIÓ una vez, no que se habló una vez.
	if !answered {
		return cfg
	}
	_ = core.WriteBootstrapMark(e.home, core.BootstrapMark{
		Repo:     plan.Repo,
		Accepted: accepted,
		Applied:  applied.Kinds,
	})

	if len(applied.Kinds) == 0 {
		return cfg
	}
	// Ya migrado por loadCfg; aquí solo hace falta releer.
	fresh, err := core.Load(e.home)
	if err != nil {
		fmt.Fprintln(e.w, warnLine(e.w, i18n.T(e.lang, "cli.bootstrap.reload_failed", err)))
		return cfg
	}
	return fresh
}

// bootstrapRepoRoot es la raíz del repo RE-ANCLADA en la ruta lógica del cwd.
//
// `git rev-parse --show-toplevel` devuelve la ruta FÍSICA (con los symlinks ya
// resueltos) y ccp resuelve las reglas contra la LÓGICA ($PWD). En cuanto el repo
// se alcanza por un enlace —el /tmp → /private/tmp de macOS, un ~/work que apunta
// a otro disco— son dos cadenas distintas para el mismo directorio, y la raíz
// física no cubre el cwd lógico. Sin re-anclar, la detección se rendía y proponía
// la regla sobre el CWD: justo el anti-patrón que el flujo existe para evitar (un
// `cd ..` cambia de perfil), y encima justificado con un «esto no es un repo
// git» que era falso.
//
// La traducción es puramente aritmética: si el cwd físico está N niveles por
// debajo de la raíz física, la raíz lógica está N niveles por encima del cwd
// lógico. Eso preserva el camino que el usuario tiene delante, que es el que
// `ccp resolve` va a comparar.
//
// Cualquier fallo devuelve la raíz tal cual y deja que core.BootstrapDetect
// aplique su red (caer al cwd, diciéndolo): esta función mejora el ancla, no
// decide nada.
func bootstrapRepoRoot(cwd string) string {
	root := gitRepoRoot(cwd)
	if root == "" || cwd == "" {
		return root
	}
	phys, err := filepath.EvalSymlinks(cwd)
	if err != nil {
		return root
	}
	rel, err := filepath.Rel(root, phys)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return root // el cwd no cuelga de la raíz: no hay nada que traducir
	}
	if rel == "." {
		return cwd // el cwd ES la raíz, por el camino del usuario
	}
	out := cwd
	for range strings.Split(rel, string(filepath.Separator)) {
		out = filepath.Dir(out)
	}
	return out
}

// printBootstrapPlan imprime el resumen: los cuatro chequeos JUNTOS, los que
// faltan y los que ya están.
//
// Se enseñan también los que están OK a propósito. El usuario está a punto de
// autorizar escrituras en su configuración: ver la lista completa es lo que le
// permite juzgar si el diagnóstico entiende su repo, y una fila «(ok)» es la
// única forma de distinguir «esto ya estaba» de «esto ni se miró».
func printBootstrapPlan(w io.Writer, lang i18n.Lang, plan core.BootstrapPlan) {
	fmt.Fprintln(w)
	fmt.Fprintln(w, boldLine(w, i18n.T(lang, "cli.bootstrap.header")))

	type row struct{ label, detail, action string }
	rows := make([]row, 0, len(plan.Items))
	for _, it := range plan.Items {
		rows = append(rows, row{
			label:  i18n.T(lang, "cli.bootstrap.label."+string(it.Kind)),
			detail: bootstrapDetail(lang, it),
			action: bootstrapAction(lang, it),
		})
	}
	// La columna de detalle se alinea, pero con TOPE: una ruta de repo profunda
	// empujaría el «(ok)» de las otras tres filas fuera de la pantalla, y la
	// columna que de verdad hay que poder leer de un vistazo es la de acción. Lo
	// que se pase del tope simplemente lleva su acción más a la derecha.
	const detailCap = 44
	labelW, detailW := 0, 0
	for _, r := range rows {
		if n := utf8.RuneCountInString(r.label); n > labelW {
			labelW = n
		}
		if n := utf8.RuneCountInString(r.detail); n > detailW && n <= detailCap {
			detailW = n
		}
	}
	for i, r := range rows {
		// Apagada solo si de verdad no hay nada pendiente: un hueco aplazado
		// (Blocked) no es un «(ok)» y no puede leerse como tal.
		muted := !plan.Items[i].Missing && !plan.Items[i].Blocked
		line := fmt.Sprintf("  · %s  %s  %s",
			padRunes(r.label, labelW), padRunes(r.detail, detailW), r.action)
		if muted {
			line = mute(w, line)
		}
		fmt.Fprintln(w, line)
	}

	// Las notas van DEBAJO de la tabla y no dentro de la celda: son la diferencia
	// entre que el usuario vea qué ruta se va a escribir y que la adivine, y no
	// caben en una columna alineada.
	for _, it := range plan.Items {
		for _, note := range bootstrapNotes(lang, it) {
			fmt.Fprintln(w, mute(w, note))
		}
	}
	fmt.Fprintln(w)
}

// bootstrapNotes son las líneas de contexto de una fila.
//
// La de la regla tiene TRES casos y no dos, y confundirlos es afirmar un hecho
// falso en la única frase que el usuario lee para decidir: puede no haber repo
// git, o haberlo y que su raíz no cubra este directorio (una raíz que no ancla
// no sirve como sitio para la regla). Decir «esto no es un repo git» cuando sí
// lo es empujaba además al anti-patrón que el flujo existe para evitar —la regla
// sobre un subdirectorio— sin que nadie pudiera notarlo.
func bootstrapNotes(lang i18n.Lang, it core.BootstrapItem) []string {
	var out []string
	switch {
	case it.Kind == core.BootstrapRule && it.Missing:
		switch {
		case it.FromGit:
			out = append(out, i18n.T(lang, "cli.bootstrap.rule_git"))
		case it.GitRoot != "":
			out = append(out, i18n.T(lang, "cli.bootstrap.rule_git_unanchored", it.GitRoot))
		default:
			out = append(out, i18n.T(lang, "cli.bootstrap.rule_no_git"))
		}
		if it.Profile == "" {
			out = append(out, i18n.T(lang, "cli.bootstrap.rule_needs_profile", it.Path))
		}
	case it.Kind == core.BootstrapChain && it.Blocked:
		// El hueco existe pero no se toca: sin regla no se sabe quién es el
		// primario, y allow_from es SU gate. Se dice en voz alta porque la fila
		// enseña perfiles denegados y callar por qué no se van a autorizar se
		// leería como que el flujo no los vio.
		out = append(out, i18n.T(lang, "cli.bootstrap.chain_blocked",
			strings.Join(it.Profiles, ", ")))
	}
	return out
}

// bootstrapDetail describe la fila. Cada kind trae sus propios datos; la prosa
// está toda en el catálogo.
func bootstrapDetail(lang i18n.Lang, it core.BootstrapItem) string {
	switch it.Kind {
	case core.BootstrapAuto:
		return i18n.T(lang, "cli.bootstrap.detail.auto")
	case core.BootstrapRule:
		if it.Profile == "" {
			return i18n.T(lang, "cli.bootstrap.detail.no_profile", it.Path)
		}
		return i18n.T(lang, "cli.bootstrap.detail.rule", it.Path, it.Profile)
	case core.BootstrapChain:
		if len(it.Profiles) > 0 {
			return it.Policy + ": " + strings.Join(it.Profiles, ", ")
		}
		return it.Policy
	case core.BootstrapSensors:
		if len(it.Profiles) == 0 {
			return i18n.T(lang, "cli.bootstrap.detail.none")
		}
		return strings.Join(it.Profiles, ", ")
	}
	return ""
}

// bootstrapAction es la última columna: qué se le va a hacer a esa fila.
//
// Es la columna con la que el usuario decide, así que no puede prometer lo que
// no va a pasar: una regla sin perfil deducible NO se crea (BootstrapApply la
// salta), y un hueco aplazado tampoco se toca. Ambos decían «(crear)» /
// «(ensanchar)».
func bootstrapAction(lang i18n.Lang, it core.BootstrapItem) string {
	if it.Blocked {
		return i18n.T(lang, "cli.bootstrap.action.later")
	}
	if !it.Missing {
		return i18n.T(lang, "cli.bootstrap.action.ok")
	}
	switch it.Kind {
	case core.BootstrapAuto:
		return i18n.T(lang, "cli.bootstrap.action.seed")
	case core.BootstrapRule:
		if it.Profile == "" {
			return i18n.T(lang, "cli.bootstrap.action.needs_profile")
		}
		return i18n.T(lang, "cli.bootstrap.action.create")
	case core.BootstrapChain:
		return i18n.T(lang, "cli.bootstrap.action.widen")
	case core.BootstrapSensors:
		return i18n.T(lang, "cli.bootstrap.action.install")
	}
	return ""
}

// printBootstrapApplied cuenta lo que se hizo, paso por paso.
//
// Desglosado y no resumido en un «hecho»: son escrituras en la configuración del
// usuario —una de ellas en un gate de cumplimiento— y quien las autorizó tiene
// derecho a leer exactamente cuáles fueron sin abrir el yaml.
func printBootstrapApplied(w io.Writer, lang i18n.Lang, a core.BootstrapApplied) {
	// Lo que NO se hizo va primero y en aviso, no al final y de pasada: es lo
	// único del parte que le deja trabajo pendiente a quien acaba de decir que sí,
	// y la marca de «preguntar una vez» ya se habrá gastado cuando lo lea.
	for _, it := range a.Skipped {
		if it.Kind != core.BootstrapRule {
			continue
		}
		fmt.Fprintln(w, warnLine(w, i18n.T(lang, "cli.bootstrap.skipped.rule", it.Path)))
	}
	for _, k := range a.Kinds {
		switch k {
		case core.BootstrapAuto:
			fmt.Fprintln(w, okLine(w, i18n.T(lang, "cli.bootstrap.done.auto")))
		case core.BootstrapRule:
			fmt.Fprintln(w, okLine(w, i18n.T(lang, "cli.bootstrap.done.rule", a.Rule, a.Profile)))
		case core.BootstrapChain:
			fmt.Fprintln(w, okLine(w, i18n.T(lang, "cli.bootstrap.done.chain",
				a.Primary, strings.Join(a.Allowed, ", "))))
		case core.BootstrapSensors:
			fmt.Fprintln(w, okLine(w, i18n.T(lang, "cli.bootstrap.done.sensors",
				strings.Join(a.Sensors, ", "))))
		}
	}
}

// bootstrapGapNames nombra los huecos para el aviso de una línea del modo no
// interactivo, usando las MISMAS etiquetas que la tabla.
func bootstrapGapNames(lang i18n.Lang, plan core.BootstrapPlan) string {
	gaps := plan.Gaps()
	out := make([]string, 0, len(gaps))
	for _, it := range gaps {
		out = append(out, i18n.T(lang, "cli.bootstrap.label."+string(it.Kind)))
	}
	return strings.Join(out, ", ")
}

// bootstrapFailedLabel traduce el paso que falló. El *BootstrapError lleva el
// kind precisamente para esto: sin él habría que adivinar por el texto de la
// causa, que es castellano de core y no se traduce.
func bootstrapFailedLabel(lang i18n.Lang, err error) string {
	var be *core.BootstrapError
	if errors.As(err, &be) {
		return i18n.T(lang, "cli.bootstrap.label."+string(be.Kind))
	}
	return "?"
}

// promptConfirm hace UNA pregunta de sí/no. Por defecto sí (el prompt es [S/n]).
//
// Devuelve DOS cosas, y la segunda es la que evita el fallo silencioso:
// `answered` distingue «contestó que no» de «no contestó». Solo la primera gasta
// la promesa de «preguntar una vez»; con la segunda, escribir la marca dejaría el
// repo sin configurar y sin que nadie vuelva a ofrecerlo jamás.
//
// Tres decisiones más, y las tres tienen consecuencias:
//
//  1. Lee BYTE A BYTE hasta el salto de línea, no con un bufio.Scanner. El mismo
//     stdin se le entrega después a `claude`, y un lector con buffer se traga
//     hasta 64 KB de una vez: lo que el usuario tecleara mientras arranca la
//     sesión desaparecería dentro de un buffer que nadie va a volver a leer.
//  2. EOF NO es un sí, y lo es en el sentido fuerte: sin salto de línea la
//     respuesta no cuenta, ni siquiera cuando llegaron caracteres antes del
//     cierre. Una línea vacía sí acepta el default —el usuario pulsó Enter a
//     conciencia—, pero un stdin que se cierra a mitad no ha terminado de decir
//     nada, y tomar eso por consentimiento para escribir en ccp.yaml es
//     exactamente el fallo que este archivo entero intenta no cometer.
//  3. Una respuesta que no se entiende es un NO. Preguntar otra vez sería
//     razonable en un formulario; aquí el usuario venía a lanzar una sesión.
func promptConfirm(w io.Writer, r io.Reader, prompt string) (accepted, answered bool) {
	if r == nil {
		return false, false
	}
	fmt.Fprint(w, prompt)

	var (
		sb   strings.Builder
		buf  [1]byte
		done bool
	)
	for {
		n, err := r.Read(buf[:])
		if n > 0 {
			if buf[0] == '\n' {
				done = true
				break
			}
			sb.WriteByte(buf[0])
		}
		if err != nil {
			break
		}
	}
	if !done {
		// El salto que el usuario no llegó a escribir, para no dejar el prompt
		// pegado a lo que imprima el comando después.
		fmt.Fprintln(w)
		return false, false
	}

	s := strings.ToLower(strings.TrimSpace(sb.String()))
	if s == "" {
		return true, true // Enter a secas = el default del prompt, que es sí.
	}
	switch s {
	case "s", "si", "sí", "y", "yes":
		return true, true
	}
	return false, true
}

// padRunes rellena a la derecha contando RUNAS, no bytes: «política» ocupa 8
// columnas y 9 bytes, y alinear por bytes torcería la tabla en español.
func padRunes(s string, width int) string {
	if n := utf8.RuneCountInString(s); n < width {
		return s + strings.Repeat(" ", width-n)
	}
	return s
}
