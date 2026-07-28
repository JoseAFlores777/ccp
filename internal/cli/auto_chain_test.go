package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/JoseAFlores777/ccp/internal/core"
)

// auto_chain_test.go — la capa de presentación de `ccp auto chain`.
//
// La semántica de las mutaciones la fija internal/core/auto_chain_test.go; aquí
// se fija lo que el usuario VE, que en este comando es parte del contrato: `add`
// toca dos claves del yaml y tiene que decir qué le hizo a cada una.

// chainCLIHome monta un home con tres perfiles, un bloque auto_handoff con gate
// declarado y una regla que hace de a-cc el primario del cwd simulado.
func chainCLIHome(t *testing.T) (home, cwd string) {
	t.Helper()
	home = autoTestHome(t, "a-cc", "b-cc", "c-cc")
	cwd = "/work/alpha"
	t.Setenv("PWD", cwd)

	if _, err := core.RuleSet(home, cwd, "a-cc"); err != nil {
		t.Fatal(err)
	}
	cfg, err := core.Load(home)
	if err != nil {
		t.Fatal(err)
	}
	cfg.AutoHandoff = &core.AutoHandoff{
		Enabled: true,
		Policies: map[string]core.AutoPolicy{
			"default": {Fallback: []string{"b-cc", "c-cc"}},
		},
		// Gate declarado: b-cc pasa, c-cc no. Es el escenario en el que la cadena
		// del yaml y la cadena real NO coinciden.
		AllowFrom: map[string][]string{"a-cc": {"b-cc"}},
	}
	if err := core.Save(home, cfg); err != nil {
		t.Fatal(err)
	}
	return home, cwd
}

// chainLineWith devuelve la primera línea que contiene want.
func chainLineWith(s, want string) string {
	for _, line := range strings.Split(s, "\n") {
		if strings.Contains(line, want) {
			return line
		}
	}
	return ""
}

// TestChainShowUsaCadenaEfectiva: `show` no puede enseñar la lista cruda del
// yaml. Un perfil que allow_from bloquea está en `fallback` y aun así NUNCA se
// usa; enseñarlo como parte de la cadena es la misma mentira que el yaml ya
// cuenta, y el comando existe para deshacerla.
func TestChainShowUsaCadenaEfectiva(t *testing.T) {
	chainCLIHome(t)

	var out, errb bytes.Buffer
	if code := dispatchAuto([]string{"chain"}, &out, &errb); code != 0 {
		t.Fatalf("chain show salió %d (%s)", code, errb.String())
	}
	s := out.String()

	cadena := chainLineWith(s, "cadena")
	if cadena == "" {
		t.Fatalf("no hay línea de cadena:\n%s", s)
	}
	if !strings.Contains(cadena, "b-cc") {
		t.Errorf("la cadena efectiva debe incluir b-cc: %q", cadena)
	}
	if strings.Contains(cadena, "c-cc") {
		t.Errorf("c-cc está denegado por allow_from y no puede salir en la cadena: %q", cadena)
	}
	if den := chainLineWith(s, "denegados"); !strings.Contains(den, "c-cc") {
		t.Errorf("falta la línea de denegados con c-cc:\n%s", s)
	}
	if !strings.Contains(s, "a-cc") {
		t.Errorf("falta el primario en la salida:\n%s", s)
	}
}

// TestChainAddImprimeCadaClavePorSeparado es el límite 2 de D5: el ensanche de
// allow_from es un cambio en un gate de cumplimiento, así que no puede quedar
// resumido dentro del «hecho» de la cadena. Dos claves, dos líneas.
func TestChainAddImprimeCadaClavePorSeparado(t *testing.T) {
	home, _ := chainCLIHome(t)

	var out, errb bytes.Buffer
	// b-cc está en la cadena Y autorizado: ahí no queda nada que hacer, y eso sí
	// es un duplicado.
	if code := dispatchAuto([]string{"chain", "add", "b-cc"}, &out, &errb); code != 1 {
		t.Fatalf("add duplicado salió %d (%s)", code, errb.String())
	}
	if !strings.Contains(errb.String(), "b-cc") {
		t.Errorf("el error de duplicado debe nombrar el perfil: %q", errb.String())
	}

	// El caso real: un perfil que no estaba y que el gate no autorizaba.
	out.Reset()
	errb.Reset()
	if _, err := core.ChainRm(home, core.ChainOpts{Cwd: "/work/alpha"}, []string{"c-cc"}); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	errb.Reset()
	if code := dispatchAuto([]string{"chain", "add", "c-cc"}, &out, &errb); code != 0 {
		t.Fatalf("chain add salió %d (%s)", code, errb.String())
	}
	s := out.String()

	fb := chainLineWith(s, "fallback")
	if !strings.Contains(fb, "b-cc → c-cc") {
		t.Errorf("la línea de fallback debe enseñar el orden resultante: %q", fb)
	}
	allow := chainLineWith(s, "allow_from")
	if !strings.Contains(allow, "a-cc") || !strings.Contains(allow, "+c-cc") {
		t.Errorf("la línea de allow_from debe decir qué entrada creció y con qué: %q", allow)
	}
	if fb == allow {
		t.Error("fallback y allow_from tienen que ir en líneas distintas")
	}
	if !strings.Contains(s, "cadena efectiva") {
		t.Errorf("falta la cadena efectiva al cierre:\n%s", s)
	}
	// Y ahora la cadena efectiva ya incluye c-cc: el gate se abrió de verdad.
	if eff := s[strings.Index(s, "cadena efectiva"):]; !strings.Contains(eff, "c-cc") {
		t.Errorf("la cadena efectiva no refleja el ensanche:\n%s", eff)
	}
}

// TestChainCLIErroresDeUso: las banderas y posiciones malas salen 1 con mensaje,
// nunca con un panic ni con un clamp mudo.
func TestChainCLIErroresDeUso(t *testing.T) {
	chainCLIHome(t)

	cases := []struct {
		name string
		args []string
		want string
	}{
		{"posición no numérica", []string{"chain", "add", "b-cc", "--at", "x"}, "posición"},
		{"posición cero", []string{"chain", "add", "b-cc", "--at", "0"}, "posición"},
		{"mv sin posición", []string{"chain", "mv", "b-cc"}, "mv"},
		{"subcomando inventado", []string{"chain", "flip"}, "flip"},
		{"bandera inventada", []string{"chain", "add", "--zzz"}, "--zzz"},
		{"add sin perfiles", []string{"chain", "add"}, "perfil"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var out, errb bytes.Buffer
			if code := dispatchAuto(tc.args, &out, &errb); code != 1 {
				t.Fatalf("salió %d, want 1 (stdout=%q stderr=%q)", code, out.String(), errb.String())
			}
			if !strings.Contains(errb.String(), tc.want) {
				t.Errorf("stderr = %q, debía mencionar %q", errb.String(), tc.want)
			}
		})
	}
}

// TestChainBanderaAntesDelSubcomando: `ccp auto chain --at 1 add x` tiene que
// AÑADIR. Antes el subcomando se tomaba de args[0] a secas, así que una bandera
// delante degradaba el comando entero a `show`: exit 0, salida con pinta de
// éxito y el yaml sin tocar. Es la peor combinación posible para un comando cuyo
// trabajo es escribir.
func TestChainBanderaAntesDelSubcomando(t *testing.T) {
	home, _ := chainCLIHome(t)
	if _, err := core.ChainRm(home, core.ChainOpts{Cwd: "/work/alpha", NoAllow: true}, []string{"c-cc"}); err != nil {
		t.Fatal(err)
	}

	var out, errb bytes.Buffer
	if code := dispatchAuto([]string{"chain", "--at", "1", "add", "c-cc"}, &out, &errb); code != 0 {
		t.Fatalf("salió %d (%s)", code, errb.String())
	}
	cfg, err := core.Load(home)
	if err != nil {
		t.Fatal(err)
	}
	got := cfg.AutoHandoff.Policies["default"].Fallback
	if len(got) == 0 || got[0] != "c-cc" {
		t.Fatalf("fallback = %v: la bandera delante degradó el comando a show", got)
	}
}

// TestChainShowRechazaPositionalesSobrantes: un subcomando mal escrito no puede
// acabar enseñando la cadena y saliendo 0.
func TestChainShowRechazaPositionalesSobrantes(t *testing.T) {
	chainCLIHome(t)

	var out, errb bytes.Buffer
	if code := dispatchAuto([]string{"chain", "show", "b-cc"}, &out, &errb); code != 1 {
		t.Fatalf("salió %d, want 1 (stdout=%q)", code, out.String())
	}
	if !strings.Contains(errb.String(), "b-cc") {
		t.Errorf("stderr no nombra lo que sobra: %q", errb.String())
	}
}

// TestChainBanderaQueNoAplica: tragarse `--at` en un `rm` sería la misma trampa
// que la de arriba por otro lado — el usuario cree haber pedido algo concreto.
func TestChainBanderaQueNoAplica(t *testing.T) {
	chainCLIHome(t)

	for _, args := range [][]string{
		{"chain", "--at", "2", "rm", "b-cc"},
		{"chain", "--no-allow", "mv", "b-cc", "1"},
	} {
		var out, errb bytes.Buffer
		if code := dispatchAuto(args, &out, &errb); code != 1 {
			t.Errorf("%v salió %d, want 1 (stderr=%q)", args, code, errb.String())
		}
	}
}

// TestChainSiempreImprimeUnaLineaDeAllowFrom: el contrato del módulo es que
// «no se tocó» y «se ensanchó» NUNCA se pintan igual. El silencio es la forma
// más fácil de romperlo, y era donde caían set, rm y mv.
func TestChainSiempreImprimeUnaLineaDeAllowFrom(t *testing.T) {
	casos := []struct {
		name string
		args []string
	}{
		{"set", []string{"chain", "set", "b-cc,c-cc"}},
		{"rm", []string{"chain", "rm", "b-cc"}},
		{"mv", []string{"chain", "mv", "c-cc", "1"}},
	}
	for _, tc := range casos {
		t.Run(tc.name, func(t *testing.T) {
			chainCLIHome(t)
			var out, errb bytes.Buffer
			if code := dispatchAuto(tc.args, &out, &errb); code != 0 {
				t.Fatalf("salió %d (%s)", code, errb.String())
			}
			if chainLineWith(out.String(), "allow_from") == "" {
				t.Errorf("no hay línea de allow_from:\n%s", out.String())
			}
		})
	}
}

// TestChainSetNoDejaLaCadenaInerte: `set` escribía la lista nueva y no tocaba el
// gate, así que podía dejar la política sin NINGÚN destino al que rotar con un
// `[ok]` delante. El único indicio era la línea de «denegados» del bloque final.
func TestChainSetNoDejaLaCadenaInerte(t *testing.T) {
	home, cwd := chainCLIHome(t)

	var out, errb bytes.Buffer
	if code := dispatchAuto([]string{"chain", "set", "c-cc"}, &out, &errb); code != 0 {
		t.Fatalf("salió %d (%s)", code, errb.String())
	}
	rc, err := core.ResolveAutoChain(home, nil, "", cwd)
	if err != nil {
		t.Fatal(err)
	}
	if len(rc.Fallback) == 0 {
		t.Fatalf("la cadena quedó inerte: %+v\nsalida:\n%s", rc, out.String())
	}
}

// TestChainShowYAddHablanElMismoIdioma: la MISMA condición no puede salir en
// inglés por un subcomando y en español por otro dentro del mismo comando.
func TestChainShowYAddHablanElMismoIdioma(t *testing.T) {
	chainCLIHome(t)
	t.Setenv("CCP_LANG", "en")

	var out, showErr bytes.Buffer
	if code := dispatchAuto([]string{"chain", "show", "--policy", "nope"}, &out, &showErr); code != 1 {
		t.Fatalf("show con política inexistente salió %d", code)
	}
	out.Reset()
	var addErr bytes.Buffer
	if code := dispatchAuto([]string{"chain", "add", "--policy", "nope", "b-cc"}, &out, &addErr); code != 1 {
		t.Fatalf("add con política inexistente salió %d", code)
	}
	if !strings.Contains(showErr.String(), "does not exist") {
		t.Errorf("`show` no habla inglés en CCP_LANG=en: %q", showErr.String())
	}
	if !strings.Contains(addErr.String(), "does not exist") {
		t.Errorf("`add` no habla inglés en CCP_LANG=en: %q", addErr.String())
	}
}

// TestChainPolicySeleccionaLaPolitica: --policy es la única forma de tocar una
// política que no sea la del cwd, y equivocarse de política sería editar la
// rotación de otro repo.
func TestChainPolicySeleccionaLaPolitica(t *testing.T) {
	home, _ := chainCLIHome(t)
	cfg, err := core.Load(home)
	if err != nil {
		t.Fatal(err)
	}
	cfg.AutoHandoff.Policies["noche"] = core.AutoPolicy{Fallback: []string{"b-cc"}}
	if err := core.Save(home, cfg); err != nil {
		t.Fatal(err)
	}

	var out, errb bytes.Buffer
	if code := dispatchAuto([]string{"chain", "add", "c-cc", "--policy", "noche"}, &out, &errb); code != 0 {
		t.Fatalf("salió %d (%s)", code, errb.String())
	}
	after, err := core.Load(home)
	if err != nil {
		t.Fatal(err)
	}
	if got := after.AutoHandoff.Policies["noche"].Fallback; len(got) != 2 || got[1] != "c-cc" {
		t.Errorf("política noche = %v", got)
	}
	if got := after.AutoHandoff.Policies["default"].Fallback; len(got) != 2 {
		t.Errorf("la política default no se debía tocar: %v", got)
	}
}

// TestHelpDocumentaAutoChain: `ccp help` es una de las dos superficies de
// descubrimiento del binario (la otra, la completion, la congela el golden).
func TestHelpDocumentaAutoChain(t *testing.T) {
	for _, lang := range []string{"es", "en"} {
		t.Setenv("CCP_HOME", t.TempDir())
		t.Setenv("CCP_LANG", lang)
		var out, errb bytes.Buffer
		if code := Dispatch([]string{"help"}, &out, &errb); code != 0 {
			t.Fatalf("%s: help salió %d (%s)", lang, code, errb.String())
		}
		for _, want := range []string{"ccp auto chain", "add|rm|mv|set"} {
			if !strings.Contains(out.String(), want) {
				t.Errorf("%s: `ccp help` no menciona %q", lang, want)
			}
		}
	}
}

// TestAutoUsageDocumentaChain: `ccp auto` a secas tiene que enumerar chain, en
// los dos idiomas.
func TestAutoUsageDocumentaChain(t *testing.T) {
	for _, lang := range []string{"es", "en"} {
		t.Setenv("CCP_HOME", t.TempDir())
		t.Setenv("CCP_LANG", lang)
		var out, errb bytes.Buffer
		if code := dispatchAuto(nil, &out, &errb); code != 0 {
			t.Fatalf("%s: auto salió %d (%s)", lang, code, errb.String())
		}
		if !strings.Contains(out.String(), "chain") {
			t.Errorf("%s: `ccp auto` no menciona chain:\n%s", lang, out.String())
		}
	}
}
