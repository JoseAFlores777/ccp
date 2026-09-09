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
	Title string
	Hint  string // línea de teclas; solo se pinta con foco
	Rows  []rowSpec
	// Cursor es la fila bajo el cursor. -1 = ninguna fila seleccionable (p. ej.
	// un panel de solo lectura sin filas navegables) — y además omite del todo
	// la columna que reserva espacio para el cursor (rowLine la recibe como
	// hasCursor := Cursor >= 0), así que un panel como Estado sale con el
	// mismo margen izquierdo que tenía antes de pasar por este shell. OJO: el
	// valor cero de Go es 0, NO -1 — un panel sin cursor que olvide fijar este
	// campo a -1 pinta un cursor fantasma en su primera fila. Todo panelSpec
	// que no tenga concepto de "fila seleccionada" TIENE que fijarlo
	// explícitamente.
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

// rowLine pinta UNA fila: antepone el cursor (si el panel tiene concepto de
// fila seleccionable) y, si hay Marker, lo colorea — y nada más. NO trunca ni
// reestiliza r.Text (ver el contrato de rowSpec.Text explicado arriba): eso ya
// lo hizo la vista, con su propio ancho.
//
// hasCursor=false omite la columna del cursor por completo, en vez de dejarla
// reservada en blanco: un panel con Cursor: -1 (p. ej. Estado, cuatro líneas
// kv sin fila seleccionable) no reservaba esa columna en el render de antes de
// este shell, y windowRows pasa hasCursor := p.Cursor >= 0 para preservarlo —
// sin esto, cualquier panel de solo lectura sale con dos espacios de más al
// principio de cada fila frente a su golden.
func (m *model) rowLine(r rowSpec, sel bool, hasCursor bool) string {
	cur := ""
	if hasCursor {
		cur = "  "
		if sel {
			cur = styleFocused.Render("▸ ")
		}
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
		out = append(out, m.rowLine(p.Rows[i], p.Focused && i == p.Cursor, p.Cursor >= 0))
	}
	if end < len(p.Rows) {
		out = append(out, styleDim.Render(i18n.T(m.lang, "tui.shell.more_down", len(p.Rows)-end)))
	}
	return out
}
