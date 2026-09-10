# 8. Lanzador de Claude Desktop en dos capas: identidad de LaunchServices fuera, bundle prístino dentro

Fecha: 2026-09-09

## Estado

Aceptada

## Contexto

`ccp desktop open` aísla la cuenta (`--user-data-dir`) y el Code tab (`CLAUDE_CONFIG_DIR`), pero todas las ventanas siguen siendo el mismo `/Applications/Claude.app`: el Dock, Cmd-Tab y Spotlight enseñan N iconos «Claude» idénticos y no hay forma de saber cuál es cuál. Se pide un nombre y un color de icono por perfil, **sin perder la sesión** y **sin quedarse fuera de las actualizaciones** de Claude.app.

Hechos establecidos por experimento en macOS 26.6.2 con Claude 1.49585.0 (cada uno tumba una alternativa):

1. **LaunchServices identifica un proceso por el bundle más interno que contiene su ejecutable según la ruta con la que se hizo `exec`, sin resolver symlinks.** De ahí salen el nombre, el icono y el bundle id que enseña el Dock.
2. **La ACL del Keychain sobre «Claude Safe Storage»** (la clave con la que Chromium cifra cookies y sesión) valida el proceso por su ruta **real** (`proc_pidpath`), y solo valida si el bundle que lo contiene es byte a byte uno firmado **y** el ejecutable en marcha es su `CFBundleExecutable`. Un `Info.plist` tocado, o un stub renombrado bajo un plist intacto, dan `errSecAuthFailed` sin prompt: la app arranca deslogueada y no puede persistir un login nuevo.
3. **Los auxiliares de Chromium (GPU, red, renderers) corren en un sandbox cuyo `BUNDLE_PATH` es el bundle principal** —el de `[NSBundle mainBundle]`, o sea la ruta sin resolver—; todo lo que esté fuera de ese subpath les es invisible. Con symlinks hacia `/Applications` mueren con «Unable to find helper app» (exit 5) y la app se cierra.
4. **Electron valida `app.asar` contra `ElectronAsarIntegrity` del bundle principal**, con la clave relativa a su `Contents/`. Si la clave no existe, `FATAL: Failed to get integrity for validatable asar archive`.
5. **A un ejecutable symlinkeado LaunchServices no lo lanza** (`open` devuelve 0 y no arranca nada), y a un script lo lanza bajo Rosetta (no puede leer la arquitectura), donde Chromium no levanta auxiliares.
6. **Squirrel (ShipIt)** rechaza destinos cuya ruta atraviesa symlinks, aborta si hay otras instancias de la app en marcha, y busca en la descarga un bundle con el id de la app **en marcha**.

Alternativas consideradas:

1. **Copiar Claude.app y retocar su `Info.plist`** (nombre, icono, id). Pierde la cuenta (hecho 2). Re-firmar ad hoc tampoco: los entitlements restringidos (`virtualization`, `keychain-access-groups`) exigen el perfil de aprovisionamiento de Anthropic.
2. **Bundle propio con symlinks** a Frameworks/Resources/MacOS de la app real. El proceso principal arranca con la identidad propia, pero los auxiliares mueren (hecho 3).
3. **Espejo prístino de hard links en `~/Applications`**, identidad por nombre de directorio e icono personalizado de Finder. Conserva la sesión, pero un clic en el Dock lanza el stub sin argumentos —no hay forma de meter `--user-data-dir` ni entorno en un bundle intacto— y abre la cuenta por defecto con el nombre del perfil. Peor que no tener lanzador.
4. **Bundle de dos capas**: la externa con la identidad y un ejecutable nuestro; la interna, un espejo prístino que es lo que realmente se ejecuta.

## Decisión

Tomamos la **4**. `Claude (<perfil>).app` es:

- **Capa externa** (lo que ve LaunchServices): copia parcheada del `Info.plist` de Claude.app —`CFBundleIdentifier` propio (`com.anthropic.claudefordesktop.ccp.<perfil>`), `CFBundleName`/`DisplayName` = la etiqueta, `CFBundleIconFile` = el `.icns` de Claude tintado, sin `CFBundleIconName` (en macOS 26 el de `Assets.car` ganaría) y **sin `CFBundleURLTypes`**—, `PkgInfo`, y como `CFBundleExecutable` un **hard link al binario de ccp** (copia si el volumen no permite links; nunca symlink ni script, hecho 5).
- **Capa interna** (`Contents/ccp/Claude`): espejo de `/Applications/Claude.app` con directorios reales, archivos como hard links y symlinks recreados. Intacto: mismo `Info.plist`, mismo `CFBundleExecutable`, mismos bytes.
- **El puente**: `Contents/MacOS/Claude-run`, symlink al stub del espejo. ccp, al arrancar como lanzador (`IsDesktopLauncher`: se ejecuta como `Contents/MacOS/Claude` junto a un manifiesto `ccp-desktop.json`), calcula el entorno del perfil (`EnvForChild`) y `--user-data-dir`, y hace `syscall.Exec` de `Claude-run`.

Cada pieza del sistema mira entonces la capa que le conviene: LaunchServices ve `…/Contents/MacOS/Claude-run` en la capa externa (hecho 1) → nombre, icono e id propios; Security resuelve el symlink y valida el bundle interno intacto (hecho 2) → la sesión se conserva; el sandbox cubre el espejo porque está **dentro** del bundle principal (hecho 3); el plist externo lleva la entrada de integridad duplicada bajo `ccp/Claude/Contents/Resources/app.asar` (hecho 4).

Actualizaciones: los hard links fijan los inodos de la versión con la que se construyó el espejo, así que actualizar Claude.app nunca rompe un lanzador. `DesktopAppStale` (versión de origen, sello del binario de ccp, presencia del espejo y del icono) hace que el siguiente arranque —desde el Dock o desde `ccp desktop open`— lo reconstruya, nunca con su instancia en marcha. La instancia no puede actualizarse a sí misma (hecho 6: id distinto); actualiza la instancia normal, como siempre, y los lanzadores la siguen.

El manifiesto vive en el bundle, no en `ccp.yaml`: es estado derivado que se regenera entero, y un ccp anterior no lo pierde al reescribir la config.

## Consecuencias

- **Solo macOS**, y solo perfiles `official`: es un bundle de app, y `default` no tiene lanzador (su instancia es la app normal).
- **`claude://` sigue siendo de Claude.app.** Un lanzador no reclama el esquema, así que el callback del login siempre llega a la instancia normal: un perfil **nuevo** se loguea con `ccp desktop open <perfil> --plain` y las demás ventanas cerradas. Los perfiles ya logueados no necesitan nada.
- **Permisos de privacidad por app**: TCC concede por bundle id, así que un lanzador puede volver a pedir pantalla/archivos/micrófono una vez.
- **Disco**: cero extra por el espejo (hard links), aunque `du` lo cuente; el binario de ccp también va enlazado.
- **Acoplamiento a la forma de Claude.app**: `CFBundleExecutable`, `CFBundleIconFile`, `ElectronAsarIntegrity` se leen del plist real en cada build, no se asumen. Si Anthropic cambiara la validación del Keychain o el sandbox, el lugar donde mirar es `desktop_app.go` y este ADR.
- **`plist.go` e `icns.go`** son implementaciones mínimas en Go puro (plist XML con orden; icns por trozos PNG) para que el parcheo y el tintado sean testables en el CI de Linux. Un plist binario pasa por `plutil` en macOS.
