package cli

// cloud.go — `ccp cloud …` (spec 2026-09-18 §10). La lógica está en
// internal/cloud/client; aquí se orquesta y se pinta.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/mattn/go-isatty"

	"github.com/JoseAFlores777/ccp/internal/cloud/api"
	"github.com/JoseAFlores777/ccp/internal/cloud/client"
	"github.com/JoseAFlores777/ccp/internal/cloud/crypt"
	"github.com/JoseAFlores777/ccp/internal/core"
	"github.com/JoseAFlores777/ccp/internal/core/i18n"
	"github.com/JoseAFlores777/ccp/internal/snapshot"
)

func dispatchCloud(args []string, stdout, stderr io.Writer) int {
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
	c := cloudCmd{ctx: ctx, home: home, lang: currentLang(), files: client.NewFiles(home), out: stdout, err: stderr}
	sub := ""
	if len(args) > 0 {
		sub, args = args[0], args[1:]
	}
	switch sub {
	case "login":
		return c.login(args)
	case "logout":
		return c.logout(args)
	case "status", "":
		return c.status(args)
	case "init":
		return c.initVault(args)
	case "unlock":
		return c.unlock(args)
	case "push":
		return c.push(args)
	case "pull":
		return c.pull(args)
	case "list", "ls":
		return c.list(args)
	case "devices":
		return c.devices(args)
	case "revoke":
		return c.revoke(args)
	case "help", "-h", "--help":
		fmt.Fprintln(stdout, i18n.T(c.lang, "cli.cloud.usage"))
		return 0
	default:
		return c.usage("cli.cloud.unknown_sub", sub)
	}
}

type cloudCmd struct {
	ctx   context.Context
	home  string
	lang  i18n.Lang
	files client.Files
	out   io.Writer
	err   io.Writer
}

func (c cloudCmd) usage(key string, a ...any) int {
	fmt.Fprintln(c.err, i18n.T(c.lang, key, a...))
	fmt.Fprintln(c.err, i18n.T(c.lang, "cli.cloud.usage"))
	return 1
}

// fail traduce los errores con solución conocida a su mensaje.
func (c cloudCmd) fail(err error) int {
	switch {
	case errors.Is(err, client.ErrNotLoggedIn):
		fmt.Fprintln(c.err, i18n.T(c.lang, "cli.cloud.not_logged_in"))
	case errors.Is(err, client.ErrLocked):
		fmt.Fprintln(c.err, i18n.T(c.lang, "cli.cloud.locked"))
	case errors.Is(err, crypt.ErrWrongSecret):
		fmt.Fprintln(c.err, i18n.T(c.lang, "cli.cloud.unlock_wrong"))
	default:
		fmt.Fprintf(c.err, "Error: %v\n", err)
	}
	return 1
}

func (c cloudCmd) args(args, bools, valued []string, maxPos int) (snapArgs, bool) {
	a, bad, ok := parseSnapArgs(args, bools, valued)
	if !ok {
		c.usage("cli.cloud.unknown_opt", bad)
		return a, false
	}
	if len(a.pos) > maxPos {
		c.usage("cli.cloud.unknown_opt", a.pos[maxPos])
		return a, false
	}
	return a, true
}

// openBrowser abre la URL del login en macOS. Best-effort: la URL también se
// imprime. CCP_NO_BROWSER lo apaga (tests, SSH).
func openBrowser(u string) {
	if os.Getenv("CCP_NO_BROWSER") != "" || runtime.GOOS != "darwin" {
		return
	}
	_ = exec.Command("open", u).Start()
}

// readCloudSecret lee un secreto de la variable env o, en una terminal, sin eco.
// confirm pide longitud mínima y repetirlo (al crear la bóveda).
func readCloudSecret(c cloudCmd, env, promptKey string, confirm bool) (string, error) {
	if v := os.Getenv(env); v != "" {
		if confirm && len([]rune(v)) < snapMinPassphrase {
			return "", errors.New(i18n.T(c.lang, "cli.cloud.pass_short", snapMinPassphrase))
		}
		return v, nil
	}
	if !isatty.IsTerminal(os.Stdin.Fd()) {
		return "", errors.New(i18n.T(c.lang, "cli.cloud.pass_needed", env))
	}
	v, err := promptSecret(c.err, i18n.T(c.lang, promptKey))
	if err != nil {
		return "", err
	}
	if !confirm {
		return v, nil
	}
	if len([]rune(v)) < snapMinPassphrase {
		return "", errors.New(i18n.T(c.lang, "cli.cloud.pass_short", snapMinPassphrase))
	}
	again, err := promptSecret(c.err, i18n.T(c.lang, "cli.cloud.pass_confirm"))
	if err != nil {
		return "", err
	}
	if again != v {
		return "", errors.New(i18n.T(c.lang, "cli.cloud.pass_mismatch"))
	}
	return v, nil
}

func (c cloudCmd) login(args []string) int {
	a, ok := c.args(args, nil, []string{"--name"}, 1)
	if !ok {
		return 1
	}
	server := ""
	if len(a.pos) == 1 {
		server = strings.TrimSuffix(a.pos[0], "/")
	} else if prev, err := c.files.LoadConfig(); err == nil {
		server = prev.Server
	}
	if server == "" {
		server = os.Getenv("CCP_CLOUD_URL")
	}
	if server == "" {
		return c.usage("cli.cloud.need_server")
	}
	// http solo para localhost: en cualquier otro sitio el token del
	// dispositivo viajaría en claro.
	local := strings.HasPrefix(server, "http://127.0.0.1") || strings.HasPrefix(server, "http://localhost")
	if !strings.HasPrefix(server, "https://") && !local {
		fmt.Fprintln(c.err, i18n.T(c.lang, "cli.cloud.need_https"))
		return 1
	}
	ep, err := client.Discover(c.ctx, http.DefaultClient, server)
	if err != nil {
		return c.fail(err)
	}
	oc := client.OAuthConfig(ep.Info.ClientID, ep.TokenURL, ep.DeviceAuthURL)
	tok, err := client.Login(c.ctx, oc, func(u, code string) {
		fmt.Fprintln(c.out, i18n.T(c.lang, "cli.cloud.login_open", u, code))
		openBrowser(u)
	})
	if err != nil {
		return c.fail(err)
	}
	if err := c.files.SaveToken(tok); err != nil {
		return c.fail(err)
	}
	cl := client.NewAPI(server, client.HTTPClient(c.ctx, oc, c.files, tok), "")
	me, err := cl.Me(c.ctx)
	if err != nil {
		return c.fail(err)
	}
	cfg := client.Config{Server: server, Issuer: ep.Info.Issuer, ClientID: ep.Info.ClientID,
		TokenURL: ep.TokenURL, DeviceURL: ep.DeviceAuthURL, UserID: me.UserID, Email: me.Email}
	// Si este equipo ya estaba registrado con esta cuenta, se reusa su dispositivo.
	// Y si NO es la misma cuenta (o el mismo servidor), se olvida la bóveda
	// anterior antes de nada: la AK abre una sola bóveda y dejarla viva haría
	// que `push` sellara y firmara con ella contra la cuenta nueva.
	switched := false
	if prev, err := c.files.LoadConfig(); err == nil {
		if prev.Server == server && prev.UserID == me.UserID {
			if prev.DeviceID != "" {
				cfg.DeviceID, cfg.DeviceName = prev.DeviceID, prev.DeviceName
			}
		} else if prev.UserID != "" || prev.Server != "" {
			if _, err := c.files.LoadAK(); err == nil {
				switched = true
			}
			if err := c.files.ForgetVault(); err != nil {
				return c.fail(err)
			}
		}
	}
	if cfg.DeviceID == "" {
		name := a.val("--name")
		if name == "" {
			name = snapMachine()
		}
		d, err := cl.RegisterDevice(c.ctx, api.DeviceIn{Name: name, Platform: runtime.GOOS + "/" + runtime.GOARCH, CCPVersion: core.Version})
		if err != nil {
			return c.fail(err)
		}
		cfg.DeviceID, cfg.DeviceName = d.ID, d.Name
	}
	if err := c.files.SaveConfig(cfg); err != nil {
		return c.fail(err)
	}
	fmt.Fprintln(c.out, okLine(c.out, i18n.T(c.lang, "cli.cloud.login_ok", me.Email, cfg.DeviceName)))
	if switched {
		fmt.Fprintln(c.out, i18n.T(c.lang, "cli.cloud.login_switched"))
	}
	if me.HasVault {
		fmt.Fprintln(c.out, i18n.T(c.lang, "cli.cloud.next_unlock"))
	} else {
		fmt.Fprintln(c.out, i18n.T(c.lang, "cli.cloud.next_init"))
	}
	return 0
}

func (c cloudCmd) logout(args []string) int {
	if _, ok := c.args(args, nil, nil, 0); !ok {
		return 1
	}
	// Revocar arriba es best-effort: sin conexión, el equipo se olvida igual.
	if cfg, cl, err := client.Session(c.ctx, c.files); err == nil {
		_ = cl.RevokeDevice(c.ctx, cfg.DeviceID)
	}
	if err := c.files.Forget(); err != nil {
		return c.fail(err)
	}
	fmt.Fprintln(c.out, okLine(c.out, i18n.T(c.lang, "cli.cloud.logged_out")))
	return 0
}

// pendingPush cuenta los snapshots locales que la nube aún no tiene. Es un
// número para enseñar, no una decisión: si el almacén local no abre, cero.
func (c cloudCmd) pendingPush() int {
	st, err := core.OpenSnapshotStore(c.home)
	if err != nil {
		return 0
	}
	ms, err := st.List()
	if err != nil {
		return 0
	}
	state, _ := c.files.LoadState()
	n := 0
	for _, m := range ms {
		if _, ok := state.Pushed[m.ID]; !ok {
			n++
		}
	}
	return n
}

func (c cloudCmd) status(args []string) int {
	a, ok := c.args(args, []string{"--json"}, nil, 0)
	if !ok {
		return 1
	}
	cfg, cl, err := client.Session(c.ctx, c.files)
	loggedIn := err == nil
	vault := "unknown"
	if loggedIn {
		if _, err := c.files.LoadAK(); err == nil {
			vault = "unlocked"
		} else {
			// Sin AK hay que preguntar arriba, y `status` no puede colgarse por
			// eso: cinco segundos y, si no, «desconocido».
			ctx, cancel := context.WithTimeout(c.ctx, 5*time.Second)
			me, err := cl.Me(ctx)
			cancel()
			switch {
			case err != nil:
				vault = "unknown"
			case me.HasVault:
				vault = "locked"
			default:
				vault = "missing"
			}
		}
	}
	pending := c.pendingPush()
	if a.flags["--json"] {
		return snapJSON(c.out, c.err, map[string]any{
			"logged_in": loggedIn, "server": cfg.Server, "email": cfg.Email, "device_id": cfg.DeviceID,
			"device_name": cfg.DeviceName, "vault": vault, "pending_push": pending,
		})
	}
	if !loggedIn {
		fmt.Fprintln(c.out, i18n.T(c.lang, "cli.cloud.not_logged_in"))
		return 0
	}
	fmt.Fprintln(c.out, i18n.T(c.lang, "cli.cloud.status_server", cfg.Server))
	fmt.Fprintln(c.out, i18n.T(c.lang, "cli.cloud.status_account", cfg.Email))
	fmt.Fprintln(c.out, i18n.T(c.lang, "cli.cloud.status_device", cfg.DeviceName))
	fmt.Fprintln(c.out, i18n.T(c.lang, "cli.cloud.status_vault", i18n.T(c.lang, "cli.cloud.vault_"+vault)))
	fmt.Fprintln(c.out, i18n.T(c.lang, "cli.cloud.status_pending", pending))
	return 0
}

func (c cloudCmd) initVault(args []string) int {
	if _, ok := c.args(args, nil, nil, 0); !ok {
		return 1
	}
	_, cl, err := client.Session(c.ctx, c.files)
	if err != nil {
		return c.fail(err)
	}
	if me, err := cl.Me(c.ctx); err != nil {
		return c.fail(err)
	} else if me.HasVault {
		fmt.Fprintln(c.err, i18n.T(c.lang, "cli.cloud.vault_exists"))
		return 1
	}
	pass, err := readCloudSecret(c, "CCP_CLOUD_PASSPHRASE", "cli.cloud.pass_prompt", true)
	if err != nil {
		return c.fail(err)
	}
	ak, code, w, err := crypt.NewVault([]byte(pass))
	if err != nil {
		return c.fail(err)
	}
	v, err := client.VaultToAPI(w)
	if err != nil {
		return c.fail(err)
	}
	if err := cl.PutVault(c.ctx, v); err != nil {
		// Dos máquinas creando la bóveda a la vez: el servidor rechaza la
		// segunda, y ahí lo que toca es desbloquear, no reintentar.
		var apiErr *client.APIError
		if errors.As(err, &apiErr) && apiErr.Code == api.CodeConflict {
			fmt.Fprintln(c.err, i18n.T(c.lang, "cli.cloud.vault_exists"))
			return 1
		}
		return c.fail(err)
	}
	if err := c.files.SaveAK(ak); err != nil {
		return c.fail(err)
	}
	fmt.Fprintln(c.out, okLine(c.out, i18n.T(c.lang, "cli.cloud.vault_created")))
	fmt.Fprintln(c.out)
	fmt.Fprintln(c.out, boldLine(c.out, i18n.T(c.lang, "cli.cloud.recovery_title")))
	fmt.Fprintln(c.out, "    "+boldLine(c.out, code))
	fmt.Fprintln(c.out, i18n.T(c.lang, "cli.cloud.recovery_hint"))
	return 0
}

func (c cloudCmd) unlock(args []string) int {
	a, ok := c.args(args, []string{"--recovery"}, nil, 0)
	if !ok {
		return 1
	}
	_, cl, err := client.Session(c.ctx, c.files)
	if err != nil {
		return c.fail(err)
	}
	v, err := cl.Vault(c.ctx)
	var apiErr *client.APIError
	if errors.As(err, &apiErr) && apiErr.Code == api.CodeNotFound {
		fmt.Fprintln(c.err, i18n.T(c.lang, "cli.cloud.no_vault"))
		return 1
	}
	if err != nil {
		return c.fail(err)
	}
	w, err := client.VaultFromAPI(v)
	if err != nil {
		return c.fail(err)
	}
	var ak []byte
	if a.flags["--recovery"] {
		code, err := readCloudSecret(c, "CCP_CLOUD_RECOVERY", "cli.cloud.recovery_prompt", false)
		if err != nil {
			return c.fail(err)
		}
		if ak, err = crypt.UnlockRecovery(w, code); err != nil {
			return c.fail(err)
		}
	} else {
		pass, err := readCloudSecret(c, "CCP_CLOUD_PASSPHRASE", "cli.cloud.pass_prompt", false)
		if err != nil {
			return c.fail(err)
		}
		if ak, err = crypt.UnlockPassphrase(w, []byte(pass)); err != nil {
			return c.fail(err)
		}
	}
	if err := c.files.SaveAK(ak); err != nil {
		return c.fail(err)
	}
	fmt.Fprintln(c.out, okLine(c.out, i18n.T(c.lang, "cli.cloud.unlocked")))
	return 0
}

// ready abre la sesión, la cuenta y el almacén local: lo que necesitan push y pull.
func (c cloudCmd) ready() (*client.API, *crypt.Account, *snapshot.Store, error) {
	_, cl, err := client.Session(c.ctx, c.files)
	if err != nil {
		return nil, nil, nil, err
	}
	acct, err := client.Account(c.files)
	if err != nil {
		return nil, nil, nil, err
	}
	st, err := core.OpenSnapshotStore(c.home)
	if err != nil {
		return nil, nil, nil, err
	}
	return cl, acct, st, nil
}

func (c cloudCmd) push(args []string) int {
	a, ok := c.args(args, []string{"--json"}, nil, 1)
	if !ok {
		return 1
	}
	cl, acct, st, err := c.ready()
	if err != nil {
		return c.fail(err)
	}
	only := ""
	if len(a.pos) == 1 {
		if only, err = st.Resolve(a.pos[0]); err != nil {
			return c.fail(err)
		}
	}
	rep, err := client.Push(c.ctx, cl, acct, st, c.files, only)
	if err != nil {
		return c.fail(err)
	}
	if a.flags["--json"] {
		return snapJSON(c.out, c.err, rep)
	}
	if rep.Snapshots == 0 {
		fmt.Fprintln(c.out, i18n.T(c.lang, "cli.cloud.push_nothing"))
	} else {
		fmt.Fprintln(c.out, okLine(c.out, i18n.T(c.lang, "cli.cloud.pushed", rep.Snapshots, rep.Uploaded, humanBytes(rep.Bytes))))
	}
	if len(rep.Missing) > 0 {
		fmt.Fprintln(c.out, warnLine(c.out, i18n.T(c.lang, "cli.cloud.push_missing", len(rep.Missing))))
	}
	if len(rep.TooLarge) > 0 {
		fmt.Fprintln(c.out, warnLine(c.out, i18n.T(c.lang, "cli.cloud.push_too_large", strings.Join(rep.TooLarge, ", "))))
	}
	return 0
}

func (c cloudCmd) pull(args []string) int {
	a, ok := c.args(args, nil, []string{"--device"}, 1)
	if !ok {
		return 1
	}
	cl, acct, st, err := c.ready()
	if err != nil {
		return c.fail(err)
	}
	device := ""
	if name := a.val("--device"); name != "" {
		d, err := c.findDevice(cl, name)
		if err != nil {
			return c.fail(err)
		}
		device = d.ID
	}
	list, err := cl.Snapshots(c.ctx, device, 1000)
	if err != nil {
		return c.fail(err)
	}
	if len(list) == 0 {
		fmt.Fprintln(c.out, i18n.T(c.lang, "cli.cloud.pull_none"))
		return 0
	}
	target := list[0].ID // «latest» por defecto: la lista viene del más nuevo al más viejo
	if len(a.pos) == 1 && a.pos[0] != "latest" {
		var found []string
		for _, s := range list {
			if strings.HasPrefix(s.ID, a.pos[0]) {
				found = append(found, s.ID)
			}
		}
		if len(found) != 1 || len(a.pos[0]) < 4 {
			fmt.Fprintln(c.err, i18n.T(c.lang, "cli.cloud.pull_ambiguous", a.pos[0]))
			return 1
		}
		target = found[0]
	}
	m, missing, err := client.Pull(c.ctx, cl, acct, st, c.files, target)
	if err != nil {
		return c.fail(err)
	}
	fmt.Fprintln(c.out, okLine(c.out, i18n.T(c.lang, "cli.cloud.pulled", snapshot.Short(target), snapshot.Short(m.ID))))
	if len(missing) > 0 {
		fmt.Fprintln(c.out, warnLine(c.out, i18n.T(c.lang, "cli.cloud.pull_missing", len(missing))))
	}
	fmt.Fprintln(c.out, mute(c.out, i18n.T(c.lang, "cli.cloud.pull_hint", snapshot.Short(m.ID))))
	return 0
}

func (c cloudCmd) list(args []string) int {
	a, ok := c.args(args, []string{"--json"}, nil, 0)
	if !ok {
		return 1
	}
	_, cl, err := client.Session(c.ctx, c.files)
	if err != nil {
		return c.fail(err)
	}
	list, err := cl.Snapshots(c.ctx, "", 1000)
	if err != nil {
		return c.fail(err)
	}
	// «Aquí» se sabe por el estado local: la nube no puede decirlo, porque el
	// id de allá arriba es un MAC del de aquí y solo esta máquina los ata.
	state, _ := c.files.LoadState()
	here := map[string]bool{}
	for _, cloudID := range state.Pushed {
		here[cloudID] = true
	}
	if a.flags["--json"] {
		type row struct {
			api.SnapshotMeta
			Here bool `json:"here"`
		}
		out := make([]row, 0, len(list))
		for _, s := range list {
			out = append(out, row{s, here[s.ID]})
		}
		return snapJSON(c.out, c.err, out)
	}
	tw := tabwriter.NewWriter(c.out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, i18n.T(c.lang, "cli.cloud.list_header"))
	for _, s := range list {
		mark := ""
		if here[s.ID] {
			mark = i18n.T(c.lang, "cli.cloud.list_here")
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", snapshot.Short(s.ID), s.Created.Local().Format("2006-01-02 15:04"), s.DeviceName, humanBytes(s.Size), mark)
	}
	_ = tw.Flush()
	return 0
}

func (c cloudCmd) findDevice(cl *client.API, ref string) (api.Device, error) {
	ds, err := cl.Devices(c.ctx)
	if err != nil {
		return api.Device{}, err
	}
	var found []api.Device
	for _, d := range ds {
		if d.Name == ref || strings.HasPrefix(d.ID, ref) {
			found = append(found, d)
		}
	}
	if len(found) != 1 {
		return api.Device{}, errors.New(i18n.T(c.lang, "cli.cloud.device_unknown", ref))
	}
	return found[0], nil
}

func (c cloudCmd) devices(args []string) int {
	a, ok := c.args(args, []string{"--json"}, nil, 0)
	if !ok {
		return 1
	}
	cfg, cl, err := client.Session(c.ctx, c.files)
	if err != nil {
		return c.fail(err)
	}
	ds, err := cl.Devices(c.ctx)
	if err != nil {
		return c.fail(err)
	}
	if a.flags["--json"] {
		return snapJSON(c.out, c.err, ds)
	}
	tw := tabwriter.NewWriter(c.out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, i18n.T(c.lang, "cli.cloud.devices_header"))
	for _, d := range ds {
		status := i18n.T(c.lang, "cli.cloud.device_active")
		switch {
		case d.ID == cfg.DeviceID:
			status = i18n.T(c.lang, "cli.cloud.device_this")
		case d.Revoked:
			status = i18n.T(c.lang, "cli.cloud.device_revoked")
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", shortID(d.ID), d.Name, d.Platform, d.LastSeen.Local().Format("2006-01-02 15:04"), status)
	}
	_ = tw.Flush()
	return 0
}

func (c cloudCmd) revoke(args []string) int {
	a, ok := c.args(args, nil, nil, 1)
	if !ok {
		return 1
	}
	if len(a.pos) != 1 {
		return c.usage("cli.cloud.device_needed")
	}
	cfg, cl, err := client.Session(c.ctx, c.files)
	if err != nil {
		return c.fail(err)
	}
	d, err := c.findDevice(cl, a.pos[0])
	if err != nil {
		return c.fail(err)
	}
	if d.ID == cfg.DeviceID {
		fmt.Fprintln(c.err, i18n.T(c.lang, "cli.cloud.revoke_self"))
		return 1
	}
	if err := cl.RevokeDevice(c.ctx, d.ID); err != nil {
		return c.fail(err)
	}
	fmt.Fprintln(c.out, okLine(c.out, i18n.T(c.lang, "cli.cloud.revoked", d.Name)))
	return 0
}

// shortID recorta un id para la tabla sin asumir su longitud: un id más corto
// de lo esperado no puede tumbar el listado de dispositivos.
func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}
