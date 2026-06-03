// parity_test.go cierra el gate de paridad Go↔bash (plan §9, Fase 4/7): corre
// el BINARIO GO sobre el mismo fixture CCP_HOME y exige que su stdout (redactado)
// y su exit code coincidan con los expected commiteados — que fueron capturados
// del oráculo bash por capture.sh. Verde aquí ⇒ Go == bash sobre el contrato
// observable (env delta, hook, resolve, path test, status --json, completions).
package golden

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// parityCase es un caso del arnés: nombre (= prefijo de los expected), cwd
// relativo al CCP_HOME temporal, y los args del binario.
type parityCase struct {
	name   string
	pwdRel string // "" => raíz del CCP_HOME
	args   []string
}

// cases espeja exactamente los `run ...` de testdata/golden/capture.sh.
var cases = []parityCase{
	{"env-default", "", []string{"_env", "default"}},
	{"env-deepseek", "", []string{"_env", "deepseek"}},
	{"env-official", "", []string{"_env", "work"}},
	{"env-missing", "", []string{"_env", "nope"}},

	{"resolve-official", "", []string{"resolve", "@/repos/work"}},
	{"resolve-deepseek", "", []string{"resolve", "@/repos/labs/x/y"}},
	{"resolve-nested", "", []string{"resolve", "@/repos/labs/secret/z"}},
	{"resolve-default", "", []string{"resolve", "@"}},

	{"hook-official", "", []string{"_hook", "@/repos/work"}},
	{"hook-deepseek", "", []string{"_hook", "@/repos/labs/x/y"}},
	{"hook-default", "", []string{"_hook", "@"}},

	{"pathtest-hit", "", []string{"path", "test", "@/repos/work"}},
	{"pathtest-miss", "", []string{"path", "test", "/tmp/no-such-rule-dir"}},

	{"status-json", "repos/work", []string{"status", "--json"}},

	{"completion-bash", "", []string{"completion", "bash"}},
	{"completion-zsh", "", []string{"completion", "zsh"}},
	{"completion-shellinit", "", []string{"completion-shellinit"}},
}

func TestGoBinaryMatchesBashGolden(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain no disponible; se omite el gate de paridad")
	}

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("no pude resolver la ruta del test")
	}
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")
	caseDir := filepath.Join(repoRoot, "testdata", "golden", "basic")
	expectedDir := filepath.Join(caseDir, "expected")
	fixture := filepath.Join(caseDir, "ccp-home")

	// 1) compilar el binario Go a un temporal.
	bin := filepath.Join(t.TempDir(), "ccp")
	build := exec.Command("go", "build", "-o", bin, "./cmd/ccp")
	build.Dir = repoRoot
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build falló:\n%s", out)
	}

	// 2) materializar CCP_HOME temporal con rutas reales (espejo de capture.sh).
	work := t.TempDir()
	if err := copyDir(fixture, work); err != nil {
		t.Fatalf("no pude copiar el fixture: %v", err)
	}
	rules, err := os.ReadFile(filepath.Join(fixture, "rules.tsv"))
	if err != nil {
		t.Fatalf("no pude leer rules.tsv del fixture: %v", err)
	}
	realRules := strings.ReplaceAll(string(rules), "__ROOT__", work)
	if err := os.WriteFile(filepath.Join(work, "rules.tsv"), []byte(realRules), 0o644); err != nil {
		t.Fatalf("no pude escribir rules.tsv: %v", err)
	}
	_ = os.Chmod(filepath.Join(work, "profiles", "deepseek", "api_key"), 0o600)
	for _, d := range []string{"repos/work", "repos/labs/secret"} {
		_ = os.MkdirAll(filepath.Join(work, d), 0o755)
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			pwd := work
			if c.pwdRel != "" {
				pwd = filepath.Join(work, c.pwdRel)
			}
			// '@' al inicio de un arg = placeholder de la raíz del CCP_HOME real.
			args := make([]string, len(c.args))
			for i, a := range c.args {
				args[i] = strings.Replace(a, "@", work, 1)
			}

			cmd := exec.Command(bin, args...)
			cmd.Dir = pwd
			// Entorno determinista: CCP_PROFILE scrubeado (active=default),
			// CCP_HOME fijo, NO_COLOR, PWD lógico = pwd (lo usa `status`).
			cmd.Env = append(os.Environ(),
				"CCP_HOME="+work,
				"NO_COLOR=1",
				"PWD="+pwd,
				"CCP_LANG=es", // igualar al oráculo bash (prosa en español)
			)
			cmd.Env = filterEnv(cmd.Env, "CCP_PROFILE")

			out, code := runCapture(cmd)
			gotOut := strings.ReplaceAll(out, work, "__CCP_HOME__")

			wantOut := readExpected(t, expectedDir, c.name+".out")
			wantCode := strings.TrimSpace(readExpected(t, expectedDir, c.name+".code"))

			if gotOut != wantOut {
				t.Errorf("%s: stdout difiere del oráculo bash\n--- got ---\n%s\n--- want ---\n%s", c.name, gotOut, wantOut)
			}
			if got := strconv.Itoa(code); got != wantCode {
				t.Errorf("%s: exit code got=%s want=%s", c.name, got, wantCode)
			}
		})
	}
}

// runCapture corre cmd y devuelve (stdout, exitCode). stderr se descarta (igual
// que capture.sh con 2>/dev/null). El expected se compara SIN el newline final
// que printf añade en emit(): los .out se escriben con `printf '%s\n'`, así que
// re-añadimos un \n al stdout capturado para igualar el formato.
func runCapture(cmd *exec.Cmd) (string, int) {
	var buf strings.Builder
	cmd.Stdout = &buf
	cmd.Stderr = nil
	err := cmd.Run()
	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		code = -1
	}
	// emit() en capture.sh hace `printf '%s\n' "$content"`: el expected lleva un
	// newline final que el stdout del binario ya provee (todos terminan en \n).
	return buf.String(), code
}

func readExpected(t *testing.T, dir, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatalf("no pude leer expected %s: %v", name, err)
	}
	return string(data)
}

func filterEnv(env []string, key string) []string {
	out := env[:0]
	prefix := key + "="
	for _, e := range env {
		if strings.HasPrefix(e, prefix) {
			continue
		}
		out = append(out, e)
	}
	return out
}

// copyDir copia recursivamente src→dst (archivos y symlinks; preserva modos).
func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		switch {
		case info.IsDir():
			return os.MkdirAll(target, 0o755)
		case info.Mode()&os.ModeSymlink != 0:
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			_ = os.MkdirAll(filepath.Dir(target), 0o755)
			_ = os.Remove(target)
			return os.Symlink(link, target)
		default:
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			return os.WriteFile(target, data, info.Mode().Perm())
		}
	})
}
