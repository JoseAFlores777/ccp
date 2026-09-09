# Changelog

## [Unreleased] — una instancia de Claude Desktop por perfil, con el Code tab aislado de verdad

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

- La statusLine mínima pasa de `emco-cc · 5h 14% · 7d 31%` a un **medidor con
  cuenta atrás**: `emco-cc  5h ▏█░░░░░░░░░▏ 2% ·2h13m  7d ▏██████░░░░▏ 59% ·3d`.
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
  `emco-cc · 31%` a `emco-cc · 5h 14% · 7d 31%`. Antes enseñaba solo el máximo
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
