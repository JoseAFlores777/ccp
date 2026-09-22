package core

import (
	"testing"
	"time"
)

// TestRateSampleVaciaSeGuardaYNoCuentaComoDato fija las dos mitades del arreglo,
// que tiran en direcciones opuestas y por eso hay que afirmarlas juntas.
//
// Se GUARDA: antes el sensor solo escribía cuando encontraba consumo, así que un
// Claude Code que no lo informa —el payload de 2.1.236 no trae `rate_limits`—
// dejaba el directorio sin crear y la pantalla decía «todavía no hay muestras»,
// la misma frase que cuando el sensor no está instalado. Dos causas, un síntoma
// mudo, y la de verdad invisible.
//
// Y NO cuenta como dato: ReadRateLimits tiene que seguir diciendo que no hay
// nada utilizable, porque devolver la muestra vacía como buena le daría al
// supervisor unos ceros que nadie midió — y con ellos decidiría que la cuenta
// está libre justo antes de que un límite la corte.
func TestRateSampleVaciaSeGuardaYNoCuentaComoDato(t *testing.T) {
	home := t.TempDir()
	now := time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)

	if err := WriteRateSample(home, "a-cc", RateLimits{}, false, "2.1.236", now); err != nil {
		t.Fatalf("WriteRateSample: %v", err)
	}

	st := ReadRateSample(home, "a-cc")
	if !st.Present {
		t.Fatal("la muestra vacía tiene que quedar en disco: es la constancia de que el sensor corrió")
	}
	if st.Reported {
		t.Error("una muestra sin consumo no puede decir que informó")
	}
	if st.CCVersion != "2.1.236" {
		t.Errorf("CCVersion = %q; el aviso necesita la versión para decir cuál no informa", st.CCVersion)
	}

	if _, _, ok := ReadRateLimits(home, "a-cc"); ok {
		t.Error("ReadRateLimits no puede dar por bueno un consumo que nadie midió")
	}
}

// TestRateSampleConConsumoSigueSiendoDato: el camino normal no cambia.
func TestRateSampleConConsumoSigueSiendoDato(t *testing.T) {
	home := t.TempDir()
	rl := RateLimits{FiveHour: Windowed{UsedPercentage: 42.5}}
	if err := WriteRateLimits(home, "a-cc", rl, time.Now()); err != nil {
		t.Fatal(err)
	}
	got, _, ok := ReadRateLimits(home, "a-cc")
	if !ok || got.FiveHour.UsedPercentage != 42.5 {
		t.Fatalf("ReadRateLimits = %+v ok=%v; quería el 42.5", got, ok)
	}
	if st := ReadRateSample(home, "a-cc"); !st.Present || !st.Reported {
		t.Fatalf("ReadRateSample = %+v; quería presente y con informe", st)
	}
}

// TestRateSampleSinMuestra: no haber corrido nunca sigue siendo su propio estado.
func TestRateSampleSinMuestra(t *testing.T) {
	if st := ReadRateSample(t.TempDir(), "a-cc"); st.Present {
		t.Fatalf("sin archivo no puede haber muestra: %+v", st)
	}
}
