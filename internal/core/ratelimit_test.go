package core

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Fixtures tomadas de líneas reales de Claude Code 2.1.220 (ver spec §Señales
// de detección). Las variantes hostiles están para fijar la invariante de que
// ningún parser panica ni inventa datos.

func TestClassifyLimitText(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want LimitWindow
	}{
		{"sesión explícita", "You've hit your session limit", WindowSession},
		{"5-hour", "You've hit your 5-hour limit · resets 3pm", WindowSession},
		{"five hour en prosa", "the five hour window is exhausted", WindowSession},
		{"weekly", "You've hit your weekly limit", WindowWeekly},
		{"7-day", "Your 7-day usage is at 100%", WindowWeekly},
		{"opus gana a weekly", "You've hit your weekly Opus limit", WindowOpus},
		{"crédito", "You've hit your usage credit limit", WindowCredit},
		{"mayúsculas", "YOU'VE HIT YOUR WEEKLY LIMIT", WindowWeekly},
		// El apóstrofo tipográfico U+2019 aparece en la mitad de las rutas de
		// CC; si no se pliega, la clasificación por prosa se pierde.
		{"apóstrofo tipográfico", "You’ve hit your session limit", WindowSession},
		{"sin nada reconocible", "internal server error", WindowUnknown},
		{"vacío", "", WindowUnknown},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ClassifyLimitText(tc.in); got != tc.want {
				t.Fatalf("ClassifyLimitText(%q) = %q, quería %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestParseTranscriptLine(t *testing.T) {
	cases := []struct {
		name       string
		line       string
		wantOK     bool
		wantWindow LimitWindow
		wantDetail string
	}{
		{
			name:       "assistant con isApiErrorMessage y 429",
			line:       `{"type":"assistant","isApiErrorMessage":true,"apiErrorStatus":429,"error":"rate_limit","message":{"content":[{"type":"text","text":"You've hit your 5-hour limit"}]}}`,
			wantOK:     true,
			wantWindow: WindowSession,
			wantDetail: "You've hit your 5-hour limit",
		},
		{
			name:       "system con error rate_limit y apóstrofo tipográfico",
			line:       "{\"type\":\"system\",\"isApiErrorMessage\":true,\"apiErrorStatus\":429,\"error\":\"rate_limit\",\"content\":\"You’ve hit your weekly limit\"}",
			wantOK:     true,
			wantWindow: WindowWeekly,
		},
		{
			name:       "error como objeto anidado",
			line:       `{"type":"assistant","isApiErrorMessage":true,"error":{"type":"rate_limit","message":"You've hit your usage credit limit"}}`,
			wantOK:     true,
			wantWindow: WindowCredit,
		},
		{
			name:       "status 429 como string",
			line:       `{"type":"assistant","isApiErrorMessage":"true","apiErrorStatus":"429","message":"You've hit your weekly limit"}`,
			wantOK:     true,
			wantWindow: WindowWeekly,
		},
		{
			name:       "solo la frase, con la línea marcada como error",
			line:       `{"type":"assistant","isApiErrorMessage":true,"message":"You've hit your Opus limit"}`,
			wantOK:     true,
			wantWindow: WindowOpus,
		},
		{
			name:       "sin ventana reconocible sigue siendo límite",
			line:       `{"type":"assistant","isApiErrorMessage":true,"apiErrorStatus":429,"error":"rate_limit"}`,
			wantOK:     true,
			wantWindow: WindowUnknown,
		},
		// El resto del enum de .error NO son límites: rotar de cuenta ante un
		// overloaded movería la conversación del usuario sin arreglar nada.
		{name: "overloaded", line: `{"type":"assistant","isApiErrorMessage":true,"apiErrorStatus":529,"error":"overloaded","message":"Overloaded"}`},
		{name: "billing_error", line: `{"type":"assistant","isApiErrorMessage":true,"error":"billing_error","message":"Credit balance too low"}`},
		{name: "authentication_failed", line: `{"type":"assistant","isApiErrorMessage":true,"error":"authentication_failed"}`},
		{name: "max_output_tokens", line: `{"type":"assistant","isApiErrorMessage":true,"error":"max_output_tokens"}`},
		{name: "línea normal del asistente", line: `{"type":"assistant","message":{"content":[{"type":"text","text":"hola"}]}}`},
		// La frase sin marca de error no basta: el asistente puede citarla.
		{name: "frase citada sin error", line: `{"type":"assistant","message":"cuando You've hit your weekly limit, cambia de perfil"}`},
		{name: "json basura", line: `{"type":`},
		{name: "no es json", line: `esto no es json`},
		{name: "array json", line: `["rate_limit",429]`},
		{name: "string json", line: `"rate_limit"`},
		{name: "vacío", line: ``},
		{name: "solo espacios", line: "   \t "},
		{name: "null", line: `null`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ev, ok := ParseTranscriptLine([]byte(tc.line))
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, quería %v (ev=%+v)", ok, tc.wantOK, ev)
			}
			if !ok {
				return
			}
			if ev.Window != tc.wantWindow {
				t.Fatalf("Window = %q, quería %q", ev.Window, tc.wantWindow)
			}
			if ev.Source != "transcript" {
				t.Fatalf("Source = %q, quería transcript", ev.Source)
			}
			if tc.wantDetail != "" && ev.Detail != tc.wantDetail {
				t.Fatalf("Detail = %q, quería %q", ev.Detail, tc.wantDetail)
			}
		})
	}
}

func TestParseStreamJSONLine(t *testing.T) {
	cases := []struct {
		name       string
		line       string
		wantOK     bool
		wantWindow LimitWindow
	}{
		{
			name:       "api_retry plano",
			line:       `{"type":"api_retry","error":"rate_limit","error_status":429,"message":"You've hit your session limit"}`,
			wantOK:     true,
			wantWindow: WindowSession,
		},
		{
			name:       "api_retry con error anidado como objeto",
			line:       `{"type":"api_retry","error":{"error":"rate_limit","error_status":429,"message":"You've hit your weekly limit"}}`,
			wantOK:     true,
			wantWindow: WindowWeekly,
		},
		{
			name:       "evento system con 429",
			line:       `{"type":"system","subtype":"api_error","error_status":429,"message":"You've hit your 7-day limit"}`,
			wantOK:     true,
			wantWindow: WindowWeekly,
		},
		{
			name:       "result con error rate_limit",
			line:       `{"type":"result","subtype":"error_during_execution","error":"rate_limit"}`,
			wantOK:     true,
			wantWindow: WindowUnknown,
		},
		// Desviación documentada: api_retry SIN evidencia de límite no cuenta
		// (también se emite por overloaded y por cortes de red).
		{name: "api_retry sin evidencia", line: `{"type":"api_retry","attempt":1,"delay_ms":2000}`},
		{name: "api_retry por overloaded", line: `{"type":"api_retry","error":"overloaded","error_status":529}`},
		{name: "evento normal", line: `{"type":"assistant","message":{"content":[{"type":"text","text":"hola"}]}}`},
		{name: "json basura", line: `{{{`},
		{name: "vacío", line: ``},
		{name: "numero suelto", line: `429`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ev, ok := ParseStreamJSONLine([]byte(tc.line))
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, quería %v (ev=%+v)", ok, tc.wantOK, ev)
			}
			if !ok {
				return
			}
			if ev.Window != tc.wantWindow {
				t.Fatalf("Window = %q, quería %q", ev.Window, tc.wantWindow)
			}
			if ev.Source != "stream-json" {
				t.Fatalf("Source = %q, quería stream-json", ev.Source)
			}
		})
	}
}

func TestRateLimitsExhausted(t *testing.T) {
	reset := time.Date(2026, 7, 25, 18, 0, 0, 0, time.UTC)
	cases := []struct {
		name       string
		rl         RateLimits
		threshold  int
		wantWindow LimitWindow
		wantOK     bool
	}{
		{
			name:       "five_hour por encima",
			rl:         RateLimits{FiveHour: Windowed{UsedPercentage: 93.2, ResetsAt: reset}},
			threshold:  90,
			wantWindow: WindowSession,
			wantOK:     true,
		},
		{
			name:       "seven_day por encima",
			rl:         RateLimits{SevenDay: Windowed{UsedPercentage: 99, ResetsAt: reset}},
			threshold:  90,
			wantWindow: WindowWeekly,
			wantOK:     true,
		},
		{
			name:       "las dos: gana la mayor",
			rl:         RateLimits{FiveHour: Windowed{UsedPercentage: 91, ResetsAt: reset}, SevenDay: Windowed{UsedPercentage: 97, ResetsAt: reset}},
			threshold:  90,
			wantWindow: WindowWeekly,
			wantOK:     true,
		},
		{
			name:       "empate: gana la de sesión",
			rl:         RateLimits{FiveHour: Windowed{UsedPercentage: 95, ResetsAt: reset}, SevenDay: Windowed{UsedPercentage: 95, ResetsAt: reset}},
			threshold:  90,
			wantWindow: WindowSession,
			wantOK:     true,
		},
		{
			name:      "por debajo",
			rl:        RateLimits{FiveHour: Windowed{UsedPercentage: 40, ResetsAt: reset}},
			threshold: 90,
		},
		// El bug conocido de CC: five_hour.utilization lee 0 con la ventana
		// cargada. Sin resets_at eso es «sin dato», no «0% usado».
		{
			name:      "ventana sin dato con umbral 0 no dispara",
			rl:        RateLimits{FiveHour: Windowed{}},
			threshold: 0,
		},
		{
			name:       "0% CON resets_at sí es dato",
			rl:         RateLimits{FiveHour: Windowed{UsedPercentage: 0, ResetsAt: reset}},
			threshold:  0,
			wantWindow: WindowSession,
			wantOK:     true,
		},
		{
			name:      "muestra vacía",
			rl:        RateLimits{},
			threshold: 90,
		},
	}
	// Instante de referencia ANTERIOR al reset: todas las ventanas de la tabla
	// están vivas, que es lo que quiere medir este caso.
	now := reset.Add(-time.Hour)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w, ok := tc.rl.ExhaustedAt(tc.threshold, now)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, quería %v", ok, tc.wantOK)
			}
			if ok && w != tc.wantWindow {
				t.Fatalf("window = %q, quería %q", w, tc.wantWindow)
			}
		})
	}
}

func TestParseStatusLineInput(t *testing.T) {
	epoch := int64(1753499999)
	cases := []struct {
		name      string
		in        string
		wantOK    bool
		wantFive  float64
		wantSeven float64
		wantReset time.Time
	}{
		{
			name:      "payload real del statusLine",
			in:        `{"session_id":"abc","model":{"id":"opus"},"rate_limits":{"five_hour":{"used_percentage":93.2,"resets_at":1753499999},"seven_day":{"used_percentage":13,"resets_at":1753499999}}}`,
			wantOK:    true,
			wantFive:  93.2,
			wantSeven: 13,
			wantReset: time.Unix(epoch, 0).UTC(),
		},
		{
			name:      "camelCase",
			in:        `{"rateLimits":{"fiveHour":{"usedPercentage":50,"resetsAt":1753499999},"sevenDay":{"usedPercentage":10,"resetsAt":1753499999}}}`,
			wantOK:    true,
			wantFive:  50,
			wantSeven: 10,
			wantReset: time.Unix(epoch, 0).UTC(),
		},
		{
			name:     "solo five_hour",
			in:       `{"rate_limits":{"five_hour":{"used_percentage":88,"resets_at":1753499999}}}`,
			wantOK:   true,
			wantFive: 88,
		},
		{
			name:      "nodo raíz sin envoltorio",
			in:        `{"five_hour":{"used_percentage":70,"resets_at":1753499999}}`,
			wantOK:    true,
			wantFive:  70,
			wantReset: time.Unix(epoch, 0).UTC(),
		},
		{name: "sin rate_limits", in: `{"session_id":"abc","model":{"id":"opus"}}`},
		{name: "rate_limits vacío", in: `{"rate_limits":{}}`},
		{name: "forma desconocida", in: `{"rate_limits":{"quincenal":{"pct":10}}}`},
		{name: "json basura", in: `{"rate_limits":`},
		{name: "no es json", in: `hola`},
		{name: "vacío", in: ``},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rl, ok := ParseStatusLineInput([]byte(tc.in))
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, quería %v (rl=%+v)", ok, tc.wantOK, rl)
			}
			if !ok {
				// Nunca se inventan ceros: sin dato reconocible, muestra cero.
				if rl != (RateLimits{}) {
					t.Fatalf("con ok=false se esperaba muestra cero, hay %+v", rl)
				}
				return
			}
			if rl.FiveHour.UsedPercentage != tc.wantFive {
				t.Fatalf("five_hour = %v, quería %v", rl.FiveHour.UsedPercentage, tc.wantFive)
			}
			if rl.SevenDay.UsedPercentage != tc.wantSeven {
				t.Fatalf("seven_day = %v, quería %v", rl.SevenDay.UsedPercentage, tc.wantSeven)
			}
			if !tc.wantReset.IsZero() && !rl.FiveHour.ResetsAt.Equal(tc.wantReset) {
				t.Fatalf("resets_at = %v, quería %v", rl.FiveHour.ResetsAt, tc.wantReset)
			}
		})
	}
}

func TestReadCachedUsage(t *testing.T) {
	iso := "2026-07-25T18:00:00Z"
	want := time.Date(2026, 7, 25, 18, 0, 0, 0, time.UTC)
	cases := []struct {
		name      string
		file      string // "" => no se crea .claude.json
		wantOK    bool
		wantFive  float64
		wantSeven float64
	}{
		{
			name:      "forma real snake_case con ISO-8601",
			file:      `{"cachedUsageUtilization":{"five_hour":{"utilization":0,"resets_at":"` + iso + `","severity":"normal"},"seven_day":{"utilization":83,"resets_at":"` + iso + `","severity":"warning"}}}`,
			wantOK:    true,
			wantSeven: 83,
		},
		{
			name:      "camelCase",
			file:      `{"cachedUsageUtilization":{"fiveHour":{"utilization":25,"resetsAt":"` + iso + `"},"sevenDay":{"utilization":40,"resetsAt":"` + iso + `"}}}`,
			wantOK:    true,
			wantFive:  25,
			wantSeven: 40,
		},
		{
			name:      "forma de lista",
			file:      `{"cachedUsageUtilization":{"windows":[{"name":"five_hour","utilization":30,"resets_at":"` + iso + `"},{"name":"seven_day","utilization":60,"resets_at":"` + iso + `"}]}}`,
			wantOK:    true,
			wantFive:  30,
			wantSeven: 60,
		},
		{name: "sin la clave", file: `{"userID":"x","projects":{}}`},
		{name: "forma irreconocible", file: `{"cachedUsageUtilization":{"algo":{"raro":1}}}`},
		{name: "json basura", file: `{"cachedUsageUtilization":`},
		{name: "archivo vacío", file: ``},
		{name: "sin archivo"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cc := t.TempDir()
			if tc.name != "sin archivo" {
				if err := os.WriteFile(filepath.Join(cc, ".claude.json"), []byte(tc.file), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			rl, ok := ReadCachedUsage(cc)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, quería %v (rl=%+v)", ok, tc.wantOK, rl)
			}
			if !ok {
				if rl != (RateLimits{}) {
					t.Fatalf("con ok=false se esperaba muestra cero, hay %+v", rl)
				}
				return
			}
			if rl.FiveHour.UsedPercentage != tc.wantFive || rl.SevenDay.UsedPercentage != tc.wantSeven {
				t.Fatalf("porcentajes = (%v,%v), quería (%v,%v)", rl.FiveHour.UsedPercentage, rl.SevenDay.UsedPercentage, tc.wantFive, tc.wantSeven)
			}
			if !rl.SevenDay.ResetsAt.Equal(want) {
				t.Fatalf("resets_at ISO = %v, quería %v", rl.SevenDay.ResetsAt, want)
			}
		})
	}
}

// Una ventana cuyo resets_at YA PASÓ no está agotada: es una medida caducada de
// una ventana que volvió a abrir. Tratarla como agotada hace que el sensor
// proactivo mate la sesión y destierre una cuenta perfectamente disponible (el
// cooldown del Chain descarta el resets_at pasado y aplica la hora de respaldo).
func TestRateLimitsExhaustedIgnoraVentanaYaReseteada(t *testing.T) {
	reset := time.Date(2026, 7, 26, 1, 0, 0, 0, time.UTC)
	rl := RateLimits{FiveHour: Windowed{UsedPercentage: 100, ResetsAt: reset}}

	if w, hit := rl.ExhaustedAt(90, reset.Add(-time.Minute)); !hit || w != WindowSession {
		t.Fatalf("antes del reset la ventana sí está agotada: %q/%v", w, hit)
	}
	if w, hit := rl.ExhaustedAt(90, reset.Add(time.Minute)); hit {
		t.Fatalf("tras el reset no puede seguir agotada: %q", w)
	}

	// La otra ventana se evalúa por su cuenta: que la de 5h haya reabierto no
	// absuelve a la semanal.
	rl.SevenDay = Windowed{UsedPercentage: 95, ResetsAt: reset.Add(48 * time.Hour)}
	if w, hit := rl.ExhaustedAt(90, reset.Add(time.Minute)); !hit || w != WindowWeekly {
		t.Fatalf("la semanal sigue agotada: %q/%v", w, hit)
	}

	// Sin resets_at no hay nada que caducar: el porcentaje manda.
	sinReset := RateLimits{SevenDay: Windowed{UsedPercentage: 99}}
	if _, hit := sinReset.ExhaustedAt(90, reset); !hit {
		t.Fatal("una ventana sin resets_at no se puede descartar por caducidad")
	}
}

// El bug de five_hour.utilization==0: la ventana se lee, pero como no aporta
// dato distinguible NO debe hacer que Exhausted informe «0% usado». Con
// resets_at presente sí hay dato, y por eso ese caso no dispara con umbral 90.
func TestCachedUsageBugFiveHourCeroNoDisparaFalsaCalma(t *testing.T) {
	cc := t.TempDir()
	body := `{"cachedUsageUtilization":{"five_hour":{"utilization":0,"resets_at":"2026-07-25T18:00:00Z"},"seven_day":{"utilization":83,"resets_at":"2026-07-25T18:00:00Z"}}}`
	if err := os.WriteFile(filepath.Join(cc, ".claude.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	rl, ok := ReadCachedUsage(cc)
	if !ok {
		t.Fatal("se esperaba lectura válida")
	}
	// Instante anterior al resets_at del caché: las ventanas siguen vivas.
	now := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	if w, hit := rl.ExhaustedAt(90, now); hit || w != WindowUnknown {
		t.Fatalf("con 0%% y 83%% bajo umbral 90 no debía haber agotamiento, hubo %q/%v", w, hit)
	}
	if w, hit := rl.ExhaustedAt(80, now); !hit || w != WindowWeekly {
		t.Fatalf("bajo umbral 80 debía disparar la weekly, hubo %q/%v", w, hit)
	}
	// Una ventana sin ningún campo reconocible es «sin dato», no 0%.
	sinDato := RateLimits{FiveHour: Windowed{}}
	if sinDato.FiveHour.HasData() {
		t.Fatal("Windowed cero no debe declarar dato")
	}
}
