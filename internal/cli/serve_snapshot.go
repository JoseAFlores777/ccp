package cli

// serve_snapshot.go — los métodos `snapshot.*` de `ccp serve`. Mismo motor que
// `ccp snapshot`; devuelven datos, nunca prosa.

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/JoseAFlores777/ccp/internal/core"
	"github.com/JoseAFlores777/ccp/internal/snapshot"
)

func srvSnapshotStore(s *server) (*snapshot.Store, string, error) {
	st, err := core.OpenSnapshotStore(s.home)
	if err != nil {
		return nil, "", err
	}
	src, err := core.ClaudeSrc()
	if err != nil {
		return nil, "", err
	}
	return st, src, nil
}

func srvSnapshotList(s *server, _ json.RawMessage) (any, error) {
	st, _, err := srvSnapshotStore(s)
	if err != nil {
		return nil, err
	}
	ms, err := st.List()
	if err != nil {
		return nil, err
	}
	out := make([]snapSummary, 0, len(ms))
	for _, m := range ms {
		out = append(out, snapSummaryOf(m, false))
	}
	return out, nil
}

func srvSnapshotShow(s *server, raw json.RawMessage) (any, error) {
	p, err := params[struct {
		ID string `json:"id"`
	}](raw)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(p.ID) == "" {
		return nil, badParams("falta el id del snapshot")
	}
	st, _, err := srvSnapshotStore(s)
	if err != nil {
		return nil, err
	}
	m, err := st.LoadManifest(p.ID)
	if err != nil {
		return nil, err
	}
	return m, nil
}

func srvSnapshotDiff(s *server, raw json.RawMessage) (any, error) {
	p, err := params[struct {
		From string `json:"from"`
		To   string `json:"to"`
	}](raw)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(p.From) == "" {
		return nil, badParams("falta el snapshot de origen")
	}
	st, src, err := srvSnapshotStore(s)
	if err != nil {
		return nil, err
	}
	from, err := st.LoadManifest(p.From)
	if err != nil {
		return nil, err
	}
	var to []snapshot.Item
	if p.To != "" {
		m, err := st.LoadManifest(p.To)
		if err != nil {
			return nil, err
		}
		to = m.Items
	} else {
		withState := countClass(from, snapshot.ClassState) > 0
		if to, err = core.SnapshotLive(s.home, src, core.SnapshotSourceOpts{WithState: withState}); err != nil {
			return nil, err
		}
	}
	changes := snapshot.Diff(from.Items, to)
	if changes == nil {
		changes = []snapshot.Change{}
	}
	return changes, nil
}

func srvSnapshotCreate(s *server, raw json.RawMessage) (any, error) {
	p, err := params[struct {
		Label     string `json:"label"`
		WithState bool   `json:"with_state"`
	}](raw)
	if err != nil {
		return nil, err
	}
	st, src, err := srvSnapshotStore(s)
	if err != nil {
		return nil, err
	}
	m, err := core.SnapshotCapture(s.home, src, st, core.SnapshotCaptureOpts{
		Trigger: "manual", Label: p.Label, WithState: p.WithState,
		Force: p.Label != "", Now: time.Now(), Machine: snapMachine(),
	})
	unchanged := errors.Is(err, snapshot.ErrNoChanges)
	if err != nil && !unchanged {
		return nil, err
	}
	return snapSummaryOf(m, unchanged), nil
}

func srvSnapshotRestore(s *server, raw json.RawMessage) (any, error) {
	p, err := params[struct {
		ID     string   `json:"id"`
		Only   []string `json:"only"`
		DryRun bool     `json:"dry_run"`
	}](raw)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(p.ID) == "" {
		return nil, badParams("falta el id del snapshot")
	}
	st, src, err := srvSnapshotStore(s)
	if err != nil {
		return nil, err
	}
	return core.SnapshotRestore(s.home, src, st, p.ID, core.SnapshotRestoreOpts{
		Only: p.Only, DryRun: p.DryRun, Now: time.Now(), Machine: snapMachine(),
	})
}

func srvSnapshotPrune(s *server, raw json.RawMessage) (any, error) {
	p, err := params[struct {
		DryRun bool     `json:"dry_run"`
		IDs    []string `json:"ids"`
	}](raw)
	if err != nil {
		return nil, err
	}
	st, _, err := srvSnapshotStore(s)
	if err != nil {
		return nil, err
	}
	// `ids` ancla la poda al plan que la GUI enseñó: entre el dry-run y el
	// confirmar pudo nacer otro snapshot y la retención recalculada se llevaría
	// uno que nadie vio en un diálogo marcado como destructivo.
	rep, err := snapshot.PruneOnly(st, snapshot.DefaultPolicy, time.Now(), time.Hour, p.DryRun, p.IDs)
	if err != nil {
		return nil, err
	}
	deleted := rep.Deleted
	if deleted == nil {
		deleted = []string{}
	}
	return map[string]any{"deleted": deleted, "kept": rep.Kept, "blobs_deleted": rep.BlobsDeleted, "dry_run": p.DryRun}, nil
}

func srvSnapshotPin(s *server, raw json.RawMessage) (any, error) {
	p, err := params[struct {
		ID     string  `json:"id"`
		Pinned bool    `json:"pinned"`
		Label  *string `json:"label"`
	}](raw)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(p.ID) == "" {
		return nil, badParams("falta el id del snapshot")
	}
	st, _, err := srvSnapshotStore(s)
	if err != nil {
		return nil, err
	}
	m, err := st.SetPin(p.ID, p.Pinned, p.Label)
	if err != nil {
		return nil, err
	}
	return snapSummaryOf(m, false), nil
}

func srvSnapshotExport(s *server, raw json.RawMessage) (any, error) {
	p, err := params[struct {
		ID         string `json:"id"`
		Dest       string `json:"dest"`
		Passphrase string `json:"passphrase"`
	}](raw)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(p.ID) == "" || strings.TrimSpace(p.Dest) == "" {
		return nil, badParams("faltan el id y el archivo de destino")
	}
	if p.Passphrase != "" && len([]rune(p.Passphrase)) < snapMinPassphrase {
		return nil, badParams("la frase necesita al menos %d caracteres", snapMinPassphrase)
	}
	st, _, err := srvSnapshotStore(s)
	if err != nil {
		return nil, err
	}
	perm := os.FileMode(0o644)
	var pass []byte
	if p.Passphrase != "" {
		perm, pass = 0o600, []byte(p.Passphrase)
	}
	tmp := p.Dest + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, perm)
	if err != nil {
		return nil, err
	}
	m, err := snapshot.Export(st, p.ID, f, pass)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(tmp, p.Dest)
	}
	if err != nil {
		_ = os.Remove(tmp)
		return nil, err
	}
	return map[string]any{"id": m.ID, "dest": p.Dest, "with_secrets": pass != nil}, nil
}

func srvSnapshotImport(s *server, raw json.RawMessage) (any, error) {
	p, err := params[struct {
		Archive    string `json:"archive"`
		Passphrase string `json:"passphrase"`
	}](raw)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(p.Archive) == "" {
		return nil, badParams("falta el archivo .ccpsnap")
	}
	st, _, err := srvSnapshotStore(s)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(p.Archive)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	rep, err := snapshot.Import(st, f, func() ([]byte, error) {
		if p.Passphrase == "" {
			return nil, fmt.Errorf("este archivo trae secretos sellados: hace falta la frase")
		}
		return []byte(p.Passphrase), nil
	})
	if err != nil {
		return nil, err
	}
	missing := rep.Missing
	if missing == nil {
		missing = []string{}
	}
	return map[string]any{"snapshot": snapSummaryOf(rep.Manifest, false), "missing": missing}, nil
}
