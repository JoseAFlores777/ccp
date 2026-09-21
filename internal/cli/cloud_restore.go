package cli

// cloud_restore.go — `ccp cloud restore` (spec §10.3.1). Es el camino 3 —una
// máquina nueva: login, desbloquear, elegir snapshot, mapear y aplicar— y
// también el que usa la GUI por debajo para el camino 1. El camino 2, el del
// portal, no pasa por aquí: llega como revisión firmada y lo aplica el agente.
//
// Termina donde terminan los tres: el motor de restauración de §8.3.

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/JoseAFlores777/ccp/internal/cloud/agent"
	"github.com/JoseAFlores777/ccp/internal/core"
	"github.com/JoseAFlores777/ccp/internal/core/i18n"
	"github.com/JoseAFlores777/ccp/internal/snapshot"
)

// parseProjectMap traduce los `--map <clave>=<ruta>`. Una ruta relativa se
// rechaza en vez de resolverse contra el directorio actual: el mapeo decide
// dónde se escriben archivos, y «./app» significaría una carpeta distinta
// según desde dónde se lanzara el comando.
func parseProjectMap(vals []string) (map[string]string, string) {
	out := map[string]string{}
	for _, v := range vals {
		key, path, ok := strings.Cut(v, "=")
		if !ok || key == "" || path == "" {
			return nil, v
		}
		if !filepath.IsAbs(path) {
			return nil, v
		}
		out[key] = path
	}
	return out, ""
}

func (c cloudCmd) restore(args []string) int {
	a, ok := c.args(args, []string{"--dry-run", "--yes", "--json"}, []string{"--device", "--only", "--map"}, 1)
	if !ok {
		return 1
	}
	projects, bad := parseProjectMap(a.vals["--map"])
	if bad != "" {
		fmt.Fprintln(c.err, i18n.T(c.lang, "cli.cloud.restore_bad_map", bad))
		return 1
	}
	cl, acct, st, err := c.ready()
	if err != nil {
		return c.fail(err)
	}
	ref := ""
	if len(a.pos) == 1 {
		ref = a.pos[0]
	}
	target, err := c.pickSnapshot(cl, a.val("--device"), ref)
	if err != nil {
		return c.fail(err)
	}
	src, err := core.ClaudeSrc()
	if err != nil {
		return c.fail(err)
	}
	o := agent.Opts{Home: c.home, Src: src, API: cl, Acct: acct, Store: st, Files: c.files,
		Machine: snapMachine()}
	ro := agent.RestoreOpts{Only: a.vals["--only"], Projects: projects, DryRun: true}

	// El plan se calcula SIEMPRE primero, aunque venga --yes: es lo que decide
	// si hay algo que escribir, y calcularlo no toca nada.
	plan, err := agent.Restore(c.ctx, o, target, ro)
	if err != nil {
		return c.fail(err)
	}
	if !a.flags["--yes"] || a.flags["--dry-run"] {
		if a.flags["--json"] {
			snapJSON(c.out, c.err, plan)
		} else {
			c.printCloudPlan(plan)
		}
		if a.flags["--dry-run"] || planWrites(plan.Plan) == 0 {
			return 0
		}
		if !a.flags["--json"] {
			fmt.Fprintln(c.err, warnLine(c.err, i18n.T(c.lang, "cli.cloud.restore_confirm")))
		}
		return 1
	}
	if planWrites(plan.Plan) == 0 {
		if a.flags["--json"] {
			return snapJSON(c.out, c.err, plan)
		}
		fmt.Fprintln(c.out, i18n.T(c.lang, "cli.snapshot.restore_nothing"))
		return 0
	}
	ro.DryRun = false
	rep, err := agent.Restore(c.ctx, o, target, ro)
	if err != nil {
		return c.fail(err)
	}
	if a.flags["--json"] {
		return snapJSON(c.out, c.err, rep)
	}
	c.printCloudPlan(rep)
	fmt.Fprintln(c.out, okLine(c.out, i18n.T(c.lang, "cli.snapshot.restored", snapshot.Short(rep.Plan.PreSnapshot))))
	if len(rep.Plan.Regenerated) > 0 {
		fmt.Fprintln(c.out, mute(c.out, i18n.T(c.lang, "cli.snapshot.regenerated", strings.Join(rep.Plan.Regenerated, ", "))))
	}
	return 0
}

// printCloudPlan enseña el plan, cómo cae cada proyecto en esta máquina y lo
// que queda a mano. Las tres cosas van juntas a propósito: el plan dice qué se
// escribe, el mapeo dónde, y los pendientes qué NO va a funcionar todavía
// aunque el restore salga bien.
func (c cloudCmd) printCloudPlan(r *agent.RestoreReport) {
	printRestorePlan(c.out, c.lang, r.Plan)
	for _, p := range r.Projects {
		switch p.Source {
		case core.ProjectMapMissing:
			fmt.Fprintln(c.out, warnLine(c.out, i18n.T(c.lang, "cli.cloud.restore_project_missing", p.Key, len(p.Files))))
		case core.ProjectMapSnapshot:
			// Cae donde ya estaba: no hay nada que explicar.
		default:
			fmt.Fprintln(c.out, mute(c.out, i18n.T(c.lang, "cli.cloud.restore_project_at", p.Key, p.Path)))
		}
	}
	if len(r.NoData) > 0 {
		fmt.Fprintln(c.out, warnLine(c.out, i18n.T(c.lang, "cli.cloud.pull_missing", len(r.NoData))))
	}
	if len(r.Pending) == 0 {
		return
	}
	fmt.Fprintln(c.out, boldLine(c.out, i18n.T(c.lang, "cli.cloud.restore_pending")))
	for _, p := range r.Pending {
		switch p.Kind {
		case core.RestorePendingLogin:
			fmt.Fprintf(c.out, "  %s\n", i18n.T(c.lang, "cli.cloud.pending_login", p.Name))
		case core.RestorePendingCommand:
			fmt.Fprintf(c.out, "  %s\n", i18n.T(c.lang, "cli.cloud.pending_command", p.Name, p.Where))
		case core.RestorePendingProject:
			fmt.Fprintf(c.out, "  %s\n", i18n.T(c.lang, "cli.cloud.pending_project", p.Name, p.Where))
		}
	}
}
