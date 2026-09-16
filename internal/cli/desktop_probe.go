package cli

import (
	"os/exec"
	"runtime"

	"github.com/JoseAFlores777/ccp/internal/core"
)

// desktop_probe.go — la única parte del diagnóstico que toca el sistema.
//
// Todo lo demás (parsear, clasificar, decidir) vive en core y es puro. Aquí solo
// se ejecuta `ps` y se entrega el texto. Separarlo así es lo que permite montar
// en un test el estado exacto del 2026-09-15 —la app principal corriendo con el
// data dir de un perfil— sin abrir una ventana.

// desktopPsCommand es la forma verificada de sacar el entorno de los procesos.
// Importa la combinación exacta: `-E` imprime el entorno, pero solo se ve con
// `-ww` (sin él, la línea se trunca a la anchura del terminal y el entorno,
// que va al final, es lo primero que se pierde).
var desktopPsCommand = []string{"ps", "-Ewww", "-axo", "pid=,command="}

// desktopProcesses devuelve los procesos de Claude Desktop vivos. Nunca falla
// hacia arriba: si `ps` no está o devuelve error, se devuelve nada y quien
// llama debe tratar eso como «no lo sé», no como «no hay ninguno». Esa
// distinción la mantiene el doctor marcando Unknown en vez de OK.
func desktopProcesses() []core.DesktopProc {
	if runtime.GOOS == "windows" {
		return nil
	}
	out, err := exec.Command(desktopPsCommand[0], desktopPsCommand[1:]...).Output()
	if err != nil {
		return nil
	}
	var claude []core.DesktopProc
	for _, p := range core.ParseDesktopProcs(string(out)) {
		if core.IsDesktopExec(p.Exec) {
			claude = append(claude, p)
		}
	}
	return claude
}

// desktopForeignInstance: ¿hay un Claude vivo que no sea el de este perfil? Es
// lo que decide si `open` necesita `-n` para no activar la ventana equivocada.
func desktopForeignInstance(profile, dataDir string) bool {
	if runtime.GOOS != "darwin" {
		return false
	}
	_ = profile
	return core.DesktopForeignInstance(desktopProcesses(), dataDir)
}
