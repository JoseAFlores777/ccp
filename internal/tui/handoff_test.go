package tui

import (
	"strings"
	"testing"

	"github.com/JoseAFlores777/ccp/internal/core"
	"github.com/JoseAFlores777/ccp/internal/core/i18n"
)

func TestHandoffProfileOptionsExcludesActive(t *testing.T) {
	cfg := &core.Config{Profiles: map[string]core.Profile{
		"personal-cc": {Type: "official"}, "emco-cc": {Type: "official"},
	}}
	opts := HandoffProfileOptions(cfg, "personal-cc")
	for _, o := range opts {
		if o == "personal-cc" {
			t.Fatal("el perfil activo no debe aparecer como destino")
		}
	}
	if len(opts) != 1 || opts[0] != "emco-cc" {
		t.Fatalf("opciones = %v, want [emco-cc]", opts)
	}
}

func TestMarkerLabel(t *testing.T) {
	m := core.Marker{
		Session: "bbc1ed61-ada1-408f-0000-000000000000",
		Cwd:     "/repo/uno", From: "personal-cc", To: "emco-cc",
		Title: "Refactor handoff", Since: "2026-07-25T14:30:00Z",
	}
	got := markerLabel(m, i18n.Es)
	for _, want := range []string{"personal-cc", "emco-cc", "bbc1ed61", "Refactor handoff"} {
		if !strings.Contains(got, want) {
			t.Errorf("label sin %q: %s", want, got)
		}
	}
}

// TestHandoffTitleBilingue: el placeholder de "sin título" sale traducido.
func TestHandoffTitleBilingue(t *testing.T) {
	if got := handoffTitle("", i18n.Es); got != "(sin título)" {
		t.Errorf("es: %q", got)
	}
	if got := handoffTitle("", i18n.En); got != "(untitled)" {
		t.Errorf("en: %q", got)
	}
	if got := handoffTitle("T", i18n.En); got != "T" {
		t.Errorf("con título: %q", got)
	}
}

func TestSessionLabelMarcaEnVuelo(t *testing.T) {
	inFlight := map[string]string{"aaa": "emco-cc"}
	got := sessionLabel(core.SessionInfo{UUID: "aaa", Title: "T"}, inFlight, i18n.Es)
	if !strings.Contains(got, "en vuelo → emco-cc") {
		t.Errorf("la sesión prestada debe marcarse: %s", got)
	}
	if en := sessionLabel(core.SessionInfo{UUID: "aaa", Title: "T"}, inFlight, i18n.En); !strings.Contains(en, "in flight → emco-cc") {
		t.Errorf("la marca debe traducirse: %s", en)
	}
	got2 := sessionLabel(core.SessionInfo{UUID: "bbb", Title: "T"}, inFlight, i18n.Es)
	if strings.Contains(got2, "en vuelo") {
		t.Errorf("la sesión libre no debe marcarse: %s", got2)
	}
}

// TestSessionOptionsOfreceLasEnVueloMarcadas es la regresión del hallazgo #1/#16:
// el picker OCULTABA (continue) las sesiones ya prestadas, así que el usuario no
// veía por qué faltaba la suya. Ahora se ofrecen TODAS, marcadas, y es
// checkSessionFree quien rechaza la que no se puede elegir. Este test recorre
// las dos funciones que usa RunHandoffSessionPicker, no una réplica del bucle.
func TestSessionOptionsOfreceLasEnVueloMarcadas(t *testing.T) {
	sess := []core.SessionInfo{
		{UUID: "11111111-0000-0000-0000-000000000000", Title: "T-11"},
		{UUID: "22222222-0000-0000-0000-000000000000", Title: "T-22"},
	}
	inFlight := map[string]string{"11111111-0000-0000-0000-000000000000": "emco-cc"}

	opts := sessionOptions(sess, inFlight, i18n.Es)
	if len(opts) != len(sess) {
		t.Fatalf("el picker debe ofrecer las %d sesiones (ofrece %d)", len(sess), len(opts))
	}
	if !strings.Contains(opts[0].Key, "en vuelo → emco-cc") {
		t.Errorf("la sesión prestada debe aparecer marcada: %q", opts[0].Key)
	}
	if strings.Contains(opts[1].Key, "en vuelo") {
		t.Errorf("la sesión libre no debe marcarse: %q", opts[1].Key)
	}
	if opts[0].Value != sess[0].UUID {
		t.Errorf("el valor de la opción debe ser el uuid: %q", opts[0].Value)
	}
}

// TestCheckSessionFreeRechazaEnVuelo cubre el otro extremo del hallazgo #1/#16:
// elegir una sesión marcada falla con un error que nombra el perfil que la
// tiene y el remedio, en vez de dejar que el core la rechace más tarde.
func TestCheckSessionFreeRechazaEnVuelo(t *testing.T) {
	inFlight := map[string]string{"aaa": "emco-cc"}
	err := checkSessionFree("aaa", inFlight, i18n.Es)
	if err == nil {
		t.Fatal("una sesión en vuelo no debe poder elegirse")
	}
	for _, want := range []string{"emco-cc", "ccp handoff resume"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("el error debe mencionar %q: %v", want, err)
		}
	}
	if err := checkSessionFree("bbb", inFlight, i18n.Es); err != nil {
		t.Errorf("una sesión libre debe aceptarse: %v", err)
	}
	if err := checkSessionFree("aaa", inFlight, i18n.En); err == nil || !strings.Contains(err.Error(), "in flight") {
		t.Errorf("el error debe traducirse: %v", err)
	}
}

func TestInFlightSessions(t *testing.T) {
	h := &core.Handoffs{Version: core.HandoffsVersion, Active: []core.Marker{
		{Session: "aaa", To: "emco-cc"}, {Session: "bbb", To: "kimi"},
	}}
	got := inFlightSessions(h)
	if got["aaa"] != "emco-cc" || got["bbb"] != "kimi" || len(got) != 2 {
		t.Fatalf("inFlightSessions = %v", got)
	}
}
