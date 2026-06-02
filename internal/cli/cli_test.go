package cli

import (
	"bytes"
	"testing"
)

func TestDispatchVersion(t *testing.T) {
	for _, arg := range []string{"version", "--version", "-v"} {
		var out, errb bytes.Buffer
		code := Dispatch([]string{arg}, &out, &errb)
		if code != 0 {
			t.Errorf("%q: exit = %d, quiero 0", arg, code)
		}
		if got := out.String(); got != "ccp v2.0.0\n" {
			t.Errorf("%q: stdout = %q, quiero %q", arg, got, "ccp v2.0.0\n")
		}
	}
}

func TestDispatchUnknown(t *testing.T) {
	var out, errb bytes.Buffer
	code := Dispatch([]string{"nope"}, &out, &errb)
	if code != 1 {
		t.Errorf("exit = %d, quiero 1", code)
	}
	if errb.Len() == 0 {
		t.Error("quiero mensaje en stderr para comando desconocido")
	}
}
