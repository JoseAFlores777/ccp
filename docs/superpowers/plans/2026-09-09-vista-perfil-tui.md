# Vista de perfil en la TUI — plan de implementación

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** que `e` sobre un perfil abra una vista con el chrome del dashboard que
responda «qué configuración aplica este perfil y de dónde sale cada cosa», con
edición de lo que `core` ya sabe escribir.

**Architecture:** primero un *shell de vista* compartido en `internal/tui` que
pasa a ser el único sitio que pinta cabecera, cajas, cursor, ventana, estado y
pie; el dashboard y la vista Config se pasan a él y la vista de perfil nace
encima. Detrás, `core` gana dos piezas: `ProfileEffective` (la procedencia como
dato) y `OverlayEnvSet/Del` (la única API de escritura nueva).

**Tech Stack:** Go 1.24 · bubbletea v1.3.10 · lipgloss · huh (formularios ya
existentes) · sin cobra, el dispatch es a mano.

**Spec:** `docs/superpowers/specs/2026-09-09-vista-perfil-tui-design.md`

## Global Constraints

- **Commits: nunca autónomos.** El paso «Commit» de cada tarea significa *dejar
  el árbol listo, mostrar el mensaje propuesto y esperar autorización explícita
  del usuario en ese turno*. Es una regla global del usuario y gana sobre este
  plan. Nunca añadir trailers `Co-Authored-By` ni ninguna atribución a
  herramientas.
- **Gates de CI, todos verdes antes de proponer commit:** `gofmt -l internal cmd`
  no imprime nada · `go vet ./...` · `go test ./...` · `go run
  github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2 run --timeout 5m`.
- **Bilingüe obligatorio.** Cada texto de usuario nuevo va como clave en
  `internal/core/i18n/catalog_tui.go` con sus dos campos `En:` y `Es:`. Nunca
  literales en el render. `TestDashboardRendersBothLangs` falla si queda una
  clave `tui.` sin traducir.
- **`internal/core` no hace I/O de presentación.** Devuelve datos o strings
  exactos; formatear es de `internal/tui` y `internal/cli`.
- **Tests nunca tocan el estado real.** Siempre `t.TempDir()` como `home` y
  `t.Setenv("CCP_CLAUDE_SRC", t.TempDir())`. Nada puede leer ni escribir
  `~/.config/ccp` ni `~/.claude`.
- **El contrato golden no se toca.** `_env`, `_hook`, `resolve`, `path test`,
  `status --json`, `completion bash|zsh`, `completion-shellinit`. Ninguna tarea
  de este plan los modifica; `internal/golden/parity_test.go` debe seguir verde.
## Estructura de archivos

| archivo | responsabilidad |
|---|---|
| `internal/tui/shell.go` **(nuevo)** | `rowSpec`/`panelSpec`/`viewSpec` y `renderView`. El único sitio que pinta chrome. |
| `internal/tui/shell_test.go` **(nuevo)** | cursor, caja sin foco, caja vacía, ventana. |
| `internal/tui/render_golden_test.go` **(nuevo)** | red de seguridad del retrofit: compara el render de dashboard y Config con `testdata/render/*.golden`. |
| `internal/tui/testdata/render/*.golden` **(nuevo)** | los renders capturados antes de refactorizar. |
| `internal/core/effective.go` **(nuevo)** | `ProfileEffective`: las tres capas, la procedencia como dato. |
| `internal/core/effective_test.go` **(nuevo)** | tabla de casos de procedencia. |
| `internal/core/overlay_env.go` **(nuevo)** | `OverlayEnvSet` / `OverlayEnvDel`. |
| `internal/core/overlay_env_test.go` **(nuevo)** | alta, sobrescritura, borrado, JSON roto no destruye. |
| `internal/tui/profile_view.go` **(nuevo)** | `modeProfile`: estado, teclas, y los tres `panelSpec`. |
| `internal/tui/profile_view_test.go` **(nuevo)** | render bilingüe, navegación, acciones. |
| `internal/tui/dashboard.go` | `viewDashboard` pasa por el shell; `e` abre la vista de perfil. |
| `internal/tui/config_view.go` | `viewConfig` pasa por el shell. |
| `internal/tui/tui.go` | `modeProfile` en el enum, campos del modelo, dispatch de `Update`/`View`. |
| `internal/core/i18n/catalog_tui.go` | claves nuevas, `En` + `Es`. |

---

### Task 1: Red de seguridad — goldens de render

Antes de refactorizar nada hay que poder demostrar que el refactor no cambia la
salida. Esta tarea captura el render actual del dashboard y de la vista Config a
archivos, con un flag `-update` para regenerarlos a propósito.

**Files:**
- Create: `internal/tui/render_golden_test.go`
- Create: `internal/tui/testdata/render/` (los `.golden` los genera el propio test)

**Interfaces:**
- Consumes: `newTestModel` (`internal/tui/lang_test.go:13`), `seedConfigHome` y
  `modelOn` (`internal/tui/config_view_test.go:23` y `:48`).
- Produces: `goldenRender(t, name, got string)` — helper que las tareas 4 y 5
  reusan sin cambios.

- [ ] **Step 1: Escribir el test que captura y compara**

```go
package tui

import (
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/JoseAFlores777/ccp/internal/core"
	"github.com/JoseAFlores777/ccp/internal/core/i18n"
)

var updateGolden = flag.Bool("update", false, "regenerar los .golden de render")

// goldenRender compara un render con su archivo en testdata/render, o lo escribe
// si se pasó -update. Es la red del retrofit al shell: el refactor es correcto
// cuando estos archivos no se mueven.
func goldenRender(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", "render", name+".golden")
	if *updateGolden {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir testdata: %v", err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatalf("escribir %s: %v", path, err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("falta el golden %s (genéralo con: go test ./internal/tui/ -update): %v", path, err)
	}
	if got != string(want) {
		t.Fatalf("el render de %q cambió.\n--- quiero ---\n%s\n--- tengo ---\n%s", name, want, got)
	}
}

// dashboardFixture arma un modelo determinista: dos perfiles, dos reglas y unas
// dimensiones fijas. Sin fixture fijo el golden no vale nada.
func dashboardFixture(t *testing.T) *model {
	t.Helper()
	t.Setenv("CCP_CLAUDE_SRC", t.TempDir())
	home := t.TempDir()
	for _, n := range []string{"a-cc", "b-cc"} {
		if err := core.ProfileAddOfficial(home, n); err != nil {
			t.Fatalf("ProfileAddOfficial(%s): %v", n, err)
		}
	}
	// RuleSet devuelve (mensaje, error): el mensaje es para el CLI, aquí sobra.
	if _, err := core.RuleSet(home, "/repo/uno", "a-cc"); err != nil {
		t.Fatalf("RuleSet: %v", err)
	}
	if _, err := core.RuleSet(home, "/repo/dos", "b-cc"); err != nil {
		t.Fatalf("RuleSet: %v", err)
	}
	m := &model{home: home, focus: panelProfiles, mode: modeDashboard, width: 100, height: 40}
	m.reload()
	// El panel Estado NO es hermético por sí solo: viewStatus() dispara
	// refreshEstado() -> computeEstado(), que lee os.Getwd(), os.Getenv("CCP_PROFILE")
	// y `git rev-parse --show-toplevel` del cwd real (internal/tui/status.go:25,30,52).
	// Sin fijarlo aquí, el golden queda con la ruta absoluta de QUIEN lo generó — pasa
	// en este checkout, falla en cualquier otro (incluido el runner de CI, que clona en
	// /home/runner/work/ccp/ccp) y `-count=2` no lo detecta porque las dos pasadas
	// corren en el mismo proceso con el mismo cwd. Fijar el snapshot a mano evita que
	// viewStatus recompute: computeEstado sólo corre `if !m.estComputed`
	// (internal/tui/dashboard.go:653).
	m.est = estado{Active: "default", Profile: "default", ProfileType: "default", Cwd: "/fixture/repo", Repo: ""}
	m.estComputed = true
	return m
}

func TestGoldenRenderDashboard(t *testing.T) {
	for _, lang := range []i18n.Lang{i18n.Es, i18n.En} {
		t.Run(string(lang), func(t *testing.T) {
			m := dashboardFixture(t)
			m.lang = lang
			goldenRender(t, "dashboard-"+string(lang), m.viewDashboard())
		})
	}
}

func TestGoldenRenderConfig(t *testing.T) {
	for _, lang := range []i18n.Lang{i18n.Es, i18n.En} {
		for _, sec := range configSections {
			t.Run(string(lang)+"-"+configTitleKey(sec), func(t *testing.T) {
				home := seedConfigHome(t, []string{"b-cc"}, nil)
				// La fila `gui_editor` de la sección Defaults se autodetecta si no se
				// fija: rowsDefaults -> m.editChoice() -> core.ResolveEditEditor(GOOS,
				// PATH, $VISUAL) (internal/tui/config_view.go:257-263,796-807), y
				// resuelve distinto según el SO y lo que haya instalado ("open -W -t" en
				// darwin, "xdg-open" en el resto, o code/cursor si están en PATH). Sin
				// fijarlo el golden depende de la máquina que lo generó.
				if err := core.SetGuiEditor(home, "nano"); err != nil {
					t.Fatalf("SetGuiEditor: %v", err)
				}
				t.Setenv("VISUAL", "")
				m := modelOn(t, home, sec, lang)
				goldenRender(t, "config-"+string(lang)+"-"+configTitleKey(sec), m.viewConfig())
			})
		}
	}
}
```

- [ ] **Step 2: Correr el test sin goldens para verificar que falla**

Run: `go test ./internal/tui/ -run TestGoldenRender -v`
Expected: FAIL en todos los subtests con `falta el golden testdata/render/...`

- [ ] **Step 3: Generar los goldens**

Run: `go test ./internal/tui/ -run TestGoldenRender -update`
Expected: PASS

- [ ] **Step 4: Mirar un golden con ojos humanos**

Run: `cat internal/tui/testdata/render/dashboard-es.golden`
Expected: se ve el logo, las tres cajas con sus bordes, la caja Perfiles marcada
con `▸`, y el pie de teclas. Si sale vacío o sin cajas, el fixture está mal y no
sirve como red — arréglalo antes de seguir. La línea `Cwd` tiene que decir
`/fixture/repo`, nunca una ruta real del disco — si dice otra cosa, el fixture no
fijó `m.est` a tiempo.

- [ ] **Step 5: Verificar que es estable ENTRE MÁQUINAS, no solo dentro del mismo proceso**

`-count=2` no basta: las dos pasadas corren en el mismo proceso, mismo cwd, mismo
entorno — cazan no-determinismo intra-proceso (orden de mapas), no el que de verdad
importa aquí (rutas y entorno de la máquina que generó el golden). La prueba real es
hermeticidad: que el resultado no cambie con el `PWD` ni el `CCP_PROFILE` de quien
corre el test.

Run: `go test ./internal/tui/ -run TestGoldenRender -count=2`
Expected: PASS (esto solo descarta no-determinismo intra-proceso, p. ej. mapas
recorridos sin ordenar — no es la garantía de hermeticidad)

Run: `CCP_PROFILE=zzz go test ./internal/tui/ -run TestGoldenRender -v`
Expected: PASS igual. Si falla aquí, `m.est` no está fijo de verdad en el fixture.

Run: `grep -rn "$PWD" internal/tui/testdata/render/`
Expected: sin resultados. Si aparece algo, el cwd real se filtró al golden.

- [ ] **Step 6: Commit (pedir autorización antes de ejecutarlo)**

```bash
git add internal/tui/render_golden_test.go internal/tui/testdata/render
git commit -m "test(tui): goldens de render del dashboard y de la vista Config"
```

---

### Task 2: El shell de vista

**Files:**
- Create: `internal/tui/shell.go`
- Create: `internal/tui/shell_test.go`
- Modify: `internal/core/i18n/catalog_tui.go` (dos claves de la ventana)

**Interfaces:**
- Consumes: `boxFocused` (`internal/tui/dashboard.go:399`), y los estilos
  `styleDim`, `styleFocused`, `styleOK`, `styleErr`, `styleCheck`, `styleCross`
  (`internal/tui/tui.go:395-403`). Nota: `innerWidth`/`truncRight` NO los usa
  `shell.go` — los sigue usando cada vista, para dimensionar su propio texto
  antes de estilizarlo (ver el contrato de `rowSpec.Text` más abajo).
- Produces: `rowSpec`, `panelSpec`, `viewSpec` y `(*model).renderView(viewSpec) string`.
  Las tareas 4, 5 y 8 construyen esos structs y llaman a `renderView`.

**Contrato de `rowSpec.Text`: la vista lo entrega YA TERMINADO — el shell no lo
vuelve a truncar ni a estilizar.** `profileRow`, `configRowLine` y las filas de
`viewRules`/`viewStatus` YA hacen hoy lo correcto por su cuenta: truncan cada
segmento en plano (`truncRight`/`truncLeft` sobre texto sin ANSI) y RECIÉN
ENTONCES lo estilizan — ese orden nunca cambia. El shell no puede repetir ese
truncado con `m.innerWidth()` porque la fila ya viene dimensionada a ese mismo
ancho por quien la construyó; hacerlo una segunda vez sobre texto que ya trae
secuencias `\x1b[...m` corta dentro de un escape (queda roto en pantalla), y
envolver ese texto en otro `st.Render(...)` es peor: el `\x1b[0m` de reset que
ya trae el texto interior mata el estilo externo antes de llegar al contenido,
así que la fila seleccionada deja de resaltarse. El shell se limita a anteponer
el cursor — nada más. Lo único que sí colorea el shell es `rowSpec.Marker`
(coloreado por PREFIJO: verde si empieza con "✓", rojo si empieza con "✗",
sirve tanto para una marca suelta como para una frase entera tipo
"✓ logueado"), porque es un campo nuevo que ninguna vista pre-estiliza.

- [ ] **Step 1: Escribir el test del shell**

```go
package tui

import (
	"strings"
	"testing"

	"github.com/JoseAFlores777/ccp/internal/core/i18n"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func shellModel() *model {
	return &model{width: 100, height: 40, lang: i18n.Es}
}

func TestPanelSinFocoSoloEnseñaElResumen(t *testing.T) {
	m := shellModel()
	out := m.renderPanel(panelSpec{
		Title:                 "Env",
		Hint:                  "a:añadir",
		Rows:                  []rowSpec{{Text: "FOO = 1"}},
		Summary:               "3 variables",
		CollapseWhenUnfocused: true, // el comportamiento de hoy en Config; el dashboard y la vista de perfil dejan esto en false
	})
	if !strings.Contains(out, "3 variables") {
		t.Fatalf("una caja sin foco tiene que enseñar su resumen: %q", out)
	}
	if strings.Contains(out, "FOO = 1") || strings.Contains(out, "a:añadir") {
		t.Fatalf("una caja sin foco no pinta filas ni teclas: %q", out)
	}
}

func TestPanelVacioUsaElTextoEmpty(t *testing.T) {
	m := shellModel()
	out := m.renderPanel(panelSpec{
		Title: "Env", Hint: "a:añadir", Focused: true,
		Empty: "(sin variables — 'a' para añadir)",
	})
	if !strings.Contains(out, "sin variables") {
		t.Fatalf("una caja enfocada y vacía tiene que decirlo: %q", out)
	}
}

func TestCursorLoPintaElShellYNoLaVista(t *testing.T) {
	m := shellModel()
	out := m.renderPanel(panelSpec{
		Title: "Env", Focused: true, Cursor: 1,
		Rows: []rowSpec{{Text: "AAA"}, {Text: "BBB"}, {Text: "CCC"}},
	})
	lineas := strings.Split(out, "\n")
	var conCursor []string
	for _, l := range lineas {
		if strings.Contains(l, "▸") && !strings.Contains(l, "Env") {
			conCursor = append(conCursor, l)
		}
	}
	if len(conCursor) != 1 || !strings.Contains(conCursor[0], "BBB") {
		t.Fatalf("el cursor tiene que estar en una sola fila, la 1 (BBB): %v", conCursor)
	}
}

func TestRenderViewMontaCabeceraEstadoYPie(t *testing.T) {
	m := shellModel()
	out := m.renderView(viewSpec{
		Header: "CABECERA",
		Panels: []panelSpec{{Title: "Uno", Summary: "resumen", CollapseWhenUnfocused: true}},
		Status: "salió bien",
		Footer: "q: salir",
	})
	for _, quiero := range []string{"CABECERA", "resumen", "salió bien", "q: salir"} {
		if !strings.Contains(out, quiero) {
			t.Fatalf("falta %q en el render:\n%s", quiero, out)
		}
	}
	if i := strings.Index(out, "CABECERA"); i > strings.Index(out, "resumen") {
		t.Fatal("la cabecera va antes que los paneles")
	}
}

// TestRowLineNoRetruncaNiReestilizaElTextoDeLaFila fija el contrato central del
// shell: rowLine NO vuelve a tocar r.Text, solo le antepone el cursor. La
// vista ya lo truncó y estilizó con su propio ancho (exactamente como hoy
// profileRow/configRowLine truncan cada segmento en plano antes de
// estilizarlo) — si rowLine repitiera ese truncado con m.innerWidth() sobre un
// texto que YA trae secuencias `\x1b[...m`, cortaría dentro de un escape.
func TestRowLineNoRetruncaNiReestilizaElTextoDeLaFila(t *testing.T) {
	m := shellModel()
	preEstilizado := "\x1b[1;38;2;201;100;65mnombre-perfil\x1b[0m        \x1b[38;2;63;155;80moficial\x1b[0m"
	out := m.rowLine(rowSpec{Text: preEstilizado}, false)
	if !strings.HasSuffix(out, preEstilizado) {
		t.Fatalf("rowLine tocó el texto de la fila; lo quiero intacto tras el cursor:\nquiero sufijo: %q\ntengo:        %q", preEstilizado, out)
	}
}

// TestRowLineColoreaLaMarcaPorPrefijo: Marker es el único campo que el shell sí
// colorea — por prefijo, así que sirve tanto para un glifo suelto como para una
// frase entera ("✓ logueado").
func TestRowLineColoreaLaMarcaPorPrefijo(t *testing.T) {
	old := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(old)

	m := shellModel()
	out := m.rowLine(rowSpec{Text: "algo", Marker: "✓ logueado"}, false)
	if !strings.Contains(out, "\x1b[") {
		t.Fatalf("una marca que empieza con ✓ tiene que salir coloreada: %q", out)
	}
	if !strings.Contains(out, "✓ logueado") {
		t.Fatalf("la marca completa tiene que aparecer: %q", out)
	}
}
```

- [ ] **Step 1b: gofmt antes de seguir**

`gofmt` alinea los valores de un literal de struct en líneas consecutivas — el
bloque de arriba ya está alineado a mano, pero si al pegarlo el editor
reindenta algo, correr esto ahora ahorra un gate rojo más tarde:

Run: `gofmt -w internal/tui/shell_test.go`

- [ ] **Step 2: Correr y verificar que falla**

Run: `go test ./internal/tui/ -run 'TestPanel|TestCursor|TestRenderView|TestRowLine' -v`
Expected: FAIL con `undefined: panelSpec`, `undefined: viewSpec`,
`m.renderPanel undefined`, `m.renderView undefined`, `m.rowLine undefined`

- [ ] **Step 3: Escribir el shell**

```go
package tui

// shell.go — el único sitio de la TUI que pinta chrome: cabecera, cajas,
// cursor, ventana, línea de estado y pie.
//
// El corte es chrome contra contenido. Alinear columnas es de cada vista (la
// Config usa labelW=16, la de perfil usa otra cosa); pintar el cursor, la caja,
// el recorte y el pie es de aquí. Antes había tres renderizadores a mano
// repitiendo la misma idea, y el cuarto iba a ser la vista de perfil — el mismo
// argumento que traceMove en internal/supervisor/trace.go: dos entry points
// pintando el mismo evento de dos formas ES el bug.

import (
	"strings"

	"github.com/JoseAFlores777/ccp/internal/core/i18n"
)

// rowSpec es una fila ya formateada por la vista. El shell no sabe si es una
// regla, una variable o un permiso: solo cuántas hay y cuál lleva el cursor.
//
// Text es SIEMPRE texto plano, sin ANSI — ver el contrato explicado más arriba
// en este plan. Marker es opcional y solo el shell lo colorea (hoy solo "✓"/"✗"
// llevan color; cualquier otro valor se pinta sin estilo).
type rowSpec struct {
	Text   string
	Marker string
}

// panelSpec es una caja.
type panelSpec struct {
	Title   string
	Hint    string // línea de teclas; solo se pinta con foco
	Rows    []rowSpec
	// Cursor es la fila bajo el cursor. -1 = ninguna fila seleccionable (p. ej.
	// un panel de solo lectura sin filas navegables). OJO: el valor cero de Go
	// es 0, NO -1 — un panel sin cursor que olvide fijar este campo a -1 pinta
	// un cursor fantasma en su primera fila. Todo panelSpec que no tenga
	// concepto de "fila seleccionada" TIENE que fijarlo explícitamente.
	Cursor  int
	Summary string // línea única cuando CollapseWhenUnfocused es true y la caja no tiene foco
	Empty   string // texto cuando Rows está vacío
	Focused bool   // tiñe el borde/título; NO decide qué contenido se pinta
	// CollapseWhenUnfocused: true = colapsa a Summary cuando !Focused (el
	// comportamiento que la vista Config ya tiene hoy — viewConfigSection
	// colapsa las secciones no enfocadas). false (el default, el cero de Go) =
	// pinta Rows/Hint SIEMPRE, tenga foco o no — el comportamiento que el
	// dashboard y la vista de perfil ya tienen hoy: el foco solo cambia el
	// color del borde. Sin este campo, un solo renderPanel no puede servir a
	// las tres vistas a la vez sin romper a alguna — es el motivo por el que
	// el retrofit de las Tareas 4/5/8 usa valores distintos de este campo.
	CollapseWhenUnfocused bool
	MaxRows               int // 0 = sin ventana; >0 recorta alrededor del cursor
}

// viewSpec es una vista entera.
type viewSpec struct {
	Header    string // logoBanner(...) o brand + eyebrow
	Panels    []panelSpec
	Status    string
	StatusErr bool
	Extra     string // barra de comandos, confirmaciones: lo que no es panel
	Footer    string
}

// renderView monta la vista completa. Es la única función que decide dónde va
// cada cosa vertical.
func (m *model) renderView(v viewSpec) string {
	var b strings.Builder
	if v.Header != "" {
		b.WriteString(v.Header + "\n\n")
	}
	for _, p := range v.Panels {
		b.WriteString(m.renderPanel(p) + "\n")
	}
	if v.Extra != "" {
		b.WriteString("\n" + v.Extra + "\n")
	}
	if v.Status != "" {
		st := styleOK
		if v.StatusErr {
			st = styleErr
		}
		b.WriteString("\n" + st.Render(v.Status) + "\n")
	}
	if v.Footer != "" {
		b.WriteString("\n" + styleDim.Render(v.Footer))
	}
	return b.String()
}

// renderPanel pinta una caja. Colapsa a Summary SOLO si el panel lo pidió
// (CollapseWhenUnfocused) y no tiene foco; en cualquier otro caso pinta sus
// filas (o el texto de vacío) y su línea de teclas — el foco, aquí, únicamente
// tiñe el borde vía boxFocused.
func (m *model) renderPanel(p panelSpec) string {
	if p.CollapseWhenUnfocused && !p.Focused {
		return m.boxFocused(false, p.Title, "", styleDim.Render(p.Summary))
	}
	if len(p.Rows) == 0 {
		return m.boxFocused(p.Focused, p.Title, p.Hint, styleDim.Render(p.Empty))
	}
	return m.boxFocused(p.Focused, p.Title, p.Hint, strings.Join(m.windowRows(p), "\n"))
}

// rowLine pinta UNA fila: antepone el cursor y, si hay Marker, lo colorea — y
// nada más. NO trunca ni reestiliza r.Text (ver el contrato de rowSpec.Text
// explicado arriba): eso ya lo hizo la vista, con su propio ancho.
func (m *model) rowLine(r rowSpec, sel bool) string {
	cur := "  "
	if sel {
		cur = styleFocused.Render("▸ ")
	}
	if r.Marker == "" {
		return cur + r.Text
	}
	return cur + m.styledMarker(r.Marker) + " " + r.Text
}

// styledMarker colorea la marca: verde si EMPIEZA con ✓, rojo si empieza con
// ✗ (así cubre tanto un mark suelto, como el de Config, como una frase entera
// tipo "✓ logueado", como el health de perfiles); cualquier otro valor se
// pinta sin color. Coloreamos por prefijo y no por igualdad exacta a propósito:
// es lo que permite que Marker cargue la frase completa sin que la vista tenga
// que separar el glifo del texto.
func (m *model) styledMarker(marker string) string {
	switch {
	case strings.HasPrefix(marker, "✓"):
		return styleCheck.Render(marker)
	case strings.HasPrefix(marker, "✗"):
		return styleCross.Render(marker)
	default:
		return marker
	}
}

// windowRows recorta a MaxRows alrededor del cursor y marca lo que queda fuera.
//
// La ventana vive aquí y no en cada vista por dos razones. El permissions.allow
// de un perfil real trae ~200 entradas y ninguna vista puede pintarlas; si cada
// una lo resolviera a su manera tendríamos tres soluciones al mismo problema. Y
// la vista Config ya tiene el mismo agujero latente hoy (un allow_from largo se
// sale de la pantalla), así que ponerlo aquí lo tapa de paso.
func (m *model) windowRows(p panelSpec) []string {
	start, end := 0, len(p.Rows)
	if p.MaxRows > 0 && len(p.Rows) > p.MaxRows {
		start = p.Cursor - p.MaxRows/2
		if start < 0 {
			start = 0
		}
		end = start + p.MaxRows
		if end > len(p.Rows) {
			end = len(p.Rows)
			start = end - p.MaxRows
		}
	}
	out := make([]string, 0, end-start+2)
	if start > 0 {
		out = append(out, styleDim.Render(i18n.T(m.lang, "tui.shell.more_up", start)))
	}
	for i := start; i < end; i++ {
		// p.Focused && ...: con CollapseWhenUnfocused=false un panel sin foco
		// SIGUE pintando todas sus filas (dashboard, vista de perfil), y sin
		// este AND el cursor de CADA panel se marcaría a la vez — varias "▸"
		// simultáneas en pantalla. El cursor solo se pinta en el panel que de
		// verdad tiene el foco, igual que hoy (profileRow: `sel := i ==
		// m.profIdx && m.focus == panelProfiles`).
		out = append(out, m.rowLine(p.Rows[i], p.Focused && i == p.Cursor))
	}
	if end < len(p.Rows) {
		out = append(out, styleDim.Render(i18n.T(m.lang, "tui.shell.more_down", len(p.Rows)-end)))
	}
	return out
}
```

- [ ] **Step 4: Añadir las dos claves de i18n**

En `internal/core/i18n/catalog_tui.go`, junto al resto de claves `tui.`:

```go
	"tui.shell.more_up": {
		En: "↑ %d more",
		Es: "↑ %d más",
	},
	"tui.shell.more_down": {
		En: "↓ %d more",
		Es: "↓ %d más",
	},
```

- [ ] **Step 5: Correr los tests y verificar que pasan**

Run: `go test ./internal/tui/ -run 'TestPanel|TestCursor|TestRenderView|TestRowLine' -v`
Expected: PASS los seis (dos de panel, uno de cursor, uno de renderView, dos de
rowLine)

- [ ] **Step 6: Commit (pedir autorización antes de ejecutarlo)**

```bash
git add internal/tui/shell.go internal/tui/shell_test.go internal/core/i18n/catalog_tui.go
git commit -m "feat(tui): shell de vista compartido"
```

---

### Task 3: La ventana del shell

`MaxRows` ya está en el struct de la tarea 2 y `windowRows` ya la implementa;
esta tarea la **fija con tests de borde**, que es donde este tipo de código se
rompe.

**Files:**
- Modify: `internal/tui/shell_test.go`

**Interfaces:**
- Consumes: `windowRows`, `panelSpec.MaxRows` (Task 2).
- Produces: nada nuevo; deja la ventana demostrada.

- [ ] **Step 1: Escribir los tests de borde**

```go
func filas(n int) []rowSpec {
	out := make([]rowSpec, n)
	for i := range out {
		out[i] = rowSpec{Text: fmt.Sprintf("fila-%02d", i)}
	}
	return out
}

func TestVentanaConCursorArribaNoMarcaMasArriba(t *testing.T) {
	m := shellModel()
	got := strings.Join(m.windowRows(panelSpec{Rows: filas(20), MaxRows: 5, Cursor: 0}), "\n")
	if strings.Contains(got, "más") && strings.Contains(got, "↑") {
		t.Fatalf("con el cursor en la primera fila no hay nada arriba:\n%s", got)
	}
	if !strings.Contains(got, "↓ 15 más") {
		t.Fatalf("faltan las 15 de abajo:\n%s", got)
	}
	if !strings.Contains(got, "fila-00") || strings.Contains(got, "fila-05") {
		t.Fatalf("la ventana tiene que ser fila-00..fila-04:\n%s", got)
	}
}

func TestVentanaConCursorAbajoNoMarcaMasAbajo(t *testing.T) {
	m := shellModel()
	got := strings.Join(m.windowRows(panelSpec{Rows: filas(20), MaxRows: 5, Cursor: 19}), "\n")
	if strings.Contains(got, "↓") {
		t.Fatalf("con el cursor en la última fila no hay nada abajo:\n%s", got)
	}
	if !strings.Contains(got, "↑ 15 más") {
		t.Fatalf("faltan las 15 de arriba:\n%s", got)
	}
	if !strings.Contains(got, "fila-19") {
		t.Fatalf("la última fila tiene que estar dentro:\n%s", got)
	}
}

func TestVentanaEnMedioMarcaLosDosLados(t *testing.T) {
	m := shellModel()
	got := strings.Join(m.windowRows(panelSpec{Rows: filas(20), MaxRows: 5, Cursor: 10}), "\n")
	if !strings.Contains(got, "↑ 8 más") || !strings.Contains(got, "↓ 7 más") {
		t.Fatalf("con el cursor en medio se marcan los dos lados:\n%s", got)
	}
	if !strings.Contains(got, "fila-10") {
		t.Fatalf("la fila del cursor tiene que verse:\n%s", got)
	}
}

func TestListaMasCortaQueLaVentanaNoRecorta(t *testing.T) {
	m := shellModel()
	got := strings.Join(m.windowRows(panelSpec{Rows: filas(3), MaxRows: 10, Cursor: 1}), "\n")
	if strings.Contains(got, "↑") || strings.Contains(got, "↓") {
		t.Fatalf("con menos filas que ventana no se marca nada:\n%s", got)
	}
	for i := 0; i < 3; i++ {
		if !strings.Contains(got, fmt.Sprintf("fila-%02d", i)) {
			t.Fatalf("falta fila-%02d:\n%s", i, got)
		}
	}
}

func TestSinMaxRowsNoHayVentana(t *testing.T) {
	m := shellModel()
	got := strings.Join(m.windowRows(panelSpec{Rows: filas(50), Cursor: 0}), "\n")
	if strings.Contains(got, "↓") {
		t.Fatalf("MaxRows=0 significa sin ventana:\n%s", got)
	}
	if !strings.Contains(got, "fila-49") {
		t.Fatalf("con MaxRows=0 se pintan todas:\n%s", got)
	}
}
```

Añade `"fmt"` al bloque de imports de `shell_test.go`.

- [ ] **Step 2: Correr y ver el resultado**

Run: `go test ./internal/tui/ -run TestVentana -v && go test ./internal/tui/ -run 'TestLista|TestSinMaxRows' -v`
Expected: PASS. Si alguno falla, el aritmético de `windowRows` está mal —
arréglalo ahí, no en el test.

- [ ] **Step 3: Commit (pedir autorización antes de ejecutarlo)**

```bash
git add internal/tui/shell_test.go
git commit -m "test(tui): bordes de la ventana del shell"
```

---

### Task 4: Retrofit del dashboard al shell

**Files:**
- Modify: `internal/tui/dashboard.go` (`viewDashboard`, `viewProfiles`,
  `viewRules`, `viewStatus`)
- Test: `internal/tui/render_golden_test.go` (Task 1, sin cambios)

**Interfaces:**
- Consumes: `renderView`, `panelSpec`, `rowSpec` (**Task 2 — dependencia dura**:
  este archivo llama a `m.renderView`/usa `panelSpec`/`rowSpec`, así que sin
  `internal/tui/shell.go` (Task 2) el paquete no compila); `goldenRender`
  (Task 1).
- Produces: nada público nuevo. Las tres funciones `viewX` pasan a devolver
  `panelSpec` en vez de string: `profilesPanel()`, `rulesPanel()`, `statusPanel()`.

**Decisión de diseño — ningún panel del dashboard colapsa sin foco.** El
dashboard de hoy pinta las TRES cajas completas siempre (foco = solo el color
del borde, nunca oculta filas ni el hint — `viewProfiles`/`viewRules`/
`viewStatus` llaman a `m.box`, que va a `boxFocused(m.focus==p, ...)` sin más).
Por eso las tres `panelSpec` de esta tarea dejan `CollapseWhenUnfocused` en su
valor por defecto (`false`, el cero de Go) — a diferencia de la vista Config
(Tarea 5), que sí colapsa hoy y sigue haciéndolo. Con eso, `Summary` queda sin
uso en el dashboard: no hace falta ninguna clave `tui.*.summary` nueva.

- [ ] **Step 1: Correr los goldens para partir de verde**

Run: `go test ./internal/tui/ -run TestGoldenRender`
Expected: PASS. Si falla aquí, algo se movió desde la tarea 1 y hay que entender
qué antes de refactorizar.

- [ ] **Step 2: Convertir las tres funciones de panel**

`viewProfiles`/`viewRules`/`viewStatus` devuelven hoy la caja ya pintada.
Conviértelas en constructores de `panelSpec`, preservando el estilo de CADA fila
tal cual está hoy — el contrato de la Tarea 2 es que `rowSpec.Text` llega YA
truncado y estilizado por la vista; el shell solo antepone el cursor (que por
eso desaparece de `profileRow`/`viewRules`/`viewStatus`: ahora lo pone
`rowLine` a partir de `panelSpec.Cursor`).

```go
// profilesPanel describe la caja Perfiles. Es profileRow (dashboard.go:588) de
// siempre, solo que construye un panelSpec en vez de un string: cada fila
// sigue truncando y estilizando sus propios segmentos, exactamente en el mismo
// orden (plano -> estilo) que ya tenía.
func (m *model) profilesPanel() panelSpec {
	focused := m.focus == panelProfiles
	p := panelSpec{
		Title:   i18n.T(m.lang, "tui.profiles.title"),
		Hint:    i18n.T(m.lang, "tui.profiles.hint"),
		Empty:   i18n.T(m.lang, "tui.profiles.empty"),
		Focused: focused,
		Cursor:  m.profIdx,
	}
	for i, name := range m.profiles {
		p.Rows = append(p.Rows, rowSpec{Text: m.profileRowText(i == m.profIdx && focused, name)})
	}
	return p
}

// profileRowText es profileRow (dashboard.go:588-604) menos el prefijo de
// cursor: recibe `sel` ya resuelto (índice Y foco del panel, como antes) para
// elegir el color del nombre, y el resto es idéntico.
func (m *model) profileRowText(sel bool, name string) string {
	nameSt := styleVal
	if sel {
		nameSt = styleSelected
	}
	t := m.profileType(name)
	nameSeg := nameSt.Render(padRight(truncRight(name, 20), 21))
	badgeSeg := typeStyle(t).Render(padRight(humanType(m.lang, t), 11))
	health := ""
	switch t {
	case "official":
		if core.HasLogin(m.home, name) {
			health = styleCheck.Render(i18n.T(m.lang, "tui.profiles.health_logged_in"))
		} else {
			health = styleCross.Render(i18n.T(m.lang, "tui.profiles.health_no_login"))
		}
	case "deepseek", "kimi", "glm":
		if _, ok := core.GetKey(m.home, name); ok {
			health = styleCheck.Render(i18n.T(m.lang, "tui.profiles.health_key"))
		} else {
			health = styleCross.Render(i18n.T(m.lang, "tui.profiles.health_no_key"))
		}
	}
	return nameSeg + badgeSeg + health
}

// rulesPanel es viewRules (dashboard.go:607-645) reescrito igual: el cómputo
// de anchos (profW/pathW) es idéntico, solo que ahora construye rowSpec en vez
// de una tira ya unida.
func (m *model) rulesPanel() panelSpec {
	focused := m.focus == panelRules
	p := panelSpec{
		Title:   i18n.T(m.lang, "tui.rules.title"),
		Hint:    i18n.T(m.lang, "tui.rules.hint"),
		Empty:   i18n.T(m.lang, "tui.rules.empty"),
		Focused: focused,
		Cursor:  m.ruleIdx,
	}
	var rules []core.Rule
	if m.cfg != nil {
		rules = m.cfg.Rules
	}
	if len(rules) == 0 {
		return p
	}
	disp := make([]string, len(rules))
	profW, maxPath := 7, 0
	for i, r := range rules {
		disp[i] = tildeHome(r.Path)
		if n := utf8.RuneCountInString(r.Profile); n > profW {
			profW = n
		}
		if n := utf8.RuneCountInString(disp[i]); n > maxPath {
			maxPath = n
		}
	}
	if profW > 18 {
		profW = 18
	}
	availPath := m.innerWidth() - profW - 5
	pathW := maxPath
	if pathW > availPath {
		pathW = availPath
	}
	if pathW < 14 {
		pathW = 14
	}
	for i, r := range rules {
		sel := i == m.ruleIdx && focused
		pathSt := styleVal
		if sel {
			pathSt = styleSelected
		}
		path := padRight(truncLeft(disp[i], pathW), pathW)
		prof := typeStyle(m.profileType(r.Profile)).Render(truncRight(r.Profile, profW))
		p.Rows = append(p.Rows, rowSpec{Text: pathSt.Render(path) + styleDim.Render(" → ") + prof})
	}
	return p
}

// statusPanel es viewStatus (dashboard.go:649-677) reescrito igual: cuatro
// líneas kv fijas, sin concepto de fila seleccionable — de ahí `Cursor: -1`
// (ver el aviso sobre el cero de Go en panelSpec.Cursor, Tarea 2). Sin esto el
// shell marcaría con "▸" la primera fila por accidente.
func (m *model) statusPanel() panelSpec {
	if !m.estComputed {
		m.refreshEstado()
	}
	e := m.est
	const labelW = 25
	valW := m.innerWidth() - labelW
	if valW < 16 {
		valW = 16
	}
	kv := func(k, v string) string {
		return lipgloss.NewStyle().Width(labelW).Foreground(cMute).Render(k) + v
	}
	repo := styleDim.Render(i18n.T(m.lang, "tui.status.not_git"))
	if e.Repo != "" {
		repo = styleVal.Render(truncLeft(tildeHome(e.Repo), valW))
	}
	profLine := styleFocused.Render(truncRight(e.Profile, valW-len(e.ProfileType)-4)) +
		styleDim.Render(" ("+e.ProfileType+")")
	return panelSpec{
		Title:   i18n.T(m.lang, "tui.status.title"),
		Hint:    i18n.T(m.lang, "tui.status.hint"),
		Focused: m.focus == panelStatus,
		Cursor:  -1,
		Rows: []rowSpec{
			{Text: kv(i18n.T(m.lang, "tui.status.active"), styleFocused.Render(truncRight(e.Active, valW)))},
			{Text: kv(i18n.T(m.lang, "tui.status.cwd_rule"), profLine)},
			{Text: kv(i18n.T(m.lang, "tui.status.cwd"), styleVal.Render(truncLeft(tildeHome(e.Cwd), valW)))},
			{Text: kv(i18n.T(m.lang, "tui.status.repo"), repo)},
		},
	}
}
```

Borra `(*model) box` (dashboard.go:391-394) al terminar este paso: era el
único llamador de las tres funciones que acabas de convertir, así que se queda
sin ningún caller — `golangci-lint` (linter `unused`, en el set por defecto de
v2, sin excepciones en `.golangci.yml`) lo marca como error, no como aviso.

- [ ] **Step 3: Reescribir `viewDashboard` sobre el shell**

```go
func (m *model) viewDashboard() string {
	v := viewSpec{
		Header:    logoBanner(m.lang),
		Panels:    []panelSpec{m.profilesPanel(), m.rulesPanel(), m.statusPanel()},
		Status:    m.statusMsg,
		StatusErr: m.statusErr,
		Footer:    i18n.T(m.lang, "tui.footer.keys"),
	}
	// Los dos pueden darse a la vez hoy (':' no apaga showDetail, solo 'tab' lo
	// hace), así que se concatenan en vez de que uno pise al otro.
	var extras []string
	if m.showDetail {
		if detail, err := core.ProfileShow(m.home, m.selectedProfile()); err == nil {
			extras = append(extras, styleDim.Render(strings.TrimRight(indent(detail), "\n")))
		}
	}
	if m.mode == modeCommand {
		extras = append(extras, m.commandBar())
	}
	v.Extra = strings.Join(extras, "\n")
	return m.renderView(v)
}

// commandBar es el bloque de la barra ':' que hoy vive inline en viewDashboard
// (dashboard.go:534-546), movido tal cual — SIN el '\n' inicial ni el final:
// esos los pone renderView alrededor de Extra.
func (m *model) commandBar() string {
	line := styleFocused.Render(": " + m.cmdInput + "▏")
	matches := cmdMatches(m.cmdInput)
	if len(matches) == 0 {
		matches = cmdList
	}
	sug := make([]string, len(matches))
	for i, c := range matches {
		sug[i] = styleSelected.Render(c)
	}
	suggestions := "  " + strings.Join(sug, styleDim.Render(" · ")) +
		styleDim.Render(i18n.T(m.lang, "tui.cmd.hint"))
	return line + "\n" + suggestions
}
```

- [ ] **Step 4: Correr los goldens**

Run: `go test ./internal/tui/ -run TestGoldenRender -v`
Expected: PASS, byte a byte, sin `-update`. El fixture de la Tarea 1 nunca pone
`showDetail` ni `modeCommand` en true, así que el único cambio de salida real
de esta tarea (mover el detalle del perfil y la barra `:` a `Extra`) no llega a
tocar el golden capturado — si el test falla aquí, es una diferencia real y hay
que corregir el código, no regenerar el golden.

- [ ] **Step 5: Correr toda la suite de la TUI y el linter**

Run: `go test ./internal/tui/ -v 2>&1 | tail -20`
Expected: PASS, incluido `TestDashboardRendersBothLangs`.

Run: `go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2 run ./internal/tui/... --timeout 5m`
Expected: `0 issues`. Si sale `func (*model).box is unused (unused)`, te faltó
borrarla en el Step 2.

- [ ] **Step 6: Commit (pedir autorización antes de ejecutarlo)**

```bash
git add internal/tui/dashboard.go
git commit -m "refactor(tui): el dashboard pinta por el shell compartido"
```

---

### Task 5: Retrofit de la vista Config al shell

**Files:**
- Modify: `internal/tui/config_view.go` (`viewConfig`, `viewConfigSection`,
  `configRowLine`)

**Interfaces:**
- Consumes: `renderView`, `panelSpec`, `rowSpec` (**Task 2 — dependencia dura**,
  igual que la Tarea 4: sin `shell.go` este archivo no compila); `goldenRender`
  (Task 1).
- Produces: `configPanel(sec configSection) panelSpec`.

**Decisión de diseño — Config SÍ colapsa sin foco, y sigue haciéndolo.** A
diferencia del dashboard (Tarea 4), la vista Config YA colapsa sus secciones no
enfocadas hoy (`viewConfigSection` hace exactamente eso). Por eso, a diferencia
de las tres `panelSpec` de la Tarea 4, aquí `CollapseWhenUnfocused` va en
`true` explícitamente — sin fijarlo, una sección sin foco mostraría el texto de
`Empty` en vez de su `Summary`, porque `CollapseWhenUnfocused` por defecto es
`false` (el comportamiento del dashboard, no el de Config).

- [ ] **Step 1: Convertir `viewConfigSection` en `configPanel`**

```go
// configPanel describe UNA sección como caja. Sin foco solo lleva su resumen
// (CollapseWhenUnfocused: true, a diferencia del dashboard); con foco, sus
// filas y su línea de teclas — cada fila sigue truncando y estilizando sus
// propios segmentos, igual que configRowLine hacía hasta ahora.
func (m *model) configPanel(sec configSection) panelSpec {
	focused := m.cfgSec == sec
	p := panelSpec{
		Title:                 i18n.T(m.lang, configTitleKey(sec)),
		Summary:               m.configSummary(sec),
		Empty:                 i18n.T(m.lang, "tui.config.empty"),
		Focused:               focused,
		Cursor:                -1,
		MaxRows:               12,
		CollapseWhenUnfocused: true,
	}
	if !focused {
		return p
	}
	p.Hint = i18n.T(m.lang, configHintKey(sec))
	p.Cursor = m.cfgRow
	for i, r := range m.configRowsFor(sec) {
		p.Rows = append(p.Rows, rowSpec{Text: m.configRowText(i == m.cfgRow, r)})
	}
	return p
}

// configRowText es configRowLine (config_view.go:943-960) menos el prefijo de
// cursor: recibe `sel` ya resuelto para elegir el color del label, y el resto
// —alineación a labelW=16, la marca ✓/✗ coloreada, el truncado del valor— es
// idéntico, en el mismo orden (plano -> estilo) que ya tenía.
func (m *model) configRowText(sel bool, r configRow) string {
	const labelW = 16
	st := styleVal
	if sel {
		st = styleSelected
	}
	mark := ""
	if r.toggle {
		mark = styleCross.Render("✗ ")
		if r.on {
			mark = styleCheck.Render("✓ ")
		}
	}
	valW := m.innerWidth() - labelW - 6
	if valW < 12 {
		valW = 12
	}
	return st.Render(padRight(truncRight(r.label, labelW), labelW+1)) +
		mark + styleVal.Render(truncRight(r.value, valW))
}
```

Borra `viewConfigSection` y `configRowLine` (config_view.go:919-961) al
terminar: `configPanel`/`configRowText` las sustituyen del todo, y dejarlas
vivas hace fallar el linter `unused`.

La nota de sección (`m.configNote(sec)`) deja de ir dentro del body: pásala como
primera fila con `rowSpec{Text: nota}` sólo si `configNote` devuelve algo, y
recuerda que entonces `Cursor` tiene que ser `m.cfgRow + 1`. Si eso te parece
frágil —lo es—, la alternativa correcta es dejar la nota fuera del panel, en
`viewSpec.Extra`. **Elige `Extra`**: una fila que desplaza el cursor es
exactamente la clase de detalle que rompe la navegación seis meses después.

- [ ] **Step 2: Reescribir `viewConfig` sobre el shell**

```go
func (m *model) viewConfig() string {
	v := viewSpec{
		Header:    styleBrand.Render("ccp") + styleSub.Render("  "+i18n.T(m.lang, "tui.config.eyebrow")),
		Status:    m.statusMsg,
		StatusErr: m.statusErr,
		Footer:    i18n.T(m.lang, "tui.config.footer"),
	}
	for _, sec := range configSections {
		v.Panels = append(v.Panels, m.configPanel(sec))
	}
	if note := m.configNote(m.cfgSec); note != "" {
		v.Extra = styleDim.Render(note)
	}
	return m.renderView(v)
}
```

- [ ] **Step 3: Regenerar los goldens de Config a propósito**

La nota de sección se mueve de dentro de la caja a debajo de los paneles. Es un
cambio de salida **deliberado**, así que aquí sí toca `-update` — pero antes hay
que mirar el diff con ojos humanos.

Run: `go test ./internal/tui/ -run TestGoldenRenderConfig -v 2>&1 | head -60`
Expected: FAIL enseñando el diff. Léelo: lo único que debe haberse movido es la
nota. Si ves cajas, filas o teclas distintas, es un bug del retrofit.

Run: `go test ./internal/tui/ -run TestGoldenRenderConfig -update`
Run: `git diff internal/tui/testdata/render/`
Expected: el diff sólo mueve la línea de la nota.

- [ ] **Step 4: Correr la suite entera de la TUI y el linter**

Run: `go test ./internal/tui/ 2>&1 | tail -10`
Expected: PASS, incluidos los tests que ya existían en `config_view_test.go`.

Run: `go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2 run ./internal/tui/... --timeout 5m`
Expected: `0 issues`. Si sale `viewConfigSection is unused` o `configRowLine is
unused`, te faltó borrarlas en el Step 1.

- [ ] **Step 5: Commit (pedir autorización antes de ejecutarlo)**

```bash
git add internal/tui/config_view.go internal/tui/testdata/render
git commit -m "refactor(tui): la vista Config pinta por el shell compartido"
```

---

### Task 6: `core.ProfileEffective`

**Files:**
- Create: `internal/core/effective.go`
- Create: `internal/core/effective_test.go`

**Interfaces:**
- Consumes: `Load` (`store.go`), `cfgInstrFile`/`cfgSettingsFile`
  (`cfg.go:29`/`:33`), `CfgInitOverlay` (`cfg.go:134`), `InstructRuleList`
  (`instruct.go:158`), `AutoHooksEnabled` (`autohooks.go:172`),
  `applyAutoLayer` (`autohooks.go:290`), `MergeJSON` (`cfg.go:47`).
- Produces: `Origin` (`OriginGlobal`/`OriginOverlay`/`OriginAuto`, con `String()`),
  `EffKind` (`EffInstructions`/`EffEnv`/`EffPermissions`/`EffHooks`/`EffPlugins`/`EffSensors`),
  `EffRow{Key,Value string; Origin Origin; Shadowed bool}`,
  `EffSection{Kind EffKind; Rows []EffRow; File string; Err error}`,
  `Effective{Profile string; Sections []EffSection}` y
  `ProfileEffective(home, name, src string) (Effective, error)`.
  La tarea 8 consume exactamente esos nombres.

- [ ] **Step 1: Escribir el test de procedencia**

```go
package core

import (
	"os"
	"path/filepath"
	"testing"
)

// seedEff arma un home con un perfil y contenidos controlados en las dos capas.
func seedEff(t *testing.T, global, overlay string) (home, src, name string) {
	t.Helper()
	src = t.TempDir()
	home = t.TempDir()
	t.Setenv("CCP_CLAUDE_SRC", src)
	name = "work"
	if err := ProfileAddOfficial(home, name); err != nil {
		t.Fatalf("ProfileAddOfficial: %v", err)
	}
	// ProfileAddOfficial solo crea profiles/<name>/cc-home/ (seedCCHome); el
	// directorio overlay/ lo crea CfgInitOverlay, no ProfileAddOfficial. Sin
	// esto, el os.WriteFile de abajo falla con ENOENT.
	if err := CfgInitOverlay(home, name); err != nil {
		t.Fatalf("CfgInitOverlay: %v", err)
	}
	if global != "" {
		if err := os.WriteFile(filepath.Join(src, "settings.json"), []byte(global), 0o644); err != nil {
			t.Fatalf("global: %v", err)
		}
	}
	if overlay != "" {
		if err := os.WriteFile(cfgSettingsFile(home, name), []byte(overlay), 0o600); err != nil {
			t.Fatalf("overlay: %v", err)
		}
	}
	return home, src, name
}

func sectionOf(t *testing.T, e Effective, k EffKind) EffSection {
	t.Helper()
	for _, s := range e.Sections {
		if s.Kind == k {
			return s
		}
	}
	t.Fatalf("falta la sección %v", k)
	return EffSection{}
}

func TestEffectiveEnvMarcaLaCapaQueGana(t *testing.T) {
	home, src, name := seedEff(t,
		`{"env":{"SOLO_GLOBAL":"g","AMBAS":"g"}}`,
		`{"env":{"SOLO_OVERLAY":"o","AMBAS":"o"}}`)
	e, err := ProfileEffective(home, name, src)
	if err != nil {
		t.Fatalf("ProfileEffective: %v", err)
	}
	got := map[string]EffRow{}
	for _, r := range sectionOf(t, e, EffEnv).Rows {
		got[r.Key] = r
	}
	if got["SOLO_GLOBAL"].Origin != OriginGlobal || got["SOLO_GLOBAL"].Shadowed {
		t.Errorf("SOLO_GLOBAL mal: %+v", got["SOLO_GLOBAL"])
	}
	if got["SOLO_OVERLAY"].Origin != OriginOverlay || got["SOLO_OVERLAY"].Shadowed {
		t.Errorf("SOLO_OVERLAY mal: %+v", got["SOLO_OVERLAY"])
	}
	if r := got["AMBAS"]; r.Origin != OriginOverlay || !r.Shadowed || r.Value != "o" {
		t.Errorf("AMBAS tiene que ganar el overlay y quedar Shadowed: %+v", r)
	}
}

func TestEffectivePermisosElArrayLoReemplazaElOverlay(t *testing.T) {
	// MergeJSON REEMPLAZA arrays; no los concatena. La procedencia tiene que
	// contar esa verdad y no una unión que no ocurre.
	home, src, name := seedEff(t,
		`{"permissions":{"allow":["Bash(ls)","Bash(cat)"]}}`,
		`{"permissions":{"allow":["Bash(rg)"]}}`)
	e, err := ProfileEffective(home, name, src)
	if err != nil {
		t.Fatalf("ProfileEffective: %v", err)
	}
	rows := sectionOf(t, e, EffPermissions).Rows
	if len(rows) != 1 || rows[0].Key != "Bash(rg)" {
		t.Fatalf("el array del overlay reemplaza al global: %+v", rows)
	}
	if rows[0].Origin != OriginOverlay || !rows[0].Shadowed {
		t.Fatalf("la fila tiene que decir que el global perdió: %+v", rows[0])
	}
}

func TestEffectiveOverlayRotoEsErrorDeSeccionNoDeTodo(t *testing.T) {
	home, src, name := seedEff(t, `{"env":{"G":"1"}}`, `{roto`)
	e, err := ProfileEffective(home, name, src)
	if err != nil {
		t.Fatalf("un overlay roto no puede tumbar ProfileEffective: %v", err)
	}
	if sectionOf(t, e, EffEnv).Err == nil {
		t.Error("la sección Env tiene que llevar el error del overlay")
	}
	if sectionOf(t, e, EffInstructions).Err != nil {
		t.Error("Instrucciones vive en otro archivo: no puede heredar ese error")
	}
}

func TestEffectiveDefaultNoTieneOverlay(t *testing.T) {
	home, src, _ := seedEff(t, `{"env":{"G":"1"}}`, "")
	e, err := ProfileEffective(home, "default", src)
	if err != nil {
		t.Fatalf("'default' tiene que abrirse, no fallar: %v", err)
	}
	env := sectionOf(t, e, EffEnv)
	if env.File != "" {
		t.Errorf("'default' no tiene archivo de overlay: %q", env.File)
	}
	if len(env.Rows) != 1 || env.Rows[0].Origin != OriginGlobal {
		t.Errorf("todo lo de 'default' viene del global: %+v", env.Rows)
	}
}

func TestEffectiveSinGlobalNoFalla(t *testing.T) {
	home, src, name := seedEff(t, "", `{"env":{"O":"1"}}`)
	e, err := ProfileEffective(home, name, src)
	if err != nil {
		t.Fatalf("sin settings.json global no se falla: %v", err)
	}
	rows := sectionOf(t, e, EffEnv).Rows
	if len(rows) != 1 || rows[0].Origin != OriginOverlay {
		t.Errorf("solo overlay: %+v", rows)
	}
}

// TestEffectiveHooksYSensoresConAutoHandoffInstalado fija el caso que el
// mockup del spec usa como ejemplo canónico ("Hooks  11 eventos  overlay ⊕
// auto"): con la capa de sensores instalada, StopFailure tiene que aparecer
// con Origin auto (lo instala AutoHooksFragment vía applyAutoLayer), y la
// sección Sensores tiene que marcar el perfil.
func TestEffectiveHooksYSensoresConAutoHandoffInstalado(t *testing.T) {
	home, src, name := seedEff(t, "", "")
	cfg, err := Load(home)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	cfg.AutoHandoff = &AutoHandoff{Hooks: []string{name}}
	if err := Save(home, cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}
	SetAutoHooksBin("ccp") // determinista: no depende de os.Executable()

	e, err := ProfileEffective(home, name, src)
	if err != nil {
		t.Fatalf("ProfileEffective: %v", err)
	}
	var stopFailure *EffRow
	for i, r := range sectionOf(t, e, EffHooks).Rows {
		if r.Key == "StopFailure" {
			stopFailure = &sectionOf(t, e, EffHooks).Rows[i]
		}
	}
	if stopFailure == nil || stopFailure.Origin != OriginAuto {
		t.Fatalf("StopFailure tiene que venir de la capa auto: %+v", sectionOf(t, e, EffHooks).Rows)
	}
	sensors := sectionOf(t, e, EffSensors).Rows
	if len(sensors) != 1 || sensors[0].Origin != OriginAuto {
		t.Fatalf("Sensores tiene que marcar el perfil instalado: %+v", sensors)
	}
}

// TestEffectiveHooksConservaElStopFailureAjenoDelOverlay fija el motivo real de
// HUECO 2: applyAutoLayer no reemplaza el StopFailure del overlay, lo CONSERVA
// detrás del suyo (keepForeignStopFailure, autohooks.go:319) porque MergeJSON
// reemplaza arrays. Si effHookRows solo mirara el fragmento crudo de
// AutoHooksFragment (en vez del resultado REAL de applyAutoLayer), contaría 1
// en vez de 2 y perdería el StopFailure del usuario en el recuento.
func TestEffectiveHooksConservaElStopFailureAjenoDelOverlay(t *testing.T) {
	overlay := `{"hooks":{"StopFailure":[{"matcher":"","hooks":[{"type":"command","command":"echo propio"}]}]}}`
	home, src, name := seedEff(t, "", overlay)
	cfg, err := Load(home)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	cfg.AutoHandoff = &AutoHandoff{Hooks: []string{name}}
	if err := Save(home, cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}
	SetAutoHooksBin("ccp")

	e, err := ProfileEffective(home, name, src)
	if err != nil {
		t.Fatalf("ProfileEffective: %v", err)
	}
	for _, r := range sectionOf(t, e, EffHooks).Rows {
		if r.Key == "StopFailure" {
			if r.Value != "2" {
				t.Fatalf("StopFailure tiene que contar el propio + el que instala la capa (2): value=%s", r.Value)
			}
			return
		}
	}
	t.Fatal("falta la fila StopFailure")
}
```

- [ ] **Step 2: Correr y verificar que falla**

Run: `go test ./internal/core/ -run TestEffective -v`
Expected: FAIL con `undefined: ProfileEffective`, `undefined: EffEnv`, etc.

- [ ] **Step 3: Escribir `effective.go`**

```go
package core

// effective.go — la procedencia como dato: qué configuración aplica un perfil y
// de qué capa sale cada cosa.
//
// Es barato porque el motor ya está partido en esas capas y este archivo solo
// las recorre otra vez, en el mismo orden que cfgMergeSettings:
// global ⊕ overlay ⊕ auto. Las instrucciones no se mergean (cfgWriteClaudeMD
// escribe dos @import), así que su procedencia es por archivo.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

type Origin int

const (
	OriginGlobal Origin = iota
	OriginOverlay
	OriginAuto
)

func (o Origin) String() string {
	switch o {
	case OriginGlobal:
		return "global"
	case OriginOverlay:
		return "overlay"
	case OriginAuto:
		return "auto"
	}
	return "?"
}

type EffKind int

const (
	EffInstructions EffKind = iota
	EffEnv
	EffPermissions
	EffHooks
	EffPlugins
	EffSensors
)

// EffRow es una fila con su capa ganadora. Shadowed es un booleano y no la
// cadena de valores perdidos: saber que el overlay pisó al global es accionable,
// saber qué traía el global es curiosidad.
type EffRow struct {
	Key      string
	Value    string
	Origin   Origin
	Shadowed bool
}

// EffSection lleva su propio Err: un settings.overlay.json roto no puede impedir
// que se vean las instrucciones, que están en otro archivo.
type EffSection struct {
	Kind EffKind
	Rows []EffRow
	File string // archivo editable de esta sección; "" si no hay
	Err  error
}

type Effective struct {
	Profile  string
	Sections []EffSection
}

// effLayer es una capa ya decodificada, en orden de precedencia creciente.
type effLayer struct {
	origin Origin
	doc    map[string]any
}

// effLayers son las capas genéricas (global, overlay — la auto NO entra aquí,
// ver effHookRows) más sus bytes crudos, que hacen falta aparte para
// reproducir el merge real de la capa auto vía applyAutoLayer.
type effLayers struct {
	docs         []effLayer
	globalBytes  []byte
	overlayBytes []byte
	overlayErr   error
}

func ProfileEffective(home, name, src string) (Effective, error) {
	if name == "" {
		return Effective{}, fmt.Errorf("ProfileEffective necesita un perfil")
	}
	cfg, err := Load(home)
	if err != nil {
		return Effective{}, err
	}
	if name != "default" {
		if _, ok := cfg.Profiles[name]; !ok {
			return Effective{}, fmt.Errorf("no existe el perfil %q", name)
		}
	}

	L := effReadLayers(home, name, src)
	settingsFile := ""
	if name != "default" {
		settingsFile = cfgSettingsFile(home, name)
	}

	e := Effective{Profile: name}
	e.Sections = append(e.Sections,
		effInstructionsSection(home, name, src),
		EffSection{Kind: EffEnv, File: settingsFile, Err: L.overlayErr,
			Rows: effMapRows(L.docs, "env")},
		EffSection{Kind: EffPermissions, File: settingsFile, Err: L.overlayErr,
			Rows: effArrayRows(L.docs, "permissions", "allow")},
		EffSection{Kind: EffHooks, File: settingsFile, Err: L.overlayErr,
			Rows: effHookRows(home, name, cfg, L)},
		EffSection{Kind: EffPlugins, File: settingsFile, Err: L.overlayErr,
			Rows: effMapRows(L.docs, "enabledPlugins")},
		effSensorsSection(cfg, name),
	)
	return e, nil
}

// effReadLayers decodifica global y overlay. Un global ausente o inválido se
// ignora (igual que hace cfgMergeSettings); un overlay inválido se reporta,
// porque ahí el usuario sí tiene algo que arreglar. La capa auto NO se
// construye aquí como una tercera capa genérica — solo toca `hooks.StopFailure`
// y `statusLine`, y tratarla como una capa más rompería el origen de env/
// permissions/plugins con datos que esa capa ni siquiera define (ver
// effHookRows para cómo se aplica de verdad).
func effReadLayers(home, name, src string) effLayers {
	var out effLayers
	if g, err := os.ReadFile(filepath.Join(src, "settings.json")); err == nil {
		out.globalBytes = g
		if doc, derr := effDecode(g); derr == nil {
			out.docs = append(out.docs, effLayer{origin: OriginGlobal, doc: doc})
		}
	}
	if name == "default" {
		return out
	}
	if o, err := os.ReadFile(cfgSettingsFile(home, name)); err == nil {
		out.overlayBytes = o
		doc, derr := effDecode(o)
		if derr != nil {
			out.overlayErr = fmt.Errorf("el overlay de %q no es JSON válido: %w", name, derr)
		} else {
			out.docs = append(out.docs, effLayer{origin: OriginOverlay, doc: doc})
		}
	}
	return out
}

func effDecode(data []byte) (map[string]any, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return map[string]any{}, nil
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var doc map[string]any
	if err := dec.Decode(&doc); err != nil {
		return nil, err
	}
	if doc == nil {
		doc = map[string]any{}
	}
	return doc, nil
}

// effAt baja por un camino de claves de objeto sin distinguir "ausente" de
// "presente con otro tipo" — sirve para lecturas de un solo documento ya
// resuelto (como el de effHookRows tras applyAutoLayer). effMapRows/
// effArrayRows usan effAtPresent en su lugar, porque a ELLAS sí les importa
// esa distinción (ver el comentario ahí).
func effAt(doc map[string]any, path ...string) any {
	var cur any = doc
	for _, k := range path {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur, ok = m[k]
		if !ok {
			return nil
		}
	}
	return cur
}

// effAtPresent es como effAt pero dice si la ÚLTIMA clave del camino estaba
// PRESENTE en su capa — necesario para no confundir "esta capa no toca la
// clave" (se ignora, no afecta lo acumulado) con "esta capa la define con un
// valor de otro tipo" (gana la subrama entera, igual que en el merge real).
func effAtPresent(doc map[string]any, path ...string) (any, bool) {
	cur := any(doc)
	for i, k := range path {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		v, present := m[k]
		if i == len(path)-1 {
			return v, present
		}
		if !present {
			return nil, false
		}
		cur = v
	}
	return cur, true
}

// effMapRows recorre una clave de tipo objeto (env, enabledPlugins): el merge
// es por clave, así que gana la última capa que la define y las anteriores
// quedan Shadowed. Se ordena por clave para que el render sea determinista.
//
// Si una capa POSTERIOR define la clave contenedora con un tipo incompatible
// (p. ej. un `null` explícito en vez de un objeto), esa capa gana la subrama
// ENTERA y lo acumulado se descarta — igual que hace el merge real
// (mergeValues, cfg.go:95-101: "gana el overlay, incluido un null explícito").
// Un `continue` liso y llano (ignorar esa capa) estaría MINTIENDO sobre cuál
// capa manda.
func effMapRows(layers []effLayer, path ...string) []EffRow {
	win := map[string]EffRow{}
	for _, l := range layers {
		v, present := effAtPresent(l.doc, path...)
		if !present {
			continue
		}
		m, ok := v.(map[string]any)
		if !ok {
			win = map[string]EffRow{}
			continue
		}
		for k, val := range m {
			prev, seen := win[k]
			win[k] = EffRow{
				Key:      k,
				Value:    effScalar(val),
				Origin:   l.origin,
				Shadowed: seen || prev.Shadowed,
			}
		}
	}
	keys := make([]string, 0, len(win))
	for k := range win {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]EffRow, 0, len(keys))
	for _, k := range keys {
		out = append(out, win[k])
	}
	return out
}

// effArrayRows recorre una clave de tipo array (permissions.allow). MergeJSON
// REEMPLAZA arrays, así que gana la última capa que lo define entera y las
// anteriores quedan Shadowed; el orden es el del documento, no alfabético.
// Mismo razonamiento de reset-no-skip que effMapRows para un tipo incompatible.
func effArrayRows(layers []effLayer, path ...string) []EffRow {
	var winner []any
	var origin Origin
	var have, shadowed bool
	for _, l := range layers {
		v, present := effAtPresent(l.doc, path...)
		if !present {
			continue
		}
		arr, ok := v.([]any)
		if !ok {
			winner, have, shadowed = nil, false, false
			continue
		}
		if have {
			shadowed = true
		}
		winner, origin, have = arr, l.origin, true
	}
	out := make([]EffRow, 0, len(winner))
	for _, v := range winner {
		out = append(out, EffRow{Key: effScalar(v), Origin: origin, Shadowed: shadowed})
	}
	return out
}

// effMapRowsCount es effMapRows para una clave cuyos valores son arrays
// (hooks): en vez del valor crudo, cuenta cuántas entradas trae cada evento —
// los hooks son arrays sin id estable, así que una fila por entrada no sería
// accionable. Mismo reset-no-skip en tipo incompatible.
func effMapRowsCount(layers []effLayer, path ...string) []EffRow {
	win := map[string]EffRow{}
	for _, l := range layers {
		v, present := effAtPresent(l.doc, path...)
		if !present {
			continue
		}
		m, ok := v.(map[string]any)
		if !ok {
			win = map[string]EffRow{}
			continue
		}
		for k, val := range m {
			n := 0
			if arr, ok := val.([]any); ok {
				n = len(arr)
			}
			_, seen := win[k]
			win[k] = EffRow{Key: k, Value: fmt.Sprintf("%d", n), Origin: l.origin, Shadowed: seen}
		}
	}
	keys := make([]string, 0, len(win))
	for k := range win {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]EffRow, 0, len(keys))
	for _, k := range keys {
		out = append(out, win[k])
	}
	return out
}

// effHookRows: el origen genérico por evento sale de las dos capas de
// siempre (global, overlay). Si la capa auto está instalada, el conteo de
// StopFailure se REEMPLAZA por el que de verdad termina en cc-home/settings.json
// — el resultado de applyAutoLayer, no el fragmento crudo de AutoHooksFragment
// — porque applyAutoLayer CONSERVA detrás cualquier StopFailure ajeno
// (keepForeignStopFailure, autohooks.go:319: hace falta porque MergeJSON
// reemplaza arrays; sin conservarlo, instalar la capa borraría en silencio del
// settings.json generado cualquier StopFailure que el usuario ya tuviera).
// Mirar solo el fragmento crudo, como hacía un borrador anterior de esta
// función, contaría 1 en vez de "propio + ajenos" y mentiría sobre cuántos
// hooks de ese evento aplican de verdad.
func effHookRows(home, name string, cfg *Config, L effLayers) []EffRow {
	rows := effMapRowsCount(L.docs, "hooks")
	if name == "default" || !AutoHooksEnabled(cfg, name) {
		return rows
	}
	preAuto, err := MergeJSON(L.globalBytes, L.overlayBytes)
	if err != nil {
		return rows // overlay roto: ya se reporta aparte vía EffSection.Err
	}
	finalDoc, err := effDecode(applyAutoLayer(home, name, preAuto))
	if err != nil {
		return rows
	}
	finalHooks, _ := effAt(finalDoc, "hooks").(map[string]any)
	arr, _ := finalHooks[autoStopFailureEvent].([]any)
	count := len(arr)

	shadowed := false
	patched := make([]EffRow, 0, len(rows)+1)
	for _, r := range rows {
		if r.Key == autoStopFailureEvent {
			shadowed = true
			continue // se reinserta abajo con el conteo real
		}
		patched = append(patched, r)
	}
	patched = append(patched, EffRow{
		Key: autoStopFailureEvent, Value: fmt.Sprintf("%d", count),
		Origin: OriginAuto, Shadowed: shadowed,
	})
	sort.Slice(patched, func(i, j int) bool { return patched[i].Key < patched[j].Key })
	return patched
}

func effScalar(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case nil:
		return "null"
	default:
		b, err := json.Marshal(t)
		if err != nil {
			return fmt.Sprintf("%v", t)
		}
		return string(b)
	}
}

// effInstructionsSection: las instrucciones no se mergean, se importan. La
// procedencia es por archivo: el global entero, y luego cada regla del bloque
// gestionado del overlay.
func effInstructionsSection(home, name, src string) EffSection {
	s := EffSection{Kind: EffInstructions}
	globalMD := filepath.Join(src, "CLAUDE.md")
	if fi, err := os.Stat(globalMD); err == nil && !fi.IsDir() {
		s.Rows = append(s.Rows, EffRow{
			Key:    globalMD,
			Value:  fmt.Sprintf("%d B", fi.Size()),
			Origin: OriginGlobal,
		})
	}
	if name == "default" {
		return s
	}
	s.File = cfgInstrFile(home, name)
	rules, err := InstructRuleList(s.File)
	if err != nil {
		s.Err = err
		return s
	}
	for _, r := range rules {
		s.Rows = append(s.Rows, EffRow{Key: r, Origin: OriginOverlay})
	}
	return s
}

// effSensorsSection no tiene File: la fuente de verdad de los sensores es
// auto_handoff.hooks en ccp.yaml, y se cambia desde la vista Config con 'c'.
func effSensorsSection(cfg *Config, name string) EffSection {
	s := EffSection{Kind: EffSensors}
	if name != "default" && AutoHooksEnabled(cfg, name) {
		s.Rows = append(s.Rows, EffRow{Key: name, Value: "on", Origin: OriginAuto})
	}
	return s
}
```

- [ ] **Step 4: Correr los tests**

Run: `go test ./internal/core/ -run TestEffective -v`
Expected: PASS los siete

- [ ] **Step 5: Verificar que no hay no-determinismo**

Run: `go test ./internal/core/ -run TestEffective -count=5`
Expected: PASS. Si falla intermitente es que falta un `sort` en algún recorrido
de mapa.

- [ ] **Step 6: Commit (pedir autorización antes de ejecutarlo)**

```bash
git add internal/core/effective.go internal/core/effective_test.go
git commit -m "feat(core): ProfileEffective, la procedencia de la config como dato"
```

---

### Task 7: `core.OverlayEnvSet` / `OverlayEnvDel`

**Files:**
- Create: `internal/core/overlay_env.go`
- Create: `internal/core/overlay_env_test.go`

**Interfaces:**
- Consumes: `CfgInitOverlay` (`cfg.go:134`), `cfgSettingsFile` (`cfg.go:33`),
  `CfgValidateBytes` (`cfg.go:167`), `CfgRegenerate` (`cfg.go:247`),
  `claudeSrc` (`cfg_cmd.go:17`), `marshalIndent` (`cfg.go`).
- Produces: `OverlayEnvSet(home, name, key, val string) error` y
  `OverlayEnvDel(home, name, key string) error`. La tarea 9 los llama.

- [ ] **Step 1: Escribir el test**

```go
package core

import (
	"os"
	"strings"
	"testing"
)

func seedEnvHome(t *testing.T) (home, name string) {
	t.Helper()
	t.Setenv("CCP_CLAUDE_SRC", t.TempDir())
	home = t.TempDir()
	name = "work"
	if err := ProfileAddOfficial(home, name); err != nil {
		t.Fatalf("ProfileAddOfficial: %v", err)
	}
	// Igual que en seedEff (Tarea 6): ProfileAddOfficial NO crea overlay/, solo
	// CfgInitOverlay lo hace. TestOverlayEnvNoDestruyeUnOverlayRoto escribe el
	// overlay a mano ANTES de llamar a OverlayEnvSet (que internamente sí llama
	// a CfgInitOverlay) — sin esto, ese os.WriteFile falla con ENOENT.
	if err := CfgInitOverlay(home, name); err != nil {
		t.Fatalf("CfgInitOverlay: %v", err)
	}
	return home, name
}

func TestOverlayEnvSetAltaYSobrescritura(t *testing.T) {
	home, name := seedEnvHome(t)
	if err := OverlayEnvSet(home, name, "FOO", "1"); err != nil {
		t.Fatalf("set: %v", err)
	}
	if err := OverlayEnvSet(home, name, "FOO", "2"); err != nil {
		t.Fatalf("re-set: %v", err)
	}
	data, _ := os.ReadFile(cfgSettingsFile(home, name))
	if !strings.Contains(string(data), `"FOO": "2"`) {
		t.Fatalf("el overlay no quedó con el valor nuevo: %s", data)
	}
	// Y el cc-home se regeneró con él.
	merged, _ := os.ReadFile(ccHomePath(home, name) + "/settings.json")
	if !strings.Contains(string(merged), `"FOO": "2"`) {
		t.Fatalf("el cc-home no refleja el overlay: %s", merged)
	}
}

func TestOverlayEnvDelBorraYLimpiaElObjetoVacio(t *testing.T) {
	home, name := seedEnvHome(t)
	if err := OverlayEnvSet(home, name, "FOO", "1"); err != nil {
		t.Fatalf("set: %v", err)
	}
	if err := OverlayEnvDel(home, name, "FOO"); err != nil {
		t.Fatalf("del: %v", err)
	}
	data, _ := os.ReadFile(cfgSettingsFile(home, name))
	if strings.Contains(string(data), "FOO") {
		t.Fatalf("FOO tenía que desaparecer: %s", data)
	}
	if strings.Contains(string(data), `"env"`) {
		t.Fatalf("un env vacío no se deja escrito: %s", data)
	}
}

func TestOverlayEnvDelDeClaveInexistenteNoFalla(t *testing.T) {
	home, name := seedEnvHome(t)
	if err := OverlayEnvDel(home, name, "NO_EXISTE"); err != nil {
		t.Fatalf("borrar lo que no está es un no-op, no un error: %v", err)
	}
}

func TestOverlayEnvNoDestruyeUnOverlayRoto(t *testing.T) {
	home, name := seedEnvHome(t)
	roto := []byte(`{esto no es json`)
	if err := os.WriteFile(cfgSettingsFile(home, name), roto, 0o600); err != nil {
		t.Fatal(err)
	}
	err := OverlayEnvSet(home, name, "FOO", "1")
	if err == nil {
		t.Fatal("con el overlay roto hay que fallar, no pisarlo")
	}
	after, _ := os.ReadFile(cfgSettingsFile(home, name))
	if string(after) != string(roto) {
		t.Fatalf("el overlay del usuario fue modificado: %s", after)
	}
}

func TestOverlayEnvRechazaDefault(t *testing.T) {
	home, _ := seedEnvHome(t)
	if err := OverlayEnvSet(home, "default", "FOO", "1"); err == nil {
		t.Fatal("'default' no tiene overlay")
	}
}
```

- [ ] **Step 2: Correr y verificar que falla**

Run: `go test ./internal/core/ -run TestOverlayEnv -v`
Expected: FAIL con `undefined: OverlayEnvSet`

- [ ] **Step 3: Escribir `overlay_env.go`**

```go
package core

// overlay_env.go — escritura de variables de entorno en el overlay de un perfil.
// Es la única API de escritura nueva de la vista de perfil: las reglas usan
// InstructRuleAdd/Rm y los hooks InstructAdd, que ya existen.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
)

// OverlayEnvSet fija env.<key> = val en el overlay del perfil y regenera su
// cc-home.
func OverlayEnvSet(home, name, key, val string) error {
	if key == "" {
		return fmt.Errorf("la variable no puede estar vacía")
	}
	return overlayEnvMutate(home, name, func(env map[string]any) {
		env[key] = val
	})
}

// OverlayEnvDel quita env.<key>. Borrar una clave que no está es un no-op.
func OverlayEnvDel(home, name, key string) error {
	if key == "" {
		return fmt.Errorf("la variable no puede estar vacía")
	}
	return overlayEnvMutate(home, name, func(env map[string]any) {
		delete(env, key)
	})
}

// overlayEnvMutate es el camino común: lee, muta, VALIDA, escribe y regenera.
// El orden importa — mismo invariante que ProfileConfig: si el resultado no
// valida, el último overlay bueno no se toca.
func overlayEnvMutate(home, name string, fn func(map[string]any)) error {
	if name == "default" {
		return fmt.Errorf("'default' = tu config GLOBAL; edítala directamente, no tiene overlay")
	}
	if err := CfgInitOverlay(home, name); err != nil {
		return err
	}
	file := cfgSettingsFile(home, name)
	data, err := os.ReadFile(file)
	if err != nil {
		return fmt.Errorf("no se pudo leer el overlay de %q: %w", name, err)
	}

	doc := map[string]any{}
	if len(bytes.TrimSpace(data)) > 0 {
		dec := json.NewDecoder(bytes.NewReader(data))
		dec.UseNumber()
		if err := dec.Decode(&doc); err != nil {
			return fmt.Errorf("el overlay de %q no es JSON válido: %w — arréglalo con: ccp profile config %s", name, err, name)
		}
	}

	env, _ := doc["env"].(map[string]any)
	if env == nil {
		env = map[string]any{}
	}
	fn(env)
	if len(env) == 0 {
		delete(doc, "env")
	} else {
		doc["env"] = env
	}

	out, err := marshalIndent(doc)
	if err != nil {
		return fmt.Errorf("no se pudo serializar el overlay de %q: %w", name, err)
	}
	if err := CfgValidateBytes(out); err != nil {
		return fmt.Errorf("el overlay resultante no valida: %w", err)
	}
	// 0o644, no 0o600: el overlay no guarda secretos (la api_key vive aparte,
	// en profiles/<name>/api_key con 0600) — mismo modo que ya usa
	// CfgInitOverlay al crear el archivo (cfg.go:147).
	if err := os.WriteFile(file, out, 0o644); err != nil {
		return fmt.Errorf("no se pudo escribir el overlay de %q: %w", name, err)
	}

	src, err := claudeSrc()
	if err != nil {
		return err
	}
	return CfgRegenerate(home, name, src)
}
```

- [ ] **Step 4: Correr los tests**

Run: `go test ./internal/core/ -run TestOverlayEnv -v`
Expected: PASS los cinco

- [ ] **Step 5: Commit (pedir autorización antes de ejecutarlo)**

```bash
git add internal/core/overlay_env.go internal/core/overlay_env_test.go
git commit -m "feat(core): OverlayEnvSet/Del sobre el overlay del perfil"
```

---

### Task 8: La vista de perfil, read-only

**Files:**
- Create: `internal/tui/profile_view.go`
- Create: `internal/tui/profile_view_test.go`
- Modify: `internal/tui/tui.go` (enum `mode`, campos del modelo, dispatch de `Update`)
- Modify: `internal/tui/dashboard.go` (`View` y la tecla `e` del panel Perfiles)
- Modify: `internal/core/cfg_cmd.go` (exponer `ClaudeSrc`)
- Modify: `internal/core/cfg.go` (exponer `ProfileSettingsFile`)
- Modify: `internal/core/i18n/catalog_tui.go`

**Interfaces:**
- Consumes: `renderView`/`panelSpec`/`rowSpec` (**Task 2 — dependencia dura**);
  `core.ProfileEffective`, `core.EffKind`, `core.EffRow`, `core.Origin`
  (**Task 6 — dependencia dura**). Deliberadamente NO consume nada de la Tarea
  7: el fixture del test siembra el overlay escribiendo el archivo a mano, no
  llamando a `core.OverlayEnvSet` — así esta tarea no obliga a hacer la 7 antes.
- Produces: `modeProfile`, `openProfileView(name string) (tea.Model, tea.Cmd)`,
  `updateProfileView(msg tea.Msg) (tea.Model, tea.Cmd)`, `viewProfile() string`.
  La tarea 9 les añade las teclas de escritura.

- [ ] **Step 1: Escribir el test de la vista**

```go
package tui

import (
	"os"
	"strings"
	"testing"

	"github.com/JoseAFlores777/ccp/internal/core"
	"github.com/JoseAFlores777/ccp/internal/core/i18n"
	tea "github.com/charmbracelet/bubbletea"
)

func profileViewModel(t *testing.T) *model {
	t.Helper()
	t.Setenv("CCP_CLAUDE_SRC", t.TempDir())
	home := t.TempDir()
	if err := core.ProfileAddOfficial(home, "a-cc"); err != nil {
		t.Fatalf("ProfileAddOfficial: %v", err)
	}
	// Escribe el overlay directo a disco (con CfgInitOverlay primero, igual que
	// seedEff/seedEnvHome) en vez de llamar a core.OverlayEnvSet: esta tarea NO
	// depende de la Tarea 7 — su Interfaces solo lista Task 2 y Task 6, y si el
	// fixture llamara a OverlayEnvSet el paquete no compilaría hasta escribir
	// también overlay_env.go.
	if err := core.CfgInitOverlay(home, "a-cc"); err != nil {
		t.Fatalf("CfgInitOverlay: %v", err)
	}
	overlay := `{"env":{"FOO":"1"}}`
	if err := os.WriteFile(core.ProfileSettingsFile(home, "a-cc"), []byte(overlay), 0o644); err != nil {
		t.Fatalf("escribir overlay: %v", err)
	}
	m := &model{home: home, focus: panelProfiles, mode: modeDashboard, width: 100, height: 40}
	m.reload()
	return m
}

func TestLaTeclaEAbreLaVistaDePerfil(t *testing.T) {
	m := profileViewModel(t)
	send(m, key('e'))
	if m.mode != modeProfile {
		t.Fatalf("'e' tiene que abrir la vista de perfil: mode=%v", m.mode)
	}
	if m.profName != "a-cc" {
		t.Fatalf("la vista tiene que ser del perfil seleccionado: %q", m.profName)
	}
	send(m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.mode != modeDashboard {
		t.Fatalf("esc vuelve al dashboard: mode=%v", m.mode)
	}
}

func TestVistaDePerfilRenderizaEnLosDosIdiomas(t *testing.T) {
	for _, l := range []i18n.Lang{i18n.Es, i18n.En} {
		m := profileViewModel(t)
		send(m, key('e'))
		m.lang = l
		out := m.viewProfile()
		if strings.TrimSpace(out) == "" {
			t.Fatalf("lang %q: render vacío", l)
		}
		if strings.Contains(out, "tui.") {
			t.Fatalf("lang %q: clave sin traducir:\n%s", l, out)
		}
		if !strings.Contains(out, "FOO") {
			t.Fatalf("lang %q: falta la variable del overlay:\n%s", l, out)
		}
	}
}

func TestTabCiclaLasTresCajas(t *testing.T) {
	m := profileViewModel(t)
	send(m, key('e'))
	if m.profPanel != profPanelInstr {
		t.Fatalf("arranca en Instrucciones: %v", m.profPanel)
	}
	send(m, tea.KeyMsg{Type: tea.KeyTab})
	if m.profPanel != profPanelEnv {
		t.Fatalf("tab lleva a Env: %v", m.profPanel)
	}
	send(m, tea.KeyMsg{Type: tea.KeyTab})
	if m.profPanel != profPanelEff {
		t.Fatalf("tab lleva a Efectivo: %v", m.profPanel)
	}
	send(m, tea.KeyMsg{Type: tea.KeyTab})
	if m.profPanel != profPanelInstr {
		t.Fatalf("tab cicla: %v", m.profPanel)
	}
}

func TestDefaultSeAbreSinOverlay(t *testing.T) {
	m := profileViewModel(t)
	m.profIdx = 0
	m.profiles = []string{"default"}
	send(m, key('e'))
	if m.mode != modeProfile {
		t.Fatalf("'default' tiene que abrirse igual: mode=%v", m.mode)
	}
	// No basta con "no renderiza vacío": eso pasaría igual si
	// ProfileEffective("default") reventara, porque viewProfile siempre pinta
	// el logo y las tres cajas pase lo que pase (probado sustituyendo
	// "default" por un perfil inexistente: el render tampoco sale vacío, pero
	// statusErr queda en true). La comprobación real es que el cálculo
	// terminó sin error y con contenido de verdad.
	if m.statusErr {
		t.Fatalf("'default' no puede quedar en error: %q", m.statusMsg)
	}
	if len(m.profEff.Sections) == 0 {
		t.Fatal("'default' no calculó ninguna sección de Effective")
	}
	if strings.TrimSpace(m.viewProfile()) == "" {
		t.Fatal("'default' renderiza vacío")
	}
}

// TestProfileEditDoneMsgSeDespachaEnLaVistaDePerfil fija que updateProfileView
// atiende profileEditDoneMsg (el mensaje que tea.Exec emite al volver del
// editor — lo usa la tecla 'e' de la Tarea 9). Sin este caso, el mensaje se
// pierde: updateProfileView descarta todo lo que no sea tea.KeyMsg, y nadie
// más en modeProfile lo atiende.
func TestProfileEditDoneMsgSeDespachaEnLaVistaDePerfil(t *testing.T) {
	m := profileViewModel(t)
	send(m, key('e'))
	ex := &profileEditExec{home: m.home, name: m.profName}
	_, _ = m.Update(profileEditDoneMsg{ex: ex})
	if m.mode != modeProfile {
		t.Fatalf("profileEditDoneMsg no debe sacar de la vista: mode=%v", m.mode)
	}
	if m.statusErr {
		t.Fatalf("una edición sin error no puede quedar en statusErr: %q", m.statusMsg)
	}
}
```

- [ ] **Step 2: Correr y verificar que falla**

Run: `go test ./internal/tui/ -run 'TestLaTeclaE|TestVistaDePerfil|TestTabCicla|TestDefaultSeAbre|TestProfileEditDoneMsg' -v`
Expected: FAIL con `undefined: modeProfile`, `m.profName undefined`, etc.

- [ ] **Step 3: Añadir el modo y el estado al modelo**

En `internal/tui/tui.go`, en el bloque `const` de `mode` (línea 39), después de
`modeConfig`:

```go
	modeProfile
```

Y en el struct `model`, después del bloque de la vista Config:

```go
	// vista de perfil (modeProfile): el perfil mirado, su Effective ya
	// calculado, la caja enfocada, la fila y el grupo desplegado de Efectivo.
	profName  string
	profEff   core.Effective
	profPanel profilePanel
	profRow   int
	profGroup core.EffKind // grupo desplegado en Efectivo
	profOpen  bool         // Efectivo desplegada
```

En `Update` (línea 160), añade la rama:

```go
	case modeProfile:
		return m.updateProfileView(msg)
```

Y en `View` (`internal/tui/dashboard.go:324`):

```go
	case modeProfile:
		return m.viewProfile()
```

- [ ] **Step 4: Cambiar la tecla `e` del panel Perfiles**

En `internal/tui/dashboard.go:134`, sustituye la llamada a `m.editConfig(name)`:

```go
	case "e": // vista del perfil
		if name := m.selectedProfile(); name != "" {
			return m.openProfileView(name)
		}
```

`editConfig` y `profileEditExec` **se quedan**: la vista los usa para su propia
tecla `e` en la tarea 9.

- [ ] **Step 5: Escribir `profile_view.go`**

```go
package tui

// profile_view.go — la vista de un perfil (modeProfile): qué configuración
// aplica y de dónde sale cada cosa.
//
// Usa el chrome del dashboard (logo, tres cajas, cursor, pie) a través del
// shell, no un render propio. Tres cajas espejando Perfiles | Reglas | Estado:
// Instrucciones y Env son las editables; Efectivo junta permisos, hooks,
// plugins y sensores en modo lectura, plegados en cuatro conteos.

import (
	"fmt"
	"strings"

	"github.com/JoseAFlores777/ccp/internal/core"
	"github.com/JoseAFlores777/ccp/internal/core/i18n"
	tea "github.com/charmbracelet/bubbletea"
)

type profilePanel int

const (
	profPanelInstr profilePanel = iota
	profPanelEnv
	profPanelEff
	numProfPanels
)

// openProfileView calcula el Effective una vez y entra en la vista.
func (m *model) openProfileView(name string) (tea.Model, tea.Cmd) {
	m.showDetail = false
	m.profName = name
	m.profPanel = profPanelInstr
	m.profRow = 0
	m.profOpen = false
	m.mode = modeProfile
	m.reloadProfileEff()
	return m, nil
}

// reloadProfileEff recalcula el Effective. Se llama al entrar y después de cada
// escritura: la vista nunca pinta datos que ella misma acaba de invalidar.
func (m *model) reloadProfileEff() {
	src, err := core.ClaudeSrc()
	if err != nil {
		m.setStatus("", err)
		return
	}
	eff, err := core.ProfileEffective(m.home, m.profName, src)
	if err != nil {
		m.setStatus("", err)
		return
	}
	m.profEff = eff
}

func (m *model) updateProfileView(msg tea.Msg) (tea.Model, tea.Cmd) {
	// profileEditDoneMsg lo emite tea.Exec al volver del editor (la tecla 'e'
	// de la Tarea 9). SIN este caso, ese mensaje se pierde en silencio: el
	// resto de esta función descarta todo lo que no sea tea.KeyMsg, y nadie
	// más en modeProfile lo atiende — el mismo patrón que updateConfig ya
	// resuelve para configEditDoneMsg (config_view.go:392-393).
	if em, ok := msg.(profileEditDoneMsg); ok {
		return m.finishProfileEdit(em)
	}
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch km.String() {
	case "q", "ctrl+c":
		m.quitting = true
		return m, tea.Quit
	case "esc":
		m.mode = modeDashboard
		return m, nil
	case "tab":
		m.profPanel = (m.profPanel + 1) % numProfPanels
		m.profRow = 0
		return m, nil
	case "shift+tab":
		m.profPanel = (m.profPanel + numProfPanels - 1) % numProfPanels
		m.profRow = 0
		return m, nil
	case "j", "down":
		if m.profRow < len(m.profRows())-1 {
			m.profRow++
		}
		return m, nil
	case "k", "up":
		if m.profRow > 0 {
			m.profRow--
		}
		return m, nil
	case "enter":
		if m.profPanel == profPanelEff {
			if m.profOpen {
				m.profOpen = false
			} else {
				// El grupo desplegado sale de la fila del resumen sobre la que
				// se pulsó: effGroups fija ese orden en un solo sitio para que
				// la fila y el grupo no puedan desincronizarse.
				g := effGroups()
				if m.profRow < len(g) {
					m.profGroup = g[m.profRow]
					m.profOpen = true
				}
			}
			m.profRow = 0
		}
		return m, nil
	}
	return m, nil
}

// section devuelve la sección del Effective de un Kind, o una vacía.
func (m *model) section(k core.EffKind) core.EffSection {
	for _, s := range m.profEff.Sections {
		if s.Kind == k {
			return s
		}
	}
	return core.EffSection{Kind: k}
}

// profRows son las filas de la caja enfocada; de aquí sale el tope del cursor.
func (m *model) profRows() []rowSpec {
	switch m.profPanel {
	case profPanelInstr:
		return m.effRowSpecs(m.section(core.EffInstructions).Rows)
	case profPanelEnv:
		return m.effRowSpecs(m.section(core.EffEnv).Rows)
	default:
		if m.profOpen {
			return m.effRowSpecs(m.section(m.profGroup).Rows)
		}
		return m.effSummaryRows()
	}
}

// effRowSpecs convierte filas del core en filas del shell. La columna de
// origen va alineada a la derecha del texto; el cursor lo pone el shell.
func (m *model) effRowSpecs(rows []core.EffRow) []rowSpec {
	out := make([]rowSpec, 0, len(rows))
	for _, r := range rows {
		txt := r.Key
		if r.Value != "" {
			txt += " = " + r.Value
		}
		txt += "   " + m.originLabel(r.Origin)
		if r.Shadowed {
			txt += " ⊕"
		}
		out = append(out, rowSpec{Text: txt})
	}
	return out
}

// originLabel traduce core.Origin — String() (core.go) da "global"/"overlay"/
// "auto" en duro, sin pasar por i18n. Esos SON las claves EN, así que en EN
// coincide por construcción; en ES no ("overlay" no es "overlay" en el sentido
// que el resto de la UI usa esa palabra prestada, pero "auto" sí lo es — el
// punto es que la fuente de verdad del texto tiene que ser el catálogo, no un
// String() de core, aunque hoy el valor termine siendo igual).
func (m *model) originLabel(o core.Origin) string {
	switch o {
	case core.OriginGlobal:
		return i18n.T(m.lang, "tui.profview.origin_global")
	case core.OriginOverlay:
		return i18n.T(m.lang, "tui.profview.origin_overlay")
	default:
		return i18n.T(m.lang, "tui.profview.origin_auto")
	}
}

// effGroups es el orden de los grupos de la caja Efectivo, en UN solo sitio: lo
// usan el resumen (para pintar las filas) y `enter` (para saber qué grupo
// desplegó el usuario). Dos listas separadas se desincronizan y el usuario
// termina abriendo Plugins cuando pulsó sobre Hooks.
func effGroups() []core.EffKind {
	return []core.EffKind{core.EffPermissions, core.EffHooks, core.EffPlugins, core.EffSensors}
}

func effGroupKey(k core.EffKind) string {
	switch k {
	case core.EffPermissions:
		return "tui.profview.permissions"
	case core.EffHooks:
		return "tui.profview.hooks"
	case core.EffPlugins:
		return "tui.profview.plugins"
	default:
		return "tui.profview.sensors"
	}
}

// effSummaryRows es la caja Efectivo plegada: cuatro conteos, que son la
// respuesta a «qué aplica este perfil». El detalle se pide con enter.
func (m *model) effSummaryRows() []rowSpec {
	groups := effGroups()
	out := make([]rowSpec, 0, len(groups))
	for _, k := range groups {
		out = append(out, rowSpec{
			Text: fmt.Sprintf("%s   %d", i18n.T(m.lang, effGroupKey(k)), len(m.section(k).Rows)),
		})
	}
	return out
}

// viewProfile no fija CollapseWhenUnfocused en ninguna de las tres cajas — se
// queda en su valor por defecto (false), igual que el dashboard (Tarea 4) y a
// diferencia de Config (Tarea 5): las tres cajas de esta vista pintan sus
// filas SIEMPRE, tengan foco o no. Es lo que hace que la variable del overlay
// se vea nada más abrir la vista, sin tener que tabular hasta Env primero —
// y es lo que pide el mockup del spec (las tres cajas muestran contenido real
// aunque no tengan el foco).
func (m *model) viewProfile() string {
	head := logoBanner(m.lang) + "\n" +
		styleDim.Render(i18n.T(m.lang, "tui.profview.eyebrow", m.profName))

	instr := m.section(core.EffInstructions)
	env := m.section(core.EffEnv)

	panels := []panelSpec{
		{
			Title:   i18n.T(m.lang, "tui.profview.instructions"),
			Hint:    i18n.T(m.lang, "tui.profview.instructions_hint"),
			Empty:   i18n.T(m.lang, "tui.profview.instructions_empty"),
			Summary: i18n.T(m.lang, "tui.profview.instructions_sum", len(instr.Rows)),
			Focused: m.profPanel == profPanelInstr,
			Cursor:  m.profRow,
			MaxRows: 8,
			Rows:    m.effRowSpecs(instr.Rows),
		},
		{
			Title:   i18n.T(m.lang, "tui.profview.env"),
			Hint:    i18n.T(m.lang, "tui.profview.env_hint"),
			Empty:   i18n.T(m.lang, "tui.profview.env_empty"),
			Summary: i18n.T(m.lang, "tui.profview.env_sum", len(env.Rows)),
			Focused: m.profPanel == profPanelEnv,
			Cursor:  m.profRow,
			MaxRows: 8,
			Rows:    m.effRowSpecs(env.Rows),
		},
		{
			Title:   i18n.T(m.lang, "tui.profview.effective"),
			Hint:    i18n.T(m.lang, "tui.profview.effective_hint"),
			Summary: i18n.T(m.lang, "tui.profview.effective_sum"),
			Focused: m.profPanel == profPanelEff,
			Cursor:  m.profRow,
			MaxRows: 12,
			Rows:    m.profRows(),
		},
	}
	if m.profPanel != profPanelEff {
		panels[2].Rows = m.effSummaryRows()
	}

	v := viewSpec{
		Header:    head,
		Panels:    panels,
		Status:    m.statusMsg,
		StatusErr: m.statusErr,
		Footer:    i18n.T(m.lang, "tui.profview.footer"),
	}
	if err := m.profErr(); err != "" {
		v.Extra = styleErr.Render(err)
	}
	return m.renderView(v)
}

// profErr junta los errores por sección en una línea: un overlay roto tiene que
// verse, pero no puede tumbar la vista.
//
// El spec pide el error "EN SU FILA" — dentro de la caja afectada, no en una
// línea compartida debajo de las tres. Aquí se queda en Extra a propósito:
// meterlo como una fila más de la caja desplaza `Cursor` (el mismo problema
// que ya evitó la nota de sección en la Tarea 5 — "una fila que desplaza el
// cursor es la clase de detalle que rompe la navegación seis meses después"),
// y la caja no tiene otro sitio para un texto que no sea una fila navegable.
// La mitad que SÍ importa para la correctud —que la caja rota no admita
// escritura— la hace `blockedByErr` en `profileAdd`/`profileDel`/`enter`, no
// esto: esto es solo para que el usuario vea el porqué.
func (m *model) profErr() string {
	var msgs []string
	for _, s := range m.profEff.Sections {
		if s.Err != nil {
			msgs = append(msgs, s.Err.Error())
		}
	}
	return strings.Join(msgs, " · ")
}
```

`core.ClaudeSrc()` no existe todavía: `claudeSrc()` es privada
(`internal/core/cfg_cmd.go:17`). Exponla añadiendo en ese mismo archivo:

```go
// ClaudeSrc expone la fuente global (CCP_CLAUDE_SRC o ~/.claude) a los
// front-ends, que la necesitan para ProfileEffective.
func ClaudeSrc() (string, error) { return claudeSrc() }
```

Tampoco existe `core.ProfileSettingsFile` — `cfgSettingsFile` es privada
(`internal/core/cfg.go:33`), y hace falta tanto para el fixture del test de
arriba como para `profFile()` (Tarea 9). Exponla en `internal/core/cfg.go`,
junto a `cfgSettingsFile`:

```go
// ProfileSettingsFile expone la ruta del settings.overlay.json de un perfil a
// los front-ends (la vista de perfil la usa para sembrar overlays en tests y
// para saber qué archivo abre 'e' sobre la caja Env/Efectivo).
func ProfileSettingsFile(home, name string) string { return cfgSettingsFile(home, name) }
```

- [ ] **Step 6: Añadir las claves de i18n**

`tui.profiles.hint` (`internal/core/i18n/catalog_tui.go:63-66`) dice hoy
`"e:overlay"` — texto de cuando 'e' abría el editor crudo sobre los dos
overlays (el arreglo de pico, antes de este plan). Ahora 'e' abre la vista, no
el editor directamente, así que ese texto miente. Reemplaza ESE bloque exacto:

```go
	"tui.profiles.hint": {
		En: "a:add d:delete r:rename s:key e:overlay l:login enter:detail",
		Es: "a:añadir d:borrar r:renombrar s:key e:overlay l:login enter:detalle",
	},
```

por:

```go
	"tui.profiles.hint": {
		En: "a:add d:delete r:rename s:key e:view l:login enter:detail",
		Es: "a:añadir d:borrar r:renombrar s:key e:vista l:login enter:detalle",
	},
```

Y en `internal/core/i18n/catalog_tui.go`, en el bloque de claves `tui.`, añade:

```go
	"tui.profview.eyebrow": {
		En: "profile %s",
		Es: "perfil %s",
	},
	"tui.profview.instructions": {
		En: "Instructions",
		Es: "Instrucciones",
	},
	"tui.profview.instructions_hint": {
		En: "a:add d:delete e:edit file",
		Es: "a:añadir d:borrar e:editar archivo",
	},
	"tui.profview.instructions_empty": {
		En: "(no instructions of its own — 'a' to add)",
		Es: "(sin instrucciones propias — 'a' para añadir)",
	},
	"tui.profview.instructions_sum": {
		En: "%d entries",
		Es: "%d entradas",
	},
	"tui.profview.env": {
		En: "Env",
		Es: "Env",
	},
	"tui.profview.env_hint": {
		En: "a:add enter:edit d:delete e:edit file",
		Es: "a:añadir enter:editar d:borrar e:editar archivo",
	},
	"tui.profview.env_empty": {
		En: "(no variables — 'a' to add)",
		Es: "(sin variables — 'a' para añadir)",
	},
	"tui.profview.env_sum": {
		En: "%d variables",
		Es: "%d variables",
	},
	"tui.profview.effective": {
		En: "Effective",
		Es: "Efectivo",
	},
	"tui.profview.effective_hint": {
		En: "enter:expand a:add hook",
		Es: "enter:desplegar a:añadir hook",
	},
	"tui.profview.effective_sum": {
		En: "permissions · hooks · plugins · sensors",
		Es: "permisos · hooks · plugins · sensores",
	},
	"tui.profview.permissions": {
		En: "Permissions",
		Es: "Permisos",
	},
	"tui.profview.hooks": {
		En: "Hooks",
		Es: "Hooks",
	},
	"tui.profview.plugins": {
		En: "Plugins",
		Es: "Plugins",
	},
	"tui.profview.sensors": {
		En: "Sensors",
		Es: "Sensores",
	},
	"tui.profview.footer": {
		En: "tab: panel · j/k: navigate · e: edit file · esc: back · q: quit",
		Es: "tab: panel · j/k: navegar · e: editar archivo · esc: volver · q: salir",
	},
	"tui.profview.origin_global": {
		En: "global",
		Es: "global",
	},
	"tui.profview.origin_overlay": {
		En: "overlay",
		Es: "overlay",
	},
	"tui.profview.origin_auto": {
		En: "auto",
		Es: "auto",
	},
```

- [ ] **Step 7: Correr los tests**

Run: `go test ./internal/tui/ -run 'TestLaTeclaE|TestVistaDePerfil|TestTabCicla|TestDefaultSeAbre|TestProfileEditDoneMsg' -v`
Expected: PASS los cinco

- [ ] **Step 8: Correr la suite entera y los gates**

Run: `go test ./... 2>&1 | tail -12 && gofmt -l internal cmd && go vet ./...`
Expected: todo verde.

- [ ] **Step 9: Commit (pedir autorización antes de ejecutarlo)**

```bash
git add internal/tui/profile_view.go internal/tui/profile_view_test.go \
        internal/tui/tui.go internal/tui/dashboard.go \
        internal/core/cfg_cmd.go internal/core/cfg.go internal/core/i18n/catalog_tui.go
git commit -m "feat(tui): vista de perfil con la procedencia de su config"
```

---

### Task 9: Escritura desde la vista

**Files:**
- Modify: `internal/tui/profile_view.go` (teclas `a`, `d`, `enter` en Env, `e`)
- Modify: `internal/tui/forms.go` (tres formularios nuevos)
- Modify: `internal/tui/profile_view_test.go`
- Modify: `internal/tui/tui.go` (`exitForm`)
- Modify: `internal/tui/dashboard.go` (`profileEditExec`)
- Modify: `internal/core/cfg_cmd.go` (`ProfileConfigOpts.File`, `ProfileConfig`)
- Modify: `internal/core/cfg.go` (exponer `ProfileInstrFile`)
- Modify: `internal/core/cfg_test.go`
- Modify: `internal/core/i18n/catalog_tui.go`

**Interfaces:**
- Consumes: `core.OverlayEnvSet`/`OverlayEnvDel` (**Task 7 — dependencia
  dura**: los tests de escritura de Env y el `enter`-edición llaman a estas
  funciones); `core.InstructRuleAdd`/`InstructRuleRm`/`InstructRuleList`
  (`instruct.go`); `core.InstructAdd`/`core.InstructCtx`
  (`instruct_cmd.go:17,47`); `core.CfgValidateBytes` (`cfg.go:167`);
  `m.start(action)` y el tipo `action` (`internal/tui/dashboard.go:189`,
  `internal/tui/forms.go`); `m.editConfig(name)` y `profileEditExec`
  (`internal/tui/dashboard.go`); `m.blockedByErr` — se define en el Step 5 de
  ESTA tarea pero se usa también en el Step 3: es la misma tarea, se pega todo
  antes de compilar, así que el orden textual no importa.
- Produces: `formAddRuleToProfile(home, name string, lang i18n.Lang) action`,
  `formSetOverlayEnv(home, name, key, val string, lang i18n.Lang) action`,
  `formAddHookToProfile(home, src, name string, lang i18n.Lang) action`.

- [ ] **Step 1: Escribir los tests de escritura**

```go
// TestAAbreUnFormQueVuelveALaVista comprueba el plomería del form embebido
// (que 'a' lo abre y que vuelve a modeProfile, no al dashboard) — NO que la
// regla termine escrita: eso requiere simular la escritura interactiva dentro
// del huh.Form, y ya está cubierto por separado a nivel de core
// (InstructRuleAdd tiene sus propios tests; formAddRuleToProfile.apply solo
// los invoca). El nombre lo dice para no prometer de más.
func TestAAbreUnFormQueVuelveALaVista(t *testing.T) {
	m := profileViewModel(t)
	send(m, key('e')) // vista de perfil, caja Instrucciones
	send(m, key('a')) // form
	if m.mode != modeForm {
		t.Fatalf("'a' tiene que abrir un form: mode=%v", m.mode)
	}
	// El form vuelve a la vista de perfil, no al dashboard.
	if m.formBack != modeProfile {
		t.Fatalf("el form tiene que devolver a la vista: %v", m.formBack)
	}
}

func TestBorrarEnvLlamaAOverlayEnvDel(t *testing.T) {
	m := profileViewModel(t)
	send(m, key('e'))
	send(m, tea.KeyMsg{Type: tea.KeyTab}) // Env
	send(m, key('d'))                     // borra la fila 0 (FOO)
	if m.statusErr {
		t.Fatalf("el borrado falló: %q", m.statusMsg)
	}
	eff, err := core.ProfileEffective(m.home, "a-cc", os.Getenv("CCP_CLAUDE_SRC"))
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range eff.Sections {
		if s.Kind == core.EffEnv {
			for _, r := range s.Rows {
				if r.Key == "FOO" {
					t.Fatal("FOO tenía que desaparecer del overlay")
				}
			}
		}
	}
}

func TestBorrarUnHookExplicaQueNoSePuede(t *testing.T) {
	m := profileViewModel(t)
	send(m, key('e'))
	send(m, tea.KeyMsg{Type: tea.KeyTab})
	send(m, tea.KeyMsg{Type: tea.KeyTab}) // Efectivo
	send(m, key('d'))
	if !m.statusErr || m.statusMsg == "" {
		t.Fatal("'d' sobre Efectivo tiene que explicar por qué no borra")
	}
}

func TestBorrarLaFilaGlobalDeInstruccionesNoBorraNada(t *testing.T) {
	m := profileViewModel(t)
	// Sembramos un CLAUDE.md global para que exista la fila.
	src := os.Getenv("CCP_CLAUDE_SRC")
	if err := os.WriteFile(filepath.Join(src, "CLAUDE.md"), []byte("hola\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	send(m, key('e'))
	m.profRow = 0 // la fila global
	send(m, key('d'))
	if !m.statusErr {
		t.Fatal("'d' sobre la fila global tiene que decir que eso es la config global")
	}
	if _, err := os.Stat(filepath.Join(src, "CLAUDE.md")); err != nil {
		t.Fatal("el CLAUDE.md global no se puede tocar")
	}
}

// TestETrasNavegarAbreElArchivoDeLaCajaEnfocada: ningún test del plan hasta
// aquí presiona 'e' DENTRO de la vista de perfil (todos los de la Tarea 8
// prueban 'e' para ENTRAR a la vista, no la 'e' de dentro). No se invoca el
// tea.Cmd que devuelve editProfileFile —send() lo descarta a propósito, igual
// que en el resto de estos tests— porque eso lanzaría un editor de verdad;
// profFile() es la decisión que ese Cmd usa, y es lo que se puede probar sin
// tocar un proceso externo.
func TestETrasNavegarAbreElArchivoDeLaCajaEnfocada(t *testing.T) {
	m := profileViewModel(t)
	send(m, key('e')) // abre la vista, arranca en Instrucciones

	instrFile := core.ProfileInstrFile(m.home, m.profName)
	settingsFile := core.ProfileSettingsFile(m.home, m.profName)

	if got := m.profFile(); got != instrFile {
		t.Fatalf("Instrucciones: quiero %q, tengo %q", instrFile, got)
	}
	send(m, tea.KeyMsg{Type: tea.KeyTab}) // Env
	if got := m.profFile(); got != settingsFile {
		t.Fatalf("Env: quiero %q, tengo %q", settingsFile, got)
	}
	send(m, tea.KeyMsg{Type: tea.KeyTab}) // Efectivo
	if got := m.profFile(); got != settingsFile {
		t.Fatalf("Efectivo: quiero %q, tengo %q", settingsFile, got)
	}

	// Y que 'e' de verdad dispara el camino de editProfileFile sin errores ni
	// sacar de la vista.
	send(m, key('e'))
	if m.mode != modeProfile {
		t.Fatalf("'e' no puede sacar de la vista: mode=%v", m.mode)
	}
}

// TestEEnSensoresNoTieneArchivo: Sensores no vive en ningún archivo — su
// fuente de verdad es auto_handoff.hooks en ccp.yaml (se toca con 'c'), no
// overlay/settings.overlay.json. Sin la guarda de profFile(), 'e' sobre
// Sensores intentaría abrir el settingsFile por descuido (comparte el
// `default:` de profFile() con Permisos/Hooks/Plugins, que sí tienen archivo).
func TestEEnSensoresNoTieneArchivo(t *testing.T) {
	m := profileViewModel(t)
	send(m, key('e'))
	send(m, tea.KeyMsg{Type: tea.KeyTab})
	send(m, tea.KeyMsg{Type: tea.KeyTab}) // Efectivo, plegada
	m.profRow = 3                         // Sensores es la 4ª fila del resumen (effGroups)
	send(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.profGroup != core.EffSensors || !m.profOpen {
		t.Fatalf("enter en la fila 3 tiene que desplegar Sensores: group=%v open=%v", m.profGroup, m.profOpen)
	}
	if got := m.profFile(); got != "" {
		t.Fatalf("Sensores no tiene archivo editable: %q", got)
	}
	send(m, key('e'))
	if !m.statusErr {
		t.Fatal("'e' sobre Sensores tiene que explicar que no hay archivo, no fallar en silencio")
	}
}
```

Añade `"path/filepath"` a los imports de `profile_view_test.go` (`"os"` ya
está desde la Tarea 8).

- [ ] **Step 2: Correr y verificar que falla**

Run: `go test ./internal/tui/ -run 'TestAAbreUnForm|TestBorrarEnv|TestBorrarUnHook|TestBorrarLaFilaGlobal|TestETrasNavegar|TestEEnSensores' -v`
Expected: FAIL — `a` y `d` todavía no hacen nada, así que `m.mode` sigue en
`modeProfile` y `m.statusErr` en `false`; `TestETrasNavegar...` falla con
`m.profFile undefined`.

- [ ] **Step 3: Añadir las teclas al `switch` de `updateProfileView`**

Dentro del `switch km.String()`, antes del `return m, nil` final:

```go
	case "a":
		return m.profileAdd()
	case "d":
		return m.profileDel()
	case "e":
		if f := m.profFile(); f != "" {
			return m.editProfileFile(f)
		}
		m.setStatus("", errCmd{i18n.T(m.lang, "tui.profview.no_file")})
		return m, nil
```

Y en el `case "enter"`, antes de la rama de `profPanelEff`:

```go
		if m.profPanel == profPanelEnv {
			if m.blockedByErr(core.EffEnv) {
				return m, nil
			}
			rows := m.section(core.EffEnv).Rows
			if m.profRow < len(rows) {
				r := rows[m.profRow]
				if r.Origin == core.OriginGlobal {
					m.setStatus("", errCmd{i18n.T(m.lang, "tui.profview.global_row")})
					return m, nil
				}
				return m.start(formSetOverlayEnv(m.home, m.profName, r.Key, r.Value, m.lang))
			}
			return m, nil
		}
```

- [ ] **Step 4: Que `e` abra UN archivo, el de la caja enfocada**

El spec pide que `e` abra el archivo de la sección, no los dos overlays.
`ProfileConfig` abre los dos (en serie desde el arreglo de pico), así que gana
una opción para acotarlo. En `internal/core/cfg_cmd.go`, en `ProfileConfigOpts`:

```go
	// File acota la edición a UN archivo del overlay (ruta absoluta, tiene que
	// ser el de instrucciones o el de settings de ESTE perfil). Vacío = los dos.
	// Lo usa la vista de perfil, donde 'e' edita la caja enfocada y abrir el
	// otro archivo sería abrir algo que el usuario no estaba mirando.
	File string
```

En `ProfileConfig` (`internal/core/cfg_cmd.go`), reemplaza ESTE bloque exacto
(ya existe tal cual, del arreglo de pico de una sesión anterior — declaraciones
Y bucle, los dos, no solo el bucle: pegar únicamente un `for` nuevo encima de
estas mismas declaraciones sería redeclarar `editor`/`instr`/`settings` con
`:=` en el mismo scope, y no compila):

```go
	editor := ResolveEditor(home)
	instr := cfgInstrFile(home, name)
	settings := cfgSettingsFile(home, name)
	for _, f := range []string{instr, settings} {
		if err := launch(editor, f); err != nil {
			return fmt.Errorf("el editor falló: %w", err)
		}
	}
```

por:

```go
	editor := ResolveEditor(home)
	instr := cfgInstrFile(home, name)
	settings := cfgSettingsFile(home, name)
	files := []string{instr, settings}
	if opts.File != "" {
		if opts.File != instr && opts.File != settings {
			return fmt.Errorf("%q no es un overlay de %q", opts.File, name)
		}
		files = []string{opts.File}
	}
	for _, f := range files {
		if err := launch(editor, f); err != nil {
			return fmt.Errorf("el editor falló: %w", err)
		}
	}
```

La validación no es paranoia decorativa: `opts.File` llega desde `EffSection.File`
y una ruta arbitraria abriría el editor sobre cualquier archivo del disco con la
excusa de «editar un perfil».

En `internal/tui/profile_view.go`, la variante de `editConfig` que pasa el
archivo. `profileEditExec` (`internal/tui/dashboard.go:205-214`) gana un campo
`file` — añádelo justo después de `name string`, dejando el resto de campos
(`err`, `in`, `out`, `errw`) tal cual están:

```go
type profileEditExec struct {
	home string
	name string
	file string // "" = los dos overlays; si no, solo ese

	err error

	in   io.Reader
	out  io.Writer
	errw io.Writer
}
```

Y su `Run()` (`internal/tui/dashboard.go`) pasa `File: c.file`:

```go
func (c *profileEditExec) Run() error {
	c.err = core.ProfileConfig(c.home, c.name, core.ProfileConfigOpts{
		Launch: c.launch,
		File:   c.file,
	})
	return nil
}
```

y la vista:

```go
// editProfileFile abre SOLO el archivo de la caja enfocada, cediendo la terminal
// con tea.Exec igual que hace la 'e' del dashboard.
func (m *model) editProfileFile(file string) (tea.Model, tea.Cmd) {
	ex := &profileEditExec{home: m.home, name: m.profName, file: file}
	return m, tea.Exec(ex, func(err error) tea.Msg {
		return profileEditDoneMsg{ex: ex, err: err}
	})
}
```

`finishProfileEdit` ya recarga y reporta; añádele, después de `m.reload()`:

```go
	if m.mode == modeProfile {
		m.reloadProfileEff()
	}
```

Test que lo fija, en `internal/core/cfg_test.go`:

```go
func TestProfileConfigConFileAbreSoloEse(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CCP_CLAUDE_SRC", t.TempDir())
	if err := ProfileAddOfficial(home, "work"); err != nil {
		t.Fatal(err)
	}
	var calls [][]string
	launch := func(_ string, files ...string) error {
		calls = append(calls, files)
		return nil
	}
	only := cfgSettingsFile(home, "work")
	if err := ProfileConfig(home, "work", ProfileConfigOpts{Launch: launch, File: only}); err != nil {
		t.Fatalf("ProfileConfig: %v", err)
	}
	if len(calls) != 1 || len(calls[0]) != 1 || calls[0][0] != only {
		t.Fatalf("con File solo se abre ese archivo: %v", calls)
	}

	// Y una ruta que no es de este perfil se rechaza.
	err := ProfileConfig(home, "work", ProfileConfigOpts{Launch: launch, File: "/etc/passwd"})
	if err == nil {
		t.Fatal("una ruta ajena tiene que rechazarse")
	}
}
```

Run: `go test ./internal/core/ -run TestProfileConfig -v`
Expected: PASS, incluidos los dos que ya existían.

- [ ] **Step 5: Escribir las acciones**

```go
// profFile es el archivo editable de la caja enfocada; "" si no lo hay. En
// Efectivo depende de qué grupo esté desplegado: Sensores no vive en ningún
// archivo (su fuente de verdad es auto_handoff.hooks en ccp.yaml, se toca
// desde la vista Config con 'c' — spec:129-132), así que 'e' ahí tiene que
// devolver "" y caer en tui.profview.no_file, no en el settingsFile de
// Permisos/Hooks/Plugins por descuido.
func (m *model) profFile() string {
	switch m.profPanel {
	case profPanelInstr:
		return m.section(core.EffInstructions).File
	case profPanelEnv:
		return m.section(core.EffEnv).File
	default:
		if m.profOpen && m.profGroup == core.EffSensors {
			return ""
		}
		return m.section(core.EffPermissions).File
	}
}

// blockedByErr rechaza una escritura cuando la sección tiene EffSection.Err —
// el spec lo pide explícito: «esa caja... desactiva su escritura; las otras
// siguen» (spec:246-249). Sin este chequeo, escribir sobre un overlay roto
// pisaría un archivo que el usuario ni siquiera pudo ver bien en la caja.
func (m *model) blockedByErr(k core.EffKind) bool {
	if err := m.section(k).Err; err != nil {
		m.setStatus("", err)
		return true
	}
	return false
}

// profileAdd: en Instrucciones añade una regla, en Env una variable, en
// Efectivo un hook. Cada una por el camino que ya existe en core, y 'default'
// se rechaza igual que ProfileConfig: no tiene overlay, no hay dónde escribir.
func (m *model) profileAdd() (tea.Model, tea.Cmd) {
	if m.profName == "default" {
		m.setStatus("", errCmd{i18n.T(m.lang, "tui.profview.no_overlay_default")})
		return m, nil
	}
	switch m.profPanel {
	case profPanelInstr:
		if m.blockedByErr(core.EffInstructions) {
			return m, nil
		}
		return m.start(formAddRuleToProfile(m.home, m.profName, m.lang))
	case profPanelEnv:
		if m.blockedByErr(core.EffEnv) {
			return m, nil
		}
		return m.start(formSetOverlayEnv(m.home, m.profName, "", "", m.lang))
	default:
		if m.blockedByErr(core.EffHooks) {
			return m, nil
		}
		src, err := core.ClaudeSrc()
		if err != nil {
			m.setStatus("", err)
			return m, nil
		}
		return m.start(formAddHookToProfile(m.home, src, m.profName, m.lang))
	}
}

// profileDel borra lo que se puede borrar, y explica lo que no.
//
// Dos cosas NO se borran y las dos lo dicen en vez de fingir: cualquier fila
// OriginGlobal (es tu ~/.claude, no el overlay del perfil — pasa tanto en
// Instrucciones como en Env, porque effMapRows mezcla las dos capas) y
// cualquier fila de Efectivo (los hooks viven en arrays sin id estable, así
// que InstructRm sólo los saca del manifiesto). Una tecla que explica es mejor
// que una que miente.
func (m *model) profileDel() (tea.Model, tea.Cmd) {
	switch m.profPanel {
	case profPanelInstr:
		if m.blockedByErr(core.EffInstructions) {
			return m, nil
		}
		s := m.section(core.EffInstructions)
		if m.profRow >= len(s.Rows) {
			return m, nil
		}
		if s.Rows[m.profRow].Origin == core.OriginGlobal {
			m.setStatus("", errCmd{i18n.T(m.lang, "tui.profview.global_row")})
			return m, nil
		}
		// El índice de InstructRuleRm es 1-based sobre las reglas del overlay,
		// y las filas globales van primero: hay que descontarlas.
		idx := m.profRow + 1
		for _, r := range s.Rows {
			if r.Origin == core.OriginGlobal {
				idx--
			}
		}
		if err := core.InstructRuleRm(s.File, idx); err != nil {
			m.setStatus("", err)
			return m, nil
		}
		m.setStatus(i18n.T(m.lang, "tui.profview.rule_removed"), nil)
	case profPanelEnv:
		if m.blockedByErr(core.EffEnv) {
			return m, nil
		}
		rows := m.section(core.EffEnv).Rows
		if m.profRow >= len(rows) {
			return m, nil
		}
		// EffEnv mezcla global+overlay (effMapRows, Tarea 6): una fila
		// OriginGlobal es tu ~/.claude/settings.json, no el overlay de este
		// perfil. OverlayEnvDel de una clave que el overlay ni define es un
		// no-op silencioso (TestOverlayEnvDelDeClaveInexistenteNoFalla, Tarea
		// 7) — sin esta guarda, "borrar" una variable global reportaría éxito
		// y la fila seguiría ahí tras recargar.
		if rows[m.profRow].Origin == core.OriginGlobal {
			m.setStatus("", errCmd{i18n.T(m.lang, "tui.profview.global_row")})
			return m, nil
		}
		if err := core.OverlayEnvDel(m.home, m.profName, rows[m.profRow].Key); err != nil {
			m.setStatus("", err)
			return m, nil
		}
		m.setStatus(i18n.T(m.lang, "tui.profview.env_removed", rows[m.profRow].Key), nil)
	default:
		m.setStatus("", errCmd{i18n.T(m.lang, "tui.profview.no_delete")})
		return m, nil
	}
	m.reloadProfileEff()
	if m.profRow > 0 {
		m.profRow--
	}
	return m, nil
}
```

- [ ] **Step 6: Escribir los formularios**

Añade `"strings"` al bloque de imports de `internal/tui/forms.go` (hoy solo
importa `"fmt"`, `"time"`, `core`, `i18n`, `huh` — `grep -c "strings\."
internal/tui/forms.go` da 0). Los tres formularios que siguen usan
`strings.TrimSpace` en su `Validate`; sin el import, el paquete no compila.

En `internal/tui/forms.go`, siguiendo el patrón de `formAddRule`
(`internal/tui/forms.go:211`):

```go
// formAddRuleToProfile añade una regla al bloque gestionado del CLAUDE.md del
// overlay del perfil. Usa InstructRuleAdd sobre el archivo directamente: así no
// pasa por InstructCtx, que trabaja sobre el perfil ACTIVO de la terminal y no
// sobre el que se está mirando.
func formAddRuleToProfile(home, name string, lang i18n.Lang) action {
	var text string
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title(i18n.T(lang, "tui.form.rule_text")).
				Value(&text).
				Validate(func(s string) error {
					if strings.TrimSpace(s) == "" {
						return fmt.Errorf("%s", i18n.T(lang, "tui.form.rule_text_empty"))
					}
					return nil
				}),
		),
	)
	apply := func() (string, error) {
		if err := core.CfgInitOverlay(home, name); err != nil {
			return "", err
		}
		file := core.ProfileInstrFile(home, name)
		added, err := core.InstructRuleAdd(file, text)
		if err != nil {
			return "", err
		}
		if !added {
			return i18n.T(lang, "tui.form.rule_dup"), nil
		}
		src, err := core.ClaudeSrc()
		if err != nil {
			return "", err
		}
		if err := core.CfgRegenerate(home, name, src); err != nil {
			return "", err
		}
		return i18n.T(lang, "tui.form.rule_added_profile", name), nil
	}
	return action{form: form, apply: apply}
}

// formSetOverlayEnv fija una variable del overlay. Con key != "" viene de
// `enter` sobre una fila y el nombre no se puede cambiar; con key == "" es un
// alta y se piden los dos campos.
func formSetOverlayEnv(home, name, key, val string, lang i18n.Lang) action {
	k, v := key, val
	fields := []huh.Field{}
	if key == "" {
		fields = append(fields, huh.NewInput().
			Title(i18n.T(lang, "tui.form.env_key")).
			Value(&k).
			Validate(func(s string) error {
				if strings.TrimSpace(s) == "" {
					return fmt.Errorf("%s", i18n.T(lang, "tui.form.env_key_empty"))
				}
				return nil
			}))
	}
	// TitleFunc, NO Title: huh.Input.Title es ESTÁTICO — se evalúa UNA vez, al
	// construir el form (field_input.go del módulo huh@v1.0.0 pinneado en
	// go.mod: "The Title is static for dynamic Title use TitleFunc"). Con
	// key=="" (alta), `k` todavía está vacío en el momento en que se
	// construye este segundo campo — Title(i18n.T(..., k)) horneaba
	// literalmente "Valor de " para siempre, sin importar lo que el usuario
	// tecleara después en el primer campo. TitleFunc sí se re-evalúa cuando
	// cambia el binding que se le pasa (`&k`).
	fields = append(fields, huh.NewInput().
		TitleFunc(func() string { return i18n.T(lang, "tui.form.env_val", k) }, &k).
		Value(&v))
	form := huh.NewForm(huh.NewGroup(fields...))
	apply := func() (string, error) {
		if err := core.OverlayEnvSet(home, name, k, v); err != nil {
			return "", err
		}
		return i18n.T(lang, "tui.form.env_saved", k, name), nil
	}
	return action{form: form, apply: apply}
}

// formAddHookToProfile añade un hook al perfil MIRADO — arma su propio
// InstructCtx con ActiveProfile = name en vez de usar el de la terminal
// (CCP_PROFILE), que sería el perfil equivocado. Mismo formato que `ccp
// instruct add profile hook '<id>={json}'` (internal/core/instruct_cmd.go:86-
// 108): el id es solo la referencia que queda en el manifiesto, el evento de
// verdad vive DENTRO del JSON (p. ej. {"hooks":{"PostToolUse":[{"matcher":"",
// "hooks":[{"type":"command","command":"..."}]}]}}). InstructAdd YA regenera
// el cc-home internamente para scope "profile" (instruct_cmd.go:102-107); no
// hay que llamar a CfgRegenerate aparte.
func formAddHookToProfile(home, src, name string, lang i18n.Lang) action {
	var id, snippet string
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title(i18n.T(lang, "tui.form.hook_id")).
				Value(&id).
				Validate(func(s string) error {
					if strings.TrimSpace(s) == "" {
						return fmt.Errorf("%s", i18n.T(lang, "tui.form.hook_id_empty"))
					}
					return nil
				}),
			huh.NewInput().
				Title(i18n.T(lang, "tui.form.hook_json")).
				Value(&snippet).
				Validate(func(s string) error {
					if core.CfgValidateBytes([]byte(s)) != nil {
						return fmt.Errorf("%s", i18n.T(lang, "tui.form.hook_json_invalid"))
					}
					return nil
				}),
		),
	)
	apply := func() (string, error) {
		ctx := core.InstructCtx{Home: home, Src: src, ActiveProfile: name}
		res, err := core.InstructAdd(ctx, "profile", "hook", id+"="+snippet)
		if err != nil {
			return "", err
		}
		return i18n.T(lang, "tui.form.hook_added", res.Name, name), nil
	}
	return action{form: form, apply: apply}
}
```

`core.ProfileInstrFile` no existe: `cfgInstrFile` es privada
(`internal/core/cfg.go:29`). Expónla en ese archivo igual que `ClaudeSrc`:

```go
// ProfileInstrFile expone la ruta del CLAUDE.md del overlay a los front-ends.
func ProfileInstrFile(home, name string) string { return cfgInstrFile(home, name) }
```

- [ ] **Step 7: Comprobar que el form vuelve a la vista y no al dashboard**

`exitForm` ya devuelve a `m.formBack`, y `enterForm` lo fija con `m.mode` al
abrir (`internal/tui/tui.go:344-350`). Como `m.mode` es `modeProfile` cuando se
pulsa `a`, esto sale correcto sin tocar nada. Añade además en `exitForm`, después
de `m.reload()`:

```go
	if m.mode == modeProfile {
		m.reloadProfileEff()
	}
```

Sin eso la vista sigue enseñando el Effective de antes de la escritura.

- [ ] **Step 8: Añadir las claves de i18n**

```go
	"tui.profview.no_file": {
		En: "this box has no editable file: sensors live in auto_handoff.hooks (press 'c')",
		Es: "esta caja no tiene archivo editable: los sensores viven en auto_handoff.hooks (pulsa 'c')",
	},
	"tui.profview.no_delete": {
		En: "hooks live in arrays with no stable id: ccp only removes them from its manifest, not from the JSON — edit the file with 'e'",
		Es: "los hooks viven en arrays sin id estable: ccp solo los saca de su manifiesto, no del JSON — edita el archivo con 'e'",
	},
	"tui.profview.global_row": {
		// Genérico a propósito: la misma clave la usan la fila global de
		// Instrucciones (~/.claude/CLAUDE.md) Y una fila OriginGlobal de Env
		// (~/.claude/settings.json) — las dos son "tu config global, no el
		// overlay de este perfil", solo cambia el archivo.
		En: "that is your global config, not this profile's overlay",
		Es: "eso es tu config global, no el overlay de este perfil",
	},
	"tui.profview.no_overlay_default": {
		En: "'default' = your GLOBAL config; edit it directly, it has no overlay",
		Es: "'default' = tu config GLOBAL; edítala directamente, no tiene overlay",
	},
	"tui.form.hook_id": {
		En: "Hook id (for the manifest, not the event name)",
		Es: "Id del hook (para el manifiesto, no el nombre del evento)",
	},
	"tui.form.hook_id_empty": {
		En: "write an id",
		Es: "escribe un id",
	},
	"tui.form.hook_json": {
		En: `JSON fragment, e.g. {"hooks":{"PostToolUse":[{"matcher":"","hooks":[{"type":"command","command":"..."}]}]}}`,
		Es: `Fragmento JSON, p. ej. {"hooks":{"PostToolUse":[{"matcher":"","hooks":[{"type":"command","command":"..."}]}]}}`,
	},
	"tui.form.hook_json_invalid": {
		En: "invalid JSON",
		Es: "JSON inválido",
	},
	"tui.form.hook_added": {
		En: "Hook '%s' added to '%s' (cc-home regenerated).",
		Es: "Hook '%s' añadido a '%s' (cc-home regenerado).",
	},
	"tui.profview.rule_removed": {
		En: "Rule removed from the overlay.",
		Es: "Regla borrada del overlay.",
	},
	"tui.profview.env_removed": {
		En: "Variable '%s' removed.",
		Es: "Variable '%s' borrada.",
	},
	"tui.form.rule_text": {
		En: "Instruction for this profile",
		Es: "Instrucción para este perfil",
	},
	"tui.form.rule_text_empty": {
		En: "write the instruction",
		Es: "escribe la instrucción",
	},
	"tui.form.rule_added_profile": {
		En: "Instruction added to '%s' (cc-home regenerated).",
		Es: "Instrucción añadida a '%s' (cc-home regenerado).",
	},
	"tui.form.rule_dup": {
		En: "That instruction was already in this profile (not duplicated).",
		Es: "Esa instrucción ya estaba en este perfil (no se duplica).",
	},
	"tui.form.env_key": {
		En: "Variable name",
		Es: "Nombre de la variable",
	},
	"tui.form.env_key_empty": {
		En: "write the variable name",
		Es: "escribe el nombre de la variable",
	},
	"tui.form.env_val": {
		En: "Value for %s",
		Es: "Valor de %s",
	},
	"tui.form.env_saved": {
		En: "%s saved in the overlay of '%s' (cc-home regenerated).",
		Es: "%s guardada en el overlay de '%s' (cc-home regenerado).",
	},
```

`tui.form.rule_dup` NO existe en ningún catálogo (`grep -rn
"tui.form.rule_dup" internal/core/i18n/` da cero resultados hoy — lo único
parecido es `cli.instruct.rule_dup`, con un `%s` de formato que espera el
archivo, y esta llamada no pasa ninguno). Está declarada más abajo, en el
Step 8, sin verbos de formato — no la reuses ni la des por existente.

- [ ] **Step 9: Correr los tests de escritura**

Run: `go test ./internal/tui/ -run 'TestAAbreUnForm|TestBorrarEnv|TestBorrarUnHook|TestBorrarLaFilaGlobal|TestETrasNavegar|TestEEnSensores' -v`
Expected: PASS los seis

- [ ] **Step 10: Correr todos los gates**

Run: `gofmt -l internal cmd && go vet ./... && go test ./... 2>&1 | tail -12`
Run: `go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2 run --timeout 5m`
Expected: `0 issues`.

- [ ] **Step 11: Commit (pedir autorización antes de ejecutarlo)**

```bash
git add internal/tui/profile_view.go internal/tui/profile_view_test.go \
        internal/tui/forms.go internal/tui/dashboard.go internal/tui/tui.go \
        internal/core/cfg.go internal/core/cfg_cmd.go internal/core/cfg_test.go \
        internal/core/i18n/catalog_tui.go
git commit -m "feat(tui): editar reglas y env del perfil desde su vista"
```

---

### Task 10: Documentación

**Files:**
- Modify: `CLAUDE.md` (sección `internal/cli & internal/tui`)
- Modify: `CHANGELOG.md` (sección `[Sin publicar]` nueva, encima de `[2.15.0]`)

**Interfaces:**
- Consumes: todo lo anterior.
- Produces: nada de código.

- [ ] **Step 1: Actualizar `CLAUDE.md`**

En la sección `### internal/cli & internal/tui`, después de la frase sobre la
TUI, añade:

```markdown
`internal/tui/shell.go` es el **único** sitio que pinta chrome: cabecera, cajas,
cursor, ventana, línea de estado y pie. Una vista construye un `viewSpec` (unos
`panelSpec` con sus `rowSpec`) y llama a `renderView`; alinear columnas sigue
siendo de la vista, pero el `▸`, el recorte y la caja no. Antes había tres
renderizadores a mano repitiendo la idea, que es el mismo bug que `traceMove`
evita en el supervisor. `panelSpec.MaxRows` es lo que permite pintar un
`permissions.allow` de 200 entradas sin viewport: una ventana alrededor del
cursor con marcas `↑ N más` / `↓ N más`.

`e` sobre un perfil abre la **vista de perfil** (`modeProfile`,
`profile_view.go`): tres cajas —Instrucciones · Env · Efectivo— sobre
`core.ProfileEffective`, que devuelve la procedencia como dato (`OriginGlobal` /
`OriginOverlay` / `OriginAuto`, más `Shadowed` cuando otra capa traía la misma
clave). Se edita lo que `core` ya sabe escribir: reglas por `InstructRuleAdd/Rm`,
variables por `OverlayEnvSet/Del`, hooks por `InstructAdd` (con el `ActiveProfile`
del `InstructCtx` fijado al perfil MIRADO, no al de la terminal). Los hooks se
añaden pero **no** se borran — viven en arrays sin id estable— y la tecla lo dice
en vez de fingir. El CLI no cambia: `ccp profile config <perfil>` sigue abriendo
el editor.
```

- [ ] **Step 2: Añadir la entrada de CHANGELOG**

**No hardcodees a qué versión está pegado esto** — este repo saca releases a
menudo (dos pasaron mientras se escribía este plan) y para cuando esta tarea se
ejecute de verdad, el tope del archivo puede ser otra. Mira el estado REAL de
`CHANGELOG.md` en ese momento:

- Si el tope YA es un `## [Unreleased]` (o `[Sin publicar]`) — lo normal, dado
  el ritmo del repo — **añade tu `### Added` a la lista que ya tiene** (como
  bullets nuevos al final de su sección `### Added`) y agrega una sección
  `### Changed` propia detrás si esa sección aún no existe. No toques el
  título de ese `## [Unreleased]` — es de la feature que llegó primero; la tuya
  se suma al mismo lote sin publicar, no lo reemplaza.
- Si el tope es directamente un `## [x.y.z]` con versión (no hay ningún
  `Unreleased` abierto), crea uno nuevo encima con tu propio título.

El contenido, en cualquiera de los dos casos:

```markdown
### Added

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
```

- [ ] **Step 3: Verificar que la doc no miente**

Run: `grep -n "modeProfile\|shell.go\|ProfileEffective" CLAUDE.md`
Expected: las tres aparecen, y los nombres coinciden **exactamente** con los del
código (`grep -rn "func ProfileEffective\|modeProfile" internal/`).

- [ ] **Step 4: Commit (pedir autorización antes de ejecutarlo)**

```bash
git add CLAUDE.md CHANGELOG.md
git commit -m "docs: vista de perfil y shell de vista compartido"
```

---

## Orden y dependencias

```
Task 1 (goldens)  ────────────┬─────────────────────────┐
Task 2 (shell)    ────────────┼──► Task 3 (ventana)      │
                  └───────────┴──► Task 4 (retrofit dashboard) ─┐
                  └───────────┴──► Task 5 (retrofit Config)    ─┤
Task 6 (ProfileEffective) ──────────────────────────────────────┤
                                                                 ├──► Task 8 (vista) ──► Task 9 (escritura) ──► Task 10 (docs)
Task 7 (OverlayEnv) ─────────────────────────────────────────────┘
```

Lo que cada flecha significa, en prosa (el ASCII de arriba pierde matices):

- **Task 4 y Task 5 necesitan Task 1 Y Task 2, las dos** — no solo Task 1.
  Task 1 es la red de seguridad (los goldens), pero el código de ambas llama a
  `m.renderView`/usa `panelSpec`/`rowSpec`, que Task 2 define: sin
  `internal/tui/shell.go`, ninguna de las dos compila.
- **Task 3 no bloquea a nadie más que a sí misma.** Solo añade tests de borde
  a `windowRows` (ya implementada en la Task 2); ninguna tarea posterior
  depende de que Task 3 exista para compilar o pasar sus propios tests.
- **Task 8 necesita Task 2 (el shell) y Task 6 (el core), pero NO Task 7.**
  El fixture de sus tests siembra el overlay escribiendo el archivo a mano
  (`os.WriteFile` + `CfgInitOverlay`), no llamando a `core.OverlayEnvSet` —
  deliberado, para no forzar un orden que el resto de la tarea no necesita.
- **Task 9 sí necesita Task 7**, además de Task 8: sus propios tests de
  escritura llaman a `core.OverlayEnvDel` de verdad.
- **Task 6 y Task 7 son de `internal/core`, sin ninguna dependencia de TUI** —
  pueden hacerse en paralelo con las Tareas 1-5.

Las tareas 6 y 7 son de `internal/core` y no dependen de nada de la TUI: se
pueden hacer en paralelo con la 1-5. La 8 necesita el shell (2-3) y el core (6);
la 9 necesita la 7 y la 8.

**La tarea 1 va primero, siempre.** Es la única que hace demostrable que el
retrofit no cambió la salida, y capturada después del refactor no vale nada.
