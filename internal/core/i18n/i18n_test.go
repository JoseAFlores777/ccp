package i18n

import "testing"

func TestResolvePrecedence(t *testing.T) {
	cases := []struct {
		name    string
		env     string // valor de CCP_LANG ("" = unset)
		cfg     string
		wantL   Lang
		wantSrc Source
	}{
		{"env gana sobre cfg", "en", "es", En, SourceEnv},
		{"cfg cuando no hay env", "", "es", Es, SourceConfig},
		{"default cuando nada", "", "", En, SourceDefault},
		{"env invalido cae a default", "fr", "", En, SourceDefault},
		{"cfg invalido cae a default", "", "klingon", En, SourceDefault},
		{"case/space insensitive", "  ES ", "", Es, SourceEnv},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if c.env == "" {
				t.Setenv("CCP_LANG", "")
			} else {
				t.Setenv("CCP_LANG", c.env)
			}
			gotL, gotSrc := ResolveWithSource(c.cfg)
			if gotL != c.wantL || gotSrc != c.wantSrc {
				t.Fatalf("got (%q,%q) want (%q,%q)", gotL, gotSrc, c.wantL, c.wantSrc)
			}
			if Resolve(c.cfg) != c.wantL {
				t.Fatalf("Resolve=%q want %q", Resolve(c.cfg), c.wantL)
			}
		})
	}
}
