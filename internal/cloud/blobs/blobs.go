// Package blobs es el almacenamiento de blobs sellados del backend. El servidor
// nunca los lee: firma URLs para que el cliente los suba y baje directamente, y
// comprueba que existen antes de aceptar un snapshot.
package blobs

import (
	"context"
	"time"
)

// Blobs es lo que el servidor necesita del almacenamiento.
type Blobs interface {
	PresignPut(ctx context.Context, key string, ttl time.Duration) (string, error)
	PresignGet(ctx context.Context, key string, ttl time.Duration) (string, error)
	// Head dice si key existe y cuánto mide. Que no exista no es un error.
	Head(ctx context.Context, key string) (size int64, exists bool, err error)
	Ping(ctx context.Context) error
}

// Key es la clave de un blob en el bucket, separada por usuario: conocer el id
// de un blob ajeno no da acceso a él, porque la clave lleva dentro a su dueño.
func Key(userID, blobID string) string { return "u/" + userID + "/b/" + blobID }
