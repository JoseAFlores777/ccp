<div align="center">

# ccp

**Perfiles para Claude Code.**

[English](README.md) · **Español**

**Una cuenta de Claude Code distinta en cada carpeta.**
En tu repo de trabajo, tu cuenta de empresa; en tu proyecto personal, la tuya; en tus experimentos, DeepSeek.
El cambio ocurre solo, con hacer `cd`.

![version](https://img.shields.io/badge/version-2.18.0-c96442)
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

¿Prefieres una ventana? También hay una **app de escritorio** (`gui/`, Tauri, beta): las mismas cuentas, carpetas, conversaciones, rotación y ventanas de Desktop, una pantalla **Configuración** donde se lee y se edita en un solo sitio toda la configuración de Claude, capa por capa, una pantalla **Snapshots** con el historial de esa configuración —y el plan de cualquier restauración antes de escribir nada—, una pantalla **Nube** donde se confirma lo que el portal propuso y ejecuta código, y un **mapa de cuentas** donde los respaldos se conectan arrastrando flechas y nada se escribe hasta revisar y aplicar. Usa el motor que ya tienes (`ccp serve --stdio`), enseña el comando equivalente de cada pantalla y abre una Terminal para lo que solo puede hacerse en una terminal (un `/login`, un handoff, una sesión supervisada). Cómo compilarla y desarrollarla: [gui/README.md](gui/README.md).

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
| `CCP_RELEASE` | `latest` | Instala un tag concreto: `curl … \| CCP_RELEASE=v2.18.0 bash` |
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

> **`ccp profile rename` renombra el perfil también dentro de `auto_handoff`; `ccp profile rm` no lo toca.** Tres sitios llevan nombres de perfil —`policies.*.fallback`, `allow_from` (**tanto** las claves como las listas) y `hooks`— y un rename reescribe los tres en la misma escritura que tus reglas de ruta, comentarios incluidos. Borrar un perfil deja su nombre colgando en los tres, y el siguiente `ccp session` en ese directorio falla con el error de arriba hasta que arregles el YAML: busca el nombre antes de cerrar el archivo. Por esos restos un rename **se niega** a usar un nombre nuevo que `auto_handoff` ya menciona: si no, la cuenta renombrada heredaría en silencio cadenas y un gate `allow_from` que nunca fueron suyos.

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
ccp desktop doctor                 # audita lanzadores, instancias e identidad
```

Si alguna vez una ventana aparece vacía, o abrir Claude desde el Dock te trae la que no es, ejecuta
`ccp desktop doctor` antes de dar nada por perdido. Te dice qué ventanas hay vivas, si alguna corre con
la identidad del Claude principal, si alguna está escribiendo su historial de Code en el `~/.claude`
global y —la más importante— si un data dir guarda sesiones de más de una cuenta. Las sesiones se
indexan por cuenta: las que no ves **no** se han borrado, reaparecen al volver a entrar con esa cuenta.
Diagnostica; nunca repara.

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

### Un lanzador por perfil: nombre e icono de color propios en el Dock

`ccp desktop open` aísla la *cuenta*, pero todas las ventanas siguen siendo el mismo `Claude.app`: el Dock,
Cmd-Tab y Spotlight enseñan N iconos «Claude» idénticos. `ccp desktop app` lo arregla con un **lanzador**
por perfil en `~/Applications`:

```bash
ccp desktop app                              # un lanzador por perfil official, colores asignados solos
ccp desktop app work-1 --color purple        # o elige: blue green purple pink teal yellow red gray orange #rrggbb
ccp desktop app work-1 --label "Claude Work" # el nombre que se ve (es también el nombre del .app)
ccp desktop open work-1                      # desde ahora lanza a través del lanzador (--plain lo salta)
ccp desktop app rm work-1                    # quita el lanzador; la instancia y su sesión se quedan
```

`Claude (work-1).app` sale en Spotlight y Launchpad, se puede fijar en el Dock, y abre la instancia del
perfil con el icono de Claude tintado de ese color y su propio nombre en el Dock, Cmd-Tab y la barra de
menús. Un clic hace exactamente lo que `ccp desktop open work-1`: el lanzador *es* `ccp`, que pone
`--user-data-dir` y `CLAUDE_CONFIG_DIR` y le cede el proceso a Claude.

Rara vez necesitas ejecutar `ccp desktop app` a mano: si un perfil no tiene lanzador, `ccp desktop open` lo
construye antes de lanzar. No es cosmética. Sin lanzador, la ventana corre desde el propio
`/Applications/Claude.app`, así que macOS no puede distinguirla de tu Claude principal: el Dock, Cmd-Tab,
`open -a` y los enlaces `claude://` tratan las dos como la misma app, y abrir Claude te trae la ventana que
le parezca.

**La sesión se conserva.** El lanzador no es una copia retocada de la app —eso pierde la cuenta en el
primer arranque, porque el Keychain (donde Claude guarda la clave de su sesión) deja de fiarse del proceso.
Dentro del lanzador hay un espejo del `Claude.app` real hecho de hard links: byte a byte el original, sin
disco extra (aunque `du` lo cuente), y el Keychain sigue fiándose.

**Las actualizaciones siguen llegando.** El espejo fija la versión con la que se construyó, así que una
actualización de `Claude.app` deja al lanzador corriendo la versión anterior hasta su siguiente arranque, en
el que `ccp` se da cuenta —por versión *y* por inodo, porque el updater no parchea el bundle: lo reemplaza
entero— y reconstruye el espejo (un segundo). Igual tras `ccp upgrade`: los lanzadores llevan el binario
dentro y en su siguiente arranque enlazan el nuevo.

**Una instancia de perfil no puede actualizar tu Claude.** Podía, y lo hizo una vez: una instancia actualizó
el `/Applications/Claude.app` de un usuario desde una ventana abierta para otra cuenta. Ahora toda instancia
distinta de `default` arranca con su updater apagado, y `ccp desktop doctor` comprueba que la barrera siga
puesta en vez de darla por hecha. `default` queda exento a propósito: esa instancia **es** tu Claude, y
apagarle las actualizaciones sería secuestrártelo — las actualizaciones te llegan por ahí, como siempre.

### Llevar una conversación a la ventana de otro perfil

Cada ventana tiene sus propias sesiones de la pestaña Code, así que una conversación empezada con una cuenta
no aparece en la ventana de otro perfil. `ccp desktop copy` la lleva: copia la conversación al perfil
destino y le pide a esa ventana que la importe, para que salga en su barra lateral con el mismo título y
sigas donde lo dejaste.

```bash
ccp desktop sessions                                # las sesiones de todas las ventanas: uuid, título, carpeta
ccp desktop sessions trabajo-1                      # solo las de ese perfil (todas)
ccp desktop copy "validador de tipo de cuota" default   # por título, o un trozo…
ccp desktop copy 37b541db trabajo-1                 # …o por uuid, o sus primeros caracteres
ccp desktop copy 37b541db trabajo-1 --from personal-1   # cuando la misma sesión vive en dos ventanas
ccp desktop copy 37b541db trabajo-1 --dry-run       # dice lo que haría, sin tocar nada
ccp desktop copy 37b541db trabajo-1 --no-open       # solo copia (luego la sigues en la terminal)
```

Qué hace:

1. **Encuentra la sesión.** Los títulos salen del índice de cada ventana: los mismos que ves en su barra
   lateral. Sin `--from` busca en todas las ventanas menos la del destino; si el nombre casa con más de una
   sesión, las lista (con su uuid completo) y se para.
2. **Copia la conversación**: el transcript de Claude Code (`<cc-home>/projects/<carpeta>/<uuid>.jsonl`) y la
   carpeta de al lado con los archivos de subagentes y workflows, al perfil destino, en la misma carpeta y con
   el mismo uuid. La copia es un archivo propio y privado (`0600`). El original no se toca nunca.
3. **Le pide a la ventana destino que la importe**, con el enlace de la propia Claude Desktop,
   `claude://resume?session=<uuid>`, mandado a *esa* ventana (a su lanzador; a tu Claude si es `default`).
   `ccp` no escribe nunca a mano el índice de Desktop: espera a que Desktop añada la sesión y solo entonces
   dice que está hecho. Si la ventana estaba cerrada, el enlace la abre.

Las reglas que la hacen segura:

- **Nunca pisa una conversación.** Si el destino ya tiene la sesión y seguiste trabajando allí, no se toca.
  Si el destino tiene una copia anterior que nadie continuó —la vuelta de un préstamo—, se pone al día, pero
  no con esa ventana abierta y la sesión en su barra lateral: podría tener cargada la versión vieja y
  bifurcaría la conversación, así que `ccp` te pide cerrarla antes. Si las dos siguieron por separado, no se
  escribe nada.
- **El enlace solo va a donde debe.** Antes de mandarlo, `ccp` comprueba con `ps` y `lsappinfo` que la
  ventana destino conserva su propia identidad. Un perfil sin lanzador (su ventana corre como tu Claude
  principal) o una ventana con la identidad colapsada (ver *Límites*) recibe la copia pero no el enlace, y se
  te dice por qué y qué hacer. Si no puede comprobarlo, no lo manda.
- **El título viaja con ella.** Desktop solo lee el título de los últimos 256 KB de la conversación; en una
  larga que se tituló al principio, `ccp` añade el título al final de la copia.
- **Es idempotente.** Repetirlo no duplica nada: a una ventana que ya lista la sesión no se le pide que la
  vuelva a importar.
- Solo se copian sesiones con transcript local (las remotas no), y solo a ventanas `official` o `default`.
  También sirve con una sesión del CLI: pásale su uuid completo. La importación es solo para macOS; en otros
  sistemas tienes la copia y el comando para seguirla en la terminal.

La original sigue en la ventana de origen. Sigue en un solo sitio: si escribes en las dos, se separan, y a
partir de ahí ninguna copia conserva las dos. Código de salida: `0` si la sesión queda en la barra lateral
del destino (o, con `--no-open`, copiada); `1` en cualquier otro caso, incluido «copiada pero sin
importar», que siempre viene con lo que hay que hacer.

### Límites

- Solo perfiles **`official`** y **`default`**. Claude Desktop no lee `ANTHROPIC_BASE_URL`, así que un
  perfil DeepSeek/Kimi/GLM se rechaza en vez de dejarte una ventana a medias.
- **`default` no se reubica**: su instancia es la de siempre, donde siempre.
- Cada instancia se baja su propia copia del Claude Code embebido (~190 MB), más el entorno de Cowork
  si lo usas. `ccp desktop list` te dice cuánto ocupa cada una.
- **Inicia sesión de una en una**: los enlaces `claude://` van a la instancia que registró el esquema
  de último, así que con varias ventanas abiertas el login puede aterrizar en la equivocada. `ccp` te
  lo recuerda en el primer arranque de cada instancia.
- **La identidad de una ventana puede colapsar.** macOS identifica un proceso por la ruta con la que se
  ejecutó, y Claude se re-ejecuta por su ruta *resuelta* al reiniciarse: tras un relanzamiento, la ventana
  puede volver llevando la identidad de tu Claude principal. Mientras eso dure, abrir Claude desde el Dock
  activa *esa* ventana en vez de la tuya. `ccp desktop doctor` te dice cuándo ha pasado, y
  `open -n -a /Applications/Claude.app` te devuelve la tuya al momento.
- `ccp desktop rm <perfil> --yes` borra la instancia: es un logout que se lleva sesión, tokens y MCP.
- **Los lanzadores son solo macOS**: son bundles de app en `~/Applications` (`CCP_DESKTOP_APPS_DIR` los
  mueve). Un lanzador nunca reclama `claude://`, así que el callback del login siempre llega a la app
  normal: inicia sesión en un perfil **nuevo** con `ccp desktop open <perfil> --plain` y las demás ventanas
  cerradas, y luego usa el lanzador. Los permisos de privacidad de macOS (archivos, pantalla, micrófono…)
  se conceden por app, así que un lanzador puede pedirlos otra vez.

## Backup y restore

Llévate todo a otra máquina, o respáldalo antes de un cambio grande:

```bash
ccp backup export ~/ccp-backup.tar.gz                 # ccp.yaml + overlays
ccp backup export ~/ccp-backup.tar.gz --with-secrets  # + api_key + logins (chmod 600)
ccp backup restore ~/ccp-backup.tar.gz                # no pisa; fusiona reglas
ccp backup restore ~/ccp-backup.tar.gz --overwrite    # reemplaza perfiles del backup
ccp backup restore ~/ccp-backup.tar.gz --force        # borra todo y restaura limpio
```

Antes de restaurar, `ccp` guarda un snapshot automático en `~/.config/ccp/.backup-pre-restore-<fecha>`, y además
un [snapshot de seguridad](#snapshots--el-historial-de-toda-tu-configuración) de todo lo demás.

## Snapshots — el historial de toda tu configuración

`ccp backup` es un archivo que hay que acordarse de hacer. `ccp snapshot` es un **historial**: cada snapshot
guarda toda la configuración, no solo la de ccp, y solo ocupa lo que cambió. Viven en
`~/.config/ccp/snapshots`.

```bash
ccp snapshot create -m "antes de probar el MCP nuevo"   # uno ahora (con etiqueta, prune no lo borra)
ccp snapshot list                                     # el historial, del más nuevo al más viejo
ccp snapshot diff latest                              # qué cambió desde entonces
ccp snapshot restore 3f2a9c                           # solo ENSEÑA el plan (sale 1): no escribe nada
ccp snapshot restore 3f2a9c --yes                     # lo aplica
ccp snapshot restore 3f2a9c --only claude/agents --yes   # solo una parte
ccp snapshot export latest ~/config.ccpsnap --with-secrets   # un archivo para otra máquina
ccp snapshot import ~/config.ccpsnap
```

Qué captura un snapshot:

| Qué | Clase | Se captura |
|---|---|---|
| `ccp.yaml`, el overlay de cada perfil (`CLAUDE.md`, `settings.overlay.json`) | config | siempre |
| `~/.claude`: `settings.json`, `CLAUDE.md`, `keybindings.json`, `agents/`, `commands/`, `skills/`, `output-styles/`, `hooks/`, las listas de plugins | config | siempre |
| `.claude/settings.local.json` y `CLAUDE.local.md` de cada carpeta con regla | config | siempre |
| La `api_key` de un proveedor, la parte de MCP de cada `.claude.json`, el `claude_desktop_config.json` de cada ventana de Desktop | secreto | siempre, sellado con la clave del almacén |
| Conversaciones (`cc-home/projects`) y `handoffs.yaml` | estado | solo con `--with-state` |
| Lo generado, las cachés, los logins, los tokens, el `machineID` | — | nunca: se regeneran o se recrean |

- **Restaurar nunca va a ciegas.** Sin `--yes` imprime el plan y no cambia nada. Con `--yes`, primero guarda un
  snapshot del estado actual; si no puede, no escribe nada. Solo escribe lo que trae el snapshot, nunca borra lo
  que solo existe en disco, y después regenera los perfiles afectados. De un `.claude.json` solo viaja la
  configuración: restaurarla la fusiona con el archivo vivo sin tocar tu sesión.
- **Snapshots automáticos.** Uno de seguridad antes de `ccp profile rm` y de `ccp backup restore`: si no se
  puede guardar, el comando no se ejecuta. Y uno diario, que toma el primer comando de gestión pasadas 20 horas
  (nunca `ccp status`, `resolve` ni el hook del prompt). `CCP_NO_AUTO_SNAPSHOT=1` apaga los dos.
- **Poda.** `ccp snapshot prune` conserva 7 días, 4 semanas y 6 meses, más el último y todo lo fijado
  (`ccp snapshot pin <id>`) o etiquetado. `--dry-run` enseña lo que borraría.
- **Los secretos en un archivo exportado** solo viajan con `--with-secrets`, sellados con una frase de al menos
  12 caracteres que `ccp` pide sin eco (`CCP_SNAPSHOT_PASSPHRASE` la da en scripts). Sin ella, el archivo no
  lleva secretos, y al importarlo se dice qué elementos llegaron sin datos.

#### La pantalla **Snapshots** — el historial, en la app

La app de escritorio enseña ese mismo historial en una línea de tiempo: etiqueta, qué lo disparó, cuándo, cuánto
capturó y si está fijado. Al elegir uno se ve **qué captura**, agrupado por partes (ccp, `~/.claude`, cada ventana
de Desktop, cada proyecto), y desde ahí se fija, se etiqueta, se exporta o se compara.

- **Restaurar va en dos pasos y el primero no escribe nada.** La app pide el plan, enseña paso a paso qué
  escribiría, qué fusionaría, qué ya coincide y qué se salta —y por qué—, y solo aplica cuando marcas qué partes
  quieres y escribes la palabra de confirmación. Es lo mismo que en la terminal, donde sin `--yes` un restore es
  solo un plan. Al terminar dice cuál es la foto previa, que es por dónde se vuelve atrás.
- **Las casillas son los `--only`**, así que la línea de CLI que se ofrece para copiar hace exactamente lo mismo
  que el botón.
- **Comparar contra «lo que hay ahora mismo»** responde a la pregunta de verdad —qué ha cambiado desde
  entonces— sin restaurar nada.
- **Podar enseña antes qué se llevaría por delante**: cuántos quedan, cuántos blobs se liberan y los ids que se
  van.
- El tamaño de un snapshot es **lo que captura**, no lo que ocupa: dos snapshots parecidos comparten los mismos
  blobs y solo se guarda lo nuevo.
- Las copias `.tar.gz` de `ccp backup` siguen en **Ajustes**: son el formato antiguo, bueno para mover una
  configuración a mano a otra máquina.

## Nube — el mismo historial, en tu propio servidor

`ccp snapshot` es el historial de esta máquina. `ccp cloud` es ese mismo historial en un servidor tuyo, para
que una segunda Mac lo recoja. **Todo se cifra aquí antes de salir**: el servidor guarda texto sellado, ids
opacos y firmas que no sabe hacer, y nunca ve la clave.

```bash
# primera máquina
ccp cloud login https://ccp.example.com   # código de dispositivo: apruébalo en el navegador
ccp cloud init                            # crea la bóveda; APUNTA el código de recuperación
ccp snapshot create -m "primera subida"
ccp cloud push                            # sube lo que la nube no tiene

# la otra máquina
ccp cloud login https://ccp.example.com
ccp cloud unlock                          # la frase de la bóveda (no la contraseña de la cuenta)
ccp cloud pull latest                     # lo baja al almacén local; te dice el id
ccp snapshot restore <id>                 # el id que acaba de decir el pull, NO `latest`:
                                          # `latest` es el más reciente por fecha de creación,
                                          # y el bajado conserva la fecha de la otra máquina.
                                          # enseña el plan; con --yes lo aplica

ccp cloud status      # servidor, cuenta, equipo, bóveda y cuántos quedan por subir
ccp cloud list        # snapshots en la nube, de todos los equipos
ccp cloud devices     # tus equipos; `ccp cloud revoke <id>` echa a uno
ccp cloud logout      # revoca este equipo y borra su token y su bóveda local
```

**Son dos secretos, y no son el mismo.** La contraseña de la cuenta se escribe en la página de acceso y
responde a *quién eres*. La **frase de la bóveda** no llega nunca al servidor y responde a *si puedes leer
esto*. El **código de recuperación** se enseña una sola vez, al crear la bóveda: guárdalo fuera de este
equipo (un gestor de contraseñas, papel). Si pierdes la frase **y** el código, la copia de la nube no se
puede recuperar —tus snapshots locales siguen siendo la fuente primaria—.

**Qué ve el servidor y qué no:**

| Lo ve | No lo ve |
|---|---|
| Cuándo se hizo cada snapshot, cuánto ocupa, qué equipo lo mandó y a cuál sigue | Qué hay dentro: blobs y manifiestos llegan sellados |
| Ids opacos (un HMAC del hash local), lo justo para guardar cada cosa una vez | Si tienes un archivo concreto: no puede comprobar un id que no recibió |
| Tus equipos: nombre, plataforma y último contacto | Tus claves de API, tokens de MCP, hooks ni instrucciones |

- **Las firmas se comprueban con tu propia clave.** Cada snapshot va firmado con una clave derivada de la de
  la bóveda, y `pull` la verifica con la pública derivada aquí, nunca con una que diga el servidor. Un
  servidor comprometido puede negarte el servicio; colar, alterar o reordenar un snapshot, no.
- **Los blobs no pasan por el API.** Van directos entre este equipo y el almacenamiento, con URLs
  prefirmadas. Lo que pase de 64 MiB se queda fuera y `push` dice qué fue.
- **Las rutas se traducen entre máquinas.** Un snapshot hecho bajo `/Users/ana` y restaurado donde el HOME es
  `/Users/jose` reescribe el HOME dentro de las reglas de carpeta, de los comandos de hooks y de MCP y de la
  ruta de cada proyecto —el restore lo dice: «Rutas de … reescritas a …»—. Las conversaciones son historia:
  sus rutas describen dónde ocurrió algo, así que se dejan como están.
- **Revocar un equipo** lo echa del API en su siguiente petición. No borra la clave que ese equipo ya tiene:
  si temes una filtración, lo que toca es rotar la clave de cuenta, que todavía no está.
- **Los archivos de la nube de este equipo** viven en `~/.config/ccp/cloud` (0700, cada archivo 0600): la
  sesión, el token del dispositivo, la clave desbloqueada y qué snapshots están ya subidos.

Montar el servidor (Postgres + Keycloak + almacenamiento S3 + el API `ccp-cloud`) es otra faena; las piezas
están en `deploy/ccp-cloud/`. **El despliegue público está pendiente de que el dueño lo autorice**, así que
hasta entonces `ccp cloud` apunta al servidor que levantes tú.

### El portal web

El portal lo sirve el propio `ccp-cloud`, en la raíz del mismo host que el API. Entras con Keycloak y te pide
la **frase de la bóveda**: la clave de cuenta se deriva **en la pestaña**, con Argon2id, y no sale del
navegador —el servidor sigue guardando cosas que no sabe abrir—. La olvida al cerrar la pestaña o tras 15
minutos sin tocar nada.

- **Dispositivos**: último contacto, versión de ccp, los perfiles que tiene cada equipo y su estado frente a
  la revisión que se le publicó, con cuántas rutas difieren.
- **Línea de tiempo** de cada máquina, con la firma de cada snapshot comprobada contra la clave derivada
  aquí, y un **diff entre dos cualesquiera**, agrupado por área y filtrable por ruta.
- **Editor** de la configuración de un snapshot, con el mismo modelo que la pantalla Configuración de la app
  —capa, tipo, dónde aplica cada elemento y, cuando no se puede editar ahí, por qué— y **«Aplicar a…»**, que
  publica lo editado como revisión deseada firmada a las máquinas que elijas. Ni restaura ni crea ni borra
  elementos: el portal propone, e inventar una ruta lógica desde el navegador es fabricar un archivo que nadie
  sabe dónde poner.
- La firma tiene tres respuestas, no dos: válida, alterada y *este navegador no sabe verificar Ed25519*, que
  no es lo mismo que válida.

No hay paso de compilación: módulos ES empotrados en el binario, una CSP estricta y ningún script de
terceros, así que desplegar el API es desplegar el portal. Cómo se construye, qué necesita Keycloak y cómo
mirarlo sin desplegar nada están en [`docs/portal.md`](docs/portal.md).

El otro extremo de eso es la pantalla **Nube** de la app: la cuenta, la bóveda, tus equipos y —lo importante—
lo que el agente dejó esperando aquí porque ejecuta código. Nada viene marcado: se aprueba ruta a ruta, y lo
que no marcas se rechaza y se informa al portal. Iniciar sesión y abrir la bóveda no se hacen desde la app:
abre una Terminal, porque la frase de bóveda desenvuelve la clave de cuenta y el cifrado de extremo a extremo
vale exactamente lo que valga el sitio por el que pasa esa frase.

## Detectar la máquina — `ccp scan` y `ccp adopt`

`ccp scan` lista todo lo de Claude que hay en esta máquina: tu `~/.claude` global, cada perfil, los MCP de cada
ventana de Desktop, los archivos de los proyectos y los `CLAUDE_CONFIG_DIR` que ccp aún no gestiona. De cada cosa
dice **dónde aplica**: CLI, pestaña Code o chat de Desktop. Los valores secretos no salen nunca, solo que existen.

`ccp adopt` lo convierte en un plan y solo lo aplica con `--yes`, tras un snapshot de seguridad:

```bash
ccp adopt              # enseña el plan (sale 1 si hay algo que aplicar)
ccp adopt --yes        # aplica los pasos marcados
ccp adopt --only <id> --yes
```

- Un MCP que solo vive en tu ventana principal de Desktop se propone como **global** (`~/.claude.json`), para que
  lo vea también la CLI. El mismo MCP con **credenciales distintas** en dos ventanas no se sube nunca: daría el
  token de una cuenta a todos los perfiles.
- Un `~/.claude-xyz` que usabas a mano se adopta como perfil nuevo **copiando** su configuración (nunca sus
  tokens ni su `env`); el original no se toca y haces un `/login`.
- Los logins, las API keys, los comandos de MCP que faltan y los lanzadores de Desktop salen como cosas por hacer
  a mano.

La app de escritorio enseña lo mismo en la pantalla **Detectar esta máquina**.

## Config por perfil

Cada perfil tiene su propia config de Claude, aplicada como **capa baseline** cuando está activo:

```bash
ccp profile config <perfil>                 # menú: instrucciones / settings / ambos
ccp profile config <perfil> instructions    # abre overlay/CLAUDE.md
ccp profile config <perfil> settings         # abre overlay/settings.overlay.json
ccp profile sync [<perfil>]                  # re-mergea cambios del global ~/.claude
ccp profile rename <viejo> <nuevo>           # renombra: reglas, cadenas, marcadores y key incluidos (official: vuelve a iniciar sesión)
ccp config editor "code -w"                  # editor a usar (fallback: $EDITOR)
```

- **Instrucciones**: `cc-home/CLAUDE.md` hace `@import` del global `~/.claude/CLAUDE.md` y luego de tu overlay.
- **Settings**: `cc-home/settings.json` = global ⊕ overlay (deep-merge puro en Go).
- **`/config` dentro de un perfil se conserva**: Claude Code lo escribe en `cc-home/settings.json`, y antes de regenerarlo `ccp` pasa al overlay del perfil lo que añadiste o cambiaste (`ccp profile sync` dice qué). Lo que quitaste solo se avisa, si el overlay también cambió esa clave gana el overlay, y un archivo inválido se copia a `profiles/<nombre>/state/` en vez de adoptarse.
- **Prioridad real**: es una baseline — la config del repo (`.claude/settings.json`) gana en conflicto.
- `default` no tiene overlay: `ccp profile config default` abre tu `~/.claude` global directo.

### MCP, agentes y skills por perfil

Un perfil ya no tiene que tomarlo todo prestado del `~/.claude` global. Lo que declara para sí vive en su overlay, y cada regeneración lo **proyecta** a los archivos que leen de verdad las apps ([ADR 0011](docs/adr/0011-una-fuente-declarada-varias-proyecciones.md)):

```bash
ccp instruct add profile mcp 'obsidian-vault={"command":"npx","args":["-y","obsidian-mcp"]}'
ccp instruct add profile rule "..."       # texto llano; un hook va como 'id={json}'
ccp instruct dest profile skill           # el directorio donde escribir un agent/command/skill
ccp profile sync <perfil>                 # reproyectar todo a mano
ccp profile sync --check [<perfil>]       # lo que CAMBIARÍA; sale 1 si algo está desfasado
```

| Capa | Archivo | A quién llega |
|---|---|---|
| Global | `~/.claude.json` (`mcpServers`) | a todos los perfiles, por su proyección |
| Perfil | `profiles/<n>/overlay/mcp.json` (la forma de `.mcp.json`) | solo a ese perfil |
| Proyecto | `<repo>/.mcp.json` | a cualquier perfil, dentro de ese repo (lo lee Claude Code; `ccp` no lo proyecta) |

**Efectivo = global ⊕ perfil − apagados**, y si un nombre choca gana el perfil. A dónde va cada servidor se declara en `ccp.yaml`:

```yaml
mcp:
  targets:                     # por servidor; por defecto, los dos
    obsidian-vault: [cli, desktop]
    jira: [cli]
  disabled:                    # este perfil apaga uno heredado sin borrarlo
    work: [finance-os]
```

- **`cli`** escribe en el `cc-home/.claude.json` del perfil, que es lo que leen tu `claude` de terminal **y** la pestaña Code de la ventana de ese perfil.
- **`desktop`** escribe en el `claude_desktop_config.json` de esa ventana, que es la mitad de **chat**. Dos límites son de la app, no nuestros: solo entradas `stdio` (una `http`/`sse` la descarta al arrancar, así que se informa en vez de escribirla), y el archivo no se relee en caliente — así que con la ventana abierta el cambio queda **pendiente** y se aplica al siguiente arranque. `ccp` nunca te cierra una ventana.
- **`ccp` solo toca los nombres que registró como suyos.** Lo que añadiste a mano en esos archivos se queda; si un nombre que declaraste ya estaba ahí a mano, se informa como conflicto en vez de pisarlo.
- En tu **ventana principal de Desktop (`default`) no se escribe nunca** — es tu propio Claude, el mismo motivo por el que sus actualizaciones se respetan.
- `overlay/{agents,commands,skills,output-styles}/` funcionan igual: en cuanto el perfil tiene algo propio, su directorio en el `cc-home` pasa a ser global ∪ perfil (directorios reales y symlinks solo en las hojas, la forma que Desktop exige), y si chocan gana el perfil.
- **Los permisos se fusionan reemplazando arrays**, como siempre. Para sumar en vez de reemplazar, dilo en el overlay: `"permissions": {"$merge": "union", "allow": ["Bash(make:*)"]}`. La marca nunca llega al `settings.json` generado.
- `ccp doctor` dice lo que está fuera de sitio: `projection_stale`, `desktop_restart_pending`, `mcp_command_missing`, `mcp_unmanaged_only_desktop` y `cc_home_symlink_nonleaf`.

#### `ccp mcp` — lo mismo sin editar archivos

```bash
ccp mcp list [--scope <capa>] [--json]               # qué declara la capa y a dónde va
ccp mcp add fs -- npx -y @modelcontextprotocol/server-filesystem ~/code
ccp mcp add linear --url https://mcp.linear.app/sse --transport sse --header 'Authorization=Bearer ${LINEAR}'
ccp mcp add jira '{"command":"npx","args":["-y","jira-mcp"]}'   # el JSON crudo, si lo prefieres
ccp mcp rm <nombre> [--scope <capa>]
ccp mcp enable|disable <nombre> [--profile <n>]      # uno heredado, en UN perfil
ccp mcp targets [<nombre> [cli|desktop|cli,desktop|none]]
```

La capa es `global`, `profile[:<nombre>]`, `project[:<ruta>]` o `desktop[:<nombre>]` (solo lectura), y **sin `--scope` es el perfil activo de la terminal**. `--env CLAVE=valor` acompaña a la forma stdio y `--header CLAVE=valor` a la remota; las tres formas (el comando tras `--`, `--url`, el JSON) no se mezclan. Un secreto entra como el literal `${VARIABLE}` —de ahí las comillas simples: con dobles lo expande tu shell antes y el token queda en claro (y la capa `project` lo rechaza).

Dos negativas son la razón de ser del comando: la **ventana de Desktop** recibe los MCP del perfil pero no los declara —escribir ahí lo desharía el siguiente sync, así que te dice que lo declares en el perfil—, y un **secreto en claro en el `.mcp.json` de un proyecto** —un archivo que viaja en el repo— se niega señalando `${VARIABLE}`. Después de cada escritura dice a quién regeneró y qué ventana se queda con los MCP de antes hasta que la reinicies.

#### La pantalla **Configuración** — las mismas capas, en la app

La app de escritorio edita todo esto desde una sola pantalla: arriba la capa (global · perfil · proyecto · ventana), a la izquierda los tipos (instrucciones, MCP, skills, agentes, comandos, hooks, permisos, variables, plugins, estilos, barra de estado y el resto de `settings.json`) y en el centro cada elemento con **de dónde viene** y **dónde aplica** (CLI · Code · Chat). Un conmutador **Efectivo** enseña el resultado fusionado de una cuenta, con lo que queda tapado marcado como tal.

- Lo que se ve y no se edita dice por qué, con las mismas palabras que la terminal: lo trae un plugin, lo fija `managed-settings`, lo proyecta `ccp` desde el overlay.
- Los secretos de un MCP se enseñan **enmascarados**, y lo que dejes enmascarado se restituye al guardar: el editor no tiene por qué revelar un token para dejarte cambiar el argumento de al lado.
- Las **acciones de capa** («llevar a…») mueven un elemento a la global, a otro perfil o a un proyecto. Es el mismo elemento con otra capa, así que acaba en el archivo que elige `core`; copiar no borra el origen, porque la capa más específica sigue ganando.
- Tras cada escritura dice dónde quedó, a quién regeneró y qué ventana de Desktop se queda con los MCP de antes hasta que la reinicies. Cada pantalla enseña su equivalente de CLI, que son exactamente los comandos de arriba.

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
| ¿Cómo renombro un perfil? | `ccp profile rename <viejo> <nuevo>` (o `r` en el TUI). Se mueven con él sus reglas, sus cadenas de rotación (`fallback`, `allow_from`, `hooks`), sus marcadores de handoff y su API key; si esa terminal lo tenía activo, corre `ccp use <nuevo>`. Un perfil official tiene que volver a iniciar sesión (`ccp profile login <nuevo>`): Claude Code nombra su credencial del Llavero según la carpeta del perfil, y ccp lo avisa en vez de tocar el Llavero. Si tenía lanzador de Desktop, ccp te da los dos comandos que lo sustituyen. |
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
| Dar a cada instancia de Desktop nombre e icono de color propios | `ccp desktop app [<n>]` |
| Ver las sesiones de la pestaña Code de cada ventana de Desktop | `ccp desktop sessions [<n>]` |
| Llevar una conversación a la ventana de Desktop de otro perfil | `ccp desktop copy <uuid\|título> <n>` |
| Estado / diagnóstico | `ccp status` · `ccp doctor` |
| Backup / restore | `ccp backup export\|restore` |
| Snapshots | `ccp snapshot create\|list\|diff\|restore\|export\|import` |
| Nube | `ccp cloud login\|init\|unlock\|push\|pull\|list\|devices` |
| Dar de alta o de baja un servidor MCP | `ccp mcp add\|rm <n>` · `ccp mcp list` |
| Apagar un MCP heredado en un perfil | `ccp mcp disable <n> --profile <perfil>` |
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
