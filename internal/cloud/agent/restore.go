package agent

// restore.go — restaurar desde la nube en ESTA máquina (spec §10.3.1, caminos
// 1 y 3). El camino 2 (el portal) no pasa por aquí: llega como revisión
// firmada y lo aplica `Once`. Los tres terminan en el mismo motor, el restore
// de §8.3, que planifica, toma un snapshot previo y regenera la proyección.
//
// Lo que este camino añade sobre `ccp snapshot restore` es lo que solo tiene
// sentido con un snapshot AJENO: bajarlo, mapear sus proyectos a las carpetas
// de aquí (§11) y decir qué queda por hacer a mano.

import (
	"context"

	"github.com/JoseAFlores777/ccp/internal/cloud/client"
	"github.com/JoseAFlores777/ccp/internal/core"
)

// RestoreOpts son las opciones de una restauración desde la nube.
type RestoreOpts struct {
	// Only limita a esas rutas lógicas (o a sus prefijos); vacío = todo.
	Only []string
	// Projects mapea a mano la clave portable de un proyecto a su carpeta
	// aquí. Lo que no se nombre se resuelve solo (§11).
	Projects map[string]string
	DryRun   bool
}

// RestoreReport es el resultado, o el plan si DryRun.
type RestoreReport struct {
	// Cloud es el id en la nube; Snapshot, el mismo snapshot ya en el almacén
	// local. Son distintos a propósito: el de la nube es un MAC del local y
	// solo esta máquina los ata, así que hay que enseñar los dos.
	Cloud    string                      `json:"cloud"`
	Snapshot string                      `json:"snapshot"`
	Plan     *core.SnapshotRestoreReport `json:"plan"`
	Projects []core.ProjectMapping       `json:"projects"`
	Pending  []core.RestorePendingItem   `json:"pending"`
	// NoData son las rutas cuyo contenido no está en la nube (un snapshot que
	// se subió sin secretos). No se restauran, y el motor las cuenta aparte.
	NoData []string `json:"no_data"`
}

// Restore baja el snapshot cloudID y lo aplica aquí. Con DryRun no escribe
// nada y devuelve el plan — que es lo que la GUI y el CLI enseñan antes de
// pedir confirmación —, pero SÍ baja el snapshot: sin sus datos no hay diff
// que enseñar, y bajarlo no toca ninguna configuración.
func Restore(ctx context.Context, o Opts, cloudID string, ro RestoreOpts) (*RestoreReport, error) {
	desired, noData, err := client.Pull(ctx, o.API, o.Acct, o.Store, o.Files, cloudID)
	if err != nil {
		return nil, err
	}
	if noData == nil {
		noData = []string{}
	}
	in := core.ProjectMapInputsFor(o.Home, o.Src)
	in.Manual = ro.Projects
	projects := core.SnapshotProjects(desired, in)
	rep := &RestoreReport{Cloud: cloudID, Snapshot: desired.ID, Projects: projects, NoData: noData,
		Pending: core.SnapshotPending(o.Store, desired, core.SnapshotPendingInputsFor(o.Home, projects))}
	rep.Plan, err = core.SnapshotRestore(o.Home, o.Src, o.Store, desired.ID, core.SnapshotRestoreOpts{
		Only: ro.Only, Projects: ResolvedProjects(projects), DryRun: ro.DryRun,
		Now: o.now(), Machine: o.Machine})
	if err != nil {
		return nil, err
	}
	return rep, nil
}

// ResolvedProjects deja solo los proyectos que tienen carpeta aquí. Los que no
// se encontraron NO se mapean: el motor los salta como `project_missing`, que
// es exactamente lo que son, en vez de escribir en una ruta inventada.
func ResolvedProjects(ms []core.ProjectMapping) map[string]string {
	out := map[string]string{}
	for _, m := range ms {
		if m.Path != "" && m.Source != core.ProjectMapMissing {
			out[m.Key] = m.Path
		}
	}
	return out
}
