package core

import (
	"sort"
	"strings"
	"time"
)

// auto.go — el bloque `auto_handoff` de ccp.yaml: la POLÍTICA que gobierna la
// rotación automática de perfiles cuando uno topa su rate limit.
//
// Aquí solo vive la política (parseo, defaults, validación y resolución para un
// cwd). La máquina de estados de la rotación (quién está agotado, cuándo vuelve
// el primario) es de internal/supervisor: este archivo es puro y sin I/O salvo
// AutoInit, que sí toca ccp.yaml.
//
// Por qué el bloque es ADITIVO y NO sube SchemaVersion: un ccp viejo que lea un
// ccp.yaml con `auto_handoff` lo captura en Config.Extra y lo reescribe intacto.
// Subir la version haría que el binario viejo se negara a leer el archivo — un
// precio absurdo por una feature que él no necesita entender.

// AutoHandoff es el bloque `auto_handoff` de ccp.yaml.
type AutoHandoff struct {
	Enabled   bool                  `yaml:"enabled"`
	Policies  map[string]AutoPolicy `yaml:"policies,omitempty"`
	AllowFrom map[string][]string   `yaml:"allow_from,omitempty"`
	Hooks     []string              `yaml:"hooks,omitempty"` // perfiles con la capa de sensores instalada
}

// AutoPolicy es una política con nombre. Los campos numéricos/duración van como
// cero-valor cuando el usuario no los escribe: Effective aplica los defaults.
// Las duraciones se guardan como STRING ("20m") y no como time.Duration porque
// el yaml lo escribe una persona y `20m` es más legible que `1200000000000`.
type AutoPolicy struct {
	Fallback    []string     `yaml:"fallback"`
	Threshold   int          `yaml:"threshold,omitempty"`
	MinDwell    string       `yaml:"min_dwell,omitempty"`
	MaxHops     int          `yaml:"max_hops,omitempty"`
	ReturnCheck string       `yaml:"return_check,omitempty"`
	ReturnIdle  string       `yaml:"return_idle,omitempty"`
	Cooldown    AutoCooldown `yaml:"cooldown,omitempty"`
}

// AutoCooldown describe cuánto esperar antes de reconsiderar un perfil agotado.
//
//	resets_at → usar el epoch `resets_at` que reporta el rate_limits de Claude
//	            Code (la señal buena; solo existe en perfiles de suscripción).
//	fixed     → ignorar resets_at y esperar siempre Fallback (perfiles con
//	            API key, donde no hay ventana que consultar).
type AutoCooldown struct {
	Strategy string `yaml:"strategy,omitempty"` // "resets_at" | "fixed"
	Fallback string `yaml:"fallback,omitempty"` // duración Go: "1h", "90m"
}

// Estrategias de cooldown admitidas. Vivir como constantes evita que el
// supervisor compare contra literales sueltos y se desincronice de la validación.
const (
	CooldownResetsAt = "resets_at"
	CooldownFixed    = "fixed"
)

// Defaults de la política. Son los de la spec: un umbral del 90% deja margen
// para terminar el turno en curso, y 20m de permanencia mínima evitan que dos
// señales seguidas (statusLine + transcript) provoquen dos saltos encadenados.
const (
	DefaultAutoThreshold   = 90
	DefaultAutoMinDwell    = 20 * time.Minute
	DefaultAutoMaxHops     = 6
	DefaultAutoReturnCheck = 10 * time.Minute
	DefaultAutoCooldown    = time.Hour

	// DefaultAutoReturnIdle es cuánto tiene que llevar CALLADA la conversación
	// para que el regreso proactivo pueda matar al hijo.
	//
	// Existe porque el regreso por `return_check` es el único movimiento
	// DISCRECIONAL del supervisor: la rotación por límite mata una sesión que ya
	// no responde (el perfil está en 429, seguir ahí no vale nada), pero volver a
	// casa mata una sesión que FUNCIONA. Sin este guard, con los defaults
	// (min_dwell 20m + return_check 10m) cualquier préstamo de más de 20 minutos
	// se termina en el instante en que vence el cooldown del primario: con el
	// usuario tecleando, a mitad de un turno, o con una tool call en vuelo — y una
	// tool call interrumpida la re-ejecuta `--resume`, que puede no ser idempotente
	// (ver README).
	//
	// 90s es el orden de magnitud de «el usuario paró de verdad»: un turno de
	// Claude Code con herramientas encadena escrituras al transcript cada pocos
	// segundos, así que minuto y medio de silencio no cabe DENTRO de un turno, y
	// sigue siendo lo bastante corto para que la primera pausa para leer o pensar
	// abra la ventana de regreso. Perder una oportunidad de volver es barato (el
	// temporizador la vuelve a ofrecer al siguiente tick); robarle el terminal al
	// usuario a media frase, no.
	DefaultAutoReturnIdle = 90 * time.Second
)

// EffectivePolicy es AutoPolicy con defaults aplicados y duraciones parseadas.
// El supervisor consume SOLO esto: nunca vuelve a mirar los strings crudos, así
// que un yaml malo falla una vez, temprano y con mensaje claro, en vez de
// romper a las 3am en mitad de un hop.
type EffectivePolicy struct {
	Name             string
	Fallback         []string
	Threshold        int
	MinDwell         time.Duration
	MaxHops          int
	ReturnCheck      time.Duration
	ReturnIdle       time.Duration
	CooldownStrategy string
	CooldownFallback time.Duration
}

// Effective valida y completa la política.
//
// Cada error nombra la política, la clave y el valor ofensivo: el usuario edita
// ccp.yaml a mano y "duración inválida" sin coordenadas lo obliga a adivinar
// cuál de las cuatro duraciones del bloque es la rota.
//
// Los errores son *ChainError TIPADOS y no fmt.Errorf. La prosa de Error() sigue
// siendo la misma castellana de siempre (los llamadores que solo la imprimen no
// notan el cambio), pero ahora internal/cli puede traducirla: estos errores
// salen por `ccp auto status`, por `ccp auto chain` y por la validación de
// `ccp config edit`, y en las tres una sesión en inglés recibía español crudo.
func (p AutoPolicy) Effective(name string) (EffectivePolicy, error) {
	if name == "" {
		name = "default"
	}
	eff := EffectivePolicy{
		Name:             name,
		Threshold:        DefaultAutoThreshold,
		MinDwell:         DefaultAutoMinDwell,
		MaxHops:          DefaultAutoMaxHops,
		ReturnCheck:      DefaultAutoReturnCheck,
		ReturnIdle:       DefaultAutoReturnIdle,
		CooldownStrategy: CooldownResetsAt,
		CooldownFallback: DefaultAutoCooldown,
	}

	// Copia defensiva: el llamador recibe un slice que puede reordenar/filtrar
	// sin mutar el Config en memoria (ResolveAutoChain filtra el primario).
	// Las entradas vacías se descartan aquí porque `fallback: [a, , b]` es un
	// typo tan común como silencioso.
	for _, f := range p.Fallback {
		if f = strings.TrimSpace(f); f != "" {
			eff.Fallback = append(eff.Fallback, f)
		}
	}

	// Threshold 0 significa "no escrito" -> default. Fuera de 1..100 no es un
	// porcentaje: rechazar es mejor que normalizar en silencio, porque un 900
	// (dedo pegado) desactivaría de facto la detección proactiva.
	if p.Threshold != 0 {
		if p.Threshold < 1 || p.Threshold > 100 {
			return EffectivePolicy{}, &ChainError{
				Kind: ChainErrThreshold, Policy: name, Key: "threshold", Num: p.Threshold}
		}
		eff.Threshold = p.Threshold
	}

	if p.MaxHops != 0 {
		if p.MaxHops < 0 {
			return EffectivePolicy{}, &ChainError{
				Kind: ChainErrMaxHops, Policy: name, Key: "max_hops", Num: p.MaxHops}
		}
		eff.MaxHops = p.MaxHops
	}

	var err error
	if eff.MinDwell, err = autoDuration(name, "min_dwell", p.MinDwell, DefaultAutoMinDwell); err != nil {
		return EffectivePolicy{}, err
	}
	if eff.ReturnCheck, err = autoDuration(name, "return_check", p.ReturnCheck, DefaultAutoReturnCheck); err != nil {
		return EffectivePolicy{}, err
	}
	// return_idle: 0s es un opt-out EXPLÍCITO del guard de inactividad (volver en
	// cuanto toque, aunque el usuario esté escribiendo). Se admite —igual que
	// `return_check: 0s` apaga el temporizador entero— pero no se ofrece como
	// default: quien lo escriba está aceptando que una tool call a medias se
	// re-ejecute al reanudar.
	if eff.ReturnIdle, err = autoDuration(name, "return_idle", p.ReturnIdle, DefaultAutoReturnIdle); err != nil {
		return EffectivePolicy{}, err
	}
	if eff.CooldownFallback, err = autoDuration(name, "cooldown.fallback", p.Cooldown.Fallback, DefaultAutoCooldown); err != nil {
		return EffectivePolicy{}, err
	}

	switch strings.TrimSpace(p.Cooldown.Strategy) {
	case "":
		// default ya sembrado
	case CooldownResetsAt, CooldownFixed:
		eff.CooldownStrategy = strings.TrimSpace(p.Cooldown.Strategy)
	default:
		return EffectivePolicy{}, &ChainError{
			Kind: ChainErrCooldown, Policy: name,
			Key: "cooldown.strategy", Value: p.Cooldown.Strategy}
	}

	return eff, nil
}

// autoDuration parsea una duración de la política. Vacío -> def. Una duración
// negativa es sintácticamente válida para time.ParseDuration ("-5m") pero no
// tiene semántica aquí: un min_dwell negativo permitiría rotar en bucle.
func autoDuration(policy, key, raw string, def time.Duration) (time.Duration, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return def, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return 0, &ChainError{Kind: ChainErrDuration, Policy: policy, Key: key, Value: raw, Cause: err}
	}
	if d < 0 {
		return 0, &ChainError{Kind: ChainErrDurationNeg, Policy: policy, Key: key, Value: raw}
	}
	return d, nil
}

// ResolvedChain es la política ya resuelta para un cwd concreto.
type ResolvedChain struct {
	Policy   EffectivePolicy
	Primary  string   // core.Resolve(cwd, cfg.Rules)
	Fallback []string // préstamos permitidos, en orden
	Denied   []string // los que allow_from bloqueó (para --dry-run)
}

// ResolveAutoChain resuelve la política aplicable a cwd: quién es el primario y
// a qué perfiles se le permite prestar la sesión, en orden.
//
// Reglas, en orden de aplicación:
//   - cfg nil -> se carga desde home (comodidad para llamadores que solo tienen
//     el home; si cfg viene dado, home no se usa).
//   - cfg.AutoHandoff ausente o Enabled==false -> error que apunta a `ccp auto init`.
//   - policyName "" -> "default".
//   - política inexistente -> error listando las que hay.
//   - perfil de fallback inexistente -> error (mejor fallar al arrancar que
//     descubrirlo cuando toque saltar).
//   - el primario dentro del fallback se descarta EN SILENCIO: es implícito, y
//     dejarlo dentro haría que el supervisor "saltara" a donde ya está.
//
// Gate allow_from (compliance, default deny). Tres formas, deliberadamente
// distintas:
//   - mapa AUSENTE o VACÍO -> no hay gate: todo el fallback pasa. Es la
//     configuración de quien no separa clientes; obligarlo a declarar el mapa
//     completo solo para permitir todo sería fricción sin valor.
//   - mapa DECLARADO y CON entrada para el primario -> pasan solo los candidatos
//     listados en esa entrada; el resto va a Denied.
//   - mapa DECLARADO y SIN entrada para el primario -> DENY TOTAL: Fallback
//     vacío y TODOS los candidatos en Denied. Declarar el mapa es declarar la
//     intención de gobernar los préstamos; un perfil olvidado debe quedarse
//     quieto, no heredar barra libre. Es lo que evita que un repo de cliente
//     nuevo (sin entrada aún) preste su sesión a un perfil personal a las 3am.
func ResolveAutoChain(home string, cfg *Config, policyName, cwd string) (ResolvedChain, error) {
	if cfg == nil {
		loaded, err := Load(home)
		if err != nil {
			return ResolvedChain{}, err
		}
		cfg = loaded
	}

	ah := cfg.AutoHandoff
	if ah == nil {
		return ResolvedChain{}, &ChainError{Kind: ChainErrNotConfigured}
	}
	if !ah.Enabled {
		return ResolvedChain{}, &ChainError{Kind: ChainErrDisabled}
	}

	if policyName == "" {
		policyName = "default"
	}
	pol, ok := ah.Policies[policyName]
	if !ok {
		return ResolvedChain{}, &ChainError{
			Kind: ChainErrNoPolicy, Policy: policyName, Detail: autoPolicyNames(ah.Policies)}
	}

	eff, err := pol.Effective(policyName)
	if err != nil {
		return ResolvedChain{}, err
	}

	primary := Resolve(cwd, cfg.Rules)

	// Validación + filtrado en una sola pasada, conservando el orden del yaml
	// (el orden ES la preferencia del usuario). Se deduplica porque un perfil
	// repetido no aporta un préstamo extra y ensucia la traza del --dry-run.
	seen := map[string]bool{}
	candidates := make([]string, 0, len(eff.Fallback))
	for _, name := range eff.Fallback {
		if !autoProfileExists(cfg, name) {
			return ResolvedChain{}, &ChainError{
				Kind: ChainErrFallbackProfile, Policy: policyName, Profile: name}
		}
		if name == primary || seen[name] {
			continue
		}
		seen[name] = true
		candidates = append(candidates, name)
	}

	rc := ResolvedChain{Policy: eff, Primary: primary}

	if len(ah.AllowFrom) == 0 {
		rc.Fallback = candidates
		return rc, nil
	}

	allowed, declared := ah.AllowFrom[primary]
	if !declared {
		// Deny total: nada pasa, pero se reporta TODO como denegado para que
		// `--dry-run` muestre exactamente qué se bloqueó y por qué.
		rc.Denied = candidates
		return rc, nil
	}
	allowSet := make(map[string]bool, len(allowed))
	for _, a := range allowed {
		allowSet[strings.TrimSpace(a)] = true
	}
	for _, name := range candidates {
		if allowSet[name] {
			rc.Fallback = append(rc.Fallback, name)
			continue
		}
		rc.Denied = append(rc.Denied, name)
	}
	return rc, nil
}

// autoProfileExists acepta 'default' además de los perfiles del yaml: 'default'
// es implícito (el ~/.claude del usuario) y es un destino de préstamo legítimo.
func autoProfileExists(cfg *Config, name string) bool {
	if name == "default" {
		return true
	}
	_, ok := cfg.Profiles[name]
	return ok
}

// autoPolicyNames lista las políticas existentes ordenadas, para los errores.
func autoPolicyNames(m map[string]AutoPolicy) string {
	if len(m) == 0 {
		return "ninguna"
	}
	names := make([]string, 0, len(m))
	for n := range m {
		names = append(names, n)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

// AutoInit siembra un bloque auto_handoff de arranque en ccp.yaml a partir de
// los perfiles existentes: una política "default" cuyo fallback son TODOS los
// perfiles (el primario se descarta luego en ResolveAutoChain, así que no hay
// que saber cuál es al sembrar) y un allow_from explícito perfil→[sí mismo] +
// el resto de perfiles OFICIALES.
//
// Por qué el allow_from inicial excluye los perfiles de provider (deepseek/kimi/
// glm): el gate existe justamente para que una sesión de cliente no acabe
// mandando su contexto a una API de terceros sin que alguien lo haya decidido.
// Sembrarlo permisivo convertiría el gate en decoración; el usuario añade esos
// destinos a mano cuando quiere.
//
// Idempotente: si ya hay bloque y force==false, NO lo pisa y devuelve nil (no es
// un error volver a correr `ccp auto init`). Con force==true lo regenera desde
// cero, perdiendo los ajustes manuales — de ahí que sea opt-in explícito.
func AutoInit(home string, force bool) error {
	cfg, err := Load(home)
	if err != nil {
		return err
	}
	if cfg.AutoHandoff != nil && !force {
		return nil
	}

	names := make([]string, 0, len(cfg.Profiles))
	for n := range cfg.Profiles {
		names = append(names, n)
	}
	sort.Strings(names)

	official := make([]string, 0, len(names))
	for _, n := range names {
		if cfg.Profiles[n].Type == "official" {
			official = append(official, n)
		}
	}

	allowFrom := make(map[string][]string, len(names))
	for _, n := range names {
		// El propio perfil primero: leer `emco-cc: [emco-cc, ...]` deja claro
		// que "no rotar" se escribe dejando solo a sí mismo.
		entry := []string{n}
		for _, o := range official {
			if o != n {
				entry = append(entry, o)
			}
		}
		allowFrom[n] = entry
	}

	cfg.AutoHandoff = &AutoHandoff{
		Enabled: true,
		Policies: map[string]AutoPolicy{
			"default": {
				Fallback:    names,
				Threshold:   DefaultAutoThreshold,
				MinDwell:    DefaultAutoMinDwell.String(),
				MaxHops:     DefaultAutoMaxHops,
				ReturnCheck: DefaultAutoReturnCheck.String(),
				ReturnIdle:  DefaultAutoReturnIdle.String(),
				Cooldown: AutoCooldown{
					Strategy: CooldownResetsAt,
					Fallback: DefaultAutoCooldown.String(),
				},
			},
		},
		AllowFrom: allowFrom,
	}
	return Save(home, cfg)
}
