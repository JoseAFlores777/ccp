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
	Policy  string // "" => "default"
	Cwd     string // desde dónde se resuelve el primario (core.Resolve)
	At      int    // solo add: posición 1-based donde insertar; 0 = al final
	NoAllow bool   // solo add: NO tocar allow_from
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
	Policy   string   // política mutada
	Primary  string   // primario resuelto para Cwd
	Fallback []string // la lista `fallback` YA mutada (sin filtrar por allow_from)
	Removed  []string // perfiles sacados de la cadena (rm)
	Moved    string   // perfil recolocado (mv)
	MovedTo  int      // su nueva posición 1-based (mv)

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
	cfg, pol, policyName, primary, err := chainBegin(home, opts.Policy, opts.Cwd)
	if err != nil {
		return ChainResult{}, err
	}
	list := chainList(pol)
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
			return ChainResult{}, &ChainError{Kind: ChainErrDuplicate, Profile: n, Policy: policyName}
		}
		if chainIndex(list, n) < 0 {
			insert = append(insert, n)
			continue
		}
		// Ya está en la cadena: solo tiene sentido seguir si queda gate que abrir.
		if opts.NoAllow || !chainGateWouldOpen(ah, primary, n) {
			return ChainResult{}, &ChainError{Kind: ChainErrDuplicate, Profile: n, Policy: policyName}
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

	res := ChainResult{Policy: policyName, Primary: primary, Fallback: list}
	for _, n := range names {
		if n == primary {
			res.Notes = append(res.Notes, ChainNote{Kind: ChainNotePrimaryImplicit, Profile: n})
		}
	}
	for _, n := range already {
		res.Notes = append(res.Notes, ChainNote{Kind: ChainNoteAlreadyInChain, Profile: n})
	}
	chainGate(ah, primary, names, nil, opts.NoAllow, &res)

	if err := chainFinish(home, cfg, policyName, pol, list); err != nil {
		return ChainResult{}, err
	}
	return res, nil
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
	cfg, pol, policyName, primary, err := chainBegin(home, opts.Policy, opts.Cwd)
	if err != nil {
		return ChainResult{}, err
	}
	list := chainList(pol)
	for _, n := range names {
		if chainIndex(list, n) < 0 {
			return ChainResult{}, &ChainError{
				Kind: ChainErrNotInChain, Profile: n, Policy: policyName, Detail: chainDetail(list)}
		}
	}

	out := make([]string, 0, len(list))
	for _, n := range list {
		if chainIndex(names, n) >= 0 {
			continue
		}
		out = append(out, n)
	}

	res := ChainResult{Policy: policyName, Primary: primary, Fallback: out, Removed: names}
	chainGate(cfg.AutoHandoff, primary, nil, names, opts.NoAllow, &res)
	if err := chainFinish(home, cfg, policyName, pol, out); err != nil {
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

	cfg, pol, policyName, primary, err := chainBegin(home, opts.Policy, opts.Cwd)
	if err != nil {
		return ChainResult{}, err
	}
	list := chainList(pol)
	i := chainIndex(list, name)
	if i < 0 {
		return ChainResult{}, &ChainError{
			Kind: ChainErrNotInChain, Profile: name, Policy: policyName, Detail: chainDetail(list)}
	}
	if pos < 1 || pos > len(list) {
		return ChainResult{}, &ChainError{Kind: ChainErrRange, Pos: pos, Min: 1, Max: len(list)}
	}

	out := make([]string, 0, len(list))
	out = append(out, list[:i]...)
	out = append(out, list[i+1:]...)
	out = chainInsert(out, pos-1, []string{name})

	res := ChainResult{Policy: policyName, Primary: primary, Fallback: out, Moved: name, MovedTo: pos}
	// mv no cambia QUIÉN está en la cadena, solo el orden, así que no hay nada que
	// autorizar ni que retirar. Se llama igual para que el resultado lleve el
	// estado del gate (--no-allow / sin gate) y el CLI pueda imprimir una línea de
	// allow_from también aquí: el silencio se lee como «no me he fijado».
	chainGate(cfg.AutoHandoff, primary, nil, nil, opts.NoAllow, &res)
	if err := chainFinish(home, cfg, policyName, pol, out); err != nil {
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
	cfg, pol, policyName, primary, err := chainBegin(home, opts.Policy, opts.Cwd)
	if err != nil {
		return ChainResult{}, err
	}
	for i, n := range names {
		if !autoProfileExists(cfg, n) {
			return ChainResult{}, &ChainError{Kind: ChainErrNoProfile, Profile: n}
		}
		if chainIndex(names[:i], n) >= 0 {
			return ChainResult{}, &ChainError{Kind: ChainErrDuplicate, Profile: n, Policy: policyName}
		}
	}

	// Los que se caen de la cadena: pierden la autorización igual que con `rm`.
	var gone []string
	for _, n := range chainList(pol) {
		if chainIndex(names, n) < 0 {
			gone = append(gone, n)
		}
	}

	res := ChainResult{Policy: policyName, Primary: primary, Fallback: names}
	for _, n := range names {
		if n == primary {
			res.Notes = append(res.Notes, ChainNote{Kind: ChainNotePrimaryImplicit, Profile: n})
		}
	}
	chainGate(cfg.AutoHandoff, primary, names, gone, opts.NoAllow, &res)
	if err := chainFinish(home, cfg, policyName, pol, names); err != nil {
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

// chainBegin carga la config y localiza la política a mutar. Devuelve también el
// primario del cwd (core.Resolve, el MISMO que usa ResolveAutoChain: reimplementar
// la resolución aquí sería tener dos ideas distintas de «dónde estoy»).
//
// NO exige `enabled: true`: editar la cadena con la rotación apagada es legítimo
// —se configura primero y se enciende después— y negarse ahí obligaría a
// habilitar el auto-handoff solo para poder preparar su política.
func chainBegin(home, policyName, cwd string) (*Config, AutoPolicy, string, string, error) {
	cfg, err := Load(home)
	if err != nil {
		return nil, AutoPolicy{}, "", "", err
	}
	ah := cfg.AutoHandoff
	if ah == nil {
		return nil, AutoPolicy{}, "", "", &ChainError{Kind: ChainErrNotConfigured}
	}
	if policyName == "" {
		policyName = "default"
	}
	pol, ok := ah.Policies[policyName]
	if !ok {
		return nil, AutoPolicy{}, "", "", &ChainError{
			Kind: ChainErrNoPolicy, Policy: policyName, Detail: autoPolicyNames(ah.Policies)}
	}
	return cfg, pol, policyName, Resolve(cwd, cfg.Rules), nil
}

// chainFinish persiste la política mutada por el camino normal del core (Save:
// tmp+rename bajo flock, conservando comentarios y Config.Extra, sin tocar la
// version del esquema).
//
// La reasignación al mapa NO es un descuido: Policies es map[string]AutoPolicy
// por VALOR, así que `Policies[n].Fallback = x` ni siquiera compila — hay que
// copiar, mutar y volver a meter.
func chainFinish(home string, cfg *Config, policyName string, pol AutoPolicy, list []string) error {
	pol.Fallback = list
	cfg.AutoHandoff.Policies[policyName] = pol
	return Save(home, cfg)
}

// chainList devuelve la cadena de la política como copia saneada: sin espacios y
// sin entradas vacías (`fallback: [a, , b]` es un typo tan común como silencioso,
// y Effective ya las descarta al resolver). Es copia para que un fallo a mitad de
// validación no deje el Config en memoria a medio mutar.
func chainList(pol AutoPolicy) []string {
	out := make([]string, 0, len(pol.Fallback))
	for _, n := range pol.Fallback {
		if n = strings.TrimSpace(n); n != "" {
			out = append(out, n)
		}
	}
	return out
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
