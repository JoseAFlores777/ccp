package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JoseAFlores777/ccp/internal/core"
)

// seedE2E deja un CCP_HOME con 2 perfiles official y una sesión en cada repo,
// dentro del cc-home de personal-cc. El home es un t.TempDir() que YA existe,
// así que la auto-migración no dispara contra el ~/.config/dsctl real.
func seedE2E(t *testing.T) (home, repoA, repoB, uuidA, uuidB string) {
	t.Helper()
	home = t.TempDir()
	cfg := &core.Config{
		Version:  core.SchemaVersion,
		Profiles: map[string]core.Profile{"personal-cc": {Type: "official"}, "emco-cc": {Type: "official"}},
	}
	if err := core.Save(home, cfg); err != nil {
		t.Fatal(err)
	}
	repoA, repoB = "/repo/uno", "/repo/dos"
	uuidA = "11111111-1111-4111-8111-111111111111"
	uuidB = "22222222-2222-4222-8222-222222222222"
	cc := filepath.Join(home, "profiles", "personal-cc", "cc-home")
	for _, x := range []struct{ cwd, uuid string }{{repoA, uuidA}, {repoB, uuidB}} {
		dir := core.ProjectDir(cc, core.SlugForCwd(x.cwd))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		line := `{"sessionId":"` + x.uuid + `","cwd":"` + x.cwd + `","type":"user"}` + "\n" +
			`{"sessionId":"` + x.uuid + `","type":"ai-title","aiTitle":"Tarea"}` + "\n"
		if err := os.WriteFile(filepath.Join(dir, x.uuid+".jsonl"), []byte(line), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return home, repoA, repoB, uuidA, uuidB
}

// TestE2EDosHandoffsResumeYEndSelectivo ejercita el binario entero (Dispatch)
// sobre el flujo multi-activo: dos forwards en repos distintos, resume del
// primero, end del segundo, y comprobación de que ni el marcador superviviente
// ni el estado se tocan cuando un end falla.
func TestE2EDosHandoffsResumeYEndSelectivo(t *testing.T) {
	home, repoA, repoB, uuidA, uuidB := seedE2E(t)
	t.Setenv("CCP_HOME", home)
	t.Setenv("CCP_PROFILE", "personal-cc")

	run := func(args ...string) (string, string, int) {
		var out, errb bytes.Buffer
		code := Dispatch(args, &out, &errb)
		return out.String(), errb.String(), code
	}

	// Forward A.
	out, errs, code := run("_handoff", repoA, "emco-cc", "--session", uuidA)
	if code != 0 {
		t.Fatalf("forward A falló: %s", errs)
	}
	if !strings.Contains(out, "CCP_RESUME_ID="+uuidA) {
		t.Fatalf("forward A no emitió el uuid: %s", out)
	}

	// Forward B en otro repo: v2 lo permite (v1 bloqueaba el segundo activo).
	if _, errs, code = run("_handoff", repoB, "emco-cc", "--session", uuidB); code != 0 {
		t.Fatalf("forward B debía permitirse: %s", errs)
	}
	h, _ := core.LoadHandoffs(home)
	if len(h.Active) != 2 {
		t.Fatalf("esperaba 2 activos: %+v", h.Active)
	}

	// Resume de A: emite el env del destino y no muta nada.
	out, errs, code = run("_handoff-resume", repoA)
	if code != 0 {
		t.Fatalf("resume A falló: %s", errs)
	}
	if !strings.Contains(out, "CCP_RESUME_ID="+uuidA) || !strings.Contains(out, "emco-cc") {
		t.Fatalf("resume A emitió mal: %s", out)
	}
	h, _ = core.LoadHandoffs(home)
	if len(h.Active) != 2 {
		t.Fatal("resume no debe archivar nada")
	}

	// End de B: solo B se archiva; A sigue vivo.
	if _, errs, code = run("_handoff-end", repoB); code != 0 {
		t.Fatalf("end B falló: %s", errs)
	}
	h, _ = core.LoadHandoffs(home)
	if len(h.Active) != 1 || h.Active[0].Session != uuidA {
		t.Fatalf("debía quedar A activo: %+v", h.Active)
	}
	if len(h.Archived) != 1 || h.Archived[0].Session != uuidB {
		t.Fatalf("archivó el equivocado: %+v", h.Archived)
	}

	// El back-sync dejó la sesión nueva en el origen (original + la de vuelta).
	origen := core.ProjectDir(filepath.Join(home, "profiles", "personal-cc", "cc-home"), core.SlugForCwd(repoB))
	entries, err := os.ReadDir(origen)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("esperaba original + sesión de vuelta en %s: %v", origen, entries)
	}
	if h.Archived[0].ReturnedAs == "" {
		t.Fatalf("el archivado debe registrar el uuid de vuelta: %+v", h.Archived[0])
	}
	if _, err := os.Stat(filepath.Join(origen, h.Archived[0].ReturnedAs+".jsonl")); err != nil {
		t.Fatalf("la sesión de vuelta no está en el origen: %v", err)
	}

	// End desde un repo sin handoff: error, sin tocar estado.
	if _, _, code = run("_handoff-end", "/repo/sin-handoff"); code == 0 {
		t.Fatal("end en repo sin handoff debe fallar")
	}
	h, _ = core.LoadHandoffs(home)
	if len(h.Active) != 1 || h.Active[0].Session != uuidA {
		t.Fatalf("un end fallido no debe mutar estado: %+v", h.Active)
	}
	if len(h.Archived) != 1 {
		t.Fatalf("un end fallido no debe archivar nada: %+v", h.Archived)
	}
}
