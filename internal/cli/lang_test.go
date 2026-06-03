package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestCmdLangSetAndShow(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CCP_HOME", home)
	t.Setenv("CCP_LANG", "") // sin override de env

	var out, errb bytes.Buffer
	if code := cmdLang([]string{"es"}, &out, &errb); code != 0 {
		t.Fatalf("set code=%d err=%s", code, errb.String())
	}
	if !strings.Contains(out.String(), "español") && !strings.Contains(out.String(), "es") {
		t.Fatalf("set out=%q", out.String())
	}

	out.Reset()
	errb.Reset()
	if code := cmdLang(nil, &out, &errb); code != 0 {
		t.Fatalf("show code=%d", code)
	}
	if !strings.Contains(out.String(), "es") || !strings.Contains(out.String(), "config") {
		t.Fatalf("show out=%q", out.String())
	}

	out.Reset()
	errb.Reset()
	if code := cmdLang([]string{"fr"}, &out, &errb); code != 1 {
		t.Fatalf("invalid code=%d want 1", code)
	}
}
