<div align="center">

# ccp

**Perfiles para Claude Code.**

[English](README.md) · **Español**

**Una cuenta de Claude Code distinta en cada carpeta.**
En tu repo de trabajo, tu cuenta de empresa; en tu proyecto personal, la tuya; en tus experimentos, DeepSeek.
El cambio ocurre solo, con hacer `cd`.

![version](https://img.shields.io/badge/version-2.15.1-c96442)
![platform](https://img.shields.io/badge/platform-macOS%20%7C%20Linux-c96442)
![shell](https://img.shields.io/badge/shell-bash%20%7C%20zsh-8a8378)
![Go](https://img.shields.io/badge/Go-1.24-00ADD8?logo=go&logoColor=white)
![license](https://img.shields.io/badge/license-MIT-8a8378)

<img src="docs/screenshots/tui.png" alt="Dashboard interactivo de ccp" width="760">

</div>

---

## Qué es

`ccp` enruta Claude Code a un **perfil** por terminal y por carpeta — nunca global. Un perfil es una de tres cosas:

- una **cuenta oficial** de Anthropic (su propio `CLAUDE_CONFIG_DIR` aislado),
- un **proveedor** compatible (su `ANTHROPIC_BASE_URL` + API key) — presets built-in para **DeepSeek**, **Kimi** (Moonshot) y **GLM** (Z.ai), o
- el reservado **`default`**: tu login normal de `~/.claude`.

Así: repo A → cuenta *work*, repo B → cuenta *personal*, repo C → *deepseek*. Sin tocar nada a mano.

## El modelo mental (30 segundos)

1. Un **perfil** es una identidad (oficial, proveedor, o `default`).
2. Una **regla** dice "esta carpeta (y sus subcarpetas) usa tal perfil".
3. Gana la regla **más específica**. Sin regla → tu login normal.

```text
~/work          → perfil "work"      (cuenta empresa)
~/work/cliente  → perfil "default"   (carve-out: tu login normal)
~/personal      → perfil "personal"  (cuenta personal)
~/labs          → perfil "deepseek"
~               → default
```

Cuando entras a una carpeta, `ccp` aplica el perfil correcto en esa terminal mediante un hook. Luego corres `claude` normal.

## La interfaz

Corre `ccp` sin argumentos (con TTY) y obtienes el **dashboard interactivo** de la foto de arriba: tres paneles (Perfiles · Reglas · Estado) con navegación por teclado, indicadores de salud (`✓` login / key), y una barra de comandos `:` con **autocompletado** (Tab). Cada acción tiene su comando CLI equivalente.

Pulsa `c` (o `:config`) para la **vista Config**, que toma el cuerpo en vez de añadir un cuarto panel — a 80 columnas los tres actuales ya van justos. Cinco secciones: Defaults, Auto-handoff, Cadena (reordena con `J`/`K`), allow_from y Sensores. `e` abre la config entera en tu editor gráfico. La vista no reimplementa ninguna regla: editar la cadena desde ahí pasa por las mismas funciones de `core` que `ccp auto chain`, gate incluido.

Pulsa `e` sobre un perfil para su **vista de perfil**: qué configuración aplica de verdad, y de qué capa sale cada valor (global, overlay, o la capa de sensores del auto-handoff). Tres cajas — Instrucciones, Env y Efectivo (permisos, hooks, plugins, sensores, plegados a sus conteos; `enter` despliega uno). `a`/`d` editan reglas y variables por las mismas funciones de `core` que usa el CLI; los hooks se añaden igual pero no se pueden borrar desde acá — viven en arrays sin id estable, así que la tecla explica por qué en vez de fingir. `e` dentro de la vista abre solo el archivo de esa caja en tu editor.

¿Sin TTY o prefieres la terminal? Todo está en el CLI, con la misma paleta:

<div align="center">
<img src="docs/screenshots/cli-help.png" alt="ccp help — CLI coloreado" width="620">
</div>

---

## Antes de empezar

- macOS o Linux con **bash** o **zsh**.
- **Claude Code** instalado (que `claude --version` funcione).
- **git**.

## Instalación

Una línea, sin clonar nada:

```bash
curl -fsSL https://raw.githubusercontent.com/JoseAFlores777/ccp/main/install.sh | bash
```

Y después, una sola vez:

```bash
ccp install           # 2. función de shell + hook automático
source ~/.zshrc       # 3. recarga TU shell (o ~/.bashrc)
ccp doctor            # 4. confirma que quedó bien
```

Si el paso 1 te avisa que `~/.local/bin` no está en tu PATH, añádelo a tu rc:

```bash
export PATH="$HOME/.local/bin:$PATH"
```

<details>
<summary>Qué hace de verdad esa línea — y cómo fijar versión, cambiar rutas o leerla antes</summary>

El instalador **descarga el binario prebuilt** de tu OS/arch (darwin/linux ×
amd64/arm64) desde el GitHub Release y **verifica su sha256** contra el
`checksums.txt` del release; si no cuadra, aborta. Como no hay checkout
alrededor, además se trae el código a `~/.config/ccp/src`
(`git clone --depth 1`, o un tarball si no tienes git) — esa copia es de donde
salen los comandos `/ccp:` y lo que `ccp upgrade` re-ejecuta después.

Si prefieres leerlo antes de correrlo, que es el instinto correcto con
`curl | bash`:

```bash
curl -fsSL https://raw.githubusercontent.com/JoseAFlores777/ccp/main/install.sh -o install.sh
less install.sh && bash install.sh
```

Perillas de entorno (todas opcionales, todas válidas también desde un clon):

| Variable | Default | Qué hace |
|---|---|---|
| `CCP_RELEASE` | `latest` | Instala un tag concreto: `curl … \| CCP_RELEASE=v2.15.1 bash` |
| `CCP_BIN_DIR` | `~/.local/bin` | Dónde queda el binario |
| `CCP_SRC_DIR` | `~/.config/ccp/src` | Dónde queda la copia del código |
| `CCP_NO_SOURCE` | `0` | `1` = solo binario, sin copia del código (sin comandos `/ccp:`, sin fuente para `ccp upgrade`) |
| `CCP_FROM_SOURCE` | `0` | `1` = compila con `go build` en vez de usar el release (necesita Go) |
| `CCP_REPO` | `JoseAFlores777/ccp` | Instalar desde un fork |

Desde un clon es el mismo script y el mismo resultado — la única diferencia es
que `ccp upgrade` queda registrado contra **tu** checkout:

```bash
git clone https://github.com/JoseAFlores777/ccp.git && cd ccp && ./install.sh
```

</details>

## Actualizar

`install.sh` registra de qué fuente instalaste (`~/.config/ccp/src` con la línea
única, tu checkout si clonaste), así que actualizar es un comando:

```bash
ccp upgrade              # re-instala + re-sincroniza perfiles (profile sync)
ccp upgrade --pull       # hace 'git pull' antes de re-instalar
ccp upgrade --no-sync    # solo el binario, sin tocar perfiles
ccp upgrade --from-source  # compila el repo registrado en vez del release
```

`upgrade` instala el **último release** publicado en GitHub; `--from-source`
compila lo que haya en tu copia del repo (necesita Go), que es lo que quieres
para probar un cambio antes de tagearlo.

Cuando una versión cambia la **función de shell** (el bloque de tu rc), el
upgrade te avisa y hay que refrescarla — el binario nuevo por sí solo no puede
tocar tu shell:

```bash
ccp install && source ~/.zshrc   # reescribe el bloque desfasado en sitio
```

Si vienes de la versión Bash (`dsctl`) o de un `ccp` viejo, la **migración es automática y perezosa**: la primera vez que corras cualquier comando que toque la config, convierte tu estado a `ccp.yaml` (schema v2), respaldando antes en `~/.config/ccp/.backup-pre-go-<fecha>`.

---

## Uso

### Caso 1 — Tu cuenta de trabajo en tu carpeta de trabajo

```bash
ccp profile add work --official    # 1. crea el perfil oficial
ccp profile login work             # 2. una vez: dentro, /login y /quit
ccp path set ~/work work           # 3. asigna la carpeta
cd ~/work && claude                # 4. arranca con tu cuenta de trabajo
```

### Caso 2 — Añadir tu cuenta personal

Igual que el caso 1, con otro nombre y otra carpeta:

```bash
ccp profile add personal --official
ccp profile login personal
ccp path set ~/personal personal
```

### Caso 3 — Una carpeta con un proveedor compatible (DeepSeek / Kimi / GLM)

```bash
ccp profile add deepseek --deepseek   # perfil de proveedor (preset DeepSeek)
ccp profile add kimi     --kimi       # Kimi (Moonshot): base_url + modelos preconfigurados
ccp profile add glm      --glm        # GLM (Z.ai): base_url + modelos preconfigurados
ccp key deepseek                      # guarda la API key (te la pide oculta)
ccp path set ~/labs deepseek
cd ~/labs && claude
```

Cada preset rellena el `ANTHROPIC_BASE_URL` correcto, los modelos por defecto y
las vars de tuning recomendadas por el proveedor (Kimi: `ENABLE_TOOL_SEARCH`,
`CLAUDE_CODE_AUTO_COMPACT_WINDOW`; GLM: `API_TIMEOUT_MS`,
`CLAUDE_CODE_AUTO_COMPACT_WINDOW`). Override cualquier campo con
`--base-url --pro --flash --effort`.

### Caso 4 — Sacar una subcarpeta de su regla (carve-out)

Tu `~/work` usa la cuenta de trabajo, pero hay un cliente puntual donde quieres tu login normal:

```bash
ccp path set ~/work/cliente-x default
```

`~/work` sigue en "work", pero `~/work/cliente-x` usa tu login normal. La subcarpeta más específica siempre manda.

### Caso 5 — Cambiar a mano en una terminal

```bash
ccp use personal      # activa un perfil aquí
ccp default           # vuelve a tu login normal
ccp run claude        # corre Claude una vez con el perfil del cwd, sin fijarlo
```

### Caso 6 — Ver qué está pasando

```bash
ccp status            # perfil activo + perfil del cwd
ccp path list         # tus reglas de carpeta
ccp profile list      # tus perfiles
ccp doctor            # logins, keys, función de shell
```

---

## Handoff — continuar una sesión bajo otro perfil

Un proceso `claude` vivo congela sus credenciales al arrancar, así que `ccp use` no puede hot-swapearlas. Cuando un perfil se queda sin tokens/cuota a media conversación, `ccp handoff` hace lo único limpio: **persiste el contexto → cambia de perfil → reanuda la misma conversación** en un proceso nuevo con los tokens del perfil destino.

Puedes tener **varios handoffs en vuelo a la vez** — uno por sesión, en tantos repos como quieras.

```bash
ccp handoff                       # panel gestor: ves los activos y eliges qué hacer
ccp handoff <to>                  # nuevo handoff hacia <to> (picker de sesión)
ccp handoff <to> --session <uuid> # salta ambos pickers (scriptable)
ccp handoff resume  [<uuid>]      # vuelve a entrar a un handoff vivo, sin cerrarlo
ccp handoff end     [<uuid>]      # trae el contexto actualizado de vuelta al origen y reanuda ahí
ccp handoff discard [<uuid>]      # suelta un marcador sin traer nada de vuelta
ccp handoff status  [--all]       # qué hay en vuelo aquí (o en todos lados)
ccp handoff list                  # activos + historial (archivados)
```

El modelo mental: **pides prestados los tokens de otro perfil para una sesión, y devuelves el trabajo al volver.** `handoff end` hace back-sync del contexto actualizado al perfil origen como **sesión nueva** (no destructivo); la sesión de vuelta muestra `[de <perfil>]` en su título. `handoff resume` es lo contrario: vuelve a entrar a un handoff que sigue vivo sin copiar nada ni cerrarlo — es lo que hace útil tener varios en vuelo.

**Cómo eligen `end`/`resume`/`discard` cuál handoff:** por el directorio actual. Si hay exactamente uno activo en este repo, actúa sobre ese sin preguntar. Si hay varios, pregunta (picker de marcador con TTY; sin TTY falla pidiendo `--session <uuid>`). Si no hay ninguno aquí, falla y te dice en qué repos sí los hay. Pasar el `<uuid>` explícito se salta la resolución.

**Cuando el transcript ya no está — `handoff discard`:** si el jsonl de la sesión desapareció del perfil destino (limpiaste `~/.claude`, borraste el perfil…), `end` y `resume` no tienen con qué trabajar y fallan siempre, y el marcador se quedaría activo para siempre — secuestrando la resolución por directorio de ese repo, contando para el aviso de cinco handoffs y bloqueando un handoff nuevo desde el perfil destino. `ccp handoff discard` archiva ese marcador **sin back-sync**: no copia ni reescribe transcripts, no borra nada del perfil destino (lo que quede ahí sigue alcanzable con `claude --resume <uuid>` desde ese perfil) y no cambia el perfil de tu shell. No es la forma normal de cerrar un handoff — para eso está `end`, que sí trae el trabajo de vuelta.

**Saltarse los prompts de permiso:** las tres operaciones que lanzan `claude` (`handoff`, `handoff resume`, `handoff end`) aceptan `--dangerously-skip-permissions`, con alias `--yolo`: la sesión reanudada arranca sin los prompts de permiso de Claude Code. **No se recuerda entre invocaciones** — no vive en el marcador ni en `ccp.yaml`, así que se pide cada vez (o se activa con `y` en el panel).

`ccp handoff` sin argumentos y con TTY abre el **panel gestor**: los activos, los de este repo primero, con `enter` reanudar · `e` terminar (pide confirmación) · `n` nuevo · `y` toggle skip-permissions · `q` salir. Sin nada en vuelo el panel ni se abre: entras directo al wizard de handoff nuevo, perfil → sesión.

Los handoffs encadenados **a mano** siguen rechazados (`A → B → C` sobre la misma sesión — termina esa con `end` primero; prestar hacia adelante *otra* sesión del mismo repo sí se puede), y también prestar una misma sesión a dos perfiles a la vez. El supervisor de auto-handoff *sí* encadena (más abajo), pero lo hace sin apilar niveles. A partir de cinco handoffs sin cerrar, uno nuevo te avisa (no bloquea). Al entrar (`cd`) a un repo con handoff activo aparece un recordatorio de una línea. `status`, `list` y `discard` funcionan en cualquier lado y no lanzan nada — `handoff status` sale con `0` si este repo tiene handoff activo y `1` si no; los comandos que reanudan la sesión (`handoff`, `resume`, `end`) corren a través de la función shell de ccp, así que `ccp install` debe estar activo.

Dos subcomandos de mantenimiento lo rematan: `ccp handoff sessions [--json]` lista las sesiones de este directorio en el perfil activo (son los datos del picker, scriptables) y `ccp handoff prune [--keep N]` recorta el historial archivado — crece una entrada por handoff cerrado y nada las quitaba nunca (`--keep` por defecto 50; `--keep 0` lo vacía). Los dos son de solo lectura y no necesitan TTY, pero una función de shell instalada *antes* de que existieran los reenvía como si fueran un perfil destino — si ves un error diciéndolo, corre `ccp install` y abre una terminal nueva.

---

## Auto-handoff — rotar de perfil solo cuando se acaba el uso

`ccp handoff` es la respuesta manual a "esta cuenta se acabó". `ccp session` es la automática.

El problema que resuelve: una sesión larga (un refactor grande, un batch de madrugada) muere cuando la cuenta topa su límite de 5 horas, semanal o de Opus. Y un `claude` vivo **no puede** cambiar de perfil — `ANTHROPIC_BASE_URL`, `ANTHROPIC_AUTH_TOKEN` y `CLAUDE_CONFIG_DIR` se leen una sola vez al arrancar. Así que lo único limpio es un **supervisor** que corre `claude` como hijo, vigila el límite, presta la sesión a otro perfil y la relanza ahí.

```bash
ccp auto init                     # siembra el bloque auto_handoff en ccp.yaml
ccp auto install                  # instala los sensores en cada perfil no-default
ccp session --dry-run             # ¿qué haría aquí? (cadena, umbrales, muestras)
ccp session                       # interactivo: claude bajo el supervisor
ccp session -p --policy overnight -- "refactoriza el parser"   # headless (cron/CI)
ccp auto status [--json]          # política resuelta, sensores, últimas muestras, cooldowns
ccp auto test [--profile <n>]     # ¿está realmente cableada la detección?
```

`ccp session` llega al binario por la rama `*) command ccp "$@"` que la función de shell ya tiene, así que **no hace falta refrescar `ccp install`** para usarlo.

En la práctica puedes saltarte las dos primeras líneas: corre `ccp session` en un repo sin configurar y te lista lo que falta —la regla, la política, los sensores—, te enseña la ruta exacta que escribiría, y pregunta una vez. Si aceptas, cierra cada hueco por el mismo camino que el comando que habrías tecleado. Pregunta **una vez por repo**, contestes lo que contestes; `--setup` lo vuelve a ofrecer y `--no-setup` lo salta.

Nunca pregunta, ni escribe, si **los dos extremos de la conversación no son una terminal**. Con `-p`, sin TTY, o con la salida redirigida a un archivo, se limita a decir por qué y sigue como antes. `ccp session -p` corre desde cron: un prompt ahí cuelga el trabajo, y un prompt escrito en un log es peor todavía — nunca verías la pregunta que tu Enter contestó.

### El ciclo: primario → préstamo → vuelta al primario

El **primario** es lo que devuelva `ccp resolve $PWD` — el dueño natural del directorio. Cualquier otro perfil de la cadena es un **préstamo temporal**. El supervisor es un péndulo, no un round-robin: sale cuando el primario se agota y vuelve en cuanto la ventana del primario se reabre, aunque queden préstamos frescos. Trabajar horas en la cuenta equivocada es peor que esperar.

```
personal-1  ──[límite]──→  work-2  ──[límite]──→  personal-deepseek
                                      │
                                      └──[se reabrió la ventana del primario]──→ personal-1
```

La vuelta a casa no es un handoff al revés: es `handoff end`, que hace back-sync de la conversación al primario **como sesión nueva con uuid nuevo** (no destructivo, el transcript viejo se queda donde está). El supervisor imprime ese uuid nuevo — es el que le pasarías a `claude --resume` para seguir a mano. Cuando la cadena se seca para con código de salida **75** (`EX_TEMPFAIL`, "reintenta luego") y una tabla de cuándo se libera cada perfil; el marcador se deja vivo, así que no se pierde nada.

Hay un préstamo que no tiene marcador que cerrar: una rotación que disparó *antes* de que la conversación existiera (el sensor proactivo puede saltar en una sesión que aún no ha tenido su primer turno) no tiene nada que prestar, así que no abre handoff. Volver de una de esas no ejecuta `handoff end` en absoluto — si nació una conversación durante el préstamo, se **adopta** en el primario como sesión nueva; y si no nació ninguna, el primario simplemente arranca de cero. En ambos casos la traza dice cuál fue. Lo que nunca pasa es un marcador apuntando al revés (`{from: préstamo, to: primario}`), que mandaría la siguiente salida limpia — y a ti — a la cuenta equivocada.

Mientras la sesión está prestada, un **temporizador `return_check`** pregunta solo, cada N minutos, si la ventana del primario se reabrió — nadie tiene que topar un límite para que vuelvas a casa. Ese es justo el punto: la ventana de 5h del primario se reabre a las 3am, y ningún sensor dispara cuando se libera *otra* cuenta, así que sin el temporizador la corrida pasaría la noche en el préstamo. La vuelta respeta igual `min_dwell` (no te sacan de un préstamo al que llegaste hace 30 segundos), y el perfil que dejas **no** se marca agotado — lo dejaste voluntariamente, conserva su crédito:

```
p2 (2h 00m) ──[return_check: personal-1 ya liberó su ventana]──→ volviendo a personal-1 (vuelta a casa, no gasta préstamo: siguen 1/6)
```

**La vuelta a casa espera al silencio (`return_idle`, 90s por defecto).** Es el único movimiento que el supervisor hace por razones propias: rotar por un límite mata a un hijo que *ya no puede trabajar* (su cuenta responde 429), pero volver a casa mataría a uno que funciona bien. Solo con los defaults (`min_dwell: 20m` + `return_check: 10m`) cualquier préstamo de más de veinte minutos terminaría en el instante en que venciera el cooldown del primario — contigo tecleando, a mitad de un turno o con una tool call en vuelo, y una tool call interrumpida la re-ejecuta `--resume` y puede no ser idempotente. Por eso el temporizador exige además que la sesión esté **ociosa**: el transcript (que crece con cada turno y cada resultado de herramienta) no puede haber sido tocado en `return_idle`. Si nunca se calla, no se fuerza nada — el temporizador solo sigue ofreciendo; vuelves cuando paras, cuando el hijo sale por su cuenta o en el siguiente límite. Perder una oportunidad de volver es barato; quitarte la terminal a media frase no. Un transcript que no existe **no** cuenta como ocioso (no sabemos nada, y matar por ignorancia es justo lo que se evita), y `return_idle: 0s` es el opt-out explícito. La regla vale también en headless (`-p`): que no haya nadie mirando no hace menos frágil una tool call a medias.

```
ccp session: p1 ya liberó su ventana; la sesión sigue activa, se volverá cuando lleve 1m30s en silencio
```

**`max_hops` cuenta préstamos, no movimientos.** Volver a casa es *cerrar* un préstamo, así que ni gasta presupuesto ni lo bloquea uno agotado — si no, `max_hops: 6` significaría "tres viajes de ida y vuelta" y un presupuesto gastado dejaría la conversación varada en la cuenta de otro con el primario libre. Sigue siendo un tope anti-bucle duro: toda vuelta a casa tiene que venir precedida de un préstamo, y ese sí paga, así que una corrida nunca puede hacer más de `2 × max_hops + 1` lanzamientos.

### Configurarlo (`auto_handoff` en `ccp.yaml`)

`ccp auto init` lo siembra a partir de tus perfiles; después edítalo a mano.

```yaml
auto_handoff:
  enabled: true              # interruptor maestro: false = ccp session se niega a correr
  hooks: [personal-1, work-2]    # perfiles con la capa de sensores instalada
                                 # (la gestiona `ccp auto install/uninstall`)
  policies:
    default:
      # Préstamos, en orden de preferencia. El primario es IMPLÍCITO (ccp resolve $PWD)
      # y se descarta en silencio si lo listas aquí.
      fallback: [work-2, personal-deepseek]
      threshold: 90          # % de la ventana de uso que dispara un salto proactivo
      min_dwell: 20m         # tiempo mínimo antes de rotar (entero solo para el sensor proactivo)
      max_hops: 6            # tope duro de PRÉSTAMOS por corrida (backstop anti-bucle;
                             # volver a casa es gratis, ver arriba)
      return_check: 10m      # cada cuánto reconsiderar el primario estando prestado
      return_idle: 90s       # …y cuánto tiempo debe llevar la sesión EN SILENCIO
                             # para que esa vuelta proactiva pueda matar al hijo
                             # (0s desactiva el guard: vuelve aunque sea a media frase)
      cooldown:
        strategy: resets_at  # usa el resets_at que reporta la API (suscripciones)
        fallback: 1h         # …o esta espera fija cuando no hay resets_at

    overnight:               # `ccp session -p --policy overnight`
      fallback: [personal-1, work-2]
      threshold: 85
      max_hops: 12

    work:
      fallback: []           # sin préstamos: si el primario muere, la corrida para

  allow_from:                # verja de cumplimiento (ver abajo)
    work-1: [work-1]                                   # nunca rota
    work-2: [work-2, personal-1]
    personal-1: [personal-1, work-2, personal-deepseek]
```

> **Qué hace de verdad el temporizador `return_check` — no es un reloj pasivo.** Solo se arma mientras la sesión está **prestada**: tiene que haber un marcador de handoff vivo cuyo origen sea el primario (una rotación *degradada* — la que ocurrió antes de que existiera transcript y por eso nunca abrió marcador — no cuenta), más `--no-return` apagado y `return_check > 0`. Ya armado, re-decide una vez por periodo, y cada decisión es barata: unas comparaciones y un `stat` del transcript. Lo que *no* es barato es lo que pasa cuando la decisión sale "a casa" — su única acción es **mandarle `SIGTERM` al `claude` vivo** (10s de gracia para que vacíe su `.jsonl` y corra sus hooks `SessionEnd`), cerrar el préstamo y **relanzar** `claude --resume` en el primario con el uuid nuevo. Eso es un proceso muerto y un hijo nuevo, no una comparación de timestamps. De ahí las cuatro condiciones que exige antes de disparar, todas: préstamo vivo · cooldown del primario vencido · `min_dwell` ya cumplido en el perfil actual · sesión **ociosa** durante `return_idle`. Un evento de límite ya encolado por un sensor también le gana (rotar en su lugar conserva el cooldown del perfil que se deja). Si falta cualquier condición, simplemente espera y vuelve a preguntar al periodo siguiente — nunca fuerza el movimiento. Desactívalo con `return_check: 0s` (entonces vuelves a casa en el siguiente evento de límite, como antes) o con `--no-return`.
>
> `--no-return` apaga la vuelta a casa **a media sesión** — el temporizador y la regla de péndulo de `Next()` —, no la limpieza al final de la corrida. Cuando el hijo por fin sale con 0 y queda un préstamo abierto, el préstamo se cierra siempre y la conversación aterriza de vuelta en el primario con un uuid nuevo, con `--no-return` o sin él. Es deliberado: la alternativa es una conversación terminada, varada en una cuenta prestada, detrás de un marcador que tienes que acordarte de cerrar mañana con `ccp handoff end`, mientras tu repo sigue resolviendo al perfil que la prestó. Si *quieres* que se quede ahí, termina la corrida con Ctrl-C (el exit 130 deja el marcador vivo a propósito) o suelta el marcador luego con `ccp handoff discard`.

**Por qué existe `allow_from`:** rotar solo, a las 3am, sin nadie mirando, significa que la conversación de un cliente podría acabar en una cuenta personal — o en la API de un proveedor externo. Las reglas de ruta son *geográficas* (qué carpeta es de quién), no una declaración de confianza, así que la verja va aparte y explícita. La regla exacta:

| `allow_from` | Efecto |
|---|---|
| ausente o vacío | **sin verja** — se permite toda la cadena `fallback` |
| declarado, con entrada para el primario | solo se permiten los perfiles de esa entrada; el resto salen como *denegados* |
| declarado, **sin** entrada para el primario | **deny total** — ningún préstamo |

Esa última fila es el punto: declarar el mapa es declarar la intención de gobernar los préstamos, así que un perfil que se te olvidó añadir se queda quieto en vez de heredar barra libre. `ccp session --dry-run` y `ccp auto status` imprimen los dos qué se denegó y por qué.

> **La fila con la que topas primero.** `ccp auto init` siembra `allow_from` con una entrada por perfil *con nombre*, y `default` nunca es una de ellas. Así que en un directorio sin regla de ruta el primario es `default`, no hay entrada para él, y la cadena entera sale denegada — `ccp session` corre igual, solo que no tiene a dónde saltar cuando llegue el límite. O pones una regla (`ccp path set . <perfil>`) o añades tú una entrada `default:` a `allow_from`. `--dry-run` lo enseña de inmediato: `chain: (empty)` con todo bajo *denegados*.

### Editar la cadena — añadir, reordenar o quitar un préstamo

```bash
ccp auto chain                        # la cadena EFECTIVA de este directorio
ccp auto chain add personal-deepseek  # lo añade al final, y autoriza el préstamo
ccp auto chain add work-2 --at 1      # lo inserta en una posición (1-based)
ccp auto chain mv work-2 2            # reordena — el orden ES la preferencia
ccp auto chain rm personal-deepseek   # lo saca, y retira la autorización
ccp auto chain set work-1,work-2      # reemplaza la cadena entera
```

Todos aceptan `--policy <nombre>` (por defecto: `default`) y actúan sobre el primario del **directorio actual**, que es el único cuya entrada de `allow_from` pueden tocar.

**`add` escribe dos claves, y eso es justo el punto.** Quien escribe «añádelo a la cadena» quiere que el perfil *se use*, y un `fallback` sin su `allow_from` no se usa jamás, en silencio. Dejar las dos claves separadas en la CLI reproduciría la trampa que el yaml ya tiende. Pero `allow_from` es un **gate de cumplimiento** y puede estar puesto a propósito, así que el ensanche va con tres límites duros:

1. Toca **solo** la entrada del primario al que resuelve el directorio actual. Nunca la de otro — un rewrite en bloque ensancharía permisos de repos donde ni siquiera estás.
2. Imprime **exactamente qué cambió en cada clave, por separado**. `+nombre` autoriza, `-nombre` retira.
3. `--no-allow` lo desactiva.

`rm` (y los perfiles que `set` deja fuera) va en la dirección contraria y **retira** la autorización, otra vez solo de esa entrada. Eso es lo que hace reversible a `add`: sin ello el gate solo podría crecer, y deshacer un `add` equivocado exigiría editar el yaml a mano — exactamente lo que estos comandos vienen a evitar.

```console
$ ccp auto chain add personal-deepseek

[ok] fallback   work-1 → work-2 → personal-deepseek
[ok] allow_from personal-1: +personal-deepseek

cadena efectiva desde este repo:
  work-1 → work-2 → personal-deepseek
```

Toda mutación cierra releyendo la cadena efectiva **del disco**, así que ves lo que va a ver el motor — incluido lo que `allow_from` siga bloqueando.

| Lo que quieres | Comando |
|---|---|
| Añadir un préstamo | `ccp auto chain add <perfil>` (escribe `fallback` **y** `allow_from[<primario>]`) |
| Cambiar el orden de preferencia | `ccp auto chain mv <perfil> <pos>` — el **orden es la preferencia** |
| Quitar un préstamo | `ccp auto chain rm <perfil>` (retira también la autorización; `--no-allow` la conserva) |
| Dejar de rotar en un directorio | `ccp auto chain rm` de cada préstamo, o dale a ese primario una política con `fallback: []` |
| Otra cadena para otro trabajo | añade otra política bajo `policies:` y usa `--policy <nombre>` |

Editar el archivo a mano sigue siendo perfectamente válido, y es como se añade una política nueva entera. `ccp` reescribe `~/.config/ccp/ccp.yaml` de forma atómica **conservando tus comentarios y cualquier clave que no conozca**, así que una anotación sobre por qué un perfil está en la lista sobrevive a cada `ccp path set`, `profile add` o `auto install` posterior.

```yaml
auto_handoff:
  policies:
    default:
      # El orden ES la preferencia: primero work-2, el proveedor solo como último recurso.
      fallback: [work-2, personal-deepseek]
  allow_from:
    personal-1: [personal-1, work-2, personal-deepseek]   # ← la entrada del primario
```

Las reglas que el resolver aplica a `fallback`, en una sola pasada: el **primario es implícito** y se descarta en silencio si lo listas; los duplicados se eliminan (no compran un préstamo extra); `default` es un destino legítimo (es tu login normal de `~/.claude`); y el orden que escribiste se respeta tal cual.

> **El único comando que te saca de un deny total.** En un directorio cuyo primario no tiene entrada en `allow_from`, `ccp auto chain show` dice `cadena (ninguno)` mientras todos los perfiles están ya dentro de `fallback` (es lo que siembra `ccp auto init`). `ccp auto chain add <perfil>` lo resuelve: un perfil que ya está en la cadena pero al que el gate cierra el paso *no* se trata como duplicado — se crea la entrada, se avisa de que la cadena en sí no cambió, y el repo empieza a rotar.

Después, los dos pasos de verificación — estar en la cadena **no** es lo mismo que tener detección:

```bash
ccp auto install personal-deepseek    # sensores para el perfil recién añadido
ccp session --dry-run                 # a qué resuelve la cadena, aquí
```

```
política: default
primario: work (cwd /home/yo/repo)
cadena de préstamos: work-2 → deepseek
umbral 90% · min_dwell 20m0s · max_hops 6 · return_check 10m0s · return_idle 1m30s · cooldown resets_at (respaldo 1h0m0s)
```

Si solo editaste `fallback` y olvidaste `allow_from`, `--dry-run` te lo dice en vez de no hacer nada en silencio — es el error más común de largo:

```
cadena de préstamos: (vacía)
denegados por allow_from: work-2, deepseek
```

**Un nombre que no existe es un error duro, no un salto silencioso.** Un typo — o un perfil que borraste — detiene `ccp session` antes de lanzar nada, con exit `1`:

```
[error] política "default": el perfil de fallback "app" no existe
```

> **`ccp profile rename` y `ccp profile rm` no tocan `auto_handoff`.** Mueven tus reglas de ruta y tus marcadores de handoff, pero la política de rotación se queda exactamente como estaba — así que renombrar o borrar un perfil que está en una cadena deja un nombre colgando, y el siguiente `ccp session` en ese directorio falla con el error de arriba hasta que arregles el YAML. Tres sitios llevan nombres de perfil: `policies.*.fallback`, `allow_from` (**tanto** las claves como las listas) y `hooks`. Busca el nombre viejo antes de cerrar el archivo.

> **`ccp auto init --force` regenera el bloque desde cero**, así que descarta tus ediciones a mano *y* tus comentarios. Úsalo para empezar de nuevo, no para refrescar. El `ccp auto init` pelado es idempotente: con un bloque ya presente no hace nada.

Ojo a que `ccp auto init` deja a propósito los perfiles de proveedor (DeepSeek/Kimi/GLM) fuera del `allow_from` sembrado de *todos los demás* perfiles. Mandar una conversación a una API de terceros es justo la decisión que la verja existe para hacer explícita, así que añadir uno a una cadena siempre es un acto manual.

### Los sensores — tres a la vez, cuatro en total

Tres sensores corren a la vez en cada modo, porque ninguno lo cubre todo. La redundancia es deliberada: detectar el mismo límite dos veces es gratis (el supervisor deduplica por contenido), estar ciego no.

| Sensor | Qué lee | ¿Proactivo? | Interactivo | Headless |
|---|---|---|---|---|
| **statusLine** (`ccp _statusline`) | los `rate_limits.{five_hour,seven_day}.used_percentage` que Claude Code le pasa a la barra de estado; `<cc-home>/.claude.json` como muestra de respaldo | ✅ dispara al `threshold` % **antes** de que falle un turno | ✅ | ❌ (sin TTY no hay barra de estado) |
| **stream-json** | los eventos `api_retry` / `error: "rate_limit"` de `claude -p --output-format stream-json` | ❌ reactivo | ❌ (no hay nada que parsear) | ✅ |
| **cola del transcript** | el `.jsonl` de la sesión, buscando `"error":"rate_limit"` / `apiErrorStatus: 429` | ❌ reactivo | ✅ | ✅ |
| **hook StopFailure** (`ccp _limit-hook`) | el payload que manda Claude Code cuando un turno muere; deja un sentinel en `~/.config/ccp/state/auto/sentinels/` | ❌ reactivo | ⚠️ sin verificar (ver abajo) | ✅ |

O sea: **interactivo** corre statusLine + transcript + sentinel; **headless** corre stream-json + transcript + sentinel. Solo el sensor de statusLine es proactivo, y es justo el que *no* funciona en headless — por eso el camino headless se apoya en el (muy limpio) evento `api_retry`.

⚠️ **Todavía sin verificar empíricamente:** si el hook `StopFailure` dispara siquiera en interactivo con un límite de *suscripción* (ahí Claude Code no termina el turno — ofrece `/rate-limit-options`). Si no dispara, la detección interactiva descansa en el porcentaje de la statusLine más la cola del transcript. `ccp auto test` verifica el cableado, no el comportamiento de Claude Code.

### `ccp auto install` toca el `settings.json` de tus perfiles

Los dos sensores in-process solo se pueden encender desde `cc-home/settings.json`, y ese archivo es **generado** (global ⊕ overlay). Así que `ccp auto install <perfil>` añade el perfil a `auto_handoff.hooks` y regenera: el merge pasa a ser global ⊕ overlay ⊕ **capa auto**, que añade

- `hooks.StopFailure` → `ccp _limit-hook`
- `statusLine` → `ccp _statusline -- <tu statusLine original>` (la tuya se **envuelve**, no se reemplaza — sigue pintando tu barra; ccp solo muestrea el stdin que le llega)

Si **no** tenías statusLine propia, ccp pinta una mínima en su lugar: el perfil más las dos ventanas de uso, cada una con su medidor y su cuenta atrás hasta el reset — `work-1  5h ▏█░░░░░░░░░▏ 2% ·2h13m  7d ▏██████░░░░▏ 59% ·3d`. Se enseñan las dos porque las dos se vigilan por separado (dispara el salto la primera que cruce el `threshold`), y un porcentaje suelto no diría si te quedan horas o días.

El medidor va verde por debajo del 70%, ámbar entre 70 y 89, y rojo del 90 en adelante — el rojo empieza exactamente en el default de `threshold`, así que la barra y el motor nunca cuentan historias distintas. Con `NO_COLOR` se va el tinte y el medidor se sigue leyendo.

Tres cosas que la barra se calla a propósito. Una ventana **sin dato** se omite en vez de pintarse como `0%`. Un `resets_at` que ya pasó **no produce cuenta atrás**: un dato caducado no puede afirmar que tu cuota volvió. Y los días redondean **hacia arriba** — `·3d` cuando faltan 2d23h, nunca `·2d`, porque lo único que una cuenta atrás no puede hacer es prometer que la cuota vuelve antes de lo que va a volver.

La línea se dimensiona sola: ccp la monta a tres niveles de detalle, los mide y pinta el más ancho que quepa. Un nombre de perfil largo cuesta celdas de medidor, no corrección — y si no cabe ninguno, recibes la forma compacta sin recortar, porque lo primero que se comería el recorte es el nombre del perfil.

Cualquier hook `StopFailure` que ya tuvieras se conserva junto al nuestro. Es totalmente reversible: `ccp auto uninstall <perfil>` lo quita de la lista y regenera de vuelta a global ⊕ overlay. Tu overlay no se modifica en ningún caso — la fuente de verdad de "quién tiene los sensores" es `auto_handoff.hooks` en `ccp.yaml`, no el archivo generado.

> La capa graba la **ruta absoluta** del binario `ccp`. Si lo mueves sin correr `ccp upgrade` (o `ccp auto install` / `ccp profile sync`), los sensores se quedan apuntando a la ruta vieja.

### Una sesión, y su traza

```
$ ccp session
# primario = personal-1 (regla de ruta de ~/Documents/Personal)

▶ personal-1 · sesión nueva 3f9c1a2b                                    (stderr)
personal-1 (4h 03m) ──[uso 94% ≥ umbral 90% (ventana session) · statusline]──→ handoff a work-2 (préstamo 1/6)
▶ work-2 · reanudando 3f9c1a2b                                          (stderr)
work-2 (1h 12m) ──[You've hit your session limit · transcript]──→ volviendo a personal-1 (vuelta a casa, no gasta préstamo: siguen 1/6)
sesión devuelta a personal-1 como 7d0e44f1
▶ personal-1 · reanudando 7d0e44f1                                      (stderr)
personal-1 ──[termina]──→ ✅ exit 0
```

**Cómo leer el contador.** Todo movimiento acaba en el mismo presupuesto, dicho de una sola manera: un movimiento de ida es `préstamo N/6` (este es el préstamo N de tus `max_hops`), y una vuelta a casa es `vuelta a casa, no gasta préstamo: siguen N/6`. El número *no* sube al volver — volver es cerrar un préstamo, no abrir uno nuevo (ver `max_hops` arriba) — así que la línea lo dice en voz alta en vez de dejarte adivinando por qué dos movimientos seguidos muestran `1/6`. A la vuelta a casa nunca se le llama *préstamo*, la haya causado lo que la haya causado (un límite en el perfil prestado o el temporizador `return_check`): es el mismo suceso, así que lleva las mismas palabras.

La traza (saltos, vuelta a casa, tabla de cooldowns) va a **stdout** porque *es* la salida del comando — es lo que acaba en el log del cron. La cháchara operativa (`▶ lanzando…`, "esperando min_dwell") va a **stderr**, para que en headless stdout siga siendo parseable: ahí lleva el `stream-json` del hijo tal cual.

Códigos de salida: `0` ok · `1` error de uso/config · `2` fallo de E/S del handoff · `75` todos los perfiles agotados (reintenta luego) · cualquier otro es el código del propio claude, así que `ccp session -p …` entra en un script donde antes iba `claude -p …`.

**Algunos filos que conviene conocer.** `--yolo` (`--dangerously-skip-permissions`) es prácticamente obligatorio para corridas desatendidas, y nunca se persiste — se pide cada vez. Un salto es un `SIGTERM` en frontera de turno, así que una tool call interrumpida la re-ejecuta `--resume` y puede no ser idempotente. `Ctrl-C` (exit 130) nunca rota: el préstamo se deja abierto y se te dice. Y `min_dwell` se aplica **entero solo al sensor proactivo** — el único que avisa antes de que nada haya fallado. Un evento reactivo significa que el turno ya falló, así que esperar veinte minutos dentro de una cuenta que responde 429 sería daño puro: esos rotan tras un piso corto de 30s, que sigue evitando que tres perfiles se quemen en un minuto. La vuelta a casa es la excepción y exige el valor completo: ahí nadie tiene prisa.

---

## Claude Desktop — una ventana por perfil

`ccp desktop` abre una instancia de **Claude Desktop** aislada por perfil: cuenta, servidores MCP,
entorno Cowork… y el Code tab. Puedes tener la de trabajo y la personal abiertas a la vez.

```bash
ccp desktop open work-1            # lanza la instancia del perfil 'work-1'
ccp desktop open                   # sin perfil: el que resuelva la carpeta actual
ccp desktop list                   # instancias en disco y su tamaño
ccp desktop open work-1 --dry-run  # imprime el plan sin lanzar ni tocar nada
```

Funciona **sin reinstalar el rc** (`ccp install` solo hace falta para el autocompletado).

### El aislamiento son dos cosas, y la segunda es la que no se ve

`--user-data-dir` mueve la identidad de la app: sesión, tokens, `claude_desktop_config.json` (MCP) y
el estado de Cowork. Eso es la mitad.

La otra mitad es el **Code tab**, que *no* lee ese directorio: lee `CLAUDE_CONFIG_DIR` del entorno del
proceso Desktop, y cae a `~/.claude` si está vacío. Sin inyectarlo, dos ventanas con cuentas distintas
compartirían credenciales de CLI, `projects/`, historial y agentes. `ccp desktop` emite las dos a la
vez, así que la ventana entera —chat y Code tab— queda en el mismo perfil que tu terminal.

### Qué cambia en el `cc-home` del perfil

Desktop **rechaza symlinks de directorio** bajo su config root, y `ccp profile add` siembra justo eso
(`cc-home/commands -> ~/.claude/commands`). La primera vez que abres una instancia, `ccp` convierte
esas entradas en directorios **reales** cuyos archivos son symlinks al global:

- sigues compartiendo comandos y plugins en vivo con `~/.claude`;
- `agents/` y `skills/` pasan a ser del perfil, así que **cada perfil puede tener sus propios agentes**;
- un archivo tuyo dentro del perfil gana sobre el global y no se pisa al re-espejar.

Si añades archivos nuevos al global, vuelve a espejarlos con `ccp desktop prepare <perfil>`.

### Límites

- Solo perfiles **`official`** y **`default`**. Claude Desktop no lee `ANTHROPIC_BASE_URL`, así que un
  perfil DeepSeek/Kimi/GLM se rechaza en vez de dejarte una ventana a medias.
- **`default` no se reubica**: su instancia es la de siempre, donde siempre.
- Cada instancia se baja su propia copia del Claude Code embebido (~190 MB), más el entorno de Cowork
  si lo usas. `ccp desktop list` te dice cuánto ocupa cada una.
- **Inicia sesión de una en una**: los enlaces `claude://` van a la instancia que registró el esquema
  de último, así que con varias ventanas abiertas el login puede aterrizar en la equivocada. `ccp` te
  lo recuerda en el primer arranque de cada instancia.
- `ccp desktop rm <perfil> --yes` borra la instancia: es un logout que se lleva sesión, tokens y MCP.

## Backup y restore

Llévate todo a otra máquina, o respáldalo antes de un cambio grande:

```bash
ccp backup export ~/ccp-backup.tar.gz                 # ccp.yaml + overlays
ccp backup export ~/ccp-backup.tar.gz --with-secrets  # + api_key + logins (chmod 600)
ccp backup restore ~/ccp-backup.tar.gz                # no pisa; fusiona reglas
ccp backup restore ~/ccp-backup.tar.gz --overwrite    # reemplaza perfiles del backup
ccp backup restore ~/ccp-backup.tar.gz --force        # borra todo y restaura limpio
```

Antes de restaurar, `ccp` guarda un snapshot automático en `~/.config/ccp/.backup-pre-restore-<fecha>`.

## Config por perfil

Cada perfil tiene su propia config de Claude, aplicada como **capa baseline** cuando está activo:

```bash
ccp profile config <perfil>                 # menú: instrucciones / settings / ambos
ccp profile config <perfil> instructions    # abre overlay/CLAUDE.md
ccp profile config <perfil> settings         # abre overlay/settings.overlay.json
ccp profile sync [<perfil>]                  # re-mergea cambios del global ~/.claude
ccp profile rename <viejo> <nuevo>           # renombra: reglas, marcadores, login y key incluidos
ccp config editor "code -w"                  # editor a usar (fallback: $EDITOR)
```

- **Instrucciones**: `cc-home/CLAUDE.md` hace `@import` del global `~/.claude/CLAUDE.md` y luego de tu overlay.
- **Settings**: `cc-home/settings.json` = global ⊕ overlay (deep-merge puro en Go).
- **Prioridad real**: es una baseline — la config del repo (`.claude/settings.json`) gana en conflicto.
- `default` no tiene overlay: `ccp profile config default` abre tu `~/.claude` global directo.

### Editar `ccp.yaml` — `ccp config edit`

```bash
ccp config edit                     # abre ~/.config/ccp/ccp.yaml, y al cerrar lo relee y valida
ccp config edit --profile personal-1   # abre el overlay de ese perfil
ccp config edit --terminal          # fuerza el editor de terminal ($EDITOR / nano)
ccp config gui-editor "code -w"     # fija el editor gráfico de una vez
```

El editor es el **primer escalón que exista**: `--editor <cmd>` → `defaults.gui_editor` → `$VISUAL` → VS Code y familia en el `PATH` (`code` → `cursor` → `code-insiders`, invocados con `-w`) → lanzador del SO (`open -W -t`, `notepad`, `xdg-open`) → `defaults.editor` / `$EDITOR` / `nano`. El comando te dice qué escalón ganó.

**El `-w` ES la decisión, no un detalle.** `code archivo` retorna al instante, y sin esperar a que el editor cierre no se puede hacer lo único que da valor real al comando: **releer el yaml y validarlo**. Un `ccp.yaml` roto por una edición gráfica no se manifiesta al guardar — se manifiesta en el siguiente `ccp session`, en mitad de un salto, con un error de parseo a las 3am. Por eso, cuando el editor bloquea, `ccp` recarga el archivo y comprueba la semántica de `auto_handoff` (cada política por `Effective()`, cada perfil del fallback existiendo de verdad) y nombra la clave y el valor ofensivos si falla.

Cuando el editor **no** bloquea (`xdg-open`, o `code` sin `-w`) esa validación es imposible, y `ccp` lo avisa en vez de fingir que sí — también en la ruta `--profile`, donde además no regenera el `cc-home` y te manda a `ccp profile sync <perfil>` para cuando termines.

## Comandos `/ccp:` — recordar y explorar artefactos

`ccp` incluye cinco comandos de Claude Code para persistir instrucciones, agents, hooks y MCP servers directo desde la conversación, sin editar archivos a mano:

| Comando | Qué hace |
|---|---|
| `/ccp:remember-global <texto>` | Persiste al `~/.claude` global (todos los perfiles) |
| `/ccp:remember-profile <texto>` | Persiste al overlay del perfil activo |
| `/ccp:remember-project <texto>` | Persiste al `.claude/` del repo git actual (versionado) |
| `/ccp:recall [scope]` | Lista lo que ccp gestiona (`global` · `profile` · `project`) |
| `/ccp:forget [scope]` | Borra por índice (lista y confirma antes) |

Se instalan con `install.sh` en `~/.claude/commands/ccp/` y quedan disponibles en todos los perfiles. La superficie CLI equivalente es `ccp instruct <add\|list\|rm\|dest\|record>`.

---

## Idioma

`ccp` habla inglés por defecto y también español. Elige el que prefieras; la elección se persiste en `ccp.yaml`.

```bash
ccp lang              # muestra el idioma actual + de dónde sale (env/config/default)
ccp lang en           # cambia a inglés y lo persiste en ccp.yaml
ccp lang es           # cambia a español y lo persiste en ccp.yaml
```

- `CCP_LANG=en|es` — override por entorno; tiene prioridad sobre la config. El valor por defecto es inglés.
- En el dashboard interactivo (TUI), pulsa **`L`** para cambiar el idioma en vivo (se persiste). Ojo: en el TUI, **iniciar sesión** (login) está en la **`l`** minúscula, y **`L`** (mayúscula) cambia el idioma.

### Atajos de teclado del TUI

| Tecla | Acción |
|---|---|
| `j` / `k` (o ↓/↑) | Mover la selección |
| `Tab` / `Shift+Tab` | Cambiar de panel (Perfiles / Reglas / Estado) |
| `Enter` | Mostrar/ocultar detalle (Perfiles) |
| `a` | Añadir |
| `d` | Borrar |
| `s` | Guardar key (DeepSeek) |
| `e` | Abrir vista de perfil |
| `e` | Editar archivo de la caja enfocada (vista de perfil) |
| `l` | Iniciar sesión (oficial) |
| `L` | Cambiar idioma (EN/ES) |
| `:` | Barra de comandos |
| `q` / `Ctrl+C` | Salir |

---

## Configurar a mano (`ccp.yaml`)

Todo vive en `~/.config/ccp/ccp.yaml` (o `$CCP_HOME/ccp.yaml`). Puedes tocarlo con comandos o a mano:

```yaml
version: 2
defaults:                  # plantilla para perfiles deepseek NUEVOS
  base_url: https://api.deepseek.com/anthropic
  model_pro: deepseek-chat
  model_flash: deepseek-chat
  effort: high
  editor: nano
profiles:
  work:                    # oficial: solo 'type'
    type: official
  deepseek:                # proveedor: los 4 campos, explícitos
    type: deepseek
    base_url: https://api.deepseek.com/anthropic
    model_pro: deepseek-chat
    model_flash: deepseek-chat
    effort: high
rules:                     # carpeta → perfil (ruta absoluta)
  - path: /Users/tu/work
    profile: work
  - path: /Users/tu/work/cliente-x
    profile: default       # carve-out
authored: []
```

- `default` es **implícito**: nunca lo pongas en `profiles`. Para una excepción, usa `profile: default` en una regla.
- La API key **no** va aquí: vive en `~/.config/ccp/profiles/<n>/api_key` (`chmod 600`). Edítala con `ccp key <n>`.
- `ccp` escribe atómico bajo un `flock`, conserva tus comentarios, y aborta si el `version` es mayor que el que conoce.

Con comandos: `ccp config show` · `ccp config set <clave> <valor>` · `ccp config reset`.

---

## Solución de problemas

| Síntoma | Arreglo |
|---|---|
| `ccp: command not found` | `~/.local/bin` no está en tu PATH (paso 2 de Instalación). |
| `ccp use …` no cambia nada | Falta la función de shell: `ccp install` y luego `source ~/.zshrc`. |
| Cambié de carpeta y el perfil no cambió | El hook recuerda la última carpeta; refresca con `cd .` |
| Al abrir Claude dice "Not logged in" | Ese perfil oficial no tiene sesión: `ccp profile login <n>`. |
| Ver el perfil de una carpeta sin entrar | `ccp resolve ~/ruta/que/sea` |
| ¿Cómo cambio el idioma de la salida? | `ccp lang en\|es`, `CCP_LANG=es`, o pulsa `L` en el TUI. |
| ¿ccp cambia algo dentro de Claude Code? | No. Solo apunta Claude Code a un perfil por carpeta (su propio config dir / proveedor). Tus cuentas y ajustes quedan intactos. |
| ¿Dónde se guardan mis API keys? | En `~/.config/ccp/profiles/<n>/api_key`, `chmod 600`. Nunca en `ccp.yaml`, en el rc del shell, ni en git. |
| ¿Cómo renombro un perfil? | `ccp profile rename <viejo> <nuevo>` (o `r` en el TUI). Se mueven con él sus reglas, sus marcadores de handoff, su login y su API key; si esa terminal lo tenía activo, corre `ccp use <nuevo>`. |
| ¿Cómo actualizo ccp? | `ccp upgrade` (re-ejecuta el instalador + `profile sync`). |
| ¿Cómo desinstalo? | `ccp uninstall` (quita el bloque del shell); opcionalmente `rm -rf ~/.config/ccp`. |

## Referencia rápida

| Quiero… | Comando |
|---|---|
| Crear cuenta oficial | `ccp profile add <n> --official` |
| Iniciar sesión en ella | `ccp profile login <n>` |
| Crear proveedor DeepSeek | `ccp profile add <n> --deepseek` |
| Crear proveedor Kimi | `ccp profile add <n> --kimi` |
| Crear proveedor GLM | `ccp profile add <n> --glm` |
| Guardar su API key | `ccp key <n>` |
| Asignar carpeta → perfil | `ccp path set <ruta> <perfil>` |
| Quitar una regla | `ccp path rm <ruta>` |
| Ver reglas / perfiles | `ccp path list` · `ccp profile list` |
| Cambiar a mano | `ccp use <n>` · `ccp default` |
| Continuar una sesión bajo otro perfil | `ccp handoff [<n>]` |
| Volver a entrar a un handoff vivo | `ccp handoff resume [<uuid>]` |
| Devolver un handoff a su origen | `ccp handoff end [<uuid>]` |
| Soltar un marcador de handoff huérfano | `ccp handoff discard [<uuid>]` |
| Ver qué hay en vuelo | `ccp handoff status [--all]` |
| Recortar el historial de handoffs | `ccp handoff prune [--keep N]` |
| Rotar de perfil solo al topar un límite | `ccp session` (`-p` para headless) |
| Ver qué haría `ccp session` | `ccp session --dry-run` |
| Montar el auto-handoff | `ccp auto init` y luego `ccp auto install` |
| Política / sensores del auto-handoff | `ccp auto status [--json]` |
| Abrir Claude Desktop con un perfil | `ccp desktop open [<n>]` |
| Estado / diagnóstico | `ccp status` · `ccp doctor` |
| Backup / restore | `ccp backup export\|restore` |
| Actualizar | `ccp upgrade` |
| Ayuda completa | `ccp help` |

> **Para scripting:** `ccp resolve [ruta]` imprime el perfil (exit `0` = no-default, `1` = default), y `ccp status --json` devuelve `active`, `profile`, `profile_type`, `cwd` y `repo`.

## Guía interactiva

¿Prefieres una guía visual paso a paso? Abre [`README.html`](README.html) en tu navegador — es una app de una sola página con playground de routing, tooltips y todos los casos:

```bash
open README.html        # macOS
xdg-open README.html    # Linux
```

## Desinstalar

```bash
ccp uninstall           # quita la función de shell del rc
rm -rf ~/.config/ccp    # (opcional) borra config y perfiles
```

---

MIT — mira [`LICENSE`](LICENSE). ¿Curioso de cómo funciona por dentro? Mira [`CLAUDE.md`](CLAUDE.md) y el [código en GitHub](https://github.com/JoseAFlores777/ccp).

## Aviso legal y marcas

**Úsalo bajo tu propio riesgo.** Este software se provee "tal cual", sin
garantía de ningún tipo (mira [`LICENSE`](LICENSE)). Eres responsable de tus
propias API keys, cuentas y configuración.

**Sin afiliación.** `ccp` (perfiles para Claude Code) es un proyecto
independiente y comunitario. **No está afiliado, avalado ni patrocinado por**
Anthropic, DeepSeek, Moonshot AI ni Z.ai. "Claude" y "Claude Code" son marcas de
Anthropic, PBC; "DeepSeek", "Kimi" (Moonshot) y "GLM" (Z.ai) son marcas de sus
respectivos dueños. Estos nombres se usan solo para describir interoperabilidad.
Mira [`NOTICE`](NOTICE).
