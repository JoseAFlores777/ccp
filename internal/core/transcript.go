package core

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"encoding/json"
	"fmt"
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

// CopyTranscript copia srcPath al directorio dstDir conservando el nombre
// (mismo uuid). El sessionId interno ya == uuid y el cwd es el mismo repo, así
// que NO se reescribe nada. Si dstDir ya tiene ese archivo: si el contenido es
// idéntico, sobre-escribe; si difiere, error salvo force. Devuelve el path destino.
func CopyTranscript(srcPath, dstDir string, force bool) (string, error) {
	data, err := os.ReadFile(srcPath)
	if err != nil {
		return "", fmt.Errorf("no se pudo leer %s: %w", srcPath, err)
	}
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return "", fmt.Errorf("no se pudo crear %s: %w", dstDir, err)
	}
	dstPath := filepath.Join(dstDir, filepath.Base(srcPath))
	if !force {
		if existing, err := os.ReadFile(dstPath); err == nil {
			if !bytes.Equal(existing, data) {
				return "", fmt.Errorf("colisión: %s ya existe con contenido distinto (usa --force)", dstPath)
			}
		}
	}
	if err := os.WriteFile(dstPath, data, 0o644); err != nil {
		return "", fmt.Errorf("no se pudo escribir %s: %w", dstPath, err)
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

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for sc.Scan() {
		raw := sc.Bytes()
		if len(bytes.TrimSpace(raw)) == 0 {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal(raw, &m); err != nil {
			return fmt.Errorf("línea JSONL inválida en %s: %w", srcPath, err)
		}
		if sid, ok := m["sessionId"].(string); ok && sid == oldID {
			m["sessionId"] = newID
		}
		if m["type"] == "ai-title" {
			if t, ok := m["aiTitle"].(string); ok && !strings.HasPrefix(t, "[de ") {
				m["aiTitle"] = "[de " + fromLabel + "] " + t
			}
		}
		if err := enc.Encode(m); err != nil { // Encode añade '\n'
			return fmt.Errorf("no se pudo serializar línea: %w", err)
		}
	}
	if err := sc.Err(); err != nil {
		return fmt.Errorf("error leyendo %s: %w", srcPath, err)
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

	if err := os.MkdirAll(filepath.Dir(dstPath), 0o755); err != nil {
		return fmt.Errorf("no se pudo crear %s: %w", filepath.Dir(dstPath), err)
	}
	if err := os.WriteFile(dstPath, out, 0o644); err != nil {
		return fmt.Errorf("no se pudo escribir %s: %w", dstPath, err)
	}
	return nil
}
