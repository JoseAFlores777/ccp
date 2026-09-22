package core

import (
	"fmt"
	"strings"
)

// auto_chain.go — las MUTACIONES de la cadena de auto-handoff: el orden de
// `auto_handoff.policies.<n>.fallback` y la actualización ACOTADA de
// `auto_handoff.allow_from[<primario>]`.
//
// auto.go resuelve (lee); este archivo escribe. La separación importa porque
// escribir aquí toca DOS claves que el yaml mantiene deliberadamente separadas y
// que juntas deciden si un perfil se usa o no:
//
//	fallback   — a quién se le puede prestar la sesión, EN ORDEN de preferencia.
//	allow_from — desde qué primario se autoriza ese préstamo (gate de cumplimiento).
//
// Un perfil en `fallback` pero fuera de `allow_from[primario]` NO se usa nunca, y
// el yaml no lo dice en ninguna parte: es exactamente la trampa que `ccp auto
// chain add` existe para cerrar. Por eso las funciones de add devuelven en el
// ChainResult qué pasó en CADA clave por separado — el CLI está obligado a
// contarlo línea a línea, no a resumirlo en un «hecho».
//
// Nada de aquí imprime: se devuelven datos y errores tipados (ChainError) para
// que internal/cli los traduzca a los dos idiomas.

// ChainOpts son las opciones comunes de las mutaciones de la cadena.
type ChainOpts struct {
	Policy  string // "" => "default". Nombrarla APUNTA a su lista compartida.
	Cwd     string // desde dónde se resuelve el primario (core.Resolve)
	At      int    // solo add: posición 1-based donde insertar; 0 = al final
	NoAllow bool   // solo add: NO tocar allow_from

	// For es el perfil cuya cadena PROPIA se edita. Vacío = el primario del cwd,
	// que es el destino por defecto desde que las cadenas son por perfil: quien
	// escribe `ccp auto chain add x` dentro de un repo está hablando de ESE
	// perfil, no de la lista que comparten todos.
	For string

	// Shared apunta a la lista compartida de la política (el comportamiento de
	// antes). Nombrar --policy lo implica: pedir una política concreta solo tiene
	// sentido sobre su propia lista.
	Shared bool
}

// ChainNoteKind clasifica los avisos que una mutación quiere que el usuario lea
// pero que no la invalidan.
type ChainNoteKind int

const (
	// ChainNotePrimaryImplicit: se metió en la cadena el perfil que YA es el
	// primario de este cwd. Se acepta —mañana el usuario mueve la regla y deja de
	// serlo, y entonces la entrada empieza a valer— pero ResolveAutoChain lo
	// filtra, así que HOY no hace nada y callarlo se leería como que sí.
	ChainNotePrimaryImplicit ChainNoteKind = iota

	// ChainNoteAlreadyInChain: el perfil YA estaba en `fallback`, así que el `add`
	// no tocó la cadena — lo único que hizo fue abrirle el gate. Sin esta nota, la
	// línea de fallback saldría idéntica a como estaba y parecería un no-op.
	ChainNoteAlreadyInChain

	// ChainNoteSelf: se metió un perfil en SU PROPIA cadena. A diferencia de
	// ChainNotePrimaryImplicit —que depende de dónde esté el cwd y mañana puede
	// dejar de cumplirse—, esto no vale nunca: nadie se presta la sesión a sí
	// mismo. Se acepta y se dice, en vez de rechazarlo, porque llegar aquí suele
	// ser un `--for` copiado y el error útil es saber que esa entrada no hará nada.
	ChainNoteSelf
)

// ChainNote es un aviso con el perfil al que se refiere.
type ChainNote struct {
	Kind    ChainNoteKind
	Profile string
}

// ChainResult es el parte de una mutación: qué quedó en cada clave y qué se
// tocó. El CLI lo imprime desglosado; no hay ningún campo que resuma «todo bien»
// a propósito.
type ChainResult struct {
	Policy   string   // política implicada (la mutada si Shared; la aplicable si no)
	Primary  string   // primario resuelto para Cwd
	Fallback []string // la lista `fallback` YA mutada (sin filtrar por allow_from)
	Removed  []string // perfiles sacados de la cadena (rm)
	Moved    string   // perfil recolocado (mv)
	MovedTo  int      // su nueva posición 1-based (mv)

	// --- el destino: qué clave del yaml se escribió --------------------------
	//
	// Owner/Shared no son adorno: son la diferencia entre haber tocado la cadena
	// de UN perfil y habérsela cambiado a todos, y el CLI está obligado a decir
	// cuál de las dos hizo. Una salida que solo dijera «fallback: a, b» ya no
	// identifica lo que cambió desde que hay más de una lista.

	Owner  string // perfil cuya cadena propia se escribió ("" => lista compartida)
	Shared bool   // se escribió policies[Policy].fallback
	Gate   string // perfil cuya entrada de allow_from se ajustó

	// Forked: este perfil HEREDABA y a partir de ahora no. Es la consecuencia
	// invisible de la primera mutación sobre una cadena que no existía, y la que
	// hay que contar en voz alta: desde aquí, los cambios de la lista compartida
	// dejan de llegarle.
	Forked    bool
	Inherited []string // la lista que deja de seguir (solo con Forked)

	// Reset: se borró la cadena propia y el perfil vuelve a heredar.
	Reset bool

	// PolicyBound / PolicyCleared: se ligó o se desligó una política del perfil.
	PolicyBound   string
	PolicyCleared bool

	// --- el desglose de allow_from, uno por estado posible del gate ---

	AllowEntry   []string // allow_from[Primary] tras la mutación (nil si no se tocó)
	AllowAdded   []string // qué se añadió a esa entrada (vacío si nada)
	AllowRemoved []string // qué se retiró de esa entrada (vacío si nada)
	AllowCreated bool     // la entrada NO existía y se creó (antes: deny total)
	GateAbsent   bool     // no hay gate declarado: allow_from no se tocó a propósito
	AllowSkipped bool     // --no-allow: el usuario pidió no tocarlo

	Notes []ChainNote
}

// ChainErrKind es el motivo por el que una mutación —o la RESOLUCIÓN— de la
// cadena se negó a seguir.
//
// Los kinds de abajo del corte cubren lo que antes devolvían `auto.go` y
// `editor.go` como `fmt.Errorf` en castellano crudo. Están aquí, y no en un tipo
// aparte, porque el consumidor es el mismo traductor del CLI: tener dos familias
// de error para la misma condición es exactamente cómo se acaba con `auto chain
// show` hablando español dentro de una sesión en inglés.
type ChainErrKind int

const (
	ChainErrNotConfigured ChainErrKind = iota // no hay bloque auto_handoff
	ChainErrNoPolicy                          // la política nombrada no existe
	ChainErrNoProfile                         // el perfil nombrado no existe
	ChainErrDuplicate                         // el perfil ya estaba en la cadena
	ChainErrNotInChain                        // rm/mv de un perfil que no está
	ChainErrRange                             // posición fuera de rango
	ChainErrEmpty                             // no se nombró ningún perfil

	// --- validación de la política (los devuelve Effective/ResolveAutoChain) ---

	ChainErrDisabled        // auto_handoff.enabled: false
	ChainErrFallbackProfile // fallback apunta a un perfil que no existe
	ChainErrThreshold       // threshold fuera de 1..100
	ChainErrMaxHops         // max_hops negativo
	ChainErrDuration        // duración que no parsea
	ChainErrDurationNeg     // duración sintácticamente válida pero negativa
	ChainErrCooldown        // cooldown.strategy desconocida

	// --- cadenas propias (auto_chains.go) ---

	ChainErrChainProfile // chains[<perfil>] apunta a un perfil que no existe
	ChainErrChainPolicy  // chains[<perfil>].policy nombra una política que no existe
	ChainErrNoChain      // el perfil no declara cadena propia (reset sobre lo que no hay)
	ChainErrTargetClash  // --for y --shared/--policy a la vez: dos destinos
)

// ChainError es un error tipado: el CLI lo traduce por Kind a los dos idiomas
// (errors.As) en vez de reenviar esta prosa. El texto de Error() existe igual
// porque un error que no se puede imprimir a secas es una trampa para el
// siguiente llamador.
type ChainError struct {
	Kind    ChainErrKind
	Policy  string
	Profile string
	Pos     int
	Min     int
	Max     int
	Detail  string // lista de contexto (políticas existentes, cadena actual)
	Owner   string // perfil dueño de la cadena propia implicada (chains[<owner>])
	Key     string // clave del yaml ofensiva (min_dwell, cooldown.fallback…)
	Value   string // su valor, tal cual lo escribió el usuario
	Num     int    // su valor cuando es numérico (threshold, max_hops)
	Cause   error  // el error de origen (time.ParseDuration), si lo hay
}

func (e *ChainError) Error() string {
	switch e.Kind {
	case ChainErrNotConfigured:
		return "auto_handoff no está configurado en ccp.yaml; corre `ccp auto init`"
	case ChainErrNoPolicy:
		return fmt.Sprintf("política %q no existe en auto_handoff.policies (hay: %s)", e.Policy, e.Detail)
	case ChainErrNoProfile:
		return fmt.Sprintf("el perfil %q no existe", e.Profile)
	case ChainErrDuplicate:
		return fmt.Sprintf("%q ya está en la cadena de la política %q", e.Profile, e.Policy)
	case ChainErrNotInChain:
		detail := e.Detail
		if detail == "" {
			detail = "cadena vacía"
		}
		return fmt.Sprintf("%q no está en la cadena de la política %q (hay: %s)", e.Profile, e.Policy, detail)
	case ChainErrRange:
		return fmt.Sprintf("posición %d fuera de rango (válido: %d..%d)", e.Pos, e.Min, e.Max)
	case ChainErrEmpty:
		return "hace falta nombrar al menos un perfil"
	case ChainErrDisabled:
		return "auto_handoff está deshabilitado (enabled: false en ccp.yaml); " +
			"ponlo en true o corre `ccp auto init --force`"
	case ChainErrFallbackProfile:
		return fmt.Sprintf("política %q: el perfil de fallback %q no existe", e.Policy, e.Profile)
	case ChainErrThreshold:
		return fmt.Sprintf("política %q: threshold %d fuera de rango (1..100)", e.Policy, e.Num)
	case ChainErrMaxHops:
		return fmt.Sprintf("política %q: max_hops %d no puede ser negativo", e.Policy, e.Num)
	case ChainErrDuration:
		return fmt.Sprintf("política %q: %s inválido (%q): %v", e.Policy, e.Key, e.Value, e.Cause)
	case ChainErrDurationNeg:
		return fmt.Sprintf("política %q: %s inválido (%q): no puede ser negativo", e.Policy, e.Key, e.Value)
	case ChainErrCooldown:
		return fmt.Sprintf("política %q: cooldown.strategy %q desconocida (usa %q o %q)",
			e.Policy, e.Value, CooldownResetsAt, CooldownFixed)
	case ChainErrChainProfile:
		return fmt.Sprintf("la cadena propia de %q apunta al perfil %q, que no existe", e.Owner, e.Profile)
	case ChainErrChainPolicy:
		return fmt.Sprintf("la cadena propia de %q liga la política %q, que no existe (hay: %s)",
			e.Owner, e.Policy, e.Detail)
	case ChainErrNoChain:
		return fmt.Sprintf("%q no declara cadena propia: ya hereda la de la política %q", e.Owner, e.Policy)
	case ChainErrTargetClash:
		return "--for y --shared/--policy nombran destinos distintos: elige uno"
	}
	return "cadena: error desconocido"
}

// Unwrap deja pasar el error de origen (time.ParseDuration) para errors.Is/As.
func (e *ChainError) Unwrap() error { return e.Cause }

// ChainAdd mete perfiles en la cadena y —salvo --no-allow— autoriza el préstamo
// desde el primario actual.
//
// Que toque DOS claves es la decisión con filo de este comando: quien escribe
// «añádelo a la cadena» quiere que el perfil SE USE, y un fallback sin su
// allow_from no se usa jamás, en silencio. Dejarlas separadas en la CLI
// reproduciría la trampa del yaml. Pero allow_from es un gate de CUMPLIMIENTO
// —puede estar puesto a propósito para que la cuenta del trabajo no le preste al
// proyecto personal—, así que el ensanche va con tres límites:
//
//  1. Se toca SOLO la entrada del primario actual. Nunca otras: un rewrite en
//     bloque ensancharía permisos de repos donde el usuario ni siquiera está.
//  2. El resultado dice exactamente qué cambió en cada clave, por separado
//     (ChainResult.AllowAdded / AllowCreated / GateAbsent / AllowSkipped).
//  3. --no-allow lo desactiva entero.
//
// Un perfil que YA está en `fallback` pero al que el gate le cierra el paso NO es
// un duplicado: es justo el estado que este comando existe para deshacer. Si se
// rechazara ahí (que es lo que hacía antes), un repo en deny total quedaría en un
// callejón sin salida — `show` diría «cadena: (ninguna)» y `add` respondería «ya
// está en la cadena» para TODOS los perfiles, y la única salida sería editar el
// yaml a mano, que es exactamente lo que el comando venía a evitar. Se acepta,
// se autoriza y se dice que la cadena no cambió (ChainNoteAlreadyInChain).
func ChainAdd(home string, opts ChainOpts, names []string) (ChainResult, error) {
	names, err := chainCleanNames(names)
	if err != nil {
		return ChainResult{}, err
	}
	cfg, tgt, list, err := chainBegin(home, opts)
	if err != nil {
		return ChainResult{}, err
	}
	ah := cfg.AutoHandoff

	// Validación COMPLETA antes de mutar, igual que `auto install`: un
	// `add bueno fantasma` que dejara `bueno` dentro y luego fallara obligaría al
	// usuario a deshacer a mano lo que el comando no llegó a terminar.
	var (
		insert  []string // los que sí entran en `fallback`
		already []string // los que ya estaban y solo se van a autorizar
	)
	for i, n := range names {
		if !autoProfileExists(cfg, n) {
			return ChainResult{}, &ChainError{Kind: ChainErrNoProfile, Profile: n}
		}
		// Repetido DENTRO de la misma invocación: eso siempre es un dedo pegado.
		if chainIndex(names[:i], n) >= 0 {
			return ChainResult{}, tgt.err(ChainErrDuplicate, n, "")
		}
		if chainIndex(list, n) < 0 {
			insert = append(insert, n)
			continue
		}
		// Ya está en la cadena: solo tiene sentido seguir si queda gate que abrir.
		if opts.NoAllow || !chainGateWouldOpen(ah, tgt.Gate, n) {
			return ChainResult{}, tgt.err(ChainErrDuplicate, n, "")
		}
		already = append(already, n)
	}

	// --at es 1-based y el límite superior es len+1 (insertar AL FINAL es una
	// posición legítima). Fuera de rango se rechaza nombrando el rango: un clamp
	// mudo dejaría la cadena en un orden que el usuario no pidió y que es justo lo
	// que el comando existe para controlar.
	at := len(list)
	if opts.At != 0 {
		if opts.At < 1 || opts.At > len(list)+1 {
			return ChainResult{}, &ChainError{Kind: ChainErrRange, Pos: opts.At, Min: 1, Max: len(list) + 1}
		}
		at = opts.At - 1
	}
	list = chainInsert(list, at, insert)

	res := tgt.result(list)
	chainSelfNotes(&res, tgt, names)
	for _, n := range already {
		res.Notes = append(res.Notes, ChainNote{Kind: ChainNoteAlreadyInChain, Profile: n})
	}
	chainGate(ah, tgt.Gate, names, nil, opts.NoAllow, &res)

	if err := chainFinish(home, cfg, tgt, list); err != nil {
		return ChainResult{}, err
	}
	return res, nil
}

// chainSelfNotes avisa de los nombres que no van a hacer nada en este destino.
// En una cadena propia es el dueño (nadie se presta a sí mismo, nunca); en la
// compartida es el primario del cwd, que HOY se filtra pero mañana puede dejar
// de ser el primario de esta carpeta — por eso son dos avisos y no uno.
func chainSelfNotes(res *ChainResult, tgt chainTarget, names []string) {
	kind := ChainNoteSelf
	subject := tgt.Owner
	if tgt.Shared {
		kind = ChainNotePrimaryImplicit
		subject = tgt.Primary
	}
	for _, n := range names {
		if n == subject {
			res.Notes = append(res.Notes, ChainNote{Kind: kind, Profile: n})
		}
	}
}

// ChainRm saca perfiles de la cadena y —salvo --no-allow— retira su autorización
// en la entrada del primario actual.
//
// Toca allow_from por la misma razón que `add`, girada: `add` es el único camino
// por CLI que ensancha el gate, así que si `rm` no lo estrecha el gate solo puede
// CRECER, y deshacer un `add` equivocado obliga a editar el yaml a mano — que es
// justo lo que estos comandos vinieron a evitar. Estrechar es además la dirección
// segura del cumplimiento: quitar un permiso nunca abre nada.
//
// Los tres límites del ensanche valen igual aquí: solo la entrada del primario
// actual, se reporta clave por clave (AllowRemoved) y --no-allow lo apaga.
//
// Un perfil que no está en la cadena aborta la operación entera nombrando lo que
// sí hay: un no-op silencioso deja al usuario creyendo que quitó algo.
func ChainRm(home string, opts ChainOpts, names []string) (ChainResult, error) {
	names, err := chainCleanNames(names)
	if err != nil {
		return ChainResult{}, err
	}
	cfg, tgt, list, err := chainBegin(home, opts)
	if err != nil {
		return ChainResult{}, err
	}
	for _, n := range names {
		if chainIndex(list, n) < 0 {
			return ChainResult{}, tgt.err(ChainErrNotInChain, n, chainDetail(list))
		}
	}

	out := make([]string, 0, len(list))
	for _, n := range list {
		if chainIndex(names, n) >= 0 {
			continue
		}
		out = append(out, n)
	}

	res := tgt.result(out)
	res.Removed = names
	chainGate(cfg.AutoHandoff, tgt.Gate, nil, names, opts.NoAllow, &res)
	if err := chainFinish(home, cfg, tgt, out); err != nil {
		return ChainResult{}, err
	}
	return res, nil
}

// ChainMv recoloca un perfil dentro de la cadena. Existe porque el ORDEN es la
// preferencia del usuario: `Next()` recorre el fallback de arriba abajo, así que
// mover es cambiar a quién se le presta primero.
//
// pos es 1-based sobre la lista RESULTANTE (mover el 1º a la posición 3 lo deja
// tercero de la misma lista, no cuarto).
func ChainMv(home string, opts ChainOpts, name string, pos int) (ChainResult, error) {
	one, err := chainCleanNames([]string{name})
	if err != nil {
		return ChainResult{}, err
	}
	name = one[0]

	cfg, tgt, list, err := chainBegin(home, opts)
	if err != nil {
		return ChainResult{}, err
	}
	i := chainIndex(list, name)
	if i < 0 {
		return ChainResult{}, tgt.err(ChainErrNotInChain, name, chainDetail(list))
	}
	if pos < 1 || pos > len(list) {
		return ChainResult{}, &ChainError{Kind: ChainErrRange, Pos: pos, Min: 1, Max: len(list)}
	}

	out := make([]string, 0, len(list))
	out = append(out, list[:i]...)
	out = append(out, list[i+1:]...)
	out = chainInsert(out, pos-1, []string{name})

	res := tgt.result(out)
	res.Moved, res.MovedTo = name, pos
	// mv no cambia QUIÉN está en la cadena, solo el orden, así que no hay nada que
	// autorizar ni que retirar. Se llama igual para que el resultado lleve el
	// estado del gate (--no-allow / sin gate) y el CLI pueda imprimir una línea de
	// allow_from también aquí: el silencio se lee como «no me he fijado».
	chainGate(cfg.AutoHandoff, tgt.Gate, nil, nil, opts.NoAllow, &res)
	if err := chainFinish(home, cfg, tgt, out); err != nil {
		return ChainResult{}, err
	}
	return res, nil
}

// ChainSet reemplaza la cadena entera.
//
// Sobre allow_from se comporta como el `rm` + `add` que de hecho es: autoriza a
// los que ENTRAN y retira a los que SALEN, siempre solo en la entrada del
// primario actual y siempre reportándolo. La alternativa —escribir `fallback` y
// no tocar el gate— es cómo `set` podía dejar la cadena INERTE con un `[ok]`
// delante: la lista nueva en el yaml, ningún destino autorizado, y el único
// indicio en la línea de «denegados» del bloque final.
//
// Exige al menos un perfil: `set` sin nombres es casi siempre un argumento que se
// quedó por el camino, y vaciar la cadena en silencio dejaría a `ccp session` sin
// ningún destino al que rotar. Vaciarla a propósito se escribe con `rm`.
func ChainSet(home string, opts ChainOpts, names []string) (ChainResult, error) {
	names, err := chainCleanNames(names)
	if err != nil {
		return ChainResult{}, err
	}
	cfg, tgt, list, err := chainBegin(home, opts)
	if err != nil {
		return ChainResult{}, err
	}
	for i, n := range names {
		if !autoProfileExists(cfg, n) {
			return ChainResult{}, &ChainError{Kind: ChainErrNoProfile, Profile: n}
		}
		if chainIndex(names[:i], n) >= 0 {
			return ChainResult{}, tgt.err(ChainErrDuplicate, n, "")
		}
	}

	// Los que se caen de la cadena: pierden la autorización igual que con `rm`.
	var gone []string
	for _, n := range list {
		if chainIndex(names, n) < 0 {
			gone = append(gone, n)
		}
	}

	res := tgt.result(names)
	chainSelfNotes(&res, tgt, names)
	chainGate(cfg.AutoHandoff, tgt.Gate, names, gone, opts.NoAllow, &res)
	if err := chainFinish(home, cfg, tgt, names); err != nil {
		return ChainResult{}, err
	}
	return res, nil
}

// chainGate aplica el ajuste ACOTADO de allow_from: autoriza a `add` y retira a
// `remove`, siempre y solo sobre la entrada del primario actual.
//
// Los tres estados del gate se tratan distinto A PROPÓSITO, y el primero es el
// que más fácil sería equivocar:
//
//   - mapa AUSENTE o VACÍO -> NO se toca. No hay gate, así que el préstamo ya
//     está permitido y no falta autorizar nada; pero además crear aquí la entrada
//     del primario CONVERTIRÍA la ausencia de gate en un gate declarado, y a
//     partir de ese momento todos los demás primarios (que siguen sin entrada)
//     pasarían a DENY TOTAL. Un `chain add` no puede apagarle la rotación al resto
//     de sus repos como efecto colateral.
//   - mapa DECLARADO CON entrada para el primario -> se le añade lo que falte y se
//     le quita lo que sobre, y solo a esa entrada.
//   - mapa DECLARADO SIN entrada para el primario -> hoy eso es deny total. Se
//     crea la entrada con EXACTAMENTE los perfiles añadidos: es el ensanche más
//     estrecho que cumple lo que el usuario pidió (que ese perfil se use) y deja
//     el resto de candidatos igual de bloqueados que estaban.
//
// El primario nunca entra ni sale de su propia entrada: no es candidato a
// préstamo de sí mismo, así que tocarlo sería ruido en el diff que el usuario va
// a leer. Y `add` gana a `remove` para el mismo nombre, que es lo que hace que
// `set a,b` sobre una cadena `[a]` no quite y vuelva a poner a `a`.
func chainGate(ah *AutoHandoff, primary string, add, remove []string, noAllow bool, res *ChainResult) {
	// `primary` es el DUEÑO de la entrada a ajustar: el primario del cwd cuando se
	// edita la lista compartida, y el perfil dueño cuando se edita su cadena
	// propia. Es el mismo criterio en los dos casos —la entrada que gobierna los
	// préstamos DESDE quien acaba de cambiar de cadena—, y por eso lo decide
	// chainBegin (tgt.Gate) y no cada llamador por su cuenta.
	if noAllow {
		res.AllowSkipped = true
		return
	}
	if len(ah.AllowFrom) == 0 {
		res.GateAbsent = true
		return
	}
	entry, declared := ah.AllowFrom[primary]
	created := !declared

	out := make([]string, 0, len(entry))
	for _, n := range entry {
		if n != primary && chainIndex(remove, n) >= 0 && chainIndex(add, n) < 0 {
			res.AllowRemoved = append(res.AllowRemoved, n)
			continue
		}
		out = append(out, n)
	}
	entry = out

	for _, n := range add {
		if n == primary || chainIndex(entry, n) >= 0 {
			continue
		}
		entry = append(entry, n)
		res.AllowAdded = append(res.AllowAdded, n)
	}

	if len(res.AllowAdded) == 0 && len(res.AllowRemoved) == 0 {
		// Nada que escribir. Importa no caer aquí en el estado «entrada creada»:
		// escribir `primario: []` no cambiaría nada y dejaría en el yaml una
		// entrada vacía que se lee como una decisión que nadie tomó.
		return
	}
	ah.AllowFrom[primary] = entry
	res.AllowEntry = entry
	res.AllowCreated = created
}

// chainGateWouldOpen dice si autorizar a `name` desde `primary` cambiaría algo.
//
// Es lo que distingue un duplicado de verdad («ya está en la cadena Y ya está
// autorizado») del repo atascado en deny total, donde el perfil está en la lista
// y aun así no se usa nunca.
func chainGateWouldOpen(ah *AutoHandoff, primary, name string) bool {
	if len(ah.AllowFrom) == 0 || name == primary {
		return false // sin gate no hay nada que abrir
	}
	entry, declared := ah.AllowFrom[primary]
	if !declared {
		return true // deny total: crear la entrada es la única salida
	}
	return chainIndex(entry, name) < 0
}

// chainTarget es A QUÉ CLAVE del yaml apunta esta mutación. Existe porque desde
// que hay cadenas por perfil «la cadena» ya no identifica nada: hay una por
// perfil más la compartida de cada política, y escribir en la que no era es
// exactamente el fallo que no se ve hasta que la rotación salta mal a las 3am.
//
// El destino se decide UNA vez, en chainBegin, y viaja hasta chainFinish y hasta
// el parte que se imprime. Que las tres cosas —de dónde se lee, dónde se escribe
// y qué se cuenta— salgan del mismo valor es lo que impide que el mensaje diga
// una cosa y el archivo acabe con otra.
type chainTarget struct {
	Shared  bool   // se edita policies[Policy].fallback
	Owner   string // perfil dueño de la cadena propia (vacío si Shared)
	Policy  string // política implicada
	Primary string // primario del cwd (contexto del parte)
	Gate    string // perfil cuya entrada de allow_from se ajusta

	// Fork/Inherited: la cadena propia NO existía y esta mutación la crea a
	// partir de la heredada.
	Fork      bool
	Inherited []string

	pol   AutoPolicy // la política cargada (solo si Shared)
	entry AutoChain  // la entrada actual de chains[Owner] (cero si no había)
}

// result siembra el parte con el destino ya decidido.
func (t chainTarget) result(list []string) ChainResult {
	return ChainResult{
		Policy: t.Policy, Primary: t.Primary, Fallback: list,
		Owner: t.Owner, Shared: t.Shared, Gate: t.Gate,
		Forked: t.Fork, Inherited: t.Inherited,
	}
}

// err completa un error con las coordenadas del destino, para que el CLI pueda
// decir «en la cadena de a-cc» o «en la política default» sin adivinarlo.
func (t chainTarget) err(kind ChainErrKind, profile, detail string) *ChainError {
	e := &ChainError{Kind: kind, Profile: profile, Policy: t.Policy, Detail: detail}
	if !t.Shared {
		e.Owner = t.Owner
	}
	return e
}

// chainBegin carga la config, decide el destino y devuelve la lista a mutar.
//
// Devuelve también el primario del cwd (core.Resolve, el MISMO que usa
// ResolveAutoChain: reimplementar la resolución aquí sería tener dos ideas
// distintas de «dónde estoy»).
//
// NO exige `enabled: true`: editar la cadena con la rotación apagada es legítimo
// —se configura primero y se enciende después— y negarse ahí obligaría a
// habilitar el auto-handoff solo para poder preparar su política.
//
// La SIEMBRA desde la lista heredada (Fork) es la decisión con filo. Un perfil
// que hereda y recibe su primer `add` podría: (a) arrancar con una cadena de un
// solo nombre, o (b) quedarse con la heredada más el nuevo. Se elige (b) porque
// (a) convierte «añade uno» en «quita todos los demás», que es justo lo contrario
// de lo que se pidió. El precio es que a partir de ese momento el perfil deja de
// seguir la lista compartida, y por eso Fork viaja hasta la salida: bifurcar en
// silencio sería dejar una herencia rota que no se nota hasta meses después.
func chainBegin(home string, opts ChainOpts) (*Config, chainTarget, []string, error) {
	cfg, err := Load(home)
	if err != nil {
		return nil, chainTarget{}, nil, err
	}
	ah := cfg.AutoHandoff
	if ah == nil {
		return nil, chainTarget{}, nil, &ChainError{Kind: ChainErrNotConfigured}
	}

	forName := strings.TrimSpace(opts.For)
	policyName := strings.TrimSpace(opts.Policy)
	shared := opts.Shared || policyName != ""
	if forName != "" && shared {
		// Dos destinos a la vez no se resuelve eligiendo uno: el usuario nombró
		// las dos cosas y cualquiera de las dos que se escriba va a sorprenderle.
		return nil, chainTarget{}, nil, &ChainError{Kind: ChainErrTargetClash}
	}
	primary := Resolve(opts.Cwd, cfg.Rules)

	if shared {
		if policyName == "" {
			policyName = "default"
		}
		pol, ok := ah.Policies[policyName]
		if !ok {
			return nil, chainTarget{}, nil, &ChainError{
				Kind: ChainErrNoPolicy, Policy: policyName, Detail: autoPolicyNames(ah.Policies)}
		}
		t := chainTarget{Shared: true, Policy: policyName, Primary: primary, Gate: primary, pol: pol}
		return cfg, t, chainClean(pol.Fallback), nil
	}

	owner := forName
	if owner == "" {
		owner = primary
	}
	if !autoProfileExists(cfg, owner) {
		return nil, chainTarget{}, nil, &ChainError{Kind: ChainErrNoProfile, Profile: owner}
	}
	pc := AutoChainFor(cfg, owner)
	if pc.Policy != "" {
		if _, ok := ah.Policies[pc.Policy]; !ok {
			return nil, chainTarget{}, nil, &ChainError{
				Kind: ChainErrChainPolicy, Policy: pc.Policy, Owner: owner,
				Detail: autoPolicyNames(ah.Policies)}
		}
	}
	t := chainTarget{
		Owner: owner, Policy: autoPolicyOr(pc.Policy), Primary: primary,
		Gate: owner, entry: ah.Chains[owner],
	}
	if pc.Declared {
		return cfg, t, pc.Fallback, nil
	}

	pol, ok := ah.Policies[t.Policy]
	if !ok {
		return nil, chainTarget{}, nil, &ChainError{
			Kind: ChainErrNoPolicy, Policy: t.Policy, Detail: autoPolicyNames(ah.Policies)}
	}
	t.Fork = true
	t.Inherited = chainClean(pol.Fallback)
	return cfg, t, append([]string(nil), t.Inherited...), nil
}

// chainFinish persiste el destino por el camino normal del core (Save: tmp+rename
// bajo flock, conservando comentarios y Config.Extra, sin tocar la version del
// esquema).
//
// Las reasignaciones al mapa NO son un descuido: Policies y Chains son mapas por
// VALOR, así que `Policies[n].Fallback = x` ni siquiera compila — hay que copiar,
// mutar y volver a meter.
//
// La entrada se escribe SIEMPRE con la cadena declarada (NewAutoChain / hasFallback):
// guardar una lista sin declarar la clave dejaría un `chains[<perfil>]` que al
// releerse significa «hereda», o sea la escritura se perdería en silencio.
func chainFinish(home string, cfg *Config, t chainTarget, list []string) error {
	ah := cfg.AutoHandoff
	if t.Shared {
		pol := t.pol
		pol.Fallback = list
		ah.Policies[t.Policy] = pol
		return Save(home, cfg)
	}
	entry := t.entry
	entry.Fallback = list
	entry.hasFallback = true
	if ah.Chains == nil {
		ah.Chains = map[string]AutoChain{}
	}
	ah.Chains[t.Owner] = entry
	return Save(home, cfg)
}

// ChainReset borra la cadena propia de un perfil: vuelve a heredar la de su
// política, y vuelve a recibir los cambios de esa lista compartida.
//
// Es la vuelta atrás de la bifurcación, y hace falta justamente porque la
// bifurcación es fácil de provocar sin querer (el primer `add` dentro de un
// repo). Sin este comando, deshacerla obligaba a editar el yaml a mano.
//
// NO toca allow_from. Quitar la cadena propia no retira permisos: el gate se
// declaró aparte y puede estar puesto a propósito, y estrecharlo aquí sería
// adivinar. Lo que sí hace es decir con qué cadena se queda el perfil.
func ChainReset(home string, opts ChainOpts) (ChainResult, error) {
	if opts.Shared || strings.TrimSpace(opts.Policy) != "" {
		return ChainResult{}, &ChainError{Kind: ChainErrTargetClash}
	}
	cfg, t, _, err := chainBegin(home, opts)
	if err != nil {
		return ChainResult{}, err
	}
	ah := cfg.AutoHandoff
	pc := AutoChainFor(cfg, t.Owner)
	if !pc.Entry {
		return ChainResult{}, &ChainError{Kind: ChainErrNoChain, Owner: t.Owner, Policy: t.Policy}
	}

	delete(ah.Chains, t.Owner)
	if len(ah.Chains) == 0 {
		// El mapa vacío se borra para que la clave desaparezca del yaml en vez de
		// quedarse como un `chains: {}` que se lee como una decisión.
		ah.Chains = nil
	}

	// La política aplicable vuelve a ser la del camino normal: la ligadura se va
	// con la entrada, así que el parte tiene que recalcularla o diría que el
	// perfil hereda la cadena de una política que ya no le aplica.
	polName := "default"
	pol, ok := ah.Policies[polName]
	if !ok {
		return ChainResult{}, &ChainError{
			Kind: ChainErrNoPolicy, Policy: polName, Detail: autoPolicyNames(ah.Policies)}
	}
	res := ChainResult{
		Policy: polName, Primary: t.Primary, Owner: t.Owner, Gate: t.Gate,
		Fallback: chainClean(pol.Fallback), Reset: true,
	}
	chainGate(ah, t.Gate, nil, nil, opts.NoAllow, &res)
	if err := Save(home, cfg); err != nil {
		return ChainResult{}, err
	}
	return res, nil
}

// ChainPolicySet liga (o desliga) una política a un perfil: `chains[<p>].policy`.
//
// Ligar una política a un perfil que HEREDA le cambia también la cadena, porque
// la hereda de la política ligada y no de `default`. Eso no se puede evitar —es
// lo que significa ligar— pero sí se puede contar: el resultado trae la cadena
// que queda en vigor, y el CLI la imprime. Lo que NO se hace es declarar de paso
// una cadena propia vacía: escribir `{policy: x}` sin `fallback` deja al perfil
// heredando, que es lo que el usuario pidió, mientras que forzar la clave le
// apagaría la rotación con un `[ok]` delante.
func ChainPolicySet(home string, opts ChainOpts, policy string, clear bool) (ChainResult, error) {
	if opts.Shared || strings.TrimSpace(opts.Policy) != "" {
		return ChainResult{}, &ChainError{Kind: ChainErrTargetClash}
	}
	policy = strings.TrimSpace(policy)
	if !clear && policy == "" {
		return ChainResult{}, &ChainError{Kind: ChainErrEmpty}
	}
	cfg, t, _, err := chainBegin(home, opts)
	if err != nil {
		return ChainResult{}, err
	}
	ah := cfg.AutoHandoff
	if !clear {
		if _, ok := ah.Policies[policy]; !ok {
			return ChainResult{}, &ChainError{
				Kind: ChainErrNoPolicy, Policy: policy, Detail: autoPolicyNames(ah.Policies)}
		}
	}

	entry := ah.Chains[t.Owner]
	if clear {
		entry.Policy = ""
	} else {
		entry.Policy = policy
		entry.long = true
	}

	if entry.Policy == "" && !entry.hasFallback && len(entry.Extra) == 0 {
		// Entrada que ya no dice nada: se borra en vez de dejar un `a-cc: {}` en
		// el yaml, que no significa nada y parece un resto.
		delete(ah.Chains, t.Owner)
		if len(ah.Chains) == 0 {
			ah.Chains = nil
		}
	} else {
		if ah.Chains == nil {
			ah.Chains = map[string]AutoChain{}
		}
		ah.Chains[t.Owner] = entry
	}

	polName := autoPolicyOr(entry.Policy)
	res := ChainResult{
		Policy: polName, Primary: t.Primary, Owner: t.Owner, Gate: t.Gate,
		PolicyBound: policy, PolicyCleared: clear,
	}
	if entry.hasFallback {
		res.Fallback = chainClean(entry.Fallback)
	} else {
		res.Fallback = chainClean(ah.Policies[polName].Fallback)
	}
	chainGate(ah, t.Gate, nil, nil, opts.NoAllow, &res)
	if err := Save(home, cfg); err != nil {
		return ChainResult{}, err
	}
	return res, nil
}

// chainCleanNames normaliza los nombres que llegan del CLI y exige al menos uno.
func chainCleanNames(names []string) ([]string, error) {
	out := make([]string, 0, len(names))
	for _, n := range names {
		if n = strings.TrimSpace(n); n != "" {
			out = append(out, n)
		}
	}
	if len(out) == 0 {
		return nil, &ChainError{Kind: ChainErrEmpty}
	}
	return out, nil
}

// chainIndex es el índice de want en list, o -1.
func chainIndex(list []string, want string) int {
	for i, s := range list {
		if s == want {
			return i
		}
	}
	return -1
}

// chainInsert mete items en la posición at (0-based, at==len => al final).
func chainInsert(list []string, at int, items []string) []string {
	out := make([]string, 0, len(list)+len(items))
	out = append(out, list[:at]...)
	out = append(out, items...)
	out = append(out, list[at:]...)
	return out
}

// chainDetail describe la cadena actual para el contexto de un error. Devuelve
// "" con la cadena vacía en vez de una frase: el CLI la traduce a «(ninguno)» en
// el idioma del usuario, y meter prosa castellana aquí la colaría en la salida
// inglesa.
func chainDetail(list []string) string {
	return strings.Join(list, ", ")
}
