package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// profile_rename_test.go — el nombre de un perfil vive en cuatro sitios
// (ccp.yaml profiles, ccp.yaml rules, <home>/profiles/<name>/ y los marcadores
// de handoffs.yaml). Renombrar tocando solo uno deja el estado incoherente:
// una regla apuntando a un perfil inexistente resuelve a `default` en silencio,
// y un marcador con el nombre viejo ya no se puede terminar.

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

	if err := ProfileRename(home, "viejo", "nuevo"); err != nil {
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

	if err := ProfileRename(home, "viejo", "nuevo"); err != nil {
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
		err := ProfileRename(home, tc.old, tc.new)
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
	if err := ProfileRename(home, "viejo", "nuevo"); err == nil {
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
