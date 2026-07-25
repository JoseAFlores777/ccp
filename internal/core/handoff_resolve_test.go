package core

import (
	"errors"
	"strings"
	"testing"
)

func mk(session, cwd, from, to string) Marker {
	return Marker{Session: session, Slug: SlugForCwd(cwd), Cwd: cwd, From: from, To: to, Since: "2026-07-25T00:00:00Z"}
}

func TestResolveActiveByCwdUnico(t *testing.T) {
	h := &Handoffs{Version: HandoffsVersion, Active: []Marker{
		mk("aaa", "/repo/uno", "personal-cc", "emco-cc"),
		mk("bbb", "/repo/dos", "personal-cc", "kimi"),
	}}
	idx, cands, err := ResolveActive(h, "/repo/uno", "")
	if err != nil {
		t.Fatalf("no esperaba error: %v", err)
	}
	if idx != 0 || len(cands) != 0 {
		t.Fatalf("esperaba idx 0 sin candidatos, got idx=%d cands=%v", idx, cands)
	}
}

func TestResolveActivePorSessionFlag(t *testing.T) {
	h := &Handoffs{Version: HandoffsVersion, Active: []Marker{
		mk("aaa", "/repo/uno", "personal-cc", "emco-cc"),
		mk("bbb", "/repo/dos", "personal-cc", "kimi"),
	}}
	// El uuid gana aunque el cwd sea otro.
	idx, _, err := ResolveActive(h, "/repo/uno", "bbb")
	if err != nil {
		t.Fatal(err)
	}
	if idx != 1 {
		t.Fatalf("esperaba idx 1, got %d", idx)
	}
}

func TestResolveActiveSessionFlagDesconocida(t *testing.T) {
	h := &Handoffs{Version: HandoffsVersion, Active: []Marker{mk("aaa", "/repo/uno", "personal-cc", "emco-cc")}}
	_, _, err := ResolveActive(h, "/repo/uno", "zzz")
	if err == nil {
		t.Fatal("esperaba error con uuid desconocido")
	}
	if !strings.Contains(err.Error(), "aaa") {
		t.Fatalf("el error debe listar los activos: %v", err)
	}
}

func TestResolveActiveSinActivosEnEsteRepo(t *testing.T) {
	h := &Handoffs{Version: HandoffsVersion, Active: []Marker{mk("aaa", "/repo/uno", "personal-cc", "emco-cc")}}
	_, _, err := ResolveActive(h, "/otro/repo", "")
	if err == nil {
		t.Fatal("esperaba error: no hay activo para este cwd")
	}
	if !strings.Contains(err.Error(), "/repo/uno") {
		t.Fatalf("el error debe nombrar dónde sí hay activos: %v", err)
	}
}

func TestResolveActiveSinActivos(t *testing.T) {
	h := &Handoffs{Version: HandoffsVersion}
	_, _, err := ResolveActive(h, "/repo/uno", "")
	if err == nil {
		t.Fatal("esperaba error sin activos")
	}
	if !strings.Contains(err.Error(), "no hay ningún handoff activo") {
		t.Fatalf("mensaje inesperado: %v", err)
	}
	// ResolveActive la comparten end, resume y discard: el mensaje no puede
	// hablar de «terminar», que es solo lo que hace end.
	if strings.Contains(err.Error(), "terminar") {
		t.Fatalf("el mensaje de lista vacía no debe nombrar una operación concreta: %v", err)
	}
}

func TestResolveActiveAmbiguo(t *testing.T) {
	h := &Handoffs{Version: HandoffsVersion, Active: []Marker{
		mk("aaa", "/repo/uno", "personal-cc", "emco-cc"),
		mk("bbb", "/repo/uno", "personal-cc", "kimi"),
	}}
	idx, cands, err := ResolveActive(h, "/repo/uno", "")
	if !errors.Is(err, ErrAmbiguousHandoff) {
		t.Fatalf("esperaba ErrAmbiguousHandoff, got %v", err)
	}
	if idx != -1 || len(cands) != 2 {
		t.Fatalf("esperaba idx -1 y 2 candidatos, got idx=%d cands=%v", idx, cands)
	}
}

func TestFindActiveSession(t *testing.T) {
	h := &Handoffs{Version: HandoffsVersion, Active: []Marker{mk("aaa", "/r", "p", "e")}}
	if FindActiveSession(h, "aaa") != 0 {
		t.Fatal("debe hallar la sesión activa")
	}
	if FindActiveSession(h, "zzz") != -1 {
		t.Fatal("uuid ausente debe dar -1")
	}
}

func TestActiveForCwd(t *testing.T) {
	h := &Handoffs{Version: HandoffsVersion, Active: []Marker{
		mk("aaa", "/repo/uno", "p", "e"),
		mk("bbb", "/repo/dos", "p", "k"),
		mk("ccc", "/repo/uno", "p", "g"),
	}}
	got := ActiveForCwd(h, "/repo/uno")
	if len(got) != 2 || got[0] != 0 || got[1] != 2 {
		t.Fatalf("esperaba índices [0 2], got %v", got)
	}
}
