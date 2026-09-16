# 9. La identidad de un lanzador no es duradera: detectarla en vez de apostar por ella

Fecha: 2026-09-16

## Estado

Aceptada. Enmienda a [0008](0008-desktop-launcher-two-layer-bundle.md), que sigue vigente en su decisión
(el bundle de dos capas se mantiene) y queda corregida en dos de sus hechos.

## Contexto

El 2026-09-15 un usuario con tres perfiles pasó una tarde entera creyendo que había perdido 40 sesiones de
trabajo. No había perdido nada: estaba mirando una instancia vacía mientras otra ocupaba la identidad de su
Claude principal. Tres cosas fallaron a la vez, y **dos estaban documentadas en el 0008 como imposibles**.

### Lo que pasó, en orden

1. Abrió `ccp desktop open personal-cc`. Ese camino lanza `/Applications/Claude.app` directamente, así que
   la ventana comparte bundle id con el Claude del usuario **por construcción**, sin necesidad de ningún
   fallo previo.
2. Desde entonces, abrir Claude por el Dock o con `open -a` **activaba esa ventana** en lugar de lanzar la
   suya. Estuvo horas sin poder llegar a su Claude principal. Solo `open -n -a` lo desbloqueó.
3. Una instancia de perfil **actualizó `/Applications/Claude.app`** de 1.52386.3 a 2.110.0.
   `~/Library/Caches/com.anthropic.claudefordesktop.ShipIt/ShipIt_stderr.log` lo registra sin ambigüedad:
   «Beginning installation» → «Moving bundle from file:///Applications/Claude.app/ …» → «Installation
   completed successfully». Es decir: actualizó la app principal del usuario desde una ventana que él había
   abierto para otra cuenta, y dejó su propio espejo en la versión vieja con los hard links huérfanos.
4. Al reloguearse en la ventana equivocada, Chromium purgó el IndexedDB de ese data dir (de ~900 KB a 20 KB)
   y el panel salió vacío. Nada en la pantalla podía decirle que sus sesiones seguían en disco, indexadas
   bajo la otra cuenta.

### Los dos hechos del 0008 que hay que corregir

**El hecho 1 no es falso: es no garantizado.** macOS mantiene *dos* rutas para el mismo proceso, y cada
subsistema usa una:

| Ruta | Quién la usa | Medido con |
|---|---|---|
| La pasada a `execve`, **sin resolver** | `_CFProcessPath`, `[NSBundle mainBundle]`, el check-in de LaunchServices | `ps` imprime el symlink |
| El vnode **real**, resuelto | `proc_pidpath`, Security.framework (Keychain), TCC | `lsof txt` imprime el destino |

El bundle de dos capas vive exactamente en ese hueco, y por eso funciona: en el arranque inicial el proceso
*sí* registra el id propio del lanzador. Pero esa identidad vale **solo mientras el proceso sea el que el
kernel ejecutó a través de `Claude-run`**. `process.execPath` de Electron sale de `uv_exepath`, que llama a
`realpath()`, así que cualquier re-exec —incluido el `app.relaunch()` que la propia app usa al actualizarse—
reencarna el proceso como `com.anthropic.claudefordesktop` ubicado en `Contents/ccp/Claude`. Y no vuelve.

**El hecho 6 es falso como garantía.** SQRLUpdater no compara contra el id del bundle en disco, sino contra
`NSRunningApplication.currentApplication.bundleIdentifier`, y su `targetBundleURL` sale de
`currentApplication.bundleURL` resuelto. Tras el colapso de identidad, ese id **es** el de Claude, así que
el updater pasa su propio chequeo y actualiza lo que LaunchServices tenga registrado bajo ese id: la app del
usuario. El camino `desktop open` ni siquiera necesita el colapso — ahí el ejecutable ya es el de la app real.

### Las tres palancas para recuperar la identidad están cerradas

Se midieron las tres, no se descartaron por intuición:

1. **Hard link en vez de symlink para `Claude-run`.** Mueve `proc_pidpath` a la capa externa, que no es
   prístina, y con ella muere la ACL de «Claude Safe Storage» (hecho 2 del 0008, que sigue en pie). Y además
   no es determinista: con hard links el kernel reporta *uno* de los nombres del inodo —el que tenga cacheado
   el vnode—, no el que se usó. Medido: un proceso lanzado por el symlink reportó el nombre del hard link.
2. **`CFProcessPath` como variable de entorno.** Funciona con un binario ad-hoc, pero macOS la ignora bajo
   hardened runtime (`CodeDirectory flags=0x10000`), que es como viene firmado Claude.
3. **`LSMultipleInstancesProhibited`.** Habla de sesiones de usuario, no de instancias dentro de una sesión,
   y no cambia a qué proceso activa `open -a`.

## Decisión

**El bundle de dos capas se mantiene** —sigue siendo la única forma de tener nombre e icono propios sin
perder la sesión, y la alternativa de retirarlo es literalmente el camino que causó el daño—, pero ccp deja
de dar la identidad por hecho. Cuatro cambios:

1. **Ninguna instancia de perfil puede actualizar nada.** Toda instancia distinta de `default` arranca con
   `DISABLE_UPDATE_CHECK=1`, por los dos caminos (`PlanDesktop` y `PlanDesktopLauncher`). La bandera no está
   documentada por Anthropic, pero está en el binario: la cadena aparece dos veces en
   `Squirrel.framework/Versions/A/Squirrel`, junto a `SQRLUpdater.m`, y `SQRLUpdater` la lee con `getenv()`
   una sola vez al inicializarse. Como no está documentada, no se da por puesta: `ccp desktop doctor`
   comprueba en cada instancia viva que la barrera siga ahí (`instance_updater_on`).

   **`default` queda exento a propósito.** Esa instancia *es* el Claude del usuario; apagarle las
   actualizaciones sería secuestrárselo, que es la misma clase de daño colateral que este ADR evita.

2. **El `-n` deja de ser incondicional.** `default` no tiene data dir propio, así que un `-n` ahí abría un
   segundo proceso Chromium sobre el data dir real del usuario en lugar de traer al frente la ventana que ya
   existe. Ahora se fuerza instancia nueva **solo** cuando alguien está ocupando la identidad del bundle que
   se va a abrir: un proceso que corre *ese mismo* ejecutable con otro `--user-data-dir`. Una instancia
   lanzada por su lanzador tiene id propio y no cuenta — contarla reintroducía el fallo por la puerta de
   atrás.

3. **El aislamiento deja de depender de desde dónde se lanzó.** `open` hereda el entorno de quien lo invoca y
   `--env` solo sobrescribe lo que nombra, así que lanzar un perfil *official* desde una terminal con un
   perfil deepseek activo metía su `ANTHROPIC_BASE_URL` viva en la ventana: el Code tab hablando con otro
   proveedor bajo la cuenta de Anthropic. El entorno del hijo se fija entero (`DesktopPlan.CleanEnv`, vía
   `EnvForChild`), también en darwin.

4. **`ccp desktop doctor`**: lo que hacía falta aquel día y no existía. Motor puro (`core/desktop_audit.go`)
   con las sondas del sistema inyectadas, bilingüe, con `--json`. Diagnostica y **nunca repara**: borrar
   lanzadores o reconstruir bundles desde un diagnóstico lo convertiría en una segunda fuente de pérdida.

   Regla que no se negocia: **una sonda que no se puede ejecutar produce `unknown`, jamás `ok`.** Un doctor
   que dice «todo bien» porque no pudo mirar convierte una duda en una falsa certeza, que es peor que no
   tener doctor.

   El hallazgo que justifica el comando por sí solo es `instance_multi_account`: dice, con números, que las
   sesiones que no se ven **no se han borrado**, porque están indexadas por `<cuenta>/<org>` y vuelven al
   entrar con esa cuenta.

Como corolario, `desktop open` **construye el lanzador si falta** en vez de caer al camino directo: sin
lanzador la ventana corre desde `/Applications/Claude.app` y macOS no puede distinguirla del Claude del
usuario. Cuando se pide `--plain` explícitamente, se dice lo que eso implica.

## Consecuencias

- **La identidad sigue sin estar garantizada, y eso ahora está escrito.** Tras un re-exec, la ventana de un
  perfil puede volver a aparecer bajo el id de Claude. La diferencia es que ccp lo detecta
  (`launcher_identity_collapsed`) y el usuario tiene una salida que antes tuvo que descubrir a mano:
  `open -n -a /Applications/Claude.app`.
- **El espejo puede quedarse desfasado sin cambiar de versión.** ShipIt no parchea el bundle, lo reemplaza
  entero con un `move`, así que los hard links quedan apuntando a inodos huérfanos y el lanzador retiene una
  copia completa de la versión vieja. `DesktopAppStale` compara además inodos (`os.SameFile`), condicionado a
  que fuente y espejo estén en el mismo dispositivo: si no lo están, `hardlinkTree` cayó a copia por
  construcción y no hay nada que reconstruir.
- **Dependemos de una bandera no documentada.** Si una versión futura de Claude deja de respetar
  `DISABLE_UPDATE_CHECK`, la barrera cae en silencio. Por eso el doctor la verifica en los procesos vivos en
  vez de asumirla, y por eso este ADR deja escrito dónde mirar.
- **TCC y el llavero siguen compartidos** entre lanzadores y app: se conceden por bundle id y firma, así que
  un permiso dado desde una ventana vale para todas y en Ajustes aparece una sola entrada «Claude». Es el
  empate central del diseño: darle identidad propia al espejo rompería su firma y con ella la sesión.
- **El sensor lee el entorno de procesos ajenos** (`ps -Ewww`). Solo se extraen cuatro variables
  (`CLAUDE_CONFIG_DIR`, `CLAUDE_USER_DATA_DIR`, `CCP_PROFILE`, `DISABLE_UPDATE_CHECK`) y ninguna es un
  secreto: por ahí viajan tokens de otras apps, y no tienen por qué acabar en un diagnóstico.
- **Cuidado con los fixtures sintéticos.** Los primeros tests del sensor usaban entornos inventados, todo en
  mayúsculas, y escondieron dos bugs: el entorno que macOS inyecta a toda app GUI empieza por
  `OSLogRateLimit=64` (minúsculas, y el `--user-data-dir` es el último argumento, así que el parser se tragaba
  medio entorno) y `ps` imprime `Claude-run`, no la ruta resuelta del espejo. Los tests van contra una salida
  real de `ps` desde entonces.
