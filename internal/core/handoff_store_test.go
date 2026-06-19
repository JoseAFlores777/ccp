package core

import (
	"testing"
)

func TestHandoffsRoundTrip(t *testing.T) {
	home := t.TempDir()
	h, err := LoadHandoffs(home)
	if err != nil {
		t.Fatal(err)
	}
	if h.Version != 1 || h.Active != nil || len(h.Archived) != 0 {
		t.Fatalf("vacío esperado, got %+v", h)
	}
	h.Active = &Marker{
		Session: "abc", Slug: "-r", Cwd: "/r",
		From: "personal-cc", To: "emco-cc", Title: "T", Since: "2026-06-19T00:00:00Z",
	}
	if err := SaveHandoffs(home, h); err != nil {
		t.Fatal(err)
	}
	h2, err := LoadHandoffs(home)
	if err != nil {
		t.Fatal(err)
	}
	if h2.Active == nil || h2.Active.To != "emco-cc" || h2.Active.Session != "abc" {
		t.Fatalf("no round-tripeó: %+v", h2.Active)
	}
}

func TestHandoffsMissingIsEmpty(t *testing.T) {
	h, err := LoadHandoffs(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if h.Active != nil {
		t.Fatal("archivo ausente debe dar Active nil")
	}
}
