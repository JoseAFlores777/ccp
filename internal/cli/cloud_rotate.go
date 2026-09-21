package cli

// `ccp cloud rotate` — rotar las claves de ACCESO a la bóveda (spec §10.2).
// Frase y código de recuperación nuevos sobre la MISMA clave de cuenta.
//
// El reparto importa y está escrito en el mensaje que sale al final: rotar
// las llaves de la caja es barato y no toca nada de lo publicado; cambiar la
// caja —una AK nueva, con el recifrado de todo— es otra operación y no está.
// Por eso rotar NO expulsa a nadie: un equipo que ya tenía la AK abierta la
// sigue teniendo, incluido uno revocado que se quedara con su copia. Quien
// lee esto en la terminal tiene que poder saberlo sin abrir el código.

import (
	"errors"
	"fmt"
	"os"

	"github.com/mattn/go-isatty"

	"github.com/JoseAFlores777/ccp/internal/cloud/client"
	"github.com/JoseAFlores777/ccp/internal/cloud/crypt"
	"github.com/JoseAFlores777/ccp/internal/core/i18n"
)

// rotateSecret lee la frase NUEVA. Va por su propia variable de entorno y no
// por CCP_CLOUD_PASSPHRASE a propósito: reutilizar la del desbloqueo haría
// que un script «rotara» a la misma frase sin que nadie se enterara.
func (c cloudCmd) rotateSecret() (string, error) {
	if v := os.Getenv("CCP_CLOUD_NEW_PASSPHRASE"); v != "" {
		if len([]rune(v)) < snapMinPassphrase {
			return "", errors.New(i18n.T(c.lang, "cli.cloud.pass_short", snapMinPassphrase))
		}
		return v, nil
	}
	if !isatty.IsTerminal(os.Stdin.Fd()) {
		return "", errors.New(i18n.T(c.lang, "cli.cloud.rotate_needs_pass"))
	}
	v, err := promptSecret(c.err, i18n.T(c.lang, "cli.cloud.rotate_prompt"))
	if err != nil {
		return "", err
	}
	if len([]rune(v)) < snapMinPassphrase {
		return "", errors.New(i18n.T(c.lang, "cli.cloud.pass_short", snapMinPassphrase))
	}
	again, err := promptSecret(c.err, i18n.T(c.lang, "cli.cloud.pass_confirm"))
	if err != nil {
		return "", err
	}
	if again != v {
		return "", errors.New(i18n.T(c.lang, "cli.cloud.pass_mismatch"))
	}
	return v, nil
}

func (c cloudCmd) rotate(args []string) int {
	if _, ok := c.args(args, nil, nil, 0); !ok {
		return 1
	}
	// La AK sale del disco de este equipo: rotar no pide la frase vieja, que
	// es justo la que puede haberse perdido o filtrado.
	ak, err := c.files.LoadAK()
	if err != nil {
		if errors.Is(err, client.ErrLocked) {
			fmt.Fprintln(c.err, i18n.T(c.lang, "cli.cloud.rotate_locked"))
			return 1
		}
		return c.fail(err)
	}
	pass, err := c.rotateSecret()
	if err != nil {
		return c.fail(err)
	}
	_, cl, err := client.Session(c.ctx, c.files)
	if err != nil {
		return c.fail(err)
	}
	code, w, err := crypt.RewrapVault(ak, []byte(pass))
	if err != nil {
		return c.fail(err)
	}
	v, err := client.VaultToAPI(w)
	if err != nil {
		return c.fail(err)
	}
	// El código se enseña DESPUÉS de subir, al revés que en `init`: allí la
	// bóveda ya estaba creada de forma irrepetible y el código solo vivía en
	// memoria; aquí, si la subida falla, las envolturas viejas siguen en pie
	// y enseñar un código que no abre nada sería peor que no enseñarlo.
	if err := cl.RewrapVault(c.ctx, v); err != nil {
		return c.fail(err)
	}
	fmt.Fprintln(c.out, boldLine(c.out, i18n.T(c.lang, "cli.cloud.recovery_title")))
	fmt.Fprintln(c.out, "    "+boldLine(c.out, code))
	fmt.Fprintln(c.out, i18n.T(c.lang, "cli.cloud.recovery_hint"))
	fmt.Fprintln(c.out)
	fmt.Fprintln(c.out, okLine(c.out, i18n.T(c.lang, "cli.cloud.rotate_done")))
	fmt.Fprintln(c.out, mute(c.out, i18n.T(c.lang, "cli.cloud.rotate_scope")))
	return 0
}
