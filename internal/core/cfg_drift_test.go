package core

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// cfg_drift_test.go — el motor puro de la deriva de cc-home/settings.json (B6).
// Todo con tablas y sin disco: lo que se fija aquí es la semántica del diff
// (la de MergeJSON) y que un cambio de formato no pase por cambio del usuario.

func obj(t *testing.T, s string) map[string]any {
	t.Helper()
	m, err := decodeJSONObject([]byte(s))
	if err != nil {
		t.Fatalf("%s: %v", s, err)
	}
	return m
}

func TestDiffSettings(t *testing.T) {
	type ch struct {
		path string
		kind changeKind
	}
	cases := []struct {
		nombre, last, cur string
		quiero            []ch
	}{
		{"nada", `{"a":1}`, `{"a":1}`, nil},
		{"añadida", `{}`, `{"autoCompactEnabled":false}`, []ch{{"autoCompactEnabled", changeAdded}}},
		{"anidada", `{"env":{"A":"1"}}`, `{"env":{"A":"1","B":"2"}}`, []ch{{"env.B", changeAdded}}},
		{"cambiada", `{"model":"opus"}`, `{"model":"sonnet"}`, []ch{{"model", changeChanged}}},
		{"array entero", `{"permissions":{"allow":["a"]}}`, `{"permissions":{"allow":["a","b"]}}`, []ch{{"permissions.allow", changeChanged}}},
		{"quitada", `{"env":{"A":"1"}}`, `{"env":{}}`, []ch{{"env.A", changeRemoved}}},
		{"número por valor", `{"n":30.0,"m":1e2}`, `{"n":30,"m":100}`, nil},
		{"número distinto", `{"n":30}`, `{"n":30.5}`, []ch{{"n", changeChanged}}},
		{"número dentro de un array", `{"a":[1.0,{"x":2e0}]}`, `{"a":[1,{"x":2}]}`, nil},
		{"cambio de tipo", `{"x":{"a":1}}`, `{"x":"s"}`, []ch{{"x", changeChanged}}},
		{"null explícito", `{"x":1}`, `{"x":null}`, []ch{{"x", changeChanged}}},
		{"orden estable", `{}`, `{"b":1,"a":1}`, []ch{{"a", changeAdded}, {"b", changeAdded}}},
	}
	for _, tc := range cases {
		t.Run(tc.nombre, func(t *testing.T) {
			var got []ch
			for _, c := range diffSettings(obj(t, tc.last), obj(t, tc.cur)) {
				got = append(got, ch{pathString(c.Path), c.Kind})
			}
			if !reflect.DeepEqual(got, tc.quiero) {
				t.Errorf("diff = %v, quiero %v", got, tc.quiero)
			}
		})
	}
}

// Value lleva el valor actual para que la adopción pueda copiarlo al overlay
// tal cual; en un borrado no hay valor que copiar.
func TestDiffSettingsLlevaElValorActual(t *testing.T) {
	got := diffSettings(obj(t, `{"model":"opus","env":{"A":"1"}}`), obj(t, `{"model":"sonnet","env":{}}`))
	if len(got) != 2 {
		t.Fatalf("diff = %v", got)
	}
	if got[0].Kind != changeRemoved || got[0].Value != nil {
		t.Errorf("env.A: %+v, quiero un borrado sin valor", got[0])
	}
	if got[1].Kind != changeChanged || got[1].Value != "sonnet" {
		t.Errorf("model: %+v, quiero sonnet", got[1])
	}
}

// Dos rutas distintas pueden unirse en la misma cadena cuando una clave lleva
// un punto ("a.b" frente a a → b). El orden tiene que seguir siendo total para
// que el informe no baile entre ejecuciones.
func TestDiffSettingsOrdenTotalConPuntosEnLaClave(t *testing.T) {
	for range 20 {
		got := diffSettings(obj(t, `{"a":{}}`), obj(t, `{"a.b":1,"a":{"b":2}}`))
		if len(got) != 2 {
			t.Fatalf("diff = %v", got)
		}
		if !reflect.DeepEqual(got[0].Path, []string{"a", "b"}) || !reflect.DeepEqual(got[1].Path, []string{"a.b"}) {
			t.Fatalf("orden = %v, %v", got[0].Path, got[1].Path)
		}
	}
}

func TestJSONEqual(t *testing.T) {
	cases := []struct {
		nombre string
		a, b   string
		quiero bool
	}{
		{"iguales", `{"a":[1,"x",true,null]}`, `{"a":[1,"x",true,null]}`, true},
		{"número por valor", `{"a":1.50}`, `{"a":1.5}`, true},
		{"exponente", `{"a":1e3}`, `{"a":1000}`, true},
		{"número y cadena", `{"a":1}`, `{"a":"1"}`, false},
		{"array de otro largo", `{"a":[1]}`, `{"a":[1,1]}`, false},
		{"orden del array importa", `{"a":[1,2]}`, `{"a":[2,1]}`, false},
		{"objeto con otra clave", `{"a":{"x":1}}`, `{"a":{"y":1}}`, false},
		{"objeto frente a array", `{"a":{}}`, `{"a":[]}`, false},
		{"null frente a ausente en objeto", `{"a":{"x":null}}`, `{"a":{}}`, false},
	}
	for _, tc := range cases {
		t.Run(tc.nombre, func(t *testing.T) {
			if got := jsonEqual(obj(t, tc.a)["a"], obj(t, tc.b)["a"]); got != tc.quiero {
				t.Errorf("jsonEqual = %v, quiero %v", got, tc.quiero)
			}
		})
	}
}

func TestJSONSetPathYLookup(t *testing.T) {
	doc := obj(t, `{"a":"x"}`)
	jsonSetPath(doc, []string{"a", "b", "c"}, true)
	if v, ok := jsonLookup(doc, []string{"a", "b", "c"}); !ok || v != true {
		t.Fatalf("lookup tras set = %v %v (doc %v)", v, ok, doc)
	}
	if _, ok := jsonLookup(doc, []string{"a", "z"}); ok {
		t.Error("una ruta que no existe no puede encontrarse")
	}
	if _, ok := jsonLookup(doc, []string{"a", "b", "c", "d"}); ok {
		t.Error("bajar por un escalar no puede encontrar nada")
	}
	// Un intermedio que ya es objeto se conserva: fijar una hoja no puede
	// borrar a sus hermanas.
	jsonSetPath(doc, []string{"a", "b", "e"}, "y")
	if v, ok := jsonLookup(doc, []string{"a", "b", "c"}); !ok || v != true {
		t.Errorf("la hermana se perdió: %v", doc)
	}
	// Un null explícito existe: no es lo mismo que una clave ausente.
	jsonSetPath(doc, []string{"n"}, nil)
	if v, ok := jsonLookup(doc, []string{"n"}); !ok || v != nil {
		t.Errorf("null explícito = %v %v", v, ok)
	}
	// La ruta vacía es el documento entero.
	if v, ok := jsonLookup(doc, nil); !ok || !reflect.DeepEqual(v, doc) {
		t.Errorf("ruta vacía = %v %v", v, ok)
	}
}

func TestDecodeJSONObjectRechazaLoQueNoEsObjeto(t *testing.T) {
	for _, s := range []string{"", "   ", "{roto", "[]", "null", `"s"`, "1", `{} basura`, `{}{}`} {
		if _, err := decodeJSONObject([]byte(s)); err == nil {
			t.Errorf("%q tenía que fallar", s)
		}
	}
	m, err := decodeJSONObject([]byte(" {\"n\": 1.0}\n"))
	if err != nil {
		t.Fatal(err)
	}
	// UseNumber: el literal se conserva para no perder precisión en el
	// round-trip, igual que en MergeJSON.
	if n, ok := m["n"].(json.Number); !ok || n != "1.0" {
		t.Errorf("n = %#v, quiero json.Number(\"1.0\")", m["n"])
	}
}

// --- la adopción (adoptSettingsDrift, dentro de CfgRegenerateReport) ---
//
// Estos sí tocan disco, siempre bajo t.TempDir(): CCP_CLAUDE_SRC y HOME apuntan a
// temporales, así que nada llega al ~/.claude ni al ~/.config/ccp reales.

// perfilConBase deja el perfil «work» recién creado: tras B8, el alta ya generó
// su settings.json y la copia, que es la línea base de la deriva. overlay != ""
// se escribe y se regenera, para que la copia ya lo incluya.
func perfilConBase(t *testing.T, global, overlay string) (home, src string) {
	t.Helper()
	home, src = t.TempDir(), t.TempDir()
	t.Setenv("CCP_CLAUDE_SRC", src)
	t.Setenv("HOME", t.TempDir())
	if global != "" {
		if err := os.WriteFile(filepath.Join(src, "settings.json"), []byte(global), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := ProfileAddOfficial(home, "work"); err != nil {
		t.Fatal(err)
	}
	if overlay != "" {
		if err := os.WriteFile(cfgSettingsFile(home, "work"), []byte(overlay), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := CfgRegenerate(home, "work", src); err != nil {
			t.Fatal(err)
		}
	}
	return home, src
}

// simulaConfig hace lo que /config de Claude Code: lee cc-home/settings.json,
// cambia algo y lo reescribe entero con su propio formato (sin el orden ni la
// sangría de ccp, y con los números normalizados: 30.0 sale como 30).
func simulaConfig(t *testing.T, home string, mutate func(map[string]any)) {
	t.Helper()
	p := filepath.Join(ccHomePath(home, "work"), "settings.json")
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	mutate(m)
	out, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, out, 0o644); err != nil {
		t.Fatal(err)
	}
}

func leeOverlay(t *testing.T, home string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(cfgSettingsFile(home, "work"))
	if err != nil {
		t.Fatal(err)
	}
	m, err := decodeOverlayObject(data)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func leeGenerado(t *testing.T, home string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(ccHomePath(home, "work"), "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	return obj(t, string(data))
}

// conCapaAuto instala la capa de sensores en «work» con el binario bin, como
// haría `ccp auto install`, y regenera para que la copia la incluya.
func conCapaAuto(t *testing.T, home, src, bin string) {
	t.Helper()
	prev := AutoHooksBin()
	SetAutoHooksBin(bin)
	t.Cleanup(func() { SetAutoHooksBin(prev) })
	c, err := Load(home)
	if err != nil {
		t.Fatal(err)
	}
	c.AutoHandoff = &AutoHandoff{Hooks: []string{"work"}}
	if err := Save(home, c); err != nil {
		t.Fatal(err)
	}
	if err := CfgRegenerate(home, "work", src); err != nil {
		t.Fatal(err)
	}
}

func TestDerivaDeConfigSeAdoptaEnElOverlay(t *testing.T) {
	home, src := perfilConBase(t, `{"cleanupPeriodDays":30.0,"permissions":{"allow":["Bash(ls)"]}}`, `{"env":{"O":"1"}}`)
	simulaConfig(t, home, func(m map[string]any) {
		m["autoCompactEnabled"] = false
		p := m["permissions"].(map[string]any)
		p["allow"] = append(p["allow"].([]any), "Bash(make)")
	})
	d, err := CfgRegenerateReport(home, "work", src)
	if err != nil {
		t.Fatal(err)
	}
	if d.Profile != "work" {
		t.Errorf("Profile = %q", d.Profile)
	}
	if want := []string{"autoCompactEnabled", "permissions.allow"}; !reflect.DeepEqual(d.Adopted, want) {
		t.Fatalf("Adopted = %v, quiero %v (el 30.0 → 30 de /config no es deriva)", d.Adopted, want)
	}
	ov := leeOverlay(t, home)
	if v, _ := jsonLookup(ov, []string{"env", "O"}); v != "1" {
		t.Errorf("se perdió lo que ya tenía el overlay: %v", ov)
	}
	if v, _ := jsonLookup(ov, []string{"autoCompactEnabled"}); v != false {
		t.Errorf("autoCompactEnabled no llegó al overlay: %v", ov)
	}
	if v, _ := jsonLookup(ov, []string{"permissions", "allow"}); !jsonEqual(v, []any{"Bash(ls)", "Bash(make)"}) {
		t.Errorf("permissions.allow no llegó entero al overlay: %v", ov)
	}
	if _, ok := ov["cleanupPeriodDays"]; ok {
		t.Error("lo que solo cambió de formato no se adopta")
	}
	gen := leeGenerado(t, home)
	if v, _ := jsonLookup(gen, []string{"autoCompactEnabled"}); v != false {
		t.Error("tras regenerar, cc-home tiene que seguir con el cambio de /config")
	}
	if v, _ := jsonLookup(gen, []string{"permissions", "allow"}); !jsonEqual(v, []any{"Bash(ls)", "Bash(make)"}) {
		t.Errorf("tras regenerar, permissions.allow = %v", v)
	}
	if d, err := CfgRegenerateReport(home, "work", src); err != nil || !d.Empty() {
		t.Errorf("la segunda regeneración no tiene nada que adoptar: %+v %v", d, err)
	}
}

func TestDerivaQuitadaSoloSeAvisa(t *testing.T) {
	home, src := perfilConBase(t, "", `{"env":{"O":"1"}}`)
	simulaConfig(t, home, func(m map[string]any) { delete(m["env"].(map[string]any), "O") })
	d, err := CfgRegenerateReport(home, "work", src)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(d.Removed, []string{"env.O"}) || len(d.Adopted) != 0 || len(d.Conflicts) != 0 {
		t.Fatalf("deriva = %+v, quiero Removed [env.O] y nada más", d)
	}
	if v, _ := jsonLookup(leeOverlay(t, home), []string{"env", "O"}); v != "1" {
		t.Error("un borrado no se puede expresar en el overlay: no se toca")
	}
	if v, _ := jsonLookup(leeGenerado(t, home), []string{"env", "O"}); v != "1" {
		t.Error("lo quitado vuelve: por eso se avisa")
	}
}

// Solo se avisa de un borrado que la regeneración va a deshacer. Si el usuario
// también lo quitó de donde salía, no vuelve y no hay nada que decir.
func TestDerivaQuitadaQueNoVuelveNoSeAvisa(t *testing.T) {
	home, src := perfilConBase(t, "", `{"env":{"O":"1"},"model":"opus"}`)
	simulaConfig(t, home, func(m map[string]any) { delete(m["env"].(map[string]any), "O") })
	if err := os.WriteFile(cfgSettingsFile(home, "work"), []byte(`{"model":"opus"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	d, err := CfgRegenerateReport(home, "work", src)
	if err != nil || !d.Empty() {
		t.Fatalf("deriva = %+v %v, quiero vacía", d, err)
	}
}

func TestDerivaConflictoGanaElOverlay(t *testing.T) {
	home, src := perfilConBase(t, "", `{"model":"a"}`)
	simulaConfig(t, home, func(m map[string]any) { m["model"] = "b" })
	if err := os.WriteFile(cfgSettingsFile(home, "work"), []byte(`{"model":"c"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	d, err := CfgRegenerateReport(home, "work", src)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(d.Conflicts, []string{"model"}) || len(d.Adopted) != 0 {
		t.Fatalf("deriva = %+v, quiero Conflicts [model]", d)
	}
	if v, _ := jsonLookup(leeOverlay(t, home), []string{"model"}); v != "c" {
		t.Errorf("el overlay no se toca en un conflicto: model = %v", v)
	}
	if v, _ := jsonLookup(leeGenerado(t, home), []string{"model"}); v != "c" {
		t.Errorf("gana el overlay recién editado: model = %v", v)
	}
}

// Solo se adopta lo que la regeneración perdería: si el valor ya sale del global,
// copiarlo al overlay solo congelaría el global en el perfil.
func TestDerivaQueYaSaleNoSeAdopta(t *testing.T) {
	home, src := perfilConBase(t, `{"model":"opus"}`, "")
	simulaConfig(t, home, func(m map[string]any) { m["model"] = "sonnet" })
	if err := os.WriteFile(filepath.Join(src, "settings.json"), []byte(`{"model":"sonnet"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	d, err := CfgRegenerateReport(home, "work", src)
	if err != nil || !d.Empty() {
		t.Fatalf("deriva = %+v %v, quiero vacía", d, err)
	}
	if _, ok := leeOverlay(t, home)["model"]; ok {
		t.Error("el overlay no tenía que ganar model")
	}
}

func TestDerivaSettingsInvalidoNoAdopta(t *testing.T) {
	home, src := perfilConBase(t, "", `{"env":{"O":"1"}}`)
	antes, err := os.ReadFile(cfgSettingsFile(home, "work"))
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(ccHomePath(home, "work"), "settings.json")
	if err := os.WriteFile(p, []byte("{roto"), 0o644); err != nil {
		t.Fatal(err)
	}
	d, err := CfgRegenerateReport(home, "work", src)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(d.Invalid) != profileStateDir(home, "work") || !strings.HasPrefix(filepath.Base(d.Invalid), "settings.invalid-") ||
		len(d.Adopted) != 0 || d.Empty() {
		t.Fatalf("deriva = %+v", d)
	}
	if c, _ := os.ReadFile(d.Invalid); string(c) != "{roto" {
		t.Errorf("la copia del inválido = %q", c)
	}
	if fi, err := os.Stat(d.Invalid); err != nil || fi.Mode().Perm() != 0o600 {
		t.Errorf("la copia del inválido no es solo del usuario: %v %v", fi, err)
	}
	if ahora, _ := os.ReadFile(cfgSettingsFile(home, "work")); !bytes.Equal(antes, ahora) {
		t.Error("con un settings.json inválido el overlay no se toca")
	}
	if g, _ := os.ReadFile(p); !json.Valid(g) {
		t.Error("tras avisar, se regenera")
	}
}

// Sin línea base tampoco se regenera encima de un inválido sin guardarlo: lo que
// hay dentro lo escribió alguien, y la copia no depende de poder atribuirlo.
func TestDerivaSettingsInvalidoSinCopiaPreviaTambienSeGuarda(t *testing.T) {
	home, src := perfilConBase(t, "", "")
	if err := os.Remove(lastSettingsPath(home, "work")); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(ccHomePath(home, "work"), "settings.json")
	if err := os.WriteFile(p, []byte("{roto"), 0o644); err != nil {
		t.Fatal(err)
	}
	d, err := CfgRegenerateReport(home, "work", src)
	if err != nil {
		t.Fatal(err)
	}
	if d.Invalid == "" {
		t.Fatalf("deriva = %+v, quiero Invalid", d)
	}
	if c, _ := os.ReadFile(d.Invalid); string(c) != "{roto" {
		t.Errorf("la copia del inválido = %q", c)
	}
}

// Si la copia del inválido no se puede guardar, no se regenera: pisarlo sería
// perder lo que el usuario escribió sin dejar rastro.
func TestDerivaSettingsInvalidoSinPoderGuardarloNoLoPisa(t *testing.T) {
	home, src := perfilConBase(t, "", "")
	st := profileStateDir(home, "work")
	if err := os.RemoveAll(st); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(st, []byte("no soy un directorio"), 0o644); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(ccHomePath(home, "work"), "settings.json")
	if err := os.WriteFile(p, []byte("{roto"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := CfgRegenerateReport(home, "work", src); err == nil {
		t.Fatal("sin poder guardar la copia del inválido, la regeneración tenía que fallar")
	}
	if g, _ := os.ReadFile(p); string(g) != "{roto" {
		t.Errorf("el settings.json inválido se pisó: %q", g)
	}
}

func TestDerivaSinCopiaPreviaNoAdopta(t *testing.T) {
	home, src := perfilConBase(t, "", "")
	if err := os.Remove(lastSettingsPath(home, "work")); err != nil {
		t.Fatal(err)
	}
	simulaConfig(t, home, func(m map[string]any) { m["model"] = "x" })
	d, err := CfgRegenerateReport(home, "work", src)
	if err != nil || len(d.Adopted) != 0 || !d.Unattributed || d.Rescued == "" {
		t.Fatalf("sin línea base no se atribuye nada, pero se rescata lo que había: %+v %v", d, err)
	}
	if c, _ := os.ReadFile(d.Rescued); !strings.Contains(string(c), `"model":"x"`) {
		t.Errorf("la copia de rescate no tiene lo de /config: %s", c)
	}
	if _, ok := leeOverlay(t, home)["model"]; ok {
		t.Error("sin línea base el overlay no se toca")
	}
	if !fileExists(lastSettingsPath(home, "work")) || !fileExists(lastOverlayPath(home, "work")) {
		t.Error("se guarda la línea base para la próxima vez")
	}
	if d, err := CfgRegenerateReport(home, "work", src); err != nil || !d.Empty() {
		t.Errorf("con la línea base ya guardada no queda nada que contar: %+v %v", d, err)
	}
}

// Sin línea base, lo que ya coincide con lo que va a salir no merece copia.
func TestDerivaSinCopiaPreviaYSinCambiosNoRescata(t *testing.T) {
	home, src := perfilConBase(t, "", "")
	if err := os.Remove(lastSettingsPath(home, "work")); err != nil {
		t.Fatal(err)
	}
	if d, err := CfgRegenerateReport(home, "work", src); err != nil || !d.Empty() {
		t.Fatalf("deriva = %+v %v, quiero vacía", d, err)
	}
}

// Una copia ilegible es estado derivado roto: vale lo mismo que no tenerla.
func TestDerivaCopiaIlegibleNoAdopta(t *testing.T) {
	home, src := perfilConBase(t, "", "")
	if err := os.WriteFile(lastSettingsPath(home, "work"), []byte("{roto"), 0o600); err != nil {
		t.Fatal(err)
	}
	simulaConfig(t, home, func(m map[string]any) { m["model"] = "x" })
	d, err := CfgRegenerateReport(home, "work", src)
	if err != nil || len(d.Adopted) != 0 || !d.Unattributed || d.Rescued == "" {
		t.Fatalf("con la copia ilegible no se atribuye nada, pero se rescata: %+v %v", d, err)
	}
	if last, _ := os.ReadFile(lastSettingsPath(home, "work")); !json.Valid(last) {
		t.Error("la copia se rehace")
	}
}

// La capa auto está en las dos copias y es de ccp: nunca se adopta, ni cuando la
// ruta del binario difiere entre lo generado y lo que hay ahora.
func TestDerivaNoAdoptaLaCapaAuto(t *testing.T) {
	home, src := perfilConBase(t, `{"statusLine":{"type":"command","command":"starship prompt"}}`, "")
	conCapaAuto(t, home, src, "/viejo/bin/ccp")
	simulaConfig(t, home, func(m map[string]any) {
		m["autoCompactEnabled"] = false
		sl := m["statusLine"].(map[string]any)
		sl["command"] = strings.Replace(sl["command"].(string), "/viejo/bin/ccp", "/nuevo/bin/ccp", 1)
		sf := m["hooks"].(map[string]any)["StopFailure"].([]any)
		h := sf[0].(map[string]any)["hooks"].([]any)[0].(map[string]any)
		h["command"] = strings.Replace(h["command"].(string), "/viejo/bin/ccp", "/nuevo/bin/ccp", 1)
	})
	d, err := CfgRegenerateReport(home, "work", src)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(d.Adopted, []string{"autoCompactEnabled"}) || len(d.Removed)+len(d.Conflicts) != 0 {
		t.Fatalf("deriva = %+v, quiero solo autoCompactEnabled adoptado", d)
	}
	ov := leeOverlay(t, home)
	for _, k := range []string{"statusLine", "hooks"} {
		if _, ok := ov[k]; ok {
			t.Errorf("la capa auto (%s) acabó en el overlay: %v", k, ov)
		}
	}
	gen := leeGenerado(t, home)
	if sl, _ := jsonLookup(gen, []string{"statusLine", "command"}); !strings.HasPrefix(fmt.Sprint(sl), "/viejo/bin/ccp _statusline -- ") ||
		!strings.Contains(fmt.Sprint(sl), "starship") {
		t.Errorf("la capa se sigue generando y envuelve el statusLine del global: %v", sl)
	}
}

// Con la capa puesta, un statusLine que /statusline dejara sin envolver pero igual
// al que ya sale del global no se adopta: la regeneración lo envuelve y el
// usuario sigue viendo el suyo. La comparación es con lo que sale de sus capas
// (global ⊕ overlay), no con el envoltorio, que nunca va a ser igual.
func TestDerivaStatusLineQueYaSaleDelGlobalNoSeAdopta(t *testing.T) {
	home, src := perfilConBase(t, `{"statusLine":{"type":"command","command":"starship prompt"}}`, "")
	conCapaAuto(t, home, src, "/opt/ccp")
	simulaConfig(t, home, func(m map[string]any) {
		m["statusLine"] = map[string]any{"type": "command", "command": "starship prompt"}
	})
	d, err := CfgRegenerateReport(home, "work", src)
	if err != nil || !d.Empty() {
		t.Fatalf("deriva = %+v %v, quiero vacía", d, err)
	}
	if _, ok := leeOverlay(t, home)["statusLine"]; ok {
		t.Error("el statusLine del global acabó congelado en el overlay")
	}
}

// Un statusLine nuevo sí es deriva aunque la capa esté puesta: se adopta y la
// regeneración lo envuelve.
func TestDerivaStatusLineNuevoConCapaAutoSeAdoptaYSeEnvuelve(t *testing.T) {
	home, src := perfilConBase(t, `{"statusLine":{"type":"command","command":"starship prompt"}}`, "")
	conCapaAuto(t, home, src, "/opt/ccp")
	simulaConfig(t, home, func(m map[string]any) {
		m["statusLine"] = map[string]any{"type": "command", "command": "mi-barra"}
	})
	d, err := CfgRegenerateReport(home, "work", src)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(d.Adopted, []string{"statusLine"}) {
		t.Fatalf("deriva = %+v, quiero statusLine adoptado", d)
	}
	if c, _ := jsonLookup(leeOverlay(t, home), []string{"statusLine", "command"}); c != "mi-barra" {
		t.Errorf("overlay statusLine = %v", c)
	}
	c, _ := jsonLookup(leeGenerado(t, home), []string{"statusLine", "command"})
	if s := fmt.Sprint(c); !strings.HasPrefix(s, "/opt/ccp _statusline -- ") || !strings.Contains(s, "mi-barra") {
		t.Errorf("la capa no envuelve lo adoptado: %v", c)
	}
}

// Un hook StopFailure propio que se añade con /hooks junto al de ccp: se adopta
// solo lo del usuario, sin la entrada de ccp, y la capa lo vuelve a poner.
func TestDerivaHookAjenoConCapaAuto(t *testing.T) {
	uno := map[string]any{"hooks": []any{map[string]any{"type": "command", "command": "notify-send uno"}}}
	home, src := perfilConBase(t, "", `{"hooks":{"StopFailure":[{"hooks":[{"type":"command","command":"notify-send uno"}]}]}}`)
	conCapaAuto(t, home, src, "/opt/ccp")
	dos := map[string]any{"hooks": []any{map[string]any{"type": "command", "command": "notify-send dos"}}}
	simulaConfig(t, home, func(m map[string]any) {
		h := m["hooks"].(map[string]any)
		h["StopFailure"] = append(h["StopFailure"].([]any), dos)
	})
	d, err := CfgRegenerateReport(home, "work", src)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(d.Adopted, []string{"hooks.StopFailure"}) || len(d.Conflicts) != 0 {
		t.Fatalf("deriva = %+v, quiero hooks.StopFailure adoptado", d)
	}
	sf, _ := jsonLookup(leeOverlay(t, home), []string{"hooks", "StopFailure"})
	if !jsonEqual(sf, []any{uno, dos}) {
		t.Errorf("overlay StopFailure = %v, quiero solo los del usuario", sf)
	}
	gen, _ := jsonLookup(leeGenerado(t, home), []string{"hooks", "StopFailure"})
	arr, _ := gen.([]any)
	if len(arr) != 3 || !isCCPStopFailureEntry(arr[0]) || !jsonEqual(arr[1:], []any{uno, dos}) {
		t.Errorf("generado StopFailure = %v, quiero [ccp, uno, dos]", gen)
	}
	if d, err := CfgRegenerateReport(home, "work", src); err != nil || !d.Empty() {
		t.Errorf("la segunda regeneración no tiene nada que adoptar: %+v %v", d, err)
	}
}

// Un overlay que es un symlink (un repo de dotfiles) sigue siéndolo tras adoptar,
// y su destino conserva el modo.
func TestDerivaRespetaUnOverlaySymlink(t *testing.T) {
	home, src := perfilConBase(t, "", "")
	dotfiles := filepath.Join(t.TempDir(), "work.json")
	if err := os.WriteFile(dotfiles, []byte("{}\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dotfiles, 0o640); err != nil {
		t.Fatal(err)
	}
	ovPath := cfgSettingsFile(home, "work")
	if err := os.Remove(ovPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(dotfiles, ovPath); err != nil {
		t.Fatal(err)
	}
	if err := CfgRegenerate(home, "work", src); err != nil {
		t.Fatal(err)
	}
	simulaConfig(t, home, func(m map[string]any) { m["model"] = "x" })
	d, err := CfgRegenerateReport(home, "work", src)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(d.Adopted, []string{"model"}) {
		t.Fatalf("deriva = %+v", d)
	}
	if !isSymlink(ovPath) {
		t.Fatal("el overlay dejó de ser un symlink")
	}
	if data, _ := os.ReadFile(dotfiles); !strings.Contains(string(data), `"model"`) {
		t.Errorf("la adopción no llegó al destino del enlace: %s", data)
	}
	if fi, err := os.Stat(dotfiles); err != nil || fi.Mode().Perm() != 0o640 {
		t.Errorf("el destino perdió su modo: %v %v", fi, err)
	}
}

// El overlay se reescribe con tmp+rename sin cambiarle el modo (un 0600 puesto a
// propósito porque lleva una clave en env sigue siendo 0600).
func TestDerivaConservaElModoDelOverlay(t *testing.T) {
	home, src := perfilConBase(t, "", `{"env":{"TOKEN":"x"}}`)
	ovPath := cfgSettingsFile(home, "work")
	if err := os.Chmod(ovPath, 0o600); err != nil {
		t.Fatal(err)
	}
	simulaConfig(t, home, func(m map[string]any) { m["model"] = "x" })
	if _, err := CfgRegenerateReport(home, "work", src); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Lstat(ovPath); err != nil || fi.Mode().Perm() != 0o600 || !fi.Mode().IsRegular() {
		t.Errorf("modo del overlay = %v %v, quiero 0600 regular", fi, err)
	}
	if v, _ := jsonLookup(leeOverlay(t, home), []string{"model"}); v != "x" {
		t.Error("no se adoptó model")
	}
	ents, _ := os.ReadDir(cfgOverlayDir(home, "work"))
	for _, e := range ents {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Errorf("quedó un temporal en overlay/: %s", e.Name())
		}
	}
}

// La adopción vive en CfgRegenerate, no en `profile sync`: cualquier camino que
// regenere (aquí, fijar una variable de entorno del overlay como hace la GUI)
// guarda antes lo que /config había cambiado.
func TestDerivaSeAdoptaPorCualquierCamino(t *testing.T) {
	home, _ := perfilConBase(t, "", "")
	simulaConfig(t, home, func(m map[string]any) { m["autoCompactEnabled"] = false })
	if err := OverlayEnvSet(home, "work", "FOO", "1"); err != nil {
		t.Fatal(err)
	}
	ov := leeOverlay(t, home)
	if v, _ := jsonLookup(ov, []string{"autoCompactEnabled"}); v != false {
		t.Errorf("el cambio de /config se perdió al regenerar desde otro camino: %v", ov)
	}
	if v, _ := jsonLookup(ov, []string{"env", "FOO"}); v != "1" {
		t.Errorf("la variable nueva no está: %v", ov)
	}
	gen := leeGenerado(t, home)
	if v, _ := jsonLookup(gen, []string{"autoCompactEnabled"}); v != false {
		t.Errorf("generado = %v", gen)
	}
}

// Un overlay que no se puede escribir (un symlink a un almacén de dotfiles de
// solo lectura, como home-manager/Nix) no puede convertir en error una
// regeneración que antes de B6 funcionaba: lo que no se pudo guardar se cuenta
// como no guardado —nunca como adoptado— y se regenera igual. Si no, cada
// sync, upgrade o auto install fallaría para siempre en ese perfil.
func TestDerivaOverlayDeSoloLecturaNoBloqueaLaRegeneracion(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root escribe en un directorio 0555")
	}
	home, src := perfilConBase(t, "", "")
	ro := t.TempDir()
	dotfiles := filepath.Join(ro, "work.json")
	if err := os.WriteFile(dotfiles, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ovPath := cfgSettingsFile(home, "work")
	if err := os.Remove(ovPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(dotfiles, ovPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(ro, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(ro, 0o755) })
	if err := CfgRegenerate(home, "work", src); err != nil {
		t.Fatal(err)
	}
	simulaConfig(t, home, func(m map[string]any) { m["autoCompactEnabled"] = false })

	d, err := CfgRegenerateReport(home, "work", src)
	if err != nil {
		t.Fatalf("un overlay de solo lectura no puede hacer fallar la regeneración: %v", err)
	}
	if len(d.Adopted) != 0 {
		t.Errorf("Adopted = %v: no se guardó, no puede contarse como adoptado", d.Adopted)
	}
	if !reflect.DeepEqual(d.Unsaved, []string{"autoCompactEnabled"}) || d.UnsavedErr == "" || d.Empty() {
		t.Fatalf("deriva = %+v, quiero Unsaved=[autoCompactEnabled] con su motivo", d)
	}
	if _, ok := leeGenerado(t, home)["autoCompactEnabled"]; ok {
		t.Error("se tenía que regenerar como antes de B6")
	}
	if d, err := CfgRegenerateReport(home, "work", src); err != nil || !d.Empty() {
		t.Errorf("la siguiente regeneración ya no tiene deriva: %+v %v", d, err)
	}
}

// Si la regeneración falla después de mirar la deriva (aquí: cc-home inválido
// y overlay roto), la deriva lo dice: la copia del inválido está guardada, pero
// no se regeneró nada, y nadie debe contar que sí.
func TestDerivaMarcaCuandoNoSeRegenero(t *testing.T) {
	home, src := perfilConBase(t, "", "")
	p := filepath.Join(ccHomePath(home, "work"), "settings.json")
	if err := os.WriteFile(p, []byte("{roto"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfgSettingsFile(home, "work"), []byte("{overlay roto"), 0o644); err != nil {
		t.Fatal(err)
	}
	d, err := CfgRegenerateReport(home, "work", src)
	if err == nil {
		t.Fatal("con el overlay roto la regeneración tiene que fallar")
	}
	if d.Invalid == "" || !d.NotRegenerated {
		t.Fatalf("deriva = %+v, quiero Invalid y NotRegenerated", d)
	}
}

// #1 de la revisión: borrar del overlay una clave que /config cambió también es
// cambiar el overlay. Gana el overlay (la clave sigue borrada) y lo de /config
// queda en la copia de rescate.
func TestDerivaBorradoEnElOverlayGana(t *testing.T) {
	home, src := perfilConBase(t, "", `{"model":"a"}`)
	simulaConfig(t, home, func(m map[string]any) { m["model"] = "b" })
	if err := os.WriteFile(cfgSettingsFile(home, "work"), []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}
	d, err := CfgRegenerateReport(home, "work", src)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(d.Conflicts, []string{"model"}) || len(d.Adopted) != 0 || d.Rescued == "" {
		t.Fatalf("deriva = %+v, quiero Conflicts [model] con copia de rescate", d)
	}
	if _, ok := leeOverlay(t, home)["model"]; ok {
		t.Error("el borrado del overlay se deshizo")
	}
	if c, _ := os.ReadFile(d.Rescued); !strings.Contains(string(c), `"model":"b"`) {
		t.Errorf("la copia de rescate no tiene el valor de /config: %s", c)
	}
}

// #2: un overlay con "hooks": {} y la capa auto puesta no es un conflicto cuando
// el usuario añade un hook con /hooks: el overlay no cambió.
func TestDerivaHooksVaciosEnElOverlayConCapaAuto(t *testing.T) {
	home, src := perfilConBase(t, "", `{"hooks":{}}`)
	conCapaAuto(t, home, src, "/opt/ccp")
	simulaConfig(t, home, func(m map[string]any) {
		h := m["hooks"].(map[string]any)
		h["PreToolUse"] = []any{map[string]any{"matcher": "Bash", "hooks": []any{map[string]any{"type": "command", "command": "echo hi"}}}}
	})
	d, err := CfgRegenerateReport(home, "work", src)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Conflicts) != 0 || len(d.Adopted) == 0 {
		t.Fatalf("deriva = %+v, quiero el hook adoptado sin conflicto", d)
	}
	if _, ok := jsonLookup(leeGenerado(t, home), []string{"hooks", "PreToolUse"}); !ok {
		t.Error("el hook de /hooks se perdió al regenerar")
	}
}

// #4: con sensores y un statusLine en el overlay, cambiar la barra con
// /statusline se adopta (y la capa la vuelve a envolver); no es un conflicto.
func TestDerivaStatusLineDelOverlayConCapaAutoSeAdopta(t *testing.T) {
	home, src := perfilConBase(t, "", `{"statusLine":{"type":"command","command":"mi-barra-vieja"}}`)
	conCapaAuto(t, home, src, "/opt/ccp")
	simulaConfig(t, home, func(m map[string]any) {
		m["statusLine"] = map[string]any{"type": "command", "command": "mi-barra-nueva"}
	})
	d, err := CfgRegenerateReport(home, "work", src)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(d.Adopted, []string{"statusLine"}) || len(d.Conflicts) != 0 {
		t.Fatalf("deriva = %+v, quiero Adopted [statusLine]", d)
	}
	if v, _ := jsonLookup(leeOverlay(t, home), []string{"statusLine", "command"}); v != "mi-barra-nueva" {
		t.Errorf("overlay statusLine = %v", v)
	}
	if v, _ := jsonLookup(leeGenerado(t, home), []string{"statusLine", "command"}); !strings.Contains(fmt.Sprint(v), "mi-barra-nueva") || !strings.Contains(fmt.Sprint(v), "_statusline") {
		t.Errorf("generado statusLine = %v, quiero la barra nueva envuelta por los sensores", v)
	}
}

// #13: env.* no se adopta nunca: suelen ser tokens, y el overlay viaja en claro
// en los backups «sin secretos». Queda en la copia de rescate (0600, fuera de
// backups y snapshots) y se avisa.
func TestDerivaEnvNoSeAdopta(t *testing.T) {
	home, src := perfilConBase(t, "", "")
	simulaConfig(t, home, func(m map[string]any) {
		m["env"] = map[string]any{"ANTHROPIC_AUTH_TOKEN": "sk-FAKE"}
		m["model"] = "opus"
	})
	d, err := CfgRegenerateReport(home, "work", src)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(d.Skipped, []string{"env"}) || !reflect.DeepEqual(d.Adopted, []string{"model"}) || d.Rescued == "" {
		t.Fatalf("deriva = %+v, quiero Skipped [env], Adopted [model] y copia", d)
	}
	if b, _ := os.ReadFile(cfgSettingsFile(home, "work")); strings.Contains(string(b), "sk-FAKE") {
		t.Fatalf("el token acabó en el overlay: %s", b)
	}
	if fi, err := os.Stat(d.Rescued); err != nil || fi.Mode().Perm() != 0o600 {
		t.Errorf("la copia de rescate no es 0600: %v %v", fi, err)
	}
}

// #11: en un conflicto gana el overlay, pero el valor de /config no se pierde sin
// rastro: queda en la copia de rescate. Y dos copias distintas no se pisan.
func TestDerivaConflictoDejaCopiaYNoSePisan(t *testing.T) {
	home, src := perfilConBase(t, "", `{"model":"a"}`)
	var rescued []string
	for _, v := range []string{"b", "b2"} {
		simulaConfig(t, home, func(m map[string]any) { m["model"] = v })
		if err := os.WriteFile(cfgSettingsFile(home, "work"), []byte(`{"model":"c-`+v+`"}`), 0o644); err != nil {
			t.Fatal(err)
		}
		d, err := CfgRegenerateReport(home, "work", src)
		if err != nil || d.Rescued == "" {
			t.Fatalf("deriva = %+v %v", d, err)
		}
		rescued = append(rescued, d.Rescued)
	}
	if rescued[0] == rescued[1] {
		t.Fatal("dos copias de rescate distintas comparten ruta: la segunda pisa la primera")
	}
	for i, v := range []string{"b", "b2"} {
		if c, _ := os.ReadFile(rescued[i]); !strings.Contains(string(c), `"model":"`+v+`"`) {
			t.Errorf("copia %d = %s", i, c)
		}
	}
}

// #6: CfgRegenerate (el camino de profile config, instruct, la GUI, los restores)
// no cuenta la deriva, así que la deja pendiente; el siguiente ProfileSyncReport
// la devuelve una vez y la borra.
func TestDerivaPendienteLaEnsenaElSiguienteSync(t *testing.T) {
	home, src := perfilConBase(t, "", `{"model":"a"}`)
	simulaConfig(t, home, func(m map[string]any) { m["model"] = "b" })
	if err := os.WriteFile(cfgSettingsFile(home, "work"), []byte(`{"model":"c"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := CfgRegenerate(home, "work", src); err != nil {
		t.Fatal(err)
	}
	ds, err := ProfileSyncReport(home, "work")
	if err != nil {
		t.Fatal(err)
	}
	if len(ds) != 1 || !reflect.DeepEqual(ds[0].Conflicts, []string{"model"}) || ds[0].Rescued == "" {
		t.Fatalf("sync = %+v, quiero el conflicto que dejó pendiente la regeneración anterior", ds)
	}
	if ds, err := ProfileSyncReport(home, "work"); err != nil || len(ds) != 0 {
		t.Errorf("el pendiente se enseña una sola vez: %+v %v", ds, err)
	}
}
