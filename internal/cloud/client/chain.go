package client

// chain.go — verificar la historia entera, no un snapshot suelto (spec
// §10.3.1). La firma de cada snapshot ata su id, su padre y su contenido desde
// F1, pero nadie comparaba nunca dos eslabones: un cliente que verifica de uno
// en uno no puede notar que falta el del medio. Eso es lo que hace esto.

import (
	"sort"

	"github.com/JoseAFlores777/ccp/internal/cloud/api"
	"github.com/JoseAFlores777/ccp/internal/cloud/crypt"
)

// Códigos de falta de la cadena. Son códigos y no prosa porque los traduce el
// CLI y los pinta la GUI; viajan igual en `--json`.
const (
	// FaultBadSignature: el eslabón no lo firmó esta cuenta, o le cambiaron el
	// id, el padre o el manifiesto.
	FaultBadSignature = "bad_signature"
	// FaultBrokenLink: su padre no está en la cadena. Es lo que deja un
	// eslabón quitado del medio.
	FaultBrokenLink = "broken_link"
	// FaultDropped: un snapshot que ESTA máquina subió ya no está. Es la única
	// forma de ver que han cortado la cadena por la cabeza, donde no queda
	// ningún padre roto que lo delate.
	FaultDropped = "dropped"
	// FaultCycle: los padres se muerden la cola, así que esos eslabones no
	// cuelgan de ningún principio.
	FaultCycle = "cycle"
	// FaultOutOfOrder: un eslabón dice ser anterior a su padre.
	FaultOutOfOrder = "out_of_order"
	// FaultDuplicateID: dos eslabones con el mismo id.
	FaultDuplicateID = "duplicate_id"
)

// ChainFault es una falta concreta. Ref es el otro eslabón implicado (el padre
// que falta, normalmente) y va vacío cuando la falta es del propio eslabón.
type ChainFault struct {
	Code string `json:"code"`
	ID   string `json:"id"`
	Ref  string `json:"ref,omitempty"`
}

// ChainReport es el resultado de verificar la cadena. Las listas nunca salen
// como null: `ccp cloud verify --json` es superficie de máquina.
type ChainReport struct {
	Links  int          `json:"links"`
	Roots  int          `json:"roots"`
	Faults []ChainFault `json:"faults"`
}

// OK dice si la historia está intacta.
func (r ChainReport) OK() bool { return len(r.Faults) == 0 }

// VerifyChain comprueba la historia que devolvió el servidor: que cada eslabón
// lo firmó esta cuenta, que ninguno cuelga de un padre que no está, que no hay
// ciclos ni ids repetidos, que las fechas no contradicen a los padres y que
// sigue estando todo lo que esta máquina subió (pushed, ids en la nube).
//
// La fecha NO va firmada, y por eso no ordena nada: el orden lo dibujan los
// padres, que sí están firmados. Lo que caza FaultOutOfOrder es una fecha que
// miente —el listado que enseña el portal sale de ella—, no un reordenamiento
// de la historia, que es imposible sin la clave de cuenta.
func VerifyChain(acct *crypt.Account, links []api.ChainLink, pushed []string) ChainReport {
	rep := ChainReport{Links: len(links), Faults: []ChainFault{}}
	byID := make(map[string]api.ChainLink, len(links))
	for _, l := range links {
		if _, dup := byID[l.ID]; dup {
			rep.Faults = append(rep.Faults, ChainFault{Code: FaultDuplicateID, ID: l.ID})
			continue
		}
		byID[l.ID] = l
	}
	for _, l := range links {
		if err := acct.VerifyDigest(l.ID, l.Parent, l.Digest, l.Sig); err != nil {
			rep.Faults = append(rep.Faults, ChainFault{Code: FaultBadSignature, ID: l.ID})
		}
		if l.Parent == "" {
			rep.Roots++
			continue
		}
		p, ok := byID[l.Parent]
		if !ok {
			rep.Faults = append(rep.Faults, ChainFault{Code: FaultBrokenLink, ID: l.ID, Ref: l.Parent})
			continue
		}
		if l.Created.Before(p.Created) {
			rep.Faults = append(rep.Faults, ChainFault{Code: FaultOutOfOrder, ID: l.ID, Ref: l.Parent})
		}
	}
	for _, id := range cycles(byID) {
		rep.Faults = append(rep.Faults, ChainFault{Code: FaultCycle, ID: id})
	}
	for _, id := range pushed {
		if _, ok := byID[id]; !ok {
			rep.Faults = append(rep.Faults, ChainFault{Code: FaultDropped, ID: id})
		}
	}
	return rep
}

// cycles devuelve, ordenados, los eslabones que no llegan a ninguna raíz
// siguiendo a sus padres. Se recorre desde cada uno con los ya resueltos
// memorizados, así que una historia larga se recorre una vez y no una por
// eslabón.
func cycles(byID map[string]api.ChainLink) []string {
	const (
		pendiente = 0
		enCamino  = 1
		sano      = 2
		enCiclo   = 3
	)
	estado := make(map[string]int, len(byID))
	var out []string
	for id := range byID {
		if estado[id] != pendiente {
			continue
		}
		// Se marca el camino entero antes de decidir: así un eslabón que cae
		// en un ciclo ya resuelto no vuelve a recorrerlo.
		var camino []string
		cur, veredicto := id, sano
		for {
			l, ok := byID[cur]
			if !ok { // padre que no está: eso ya lo cuenta FaultBrokenLink
				break
			}
			if st := estado[cur]; st != pendiente {
				if st == enCamino || st == enCiclo {
					veredicto = enCiclo
				}
				break
			}
			estado[cur] = enCamino
			camino = append(camino, cur)
			if l.Parent == "" {
				break
			}
			cur = l.Parent
		}
		for _, c := range camino {
			estado[c] = veredicto
			if veredicto == enCiclo {
				out = append(out, c)
			}
		}
	}
	sort.Strings(out)
	return out
}
