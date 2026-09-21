package client

// retry.go — UNA política de reintento para todo el cliente.
//
// Los dos caminos que salen de aquí fallan igual —el JSON contra el API y las
// URLs prefirmadas contra el bucket— y esperar distinto en cada uno es la misma
// clase de error que tener dos formateadores para un mismo evento: se arregla
// uno, el otro se queda, y nadie lo nota hasta que el servidor está apretado.

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/JoseAFlores777/ccp/internal/cloud/api"
)

const (
	// retryAttempts: intentos totales, no reintentos.
	retryAttempts = 4
	retryBase     = 500 * time.Millisecond
	// retryAfterCap: el servidor propone cuánto esperar y aquí se acepta, pero
	// con techo. Sin él, un `Retry-After` de un día —de un servidor mal
	// configurado o de uno que miente— deja la terminal parada sin que quien la
	// mira tenga forma de saber por qué.
	retryAfterCap = 30 * time.Second
)

// postRepetible son los únicos POST que se pueden repetir, y cada uno tiene su
// motivo: `presign` es una lectura disfrazada de POST (lo es porque la lista de
// ids no cabe en una query), y `snapshots` contesta 200 en vez de 201 cuando el
// snapshot ya estaba, que es la definición de idempotente. Los demás crean una
// fila por llamada: repetir un alta de dispositivo deja un equipo fantasma en
// la cuenta y repetir una revisión la publica dos veces.
var postRepetible = map[string]bool{
	"/v1/blobs/presign": true,
	"/v1/snapshots":     true,
}

// idempotent dice si repetir esa petición es seguro. GET, HEAD, PUT y DELETE lo
// son por definición de HTTP; POST, solo donde lo diga postRepetible.
func idempotent(method, path string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodPut, http.MethodDelete:
		return true
	case http.MethodPost:
		return postRepetible[strings.SplitN(path, "?", 2)[0]]
	}
	return false
}

// retryableStatus: un 429 (el límite por usuario) y cualquier 5xx. El resto son
// respuestas del servidor sobre lo que se le pidió, y repetir la pregunta no
// cambia la contestación.
func retryableStatus(code int) bool {
	return code == http.StatusTooManyRequests || code >= 500
}

// parseRetryAfter lee la cabecera `Retry-After` en sus dos formas (segundos o
// fecha HTTP) y devuelve la espera, acotada a retryAfterCap. Lo que no se
// entiende son 0 segundos, no «para siempre»: el retroceso exponencial decide.
func parseRetryAfter(h string, now time.Time) time.Duration {
	h = strings.TrimSpace(h)
	if h == "" {
		return 0
	}
	var d time.Duration
	if n, err := strconv.Atoi(h); err == nil {
		d = time.Duration(n) * time.Second
	} else if t, err := http.ParseTime(h); err == nil {
		d = t.Sub(now)
	}
	return min(max(d, 0), retryAfterCap)
}

// retryWait es lo que se espera antes del intento número attempt (desde 0). La
// pista del servidor solo manda cuando pide esperar MÁS: correr antes de tiempo
// no adelanta nada —el límite es por usuario— y sí gasta la ráfaga siguiente.
func retryWait(attempt int, hint time.Duration) time.Duration {
	return max(time.Duration(1<<attempt)*retryBase, hint)
}

// retry ejecuta fn hasta retryAttempts veces mientras diga que el fallo es
// reintentable. hint es lo que el servidor propuso esperar; 0 = solo el
// retroceso exponencial.
func retry(ctx context.Context, fn func() (retryable bool, hint time.Duration, err error)) error {
	var err error
	for attempt := range retryAttempts {
		var again bool
		var hint time.Duration
		if again, hint, err = fn(); err == nil || !again {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(retryWait(attempt, hint)):
		}
	}
	return err
}

// presignCovers comprueba que la respuesta trae cada id pedido exactamente una
// vez y ninguno de más. Un id que no vuelve NO es «ese blob no está» —eso es
// `Exists: false`, que sí viaja—, es el servidor contestando otra cosa a lo que
// se le preguntó; al bajar, ese elemento se quedaría fuera del almacén local
// sin salir siquiera en la lista de lo que faltaba.
func presignCovers(ids []string, items []api.PresignItem) error {
	pedidos := make(map[string]bool, len(ids))
	for _, id := range ids {
		pedidos[id] = true
	}
	vistos := make(map[string]bool, len(items))
	for _, it := range items {
		if !pedidos[it.ID] {
			return fmt.Errorf("la nube devolvió una URL para un blob que nadie pidió (%s)", short(it.ID))
		}
		if vistos[it.ID] {
			return fmt.Errorf("la nube devolvió dos veces el mismo blob (%s)", short(it.ID))
		}
		vistos[it.ID] = true
	}
	for _, id := range ids {
		if !vistos[id] {
			return fmt.Errorf("la nube no contestó por %d de los %d blobs que se le pidieron (falta %s)",
				len(ids)-len(vistos), len(ids), short(id))
		}
	}
	return nil
}

// short recorta un id para los mensajes: entero no cabe y no dice más.
func short(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
}
