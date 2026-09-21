package cli

// cloud_agent.go — `ccp cloud agent`, `ccp cloud review` y `ccp cloud policy`
// (spec §10.3). El portal propone y esta máquina aplica: aquí solo se orquesta
// y se pinta; decidir qué se escribe es de internal/cloud/agent.

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/mattn/go-isatty"

	"github.com/JoseAFlores777/ccp/internal/cloud/agent"
	"github.com/JoseAFlores777/ccp/internal/cloud/client"
	"github.com/JoseAFlores777/ccp/internal/core"
	"github.com/JoseAFlores777/ccp/internal/core/i18n"
	"github.com/JoseAFlores777/ccp/internal/snapshot"
)

// cloudAgentOpts monta el agente con la sesión, la bóveda y el almacén de este
// equipo. Sin bóveda no hay nada que hacer: una revisión llega sellada.
//
// Vive fuera de cloudCmd porque `ccp serve` monta el mismo agente para la
// pantalla Nube (P-21): dos construcciones del agente con distinta política o
// distinto almacén serían dos máquinas distintas aplicando la misma revisión.
func cloudAgentOpts(ctx context.Context, home string, files client.Files) (agent.Opts, error) {
	cfg, cl, err := client.Session(ctx, files)
	if err != nil {
		return agent.Opts{}, err
	}
	acct, err := client.Account(files)
	if err != nil {
		return agent.Opts{}, err
	}
	st, err := core.OpenSnapshotStore(home)
	if err != nil {
		return agent.Opts{}, err
	}
	src, err := core.ClaudeSrc()
	if err != nil {
		return agent.Opts{}, err
	}
	return agent.Opts{Home: home, Src: src, API: cl, Acct: acct, Store: st, Files: files,
		DeviceID: cfg.DeviceID, Policy: cfg.Policy, Machine: snapMachine(), Now: time.Now}, nil
}

func (c cloudCmd) agentOpts() (agent.Opts, error) {
	return cloudAgentOpts(c.ctx, c.home, c.files)
}

// printOutcome cuenta una pasada. El orden es el de la pregunta que se hace
// quien lo lee: qué se escribió, qué espera a una persona, qué chocó.
func (c cloudCmd) printOutcome(out *agent.Outcome) {
	if len(out.Applied) > 0 {
		fmt.Fprintln(c.out, okLine(c.out, i18n.T(c.lang, "cli.cloud.agent_applied", len(out.Applied), shortID(out.Revision))))
	}
	if out.PreSnapshot != "" {
		fmt.Fprintln(c.out, mute(c.out, i18n.T(c.lang, "cli.cloud.agent_pre", snapshot.Short(out.PreSnapshot))))
	}
	for _, s := range out.Skipped {
		fmt.Fprintln(c.out, warnLine(c.out, i18n.T(c.lang, "cli.cloud.agent_skipped", s.LPath, s.Reason)))
	}
	if len(out.Conflicts) > 0 {
		fmt.Fprintln(c.out, warnLine(c.out, i18n.T(c.lang, "cli.cloud.agent_conflicts", len(out.Conflicts), strings.Join(out.Conflicts, ", "))))
	}
	if out.Waiting {
		fmt.Fprintln(c.out, warnLine(c.out, i18n.T(c.lang, "cli.cloud.agent_waiting", len(out.Pending))))
	}
	if out.State != "" {
		fmt.Fprintln(c.out, mute(c.out, i18n.T(c.lang, "cli.cloud.agent_state", i18n.T(c.lang, "cli.cloud.rev_"+out.State))))
	}
}

func (c cloudCmd) agent(args []string) int {
	a, ok := c.args(args, []string{"--once", "--json"}, []string{"--interval"}, 0)
	if !ok {
		return 1
	}
	every := agent.DefaultInterval
	if v := a.val("--interval"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil || d <= 0 {
			return c.usage("cli.cloud.agent_bad_interval", v)
		}
		every = d
	}
	o, err := c.agentOpts()
	if err != nil {
		return c.fail(err)
	}
	if a.flags["--once"] {
		out, err := agent.Once(c.ctx, o)
		if errors.Is(err, agent.ErrNothing) {
			if a.flags["--json"] {
				return snapJSON(c.out, c.err, map[string]any{"revision": "", "state": ""})
			}
			fmt.Fprintln(c.out, i18n.T(c.lang, "cli.cloud.agent_nothing"))
			return 0
		}
		if err != nil {
			return c.fail(err)
		}
		if a.flags["--json"] {
			return snapJSON(c.out, c.err, out)
		}
		c.printOutcome(out)
		return 0
	}
	fmt.Fprintln(c.err, i18n.T(c.lang, "cli.cloud.agent_watching", every))
	agent.Loop(c.ctx, o, every, func(out *agent.Outcome, err error) {
		switch {
		case err != nil:
			fmt.Fprintf(c.err, "Error: %v\n", err)
		case a.flags["--json"]:
			snapJSON(c.out, c.err, out)
		default:
			c.printOutcome(out)
		}
	})
	return 0
}

// review es la confirmación local (D6): lo que ejecuta código en esta máquina
// no entra porque lo diga la nube, entra porque lo dice alguien que está aquí.
func (c cloudCmd) review(args []string) int {
	a, ok := c.args(args, []string{"--yes", "--reject", "--json"}, nil, 0)
	if !ok {
		return 1
	}
	if a.flags["--yes"] && a.flags["--reject"] {
		return c.usage("cli.cloud.review_both")
	}
	r, has, err := agent.LoadReview(c.files)
	if err != nil {
		return c.fail(err)
	}
	if !has || (len(r.Pending) == 0 && len(r.Conflicts) == 0) {
		if a.flags["--json"] {
			return snapJSON(c.out, c.err, agent.Review{Pending: []agent.Pending{}, Conflicts: []agent.Decision{}})
		}
		fmt.Fprintln(c.out, i18n.T(c.lang, "cli.cloud.review_nothing"))
		return 0
	}
	if a.flags["--json"] {
		return snapJSON(c.out, c.err, r)
	}
	c.printReview(r)
	if len(r.Pending) == 0 {
		// Solo choques: se resuelven desde el portal o la GUI, no aquí.
		return 0
	}
	approve, ok := c.askReview(r, a.flags["--yes"], a.flags["--reject"])
	if !ok {
		return 1
	}
	o, err := c.agentOpts()
	if err != nil {
		return c.fail(err)
	}
	out, err := agent.Resolve(c.ctx, o, approve)
	if errors.Is(err, agent.ErrSuperseded) {
		fmt.Fprintln(c.err, i18n.T(c.lang, "cli.cloud.review_superseded"))
		return 1
	}
	if err != nil {
		return c.fail(err)
	}
	c.printOutcome(out)
	return 0
}

func (c cloudCmd) printReview(r agent.Review) {
	fmt.Fprintln(c.out, i18n.T(c.lang, "cli.cloud.review_header", shortID(r.Revision), len(r.Pending)))
	for _, p := range r.Pending {
		fmt.Fprintln(c.out, "  "+i18n.T(c.lang, "cli.cloud.review_item", p.LPath, c.why(p.Why)))
	}
	if len(r.Conflicts) > 0 {
		fmt.Fprintln(c.out, warnLine(c.out, i18n.T(c.lang, "cli.cloud.review_conflicts", len(r.Conflicts))))
		for _, d := range r.Conflicts {
			fmt.Fprintln(c.out, "  "+d.LPath)
		}
	}
}

// why traduce los motivos. Un motivo que este binario no conozca se enseña tal
// cual: callarlo dejaría al usuario confirmando algo sin saber qué es.
func (c cloudCmd) why(ds []agent.Danger) string {
	if len(ds) == 0 {
		return i18n.T(c.lang, "cli.cloud.why_policy")
	}
	out := make([]string, 0, len(ds))
	for _, d := range ds {
		key := "cli.cloud.why_" + string(d)
		if t := i18n.T(c.lang, key); t != key {
			out = append(out, t)
			continue
		}
		out = append(out, string(d))
	}
	return strings.Join(out, ", ")
}

// askReview decide qué se acepta. Sin terminal y sin --yes/--reject no se
// inventa una respuesta: se enseña la lista y se sale con 1, como el restore.
func (c cloudCmd) askReview(r agent.Review, yes, reject bool) ([]string, bool) {
	all := make([]string, 0, len(r.Pending))
	for _, p := range r.Pending {
		all = append(all, p.LPath)
	}
	switch {
	case yes:
		return all, true
	case reject:
		return nil, true
	}
	if !isatty.IsTerminal(os.Stdin.Fd()) {
		fmt.Fprintln(c.err, i18n.T(c.lang, "cli.cloud.review_need_answer"))
		return nil, false
	}
	in := bufio.NewReader(os.Stdin)
	var approve []string
	for _, p := range r.Pending {
		fmt.Fprint(c.out, i18n.T(c.lang, "cli.cloud.review_prompt", p.LPath, c.why(p.Why)))
		line, err := in.ReadString('\n')
		if err != nil {
			fmt.Fprintln(c.err)
			return nil, false
		}
		if s := strings.ToLower(strings.TrimSpace(line)); s == "s" || s == "y" || s == "si" || s == "sí" || s == "yes" {
			approve = append(approve, p.LPath)
		}
	}
	return approve, true
}

// policy enseña o cambia la política de ESTE dispositivo (§10.3). Vive en el
// config local y no en la nube a propósito: es la defensa de esta máquina
// frente a la cuenta, y una defensa que se pueda cambiar desde donde viene el
// ataque no defiende de nada.
func (c cloudCmd) policy(args []string) int {
	a, ok := c.args(args, nil, nil, 1)
	if !ok {
		return 1
	}
	cfg, err := c.files.LoadConfig()
	if err != nil {
		return c.fail(err)
	}
	if len(a.pos) == 0 {
		fmt.Fprintln(c.out, i18n.T(c.lang, "cli.cloud.policy_now",
			i18n.T(c.lang, "cli.cloud.policy_"+policyOf(cfg))))
		return 0
	}
	switch a.pos[0] {
	case agent.PolicyAuto, agent.PolicyManual:
	default:
		return c.usage("cli.cloud.policy_bad", a.pos[0])
	}
	cfg.Policy = a.pos[0]
	if err := c.files.SaveConfig(cfg); err != nil {
		return c.fail(err)
	}
	fmt.Fprintln(c.out, okLine(c.out, i18n.T(c.lang, "cli.cloud.policy_set",
		i18n.T(c.lang, "cli.cloud.policy_"+cfg.Policy))))
	return 0
}

// policyOf: vacío es `auto`, que es lo que hace un dispositivo recién dado de
// alta. Se nombra aquí para que status y policy digan lo mismo.
func policyOf(cfg client.Config) string {
	if cfg.Policy == agent.PolicyManual {
		return agent.PolicyManual
	}
	return agent.PolicyAuto
}
