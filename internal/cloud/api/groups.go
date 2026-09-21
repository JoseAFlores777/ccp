package api

// Grupos de dispositivos (spec §10.3): «todas mis Macs». Un grupo es una
// etiqueta con miembros y NO autoriza nada — aplicar a un grupo es publicar N
// revisiones, una firmada por máquina. Ver `Group` abajo.

import "time"

const (
	// MaxGroupNameLen: el nombre es un asa para una persona («Macs»), no un
	// campo de texto libre.
	MaxGroupNameLen = 64
	// MaxGroupMembers: una cuenta con más de 200 equipos no es el caso que
	// esto resuelve, y el tope evita que una petición pida 10 000 escrituras.
	MaxGroupMembers = 200
)

// Group es un grupo de dispositivos. La membresía NO va firmada, así que un
// grupo no puede decidir quién obedece una orden: lo firmado ata cada revisión
// a SU máquina. El grupo solo dice a quiénes se publicó y sirve para contar el
// resultado después.
type Group struct {
	ID      string    `json:"id"`
	Name    string    `json:"name"`
	Members []string  `json:"members"`
	Created time.Time `json:"created"`
	Updated time.Time `json:"updated"`
}

// GroupIn crea o reescribe un grupo (POST /v1/groups, PUT /v1/groups/{id}).
// Los miembros van enteros, no por diferencias: mandar la lista que se ve es
// lo único que no depende de qué versión del grupo tenía delante el cliente.
type GroupIn struct {
	Name    string   `json:"name"`
	Members []string `json:"members"`
}

// GroupMemberStatus es cómo le fue a UN equipo la última orden publicada a
// este grupo. Revision vacío significa que a ese equipo nunca se le publicó
// ninguna con esta etiqueta —pasa con quien entró al grupo después— y entonces
// State también va vacío: «no hay nada que contar» no es un estado de una
// orden que no existe.
type GroupMemberStatus struct {
	DeviceID   string `json:"device_id"`
	DeviceName string `json:"device_name"`
	Revoked    bool   `json:"revoked"`
	// Member dice si SIGUE en el grupo. Sale false cuando a ese equipo se le
	// publicó una orden con esta etiqueta y luego se le sacó del grupo: la
	// orden sigue puesta y sacarle no la retira, así que esconderle aquí
	// dejaría una orden viva sin nadie que la contara.
	Member   bool      `json:"member"`
	Revision string    `json:"revision,omitempty"`
	State    string    `json:"state,omitempty"`
	Reason   string    `json:"reason,omitempty"`
	Created  time.Time `json:"created,omitempty"`
	Updated  time.Time `json:"updated,omitempty"`
}

// GroupStatus es el estado por dispositivo dentro de un grupo
// (GET /v1/groups/{id}/status).
type GroupStatus struct {
	Group   Group               `json:"group"`
	Members []GroupMemberStatus `json:"members"`
}
