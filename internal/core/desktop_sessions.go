package core

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

// desktop_sessions.go — llevar una conversación de la pestaña Code de la
// ventana de Desktop de un perfil a la de otro (`ccp desktop sessions|copy`).
//
// Una sesión de la pestaña Code son DOS cosas en dos sitios, y ccp solo
// escribe una:
//
//  1. El transcript, <cc-home>/projects/<slug>/<uuid>.jsonl: el mismo formato
//     que el CLI, porque la pestaña Code ES Claude Code. Es la conversación, lo
//     único irrecuperable, y es lo que ccp copia.
//  2. La entrada del índice de Desktop,
//     <data-dir>/claude-code-sessions/<cuenta>/<org>/local_<id>.json, con el
//     título, la carpeta y el `cliSessionId` que apunta a (1). Es lo que pinta
//     la barra lateral. ccp la LEE (de ahí salen los títulos) y no la escribe
//     nunca: es un formato interno, y escribirla con la app abierta sería
//     pelearse con el estado que Desktop tiene en memoria.
//
// La entrada del destino la crea el propio Desktop al recibir
// `claude://resume?session=<uuid>` (importCliSession; medido en el app.asar de
// 2.2553.1 y presente también en 2.110.1). Tres hechos de ese lado dan forma a
// este fichero:
//
//   - la importación busca <uuid>.jsonl en cualquier <config-dir>/projects/*/ y
//     saca la carpeta de trabajo del propio transcript: basta con copiarlo al
//     mismo proyecto con el mismo uuid;
//   - el título lo lee SOLO de los últimos 256 KiB (líneas `custom-title`). Una
//     sesión larga que se tituló al principio se importa sin nombre —le pasó a
//     la primera que se movió a mano, el 2026-09-18—, así que la copia lleva el
//     título al final;
//   - RECHAZA un transcript con más de un hard link (`onMultiLink: refuse`). El
//     truco que abarata el espejo de los lanzadores aquí rompería la
//     importación: la copia es siempre un archivo propio.

// desktopImportTitleWindow son los bytes, contados desde el final, en los que
// Desktop busca el título al importar (`vji=262144` en app.asar).
const desktopImportTitleWindow = 256 * 1024

// desktopMainBundleID es el id de Claude.app. Solo se usa si no se puede leer
// del Info.plist de la app instalada.
const desktopMainBundleID = "com.anthropic.claudefordesktop"

// DesktopResumeURL es el enlace con el que Desktop importa una sesión del CLI y
// la añade a su barra lateral.
func DesktopResumeURL(uuid string) string {
	return "claude://resume?session=" + uuid
}

// DesktopUserDataDir devuelve dónde guarda sus datos la instancia de Desktop de
// un perfil. Para los perfiles es su --user-data-dir; para `default`, el
// directorio de siempre de la app, que DesktopDataDir deja vacío a propósito (no
// hay que pasárselo a nadie) pero que aquí hace falta para leer su índice.
// CCP_DESKTOP_DEFAULT_DATA_DIR lo sustituye en los tests.
func DesktopUserDataDir(home, name string) (string, error) {
	if d := DesktopDataDir(home, name); d != "" {
		return d, nil
	}
	if d := os.Getenv("CCP_DESKTOP_DEFAULT_DATA_DIR"); d != "" {
		return d, nil
	}
	uh, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("no se pudo determinar HOME: %w", err)
	}
	if runtime.GOOS == "darwin" {
		return filepath.Join(uh, "Library", "Application Support", "Claude"), nil
	}
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		return filepath.Join(x, "Claude"), nil
	}
	return filepath.Join(uh, ".config", "Claude"), nil
}

// DesktopBundleIDOf lee el CFBundleIdentifier de una app. Existe para no dar
// por hecho el id del Claude principal: una build beta o una copia que el
// usuario indicó con --app puede llevar otro.
func DesktopBundleIDOf(app string) string {
	src, err := readDesktopSource(app)
	if err != nil {
		return desktopMainBundleID
	}
	if id := src.plist.String("CFBundleIdentifier"); id != "" {
		return id
	}
	return desktopMainBundleID
}

// DesktopSession es una conversación de la pestaña Code tal como la ve ccp: lo
// que dice el índice de Desktop, más dónde está su transcript.
type DesktopSession struct {
	Profile      string
	UUID         string // cliSessionId: el nombre del transcript
	Title        string
	Cwd          string
	LastActivity time.Time
	Archived     bool
	Transcript   string // ruta del .jsonl; "" si no está en el cc-home del perfil
	Indexed      bool   // la conoce Desktop; false = sesión del CLI localizada por uuid
}

// desktopIndexEntry es lo que ccp lee de cada local_*.json: solo los campos que
// usa, y todos opcionales. Es un formato interno de Desktop, así que un campo
// que cambie de nombre debe degradar a «sin título», no romper el listado.
// lastActivityAt va en float64 porque json.Unmarshal descarta el struct entero
// si un número con decimales cae en un int.
type desktopIndexEntry struct {
	CliSessionID   string  `json:"cliSessionId"`
	Title          string  `json:"title"`
	Cwd            string  `json:"cwd"`
	LastActivityAt float64 `json:"lastActivityAt"`
	IsArchived     bool    `json:"isArchived"`
}

// readDesktopIndex devuelve las entradas del índice de una instancia, de todas
// sus cuentas: Desktop las guarda por <cuenta>/<org>, y la sesión que se busca
// puede estar bajo una que no es la activa. Un data dir sin índice es una
// instancia sin sesiones, no un error.
func readDesktopIndex(dataDir string) []desktopIndexEntry {
	root := filepath.Join(dataDir, "claude-code-sessions")
	var out []desktopIndexEntry
	accounts, _ := os.ReadDir(root)
	for _, a := range accounts {
		if !a.IsDir() {
			continue
		}
		orgs, _ := os.ReadDir(filepath.Join(root, a.Name()))
		for _, o := range orgs {
			if !o.IsDir() {
				continue
			}
			dir := filepath.Join(root, a.Name(), o.Name())
			files, _ := os.ReadDir(dir)
			for _, f := range files {
				name := f.Name()
				if !f.Type().IsRegular() || !strings.HasPrefix(name, "local_") || !strings.HasSuffix(name, ".json") {
					continue
				}
				data, err := os.ReadFile(filepath.Join(dir, name))
				if err != nil {
					continue
				}
				var e desktopIndexEntry
				if json.Unmarshal(data, &e) != nil || validateSessionID(e.CliSessionID) != nil {
					continue
				}
				e.CliSessionID = strings.ToLower(e.CliSessionID)
				out = append(out, e)
			}
		}
	}
	return out
}

// DesktopIndexed dice si la instancia de ese data dir ya tiene en su barra
// lateral una sesión que apunta a ese transcript, con cualquier id local. Es lo
// que evita importar dos veces: una segunda entrada sobre el mismo archivo son
// dos ventanas escribiendo la misma conversación.
func DesktopIndexed(dataDir, uuid string) bool {
	uuid = strings.ToLower(uuid)
	for _, e := range readDesktopIndex(dataDir) {
		if e.CliSessionID == uuid {
			return true
		}
	}
	return false
}

// desktopTranscripts indexa por uuid los transcripts de un cc-home. Si un uuid
// aparece en dos proyectos gana el más grande, que es el mismo criterio con el
// que lo resuelve Desktop. Los symlinks no cuentan: la copia tiene que leer un
// archivo del perfil, no seguir un enlace fuera de él.
func desktopTranscripts(ccHome string) map[string]string {
	out := map[string]string{}
	size := map[string]int64{}
	projects := filepath.Join(ccHome, "projects")
	dirs, _ := os.ReadDir(projects)
	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}
		entries, _ := os.ReadDir(filepath.Join(projects, d.Name()))
		for _, e := range entries {
			id := strings.TrimSuffix(e.Name(), ".jsonl")
			if !e.Type().IsRegular() || id == e.Name() || validateSessionID(id) != nil {
				continue
			}
			info, err := e.Info()
			if err != nil {
				continue
			}
			key := strings.ToLower(id)
			if prev, seen := size[key]; seen && prev >= info.Size() {
				continue
			}
			out[key] = filepath.Join(projects, d.Name(), e.Name())
			size[key] = info.Size()
		}
	}
	return out
}

// DesktopSessions lista las conversaciones de la pestaña Code de la instancia de
// un perfil, de la más reciente a la más vieja. Una que el índice nombra y cuyo
// transcript no está en el cc-home (remota, por SSH, borrada) sale con
// Transcript vacío: existe para Desktop, pero no hay nada que copiar.
func DesktopSessions(profile, dataDir, ccHome string) []DesktopSession {
	transcripts := desktopTranscripts(ccHome)
	at := map[string]int{}
	var out []DesktopSession
	for _, e := range readDesktopIndex(dataDir) {
		s := DesktopSession{
			Profile: profile, UUID: e.CliSessionID, Title: normalizeTitle(e.Title),
			Cwd: e.Cwd, LastActivity: msToTime(e.LastActivityAt), Archived: e.IsArchived,
			Transcript: transcripts[e.CliSessionID], Indexed: true,
		}
		i, dup := at[s.UUID]
		if !dup {
			at[s.UUID] = len(out)
			out = append(out, s)
			continue
		}
		// Dos entradas sobre el mismo transcript (una nativa y otra importada):
		// manda la más reciente, sin perder un título que solo tenga la otra.
		prev := out[i]
		if s.LastActivity.After(prev.LastActivity) {
			if s.Title == "" {
				s.Title = prev.Title
			}
			out[i] = s
		} else if prev.Title == "" {
			out[i].Title = s.Title
		}
	}
	for i := range out {
		if out[i].Title == "" && out[i].Transcript != "" {
			out[i].Title = TranscriptTitle(out[i].Transcript)
		}
	}
	sort.SliceStable(out, func(a, b int) bool { return out[a].LastActivity.After(out[b].LastActivity) })
	return out
}

// DesktopTranscriptSession localiza por uuid, en el cc-home de un perfil, una
// sesión que el índice de Desktop no conoce: una del CLI, típicamente. Se puede
// importar igual, porque el enlace de importación está hecho justo para eso.
func DesktopTranscriptSession(profile, ccHome, uuid string) (DesktopSession, bool) {
	uuid = strings.ToLower(uuid)
	path, ok := desktopTranscripts(ccHome)[uuid]
	if !ok {
		return DesktopSession{}, false
	}
	s := DesktopSession{
		Profile: profile, UUID: uuid, Transcript: path,
		Title: TranscriptTitle(path), Cwd: TranscriptCwd(path),
	}
	if info, err := os.Stat(path); err == nil {
		s.LastActivity = info.ModTime()
	}
	return s, true
}

// IsSessionID dice si s tiene la forma de un uuid de sesión completo: es lo
// que distingue «búscalo por uuid también fuera del índice» de un prefijo o un
// título.
func IsSessionID(s string) bool {
	return validateSessionID(strings.TrimSpace(s)) == nil
}

// normalizeTitle aplana los espacios como hace Desktop antes de pintar un
// título, para comparar lo que el usuario teclea con lo que ve.
func normalizeTitle(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func msToTime(ms float64) time.Time {
	if ms <= 0 {
		return time.Time{}
	}
	return time.UnixMilli(int64(ms))
}

// eachTranscriptLine recorre un JSONL con bufio.Reader, no con Scanner: una
// línea con una imagen pegada supera cualquier tope razonable, y aquí un tope
// significaría no ver lo que viene detrás (ver RewriteSession). Para en cuanto
// fn devuelve false.
func eachTranscriptLine(path string, fn func([]byte) bool) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	rd := bufio.NewReaderSize(f, 64*1024)
	for {
		raw, err := rd.ReadBytes('\n')
		if line := bytes.TrimSpace(raw); len(line) > 0 && !fn(line) {
			return
		}
		if err != nil {
			return
		}
	}
}

// titleLine es la forma de las dos líneas de título de un transcript.
type titleLine struct {
	Type        string `json:"type"`
	CustomTitle string `json:"customTitle"`
	AITitle     string `json:"aiTitle"`
}

// TranscriptTitle devuelve el título de un transcript con la precedencia con
// que lo enseña Claude Code: el último `custom-title` (el que pone el usuario o
// Desktop) y, si no hay ninguno, el último `ai-title`.
func TranscriptTitle(path string) string {
	custom, ai := "", ""
	eachTranscriptLine(path, func(line []byte) bool {
		// El filtro barato antes del Unmarshal: una línea de título es corta,
		// pero la de un tool_result puede pesar megas.
		if !bytes.Contains(line, []byte(`-title"`)) {
			return true
		}
		var m titleLine
		if json.Unmarshal(line, &m) != nil {
			return true
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
		return true
	})
	if custom != "" {
		return custom
	}
	return ai
}

// TranscriptCwd devuelve la carpeta de trabajo que registra un transcript: la
// de la primera línea que la lleve.
func TranscriptCwd(path string) string {
	cwd := ""
	eachTranscriptLine(path, func(line []byte) bool {
		if !bytes.Contains(line, []byte(`"cwd"`)) {
			return true
		}
		var m struct {
			Cwd string `json:"cwd"`
		}
		if json.Unmarshal(line, &m) == nil && m.Cwd != "" {
			cwd = m.Cwd
			return false
		}
		return true
	})
	return cwd
}

// uuidPrefixRe acepta lo que puede ser el principio de un uuid: 4 hex o más.
var uuidPrefixRe = regexp.MustCompile(`^[0-9a-f]{4}[0-9a-f-]{0,32}$`)

// MatchDesktopSessions elige entre los candidatos por lo que teclea el usuario,
// quedándose con el PRIMER nivel que dé algo:
//
//  1. el uuid completo;
//  2. un prefijo del uuid de 4 caracteres o más (lo que enseña `desktop sessions`);
//  3. el título exacto, sin distinguir mayúsculas ni espacios de más;
//  4. un trozo del título.
//
// Devuelve todos los del nivel ganador. Uno es la respuesta; varios, una
// ambigüedad que resuelve el usuario —con el uuid o con --from—, nunca ccp:
// copiar la conversación equivocada a otra cuenta no se deshace con un Ctrl-Z.
func MatchDesktopSessions(query string, cands []DesktopSession) []DesktopSession {
	title := normalizeTitle(query)
	q := strings.ToLower(title)
	if q == "" {
		return nil
	}
	tiers := []func(DesktopSession) bool{
		func(s DesktopSession) bool { return s.UUID == q },
		func(s DesktopSession) bool { return uuidPrefixRe.MatchString(q) && strings.HasPrefix(s.UUID, q) },
		func(s DesktopSession) bool { return s.Title != "" && strings.EqualFold(s.Title, title) },
		func(s DesktopSession) bool { return s.Title != "" && strings.Contains(strings.ToLower(s.Title), q) },
	}
	for _, match := range tiers {
		var out []DesktopSession
		for _, s := range cands {
			if match(s) {
				out = append(out, s)
			}
		}
		if len(out) > 0 {
			return out
		}
	}
	return nil
}

// DesktopCopyPlan es la copia de una sesión de un perfil a otro, ya resuelta.
type DesktopCopyPlan struct {
	From, To      string
	UUID          string
	Title         string
	Cwd           string
	SrcTranscript string
	SrcCompanion  string // <proyecto>/<uuid>/: subagentes y workflows (puede no existir)
	DstTranscript string
	DstCompanion  string
}

// PlanDesktopCopy calcula dónde va cada cosa. El destino conserva el nombre del
// proyecto y el uuid: Desktop encuentra el transcript por uuid, y `claude
// --resume` lanzado desde la carpeta de la sesión, por proyecto.
func PlanDesktopCopy(src DesktopSession, to, dstCCHome string) (DesktopCopyPlan, error) {
	if src.Profile == to {
		return DesktopCopyPlan{}, fmt.Errorf("la sesión ya está en %s: el destino tiene que ser otro perfil", to)
	}
	if src.Transcript == "" {
		return DesktopCopyPlan{}, fmt.Errorf(
			"la sesión %s no tiene transcript en el perfil %s (¿es remota o se borró?): no hay nada que copiar",
			ShortUUID(src.UUID), src.Profile)
	}
	// El nombre del archivo y no src.UUID: es lo que Desktop y `claude --resume`
	// buscan, y el que se usa para la carpeta hermana.
	id := strings.TrimSuffix(filepath.Base(src.Transcript), ".jsonl")
	if err := validateSessionID(id); err != nil {
		return DesktopCopyPlan{}, err
	}
	srcDir := filepath.Dir(src.Transcript)
	dstDir := ProjectDir(dstCCHome, filepath.Base(srcDir))
	cwd := src.Cwd
	if cwd == "" {
		cwd = TranscriptCwd(src.Transcript)
	}
	return DesktopCopyPlan{
		From: src.Profile, To: to, UUID: id, Title: src.Title, Cwd: cwd,
		SrcTranscript: src.Transcript, SrcCompanion: filepath.Join(srcDir, id),
		DstTranscript: filepath.Join(dstDir, id+".jsonl"), DstCompanion: filepath.Join(dstDir, id),
	}, nil
}

// DesktopCopyOutcome dice qué le pasa (o le pasaría) al transcript del destino.
type DesktopCopyOutcome string

const (
	DesktopCopyNew     DesktopCopyOutcome = "new"     // el destino no la tenía
	DesktopCopySame    DesktopCopyOutcome = "same"    // ya estaba, idéntica
	DesktopCopyUpdated DesktopCopyOutcome = "updated" // tenía una copia anterior: se le trae lo nuevo
	DesktopCopyAhead   DesktopCopyOutcome = "ahead"   // ya la tiene y siguió por su cuenta: no se toca
)

// ErrDesktopCopyDiverged: origen y destino siguieron la conversación cada uno
// por su lado. No hay copia que no pierda una de las dos, así que no se hace.
var ErrDesktopCopyDiverged = errors.New("la sesión siguió por separado en el origen y en el destino")

// DesktopCopyResult es lo que hizo CopyDesktopSession.
type DesktopCopyResult struct {
	Outcome       DesktopCopyOutcome
	TitleAppended bool // se añadió el custom-title al final de la copia
	FilesCopied   int  // archivos de la carpeta hermana copiados o puestos al día
	FilesKept     int  // ... que el destino tenía distintos y se dejaron como estaban
}

// InspectDesktopCopy dice qué haría CopyDesktopSession con el transcript sin
// escribir nada. Lo usan el --dry-run y la guarda de la CLI que no deja poner
// al día una sesión que la ventana del destino podría tener cargada.
func InspectDesktopCopy(p DesktopCopyPlan) (DesktopCopyOutcome, error) {
	src, want, err := desktopCopyPayload(p)
	if err != nil {
		return "", err
	}
	return desktopCopyOutcome(p.DstTranscript, src, want)
}

// CopyDesktopSession ejecuta la copia. Es idempotente y nunca pisa nada que el
// destino tenga y el origen no: la conversación es lo único irrecuperable de
// todo esto, y un destino que siguió por su cuenta perdería trabajo.
//
// Comparando bytes, hay cuatro casos:
//
//   - el destino no la tiene: se copia;
//   - ya la tiene idéntica: nada;
//   - tiene una copia anterior (lo suyo es el comienzo de lo del origen, a lo
//     sumo con metadatos detrás): se reemplaza por la actual. Es la vuelta de un
//     préstamo de ida y vuelta;
//   - la tiene y siguió por su cuenta (lo del origen es el comienzo de lo suyo):
//     no se toca.
//
// Cualquier otra cosa es que las dos siguieron por separado:
// ErrDesktopCopyDiverged, sin escribir nada.
func CopyDesktopSession(p DesktopCopyPlan) (DesktopCopyResult, error) {
	var res DesktopCopyResult
	src, want, err := desktopCopyPayload(p)
	if err != nil {
		return res, err
	}
	res.Outcome, err = desktopCopyOutcome(p.DstTranscript, src, want)
	if err != nil {
		return res, err
	}
	if res.Outcome == DesktopCopyNew || res.Outcome == DesktopCopyUpdated {
		// 0700 antes de escribir: writeFileAtomic crearía el proyecto con 0755, y
		// la conversación de una cuenta no tiene por qué ser legible por otros
		// usuarios de la máquina.
		if err := os.MkdirAll(filepath.Dir(p.DstTranscript), 0o700); err != nil {
			return res, fmt.Errorf("no se pudo crear %s: %w", filepath.Dir(p.DstTranscript), err)
		}
		if err := writeFileAtomic(p.DstTranscript, want, 0o600); err != nil {
			return res, err
		}
		res.TitleAppended = len(want) > len(src)
	}
	res.FilesCopied, res.FilesKept, err = copyDesktopCompanion(p.SrcCompanion, p.DstCompanion)
	return res, err
}

// desktopCopyPayload lee el origen y arma lo que debería quedar en el destino:
// el transcript tal cual, más el título al final cuando Desktop no lo vería.
func desktopCopyPayload(p DesktopCopyPlan) (src, want []byte, err error) {
	src, err = os.ReadFile(p.SrcTranscript)
	if err != nil {
		return nil, nil, fmt.Errorf("no se pudo leer %s: %w", p.SrcTranscript, err)
	}
	if len(src) > 0 && src[len(src)-1] != '\n' {
		src = append(src, '\n')
	}
	want = src
	if line := desktopTitleLine(src, p.Title, p.UUID); line != nil {
		want = append(append(make([]byte, 0, len(src)+len(line)), src...), line...)
	}
	return src, want, nil
}

// desktopCopyOutcome clasifica el destino frente a lo que se quiere dejar. El
// orden importa: primero lo que no pierde nada (idéntico, o el destino es el
// comienzo de lo nuevo), luego lo que no hay que tocar (el destino ya contiene
// todo el origen), y solo al final la copia vieja con metadatos detrás.
func desktopCopyOutcome(dst string, src, want []byte) (DesktopCopyOutcome, error) {
	have, err := os.ReadFile(dst)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return DesktopCopyNew, nil
	case err != nil:
		return "", fmt.Errorf("no se pudo leer %s: %w", dst, err)
	case bytes.Equal(have, want):
		return DesktopCopySame, nil
	case bytes.HasPrefix(want, have):
		return DesktopCopyUpdated, nil
	case bytes.HasPrefix(have, src):
		return DesktopCopyAhead, nil
	case bytes.HasPrefix(src, trimTrailingMeta(have)):
		return DesktopCopyUpdated, nil
	}
	return "", fmt.Errorf("%w: %s no es ni una copia anterior ni una continuación de la del origen",
		ErrDesktopCopyDiverged, dst)
}

// trimTrailingMeta quita del final las líneas que no son parte de la
// conversación: las que no llevan `uuid` (títulos, modo, last-prompt…). Desktop
// añade varias con solo abrir una sesión, y sin quitarlas una copia vieja que
// nadie continuó parecería una conversación distinta. Una línea que no se puede
// leer cuenta como conversación: ante la duda, no se reemplaza nada.
func trimTrailingMeta(b []byte) []byte {
	for {
		body := bytes.TrimRight(b, "\n")
		i := bytes.LastIndexByte(body, '\n')
		last := body[i+1:]
		var m map[string]json.RawMessage
		if len(last) == 0 || json.Unmarshal(last, &m) != nil {
			return b
		}
		if _, isMessage := m["uuid"]; isMessage {
			return b
		}
		b = body[:i+1]
	}
}

// desktopTitleLine devuelve la línea custom-title que hay que añadir al final
// de la copia para que Desktop la importe con su nombre, o nil si no hace falta:
// sin título, o si el último custom-title de la ventana que lee Desktop ya es
// ese.
func desktopTitleLine(src []byte, title, uuid string) []byte {
	title = normalizeTitle(title)
	if title == "" {
		return nil
	}
	tail := src
	if len(tail) > desktopImportTitleWindow {
		tail = tail[len(tail)-desktopImportTitleWindow:]
	}
	if lastCustomTitle(tail) == title {
		return nil
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false) // como lo escribe Claude Code: «<» y «&» tal cual
	if err := enc.Encode(struct {
		Type        string `json:"type"`
		CustomTitle string `json:"customTitle"`
		SessionID   string `json:"sessionId"`
	}{"custom-title", title, uuid}); err != nil {
		return nil
	}
	return buf.Bytes() // Encode ya termina en '\n'
}

// lastCustomTitle devuelve el último custom-title de un trozo de JSONL. El trozo
// puede empezar a media línea: esa no se entiende como JSON y se salta.
func lastCustomTitle(chunk []byte) string {
	title := ""
	for _, line := range bytes.Split(chunk, []byte("\n")) {
		if !bytes.Contains(line, []byte(`"custom-title"`)) {
			continue
		}
		var m titleLine
		if json.Unmarshal(bytes.TrimSpace(line), &m) == nil && m.Type == "custom-title" {
			title = normalizeTitle(m.CustomTitle)
		}
	}
	return title
}

// copyDesktopCompanion copia la carpeta hermana del transcript (<uuid>/:
// transcripts de subagentes, workflows, resultados grandes de herramientas) con
// la misma regla que el transcript, archivo por archivo: lo que falta se copia,
// lo que creció en el origen se pone al día y lo que el destino tiene distinto
// se deja como está y se cuenta. Los symlinks ni se siguen ni se recrean.
func copyDesktopCompanion(srcDir, dstDir string) (copied, kept int, err error) {
	info, err := os.Lstat(srcDir)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, 0, nil
	}
	if err != nil {
		return 0, 0, err
	}
	if !info.IsDir() {
		return 0, 0, nil
	}
	err = filepath.WalkDir(srcDir, func(path string, d fs.DirEntry, werr error) error {
		if werr != nil {
			return werr
		}
		rel, rerr := filepath.Rel(srcDir, path)
		if rerr != nil {
			return rerr
		}
		dst := filepath.Join(dstDir, rel)
		if d.IsDir() {
			return os.MkdirAll(dst, 0o700)
		}
		if !d.Type().IsRegular() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		have, herr := os.ReadFile(dst)
		switch {
		case herr == nil && bytes.Equal(have, data):
			return nil
		case herr == nil && !bytes.HasPrefix(data, have):
			kept++
			return nil
		case herr != nil && !errors.Is(herr, fs.ErrNotExist):
			return herr
		}
		if err := writeFileAtomic(dst, data, 0o600); err != nil {
			return err
		}
		copied++
		return nil
	})
	return copied, kept, err
}

// LSApp es una app viva tal como la tiene registrada LaunchServices.
type LSApp struct {
	BundleID   string
	BundlePath string
	PID        int
}

// ParseLSAppInfo interpreta la salida de `lsappinfo list`: un bloque por app que
// empieza con «N) "Nombre" ASN:…» y sigue con líneas `bundleID="…"`, `bundle
// path="…"` y `pid = N …`.
func ParseLSAppInfo(out string) []LSApp {
	var apps []LSApp
	cur := -1
	for _, line := range strings.Split(out, "\n") {
		t := strings.TrimSpace(line)
		switch {
		case lsappHeaderRe.MatchString(t):
			apps = append(apps, LSApp{})
			cur = len(apps) - 1
		case cur < 0:
			continue
		case strings.HasPrefix(t, "bundleID="):
			apps[cur].BundleID = strings.Trim(strings.TrimPrefix(t, "bundleID="), `" `)
		case strings.HasPrefix(t, "bundle path="):
			apps[cur].BundlePath = strings.Trim(strings.TrimPrefix(t, "bundle path="), `" `)
		case strings.HasPrefix(t, "pid = "):
			// Sin pid el bloque sigue valiendo: lo que decide es el id y la ruta.
			if f := strings.Fields(strings.TrimPrefix(t, "pid = ")); len(f) > 0 {
				apps[cur].PID, _ = strconv.Atoi(f[0])
			}
		}
	}
	return apps
}

var lsappHeaderRe = regexp.MustCompile(`^\d+\) "`)

// Motivos por los que no es seguro mandar el enlace de importación. Estables:
// los consume --json.
const (
	DesktopImportNoLauncher   = "no_launcher"        // sin lanzador, el enlace iría al Claude principal
	DesktopImportCollapsed    = "identity_collapsed" // la ventana del perfil está registrada como el Claude principal
	DesktopImportHijacked     = "main_id_hijacked"   // otra ventana ocupa la identidad del Claude principal
	DesktopImportUnsafe       = "instance_unsafe"    // la ventana del perfil corre sin su aislamiento
	DesktopImportProbeMissing = "probe_unavailable"  // no se pudo comprobar a quién le llegaría
)

// DesktopImportInput es el estado del mundo que mira DesktopImportRoute. Las
// sondas entran como dato, igual que en el doctor, para poder montar en un test
// el estado del 2026-09-15 sin abrir ninguna ventana.
type DesktopImportInput struct {
	Profile      string
	MainApp      string      // la Claude.app del usuario: el destino de `default`
	MainBundleID string      // su CFBundleIdentifier
	Launcher     *DesktopApp // el lanzador del perfil; nil si no tiene
	DataDir      string      // --user-data-dir del perfil ("" en default)
	CCHome       string
	Procs        []DesktopProc
	ProcsOK      bool // false = `ps` no se pudo mirar
	Apps         []LSApp
	AppsOK       bool // false = `lsappinfo` no se pudo mirar
}

// DesktopImportTarget es a quién mandarle el enlace, o por qué no.
type DesktopImportTarget struct {
	App     string // el bundle al que se le manda con `open -a`
	Running bool   // su ventana ya está abierta (si no, el enlace la abre)
	Refuse  string // "" = adelante; si no, uno de DesktopImport*
	Detail  string
}

// DesktopImportRoute decide a qué app hay que mandarle claude://resume para que
// la sesión aparezca en la ventana de ESE perfil, y si es seguro hacerlo.
//
// El enlace le llega a quien macOS tenga registrado con el id de la app a la
// que se le manda. Solo es seguro si cada identidad está donde debe: si la
// ventana de un perfil se reencarnó como el Claude principal (ADR 0009), el
// enlace para `default` aterrizaría en ella y la importación se haría contra el
// cc-home equivocado. Por eso aquí, como en el doctor, «no pude comprobarlo»
// también es un no.
func DesktopImportRoute(in DesktopImportInput) DesktopImportTarget {
	switch {
	case !in.ProcsOK:
		return DesktopImportTarget{Refuse: DesktopImportProbeMissing, Detail: "ps"}
	case !in.AppsOK:
		return DesktopImportTarget{Refuse: DesktopImportProbeMissing, Detail: "lsappinfo"}
	}

	if in.Profile == "default" {
		if in.MainApp == "" {
			return DesktopImportTarget{Refuse: DesktopImportProbeMissing, Detail: "Claude.app"}
		}
		main := filepath.Clean(in.MainApp)
		mainID := in.MainBundleID
		if mainID == "" {
			mainID = desktopMainBundleID
		}
		t := DesktopImportTarget{App: main}
		for _, a := range in.Apps {
			if a.BundleID != mainID {
				continue
			}
			if filepath.Clean(a.BundlePath) != main {
				return DesktopImportTarget{Refuse: DesktopImportHijacked, Detail: a.BundlePath}
			}
			t.Running = true
		}
		// Lo mismo visto por `ps`: la app principal corriendo con el data dir de
		// un perfil comparte id con la de verdad y se quedaría el enlace.
		if DesktopForeignInstance(in.Procs, "", main) {
			return DesktopImportTarget{Refuse: DesktopImportHijacked, Detail: main}
		}
		return t
	}

	// Sin lanzador, la ventana del perfil corre como /Applications/Claude.app:
	// comparte id con el Claude principal y no hay forma de dirigirle el enlace.
	if in.Launcher == nil {
		return DesktopImportTarget{Refuse: DesktopImportNoLauncher}
	}
	t := DesktopImportTarget{App: in.Launcher.Path}
	// Dos rutas y solo dos: la del lanzador (la ventana sana) y la del espejo
	// anidado, que es donde reaparece la ventana cuando se reencarna con el id
	// del Claude principal. Los helpers de Chromium cuelgan también del
	// lanzador, pero con su propio id (…helper): contarlos daría por colapsada
	// toda ventana viva.
	launcher := filepath.Clean(in.Launcher.Path)
	mirror := filepath.Join(launcher, "Contents", filepath.FromSlash(desktopAppNestedRel))
	for _, a := range in.Apps {
		if p := filepath.Clean(a.BundlePath); p != launcher && p != mirror {
			continue
		}
		if a.BundleID != in.Launcher.Manifest.BundleID {
			return DesktopImportTarget{Refuse: DesktopImportCollapsed, Detail: a.BundleID}
		}
		t.Running = true
	}
	for _, is := range DesktopPreflight(DesktopPreflightInput{
		Profile: in.Profile, DataDir: in.DataDir, CCHome: in.CCHome,
		LauncherPath: in.Launcher.Path, Procs: in.Procs,
	}) {
		switch {
		case is.Fatal:
			// Una ventana sin CLAUDE_CONFIG_DIR, o con el de otro perfil, buscaría
			// el transcript en otro cc-home y crearía la entrada en la cuenta
			// equivocada.
			return DesktopImportTarget{Refuse: DesktopImportUnsafe, Detail: is.Code}
		case is.Code == DesktopIssueAlreadyRunning:
			t.Running = true
		}
	}
	return t
}
