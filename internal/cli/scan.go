package cli

// scan.go — `ccp scan` y `ccp adopt` (spec 2026-09-18 §5): qué hay de Claude en
// esta máquina y cómo traerlo al modelo de ccp. El motor está en
// core/inventory*.go y core/adopt.go; aquí se construyen las raíces reales y se
// pinta.

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/JoseAFlores777/ccp/internal/core"
	"github.com/JoseAFlores777/ccp/internal/core/i18n"
)

// inventoryRoots son las raíces de esta máquina. Todo sale del entorno aquí y
// en ningún otro sitio: el motor las recibe inyectadas.
func inventoryRoots(home string) (core.InventoryRoots, error) {
	uh, err := os.UserHomeDir()
	if err != nil {
		return core.InventoryRoots{}, err
	}
	src, err := claudeSrc()
	if err != nil {
		return core.InventoryRoots{}, err
	}
	def, _ := core.DesktopUserDataDir(home, "default")
	managed := os.Getenv("CCP_MANAGED_DIR")
	if managed == "" {
		managed = "/Library/Application Support/ClaudeCode"
	}
	var rcs []string
	for _, f := range []string{".zshrc", ".zprofile", ".bashrc", ".bash_profile"} {
		rcs = append(rcs, filepath.Join(uh, f))
	}
	return core.InventoryRoots{Home: uh, CCPHome: home, ClaudeSrc: src, DesktopDefaultDataDir: def,
		ManagedDir: managed, RCFiles: rcs, LookPath: exec.LookPath}, nil
}

// tilde acorta el HOME en las rutas que se enseñan.
func tilde(p string) string {
	if uh, err := os.UserHomeDir(); err == nil && uh != "" && strings.HasPrefix(p, uh) {
		return "~" + strings.TrimPrefix(p, uh)
	}
	return p
}

func dispatchScan(args []string, stdout, stderr io.Writer) int {
	lang := currentLang()
	a, bad, ok := parseSnapArgs(args, []string{"--json"}, nil)
	if !ok || len(a.pos) > 0 {
		if bad == "" && len(a.pos) > 0 {
			bad = a.pos[0]
		}
		fmt.Fprintln(stderr, i18n.T(lang, "cli.scan.unknown_opt", bad))
		fmt.Fprintln(stderr, i18n.T(lang, "cli.scan.usage"))
		return 1
	}
	home := resolveHome()
	if err := ensureMigrated(home); err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}
	r, err := inventoryRoots(home)
	if err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}
	inv := core.BuildInventory(r)
	if a.flags["--json"] {
		return snapJSON(stdout, stderr, inv)
	}
	printInventory(stdout, lang, inv)
	return 0
}

// printInventory agrupa capa → tipo → elemento, con su origen y dónde aplica.
func printInventory(w io.Writer, lang i18n.Lang, inv core.Inventory) {
	type group struct {
		scope string
		items []core.InvItem
	}
	idx := map[string]int{}
	var groups []group
	for _, it := range inv.Items {
		k := it.Scope.Level
		if it.Scope.Name != "" {
			k += ":" + it.Scope.Name
		}
		i, ok := idx[k]
		if !ok {
			i = len(groups)
			idx[k] = i
			groups = append(groups, group{scope: k})
		}
		groups[i].items = append(groups[i].items, it)
	}
	for _, g := range groups {
		fmt.Fprintln(w, boldLine(w, g.scope))
		sort.SliceStable(g.items, func(i, j int) bool { return g.items[i].Kind < g.items[j].Kind })
		for _, it := range g.items {
			applies := strings.Join(it.AppliesTo, " · ")
			if applies == "" {
				applies = "—"
			}
			line := fmt.Sprintf("  %-12s %-28s %s  %s", it.Kind, it.Name, mute(w, applies), mute(w, tilde(it.Source)))
			fmt.Fprintln(w, line)
			if it.Why != "" {
				fmt.Fprintln(w, "      "+mute(w, it.Why))
			}
			if it.Missing != "" {
				fmt.Fprintln(w, "      "+warnLine(w, i18n.T(lang, "cli.scan.missing_cmd", it.Missing)))
			}
		}
	}
	unknown := 0
	for _, p := range inv.Probes {
		if p.Status == "unknown" {
			unknown++
			fmt.Fprintln(w, warnLine(w, i18n.T(lang, "cli.scan.unknown", tilde(p.Source), p.Err)))
		}
	}
	fmt.Fprintln(w, mute(w, i18n.T(lang, "cli.scan.summary", len(inv.Items), unknown)))
}

// dispatchAdopt: como `snapshot restore`, sin --yes solo enseña el plan (sale 1
// si hay algo que aplicar); --dry-run sale 0; --yes aplica tras un snapshot de
// seguridad. --only acepta IDs de paso o tipos (lift-mcp-global…).
func dispatchAdopt(args []string, stdout, stderr io.Writer) int {
	lang := currentLang()
	a, bad, ok := parseSnapArgs(args, []string{"--json", "--dry-run", "--yes"}, []string{"--only"})
	if !ok || len(a.pos) > 0 {
		if bad == "" && len(a.pos) > 0 {
			bad = a.pos[0]
		}
		fmt.Fprintln(stderr, i18n.T(lang, "cli.adopt.unknown_opt", bad))
		fmt.Fprintln(stderr, i18n.T(lang, "cli.adopt.usage"))
		return 1
	}
	home := resolveHome()
	if err := ensureMigrated(home); err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}
	r, err := inventoryRoots(home)
	if err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}
	cfg, err := core.Load(home)
	if err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}
	apps, _ := core.DesktopAppsDir()
	steps := core.AdoptPlan(core.BuildInventory(r), core.AdoptInputsFor(home, r.ClaudeSrc, cfg, apps))

	var only []string
	if sel := a.vals["--only"]; len(sel) > 0 {
		only = []string{}
		for _, s := range sel {
			n := 0
			for _, st := range steps {
				if st.ID == s || st.Kind == s {
					only = append(only, st.ID)
					n++
				}
			}
			if n == 0 {
				fmt.Fprintln(stderr, i18n.T(lang, "cli.adopt.no_such_step", s))
				return 1
			}
		}
	}
	applicable := 0
	for _, st := range steps {
		if !st.Pending && (only == nil && st.Default || slices.Contains(only, st.ID)) {
			applicable++
		}
	}

	if !a.flags["--yes"] || a.flags["--dry-run"] {
		if a.flags["--json"] {
			snapJSON(stdout, stderr, map[string]any{"steps": steps})
		} else {
			printAdoptPlan(stdout, lang, steps, only)
		}
		if a.flags["--dry-run"] || applicable == 0 {
			return 0
		}
		if !a.flags["--json"] {
			fmt.Fprintln(stderr, warnLine(stderr, i18n.T(lang, "cli.adopt.confirm")))
		}
		return 1
	}
	if applicable == 0 {
		fmt.Fprintln(stdout, i18n.T(lang, "cli.adopt.nothing"))
		return 0
	}
	rep, err := core.AdoptApply(home, r, steps, core.AdoptApplyOpts{Only: only, Before: func() error {
		if !withSafetySnapshot(home, "pre-adopt", lang, stderr) {
			return fmt.Errorf("%s", i18n.T(lang, "cli.adopt.no_snapshot"))
		}
		return nil
	}})
	if err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}
	if a.flags["--json"] {
		return snapJSON(stdout, stderr, rep)
	}
	for _, s := range rep.Applied {
		fmt.Fprintln(stdout, okLine(stdout, s.Title))
	}
	for _, s := range rep.Skipped {
		fmt.Fprintln(stdout, warnLine(stdout, s.Title+": "+s.Reason))
	}
	if len(rep.Pending) > 0 {
		fmt.Fprintln(stdout, boldLine(stdout, i18n.T(lang, "cli.adopt.pending_header")))
		for _, s := range rep.Pending {
			fmt.Fprintf(stdout, "  %s — %s\n", s.Title, s.Detail)
		}
	}
	return 0
}

func printAdoptPlan(w io.Writer, lang i18n.Lang, steps []core.AdoptStep, only []string) {
	if len(steps) == 0 {
		fmt.Fprintln(w, i18n.T(lang, "cli.adopt.empty"))
		return
	}
	fmt.Fprintln(w, boldLine(w, i18n.T(lang, "cli.adopt.plan_header")))
	for _, s := range steps {
		mark := "[ ]"
		switch {
		case s.Pending:
			mark = " · "
		case only == nil && s.Default, slices.Contains(only, s.ID):
			mark = "[x]"
		}
		fmt.Fprintf(w, "%s %s  %s\n", mark, mute(w, s.ID), s.Title)
		if s.Detail != "" {
			fmt.Fprintln(w, "      "+mute(w, s.Detail))
		}
	}
	fmt.Fprintln(w, mute(w, i18n.T(lang, "cli.adopt.legend")))
}
