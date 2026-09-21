package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/JoseAFlores777/ccp/internal/core"
	"github.com/JoseAFlores777/ccp/internal/core/i18n"
)

// serve_methods.go — los métodos de `ccp serve` para la app, las cuentas, las
// carpetas, la configuración y la memoria de Claude. Los de conversaciones,
// rotación, Desktop y sistema están en serve_ops.go.
//
// Todos devuelven datos, nunca prosa: la GUI pinta los textos en su idioma. La
// excepción son los métodos que ejecutan un comando de la CLI (runInProcess),
// que devuelven su salida tal cual porque ES el resultado que la GUI enseña.

func serveRegistry() map[string]serveMethod {
	r := func(fn func(*server, json.RawMessage) (any, error)) serveMethod { return serveMethod{fn: fn} }
	w := func(fn func(*server, json.RawMessage) (any, error)) serveMethod {
		return serveMethod{write: true, fn: fn}
	}
	return map[string]serveMethod{
		"app.info":        r(srvAppInfo),
		"app.setLang":     w(srvSetLang),
		"context.folders": r(srvFolders),
		"resolve":         r(srvResolve),

		"profiles.list":      r(srvProfilesList),
		"profiles.add":       w(srvProfilesAdd),
		"profiles.update":    w(srvProfilesUpdate),
		"profiles.rename":    w(srvProfilesRename),
		"profiles.remove":    w(srvProfilesRemove),
		"profiles.setKey":    w(srvProfilesSetKey),
		"profiles.sync":      w(srvProfilesSync),
		"profiles.drift":     r(srvProfilesDrift),
		"profiles.effective": r(srvProfilesEffective),
		"overlay.envSet":     w(srvOverlayEnvSet),
		"overlay.envDel":     w(srvOverlayEnvDel),
		"overlay.ruleAdd":    w(srvOverlayRuleAdd),
		"overlay.ruleRemove": w(srvOverlayRuleRemove),

		"rules.list":   r(srvRulesList),
		"rules.set":    w(srvRulesSet),
		"rules.remove": w(srvRulesRemove),
		"rules.clear":  w(srvRulesClear),

		"config.get":        r(srvConfigGet),
		"config.setDefault": w(srvConfigSetDefault),
		"config.reset":      w(srvConfigReset),
		"config.setEditor":  w(srvConfigSetEditor),

		"memory.list":   r(srvMemoryList),
		"memory.add":    w(srvMemoryAdd),
		"memory.remove": w(srvMemoryRemove),

		"handoffs.list":    r(srvHandoffsList),
		"handoffs.discard": w(srvHandoffsDiscard),
		"handoffs.prune":   w(srvHandoffsPrune),

		"conversations.list": r(srvConversationsList),
		"conversations.plan": r(srvConversationsPlan),
		"conversations.copy": w(srvConversationsCopy),

		"auto.status":         r(srvAutoStatus),
		"auto.init":           w(srvAutoInit),
		"auto.setEnabled":     w(srvAutoSetEnabled),
		"auto.sensors":        w(srvAutoSensors),
		"auto.chain":          w(srvAutoChain),
		"auto.allow":          w(srvAutoAllow),
		"auto.policy":         w(srvAutoPolicy),
		"auto.test":           w(srvAutoTest),
		"auto.simulate":       r(srvAutoSimulate),
		"auto.bootstrap":      r(srvAutoBootstrap),
		"auto.bootstrapApply": w(srvAutoBootstrapApply),

		"desktop.list":   r(srvDesktopList),
		"desktop.doctor": r(srvDesktopDoctor),
		"desktop.run":    w(srvDesktopRun),

		"diag.run": r(srvDiagRun),

		"backup.export":  w(srvBackupExport),
		"backup.restore": w(srvBackupRestore),

		"inventory.scan": r(srvInventoryScan),
		"adopt.plan":     r(srvAdoptPlan),
		"adopt.apply":    w(srvAdoptApply),

		"snapshot.list":    r(srvSnapshotList),
		"snapshot.show":    r(srvSnapshotShow),
		"snapshot.diff":    r(srvSnapshotDiff),
		"snapshot.create":  w(srvSnapshotCreate),
		"snapshot.restore": w(srvSnapshotRestore),
		"snapshot.prune":   w(srvSnapshotPrune),
		"snapshot.pin":     w(srvSnapshotPin),
		"snapshot.export":  w(srvSnapshotExport),
		"snapshot.import":  w(srvSnapshotImport),

		"system.run": w(srvSystemRun),
	}
}

// --- ayudas comunes ---

func (s *server) cfg() (*core.Config, error) { return core.Load(s.home) }

// allProfileNames devuelve `default` y los perfiles de la config, por nombre.
func allProfileNames(cfg *core.Config) []string {
	names := []string{"default"}
	var rest []string
	for n := range cfg.Profiles {
		rest = append(rest, n)
	}
	sort.Strings(rest)
	return append(names, rest...)
}

// profileType devuelve el tipo de un perfil: "default", "official" o el
// proveedor ("deepseek", "kimi", "glm").
func profileType(cfg *core.Config, name string) string {
	if name == "default" {
		return "default"
	}
	if p, ok := cfg.Profiles[name]; ok {
		return p.Type
	}
	return ""
}

// profileExists dice si el nombre es un perfil conocido (default incluido).
func profileExists(cfg *core.Config, name string) bool {
	if name == "default" {
		return true
	}
	_, ok := cfg.Profiles[name]
	return ok
}

func timeOrEmpty(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// profileLoggedIn dice si una cuenta official (o default) inició sesión. Para
// default, el archivo es ~/.claude.json (src + ".json"), fuera de ~/.claude: por
// eso va por core.DefaultHasLogin y no por core.HasLogin, que mira el cc-home.
func profileLoggedIn(home, name string) bool {
	if name == "default" {
		src, err := claudeSrc()
		if err != nil {
			return false
		}
		return core.DefaultHasLogin(src)
	}
	return core.HasLogin(home, name)
}

// --- app ---

func srvAppInfo(s *server, _ json.RawMessage) (any, error) {
	cfg, err := s.cfg()
	if err != nil {
		return nil, err
	}
	rc := rcPath("")
	installed := fileContains(rc, "ccp shell init")
	uh, _ := os.UserHomeDir()
	exe, _ := os.Executable()
	lang, src := i18n.ResolveWithSource(cfg.Lang)
	return map[string]any{
		"version":     core.Version,
		"protocol":    serveProtocol,
		"home":        s.home,
		"user_home":   uh,
		"os":          runtime.GOOS,
		"binary":      exe,
		"sensor_bin":  core.AutoHooksBin(),
		"lang":        string(lang),
		"lang_source": string(src),
		"shell": map[string]any{
			"rc":        rc,
			"installed": installed,
			"stale":     installed && installedBlock(rc) != core.ShellInit,
		},
	}, nil
}

func srvSetLang(s *server, raw json.RawMessage) (any, error) {
	p, err := params[struct {
		Lang string `json:"lang"`
	}](raw)
	if err != nil {
		return nil, err
	}
	want := strings.ToLower(strings.TrimSpace(p.Lang))
	switch want {
	case "en", "es":
	case "auto", "":
		want = ""
	default:
		return nil, badParams("idioma no soportado: %q (en, es o auto)", p.Lang)
	}
	cfg, err := s.cfg()
	if err != nil {
		return nil, err
	}
	cfg.Lang = want
	return nil, core.Save(s.home, cfg)
}

// srvFolders devuelve las carpetas que la GUI ofrece como «carpeta en
// contexto»: la de usuario, las de las reglas y las de los préstamos vivos.
func srvFolders(s *server, _ json.RawMessage) (any, error) {
	cfg, err := s.cfg()
	if err != nil {
		return nil, err
	}
	type folder struct {
		Path   string `json:"path"`
		Source string `json:"source"`
	}
	seen := map[string]bool{}
	var out []folder
	add := func(p, src string) {
		if p == "" || seen[p] {
			return
		}
		seen[p] = true
		out = append(out, folder{Path: p, Source: src})
	}
	if uh, err := os.UserHomeDir(); err == nil {
		add(uh, "home")
	}
	for _, r := range cfg.Rules {
		add(r.Path, "rule")
	}
	if h, err := core.LoadHandoffs(s.home); err == nil {
		for _, m := range h.Active {
			add(m.Cwd, "handoff")
		}
	}
	return out, nil
}

// srvResolve es `ccp resolve` con explicación: qué perfil toca en una ruta, qué
// regla lo decide y qué reglas de ancestros quedaron descartadas por estar más
// arriba.
func srvResolve(s *server, raw json.RawMessage) (any, error) {
	p, err := params[struct {
		Path string `json:"path"`
	}](raw)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(p.Path) == "" {
		return nil, badParams("falta path")
	}
	cfg, err := s.cfg()
	if err != nil {
		return nil, err
	}
	path := core.NormalizePath(p.Path)
	var matches []core.Rule
	for _, r := range cfg.Rules {
		if r.Path == path || r.Path == "/" || strings.HasPrefix(path, strings.TrimSuffix(r.Path, "/")+"/") {
			matches = append(matches, r)
		}
	}
	sort.SliceStable(matches, func(a, b int) bool { return len(matches[a].Path) > len(matches[b].Path) })
	type rule struct {
		Path    string `json:"path"`
		Profile string `json:"profile"`
	}
	profile := core.Resolve(path, cfg.Rules)
	out := map[string]any{
		"path":     path,
		"profile":  profile,
		"type":     profileType(cfg, profile),
		"rule":     nil,
		"shadowed": []rule{},
	}
	if len(matches) > 0 {
		out["rule"] = rule{Path: matches[0].Path, Profile: matches[0].Profile}
		shadowed := []rule{}
		for _, m := range matches[1:] {
			shadowed = append(shadowed, rule{Path: m.Path, Profile: m.Profile})
		}
		out["shadowed"] = shadowed
	}
	return out, nil
}

// --- perfiles ---

type srvWindow struct {
	Pct      float64 `json:"pct"`
	ResetsAt string  `json:"resets_at"`
}

type srvUsage struct {
	FiveHour  srvWindow `json:"five_hour"`
	SevenDay  srvWindow `json:"seven_day"`
	SampledAt string    `json:"sampled_at"`
}

type srvLauncher struct {
	Path     string `json:"path"`
	Label    string `json:"label"`
	Color    string `json:"color"`
	BundleID string `json:"bundle_id"`
}

type srvProfileDesktop struct {
	Eligible bool         `json:"eligible"`
	Instance bool         `json:"instance"`
	Running  bool         `json:"running"`
	Launcher *srvLauncher `json:"launcher"`
}

type srvProfile struct {
	Name       string            `json:"name"`
	Type       string            `json:"type"`
	BaseURL    string            `json:"base_url"`
	ModelPro   string            `json:"model_pro"`
	ModelFlash string            `json:"model_flash"`
	Effort     string            `json:"effort"`
	Access     string            `json:"access"` // ok | nologin | nokey
	Usage      *srvUsage         `json:"usage"`
	Sensors    string            `json:"sensors"` // installed | missing | na
	InChain    bool              `json:"in_chain"`
	Rules      int               `json:"rules"`
	Desktop    srvProfileDesktop `json:"desktop"`
}

func srvProfilesList(s *server, _ json.RawMessage) (any, error) {
	cfg, err := s.cfg()
	if err != nil {
		return nil, err
	}
	procs := core.DesktopMainProcs(desktopProcesses())
	apps := map[string]*core.DesktopApp{}
	if dir, err := core.DesktopAppsDir(); err == nil {
		if found, err := core.ListDesktopApps(dir); err == nil {
			for _, a := range found {
				apps[a.Manifest.Profile] = a
			}
		}
	}
	inChain := map[string]bool{}
	if cfg.AutoHandoff != nil {
		for _, pol := range cfg.AutoHandoff.Policies {
			for _, f := range pol.Fallback {
				inChain[strings.TrimSpace(f)] = true
			}
		}
	}
	rules := map[string]int{}
	for _, r := range cfg.Rules {
		rules[r.Profile]++
	}

	out := []srvProfile{}
	for _, name := range allProfileNames(cfg) {
		p := cfg.Profiles[name]
		sp := srvProfile{
			Name: name, Type: profileType(cfg, name),
			BaseURL: p.BaseURL, ModelPro: p.ModelPro, ModelFlash: p.ModelFlash, Effort: p.Effort,
			InChain: inChain[name], Rules: rules[name],
		}
		sp.Access = profileAccess(s.home, cfg, name)
		sp.Sensors = profileSensors(cfg, name)
		if rl, sampled, ok := core.ReadRateLimits(s.home, name); ok {
			sp.Usage = &srvUsage{
				FiveHour:  srvWindow{Pct: rl.FiveHour.UsedPercentage, ResetsAt: timeOrEmpty(rl.FiveHour.ResetsAt)},
				SevenDay:  srvWindow{Pct: rl.SevenDay.UsedPercentage, ResetsAt: timeOrEmpty(rl.SevenDay.ResetsAt)},
				SampledAt: timeOrEmpty(sampled),
			}
		}
		sp.Desktop.Eligible = core.DesktopEligible(cfg, name) == nil
		if sp.Desktop.Eligible {
			if dir, err := core.DesktopUserDataDir(s.home, name); err == nil {
				if info, err := os.Stat(dir); err == nil && info.IsDir() {
					sp.Desktop.Instance = true
				}
			}
			dataDir := core.DesktopDataDir(s.home, name)
			for _, pr := range procs {
				if pr.DataDir == dataDir {
					sp.Desktop.Running = true
					break
				}
			}
			if a := apps[name]; a != nil {
				sp.Desktop.Launcher = &srvLauncher{Path: a.Path, Label: a.Manifest.Label,
					Color: a.Manifest.Color, BundleID: a.Manifest.BundleID}
			}
		}
		out = append(out, sp)
	}
	return out, nil
}

type srvProfileFields struct {
	Name       string  `json:"name"`
	Type       string  `json:"type"`
	BaseURL    *string `json:"base_url"`
	ModelPro   *string `json:"model_pro"`
	ModelFlash *string `json:"model_flash"`
	Effort     *string `json:"effort"`
}

func srvProfilesAdd(s *server, raw json.RawMessage) (any, error) {
	p, err := params[srvProfileFields](raw)
	if err != nil {
		return nil, err
	}
	name := strings.TrimSpace(p.Name)
	if name == "" {
		return nil, badParams("falta el nombre")
	}
	if name == "default" {
		return nil, badParams("'default' es un perfil reservado")
	}
	switch {
	case p.Type == "official":
		return nil, core.ProfileAddOfficial(s.home, name)
	case core.IsProviderType(p.Type):
		var d core.Defaults
		if p.Type == "deepseek" {
			if d, err = core.GetDefaults(s.home); err != nil {
				return nil, err
			}
		} else {
			d = core.PresetDefaults(p.Type)
		}
		if p.BaseURL != nil && strings.TrimSpace(*p.BaseURL) != "" {
			d.BaseURL = strings.TrimSpace(*p.BaseURL)
		}
		if p.ModelPro != nil && strings.TrimSpace(*p.ModelPro) != "" {
			d.ModelPro = strings.TrimSpace(*p.ModelPro)
		}
		if p.ModelFlash != nil && strings.TrimSpace(*p.ModelFlash) != "" {
			d.ModelFlash = strings.TrimSpace(*p.ModelFlash)
		}
		if p.Effort != nil && strings.TrimSpace(*p.Effort) != "" {
			d.Effort = strings.TrimSpace(*p.Effort)
		}
		return nil, core.ProfileAddProvider(s.home, name, p.Type, d)
	}
	return nil, badParams("tipo desconocido: %q (official, %s)", p.Type, strings.Join(core.ProviderTypes(), ", "))
}

func srvProfilesUpdate(s *server, raw json.RawMessage) (any, error) {
	p, err := params[srvProfileFields](raw)
	if err != nil {
		return nil, err
	}
	return nil, core.ProfileUpdate(s.home, p.Name, core.ProfileFields{
		BaseURL: p.BaseURL, ModelPro: p.ModelPro, ModelFlash: p.ModelFlash, Effort: p.Effort,
	})
}

func srvProfilesRename(s *server, raw json.RawMessage) (any, error) {
	p, err := params[struct {
		From string `json:"from"`
		To   string `json:"to"`
	}](raw)
	if err != nil {
		return nil, err
	}
	res, err := core.ProfileRename(s.home, p.From, strings.TrimSpace(p.To))
	if err != nil {
		return nil, err
	}
	// relogin: la GUI lo enseña al confirmar (B7). "ok" se queda para no cambiar
	// la forma de lo que ya se respondía.
	return map[string]any{"ok": true, "relogin": res.Relogin}, nil
}

func srvProfilesRemove(s *server, raw json.RawMessage) (any, error) {
	p, err := params[struct {
		Name string `json:"name"`
	}](raw)
	if err != nil {
		return nil, err
	}
	if _, err := autoSnapshot(s.home, "pre-profile-rm"); err != nil {
		return nil, fmt.Errorf("no se pudo guardar el snapshot de seguridad y no se borró nada: %w", err)
	}
	return nil, core.ProfileRm(s.home, p.Name)
}

func srvProfilesSetKey(s *server, raw json.RawMessage) (any, error) {
	p, err := params[struct {
		Name string `json:"name"`
		Key  string `json:"key"`
	}](raw)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(p.Key) == "" {
		return nil, badParams("la key está vacía")
	}
	return nil, core.ProfileSetKey(s.home, p.Name, strings.TrimSpace(p.Key))
}

func srvProfilesSync(s *server, raw json.RawMessage) (any, error) {
	p, err := params[struct {
		Name string `json:"name"`
	}](raw)
	if err != nil {
		return nil, err
	}
	drifts, err := core.ProfileSyncReport(s.home, p.Name)
	if err != nil {
		// Una respuesta de error no lleva resultado: lo reunido hasta el fallo (lo
		// ya adoptado en perfiles anteriores, sus copias de rescate) se deja
		// pendiente para el siguiente sync en vez de perderlo.
		core.SavePendingDrift(s.home, drifts)
		return nil, err
	}
	// drift: lo que /config había cambiado en cada perfil (B6). Siempre array, y
	// cada lista también, para que la GUI no tenga que distinguir null de vacío.
	// "ok" se queda: es lo que este método respondía antes y la GUI ya lo lee.
	// Los campos de skipped en adelante son aditivos: un cliente viejo los ignora.
	type row struct {
		Profile        string   `json:"profile"`
		Adopted        []string `json:"adopted"`
		Removed        []string `json:"removed"`
		Conflicts      []string `json:"conflicts"`
		Invalid        string   `json:"invalid"`
		Skipped        []string `json:"skipped"`
		Unsaved        []string `json:"unsaved"`
		UnsavedError   string   `json:"unsaved_error"`
		Rescued        string   `json:"rescued"`
		Unattributed   bool     `json:"unattributed"`
		NotRegenerated bool     `json:"not_regenerated"`
		// mcp: lo que la regeneración proyectó a cada destino (Fase B), y su
		// error si falló. Aditivo: un cliente viejo lo ignora.
		MCP    []core.MCPProjection `json:"mcp"`
		MCPErr string               `json:"mcp_error"`
	}
	nz, nzMCP := nzStrings, nzMCPProjections
	out := []row{}
	for _, d := range drifts {
		out = append(out, row{
			Profile: d.Profile, Adopted: nz(d.Adopted), Removed: nz(d.Removed), Conflicts: nz(d.Conflicts), Invalid: d.Invalid,
			Skipped: nz(d.Skipped), Unsaved: nz(d.Unsaved), UnsavedError: d.UnsavedErr, Rescued: d.Rescued,
			Unattributed: d.Unattributed, NotRegenerated: d.NotRegenerated,
			MCP: nzMCP(d.MCP), MCPErr: d.MCPErr,
		})
	}
	return map[string]any{"ok": true, "drift": out}, nil
}

// srvProfilesDrift es `ccp profile sync --check` para la GUI: qué cambiaría la
// proyección de cada perfil, sin escribir nada. Va por r() (lectura) a
// propósito: no toca disco, así que no tiene por qué serializarse con las
// escrituras.
func srvProfilesDrift(s *server, raw json.RawMessage) (any, error) {
	p, err := params[struct {
		Name string `json:"name"`
	}](raw)
	if err != nil {
		return nil, err
	}
	names := []string{p.Name}
	if p.Name == "" {
		if names, err = core.ProfileList(s.home); err != nil {
			return nil, err
		}
	}
	type row struct {
		Profile string `json:"profile"`
		// Stale es la misma regla que decide el exit code del CLI: lo que un
		// sync arreglaría. La GUI no la recalcula sumando listas, que es como se
		// desincronizan dos front-ends sobre el mismo dato.
		Stale        bool                 `json:"stale"`
		MCP          []core.MCPProjection `json:"mcp"`
		Artifacts    []string             `json:"artifacts"`
		Settings     bool                 `json:"settings"`
		Instructions bool                 `json:"instructions"`
		Error        string               `json:"error"`
	}
	out := []row{}
	for _, n := range names {
		c, err := core.ProfileProjectionCheck(s.home, n)
		if err != nil {
			return nil, err
		}
		out = append(out, row{Profile: c.Profile, Stale: c.Stale(),
			MCP: nzMCPProjections(c.MCP), Artifacts: nzStrings(c.Artifacts),
			Settings: c.Settings, Instructions: c.Instructions, Error: c.Err})
	}
	return map[string]any{"drift": out}, nil
}

// nzStrings / nzMCPProjections: ninguna lista del protocolo sale null, para que
// la GUI no tenga que distinguirlo de vacío.
func nzStrings(v []string) []string {
	if v == nil {
		return []string{}
	}
	return v
}

func nzMCPProjections(v []core.MCPProjection) []core.MCPProjection {
	out := make([]core.MCPProjection, 0, len(v))
	for _, p := range v {
		p.Written, p.Removed = nzStrings(p.Written), nzStrings(p.Removed)
		p.Conflicts, p.RemoteSkipped = nzStrings(p.Conflicts), nzStrings(p.RemoteSkipped)
		out = append(out, p)
	}
	return out
}

// effKindName es el nombre estable de cada sección de la configuración efectiva.
func effKindName(k core.EffKind) string {
	switch k {
	case core.EffInstructions:
		return "instructions"
	case core.EffEnv:
		return "env"
	case core.EffPermissions:
		return "permissions"
	case core.EffHooks:
		return "hooks"
	case core.EffPlugins:
		return "plugins"
	case core.EffSensors:
		return "sensors"
	case core.EffMCP:
		return "mcp"
	case core.EffDeny:
		return "deny"
	case core.EffAsk:
		return "ask"
	case core.EffSettings:
		return "settings"
	}
	return "other"
}

func srvProfilesEffective(s *server, raw json.RawMessage) (any, error) {
	p, err := params[struct {
		Name string `json:"name"`
	}](raw)
	if err != nil {
		return nil, err
	}
	src, err := claudeSrc()
	if err != nil {
		return nil, err
	}
	eff, err := core.ProfileEffective(s.home, p.Name, src)
	if err != nil {
		return nil, err
	}
	type row struct {
		Key      string `json:"key"`
		Value    string `json:"value"`
		Origin   string `json:"origin"`
		Shadowed bool   `json:"shadowed"`
		// applies_to: dónde se lee de verdad esta fila (ADR 0016). Campo añadido,
		// no cambio de forma: un cliente viejo lo ignora.
		AppliesTo []string `json:"applies_to"`
	}
	type section struct {
		Kind  string `json:"kind"`
		File  string `json:"file"`
		Error string `json:"error"`
		Rows  []row  `json:"rows"`
	}
	out := struct {
		Profile  string    `json:"profile"`
		Sections []section `json:"sections"`
	}{Profile: eff.Profile, Sections: []section{}}
	for _, sec := range eff.Sections {
		sc := section{Kind: effKindName(sec.Kind), File: sec.File, Error: errString(sec.Err), Rows: []row{}}
		for _, r := range sec.Rows {
			sc.Rows = append(sc.Rows, row{Key: r.Key, Value: r.Value, Origin: r.Origin.String(),
				Shadowed: r.Shadowed, AppliesTo: nzStrings(r.AppliesTo)})
		}
		out.Sections = append(out.Sections, sc)
	}
	return out, nil
}

// regenerate rehace el cc-home de un perfil tras tocar su overlay.
func (s *server) regenerate(name string) error {
	src, err := claudeSrc()
	if err != nil {
		return err
	}
	return core.CfgRegenerate(s.home, name, src)
}

func srvOverlayEnvSet(s *server, raw json.RawMessage) (any, error) {
	p, err := params[struct {
		Name  string `json:"name"`
		Key   string `json:"key"`
		Value string `json:"value"`
	}](raw)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(p.Key) == "" {
		return nil, badParams("falta la clave")
	}
	if err := core.CfgInitOverlay(s.home, p.Name); err != nil {
		return nil, err
	}
	if err := core.OverlayEnvSet(s.home, p.Name, strings.TrimSpace(p.Key), p.Value); err != nil {
		return nil, err
	}
	return nil, s.regenerate(p.Name)
}

func srvOverlayEnvDel(s *server, raw json.RawMessage) (any, error) {
	p, err := params[struct {
		Name string `json:"name"`
		Key  string `json:"key"`
	}](raw)
	if err != nil {
		return nil, err
	}
	if err := core.OverlayEnvDel(s.home, p.Name, p.Key); err != nil {
		return nil, err
	}
	return nil, s.regenerate(p.Name)
}

func srvOverlayRuleAdd(s *server, raw json.RawMessage) (any, error) {
	p, err := params[struct {
		Name string `json:"name"`
		Text string `json:"text"`
	}](raw)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(p.Text) == "" {
		return nil, badParams("la instrucción está vacía")
	}
	if err := core.CfgInitOverlay(s.home, p.Name); err != nil {
		return nil, err
	}
	added, err := core.InstructRuleAdd(core.ProfileInstrFile(s.home, p.Name), p.Text)
	if err != nil {
		return nil, err
	}
	if err := s.regenerate(p.Name); err != nil {
		return nil, err
	}
	return map[string]any{"added": added}, nil
}

func srvOverlayRuleRemove(s *server, raw json.RawMessage) (any, error) {
	p, err := params[struct {
		Name  string `json:"name"`
		Index int    `json:"index"` // 1-based entre las reglas del overlay
	}](raw)
	if err != nil {
		return nil, err
	}
	if err := core.InstructRuleRm(core.ProfileInstrFile(s.home, p.Name), p.Index); err != nil {
		return nil, err
	}
	return nil, s.regenerate(p.Name)
}

// --- reglas ---

func srvRulesList(s *server, _ json.RawMessage) (any, error) {
	cfg, err := s.cfg()
	if err != nil {
		return nil, err
	}
	type rule struct {
		Path    string `json:"path"`
		Profile string `json:"profile"`
		Depth   int    `json:"depth"`  // cuántas reglas de ancestros tiene encima
		Parent  string `json:"parent"` // la regla del ancestro más cercano ("" = ninguna)
		Exists  bool   `json:"exists"` // la carpeta sigue existiendo
		Orphan  bool   `json:"orphan"` // su perfil ya no existe
	}
	rules := append([]core.Rule(nil), cfg.Rules...)
	sort.SliceStable(rules, func(a, b int) bool { return rules[a].Path < rules[b].Path })
	out := []rule{}
	for _, r := range rules {
		rr := rule{Path: r.Path, Profile: r.Profile, Orphan: !profileExists(cfg, r.Profile)}
		if info, err := os.Stat(r.Path); err == nil && info.IsDir() {
			rr.Exists = true
		}
		for _, o := range rules {
			if o.Path != r.Path && (o.Path == "/" || strings.HasPrefix(r.Path, strings.TrimSuffix(o.Path, "/")+"/")) {
				rr.Depth++
				if len(o.Path) > len(rr.Parent) {
					rr.Parent = o.Path
				}
			}
		}
		out = append(out, rr)
	}
	return out, nil
}

func srvRulesSet(s *server, raw json.RawMessage) (any, error) {
	p, err := params[struct {
		Path    string `json:"path"`
		Profile string `json:"profile"`
	}](raw)
	if err != nil {
		return nil, err
	}
	path, err := core.RuleSet(s.home, p.Path, p.Profile)
	if err != nil {
		return nil, err
	}
	return map[string]any{"path": path}, nil
}

func srvRulesRemove(s *server, raw json.RawMessage) (any, error) {
	p, err := params[struct {
		Path string `json:"path"`
	}](raw)
	if err != nil {
		return nil, err
	}
	path, err := core.RuleDel(s.home, p.Path)
	if err != nil {
		return nil, err
	}
	return map[string]any{"path": path}, nil
}

func srvRulesClear(s *server, _ json.RawMessage) (any, error) {
	return nil, core.RulesClear(s.home)
}

// --- configuración ---

func srvConfigGet(s *server, _ json.RawMessage) (any, error) {
	d, err := core.GetDefaults(s.home)
	if err != nil {
		return nil, err
	}
	gui, _ := core.GetGuiEditor(s.home)
	editor, _ := core.GetEditor(s.home, os.Getenv("EDITOR"))
	return map[string]any{
		"defaults": map[string]string{
			"base_url": d.BaseURL, "model_pro": d.ModelPro, "model_flash": d.ModelFlash, "effort": d.Effort,
		},
		"editor":     editor,
		"gui_editor": gui,
		"providers":  core.ProviderTypes(),
	}, nil
}

func srvConfigSetDefault(s *server, raw json.RawMessage) (any, error) {
	p, err := params[struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}](raw)
	if err != nil {
		return nil, err
	}
	return nil, core.SetDefault(s.home, p.Key, p.Value)
}

func srvConfigReset(s *server, _ json.RawMessage) (any, error) {
	return nil, core.ResetDefaults(s.home)
}

func srvConfigSetEditor(s *server, raw json.RawMessage) (any, error) {
	p, err := params[struct {
		Editor    *string `json:"editor"`
		GuiEditor *string `json:"gui_editor"`
	}](raw)
	if err != nil {
		return nil, err
	}
	if p.Editor != nil {
		if err := core.SetEditor(s.home, strings.TrimSpace(*p.Editor)); err != nil {
			return nil, err
		}
	}
	if p.GuiEditor != nil {
		if err := core.SetGuiEditor(s.home, strings.TrimSpace(*p.GuiEditor)); err != nil {
			return nil, err
		}
	}
	return nil, nil
}

// --- memoria de Claude (instruct) ---

type srvMemoryParams struct {
	Scope   string `json:"scope"`   // global | profile | project
	Profile string `json:"profile"` // el perfil mirado (scope profile)
	Cwd     string `json:"cwd"`     // la carpeta en contexto (scope project)
	Type    string `json:"type"`    // rule | hook | mcp | agent | command | skill
	Text    string `json:"text"`
	Name    string `json:"name"` // hook/mcp/agent/command/skill
	Index   int    `json:"index"`
}

// memoryCtx arma el contexto de instruct para lo que la GUI está mirando, no
// para la terminal: el perfil elegido en la pantalla y la raíz git de la
// carpeta en contexto.
func (s *server) memoryCtx(p srvMemoryParams) (core.InstructCtx, error) {
	src, err := claudeSrc()
	if err != nil {
		return core.InstructCtx{}, err
	}
	repo := ""
	if p.Cwd != "" {
		repo = gitRepoRoot(p.Cwd)
	}
	active := p.Profile
	if active == "" {
		active = "default"
	}
	return core.InstructCtx{Home: s.home, Src: src, RepoRoot: repo, ActiveProfile: active}, nil
}

func srvMemoryList(s *server, raw json.RawMessage) (any, error) {
	p, err := params[srvMemoryParams](raw)
	if err != nil {
		return nil, err
	}
	ctx, err := s.memoryCtx(p)
	if err != nil {
		return nil, err
	}
	type item struct {
		Index int    `json:"index"`
		Type  string `json:"type"`
		Text  string `json:"text"`
		Where string `json:"where"`
	}
	out := struct {
		Scope     string `json:"scope"`
		Available bool   `json:"available"`
		Reason    string `json:"reason"`
		Repo      string `json:"repo"`
		Items     []item `json:"items"`
	}{Scope: p.Scope, Available: true, Repo: ctx.RepoRoot, Items: []item{}}
	switch {
	case p.Scope == "profile" && ctx.ActiveProfile == "default":
		out.Available, out.Reason = false, "default_no_overlay"
		return out, nil
	case p.Scope == "project" && ctx.RepoRoot == "":
		out.Available, out.Reason = false, "no_repo"
		return out, nil
	}
	rows, err := core.InstructList(ctx, p.Scope)
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		where, _ := core.InstructDest(p.Scope, r.Type, s.home, ctx.ActiveProfile, ctx.Src, ctx.RepoRoot)
		out.Items = append(out.Items, item{Index: r.Index, Type: r.Type, Text: r.Text, Where: where})
	}
	return out, nil
}

var memorySlugRe = regexp.MustCompile(`[^a-z0-9]+`)

// memorySlug convierte un nombre en un nombre de archivo inocuo.
func memorySlug(s string) string {
	s = memorySlugRe.ReplaceAllString(strings.ToLower(strings.TrimSpace(s)), "-")
	s = strings.Trim(s, "-")
	if len(s) > 48 {
		s = strings.Trim(s[:48], "-")
	}
	return s
}

func srvMemoryAdd(s *server, raw json.RawMessage) (any, error) {
	p, err := params[srvMemoryParams](raw)
	if err != nil {
		return nil, err
	}
	ctx, err := s.memoryCtx(p)
	if err != nil {
		return nil, err
	}
	switch p.Type {
	case "rule", "hook", "mcp":
		text := p.Text
		if p.Type != "rule" {
			// hook y mcp viajan como nombre={json}, igual que en la CLI.
			if strings.TrimSpace(p.Name) == "" {
				return nil, badParams("falta el nombre del %s", p.Type)
			}
			var probe any
			if err := json.Unmarshal([]byte(p.Text), &probe); err != nil {
				return nil, badParams("el JSON no es válido: %v", err)
			}
			text = strings.TrimSpace(p.Name) + "=" + strings.TrimSpace(p.Text)
		}
		res, err := core.InstructAdd(ctx, p.Scope, p.Type, text)
		if err != nil {
			return nil, err
		}
		return map[string]any{"type": res.Type, "scope": res.Scope, "dest": res.Dest,
			"name": res.Name, "duplicate": res.Duplicate}, nil
	case "agent", "command", "skill":
		// Los mismos tres pasos que siguen los /ccp:remember-*: pedir el destino
		// oficial, escribir el archivo ahí y registrarlo en el manifiesto.
		slug := memorySlug(p.Name)
		if slug == "" {
			return nil, badParams("falta un nombre para el %s", p.Type)
		}
		dir, err := core.InstructDestCmd(ctx, p.Scope, p.Type)
		if err != nil {
			return nil, err
		}
		desc := firstLine(p.Text)
		var path, body string
		switch p.Type {
		case "skill":
			path = filepath.Join(dir, slug, "SKILL.md")
			body = fmt.Sprintf("---\nname: %s\ndescription: %s\n---\n\n%s\n", slug, desc, strings.TrimSpace(p.Text))
		default:
			path = filepath.Join(dir, slug+".md")
			body = fmt.Sprintf("---\nname: %s\ndescription: %s\n---\n\n%s\n", slug, desc, strings.TrimSpace(p.Text))
		}
		if _, err := os.Stat(path); err == nil {
			return nil, badParams("ya existe %s", path)
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, err
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			return nil, err
		}
		if err := core.InstructRecord(ctx, p.Scope, p.Type, path, desc); err != nil {
			return nil, err
		}
		return map[string]any{"type": p.Type, "scope": p.Scope, "dest": path}, nil
	}
	return nil, badParams("tipo desconocido: %q", p.Type)
}

// firstLine devuelve la primera línea no vacía, recortada a 120 caracteres.
func firstLine(s string) string {
	for _, l := range strings.Split(s, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			if len(l) > 120 {
				l = strings.TrimSpace(l[:120])
			}
			return strings.ReplaceAll(l, "\"", "'")
		}
	}
	return ""
}

func srvMemoryRemove(s *server, raw json.RawMessage) (any, error) {
	p, err := params[srvMemoryParams](raw)
	if err != nil {
		return nil, err
	}
	ctx, err := s.memoryCtx(p)
	if err != nil {
		return nil, err
	}
	res, err := core.InstructRm(ctx, p.Scope, p.Index)
	if err != nil {
		return nil, err
	}
	return map[string]any{"scope": res.Scope, "type": res.Type, "ref": res.Ref, "was_rule": res.WasRule}, nil
}
