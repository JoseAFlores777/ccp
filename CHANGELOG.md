# Changelog

## [Unreleased]

### Added

- **`ccp cloud verify`: la historia es una cadena firmada, y ahora se comprueba entera** (spec §10.3.1,
  F3-1). La firma de cada snapshot ata su id, su padre y su manifiesto desde F1, pero nadie comparaba nunca
  dos eslabones: un cliente que verifica de uno en uno —lo que hacía `pull`— sabe que *ese* snapshot es
  auténtico y jamás que falta el de al lado. `verify` baja la cadena completa y canta la falta con su
  código: `bad_signature` (no la firmó esta cuenta, o le cambiaron algo), `broken_link` (su padre no está:
  lo que deja un eslabón quitado del medio), `dropped` (esta máquina lo subió y ya no está, que es la única
  señal de que han cortado por la cabeza, donde no queda ningún padre roto que delate nada), `cycle`,
  `out_of_order` y `duplicate_id`. Sale 1 si algo no cuadra, y dice lo que hay que decir: aquí no se ha
  borrado nada, tus snapshots siguen en esta máquina.
  - **El digest lo calcula el servidor, no el cliente.** `GET /v1/snapshots/chain` sirve los eslabones con
    el sha256 del manifiesto sellado —lo único del manifiesto que entra en la firma—, así que la cadena se
    verifica de un tirón sin bajar un solo manifiesto. Lo calcula Postgres sobre el valor que escribe
    (`sha256($manifest)`, columna `manifest_sha256`, migración 0004): un digest que viniera de fuera
    comprobaría el manifiesto contra lo que dijera quien lo mandó.
  - La cadena se sirve **sin filtro por dispositivo**: las cadenas se cruzan —una máquina que baja un
    snapshot ajeno encadena el suyo encima—, y media cadena tendría huecos que no son huecos.
  - La fecha **no va firmada** y por eso no ordena nada: el orden lo dibujan los padres. Lo que caza
    `out_of_order` es una fecha que miente (el listado del portal sale de ella), no un reordenamiento de la
    historia, que es imposible sin la clave de cuenta.

- **P-21 Nube en la app: la cuenta, la bóveda, tus equipos y lo que espera tu confirmación** (spec §10.3,
  F2-5, [ADR 0014](docs/adr/0014-portal-proposes-machine-applies.md)). Es el otro extremo del portal: lo que
  éste propone acaba aquí, y lo que ejecuta código no entra hasta que alguien de esta máquina lo mira.
  - **Nada viene marcado.** Los cambios que esperan —hooks, el `command` de un MCP, la `statusLine`, plugins,
    un script, los permisos que amplían— se aprueban **ruta a ruta**, con el motivo de cada uno al lado; lo
    que no marcas se rechaza y se informa al portal. Confirmar por omisión es justo lo que esta barrera
    existe para evitar (D6). Antes de escribir se guarda un snapshot de seguridad, y la pantalla enseña su id.
  - **Un secreto de la bóveda no cruza el puente.** Iniciar sesión, crear la bóveda y desbloquearla abren
    **Terminal** con el comando, como el `/login` de una cuenta: la frase de bóveda desenvuelve la clave de
    cuenta, y el cifrado de extremo a extremo vale exactamente lo que valga el sitio por el que pasa esa
    frase. La app no tiene ningún campo donde escribirla.
  - **La política de este equipo se elige aquí** (`auto` · `manual`, `ccp cloud policy`), con la diferencia
    dicha en la pantalla: ninguna de las dos aplica sola lo ejecutable; lo que cambia es si un `CLAUDE.md` o
    una regla entran sin preguntar. Los choques se listan pero no se resuelven desde aquí, porque el portal
    es quien publica sobre el snapshot actual.
  - **`ccp serve` gana los métodos de nube**: `cloud.status`, `cloud.devices`, `cloud.review`,
    `cloud.reviewResolve`, `cloud.setPolicy` y `cloud.revoke`. `cloud.status` funciona sin sesión y sin red
    —es el estado en el que la pantalla se abre la primera vez— y nunca se cuelga. Login, `init` y `unlock`
    **no** están: no hay forma de pedirle a `serve` que maneje la frase. El equipo propio no se revoca desde
    la app: se dejaría sin nube y sin forma de arreglarlo desde ahí.
  - `cloudAgentOpts` sale de `cloudCmd` para que el CLI y `serve` monten **el mismo** agente: dos
    construcciones con distinta política o distinto almacén serían dos máquinas distintas aplicando la misma
    revisión.

- **El portal edita la configuración y la publica: «Aplicar a…»** (spec §10.3, F2-4,
  [`docs/portal.md`](docs/portal.md)). El mismo modelo de **P-20** sobre el manifiesto de un snapshot —capa
  (ccp · global · cada perfil · cada proyecto · cada ventana de Desktop), tipo (Instrucciones · MCP · Ajustes ·
  Skills · Agents · Commands · Plugins · Atajos · Claves), la ruta, **dónde aplica** (CLI · Code · Chat,
  [ADR 0016](docs/adr/0016-what-desktop-reads-from-a-profile.md)) y, en lo que no se edita desde ahí, **por
  qué**—. Al terminar, «Aplicar a…» publica lo editado como una **revisión deseada firmada** a las máquinas
  que elijas; cada una la aplica cuando su agente contacte, y lo ejecutable lo sigue confirmando una persona
  allí ([ADR 0014](docs/adr/0014-portal-proposes-machine-applies.md)).
  - **Es una edición, no una restauración.** La `base` de la revisión es el snapshot que se editó, así que la
    máquina lo usa de base de su merge a tres bandas: lo que ella cambió por su cuenta desde entonces se
    queda, y una ruta que cambió en los dos sitios sale como **conflicto** en vez de pisarse. El portal no
    restaura nada por sí mismo ni abre ninguna conexión hacia ninguna máquina.
  - **La unidad es el archivo del snapshot**, porque lo que el portal tiene delante es un manifiesto y no la
    máquina: se edita su texto, con el JSON validado, y no hay formularios por tipo como en la GUI. La
    excepción son los ajustes: un `settings.json` es un archivo con varios tipos de P-20 dentro, así que se
    abre por secciones (`permissions`, `env`, `hooks`, `statusLine`, `model`, `outputStyle`) y lo que ccp no
    reconoce viaja **entero**, porque enseñar medio archivo y luego guardarlo pierde la otra mitad.
  - **Lo que lleva claves no se pinta hasta que lo pides.** La clave de cuenta está en la pestaña, así que el
    portal *puede* enseñar un `api_key` o un `claude_desktop_config.json`; enseñarlo por haber pulsado en una
    lista es otra cosa, y basta con que alguien pase por detrás.
  - **El navegador fabrica el snapshot**: sella los blobs de lo editado, monta el manifiesto y lo firma con la
    clave de cuenta. Lo delicado es el id, porque la máquina lo **recalcula** al guardarlo, así que el JSON
    del navegador sale byte a byte como el de `json.Marshal` —con su orden de campos, sus `omitempty` y el
    escapado de `<`, `>` y `&` que `JSON.stringify` no hace—. Los vectores los genera Go, y el test de
    publicación hace luego de agente: verifica, abre y **guarda** en un `snapshot.Store` de verdad.
  - **El API sirve blobs por su mismo origen** (`GET`/`PUT /v1/blobs/{id}`), y solo por el portal: una pestaña
    no puede hablar con el bucket —su CSP solo deja salir hacia su propio origen y hacia Keycloak, y ampliarla
    no bastaría porque el bucket tendría que responder CORS—. Lo que pasa por ahí sigue sellado.
  - La demo tiene ahora contenido de verdad y valida lo que el portal sube como lo validaría el agente:
    `CCP_PORTAL_DEMO=1 go test ./internal/cloud/portal -run Demo -v`.
- **El portal web: cuenta, dispositivos y línea de tiempo** (spec §10.3, F2-3,
  [`docs/portal.md`](docs/portal.md)). Lo sirve el propio `ccp-cloud` en la raíz del mismo host que el API, y
  se despliega con él: no hay bundler, ni npm, ni paquete generado que mantener al día con el código, solo
  módulos ES empotrados con `go:embed`.
  - **La bóveda se abre en el navegador.** Entras con Keycloak (código de autorización + PKCE, cliente
    público `ccp-portal`) y el portal pide la **frase de bóveda**: la clave de cuenta se deriva en la pestaña
    con Argon2id y no sale de ahí. Vive solo en memoria y se olvida al cerrar la pestaña o tras 15 minutos sin
    tocar nada. También vale el código de recuperación.
  - **Argon2id y XChaCha20-Poly1305 van escritos a mano** (`web/js/crypto.js`): no están en ningún navegador,
    y traerlos de una dependencia —con su build y su cadena de suministro— para la única página que toca la
    clave de cuenta era peor negocio que escribirlos. Lo que WebCrypto sí trae (HKDF, SHA-256, Ed25519) sale
    de ahí. `crypto_test.mjs` los ejecuta bajo node contra vectores que genera el propio Go, así que «el
    navegador abre lo que `ccp` sella» es un test y no una esperanza.
  - **Dispositivos**: último contacto, versión de ccp, los perfiles de cada equipo y su estado frente a la
    revisión publicada, con cuántas rutas difieren. **Línea de tiempo** por máquina y **diff entre dos
    snapshots cualesquiera**, agrupado por área (ccp, global, cada perfil, cada proyecto) y filtrable. El
    diff del portal se compara en los tests contra `snapshot.Diff`: dos diffs que no coinciden sobre los
    mismos snapshots son dos verdades y nadie sabría cuál mirar.
  - **La firma tiene tres respuestas**: válida, alterada y «este navegador no sabe verificar Ed25519», que no
    es lo mismo que válida ([ADR 0009](docs/adr/0009-desktop-identity-is-not-durable.md)). Se comprueba con la
    pública **derivada de la clave de cuenta**, no con la que manda el servidor.
  - **CSP estricta y nada de terceros**: `default-src 'none'`, `script-src 'self'`, `connect-src` solo el
    propio origen y el de Keycloak. `/v1/info` gana `portal_client_id` (campo añadido, forma intacta) porque
    la pestaña lo necesita antes de tener sesión. `CCP_CLOUD_PORTAL=0` apaga el portal;
    `CCP_CLOUD_OIDC_PORTAL_CLIENT_ID` cambia el cliente.
  - Para mirarlo sin desplegar nada: `CCP_PORTAL_DEMO=1 go test ./internal/cloud/portal -run Demo -v`.
- **`ccp cloud agent`: el portal propone y esta máquina aplica** (spec §10.3,
  [ADR 0014](docs/adr/0014-portal-proposes-machine-applies.md)). El portal publica una **revisión deseada**
  firmada con la clave de cuenta —que el servidor no tiene— y dirigida a un dispositivo concreto; la máquina
  tira de ella, verifica la firma con su propia clave y decide qué escribe. No hay ningún puerto abierto hacia
  tu Mac.
  - **Reconciliación a tres bandas por ruta lógica**: base (la `base` firmada de la revisión), lo vivo aquí y
    lo deseado. Lo que solo cambió arriba se aplica; lo que cambió en los dos sitios queda como **conflicto** y
    no se toca; lo que solo cambió aquí se conserva. Una revisión sin base es una orden absoluta y se aplica
    como un restore. **Antes de escribir, snapshot automático** (el motor de `ccp snapshot restore`), y el
    agente dice con qué id se deshace.
  - **Lo que ejecuta código no se aplica solo**: hooks, `command`/`args` de un MCP, `statusLine`, plugins,
    skills con script y los permisos que **amplían** (`permissions.allow`, `defaultMode`) esperan a
    `ccp cloud review` en la propia máquina. Una cuenta robada no basta para ejecutar código en tus Macs. Se
    mira el contenido y no solo la ruta: un `settings.json` que solo cambia `model` se aplica solo, porque
    preguntar por todo enseña a decir que sí sin leer.
  - **La revisión se queda abierta mientras espera a una persona.** El resultado se informa una sola vez, así
    que cerrarla con «parcial, esperando confirmación» dejaría al portal un resultado incorregible; es
    `ccp cloud review` quien la cierra (`aplicada`, `parcial`, `en conflicto`, `fallida`).
  - `ccp cloud policy [auto|manual]` fija la política de ESTE equipo, y vive en su disco y no en la cuenta: es
    su defensa frente a la propia cuenta. `ccp cloud status` dice la política y si hay algo esperando
    confirmación.
  - **En segundo plano es opcional y lo instalas tú**: ccp no escribe en tus `LaunchAgents`. El plist está en
    [`docs/launchagent-cloud-agent.md`](docs/launchagent-cloud-agent.md).
- **`ccp cloud`: el historial de snapshots en un servidor propio, cifrado de punta a punta** (spec §10,
  [ADR 0013](docs/adr/0013-cloud-end-to-end-encryption.md) y
  [ADR 0015](docs/adr/0015-identity-keycloak-vault-separate.md)). `login` (código de dispositivo), `init`,
  `unlock`, `push`, `pull`, `list`, `devices`, `revoke`, `logout` y `status [--json]`.
  - **Todo se cifra en el equipo antes de salir.** Blobs y manifiestos van sellados con su id como dato
    asociado, los ids de la nube son un HMAC de los hashes locales —el servidor deduplica sin saber qué
    guarda— y cada snapshot va firmado con Ed25519. La firma se verifica con la clave pública **derivada de
    la clave de cuenta**, no con una que diga el servidor: un servidor comprometido puede negar el servicio,
    pero no colar, alterar ni reordenar un snapshot. Los blobs van directos al almacenamiento con URLs
    prefirmadas y no pasan por el API.
  - **Dos secretos con dos dueños**: la identidad la da Keycloak y el cifrado una **frase de bóveda** que
    nunca llega al servidor, más un **código de recuperación** que se enseña una sola vez. Perder los dos
    hace irrecuperable la copia de la nube; los snapshots locales siguen siendo la fuente primaria.
  - **Revocar un equipo** lo echa del API en su siguiente petición —el dispositivo se comprueba en cada una—,
    pero no borra la clave que ese equipo ya tiene: para eso hace falta rotar la clave de cuenta (F4).
  - Los archivos de la nube de este equipo viven en `~/.config/ccp/cloud` (0700, todos 0600).

- **El backend `ccp-cloud`**: el API `/v1` (dispositivos, bóveda, blobs prefirmados y snapshots) con
  Postgres, migraciones SQL embebidas y almacenamiento S3, en `cmd/ccp-cloud` e `internal/cloud/`. Se
  configura por entorno y guarda **solo datos opacos**. El despliegue público queda pendiente de que el
  usuario lo autorice.

- **`ccp snapshot restore` traduce el HOME de origen.** Un snapshot hecho bajo `/Users/ana` y restaurado
  donde el HOME es `/Users/jose` traía rutas que no existen. Ahora el HOME se reescribe dentro de las reglas
  de carpeta, los comandos de hooks y de MCP y la ruta de cada proyecto, y el plan lo anuncia. Las
  conversaciones se dejan intactas: sus rutas son historia, describen dónde ocurrió algo.

- **La app: P-17 evoluciona de «Copias» a Snapshots** (spec §8). La historia de toda la
  configuración en una pantalla: línea de tiempo con etiqueta, disparador, fecha, tamaño y fijados;
  el detalle de qué captura cada snapshot, agrupado por el mismo prefijo que entiende `--only`;
  diff contra otro snapshot o **contra lo que hay ahora mismo**, que es la pregunta de verdad; y
  exportar/importar `.ccpsnap` pidiendo la frase solo cuando los secretos viajan.
  - **Restaurar va en dos pasos y el primero no escribe nada**: se calcula el plan (dry-run), se
    marca qué partes se quieren y se confirma escribiendo la palabra, igual que la CLI exige
    `--yes`. Cada paso dice si escribe, fusiona, ya coincide o se salta —y por qué—, y al terminar
    se enseña la foto previa, que es por dónde se vuelve atrás.
  - **Podar enseña antes qué se llevaría por delante**: cuántos quedan, cuántos blobs se liberan y
    los ids que se van. La retención es una política (7 diarios, 4 semanales, 6 mensuales, más los
    fijados y los etiquetados), no una intuición.
  - Las copias `.tar.gz` de `ccp backup` siguen en Ajustes: son el formato viejo, bueno para mover
    una configuración a mano a otra máquina.
  - El resumen de un snapshot (`snapshot.list`/`create`, y `ccp snapshot list --json`) gana `bytes`:
    lo que captura, que no es lo que ocupa —los blobs se comparten entre snapshots—. Es un campo
    añadido, no una forma cambiada, así que el protocolo sigue en `1`.

- **La app: P-20 Configuración, el editor unificado** (spec §7). Una pantalla para toda la
  configuración de Claude: arriba la capa (global · perfil · proyecto · ventana), a la izquierda los
  tipos (instrucciones, MCP, skills, agentes, comandos, hooks, permisos, variables, plugins, estilos,
  barra de estado y ajustes) y en el centro los elementos con su **procedencia** y sus distintivos
  **dónde aplica** (CLI · Code · Chat). Absorbe P-05 —el conmutador «Efectivo» es la vista fusionada
  de una cuenta, con lo tapado marcado— y la vista de solo-gestionado de P-15.
  - Lo que se ve y no se edita dice por qué, con la misma frase que la terminal: lo trae un plugin,
    lo fija managed-settings, lo proyecta ccp desde el overlay.
  - Editores: MCP (stdio/http/sse/JSON, con los secretos **enmascarados** —lo que no se toca se
    restituye al guardar—, el destino por servidor y el conmutador de apagarlo en un perfil), skills,
    agentes, comandos y estilos como archivo, hooks por evento, permisos con sus tres listas y
    CLAUDE.md con vista previa de sus `@import`.
  - **Los hooks se editan y se quitan por evento**, no solo se añaden: la unidad que ccp sabe escribir es el
    array del evento, así que la pantalla lo reescribe entero (la TUI sigue solo añadiendo, porque ahí no hay
    id estable al que agarrarse).
  - La capa de una **ventana de Desktop es de solo lectura**: enseña lo que la ventana recibe y dice dónde
    declararlo, porque escribir ahí lo desharía la siguiente regeneración sin avisar.
  - **Acciones de capa**: llevar un elemento a la global, a otro perfil o a un proyecto, dejándolo o
    quitándolo del origen. Es el mismo elemento con otra capa: core decide el archivo, así que no hay
    una ruta nueva por destino.
  - Tras cada escritura se dice dónde quedó, a quién regeneró y **qué ventana de Desktop se queda con
    los MCP de antes** hasta reiniciarla. La proyección la hace la propia escritura, no una llamada
    aparte. Todo pasa por `core` (`config.item.*` y `mcp.*`), así que las barreras son las mismas que
    en la terminal, y cada pantalla enseña su equivalente de CLI.

- **`ccp serve`: los métodos del editor de configuración** (spec §7): `config.items`,
  `config.item.get|put|delete`, `config.effective` y `mcp.list|put|delete|setTargets|disable`. Son altas en
  el registro, así que el protocolo sigue en `1`. La capa viaja SIEMPRE en los parámetros —serve no tiene
  terminal de la que sacar el perfil activo— y las escrituras devuelven, junto a `ok`, el archivo tocado,
  los perfiles regenerados y `restart_pending`: qué ventana de Desktop se queda con los MCP de antes hasta
  que se reinicie. `config.effective` es el mismo resultado que `profiles.effective`, registrado con el
  nombre que usa la pantalla.

- **`ccp mcp`: alta, baja y destinos de los servidores MCP desde la terminal** (spec §7). La cara de
  terminal del editor: `list`, `add`, `rm`, `enable`, `disable` y `targets`, con `--json` en todos y
  `--scope global | profile[:<n>] | project[:<ruta>] | desktop[:<n>]`. Sin `--scope` se edita el perfil
  activo de la terminal, que es donde está quien teclea.
  - `add` tiene tres formas y solo una por llamada: el comando tras `--` (stdio, con `--env CLAVE=valor`),
    `--url` (remoto, con `--transport` y `--header`) o el JSON pegado. Mezclarlas se rechaza antes de
    escribir, porque en el archivo ya no se ve.
  - La capa manda y las barreras son las de `core`: la ventana de Desktop recibe los MCP del perfil pero no
    los declara, y un secreto en claro en un `.mcp.json` —que viaja en el repo— se niega diciendo cómo
    escribirlo (`${VARIABLE}`).
  - `list` enseña también lo que el perfil recibe pero no está en su `cc-home`: lo apagado y lo que solo va
    al chat. Si no, apagar un servidor lo haría desaparecer de la lista desde la que se vuelve a encender.
  - Tras cada escritura dice a quién regeneró y qué ventana de Desktop se queda con los MCP de antes hasta
    que se reinicie. No entra en la completion, como `backup`, `serve` y `snapshot`.
- **MCP, agentes y skills por perfil: una fuente declarada y varias proyecciones** (spec §6,
  [ADR 0011](docs/adr/0011-una-fuente-declarada-varias-proyecciones.md)). Un perfil ya declara lo suyo en su
  overlay y cada regeneración lo proyecta a los archivos que leen de verdad las apps: `overlay/mcp.json` →
  el `cc-home/.claude.json` (tu `claude` y la pestaña Code) y el `claude_desktop_config.json` de su ventana
  (el chat). Desaparecen los códigos 3 y 5 de `instruct`: `ccp instruct add profile mcp` ya tiene dónde
  escribir, y también `agent`, `command` y `skill`.
  - **Capas**: global (`~/.claude.json`) ⊕ perfil (`overlay/mcp.json`) − apagados; si un nombre choca, gana
    el perfil. El bloque `mcp:` de `ccp.yaml` dice a qué destinos va cada servidor (`targets`), cuáles apaga
    un perfil (`disabled`) y si la ventana `default` participa (`desktop_default`, reservado). Es aditivo:
    `version` sigue en `2`.
  - **ccp solo toca los nombres que registró como suyos.** Lo que añadiste a mano en esos archivos se queda,
    y un nombre declarado que ya estaba ahí se informa como conflicto en vez de pisarse.
  - **El chat de Desktop tiene dos límites, y son suyos** (ADR 0016): solo entradas `stdio` —una `http`/`sse`
    se informa en vez de escribirse— y no se escribe con la ventana abierta, porque no la relee en caliente:
    el cambio queda pendiente hasta el siguiente arranque, y es el arranque el que lo aplica: tanto
    `ccp desktop open` como el lanzador del Dock aplican lo pendiente justo antes de lanzar la ventana.
    En tu ventana principal (`default`) no se escribe.
  - **`overlay/{agents,commands,skills,output-styles}/`**: en cuanto el perfil tiene algo propio, su
    directorio del `cc-home` pasa a ser global ∪ perfil, con directorios reales y symlinks solo en las hojas
    (la forma que Desktop exige). `profile add` no cambia.
  - **Permisos con unión opcional**: el merge sigue reemplazando arrays (ADR 0002); para sumar se escribe
    `"permissions": {"$merge": "union", …}` en el overlay. La marca no llega al `settings.json` generado.
    Los hooks se editan como el array entero de su evento, que es lo que faltaba para poder borrarlos.
  - **`ccp profile sync --check`** dice lo que la proyección cambiaría sin escribir nada y sale 1 si algo está
    desfasado: MCP, artefactos y los dos archivos generados del perfil (`cc-home/settings.json` y
    `cc-home/CLAUDE.md`); en `serve`, `profiles.drift`. Los conflictos y lo que el chat no puede cargar se cuentan, pero
    no mandan en el código de salida: ningún sync los arregla.
  - **`ccp doctor`** gana `projection_stale`, `desktop_restart_pending`, `mcp_command_missing`,
    `mcp_unmanaged_only_desktop` y `cc_home_symlink_nonleaf`. Cada fila de la vista efectiva lleva su
    `AppliesTo` (`cli`, `desktop-code`, `desktop-chat`).
  - `overlay/mcp.json` lleva `env` y `headers`, así que en los snapshots viaja como `secret` (sellado), a
    diferencia del resto del overlay.
- **`ccp scan` y `ccp adopt`: detectar la máquina** (spec §5). `scan` hace inventario de toda la configuración de
  Claude: global, perfiles, cada ventana de Desktop, proyectos y `CLAUDE_CONFIG_DIR` sin gestionar. De cada
  elemento dice dónde aplica (CLI, Code o chat), según lo medido en el ADR 0016. Un archivo ilegible cuenta como
  «desconocido», nunca como vacío, y los secretos solo salen por su ruta.
  - `adopt` enseña un plan con IDs estables y solo lo aplica con `--yes`, tras un snapshot de seguridad. Sube a
    global los MCP que solo tenía Desktop, salvo que dos ventanas los tengan con credenciales distintas, y adopta
    un `~/.claude-x` como perfil copiando su configuración, sin tokens ni `env`.
  - `ccp serve`: `inventory.scan`, `adopt.plan` y `adopt.apply`. En la app de escritorio, la pantalla P-19
    «Detectar esta máquina», también desde Ajustes.
- **`ccp snapshot`** — el historial de toda la configuración, no solo la de ccp. Entran `ccp.yaml`, los
  overlays, `~/.claude` (settings, CLAUDE.md, agentes, comandos, skills, hooks, plugins), la parte de MCP de
  cada `.claude.json`, la config de cada ventana de Desktop y los archivos locales de cada carpeta con regla.
  Ver el [ADR 0012](docs/adr/0012-snapshots-content-addressed.md).
  - `create [-m <etiqueta>]`, `list`, `show`, `diff <id> [<id>]`, `restore <id> [--only <ruta>]`,
    `prune`, `pin`/`unpin`, `export <id> <archivo> [--with-secrets]` e `import <archivo>`. Todos aceptan
    `--json` salvo `pin`, `export` e `import`.
  - Almacén direccionado por contenido en `~/.config/ccp/snapshots`: lo que no cambia entre dos snapshots se
    guarda una vez. Los secretos (keys de proveedor, MCP, config de Desktop) van sellados con una clave local
    `0600`.
  - **`restore` sin `--yes` solo enseña el plan.** Con `--yes`, primero guarda un snapshot del estado actual y
    sin él no escribe; después regenera los perfiles afectados. De un `.claude.json` solo se fusiona su
    configuración, nunca la sesión.
  - **Snapshot de seguridad antes de `ccp profile rm` y `ccp backup restore`** (también desde la app de
    escritorio). Si no se puede guardar, no se borra ni se restaura nada.
  - **Un snapshot diario**, lo toma el primer comando de gestión pasadas 20 horas. Los comandos de scripting y
    el hook del prompt no lo disparan. `CCP_NO_AUTO_SNAPSHOT=1` apaga el diario y el de seguridad.
  - `prune` conserva 7 días, 4 semanas y 6 meses, el último y todo lo fijado o etiquetado.
  - Un `.ccpsnap` solo lleva secretos con `--with-secrets`, sellados con una frase de al menos 12 caracteres
    (Argon2id + XChaCha20-Poly1305).
- **`ccp serve`: métodos `snapshot.*`** (`list`, `show`, `diff`, `create`, `restore`, `prune`, `pin`,
  `export`, `import`) para la app de escritorio. El protocolo sigue en `1`.

- **App de escritorio (`gui/`, Tauri + React)** — todo ccp desde una ventana: qué cuenta usa cada carpeta,
  el uso que le queda a cada una, las reglas con un probador, las conversaciones de todas las cuentas (de
  terminal y de Desktop) y un asistente para moverlas, los préstamos, la rotación y un **mapa de cuentas**
  donde los respaldos se conectan arrastrando y nada se escribe hasta revisar y aplicar, las ventanas de
  Desktop, el diagnóstico, la memoria de Claude, los ajustes y las copias de seguridad. Cada pantalla enseña
  su comando equivalente, y lo que solo puede ocurrir en una terminal (un `/login`, un préstamo, una sesión
  supervisada) se abre en Terminal con el comando puesto en vez de fingirse. Ver [gui/README.md](gui/README.md).
- **`ccp serve --stdio`** — el motor de ccp como API: JSON por stdin/stdout, una petición y una respuesta
  por línea. Es lo que usa la app de escritorio, y reutiliza los mismos comandos del CLI en vez de
  reimplementarlos. Las lecturas van en paralelo y las escrituras en serie.

- **`ccp desktop copy <uuid|título> <perfil>`** — lleva una conversación de la pestaña Code a la ventana de
  Desktop de otro perfil: la copia al cc-home del destino y le pide a esa ventana que la importe, para que
  salga en su barra lateral con el mismo título. Ver el [ADR 0010](docs/adr/0010-desktop-session-copy-via-import-link.md).
  - **La importación la hace Desktop**, con su propio enlace `claude://resume?session=<uuid>` mandado con
    `open -a` a esa ventana concreta. ccp lee el índice de Desktop pero no lo escribe nunca, y no da la
    importación por hecha hasta verla en él.
  - **Nunca pisa una conversación**: si el destino siguió por su cuenta, no se toca; si las dos siguieron
    por separado, no se escribe nada; una copia vieja se pone al día solo con esa ventana cerrada.
  - **El enlace solo va a donde debe**: se comprueba antes con `ps` y `lsappinfo` que la ventana destino
    tenga su propia identidad, y si no se puede comprobar no se manda.
  - El título viaja con la copia aunque se pusiera al principio de una sesión larga (Desktop solo lo busca en
    los últimos 256 KB), y la carpeta de subagentes y workflows también.
  - `--from <perfil>`, `--dry-run`, `--no-open`. Funciona también con una sesión del CLI, por su uuid.
- **`ccp desktop sessions [<perfil>] [--archived] [--json]`** — las sesiones de la pestaña Code de cada
  ventana, con el título de su barra lateral, la carpeta y el uuid. `--json` emite siempre un array con
  `profile`, `uuid`, `title`, `cwd`, `last_activity`, `archived` y `transcript`.

### Fixed

- **El portal buscaba `projects/` donde ccp escribe `project/`.** Los cambios de un repo salían agrupados en
  «otros» en la línea de tiempo, sin nombre de proyecto. Lo encontró el test nuevo del modelo de P-20, que
  toma su inventario de rutas lógicas de `core` en vez de una lista escrita a mano.
- **Un caso de los tests de revisiones dejaba de probar lo que decía una de cada dieciséis veces.** Derivaba
  el «dispositivo desconocido» cambiándole el último dígito al del portal, y ese dígito ya era el mismo con
  esa probabilidad: entonces el desconocido era el propio portal, que sí existe.
- **La pista de `ccp instruct add profile mcp` ya no promete que `global` llega a todos los perfiles.**
  `global` escribe en `~/.claude.json`, que solo lee `default`: cada perfil official lee su propio
  `cc-home/.claude.json`. Ahora sugiere `project` (el `.mcp.json` del repo, que ven todos) y dice a quién
  llega `global`. El MCP por perfil de verdad llega con la proyección por capas.
- **`ccp backup restore` regenera el cc-home de los perfiles restaurados.** Antes, `settings.json` y
  `CLAUDE.md` seguían viejos hasta el siguiente `ccp profile sync`. La salida y `backup.restore` de
  `ccp serve` dicen cuáles se regeneraron.
- **«Tiene login» ya no significa «existe `.claude.json`».** Claude Code crea ese archivo en su primer
  arranque, antes del `/login`, así que `ccp doctor`, `ccp profile show`, la TUI y la app de escritorio
  daban por iniciada una sesión que no lo estaba. Ahora cuenta la cuenta registrada (`oauthAccount` o
  `primaryApiKey`), también para `default`.
- **Cada perfil hereda también `output-styles/`, `hooks/` y `keybindings.json` del `~/.claude` global**,
  como ya heredaba `commands/`, `agents/`, `skills/` y `plugins/`. Antes no veía los estilos de salida ni
  los atajos, y un hook global que invocaba un script de `~/.claude/hooks` desde el perfil fallaba. Los
  perfiles ya creados los ganan con `ccp profile sync`, que ahora siembra lo que falte sin pisar nada y que
  `ccp upgrade` ya ejecuta. El espejo de Desktop también los convierte.
- **La vista efectiva de un perfil enseña sus servidores MCP, `permissions.deny` y `permissions.ask`,
  y los ajustes sueltos** (`model`, `outputStyle`, `permissions.defaultMode`, la barra de estado), con la
  capa de la que sale cada uno. En la TUI (`e` sobre un perfil), en la pantalla Config de la app y en
  `profiles.effective` de `ccp serve`.
- **`ccp profile add` genera la config del perfil al crearlo**, como ya decía: `overlay/`,
  `cc-home/CLAUDE.md` y `cc-home/settings.json`. Antes el perfil nacía sin ellos hasta el primer
  `ccp profile sync` (el oráculo bash sí los generaba). También desde la TUI y la app de escritorio. Si esa
  generación falla, el perfil queda creado y el error dice cómo terminarla (`ccp profile sync <perfil>`); la
  TUI guarda igual la API key que tecleaste.
- **Lo que cambias con `/config` (o `/model`, `/permissions`…) dentro de un perfil ya no se pierde en el
  siguiente `ccp profile sync`.** Claude Code lo escribe en `cc-home/settings.json`, que ccp genera.
  Ahora ccp guarda una copia de lo último que generó y, antes de regenerar, pasa al overlay del perfil lo
  que añadiste o cambiaste. Pasa en todo lo que regenera: `profile sync`, `profile config`, `instruct add`,
  los restores, el rename y la app de escritorio.
  - Lo que quitaste solo se avisa: el overlay no puede expresar un borrado.
  - Si la clave cambió también en el overlay desde la última regeneración (incluido borrarla), gana el
    overlay y se avisa.
  - `env` nunca se adopta: suele llevar tokens, y el overlay va en claro en los backups «sin secretos» y
    en los snapshots. Se avisa, y se pone a mano con `ccp profile config` si se quiere en el perfil.
  - Si el overlay no se puede escribir (un enlace a un almacén de dotfiles de solo lectura), no se cuenta
    como guardado y la regeneración sigue como antes.
  - Nada de lo que escribiste se pierde sin rastro: lo que no pasa al overlay (un conflicto, `env`, lo que
    no se pudo guardar, un archivo que no era JSON) queda en una copia `0600` en
    `profiles/<perfil>/state/`, que no entra en backups ni snapshots. Dos copias distintas no se pisan.
  - Los sensores de `auto_handoff` nunca se adoptan.
  - `ccp profile sync`, `ccp profile config`, `ccp instruct add` y `profiles.sync` de `ccp serve` (`drift`)
    dicen qué se adoptó y qué no. Lo que encuentra otro camino (la TUI, los restores, el rename) queda
    pendiente hasta el siguiente de esos. La app de escritorio enseña el detalle en el aviso de
    «Resincronizar todas».
  - Un perfil creado antes de este arreglo no tiene esa línea base: en su primer sync, que `ccp upgrade`
    ya ejecuta, no se adopta nada, pero si lo que había no coincide con lo que se genera queda una copia; y
    adopta desde el siguiente.
- **`ccp profile rename` avisa de que hay que volver a iniciar sesión.** Claude Code guarda la credencial
  de un perfil official con un nombre que sale de la ruta de su cc-home (ADR 0016, M4), y el rename cambia
  esa ruta. ccp no toca el Llavero: dice que hace falta `ccp profile login <nuevo>`, también cuando lo que
  falla es la regeneración de después, porque para entonces la carpeta ya se movió. Lo dicen también la
  TUI, `profiles.rename` de `ccp serve` (`relogin`) y la app de escritorio, que lo advierte antes de
  confirmar.
- **`ccp profile rename` ya no mueve un perfil con su ventana de Claude Desktop abierta.** El directorio que
  se mueve lleva dentro el data dir de esa ventana y el cc-home de su pestaña Code, y la app en marcha sigue
  escribiendo por ruta con el nombre viejo: podía recrear un `profiles/<viejo>/…` a medias y dejar el estado
  del perfil partido en dos. Ahora se niega (salida 1) hasta que cierres esa ventana; `--force` se salta la
  comprobación, como en `ccp desktop app rm`.
- **`ccp profile rename` renombra también el perfil dentro de `auto_handoff`**: el `fallback` de cada
  política, las claves y las listas de `allow_from` y `hooks`, en la misma escritura de `ccp.yaml` que las
  reglas. Antes se quedaban con el nombre viejo, así que la cuenta renombrada dejaba de usarse como préstamo
  sin avisar (`ccp session` fallaba con «el perfil de fallback no existe» y el diagnóstico lo marcaba como
  `chain_unknown_profile`), el gate `allow_from` dejaba de reconocerla y la siguiente regeneración de su
  cc-home le quitaba los sensores.
  - Si un paso posterior falla, `ccp.yaml` vuelve a quedar byte a byte como estaba: se guarda la
    configuración tal como se leyó en vez de invertir el cambio a mano.
  - Se niega a usar un nombre nuevo que `auto_handoff` ya menciona (restos de un perfil borrado: `profile rm`
    no limpia el bloque). Si no, la cuenta renombrada heredaría cadenas y un gate `allow_from` que no eran
    suyos, y un gate que le negaba préstamos se abriría sin que nadie lo decidiera.
  - Los comentarios de `ccp.yaml` que colgaban de la clave renombrada, en `profiles` y en `allow_from`, ya
    no se pierden.
  - Si el perfil tenía lanzador de Desktop, avisa de que ya no abre y da los dos comandos que lo sustituyen
    conservando su color (y su nombre, si era propio). El lanzador no se toca.
- **Las sesiones de la pestaña Code de Claude Desktop ya no salen «(sin título)»** en el selector de
  `ccp handoff`, en `ccp handoff sessions` (también en el `title` de `--json`) ni en el título que guarda el
  marcador del handoff. ccp solo leía el título que genera el modelo (`ai-title`), y Desktop, como el
  `/rename` del CLI, guarda el suyo como `custom-title`. Ahora se lee en el orden en que lo muestra Claude
  Code: el último `customTitle` y, si no hay, el último `aiTitle`.
- **La sesión que vuelve con `ccp handoff end` lleva `[de <perfil>]` en el título que se ve**, también si
  estaba renombrada o venía de Desktop. Antes solo se marcaba `aiTitle`, que Claude Code no muestra cuando hay
  un `customTitle`.
- Una línea de más de 8 MB en el transcript (una imagen pegada, un `tool_result` grande) ya no corta la
  lectura del título. Claude Code vuelve a escribir el título al final del archivo cada pocos turnos, así que
  el vigente solía quedar detrás del corte.

## [2.18.0] — una instancia de perfil ya no puede suplantar ni actualizar a tu Claude

Esta versión sale de un incidente real (2026-09-15). Un usuario con tres perfiles pasó una tarde creyendo
que había perdido 40 sesiones de trabajo. No había perdido nada: estaba mirando una instancia vacía mientras
otra ocupaba la identidad de su Claude principal, que durante horas no pudo abrir. Dos de las tres cosas que
fallaron estaban documentadas aquí y en el [ADR 0008](docs/adr/0008-desktop-launcher-two-layer-bundle.md)
como imposibles. Ver el [ADR 0009](docs/adr/0009-desktop-identity-is-not-durable.md).

### Fixed

- **Una instancia de perfil ya no puede actualizar tu `/Applications/Claude.app`.** Podía, y lo hizo: el log
  de ShipIt registra una instancia de perfil moviendo la app del usuario de 1.52386.3 a 2.110.0 desde una
  ventana abierta para otra cuenta. Lo que 2.17.0 afirmaba —«el lanzador nunca se actualiza a sí mismo (su
  bundle id no es el de Claude…)»— **era falso**: SQRLUpdater compara contra
  `NSRunningApplication.currentApplication.bundleIdentifier`, y ese id es el de Claude en cuanto el proceso
  corre desde el espejo. Toda instancia distinta de `default` arranca ahora con `DISABLE_UPDATE_CHECK=1`.
  `default` queda exento a propósito: esa instancia **es** tu Claude, y apagarle las actualizaciones sería
  secuestrártelo.
- **`ccp desktop open` sin perfil ya no abre una segunda ventana sobre tu data dir real.** El `-n` era
  incondicional, y como `default` no tiene data dir propio, ponía dos procesos Chromium sobre
  `~/Library/Application Support/Claude`. Ahora se fuerza instancia nueva solo cuando alguien está ocupando
  la identidad del bundle que se va a abrir — que es, justamente, cuando hace falta para poder llegar a tu
  Claude.
- **El aislamiento deja de depender de desde qué terminal lanzaste.** `open` hereda el entorno de quien lo
  invoca y `--env` solo sobrescribe lo que nombra: lanzar un perfil *official* desde una terminal con un
  perfil deepseek activo metía su `ANTHROPIC_BASE_URL` viva en la ventana, con el Code tab hablando con otro
  proveedor bajo la cuenta de Anthropic. El entorno del hijo se fija entero.
- **El lanzador se reconstruye también cuando el espejo se queda huérfano.** ShipIt no parchea el bundle: lo
  reemplaza entero con un `move`, así que los hard links quedan apuntando a la versión vieja aunque el número
  de versión no cambie, y el lanzador retenía una copia completa de Claude en disco sin decirlo.
- **`ccp desktop app` ya no falla cuando la reconstrucción iba a ser un no-op**, y `ccp desktop app rm`
  acepta el `--force` que su propio mensaje sugería. Las rutas que borran o reconstruyen un bundle con su
  ventana abierta ahora están todas cubiertas, no dos de cinco.
- Un clic en el icono del Dock ya no dispara la migración de configuración: no es el usuario pidiendo que se
  le reescriba `~/.config`.

### Added

- **`ccp desktop doctor [<perfil>] [--json]`** — lo que aquel día hubo que averiguar a mano con `lsappinfo`,
  `ps`, `codesign` y `du`. Dice qué ventanas hay vivas, si alguna corre bajo la identidad del Claude
  principal, si alguna escribe su historial de Code en el `~/.claude` global, si el espejo de un lanzador se
  quedó atrás y —lo que de verdad importaba— si un data dir guarda sesiones de más de una cuenta:
  **no se ha borrado nada**, las sesiones se indexan por cuenta y vuelven al entrar con ella.
  - **Diagnostica; nunca repara.** Borrar lanzadores o reconstruir bundles desde un diagnóstico lo
    convertiría en una segunda fuente de pérdida.
  - **Una sonda que no se puede ejecutar produce `unknown`, jamás `ok`.** Un doctor que dice «todo bien»
    porque no pudo mirar convierte una duda en una falsa certeza.
  - `--json` emite siempre un array, con `code`, `severity`, `profile` y `detail`. Sale 1 solo si hay algo
    roto *ahora*: un espejo desfasado o una sonda ausente no hacen fallar un script.
- **`ccp desktop open --force`**: salida de emergencia del preflight nuevo, que por defecto se niega a abrir
  una segunda ventana sobre un data dir que ya tiene una.

### Changed

- **`ccp desktop open` construye el lanzador del perfil si no existe**, en vez de caer al lanzamiento
  directo. Sin lanzador la ventana corre desde `/Applications/Claude.app` y macOS no puede distinguirla de
  tu Claude: es exactamente lo que provocó el incidente. Con `--plain` sigues pudiendo pedir el camino
  directo, y ahora se te dice lo que implica.
- La documentación corrige lo que quedó desmentido: el ADR 0008 lleva una nota de enmienda, y `CLAUDE.md`,
  los cuatro README y la spec de diseño ya no afirman que una actualización nunca rompe un lanzador ni que
  una instancia no puede actualizarse a sí misma.

## [2.17.0] — lanzadores de Claude Desktop con identidad propia, y la vista de perfil en la TUI

### Added

- **`ccp desktop app`**: un lanzador por perfil en `~/Applications` (`Claude (<perfil>).app`) con nombre
  propio y el icono de Claude tintado de un color (paleta o `#rrggbb`, asignado solo si no se elige). Sale
  en Spotlight y Launchpad, se fija en el Dock, y abre la instancia del perfil con esa identidad en el Dock,
  Cmd-Tab y la barra de menús. `ccp desktop open` lanza a través de él cuando existe (`--plain` lo salta);
  `ccp desktop app rm` lo quita; `ccp desktop list` lo enseña (`--json` gana `app` y `color`).
  - **La sesión se conserva.** No es una copia retocada de la app: dentro hay un espejo prístino de
    `Claude.app` hecho de hard links (byte a byte el original, sin disco extra) y el ejecutable es el
    propio `ccp`, que pone `--user-data-dir` y `CLAUDE_CONFIG_DIR` y hace exec del stub del espejo a través
    de un symlink de la capa externa. Con eso LaunchServices identifica el proceso con el lanzador (nombre,
    icono, bundle id propio) mientras el Keychain lo valida contra el bundle prístino —una copia con el
    `Info.plist` tocado, o cuyo ejecutable en marcha no es su `CFBundleExecutable`, pierde la cuenta con
    `errSecAuthFailed`.
  - **Las actualizaciones siguen llegando.** Los hard links fijan la versión con la que se construyó el
    espejo, así que actualizar `Claude.app` no rompe nada: en el siguiente arranque (Dock o `ccp desktop
    open`) `ccp` ve el `CFBundleVersion` nuevo y reconstruye el espejo; también tras `ccp upgrade`. El
    lanzador nunca se actualiza a sí mismo (su bundle id no es el de Claude, así que Squirrel no encuentra
    qué instalar).
  - Nunca reclama `claude://`: los deep links y el callback del login siguen yendo a la app normal.
  - El ejecutable del lanzador es un hard link (o copia) del binario, no un symlink ni un script: a un
    symlink LaunchServices no lo lanza y a un script lo lanza bajo Rosetta, donde Chromium no arranca.
- Completion de `desktop` con `app`; `ccp install` la refresca.
- **Vista de perfil** (`e` sobre un perfil). Tres cajas con el chrome del
  dashboard —Instrucciones · Env · Efectivo— que responden lo que hasta ahora no
  respondía nada: qué configuración aplica ese perfil y **de qué capa sale cada
  cosa** (global, overlay, o la capa de sensores del auto-handoff). Se editan las
  reglas y las variables de entorno desde ahí; los hooks se añaden pero no se
  borran, y la tecla lo explica en vez de fingir que puede.
- **`core.ProfileEffective`**: la procedencia como dato, recorriendo las mismas
  tres capas y en el mismo orden que `cfgMergeSettings`.
- **`core.OverlayEnvSet` / `OverlayEnvDel`**: escritura de variables en el
  overlay, con el invariante de `ProfileConfig` — si el resultado no valida, el
  último overlay bueno no se toca.

### Changed

- **Un solo sitio pinta la TUI.** `internal/tui/shell.go` pasa a ser el único
  que dibuja cabecera, cajas, cursor, ventana, estado y pie; el dashboard y la
  vista Config se pasaron a él. De paso trae ventana alrededor del cursor, que
  tapa un agujero que la vista Config ya tenía: una lista larga se salía de la
  pantalla sin avisar.

## [2.16.0] — una instancia de Claude Desktop por perfil, con el Code tab aislado de verdad

### Added

- **`ccp desktop`**: una instancia aislada de Claude Desktop por perfil. Llega al binario por el
  `*) command ccp "$@"` que el rc ya tiene, así que **funciona sin reinstalar el rc**; `ccp install`
  solo hace falta para que autocomplete.
  - `ccp desktop open [<perfil>]` (sin perfil, se resuelve por el cwd igual que el hook del prompt),
    `list [--json]`, `path <perfil>`, `prepare <perfil>`, `rm <perfil> --yes`.
  - El aislamiento son **dos** cosas, y la segunda es la que no se ve. `--user-data-dir` mueve la
    identidad de la app (sesión, tokens, MCP, Cowork), pero **el Code tab de Desktop no lo lee**:
    lee `process.env.CLAUDE_CONFIG_DIR` y cae a `~/.claude` si está vacío. Sin inyectar el entorno
    del perfil al lanzar, dos ventanas con cuentas distintas comparten credenciales de CLI,
    `projects/` e historial. `ccp desktop` emite las dos a la vez.
  - Solo perfiles `official` y `default`. Un perfil de proveedor se rechaza con mensaje explícito:
    Desktop no lee `ANTHROPIC_BASE_URL`, así que la ventana quedaría con el Code tab en DeepSeek y
    la mitad de chat en Anthropic sin autenticar, sin forma de saber cuál estás mirando.
  - `default` **no se reubica**: su instancia es la de siempre, en la ubicación estándar de la app.
  - En el primer arranque de cada instancia se avisa de la carrera de `claude://` (los deep links
    van a la instancia que registró el esquema de último, así que el login se hace de una en una).

- **`MirrorForDesktop`**, que es lo que hace que lo anterior sea posible. Desktop valida el
  «co-writable boundary» y **rechaza symlinks en cualquier componente no-hoja bajo el config root**,
  y `seedCCHome` siembra exactamente eso (`cc-home/commands -> ~/.claude/commands`). No se cambia
  `seedCCHome`: el oráculo bash afirma `[[ -L "$cch/plugins" ]]`, o sea que la forma con symlinks de
  directorio **es** el contrato de `profile add`. La conversión es opt-in y por perfil.
  - El espejo son directorios **reales** replicando el árbol global, con symlinks solo en los
    archivos **hoja** — que la regla exime, porque recorre `c[0..len-2]`. Se conserva el compartido
    en vivo con `~/.claude` sin dejar un symlink en posición no-hoja.
  - `agents/` y `skills/` pasan a ser directorios reales del perfil aunque el global no los tenga:
    es lo que permite tener **agentes distintos por perfil**.
  - Una entrada que ya existe **nunca se pisa**, así que el re-espejado es idempotente y un archivo
    propio del perfil gana sobre el global. Solo se podan enlaces colgados que apuntan **dentro** del
    origen global; un symlink del usuario a otro sitio no es nuestro y se respeta.

## [2.15.1] — la rotación deja de apagarse sola

Tanda de correcciones sobre `ccp session`, el handoff y el conteo de límites. Ninguna
toca el contrato congelado (parity gate), el esquema sigue en `version: 2` y no hay
claves nuevas de configuración. Cinco de las seis venían de una auditoría del código;
la sexta salió de que la rama estaba en rojo y nadie lo sabía.

### Fixed

- **Un `claude` que atrape `SIGTERM` y salga con 0 ya no apaga la rotación entera.**
  El bucle decidía el desenlace por el código de salida del hijo **sin mirar quién lo
  había matado**, y eso está bien para casi todo menos para el único código que
  nuestro propio `SIGTERM` puede producir: el 0. Si `claude` instala un handler para
  correr sus hooks `SessionEnd` —justo lo que los 10s de gracia existen para
  concederle— y sale con 0 después de que lo matáramos por un límite, aquello se leía
  como «fin feliz»: se cerraba el préstamo y se retornaba. La corrida terminaba sola
  al primer límite, de madrugada y con el trabajo a medias, **sin un solo síntoma**.
  - `Process.Terminate` devuelve ahora si la señal llegó a salir, y de ahí sale
    `childOutcome.exited`. La carrera que el diseño original protegía sigue cubierta:
    el hijo que imprime el límite y muere en el mismo instante se encuentra ya muerto,
    así que vuelve a contar como salida propia.
  - El caso de `Ctrl-C` (130) **sigue decidiéndose solo por el código**, y la asimetría
    es deliberada: un 130 no puede salir de nuestro `SIGTERM`, pero un 0 sí.
- **`min_dwell` ya no te retiene dentro de un 429.** Se aplicaba entero a los eventos
  *reactivos*, o sea a los que llegan cuando el turno **ya falló**: con los defaults,
  hasta 20 minutos mirando un error dentro de una cuenta que no responde, teniendo
  otra fresca al lado. Y el reloj se siembra al arrancar la corrida, así que en el
  primer lanzamiento era el piso completo.
  - El dwell entero se reserva ahora para el sensor **proactivo** (`statusline`), que
    es el único que avisa antes de que nada falle. Los reactivos —`transcript`,
    `stream-json`, `hook`— caen a un techo de 30s, que conserva la protección real
    (que tres perfiles no se quemen en un minuto) sin el castigo.
  - Un origen desconocido cae al lado **urgente**: el `Source` de un sentinel lo
    escribe el hook desde un payload externo, y ante la duda es mejor rotar pronto de
    más que retener al usuario dentro de un límite.
  - La vuelta a casa por `return_check` no cambia: sigue exigiendo el `min_dwell`
    completo, porque ahí nadie tiene prisa.
- **`handoff end` era imposible para siempre con una línea de más de 8 MB.** La
  reescritura del transcript fijaba ese techo por línea, y una imagen pegada o un
  `tool_result` grande lo pasa. El límite además era **asimétrico**: el camino de ida
  no tenía ninguno, así que la sesión se podía prestar y no se podía devolver nunca,
  con `discard` como única salida — o sea dejar la conversación viviendo solo en el
  perfil prestado, justo el desenlace que el módulo entero existe para evitar. Ya no
  hay techo.
- **Los transcripts se escriben de forma atómica.** Eran el único dato irremplazable
  de todo ccp y lo único que se escribía truncando el destino antes de tener el
  contenido nuevo: un `Ctrl-C` a media escritura dejaba un `.jsonl` a medias **con el
  uuid bueno**, que es peor que no tener nada porque `claude --resume` lo encuentra y
  lo abre. Ahora van por temporal + rename, con nombre aleatorio (dos procesos pueden
  escribir el mismo destino fuera del lock) y con un sufijo que **no** acaba en
  `.jsonl`, para que el temporal no se cuele en el selector de sesiones.
- **El bloque `auto_handoff` ya no pierde claves que no entiende.** `auto_handoff`
  estaba protegido a nivel de clave, pero no por dentro: cualquier clave desconocida
  dentro del bloque o de una política se perdía en el siguiente guardado — y «el
  siguiente guardado» es cualquier `ccp rule set` o `ccp profile add`, no una
  operación de auto. Instalar una versión anterior y tocar una regla te borraba parte
  de la política sin decir nada. Es aditivo: `version` sigue en 2.
- **El sensor proactivo ya no destierra un perfil por una señal sin fecha.** Un
  `usedPercentage` alto sin `resets_at` —el defecto conocido de la caché de Claude
  Code— disparaba la rotación, y sin ventana que esperar el destierro es el cooldown
  de respaldo entero: una hora, por una medida que quizá describe una ventana ya
  cerrada. Ahora el sensor exige la fecha antes de emitir. **Pintando no cambia
  nada**: la barra de estado y `ccp auto status` siguen mostrando «95% sin fecha»,
  porque ahí es información válida; la regla va donde se *decide*, no donde se enseña.
- **Dos tests llevaban desde el 1 de agosto fallando sin que nadie lo viera.** Fijaban
  un `resets_at` con una fecha literal (`2026-08-01`), y como las ventanas ya
  reseteadas se descartan a propósito, el día que el calendario alcanzó la constante
  dejaron de disparar. Ahora la fecha es relativa a `now`, con el porqué escrito al
  lado para que no se repita.

### Fixed (TUI)

- **El estilo del panel de `ccp handoff` era código muerto.** El panel se pinta en
  `/dev/tty`, pero lipgloss decide si hay color midiendo su renderer por defecto, que
  mira `os.Stdout` — y en la ruta real ese stdout es la sustitución de comando de la
  función de shell, o sea una tubería. Resultado: ni un byte de ANSI en producción,
  mientras los tests (también sin tty) lo daban por bueno. Es el mismo fallo que el
  CLI ya había diagnosticado y corregido en su día. `NO_COLOR` sigue mandando.

## [2.15.0] — instalar con una sola línea, y la TUI suelta la terminal al editar

### Added

- **`install.sh` corre en remoto**: `curl -fsSL https://raw.githubusercontent.com/JoseAFlores777/ccp/main/install.sh | bash`
  instala sin que haya que clonar el repo antes. Es ahora la forma documentada de instalar
  (README, README.html y la landing), y el clon sigue funcionando igual.
  - Con `curl | bash` no hay `BASH_SOURCE`, así que el checkout se detecta por **contenido**
    (`install.sh` + `cmd/ccp/main.go`), nunca por `$0` — que en una tubería es `bash` y haría que
    el script tomara el cwd por repo y registrara una `install-source` mentirosa.
  - Sin checkout alrededor, el script se trae uno a `$CCP_HOME/src` (`git clone --depth 1`, o
    tarball de `codeload` si no hay git). No es un capricho: de ahí salen los comandos `/ccp:` y es
    lo que `ccp upgrade` re-ejecuta después. Si ya existe y tiene cambios sin guardar, **no se toca**.
  - Traerse el código **nunca es fatal**: si falla, el binario se instala igual (viene del release,
    no del código) y el aviso dice qué se pierde — comandos `/ccp:` y fuente para `ccp upgrade`.
  - Perillas nuevas: `CCP_SRC_DIR` (dónde queda esa copia) y `CCP_NO_SOURCE=1` (solo binario).
  - El binario sigue saliendo del GitHub Release con **sha256 verificado**; nada de eso cambia.

### Fixed

- **La tecla `e` del panel Perfiles abre el editor de verdad.** Salía del alt-screen con
  `tea.Sequence(tea.ExitAltScreen, …)`, que quita la pantalla alternativa pero **no suelta la tty**:
  el renderer de bubbletea seguía repintando y su lector seguía leyendo stdin, así que el `nano`
  arrancaba debajo del dashboard y las teclas se repartían entre los dos procesos. Medido en un pty:
  con `nano` abierto, teclear `HOLA` le llegaba solo la `H`. Ahora va por `tea.Exec`, el mismo
  adaptador que la vista Config ya usaba.
  - El daño no se quedaba en la `e`: a partir de ahí **toda la sesión** quedaba corrupta, porque el
    `tab` para llegar al panel Reglas también se lo comía el editor invisible y la `a` abría el form
    de añadir perfil. Lo que parecían dos bugs («la `e` parpadea», «no me deja añadir una regla») era
    este.
- La leyenda del panel Perfiles decía `e:config` mientras el pie decía `c: config` para la vista
  Config — dos cosas distintas con el mismo nombre. Ahora dice `e:overlay`, que es lo que edita.

## [2.14.0] — `ccp session` se configura solo, y la TUI edita la config

### Added

- **`ccp session` detecta lo que le falta al repo y lo ofrece.** Lista los huecos —la política, la
  regla, la cadena vacía por el gate, los sensores—, enseña **la ruta exacta** que va a escribir, y
  pregunta una vez. Cada hueco se cierra por el **mismo camino** que el comando que habrías tecleado
  (`auto init`, `path set`, `auto chain add`, `auto install`): cero rutas de escritura nuevas.
  La regla se propone sobre la **raíz del repo git**, no sobre el cwd — una regla en un subdirectorio
  es casi siempre un error de dedo que luego confunde.
  Pregunta **una vez por repo**, contestes lo que contestes; la marca vive bajo `state/auto/bootstrap/`,
  que es caché: borrarla vuelve a ofrecerlo. `--setup` lo fuerza, `--no-setup` lo salta.
- **Vista Config en la TUI** (`c`, o `:config`). Toma el cuerpo del dashboard en vez de añadir un
  cuarto panel: a 80 columnas los tres actuales ya van justos. Cinco secciones —Defaults,
  Auto-handoff, Cadena (reordena con `J`/`K`), allow_from y Sensores— y `e` abre la config entera en
  el editor gráfico. No reimplementa ninguna regla: la cadena y el gate salen de las mismas funciones
  de `core` que usa `ccp auto chain`.
- Completions de segundo nivel: `ccp auto chain add|rm|mv <TAB>` ofrece **nombres de perfil**, y los
  flags (`--policy`, `--at`, `--no-allow`, `--setup`, `--no-setup`, y los de `config edit`) se
  completan. Antes solo se completaba el comando, no el argumento que de verdad hay que recordar.

### Fixed

- El bootstrap **no escribe nada si los dos extremos de la conversación no son una terminal**. La
  primera versión solo miraba stdin: con `ccp session > log 2>&1` desde una terminal real, la pregunta
  se escribía en el archivo, el usuario no la veía, y como el default de `[S/n]` es sí, un Enter
  aplicaba la configuración entera. Ahora se comprueban stdin **y** la salida, con un mensaje distinto
  por motivo (sin TTY / `-p` / salida redirigida).
- El detector de TTY del bootstrap usa `isatty` y no `os.ModeCharDevice`: `/dev/null` **es** un
  dispositivo de caracteres, así que `ccp session < /dev/null` desde cron pasaba por interactivo.
- Un EOF a mitad del prompt ya no gasta la marca de «preguntar una vez»: una pregunta que nadie llegó
  a contestar no cuenta como contestada.
- La regla se ancla bien **a través de symlinks**: antes proponía un subdirectorio y además afirmaba
  que no estabas en un repo git cuando sí lo estabas.
- Cuando no se puede deducir el perfil, la columna dice `(falta perfil)` en vez de `(crear)`, se avisa
  con el `ccp path set` exacto, y **no** se crea una entrada `allow_from` para `default` (el `~/.claude`
  llano) hacia terceros.

## [2.13.0] — editar la config sin abrir el yaml a mano

### Added

- **`ccp auto chain`** — `show` / `add` / `rm` / `mv` / `set`, con `--policy`, `--at N` y
  `--no-allow`. El orden de la cadena ES la preferencia, y ahora se edita con un comando en vez de
  a mano en el yaml.
  `add` toca **también `allow_from`**, porque quien escribe «añádelo a la cadena» quiere que el
  perfil se use, y `fallback` sin `allow_from` no lo usa. Con tres límites: solo la entrada del
  primario del cwd (nunca las de otros), se reporta qué cambió en **cada** clave por separado, y
  `--no-allow` lo desactiva. `rm` estrecha el gate en la dirección contraria, así que `add` seguido
  de `rm` devuelve las dos claves al estado previo.
- **`ccp config edit`** — abre `ccp.yaml` en un editor **gráfico**: `--editor`, `defaults.gui_editor`,
  `$VISUAL`, VS Code y familia (`code`/`cursor`/`code-insiders` con `-w`), el lanzador del SO
  (`open -W -t`, `notepad`, `xdg-open`) y, de último recurso, el editor de terminal de siempre.
  `--profile <n>` abre el overlay de un perfil; `--terminal` salta directo al fallback.
  Si el editor **espera**, al cerrarlo ccp relee el archivo y lo valida, nombrando la clave y el
  valor ofensivos. Si **no** espera, lo dice en vez de fingir que validó — un `ccp.yaml` roto por
  una edición gráfica no se manifiesta al guardar, se manifiesta en el siguiente `ccp session`.
- **`ccp config gui-editor <cmd>`** — fija la preferencia. Sin argumento, enseña lo que la cadena
  elegiría **ahora mismo en esta máquina**: saber que la clave está vacía no te dice qué se va a abrir.
- Completions bash y zsh para todo lo anterior, con el oráculo bash y el golden regenerados.

### Fixed

- `ccp auto chain add` ya no deja el repo **atascado** cuando `allow_from` está declarado sin entrada
  para el primario (deny total). Antes el chequeo de duplicado miraba la lista `fallback` cruda y
  respondía «ya está en la cadena» para todos los perfiles, abortando antes de crear la entrada que
  desbloquea el repo — con `auto init` sembrando el fallback con todos los perfiles, ese era el estado
  de partida por defecto. Ahora crea la entrada y explica que lo que se abrió fue el gate, no la cadena.
- Los errores de política (`ccp auto chain show`, `ccp auto status`, el campo `error` de
  `auto status --json` y el pre-chequeo de `ccp session`) salen **en tu idioma**. Antes la misma
  condición se veía en inglés por un subcomando y en español por otro, porque `ResolveAutoChain`
  devolvía prosa castellana cruda. Ahora es un `ChainError` tipado que cada front-end renderiza.

## [2.12.0] — la barra dice cuándo vuelve la cuota

### Added

- La statusLine mínima pasa de `work-1 · 5h 14% · 7d 31%` a un **medidor con
  cuenta atrás**: `work-1  5h ▏█░░░░░░░░░▏ 2% ·2h13m  7d ▏██████░░░░▏ 59% ·3d`.
  El porcentaje solo decía cuánto llevas gastado; no respondía la pregunta que
  uno se hace al mirar la barra, que es **cuándo vuelve la cuota**.
  El medidor va verde por debajo del 70%, ámbar de 70 a 89 y rojo del 90 en
  adelante — el rojo empieza exactamente en el default de `threshold`, para que
  la barra y el motor no cuenten historias distintas. `NO_COLOR` quita el tinte y
  el medidor se sigue leyendo: por eso es medidor y no un punto de color.
- `core.RenderGauge` y `core.HumanUntil`/`HumanUntilAt`, primitivas puras (sin
  color, sin ancho, sin layout) para que la barra y el futuro panel Estado de la
  TUI compartan el medidor sin duplicarlo — `internal/tui` no puede importar
  `internal/cli`.

### Changed

- La línea **se dimensiona sola**: se monta a tres niveles de detalle, se mide
  cada uno en runas sobre la variante sin color y se pinta el más ancho que
  quepa. Un nombre de perfil largo cuesta celdas de medidor, no corrección. Si
  no cabe ninguno se entrega la forma compacta sin recortar, porque lo primero
  que se comería el recorte es el nombre del perfil.
- La barra **se tiñe aunque stdout sea un pipe**. `useColor` exige un char
  device, y Claude Code captura el statusLine por un pipe: el semáforo era código
  muerto justo donde el usuario lo mira. La barra usa ahora un gate mínimo que
  solo consulta `NO_COLOR` — quien renderiza esta línea no es la terminal, es CC.

### Fixed

- Un `resets_at` **ya vencido no produce cuenta atrás**. Un dato caducado no
  puede afirmar que tu cuota volvió; es la misma regla por la que una ventana sin
  dato se omite en vez de pintarse como `0%`.
- Los días de la cuenta atrás redondean **hacia arriba**: a 2d23h del reset
  pintaba `·2d` y volvías un día antes de que el perfil se liberara. El error
  máximo es de un día en ambos sentidos, así que lo único que se elige es la
  dirección — y nunca puede ser la que promete que la cuota está más cerca.
- Un `used_percentage` imposible ya no revienta el ancho de la línea: el clamp
  vivía dentro del medidor, así que el número y el medidor partían de valores
  distintos. Ahora ambos salen del mismo valor saneado (`core.ClampPct`).

## [2.11.2] — la barra propia enseña las dos ventanas

### Changed

- La statusLine mínima que ccp pinta cuando **no** tienes una propia pasa de
  `work-1 · 31%` a `work-1 · 5h 14% · 7d 31%`. Antes enseñaba solo el máximo
  entre las dos ventanas, que es el número que decide (la más gastada corta
  primero) pero como porcentaje suelto era ambiguo: no dice si te quedan horas
  o días, y saltaba de una ventana a otra en cuanto la otra la adelantaba, sin
  indicarlo. El sensor vigila las dos por separado —`ExhaustedAt` compara cada
  una contra el umbral—, así que la barra ahora enseña las dos.
  Una ventana **sin dato se omite** en vez de pintarse como `0%`: el bug conocido
  de CC 2.1.220 (`five_hour` a 0 con `seven_day` poblado) haría que ese `0%`
  afirmara justo lo contrario de lo que sabemos. Las etiquetas `5h`/`7d` son las
  que ya usaba `ccp auto status` y las claves del propio Claude Code.
  Si tienes statusLine propia no cambia nada: se sigue envolviendo y se sigue
  pintando la tuya intacta.

## [2.11.1] — CI verde en el commit tageado

Sin cambios de producto: los binarios son idénticos a los de 2.11.0. Lo único
que cambia es un test.

### Fixed

- `TestRunContextoCanceladoMataAlHijo` pasaba en macOS y fallaba **siempre** en
  CI, clavado en los 10s de `termGrace`. La causa era el falso claude del test,
  no el supervisor: `#!/bin/sh` + `sleep 30` deja al shell como hijo directo, y
  ahí manda qué sea `/bin/sh`. El bash de macOS hace `exec` implícito del último
  comando (el hijo ES `sleep` y muere con el SIGTERM), pero dash —el `/bin/sh`
  de Ubuntu— aplaza la señal mientras espera a un hijo en foreground, así que el
  shell sobrevivía la gracia entera y solo moría con el SIGKILL. Con `exec` el
  hijo es un único proceso, que es como corre el `claude` real (directo, sin
  shell en medio).

## [2.11.0] — auto-handoff

### Added

- **`ccp session`** — supervisor que corre `claude` como proceso hijo, detecta
  el límite de uso, presta la sesión a otro perfil y la relanza ahí. Un `claude`
  vivo no puede cambiar de perfil (`ANTHROPIC_BASE_URL`, `ANTHROPIC_AUTH_TOKEN`
  y `CLAUDE_CONFIG_DIR` se leen una sola vez al arrancar), así que lo único
  limpio es un proceso de fuera que pueda matarlo y volver a arrancarlo.
  Interactivo y headless (`-p`), con `--policy`, `--max-hops`, `--yolo`,
  `--session`, `--dry-run`, `--no-return` y `--claude-bin`.
  Códigos de salida: `0` ok · `1` uso/config · `2` E/S de handoff · `75` todos
  los perfiles agotados (`EX_TEMPFAIL`) · cualquier otro es el de claude.
- **Rotación en péndulo, no round-robin.** El primario es `ccp resolve $PWD`;
  todo lo demás son préstamos. Se vuelve a casa en cuanto la ventana del
  primario se reabre, aunque queden préstamos frescos. `max_hops` cuenta
  préstamos, no movimientos: la vuelta a casa cierra un préstamo, así que ni
  gasta presupuesto ni la bloquea uno agotado (tope real: `2·max_hops + 1`
  lanzamientos).
- **Temporizador `return_check` + guard `return_idle`.** Ningún sensor dispara
  cuando se libera *otra* cuenta, así que un temporizador pregunta solo si el
  primario ya se liberó. Como esa es la única jugada que mata a un hijo *sano*,
  exige cuatro condiciones a la vez: préstamo vivo, cooldown del primario
  vencido, `min_dwell` cumplido y transcript en silencio `return_idle` (90s por
  defecto; `0s` es el opt-out).
- **Cuatro sensores, tres a la vez.** `ccp _statusline` (proactivo, dispara al
  `threshold` % antes de que falle un turno), el `api_retry` del stream-json en
  headless, la cola del transcript, y el hook `StopFailure` (`ccp _limit-hook`).
  Los dos internos siempre salen con 0: una statusLine rota dejaría a Claude
  Code sin barra de estado y un hook que falla molesta en cada turno.
- **`ccp auto init | install | uninstall | status [--json] | test`** — siembra
  la política, instala la capa de sensores en el `settings.json` generado de
  cada perfil (envolviendo tu propia statusLine, sin reemplazarla) y verifica el
  cableado. Reversible: la fuente de verdad es `auto_handoff.hooks` en
  `ccp.yaml`, no el archivo generado.
- **Verja `allow_from`** (compliance, default deny). Rotar solo, de madrugada,
  puede acabar mandando la conversación de un cliente a una cuenta personal o a
  una API de terceros; las reglas de ruta son geográficas, no una declaración de
  confianza. Tres estados: ausente ⇒ sin verja · declarada con entrada para el
  primario ⇒ solo eso pasa · declarada **sin** entrada ⇒ deny total.
- **`ccp handoff prune [--keep N]`** y **`ccp handoff sessions [--json]`**. El
  historial archivado crecía una entrada por handoff cerrado y nada lo limpiaba.
- Documentación: sección de auto-handoff en ambos READMEs, capítulo runbook
  nuevo en los dos manuales SPA, e `index.html` convertido de redirect a hub
  bilingüe con tabla situación → comando.

### Changed

- `handoffs.yaml` gana `auto: true` y `hops: [...]` en los marcadores (ambos
  `omitempty`; la versión del schema no cambia). `ccp.yaml` gana el bloque
  `auto_handoff` y **sigue en schema `version: 2`**, así que un binario viejo
  conserva las claves nuevas en vez de rechazar el archivo.
- El supervisor **sí encadena** handoffs, mutando el marcador vivo en sitio
  (`To`/`Hops`) en vez de apilar un nivel, así que `handoff end` sigue volviendo
  a casa en un solo paso. Encadenar a mano sigue prohibido, igual que prestar
  una sesión a dos perfiles a la vez.
- El bloque de shell añade `prune|sessions` a la lista de passthrough de
  `handoff` — un rc instalado antes los reenviaba como si fueran un perfil
  destino. `ccp session` y `ccp auto` **no** necesitan reinstalarlo: llegan por
  la rama `*) command ccp "$@"` que ya existía.

### Fixed

- El watcher del transcript ya no rebobina con un archivo de tamaño cero. Una
  reescritura no atómica (truncar y escribir) exponía un instante vacío en el
  que el offset caía por debajo de la línea base, y la historia ya descartada se
  releía como un límite de ahora.

## [2.10.0] — renombrar perfiles

### Added

- **`ccp profile rename <viejo> <nuevo>`** (alias `mv`, y tecla `r` en el panel
  de perfiles del TUI). El nombre de un perfil vive en cuatro sitios —la clave
  de `profiles` y el destino de cada regla en `ccp.yaml`, el directorio con la
  api_key y el login, y el `from`/`to` de cada marcador de handoff—, así que el
  rename los mueve todos bajo el mismo criterio y regenera el overlay (el
  `CLAUDE.md` del cc-home lleva la ruta absoluta del perfil dentro).
  Renombrar a mano solo uno de los cuatro fallaba en silencio: una regla
  huérfana no da error, resuelve a `default`.
  El directorio se mueve **antes** de tocar `ccp.yaml` y vuelve a su sitio si la
  escritura falla; los nombres se validan como componente de ruta (nada de `/`
  ni `..`). Las completions bash/zsh completan `rename` y sus perfiles.

## [2.9.0] — handoff multi-activo

### Added

- **Varios handoffs en vuelo a la vez.** `handoffs.yaml` sube a `version: 2` con
  `active` como **lista** de marcadores (migración transparente desde el mapping
  de v1). `end`, `resume` y `discard` resuelven **por el directorio actual**: si
  hay uno solo aquí, ese; si hay varios, picker TUI (o `--session <uuid>` sin
  TTY); si no hay ninguno aquí, el error dice en qué repos sí los hay.
- **`ccp handoff resume [<uuid>]`**: vuelve a entrar a un handoff vivo sin
  cerrarlo — no copia transcripts ni toca el marcador. Es lo que hace útil tener
  varios abiertos.
- **`ccp handoff discard [<uuid>]`**: suelta un marcador zombi (el jsonl del
  destino desapareció) sin back-sync, el caso donde `end` falla siempre.
- **`ccp handoff` sin argumentos y con TTY**: panel gestor con los activos (los
  de este repo primero) — `enter` reanudar · `e` terminar (con confirmación) ·
  `n` nuevo · `y` toggle skip-permissions · `q` salir. Sin activos entra directo
  al wizard de handoff nuevo.
- **`--dangerously-skip-permissions` (alias `--yolo`)** en las tres operaciones
  que lanzan `claude`. No se recuerda entre invocaciones: ni en el marcador ni en
  `ccp.yaml`.
- **`ccp handoff status --all`** agrupado por repo; `list` marca los activos de
  este proyecto.
- **`ccp upgrade --from-source`** (y `CCP_FROM_SOURCE=1` en `install.sh`):
  reinstala compilando el repo registrado en vez de bajar el último release —
  para probar un cambio antes de tagearlo.

### Changed

- La función shell gana las ramas `resume` y `discard` y el `if` de
  `CCP_RESUME_YOLO`; las completions bash/zsh completan los subcomandos de
  handoff. Todo byte-idéntico con el oráculo bash y con el golden regenerado.
- Aviso no bloqueante a partir de 5 handoffs sin cerrar, y recordatorio de una
  línea al entrar (`cd`) a un repo con handoff activo.
- Exit codes de las operaciones de handoff: **0** ok, **1** pre-chequeo, **2**
  fallo de I/O al persistir `handoffs.yaml` (el 2 no se emitía nunca).

### Fixed

- `--session` se valida como uuid antes de tocar disco: un valor con `../`
  copiaba cualquier `.jsonl` legible al perfil destino y dejaba un marcador
  basura.
- Un `handoffs.yaml` escrito por un ccp más nuevo se detecta **antes** de copiar
  nada, y `status`/`list`/panel/hook lo dicen en vez de reportar «no hay handoff
  activo».
- `--force` llega hasta `CopyTranscript` (antes se parseaba y se ignoraba, así
  que el remedio que sugería el error de colisión no funcionaba).
- El picker de sesiones **marca** las que ya están en vuelo en vez de ocultarlas.
- `--no-marker` y `--force` valen también cuando el forward sale del panel.
- Un argumento posicional sobrante ya no se descarta en silencio.

Spec y plan: `docs/superpowers/{specs,plans}/2026-07-25-handoff-multi*`.

## [2.8.3] — parches de handoff e instalación

### Fixed

- `SlugForCwd` aplana **todo** carácter no alfanumérico, igual que Claude Code:
  con un cwd con `.` o `_` el slug no coincidía y ccp no encontraba las sesiones.
- `ccp install` sobre un rc que ya tiene el bloque pero **desfasado** lo reescribe
  en sitio en vez de ser un no-op: antes, una versión que cambiara la función de
  shell dejaba al usuario con la vieja para siempre.

## [2.8.0] — proveedores Kimi/GLM

### Added

- **Presets de proveedor para Kimi (Moonshot) y GLM (Z.ai)**: `ccp profile add
  <n> --kimi` / `--glm`. Cada preset rellena el `ANTHROPIC_BASE_URL` correcto,
  los modelos por defecto y las vars de tuning recomendadas por el proveedor
  (Kimi: `ENABLE_TOOL_SEARCH`, `CLAUDE_CODE_AUTO_COMPACT_WINDOW`; GLM:
  `API_TIMEOUT_MS`, `CLAUDE_CODE_AUTO_COMPACT_WINDOW`). Cualquier campo se
  sobreescribe con `--base-url --pro --flash --effort`.

## [2.7.0] — ccp handoff

### Added

- **`ccp handoff`**: continúa una conversación de Claude Code bajo otro perfil
  sin perder contexto, con tokens frescos del perfil destino. Persiste el
  transcript → cambia de perfil → reanuda la misma sesión en un proceso nuevo
  (`claude --resume`). TUI con pickers de perfil destino y de sesión; equivalente
  por flags (`ccp handoff <to> --session <uuid>`) para no-TTY/scripting.
- **`ccp handoff end`**: back-sync del contexto actualizado destino→origen como
  **sesión nueva** (no destructivo) y relanza en el origen; la sesión de vuelta
  muestra `[de <perfil>]` en su título.
- **`ccp handoff status` / `list`**: handoff en vuelo + historial archivado
  (solo lectura, funcionan sin la función shell).
- Marcador atómico (`handoffs.yaml`), warning no-bloqueante en handoff
  cross-provider, y el `case handoff)` añadido al shell-init byte-idéntico (gate
  de paridad). Ver spec/plan en `docs/superpowers/{specs,plans}/2026-06-19-ccp-handoff*`.

## [2.0.0] — rewrite a Go + cutover

### Changed (breaking en distribución, NO en el contrato observable)

- **Reescritura completa de Bash a Go** (`cmd/ccp` + `internal/{core,cli,tui}`). El
  contrato observable (`_env`, `_hook`, `resolve`, `path test`, `status --json`,
  `completion bash|zsh`, `completion-shellinit`) es **byte-idéntico** al bash,
  garantizado por el gate golden-diff Go↔bash (`internal/golden/parity_test.go`).
- **El bash quedó archivado en `legacy/`** como oráculo del contrato (binario,
  libs y suite de tests). El bloque rc no cambia (sigue llamando `command ccp`),
  así que actualizar NO requiere reinstalar el shell-init.
- **`ccp.yaml` es la fuente de verdad única** (reemplaza `profiles.tsv` +
  `rules.tsv` + `config` + los `meta` + `authored.tsv` global/profile). Schema
  `version: 2`, escritura atómica bajo `flock`, preserva comentarios y claves
  desconocidas. Secretos (`api_key`, OAuth) quedan **fuera** del YAML.
- **Migración automática y respaldada** dsctl→ccp(TSV)→`ccp.yaml` en el primer
  arranque Go (`.backup-pre-go-*`), idempotente y reversible.

### Added

- **`install.sh` Go-aware**: descarga el binario prebuilt por OS/arch del GitHub
  Release y verifica su `sha256` contra `checksums.txt`; fallback `go build` si
  hay toolchain. Limpia las libs bash viejas y re-apunta `install-source`.
- **Pipeline de release** (`.github/workflows/release.yml`): publica binarios
  darwin/linux × amd64/arm64 + `checksums.txt` en cada tag `v*`.
- **`ccp upgrade`** (Go): re-ejecuta `install.sh` + `profile sync` con el binario nuevo.
- **TUI** bubbletea+huh (3 paneles: Perfiles | Reglas | Estado) al correr `ccp`
  sin args con TTY; sin TTY cae a la CLI.
- **`ccp backup export|restore`** (`.tar.gz` + `manifest.yaml`, checksum por
  miembro, restore no-destructivo con snapshot previo).
- Comandos `/ccp:remember-{global,profile,project}`, `/ccp:recall`, `/ccp:forget`
  y la superficie CLI `ccp instruct add|list|rm|dest|record`: capturan artefactos
  (rule/agent/command/hook/mcp/skill) en la estructura oficial de Claude Code.
  Ver docs/adr/0004, 0005, 0006, 0007.

## [2.1.0]
- Autocompletado de shell (`dsctl completion bash|zsh`): subcomandos, llaves de
  config, rutas y verbos de `ds`. Se auto-carga vía el shell init.
- Cache en el hook `_ds_autocheck`: evita el fork de `dsctl _resolve` cuando el
  `$PWD` no cambió desde el último prompt.
- Salida machine-readable: `dsctl status --json`, exit codes en `dsctl path test`
  (0=deepseek, 1=official) y comando público `dsctl resolve`.
- `doctor` reporta si el autocompletado está cargado.
- Fix: `path list` no mostraba reglas en macOS (BSD grep no soporta `grep -P`).
  Reemplazado por filtrado con `read` (portable). El ruteo nunca estuvo afectado.

## [2.0.0]
- Gestión de paths con reglas `include` / `exclude` y precedencia por especificidad.
- Motor de resolución aislado en `lib/paths.sh` (testeable).
- Hook por carpeta basado en `dsctl _resolve "$PWD"`.
- Submenú interactivo de paths; `path list/test/edit/clear`.
- Soporte bash y zsh.

## [1.0.0]
- Versión inicial: función `ds on/off/run`, lista plana de auto-repos,
  key segura, config, doctor, menú.
