package cli

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"runtime/debug"
	"sync"

	"github.com/JoseAFlores777/ccp/internal/core"
	"github.com/JoseAFlores777/ccp/internal/core/i18n"
)

// serve.go — `ccp serve --stdio`: el motor de ccp como API para máquinas.
//
// La interfaz gráfica (gui/, Tauri) no reimplementa nada: arranca `ccp serve
// --stdio` y le habla en JSON, una petición y una respuesta por línea. Así se
// sostiene también para la GUI la regla del repo: toda la lógica vive en
// internal/core y cada acción tiene su comando.
//
// Protocolo (versión 1):
//
//	→ {"id":1,"method":"profiles.list","params":{}}
//	← {"id":1,"result":[…]}   o   {"id":1,"error":{"code":"…","message":"…"}}
//
// Al arrancar se emite {"event":"ready","protocol":1,"version":"…"}. Con EOF en
// stdin, se terminan las peticiones en curso y se sale con 0.
//
// Las lecturas corren en paralelo; las escrituras se serializan con un mutex,
// porque varias hacen un leer-modificar-escribir de ccp.yaml que el flock de
// Save no cubre entero. Las operaciones largas (copiar una sesión e importarla,
// actualizar ccp) son escrituras: bloquean otras escrituras, nunca las lecturas.

// serveProtocol es la versión del protocolo. Sube solo si cambia la forma de un
// mensaje existente; añadir métodos no la cambia.
const serveProtocol = 1

type serveRequest struct {
	ID     json.RawMessage `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}

type serveError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *serveError) Error() string { return e.Message }

type serveResponse struct {
	ID     json.RawMessage `json:"id"`
	Result any             `json:"result,omitempty"`
	Error  *serveError     `json:"error,omitempty"`
}

// serveMethod es un método del protocolo. write=true lo serializa con las demás
// escrituras.
type serveMethod struct {
	write bool
	fn    func(s *server, params json.RawMessage) (any, error)
}

// server es el estado de una sesión de `ccp serve`.
type server struct {
	home    string
	methods map[string]serveMethod

	writeMu sync.Mutex // serializa los métodos que escriben
	outMu   sync.Mutex // una respuesta por línea, sin intercalarse
	out     io.Writer
}

// okResult es la respuesta de los métodos que no devuelven datos.
var okResult = map[string]any{"ok": true}

func cmdServe(args []string, stdout, stderr io.Writer) int {
	stdio := false
	for _, a := range args {
		switch a {
		case "--stdio":
			stdio = true
		default:
			fmt.Fprintf(stderr, "[error] %s\n", i18n.T(currentLang(), "cli.serve.usage"))
			return 1
		}
	}
	if !stdio {
		fmt.Fprintf(stderr, "[error] %s\n", i18n.T(currentLang(), "cli.serve.usage"))
		return 1
	}
	// stdout es el canal del protocolo: cualquier Print suelto de otra parte del
	// binario lo corrompería. Se desvía os.Stdout a stderr para que lo que se
	// escape acabe en el registro de la GUI y no en mitad de una respuesta.
	if f, ok := stdout.(*os.File); ok && f == os.Stdout {
		os.Stdout = os.Stderr
	}
	// La GUI puede correr una copia de ccp que no es la que el usuario tiene
	// instalada; los sensores deben apuntar a la instalada, que es la que
	// seguirá ahí cuando la app se cierre.
	if bin := os.Getenv("CCP_SENSOR_BIN"); bin != "" {
		if info, err := os.Stat(bin); err == nil && !info.IsDir() {
			core.SetAutoHooksBin(bin)
		}
	}
	return serveLoop(os.Stdin, stdout, stderr)
}

// serveLoop atiende peticiones hasta EOF. Es la función que prueban los tests.
func serveLoop(in io.Reader, out, stderr io.Writer) int {
	home := resolveHome()
	if err := ensureMigrated(home); err != nil {
		fmt.Fprintf(stderr, "[error] %v\n", err)
		return 1
	}
	s := &server{home: home, out: out, methods: serveRegistry()}
	s.emit(map[string]any{"event": "ready", "protocol": serveProtocol, "version": core.Version})

	rd := bufio.NewReaderSize(in, 1<<20)
	var wg sync.WaitGroup
	for {
		line, err := rd.ReadBytes('\n')
		if trimmed := bytes.TrimSpace(line); len(trimmed) > 0 {
			var req serveRequest
			if jerr := json.Unmarshal(trimmed, &req); jerr != nil {
				s.reply(nil, nil, &serveError{Code: "parse_error", Message: jerr.Error()})
			} else {
				wg.Add(1)
				go func() {
					defer wg.Done()
					s.handle(req)
				}()
			}
		}
		if err != nil {
			break
		}
	}
	wg.Wait()
	return 0
}

func (s *server) handle(req serveRequest) {
	m, ok := s.methods[req.Method]
	if !ok {
		s.reply(req.ID, nil, &serveError{Code: "unknown_method", Message: "método desconocido: " + req.Method})
		return
	}
	if m.write {
		s.writeMu.Lock()
		defer s.writeMu.Unlock()
	}
	res, err := s.call(m, req.Params)
	if err != nil {
		se, ok := err.(*serveError)
		if !ok {
			se = &serveError{Code: "failed", Message: err.Error()}
		}
		s.reply(req.ID, nil, se)
		return
	}
	if res == nil {
		res = okResult
	}
	s.reply(req.ID, res, nil)
}

// call ejecuta un método convirtiendo un panic en un error: un fallo en un
// método no puede tumbar el servidor y dejar la GUI sin motor.
func (s *server) call(m serveMethod, params json.RawMessage) (res any, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = &serveError{Code: "internal", Message: fmt.Sprintf("%v\n%s", r, debug.Stack())}
		}
	}()
	return m.fn(s, params)
}

func (s *server) reply(id json.RawMessage, result any, e *serveError) {
	if id == nil {
		id = json.RawMessage("null")
	}
	s.emit(serveResponse{ID: id, Result: result, Error: e})
}

func (s *server) emit(v any) {
	data, err := json.Marshal(v)
	if err != nil {
		data, _ = json.Marshal(serveResponse{ID: json.RawMessage("null"),
			Error: &serveError{Code: "internal", Message: err.Error()}})
	}
	s.outMu.Lock()
	defer s.outMu.Unlock()
	_, _ = s.out.Write(append(data, '\n'))
}

// params decodifica los parámetros de un método. Sin parámetros vale el valor
// cero del tipo: casi todos los métodos tienen todo opcional.
func params[T any](raw json.RawMessage) (T, error) {
	var v T
	if len(bytes.TrimSpace(raw)) == 0 || string(bytes.TrimSpace(raw)) == "null" {
		return v, nil
	}
	if err := json.Unmarshal(raw, &v); err != nil {
		return v, &serveError{Code: "invalid_params", Message: err.Error()}
	}
	return v, nil
}

// badParams es el error de un parámetro que falta o no vale.
func badParams(format string, a ...any) error {
	return &serveError{Code: "invalid_params", Message: fmt.Sprintf(format, a...)}
}

// cliRun es el resultado de un comando de la CLI ejecutado dentro del proceso.
type cliRun struct {
	Args   []string `json:"args"`
	Exit   int      `json:"exit"`
	Stdout string   `json:"stdout"`
	Stderr string   `json:"stderr"`
}

// runInProcess ejecuta un comando de la CLI dentro del mismo proceso y devuelve lo que
// imprimió. Es lo que usan las operaciones compuestas que ya existen como
// comando (abrir una ventana de Desktop, instalar sensores, actualizar): así la
// GUI hace exactamente lo mismo que la terminal, con los mismos avisos, sin una
// segunda implementación que se desincronice.
func runInProcess(args ...string) cliRun {
	var out, errb bytes.Buffer
	code := Dispatch(args, &out, &errb)
	return cliRun{Args: args, Exit: code, Stdout: out.String(), Stderr: errb.String()}
}
