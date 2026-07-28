package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// bootstrap_test.go — la detección de huecos, su aplicación y la caché de
// «preguntar una vez».
//
// La detección se prueba SIN disco y SIN tty: es pura, y esa pureza es lo que
// permite fijar aquí los cinco escenarios en una tabla en vez de montar cinco
// repos de mentira.

const bootstrapCwd = "/tmp/wk/repo"

func bootstrapProfiles() map[string]Profile {
	return map[string]Profile{
		"p1": {Type: "official"},
		"p2": {Type: "official"},
	}
}

// bootstrapFullAuto es un bloque auto_handoff sano: cadena con los dos perfiles,
// gate que autoriza a p1 a prestarle a p2, y sensores instalados en ambos.
func bootstrapFullAuto() *AutoHandoff {
	return &AutoHandoff{
		Enabled:   true,
		Policies:  map[string]AutoPolicy{"default": {Fallback: []string{"p1", "p2"}}},
		AllowFrom: map[string][]string{"p1": {"p1", "p2"}},
		Hooks:     []string{"p1", "p2"},
	}
}

func bootstrapMissing(p BootstrapPlan) map[BootstrapKind]bool {
	out := map[BootstrapKind]bool{}
	for _, it := range p.Items {
		out[it.Kind] = it.Missing
	}
	return out
}

func bootstrapItem(p BootstrapPlan, k BootstrapKind) BootstrapItem {
	for _, it := range p.Items {
		if it.Kind == k {
			return it
		}
	}
	return BootstrapItem{}
}

func TestBootstrapDetectaHuecos(t *testing.T) {
	cases := []struct {
		name  string
		cfg   *Config
		in    BootstrapInput
		want  map[BootstrapKind]bool
		check func(t *testing.T, p BootstrapPlan)
	}{
		{
			// Repo virgen con regla: falta el bloque entero. La fila de cadena
			// tiene que salir OK aunque HOY no haya bloque, porque se calcula
			// sobre el que AutoInit va a sembrar; si saliera «vacía» el resumen
			// asustaría con un problema que se resuelve solo.
			name: "sin bloque auto_handoff",
			cfg:  &Config{Profiles: bootstrapProfiles(), Rules: []Rule{{Path: bootstrapCwd, Profile: "p1"}}},
			in:   BootstrapInput{Cwd: bootstrapCwd},
			want: map[BootstrapKind]bool{
				BootstrapAuto: true, BootstrapRule: false, BootstrapChain: false, BootstrapSensors: true,
			},
			check: func(t *testing.T, p BootstrapPlan) {
				if p.Primary != "p1" {
					t.Fatalf("primario = %q, quería p1", p.Primary)
				}
				if got := bootstrapItem(p, BootstrapSensors).Profiles; len(got) != 2 {
					t.Fatalf("sensores a instalar = %v, quería p1 y p2", got)
				}
			},
		},
		{
			// La regla se propone sobre la RAÍZ del repo, no sobre el cwd, y el
			// perfil sale del activo de la terminal.
			name: "sin regla",
			cfg:  &Config{Profiles: bootstrapProfiles(), AutoHandoff: bootstrapFullAuto()},
			in:   BootstrapInput{Cwd: bootstrapCwd, Repo: "/tmp/wk", Active: "p1"},
			want: map[BootstrapKind]bool{
				BootstrapAuto: false, BootstrapRule: true, BootstrapChain: false, BootstrapSensors: false,
			},
			check: func(t *testing.T, p BootstrapPlan) {
				it := bootstrapItem(p, BootstrapRule)
				if it.Path != "/tmp/wk" || it.Profile != "p1" || !it.FromGit {
					t.Fatalf("regla propuesta = %+v", it)
				}
				// El primario del plan es el de DESPUÉS de la regla: si fuera el
				// de ahora ("default"), las filas de cadena y sensores hablarían
				// de otro repo.
				if p.Primary != "p1" {
					t.Fatalf("primario = %q, quería p1 (con la regla propuesta ya contada)", p.Primary)
				}
			},
		},
		{
			// Sin perfil activo y con varios donde elegir no se adivina: el hueco
			// se reporta, pero sin perfil, y BootstrapApply lo salta.
			name: "sin regla y sin perfil deducible",
			cfg: &Config{Profiles: bootstrapProfiles(), AutoHandoff: &AutoHandoff{
				Enabled:  true,
				Policies: map[string]AutoPolicy{"default": {Fallback: []string{"p1", "p2"}}},
				Hooks:    []string{"p1", "p2"},
			}},
			in: BootstrapInput{Cwd: bootstrapCwd, Repo: "/tmp/wk"},
			want: map[BootstrapKind]bool{
				BootstrapAuto: false, BootstrapRule: true, BootstrapChain: false, BootstrapSensors: false,
			},
			check: func(t *testing.T, p BootstrapPlan) {
				if got := bootstrapItem(p, BootstrapRule).Profile; got != "" {
					t.Fatalf("perfil adivinado %q: con varios candidatos no debe elegir ninguno", got)
				}
			},
		},
		{
			// El mismo caso PERO con el gate en deny total. Sin regla no se sabe
			// quién va a ser el primario, así que ResolveAutoChain resuelve contra
			// `default` —el ~/.claude llano— y ensanchar ahí escribiría permisos de
			// préstamo hacia terceros para un primario que el usuario no ha elegido.
			// El hueco se enseña (Blocked) y se aplaza; NO se aplica.
			name: "el gate no se ensancha sin saber el primario",
			cfg: &Config{Profiles: bootstrapProfiles(), AutoHandoff: &AutoHandoff{
				Enabled:   true,
				Policies:  map[string]AutoPolicy{"default": {Fallback: []string{"p1", "p2"}}},
				AllowFrom: map[string][]string{"otro": {"otro"}},
				Hooks:     []string{"p1", "p2"},
			}},
			in: BootstrapInput{Cwd: bootstrapCwd, Repo: "/tmp/wk"},
			want: map[BootstrapKind]bool{
				BootstrapAuto: false, BootstrapRule: true, BootstrapChain: false, BootstrapSensors: false,
			},
			check: func(t *testing.T, p BootstrapPlan) {
				it := bootstrapItem(p, BootstrapChain)
				if !it.Blocked {
					t.Fatalf("el hueco de cadena no quedó aplazado: %+v", it)
				}
				if len(it.Profiles) == 0 {
					t.Fatal("un hueco aplazado tiene que decir a quién afecta")
				}
				for _, g := range p.Gaps() {
					if g.Kind == BootstrapChain {
						t.Fatal("un hueco aplazado no se puede aplicar")
					}
				}
			},
		},
		{
			// La raíz git que NO cubre el cwd (queda fuera tras resolver enlaces) no
			// sirve de ancla: la regla cae sobre el cwd, pero se conserva la raíz
			// para poder decir POR QUÉ. «Esto no es un repo git» sería falso.
			name: "raíz git que no ancla el cwd",
			cfg:  &Config{Profiles: bootstrapProfiles(), AutoHandoff: bootstrapFullAuto()},
			in:   BootstrapInput{Cwd: bootstrapCwd, Repo: "/otro/sitio", Active: "p1"},
			want: map[BootstrapKind]bool{BootstrapRule: true},
			check: func(t *testing.T, p BootstrapPlan) {
				it := bootstrapItem(p, BootstrapRule)
				if it.FromGit {
					t.Fatalf("una raíz que no cubre el cwd no puede darse por buena: %+v", it)
				}
				if it.Path != bootstrapCwd {
					t.Fatalf("la regla no cayó sobre el cwd: %q", it.Path)
				}
				if it.GitRoot != "/otro/sitio" {
					t.Fatalf("se perdió la raíz git y con ella el motivo: %q", it.GitRoot)
				}
			},
		},
		{
			// allow_from declarado SIN entrada para el primario = deny total: la
			// cadena queda vacía aunque el fallback tenga perfiles.
			name: "cadena vacía por el gate",
			cfg: &Config{
				Profiles: bootstrapProfiles(),
				Rules:    []Rule{{Path: bootstrapCwd, Profile: "p1"}},
				AutoHandoff: &AutoHandoff{
					Enabled:   true,
					Policies:  map[string]AutoPolicy{"default": {Fallback: []string{"p1", "p2"}}},
					AllowFrom: map[string][]string{"otro": {"otro"}},
					Hooks:     []string{"p1", "p2"},
				},
			},
			in: BootstrapInput{Cwd: bootstrapCwd},
			want: map[BootstrapKind]bool{
				BootstrapAuto: false, BootstrapRule: false, BootstrapChain: true, BootstrapSensors: false,
			},
			check: func(t *testing.T, p BootstrapPlan) {
				it := bootstrapItem(p, BootstrapChain)
				if len(it.Profiles) != 1 || it.Profiles[0] != "p2" {
					t.Fatalf("denegados = %v, quería [p2]", it.Profiles)
				}
				if it.Policy != "default" {
					t.Fatalf("política = %q", it.Policy)
				}
			},
		},
		{
			// Una cadena vacía SIN denegados no es un hueco: no hay más perfiles,
			// y ensanchar el gate no crearía ninguno.
			name: "cadena vacía sin denegados no es hueco",
			cfg: &Config{
				Profiles: map[string]Profile{"p1": {Type: "official"}},
				Rules:    []Rule{{Path: bootstrapCwd, Profile: "p1"}},
				AutoHandoff: &AutoHandoff{
					Enabled:  true,
					Policies: map[string]AutoPolicy{"default": {Fallback: []string{"p1"}}},
					Hooks:    []string{"p1"},
				},
			},
			in: BootstrapInput{Cwd: bootstrapCwd},
			want: map[BootstrapKind]bool{
				BootstrapAuto: false, BootstrapRule: false, BootstrapChain: false, BootstrapSensors: false,
			},
		},
		{
			name: "sensores ausentes",
			cfg: &Config{
				Profiles: bootstrapProfiles(),
				Rules:    []Rule{{Path: bootstrapCwd, Profile: "p1"}},
				AutoHandoff: &AutoHandoff{
					Enabled:   true,
					Policies:  map[string]AutoPolicy{"default": {Fallback: []string{"p1", "p2"}}},
					AllowFrom: map[string][]string{"p1": {"p1", "p2"}},
					Hooks:     []string{"p1"},
				},
			},
			in: BootstrapInput{Cwd: bootstrapCwd},
			want: map[BootstrapKind]bool{
				BootstrapAuto: false, BootstrapRule: false, BootstrapChain: false, BootstrapSensors: true,
			},
			check: func(t *testing.T, p BootstrapPlan) {
				got := bootstrapItem(p, BootstrapSensors).Profiles
				if len(got) != 1 || got[0] != "p2" {
					t.Fatalf("sensores a instalar = %v, quería solo [p2]", got)
				}
			},
		},
		{
			name: "todo ok",
			cfg: &Config{
				Profiles:    bootstrapProfiles(),
				Rules:       []Rule{{Path: bootstrapCwd, Profile: "p1"}},
				AutoHandoff: bootstrapFullAuto(),
			},
			in: BootstrapInput{Cwd: bootstrapCwd},
			want: map[BootstrapKind]bool{
				BootstrapAuto: false, BootstrapRule: false, BootstrapChain: false, BootstrapSensors: false,
			},
			check: func(t *testing.T, p BootstrapPlan) {
				if p.HasGaps() || len(p.Gaps()) != 0 {
					t.Fatalf("un repo configurado no tiene huecos: %+v", p.Gaps())
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			plan, err := BootstrapDetect(tc.cfg, tc.in)
			if err != nil {
				t.Fatalf("BootstrapDetect: %v", err)
			}
			if len(plan.Items) != 4 {
				t.Fatalf("el resumen tiene que enseñar los CUATRO chequeos, salieron %d", len(plan.Items))
			}
			got := bootstrapMissing(plan)
			for k, want := range tc.want {
				if got[k] != want {
					t.Fatalf("hueco %q = %v, quería %v (plan: %+v)", k, got[k], want, plan.Items)
				}
			}
			if tc.check != nil {
				tc.check(t, plan)
			}
		})
	}
}

// TestBootstrapDetectNoMutaElConfig fija que la detección es pura: simula la
// regla y el bloque sobre copias, nunca sobre el Config del llamador.
func TestBootstrapDetectNoMutaElConfig(t *testing.T) {
	cfg := &Config{Profiles: bootstrapProfiles()}
	if _, err := BootstrapDetect(cfg, BootstrapInput{Cwd: bootstrapCwd, Repo: "/tmp/wk", Active: "p1"}); err != nil {
		t.Fatal(err)
	}
	if cfg.AutoHandoff != nil {
		t.Fatal("la detección sembró auto_handoff en el Config del llamador")
	}
	if len(cfg.Rules) != 0 {
		t.Fatalf("la detección escribió reglas en el Config del llamador: %+v", cfg.Rules)
	}
}

// TestBootstrapDetectErrorDeConfig: una política que no existe no es un hueco
// que el bootstrap sepa cerrar. Devuelve error para que el llamador se salte el
// flujo entero y deje hablar al validador de `ccp session`.
func TestBootstrapDetectErrorDeConfig(t *testing.T) {
	cfg := &Config{Profiles: bootstrapProfiles(), AutoHandoff: bootstrapFullAuto()}
	_, err := BootstrapDetect(cfg, BootstrapInput{Cwd: bootstrapCwd, Policy: "fantasma"})
	if err == nil {
		t.Fatal("una política inexistente tiene que devolver error")
	}
}

// --- aplicación --------------------------------------------------------------

// bootstrapHome monta un home real con dos perfiles official. CCP_CLAUDE_SRC
// apunta a un temporal para que ProfileSync no lea el ~/.claude del usuario.
func bootstrapHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("CCP_CLAUDE_SRC", t.TempDir())
	for _, p := range []string{"p1", "p2"} {
		if err := ProfileAddOfficial(home, p); err != nil {
			t.Fatal(err)
		}
	}
	return home
}

// TestBootstrapAplicaLosCuatroHuecos comprueba el efecto en disco, que es lo
// único que cuenta: el yaml después de aplicar tiene que dejar el repo listo
// para que ResolveAutoChain devuelva cadena.
func TestBootstrapAplicaLosCuatroHuecos(t *testing.T) {
	home := bootstrapHome(t)
	repo := filepath.Join(t.TempDir(), "proyecto")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(home)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := BootstrapDetect(cfg, BootstrapInput{Cwd: repo, Repo: repo, Active: "p1"})
	if err != nil {
		t.Fatal(err)
	}
	applied, err := BootstrapApply(home, plan)
	if err != nil {
		t.Fatalf("BootstrapApply: %v", err)
	}
	if len(applied.Kinds) == 0 {
		t.Fatal("no se aplicó nada")
	}

	after, err := Load(home)
	if err != nil {
		t.Fatal(err)
	}
	if after.AutoHandoff == nil || !after.AutoHandoff.Enabled {
		t.Fatal("auto_handoff no quedó sembrado")
	}
	if got := Resolve(repo, after.Rules); got != "p1" {
		t.Fatalf("la regla no enruta el repo: %q", got)
	}
	rc, err := ResolveAutoChain(home, after, "", repo)
	if err != nil {
		t.Fatal(err)
	}
	if len(rc.Fallback) == 0 {
		t.Fatalf("tras el bootstrap la cadena sigue vacía (denegados: %v)", rc.Denied)
	}
	for _, n := range append([]string{"p1"}, rc.Fallback...) {
		if !AutoHooksEnabled(after, n) {
			t.Fatalf("el perfil %q se quedó sin sensores", n)
		}
	}
	// La capa de sensores tiene que estar en el settings.json, no solo en la
	// lista `hooks`: la lista es la fuente de verdad, pero quien la lee es CC.
	data, err := os.ReadFile(filepath.Join(home, "profiles", "p1", "cc-home", "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "_statusline") {
		t.Fatalf("settings.json de p1 sin la capa auto:\n%s", data)
	}
}

// TestBootstrapAplicaSinPerfilNoEscribeRegla: el hueco de regla sin perfil
// deducible se salta en silencio (el resumen ya se lo dijo al usuario). Escribir
// la regla de un perfil adivinado enrutaría el repo a la cuenta equivocada.
func TestBootstrapAplicaSinPerfilNoEscribeRegla(t *testing.T) {
	home := bootstrapHome(t)
	repo := filepath.Join(t.TempDir(), "proyecto")

	cfg, err := Load(home)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := BootstrapDetect(cfg, BootstrapInput{Cwd: repo, Repo: repo})
	if err != nil {
		t.Fatal(err)
	}
	applied, err := BootstrapApply(home, plan)
	if err != nil {
		t.Fatalf("BootstrapApply: %v", err)
	}
	if applied.Rule != "" {
		t.Fatalf("escribió una regla sin saber a qué perfil: %q", applied.Rule)
	}
	// Y lo dice: un hueco que se enseñó y no se cerró tiene que salir en el parte.
	// Callarlo dejaba al usuario creyendo que su repo quedó enrutado, con la marca
	// de «preguntar una vez» ya gastada.
	if len(applied.Skipped) != 1 || applied.Skipped[0].Kind != BootstrapRule {
		t.Fatalf("el hueco saltado no se reporta: %+v", applied.Skipped)
	}
	after, err := Load(home)
	if err != nil {
		t.Fatal(err)
	}
	if len(after.Rules) != 0 {
		t.Fatalf("quedaron reglas: %+v", after.Rules)
	}
	// El resto del plan sí se aplica: que no sepamos el perfil no es motivo para
	// dejar el bloque auto_handoff sin sembrar.
	if after.AutoHandoff == nil {
		t.Fatal("el hueco de la regla bloqueó los demás")
	}
}

// --- la caché ----------------------------------------------------------------

func TestBootstrapMarkCache(t *testing.T) {
	home := t.TempDir()
	repo := "/tmp/wk/proyecto"

	if _, ok := ReadBootstrapMark(home, repo); ok {
		t.Fatal("sin escribir nada no puede haber marca")
	}
	if err := WriteBootstrapMark(home, BootstrapMark{Repo: repo, Accepted: false}); err != nil {
		t.Fatal(err)
	}
	m, ok := ReadBootstrapMark(home, repo)
	if !ok {
		t.Fatal("la marca recién escrita no se lee")
	}
	if m.Accepted {
		t.Fatal("Accepted se guardó al revés")
	}
	if m.AskedAt.IsZero() {
		t.Fatal("la marca no lleva instante: el mtime no vale, lo pisa cualquier copia")
	}

	// Otro repo, otra marca: la caché es por repo.
	if _, ok := ReadBootstrapMark(home, "/tmp/wk/otro"); ok {
		t.Fatal("la marca de un repo se leyó para otro")
	}

	// Borrarla vuelve a dejar el repo «sin preguntar». Es lo que la convierte en
	// caché: borrarla cuesta que te vuelvan a preguntar, nunca corrección.
	if err := os.RemoveAll(filepath.Join(AutoStateDir(home), "bootstrap")); err != nil {
		t.Fatal(err)
	}
	if _, ok := ReadBootstrapMark(home, repo); ok {
		t.Fatal("tras borrar la caché sigue habiendo marca")
	}
}

// TestBootstrapMarkBasura: una marca corrupta, o una cuyo `repo` no coincide con
// el que se pregunta, se trata como ausencia. Nunca como la marca de otro repo:
// el nombre del archivo lleva el basename saneado y truncado y podría colisionar.
func TestBootstrapMarkBasura(t *testing.T) {
	home := t.TempDir()
	repo := "/tmp/wk/proyecto"

	if err := WriteBootstrapMark(home, BootstrapMark{Repo: repo, AskedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(AutoStateDir(home), "bootstrap")
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("entries=%v err=%v", entries, err)
	}
	path := filepath.Join(dir, entries[0].Name())

	if err := os.WriteFile(path, []byte("{no soy json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, ok := ReadBootstrapMark(home, repo); ok {
		t.Fatal("un JSON roto no puede contar como marca")
	}

	if err := os.WriteFile(path, []byte(`{"repo":"/otro/sitio"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, ok := ReadBootstrapMark(home, repo); ok {
		t.Fatal("una marca de otro repo no puede contar como la de este")
	}
}

// TestBootstrapMarkNombreEstable: el nombre lleva el basename (para reconocerlo)
// y un hash de la ruta completa (para que dos repos con el mismo basename no
// compartan marca).
func TestBootstrapMarkNombreEstable(t *testing.T) {
	a := bootstrapMarkName("/home/x/trabajo/api")
	b := bootstrapMarkName("/home/x/personal/api")
	if a == b {
		t.Fatalf("dos repos distintos con el mismo basename colisionan: %s", a)
	}
	if a != bootstrapMarkName("/home/x/trabajo/api") {
		t.Fatal("el nombre no es estable entre llamadas")
	}
	if !strings.HasPrefix(a, "api-") || !strings.HasSuffix(a, ".json") {
		t.Fatalf("nombre poco reconocible: %s", a)
	}
	// Una ruta con separadores raros no puede escaparse del directorio.
	if n := bootstrapMarkName("/a/../../etc/passwd"); strings.Contains(n, "/") || strings.Contains(n, "..") {
		t.Fatalf("nombre peligroso: %s", n)
	}
}
