package snapshot

import (
	"errors"
	"io/fs"
	"reflect"
	"testing"
	"time"
)

func src(lpath string, class Class, data string) Source {
	return Source{LPath: lpath, Class: class, Mode: 0o644, Read: func() ([]byte, error) { return []byte(data), nil }}
}

func meta(at time.Time) Meta {
	return Meta{Created: at, Machine: "mac", CCPVersion: "2.18.0", Trigger: "manual"}
}

func TestCaptureChainsAndDedups(t *testing.T) {
	st := openTemp(t)
	srcs := []Source{src("claude/settings.json", ClassAuthored, `{"a":1}`), src("ccp/ccp.yaml", ClassAuthored, "version: 2\n")}

	m1, err := Capture(st, srcs, meta(t0), false)
	if err != nil {
		t.Fatalf("primera captura: %v", err)
	}
	if m1.Parent != "" || len(m1.Items) != 2 || m1.Items[0].LPath != "ccp/ccp.yaml" {
		t.Fatalf("m1 = %+v", m1)
	}

	same, err := Capture(st, srcs, meta(t0.Add(time.Hour)), false)
	if !errors.Is(err, ErrNoChanges) || same.ID != m1.ID {
		t.Fatalf("captura sin cambios = %v, %v; quiero el mismo snapshot y ErrNoChanges", same, err)
	}

	srcs[0] = src("claude/settings.json", ClassAuthored, `{"a":2}`)
	m2, err := Capture(st, srcs, meta(t0.Add(2*time.Hour)), false)
	if err != nil || m2.Parent != m1.ID {
		t.Fatalf("m2 = %+v, %v; quiero padre %s", m2, err, m1.ID)
	}

	forced, err := Capture(st, srcs, meta(t0.Add(3*time.Hour)), true)
	if err != nil || forced.ID == m2.ID || forced.Parent != m2.ID {
		t.Fatalf("captura forzada = %+v, %v", forced, err)
	}
}

// Un archivo que desaparece entre listarlo y leerlo no entra, y no rompe la captura.
func TestCaptureSkipsVanishedFiles(t *testing.T) {
	st := openTemp(t)
	gone := Source{LPath: "claude/agents/x.md", Class: ClassAuthored, Read: func() ([]byte, error) { return nil, fs.ErrNotExist }}
	m, err := Capture(st, []Source{src("ccp/ccp.yaml", ClassAuthored, "v"), gone}, meta(t0), false)
	if err != nil || len(m.Items) != 1 {
		t.Fatalf("m = %+v, %v", m, err)
	}
}

func TestCaptureRejectsBadSources(t *testing.T) {
	st := openTemp(t)
	bad := map[string][]Source{
		"repetida":   {src("a", ClassAuthored, "1"), src("a", ClassAuthored, "2")},
		"con ..":     {src("../a", ClassAuthored, "1")},
		"clase":      {src("a", "cache", "1")},
		"sin nada":   {},
		"otro fallo": {{LPath: "a", Class: ClassAuthored, Read: func() ([]byte, error) { return nil, errors.New("disco") }}},
	}
	for name, srcs := range bad {
		if _, err := Capture(st, srcs, meta(t0), false); err == nil {
			t.Errorf("%s: Capture sin error", name)
		}
	}
}

func TestItemsOfMatchesCapture(t *testing.T) {
	st := openTemp(t)
	srcs := []Source{src("b", ClassSecret, "sk"), src("a", ClassAuthored, "x")}
	m, _ := Capture(st, srcs, meta(t0), false)
	items, err := ItemsOf(srcs)
	if err != nil || !reflect.DeepEqual(items, m.Items) {
		t.Fatalf("ItemsOf = %+v, %v; quiero %+v", items, err, m.Items)
	}
}

func TestDiff(t *testing.T) {
	a := []Item{{LPath: "keep", Hash: "1"}, {LPath: "mod", Hash: "2"}, {LPath: "old", Hash: "3"}, {LPath: "perm", Hash: "4", Mode: 0o644}}
	b := []Item{{LPath: "keep", Hash: "1"}, {LPath: "mod", Hash: "9"}, {LPath: "new", Hash: "5"}, {LPath: "perm", Hash: "4", Mode: 0o600}}
	got := Diff(a, b)
	want := []struct {
		l string
		k ChangeKind
	}{{"mod", ChangeModified}, {"new", ChangeAdded}, {"old", ChangeRemoved}, {"perm", ChangeModified}}
	if len(got) != len(want) {
		t.Fatalf("Diff = %+v", got)
	}
	for i, w := range want {
		if got[i].LPath != w.l || got[i].Kind != w.k {
			t.Errorf("cambio %d = %s %s, quiero %s %s", i, got[i].Kind, got[i].LPath, w.k, w.l)
		}
	}
	if len(Diff(a, a)) != 0 {
		t.Fatal("Diff de algo consigo mismo no es vacío")
	}
}
