# 10. Llevar una sesión de Desktop a otro perfil: copiar el transcript y que Desktop la importe

Fecha: 2026-09-18

## Estado

Aceptada. Se apoya en [0008](0008-desktop-launcher-two-layer-bundle.md) (cada perfil tiene su lanzador y su
identidad) y respeta las reglas de [0009](0009-desktop-identity-is-not-durable.md) (la identidad no se da por
hecha: se comprueba, y una comprobación que no se puede hacer cuenta como «no»).

## Contexto

El 2026-09-18 un usuario tenía una conversación larga en la pestaña Code de la ventana de un perfil (`e-cc`) y
quería seguirla en su Claude principal. Cada ventana tiene sus propias sesiones, así que no había forma de
verla desde la otra. Se movió a mano, y en el proceso se midieron los hechos que dan forma a esta decisión.

Una sesión de la pestaña Code son **dos cosas en dos sitios**:

1. **El transcript**, `<cc-home>/projects/<slug>/<uuid>.jsonl`, con el mismo formato que el CLI: la pestaña
   Code es Claude Code. Es la conversación, lo único irrecuperable. Junto a él puede haber una carpeta
   `<uuid>/` con los transcripts de subagentes y el estado de los workflows.
2. **La entrada del índice de Desktop**,
   `<data-dir>/claude-code-sessions/<cuenta>/<org>/local_<id>.json`, con el título, la carpeta y el
   `cliSessionId` que apunta a (1). Es lo que pinta la barra lateral.

Copiar solo (1) deja la conversación disponible para `claude --resume`, pero no en la ventana. Escribir (2) a
mano es un formato interno y sin documentar, y escribirlo con la app abierta es pelearse con el estado que
tiene en memoria.

### Lo que se midió en `app.asar` (2.2553.1; el enlace existe también en 2.110.1)

- **Desktop tiene un enlace de importación**: `claude://resume?session=<uuid>` llama a `importCliSession`, que
  busca `<uuid>.jsonl` en cualquier `<config-dir>/projects/*/`, saca la carpeta de trabajo del propio
  transcript, crea la entrada del índice (`local_<uuid>`, `adoptedFromOtherSurface: true`) y abre la sesión.
  Es idempotente: si ya existe `local_<uuid>`, la desarchiva y la abre.
- **El título lo lee solo de los últimos 256 KiB** (`vji=262144`): la última línea `custom-title` de esa
  ventana. La primera sesión movida (1,4 MB, titulada en la línea 22) se importó **sin nombre**.
- **Rechaza un transcript con más de un hard link** (`onMultiLink: refuse` en modo `adopt`). El truco que
  abarata el espejo de los lanzadores rompería aquí la importación.
- **El enlace le llega a la app a la que se le manda.** `open -a <lanzador> 'claude://…'` lo entrega a la
  ventana de ese perfil aunque el lanzador no declare el esquema (medido con una ruta que el manejador no
  reconoce, `claude://code/<sonda>`, sin efectos: el log registra `unrecognized code path`). Un `open` sin
  `-a` se lo daría a quien tenga registrado el esquema, que es siempre el Claude principal.
- **Tras la importación, Desktop escribe en el transcript**: `custom-title`, `mode`, `atis-latch`,
  `last-prompt`… con solo abrir la sesión, y la conversación sigue creciendo ahí. El destino deja de ser una
  copia byte a byte en cuanto se usa.

## Decisión

`ccp desktop copy <uuid|título> <perfil>` hace la parte que le toca a ccp —copiar la conversación— y deja la
otra a Desktop:

1. **Localiza la sesión** leyendo el índice de cada ventana (de todas sus cuentas), sin escribirlo nunca. Sin
   `--from` busca en todas menos la del destino. Casa por uuid, prefijo del uuid, título exacto o trozo del
   título, y se queda con el primer nivel que dé algo; varios resultados son una ambigüedad que decide el
   usuario, nunca ccp.
2. **Copia** el transcript y su carpeta hermana al cc-home del destino, con el mismo proyecto y el mismo uuid,
   como archivo propio (`0600`, sin hard links) por tmp+rename. Si el título no está en los últimos 256 KiB,
   añade al final una línea `custom-title`, el mismo tipo de línea que escribe el propio Claude Code al
   renombrar.
3. **Decide sin pisar nada**, comparando bytes: el destino no la tiene → se copia; idéntica → nada; el destino
   es el comienzo de lo que se escribiría, o una copia anterior con metadatos detrás (líneas sin `uuid`) → se
   pone al día; el destino ya contiene todo el origen → no se toca; cualquier otra cosa → divergencia, error
   y nada escrito. La puesta al día se niega si la ventana destino está abierta y ya lista la sesión: podría
   tenerla cargada y seguiría desde la versión vieja.
4. **Pide la importación** con `open -a` a la app concreta —el lanzador del perfil o la Claude.app principal
   para `default`—, solo después de comprobar con `ps` y `lsappinfo` (inyectados; `DesktopImportRoute` es
   puro) que cada identidad está donde debe. No se manda si el perfil no tiene lanzador, si su ventana está
   registrada con el id del Claude principal, si otra ventana ocupa ese id, si la ventana corre sin su
   `CLAUDE_CONFIG_DIR` o si una sonda no responde. Tampoco si la ventana ya lista la sesión: dos entradas
   sobre un transcript son dos escritores.
5. **Confirma** leyendo el índice del destino hasta que aparece la entrada (20 s con la ventana abierta, 90 s
   si el enlace tiene que arrancarla). El código de salida de `open` solo dice que el enlace se entregó.

## Consecuencias

- La conversación sale en la barra lateral del destino con su título sin que ccp toque un formato interno:
  si Desktop cambia su índice, lo que se degrada es la lectura de títulos, no los datos del usuario.
- ccp depende de un enlace sin documentar. Si desaparece, la copia sigue funcionando y el comando dice cómo
  seguir la conversación en la terminal; la importación falla sin confirmar, y lo dice.
- El mismo uuid vive en dos perfiles. Es lo que permite la vuelta (copiar de nuevo hacia el origen pone al
  día su transcript y su entrada ya existente la muestra), y es por lo que la búsqueda excluye el destino y
  admite `--from`.
- Las dos copias pueden divergir si se escribe en las dos. ccp no intenta fusionarlas: lo detecta y no
  escribe. Importar la versión divergente como sesión aparte (uuid nuevo) queda fuera por ahora.
- La importación es solo para macOS: es lo único que sabe dirigir un enlace a una ventana concreta.
- `ccp handoff` sigue copiando solo el `.jsonl`, sin la carpeta hermana. Unificar las dos copias es trabajo
  aparte: el handoff tiene su vuelta con uuid nuevo (`RewriteSession`), que también tendría que renombrar esa
  carpeta.
