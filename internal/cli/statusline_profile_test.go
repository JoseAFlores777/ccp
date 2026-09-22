package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/JoseAFlores777/ccp/internal/core"
)

// TestStatusLineProfileDeduceDelConfigDir: sin CCP_PROFILE, el perfil se deduce
// de CLAUDE_CONFIG_DIR en vez de caer a "default".
//
// Caer a "default" no era un fallo visible: era peor. Quien lanza `claude` con
// el config dir a mano —algo que la propia documentación de Claude Code enseña—
// veía su consumo contabilizado en OTRA cuenta, en silencio, mientras la cuenta
// real seguía diciendo «sin datos». Un número en la casilla equivocada es peor
// que ninguno, porque se actúa sobre él.
func TestStatusLineProfileDeduceDelConfigDir(t *testing.T) {
	for _, tc := range []struct{ prof, dir, want string }{
		{"a-cc", "/x/profiles/otro/cc-home", "a-cc"},    // CCP_PROFILE manda
		{"", "/x/profiles/trabajo/cc-home", "trabajo"},  // se deduce
		{"", "/x/profiles/trabajo/cc-home/", "trabajo"}, // con barra final
		{"", "/Users/j/.claude", "default"},             // el global es default
		{"", "", "default"},                             // sin nada, default
		{"  ", "/x/profiles/e-cc/cc-home", "e-cc"},      // CCP_PROFILE en blanco no cuenta
	} {
		t.Setenv("CCP_PROFILE", tc.prof)
		t.Setenv("CLAUDE_CONFIG_DIR", tc.dir)
		if got := statusLineProfile(); got != tc.want {
			t.Errorf("perfil=%q dir=%q -> %q, quería %q", tc.prof, tc.dir, got, tc.want)
		}
	}
}

// TestStatusLinePersisteMuestraVacia es el arreglo del síntoma que costó la
// tarde: un payload SIN consumo —el de Claude Code 2.1.236, cuyas claves son
// context_window, cost, model, thinking…— tiene que dejar constancia igualmente.
//
// Sin esto el sensor corría en cada refresco, no encontraba nada, no escribía, y
// la pantalla decía «todavía no hay muestras» para siempre: la misma frase que
// cuando el sensor no está instalado. Dos causas con arreglos distintos y un
// único síntoma mudo.
func TestStatusLinePersisteMuestraVacia(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CCP_HOME", home)
	t.Setenv("CCP_PROFILE", "a-cc")
	if err := os.WriteFile(filepath.Join(home, "ccp.yaml"), []byte("version: 2\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// El payload REAL de 2.1.236, recortado: ni rate_limits ni rateLimits.
	payload := `{"session_id":"s","version":"2.1.236","cost":{"total_cost_usd":0},` +
		`"context_window":{"used_percentage":null},"model":{"id":"claude-opus-5"}}`

	var out, errb bytes.Buffer
	if code := runStatusLine(bytes.NewReader([]byte(payload)), nil, &out, &errb); code != 0 {
		t.Fatalf("el sensor SIEMPRE sale 0 (salió %d): si no, CC se queda sin barra", code)
	}

	st := core.ReadRateSample(home, "a-cc")
	if !st.Present {
		t.Fatal("un payload sin consumo tiene que dejar constancia de que el sensor corrió")
	}
	if st.Reported {
		t.Error("…pero no puede decir que informó: no había consumo que informar")
	}
	if st.CCVersion != "2.1.236" {
		t.Errorf("CCVersion = %q; el aviso necesita la versión para nombrar cuál no informa", st.CCVersion)
	}
	if _, _, ok := core.ReadRateLimits(home, "a-cc"); ok {
		t.Error("y no puede pasar por dato bueno: serían ceros que nadie midió")
	}
}
