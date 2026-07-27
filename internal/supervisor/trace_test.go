package supervisor

import (
	"context"
	"regexp"
	"strings"
	"testing"
)

// trace_test.go — el TEXTO de la traza es contrato.
//
// No es adorno: es lo que queda en el log del cron de las 3am y lo único con lo
// que el usuario responde a la mañana siguiente «¿por dónde pasó mi sesión y
// cuánto presupuesto me queda?». Un contador que se llama de dos maneras, o que
// muestra el mismo número en dos movimientos seguidos sin decir por qué, no es
// un problema estético: hace la respuesta imposible.

// Una corrida ida → vuelta → ida, leída como la leería el usuario.
//
// El fallo que cubre: tras el cambio de semántica de `max_hops` (la vuelta a
// casa NO consume presupuesto, ver policy.go) la traza quedó con dos
// vocabularios para el mismo suceso. La vuelta a casa POR LÍMITE salía por
// traceHop como «(vuelta a casa, salto N/M)» con N ya sin incluir ese
// movimiento, y la vuelta POR TEMPORIZADOR salía como «(vuelta a casa,
// préstamos N/M)». Resultado real: dos movimientos consecutivos imprimiendo
// ambos «salto 1/4» —sin una palabra sobre por qué el número no subía— y la ida
// siguiente llamándose «préstamo» con el mismo formato que el regreso.
//
// Las tres propiedades que se exigen aquí son las del arreglo:
//   - un solo nombre para el contador («préstamo(s)»; «salto» ya no existe);
//   - la vuelta a casa no se presenta nunca como un préstamo;
//   - y cuando el número se repite, la línea DICE por qué.
func TestTrazaIdaYVueltaYVueltaAIrHablaUnSoloIdioma(t *testing.T) {
	e := setup(t, seed{
		fallback: []string{"p2"},
		maxHops:  4,
		// Cooldown de 0s: cada perfil vuelve a estar disponible en cuanto se marca
		// agotado. Es lo que hace determinista el ida-vuelta-ida sin tocar relojes:
		// el límite de p2 encuentra al primario ya libre (⇒ vuelta a casa) y el
		// límite siguiente de p1 encuentra a p2 libre (⇒ segundo préstamo).
		cooldown: "0s",
		plan: []runStep{
			{exit: 1, where: "stdout", emit: limitStdout("p1")},          // ida
			{exit: 1, where: "stdout", emit: limitStdout("p2")},          // vuelta
			{exit: 1, where: "stdout", emit: limitStdout("p1-otra-vez")}, // ida otra vez
			{exit: 0}, // termina en p2 y se devuelve
		},
	})

	res, err := Run(context.Background(), e.opts(seedSession))
	if err != nil {
		t.Fatalf("Run: %v (out=%q err=%q)", err, e.out.String(), e.errb.String())
	}
	if res.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, quería 0 (out=%q)", res.ExitCode, e.out.String())
	}
	if got := e.profiles(t); len(got) != 4 {
		t.Fatalf("perfiles lanzados = %v, quería cuatro lanzamientos (p1 p2 p1 p2)", got)
	}

	trace := e.out.String()
	moves := movementLines(trace)
	if len(moves) != 3 {
		t.Fatalf("movimientos en la traza = %d, quería 3 (ida, vuelta, ida):\n%s", len(moves), trace)
	}

	// El texto exacto, que es lo que el usuario lee. Va por sufijo porque el
	// prefijo (perfil, permanencia, motivo) varía con el reloj real.
	quiere := []string{
		"handoff a p2 (préstamo 1/4)",
		"volviendo a p1 (vuelta a casa, no gasta préstamo: siguen 1/4)",
		"handoff a p2 (préstamo 2/4)",
	}
	for i, want := range quiere {
		if !strings.HasSuffix(moves[i], want) {
			t.Errorf("movimiento %d:\n  got  %s\n  want …%s", i+1, moves[i], want)
		}
	}

	// (a) Un solo vocabulario: «salto» era el otro nombre del mismo contador.
	if strings.Contains(trace, "salto") {
		t.Errorf("la traza sigue usando dos nombres para el mismo contador:\n%s", trace)
	}

	// (b) La vuelta a casa no es un préstamo: es su cierre. Presentarla con el
	// mismo verbo mandaría al usuario a buscar su conversación en la cuenta ajena.
	if strings.Contains(moves[1], "handoff a") {
		t.Errorf("la vuelta a casa se anuncia como un préstamo más: %s", moves[1])
	}

	// (c) La propiedad de fondo, escrita como propiedad y no como literal: dos
	// movimientos consecutivos con el MISMO número solo son legibles si la línea
	// explica por qué no subió.
	for i := 1; i < len(moves); i++ {
		if budgetOf(moves[i]) != budgetOf(moves[i-1]) {
			continue
		}
		if !strings.Contains(moves[i], "no gasta préstamo") {
			t.Errorf("dos movimientos seguidos muestran %s sin explicar por qué no subió:\n  %s\n  %s",
				budgetOf(moves[i]), moves[i-1], moves[i])
		}
	}
}

// movementLines se queda con las líneas de la traza que anuncian un MOVIMIENTO
// de la conversación. Se filtra por el verbo y no por la flecha porque el
// desenlace («p1 ──[termina]──→ ✅ exit 0») usa la misma flecha y no mueve nada.
func movementLines(trace string) []string {
	var out []string
	for _, ln := range strings.Split(strings.TrimSpace(trace), "\n") {
		if strings.Contains(ln, "handoff a ") || strings.Contains(ln, "volviendo a ") {
			out = append(out, ln)
		}
	}
	return out
}

var budgetRe = regexp.MustCompile(`\d+/\d+`)

// budgetOf extrae el «N/M» de una línea de movimiento ("" si no lo lleva, que
// también es un fallo: todo movimiento debe decir cómo queda el presupuesto).
func budgetOf(line string) string { return budgetRe.FindString(line) }
