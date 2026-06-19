package core

import (
	"fmt"
	"os"
	"path/filepath"

	yaml "github.com/goccy/go-yaml"
)

// handoff_store.go — persistencia de ~/.config/ccp/handoffs.yaml. Estado de
// runtime (NO config): vive aparte de ccp.yaml para no ensuciar su diff ni
// arriesgar su version. Escritura atómica tmp+rename bajo el mismo flock que
// store.go (acquireLock). Si el archivo falta o está corrupto, degradación
// suave: LoadHandoffs devuelve un Handoffs vacío.

// Marker es el handoff en vuelo (0 o 1 a la vez en v1).
type Marker struct {
	Session string `yaml:"session"`
	Slug    string `yaml:"slug"`
	Cwd     string `yaml:"cwd"`
	From    string `yaml:"from"`
	To      string `yaml:"to"`
	Title   string `yaml:"title,omitempty"`
	Since   string `yaml:"since"`
}

// ArchivedMarker es un handoff terminado (historial para `handoff list`).
type ArchivedMarker struct {
	Session    string `yaml:"session"`
	From       string `yaml:"from"`
	To         string `yaml:"to"`
	Slug       string `yaml:"slug"`
	ReturnedAs string `yaml:"returned_as"`
	Since      string `yaml:"since"`
	Ended      string `yaml:"ended"`
}

// Handoffs es el modelo en memoria de handoffs.yaml.
type Handoffs struct {
	Version  int              `yaml:"version"`
	Active   *Marker          `yaml:"active,omitempty"`
	Archived []ArchivedMarker `yaml:"archived,omitempty"`
}

func handoffsPath(home string) string { return filepath.Join(home, "handoffs.yaml") }

// LoadHandoffs lee handoffs.yaml. Ausente o corrupto => Handoffs vacío v1 (sin
// error: degradación suave; el rastro se pierde pero los jsonl siguen en disco).
func LoadHandoffs(home string) (*Handoffs, error) {
	data, err := os.ReadFile(handoffsPath(home))
	if err != nil {
		return &Handoffs{Version: 1}, nil
	}
	h := &Handoffs{}
	if err := yaml.Unmarshal(data, h); err != nil {
		return &Handoffs{Version: 1}, nil
	}
	if h.Version == 0 {
		h.Version = 1
	}
	return h, nil
}

// SaveHandoffs escribe handoffs.yaml atómicamente bajo flock (reusa acquireLock).
func SaveHandoffs(home string, h *Handoffs) error {
	if h.Version == 0 {
		h.Version = 1
	}
	if err := os.MkdirAll(home, 0o755); err != nil {
		return fmt.Errorf("no se pudo crear %s: %w", home, err)
	}
	out, err := yaml.Marshal(h)
	if err != nil {
		return fmt.Errorf("no se pudo serializar handoffs.yaml: %w", err)
	}
	unlock, err := acquireLock(home)
	if err != nil {
		return err
	}
	defer unlock()

	path := handoffsPath(home)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, out, 0o644); err != nil {
		return fmt.Errorf("no se pudo escribir %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("no se pudo renombrar %s -> %s: %w", tmp, path, err)
	}
	return nil
}
