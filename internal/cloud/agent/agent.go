package agent

// agent.go — la máquina aplica (spec §10.3, ADR 0014). El portal no abre
// conexiones hacia aquí: es este lado el que tira, verifica la firma con la
// clave de cuenta —que el servidor no tiene—, reconcilia a tres bandas y
// escribe solo lo que no choca y no ejecuta código.
//
// Una revisión se cierra UNA sola vez (el servidor solo acepta el resultado
// mientras sigue pendiente), así que quien la cierra es quien tiene la última
// palabra: si algo espera a una persona, el agente NO la cierra y deja el
// trabajo en `review.json`; la cierra `ccp cloud review`. Cerrarla antes con
// «parcial, esperando confirmación» dejaría al portal con un resultado que ya
// no se puede corregir cuando la persona confirme.

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/JoseAFlores777/ccp/internal/cloud/api"
	"github.com/JoseAFlores777/ccp/internal/cloud/client"
	"github.com/JoseAFlores777/ccp/internal/cloud/crypt"
	"github.com/JoseAFlores777/ccp/internal/core"
	"github.com/JoseAFlores777/ccp/internal/snapshot"
)

// Políticas de dispositivo (§10.3).
const (
	// PolicyAuto aplica sin preguntar lo que no ejecuta código.
	PolicyAuto = "auto"
	// PolicyManual no aplica nada sin confirmación. Para una máquina donde ni
	// siquiera un CLAUDE.md debe cambiar solo.
	PolicyManual = "manual"
)

// ReviewFile es el archivo del directorio de nube donde espera lo que necesita
// una persona.
const ReviewFile = "review.json"

// Opts son las dependencias del agente. Todas inyectadas: una pasada entera
// cabe en un test con su propio CCP_HOME y un servidor de mentira.
type Opts struct {
	Home     string // CCP_HOME
	Src      string // ~/.claude
	API      *client.API
	Acct     *crypt.Account
	Store    *snapshot.Store
	Files    client.Files
	DeviceID string
	Policy   string
	Machine  string
	Now      func() time.Time
}

func (o Opts) now() time.Time {
	if o.Now != nil {
		return o.Now()
	}
	return time.Now()
}

// Pending es un cambio que no se aplica sin que alguien lo mire.
type Pending struct {
	LPath string   `json:"lpath"`
	Why   []Danger `json:"why"`
}

// Skipped es algo que la revisión pedía y no se pudo hacer.
type Skipped struct {
	LPath  string `json:"lpath"`
	Reason string `json:"reason"`
}

// Review es lo que queda esperando a una persona en esta máquina. Se guarda
// entero para que `ccp cloud review` no tenga que rehacer la reconciliación:
// entre las dos llamadas el disco pudo cambiar, y confirmar una lista distinta
// de la que se enseñó sería confirmar otra cosa.
type Review struct {
	Revision  string     `json:"revision"`
	Cloud     string     `json:"cloud"`    // id del snapshot deseado en la nube
	Snapshot  string     `json:"snapshot"` // el mismo, ya en el almacén local
	Created   time.Time  `json:"created"`
	Pending   []Pending  `json:"pending"`
	Conflicts []Decision `json:"conflicts"`
	Applied   []string   `json:"applied"`
	Skipped   []Skipped  `json:"skipped"`
}

// Outcome es lo que hizo una pasada del agente.
type Outcome struct {
	Revision    string    `json:"revision"`
	Snapshot    string    `json:"snapshot,omitempty"`
	PreSnapshot string    `json:"pre_snapshot,omitempty"`
	Applied     []string  `json:"applied"`
	Pending     []Pending `json:"pending"`
	Conflicts   []string  `json:"conflicts"`
	Skipped     []Skipped `json:"skipped"`
	// State es el resultado que se informó al servidor, o "" si la revisión
	// sigue abierta esperando a una persona.
	State  string `json:"state"`
	Reason string `json:"reason,omitempty"`
	// Waiting: hay algo en `ccp cloud review`.
	Waiting bool `json:"waiting"`
}

// ErrNothing es «no había nada que aplicar». Lo devuelve Once para que quien
// lo llama en bucle no tenga que mirar un Outcome vacío.
var ErrNothing = errors.New("agent: no hay ninguna revisión pendiente")

// Once hace una pasada: recoge la revisión pendiente de este equipo, la
// verifica, reconcilia y aplica lo que puede.
func Once(ctx context.Context, o Opts) (*Outcome, error) {
	rev, err := o.API.PendingRevision(ctx)
	if errors.Is(err, client.ErrNoRevision) {
		return nil, ErrNothing
	}
	if err != nil {
		return nil, err
	}
	out := &Outcome{Revision: rev.ID, Applied: []string{}, Pending: []Pending{}, Conflicts: []string{}, Skipped: []Skipped{}}

	// Si ya está esperando a una persona no se vuelve a tocar nada: rehacerlo
	// cada pocos minutos repetiría el snapshot previo y, peor, cambiaría bajo
	// sus pies la lista que se le enseñó para confirmar.
	if r, ok, err := LoadReview(o.Files); err != nil {
		return nil, err
	} else if ok && r.Revision == rev.ID {
		out.Snapshot, out.Pending, out.Applied, out.Skipped = r.Snapshot, r.Pending, r.Applied, r.Skipped
		out.Conflicts, out.Waiting = lpaths(r.Conflicts), true
		return out, nil
	}

	// La firma es lo primero: mientras no verifique, esto no es una orden del
	// dueño de la cuenta, es una cadena de bytes que mandó un servidor.
	parts := crypt.RevisionParts{ID: rev.ID, Prev: rev.Prev, Device: rev.DeviceID,
		Snapshot: rev.Snapshot, Base: rev.Base, Body: rev.Body}
	if err := o.Acct.VerifyRevision(parts, rev.Sig); err != nil {
		return out, closeRev(ctx, o, out, api.RevFailed,
			"la firma de la revisión no verifica con la clave de esta cuenta; no se aplicó nada")
	}
	// El destinatario va dentro de la firma, así que esto no lo puede montar
	// el servidor; se comprueba igual porque el coste es cero y el fallo sería
	// aplicar en la máquina equivocada.
	if rev.DeviceID != o.DeviceID {
		return out, closeRev(ctx, o, out, api.RevFailed,
			"la revisión está dirigida a otro dispositivo ("+rev.DeviceID+")")
	}
	if rev.Snapshot == "" {
		return out, closeRev(ctx, o, out, api.RevFailed,
			"esta versión de ccp solo aplica revisiones que nombran un snapshot")
	}
	return out, applyRevision(ctx, o, rev, out)
}

// LoadReview lee lo que espera confirmación en esta máquina.
func LoadReview(files client.Files) (Review, bool, error) {
	var r Review
	ok, err := files.LoadJSON(ReviewFile, &r)
	return r, ok, err
}

// saveReview normaliza las listas antes de guardar: `review.json` lo lee
// `ccp cloud review --json` y, por la misma regla que el resto del CLI, una
// lista vacía es [] y nunca null — quien lo consuma hace `.conflicts | length`.
func saveReview(files client.Files, r Review) error {
	if r.Pending == nil {
		r.Pending = []Pending{}
	}
	if r.Conflicts == nil {
		r.Conflicts = []Decision{}
	}
	if r.Applied == nil {
		r.Applied = []string{}
	}
	if r.Skipped == nil {
		r.Skipped = []Skipped{}
	}
	return files.SaveJSON(ReviewFile, r)
}

// ClearReview olvida lo que esperaba confirmación.
func ClearReview(files client.Files) error { return files.RemoveJSON(ReviewFile) }

func lpaths(ds []Decision) []string {
	out := make([]string, 0, len(ds))
	for _, d := range ds {
		out = append(out, d.LPath)
	}
	sort.Strings(out)
	return out
}

// closeRev informa del resultado y lo deja en el Outcome. Un fallo al informar
// no borra lo que se hizo: se devuelve el error, y la próxima pasada volverá a
// encontrar la revisión pendiente y la reconciliará contra un disco que ya
// tiene lo aplicado, así que no repetirá nada.
func closeRev(ctx context.Context, o Opts, out *Outcome, state, reason string) error {
	reason = trimReason(reason)
	out.State, out.Reason = state, reason
	if _, err := o.API.SetRevisionState(ctx, out.Revision, state, reason); err != nil {
		return fmt.Errorf("no se pudo informar del resultado de la revisión: %w", err)
	}
	return nil
}

// trimReason recorta el motivo al tope del servidor, que cuenta BYTES. El «…»
// entra DENTRO del presupuesto (son 3 bytes, no 1: recortar a MaxReasonLen-1 y
// concatenarlo dejaba 2002 bytes, el servidor devolvía 400 y la revisión no se
// cerraba jamás), y el corte retrocede hasta frontera de runa porque los
// motivos llevan acentos y media runa se convierte en U+FFFD al serializar.
func trimReason(reason string) string {
	if len(reason) <= api.MaxReasonLen {
		return reason
	}
	const ell = "…"
	cut := reason[:api.MaxReasonLen-len(ell)]
	for len(cut) > 0 {
		if r, n := utf8.DecodeLastRuneInString(cut); r != utf8.RuneError || n > 1 {
			break
		}
		cut = cut[:len(cut)-1]
	}
	return cut + ell
}

// summarize decide el estado final y su motivo. El orden importa: un conflicto
// es lo que una persona tiene que resolver, y decir «parcial» cuando lo que
// hay es un choque manda a buscar la explicación al sitio equivocado.
func summarize(out *Outcome) (string, string) {
	switch {
	case len(out.Conflicts) > 0:
		return api.RevConflict, fmt.Sprintf("%d rutas cambiaron aquí y en la revisión: %s",
			len(out.Conflicts), strings.Join(out.Conflicts, ", "))
	case len(out.Skipped) > 0:
		var parts []string
		for _, s := range out.Skipped {
			parts = append(parts, s.LPath+" ("+s.Reason+")")
		}
		return api.RevPartial, fmt.Sprintf("aplicadas %d rutas; %d sin aplicar: %s",
			len(out.Applied), len(out.Skipped), strings.Join(parts, ", "))
	default:
		return api.RevApplied, ""
	}
}

// applyRevision es el cuerpo: bajar el deseado, resolver la base, reconciliar
// contra lo vivo y escribir lo que no choca ni ejecuta nada.
func applyRevision(ctx context.Context, o Opts, rev api.Revision, out *Outcome) error {
	desired, noData, err := client.Pull(ctx, o.API, o.Acct, o.Store, o.Files, rev.Snapshot)
	if err != nil {
		return closeRev(ctx, o, out, api.RevFailed, "no se pudo bajar el snapshot deseado: "+err.Error())
	}
	out.Snapshot = desired.ID
	sinDatos := map[string]bool{}
	for _, l := range noData {
		sinDatos[l] = true
	}

	// La base es «el último aplicado sobre el que van esos cambios». Sin ella
	// la orden es absoluta («llega a este snapshot»), y se reconcilia pasando
	// lo local como base: entonces nada choca y lo deseado gana, que es lo que
	// significa restaurar.
	var base []snapshot.Item
	if rev.Base != "" {
		bm, err := manifestFor(ctx, o, rev.Base)
		if err != nil {
			return closeRev(ctx, o, out, api.RevFailed,
				"no se pudo leer la base "+rev.Base+" de la revisión: "+err.Error())
		}
		base = bm.Items
	}

	withState := hasState(desired.Items) || hasState(base)
	local, err := core.SnapshotLive(o.Home, o.Src, core.SnapshotSourceOpts{WithState: withState})
	if err != nil {
		return closeRev(ctx, o, out, api.RevFailed, "no se pudo leer la configuración de esta máquina: "+err.Error())
	}
	if rev.Base == "" {
		base = local
	}

	var (
		auto    []string
		pending []Pending
		review  = Review{Revision: rev.ID, Cloud: rev.Snapshot, Snapshot: desired.ID, Created: o.now()}
	)
	for _, d := range Merge(base, local, desired.Items) {
		switch d.Action {
		case ActionConflict:
			review.Conflicts = append(review.Conflicts, d)
		case ActionKeep:
			// Deriva local que la orden no toca: no se escribe ni se cuenta
			// como pendiente. El portal la verá en el próximo snapshot.
		case ActionRemove:
			// El motor de restauración no borra nada (§8.3): decirlo es más
			// honesto que fingir que la revisión se aplicó entera.
			out.Skipped = append(out.Skipped, Skipped{LPath: d.LPath, Reason: "ccp no borra archivos al restaurar"})
		case ActionTake:
			if sinDatos[d.LPath] {
				out.Skipped = append(out.Skipped, Skipped{LPath: d.LPath, Reason: "sus datos no están en la nube"})
				continue
			}
			why := dangersOf(o, d)
			if len(why) == 0 && o.Policy != PolicyManual {
				auto = append(auto, d.LPath)
				continue
			}
			if o.Policy == PolicyManual && len(why) == 0 {
				why = nil // en manual se confirma todo, sin inventarle un motivo
			}
			pending = append(pending, Pending{LPath: d.LPath, Why: why})
		}
	}

	if len(auto) > 0 {
		rep, err := core.SnapshotRestore(o.Home, o.Src, o.Store, desired.ID, core.SnapshotRestoreOpts{
			Only: auto, Now: o.now(), Machine: o.Machine})
		if err != nil {
			return closeRev(ctx, o, out, api.RevFailed, "falló al aplicar: "+err.Error())
		}
		out.PreSnapshot = rep.PreSnapshot
		out.Applied, out.Skipped = tally(rep, out.Skipped)
	}
	out.Conflicts = lpaths(review.Conflicts)
	out.Pending = pending
	review.Pending, review.Applied, review.Skipped = pending, out.Applied, out.Skipped

	// Si algo espera a una persona, la revisión se queda abierta: cerrarla
	// ahora le quitaría a esa persona la posibilidad de contar el resultado.
	if len(pending) > 0 {
		out.Waiting = true
		return saveReview(o.Files, review)
	}
	if err := ClearReview(o.Files); err != nil {
		return err
	}
	state, reason := summarize(out)
	return closeRev(ctx, o, out, state, reason)
}

// manifestFor devuelve el manifiesto local de un snapshot de la nube. Si esta
// máquina no lo tiene se baja: la base puede venir de otro equipo y sin ella
// no hay tres bandas, solo un restore disfrazado.
func manifestFor(ctx context.Context, o Opts, cloudID string) (*snapshot.Manifest, error) {
	state, err := o.Files.LoadState()
	if err != nil {
		return nil, err
	}
	for localID, up := range state.Pushed {
		if up == cloudID {
			if m, err := o.Store.LoadManifest(localID); err == nil {
				return m, nil
			}
			break // está anotado pero ya no está en el almacén: se vuelve a bajar
		}
	}
	m, _, err := client.Pull(ctx, o.API, o.Acct, o.Store, o.Files, cloudID)
	return m, err
}

func hasState(items []snapshot.Item) bool {
	for _, it := range items {
		if it.Class == snapshot.ClassState {
			return true
		}
	}
	return false
}

// dangersOf clasifica un cambio leyendo los dos contenidos. Un blob que no se
// puede leer no se da por inocente: se pregunta.
func dangersOf(o Opts, d Decision) []Danger {
	var from, to []byte
	if d.Base != nil {
		from, _ = o.Store.GetBlob(d.Base.Hash)
	}
	if d.Desired != nil {
		var err error
		if to, err = o.Store.GetBlob(d.Desired.Hash); err != nil {
			return []Danger{DangerScript}
		}
	}
	return Dangers(*d.Desired, from, to)
}

// tally reparte los pasos del restore: lo escrito y lo que se saltó, con el
// motivo que dio el motor. Un paso «same» no se cuenta como aplicado porque no
// se escribió nada, pero tampoco como saltado: el destino ya estaba bien.
func tally(rep *core.SnapshotRestoreReport, skipped []Skipped) ([]string, []Skipped) {
	applied := []string{}
	for _, s := range rep.Steps {
		switch s.Action {
		case "write", "merge":
			applied = append(applied, s.LPath)
		case "skip":
			skipped = append(skipped, Skipped{LPath: s.LPath, Reason: s.Reason})
		}
	}
	sort.Strings(applied)
	return applied, skipped
}

// DefaultInterval es cada cuánto pregunta el agente. El servidor de F2 no
// tiene long-poll, así que se consulta; son unos pocos bytes cada vez y el
// coste de enterarse tarde es esperar un rato, no perder nada.
const DefaultInterval = 5 * time.Minute

// Loop pregunta cada `every` hasta que se cancela el contexto. report recibe
// cada pasada con algo que contar; ErrNothing no se informa, que es el caso
// normal. Un error NO para el bucle: la nube nunca bloquea a ccp (§10.5.3), y
// un agente que se muere con el primer corte de red deja de aplicar para
// siempre sin que nadie se entere.
func Loop(ctx context.Context, o Opts, every time.Duration, report func(*Outcome, error)) {
	if every <= 0 {
		every = DefaultInterval
	}
	for {
		out, err := Once(ctx, o)
		switch {
		case errors.Is(err, ErrNothing):
		case ctx.Err() != nil:
			return
		default:
			report(out, err)
		}
		t := time.NewTimer(every)
		select {
		case <-ctx.Done():
			t.Stop()
			return
		case <-t.C:
		}
	}
}
