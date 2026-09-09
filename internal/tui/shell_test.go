package tui

import (
	"fmt"
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
	out := m.rowLine(rowSpec{Text: preEstilizado}, false, true)
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
	out := m.rowLine(rowSpec{Text: "algo", Marker: "✓ logueado"}, false, true)
	if !strings.Contains(out, "\x1b[") {
		t.Fatalf("una marca que empieza con ✓ tiene que salir coloreada: %q", out)
	}
	if !strings.Contains(out, "✓ logueado") {
		t.Fatalf("la marca completa tiene que aparecer: %q", out)
	}
}

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
