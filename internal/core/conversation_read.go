package core

// conversation_read.go — el texto de una conversación, legible.
//
// El transcript de Claude Code no es una lista de mensajes: es un diario de
// eventos. Cada bloque de una respuesta viaja en su propia línea (texto,
// herramienta, pensamiento, con el mismo message.id), las respuestas de las
// herramientas vuelven como mensajes del «usuario», y entre medias hay títulos,
// adjuntos, instantáneas de archivos y marcas internas. Esto lo convierte en lo
// que una persona reconoce como la conversación: lo que escribió, lo que Claude
// contestó y, aparte y resumido, lo que hizo con las herramientas.
//
// Reglas:
//   - Solo lectura, con bufio.Reader y sin techo por línea (la misma lección
//     que RewriteSession: una línea con una imagen pegada no puede romperlo).
//   - El pensamiento del modelo no se enseña: no es parte de lo que se dijo.
//   - Todo texto se recorta a convTextMax y se dice que se recortó; y se
//     devuelven los últimos `limit` mensajes, diciendo cuántos quedaron fuera.
//     Un transcript de cientos de megas no puede viajar entero a la app.

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"
)

const (
	convTextMax     = 20000 // caracteres por mensaje de texto
	convToolMax     = 1500  // caracteres de un resultado o una entrada de herramienta
	convDefaultLast = 400   // mensajes que se devuelven por defecto
)

// Tipos de mensaje.
const (
	ConvUser       = "user"        // lo que escribió la persona
	ConvAssistant  = "assistant"   // lo que contestó Claude
	ConvToolUse    = "tool_use"    // una herramienta que Claude llamó
	ConvToolResult = "tool_result" // lo que devolvió
	ConvError      = "error"       // un error de la API (un límite, por ejemplo)
)

// ConvMessage es un mensaje ya legible.
type ConvMessage struct {
	Kind      string    `json:"kind"`
	Text      string    `json:"text"`
	Tool      string    `json:"tool,omitempty"` // nombre de la herramienta (tool_use)
	At        time.Time `json:"at"`
	Truncated bool      `json:"truncated,omitempty"`
}

// ConvStats resume la conversación entera, no solo lo devuelto.
type ConvStats struct {
	UserMessages      int            `json:"user_messages"`
	AssistantMessages int            `json:"assistant_messages"`
	ToolCalls         int            `json:"tool_calls"`
	Tools             map[string]int `json:"tools"`
	Errors            int            `json:"errors"`
	Models            []string       `json:"models"`
	InputTokens       int64          `json:"input_tokens"`
	OutputTokens      int64          `json:"output_tokens"`
	First             time.Time      `json:"first"`
	Last              time.Time      `json:"last"`
}

// ConvText es la conversación legible.
type ConvText struct {
	Title    string        `json:"title"`
	Cwd      string        `json:"cwd"`
	Messages []ConvMessage `json:"messages"`
	Omitted  int           `json:"omitted"` // mensajes anteriores que no se devuelven
	Stats    ConvStats     `json:"stats"`
}

// FindTranscript devuelve el transcript de `uuid` dentro de <ccHome>/projects,
// o "" si esa cuenta no lo tiene. Solo busca ahí: quien pide leer una
// conversación nombra una cuenta y un uuid, nunca una ruta.
func FindTranscript(ccHome, uuid string) string {
	if validateSessionID(uuid) != nil {
		return ""
	}
	return desktopTranscripts(ccHome)[strings.ToLower(uuid)]
}

// ReadConversation lee el transcript y devuelve sus últimos `limit` mensajes
// (0 = convDefaultLast) con las estadísticas de toda la conversación.
func ReadConversation(path string, limit int) (ConvText, error) {
	if limit <= 0 {
		limit = convDefaultLast
	}
	f, err := os.Open(path)
	if err != nil {
		return ConvText{}, fmt.Errorf("no se pudo leer la conversación: %w", err)
	}
	defer f.Close()

	out := ConvText{Messages: []ConvMessage{}}
	out.Stats.Tools = map[string]int{}
	models := map[string]bool{}
	var all []ConvMessage
	lastAssistantID := ""

	rd := bufio.NewReaderSize(f, 64*1024)
	for {
		raw, rerr := rd.ReadBytes('\n')
		if len(strings.TrimSpace(string(raw))) > 0 {
			var ev convEvent
			if json.Unmarshal(raw, &ev) == nil {
				at := parseConvTime(ev.Timestamp)
				if !at.IsZero() {
					if out.Stats.First.IsZero() || at.Before(out.Stats.First) {
						out.Stats.First = at
					}
					if at.After(out.Stats.Last) {
						out.Stats.Last = at
					}
				}
				if ev.Cwd != "" {
					out.Cwd = ev.Cwd
				}
				switch ev.Type {
				case "custom-title":
					if strings.TrimSpace(ev.CustomTitle) != "" {
						out.Title = ev.CustomTitle
					}
				case "ai-title":
					if out.Title == "" && strings.TrimSpace(ev.AITitle) != "" {
						out.Title = ev.AITitle
					}
				case "user", "assistant":
					if ev.IsMeta {
						break
					}
					if ev.IsAPIError {
						out.Stats.Errors++
						all = append(all, ConvMessage{Kind: ConvError, Text: clip(convPlain(ev.Message.Content), convTextMax), At: at})
						break
					}
					msgs := convMessages(ev, at)
					if ev.Type == "assistant" {
						if ev.Message.Model != "" && !strings.HasPrefix(ev.Message.Model, "<") {
							models[ev.Message.Model] = true
						}
						if ev.Message.Usage != nil && ev.Message.ID != lastAssistantID {
							out.Stats.InputTokens += ev.Message.Usage.InputTokens + ev.Message.Usage.CacheRead + ev.Message.Usage.CacheCreate
							out.Stats.OutputTokens += ev.Message.Usage.OutputTokens
						}
					}
					for _, m := range msgs {
						switch m.Kind {
						case ConvUser:
							out.Stats.UserMessages++
						case ConvToolUse:
							out.Stats.ToolCalls++
							out.Stats.Tools[m.Tool]++
						case ConvAssistant:
							// Una respuesta llega en varias líneas con el mismo id:
							// se cuenta una vez y su texto se junta en un mensaje.
							if ev.Message.ID != "" && ev.Message.ID == lastAssistantID && len(all) > 0 && all[len(all)-1].Kind == ConvAssistant {
								prev := &all[len(all)-1]
								prev.Text = clip(prev.Text+"\n\n"+m.Text, convTextMax)
								prev.Truncated = prev.Truncated || m.Truncated || len(prev.Text) >= convTextMax
								continue
							}
							out.Stats.AssistantMessages++
						}
						all = append(all, m)
					}
					if ev.Type == "assistant" {
						lastAssistantID = ev.Message.ID
					}
				}
			}
		}
		if rerr != nil {
			if errors.Is(rerr, io.EOF) {
				break
			}
			return ConvText{}, fmt.Errorf("error leyendo la conversación: %w", rerr)
		}
	}

	for m := range models {
		out.Stats.Models = append(out.Stats.Models, m)
	}
	sort.Strings(out.Stats.Models)
	if out.Stats.Models == nil {
		out.Stats.Models = []string{}
	}
	if len(all) > limit {
		out.Omitted = len(all) - limit
		all = all[len(all)-limit:]
	}
	if all != nil {
		out.Messages = all
	}
	return out, nil
}

type convEvent struct {
	Type        string `json:"type"`
	Timestamp   string `json:"timestamp"`
	Cwd         string `json:"cwd"`
	IsMeta      bool   `json:"isMeta"`
	IsAPIError  bool   `json:"isApiErrorMessage"`
	CustomTitle string `json:"customTitle"`
	AITitle     string `json:"aiTitle"`
	Message     struct {
		ID      string          `json:"id"`
		Model   string          `json:"model"`
		Content json.RawMessage `json:"content"`
		Usage   *struct {
			InputTokens  int64 `json:"input_tokens"`
			OutputTokens int64 `json:"output_tokens"`
			CacheRead    int64 `json:"cache_read_input_tokens"`
			CacheCreate  int64 `json:"cache_creation_input_tokens"`
		} `json:"usage"`
	} `json:"message"`
}

type convBlock struct {
	Type    string          `json:"type"`
	Text    string          `json:"text"`
	Name    string          `json:"name"`
	Input   json.RawMessage `json:"input"`
	Content json.RawMessage `json:"content"`
	IsError bool            `json:"is_error"`
}

// convMessages convierte una línea de usuario o de asistente en mensajes.
func convMessages(ev convEvent, at time.Time) []ConvMessage {
	c := ev.Message.Content
	var s string
	if json.Unmarshal(c, &s) == nil {
		if strings.TrimSpace(s) == "" {
			return nil
		}
		kind := ConvUser
		if ev.Type == "assistant" {
			kind = ConvAssistant
		}
		t := clip(s, convTextMax)
		return []ConvMessage{{Kind: kind, Text: t, At: at, Truncated: len(t) < len(s)}}
	}
	var blocks []convBlock
	if json.Unmarshal(c, &blocks) != nil {
		return nil
	}
	var out []ConvMessage
	for _, b := range blocks {
		switch b.Type {
		case "text":
			if strings.TrimSpace(b.Text) == "" {
				continue
			}
			kind := ConvUser
			if ev.Type == "assistant" {
				kind = ConvAssistant
			}
			t := clip(b.Text, convTextMax)
			out = append(out, ConvMessage{Kind: kind, Text: t, At: at, Truncated: len(t) < len(b.Text)})
		case "image":
			out = append(out, ConvMessage{Kind: ConvUser, Text: "[imagen]", At: at})
		case "tool_use":
			in := toolInputSummary(b.Input)
			t := clip(in, convToolMax)
			out = append(out, ConvMessage{Kind: ConvToolUse, Tool: b.Name, Text: t, At: at, Truncated: len(t) < len(in)})
		case "tool_result":
			res := convPlain(b.Content)
			if b.IsError {
				res = "[error] " + res
			}
			t := clip(res, convToolMax)
			out = append(out, ConvMessage{Kind: ConvToolResult, Text: t, At: at, Truncated: len(t) < len(res)})
		}
		// thinking y lo demás no se enseñan.
	}
	return out
}

// toolInputSummary deja la entrada de una herramienta en una forma legible: el
// argumento que la define (comando, ruta, patrón…) delante, y el resto como JSON.
func toolInputSummary(raw json.RawMessage) string {
	var m map[string]any
	if json.Unmarshal(raw, &m) != nil {
		return string(raw)
	}
	for _, k := range []string{"command", "file_path", "path", "pattern", "url", "query", "description", "prompt"} {
		if v, ok := m[k].(string); ok && strings.TrimSpace(v) != "" {
			return v
		}
	}
	b, _ := json.Marshal(m)
	return string(b)
}

// convPlain saca el texto de un contenido que puede ser un string o una lista
// de bloques.
func convPlain(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var blocks []convBlock
	if json.Unmarshal(raw, &blocks) == nil {
		var parts []string
		for _, b := range blocks {
			switch b.Type {
			case "text":
				parts = append(parts, b.Text)
			case "image":
				parts = append(parts, "[imagen]")
			}
		}
		return strings.Join(parts, "\n")
	}
	return ""
}

// clip recorta a n caracteres (runas), sin partir una.
func clip(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

func parseConvTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}
	}
	return t
}
