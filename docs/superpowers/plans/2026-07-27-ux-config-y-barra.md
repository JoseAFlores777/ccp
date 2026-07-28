# Plan — La config y el estado, dentro de ccp

**Fecha:** 2026-07-27
**Estado:** propuesto
**Alcance:** `internal/core` (gauge, auto, config), `internal/cli` (statusline, config, auto, session), `internal/tui`, `legacy/` + golden

---

## 1. El problema real

El auto-handoff funciona. Lo que no funciona es **vivir con él**.

Hoy, para cambiar el orden en que ccp presta perfiles, el usuario abre `~/.config/ccp/ccp.yaml`
en un editor de terminal, edita dos claves que no están juntas (`policies.default.fallback` y
`allow_from.<primario>`), y descubre si acertó corriendo `ccp auto status`. Si se equivoca en el
gate, la cadena queda vacía **en silencio**: `allow_from` declarado sin entrada para el primario
es deny total, y eso no se ve en el yaml, se ve en la ausencia de saltos a las tres de la mañana.

Y la barra de estado dice `personal-cc · 5h 2% · 7d 59%`. Un porcentaje sin horizonte. «59%» no
responde la única pregunta que el usuario se hace al mirarla: *¿me va a cortar hoy, y cuándo
vuelvo a tener cuota?*

Los seis pedidos son seis síntomas de una misma cosa: **la configuración y el estado del
auto-handoff viven fuera de la herramienta**. El plan los devuelve adentro.

Tres bloques:

| Bloque | Pedido | Qué resuelve |
|---|---|---|
| **Ver** | barra con medidor + reset | saber cuándo se libera la cuota, no solo cuánto queda |
| **Editar** | `config edit` GUI · `auto chain` · panel Config en la TUI | cambiar la política sin abrir un yaml a mano |
| **Arrancar** | `ccp session` que se autoconfigura | que el primer `ccp session` en un repo funcione |

---

## 2. Decisiones y trade-offs

### D1 · La barra: medidor de bloques, con degradación por ancho

**Elegido**

```
personal-cc  5h ▏██░░░░░░░░▏ 2% ·2h14m  7d ▏██████░░░░▏ 59% ·3d
```

**Descartado**

- *Puntos de color*: compacto, pero un `●` ámbar obliga a recordar qué umbral significa ámbar.
  El medidor se lee sin leyenda.
- *Sparkline minimalista*: demasiado discreto para algo que existe justamente para avisar.

**El trade-off es el ancho.** La barra de CC es una línea; el formato de arriba pide ~62 columnas.
En una terminal partida en dos, o con un statusLine propio del usuario ya ocupando espacio, eso
se trunca — y una barra truncada es peor que una barra corta, porque corta justo por el final,
que es donde va el dato del reset.

**Mitigación: tres niveles, elegidos por ancho disponible.** No es un extra, es parte del diseño:

| Ancho | Render |
|---|---|
| ≥ 60 | `personal-cc  5h ▏██░░░░░░░░▏ 2% ·2h14m  7d ▏██████░░░░▏ 59% ·3d` |
| 40–59 | `personal-cc · 5h ▏██░░▏2% ·2h14m · 7d ▏██▆░▏59% ·3d` (medidor de 4) |
| < 40 | `personal-cc · 5h 2% ·2h · 7d 59% ·3d` (sin medidor) |

El ancho sale de `COLUMNS` y, si no está, de un default de 80. **No se consulta la tty**: el
statusLine corre dentro de CC con el stdout redirigido, así que preguntarle a la terminal es a la
vez caro y poco fiable, y este comando corre varias veces por minuto.

**Color**: verde `<70%`, ámbar `70–89%`, rojo `≥90%` — los mismos cortes que ya usa el umbral por
defecto de la política (90). Se respeta `NO_COLOR`; sin color el medidor sigue siendo legible,
que es exactamente por qué se eligió el medidor y no el punto.

### D2 · El reset: cuenta atrás, y silencio cuando el dato miente

Formato por magnitud, una sola unidad significativa: `·47m`, `·2h14m`, `·3d`.

Los días **redondean hacia arriba**. La primera versión truncaba, y truncar era justo el error que
esta cuenta atrás existe para evitar: a 2d23h59m del reset pintaba `·2d`, el usuario volvía un día
antes de tiempo y se encontraba el perfil todavía agotado. El error máximo es de un día en cualquiera
de los dos sentidos, así que lo único que se elige aquí es la **dirección** — y es la misma que
defiende la regla de abajo: ante la duda, nunca digas que la cuota está más cerca de lo que sabes.

La otra regla, la que más importa: **un `resets_at` en el pasado o a cero no se pinta.** Es la misma
filosofía que `Windowed.HasData` ya defiende en `ratelimit.go` — hay un bug conocido de CC donde
`five_hour` llega a 0 con `seven_day` poblado, y pintar `·0m` diría «ya reabrió» cuando lo que
sabemos es «no sabemos». Una ventana con dato pero sin `resets_at` sale con su porcentaje y sin
cuenta atrás.

La cuenta se recalcula en cada render desde el `resets_at` de la muestra. No hay caché que
invalidar: la barra se repinta sola varias veces por minuto.

### D3 · Dónde vive el código del medidor

`internal/core` no hace presentación… salvo cuando el string *es* el contrato (`env.go`,
`shellinit.go`). El medidor no es contrato, pero sí lo necesitan **dos** front-ends: la barra
(`internal/cli`) y el panel Estado de la TUI (`internal/tui`), y `tui` no importa `cli`.

**Decisión:** primitivas **puras** en `internal/core/gauge.go` — `RenderGauge(pct, cells)` y
`HumanUntil(d)` devuelven texto sin color ni layout. El color, el ancho y el orden los deciden los
front-ends. Así se comparte lo que se puede compartir sin meter una dependencia de presentación en
el core.

### D4 · `ccp config edit`: el editor gráfico, y lo que pasa al cerrarlo

Cadena de resolución, primero que exista gana:

1. `--editor <cmd>` (flag)
2. `defaults.gui_editor` en `ccp.yaml` (clave nueva, aditiva)
3. `$VISUAL`
4. VS Code y familia en el `PATH`: `code` → `cursor` → `code-insiders`, invocados con **`-w`**
5. Fallback del SO: macOS `open -W -t`, Windows `notepad`, Linux `xdg-open`
6. Último recurso: el editor de terminal de siempre (`core.ResolveEditor` → `defaults.editor` → `$EDITOR` → `nano`)

**El `-w` es la decisión, no un detalle.** `code archivo` retorna al instante; sin esperar a que el
editor cierre no podemos hacer lo único que da valor real a este comando: **releer el yaml y
validarlo**. Un `ccp.yaml` roto por una edición gráfica no se manifiesta al guardar — se manifiesta
en el siguiente `ccp session`, en mitad de un salto, con un error de parseo.

Con editor bloqueante: al cerrar, `core.Load` + validación de `auto_handoff` (`Effective` de cada
política, existencia de cada perfil del fallback). Si falla, se dice **qué** clave está mal y se
ofrece reabrir. Con `xdg-open` (no bloqueante) esa validación es imposible: se avisa explícitamente
de que no se va a validar, en vez de fingir que sí.

Objetivo por defecto: `ccp.yaml`. `--profile <n>` abre el overlay de ese perfil (ya existe
`core.ProfileConfig` para eso, y ya regenera el `cc-home` tras editar).

`ccp config gui-editor <cmd>` fija la preferencia, espejando el `ccp config editor` que ya existe.

### D5 · `ccp auto chain`: el orden y el permiso, en un solo gesto

```
ccp auto chain [show]                    # cadena efectiva para el cwd
ccp auto chain add <perfil>... [--at N]  # añade (al final, o en la posición N)
ccp auto chain rm  <perfil>...
ccp auto chain mv  <perfil> <pos>        # el orden ES la prioridad
ccp auto chain set <a,b,c>               # reemplaza la lista entera
```

Todos aceptan `--policy <n>` (default: `default`).

**`add` toca también `allow_from`.** Ese es el punto: quien escribe «añádelo a la cadena» quiere
que el perfil se use, y `fallback` sin `allow_from` no lo usa. Dejar las dos claves separadas es
reproducir en la CLI exactamente la trampa que el yaml ya tiende.

Pero `allow_from` es un **gate de cumplimiento** — puede estar puesto a propósito, para que la
cuenta del trabajo no acabe prestándole al proyecto personal. Ensancharlo sin avisar sería peor
que el problema que resuelve. Tres límites:

1. Se toca **solo la entrada del primario actual**, nunca otras. Un rewrite en bloque ensancharía
   permisos para repos en los que el usuario no está parado.
2. Se imprime **exactamente qué cambió en cada clave** (ver preview abajo).
3. `--no-allow` desactiva el comportamiento.

```
$ ccp auto chain add personal-deepseek

✔ fallback   app-cc → emco-cc → personal-deepseek
✔ allow_from personal-cc: +personal-deepseek

cadena efectiva desde este repo:
  app-cc → emco-cc → personal-deepseek
```

Validaciones: el perfil existe (`autoProfileExists`), no se duplica, y añadir el **primario** al
fallback se acepta en silencio (ya se filtra en `ResolveAutoChain`) pero se avisa de que es
implícito.

### D6 · `ccp session` autoconfigurable: preguntar una vez, luego automático

Al arrancar, si falta algo, se detecta en este orden y se muestra junto:

```
$ ccp session

Este repo no está configurado del todo:
  · regla    joseiz-poc → personal-cc   (crear)
  · sensores personal-cc, app-cc          (instalar)
  · política default                      (ok)

¿Configurar ahora? [S/n]
```

Qué se comprueba:

1. `auto_handoff` existe y `enabled` → si no, `AutoInit`
2. una regla cubre el cwd → si no, se ofrece crearla **sobre la raíz del repo git**, no sobre el
   cwd (una regla en un subdirectorio es casi siempre un error de dedo que luego confunde)
3. la cadena no queda vacía tras el gate → si sí, se ofrece ensanchar `allow_from`
4. sensores instalados en el primario y en cada miembro de la cadena → si no, `auto install`

**«Una vez» necesita memoria.** No va en `ccp.yaml`: es estado, no configuración. Va en
`state/auto/bootstrap/<hash-del-repo>.json`, bajo la carpeta que este proyecto ya define como
**caché** — borrarla cuesta calidad, nunca corrección. Si el usuario la borra, se le vuelve a
preguntar. Es el comportamiento correcto para una caché y hay que documentarlo así.

**El guard duro: sin tty, o con `-p`/`--headless`, no se pregunta ni se muta nada.** `ccp session -p`
corre desde cron. Un prompt ahí cuelga el job; una mutación silenciosa ahí cambia la config del
usuario sin que nadie lo vea. En ese modo se avisa por `stderr` y se sigue con el comportamiento
de hoy (fallar si `auto_handoff` no está configurado).

Flags: `--setup` fuerza el flujo aunque la caché diga que ya se preguntó; `--no-setup` lo salta.

### D7 · La TUI: una vista Config, no un cuarto panel

**Descartado: un cuarto panel** en la fila `Perfiles | Reglas | Config | Estado`. A 80 columnas los
tres actuales ya van justos; el cuarto los deja ilegibles.

**Elegido: una vista que toma el cuerpo del dashboard**, con la tecla `c` (o `:config`), siguiendo
el patrón que el panel Perfiles ya usa con `showDetail`. Secciones navegables:

- **Defaults** — `base_url`, `model_pro`, `model_flash`, `effort`, `editor`, `gui_editor`
- **Auto-handoff** — `enabled`, umbral, `min_dwell`, `max_hops`, `return_check`, `return_idle`, cooldown
- **Cadena** — lista reordenable con `J`/`K`, `a` añade, `d` quita
- **allow_from** — el gate del primario, con toggles
- **Sensores** — instalar/desinstalar por perfil

Cada campo abre un formulario `huh`, que es el mecanismo que la TUI ya tiene (`m.cur action`).
Además, `e` abre el editor gráfico de D4 desde dentro de la TUI.

**Esto fija el orden de las fases.** `CLAUDE.md` es explícito: *«The TUI only calls `internal/core`
— every action has a CLI equivalent»*. Así que la vista Config no puede existir antes que los
comandos que refleja. La TUI va **última**.

---

## 3. Plan de ejecución

| Fase | Qué | Archivos | Visible |
|---|---|---|---|
| **P0** | Primitivas puras: `RenderGauge`, `HumanUntil` + tests de tabla | `internal/core/gauge.go` | no |
| **P1** | La barra: medidor, color, reset, degradación por ancho | `internal/cli/auto.go`, i18n | **sí** |
| **P2** | `config edit` + `gui_editor` + validación post-edición | `internal/core/cfg_cmd.go`, `internal/cli/config.go` | sí |
| **P3** | `auto chain add/rm/mv/set/show` + `allow_from` | `internal/core/auto.go`, `internal/cli/auto.go` | sí |
| **P4** | Bootstrap de `session` (detectar → preguntar → aplicar → cachear) | `internal/cli/session.go`, `internal/core/autostate.go` | sí |
| **P5** | Vista Config en la TUI | `internal/tui/` | sí |
| **P6** | Completions + oráculo + golden + docs | `internal/core/shellinit.go`, `legacy/bin/ccp`, `testdata/golden/`, README | sí |

P1 va primero dentro de lo visible porque es lo que el usuario mira todos los días. P0 existe para
que P1 y P5 compartan el medidor sin duplicarlo.

---

## 4. Riesgos y rollback

### R1 · Las completions están en el golden gate ⚠️

`internal/core/shellinit.go:77` enumera los subcomandos de `auto` (`init install uninstall status
test`) y la línea 66 los de nivel superior. `legacy/bin/ccp:645` y `:669` los espejan, y
`completion bash|zsh` **es parte del contrato congelado** que verifica
`internal/golden/parity_test.go`.

Añadir `chain` a `auto` y `edit`/`gui-editor` a `config` rompe la parity **sí o sí**. No es un
efecto colateral evitable: es trabajo obligatorio de P6.

Secuencia, y el orden importa:

```bash
# 1. editar internal/core/shellinit.go
# 2. editar legacy/bin/ccp con el MISMO texto
bash legacy/tests/run.sh                      # el oráculo sigue verde
bash testdata/golden/capture.sh               # regenerar desde el oráculo
go test ./...                                 # parity verde ⇒ Go == bash
```

*Rollback:* `git checkout testdata/golden legacy internal/core/shellinit.go`.

### R2 · El statusLine no puede fallar nunca

Contrato duro de `CLAUDE.md`: `_statusline` **siempre** sale 0. Un medidor que hace pánico deja al
usuario sin barra y con un error recurrente en la UI. El `recover()` ya está; el código nuevo se
suma bajo él, y `RenderGauge` se escribe defensivo (porcentaje fuera de `[0,100]`, `cells<=0`,
duraciones negativas) con tests que cubren esos casos explícitamente.

*Rollback:* la barra vieja es una función de tres líneas (`autoUsageLabel`); volver a ella es un
revert de un commit.

### R3 · Escribir en `allow_from` es tocar un gate de cumplimiento

Mitigado en D5 (solo la entrada del primario, reporte explícito, `--no-allow`). El test que hay que
escribir por nombre: **añadir a la cadena no altera la entrada de ningún otro primario**.

*Rollback:* `ccp auto chain rm` deshace ambas claves.

### R4 · `ccp session` mutando config sola

Mitigado en D6 con el guard de tty/headless. El test por nombre: **con `--headless` y sin tty, el
bootstrap no escribe nada** — ni `ccp.yaml`, ni sensores, ni la caché.

*Rollback:* `--no-setup`, y la caché se borra con `rm -rf ~/.config/ccp/state/auto/bootstrap`.

### R5 · Claves nuevas en `ccp.yaml`

`defaults.gui_editor` es aditiva y el esquema **se queda en `version: 2`**. Subir la versión cobraría
un precio absurdo por una clave que el binario viejo no necesita entender.

**Corregido tras implementar.** La versión original de este riesgo decía que un binario viejo la
conserva vía `Config.Extra`. Es falso, y conviene tenerlo claro porque cambia lo que se puede
prometer: `Extra` es el catch-all de **nivel superior** (`Config`), y `Defaults` no lleva
`yaml:",inline"`. Un ccp anterior a esta clave **lee** el archivo sin quejarse —que es la parte que
de verdad importa, y esa sí se cumple—, pero **la borra en la primera escritura** de cualquier
comando (`config set`, `path set`, `profile add`…). Con dos máquinas en versiones distintas, el
ajuste se pierde en silencio cada vez que escribe la vieja.

Se acepta la degradación en vez de mover `gui_editor` al nivel superior: es una preferencia de UI
**reconstruible** —vacía significa «autodetecta», y la cadena de seis escalones vuelve a resolver—,
y sacarla de `defaults` la separaría de `editor`, que es exactamente su gemela. Lo que **no** puede
pasar es que la misma degradación afecte a `auto_handoff`: ahí las claves desconocidas sí sobreviven,
porque el bloque entero viaja en `Extra`.

### R6 · Unicode y terminales

`█░▏` fallan en fuentes muy pobres. El nivel `<40` de D1 ya no lleva medidor, así que existe una
salida; si aparece un reporte, `CCP_STATUSLINE_PLAIN=1` fuerza ese nivel. No se implementa la
variable hasta que haga falta — es plan de contingencia, no feature.

---

## 5. Qué no se toca

- El contrato congelado en su comportamiento: `_env`, `_hook`, `resolve`, `path test`,
  `status --json`. Solo cambia el **texto de las completions** (R1).
- La forma de `auto status --json`. Se pueden **añadir** campos (`resets_in`), nunca quitar;
  `fallback`/`denied`/`sensors` siguen siendo arrays, nunca `null`.
- `HandoffsVersion` y el esquema de `handoffs.yaml`.
- La invariante del supervisor: nunca dos hijos vivos.

---

## 6. Verificación

```bash
gofmt -l internal cmd            # vacío
go vet ./...
go test ./...                    # incluye parity
bash legacy/tests/run.sh
bash testdata/golden/capture.sh --check
go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2 run --timeout 5m
```

Manual, con `CCP_HOME` temporal para no tocar la config real:

```bash
export CCP_HOME=$(mktemp -d)
ccp auto init
ccp auto chain add emco-cc
ccp auto status
COLUMNS=40 ccp _statusline </dev/null    # nivel medio
COLUMNS=30 ccp _statusline </dev/null    # sin medidor
NO_COLOR=1 COLUMNS=100 ccp _statusline </dev/null
```

Tests nuevos, nombrados por la regla que fijan:

- `TestRenderGaugeExtremos` — 0, 100, fuera de rango, `cells<=0`
- `TestHumanUntilOmiteVencido` — un `resets_at` pasado no produce texto
- `TestStatusLineDegradaPorAncho` — los tres niveles
- `TestStatusLineSiempreSaleCero` — con stdin hostil
- `TestChainAddNoTocaOtrosPrimarios` — R3
- `TestSessionHeadlessNoEscribeNada` — R4
- `TestConfigEditValidaTrasCerrar` — yaml roto ⇒ error nombrando la clave
