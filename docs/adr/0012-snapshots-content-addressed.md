# 12. Snapshots direccionados por contenido, con rutas lógicas y clases de elemento

Fecha: 2026-09-18

## Estado

Aceptada. Implementa el §8 del spec
[2026-09-18-config-unificada-snapshots-nube](../superpowers/specs/2026-09-18-config-unificada-snapshots-nube-design.md).
La [0011](0011-una-fuente-declarada-varias-proyecciones.md) (una fuente declarada, varias proyecciones) y la
[0013](0013-cloud-end-to-end-encryption.md) (nube cifrada de extremo a extremo) salieron con sus planes; este
formato es el que la 0013 sube.

## Contexto

`ccp backup` guarda `ccp.yaml` y los overlays en un tar.gz. No cubre:

- `~/.claude`;
- la configuración que vive dentro de los `.claude.json` (MCP incluidos);
- Desktop;
- los archivos locales de los proyectos.

Tampoco guarda historial: cada copia es un archivo suelto que alguien tiene que acordarse de hacer.

Lo que viene después pide más al formato. Para subir la configuración a la nube y traerla a otra máquina hace
falta algo que se pueda sincronizar por partes y que no dependa de las rutas de una máquina concreta.

## Decisión

- **Un almacén direccionado por contenido** en `~/.config/ccp/snapshots`:
  - Cada archivo es un blob cuyo nombre es el sha256 de su contenido (`objects/ab/cdef…`).
  - Cada snapshot es un manifiesto que los lista (`snaps/<fecha-ns>-<id>.json`).
  - Dos snapshots que comparten un archivo lo guardan una sola vez, así que un snapshot diario cuesta casi
    nada.
  - El id del manifiesto es el hash de su contenido canónico, sin `id`, `label` ni `pinned`. Un manifiesto
    alterado no carga, y fijar o etiquetar un snapshot no le cambia el id.
- **Rutas lógicas, no rutas de disco.** Ejemplos: `claude/settings.json`,
  `ccp/profiles/work/overlay/CLAUDE.md`, `project/<clave>/.claude/settings.local.json`.
  - El único mapa entre las dos vive en `core/snapshot_layout.go` (ida) y `core/snapshot_restore.go`
    (vuelta).
  - La vuelta valida cada ruta contra una lista cerrada de destinos, porque una ruta lógica también puede
    llegar en un `.ccpsnap` ajeno.
  - Los proyectos se identifican por su remoto de git: el mismo repo en otra ruta es el mismo proyecto.
- **Clases de elemento:**
  - `authored` se captura siempre.
  - `secret` se captura siempre, pero va sellada con `store.key` (XChaCha20-Poly1305, 0600). Ejemplos: la
    `api_key` de un proveedor, la configuración de un `.claude.json`, `claude_desktop_config.json`.
  - `state` (conversaciones, `handoffs.yaml`) solo se captura con `--with-state`.
  - Lo generado, la caché y lo atado a la máquina no se capturan nunca: se regeneran o se recrean.
- **De un `.claude.json` solo viaja la configuración**: `mcpServers`, y por proyecto `mcpServers`,
  `enabledMcpjsonServers`, `disabledMcpjsonServers` y `allowedTools`. Restaurar es fusionar esa parte en el
  archivo vivo. El resto del archivo es de la máquina y de la sesión (tokens, `machineID`, contadores) y no se
  toca.
- **Restaurar va en tres pasos:**
  1. El plan.
  2. Un snapshot del estado actual. Sin él no se escribe nada.
  3. La escritura, atómica y con `ccp.yaml` primero bajo su lock, y la regeneración de los perfiles
     afectados.

  No se borra nada que exista y no esté en el snapshot. Un `ccp.yaml` con un schema más nuevo que el binario
  aborta en el plan, antes del snapshot previo.
- **Snapshots automáticos:**
  - Uno de seguridad antes de `profile rm` y de `backup restore`. Si no se puede guardar, la operación no se
    hace.
  - Uno diario, cada 20 horas como mínimo, y solo desde comandos de gestión. Los de scripting corren en cada
    prompt y no pueden pagar una captura.
  - El diario es best-effort a propósito: el comando que se pidió no puede fallar por la copia del día.
  - `CCP_NO_AUTO_SNAPSHOT=1` apaga ambos.
- **Retención al estilo restic:** 7 días, 4 semanas y 6 meses, contando solo los periodos que tienen
  snapshots. Además se conservan siempre el último y todo lo fijado o etiquetado. La poda deja una hora de
  gracia a los blobs, por si una captura concurrente acaba de escribirlos.
- **`.ccpsnap` para mover un snapshot a mano.** Es un tar.gz con el manifiesto y sus blobs. Los secretos solo
  viajan si se piden, y entonces van sellados con una frase: Argon2id (3 pasadas, 64 MiB) y
  XChaCha20-Poly1305. La frase tiene 12 caracteres como mínimo.
- **`internal/snapshot` no importa `core`.** Lo compartirá el backend de la nube, que no sabe nada de
  `~/.claude`.

## Consecuencias

- El plan de la nube sube este mismo formato, cifrado por completo con la clave de la bóveda. No hay un
  segundo formato que mantener.
- `ccp backup` sigue existiendo tal cual. Unificarlo con `snapshot export` es una decisión aparte.
- El almacén crece con cada cambio. `ccp snapshot prune` lo mantiene acotado, y lo fijado nunca se borra.
- El snapshot diario falla en silencio por diseño. Avisar en `ccp doctor` cuando el último tiene demasiados
  días queda pendiente.
- Una restauración en otra máquina con otro `HOME`, o con un proyecto en otra ruta, necesita traducir rutas
  (spec §11). Llega con la nube. Hasta entonces, un proyecto cuya carpeta no existe se omite con motivo
  `project_missing`.
