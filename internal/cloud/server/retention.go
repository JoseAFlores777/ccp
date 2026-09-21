package server

// retention.go — la poda del servidor (spec §10.3.1). Dos reglas mandan sobre
// todo lo demás:
//
//  1. **Podar borra el contenido, nunca el eslabón.** De un snapshot podado se
//     van el manifiesto y sus blobs —que es todo lo que ocupa—, y la fila se
//     queda como lápida con su id, su padre, su fecha, su digest y su firma.
//     Si se borrara entera, la cadena tendría un hueco, y un hueco que deja la
//     retención es indistinguible de uno que deja un servidor comprometido: la
//     comprobación de §10.3.1 se convertiría en un aviso que hay que ignorar
//     cada día, o sea en ninguna comprobación.
//  2. **Un fijado no se poda jamás**, ni por edad ni por cuota. En el servidor
//     «fijado» es una sola bandera: la etiqueta vive dentro del manifiesto
//     sellado, que él no puede leer, así que es el cliente quien le dice cuáles
//     lo están (al publicarlos y en cada `push`).

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"time"

	"github.com/JoseAFlores777/ccp/internal/cloud/api"
	"github.com/JoseAFlores777/ccp/internal/cloud/blobs"
	"github.com/JoseAFlores777/ccp/internal/cloud/store"
)

// Retention es la política de retención, configurable por despliegue. El cero
// es «guardar todo», que es el valor por defecto de §10.3.1: con deduplicación
// un snapshot de configuración pesa kilobytes y conservarlos todos es lo
// razonable; podar es la excepción que se enciende a mano.
type Retention struct {
	Daily   int
	Weekly  int
	Monthly int
	// Grace protege a un blob recién subido de la poda aunque no lo referencie
	// nadie todavía: puede ser de un `push` a medias, que sube los blobs antes
	// de publicar el snapshot que los nombra.
	Grace time.Duration
	// Every es cada cuánto se barre como mucho la historia de una cuenta.
	Every time.Duration
}

// Enabled dice si hay algo que podar. Sin ningún periodo, el servidor guarda
// todo, que es lo que hace desde F1.
func (r Retention) Enabled() bool { return r.Daily > 0 || r.Weekly > 0 || r.Monthly > 0 }

// retain devuelve los ids que se conservan, con la misma política que la poda
// local (`snapshot.Retain`, §8.3): el más reciente de cada uno de los últimos N
// días, semanas y meses **que tengan snapshots**, así que un mes sin ninguno no
// gasta un hueco. Se reescribe aquí en vez de reusar aquella porque el servidor
// no puede depender del almacén local —no sabe nada de `~/.claude`— y porque lo
// que ve son filas opacas, no manifiestos; si una de las dos cambia, la otra
// tiene que cambiar con ella.
func retain(list []store.Snapshot, p Retention) map[string]bool {
	keep := map[string]bool{}
	sorted := append([]store.Snapshot(nil), list...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Created.After(sorted[j].Created) })
	for _, s := range sorted {
		// Una lápida ya no tiene contenido que liberar, y sostiene la cadena.
		if s.Pinned || s.Pruned {
			keep[s.ID] = true
		}
	}
	if !p.Enabled() {
		for _, s := range sorted {
			keep[s.ID] = true
		}
		return keep
	}
	if len(sorted) > 0 {
		keep[sorted[0].ID] = true
	}
	// Las lápidas no entran en los cubos: ya no ocupan nada, y dejarlas
	// gastar el hueco de su día podaría al que sí tiene contenido.
	bucket := func(n int, key func(time.Time) string) {
		last, count := "", 0
		for _, s := range sorted {
			if count >= n {
				return
			}
			if s.Pruned {
				continue
			}
			k := key(s.Created.UTC())
			if k == last {
				continue
			}
			keep[s.ID] = true
			last, count = k, count+1
		}
	}
	bucket(p.Daily, func(t time.Time) string { return t.Format("2006-01-02") })
	bucket(p.Weekly, func(t time.Time) string { y, w := t.ISOWeek(); return fmt.Sprintf("%d-W%02d", y, w) })
	bucket(p.Monthly, func(t time.Time) string { return t.Format("2006-01") })
	return keep
}

// sweep poda la historia de un usuario según la política. Corre DENTRO de la
// petición que publica un snapshot, no en una goroutine suelta: allí el
// resultado se sale de la petición que lo causó y no queda nadie mirándolo, y
// aquí, con `Every` de por medio, un `push` normal no paga nada. Es siempre
// «lo mejor que se pueda»: si algo falla se anota y la publicación sigue,
// porque no poder liberar sitio no es razón para rechazar un snapshot.
func (s *srv) sweep(ctx context.Context, r *http.Request, userID string) {
	p := s.cfg.Retention
	if !p.Enabled() || !s.sweepDue(userID, p.Every) {
		return
	}
	list, err := s.cfg.Store.Chain(ctx, userID, api.MaxChainLinks)
	if err != nil {
		s.cfg.Log.Error("retención: no se pudo leer la cadena", "err", err, "req", reqID(r))
		return
	}
	keep := retain(list, p)
	var sobran []string
	for _, sn := range list {
		if !keep[sn.ID] {
			sobran = append(sobran, sn.ID)
		}
	}
	// Aunque no sobre ningún snapshot se sigue: la poda anterior pudo dejar
	// blobs que el bucket no aceptó borrar, y esta es la vuelta que los
	// reintenta.
	libres, err := s.cfg.Store.PruneSnapshots(ctx, userID, sobran, s.cfg.Now().Add(-p.Grace))
	if err != nil {
		s.cfg.Log.Error("retención: no se pudo podar", "err", err, "req", reqID(r))
		return
	}
	// Del bucket, después de la base y uno a uno: una fila que apuntara a un
	// objeto que ya no está sería un snapshot roto. El que no se pueda borrar
	// se queda apuntado en la base (`libres` lo volverá a traer la próxima
	// vez); solo se olvida el que se fue de verdad.
	idos := []string{}
	for _, id := range libres {
		if err := s.cfg.Blobs.Delete(ctx, blobs.Key(userID, id)); err != nil {
			s.cfg.Log.Error("retención: blob que se queda en el almacenamiento, se reintentará", "err", err, "blob", id)
			continue
		}
		idos = append(idos, id)
	}
	if err := s.cfg.Store.ForgetBlobs(ctx, userID, idos); err != nil {
		// Olvidarlos es lo único que queda: no hacerlo solo cuesta un borrado
		// repetido, que el almacenamiento acepta (ver blobs.Blobs.Delete).
		s.cfg.Log.Error("retención: no se pudo olvidar los blobs borrados", "err", err, "req", reqID(r))
	}
	if len(sobran) == 0 && len(idos) == 0 {
		return
	}
	s.cfg.Log.Info("retención", "user", userID, "podados", len(sobran), "blobs", len(idos))
}

// sweepDue dice si toca barrer y, si toca, lo apunta. Sin esto, cada
// publicación recorrería la historia entera de la cuenta.
func (s *srv) sweepDue(userID string, every time.Duration) bool {
	now := s.cfg.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	if every > 0 && now.Sub(s.swept[userID]) < every {
		return false
	}
	s.swept[userID] = now
	return true
}
