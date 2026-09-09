package supervisor

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

// writeScript deja un script sh ejecutable en dir y devuelve su ruta absoluta.
// Los tests nunca lanzan el `claude` real: un script es suficiente para ejercer
// lo único que launcher.go promete (fds, exit codes, señales).
func writeScript(t *testing.T, dir, name, body string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte("#!/bin/sh\n"+body), 0o755); err != nil {
		t.Fatalf("escribiendo %s: %v", p, err)
	}
	return p
}

// devNull evita que los tests hereden el stdin/stdout del proceso de test: con
// stdin real de tty, saveTermios tocaría la terminal de quien corre `go test`.
func devNull(t *testing.T) *os.File {
	t.Helper()
	f, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err != nil {
		t.Fatalf("abriendo %s: %v", os.DevNull, err)
	}
	t.Cleanup(func() { f.Close() })
	return f
}

func TestBuildArgs(t *testing.T) {
	cases := []struct {
		name     string
		session  string
		resume   bool
		headless bool
		yolo     bool
		user     []string
		want     []string
	}{
		{
			name:    "sesion nueva interactive",
			session: "abc",
			want:    []string{"--session-id", "abc"},
		},
		{
			name:    "resume interactive",
			session: "abc",
			resume:  true,
			want:    []string{"--resume", "abc"},
		},
		{
			name: "sin sesion no emite selector",
			user: []string{"hola"},
			want: []string{"hola"},
		},
		{
			name:     "headless completo",
			session:  "abc",
			headless: true,
			want:     []string{"--session-id", "abc", "-p", "--output-format", "stream-json", "--verbose"},
		},
		{
			name:     "headless con yolo y args de usuario al final",
			session:  "abc",
			resume:   true,
			headless: true,
			yolo:     true,
			user:     []string{"sigue el plan"},
			want: []string{
				"--resume", "abc", "-p", "--output-format", "stream-json", "--verbose",
				"--dangerously-skip-permissions", "sigue el plan",
			},
		},
		{
			name:    "usuario ya pasó --resume: no duplicamos selector",
			session: "abc",
			user:    []string{"--resume", "otra"},
			want:    []string{"--resume", "otra"},
		},
		{
			name:    "usuario ya pasó -r",
			session: "abc",
			user:    []string{"-r", "otra"},
			want:    []string{"-r", "otra"},
		},
		{
			name:    "usuario ya pasó --session-id en modo resume",
			session: "abc",
			resume:  true,
			user:    []string{"--session-id", "otra"},
			want:    []string{"--session-id", "otra"},
		},
		{
			name:    "usuario ya pasó --session-id=valor pegado",
			session: "abc",
			user:    []string{"--session-id=otra"},
			want:    []string{"--session-id=otra"},
		},
		{
			name:    "usuario ya pasó --continue",
			session: "abc",
			user:    []string{"--continue"},
			want:    []string{"--continue"},
		},
		{
			name:    "usuario ya pasó -c",
			session: "abc",
			user:    []string{"-c"},
			want:    []string{"-c"},
		},
		{
			name:     "usuario ya pasó -p",
			session:  "abc",
			headless: true,
			user:     []string{"-p", "hola"},
			want:     []string{"--session-id", "abc", "--output-format", "stream-json", "--verbose", "-p", "hola"},
		},
		{
			name:     "usuario ya pasó --print",
			session:  "abc",
			headless: true,
			user:     []string{"--print"},
			want:     []string{"--session-id", "abc", "--output-format", "stream-json", "--verbose", "--print"},
		},
		{
			name:     "usuario ya pasó --output-format separado",
			session:  "abc",
			headless: true,
			user:     []string{"--output-format", "json"},
			want:     []string{"--session-id", "abc", "-p", "--verbose", "--output-format", "json"},
		},
		{
			name:     "usuario ya pasó --output-format=json pegado",
			session:  "abc",
			headless: true,
			user:     []string{"--output-format=json"},
			want:     []string{"--session-id", "abc", "-p", "--verbose", "--output-format=json"},
		},
		{
			name:     "usuario ya pasó --verbose",
			session:  "abc",
			headless: true,
			user:     []string{"--verbose"},
			want:     []string{"--session-id", "abc", "-p", "--output-format", "stream-json", "--verbose"},
		},
		{
			name: "usuario ya pasó --dangerously-skip-permissions",
			yolo: true,
			user: []string{"--dangerously-skip-permissions"},
			want: []string{"--dangerously-skip-permissions"},
		},
		{
			name:     "usuario lo pasó TODO: no añadimos nada",
			session:  "abc",
			resume:   true,
			headless: true,
			yolo:     true,
			user: []string{
				"--resume", "otra", "-p", "--output-format", "stream-json", "--verbose",
				"--dangerously-skip-permissions",
			},
			want: []string{
				"--resume", "otra", "-p", "--output-format", "stream-json", "--verbose",
				"--dangerously-skip-permissions",
			},
		},
		{
			// Un prompt posicional que MENCIONA un flag no es un flag: sin esta
			// regla, `ccp session -- "explica --resume"` se quedaría sin sesión.
			name:    "prompt que menciona un flag no cuenta como flag",
			session: "abc",
			user:    []string{"explica el flag --resume porfa"},
			want:    []string{"--session-id", "abc", "explica el flag --resume porfa"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := BuildArgs(tc.session, tc.resume, tc.headless, tc.yolo, tc.user)
			if len(got) == 0 && len(tc.want) == 0 {
				return
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("BuildArgs = %q, quiero %q", got, tc.want)
			}
		})
	}
}

// BuildArgs no debe mutar el slice del usuario: el caller lo reutiliza entre
// hops (cada relanzamiento vuelve a construir los args con los mismos user args).
func TestBuildArgsNoMutaUser(t *testing.T) {
	user := []string{"hola"}
	_ = BuildArgs("abc", false, true, true, user)
	if !reflect.DeepEqual(user, []string{"hola"}) {
		t.Fatalf("user mutado: %q", user)
	}
}

func TestLaunchWaitExitCode(t *testing.T) {
	dir := t.TempDir()
	sh := writeScript(t, dir, "fake-claude", "printf 'hola\\n'; exit 7\n")

	out := &launcherBuf{}
	null := devNull(t)
	p, err := Launch(context.Background(), LaunchSpec{
		Bin:    sh,
		Stdin:  null,
		Stdout: out,
		Stderr: null,
	})
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}
	code, err := p.Wait()
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if code != 7 {
		t.Fatalf("exit code = %d, quiero 7", code)
	}
	if got := out.String(); got != "hola\n" {
		t.Fatalf("stdout = %q, quiero %q", got, "hola\n")
	}
	// Wait es idempotente: el bucle principal puede esperarlo desde varios sitios.
	if code2, _ := p.Wait(); code2 != 7 {
		t.Fatalf("segundo Wait = %d, quiero 7", code2)
	}
	// En interactive no hay pipe de stdout.
	if p.StdoutPipe() != nil {
		t.Fatalf("StdoutPipe debería ser nil en interactive")
	}
}

func TestLaunchEnvYDir(t *testing.T) {
	dir := t.TempDir()
	sh := writeScript(t, dir, "fake-claude", "printf '%s\\n' \"$CCP_TEST_VAR\"; : > marca\n")

	out := &launcherBuf{}
	null := devNull(t)
	p, err := Launch(context.Background(), LaunchSpec{
		Bin:    sh,
		Env:    []string{"CCP_TEST_VAR=perfil-b"},
		Dir:    dir,
		Stdin:  null,
		Stdout: out,
		Stderr: null,
	})
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}
	if code, err := p.Wait(); err != nil || code != 0 {
		t.Fatalf("Wait = (%d, %v)", code, err)
	}
	if got := out.String(); got != "perfil-b\n" {
		t.Fatalf("el hijo no recibió Env: %q", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "marca")); err != nil {
		t.Fatalf("el hijo no corrió en Dir: %v", err)
	}
}

func TestLaunchHeadlessStdoutPipe(t *testing.T) {
	dir := t.TempDir()
	sh := writeScript(t, dir, "fake-claude",
		"printf '{\"type\":\"system\"}\\n'; printf '{\"type\":\"result\"}\\n'; exit 0\n")

	null := devNull(t)
	p, err := Launch(context.Background(), LaunchSpec{
		Bin:      sh,
		Headless: true,
		Stderr:   null,
	})
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}
	r := p.StdoutPipe()
	if r == nil {
		t.Fatal("StdoutPipe nil en headless")
	}
	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("leyendo el pipe: %v", err)
	}
	want := "{\"type\":\"system\"}\n{\"type\":\"result\"}\n"
	if string(data) != want {
		t.Fatalf("stream = %q, quiero %q", data, want)
	}
	if code, err := p.Wait(); err != nil || code != 0 {
		t.Fatalf("Wait = (%d, %v)", code, err)
	}
	if c, ok := r.(io.Closer); ok {
		_ = c.Close()
	}
}

// El caso que justifica el SIGKILL: un hijo que ignora SIGTERM. Verifica que
// Terminate no se queda esperando para siempre y que Wait devuelve 128+9.
func TestTerminateEscalaASIGKILL(t *testing.T) {
	dir := t.TempDir()
	ready := filepath.Join(dir, "ready")
	sh := writeScript(t, dir, "terco",
		"trap '' TERM\n: > \"$1\"\nwhile :; do sleep 0.02; done\n")

	null := devNull(t)
	p, err := Launch(context.Background(), LaunchSpec{
		Bin:    sh,
		Args:   []string{ready},
		Stdin:  null,
		Stdout: null,
		Stderr: null,
	})
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}
	waitFor(t, func() bool { _, err := os.Stat(ready); return err == nil })

	const grace = 60 * time.Millisecond
	start := time.Now()
	signaled, err := p.Terminate(grace)
	if err != nil {
		t.Fatalf("Terminate: %v", err)
	}
	if !signaled {
		t.Fatal("Terminate sobre un hijo VIVO devolvió signaled=false")
	}
	elapsed := time.Since(start)
	if elapsed < grace {
		t.Fatalf("Terminate volvió en %v: no respetó el grace de %v", elapsed, grace)
	}
	if elapsed > 3*time.Second {
		t.Fatalf("Terminate tardó %v: el SIGKILL no llegó", elapsed)
	}

	code, err := p.Wait()
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if code != 137 { // 128 + SIGKILL(9)
		t.Fatalf("exit code = %d, quiero 137", code)
	}
}

// El camino feliz de Terminate: el hijo atiende el SIGTERM (corre sus hooks) y
// sale antes del grace, así que Wait reporta 143 y no 137.
func TestTerminateSIGTERMLimpio(t *testing.T) {
	dir := t.TempDir()
	ready := filepath.Join(dir, "ready")
	sh := writeScript(t, dir, "obediente", ": > \"$1\"\nwhile :; do sleep 0.02; done\n")

	null := devNull(t)
	p, err := Launch(context.Background(), LaunchSpec{
		Bin:    sh,
		Args:   []string{ready},
		Stdin:  null,
		Stdout: null,
		Stderr: null,
	})
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}
	waitFor(t, func() bool { _, err := os.Stat(ready); return err == nil })

	signaled, err := p.Terminate(2 * time.Second)
	if err != nil {
		t.Fatalf("Terminate: %v", err)
	}
	if !signaled {
		t.Fatal("Terminate sobre un hijo VIVO devolvió signaled=false")
	}
	code, err := p.Wait()
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if code != 143 { // 128 + SIGTERM(15)
		t.Fatalf("exit code = %d, quiero 143", code)
	}
}

// Terminate sobre un hijo ya muerto no es un error: el bucle principal puede
// llegar tarde (el hijo salió solo justo cuando saltaba el detector).
func TestTerminateTrasSalidaEsNoOp(t *testing.T) {
	dir := t.TempDir()
	sh := writeScript(t, dir, "rapido", "exit 0\n")

	null := devNull(t)
	p, err := Launch(context.Background(), LaunchSpec{Bin: sh, Stdin: null, Stdout: null, Stderr: null})
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}
	if code, err := p.Wait(); err != nil || code != 0 {
		t.Fatalf("Wait = (%d, %v)", code, err)
	}
	// signaled=false es el punto: el hijo murió SOLO, no lo matamos nosotros.
	// De ese bit cuelga que el bucle lea su exit code como decisión suya.
	signaled, err := p.Terminate(time.Second)
	if err != nil {
		t.Fatalf("Terminate tras salida: %v", err)
	}
	if signaled {
		t.Fatal("Terminate sobre un hijo YA MUERTO devolvió signaled=true")
	}
}

func TestLaunchBinarioInexistente(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "no-existe-claude")

	null := devNull(t)
	p, err := Launch(context.Background(), LaunchSpec{Bin: missing, Stdin: null, Stdout: null, Stderr: null})
	if err == nil {
		_, _ = p.Wait()
		t.Fatal("quiero error al lanzar un binario inexistente")
	}
	if !strings.Contains(err.Error(), "no-existe-claude") {
		t.Fatalf("el error no dice qué se intentó lanzar: %v", err)
	}
}

func TestLaunchBinVacio(t *testing.T) {
	if _, err := Launch(context.Background(), LaunchSpec{}); err == nil {
		t.Fatal("quiero error con Bin vacío")
	}
}

// launcherBuf es un bytes.Buffer con lock: cuando Stdout no es un *os.File,
// os/exec copia en un goroutine propio, así que el buffer se escribe desde otro
// hilo que el del test.
type launcherBuf struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *launcherBuf) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *launcherBuf) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// waitFor espera a que cond se cumpla con poll corto: los tests del supervisor
// deben terminar en milisegundos, no en segundos.
func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("timeout esperando la condición")
}
