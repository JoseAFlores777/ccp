package cli

import (
	"github.com/JoseAFlores777/ccp/internal/core"
	"github.com/JoseAFlores777/ccp/internal/core/i18n"
)

// currentLang resuelve el idioma efectivo para la salida del CLI cuando el
// comando no tiene ya un *core.Config a mano (help, version, comando
// desconocido). Best-effort: si no hay home o ccp.yaml aún, Resolve("") cae a
// CCP_LANG o en sin fallar. Los comandos que YA cargaron cfg deben llamar
// i18n.Resolve(cfg.Lang) directo en vez de esto.
func currentLang() i18n.Lang {
	home, err := ccpHome()
	if err != nil {
		return i18n.Resolve("")
	}
	cfg, err := core.Load(home) // no migra: leer lang no justifica disparar migración
	if err != nil {
		return i18n.Resolve("")
	}
	return i18n.Resolve(cfg.Lang)
}
