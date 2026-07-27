package core

import (
	"os"
	"strings"
	"testing"
	"time"
)

// seedChainEnv crea un ccp.yaml con TRES perfiles official: el mínimo para
// probar una cadena real (primario + dos préstamos), que es lo que distingue
// este archivo del resto de tests de handoff.
func seedChainEnv(t *testing.T, home string) {
	t.Helper()
	cfg := &Config{
		Version: SchemaVersion,
		Profiles: map[string]Profile{
			"personal-cc": {Type: "official"},
			"emco-cc":     {Type: "official"},
			"kimi-cc":     {Type: "official"},
		},
	}
	if err := Save(home, cfg); err != nil {
		t.Fatal(err)
	}
}

func ccHomeOf(home, profile string) string {
	return home + "/profiles/" + profile + "/cc-home"
}

func TestHandoffChainPrimerHopSiembraMarcador(t *testing.T) {
	home := t.TempDir()
	seedChainEnv(t, home)
	cwd := "/repo/uno"
	slug := SlugForCwd(cwd)
	uuid := "11111111-1111-4111-8111-111111111111"
	writeJSONL(t, ProjectDir(ccHomeOf(home, "personal-cc"), slug), uuid, "Refactor", time.Now())

	now := time.Date(2026, 7, 25, 3, 0, 0, 0, time.UTC)
	m, err := HandoffChain(home, "personal-cc", "emco-cc", cwd, uuid, true, false, now)
	if err != nil {
		t.Fatal(err)
	}
	if m.From != "personal-cc" || m.To != "emco-cc" {
		t.Fatalf("marcador con perfiles incorrectos: %+v", m)
	}
	if !m.Auto {
		t.Fatalf("auto=true debe marcar el marcador: %+v", m)
	}
	if len(m.Hops) != 1 || m.Hops[0] != "emco-cc" {
		t.Fatalf("hops mal sembrado: %+v", m.Hops)
	}
	if m.Title != "Refactor" {
		t.Fatalf("título no leído del transcript: %q", m.Title)
	}
	if m.Since != "2026-07-25T03:00:00Z" {
		t.Fatalf("since inesperado: %q", m.Since)
	}
	// Copió el jsonl al destino con el mismo uuid.
	if _, err := os.Stat(ProjectDir(ccHomeOf(home, "emco-cc"), slug) + "/" + uuid + ".jsonl"); err != nil {
		t.Fatalf("no copió al destino: %v", err)
	}
	h, _ := LoadHandoffs(home)
	if len(h.Active) != 1 {
		t.Fatalf("esperaba 1 activo: %+v", h.Active)
	}
	if !h.Active[0].Auto || len(h.Active[0].Hops) != 1 {
		t.Fatalf("auto/hops no persistieron: %+v", h.Active[0])
	}
}

// El caso que HandoffForward rechaza con «handoff encadenado no soportado» es
// aquí el camino feliz: el marcador se muta en sitio, sin apilar un segundo nivel.
func TestHandoffChainSegundoHopMutaElMismoMarcador(t *testing.T) {
	home := t.TempDir()
	seedChainEnv(t, home)
	cwd := "/repo/uno"
	slug := SlugForCwd(cwd)
	uuid := "22222222-2222-4222-8222-222222222222"
	writeJSONL(t, ProjectDir(ccHomeOf(home, "personal-cc"), slug), uuid, "Largo", time.Now())

	t0 := time.Date(2026, 7, 25, 3, 0, 0, 0, time.UTC)
	if _, err := HandoffChain(home, "personal-cc", "emco-cc", cwd, uuid, true, false, t0); err != nil {
		t.Fatalf("primer hop: %v", err)
	}
	// El transcript creció en emco-cc mientras se usaba; el segundo hop copia ESE.
	writeJSONL(t, ProjectDir(ccHomeOf(home, "emco-cc"), slug), uuid, "Largo", time.Now())

	t1 := t0.Add(2 * time.Hour)
	m, err := HandoffChain(home, "emco-cc", "kimi-cc", cwd, uuid, true, false, t1)
	if err != nil {
		t.Fatalf("segundo hop debe permitirse (esto es lo que forward bloquea): %v", err)
	}
	if m.From != "personal-cc" {
		t.Fatalf("From debe seguir siendo el primario, got %q", m.From)
	}
	if m.To != "kimi-cc" {
		t.Fatalf("To debe apuntar al último destino, got %q", m.To)
	}
	if m.Since != "2026-07-25T03:00:00Z" {
		t.Fatalf("Since no debe cambiar en un hop encadenado: %q", m.Since)
	}
	if len(m.Hops) != 2 || m.Hops[0] != "emco-cc" || m.Hops[1] != "kimi-cc" {
		t.Fatalf("hops debe rastrear los dos saltos: %+v", m.Hops)
	}
	h, _ := LoadHandoffs(home)
	if len(h.Active) != 1 {
		t.Fatalf("el encadenado NO debe apilar un segundo marcador: %+v", h.Active)
	}
	if h.Active[0].From != "personal-cc" || h.Active[0].To != "kimi-cc" {
		t.Fatalf("marcador persistido incorrecto: %+v", h.Active[0])
	}
	if _, err := os.Stat(ProjectDir(ccHomeOf(home, "kimi-cc"), slug) + "/" + uuid + ".jsonl"); err != nil {
		t.Fatalf("no copió al tercer perfil: %v", err)
	}
}

// La invariante que SÍ sobrevive al lift: la misma sesión no puede prestarse
// desde un perfil que no es quien la tiene ahora mismo.
func TestHandoffChainFanOutSigueProhibido(t *testing.T) {
	home := t.TempDir()
	seedChainEnv(t, home)
	cwd := "/repo/uno"
	slug := SlugForCwd(cwd)
	uuid := "33333333-3333-4333-8333-333333333333"
	// Activo personal-cc → emco-cc: quien tiene la sesión es emco-cc.
	if err := SaveHandoffs(home, &Handoffs{Version: HandoffsVersion, Active: []Marker{{
		Session: uuid, Slug: slug, Cwd: cwd, From: "personal-cc", To: "emco-cc",
		Since: "2026-07-25T00:00:00Z",
	}}}); err != nil {
		t.Fatal(err)
	}
	// El jsonl sigue en el primario (el forward no lo borra): un fan-out desde
	// personal-cc sería técnicamente posible, y es justo lo que hay que impedir.
	writeJSONL(t, ProjectDir(ccHomeOf(home, "personal-cc"), slug), uuid, "A", time.Now())

	_, err := HandoffChain(home, "personal-cc", "kimi-cc", cwd, uuid, true, false, time.Now())
	if err == nil {
		t.Fatal("esperaba error de fan-out: la sesión la tiene emco-cc, no personal-cc")
	}
	if !strings.Contains(err.Error(), "ya está en vuelo") {
		t.Fatalf("mensaje inesperado: %v", err)
	}
	// Ni marcador nuevo ni transcript copiado al tercer perfil.
	h, _ := LoadHandoffs(home)
	if len(h.Active) != 1 || h.Active[0].To != "emco-cc" {
		t.Fatalf("el fan-out rechazado no debe tocar el estado: %+v", h.Active)
	}
	if _, err := os.Stat(ProjectDir(ccHomeOf(home, "kimi-cc"), slug)); err == nil {
		t.Fatal("el fan-out rechazado copió al tercer perfil")
	}
}

func TestHandoffChainRechazos(t *testing.T) {
	cwd := "/repo/uno"
	uuid := "44444444-4444-4444-8444-444444444444"
	cases := []struct {
		name       string
		from, to   string
		session    string
		wantSubstr string
	}{
		{"mismo perfil", "emco-cc", "emco-cc", uuid, "el mismo que el origen"},
		{"destino desconocido", "personal-cc", "no-existe", uuid, "perfil destino desconocido"},
		{"origen desconocido", "no-existe", "emco-cc", uuid, "perfil origen desconocido"},
		{"uuid inválido", "personal-cc", "emco-cc", "../../x", "id de sesión inválido"},
		{"sesión ausente", "personal-cc", "emco-cc", "55555555-5555-4555-8555-555555555555", "no encuentro la sesión"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			seedChainEnv(t, home)
			writeJSONL(t, ProjectDir(ccHomeOf(home, "personal-cc"), SlugForCwd(cwd)), uuid, "A", time.Now())
			_, err := HandoffChain(home, tc.from, tc.to, cwd, tc.session, true, false, time.Now())
			if err == nil {
				t.Fatalf("esperaba error %q", tc.wantSubstr)
			}
			if !strings.Contains(err.Error(), tc.wantSubstr) {
				t.Fatalf("mensaje inesperado: %v", err)
			}
			h, _ := LoadHandoffs(home)
			if len(h.Active) != 0 {
				t.Fatalf("un rechazo no debe dejar marcador: %+v", h.Active)
			}
		})
	}
}

// El gate de versión tiene que llegar ANTES de CopyTranscript: si no, el jsonl
// queda en el perfil destino sin marcador que lo referencie.
func TestHandoffChainVersionFuturaNoCopia(t *testing.T) {
	home := t.TempDir()
	seedChainEnv(t, home)
	cwd := "/repo/uno"
	slug := SlugForCwd(cwd)
	uuid := "66666666-6666-4666-8666-666666666666"
	writeJSONL(t, ProjectDir(ccHomeOf(home, "personal-cc"), slug), uuid, "A", time.Now())
	writeFutureHandoffs(t, home)

	_, err := HandoffChain(home, "personal-cc", "emco-cc", cwd, uuid, true, false, time.Now())
	if err == nil {
		t.Fatal("con versión futura debe abortar")
	}
	if !strings.Contains(err.Error(), "versión más nueva") {
		t.Fatalf("mensaje inesperado: %v", err)
	}
	dst := ProjectDir(ccHomeOf(home, "emco-cc"), slug)
	if _, err := os.Stat(dst); err == nil {
		t.Fatalf("el chain abortado creó %s", dst)
	}
	data, err := os.ReadFile(home + "/handoffs.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "version: 99") {
		t.Fatalf("handoffs.yaml fue pisado: %s", data)
	}
}

// Lo que justifica mutar en sitio en vez de apilar niveles: tras dos saltos, un
// solo `end` devuelve la conversación al primario ORIGINAL.
func TestHandoffEndSessionTrasCadenaVuelveAlPrimario(t *testing.T) {
	home := t.TempDir()
	seedChainEnv(t, home)
	cwd := "/repo/uno"
	slug := SlugForCwd(cwd)
	uuid := "77777777-7777-4777-8777-777777777777"
	writeJSONL(t, ProjectDir(ccHomeOf(home, "personal-cc"), slug), uuid, "Sesión", time.Now())

	t0 := time.Date(2026, 7, 25, 3, 0, 0, 0, time.UTC)
	if _, err := HandoffChain(home, "personal-cc", "emco-cc", cwd, uuid, true, false, t0); err != nil {
		t.Fatal(err)
	}
	writeJSONL(t, ProjectDir(ccHomeOf(home, "emco-cc"), slug), uuid, "Sesión", time.Now())
	if _, err := HandoffChain(home, "emco-cc", "kimi-cc", cwd, uuid, true, false, t0.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}

	m, newID, err := HandoffEndSession(home, cwd, "", t0.Add(2*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if m.From != "personal-cc" || m.To != "kimi-cc" {
		t.Fatalf("marcador cerrado inesperado: %+v", m)
	}
	if newID == "" || newID == uuid {
		t.Fatalf("uuid de vuelta inválido: %q", newID)
	}
	// La sesión de vuelta aterriza en el PRIMARIO, no en el perfil intermedio.
	back := ProjectDir(ccHomeOf(home, "personal-cc"), slug) + "/" + newID + ".jsonl"
	data, err := os.ReadFile(back)
	if err != nil {
		t.Fatalf("no creó la sesión de vuelta en el primario: %v", err)
	}
	if !strings.Contains(string(data), "[de kimi-cc] Sesión") {
		t.Fatalf("el título no marca el último perfil: %s", data)
	}
	h, _ := LoadHandoffs(home)
	if len(h.Active) != 0 || len(h.Archived) != 1 {
		t.Fatalf("un solo end debe cerrar toda la cadena: %+v", h)
	}
	if h.Archived[0].ReturnedAs != newID || h.Archived[0].From != "personal-cc" {
		t.Fatalf("archivado incorrecto: %+v", h.Archived[0])
	}
}

func TestHandoffPrune(t *testing.T) {
	// Historial en orden de inserción: a0 es el más viejo, a4 el más nuevo.
	seed := func(t *testing.T) string {
		t.Helper()
		home := t.TempDir()
		seedChainEnv(t, home)
		var arch []ArchivedMarker
		for i := 0; i < 5; i++ {
			arch = append(arch, ArchivedMarker{
				Session: string(rune('a'+i)) + "-sess", From: "personal-cc", To: "emco-cc",
				Slug: "-repo", ReturnedAs: "r", Since: "2026-07-0" + string(rune('1'+i)) + "T00:00:00Z",
				Ended: "2026-07-0" + string(rune('1'+i)) + "T01:00:00Z",
			})
		}
		if err := SaveHandoffs(home, &Handoffs{Version: HandoffsVersion, Archived: arch}); err != nil {
			t.Fatal(err)
		}
		return home
	}

	cases := []struct {
		name        string
		keep        int
		wantRemoved int
		wantLeft    []string // sesiones que deben quedar, en orden
	}{
		{"keep 0 borra todo", 0, 5, nil},
		{"keep 1 deja la más reciente", 1, 4, []string{"e-sess"}},
		{"keep 3 deja las 3 más recientes", 3, 2, []string{"c-sess", "d-sess", "e-sess"}},
		{"keep == len no quita nada", 5, 0, []string{"a-sess", "b-sess", "c-sess", "d-sess", "e-sess"}},
		{"keep > len no quita nada", 99, 0, []string{"a-sess", "b-sess", "c-sess", "d-sess", "e-sess"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			home := seed(t)
			removed, err := HandoffPrune(home, tc.keep)
			if err != nil {
				t.Fatal(err)
			}
			if removed != tc.wantRemoved {
				t.Fatalf("removed = %d, want %d", removed, tc.wantRemoved)
			}
			h, _ := LoadHandoffs(home)
			if len(h.Archived) != len(tc.wantLeft) {
				t.Fatalf("quedaron %d entradas, want %d: %+v", len(h.Archived), len(tc.wantLeft), h.Archived)
			}
			for i, want := range tc.wantLeft {
				if h.Archived[i].Session != want {
					t.Fatalf("archived[%d] = %q, want %q", i, h.Archived[i].Session, want)
				}
			}
		})
	}

	t.Run("keep negativo es error", func(t *testing.T) {
		home := seed(t)
		if _, err := HandoffPrune(home, -1); err == nil {
			t.Fatal("esperaba error con keep negativo")
		}
		h, _ := LoadHandoffs(home)
		if len(h.Archived) != 5 {
			t.Fatalf("un prune rechazado no debe tocar el historial: %+v", h.Archived)
		}
	})

	t.Run("no toca los activos", func(t *testing.T) {
		home := t.TempDir()
		seedChainEnv(t, home)
		if err := SaveHandoffs(home, &Handoffs{Version: HandoffsVersion,
			Active:   []Marker{{Session: "vivo", Slug: "-repo", Cwd: "/repo", From: "personal-cc", To: "emco-cc", Since: "2026-07-25T00:00:00Z"}},
			Archived: []ArchivedMarker{{Session: "viejo"}},
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := HandoffPrune(home, 0); err != nil {
			t.Fatal(err)
		}
		h, _ := LoadHandoffs(home)
		if len(h.Active) != 1 || len(h.Archived) != 0 {
			t.Fatalf("prune debe recortar solo el historial: %+v", h)
		}
	})

	t.Run("versión futura aborta", func(t *testing.T) {
		home := t.TempDir()
		seedChainEnv(t, home)
		writeFutureHandoffs(t, home)
		if _, err := HandoffPrune(home, 0); err == nil {
			t.Fatal("con versión futura debe abortar, no reportar 0 quitadas")
		}
	})
}
