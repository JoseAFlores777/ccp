# 18. El espejo del lanzador vive fuera de `Contents/`

Fecha: 2026-10-28

## Estado

Aceptada · enmienda el hecho 4 y la capa interna de [0008](0008-desktop-launcher-two-layer-bundle.md).

## Contexto

El 2026-10-28 `ccp desktop open a-cc` decía «ok» y la ventana no aparecía. Claude.app se había actualizado a
4.29.0 (Electron 44.9.3, Chromium 152), y el lanzador de cada perfil moría en menos de un segundo con
`EXC_BREAKPOINT`. Lanzándolo desde una terminal, el motivo:

```
FATAL:electron/shell/common/asar/archive.cc:256] Failed to get integrity for validatable asar archive: ccp/Claude/Contents/Resources/app.asar
```

La entrada existía en el `Info.plist` del lanzador: era la que el 0008 duplicaba a propósito (hecho 4). Lo que
cambió está en `shell/common/asar/integrity_digest.mm` de Electron 44: el framework firmado lleva, en la
sección `__DATA_CONST,__asar_integrity`, una huella SHA-256 del diccionario `ElectronAsarIntegrity` **entero**
(cada clave, su `algorithm` y su `hash`, en orden). `HeaderIntegrity()` la comprueba antes de buscar la clave,
y si no cuadra da el diccionario por inexistente. Medido sobre el framework instalado:

- huella embebida (`used=1`, `version=1`): `57576bf6…`
- huella del diccionario original de Claude.app: `57576bf6…` — cuadra;
- huella del diccionario del lanzador, con las dos entradas `ccp/Claude/…` añadidas: `fe3b7806…` — no cuadra.

Quitar las entradas no basta. `RelativePath()` calcula la clave como la ruta del `.asar` relativa al
`Contents/` del bundle principal, y el `.asar` que se carga sale de la ruta **resuelta** del ejecutable (el
espejo): con el espejo en `Contents/ccp/Claude` la clave es `ccp/Claude/Contents/Resources/app.asar`, que el
diccionario original no tiene. El framework no se puede tocar (va firmado y su firma es la que valida el
Keychain, hecho 2 del 0008). Así que el espejo dentro de `Contents/` no tiene arreglo.

Lo que sí da `archive.cc`: la validación solo entra si `RelativePath()` existe, es decir, si el `.asar` está
dentro del `Contents/` del bundle principal. Uno que esté fuera se abre sin validar.

## Decisión

El espejo pasa de `Contents/ccp/Claude` a **`ccp/Claude`, en la raíz del `.app`**, y `Claude-run` apunta a
`../../ccp/Claude/Contents/MacOS/Claude`. El `Info.plist` externo conserva `ElectronAsarIntegrity` **tal cual**
viene de Claude.app: ni una entrada de más.

Cada hecho del 0008 sigue cumpliéndose, y se comprobó con un bundle de prueba antes de cambiar el código:

- **Identidad (hecho 1)**: el exec sigue siendo `Contents/MacOS/Claude-run`; `lsappinfo` lo dio con el id y el
  nombre propios del lanzador.
- **Keychain (hecho 2)**: el ejecutable real sigue en un bundle intacto de hard links; solo cambió dónde.
- **Sandbox (hecho 3)**: el espejo sigue **dentro** del bundle principal, aunque fuera de `Contents/`; GPU, red,
  audio y renderers arrancaron y siguieron vivos.
- **Integridad (hecho 4, enmendado)**: el `.asar` del espejo queda fuera de `Contents/` y no se valida; el
  diccionario del plist es el original y la huella cuadra. Lo que no se valida son hard links de los `.asar`
  firmados, byte a byte los mismos que la app normal sí valida.

Un lanzador de la forma vieja se reconoce y se reconstruye: `DesktopAppStale` lo da por obsoleto aunque la
versión coincida, y `ccp desktop doctor` lo dice como Error (`launcher_layout_legacy`). Quien reconoce una
ventana **viva** —preflight, `desktop copy`— acepta las dos rutas del espejo, porque una ventana abierta
desde un lanzador viejo sigue usando la ruta vieja hasta que se cierra y el lanzador se reconstruye.

## Consecuencias

- **Se depende de una rama de Electron que no está documentada como contrato**: que un `.asar` fuera del
  `Contents/` principal no se valide. Si una versión futura valida también esos, el síntoma será el mismo FATAL
  con otra ruta, y el lugar donde mirar es `archive.cc` / `archive_mac.mm` de esa versión y este ADR.
- **El icono del Dock de un lanzador viejo no se arregla solo.** Ese icono ejecuta el ccp con el que se
  construyó (es un hard link suyo), y ese ccp lo reconstruiría con la forma vieja. Hay que reconstruirlo una
  vez con un ccp nuevo: `ccp desktop app <perfil> --force` o `ccp desktop open <perfil>`, con su ventana
  cerrada. Por eso el remedio del doctor nombra el comando y no el icono.
- La forma nueva arranca también con versiones de Claude anteriores a 4.29.0: sin huella, un `.asar` fuera de
  `Contents/` tampoco se validaba.
