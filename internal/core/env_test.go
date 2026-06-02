package core

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Infra de fixture: materializa un CCP_HOME temporal desde
// testdata/golden/basic/ccp-home EXACTAMENTE como capture.sh (sustituye
// __ROOT__ en rules.tsv, chmod 600 la api_key, crea repos/*), corre Migrate
// para producir ccp.yaml, y carga el Config.
// ---------------------------------------------------------------------------

func envGoldenDir(t *testing.T) string {
	t.Helper()
	// internal/core -> ../../testdata/golden/basic
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("no se pudo localizar el archivo de test")
	}
	root := filepath.Join(filepath.Dir(file), "..", "..")
	return filepath.Join(root, "testdata", "golden", "basic")
}

func copyTreeTest(t *testing.T, src, dst string) {
	t.Helper()
	err := filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.WriteFile(target, data, info.Mode().Perm())
	})
	if err != nil {
		t.Fatalf("copyTree %s -> %s: %v", src, dst, err)
	}
}

// materializeHome crea el CCP_HOME temporal y devuelve (home, cfg).
func materializeHome(t *testing.T) (string, *Config) {
	t.Helper()
	gd := envGoldenDir(t)
	fixture := filepath.Join(gd, "ccp-home")

	home := t.TempDir()
	copyTreeTest(t, fixture, home)

	// sustituir __ROOT__ por el home temporal en rules.tsv (como capture.sh).
	rulesPath := filepath.Join(home, "rules.tsv")
	raw, err := os.ReadFile(rulesPath)
	if err != nil {
		t.Fatalf("leyendo rules.tsv: %v", err)
	}
	out := strings.ReplaceAll(string(raw), "__ROOT__", home)
	if err := os.WriteFile(rulesPath, []byte(out), 0o644); err != nil {
		t.Fatalf("escribiendo rules.tsv: %v", err)
	}

	// chmod 600 api_key.
	_ = os.Chmod(filepath.Join(home, "profiles", "deepseek", "api_key"), 0o600)

	// dirs-regla reales.
	for _, d := range []string{"repos/work", "repos/labs/secret"} {
		if err := os.MkdirAll(filepath.Join(home, d), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", d, err)
		}
	}

	// TSV/meta -> ccp.yaml.
	if err := MigrateFrom(home, "", "test"); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	cfg, err := Load(home)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return home, cfg
}

// writeStressProfileOnDisk escribe un perfil deepseek (con valores que
// contienen espacios/especiales) en el formato TSV/meta que el oráculo bash
// lee, para validar paridad real de eval-effect contra bin/ccp _env.
func writeStressProfileOnDisk(t *testing.T, home string, p Profile, key string) {
	t.Helper()
	dir := filepath.Join(home, "profiles", "stress")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir stress: %v", err)
	}
	meta := "type=deepseek\n" +
		"base_url=" + p.BaseURL + "\n" +
		"model_pro=" + p.ModelPro + "\n" +
		"model_flash=" + p.ModelFlash + "\n" +
		"effort=" + p.Effort + "\n"
	if err := os.WriteFile(filepath.Join(dir, "meta"), []byte(meta), 0o644); err != nil {
		t.Fatalf("escribir meta stress: %v", err)
	}
	if err := SetKey(home, "stress", key); err != nil {
		t.Fatalf("SetKey stress: %v", err)
	}
	// índice profiles.tsv: añade la entrada stress.
	idx := filepath.Join(home, "profiles.tsv")
	data, _ := os.ReadFile(idx)
	if err := os.WriteFile(idx, append(data, []byte("stress\tdeepseek\n")...), 0o644); err != nil {
		t.Fatalf("escribir profiles.tsv: %v", err)
	}
}

func envReadGolden(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(envGoldenDir(t), "expected", name))
	if err != nil {
		t.Fatalf("leyendo golden %s: %v", name, err)
	}
	return string(data)
}

func redact(s, home string) string {
	return strings.ReplaceAll(s, home, "__CCP_HOME__")
}

// ---------------------------------------------------------------------------
// 1) Golden-diff: EnvDelta byte-idéntico a los expected del oráculo.
// ---------------------------------------------------------------------------

func TestEnvDeltaGolden(t *testing.T) {
	home, cfg := materializeHome(t)

	cases := []struct {
		golden  string
		profile string
	}{
		{"env-default.out", "default"},
		{"env-deepseek.out", "deepseek"},
		{"env-official.out", "work"},
		{"env-missing.out", "nope"},
	}
	for _, c := range cases {
		t.Run(c.golden, func(t *testing.T) {
			got := redact(EnvDelta(home, c.profile, cfg), home)
			want := envReadGolden(t, c.golden)
			if got != want {
				t.Errorf("EnvDelta(%q) mismatch:\n--- got ---\n%q\n--- want ---\n%q", c.profile, got, want)
			}
		})
	}
}

// hook reusa EnvDelta sobre el perfil resuelto del path: mismos goldens.
func TestHookGolden(t *testing.T) {
	home, cfg := materializeHome(t)

	cases := []struct {
		golden string
		query  string
	}{
		{"hook-official.out", filepath.Join(home, "repos", "work")},
		{"hook-deepseek.out", filepath.Join(home, "repos", "labs", "x", "y")},
		{"hook-default.out", home},
	}
	for _, c := range cases {
		t.Run(c.golden, func(t *testing.T) {
			prof := Resolve(c.query, cfg.Rules)
			got := redact(EnvDelta(home, prof, cfg), home)
			want := envReadGolden(t, c.golden)
			if got != want {
				t.Errorf("hook(%q)->%q mismatch:\n--- got ---\n%q\n--- want ---\n%q", c.query, prof, got, want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// 2) Resolve / path-test goldens + exit-code semantics (0=no-default, 1=default).
// ---------------------------------------------------------------------------

func TestResolveGolden(t *testing.T) {
	home, cfg := materializeHome(t)

	cases := []struct {
		out, code string
		query     string
	}{
		{"resolve-official.out", "resolve-official.code", filepath.Join(home, "repos", "work")},
		{"resolve-deepseek.out", "resolve-deepseek.code", filepath.Join(home, "repos", "labs", "x", "y")},
		{"resolve-nested.out", "resolve-nested.code", filepath.Join(home, "repos", "labs", "secret", "z")},
		{"resolve-default.out", "resolve-default.code", home},
		{"pathtest-hit.out", "pathtest-hit.code", filepath.Join(home, "repos", "work")},
		{"pathtest-miss.out", "pathtest-miss.code", "/tmp/no-such-rule-dir"},
	}
	for _, c := range cases {
		t.Run(c.out, func(t *testing.T) {
			prof := Resolve(c.query, cfg.Rules)
			got := prof + "\n" // oráculo imprime nombre + newline (printf '%s\n' via emit)
			want := envReadGolden(t, c.out)
			if got != want {
				t.Errorf("Resolve(%q) = %q, want %q", c.query, got, want)
			}
			// exit code: 0 si no-default, 1 si default.
			wantCode := strings.TrimSpace(envReadGolden(t, c.code))
			gotCode := "0"
			if prof == "default" {
				gotCode = "1"
			}
			if gotCode != wantCode {
				t.Errorf("Resolve(%q) exit-code = %s, want %s (profile=%q)", c.query, gotCode, wantCode, prof)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// 3) eval-effect: el verdadero gate anti-quoting. Toma la salida de EnvDelta,
// la evalúa en bash Y zsh reales, y compara el SET de variables gestionadas
// con lo que el oráculo bash (bin/ccp _env <p>) produce bajo el mismo home.
// ---------------------------------------------------------------------------

// dumpManagedVars genera el snippet que, tras el eval del delta, imprime cada
// var gestionada como NAME=VALUE\n (solo las definidas). Determinista y
// portable entre bash/zsh.
func dumpManagedVarsScript() string {
	var b strings.Builder
	for _, v := range strings.Fields(CCPManagedVars) {
		// Usa parameter expansion indirecta evitando: imprime solo si set.
		b.WriteString("if [ -n \"${" + v + "+x}\" ]; then printf '%s=%s\\n' " + v + " \"${" + v + "}\"; fi\n")
	}
	return b.String()
}

func evalAndDump(t *testing.T, shell, shellPath, delta, home string) (map[string]string, bool) {
	t.Helper()
	script := delta + "\n" + dumpManagedVarsScript()
	cmd := exec.Command(shellPath, "-c", script)
	// Entorno mínimo: limpia cualquier var gestionada heredada para que el
	// SET reflejado venga solo del delta.
	env := []string{"HOME=" + home, "PATH=/usr/bin:/bin"}
	cmd.Env = env
	outBytes, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s eval falló: %v\nsalida:\n%s\nscript:\n%s", shell, err, outBytes, script)
	}
	return parseDump(string(outBytes)), true
}

func parseDump(s string) map[string]string {
	m := map[string]string{}
	for _, line := range strings.Split(s, "\n") {
		if line == "" {
			continue
		}
		i := strings.IndexByte(line, '=')
		if i < 0 {
			continue
		}
		m[line[:i]] = line[i+1:]
	}
	return m
}

func sortedKeys(m map[string]string) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

func TestEvalEffect(t *testing.T) {
	home, cfg := materializeHome(t)

	bashPath, hasBash := lookShell("bash")
	zshPath, hasZsh := lookShell("zsh")
	if !hasBash && !hasZsh {
		t.Skip("ni bash ni zsh disponibles")
	}

	// Inyecta valores con espacio y URL/token para estresar shellQuote:
	// añadimos un perfil deepseek sintético con base_url con espacio. Se
	// escribe TANTO en el Config en memoria COMO en disco (meta + índice +
	// api_key) para que el oráculo bash bin/ccp lo conozca y la comparación de
	// paridad sea legítima (no la rama "perfil no existe").
	stress := Profile{
		Type:       "deepseek",
		BaseURL:    "https://api example.com/v1?q=a b&x=1",
		ModelPro:   "model pro-v2",
		ModelFlash: "model flash",
		Effort:     "high",
	}
	cfg.Profiles["stress"] = stress
	writeStressProfileOnDisk(t, home, stress, "sk-tok en_with space&special$1")

	profiles := []string{"default", "work", "deepseek", "stress", "nope"}

	for _, prof := range profiles {
		delta := EnvDelta(home, prof, cfg)
		t.Run(prof, func(t *testing.T) {
			var bashVars, zshVars map[string]string
			if hasBash {
				bashVars, _ = evalAndDump(t, "bash", bashPath, delta, home)
			}
			if hasZsh {
				zshVars, _ = evalAndDump(t, "zsh", zshPath, delta, home)
			}
			if hasBash && hasZsh {
				if !equalMaps(bashVars, zshVars) {
					t.Errorf("bash vs zsh divergen para %q:\nbash=%v\nzsh=%v", prof, bashVars, zshVars)
				}
			}
			// El SET de variables debe coincidir con el oráculo bin/ccp _env.
			ref := oracleEnv(t, home, prof)
			if ref == nil {
				t.Skip("oráculo bin/ccp no disponible/ejecutable")
			}
			check := bashVars
			if check == nil {
				check = zshVars
			}
			if !equalMaps(check, ref) {
				t.Errorf("eval-effect vs oráculo divergen para %q:\nkeys eval=%v\nkeys oracle=%v\neval=%v\noracle=%v",
					prof, sortedKeys(check), sortedKeys(ref), check, ref)
			}
		})
	}
}

func equalMaps(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

func lookShell(name string) (string, bool) {
	// Prefiere el que el oráculo usaría (env bash -> PATH). Busca primero en
	// /opt/homebrew/bin (bash 5.x en macOS) y luego en PATH.
	for _, p := range []string{"/opt/homebrew/bin/" + name, "/usr/local/bin/" + name} {
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			return p, true
		}
	}
	if p, err := exec.LookPath(name); err == nil {
		return p, true
	}
	return "", false
}

// oracleEnv corre el binario bash oráculo bin/ccp _env <profile> bajo el home
// dado y devuelve el SET de variables gestionadas que su salida exporta, tras
// evaluarla en bash. Devuelve nil si el oráculo no es ejecutable.
func oracleEnv(t *testing.T, home, profile string) map[string]string {
	t.Helper()
	gd := envGoldenDir(t)
	oracle := filepath.Join(gd, "..", "..", "..", "bin", "ccp")
	if fi, err := os.Stat(oracle); err != nil || fi.IsDir() {
		return nil
	}
	bashPath, ok := lookShell("bash")
	if !ok {
		return nil
	}
	// Genera el delta con el oráculo (CCP_HOME=home), luego lo evalúa.
	cmd := exec.Command(oracle, "_env", profile)
	cmd.Env = []string{"HOME=" + home, "CCP_HOME=" + home, "NO_COLOR=1", "PATH=/opt/homebrew/bin:/usr/bin:/bin"}
	out, err := cmd.Output()
	if err != nil {
		t.Logf("oráculo _env %q falló: %v", profile, err)
		return nil
	}
	vars, _ := evalAndDump(t, "bash(oracle-delta)", bashPath, string(out), home)
	return vars
}
