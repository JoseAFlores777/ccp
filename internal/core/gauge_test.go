package core

import (
	"math"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

// gauge_test.go — las reglas del medidor y de la cuenta atrás, fijadas por
// nombre. El consumidor de estas primitivas es `ccp _statusline`, que tiene
// prohibido fallar, así que la mitad de los casos de aquí son entradas hostiles:
// lo que se pinnea no es solo el formato bonito, es que ninguna entrada rara
// llegue a un panic.

func TestRenderGaugeExtremos(t *testing.T) {
	casos := []struct {
		nombre string
		pct    float64
		cells  int
		quiere string
	}{
		// Los dos extremos legítimos.
		{"cero por ciento", 0, 10, "▏░░░░░░░░░░▏"},
		{"cien por ciento", 100, 10, "▏██████████▏"},

		// Fuera de banda: se acota, nunca se extrapola. Un -5 % no vacía «más
		// que vacío» y un 300 % no desborda el medidor.
		{"negativo se acota a vacio", -5, 4, "▏░░░░▏"},
		{"mayor que cien se acota a lleno", 300, 4, "▏████▏"},

		// No-medidas. NaN falla toda comparación, así que si el clamp fuera lo
		// único que hubiera sobreviviría hasta math.Round; se trata como 0
		// porque un medidor lleno mentiría más que uno vacío.
		{"NaN se lee como cero", math.NaN(), 4, "▏░░░░▏"},
		{"Inf positivo se lee como cero", math.Inf(1), 4, "▏░░░░▏"},
		{"Inf negativo se lee como cero", math.Inf(-1), 4, "▏░░░░▏"},

		// Sin celdas no hay medidor: ni siquiera los delimitadores, porque un
		// "▏▏" suelto no dice nada y ocupa. Que el front-end decida el relleno.
		{"cero celdas devuelve vacio", 50, 0, ""},
		{"celdas negativas devuelve vacio", 50, -3, ""},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			if got := RenderGauge(c.pct, c.cells); got != c.quiere {
				t.Errorf("RenderGauge(%v, %d) = %q, quiere %q", c.pct, c.cells, got, c.quiere)
			}
		})
	}

	// cells absurdo: se acota a maxGaugeCells en vez de construir un string de
	// megabytes. `cells` sale de aritmética sobre COLUMNS, o sea de fuera.
	t.Run("celdas enormes se acotan", func(t *testing.T) {
		got := RenderGauge(100, 1_000_000)
		// 2 delimitadores + maxGaugeCells celdas.
		if n := utf8.RuneCountInString(got); n != maxGaugeCells+2 {
			t.Fatalf("runas = %d, quiere %d", n, maxGaugeCells+2)
		}
		if strings.Count(got, gaugeFull) != maxGaugeCells {
			t.Errorf("celdas llenas = %d, quiere %d", strings.Count(got, gaugeFull), maxGaugeCells)
		}
	})
}

func TestRenderGaugeProporcion(t *testing.T) {
	casos := []struct {
		nombre string
		pct    float64
		cells  int
		llenas int
	}{
		// Redondeo al más cercano, no truncado: 59 % de 10 celdas es «más de la
		// mitad» y tiene que verse como 6, no como 5.
		{"59 de 10", 59, 10, 6},
		{"50 de 10", 50, 10, 5},
		{"87.6 de 10", 87.6, 10, 9},
		{"medio hacia arriba", 5, 10, 1}, // 0.5 → 1 (math.Round redondea al alza)
		{"59 de 4", 59, 4, 2},
		{"88 de 4", 88, 4, 4},
		{"33 de 3", 33, 3, 1},
		{"100 de 1", 100, 1, 1},

		// Suelo de una celda: por debajo de media celda la proporción diría 0, y
		// entonces «no has gastado nada» y «has empezado a gastar» se verían
		// idénticos — justo la distinción que el medidor existe para dar de un
		// vistazo. La precisión no se pierde: el número exacto va al lado.
		{"2 de 10 no llega a media celda pero se ve", 2, 10, 1},
		{"10 de 4 no llega a media celda pero se ve", 10, 4, 1},
		{"0.0001 sigue siendo consumo", 0.0001, 10, 1},
		// Con una sola celda el medidor es binario hay/no hay. Es lo único que
		// una celda puede decir con honestidad.
		{"1 de 1 llena la unica celda", 1, 1, 1},
		// Y el suelo NO inventa consumo donde no lo hay: 0 sigue siendo 0.
		{"0 de 10 sigue vacio", 0, 10, 0},
		{"negativo sigue vacio", -5, 10, 0},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			got := RenderGauge(c.pct, c.cells)
			// El medidor SIEMPRE mide lo pedido: cells celdas entre dos
			// delimitadores. Es lo que permite al front-end presupuestar ancho.
			if n := utf8.RuneCountInString(got); n != c.cells+2 {
				t.Fatalf("RenderGauge(%v, %d) = %q: runas = %d, quiere %d", c.pct, c.cells, got, n, c.cells+2)
			}
			if !strings.HasPrefix(got, gaugeEdge) || !strings.HasSuffix(got, gaugeEdge) {
				t.Fatalf("RenderGauge(%v, %d) = %q: faltan delimitadores", c.pct, c.cells, got)
			}
			if n := strings.Count(got, gaugeFull); n != c.llenas {
				t.Errorf("RenderGauge(%v, %d) = %q: llenas = %d, quiere %d", c.pct, c.cells, got, n, c.llenas)
			}
			if n := strings.Count(got, gaugeEmpty); n != c.cells-c.llenas {
				t.Errorf("RenderGauge(%v, %d) = %q: vacias = %d, quiere %d", c.pct, c.cells, got, n, c.cells-c.llenas)
			}
		})
	}
}

// TestClampPct fija el saneado que comparten el medidor y el número.
//
// Está exportado y probado por separado porque el bug que arregla fue
// exactamente que solo lo aplicaba UNA de las dos superficies: el medidor
// acotaba y el porcentaje de al lado se imprimía crudo, así que una muestra con
// `used_percentage: 9e99` sacaba un medidor lleno junto a 300 dígitos.
func TestClampPct(t *testing.T) {
	casos := []struct {
		nombre string
		pct    float64
		quiere float64
	}{
		{"dentro de banda se respeta", 59.4, 59.4},
		{"cero", 0, 0},
		{"cien", 100, 100},
		{"negativo se acota", -40, 0},
		{"mayor que cien se acota", 300, 100},
		{"absurdo se acota", 9e99, 100},
		// No-medidas: 0 porque un medidor lleno mentiría más que uno vacío.
		{"NaN", math.NaN(), 0},
		{"Inf positivo", math.Inf(1), 0},
		{"Inf negativo", math.Inf(-1), 0},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			if got := ClampPct(c.pct); got != c.quiere {
				t.Errorf("ClampPct(%v) = %v, quiere %v", c.pct, got, c.quiere)
			}
		})
	}
}

func TestHumanUntilOmiteVencido(t *testing.T) {
	// Un resets_at vencido o a cero no se pinta. Pintar "·0m" diría «ya
	// reabrió», que es justo lo contrario de lo que sabemos cuando el dato es
	// viejo o falta (misma filosofía que Windowed.HasData).
	casos := []struct {
		nombre string
		d      time.Duration
	}{
		{"cero", 0},
		{"negativo por un segundo", -time.Second},
		{"negativo por horas", -3 * time.Hour},
		{"minimo representable", time.Duration(math.MinInt64)},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			if got := HumanUntil(c.d); got != "" {
				t.Errorf("HumanUntil(%v) = %q, quiere \"\"", c.d, got)
			}
		})
	}
}

func TestHumanUntilFormatos(t *testing.T) {
	casos := []struct {
		nombre string
		d      time.Duration
		quiere string
	}{
		// < 1h: minutos a secas, sin ceros a la izquierda.
		{"menos de una hora", 47 * time.Minute, "47m"},
		{"un minuto justo", time.Minute, "1m"},
		{"segundos sueltos siguen siendo un minuto", 30 * time.Second, "1m"},
		{"casi una hora", 59*time.Minute + 59*time.Second, "59m"},

		// < 24h: horas y minutos pegados, que es lo que cabe en la barra.
		{"una hora justa", time.Hour, "1h"},
		{"horas y minutos", 2*time.Hour + 14*time.Minute, "2h14m"},
		{"minutos en cero se omiten", 5 * time.Hour, "5h"},
		{"los segundos no ascienden a minuto", 2*time.Hour + 59*time.Second, "2h"},
		{"casi un dia", 23*time.Hour + 59*time.Minute, "23h59m"},

		// >= 24h: días hacia ARRIBA. Un día exacto no asciende (no hay resto),
		// pero cualquier resto sí: lo único que no puede pasar es que la barra
		// diga que la cuota vuelve antes de lo que sabemos.
		{"un dia justo", 24 * time.Hour, "1d"},
		{"siete dias justos", 7 * 24 * time.Hour, "7d"},
		{"un resto minimo ya asciende", 24*time.Hour + time.Second, "2d"},
		{"tres dias largos", 3*24*time.Hour + 20*time.Hour, "4d"},
		// El caso que motivó el cambio: truncando salía "2d" y el usuario
		// volvía casi un día antes de que el perfil se liberara.
		{"casi tres dias", 2*24*time.Hour + 23*time.Hour + 59*time.Minute, "3d"},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			if got := HumanUntil(c.d); got != c.quiere {
				t.Errorf("HumanUntil(%v) = %q, quiere %q", c.d, got, c.quiere)
			}
		})
	}
}

func TestHumanUntilAtIgnoraInstantesAusentes(t *testing.T) {
	now := time.Date(2026, 7, 27, 10, 0, 0, 0, time.UTC)
	casos := []struct {
		nombre   string
		resetsAt time.Time
		now      time.Time
		quiere   string
	}{
		// El guard que existe este helper para no repetir: sin resets_at no hay
		// cuenta atrás. Restar del cero de time daría siglos negativos, y aun
		// invertido sería una cifra inventada.
		{"resets_at a cero", time.Time{}, now, ""},
		{"now a cero", now.Add(time.Hour), time.Time{}, ""},
		{"resets_at pasado", now.Add(-time.Hour), now, ""},
		{"resets_at futuro", now.Add(2*time.Hour + 14*time.Minute), now, "2h14m"},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			if got := HumanUntilAt(c.resetsAt, c.now); got != c.quiere {
				t.Errorf("HumanUntilAt(%v, %v) = %q, quiere %q", c.resetsAt, c.now, got, c.quiere)
			}
		})
	}
}
