package cli

// sync.go — `ccp sync …` (spec 2026-09-18 §9, E2): publicar la configuración
// en una carpeta o un bucket, sin operar ningún servicio. El motor está en
// internal/cloud/remote; aquí se orquesta y se pinta.
//
// Lo que `sync` NO tiene, y es a propósito: cuentas, dispositivos y auditoría.
// Una carpeta no sabe quién escribió en ella, así que nada de eso se puede
// contar sin inventárselo. Lo que sí tiene es lo que importa: lo que sale de
// aquí va sellado y firmado con la clave de la bóveda, y quien opera la
// carpeta mueve bultos que no sabe abrir.

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/JoseAFlores777/ccp/internal/cloud/client"
	"github.com/JoseAFlores777/ccp/internal/cloud/crypt"
	"github.com/JoseAFlores777/ccp/internal/cloud/remote"
	"github.com/JoseAFlores777/ccp/internal/core"
	"github.com/JoseAFlores777/ccp/internal/core/i18n"
	"github.com/JoseAFlores777/ccp/internal/snapshot"
)

// Los secretos de la bóveda de un destino, sin preguntar. Se llaman como los
// de la nube con otro prefijo: son el mismo secreto de otra bóveda.
const (
	envSyncPassphrase = "CCP_SYNC_PASSPHRASE"
	envSyncRecovery   = "CCP_SYNC_RECOVERY"
)

func dispatchSync(args []string, stdout, stderr io.Writer) int {
	home, err := ccpHome()
	if err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}
	if err := ensureMigrated(home); err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	c := syncCmd{ctx: ctx, home: home, lang: currentLang(), out: stdout, err: stderr}
	sub := ""
	if len(args) > 0 {
		sub, args = args[0], args[1:]
	}
	switch sub {
	case "remote", "remotes":
		return c.remote(args)
	case "push":
		return c.push(args)
	case "pull":
		return c.pull(args)
	case "apply":
		return c.apply(args)
	case "help", "-h", "--help", "":
		fmt.Fprintln(stdout, i18n.T(c.lang, "cli.sync.usage"))
		return 0
	default:
		return c.usage("cli.sync.unknown_sub", sub)
	}
}

type syncCmd struct {
	ctx  context.Context
	home string
	lang i18n.Lang
	out  io.Writer
	err  io.Writer
}

func (c syncCmd) usage(key string, a ...any) int {
	fmt.Fprintln(c.err, i18n.T(c.lang, key, a...))
	fmt.Fprintln(c.err, i18n.T(c.lang, "cli.sync.usage"))
	return 1
}

func (c syncCmd) fail(err error) int {
	switch {
	case errors.Is(err, crypt.ErrWrongSecret):
		fmt.Fprintln(c.err, i18n.T(c.lang, "cli.cloud.unlock_wrong"))
	default:
		fmt.Fprintf(c.err, "Error: %v\n", err)
	}
	return 1
}

func (c syncCmd) args(args, bools, valued []string, maxPos int) (snapArgs, bool) {
	a, bad, ok := parseSnapArgs(args, bools, append(valued, "--remote"))
	if !ok {
		c.usage("cli.sync.unknown_opt", bad)
		return a, false
	}
	if len(a.pos) > maxPos {
		c.usage("cli.sync.unknown_opt", a.pos[maxPos])
		return a, false
	}
	return a, true
}

// pick elige el destino: el que dice --remote o, si solo hay uno, ese.
// Adivinar entre varios sería publicar en la carpeta equivocada, así que con
// más de uno se pide el nombre.
func (c syncCmd) pick(name string) (remote.Entry, error) {
	reg, err := remote.LoadRegistry(c.home)
	if err != nil {
		return remote.Entry{}, err
	}
	if name != "" {
		e, ok := reg.Find(name)
		if !ok {
			return remote.Entry{}, errors.New(i18n.T(c.lang, "cli.sync.no_such_remote", name))
		}
		return e, nil
	}
	switch len(reg.Remotes) {
	case 0:
		return remote.Entry{}, errors.New(i18n.T(c.lang, "cli.sync.no_remotes"))
	case 1:
		return reg.Remotes[0], nil
	default:
		names := make([]string, 0, len(reg.Remotes))
		for _, e := range reg.Remotes {
			names = append(names, e.Name)
		}
		sort.Strings(names)
		return remote.Entry{}, errors.New(i18n.T(c.lang, "cli.sync.many_remotes", strings.Join(names, ", ")))
	}
}

// ready abre el destino, su cuenta y el almacén local: lo que necesitan push,
// pull y apply.
func (c syncCmd) ready(name string) (*remote.Store, *crypt.Account, *snapshot.Store, remote.Entry, error) {
	e, err := c.pick(name)
	if err != nil {
		return nil, nil, nil, e, err
	}
	r, err := remote.Open(e.URL)
	if err != nil {
		return nil, nil, nil, e, err
	}
	files := remote.FilesFor(c.home, e.Name)
	ak, err := files.LoadAK()
	if errors.Is(err, client.ErrLocked) {
		return nil, nil, nil, e, errors.New(i18n.T(c.lang, "cli.sync.locked", e.Name, e.Name))
	}
	if err != nil {
		return nil, nil, nil, e, err
	}
	acct, err := crypt.NewAccount(ak)
	if err != nil {
		return nil, nil, nil, e, err
	}
	if err := c.sameVault(r, acct, e.Name); err != nil {
		return nil, nil, nil, e, err
	}
	st, err := core.OpenSnapshotStore(c.home)
	if err != nil {
		return nil, nil, nil, e, err
	}
	return r, acct, st, e, nil
}

// sameVault comprueba que la bóveda que hay AHORA en el destino es la que abre
// la clave de este equipo. `PutVault` ya impide pisar una bóveda, pero no
// impide que la carpeta pierda su remote.json —iCloud Drive, Dropbox y
// Syncthing lo hacen— y que otra máquina cree allí la suya: desde ese momento
// las dos trabajan en el mismo sitio con AK distintas. Nada aguas abajo lo
// nota, porque cada una sella y verifica con la suya: push seguiría diciendo
// «subido» sobre unos blobs que la otra no podrá abrir jamás, y un pull de lo
// ajeno saldría como firma inválida, o sea acusando de manipulación algo que
// no es un ataque. Se dice aquí, una vez, y con la salida: volver a añadir el
// destino (que es lo que reemplaza la clave y olvida el estado local).
func (c syncCmd) sameVault(r *remote.Store, acct *crypt.Account, name string) error {
	v, err := r.Vault(c.ctx)
	if errors.Is(err, remote.ErrNoVault) {
		return errors.New(i18n.T(c.lang, "cli.sync.vault_gone", name, name))
	}
	if err != nil {
		return err
	}
	if !bytes.Equal(acct.SignPublic(), v.SignPub) {
		return errors.New(i18n.T(c.lang, "cli.sync.vault_other", name, name))
	}
	return nil
}

func (c syncCmd) remote(args []string) int {
	sub := ""
	if len(args) > 0 {
		sub, args = args[0], args[1:]
	}
	switch sub {
	case "add":
		return c.remoteAdd(args)
	case "list", "ls", "":
		return c.remoteList(args)
	case "rm", "remove":
		return c.remoteRm(args)
	default:
		return c.usage("cli.sync.remote_sub", sub)
	}
}

// remoteAdd registra un destino y deja su bóveda abierta en este equipo. Son
// una sola orden porque son una sola decisión: un destino sin bóveda abierta
// no sirve para nada, y la primera máquina la crea mientras la segunda la
// desbloquea con la misma frase sin tener que saber cuál de las dos es.
func (c syncCmd) remoteAdd(args []string) int {
	a, ok := c.args(args, []string{"--recovery"}, nil, 2)
	if !ok {
		return 1
	}
	if len(a.pos) == 0 {
		return c.usage("cli.sync.need_name")
	}
	name, url := a.pos[0], ""
	if len(a.pos) > 1 {
		url = a.pos[1]
	}
	reg, err := remote.LoadRegistry(c.home)
	if err != nil {
		return c.fail(err)
	}
	e, yaEsta := reg.Find(name)
	switch {
	case yaEsta && url != "" && url != e.URL:
		// Dos URLs bajo un nombre serían dos bóvedas compartiendo el
		// directorio de estado, y la clave de una no abre la otra.
		fmt.Fprintln(c.err, i18n.T(c.lang, "cli.sync.remote_other_url", name, e.URL, name))
		return 1
	case yaEsta:
		url = e.URL // volver a añadirlo es volver a desbloquearlo
		// Find no distingue mayúsculas (la carpeta tampoco en APFS): se
		// sigue usando el nombre REGISTRADO, no el que se acaba de teclear,
		// para no crear un segundo directorio donde el sistema de archivos
		// sí las distinga.
		name = e.Name
	case url == "":
		return c.usage("cli.sync.need_url")
	default:
		// El nombre se valida ANTES de abrir nada: si no vale, no se crea
		// ninguna bóveda en una carpeta que luego habría que limpiar a mano.
		if err := reg.Add(name, url); err != nil {
			return c.fail(err)
		}
	}
	r, err := remote.Open(url)
	if err != nil {
		return c.fail(err)
	}
	files := remote.FilesFor(c.home, name)
	v, err := r.Vault(c.ctx)
	switch {
	case errors.Is(err, remote.ErrNoVault):
		if err := forgetOtherVault(files, nil); err != nil {
			return c.fail(err)
		}
		if code := c.initVault(r, files); code != 0 {
			return code
		}
	case err != nil:
		return c.fail(err)
	default:
		if err := forgetOtherVault(files, v.SignPub); err != nil {
			return c.fail(err)
		}
		if code := c.unlockVault(r, files, a.flags["--recovery"]); code != 0 {
			return code
		}
	}
	if !yaEsta {
		if err := remote.SaveRegistry(c.home, reg); err != nil {
			return c.fail(err)
		}
	}
	fmt.Fprintln(c.out, okLine(c.out, i18n.T(c.lang, "cli.sync.remote_added", name, url)))
	return 0
}

// forgetOtherVault olvida el estado local cuando la bóveda que hay ahora en el
// destino no es la que abre la clave guardada: signPub nil significa que no hay
// bóveda ninguna (la carpeta se vació, iCloud la perdió) y una distinta, que la
// rehizo otra máquina. Volver a añadir el destino reemplaza la clave, pero sin
// esto `state.json` conservaba el mapa de «ya subido» de la bóveda anterior y
// el siguiente push decía «Nada que subir» contra un destino vacío: el usuario
// creería publicada una configuración que no está en ninguna parte. Es la misma
// defensa que la nube aplica al cambiar de cuenta o de servidor.
func forgetOtherVault(files client.Files, signPub []byte) error {
	ak, err := files.LoadAK()
	if errors.Is(err, client.ErrLocked) {
		return nil // nada que olvidar: este equipo nunca la abrió
	}
	if err == nil && signPub != nil {
		if acct, aerr := crypt.NewAccount(ak); aerr == nil && bytes.Equal(acct.SignPublic(), signPub) {
			return nil // es la misma bóveda: el estado local sigue siendo cierto
		}
	}
	return files.ForgetVault()
}

// initVault crea la bóveda del destino. El código de recuperación se enseña
// ANTES de tocar el disco, y el orden no es cosmético: la bóveda ya está
// creada de forma irrepetible y de ella solo queda la envoltura, nunca el
// código, que hasta ese momento vivía únicamente en esta memoria. Si guardar
// la clave falla se pierde el desbloqueo de este equipo, que se recupera con
// la frase; el código no se podría reemitir jamás.
func (c syncCmd) initVault(r *remote.Store, files client.Files) int {
	pass, err := readSecret(c.lang, c.err, envSyncPassphrase, "cli.cloud.pass_prompt", true)
	if err != nil {
		return c.fail(err)
	}
	ak, code, err := remote.InitVault(c.ctx, r, []byte(pass))
	if err != nil {
		return c.fail(err)
	}
	fmt.Fprintln(c.out, boldLine(c.out, i18n.T(c.lang, "cli.sync.recovery_title")))
	fmt.Fprintln(c.out, "    "+boldLine(c.out, code))
	fmt.Fprintln(c.out, i18n.T(c.lang, "cli.sync.recovery_hint"))
	fmt.Fprintln(c.out)
	if err := files.SaveAK(ak); err != nil {
		fmt.Fprintln(c.err, i18n.T(c.lang, "cli.sync.vault_created_locked"))
		return c.fail(err)
	}
	return 0
}

func (c syncCmd) unlockVault(r *remote.Store, files client.Files, byRecovery bool) int {
	var ak []byte
	if byRecovery {
		code, err := readSecret(c.lang, c.err, envSyncRecovery, "cli.cloud.recovery_prompt", false)
		if err != nil {
			return c.fail(err)
		}
		if ak, err = remote.UnlockRecovery(c.ctx, r, code); err != nil {
			return c.fail(err)
		}
	} else {
		pass, err := readSecret(c.lang, c.err, envSyncPassphrase, "cli.cloud.pass_prompt", false)
		if err != nil {
			return c.fail(err)
		}
		if ak, err = remote.Unlock(c.ctx, r, []byte(pass)); err != nil {
			return c.fail(err)
		}
	}
	if err := files.SaveAK(ak); err != nil {
		return c.fail(err)
	}
	return 0
}

// syncRemoteRow es la forma JSON estable de `ccp sync remote list --json`.
// `unlocked` dice si ESTA máquina puede abrir el destino, que es lo que
// decide si push y pull van a funcionar.
type syncRemoteRow struct {
	Name     string    `json:"name"`
	URL      string    `json:"url"`
	Added    time.Time `json:"added"`
	Unlocked bool      `json:"unlocked"`
}

func (c syncCmd) remoteList(args []string) int {
	a, ok := c.args(args, []string{"--json"}, nil, 0)
	if !ok {
		return 1
	}
	reg, err := remote.LoadRegistry(c.home)
	if err != nil {
		return c.fail(err)
	}
	rows := make([]syncRemoteRow, 0, len(reg.Remotes))
	for _, e := range reg.Remotes {
		_, err := remote.FilesFor(c.home, e.Name).LoadAK()
		rows = append(rows, syncRemoteRow{Name: e.Name, URL: e.URL, Added: e.Added, Unlocked: err == nil})
	}
	if a.flags["--json"] {
		return snapJSON(c.out, c.err, rows)
	}
	if len(rows) == 0 {
		fmt.Fprintln(c.out, i18n.T(c.lang, "cli.sync.remote_none"))
		return 0
	}
	w := tabwriter.NewWriter(c.out, 0, 0, 2, ' ', 0)
	for _, r := range rows {
		estado := i18n.T(c.lang, "cli.sync.locked_mark")
		if r.Unlocked {
			estado = ""
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n", r.Name, r.URL, estado)
	}
	_ = w.Flush()
	return 0
}

func (c syncCmd) remoteRm(args []string) int {
	a, ok := c.args(args, nil, nil, 1)
	if !ok {
		return 1
	}
	if len(a.pos) == 0 {
		return c.usage("cli.sync.need_name")
	}
	name := a.pos[0]
	reg, err := remote.LoadRegistry(c.home)
	if err != nil {
		return c.fail(err)
	}
	e, found := reg.Find(name)
	if !found {
		fmt.Fprintln(c.err, i18n.T(c.lang, "cli.sync.no_such_remote", name))
		return 1
	}
	name = e.Name // el registrado, no el tecleado (ver remoteAdd)
	reg.Remove(name)
	if err := remote.SaveRegistry(c.home, reg); err != nil {
		return c.fail(err)
	}
	// Se borra lo de AQUÍ —la clave desbloqueada y lo que este equipo creía
	// subido—, nunca lo del destino: quitar un remoto no puede ser la forma
	// accidental de perder la configuración de todas las máquinas.
	if err := os.RemoveAll(remote.FilesFor(c.home, name).Dir); err != nil {
		return c.fail(err)
	}
	fmt.Fprintln(c.out, okLine(c.out, i18n.T(c.lang, "cli.sync.remote_removed", name, e.URL)))
	return 0
}

func (c syncCmd) push(args []string) int {
	a, ok := c.args(args, []string{"--json"}, nil, 1)
	if !ok {
		return 1
	}
	r, acct, st, e, err := c.ready(a.val("--remote"))
	if err != nil {
		return c.fail(err)
	}
	only := ""
	if len(a.pos) == 1 {
		if only, err = st.Resolve(a.pos[0]); err != nil {
			return c.fail(err)
		}
	}
	rep, err := remote.Push(c.ctx, r, acct, st, remote.FilesFor(c.home, e.Name), only)
	if err != nil {
		return c.fail(err)
	}
	if a.flags["--json"] {
		return snapJSON(c.out, c.err, rep)
	}
	if rep.Snapshots == 0 {
		fmt.Fprintln(c.out, i18n.T(c.lang, "cli.sync.push_nothing", e.Name))
	} else {
		fmt.Fprintln(c.out, okLine(c.out, i18n.T(c.lang, "cli.sync.pushed", e.Name, rep.Snapshots, rep.Uploaded, humanBytes(rep.Bytes))))
	}
	if len(rep.Missing) > 0 {
		fmt.Fprintln(c.out, warnLine(c.out, i18n.T(c.lang, "cli.sync.push_missing", len(rep.Missing))))
	}
	if len(rep.TooLarge) > 0 {
		fmt.Fprintln(c.out, warnLine(c.out, i18n.T(c.lang, "cli.sync.push_too_large", strings.Join(rep.TooLarge, ", "))))
	}
	return 0
}

// pullOne baja un snapshot del destino al almacén local y devuelve su
// manifiesto. Lo comparten `pull` y `apply`: aplicar lo que está en la carpeta
// es bajarlo primero, y dos caminos para traerlo serían dos formas de traer
// cosas distintas.
func (c syncCmd) pullOne(r *remote.Store, acct *crypt.Account, st *snapshot.Store,
	e remote.Entry, ref string) (*snapshot.Manifest, []string, error) {
	target, err := remote.Pick(c.ctx, r, ref)
	if err != nil {
		return nil, nil, err
	}
	return remote.Pull(c.ctx, r, acct, st, remote.FilesFor(c.home, e.Name), target)
}

func (c syncCmd) pull(args []string) int {
	a, ok := c.args(args, []string{"--json"}, nil, 1)
	if !ok {
		return 1
	}
	r, acct, st, e, err := c.ready(a.val("--remote"))
	if err != nil {
		return c.fail(err)
	}
	ref := ""
	if len(a.pos) == 1 {
		ref = a.pos[0]
	}
	m, missing, err := c.pullOne(r, acct, st, e, ref)
	if errors.Is(err, remote.ErrNoSnapshots) {
		// Que el destino esté vacío no es un fallo: es una carpeta recién
		// elegida, y quien la eligió tiene que poder distinguirlo de un error.
		fmt.Fprintln(c.out, i18n.T(c.lang, "cli.sync.pull_none", e.Name))
		return 0
	}
	if err != nil {
		return c.fail(err)
	}
	if a.flags["--json"] {
		return snapJSON(c.out, c.err, snapSummaryOf(m, false))
	}
	fmt.Fprintln(c.out, okLine(c.out, i18n.T(c.lang, "cli.sync.pulled", e.Name, snapshot.Short(m.ID))))
	if len(missing) > 0 {
		fmt.Fprintln(c.out, warnLine(c.out, i18n.T(c.lang, "cli.sync.pull_missing", len(missing))))
	}
	fmt.Fprintln(c.out, mute(c.out, i18n.T(c.lang, "cli.sync.pull_hint", snapshot.Short(m.ID))))
	return 0
}

// apply deja esta máquina en un snapshot del destino: lo baja y lo restaura.
//
// Bajar primero también con --plan es a propósito: no se puede planear lo que
// no se tiene, y bajar solo AÑADE al almacén local —no toca ni un archivo de
// la configuración—, así que ver el plan sigue sin cambiar nada.
//
// Sin --yes enseña el plan y sale 1, como `ccp snapshot restore`: es la misma
// operación destructiva y una regla distinta sería una trampa. --plan es la
// forma de pedir solo el plan y que eso NO sea un fallo (sale 0).
func (c syncCmd) apply(args []string) int {
	a, ok := c.args(args, []string{"--plan", "--dry-run", "--yes", "--json"}, []string{"--only"}, 1)
	if !ok {
		return 1
	}
	soloPlan := a.flags["--plan"] || a.flags["--dry-run"]
	r, acct, st, e, err := c.ready(a.val("--remote"))
	if err != nil {
		return c.fail(err)
	}
	src, err := core.ClaudeSrc()
	if err != nil {
		return c.fail(err)
	}
	ref := ""
	if len(a.pos) == 1 {
		ref = a.pos[0]
	}
	m, missing, err := c.pullOne(r, acct, st, e, ref)
	if errors.Is(err, remote.ErrNoSnapshots) {
		fmt.Fprintln(c.out, i18n.T(c.lang, "cli.sync.pull_none", e.Name))
		return 0
	}
	if err != nil {
		return c.fail(err)
	}
	if len(missing) > 0 {
		// Lo que no llegó no se puede restaurar, y decirlo antes del plan es
		// la única ocasión: en el plan esas rutas salen como «se salta».
		fmt.Fprintln(c.err, warnLine(c.err, i18n.T(c.lang, "cli.sync.pull_missing", len(missing))))
	}
	o := core.SnapshotRestoreOpts{Only: a.vals["--only"], DryRun: true, Now: time.Now(), Machine: snapMachine()}
	plan, err := core.SnapshotRestore(c.home, src, st, m.ID, o)
	if err != nil {
		return c.fail(err)
	}
	if !a.flags["--yes"] || soloPlan {
		if a.flags["--json"] {
			snapJSON(c.out, c.err, plan)
		} else {
			c.printPlan(plan)
		}
		if soloPlan || planWrites(plan) == 0 {
			return 0
		}
		if !a.flags["--json"] {
			fmt.Fprintln(c.err, warnLine(c.err, i18n.T(c.lang, "cli.sync.apply_confirm")))
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
	rep, err := core.SnapshotRestore(c.home, src, st, m.ID, o)
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

// printPlan es el MISMO impresor que `ccp snapshot restore` y `ccp cloud
// restore`: tres formas de contar el mismo plan acabarían contando cosas
// distintas del mismo plan.
func (c syncCmd) printPlan(r *core.SnapshotRestoreReport) {
	printRestorePlan(c.out, c.lang, r)
}
