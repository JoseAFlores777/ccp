package core

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"sort"
	"time"
)

// conversations.go — todas las conversaciones de un perfil, vengan de la
// terminal o de la pestaña Code de Desktop, en una sola lista.
//
// Hasta ahora ccp listaba dos subconjuntos: las de una carpeta en el perfil
// activo (`handoff sessions`) y las que conoce el índice de Desktop (`desktop
// sessions`). La interfaz gráfica busca «esta conversación» sin saber dónde
// nació, así que las junta: el transcript manda (es la conversación) y el índice
// de Desktop aporta el título que se ve en su barra lateral.

// Conversation es una conversación de un perfil.
type Conversation struct {
	Profile      string
	UUID         string
	Title        string
	Cwd          string
	LastActivity time.Time
	Transcript   string
	Bytes        int64
	InDesktop    bool // la barra lateral de la ventana de este perfil la lista
	Archived     bool // archivada en Desktop
}

// convTitleTail y convTitleHead son las ventanas en las que se busca el título
// de una conversación que Desktop no tiene indexada. Leer el archivo entero es
// lo correcto para UNA sesión (readSessionTitle) pero no para listar cientos:
// Claude Code reescribe el título al final cada pocos turnos, así que el final
// casi siempre basta, y el principio cubre las sesiones cortas.
const (
	convTitleTail = 256 * 1024
	convTitleHead = 64 * 1024
)

// ProfileConversations lista las conversaciones de un perfil, de la más
// reciente a la más vieja. dataDir es el de su ventana de Desktop ("" si no
// tiene); ccHome, su CLAUDE_CONFIG_DIR.
func ProfileConversations(profile, ccHome, dataDir string) []Conversation {
	index := map[string]desktopIndexEntry{}
	if dataDir != "" {
		for _, e := range readDesktopIndex(dataDir) {
			if prev, ok := index[e.CliSessionID]; ok && prev.LastActivityAt >= e.LastActivityAt {
				if prev.Title == "" && e.Title != "" {
					prev.Title = e.Title
					index[e.CliSessionID] = prev
				}
				continue
			}
			index[e.CliSessionID] = e
		}
	}
	var out []Conversation
	for id, path := range desktopTranscripts(ccHome) {
		c := Conversation{Profile: profile, UUID: id, Transcript: path}
		if info, err := os.Stat(path); err == nil {
			c.LastActivity = info.ModTime()
			c.Bytes = info.Size()
		}
		if e, ok := index[id]; ok {
			c.InDesktop = true
			c.Archived = e.IsArchived
			c.Title = normalizeTitle(e.Title)
			c.Cwd = e.Cwd
			if t := msToTime(e.LastActivityAt); t.After(c.LastActivity) {
				c.LastActivity = t
			}
		}
		if c.Title == "" {
			c.Title = transcriptTitleFast(path)
		}
		if c.Cwd == "" {
			c.Cwd = TranscriptCwd(path)
		}
		out = append(out, c)
	}
	sort.SliceStable(out, func(a, b int) bool { return out[a].LastActivity.After(out[b].LastActivity) })
	return out
}

// transcriptTitleFast lee el título mirando solo el final del transcript y, si
// ahí no está, el principio. Mismo orden que readSessionTitle: el último
// customTitle no vacío y, si no hay, el último aiTitle.
func transcriptTitleFast(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return ""
	}
	size := info.Size()
	readAt := func(off, n int64) []byte {
		if n <= 0 {
			return nil
		}
		buf := make([]byte, n)
		got, err := f.ReadAt(buf, off)
		if err != nil && err != io.EOF {
			return nil
		}
		return buf[:got]
	}
	tailLen := min(size, int64(convTitleTail))
	if t := titleInChunk(readAt(size-tailLen, tailLen)); t != "" {
		return t
	}
	if size > tailLen {
		return titleInChunk(readAt(0, min(size, int64(convTitleHead))))
	}
	return ""
}

// titleInChunk busca el título en un trozo de JSONL, que puede empezar o
// acabar a media línea (esas líneas no se entienden como JSON y se saltan).
func titleInChunk(chunk []byte) string {
	custom, ai := "", ""
	for _, line := range bytes.Split(chunk, []byte("\n")) {
		if !bytes.Contains(line, []byte(`-title"`)) {
			continue
		}
		var m titleLine
		if json.Unmarshal(bytes.TrimSpace(line), &m) != nil {
			continue
		}
		switch m.Type {
		case "custom-title":
			if t := normalizeTitle(m.CustomTitle); t != "" {
				custom = t
			}
		case "ai-title":
			if t := normalizeTitle(m.AITitle); t != "" {
				ai = t
			}
		}
	}
	if custom != "" {
		return custom
	}
	return ai
}
