package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeClaudeApp monta un Claude.app de mentira con lo que BuildDesktopApp
// necesita: Info.plist, el stub, el icono, un app.asar y un framework con
// symlinks (que es lo que un espejo tiene que saber reproducir).
func fakeClaudeApp(t *testing.T, version string) string {
	t.Helper()
	app := filepath.Join(t.TempDir(), "Claude.app")
	fw := filepath.Join(app, "Contents", "Frameworks", "Electron Framework.framework")
	for _, d := range []string{
		filepath.Join(app, "Contents", "MacOS"),
		filepath.Join(app, "Contents", "Resources"),
		filepath.Join(fw, "Versions", "A"),
	} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.WriteFile(filepath.Join(app, "Contents", "MacOS", "Claude"), []byte("stub"), 0o755))
	must(os.WriteFile(filepath.Join(app, "Contents", "Resources", "electron.icns"), icnsDePrueba(t), 0o644))
	must(os.WriteFile(filepath.Join(app, "Contents", "Resources", "app.asar"), []byte("asar"), 0o644))
	must(os.WriteFile(filepath.Join(fw, "Versions", "A", "Electron Framework"), []byte("fw"), 0o755))
	must(os.Symlink("A", filepath.Join(fw, "Versions", "Current")))
	must(os.Symlink("Versions/Current/Electron Framework", filepath.Join(fw, "Electron Framework")))

	p, err := parsePlist([]byte(testPlistXML))
	must(err)
	p.Set("CFBundleName", "Claude")
	p.Set("CFBundleVersion", version)
	p.Set("CFBundleIconFile", "electron.icns")
	p.Set("CFBundleIconName", "Claude")
	p.Set("CFBundleURLTypes", []any{map[string]any{"CFBundleURLSchemes": []any{"claude"}}})
	must(os.WriteFile(filepath.Join(app, "Contents", "Info.plist"), marshalPlist(p), 0o644))
	return app
}

func desktopAppOpts(t *testing.T, profile string) (DesktopAppOptions, string) {
	t.Helper()
	root := t.TempDir()
	ccpBin := filepath.Join(root, "bin", "ccp")
	if err := os.MkdirAll(filepath.Dir(ccpBin), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ccpBin, []byte("ccp"), 0o755); err != nil {
		t.Fatal(err)
	}
	appsDir := filepath.Join(root, "Applications")
	return DesktopAppOptions{
		Home:      filepath.Join(root, "ccp-home"),
		Profile:   profile,
		Cfg:       cfgConPerfiles(t, map[string]Profile{"work": {Type: "official"}, "personal": {Type: "official"}, "ds": {Type: "deepseek"}}),
		AppsDir:   appsDir,
		SourceApp: fakeClaudeApp(t, "1.0.0"),
		CCPBin:    ccpBin,
		Generator: "test",
		Now:       func() time.Time { return time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC) },
	}, appsDir
}

func readlinkT(t *testing.T, p string) string {
	t.Helper()
	l, err := os.Readlink(p)
	if err != nil {
		t.Fatalf("readlink %s: %v", p, err)
	}
	return l
}

// TestBuildDesktopAppMontaLasDosCapas es el contrato del bundle: capa externa
// con identidad propia y capa interna idéntica al original.
func TestBuildDesktopAppMontaLasDosCapas(t *testing.T) {
	o, appsDir := desktopAppOpts(t, "work")
	res, err := BuildDesktopApp(o)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Changed || res.Reason != "created" {
		t.Errorf("primer build: changed=%v reason=%q", res.Changed, res.Reason)
	}
	app := res.App.Path
	if want := filepath.Join(appsDir, "Claude (work).app"); app != want {
		t.Fatalf("ruta = %s, quería %s", app, want)
	}

	// Capa externa: Info.plist parcheado.
	p, err := readPlist(filepath.Join(app, "Contents", "Info.plist"))
	if err != nil {
		t.Fatal(err)
	}
	if got := p.String("CFBundleIdentifier"); got != DesktopBundleID("work") {
		t.Errorf("CFBundleIdentifier = %q", got)
	}
	if p.String("CFBundleName") != "Claude (work)" || p.String("CFBundleDisplayName") != "Claude (work)" {
		t.Errorf("nombres: %q / %q", p.String("CFBundleName"), p.String("CFBundleDisplayName"))
	}
	if p.String("CFBundleExecutable") != "Claude" || p.String("CFBundleIconFile") != "ccp-icon.icns" {
		t.Errorf("exe/icono: %q / %q", p.String("CFBundleExecutable"), p.String("CFBundleIconFile"))
	}
	if _, ok := p.Get("CFBundleIconName"); ok {
		t.Error("CFBundleIconName debería quitarse (en macOS 26 ganaría al .icns)")
	}
	if _, ok := p.Get("CFBundleURLTypes"); ok {
		t.Error("CFBundleURLTypes debería quitarse: claude:// es de la app normal")
	}
	if p.Dict("LSEnvironment") == nil || p.Dict("LSEnvironment").String("MallocNanoZone") != "0" {
		t.Error("LSEnvironment del original debería conservarse")
	}
	integ := p.Dict("ElectronAsarIntegrity")
	if integ == nil || integ.Dict("Resources/app.asar") == nil {
		t.Fatal("ElectronAsarIntegrity original perdido")
	}
	nestedKey := "ccp/Claude/Contents/Resources/app.asar"
	if integ.Dict(nestedKey) == nil || integ.Dict(nestedKey).String("hash") != "abc123" {
		t.Errorf("falta la entrada de integridad del espejo anidado: %v", integ.Keys())
	}

	// Ejecutables: el lanzador es ccp (hard link, nunca symlink: LaunchServices
	// no lanza ejecutables symlinkeados); Claude-run apunta al stub del espejo.
	launcher, err := os.Lstat(filepath.Join(app, "Contents", "MacOS", "Claude"))
	if err != nil {
		t.Fatal(err)
	}
	if launcher.Mode()&os.ModeSymlink != 0 {
		t.Error("MacOS/Claude no puede ser un symlink")
	}
	if ccpInfo, _ := os.Stat(o.CCPBin); !os.SameFile(ccpInfo, launcher) {
		t.Error("MacOS/Claude debería ser un hard link del binario de ccp")
	}
	if launcher.Mode()&0o111 == 0 {
		t.Error("el lanzador tiene que ser ejecutable")
	}
	if got := readlinkT(t, filepath.Join(app, "Contents", "MacOS", "Claude-run")); got != "../ccp/Claude/Contents/MacOS/Claude" {
		t.Errorf("MacOS/Claude-run -> %s", got)
	}

	// Capa interna: idéntica y por hard link.
	nested := filepath.Join(app, "Contents", "ccp", "Claude")
	srcStub, _ := os.Stat(filepath.Join(o.SourceApp, "Contents", "MacOS", "Claude"))
	dstStub, err := os.Stat(filepath.Join(nested, "Contents", "MacOS", "Claude"))
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(srcStub, dstStub) {
		t.Error("el stub del espejo debería ser un hard link del original")
	}
	srcPlist, _ := os.ReadFile(filepath.Join(o.SourceApp, "Contents", "Info.plist"))
	dstPlist, _ := os.ReadFile(filepath.Join(nested, "Contents", "Info.plist"))
	if string(srcPlist) != string(dstPlist) {
		t.Error("el Info.plist del espejo debe ser byte a byte el original")
	}
	fw := filepath.Join(nested, "Contents", "Frameworks", "Electron Framework.framework")
	if got := readlinkT(t, filepath.Join(fw, "Versions", "Current")); got != "A" {
		t.Errorf("symlink del framework no reproducido: %q", got)
	}
	if _, err := os.Stat(filepath.Join(fw, "Electron Framework")); err != nil {
		t.Errorf("el symlink relativo del framework debería resolver dentro del espejo: %v", err)
	}

	// Icono tintado, manifiesto y PkgInfo.
	icon, err := os.ReadFile(filepath.Join(app, "Contents", "Resources", "ccp-icon.icns"))
	if err != nil {
		t.Fatal(err)
	}
	chunks, err := icnsSplit(icon)
	if err != nil || len(chunks) != 1 {
		t.Errorf("icono tintado: %v, %d trozos", err, len(chunks))
	}
	m := res.App.Manifest
	if m.Profile != "work" || m.Label != "Claude (work)" || m.Color != "blue" || m.SourceVersion != "1.0.0" ||
		m.BundleID != DesktopBundleID("work") || m.CCPBin != o.CCPBin || m.Home != o.Home {
		t.Errorf("manifiesto: %+v", m)
	}
	if b, _ := os.ReadFile(filepath.Join(app, "Contents", "PkgInfo")); string(b) != "APPL????" {
		t.Errorf("PkgInfo = %q", b)
	}
}

func TestBuildDesktopAppEsIdempotente(t *testing.T) {
	o, _ := desktopAppOpts(t, "work")
	if _, err := BuildDesktopApp(o); err != nil {
		t.Fatal(err)
	}
	res, err := BuildDesktopApp(o)
	if err != nil {
		t.Fatal(err)
	}
	if res.Changed || res.Reason != "unchanged" {
		t.Errorf("segundo build debería ser no-op: changed=%v reason=%q", res.Changed, res.Reason)
	}
	o.Force = true
	res, err = BuildDesktopApp(o)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Changed || res.Reason != "refreshed" {
		t.Errorf("--force debería reconstruir: changed=%v reason=%q", res.Changed, res.Reason)
	}
}

// TestDesktopAppStaleYRefrescoPorVersion: cuando Claude.app se actualiza, el
// lanzador se declara obsoleto y el siguiente build lo reconstruye.
func TestDesktopAppStaleYRefrescoPorVersion(t *testing.T) {
	o, _ := desktopAppOpts(t, "work")
	res, err := BuildDesktopApp(o)
	if err != nil {
		t.Fatal(err)
	}
	if reason, stale := DesktopAppStale(res.App, o.SourceApp); stale {
		t.Fatalf("recién construido no debería estar obsoleto: %s", reason)
	}

	// Simula la actualización: nuevo CFBundleVersion en el original.
	plistPath := filepath.Join(o.SourceApp, "Contents", "Info.plist")
	p, _ := readPlist(plistPath)
	p.Set("CFBundleVersion", "2.0.0")
	if err := os.WriteFile(plistPath, marshalPlist(p), 0o644); err != nil {
		t.Fatal(err)
	}
	reason, stale := DesktopAppStale(res.App, o.SourceApp)
	if !stale || !strings.Contains(reason, "2.0.0") {
		t.Fatalf("debería estar obsoleto por versión: %v %q", stale, reason)
	}
	res, err = BuildDesktopApp(o)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Changed || res.Reason != "refreshed" || res.App.Manifest.SourceVersion != "2.0.0" {
		t.Errorf("refresco: %+v %+v", res.Reason, res.App.Manifest)
	}

	// Y si ccp se actualiza (otro archivo en la misma ruta), también: el
	// lanzador tiene el inodo viejo y hay que volver a enlazar el nuevo.
	if err := os.WriteFile(o.CCPBin, []byte("ccp v2"), 0o755); err != nil {
		t.Fatal(err)
	}
	future := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(o.CCPBin, future, future); err != nil {
		t.Fatal(err)
	}
	if reason, stale := DesktopAppStale(res.App, o.SourceApp); !stale {
		t.Errorf("con ccp actualizado debería ser obsoleto: %q", reason)
	}
	res, err = BuildDesktopApp(o)
	if err != nil || !res.Changed {
		t.Fatalf("debería reconstruir con el ccp nuevo: %v %+v", err, res)
	}
	if b, _ := os.ReadFile(filepath.Join(res.App.Path, "Contents", "MacOS", "Claude")); string(b) != "ccp v2" {
		t.Error("el lanzador debería enlazar el binario nuevo")
	}
	if err := os.Remove(o.CCPBin); err != nil {
		t.Fatal(err)
	}
	if _, stale := DesktopAppStale(res.App, o.SourceApp); !stale {
		t.Error("sin el binario de ccp registrado debería ser obsoleto")
	}
}

func TestBuildDesktopAppCambioDeLabelRenombra(t *testing.T) {
	o, appsDir := desktopAppOpts(t, "work")
	first, err := BuildDesktopApp(o)
	if err != nil {
		t.Fatal(err)
	}
	o.Label = "Trabajo.app" // el sufijo se tolera y se quita
	res, err := BuildDesktopApp(o)
	if err != nil {
		t.Fatal(err)
	}
	if res.App.Path != filepath.Join(appsDir, "Trabajo.app") || res.App.Manifest.Label != "Trabajo" {
		t.Errorf("ruta/label tras renombrar: %s %q", res.App.Path, res.App.Manifest.Label)
	}
	if _, err := os.Stat(first.App.Path); err == nil {
		t.Error("el lanzador con el nombre anterior debería haberse retirado")
	}
	found, _ := FindDesktopApp(appsDir, "work")
	if found == nil || found.Path != res.App.Path {
		t.Errorf("FindDesktopApp debería dar el nuevo: %+v", found)
	}
	// Un segundo build sin --label conserva el nombre elegido.
	o.Label = ""
	res, err = BuildDesktopApp(o)
	if err != nil || res.Changed {
		t.Errorf("sin --label debería conservar 'Trabajo' sin tocar nada: %v %+v", err, res)
	}
	o.Label = "con/barra"
	if _, err := BuildDesktopApp(o); err == nil {
		t.Error("un label con '/' debería rechazarse")
	}
}

func TestBuildDesktopAppColorAutomaticoEvitaLosUsados(t *testing.T) {
	o, _ := desktopAppOpts(t, "work")
	a, err := BuildDesktopApp(o)
	if err != nil {
		t.Fatal(err)
	}
	o2 := o
	o2.Profile = "personal"
	b, err := BuildDesktopApp(o2)
	if err != nil {
		t.Fatal(err)
	}
	if a.App.Manifest.Color != "blue" || b.App.Manifest.Color != "green" {
		t.Errorf("colores automáticos: %s / %s", a.App.Manifest.Color, b.App.Manifest.Color)
	}
	o.Color = "pink"
	a, err = BuildDesktopApp(o)
	if err != nil {
		t.Fatal(err)
	}
	if !a.Changed || a.App.Manifest.Color != "pink" {
		t.Errorf("--color debería reconstruir con el nuevo color: %+v", a.App.Manifest)
	}
	o.Color = "fucsia"
	if _, err := BuildDesktopApp(o); err == nil {
		t.Error("un color desconocido debería rechazarse")
	}
}

func TestBuildDesktopAppNoPisaUnaAppAjena(t *testing.T) {
	o, appsDir := desktopAppOpts(t, "work")
	ajena := filepath.Join(appsDir, "Claude (work).app", "Contents")
	if err := os.MkdirAll(ajena, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := BuildDesktopApp(o); err == nil {
		t.Fatal("no debería sustituir un .app sin manifiesto de ccp")
	}
	if _, err := os.Stat(ajena); err != nil {
		t.Error("la app ajena debería seguir intacta")
	}
}

func TestBuildDesktopAppRechazaDefaultYProveedores(t *testing.T) {
	o, _ := desktopAppOpts(t, "default")
	if _, err := BuildDesktopApp(o); err == nil {
		t.Error("default no tiene lanzador")
	}
	o.Profile = "ds"
	if _, err := BuildDesktopApp(o); err == nil {
		t.Error("un perfil de proveedor no es elegible")
	}
	o.Profile = "work"
	o.CCPBin = "ccp"
	if _, err := BuildDesktopApp(o); err == nil {
		t.Error("el binario tiene que ser una ruta absoluta")
	}
}

func TestRemoveDesktopAppSoloLoNuestro(t *testing.T) {
	o, appsDir := desktopAppOpts(t, "work")
	res, err := BuildDesktopApp(o)
	if err != nil {
		t.Fatal(err)
	}
	fuera := &DesktopApp{Path: filepath.Join(t.TempDir(), "X.app"), Manifest: res.App.Manifest}
	if err := RemoveDesktopApp(appsDir, fuera); err == nil {
		t.Error("fuera de appsDir no se borra")
	}
	sinManifiesto := filepath.Join(appsDir, "Otra.app")
	if err := os.MkdirAll(sinManifiesto, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := RemoveDesktopApp(appsDir, &DesktopApp{Path: sinManifiesto}); err == nil {
		t.Error("sin manifiesto no se borra")
	}
	if err := RemoveDesktopApp(appsDir, res.App); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(res.App.Path); err == nil {
		t.Error("el lanzador debería haberse borrado")
	}
	if found, _ := FindDesktopApp(appsDir, "work"); found != nil {
		t.Error("tras borrar no debería encontrarse")
	}
}

// TestIsDesktopLauncherYPlan cubre el modo lanzador: reconocer desde dónde se
// ejecuta ccp y calcular el exec con los dos aislamientos.
func TestIsDesktopLauncherYPlan(t *testing.T) {
	o, _ := desktopAppOpts(t, "work")
	res, err := BuildDesktopApp(o)
	if err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(res.App.Path, "Contents", "MacOS", "Claude")
	app, ok := IsDesktopLauncher(exe)
	if !ok || app != res.App.Path {
		t.Fatalf("IsDesktopLauncher(%s) = %q, %v", exe, app, ok)
	}
	if _, ok := IsDesktopLauncher(o.CCPBin); ok {
		t.Error("el binario suelto no es un lanzador")
	}
	if _, ok := IsDesktopLauncher(filepath.Join(t.TempDir(), "Foo.app", "Contents", "MacOS", "Claude")); ok {
		t.Error("sin manifiesto no es un lanzador")
	}

	environ := []string{"PATH=/usr/bin", "ANTHROPIC_BASE_URL=https://viejo", "CLAUDE_CONFIG_DIR=/otro"}
	plan, err := PlanDesktopLauncher(res.App, o.Home, o.Cfg, []string{"--extra"}, environ)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Bin != filepath.Join(res.App.Path, "Contents", "MacOS", "Claude-run") {
		t.Errorf("Bin = %s", plan.Bin)
	}
	wantData := filepath.Join(o.Home, "profiles", "work", "desktop")
	if len(plan.Args) != 3 || plan.Args[0] != plan.Bin || plan.Args[1] != "--user-data-dir="+wantData || plan.Args[2] != "--extra" {
		t.Errorf("Args = %v", plan.Args)
	}
	env := strings.Join(plan.Env, "\n")
	if !strings.Contains(env, "CLAUDE_CONFIG_DIR="+filepath.Join(o.Home, "profiles", "work", "cc-home")+"\n") &&
		!strings.HasSuffix(env, "CLAUDE_CONFIG_DIR="+filepath.Join(o.Home, "profiles", "work", "cc-home")) {
		t.Errorf("falta CLAUDE_CONFIG_DIR del perfil:\n%s", env)
	}
	if strings.Contains(env, "ANTHROPIC_BASE_URL=") || strings.Contains(env, "CLAUDE_CONFIG_DIR=/otro") {
		t.Errorf("las vars gestionadas heredadas deben limpiarse:\n%s", env)
	}
	for _, kv := range plan.Env {
		if strings.HasSuffix(kv, "=") {
			t.Errorf("no se emite ninguna var gestionada vacía: %q", kv)
		}
	}
	if !strings.Contains(env, "PATH=/usr/bin") {
		t.Error("el resto del entorno se hereda")
	}

	bin, args := DesktopAppOpenCommand(res.App, []string{"--x"})
	if bin != "open" || strings.Join(args, " ") != "-a "+res.App.Path+" --args --x" {
		t.Errorf("open: %s %v", bin, args)
	}
}
