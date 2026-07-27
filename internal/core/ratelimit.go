package core

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// ratelimit.go — parsers PUROS de las señales de límite de Claude Code.
//
// Hay cuatro fuentes con formatos distintos y ninguna es estable por contrato
// (son detalles internos de CC, verificados contra 2.1.220):
//
//   - transcript JSONL: la línea de error del turno que falló (reactivo).
//   - stream-json de `claude -p`: eventos `api_retry` (reactivo, headless).
//   - stdin del statusLine: `rate_limits` con % y `resets_at` epoch (proactivo).
//   - <ccHome>/.claude.json → `cachedUsageUtilization` (respaldo del anterior).
//
// De ahí la obsesión de este archivo con la tolerancia: cada parser acepta
// variantes de nombre de clave, tipos laxos (números como strings), anidamientos
// y basura, y NUNCA panica ni devuelve datos inventados. La regla es
// «entrada que no entiendo => (zero, false)»: el supervisor prefiere quedarse
// ciego a rotar de perfil por un falso positivo, que es una acción destructiva
// (mueve la conversación del usuario a otra cuenta).

// LimitWindow es el tipo de ventana de consumo que se agotó. El nombre de la
// ventana NO viene en ningún campo estructurado: solo en la prosa inglesa del
// mensaje de error, por eso hace falta ClassifyLimitText.
type LimitWindow string

const (
	WindowSession LimitWindow = "session" // five_hour
	WindowWeekly  LimitWindow = "weekly"  // seven_day
	WindowOpus    LimitWindow = "opus"
	WindowCredit  LimitWindow = "credit"
	WindowUnknown LimitWindow = "unknown"
)

// LimitEvent es la detección normalizada que consume el supervisor. Los tags
// json existen porque el evento se serializa tal cual dentro del sentinel que
// deja el hook (ver autostate.go): el formato en disco es este.
type LimitEvent struct {
	Window   LimitWindow `json:"window"`
	ResetsAt time.Time   `json:"resets_at,omitempty"` // cero si se desconoce
	Source   string      `json:"source"`              // "transcript"|"stream-json"|"statusline"|"hook"|"exit"
	Detail   string      `json:"detail,omitempty"`
}

// detailMax recorta el texto que se guarda en Detail. El mensaje de CC puede
// arrastrar un bloque largo (opciones de /rate-limit-options, enlaces); el
// sentinel se escribe en disco desde un hook y no queremos archivos de KBs por
// cada turno fallido.
const detailMax = 240

// apostropheFolder normaliza las variantes de apóstrofo a la recta. CC emite
// «You’ve hit your …» con U+2019 tipográfico en unas rutas y con ' recto en
// otras: sin plegarlas, media detección por texto se pierde.
var apostropheFolder = strings.NewReplacer("’", "'", "ʼ", "'", "‘", "'", "`", "'")

// foldText baja a minúsculas y pliega apóstrofos: toda comparación de prosa de
// este archivo pasa por aquí para ser case-insensitive y tipografía-insensible.
func foldText(s string) string {
	return apostropheFolder.Replace(strings.ToLower(s))
}

// mentionsHitLimit reconoce la frase canónica «You've hit your <X> limit».
// Es una señal DÉBIL a propósito: solo se usa como refuerzo cuando la línea ya
// se declaró error (isApiErrorMessage), nunca por sí sola, porque un mensaje
// del asistente puede citar la frase sin que haya límite alguno.
func mentionsHitLimit(s string) bool {
	l := foldText(s)
	return strings.Contains(l, "hit your") && strings.Contains(l, "limit")
}

// ClassifyLimitText mapea el texto de CC ("You've hit your weekly limit…") a
// ventana.
//
// El orden de las comprobaciones ES la política de precedencia y no es
// arbitrario: «You've hit your weekly Opus limit» menciona dos ventanas, y la
// de Opus es la específica (la weekly general puede seguir viva), así que Opus
// gana. El crédito de uso va después porque es un cubo aparte del plan, y solo
// al final se distinguen weekly y session, que son las genéricas.
func ClassifyLimitText(s string) LimitWindow {
	l := foldText(s)
	switch {
	case strings.Contains(l, "opus"):
		return WindowOpus
	case strings.Contains(l, "usage credit"):
		return WindowCredit
	case containsAny(l, "weekly", "7-day", "7 day", "seven-day", "seven day"):
		return WindowWeekly
	case containsAny(l, "session limit", "5-hour", "5 hour", "five-hour", "five hour"):
		return WindowSession
	}
	return WindowUnknown
}

// containsAny es el `strings.Contains` sobre varias agujas, para que los switch
// de arriba se lean como la tabla de sinónimos que son.
func containsAny(hay string, needles ...string) bool {
	for _, n := range needles {
		if strings.Contains(hay, n) {
			return true
		}
	}
	return false
}

// rawLimitLine es la vista laxa de una línea JSON de cualquiera de las dos
// fuentes de eventos. Todo lo que no es string va como RawMessage porque los
// tipos reales varían entre versiones de CC (429 como número o como "429") y un
// json.Unmarshal estricto tiraría la línea entera por un campo secundario.
type rawLimitLine struct {
	Type              string          `json:"type"`
	Subtype           string          `json:"subtype"`
	IsAPIErrorMessage json.RawMessage `json:"isApiErrorMessage"`
	APIErrorStatus    json.RawMessage `json:"apiErrorStatus"`
	ErrorStatus       json.RawMessage `json:"error_status"`
	Error             json.RawMessage `json:"error"`
	Message           json.RawMessage `json:"message"`
	Content           json.RawMessage `json:"content"`
	Text              json.RawMessage `json:"text"`
	ResetsAt          json.RawMessage `json:"resets_at"`
	ResetsAtCamel     json.RawMessage `json:"resetsAt"`
}

// jsonStr devuelve el valor si el RawMessage es una cadena JSON.
func jsonStr(raw json.RawMessage) (string, bool) {
	if len(raw) == 0 {
		return "", false
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return "", false
	}
	return s, true
}

// jsonNum acepta número JSON o número dentro de una cadena ("429"), porque los
// hooks de CC reenvían a veces el status ya stringificado.
func jsonNum(raw json.RawMessage) (float64, bool) {
	if len(raw) == 0 {
		return 0, false
	}
	var f float64
	if err := json.Unmarshal(raw, &f); err == nil {
		return f, true
	}
	if s, ok := jsonStr(raw); ok {
		if f, err := strconv.ParseFloat(strings.TrimSpace(s), 64); err == nil {
			return f, true
		}
	}
	return 0, false
}

// jsonBool acepta true/false y sus formas en cadena.
func jsonBool(raw json.RawMessage) (bool, bool) {
	if len(raw) == 0 {
		return false, false
	}
	var b bool
	if err := json.Unmarshal(raw, &b); err == nil {
		return b, true
	}
	if s, ok := jsonStr(raw); ok {
		if b, err := strconv.ParseBool(strings.TrimSpace(s)); err == nil {
			return b, true
		}
	}
	return false, false
}

// errorFacts extrae (clase, mensaje, status) del campo `error`, que aparece en
// tres formas distintas según la fuente:
//
//	"error":"rate_limit"
//	"error":{"type":"rate_limit","message":"You've hit…"}
//	"error":{"error":"rate_limit","error_status":429}   ← stream-json anidado
//
// La recursión está acotada (depth) para que un JSON hostil con anidamiento
// profundo no consuma pila: este parser corre sobre datos de terceros.
func errorFacts(raw json.RawMessage, depth int) (kind, text string, status int) {
	if len(raw) == 0 || depth > 4 {
		return "", "", 0
	}
	if s, ok := jsonStr(raw); ok {
		return s, "", 0
	}
	var obj map[string]json.RawMessage
	if json.Unmarshal(raw, &obj) != nil {
		return "", "", 0
	}
	for _, k := range []string{"type", "code", "reason", "name"} {
		if s, ok := jsonStr(obj[k]); ok && s != "" {
			kind = s
			break
		}
	}
	if s, ok := jsonStr(obj["message"]); ok {
		text = s
	}
	for _, k := range []string{"error_status", "status", "apiErrorStatus", "status_code"} {
		if f, ok := jsonNum(obj[k]); ok {
			status = int(f)
			break
		}
	}
	if nested, ok := obj["error"]; ok {
		nk, nt, ns := errorFacts(nested, depth+1)
		if kind == "" {
			kind = nk
		}
		if text == "" {
			text = nt
		}
		if status == 0 {
			status = ns
		}
	}
	return kind, text, status
}

// isRateLimitKind reconoce solo la clase `rate_limit` del enum de CC
// (authentication_failed, billing_error, overloaded, server_error, …). El resto
// son fallos que NO se arreglan cambiando de cuenta: rotar ante un
// `overloaded` movería la sesión sin motivo y consumiría un hop del presupuesto.
func isRateLimitKind(kind string) bool {
	k := strings.ToLower(strings.TrimSpace(kind))
	k = strings.NewReplacer("-", "_", " ", "_").Replace(k)
	switch k {
	case "rate_limit", "rate_limited", "ratelimit", "rate_limit_error", "rate_limit_exceeded":
		return true
	}
	return false
}

// extractText recompone el texto humano de la línea. El mensaje puede venir como
// cadena suelta, como `message.content` (array de bloques {type,text} del API de
// Anthropic) o como `content` a pelo; se recorre todo hasta dar con prosa.
func extractText(raw json.RawMessage, depth int) string {
	if len(raw) == 0 || depth > 5 {
		return ""
	}
	if s, ok := jsonStr(raw); ok {
		return s
	}
	var arr []json.RawMessage
	if json.Unmarshal(raw, &arr) == nil {
		var parts []string
		for _, e := range arr {
			if t := extractText(e, depth+1); t != "" {
				parts = append(parts, t)
			}
		}
		return strings.Join(parts, "\n")
	}
	var obj map[string]json.RawMessage
	if json.Unmarshal(raw, &obj) == nil {
		for _, k := range []string{"text", "content", "message"} {
			if t := extractText(obj[k], depth+1); t != "" {
				return t
			}
		}
	}
	return ""
}

// ExtractProse expone el extractor de prosa a los llamadores de fuera de core
// (el hook StopFailure del CLI, que recibe payloads con el error como objeto).
// Existe para que nadie tenga que volcar JSON crudo como si fuera texto: el
// volcado arrastra identificadores y contadores que luego se leen como si fueran
// el mensaje del error.
func ExtractProse(raw json.RawMessage) string { return extractText(raw, 0) }

// lineText junta los sitios donde puede estar la prosa de una línea.
func (l rawLimitLine) lineText() string {
	for _, raw := range []json.RawMessage{l.Text, l.Message, l.Content} {
		if t := extractText(raw, 0); t != "" {
			return t
		}
	}
	return ""
}

// resetsAtField lee `resets_at`/`resetsAt` de la raíz de la línea si existiera.
func (l rawLimitLine) resetsAtField() time.Time {
	if t := parseResetsAt(l.ResetsAt); !t.IsZero() {
		return t
	}
	return parseResetsAt(l.ResetsAtCamel)
}

// decodeLimitLine hace el trabajo común de las dos fuentes de eventos: decodifica
// la línea y reúne los hechos que deciden si es un límite. Devuelve ok=false ante
// cualquier cosa que no sea un objeto JSON (líneas vacías, arrays, basura).
func decodeLimitLine(line []byte) (l rawLimitLine, kind, text string, status int, isErr bool, ok bool) {
	b := bytes.TrimSpace(line)
	// El descarte por primer byte evita gastar el decodificador en las muchas
	// líneas no-JSON que puede haber en un stream (banners, logs sueltos).
	if len(b) == 0 || b[0] != '{' {
		return l, "", "", 0, false, false
	}
	if json.Unmarshal(b, &l) != nil {
		return l, "", "", 0, false, false
	}
	kind, errText, errStatus := errorFacts(l.Error, 0)
	status = errStatus
	for _, raw := range []json.RawMessage{l.APIErrorStatus, l.ErrorStatus} {
		if status != 0 {
			break
		}
		if f, ok := jsonNum(raw); ok {
			status = int(f)
		}
	}
	text = l.lineText()
	if text == "" {
		text = errText
	}
	isErr, _ = jsonBool(l.IsAPIErrorMessage)
	return l, kind, text, status, isErr, true
}

// isLimit aplica la política de detección compartida. El status 429 es
// autoritativo por sí solo (HTTP no lo usa para otra cosa en esta API), la clase
// `rate_limit` también, y la frase solo cuenta si la línea ya se marcó como
// error — ver mentionsHitLimit.
func isLimit(kind string, status int, isErr bool, text string) bool {
	switch {
	case isRateLimitKind(kind):
		return true
	case status == 429:
		return true
	case isErr && mentionsHitLimit(text):
		return true
	}
	return false
}

// buildLimitEvent normaliza los hechos en el evento que consume el supervisor.
// `rawLine` es el respaldo para clasificar la ventana cuando no se pudo aislar
// la prosa: la frase con el nombre de la ventana está en algún sitio de la línea
// y perder el tipo de ventana degrada la decisión de a qué perfil saltar.
func buildLimitEvent(source, text string, rawLine []byte, resets time.Time, kind string) LimitEvent {
	classifySrc := text
	if classifySrc == "" {
		classifySrc = string(rawLine)
	}
	detail := strings.TrimSpace(text)
	if detail == "" {
		detail = kind
	}
	if len(detail) > detailMax {
		detail = detail[:detailMax]
	}
	return LimitEvent{
		Window:   ClassifyLimitText(classifySrc),
		ResetsAt: resets,
		Source:   source,
		Detail:   detail,
	}
}

// ParseTranscriptLine detecta rate limit en una línea JSONL del transcript:
// isApiErrorMessage==true && apiErrorStatus==429, o error=="rate_limit".
//
// Desviación deliberada del contrato: apiErrorStatus==429 basta AUNQUE falte
// isApiErrorMessage. Ese status no significa otra cosa en esta API y hay
// variantes de línea (type "system") que no traen el flag; exigir la conjunción
// dejaría ciega la detección justo en la fuente que sí funciona en interactive.
func ParseTranscriptLine(line []byte) (LimitEvent, bool) {
	l, kind, text, status, isErr, ok := decodeLimitLine(line)
	if !ok || !isLimit(kind, status, isErr, text) {
		return LimitEvent{}, false
	}
	return buildLimitEvent("transcript", text, line, l.resetsAtField(), kind), true
}

// ParseStreamJSONLine idem para el stream-json de `claude -p`
// (type=="api_retry" o system/error con error=="rate_limit" / error_status==429).
//
// Desviación deliberada: un `api_retry` SIN evidencia de límite (sin
// error=="rate_limit" ni 429) NO se reporta. `api_retry` también se emite por
// `overloaded` o cortes de red, que se resuelven solos con el reintento; rotar
// de perfil ahí sería mover la conversación del usuario por nada.
func ParseStreamJSONLine(line []byte) (LimitEvent, bool) {
	l, kind, text, status, isErr, ok := decodeLimitLine(line)
	if !ok {
		return LimitEvent{}, false
	}
	// En el stream el «esto es un error» no viaja como isApiErrorMessage sino
	// en el propio tipo del evento, así que se deriva de ahí para que la señal
	// por frase (mentionsHitLimit) siga disponible.
	streamErr := isErr || isErrorishType(l.Type) || isErrorishType(l.Subtype)
	if !isLimit(kind, status, streamErr, text) {
		return LimitEvent{}, false
	}
	return buildLimitEvent("stream-json", text, line, l.resetsAtField(), kind), true
}

// isErrorishType reconoce los tipos/subtipos de evento del stream que denotan
// fallo, y por tanto habilitan la detección por frase.
func isErrorishType(t string) bool {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case "api_retry", "error", "api_error", "system":
		return true
	}
	return false
}

// Windowed es una ventana de consumo con su porcentaje y su reset.
type Windowed struct {
	UsedPercentage float64   `json:"used_percentage"`
	ResetsAt       time.Time `json:"resets_at,omitempty"`
}

// HasData distingue «0% usado» de «sin dato».
//
// Importa porque hay un bug conocido (CC 2.1.220): five_hour.utilization lee 0
// en cachés poblados mientras seven_day lee 83. Una ventana con 0% y sin
// resets_at es indistinguible de un struct recién inicializado, así que se trata
// como ausencia de medida en vez de como «la ventana está libre» — lo contrario
// haría que Exhausted diera un veredicto tranquilizador basado en nada.
func (w Windowed) HasData() bool {
	return w.UsedPercentage != 0 || !w.ResetsAt.IsZero()
}

// RateLimits es la muestra de las dos ventanas de suscripción.
type RateLimits struct {
	FiveHour Windowed `json:"five_hour"`
	SevenDay Windowed `json:"seven_day"`
}

// ExhaustedAt reporta si alguna ventana supera el umbral (%) EN `now`.
//
// Las ventanas sin dato (ver Windowed.HasData) se ignoran. Si las dos superan el
// umbral gana la de mayor porcentaje, y en empate la de sesión, que es la que
// antes se libera y por tanto la que da la mejor pista de cooldown.
//
// El reloj entra por parámetro porque una muestra puede describir una ventana
// que YA reabrió: un `.claude.json` de ayer con five_hour al 100% y resets_at a
// la 1am de esta madrugada mide una ventana caducada, y leerla como «perfil
// agotado» abandona una cuenta disponible y la manda a la nevera (el cooldown
// del Chain descarta el resets_at pasado y aplica la hora de respaldo). Una
// ventana con resets_at no-cero que ya pasó es dato caducado, no agotamiento.
// Sin resets_at no hay nada que caducar y manda el porcentaje.
func (r RateLimits) ExhaustedAt(threshold int, now time.Time) (LimitWindow, bool) {
	win := WindowUnknown
	best := -1.0
	found := false
	check := func(w Windowed, lw LimitWindow) {
		if !w.HasData() || w.UsedPercentage < float64(threshold) {
			return
		}
		if !w.ResetsAt.IsZero() && !now.IsZero() && !w.ResetsAt.After(now) {
			return
		}
		if w.UsedPercentage > best {
			best, win, found = w.UsedPercentage, lw, true
		}
	}
	check(r.FiveHour, WindowSession)
	check(r.SevenDay, WindowWeekly)
	return win, found
}

// parseResetsAt acepta las dos codificaciones que usa CC para el mismo dato:
// epoch en segundos (statusLine) e ISO-8601 (.claude.json). Se distinguen por
// tipo JSON, y dentro del número por magnitud: por encima de 1e12 solo puede ser
// milisegundos (1e12 s son 33 milenios). Un valor <= 0 es «sin dato».
func parseResetsAt(raw json.RawMessage) time.Time {
	if len(raw) == 0 {
		return time.Time{}
	}
	if s, ok := jsonStr(raw); ok {
		return parseResetsAtString(s)
	}
	f, ok := jsonNum(raw)
	if !ok || f <= 0 {
		return time.Time{}
	}
	return epochToTime(f)
}

// epochToTime convierte segundos (o milisegundos) epoch a UTC.
func epochToTime(f float64) time.Time {
	if f >= 1e12 {
		return time.UnixMilli(int64(f)).UTC()
	}
	sec := int64(f)
	nsec := int64((f - float64(sec)) * 1e9)
	return time.Unix(sec, nsec).UTC()
}

// parseResetsAtString cubre ISO-8601 con y sin zona, y el epoch stringificado.
func parseResetsAtString(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05", "2006-01-02 15:04:05"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC()
		}
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil && f > 0 {
		return epochToTime(f)
	}
	return time.Time{}
}

// windowPctKeys y windowResetKeys son los sinónimos vistos entre el statusLine
// (used_percentage) y el caché de .claude.json (utilization).
var windowPctKeys = []string{"used_percentage", "usedPercentage", "utilization", "used", "percent", "percentage"}
var windowResetKeys = []string{"resets_at", "resetsAt", "reset_at", "resetAt", "resets"}

// parseWindow lee una ventana suelta. ok=false si no se reconoció ni el
// porcentaje ni el reset: devolver una Windowed vacía «válida» sería inventarse
// un 0% que el supervisor leería como cuenta fresca.
func parseWindow(raw json.RawMessage) (Windowed, bool) {
	if len(raw) == 0 {
		return Windowed{}, false
	}
	var obj map[string]json.RawMessage
	if json.Unmarshal(raw, &obj) != nil {
		return Windowed{}, false
	}
	var w Windowed
	ok := false
	for _, k := range windowPctKeys {
		if f, got := jsonNum(obj[k]); got {
			w.UsedPercentage, ok = f, true
			break
		}
	}
	for _, k := range windowResetKeys {
		if t := parseResetsAt(obj[k]); !t.IsZero() {
			w.ResetsAt, ok = t, true
			break
		}
	}
	return w, ok
}

// windowNameKind clasifica el nombre de una ventana en la forma de lista
// ({"windows":[{"name":"five_hour",…}]}).
func windowNameKind(name string) LimitWindow {
	n := strings.ToLower(strings.TrimSpace(name))
	switch {
	case containsAny(n, "seven", "7d", "7_day", "7-day", "week"):
		return WindowWeekly
	case containsAny(n, "five", "5h", "5_hour", "5-hour", "hour", "session"):
		return WindowSession
	}
	return WindowUnknown
}

// parseWindowSet lee el nodo que contiene las dos ventanas. Acepta las tres
// formas observadas: snake_case, camelCase y lista bajo "windows". Si no
// reconoce ninguna devuelve ok=false — el contrato dice explícitamente que ante
// lo desconocido NO se devuelvan ceros, porque un 0% falso es peor que no saber.
func parseWindowSet(raw json.RawMessage) (RateLimits, bool) {
	if len(raw) == 0 {
		return RateLimits{}, false
	}
	var obj map[string]json.RawMessage
	if json.Unmarshal(raw, &obj) != nil {
		return RateLimits{}, false
	}
	var rl RateLimits
	ok := false
	for _, k := range []string{"five_hour", "fiveHour", "5h", "session"} {
		if w, got := parseWindow(obj[k]); got {
			rl.FiveHour, ok = w, true
			break
		}
	}
	for _, k := range []string{"seven_day", "sevenDay", "7d", "weekly"} {
		if w, got := parseWindow(obj[k]); got {
			rl.SevenDay, ok = w, true
			break
		}
	}
	if ok {
		return rl, true
	}
	// Forma de lista: cada elemento se autodescribe con un nombre.
	var list []json.RawMessage
	if err := json.Unmarshal(obj["windows"], &list); err != nil {
		return RateLimits{}, false
	}
	for _, e := range list {
		var eo map[string]json.RawMessage
		if json.Unmarshal(e, &eo) != nil {
			continue
		}
		name := ""
		for _, k := range []string{"name", "window", "type", "id"} {
			if s, got := jsonStr(eo[k]); got && s != "" {
				name = s
				break
			}
		}
		w, got := parseWindow(e)
		if !got {
			continue
		}
		switch windowNameKind(name) {
		case WindowSession:
			rl.FiveHour, ok = w, true
		case WindowWeekly:
			rl.SevenDay, ok = w, true
		}
	}
	return rl, ok
}

// ParseStatusLineInput extrae rate_limits del JSON que CC pasa por stdin al
// statusLine (resets_at en epoch seconds).
//
// Si no hay nodo `rate_limits` se intenta con la raíz: así el mismo parser sirve
// para el stdin completo del statusLine y para un fragmento ya recortado.
func ParseStatusLineInput(data []byte) (RateLimits, bool) {
	var obj map[string]json.RawMessage
	if json.Unmarshal(bytes.TrimSpace(data), &obj) != nil {
		return RateLimits{}, false
	}
	for _, k := range []string{"rate_limits", "rateLimits"} {
		if node, got := obj[k]; got {
			return parseWindowSet(node)
		}
	}
	return parseWindowSet(json.RawMessage(bytes.TrimSpace(data)))
}

// ReadCachedUsage lee <ccHome>/.claude.json -> .cachedUsageUtilization
// (utilization %, resets_at ISO-8601). Es el respaldo cuando no hay muestra del
// statusLine (perfiles sin la capa de sensores, o CC recién arrancado).
func ReadCachedUsage(ccHome string) (RateLimits, bool) {
	if strings.TrimSpace(ccHome) == "" {
		return RateLimits{}, false
	}
	data, err := os.ReadFile(filepath.Join(ccHome, ".claude.json"))
	if err != nil {
		return RateLimits{}, false
	}
	var obj map[string]json.RawMessage
	if json.Unmarshal(bytes.TrimSpace(data), &obj) != nil {
		return RateLimits{}, false
	}
	for _, k := range []string{"cachedUsageUtilization", "cached_usage_utilization"} {
		if node, got := obj[k]; got {
			return parseWindowSet(node)
		}
	}
	return RateLimits{}, false
}
