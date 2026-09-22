package core

import (
	"sort"
	"strings"

	yaml "github.com/goccy/go-yaml"
)

// auto_chains.go — `auto_handoff.chains`: la cadena PROPIA de un perfil.
//
// Hasta aquí la cadena era UNA: `policies.<n>.fallback`, la misma para todos los
// primarios. El gate `allow_from` era lo único por perfil, y solo sabe RESTAR —
// no puede darle a un primario un orden distinto ni un destino que la lista
// global no tuviera ya. La lista compartida era el techo de todos.
//
// Eso convierte en imposible lo que es el caso normal de quien tiene cuentas de
// varios sitios: `trabajo-a` puede prestarle a `trabajo-a-2` y a nadie más,
// `personal` le presta a su proveedor, y el cliente no le presta a nadie. Son
// tres cadenas distintas, no tres recortes de una.
//
// El reparto de responsabilidades que se elige aquí, y que conviene no borrar:
//
//	chains[<perfil>]  — A QUIÉN le presta ese primario, EN ORDEN. Por perfil.
//	policies[<n>]     — CUÁNDO y CUÁNTO (umbral, permanencia, hops, cooldown).
//	allow_from[<p>]   — gate de CUMPLIMIENTO, sigue igual y sigue aplicándose
//	                    DESPUÉS: una cadena propia no se salta el gate.
//
// Por qué la herencia existe y no se declara todo: un perfil SIN entrada usa el
// `fallback` de su política, que es exactamente lo que hace hoy. Así una config
// que ya funcionaba sigue funcionando sin migrar nada, y un perfil nuevo arranca
// rotando en vez de arrancar mudo. Declararlo todo habría cambiado el
// significado de los ccp.yaml existentes, que es el precio que no toca pagar por
// una feature aditiva.
//
// Y la regla que sí tiene filo, la misma que `allow_from` aprendió antes: los
// estados son TRES, no dos. Entrada AUSENTE es «hereda»; entrada DECLARADA con
// lista vacía es «este perfil no presta a nadie». Lo segundo es una decisión y
// hay que poder escribirla; si `[]` se leyera como ausente, apagar la rotación
// de un solo perfil no se podría expresar y habría que apagarla entera.
//
// El bloque es ADITIVO: `version` sigue en 2. Un ccp viejo captura `chains` en
// AutoHandoff.Extra y lo reescribe intacto (para eso se puso ese catch-all), así
// que no destruye la configuración — pero NO la entiende: rotará por la lista
// compartida. Es degradación visible en el comportamiento, no pérdida de datos.

// AutoChain es la cadena propia de un perfil. Admite DOS formas en el yaml y la
// diferencia es solo de comodidad:
//
//	a-cc: [a-cc-2, personal-cc]          # forma corta: solo la cadena
//	a-cc:                                # forma larga: cadena + política propia
//	  fallback: [a-cc-2]
//	  policy: relajada
//
// La forma larga existe por `policy`: liga ese primario a una política con
// nombre, o sea a OTROS knobs (umbral, cooldown, permanencia). Reusa `policies`
// en vez de inventar una tercera capa de parámetros — un perfil con API key, que
// no tiene ventana `resets_at` que consultar, necesita `cooldown.strategy: fixed`
// y ya había sitio donde escribirlo.
type AutoChain struct {
	// Fallback es la cadena, en ORDEN de preferencia. Ojo: vacía NO basta para
	// saber qué quiso decir el usuario — ver hasFallback.
	Fallback []string `yaml:"fallback,omitempty"`

	// Policy liga este primario a una política con nombre. Vacío = la política
	// que corresponda por el camino normal (la bandera --policy, o `default`).
	Policy string `yaml:"policy,omitempty"`

	// Extra es el catch-all de la entrada, por la misma razón que los de
	// AutoHandoff y AutoPolicy: sin él, una clave que este binario no conozca se
	// pierde en el siguiente Save, o sea en cualquier `ccp rule set` hecho con
	// un ccp de otra versión.
	Extra map[string]any `yaml:",inline"`

	// long recuerda que el usuario escribió la forma LARGA aunque solo pusiera
	// `fallback`. Sin esto, releer y volver a guardar su archivo le convertiría
	// `{fallback: [a]}` en `[a]`: una normalización que nadie pidió, en un
	// archivo que edita a mano. No se serializa.
	long bool

	// hasFallback dice si la clave `fallback` (o la forma corta) ESTABA. Es la
	// diferencia entre las dos cosas que una lista vacía podría significar, y
	// deducirla de len(Fallback)==0 era imposible:
	//
	//	a-cc: []                  -> hasFallback -> «no le presta a nadie»
	//	a-cc: {policy: relajada}  -> !hasFallback -> «misma cadena, otros knobs»
	//
	// Apagar la rotación de UN perfil tiene que poder escribirse, y ligarle una
	// política no puede apagársela de rebote. Sin este campo una de las dos se
	// come a la otra, y las dos son cosas que la gente quiere.
	hasFallback bool
}

// knownChainKeys es knownTopKeys para el catch-all de una entrada de `chains`.
var knownChainKeys = map[string]struct{}{
	"fallback": {},
	"policy":   {},
}

// Los autoChainRaw* son AutoChain sin sus métodos: tipos aparte, no alias con
// los mismos, que es lo que evita que el (Un)MarshalYAML de abajo se llame a sí
// mismo en bucle al delegar en goccy.
//
// Son DOS porque `fallback` no puede llevar omitempty y tampoco puede salir
// siempre: con omitempty, una cadena declarada vacía («no presta a nadie») se
// escribiría sin la clave y al releerla pasaría a «hereda» — la rotación
// volvería sola. Sin omitempty, una entrada que solo liga política escribiría
// `fallback: []` y apagaría la rotación de ese perfil. Elegir el tipo según
// hasFallback es lo que hace que guardar y releer devuelva lo mismo.
type autoChainRaw struct {
	Fallback []string       `yaml:"fallback"`
	Policy   string         `yaml:"policy,omitempty"`
	Extra    map[string]any `yaml:",inline"`
}

type autoChainRawNoFallback struct {
	Policy string         `yaml:"policy,omitempty"`
	Extra  map[string]any `yaml:",inline"`
}

// UnmarshalYAML acepta las dos formas, y además distingue la clave AUSENTE de
// la lista vacía. Se sondea el nodo a `any` primero porque es la única manera
// exacta de saber qué escribió el usuario: decodificar directo a []string
// convierte `null`, `[]` y «no había clave» en el mismo nil.
func (c *AutoChain) UnmarshalYAML(b []byte) error {
	var probe any
	if err := yaml.Unmarshal(b, &probe); err != nil {
		return err
	}
	switch probe.(type) {
	case nil:
		// `a-cc:` a secas. No declara nada, así que no sustituye nada: la entrada
		// es un no-op y el perfil sigue heredando. Tratarla como «lista vacía»
		// apagaría la rotación de ese perfil por una línea a medio escribir.
		*c = AutoChain{}
		return nil
	case []any:
		var list []string
		if err := yaml.Unmarshal(b, &list); err != nil {
			return err
		}
		*c = AutoChain{Fallback: list, hasFallback: true}
		return nil
	}

	var raw autoChainRaw
	if err := yaml.Unmarshal(b, &raw); err != nil {
		return err
	}
	c.Fallback = raw.Fallback
	c.Policy = strings.TrimSpace(raw.Policy)
	c.Extra = raw.Extra
	c.long = true
	c.hasFallback = autoChainHasKey(probe, "fallback")
	return nil
}

// autoChainHasKey mira la presencia de una clave en el nodo ya sondeado. goccy
// decodifica un mapa a map[string]any cuando el destino es `any`, pero se
// contempla también map[any]any por si cambiara: una comprobación de presencia
// que devolviera «no» por el tipo del mapa apagaría cadenas declaradas.
func autoChainHasKey(probe any, key string) bool {
	switch m := probe.(type) {
	case map[string]any:
		_, ok := m[key]
		return ok
	case map[any]any:
		_, ok := m[key]
		return ok
	}
	return false
}

// MarshalYAML devuelve la forma corta cuando no hay nada más que la cadena Y el
// usuario no había escrito la larga. Es cosmética, pero el archivo se edita a
// mano: `a-cc: [a-cc-2]` se lee de un vistazo y la forma larga de tres líneas
// no, y quien escribió la larga tiene derecho a que siga ahí.
func (c AutoChain) MarshalYAML() (any, error) {
	if c.hasFallback && !c.long && c.Policy == "" && len(c.Extra) == 0 {
		if c.Fallback == nil {
			return []string{}, nil
		}
		return c.Fallback, nil
	}
	if !c.hasFallback {
		return autoChainRawNoFallback{Policy: c.Policy, Extra: c.Extra}, nil
	}
	list := c.Fallback
	if list == nil {
		list = []string{}
	}
	return autoChainRaw{Fallback: list, Policy: c.Policy, Extra: c.Extra}, nil
}

// NewAutoChain construye una entrada con cadena propia declarada. Es el
// constructor que usan las mutaciones: poner el struct a mano desde otro paquete
// dejaría hasFallback en false y la entrada escrita se leería como «hereda».
func NewAutoChain(fallback []string, policy string) AutoChain {
	if fallback == nil {
		fallback = []string{}
	}
	return AutoChain{
		Fallback:    fallback,
		Policy:      strings.TrimSpace(policy),
		hasFallback: true,
		long:        strings.TrimSpace(policy) != "",
	}
}

// ProfileChain es el estado de la cadena de UN primario, ya resuelto. Los tres
// estados del comentario de cabecera salen de combinar Declared con Fallback:
// Declared=false es «hereda», Declared=true con Fallback vacío es «no presta».
type ProfileChain struct {
	Declared bool     // el perfil declara cadena propia (la clave `fallback` estaba)
	Fallback []string // su cadena (vacía y declarada = no presta a nadie)
	Policy   string   // política ligada, "" si no liga ninguna
	Entry    bool     // hay entrada en `chains` (aunque solo ligue política)
}

// AutoChainFor lee la cadena propia de un primario. Es el ÚNICO lector, igual
// que AutoGateFor lo es del gate: quien copie la regla de los estados para
// pintarla acabará enseñando un reparto que el supervisor no reconoce, que es la
// tarde que ya se perdió una vez con allow_from.
//
// Devuelve copia de la lista: el llamador la filtra y la reordena (el primario
// se descarta de su propia cadena al resolver) sin mutar el Config en memoria.
func AutoChainFor(cfg *Config, primary string) ProfileChain {
	if cfg == nil || cfg.AutoHandoff == nil || len(cfg.AutoHandoff.Chains) == 0 {
		return ProfileChain{}
	}
	entry, ok := cfg.AutoHandoff.Chains[primary]
	if !ok {
		return ProfileChain{}
	}
	return ProfileChain{
		Declared: entry.hasFallback,
		Fallback: chainClean(entry.Fallback),
		Policy:   strings.TrimSpace(entry.Policy),
		Entry:    true,
	}
}

// ChainOverviewRow es una fila de `ccp auto chain list`: qué cadena usa cada
// perfil y de dónde sale.
//
// Own=false NO significa «sin cadena»: significa que usa la de su política. Que
// la fila traiga igualmente la lista EFECTIVA es a propósito — una tabla donde
// las filas heredadas salieran vacías haría creer que esos perfiles no rotan,
// que es lo contrario de lo que pasa.
type ChainOverviewRow struct {
	Profile  string
	Own      bool     // declara cadena propia
	Fallback []string // la cadena que usa: la propia, o la heredada
	Policy   string   // política aplicable
	Pinned   bool     // esa política viene de chains[<perfil>].policy
	Missing  []string // perfiles de la cadena que ya no existen
	Orphan   bool     // hay entrada en `chains` para un perfil que ya no existe
}

// ChainOverview describe la cadena de TODOS los perfiles conocidos, `default`
// incluido (es un primario legítimo: una carpeta sin regla resuelve ahí).
//
// No filtra por allow_from a propósito: esto es el mapa de lo DECLARADO, y el
// gate es una capa aparte que `ccp auto chain show` sí aplica. Mezclarlos aquí
// dejaría una tabla en la que no se puede distinguir «no lo puse» de «el gate lo
// bloquea», que son dos arreglos distintos.
func ChainOverview(cfg *Config) []ChainOverviewRow {
	if cfg == nil {
		return nil
	}
	seen := map[string]bool{"default": true}
	names := []string{"default"}
	add := func(n string) {
		if !seen[n] {
			seen[n] = true
			names = append(names, n)
		}
	}
	for n := range cfg.Profiles {
		add(n)
	}
	// Un perfil borrado que aún tenga entrada en `chains` sale también: es basura
	// que hay que poder ver para poder quitarla, y callarla la deja creciendo.
	if cfg.AutoHandoff != nil {
		for n := range cfg.AutoHandoff.Chains {
			add(n)
		}
	}
	sort.Strings(names[1:])

	rows := make([]ChainOverviewRow, 0, len(names))
	for _, n := range names {
		pc := AutoChainFor(cfg, n)
		row := ChainOverviewRow{
			Profile: n,
			Own:     pc.Declared,
			Policy:  autoPolicyOr(pc.Policy),
			Pinned:  pc.Policy != "",
			Orphan:  pc.Entry && !autoProfileExists(cfg, n),
		}
		if pc.Declared {
			row.Fallback = pc.Fallback
		} else if cfg.AutoHandoff != nil {
			row.Fallback = chainClean(cfg.AutoHandoff.Policies[row.Policy].Fallback)
		}
		// El propio perfil se descarta de su fila, y los repetidos también: es la
		// MISMA regla que aplica ResolveAutoChain al resolver, y una tabla que
		// enseñara `personal-cc → personal-deepseek` en la fila de `personal-cc`
		// estaría diciendo que se presta a sí mismo, que no pasa nunca.
		seenFB := map[string]bool{}
		kept := make([]string, 0, len(row.Fallback))
		for _, f := range row.Fallback {
			if f == n || seenFB[f] {
				continue
			}
			seenFB[f] = true
			kept = append(kept, f)
			if !autoProfileExists(cfg, f) {
				row.Missing = append(row.Missing, f)
			}
		}
		row.Fallback = kept
		rows = append(rows, row)
	}
	return rows
}

// autoPolicyOr normaliza el nombre de política vacío a "default".
func autoPolicyOr(name string) string {
	if name = strings.TrimSpace(name); name != "" {
		return name
	}
	return "default"
}

// chainClean copia la lista quitando espacios y entradas vacías. `fallback:
// [a, , b]` es un typo tan común como silencioso, y la copia evita que un fallo
// a mitad de validación deje el Config en memoria a medio mutar.
func chainClean(list []string) []string {
	out := make([]string, 0, len(list))
	for _, n := range list {
		if n = strings.TrimSpace(n); n != "" {
			out = append(out, n)
		}
	}
	return out
}
