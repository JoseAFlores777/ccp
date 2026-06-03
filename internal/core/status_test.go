package core

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/JoseAFlores777/ccp/internal/core/i18n"
)

// goldenStatusJSONPath devuelve la ruta absoluta al golden commiteado.
func goldenStatusJSONPath(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("no pude resolver la ruta del test")
	}
	return filepath.Join(
		filepath.Dir(thisFile), "..", "..",
		"testdata", "golden", "basic", "expected", "status-json.out",
	)
}

// TestStatusJSONGolden verifica que StatusJSON con los args del fixture
// produce exactamente los bytes commiteados (incluyendo el \n final).
func TestStatusJSONGolden(t *testing.T) {
	goldenBytes, err := os.ReadFile(goldenStatusJSONPath(t))
	if err != nil {
		t.Fatalf("no pude leer el golden: %v", err)
	}

	got := StatusJSON("default", "work", "official", "__CCP_HOME__/repos/work", "") + "\n"

	if got != string(goldenBytes) {
		t.Errorf("StatusJSON no coincide con el golden byte a byte.\ngot:  %q\nwant: %q",
			got, string(goldenBytes))
	}
}

// TestStatusJSONEscaping verifica la paridad de statusJSONEsc con _json_esc
// del oráculo bash para los casos con " y \ en los valores.
func TestStatusJSONEscaping(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "sin_caracteres_especiales",
			input: "default",
			want:  "default",
		},
		{
			name:  "comilla_doble",
			input: `a"b`,
			want:  `a\"b`,
		},
		{
			name:  "barra_invertida",
			input: `a\b`,
			want:  `a\\b`,
		},
		{
			name:  "ambos_a_la_vez",
			input: `a"b\c`,
			want:  `a\"b\\c`,
		},
		{
			name:  "barra_antes_de_comilla",
			input: `a\"b`,
			want:  `a\\\"b`,
		},
		{
			name:  "multiples_barras",
			input: `a\\b`,
			want:  `a\\\\b`,
		},
		{
			name:  "solo_comilla",
			input: `"`,
			want:  `\"`,
		},
		{
			name:  "solo_barra",
			input: `\`,
			want:  `\\`,
		},
		{
			name:  "vacio",
			input: "",
			want:  "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := statusJSONEsc(tc.input)
			if got != tc.want {
				t.Errorf("statusJSONEsc(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

// TestStatusJSONFormat verifica la forma completa del JSON producido por StatusJSON.
func TestStatusJSONFormat(t *testing.T) {
	cases := []struct {
		name        string
		active      string
		profile     string
		profileType string
		cwd         string
		repo        string
		want        string
	}{
		{
			name:        "caso_basico",
			active:      "default",
			profile:     "work",
			profileType: "official",
			cwd:         "/home/user/repos/work",
			repo:        "",
			want:        `{"active":"default","profile":"work","profile_type":"official","cwd":"/home/user/repos/work","repo":""}`,
		},
		{
			name:        "con_repo",
			active:      "work",
			profile:     "work",
			profileType: "official",
			cwd:         "/home/user/repos/work",
			repo:        "/home/user/repos/work",
			want:        `{"active":"work","profile":"work","profile_type":"official","cwd":"/home/user/repos/work","repo":"/home/user/repos/work"}`,
		},
		{
			name:        "valores_con_comillas",
			active:      `say "hello"`,
			profile:     "work",
			profileType: "official",
			cwd:         "/tmp",
			repo:        "",
			want:        `{"active":"say \"hello\"","profile":"work","profile_type":"official","cwd":"/tmp","repo":""}`,
		},
		{
			name:        "valores_con_barras",
			active:      `a\b`,
			profile:     "work",
			profileType: "official",
			cwd:         "/tmp",
			repo:        "",
			want:        `{"active":"a\\b","profile":"work","profile_type":"official","cwd":"/tmp","repo":""}`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := StatusJSON(tc.active, tc.profile, tc.profileType, tc.cwd, tc.repo)
			if got != tc.want {
				t.Errorf("StatusJSON() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestStatusHumanFormat verifica el formato de texto plano (NO_COLOR) de StatusHuman.
func TestStatusHumanFormat(t *testing.T) {
	t.Setenv("CCP_LANG", "es")
	got := StatusHuman(i18n.Es, "default", "work", "official", "/home/user/repos/work", "")

	// Verifica que empieza y termina con la línea hr.
	if got[:len(statusHR)] != statusHR {
		t.Errorf("StatusHuman no empieza con hr:\n%q", got[:len(statusHR)])
	}
	// El string termina con hr + \n.
	wantSuffix := statusHR + "\n"
	if got[len(got)-len(wantSuffix):] != wantSuffix {
		t.Errorf("StatusHuman no termina con hr+\\n; sufijo = %q", got[len(got)-len(wantSuffix):])
	}

	// Verifica que contiene las etiquetas clave.
	for _, substr := range []string{
		"Estado de ccp en esta terminal",
		"Perfil activo (terminal): default",
		"Perfil del cwd (regla):   work  (official)",
		"Cwd:                      /home/user/repos/work",
		"Repo:                     no es git",
	} {
		if !contains(got, substr) {
			t.Errorf("StatusHuman no contiene %q;\ngot:\n%s", substr, got)
		}
	}
}

// TestStatusHumanConRepo verifica que cuando repo != "" se imprime el repo.
func TestStatusHumanConRepo(t *testing.T) {
	t.Setenv("CCP_LANG", "es")
	got := StatusHuman(i18n.Es, "work", "work", "official", "/home/user/repos/work", "/home/user/repos/work")

	if !contains(got, "Repo:                     /home/user/repos/work") {
		t.Errorf("StatusHuman con repo no contiene la ruta del repo;\ngot:\n%s", got)
	}
	if contains(got, "no es git") {
		t.Errorf("StatusHuman con repo no debe contener 'no es git';\ngot:\n%s", got)
	}
}

// contains es un helper para evitar importar strings en los tests.
func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && indexSubstr(s, sub) >= 0)
}

func indexSubstr(s, sub string) int {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
