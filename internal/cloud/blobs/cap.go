package blobs

import (
	"errors"
	"io"

	"github.com/JoseAFlores777/ccp/internal/cloud/api"
)

// ErrTooLarge dice que el objeto pasa del tope y por eso no se devuelve.
//
// El tope hay que aplicarlo aquí porque no lo aplica nadie antes: la URL PUT
// prefirmada no ata el tamaño (SigV4 no firma Content-Length si no se fija), y
// los 64 MiB solo se comprueban al comprometer un snapshot, cosa que quien
// sube no está obligado a hacer. Así que el bucket puede tener un objeto de
// gigabytes bajo el prefijo de un usuario; leerlo entero para servirlo no es
// un error del que lo pide, es el proceso del API cayendo por OOM.
var ErrTooLarge = errors.New("blob por encima del tope")

// ReadCapped lee r hasta max bytes; con un byte más devuelve ErrTooLarge.
// max ≤ 0 significa el tope de siempre (api.MaxBlobBytes).
func ReadCapped(r io.Reader, max int64) ([]byte, error) {
	if max <= 0 {
		max = api.MaxBlobBytes
	}
	data, err := io.ReadAll(io.LimitReader(r, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > max {
		return nil, ErrTooLarge
	}
	return data, nil
}
