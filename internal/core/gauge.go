package core

import (
	"fmt"
	"math"
	"strings"
	"time"
)

// gauge.go — primitivas de medidor y cuenta atrás para las superficies que
// enseñan consumo (la barra de `ccp _statusline` y, más adelante, el panel
// Estado del TUI).
//
// Son PURAS a propósito: devuelven texto sin color, sin ancho y sin orden. El
// core no puede decidir nada de eso porque lo consumen DOS front-ends con
// políticas distintas (`internal/cli` colorea con ANSI y mide con COLUMNS,
// `internal/tui` pinta con lipgloss) y `internal/tui` no puede importar
// `internal/cli`. Si el color o el layout vivieran aquí, uno de los dos tendría
// que deshacerlos. Regla: aquí el QUÉ dice el medidor; el CÓMO se ve lo deciden
// arriba.

const (
	// Los tres glifos del medidor. Se eligió un medidor de bloques y no un punto
	// de color porque tiene que seguir siendo legible con NO_COLOR y en un pipe:
	// la forma lleva la información, el color solo la subraya.
	gaugeEdge  = "▏" // ▏ delimitador fino, a ambos lados
	gaugeFull  = "█" // █ celda consumida
	gaugeEmpty = "░" // ░ celda libre

	// maxGaugeCells acota el tamaño del medidor.
	//
	// `cells` no lo escribe una persona: sale de aritmética de ancho en el
	// front-end (COLUMNS menos las etiquetas), y COLUMNS viene del entorno, o
	// sea de fuera. Un COLUMNS absurdo o un cálculo que se desmadre pediría un
	// medidor de megabytes... en un comando que Claude Code ejecuta varias veces
	// por minuto. 200 celdas ya es más ancho que cualquier terminal real, así
	// que acotar ahí no recorta ningún caso legítimo y sí cierra el de memoria.
	maxGaugeCells = 200
)

// ClampPct devuelve pct llevado a una medida presentable: [0,100], con NaN e Inf
// leídos como 0.
//
// Existe EXPORTADA y separada de RenderGauge porque el porcentaje se enseña dos
// veces —el medidor y el número al lado— y las dos tienen que decir lo mismo. Con
// el clamp escondido dentro de RenderGauge pasó justo lo contrario: una muestra
// con `used_percentage: 9e99` pintaba el medidor lleno (correcto) y a su lado un
// número de 300 dígitos que reventaba la línea de estado entera; y un -40 pintaba
// el medidor vacío junto a un "-40%" en verde. El front-end no puede acordarse de
// acotar por su cuenta: la única forma de que no se contradigan es que ambos
// partan del MISMO valor saneado.
//
// NaN se descarta ANTES de comparar: falla todas las comparaciones, así que un
// clamp a secas lo dejaría pasar intacto hasta math.Round, y de ahí saldría un
// int indefinido que como cuenta de repeticiones puede ser cualquier cosa. Inf sí
// sobreviviría al clamp, pero se trata igual por coherencia: ninguno de los dos
// es una medida, y ante «no sé» un medidor lleno mentiría más que uno vacío.
func ClampPct(pct float64) float64 {
	if math.IsNaN(pct) || math.IsInf(pct, 0) {
		return 0
	}
	switch {
	case pct < 0:
		return 0
	case pct > 100:
		return 100
	}
	return pct
}

// RenderGauge devuelve un medidor de bloques de exactamente cells celdas para
// pct, con sus delimitadores: "▏███░░░░░░░▏".
//
// Solo el medidor: ni porcentaje, ni etiqueta, ni ANSI. El llamador compone.
//
// Defensivo por contrato, porque el consumidor final es `ccp _statusline`, que
// SIEMPRE tiene que salir 0: un pct que llega de un JSON ajeno puede ser NaN,
// infinito o negativo, y aquí nada de eso puede acabar en un panic ni en un
// strings.Repeat con cuenta negativa. El saneado del porcentaje es ClampPct.
//   - cells <= 0 → "" (no cabe ni un delimitador; que el llamador decida qué poner).
//   - cells > maxGaugeCells → se acota (ver la constante).
func RenderGauge(pct float64, cells int) string {
	if cells <= 0 {
		return ""
	}
	if cells > maxGaugeCells {
		cells = maxGaugeCells
	}

	pct = ClampPct(pct)

	// Redondeo al entero más cercano, no truncado: con 10 celdas un 59 % debe
	// verse como 6 celdas (más de la mitad), no como 5.
	filled := int(math.Round(pct / 100 * float64(cells)))
	// Cinturón sobre el tirante: el clamp de pct ya garantiza el rango, pero la
	// aritmética en float no es algo sobre lo que apostar un strings.Repeat.
	if filled < 0 {
		filled = 0
	}
	if filled > cells {
		filled = cells
	}

	// Suelo de una celda: cualquier consumo POR ENCIMA de cero ocupa al menos
	// una. Sin esto un 2 % de 10 celdas redondea a 0 y sale idéntico a un 0 % —
	// y «no has gastado nada» y «has empezado a gastar» es justo la distinción
	// que el medidor existe para dar de un vistazo. La precisión no se pierde:
	// el número exacto va al lado. El redondeo proporcional sigue mandando en
	// todo lo demás; esto solo impide que la primera celda desaparezca.
	//
	// Arriba NO hay simetría a propósito: 95 % redondea a lleno y se queda
	// lleno, porque el mensaje de un medidor lleno («esta ventana está para
	// cortar») es exactamente el correcto a esa altura — el motor da la ventana
	// por agotada en el 90 %.
	//
	// Con cells == 1 esto convierte el medidor en binario hay/no hay. Es lo
	// único que una sola celda puede decir con honestidad.
	if filled == 0 && pct > 0 {
		filled = 1
	}

	var b strings.Builder
	b.Grow((cells + 2) * 3) // cada glifo son 3 bytes en UTF-8
	b.WriteString(gaugeEdge)
	b.WriteString(strings.Repeat(gaugeFull, filled))
	b.WriteString(strings.Repeat(gaugeEmpty, cells-filled))
	b.WriteString(gaugeEdge)
	return b.String()
}

// HumanUntil devuelve la cuenta atrás legible hasta un reset: "47m", "2h14m",
// "3d". Una sola unidad significativa, porque esto compite por sitio con el
// nombre del perfil y dos porcentajes en una línea de estado.
//
// Devuelve "" cuando d <= 0, y esa es la regla importante: un resets_at vencido
// o a cero NO se pinta. Es la misma filosofía que defiende Windowed.HasData —
// hay un bug conocido de CC donde five_hour llega vacío con seven_day poblado, y
// pintar "·0m" afirmaría «ya reabrió» cuando lo único que sabemos es que no
// sabemos. Callar es el único mensaje honesto.
//
// Escalones:
//   - < 1h  → minutos ("47m")
//   - < 24h → horas y minutos ("2h14m"), o solo horas si los minutos son 0 ("2h")
//   - >= 24h → días hacia ARRIBA ("3d"): a esa distancia el minuto no informa,
//     y redondear hacia arriba es lo único que nunca promete que falta menos de
//     lo que falta. Truncar sí lo promete: a 2d23h59m del reset pintaría "2d",
//     el usuario volvería un día antes de tiempo y se encontraría el perfil
//     todavía agotado. El error máximo es de un día en cualquiera de los dos
//     sentidos, así que lo único que se elige aquí es la dirección — y es la
//     misma que ya defiende el guard de `d <= 0`: ante la duda, no digas que la
//     cuota está más cerca de lo que sabes.
func HumanUntil(d time.Duration) string {
	if d <= 0 {
		return ""
	}
	switch {
	case d < time.Hour:
		m := int(d / time.Minute)
		// Un resto positivo por debajo del minuto sigue siendo «todavía no».
		// Pintar "0m" lo confundiría con el caso vencido, que justamente se
		// calla; el mínimo es 1m.
		if m < 1 {
			m = 1
		}
		return fmt.Sprintf("%dm", m)
	case d < 24*time.Hour:
		h := int(d / time.Hour)
		m := int((d % time.Hour) / time.Minute)
		if m == 0 {
			return fmt.Sprintf("%dh", h)
		}
		return fmt.Sprintf("%dh%dm", h, m)
	default:
		const day = 24 * time.Hour
		days := int(d / day)
		if d%day != 0 {
			days++
		}
		return fmt.Sprintf("%dd", days)
	}
}

// HumanUntilAt es HumanUntil con la resta hecha: la cuenta atrás desde now
// hasta resetsAt, o "" si no hay nada que decir.
//
// Existe para que la regla completa «sin resets_at, o ya vencido, no se pinta»
// viva en UN sitio. Los dos front-ends parten del mismo dato (Windowed.ResetsAt,
// que es un time.Time que puede venir a cero) y sin este helper cada uno tendría
// que recordar por su cuenta el guard del IsZero — y el que lo olvidara pintaría
// una cuenta atrás gigantesca desde el año 1. El reloj entra por parámetro, como
// en ExhaustedAt, para que la decisión sea testeable sin esperar.
//
// Sigue siendo pura: ni color, ni ancho, ni layout.
func HumanUntilAt(resetsAt, now time.Time) string {
	if resetsAt.IsZero() || now.IsZero() {
		return ""
	}
	return HumanUntil(resetsAt.Sub(now))
}
