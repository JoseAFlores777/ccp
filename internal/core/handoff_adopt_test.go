package core

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

// handoff_adopt_test.go — HandoffAdoptHome, la vuelta a casa SIN marcador.
//
// El estado que cubre solo lo produce el supervisor: una rotación que ocurrió
// antes de que existiera el jsonl (nada que prestar ⇒ ningún marcador) y una
// conversación que nace DESPUÉS, ya dentro de ese préstamo. Volver de ahí con
// HandoffChain crearía un marcador ACTIVO INVERTIDO {From: préstamo, To:
// primario}; con HandoffEndSession reventaría por falta de marcador.

func TestHandoffAdoptHomeMueveLaConversacionSinCrearMarcador(t *testing.T) {
	home := t.TempDir()
	seedChainEnv(t, home)
	cwd := "/repo/uno"
	slug := SlugForCwd(cwd)
	uuid := "aaaaaaaa-3333-4333-8333-aaaaaaaaaaaa"
	// La conversación vive en el PRÉSTAMO (work-1): nació allí.
	writeJSONL(t, ProjectDir(ccHomeOf(home, "work-1"), slug), uuid, "Refactor", time.Now())

	newID, err := HandoffAdoptHome(home, "work-1", "personal-1", cwd, uuid)
	if err != nil {
		t.Fatal(err)
	}
	if newID == uuid || len(newID) != 36 {
		t.Fatalf("newID = %q, quería un uuid NUEVO", newID)
	}

	// Llegó a casa con el uuid nuevo y con el sessionId reescrito: si quedara el
	// viejo, `claude --resume` abriría una conversación que se cree de otro sitio.
	dst := ProjectDir(ccHomeOf(home, "personal-1"), slug) + "/" + newID + ".jsonl"
	data, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("no llegó a casa: %v", err)
	}
	if strings.Contains(string(data), uuid) {
		t.Fatalf("quedó el sessionId viejo en el destino: %q", string(data))
	}
	// El título dice de dónde viene, igual que en un `handoff end`.
	var titled bool
	for _, ln := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		var m map[string]any
		if json.Unmarshal([]byte(ln), &m) != nil {
			continue
		}
		if m["type"] == "ai-title" {
			if s, _ := m["aiTitle"].(string); strings.HasPrefix(s, "[de work-1] ") {
				titled = true
			}
		}
	}
	if !titled {
		t.Fatalf("el aiTitle no quedó prefijado con el origen: %q", string(data))
	}

	// No destructivo: el original sigue en el préstamo.
	if _, err := os.Stat(ProjectDir(ccHomeOf(home, "work-1"), slug) + "/" + uuid + ".jsonl"); err != nil {
		t.Fatalf("borró el transcript del origen: %v", err)
	}

	// Y lo esencial: handoffs.yaml queda INTACTO. Ni marcador activo (que nunca
	// se cerraría estando en casa) ni entrada en archived (no hubo préstamo que
	// historiar).
	h, err := LoadHandoffs(home)
	if err != nil {
		t.Fatal(err)
	}
	if len(h.Active) != 0 || len(h.Archived) != 0 {
		t.Fatalf("adoptar no puede tocar handoffs.yaml: %+v / %+v", h.Active, h.Archived)
	}
}

// Adoptar es EXCLUSIVO de «no hay marcador»: con uno vivo lo que toca es
// HandoffEndSession, que además archiva. Adoptar por encima dejaría la
// conversación duplicada y el marcador apuntando a una copia congelada.
func TestHandoffAdoptHomeRechazaSiHayMarcadorVivo(t *testing.T) {
	home := t.TempDir()
	seedChainEnv(t, home)
	cwd := "/repo/uno"
	slug := SlugForCwd(cwd)
	uuid := "bbbbbbbb-3333-4333-8333-bbbbbbbbbbbb"
	writeJSONL(t, ProjectDir(ccHomeOf(home, "personal-1"), slug), uuid, "Refactor", time.Now())

	if _, err := HandoffChain(home, "personal-1", "work-1", cwd, uuid, true, false, time.Now()); err != nil {
		t.Fatal(err)
	}
	_, err := HandoffAdoptHome(home, "work-1", "personal-1", cwd, uuid)
	if err == nil {
		t.Fatal("se esperaba rechazo: hay un handoff activo para esa sesión")
	}
	if !strings.Contains(err.Error(), "handoff activo") {
		t.Fatalf("error poco explicativo: %v", err)
	}
}

func TestHandoffAdoptHomeErrores(t *testing.T) {
	home := t.TempDir()
	seedChainEnv(t, home)
	cwd := "/repo/uno"
	uuid := "cccccccc-3333-4333-8333-cccccccccccc"

	if _, err := HandoffAdoptHome(home, "work-1", "work-1", cwd, uuid); err == nil {
		t.Error("origen == destino debe fallar")
	}
	if _, err := HandoffAdoptHome(home, "work-1", "no-existe", cwd, uuid); err == nil {
		t.Error("perfil desconocido debe fallar")
	}
	// El uuid se concatena para formar el path: un `..` leería un jsonl ajeno.
	if _, err := HandoffAdoptHome(home, "work-1", "personal-1", cwd, "../../etc/passwd"); err == nil {
		t.Error("un uuid con travesía debe rechazarse")
	}
	// Sin transcript no hay nada que adoptar, y decirlo importa: el supervisor
	// distingue ese caso ANTES de llamar (arranca de cero), así que llegar aquí
	// significa que algo desapareció entre medias.
	if _, err := HandoffAdoptHome(home, "work-1", "personal-1", cwd, uuid); err == nil {
		t.Error("sin transcript en el origen debe fallar")
	}
}
