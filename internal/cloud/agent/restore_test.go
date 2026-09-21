package agent

// Restaurar desde la nube (spec §10.3.1, caminos 1 y 3): baja el snapshot,
// mapea los proyectos a ESTA máquina (§11) y termina en el motor de §8.3.

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/JoseAFlores777/ccp/internal/core"
)

func TestRestoreDesdeLaNubeVuelveAlEstadoDelSnapshot(t *testing.T) {
	ctx := context.Background()
	m := nueva(t)
	if err := core.ProfileAddOfficial(m.o.Home, "work"); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(m.o.Src, "CLAUDE.md"), "uno")
	snap := m.captura()
	write(t, filepath.Join(m.o.Src, "CLAUDE.md"), "dos")

	plan, err := Restore(ctx, m.o, snap, RestoreOpts{DryRun: true})
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if read(t, filepath.Join(m.o.Src, "CLAUDE.md")) != "dos" {
		t.Fatal("el plan escribió")
	}
	if plan.Plan.PreSnapshot != "" {
		t.Error("un plan no toma snapshot previo")
	}
	if plan.Cloud != snap || plan.Snapshot == "" {
		t.Errorf("el informe tiene que nombrar los dos ids: %+v", plan)
	}
	// El perfil official del snapshot no tiene sesión aquí: los tokens no
	// viajan, así que eso es un pendiente y se dice ANTES de aplicar.
	var logins []string
	for _, p := range plan.Pending {
		if p.Kind == core.RestorePendingLogin {
			logins = append(logins, p.Name)
		}
	}
	if len(logins) != 1 || logins[0] != "work" {
		t.Errorf("logins pendientes = %v; quiero [work]", logins)
	}

	rep, err := Restore(ctx, m.o, snap, RestoreOpts{})
	if err != nil {
		t.Fatalf("restore: %v", err)
	}
	if got := read(t, filepath.Join(m.o.Src, "CLAUDE.md")); got != "uno" {
		t.Errorf("CLAUDE.md = %q; quiero el del snapshot", got)
	}
	if rep.Plan.PreSnapshot == "" {
		t.Error("aplicar toma antes un snapshot de seguridad")
	}
}

func TestRestoreSoloLoMarcadoNoTocaElResto(t *testing.T) {
	ctx := context.Background()
	m := nueva(t)
	write(t, filepath.Join(m.o.Src, "CLAUDE.md"), "uno")
	write(t, filepath.Join(m.o.Src, "settings.json"), `{"theme":"dark"}`)
	snap := m.captura()
	write(t, filepath.Join(m.o.Src, "CLAUDE.md"), "dos")
	write(t, filepath.Join(m.o.Src, "settings.json"), `{"theme":"light"}`)

	if _, err := Restore(ctx, m.o, snap, RestoreOpts{Only: []string{"claude/CLAUDE.md"}}); err != nil {
		t.Fatalf("restore: %v", err)
	}
	if got := read(t, filepath.Join(m.o.Src, "CLAUDE.md")); got != "uno" {
		t.Errorf("CLAUDE.md = %q", got)
	}
	if got := read(t, filepath.Join(m.o.Src, "settings.json")); got != `{"theme":"light"}` {
		t.Errorf("settings.json = %q; lo que no se marcó no se toca", got)
	}
}

func TestRestoreMapeaElProyectoAlCarpetaDeAqui(t *testing.T) {
	ctx := context.Background()
	m := nueva(t)
	repo := t.TempDir()
	write(t, filepath.Join(repo, ".git", "config"), "[remote \"origin\"]\n\turl = git@github.com:Org/App.git\n")
	write(t, filepath.Join(repo, "CLAUDE.local.md"), "del proyecto")
	if _, err := core.RuleSet(m.o.Home, repo, "default"); err != nil {
		t.Fatal(err)
	}
	snap := m.captura()
	// El repo se mudó: aquí está clonado en otro sitio.
	clon := t.TempDir()
	write(t, filepath.Join(clon, ".git", "config"), "[remote \"origin\"]\n\turl = https://github.com/org/app\n")
	if err := os.RemoveAll(repo); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", filepath.Dir(clon))

	rep, err := Restore(ctx, m.o, snap, RestoreOpts{Projects: map[string]string{
		core.ProjectKey(repo, "git@github.com:Org/App.git"): clon,
	}})
	if err != nil {
		t.Fatalf("restore: %v", err)
	}
	if got, err := os.ReadFile(filepath.Join(clon, "CLAUDE.local.md")); err != nil || string(got) != "del proyecto" {
		t.Fatalf("el archivo del proyecto no llegó al clon: %v %q", err, got)
	}
	if len(rep.Projects) != 1 || rep.Projects[0].Source != core.ProjectMapManual {
		t.Errorf("el informe tiene que contar el mapeo: %+v", rep.Projects)
	}
}

// El camino 2 (el portal manda una revisión) también mapea proyectos: una
// máquina nueva tiene los repos en otro sitio, y sin esto la orden se aplica a
// medias y nadie dice por qué.
func TestRevisionEncuentraElProyectoPorSuRemoto(t *testing.T) {
	ctx := context.Background()
	m := nueva(t)
	casa := t.TempDir()
	t.Setenv("HOME", casa)
	repo := filepath.Join(casa, "code", "app")
	write(t, filepath.Join(repo, ".git", "config"), "[remote \"origin\"]\n\turl = git@github.com:Org/App.git\n")
	write(t, filepath.Join(repo, "CLAUDE.local.md"), "del proyecto")
	if _, err := core.RuleSet(m.o.Home, repo, "default"); err != nil {
		t.Fatal(err)
	}
	snap := m.captura()

	// El repo se movió dentro de esta misma máquina: la regla sigue apuntando
	// a donde estaba, pero el clon está en ~/src/app con el mismo remoto.
	clon := filepath.Join(casa, "src", "app")
	write(t, filepath.Join(clon, ".git", "config"), "[remote \"origin\"]\n\turl = https://github.com/org/app\n")
	if err := os.RemoveAll(repo); err != nil {
		t.Fatal(err)
	}

	m.publica(revID(7), snap, "")
	out, err := Once(ctx, m.o)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(filepath.Join(clon, "CLAUDE.local.md")); err != nil || string(got) != "del proyecto" {
		t.Fatalf("la revisión tenía que escribir en el clon: %v %q (%+v)", err, got, out)
	}
}
