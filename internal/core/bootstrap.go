package core

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// bootstrap.go — la DETECCIÓN de lo que le falta a un repo para que
// `ccp session` pueda rotar perfiles, y la aplicación de ese plan.
//
// La detección es PURA: recibe el Config y el contexto (cwd, raíz git, perfil
// activo) y devuelve DATOS, nunca texto. Eso es lo que permite probar los cuatro
// huecos sin tty, sin prompt y sin escribir un byte — que es justo la invariante
// que este archivo existe para sostener, porque el consumidor es un comando que
// también corre desde cron.
//
// La aplicación NO inventa rutas de escritura: cada hueco se cierra llamando a
// la MISMA función que usa el comando que el usuario podría haber tecleado
// (AutoInit ≙ `ccp auto init`, RuleSet ≙ `ccp path set`, ChainAdd ≙ `ccp auto
// chain add`, hooks+Save+ProfileSync ≙ `ccp auto install`). Si algún día uno de
// esos comandos cambia de semántica, el bootstrap cambia con él en vez de
// quedarse con una copia divergente.

// BootstrapKind identifica QUÉ se comprueba. Es string y no int para que el
// valor sobreviva legible en el JSON de la caché: un `applied: ["rule"]` se
// entiende sin el binario delante, y un kind nuevo no renumera a los viejos.
type BootstrapKind string

const (
	// BootstrapAuto: el bloque auto_handoff de ccp.yaml.
	BootstrapAuto BootstrapKind = "auto"
	// BootstrapRule: una regla de path que cubra el cwd.
	BootstrapRule BootstrapKind = "rule"
	// BootstrapChain: que quede algún préstamo posible tras el gate allow_from.
	BootstrapChain BootstrapKind = "chain"
	// BootstrapSensors: la capa de sensores en el primario y en la cadena.
	BootstrapSensors BootstrapKind = "sensors"
)

// BootstrapItem es UN chequeo ya resuelto. Lleva el resultado (Missing) y los
// datos que el front-end necesita para describirlo, nunca la descripción: la
// prosa vive en el catálogo i18n de internal/cli, en los dos idiomas.
type BootstrapItem struct {
	Kind    BootstrapKind
	Missing bool

	// Blocked marca un hueco REAL que hoy no se puede ofrecer porque depende de
	// otro que el bootstrap no sabe cerrar solo. Hoy solo lo usa la cadena: sin
	// saber quién va a ser el primario, ensanchar `allow_from` escribiría el gate
	// de cumplimiento del perfil equivocado (ver §3 en BootstrapDetect). No cuenta
	// como Missing —no se aplica— pero el front-end tiene que poder decirlo en vez
	// de pintar un «(ok)» que no es verdad.
	Blocked bool

	// --- BootstrapRule ---
	Path    string // ruta donde iría la regla (raíz git, o cwd si no hay repo)
	FromGit bool   // Path salió de `git rev-parse --show-toplevel`
	GitRoot string // la raíz git que dio el llamador; "" = no estamos en un repo
	Profile string // perfil destino; "" = no se pudo deducir (no se aplica)

	// --- BootstrapChain ---
	Policy string // política resuelta

	// --- BootstrapChain / BootstrapSensors ---
	//
	// En chain: los candidatos que allow_from bloqueó. En sensors: los perfiles
	// a los que falta instalarles la capa (o, si el chequeo salió bien, los que
	// ya la tienen), para que el resumen pueda nombrarlos.
	Profiles []string
}

// BootstrapPlan es el parte completo: los cuatro chequeos en orden, más el
// contexto con el que se resolvieron.
type BootstrapPlan struct {
	// Repo es la clave de la caché de «preguntar una vez»: la raíz git, o el cwd
	// si no estamos en un repo. Ya normalizada.
	Repo string
	Cwd  string
	// Primary es el perfil que gobernaría el cwd DESPUÉS de aplicar el plan (con
	// la regla propuesta ya contada), no el de ahora: es el que el usuario va a
	// ver en el resumen y el que decide a quién le hacen falta sensores.
	Primary string
	Items   []BootstrapItem
}

// Gaps son los chequeos que fallaron, en el orden en que hay que aplicarlos.
func (p BootstrapPlan) Gaps() []BootstrapItem {
	out := make([]BootstrapItem, 0, len(p.Items))
	for _, it := range p.Items {
		if it.Missing {
			out = append(out, it)
		}
	}
	return out
}

// HasGaps reporta si hay algo que ofrecer.
func (p BootstrapPlan) HasGaps() bool {
	for _, it := range p.Items {
		if it.Missing {
			return true
		}
	}
	return false
}

// BootstrapInput es el contexto que la detección no puede averiguar por sí
// misma sin dejar de ser pura: el cwd, la raíz git (que sale de un exec) y el
// perfil activo de la terminal (que sale del entorno).
type BootstrapInput struct {
	Cwd    string // directorio desde el que se lanzó `ccp session`
	Repo   string // raíz del repo git; "" si no estamos en uno
	Active string // $CCP_PROFILE de esta terminal; "" o "default" = ninguno
	Policy string // política pedida con --policy; "" = "default"
}

// BootstrapDetect resuelve los cuatro chequeos. Pura: no lee ni escribe disco.
//
// La clave del diseño es que SIMULA el plan mientras lo construye. El bloque
// auto_handoff que sembraría AutoInit y la regla que crearía RuleSet se aplican
// sobre una copia en memoria ANTES de resolver la cadena, así que las filas de
// «cadena» y «sensores» describen el estado en el que quedará el repo, no el de
// ahora. Sin eso, en un repo virgen las dos últimas filas se calcularían con
// primario `default` y cadena vacía, y el resumen mentiría en el único momento
// en que el usuario lo lee.
//
// El error que devuelve es el de ResolveAutoChain (política inexistente,
// duración mal escrita, fallback a un perfil que no existe): son fallos de
// ccp.yaml que el bootstrap no sabe arreglar, y el llamador debe saltarse el
// bootstrap entero — el propio `ccp session` los va a reportar acto seguido con
// su traductor, que es el mismo.
func BootstrapDetect(cfg *Config, in BootstrapInput) (BootstrapPlan, error) {
	cwd := NormalizePath(in.Cwd)
	repo := NormalizePath(strings.TrimSpace(in.Repo))
	fromGit := repo != ""

	// La raíz git solo vale si de verdad CUBRE el cwd con el que se resuelven las
	// reglas: escribir la regla sobre una ruta que Resolve nunca va a casar deja
	// una regla inerte en el yaml y un repo que sigue sin enrutar.
	//
	// El caso que hace falta distinguir aquí es el del SYMLINK: `git rev-parse
	// --show-toplevel` devuelve la ruta FÍSICA y el cwd de ccp es la LÓGICA ($PWD,
	// sin resolver enlaces), así que en macOS son dos cadenas distintas para el
	// mismo directorio en cuanto hay un /var o un /tmp por medio. Re-anclar la
	// raíz en la ruta lógica es trabajo del llamador (necesita disco, y esta
	// función es pura); lo que queda aquí es la RED: si aun así no cubre el cwd,
	// la regla cae sobre el cwd —peor sitio, pero funciona— y el resumen tiene que
	// poder decir por qué. Por eso se conserva GitRoot: «no es un repo git» y «la
	// raíz del repo no cubre este directorio» son dos frases distintas, y afirmar
	// la primera cuando es la segunda es mentirle al usuario en la única línea que
	// lee para decidir.
	gitRoot := repo
	if fromGit && !ruleIsAncestor(repo, cwd) {
		fromGit = false
	}
	if !fromGit {
		repo = cwd
	}

	plan := BootstrapPlan{Repo: repo, Cwd: cwd}

	// --- 1. el bloque auto_handoff ------------------------------------------
	//
	// `enabled: false` NO cuenta como hueco: apagar la rotación es un acto
	// explícito del usuario, y un asistente que la vuelve a encender borra esa
	// decisión sin preguntar por ella (el prompt dice «configurar», no
	// «reactivar»). Ese caso lo sigue reportando `ccp session` con su mensaje de
	// siempre, que ya nombra cómo revertirlo.
	ah := cfg.AutoHandoff
	auto := BootstrapItem{Kind: BootstrapAuto}
	if ah == nil {
		auto.Missing = true
		ah = newAutoHandoff(cfg)
	}
	plan.Items = append(plan.Items, auto)

	// --- 2. la regla de path -------------------------------------------------
	rules := cfg.Rules
	rule := BootstrapItem{Kind: BootstrapRule, Path: repo, FromGit: fromGit, GitRoot: gitRoot}
	if rulesCover(cwd, rules) {
		rule.Profile = Resolve(cwd, rules)
	} else {
		rule.Missing = true
		rule.Profile = bootstrapRuleProfile(cfg, in.Active)
		if rule.Profile != "" {
			// Copia: el Config del llamador no se toca. La regla propuesta va
			// sobre la RAÍZ del repo, no sobre el cwd — una regla en un
			// subdirectorio es casi siempre un error de dedo, y luego confunde
			// porque el perfil cambia al hacer `cd ..`.
			rules = append(append(make([]Rule, 0, len(rules)+1), rules...),
				Rule{Path: repo, Profile: rule.Profile})
		}
	}
	plan.Items = append(plan.Items, rule)

	// primaryKnown: el primario del plan es el de DESPUÉS de aplicarlo, y solo se
	// sabe si ya hay regla o si la propuesta se va a poder escribir. Con un hueco
	// de regla que no se puede cerrar, todo lo que dependa del primario queda en
	// el aire — y `allow_from` es lo que más depende de él.
	primaryKnown := !rule.Missing || rule.Profile != ""

	// --- 3. la cadena tras el gate -------------------------------------------
	sim := &Config{Version: cfg.Version, Profiles: cfg.Profiles, Rules: rules, AutoHandoff: ah}
	rc, err := ResolveAutoChain("", sim, in.Policy, cwd)
	if err != nil {
		return BootstrapPlan{}, err
	}
	plan.Primary = rc.Primary

	chain := BootstrapItem{Kind: BootstrapChain, Policy: rc.Policy.Name}
	// Solo es hueco si el gate se comió TODO lo que había. Una cadena vacía sin
	// denegados no es un problema que ensanchar allow_from resuelva: es que no
	// hay más perfiles, y ofrecer «ensanchar» ahí sería ofrecer un no-op.
	loans := rc.Fallback
	if len(rc.Fallback) == 0 && len(rc.Denied) > 0 {
		if primaryKnown {
			chain.Missing = true
			// Tras ensanchar, los denegados SON la cadena: los sensores se calculan
			// sobre ella para que las dos filas del resumen sean coherentes entre sí.
			loans = rc.Denied
		} else {
			// El primario NO se sabe: la regla es un hueco que el bootstrap no
			// puede cerrar (varios perfiles y ninguno activo), así que
			// ResolveAutoChain ha resuelto contra `default` —el ~/.claude llano— y
			// ensanchar ahí escribiría una entrada de allow_from para un primario
			// que el usuario todavía no ha elegido: autorizarle préstamos hacia
			// terceros desde su login de siempre, sin que nadie lo haya pedido.
			// El hueco se ENSEÑA (Blocked) y se aplaza hasta que exista la regla.
			chain.Blocked = true
		}
		chain.Profiles = rc.Denied
	}
	plan.Items = append(plan.Items, chain)

	// --- 4. los sensores ------------------------------------------------------
	//
	// Hacen falta en el primario (es quien va a medir su propio consumo) y en
	// cada miembro de la cadena (cuando la sesión aterrice ahí, la barra tiene
	// que seguir midiendo). `default` queda fuera: no tiene cc-home propio, así
	// que `ccp auto install default` es un error, no una omisión.
	sensors := BootstrapItem{Kind: BootstrapSensors}
	var have, want []string
	for _, n := range append([]string{rc.Primary}, loans...) {
		if n == "" || n == "default" || autoSeen(want, n) || autoSeen(have, n) {
			continue
		}
		if AutoHooksEnabled(sim, n) {
			have = append(have, n)
			continue
		}
		want = append(want, n)
	}
	if len(want) > 0 {
		sensors.Missing = true
		sensors.Profiles = want
	} else {
		sensors.Profiles = have
	}
	plan.Items = append(plan.Items, sensors)

	return plan, nil
}

// bootstrapRuleProfile elige el perfil de la regla propuesta.
//
// Solo hay dos fuentes honestas, y ninguna adivina:
//
//   - el perfil ACTIVO de esta terminal ($CCP_PROFILE): el usuario ya eligió, y
//     lo más probable es que quiera fijar esa elección para el repo;
//   - si solo existe un perfil, es el único candidato posible.
//
// Con varios perfiles y sin uno activo devuelve "": la regla se muestra igual en
// el resumen (para que el usuario vea que falta) pero no se aplica, porque
// escribir la de otro perfil enrutaría el repo a la cuenta equivocada — y eso, en
// un repo de cliente, es exactamente el fallo que allow_from existe para evitar.
func bootstrapRuleProfile(cfg *Config, active string) string {
	active = strings.TrimSpace(active)
	if active != "" && active != "default" {
		if _, ok := cfg.Profiles[active]; ok {
			return active
		}
	}
	if len(cfg.Profiles) == 1 {
		for n := range cfg.Profiles {
			return n
		}
	}
	return ""
}

// rulesCover reporta si ALGUNA regla es ancestro-o-igual de query.
//
// No se puede sustituir por `Resolve(...) != "default"`: una regla explícita
// hacia `default` («este repo usa mi ~/.claude de siempre») es una decisión
// tomada, y volver a ofrecerla en cada `ccp session` sería preguntar dos veces lo
// mismo con otras palabras.
func rulesCover(query string, rules []Rule) bool {
	q := NormalizePath(query)
	if q == "" {
		return false
	}
	for _, r := range rules {
		if r.Path == "" || r.Profile == "" {
			continue
		}
		if ruleIsAncestor(r.Path, q) {
			return true
		}
	}
	return false
}

// autoSeen es el `slices.Contains` de este archivo (mismo criterio que el resto
// del paquete, que no usa slices).
func autoSeen(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

// BootstrapApplied es el parte de lo que la aplicación llegó a hacer. Se
// devuelve TAMBIÉN cuando hay error: el llamador tiene que poder decir «creé la
// regla pero no pude instalar los sensores», que es lo único que le permite al
// usuario retomar donde se quedó.
type BootstrapApplied struct {
	Kinds []BootstrapKind // huecos cerrados, en orden

	// Skipped son los huecos que se ENSEÑARON en el resumen y no se llegaron a
	// cerrar porque el bootstrap no sabe hacerlo solo (hoy: la regla sin perfil
	// deducible). Van con el item entero, no con el kind, para que el front-end
	// pueda nombrar la ruta concreta que quedó sin cubrir.
	//
	// Existe porque callarlos es el peor de los dos fallos posibles: el usuario
	// autorizó una configuración leyendo una tabla que prometía «(crear)», el
	// parte de lo aplicado no la mencionaba, y como la marca de «preguntar una
	// vez» se escribe igual, el flujo que existía para arreglarlo no volvía a
	// hablar nunca.
	Skipped []BootstrapItem

	Rule    string   // ruta normalizada que RuleSet escribió de verdad
	Profile string   // perfil de esa regla
	Primary string   // primario cuya entrada allow_from se ensanchó
	Allowed []string // perfiles autorizados al ensanchar allow_from
	Sensors []string // perfiles a los que se les instaló la capa
}

// BootstrapError envuelve el fallo de un hueco concreto para que internal/cli
// pueda traducir el marco (qué se estaba haciendo) sin depender de la prosa de
// core, que es castellana y no se traduce.
type BootstrapError struct {
	Kind  BootstrapKind
	Cause error
}

func (e *BootstrapError) Error() string {
	return "bootstrap " + string(e.Kind) + ": " + e.Cause.Error()
}

func (e *BootstrapError) Unwrap() error { return e.Cause }

// BootstrapApply cierra los huecos del plan, en orden y parando en el primero
// que falle.
//
// El orden NO es cosmético: AutoInit tiene que preceder a todo (sin bloque
// auto_handoff, `auto install` y `chain add` se niegan), y la regla tiene que
// preceder a la cadena y a los sensores porque es la que decide QUIÉN es el
// primario — ChainAdd resuelve el primario del cwd por su cuenta, y aplicarla
// antes de escribir la regla ensancharía la entrada allow_from del perfil
// equivocado.
//
// Cada paso recarga ccp.yaml por su cuenta (es lo que ya hacen AutoInit, RuleSet
// y ChainAdd): no hay un *Config vivo cruzando los pasos que pueda quedarse
// obsoleto a mitad.
func BootstrapApply(home string, plan BootstrapPlan) (BootstrapApplied, error) {
	var out BootstrapApplied

	for _, it := range plan.Gaps() {
		switch it.Kind {
		case BootstrapAuto:
			if err := AutoInit(home, false); err != nil {
				return out, &BootstrapError{Kind: it.Kind, Cause: err}
			}
			out.Kinds = append(out.Kinds, it.Kind)

		case BootstrapRule:
			// Sin perfil deducible no hay nada que escribir. No es un error —el
			// resumen ya nombró `ccp path set`— pero SÍ se registra: un hueco que se
			// prometió cerrar y no se cerró tiene que salir en el parte, o el
			// usuario se queda creyendo que su repo quedó enrutado.
			if it.Profile == "" || it.Path == "" {
				out.Skipped = append(out.Skipped, it)
				continue
			}
			norm, err := RuleSet(home, it.Path, it.Profile)
			if err != nil {
				return out, &BootstrapError{Kind: it.Kind, Cause: err}
			}
			out.Rule, out.Profile = norm, it.Profile
			out.Kinds = append(out.Kinds, it.Kind)

		case BootstrapChain:
			// Inalcanzable con un plan de BootstrapDetect (un hueco de cadena
			// SIEMPRE trae los denegados), pero un plan lo construye quien quiera:
			// sin nombres no hay a quién autorizar.
			if len(it.Profiles) == 0 {
				continue
			}
			// Exactamente `ccp auto chain add <denegados>`: los perfiles ya están
			// en `fallback` (por eso salieron como Denied y no como ausentes), así
			// que lo único que hace es abrirles el gate del primario actual — y
			// solo el del primario actual, que es el límite que ChainAdd garantiza.
			res, err := ChainAdd(home, ChainOpts{Policy: it.Policy, Cwd: plan.Cwd}, it.Profiles)
			if err != nil {
				return out, &BootstrapError{Kind: it.Kind, Cause: err}
			}
			out.Primary, out.Allowed = res.Primary, res.AllowAdded
			out.Kinds = append(out.Kinds, it.Kind)

		case BootstrapSensors:
			if len(it.Profiles) == 0 {
				continue
			}
			if err := autoHooksInstall(home, it.Profiles); err != nil {
				return out, &BootstrapError{Kind: it.Kind, Cause: err}
			}
			out.Sensors = it.Profiles
			out.Kinds = append(out.Kinds, it.Kind)
		}
	}
	return out, nil
}

// autoHooksInstall es el camino de escritura de `ccp auto install <perfiles>`:
// mete los nombres en auto_handoff.hooks, PERSISTE, y solo entonces regenera
// cada cc-home.
//
// El orden es el contrato que documenta cli.autoInstall: `hooks` es la fuente de
// verdad que lee CfgRegenerate, así que regenerar antes de guardar escribiría un
// settings.json con la capa vieja. Si el guardado falla no se regenera nada y el
// estado queda coherente.
//
// La reconstrucción de la lista se delega en AutoHooksSet, que es la MISMA
// función que usa el comando: el dedupe defensivo y la conservación del orden que
// el usuario escribió a mano viven en un solo sitio.
func autoHooksInstall(home string, names []string) error {
	cfg, err := Load(home)
	if err != nil {
		return err
	}
	if cfg.AutoHandoff == nil {
		return &ChainError{Kind: ChainErrNotConfigured}
	}
	cfg.AutoHandoff.Hooks = AutoHooksSet(cfg.AutoHandoff.Hooks, names, true)
	if err := Save(home, cfg); err != nil {
		return err
	}
	for _, n := range names {
		if err := ProfileSync(home, n); err != nil {
			return err
		}
	}
	return nil
}

// --- la caché de «preguntar una vez» ----------------------------------------
//
// Vive bajo <home>/state/auto/bootstrap/, la carpeta que este proyecto define
// como CACHÉ: borrarla cuesta calidad, nunca corrección. Borrar una marca hace
// que se vuelva a preguntar, que es exactamente lo que debe pasar — la marca no
// autoriza ni bloquea nada, solo recuerda que ya se preguntó una vez.
//
// Por eso NO va en ccp.yaml: es estado, no configuración. Y por eso el lector
// devuelve (valor, ok) y jamás un error: ausencia, permiso denegado y JSON
// truncado son el mismo estado —no hay memoria— y la consecuencia correcta es
// preguntar otra vez.

// BootstrapMark es lo que queda en disco tras preguntar.
type BootstrapMark struct {
	// Repo es la ruta completa, no el nombre del archivo: el nombre lleva la
	// versión saneada y truncada, y se verifica al leer para que dos repos
	// distintos no puedan compartir marca por una colisión de nombre.
	Repo     string          `json:"repo"`
	AskedAt  time.Time       `json:"asked_at"`
	Accepted bool            `json:"accepted"`
	Applied  []BootstrapKind `json:"applied,omitempty"`
}

// bootstrapDir es <home>/state/auto/bootstrap.
func bootstrapDir(home string) string { return filepath.Join(AutoStateDir(home), "bootstrap") }

// bootstrapMarkName forma el nombre de archivo de un repo: el basename saneado
// (para que un humano reconozca el archivo) + un hash de la ruta COMPLETA (para
// que dos repos con el mismo basename, o dos rutas largas con prefijo común que
// autoNameMax truncaría igual, no colisionen).
func bootstrapMarkName(repo string) string {
	sum := sha256.Sum256([]byte(repo))
	hash := hex.EncodeToString(sum[:6])
	base, err := sanitizeAutoName("repo", filepath.Base(repo))
	if err != nil {
		base = "repo"
	}
	if len(base) > 40 {
		base = base[:40]
	}
	return base + "-" + hash + ".json"
}

// WriteBootstrapMark deja la marca de «ya se preguntó por este repo».
//
// Se escribe diga el usuario que SÍ o que NO: «preguntar una vez» significa una
// vez. Si solo se marcara el sí, quien contesta que no volvería a ver el prompt
// en cada `ccp session` — que es la forma más rápida de que alguien acepte por
// cansancio algo que ya había rechazado.
func WriteBootstrapMark(home string, m BootstrapMark) error {
	repo := NormalizePath(m.Repo)
	if repo == "" {
		return &BootstrapError{Kind: BootstrapRule, Cause: errBootstrapNoRepo}
	}
	m.Repo = repo
	if m.AskedAt.IsZero() {
		m.AskedAt = time.Now()
	}
	m.AskedAt = m.AskedAt.UTC()
	return writeAtomicJSON(filepath.Join(bootstrapDir(home), bootstrapMarkName(repo)), m)
}

// ReadBootstrapMark devuelve la marca de un repo. ok=false si no hay ninguna
// utilizable, sin distinguir el motivo: ver §caché arriba.
func ReadBootstrapMark(home, repo string) (BootstrapMark, bool) {
	repo = NormalizePath(repo)
	if repo == "" {
		return BootstrapMark{}, false
	}
	data, err := os.ReadFile(filepath.Join(bootstrapDir(home), bootstrapMarkName(repo)))
	if err != nil {
		return BootstrapMark{}, false
	}
	var m BootstrapMark
	if json.Unmarshal(data, &m) != nil {
		return BootstrapMark{}, false
	}
	// El nombre puede colisionar (basename saneado + hash truncado); la ruta de
	// dentro es la que manda. Un desacuerdo se trata como «no hay marca», nunca
	// como la marca de otro repo.
	if NormalizePath(m.Repo) != repo {
		return BootstrapMark{}, false
	}
	return m, true
}

// errBootstrapNoRepo es el único error propio de la caché. No lo ve el usuario
// (el llamador escribe la marca best-effort), así que no necesita traducción.
var errBootstrapNoRepo = errors.New("bootstrap: repo vacío")
