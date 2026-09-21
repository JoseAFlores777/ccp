package remote

// vault.go — la bóveda de un destino (§10.2) sin servidor de por medio. Es lo
// que hace que una carpeta o un bucket sea self-contained: dentro va TODO lo
// que hace falta para abrirlo —los parámetros del KDF y las dos envolturas de
// la clave de cuenta— y nada que sirva para abrirlo sin el secreto. Quien
// tiene acceso al almacenamiento tiene ids HMAC y bytes sellados.
//
// La clave de cuenta (AK) nunca se escribe aquí: sale de estas funciones para
// que la guarde quien las llama, en <CCP_HOME>/cloud, como hace la nube.

import (
	"context"
	"errors"

	"github.com/JoseAFlores777/ccp/internal/cloud/client"
	"github.com/JoseAFlores777/ccp/internal/cloud/crypt"
)

// InitVault crea la bóveda del destino y devuelve la AK desbloqueada y el
// código de recuperación, que hay que enseñar UNA vez: de él solo queda la
// envoltura, y no se puede reemitir.
//
// Si el destino ya tiene bóveda devuelve ErrVaultExists sin tocar nada. El
// caso real no es un ataque: son dos equipos apuntando a la misma carpeta, y
// el segundo tiene que desbloquear, no crear otra.
func InitVault(ctx context.Context, r Remote, passphrase []byte) (ak []byte, recovery string, err error) {
	// Se pregunta antes de generar nada para no gastar un código de
	// recuperación que nadie va a poder usar.
	if _, err := r.Vault(ctx); err == nil {
		return nil, "", ErrVaultExists
	} else if !isNoVault(err) {
		return nil, "", err
	}
	ak, recovery, w, err := crypt.NewVault(passphrase)
	if err != nil {
		return nil, "", err
	}
	// El mismo api.Vault que viaja a la nube: un destino y otro guardan la
	// bóveda en el mismo formato, así que un equipo que sabe abrir una sabe
	// abrir la otra.
	v, err := client.VaultToAPI(w)
	if err != nil {
		return nil, "", err
	}
	if err := r.PutVault(ctx, v); err != nil {
		return nil, "", err
	}
	return ak, recovery, nil
}

// Unlock abre la bóveda del destino con la frase y devuelve la AK.
func Unlock(ctx context.Context, r Remote, passphrase []byte) ([]byte, error) {
	w, err := wraps(ctx, r)
	if err != nil {
		return nil, err
	}
	return crypt.UnlockPassphrase(w, passphrase)
}

// UnlockRecovery la abre con el código de recuperación: la otra llave de la
// misma caja, para cuando la frase se perdió.
func UnlockRecovery(ctx context.Context, r Remote, code string) ([]byte, error) {
	w, err := wraps(ctx, r)
	if err != nil {
		return nil, err
	}
	return crypt.UnlockRecovery(w, code)
}

// Account abre la cuenta de un destino en un paso: es lo que necesita quien va
// a sellar o a abrir, que es casi todo el mundo.
func Account(ctx context.Context, r Remote, passphrase []byte) (*crypt.Account, error) {
	ak, err := Unlock(ctx, r, passphrase)
	if err != nil {
		return nil, err
	}
	return crypt.NewAccount(ak)
}

func wraps(ctx context.Context, r Remote) (crypt.Wraps, error) {
	v, err := r.Vault(ctx)
	if err != nil {
		return crypt.Wraps{}, err
	}
	return client.VaultFromAPI(v)
}

// isNoVault reconoce «aquí todavía no hay bóveda» venga de donde venga: de la
// carpeta (ErrNoVault) o de la nube, que lo dice con un 404 traducido.
func isNoVault(err error) bool { return errors.Is(err, ErrNoVault) }
