package store

// El registro de auditoría (§10.5, «auditoría de solo inserción»). Aquí solo
// viven sus tipos y la regla que lo mantiene honesto; escribirlo y leerlo es
// de cada implementación.

import "time"

const (
	// MaxAuditDetailLen es lo que cabe en un valor del detalle. Un id son 64
	// hexadecimales y un nombre de equipo es corto: lo que no cabe ahí no es
	// un detalle, es configuración, y el servidor no la puede leer por
	// diseño. Recortar aquí es lo que impide que un descuido de quien llama
	// convierta el registro en la fuga que el cifrado evita.
	MaxAuditDetailLen = 256
	// MaxAuditDetailKeys tapa el otro lado del mismo agujero: un detalle con
	// cien claves es una estructura, no una nota.
	MaxAuditDetailKeys = 24
	// DefaultAuditLimit es lo que devuelve una lectura sin límite. El
	// registro crece sin fin, así que «todas» aquí sería una trampa — al
	// revés que en GroupRevisions, donde la respuesta ES la última de cada
	// equipo y una ventana escondería justo al que lleva más tiempo mudo.
	DefaultAuditLimit = 200
)

// AuditEntry es una línea del registro: quién hizo qué y cuándo. Nunca lleva
// configuración (ver SanitizeAuditDetail).
type AuditEntry struct {
	ID       int64
	At       time.Time
	DeviceID string
	// DeviceName se resuelve al leer, y queda vacío si el equipo ya no está:
	// el registro guarda el id, que es lo que no cambia.
	DeviceName string
	Action     string
	Detail     map[string]any
}

// AuditFilter acota una lectura. El cero es «lo último de toda la cuenta».
type AuditFilter struct {
	DeviceID string
	Action   string
	Since    time.Time
	Limit    int
}

// SanitizeAuditDetail deja el detalle en lo que es seguro guardar: valores
// sueltos y cortos. Un objeto o una lista anidados se van enteros —son la
// forma que tiene la configuración— y una cadena larga se recorta. No
// devuelve error a propósito: apuntar la acción importa más que el detalle, y
// fallar aquí dejaría sin registro justo a la operación que hay que auditar.
func SanitizeAuditDetail(d map[string]any) map[string]any {
	if len(d) == 0 {
		return map[string]any{}
	}
	out := make(map[string]any, len(d))
	for k, v := range d {
		if len(out) >= MaxAuditDetailKeys {
			break
		}
		if len(k) > MaxAuditDetailLen {
			continue
		}
		switch t := v.(type) {
		case string:
			if len(t) > MaxAuditDetailLen {
				t = t[:MaxAuditDetailLen]
			}
			out[k] = t
		case bool, int, int64, float64:
			out[k] = t
		case []byte: // unos bytes en el registro son siempre contenido
			continue
		default:
			continue
		}
	}
	return out
}
