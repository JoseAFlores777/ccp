# Vista de perfil en la TUI, y un shell de vista compartido

**Fecha:** 2026-09-09
**Estado:** diseño aprobado, pendiente de plan de implementación

## El problema

La tecla `e` del panel Perfiles abre `$EDITOR` sobre los dos archivos del overlay
del perfil. Para alguien que la pulsa sin un cambio concreto en la cabeza —el
caso que originó este diseño— eso es un editor sobre un `CLAUDE.md` de 0 bytes:
no dice nada del perfil, no dice qué aplica, y no da por dónde empezar.

`enter` tampoco lo cubre: muestra `core.ProfileShow` (tipo, base URL, modelos,
effort, API key, config dir, login), que es la **identidad** del perfil. Lo que
no existe en ninguna parte es la respuesta a *«qué configuración aplica este
perfil de verdad, y de dónde sale cada cosa»*.

Hay además un problema de forma. Hoy hay tres renderizadores de TUI escritos a
mano que repiten la misma idea sin compartir código:

| vista | cabecera | cajas | estado | pie |
|---|---|---|---|---|
| dashboard | `logoBanner` | `box` ×3 fijas | copiado | `tui.footer.keys` |
| Config (`c`) | `ccp` + eyebrow | `boxFocused` ×5, la enfocada expandida | copiado | `tui.config.footer` |
| panel de handoff | header propio | sin cajas, lista suelta | — | — |

Una cuarta vista escrita igual empeoraría eso. El repo ya tiene el argumento
escrito en `internal/supervisor/trace.go`: dos entry points pintando el mismo
evento de dos formas es exactamente el bug que un formateador compartido evita.

## Qué se construye

1. Un **shell de vista** compartido: el único sitio que pinta cabecera, cajas,
   cursor, ventana, línea de estado y pie.
2. Una **vista de perfil** (`modeProfile`) construida sobre ese shell, con el
   chrome del dashboard: logo, tres cajas, `▸` marcando foco y fila, pie de
   teclas.
3. El **retrofit** del dashboard y de la vista Config al mismo shell.
4. Dos piezas nuevas en `core`: el cálculo de procedencia y la escritura de
   variables de entorno en el overlay.

El panel de handoff queda fuera: es otro `tea.Program`, sobre `/dev/tty`, con
otro ciclo de vida. Tocarlo sería riesgo sin demanda.

## Decisiones y por qué

### El shell posee el chrome; la vista posee el contenido

```go
// internal/tui/shell.go

type rowSpec struct {
    Text   string // ya alineada por la subvista: las columnas son cosa suya
    Marker string // "•" u otra marca opcional, antes del texto
}

type panelSpec struct {
    Title   string
    Hint    string     // línea de teclas; solo se pinta con foco
    Rows    []rowSpec
    Cursor  int        // fila bajo el cursor; -1 = ninguna
    Summary string     // línea única cuando la caja NO tiene foco
    Empty   string     // texto cuando Rows está vacío
    Focused bool
}

type viewSpec struct {
    Header    string // logoBanner(...) o brand + eyebrow
    Panels    []panelSpec
    Status    string
    StatusErr bool
    Extra     string // barra de comandos, confirmaciones: lo que no es panel
    Footer    string
}

func (m *model) renderView(v viewSpec) string
```

El corte está en **chrome contra contenido**. Alinear columnas es de la vista
(la Config usa `labelW = 16`, la de perfil querrá otra cosa); pintar el cursor,
la caja, el recorte y el pie es del shell. Si el cursor lo pintara cada vista,
volveríamos a tener cuatro `▸` distintos, que es el estado del que salimos.

`Rows` son strings ya formateadas y no datos estructurados a propósito: el shell
no debe saber qué es una regla, una variable de entorno o un permiso. Lo único
que necesita saber es cuántas filas hay y cuál está bajo el cursor.

**Descartado:** una interfaz `View` con métodos (`Panels()`, `Footer()`) que cada
vista implemente. Suena más limpio y da peor resultado: las vistas comparten
`*model`, así que la interfaz acabaría recibiendo el modelo por parámetro o
duplicando estado. Un struct de datos que la vista rellena y una función que lo
pinta es menos ceremonia y se testea sin montar un modelo.

### La ventana vive en el shell

Cuando una caja tiene más filas de las que caben, el shell renderiza una
**ventana alrededor del cursor**, con `↑ N más` / `↓ N más` en los bordes. `j`/`k`
mueven el cursor y la ventana lo sigue.

Va en el shell y no en cada vista por dos razones. Una: `permissions.allow` del
usuario tiene ~200 entradas y ninguna vista puede pintarlas; si cada una resuelve
eso a su manera tendremos tres soluciones distintas al mismo problema. Dos: la
vista Config ya tiene el mismo agujero latente hoy —un `allow_from` largo se
sale de la pantalla— y ponerlo en el shell lo arregla de paso.

**Descartado:** un viewport de bubbles. El dashboard se diseñó sin scroll a
propósito (está escrito en `config_view.go`: a 80 columnas los tres paneles ya
van justos) y meter un componente con su propio estado de scroll cambia el modelo
de navegación de toda la TUI para resolver un caso.

### `core.ProfileEffective`: la procedencia como dato

```go
// internal/core/effective.go

type Origin int // OriginGlobal | OriginOverlay | OriginAuto

type EffRow struct {
    Key      string // "CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS", "SessionStart", "Bash(...)"
    Value    string
    Origin   Origin // la capa que ganó
    Shadowed bool   // otra capa traía la misma clave y perdió
}

type EffSection struct {
    Kind EffKind // Instrucciones | Env | Permisos | Hooks | Plugins | Sensores
    Rows []EffRow
    File string  // el archivo que 'e' abre en esta sección; "" si no hay
                 // (Sensores no vive en un archivo editable: su fuente de
                 //  verdad es auto_handoff.hooks en ccp.yaml, y se cambia
                 //  desde la vista Config con 'c')
    Err  error   // esta sección no se pudo leer; el resto sigue
}

type Effective struct {
    Profile  string
    Sections []EffSection
}

func ProfileEffective(home, name, src string) (Effective, error)
```

Devuelve datos, no texto: la regla que ya sigue el resto de `core`. La única
razón por la que esto es barato es que el motor ya está partido en esas capas.
Las instrucciones no se mergean —`cfgWriteClaudeMD` escribe dos `@import`,
global y overlay—, así que su procedencia es por archivo. Los settings sí:
`MergeJSON(global, overlay)` y después `applyAutoLayer`. Tres capas, orden
conocido, recorrerlas otra vez es determinista.

`Shadowed` es un booleano y no la cadena de valores perdidos. Saber que el
overlay pisó al global es accionable; saber qué valor traía el global es
curiosidad, y guardarlo obligaría a decidir cómo se presenta.

`Err` es por sección y no por `Effective`: un `settings.overlay.json` roto no
puede impedir que se vean las instrucciones, que están en otro archivo.

### `core.OverlayEnvSet` / `OverlayEnvDel`

```go
// internal/core/overlay_env.go
func OverlayEnvSet(home, name, key, val string) error
func OverlayEnvDel(home, name, key string) error
```

Leen el overlay, mutan `.env`, **validan el JSON antes de escribir**, escriben, y
regeneran el cc-home. Mismo invariante que `ProfileConfig`: si el resultado no
valida, el último bueno no se toca.

Es la única API nueva de escritura. El resto reusa lo que hay:

- **Reglas** → `InstructRuleList/Add/Rm(file)`, que ya son puras sobre un archivo.
  Usarlas directamente esquiva `InstructCtx` y con él el problema del «perfil
  activo»: la vista opera sobre el perfil **seleccionado**, no sobre el de la
  terminal.
- **Hooks** → `InstructAdd` con `ctx.ActiveProfile` = el seleccionado.

### Los hooks se añaden pero no se borran, y la vista lo dice

`InstructRm` no borra la entrada JSON de un hook: los hooks viven en arrays sin
id estable, así que solo lo saca del manifiesto. La vista no ofrece `d` sobre un
hook fingiendo que borra; responde explicando por qué no puede. Una tecla que
explica es mejor que una que miente.

### `e` abre un solo archivo

Dentro de la vista, `e` abre el archivo de la caja donde está el foco, no los dos
overlays. Esto es posible desde el arreglo del mismo día: `ProfileConfig` pasaba
los dos archivos en una sola invocación del editor, y en macOS `nano` es un
symlink a `pico`, que abre el primer argumento y **descarta el resto sin avisar**.

## La vista de perfil

Entrada: `e` en el panel Perfiles. Salida: `esc`, que devuelve el foco al
dashboard donde estaba.

```
        [logo ccp]
        v2.0.0 — profiles for Claude Code
        perfil emco-cc · official · ~/.config/ccp/profiles/emco-cc/cc-home

╭──────────────────────────────────────────────────────────╮
│ ▸ Instrucciones  a:añadir d:borrar e:editar archivo      │
│ ▸ global   ~/.claude/CLAUDE.md              4.2 KB       │
│   (sin reglas propias — 'a' para añadir)                 │
╰──────────────────────────────────────────────────────────╯
╭──────────────────────────────────────────────────────────╮
│   Env  a:añadir enter:editar d:borrar e:editar archivo   │
│   CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS = 1     overlay   │
╰──────────────────────────────────────────────────────────╯
╭──────────────────────────────────────────────────────────╮
│   Efectivo  enter:desplegar a:añadir hook                │
│   Permisos    203 entradas              overlay          │
│   Hooks        11 eventos      overlay ⊕ auto            │
│   Plugins       8                       overlay          │
│   Sensores     instalados                   auto         │
╰──────────────────────────────────────────────────────────╯

tab: panel · j/k: navegar · e: editar archivo · esc: volver · q: salir
```

Tres cajas, espejo de Perfiles | Reglas | Estado:

| caja | filas | archivo de `e` | acciones |
|---|---|---|---|
| **Instrucciones** | el global (`~/.claude/CLAUDE.md`, tamaño) + cada regla del bloque gestionado del overlay | `overlay/CLAUDE.md` | `a` · `d` |

La primera fila de **Instrucciones** —el `~/.claude/CLAUDE.md` global— es de solo
lectura: `d` sobre ella no borra, responde que esa es la config global y no el
overlay del perfil. Misma regla que el hook: la tecla explica en vez de fingir.
| **Env** | una variable por fila, con origen | `overlay/settings.overlay.json` | `a` · `enter` · `d` |
| **Efectivo** | plegada: permisos, hooks, plugins, sensores con sus conteos | `overlay/settings.overlay.json` | `enter` despliega · `a` añade hook |

**Efectivo** arranca plegada en cuatro filas de conteo; `enter` despliega un grupo
en sus filas reales con su columna de origen, y ahí entra la ventana del shell.
Plegada por defecto porque las cuatro cifras son la respuesta a *«qué aplica este
perfil»*; las 203 entradas son el detalle que se pide cuando se quiere.

## Casos borde

- **`default`** no tiene overlay ni cc-home. La vista abre igual, en modo
  solo-global: las tres cajas muestran lo que hay en `~/.claude`, y las teclas de
  escritura responden con el mismo texto que ya da `ProfileConfig` (*«'default' =
  tu config GLOBAL; edítala directamente, no tiene overlay»*). Abrir no es un
  error.
- **Overlay ausente** → se ve vacío. `CfgInitOverlay` corre en el primer *write*,
  nunca al mirar. Inspeccionar no crea archivos.
- **`settings.overlay.json` inválido** → esa caja muestra el error en su fila
  (`EffSection.Err`) y desactiva su escritura; las otras siguen. La vista nunca
  revienta por un JSON malo.
- **Global ausente o inválido** → se ignora, igual que hace hoy
  `cfgMergeSettings`.
- **Fallo al regenerar tras un write** → sale por la línea de estado, y el
  overlay ya quedó escrito: mismo comportamiento que hoy tiene `ProfileConfig`.

## Testing

**El retrofit tiene que ser demostrablemente idéntico en salida.** Antes de tocar
`viewDashboard` y `viewConfig` se capturan sus renders actuales a
`internal/tui/testdata/render/*.golden` con fixtures fijos, en ES y EN. El
retrofit es correcto cuando esos goldens siguen pasando. Es la misma idea que ya
usa `internal/golden/parity_test.go` con el oráculo bash, aplicada a la TUI.

- **`shell.go`** — ventana con más filas que alto (bordes: cursor en la primera,
  en la última, lista más corta que la ventana), cursor, caja sin foco con
  `Summary`, caja vacía con `Empty`, línea de estado en sus dos colores.
- **`core.ProfileEffective`** — tabla: solo global · solo overlay · ambos con
  override (fija `Shadowed`) · capa auto encendida y apagada · `default` · JSON
  roto (comprueba que `Err` es por sección) · global ausente.
- **`core.OverlayEnvSet/Del`** — alta, sobrescritura, borrado de clave
  inexistente, JSON inválido que **no** destruye el último bueno, y que regenera
  el cc-home.
- **`profile_view.go`** — render en ES y EN sin claves `tui.` crudas, navegación
  `tab`/`j`/`k`, cada acción pegando en el camino de `core`, y que `e` abre
  **un** archivo.

## Lo que no cambia

- El contrato golden del CLI (`_env`, `_hook`, `resolve`, `path test`,
  `status --json`, `completion`). Esta vista no lo toca.
- `ccp profile config <perfil>` en el CLI sigue abriendo el editor.
- El panel de handoff.
- El esquema de `ccp.yaml`: no hay claves nuevas.

## Efecto lateral que vale la pena nombrar

Tres de los cuatro overlays del usuario pesan 11 806 B y son **byte a byte
idénticos**: `enabledPlugins`, `extraKnownMarketplaces`, `theme`, once tipos de
`hooks` y un `permissions.allow` enorme. Eso no es un overlay —un delta de menor
precedencia por perfil— sino una copia del `~/.claude/settings.json` global,
duplicada tres veces.

La vista no lo arregla. Lo hace **visible**: esas filas van a aparecer marcadas
`overlay` y casi todas `Shadowed`. Ver el problema es el paso previo a decidir
qué hacer con él, y es trabajo aparte.
