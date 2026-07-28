package core

// editor.go — la cadena de resolución del editor para `ccp config edit` y el
// lanzamiento del proceso.
//
// Por qué esto no es `ResolveEditor` con dos ifs más: `ResolveEditor` responde
// "¿con qué abro un archivo en esta terminal?" y su respuesta siempre bloquea
// (nano, vim). `config edit` quiere abrir el ccp.yaml en el editor GRÁFICO que
// el usuario ya tiene delante, y ahí la respuesta puede volver ANTES de que el
// usuario haya escrito nada. De ese matiz depende lo único que da valor real al
// comando —releer y validar el yaml al cerrar—, así que la resolución devuelve
// el comando JUNTO CON si bloquea, en vez de dejar que cada llamador lo deduzca
// del nombre del binario por su cuenta.

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// Orígenes posibles de la línea de editor. Son tokens de configuración (no
// prosa): se imprimen igual en ambos idiomas, como las claves de ccp.yaml en
// los mensajes de error.
const (
	EditorSourceFlag      = "--editor"
	EditorSourceGUIConfig = "defaults.gui_editor"
	EditorSourceVisual    = "$VISUAL"
	EditorSourcePath      = "PATH"
	EditorSourceOS        = "so"
	EditorSourceFallback  = "defaults.editor/$EDITOR"
)

// EditorChoice es el resultado de la cadena: qué se va a ejecutar, si esperará
// a que el usuario cierre, y de qué escalón salió (para poder decírselo).
type EditorChoice struct {
	Cmd      string // línea completa, tokenizable con strings.Fields
	Blocking bool
	Source   string
}

// EditorEnv son TODAS las entradas externas de la resolución. Existe para que
// la cadena sea una función pura: un test tiene que poder comprobar que
// `defaults.gui_editor` gana a `$VISUAL` en una máquina donde no hay ni VS Code
// ni `xdg-open` instalados.
type EditorEnv struct {
	Flag      string // --editor <cmd>
	GUIEditor string // defaults.gui_editor
	Visual    string // $VISUAL
	Terminal  bool   // --terminal: cortocircuita al último escalón
	Fallback  string // ResolveEditor(home), ya resuelto por el llamador
	GOOS      string // runtime.GOOS
	LookPath  func(string) (string, error)
}

// guiEditorCandidates es el orden de preferencia al mirar el PATH. `code`
// primero porque es el instalado por defecto; `cursor` antes que
// `code-insiders` porque quien tiene Cursor lo usa como editor principal,
// mientras que Insiders suele convivir con un `code` estable.
var guiEditorCandidates = []string{"code", "cursor", "code-insiders"}

// guiWaitFlags mapea lanzador GUI → banderas que lo hacen esperar al cierre.
// Un valor nil significa "este lanzador NO puede esperar" (xdg-open delega en
// el escritorio y retorna al instante; no hay bandera que lo evite).
//
// Este mapa es el ÚNICO sitio donde se decide si una línea de editor bloquea.
// Tenerlo repartido es cómo se acaba validando un ccp.yaml que el usuario
// todavía no ha guardado.
var guiWaitFlags = map[string][]string{
	"code":          {"-w", "--wait"},
	"code-insiders": {"-w", "--wait"},
	"codium":        {"-w", "--wait"},
	"cursor":        {"-w", "--wait"},
	"windsurf":      {"-w", "--wait"},
	"zed":           {"-w", "--wait"},
	"subl":          {"-w", "--wait"},
	"sublime_text":  {"-w", "--wait"},
	"open":          {"-W", "--wait-apps"},
	"xdg-open":      nil,
	"gnome-open":    nil,
	"kde-open":      nil,
	"start":         nil,
}

// EditorBlocks decide si una línea de editor espera a que el usuario cierre.
//
// La regla por defecto es "bloquea": los editores de terminal (nano, vim, nvim,
// emacs -nw, helix…) son innumerables y todos bloquean, así que enumerarlos
// sería una lista que envejece mal. Los que NO bloquean son pocos y conocidos
// —los lanzadores GUI—, y de esos solo importa si llevan su bandera de espera.
func EditorBlocks(line string) bool {
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return true
	}
	bin := strings.TrimSuffix(filepath.Base(fields[0]), ".exe")
	waits, isLauncher := guiWaitFlags[bin]
	if !isLauncher {
		return true
	}
	for _, arg := range fields[1:] {
		for _, w := range waits {
			if arg == w {
				return true
			}
		}
	}
	return false
}

// ResolveEditEditor recorre la cadena de `ccp config edit`. Gana el PRIMER
// escalón que exista:
//
//  1. --editor <cmd>
//  2. defaults.gui_editor
//  3. $VISUAL
//  4. VS Code y familia en el PATH (code → cursor → code-insiders), con -w
//  5. fallback del SO (macOS `open -W -t`, Windows `notepad`, resto `xdg-open`)
//  6. ResolveEditor: defaults.editor → $EDITOR → nano
//
// --terminal salta directo al 6: es la vía de escape de quien está en ssh o
// simplemente no quiere que se le abra una ventana.
//
// Es pura: todo lo externo (PATH, GOOS, $VISUAL, el yaml ya leído) entra por
// EditorEnv.
func ResolveEditEditor(env EditorEnv) EditorChoice {
	fallback := strings.TrimSpace(env.Fallback)
	if fallback == "" {
		fallback = "nano"
	}
	last := EditorChoice{Cmd: fallback, Blocking: EditorBlocks(fallback), Source: EditorSourceFallback}

	if env.Terminal {
		return last
	}
	if cmd := strings.TrimSpace(env.Flag); cmd != "" {
		return EditorChoice{Cmd: cmd, Blocking: EditorBlocks(cmd), Source: EditorSourceFlag}
	}
	if cmd := strings.TrimSpace(env.GUIEditor); cmd != "" {
		return EditorChoice{Cmd: cmd, Blocking: EditorBlocks(cmd), Source: EditorSourceGUIConfig}
	}
	if cmd := strings.TrimSpace(env.Visual); cmd != "" {
		return EditorChoice{Cmd: cmd, Blocking: EditorBlocks(cmd), Source: EditorSourceVisual}
	}

	look := env.LookPath
	if look == nil {
		look = exec.LookPath
	}
	for _, bin := range guiEditorCandidates {
		if _, err := look(bin); err == nil {
			// El -w ES la decisión: sin él `code archivo` retorna al instante y
			// no hay nada que revalidar al "cerrar".
			cmd := bin + " -w"
			return EditorChoice{Cmd: cmd, Blocking: true, Source: EditorSourcePath}
		}
	}

	// Fallback del SO. Solo se ofrece si el lanzador existe de verdad: en un
	// Linux sin xdg-open, prometer una ventana y fallar es peor que abrir nano.
	var osCmd string
	switch env.GOOS {
	case "darwin":
		osCmd = "open -W -t"
	case "windows":
		osCmd = "notepad"
	case "":
		// GOOS sin declarar: el llamador no quiso el escalón del SO.
	default:
		osCmd = "xdg-open"
	}
	if osCmd != "" {
		bin := strings.Fields(osCmd)[0]
		if _, err := look(bin); err == nil {
			return EditorChoice{Cmd: osCmd, Blocking: EditorBlocks(osCmd), Source: EditorSourceOS}
		}
	}

	return last
}

// LaunchEditor expone el lanzador real (tokeniza por espacios y conecta el
// stdio del proceso actual) para que internal/cli pueda inyectarlo en
// ProfileConfigOpts.Launch cuando la línea del editor ya viene resuelta por la
// cadena de `config edit` en vez de por ResolveEditor.
func LaunchEditor(editorLine string, files ...string) error {
	return launchEditor(editorLine, files...)
}

// ConfigEditOpts inyecta el lanzador en tests, igual que ProfileConfigOpts.
type ConfigEditOpts struct {
	Launch func(editorLine string, files ...string) error
}

// ConfigEditResult cuenta qué pasó, para que el CLI pueda ser honesto: con un
// editor no bloqueante NO se revalidó nada, y decirlo es la diferencia entre
// una advertencia y una mentira.
type ConfigEditResult struct {
	File      string
	Editor    EditorChoice
	Validated bool
}

// EditErrKind clasifica en qué punto falló `config edit`.
type EditErrKind int

const (
	EditErrLaunch  EditErrKind = iota // el editor no llegó a ejecutarse (o salió !=0)
	EditErrReread                     // al volver, el yaml ya ni se puede leer
	EditErrInvalid                    // se lee, pero la semántica de auto_handoff no cuadra
)

// EditError es el error tipado de ConfigEdit.
//
// Existe por la misma razón que ChainError: esta es la ÚNICA ruta que da valor
// real al comando —decir qué clave y qué valor están mal, y ofrecer reabrir— y
// se estaba imprimiendo en castellano dentro de sesiones en inglés. Nombrar la
// clave del yaml en dos idiomas es cosa del catálogo de internal/cli; aquí solo
// se dice QUÉ pasó y con qué.
type EditError struct {
	Kind EditErrKind
	Cmd  string // la línea de editor, para EditErrLaunch
	Err  error  // la causa (Load, ValidateEditedConfig, exec)
}

func (e *EditError) Error() string {
	switch e.Kind {
	case EditErrLaunch:
		return fmt.Sprintf("el editor falló (%s): %v", e.Cmd, e.Err)
	case EditErrReread:
		return fmt.Sprintf("%v — reedita: ccp config edit", e.Err)
	case EditErrInvalid:
		return fmt.Sprintf("%v — reedita: ccp config edit", e.Err)
	}
	return fmt.Sprintf("config edit: %v", e.Err)
}

// Unwrap deja llegar el *ChainError de dentro a errors.As, que es como el CLI
// traduce la línea que de verdad importa (la clave ofensiva del yaml).
func (e *EditError) Unwrap() error { return e.Err }

// ConfigEdit abre <home>/ccp.yaml con el editor elegido y, SOLO si ese editor
// bloquea, lo relee y lo valida al volver.
//
// Un ccp.yaml roto por una edición a mano no se manifiesta al guardar: se
// manifiesta en el siguiente `ccp session`, a mitad de un salto. Esta
// revalidación es lo único que convierte "te abro un archivo" en un comando.
func ConfigEdit(home string, choice EditorChoice, opts ConfigEditOpts) (ConfigEditResult, error) {
	file := yamlPath(home)
	res := ConfigEditResult{File: file, Editor: choice}

	// Nos aseguramos de que el archivo exista antes de abrirlo: editar un
	// buffer vacío y guardarlo produciría un yaml sin `version`, y el usuario
	// no tiene por qué saber que ese campo es obligatorio.
	if _, err := os.Stat(file); os.IsNotExist(err) {
		cfg, err := Load(home)
		if err != nil {
			return res, err
		}
		if err := Save(home, cfg); err != nil {
			return res, err
		}
	}

	launch := opts.Launch
	if launch == nil {
		launch = launchEditor
	}
	if err := launch(choice.Cmd, file); err != nil {
		return res, &EditError{Kind: EditErrLaunch, Cmd: choice.Cmd, Err: err}
	}

	if !choice.Blocking {
		return res, nil
	}

	cfg, err := Load(home)
	if err != nil {
		return res, &EditError{Kind: EditErrReread, Cmd: choice.Cmd, Err: err}
	}
	if err := ValidateEditedConfig(cfg); err != nil {
		return res, &EditError{Kind: EditErrInvalid, Cmd: choice.Cmd, Err: err}
	}
	res.Validated = true
	return res, nil
}

// ValidateEditedConfig revalida lo que una edición a mano puede romper sin que
// Load se entere. Load ya cubre la sintaxis YAML y la version del schema; lo
// que no cubre es la SEMÁNTICA del bloque auto_handoff, que solo se comprueba
// cuando el supervisor va a saltar — es decir, tarde.
//
// No duplica reglas: pide exactamente las dos que ResolveAutoChain da por
// hechas (AutoPolicy.Effective de cada política y la existencia de cada perfil
// del fallback), y las pide para TODAS las políticas, no solo la que resolvería
// el cwd actual: el usuario acaba de editar el archivo entero.
func ValidateEditedConfig(cfg *Config) error {
	if cfg == nil || cfg.AutoHandoff == nil {
		return nil
	}
	names := make([]string, 0, len(cfg.AutoHandoff.Policies))
	for n := range cfg.AutoHandoff.Policies {
		names = append(names, n)
	}
	sort.Strings(names) // orden estable: el primer error no debe depender del mapa
	for _, name := range names {
		// El error de Effective ya nombra política, clave y valor ofensivo; no
		// se le antepone la ruta yaml porque quedaría tartamudeando el nombre
		// de la política dos veces en la misma línea.
		eff, err := cfg.AutoHandoff.Policies[name].Effective(name)
		if err != nil {
			return err
		}
		for _, f := range eff.Fallback {
			if !autoProfileExists(cfg, f) {
				// Mismo Kind que usa ResolveAutoChain para esta misma condición: el
				// CLI no puede tener dos traducciones de «el fallback apunta a un
				// perfil que no existe» según por dónde se descubra.
				return &ChainError{Kind: ChainErrFallbackProfile, Policy: name, Profile: f}
			}
		}
	}
	return nil
}
