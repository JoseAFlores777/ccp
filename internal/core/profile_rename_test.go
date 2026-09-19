package core

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// profile_rename_test.go — el nombre de un perfil vive en cinco sitios
// (ccp.yaml profiles, ccp.yaml rules, el bloque auto_handoff de ccp.yaml,
// <home>/profiles/<name>/ y los marcadores de handoffs.yaml). Renombrar tocando
// solo uno deja el estado incoherente: una regla apuntando a un perfil
// inexistente resuelve a `default` en silencio, una cadena de rotación con el
// nombre viejo deja de prestar esa cuenta, y un marcador con el nombre viejo ya
// no se puede terminar.

// seedRenameHome deja un home con dos perfiles, reglas hacia ambos, un
// artefacto authored del perfil a renombrar y su directorio con api_key.
func seedRenameHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	c := &Config{
		Version:  SchemaVersion,
		Profiles: map[string]Profile{"viejo": {Type: "official"}, "otro": {Type: "official"}},
		Rules: []Rule{
			{Path: "/repo/uno", Profile: "viejo"},
			{Path: "/repo/dos", Profile: "otro"},
			{Path: "/repo/tres", Profile: "viejo"},
		},
		Authored: []Authored{
			{Scope: "profile", Profile: "viejo", Type: "skill", Ref: "x", Desc: "d"},
			{Scope: "global", Type: "skill", Ref: "y", Desc: "d"},
		},
	}
	if err := Save(home, c); err != nil {
		t.Fatal(err)
	}
	dir := profileDirPath(home, "viejo")
	if err := os.MkdirAll(filepath.Join(dir, "cc-home"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "api_key"), []byte("sk-secreta\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return home
}

func TestProfileRenameMueveTodoElEstado(t *testing.T) {
	home := seedRenameHome(t)
	t.Setenv("CCP_CLAUDE_SRC", t.TempDir()) // seed/regenerate sin tocar ~/.claude

	if _, err := ProfileRename(home, "viejo", "nuevo"); err != nil {
		t.Fatalf("rename: %v", err)
	}
	c, err := Load(home)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := c.Profiles["viejo"]; ok {
		t.Error("el perfil viejo sigue en ccp.yaml")
	}
	if p, ok := c.Profiles["nuevo"]; !ok || p.Type != "official" {
		t.Fatalf("el perfil nuevo no quedó bien: %+v", c.Profiles)
	}
	for _, r := range c.Rules {
		if r.Profile == "viejo" {
			t.Fatalf("una regla sigue apuntando al nombre viejo: %+v", c.Rules)
		}
	}
	var haciaNuevo int
	for _, r := range c.Rules {
		if r.Profile == "nuevo" {
			haciaNuevo++
		}
	}
	if haciaNuevo != 2 {
		t.Fatalf("esperaba 2 reglas hacia el nombre nuevo, got %+v", c.Rules)
	}
	if c.Authored[0].Profile != "nuevo" {
		t.Errorf("el authored de scope profile no se renombró: %+v", c.Authored[0])
	}
	if c.Authored[1].Profile != "" {
		t.Errorf("el authored global no debe ganar perfil: %+v", c.Authored[1])
	}

	// El directorio viaja entero: la api_key y el login (cc-home) se conservan.
	if _, err := os.Stat(profileDirPath(home, "viejo")); !os.IsNotExist(err) {
		t.Error("el directorio viejo sigue existiendo")
	}
	key, err := os.ReadFile(filepath.Join(profileDirPath(home, "nuevo"), "api_key"))
	if err != nil || !strings.Contains(string(key), "sk-secreta") {
		t.Fatalf("la api_key no viajó con el perfil: %v", err)
	}
}

func TestProfileRenameReescribeMarcadoresDeHandoff(t *testing.T) {
	home := seedRenameHome(t)
	t.Setenv("CCP_CLAUDE_SRC", t.TempDir())
	if err := SaveHandoffs(home, &Handoffs{
		Version: HandoffsVersion,
		Active: []Marker{
			{Session: "aaa", Slug: "-r", Cwd: "/r", From: "viejo", To: "otro", Since: "2026-07-01T00:00:00Z"},
			{Session: "bbb", Slug: "-s", Cwd: "/s", From: "otro", To: "viejo", Since: "2026-07-01T00:00:00Z"},
		},
		Archived: []ArchivedMarker{
			{Session: "ccc", From: "viejo", To: "otro", Slug: "-r", ReturnedAs: "ddd", Since: "x", Ended: "y"},
		},
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := ProfileRename(home, "viejo", "nuevo"); err != nil {
		t.Fatalf("rename: %v", err)
	}
	h, err := LoadHandoffs(home)
	if err != nil {
		t.Fatal(err)
	}
	if h.Active[0].From != "nuevo" || h.Active[1].To != "nuevo" {
		t.Fatalf("los marcadores activos no se renombraron: %+v", h.Active)
	}
	if h.Archived[0].From != "nuevo" {
		t.Fatalf("el historial no se renombró: %+v", h.Archived)
	}
	if h.Active[0].To != "otro" {
		t.Fatalf("renombró un perfil que no tocaba: %+v", h.Active)
	}
}

func TestProfileRenameRechaza(t *testing.T) {
	home := seedRenameHome(t)
	t.Setenv("CCP_CLAUDE_SRC", t.TempDir())
	for _, tc := range []struct{ name, old, new, want string }{
		{"origen default", "default", "x", "default"},
		{"destino default", "viejo", "default", "default"},
		{"origen inexistente", "fantasma", "x", "no existe"},
		{"destino ocupado", "viejo", "otro", "ya existe"},
		{"mismo nombre", "viejo", "viejo", "ya existe"},
		{"destino vacío", "viejo", "", "inválido"},
		{"destino con barra", "viejo", "a/b", "inválido"},
		{"destino con ..", "viejo", "..", "inválido"},
	} {
		_, err := ProfileRename(home, tc.old, tc.new)
		if err == nil {
			t.Errorf("%s: esperaba error", tc.name)
			continue
		}
		if !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: mensaje %q no contiene %q", tc.name, err, tc.want)
		}
	}
	// Ningún rechazo puede haber tocado el estado.
	c, _ := Load(home)
	if _, ok := c.Profiles["viejo"]; !ok {
		t.Fatal("un rename rechazado no debe borrar el perfil")
	}
	if _, err := os.Stat(profileDirPath(home, "viejo")); err != nil {
		t.Fatal("un rename rechazado no debe mover el directorio")
	}
}

// TestProfileRenameRevierteSiFallaElConfig: el directorio se mueve antes de
// escribir ccp.yaml (así el fallo típico —permisos, disco— ocurre con el
// config aún íntegro). Si la escritura falla igualmente, el directorio tiene
// que volver a su sitio: un ccp.yaml apuntando a un perfil sin directorio deja
// al usuario sin login y sin key.
func TestProfileRenameRevierteSiFallaElConfig(t *testing.T) {
	home := seedRenameHome(t)
	t.Setenv("CCP_CLAUDE_SRC", t.TempDir())
	// ccp.yaml.tmp como directorio: el tmp+rename de Save falla.
	if err := os.Mkdir(filepath.Join(home, "ccp.yaml.tmp"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := ProfileRename(home, "viejo", "nuevo"); err == nil {
		t.Fatal("esperaba error al persistir ccp.yaml")
	}
	if _, err := os.Stat(profileDirPath(home, "viejo")); err != nil {
		t.Fatalf("el directorio no volvió a su sitio: %v", err)
	}
	if _, err := os.Stat(profileDirPath(home, "nuevo")); !os.IsNotExist(err) {
		t.Fatal("quedó el directorio con el nombre nuevo")
	}
	c, _ := Load(home)
	if _, ok := c.Profiles["viejo"]; !ok {
		t.Fatal("ccp.yaml no debería haber cambiado")
	}
}

// seedRenameAutoHome es seedRenameHome con un bloque auto_handoff que nombra a
// «viejo» en los tres sitios que llevan perfiles: el fallback de las políticas,
// allow_from (clave y listas) y hooks. Lleva además un nombre entre espacios
// (así lo deja una edición a mano, y los lectores del bloque lo recortan) y
// claves que este binario no conoce, que el rename tiene que conservar.
func seedRenameAutoHome(t *testing.T) string {
	t.Helper()
	home := seedRenameHome(t)
	c, err := Load(home)
	if err != nil {
		t.Fatal(err)
	}
	c.Profiles["cliente"] = Profile{Type: "official"}
	c.AutoHandoff = &AutoHandoff{
		Enabled: true,
		Policies: map[string]AutoPolicy{
			"default": {Fallback: []string{"otro", "viejo"}, Threshold: 80, Extra: map[string]any{"park_wait": "5m"}},
			"noche":   {Fallback: []string{" viejo ", "cliente"}},
			"sin-el":  {Fallback: []string{"otro"}},
		},
		AllowFrom: map[string][]string{
			"viejo":   {"viejo", "otro"},
			"otro":    {"otro", "viejo"},
			"cliente": {"cliente"},
		},
		Hooks: []string{"otro", "viejo"},
		Extra: map[string]any{"statusline_augment": true},
	}
	if err := Save(home, c); err != nil {
		t.Fatal(err)
	}
	return home
}

// TestProfileRenameRenombraAutoHandoff: sin esto, tras un rename la política
// nombraba a un perfil inexistente (`ccp session` fallaba con «el perfil de
// fallback no existe»), el allow_from del perfil dejaba de encontrarse y la
// siguiente regeneración de su cc-home le quitaba los sensores.
func TestProfileRenameRenombraAutoHandoff(t *testing.T) {
	home := seedRenameAutoHome(t)
	t.Setenv("CCP_CLAUDE_SRC", t.TempDir())

	if _, err := ProfileRename(home, "viejo", "nuevo"); err != nil {
		t.Fatalf("rename: %v", err)
	}
	c, err := Load(home)
	if err != nil {
		t.Fatal(err)
	}
	ah := c.AutoHandoff
	for pol, want := range map[string][]string{
		"default": {"otro", "nuevo"},
		"noche":   {"nuevo", "cliente"},
		"sin-el":  {"otro"},
	} {
		if got := ah.Policies[pol].Fallback; !slices.Equal(got, want) {
			t.Errorf("fallback de %q = %q, quiero %q", pol, got, want)
		}
	}
	if _, ok := ah.AllowFrom["viejo"]; ok {
		t.Errorf("allow_from conserva la clave vieja: %v", ah.AllowFrom)
	}
	for primary, want := range map[string][]string{
		"nuevo":   {"nuevo", "otro"},
		"otro":    {"otro", "nuevo"},
		"cliente": {"cliente"},
	} {
		if got := ah.AllowFrom[primary]; !slices.Equal(got, want) {
			t.Errorf("allow_from.%s = %q, quiero %q", primary, got, want)
		}
	}
	if !slices.Equal(ah.Hooks, []string{"otro", "nuevo"}) {
		t.Errorf("hooks = %q", ah.Hooks)
	}
	// Lo que no es un nombre de perfil viaja intacto.
	if d := ah.Policies["default"]; d.Threshold != 80 || d.Extra["park_wait"] != "5m" {
		t.Errorf("la política perdió sus ajustes: %+v", d)
	}
	if ah.Extra["statusline_augment"] != true || !ah.Enabled {
		t.Errorf("el bloque perdió sus claves: %+v", ah)
	}

	// Lo que de verdad importa: la cuenta renombrada sigue prestándose, y como
	// primario conserva su propio gate.
	rc, err := ResolveAutoChain(home, nil, "default", "/repo/dos")
	if err != nil {
		t.Fatalf("la cadena desde otro no resuelve: %v", err)
	}
	if !slices.Equal(rc.Fallback, []string{"nuevo"}) || len(rc.Denied) != 0 {
		t.Errorf("desde otro: fallback %q, denegados %q", rc.Fallback, rc.Denied)
	}
	rc, err = ResolveAutoChain(home, nil, "default", "/repo/uno")
	if err != nil {
		t.Fatalf("la cadena desde nuevo no resuelve: %v", err)
	}
	if rc.Primary != "nuevo" || !slices.Equal(rc.Fallback, []string{"otro"}) || len(rc.Denied) != 0 {
		t.Errorf("desde nuevo: %+v", rc)
	}

	// Y ProfileSync, al regenerar el cc-home con el nombre nuevo, le vuelve a
	// poner los sensores.
	settings, err := os.ReadFile(filepath.Join(ccHomePath(home, "nuevo"), "settings.json"))
	if err != nil {
		t.Fatalf("no se regeneró el settings.json: %v", err)
	}
	for _, want := range []string{autoStatusLineCmd, autoLimitHookCmd} {
		if !strings.Contains(string(settings), want) {
			t.Errorf("el settings.json regenerado no lleva %s:\n%s", want, settings)
		}
	}
}

// TestProfileRenameDeshaceCcpYamlEnteroSiFallanLosMarcadores: ccp.yaml se
// escribe (reglas y auto_handoff en el mismo Save) y luego falla handoffs.yaml.
// Deshacer tiene que dejar ccp.yaml byte a byte como estaba. La regla huérfana
// hacia «nuevo» es la prueba de que se guarda lo leído en vez de invertir el
// cambio a mano: una inversión nuevo→viejo la habría convertido en una regla
// del perfil viejo.
func TestProfileRenameDeshaceCcpYamlEnteroSiFallanLosMarcadores(t *testing.T) {
	home := seedRenameAutoHome(t)
	t.Setenv("CCP_CLAUDE_SRC", t.TempDir())
	c, err := Load(home)
	if err != nil {
		t.Fatal(err)
	}
	c.Rules = append(c.Rules, Rule{Path: "/repo/huerfana", Profile: "nuevo"})
	if err := Save(home, c); err != nil {
		t.Fatal(err)
	}
	if err := SaveHandoffs(home, &Handoffs{
		Version: HandoffsVersion,
		Active:  []Marker{{Session: "aaa", Slug: "-r", Cwd: "/r", From: "viejo", To: "otro", Since: "2026-07-01T00:00:00Z"}},
	}); err != nil {
		t.Fatal(err)
	}
	antes, err := os.ReadFile(filepath.Join(home, "ccp.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	// handoffs.yaml.tmp como directorio: el tmp+rename de los marcadores falla
	// después de que ccp.yaml ya se escribiera renombrado.
	if err := os.Mkdir(filepath.Join(home, "handoffs.yaml.tmp"), 0o755); err != nil {
		t.Fatal(err)
	}

	if _, err := ProfileRename(home, "viejo", "nuevo"); err == nil {
		t.Fatal("esperaba error al escribir handoffs.yaml")
	}
	despues, err := os.ReadFile(filepath.Join(home, "ccp.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(antes, despues) {
		t.Fatalf("ccp.yaml no volvió a como estaba.\nantes:\n%s\ndespués:\n%s", antes, despues)
	}
	if _, err := os.Stat(profileDirPath(home, "viejo")); err != nil {
		t.Fatalf("el directorio no volvió a su sitio: %v", err)
	}
	if _, err := os.Stat(profileDirPath(home, "nuevo")); !os.IsNotExist(err) {
		t.Fatal("quedó el directorio con el nombre nuevo")
	}
}

// TestProfileRenameRechazaUnNombreQueAutoHandoffYaMenciona: un nombre destino
// que el bloque ya nombra es un resto de otro perfil. Renombrar encima le daría
// al perfil renombrado cadenas y permisos que no eran suyos; el caso de
// allow_from es el grave, porque abriría un gate que hoy le deniega préstamos.
func TestProfileRenameRechazaUnNombreQueAutoHandoffYaMenciona(t *testing.T) {
	for _, tc := range []struct {
		name  string
		apply func(*AutoHandoff)
		where string
	}{
		{"en un fallback", func(a *AutoHandoff) {
			p := a.Policies["sin-el"]
			p.Fallback = append(p.Fallback, "nuevo")
			a.Policies["sin-el"] = p
		}, "policies.sin-el.fallback"},
		{"como clave de allow_from", func(a *AutoHandoff) { a.AllowFrom["nuevo"] = []string{"otro"} }, "allow_from.nuevo"},
		{"en una lista de allow_from", func(a *AutoHandoff) { a.AllowFrom["cliente"] = []string{"cliente", " nuevo"} }, "allow_from.cliente"},
		{"en hooks", func(a *AutoHandoff) { a.Hooks = append(a.Hooks, "nuevo") }, "hooks"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := seedRenameAutoHome(t)
			t.Setenv("CCP_CLAUDE_SRC", t.TempDir())
			c, err := Load(home)
			if err != nil {
				t.Fatal(err)
			}
			tc.apply(c.AutoHandoff)
			if err := Save(home, c); err != nil {
				t.Fatal(err)
			}
			antes, _ := os.ReadFile(filepath.Join(home, "ccp.yaml"))

			_, err = ProfileRename(home, "viejo", "nuevo")
			if err == nil {
				t.Fatal("esperaba que el rename se negara")
			}
			if !strings.Contains(err.Error(), "auto_handoff") || !strings.Contains(err.Error(), tc.where) {
				t.Errorf("el error no dice dónde está la mención (%s): %v", tc.where, err)
			}
			despues, _ := os.ReadFile(filepath.Join(home, "ccp.yaml"))
			if !bytes.Equal(antes, despues) {
				t.Error("un rename rechazado no debe tocar ccp.yaml")
			}
			if _, err := os.Stat(profileDirPath(home, "viejo")); err != nil {
				t.Error("un rename rechazado no debe mover el directorio")
			}
		})
	}
}

// TestProfileRenameConservaLosComentarios: goccy recoloca cada comentario por
// su ruta en el yaml, así que los que cuelgan de una clave renombrada
// (profiles.<viejo>, allow_from.<viejo>) se perdían en silencio. Los de allow_from
// son los que más duele perder: ahí se apunta por qué un cliente no presta a
// cierta cuenta. El nombre con punto cubre el caso en que goccy entrecomilla la
// clave dentro de la ruta.
func TestProfileRenameConservaLosComentarios(t *testing.T) {
	const yamlConComentarios = `version: 2
profiles:
  # la cuenta del trabajo
  viejo:
    type: official # login con SSO
  otro:
    type: official
rules:
  - path: /repo/uno
    profile: viejo # el repo de la empresa
authored: []
auto_handoff:
  enabled: true
  policies:
    default:
      fallback:
        - otro
        - viejo # préstamo de noche
  allow_from:
    # contrato: no presta a cuentas personales
    viejo:
      - viejo # solo a sí mismo
    otro: [otro, viejo]
  hooks:
    - viejo # sensores
`
	for _, nuevo := range []string{"nuevo", "work.v2"} {
		t.Run(nuevo, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("CCP_CLAUDE_SRC", t.TempDir())
			if err := os.WriteFile(filepath.Join(home, "ccp.yaml"), []byte(yamlConComentarios), 0o644); err != nil {
				t.Fatal(err)
			}
			if _, err := ProfileRename(home, "viejo", nuevo); err != nil {
				t.Fatalf("rename: %v", err)
			}
			b, err := os.ReadFile(filepath.Join(home, "ccp.yaml"))
			if err != nil {
				t.Fatal(err)
			}
			out := string(b)
			if strings.Contains(out, "viejo:") {
				t.Errorf("quedó una clave con el nombre viejo:\n%s", out)
			}
			// Cada comentario sigue ahí y pegado a lo que comentaba: los de
			// cabecera, justo encima de la clave renombrada.
			for _, want := range []string{
				"# la cuenta del trabajo\n  " + nuevo + ":\n",
				"type: official # login con SSO",
				"profile: " + nuevo + " # el repo de la empresa",
				"- " + nuevo + " # préstamo de noche",
				"# contrato: no presta a cuentas personales\n    " + nuevo + ":\n",
				"- " + nuevo + " # solo a sí mismo",
				"- " + nuevo + " # sensores",
			} {
				if !strings.Contains(out, want) {
					t.Errorf("falta %q en:\n%s", want, out)
				}
			}
		})
	}
}

// Un perfil official con sesión pierde el login al renombrarlo: Claude Code
// guarda la credencial con un nombre que sale de la ruta del cc-home (ADR 0016,
// M4). ccp no toca el Llavero, así que lo dice (B7).
func TestProfileRenameAvisaSiHabiaLogin(t *testing.T) {
	cases := []struct {
		nombre, tipo, claudeJSON string
		quiero                   bool
	}{
		{"official con cuenta", "official", `{"oauthAccount":{"emailAddress":"a@b"}}`, true},
		{"official con clave de consola", "official", `{"primaryApiKey":"sk-ant-x"}`, true},
		{"official sin login", "official", `{}`, false},
		{"proveedor", "deepseek", `{"oauthAccount":{"emailAddress":"a@b"}}`, false},
	}
	for _, tc := range cases {
		t.Run(tc.nombre, func(t *testing.T) {
			home := seedRenameHome(t)
			t.Setenv("CCP_CLAUDE_SRC", t.TempDir())
			if tc.tipo != "official" {
				c, err := Load(home)
				if err != nil {
					t.Fatal(err)
				}
				c.Profiles["viejo"] = Profile{Type: tc.tipo, BaseURL: "https://x", ModelPro: "m", ModelFlash: "m", Effort: "high"}
				if err := Save(home, c); err != nil {
					t.Fatal(err)
				}
			}
			cj := filepath.Join(ccHomePath(home, "viejo"), ".claude.json")
			if err := os.WriteFile(cj, []byte(tc.claudeJSON), 0o600); err != nil {
				t.Fatal(err)
			}
			res, err := ProfileRename(home, "viejo", "nuevo")
			if err != nil {
				t.Fatal(err)
			}
			if res.Relogin != tc.quiero {
				t.Errorf("Relogin = %v, quiero %v", res.Relogin, tc.quiero)
			}
		})
	}
}

// Si el directorio ya se movió y lo que falla es la regeneración, el rename es
// válido y el login se perdió igual: el aviso tiene que viajar junto al error,
// o el usuario arregla la regeneración y se queda con un perfil que no entra.
// Un rename que se deshace entero, en cambio, no cambió ninguna ruta.
func TestProfileRenameAvisaDelLoginAunqueFalleLaRegeneracion(t *testing.T) {
	home := seedRenameHome(t)
	t.Setenv("CCP_CLAUDE_SRC", t.TempDir())
	cj := filepath.Join(ccHomePath(home, "viejo"), ".claude.json")
	if err := os.WriteFile(cj, []byte(`{"oauthAccount":{"emailAddress":"a@b"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	// Un overlay ilegible hace fallar el merge de settings, que es lo último.
	if err := os.MkdirAll(cfgOverlayDir(home, "viejo"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfgSettingsFile(home, "viejo"), []byte(`{roto`), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := ProfileRename(home, "viejo", "nuevo")
	if err == nil {
		t.Fatal("quería el error de la regeneración")
	}
	if !res.Relogin {
		t.Errorf("Relogin = false junto al error de regeneración (%v); el login se perdió igual", err)
	}
	if _, serr := os.Stat(profileDirPath(home, "nuevo")); serr != nil {
		t.Fatalf("el directorio debería haberse movido: %v", serr)
	}
}

// Un rename que falla antes de mover no cambia ninguna ruta, así que no hay
// login que rehacer aunque el perfil tuviera sesión.
func TestProfileRenameRechazadoNoPideLogin(t *testing.T) {
	home := seedRenameHome(t)
	t.Setenv("CCP_CLAUDE_SRC", t.TempDir())
	cj := filepath.Join(ccHomePath(home, "viejo"), ".claude.json")
	if err := os.WriteFile(cj, []byte(`{"oauthAccount":{"emailAddress":"a@b"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	res, err := ProfileRename(home, "viejo", "otro") // ya existe
	if err == nil {
		t.Fatal("quería el error de nombre ocupado")
	}
	if res.Relogin {
		t.Error("Relogin = true en un rename que no movió nada")
	}
}
