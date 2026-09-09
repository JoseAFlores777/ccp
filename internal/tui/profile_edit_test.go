package tui

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/JoseAFlores777/ccp/internal/core"
	"github.com/JoseAFlores777/ccp/internal/core/i18n"
)

// TestProfileEditCedeLaTerminal fija la única cosa que hace que la tecla 'e' del
// panel Perfiles se vea: ceder la terminal con tea.Exec. Con el patrón viejo
// —tea.Sequence(tea.ExitAltScreen, …)— la pantalla alternativa se va pero el
// renderer de bubbletea sigue vivo repintando y su lector sigue comiéndose
// stdin, así que nano arranca debajo del dashboard y el usuario solo ve un
// parpadeo. Es el mismo motivo documentado en configEditExec (config_view.go).
func TestProfileEditCedeLaTerminal(t *testing.T) {
	home := seedConfigHome(t, nil, nil)
	m := &model{home: home, focus: panelProfiles, width: 100, height: 40}
	m.reload()

	_, cmd := m.editConfig("a-cc")
	if cmd == nil {
		t.Fatal("editConfig no devolvió comando")
	}
	// tea.execMsg es el mensaje interno que emite tea.Exec; tea.Sequence emite
	// tea.sequenceMsg. Comparar el tipo es lo único que distingue "cedió la
	// terminal" de "salió del alt-screen y siguió pintando".
	if got := fmt.Sprintf("%T", cmd()); got != "tea.execMsg" {
		t.Fatalf("editConfig no cede la terminal a bubbletea: %s", got)
	}
}

// TestProfileEditExecUsaElStdioCedido: el editor tiene que colgarse del stdio
// que bubbletea acaba de liberar, no de os.Stdin/os.Stdout (core.LaunchEditor
// usa esos directamente), y Run() nunca devuelve error —un editor que falla es
// una línea de estado, no una terminal rota—.
func TestProfileEditExecUsaElStdioCedido(t *testing.T) {
	home := seedConfigHome(t, nil, nil)
	cfg, err := core.Load(home)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	cfg.Defaults.Editor = "echo" // bloquea, sale 0, escribe por stdout
	if err := core.Save(home, cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}

	var out bytes.Buffer
	ex := &profileEditExec{home: home, name: "a-cc"}
	ex.SetStdin(strings.NewReader(""))
	ex.SetStdout(&out)
	ex.SetStderr(io.Discard)
	if err := ex.Run(); err != nil {
		t.Fatalf("Run devolvió error a bubbletea: %v", err)
	}
	if ex.err != nil {
		t.Fatalf("ProfileConfig falló: %v", ex.err)
	}
	if !strings.Contains(out.String(), "settings.overlay.json") {
		t.Fatalf("el editor no escribió en el stdio cedido: %q", out.String())
	}

	m := &model{home: home, focus: panelProfiles, width: 100, height: 40}
	m.reload()
	m.lang = i18n.Es
	m.finishProfileEdit(profileEditDoneMsg{ex: ex})
	if m.statusErr {
		t.Fatalf("una edición correcta no puede quedar en error: %q", m.statusMsg)
	}

	// Un editor inexistente no rompe bubbletea: sale por la línea de estado.
	bad := &profileEditExec{home: home, name: "a-cc"}
	bad.SetStdin(strings.NewReader(""))
	bad.SetStdout(io.Discard)
	bad.SetStderr(io.Discard)
	cfg.Defaults.Editor = "ccp-no-existe-jamas"
	if err := core.Save(home, cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := bad.Run(); err != nil {
		t.Fatalf("Run (editor inexistente) devolvió error a bubbletea: %v", err)
	}
	if bad.err == nil {
		t.Fatal("un editor inexistente tiene que fallar")
	}
	m.finishProfileEdit(profileEditDoneMsg{ex: bad})
	if !m.statusErr {
		t.Fatalf("el fallo del editor no se reporta: %q", m.statusMsg)
	}
}
