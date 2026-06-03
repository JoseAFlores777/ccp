package tui

import "testing"

func TestCompleteCmd(t *testing.T) {
	cases := []struct{ in, want string }{
		{"ba", "backup-"}, // prefijo común de backup-export/restore
		{"backup-e", "backup-export"},
		{"d", "doctor"},
		{"s", "sync"},
		{"i", "install"},
		{"x", "x"}, // sin coincidencias: no cambia
		{"", ""},   // todos: prefijo común vacío
	}
	for _, c := range cases {
		if got := completeCmd(c.in); got != c.want {
			t.Errorf("completeCmd(%q) = %q, quiero %q", c.in, got, c.want)
		}
	}
}

func TestCmdMatches(t *testing.T) {
	if got := cmdMatches("backup-"); len(got) != 2 {
		t.Errorf("cmdMatches(backup-) = %v, quiero 2", got)
	}
	if got := cmdMatches("zzz"); len(got) != 0 {
		t.Errorf("cmdMatches(zzz) = %v, quiero 0", got)
	}
}
