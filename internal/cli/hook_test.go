package cli

import (
	"bytes"
	"os/exec"
	"strings"
	"testing"

	"github.com/JoseAFlores777/ccp/internal/core"
)

// hook_test.go — el aviso de handoff activo que _ccp_autocheck imprime al
// entrar (`cd`) a un repo con préstamo vivo. Viaja DENTRO del emit como
// `echo … >&2` porque el hook se evalúa en la shell del usuario; por eso los
// tests comprueban tanto el texto como que el emit evalúa limpio.

func TestHookAvisaHandoffActivo(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CCP_HOME", home)
	t.Setenv("CCP_LANG", "es")
	cwd := "/repo/uno"
	if err := core.SaveHandoffs(home, &core.Handoffs{
		Version: core.HandoffsVersion,
		Active: []core.Marker{{
			Session: "aaa", Slug: core.SlugForCwd(cwd), Cwd: cwd,
			From: "personal-cc", To: "emco-cc", Since: "2026-07-25T00:00:00Z",
		}},
	}); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	if code := Dispatch([]string{"_hook", cwd}, &out, &errb); code != 0 {
		t.Fatalf("_hook debe salir 0, got %d", code)
	}
	s := out.String()
	if !strings.Contains(s, "handoff activo") || !strings.Contains(s, ">&2") {
		t.Fatalf("el emit del hook debe llevar el aviso a stderr: %s", s)
	}
	if !strings.Contains(s, "CLAUDE_CONFIG_DIR") && !strings.Contains(s, "unset") {
		t.Fatalf("el hook debe seguir emitiendo el delta de env: %s", s)
	}
}

func TestHookSinHandoffNoAvisa(t *testing.T) {
	t.Setenv("CCP_HOME", t.TempDir())
	var out, errb bytes.Buffer
	Dispatch([]string{"_hook", "/repo/uno"}, &out, &errb)
	if strings.Contains(out.String(), "handoff activo") {
		t.Fatalf("sin marcadores no debe avisar: %s", out.String())
	}
}

// Un activo en OTRO repo no debe avisar aquí: el aviso es por cwd.
func TestHookHandoffDeOtroRepoNoAvisa(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CCP_HOME", home)
	t.Setenv("CCP_LANG", "es")
	_ = core.SaveHandoffs(home, &core.Handoffs{
		Version: core.HandoffsVersion,
		Active: []core.Marker{{
			Session: "aaa", Slug: core.SlugForCwd("/repo/otro"), Cwd: "/repo/otro",
			From: "personal-cc", To: "emco-cc", Since: "2026-07-25T00:00:00Z",
		}},
	})
	var out, errb bytes.Buffer
	Dispatch([]string{"_hook", "/repo/uno"}, &out, &errb)
	if strings.Contains(out.String(), "handoff activo") {
		t.Fatalf("un handoff de otro repo no debe avisar aquí: %s", out.String())
	}
}

func TestHookAvisaVariosHandoffs(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CCP_HOME", home)
	t.Setenv("CCP_LANG", "es")
	cwd := "/repo/uno"
	_ = core.SaveHandoffs(home, &core.Handoffs{
		Version: core.HandoffsVersion,
		Active: []core.Marker{
			{Session: "aaa", Slug: core.SlugForCwd(cwd), Cwd: cwd, From: "personal-cc", To: "emco-cc", Since: "2026-07-25T00:00:00Z"},
			{Session: "bbb", Slug: core.SlugForCwd(cwd), Cwd: cwd, From: "personal-cc", To: "kimi", Since: "2026-07-25T01:00:00Z"},
		},
	})
	var out, errb bytes.Buffer
	Dispatch([]string{"_hook", cwd}, &out, &errb)
	if !strings.Contains(out.String(), "2 handoffs activos") {
		t.Fatalf("con 2+ activos el aviso debe ser el plural: %s", out.String())
	}
}

// El aviso lleva backticks (\`ccp handoff end\`): dentro de echo "…" tienen que
// ir escapados o bash los ejecutaría como sustitución de comandos. Este test
// EVALÚA el emit en bash real, que es donde eso se nota.
func TestHookAvisoEsEvalSeguro(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CCP_HOME", home)
	t.Setenv("CCP_LANG", "es")
	cwd := "/repo/uno"
	_ = core.SaveHandoffs(home, &core.Handoffs{
		Version: core.HandoffsVersion,
		Active: []core.Marker{{
			Session: "aaa", Slug: core.SlugForCwd(cwd), Cwd: cwd,
			From: "personal-cc", To: "emco-cc", Since: "2026-07-25T00:00:00Z",
		}},
	})
	var out, errb bytes.Buffer
	Dispatch([]string{"_hook", cwd}, &out, &errb)

	shOut, err := exec.Command("bash", "-c", out.String()).CombinedOutput()
	if err != nil {
		t.Fatalf("el emit del hook no evalúa limpio: %v\n%s", err, shOut)
	}
	if !strings.Contains(string(shOut), "handoff activo") {
		t.Fatalf("el aviso no llegó a stderr: %s", shOut)
	}
	// Los backticks deben llegar LITERALES: si bash los hubiera ejecutado,
	// `ccp handoff end` habría desaparecido del texto.
	if !strings.Contains(string(shOut), "`ccp handoff end`") {
		t.Fatalf("los backticks no llegaron literales (¿sustitución de comandos?): %s", shOut)
	}
}

// Un nombre de perfil con metacaracteres no debe ejecutarse al evaluar el hook.
func TestHookAvisoNoEjecutaMetacaracteres(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CCP_HOME", home)
	t.Setenv("CCP_LANG", "es")
	cwd := "/repo/uno"
	_ = core.SaveHandoffs(home, &core.Handoffs{
		Version: core.HandoffsVersion,
		Active: []core.Marker{{
			Session: "aaa", Slug: core.SlugForCwd(cwd), Cwd: cwd,
			From: "p$(echo PWNED)", To: "e`echo PWNED2`", Since: "2026-07-25T00:00:00Z",
		}},
	})
	var out, errb bytes.Buffer
	Dispatch([]string{"_hook", cwd}, &out, &errb)

	shOut, err := exec.Command("bash", "-c", out.String()).CombinedOutput()
	if err != nil {
		t.Fatalf("el emit del hook no evalúa limpio: %v\n%s", err, shOut)
	}
	// Los datos tienen que llegar LITERALES: si bash los hubiera expandido,
	// `$(echo PWNED)` habría quedado reducido a `PWNED` en el texto.
	if !strings.Contains(string(shOut), "p$(echo PWNED)") || !strings.Contains(string(shOut), "e`echo PWNED2`") {
		t.Fatalf("el aviso expandió datos del marcador: %s", shOut)
	}
}
