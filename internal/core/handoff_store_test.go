package core

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHandoffsRoundTrip(t *testing.T) {
	home := t.TempDir()
	h, err := LoadHandoffs(home)
	if err != nil {
		t.Fatal(err)
	}
	if h.Version != HandoffsVersion || len(h.Active) != 0 || len(h.Archived) != 0 {
		t.Fatalf("vacío esperado, got %+v", h)
	}
	h.Active = []Marker{
		{Session: "abc", Slug: "-r", Cwd: "/r", From: "personal-cc", To: "emco-cc", Title: "T", Since: "2026-07-25T00:00:00Z"},
		{Session: "def", Slug: "-s", Cwd: "/s", From: "personal-cc", To: "kimi", Title: "U", Since: "2026-07-25T01:00:00Z"},
	}
	if err := SaveHandoffs(home, h); err != nil {
		t.Fatal(err)
	}
	h2, err := LoadHandoffs(home)
	if err != nil {
		t.Fatal(err)
	}
	if len(h2.Active) != 2 {
		t.Fatalf("esperaba 2 activos, got %+v", h2.Active)
	}
	if h2.Active[0].To != "emco-cc" || h2.Active[1].To != "kimi" {
		t.Fatalf("no round-tripeó en orden: %+v", h2.Active)
	}
	if h2.Version != HandoffsVersion {
		t.Fatalf("version esperada %d, got %d", HandoffsVersion, h2.Version)
	}
}

func TestHandoffsMissingIsEmpty(t *testing.T) {
	h, err := LoadHandoffs(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(h.Active) != 0 {
		t.Fatal("archivo ausente debe dar Active vacío")
	}
}

// v1 escribía `active:` como un mapping único. LoadHandoffs debe elevarlo a
// lista de uno sin perder el archivado.
func TestHandoffsMigratesV1Mapping(t *testing.T) {
	home := t.TempDir()
	v1 := `version: 1
active:
  session: bbc1ed61
  slug: -repo
  cwd: /repo
  from: personal-cc
  to: emco-cc
  title: Refactor
  since: 2026-06-19T14:30:00Z
archived:
  - session: 9c2e0d4f
    from: personal-cc
    to: emco-cc
    slug: -repo
    returned_as: a1b2f0d3
    since: 2026-06-18T10:00:00Z
    ended: 2026-06-18T15:20:00Z
`
	if err := os.WriteFile(filepath.Join(home, "handoffs.yaml"), []byte(v1), 0o644); err != nil {
		t.Fatal(err)
	}
	h, err := LoadHandoffs(home)
	if err != nil {
		t.Fatal(err)
	}
	if len(h.Active) != 1 || h.Active[0].Session != "bbc1ed61" || h.Active[0].To != "emco-cc" {
		t.Fatalf("no elevó el marcador v1: %+v", h.Active)
	}
	if len(h.Archived) != 1 || h.Archived[0].ReturnedAs != "a1b2f0d3" {
		t.Fatalf("perdió el archivado: %+v", h.Archived)
	}
	if h.Version != HandoffsVersion {
		t.Fatalf("debe reportar v%d en memoria, got %d", HandoffsVersion, h.Version)
	}
	// Persistir deja el archivo ya en v2.
	if err := SaveHandoffs(home, h); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(home, "handoffs.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	h3, err := LoadHandoffs(home)
	if err != nil {
		t.Fatal(err)
	}
	if len(h3.Active) != 1 {
		t.Fatalf("tras Save no round-tripea v2: %s", data)
	}
}

// Un handoffs.yaml de una versión futura degrada suave (runtime, no config).
func TestHandoffsFutureVersionDegrades(t *testing.T) {
	home := t.TempDir()
	future := "version: 99\nactive: []\n"
	if err := os.WriteFile(filepath.Join(home, "handoffs.yaml"), []byte(future), 0o644); err != nil {
		t.Fatal(err)
	}
	h, err := LoadHandoffs(home)
	if err != nil {
		t.Fatal(err)
	}
	if len(h.Active) != 0 || h.Version != HandoffsVersion {
		t.Fatalf("versión futura debe degradar a vacío v%d: %+v", HandoffsVersion, h)
	}
}
