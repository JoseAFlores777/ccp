package agent

// review.go — la confirmación local (spec §10.3, D6). Lo ejecutable solo entra
// si una persona de esta máquina dice que sí: una cuenta robada no basta para
// ejecutar código en tus máquinas.

import (
	"context"
	"errors"
	"fmt"

	"github.com/JoseAFlores777/ccp/internal/cloud/api"
	"github.com/JoseAFlores777/ccp/internal/cloud/client"
	"github.com/JoseAFlores777/ccp/internal/core"
)

// ErrSuperseded: lo que esperaba confirmación ya no es la orden vigente. No se
// aplica: confirmar a ciegas una orden que el portal ya sustituyó es escribir
// lo que nadie pidió, y encima la nueva llegará en la siguiente pasada.
var ErrSuperseded = errors.New("agent: la revisión que esperaba confirmación ya no está vigente")

// Resolve cierra la revisión que esperaba a una persona: aplica las rutas de
// approve y rechaza el resto. La lista se compara contra la que se guardó, no
// contra una reconciliación nueva: entre que se enseñó y se contestó el disco
// pudo cambiar, y confirmar una lista distinta de la que se leyó sería
// confirmar otra cosa.
func Resolve(ctx context.Context, o Opts, approve []string) (*Outcome, error) {
	review, ok, err := LoadReview(o.Files)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrNothing
	}
	// Que siga siendo la vigente lo dice el servidor: publicar una orden nueva
	// deja la vieja en `superseded` y ya no acepta su resultado.
	rev, err := o.API.PendingRevision(ctx)
	if errors.Is(err, client.ErrNoRevision) || (err == nil && rev.ID != review.Revision) {
		return nil, errors.Join(ErrSuperseded, ClearReview(o.Files))
	}
	if err != nil {
		return nil, err
	}

	yes := map[string]bool{}
	for _, l := range approve {
		yes[l] = true
	}
	out := &Outcome{Revision: review.Revision, Snapshot: review.Snapshot,
		Applied: review.Applied, Pending: []Pending{}, Conflicts: lpaths(review.Conflicts), Skipped: review.Skipped}
	if out.Applied == nil {
		out.Applied = []string{}
	}
	if out.Skipped == nil {
		out.Skipped = []Skipped{}
	}
	var take []string
	for _, p := range review.Pending {
		if yes[p.LPath] {
			take = append(take, p.LPath)
			continue
		}
		out.Skipped = append(out.Skipped, Skipped{LPath: p.LPath, Reason: "no se confirmó en la máquina"})
	}

	if len(take) > 0 {
		rep, err := core.SnapshotRestore(o.Home, o.Src, o.Store, review.Snapshot, core.SnapshotRestoreOpts{
			Only: take, Now: o.now(), Machine: o.Machine})
		if err != nil {
			// La revisión NO se cierra: el disco quedó como estaba (el motor
			// no escribe sin poder guardar antes el estado actual) y la
			// siguiente pasada la volverá a encontrar.
			return out, fmt.Errorf("no se pudo aplicar lo confirmado: %w", err)
		}
		out.PreSnapshot = rep.PreSnapshot
		applied, skipped := tally(rep, out.Skipped)
		out.Applied, out.Skipped = append(out.Applied, applied...), skipped
	}
	if err := ClearReview(o.Files); err != nil {
		return out, err
	}
	state, reason := summarize(out)
	if state == api.RevPartial && len(take) == 0 && len(out.Applied) == 0 {
		reason = "nada se confirmó en la máquina: " + reason
	}
	return out, closeRev(ctx, o, out, state, reason)
}
