package core

import "testing"

// TestDesktopMainPIDsIgnoraHelpers es la regla que hace correcto el reinicio:
// solo el proceso PRINCIPAL de esa instancia.
//
// Los helpers de Chromium cuelgan del principal, así que señalarlos no cierra la
// ventana —el principal los relanza, o se cae llevándose la sesión sin guardar—
// y solo el principal convierte un SIGTERM en un cierre ordenado. Se afirma por
// nombre porque en una máquina con una sola ventana abierta las dos versiones se
// comportan parecido: el fallo aparece cuando hay helpers, que es siempre, pero
// solo se nota cuando el reinicio deja el perfil a medias.
func TestDesktopMainPIDsIgnoraHelpers(t *testing.T) {
	procs := []DesktopProc{
		{PID: 10, DataDir: "/d/trabajo"},
		{PID: 11, DataDir: "/d/trabajo", Helper: true},
		{PID: 12, DataDir: "/d/trabajo", Helper: true},
		{PID: 20, DataDir: "/d/personal"},
		{PID: 30},
	}
	got := DesktopMainPIDs(procs, "/d/trabajo")
	if len(got) != 1 || got[0] != 10 {
		t.Fatalf("DesktopMainPIDs = %v; quería solo el principal [10]", got)
	}
}

// TestDesktopMainPIDsDataDirVacio: un perfil sin instancia preparada trae el
// data dir vacío. Tratarlo como «coincide con todo» señalaría cualquier Claude
// abierto, incluido el del usuario — que es el proceso que este comando jamás
// debe tocar por accidente.
func TestDesktopMainPIDsDataDirVacio(t *testing.T) {
	procs := []DesktopProc{{PID: 10, DataDir: ""}, {PID: 11, DataDir: "/d/x"}}
	if got := DesktopMainPIDs(procs, ""); got != nil {
		t.Fatalf("un data dir vacío no puede señalar a nadie: %v", got)
	}
	if got := DesktopMainPIDs(procs, "  "); got != nil {
		t.Fatalf("ni siquiera en blanco: %v", got)
	}
}

// TestDesktopMainPIDsOrdenYPidsInvalidos: varios principales sobre el mismo data
// dir salen ordenados (la salida de `ps` no lo está) y un pid imposible se
// descarta en vez de acabar en un os.FindProcess.
func TestDesktopMainPIDsOrdenYPidsInvalidos(t *testing.T) {
	procs := []DesktopProc{
		{PID: 42, DataDir: "/d/x"},
		{PID: 7, DataDir: "/d/x"},
		{PID: 0, DataDir: "/d/x"},
		{PID: -3, DataDir: "/d/x"},
	}
	got := DesktopMainPIDs(procs, "/d/x")
	if len(got) != 2 || got[0] != 7 || got[1] != 42 {
		t.Fatalf("DesktopMainPIDs = %v; quería [7 42]", got)
	}
}
