package tui

import (
	"strings"
	"testing"

	"github.com/JoseAFlores777/ccp/internal/core"
	"github.com/JoseAFlores777/ccp/internal/core/i18n"
)

// newTestModel arma un model mínimo apuntado a un home temporal, con dimensiones
// suficientes para que el render no colapse las cajas.
func newTestModel(t *testing.T) *model {
	t.Helper()
	home := t.TempDir()
	m := &model{
		home:   home,
		cfg:    &core.Config{Version: core.SchemaVersion},
		focus:  panelProfiles,
		mode:   modeDashboard,
		width:  100,
		height: 40,
	}
	m.reload()
	return m
}

func TestDashboardRendersBothLangs(t *testing.T) {
	for _, l := range []i18n.Lang{i18n.En, i18n.Es} {
		m := newTestModel(t)
		m.lang = l
		out := m.viewDashboard()
		if strings.TrimSpace(out) == "" {
			t.Fatalf("lang %q: render vacío", l)
		}
		if strings.Contains(out, "tui.") {
			t.Fatalf("lang %q: key sin traducir en render:\n%s", l, out)
		}
	}
}
