package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// countFiles cuenta los .json de un directorio (0 si no existe).
func countFiles(t *testing.T, dir string) int {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0
		}
		t.Fatal(err)
	}
	n := 0
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
			n++
		}
	}
	return n
}

func TestAutoStateDir(t *testing.T) {
	home := "/tmp/ccp-home"
	if got, want := AutoStateDir(home), filepath.Join(home, "state", "auto"); got != want {
		t.Fatalf("AutoStateDir = %q, quería %q", got, want)
	}
}

func TestWriteReadRateLimitsRoundTrip(t *testing.T) {
	home := t.TempDir()
	now := time.Date(2026, 7, 25, 12, 30, 0, 0, time.UTC)
	reset := time.Date(2026, 7, 25, 18, 0, 0, 0, time.UTC)
	rl := RateLimits{
		FiveHour: Windowed{UsedPercentage: 93.2, ResetsAt: reset},
		SevenDay: Windowed{UsedPercentage: 13, ResetsAt: reset},
	}
	if err := WriteRateLimits(home, "personal-1", rl, now); err != nil {
		t.Fatalf("WriteRateLimits: %v", err)
	}
	got, sampled, ok := ReadRateLimits(home, "personal-1")
	if !ok {
		t.Fatal("ReadRateLimits ok=false tras escribir")
	}
	if !sampled.Equal(now) {
		t.Fatalf("sampled = %v, quería %v", sampled, now)
	}
	if got.FiveHour.UsedPercentage != 93.2 || !got.FiveHour.ResetsAt.Equal(reset) {
		t.Fatalf("five_hour no sobrevivió el round-trip: %+v", got.FiveHour)
	}
	if got.SevenDay.UsedPercentage != 13 || !got.SevenDay.ResetsAt.Equal(reset) {
		t.Fatalf("seven_day no sobrevivió el round-trip: %+v", got.SevenDay)
	}
	// Sobrescribir deja UNA sola muestra por perfil (es la «última lectura»).
	if err := WriteRateLimits(home, "personal-1", RateLimits{FiveHour: Windowed{UsedPercentage: 10, ResetsAt: reset}}, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if n := countFiles(t, filepath.Join(AutoStateDir(home), "rate-limits")); n != 1 {
		t.Fatalf("archivos de muestra = %d, quería 1", n)
	}
	got, _, _ = ReadRateLimits(home, "personal-1")
	if got.FiveHour.UsedPercentage != 10 {
		t.Fatalf("la segunda escritura no pisó la primera: %+v", got.FiveHour)
	}
}

func TestReadRateLimitsDegradaSuave(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(AutoStateDir(home), "rate-limits")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "roto.json"), []byte("{no json"), 0o644); err != nil {
		t.Fatal(err)
	}
	cases := []string{"sin-muestra", "roto"}
	for _, profile := range cases {
		t.Run(profile, func(t *testing.T) {
			rl, sampled, ok := ReadRateLimits(home, profile)
			if ok {
				t.Fatalf("ok=true para %q (rl=%+v)", profile, rl)
			}
			if rl != (RateLimits{}) || !sampled.IsZero() {
				t.Fatalf("con ok=false todo debe ser cero: %+v %v", rl, sampled)
			}
		})
	}
}

func TestWriteRateLimitsPerfilVacio(t *testing.T) {
	home := t.TempDir()
	if err := WriteRateLimits(home, "   ", RateLimits{}, time.Now()); err == nil {
		t.Fatal("un perfil vacío debe rechazarse: no hay nombre de archivo válido")
	}
	if n := countFiles(t, filepath.Join(AutoStateDir(home), "rate-limits")); n != 0 {
		t.Fatalf("no debió crearse ningún archivo, hay %d", n)
	}
}

// Los nombres llegan por stdin de un hook: han de quedar SIEMPRE dentro de
// <home>/state/auto/sentinels, pase lo que pase.
func TestWriteSentinelNombresHostiles(t *testing.T) {
	base := time.Date(2026, 7, 25, 9, 0, 0, 0, time.UTC)
	cases := []struct {
		name    string
		profile string
		session string
	}{
		{"escape con ..", "../../x", "../../../etc/passwd"},
		{"barra dentro", "a/b", "a/b"},
		{"solo puntos", "..", "."},
		{"espacios y comillas", `perfil "raro"`, "ses ion"},
		{"unicode", "perfil-ñ", "sesión-✓"},
		{"nulo y saltos", "a\nb", "a\x00b"},
		{"larguísimo", strings.Repeat("p", 500), strings.Repeat("s", 500)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			s := Sentinel{
				Profile: tc.profile,
				Session: tc.session,
				Event:   LimitEvent{Window: WindowSession, Source: "hook", Detail: "You've hit your session limit"},
				At:      base,
			}
			if err := WriteSentinel(home, s); err != nil {
				t.Fatalf("WriteSentinel: %v", err)
			}
			dir := filepath.Join(AutoStateDir(home), "sentinels")
			entries, err := os.ReadDir(dir)
			if err != nil {
				t.Fatalf("el sentinel no aterrizó en %s: %v", dir, err)
			}
			if len(entries) != 1 {
				t.Fatalf("archivos en sentinels = %d, quería 1", len(entries))
			}
			name := entries[0].Name()
			if strings.ContainsAny(name, `/\`) || strings.Contains(name, "..") {
				t.Fatalf("nombre de archivo peligroso: %q", name)
			}
			// Ningún archivo fuera del directorio de sentinels.
			var stray []string
			_ = filepath.Walk(home, func(p string, info os.FileInfo, err error) error {
				if err != nil || info.IsDir() {
					return nil
				}
				if filepath.Dir(p) != dir {
					stray = append(stray, p)
				}
				return nil
			})
			if len(stray) > 0 {
				t.Fatalf("se escribió fuera del directorio de sentinels: %v", stray)
			}
			// El contenido conserva los valores ORIGINALES: la sanitización es
			// solo del nombre de archivo, no del dato.
			got, err := ReadSentinels(home, time.Time{})
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != 1 {
				t.Fatalf("ReadSentinels devolvió %d, quería 1", len(got))
			}
			if got[0].Profile != tc.profile || got[0].Session != tc.session {
				t.Fatalf("contenido alterado: %+v", got[0])
			}
			if got[0].Event.Window != WindowSession || got[0].Event.Source != "hook" {
				t.Fatalf("el LimitEvent no sobrevivió: %+v", got[0].Event)
			}
		})
	}
}

func TestWriteSentinelSesionVacia(t *testing.T) {
	home := t.TempDir()
	err := WriteSentinel(home, Sentinel{Profile: "personal-1", Session: "", At: time.Now()})
	if err == nil {
		t.Fatal("una sesión vacía debe rechazarse")
	}
	if n := countFiles(t, filepath.Join(AutoStateDir(home), "sentinels")); n != 0 {
		t.Fatalf("no debió crearse ningún archivo, hay %d", n)
	}
}

func TestReadSentinelsOrdenYSince(t *testing.T) {
	home := t.TempDir()
	base := time.Date(2026, 7, 25, 9, 0, 0, 0, time.UTC)
	// Se escriben desordenados a propósito: el orden lo debe imponer el lector.
	offsets := []time.Duration{2 * time.Minute, 0, time.Minute}
	for _, off := range offsets {
		s := Sentinel{
			Profile: "personal-1",
			Session: "ses-1",
			Event:   LimitEvent{Window: WindowWeekly, Source: "hook", Detail: off.String()},
			At:      base.Add(off),
		}
		if err := WriteSentinel(home, s); err != nil {
			t.Fatal(err)
		}
	}
	// Un archivo corrupto en medio no puede tumbar la lectura.
	dir := filepath.Join(AutoStateDir(home), "sentinels")
	if err := os.WriteFile(filepath.Join(dir, "20260725T090030.000000000Z-basura.json"), []byte("{{{"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Y un archivo que no es .json se ignora sin más.
	if err := os.WriteFile(filepath.Join(dir, "notas.txt"), []byte("hola"), 0o644); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name  string
		since time.Time
		want  []time.Duration
	}{
		{"todo", time.Time{}, []time.Duration{0, time.Minute, 2 * time.Minute}},
		{"desde el primero (exclusivo)", base, []time.Duration{time.Minute, 2 * time.Minute}},
		{"desde el medio", base.Add(time.Minute), []time.Duration{2 * time.Minute}},
		{"desde el futuro", base.Add(time.Hour), nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ReadSentinels(home, tc.since)
			if err != nil {
				t.Fatalf("ReadSentinels: %v", err)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("n = %d, quería %d (%+v)", len(got), len(tc.want), got)
			}
			for i, off := range tc.want {
				if !got[i].At.Equal(base.Add(off)) {
					t.Fatalf("posición %d: At = %v, quería %v", i, got[i].At, base.Add(off))
				}
			}
		})
	}
}

// Los sentinels los escribe el hook StopFailure en CUALQUIER `claude` del
// perfil, haya supervisor o no, y solo se borran en el bloque de rotación. Sin
// reclamación por edad el directorio crece para siempre: texto de errores en
// disco a perpetuidad, y un ReadSentinels por poll (cada 2s) que recorre todo el
// historial durante toda la sesión.
func TestWriteSentinelReclamaLasSeñalesViejas(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(AutoStateDir(home), "sentinels")
	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)

	vieja := Sentinel{
		Profile: "personal-1", Session: "de-anteayer",
		Event: LimitEvent{Window: WindowSession, Source: "hook"},
		At:    now.Add(-48 * time.Hour),
	}
	if err := WriteSentinel(home, vieja); err != nil {
		t.Fatal(err)
	}
	if n := countFiles(t, dir); n != 1 {
		t.Fatalf("archivos = %d, quería 1 recién escrito", n)
	}

	nueva := Sentinel{
		Profile: "personal-1", Session: "de-ahora",
		Event: LimitEvent{Window: WindowSession, Source: "hook"},
		At:    now,
	}
	if err := WriteSentinel(home, nueva); err != nil {
		t.Fatal(err)
	}
	if n := countFiles(t, dir); n != 1 {
		t.Fatalf("archivos = %d, quería 1: la señal de hace 48h debía reclamarse", n)
	}
	got, err := ReadSentinels(home, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Session != "de-ahora" {
		t.Fatalf("quedó = %+v, quería solo la reciente", got)
	}
}

func TestReadSentinelsSinDirectorio(t *testing.T) {
	home := t.TempDir()
	got, err := ReadSentinels(home, time.Time{})
	if err != nil {
		t.Fatalf("un home sin sentinels no es un error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("se esperaba lista vacía, hay %d", len(got))
	}
}

func TestClearSentinels(t *testing.T) {
	home := t.TempDir()
	base := time.Date(2026, 7, 25, 9, 0, 0, 0, time.UTC)
	write := func(session string, off time.Duration) {
		t.Helper()
		if err := WriteSentinel(home, Sentinel{
			Profile: "personal-1",
			Session: session,
			Event:   LimitEvent{Window: WindowSession, Source: "hook"},
			At:      base.Add(off),
		}); err != nil {
			t.Fatal(err)
		}
	}
	write("ses-1", 0)
	write("ses-1", time.Minute)
	write("ses-2", 2*time.Minute)
	// Dos sesiones que colisionan al sanear ("a/b" y "a_b" comparten nombre):
	// el emparejamiento va por el contenido, así que no se pisan.
	write("a/b", 3*time.Minute)
	write("a_b", 4*time.Minute)

	if err := ClearSentinels(home, "ses-1"); err != nil {
		t.Fatalf("ClearSentinels: %v", err)
	}
	got, err := ReadSentinels(home, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("quedaron %d sentinels, querían 3: %+v", len(got), got)
	}
	for _, s := range got {
		if s.Session == "ses-1" {
			t.Fatalf("sobrevivió un sentinel de ses-1: %+v", s)
		}
	}

	if err := ClearSentinels(home, "a/b"); err != nil {
		t.Fatal(err)
	}
	got, _ = ReadSentinels(home, time.Time{})
	sessions := map[string]bool{}
	for _, s := range got {
		sessions[s.Session] = true
	}
	if sessions["a/b"] {
		t.Fatal("no se borró la sesión a/b")
	}
	if !sessions["a_b"] {
		t.Fatal("se borró a_b por colisión de nombre saneado: el match debe ir por contenido")
	}

	// Borrar una sesión inexistente, o sobre un home sin directorio, es un no-op.
	if err := ClearSentinels(home, "no-existe"); err != nil {
		t.Fatalf("borrar lo que no hay no es error: %v", err)
	}
	if err := ClearSentinels(t.TempDir(), "ses-1"); err != nil {
		t.Fatalf("home sin sentinels no es error: %v", err)
	}
	if err := ClearSentinels(home, ""); err == nil {
		t.Fatal("una sesión vacía debe rechazarse (borraría por comodín)")
	}
}
