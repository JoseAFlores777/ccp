package cli

// serve_cloud.go — los métodos de nube de `ccp serve` (P-21, spec §10.3).
//
// Lo que pide un secreto por teclado —`cloud login`, `cloud init`, `cloud
// unlock`— NO está aquí: la GUI abre Terminal con el comando, como con
// `/login`. Una frase de bóveda que cruza el puente y pasa por el proceso de la
// app es exactamente lo que el cifrado de extremo a extremo evita.
//
// Lo que sí está es leer el estado y **confirmar lo ejecutable** (D6): el
// agente deja en `review.json` lo que no aplica solo y aquí se cierra, con la
// misma lista que se enseñó.

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/JoseAFlores777/ccp/internal/cloud/agent"
	"github.com/JoseAFlores777/ccp/internal/cloud/api"
	"github.com/JoseAFlores777/ccp/internal/cloud/client"
	"github.com/JoseAFlores777/ccp/internal/core"
)

func (s *server) cloudFiles() client.Files { return client.NewFiles(s.home) }

// srvCloudStatus: la cabecera de la pantalla. Funciona sin sesión y sin red —
// es el estado en el que se abre la primera vez— y nunca se cuelga: preguntar
// arriba por la bóveda tiene el mismo tope de 5 s que `ccp cloud status`.
func srvCloudStatus(s *server, _ json.RawMessage) (any, error) {
	files := s.cloudFiles()
	ctx := context.Background()
	cfg, cl, err := client.Session(ctx, files)
	loggedIn := err == nil
	vault := "unknown"
	if loggedIn {
		if _, err := files.LoadAK(); err == nil {
			vault = "unlocked"
		} else {
			tctx, cancel := context.WithTimeout(ctx, 5*time.Second)
			me, err := cl.Me(tctx)
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
	review, _, _ := agent.LoadReview(files)
	return map[string]any{
		"logged_in": loggedIn, "server": cfg.Server, "email": cfg.Email,
		"device_id": cfg.DeviceID, "device_name": cfg.DeviceName,
		"vault": vault, "pending_push": cloudPendingPush(s.home, files),
		"policy": policyOf(cfg), "pending_review": len(review.Pending),
		"conflicts": len(review.Conflicts), "revision": review.Revision,
	}, nil
}

// cloudPendingPush cuenta los snapshots locales que aún no están arriba. Un
// almacén ilegible vale 0: es un dato informativo, no puede tumbar la pantalla.
func cloudPendingPush(home string, files client.Files) int {
	st, err := core.OpenSnapshotStore(home)
	if err != nil {
		return 0
	}
	ms, err := st.List()
	if err != nil {
		return 0
	}
	state, _ := files.LoadState()
	n := 0
	for _, m := range ms {
		if _, ok := state.Pushed[m.ID]; !ok {
			n++
		}
	}
	return n
}

// srvCloudDevices lista los equipos de la cuenta. `this` va aparte porque la
// fila del propio equipo no se revoca desde aquí: cerrarse la puerta a sí mismo
// deja la máquina sin nube y sin forma de arreglarlo desde la app.
func srvCloudDevices(s *server, _ json.RawMessage) (any, error) {
	files := s.cloudFiles()
	ctx := context.Background()
	cfg, cl, err := client.Session(ctx, files)
	if err != nil {
		return nil, err
	}
	ds, err := cl.Devices(ctx)
	if err != nil {
		return nil, err
	}
	if ds == nil {
		ds = []api.Device{}
	}
	return map[string]any{"this": cfg.DeviceID, "devices": ds}, nil
}

// srvCloudReview devuelve lo que espera a una persona en esta máquina, tal y
// como el agente lo guardó. Sin review.json se devuelve el vacío normalizado,
// no un error: «no hay nada que confirmar» es un estado, no un fallo.
func srvCloudReview(s *server, _ json.RawMessage) (any, error) {
	r, ok, err := agent.LoadReview(s.cloudFiles())
	if err != nil {
		return nil, err
	}
	if !ok {
		r = agent.Review{}
	}
	if r.Pending == nil {
		r.Pending = []agent.Pending{}
	}
	if r.Conflicts == nil {
		r.Conflicts = []agent.Decision{}
	}
	if r.Applied == nil {
		r.Applied = []string{}
	}
	if r.Skipped == nil {
		r.Skipped = []agent.Skipped{}
	}
	return r, nil
}

// srvCloudReviewResolve cierra la revisión: aplica las rutas confirmadas y
// rechaza el resto. `approve` es obligatorio y puede ir vacío —eso es
// «rechazarlo todo»—, para que no haya forma de confirmar por omisión.
func srvCloudReviewResolve(s *server, raw json.RawMessage) (any, error) {
	p, err := params[struct {
		Approve []string `json:"approve"`
	}](raw)
	if err != nil {
		return nil, err
	}
	o, err := cloudAgentOpts(context.Background(), s.home, s.cloudFiles())
	if err != nil {
		return nil, err
	}
	out, err := agent.Resolve(context.Background(), o, p.Approve)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// srvCloudSetPolicy fija la política de este dispositivo. Se guarda aunque no
// haya sesión: decidir de antemano que aquí no se aplica nada solo es
// razonable antes de dar de alta la máquina.
func srvCloudSetPolicy(s *server, raw json.RawMessage) (any, error) {
	p, err := params[struct {
		Policy string `json:"policy"`
	}](raw)
	if err != nil {
		return nil, err
	}
	switch p.Policy {
	case agent.PolicyAuto, agent.PolicyManual:
	default:
		return nil, badParams("política desconocida: %q", p.Policy)
	}
	files := s.cloudFiles()
	cfg, err := files.LoadConfig()
	if err != nil && !errors.Is(err, client.ErrNotLoggedIn) {
		return nil, err
	}
	cfg.Policy = p.Policy
	if err := files.SaveConfig(cfg); err != nil {
		return nil, err
	}
	return map[string]any{"policy": cfg.Policy}, nil
}

// srvCloudRevoke revoca otro equipo. El propio no: ver srvCloudDevices.
func srvCloudRevoke(s *server, raw json.RawMessage) (any, error) {
	p, err := params[struct {
		Device string `json:"device"`
	}](raw)
	if err != nil {
		return nil, err
	}
	if p.Device == "" {
		return nil, badParams("falta el dispositivo")
	}
	files := s.cloudFiles()
	ctx := context.Background()
	cfg, cl, err := client.Session(ctx, files)
	if err != nil {
		return nil, err
	}
	if p.Device == cfg.DeviceID {
		return nil, badParams("este equipo no se revoca a sí mismo")
	}
	if err := cl.RevokeDevice(ctx, p.Device); err != nil {
		return nil, err
	}
	return map[string]any{"device": p.Device}, nil
}
