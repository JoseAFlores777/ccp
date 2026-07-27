package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/JoseAFlores777/ccp/internal/core"
	"github.com/JoseAFlores777/ccp/internal/core/i18n"
)

// auto.go — `ccp auto` (gestión de la capa de sensores) y los dos comandos
// internos que ESA capa invoca desde dentro de Claude Code:
//
//	ccp _statusline  ← statusLine de CC: muestrea rate_limits (sensor proactivo)
//	ccp _limit-hook  ← hook StopFailure de CC: deja un sentinel (backstop reactivo)
//
// Los dos internos comparten una regla que manda sobre todo lo demás: SALEN 0
// pase lo que pase. No son comandos del usuario, son código que corre DENTRO del
// bucle de CC — un statusLine que revienta deja la UI sin barra de estado y un
// hook que falla interrumpe el turno. Ante cualquier duda: no medir es
// preferible a molestar.

// init cablea el binario propio en core en cuanto arranca CUALQUIER comando del
// CLI, no solo `ccp auto`.
//
// Tiene que ser así porque la capa de sensores la escribe core.CfgRegenerate, al
// que también llegan `profile add`, `profile config`, `profile sync` y el TUI. Si
// solo `ccp auto install` inyectara la ruta, cada `ccp profile sync` reescribiría
// el settings.json con el default ("ccp") y el archivo bailaría entre dos valores
// a cada regeneración — churn puro en un archivo que el usuario mira.
func init() { core.SetAutoHooksBin(autoCCPBin()) }

// autoCCPBin resuelve la ruta absoluta del binario en marcha. El fallback es el
// nombre pelado: si os.Executable falla (procfs raro, binario borrado en
// caliente) preferimos depender del PATH —donde install.sh deja ccp— antes que
// escribir una ruta inventada en el settings.json del usuario.
func autoCCPBin() string {
	if p, err := os.Executable(); err == nil && strings.TrimSpace(p) != "" {
		return p
	}
	return "ccp"
}

// dispatchAuto maneja `ccp auto <sub>`. No es shell-only: no toca el entorno del
// shell padre, así que llega al binario por el `*) command ccp "$@"` del rc.
func dispatchAuto(args []string, stdout, stderr io.Writer) int {
	var sub string
	if len(args) > 0 {
		sub = args[0]
	}
	rest := []string{}
	if len(args) > 1 {
		rest = args[1:]
	}
	switch sub {
	case "init":
		return autoInit(rest, stdout, stderr)
	case "install":
		return autoInstall(rest, true, stdout, stderr)
	case "uninstall":
		return autoInstall(rest, false, stdout, stderr)
	case "status":
		return autoStatus(rest, stdout, stderr)
	case "test":
		return autoTest(rest, stdout, stderr)
	case "", "help", "--help", "-h":
		fmt.Fprintln(stdout, i18n.T(currentLang(), "cli.auto.usage"))
		return 0
	default:
		fmt.Fprintln(stderr, i18n.T(currentLang(), "cli.auto.unknown_sub", sub))
		fmt.Fprintln(stderr, i18n.T(currentLang(), "cli.auto.usage"))
		return 1
	}
}

// --- ccp auto init ---

// autoInit siembra el bloque auto_handoff. El chequeo de «ya existe» se hace
// AQUÍ y no se delega en core.AutoInit porque core.AutoInit es idempotente por
// contrato (no pisa y devuelve nil): sin este pre-chequeo, `ccp auto init` sobre
// una config ya sembrada imprimiría «hecho» sin haber hecho nada, que es la
// clase de mentira que hace perder una tarde.
func autoInit(args []string, stdout, stderr io.Writer) int {
	force := false
	for _, a := range args {
		switch a {
		case "--force", "-f":
			force = true
		default:
			fmt.Fprintln(stderr, i18n.T(currentLang(), "cli.auto.unknown_flag", a))
			return 1
		}
	}
	home := resolveHome()
	cfg, err := loadCfg(home)
	if err != nil {
		fmt.Fprintf(stderr, "[error] %v\n", err)
		return 1
	}
	lang := i18n.Resolve(cfg.Lang)
	if cfg.AutoHandoff != nil && !force {
		fmt.Fprintln(stderr, i18n.T(lang, "cli.auto.init_exists"))
		return 1
	}
	if err := core.AutoInit(home, force); err != nil {
		fmt.Fprintf(stderr, "[error] %v\n", err)
		return 1
	}
	after, err := core.Load(home)
	if err != nil {
		fmt.Fprintf(stderr, "[error] %v\n", err)
		return 1
	}
	n := 0
	if after.AutoHandoff != nil {
		n = len(after.AutoHandoff.Policies["default"].Fallback)
	}
	fmt.Fprintln(stdout, okLine(stdout, i18n.T(lang, "cli.auto.init_done", n)))
	fmt.Fprintln(stdout, mute(stdout, i18n.T(lang, "cli.auto.init_next")))
	return 0
}

// --- ccp auto install / uninstall ---

// autoInstall añade (install=true) o quita (install=false) perfiles de
// auto_handoff.hooks y regenera su cc-home para que el settings.json refleje el
// cambio. Sin argumentos actúa sobre TODOS los perfiles no-default.
//
// El orden importa: primero se persiste ccp.yaml y después se regenera. La lista
// `hooks` es la fuente de verdad que lee CfgRegenerate, así que regenerar antes
// de guardar produciría settings.json con la capa vieja. Si el guardado falla no
// se regenera nada y el estado queda coherente.
func autoInstall(args []string, install bool, stdout, stderr io.Writer) int {
	var names []string
	for _, a := range args {
		if strings.HasPrefix(a, "-") {
			fmt.Fprintln(stderr, i18n.T(currentLang(), "cli.auto.unknown_flag", a))
			return 1
		}
		names = append(names, a)
	}

	home := resolveHome()
	cfg, err := loadCfg(home)
	if err != nil {
		fmt.Fprintf(stderr, "[error] %v\n", err)
		return 1
	}
	lang := i18n.Resolve(cfg.Lang)
	if cfg.AutoHandoff == nil {
		fmt.Fprintln(stderr, i18n.T(lang, "cli.auto.not_configured"))
		return 1
	}

	if len(names) == 0 {
		names = autoSortedProfileNames(cfg)
		if len(names) == 0 {
			fmt.Fprintln(stderr, i18n.T(lang, "cli.auto.no_profiles"))
			return 1
		}
	}
	// Validación completa ANTES de mutar: un `ccp auto install a fantasma b` que
	// dejara `a` instalado y luego fallara obligaría al usuario a deshacer a mano.
	for _, n := range names {
		if n == "default" {
			fmt.Fprintln(stderr, i18n.T(lang, "cli.auto.reject_default"))
			return 1
		}
		if _, ok := cfg.Profiles[n]; !ok {
			fmt.Fprintln(stderr, i18n.T(lang, "cli.auto.unknown_profile", n))
			return 1
		}
	}

	had := make(map[string]bool, len(cfg.AutoHandoff.Hooks))
	for _, n := range cfg.AutoHandoff.Hooks {
		had[n] = true
	}
	target := make(map[string]bool, len(names))
	for _, n := range names {
		target[n] = true
	}

	// Reconstrucción conservando el orden existente: la lista la puede haber
	// escrito el usuario a mano y reordenarla ensuciaría su diff de ccp.yaml.
	var hooks []string
	for _, n := range cfg.AutoHandoff.Hooks {
		if !install && target[n] {
			continue
		}
		if autoSeenIn(hooks, n) {
			continue // dedupe defensivo: un yaml editado a mano puede repetir
		}
		hooks = append(hooks, n)
	}
	if install {
		for _, n := range names {
			if !autoSeenIn(hooks, n) {
				hooks = append(hooks, n)
			}
		}
	}
	cfg.AutoHandoff.Hooks = hooks
	if err := core.Save(home, cfg); err != nil {
		fmt.Fprintf(stderr, "[error] %v\n", err)
		return 1
	}

	rc := 0
	for _, n := range names {
		// ProfileSync = CfgMigrateLegacy + CfgRegenerate con el src global ya
		// resuelto por core; no duplicamos aquí la resolución de ~/.claude.
		if err := core.ProfileSync(home, n); err != nil {
			fmt.Fprintln(stderr, i18n.T(lang, "cli.auto.regen_failed", n, err))
			rc = 1
			continue
		}
		switch {
		case install && had[n]:
			fmt.Fprintln(stdout, okLine(stdout, i18n.T(lang, "cli.auto.already_installed", n)))
		case install:
			fmt.Fprintln(stdout, okLine(stdout, i18n.T(lang, "cli.auto.installed", n)))
		case had[n]:
			fmt.Fprintln(stdout, okLine(stdout, i18n.T(lang, "cli.auto.uninstalled", n)))
		default:
			fmt.Fprintln(stdout, mute(stdout, i18n.T(lang, "cli.auto.not_installed", n)))
		}
	}
	return rc
}

// autoSeenIn es el `slices.Contains` del repo (Go 1.24 lo tiene, pero el resto del
// paquete no usa slices y mezclar estilos aquí no aporta).
func autoSeenIn(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

// autoSortedProfileNames lista los perfiles del yaml en orden estable ('default' no
// está en el mapa: es implícito).
func autoSortedProfileNames(cfg *core.Config) []string {
	out := make([]string, 0, len(cfg.Profiles))
	for n := range cfg.Profiles {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// --- ccp auto status ---

// autoStatusJSON es el contrato scriptable de `ccp auto status --json`. Las
// claves son estables (añadir sí, renombrar no):
//
//	enabled       bool    — auto_handoff existe y tiene enabled: true
//	cwd           string  — directorio con el que se resolvió la política
//	error         string  — por qué no se pudo resolver (ausente si todo fue bien)
//	policy        objeto  — política efectiva ya con defaults aplicados
//	primary       string  — perfil que las reglas resuelven para cwd
//	fallback      []string— préstamos permitidos, EN ORDEN de preferencia
//	denied        []string— candidatos que allow_from bloqueó
//	sensors       []objeto— un elemento por perfil conocido (ver autoSensorJSON)
//
// `fallback`, `denied` y `sensors` se emiten siempre como array (nunca null)
// para que un `jq '.fallback | length'` funcione sin guardas.
type autoStatusJSON struct {
	Enabled  bool             `json:"enabled"`
	Cwd      string           `json:"cwd"`
	Error    string           `json:"error,omitempty"`
	Policy   *autoPolicyJSON  `json:"policy,omitempty"`
	Primary  string           `json:"primary,omitempty"`
	Fallback []string         `json:"fallback"`
	Denied   []string         `json:"denied"`
	Sensors  []autoSensorJSON `json:"sensors"`
}

// autoPolicyJSON son las duraciones ya normalizadas a texto Go ("20m0s"): el
// consumidor no tiene que saber que en el yaml pueden estar ausentes.
type autoPolicyJSON struct {
	Name             string `json:"name"`
	Threshold        int    `json:"threshold"`
	MinDwell         string `json:"min_dwell"`
	MaxHops          int    `json:"max_hops"`
	ReturnCheck      string `json:"return_check"`
	ReturnIdle       string `json:"return_idle"`
	CooldownStrategy string `json:"cooldown_strategy"`
	CooldownFallback string `json:"cooldown_fallback"`
}

// autoSensorJSON es el estado de un perfil:
//
//	profile             string — nombre
//	installed           bool   — está en auto_handoff.hooks
//	has_sample          bool   — hay muestra del statusLine en disco
//	sampled_at          string — RFC3339 UTC de la muestra (si has_sample)
//	age_seconds         int    — antigüedad de la muestra en segundos
//	five_hour_pct       float  — % usado de la ventana de 5h
//	seven_day_pct       float  — % usado de la ventana de 7d
//	five_hour_resets_at string — RFC3339 UTC (si se conoce)
//	seven_day_resets_at string — RFC3339 UTC (si se conoce)
//	cooldown_until      string — cuándo vuelve a estar disponible, si alguna
//	                             ventana ya supera el umbral de la política
type autoSensorJSON struct {
	Profile          string  `json:"profile"`
	Installed        bool    `json:"installed"`
	HasSample        bool    `json:"has_sample"`
	SampledAt        string  `json:"sampled_at,omitempty"`
	AgeSeconds       int64   `json:"age_seconds"`
	FiveHourPct      float64 `json:"five_hour_pct"`
	SevenDayPct      float64 `json:"seven_day_pct"`
	FiveHourResetsAt string  `json:"five_hour_resets_at,omitempty"`
	SevenDayResetsAt string  `json:"seven_day_resets_at,omitempty"`
	CooldownUntil    string  `json:"cooldown_until,omitempty"`
}

// autoStatus imprime la política resuelta para el cwd y el estado de los
// sensores. Exit 1 cuando la política no resuelve (bloque ausente, deshabilitado,
// fallback inexistente): así un script puede usarlo de gate antes de `ccp session`.
// En --json el error viaja DENTRO del JSON además del exit code, porque un
// consumidor que parsea stdout no debería tener que leer stderr para saber qué
// pasó.
func autoStatus(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	for _, a := range args {
		switch a {
		case "--json":
			asJSON = true
		default:
			fmt.Fprintln(stderr, i18n.T(currentLang(), "cli.auto.unknown_flag", a))
			return 1
		}
	}
	home := resolveHome()
	cfg, err := loadCfg(home)
	if err != nil {
		fmt.Fprintf(stderr, "[error] %v\n", err)
		return 1
	}
	lang := i18n.Resolve(cfg.Lang)
	cwd := currentDir()

	out := autoStatusJSON{
		Cwd:      cwd,
		Fallback: []string{},
		Denied:   []string{},
		Sensors:  []autoSensorJSON{},
	}
	out.Enabled = cfg.AutoHandoff != nil && cfg.AutoHandoff.Enabled

	rc, rerr := core.ResolveAutoChain(home, cfg, "", cwd)
	threshold := core.DefaultAutoThreshold
	if rerr != nil {
		out.Error = rerr.Error()
	} else {
		threshold = rc.Policy.Threshold
		out.Primary = rc.Primary
		out.Fallback = append(out.Fallback, rc.Fallback...)
		out.Denied = append(out.Denied, rc.Denied...)
		out.Policy = &autoPolicyJSON{
			Name:             rc.Policy.Name,
			Threshold:        rc.Policy.Threshold,
			MinDwell:         rc.Policy.MinDwell.String(),
			MaxHops:          rc.Policy.MaxHops,
			ReturnCheck:      rc.Policy.ReturnCheck.String(),
			ReturnIdle:       rc.Policy.ReturnIdle.String(),
			CooldownStrategy: rc.Policy.CooldownStrategy,
			CooldownFallback: rc.Policy.CooldownFallback.String(),
		}
	}
	out.Sensors = autoCollectSensors(home, cfg, threshold, time.Now())

	if asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(out); err != nil {
			fmt.Fprintf(stderr, "[error] %v\n", err)
			return 1
		}
		if rerr != nil {
			return 1
		}
		return 0
	}

	printAutoStatus(stdout, lang, out, cfg.AutoHandoff != nil)
	if rerr != nil {
		fmt.Fprintf(stderr, "[error] %v\n", rerr)
		return 1
	}
	return 0
}

// autoCollectSensors reúne, por perfil, si tiene la capa instalada y cuál fue su
// última muestra del statusLine.
//
// Incluye a 'default' solo si aparece en `hooks` o si tiene muestra: es un
// perfil implícito (no está en cfg.Profiles) y listarlo siempre con «sin
// muestra» sería ruido en la salida de quien no lo usa.
func autoCollectSensors(home string, cfg *core.Config, threshold int, now time.Time) []autoSensorJSON {
	names := autoSortedProfileNames(cfg)
	if core.AutoHooksEnabled(cfg, "default") {
		names = append(names, "default")
	} else if _, _, ok := core.ReadRateLimits(home, "default"); ok {
		names = append(names, "default")
	}

	out := make([]autoSensorJSON, 0, len(names))
	for _, n := range names {
		row := autoSensorJSON{Profile: n, Installed: core.AutoHooksEnabled(cfg, n)}
		rl, sampled, ok := core.ReadRateLimits(home, n)
		if ok {
			row.HasSample = true
			row.SampledAt = sampled.UTC().Format(time.RFC3339)
			age := now.Sub(sampled)
			if age < 0 {
				age = 0 // muestra del futuro (reloj movido): 0 se lee mejor que -3h
			}
			row.AgeSeconds = int64(age.Seconds())
			row.FiveHourPct = rl.FiveHour.UsedPercentage
			row.SevenDayPct = rl.SevenDay.UsedPercentage
			if t := rl.FiveHour.ResetsAt; !t.IsZero() {
				row.FiveHourResetsAt = t.UTC().Format(time.RFC3339)
			}
			if t := rl.SevenDay.ResetsAt; !t.IsZero() {
				row.SevenDayResetsAt = t.UTC().Format(time.RFC3339)
			}
			// El cooldown conocido es el resets_at de la ventana que ya pasó el
			// umbral: es la única pista real de cuándo este perfil vuelve a servir.
			// Con el reloj: una ventana cuyo resets_at ya pasó reabrió, y anunciar un
			// cooldown vencido haría creer que el perfil sigue inservible.
			if _, exhausted := rl.ExhaustedAt(threshold, now); exhausted {
				row.CooldownUntil = autoCooldownUntil(rl, threshold, now)
			}
		}
		out = append(out, row)
	}
	return out
}

// autoCooldownUntil devuelve el resets_at MÁS TARDÍO entre las ventanas que superan
// el umbral: si la semanal está agotada, que la de 5h se libere en 20 minutos no
// hace al perfil utilizable.
func autoCooldownUntil(rl core.RateLimits, threshold int, now time.Time) string {
	var latest time.Time
	check := func(w core.Windowed) {
		if !w.HasData() || w.UsedPercentage < float64(threshold) || w.ResetsAt.IsZero() {
			return
		}
		if !w.ResetsAt.After(now) {
			return // ventana ya reabierta: no es cooldown, es historia
		}
		if w.ResetsAt.After(latest) {
			latest = w.ResetsAt
		}
	}
	check(rl.FiveHour)
	check(rl.SevenDay)
	if latest.IsZero() {
		return ""
	}
	return latest.UTC().Format(time.RFC3339)
}

// printAutoStatus renderiza la versión legible del mismo dato que emite --json.
//
// `configured` no sale de autoStatusJSON porque ahí los dos estados que hay que
// separar —bloque ausente y bloque con enabled:false— colapsan en el mismo
// `enabled: false`; el JSON los distingue por el campo `error`, la salida legible
// necesita el dato aparte.
func printAutoStatus(w io.Writer, lang i18n.Lang, s autoStatusJSON, configured bool) {
	fmt.Fprintln(w, boldLine(w, i18n.T(lang, "cli.auto.status_header")))
	fmt.Fprintln(w, hr(w))
	if s.Policy != nil {
		fmt.Fprintln(w, "  "+i18n.T(lang, "cli.auto.status_policy", s.Policy.Name))
		fmt.Fprintln(w, "  "+i18n.T(lang, "cli.auto.status_primary", accent(w, s.Primary), s.Cwd))
		fmt.Fprintln(w, "  "+i18n.T(lang, "cli.auto.status_fallback", autoJoinOrNone(lang, s.Fallback)))
		if len(s.Denied) > 0 {
			fmt.Fprintln(w, "  "+mute(w, i18n.T(lang, "cli.auto.status_denied", strings.Join(s.Denied, ", "))))
		}
		fmt.Fprintln(w, "  "+mute(w, i18n.T(lang, "cli.auto.status_limits",
			s.Policy.Threshold, s.Policy.MinDwell, s.Policy.MaxHops, s.Policy.ReturnCheck)))
		fmt.Fprintln(w, "  "+mute(w, i18n.T(lang, "cli.auto.status_return_idle", s.Policy.ReturnIdle)))
		fmt.Fprintln(w, "  "+mute(w, i18n.T(lang, "cli.auto.status_cooldown",
			s.Policy.CooldownStrategy, s.Policy.CooldownFallback)))
	} else if !configured {
		fmt.Fprintln(w, "  "+warnLine(w, i18n.T(lang, "cli.auto.status_not_configured")))
	} else if !s.Enabled {
		fmt.Fprintln(w, "  "+warnLine(w, i18n.T(lang, "cli.auto.status_disabled")))
	}

	fmt.Fprintln(w)
	fmt.Fprintln(w, boldLine(w, i18n.T(lang, "cli.auto.status_sensors")))
	for _, sen := range s.Sensors {
		state := i18n.T(lang, "cli.auto.status_sensor_off")
		if sen.Installed {
			state = i18n.T(lang, "cli.auto.status_sensor_on")
		}
		sample := i18n.T(lang, "cli.auto.status_no_sample")
		if sen.HasSample {
			sample = i18n.T(lang, "cli.auto.status_sample",
				sen.FiveHourPct, sen.SevenDayPct,
				autoShortAge(time.Duration(sen.AgeSeconds)*time.Second))
		}
		fmt.Fprintf(w, "  %s  %s · %s\n", accent(w, sen.Profile), state, mute(w, sample))
		if sen.CooldownUntil != "" {
			fmt.Fprintln(w, "      "+warnLine(w,
				i18n.T(lang, "cli.auto.status_cooldown_until", sen.CooldownUntil)))
		}
	}
}

// autoJoinOrNone evita imprimir una lista vacía como línea en blanco, que se lee
// como «no se pudo calcular» en vez de como «no hay».
func autoJoinOrNone(lang i18n.Lang, list []string) string {
	if len(list) == 0 {
		return i18n.T(lang, "cli.auto.status_none")
	}
	return strings.Join(list, " → ")
}

// autoShortAge formatea una antigüedad en la forma compacta que cabe en una línea de
// estado (12s / 4m / 3h / 2d).
func autoShortAge(d time.Duration) string {
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}

// --- ccp auto test ---

// autoTest fabrica un StopFailure sintético y lo mete por la MISMA función que
// ejecuta el hook real (limitHook), para luego comprobar con core.ReadSentinels
// que la señal llegó a disco.
//
// Por qué no invocar el binario como subproceso: lo único que añadiría es el
// fork; la ruta de decisión (parseo, clasificación, escritura del sentinel) es
// exactamente la misma función. Y así el diagnóstico funciona igual desde un
// binario recién compilado que no esté aún en el PATH.
func autoTest(args []string, stdout, stderr io.Writer) int {
	profile := ""
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--profile", "-p":
			if i+1 >= len(args) {
				fmt.Fprintln(stderr, i18n.T(currentLang(), "cli.auto.flag_needs_value", args[i]))
				return 1
			}
			profile = args[i+1]
			i++
		default:
			fmt.Fprintln(stderr, i18n.T(currentLang(), "cli.auto.unknown_flag", args[i]))
			return 1
		}
	}
	home := resolveHome()
	cfg, err := loadCfg(home)
	if err != nil {
		fmt.Fprintf(stderr, "[error] %v\n", err)
		return 1
	}
	lang := i18n.Resolve(cfg.Lang)
	cwd := currentDir()
	if profile == "" {
		profile = activeProfile(home, cwd)
	}

	fmt.Fprintln(stdout, boldLine(stdout, i18n.T(lang, "cli.auto.test_header", profile)))
	ok := true

	// Paso 1 — ¿está la capa instalada? Es un aviso, no un fallo: la prueba
	// sintética sigue siendo válida (mide la ruta de detección, no la
	// instalación) y separar ambas cosas ayuda a diagnosticar.
	if core.AutoHooksEnabled(cfg, profile) {
		fmt.Fprintln(stdout, okLine(stdout, i18n.T(lang, "cli.auto.test_layer_ok", profile)))
	} else {
		fmt.Fprintln(stdout, warnLine(stdout, i18n.T(lang, "cli.auto.test_layer_missing", profile, profile)))
	}

	// Paso 2 — inyectar el payload por la ruta del hook real.
	session := fmt.Sprintf("ccp-auto-test-%d", time.Now().UnixNano())
	since := time.Now().Add(-time.Second)
	payload := autoSyntheticStopFailure(session, cwd)
	// El stdout del hook se descarta: CC no lo muestra y aquí solo interesan el
	// exit code y el efecto en disco.
	if code := limitHook(bytes.NewReader(payload), nil, io.Discard, stderr); code != 0 {
		fmt.Fprintln(stdout, warnLine(stdout, i18n.T(lang, "cli.auto.test_hook_fail", code)))
		ok = false
	} else {
		fmt.Fprintln(stdout, okLine(stdout, i18n.T(lang, "cli.auto.test_hook_ok")))
	}

	// Paso 3 — ¿aparece el sentinel? Es lo que vigila el supervisor.
	found := false
	sentinels, serr := core.ReadSentinels(home, since)
	if serr != nil {
		fmt.Fprintf(stderr, "[error] %v\n", serr)
	}
	for _, s := range sentinels {
		if s.Session == session {
			fmt.Fprintln(stdout, okLine(stdout, i18n.T(lang, "cli.auto.test_sentinel_ok", string(s.Event.Window))))
			found = true
			break
		}
	}
	if !found {
		fmt.Fprintln(stdout, warnLine(stdout,
			i18n.T(lang, "cli.auto.test_sentinel_missing", core.AutoStateDir(home))))
		ok = false
	}

	// Paso 4 — limpiar SIEMPRE, incluso si algo falló: un sentinel sintético
	// olvidado haría que el próximo `ccp session` creyera ver un límite real.
	if err := core.ClearSentinels(home, session); err != nil {
		fmt.Fprintln(stdout, warnLine(stdout, i18n.T(lang, "cli.auto.test_cleanup_fail", err)))
		ok = false
	} else {
		fmt.Fprintln(stdout, okLine(stdout, i18n.T(lang, "cli.auto.test_cleanup_ok")))
	}

	if !ok {
		fmt.Fprintln(stdout, warnLine(stdout, i18n.T(lang, "cli.auto.test_fail")))
		return 1
	}
	fmt.Fprintln(stdout, okLine(stdout, i18n.T(lang, "cli.auto.test_pass")))
	return 0
}

// autoSyntheticStopFailure arma el payload de prueba.
//
// El límite va como PROSA en `error` (y no como {"type":"rate_limit"}) a
// propósito: es la forma más pobre en la que CC puede entregar la señal, así que
// probar esa es probar el peor caso. Si esto se detecta, la variante estructurada
// —que trae 429 explícito— también.
func autoSyntheticStopFailure(session, cwd string) []byte {
	payload := map[string]any{
		"hook_event_name": "StopFailure",
		"session_id":      session,
		"transcript_path": filepath.Join(cwd, session+".jsonl"),
		"cwd":             cwd,
		"error":           "You've hit your 5-hour limit · resets at 3pm (ccp auto test synthetic payload)",
	}
	b, _ := json.Marshal(payload)
	return b
}

// --- ccp _statusline ---

// cmdStatusLine implementa `ccp _statusline [-- <cmd envuelto>]`.
func cmdStatusLine(args []string, stdout, stderr io.Writer) int {
	return runStatusLine(os.Stdin, args, stdout, stderr)
}

// runStatusLine es el cuerpo, con el stdin inyectable (lo usan los tests y
// cualquier llamador interno).
//
// Contrato duro: SIEMPRE devuelve 0. Claude Code ejecuta esto en cada refresco de
// la barra de estado; si sale distinto de 0 o escribe basura, el usuario se queda
// sin barra y con un error recurrente en la UI. Todo error se traga.
func runStatusLine(stdin io.Reader, args []string, stdout, stderr io.Writer) (code int) {
	// El recover es la última red: un panic aquí (JSON hostil, disco raro) no
	// puede convertirse en un crash visible dentro de la UI de CC.
	defer func() {
		if r := recover(); r != nil {
			code = 0
		}
	}()

	data := autoReadCapped(stdin)

	// Muestreo. resolveHome() y NO ensureMigrated(): esto corre varias veces por
	// minuto desde dentro de CC y disparar la migración de config desde un
	// sensor sería mutar el estado del usuario en su nombre, sin que lo pida.
	home := resolveHome()
	profile := os.Getenv("CCP_PROFILE")
	if profile == "" {
		profile = "default"
	}
	// Un solo reloj para el muestreo y para la cuenta atrás: si se leyera dos
	// veces, la muestra que se persiste y la que se pinta podrían caer a lados
	// distintos de un reset.
	now := time.Now()
	rl, sampled := core.ParseStatusLineInput(data)
	if sampled {
		_ = core.WriteRateLimits(home, profile, rl, now)
	}

	wrapped := autoWrappedCommand(args)
	if len(wrapped) == 0 {
		// Sin comando envuelto la barra la pintamos nosotros. El perfil solo es
		// el suelo: sin muestra no hay nada honesto que añadirle.
		line := profile
		if sampled {
			if bar := statusBarRender(profile, rl, now, statusBarColumns(), statusBarColor()); bar != "" {
				line = bar
			}
		}
		fmt.Fprintln(stdout, line)
		return 0
	}

	// El envuelto recibe el MISMO stdin (ya consumido, por eso se guardó en
	// memoria) y su stdout se reenvía tal cual: para CC la barra sigue siendo la
	// del usuario, nosotros solo miramos de pasada.
	//
	// Se ejecuta directo, sin `sh -c`: CC ya lanzó ESTE comando a través de un
	// shell, así que los argumentos que nos llegan vienen expandidos. Volver a
	// pasarlos por un shell los expandiría dos veces (un argumento con espacios
	// se partiría, un `$HOME` literal desaparecería).
	cmd := exec.Command(wrapped[0], wrapped[1:]...)
	cmd.Stdin = bytes.NewReader(data)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	_ = cmd.Run()
	return 0
}

// autoReadCapped lee el stdin completo con tope. El tope existe porque el productor
// es un proceso ajeno: sin él, un stdin que no cierra dejaría el statusLine
// colgado y con él la barra de CC.
func autoReadCapped(r io.Reader) []byte {
	if r == nil {
		return nil
	}
	data, err := io.ReadAll(io.LimitReader(r, 4<<20))
	if err != nil {
		// Lectura parcial: se conserva lo leído. Un JSON truncado simplemente no
		// parseará, y eso ya está contemplado.
		return data
	}
	return data
}

// autoWrappedCommand extrae el comando envuelto de los args. Acepta las dos formas
// (`-- cmd args` y `cmd args` a secas): el `--` lo escribe la capa gestionada,
// pero un usuario que edite el settings.json a mano se lo puede comer.
func autoWrappedCommand(args []string) []string {
	for i, a := range args {
		if a == "--" {
			return args[i+1:]
		}
	}
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		return args
	}
	return nil
}

// --- la barra propia ---
//
// El layout y el color viven AQUÍ y solo aquí. Las piezas puras (el medidor y la
// cuenta atrás) las pone core.RenderGauge / core.HumanUntilAt, que no saben nada
// de ANSI ni de anchos: así el panel Estado del TUI puede reusarlas sin tener que
// deshacer decisiones tomadas para una terminal.

const (
	// Umbrales del semáforo, en porcentaje de consumo de la ventana.
	//
	// El crítico es core.DefaultAutoThreshold A PROPÓSITO, no por casualidad: 90
	// es el punto en el que el motor da la ventana por agotada y muda la
	// conversación a otro perfil. Si la barra pintara el rojo en otro sitio, el
	// usuario vería «tranquilo» justo cuando `ccp session` está a punto de
	// rotarle el perfil — dos superficies contando historias distintas del mismo
	// número.
	//
	// El de aviso no tiene equivalente en el motor: es el punto en el que aún se
	// puede decidir algo (cerrar el turno, cambiar de perfil a mano) antes de que
	// lo decida la rotación.
	statusBarWarnPct = 70
	statusBarCritPct = core.DefaultAutoThreshold

	// Celdas de medidor de cada escalón, y el ancho que se supone cuando no hay
	// forma de saberlo. NO hay umbrales de columnas por escalón: el nivel se
	// elige midiendo la línea ya montada (ver statusBarRender).
	statusBarDefaultCols = 80
	statusBarWideCells   = 10
	statusBarMidCells    = 4
)

// statusBarLevel es cuánto detalle cabe en la línea.
type statusBarLevel int

const (
	statusBarCompact statusBarLevel = iota // sin medidor
	statusBarMid                           // medidor de 4 celdas, pegado al %
	statusBarWide                          // medidor de 10 celdas, holgado
)

// statusBarColumns devuelve el ancho que se le supone a la barra, con 80 por
// defecto.
//
// Honestidad sobre de dónde sale el dato: COLUMNS es una variable de SHELL, no
// de entorno. Ni bash ni zsh la exportan por defecto, y los procesos `claude`
// reales no la llevan — o sea que en la ruta de producción esto devuelve 80 casi
// siempre. Quien quiera exactitud tiene que `export COLUMNS` a mano.
//
// Y aun así 80 no vuelve inalcanzable la escalera de detalle, porque el nivel no
// se elige por umbrales de columnas sino midiendo la línea completa contra este
// presupuesto (statusBarRender): un nombre de perfil largo degrada la barra a 80
// columnas igual que una terminal estrecha degradaría una barra corta.
//
// La tty NO se consulta a propósito: el statusLine corre dentro de Claude Code
// con stdout capturado por un pipe —no hay tty al otro lado a la que preguntar—
// y se ejecuta varias veces por minuto, así que abrir /dev/tty por refresco sería
// coste recurrente para un dato que ni siquiera es el ancho que CC reserva a la
// barra.
func statusBarColumns() int {
	n, err := strconv.Atoi(strings.TrimSpace(os.Getenv("COLUMNS")))
	if err != nil || n <= 0 {
		return statusBarDefaultCols
	}
	return n
}

// statusBarColor decide si la barra propia lleva ANSI.
//
// Deliberadamente NO usa useColor, el gate general del paquete: useColor exige
// que el destino sea un dispositivo de caracteres, y el destino de esta línea es
// SIEMPRE un pipe (Claude Code captura el stdout del statusLine para componer su
// barra). Con el gate general, verde/ámbar/rojo no se pintaban ni una sola vez en
// la única superficie que los usa: el semáforo entero era código muerto.
//
// Quien renderiza aquí no es la terminal, es CC, y CC sí interpreta las
// secuencias. Así que el único gate que queda con sentido es el que el usuario
// controla, NO_COLOR — y sin color el medidor de bloques sigue siendo legible,
// que es la razón de haber elegido medidor y no un punto de color.
func statusBarColor() bool {
	return colorAllowed()
}

// statusBarRender arma la línea entera (perfil + uso) eligiendo el nivel de
// detalle que CABE en cols.
//
// La elección se hace MIDIENDO la línea ya montada, no comparando cols contra
// umbrales fijos, y esa es la corrección importante: el nombre del perfil lo
// elige el usuario y puede tener cualquier longitud, y la cuenta atrás aparece o
// no según haya resets_at. Con umbrales fijos los tres niveles desbordaban su
// propio umbral en cuanto el perfil pasaba de cuatro letras — degradaba «por
// ancho» a un ancho que no era el de la línea, que es la peor de las dos
// alternativas: ni cabía ni conservaba el detalle.
//
// Se mide en runas sobre la variante SIN color: las secuencias ANSI ocupan bytes
// pero no columnas, y contarlas haría que la barra se degradara sola al
// encenderse el color. Runa ≈ columna vale aquí porque todo lo que entra son
// dígitos, ASCII y los glifos del medidor, todos de ancho 1.
//
// Devuelve "" cuando ninguna ventana trae dato: el llamador se queda con el
// perfil a secas.
func statusBarRender(profile string, rl core.RateLimits, now time.Time, cols int, color bool) string {
	if !rl.FiveHour.HasData() && !rl.SevenDay.HasData() {
		return ""
	}
	// currentLang() lee ccp.yaml: se resuelve UNA vez aunque probemos tres
	// niveles, porque esto corre varias veces por minuto.
	lang := currentLang()
	build := func(level statusBarLevel, tinted bool) string {
		return i18n.T(lang, statusBarJoinKey(level), profile, statusBarUsage(rl, now, level, tinted))
	}
	for _, level := range []statusBarLevel{statusBarWide, statusBarMid, statusBarCompact} {
		if utf8.RuneCountInString(build(level, false)) <= cols {
			return build(level, color)
		}
	}
	// Ni el compacto cabe (perfil larguísimo, terminal minúscula). Se entrega el
	// compacto igualmente en vez de recortar: lo primero que habría que cortar es
	// el nombre del perfil, y ese es justo el dato que la barra existe para dar
	// —en qué cuenta estás—. Que decida el emulador qué hacer con lo que sobra.
	return build(statusBarCompact, color)
}

// statusBarJoinKey elige el separador entre perfil y uso. En ancho completo es
// doble espacio: con dos medidores en la línea, un `·` más añade ruido a algo que
// ya está visualmente separado por los delimitadores del medidor.
func statusBarJoinKey(level statusBarLevel) string {
	if level == statusBarWide {
		return "cli.auto.statusline_usage_wide"
	}
	return "cli.auto.statusline_usage"
}

// statusBarSeverity traduce consumo a nivel de semáforo.
func statusBarSeverity(pct float64) severity {
	switch {
	case pct >= statusBarCritPct:
		return sevCrit
	case pct >= statusBarWarnPct:
		return sevWarn
	default:
		return sevOK
	}
}

// statusBarUsage arma el trozo de uso de la barra propia: las ventanas CON dato,
// etiquetadas, en orden de la que antes se libera a la que más tarda. Devuelve ""
// cuando ninguna trae dato.
//
// Enseña las DOS a propósito. Antes se enseñaba solo el máximo —la ventana más
// gastada, que es la que va a cortar primero— y como número suelto era
// ambiguo: un "31%" no dice si te quedan horas o días, y saltaba de una ventana
// a otra en cuanto la otra la adelantaba, sin que nada lo indicara. El sensor
// vigila las dos por separado (core.RateLimits.ExhaustedAt), así que la barra
// enseña las dos.
//
// Las etiquetas son fijas, no traducidas: `5h`/`7d` es como las nombra ya
// `ccp auto status` (cli.auto.status_sample), idénticas en ambos idiomas, y son
// las claves que usa el propio Claude Code (five_hour / seven_day).
func statusBarUsage(rl core.RateLimits, now time.Time, level statusBarLevel, color bool) string {
	parts := make([]string, 0, 2)
	for _, win := range []struct {
		win   core.Windowed
		label string
	}{
		{rl.FiveHour, "5h"},
		{rl.SevenDay, "7d"},
	} {
		// Una ventana sin dato se omite en vez de pintarse como 0%: el bug
		// conocido de CC (five_hour a 0 con seven_day poblado) haría que un
		// "5h 0%" dijera justo lo contrario de lo que sabemos.
		if !win.win.HasData() {
			continue
		}
		parts = append(parts, statusBarWindow(win.label, win.win, now, level, color))
	}
	sep := " · "
	if level == statusBarWide {
		sep = "  "
	}
	return strings.Join(parts, sep)
}

// statusBarWindow pinta UNA ventana: etiqueta, medidor (si cabe), porcentaje y
// cuenta atrás hasta el reset.
//
// El medidor y el porcentaje van del mismo color porque son el mismo dato dicho
// dos veces —uno para leer de un vistazo, otro para leer exacto—; teñir solo uno
// invitaría a pensar que miden cosas distintas. La cuenta atrás se queda sin
// teñir: la urgencia la lleva el consumo, no el reloj (un 95% que reabre en 5
// minutos sigue siendo un 95% ahora mismo).
//
// El porcentaje sale de core.ClampPct, el MISMO saneado que usa el medidor, y no
// del valor crudo. Es lo que impide que el medidor y el número se contradigan:
// con el crudo, un `used_percentage: 9e99` de una muestra corrupta pintaba el
// medidor lleno y a su lado 300 dígitos de porcentaje —una barra de estado de
// cientos de columnas—, y un -40 pintaba el medidor vacío junto a un "-40%" en
// verde. El sensor no puede fallar, pero tampoco puede escupir eso.
func statusBarWindow(label string, win core.Windowed, now time.Time, level statusBarLevel, color bool) string {
	pct := core.ClampPct(win.UsedPercentage)
	sev := statusBarSeverity(pct)
	var b strings.Builder
	b.WriteString(label)
	b.WriteString(" ")
	switch level {
	case statusBarWide:
		b.WriteString(severityTint(color, core.RenderGauge(pct, statusBarWideCells), sev))
		b.WriteString(" ")
	case statusBarMid:
		// Sin espacio entre medidor y porcentaje: a 4 celdas los delimitadores
		// ya separan, y cada carácter cuenta en un ancho que ya iba justo.
		b.WriteString(severityTint(color, core.RenderGauge(pct, statusBarMidCells), sev))
	}
	b.WriteString(severityTint(color, fmt.Sprintf("%.0f%%", pct), sev))

	// HumanUntilAt ya se calla ante un resets_at ausente o vencido; aquí solo hay
	// que no inventarse el separador cuando no hay nada que separar.
	if until := core.HumanUntilAt(win.ResetsAt, now); until != "" {
		b.WriteString(" ·")
		b.WriteString(until)
	}
	return b.String()
}

// --- ccp _limit-hook ---

// cmdLimitHook implementa `ccp _limit-hook` (hook StopFailure de CC).
func cmdLimitHook(args []string, stdout, stderr io.Writer) int {
	return limitHook(os.Stdin, args, stdout, stderr)
}

// limitHook es el cuerpo, con stdin inyectable.
//
// Contrato duro: SIEMPRE devuelve 0, también con stdin vacío o basura. Un hook
// que sale != 0 le aparece al usuario como un error en mitad de su sesión; y el
// caso «no supe interpretar el payload» no es un error suyo, es nuestro, y la
// consecuencia correcta es no detectar nada.
//
// No escribe nada en stdout: CC reenvía el stdout de algunos hooks al modelo, y
// un mensaje de ccp ahí sería contexto contaminado.
func limitHook(stdin io.Reader, _ []string, _, _ io.Writer) (code int) {
	defer func() {
		if r := recover(); r != nil {
			code = 0
		}
	}()

	data := autoReadCapped(stdin)
	ev, ok := autoHookLimitEvent(data)
	if !ok {
		return 0
	}
	session := autoHookSession(data)
	if session == "" {
		// Un sentinel sin sesión es inconsumible: el supervisor filtra por la
		// sesión que él lanzó. Escribirlo solo dejaría basura que nadie borra.
		return 0
	}
	profile := os.Getenv("CCP_PROFILE")
	if profile == "" {
		profile = "default"
	}
	_ = core.WriteSentinel(resolveHome(), core.Sentinel{
		Profile: profile,
		Session: session,
		Event:   ev,
		At:      time.Now(),
	})
	return 0
}

// autoHookLimitEvent decide si el payload del hook describe un rate limit.
//
// Dos pasadas, de más fiable a menos:
//
//  1. El parser compartido de ratelimit.go, que reconoce 429 y la clase
//     `rate_limit` en cualquiera de sus formas anidadas. Antes de llamarlo se
//     marca el payload como error (isApiErrorMessage) porque un StopFailure LO
//     ES por definición: sin esa marca, el parser descarta la detección por
//     frase, que es justo la que aplica aquí.
//  2. Si el parser no vio nada, se busca la prosa del límite en los campos de
//     texto del payload. Hace falta porque la forma más probable del hook es
//     `"error": "You've hit your weekly limit…"` — una cadena suelta, que el
//     parser interpreta como *clase* de error, no como mensaje, y por tanto
//     descarta.
func autoHookLimitEvent(data []byte) (core.LimitEvent, bool) {
	var obj map[string]json.RawMessage
	if json.Unmarshal(bytes.TrimSpace(data), &obj) != nil {
		return core.LimitEvent{}, false
	}
	if _, has := obj["isApiErrorMessage"]; !has {
		obj["isApiErrorMessage"] = json.RawMessage("true")
	}
	if marked, err := json.Marshal(obj); err == nil {
		if ev, ok := core.ParseTranscriptLine(marked); ok {
			ev.Source = "hook"
			return ev, true
		}
	}

	text := autoHookText(obj)
	if !autoLooksLikeLimitText(text) {
		return core.LimitEvent{}, false
	}
	return core.LimitEvent{
		Window: core.ClassifyLimitText(text),
		Source: "hook",
		Detail: autoTruncate(strings.TrimSpace(text), 240),
	}, true
}

// autoHookTextKeys son los campos donde CC puede dejar la prosa del fallo. El orden
// es de más específico a más genérico.
var autoHookTextKeys = []string{"error", "message", "reason", "stderr", "output", "text", "detail"}

// autoHookText concatena la prosa encontrada. Se concatena en vez de quedarse con la
// primera porque el nombre de la ventana ("weekly", "5-hour") puede estar en un
// campo distinto del que dice "limit".
func autoHookText(obj map[string]json.RawMessage) string {
	var parts []string
	for _, k := range autoHookTextKeys {
		raw, ok := obj[k]
		if !ok {
			continue
		}
		var s string
		if json.Unmarshal(raw, &s) == nil {
			if s = strings.TrimSpace(s); s != "" {
				parts = append(parts, s)
			}
			continue
		}
		// Objeto o array: se extrae solo la PROSA (text/content/message), nunca el
		// JSON crudo. Volcarlo entero metía en el texto identificadores y contadores
		// —un request_id como "req_011CT4291xYz"— que luego la heurística leía como
		// si fueran el mensaje del error.
		if s := strings.TrimSpace(core.ExtractProse(raw)); s != "" {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, " ")
}

// autoLooksLikeLimitText es deliberadamente CONSERVADOR: un falso positivo aquí
// provoca que el supervisor mueva la conversación del usuario a otra cuenta, que
// es una acción cara de deshacer. Por eso no basta con que core.ClassifyLimitText
// devuelva una ventana (le vale con que el texto diga "opus"): se exige además
// una frase que solo aparece cuando de verdad se topó un límite.
func autoLooksLikeLimitText(s string) bool {
	l := strings.ToLower(strings.NewReplacer("’", "'", "ʼ", "'", "‘", "'", "`", "'").Replace(s))
	// El 429 vale como señal solo si aparece como NÚMERO suelto, es decir como el
	// status HTTP que dice ser. Como subcadena libre lo contienen un request-id
	// (`req_011CT4291xYz`), un contador de tokens o un número de línea, y cada uno
	// de esos habría provocado un handoff completo por un error que no es límite.
	if hasStandaloneNumber(l, "429") {
		return true
	}
	if strings.Contains(l, "rate limit") || strings.Contains(l, "rate_limit") {
		return true
	}
	if !strings.Contains(l, "limit") {
		return false
	}
	return strings.Contains(l, "hit your") ||
		strings.Contains(l, "usage limit") ||
		strings.Contains(l, "session limit") ||
		strings.Contains(l, "weekly limit") ||
		strings.Contains(l, "hour limit") ||
		strings.Contains(l, "limit reached") ||
		strings.Contains(l, "limit exceeded")
}

// hasStandaloneNumber busca `num` como palabra: ni pegado a otro dígito ni a
// una letra o guion bajo. Es lo que separa «API Error: 429» de «CT4291xYz».
func hasStandaloneNumber(hay, num string) bool {
	for i := 0; i+len(num) <= len(hay); i++ {
		if hay[i:i+len(num)] != num {
			continue
		}
		if i > 0 && isWordByte(hay[i-1]) {
			continue
		}
		if j := i + len(num); j < len(hay) && isWordByte(hay[j]) {
			continue
		}
		return true
	}
	return false
}

// isWordByte reporta si el byte forma parte de un identificador (letra ASCII,
// dígito o guion bajo). Basta con ASCII: lo que se busca son dígitos.
func isWordByte(b byte) bool {
	switch {
	case b >= '0' && b <= '9', b >= 'a' && b <= 'z', b >= 'A' && b <= 'Z', b == '_':
		return true
	}
	return false
}

// autoHookSession saca el uuid de sesión del payload. Si no viene explícito se
// deriva del nombre del transcript, que ES el uuid (CC nombra el jsonl con él):
// es la misma convención que usa el resto de ccp para localizar sesiones.
func autoHookSession(data []byte) string {
	var p struct {
		SessionID      string `json:"session_id"`
		SessionIDCamel string `json:"sessionId"`
		TranscriptPath string `json:"transcript_path"`
	}
	if json.Unmarshal(bytes.TrimSpace(data), &p) != nil {
		return ""
	}
	for _, s := range []string{p.SessionID, p.SessionIDCamel} {
		if s = strings.TrimSpace(s); s != "" {
			return s
		}
	}
	if p.TranscriptPath != "" {
		return strings.TrimSuffix(filepath.Base(p.TranscriptPath), ".jsonl")
	}
	return ""
}

// autoTruncate acota una cadena a n bytes (el Detail del sentinel acaba en disco).
func autoTruncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
