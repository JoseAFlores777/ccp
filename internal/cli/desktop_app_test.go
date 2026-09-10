package cli

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JoseAFlores777/ccp/internal/core"
)

// fakeClaudeAppCLI monta un Claude.app mínimo (plist, stub, icono) para que
// `desktop app` tenga de dónde espejar sin depender de la máquina.
func fakeClaudeAppCLI(t *testing.T) string {
	t.Helper()
	app := filepath.Join(t.TempDir(), "Claude.app")
	for _, d := range []string{"Contents/MacOS", "Contents/Resources"} {
		if err := os.MkdirAll(filepath.Join(app, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	plist := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>CFBundleExecutable</key>
	<string>Claude</string>
	<key>CFBundleIconFile</key>
	<string>electron.icns</string>
	<key>CFBundleIdentifier</key>
	<string>com.anthropic.claudefordesktop</string>
	<key>CFBundleName</key>
	<string>Claude</string>
	<key>CFBundleVersion</key>
	<string>1.2.3</string>
	<key>CFBundleURLTypes</key>
	<array>
		<dict>
			<key>CFBundleURLSchemes</key>
			<array>
				<string>claude</string>
			</array>
		</dict>
	</array>
</dict>
</plist>
`
	if err := os.WriteFile(filepath.Join(app, "Contents", "Info.plist"), []byte(plist), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(app, "Contents", "MacOS", "Claude"), []byte("stub"), 0o755); err != nil {
		t.Fatal(err)
	}
	img := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	for i := range img.Pix {
		img.Pix[i] = 0xff
	}
	img.SetNRGBA(0, 0, color.NRGBA{R: 0xd9, G: 0x77, B: 0x57, A: 0xff})
	var pngBuf bytes.Buffer
	if err := png.Encode(&pngBuf, img); err != nil {
		t.Fatal(err)
	}
	var icns bytes.Buffer
	icns.WriteString("icns")
	_ = binary.Write(&icns, binary.BigEndian, uint32(8+8+pngBuf.Len()))
	icns.WriteString("ic07")
	_ = binary.Write(&icns, binary.BigEndian, uint32(8+pngBuf.Len()))
	icns.Write(pngBuf.Bytes())
	if err := os.WriteFile(filepath.Join(app, "Contents", "Resources", "electron.icns"), icns.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	return app
}

// homeConLanzadores prepara un CCP_HOME con un perfil official, un Claude.app
// falso y un directorio de lanzadores temporal (nunca ~/Applications).
func homeConLanzadores(t *testing.T) (home, appsDir string) {
	t.Helper()
	home = homeConPerfil(t, "work", "official")
	t.Setenv("CCP_DESKTOP_APP", fakeClaudeAppCLI(t))
	appsDir = filepath.Join(t.TempDir(), "Applications")
	t.Setenv("CCP_DESKTOP_APPS_DIR", appsDir)
	return home, appsDir
}

// TestDesktopAppCreaYListaYBorra es el ciclo de vida entero del lanzador
// desde la CLI.
func TestDesktopAppCreaYListaYBorra(t *testing.T) {
	_, appsDir := homeConLanzadores(t)

	var out, errb bytes.Buffer
	if code := Dispatch([]string{"desktop", "app", "work", "--color", "purple"}, &out, &errb); code != 0 {
		t.Fatalf("exit = %d: %s", code, errb.String())
	}
	want := filepath.Join(appsDir, "Claude (work).app")
	if !strings.Contains(out.String(), want) || !strings.Contains(out.String(), "purple") {
		t.Errorf("debería anunciar ruta y color: %s", out.String())
	}
	if _, err := os.Lstat(filepath.Join(want, "Contents", "MacOS", "Claude")); err != nil {
		t.Fatalf("falta el ejecutable del lanzador: %v", err)
	}

	// Segunda vez: sin cambios.
	out.Reset()
	if code := Dispatch([]string{"desktop", "app", "work"}, &out, &errb); code != 0 {
		t.Fatalf("exit = %d: %s", code, errb.String())
	}
	if !strings.Contains(out.String(), "purple") {
		t.Errorf("sin --color debería conservar el color: %s", out.String())
	}

	// list --json enseña el lanzador.
	out.Reset()
	if code := Dispatch([]string{"desktop", "list", "--json"}, &out, &errb); code != 0 {
		t.Fatalf("exit = %d: %s", code, errb.String())
	}
	var rows []map[string]any
	if err := json.Unmarshal(out.Bytes(), &rows); err != nil {
		t.Fatalf("JSON inválido: %v\n%s", err, out.String())
	}
	var found bool
	for _, r := range rows {
		if r["profile"] == "work" {
			found = true
			if r["app"] != want || r["color"] != "purple" {
				t.Errorf("fila de work: %v", r)
			}
		}
		if r["profile"] == "default" && (r["app"] != "" || r["color"] != "") {
			t.Errorf("default no tiene lanzador: %v", r)
		}
	}
	if !found {
		t.Fatal("work no aparece en list --json")
	}

	// rm del lanzador: la instancia (si la hubiera) se queda.
	out.Reset()
	if code := Dispatch([]string{"desktop", "app", "rm", "work"}, &out, &errb); code != 0 {
		t.Fatalf("exit = %d: %s", code, errb.String())
	}
	if _, err := os.Stat(want); err == nil {
		t.Error("el lanzador debería haberse borrado")
	}
	out.Reset()
	errb.Reset()
	if code := Dispatch([]string{"desktop", "app", "rm", "work"}, &out, &errb); code == 0 {
		t.Error("borrar un lanzador inexistente debería fallar")
	}
}

// TestDesktopAppSinPerfilCubreTodosLosOfficial: `ccp desktop app` a secas crea
// uno por perfil elegible, nunca para default ni para proveedores.
func TestDesktopAppSinPerfilCubreTodosLosOfficial(t *testing.T) {
	home, appsDir := homeConLanzadores(t)
	if err := os.MkdirAll(filepath.Join(home, "profiles", "ds"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := loadCfgT(t, home)
	cfg.Profiles["ds"] = core.Profile{Type: "deepseek"}
	if err := core.Save(home, cfg); err != nil {
		t.Fatal(err)
	}

	var out, errb bytes.Buffer
	if code := Dispatch([]string{"desktop", "app"}, &out, &errb); code != 0 {
		t.Fatalf("exit = %d: %s", code, errb.String())
	}
	entries, _ := os.ReadDir(appsDir)
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	if strings.Join(names, ",") != "Claude (work).app" {
		t.Errorf("lanzadores creados: %v", names)
	}
}

// TestDesktopOpenUsaElLanzador: con lanzador, `open` pasa por LaunchServices
// con el bundle del perfil y sin --env/--args (los pone el lanzador); con
// --plain vuelve al lanzamiento directo de siempre.
func TestDesktopOpenUsaElLanzador(t *testing.T) {
	_, appsDir := homeConLanzadores(t)
	var out, errb bytes.Buffer
	if code := Dispatch([]string{"desktop", "app", "work"}, &out, &errb); code != 0 {
		t.Fatalf("exit = %d: %s", code, errb.String())
	}

	out.Reset()
	if code := Dispatch([]string{"desktop", "open", "work", "--dry-run"}, &out, &errb); code != 0 {
		t.Fatalf("exit = %d: %s", code, errb.String())
	}
	want := "open -a " + filepath.Join(appsDir, "Claude (work).app")
	if !strings.Contains(out.String(), want) {
		t.Errorf("debería lanzar por el lanzador:\n%s", out.String())
	}
	if strings.Contains(out.String(), "--user-data-dir=") {
		t.Errorf("el user-data-dir lo pone el lanzador, no open:\n%s", out.String())
	}

	out.Reset()
	if code := Dispatch([]string{"desktop", "open", "work", "--dry-run", "--plain"}, &out, &errb); code != 0 {
		t.Fatalf("exit = %d: %s", code, errb.String())
	}
	if strings.Contains(out.String(), "Claude (work).app") || !strings.Contains(out.String(), "--user-data-dir=") {
		t.Errorf("--plain debería ser el lanzamiento directo:\n%s", out.String())
	}
}

// TestDesktopOpenRefrescaElLanzadorDesfasado: si Claude.app cambió de
// versión, `open` reconstruye el lanzador antes de lanzar (o lo anuncia en
// --dry-run).
func TestDesktopOpenRefrescaElLanzadorDesfasado(t *testing.T) {
	_, appsDir := homeConLanzadores(t)
	var out, errb bytes.Buffer
	if code := Dispatch([]string{"desktop", "app", "work"}, &out, &errb); code != 0 {
		t.Fatalf("exit = %d: %s", code, errb.String())
	}
	src := os.Getenv("CCP_DESKTOP_APP")
	plist := filepath.Join(src, "Contents", "Info.plist")
	b, _ := os.ReadFile(plist)
	if err := os.WriteFile(plist, bytes.Replace(b, []byte("1.2.3"), []byte("9.9.9"), 1), 0o644); err != nil {
		t.Fatal(err)
	}

	out.Reset()
	if code := Dispatch([]string{"desktop", "open", "work", "--dry-run"}, &out, &errb); code != 0 {
		t.Fatalf("exit = %d: %s", code, errb.String())
	}
	if !strings.Contains(out.String(), "9.9.9") {
		t.Errorf("el dry-run debería anunciar el refresco por versión:\n%s", out.String())
	}
	manifest, _ := os.ReadFile(filepath.Join(appsDir, "Claude (work).app", "Contents", "ccp-desktop.json"))
	if !strings.Contains(string(manifest), `"1.2.3"`) {
		t.Error("--dry-run no debería haber reconstruido nada")
	}
}

// TestDesktopRmSeLlevaElLanzador: borrar la instancia deja el lanzador sin
// nada que abrir, así que se va con ella.
func TestDesktopRmSeLlevaElLanzador(t *testing.T) {
	home, appsDir := homeConLanzadores(t)
	var out, errb bytes.Buffer
	if code := Dispatch([]string{"desktop", "app", "work"}, &out, &errb); code != 0 {
		t.Fatalf("exit = %d: %s", code, errb.String())
	}
	if err := os.MkdirAll(filepath.Join(home, "profiles", "work", "desktop"), 0o700); err != nil {
		t.Fatal(err)
	}
	if code := Dispatch([]string{"desktop", "rm", "work", "--yes"}, &out, &errb); code != 0 {
		t.Fatalf("exit = %d: %s", code, errb.String())
	}
	if _, err := os.Stat(filepath.Join(appsDir, "Claude (work).app")); err == nil {
		t.Error("el lanzador debería irse con la instancia")
	}
}

func TestDesktopAppRechazaDefaultYColorInvalido(t *testing.T) {
	homeConLanzadores(t)
	var out, errb bytes.Buffer
	if code := Dispatch([]string{"desktop", "app", "default"}, &out, &errb); code == 0 {
		t.Error("default no tiene lanzador")
	}
	errb.Reset()
	if code := Dispatch([]string{"desktop", "app", "work", "--color", "fucsia"}, &out, &errb); code == 0 {
		t.Error("un color desconocido debería rechazarse")
	}
	if !strings.Contains(errb.String(), "blue") {
		t.Errorf("el error debería enseñar la paleta: %s", errb.String())
	}
}
