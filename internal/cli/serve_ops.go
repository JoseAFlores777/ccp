package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/JoseAFlores777/ccp/internal/core"
)

// serve_ops.go — los métodos de `ccp serve` para préstamos, conversaciones,
// rotación, Desktop, diagnóstico, copias de seguridad y sistema.

// profileAccess dice si una cuenta puede usarse: "ok", "nokey" (proveedor sin
// key) o "nologin" (cuenta de Anthropic sin sesión iniciada).
func profileAccess(home string, cfg *core.Config, name string) string {
	if p, ok := cfg.Profiles[name]; ok && core.IsProviderType(p.Type) {
		if _, ok := core.GetKey(home, name); ok {
			return "ok"
		}
		return "nokey"
	}
	if profileLoggedIn(home, name) {
		return "ok"
	}
	return "nologin"
}

func profileSensors(cfg *core.Config, name string) string {
	switch {
	case name == "default":
		return "na"
	case core.AutoHooksEnabled(cfg, name):
		return "installed"
	}
	return "missing"
}

// --- préstamos (handoffs) ---

func srvHandoffsList(s *server, _ json.RawMessage) (any, error) {
	h, err := core.LoadHandoffs(s.home)
	if err != nil {
		return nil, err
	}
	if err := h.CheckUsable(); err != nil {
		return nil, err
	}
	type active struct {
		Session string   `json:"session"`
		Title   string   `json:"title"`
		From    string   `json:"from"`
		To      string   `json:"to"`
		Cwd     string   `json:"cwd"`
		Since   string   `json:"since"`
		Auto    bool     `json:"auto"`
		Hops    []string `json:"hops"`
		Present bool     `json:"present"` // el transcript sigue en el destino
	}
	type archived struct {
		Session    string `json:"session"`
		From       string `json:"from"`
		To         string `json:"to"`
		ReturnedAs string `json:"returned_as"`
		Since      string `json:"since"`
		Ended      string `json:"ended"`
	}
	out := struct {
		Active   []active   `json:"active"`
		Archived []archived `json:"archived"`
	}{Active: []active{}, Archived: []archived{}}
	for _, m := range h.Active {
		a := active{Session: m.Session, Title: m.Title, From: m.From, To: m.To, Cwd: m.Cwd,
			Since: m.Since, Auto: m.Auto, Hops: append([]string{}, m.Hops...)}
		if toCC, err := core.CCHome(s.home, m.To); err == nil {
			if _, err := os.Stat(filepath.Join(core.ProjectDir(toCC, m.Slug), m.Session+".jsonl")); err == nil {
				a.Present = true
			}
		}
		out.Active = append(out.Active, a)
	}
	for i := len(h.Archived) - 1; i >= 0; i-- {
		a := h.Archived[i]
		out.Archived = append(out.Archived, archived{Session: a.Session, From: a.From, To: a.To,
			ReturnedAs: a.ReturnedAs, Since: a.Since, Ended: a.Ended})
	}
	return out, nil
}

func srvHandoffsDiscard(s *server, raw json.RawMessage) (any, error) {
	p, err := params[struct {
		Session string `json:"session"`
		Cwd     string `json:"cwd"`
	}](raw)
	if err != nil {
		return nil, err
	}
	if p.Session == "" {
		return nil, badParams("falta session")
	}
	m, err := core.HandoffDiscard(s.home, p.Cwd, p.Session, time.Now())
	if err != nil {
		return nil, err
	}
	return map[string]any{"session": m.Session, "from": m.From, "to": m.To}, nil
}

func srvHandoffsPrune(s *server, raw json.RawMessage) (any, error) {
	p, err := params[struct {
		Keep *int `json:"keep"`
	}](raw)
	if err != nil {
		return nil, err
	}
	keep := defaultHandoffKeep
	if p.Keep != nil {
		if *p.Keep < 0 {
			return nil, badParams("keep no puede ser negativo")
		}
		keep = *p.Keep
	}
	removed, err := core.HandoffPrune(s.home, keep)
	if err != nil {
		return nil, err
	}
	return map[string]any{"removed": removed, "kept": keep}, nil
}

// --- conversaciones ---

type srvLoan struct {
	From string `json:"from"`
	To   string `json:"to"`
	Auto bool   `json:"auto"`
}

type srvConversation struct {
	Profile      string   `json:"profile"`
	UUID         string   `json:"uuid"`
	Title        string   `json:"title"`
	Cwd          string   `json:"cwd"`
	LastActivity string   `json:"last_activity"`
	Bytes        int64    `json:"bytes"`
	InDesktop    bool     `json:"in_desktop"`
	Archived     bool     `json:"archived"`
	Loan         *srvLoan `json:"loan"`
	Transcript   string   `json:"transcript"`
}

func srvConversationsList(s *server, raw json.RawMessage) (any, error) {
	p, err := params[struct {
		Profile  string `json:"profile"`
		Cwd      string `json:"cwd"`
		Limit    int    `json:"limit"`
		Archived bool   `json:"archived"`
	}](raw)
	if err != nil {
		return nil, err
	}
	cfg, err := s.cfg()
	if err != nil {
		return nil, err
	}
	loans := map[string]srvLoan{}
	if h, err := core.LoadHandoffs(s.home); err == nil {
		for _, m := range h.Active {
			loans[m.Session] = srvLoan{From: m.From, To: m.To, Auto: m.Auto}
		}
	}
	cwd := ""
	if p.Cwd != "" {
		cwd = strings.TrimSuffix(core.NormalizePath(p.Cwd), "/")
	}
	var all []core.Conversation
	for _, name := range allProfileNames(cfg) {
		if p.Profile != "" && name != p.Profile {
			continue
		}
		ccHome, err := core.CCHome(s.home, name)
		if err != nil {
			continue
		}
		dataDir := ""
		if core.DesktopEligible(cfg, name) == nil {
			dataDir, _ = core.DesktopUserDataDir(s.home, name)
		}
		all = append(all, core.ProfileConversations(name, ccHome, dataDir)...)
	}
	sort.SliceStable(all, func(a, b int) bool { return all[a].LastActivity.After(all[b].LastActivity) })

	limit := p.Limit
	if limit <= 0 {
		limit = 300
	}
	out := struct {
		Total int               `json:"total"`
		Items []srvConversation `json:"items"`
	}{Items: []srvConversation{}}
	for _, c := range all {
		if c.Archived && !p.Archived {
			continue
		}
		if cwd != "" && c.Cwd != cwd && !strings.HasPrefix(c.Cwd, cwd+"/") {
			continue
		}
		out.Total++
		if len(out.Items) >= limit {
			continue
		}
		item := srvConversation{Profile: c.Profile, UUID: c.UUID, Title: c.Title, Cwd: c.Cwd,
			LastActivity: timeOrEmpty(c.LastActivity), Bytes: c.Bytes, InDesktop: c.InDesktop,
			Archived: c.Archived, Transcript: c.Transcript}
		if l, ok := loans[c.UUID]; ok && (l.From == c.Profile || l.To == c.Profile) {
			loan := l
			item.Loan = &loan
		}
		out.Items = append(out.Items, item)
	}
	return out, nil
}

type srvCopyParams struct {
	UUID   string `json:"uuid"`
	From   string `json:"from"`
	To     string `json:"to"`
	NoOpen bool   `json:"no_open"`
}

// srvConversationsPlan dice, sin tocar nada, qué haría `ccp desktop copy`: el
// resultado sobre el transcript del destino y si el enlace de importación
// llegaría a su ventana. Es el paso «antes de confirmar» del asistente.
func srvConversationsPlan(s *server, raw json.RawMessage) (any, error) {
	p, err := params[srvCopyParams](raw)
	if err != nil {
		return nil, err
	}
	cfg, err := s.cfg()
	if err != nil {
		return nil, err
	}
	if !profileExists(cfg, p.From) || !profileExists(cfg, p.To) {
		return nil, badParams("perfil desconocido")
	}
	if err := core.DesktopEligible(cfg, p.To); err != nil {
		return nil, err
	}
	srcCC, err := core.CCHome(s.home, p.From)
	if err != nil {
		return nil, err
	}
	src, found := core.DesktopSession{}, false
	if core.DesktopEligible(cfg, p.From) == nil {
		dataDir, _ := core.DesktopUserDataDir(s.home, p.From)
		for _, ds := range core.DesktopSessions(p.From, dataDir, srcCC) {
			if ds.UUID == strings.ToLower(p.UUID) && ds.Transcript != "" {
				src, found = ds, true
				break
			}
		}
	}
	if !found {
		if src, found = core.DesktopTranscriptSession(p.From, srcCC, p.UUID); !found {
			return nil, &serveError{Code: "not_found", Message: "no encuentro esa conversación en " + p.From}
		}
	}
	dstCC, err := core.CCHome(s.home, p.To)
	if err != nil {
		return nil, err
	}
	plan, err := core.PlanDesktopCopy(src, p.To, dstCC)
	if err != nil {
		return nil, err
	}
	outcome, oerr := core.InspectDesktopCopy(plan)
	outcomeName := string(outcome)
	if errors.Is(oerr, core.ErrDesktopCopyDiverged) {
		outcomeName = "diverged"
	} else if oerr != nil {
		return nil, oerr
	}
	dstData, err := core.DesktopUserDataDir(s.home, p.To)
	if err != nil {
		return nil, err
	}
	indexed := core.DesktopIndexed(dstData, plan.UUID)
	running := desktopProfileRunning(s.home, p.To)

	route := map[string]any{"supported": desktopImportSupported()}
	if desktopImportSupported() {
		t := core.DesktopImportRoute(desktopImportProbe(s.home, p.To))
		route["app"], route["running"], route["refuse"], route["detail"] = t.App, t.Running, t.Refuse, t.Detail
	}
	busy := -1
	if info, err := os.Stat(plan.SrcTranscript); err == nil {
		if age := time.Since(info.ModTime()); age < desktopCopyBusyWindow {
			busy = int(age.Seconds())
		}
	}
	return map[string]any{
		"uuid": plan.UUID, "title": plan.Title, "cwd": plan.Cwd, "cwd_exists": plan.Cwd != "" && dirExists(plan.Cwd),
		"from": plan.From, "to": plan.To, "src": plan.SrcTranscript, "dst": plan.DstTranscript,
		"outcome": outcomeName, "indexed": indexed, "dst_running": running,
		"update_open":  outcome == core.DesktopCopyUpdated && indexed && running,
		"route":        route,
		"busy_seconds": busy,
	}, nil
}

// srvConversationsCopy ejecuta `ccp desktop copy` tal cual: la GUI enseña la
// misma explicación que la terminal, y el código de salida dice si la sesión
// quedó en la barra lateral del destino.
func srvConversationsCopy(s *server, raw json.RawMessage) (any, error) {
	p, err := params[srvCopyParams](raw)
	if err != nil {
		return nil, err
	}
	if !core.IsSessionID(p.UUID) || p.From == "" || p.To == "" {
		return nil, badParams("hacen falta uuid (completo), from y to")
	}
	args := []string{"desktop", "copy", p.UUID, p.To, "--from", p.From}
	if p.NoOpen {
		args = append(args, "--no-open")
	}
	return runInProcess(args...), nil
}

// --- rotación (auto-handoff) ---

type srvPolicyParams struct {
	Threshold        int    `json:"threshold"`
	MinDwell         string `json:"min_dwell"`
	MaxHops          int    `json:"max_hops"`
	ReturnCheck      string `json:"return_check"`
	ReturnIdle       string `json:"return_idle"`
	CooldownStrategy string `json:"cooldown_strategy"`
	CooldownFallback string `json:"cooldown_fallback"`
}

func effectiveParams(e core.EffectivePolicy) srvPolicyParams {
	return srvPolicyParams{
		Threshold: e.Threshold, MinDwell: e.MinDwell.String(), MaxHops: e.MaxHops,
		ReturnCheck: e.ReturnCheck.String(), ReturnIdle: e.ReturnIdle.String(),
		CooldownStrategy: e.CooldownStrategy, CooldownFallback: e.CooldownFallback.String(),
	}
}

func (s *server) cwdOrHome(cwd string) string {
	if strings.TrimSpace(cwd) != "" {
		return core.NormalizePath(cwd)
	}
	if uh, err := os.UserHomeDir(); err == nil {
		return uh
	}
	return "/"
}

func srvAutoStatus(s *server, raw json.RawMessage) (any, error) {
	p, err := params[struct {
		Cwd    string `json:"cwd"`
		Policy string `json:"policy"`
	}](raw)
	if err != nil {
		return nil, err
	}
	cfg, err := s.cfg()
	if err != nil {
		return nil, err
	}
	cwd := s.cwdOrHome(p.Cwd)
	out := map[string]any{"present": cfg.AutoHandoff != nil, "cwd": cwd,
		"primary": core.Resolve(cwd, cfg.Rules)}
	if cfg.AutoHandoff == nil {
		return out, nil
	}
	block := cfg.AutoHandoff
	name := p.Policy
	if name == "" {
		name = "default"
	}
	pol := block.Policies[name]
	out["enabled"] = block.Enabled
	out["policies"] = core.AutoPolicyNames(cfg)
	out["policy"] = name
	out["raw"] = srvPolicyParams{Threshold: pol.Threshold, MinDwell: pol.MinDwell, MaxHops: pol.MaxHops,
		ReturnCheck: pol.ReturnCheck, ReturnIdle: pol.ReturnIdle,
		CooldownStrategy: pol.Cooldown.Strategy, CooldownFallback: pol.Cooldown.Fallback}
	allow := map[string][]string{}
	for k, v := range block.AllowFrom {
		allow[k] = append([]string{}, v...)
	}
	out["allow_from"] = allow
	out["allow_declared"] = block.AllowFrom != nil
	out["hooks"] = append([]string{}, block.Hooks...)

	eff, err := pol.Effective(name)
	if err != nil {
		out["error"] = err.Error()
		out["fallback"] = append([]string{}, pol.Fallback...)
		return out, nil
	}
	out["params"] = effectiveParams(eff)
	out["fallback"] = eff.Fallback

	rc, rerr := core.ResolveAutoChain(s.home, cfg, name, cwd)
	if rerr != nil {
		out["error"] = rerr.Error()
		return out, nil
	}
	gate := core.AutoGateFor(cfg, rc.Primary)
	out["primary"] = rc.Primary
	out["gate"] = map[string]any{"absent": gate.Absent, "declared": gate.Declared, "entry": gate.Entry}
	allowed := map[string]bool{}
	for _, f := range rc.Fallback {
		allowed[f] = true
	}
	type link struct {
		Profile string `json:"profile"`
		Type    string `json:"type"`
		Order   int    `json:"order"`   // posición en la política (1-based)
		Allowed bool   `json:"allowed"` // el permiso deja usarla desde el primario
		Reason  string `json:"reason"`  // no_entry | not_in_entry
		Access  string `json:"access"`
		Sensors string `json:"sensors"`
	}
	chain := []link{}
	for i, f := range eff.Fallback {
		if f == rc.Primary {
			continue
		}
		l := link{Profile: f, Type: profileType(cfg, f), Order: i + 1, Allowed: allowed[f],
			Access: profileAccess(s.home, cfg, f), Sensors: profileSensors(cfg, f)}
		if !l.Allowed {
			if !gate.Declared {
				l.Reason = "no_entry"
			} else {
				l.Reason = "not_in_entry"
			}
		}
		chain = append(chain, l)
	}
	out["chain"] = chain
	return out, nil
}

func srvAutoInit(s *server, raw json.RawMessage) (any, error) {
	p, err := params[struct {
		Force bool `json:"force"`
	}](raw)
	if err != nil {
		return nil, err
	}
	return nil, core.AutoInit(s.home, p.Force)
}

func srvAutoSetEnabled(s *server, raw json.RawMessage) (any, error) {
	p, err := params[struct {
		Enabled bool `json:"enabled"`
	}](raw)
	if err != nil {
		return nil, err
	}
	return nil, core.AutoSetEnabled(s.home, p.Enabled)
}

func srvAutoSensors(s *server, raw json.RawMessage) (any, error) {
	p, err := params[struct {
		Profiles []string `json:"profiles"`
		Install  bool     `json:"install"`
	}](raw)
	if err != nil {
		return nil, err
	}
	args := []string{"auto", "uninstall"}
	if p.Install {
		args[1] = "install"
	}
	return runInProcess(append(args, p.Profiles...)...), nil
}

func srvAutoChain(s *server, raw json.RawMessage) (any, error) {
	p, err := params[struct {
		Op     string   `json:"op"`
		Policy string   `json:"policy"`
		Cwd    string   `json:"cwd"`
		Names  []string `json:"names"`
		Pos    int      `json:"pos"`
		At     int      `json:"at"`
		Allow  *bool    `json:"allow"`
	}](raw)
	if err != nil {
		return nil, err
	}
	opts := core.ChainOpts{Policy: p.Policy, Cwd: s.cwdOrHome(p.Cwd), At: p.At,
		NoAllow: p.Allow != nil && !*p.Allow}
	var res core.ChainResult
	switch p.Op {
	case "add":
		res, err = core.ChainAdd(s.home, opts, p.Names)
	case "rm":
		res, err = core.ChainRm(s.home, opts, p.Names)
	case "mv":
		if len(p.Names) != 1 {
			return nil, badParams("mv mueve un perfil")
		}
		res, err = core.ChainMv(s.home, opts, p.Names[0], p.Pos)
	case "set":
		res, err = core.ChainSet(s.home, opts, p.Names)
	default:
		return nil, badParams("op desconocida: %q (add, rm, mv, set)", p.Op)
	}
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"policy": res.Policy, "primary": res.Primary, "fallback": res.Fallback,
		"removed": res.Removed, "moved": res.Moved, "moved_to": res.MovedTo,
		"allow_entry": res.AllowEntry, "allow_added": res.AllowAdded, "allow_removed": res.AllowRemoved,
		"allow_created": res.AllowCreated, "gate_absent": res.GateAbsent, "allow_skipped": res.AllowSkipped,
	}, nil
}

func srvAutoAllow(s *server, raw json.RawMessage) (any, error) {
	p, err := params[struct {
		AllowFrom json.RawMessage `json:"allow_from"`
	}](raw)
	if err != nil {
		return nil, err
	}
	if len(p.AllowFrom) == 0 {
		return nil, badParams("falta allow_from (un mapa, o null para quitar el control de permisos)")
	}
	if strings.TrimSpace(string(p.AllowFrom)) == "null" {
		return nil, core.AutoAllowReplace(s.home, nil)
	}
	var m map[string][]string
	if err := json.Unmarshal(p.AllowFrom, &m); err != nil {
		return nil, badParams("allow_from no es un mapa perfil → [perfiles]: %v", err)
	}
	if m == nil {
		m = map[string][]string{}
	}
	return nil, core.AutoAllowReplace(s.home, m)
}

func srvAutoPolicy(s *server, raw json.RawMessage) (any, error) {
	p, err := params[struct {
		Policy           string  `json:"policy"`
		Threshold        *int    `json:"threshold"`
		MinDwell         *string `json:"min_dwell"`
		MaxHops          *int    `json:"max_hops"`
		ReturnCheck      *string `json:"return_check"`
		ReturnIdle       *string `json:"return_idle"`
		CooldownStrategy *string `json:"cooldown_strategy"`
		CooldownFallback *string `json:"cooldown_fallback"`
	}](raw)
	if err != nil {
		return nil, err
	}
	return nil, core.AutoPolicySet(s.home, p.Policy, core.AutoPolicyPatch{
		Threshold: p.Threshold, MinDwell: p.MinDwell, MaxHops: p.MaxHops,
		ReturnCheck: p.ReturnCheck, ReturnIdle: p.ReturnIdle,
		CooldownStrategy: p.CooldownStrategy, CooldownFallback: p.CooldownFallback,
	})
}

func srvAutoTest(s *server, raw json.RawMessage) (any, error) {
	p, err := params[struct {
		Profile string `json:"profile"`
	}](raw)
	if err != nil {
		return nil, err
	}
	args := []string{"auto", "test"}
	if p.Profile != "" {
		args = append(args, "--profile", p.Profile)
	}
	return runInProcess(args...), nil
}

// srvAutoSimulate responde «¿qué pasa si se agota esta cuenta?» con los datos
// de uso que hay: a qué cuenta iría, cuáles saltaría y hasta cuándo, y cuándo
// podría volver a casa. Es una estimación a partir de las últimas muestras; el
// supervisor decide en vivo con más información (los límites que va viendo).
func srvAutoSimulate(s *server, raw json.RawMessage) (any, error) {
	p, err := params[struct {
		Cwd     string `json:"cwd"`
		Primary string `json:"primary"`
		Policy  string `json:"policy"`
	}](raw)
	if err != nil {
		return nil, err
	}
	cfg, err := s.cfg()
	if err != nil {
		return nil, err
	}
	block, err := func() (*core.AutoHandoff, error) {
		if cfg.AutoHandoff == nil {
			return nil, &serveError{Code: "no_auto", Message: "no hay bloque auto_handoff: créalo con `ccp auto init`"}
		}
		return cfg.AutoHandoff, nil
	}()
	if err != nil {
		return nil, err
	}
	name := p.Policy
	if name == "" {
		name = "default"
	}
	eff, err := block.Policies[name].Effective(name)
	if err != nil {
		return nil, err
	}
	primary := p.Primary
	if primary == "" {
		primary = core.Resolve(s.cwdOrHome(p.Cwd), cfg.Rules)
	}
	gate := core.AutoGateFor(cfg, primary)
	entry := map[string]bool{}
	for _, e := range gate.Entry {
		entry[e] = true
	}
	now := time.Now()
	threshold := float64(eff.Threshold)

	type step struct {
		Profile   string  `json:"profile"`
		Role      string  `json:"role"` // primary | loan
		Order     int     `json:"order"`
		Status    string  `json:"status"` // available | cooling | exhausted | blocked | nodata | denied
		Until     string  `json:"until"`
		Pct5      float64 `json:"pct5"`
		Pct7      float64 `json:"pct7"`
		SampledAt string  `json:"sampled_at"`
	}
	status := func(name string) step {
		st := step{Profile: name}
		if profileAccess(s.home, cfg, name) != "ok" {
			st.Status = "blocked"
			return st
		}
		rl, sampled, ok := core.ReadRateLimits(s.home, name)
		if !ok {
			st.Status = "nodata"
			return st
		}
		st.Pct5, st.Pct7, st.SampledAt = rl.FiveHour.UsedPercentage, rl.SevenDay.UsedPercentage, timeOrEmpty(sampled)
		var until time.Time
		over := false
		for _, w := range []core.Windowed{rl.FiveHour, rl.SevenDay} {
			if w.UsedPercentage >= threshold {
				over = true
				if w.ResetsAt.After(until) {
					until = w.ResetsAt
				}
			}
		}
		switch {
		case !over:
			st.Status = "available"
		case until.IsZero():
			st.Status = "exhausted"
		case until.After(now):
			st.Status, st.Until = "cooling", timeOrEmpty(until)
		default:
			st.Status = "available" // la ventana ya se reinició
		}
		return st
	}

	prim := status(primary)
	prim.Role = "primary"
	steps := []step{prim}
	target := ""
	for i, f := range eff.Fallback {
		if f == primary {
			continue
		}
		st := status(f)
		st.Role, st.Order = "loan", i+1
		if !gate.Absent && (!gate.Declared || !entry[f]) {
			st.Status = "denied"
		}
		if target == "" && (st.Status == "available" || st.Status == "nodata") {
			target = f
		}
		steps = append(steps, st)
	}
	// Vuelta a casa: cuando el primario se reinicia; si no hay fecha, tras el
	// enfriamiento fijo de la política.
	returnAt := prim.Until
	if returnAt == "" {
		returnAt = timeOrEmpty(now.Add(eff.CooldownFallback))
	}
	return map[string]any{
		"policy": name, "primary": primary, "threshold": eff.Threshold, "max_hops": eff.MaxHops,
		"steps": steps, "target": target, "return_at": returnAt,
		"return_check": eff.ReturnCheck.String(), "return_idle": eff.ReturnIdle.String(),
	}, nil
}

type srvBootstrapParams struct {
	Cwd     string `json:"cwd"`
	Policy  string `json:"policy"`
	Profile string `json:"profile"` // la cuenta de la carpeta, si la GUI la sabe
}

func (s *server) bootstrapPlan(p srvBootstrapParams) (core.BootstrapPlan, error) {
	cfg, err := s.cfg()
	if err != nil {
		return core.BootstrapPlan{}, err
	}
	cwd := s.cwdOrHome(p.Cwd)
	return core.BootstrapDetect(cfg, core.BootstrapInput{
		Cwd: cwd, Repo: gitRepoRoot(cwd), Active: p.Profile, Policy: p.Policy,
	})
}

func bootstrapJSON(plan core.BootstrapPlan) map[string]any {
	type item struct {
		Kind     string   `json:"kind"`
		Missing  bool     `json:"missing"`
		Blocked  bool     `json:"blocked"`
		Path     string   `json:"path"`
		Profile  string   `json:"profile"`
		Policy   string   `json:"policy"`
		Profiles []string `json:"profiles"`
	}
	items := []item{}
	for _, it := range plan.Items {
		items = append(items, item{Kind: string(it.Kind), Missing: it.Missing, Blocked: it.Blocked,
			Path: it.Path, Profile: it.Profile, Policy: it.Policy, Profiles: append([]string{}, it.Profiles...)})
	}
	return map[string]any{"repo": plan.Repo, "cwd": plan.Cwd, "primary": plan.Primary, "items": items}
}

func srvAutoBootstrap(s *server, raw json.RawMessage) (any, error) {
	p, err := params[srvBootstrapParams](raw)
	if err != nil {
		return nil, err
	}
	plan, err := s.bootstrapPlan(p)
	if err != nil {
		return nil, err
	}
	return bootstrapJSON(plan), nil
}

func srvAutoBootstrapApply(s *server, raw json.RawMessage) (any, error) {
	p, err := params[srvBootstrapParams](raw)
	if err != nil {
		return nil, err
	}
	plan, err := s.bootstrapPlan(p)
	if err != nil {
		return nil, err
	}
	if _, err := core.BootstrapApply(s.home, plan); err != nil {
		return nil, err
	}
	after, err := s.bootstrapPlan(p)
	if err != nil {
		return nil, err
	}
	return bootstrapJSON(after), nil
}

// --- Desktop ---

func srvDesktopList(s *server, _ json.RawMessage) (any, error) {
	cfg, err := s.cfg()
	if err != nil {
		return nil, err
	}
	bytes := map[string]int64{}
	if insts, err := core.DesktopList(s.home, cfg); err == nil {
		for _, in := range insts {
			bytes[in.Profile] = in.Bytes
		}
	}
	apps := map[string]*core.DesktopApp{}
	if dir, err := core.DesktopAppsDir(); err == nil {
		if found, err := core.ListDesktopApps(dir); err == nil {
			for _, a := range found {
				apps[a.Manifest.Profile] = a
			}
		}
	}
	mainApp, _ := core.ResolveDesktopApp(desktopHost(""))
	mainID := ""
	if mainApp != "" {
		mainID = core.DesktopBundleIDOf(mainApp)
	}
	procs, procsOK := desktopProcsProbe()
	mains := core.DesktopMainProcs(procs)
	lsapps, lsOK := desktopLSApps()

	type launcher struct {
		Path     string `json:"path"`
		Label    string `json:"label"`
		Color    string `json:"color"`
		BundleID string `json:"bundle_id"`
		Stale    string `json:"stale"` // motivo si el espejo quedó atrás; "" = al día
	}
	type row struct {
		Profile  string    `json:"profile"`
		DataDir  string    `json:"data_dir"`
		Instance bool      `json:"instance"`
		Bytes    int64     `json:"bytes"`
		Running  bool      `json:"running"`
		Identity string    `json:"identity"` // ok | collapsed | hijacked | unknown | none
		Issues   []string  `json:"issues"`
		Launcher *launcher `json:"launcher"`
	}
	out := []row{}
	for _, name := range allProfileNames(cfg) {
		if core.DesktopEligible(cfg, name) != nil {
			continue
		}
		r := row{Profile: name, Identity: "none", Issues: []string{}, Bytes: bytes[name]}
		userData, _ := core.DesktopUserDataDir(s.home, name)
		r.DataDir = userData
		if info, err := os.Stat(userData); err == nil && info.IsDir() {
			r.Instance = true
		}
		dataDir := core.DesktopDataDir(s.home, name)
		for _, pr := range mains {
			if pr.DataDir == dataDir {
				r.Running = true
				break
			}
		}
		a := apps[name]
		if a != nil {
			r.Launcher = &launcher{Path: a.Path, Label: a.Manifest.Label, Color: a.Manifest.Color,
				BundleID: a.Manifest.BundleID}
			if mainApp != "" {
				if reason, stale := core.DesktopAppStale(a, mainApp); stale {
					r.Launcher.Stale = reason
				}
			}
		}
		switch {
		case !procsOK || !lsOK:
			r.Identity = "unknown"
		case name == "default":
			if r.Running {
				r.Identity = "ok"
			}
			for _, la := range lsapps {
				if la.BundleID == mainID && filepath.Clean(la.BundlePath) != filepath.Clean(mainApp) {
					r.Identity = "hijacked"
				}
			}
		case r.Running && a != nil:
			r.Identity = "unknown"
			mirror := filepath.Join(a.Path, "Contents", "ccp", "Claude")
			for _, la := range lsapps {
				bp := filepath.Clean(la.BundlePath)
				switch {
				case bp == filepath.Clean(a.Path) && la.BundleID == a.Manifest.BundleID:
					r.Identity = "ok"
				case bp == mirror || (bp == filepath.Clean(a.Path) && la.BundleID != a.Manifest.BundleID):
					r.Identity = "collapsed"
				}
			}
		}
		if name != "default" && procsOK {
			ccHome, _ := core.CCHome(s.home, name)
			launcherPath := ""
			if a != nil {
				launcherPath = a.Path
			}
			for _, is := range core.DesktopPreflight(core.DesktopPreflightInput{Profile: name, DataDir: dataDir,
				CCHome: ccHome, LauncherPath: launcherPath, Procs: procs}) {
				if is.Code != core.DesktopIssueAlreadyRunning {
					r.Issues = append(r.Issues, is.Code)
				}
			}
		}
		out = append(out, r)
	}
	return out, nil
}

func (s *server) desktopFindings(cfg *core.Config, profile string) []core.DesktopFinding {
	appsDir, _ := core.DesktopAppsDir()
	fs := core.DesktopAudit(core.DesktopAuditOptions{
		Home: s.home, Cfg: cfg, AppsDir: appsDir, Profile: profile,
		Probes: core.DesktopProbes{Procs: desktopProcsProbe, RunningIdentity: desktopRunningIdentity},
	})
	if fs == nil {
		fs = []core.DesktopFinding{}
	}
	return fs
}

func srvDesktopDoctor(s *server, raw json.RawMessage) (any, error) {
	p, err := params[struct {
		Profile string `json:"profile"`
	}](raw)
	if err != nil {
		return nil, err
	}
	cfg, err := s.cfg()
	if err != nil {
		return nil, err
	}
	return s.desktopFindings(cfg, p.Profile), nil
}

// srvDesktopRun ejecuta las operaciones de ventanas que ya existen como
// comando, con la misma lógica y los mismos avisos que en la terminal.
func srvDesktopRun(s *server, raw json.RawMessage) (any, error) {
	p, err := params[struct {
		Action  string `json:"action"`
		Profile string `json:"profile"`
		Color   string `json:"color"`
		Label   string `json:"label"`
		Plain   bool   `json:"plain"`
		Force   bool   `json:"force"`
	}](raw)
	if err != nil {
		return nil, err
	}
	cfg, err := s.cfg()
	if err != nil {
		return nil, err
	}
	if err := core.DesktopEligible(cfg, p.Profile); err != nil {
		return nil, err
	}
	var args []string
	switch p.Action {
	case "open":
		args = []string{"desktop", "open", p.Profile}
		if p.Plain {
			args = append(args, "--plain")
		}
	case "app":
		args = []string{"desktop", "app", p.Profile}
		if p.Color != "" {
			args = append(args, "--color", p.Color)
		}
		if p.Label != "" {
			args = append(args, "--label", p.Label)
		}
	case "app_rm":
		args = []string{"desktop", "app", "rm", p.Profile}
	case "prepare":
		args = []string{"desktop", "prepare", p.Profile}
	case "rm":
		// La CLI no lo comprueba; la GUI sí lo promete: borrar el data dir de una
		// ventana abierta le arranca a Chromium los archivos que tiene mapeados.
		if desktopProfileRunning(s.home, p.Profile) {
			return nil, &serveError{Code: "window_open", Message: "la ventana de " + p.Profile + " está abierta: ciérrala antes de borrar su instancia"}
		}
		args = []string{"desktop", "rm", p.Profile, "--yes"}
	default:
		return nil, badParams("acción desconocida: %q", p.Action)
	}
	if p.Force && p.Action != "prepare" && p.Action != "rm" {
		args = append(args, "--force")
	}
	return runInProcess(args...), nil
}

// --- diagnóstico ---

type srvFinding struct {
	Code     string `json:"code"`
	Severity string `json:"severity"` // error | warn | unknown | info
	Profile  string `json:"profile,omitempty"`
	Subject  string `json:"subject,omitempty"`
	Detail   string `json:"detail,omitempty"`
}

// sensorBinOf devuelve el binario al que apunta la statusLine gestionada de un
// perfil, o "" si no la tiene.
func sensorBinOf(ccHome string) string {
	data, err := os.ReadFile(filepath.Join(ccHome, "settings.json"))
	if err != nil {
		return ""
	}
	var st struct {
		StatusLine struct {
			Command string `json:"command"`
		} `json:"statusLine"`
	}
	if json.Unmarshal(data, &st) != nil {
		return ""
	}
	cmd := st.StatusLine.Command
	i := strings.Index(cmd, " _statusline")
	if i <= 0 {
		return ""
	}
	return strings.Trim(strings.TrimSpace(cmd[:i]), `'"`)
}

// srvDiagRun junta en una lista lo que hoy dan `ccp doctor`, `ccp desktop
// doctor` y varias comprobaciones que nadie hacía explícitas. Diagnostica y no
// repara: la acción la decide el usuario desde la pantalla que toca.
func srvDiagRun(s *server, _ json.RawMessage) (any, error) {
	cfg, err := s.cfg()
	if err != nil {
		return nil, err
	}
	var out []srvFinding
	add := func(f srvFinding) { out = append(out, f) }

	for _, bin := range []string{"claude", "node", "git"} {
		if _, err := exec.LookPath(bin); err != nil {
			sev := "warn"
			if bin == "claude" {
				sev = "error"
			}
			add(srvFinding{Code: "tool_missing", Severity: sev, Subject: bin})
		}
	}
	rc := rcPath("")
	switch installed := fileContains(rc, "ccp shell init"); {
	case !installed:
		add(srvFinding{Code: "shell_missing", Severity: "warn", Subject: rc})
	case installedBlock(rc) != core.ShellInit:
		add(srvFinding{Code: "shell_stale", Severity: "error", Subject: rc})
	}
	for _, name := range allProfileNames(cfg) {
		switch profileAccess(s.home, cfg, name) {
		case "nokey":
			add(srvFinding{Code: "profile_no_key", Severity: "error", Profile: name})
		case "nologin":
			add(srvFinding{Code: "profile_no_login", Severity: "warn", Profile: name})
		}
		if name != "default" && core.AutoHooksEnabled(cfg, name) {
			ccHome, _ := core.CCHome(s.home, name)
			if bin := sensorBinOf(ccHome); bin != "" {
				if _, err := os.Stat(bin); err != nil {
					add(srvFinding{Code: "sensors_bin_missing", Severity: "error", Profile: name, Detail: bin})
				}
			}
		}
	}
	for _, r := range cfg.Rules {
		if !profileExists(cfg, r.Profile) {
			add(srvFinding{Code: "rule_orphan", Severity: "warn", Subject: r.Path, Detail: r.Profile})
		} else if info, err := os.Stat(r.Path); err != nil || !info.IsDir() {
			add(srvFinding{Code: "rule_missing_dir", Severity: "info", Subject: r.Path, Profile: r.Profile})
		}
	}
	if h, err := core.LoadHandoffs(s.home); err == nil {
		for _, m := range h.Active {
			toCC, err := core.CCHome(s.home, m.To)
			if err != nil {
				continue
			}
			if _, err := os.Stat(filepath.Join(core.ProjectDir(toCC, m.Slug), m.Session+".jsonl")); err != nil {
				add(srvFinding{Code: "handoff_zombie", Severity: "warn", Subject: m.Session, Detail: m.From + " → " + m.To})
			}
		}
	}
	if block := cfg.AutoHandoff; block != nil {
		if !block.Enabled {
			add(srvFinding{Code: "auto_disabled", Severity: "info"})
		}
		for _, name := range core.AutoPolicyNames(cfg) {
			eff, err := block.Policies[name].Effective(name)
			if err != nil {
				add(srvFinding{Code: "policy_invalid", Severity: "error", Subject: name, Detail: err.Error()})
				continue
			}
			for _, f := range eff.Fallback {
				if !profileExists(cfg, f) {
					add(srvFinding{Code: "chain_unknown_profile", Severity: "error", Subject: name, Profile: f})
					continue
				}
				if profileAccess(s.home, cfg, f) != "ok" {
					add(srvFinding{Code: "chain_no_access", Severity: "error", Subject: name, Profile: f})
				}
				if f != "default" && !core.AutoHooksEnabled(cfg, f) {
					add(srvFinding{Code: "chain_no_sensors", Severity: "warn", Subject: name, Profile: f})
				}
				if p, ok := cfg.Profiles[f]; ok && core.IsProviderType(p.Type) && eff.CooldownStrategy == core.CooldownResetsAt {
					add(srvFinding{Code: "chain_provider_resets_at", Severity: "info", Subject: name, Profile: f})
				}
			}
		}
	}
	for _, f := range s.desktopFindings(cfg, "") {
		if f.Severity == core.DesktopSevOK {
			continue
		}
		add(srvFinding{Code: f.Code, Severity: f.Severity, Profile: f.Profile, Detail: f.Detail})
	}
	rank := map[string]int{"error": 0, "warn": 1, "unknown": 2, "info": 3}
	sort.SliceStable(out, func(a, b int) bool { return rank[out[a].Severity] < rank[out[b].Severity] })
	if out == nil {
		out = []srvFinding{}
	}
	return out, nil
}

// --- copias de seguridad ---

func srvBackupExport(s *server, raw json.RawMessage) (any, error) {
	p, err := params[struct {
		Dest        string `json:"dest"`
		WithSecrets bool   `json:"with_secrets"`
	}](raw)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(p.Dest) == "" {
		return nil, badParams("falta el archivo de destino")
	}
	if err := core.BackupExport(s.home, p.Dest, p.WithSecrets, time.Now()); err != nil {
		return nil, err
	}
	return map[string]any{"dest": p.Dest, "with_secrets": p.WithSecrets}, nil
}

func srvBackupRestore(s *server, raw json.RawMessage) (any, error) {
	p, err := params[struct {
		Archive string `json:"archive"`
		Mode    string `json:"mode"` // merge | overwrite | force
	}](raw)
	if err != nil {
		return nil, err
	}
	opts := core.RestoreOpts{}
	switch p.Mode {
	case "", "merge":
	case "overwrite":
		opts.Overwrite = true
	case "force":
		opts.Force = true
	default:
		return nil, badParams("modo desconocido: %q (merge, overwrite, force)", p.Mode)
	}
	if _, err := autoSnapshot(s.home, "pre-backup-restore"); err != nil {
		return nil, fmt.Errorf("no se pudo guardar el snapshot de seguridad y no se restauró nada: %w", err)
	}
	rep, err := core.BackupRestore(s.home, p.Archive, opts)
	if err != nil {
		return nil, err
	}
	nz := func(v []string) []string {
		if v == nil {
			return []string{}
		}
		return v
	}
	return map[string]any{"created": nz(rep.Created), "skipped": nz(rep.Skipped),
		"overwritten": nz(rep.Overwritten), "rules_added": rep.RulesAdded, "snapshot": rep.SnapshotDir}, nil
}

// --- sistema ---

// srvSystemRun ejecuta los comandos de ciclo de vida. Lista cerrada: la GUI no
// puede pedir un comando cualquiera.
func srvSystemRun(s *server, raw json.RawMessage) (any, error) {
	p, err := params[struct {
		Action     string `json:"action"`
		FromSource bool   `json:"from_source"`
		Pull       bool   `json:"pull"`
	}](raw)
	if err != nil {
		return nil, err
	}
	switch p.Action {
	case "install", "uninstall", "doctor":
		return runInProcess(p.Action), nil
	case "upgrade":
		args := []string{"upgrade"}
		if p.FromSource {
			args = append(args, "--from-source")
		}
		if p.Pull {
			args = append(args, "--pull")
		}
		return runInProcess(args...), nil
	}
	return nil, badParams("acción desconocida: %q", p.Action)
}
