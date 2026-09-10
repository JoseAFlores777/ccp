# Lanzadores de Claude Desktop con nombre e icono propios — diseño

Fecha: 2026-09-09 · Estado: implementado (`ccp desktop app`) · ADR: [0008](../../adr/0008-desktop-launcher-two-layer-bundle.md)

## Problema

`ccp desktop open <perfil>` abre instancias aisladas de Claude Desktop (cuenta + Code tab), pero
todas son el mismo `Claude.app`: el Dock, Cmd-Tab y Spotlight enseñan iconos «Claude» idénticos.
Se quiere:

1. un **nombre** y un **color de icono** por perfil, visibles en el Dock/Cmd-Tab/Spotlight;
2. que la **cuenta se conserve** (nada de volver a loguearse);
3. que las **actualizaciones** de Claude.app sigan llegando a esas instancias;
4. que un clic en el Dock o en Spotlight abra **la instancia correcta**, igual que `ccp desktop open`.

## Restricciones descubiertas

Todas verificadas en la máquina del autor (macOS 26.6.2, Claude 1.49585.0, Electron con
`Squirrel.framework`), con lanzadores desechables y un clon APFS de un perfil logueado:

| Hecho | Consecuencia |
|---|---|
| LaunchServices identifica el proceso por el bundle más interno del ejecutable **según la ruta de exec** (symlinks sin resolver). | La identidad del Dock la decide desde dónde se hace `exec`, no el archivo real. |
| El Keychain («Claude Safe Storage») valida el proceso por su ruta **real**; exige bundle byte-idéntico a uno firmado **y** que el ejecutable en marcha sea su `CFBundleExecutable`. | Ni copiar+parchear el plist, ni renombrar el stub: `errSecAuthFailed` y sesión perdida. |
| Los auxiliares de Chromium (GPU/red/renderer) corren en sandbox con `BUNDLE_PATH` = bundle principal (ruta sin resolver). | Lo que esté fuera de ese árbol (symlinks a `/Applications`) es invisible: «Unable to find helper app», exit 5. |
| Electron valida `app.asar` contra `ElectronAsarIntegrity` del bundle principal, por ruta relativa a `Contents/`. | El plist externo necesita la clave con la ruta del espejo anidado. |
| LS no lanza ejecutables symlinkeados; a un script lo lanza bajo Rosetta. | El ejecutable del lanzador es un Mach-O real: hard link (o copia) de ccp. |
| ShipIt aborta con otras instancias en marcha, rechaza rutas con symlinks y empareja la descarga por el bundle id de la app en marcha. | Con id propio, la instancia lanzada no puede actualizarse a sí misma; actualiza la normal. |

## Diseño

```
~/Applications/Claude (work).app/
  Contents/Info.plist              copia parcheada: id propio, nombre, icono, sin claude://,
                                   ElectronAsarIntegrity con la clave del espejo
  Contents/PkgInfo                 APPL????
  Contents/MacOS/Claude            hard link al binario de ccp  (CFBundleExecutable)
  Contents/MacOS/Claude-run        -> ../ccp/Claude/Contents/MacOS/Claude
  Contents/Resources/ccp-icon.icns icono de Claude.app tintado
  Contents/ccp/Claude/…            espejo prístino de /Applications/Claude.app (hard links)
  Contents/ccp-desktop.json        manifiesto
```

**Arranque.** LS ejecuta `Contents/MacOS/Claude` (ccp). `cmd/ccp/main.go` llama a
`cli.DesktopLauncherApp()` antes de nada: si `os.Executable()` (sin resolver) es un
`Contents/MacOS/Claude` junto a un manifiesto, corre `RunDesktopLauncher`, que:

1. lee el manifiesto (perfil, `home` de ccp, app de origen, versión, color…);
2. carga la config (`CCP_HOME` → `manifest.home` → `~/.config/ccp`) y comprueba elegibilidad;
3. si `DesktopAppStale` y la instancia **no** está en marcha (`ps`), reconstruye el lanzador con el
   binario instalado (el del manifiesto, no el hard link que está ejecutándose);
4. espeja el cc-home (`MirrorForDesktop`), crea el user-data-dir;
5. `PlanDesktopLauncher`: entorno = `EnvForChild` sin variables gestionadas vacías; argv =
   `Claude-run --user-data-dir=<dir> <args de LS>`; `syscall.Exec`.

Los fallos se enseñan con un `display alert` de osascript cuando no hay terminal (el clic en el
Dock no tiene stderr).

**Construcción** (`core.BuildDesktopApp`): lee el plist real (`CFBundleExecutable`,
`CFBundleVersion`, `CFBundleIconFile`, `ElectronAsarIntegrity`), decide etiqueta y color
(explícitos > los del lanzador existente > `Claude (<perfil>)` y el primer color de la paleta libre),
monta todo en un temporal hermano (mismo volumen, por los hard links), lo intercambia por el anterior
y registra el bundle en LS (`lsregister -f`). Un `.app` sin manifiesto en la ruta destino nunca se toca.

**Obsolescencia** (`core.DesktopAppStale`): versión del origen ≠ manifiesto; sello (tamaño+mtime) del
binario de ccp ≠ manifiesto; falta el stub del espejo, el ejecutable o el icono.

**Icono** (`core.TintICNS`): trozos PNG del `.icns` (`ic07`+); se calibra el matiz dominante del
área saturada y se rota hasta el destino conservando saturación, luminosidad y alfa; `gray`
desatura; `orange` devuelve el original; `#rrggbb` toma matiz y saturación del hex. Los trozos no
PNG (ARGB, TOC, info) se descartan. Paleta en orden de asignación: blue, green, purple, pink, teal,
yellow, red, gray, orange.

**Plist** (`core/plist.go`): lector/escritor XML con orden de claves, Go puro; `bplist` → `plutil`
en macOS.

## CLI

```
ccp desktop app [<perfil>…] [--color <c>] [--label <nombre>] [--app <ruta>] [--force] [--dry-run]
ccp desktop app rm <perfil> | app list [--json]
ccp desktop open <perfil> [--plain] [-- <args>]     # con lanzador: `open -a <lanzador>`
ccp desktop list [--json]                           # + app/color por perfil
ccp desktop rm <perfil> --yes                       # también borra el lanzador
```

`CCP_DESKTOP_APPS_DIR` cambia `~/Applications` (y es lo que usa el CI, que corre en Linux).

## Límites

- Solo macOS y solo perfiles `official`; `default` no tiene lanzador.
- Un lanzador no reclama `claude://`: el login de un perfil nuevo se hace con `--plain` y las demás
  ventanas cerradas. Los perfiles ya logueados no necesitan nada.
- TCC concede permisos por bundle id: un lanzador puede volver a pedirlos una vez.
- `du` cuenta el espejo (~850 MB) aunque no ocupe disco extra.

## Verificación

- Unit: `plist_test.go` (round-trip, orden), `icns_test.go` (matiz, grises, alfa, hex, identidad),
  `desktop_app_test.go` (dos capas, hard links, plist parcheado, idempotencia, refresco por versión y
  por binario, cambio de etiqueta, colores, guardas de borrado, plan del lanzador),
  `cli/desktop_app_test.go` (ciclo de vida desde la CLI, `open` por lanzador y `--plain`, `list --json`).
- E2E en la máquina del autor: lanzador desde terminal y desde LS (`ccp desktop open`): 7 auxiliares
  vivos, user-data-dir poblado, `lsappinfo` con el bundle, nombre e id del lanzador, entorno del
  perfil inyectado, cero fallos de Keychain; sobre un clon del perfil `a-cc`, «claude.ai account
  active and logged in». Reabrir con la instancia en marcha la activa en vez de duplicarla.
