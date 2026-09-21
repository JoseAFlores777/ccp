package agent

import (
	"errors"
	"testing"
)

// loopNotes se prueba POR NOMBRE, como el armado del temporizador de vuelta en
// el supervisor, porque equivocarse aquí no se ve mirando correr el bucle: el
// síntoma tarda horas en aparecer y es o una pantalla con la misma línea cada
// cinco minutos, o un corte que se contó una vez y del que nadie supo nunca
// cuándo se arregló.
func TestLoopNotesCuentaUnaVezYAvisaAlVolver(t *testing.T) {
	caido := errors.New("dial tcp: connection refused")
	otro := errors.New("la nube respondió 503: el almacenamiento no responde")
	var n loopNotes

	if rep, rec := n.step(caido); !rep || rec {
		t.Fatalf("el primer fallo = contar %v, vuelta %v", rep, rec)
	}
	// El mismo fallo, cinco minutos después, y otra vez, y otra: ya se dijo.
	for i := range 3 {
		if rep, rec := n.step(caido); rep || rec {
			t.Fatalf("repetición %d = contar %v, vuelta %v", i, rep, rec)
		}
	}
	// Un fallo DISTINTO sí se cuenta: es información nueva sobre qué pasa.
	if rep, rec := n.step(otro); !rep || rec {
		t.Fatalf("un fallo distinto = contar %v, vuelta %v", rep, rec)
	}
	// Y la vuelta se avisa una vez, no en cada pasada buena de después.
	if rep, rec := n.step(nil); rep || !rec {
		t.Fatalf("la vuelta = contar %v, vuelta %v", rep, rec)
	}
	if rep, rec := n.step(nil); rep || rec {
		t.Fatalf("una pasada buena más = contar %v, vuelta %v", rep, rec)
	}
	// Y si vuelve a caerse, se cuenta otra vez aunque sea el mismo error de
	// antes: entre medias hubo una pasada buena, así que es un corte nuevo.
	if rep, rec := n.step(caido); !rep || rec {
		t.Fatalf("el corte siguiente = contar %v, vuelta %v", rep, rec)
	}
}
