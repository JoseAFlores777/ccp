package core

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// transcript.go — operaciones puras sobre los transcripts JSONL de Claude Code.
// Una sesión vive en <cc-home>/projects/<slug>/<uuid>.jsonl. La identidad de la
// sesión es el uuid del nombre de archivo, que DEBE igualar el campo sessionId
// dentro de cada línea. Validado contra CC v2.1.183 (ver spec §08).

// slugNonAlnum casa cualquier carácter que NO sea [a-zA-Z0-9]. Claude Code
// codifica el cwd en el nombre del directorio de proyecto reemplazando todos
// esos caracteres (no solo '/') por '-': '_' y '.' también se aplanan. P.ej.
// /mnt/Big_SSD_2TB/Code -> -mnt-Big-SSD-2TB-Code.
var slugNonAlnum = regexp.MustCompile(`[^a-zA-Z0-9]`)

// SlugForCwd convierte un cwd absoluto al slug de proyecto de CC: cada carácter
// no alfanumérico ('/', '_', '.', espacio, ...) -> '-'. Debe igualar la
// codificación de CC, de lo contrario el directorio de transcripts no se halla.
func SlugForCwd(cwd string) string {
	return slugNonAlnum.ReplaceAllString(cwd, "-")
}

// ProjectDir devuelve <ccHome>/projects/<slug>.
func ProjectDir(ccHome, slug string) string {
	return filepath.Join(ccHome, "projects", slug)
}

// NewUUID genera un UUID v4 (RFC 4122) usando crypto/rand.
func NewUUID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("no se pudo generar uuid: %w", err)
	}
	b[6] = (b[6] & 0x0f) | 0x40 // versión 4
	b[8] = (b[8] & 0x3f) | 0x80 // variante 10
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}

// sessionUUIDRe casa la forma 8-4-4-4-12 hex de un uuid. Se valida la FORMA, no
// la versión/variante RFC 4122: lo que importa es que el valor sea un nombre de
// archivo inocuo, y ser más estricto rechazaría uuids legítimos de sesiones
// viejas de CC.
var sessionUUIDRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// validateSessionID rechaza cualquier id de sesión que no sea un uuid.
//
// El id se concatena para formar un path (<cc-home>/projects/<slug>/<id>.jsonl),
// así que un valor con `/` o `..` sale del cc-home: sin esta validación,
// `--session ../../../../x` copiaría cualquier .jsonl legible del disco al perfil
// destino y dejaría un marcador activo apuntando fuera del home — un marcador que
// además secuestra la resolución por cwd de ese repo. Validar la FORMA es la
// defensa correcta (no filepath.Clean: un `..` limpio sigue escapando).
func validateSessionID(s string) error {
	if sessionUUIDRe.MatchString(s) {
		return nil
	}
	return fmt.Errorf("id de sesión inválido: %q; se espera el uuid del transcript (8-4-4-4-12 hexadecimal)", s)
}

// CCHome devuelve el CLAUDE_CONFIG_DIR de un perfil. Para 'default' es
// ~/.claude; para el resto, <home>/profiles/<perfil>/cc-home. Espeja la lógica
// de EnvDelta (env.go).
func CCHome(home, profile string) (string, error) {
	if profile == "default" {
		uh, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("no se pudo determinar HOME: %w", err)
		}
		return filepath.Join(uh, ".claude"), nil
	}
	return ccHomePath(home, profile), nil
}

// SessionInfo describe un transcript en disco para el picker.
type SessionInfo struct {
	UUID    string
	Path    string
	Title   string // aiTitle (último visto); "" si no hay
	ModTime time.Time
}

// ListSessions escanea <ccHome>/projects/<slug>/*.jsonl y devuelve las sesiones
// ordenadas de más nueva a más vieja (por mtime). Carpeta inexistente => lista
// vacía sin error.
func ListSessions(ccHome, slug string) ([]SessionInfo, error) {
	dir := ProjectDir(ccHome, slug)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("no se pudo leer %s: %w", dir, err)
	}
	var out []SessionInfo
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		uuid := strings.TrimSuffix(e.Name(), ".jsonl")
		full := filepath.Join(dir, e.Name())
		info, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, SessionInfo{
			UUID:    uuid,
			Path:    full,
			Title:   readAITitle(full),
			ModTime: info.ModTime(),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ModTime.After(out[j].ModTime) })
	return out, nil
}

// readAITitle devuelve el último aiTitle del transcript, o "" si no hay.
func readAITitle(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	title := ""
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024) // líneas grandes
	for sc.Scan() {
		var m map[string]any
		if json.Unmarshal(sc.Bytes(), &m) != nil {
			continue
		}
		if m["type"] == "ai-title" {
			if t, ok := m["aiTitle"].(string); ok {
				title = t
			}
		}
	}
	return title
}

// writeTranscriptAtomic escribe data en path por tmp+rename.
//
// El transcript es el ÚNICO dato de todo ccp que no se puede reconstruir: si se
// corrompe, la conversación del usuario se pierde. Y era lo único que se
// escribía con un os.WriteFile a pelo, o sea truncando el destino antes de tener
// el contenido nuevo: un Ctrl-C o un disco lleno a media escritura dejaba un
// jsonl medio escrito CON EL UUID BUENO, que es peor que no tener nada porque
// `claude --resume` lo encuentra y lo abre.
//
// El temporal se crea con os.CreateTemp en el MISMO directorio (rename entre
// sistemas de archivos falla) y con nombre aleatorio, no fijo: el RewriteSession
// de HandoffAdoptHome corre fuera del flock de handoffs.yaml, así que dos
// procesos pueden estar escribiendo el mismo destino y un tmp compartido los
// haría pisarse. El patrón "ccp-*.tmp" no acaba en .jsonl a propósito: si
// acabara, ListSessions ofrecería el temporal en el picker de sesiones.
func writeTranscriptAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("no se pudo crear %s: %w", dir, err)
	}
	tmp, err := os.CreateTemp(dir, "ccp-*.tmp")
	if err != nil {
		return fmt.Errorf("no se pudo crear temporal en %s: %w", dir, err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op si el rename salió bien

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("no se pudo escribir %s: %w", tmpName, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("no se pudo cerrar %s: %w", tmpName, err)
	}
	if err := os.Chmod(tmpName, 0o644); err != nil {
		return fmt.Errorf("no se pudo ajustar permisos de %s: %w", tmpName, err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("no se pudo renombrar %s -> %s: %w", tmpName, path, err)
	}
	return nil
}

// CopyTranscript copia srcPath al directorio dstDir conservando el nombre
// (mismo uuid). El sessionId interno ya == uuid y el cwd es el mismo repo, así
// que NO se reescribe nada. Si dstDir ya tiene ese archivo: si el contenido es
// idéntico, sobre-escribe; si difiere, error salvo force. Devuelve el path destino.
func CopyTranscript(srcPath, dstDir string, force bool) (string, error) {
	data, err := os.ReadFile(srcPath)
	if err != nil {
		return "", fmt.Errorf("no se pudo leer %s: %w", srcPath, err)
	}
	dstPath := filepath.Join(dstDir, filepath.Base(srcPath))
	if !force {
		if existing, err := os.ReadFile(dstPath); err == nil {
			if !bytes.Equal(existing, data) {
				return "", fmt.Errorf("colisión: %s ya existe con contenido distinto (usa --force)", dstPath)
			}
		}
	}
	if err := writeTranscriptAtomic(dstPath, data); err != nil {
		return "", err
	}
	return dstPath, nil
}

// RewriteSession lee srcPath (JSONL), reescribe cada campo sessionId de oldID a
// newID, y prepende "[de <fromLabel>] " al aiTitle (idempotente: no duplica si
// ya empieza con "[de "). NO toca cwd, el árbol uuid/parentUuid/leafUuid,
// messageId, timestamps ni el contenido de los mensajes. Escribe en dstPath y
// valida que el resultado no contenga oldID en sessionId y sea JSONL válido.
func RewriteSession(srcPath, dstPath, oldID, newID, fromLabel string) error {
	f, err := os.Open(srcPath)
	if err != nil {
		return fmt.Errorf("no se pudo leer %s: %w", srcPath, err)
	}
	defer f.Close()

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false) // no escapar <,>,& : mantener el JSON natural

	// bufio.Reader y no bufio.Scanner: el Scanner obliga a declarar un techo por
	// línea y CUALQUIER techo aquí es un fallo permanente. Una sola línea con una
	// imagen en base64 o un tool_result grande pasaba de los 8 MB que se fijaban
	// aquí, y entonces `handoff end` —y la vuelta a casa del supervisor, que la
	// usa— fallaban SIEMPRE para esa sesión: la conversación se podía prestar y
	// no se podía devolver nunca, con `discard` como única salida, o sea dejarla
	// viviendo solo en el perfil prestado. Justo el desenlace que todo el módulo
	// de handoff existe para evitar. La ida (CopyTranscript) nunca tuvo tope, así
	// que el límite además era asimétrico.
	//
	// No se empeora el uso de memoria: la salida ya se materializa entera en buf.
	rd := bufio.NewReaderSize(f, 64*1024)
	for lineno := 1; ; lineno++ {
		raw, err := rd.ReadBytes('\n')
		if len(bytes.TrimSpace(raw)) > 0 {
			var m map[string]any
			if jerr := json.Unmarshal(raw, &m); jerr != nil {
				return fmt.Errorf("línea JSONL inválida en %s:%d: %w", srcPath, lineno, jerr)
			}
			if sid, ok := m["sessionId"].(string); ok && sid == oldID {
				m["sessionId"] = newID
			}
			if m["type"] == "ai-title" {
				if t, ok := m["aiTitle"].(string); ok && !strings.HasPrefix(t, "[de ") {
					m["aiTitle"] = "[de " + fromLabel + "] " + t
				}
			}
			if eerr := enc.Encode(m); eerr != nil { // Encode añade '\n'
				return fmt.Errorf("no se pudo serializar línea: %w", eerr)
			}
		}
		if err != nil {
			// io.EOF sin newline final es fin normal: la última línea ya se
			// procesó arriba. Cualquier otro error sí es de lectura.
			if errors.Is(err, io.EOF) {
				break
			}
			return fmt.Errorf("error leyendo %s: %w", srcPath, err)
		}
	}

	// Validación: cero oldID en sessionId, JSONL válido. Una salida vacía (todas
	// las líneas de entrada en blanco) es válida: se omite el loop de validación
	// para no parsear "" como JSON.
	out := buf.Bytes()
	if len(bytes.TrimSpace(out)) != 0 {
		for _, ln := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			var m map[string]any
			if err := json.Unmarshal([]byte(ln), &m); err != nil {
				return fmt.Errorf("salida JSONL inválida: %w", err)
			}
			if sid, ok := m["sessionId"].(string); ok && sid == oldID {
				return fmt.Errorf("reescritura incompleta: quedó sessionId viejo")
			}
		}
	}

	return writeTranscriptAtomic(dstPath, out)
}
