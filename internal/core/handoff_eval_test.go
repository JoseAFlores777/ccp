package core

import (
	"os/exec"
	"strings"
	"testing"
	"time"
)

// TestHandoffEmitEvalEffect verifica que el emit de HandoffForward, al ser
// evaluado en bash y zsh reales, exporta CLAUDE_CONFIG_DIR del perfil destino
// y define CCP_RESUME_ID igual al uuid de la sesión.
func TestHandoffEmitEvalEffect(t *testing.T) {
	for _, sh := range []string{"bash", "zsh"} {
		sh := sh
		t.Run(sh, func(t *testing.T) {
			shPath, ok := lookShell(sh)
			if !ok {
				t.Skipf("%s no disponible en PATH", sh)
			}

			home := t.TempDir()
			seedHandoffEnv(t, home)

			cwd := "/repo"
			uuid := "cdcdcdcd-cdcd-4dcd-8dcd-cdcdcdcdcdcd"
			srcDir := ProjectDir(home+"/profiles/personal-cc/cc-home", SlugForCwd(cwd))
			writeJSONL(t, srcDir, uuid, "T", time.Now())

			emit, err := HandoffForward(home, "personal-cc", "emco-cc", cwd, uuid, true, false, false, time.Now())
			if err != nil {
				t.Fatalf("HandoffForward: %v", err)
			}

			// Evaluar el emit en el shell y capturar las variables de interés.
			script := emit + "\necho \"CCD=$CLAUDE_CONFIG_DIR\"\necho \"RID=$CCP_RESUME_ID\"\n"
			out, err := exec.Command(shPath, "-c", script).CombinedOutput()
			if err != nil {
				t.Fatalf("%s eval falló: %v\nsalida:\n%s\nscript:\n%s", sh, err, out, script)
			}

			s := string(out)
			if !strings.Contains(s, "emco-cc/cc-home") {
				t.Errorf("%s: CLAUDE_CONFIG_DIR no apunta al cc-home del destino:\n%s", sh, s)
			}
			if !strings.Contains(s, "RID="+uuid) {
				t.Errorf("%s: CCP_RESUME_ID no es el uuid esperado:\n%s", sh, s)
			}
		})
	}
}

// TestHandoffYoloEvalEffect verifica en bash y zsh reales que el emit define
// CCP_RESUME_YOLO con --yolo y lo deja SIN definir en el caso normal, incluso
// si la variable venía seteada del entorno (por eso el `export` previo).
func TestHandoffYoloEvalEffect(t *testing.T) {
	for _, sh := range []string{"bash", "zsh"} {
		sh := sh
		t.Run(sh, func(t *testing.T) {
			shPath, ok := lookShell(sh)
			if !ok {
				t.Skipf("%s no disponible en PATH", sh)
			}
			home := t.TempDir()
			seedHandoffEnv(t, home)
			cwd := "/repo"
			uuid := "dddddddd-dddd-4ddd-8ddd-dddddddddddd"
			srcDir := ProjectDir(home+"/profiles/personal-cc/cc-home", SlugForCwd(cwd))
			writeJSONL(t, srcDir, uuid, "T", time.Now())

			for _, tc := range []struct {
				name string
				yolo bool
				want string
			}{
				{"con yolo", true, "YOLO=1"},
				{"sin yolo", false, "YOLO="},
			} {
				emit, err := HandoffForward(home, "personal-cc", "emco-cc", cwd, uuid, false, tc.yolo, false, time.Now())
				if err != nil {
					t.Fatalf("%s: %v", tc.name, err)
				}
				script := "export CCP_RESUME_YOLO=heredado\n" + emit + "\necho \"YOLO=$CCP_RESUME_YOLO\"\n"
				out, err := exec.Command(shPath, "-c", script).CombinedOutput()
				if err != nil {
					t.Fatalf("%s/%s eval falló: %v\nsalida:\n%s", sh, tc.name, err, out)
				}
				// Comparación EXACTA de la última línea: "YOLO=" es substring de
				// "YOLO=1", así que un Contains dejaría pasar un unset que no ocurrió
				// (la variable venía como "heredado" del export previo).
				lines := strings.Split(strings.TrimRight(string(out), "\n"), "\n")
				got := lines[len(lines)-1]
				if got != tc.want {
					t.Errorf("%s/%s: esperaba %q, got %q (salida completa:\n%s)", sh, tc.name, tc.want, got, out)
				}
			}
		})
	}
}
