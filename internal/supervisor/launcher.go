package supervisor

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

// launcher.go — lanzar y matar a `claude`. Es la única pieza del supervisor que
// toca proceso, señales y terminal; el resto del paquete trabaja con datos.
//
// La decisión estructural: el hijo corre en el MISMO process group que el
// supervisor. Podríamos darle grupo propio (Setpgid) y cederle la terminal con
// TIOCSPGRP, pero entonces tendríamos que hacer de shell de verdad (manejar
// SIGTTOU al ceder/recuperar el grupo de primer plano, SIGTSTP, reanudaciones).
// Compartiendo grupo, el hijo YA es el proceso de primer plano de la tty y
// hereda los fds sin ceremonia. El precio es que las señales del teclado
// (Ctrl-C = SIGINT) llegan a los dos, y de ahí que el supervisor las ignore
// mientras el hijo vive: queremos que Ctrl-C mate la sesión de claude, no al
// supervisor que debe reportar el resultado y cerrar el marcador de handoff.

// LaunchSpec describe una invocación concreta de claude.
type LaunchSpec struct {
	Bin      string
	Args     []string
	Env      []string
	Dir      string
	Headless bool
	Stdin    io.Reader
	Stdout   io.Writer // en headless recibe el stream-json ya teed
	Stderr   io.Writer
}

// Process es el hijo en marcha.
//
// Un goroutine «cosechador» arrancado en Launch es el ÚNICO que llama a
// cmd.Wait(): así el hijo nunca queda zombi aunque el caller se olvide de
// esperarlo, Terminate puede observar la muerte real (canal done) sin correr
// carreras con el caller, y Wait() es idempotente (varias llamadas devuelven lo
// mismo en vez de romper con "Wait was already called").
type Process struct {
	cmd     *exec.Cmd
	stdoutR *os.File // extremo de lectura del pipe en headless; nil en interactive
	done    chan struct{}

	// resultado publicado por el cosechador antes de cerrar done.
	code int
	err  error

	// restauración del entorno del supervisor: se ejecuta una sola vez, cuando
	// el hijo ya murió (devolver SIGINT al supervisor y el termios a la tty).
	restore     func()
	restoreOnce sync.Once
}

// Launch arranca claude. En interactive hereda la tty (mismo process group, el
// supervisor ignora SIGINT para que Ctrl-C llegue solo al hijo) y guarda/restaura
// el termios. En headless conecta un pipe a stdout para el detector de stream-json.
func Launch(ctx context.Context, spec LaunchSpec) (*Process, error) {
	if strings.TrimSpace(spec.Bin) == "" {
		return nil, fmt.Errorf("no se puede lanzar: binario vacío")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	cmd := exec.CommandContext(ctx, spec.Bin, spec.Args...)
	cmd.Dir = spec.Dir
	cmd.Env = spec.Env
	// Cancelar el contexto NO debe ser un SIGKILL: claude tiene hooks SessionEnd
	// y estado de sesión que escribir. Mandamos TERM y dejamos que os/exec haga
	// el KILL de gracia si tras WaitDelay sigue vivo.
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return cmd.Process.Signal(syscall.SIGTERM)
	}
	cmd.WaitDelay = 10 * time.Second

	// Grupo de procesos: dejamos SysProcAttr a cero a propósito (ver cabecera).
	// Setpgid=true rompería Ctrl-C en interactive: el hijo dejaría de ser el
	// grupo de primer plano y no recibiría nada del teclado.

	p := &Process{cmd: cmd, done: make(chan struct{})}

	// SIGINT ignorado mientras el hijo vive. signal.Notify basta: registrar la
	// señal desactiva la acción por defecto (terminar) y los envíos que no
	// quepan en el buffer se descartan, que es exactamente lo que queremos —
	// nadie lee este canal.
	//
	// Vale para los dos modos: el Ctrl-C del teclado llega al hijo por ser del
	// mismo grupo (y `claude -p` también muere con él), así que el supervisor
	// sobrevive para reportar el exit 130 y dejar el marcador de handoff
	// coherente tanto en interactive como en headless.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT)
	ttyFd, savedTermios := -1, (*unix.Termios)(nil)
	p.restore = func() {
		signal.Stop(sigCh)
		restoreTermios(ttyFd, savedTermios)
	}

	if spec.Headless {
		// El stdout del hijo va a un pipe propio en vez de a spec.Stdout: quien
		// lee es NewStreamDetector, que parsea el NDJSON y lo copia tal cual a
		// spec.Stdout (el «tee»). Usamos os.Pipe y no cmd.StdoutPipe() porque
		// este último lo cierra dentro de Wait, y aquí Wait corre en el
		// cosechador, en paralelo al detector: cerrarle el lector por debajo le
		// comería las últimas líneas — justo donde suele venir el rate limit.
		r, w, err := os.Pipe()
		if err != nil {
			p.runRestore()
			return nil, fmt.Errorf("no se pudo crear el pipe de stdout: %w", err)
		}
		cmd.Stdout = w
		p.stdoutR = r
		cmd.Stdin = spec.Stdin // nil ⇒ /dev/null, que es lo correcto para `claude -p`
		cmd.Stderr = orStderr(spec.Stderr)
		defer w.Close() // el padre suelta su copia del extremo de escritura tras Start

		if err := cmd.Start(); err != nil {
			r.Close()
			p.runRestore()
			return nil, launchError(spec.Bin, err)
		}
	} else {
		// Interactive: los tres fds se heredan literalmente. Cuando el valor es
		// un *os.File, os/exec pasa el descriptor al hijo sin goroutines de
		// copia — condición necesaria para que la TUI de claude vea una tty de
		// verdad (raw mode, tamaño de ventana, SIGWINCH). Por eso el supervisor
		// pone aquí los writers SIN envolver (ver Options.childOut/childErr):
		// cualquier io.Writer que no sea *os.File convierte estos fds en pipes.
		cmd.Stdin = orStdin(spec.Stdin)
		cmd.Stdout = orStdout(spec.Stdout)
		cmd.Stderr = orStderr(spec.Stderr)

		// El termios se guarda ANTES de arrancar: si más tarde matamos a claude
		// en mitad de su TUI (rotación por rate limit), el modo raw se queda
		// puesto y el usuario recupera una shell sin eco ni salto de línea. Lo
		// restauramos cuando el hijo muere. Si stdin no es una tty (pipe, CI,
		// `go test`) el ioctl falla con ENOTTY y simplemente no hay nada que
		// guardar: no es un error.
		ttyFd, savedTermios = saveTermios(cmd.Stdin)

		if err := cmd.Start(); err != nil {
			p.runRestore()
			return nil, launchError(spec.Bin, err)
		}
	}

	go p.reap()
	return p, nil
}

// reap es el cosechador: la única llamada a cmd.Wait del proceso.
func (p *Process) reap() {
	err := p.cmd.Wait()
	p.code, p.err = exitCodeOf(err)
	p.runRestore()
	close(p.done)
}

func (p *Process) runRestore() {
	p.restoreOnce.Do(func() {
		if p.restore != nil {
			p.restore()
		}
	})
}

// StdoutPipe es el lector del stdout del hijo en headless (nil en interactive).
//
// El *os.File devuelto también es io.Closer: quien lo consuma (el detector de
// stream-json) puede cerrarlo al terminar. Wait no lo cierra a propósito, para
// no truncar al lector — ver el comentario del pipe en Launch.
func (p *Process) StdoutPipe() io.Reader {
	if p.stdoutR == nil {
		return nil
	}
	return p.stdoutR
}

// Wait espera al hijo y devuelve su exit code.
//
// Es idempotente y no cuelga si otro llamador ya esperó: solo bloquea hasta que
// el cosechador publique el resultado. Un exit no-cero (incluido morir por
// señal) NO es error: es información que el bucle principal necesita. El error
// se reserva para fallos de la propia espera (I/O), que sí son excepcionales.
func (p *Process) Wait() (int, error) {
	<-p.done
	return p.code, p.err
}

// Terminate manda SIGTERM y, si no muere en `grace`, SIGKILL. Es lo que corre el
// hook SessionEnd del hijo antes de morir (exit 143).
//
// La señal va al PID exacto, nunca a -PID: el hijo comparte process group con
// nosotros, así que un envío al grupo mataría también al supervisor (y a la
// shell del usuario en el peor caso).
//
// Devuelve `signaled`: si la señal LLEGÓ a salir, o sea si el hijo murió porque
// nosotros lo matamos. Es false cuando ya estaba muerto al entrar (el select de
// abajo) o cuando el Signal rebota con «process already finished», que es la
// misma carrera vista un instante después. Ese bit es la única forma que tiene
// el bucle de distinguir «terminó su trabajo» de «lo maté y atrapó la señal»:
// un claude que instale un handler de SIGTERM para correr sus hooks SessionEnd
// —justo lo que el `grace` de aquí existe para concederle— puede salir con 0
// después de que lo hayamos matado, y por el código solo es indistinguible de
// un fin feliz. Ver el switch de desenlaces en supervisor.go.
func (p *Process) Terminate(grace time.Duration) (bool, error) {
	select {
	case <-p.done:
		return false, nil // ya murió; nada que señalar
	default:
	}
	if p.cmd.Process == nil {
		return false, nil
	}

	if err := p.cmd.Process.Signal(syscall.SIGTERM); err != nil {
		if isProcessDone(err) {
			// Murió solo entre el select de arriba y este Signal. Nadie lo mató:
			// la ventana es de microsegundos pero el desenlace que decide es el
			// opuesto, así que se reporta como lo que fue.
			return false, nil
		}
		return false, fmt.Errorf("no se pudo mandar SIGTERM a %d: %w", p.cmd.Process.Pid, err)
	}

	if grace > 0 {
		t := time.NewTimer(grace)
		defer t.Stop()
		select {
		case <-p.done:
			return true, nil
		case <-t.C:
		}
	}

	// Se acabó la cortesía. SIGKILL no se puede ignorar ni atrapar, así que a
	// partir de aquí solo esperamos a que el kernel lo recoja.
	if err := p.cmd.Process.Signal(syscall.SIGKILL); err != nil && !isProcessDone(err) {
		return true, fmt.Errorf("no se pudo mandar SIGKILL a %d: %w", p.cmd.Process.Pid, err)
	}
	<-p.done
	return true, nil
}

// BuildArgs arma los args de claude: --session-id o --resume, -p +
// --output-format stream-json --verbose en headless,
// --dangerously-skip-permissions con yolo, más los args del usuario.
// Si el usuario ya pasó --resume/--session-id/-p/--output-format, respeta el suyo
// y no lo duplica.
//
// Es una función pura y deliberadamente conservadora: ante la duda, gana el
// usuario. claude aborta si recibe dos veces el mismo flag o dos selectores de
// sesión distintos, y ese fallo saldría como un exit code raro que el bucle
// interpretaría como «error del modelo».
//
// El escaneo de los args del usuario solo considera tokens que empiezan por "-".
// Un prompt posicional ("explica el flag --resume") no se confunde con un flag;
// un prompt que EMPIECE por "--resume" sí, pero entonces claude tampoco lo
// habría tratado como texto.
func BuildArgs(session string, resume, headless, yolo bool, user []string) []string {
	var out []string

	// Selector de sesión. --continue entra en el grupo aunque el contrato solo
	// nombre --resume/--session-id: también fija la conversación, y combinarlo
	// con --session-id es un error de uso en claude.
	if session != "" && !userHasFlag(user, "--resume", "-r", "--session-id", "--continue", "-c") {
		if resume {
			out = append(out, "--resume", session)
		} else {
			out = append(out, "--session-id", session)
		}
	}

	if headless {
		// -p sin --output-format stream-json daría texto plano y el detector se
		// quedaría ciego; --verbose es lo que hace que claude emita los eventos
		// intermedios (incluidos los api_retry del rate limit).
		if !userHasFlag(user, "-p", "--print") {
			out = append(out, "-p")
		}
		if !userHasFlag(user, "--output-format") {
			out = append(out, "--output-format", "stream-json")
		}
		if !userHasFlag(user, "--verbose") {
			out = append(out, "--verbose")
		}
	}

	if yolo && !userHasFlag(user, "--dangerously-skip-permissions") {
		out = append(out, "--dangerously-skip-permissions")
	}

	out = append(out, user...)
	return out
}

// userHasFlag reporta si alguno de los args del usuario es uno de `names`,
// aceptando tanto la forma separada (`--output-format stream-json`) como la
// pegada (`--output-format=stream-json`).
func userHasFlag(user []string, names ...string) bool {
	for _, a := range user {
		if len(a) < 2 || a[0] != '-' {
			continue // posicional (prompt, ruta): no es un flag
		}
		name := a
		if i := strings.IndexByte(a, '='); i > 0 {
			name = a[:i]
		}
		for _, n := range names {
			if name == n {
				return true
			}
		}
	}
	return false
}

// exitCodeOf traduce el error de cmd.Wait al exit code que espera el bucle
// principal: 0 si salió limpio, su código si salió con uno, y 128+señal si lo
// mató una señal (130 SIGINT, 143 SIGTERM, 137 SIGKILL) — el mismo convenio que
// usan bash y zsh, así que el número que ve el usuario coincide con el que vería
// lanzando claude a mano.
func exitCodeOf(err error) (int, error) {
	if err == nil {
		return 0, nil
	}
	var ee *exec.ExitError
	if !errors.As(err, &ee) {
		// No es una salida del hijo sino un fallo de la espera (I/O de los
		// pipes, por ejemplo). Devolvemos 1 para que el caller tenga un código
		// utilizable y el error para que lo pueda reportar.
		return 1, err
	}
	if ws, ok := ee.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
		return 128 + int(ws.Signal()), nil
	}
	code := ee.ExitCode()
	if code < 0 {
		code = 1
	}
	return code, nil
}

// isProcessDone distingue «la señal no llegó porque el hijo ya murió» (benigno:
// es una carrera inevitable entre Terminate y el cosechador) de un fallo real.
func isProcessDone(err error) bool {
	return errors.Is(err, os.ErrProcessDone)
}

// launchError da un error accionable cuando ni siquiera se pudo arrancar: el
// caso real es un `claude` que no está en el PATH del perfil, y el mensaje de
// os/exec por sí solo ("executable file not found in $PATH") no dice qué se
// intentó lanzar.
func launchError(bin string, err error) error {
	return fmt.Errorf("no se pudo lanzar %q: %w", bin, err)
}

func orStdin(r io.Reader) io.Reader {
	if r == nil {
		return os.Stdin
	}
	return r
}

func orStdout(w io.Writer) io.Writer {
	if w == nil {
		return os.Stdout
	}
	return w
}

func orStderr(w io.Writer) io.Writer {
	if w == nil {
		return os.Stderr
	}
	return w
}

// --- termios ---------------------------------------------------------------
//
// x/sys/unix no expone un nombre portable para el ioctl de termios: en Darwin
// es TIOCGETA/TIOCSETA y en Linux TCGETS/TCSETS, y cada constante solo existe
// en su GOOS (referenciar la otra no compila). Como este paquete no puede
// partirse en archivos con build tags, resolvemos el número en runtime. Los
// valores son los de x/sys/unix para las cuatro plataformas que ccp publica
// (darwin/linux × amd64/arm64). En cualquier otra, ok=false y nos limitamos a
// no tocar la terminal: perder la restauración es un inconveniente, abortar el
// lanzamiento sería un fallo.
func termiosReqs() (get, set uint, ok bool) {
	switch runtime.GOOS {
	case "darwin":
		return 0x40487413, 0x80487414, true // TIOCGETA, TIOCSETA
	case "linux":
		switch runtime.GOARCH {
		case "amd64", "arm64", "386", "arm", "riscv64", "loong64":
			return 0x5401, 0x5402, true // TCGETS, TCSETS
		}
	}
	return 0, 0, false
}

// saveTermios captura el estado de la tty de stdin. fd=-1 / nil cuando no hay
// tty o la plataforma no está soportada.
func saveTermios(stdin io.Reader) (int, *unix.Termios) {
	f, ok := stdin.(*os.File)
	if !ok || f == nil {
		return -1, nil
	}
	get, _, ok := termiosReqs()
	if !ok {
		return -1, nil
	}
	fd := int(f.Fd())
	st, err := unix.IoctlGetTermios(fd, get)
	if err != nil {
		return -1, nil // ENOTTY: stdin es un pipe o un archivo. Normal en scripts y tests.
	}
	return fd, st
}

// restoreTermios devuelve la tty al modo que tenía antes de lanzar. Falla en
// silencio: si la terminal desapareció (el usuario cerró el emulador) no hay
// nada que arreglar ni a quién avisar.
func restoreTermios(fd int, st *unix.Termios) {
	if fd < 0 || st == nil {
		return
	}
	_, set, ok := termiosReqs()
	if !ok {
		return
	}
	_ = unix.IoctlSetTermios(fd, set, st)
}
