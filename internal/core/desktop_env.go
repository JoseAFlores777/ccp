package core

import "strings"

// desktop_env.go — la barrera del updater.
//
// El 2026-09-15 una instancia de perfil actualizó el Claude.app del usuario.
// El log lo dice sin ambigüedad (~/Library/Caches/com.anthropic.claudefordesktop.ShipIt/
// ShipIt_stderr.log):
//
//	22:04:21 Beginning installation
//	22:04:25 Moving bundle from file:///Applications/Claude.app/ to /var/folders/…
//	22:04:26 Installation completed successfully
//
// La app pasó de 1.52386.3 a 2.110.0 sin que nadie lo pidiera desde esa ventana,
// y el espejo del lanzador se quedó en la versión vieja. CLAUDE.md afirmaba que
// eso era imposible («The instance cannot self-update: Squirrel looks in the
// download for a bundle with the running app's id and finds none»); la afirmación
// era falsa. SQRLUpdater compara contra
// `NSRunningApplication.currentApplication.bundleIdentifier`, y ese id es el REAL
// en cuanto el proceso corre desde el espejo — que es donde acaba siempre el
// camino `desktop open`, y donde acaba el lanzador tras cualquier re-exec de
// `process.execPath` (Electron lo resuelve con realpath, así que apunta al
// espejo y la identidad externa no vuelve).
//
// Que la instancia actualice es malo de dos maneras distintas:
//
//   - por el camino `desktop open` toca /Applications/Claude.app, es decir el
//     Claude PRINCIPAL del usuario, desde una ventana que él abrió para otra
//     cuenta;
//   - por el camino del lanzador el `targetBundleURL` sería el ESPEJO, lo que
//     invalidaría el ElectronAsarIntegrity del plist externo y dejaría ese
//     lanzador roto de forma permanente.
//
// DISABLE_UPDATE_CHECK es la palanca. No está documentada, pero está en el
// binario: la cadena aparece dos veces en
// Claude.app/Contents/Frameworks/Squirrel.framework/Versions/A/Squirrel, junto a
// SQRLUpdater.m, y SQRLUpdater la lee con getenv() una sola vez al inicializarse,
// dejando su checkForUpdatesCommand deshabilitado para toda la vida del proceso.
// Por ser una bandera no documentada, `ccp desktop doctor` no la da por puesta:
// comprueba en cada instancia viva de perfil que siga ahí (DesktopIssueUpdaterOn
// → hallazgo `instance_updater_on`), de modo que una ventana arrancada por un
// ccp viejo, o por un camino que se nos escape, se ve en vez de pasar
// desapercibida.
//
// `default` queda FUERA a propósito y esto no es un descuido: esa instancia ES
// el Claude del usuario, en su ubicación de siempre. Apagarle las
// actualizaciones sería secuestrárselo — justo el tipo de daño colateral que
// este fichero existe para evitar.

// DesktopDisableUpdateVar es la variable que apaga el updater de Squirrel en una
// instancia concreta. Exportada porque el diagnóstico (`ccp desktop doctor`) la
// busca en el entorno de los procesos vivos para saber si la barrera está puesta.
const DesktopDisableUpdateVar = "DISABLE_UPDATE_CHECK"

// desktopGuardEnv son las variables que protegen a una instancia de perfil de sí
// misma. Hoy solo apaga el updater; es el sitio donde añadir cualquier otra
// barrera por instancia, y por eso devuelve un slice y no un par suelto.
//
// Devuelve nada para `default`: ver la nota de arriba.
func desktopGuardEnv(profile string) []EnvVar {
	if profile == "default" {
		return nil
	}
	return []EnvVar{{Name: DesktopDisableUpdateVar, Value: "1"}}
}

// desktopStripGuards quita del entorno heredado las variables que esta capa
// gestiona, para que solo estén puestas donde desktopGuardEnv dice.
//
// Sin esto, `default` heredaba la barrera de la terminal (basta un
// `export DISABLE_UPDATE_CHECK=1` suelto, o lanzar desde una shell que ya venía
// de una instancia de perfil) y el Claude del usuario se quedaba mudo de
// actualizaciones sin que nadie lo hubiera pedido. EnvForChild no lo cubre:
// solo limpia las CCPManagedVars, y esta no lo es ni debe serlo — no describe un
// perfil, describe una instancia.
func desktopStripGuards(env []string) []string {
	out := env[:0:0] // copia nueva: no se muta el slice de quien llama
	for _, kv := range env {
		if strings.HasPrefix(kv, DesktopDisableUpdateVar+"=") {
			continue
		}
		out = append(out, kv)
	}
	return out
}
