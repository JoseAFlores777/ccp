package cli

import (
	"os"
	"testing"
)

// TestMain aísla a todo el paquete de lo que el snapshot automático leería fuera
// de CCP_HOME: ~/.claude y la ventana `default` de Desktop. Por defecto el
// snapshot automático va APAGADO en los tests para que los que ya existían no
// cambien de comportamiento; los que lo prueban lo encienden con
// t.Setenv("CCP_NO_AUTO_SNAPSHOT", "").
func TestMain(m *testing.M) {
	os.Setenv("CCP_NO_AUTO_SNAPSHOT", "1")
	var tmp string
	if os.Getenv("CCP_DESKTOP_DEFAULT_DATA_DIR") == "" {
		if d, err := os.MkdirTemp("", "ccp-desktop-default-*"); err == nil {
			tmp = d
			os.Setenv("CCP_DESKTOP_DEFAULT_DATA_DIR", d)
		}
	}
	code := m.Run()
	if tmp != "" {
		os.RemoveAll(tmp)
	}
	os.Exit(code)
}
