// Package blobs es el almacenamiento de blobs sellados del backend. El servidor
// nunca los lee: firma URLs para que el cliente los suba y baje directamente, y
// comprueba que existen antes de aceptar un snapshot.
package blobs

import (
	"context"
	"time"
)

// Blobs es lo que el servidor necesita del almacenamiento.
//
// Get y Put existen por el portal: una pestaña no puede hablar con el bucket.
// Su CSP solo deja salir hacia su propio origen y hacia Keycloak, y aunque se
// ampliara, el bucket tendría que responder CORS a la pestaña — configuración
// de despliegue que no se puede probar aquí y que falla en silencio. Así que
// para el portal el API hace de intermediario y los bytes pasan por él. Siguen
// sellados: el servidor mueve bultos que no sabe abrir, igual que antes.
type Blobs interface {
	PresignPut(ctx context.Context, key string, ttl time.Duration) (string, error)
	PresignGet(ctx context.Context, key string, ttl time.Duration) (string, error)
	// Head dice si key existe y cuánto mide. Que no exista no es un error.
	Head(ctx context.Context, key string) (size int64, exists bool, err error)
	// Get devuelve el contenido de key. Que no exista no es un error.
	Get(ctx context.Context, key string) (data []byte, exists bool, err error)
	// Put guarda data en key, sobrescribiendo. El contenido de una clave es
	// siempre el mismo (el id es su HMAC), así que reescribirla no pierde nada.
	Put(ctx context.Context, key string, data []byte) error
	Ping(ctx context.Context) error
}

// Key es la clave de un blob en el bucket, separada por usuario: conocer el id
// de un blob ajeno no da acceso a él, porque la clave lleva dentro a su dueño.
func Key(userID, blobID string) string { return "u/" + userID + "/b/" + blobID }
