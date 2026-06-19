package tui

import (
	"testing"

	"github.com/JoseAFlores777/ccp/internal/core"
)

func TestHandoffProfileOptionsExcludesActive(t *testing.T) {
	cfg := &core.Config{Profiles: map[string]core.Profile{
		"personal-cc": {Type: "official"}, "emco-cc": {Type: "official"},
	}}
	opts := HandoffProfileOptions(cfg, "personal-cc")
	for _, o := range opts {
		if o == "personal-cc" {
			t.Fatal("el perfil activo no debe aparecer como destino")
		}
	}
	if len(opts) != 1 || opts[0] != "emco-cc" {
		t.Fatalf("opciones = %v, want [emco-cc]", opts)
	}
}
