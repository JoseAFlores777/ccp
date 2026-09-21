# 15. Identidad en Keycloak y cifrado en la bóveda: dos secretos, dos dueños

Fecha: 2026-09-21

## Estado

Aceptada. Implementa el §10.2 del spec
[2026-09-18-config-unificada-snapshots-nube](../superpowers/specs/2026-09-18-config-unificada-snapshots-nube-design.md)
(decisión D9) y es la otra mitad del [ADR 0013](0013-cloud-end-to-end-encryption.md): allí se decide que el
servidor no puede leer; aquí, de dónde sale la clave con la que sí se lee.

Implementada en F1 (`internal/cloud/crypt`, `internal/cloud/client`, `ccp cloud login|init|unlock|revoke`). La
envoltura por dispositivo (X25519) y la aprobación de un equipo desde otro son de F3.

## Contexto

Hay dos preguntas y se parecen lo justo para confundirlas:

- **«¿Quién eres?»** — la contesta el proveedor de identidad y da acceso al API.
- **«¿Puedes leer esto?»** — la contesta una clave, y sin ella el contenido es ruido.

Con Keycloak la separación no es una preferencia, es obligatoria. La contraseña se escribe **en la página de
Keycloak**, así que `ccp` nunca la ve y no puede derivar de ella la clave de cifrado. Y aunque pudiera: un
reset de contraseña —lo más normal del mundo, y algo que un admin del realm puede hacer— destruiría todos los
datos de la cuenta.

## Decisión

**Dos secretos, cada uno con su dueño, y ninguno sustituye al otro.**

- **«¿Quién eres?» es de Keycloak** (realm `ccp` del stack `ccp-cloud`): concesión de dispositivo
  (RFC 8628) para la CLI, sesión offline como credencial de larga vida del equipo, MFA, y usuarios gestionados
  en su consola. El API valida cada JWT contra el JWKS del realm (`iss`, `aud`, `exp` y firma).
- **«¿Puedes leer esto?» es de la bóveda.** Una AK aleatoria de 256 bits con dos envolturas que el servidor
  guarda sin poder abrir (`crypt.NewVault`):
  - una con una **frase de bóveda** (Argon2id con parámetros acotados), distinta de la contraseña de Keycloak;
  - otra con un **código de recuperación** de 160 bits en base32 por grupos (`vault.NewRecoveryCode`), que se
    enseña una sola vez y se deriva por HKDF, no por Argon2: ya es aleatorio, no hay nada que endurecer.

  Junto a las dos envolturas se guarda la **clave pública de firma**, y `checkAK` exige al desbloquear que la
  AK abierta corresponda a ella: una bóveda que abre pero firma con otra clave es la de otra cuenta, y
  adoptarla dejaría al equipo subiendo snapshots que su propia cuenta no podría verificar.
- **En F1 cada equipo guarda la AK ya desbloqueada en `<CCP_HOME>/cloud/vault.key` (0600)**, dentro de un
  directorio 0700. Para un mismo usuario de macOS, un archivo 0600 y un Keychain que `security` abre sin más
  protegen lo mismo; meterla en el Keychain añadiría una dependencia de plataforma sin cambiar a quién se
  resiste. La envoltura por dispositivo (X25519, con la privada en el Keychain) llega con la aprobación de un
  equipo desde otro, en F3.
- **Revocar un equipo lo expulsa del API en la siguiente petición**: el servidor comprueba el dispositivo de
  la cabecera en **cada** una, no solo al renovar el token. Es defensa en profundidad frente a un token de
  acceso que sigue vivo cinco minutos.
- **Y expulsa a la credencial, no solo a ese id.** El alta (`POST /v1/devices`) es el único camino que no
  exige cabecera de dispositivo, así que era la puerta de atrás: con el `token.json` copiado bastaba pedir un
  equipo nuevo para deshacer la revocación y volver a `GET /v1/vault` y a las URLs prefirmadas. Cada equipo
  guarda por eso el `sid` del token que lo dio de alta —la sesión de Keycloak, que sobrevive a los refrescos
  mientras el `jti` cambia en cada uno—; revocar uno revoca a sus hermanos de sesión y cierra el alta para
  esa sesión. Para volver hay que iniciar sesión otra vez, que es justo lo que el ladrón del archivo no
  puede hacer. La sesión offline sigue existiendo en Keycloak y se puede matar desde su consola: eso es lo
  que invalida además el token de refresco en sí, y ccp no lo hace por ti.
- **Revocar no borra la AK que ese equipo ya tiene**, y la CLI no finge que sí. Si se teme una filtración hay
  que rotar la AK, que es una acción explícita de F4.
- `ccp cloud logout` es la revocación de uno mismo: revoca este equipo y borra su token y su `vault.key`.

## Consecuencias

- **Hay dos secretos que recordar.** Es el precio de que el servidor no pueda leer; el ADR 0013 explica por
  qué ese precio vale la pena.
- **Perder la frase *y* el código de recuperación hace irrecuperables los datos de la nube.** No es una
  pérdida total: los snapshots locales siguen siendo la fuente primaria y la nube es la copia.
- Un reset de contraseña en Keycloak devuelve el acceso al API y **no** toca la bóveda: se vuelve a entrar y
  se desbloquea con la frase, como antes. Ese era el escenario que hundía la alternativa de derivar la clave
  de la contraseña.
- Un admin del realm puede suplantar a un usuario ante el API, y aun así **no puede leer nada**: vería ids
  opacos, textos sellados y firmas que no sabe hacer.
- Mientras no exista F3, dar de alta un equipo nuevo pasa siempre por escribir la frase (o el código). No hay
  «apruébalo desde el otro Mac».
