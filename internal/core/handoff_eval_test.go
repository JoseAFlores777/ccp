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

			emit, err := HandoffForward(home, "personal-cc", "emco-cc", cwd, uuid, true, time.Now())
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
