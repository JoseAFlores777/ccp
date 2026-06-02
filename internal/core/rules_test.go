package core

import (
	"os/exec"
	"strings"
	"testing"
)

func TestResolveUnit(t *testing.T) {
	rules := []Rule{
		{Path: "/r/repos/work", Profile: "work"},
		{Path: "/r/repos/labs", Profile: "deepseek"},
		{Path: "/r/repos/labs/secret", Profile: "work"},
	}
	cases := []struct {
		query string
		want  string
	}{
		{"/r/repos/work", "work"},          // match exacto
		{"/r/repos/work/sub", "work"},      // descendiente
		{"/r/repos/labs/x/y", "deepseek"},  // ancestro labs
		{"/r/repos/labs/secret/z", "work"}, // deepest-wins: secret > labs
		{"/r", "default"},                  // ninguna regla aplica
		{"/", "default"},                   // raíz: ninguna
		{"/r/repos/labsX", "default"},      // no es descendiente de labs
		{"/r/repos/labs", "deepseek"},      // match exacto labs
		{"/r/repos/labs/secret", "work"},   // match exacto secret
	}
	for _, c := range cases {
		if got := Resolve(c.query, rules); got != c.want {
			t.Errorf("Resolve(%q) = %q, want %q", c.query, got, c.want)
		}
	}
}

func TestResolveRootRuleMatchesAll(t *testing.T) {
	rules := []Rule{{Path: "/", Profile: "global"}}
	if got := Resolve("/anything/deep", rules); got != "global" {
		t.Errorf("regla raíz debe ser ancestro de todo: got %q", got)
	}
}

func TestResolveNoRules(t *testing.T) {
	if got := Resolve("/x/y", nil); got != "default" {
		t.Errorf("sin reglas -> default, got %q", got)
	}
}

// TestShellQuoteMatchesBash valida shellQuote BYTE-A-BYTE contra bash `printf
// %q` real, sobre un espectro de entradas. Es el gate directo del #1 riesgo.
func TestShellQuoteMatchesBash(t *testing.T) {
	bashPath, ok := lookShell("bash")
	if !ok {
		t.Skip("bash no disponible")
	}
	inputs := []string{
		"",
		"plain123",
		"deepseek-chat",
		"default",
		"high",
		"https://api.deepseek.com/anthropic",
		"sk-fixture-deepseek-key-0000",
		"/home/user/.config/ccp/profiles/work/cc-home",
		"a path with spaces",
		"value'with'quotes",
		"value\"with\"dquotes",
		"dollar$var",
		"back`tick`",
		"amp&and;semi|pipe",
		"glob*star?question[bracket]",
		"paren(s)brace{s}",
		"lt<gt>",
		"bang!here",
		"caret^x",
		"comma,sep",
		"trail#hash",
		"#leadhash",
		"~leadtilde",
		"mid~tilde",
		"at@colon:plus+eq=pct%dot.dash-under_",
		"backslash\\here",
		"https://api example.com/v1?q=a b&x=1",
		"sk-tok en_with space&special$1",
		"héllo-unicode-✓",
	}
	for _, in := range inputs {
		want := bashQuote(t, bashPath, in)
		got := shellQuote(in)
		if got != want {
			t.Errorf("shellQuote(%q) = %q, bash %%q = %q", in, got, want)
		}
	}
}

// bashQuote ejecuta `printf %q` en bash para una entrada dada (pasada por
// argumento posicional para evitar reinterpretación). Devuelve el resultado
// sin newline final.
func bashQuote(t *testing.T, bashPath, s string) string {
	t.Helper()
	cmd := exec.Command(bashPath, "-c", `printf '%q' "$1"`, "bash", s)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("bash printf %%q falló para %q: %v", s, err)
	}
	return strings.TrimRight(string(out), "\n")
}
