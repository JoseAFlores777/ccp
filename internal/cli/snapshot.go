package cli

// snapshot.go — `ccp snapshot …` y los snapshots automáticos (spec 2026-09-18
// §8). La lógica está en internal/snapshot (formato y almacén) y
// core/snapshot_layout.go + snapshot_restore.go (qué se captura y adónde
// vuelve); aquí solo se orquesta y se pinta.

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/mattn/go-isatty"

	"github.com/JoseAFlores777/ccp/internal/core"
	"github.com/JoseAFlores777/ccp/internal/core/i18n"
	"github.com/JoseAFlores777/ccp/internal/snapshot"
)

// dispatchSnapshot maneja `ccp snapshot <subcomando> …`.
func dispatchSnapshot(args []string, stdout, stderr io.Writer) int {
	home, err := ccpHome()
	if err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}
	if err := ensureMigrated(home); err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}
	lang := currentLang()
	sub := ""
	if len(args) > 0 {
		sub, args = args[0], args[1:]
	}
	if sub == "help" || sub == "-h" || sub == "--help" {
		fmt.Fprintln(stdout, i18n.T(lang, "cli.snapshot.usage"))
		return 0
	}
	st, err := core.OpenSnapshotStore(home)
	if err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}
	c := snapCmd{home: home, lang: lang, st: st, out: stdout, err: stderr}
	switch sub {
	case "create":
		return c.create(args)
	case "list", "ls", "":
		return c.list(args)
	case "show":
		return c.show(args)
	case "diff":
		return c.diff(args)
	case "restore":
		return c.restore(args)
	case "prune":
		return c.prune(args)
	case "pin":
		return c.pin(args, true)
	case "unpin":
		return c.pin(args, false)
	case "export":
		return c.export(args)
	case "import":
		return c.importArchive(args)
	default:
		return c.usage("cli.snapshot.unknown_sub", sub)
	}
}

type snapCmd struct {
	home string
	lang i18n.Lang
	st   *snapshot.Store
	out  io.Writer
	err  io.Writer
}

func (c snapCmd) fail(err error) int {
	fmt.Fprintf(c.err, "Error: %v\n", err)
	return 1
}

// usage pinta el motivo y la ayuda del subcomando, y devuelve 1.
func (c snapCmd) usage(key string, a ...any) int {
	fmt.Fprintln(c.err, i18n.T(c.lang, key, a...))
	fmt.Fprintln(c.err, i18n.T(c.lang, "cli.snapshot.usage"))
	return 1
}

// snapArgs son los argumentos de un subcomando ya separados.
type snapArgs struct {
	pos   []string
	flags map[string]bool
	vals  map[string][]string
}

// val devuelve el último valor dado a cualquiera de keys («-m» y «--label» son
// la misma opción).
func (a snapArgs) val(keys ...string) string {
	v := ""
	for _, k := range keys {
		if l := a.vals[k]; len(l) > 0 {
			v = l[len(l)-1]
		}
	}
	return v
}

// has dice si la opción se dio, con el valor que sea (incluido el vacío).
func (a snapArgs) has(keys ...string) bool {
	for _, k := range keys {
		if len(a.vals[k]) > 0 {
			return true
		}
	}
	return false
}

// parseSnapArgs separa posicionales, opciones sin valor (bools) y con valor
// (valued, que pueden repetirse). Si algo no encaja devuelve ok=false y el
// argumento culpable.
func parseSnapArgs(args, bools, valued []string) (a snapArgs, bad string, ok bool) {
	a = snapArgs{flags: map[string]bool{}, vals: map[string][]string{}}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case slices.Contains(bools, arg):
			a.flags[arg] = true
		case slices.Contains(valued, arg):
			if i+1 >= len(args) {
				return a, arg, false
			}
			i++
			a.vals[arg] = append(a.vals[arg], args[i])
		case strings.HasPrefix(arg, "-") && arg != "-":
			return a, arg, false
		default:
			a.pos = append(a.pos, arg)
		}
	}
	return a, "", true
}

// snapSummary es la forma JSON estable de un snapshot en list/create.
type snapSummary struct {
	ID        string `json:"id"`
	Parent    string `json:"parent"`
	Created   string `json:"created"`
	Machine   string `json:"machine"`
	Trigger   string `json:"trigger"`
	Label     string `json:"label"`
	Pinned    bool   `json:"pinned"`
	Items     int    `json:"items"`
	Secrets   int    `json:"secrets"`
	Bytes     int64  `json:"bytes"`
	Unchanged bool   `json:"unchanged,omitempty"`
}

func snapSummaryOf(m *snapshot.Manifest, unchanged bool) snapSummary {
	return snapSummary{
		ID: m.ID, Parent: m.Parent, Created: m.Created.UTC().Format(time.RFC3339),
		Machine: m.Machine, Trigger: m.Trigger, Label: m.Label, Pinned: m.Pinned,
		Items: len(m.Items), Secrets: countClass(m, snapshot.ClassSecret),
		Bytes: snapBytes(m), Unchanged: unchanged,
	}
}

// snapBytes suma lo que captura un manifiesto. No es lo que ocupa en disco: los
// blobs se comparten entre snapshots, así que el segundo de dos snapshots casi
// iguales no añade casi nada. Es el tamaño de lo capturado, que es lo que la
// línea de tiempo necesita para decir de qué estamos hablando.
func snapBytes(m *snapshot.Manifest) int64 {
	var n int64
	for _, it := range m.Items {
		n += it.Size
	}
	return n
}

func countClass(m *snapshot.Manifest, class snapshot.Class) int {
	n := 0
	for _, it := range m.Items {
		if it.Class == class {
			n++
		}
	}
	return n
}

func snapJSON(stdout, stderr io.Writer, v any) int {
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}
	return 0
}

// snapMachine es el nombre con el que un snapshot recuerda de qué máquina salió.
func snapMachine() string {
	if h, err := os.Hostname(); err == nil && h != "" {
		return h
	}
	return "unknown"
}

func (c snapCmd) create(args []string) int {
	a, bad, ok := parseSnapArgs(args, []string{"--with-state", "--json"}, []string{"-m", "--label"})
	if !ok {
		return c.usage("cli.snapshot.unknown_opt", bad)
	}
	if len(a.pos) > 0 {
		return c.usage("cli.snapshot.unknown_opt", a.pos[0])
	}
	src, err := core.ClaudeSrc()
	if err != nil {
		return c.fail(err)
	}
	label := a.val("-m", "--label")
	m, err := core.SnapshotCapture(c.home, src, c.st, core.SnapshotCaptureOpts{
		Trigger: "manual", Label: label, WithState: a.flags["--with-state"],
		// Quien etiqueta quiere ESE punto aunque no haya cambios.
		Force: label != "", Now: time.Now(), Machine: snapMachine(),
	})
	unchanged := errors.Is(err, snapshot.ErrNoChanges)
	if err != nil && !unchanged {
		return c.fail(err)
	}
	if a.flags["--json"] {
		return snapJSON(c.out, c.err, snapSummaryOf(m, unchanged))
	}
	if unchanged {
		fmt.Fprintln(c.out, i18n.T(c.lang, "cli.snapshot.unchanged", snapshot.Short(m.ID)))
		return 0
	}
	fmt.Fprintln(c.out, okLine(c.out, i18n.T(c.lang, "cli.snapshot.created",
		snapshot.Short(m.ID), len(m.Items), countClass(m, snapshot.ClassSecret))))
	return 0
}

func (c snapCmd) list(args []string) int {
	a, bad, ok := parseSnapArgs(args, []string{"--json"}, nil)
	if !ok {
		return c.usage("cli.snapshot.unknown_opt", bad)
	}
	if len(a.pos) > 0 {
		return c.usage("cli.snapshot.unknown_opt", a.pos[0])
	}
	ms, err := c.st.List()
	if err != nil {
		return c.fail(err)
	}
	if a.flags["--json"] {
		out := make([]snapSummary, 0, len(ms))
		for _, m := range ms {
			out = append(out, snapSummaryOf(m, false))
		}
		return snapJSON(c.out, c.err, out)
	}
	if len(ms) == 0 {
		fmt.Fprintln(c.out, i18n.T(c.lang, "cli.snapshot.list_empty"))
		return 0
	}
	tw := tabwriter.NewWriter(c.out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, i18n.T(c.lang, "cli.snapshot.list_header"))
	for _, m := range ms {
		label := m.Label
		if m.Pinned {
			label = strings.TrimSpace(i18n.T(c.lang, "cli.snapshot.pinned_mark") + " " + label)
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%s\n", snapshot.Short(m.ID),
			m.Created.Local().Format("2006-01-02 15:04"), m.Trigger, len(m.Items), label)
	}
	_ = tw.Flush()
	return 0
}

func (c snapCmd) show(args []string) int {
	a, bad, ok := parseSnapArgs(args, []string{"--json"}, nil)
	if !ok {
		return c.usage("cli.snapshot.unknown_opt", bad)
	}
	if len(a.pos) != 1 {
		return c.usage("cli.snapshot.need_id")
	}
	m, err := c.st.LoadManifest(a.pos[0])
	if err != nil {
		return c.fail(err)
	}
	if a.flags["--json"] {
		return snapJSON(c.out, c.err, m)
	}
	fmt.Fprintln(c.out, boldLine(c.out, i18n.T(c.lang, "cli.snapshot.show_header",
		snapshot.Short(m.ID), m.Created.Local().Format("2006-01-02 15:04"), m.Machine, m.Trigger)))
	if m.Parent != "" {
		fmt.Fprintln(c.out, mute(c.out, i18n.T(c.lang, "cli.snapshot.show_parent", snapshot.Short(m.Parent))))
	}
	if m.Label != "" {
		fmt.Fprintln(c.out, i18n.T(c.lang, "cli.snapshot.show_label", m.Label))
	}
	tw := tabwriter.NewWriter(c.out, 0, 0, 2, ' ', 0)
	for _, it := range m.Items {
		fmt.Fprintf(tw, "  %s\t%s\t%s\n", i18n.T(c.lang, "cli.snapshot.class_"+string(it.Class)), humanBytes(it.Size), it.LPath)
	}
	_ = tw.Flush()
	return 0
}

func (c snapCmd) diff(args []string) int {
	a, bad, ok := parseSnapArgs(args, []string{"--json", "--with-state"}, nil)
	if !ok {
		return c.usage("cli.snapshot.unknown_opt", bad)
	}
	if len(a.pos) < 1 || len(a.pos) > 2 {
		return c.usage("cli.snapshot.need_id")
	}
	from, err := c.st.LoadManifest(a.pos[0])
	if err != nil {
		return c.fail(err)
	}
	var to []snapshot.Item
	if len(a.pos) == 2 {
		m, err := c.st.LoadManifest(a.pos[1])
		if err != nil {
			return c.fail(err)
		}
		to = m.Items
	} else {
		src, err := core.ClaudeSrc()
		if err != nil {
			return c.fail(err)
		}
		// Si el snapshot capturó estado, «ahora» también lo incluye: si no, cada
		// conversación saldría como borrada.
		withState := a.flags["--with-state"] || countClass(from, snapshot.ClassState) > 0
		if to, err = core.SnapshotLive(c.home, src, core.SnapshotSourceOpts{WithState: withState}); err != nil {
			return c.fail(err)
		}
	}
	changes := snapshot.Diff(from.Items, to)
	if a.flags["--json"] {
		out := make([]map[string]string, 0, len(changes))
		for _, ch := range changes {
			out = append(out, map[string]string{"lpath": ch.LPath, "kind": string(ch.Kind)})
		}
		return snapJSON(c.out, c.err, out)
	}
	if len(changes) == 0 {
		fmt.Fprintln(c.out, i18n.T(c.lang, "cli.snapshot.diff_none"))
		return 0
	}
	marks := map[snapshot.ChangeKind]string{snapshot.ChangeAdded: "+", snapshot.ChangeRemoved: "-", snapshot.ChangeModified: "~"}
	for _, ch := range changes {
		fmt.Fprintf(c.out, "%s %s\n", marks[ch.Kind], ch.LPath)
	}
	return 0
}

// restore sin --yes enseña el plan y no toca nada: sale con 1 y dice cómo
// aplicarlo. Con --dry-run, el plan y 0. Solo --yes aplica: lo irreversible se
// pide explícitamente, igual que `desktop rm --yes`.
func (c snapCmd) restore(args []string) int {
	a, bad, ok := parseSnapArgs(args, []string{"--dry-run", "--yes", "--json"}, []string{"--only"})
	if !ok {
		return c.usage("cli.snapshot.unknown_opt", bad)
	}
	if len(a.pos) != 1 {
		return c.usage("cli.snapshot.need_id")
	}
	src, err := core.ClaudeSrc()
	if err != nil {
		return c.fail(err)
	}
	o := core.SnapshotRestoreOpts{Only: a.vals["--only"], DryRun: true, Now: time.Now(), Machine: snapMachine()}
	plan, err := core.SnapshotRestore(c.home, src, c.st, a.pos[0], o)
	if err != nil {
		return c.fail(err)
	}
	if !a.flags["--yes"] || a.flags["--dry-run"] {
		if a.flags["--json"] {
			snapJSON(c.out, c.err, plan)
		} else {
			c.printPlan(plan)
		}
		if a.flags["--dry-run"] || planWrites(plan) == 0 {
			return 0
		}
		if !a.flags["--json"] {
			fmt.Fprintln(c.err, warnLine(c.err, i18n.T(c.lang, "cli.snapshot.restore_confirm")))
		}
		return 1
	}
	if planWrites(plan) == 0 {
		if a.flags["--json"] {
			return snapJSON(c.out, c.err, plan)
		}
		fmt.Fprintln(c.out, i18n.T(c.lang, "cli.snapshot.restore_nothing"))
		return 0
	}
	o.DryRun = false
	rep, err := core.SnapshotRestore(c.home, src, c.st, a.pos[0], o)
	if err != nil {
		return c.fail(err)
	}
	if a.flags["--json"] {
		return snapJSON(c.out, c.err, rep)
	}
	c.printPlan(rep)
	fmt.Fprintln(c.out, okLine(c.out, i18n.T(c.lang, "cli.snapshot.restored", snapshot.Short(rep.PreSnapshot))))
	if len(rep.Regenerated) > 0 {
		fmt.Fprintln(c.out, mute(c.out, i18n.T(c.lang, "cli.snapshot.regenerated", strings.Join(rep.Regenerated, ", "))))
	}
	return 0
}

// planWrites cuenta los pasos que escribirían algo.
func planWrites(r *core.SnapshotRestoreReport) int {
	n := 0
	for _, s := range r.Steps {
		if s.Action == "write" || s.Action == "merge" {
			n++
		}
	}
	return n
}

func (c snapCmd) printPlan(r *core.SnapshotRestoreReport) {
	fmt.Fprintln(c.out, boldLine(c.out, i18n.T(c.lang, "cli.snapshot.restore_plan", snapshot.Short(r.Snapshot))))
	same := 0
	for _, s := range r.Steps {
		switch s.Action {
		case "same":
			same++
		case "write", "merge":
			fmt.Fprintf(c.out, "  %s  %s\n", i18n.T(c.lang, "cli.snapshot.act_"+s.Action), s.LPath)
		case "skip":
			fmt.Fprintf(c.out, "  %s  %s  (%s)\n", i18n.T(c.lang, "cli.snapshot.act_skip"), s.LPath,
				i18n.T(c.lang, "cli.snapshot.reason_"+s.Reason))
		}
	}
	if same > 0 {
		fmt.Fprintln(c.out, mute(c.out, i18n.T(c.lang, "cli.snapshot.restore_same", same)))
	}
	// El snapshot viene de otra máquina: lo que se escribe no es literalmente
	// lo que guardó, y decirlo aquí es la única ocasión antes de escribirlo.
	if r.HomeFrom != "" {
		fmt.Fprintln(c.out, mute(c.out, i18n.T(c.lang, "cli.snapshot.home_translated", r.HomeFrom, r.HomeTo)))
	}
}

func (c snapCmd) prune(args []string) int {
	a, bad, ok := parseSnapArgs(args, []string{"--dry-run", "--json"}, nil)
	if !ok {
		return c.usage("cli.snapshot.unknown_opt", bad)
	}
	if len(a.pos) > 0 {
		return c.usage("cli.snapshot.unknown_opt", a.pos[0])
	}
	dry := a.flags["--dry-run"]
	rep, err := snapshot.Prune(c.st, snapshot.DefaultPolicy, time.Now(), time.Hour, dry)
	if err != nil {
		return c.fail(err)
	}
	if a.flags["--json"] {
		deleted := rep.Deleted
		if deleted == nil {
			deleted = []string{}
		}
		return snapJSON(c.out, c.err, map[string]any{
			"deleted": deleted, "kept": rep.Kept, "blobs_deleted": rep.BlobsDeleted, "dry_run": dry,
		})
	}
	key := "cli.snapshot.pruned"
	if dry {
		key = "cli.snapshot.prune_dry"
	}
	fmt.Fprintln(c.out, i18n.T(c.lang, key, len(rep.Deleted), rep.BlobsDeleted, rep.Kept))
	return 0
}

func (c snapCmd) pin(args []string, pinned bool) int {
	a, bad, ok := parseSnapArgs(args, nil, []string{"-m", "--label"})
	if !ok {
		return c.usage("cli.snapshot.unknown_opt", bad)
	}
	if len(a.pos) != 1 {
		return c.usage("cli.snapshot.need_id")
	}
	// Por PRESENCIA de la opción, no por su valor: `-m ""` es la única forma
	// de BORRAR una etiqueta (SetPin solo mira label != nil), y es lo que la
	// GUI ofrece como equivalente cuando se vacía el campo.
	var label *string
	if a.has("-m", "--label") {
		l := a.val("-m", "--label")
		label = &l
	}
	m, err := c.st.SetPin(a.pos[0], pinned, label)
	if err != nil {
		return c.fail(err)
	}
	key := "cli.snapshot.pinned"
	if !pinned {
		key = "cli.snapshot.unpinned"
	}
	fmt.Fprintln(c.out, okLine(c.out, i18n.T(c.lang, key, snapshot.Short(m.ID))))
	return 0
}

// snapMinPassphrase: una frase más corta no aguanta un ataque de diccionario
// contra un archivo que puede acabar en cualquier sitio.
const snapMinPassphrase = 12

// readSnapPassphrase pide la frase de un .ccpsnap. CCP_SNAPSHOT_PASSPHRASE la
// da sin preguntar (scripts y tests); si no, se pide por la terminal sin eco,
// como `ccp key`. confirm (al exportar) exige longitud mínima y repetirla.
func readSnapPassphrase(lang i18n.Lang, w io.Writer, confirm bool) ([]byte, error) {
	check := func(p string) error {
		if confirm && len([]rune(p)) < snapMinPassphrase {
			return errors.New(i18n.T(lang, "cli.snapshot.pass_short", snapMinPassphrase))
		}
		return nil
	}
	if p := os.Getenv("CCP_SNAPSHOT_PASSPHRASE"); p != "" {
		return []byte(p), check(p)
	}
	if !isatty.IsTerminal(os.Stdin.Fd()) {
		return nil, errors.New(i18n.T(lang, "cli.snapshot.pass_needed"))
	}
	p1, err := promptSecret(w, i18n.T(lang, "cli.snapshot.pass_prompt"))
	if err != nil {
		return nil, err
	}
	if err := check(p1); err != nil {
		return nil, err
	}
	if confirm {
		p2, err := promptSecret(w, i18n.T(lang, "cli.snapshot.pass_confirm"))
		if err != nil {
			return nil, err
		}
		if p1 != p2 {
			return nil, errors.New(i18n.T(lang, "cli.snapshot.pass_mismatch"))
		}
	}
	return []byte(p1), nil
}

func (c snapCmd) export(args []string) int {
	a, bad, ok := parseSnapArgs(args, []string{"--with-secrets"}, nil)
	if !ok {
		return c.usage("cli.snapshot.unknown_opt", bad)
	}
	if len(a.pos) != 2 {
		return c.usage("cli.snapshot.need_file")
	}
	var pass []byte
	if a.flags["--with-secrets"] {
		p, err := readSnapPassphrase(c.lang, c.err, true)
		if err != nil {
			return c.fail(err)
		}
		pass = p
	}
	dest := a.pos[1]
	perm := os.FileMode(0o644)
	if pass != nil {
		perm = 0o600
	}
	tmp := dest + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, perm)
	if err != nil {
		return c.fail(err)
	}
	m, err := snapshot.Export(c.st, a.pos[0], f, pass)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(tmp, dest)
	}
	if err != nil {
		_ = os.Remove(tmp)
		return c.fail(err)
	}
	key := "cli.snapshot.exported"
	if pass != nil {
		key = "cli.snapshot.exported_secrets"
	}
	fmt.Fprintln(c.out, okLine(c.out, i18n.T(c.lang, key, snapshot.Short(m.ID), dest)))
	return 0
}

func (c snapCmd) importArchive(args []string) int {
	a, bad, ok := parseSnapArgs(args, nil, nil)
	if !ok {
		return c.usage("cli.snapshot.unknown_opt", bad)
	}
	if len(a.pos) != 1 {
		return c.usage("cli.snapshot.need_file")
	}
	f, err := os.Open(a.pos[0])
	if err != nil {
		return c.fail(err)
	}
	defer f.Close()
	rep, err := snapshot.Import(c.st, f, func() ([]byte, error) { return readSnapPassphrase(c.lang, c.err, false) })
	if err != nil {
		return c.fail(err)
	}
	fmt.Fprintln(c.out, okLine(c.out, i18n.T(c.lang, "cli.snapshot.imported", snapshot.Short(rep.Manifest.ID), len(rep.Manifest.Items))))
	if len(rep.Missing) > 0 {
		fmt.Fprintln(c.out, warnLine(c.out, i18n.T(c.lang, "cli.snapshot.imported_missing", len(rep.Missing))))
	}
	return 0
}

// --- snapshots automáticos (spec §8.3) ---

// autoSnapshot guarda el estado antes de una operación que borra o sustituye
// configuración y devuelve su id ("" si está apagado). Si no sale, quien llama
// NO sigue: una red que falla en silencio es peor que no tenerla. Si no hubo
// cambios desde el último snapshot, ese último ya es la red.
// CCP_NO_AUTO_SNAPSHOT=1 lo apaga.
func autoSnapshot(home, trigger string) (string, error) {
	if os.Getenv("CCP_NO_AUTO_SNAPSHOT") == "1" {
		return "", nil
	}
	src, err := core.ClaudeSrc()
	if err != nil {
		return "", err
	}
	st, err := core.OpenSnapshotStore(home)
	if err != nil {
		return "", err
	}
	m, err := core.SnapshotCapture(home, src, st, core.SnapshotCaptureOpts{Trigger: trigger, Now: time.Now(), Machine: snapMachine()})
	if err != nil && !errors.Is(err, snapshot.ErrNoChanges) {
		return "", err
	}
	return m.ID, nil
}

// withSafetySnapshot es autoSnapshot para la CLI: informa del id por stderr o,
// si falla, pinta por qué y devuelve false.
func withSafetySnapshot(home, trigger string, lang i18n.Lang, stderr io.Writer) bool {
	id, err := autoSnapshot(home, trigger)
	if err != nil {
		fmt.Fprintln(stderr, errLine(stderr, i18n.T(lang, "cli.snapshot.auto_failed", err)))
		return false
	}
	if id != "" {
		fmt.Fprintln(stderr, mute(stderr, i18n.T(lang, "cli.snapshot.auto_saved", snapshot.Short(id))))
	}
	return true
}

// dailySnapshotCmds son los comandos de gestión, los que teclea una persona. Los
// de scripting (_env, _hook, resolve, status, path, completion) corren en cada
// prompt o desde otros programas y no pueden pagar el coste de una captura.
var dailySnapshotCmds = map[string]bool{
	"profile": true, "account": true, "instruct": true, "backup": true, "config": true,
	"desktop": true, "auto": true, "handoff": true, "session": true, "snapshot": true,
	"doctor": true, "lang": true, "upgrade": true, "update": true, "adopt": true,
	"mcp": true,
}

// dailySnapshotEvery: 20 horas y no 24, para que quien abre ccp a la misma hora
// cada día no se salte un día por minutos.
const dailySnapshotEvery = 20 * time.Hour

// maybeDailySnapshot toma el snapshot del día si toca. Es best-effort: nunca
// falla ni imprime, porque el comando que se pidió no puede fallar por la copia
// del día. La marca .daily-check evita volver a leer y hashear todo en cada
// comando cuando no hubo cambios.
func maybeDailySnapshot(now time.Time) {
	if os.Getenv("CCP_NO_AUTO_SNAPSHOT") == "1" {
		return
	}
	home, err := ccpHome()
	if err != nil {
		return
	}
	if _, err := os.Stat(filepath.Join(home, "ccp.yaml")); err != nil {
		return // sin configuración aún: nada que guardar
	}
	marker := filepath.Join(core.SnapshotStoreDir(home), ".daily-check")
	if fi, err := os.Stat(marker); err == nil && now.Sub(fi.ModTime()) < dailySnapshotEvery {
		return
	}
	st, err := core.OpenSnapshotStore(home)
	if err != nil {
		return
	}
	touch := func() {
		if os.WriteFile(marker, nil, 0o600) == nil {
			_ = os.Chtimes(marker, now, now)
		}
	}
	if last, ok := st.LatestTime(); ok && now.Sub(last) < dailySnapshotEvery {
		touch()
		return
	}
	src, err := core.ClaudeSrc()
	if err != nil {
		return
	}
	_, err = core.SnapshotCapture(home, src, st, core.SnapshotCaptureOpts{Trigger: "daily", Now: now, Machine: snapMachine()})
	if err == nil || errors.Is(err, snapshot.ErrNoChanges) {
		touch()
	}
}
