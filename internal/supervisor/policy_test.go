package supervisor

import (
	"testing"
	"time"

	"github.com/JoseAFlores777/ccp/internal/core"
)

// policy_test.go — la rotación se prueba con tiempos SINTÉTICOS: t0 fijo y
// desplazamientos. Chain no lee el reloj, así que un escenario que en
// producción dura seis horas aquí corre en microsegundos y sin sleeps.

var t0 = time.Date(2026, 7, 25, 10, 0, 0, 0, time.UTC)

// action es lo que hace cada paso del guion. Un escenario es una secuencia de
// pasos sobre la MISMA cadena: la rotación es un estado que evoluciona, y
// probar llamadas sueltas no vería los bugs de transición.
type action int

const (
	actNext    action = iota // pide destino y lo compara
	actMark                  // marca un perfil agotado
	actAdvance               // registra un salto ya ejecutado
	actDwell                 // consulta DwellSatisfied
	actHops                  // comprueba el contador de saltos
	actCurrent               // comprueba el perfil activo
)

type step struct {
	do action
	at time.Duration // desplazamiento desde t0

	profile string        // actMark / actAdvance
	resets  time.Duration // actMark: desplazamiento del resets_at; 0 => desconocido

	want     string // actNext / actCurrent
	wantOK   bool   // actNext
	wantBool bool   // actDwell
	wantInt  int    // actHops
}

// pol arma una EffectivePolicy pasando por Effective para heredar exactamente
// los mismos defaults y validaciones que el yaml real.
func pol(t *testing.T, p core.AutoPolicy) core.EffectivePolicy {
	t.Helper()
	eff, err := p.Effective("test")
	if err != nil {
		t.Fatalf("Effective: %v", err)
	}
	return eff
}

func TestChainRotation(t *testing.T) {
	// Política base de los escenarios: cooldown fijo de 1h y sin permanencia
	// mínima, para que cada escenario controle solo la variable que estudia.
	base := core.AutoPolicy{
		Fallback: []string{"app", "deep"},
		MinDwell: "0s",
		Cooldown: core.AutoCooldown{Strategy: core.CooldownFixed, Fallback: "1h"},
	}

	cases := []struct {
		name     string
		policy   core.AutoPolicy
		primary  string
		fallback []string
		maxHops  int
		noReturn bool
		steps    []step
	}{
		{
			// El primer agotamiento presta la sesión al fallback[0]: el orden
			// del yaml ES la preferencia del usuario.
			name:     "primer agotamiento salta al primer fallback",
			policy:   base,
			primary:  "personal",
			fallback: []string{"app", "deep"},
			steps: []step{
				{do: actCurrent, want: "personal"},
				{do: actMark, profile: "personal", at: time.Minute},
				{do: actNext, at: time.Minute, want: "app", wantOK: true},
				{do: actAdvance, profile: "app", at: time.Minute},
				{do: actCurrent, want: "app"},
				{do: actHops, wantInt: 1},
			},
		},
		{
			// Segundo agotamiento con el primario aún en cooldown: baja al
			// siguiente préstamo, no vuelve a casa antes de tiempo.
			name:     "segundo agotamiento salta al segundo fallback",
			policy:   base,
			primary:  "personal",
			fallback: []string{"app", "deep"},
			steps: []step{
				{do: actMark, profile: "personal", at: time.Minute},
				{do: actNext, at: time.Minute, want: "app", wantOK: true},
				{do: actAdvance, profile: "app", at: time.Minute},
				{do: actMark, profile: "app", at: 30 * time.Minute},
				{do: actNext, at: 30 * time.Minute, want: "deep", wantOK: true},
				{do: actAdvance, profile: "deep", at: 30 * time.Minute},
				{do: actCurrent, want: "deep"},
				{do: actHops, wantInt: 2},
			},
		},
		{
			// El péndulo: con el primario ya libre se vuelve a casa AUNQUE
			// quede "deep" fresco. Volver siempre gana sobre seguir prestando.
			name:     "con el primario libre vuelve al primario aunque queden fallbacks",
			policy:   base,
			primary:  "personal",
			fallback: []string{"app", "deep"},
			steps: []step{
				{do: actMark, profile: "personal", at: 0},
				{do: actNext, at: 0, want: "app", wantOK: true},
				{do: actAdvance, profile: "app", at: 0},
				{do: actMark, profile: "app", at: 90 * time.Minute},
				// A los 90m el cooldown de 1h del primario ya expiró.
				{do: actNext, at: 90 * time.Minute, want: "personal", wantOK: true},
				{do: actAdvance, profile: "personal", at: 90 * time.Minute},
				{do: actCurrent, want: "personal"},
			},
		},
		{
			// noReturn apaga el regreso: la cadena solo avanza por la lista de
			// préstamos aunque el primario esté disponible desde hace rato.
			name:     "noReturn nunca vuelve al primario",
			policy:   base,
			primary:  "personal",
			fallback: []string{"app", "deep"},
			noReturn: true,
			steps: []step{
				{do: actMark, profile: "personal", at: 0},
				{do: actNext, at: 0, want: "app", wantOK: true},
				{do: actAdvance, profile: "app", at: 0},
				{do: actMark, profile: "app", at: 90 * time.Minute},
				{do: actNext, at: 90 * time.Minute, want: "deep", wantOK: true},
				{do: actAdvance, profile: "deep", at: 90 * time.Minute},
				// "deep" agotado y el primario libre: con noReturn se acabó.
				{do: actMark, profile: "deep", at: 100 * time.Minute},
				{do: actNext, at: 100 * time.Minute, want: "", wantOK: false},
			},
		},
		{
			// max_hops es un tope duro sobre los PRÉSTAMOS: con presupuesto 1 y
			// el primario todavía en cooldown, no hay segundo salto aunque
			// "deep" esté fresco.
			name:     "max_hops corta la rotación",
			policy:   base,
			primary:  "personal",
			fallback: []string{"app", "deep"},
			maxHops:  1,
			steps: []step{
				{do: actMark, profile: "personal", at: 0},
				{do: actNext, at: 0, want: "app", wantOK: true},
				{do: actAdvance, profile: "app", at: 0},
				{do: actMark, profile: "app", at: 30 * time.Minute},
				// El cooldown de 1h del primario aún no venció: nada disponible.
				{do: actNext, at: 30 * time.Minute, want: "", wantOK: false},
				{do: actHops, wantInt: 1},
			},
		},
		{
			// …pero el tope NO encierra la conversación en la cuenta ajena: con
			// el presupuesto consumido y el primario ya libre, la vuelta a casa
			// sigue siendo posible y NO cuesta un préstamo. Es la contrapartida
			// de que max_hops cuente préstamos y no movimientos.
			name:     "max_hops agotado no impide volver a casa",
			policy:   base,
			primary:  "personal",
			fallback: []string{"app", "deep"},
			maxHops:  1,
			steps: []step{
				{do: actMark, profile: "personal", at: 0},
				{do: actNext, at: 0, want: "app", wantOK: true},
				{do: actAdvance, profile: "app", at: 0},
				{do: actHops, wantInt: 1},
				// A los 90m el cooldown de 1h del primario ya expiró.
				{do: actNext, at: 90 * time.Minute, want: "personal", wantOK: true},
				{do: actAdvance, profile: "personal", at: 90 * time.Minute},
				{do: actCurrent, want: "personal"},
				{do: actHops, wantInt: 1}, // la vuelta no consumió presupuesto
				// Y con el presupuesto ya gastado, prestar otra vez sí está
				// prohibido: el backstop sigue en pie.
				{do: actMark, profile: "personal", at: 100 * time.Minute},
				{do: actNext, at: 100 * time.Minute, want: "", wantOK: false},
			},
		},
		{
			// El maxHops del parámetro solo manda si es > 0; en cero se hereda
			// el de la política (así el flag --max-hops se pasa tal cual).
			name: "maxHops cero hereda el de la política",
			policy: core.AutoPolicy{
				Fallback: []string{"app", "deep"},
				MinDwell: "0s",
				MaxHops:  1,
				Cooldown: core.AutoCooldown{Strategy: core.CooldownFixed, Fallback: "1h"},
			},
			primary:  "personal",
			fallback: []string{"app", "deep"},
			maxHops:  0,
			steps: []step{
				{do: actMark, profile: "personal", at: 0},
				{do: actNext, at: 0, want: "app", wantOK: true},
				{do: actAdvance, profile: "app", at: 0},
				{do: actMark, profile: "app", at: time.Minute},
				{do: actNext, at: time.Minute, want: "", wantOK: false},
			},
		},
		{
			// Nunca se propone el perfil actual: "deep" está fresco pero es
			// donde ya estamos, así que el único destino posible es "app".
			name:     "nunca devuelve el perfil actual",
			policy:   base,
			primary:  "personal",
			fallback: []string{"app", "deep"},
			noReturn: true,
			steps: []step{
				{do: actMark, profile: "personal", at: 0},
				{do: actNext, at: 0, want: "app", wantOK: true},
				{do: actAdvance, profile: "deep", at: 0}, // salto forzado a deep
				{do: actMark, profile: "deep", at: time.Minute},
				{do: actNext, at: time.Minute, want: "app", wantOK: true},
			},
		},
		{
			// El primario colado dentro de fallback se ignora: NewChain lo
			// filtra para no proponer un salto a donde ya estamos.
			name:     "el primario dentro del fallback se ignora",
			policy:   base,
			primary:  "personal",
			fallback: []string{"personal", "app"},
			steps: []step{
				{do: actMark, profile: "personal", at: 0},
				{do: actNext, at: 0, want: "app", wantOK: true},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rc := core.ResolvedChain{
				Policy:   pol(t, tc.policy),
				Primary:  tc.primary,
				Fallback: tc.fallback,
			}
			c := NewChain(rc, tc.maxHops, tc.noReturn, t0)
			runSteps(t, c, tc.steps)
		})
	}
}

// runSteps ejecuta el guion contra la cadena, fallando con el índice del paso:
// en un escenario de seis pasos, "want app got deep" sin coordenadas obliga a
// contar a mano cuál falló.
func runSteps(t *testing.T, c *Chain, steps []step) {
	t.Helper()
	for i, s := range steps {
		now := t0.Add(s.at)
		switch s.do {
		case actNext:
			got, ok := c.Next(now)
			if got != s.want || ok != s.wantOK {
				t.Fatalf("paso %d: Next(+%v) = (%q, %v); quiero (%q, %v)",
					i, s.at, got, ok, s.want, s.wantOK)
			}
		case actMark:
			var resets time.Time
			if s.resets != 0 {
				resets = t0.Add(s.resets)
			}
			c.MarkExhausted(s.profile, resets, now)
		case actAdvance:
			c.Advance(s.profile, now)
		case actDwell:
			if got := c.DwellSatisfied(now); got != s.wantBool {
				t.Fatalf("paso %d: DwellSatisfied(+%v) = %v; quiero %v", i, s.at, got, s.wantBool)
			}
		case actHops:
			if got := c.Hops(); got != s.wantInt {
				t.Fatalf("paso %d: Hops() = %d; quiero %d", i, got, s.wantInt)
			}
		case actCurrent:
			if got := c.Current(); got != s.want {
				t.Fatalf("paso %d: Current() = %q; quiero %q", i, got, s.want)
			}
		}
	}
}

// TestChainReturnDue fija la pregunta que hace el temporizador de `return_check`
// (ver launchAndWatch): «¿toca volver a casa?», respondida SIN que haya ocurrido
// ningún límite. Es la única consulta de la cadena que no nace de un sensor, y
// por eso se prueba aparte de Next.
func TestChainReturnDue(t *testing.T) {
	newChain := func(noReturn bool, maxHops int) *Chain {
		eff := pol(t, core.AutoPolicy{
			Fallback: []string{"app", "deep"},
			MinDwell: "0s",
			MaxHops:  maxHops,
			Cooldown: core.AutoCooldown{Strategy: core.CooldownFixed, Fallback: "1h"},
		})
		return NewChain(core.ResolvedChain{
			Policy: eff, Primary: "personal", Fallback: []string{"app", "deep"},
		}, 0, noReturn, t0)
	}

	// En casa nunca toca volver, esté el primario como esté.
	if c := newChain(false, 6); c.ReturnDue(t0.Add(time.Hour)) {
		t.Error("ReturnDue en el primario = true; quiero false")
	}

	// Prestados: manda el cooldown del primario, no el reloj.
	c := newChain(false, 6)
	c.MarkExhausted("personal", time.Time{}, t0) // libre a partir de t0+1h
	c.Advance("app", t0)
	if c.ReturnDue(t0.Add(59 * time.Minute)) {
		t.Error("ReturnDue con el primario en cooldown = true; quiero false")
	}
	if !c.ReturnDue(t0.Add(time.Hour)) {
		t.Error("ReturnDue en el instante del reset = false; quiero true (borde inclusivo)")
	}

	// Con el presupuesto de préstamos consumido, volver a casa sigue tocando: el
	// tope limita a dónde se puede PRESTAR, no el derecho a recuperar la sesión.
	tope := newChain(false, 1)
	tope.MarkExhausted("personal", time.Time{}, t0)
	tope.Advance("app", t0)
	if tope.Hops() != 1 {
		t.Fatalf("Hops() = %d; quiero 1", tope.Hops())
	}
	if !tope.ReturnDue(t0.Add(time.Hour)) {
		t.Error("ReturnDue con max_hops agotado = false; quiero true")
	}

	// --no-return lo apaga entero.
	sin := newChain(true, 6)
	sin.MarkExhausted("personal", time.Time{}, t0)
	sin.Advance("app", t0)
	if sin.ReturnDue(t0.Add(5 * time.Hour)) {
		t.Error("ReturnDue con noReturn = true; quiero false")
	}
}

// TestChainAllExhausted cubre el final de trayecto: sin destino posible, el
// caller necesita la tabla de cooldowns POBLADA para poder decirle al usuario
// cuándo relanzar. Un ok=false con Cooldowns vacío sería un "ríndete" sin
// explicación.
func TestChainAllExhausted(t *testing.T) {
	eff := pol(t, core.AutoPolicy{
		Fallback: []string{"app", "deep"},
		MinDwell: "0s",
		Cooldown: core.AutoCooldown{Strategy: core.CooldownResetsAt, Fallback: "1h"},
	})
	c := NewChain(core.ResolvedChain{
		Policy:   eff,
		Primary:  "personal",
		Fallback: []string{"app", "deep"},
	}, 0, false, t0)

	// El primario reporta un resets_at explícito (señal buena); los préstamos
	// caen al cooldown fijo por no traer ventana.
	c.MarkExhausted("personal", t0.Add(5*time.Hour), t0)
	if target, ok := c.Next(t0); !ok || target != "app" {
		t.Fatalf("Next inicial = (%q, %v); quiero (app, true)", target, ok)
	}
	c.Advance("app", t0)
	c.MarkExhausted("app", time.Time{}, t0.Add(time.Minute))
	c.Advance("deep", t0.Add(time.Minute))
	c.MarkExhausted("deep", time.Time{}, t0.Add(2*time.Minute))

	if target, ok := c.Next(t0.Add(3 * time.Minute)); ok {
		t.Fatalf("Next con todo agotado = (%q, true); quiero ok=false", target)
	}

	cd := c.Cooldowns()
	want := map[string]time.Time{
		"personal": t0.Add(5 * time.Hour),           // resets_at respetado
		"app":      t0.Add(time.Minute + time.Hour), // desconocido -> now + 1h
		"deep":     t0.Add(2*time.Minute + time.Hour),
	}
	if len(cd) != len(want) {
		t.Fatalf("Cooldowns() = %v; quiero %d entradas", cd, len(want))
	}
	for k, v := range want {
		if !cd[k].Equal(v) {
			t.Errorf("Cooldowns()[%q] = %v; quiero %v", k, cd[k], v)
		}
	}

	// La copia defensiva: mutar lo devuelto no puede resucitar un perfil.
	cd["personal"] = t0
	if got := c.Cooldowns()["personal"]; !got.Equal(t0.Add(5 * time.Hour)) {
		t.Errorf("Cooldowns() expone el mapa interno: %v", got)
	}

	// Pasadas las 5h el primario reabre y el péndulo vuelve a casa.
	if target, ok := c.Next(t0.Add(5 * time.Hour)); !ok || target != "personal" {
		t.Fatalf("Next tras el reset = (%q, %v); quiero (personal, true)", target, ok)
	}
}

// TestChainCooldownSources fija cómo se traduce cada forma de señal a un
// instante de disponibilidad. Es la regla más fácil de romper sin darse cuenta
// al tocar MarkExhausted.
func TestChainCooldownSources(t *testing.T) {
	cases := []struct {
		name     string
		strategy string
		resets   time.Time
		markAt   time.Duration
		want     time.Duration // desplazamiento esperado desde t0
	}{
		{
			name:     "resets_at conocido y futuro manda",
			strategy: core.CooldownResetsAt,
			resets:   t0.Add(4 * time.Hour),
			markAt:   10 * time.Minute,
			want:     4 * time.Hour,
		},
		{
			// El caso del contrato: sin ventana que consultar, cooldown fijo
			// contado desde el momento del agotamiento (no desde t0).
			name:     "resets_at cero cae al cooldown fijo de la política",
			strategy: core.CooldownResetsAt,
			resets:   time.Time{},
			markAt:   10 * time.Minute,
			want:     10*time.Minute + 90*time.Minute,
		},
		{
			// Una señal que dice "reabrió hace rato" se contradice con el
			// hecho de haber topado el límite: se trata como ausente para no
			// entrar en ping-pong.
			name:     "resets_at ya pasado se ignora",
			strategy: core.CooldownResetsAt,
			resets:   t0.Add(time.Minute),
			markAt:   10 * time.Minute,
			want:     10*time.Minute + 90*time.Minute,
		},
		{
			// strategy fixed ignora resets_at por diseño (perfiles con API key).
			name:     "strategy fixed ignora resets_at",
			strategy: core.CooldownFixed,
			resets:   t0.Add(4 * time.Hour),
			markAt:   10 * time.Minute,
			want:     10*time.Minute + 90*time.Minute,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			eff := pol(t, core.AutoPolicy{
				Fallback: []string{"app"},
				MinDwell: "0s",
				Cooldown: core.AutoCooldown{Strategy: tc.strategy, Fallback: "90m"},
			})
			c := NewChain(core.ResolvedChain{
				Policy: eff, Primary: "personal", Fallback: []string{"app"},
			}, 0, false, t0)

			c.MarkExhausted("personal", tc.resets, t0.Add(tc.markAt))
			got := c.Cooldowns()["personal"]
			if !got.Equal(t0.Add(tc.want)) {
				t.Fatalf("cooldown = %v; quiero %v", got, t0.Add(tc.want))
			}
			// Coherencia con Next: un instante antes sigue agotado, en el
			// instante exacto ya es elegible (borde inclusivo).
			c.Advance("app", t0.Add(tc.markAt))
			if target, ok := c.Next(t0.Add(tc.want - time.Nanosecond)); ok {
				t.Fatalf("Next justo antes del reset = %q; quiero ok=false", target)
			}
			if target, ok := c.Next(t0.Add(tc.want)); !ok || target != "personal" {
				t.Fatalf("Next en el reset = (%q, %v); quiero (personal, true)", target, ok)
			}
		})
	}
}

// TestChainDwell comprueba que la permanencia mínima se mide desde la LLEGADA
// al perfil actual, que es lo que amortigua la ráfaga de señales duplicadas.
func TestChainDwell(t *testing.T) {
	eff := pol(t, core.AutoPolicy{
		Fallback: []string{"app"},
		MinDwell: "20m",
		Cooldown: core.AutoCooldown{Strategy: core.CooldownFixed, Fallback: "1h"},
	})
	c := NewChain(core.ResolvedChain{
		Policy: eff, Primary: "personal", Fallback: []string{"app"},
	}, 0, false, t0)

	cases := []struct {
		at   time.Duration
		want bool
	}{
		{at: 0, want: false},
		{at: 19 * time.Minute, want: false},
		{at: 20 * time.Minute, want: true}, // borde inclusivo
		{at: time.Hour, want: true},
	}
	for _, tc := range cases {
		if got := c.DwellSatisfied(t0.Add(tc.at)); got != tc.want {
			t.Errorf("DwellSatisfied(+%v) = %v; quiero %v", tc.at, got, tc.want)
		}
	}

	// Advance reinicia el reloj: al llegar a "app" a los 30m, la permanencia
	// vuelve a contar desde cero aunque llevemos media hora de sesión.
	c.Advance("app", t0.Add(30*time.Minute))
	if c.DwellSatisfied(t0.Add(30 * time.Minute)) {
		t.Error("DwellSatisfied justo tras Advance = true; quiero false")
	}
	if !c.DwellSatisfied(t0.Add(50 * time.Minute)) {
		t.Error("DwellSatisfied 20m tras Advance = false; quiero true")
	}

	// MinDwell 0 (o ausente) no bloquea nunca: sin amortiguación configurada,
	// el supervisor rota en cuanto detecta.
	eff0 := pol(t, core.AutoPolicy{Fallback: []string{"app"}, MinDwell: "0s"})
	c0 := NewChain(core.ResolvedChain{Policy: eff0, Primary: "personal"}, 0, false, t0)
	if !c0.DwellSatisfied(t0) {
		t.Error("DwellSatisfied con MinDwell 0 = false; quiero true")
	}
}

// TestChainAdvanceNoop protege la invariante que hace segura la separación
// elegir/ejecutar: Advance a donde ya estamos (o a vacío, si el handoff falló
// y el caller reintenta) no puede consumir presupuesto ni reiniciar el dwell.
func TestChainAdvanceNoop(t *testing.T) {
	eff := pol(t, core.AutoPolicy{Fallback: []string{"app"}, MinDwell: "20m"})
	c := NewChain(core.ResolvedChain{
		Policy: eff, Primary: "personal", Fallback: []string{"app"},
	}, 0, false, t0)

	c.Advance("personal", t0.Add(time.Hour))
	c.Advance("", t0.Add(time.Hour))
	if c.Hops() != 0 {
		t.Errorf("Hops() = %d tras Advance no-op; quiero 0", c.Hops())
	}
	if !c.DwellSatisfied(t0.Add(21 * time.Minute)) {
		t.Error("Advance no-op reinició el dwell")
	}
}

// TestDwellForSeparaProactivoDeReactivo fija la asimetría de la permanencia
// mínima, que es la única regla de política que distingue quién dio el aviso.
//
// El dwell entero solo tiene sentido cuando aún no ha fallado nada: el sensor
// proactivo avisa ANTES del 429, así que esperar es gratis y evita quemar la
// cadena por un pico. Un evento reactivo significa que el turno YA falló, y como
// `since` se siembra en NewChain, aplicarle el dwell entero retiene al usuario
// los 20 minutos completos desde el arranque de la corrida dentro de una cuenta
// que devuelve 429, teniendo otra fresca al lado.
//
// El caso que más importa es el último: un origen que no reconocemos cae al lado
// urgente, no al conservador. El Source de un sentinel lo escribe el hook desde
// un payload externo, así que "no sé qué es esto" tiene que rotar pronto de más
// antes que retener de más.
func TestDwellForSeparaProactivoDeReactivo(t *testing.T) {
	chainCon := func(minDwell string) *Chain {
		eff := pol(t, core.AutoPolicy{Fallback: []string{"app"}, MinDwell: minDwell})
		return NewChain(core.ResolvedChain{
			Policy: eff, Primary: "personal", Fallback: []string{"app"},
		}, 0, false, t0)
	}
	c := chainCon("20m")

	if got := c.DwellFor("statusline"); got != 20*time.Minute {
		t.Errorf("DwellFor(statusline) = %v, quería el MinDwell entero (20m)", got)
	}
	for _, src := range []string{"transcript", "stream-json", "hook", "", "vete-a-saber"} {
		if got := c.DwellFor(src); got != reactiveDwellCap {
			t.Errorf("DwellFor(%q) = %v, quería el techo corto %v", src, got, reactiveDwellCap)
		}
	}

	// Con un MinDwell más corto que el techo manda el MinDwell: el techo acota,
	// no impone un mínimo. Un usuario que pidió 5s no debe esperar 30.
	corto := chainCon("5s")
	if got := corto.DwellFor("transcript"); got != 5*time.Second {
		t.Errorf("DwellFor con MinDwell 5s = %v, quería 5s", got)
	}
	// Y con MinDwell 0 (el default de los tests) no hay espera de ninguna clase.
	cero := chainCon("0s")
	if got := cero.DwellFor("transcript"); got != 0 {
		t.Errorf("DwellFor con MinDwell 0 = %v, quería 0", got)
	}
	if !cero.DwellSatisfiedFor(t0, 0) {
		t.Error("DwellSatisfiedFor con dwell 0 = false; quiero true")
	}
}
